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

package sockmem

import (
	"context"
	"fmt"

	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/util/errors"

	memconsts "github.com/kubewharf/katalyst-core/pkg/agent/qrm-plugins/memory/consts"
	coreconfig "github.com/kubewharf/katalyst-core/pkg/config"
	dynamicconfig "github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic"
	coreconsts "github.com/kubewharf/katalyst-core/pkg/consts"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	"github.com/kubewharf/katalyst-core/pkg/metaserver/agent/metric/helper"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	"github.com/kubewharf/katalyst-core/pkg/util/cgroup/common"
	cgroupcm "github.com/kubewharf/katalyst-core/pkg/util/cgroup/common"
	cgroupmgr "github.com/kubewharf/katalyst-core/pkg/util/cgroup/manager"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
	"github.com/kubewharf/katalyst-core/pkg/util/native"
)

type SockMemConfig struct {
	globalTCPMemRatio float64
	cgroupTCPMemRatio float64
}

func updateTCPMemLimit(tcpMem []uint64, memTotal, tcpMemRatio uint64, tcpMemFile string) error {
	pageSize := uint64(unix.Getpagesize())
	newUpperLimit := (memTotal / pageSize / 100) * tcpMemRatio

	// Step1: Check if the new limit is different from the current value
	if newUpperLimit != tcpMem[0] {
		general.Infof("Updating host tcp_mem, ratio=%v, newLimit=%d, oldLimit=%d", tcpMemRatio, newUpperLimit, tcpMem[0])

		// Set new memory limits
		for i := range tcpMem {
			tcpMem[i] = newUpperLimit
		}

		// Step2: Write the new values to the file
		if err := setHostTCPMemFile(tcpMemFile, tcpMem); err != nil {
			return err
		}
	}

	return nil
}

func setHostTCPMem(emitter metrics.MetricEmitter, memTotal uint64, sockMemConfig *SockMemConfig) error {
	// Validate inputs
	if sockMemConfig == nil {
		return fmt.Errorf("sockMemConfig is nil")
	}

	tcpMemRatio := sockMemConfig.globalTCPMemRatio
	tcpMem, err := getHostTCPMemFile(hostTCPMemFile)
	if err != nil {
		return err
	}

	if len(tcpMem) != 3 {
		return fmt.Errorf("tcpMem array must have exactly three elements, got %d", len(tcpMem))
	}

	err = updateTCPMemLimit(tcpMem, memTotal, uint64(tcpMemRatio), hostTCPMemFile)
	if err != nil {
		return err
	}
	_ = emitter.StoreInt64(metricNameTCPMemoryHost, int64(tcpMemRatio), metrics.MetricTypeNameRaw)
	return nil
}

func setCg1TCPMem(emitter metrics.MetricEmitter, podUID, containerID string, memLimit, memTCPLimit int64, sockMemConfig *SockMemConfig) error {
	newMemTCPLimit := memLimit / 100 * int64(sockMemConfig.cgroupTCPMemRatio)
	newMemTCPLimit = alignToPageSize(newMemTCPLimit)
	newMemTCPLimit = int64(general.Clamp(float64(newMemTCPLimit), cgroupTCPMemMin2G, kernSockMemAccountingOn))

	cgroupPath, err := cgroupcm.GetContainerRelativeCgroupPath(podUID, containerID)
	if err != nil {
		return err
	}
	if newMemTCPLimit != memTCPLimit {
		_ = cgroupmgr.ApplyMemoryWithRelativePath(cgroupPath, &cgroupcm.MemoryData{
			TCPMemLimitInBytes: newMemTCPLimit,
		})
		general.Infof("Apply TCPMemLimitInBytes: %v, old value=%d, new value=%d", cgroupPath, memTCPLimit, newMemTCPLimit)
		_ = emitter.StoreInt64(metricNameTCPMemoryCgroup, newMemTCPLimit, metrics.MetricTypeNameRaw,
			metrics.ConvertMapToTags(map[string]string{
				"podUID":      podUID,
				"containerID": containerID,
			})...)
	}
	return nil
}

/*
SetSockMemLimit is the unified solution for tcpmem limitation.
* it includes 3 parts:
* 1, set the global tcpmem limitation by changing net.ipv4.tcp_mem.
* 2, do nothing under cgroupv2.
* 3, set the cgroup tcpmem limitation under cgroupv1.
*/
func SetSockMemLimit(conf *coreconfig.Configuration,
	_ interface{}, _ *dynamicconfig.DynamicAgentConfiguration,
	emitter metrics.MetricEmitter, metaServer *metaserver.MetaServer,
) {
	general.Infof("called")

	var errList []error
	defer func() {
		_ = general.UpdateHealthzStateByError(memconsts.SetSockMem, errors.NewAggregate(errList))
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

	// SettingSockMem featuregate.
	if !conf.EnableSettingSockMem {
		general.Infof("SetSockMemLimit disabled")
		return
	}

	sockMemConfig := SockMemConfig{}
	sockMemConfig.globalTCPMemRatio = general.Clamp(float64(conf.SetGlobalTCPMemRatio), globalTCPMemRatioMin, globalTCPMemRatioMax)
	sockMemConfig.cgroupTCPMemRatio = general.Clamp(float64(conf.SetCgroupTCPMemRatio), cgroupTCPMemRatioMin, cgroupTCPMemRatioMax)
	/*
	 * Step1, set the [limit] value for host net.ipv4.tcp_mem.
	 *
	 * Description of net.ipv4.tcp_mem:
	 * It includes 3 parts: min, pressure, limit.
	 * The format is like the following:
	 * net.ipv4.tcp_mem = [min] [pressure] [limit]
	 *
	 * Each parts means:
	 * [min]: represents the minimum number of pages allowed in the queue.
	 * [pressure]: represents the threshold at which the system considers memory
	 *   to be under pressure due to TCP socket usage. When the memory usage reaches
	 *   this value, the system may start taking actions like cleaning up or reclaiming memory.
	 * [limit]: indicates the maximum number of pages allowed in the queue.
	 */
	// 0 means skip this feature.
	if conf.SetGlobalTCPMemRatio != 0 {
		_ = setHostTCPMem(emitter, metaServer.MemoryCapacity, &sockMemConfig)
	}
	// Step2, do nothing for cg2.
	// In cg2, tcpmem is accounted together with other memory(anon, kernel, file...).
	// So, we don't need to limit it.
	if common.CheckCgroup2UnifiedMode() {
		general.Infof("skip setSockMemLimit in cg2 env")
		return
	}

	// Step3, set tcp_mem accounting for pods under cgroupv1.
	// In cg1, tcpmem is accounted for separately from other memory(anon, kernel, file..).
	// So, we need to limit it by manually.
	// 0 means skip this feature.
	if conf.SetCgroupTCPMemRatio == 0 {
		return
	}

	podList, err := metaServer.GetPodList(context.Background(), native.PodIsActive)
	if err != nil {
		errList = append(errList, err)
		general.Errorf("get pod list failed, err: %v", err)
		return
	}

	for _, pod := range podList {
		if pod == nil {
			general.Errorf("get nil pod from metaServer")
			continue
		}
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if !containerStatus.Ready {
				continue
			}

			podUID, containerID := string(pod.UID), native.TrimContainerIDPrefix(containerStatus.ContainerID)
			memLimit, err := helper.GetPodMetric(metaServer.MetricsFetcher, emitter, pod, coreconsts.MetricMemLimitContainer, -1)
			if err != nil {
				continue
			}
			memTCPLimit, err := helper.GetPodMetric(metaServer.MetricsFetcher, emitter, pod, coreconsts.MetricMemTCPLimitContainer, -1)
			if err != nil {
				continue
			}

			err = setCg1TCPMem(emitter, podUID, containerID, int64(memLimit), int64(memTCPLimit), &sockMemConfig)
			if err != nil {
				errList = append(errList, err)
			}
		}
	}
}
