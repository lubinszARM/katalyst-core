//go:build linux
// +build linux

package sockmem

import (
	"fmt"

	coreconfig "github.com/kubewharf/katalyst-core/pkg/config"
	dynamicconfig "github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
)

func SetSockMemLimit(_ *coreconfig.Configuration,
	extraConf interface{}, _ *dynamicconfig.DynamicAgentConfiguration,
	_ metrics.MetricEmitter, metaServer *metaserver.MetaServer) {
	general.Infof("called")
	fmt.Printf("BBLU got here.....\n")
}
