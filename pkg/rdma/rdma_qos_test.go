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
	rdmatypes "github.com/k8snetworkplumbingwg/rdma-cni/pkg/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RdmaQoSManager", func() {
	var (
		qosManager QoSManager
		//t          GinkgoTInterface
	)

	JustBeforeEach(func() {
		qosManager = &rdmaQoSManager{
			qosConf: rdmatypes.RDMAQoS{TOS: 0, TC: 0},
			ops:     newFakeRdmaQoSManagerOps(rdmatypes.RDMAQoS{TOS: 96, TC: 0}),
		}
	})

	Describe("Test GetRdmaDevQoS()", func() {
		Context("Basic Call, no failure from RdmaBasicOps", func() {
			It("Should pass and return value as provided by rdmaBasicOps", func() {
				retVal := rdmatypes.RDMAQoS{TOS: 96, TC: 0}
				qos, err := qosManager.GetRdmaDevQoS("mlx5_2")
				Expect(err).ToNot(HaveOccurred())
				Expect(qos).To(Equal(retVal))
			})
		})
	})
	Describe("Test SetRdmaDevQoS()", func() {
		Context("Basic Call, no failure from RdmaBasicOps", func() {
			It("Should pass and return value as provided by rdmaBasicOps", func() {
				netNs := &dummyNetNs{fd: 88}
				err := qosManager.SetRdmaDevQoS(netNs, "mlx5_6", rdmatypes.RDMAQoS{TOS: 0, TC: 0})
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
