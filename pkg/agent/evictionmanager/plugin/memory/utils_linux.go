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

package memory

import (
	"bytes"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var (
	hostZoneinfoFile = "/proc/zoneinfo"
)

var nodeZoneRE = regexp.MustCompile(`(\d+), zone\s+(\w+)`)

func parseZoneinfo(zoneinfoData []byte) ([]nodeWatermarkInfo, error) {
	var zoneinfo []nodeWatermarkInfo
	zoneinfoBlocks := bytes.Split(zoneinfoData, []byte("\nNode"))

	for _, block := range zoneinfoBlocks {
		block = bytes.TrimSpace(block)
		if len(block) == 0 {
			continue
		}

		var zoneinfoElement nodeWatermarkInfo
		zoneinfoElement.node = -1
		lines := strings.Split(string(block), "\n")
		for _, line := range lines {
			if nodeZone := nodeZoneRE.FindStringSubmatch(line); nodeZone != nil {
				if nodeZone[2] == "Normal" {
					zoneinfoElement.node = int64(StringToUint64(nodeZone[1]))
				}
				continue
			}
			if zoneinfoElement.node == -1 {
				continue
			}
			if strings.HasPrefix(strings.TrimSpace(line), "per-node stats") {
				continue
			}
			parts := strings.Fields(strings.TrimSpace(line))
			if len(parts) < 2 {
				continue
			}
			switch parts[0] {
			case "nr_free_pages":
				zoneinfoElement.free = StringToUint64(parts[1])
			case "min":
				zoneinfoElement.min = StringToUint64(parts[1])
			case "low":
				zoneinfoElement.low = StringToUint64(parts[1])
			}
		}
		if zoneinfoElement.node != -1 {
			zoneinfo = append(zoneinfo, zoneinfoElement)
		}
	}
	return zoneinfo, nil
}

func StringToUint64(s string) uint64 {
	uintValue, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return uintValue
}

func ReadZoneinfo() []nodeWatermarkInfo {
	data, err := os.ReadFile(hostZoneinfoFile)
	if err != nil {
		return nil
	}
	zoneinfo, err := parseZoneinfo(data)
	if err != nil {
		return nil
	}
	return zoneinfo
}
