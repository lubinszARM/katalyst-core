// todo: All implementations here are only used to replace exactly
//  the same logic as current-versioned sysprobe provides, but the sysprobe-implementation
//  itself is a kind of mess and lack of reasonable and clear justification.
//  So we need to re-design the entire mechanism for IO-QoS when the functionality
//  in kernel is ready, and maybe we also need to clarify more explicit APIs/Enhancements
//  to support this.
//  Before that, we will not allow for any newly-supplemented strategies for IO-QoS
//  (except only for testing requirements).
//
// todo (with @zhanghaoyu.zhy): the unclear parts include but not limit as follows:
//  1. lack of explicit APIs/Enhancements
//  2. it seems some config-files can be alerted dynamically but others not, and lacking of clear explanations or comments
//  3. too many hard-codes for dev/file or something like that
//  4. the meaning for some flags are kind of confusing
//  ...

package memprotection

import (
	"context"
	"fmt"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"

	"github.com/kubewharf/katalyst-core/pkg/config"
	coreconfig "github.com/kubewharf/katalyst-core/pkg/config"
	dynamicconfig "github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	cgroupcm "github.com/kubewharf/katalyst-core/pkg/util/cgroup/common"
	cgroupmgr "github.com/kubewharf/katalyst-core/pkg/util/cgroup/manager"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
	"github.com/kubewharf/katalyst-core/pkg/util/native"
)

const (
	EnableCGMemProtectionPeriodicalHandlerName = "MemProtection"

	memProtectionLow = "low"
	memProtectionMin = "min"

	cgMemProtectionMin = 104857600.0  // 100M
	cgMemProtectionMax = 2147483648.0 // 2G

	defaultTimeout = 5
)

type MemProtectionConfig struct {
	MemProtectionLow map[string]uint64 `json:"low"`
	MemProtectionMin map[string]uint64 `json:"min"`
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

	if !conf.EnableCgMemProtection {
		general.Infof("MemProtection disabled")
		return
	}

	applyMemProtectionK8sLevelConfig(conf, metaServer, 0)
}

func applyMemProtectionK8sLevelConfig(conf *config.Configuration, metaServer *metaserver.MetaServer, disable int) {
	if !conf.EnableCgMemProtection {
		general.Infof("EnableSetCgMemProtection disabled, skip applyMemProtectionK8sLevelConfig")
		return
	} else if conf.CgMemProtectionK8sLevelConfigFile == "" {
		general.Errorf("CgMemProtectionK8sLevelConfigFile isn't configured")
		return
	}

	var memProtectionConfig MemProtectionConfig
	if err := general.LoadJsonConfig(conf.CgMemProtectionK8sLevelConfigFile, &memProtectionConfig); err != nil {
		general.Errorf("CgMemProtectionK8sLevelConfigFile load failed:%v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout*time.Second)
	defer cancel()

	pods, err := metaServer.GetPodList(ctx, nil)
	if err != nil {
		general.Errorf("GetPodList failed with error: %v", err)
		return
	}

	if memProtectionConfig.MemProtectionLow != nil {
		updateMemLimitForPods(memProtectionConfig.MemProtectionLow, pods, memProtectionLow)
	}

	if memProtectionConfig.MemProtectionMin != nil {
		updateMemLimitForPods(memProtectionConfig.MemProtectionMin, pods, memProtectionMin)
	}
}

func updateMemLimitForPods(memProtectionConfigs map[string]uint64, pods []*v1.Pod, limitType string) {
	for _, pod := range pods {
		updateMemLimitForPod(memProtectionConfigs, pod, limitType)
	}
}

func updateMemLimitForPod(memProtectionConfigs map[string]uint64, pod *v1.Pod, limitType string) {
	if pod == nil {
		return
	}

	for _, containerStatus := range pod.Status.ContainerStatuses {
		podUID, containerID := string(pod.UID), native.TrimContainerIDPrefix(containerStatus.ContainerID)
		relCgroupPath, err := cgroupcm.GetContainerRelativeCgroupPath(podUID, containerID)
		if err != nil {
			general.Errorf("GetContainerRelativeCgroupPath %v/%v err %v", podUID, containerID, err)
			continue
		}

		updateMemLimitForContainer(memProtectionConfigs, relCgroupPath, limitType)
	}
}

func updateMemLimitForContainer(memProtectionConfigs map[string]uint64, relCgroupPath, limitType string) {
	var targetRatio int64 = 0
	for cgPath, ratio := range memProtectionConfigs {
		if strings.HasPrefix(relCgroupPath, "/"+cgPath) {
			targetRatio = int64(ratio)
			break
		}
	}
	if targetRatio == 0 {
		return
	}
	updateMemLimitIfNeeded(relCgroupPath, targetRatio, limitType)
}

func updateMemLimitIfNeeded(relCgroupPath string, ratio int64, limitType string) error {
	// get memory.limit
	memStat, err := cgroupmgr.GetMemoryWithRelativePath(relCgroupPath)
	if err != nil {
		return err
	}

	memLimit := memStat.Limit
	newLimit := int64(float64(memLimit) * float64(ratio) / 100)
	newLimit = int64(general.Clamp(float64(newLimit), cgMemProtectionMin, cgMemProtectionMax))

	var data *cgroupcm.MemoryData

	switch limitType {
	case memProtectionLow:
		data = &cgroupcm.MemoryData{SoftLimitInBytes: newLimit}
	case memProtectionMin:
		data = &cgroupcm.MemoryData{MinInBytes: newLimit}
	default:
		return fmt.Errorf("unknown limit type: %s", limitType)
	}

	if err := cgroupmgr.ApplyMemoryWithRelativePath(relCgroupPath, data); err != nil {
		return err
	}

	general.Infof("Apply Mem%sLimitInBytes: %v, new value=%d", strings.Title(limitType), relCgroupPath, newLimit)
	return nil
}
