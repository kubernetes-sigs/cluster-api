/*
Copyright 2025 The Kubernetes Authors.

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

package v1beta2

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestBootstrapTokenStringMarshalJSON(t *testing.T) {
	var tests = []struct {
		bts      BootstrapTokenString
		expected string
	}{
		{BootstrapTokenString{ID: "abcdef", Secret: "abcdef0123456789"}, `"abcdef.abcdef0123456789"`},
		{BootstrapTokenString{ID: "foo", Secret: "bar"}, `"foo.bar"`},
		{BootstrapTokenString{ID: "h", Secret: "b"}, `"h.b"`},
	}
	for _, rt := range tests {
		t.Run(rt.bts.ID, func(t *testing.T) {
			b, err := json.Marshal(rt.bts)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if string(b) != rt.expected {
				t.Fatalf("Expected %s to equal %s", string(b), rt.expected)
			}
		})
	}
}

func TestBootstrapTokenStringUnmarshalJSON(t *testing.T) {
	var tests = []struct {
		input         string
		bts           *BootstrapTokenString
		expectedError bool
	}{
		{`"f.s"`, &BootstrapTokenString{}, true},
		{`"abcdef."`, &BootstrapTokenString{}, true},
		{`"abcdef:abcdef0123456789"`, &BootstrapTokenString{}, true},
		{`abcdef.abcdef0123456789`, &BootstrapTokenString{}, true},
		{`"abcdef.abcdef0123456789`, &BootstrapTokenString{}, true},
		{`"abcdef.ABCDEF0123456789"`, &BootstrapTokenString{}, true},
		{`"abcdef.abcdef0123456789"`, &BootstrapTokenString{ID: "abcdef", Secret: "abcdef0123456789"}, false},
		{`"123456.aabbccddeeffgghh"`, &BootstrapTokenString{ID: "123456", Secret: "aabbccddeeffgghh"}, false},
	}
	for _, rt := range tests {
		t.Run(rt.input, func(t *testing.T) {
			newbts := &BootstrapTokenString{}
			err := json.Unmarshal([]byte(rt.input), newbts)
			if rt.expectedError {
				if err == nil {
					t.Fatalf("Expected error but didn't return an error")
				}
			} else {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
			}
			if *newbts != *rt.bts {
				t.Fatalf("Expected %s to equal %s", *newbts, *rt.bts)
			}
		})
	}
}

func TestBootstrapTokenStringJSONRoundtrip(t *testing.T) {
	var tests = []struct {
		input string
		bts   *BootstrapTokenString
	}{
		{`"abcdef.abcdef0123456789"`, nil},
		{"", &BootstrapTokenString{ID: "abcdef", Secret: "abcdef0123456789"}},
	}
	for _, rt := range tests {
		t.Run(rt.input, func(t *testing.T) {
			if err := roundtrip(rt.input, rt.bts); err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
		})
	}
}

func roundtrip(input string, bts *BootstrapTokenString) error {
	var b []byte
	var err error
	newbts := &BootstrapTokenString{}
	// If string input was specified, roundtrip like this: string -> (unmarshal) -> object -> (marshal) -> string
	if input != "" {
		if err := json.Unmarshal([]byte(input), newbts); err != nil {
			return fmt.Errorf("expected no unmarshal error, got error: %w", err)
		}
		if b, err = json.Marshal(newbts); err != nil {
			return fmt.Errorf("expected no marshal error, got error: %w", err)
		}
		if input != string(b) {
			return fmt.Errorf(
				"expected token: %s\n\t  actual: %s",
				input,
				string(b),
			)
		}
	} else { // Otherwise, roundtrip like this: object -> (marshal) -> string -> (unmarshal) -> object
		if b, err = json.Marshal(bts); err != nil {
			return fmt.Errorf( "expected no marshal error, got error: %w", err)
		}
		if err := json.Unmarshal(b, newbts); err != nil {
			return fmt.Errorf( "expected no unmarshal error, got error: %w", err)
		}
		if *bts != *newbts {
			return fmt.Errorf(
				"expected object: %v\n\t  actual: %v\n\t",
				bts,
				newbts,
			)
		}
	}
	return nil
}

func TestBootstrapTokenStringTokenFromIDAndSecret(t *testing.T) {
	var tests = []struct {
		bts      BootstrapTokenString
		expected string
	}{
		{BootstrapTokenString{ID: "foo", Secret: "bar"}, "foo.bar"},
		{BootstrapTokenString{ID: "abcdef", Secret: "abcdef0123456789"}, "abcdef.abcdef0123456789"},
		{BootstrapTokenString{ID: "h", Secret: "b"}, "h.b"},
	}
	for _, rt := range tests {
		t.Run(rt.bts.ID, func(t *testing.T) {
			actual := rt.bts.String()
			if actual != rt.expected {
				t.Fatalf("Expected %s to equal %s", actual, rt.expected)
			}
		})
	}
}

func TestNewBootstrapTokenString(t *testing.T) {
	var tests = []struct {
		token         string
		expectedError bool
		bts           *BootstrapTokenString
	}{
		{token: "", expectedError: true, bts: nil},
		{token: ".", expectedError: true, bts: nil},
		{token: "1234567890123456789012", expectedError: true, bts: nil},   // invalid parcel size
		{token: "12345.1234567890123456", expectedError: true, bts: nil},   // invalid parcel size
		{token: ".1234567890123456", expectedError: true, bts: nil},        // invalid parcel size
		{token: "123456.", expectedError: true, bts: nil},                  // invalid parcel size
		{token: "123456:1234567890.123456", expectedError: true, bts: nil}, // invalid separation
		{token: "abcdef:1234567890123456", expectedError: true, bts: nil},  // invalid separation
		{token: "Abcdef.1234567890123456", expectedError: true, bts: nil},  // invalid token id
		{token: "123456.AABBCCDDEEFFGGHH", expectedError: true, bts: nil},  // invalid token secret
		{token: "123456.AABBCCD-EEFFGGHH", expectedError: true, bts: nil},  // invalid character
		{token: "abc*ef.1234567890123456", expectedError: true, bts: nil},  // invalid character
		{token: "abcdef.1234567890123456", expectedError: false, bts: &BootstrapTokenString{ID: "abcdef", Secret: "1234567890123456"}},
		{token: "123456.aabbccddeeffgghh", expectedError: false, bts: &BootstrapTokenString{ID: "123456", Secret: "aabbccddeeffgghh"}},
		{token: "abcdef.abcdef0123456789", expectedError: false, bts: &BootstrapTokenString{ID: "abcdef", Secret: "abcdef0123456789"}},
		{token: "123456.1234560123456789", expectedError: false, bts: &BootstrapTokenString{ID: "123456", Secret: "1234560123456789"}},
	}
	for _, rt := range tests {
		t.Run(rt.token, func(t *testing.T) {
			actual, err := NewBootstrapTokenString(rt.token)
			if rt.expectedError {
				if err == nil {
					t.Fatalf("Expected error but didn't return an error")
				}
			} else {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
			}
			if !reflect.DeepEqual(actual, rt.bts) {
				t.Fatalf("Expected %s to equal %s", *actual, *rt.bts)
			}
		})
	}
}
