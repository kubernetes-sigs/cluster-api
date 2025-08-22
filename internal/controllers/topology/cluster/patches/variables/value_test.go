/*
Copyright 2021 The Kubernetes Authors.

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

package variables

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
)

func TestParsePathSegment(t *testing.T) {
	tests := []struct {
		name            string
		segment         string
		wantPathSegment *pathSegment
		wantErr         bool
	}{
		{
			name:    "parse basic segment",
			segment: "propertyName",
			wantPathSegment: &pathSegment{
				path:  "propertyName",
				index: nil,
			},
		},
		{
			name:    "parse segment with index",
			segment: "arrayProperty[5]",
			wantPathSegment: &pathSegment{
				path:  "arrayProperty",
				index: ptr.To(5),
			},
		},
		{
			name:    "fail invalid syntax: only left delimiter",
			segment: "arrayProperty[",
			wantErr: true,
		},
		{
			name:    "fail invalid syntax: only right delimiter",
			segment: "arrayProperty]",
			wantErr: true,
		},
		{
			name:    "fail invalid syntax: both delimiter but no index",
			segment: "arrayProperty[]",
			wantErr: true,
		},
		{
			name:    "fail invalid syntax: negative index",
			segment: "arrayProperty[-1]",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := parsePathSegment(tt.segment)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(Equal(tt.wantPathSegment))
		})
	}
}
