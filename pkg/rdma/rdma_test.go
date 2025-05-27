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
	"fmt"

	"github.com/containernetworking/plugins/pkg/ns"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"github.com/vishvananda/netlink"

	"github.com/k8snetworkplumbingwg/rdma-cni/pkg/rdma/mocks"
)

type dummyNetNs struct {
	ns.NetNS
	fd uintptr
}

func (dns *dummyNetNs) Fd() uintptr {
	return dns.fd
}

var _ = Describe("Rdma Manager", func() {
	var (
		rdmaManager Manager
		rdmaOpsMock mocks.RdmaBasicOps
		t           GinkgoTInterface
	)

	JustBeforeEach(func() {
		rdmaOpsMock = mocks.RdmaBasicOps{}
		rdmaManager = &rdmaManagerNetlink{rdmaOps: &rdmaOpsMock}
		t = GinkgoT()
	})

	Describe("Test GetRdmaDevsForPciDev()", func() {
		Context("Basic Call, no failure from RdmaBasicOps", func() {
			It("Should pass and return value as provided by rdmaBasicOps", func() {
				retVal := []string{"mlx5_3"}
				rdmaOpsMock.On("GetRdmaDevicesForPcidev", mock.AnythingOfType("string")).Return(retVal, nil)
				ret, err := rdmaManager.GetRdmaDevsForPciDev("04:00.1")
				rdmaOpsMock.AssertExpectations(t)
				Expect(err).ToNot(HaveOccurred())
				Expect(ret).To(Equal(retVal))
			})
		})
		Context("no RDMA devices for PCI dev returned from RdmaBasicOps", func() {
			It("Should return the same", func() {
				retVal := []string{}
				rdmaOpsMock.On("GetRdmaDevicesForPcidev", mock.AnythingOfType("string")).Return(retVal, nil)
				ret, err := rdmaManager.GetRdmaDevsForPciDev("04:00.1")
				rdmaOpsMock.AssertExpectations(t)
				Expect(err).ToNot(HaveOccurred())
				Expect(ret).To(Equal(retVal))
			})
		})
	})

	Describe("Test GetSystemRdmaMode()", func() {
		Context("Basic Call - no error", func() {
			It("Is a Proxy for RdmaBasicOps.GetSystemRdmaMode", func() {
				retVal := RdmaSysModeExclusive
				rdmaOpsMock.On("RdmaSystemGetNetnsMode").Return(retVal, nil)
				ret, err := rdmaManager.GetSystemRdmaMode()
				rdmaOpsMock.AssertExpectations(t)
				Expect(err).ToNot(HaveOccurred())
				Expect(ret).To(Equal(retVal))
			})
		})
		Context("Basic Call - with error", func() {
			It("Is a Proxy for RdmaBasicOps.GetSystemRdmaMode", func() {
				retErr := fmt.Errorf("some Error")
				retVal := "dummy"
				rdmaOpsMock.On("RdmaSystemGetNetnsMode").Return(retVal, retErr)
				ret, err := rdmaManager.GetSystemRdmaMode()
				rdmaOpsMock.AssertExpectations(t)
				Expect(err).To(Equal(retErr))
				Expect(ret).To(Equal(retVal))
			})
		})
	})

	Describe("Test SetSystemRdmaMode()", func() {
		Context("Basic Call - no error", func() {
			It("Is a Proxy for RdmaBasicOps.SetSystemRdmaMode", func() {
				mode := RdmaSysModeExclusive
				rdmaOpsMock.On("RdmaSystemSetNetnsMode", mode).Return(nil)
				err := rdmaManager.SetSystemRdmaMode(mode)
				rdmaOpsMock.AssertExpectations(t)
				Expect(err).ToNot(HaveOccurred())
			})
		})
		Context("Basic Call - with error", func() {
			It("Is a Proxy for RdmaBasicOps.SetSystemRdmaMode", func() {
				retErr := fmt.Errorf("some Error")
				mode := "dummy"
				rdmaOpsMock.On("RdmaSystemSetNetnsMode", mode).Return(retErr)
				err := rdmaManager.SetSystemRdmaMode(mode)
				rdmaOpsMock.AssertExpectations(t)
				Expect(err).To(Equal(retErr))
			})
		})
	})

	Describe("Test MoveRdmaDevToNs()", func() {
		Context("Basic Call - no error", func() {
			It("Calls rdmaOps.RdmaLinkSetNsFd with the correct netNS file desc and the rdma Link index", func() {
				link := &netlink.RdmaLink{}
				netNs := &dummyNetNs{fd: 17}
				rdmaOpsMock.On("RdmaLinkByName", mock.AnythingOfType("string")).Return(link, nil)
				rdmaOpsMock.On("RdmaLinkSetNsFd", link, uint32(netNs.Fd())).Return(nil)
				err := rdmaManager.MoveRdmaDevToNs("mlx5_9", netNs)
				rdmaOpsMock.AssertExpectations(t)
				Expect(err).ToNot(HaveOccurred())
			})
		})
		Context("Basic Call - with error", func() {
			It("returns error in case rdma link cannot be retrieved", func() {
				netNs := &dummyNetNs{fd: 17}
				rdmaOpsMock.On("RdmaLinkByName", mock.AnythingOfType("string")).Return(nil, fmt.Errorf("error"))
				err := rdmaManager.MoveRdmaDevToNs("mlx5_9", netNs)
				rdmaOpsMock.AssertExpectations(t)
				Expect(err).To(HaveOccurred())
			})
			It("returns error in case rdma link fails to move to namespace", func() {
				link := &netlink.RdmaLink{}
				netNs := &dummyNetNs{fd: 17}
				rdmaOpsMock.On("RdmaLinkByName", mock.AnythingOfType("string")).Return(link, nil)
				rdmaOpsMock.On("RdmaLinkSetNsFd", link, uint32(netNs.Fd())).Return(fmt.Errorf("error"))
				err := rdmaManager.MoveRdmaDevToNs("mlx5_9", netNs)
				rdmaOpsMock.AssertExpectations(t)
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
