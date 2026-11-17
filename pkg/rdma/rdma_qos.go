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
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"syscall"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/rs/zerolog/log"
	"github.com/spf13/afero"

	rdmatypes "github.com/k8snetworkplumbingwg/rdma-cni/pkg/types"
	"github.com/k8snetworkplumbingwg/rdma-cni/pkg/utils"
)

const (
	rdmaCMConfigfsPath = "/sys/kernel/config/rdma_cm"
	rdmaSysfsPath      = "/sys/class/infiniband"
	sysfsDirMode       = 0755
	sysfsFileMode      = 0644
)

// QoSManager interface for managing RDMA device QoS configuration.
type QoSManager interface {
	// Get RDMA device QoS
	GetRdmaDevQoS(rdmaDev string) (rdmatypes.RDMAQoS, rdmatypes.RDMAQoS, error)
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

type rdmaQoSManagerOpsIml struct {
	fs            afero.Afero
	mountConfigFS func() error
}

func newrdmaQoSManagerOps() rdmaQoSManagerOps {
	return &rdmaQoSManagerOpsIml{
		fs:            afero.Afero{Fs: afero.NewOsFs()},
		mountConfigFS: utils.MountConfigFS,
	}
}

// GetRdmaDevQoS returns RDMA QoS for the given RDMA device.
// 1. create /sys/kernel/config/rdma_cm/<rdmaDev> directory
// 2. read the default_roce_tos value from /sys/kernel/config/rdma_cm/<rdmaDev>/ports/1/default_roce_tos
// 3. read the traffic class value from /sys/class/infiniband/<rdmaDev>/tc/1/traffic_class
func (rqm *rdmaQoSManagerOpsIml) getRdmaDevQoSFormSysfs(rdmaDev string) (rdmatypes.RDMAQoS, error) {
	var (
		tosVal uint32
		tcVal  *uint32
	)
	rdmaDevQoSPath := path.Join(rdmaCMConfigfsPath, rdmaDev)
	err := rqm.fs.MkdirAll(rdmaDevQoSPath, sysfsDirMode)
	if err != nil {
		return rdmatypes.RDMAQoS{}, fmt.Errorf("failed to create directory %s: %w", rdmaDevQoSPath, err)
	}
	tos, err := rqm.fs.ReadFile(path.Join(rdmaDevQoSPath, "ports", "1", "default_roce_tos"))
	if err != nil {
		return rdmatypes.RDMAQoS{}, err
	}

	tosVal, err = parseUint32(tos)
	if err != nil {
		return rdmatypes.RDMAQoS{}, err
	}

	// check if /sys/class/infiniband/<rdmaDev>/tc exists
	if _, err := rqm.fs.Stat(path.Join(rdmaSysfsPath, rdmaDev, "tc")); err == nil {
		// read the traffic class value from /sys/class/infiniband/<rdmaDev>/tc/1/traffic_class
		// A missing global entry means application-selected traffic classes are not overridden.
		tc, err := rqm.fs.ReadFile(path.Join(rdmaSysfsPath, rdmaDev, "tc", "1", "traffic_class"))
		if err != nil {
			return rdmatypes.RDMAQoS{}, fmt.Errorf("failed to read traffic class file for RDMA device %s: %w", rdmaDev, err)
		}

		tcVal, err = parseTrafficClass(tc)
		if err != nil {
			return rdmatypes.RDMAQoS{}, fmt.Errorf("failed to parse traffic class for RDMA device %s: %w", rdmaDev, err)
		}
	} else {
		if !os.IsNotExist(err) {
			return rdmatypes.RDMAQoS{}, err
		}
		log.Warn().Msgf("TC (traffic class) was not found for RDMA device %s.", rdmaDev)
	}

	return rdmatypes.RDMAQoS{TOS: tosVal, TC: tcVal}, nil
}

// setRdmaDevQoSToSysfs sets TOS in either namespace and TC only in the host namespace.
// The kernel does not preserve default_roce_tos when moving an RDMA device
// to another netns via netlink.
// CMA's cma_device is removed and re-created with zeroed default_roce_tos.
// Setting QoS in the target namespace after the move is therefore required.
func (rqm *rdmaQoSManagerOpsIml) setRdmaDevQoSToSysfs(targetNs ns.NetNS, rdmaDev string, qos rdmatypes.RDMAQoS) error {
	// mount configfs in case executed in non-root network namespace
	if targetNs != nil {
		err := rqm.mountConfigFS()
		if err != nil {
			return fmt.Errorf("failed to mount configfs in target namespace: %w", err)
		}
	}
	rdmaDevQoSPath := path.Join(rdmaCMConfigfsPath, rdmaDev)
	err := rqm.fs.MkdirAll(rdmaDevQoSPath, sysfsDirMode)
	if err != nil {
		return fmt.Errorf("failed to create directory %s: %w", rdmaDevQoSPath, err)
	}
	err = rqm.fs.WriteFile(path.Join(rdmaDevQoSPath, "ports", "1", "default_roce_tos"),
		[]byte(strconv.Itoa(int(qos.TOS))), sysfsFileMode)
	if err != nil {
		if errors.Is(err, syscall.ENODEV) {
			return fmt.Errorf("rdma device %s not found while writing default_roce_tos: %w",
				rdmaDev, errors.Join(ErrRdmaDeviceNotFound, err))
		}
		return err
	}

	// mlx5 traffic class is host-only. Never change it after moving into a container namespace.
	if targetNs != nil {
		return nil
	}

	// check if /sys/class/infiniband/<rdmaDev>/tc exists
	if _, err := rqm.fs.Stat(path.Join(rdmaSysfsPath, rdmaDev, "tc")); err == nil {
		trafficClass := "-1"
		if qos.TC != nil {
			trafficClass = strconv.FormatUint(uint64(*qos.TC), 10)
		}
		err = rqm.fs.WriteFile(path.Join(rdmaSysfsPath, rdmaDev, "tc", "1", "traffic_class"),
			[]byte(trafficClass), sysfsFileMode)
		if err != nil {
			return err
		}
		// read the traffic class value from /sys/class/infiniband/<rdmaDev>/tc/1/traffic_class
		tc, err := rqm.fs.ReadFile(path.Join(rdmaSysfsPath, rdmaDev, "tc", "1", "traffic_class"))
		if err != nil {
			return err
		}
		log.Warn().Msgf("TC: '%s'val: '%s' for RDMA device %s.",
			path.Join(rdmaSysfsPath, rdmaDev, "tc", "1", "traffic_class"), tc, rdmaDev)
	} else {
		if !os.IsNotExist(err) {
			return err
		}
		log.Warn().Msgf("TC (traffic class) was not applied to RDMA device %s. Skipping TC setting.", rdmaDev)
	}

	return nil
}

// LoadRdmaCniQoSConfig sets RDMA CNI QoS configuration.
func (rqm *rdmaQoSManager) LoadRdmaCniQoSConfig(qosConf rdmatypes.RDMAQoS) {
	if qosConf.TC != nil {
		tc := *qosConf.TC
		qosConf.TC = &tc
	}
	rqm.qosConf = qosConf
}

// GetRdmaDevQoS gets RDMA device host QoS and CNI QoS configuration.
func (rqm *rdmaQoSManager) GetRdmaDevQoS(rdmaDev string) (rdmatypes.RDMAQoS, rdmatypes.RDMAQoS, error) {
	log.Info().Msgf("getting RDMA device %s QoS", rdmaDev)
	hostQos, err := rqm.ops.getRdmaDevQoSFormSysfs(rdmaDev)
	if err != nil {
		return rdmatypes.RDMAQoS{}, rdmatypes.RDMAQoS{}, fmt.Errorf("failed to get RDMA device %s host QoS: %w", rdmaDev, err)
	}

	// Zero or omitted configuration values inherit the host settings. Keep the
	// disabled global-TC state distinct from a global override of zero.
	qos := hostQos
	if rqm.qosConf.TOS != 0 {
		qos.TOS = rqm.qosConf.TOS
	}
	if rqm.qosConf.TC != nil && *rqm.qosConf.TC != 0 {
		qos.TC = rqm.qosConf.TC
	}
	if qos.TC != nil {
		tc := *qos.TC
		qos.TC = &tc
	}
	return hostQos, qos, nil
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

// parseTrafficClass reads only the global override. Rule-specific traffic classes
// must not become a global setting; an empty or rules-only file has no override.
func parseTrafficClass(data []byte) (*uint32, error) {
	const globalPrefix = "Global tclass="
	for _, line := range strings.Split(string(data), "\n") {
		value, found := strings.CutPrefix(strings.TrimSpace(line), globalPrefix)
		if !found {
			continue
		}
		tc, err := strconv.ParseUint(strings.TrimSpace(value), 10, 32)
		if err != nil {
			return nil, err
		}
		result := uint32(tc)
		return &result, nil
	}
	return nil, nil
}
