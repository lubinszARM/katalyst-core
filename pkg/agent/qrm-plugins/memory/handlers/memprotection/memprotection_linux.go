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
	"strconv"

	"github.com/kubewharf/katalyst-core/pkg/agent/qrm-plugins/commonstate"
	coreconfig "github.com/kubewharf/katalyst-core/pkg/config"
	dynamicconfig "github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic"
	coreconsts "github.com/kubewharf/katalyst-core/pkg/consts"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	"github.com/kubewharf/katalyst-core/pkg/metaserver/agent/metric/helper"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	cgroupcm "github.com/kubewharf/katalyst-core/pkg/util/cgroup/common"
	cgroupmgr "github.com/kubewharf/katalyst-core/pkg/util/cgroup/manager"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
	"github.com/kubewharf/katalyst-core/pkg/util/native"
)

func getUserSpecifiedMemoryProtectionInBytes(memUsage, memFile float64, ratioUser string) int64 {
	ratio, err := strconv.Atoi(ratioUser)
	if err != nil {
		general.Infof("Atoi failed with err: %v", err)
		return 0
	}
	if ratio < 100 {
		ratio = 100 - ratio // reserv ratio part of file memory
	} else {
		ratio = 100 - cgroupMemoryReserveFileDefaultRatio
	}
	softLimit := int64(memUsage - (memFile/100.0)*float64(ratio))

	return softLimit
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
		if conf.QoSConfiguration == nil {
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
			continue
		}

		for _, containerStatus := range pod.Status.ContainerStatuses {
			podUID, containerID := string(pod.UID), native.TrimContainerIDPrefix(containerStatus.ContainerID)

			/*
			 * I hope to protect cgroup from System-Thrashing(insufficient hot file memory)
			 * through memory.low.
			 */
			// Step1, get cgroup memory.usage, file-memoory, inactive-file-memory.
			memUsage, err := helper.GetPodMetric(metaServer.MetricsFetcher, emitter, pod, coreconsts.MetricMemUsageContainer, -1)
			if err != nil {
				general.Infof("memory usage not found:%v..\n", podUID)
				continue
			}
			memFile, err := helper.GetPodMetric(metaServer.MetricsFetcher, emitter, pod, coreconsts.MetricMemCacheContainer, -1)
			if err != nil {
				general.Infof("file memory usage not found:%v..\n", podUID)
				continue
			}
			memFileInactive, err := helper.GetPodMetric(metaServer.MetricsFetcher, emitter, pod, coreconsts.MetricMemFileInactiveContainer, -1)
			if err != nil {
				general.Infof("file memory cold part usage not found:%v..\n", podUID)
				continue
			}

			// Step2, Reserve a certain ratio of file memory for high-QoS cgroups.
			softLimit := getUserSpecifiedMemoryProtectionInBytes(memUsage, memFile, qosLevelDefaultValue)
			if softLimit == 0 {
				general.Warningf("getUserSpecifiedMemoryProtectionBytes return 0")
				continue
			}

			// Step3, I don't want to hurt existing hot file-memory.
			// If the reserve file memory is not sufficient for current hot file-memory,
			// then the final memory.low will be based on current hot file-memory.
			minSoftLimit := memUsage - memFileInactive
			maxSoftLimit := memUsage
			softLimit = int64(general.Clamp(float64(softLimit), minSoftLimit, maxSoftLimit))

			// Step4, OK. Set the value for memory.low.
			relCgPath, err := cgroupcm.GetContainerRelativeCgroupPath(podUID, containerID)
			if err != nil {
				general.Warningf("GetContainerRelativeCgroupPath failed, pod=%v, container=%v, err=%v", podUID, containerID, err)
				continue
			}
			var data *cgroupcm.MemoryData
			data = &cgroupcm.MemoryData{SoftLimitInBytes: softLimit}
			if err := cgroupmgr.ApplyMemoryWithRelativePath(relCgPath, data); err != nil {
				general.Warningf("ApplyMemoryWithRelativePath failed, cgpath=%v, err=%v", relCgPath, err)
				continue
			}

			_ = emitter.StoreInt64(metricNameMemLow, softLimit, metrics.MetricTypeNameRaw,
				metrics.ConvertMapToTags(map[string]string{
					"podUID":      podUID,
					"containerID": containerID,
				})...)
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
