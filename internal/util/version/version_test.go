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
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestAddMachineKubeletVersions(t *testing.T) {
	t.Run("adds machine kubelet versions", func(t *testing.T) {
		g := NewWithT(t)

		versionCounts := map[string]int32{}
		machines := []*clusterv1.Machine{
			{Status: clusterv1.MachineStatus{NodeInfo: &corev1.NodeSystemInfo{KubeletVersion: "v1.31.1"}}},
			{Status: clusterv1.MachineStatus{NodeInfo: &corev1.NodeSystemInfo{KubeletVersion: "v1.31.1"}}},
			{Status: clusterv1.MachineStatus{NodeInfo: &corev1.NodeSystemInfo{KubeletVersion: "v1.32.0"}}},
			{Status: clusterv1.MachineStatus{NodeInfo: &corev1.NodeSystemInfo{}}},
			{Status: clusterv1.MachineStatus{}},
		}

		AddMachineKubeletVersions(versionCounts, machines)

		g.Expect(versionCounts).To(Equal(map[string]int32{
			"v1.31.1": 2,
			"v1.32.0": 1,
		}))
	})
}

func TestAddStatusVersions(t *testing.T) {
	t.Run("adds status versions and handles nil replicas", func(t *testing.T) {
		g := NewWithT(t)

		versionCounts := map[string]int32{"v1.31.1": 1}
		versions := []clusterv1.StatusVersion{
			{Version: "v1.31.1", Replicas: ptr.To[int32](2)},
			{Version: "v1.32.0", Replicas: ptr.To[int32](3)},
			{Version: "v1.33.0"},
		}

		AddStatusVersions(versionCounts, versions)

		g.Expect(versionCounts).To(Equal(map[string]int32{
			"v1.31.1": 3,
			"v1.32.0": 3,
			"v1.33.0": 0,
		}))
	})
}

func TestStatusVersionsFromCountMap(t *testing.T) {
	t.Run("returns nil for empty map", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(StatusVersionsFromCountMap(map[string]int32{})).To(BeNil())
	})

	t.Run("sorts semver first and then lexicographically", func(t *testing.T) {
		g := NewWithT(t)

		got := StatusVersionsFromCountMap(map[string]int32{
			"foo":      1,
			"v1.30.10": 1,
			"v1.30.2":  1,
			"bar":      1,
		})

		g.Expect(got).To(Equal([]clusterv1.StatusVersion{
			{Version: "v1.30.2", Replicas: ptr.To[int32](1)},
			{Version: "v1.30.10", Replicas: ptr.To[int32](1)},
			{Version: "bar", Replicas: ptr.To[int32](1)},
			{Version: "foo", Replicas: ptr.To[int32](1)},
		}))
	})
}

func TestVersionsFromMachines(t *testing.T) {
	t.Run("returns sorted versions from machines", func(t *testing.T) {
		g := NewWithT(t)

		machines := []*clusterv1.Machine{
			{Status: clusterv1.MachineStatus{NodeInfo: &corev1.NodeSystemInfo{KubeletVersion: "v1.32.0"}}},
			{Status: clusterv1.MachineStatus{NodeInfo: &corev1.NodeSystemInfo{KubeletVersion: "v1.31.1"}}},
			{Status: clusterv1.MachineStatus{NodeInfo: &corev1.NodeSystemInfo{KubeletVersion: "v1.31.1"}}},
		}

		g.Expect(VersionsFromMachines(machines)).To(Equal([]clusterv1.StatusVersion{
			{Version: "v1.31.1", Replicas: ptr.To[int32](2)},
			{Version: "v1.32.0", Replicas: ptr.To[int32](1)},
		}))
	})
}
