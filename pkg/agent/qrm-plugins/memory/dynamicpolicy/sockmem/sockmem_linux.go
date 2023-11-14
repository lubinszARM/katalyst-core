//go:build linux
// +build linux

/*
Copyright 2023 The Katalyst Authors.

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

package sockmem

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	info "github.com/google/cadvisor/info/v1"
	"golang.org/x/sys/unix"

	cgroupcm "github.com/kubewharf/katalyst-core/pkg/util/cgroup/common"
	cgroupmgr "github.com/kubewharf/katalyst-core/pkg/util/cgroup/manager"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
)

type SockMemConfig struct {
	hostTCPMemRatio   int
	cgroupTCPMemRatio int
}

var sockMemConfig = SockMemConfig{
	hostTCPMemRatio:   20,  // default: 20% * {host total memory}
	cgroupTCPMemRatio: 100, // default: 100% * {cgroup memory limit}
}

func UpdateHostTCPMemRatio(ratio int) {
	if ratio < hostTCPMemRatioMin {
		ratio = hostTCPMemRatioMin
	} else if ratio > hostTCPMemRatioMax {
		ratio = hostTCPMemRatioMax
	}
	sockMemConfig.hostTCPMemRatio = ratio
}

func UpdateCgroupTCPMemRatio(ratio int) {
	if ratio < cgroupTCPMemRatioMin {
		ratio = cgroupTCPMemRatioMin
	} else if ratio > cgroupTCPMemRatioMax {
		ratio = cgroupTCPMemRatioMax
	}
	sockMemConfig.cgroupTCPMemRatio = ratio
}

func getHostTCPMemFile(TCPMemFile string) ([]uint64, error) {
	data, err := os.ReadFile(TCPMemFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s, err %v", TCPMemFile, err)
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty content in %s", TCPMemFile)
	}

	line := lines[0]
	fields := strings.Fields(line)
	if len(fields) != 3 {
		return nil, fmt.Errorf("unexpected number of fields in %s", TCPMemFile)
	}

	var tcpMem []uint64
	for _, field := range fields {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return nil, err
		}
		tcpMem = append(tcpMem, value)
	}

	return tcpMem, nil
}

func setHostTCPMemFile(TCPMemFile string, tcpMem []uint64) error {
	if len(tcpMem) != 3 {
		return fmt.Errorf("tcpMem array must have exactly three elements")
	}

	content := fmt.Sprintf("%d\t%d\t%d\n", tcpMem[0], tcpMem[1], tcpMem[2])
	err := os.WriteFile(TCPMemFile, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("failed to write to %s, err %v", TCPMemFile, err)
	}

	return nil
}

func SetHostTCPMem(machineInfo *info.MachineInfo) {
	tcpMemRatio := sockMemConfig.hostTCPMemRatio
	if tcpMemRatio < hostTCPMemRatioMin {
		tcpMemRatio = hostTCPMemRatioMin
	} else if tcpMemRatio > hostTCPMemRatioMax {
		tcpMemRatio = hostTCPMemRatioMax
	}
	fmt.Printf("BBLU machinInfo1111: %v...\n", machineInfo)

	tcpMem, err := getHostTCPMemFile(hostTCPMemFile)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("BBLU tcpMem:%v..\n", tcpMem)

	pageSize := uint64(unix.Getpagesize())
	memTotal := machineInfo.MemoryCapacity

	newUpperLimit := memTotal / pageSize / 100 * uint64(tcpMemRatio)

	if (newUpperLimit != tcpMem[2]) && (newUpperLimit > tcpMem[1]) {
		tcpMem[2] = newUpperLimit
		setHostTCPMemFile(hostTCPMemFile, tcpMem)
	}

	fmt.Printf("BBLU setTCPMEM:%v, %v.\n", tcpMemRatio, newUpperLimit)
}

func SetCg1TCPMem(podUID, containerID string, memLimit, memTCPLimit uint64) {
	newMemTCPLimit := memLimit / 100 * uint64(sockMemConfig.cgroupTCPMemRatio)
	if newMemTCPLimit < cgroupTCPMemMin2G {
		newMemTCPLimit = cgroupTCPMemMin2G
	}
	if newMemTCPLimit == kernSockMemOff {
		newMemTCPLimit = kernSockMemEnabled
	}

	cgroupPath, err := cgroupcm.GetContainerRelativeCgroupPath(podUID, containerID)
	if err != nil {
		return
	}

	fmt.Printf("BBLU Apply TCPMemLimitInBytes: %v, old value=%d, new value=%d\n", cgroupPath, memTCPLimit, newMemTCPLimit)
	if newMemTCPLimit != memTCPLimit {
		_ = cgroupmgr.ApplyMemoryWithRelativePath(cgroupPath, &cgroupcm.MemoryData{
			TCPMemLimitInBytes: int64(newMemTCPLimit),
		})
		general.Infof("Apply TCPMemLimitInBytes: %v, old value=%d, new value=%d", cgroupPath, memTCPLimit, newMemTCPLimit)
	}
}
