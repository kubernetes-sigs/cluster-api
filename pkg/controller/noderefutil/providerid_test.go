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
	"strings"
	"testing"

	"github.com/pkg/errors"
)

func TestNewProviderID(t *testing.T) {
	input1 := "aws:////instance-id"
	_, err := NewProviderID(input1)
	if err != nil {
		t.Fatalf("Expected no errors, got %v", err)
	}
}

func TestInvalidProviderID(t *testing.T) {
	testCases := []struct {
		input string
		err   error
	}{
		{
			input: "aws:///////",
			err:   ErrInvalidProviderID,
		},
		{
			input: ":///instance-id",
			err:   errors.New("missing protocol scheme"),
		},
		{
			input: "///instance-id",
			err:   ErrInvalidProviderID,
		},
		{
			input: "/instance-id",
			err:   ErrInvalidProviderID,
		},
		{
			input: "instance-id",
			err:   ErrInvalidProviderID,
		},
		{
			input: "#IAmTheSenate",
			err:   ErrInvalidProviderID,
		},
	}

	for _, test := range testCases {
		_, err := NewProviderID(test.input)
		if !strings.Contains(err.Error(), test.err.Error()) {
			t.Fatalf("Expected error %v, got %v", test.err, err)
		}
	}
}

func TestProviderIDEquals(t *testing.T) {
	input1 := "aws:////instance-id1"
	parsed1, err := NewProviderID(input1)
	if err != nil {
		t.Fatalf("Expected no errors, got %v", err)
	}

	if parsed1.String() != input1 {
		t.Fatalf("Expected String output to match original input %q, got %q", input1, parsed1.String())
	}

	if parsed1.ID() != "instance-id1" {
		t.Fatalf("Expected valid ID, got %v", parsed1.ID())
	}

	if parsed1.CloudProvider() != "aws" {
		t.Fatalf("Expected valid CloudProvider, got %v", parsed1.CloudProvider())
	}

	input2 := "aws:///us-west-1/instance-id1"
	parsed2, err := NewProviderID(input2)
	if err != nil {
		t.Fatalf("Expected no errors, got %v", err)
	}

	if parsed2.String() != input2 {
		t.Fatalf("Expected String output to match original input %q, got %q", input1, parsed1.String())
	}

	if parsed2.ID() != "instance-id1" {
		t.Fatalf("Expected valid ID, got %v", parsed2.ID())
	}

	if parsed2.CloudProvider() != "aws" {
		t.Fatalf("Expected valid CloudProvider, got %v", parsed2.CloudProvider())
	}

	if !parsed1.Equals(parsed2) {
		t.Fatal("Expected ProviderIDs to be equal")
	}

}
