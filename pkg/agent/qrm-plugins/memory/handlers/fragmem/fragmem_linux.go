//go:build linux
// +build linux

/*
Copyright 2022 The Katalyst Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package fragmem

import (
	"fmt"
	"math"
	"os"
	"time"

	coreconfig "github.com/kubewharf/katalyst-core/pkg/config"
	dynamicconfig "github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic"
	"github.com/kubewharf/katalyst-core/pkg/consts"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	"github.com/kubewharf/katalyst-core/pkg/metaserver/agent/metric/helper"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
)

/* SetMemCompact is the unified solution for memory compaction.
* it includes 3 parts:
* 1, set the threshold of fragmentation score that triggers synchronous memory compaction in the memory slow path.
* 2, if has proactive compaction feature, then set the threshold of fragmentation score for asynchronous memory compaction through compaction_proactiveness.
* 3, if no proactive compaction feature, then use the async threshold of fragmentation score to trigger manually memory compaction.
 */
func SetMemCompact(conf *coreconfig.Configuration,
	_ interface{}, _ *dynamicconfig.DynamicAgentConfiguration,
	emitter metrics.MetricEmitter, metaServer *metaserver.MetaServer) {
	general.Infof("called")

	if conf == nil {
		general.Errorf("nil extraConf")
		return
	} else if emitter == nil {
		general.Errorf("nil emitter")
		return
	} else if metaServer == nil {
		general.Errorf("nil metaServer")
		return
	}

	// EnableSettingMemCompact featuregate.
	if !conf.EnableSettingFragMem {
		general.Infof("EnableSettingFragMem disabled")
		return
	}

	/*
	 * Step1, set the threshold of fragmentation score that triggers synchronous memory compaction in the memory slow path.
	 *
	 * Description of vm.extfrag_threshold:
	 * vm.extfrag_threshold is a parameter related to synchronous memory compaction in the slow path of user memory allocation.
	 * When the user process memory allocation fails, the buddy system will compare the order's memory fragmentation score.
	 * If it is higher than vm.extfrag_threshold, it will trigger a synchronous memory grooming behavior.
	 */
	// 0 means skip this feature.
	if conf.SetMemFragScoreSync != 0 {
		_ = setHostExtFragFile(hostFragScoreSyncThreshold, conf.SetMemFragScoreSync)
	}

	// Step2, set proactive compaction.
	// 0 means skip step 2 & 3.
	if conf.SetMemFragScoreAsync == 0 {
		return
	}

	asyncWatermark := uint64(general.Clamp(float64(conf.SetMemFragScoreAsync), fragScoreMin, fragScoreMax))
	_, err := os.Stat(hostFragScoreAsyncThreshold)
	if err == nil {
		_ = setHostProactiveCompctFile(hostFragScoreAsyncThreshold, asyncWatermark)
		return
	}

	// Step3, if no proactive compaction feature, then a user space solution for memory compaction will be triggered.
	// Step3.0, avoid too much system pressure.
	load, err := helper.GetNodeMetricWithTime(metaServer.MetricsFetcher, emitter, consts.MetricLoad15MinSystem)
	if err != nil {
		fmt.Printf("BBLU got load failed..\n")
		return
	}
	fmt.Printf("BBLU got load:%v..\n", load)
	if load.Value > loadGate {
		return
	}
	// Step3.1, get the fragmentation score.
	fragScore, err := gatherFragScore(hostFragScoreFile)
	if err != nil {
		return
	}
	fmt.Printf("BBLU got frag score:%v.\n", fragScore)
	maxScore := fragScore.MaxValue
	avgScore := fragScore.AverageValue

	// Step3.2, async user space memory compaction will be trigger while exceeding the conf.SetMemFragScoreAsync.
	if maxScore < int(asyncWatermark) {
		return
	}
	setHostMemCompact(hostMemCompactFile)

	// Step3.3, get the fragmentation score again.
	fragScore, err = gatherFragScore(hostFragScoreFile)
	if err != nil {
		return
	}
	newAvgScore := fragScore.AverageValue

	// Step3.4, compare the new and old average fragmentation core to avoid ineffective compaction.
	diff := math.Abs(float64(newAvgScore) - float64(avgScore))
	if diff > minFragScoreGap {
		return
	}
	time.Sleep(delayCompactTime)
}
