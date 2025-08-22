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

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
)

const (
	ciLatestVersionURL = "https://dl.k8s.io/ci/latest.txt"
	stableVersionURL   = "https://dl.k8s.io/release/stable-%s.txt"
	ciVersionURL       = "https://dl.k8s.io/ci/latest-%s.txt"
	tagPrefix          = "v"
)

// LatestCIRelease fetches the latest main branch Kubernetes version.
func LatestCIRelease() (string, error) {
	return getVersionFromMarkerFile(context.TODO(), ciLatestVersionURL)
}

// LatestPatchRelease returns the latest patch release matching.
func LatestPatchRelease(searchVersion string) (string, error) {
	ctx := context.TODO()

	searchSemVer, err := semver.Make(strings.TrimPrefix(searchVersion, tagPrefix))
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf(stableVersionURL, fmt.Sprintf("%d.%d", searchSemVer.Major, searchSemVer.Minor))

	return getVersionFromMarkerFile(ctx, url)
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

// ResolveVersion resolves Kubernetes versions.
// These are usually used to configure job in test-infra to test various upgrades.
// Versions can have the following formats:
// * v1.28.0 => will return the same version for convenience
// * stable-1.28 => will return the latest patch release for v1.28, e.g. v1.28.5
// * ci/latest-1.28 => will return the latest built version from the release branch, e.g. v1.28.5-26+72feddd3acde14
// This implementation mirrors what is implemented in ci-e2e-lib.sh k8s::resolveVersion().
func ResolveVersion(ctx context.Context, version string) (string, error) {
	if strings.HasPrefix(version, "v") {
		// version is already a version
		return version, nil
	}

	url, err := calculateURL(version)
	if err != nil {
		return "", err
	}

	return getVersionFromMarkerFile(ctx, url)
}

func calculateURL(version string) (string, error) {
	if strings.HasPrefix(version, "stable-") {
		return fmt.Sprintf(stableVersionURL, strings.TrimPrefix(version, "stable-")), nil
	}

	if strings.HasPrefix(version, "ci/latest-") {
		return fmt.Sprintf(ciVersionURL, strings.TrimPrefix(version, "ci/latest-")), nil
	}

	return "", fmt.Errorf("failed to resolve version %s", version)
}

func getVersionFromMarkerFile(ctx context.Context, url string) (string, error) {
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
