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

package global

import (
	cliflag "k8s.io/component-base/cli/flag"

	"github.com/kubewharf/katalyst-core/pkg/config/agent/global"
)

// QRMSockMemOptions holds the configurations for both qrm plugins and sys advisor qrm servers
type QRMSockMemOptions struct {
	// SetHostTCPMemLimit limit host max tcp memory usage.
	SetHostTCPMemLimitRatio int
}

// NewQRMSockMemOptions creates a new options with a default config
func NewQRMSockMemOptions() *QRMSockMemOptions {
	return &QRMSockMemOptions{
		SetHostTCPMemLimitRatio: 20,
	}
}

// AddFlags adds flags to the specified FlagSet.
func (o *QRMSockMemOptions) AddFlags(fss *cliflag.NamedFlagSets) {
	fs := fss.FlagSet("qrm-advisor")

	fs.IntVar(&o.SetHostTCPMemLimitRatio, "qrm-memory-host-tcpmem-limit-ratio", o.SetHostTCPMemLimitRatio, "limit host max tcp memory usage")
}

// ApplyTo fills up config with options
func (o *QRMSockMemOptions) ApplyTo(c *global.QRMSockMemConfiguration) error {
	c.SetHostTCPMemLimitRatio = o.SetHostTCPMemLimitRatio
	return nil
}
