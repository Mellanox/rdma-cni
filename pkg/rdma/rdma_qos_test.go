// Copyright 2026 NVIDIA CORPORATION & AFFILIATES
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
	"path"
	"strconv"

	"github.com/containernetworking/plugins/pkg/ns"
	rdmatypes "github.com/k8snetworkplumbingwg/rdma-cni/pkg/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/afero"
)

var _ = Describe("RdmaQoSManager", func() {
	var (
		qosManager QoSManager
		//t          GinkgoTInterface
	)

	JustBeforeEach(func() {
		qosManager = &rdmaQoSManager{
			qosConf: rdmatypes.RDMAQoS{TOS: 99, TC: 11},
			ops:     newFakerdmaQoSManagerOps(rdmatypes.RDMAQoS{TOS: 96, TC: 11}),
		}
	})

	Describe("parseUint32", func() {
		It("treats empty or whitespace-only input as 0", func() {
			val, err := parseUint32([]byte(""))
			Expect(err).ToNot(HaveOccurred())
			Expect(val).To(Equal(uint32(0)))
			val, err = parseUint32([]byte("\n"))
			Expect(err).ToNot(HaveOccurred())
			Expect(val).To(Equal(uint32(0)))
			val, err = parseUint32([]byte("   \t\n"))
			Expect(err).ToNot(HaveOccurred())
			Expect(val).To(Equal(uint32(0)))
		})
		It("parses valid numbers", func() {
			val, err := parseUint32([]byte("96\n"))
			Expect(err).ToNot(HaveOccurred())
			Expect(val).To(Equal(uint32(96)))
		})
	})

	Describe("Test GetRdmaDevQoS()", func() {
		Context("Basic Call, no failure from RdmaBasicOps", func() {
			It("Should pass and return value as provided by rdmaBasicOps", func() {
				retVal := rdmatypes.RDMAQoS{TOS: 96, TC: 11}
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

type fakerdmaQoSManagerOps struct {
	fakefs afero.Afero
	qos    rdmatypes.RDMAQoS
}

func newFakerdmaQoSManagerOps(qos rdmatypes.RDMAQoS) rdmaQoSManagerOps {
	return &fakerdmaQoSManagerOps{fakefs: afero.Afero{Fs: afero.NewMemMapFs()}, qos: qos}
}

func (fqm *fakerdmaQoSManagerOps) getRdmaDevQoSFormSysfs(rdmaDev string) (rdmatypes.RDMAQoS, error) {
	rdmaDevQoSPath := path.Join(rdmaCMConfigfsPath, rdmaDev, "ports", "1")
	err := fqm.fakefs.MkdirAll(rdmaDevQoSPath, 0755)
	if err != nil {
		return rdmatypes.RDMAQoS{}, fmt.Errorf("failed to create directory %s: %w", rdmaDevQoSPath, err)
	}

	tosPath := path.Join(rdmaDevQoSPath, "default_roce_tos")
	fqm.fakefs.WriteFile(tosPath, []byte(strconv.Itoa(int(fqm.qos.TOS))+"\n"), 0644)

	tos, err := fqm.fakefs.ReadFile(tosPath)
	if err != nil {
		return rdmatypes.RDMAQoS{}, err
	}
	tosVal, err := parseUint32(tos)
	if err != nil {
		return rdmatypes.RDMAQoS{}, err
	}
	tcPath := path.Join(rdmaDevQoSPath, "tc", "1", "traffic_class")
	fqm.fakefs.WriteFile(tcPath, []byte(strconv.Itoa(int(fqm.qos.TC))+"\n"), 0644)

	tc, err := fqm.fakefs.ReadFile(tcPath)
	if err != nil {
		return rdmatypes.RDMAQoS{}, err
	}
	tcVal, err := parseUint32(tc)
	if err != nil {
		return rdmatypes.RDMAQoS{}, err
	}

	return rdmatypes.RDMAQoS{TOS: tosVal, TC: tcVal}, nil
}

func (fqm *fakerdmaQoSManagerOps) setRdmaDevQoSToSysfs(targetNs ns.NetNS, rdmaDev string, qos rdmatypes.RDMAQoS) error {
	rdmaDevQoSPath := path.Join(rdmaCMConfigfsPath, rdmaDev)
	err := fqm.fakefs.MkdirAll(rdmaDevQoSPath, 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory %s: %w", rdmaDevQoSPath, err)
	}
	err = fqm.fakefs.WriteFile(path.Join(rdmaDevQoSPath, "ports", "1", "default_roce_tos"), []byte(strconv.Itoa(int(qos.TOS))), 0644)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", path.Join(rdmaDevQoSPath, "ports", "1", "default_roce_tos"), err)
	}

	err = fqm.fakefs.WriteFile(path.Join(rdmaDevQoSPath, "tc", "1", "traffic_class"), []byte(strconv.Itoa(int(qos.TC))), 0644)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", path.Join(rdmaDevQoSPath, "tc", "1", "traffic_class"), err)
	}

	return nil
}
