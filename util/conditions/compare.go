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

package conditions

import (
	"fmt"

	"github.com/google/go-cmp/cmp"
)

// Check compares the actual and expected values provided using the
// cmp package and handles panic errors in case of comparison.
func Check(actual interface{}, expected interface{}) (result bool, diff string, err error) {
	result = true
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("panic occurred got %v expected %v", actual, expected)
		}
	}()
	diff = cmp.Diff(actual, expected)
	if err != nil {
		return !result, "", err
	}
	if diff != "" {
		return !result, diff, nil
	}
	return result, diff, nil
}
