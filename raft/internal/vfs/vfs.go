// Copyright 2017-2019 Lei Ni (nilei81@gmail.com) and other contributors.
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

package vfs

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/errors/oserror"
	gvfs "github.com/lni/vfs"
	pvfs "github.com/zuoyebang/bitalostable/vfs"
)

// IFS is the vfs interface used by dragonboat.
type IFS = gvfs.FS

// MemFS is a memory backed file system for testing purposes.
type MemFS = gvfs.MemFS

// DefaultFS is a vfs instance using underlying OS fs.
var DefaultFS IFS = gvfs.Default

// MemStrictFS is a vfs instance using memfs.
var MemStrictFS IFS = gvfs.NewStrictMem()

// File is the file interface returned by IFS.
type File = gvfs.File

// NewMemFS creates a in-memory fs.
func NewMemFS() IFS {
	return gvfs.NewStrictMem()
}

// BitableFS is a wrapper struct that implements the bitable/vfs.FS interface.
type BitableFS struct {
	fs IFS
}

func (p *BitableFS) GetDiskUsage(path string) (pvfs.DiskUsage, error) {
	return pvfs.DiskUsage{}, nil
}

var _ pvfs.FS = (*BitableFS)(nil)

// NewBitableFS creates a new bitable/vfs.FS instance.
func NewBitableFS(fs IFS) pvfs.FS {
	return &BitableFS{fs}
}

// GetFreeSpace ...
func (p *BitableFS) GetFreeSpace(path string) (uint64, error) {
	return p.fs.GetFreeSpace(path)
}

// Create ...
func (p *BitableFS) Create(name string) (pvfs.File, error) {
	return p.fs.Create(name)
}

// Link ...
func (p *BitableFS) Link(oldname, newname string) error {
	return p.fs.Link(oldname, newname)
}

// Open ...
func (p *BitableFS) Open(name string, opts ...pvfs.OpenOption) (pvfs.File, error) {
	f, err := p.fs.Open(name)
	if err != nil {
		return nil, err
	}
	for _, opt := range opts {
		opt.Apply(f)
	}
	return f, nil
}

// OpenDir ...
func (p *BitableFS) OpenDir(name string) (pvfs.File, error) {
	return p.fs.OpenDir(name)
}

// Remove ...
func (p *BitableFS) Remove(name string) error {
	return p.fs.Remove(name)
}

// RemoveAll ...
func (p *BitableFS) RemoveAll(name string) error {
	return p.fs.RemoveAll(name)
}

// Rename ...
func (p *BitableFS) Rename(oldname, newname string) error {
	return p.fs.Rename(oldname, newname)
}

// ReuseForWrite ...
func (p *BitableFS) ReuseForWrite(oldname, newname string) (pvfs.File, error) {
	return p.fs.ReuseForWrite(oldname, newname)
}

// MkdirAll ...
func (p *BitableFS) MkdirAll(dir string, perm os.FileMode) error {
	return p.fs.MkdirAll(dir, perm)
}

// Lock ...
func (p *BitableFS) Lock(name string) (io.Closer, error) {
	return p.fs.Lock(name)
}

// List ...
func (p *BitableFS) List(dir string) ([]string, error) {
	return p.fs.List(dir)
}

// Stat ...
func (p *BitableFS) Stat(name string) (os.FileInfo, error) {
	return p.fs.Stat(name)
}

// PathBase ...
func (p *BitableFS) PathBase(path string) string {
	return p.fs.PathBase(path)
}

// PathJoin ...
func (p *BitableFS) PathJoin(elem ...string) string {
	return p.fs.PathJoin(elem...)
}

// PathDir ...
func (p *BitableFS) PathDir(path string) string {
	return p.fs.PathDir(path)
}

// IsNotExist returns a boolean value indicating whether the specified error is
// to indicate that a file or directory does not exist.
func IsNotExist(err error) bool {
	return oserror.IsNotExist(err)
}

// IsExist returns a boolean value indicating whether the specified error is to
// indicate that a file or directory already exists.
func IsExist(err error) bool {
	return oserror.IsExist(err)
}

// TempDir returns the directory use for storing temporary files.
func TempDir() string {
	return os.TempDir()
}

// Clean is a wrapper for filepath.Clean.
func Clean(dir string) string {
	return filepath.Clean(dir)
}

// ReportLeakedFD reports leaked file fds.
func ReportLeakedFD(fs IFS, t *testing.T) {
	gvfs.ReportLeakedFD(fs, t)
}
