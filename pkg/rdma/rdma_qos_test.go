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
	"encoding/json"
	"errors"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/afero"

	rdmatypes "github.com/k8snetworkplumbingwg/rdma-cni/pkg/types"
)

func qosTC(value uint32) *uint32 {
	return &value
}

var _ = Describe("RDMA QoS", func() {
	var (
		ops     *rdmaQoSManagerOpsIml
		manager *rdmaQoSManager
		tosPath string
		tcPath  string
	)

	BeforeEach(func() {
		root := GinkgoT().TempDir()
		ops = &rdmaQoSManagerOpsIml{
			fs:            afero.Afero{Fs: afero.NewBasePathFs(afero.NewOsFs(), root)},
			mountConfigFS: func() error { return nil },
		}
		manager = &rdmaQoSManager{qosConf: rdmatypes.RDMAQoS{}, ops: ops}
		tosPath = filepath.Join(rdmaCMConfigfsPath, "mlx5_2", "ports", "1", "default_roce_tos")
		tcPath = filepath.Join(rdmaSysfsPath, "mlx5_2", "tc", "1", "traffic_class")
		Expect(ops.fs.MkdirAll(filepath.Dir(tosPath), 0700)).To(Succeed())
		Expect(ops.fs.MkdirAll(filepath.Dir(tcPath), 0700)).To(Succeed())
		Expect(ops.fs.WriteFile(tosPath, []byte("55\n"), 0600)).To(Succeed())
		Expect(ops.fs.WriteFile(tcPath, []byte("Global tclass=11\n"), 0600)).To(Succeed())
	})

	DescribeTable("reading the global traffic-class override", func(contents string, expected *uint32) {
		Expect(ops.fs.WriteFile(tcPath, []byte(contents), 0600)).To(Succeed())
		host, _, err := manager.GetRdmaDevQoS("mlx5_2")
		Expect(err).NotTo(HaveOccurred())
		Expect(host).To(Equal(rdmatypes.RDMAQoS{TOS: 55, TC: expected}))
	},
		Entry("empty device setting is disabled", "", nil),
		Entry("whitespace is disabled", " \t\n", nil),
		Entry("an explicit global zero remains zero", "Global tclass=0\n", qosTC(0)),
		Entry("an explicit global class remains set", "Global tclass=41\n", qosTC(41)),
		Entry("rules alone do not become a global class", "src_ip=1.1.1.1,tclass=41\n", nil),
		Entry("a global class is independent of rules", "Global tclass=41\nsrc_ip=1.1.1.1,tclass=7\n", qosTC(41)),
	)

	DescribeTable("rejecting malformed global settings", func(contents string) {
		Expect(ops.fs.WriteFile(tcPath, []byte(contents), 0600)).To(Succeed())
		_, _, err := manager.GetRdmaDevQoS("mlx5_2")
		Expect(err).To(HaveOccurred())
	},
		Entry("missing number", "Global tclass=\n"),
		Entry("invalid number", "Global tclass=invalid\n"),
		Entry("negative number", "Global tclass=-1\n"),
		Entry("overflow", "Global tclass=4294967296\n"),
	)

	DescribeTable("resolving configured values", func(config, expected rdmatypes.RDMAQoS) {
		manager.LoadRdmaCniQoSConfig(config)
		host, qos, err := manager.GetRdmaDevQoS("mlx5_2")
		Expect(err).NotTo(HaveOccurred())
		Expect(host).To(Equal(rdmatypes.RDMAQoS{TOS: 55, TC: qosTC(11)}))
		Expect(qos).To(Equal(expected))
		var configuredTC uint32
		if config.TC != nil {
			configuredTC = *config.TC
		}
		*qos.TC = 99
		Expect(*host.TC).To(Equal(uint32(11)))
		Expect(manager.qosConf).To(Equal(config))
		if config.TC != nil {
			Expect(*config.TC).To(Equal(configuredTC))
		}
	},
		Entry("omitted QoS inherits the host", rdmatypes.RDMAQoS{}, rdmatypes.RDMAQoS{TOS: 55, TC: qosTC(11)}),
		Entry("zero values inherit the host", rdmatypes.RDMAQoS{TOS: 0, TC: qosTC(0)},
			rdmatypes.RDMAQoS{TOS: 55, TC: qosTC(11)}),
		Entry("TOS-only inherits the host TC", rdmatypes.RDMAQoS{TOS: 96, TC: nil},
			rdmatypes.RDMAQoS{TOS: 96, TC: qosTC(11)}),
		Entry("TC-only inherits the host TOS", rdmatypes.RDMAQoS{TOS: 0, TC: qosTC(41)},
			rdmatypes.RDMAQoS{TOS: 55, TC: qosTC(41)}),
		Entry("configured values override the host", rdmatypes.RDMAQoS{TOS: 96, TC: qosTC(41)},
			rdmatypes.RDMAQoS{TOS: 96, TC: qosTC(41)}),
	)

	It("owns the loaded configuration independently of the caller", func() {
		config := rdmatypes.RDMAQoS{TOS: 96, TC: qosTC(41)}
		manager.LoadRdmaCniQoSConfig(config)
		*config.TC = 99
		_, qos, err := manager.GetRdmaDevQoS("mlx5_2")
		Expect(err).NotTo(HaveOccurred())
		Expect(qos).To(Equal(rdmatypes.RDMAQoS{TOS: 96, TC: qosTC(41)}))
	})

	It("inherits a disabled global override without turning it into zero", func() {
		Expect(ops.fs.WriteFile(tcPath, nil, 0600)).To(Succeed())
		manager.LoadRdmaCniQoSConfig(rdmatypes.RDMAQoS{TOS: 96, TC: qosTC(0)})
		host, qos, err := manager.GetRdmaDevQoS("mlx5_2")
		Expect(err).NotTo(HaveOccurred())
		Expect(host.TC).To(BeNil())
		Expect(qos).To(Equal(rdmatypes.RDMAQoS{TOS: 96, TC: nil}))
	})

	It("supports devices without the optional traffic-class attribute", func() {
		Expect(ops.fs.RemoveAll(filepath.Dir(filepath.Dir(tcPath)))).To(Succeed())
		host, _, err := manager.GetRdmaDevQoS("mlx5_2")
		Expect(err).NotTo(HaveOccurred())
		Expect(host.TC).To(BeNil())
		Expect(manager.SetRdmaDevQoS(nil, "mlx5_2", host)).To(Succeed())
	})

	It("reports a failure to read the traffic-class snapshot", func() {
		Expect(ops.fs.Remove(tcPath)).To(Succeed())
		_, _, err := manager.GetRdmaDevQoS("mlx5_2")
		Expect(err).To(HaveOccurred())
	})

	DescribeTable("restoring traffic class in the host namespace", func(tc *uint32, expected string) {
		Expect(manager.SetRdmaDevQoS(nil, "mlx5_2", rdmatypes.RDMAQoS{TOS: 55, TC: tc})).To(Succeed())
		actual, err := ops.fs.ReadFile(tcPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(actual)).To(Equal(expected))
	},
		Entry("disabled global override uses the driver reset operation", nil, "-1"),
		Entry("global zero stays explicit", qosTC(0), "0"),
		Entry("global nonzero is restored", qosTC(41), "41"),
	)

	It("sets only TOS in the container namespace", func() {
		Expect(manager.SetRdmaDevQoS(&dummyNetNs{fd: 88}, "mlx5_2",
			rdmatypes.RDMAQoS{TOS: 96, TC: nil})).To(Succeed())
		tos, err := ops.fs.ReadFile(tosPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(tos)).To(Equal("96"))
		tc, err := ops.fs.ReadFile(tcPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(tc)).To(Equal("Global tclass=11\n"))
	})

	It("reports configfs mount failures before changing settings", func() {
		mountErr := errors.New("mount failed")
		ops.mountConfigFS = func() error { return mountErr }
		Expect(manager.SetRdmaDevQoS(&dummyNetNs{fd: 88}, "mlx5_2",
			rdmatypes.RDMAQoS{TOS: 96, TC: nil})).To(MatchError(ContainSubstring("mount failed")))
	})

	It("reports a TOS write failure before changing traffic class", func() {
		Expect(ops.fs.Remove(tosPath)).To(Succeed())
		Expect(ops.fs.Mkdir(tosPath, 0700)).To(Succeed())
		Expect(manager.SetRdmaDevQoS(nil, "mlx5_2", rdmatypes.RDMAQoS{TOS: 96, TC: qosTC(41)})).NotTo(Succeed())
		tc, err := ops.fs.ReadFile(tcPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(tc)).To(Equal("Global tclass=11\n"))
	})

	It("reports a traffic-class write failure", func() {
		Expect(ops.fs.Remove(tcPath)).To(Succeed())
		Expect(ops.fs.Mkdir(tcPath, 0700)).To(Succeed())
		Expect(manager.SetRdmaDevQoS(nil, "mlx5_2", rdmatypes.RDMAQoS{TOS: 55, TC: qosTC(41)})).NotTo(Succeed())
	})

	DescribeTable("parsing TOS", func(input string, expected uint32) {
		value, err := parseUint32([]byte(input))
		Expect(err).NotTo(HaveOccurred())
		Expect(value).To(Equal(expected))
	},
		Entry("empty", "", uint32(0)),
		Entry("whitespace", " \t\n", uint32(0)),
		Entry("numeric", "96\n", uint32(96)),
	)
})

var _ = Describe("RDMA QoS cache compatibility", func() {
	It("leaves the snapshot absent when reading a cache from before QoS support", func() {
		var state rdmatypes.RdmaNetState
		Expect(json.Unmarshal([]byte(`{"version":"1.0","deviceID":"0000:04:00.5"}`), &state)).To(Succeed())
		Expect(state.RDMAQoS).To(BeNil())
	})

	It("preserves a legacy numeric zero as a global override", func() {
		var state rdmatypes.RdmaNetState
		Expect(json.Unmarshal([]byte(`{"version":"1.0","rdmaQoS":{"tos":0,"tc":0}}`), &state)).To(Succeed())
		Expect(state.RDMAQoS).To(Equal(&rdmatypes.RDMAQoS{TOS: 0, TC: qosTC(0)}))
	})

	It("round trips a disabled override separately from a missing snapshot", func() {
		state := rdmatypes.NewRdmaNetState()
		state.RDMAQoS = &rdmatypes.RDMAQoS{TOS: 55, TC: nil}
		data, err := json.Marshal(state)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(`"tc":null`))
		var restored rdmatypes.RdmaNetState
		Expect(json.Unmarshal(data, &restored)).To(Succeed())
		Expect(restored.RDMAQoS).To(Equal(state.RDMAQoS))

		// Older plugins can still decode the wire format, although they cannot
		// distinguish disabled TC from their former numeric-zero default.
		var legacy struct {
			RDMAQoS struct {
				TOS uint32 `json:"tos"`
				TC  uint32 `json:"tc"`
			} `json:"rdmaQoS"`
		}
		Expect(json.Unmarshal(data, &legacy)).To(Succeed())
		Expect(legacy.RDMAQoS.TOS).To(Equal(uint32(55)))
		Expect(legacy.RDMAQoS.TC).To(BeZero())
	})
})
