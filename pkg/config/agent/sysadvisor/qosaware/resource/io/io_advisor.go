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

package io

import (
	"github.com/kubewharf/katalyst-core/pkg/agent/sysadvisor/types"
)

// IOAdvisorConfiguration stores configurations of io advisors in qos aware plugin
type IOAdvisorConfiguration struct {
	IOAdvisorPlugins []types.IOAdvisorPluginName
}

// NewIOAdvisorConfiguration creates new io advisor configurations
func NewIOAdvisorConfiguration() *IOAdvisorConfiguration {
	return &IOAdvisorConfiguration{
		IOAdvisorPlugins: make([]types.IOAdvisorPluginName, 0),
	}
}
