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

package server

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"

	"github.com/kubewharf/katalyst-core/pkg/agent/qrm-plugins/advisorsvc"
	"github.com/kubewharf/katalyst-core/pkg/agent/sysadvisor/metacache"
	"github.com/kubewharf/katalyst-core/pkg/agent/sysadvisor/types"
	"github.com/kubewharf/katalyst-core/pkg/config"
	"github.com/kubewharf/katalyst-core/pkg/metaserver"
	"github.com/kubewharf/katalyst-core/pkg/metrics"
	"github.com/kubewharf/katalyst-core/pkg/util/general"
)

const (
	ioServerName string = "io-server"
)

type ioServer struct {
	*baseServer
	ioPluginClient     advisorsvc.QRMServiceClient
	listAndWatchCalled bool
}

func NewIOServer(recvCh chan types.InternalIOCalculationResult, sendCh chan types.TriggerInfo, conf *config.Configuration,
	metaCache metacache.MetaCache, metaServer *metaserver.MetaServer, emitter metrics.MetricEmitter) (*ioServer, error) {
	cs := &ioServer{}
	cs.baseServer = newBaseServer(ioServerName, conf, recvCh, sendCh, metaCache, metaServer, emitter, cs)
	cs.advisorSocketPath = conf.IOAdvisorSocketAbsPath
	cs.pluginSocketPath = conf.IOPluginSocketAbsPath
	cs.resourceRequestName = "IORequest"
	return cs, nil
}

func (is *ioServer) RegisterAdvisorServer() {
	grpcServer := grpc.NewServer()
	advisorsvc.RegisterAdvisorServiceServer(grpcServer, is)
	is.grpcServer = grpcServer
}

// Start is override to list containers when starting up
func (is *ioServer) Start() error {
	if err := is.baseServer.Start(); err != nil {
		return err
	}

	if err := is.StartListContainers(); err != nil {
		return err
	}

	return nil
}

func (is *ioServer) StartListContainers() error {
	conn, err := is.dial(is.pluginSocketPath, is.period)
	if err != nil {
		klog.ErrorS(err, "dial io plugin failed", "io plugin socket path", is.pluginSocketPath)
		goto unimplementedError
	}

	is.ioPluginClient = advisorsvc.NewQRMServiceClient(conn)

	err = retry.OnError(retry.DefaultRetry, func(err error) bool {
		return !general.IsUnimplementedError(err)
	}, func() error {
		if err := is.listContainers(); err != nil {
			_ = is.metaCache.ClearContainers()
			return err
		}
		return nil
	})
	if err == nil {
		return nil
	}

unimplementedError:
	go func() {
		wait.PollUntil(durationToWaitListAndWatchCalled, func() (done bool, err error) { return is.listAndWatchCalled, nil }, is.stopCh)

		// Is listContainer RPC is not implemented, we need to wait for the QRM to call addContainer to update the metaCache.
		// Actually, this does not guarantee that all the containers will be fully walked through.
		general.Infof("wait %v to get add container query", durationToWaitAddContainer.String())
		time.Sleep(durationToWaitAddContainer)
		is.sendCh <- types.TriggerInfo{TimeStamp: time.Now()}
	}()
	return nil
}

func (is *ioServer) listContainers() error {
	resp, err := is.ioPluginClient.ListContainers(context.TODO(), &advisorsvc.Empty{})
	if err != nil {
		return err
	}
	for _, container := range resp.Containers {
		if err := is.addContainer(container); err != nil {
			general.ErrorS(err, "add container failed", "podUID", container.PodUid, "containerName", container.ContainerName)
			return err
		}
		general.InfoS("add container", "container", container.String())
	}
	go func() {
		is.sendCh <- types.TriggerInfo{TimeStamp: time.Now()}
	}()
	return nil
}

func (is *ioServer) ListAndWatch(_ *advisorsvc.Empty, server advisorsvc.AdvisorService_ListAndWatchServer) error {
	_ = is.emitter.StoreInt64(is.genMetricsName(metricServerLWCalled), int64(is.period.Seconds()), metrics.MetricTypeNameCount)

	recvCh, ok := is.recvCh.(chan types.InternalIOCalculationResult)
	if !ok {
		return fmt.Errorf("recvCh convert failed")
	}
	is.listAndWatchCalled = true

	for {
		select {
		case <-server.Context().Done():
			klog.Infof("[qosaware-server-io] lw stream server exited")
			return nil
		case <-is.stopCh:
			klog.Infof("[qosaware-server-io] lw stopped because %v stopped", is.name)
			return nil
		case advisorResp, more := <-recvCh:
			if !more {
				klog.Infof("[qosaware-server-io] %v recv channel is closed", is.name)
				return nil
			}
			if advisorResp.TimeStamp.Add(is.period * 2).Before(time.Now()) {
				klog.Warningf("[qosaware-server-io] advisorResp is expired")
				continue
			}
			resp := is.assembleResponse(&advisorResp)
			if resp != nil {
				if err := server.Send(resp); err != nil {
					klog.Warningf("[qosaware-server-io] send response failed: %v", err)
					_ = is.emitter.StoreInt64(is.genMetricsName(metricServerLWSendResponseFailed), int64(is.period.Seconds()), metrics.MetricTypeNameCount)
					return err
				}

				klog.Infof("[qosaware-server-io] send calculation result: %v", general.ToString(resp))
				_ = is.emitter.StoreInt64(is.genMetricsName(metricServerLWSendResponseSucceeded), int64(is.period.Seconds()), metrics.MetricTypeNameCount)
			}
		}
	}
}

func (is *ioServer) assembleResponse(result *types.InternalIOCalculationResult) *advisorsvc.ListAndWatchResponse {
	resp := advisorsvc.ListAndWatchResponse{
		PodEntries:   make(map[string]*advisorsvc.CalculationEntries),
		ExtraEntries: make([]*advisorsvc.CalculationInfo, 0),
	}
	if result == nil {
		return nil
	}
	/*
		for _, advice := range result.ContainerEntries {
			podEntry, ok := resp.PodEntries[advice.PodUID]
			if !ok {
				podEntry = &advisorsvc.CalculationEntries{
					ContainerEntries: map[string]*advisorsvc.CalculationInfo{},
				}
				resp.PodEntries[advice.PodUID] = podEntry
			}
			calculationInfo, ok := podEntry.ContainerEntries[advice.ContainerName]
			if !ok {
				calculationInfo = &advisorsvc.CalculationInfo{
					CalculationResult: &advisorsvc.CalculationResult{
						Values: make(map[string]string),
					},
				}
				podEntry.ContainerEntries[advice.ContainerName] = calculationInfo
			}
			for k, v := range advice.Values {
				calculationInfo.CalculationResult.Values[k] = v
			}
		}
	*/
	for _, advice := range result.ExtraEntries {
		found := false
		for _, entry := range resp.ExtraEntries {
			if advice.CgroupPath == entry.CgroupPath {
				found = true
				for k, v := range advice.Values {
					entry.CalculationResult.Values[k] = v
				}
				break
			}
		}
		if !found {
			calculationInfo := &advisorsvc.CalculationInfo{
				CgroupPath: advice.CgroupPath,
				CalculationResult: &advisorsvc.CalculationResult{
					Values: general.DeepCopyMap(advice.Values),
				},
			}
			resp.ExtraEntries = append(resp.ExtraEntries, calculationInfo)
		}
	}

	return &resp
}
