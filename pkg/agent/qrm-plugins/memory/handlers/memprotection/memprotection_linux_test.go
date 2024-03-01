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

package memprotection

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	coreconfig "github.com/kubewharf/katalyst-core/pkg/config"
	"github.com/kubewharf/katalyst-core/pkg/config/agent"
	configagent "github.com/kubewharf/katalyst-core/pkg/config/agent"
	dynamicconfig "github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic"
	"github.com/kubewharf/katalyst-core/pkg/config/agent/qrm"
	"github.com/kubewharf/katalyst-core/pkg/config/generic"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	metaagent "github.com/kubewharf/katalyst-core/pkg/metaserver/agent"
	"github.com/kubewharf/katalyst-core/pkg/metaserver/agent/metric"
	"github.com/kubewharf/katalyst-core/pkg/metaserver/agent/pod"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	"github.com/kubewharf/katalyst-core/pkg/util/machine"
)

func makeMetaServer() (*metaserver.MetaServer, error) {
	server := &metaserver.MetaServer{
		MetaAgent: &metaagent.MetaAgent{},
	}

	cpuTopology, err := machine.GenerateDummyCPUTopology(16, 1, 2)
	if err != nil {
		return nil, err
	}

	server.KatalystMachineInfo = &machine.KatalystMachineInfo{
		CPUTopology: cpuTopology,
	}
	server.MetricsFetcher = metric.NewFakeMetricsFetcher(metrics.DummyMetrics{})
	return server, nil
}

func TestMemProtection(t *testing.T) {
	t.Parallel()
	MemProtectionTaskFunc(&coreconfig.Configuration{
		AgentConfiguration: &agent.AgentConfiguration{
			StaticAgentConfiguration: &configagent.StaticAgentConfiguration{
				QRMPluginsConfiguration: &qrm.QRMPluginsConfiguration{
					MemoryQRMPluginConfig: &qrm.MemoryQRMPluginConfig{
						MemProtectionOptions: qrm.MemProtectionOptions{
							EnableSettingMemProtection: false,
						},
					},
				},
			},
		},
	}, nil, &dynamicconfig.DynamicAgentConfiguration{}, nil, nil)

	MemProtectionTaskFunc(&coreconfig.Configuration{
		AgentConfiguration: &agent.AgentConfiguration{
			StaticAgentConfiguration: &configagent.StaticAgentConfiguration{
				QRMPluginsConfiguration: &qrm.QRMPluginsConfiguration{
					MemoryQRMPluginConfig: &qrm.MemoryQRMPluginConfig{
						MemProtectionOptions: qrm.MemProtectionOptions{
							EnableSettingMemProtection: true,
						},
					},
				},
			},
		},
	}, nil, &dynamicconfig.DynamicAgentConfiguration{}, metrics.DummyMetrics{}, nil)

	MemProtectionTaskFunc(&coreconfig.Configuration{
		AgentConfiguration: &agent.AgentConfiguration{
			StaticAgentConfiguration: &configagent.StaticAgentConfiguration{
				QRMPluginsConfiguration: &qrm.QRMPluginsConfiguration{
					MemoryQRMPluginConfig: &qrm.MemoryQRMPluginConfig{
						MemProtectionOptions: qrm.MemProtectionOptions{
							EnableSettingMemProtection: true,
						},
					},
				},
			},
		},
	}, metrics.DummyMetrics{}, &dynamicconfig.DynamicAgentConfiguration{}, metrics.DummyMetrics{}, nil)

	metaServer, err := makeMetaServer()
	assert.NoError(t, err)
	metaServer.PodFetcher = &pod.PodFetcherStub{PodList: []*v1.Pod{}}

	MemProtectionTaskFunc(&coreconfig.Configuration{
		AgentConfiguration: &agent.AgentConfiguration{
			StaticAgentConfiguration: &configagent.StaticAgentConfiguration{
				QRMPluginsConfiguration: &qrm.QRMPluginsConfiguration{
					MemoryQRMPluginConfig: &qrm.MemoryQRMPluginConfig{
						MemProtectionOptions: qrm.MemProtectionOptions{
							EnableSettingMemProtection: true,
						},
					},
				},
			},
		},
	}, metrics.DummyMetrics{}, &dynamicconfig.DynamicAgentConfiguration{}, metrics.DummyMetrics{}, metaServer)

	MemProtectionTaskFunc(&coreconfig.Configuration{
		AgentConfiguration: &agent.AgentConfiguration{
			StaticAgentConfiguration: &configagent.StaticAgentConfiguration{
				QRMPluginsConfiguration: &qrm.QRMPluginsConfiguration{
					MemoryQRMPluginConfig: &qrm.MemoryQRMPluginConfig{
						MemProtectionOptions: qrm.MemProtectionOptions{
							EnableSettingMemProtection: false,
						},
					},
				},
			},
		},
	}, metrics.DummyMetrics{}, &dynamicconfig.DynamicAgentConfiguration{}, metrics.DummyMetrics{}, metaServer)

	normalPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:  "normalPod",
			Name: "normalPod",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "c",
				},
			},
		},
	}

	metaServer.PodFetcher = &pod.PodFetcherStub{PodList: []*v1.Pod{normalPod}}

	MemProtectionTaskFunc(&coreconfig.Configuration{
		AgentConfiguration: &agent.AgentConfiguration{
			StaticAgentConfiguration: &configagent.StaticAgentConfiguration{
				QRMPluginsConfiguration: &qrm.QRMPluginsConfiguration{
					MemoryQRMPluginConfig: &qrm.MemoryQRMPluginConfig{
						MemProtectionOptions: qrm.MemProtectionOptions{
							EnableSettingMemProtection: true,
						},
					},
				},
			},
		},
	}, metrics.DummyMetrics{}, &dynamicconfig.DynamicAgentConfiguration{}, metrics.DummyMetrics{}, metaServer)

	MemProtectionTaskFunc(&coreconfig.Configuration{
		AgentConfiguration: &agent.AgentConfiguration{
			StaticAgentConfiguration: &configagent.StaticAgentConfiguration{
				QRMPluginsConfiguration: &qrm.QRMPluginsConfiguration{
					MemoryQRMPluginConfig: &qrm.MemoryQRMPluginConfig{
						MemProtectionOptions: qrm.MemProtectionOptions{
							EnableSettingMemProtection:     true,
							MemSoftLimitQoSLevelConfigFile: "",
						},
					},
				},
			},
		},
	}, metrics.DummyMetrics{}, &dynamicconfig.DynamicAgentConfiguration{}, metrics.DummyMetrics{}, metaServer)

	MemProtectionTaskFunc(&coreconfig.Configuration{
		AgentConfiguration: &agent.AgentConfiguration{
			StaticAgentConfiguration: &configagent.StaticAgentConfiguration{
				QRMPluginsConfiguration: &qrm.QRMPluginsConfiguration{
					MemoryQRMPluginConfig: &qrm.MemoryQRMPluginConfig{
						MemProtectionOptions: qrm.MemProtectionOptions{
							EnableSettingMemProtection:     true,
							MemSoftLimitQoSLevelConfigFile: "fake",
						},
					},
				},
			},
		},
	}, metrics.DummyMetrics{}, &dynamicconfig.DynamicAgentConfiguration{}, metrics.DummyMetrics{}, metaServer)

	applyMemSoftLimitQoSLevelConfig(&coreconfig.Configuration{
		AgentConfiguration: &agent.AgentConfiguration{
			StaticAgentConfiguration: &configagent.StaticAgentConfiguration{
				QRMPluginsConfiguration: &qrm.QRMPluginsConfiguration{
					MemoryQRMPluginConfig: &qrm.MemoryQRMPluginConfig{
						MemProtectionOptions: qrm.MemProtectionOptions{
							EnableSettingMemProtection:     true,
							MemSoftLimitQoSLevelConfigFile: "",
						},
					},
				},
			},
		},
	}, metrics.DummyMetrics{}, metaServer)

	jsonContent := `{
		"mem_softlimit": {
			"control_knob_info": {
				"cgroup_subsys_name": "memory",
				"cgroup_version_to_iface_name": {
					"v1": "memory.soft_limit_in_bytes",
					"v2": "memory.low"
				},
				"control_knob_value": "0",
				"oci_property_name": ""
			},
			"pod_explicitly_annotation_key": "MemcgSoftLimitValue",
			"qos_level_to_default_value": {
				"dedicated_cores": "15",
				"shared_cores": "15"
			}
		}
	}`

	// Create a temporary file
	tempFile, err := ioutil.TempFile("", "test.json")
	if err != nil {
		fmt.Println("Error creating temporary file:", err)
		return
	}
	defer os.Remove(tempFile.Name()) // Defer removing the temporary file

	// Write the JSON content to the temporary file
	if _, err := tempFile.WriteString(jsonContent); err != nil {
		fmt.Println("Error writing to temporary file:", err)
		return
	}

	absPath, err := filepath.Abs(tempFile.Name())
	if err != nil {
		fmt.Println("Error obtaining absolute path:", err)
		return
	}

	applyMemSoftLimitQoSLevelConfig(&coreconfig.Configuration{
		AgentConfiguration: &agent.AgentConfiguration{
			StaticAgentConfiguration: &configagent.StaticAgentConfiguration{
				QRMPluginsConfiguration: &qrm.QRMPluginsConfiguration{
					MemoryQRMPluginConfig: &qrm.MemoryQRMPluginConfig{
						MemProtectionOptions: qrm.MemProtectionOptions{
							EnableSettingMemProtection:     true,
							MemSoftLimitQoSLevelConfigFile: absPath,
						},
					},
				},
			},
		},
		GenericConfiguration: &generic.GenericConfiguration{
			QoSConfiguration: nil,
		},
	}, metrics.DummyMetrics{}, metaServer)
}

func TestGetSoftMemLimitInBytes(t *testing.T) {
	t.Parallel()
	// Test case where memRatio is 50% of memLimit
	result := getMemProtectionInBytes(1024*1024*1024, 50)
	expected := uint64(536870912) // 50% of 1GB
	assert.Equal(t, expected, result, "Test case 1 failed")

	// Test case where memRatio is 75% of memLimit
	result = getMemProtectionInBytes(1024*1024*1024, 75)
	expected = uint64(805306368) // 75% of 1GB
	assert.Equal(t, expected, result, "Test case 2 failed")

	// Test case where memRatio is 25% of memLimit
	result = getMemProtectionInBytes(512*1024*1024, 25)
	expected = uint64(134217728) // 25% of 512MB
	assert.Equal(t, expected, result, "Test case 3 failed")

	// Test case where memRatio is 0% (minimum value)
	result = getMemProtectionInBytes(1024*1024*1024, 0)
	expected = uint64(134217728) // 0% should be clamped to the minimum value
	assert.Equal(t, expected, result, "Test case 4 failed")
}

func TestGetUserSpecifiedMemoryProtectionInBytes(t *testing.T) {
	t.Parallel()

	result := getUserSpecifiedMemoryProtectionInBytes("fake", "15")
	var expected int64 = 0
	assert.Equal(t, expected, result, "Test getUserSpecifiedMemProtectionInBytes failed")
}
