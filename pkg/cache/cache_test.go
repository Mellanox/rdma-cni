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

package cache

import (
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type myTestState struct {
	FirstState  string `json:"firstState"`
	SecondState int    `json:"secondState"`
}

var _ = Describe("Cache - Simple marshall-able state-object cache", func() {
	var stateCache StateCache
	var fs FileSystemOps
	JustBeforeEach(func() {
		fs = newFakeFileSystemOps()
		stateCache = &FsStateCache{basePath: CacheDir, fsOps: fs}
	})

	Describe("Get State reference", func() {
		Context("Basic call", func() {
			It("Should return <network>-<cid>-<ifname>", func() {
				Expect(stateCache.GetStateRef("myNet", "containerUniqueIdentifier", "net1")).To(
					BeEquivalentTo("myNet-containerUniqueIdentifier-net1"))
			})
		})
	})

	Describe("Save and Load State", func() {
		var sRef StateRef
		JustBeforeEach(func() {
			sRef = stateCache.GetStateRef("mynet", "cid", "net1")
		})

		Context("Save and Load with marshallable object", func() {
			It("Should save/load the state", func() {
				savedState := myTestState{FirstState: "first", SecondState: 42}
				var loadedState myTestState
				Expect(stateCache.Save(sRef, &savedState)).Should(Succeed())
				_, err := fs.Stat(path.Join(CacheDir, string(sRef)))
				Expect(err).ToNot(HaveOccurred())
				Expect(stateCache.Load(sRef, &loadedState)).Should(Succeed())
				Expect(loadedState).Should(Equal(savedState))
			})
		})
		Context("Load non-existent state", func() {
			It("Should fail", func() {
				var loadedState myTestState
				Expect(stateCache.Load(sRef, &loadedState)).ShouldNot(Succeed())
			})
		})
	})

	Describe("Delete State", func() {
		var sRef StateRef
		JustBeforeEach(func() {
			sRef = stateCache.GetStateRef("mynet", "cid", "net1")
		})

		Context("Delete a saved state", func() {
			It("Should not exist after delete", func() {
				savedState := myTestState{FirstState: "first", SecondState: 42}
				Expect(stateCache.Save(sRef, &savedState)).Should(Succeed())
				_, err := fs.Stat(path.Join(CacheDir, string(sRef)))
				Expect(err).ToNot(HaveOccurred())
				Expect(stateCache.Delete(sRef)).Should(Succeed())
				_, err = fs.Stat(path.Join(CacheDir, string(sRef)))
				Expect(err).To(HaveOccurred())
			})
		})
		Context("Delete a non existent state", func() {
			It("Should Fail", func() {
				altRef := stateCache.GetStateRef("alt-mynet", "cid", "net1")
				Expect(stateCache.Delete(altRef)).To(HaveOccurred())
				_, err := fs.Stat(path.Join(CacheDir, string(altRef)))
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
