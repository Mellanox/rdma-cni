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
	"os"

	"github.com/spf13/afero"
)

func newFsOps() FileSystemOps {
	return &stdFileSystemOps{}
}

// interface consolidating all file system operations
type FileSystemOps interface {
	// Eqivalent to os.ReadFile(...)
	ReadFile(filename string) ([]byte, error)
	// Eqivalent to os.WriteFile(...)
	WriteFile(filename string, data []byte, perm os.FileMode) error
	// Eqivalent to os.MkdirAll(...)
	MkdirAll(path string, perm os.FileMode) error
	// Equivalent to os.Remove(...)
	Remove(name string) error
	// Equvalent to os.Stat(...)
	Stat(name string) (os.FileInfo, error)
}

type stdFileSystemOps struct{}

func (sfs *stdFileSystemOps) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

func (sfs *stdFileSystemOps) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return os.WriteFile(filename, data, perm)
}

func (sfs *stdFileSystemOps) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (sfs *stdFileSystemOps) Remove(name string) error {
	return os.Remove(name)
}

func (sfs *stdFileSystemOps) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

// Fake fileSystemOps used for Unit testing
func newFakeFileSystemOps() FileSystemOps {
	return &fakeFileSystemOps{fakefs: afero.Afero{Fs: afero.NewMemMapFs()}}
}

type fakeFileSystemOps struct {
	fakefs afero.Afero
}

func (ffs *fakeFileSystemOps) ReadFile(filename string) ([]byte, error) {
	return ffs.fakefs.ReadFile(filename)
}

func (ffs *fakeFileSystemOps) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return ffs.fakefs.WriteFile(filename, data, perm)
}

func (ffs *fakeFileSystemOps) MkdirAll(path string, perm os.FileMode) error {
	return ffs.fakefs.MkdirAll(path, perm)
}

func (ffs *fakeFileSystemOps) Remove(name string) error {
	return ffs.fakefs.Remove(name)
}

func (ffs *fakeFileSystemOps) Stat(name string) (os.FileInfo, error) {
	return ffs.fakefs.Stat(name)
}
