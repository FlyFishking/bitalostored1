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

package cmd_test

import (
	"time"

	"github.com/gomodule/redigo/redis"
)

func init() {
	addr := "127.0.0.1:8950"
	initRedisPool(addr, 150)
	cacheEable := false
	if cacheEable {
		readNum = 2
	}
	skipTx = true
}

var skipTx bool
var readNum int = 1
var redisPool *redis.Pool

type RedisConnConf struct {
	HostPort     string        `toml:"host_port" json:"host_port,omitempty"`
	MaxIdle      int           `toml:"max_idle" json:"max_idle"`
	MaxActive    int           `toml:"max_active" json:"max_active"`
	IdleTimeout  time.Duration `toml:"idle_timeout" json:"idle_timeout"`
	Password     string        `toml:"password" json:"password"`
	DataBase     int           `toml:"database" json:"database"`
	ConnTimeout  time.Duration `toml:"conn_timeout" json:"conn_timeout"`
	ReadTimeout  time.Duration `toml:"read_timeout" json:"read_timeout"`
	WriteTimeout time.Duration `toml:"write_timeout" json:"write_timeout"`
}

func getPool(conf RedisConnConf) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     conf.MaxIdle,
		MaxActive:   conf.MaxActive,
		IdleTimeout: conf.IdleTimeout,
		Wait:        true,
		Dial: func() (conn redis.Conn, e error) {
			conn, err := redis.Dial("tcp", conf.HostPort,
				redis.DialPassword(conf.Password),
				redis.DialDatabase(conf.DataBase),
				redis.DialConnectTimeout(conf.ConnTimeout),
				redis.DialReadTimeout(conf.ReadTimeout),
				redis.DialWriteTimeout(conf.WriteTimeout),
			)
			if err != nil {
				return nil, err
			}
			return conn, nil
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := c.Do("PING")
			return err
		},
	}
}

func initRedisPool(addr string, cnum int) {
	conf := RedisConnConf{
		HostPort:     addr,
		MaxIdle:      cnum,
		MaxActive:    cnum,
		IdleTimeout:  1 * time.Hour,
		Password:     "",
		DataBase:     0,
		ConnTimeout:  1 * time.Second,
		ReadTimeout:  1 * time.Second,
		WriteTimeout: 1 * time.Second,
	}

	redisPool = getPool(conf)
}

func getTestConn() redis.Conn {
	return redisPool.Get()
}
