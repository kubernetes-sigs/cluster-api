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
	"log"

	"github.com/google/go-cmp/cmp"
)

func Check(actual interface{}, expected interface{}) (result bool, err error) {
	defer func() error {
		if recover() != nil {
			log.Printf("panic occured got %v expected %v", actual, expected)
			err = fmt.Errorf("panic occured got %v expected %v", actual, expected)
		}
		return nil
	}()
	diff := cmp.Diff(actual, expected)
	if diff == "" {
		return true, err
	}
	log.Printf("mismatch of objects actual %v expected %v diff %v", actual, expected, diff)
	return false, err
}
