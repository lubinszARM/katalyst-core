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

const EnableSetFragMemPeriodicalHandlerName = "SetFragMem"

const (
	// Constants for fragmem related kernel features
	hostFragScoreFile           = "/sys/kernel/debug/extfrag/unusable_index"
	hostFragScoreSyncThreshold  = "/proc/sys/vm/extfrag_threshold"
	hostFragScoreAsyncThreshold = "/proc/sys/vm/compaction_proactiveness"
	hostMemNodePath             = "/sys/devices/system/node/node"

	fragScoreMin     = 500.0
	fragScoreMax     = 900.0
	minFragScoreGap  = 50
	delayCompactTime = 3600
	loadGate         = 100

	/*
	 * Consistent with PAGE_ALLOC_COSTLY_ORDER in the kernel,
	 * we only calculate the fragmentation score of the memory order
	 * exceeding PAGE_ALLOC_COSTLY_ORDER=3.
	 * Ref:
	 * https://github.com/torvalds/linux/blob/v6.4/include/linux/mmzone.h#L43
	 */
	highMemoryOrder = 3
)
