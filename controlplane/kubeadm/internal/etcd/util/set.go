/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Modified copy of k8s.io/apimachinery/pkg/util/sets/int64.go
// Modifications
//   - int64 became uint64
//   - Empty type is added to this file and changed to a non exported symbol

package util

import (
	"sort"
)

type empty struct{}

// util.UInt64Set is a set of uint64s, implemented via map[uint64]struct{} for minimal memory consumption.
type UInt64Set map[uint64]empty

// NewUInt64Set creates a UInt64Set from a list of values.
func NewUInt64Set(items ...uint64) UInt64Set {
	ss := UInt64Set{}
	ss.Insert(items...)
	return ss
}

// Insert adds items to the set.
func (s UInt64Set) Insert(items ...uint64) UInt64Set {
	for _, item := range items {
		s[item] = empty{}
	}
	return s
}

// Delete removes all items from the set.
func (s UInt64Set) Delete(items ...uint64) UInt64Set {
	for _, item := range items {
		delete(s, item)
	}
	return s
}

// Has returns true if and only if item is contained in the set.
func (s UInt64Set) Has(item uint64) bool {
	_, contained := s[item]
	return contained
}

// HasAll returns true if and only if all items are contained in the set.
func (s UInt64Set) HasAll(items ...uint64) bool {
	for _, item := range items {
		if !s.Has(item) {
			return false
		}
	}
	return true
}

// HasAny returns true if any items are contained in the set.
func (s UInt64Set) HasAny(items ...uint64) bool {
	for _, item := range items {
		if s.Has(item) {
			return true
		}
	}
	return false
}

// Difference returns a set of objects that are not in s2
// For example:
// s1 = {a1, a2, a3}
// s2 = {a1, a2, a4, a5}
// s1.Difference(s2) = {a3}
// s2.Difference(s1) = {a4, a5}
func (s UInt64Set) Difference(s2 UInt64Set) UInt64Set {
	result := NewUInt64Set()
	for key := range s {
		if !s2.Has(key) {
			result.Insert(key)
		}
	}
	return result
}

// Union returns a new set which includes items in either s1 or s2.
// For example:
// s1 = {a1, a2}
// s2 = {a3, a4}
// s1.Union(s2) = {a1, a2, a3, a4}
// s2.Union(s1) = {a1, a2, a3, a4}
func (s UInt64Set) Union(s2 UInt64Set) UInt64Set {
	s1 := s
	result := NewUInt64Set()
	for key := range s1 {
		result.Insert(key)
	}
	for key := range s2 {
		result.Insert(key)
	}
	return result
}

// Intersection returns a new set which includes the item in BOTH s1 and s2
// For example:
// s1 = {a1, a2}
// s2 = {a2, a3}
// s1.Intersection(s2) = {a2}
func (s UInt64Set) Intersection(s2 UInt64Set) UInt64Set {
	s1 := s
	var walk, other UInt64Set
	result := NewUInt64Set()
	if s1.Len() < s2.Len() {
		walk = s1
		other = s2
	} else {
		walk = s2
		other = s1
	}
	for key := range walk {
		if other.Has(key) {
			result.Insert(key)
		}
	}
	return result
}

// IsSuperset returns true if and only if s1 is a superset of s2.
func (s UInt64Set) IsSuperset(s2 UInt64Set) bool {
	s1 := s
	for item := range s2 {
		if !s1.Has(item) {
			return false
		}
	}
	return true
}

// Equal returns true if and only if s1 is equal (as a set) to s2.
// Two sets are equal if their membership is identical.
// (In practice, this means same elements, order doesn't matter)
func (s UInt64Set) Equal(s2 UInt64Set) bool {
	s1 := s
	return len(s1) == len(s2) && s1.IsSuperset(s2)
}

type sortableSliceOfUInt64 []uint64

func (s sortableSliceOfUInt64) Len() int           { return len(s) }
func (s sortableSliceOfUInt64) Less(i, j int) bool { return lessUInt64(s[i], s[j]) }
func (s sortableSliceOfUInt64) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// List returns the contents as a sorted uint64 slice.
func (s UInt64Set) List() []uint64 {
	res := make(sortableSliceOfUInt64, 0, len(s))
	for key := range s {
		res = append(res, key)
	}
	sort.Sort(res)
	return []uint64(res)
}

// UnsortedList returns the slice with contents in random order.
func (s UInt64Set) UnsortedList() []uint64 {
	res := make([]uint64, 0, len(s))
	for key := range s {
		res = append(res, key)
	}
	return res
}

// Returns a single element from the set.
func (s UInt64Set) PopAny() (uint64, bool) {
	for key := range s {
		s.Delete(key)
		return key, true
	}
	var zeroValue uint64
	return zeroValue, false
}

// Len returns the size of the set.
func (s UInt64Set) Len() int {
	return len(s)
}

func lessUInt64(lhs, rhs uint64) bool {
	return lhs < rhs
}
