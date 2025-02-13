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
	"bytes"
	"math"
	"strconv"
	"strings"

	"github.com/zuoyebang/bitalostored/butils/extend"
	"github.com/zuoyebang/bitalostored/butils/unsafe2"
	"github.com/zuoyebang/bitalostored/stored/engine/bitsdb/btools"
	"github.com/zuoyebang/bitalostored/stored/internal/errn"
	"github.com/zuoyebang/bitalostored/stored/internal/resp"
	"github.com/zuoyebang/bitalostored/stored/internal/utils"
)

func init() {
	AddCommand(map[string]*Cmd{
		resp.ZADD:             {Sync: resp.IsWriteCmd(resp.ZADD), Handler: zaddCommand},
		resp.ZINCRBY:          {Sync: resp.IsWriteCmd(resp.ZINCRBY), Handler: zincrbyCommand},
		resp.ZREM:             {Sync: resp.IsWriteCmd(resp.ZREM), Handler: zremCommand},
		resp.ZREMRANGEBYSCORE: {Sync: resp.IsWriteCmd(resp.ZREMRANGEBYSCORE), Handler: zremrangebyscoreCommand},
		resp.ZREMRANGEBYRANK:  {Sync: resp.IsWriteCmd(resp.ZREMRANGEBYRANK), Handler: zremrangebyrankCommand},
		resp.ZREMRANGEBYLEX:   {Sync: resp.IsWriteCmd(resp.ZREMRANGEBYLEX), Handler: zremrangebylexCommand},
		resp.ZRANGE:           {Sync: resp.IsWriteCmd(resp.ZRANGE), Handler: zrangeCommand},
		resp.ZREVRANGE:        {Sync: resp.IsWriteCmd(resp.ZREVRANGE), Handler: zrevrangeCommand},
		resp.ZRANGEBYLEX:      {Sync: resp.IsWriteCmd(resp.ZRANGEBYLEX), Handler: zrangebylexCommand},
		resp.ZRANGEBYSCORE:    {Sync: resp.IsWriteCmd(resp.ZRANGEBYSCORE), Handler: zrangebyscoreCommand},
		resp.ZREVRANGEBYSCORE: {Sync: resp.IsWriteCmd(resp.ZREVRANGEBYSCORE), Handler: zrevrangebyscoreCommand},
		resp.ZRANK:            {Sync: resp.IsWriteCmd(resp.ZRANK), Handler: zrankCommand},
		resp.ZREVRANK:         {Sync: resp.IsWriteCmd(resp.ZREVRANK), Handler: zrevrankCommand},
		resp.ZSCORE:           {Sync: resp.IsWriteCmd(resp.ZSCORE), Handler: zscoreCommand},
		resp.ZLEXCOUNT:        {Sync: resp.IsWriteCmd(resp.ZLEXCOUNT), Handler: zlexcountCommand},
		resp.ZCOUNT:           {Sync: resp.IsWriteCmd(resp.ZCOUNT), Handler: zcountCommand},
		resp.ZCARD:            {Sync: resp.IsWriteCmd(resp.ZCARD), Handler: zcardCommand},
		resp.ZCLEAR:           {Sync: resp.IsWriteCmd(resp.ZCLEAR), Handler: zclearCommand, KeySkip: 1},
		resp.ZKEYEXISTS:       {Sync: resp.IsWriteCmd(resp.ZKEYEXISTS), Handler: zkeyexistsCommand},
		resp.ZEXPIRE:          {Sync: resp.IsWriteCmd(resp.ZEXPIRE), Handler: zexpireCommand},
		resp.ZEXPIREAT:        {Sync: resp.IsWriteCmd(resp.ZEXPIREAT), Handler: zexpireAtCommand},
		resp.ZTTL:             {Sync: resp.IsWriteCmd(resp.ZTTL), Handler: zttlCommand},
		resp.ZPERSIST:         {Sync: resp.IsWriteCmd(resp.ZPERSIST), Handler: zpersistCommand},
	})
}

func zaddCommand(c *Client) error {
	args := c.Args
	if len(args) < 3 {
		return errn.CmdParamsErr(resp.ZADD)
	}

	if len(args[1:])&1 != 0 {
		return errn.CmdParamsErr(resp.ZADD)
	}

	key := args[0]
	args = args[1:]

	params := make([]btools.ScorePair, len(args)>>1)
	for i := 0; i < len(params); i++ {

		score, err := extend.ParseFloat64(unsafe2.String(args[2*i]))
		if err != nil || score < float64(math.MinInt64) || score > float64(math.MaxInt64) {
			return errn.ErrValue
		}

		params[i].Score = score
		params[i].Member = args[2*i+1]
	}

	n, err := c.DB.ZAdd(key, c.KeyHash, params...)

	if err == nil {
		c.Writer.WriteInteger(n)
	}

	return err
}

func zincrbyCommand(c *Client) error {
	args := c.Args
	if len(args) != 3 {
		return errn.CmdParamsErr(resp.ZINCRBY)
	}

	delta, err := extend.ParseFloat64(unsafe2.String(args[1]))
	if err != nil {
		return errn.ErrValue
	}

	key := args[0]

	v, err := c.DB.ZIncrBy(key, c.KeyHash, delta, args[2])

	if err == nil {
		c.Writer.WriteBulk(extend.FormatFloat64ToSlice(v))
	}

	return err
}

func zremCommand(c *Client) error {
	args := c.Args
	if len(args) < 2 {
		return errn.CmdParamsErr(resp.ZREM)
	}

	n, err := c.DB.ZRem(args[0], c.KeyHash, args[1:]...)

	if err == nil {
		c.Writer.WriteInteger(n)
	}

	return err
}

func zremrangebyscoreCommand(c *Client) error {
	args := c.Args
	if len(args) != 3 {
		return errn.CmdParamsErr(resp.ZREMRANGEBYSCORE)
	}

	min, max, leftClose, rightClose, err := zparseScoreRange(args[1], args[2])
	if err != nil {
		return err
	}

	key := args[0]

	n, err := c.DB.ZRemRangeByScore(key, c.KeyHash, min, max, leftClose, rightClose)

	if err == nil {
		c.Writer.WriteInteger(n)
	}

	return err
}

func zremrangebyrankCommand(c *Client) error {
	args := c.Args
	if len(args) != 3 {
		return errn.CmdParamsErr(resp.ZREMRANGEBYRANK)
	}

	start, stop, err := zparseRange(args[1], args[2])
	if err != nil {
		return errn.ErrValue
	}

	key := args[0]
	n, err := c.DB.ZRemRangeByRank(key, c.KeyHash, start, stop)

	if err == nil {
		c.Writer.WriteInteger(n)
	}

	return err
}

func zremrangebylexCommand(c *Client) error {
	args := c.Args
	if len(args) != 3 {
		return errn.CmdParamsErr(resp.ZREMRANGEBYLEX)
	}

	min, max, leftClose, rightClose, err := zparseLexMemberRange(args[1], args[2])
	if err != nil {
		return err
	}

	key := args[0]

	if n, err := c.DB.ZRemRangeByLex(key, c.KeyHash, min, max, leftClose, rightClose); err != nil {
		return err
	} else {
		c.Writer.WriteInteger(n)
	}

	return nil
}

func zparseRange(a1 []byte, a2 []byte) (start int64, stop int64, err error) {
	if start, err = strconv.ParseInt(unsafe2.String(a1), 10, 64); err != nil {
		return
	}

	if stop, err = strconv.ParseInt(unsafe2.String(a2), 10, 64); err != nil {
		return
	}

	return
}

func zrangeGeneric(c *Client, reverse bool, cmd string) error {
	args := c.Args
	if len(args) < 3 {
		return errn.CmdParamsErr(resp.ZRANGE)
	}

	key := args[0]

	start, stop, err := zparseRange(args[1], args[2])
	if err != nil {
		return errn.ErrValue
	}

	args = args[3:]
	var withScores bool = false

	if len(args) > 0 {
		if len(args) != 1 {
			return errn.CmdParamsErr(cmd)
		}
		if strings.ToLower(unsafe2.String(args[0])) == "withscores" {
			withScores = true
		} else {
			return errn.ErrSyntax
		}
	}

	if datas, err := c.DB.ZRangeGeneric(key, c.KeyHash, start, stop, reverse); err != nil {
		return err
	} else {
		c.Writer.WriteScorePairArray(datas, withScores)
	}
	return nil
}

func zrangeCommand(c *Client) error {
	return zrangeGeneric(c, false, resp.ZRANGE)
}

func zrevrangeCommand(c *Client) error {
	return zrangeGeneric(c, true, resp.ZREVRANGE)
}

func zrangebylexCommand(c *Client) error {
	args := c.Args
	if len(args) != 3 && len(args) != 6 {
		return errn.CmdParamsErr(resp.ZRANGEBYLEX)
	}

	min, max, leftClose, rightClose, err := zparseLexMemberRange(args[1], args[2])
	if err != nil {
		return err
	}

	var offset int = 0
	var count int = -1

	if len(args) == 6 {
		if strings.ToLower(unsafe2.String(args[3])) != "limit" {
			return errn.ErrSyntax
		}

		if offset, err = strconv.Atoi(unsafe2.String(args[4])); err != nil {
			return errn.ErrValue
		}

		if offset < 0 {
			c.Writer.WriteSliceArray(make([][]byte, 0, 4))
			return nil
		}

		if count, err = strconv.Atoi(unsafe2.String(args[5])); err != nil {
			return errn.ErrValue
		}
	}

	key := args[0]

	if ay, err := c.DB.ZRangeByLex(key, c.KeyHash, min, max, leftClose, rightClose, offset, count); err != nil {
		return err
	} else {
		c.Writer.WriteSliceArray(ay)
	}

	return nil
}

func zrangebyscoreGeneric(c *Client, reverse bool) error {
	args := c.Args
	if len(args) < 3 {
		return errn.CmdParamsErr(resp.ZRANGEBYSCORE)
	}

	key := args[0]

	var minScore, maxScore []byte

	if !reverse {
		minScore, maxScore = args[1], args[2]
	} else {
		minScore, maxScore = args[2], args[1]
	}

	min, max, leftClose, rightClose, err := zparseScoreRange(minScore, maxScore)

	if err != nil {
		return err
	}

	args = args[3:]

	var withScores bool = false

	if len(args) > 0 {
		if strings.ToLower(unsafe2.String(args[0])) == "withscores" {
			withScores = true
			args = args[1:]
		}
	}

	var offset int = 0
	var count int = -1

	if len(args) > 0 {
		if len(args) < 3 {
			return errn.CmdParamsErr(resp.ZRANGEBYSCORE)
		}

		if strings.ToLower(unsafe2.String(args[0])) != "limit" {
			return errn.ErrSyntax
		}

		if offset, err = strconv.Atoi(unsafe2.String(args[1])); err != nil {
			return errn.ErrValue
		}

		if count, err = strconv.Atoi(unsafe2.String(args[2])); err != nil {
			return errn.ErrValue
		}

		if len(args) == 4 {
			if strings.ToLower(unsafe2.String(args[3])) == "withscores" {
				withScores = true
			}
		}
	}

	if offset < 0 {
		c.Writer.WriteArray([]interface{}{})
		return nil
	}

	if datas, err := c.DB.ZRangeByScoreGeneric(key, c.KeyHash, min, max, leftClose, rightClose, offset, count, reverse); err != nil {
		return err
	} else {
		c.Writer.WriteScorePairArray(datas, withScores)
	}

	return nil
}

func zrangebyscoreCommand(c *Client) error {
	return zrangebyscoreGeneric(c, false)
}

func zrevrangebyscoreCommand(c *Client) error {
	return zrangebyscoreGeneric(c, true)
}

func zrankCommand(c *Client) error {
	args := c.Args
	if len(args) != 2 {
		return errn.CmdParamsErr(resp.ZRANK)
	}
	if n, err := c.DB.ZRank(args[0], c.KeyHash, args[1]); err != nil {
		if err == errn.ErrZsetMemberNil {
			c.Writer.WriteBulk(nil)
		} else {
			return err
		}
	} else if n == -1 {
		c.Writer.WriteBulk(nil)
	} else {
		c.Writer.WriteInteger(n)
	}

	return nil
}

func zrevrankCommand(c *Client) error {
	args := c.Args
	if len(args) != 2 {
		return errn.CmdParamsErr(resp.ZREVRANK)
	}

	if n, err := c.DB.ZRevRank(args[0], c.KeyHash, args[1]); err != nil {
		if err == errn.ErrZsetMemberNil {
			c.Writer.WriteBulk(nil)
		} else {
			return err
		}
	} else if n == -1 {
		c.Writer.WriteBulk(nil)
	} else {
		c.Writer.WriteInteger(n)
	}

	return nil
}

func zscoreCommand(c *Client) error {
	args := c.Args
	if len(args) != 2 {
		return errn.CmdParamsErr(resp.ZSCORE)
	}

	if s, err := c.DB.ZScore(args[0], c.KeyHash, args[1]); err != nil {
		if err == errn.ErrZsetMemberNil {
			c.Writer.WriteBulk(nil)
		} else {
			return err
		}
	} else {
		c.Writer.WriteBulk(extend.FormatFloat64ToSlice(s))
	}

	return nil
}

func zlexcountCommand(c *Client) error {
	args := c.Args
	if len(args) != 3 {
		return errn.CmdParamsErr(resp.ZLEXCOUNT)
	}

	min, max, leftClose, rightClose, err := zparseLexMemberRange(args[1], args[2])
	if err != nil {
		return err
	}

	key := args[0]

	if n, err := c.DB.ZLexCount(key, c.KeyHash, min, max, leftClose, rightClose); err != nil {
		return err
	} else {
		c.Writer.WriteInteger(n)
	}

	return nil
}

func zcountCommand(c *Client) error {
	args := c.Args
	if len(args) != 3 {
		return errn.CmdParamsErr(resp.ZCOUNT)
	}

	min, max, leftClose, rightClose, err := zparseScoreRange(args[1], args[2])

	if err != nil {
		return errn.ErrValue
	}

	if min > max {
		c.Writer.WriteInteger(0)
		return nil
	}

	if n, err := c.DB.ZCount(args[0], c.KeyHash, min, max, leftClose, rightClose); err != nil {
		return err
	} else {
		c.Writer.WriteInteger(n)
	}

	return nil
}

func zcardCommand(c *Client) error {
	args := c.Args
	if len(args) != 1 {
		return errn.CmdParamsErr(resp.ZCARD)
	}

	if n, err := c.DB.ZCard(args[0], c.KeyHash); err != nil {
		return err
	} else {
		c.Writer.WriteInteger(n)
	}

	return nil
}

func zkeyexistsCommand(c *Client) error {
	args := c.Args
	if len(args) != 1 {
		return errn.CmdParamsErr(resp.ZKEYEXISTS)
	}

	if n, err := c.DB.Exists(args[0], c.KeyHash); err != nil {
		return err
	} else {
		c.Writer.WriteInteger(n)
	}
	return nil
}

func zclearCommand(c *Client) error {
	args := c.Args
	if len(args) < 1 {
		return errn.CmdParamsErr(resp.ZCLEAR)
	}

	n, err := c.DB.ZClear(c.KeyHash, args...)

	if err == nil {
		c.Writer.WriteInteger(n)
	}

	return err
}

func zexpireCommand(c *Client) error {
	args := c.Args
	if len(args) != 2 {
		return errn.CmdParamsErr(resp.ZEXPIRE)
	}

	duration, err := utils.ByteToInt64(args[1])
	if err != nil {
		return errn.ErrValue
	}

	var n int64
	n, err = c.DB.Expire(args[0], c.KeyHash, duration)
	if err != nil {
		return err
	}
	c.Writer.WriteInteger(n)
	return nil
}

func zexpireAtCommand(c *Client) error {
	args := c.Args
	if len(args) != 2 {
		return errn.CmdParamsErr(resp.ZEXPIREAT)
	}

	when, err := utils.ByteToInt64(args[1])
	if err != nil {
		return errn.ErrValue
	}

	var n int64
	n, err = c.DB.ExpireAt(args[0], c.KeyHash, when)
	if err != nil {
		return err
	}
	c.Writer.WriteInteger(n)
	return nil
}

func zttlCommand(c *Client) error {
	args := c.Args
	if len(args) != 1 {
		return errn.CmdParamsErr(resp.ZTTL)
	}

	if v, err := c.DB.TTl(args[0], c.KeyHash); err != nil {
		return err
	} else {
		c.Writer.WriteInteger(v)
	}

	return nil
}

func zpersistCommand(c *Client) error {
	args := c.Args
	if len(args) != 1 {
		return errn.CmdParamsErr(resp.ZPERSIST)
	}

	n, err := c.DB.Persist(args[0], c.KeyHash)

	if err == nil {
		c.Writer.WriteInteger(n)
	}

	return err
}

func zparseLexMemberRange(minBuf []byte, maxBuf []byte) (min []byte, max []byte, leftClose bool, rightClose bool, err error) {
	if bytes.Equal(minBuf, []byte{'-'}) {
		min = minBuf
	} else {
		if len(minBuf) == 0 {
			err = errn.ErrInvalidRangeItem
			return
		}

		if minBuf[0] == '(' {
			leftClose = true
			min = minBuf[1:]
		} else if minBuf[0] == '[' {
			min = minBuf[1:]
		} else {
			err = errn.ErrInvalidRangeItem
			return
		}
	}

	if bytes.Equal(maxBuf, []byte{'+'}) {
		max = maxBuf
	} else {
		if len(maxBuf) == 0 {
			err = errn.ErrInvalidRangeItem
			return
		}
		if maxBuf[0] == '(' {
			rightClose = true
			max = maxBuf[1:]
		} else if maxBuf[0] == '[' {
			max = maxBuf[1:]
		} else {
			err = errn.ErrInvalidRangeItem
			return
		}
	}
	return
}

func zparseScoreRange(minBuf []byte, maxBuf []byte) (minFloat64 float64, maxFloat64 float64, leftClose bool, rightClose bool, err error) {
	if strings.ToLower(unsafe2.String(minBuf)) == "-inf" {
		minFloat64 = -math.MaxFloat64
	} else {
		if len(minBuf) == 0 {
			return minFloat64, maxFloat64, leftClose, rightClose, errn.ErrZSetScoreRange
		}

		if minBuf[0] == '(' {
			leftClose = true
			minBuf = minBuf[1:]
		}

		minFloat64, err = extend.ParseFloat64(unsafe2.String(minBuf))
		if err != nil {
			return 0, 0, leftClose, rightClose, errn.ErrZSetScoreRange
		}
	}

	if strings.ToLower(unsafe2.String(maxBuf)) == "+inf" {
		maxFloat64 = math.MaxFloat64
	} else {
		if len(maxBuf) == 0 {
			return minFloat64, maxFloat64, leftClose, rightClose, errn.ErrZSetScoreRange
		}
		if maxBuf[0] == '(' {
			rightClose = true
			maxBuf = maxBuf[1:]
		}

		maxFloat64, err = extend.ParseFloat64(unsafe2.String(maxBuf))
		if err != nil {
			return minFloat64, maxFloat64, leftClose, rightClose, errn.ErrZSetScoreRange
		}
	}
	return
}
