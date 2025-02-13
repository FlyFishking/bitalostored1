// Copyright 2017-2020 Lei Ni (nilei81@gmail.com) and other contributors.
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

//go:build dragonboat_memfs_test
// +build dragonboat_memfs_test

package logdb

import (
	"github.com/zuoyebang/bitalostored/raft/config"
	"github.com/zuoyebang/bitalostored/raft/internal/logdb/kv"
	"github.com/zuoyebang/bitalostored/raft/internal/logdb/kv/bitable"
	"github.com/zuoyebang/bitalostored/raft/internal/vfs"
)

func newDefaultKVStore(config config.LogDBConfig,
	callback kv.LogDBCallback,
	dir string, wal string, fs vfs.IFS) (kv.IKVStore, error) {
	if fs == nil {
		panic("nil fs")
	}
	if _, ok := fs.(*vfs.MemFS); !ok {
		if _, ok := fs.(*vfs.ErrorFS); !ok {
			panic("invalid fs")
		}
	}
	return bitable.NewKVStore(config, callback, dir, wal, fs)
}
