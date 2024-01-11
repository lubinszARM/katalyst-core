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
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/errors"

	memconsts "github.com/kubewharf/katalyst-core/pkg/agent/qrm-plugins/memory/consts"
	coreconfig "github.com/kubewharf/katalyst-core/pkg/config"
	dynamicconfig "github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic"
	"github.com/kubewharf/katalyst-core/pkg/consts"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	"github.com/kubewharf/katalyst-core/pkg/metaserver/agent/metric/helper"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
)

var (
	delayTimes int
	mu         sync.RWMutex
)

// SetDelayValue sets the value of the global delayTimes
func SetDelayTimes(value int) {
	mu.Lock()
	defer mu.Unlock()
	delayTimes = value
}

// GetDelayValue returns the value of the global delayTimes
func GetDelayTimes() int {
	mu.RLock()
	defer mu.RUnlock()
	return delayTimes
}

/* SetMemCompact is the unified solution for memory compaction.
* it includes 3 parts:
* 1, set the threshold of fragmentation score that triggers synchronous memory compaction in the memory slow path.
* 2, if has proactive compaction feature, then set the threshold of fragmentation score for asynchronous memory compaction through compaction_proactiveness.
* 3, if no proactive compaction feature, then use the async threshold of fragmentation score to trigger manually memory compaction.
 */
func SetMemCompact(conf *coreconfig.Configuration,
	_ interface{}, _ *dynamicconfig.DynamicAgentConfiguration,
	emitter metrics.MetricEmitter, metaServer *metaserver.MetaServer,
) {
	general.Infof("called")

	var errList []error
	defer func() {
		_ = general.UpdateHealthzStateByError(memconsts.SetMemCompact, errors.NewAggregate(errList))
	}()

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

	delay := GetDelayTimes()
	if delay > 0 {
		general.Infof("No memory fragmentation in this node, skip this scanning cycle, delay=%d", delay)
		delay--
		SetDelayTimes(delay)
		return
	}

	// EnableSettingMemCompact featuregate.
	if !conf.EnableSettingFragMem {
		general.Infof("EnableSettingFragMem disabled")
		return
	}

	// Step1, check proactive compaction.
	// if proactive compaction feature enabled, then return.
	if !checkCompactionProactivenessDisabled(hostCompactProactivenessFile) {
		general.Infof("proactive compaction enabled, then return")
		return
	}

	// Step2, if proactive compaction feature was disabled, then a user space solution for memory compaction will be triggered.
	// Step2.0, avoid too much system pressure.
	load, err := helper.GetNodeMetricWithTime(metaServer.MetricsFetcher, emitter, consts.MetricLoad5MinSystem)
	if err != nil {
		return
	}
	numCPU := metaServer.CPUTopology.NumCPUs
	loadPerCPU := int(load.Value) * 100 / numCPU
	general.Infof("Host load info: load:%v, numCPU:%v, loadPerCPU:%v", load.Value, numCPU, loadPerCPU)
	if loadPerCPU > minHostLoad {
		return
	}
	// Step2.1, get the fragmentation score.
	/*for _, numaID := range metaServer.CPUDetails.NUMANodes().ToSliceNoSortInt() {
		general.Infof("BBLU got numa:%v.\n", numaID)
	}*/
	asyncWatermark := uint64(general.Clamp(float64(conf.SetMemFragScoreAsync), fragScoreMin, fragScoreMax))
	/*fragScores, err := GetNumaFragScore(hostFragScoreFile)
	if err != nil {
		general.Errorf("gatherFragScore failed:%v.\n", err)
		return
	}*/
	// Step2.2, async user space memory compaction will be trigger while exceeding the conf.SetMemFragScoreAsync.
	//for _, scoreInfo := range fragScores {
	for _, numaID := range metaServer.CPUDetails.NUMANodes().ToSliceNoSortInt() {
		score, err := helper.GetNumaMetricWithTime(metaServer.MetricsFetcher, emitter, consts.MetricMemFragScoreNuma, numaID)
		if err != nil {
			general.Errorf("BBLU failed to get frag score")
			continue
		}
		fragScore := int(score.Value)
		general.Infof("BBLU Node fragScore info: node:%d, fragScore:%d, fragScoreGate:%d", numaID, fragScore, asyncWatermark)
		if fragScore < int(asyncWatermark) {
			continue
		}
		nodeId := numaID

		// Step 2.3, check if kcompactd is in D state
		if isCommandInDState(commandKcompactd) {
			general.Infof("kcompactd is in D state")
			return
		}

		// Step 2.4, do memory compaction in node level
		_ = emitter.StoreInt64(metricNameMemoryCompaction, 1, metrics.MetricTypeNameRaw)
		setHostMemCompact(nodeId)

		time.Sleep(10 * time.Second)
		newScore, err := helper.GetNumaMetricWithTime(metaServer.MetricsFetcher, emitter, consts.MetricMemFragScoreNuma, numaID)
		if err != nil {
			continue
		}
		newFragScore := int(newScore.Value)
		general.Infof("Node fragScore new info: node:%d, fragScore:%d", numaID, newFragScore)
		// compare the new and old average fragmentation core to avoid ineffective compaction.
		if newFragScore >= int(asyncWatermark-minFragScoreGap) {
			general.Infof("No memory fragmentation in this node, increase the scanning cycle")
			SetDelayTimes(delayCompactTimes)
		}
	}
}