/*
Copyright 2023 The Kubernetes Authors.

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

// Package hash provides utils to calculate hashes.
package hash

import (
	"fmt"
	"hash/fnv"

	"github.com/davecgh/go-spew/spew"
)

// Compute computes the hash of an object using the spew library.
// Note: spew follows pointers and prints actual values of the nested objects
// ensuring the hash does not change when a pointer changes.
func Compute(obj interface{}) (uint32, error) {
	hasher := fnv.New32a()

	printer := spew.ConfigState{
		Indent:         " ",
		SortKeys:       true,
		DisableMethods: true,
		SpewKeys:       true,
	}

	if _, err := printer.Fprintf(hasher, "%#v", obj); err != nil {
		return 0, fmt.Errorf("failed to calculate hash")
	}

	return hasher.Sum32(), nil
}
