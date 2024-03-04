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

package iocost

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kubewharf/katalyst-core/pkg/config"
	coreconfig "github.com/kubewharf/katalyst-core/pkg/config"
	"github.com/kubewharf/katalyst-core/pkg/config/agent"
	configagent "github.com/kubewharf/katalyst-core/pkg/config/agent"
	dynamicconfig "github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic"
	"github.com/kubewharf/katalyst-core/pkg/config/agent/qrm"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	"github.com/kubewharf/katalyst-core/pkg/util/cgroup/common"
)

func TestSetIOCost(t *testing.T) {
	t.Parallel()
	SetIOCost(&coreconfig.Configuration{
		AgentConfiguration: &agent.AgentConfiguration{
			StaticAgentConfiguration: &configagent.StaticAgentConfiguration{
				QRMPluginsConfiguration: &qrm.QRMPluginsConfiguration{
					IOQRMPluginConfig: &qrm.IOQRMPluginConfig{
						IOCostOption: qrm.IOCostOption{
							EnableSettingIOCost: false,
						},
					},
				},
			},
		},
	}, nil, &dynamicconfig.DynamicAgentConfiguration{}, nil, nil)

	SetIOCost(&coreconfig.Configuration{
		AgentConfiguration: &agent.AgentConfiguration{
			StaticAgentConfiguration: &configagent.StaticAgentConfiguration{
				QRMPluginsConfiguration: &qrm.QRMPluginsConfiguration{
					IOQRMPluginConfig: &qrm.IOQRMPluginConfig{
						IOCostOption: qrm.IOCostOption{
							EnableSettingIOCost: true,
						},
					},
				},
			},
		},
	}, nil, &dynamicconfig.DynamicAgentConfiguration{}, nil, nil)

	SetIOCost(&coreconfig.Configuration{
		AgentConfiguration: &agent.AgentConfiguration{
			StaticAgentConfiguration: &configagent.StaticAgentConfiguration{
				QRMPluginsConfiguration: &qrm.QRMPluginsConfiguration{
					IOQRMPluginConfig: &qrm.IOQRMPluginConfig{
						IOCostOption: qrm.IOCostOption{
							EnableSettingIOCost:        true,
							EnableSettingIOCostHDDOnly: true,
						},
					},
				},
			},
		},
	}, nil, &dynamicconfig.DynamicAgentConfiguration{}, nil, nil)
}
func Test_disableIOCost(t *testing.T) {
	type args struct {
		conf *config.Configuration
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "disableIOCost with EnableIOCostControl: false",
			args: args{
				conf: &config.Configuration{
					AgentConfiguration: &agent.AgentConfiguration{
						StaticAgentConfiguration: &configagent.StaticAgentConfiguration{
							QRMPluginsConfiguration: &qrm.QRMPluginsConfiguration{
								IOQRMPluginConfig: &qrm.IOQRMPluginConfig{
									IOCostOption: qrm.IOCostOption{
										EnableSettingIOCost: false,
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "disableIOCost with EnableIOCostControl: true",
			args: args{
				conf: &config.Configuration{
					AgentConfiguration: &agent.AgentConfiguration{
						StaticAgentConfiguration: &configagent.StaticAgentConfiguration{
							QRMPluginsConfiguration: &qrm.QRMPluginsConfiguration{
								IOQRMPluginConfig: &qrm.IOQRMPluginConfig{
									IOCostOption: qrm.IOCostOption{
										EnableSettingIOCost:        true,
										EnableSettingIOCostHDDOnly: true,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			disableIOCost(tt.args.conf)
		})
	}
}

func Test_applyIOCostConfig(t *testing.T) {
	type args struct {
		conf    *config.Configuration
		emitter metrics.MetricEmitter
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "applyIOCostConfig with EnableIOCostControl: false",
			args: args{
				conf: &config.Configuration{
					AgentConfiguration: &agent.AgentConfiguration{
						StaticAgentConfiguration: &configagent.StaticAgentConfiguration{
							QRMPluginsConfiguration: &qrm.QRMPluginsConfiguration{
								IOQRMPluginConfig: &qrm.IOQRMPluginConfig{
									IOCostOption: qrm.IOCostOption{
										EnableSettingIOCost: false,
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "applyIOCostConfig without config file",
			args: args{
				conf: &config.Configuration{
					AgentConfiguration: &agent.AgentConfiguration{
						StaticAgentConfiguration: &configagent.StaticAgentConfiguration{
							QRMPluginsConfiguration: &qrm.QRMPluginsConfiguration{
								IOQRMPluginConfig: &qrm.IOQRMPluginConfig{
									IOCostOption: qrm.IOCostOption{
										EnableSettingIOCost: true,
									},
								},
							},
						}},
				},
			},
		},
		{
			name: "applyIOCostConfig with config file",
			args: args{
				conf: &config.Configuration{
					AgentConfiguration: &agent.AgentConfiguration{
						StaticAgentConfiguration: &configagent.StaticAgentConfiguration{
							QRMPluginsConfiguration: &qrm.QRMPluginsConfiguration{
								IOQRMPluginConfig: &qrm.IOQRMPluginConfig{
									IOCostOption: qrm.IOCostOption{
										EnableSettingIOCost:        true,
										EnableSettingIOCostHDDOnly: true,
										IOCostModelConfigFile:      "/tmp/fakeFile1",
										IOCostQoSConfigFile:        "/tmp/fakeFile2",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyIOCostConfig(tt.args.conf, tt.args.emitter)
		})
	}
}

func Test_reportDevicesIOCostVrate(t *testing.T) {
	type args struct {
		emitter metrics.MetricEmitter
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "reportDevicesIOCostVrate with nil emitter",
			args: args{
				emitter: nil,
			},
		},
		{
			name: "reportDevicesIOCostVrate with fake emitter",
			args: args{
				emitter: metrics.DummyMetrics{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reportDevicesIOCostVrate(tt.args.emitter)
		})
	}
}

func Test_applyIOCostModel(t *testing.T) {
	type args struct {
		ioCostModelConfigs map[DevModel]*common.IOCostModelData
		devsIDToModel      map[string]DevModel
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "applyIOCostModel with fake data",
			args: args{
				ioCostModelConfigs: map[DevModel]*common.IOCostModelData{
					DevModelDefault: &common.IOCostModelData{
						CtrlMode: common.IOCostCtrlModeAuto,
					},
				},
				devsIDToModel: map[string]DevModel{
					"fakeID": DevModelDefault,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyIOCostModel(tt.args.ioCostModelConfigs, tt.args.devsIDToModel)
		})
	}
}

func TestApplyIOCostQoSWithAbsolutePathIsHDDTrue(t *testing.T) {
	t.Parallel()
	devsIDToModel := map[string]DevModel{
		"test": "default",
	}
	applyIOCostQoSWithDefault(nil, devsIDToModel, true)
	ioCostQoSData := common.IOCostQoSData{}
	err := applyIOCostQoSWithAbsolutePath("absPath", "devID", &ioCostQoSData, true)
	assert.Error(t, err)
	err = applyIOCostQoSWithAbsolutePath("absPath", "devID", &ioCostQoSData, false)
	assert.Error(t, err)
	err = applyIOCostQoSWithAbsolutePath("absPath", "8:1", &ioCostQoSData, true)
	assert.Error(t, err)
	err = applyIOCostQoSWithAbsolutePath("absPath", "254:0", &ioCostQoSData, true)
	assert.Error(t, err)
	err = applyIOCostQoSWithAbsolutePath("absPath", "254:16", &ioCostQoSData, true)
	assert.Error(t, err)
	err = applyIOCostQoSWithAbsolutePath("absPath", "11:0", &ioCostQoSData, true)
	assert.Error(t, err)
	err = applyIOCostQoSWithAbsolutePath("absPath", "259:0", &ioCostQoSData, true)
	assert.Error(t, err)
}
