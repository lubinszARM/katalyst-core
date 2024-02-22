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

package io

import (
	"context"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/kubewharf/katalyst-core/pkg/agent/sysadvisor/metacache"
	ioadvisorplugin "github.com/kubewharf/katalyst-core/pkg/agent/sysadvisor/plugin/qosaware/resource/io/plugin"
	"github.com/kubewharf/katalyst-core/pkg/agent/sysadvisor/types"
	"github.com/kubewharf/katalyst-core/pkg/config"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
)

func init() {
}

// ioResourceAdvisor updates io resource
type ioResourceAdvisor struct {
	conf      *config.Configuration
	startTime time.Time
	plugins   []ioadvisorplugin.IOAdvisorPlugin
	mutex     sync.RWMutex

	metaReader metacache.MetaReader
	metaServer *metaserver.MetaServer
	emitter    metrics.MetricEmitter

	recvCh   chan types.TriggerInfo
	sendChan chan types.InternalIOCalculationResult
}

// NewIOResourceAdvisor returns a ioResourceAdvisor instance
func NewIOResourceAdvisor(conf *config.Configuration, extraConf interface{}, metaCache metacache.MetaCache,
	metaServer *metaserver.MetaServer, emitter metrics.MetricEmitter) *ioResourceAdvisor {
	ra := &ioResourceAdvisor{
		startTime: time.Now(),

		conf:       conf,
		metaReader: metaCache,
		metaServer: metaServer,
		emitter:    emitter,
		recvCh:     make(chan types.TriggerInfo, 1),
		sendChan:   make(chan types.InternalIOCalculationResult, 1),
	}

	ioAdvisorPluginInitializers := ioadvisorplugin.GetRegisteredInitializers()
	for _, ioadvisorPluginName := range conf.IOAdvisorPlugins {
		initFunc, ok := ioAdvisorPluginInitializers[ioadvisorPluginName]
		if !ok {
			klog.Errorf("failed to find registered initializer %v", ioadvisorPluginName)
			continue
		}
		general.InfoS("add new io advisor policy", "policyName", ioadvisorPluginName)
		ra.plugins = append(ra.plugins, initFunc(conf, extraConf, metaCache, metaServer, emitter))
	}

	return ra
}

func (ra *ioResourceAdvisor) Run(ctx context.Context) {
	period := ra.conf.SysAdvisorPluginsConfiguration.QoSAwarePluginConfiguration.SyncPeriod

	general.InfoS("wait to list containers")
	<-ra.recvCh
	general.InfoS("list containers successfully")

	go wait.Until(ra.update, period, ctx.Done())
}

func (ra *ioResourceAdvisor) GetChannels() (interface{}, interface{}) {
	return ra.recvCh, ra.sendChan
}

func (ra *ioResourceAdvisor) GetHeadroom() (resource.Quantity, error) {
	ra.mutex.RLock()
	defer ra.mutex.RUnlock()

	return resource.Quantity{}, nil
}

func (ra *ioResourceAdvisor) sendAdvices() {
	// send to server
	result := types.InternalIOCalculationResult{TimeStamp: time.Now()}
	for _, plugin := range ra.plugins {
		advices := plugin.GetAdvices()
		result.ContainerEntries = append(result.ContainerEntries, advices.ContainerEntries...)
	}

	select {
	case ra.sendChan <- result:
		general.Infof("notify io server: %+v", result)
	default:
		general.Errorf("channel is full")
	}
}

func (ra *ioResourceAdvisor) update() {
	ra.mutex.Lock()
	defer ra.mutex.Unlock()

	if !ra.metaReader.HasSynced() {
		general.InfoS("metaReader has not synced, skip updating")
		return
	}

	ra.sendAdvices()
}
