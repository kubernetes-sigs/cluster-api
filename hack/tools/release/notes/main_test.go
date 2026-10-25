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
	"strings"
	"testing"

	"github.com/blang/semver/v4"
	. "github.com/onsi/gomega"
)

func Test_trimTitle(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{
			name:  "regular PR title",
			title: "📖 book: Use relative links in generate CRDs doc",
			want:  "📖 book: Use relative links in generate CRDs doc",
		},
		{
			name:  "PR title with WIP",
			title: "WIP 📖 book: Use relative links in generate CRDs doc",
			want:  "WIP 📖 book: Use relative links in generate CRDs doc",
		},
		{
			name:  "PR title with [WIP]",
			title: "[WIP] 📖 book: Use relative links in generate CRDs doc",
			want:  "[WIP] 📖 book: Use relative links in generate CRDs doc",
		},
		{
			name:  "PR title with [release-1.0]",
			title: "[release-1.0] 📖 book: Use relative links in generate CRDs doc",
			want:  "📖 book: Use relative links in generate CRDs doc",
		},
		{
			name:  "PR title with [WIP][release-1.0]",
			title: "[WIP][release-1.0] 📖 book: Use relative links in generate CRDs doc",
			want:  "[WIP][release-1.0] 📖 book: Use relative links in generate CRDs doc",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trimTitle(tt.title); got != tt.want {
				t.Errorf("trimTitle() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_trimAreaFromTitle(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{
			name:  "PR title with area",
			title: "test: improve logging for a detected rollout",
			want:  "improve logging for a detected rollout",
		},
		{
			name:  "PR title without area",
			title: "improve logging for a detected rollout",
			want:  "improve logging for a detected rollout",
		},
		{
			name:  "PR title without area being prefixed",
			title: "test/e2e: improve logging for a detected rollout",
			want:  "improve logging for a detected rollout",
		},
		{
			name:  "PR title without space between area and title",
			title: "e2e:improve logging for a detected rollout",
			want:  "improve logging for a detected rollout",
		},
		{
			name:  "PR title with space between area and title",
			title: "e2e/test: improve logging for a detected rollout",
			want:  "improve logging for a detected rollout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trimAreaFromTitle(tt.title); got != tt.want {
				t.Errorf("trimAreaFromTitle() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_defaultBranchForNewTag(t *testing.T) {
	tests := []struct {
		name       string
		newVersion string
		want       string
	}{
		{
			name:       "new minor",
			newVersion: "v1.5.0",
			want:       "release-1.5",
		},
		{
			name:       "new patch",
			newVersion: "v1.6.1",
			want:       "release-1.6",
		},
		{
			name:       "first RC",
			newVersion: "v1.6.0-rc.0",
			want:       "main",
		},
		{
			name:       "second RC",
			newVersion: "v1.6.0-rc.1",
			want:       "release-1.6",
		},
		{
			name:       "third RC",
			newVersion: "v1.6.0-rc.2",
			want:       "release-1.6",
		},
		{
			name:       "first Beta",
			newVersion: "v1.6.0-beta.0",
			want:       "main",
		},
		{
			name:       "second Beta",
			newVersion: "v1.6.0-beta.1",
			want:       "main",
		},
		{
			name:       "third Beta",
			newVersion: "v1.6.0-beta.2",
			want:       "main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			version, err := semver.ParseTolerant(tt.newVersion)
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(defaultBranchForNewTag(version)).To(Equal(tt.want))
		})
	}
}

func Test_validateConfig(t *testing.T) {
	tests := []struct {
		name         string
		args         *notesCmdConfig
		wantErr      bool
		errorMessage string
	}{
		{
			name: "Missing fromRef or newTag when branch is set only",
			args: &notesCmdConfig{
				branch: "main",
			},
			wantErr:      true,
			errorMessage: "at least one of --from or --release need to be set",
		},
		{
			name: "Missing branch or newTag when fromRef is set only",
			args: &notesCmdConfig{
				fromRef: "ref1/tags",
			},
			wantErr:      true,
			errorMessage: "at least one of --branch or --release need to be set",
		},
		{
			name: "Invalid fromRef",
			args: &notesCmdConfig{
				fromRef: "invalid",
				branch:  "main",
			},
			wantErr:      true,
			errorMessage: "invalid ref invalid: must follow [type]/[value]",
		},
		{
			name: "Invalid toRef",
			args: &notesCmdConfig{
				toRef:   "invalid",
				branch:  "main",
				fromRef: "ref1/tags",
			},
			wantErr:      true,
			errorMessage: "invalid ref invalid: must follow [type]/[value]",
		},
		{
			name: "Valid fromRef, toRef, and newTag",
			args: &notesCmdConfig{
				fromRef: "ref1/tags",
				toRef:   "ref2/tags",
				newTag:  "v1.0.0",
			},
			wantErr: false,
		},
		{
			name: "Missing branch when newTag is set",
			args: &notesCmdConfig{
				branch: "main",
				toRef:  "ref2/tags",
				newTag: "v1.0.0",
			},
			wantErr: false,
		},
		{
			name: "Invalid previousReleaseVersion without ref format",
			args: &notesCmdConfig{
				fromRef:                "ref1/tags",
				toRef:                  "ref2/tags",
				newTag:                 "v1.0.0",
				previousReleaseVersion: "v1.0.0-rc.0",
			},
			wantErr:      true,
			errorMessage: "--previous-release-version must be in ref format",
		},
		{
			name: "Valid previousReleaseVersion with rc in ref format",
			args: &notesCmdConfig{
				fromRef:                "ref1/tags",
				toRef:                  "ref2/tags",
				newTag:                 "v1.0.0",
				previousReleaseVersion: "tags/v1.0.0-rc.0",
			},
			wantErr: false,
		},
		{
			name: "Valid previousReleaseVersion with alpha in ref format",
			args: &notesCmdConfig{
				fromRef:                "ref1/tags",
				toRef:                  "ref2/tags",
				newTag:                 "v1.0.0",
				previousReleaseVersion: "tags/v1.0.0-alpha.1",
			},
			wantErr: false,
		},
		{
			name: "Valid previousReleaseVersion with beta in ref format",
			args: &notesCmdConfig{
				fromRef:                "ref1/tags",
				toRef:                  "ref2/tags",
				newTag:                 "v1.0.0",
				previousReleaseVersion: "tags/v1.0.0-beta.1",
			},
			wantErr: false,
		},
		{
			name: "Invalid previousReleaseVersion without pre-release in ref format",
			args: &notesCmdConfig{
				fromRef:                "ref1/tags",
				toRef:                  "ref2/tags",
				newTag:                 "v1.0.0",
				previousReleaseVersion: "tags/v1.0.0",
			},
			wantErr:      true,
			errorMessage: "--previous-release-version must contain 'alpha', 'beta', or 'rc' pre-release identifier",
		},
		{
			name: "Invalid previousReleaseVersion with unsupported pre-release type",
			args: &notesCmdConfig{
				fromRef:                "ref1/tags",
				toRef:                  "ref2/tags",
				newTag:                 "v1.0.0",
				previousReleaseVersion: "tags/v1.0.0-dev.1",
			},
			wantErr:      true,
			errorMessage: "--previous-release-version must contain 'alpha', 'beta', or 'rc' pre-release identifier",
		},
		{
			name: "Invalid previousReleaseVersion with invalid semver",
			args: &notesCmdConfig{
				fromRef:                "ref1/tags",
				toRef:                  "ref2/tags",
				newTag:                 "v1.0.0",
				previousReleaseVersion: "tags/invalid-version",
			},
			wantErr:      true,
			errorMessage: "invalid --previous-release-version, is not a valid semver",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.args)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), tt.errorMessage) {
					t.Errorf("expected error '%s', got '%v'", tt.errorMessage, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func Test_computeConfigDefaults(t *testing.T) {
	tests := []struct {
		name    string
		args    *notesCmdConfig
		want    *notesCmdConfig
		wantErr bool
	}{
		{
			name: "Calculate fromRef when newTag is a new minor release and toRef",
			args: &notesCmdConfig{
				branch: "develop",
				newTag: "v1.1.0",
			},
			want: &notesCmdConfig{
				fromRef: "tags/v1.0.0",
				branch:  "develop",
				toRef:   "heads/develop",
				newTag:  "v1.1.0",
			},
			wantErr: false,
		},
		{
			name: "Calculate fromRef when newTag is not a new minor release, branch and toRef",
			args: &notesCmdConfig{
				newTag: "v1.1.3",
			},
			want: &notesCmdConfig{
				fromRef: "tags/v1.1.2",
				branch:  "release-1.1",
				toRef:   "heads/release-1.1",
				newTag:  "v1.1.3",
			},
			wantErr: false,
		},
		{
			name: "Fail when newTag is not a valid semver",
			args: &notesCmdConfig{
				newTag: "invalid-tag",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := computeConfigDefaults(tt.args)
			g := NewWithT(t)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring("invalid --release, is not a semver:"))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(tt.args.fromRef).To(Equal(tt.want.fromRef))
				g.Expect(tt.args.branch).To(Equal(tt.want.branch))
				g.Expect(tt.args.toRef).To(Equal(tt.want.toRef))
			}
		})
	}
}
