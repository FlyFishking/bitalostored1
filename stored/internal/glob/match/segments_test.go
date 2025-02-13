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

package match

import (
	"sync"
	"testing"
)

func benchPool(i int, b *testing.B) {
	pool := sync.Pool{New: func() interface{} {
		return make([]int, 0, i)
	}}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s := pool.Get().([]int)[:0]
			pool.Put(s)
		}
	})
}

func benchMake(i int, b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = make([]int, 0, i)
		}
	})
}

func BenchmarkSegmentsPool_1(b *testing.B) {
	benchPool(1, b)
}
func BenchmarkSegmentsPool_2(b *testing.B) {
	benchPool(2, b)
}
func BenchmarkSegmentsPool_4(b *testing.B) {
	benchPool(4, b)
}
func BenchmarkSegmentsPool_8(b *testing.B) {
	benchPool(8, b)
}
func BenchmarkSegmentsPool_16(b *testing.B) {
	benchPool(16, b)
}
func BenchmarkSegmentsPool_32(b *testing.B) {
	benchPool(32, b)
}
func BenchmarkSegmentsPool_64(b *testing.B) {
	benchPool(64, b)
}
func BenchmarkSegmentsPool_128(b *testing.B) {
	benchPool(128, b)
}
func BenchmarkSegmentsPool_256(b *testing.B) {
	benchPool(256, b)
}

func BenchmarkSegmentsMake_1(b *testing.B) {
	benchMake(1, b)
}
func BenchmarkSegmentsMake_2(b *testing.B) {
	benchMake(2, b)
}
func BenchmarkSegmentsMake_4(b *testing.B) {
	benchMake(4, b)
}
func BenchmarkSegmentsMake_8(b *testing.B) {
	benchMake(8, b)
}
func BenchmarkSegmentsMake_16(b *testing.B) {
	benchMake(16, b)
}
func BenchmarkSegmentsMake_32(b *testing.B) {
	benchMake(32, b)
}
func BenchmarkSegmentsMake_64(b *testing.B) {
	benchMake(64, b)
}
func BenchmarkSegmentsMake_128(b *testing.B) {
	benchMake(128, b)
}
func BenchmarkSegmentsMake_256(b *testing.B) {
	benchMake(256, b)
}
