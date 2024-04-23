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

package iolatency

import (
	"sync"

	"github.com/kubewharf/katalyst-core/pkg/config"
	coreconfig "github.com/kubewharf/katalyst-core/pkg/config"
	dynamicconfig "github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	"github.com/kubewharf/katalyst-core/pkg/util/cgroup/common"
	"github.com/kubewharf/katalyst-core/pkg/util/cgroup/manager"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
)

var (
	initializeOnce sync.Once
)

func IOLatencyQoSTaskFunc(conf *coreconfig.Configuration,
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

	if !common.CheckCgroup2UnifiedMode() {
		general.Infof("not in cgv2 environment, skip IOLatencyQoSFunc")
		return
	}

	if !conf.EnableIOLatencyQoS {
		general.Infof("IOLatencyQoS disabled")
		initializeOnce.Do(func() {
			applyIOLatencyQoSCgroupLevelConfig(conf, 1)
		})
		return
	}

	initializeOnce.Do(func() {
		applyIOLatencyQoSCgroupLevelConfig(conf, 0)
	})
}

func applyIOLatencyQoSCgroupLevelConfig(conf *config.Configuration, disable int) {
	if conf.IOLatencyCgroupLevelConfigFile == "" {
		general.Errorf("IOLatencyCgroupLevelConfigFile isn't configured")
		return
	}

	ioLatencyQoSCgroupLevelConfigs := make(map[string]uint64)
	err := general.LoadJsonConfig(conf.IOLatencyCgroupLevelConfigFile, &ioLatencyQoSCgroupLevelConfigs)
	if err != nil {
		general.Errorf("load IOLatencyQoSCgroupLevelConfig failed with error: %v", err)
		return
	}

	for relativeCgPath, target := range ioLatencyQoSCgroupLevelConfigs {
		absCgroupPath := common.GetAbsCgroupPath(common.CgroupSubsysIO, relativeCgPath)
		// get each device id from io.stat
		devIDToIOStat, err := manager.GetIOStatWithAbsolutePath(absCgroupPath)
		if err != nil {
			general.Errorf("GetIOStatWithAbsolutePath failed with error: %v", err)
			return
		}

		for devID, _ := range devIDToIOStat {
			latency := target
			if disable == 1 {
				latency = 0
			}
			err := manager.ApplyIOLatencyWithAbsolutePath(absCgroupPath, devID, latency)
			if err != nil {
				general.Errorf("ApplyIOLatencyWithRelativePath for devID: %s in relativeCgPath: %s failed with error: %v",
					devID, relativeCgPath, err)
			}
		}
	}
}
