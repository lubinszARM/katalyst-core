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
	"github.com/kubewharf/katalyst-core/pkg/util/general"
	"golang.org/x/sys/unix"
)

const (
	// Constants for host tcp mem ratio
	hostTCPMemRatioMin = 15 // min ratio for host tcp mem: 15%
	hostTCPMemRatioMax = 60 // max ratio for host tcp mem: 60%
	hostTCPMemFile     = "/proc/sys/net/ipv4/tcp_mem"
)

func getLimitFromTCPMemFile(TCPMemFile string) (uint64, error) {
	data, err := os.ReadFile(TCPMemFile)
	if err != nil {
		return 0, fmt.Errorf("failed to read %s, err %v", TCPMemFile, err)
	}

	upperLimit := 0
	lines := strings.Split(string(data), "\n")
	if len(lines) >= 1 {
		values := strings.Fields(lines[0])

		if len(values) >= 3 {
			upperLimit, err = strconv.Atoi(values[2])
			if err != nil {
				return 0, fmt.Errorf("error converting upper limit value: %v", err)
			}
		} else {
			return 0, fmt.Errorf("invalid data\n")
		}
	}
	return uint64(upperLimit), nil
}

func setLimitToTCPMemFile(TCPMemFile string, upper uint64) {
	data, err := os.ReadFile(TCPMemFile)
	if err != nil {
		return
	}

	parts := strings.Fields(string(data))
	if len(parts) < 3 {
		general.Errorf("Invalid data in %v", TCPMemFile)
		return
	}

	parts[2] = fmt.Sprintf("%d", upper)
	newData := strings.Join(parts, " ")
	err = os.WriteFile(TCPMemFile, []byte(newData), 0)
	if err != nil {
		general.Errorf("Invalid writing to file %v", TCPMemFile)
		return
	}
}

func SetHostTCPMem(machineInfo *info.MachineInfo) {
	tcpMemRatio := 30
	if tcpMemRatio < hostTCPMemRatioMin {
		tcpMemRatio = hostTCPMemRatioMin
	} else if tcpMemRatio > hostTCPMemRatioMax {
		tcpMemRatio = hostTCPMemRatioMax
	}
	fmt.Printf("BBLU machinInfo1111: %v...\n", machineInfo)
	upperLimit, err := getLimitFromTCPMemFile(hostTCPMemFile)
	if err != nil {
		general.Errorf("set host tcp_mem failed")
		return
	}
	pageSize := uint64(unix.Getpagesize())
	memTotal := machineInfo.MemoryCapacity

	newUpperLimit := memTotal / pageSize / 100 * uint64(tcpMemRatio)
	/*
		if newUpperLimit > upperLimit {
			setLimitToTCPMemFile(hostTCPMemFile, newUpperLimit)
		}
	*/
	fmt.Printf("BBLU setTCPMEM:%v, %v..\n", newUpperLimit, upperLimit)
}
