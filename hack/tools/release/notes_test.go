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

import "testing"

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
		area  string
		want  string
	}{
		{
			name:  "PR title with area",
			title: "e2e: improve logging for a detected rollout",
			area:  "e2e",
			want:  "improve logging for a detected rollout",
		},
		{
			name:  "PR title without area",
			title: "improve logging for a detected rollout",
			area:  "e2e",
			want:  "improve logging for a detected rollout",
		},
		{
			name:  "PR title without area being prefixed",
			title: "test/e2e: improve logging for a detected rollout",
			area:  "e2e",
			want:  "test/e2e: improve logging for a detected rollout",
		},
		{
			name:  "PR title without space between area and title",
			title: "e2e:improve logging for a detected rollout",
			area:  "e2e",
			want:  "improve logging for a detected rollout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trimAreaFromTitle(tt.title, tt.area); got != tt.want {
				t.Errorf("trimAreaFromTitle() = %v, want %v", got, tt.want)
			}
		})
	}
}
