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
	"fmt"
	"strings"

	sutil "github.com/zuoyebang/bitalostored/stored/internal/glob/util/strings"
)

type SuffixAny struct {
	Suffix     string
	Separators []rune
}

func NewSuffixAny(s string, sep []rune) SuffixAny {
	return SuffixAny{s, sep}
}

func (self SuffixAny) Index(s string) (int, []int) {
	idx := strings.Index(s, self.Suffix)
	if idx == -1 {
		return -1, nil
	}

	i := sutil.LastIndexAnyRunes(s[:idx], self.Separators) + 1

	return i, []int{idx + len(self.Suffix) - i}
}

func (self SuffixAny) Len() int {
	return lenNo
}

func (self SuffixAny) Match(s string) bool {
	if !strings.HasSuffix(s, self.Suffix) {
		return false
	}
	return sutil.IndexAnyRunes(s[:len(s)-len(self.Suffix)], self.Separators) == -1
}

func (self SuffixAny) String() string {
	return fmt.Sprintf("<suffix_any:![%s]%s>", string(self.Separators), self.Suffix)
}
