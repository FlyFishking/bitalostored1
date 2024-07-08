// Copyright 2017-2021 Lei Ni (nilei81@gmail.com) and other contributors.
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

package fileutil

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/cockroachdb/errors/oserror"

	"github.com/zuoyebang/bitalostored/raft/internal/utils"
	"github.com/zuoyebang/bitalostored/raft/internal/vfs"
	pb "github.com/zuoyebang/bitalostored/raft/raftpb"
)

const (
	// SnapshotFlagFilename defines the filename of the snapshot flag file.
	SnapshotFlagFilename = "dragonboat.snapshot.message"
	defaultDirFileMode   = 0750
	deleteFilename       = "DELETED.dragonboat"
)

var firstError = utils.FirstError

var ws = errors.WithStack

// MustWrite writes the specified data to the input writer. It will panic if
// there is any error.
func MustWrite(w io.Writer, data []byte) {
	if _, err := w.Write(data); err != nil {
		panic(err)
	}
}

// DirExist returns whether the specified filesystem entry exists.
func DirExist(name string, fs vfs.IFS) (result bool, err error) {
	if name == "." || name == "/" {
		return true, nil
	}
	f, err := fs.OpenDir(name)
	if err != nil && vfs.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() {
		err = firstError(err, ws(f.Close()))
	}()
	s, err := f.Stat()
	if err != nil {
		return false, ws(err)
	}
	if !s.IsDir() {
		panic("not a dir")
	}
	return true, nil
}

// Exist returns whether the specified filesystem entry exists.
func Exist(name string, fs vfs.IFS) (bool, error) {
	if name == "." || name == "/" {
		return true, nil
	}
	_, err := fs.Stat(name)
	if err != nil && vfs.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// MkdirAll creates the specified dir along with any necessary parents.
func MkdirAll(dir string, fs vfs.IFS) error {
	exist, err := DirExist(dir, fs)
	if err != nil {
		return err
	}
	if exist {
		return nil
	}
	parent := fs.PathDir(dir)
	exist, err = DirExist(parent, fs)
	if err != nil {
		return err
	}
	if !exist {
		if err := MkdirAll(parent, fs); err != nil {
			return err
		}
	}
	return Mkdir(dir, fs)
}

// Mkdir creates the specified dir.
func Mkdir(dir string, fs vfs.IFS) error {
	parent := fs.PathDir(dir)
	exist, err := DirExist(parent, fs)
	if err != nil {
		return err
	}
	if !exist {
		panic(fmt.Sprintf("%s doesn't exist when creating %s", parent, dir))
	}
	if err := fs.MkdirAll(dir, defaultDirFileMode); err != nil {
		return err
	}
	return SyncDir(parent, fs)
}

// SyncDir calls fsync on the specified directory.
func SyncDir(dir string, fs vfs.IFS) (err error) {
	if runtime.GOOS == "windows" {
		return nil
	}
	if dir == "." {
		return nil
	}
	f, err := fs.OpenDir(dir)
	if err != nil {
		return err
	}
	defer func() {
		err = firstError(err, ws(f.Close()))
	}()
	fileInfo, err := f.Stat()
	if err != nil {
		return ws(err)
	}
	if !fileInfo.IsDir() {
		panic("not a dir")
	}
	df, err := fs.OpenDir(vfs.Clean(dir))
	if err != nil {
		return err
	}
	defer func() {
		err = firstError(err, ws(df.Close()))
	}()
	return ws(df.Sync())
}

// MarkDirAsDeleted marks the specified directory as deleted.
func MarkDirAsDeleted(dir string, msg pb.Marshaler, fs vfs.IFS) error {
	return CreateFlagFile(dir, deleteFilename, msg, fs)
}

// IsDirMarkedAsDeleted returns a boolean flag indicating whether the specified
// directory has been marked as deleted.
func IsDirMarkedAsDeleted(dir string, fs vfs.IFS) (bool, error) {
	return Exist(fs.PathJoin(dir, deleteFilename), fs)
}

func getHash(data []byte) []byte {
	h := md5.New()
	MustWrite(h, data)
	s := h.Sum(nil)
	return s[8:]
}

// CreateFlagFile creates a flag file in the specific location. The flag file
// contains the marshaled data of the specified protobuf message.
//
// CreateFlagFile is not atomic meaning you can end up having a file at
// fs.PathJoin(dir, filename) with partial or corrupted content when the machine
// crashes in the middle of this function call. Special care must be taken to
// handle such situation, see how CreateFlagFile is used by snapshot images as
// an example.
func CreateFlagFile(dir string,
	filename string, msg pb.Marshaler, fs vfs.IFS) (err error) {
	fp := fs.PathJoin(dir, filename)
	f, err := fs.Create(fp)
	if err != nil {
		return err
	}
	defer func() {
		err = firstError(err, ws(f.Close()))
		err = firstError(err, SyncDir(dir, fs))
	}()
	data := pb.MustMarshal(msg)
	h := getHash(data)
	n, err := f.Write(h)
	if err != nil {
		return ws(err)
	}
	if n != len(h) {
		return ws(io.ErrShortWrite)
	}
	n, err = f.Write(data)
	if err != nil {
		return ws(err)
	}
	if n != len(data) {
		return ws(io.ErrShortWrite)
	}
	return ws(f.Sync())
}

// GetFlagFileContent gets the content of the flag file found in the specified
// location. The data of the flag file will be unmarshaled into the specified
// protobuf message.
func GetFlagFileContent(dir string,
	filename string, msg pb.Unmarshaler, fs vfs.IFS) (err error) {
	fp := fs.PathJoin(dir, filename)
	f, err := fs.Open(vfs.Clean(fp))
	if err != nil {
		return err
	}
	defer func() {
		err = firstError(err, ws(f.Close()))
	}()
	data, err := ReadAll(f)
	if err != nil {
		return ws(err)
	}
	if len(data) < 8 {
		panic("corrupted flag file")
	}
	h := data[:8]
	buf := data[8:]
	expectedHash := getHash(buf)
	if !bytes.Equal(h, expectedHash) {
		panic("corrupted flag file content")
	}
	pb.MustUnmarshal(msg, buf)
	return nil
}

// HasFlagFile returns a boolean value indicating whether flag file can be
// found in the specified location.
func HasFlagFile(dir string, filename string, fs vfs.IFS) bool {
	fp := fs.PathJoin(dir, filename)
	fi, err := fs.Stat(fp)
	if err != nil {
		return false
	}
	if fi.IsDir() {
		return false
	}
	return true
}

// RemoveFlagFile removes the specified flag file.
func RemoveFlagFile(dir string, filename string, fs vfs.IFS) error {
	return fs.Remove(fs.PathJoin(dir, filename))
}

// ExtractTarBz2 extracts files and directories from the specified tar.bz2 file
// to the specified target directory.
func ExtractTarBz2(bz2fn string, toDir string, fs vfs.IFS) (err error) {
	f, err := fs.Open(bz2fn)
	if err != nil {
		return err
	}
	defer func() {
		err = firstError(err, f.Close())
	}()
	ts := bzip2.NewReader(f)
	tarReader := tar.NewReader(ts)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			target := fs.PathJoin(toDir, header.Name)
			if err := fs.MkdirAll(target, defaultDirFileMode); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := func() error {
				fp := fs.PathJoin(toDir, header.Name)
				nf, err := fs.Create(fp)
				if err != nil {
					return err
				}
				defer func() {
					err = firstError(err, nf.Close())
				}()
				_, err = io.Copy(nf, tarReader)
				return err
			}(); err != nil {
				return err
			}
		default:
			panic("unknown type")
		}
	}
}

// TempFile and the following rand functions are derived from the golang source
// https://golang.org/src/io/ioutil/tempfile.go
//
// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
var rand uint32
var randmu sync.Mutex

func reseed() uint32 {
	return uint32(time.Now().UnixNano())
}

func nextRandom() string {
	randmu.Lock()
	r := rand
	if r == 0 {
		r = reseed()
	}
	r = r*1664525 + 1013904223 // constants from Numerical Recipes
	rand = r
	randmu.Unlock()
	return strconv.Itoa(int(1e9 + r%1e9))[1:]
}

// TempFile and TempDir functions below are modified from golang's
// TempFile and TempDir functions.
//
// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// TempFile returns a temp file.
func TempFile(dir,
	pattern string, fs vfs.IFS) (f vfs.File, name string, err error) {
	if dir == "" {
		dir = vfs.TempDir()
		if fs != vfs.DefaultFS {
			if err := fs.MkdirAll(dir, defaultDirFileMode); err != nil {
				return nil, "", err
			}
		}
	}
	var prefix, suffix string
	if pos := strings.LastIndex(pattern, "*"); pos != -1 {
		prefix, suffix = pattern[:pos], pattern[pos+1:]
	} else {
		prefix = pattern
	}
	nconflict := 0
	for i := 0; i < 10000; i++ {
		name = fs.PathJoin(dir, prefix+nextRandom()+suffix)
		f, err = fs.Create(name)
		if vfs.IsExist(err) {
			if nconflict++; nconflict > 10 {
				randmu.Lock()
				rand = reseed()
				randmu.Unlock()
			}
			continue
		}
		break
	}
	return
}

var errPatternHasSeparator = errors.New("pattern contains path separator")

// prefixAndSuffix splits pattern by the last wildcard "*", if applicable,
// returning prefix as the part before "*" and suffix as the part after "*".
func prefixAndSuffix(pattern string) (prefix, suffix string, err error) {
	if strings.ContainsRune(pattern, os.PathSeparator) {
		err = errPatternHasSeparator
		return
	}
	if pos := strings.LastIndex(pattern, "*"); pos != -1 {
		prefix, suffix = pattern[:pos], pattern[pos+1:]
	} else {
		prefix = pattern
	}
	return
}

// TempDir creates a new temporary directory in the directory dir.
// The directory name is generated by taking pattern and applying a
// random string to the end. If pattern includes a "*", the random string
// replaces the last "*". TempDir returns the name of the new directory.
// If dir is the empty string, TempDir uses the
// default directory for temporary files (see os.TempDir).
// Multiple programs calling TempDir simultaneously
// will not choose the same directory. It is the caller's responsibility
// to remove the directory when no longer needed.
func TempDir(dir, pattern string, fs vfs.IFS) (name string, err error) {
	if dir == "" {
		dir = os.TempDir()
	}

	prefix, suffix, err := prefixAndSuffix(pattern)
	if err != nil {
		return
	}

	nconflict := 0
	for i := 0; i < 10000; i++ {
		try := fs.PathJoin(dir, prefix+nextRandom()+suffix)
		err = fs.MkdirAll(try, 0700)
		if oserror.IsExist(err) {
			if nconflict++; nconflict > 10 {
				randmu.Lock()
				rand = reseed()
				randmu.Unlock()
			}
			continue
		}
		if oserror.IsNotExist(err) {
			if _, err := fs.Stat(dir); oserror.IsNotExist(err) {
				return "", err
			}
		}
		if err == nil {
			name = try
		}
		break
	}
	return
}
