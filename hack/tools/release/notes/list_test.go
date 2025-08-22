//go:build tools
// +build tools

/*
Copyright 2023 The Kubernetes Authors.

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

func Test_buildSetOfPRNumbers(t *testing.T) {
	tests := []struct {
		name    string
		commits []githubCommitNode
		want    map[string]struct{}
	}{
		{
			name: "merge commit",
			commits: []githubCommitNode{
				{
					Commit: githubCommit{
						Message: "Merge pull request #9072 from k8s-infra-cherrypick-robot/cherry-pick-9070-to-release-1.5\n\n[release-1.5] :bug: Change tilt debug base image to golang",
					},
				},
			},
			want: map[string]struct{}{
				"9072": {},
			},
		},
		{
			name: "squashed commit by tide",
			commits: []githubCommitNode{
				{
					Commit: githubCommit{
						Message: ":seedling: Add dependabot groups. Allow additional patch updates (#9263)\n\n* Allow patch updates on dependabot ignore list\n\nSigned-off-by: user <user@test.com>\n\n* Add dependency groups for dependabot\n\nSigned-off-by: user <user@test.com>\n\n---------\n\nSigned-off-by: user <user@test.com>",
					},
				},
			},
			want: map[string]struct{}{
				"9263": {},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(buildSetOfPRNumbers(tt.commits)).To(Equal(tt.want))
		})
	}
}
