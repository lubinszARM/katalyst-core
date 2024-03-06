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

package iocost

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kubewharf/katalyst-core/pkg/util/general"
)

func TestGetDevicesIdToModel(t *testing.T) {
	allDeviceNames, _ := getAllDeviceNames()

	type args struct {
		deviceNames []string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{

			name: "test getDevicesIdToModel with allDeviceNames",
			args: args{
				deviceNames: allDeviceNames,
			},
			wantErr: false,
		},
		{

			name: "test getDevicesIdToModel with fake device names",
			args: args{
				deviceNames: []string{"fake"},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getDevicesIdToModel(tt.args.deviceNames)
			if (err != nil) != tt.wantErr {
				t.Errorf("getDevicesIdToModel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestLoadJsonConfig(t *testing.T) {
	type args struct {
		configAbsPath string
		configObject  interface{}
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "test LoadJsonConfig",
			args: args{
				configAbsPath: "fakePath",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := general.LoadJsonConfig(tt.args.configAbsPath, tt.args.configObject); (err != nil) != tt.wantErr {
				t.Errorf("LoadJsonConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_getContainerdRootDir(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "test getContainerdRootDir"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getContainerdRootDir()
		})
	}
}

func TestGetDeviceNameFromID(t *testing.T) {
	targetDevID := "1234"
	_, found, err := getDeviceNameFromID(targetDevID)

	assert.NoError(t, err)
	assert.False(t, found)
}
