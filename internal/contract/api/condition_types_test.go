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

package api

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestConditionsUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     []byte
		expectErr bool
		expect    Conditions
	}{
		{
			name:   "empty array unmarshals to no conditions",
			input:  []byte(`[]`),
			expect: nil,
		},
		{
			name:      "invalid JSON returns an error",
			input:     []byte(`not-json`),
			expectErr: true,
		},
		{
			name: "keeps only the Ready condition, discards others",
			input: []byte(`[
				{"type":"Available","status":"True"},
				{"type":"Ready","status":"True","reason":"AllGood","message":"all good"},
				{"type":"Degraded","status":"False"}
			]`),
			expect: Conditions{
				{Type: "Ready", Status: "True", Reason: "AllGood", Message: "all good"},
			},
		},
		{
			name:   "no Ready condition present unmarshals to no conditions",
			input:  []byte(`[{"type":"Available","status":"True"}]`),
			expect: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			var got Conditions
			err := got.UnmarshalJSON(tt.input)

			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.expect))
		})
	}
}

func TestV1Beta1ConditionsConditionsUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     []byte
		expectErr bool
		expect    V1Beta1Conditions
	}{
		{
			name:   "empty array unmarshals to no conditions",
			input:  []byte(`[]`),
			expect: nil,
		},
		{
			name:      "invalid JSON returns an error",
			input:     []byte(`not-json`),
			expectErr: true,
		},
		{
			name: "keeps only the Ready condition, discards others",
			input: []byte(`[
				{"type":"Available","status":"True"},
				{"type":"Ready","status":"True","reason":"AllGood","message":"all good"},
				{"type":"Degraded","status":"False"}
			]`),
			expect: V1Beta1Conditions{
				{Type: "Ready", Status: "True", Reason: "AllGood", Message: "all good"},
			},
		},
		{
			name:   "no Ready condition present unmarshals to no conditions",
			input:  []byte(`[{"type":"Available","status":"True"}]`),
			expect: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			var got V1Beta1Conditions
			err := got.UnmarshalJSON(tt.input)

			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.expect))
		})
	}
}
