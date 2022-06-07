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
