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

package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/kubewharf/katalyst-core/pkg/agent/sysadvisor/types"
	tmoconf "github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic/tmo"
)

func TestTMOBlockFnFromDynamicConfig(t *testing.T) {
	t.Parallel()

	blockTargetPSM := &tmoconf.TMOBlockConfig{
		LabelsSelector: labels.SelectorFromSet(labels.Set{"psm": "data.merlin.seed_project_cn"}),
	}

	tests := []struct {
		name     string
		ci       *types.ContainerInfo
		conf     interface{}
		expected bool
	}{
		{
			name:     "nil dynamic config never blocks",
			ci:       &types.ContainerInfo{NativeQoSClass: "Guaranteed"},
			conf:     nil,
			expected: false,
		},
		{
			name:     "native qos class matched is blocked",
			ci:       &types.ContainerInfo{NativeQoSClass: "Guaranteed", QoSLevel: "shared_cores"},
			conf:     &tmoconf.TMOBlockConfig{NativeQoSClasses: []string{"Guaranteed"}},
			expected: true,
		},
		{
			name:     "native qos class unmatched is not blocked",
			ci:       &types.ContainerInfo{NativeQoSClass: "Burstable", QoSLevel: "shared_cores"},
			conf:     &tmoconf.TMOBlockConfig{NativeQoSClasses: []string{"Guaranteed"}},
			expected: false,
		},
		{
			name:     "runtime class matched is blocked",
			ci:       &types.ContainerInfo{NativeQoSClass: "Burstable", RuntimeClassName: "kata-duma"},
			conf:     &tmoconf.TMOBlockConfig{RuntimeClasses: []string{"kata-duma"}},
			expected: true,
		},
		{
			name:     "runtime class unmatched is not blocked",
			ci:       &types.ContainerInfo{NativeQoSClass: "Burstable", RuntimeClassName: "runc"},
			conf:     &tmoconf.TMOBlockConfig{RuntimeClasses: []string{"kata-duma"}},
			expected: false,
		},
		{
			name:     "label selector matched is blocked",
			ci:       &types.ContainerInfo{Labels: map[string]string{"psm": "data.merlin.seed_project_cn"}},
			conf:     blockTargetPSM,
			expected: true,
		},
		{
			name:     "label selector unmatched is not blocked",
			ci:       &types.ContainerInfo{},
			conf:     blockTargetPSM,
			expected: false,
		},
		{
			name:     "annotation selector matched is blocked",
			ci:       &types.ContainerInfo{Annotations: map[string]string{"k": "v"}},
			conf:     &tmoconf.TMOBlockConfig{AnnotationsSelector: labels.SelectorFromSet(labels.Set{"k": "v"})},
			expected: true,
		},
		{
			name: "target psm burstable without runtime class is not blocked",
			ci: &types.ContainerInfo{
				Labels:           map[string]string{"psm": "data.merlin.seed_project_cn"},
				NativeQoSClass:   "Burstable",
				RuntimeClassName: "",
			},
			conf: &tmoconf.TMOBlockConfig{
				NativeQoSClasses:    []string{"Guaranteed"},
				RuntimeClasses:      []string{"kata-duma"},
				AnnotationsSelector: labels.SelectorFromSet(labels.Set{"k": "v"}),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, TMOBlockFnFromDynamicConfig(tt.ci, nil, tt.conf))
		})
	}
}
