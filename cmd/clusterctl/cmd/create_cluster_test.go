/*
Copyright 2018 The Kubernetes Authors.

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

package cmd

import (
	"testing"
)

func TestGetProvider(t *testing.T) {
	var testcases = []struct {
		provider  string
		expectErr bool
	}{
		{
			provider:  "blah blah",
			expectErr: true,
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.provider, func(t *testing.T) {
			_, err := getProvider(testcase.provider)
			if (testcase.expectErr && err == nil) || (!testcase.expectErr && err != nil) {
				t.Fatalf("Unexpected returned error. Got: %v, Want Err: %v", err, testcase.expectErr)
			}
		})
	}
}
