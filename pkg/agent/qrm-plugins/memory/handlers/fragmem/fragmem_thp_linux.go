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
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/errors"

	memconsts "github.com/kubewharf/katalyst-core/pkg/agent/qrm-plugins/memory/consts"
	coreconfig "github.com/kubewharf/katalyst-core/pkg/config"
	dynamicconfig "github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	malachiteclient "github.com/kubewharf/katalyst-core/pkg/metaserver/agent/metric/provisioner/malachite/client"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
)

const (
	// thpDefaultPeriod is the default execution interval for THP-related handler.
	// It is used only for documentation/reference; the actual scheduling period
	// is configured at registration time.
	thpDefaultPeriod = 60 * time.Second
)

// SetMemTHP is a framework handler for THP related tuning.
//
// NOTE: the strategy implementation is intentionally left empty for now.
// You can add the real THP policy logic into doMemTHP in follow-up changes.
func SetMemTHP(conf *coreconfig.Configuration,
	_ interface{}, _ *dynamicconfig.DynamicAgentConfiguration,
	emitter metrics.MetricEmitter, metaServer *metaserver.MetaServer,
) {
	general.Infof("called")

	var errList []error
	defer func() {
		_ = general.UpdateHealthzStateByError(memconsts.SetMemTHP, errors.NewAggregate(errList))
	}()

	if conf == nil || emitter == nil || metaServer == nil {
		general.Errorf("nil input, conf:%v, emitter:%v, metaServer:%v", conf, emitter, metaServer)
		return
	}

	// Reuse existing feature gate for now to avoid introducing new config knobs.
	// If we need an independent switch later, add a dedicated option under MemoryQRMPluginConfig.
	if !conf.EnableSettingFragMem {
		general.Infof("EnableSettingFragMem disabled")
		return
	}

	// TODO: Implement THP strategy here.
	// Keep the framework minimal: read necessary metrics/state, decide, and apply.
	doMemTHP(conf, metaServer, emitter)
}

func doMemTHP(_ *coreconfig.Configuration, metaServer *metaserver.MetaServer, emitter metrics.MetricEmitter) {
	if metaServer == nil || emitter == nil {
		return
	}

	// Read mem_order_scores from Malachite system/memory extfrag.
	mc := malachiteclient.NewMalachiteClient(metaServer.PodFetcher, emitter)
	stats, err := mc.GetSystemMemoryStats()
	if err != nil {
		general.Infof("get system memory stats failed, err=%v", err)
		return
	}

	for _, ext := range stats.ExtFrag {
		orderScores := ext.MemOrderScores
		sort.Slice(orderScores, func(i, j int) bool { return orderScores[i].Order < orderScores[j].Order })

		var b strings.Builder
		for i, os := range orderScores {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(fmt.Sprintf("%d:%d", os.Order, os.Score))
		}
		general.Infof("THP extfrag numa=%d mem_order_scores=[%s]", ext.ID, b.String())
	}
}
