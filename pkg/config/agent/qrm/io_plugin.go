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

package qrm

type IOQRMPluginConfig struct {
	// PolicyName is used to switch between several strategies
	PolicyName string

	WritebackThrottlingOption
	IOCostOption
	IOWeightOption
}

type WritebackThrottlingOption struct {
	EnableSettingWBT bool
	WBTValueHDD      int
	WBTValueSSD      int
}

type IOCostOption struct {
	EnableSettingIOCost   bool
	IOCostQoSConfigFile   string
	IOCostModelConfigFile string
}

type IOWeightOption struct {
	EnableSettingIOWeight         bool
	IOWeightQoSLevelConfigFile    string
	IOWeightCgroupLevelConfigFile string
}

func NewIOQRMPluginConfig() *IOQRMPluginConfig {
	return &IOQRMPluginConfig{}
}
