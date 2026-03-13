// Copyright 2025 NVIDIA CORPORATION & AFFILIATES
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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
	"bytes"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/containernetworking/plugins/pkg/ns"
	rdmatypes "github.com/k8snetworkplumbingwg/rdma-cni/pkg/types"
	"github.com/k8snetworkplumbingwg/rdma-cni/pkg/utils"
	"github.com/rs/zerolog/log"
)

const (
	rdmaCMConfigfsPath = "/sys/kernel/config/rdma_cm"
	rdmaSysfsPath      = "/sys/class/infiniband"
)

// QoSManager interface for managing RDMA device QoS configuration.
type QoSManager interface {
	// Get RDMA device QoS
	GetRdmaDevQoS(rdmaDev string) (rdmatypes.RDMAQoS, error)
	// Set RDMA device QoS
	SetRdmaDevQoS(targetNs ns.NetNS, rdmaDev string, qos rdmatypes.RDMAQoS) error
	// Set RDMA CNI QoS configuration
	LoadRdmaCniQoSConfig(qosConf rdmatypes.RDMAQoS)
}

func NewRdmaQoSManager() QoSManager {
	return &rdmaQoSManager{
		qosConf: rdmatypes.RDMAQoS{},
		ops:     newrdmaQoSManagerOps(),
	}
}

type rdmaQoSManager struct {
	qosConf rdmatypes.RDMAQoS
	ops     rdmaQoSManagerOps
}

type rdmaQoSManagerOps interface {
	getRdmaDevQoSFormSysfs(rdmaDev string) (rdmatypes.RDMAQoS, error)
	setRdmaDevQoSToSysfs(targetNs ns.NetNS, rdmaDev string, qos rdmatypes.RDMAQoS) error
}

type rdmaQoSManagerOpsIml struct{}

func newrdmaQoSManagerOps() rdmaQoSManagerOps {
	return &rdmaQoSManagerOpsIml{}
}

// GetRdmaDevQoS returns RDMA QoS for the given RDMA device.
// 1. create /sys/kernel/config/rdma_cm/<rdmaDev> directory
// 2. read the default_roce_tos value from /sys/kernel/config/rdma_cm/<rdmaDev>/ports/1/default_roce_tos
// 3. read the traffic class value from /sys/class/infiniband/<rdmaDev>/tc/1/traffic_class
func (rqm *rdmaQoSManagerOpsIml) getRdmaDevQoSFormSysfs(rdmaDev string) (rdmatypes.RDMAQoS, error) {
	var (
		tosVal uint32
		tcVal  uint32
	)
	rdmaDevQoSPath := path.Join(rdmaCMConfigfsPath, rdmaDev)
	err := os.MkdirAll(rdmaDevQoSPath, 0755)
	if err != nil {
		return rdmatypes.RDMAQoS{}, fmt.Errorf("failed to create directory %s: %w", rdmaDevQoSPath, err)
	}
	tos, err := os.ReadFile(path.Join(rdmaDevQoSPath, "ports", "1", "default_roce_tos"))
	if err != nil {
		return rdmatypes.RDMAQoS{}, err
	}

	tosVal, err = parseUint32(tos)
	if err != nil {
		return rdmatypes.RDMAQoS{}, err
	}

	// check if /sys/class/infiniband/<rdmaDev>/tc exists
	if _, err := os.Stat(path.Join(rdmaSysfsPath, rdmaDev, "tc")); err == nil {
		// read the traffic class value from /sys/class/infiniband/<rdmaDev>/tc/1/traffic_class
		// file may contain multiple lines; first line is "Global tclass=<value>". If missing or invalid, use 0 so CNI config applies.
		tc, err := os.ReadFile(path.Join(rdmaSysfsPath, rdmaDev, "tc", "1", "traffic_class"))
		if err != nil {
			return rdmatypes.RDMAQoS{}, fmt.Errorf("failed to read traffic class file for RDMA device %s: %w", rdmaDev, err)
		}

		// if tc is not empty, parse the traffic class value
		if len(tc) > 0 {
			tcValStr := string(tc)
			if i := strings.Index(tcValStr, "tclass="); i >= 0 {
				// fetch tclass value, handle case where tc is a multi-line string
				tcValStr = strings.TrimSpace(strings.SplitN(tcValStr[i+len("tclass="):], "\n", 2)[0])
			}
			tcVal, err = parseUint32([]byte(tcValStr))
			if err != nil {
				return rdmatypes.RDMAQoS{}, err
			}
		}
	} else {
		if !os.IsNotExist(err) {
			return rdmatypes.RDMAQoS{}, err
		}
		log.Warn().Msgf("TC (traffic class) was not found for RDMA device %s.", rdmaDev)
	}

	return rdmatypes.RDMAQoS{TOS: tosVal, TC: tcVal}, nil

}

// SetRdmaDevQoS sets RoCE TOS and TC for the RDMA device in the target network namespace.
// The kernel does not preserve default_roce_tos when moving an RDMA device
// to another netns via netlink.
// CMA's cma_device is removed and re-created with zeroed default_roce_tos.
// Setting QoS in the target namespace after the move is therefore required.
func (rqm *rdmaQoSManagerOpsIml) setRdmaDevQoSToSysfs(targetNs ns.NetNS, rdmaDev string, qos rdmatypes.RDMAQoS) error {

	if qos.TOS > 0 {
		// mount configfs in case executed in non-root network namespace
		if targetNs != nil {
			err := utils.MountConfigFS()
			if err != nil {
				return fmt.Errorf("failed to mount configfs in target namespace: %w", err)
			}
		}
		rdmaDevQoSPath := path.Join(rdmaCMConfigfsPath, rdmaDev)
		err := os.MkdirAll(rdmaDevQoSPath, 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory %s: %w", rdmaDevQoSPath, err)
		}
		err = os.WriteFile(path.Join(rdmaDevQoSPath, "ports", "1", "default_roce_tos"), []byte(strconv.Itoa(int(qos.TOS))), 0644)
		if err != nil {
			return err
		}
	}

	if qos.TC > 0 {
		// check if /sys/class/infiniband/<rdmaDev>/tc exists
		if _, err := os.Stat(path.Join(rdmaSysfsPath, rdmaDev, "tc")); err == nil {
			err = os.WriteFile(path.Join(rdmaSysfsPath, rdmaDev, "tc", "1", "traffic_class"), []byte(strconv.Itoa(int(qos.TC))), 0644)
			if err != nil {
				return err
			}
			// read the traffic class value from /sys/class/infiniband/<rdmaDev>/tc/1/traffic_class
			tc, err := os.ReadFile(path.Join(rdmaSysfsPath, rdmaDev, "tc", "1", "traffic_class"))
			if err != nil {
				return err
			}
			log.Warn().Msgf("TC: '%s'val: '%s' for RDMA device %s.", path.Join(rdmaSysfsPath, rdmaDev, "tc", "1", "traffic_class"), tc, rdmaDev)
		} else {
			if !os.IsNotExist(err) {
				return err
			}
			log.Warn().Msgf("TC (traffic class) was not applied to RDMA device %s. Skipping TC setting.", rdmaDev)
		}
	}

	return nil
}

// LoadRdmaCniQoSConfig sets RDMA CNI QoS configuration.
func (rqm *rdmaQoSManager) LoadRdmaCniQoSConfig(qosConf rdmatypes.RDMAQoS) {
	rqm.qosConf = qosConf
}

// GetRdmaDevQoS gets RDMA device QoS.
func (rqm *rdmaQoSManager) GetRdmaDevQoS(rdmaDev string) (rdmatypes.RDMAQoS, error) {
	log.Info().Msgf("getting RDMA device %s QoS", rdmaDev)
	qos, err := rqm.ops.getRdmaDevQoSFormSysfs(rdmaDev)
	if err != nil {
		return rdmatypes.RDMAQoS{}, fmt.Errorf("failed to get RDMA device %s QoS: %w", rdmaDev, err)
	}

	if qos.TOS == 0 {
		qos.TOS = rqm.qosConf.TOS
	}
	if qos.TC == 0 {
		qos.TC = rqm.qosConf.TC
	}

	return qos, nil
}

// SetRdmaDevQoS sets RDMA device QoS.
func (rqm *rdmaQoSManager) SetRdmaDevQoS(targetNs ns.NetNS, rdmaDev string, qos rdmatypes.RDMAQoS) error {
	return rqm.ops.setRdmaDevQoSToSysfs(targetNs, rdmaDev, qos)
}

// parseUint32 trims trailing newline and parses a 32-bit unsigned integer, rejecting negative or overflow.
// Empty or whitespace-only input (e.g. from uninitialized sysfs files) is treated as 0 so CNI QoS defaults can apply.
func parseUint32(b []byte) (uint32, error) {
	s := strings.TrimSpace(string(bytes.TrimSuffix(b, []byte("\n"))))
	if s == "" {
		return 0, nil
	}
	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint32(v), nil
}
