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
)

const (
	// Constants for host tcp mem ratio
	hostTCPMemRatioMin = 15 // min ratio for host tcp mem: 15%
	hostTCPMemRatioMax = 60 // max ratio for host tcp mem: 60%
	hostTCPMemFile     = "/proc/sys/net/ipv4/tcp_mem"

	// Constants for cgroupv1 sock memory ratio
	kernSockMemMin2G         = 2147483648 // static min value for pod's sockmem: 2G
	kernSockMemRatioMin      = 20         // min ratio for pod's sockmem: 20%
	kernSockMemRatioMax      = 200        // max ratio for pod's sockmem: 200%
	kernSockMemRatioDisabled = 0
)

type SockMemConfig struct {
	hostTCPMemRatio int
}

var sockMemConfig = SockMemConfig{
	hostTCPMemRatio: 20,
}

func UpdateHostTCPMemRatio(ratio int) {
	sockMemConfig.hostTCPMemRatio = ratio
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
