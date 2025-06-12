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

package dirtymem

import (
	"fmt"
	"io/ioutil"
	"os"

	coreconfig "github.com/kubewharf/katalyst-core/pkg/config"
	"github.com/kubewharf/katalyst-core/pkg/consts"
	coreconsts "github.com/kubewharf/katalyst-core/pkg/consts"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	"github.com/kubewharf/katalyst-core/pkg/metaserver/agent/metric/helper"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	cgcommon "github.com/kubewharf/katalyst-core/pkg/util/cgroup/common"
	"github.com/kubewharf/katalyst-core/pkg/util/cgroup/manager"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
)

var (
	ioCgroupRootPath = cgcommon.GetCgroupRootPath(cgcommon.CgroupSubsysIO)
)

func getWBTValueForDiskType(diskType int, conf *coreconfig.Configuration) (int, bool) {
	switch diskType {
	case consts.DiskTypeHDD:
		if conf.WBTValueHDD == -1 {
			return 0, false
		}
		return conf.WBTValueHDD, true
	case consts.DiskTypeSSD:
		if conf.WBTValueSSD == -1 {
			return 0, false
		}
		return conf.WBTValueSSD, true
	case consts.DiskTypeNVME:
		if conf.WBTValueNVME == -1 {
			return 0, false
		}
		return conf.WBTValueNVME, true
	case consts.DiskTypeVIRTIO:
		if conf.WBTValueVIRTIO == -1 {
			return 0, false
		}
		return conf.WBTValueVIRTIO, true
	default:
		return 0, false // Unsupported disk type
	}
}

func disableIOCost(conf *coreconfig.Configuration) {
	if !cgcommon.CheckCgroup2UnifiedMode() {
		return
	}

	devIDToIOCostQoSData, err := manager.GetIOCostQoSWithAbsolutePath(ioCgroupRootPath)
	if err != nil {
		general.Errorf("GetIOCostQoSWithAbsolutePath failed with error: %v in Init", err)
	}

	disabledIOCostQoSData := &cgcommon.IOCostQoSData{Enable: 0}
	for devID, ioCostQoSData := range devIDToIOCostQoSData {
		if ioCostQoSData == nil {
			general.Warningf("nil ioCostQoSData")
			continue
		} else if ioCostQoSData.Enable == 0 {
			general.Warningf("devID: %s ioCostQoS is already disabled", devID)
			continue
		}

		err = manager.ApplyIOCostQoSWithAbsolutePath(ioCgroupRootPath, devID, disabledIOCostQoSData)
		if err != nil {
			general.Errorf("ApplyIOCostQoSWithAbsolutePath for devID: %s, failed with error: %v", devID, err)
		} else {
			general.Infof("disable ioCostQoS for devID: %s successfully", devID)
		}
	}
}

func SetWBTLimit(conf *coreconfig.Configuration,
	emitter metrics.MetricEmitter, metaServer *metaserver.MetaServer,
) {
	general.Infof("called")
	if conf == nil {
		general.Errorf("nil Conf")
		return
	} else if emitter == nil {
		general.Errorf("nil emitter")
		return
	} else if metaServer == nil {
		general.Errorf("nil metaServer")
		return
	}
	dir, err := ioutil.ReadDir(sysDiskPrefix)
	if err != nil {
		general.Errorf("failed to readdir:%v, err:%v", sysDiskPrefix, err)
		return
	}
	for _, entry := range dir {
		diskType, err := helper.GetDeviceMetric(metaServer.MetricsFetcher, emitter, coreconsts.MetricIODiskType, entry.Name())
		if err != nil {
			continue
		}

		wbtValue, shouldApply := getWBTValueForDiskType(int(diskType), conf)
		if !shouldApply {
			continue
		}

		oldWBTValue, err := helper.GetDeviceMetric(metaServer.MetricsFetcher, emitter, coreconsts.MetricIODiskWBTValue, entry.Name())
		if err != nil {
			continue
		}

		if oldWBTValue == float64(wbtValue) {
			continue // no need to set it.
		}

		wbtFilePath := sysDiskPrefix + "/" + entry.Name() + "/" + wbtSuffix
		general.Infof("Apply WBT, device=%v, old value=%v, new value=%v", entry.Name(), oldWBTValue, wbtValue)
		if wbtValue != 0 {
			disableIOCost(conf)
		}
		err = os.WriteFile(wbtFilePath, []byte(fmt.Sprintf("%d", wbtValue)), 0o644)
		if err != nil {
			general.Errorf("failed to write new wbt:%v to :%v, err:%v", wbtValue, entry.Name(), err)
			continue
		}
		_ = emitter.StoreInt64(metricNameDiskWBT, int64(wbtValue), metrics.MetricTypeNameRaw,
			metrics.ConvertMapToTags(map[string]string{
				"diskName": entry.Name(),
			})...)
	}
}
