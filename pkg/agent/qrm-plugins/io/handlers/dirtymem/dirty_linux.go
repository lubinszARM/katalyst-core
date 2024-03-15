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

package dirtymem

import (
	"fmt"
	"os"
	"strconv"

	coreconfig "github.com/kubewharf/katalyst-core/pkg/config"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
)

func setHostDirtyFile(DirtyFile string, value uint64) error {
	strValue := strconv.FormatUint(value, 10)
	data := []byte(strValue)

	err := os.WriteFile(DirtyFile, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write to %s, err %v", DirtyFile, err)
	}
	return nil
}

func SetDirtyLimit(conf *coreconfig.Configuration,
	emitter metrics.MetricEmitter, metaServer *metaserver.MetaServer) {
	general.Infof("called")
	if conf == nil {
		general.Errorf("nil Conf")
		return
	} else if emitter == nil {
		general.Errorf("nil emitter")
		return
	} else if metaServer == nil {
		general.Errorf("nil metaServer")
		return
	}

	// SettingSockMem featuregate.
	if !conf.EnableSettingDirty {
		general.Infof("EnableSettingDirty disabled")
		return
	}

	if conf.DirtyBackgroundBytes != -1 {
		_ = setHostDirtyFile(hostDirtyBackgroundBytes, uint64(conf.DirtyBackgroundBytes))
	}

	if conf.DirtyBytes != -1 {
		_ = setHostDirtyFile(hostDirtyBytes, uint64(conf.DirtyBytes))
	}

	if conf.DirtyWritebackCycle != -1 {
		_ = setHostDirtyFile(hostDirtyWritebackCentisecs, uint64(conf.DirtyWritebackCycle))
	}
}
