// Copyright © 2017 The Kubicorn Authors
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

package compare

import (
	"testing"
)

func TestCompare(t *testing.T) {
	tt := []struct {
		name     string
		actual   interface{}
		expected interface{}
		isEqual  bool
	}{
		{"a equals a", "a", "a", true},
		{"a not equals b", "a", "b", false},
		{"bool equals bool", true, true, true},
		{"bool not equals bool", true, false, false},
		{"1 equals 1", 1, 1, true},
		{"1 not equals 1", 1, 0, false},
		{"slice equals slice", []string{"one", "two", "three"}, []string{"one", "two", "three"}, true},
		{"slice not equals slice", []string{"one", "two", "three"}, []string{"one", "four", "three"}, false},
		{"map equals map", map[string]int{"one": 1, "two": 2}, map[string]int{"one": 1, "two": 2}, true},
		{"map not equals map", map[string]int{"one": 1, "two": 2}, map[string]int{"one": 1, "two": 3}, false},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			isEqual, _ := IsEqual(tc.actual, tc.expected)
			if isEqual != tc.isEqual {
				t.Fatalf("%v should be %v got %v\n", tc.name, tc.isEqual, isEqual)
			}
		})
	}

}
