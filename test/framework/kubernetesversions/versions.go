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

package kubernetesversions

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/blang/semver"
)

const (
	ciVersionURL     = "https://dl.k8s.io/ci/latest.txt"
	stableVersionURL = "https://storage.googleapis.com/kubernetes-release/release/stable-%d.%d.txt"
	tagPrefix        = "v"
)

// LatestCIRelease fetches the latest main branch Kubernetes version.
func LatestCIRelease() (string, error) {
	resp, err := http.Get(ciVersionURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(b)), nil
}

// LatestPatchRelease returns the latest patch release matching.
func LatestPatchRelease(searchVersion string) (string, error) {
	searchSemVer, err := semver.Make(strings.TrimPrefix(searchVersion, tagPrefix))
	if err != nil {
		return "", err
	}
	resp, err := http.Get(fmt.Sprintf(stableVersionURL, searchSemVer.Major, searchSemVer.Minor))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(b)), nil
}

// PreviousMinorRelease returns the latest patch release for the previous version
// of Kubernetes, e.g. v1.19.1 returns v1.18.8 as of Sep 2020.
func PreviousMinorRelease(searchVersion string) (string, error) {
	semVer, err := semver.Make(strings.TrimPrefix(searchVersion, tagPrefix))
	if err != nil {
		return "", err
	}
	semVer.Minor--

	return LatestPatchRelease(semVer.String())
}
