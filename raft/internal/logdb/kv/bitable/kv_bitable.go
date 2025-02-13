// Copyright 2017-2021 Lei Ni (nilei81@gmail.com), Bitalostored author and other contributors.
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

package bitable

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"github.com/lni/goutils/syncutil"
	bitable "github.com/zuoyebang/bitalostable"
	"github.com/zuoyebang/bitalostored/raft/config"
	"github.com/zuoyebang/bitalostored/raft/internal/fileutil"
	"github.com/zuoyebang/bitalostored/raft/internal/logdb/kv"
	"github.com/zuoyebang/bitalostored/raft/internal/utils"
	"github.com/zuoyebang/bitalostored/raft/internal/vfs"
	"github.com/zuoyebang/bitalostored/raft/logger"
)

const bitableLogTag = "[bitable/raftlog]"

var plog = logger.GetLogger("bitablekv")

var firstError = utils.FirstError

type eventListener struct {
	kv      *KV
	stopper *syncutil.Stopper
}

func (l *eventListener) close() {
	l.stopper.Stop()
}

func (l *eventListener) notify() {
	l.stopper.RunWorker(func() {
		select {
		case <-l.kv.dbSet:
			if l.kv.callback != nil {
				memSizeThreshold := l.kv.config.KVWriteBufferSize *
					l.kv.config.KVMaxWriteBufferNumber * 19 / 20
				l0FileNumThreshold := l.kv.config.KVLevel0StopWritesTrigger - 1
				m := l.kv.db.Metrics()
				busy := m.MemTable.Size >= memSizeThreshold ||
					uint64(m.Levels[0].Sublevels) >= l0FileNumThreshold
				l.kv.callback(busy)
			}
		default:
		}
	})
}

func (l *eventListener) onCompactionEnd(info bitable.CompactionInfo) {
	plog.Infof("%s %s", bitableLogTag, info)
	l.notify()
}

func (l *eventListener) onFlushEnd(info bitable.FlushInfo) {
	plog.Infof("%s %s", bitableLogTag, info)
	l.notify()
}

func (l *eventListener) onWALCreated(bitable.WALCreateInfo) {
	l.notify()
}

type bitableWriteBatch struct {
	wb *bitable.Batch
	db *bitable.DB
	wo *bitable.WriteOptions
}

func (w *bitableWriteBatch) Destroy() {
	if err := w.wb.Close(); err != nil {
		panic(err)
	}
}

func (w *bitableWriteBatch) Put(key []byte, val []byte) {
	if err := w.wb.Set(key, val, w.wo); err != nil {
		panic(err)
	}
}

func (w *bitableWriteBatch) Delete(key []byte) {
	if err := w.wb.Delete(key, w.wo); err != nil {
		panic(err)
	}
}

func (w *bitableWriteBatch) Clear() {
	if err := w.wb.Close(); err != nil {
		panic(err)
	}
	w.wb = w.db.NewBatch()
}

func (w *bitableWriteBatch) Count() int {
	return int(w.wb.Count())
}

var _ bitable.Logger = (*bitableLogger)(nil)

type bitableLogger struct{}

func (l bitableLogger) Info(args ...interface{}) {
	plog.Infof(fmt.Sprint(args...))
}

func (l bitableLogger) Warn(args ...interface{}) {
	plog.Warningf(fmt.Sprint(args...))
}

func (l bitableLogger) Error(args ...interface{}) {
	plog.Errorf(fmt.Sprint(args...))
}

func (l bitableLogger) Cost(args ...interface{}) func() {
	begin := time.Now()
	return func() {
		plog.Infof(fmt.Sprint(fmt.Sprint(args...), " ", fmtDuration(time.Now().Sub(begin))))
	}
}

func (l bitableLogger) Warnf(format string, args ...interface{}) {
	plog.Warningf(format, args...)
}

func (l bitableLogger) Errorf(format string, args ...interface{}) {
	plog.Errorf(format, args...)
}

func (bitableLogger) Infof(format string, args ...interface{}) {
	plog.Infof(format, args...)
}

func (bitableLogger) Fatalf(format string, args ...interface{}) {
	plog.Warningf(format, args...)
}

func fmtDuration(d time.Duration) string {
	if d > time.Second {
		return fmt.Sprintf("cost:%d.%03ds", d/time.Second, d/time.Millisecond%1000)
	}
	if d > time.Millisecond {
		return fmt.Sprintf("cost:%d.%03dms", d/time.Millisecond, d/time.Microsecond%1000)
	}
	if d > time.Microsecond {
		return fmt.Sprintf("cost:%d.%03dus", d/time.Microsecond, d%1000)
	}
	return fmt.Sprintf("cost:%dns", d)
}

// NewKVStore returns a bitable based IKVStore instance.
func NewKVStore(config config.LogDBConfig, callback kv.LogDBCallback,
	dir string, wal string, fs vfs.IFS) (kv.IKVStore, error) {
	return openBitableDB(config, callback, dir, wal, fs)
}

// KV is a bitable based IKVStore type.
type KV struct {
	db       *bitable.DB
	dbSet    chan struct{}
	opts     *bitable.Options
	ro       *bitable.IterOptions
	wo       *bitable.WriteOptions
	event    *eventListener
	callback kv.LogDBCallback
	config   config.LogDBConfig
}

var _ kv.IKVStore = (*KV)(nil)

var bitableWarning sync.Once

func openBitableDB(config config.LogDBConfig, callback kv.LogDBCallback,
	dir string, walDir string, fs vfs.IFS) (kv.IKVStore, error) {
	if config.IsEmpty() {
		panic("invalid LogDBConfig")
	}
	bitableWarning.Do(func() {
		if fs == vfs.MemStrictFS {
			plog.Warningf("running in bitable memfs test mode")
		}
	})
	//blockSize := int(config.KVBlockSize)
	blockSize := 128 << 10
	writeBufferSize := 128 << 20
	targetFileSizeBase := int64(128 << 20)
	//cacheSize := int64(config.KVLRUCacheSize)
	cacheSize := int64(0)
	//levelSizeMultiplier := int64(config.KVTargetFileSizeMultiplier)
	levelSizeMultiplier := int64(2)
	//numOfLevels := int64(config.KVNumOfLevels)
	numOfLevels := int64(7)
	lopts := make([]bitable.LevelOptions, 0)
	sz := targetFileSizeBase
	for l := int64(0); l < numOfLevels; l++ {
		opt := bitable.LevelOptions{
			Compression:    bitable.SnappyCompression,
			BlockSize:      blockSize,
			TargetFileSize: sz,
		}
		sz = sz * levelSizeMultiplier
		lopts = append(lopts, opt)
	}
	if inMonkeyTesting {
		writeBufferSize = 4 << 20
	}
	cache := bitable.NewCache(cacheSize)
	ro := &bitable.IterOptions{}
	wo := &bitable.WriteOptions{Sync: false}
	opts := &bitable.Options{
		Levels:                      lopts,
		MaxManifestFileSize:         128 << 20,
		MemTableSize:                writeBufferSize,
		MemTableStopWritesThreshold: 8,
		LBaseMaxBytes:               1 << 30,
		L0CompactionThreshold:       48,
		L0StopWritesThreshold:       96,
		Cache:                       cache,
		Logger:                      bitableLogger{},
		LogTag:                      bitableLogTag,
		MaxOpenFiles:                8000,
	}
	if fs != vfs.DefaultFS {
		opts.FS = vfs.NewBitableFS(fs)
	}
	kv := &KV{
		ro:       ro,
		wo:       wo,
		opts:     opts,
		config:   config,
		callback: callback,
		dbSet:    make(chan struct{}),
	}
	event := &eventListener{
		kv:      kv,
		stopper: syncutil.NewStopper(),
	}
	opts.EventListener = bitable.EventListener{
		WALCreated:    event.onWALCreated,
		FlushEnd:      event.onFlushEnd,
		CompactionEnd: event.onCompactionEnd,
	}
	if len(walDir) > 0 {
		if err := fileutil.MkdirAll(walDir, fs); err != nil {
			return nil, err
		}
		opts.WALDir = walDir
	}
	if err := fileutil.MkdirAll(dir, fs); err != nil {
		return nil, err
	}
	pdb, err := bitable.Open(dir, opts)
	if err != nil {
		return nil, err
	}
	cache.Unref()
	kv.db = pdb
	kv.setEventListener(event)
	plog.Infof("bitable open success MemTableSize:%d MemTableStopWritesThreshold:%d MaxManifestFileSize:%d L0StopWritesThreshold:%d",
		opts.MemTableSize,
		opts.MemTableStopWritesThreshold,
		opts.MaxManifestFileSize,
		opts.L0StopWritesThreshold)
	return kv, nil
}

func (r *KV) setEventListener(event *eventListener) {
	if r.db == nil || r.event != nil {
		panic("unexpected kv state")
	}
	r.event = event
	close(r.dbSet)
	// force a WALCreated event as the one issued when opening the DB didn't get
	// handled
	event.onWALCreated(bitable.WALCreateInfo{})
}

// Name returns the IKVStore type name.
func (r *KV) Name() string {
	return "pebble"
}

// Close closes the RDB object.
func (r *KV) Close() error {
	if err := r.db.Close(); err != nil {
		return err
	}
	r.event.close()
	return nil
}

func iteratorIsValid(iter *bitable.Iterator) bool {
	v := iter.Valid()
	if err := iter.Error(); err != nil {
		plog.Panicf("%+v", err)
	}
	return v
}

// IterateValue ...
func (r *KV) IterateValue(fk []byte, lk []byte, inc bool,
	op func(key []byte, data []byte) (bool, error)) (err error) {
	iter := r.db.NewIter(r.ro)
	defer func() {
		err = firstError(err, iter.Close())
	}()
	for iter.SeekGE(fk); iteratorIsValid(iter); iter.Next() {
		key := iter.Key()
		val := iter.Value()
		if inc {
			if bytes.Compare(key, lk) > 0 {
				return nil
			}
		} else {
			if bytes.Compare(key, lk) >= 0 {
				return nil
			}
		}
		cont, err := op(key, val)
		if err != nil {
			return err
		}
		if !cont {
			break
		}
	}
	return nil
}

// GetValue ...
func (r *KV) GetValue(key []byte, op func([]byte) error) (err error) {
	val, closer, err := r.db.Get(key)
	if err != nil && err != bitable.ErrNotFound {
		return err
	}
	defer func() {
		if closer != nil {
			err = firstError(err, closer.Close())
		}
	}()
	return op(val)
}

// SaveValue ...
func (r *KV) SaveValue(key []byte, value []byte) error {
	return r.db.Set(key, value, r.wo)
}

// DeleteValue ...
func (r *KV) DeleteValue(key []byte) error {
	return r.db.Delete(key, r.wo)
}

// GetWriteBatch ...
func (r *KV) GetWriteBatch() kv.IWriteBatch {
	return &bitableWriteBatch{
		wb: r.db.NewBatch(),
		db: r.db,
		wo: r.wo,
	}
}

// CommitWriteBatch ...
func (r *KV) CommitWriteBatch(wb kv.IWriteBatch) error {
	pwb, ok := wb.(*bitableWriteBatch)
	if !ok {
		panic("unknown type")
	}
	if pwb.db != r.db {
		panic("pwb.db != r.db")
	}
	return r.db.Apply(pwb.wb, r.wo)
}

// BulkRemoveEntries ...
func (r *KV) BulkRemoveEntries(fk []byte, lk []byte) (err error) {
	wb := r.db.NewBatch()
	defer func() {
		err = firstError(err, wb.Close())
	}()
	if err := wb.DeleteRange(fk, lk, r.wo); err != nil {
		return err
	}
	return r.db.Apply(wb, r.wo)
}

// CompactEntries ...
func (r *KV) CompactEntries(fk []byte, lk []byte) error {
	return r.db.Compact(fk, lk, false)
}

// FullCompaction ...
func (r *KV) FullCompaction() error {
	fk := make([]byte, kv.MaxKeyLength)
	lk := make([]byte, kv.MaxKeyLength)
	for i := uint64(0); i < kv.MaxKeyLength; i++ {
		fk[i] = 0
		lk[i] = 0xFF
	}
	return r.db.Compact(fk, lk, false)
}
