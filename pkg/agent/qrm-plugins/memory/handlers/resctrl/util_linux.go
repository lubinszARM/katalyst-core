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

package resctrl

import (
	"fmt"
	"os"
	"syscall"

	"github.com/kubewharf/katalyst-core/pkg/util/general"
)

const (
	CPUInfoFileName       string = "cpuinfo"
	KernelCmdlineFileName string = "cmdline"

	ResctrlName string = "resctrl"

	ResctrlDir string = "resctrl/"
	RdtInfoDir string = "info"
	L3CatDir   string = "L3"

	ResctrlSchemataName string = "schemata"
	ResctrlCbmMaskName  string = "cbm_mask"
	ResctrlTasksName    string = "tasks"

	// L3SchemataPrefix is the prefix of l3 cat schemata
	L3SchemataPrefix = "L3"
	// MbSchemataPrefix is the prefix of mba schemata
	MbSchemataPrefix = "MB"

	// other cpu vendor like "GenuineIntel"
	AMD_VENDOR_ID = "AuthenticAMD"
)

// MountResctrlSubsystem mounts resctrl fs under the sysFSRoot to enable the kernel feature on supported environment
// NOTE: Linux kernel (>= 4.10), Intel cpu and bare-mental host are required; Also, Intel RDT
// features should be enabled in kernel configurations and kernel commandline.
// For more info, please see https://github.com/intel/intel-cmt-cat/wiki/resctrl
func MountResctrlSubsystem() (bool, error) {
	subsystemPath := "/sys/fs/resctrl"
	err := syscall.Mount(ResctrlName, subsystemPath, ResctrlName, syscall.MS_RELATIME, "")
	if err != nil {
		return false, err
	}
	return true, nil
}

// @return /sys/fs/resctrl/info/L3/cbm_mask
func GetResctrlL3CbmFilePath() string {
	return "/sys/fs/resctrl/info/L3/cbm_mask"
	//return ResctrlL3CbmMask.Path("")
}

// CheckAndTryEnableResctrlCat checks if resctrl and l3_cat are enabled; if not, try to enable the features by mount
// resctrl subsystem; See MountResctrlSubsystem() for the detail.
// It returns whether the resctrl cat is enabled, and the error if failed to enable or to check resctrl interfaces
func CheckAndTryEnableResctrlCat() error {
	// resctrl cat is correctly enabled: l3_cbm path exists
	l3CbmFilePath := GetResctrlL3CbmFilePath()
	_, err := os.Stat(l3CbmFilePath)
	if err == nil {
		return nil
	}
	newMount, err := MountResctrlSubsystem()
	if err != nil {
		return err
	}
	if newMount {
		general.Infof("mount resctrl successfully, resctrl enabled")
	}
	// double check l3_cbm path to ensure both resctrl and cat are correctly enabled
	l3CbmFilePath = GetResctrlL3CbmFilePath()
	_, err = os.Stat(l3CbmFilePath)
	if err != nil {
		return fmt.Errorf("resctrl cat is not enabled, err: %s", err)
	}
	return nil
}

func initCatResctrl() error {
	// check if the resctrl root and l3_cat feature are enabled correctly
	if err := CheckAndTryEnableResctrlCat(); err != nil {
		general.Errorf("check resctrl cat failed, err: %s", err)
		return err
	}
	/*
		for _, group := range resctrlGroupList {
			if err := initCatGroupIfNotExist(group); err != nil {
				klog.Errorf("init cat group dir %v failed, error %v", group, err)
			} else {
				klog.V(5).Infof("create cat dir for group %v successfully", group)
			}
		}*/
	return nil
}
