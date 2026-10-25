/*
Copyright 2021 The Kubernetes Authors.

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

package version

import (
	"testing"

	"github.com/blang/semver/v4"
	. "github.com/onsi/gomega"
)

func TestCompare(t *testing.T) {
	tests := []struct {
		name     string
		aVersion semver.Version
		bVersion semver.Version
		options  []CompareOption
		want     int
	}{
		{
			name:     "comparing with no options should perform standard compare",
			aVersion: semver.MustParse("1.2.3"),
			bVersion: semver.MustParse("1.3.1"),
			want:     -1,
		},
		{
			name:     "comparing with no options should perform standard compare - equal versions",
			aVersion: semver.MustParse("1.2.3+xyz.1"),
			bVersion: semver.MustParse("1.2.3+xyz.2"),
			want:     0,
		},
		{
			name:     "compare with build tags using the WithBuildTags option",
			aVersion: semver.MustParse("1.2.3+xyz.1"),
			bVersion: semver.MustParse("1.2.3+xyz.2"),
			options:  []CompareOption{WithBuildTags()},
			want:     -1,
		},
		{
			name:     "compare with no build identifiers",
			aVersion: mustParseTolerant("v1.20.1"),
			bVersion: mustParseTolerant("v1.20.2"),
			options:  []CompareOption{WithBuildTags()},
			want:     -1,
		},
		{
			name:     "compare with pre release versions and no build identifiers",
			aVersion: mustParseTolerant("v1.20.1-alpha.1"),
			bVersion: mustParseTolerant("v1.20.1-alpha.2"),
			options:  []CompareOption{WithBuildTags()},
			want:     -1,
		},
		{
			name:     "compare with pre release versions and build identifiers",
			aVersion: mustParseTolerant("v1.20.1-alpha.1+xyz.1"),
			bVersion: mustParseTolerant("v1.20.1-alpha.1+xyz.2"),
			options:  []CompareOption{WithBuildTags()},
			want:     -1,
		},
		{
			name:     "compare with build identifiers - smaller",
			aVersion: mustParseTolerant("v1.20.1+xyz.1"),
			bVersion: mustParseTolerant("v1.20.1+xyz.2"),
			options:  []CompareOption{WithBuildTags()},
			want:     -1,
		},
		{
			name:     "compare with build identifiers - equal",
			aVersion: mustParseTolerant("v1.20.1+xyz.1"),
			bVersion: mustParseTolerant("v1.20.1+xyz.1"),
			options:  []CompareOption{WithBuildTags()},
			want:     0,
		},
		{
			name:     "compare with build identifiers - greater",
			aVersion: mustParseTolerant("v1.20.1+xyz.3"),
			bVersion: mustParseTolerant("v1.20.1+xyz.2"),
			options:  []CompareOption{WithBuildTags()},
			want:     1,
		},
		{
			name:     "compare with build identifiers - smaller by sub version",
			aVersion: mustParseTolerant("v1.20.1+xyz.1.0"),
			bVersion: mustParseTolerant("v1.20.1+xyz.1.1"),
			options:  []CompareOption{WithBuildTags()},
			want:     -1,
		},
		{
			name:     "compare with build identifiers - smaller - different version lengths",
			aVersion: mustParseTolerant("v1.20.1+xyz.1.1"),
			bVersion: mustParseTolerant("v1.20.1+xyz.2"),
			options:  []CompareOption{WithBuildTags()},
			want:     -1,
		},
		{
			name:     "compare with build identifiers - greater by length",
			aVersion: mustParseTolerant("v1.20.1+xyz.1.1"),
			bVersion: mustParseTolerant("v1.20.1+xyz.1"),
			options:  []CompareOption{WithBuildTags()},
			want:     1,
		},
		{
			name:     "compare with build identifiers - different non numeric",
			aVersion: mustParseTolerant("v1.20.1+xyz.a"),
			bVersion: mustParseTolerant("v1.20.1+xyz.b"),
			options:  []CompareOption{WithBuildTags()},
			want:     2,
		},
		{
			name:     "compare with build identifiers - equal non numeric",
			aVersion: mustParseTolerant("v1.20.1+xyz.a"),
			bVersion: mustParseTolerant("v1.20.1+xyz.a"),
			options:  []CompareOption{WithBuildTags()},
			want:     0,
		},
		{
			name:     "compare with build identifiers - smaller - a is numeric b is not",
			aVersion: mustParseTolerant("v1.20.1+xyz.1"),
			bVersion: mustParseTolerant("v1.20.1+xyz.abc"),
			options:  []CompareOption{WithBuildTags()},
			want:     -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(Compare(tt.aVersion, tt.bVersion, tt.options...)).To(Equal(tt.want))
		})
	}
}

func mustParseTolerant(s string) semver.Version {
	v, err := semver.ParseTolerant(s)
	if err != nil {
		panic(`semver: ParseTolerant(` + s + `): ` + err.Error())
	}
	return v
}
