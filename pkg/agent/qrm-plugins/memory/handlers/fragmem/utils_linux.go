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

type FragScoreResult struct {
	MaxValue     int
	AverageValue int
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

func setHostProactiveCompctFile(ProactiveCompctFile string, threshold uint64) error {
	_, err := os.Stat(ProactiveCompctFile)
	if err != nil {
		return err
	}

	// convert the thresold into the watermark for Proactive Compction.
	// Example, threshold=800:
	// target watermark = 100 - (threshold/10) = 20
	target := uint64(general.Clamp(float64(threshold), fragScoreMin, fragScoreMax))
	target = 100 - (uint64(target) / 10)
	err = os.WriteFile(ProactiveCompctFile, []byte(fmt.Sprintf("%d", target)), 0644)
	if err != nil {
		return fmt.Errorf("failed to write to %s, err %v", ProactiveCompctFile, err)
	}

	return nil
}

func getAverage(values []int) int {
	if len(values) == 0 {
		return 0
	}

	sum := 0
	for _, v := range values {
		sum += v
	}

	return sum / len(values)
}

func getMax(values []int) int {
	if len(values) == 0 {
		// Return a special value or handle the case based on your requirements
		return 0
	}

	max := values[0]
	for _, v := range values {
		if v > max {
			max = v
		}
	}

	return max
}

func parseFloatValues(str string) []float64 {
	var values []float64
	for _, valStr := range regexp.MustCompile(`\S+`).FindAllString(str, -1) {
		val, _ := strconv.ParseFloat(valStr, 64)
		values = append(values, val)
	}
	return values
}

func GetNumaFragScore(fragScoreFile string) ([]int, error) {
	file, err := os.Open(fragScoreFile)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	re := regexp.MustCompile(`^Node ([0-9]+), zone +Normal ([0-9. ]+)`)

	var results []int
	for scanner.Scan() {
		line := scanner.Text()
		match := re.FindStringSubmatch(line)
		if len(match) == 3 {
			valuesStr := match[2]
			values := parseFloatValues(valuesStr)

			if len(values) >= 8 {
				total := 0.0
				highOrder := highMemoryOrder + 1
				for i := highOrder; i < len(values); i++ {
					total += values[i]
				}

				average := int(total / float64(len(values)-highOrder) * 1000)
				results = append(results, average)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("Error scanning file:", err)
		return nil, err
	}

	return results, nil
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
func gatherFragScore(fragScoreFile string) (FragScoreResult, error) {
	results, err := GetNumaFragScore(fragScoreFile)
	if err != nil {
		return FragScoreResult{0, 0}, err
	}

	maxValue := getMax(results)
	averageValue := getAverage(results)

	return FragScoreResult{MaxValue: maxValue, AverageValue: averageValue}, nil
}

func setHostMemCompact(memCompactFile string) {
	_ = os.WriteFile(memCompactFile, []byte(fmt.Sprintf("%d", 1)), 0644)
}
