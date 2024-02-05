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
	"sync"

	"github.com/kubewharf/katalyst-core/pkg/config"
	coreconfig "github.com/kubewharf/katalyst-core/pkg/config"
	dynamicconfig "github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
)

const (
	EnableCGMemProtectionPeriodicalHandlerName = "MemProtection"
)

var (
	initializeOnce sync.Once
)

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

	applyMemProtectionContainerLevelConfig(conf, metaServer, 0)
}

func applyMemProtectionContainerLevelConfig(conf *config.Configuration, metaServer *metaserver.MetaServer, disable int) {
	if !conf.EnableCgMemProtection {
		general.Infof("EnableSetCgMemProtection disabled, skip applyMemProtectionContainerLevelConfig")
		return
	} else if conf.CgMemProtectionContainerLevelConfigFile == "" {
		general.Errorf("CgMemProtectionContainerLevelConfigFile isn't configured")
		return
	}
	/*
	   memLowPSMConfigs := make(map[string]uint64)
	   err := utilgeneral.LoadJsonConfig(conf.MemLowPSMLevelConfigFile, &memLowPSMConfigs)

	   	if err != nil {
	   		general.Errorf("load MemLowPSMLevelConfig failed with error: %v", err)
	   		return
	   	}

	   ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	   defer cancel()
	   pods, err := metaServer.GetPodList(ctx, nil)

	   	if err != nil {
	   		general.Errorf("GetPodList failed with error: %v", err)
	   		return
	   	}

	   	for _, pod := range pods {
	   		if pod == nil {
	   			continue
	   		}

	   		psm, err := bytedance.GetObjectLabelPSM(pod)
	   		if err != nil {
	   			general.Errorf("GetObjectLabelPSM for pod: %s/%s failed with error: %v",
	   				pod.Namespace, pod.Name, err)
	   			continue
	   		}

	   		if _, found := memLowPSMConfigs[psm]; !found {
	   			continue
	   		}

	   		_ = cgroupmgr.ApplyMemoryWithRelativePath("pod"+string(pod.UID), &cgroupcm.MemoryData{
	   			SoftLimitInBytes: memLowPSMConfigs[psm],
	   		})
	   	}
	*/
}
