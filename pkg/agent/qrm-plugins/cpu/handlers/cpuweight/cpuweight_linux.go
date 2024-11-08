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

package cpuweight

import (
	coreconfig "github.com/kubewharf/katalyst-core/pkg/config"
	dynamicconfig "github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	cgroupcm "github.com/kubewharf/katalyst-core/pkg/util/cgroup/common"
	cgroupmgr "github.com/kubewharf/katalyst-core/pkg/util/cgroup/manager"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
)

func applyCPUWeightCgroupLevelConfig(conf *coreconfig.Configuration, emitter metrics.MetricEmitter) {
	if conf.CPUWeightConfigFile == "" {
		general.Errorf("CPUWeightConfigFile isn't configured")
		return
	}

	cpuWightCgroupLevelConfigs := make(map[string]uint64)
	err := general.LoadJsonConfig(conf.CPUWeightConfigFile, &cpuWightCgroupLevelConfigs)
	if err != nil {
		general.Errorf("load CPUWeightCgroupLevelConfig failed with error: %v", err)
		return
	}

	for relativeCgPath, weight := range cpuWightCgroupLevelConfigs {
		err := cgroupmgr.ApplyCPUWithRelativePath(relativeCgPath, &cgroupcm.CPUData{
			Shares: weight,
		})
		if err != nil {
			general.Errorf("ApplyCPUWeightWithRelativePath in relativeCgPath: %s failed with error: %v",
				relativeCgPath, err)
		} else {
			general.Infof("ApplyCPUWeightWithRelativePath weight: %d in relativeCgPath: %s successfully",
				weight, relativeCgPath)
			_ = emitter.StoreInt64(metricNameCPUWeight, int64(weight), metrics.MetricTypeNameRaw,
				metrics.ConvertMapToTags(map[string]string{
					"cgPath": relativeCgPath,
				})...)

		}
	}
}

func SetCPUWeight(conf *coreconfig.Configuration,
	_ interface{}, _ *dynamicconfig.DynamicAgentConfiguration,
	emitter metrics.MetricEmitter, metaServer *metaserver.MetaServer,
) {
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

	// SettingCPUWeight featuregate.
	if !conf.EnableSettingCPUWeight {
		general.Infof("SetCPUWeight disabled")
		return
	}

	// checking cgroup-level cpu.weight configuration.
	if len(conf.CPUWeightConfigFile) > 0 {
		applyCPUWeightCgroupLevelConfig(conf, emitter)
	}
}
