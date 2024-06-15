//go:build tools
// +build tools

/*
Copyright 2022 The Kubernetes Authors.

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

package main

import (
	"testing"

	. "github.com/onsi/gomega"
)

func Test_GetReleaseDetails(t *testing.T) {
	tests := []struct {
		name        string
		releaseTag  string
		releaseDate string
		want        releaseDetails
		expectErr   bool
		err         string
	}{
		{
			name:        "Correct RELEASE_TAG and RELEASE_DATE are set",
			releaseTag:  "v1.7.0-beta.0",
			releaseDate: "2024-04-16",
			want: releaseDetails{
				ReleaseDate:      "Tuesday, 16th April 2024",
				ReleaseTag:       "v1.7.0",
				BetaTag:          "v1.7.0-beta.0",
				ReleaseLink:      "https://github.com/kubernetes-sigs/cluster-api/tree/main/docs/release/releases/release-1.7.md#timeline",
				ReleaseNotesLink: "https://github.com/kubernetes-sigs/cluster-api/releases/tag/v1.7.0-beta.0",
			},
			expectErr: false,
		},
		{
			name:        "RELEASE_TAG is not in the format ^v\\d+\\.\\d+\\.\\d+-beta\\.\\d+$",
			releaseTag:  "v1.7.0.1",
			releaseDate: "2024-04-16",
			expectErr:   true,
			err:         "release tag must be in format `^v\\d+\\.\\d+\\.\\d+-beta\\.\\d+$` e.g. v1.7.0-beta.0",
		},
		{
			name:        "RELEASE_TAG does not have prefix 'v' in its semver",
			releaseTag:  "1.7.0-beta.0",
			releaseDate: "2024-04-16",
			expectErr:   true,
			err:         "release tag must be in format `^v\\d+\\.\\d+\\.\\d+-beta\\.\\d+$` e.g. v1.7.0-beta.0",
		},
		{
			name:        "RELEASE_TAG contains invalid Major.Minor.Patch SemVer",
			releaseTag:  "v1.x.0-beta.0",
			releaseDate: "2024-04-16",
			expectErr:   true,
			err:         "release tag must be in format `^v\\d+\\.\\d+\\.\\d+-beta\\.\\d+$` e.g. v1.7.0-beta.0",
		},
		{
			name:        "invalid yyyy-dd-mm RELEASE_DATE entered",
			releaseTag:  "v1.7.0-beta.0",
			releaseDate: "2024-16-4",
			expectErr:   true,
			err:         "unable to parse the date",
		},
		{
			name:        "invalid yyyy/dd/mm RELEASE_DATE entered",
			releaseTag:  "v1.7.0-beta.0",
			releaseDate: "2024/16/4",
			expectErr:   true,
			err:         "unable to parse the date",
		},
		{
			name:        "invalid yyyy/mm/dd RELEASE_DATE entered",
			releaseTag:  "v1.7.0-beta.0",
			releaseDate: "2024/4/16",
			expectErr:   true,
			err:         "unable to parse the date",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			t.Setenv("RELEASE_TAG", tt.releaseTag)
			t.Setenv("RELEASE_DATE", tt.releaseDate)

			got, err := getReleaseDetails()
			if tt.expectErr {
				g.Expect(err.Error()).To(Equal(tt.err))
			} else {
				g.Expect(got.ReleaseDate).To(Equal(tt.want.ReleaseDate))
				g.Expect(got.ReleaseTag).To(Equal(tt.want.ReleaseTag))
				g.Expect(got.BetaTag).To(Equal(tt.want.BetaTag))
				g.Expect(got.ReleaseLink).To(Equal(tt.want.ReleaseLink))
			}
		})
	}
}
