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

package rdma

import (
	"github.com/Mellanox/rdmamap"
	"github.com/vishvananda/netlink"
)

// Interface to be used by RDMA manager for basic operations
type BasicOps interface {
	// Equivalent to netlink.RdmaLinkByName(...)
	RdmaLinkByName(name string) (*netlink.RdmaLink, error)
	// Equivalent to netlink.RdmaLinkSetNsFd(...)
	RdmaLinkSetNsFd(link *netlink.RdmaLink, fd uint32) error
	// Equivalent to netlink.RdmaSystemGetNetnsMode(...)
	RdmaSystemGetNetnsMode() (string, error)
	// Equivalent to netlink.RdmaSystemSetNetnsMode(...)
	RdmaSystemSetNetnsMode(newMode string) error
	// Equivalent to rdmamap.GetRdmaDevicesForPcidev(...)
	GetRdmaDevicesForPcidev(pcidevName string) []string
}

func newRdmaBasicOps() BasicOps {
	return &rdmaBasicOpsImpl{}
}

type rdmaBasicOpsImpl struct {
}

// Equivalent to netlink.RdmaLinkByName(...)
func (rdma *rdmaBasicOpsImpl) RdmaLinkByName(name string) (*netlink.RdmaLink, error) {
	return netlink.RdmaLinkByName(name)
}

// Equivalent to netlink.RdmaLinkSetNsFd(...)
func (rdma *rdmaBasicOpsImpl) RdmaLinkSetNsFd(link *netlink.RdmaLink, fd uint32) error {
	return netlink.RdmaLinkSetNsFd(link, fd)
}

// Equivalent to netlink.RdmaSystemGetNetnsMode(...)
func (rdma *rdmaBasicOpsImpl) RdmaSystemGetNetnsMode() (string, error) {
	return netlink.RdmaSystemGetNetnsMode()
}

// Equivalent to netlink.RdmaSystemSetNetnsMode(...)
func (rdma *rdmaBasicOpsImpl) RdmaSystemSetNetnsMode(newMode string) error {
	return netlink.RdmaSystemSetNetnsMode(newMode)
}

// Equivalent to rdmamap.GetRdmaDevicesForPcidev(...)
func (rdma *rdmaBasicOpsImpl) GetRdmaDevicesForPcidev(pcidevName string) []string {
	return rdmamap.GetRdmaDevicesForPcidev(pcidevName)
}
