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

package ioweight

import (
	"fmt"

	v1 "k8s.io/api/core/v1"

	"github.com/kubewharf/katalyst-core/pkg/config/generic"
	"github.com/kubewharf/katalyst-core/pkg/util/native"
)

func GetQoS(qosConfig *generic.QoSConfiguration, pod *v1.Pod) {
	qosLevel, err := qosConfig.GetQoSLevelForPod(pod)
	if err != nil {
		fmt.Printf("BBLU111 failed:%v..\n", err)
		return
	}

	for _, containerStatus := range pod.Status.ContainerStatuses {
		podUID, containerID := string(pod.UID), native.TrimContainerIDPrefix(containerStatus.ContainerID)
		fmt.Printf("BBLU2223 got qos:%v, pod=%v, containerid=%v\n", qosLevel, podUID, containerID)
	}
	return
}
