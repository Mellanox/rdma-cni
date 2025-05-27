// Copyright 2025 NVIDIA CORPORATION & AFFILIATES
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package types

import (
	"github.com/containernetworking/cni/pkg/types"
)

type RdmaNetConf struct {
	types.NetConf
	DeviceID string  `json:"deviceID"` // PCI address of a VF in valid sysfs format
	Args     CNIArgs `json:"args"`     // optional arguments passed to CNI as defined in CNI spec 0.2.0
}

type CNIArgs struct {
	CNI RdmaCNIArgs `json:"cni"`
}

type RdmaCNIArgs struct {
	types.CommonArgs
	Debug bool `json:"debug"` // Run CNI in debug mode
}

// RDMA Network state struct version
// minor should be bumped when new fields are added
// major should be bumped when non backward compatible changes are introduced
const RdmaNetStateVersion = "1.0"

func NewRdmaNetState() RdmaNetState {
	return RdmaNetState{Version: RdmaNetStateVersion}
}

type RdmaNetState struct {
	// RDMA network state struct version
	Version string `json:"version"`
	// PCI device ID associated with the RDMA device
	DeviceID string `json:"deviceID"`
	// RDMA device name as originally appeared in sandbox
	SandboxRdmaDevName string `json:"sandboxRdmaDevName"`
	// RDMA device name in container
	ContainerRdmaDevName string `json:"containerRdmaDevName"`
}
