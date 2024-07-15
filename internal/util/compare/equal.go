/*
Copyright 2024 The Kubernetes Authors.

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

// Package compare contains utils to compare objects.
package compare

import (
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
)

// Diff uses cmp.Diff to compare x and y.
// If cmp.Diff panics, Equal returns an error.
func Diff(x any, y any) (equal bool, diff string, matchErr error) {
	defer func() {
		if r := recover(); r != nil {
			equal = false
			if err, ok := r.(error); ok {
				matchErr = errors.Wrapf(err, "error diffing objects")
				return
			}

			if errMsg, ok := r.(string); ok {
				matchErr = errors.Errorf("error diffing objects: %s", errMsg)
				return
			}

			matchErr = errors.Errorf("error diffing objects: panic of unknown type %T", r)
		}
	}()

	diff = cmp.Diff(x, y)

	if diff != "" {
		// Replace non-breaking space (NBSP) through a regular space.
		// This prevents output like this: "\u00a0\u00a0int(\n-\u00a0\t1,\n+\u00a0\t2,\n\u00a0\u00a0)\n"
		diff = strings.ReplaceAll(diff, "\u00a0", " ")
		// Replace \t through "  " because it's easier to read in log output
		diff = strings.ReplaceAll(diff, "\t", "  ")
		diff = strings.TrimSpace(diff)
	}

	return diff == "", diff, nil
}
