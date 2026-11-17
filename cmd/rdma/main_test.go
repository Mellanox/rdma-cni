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

package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ns"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/k8snetworkplumbingwg/rdma-cni/pkg/cache"
	cacheMocks "github.com/k8snetworkplumbingwg/rdma-cni/pkg/cache/mocks"
	"github.com/k8snetworkplumbingwg/rdma-cni/pkg/rdma"
	rdmaMocks "github.com/k8snetworkplumbingwg/rdma-cni/pkg/rdma/mocks"
	rdmaTypes "github.com/k8snetworkplumbingwg/rdma-cni/pkg/types"
)

func tcValue(value uint32) *uint32 {
	return &value
}

func generateNetConfCmdDel(qos rdmaTypes.RDMAQoS) rdmaTypes.RdmaNetConf {
	return rdmaTypes.RdmaNetConf{
		NetConf: types.NetConf{
			CNIVersion:    "0.4.0",
			Name:          "rdma-net",
			Type:          "rdma",
			Capabilities:  nil,
			IPAM:          types.IPAM{},
			DNS:           types.DNS{},
			RawPrevResult: nil,
			PrevResult:    nil,
		},
		DeviceID: "",
		RDMAQoS:  qos,
	}
}

func generateNetConfCmdAdd(netName, cIfname, deviceID string, rdmaDevName string, qos rdmaTypes.RDMAQoS) rdmaTypes.RdmaNetConf {
	prevResult := current.Result{
		CNIVersion: "0.4.0",
		Interfaces: []*current.Interface{{
			Name:    cIfname,
			Mac:     "42:86:24:84:4f:b1",
			Sandbox: "/proc/1/ns/net",
		}},
		IPs:    nil,
		Routes: nil,
		DNS:    types.DNS{},
	}
	bytes, _ := json.Marshal(prevResult)
	var raw map[string]interface{}
	_ = json.Unmarshal(bytes, &raw)

	return rdmaTypes.RdmaNetConf{
		NetConf: types.NetConf{
			CNIVersion:    "0.4.0",
			Name:          netName,
			Type:          "rdma",
			Capabilities:  nil,
			IPAM:          types.IPAM{},
			DNS:           types.DNS{},
			RawPrevResult: raw,
			PrevResult:    nil,
		},
		DeviceID: deviceID,
		RDMAQoS:  qos,
		Args:     rdmaTypes.CNIArgs{CNI: rdmaTypes.RdmaCNIArgs{RDMADeviceName: rdmaDevName}},
	}
}

func generateArgs(nsPath, cid, cIfname string, netconf *rdmaTypes.RdmaNetConf) skel.CmdArgs {
	bytes, _ := json.Marshal(*netconf)
	return skel.CmdArgs{
		ContainerID: cid,
		Netns:       nsPath,
		IfName:      cIfname,
		Args:        "",
		Path:        "",
		StdinData:   bytes,
	}
}

func generateRdmaNetState(deviceID, sanboxRdmaDev, containerRdmaDev string, qos rdmaTypes.RDMAQoS) rdmaTypes.RdmaNetState {
	state := rdmaTypes.NewRdmaNetState()
	state.DeviceID = deviceID
	state.SandboxRdmaDevName = sanboxRdmaDev
	state.ContainerRdmaDevName = containerRdmaDev
	state.RDMAQoS = &qos
	return state
}

type dummyNetNs struct {
	fd   uintptr
	path string
}

func (dns *dummyNetNs) Fd() uintptr {
	return dns.fd
}

func (dns *dummyNetNs) Do(toRun func(ns.NetNS) error) error {
	return toRun(&dummyNetNs{fd: 19, path: "dummy/path"})
}

func (dns *dummyNetNs) Set() error {
	return nil
}

func (dns *dummyNetNs) Path() string {
	return dns.path
}

func (dns *dummyNetNs) Close() error {
	return nil
}

type dummyNsMananger struct {
	getNSErr error
}

func (nsm *dummyNsMananger) GetNS(nspath string) (ns.NetNS, error) {
	if nsm.getNSErr != nil {
		return nil, nsm.getNSErr
	}
	return &dummyNetNs{path: nspath, fd: 17}, nil
}

func (nsm *dummyNsMananger) GetCurrentNS() (ns.NetNS, error) {
	return &dummyNetNs{path: "/proc/2/ns/net", fd: 17}, nil
}

var _ = Describe("Main", func() {
	var (
		plugin             rdmaCniPlugin
		dummyNsMgr         dummyNsMananger
		rdmaMgrMock        rdmaMocks.MockManager
		rdmaQoSManagerMock rdmaMocks.MockQoSManager
		stateCacheMock     cacheMocks.MockStateCache
		t                  GinkgoTInterface
	)

	JustBeforeEach(func() {
		rdmaMgrMock = rdmaMocks.MockManager{}
		rdmaQoSManagerMock = rdmaMocks.MockQoSManager{}
		dummyNsMgr = dummyNsMananger{}
		stateCacheMock = cacheMocks.MockStateCache{}
		t = GinkgoT()
		plugin = rdmaCniPlugin{
			rdmaManager: &rdmaMgrMock,
			qosManager:  &rdmaQoSManagerMock,
			stateCache:  &stateCacheMock,
			nsManager:   &dummyNsMgr,
		}
	})

	Describe("Test ensureRdmaSystemMode()", func() {

		Context("Bad flows", func() {
			It("Should error out if rdma system namespace mode is not exclusive", func() {
				rdmaMgrMock.On("GetSystemRdmaMode").Return(rdma.RdmaSysModeShared, nil)
				err := plugin.ensureRdmaSystemMode()
				Expect(err).To(HaveOccurred())
				rdmaMgrMock.AssertExpectations(t)
			})
			It("Should error out on failure to get rdma system namespace mode", func() {
				retErr := fmt.Errorf("error")
				rdmaMgrMock.On("GetSystemRdmaMode").Return("", retErr)
				err := plugin.ensureRdmaSystemMode()
				Expect(err).To(HaveOccurred())
				rdmaMgrMock.AssertExpectations(t)
			})
		})
		Context("Good flow", func() {
			It("Should succeed if rdma system namespace mode is exclusive", func() {
				rdmaMgrMock.On("GetSystemRdmaMode").Return(rdma.RdmaSysModeExclusive, nil)
				Expect(plugin.ensureRdmaSystemMode()).To(Succeed())
				rdmaMgrMock.AssertExpectations(t)
			})
		})
	})

	Describe("Test moveRdmaDevToNs()", func() {

		Context("Good flow", func() {
			It("Should succeed and move RDMA device to given namespace", func() {
				rdmaDev := "mlx5_5"
				nsPath := "/proc/666/ns/net"
				containerNs, _ := dummyNsMgr.GetNS(nsPath)
				rdmaMgrMock.On("MoveRdmaDevToNs", rdmaDev, containerNs).Return(nil)
				Expect(plugin.moveRdmaDevToNs(rdmaDev, nsPath)).To(Succeed())
				rdmaMgrMock.AssertExpectations(t)
			})
		})
		Context("Bad flow", func() {
			It("Should fail", func() {
				retErr := fmt.Errorf("error occurred")
				rdmaMgrMock.On(
					"MoveRdmaDevToNs",
					mock.AnythingOfType("string"),
					mock.AnythingOfType("*main.dummyNetNs")).Return(retErr)
				err := plugin.moveRdmaDevToNs("mlx5_5", "/proc/666/ns/net")
				Expect(err).To(HaveOccurred())
				rdmaMgrMock.AssertExpectations(t)
			})
		})
	})

	Describe("Test moveRdmaDevFromNs()", func() {

		Context("Good flow", func() {
			It("Should succeed and move RDMA device to current namespace", func() {
				rdmaDev := "mlx5_5"
				nsPath := "/proc/666/ns/net"
				currNs, _ := dummyNsMgr.GetCurrentNS()
				rdmaMgrMock.On("MoveRdmaDevToNs", rdmaDev, currNs).Return(nil)
				Expect(plugin.moveRdmaDevFromNs(rdmaDev, nsPath)).To(Succeed())
				rdmaMgrMock.AssertExpectations(t)
			})
		})
		Context("Bad flow", func() {
			It("Should fail", func() {
				retErr := fmt.Errorf("error occurred")
				rdmaMgrMock.On("MoveRdmaDevToNs",
					mock.AnythingOfType("string"),
					mock.AnythingOfType("*main.dummyNetNs")).Return(retErr)
				err := plugin.moveRdmaDevFromNs("mlx5_5", "/proc/666/ns/net")
				Expect(err).To(HaveOccurred())
				rdmaMgrMock.AssertExpectations(t)
			})
		})
	})

	Describe("Test CmdAdd()", func() {
		Context("Valid configuration provided", func() {
			It("Should succeed and move Rdma device associated with provided PCI DeviceID to Namespace", func() {
				pciDev := "0000:04:00.5"
				netName := "rdma-net"
				cIfname := "net1"
				origRdmaDev := "mlx5_4"
				cid := "a1b2c3d4e5f6"
				qos := rdmaTypes.RDMAQoS{TOS: 96, TC: tcValue(26)}
				hostQos := rdmaTypes.RDMAQoS{TOS: 0, TC: tcValue(0)}
				cnsPath := "/proc/12444/ns/net"
				cns, _ := dummyNsMgr.GetNS(cnsPath)
				netconf := generateNetConfCmdAdd(netName, cIfname, pciDev, "", qos)
				args := generateArgs(cnsPath, cid, cIfname, &netconf)
				rdmaMgrMock.On("GetSystemRdmaMode").Return(rdma.RdmaSysModeExclusive, nil)
				rdmaMgrMock.On("GetRdmaDevsForPciDev", pciDev).Return([]string{origRdmaDev}, nil)
				rdmaMgrMock.On("MoveRdmaDevToNs", origRdmaDev, cns).Return(nil)
				rdmaQoSManagerMock.On("LoadRdmaCniQoSConfig", qos).Return(nil)
				rdmaQoSManagerMock.On("GetRdmaDevQoS", origRdmaDev).Return(hostQos, qos, nil)
				rdmaQoSManagerMock.On("SetRdmaDevQoS", nil, origRdmaDev,
					rdmaTypes.RDMAQoS{TOS: hostQos.TOS, TC: qos.TC}).Return(nil)
				rdmaQoSManagerMock.On("SetRdmaDevQoS", cns, origRdmaDev, rdmaTypes.RDMAQoS{TOS: qos.TOS}).Return(nil)
				stateCacheMock.On("GetStateRef", netName, cid, cIfname).Return(cache.StateRef("some-ref"))
				expectedState := generateRdmaNetState(pciDev, origRdmaDev, origRdmaDev, hostQos)
				stateCacheMock.On("Save", mock.AnythingOfType("cache.StateRef"), &expectedState).Return(nil)
				err := plugin.CmdAdd(&args)
				Expect(err).ToNot(HaveOccurred())
				rdmaMgrMock.AssertExpectations(t)
				stateCacheMock.AssertExpectations(t)
				rdmaQoSManagerMock.AssertExpectations(t)
			})
			It("Should succeed and move Rdma device associated with auxiliary device DeviceID to Namespace", func() {
				auxDev := "mlx5_core.sf.6"
				netName := "rdma-net"
				cIfname := "net2"
				origRdmaDev := "mlx5_6"
				cid := "a6b5c4d3e2f1"
				qos := rdmaTypes.RDMAQoS{TOS: 102, TC: tcValue(10)}
				hostQos := rdmaTypes.RDMAQoS{TOS: 51, TC: tcValue(5)}
				cnsPath := "/proc/11142/ns/net"
				cns, _ := dummyNsMgr.GetNS(cnsPath)
				netconf := generateNetConfCmdAdd(netName, cIfname, auxDev, "", qos)
				args := generateArgs(cnsPath, cid, cIfname, &netconf)
				rdmaMgrMock.On("GetSystemRdmaMode").Return(rdma.RdmaSysModeExclusive, nil)
				rdmaMgrMock.On("GetRdmaDevsForAuxDev", auxDev).Return([]string{origRdmaDev}, nil)
				rdmaMgrMock.On("MoveRdmaDevToNs", origRdmaDev, cns).Return(nil)
				rdmaQoSManagerMock.On("LoadRdmaCniQoSConfig", qos).Return(nil)
				rdmaQoSManagerMock.On("GetRdmaDevQoS", origRdmaDev).Return(hostQos, qos, nil)
				rdmaQoSManagerMock.On("SetRdmaDevQoS", nil, origRdmaDev,
					rdmaTypes.RDMAQoS{TOS: hostQos.TOS, TC: qos.TC}).Return(nil)
				rdmaQoSManagerMock.On("SetRdmaDevQoS", cns, origRdmaDev, rdmaTypes.RDMAQoS{TOS: qos.TOS}).Return(nil)
				stateCacheMock.On("GetStateRef", netName, cid, cIfname).Return(cache.StateRef("some-ref"))
				expectedState := generateRdmaNetState(auxDev, origRdmaDev, origRdmaDev, hostQos)
				stateCacheMock.On("Save", mock.AnythingOfType("cache.StateRef"), &expectedState).Return(nil)
				err := plugin.CmdAdd(&args)
				Expect(err).ToNot(HaveOccurred())
				rdmaMgrMock.AssertExpectations(t)
				stateCacheMock.AssertExpectations(t)
				rdmaQoSManagerMock.AssertExpectations(t)
			})
			It("Should succeed and move Rdma device associated with provided PCI DeviceID to Namespace with custom RDMA device name", func() {
				pciDev := "0000:04:00.5"
				netName := "rdma-net"
				cIfname := "net1"
				origRdmaDev := "mlx5_4"
				customRdmaDev := "custom-rdma"
				qos := rdmaTypes.RDMAQoS{TOS: 96, TC: tcValue(26)}
				cid := "a1b2c3d4e5f6"
				cnsPath := "/proc/12444/ns/net"
				cns, _ := dummyNsMgr.GetNS(cnsPath)
				netconf := generateNetConfCmdAdd(netName, cIfname, pciDev, customRdmaDev, qos)
				args := generateArgs(cnsPath, cid, cIfname, &netconf)
				rdmaMgrMock.On("GetSystemRdmaMode").Return(rdma.RdmaSysModeExclusive, nil)
				rdmaMgrMock.On("GetRdmaDevsForPciDev", pciDev).Return([]string{origRdmaDev}, nil)
				rdmaMgrMock.On("SetRdmaDevName", origRdmaDev, customRdmaDev).Return(nil)
				rdmaDev := netconf.Args.CNI.RDMADeviceName
				rdmaQoSManagerMock.On("LoadRdmaCniQoSConfig", qos).Return(nil)
				rdmaQoSManagerMock.On("GetRdmaDevQoS", origRdmaDev).Return(rdmaTypes.RDMAQoS{TOS: 0, TC: tcValue(0)}, qos, nil)
				rdmaQoSManagerMock.On("SetRdmaDevQoS", nil, origRdmaDev, rdmaTypes.RDMAQoS{TOS: 0, TC: qos.TC}).Return(nil)
				rdmaQoSManagerMock.On("SetRdmaDevQoS", cns, rdmaDev, rdmaTypes.RDMAQoS{TOS: qos.TOS}).Return(nil)
				rdmaMgrMock.On("MoveRdmaDevToNs", rdmaDev, cns).Return(nil)
				stateCacheMock.On("GetStateRef", netName, cid, cIfname).Return(cache.StateRef("some-ref"))
				expectedState := generateRdmaNetState(pciDev, origRdmaDev, rdmaDev, rdmaTypes.RDMAQoS{TOS: 0, TC: tcValue(0)})
				stateCacheMock.On("Save", mock.AnythingOfType("cache.StateRef"), &expectedState).Return(nil)
				err := plugin.CmdAdd(&args)
				Expect(err).ToNot(HaveOccurred())
				rdmaMgrMock.AssertExpectations(t)
				stateCacheMock.AssertExpectations(t)
				rdmaQoSManagerMock.AssertExpectations(t)
			})
		})
		// TODO(adrian): Add additional tests to cover bad flows / differen network configurations
	})

	Describe("Test CmdDel()", func() {
		Context("Valid configuration provided", func() {
			It("Should succeed and move Rdma device associated with PCI net device back to sandbox namespace", func() {
				pciDev := "0000:04:00.5"
				netName := "rdma-net"
				rdmaDev := "mlx5_4"
				cIfname := "net1"
				containerRdmaDev := rdmaDevPrefix + cIfname
				cid := "a1b2c3d4e5f6"
				cnsPath := "/proc/12444/ns/net"
				cns, _ := dummyNsMgr.GetCurrentNS()
				qos := rdmaTypes.RDMAQoS{TOS: 96, TC: tcValue(33)}
				rdmaState := generateRdmaNetState(pciDev, rdmaDev, containerRdmaDev, rdmaTypes.RDMAQoS{TOS: 0, TC: tcValue(0)})
				netconf := generateNetConfCmdDel(qos)
				args := generateArgs(cnsPath, cid, cIfname, &netconf)
				rdmaQoSManagerMock.On("SetRdmaDevQoS", nil, containerRdmaDev, *rdmaState.RDMAQoS).Return(nil)
				stateCacheMock.On("GetStateRef", netName, cid, cIfname).Return(cache.StateRef("some-ref"))
				stateCacheMock.On("Load", mock.AnythingOfType("cache.StateRef"),
					mock.AnythingOfType("*types.RdmaNetState")).Return(nil).Run(func(args mock.Arguments) {
					arg := args.Get(1).(*rdmaTypes.RdmaNetState)
					*arg = rdmaState
				})
				rdmaMgrMock.On("MoveRdmaDevToNs", containerRdmaDev, cns).Return(nil)
				rdmaMgrMock.On("SetRdmaDevName", containerRdmaDev, rdmaDev).Return(nil)
				stateCacheMock.On("Delete", mock.AnythingOfType("cache.StateRef")).Return(nil)
				err := plugin.CmdDel(&args)
				Expect(err).ToNot(HaveOccurred())
				rdmaMgrMock.AssertExpectations(t)
				stateCacheMock.AssertExpectations(t)
				rdmaQoSManagerMock.AssertExpectations(t)
			})
			It("Should succeed and move Rdma device associated with auxiliary device back to sandbox namespace", func() {
				auxDev := "mlx5_core.sf.6"
				netName := "rdma-net"
				rdmaDev := "mlx5_6"
				cIfname := "net2"
				containerRdmaDev := rdmaDevPrefix + cIfname
				cid := "a1b2c3d4e5f6"
				qos := rdmaTypes.RDMAQoS{TOS: 102, TC: tcValue(55)}
				cnsPath := "/proc/12444/ns/net"
				cns, _ := dummyNsMgr.GetCurrentNS()
				rdmaState := generateRdmaNetState(auxDev, rdmaDev, containerRdmaDev, rdmaTypes.RDMAQoS{TOS: 0, TC: tcValue(0)})
				netconf := generateNetConfCmdDel(qos)
				args := generateArgs(cnsPath, cid, cIfname, &netconf)
				rdmaQoSManagerMock.On("SetRdmaDevQoS", nil, containerRdmaDev, *rdmaState.RDMAQoS).Return(nil)
				stateCacheMock.On("GetStateRef", netName, cid, cIfname).Return(cache.StateRef("some-ref"))
				stateCacheMock.On("Load", mock.AnythingOfType("cache.StateRef"),
					mock.AnythingOfType("*types.RdmaNetState")).Return(nil).Run(func(args mock.Arguments) {
					arg := args.Get(1).(*rdmaTypes.RdmaNetState)
					*arg = rdmaState
				})
				rdmaMgrMock.On("MoveRdmaDevToNs", containerRdmaDev, cns).Return(nil)
				rdmaMgrMock.On("SetRdmaDevName", containerRdmaDev, rdmaDev).Return(nil)
				stateCacheMock.On("Delete", mock.AnythingOfType("cache.StateRef")).Return(nil)
				err := plugin.CmdDel(&args)
				Expect(err).ToNot(HaveOccurred())
				rdmaMgrMock.AssertExpectations(t)
				stateCacheMock.AssertExpectations(t)
				rdmaQoSManagerMock.AssertExpectations(t)
			})
		})
		// TODO(adrian): Add additional tests to cover bad flows / different network configurations
	})

	DescribeTable("ADD preserves host TC when no override is requested",
		func(config rdmaTypes.RDMAQoS, omitQoS bool, hostTC *uint32) {
			hostQoS := rdmaTypes.RDMAQoS{TOS: 61, TC: hostTC}
			effectiveQoS := rdmaTypes.RDMAQoS{TOS: 61, TC: hostTC}
			if config.TOS != 0 {
				effectiveQoS.TOS = config.TOS
			}
			conf := generateNetConfCmdAdd("rdma-net", "net1", "0000:04:00.5", "", config)
			args := generateArgs("/proc/123/ns/net", "container", "net1", &conf)
			if omitQoS {
				var raw map[string]interface{}
				Expect(json.Unmarshal(args.StdinData, &raw)).To(Succeed())
				delete(raw, "rdmaQoS")
				var marshalErr error
				args.StdinData, marshalErr = json.Marshal(raw)
				Expect(marshalErr).ToNot(HaveOccurred())
			}
			cns, _ := dummyNsMgr.GetNS(args.Netns)
			rdmaMgrMock.On("GetSystemRdmaMode").Return(rdma.RdmaSysModeExclusive, nil).Once()
			rdmaMgrMock.On("GetRdmaDevsForPciDev", conf.DeviceID).Return([]string{"mlx5_4"}).Once()
			rdmaMgrMock.On("MoveRdmaDevToNs", "mlx5_4", cns).Return(nil).Once()
			rdmaQoSManagerMock.On("LoadRdmaCniQoSConfig", config).Return().Once()
			rdmaQoSManagerMock.On("GetRdmaDevQoS", "mlx5_4").Return(hostQoS, effectiveQoS, nil).Once()
			rdmaQoSManagerMock.On("SetRdmaDevQoS", cns, "mlx5_4",
				rdmaTypes.RDMAQoS{TOS: effectiveQoS.TOS}).Return(nil).Once()
			stateCacheMock.On("GetStateRef", conf.Name, args.ContainerID, args.IfName).Return(cache.StateRef("ref")).Once()
			state := generateRdmaNetState(conf.DeviceID, "mlx5_4", "mlx5_4", hostQoS)
			stateCacheMock.On("Save", cache.StateRef("ref"), &state).Return(nil).Once()
			Expect(plugin.CmdAdd(&args)).To(Succeed())
			rdmaMgrMock.AssertExpectations(t)
			rdmaQoSManagerMock.AssertExpectations(t)
			stateCacheMock.AssertExpectations(t)
		},
		Entry("omitted QoS and disabled host TC", rdmaTypes.RDMAQoS{}, true, nil),
		Entry("empty QoS and forced host zero", rdmaTypes.RDMAQoS{}, false, tcValue(0)),
		Entry("zero QoS fields inherit nonzero host settings", rdmaTypes.RDMAQoS{TC: tcValue(0)}, false, tcValue(41)),
		Entry("TOS-only config and disabled host TC", rdmaTypes.RDMAQoS{TOS: 96}, false, nil),
	)

	DescribeTable("ADD rolls back completed mutations",
		func(failAt string) {
			hostQoS := rdmaTypes.RDMAQoS{TOS: 61, TC: nil}
			qos := rdmaTypes.RDMAQoS{TOS: 96, TC: tcValue(41)}
			conf := generateNetConfCmdAdd("rdma-net", "net1", "0000:04:00.5", "custom", qos)
			args := generateArgs("/proc/123/ns/net", "container", "net1", &conf)
			cns, _ := dummyNsMgr.GetNS(args.Netns)
			hostNS, _ := dummyNsMgr.GetCurrentNS()
			failure := fmt.Errorf("failure at %s", failAt)
			resultAt := func(stage string) error {
				if stage == failAt {
					return failure
				}
				return nil
			}
			rdmaMgrMock.On("GetSystemRdmaMode").Return(rdma.RdmaSysModeExclusive, nil).Once()
			rdmaMgrMock.On("GetRdmaDevsForPciDev", conf.DeviceID).Return([]string{"mlx5_4"}).Once()
			rdmaQoSManagerMock.On("LoadRdmaCniQoSConfig", qos).Return().Once()
			rdmaQoSManagerMock.On("GetRdmaDevQoS", "mlx5_4").Return(hostQoS, qos, nil).Once()
			applyTC := rdmaQoSManagerMock.On("SetRdmaDevQoS", nil, "mlx5_4",
				rdmaTypes.RDMAQoS{TOS: hostQoS.TOS, TC: qos.TC}).Return(resultAt("host QoS")).Once()
			currentDev := "mlx5_4"
			lastCall := applyTC
			if failAt != "host QoS" {
				lastCall = rdmaMgrMock.On("SetRdmaDevName", "mlx5_4", "custom").Return(resultAt("rename")).Once()
				if failAt != "rename" {
					currentDev = "custom"
					lastCall = rdmaMgrMock.On("MoveRdmaDevToNs", "custom", cns).Return(resultAt("move")).Once()
					if failAt != "move" {
						lastCall = rdmaQoSManagerMock.On("SetRdmaDevQoS", cns, "custom",
							rdmaTypes.RDMAQoS{TOS: qos.TOS}).Return(resultAt("container QoS")).Once()
						if failAt == "save" {
							stateCacheMock.On("GetStateRef", conf.Name, args.ContainerID, args.IfName).
								Return(cache.StateRef("ref")).Once()
							state := generateRdmaNetState(conf.DeviceID, "mlx5_4", "custom", hostQoS)
							lastCall = stateCacheMock.On("Save", cache.StateRef("ref"), &state).Return(failure).Once()
						}
						lastCall = rdmaMgrMock.On("MoveRdmaDevToNs", "custom", hostNS).
							Return(nil).Once().NotBefore(lastCall)
					}
				}
			}
			restore := rdmaQoSManagerMock.On("SetRdmaDevQoS", nil, currentDev, hostQoS).
				Return(nil).Once().NotBefore(lastCall)
			if currentDev != "mlx5_4" {
				rdmaMgrMock.On("SetRdmaDevName", currentDev, "mlx5_4").Return(nil).Once().NotBefore(restore)
			}
			Expect(plugin.CmdAdd(&args)).To(MatchError(ContainSubstring(failure.Error())))
			rdmaMgrMock.AssertExpectations(t)
			rdmaQoSManagerMock.AssertExpectations(t)
			stateCacheMock.AssertExpectations(t)
		},
		Entry("host QoS failure", "host QoS"),
		Entry("rename failure", "rename"),
		Entry("move failure", "move"),
		Entry("container QoS failure", "container QoS"),
		Entry("cache save failure", "save"),
	)

	It("DEL skips QoS restoration for a pre-QoS cache entry", func() {
		conf := generateNetConfCmdDel(rdmaTypes.RDMAQoS{})
		args := generateArgs("/proc/123/ns/net", "container", "net1", &conf)
		stateCacheMock.On("GetStateRef", conf.Name, args.ContainerID, args.IfName).Return(cache.StateRef("ref")).Once()
		stateCacheMock.On("Load", cache.StateRef("ref"), mock.Anything).Run(func(args mock.Arguments) {
			state := args.Get(1).(*rdmaTypes.RdmaNetState)
			Expect(json.Unmarshal([]byte(`{"containerRdmaDevName":"mlx5_4","sandboxRdmaDevName":"mlx5_4"}`), state)).
				To(Succeed())
		}).Return(nil).Once()
		hostNS, _ := dummyNsMgr.GetCurrentNS()
		rdmaMgrMock.On("MoveRdmaDevToNs", "mlx5_4", hostNS).Return(nil).Once()
		stateCacheMock.On("Delete", cache.StateRef("ref")).Return(nil).Once()
		Expect(plugin.CmdDel(&args)).To(Succeed())
		rdmaMgrMock.AssertExpectations(t)
		rdmaQoSManagerMock.AssertNotCalled(t, "SetRdmaDevQoS", mock.Anything, mock.Anything, mock.Anything)
		stateCacheMock.AssertExpectations(t)
	})

	DescribeTable("DEL retries restore the device under its actual host name",
		func(hostName string, netns string, missingNS bool) {
			conf := generateNetConfCmdDel(rdmaTypes.RDMAQoS{})
			args := generateArgs(netns, "container", "net1", &conf)
			state := generateRdmaNetState("0000:04:00.5", "mlx5_4", "custom", rdmaTypes.RDMAQoS{TOS: 61})
			stateCacheMock.On("GetStateRef", conf.Name, args.ContainerID, args.IfName).Return(cache.StateRef("ref"))
			stateCacheMock.On("Load", cache.StateRef("ref"), mock.Anything).Run(func(args mock.Arguments) {
				*args.Get(1).(*rdmaTypes.RdmaNetState) = state
			}).Return(nil)
			hostNS, _ := dummyNsMgr.GetCurrentNS()
			failure := errors.New("transient QoS restore failure")
			if missingNS {
				plugin.nsManager = &dummyNsMananger{getNSErr: ns.NSPathNotExistErr{}}
			} else if netns != "" {
				rdmaMgrMock.On("MoveRdmaDevToNs", "custom", hostNS).Return(nil).Once()
				rdmaQoSManagerMock.On("SetRdmaDevQoS", nil, "custom", *state.RDMAQoS).Return(failure).Once()
				Expect(plugin.CmdDel(&args)).To(MatchError(ContainSubstring(failure.Error())))
				stateCacheMock.AssertNotCalled(t, "Delete", mock.Anything)
				rdmaMgrMock.On("MoveRdmaDevToNs", "custom", hostNS).Return(rdma.ErrRdmaDeviceNotFound).Once()
			}
			rdmaMgrMock.On("GetRdmaDevsForPciDev", state.DeviceID).Return([]string{hostName}).Once()
			rdmaQoSManagerMock.On("SetRdmaDevQoS", nil, hostName, *state.RDMAQoS).Return(nil).Once()
			if hostName != state.SandboxRdmaDevName {
				rdmaMgrMock.On("SetRdmaDevName", hostName, state.SandboxRdmaDevName).Return(nil).Once()
			}
			stateCacheMock.On("Delete", cache.StateRef("ref")).Return(nil).Once()
			Expect(plugin.CmdDel(&args)).To(Succeed())
			rdmaMgrMock.AssertExpectations(t)
			rdmaQoSManagerMock.AssertExpectations(t)
			stateCacheMock.AssertExpectations(t)
		},
		Entry("partial DEL with a custom host name", "custom", "/proc/123/ns/net", false),
		Entry("preceding CNI restored the original name", "mlx5_4", "/proc/123/ns/net", false),
		Entry("empty namespace", "custom", "", false),
		Entry("removed namespace", "custom", "/proc/123/ns/net", true),
	)

	It("succeeds without changing QoS when the host device is already absent", func() {
		conf := generateNetConfCmdDel(rdmaTypes.RDMAQoS{})
		args := generateArgs("", "container", "net1", &conf)
		state := generateRdmaNetState("0000:04:00.5", "mlx5_4", "custom", rdmaTypes.RDMAQoS{TOS: 61})
		stateCacheMock.On("GetStateRef", conf.Name, args.ContainerID, args.IfName).Return(cache.StateRef("ref")).Once()
		stateCacheMock.On("Load", cache.StateRef("ref"), mock.Anything).Run(func(args mock.Arguments) {
			*args.Get(1).(*rdmaTypes.RdmaNetState) = state
		}).Return(nil).Once()
		rdmaMgrMock.On("GetRdmaDevsForPciDev", state.DeviceID).Return([]string{}).Once()
		Expect(plugin.CmdDel(&args)).To(Succeed())
		stateCacheMock.AssertNotCalled(t, "Delete", mock.Anything)
		rdmaQoSManagerMock.AssertNotCalled(t, "SetRdmaDevQoS", mock.Anything, mock.Anything, mock.Anything)
	})

	It("retains the cache and refuses QoS writes when host device identity is ambiguous", func() {
		conf := generateNetConfCmdDel(rdmaTypes.RDMAQoS{})
		args := generateArgs("", "container", "net1", &conf)
		state := generateRdmaNetState("0000:04:00.5", "mlx5_4", "custom", rdmaTypes.RDMAQoS{TOS: 61})
		stateCacheMock.On("GetStateRef", conf.Name, args.ContainerID, args.IfName).Return(cache.StateRef("ref")).Once()
		stateCacheMock.On("Load", cache.StateRef("ref"), mock.Anything).Run(func(args mock.Arguments) {
			*args.Get(1).(*rdmaTypes.RdmaNetState) = state
		}).Return(nil).Once()
		rdmaMgrMock.On("GetRdmaDevsForPciDev", state.DeviceID).Return([]string{"mlx5_4", "mlx5_5"}).Once()
		Expect(plugin.CmdDel(&args)).To(MatchError(ContainSubstring("discovered more than one RDMA device")))
		stateCacheMock.AssertNotCalled(t, "Delete", mock.Anything)
		rdmaQoSManagerMock.AssertNotCalled(t, "SetRdmaDevQoS", mock.Anything, mock.Anything, mock.Anything)
	})

	It("reports failed namespace rollback without writing host QoS to a device still in the container", func() {
		hostNS, _ := dummyNsMgr.GetCurrentNS()
		failure := errors.New("namespace restore failed")
		rdmaMgrMock.On("MoveRdmaDevToNs", "custom", hostNS).Return(failure).Once()
		Expect(plugin.rollbackRdmaDev("custom", "mlx5_4", "/proc/123/ns/net", true,
			&rdmaTypes.RDMAQoS{TOS: 61})).To(MatchError(ContainSubstring(failure.Error())))
		rdmaQoSManagerMock.AssertNotCalled(t, "SetRdmaDevQoS", mock.Anything, mock.Anything, mock.Anything)
		rdmaMgrMock.AssertNotCalled(t, "SetRdmaDevName", mock.Anything, mock.Anything)
	})

	It("reports QoS rollback failure and still restores the device name", func() {
		failure := errors.New("QoS restore failed")
		qos := rdmaTypes.RDMAQoS{TOS: 61}
		restore := rdmaQoSManagerMock.On("SetRdmaDevQoS", nil, "custom", qos).Return(failure).Once()
		rdmaMgrMock.On("SetRdmaDevName", "custom", "mlx5_4").Return(nil).Once().NotBefore(restore)
		Expect(plugin.rollbackRdmaDev("custom", "mlx5_4", "", false, &qos)).To(MatchError(ContainSubstring(failure.Error())))
		rdmaMgrMock.AssertExpectations(t)
		rdmaQoSManagerMock.AssertExpectations(t)
	})

	Describe("Test CmdCheck()", func() {
		It("Should basically do nothing", func() {
			Expect(plugin.CmdCheck(nil)).To(Succeed())
		})
	})
})
