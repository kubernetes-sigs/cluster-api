/*
Copyright 2019 The Kubernetes Authors.

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

package cloudinit

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestTemplateYAMLIndent(t *testing.T) {
	testcases := []struct {
		name     string
		input    string
		indent   int
		expected string
	}{
		{
			name:     "simple case",
			input:    "hello\n  world",
			indent:   2,
			expected: "  hello\n    world",
		},
		{
			name:     "more indent",
			input:    "  some extra:\n    indenting\n",
			indent:   4,
			expected: "      some extra:\n        indenting\n    ",
		},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(templateYAMLIndent(tc.indent, tc.input)).To(Equal(tc.expected))
		})
	}
}
