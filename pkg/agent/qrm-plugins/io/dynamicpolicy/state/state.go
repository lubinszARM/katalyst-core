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

package state

import (
	"encoding/json"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	pluginapi "k8s.io/kubelet/pkg/apis/resourceplugin/v1alpha1"

	"github.com/kubewharf/katalyst-core/pkg/agent/qrm-plugins/commonstate"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
)

type AllocationInfo struct {
	PodUid         string `json:"pod_uid,omitempty"`
	PodNamespace   string `json:"pod_namespace,omitempty"`
	PodName        string `json:"pod_name,omitempty"`
	ContainerName  string `json:"container_name,omitempty"`
	ContainerType  string `json:"container_type,omitempty"`
	ContainerIndex uint64 `json:"container_index,omitempty"`
	RampUp         bool   `json:"ramp_up,omitempty"`
	PodRole        string `json:"pod_role,omitempty"`
	PodType        string `json:"pod_type,omitempty"`

	// keyed by control knob names referred in ioadvisor package
	ExtraControlKnobInfo map[string]commonstate.ControlKnobInfo `json:"extra_control_knob_info"`
	Labels               map[string]string                      `json:"labels"`
	Annotations          map[string]string                      `json:"annotations"`
	QoSLevel             string                                 `json:"qosLevel"`
}

type ContainerEntries map[string]*AllocationInfo       // Keyed by container name
type PodEntries map[string]ContainerEntries            // Keyed by pod UID
type PodResourceEntries map[v1.ResourceName]PodEntries // Keyed by resource name

func (ai *AllocationInfo) String() string {
	if ai == nil {
		return ""
	}

	contentBytes, err := json.Marshal(ai)
	if err != nil {
		klog.Errorf("[AllocationInfo.String] marshal AllocationInfo failed with error: %v", err)
		return ""
	}
	return string(contentBytes)
}

func (ai *AllocationInfo) Clone() *AllocationInfo {
	if ai == nil {
		return nil
	}

	clone := &AllocationInfo{
		PodUid:         ai.PodUid,
		PodNamespace:   ai.PodNamespace,
		PodName:        ai.PodName,
		ContainerName:  ai.ContainerName,
		ContainerType:  ai.ContainerType,
		ContainerIndex: ai.ContainerIndex,
		RampUp:         ai.RampUp,
		PodRole:        ai.PodRole,
		PodType:        ai.PodType,
		QoSLevel:       ai.QoSLevel,
		Labels:         general.DeepCopyMap(ai.Labels),
		Annotations:    general.DeepCopyMap(ai.Annotations),
	}

	if ai.ExtraControlKnobInfo != nil {
		clone.ExtraControlKnobInfo = make(map[string]commonstate.ControlKnobInfo)

		for name := range ai.ExtraControlKnobInfo {
			clone.ExtraControlKnobInfo[name] = ai.ExtraControlKnobInfo[name]
		}
	}

	return clone
}

// CheckMainContainer returns true if the AllocationInfo is for main container
func (ai *AllocationInfo) CheckMainContainer() bool {
	return ai.ContainerType == pluginapi.ContainerType_MAIN.String()
}

// CheckSideCar returns true if the AllocationInfo is for side-car container
func (ai *AllocationInfo) CheckSideCar() bool {
	return ai.ContainerType == pluginapi.ContainerType_SIDECAR.String()
}

// GetResourceAllocation transforms resource allocation information into *pluginapi.ResourceAllocation
func (ai *AllocationInfo) GetResourceAllocation() (*pluginapi.ResourceAllocation, error) {
	return nil, nil
}

func (pe PodEntries) Clone() PodEntries {
	if pe == nil {
		return nil
	}

	clone := make(PodEntries)
	for podUID, containerEntries := range pe {
		if containerEntries == nil {
			continue
		}

		clone[podUID] = make(ContainerEntries)
		for containerName, allocationInfo := range containerEntries {
			clone[podUID][containerName] = allocationInfo.Clone()
		}
	}
	return clone
}

// GetMainContainerAllocation returns AllocationInfo that belongs
// the main container for this pod
func (pe PodEntries) GetMainContainerAllocation(podUID string) (*AllocationInfo, bool) {
	for _, allocationInfo := range pe[podUID] {
		if allocationInfo.CheckMainContainer() {
			return allocationInfo, true
		}
	}
	return nil, false
}

func (pre PodResourceEntries) String() string {
	if pre == nil {
		return ""
	}

	contentBytes, err := json.Marshal(pre)
	if err != nil {
		klog.Errorf("[PodResourceEntries.String] marshal PodResourceEntries failed with error: %v", err)
		return ""
	}
	return string(contentBytes)
}

func (pre PodResourceEntries) Clone() PodResourceEntries {
	if pre == nil {
		return nil
	}

	clone := make(PodResourceEntries)
	for resourceName, podEntries := range pre {
		clone[resourceName] = podEntries.Clone()
	}
	return clone
}

/*
// reader is used to get information from local states
type reader interface {
	GetMachineState() NUMANodeResourcesMap
	GetPodResourceEntries() PodResourceEntries
	GetAllocationInfo(resourceName v1.ResourceName, podUID, containerName string) *AllocationInfo
}

// writer is used to store information into local states,
// and it also provides functionality to maintain the local files
type writer interface {
	SetMachineState(numaNodeResourcesMap NUMANodeResourcesMap)
	SetPodResourceEntries(podResourceEntries PodResourceEntries)
	SetAllocationInfo(resourceName v1.ResourceName, podUID, containerName string, allocationInfo *AllocationInfo)

	Delete(resourceName v1.ResourceName, podUID, containerName string)
	ClearState()
}

// ReadonlyState interface only provides methods for tracking pod assignments
type ReadonlyState interface {
	reader

	GetMachineInfo() *info.MachineInfo
}

// State interface provides methods for tracking and setting pod assignments
type State interface {
	writer
	ReadonlyState
}
*/
