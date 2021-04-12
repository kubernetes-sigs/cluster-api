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

package noderefutil

import (
	"testing"

	. "github.com/onsi/gomega"
)

const aws = "aws"

func TestNewProviderID(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		expectedID string
	}{
		{
			name:       "2 slashes after colon, one segment",
			input:      "aws://instance-id",
			expectedID: "instance-id",
		},
		{
			name:       "more than 2 slashes after colon, one segment",
			input:      "aws:////instance-id",
			expectedID: "instance-id",
		},
		{
			name:       "multiple filled-in segments (aws format)",
			input:      "aws:///zone/instance-id",
			expectedID: "instance-id",
		},
		{
			name:       "multiple filled-in segments",
			input:      "aws://bar/baz/instance-id",
			expectedID: "instance-id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			id, err := NewProviderID(tc.input)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(id.CloudProvider()).To(Equal(aws))
			g.Expect(id.ID()).To(Equal(tc.expectedID))
		})
	}
}

func TestInvalidProviderID(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		err   error
	}{
		{
			name:  "empty id",
			input: "",
			err:   ErrEmptyProviderID,
		},
		{
			name:  "only empty segments",
			input: "aws:///////",
			err:   ErrInvalidProviderID,
		},
		{
			name:  "missing cloud provider",
			input: "://instance-id",
			err:   ErrInvalidProviderID,
		},
		{
			name:  "missing cloud provider and colon",
			input: "//instance-id",
			err:   ErrInvalidProviderID,
		},
		{
			name:  "missing cloud provider, colon, one leading slash",
			input: "/instance-id",
			err:   ErrInvalidProviderID,
		},
		{
			name:  "just an id",
			input: "instance-id",
			err:   ErrInvalidProviderID,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)

			_, err := NewProviderID(test.input)
			g.Expect(err).To(MatchError(test.err))
		})
	}
}

func TestProviderIDEquals(t *testing.T) {
	g := NewWithT(t)

	input1 := "aws:////instance-id1"
	parsed1, err := NewProviderID(input1)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(parsed1.String()).To(Equal(input1))
	g.Expect(parsed1.ID()).To(Equal("instance-id1"))
	g.Expect(parsed1.CloudProvider()).To(Equal(aws))

	input2 := "aws:///us-west-1/instance-id1"
	parsed2, err := NewProviderID(input2)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(parsed2.String()).To(Equal(input2))
	g.Expect(parsed2.ID()).To(Equal("instance-id1"))
	g.Expect(parsed2.CloudProvider()).To(Equal(aws))

	g.Expect(parsed1.Equals(parsed2)).To(BeTrue())
}
