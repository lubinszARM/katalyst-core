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

package memprotection

import (
	"context"
	"fmt"
	"strconv"

	"github.com/kubewharf/katalyst-core/pkg/agent/qrm-plugins/commonstate"
	coreconfig "github.com/kubewharf/katalyst-core/pkg/config"
	dynamicconfig "github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	cgroupcm "github.com/kubewharf/katalyst-core/pkg/util/cgroup/common"
	cgroupmgr "github.com/kubewharf/katalyst-core/pkg/util/cgroup/manager"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
	"github.com/kubewharf/katalyst-core/pkg/util/native"
)

func convertMemRatioToBytes(memLimit, memRatio uint64) uint64 {
	limitInBytes := memLimit / 100 * memRatio
	// Any value related to cgroup memory limitation should be aligned with the page size.
	limitInBytes = general.AlignToPageSize(limitInBytes)

	return limitInBytes
}

func getMemProtectionInBytes(memLimit, memRatio uint64) uint64 {
	// Step1, convert ratio into bytes
	result := convertMemRatioToBytes(memLimit, memRatio)
	// Step2, performing specific operations within the memory.low
	// Notice: we limited memory.low between {128M, 2G}
	result = uint64(general.Clamp(float64(result), float64(cgroupMemoryLimit128M), float64(cgroupMemoryLimit2G)))

	return result
}

func getUserSpecifiedMemoryProtectionInBytes(relCgroupPath, ratio string) int64 {
	memStat, err := cgroupmgr.GetMemoryWithRelativePath(relCgroupPath)
	if err != nil {
		general.Warningf("getUserSpecifiedMemoryProtectionInBytes failed with err: %v", err)
		return 0
	}

	memProtectionRatio, err := strconv.Atoi(ratio)
	if err != nil {
		general.Warningf("getUserSpecifiedMemoryProtectionInBytes failed with err: %v", err)
		return 0
	}

	bytes := getMemProtectionInBytes(memStat.Limit, uint64(memProtectionRatio))
	return int64(bytes)
}

func applyMemSoftLimitQoSLevelConfig(conf *coreconfig.Configuration,
	emitter metrics.MetricEmitter, metaServer *metaserver.MetaServer) {
	if conf.MemSoftLimitQoSLevelConfigFile == "" {
		general.Infof("no MemSoftLimitQoSLevelConfigFile found")
		return
	}

	var extraControlKnobConfigs commonstate.ExtraControlKnobConfigs
	if err := general.LoadJsonConfig(conf.MemSoftLimitQoSLevelConfigFile, &extraControlKnobConfigs); err != nil {
		general.Errorf("MemSoftLimitQoSLevelConfigFile load failed:%v", err)
		return
	}
	ctx := context.Background()
	podList, err := metaServer.GetPodList(ctx, native.PodIsActive)
	if err != nil {
		general.Infof("get pod list failed: %v", err)
		return
	}

	for _, pod := range podList {
		if pod == nil {
			general.Warningf("get nil pod from metaServer")
			continue
		}
		qosConfig := conf.QoSConfiguration
		qosLevel, err := qosConfig.GetQoSLevelForPod(pod)
		if err != nil {
			general.Warningf("GetQoSLevelForPod failed:%v", err)
			continue
		}
		qosLevelDefaultValue, ok := extraControlKnobConfigs[controlKnobKeyMemSoftLimit].QoSLevelToDefaultValue[qosLevel]
		if !ok {
			general.Warningf("no QoSLevelToDefaultValue in extraControlKnobConfigs")
			continue
		}

		for _, containerStatus := range pod.Status.ContainerStatuses {
			podUID, containerID := string(pod.UID), native.TrimContainerIDPrefix(containerStatus.ContainerID)
			relCgPath, err := cgroupcm.GetContainerRelativeCgroupPath(podUID, containerID)
			if err != nil {
				general.Warningf("GetContainerRelativeCgroupPath failed, pod=%v, container=%v, err=%v", podUID, containerID, err)
				continue
			}
			softLimit := getUserSpecifiedMemoryProtectionInBytes(relCgPath, qosLevelDefaultValue)
			if softLimit == 0 {
				general.Warningf("getUserSpecifiedMemoryProtectionBytes return 0")
				continue
			}

			fmt.Printf("BBLU888 set cgpath:%v, softlimit=%v.\n", relCgPath, softLimit)
			var data *cgroupcm.MemoryData
			data = &cgroupcm.MemoryData{SoftLimitInBytes: softLimit}
			if err := cgroupmgr.ApplyMemoryWithRelativePath(relCgPath, data); err != nil {
				general.Warningf("ApplyMemoryWithRelativePath failed, cgpath=%v, err=%v", relCgPath, err)
				continue
			}
		}
	}
}

func MemProtectionTaskFunc(conf *coreconfig.Configuration,
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

	// SettingMemProtection featuregate.
	if !conf.EnableSettingMemProtection {
		general.Infof("EnableSettingMemProtection disabled")
		return
	}

	// checking qos-level memory.low configuration.
	if len(conf.MemSoftLimitQoSLevelConfigFile) > 0 {
		applyMemSoftLimitQoSLevelConfig(conf, emitter, metaServer)
	}
}
