// Copyright 2019-2024 Xu Ruibo (hustxurb@163.com) and Contributors
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

package server

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zuoyebang/bitalostored/butils/hash"
	"github.com/zuoyebang/bitalostored/butils/unsafe2"
	"github.com/zuoyebang/bitalostored/stored/engine"
	"github.com/zuoyebang/bitalostored/stored/internal/config"
	"github.com/zuoyebang/bitalostored/stored/internal/errn"
	"github.com/zuoyebang/bitalostored/stored/internal/log"
	"github.com/zuoyebang/bitalostored/stored/internal/resp"
	"github.com/zuoyebang/bitalostored/stored/internal/utils"
)

const (
	TxStateNone    int = 0
	TxStateWatch   int = 0x1
	TxStateMulti   int = 0x2
	TxStatePrepare int = 0x4
)

const (
	PrepareStateNone = iota
	PrepareStateKeyModified
	PrepareStateLockFail
	PrepareStateLocked
	PrepareStateUnlock
)

var raftClientPool sync.Pool

type Client struct {
	Cmd            string
	Args           [][]byte
	Keys           []byte
	Data           [][]byte
	ParseMarks     []int
	Reader         *resp.Reader
	Writer         *resp.Writer
	DB             *engine.Bitalos
	QueryStartTime time.Time
	KeyHash        uint32
	IsMaster       func() bool

	server            *Server
	remoteAddr        string
	closed            atomic.Bool
	txState           int
	txCommandQueued   bool
	watchKeys         map[string]int64
	commandQueue      [][][]byte
	hasPrepareLock    atomic.Bool
	prepareState      atomic.Int32
	prepareUnlockSig  chan struct{}
	queueCommandDone  chan struct{}
	prepareUnlockDone chan struct{}
}

func init() {
	raftClientPool = sync.Pool{
		New: func() interface{} {
			return newRaftClient()
		},
	}
	for i := 0; i < 128; i++ {
		raftClientPool.Put(raftClientPool.New())
	}
}

func GetRaftClientFromPool(s *Server, data [][]byte, keyHash uint32) *Client {
	c := raftClientPool.Get().(*Client)
	c.FormatData(data)
	c.DB = s.GetDB()
	c.server = s
	c.IsMaster = s.IsMaster
	c.KeyHash = keyHash
	return c
}

func GetVmFromPool(s *Server) *Client {
	c := raftClientPool.Get().(*Client)
	c.DB = s.GetDB()
	c.server = s
	c.IsMaster = s.IsMaster
	return c
}

func PutRaftClientToPool(c *Client) {
	c.Writer.Reset()
	raftClientPool.Put(c)
}

func newRaftClient() *Client {
	return &Client{
		Writer: resp.NewWriter(),
	}
}

func newConnClient(s *Server, remoteAddr string) *Client {
	c := &Client{
		DB:         s.GetDB(),
		IsMaster:   s.IsMaster,
		ParseMarks: make([]int, 0, 1<<4),
		Reader:     resp.NewReader(),
		Writer:     resp.NewWriter(),
		remoteAddr: remoteAddr,
		server:     s,
	}

	s.Info.Client.ClientTotal.Add(1)
	s.Info.Client.ClientAlive.Add(1)

	if s.openDistributedTx {
		c.prepareUnlockSig = make(chan struct{}, 1)
		c.queueCommandDone = make(chan struct{}, 1)
		c.prepareUnlockDone = make(chan struct{}, 1)
	}

	return c
}

func (c *Client) Close() {
	if !c.closed.CompareAndSwap(false, true) {
		return
	}

	if c.server.openDistributedTx {
		c.discard()
	}

	c.server.Info.Client.ClientAlive.Add(-1)
}

func (c *Client) ResetQueryStartTime() {
	c.QueryStartTime = time.Now()
}

func (c *Client) FormatData(reqData [][]byte) {
	c.ResetQueryStartTime()
	c.Data = reqData
	c.Cmd = ""
	if len(reqData) == 0 {
		c.Args = reqData[0:0]
	} else {
		c.Cmd = unsafe2.String(LowerSlice(reqData[0]))
		c.Args = reqData[1:]
		if len(c.Args) > 0 {
			c.Keys = c.Args[0]
		} else {
			c.Keys = c.Keys[0:0]
		}
	}
}

func (c *Client) HandleRequest(reqData [][]byte, isHashTag bool) (err error) {
	c.FormatData(reqData)

	if len(c.Cmd) == 0 {
		err = errn.CmdEmptyErr(c.Cmd)
		c.Writer.WriteError(err)
		return err
	}

	if c.server.openDistributedTx && c.checkCommandEnterQueue() {
		txReqData := make([][]byte, len(reqData))
		for i := range reqData {
			txReqData[i] = append([]byte{}, reqData[i]...)
		}
		c.commandQueue = append(c.commandQueue, txReqData)
		c.Writer.WriteStatus(resp.ReplyQUEUED)
		return nil
	}

	if c.server.isDebug {
		if c.Cmd != "info" && c.Cmd != "dbconfig" {
			tmpArgs := make([]string, 0, len(c.Args)+1)
			tmpArgs = append(tmpArgs, c.Cmd)
			for i := range c.Args {
				tmpArgs = append(tmpArgs, unsafe2.String(c.Args[i]))
			}
			log.Debug("command : ", tmpArgs)
		}
	}

	if c.Cmd == "script" {
		if len(c.Args) < 1 {
			err = errn.CmdParamsErr(c.Cmd)
			c.Writer.WriteError(err)
			return err
		}
		c.Cmd = c.Cmd + unsafe2.String(LowerSlice(c.Args[0]))
	}

	if c.Cmd == "QUIT" {
		c.Writer.WriteStatus(resp.ReplyOK)
		return errn.ErrClientQuit
	}

	if !c.checkCommand() {
		c.Writer.WriteBulk(nil)
		return nil
	}

	var ok bool
	var execCmd *Cmd

	if execCmd, ok = commands[c.Cmd]; !ok {
		err = errn.CmdEmptyErr(c.Cmd)
		c.Writer.WriteError(err)
		return err
	}
	if c.server.openDistributedTx && c.txState&TxStateMulti != 0 && execCmd.NotAllowedInTx {
		err = fmt.Errorf("ERR %s inside MULTI is not allowed", c.Cmd)
		c.Writer.WriteError(err)
		return err
	}
	if c.server.IsWitness {
		err = c.ApplyDB(0)
		if err != nil {
			c.Writer.WriteError(err)
		}
		return err
	}

	if c.server.isOpenRaft && c.server.slowQuery != nil && c.server.slowQuery.CheckSlowShield(c.Cmd, c.Keys) {
		c.Writer.WriteError(errn.ErrSlowShield)
		return errn.ErrSlowShield
	}

	if !isHashTag {
		c.KeyHash = hash.Fnv32(c.Keys)
	} else {
		c.KeyHash = utils.GetHashTagFnv(c.Keys)
	}

	var isRedirect bool
	var lockFunc func()

	if isRedirect, lockFunc = c.DB.CheckRedirectAndLockFunc(c.Cmd, c.Keys, c.KeyHash); lockFunc != nil {
		defer lockFunc()
	}

	if isRedirect {
		var updateKeyModifyTs func()
		if c.server.openDistributedTx {
			updateKeyModifyTs = c.markWatchKeyModified(execCmd)
		}
		err = c.DB.Redirect(c.Cmd, c.Keys, reqData, c.Writer)
		if updateKeyModifyTs != nil {
			updateKeyModifyTs()
		}
	} else if c.server.isOpenRaft && execCmd.Sync && !config.GlobalConfig.CheckIsDegradeSingleNode() {
		err = c.RaftSync()
	} else {
		err = c.ApplyDB(0)
	}
	if err != nil {
		c.Writer.WriteError(err)
	}
	return err
}

func (c *Client) RaftSync() error {
	start := time.Now()
	resData, err := c.server.DoRaftSync(c.KeyHash, c.Data)
	if err != nil {
		return err
	}

	if resData == nil {
		return c.ApplyDB(time.Since(start).Nanoseconds())
	} else {
		c.Writer.WriteBytes(resData)
		return nil
	}
}

func (c *Client) ApplyDB(raftSyncCostNs int64) error {
	var err error
	var ok bool
	var execCmd *Cmd

	if execCmd, ok = commands[c.Cmd]; !ok {
		err = errn.CmdEmptyErr(c.Cmd)
		return err
	}

	var updateKeyModifyTs func()
	if c.server.openDistributedTx {
		updateKeyModifyTs = c.markWatchKeyModified(execCmd)
	}

	if err = execCmd.Handler(c); err != nil {
		if updateKeyModifyTs != nil {
			updateKeyModifyTs()
		}
		return err
	}
	if updateKeyModifyTs != nil {
		updateKeyModifyTs()
	}

	c.server.Info.Stats.TotolCmd.Add(1)

	costNs := time.Since(c.QueryStartTime).Nanoseconds()
	if costNs >= config.GlobalConfig.Server.SlowTime.Int64() {
		if c.server.slowQuery != nil {
			c.server.slowQuery.Send(c.Cmd, c.Keys, costNs-raftSyncCostNs)
		}
		costUs := costNs / 1000
		raftSyncCostUs := raftSyncCostNs / 1000
		log.SlowLog(c.remoteAddr, costUs, raftSyncCostUs, c.Data, err)
	}
	return err
}

func (c *Client) GetInfo() *SInfo {
	return c.server.Info
}

func (c *Client) checkCommand() bool {
	if !c.server.IsWitness {
		return true
	}

	switch c.Cmd {
	case resp.INFO:
		return true
	case resp.PING:
		return true
	case resp.ECHO:
		return true
	case resp.SHUTDOWN:
		return true
	default:
		return false
	}
}

func (c *Client) checkCommandEnterQueue() bool {
	if c.txCommandQueued {
		switch c.Cmd {
		case resp.WATCH, resp.UNWATCH, resp.MULTI, resp.PREPARE, resp.EXEC, resp.DISCARD:
			return false
		default:
			return true
		}
	}
	return false
}

func (c *Client) markWatchKeyModified(execCmd *Cmd) func() {
	if execCmd == nil {
		return nil
	}
	if !(c.IsMaster() && execCmd.Sync && !execCmd.NotAllowedInTx && !execCmd.NoKey) {
		return nil
	}

	modifyFuncs := make([]func(), 0, 1)
	argNum := len(c.Args)
	firstPos := 0
	khash := uint32(0)

	addMofidyFunc := func(keyByte []byte, khash uint32) {
		wk := c.server.txLocks.GetWatchKeyWithKhash(khash, unsafe2.String(keyByte))
		if wk == nil {
			return
		}
		if !wk.watched.Load() {
			return
		}

		if c.hasPrepareLock.Load() {
			modifyFuncs = append(modifyFuncs, func() {
				wk.modifyTs.Store(c.QueryStartTime.UnixNano())
			})
		} else {
			wk.mu.Lock()
			modifyFuncs = append(modifyFuncs, func() {
				wk.modifyTs.Store(c.QueryStartTime.UnixNano())
				wk.mu.Unlock()
			})
		}
	}

	for pos := firstPos; pos < argNum; pos += int(execCmd.KeySkip) {
		if pos == 0 {
			khash = c.KeyHash
		} else {
			khash = hash.Fnv32(c.Args[pos])
		}
		addMofidyFunc(c.Args[pos], khash)
		if execCmd.KeySkip == 0 {
			break
		}
	}
	return func() {
		l := len(modifyFuncs)
		for i := l - 1; i >= 0; i-- {
			modifyFuncs[i]()
		}
	}
}

func (c *Client) enableCommandQueued() {
	c.txCommandQueued = true
}

func (c *Client) disableCommandQueued() {
	c.txCommandQueued = false
}

func (c *Client) resetTx() {
	if c.txState&TxStateMulti != 0 {
		c.server.txParallelCounter.Add(-1)
	}
	c.txState = TxStateNone
	c.disableCommandQueued()
	c.commandQueue = nil
	c.watchKeys = nil
	c.hasPrepareLock.Store(false)
}

func (c *Client) addWatchKey(txLock *TxLocker, key []byte, ts time.Time) {
	keyStr := string(key)
	txLock.addWatchKey(c, keyStr, true)

	if len(c.watchKeys) == 0 {
		c.watchKeys = make(map[string]int64, 10)
	}
	if _, ok := c.watchKeys[keyStr]; !ok {
		c.watchKeys[keyStr] = ts.UnixNano()
	}
}

func (c *Client) unwatchKey() {
	for key := range c.watchKeys {
		txLock := c.server.txLocks.GetTxLockByKey(unsafe2.ByteSlice(key))
		txLock.removeWatchKey(c, key)
	}
	c.watchKeys = nil
}

func (c *Client) discard() {
	if c.txState == TxStateNone {
		return
	}

	if c.txState&TxStatePrepare != 0 {
		prepareState := c.prepareState.Load()
		switch prepareState {
		case PrepareStateNone:
			c.unwatchKey()
			c.resetTx()
		case PrepareStateLocked:
			c.prepareUnlockSig <- struct{}{}
			c.queueCommandDone <- struct{}{}
			<-c.prepareUnlockDone
		}
		return
	} else if c.txState&TxStateWatch != 0 {
		c.unwatchKey()
	}
	c.resetTx()
}
