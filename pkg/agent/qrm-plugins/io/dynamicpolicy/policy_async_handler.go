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

package dynamicpolicy

import (
	"fmt"

	v1 "k8s.io/api/core/v1"

	"github.com/kubewharf/katalyst-core/pkg/agent/qrm-plugins/commonstate"
	"github.com/kubewharf/katalyst-core/pkg/agent/qrm-plugins/io/dynamicpolicy/state"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
)

// setExtraControlKnobByConfigForAllocationInfo sets control knob entry for container,
// if the container doesn't have the entry in the checkpoint.
// pod specified value has higher priority than config value.
func setExtraControlKnobByConfigForAllocationInfo(allocationInfo *state.AllocationInfo, extraControlKnobConfigs commonstate.ExtraControlKnobConfigs, pod *v1.Pod) {
}

func (p *DynamicPolicy) setExtraControlKnobByConfigs() {
	general.Infof("called")
	fmt.Printf("BBLU setExtraControlKnobByConfigs..\n")
}

func (p *DynamicPolicy) applyExternalCgroupParams() {
	general.Infof("called")
	fmt.Printf("BBLU applyExternalCgroupParams..\n")
}
