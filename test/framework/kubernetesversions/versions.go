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
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/blang/semver"
	"github.com/pkg/errors"
)

const (
	ciVersionURL     = "https://dl.k8s.io/ci/latest.txt"
	stableVersionURL = "https://dl.k8s.io/release/stable-%d.%d.txt"
	tagPrefix        = "v"
)

// LatestCIRelease fetches the latest main branch Kubernetes version.
func LatestCIRelease() (string, error) {
	ctx := context.TODO()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ciVersionURL, http.NoBody)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get %s: failed to create request", ciVersionURL)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get %s", ciVersionURL)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get %s: failed to read body", ciVersionURL)
	}

	return strings.TrimSpace(string(b)), nil
}

// LatestPatchRelease returns the latest patch release matching.
func LatestPatchRelease(searchVersion string) (string, error) {
	ctx := context.TODO()

	searchSemVer, err := semver.Make(strings.TrimPrefix(searchVersion, tagPrefix))
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf(stableVersionURL, searchSemVer.Major, searchSemVer.Minor)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get %s: failed to create request", url)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get %s", url)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get %s: failed to read body", url)
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
