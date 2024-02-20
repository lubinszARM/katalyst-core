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

package dynamicpolicy

import (
	"context"
	"fmt"
	"net"
	"os"
	"path"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/kubewharf/katalyst-core/pkg/agent/qrm-plugins/advisorsvc"
	"github.com/kubewharf/katalyst-core/pkg/agent/qrm-plugins/util"
	"github.com/kubewharf/katalyst-core/pkg/config"
	dynamicconfig "github.com/kubewharf/katalyst-core/pkg/config/agent/dynamic"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
	"github.com/kubewharf/katalyst-core/pkg/util/process"
)

const (
	ioAdvisorLWRecvTimeMonitorName              = "ioAdvisorLWRecvTimeMonitor"
	ioAdvisorLWRecvTimeMonitorDurationThreshold = 30 * time.Second
	ioAdvisorLWRecvTimeMonitorInterval          = 30 * time.Second
)

// initAdvisorClientConn initializes io-advisor related connections
func (p *DynamicPolicy) initAdvisorClientConn() (err error) {
	ioAdvisorConn, err := process.Dial(p.ioAdvisorSocketAbsPath, 5*time.Second)
	if err != nil {
		err = fmt.Errorf("get io advisor connection with socket: %s failed with error: %v", p.ioAdvisorSocketAbsPath, err)
		return
	}

	p.advisorClient = advisorsvc.NewAdvisorServiceClient(ioAdvisorConn)
	p.advisorConn = ioAdvisorConn
	return nil
}

// lwIOAdvisorServer works as a client to connect with io-advisor.
// it will wait to receive allocations from io-advisor, and perform allocate actions
func (p *DynamicPolicy) lwIOAdvisorServer(stopCh <-chan struct{}) error {
	general.Infof("called")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-stopCh
		general.Infof("received stop signal, stop calling ListAndWatch of IOAdvisorServer")
		cancel()
	}()

	stream, err := p.advisorClient.ListAndWatch(ctx, &advisorsvc.Empty{})
	if err != nil {
		return fmt.Errorf("call ListAndWatch of IOAdvisorServer failed with error: %v", err)
	}

	for {
		resp, err := stream.Recv()

		if p.lwRecvTimeMonitor != nil {
			p.lwRecvTimeMonitor.UpdateRefreshTime()
		}

		if err != nil {
			_ = p.emitter.StoreInt64(util.MetricNameLWAdvisorServerFailed, 1, metrics.MetricTypeNameRaw)
			return fmt.Errorf("receive ListAndWatch response of IOAdvisorServer failed with error: %v, grpc code: %v",
				err, status.Code(err))
		}

		err = p.handleAdvisorResp(resp)
		if err != nil {
			general.Errorf("handle ListAndWatch response of IOAdvisorServer failed with error: %v", err)
		}
	}
}

func (p *DynamicPolicy) handleAdvisorResp(advisorResp *advisorsvc.ListAndWatchResponse) (retErr error) {
	if advisorResp == nil {
		return fmt.Errorf("handleAdvisorResp got nil advisorResp")
	}
	fmt.Printf("BBLU handleAdvisorResp....\n")
	general.Infof("called")
	p.Lock()
	defer func() {
		p.Unlock()
		if retErr != nil {
			_ = p.emitter.StoreInt64(util.MetricNameHandleAdvisorRespFailed, 1, metrics.MetricTypeNameRaw)
		}
	}()
	return nil
}

// serveForAdvisor starts a server for io-advisor (as a client) to connect with
func (p *DynamicPolicy) serveForAdvisor(stopCh <-chan struct{}) {
	ioPluginSocketDir := path.Dir(p.ioPluginSocketAbsPath)

	err := general.EnsureDirectory(ioPluginSocketDir)
	if err != nil {
		general.Errorf("ensure ioPluginSocketDir: %s failed with error: %v", ioPluginSocketDir, err)
		return
	}

	general.Infof("ensure ioPluginSocketDir: %s successfully", ioPluginSocketDir)
	if err := os.Remove(p.ioPluginSocketAbsPath); err != nil && !os.IsNotExist(err) {
		general.Errorf("failed to remove %s: %v", p.ioPluginSocketAbsPath, err)
		return
	}

	sock, err := net.Listen("unix", p.ioPluginSocketAbsPath)
	if err != nil {
		general.Errorf("listen at socket: %s failed with err: %v", p.ioPluginSocketAbsPath, err)
		return
	}
	general.Infof("listen at: %s successfully", p.ioPluginSocketAbsPath)

	grpcServer := grpc.NewServer()
	advisorsvc.RegisterQRMServiceServer(grpcServer, p)

	exitCh := make(chan struct{})
	go func() {
		general.Infof("starting io plugin checkpoint grpc server at socket: %s", p.ioPluginSocketAbsPath)
		if err := grpcServer.Serve(sock); err != nil {
			general.Errorf("io plugin checkpoint grpc server crashed with error: %v at socket: %s", err, p.ioPluginSocketAbsPath)
		} else {
			general.Infof("io plugin checkpoint grpc server at socket: %s exists normally", p.ioPluginSocketAbsPath)
		}

		exitCh <- struct{}{}
	}()

	if conn, err := process.Dial(p.ioPluginSocketAbsPath, 5*time.Second); err != nil {
		grpcServer.Stop()
		general.Errorf("dial check at socket: %s failed with err: %v", p.ioPluginSocketAbsPath, err)
	} else {
		_ = conn.Close()
	}

	select {
	case <-exitCh:
		return
	case <-stopCh:
		grpcServer.Stop()
		return
	}
}

func (p *DynamicPolicy) ListContainers(context.Context, *advisorsvc.Empty) (*advisorsvc.ListContainersResponse, error) {
	general.Infof("called")
	resp := &advisorsvc.ListContainersResponse{}
	/*
		podEntries := p.state.GetPodResourceEntries()[v1.ResourceMemory]
		for _, entries := range podEntries {
			for _, allocationInfo := range entries {
				if allocationInfo == nil {
					continue
				}

				containerType, found := pluginapi.ContainerType_value[allocationInfo.ContainerType]
				if !found {
					return nil, fmt.Errorf("sync pod: %s/%s, container: %s to memory advisor failed with error: containerType: %s not found",
						allocationInfo.PodNamespace, allocationInfo.PodName,
						allocationInfo.ContainerName, allocationInfo.ContainerType)
				}

				resp.Containers = append(resp.Containers, &advisorsvc.ContainerMetadata{
					PodUid:         allocationInfo.PodUid,
					PodNamespace:   allocationInfo.PodNamespace,
					PodName:        allocationInfo.PodName,
					ContainerName:  allocationInfo.ContainerName,
					ContainerType:  pluginapi.ContainerType(containerType),
					ContainerIndex: allocationInfo.ContainerIndex,
					Labels:         maputil.CopySS(allocationInfo.Labels),
					Annotations:    maputil.CopySS(allocationInfo.Annotations),
					QosLevel:       allocationInfo.QoSLevel,
				})
			}
		}
	*/
	return resp, nil
}

// handleAdvisorIOWeight handles io.weight provisions from io-advisor
func (p *DynamicPolicy) handleAdvisorIOWeight(_ *config.Configuration,
	_ interface{},
	_ *dynamicconfig.DynamicAgentConfiguration,
	emitter metrics.MetricEmitter,
	metaServer *metaserver.MetaServer,
	entryName, subEntryName string,
	calculationInfo *advisorsvc.CalculationInfo) error {
	fmt.Printf("BBLU got handleAdvisorIOWeight..\n")
	return nil

}
