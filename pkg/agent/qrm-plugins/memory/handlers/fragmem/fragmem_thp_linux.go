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

package fragmem

import (
	"fmt"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/util/errors"

	memconsts "github.com/kubewharf/katalyst-core/pkg/agent/qrm-plugins/memory/consts"
	coreconfig "github.com/kubewharf/katalyst-core/pkg/config"
	dynamicconfig "github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	malachiteclient "github.com/kubewharf/katalyst-core/pkg/metaserver/agent/metric/provisioner/malachite/client"
	malachitetypes "github.com/kubewharf/katalyst-core/pkg/metaserver/agent/metric/provisioner/malachite/types"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
	procfsm "github.com/kubewharf/katalyst-core/pkg/util/procfs/manager"
)

const (
	thpModeMadvise = "madvise"
	thpModeAlways  = "always"
	thpModeNever   = "never"

	// High-order range is fixed to 9~10 for now.
	highOrderMin = 9
	highOrderMax = 10

	defaultHighOrderThreshold = 85.0

	// hysteresisRatio adds hysteresis between disable/enable thresholds to avoid frequent toggling.
	hysteresisRatio = 0.9
)

// thpEnabledPath is the sysfs path we write to when tuning THP.
// It is a var (not const) so tests can override it with a temp file.
var thpEnabledPath = procfsm.TransparentHugepageEnabledPath

// newMalachiteClient is a seam for unit tests.
// DO NOT mutate it in production code.
var newMalachiteClient = malachiteclient.NewMalachiteClient

type thpDecision int

const (
	thpDecisionNone thpDecision = iota
	thpDecisionDisable
	thpDecisionEnable
)

// SetMemTHP periodically tunes host THP based on high-order extfrag scores.
func SetMemTHP(conf *coreconfig.Configuration,
	_ interface{}, _ *dynamicconfig.DynamicAgentConfiguration,
	emitter metrics.MetricEmitter, metaServer *metaserver.MetaServer,
) {
	var errList []error
	defer func() {
		_ = general.UpdateHealthzStateByError(memconsts.SetMemTHP, k8serrors.NewAggregate(errList))
	}()

	if conf == nil {
		err := fmt.Errorf("SetMemTHP: nil configuration")
		errList = append(errList, err)
		general.Errorf("%v", err)
		return
	}
	if !conf.EnableSettingFragMem {
		general.Infof("SetMemTHP skipped: EnableSettingFragMem disabled")
		return
	}

	mode := normalizeTHPMode(conf.THPDefaultConfig)
	// If THPDefaultConfig is empty, skip THP tuning entirely.
	if mode == "" {
		general.Infof("SetMemTHP skipped: THPDefaultConfig is empty")
		return
	}
	// If THPDefaultConfig is "never", fast-path to disable THP directly.
	if mode == thpModeNever {
		general.Infof("SetMemTHP: THPDefaultConfig=never, disable THP directly")
		if err := setTHPModeAtPath(thpEnabledPath, thpModeNever); err != nil {
			errList = append(errList, err)
		}
		return
	}

	if emitter == nil || metaServer == nil {
		err := fmt.Errorf("SetMemTHP: nil input, emitter=%T metaServer=%T", emitter, metaServer)
		errList = append(errList, err)
		general.Errorf("%v", err)
		return
	}

	if err := doMemTHP(conf, metaServer, emitter); err != nil {
		errList = append(errList, err)
	}
}

func doMemTHP(conf *coreconfig.Configuration, metaServer *metaserver.MetaServer, emitter metrics.MetricEmitter) error {
	if metaServer == nil || emitter == nil {
		return nil
	}

	// Read mem_order_scores from Malachite system/memory extfrag.
	mc := newMalachiteClient(metaServer.PodFetcher, emitter)
	stats, err := mc.GetSystemMemoryStats()
	if err != nil {
		return fmt.Errorf("get system memory stats failed: %w", err)
	}

	threshold := getHighOrderThreshold(conf)
	enableThreshold := threshold * hysteresisRatio

	maxScore := -1.0
	maxNumaID := -1
	var missingOrders int
	var validNUMACnt int

	for _, ext := range stats.ExtFrag {
		highOrderScore, missing, ok := calcHighOrderScore(ext.MemOrderScores)
		if !ok {
			missingOrders++
			general.Infof("THP extfrag numa=%d missing orders in range [%d,%d]=%v, skip", ext.ID, highOrderMin, highOrderMax, missing)
			continue
		}

		general.Infof("THP extfrag numa=%d highOrderRange=[%d,%d] highOrderScore=%.1f", ext.ID, highOrderMin, highOrderMax, highOrderScore)
		validNUMACnt++
		if highOrderScore > maxScore {
			maxScore = highOrderScore
			maxNumaID = ext.ID
		}
	}

	if missingOrders > 0 {
		general.Infof("THP extfrag missing required orders on %d NUMA nodes", missingOrders)
	}

	decision := decideTHPDecision(maxScore, threshold)
	switch decision {
	case thpDecisionDisable:
		general.Infof("THP disable triggered: maxHighOrderScore=%.1f numa=%d threshold=%.1f", maxScore, maxNumaID, threshold)
		return setTHPModeAtPath(thpEnabledPath, thpModeNever)
	case thpDecisionEnable:
		// Be conservative: only try to recover when we have valid scores for all NUMA nodes.
		// If Malachite misses required orders, keep current THP mode unchanged.
		if validNUMACnt > 0 && missingOrders == 0 {
			mode := normalizeTHPMode(conf.THPDefaultConfig)
			if mode == "" {
				mode = thpModeMadvise
			}
			general.Infof("THP enable triggered: maxHighOrderScore=%.1f enableThreshold=%.1f threshold=%.1f recoverTo=%s", maxScore, enableThreshold, threshold, mode)
			return setTHPModeAtPath(thpEnabledPath, mode)
		}
		general.Infof("THP enable skipped due to missing order scores: maxHighOrderScore=%.1f enableThreshold=%.1f threshold=%.1f missingOrders=%d", maxScore, enableThreshold, threshold, missingOrders)
		return nil
	default:
		// Keep current mode to avoid flapping between disable/enable.
		return nil
	}
}

func getHighOrderThreshold(conf *coreconfig.Configuration) float64 {
	if conf == nil {
		return defaultHighOrderThreshold
	}

	val := float64(conf.THPHighOrderScoreThreshold)
	if val <= 0 {
		return defaultHighOrderThreshold
	}
	// Clamp to a reasonable range.
	return general.Clamp(val, 1, 100)
}

func decideTHPDecision(maxScore, threshold float64) thpDecision {
	if threshold <= 0 {
		threshold = defaultHighOrderThreshold
	}

	// Use two thresholds to avoid flapping:
	// - Disable threshold: maxScore > threshold
	// - Enable threshold:  maxScore < threshold*hysteresisRatio
	if maxScore > threshold {
		return thpDecisionDisable
	}
	if maxScore >= 0 && maxScore < threshold*hysteresisRatio {
		return thpDecisionEnable
	}
	return thpDecisionNone
}

func calcHighOrderScore(scores []malachitetypes.MemOrderScore) (float64, []int, bool) {
	minOrder, maxOrder := highOrderMin, highOrderMax
	if len(scores) == 0 {
		return 0, nil, false
	}

	expected := make(map[int]uint64, maxOrder-minOrder+1)
	for _, s := range scores {
		o := int(s.Order)
		if o < minOrder || o > maxOrder {
			continue
		}
		expected[o] = s.Score
	}

	missing := make([]int, 0)
	var sum float64
	for o := minOrder; o <= maxOrder; o++ {
		s, ok := expected[o]
		if !ok {
			missing = append(missing, o)
			continue
		}
		sum += float64(s)
	}

	if len(missing) > 0 {
		return 0, missing, false
	}
	count := float64(maxOrder - minOrder + 1)
	return sum / count, nil, true
}

func setTHPModeAtPath(path, mode string) error {
	normalizedMode := normalizeTHPMode(mode)
	switch normalizedMode {
	case thpModeMadvise, thpModeAlways, thpModeNever:
	default:
		return fmt.Errorf("invalid THP mode %q, expected one of %q/%q/%q", normalizedMode, thpModeMadvise, thpModeAlways, thpModeNever)
	}

	content, err := procfsm.ReadFileNoStat(path)
	if err != nil {
		return fmt.Errorf("read THP enabled file %s failed: %w", path, err)
	}
	contentStr := string(content)
	current := strings.TrimSpace(contentStr)

	// Avoid redundant writes:
	// - Typical sysfs format: "always [madvise] never"
	// - Our unit tests may use plain "madvise"/"never" content.
	if current == normalizedMode || strings.Contains(contentStr, fmt.Sprintf("[%s]", normalizedMode)) {
		general.Infof("THP already in %s, skip writing %q to %s", normalizedMode, normalizedMode, path)
		return nil
	}

	if err := procfsm.ApplyTransparentHugepageEnabledAtPath(path, normalizedMode); err != nil {
		return fmt.Errorf("set THP mode failed, write %q to %s: %w", normalizedMode, path, err)
	}

	// Best-effort verify; sysfs content may vary, so only log on unexpected.
	newContent, rerr := procfsm.ReadFileNoStat(path)
	if rerr == nil && len(newContent) > 0 && !strings.Contains(string(newContent), normalizedMode) {
		return fmt.Errorf("set THP mode verification failed: wrote=%q path=%s content=%q", normalizedMode, path, strings.TrimSpace(string(newContent)))
	}

	general.Infof("THP set to %s by writing %q to %s", normalizedMode, normalizedMode, path)
	return nil
}

func normalizeTHPMode(mode string) string {
	return strings.TrimSpace(strings.ToLower(mode))
}
