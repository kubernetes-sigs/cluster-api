/*
Copyright 2026 The Kubernetes Authors.

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

// Package version provides helpers for aggregating and sorting Kubernetes versions.
package version

import (
	"sort"

	"github.com/blang/semver/v4"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	capiversion "sigs.k8s.io/cluster-api/util/version"
)

type orderedStatusVersion struct {
	clusterv1.StatusVersion
	order int
}

// AggregateStatusVersions returns versions aggregated by version.
func AggregateStatusVersions(versions []clusterv1.StatusVersion) []clusterv1.StatusVersion {
	if len(versions) == 0 {
		return nil
	}

	versionIndexes := map[string]int{}
	aggregatedVersions := []orderedStatusVersion{}
	for _, statusVersion := range versions {
		if statusVersion.Version == "" {
			continue
		}
		if index, ok := versionIndexes[statusVersion.Version]; ok {
			aggregatedVersions[index].Replicas += statusVersion.Replicas
			continue
		}
		versionIndexes[statusVersion.Version] = len(aggregatedVersions)
		aggregatedVersions = append(aggregatedVersions, orderedStatusVersion{
			StatusVersion: clusterv1.StatusVersion{
				Version:  statusVersion.Version,
				Replicas: statusVersion.Replicas,
			},
			order: len(aggregatedVersions),
		})
	}

	return statusVersions(aggregatedVersions)
}

// VersionsFromMachines returns versions aggregated from machine spec versions.
// If build metadata makes two versions non-sortable, the machine creation timestamp/name order is used as a fallback.
func VersionsFromMachines(machines []*clusterv1.Machine) []clusterv1.StatusVersion {
	if len(machines) == 0 {
		return nil
	}

	sortedMachines := append([]*clusterv1.Machine(nil), machines...)
	sort.SliceStable(sortedMachines, func(i, j int) bool {
		if sortedMachines[i] == nil {
			return false
		}
		if sortedMachines[j] == nil {
			return true
		}
		if sortedMachines[i].CreationTimestamp.Equal(&sortedMachines[j].CreationTimestamp) {
			return sortedMachines[i].Name < sortedMachines[j].Name
		}
		return sortedMachines[i].CreationTimestamp.Before(&sortedMachines[j].CreationTimestamp)
	})

	versionIndexes := map[string]int{}
	versions := []orderedStatusVersion{}
	for _, machine := range sortedMachines {
		if machine == nil || machine.Spec.Version == "" {
			continue
		}
		if index, ok := versionIndexes[machine.Spec.Version]; ok {
			versions[index].Replicas++
			continue
		}
		versionIndexes[machine.Spec.Version] = len(versions)
		versions = append(versions, orderedStatusVersion{
			StatusVersion: clusterv1.StatusVersion{
				Version:  machine.Spec.Version,
				Replicas: 1,
			},
			order: len(versions),
		})
	}

	return statusVersions(versions)
}

func statusVersions(versions []orderedStatusVersion) []clusterv1.StatusVersion {
	if len(versions) == 0 {
		return nil
	}

	sort.SliceStable(versions, func(i, j int) bool {
		return statusVersionLess(versions[i], versions[j])
	})

	out := make([]clusterv1.StatusVersion, 0, len(versions))
	for _, version := range versions {
		out = append(out, version.StatusVersion)
	}
	return out
}

func statusVersionLess(i, j orderedStatusVersion) bool {
	vi, erri := semver.ParseTolerant(i.Version)
	vj, errj := semver.ParseTolerant(j.Version)
	if erri == nil && errj == nil {
		switch cmp := capiversion.Compare(vi, vj, capiversion.WithBuildTags()); cmp {
		case -1:
			return true
		case 0:
			if i.Version != j.Version {
				return i.Version < j.Version
			}
			return i.order < j.order
		case 1:
			return false
		default:
			return i.order < j.order
		}
	}
	if erri == nil {
		return true
	}
	if errj == nil {
		return false
	}
	if i.Version != j.Version {
		return i.Version < j.Version
	}
	return i.order < j.order
}
