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

package staticpolicy

import (
	"context"

	"github.com/kubewharf/katalyst-core/pkg/agent/qrm-plugins/io/handlers/ioweight"
	coreconfig "github.com/kubewharf/katalyst-core/pkg/config"
	dynamicconfig "github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
	"github.com/kubewharf/katalyst-core/pkg/util/native"
)

func (p *StaticPolicy) syncIOWeight(conf *coreconfig.Configuration,
	_ interface{}, _ *dynamicconfig.DynamicAgentConfiguration,
	emitter metrics.MetricEmitter, metaServer *metaserver.MetaServer) {
	general.Infof("called")

	if metaServer == nil {
		general.Errorf("nil metaServer")
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
		ioweight.GetQoS(conf.QoSConfiguration, pod)
	}

}
