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

//go:build amd64 && !nosimd

package vectormap

import (
	"math/bits"
	_ "unsafe"

	"github.com/zuoyebang/bitalostored/butils/vectormap/simd"
)

const (
	groupSize       = 16
	maxAvgGroupLoad = 14
)

type bitset uint16

func metaMatchH2(m *metadata, h h2) bitset {
	b := simd.MatchMetadata((*[16]int8)(m), int8(h))
	return bitset(b)
}

func metaMatchEmpty(m *metadata) bitset {
	b := simd.MatchMetadata((*[16]int8)(m), empty)
	return bitset(b)
}

func nextMatch(b *bitset) (s uint32) {
	s = uint32(bits.TrailingZeros16(uint16(*b)))
	*b &= ^(1 << s) // clear bit |s|
	return
}
