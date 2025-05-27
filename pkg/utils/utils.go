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
	"path"
	"path/filepath"

	"github.com/vishvananda/netlink"
)

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
