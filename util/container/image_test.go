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

package container

import (
	"strings"
	"testing"

	"github.com/docker/distribution/reference"
	. "github.com/onsi/gomega"
)

func TestParseImageName(t *testing.T) {
	testCases := []struct {
		name      string
		input     string
		repo      string
		imageName string
		tag       string
		digest    string
		wantError bool
	}{
		{
			name:      "input with path and tag",
			input:     "k8s.gcr.io/dev/coredns:1.6.2",
			repo:      "k8s.gcr.io/dev",
			imageName: "coredns",
			tag:       "1.6.2",
			wantError: false,
		},
		{
			name:      "input with name only",
			input:     "example.com/root",
			repo:      "example.com",
			imageName: "root",
			wantError: false,
		},
		{
			name:      "input with name and tag without path",
			input:     "example.com/root:tag",
			repo:      "example.com",
			imageName: "root",
			tag:       "tag",
			wantError: false,
		},
		{
			name:      "input with name and digest without tag",
			input:     "example.com/root@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			repo:      "example.com",
			imageName: "root",
			digest:    "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			wantError: false,
		},
		{
			name:      "input with path and name",
			input:     "example.com/user/repo",
			repo:      "example.com/user",
			imageName: "repo",
			wantError: false,
		},
		{
			name:      "input with path, name and tag",
			input:     "example.com/user/repo:tag",
			repo:      "example.com/user",
			imageName: "repo",
			tag:       "tag",
			wantError: false,
		},
		{
			name:      "input with path, name, tag and digest",
			input:     "example.com/user/repo:tag@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			repo:      "example.com/user",
			imageName: "repo",
			tag:       "tag",
			digest:    "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			wantError: false,
		},
		{
			name:      "input with url with port",
			input:     "url:5000/repo",
			repo:      "url:5000",
			imageName: "repo",
			wantError: false,
		},
		{
			name:      "input with url with port and tag",
			input:     "url:5000/repo:tag",
			repo:      "url:5000",
			imageName: "repo",
			tag:       "tag",
			wantError: false,
		},
		{
			name:      "input with url with port and digest",
			input:     "url:5000/repo@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			repo:      "url:5000",
			imageName: "repo",
			digest:    "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			wantError: false,
		},
		{
			name:      "input with invalid image name",
			input:     "url$#",
			repo:      "",
			imageName: "",
			tag:       "",
			wantError: true,
		},
	}
	for _, tc := range testCases {
		g := NewWithT(t)

		t.Run(tc.name, func(t *testing.T) {
			image, err := ImageFromString(tc.input)
			if tc.wantError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(image.Repository).To(Equal(tc.repo))
				g.Expect(image.Name).To(Equal(tc.imageName))
				g.Expect(image.Tag).To(Equal(tc.tag))
				g.Expect(image.Digest).To(Equal(tc.digest))
				g.Expect(image.String()).To(Equal(tc.input))
			}
		})
	}
}

func TestModifyImageRepository(t *testing.T) {
	const testRepository = "example.com/new"

	testCases := []struct {
		name           string
		image          string
		repo           string
		want           string
		wantError      bool
		wantErrMessage string
	}{
		{
			name:           "updates the repository of the image",
			image:          "example.com/subpaths/are/okay/image:1.17.3",
			repo:           testRepository,
			want:           "example.com/new/image:1.17.3",
			wantError:      false,
			wantErrMessage: "",
		},
		{
			name:           "errors if the repository name is too long",
			image:          "example.com/image:1.17.3",
			repo:           strings.Repeat("a", 255),
			want:           "",
			wantError:      true,
			wantErrMessage: reference.ErrNameTooLong.Error(),
		},
		{
			name:           "errors if the image name is not canonical",
			image:          "image:1.17.3",
			repo:           testRepository,
			want:           "",
			wantError:      true,
			wantErrMessage: reference.ErrNameNotCanonical.Error(),
		},
		{
			name:           "errors if the image name is not tagged",
			image:          "example.com/image",
			repo:           testRepository,
			want:           "",
			wantError:      true,
			wantErrMessage: "image must be tagged",
		},
		{
			name:           "errors if the image name is not valid",
			image:          "example.com/image:$@$(*",
			repo:           testRepository,
			want:           "",
			wantError:      true,
			wantErrMessage: "failed to parse image name",
		},
	}
	for _, tc := range testCases {
		g := NewWithT(t)

		t.Run(tc.name, func(t *testing.T) {
			res, err := ModifyImageRepository(tc.image, tc.repo)
			if tc.wantError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(MatchError(ContainSubstring(tc.wantErrMessage)))
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(res).To(Equal(tc.want))
			}
		})
	}
}

func TestModifyImageTag(t *testing.T) {
	g := NewWithT(t)
	t.Run("should ensure image is a docker compatible tag", func(t *testing.T) {
		testTag := "v1.17.4+build1"
		image := "example.com/image:1.17.3"
		res, err := ModifyImageTag(image, testTag)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res).To(Equal("example.com/image:v1.17.4_build1"))
	})
}
