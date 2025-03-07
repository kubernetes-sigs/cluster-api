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

func Test_githubFromToPRLister_listPRs(t *testing.T) {
	type fields struct {
		client  *githubClient
		fromRef *ref
		toRef   *ref
		branch  string
	}
	type args struct {
		previousReleaseRef ref
	}
	tests := []struct {
		name    string
		fields  *githubFromToPRLister
		args    ref
		wantErr bool
	}{
		{
			name: "Successful PR Listing",
			fields: &githubFromToPRLister{
				client:  &githubClient{
					repo:    "kubernetes-sigs/kind",
				},
				fromRef: ref{
					reType: "tags",
					value: "v0.26.0",
				},
				toRef: ref{
					reType: "tags",
					value: "v0.27.0",
				},
				branch:  "main",
			},
			args: ref{
				reType: "tags",
				value: "v0.26.0",
			},
			wantErr: false,
		},
		{
			name: "Setting previousReleaseRef.value blank - should use toRef and fromRef from fields",
			fields: &githubFromToPRLister{
				client:  &githubClient{
					repo:    "kubernetes-sigs/kind",
				},
				fromRef: ref{
					reType: "tags",
					value: "v0.26.0",
				},
				toRef: ref{
					reType: "tags",
					value: "v0.27.0",
				},
				branch:  "main",
			},
			args: ref{
				reType: "tags",
				value: "",
			},
			wantErr: false,
		},
		{
			name: "Create PR List when fromRef is not set",
			fields: &githubFromToPRLister{
				client:  &githubClient{
					repo:    "kubernetes-sigs/kind",
				},
				toRef: ref{
					reType: "tags",
					value: "v0.27.0",
				},
				branch:  "main",
			},
			args: ref{
				reType: "tags",
				value: "v0.26.0",
			},
			wantErr: false,
		},
		{
			name: "Fail when previousReleaseRef.value is set to invalid",
			fields: &githubFromToPRLister{
				client:  &githubClient{
					repo:    "kubernetes-sigs/kind",
				},
				fromRef: ref{
					reType: "tags",
					value: "v0.26.0",
				},
				toRef: ref{
					reType: "tags",
					value: "v0.27.0",
				},
				branch:  "main",
			},
			args: ref{
				reType: "tags",
				value: "invalid",
			},
			wantErr: true,
		},
		{
			name: "Fail when toRef and previousReleaseRef set blank",
			fields: &githubFromToPRLister{
				client:  &githubClient{
					repo:    "kubernetes-sigs/kind",
				},
				fromRef: ref{
					reType: "tags",
					value: "v0.26.0",
				},
				toRef: ref{
					reType: "tags",
					value: "",
				},
				branch:  "main",
			},
			args: ref{
				reType: "tags",
				value: "",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &githubFromToPRLister{
				client:  tt.fields.client,
				fromRef: tt.fields.fromRef,
				toRef:   tt.fields.toRef,
				branch:  tt.fields.branch,
			}
			_, err := l.listPRs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("githubFromToPRLister.listPRs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
