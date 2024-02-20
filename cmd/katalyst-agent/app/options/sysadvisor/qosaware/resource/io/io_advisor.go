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
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kubewharf/katalyst-core/pkg/agent/sysadvisor/types"
	"github.com/kubewharf/katalyst-core/pkg/config/agent/sysadvisor/qosaware/resource/io"
)

// IOAdvisorOptions holds the configurations for io advisor in qos aware plugin
type IOAdvisorOptions struct {
	IOAdvisorPlugins []string
	DefaultIOWeight  resource.QuantityValue
}

// NewIOAdvisorOptions creates a new Options with a default config
func NewIOAdvisorOptions() *IOAdvisorOptions {
	return &IOAdvisorOptions{
		IOAdvisorPlugins: []string{},
		DefaultIOWeight:  resource.QuantityValue{Quantity: resource.MustParse("100")},
	}
}

// AddFlags adds flags to the specified FlagSet.
func (o *IOAdvisorOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringSliceVar(&o.IOAdvisorPlugins, "io-advisor-plugins", o.IOAdvisorPlugins,
		"io advisor plugins to use.")
	fs.Var(&o.DefaultIOWeight, "io-advisor-default-io-weight", "default-io-weight")
}

// ApplyTo fills up config with options
func (o *IOAdvisorOptions) ApplyTo(c *io.IOAdvisorConfiguration) error {
	for _, plugin := range o.IOAdvisorPlugins {
		c.IOAdvisorPlugins = append(c.IOAdvisorPlugins, types.IOAdvisorPluginName(plugin))
	}
	c.DefaultIOWeight = o.DefaultIOWeight.Value()

	return nil
}
