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

package utils

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

// configFSMountPoint is the path where configfs is typically mounted.
const configFSMountPoint = "/sys/kernel/config"

// Get VF PCI device associated with the given MAC.
// this method compares with administrative MAC for SRIOV configured net devices
// TODO: move this method to github: Mellanox/sriovnet
func GetVfPciDevFromMAC(mac string) (string, error) {
	var err error
	var links []netlink.Link
	var vfPath string
	links, err = netlink.LinkList()
	if err != nil {
		return "", err
	}
	matchDevs := []string{}
	for _, link := range links {
		if len(link.Attrs().Vfs) > 0 {
			for i := range link.Attrs().Vfs {
				if link.Attrs().Vfs[i].Mac.String() == mac {
					vfPath, err = filepath.EvalSymlinks(
						fmt.Sprintf("/sys/class/net/%s/device/virtfn%d", link.Attrs().Name, link.Attrs().Vfs[i].ID))
					if err == nil {
						matchDevs = append(matchDevs, path.Base(vfPath))
					}
				}
			}
		}
	}

	var dev string
	switch len(matchDevs) {
	case 1:
		dev = matchDevs[0]
		err = nil
	case 0:
		err = fmt.Errorf("could not find VF PCI device according to administrative mac address set on PF")
	default:
		err = fmt.Errorf("found more than one VF PCI device matching provided administrative mac address")
	}
	return dev, err
}

// IsPCIAddress returns whether the input is a valid PCI address.
func IsPCIAddress(pciAddress string) bool {
	re := regexp.MustCompile(`^[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-7]$`)
	return re.MatchString(pciAddress)
}

func GetRdmaDevQoS(rdmaDev string) (uint32, error) {

	// mimic the following bash command in go:
	// mkdir -p /sys/kernel/config/rdma_cm/${RDMA_DEV}
	// echo ${ROCE_TOS_VAL} > /sys/kernel/config/rdma_cm/${RDMA_DEV}/ports/1/default_roce_tos
	// 1. create /sys/kernel/config/rdma_cm/${RDMA_DEV} directory
	// 2. read the default_roce_tos value from /sys/kernel/config/rdma_cm/${RDMA_DEV}/ports/1/default_roce_tos
	// 3. return the value as uint32
	// create the directory if it doesn't exist
	rdmaDevQoSPath := path.Join("/sys/kernel/config/rdma_cm", rdmaDev)
	err := os.MkdirAll(rdmaDevQoSPath, 0755)
	if err != nil {
		return 0, fmt.Errorf("failed to create directory %s: %w", rdmaDevQoSPath, err)
	}
	qos, err := os.ReadFile(path.Join(rdmaDevQoSPath, "ports", "1", "default_roce_tos"))
	if err != nil {
		return 0, err
	}
	qosInt, err := strconv.Atoi(string(qos[:len(qos)-1]))
	if err != nil {
		return 0, err
	}
	return uint32(qosInt), nil

}

// isConfigFSMounted reads /proc/mounts in the current namespace and reports
// whether configfs is mounted at configFSMountPoint.
func isConfigFSMounted() (bool, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[1] == configFSMountPoint && fields[2] == "configfs" {
			return true, nil
		}
	}
	return false, nil
}

// MountConfigFSInNetns is the Go equivalent of:
//
//	ip netns exec ${netnsPath} sh -c 'grep -q /sys/kernel/config /proc/mounts || mount -t configfs none /sys/kernel/config'
//
// It switches the current thread into the network namespace at netnsPath (e.g.
// /var/run/netns/ns1), mounts configfs at /sys/kernel/config if not already
// mounted, then switches back. Uses github.com/vishvananda/netns to change
// namespace; the actual mount is done via the mount(2) syscall (netlink does
// not run arbitrary commands).
//
// Requires root. netnsPath is typically CNI_NETNS or /var/run/netns/<name>.
// The caller must ensure the calling goroutine is locked to its OS thread (e.g.
// main's init() already calls runtime.LockOSThread() for the main goroutine).
func MountConfigFSInNetns(targetNs ns.NetNS) error {
	origNs, err := netns.Get()
	if err != nil {
		return fmt.Errorf("get current netns: %w", err)
	}

	if err := netns.Set(netns.NsHandle(targetNs.Fd())); err != nil {
		return fmt.Errorf("set netns to %q: %w", targetNs.Path(), err)
	}
	defer func() { _ = netns.Set(origNs) }()

	mounted, err := isConfigFSMounted()
	if err != nil {
		return fmt.Errorf("check configfs mount: %w", err)
	}
	if mounted {
		return nil
	}

	if err := syscall.Mount("none", configFSMountPoint, "configfs", 0, ""); err != nil {
		return fmt.Errorf("mount configfs at %s: %w", configFSMountPoint, err)
	}
	return nil
}

// SetRdmaDevQoS sets RoCE default TOS (and optionally TC) for the RDMA device in the target
// network namespace. The kernel does not preserve default_roce_tos when moving an RDMA device
// to another netns via netlink (rdma dev set ... netns ...): it runs disable_device then
// enable_device_and_get, so CMA's cma_device is removed and re-created with zeroed default_roce_tos.
// Setting QoS in the target namespace after the move is therefore required.
func SetRdmaDevQoS(targetNs ns.NetNS, rdmaDev string, qos uint32, TC uint32) error {
	// mimic the following bash command in go:
	// ip netns exec ${USER_NS} sh -c'
	//grep -q /sys/kernel/config /proc/mounts || mount -t configfs none /sys/kernel/config
	//mkdir -p /sys/kernel/config/rdma_cm/'"${RDMA_DEV}"'
	//echo '"${ROCE_TOS}"' > /sys/kernel/config/rdma_cm/'"${RDMA_DEV}"'/ports/1/default_roce_tos'
	// mkdir -p /sys/kernel/config/rdma_cm/${RDMA_DEV}
	// echo ${ROCE_TOS_VAL} > /sys/kernel/config/rdma_cm/${RDMA_DEV}/ports/1/default_roce_tos
	if targetNs != nil {
		MountConfigFSInNetns(targetNs)
	}
	rdmaDevQoSPath := path.Join("/sys/kernel/config/rdma_cm", rdmaDev)
	err := os.MkdirAll(rdmaDevQoSPath, 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory %s: %w", rdmaDevQoSPath, err)
	}
	err = os.WriteFile(path.Join(rdmaDevQoSPath, "ports", "1", "default_roce_tos"), []byte(strconv.Itoa(int(qos))), 0644)
	if err != nil {
		return err
	}

	return nil
}
