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

package metacache

import (
	"fmt"
	"sync"

	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager"
	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager/errors"

	"github.com/kubewharf/katalyst-core/pkg/agent/sysadvisor/types"
	"github.com/kubewharf/katalyst-core/pkg/config"
	"github.com/kubewharf/katalyst-core/pkg/metaserver/agent/metric"
	"github.com/kubewharf/katalyst-core/pkg/util/machine"
)

// [notice]
// to compatible with checkpoint checksum calculation,
// we should make guarantees below in checkpoint properties assignment
// 1. resource.Quantity use resource.MustParse("0") to initialize, not to use resource.Quantity{}
// 2. CPUSet use NewCPUSet(...) to initialize, not to use CPUSet{}
// 3. not use omitempty in map property and must make new map to do initialization

const (
	stateFileName string = "sys_advisor_state"
)

// MetaReader provides a standard interface to refer to metadata type
type MetaReader interface {
	// functions to get raw metadata (generated by other agents)

	GetContainerEntries(podUID string) (types.ContainerEntries, bool)
	GetContainerInfo(podUID string, containerName string) (*types.ContainerInfo, bool)
	GetContainerMetric(podUID string, containerName string, metricName string) (float64, error)
	RangeContainer(f func(podUID string, containerName string, containerInfo *types.ContainerInfo) bool)

	// functions to get advised metadata (generated by sysadvisor)

	GetPoolInfo(poolName string) (*types.PoolInfo, bool)
	GetPoolSize(poolName string) (int, bool)
}

// RawMetaWriter provides a standard interface to modify raw metadata (generated by other agents) in local cache
type RawMetaWriter interface {
	AddContainer(podUID string, containerName string, containerInfo *types.ContainerInfo) error
	SetContainerInfo(podUID string, containerName string, containerInfo *types.ContainerInfo) error
	RangeAndUpdateContainer(f func(podUID string, containerName string, containerInfo *types.ContainerInfo) bool)

	DeleteContainer(podUID string, containerName string) error
	RemovePod(podUID string) error
}

// AdvisorMetaWriter provides a standard interface to modify advised metadata (generated by sysadvisor)
type AdvisorMetaWriter interface {
	DeletePool(poolName string) error
	GCPoolEntries(livingPoolNameSet map[string]struct{}) error
	SetPoolInfo(poolName string, poolInfo *types.PoolInfo) error
}

type MetaCache interface {
	MetaReader
	RawMetaWriter
	AdvisorMetaWriter
}

// MetaCacheImp stores metadata and info of pod, node, pool, subnuma etc. as a cache,
// and synchronizes data to sysadvisor state file. It is thread-safe to read and write.
// Deep copy logic is performed during accessing metacache entries instead of directly
// return pointer of each struct to avoid mis-overwrite.
type MetaCacheImp struct {
	podEntries  types.PodEntries
	poolEntries types.PoolEntries
	mutex       sync.RWMutex

	checkpointManager checkpointmanager.CheckpointManager
	checkpointName    string

	metricsFetcher metric.MetricsFetcher
}

var _ MetaCache = &MetaCacheImp{}

// NewMetaCacheImp returns the single instance of MetaCacheImp
func NewMetaCacheImp(conf *config.Configuration, metricsFetcher metric.MetricsFetcher) (*MetaCacheImp, error) {
	stateFileDir := conf.GenericSysAdvisorConfiguration.StateFileDirectory
	checkpointManager, err := checkpointmanager.NewCheckpointManager(stateFileDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize checkpoint manager: %v", err)
	}

	mc := &MetaCacheImp{
		podEntries:        make(types.PodEntries),
		poolEntries:       make(types.PoolEntries),
		checkpointManager: checkpointManager,
		checkpointName:    stateFileName,
		metricsFetcher:    metricsFetcher,
	}

	// Restore from checkpoint before any function call to metacache api
	if err := mc.restoreState(); err != nil {
		return mc, err
	}

	return mc, nil
}

/*
	standard implementation for MetaReader
*/

// GetContainerEntries returns a ContainerEntry copy keyed by pod uid
func (mc *MetaCacheImp) GetContainerEntries(podUID string) (types.ContainerEntries, bool) {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	v, ok := mc.podEntries[podUID]
	return v.Clone(), ok
}

// GetContainerInfo returns a ContainerInfo copy keyed by pod uid and container name
func (mc *MetaCacheImp) GetContainerInfo(podUID string, containerName string) (*types.ContainerInfo, bool) {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	podInfo, ok := mc.podEntries[podUID]
	if !ok {
		return nil, false
	}
	containerInfo, ok := podInfo[containerName]

	return containerInfo.Clone(), ok
}

// RangeContainer applies a function to every podUID, containerName, containerInfo set.
// Deep copy logic is applied so that pod and container entries will not be overwritten.
func (mc *MetaCacheImp) RangeContainer(f func(podUID string, containerName string, containerInfo *types.ContainerInfo) bool) {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	for podUID, podInfo := range mc.podEntries.Clone() {
		for containerName, containerInfo := range podInfo {
			if !f(podUID, containerName, containerInfo) {
				break
			}
		}
	}
}

// GetContainerMetric returns the metric value of a container
func (mc *MetaCacheImp) GetContainerMetric(podUID string, containerName string, metricName string) (float64, error) {
	return mc.metricsFetcher.GetContainerMetric(podUID, containerName, metricName)
}

// GetPoolInfo returns a PoolInfo copy by pool name
func (mc *MetaCacheImp) GetPoolInfo(poolName string) (*types.PoolInfo, bool) {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	poolInfo, ok := mc.poolEntries[poolName]
	return poolInfo.Clone(), ok
}

// GetPoolSize returns the size of pool as integer
func (mc *MetaCacheImp) GetPoolSize(poolName string) (int, bool) {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	pi, ok := mc.poolEntries[poolName]
	if !ok {
		return 0, false
	}
	return machine.CountCPUAssignmentCPUs(pi.TopologyAwareAssignments), true
}

/*
	standard implementation for RawMetaWriter
*/

// AddContainer adds a container keyed by pod uid and container name. For repeatedly added
// container, only mutable metadata will be updated, i.e. request quantity changed by vpa
func (mc *MetaCacheImp) AddContainer(podUID string, containerName string, containerInfo *types.ContainerInfo) error {
	if podInfo, ok := mc.podEntries[podUID]; ok {
		if ci, ok := podInfo[containerName]; ok {
			ci.UpdateMeta(containerInfo)
			return nil
		}
	}
	return mc.SetContainerInfo(podUID, containerName, containerInfo)
}

// SetContainerInfo updates ContainerInfo keyed by pod uid and container name
func (mc *MetaCacheImp) SetContainerInfo(podUID string, containerName string, containerInfo *types.ContainerInfo) error {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	podInfo, ok := mc.podEntries[podUID]
	if !ok {
		mc.podEntries[podUID] = make(types.ContainerEntries)
		podInfo = mc.podEntries[podUID]
	}
	podInfo[containerName] = containerInfo

	return mc.storeState()
}

// DeleteContainer deletes a ContainerInfo keyed by pod uid and container name
func (mc *MetaCacheImp) DeleteContainer(podUID string, containerName string) error {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	podInfo, ok := mc.podEntries[podUID]
	if !ok {
		return nil
	}
	_, ok = podInfo[containerName]
	if !ok {
		return nil
	}
	delete(podInfo, containerName)

	return mc.storeState()
}

// RangeAndUpdateContainer applies a function to every podUID, containerName, containerInfo set.
// Not recommended to use if RangeContainer satisfies the requirement.
func (mc *MetaCacheImp) RangeAndUpdateContainer(f func(podUID string, containerName string, containerInfo *types.ContainerInfo) bool) {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	for podUID, podInfo := range mc.podEntries {
		for containerName, containerInfo := range podInfo {
			if !f(podUID, containerName, containerInfo) {
				break
			}
		}
	}
}

// RemovePod deletes a PodInfo keyed by pod uid. Repeatedly remove will be ignored.
func (mc *MetaCacheImp) RemovePod(podUID string) error {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	_, ok := mc.podEntries[podUID]
	if !ok {
		return nil
	}
	delete(mc.podEntries, podUID)

	return mc.storeState()
}

/*
	standard implementation for AdvisorMetaWriter
*/

// SetPoolInfo stores a PoolInfo by pool name
func (mc *MetaCacheImp) SetPoolInfo(poolName string, poolInfo *types.PoolInfo) error {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	mc.poolEntries[poolName] = poolInfo

	return mc.storeState()
}

// DeletePool deletes a PoolInfo keyed by pool name
func (mc *MetaCacheImp) DeletePool(poolName string) error {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	delete(mc.poolEntries, poolName)

	return mc.storeState()
}

// GCPoolEntries deletes GCPoolEntries not existing on node
func (mc *MetaCacheImp) GCPoolEntries(livingPoolNameSet map[string]struct{}) error {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	for poolName := range mc.poolEntries {
		if _, ok := livingPoolNameSet[poolName]; !ok {
			delete(mc.poolEntries, poolName)
		}
	}

	return mc.storeState()
}

/*
	other helper functions
*/

func (mc *MetaCacheImp) storeState() error {
	checkpoint := NewMetaCacheCheckpoint()
	checkpoint.PodEntries = mc.podEntries
	checkpoint.PoolEntries = mc.poolEntries

	if err := mc.checkpointManager.CreateCheckpoint(mc.checkpointName, checkpoint); err != nil {
		klog.Errorf("[metacache] store state failed: %v", err)
		return err
	}
	klog.Infof("[metacache] store state succeeded")

	return nil
}

func (mc *MetaCacheImp) restoreState() error {
	checkpoint := NewMetaCacheCheckpoint()

	if err := mc.checkpointManager.GetCheckpoint(mc.checkpointName, checkpoint); err != nil {
		if err == errors.ErrCheckpointNotFound {
			klog.Infof("[metacache] checkpoint %v not found, create", mc.checkpointName)
			return mc.storeState()
		}
		klog.Errorf("[metacache] restore state failed: %v", err)
		return err
	}

	mc.podEntries = checkpoint.PodEntries
	mc.poolEntries = checkpoint.PoolEntries

	klog.Infof("[metacache] restore state succeeded")

	return nil
}
