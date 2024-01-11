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
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"

	"github.com/kubewharf/katalyst-core/pkg/util/general"
)

type nodeFragScoreInfo struct {
	Node  int
	Score int
}

func setHostExtFragFile(ExtFragFile string, threshold int) error {
	_, err := os.Stat(ExtFragFile)
	if err != nil {
		return err
	}

	target := general.Clamp(float64(threshold), fragScoreMin, fragScoreMax)
	err = os.WriteFile(ExtFragFile, []byte(fmt.Sprintf("%d", uint64(target))), 0644)
	if err != nil {
		return fmt.Errorf("failed to write to %s, err %v", ExtFragFile, err)
	}

	return nil
}

func parseFloatValues(str string) []float64 {
	var values []float64
	for _, valStr := range regexp.MustCompile(`\S+`).FindAllString(str, -1) {
		val, _ := strconv.ParseFloat(valStr, 64)
		values = append(values, val)
	}
	return values
}

/*
* Unusable_index and fragmentation_score have the same algorithm logic
* in the kernel side.
* So, we can aslo calculate it by reading unusable_index
* according to the method in kernel side.
*
* The method of calculating the fragmentation score in kernel:
* https://github.com/torvalds/linux/blob/v6.4/mm/vmstat.c#L1115
* Unusable_index: https://github.com/torvalds/linux/blob/v6.4/mm/vmstat.c#L2134
 */
// GetNumaFragScore parses the fragScoreFile and returns a slice of nodeFragScoreInfo
func GetNumaFragScore(fragScoreFile string) ([]nodeFragScoreInfo, error) {
	file, err := os.Open(fragScoreFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	re := regexp.MustCompile(`^Node ([0-9]+), zone +Normal ([0-9. ]+)`)

	var results []nodeFragScoreInfo
	for scanner.Scan() {
		line := scanner.Text()
		match := re.FindStringSubmatch(line)
		if len(match) == 3 {
			nodeID, err := strconv.Atoi(match[1])
			if err != nil {
				return nil, err
			}
			valuesStr := match[2]
			values := parseFloatValues(valuesStr)

			if len(values) >= 8 {
				total := 0.0
				highOrder := 8
				for i := highOrder; i < len(values); i++ {
					total += values[i]
				}

				average := int(total / float64(len(values)-highOrder) * 1000)
				results = append(results, nodeFragScoreInfo{Node: nodeID, Score: average})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

func setHostMemCompact(node int) {
	targetFile := hostMemNodePath + strconv.Itoa(node) + "/compact"
	_ = os.WriteFile(targetFile, []byte(fmt.Sprintf("%d", 1)), 0644)
}
