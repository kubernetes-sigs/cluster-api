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

package version

import (
	"sort"

	"github.com/blang/semver/v4"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	capiversion "sigs.k8s.io/cluster-api/util/version"
)

// AddMachineKubeletVersions adds machine kubelet versions to versionCounts.
func AddMachineKubeletVersions(versionCounts map[string]int32, machines []*clusterv1.Machine) {
	if versionCounts == nil {
		return
	}

	for _, machine := range machines {
		if machine.Status.NodeInfo == nil || machine.Status.NodeInfo.KubeletVersion == "" {
			continue
		}
		versionCounts[machine.Status.NodeInfo.KubeletVersion]++
	}
}

// AddStatusVersions adds versions and replicas to versionCounts.
func AddStatusVersions(versionCounts map[string]int32, versions []clusterv1.StatusVersion) {
	if versionCounts == nil {
		return
	}

	for _, statusVersion := range versions {
		versionCounts[statusVersion.Version] += ptr.Deref(statusVersion.Replicas, 0)
	}
}

// StatusVersionsFromCountMap converts versionCounts into sorted status versions.
func StatusVersionsFromCountMap(versionCounts map[string]int32) []clusterv1.StatusVersion {
	if len(versionCounts) == 0 {
		return nil
	}

	versions := make([]clusterv1.StatusVersion, 0, len(versionCounts))
	for v, replicas := range versionCounts {
		versions = append(versions, clusterv1.StatusVersion{
			Version:  v,
			Replicas: ptr.To(replicas),
		})
	}

	sort.Slice(versions, func(i, j int) bool {
		vi, erri := semver.ParseTolerant(versions[i].Version)
		vj, errj := semver.ParseTolerant(versions[j].Version)
		if erri == nil && errj == nil {
			if cmp := capiversion.Compare(vi, vj, capiversion.WithBuildTags()); cmp != 0 {
				return cmp < 0
			}
		}
		if erri == nil {
			return true
		}
		if errj == nil {
			return false
		}
		return versions[i].Version < versions[j].Version
	})

	return versions
}

// VersionsFromMachines returns sorted versions aggregated from machine kubelet versions.
func VersionsFromMachines(machines []*clusterv1.Machine) []clusterv1.StatusVersion {
	versionCounts := map[string]int32{}
	AddMachineKubeletVersions(versionCounts, machines)
	return StatusVersionsFromCountMap(versionCounts)
}
