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

package repository

import (
	"context"
	"sort"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/version"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/scheme"
)

const (
	latestVersionTag = "latest"
)

// latestContractRelease returns the latest patch release for a repository for the current API contract, according to
// semantic version order of the release tag name.
func latestContractRelease(ctx context.Context, repo Repository, contract string) (string, error) {
	latest, err := latestRelease(ctx, repo)
	if err != nil {
		return latest, err
	}
	// Attempt to check if the latest release satisfies the API Contract
	// This is a best-effort attempt to find the latest release for an older API contract if it's not the latest release.
	file, err := repo.GetFile(ctx, latest, metadataFile)
	// If an error occurs, we just return the latest release.
	if err != nil {
		if errors.Is(err, errNotFound) {
			// If it was ErrNotFound, then there is no release yet for the resolved tag.
			// Ref: https://github.com/kubernetes-sigs/cluster-api/issues/7889
			return "", err
		}
		// if we can't get the metadata file from the release, we return latest.
		return latest, nil
	}
	latestMetadata := &clusterctlv1.Metadata{}
	codecFactory := serializer.NewCodecFactory(scheme.Scheme)
	if err := runtime.DecodeInto(codecFactory.UniversalDecoder(), file, latestMetadata); err != nil {
		return latest, nil //nolint:nilerr
	}

	releaseSeries := latestMetadata.GetReleaseSeriesForContract(contract)
	if releaseSeries == nil {
		return latest, nil
	}

	sv, err := version.ParseSemantic(latest)
	if err != nil {
		return latest, nil //nolint:nilerr
	}

	// If the Major or Minor version of the latest release doesn't match the release series for the current contract,
	// return the latest patch release of the desired Major/Minor version.
	if sv.Major() != releaseSeries.Major || sv.Minor() != releaseSeries.Minor {
		return latestPatchRelease(ctx, repo, &releaseSeries.Major, &releaseSeries.Minor)
	}
	return latest, nil
}

// latestRelease returns the latest release for a repository, according to
// semantic version order of the release tag name.
func latestRelease(ctx context.Context, repo Repository) (string, error) {
	return latestPatchRelease(ctx, repo, nil, nil)
}

// latestPatchRelease returns the latest patch release for a given Major and Minor version.
func latestPatchRelease(ctx context.Context, repo Repository, major, minor *uint) (string, error) {
	versions, err := repo.GetVersions(ctx)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get repository versions")
	}

	// Search for the latest release according to semantic version ordering.
	// Releases with tag name that are not in semver format are ignored.
	versionCandidates := []*version.Version{}

	for _, v := range versions {
		sv, err := version.ParseSemantic(v)
		if err != nil {
			// discard releases with tags that are not a valid semantic versions (the user can point explicitly to such releases)
			continue
		}

		if (major != nil && sv.Major() != *major) || (minor != nil && sv.Minor() != *minor) {
			// skip versions that don't match the desired Major.Minor version.
			continue
		}

		versionCandidates = append(versionCandidates, sv)
	}

	if len(versionCandidates) == 0 {
		return "", errors.New("failed to find releases tagged with a valid semantic version number")
	}

	// Sort parsed versions by semantic version order.
	sort.SliceStable(versionCandidates, func(i, j int) bool {
		// Prioritize release versions over pre-releases. For example v1.0.0 > v2.0.0-alpha
		// If both are pre-releases, sort by semantic version order as usual.
		if versionCandidates[j].PreRelease() == "" && versionCandidates[i].PreRelease() != "" {
			return false
		}
		if versionCandidates[i].PreRelease() == "" && versionCandidates[j].PreRelease() != "" {
			return true
		}

		return versionCandidates[j].LessThan(versionCandidates[i])
	})

	// Limit the number of searchable versions by 5.
	versionCandidates = versionCandidates[:min(5, len(versionCandidates))]

	for _, v := range versionCandidates {
		// Iterate through sorted versions and try to fetch a file from that release.
		// If it's completed successfully, we get the latest release.
		// Note: the fetched file will be cached and next time we will get it from the cache.
		versionString := "v" + v.String()
		_, err := repo.GetFile(ctx, versionString, metadataFile)
		if err != nil {
			if errors.Is(err, errNotFound) {
				// Ignore this version
				continue
			}

			return "", err
		}

		return versionString, nil
	}

	// If we reached this point, it means we didn't find any release.
	return "", errors.New("failed to find releases tagged with a valid semantic version number")
}
