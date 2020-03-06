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

package internal

import (
	"testing"

	"github.com/onsi/gomega"
)

func TestValidateCoreDNSImageTag(t *testing.T) {
	tests := []struct {
		name            string
		fromVer         string
		toVer           string
		expectErrSubStr string
	}{
		{
			name:            "fromVer is higher than toVer",
			fromVer:         "1.6.2",
			toVer:           "1.1.3",
			expectErrSubStr: "must be greater than",
		},
		{
			name:            "fromVer is not a valid coredns version",
			fromVer:         "0.204.123",
			toVer:           "1.6.3",
			expectErrSubStr: "not a compatible coredns version",
		},
		{
			name:            "toVer is not a valid coredns version format",
			fromVer:         "1.6.1",
			toVer:           "v1.6.2",
			expectErrSubStr: "failed to semver parse",
		},
		{
			name:            "toVer is not a valid semver",
			fromVer:         "1.5.1",
			toVer:           "foobar",
			expectErrSubStr: "failed to semver parse",
		},
		{
			name:            "fromVer is not a valid semver",
			fromVer:         "foobar",
			toVer:           "1.6.1",
			expectErrSubStr: "failed to semver parse",
		},
		{
			name:            "fromVer is equal to toVer, return false because there's no need to upgrade",
			fromVer:         "1.6.5",
			toVer:           "1.6.5",
			expectErrSubStr: "must be greater than",
		},
		{
			name:    "fromVer is lower but has meta",
			fromVer: "1.6.5-foobar.1",
			toVer:   "1.7.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			err := validateCoreDNSImageTag(tt.fromVer, tt.toVer)
			if tt.expectErrSubStr != "" {
				g.Expect(err.Error()).To(gomega.ContainSubstring(tt.expectErrSubStr))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}
