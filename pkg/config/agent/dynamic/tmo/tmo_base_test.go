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

package tmo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/kubewharf/katalyst-api/pkg/apis/config/v1alpha1"
	"github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic/crd"
)

func TestApplyConfigurationBlockConfig(t *testing.T) {
	t.Parallel()

	conf := NewTransparentMemoryOffloadingConfiguration()
	conf.ApplyConfiguration(&crd.DynamicConfigCRD{
		TransparentMemoryOffloadingConfiguration: &v1alpha1.TransparentMemoryOffloadingConfiguration{
			Spec: v1alpha1.TransparentMemoryOffloadingConfigurationSpec{
				Config: v1alpha1.TransparentMemoryOffloadingConfig{
					BlockConfig: &v1alpha1.BlockConfig{
						Labels: []metav1.LabelSelectorRequirement{
							{
								Key:      "psm",
								Operator: metav1.LabelSelectorOpNotIn,
								Values:   []string{"data.merlin.seed_project_cn"},
							},
						},
						NativeQoSClasses: []corev1.PodQOSClass{
							corev1.PodQOSGuaranteed,
							corev1.PodQOSBestEffort,
						},
						RuntimeClasses: []string{"kata-duma"},
					},
				},
			},
		},
	})

	assert.NotNil(t, conf.BlockConfig)
	assert.NotNil(t, conf.BlockConfig.LabelsSelector)
	assert.True(t, conf.BlockConfig.LabelsSelector.Matches(labels.Set(map[string]string{"psm": "other.psm"})))
	assert.False(t, conf.BlockConfig.LabelsSelector.Matches(labels.Set(map[string]string{"psm": "data.merlin.seed_project_cn"})))
	assert.Equal(t, []string{"Guaranteed", "BestEffort"}, conf.BlockConfig.NativeQoSClasses)
	assert.Equal(t, []string{"kata-duma"}, conf.BlockConfig.RuntimeClasses)
}

func TestApplyConfigurationWithoutBlockConfig(t *testing.T) {
	t.Parallel()

	conf := NewTransparentMemoryOffloadingConfiguration()
	conf.ApplyConfiguration(&crd.DynamicConfigCRD{
		TransparentMemoryOffloadingConfiguration: &v1alpha1.TransparentMemoryOffloadingConfiguration{
			Spec: v1alpha1.TransparentMemoryOffloadingConfigurationSpec{
				Config: v1alpha1.TransparentMemoryOffloadingConfig{},
			},
		},
	})

	assert.NotNil(t, conf.BlockConfig)
	assert.Nil(t, conf.BlockConfig.LabelsSelector)
	assert.Nil(t, conf.BlockConfig.AnnotationsSelector)
	assert.Empty(t, conf.BlockConfig.NativeQoSClasses)
	assert.Empty(t, conf.BlockConfig.RuntimeClasses)
}
