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

// White box testing for functionality in ssh.go
package google

import (
	"fmt"
	"testing"
)

func TestSSHMetadata(t *testing.T) {
	tests := []struct {
		name             string
		metadata         map[string]string
		user             string
		publicKey        string
		expectedMetadata map[string]string
	}{
		{
			name:             "Empty metadata",
			metadata:         map[string]string{},
			user:             "user1",
			publicKey:        "key1",
			expectedMetadata: map[string]string{"ssh-keys": "user1:key1"},
		},
		{
			name:             "Some metadata",
			metadata:         map[string]string{"foo": "bar"},
			user:             "user2",
			publicKey:        "key2",
			expectedMetadata: map[string]string{"foo": "bar", "ssh-keys": "user2:key2"},
		},
		{
			name:             "Overwrite metadata",
			metadata:         map[string]string{"foo": "bar", "ssh-keys": "user1:key1"},
			user:             "user3",
			publicKey:        "key3",
			expectedMetadata: map[string]string{"foo": "bar", "ssh-keys": "user3:key3"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := sshMetadata(test.metadata, test.user, test.publicKey)
			if len(m) != len(test.expectedMetadata) {
				t.Fatalf("Unexepected metadata count. Got:%v, Want:%v", len(m), len(test.expectedMetadata))
			}
			for eKey, eValue := range test.expectedMetadata {
				value, ok := m[eKey]
				if !ok {
					t.Fatalf("Missing metadata value for key:%v", eKey)
				}
				if value != eValue {
					t.Fatalf("Unexpected value for key %v. Got:%v, Want:%v", eKey, value, eValue)
				}
			}
		})
	}
}

func TestRemoteSSHCommand(t *testing.T) {
	oldRunOutput := runOutput
	defer func() { runOutput = oldRunOutput }()

	tests := []struct {
		name        string
		runErr      error
		runOut      string
		expectErr   bool
		expectedOut string
	}{
		{
			name: "success with no output",
		},
		{
			name:        "success with output",
			runOut:      "legen...",
			expectedOut: "legen...",
		},
		{
			name:        "success with cleaned output",
			runOut:      "  ...wait for it... ...dary",
			expectedOut: "...wait for it... ...dary",
		},
		{
			name:      "error",
			runErr:    fmt.Errorf("404"),
			expectErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runOutput = func(cmd string, args ...string) (string, error) {
				if cmd != "ssh" {
					t.Errorf("Unexpected call to %v", cmd)
				}
				return test.runOut, test.runErr
			}

			out, err := remoteSshCommand("", "", "", "")
			if (test.expectErr && err == nil) || (!test.expectErr && err != nil) {
				t.Fatalf("Error not as expected. Got: %v, Want Err: %v", err, test.expectErr)
			}
			if test.expectedOut != out {
				t.Fatalf("Unexpected output. Got:%v, Want:%v", out, test.expectedOut)
			}
		})
	}
}
