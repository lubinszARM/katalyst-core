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

package memory

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/events"

	pluginapi "github.com/kubewharf/katalyst-api/pkg/protocol/evictionplugin/v1alpha1"
	"github.com/kubewharf/katalyst-core/pkg/agent/evictionmanager/plugin"
	"github.com/kubewharf/katalyst-core/pkg/client"
	"github.com/kubewharf/katalyst-core/pkg/config"
	"github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic"
	"github.com/kubewharf/katalyst-core/pkg/consts"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	"github.com/kubewharf/katalyst-core/pkg/metaserver/agent/metric/helper"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	"github.com/kubewharf/katalyst-core/pkg/util/cgroup/common"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
	"github.com/kubewharf/katalyst-core/pkg/util/native"
	"github.com/kubewharf/katalyst-core/pkg/util/process"
)

const (
	EvictionPluginNameSystemMemoryPressure = "system-memory-pressure-eviction-plugin"
	EvictionScopeSystemMemory              = "SystemMemory"
	evictionConditionMemoryPressure        = "MemoryPressure"
	syncTolerationTurns                    = 1
)

func NewSystemPressureEvictionPlugin(_ *client.GenericClientSet, _ events.EventRecorder,
	metaServer *metaserver.MetaServer, emitter metrics.MetricEmitter, conf *config.Configuration,
) plugin.EvictionPlugin {
	p := &SystemPressureEvictionPlugin{
		pluginName:                EvictionPluginNameSystemMemoryPressure,
		emitter:                   emitter,
		StopControl:               process.NewStopControl(time.Time{}),
		metaServer:                metaServer,
		evictionManagerSyncPeriod: conf.EvictionManagerSyncPeriod,
		coolDownPeriod:            conf.SystemPressureCoolDownPeriod,
		syncPeriod:                time.Duration(conf.SystemPressureSyncPeriod) * time.Second,
		dynamicConfig:             conf.DynamicAgentConfiguration,
		reclaimedPodFilter:        conf.CheckReclaimedQoSForPod,
		evictionHelper:            NewEvictionHelper(emitter, metaServer, conf),
	}
	return p
}

// SystemPressureEvictionPlugin implements the EvictPlugin interface.
// It triggers pod eviction based on the system pressure of memory.
type SystemPressureEvictionPlugin struct {
	*process.StopControl
	sync.Mutex

	emitter                   metrics.MetricEmitter
	reclaimedPodFilter        func(pod *v1.Pod) (bool, error)
	evictionManagerSyncPeriod time.Duration
	pluginName                string
	metaServer                *metaserver.MetaServer
	evictionHelper            *EvictionHelper

	syncPeriod     time.Duration
	coolDownPeriod int
	dynamicConfig  *dynamic.DynamicAgentConfiguration

	systemAction                   int
	isUnderSystemPressure          bool
	kswapdStealPreviousCycle       float64
	kswapdStealPreviousCycleTime   time.Time
	kswapdStealRateExceedStartTime *time.Time

	onlineMemHighPressureHitThresAt time.Time

	onlineMemPgscanLast  uint64
	onlineMemPgscanTimes uint64

	lastEvictionTime time.Time
}

func (s *SystemPressureEvictionPlugin) Name() string {
	if s == nil {
		return ""
	}

	return s.pluginName
}

func (s *SystemPressureEvictionPlugin) Start() {
	general.RegisterHeartbeatCheck(EvictionPluginNameSystemMemoryPressure, syncTolerationTurns*s.syncPeriod,
		general.HealthzCheckStateNotReady, syncTolerationTurns*s.syncPeriod)
	go wait.UntilWithContext(context.TODO(), s.detectSystemPressures, s.syncPeriod)
}

func (s *SystemPressureEvictionPlugin) ThresholdMet(_ context.Context) (*pluginapi.ThresholdMetResponse, error) {
	resp := &pluginapi.ThresholdMetResponse{
		MetType: pluginapi.ThresholdMetType_NOT_MET,
	}

	dynamicConfig := s.dynamicConfig.GetDynamicConfiguration()
	if !dynamicConfig.EnableSystemLevelEviction {
		return resp, nil
	}

	// TODO maybe we should set timeout for this lock operation in case it blocks the entire sync loop
	s.Lock()
	defer s.Unlock()

	if s.isUnderSystemPressure {
		resp = &pluginapi.ThresholdMetResponse{
			MetType:       pluginapi.ThresholdMetType_HARD_MET,
			EvictionScope: EvictionScopeSystemMemory,
			Condition: &pluginapi.Condition{
				ConditionType: pluginapi.ConditionType_NODE_CONDITION,
				Effects:       []string{string(v1.TaintEffectNoSchedule)},
				ConditionName: evictionConditionMemoryPressure,
				MetCondition:  true,
			},
		}
	}

	general.Infof("ThresholdMet result, m.isUnderSystemPressure: %+v, m.systemAction: %+v", s.isUnderSystemPressure, s.systemAction)

	return resp, nil
}

func (s *SystemPressureEvictionPlugin) detectSystemPressures(_ context.Context) {
	s.Lock()
	defer s.Unlock()
	var err error
	defer func() {
		_ = general.UpdateHealthzStateByError(EvictionPluginNameSystemMemoryPressure, err)
	}()

	s.isUnderSystemPressure = false
	s.systemAction = actionNoop

	if common.CheckCgroup2UnifiedMode() {
		err = s.detectOnlineMemPSI()
		//err = s.detectOnlineMemReclaim()
	} else {
		err = s.detectSystemWatermarkPressure()
		err = s.detectSystemKswapdStealPressure()
	}
	switch s.systemAction {
	case actionReclaimedEviction:
		_ = s.emitter.StoreInt64(metricsNameThresholdMet, 1, metrics.MetricTypeNameCount,
			metrics.ConvertMapToTags(map[string]string{
				metricsTagKeyEvictionScope:  EvictionScopeSystemMemory,
				metricsTagKeyDetectionLevel: metricsTagValueDetectionLevelSystem,
				metricsTagKeyAction:         metricsTagValueActionReclaimedEviction,
			})...)
	case actionEviction:
		_ = s.emitter.StoreInt64(metricsNameThresholdMet, 1, metrics.MetricTypeNameCount,
			metrics.ConvertMapToTags(map[string]string{
				metricsTagKeyEvictionScope:  EvictionScopeSystemMemory,
				metricsTagKeyDetectionLevel: metricsTagValueDetectionLevelSystem,
				metricsTagKeyAction:         metricsTagValueActionEviction,
			})...)
	}
}

func (s *SystemPressureEvictionPlugin) detectOnlineMemPSI() error {
	fileName := "/sys/fs/cgroup/kubepods/burstable/memory.pressure"
	pressure, err := readRespressureFromFile(fileName, FULL)
	if err != nil {
		general.Infof("detectOnlineMemPSI rror:", err)
		s.onlineMemHighPressureHitThresAt = time.Time{}
		return err
	}

	now := time.Now()

	if pressure.Avg10 >= 25 {
		if s.onlineMemHighPressureHitThresAt.IsZero() {
			s.onlineMemHighPressureHitThresAt = now
		}

		diff := now.Sub(s.onlineMemHighPressureHitThresAt).Seconds()
		if int64(diff) >= 30 {
			s.isUnderSystemPressure = true
			s.systemAction = actionReclaimedEviction
			general.Infof("BBLU1 online mem.psi thresholdMet: psi=%+v, duration=%+v", pressure.Avg10, int64(diff))
		}
		/*
			if int64(diff) >= 100 {
				s.isUnderSystemPressure = true
				s.systemAction = actionEviction
			}
		*/
	} else {
		if pressure.Avg10 > 10 {
			general.Infof("BBLU2 online mem.psi: psi=%+v", pressure.Avg10)
		}
		s.onlineMemHighPressureHitThresAt = time.Time{}
	}
	return nil
}

func (s *SystemPressureEvictionPlugin) detectOnlineMemReclaim() error {
	fileName := "/sys/fs/cgroup/kubepods/burstable/memory.stat"
	pgscan, err := readPgscanFromFile(fileName)
	if err != nil {
		general.Infof("readPgscanFromFile rror:", err)
		s.onlineMemPgscanTimes = 0
		return err
	}

	if pgscan > s.onlineMemPgscanLast {
		s.onlineMemPgscanTimes++
		s.onlineMemPgscanLast = pgscan
	} else {
		s.onlineMemPgscanTimes = 0
	}

	if s.onlineMemPgscanTimes > 2 {
		general.Infof("BBLU online mem.pgscan duration:%v", s.onlineMemPgscanTimes)
	}

	return nil
}

func (s *SystemPressureEvictionPlugin) detectSystemWatermarkPressure() error {
	free, total, scaleFactor, err := helper.GetWatermarkMetrics(s.metaServer.MetricsFetcher, s.emitter, nonExistNumaID)
	if err != nil {
		_ = s.emitter.StoreInt64(metricsNameFetchMetricError, 1, metrics.MetricTypeNameCount,
			metrics.ConvertMapToTags(map[string]string{
				metricsTagKeyNumaID: strconv.Itoa(nonExistNumaID),
			})...)
		general.Errorf("failed to getWatermarkMetrics for system, err: %v", err)
		return err
	}

	thresholdMinimum := float64(s.dynamicConfig.GetDynamicConfiguration().SystemFreeMemoryThresholdMinimum)
	threshold := math.Max(thresholdMinimum, total*scaleFactor/10000)

	general.Infof("system watermark metrics, "+
		"free: %+v, total: %+v, scaleFactor: %+v, configuration minimum: %+v, final threshold: %+v",
		free, total, scaleFactor, thresholdMinimum, threshold)

	if free < threshold {
		s.isUnderSystemPressure = true
		s.systemAction = actionReclaimedEviction
	}
	return nil
}

func (s *SystemPressureEvictionPlugin) detectSystemKswapdStealPressure() error {
	kswapdSteal, err := helper.GetNodeMetricWithTime(s.metaServer.MetricsFetcher, s.emitter, consts.MetricMemKswapdstealSystem)
	if err != nil {
		s.kswapdStealPreviousCycle = kswapdStealPreviousCycleMissing
		s.kswapdStealPreviousCycleTime = time.Now()
		_ = s.emitter.StoreInt64(metricsNameFetchMetricError, 1, metrics.MetricTypeNameCount,
			metrics.ConvertMapToTags(map[string]string{
				metricsTagKeyNumaID: strconv.Itoa(nonExistNumaID),
			})...)
		general.Errorf("failed to getSystemKswapdStealMetrics, err: %v", err)
		return err
	}

	if kswapdSteal.Time.Equal(s.kswapdStealPreviousCycleTime) {
		general.Warningf("getSystemKswapdStealMetrics get same result as last round,skip current round")
		return nil
	}

	dynamicConfig := s.dynamicConfig.GetDynamicConfiguration()
	general.Infof("system kswapd metrics, "+
		"kswapdSteal: %+v, kswapdStealPreviousCycle: %+v, kswapdStealPreviousCycleTime: %+v, systemKswapdRateThreshold: %+v, evictionManagerSyncPeriod: %+v, "+
		"kswapdStealRateExceedStartTime: %+v, SystemKswapdRateExceedDurationThreshold: %+v",
		kswapdSteal, s.kswapdStealPreviousCycle, s.kswapdStealPreviousCycleTime, dynamicConfig.SystemKswapdRateThreshold,
		s.evictionManagerSyncPeriod.Seconds(), s.kswapdStealRateExceedStartTime,
		dynamicConfig.SystemKswapdRateExceedDurationThreshold)
	if s.kswapdStealRateExceedStartTime != nil && !s.kswapdStealRateExceedStartTime.IsZero() {
		duration := kswapdSteal.Time.Sub(*s.kswapdStealRateExceedStartTime)
		_ = s.emitter.StoreFloat64(metricsNameSystemMetric, duration.Seconds(), metrics.MetricTypeNameRaw,
			metrics.ConvertMapToTags(map[string]string{
				metricsTagKeyMetricName: metricsTagValueSystemKswapdRateExceedDuration,
			})...)
	}
	_ = s.emitter.StoreFloat64(metricsNameSystemMetric, kswapdSteal.Value-s.kswapdStealPreviousCycle, metrics.MetricTypeNameRaw,
		metrics.ConvertMapToTags(map[string]string{
			metricsTagKeyMetricName: metricsTagValueSystemKswapdDiff,
		})...)

	kswapdStealPreviousCycle := s.kswapdStealPreviousCycle
	kswapdStealPreviousCycleTime := s.kswapdStealPreviousCycleTime
	s.kswapdStealPreviousCycle = kswapdSteal.Value
	s.kswapdStealPreviousCycleTime = *(kswapdSteal.Time)
	if kswapdStealPreviousCycle == kswapdStealPreviousCycleMissing {
		general.Warningf("kswapd steal of the previous cycle is missing")
		return nil
	}

	if (kswapdSteal.Value-kswapdStealPreviousCycle)/(kswapdSteal.Time.Sub(kswapdStealPreviousCycleTime)).Seconds() >= float64(dynamicConfig.SystemKswapdRateThreshold) {
		// the pressure continues,if there is no recorded start time,we record the previous cycle time as the pressure start time
		if s.kswapdStealRateExceedStartTime == nil || s.kswapdStealRateExceedStartTime.IsZero() {
			s.kswapdStealRateExceedStartTime = &kswapdStealPreviousCycleTime
		}
	} else {
		// there is no pressure anymore, clear the start time
		s.kswapdStealRateExceedStartTime = nil
	}

	if s.kswapdStealRateExceedStartTime != nil && !s.kswapdStealRateExceedStartTime.IsZero() {
		pressureDuration := kswapdSteal.Time.Sub(*(s.kswapdStealRateExceedStartTime)).Seconds()
		if int(pressureDuration) >= dynamicConfig.SystemKswapdRateExceedDurationThreshold {
			s.isUnderSystemPressure = true
			s.systemAction = actionEviction
		}
	}
	return nil
}

func (s *SystemPressureEvictionPlugin) GetTopEvictionPods(_ context.Context, request *pluginapi.GetTopEvictionPodsRequest) (*pluginapi.GetTopEvictionPodsResponse, error) {
	if request == nil {
		return nil, fmt.Errorf("GetTopEvictionPods got nil request")
	}

	if len(request.ActivePods) == 0 {
		general.Warningf("GetTopEvictionPods got empty active pods list")
		return &pluginapi.GetTopEvictionPodsResponse{}, nil
	}

	now := time.Now()
	if !s.lastEvictionTime.IsZero() && now.Sub(s.lastEvictionTime) < time.Duration(s.coolDownPeriod)*time.Second {
		general.Infof("in eviction cool-down time, skip eviction. now: %s, lastEvictionTime: %s",
			now.String(), s.lastEvictionTime.String())
		return &pluginapi.GetTopEvictionPodsResponse{}, nil
	}
	s.lastEvictionTime = now

	dynamicConfig := s.dynamicConfig.GetDynamicConfiguration()
	targetPods := make([]*v1.Pod, 0, len(request.ActivePods))
	podToEvictMap := make(map[string]*v1.Pod)

	general.Infof("GetTopEvictionPods condition, m.isUnderSystemPressure: %+v, "+
		"m.systemAction: %+v", s.isUnderSystemPressure, s.systemAction)

	if dynamicConfig.EnableSystemLevelEviction && s.isUnderSystemPressure {
		s.evictionHelper.selectTopNPodsToEvictByMetrics(request.ActivePods, request.TopN, nonExistNumaID, s.systemAction,
			dynamicConfig.SystemEvictionRankingMetrics, podToEvictMap)
	}

	for uid := range podToEvictMap {
		targetPods = append(targetPods, podToEvictMap[uid])
	}

	_ = s.emitter.StoreInt64(metricsNameNumberOfTargetPods, int64(len(targetPods)), metrics.MetricTypeNameRaw)
	general.Infof("GetTopEvictionPods result, targetPods: %+v", native.GetNamespacedNameListFromSlice(targetPods))

	resp := &pluginapi.GetTopEvictionPodsResponse{
		TargetPods: targetPods,
	}
	if gracePeriod := dynamicConfig.MemoryPressureEvictionConfiguration.GracePeriod; gracePeriod > 0 {
		resp.DeletionOptions = &pluginapi.DeletionOptions{
			GracePeriodSeconds: gracePeriod,
		}
	}

	return resp, nil
}

func (s *SystemPressureEvictionPlugin) GetEvictPods(_ context.Context, request *pluginapi.GetEvictPodsRequest) (*pluginapi.GetEvictPodsResponse, error) {
	if request == nil {
		return nil, fmt.Errorf("GetEvictPods got nil request")
	}

	return &pluginapi.GetEvictPodsResponse{}, nil
}
