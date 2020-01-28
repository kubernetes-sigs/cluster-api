/*
Copyright The Kubernetes Authors.

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

package util

import (
	"reflect"
	"sort"
)

type Empty struct{}

// sets.UInt64 is a set of int64s, implemented via map[uint64]struct{} for minimal memory consumption.
type UInt64 map[uint64]Empty

// NewUInt64 creates a UInt64 from a list of values.
func NewUInt64(items ...uint64) UInt64 {
	ss := UInt64{}
	ss.Insert(items...)
	return ss
}

// Int64KeySet creates a UInt64 from a keys of a map[uint64](? extends interface{}).
// If the value passed in is not actually a map, this will panic.
func Int64KeySet(theMap interface{}) UInt64 {
	v := reflect.ValueOf(theMap)
	ret := UInt64{}

	for _, keyValue := range v.MapKeys() {
		ret.Insert(keyValue.Interface().(uint64))
	}
	return ret
}

// Insert adds items to the set.
func (s UInt64) Insert(items ...uint64) UInt64 {
	for _, item := range items {
		s[item] = Empty{}
	}
	return s
}

// Delete removes all items from the set.
func (s UInt64) Delete(items ...uint64) UInt64 {
	for _, item := range items {
		delete(s, item)
	}
	return s
}

// Has returns true if and only if item is contained in the set.
func (s UInt64) Has(item uint64) bool {
	_, contained := s[item]
	return contained
}

// HasAll returns true if and only if all items are contained in the set.
func (s UInt64) HasAll(items ...uint64) bool {
	for _, item := range items {
		if !s.Has(item) {
			return false
		}
	}
	return true
}

// HasAny returns true if any items are contained in the set.
func (s UInt64) HasAny(items ...uint64) bool {
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
func (s UInt64) Difference(s2 UInt64) UInt64 {
	result := NewUInt64()
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
func (s1 UInt64) Union(s2 UInt64) UInt64 {
	result := NewUInt64()
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
func (s1 UInt64) Intersection(s2 UInt64) UInt64 {
	var walk, other UInt64
	result := NewUInt64()
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
func (s1 UInt64) IsSuperset(s2 UInt64) bool {
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
func (s1 UInt64) Equal(s2 UInt64) bool {
	return len(s1) == len(s2) && s1.IsSuperset(s2)
}

type sortableSliceOfInt64 []uint64

func (s sortableSliceOfInt64) Len() int           { return len(s) }
func (s sortableSliceOfInt64) Less(i, j int) bool { return lessInt64(s[i], s[j]) }
func (s sortableSliceOfInt64) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// List returns the contents as a sorted uint64 slice.
func (s UInt64) List() []uint64 {
	res := make(sortableSliceOfInt64, 0, len(s))
	for key := range s {
		res = append(res, key)
	}
	sort.Sort(res)
	return []uint64(res)
}

// UnsortedList returns the slice with contents in random order.
func (s UInt64) UnsortedList() []uint64 {
	res := make([]uint64, 0, len(s))
	for key := range s {
		res = append(res, key)
	}
	return res
}

// Returns a single element from the set.
func (s UInt64) PopAny() (uint64, bool) {
	for key := range s {
		s.Delete(key)
		return key, true
	}
	var zeroValue uint64
	return zeroValue, false
}

// Len returns the size of the set.
func (s UInt64) Len() int {
	return len(s)
}

func lessInt64(lhs, rhs uint64) bool {
	return lhs < rhs
}
