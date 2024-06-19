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
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"

	"github.com/kubewharf/katalyst-core/pkg/config"
	evictionconfig "github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic/adminqos/eviction"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	"github.com/kubewharf/katalyst-core/pkg/metaserver/agent/metric/helper"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
	"github.com/kubewharf/katalyst-core/pkg/util/native"
)

// eviction scope related variables
// actionReclaimedEviction for reclaimed_cores, while actionEviction for all pods
const (
	actionNoop = iota
	actionReclaimedEviction
	actionEviction

	healthCheckTimeout = 1 * time.Minute
)

// control-related variables
const (
	kswapdStealPreviousCycleMissing = -1
	nonExistNumaID                  = -1
)

const (
	metricsNameFetchMetricError   = "fetch_metric_error_count"
	metricsNameNumberOfTargetPods = "number_of_target_pods_raw"
	metricsNameThresholdMet       = "threshold_met_count"
	metricsNameNumaMetric         = "numa_metric_raw"
	metricsNameSystemMetric       = "system_metric_raw"

	metricsTagKeyEvictionScope  = "eviction_scope"
	metricsTagKeyDetectionLevel = "detection_level"
	metricsTagKeyNumaID         = "numa_id"
	metricsTagKeyAction         = "action"
	metricsTagKeyMetricName     = "metric_name"

	metricsTagValueDetectionLevelNuma             = "numa_debug"
	metricsTagValueDetectionLevelSystem           = "system_debug"
	metricsTagValueActionReclaimedEviction        = "reclaimed_eviction"
	metricsTagValueActionEviction                 = "eviction"
	metricsTagValueNumaFreeBelowWatermarkTimes    = "numa_free_below_watermark_times"
	metricsTagValueSystemKswapdDiff               = "system_kswapd_diff"
	metricsTagValueSystemKswapdRateExceedDuration = "system_kswapd_rate_exceed_duration"
)

const (
	errMsgCheckReclaimedPodFailed = "failed to check reclaimed pod, pod: %s/%s, err: %v"
)

// EvictionHelper is a general tool collection for all memory eviction plugin
type EvictionHelper struct {
	metaServer         *metaserver.MetaServer
	emitter            metrics.MetricEmitter
	reclaimedPodFilter func(pod *v1.Pod) (bool, error)
}

func NewEvictionHelper(emitter metrics.MetricEmitter, metaServer *metaserver.MetaServer, conf *config.Configuration) *EvictionHelper {
	return &EvictionHelper{
		metaServer:         metaServer,
		emitter:            emitter,
		reclaimedPodFilter: conf.CheckReclaimedQoSForPod,
	}
}

func (e *EvictionHelper) selectTopNPodsToEvictByMetrics(activePods []*v1.Pod, topN uint64, numaID,
	action int, rankingMetrics []string, podToEvictMap map[string]*v1.Pod,
) {
	filteredPods := e.filterPods(activePods, action)
	if filteredPods != nil {
		general.NewMultiSorter(e.getEvictionCmpFuncs(rankingMetrics, numaID)...).Sort(native.NewPodSourceImpList(filteredPods))
		for i := 0; uint64(i) < general.MinUInt64(topN, uint64(len(filteredPods))); i++ {
			podToEvictMap[string(filteredPods[i].UID)] = filteredPods[i]
		}
	}
}

func (e *EvictionHelper) filterPods(pods []*v1.Pod, action int) []*v1.Pod {
	switch action {
	case actionReclaimedEviction:
		return native.FilterPods(pods, e.reclaimedPodFilter)
	case actionEviction:
		return pods
	default:
		return nil
	}
}

// getEvictionCmpFuncs returns a comparison function list to judge the eviction order of different pods
func (e *EvictionHelper) getEvictionCmpFuncs(rankingMetrics []string, numaID int) []general.CmpFunc {
	cmpFuncs := make([]general.CmpFunc, 0, len(rankingMetrics))

	for _, m := range rankingMetrics {
		currentMetric := m
		cmpFuncs = append(cmpFuncs, func(s1, s2 interface{}) int {
			p1, p2 := s1.(*v1.Pod), s2.(*v1.Pod)
			switch currentMetric {
			case evictionconfig.FakeMetricQoSLevel:
				isReclaimedPod1, err1 := e.reclaimedPodFilter(p1)
				if err1 != nil {
					general.Errorf(errMsgCheckReclaimedPodFailed, p1.Namespace, p1.Name, err1)
				}

				isReclaimedPod2, err2 := e.reclaimedPodFilter(p2)
				if err2 != nil {
					general.Errorf(errMsgCheckReclaimedPodFailed, p2.Namespace, p2.Name, err2)
				}

				if err1 != nil || err2 != nil {
					// prioritize evicting the pod for which no error is returned
					return general.CmpError(err1, err2)
				}

				// prioritize evicting the pod whose QoS level is reclaimed_cores
				return general.CmpBool(isReclaimedPod1, isReclaimedPod2)
			case evictionconfig.FakeMetricPriority:
				// prioritize evicting the pod whose priority is lower
				return general.ReverseCmpFunc(native.PodPriorityCmpFunc)(p1, p2)
			default:
				p1Metric, p1Err := helper.GetPodMetric(e.metaServer.MetricsFetcher, e.emitter, p1, currentMetric, numaID)
				p2Metric, p2Err := helper.GetPodMetric(e.metaServer.MetricsFetcher, e.emitter, p2, currentMetric, numaID)
				p1Found := p1Err == nil
				p2Found := p2Err == nil
				if !p1Found || !p2Found {
					_ = e.emitter.StoreInt64(metricsNameFetchMetricError, 1, metrics.MetricTypeNameCount,
						metrics.ConvertMapToTags(map[string]string{
							metricsTagKeyNumaID: strconv.Itoa(numaID),
						})...)
					// prioritize evicting the pod for which no stats were found
					return general.CmpBool(!p1Found, !p2Found)
				}

				// prioritize evicting the pod whose metric value is greater
				return general.CmpFloat64(p1Metric, p2Metric)
			}
		})
	}

	return cmpFuncs
}

type PressureType int

const (
	SOME PressureType = iota
	FULL
)

func pressureTypeToString(t PressureType) string {
	switch t {
	case SOME:
		return "some"
	case FULL:
		return "full"
	default:
		panic("unreachable")
	}
}

type PsiFormat int

const (
	UPSTREAM PsiFormat = iota
	EXPERIMENTAL
	MISSING
	INVALID
)

type ResourcePressure struct {
	Avg10  float64
	Avg60  float64
	Avg300 float64
	Total  *time.Duration
}

func getPsiFormat(lines []string) PsiFormat {
	// Dummy implementation for getPsiFormat
	// You should replace this with your actual implementation
	if strings.Contains(lines[0], "avg10") {
		return UPSTREAM
	} else if strings.Contains(lines[0], "aggr") {
		return EXPERIMENTAL
	} else if len(lines) == 0 {
		return MISSING
	}
	return INVALID
}

func split(s, sep string) []string {
	return strings.Split(s, sep)
}

func readRespressureFromFile(fileName string, t PressureType) (ResourcePressure, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return ResourcePressure{}, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return ResourcePressure{}, err
	}

	return readRespressureFromLines(lines, t)
}

func readRespressureFromLines(lines []string, t PressureType) (ResourcePressure, error) {
	typeName := pressureTypeToString(t)
	var pressureLineIndex int
	switch t {
	case SOME:
		pressureLineIndex = 0
	case FULL:
		pressureLineIndex = 1
	}

	switch getPsiFormat(lines) {
	case UPSTREAM:
		toks := split(lines[pressureLineIndex], " ")
		if toks[0] != typeName {
			return ResourcePressure{}, errors.New("invalid type name")
		}
		avg10 := split(toks[1], "=")
		if avg10[0] != "avg10" {
			return ResourcePressure{}, errors.New("invalid avg10 format")
		}
		avg60 := split(toks[2], "=")
		if avg60[0] != "avg60" {
			return ResourcePressure{}, errors.New("invalid avg60 format")
		}
		avg300 := split(toks[3], "=")
		if avg300[0] != "avg300" {
			return ResourcePressure{}, errors.New("invalid avg300 format")
		}
		total := split(toks[4], "=")
		if total[0] != "total" {
			return ResourcePressure{}, errors.New("invalid total format")
		}

		totalMicroseconds, err := strconv.ParseUint(total[1], 10, 64)
		if err != nil {
			return ResourcePressure{}, err
		}
		totalDuration := time.Duration(totalMicroseconds) * time.Microsecond

		return ResourcePressure{
			Avg10:  atof(avg10[1]),
			Avg60:  atof(avg60[1]),
			Avg300: atof(avg300[1]),
			Total:  &totalDuration,
		}, nil

	case EXPERIMENTAL:
		toks := split(lines[pressureLineIndex+1], " ")
		if toks[0] != typeName {
			return ResourcePressure{}, errors.New("invalid type name")
		}

		return ResourcePressure{
			Avg10:  atof(toks[1]),
			Avg60:  atof(toks[2]),
			Avg300: atof(toks[3]),
			Total:  nil,
		}, nil

	case MISSING:
		return ResourcePressure{}, errors.New("missing control file")
	case INVALID:
		return ResourcePressure{}, errors.New("invalid format")
	}
	return ResourcePressure{}, errors.New("unreachable")
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func readPgscanFromFile(fileName string) (uint64, error) {
	// Open the file
	file, err := os.Open(fileName)
	if err != nil {
		return 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Initialize the variable to store pgscan value
	var pgscanValue uint64

	// Read the file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Split the line into key and value
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		value := parts[1]

		// Check if the key is pgscan
		if key == "pgscan" {
			// Convert the value to an integer
			pgscanValue, err = strconv.ParseUint(value, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("failed to parse pgscan value: %w", err)
			}
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("error reading file: %w", err)
	}

	return pgscanValue, nil
}
