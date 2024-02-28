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

package ioweight

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"

	"github.com/kubewharf/katalyst-core/pkg/agent/qrm-plugins/commonstate"
	coreconfig "github.com/kubewharf/katalyst-core/pkg/config"
	dynamicconfig "github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic"
	"github.com/kubewharf/katalyst-core/pkg/config/generic"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
	"github.com/kubewharf/katalyst-core/pkg/util/native"
)

func GetQoS(qosConfig *generic.QoSConfiguration, pod *v1.Pod) {
	qosLevel, err := qosConfig.GetQoSLevelForPod(pod)
	if err != nil {
		fmt.Printf("BBLU111 failed:%v..\n", err)
		return
	}
	fmt.Printf("BBLU112 test:%v:%v..\n", qosLevel, string(pod.UID))
	return
}

func IOWeightTaskFunc(conf *coreconfig.Configuration,
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

	// SettingIOWeight featuregate.
	if !conf.EnableSettingIOWeight {
		general.Infof("EnableSettingIOWeight disabled")
		return
	}
	/*
		ctx := context.Background()
		podList, err := metaServer.GetPodList(ctx, native.PodIsActive)
		if err != nil {
			general.Infof("get pod list failed: %v", err)
			return
		}

		for _, pod := range podList {
			if pod == nil {
				general.Errorf("get nil pod from metaServer")
				continue
			}
			GetQoS(conf.QoSConfiguration, pod)
		}
	*/
	// checking qos-level io.weight configuration.
	if len(conf.IOWeightQoSLevelConfigFile) > 0 {
		/*
		   extraControlKnobConfigs := make(commonstate.ExtraControlKnobConfigs)
		   extraControlKnobConfigs, err := commonstate.LoadExtraControlKnobConfigs(conf.IOWeightQoSLevelConfigFile)

		   	if err != nil {
		   		return
		   	}
		*/
		var extraControlKnobConfigs commonstate.ExtraControlKnobConfigs
		if err := general.LoadJsonConfig(conf.IOWeightQoSLevelConfigFile, &extraControlKnobConfigs); err != nil {
			general.Errorf("CgMemProtectionPSMLevelConfigFile load failed:%v", err)
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
				general.Errorf("get nil pod from metaServer")
				continue
			}
			qosConfig := conf.QoSConfiguration
			qosLevel, err := qosConfig.GetQoSLevelForPod(pod)
			if err != nil {
				fmt.Printf("BBLU111 failed:%v..\n", err)
				return
			}
			fmt.Printf("BBLU112 test:%v:%v..\n", qosLevel, string(pod.UID))
			/*
				qosLevelDefaultValue, ok := extraControlKnobConfigs.QoSLevelToDefaultValue[qosLevel]
				if ok {
					fmt.Printf("BBLU333:%v..\n", qosLevelDefaultValue)
				}
			*/
			for key, value := range extraControlKnobConfigs["io_weight"].QoSLevelToDefaultValue {
				fmt.Printf("QoS Level: %s, Default Value: %s\n", key, value)
			}

			qosLevelDefaultValue, ok := extraControlKnobConfigs["io_weight"].QoSLevelToDefaultValue[qosLevel]
			if ok {
				fmt.Printf("BBLU333:%v..\n", qosLevelDefaultValue)
			}
		}
	}

	// checking cgroup-level io.weight configuration.
}
