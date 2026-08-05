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
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestKubeletVersionMatches(t *testing.T) {
	tests := []struct {
		name           string
		kubeletVersion string
		desiredVersion string
		want           bool
	}{
		{
			name:           "matches for equal versions",
			kubeletVersion: "v1.33.1",
			desiredVersion: "v1.33.1",
			want:           true,
		},
		{
			name:           "matches if only the build metadata differs",
			kubeletVersion: "v1.33.1+test",
			desiredVersion: "v1.33.1+test.0",
			want:           true,
		},
		{
			name:           "matches if only one of the versions has build metadata",
			kubeletVersion: "v1.33.1+test",
			desiredVersion: "v1.33.1",
			want:           true,
		},
		{
			name:           "does not match for a different patch version",
			kubeletVersion: "v1.33.0+test",
			desiredVersion: "v1.33.1+test.0",
			want:           false,
		},
		{
			name:           "does not match for a different minor version",
			kubeletVersion: "v1.32.1",
			desiredVersion: "v1.33.1",
			want:           false,
		},
		{
			name:           "does not match if the pre-release differs",
			kubeletVersion: "v1.33.1-alpha.1",
			desiredVersion: "v1.33.1",
			want:           false,
		},
		{
			name:           "falls back to string comparison for unparsable versions",
			kubeletVersion: "not-a-version",
			desiredVersion: "v1.33.1",
			want:           false,
		},
		{
			name:           "falls back to string comparison for equal unparsable versions",
			kubeletVersion: "not-a-version",
			desiredVersion: "not-a-version",
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(KubeletVersionMatches(tt.kubeletVersion, tt.desiredVersion)).To(Equal(tt.want))
		})
	}
}

func TestAggregateStatusVersions(t *testing.T) {
	t.Run("returns nil for empty versions", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(AggregateStatusVersions(nil)).To(BeNil())
	})

	t.Run("sorts semver first and then lexicographically", func(t *testing.T) {
		g := NewWithT(t)

		got := AggregateStatusVersions([]clusterv1.StatusVersion{
			{Version: "foo", Replicas: 1},
			{Version: "v1.30.10", Replicas: 1},
			{Version: "v1.30.2", Replicas: 1},
			{Version: "bar", Replicas: 1},
		})

		g.Expect(got).To(Equal([]clusterv1.StatusVersion{
			{Version: "v1.30.2", Replicas: 1},
			{Version: "v1.30.10", Replicas: 1},
			{Version: "bar", Replicas: 1},
			{Version: "foo", Replicas: 1},
		}))
	})

	t.Run("uses deterministic tie breakers for equivalent and non-sortable versions", func(t *testing.T) {
		g := NewWithT(t)

		got := AggregateStatusVersions([]clusterv1.StatusVersion{
			{Version: "v1.30.0+bbb", Replicas: 1},
			{Version: "v1.30.0+aaa", Replicas: 1},
			{Version: "v1.30.0", Replicas: 1},
			{Version: "1.30.0", Replicas: 1},
		})

		g.Expect(got).To(Equal([]clusterv1.StatusVersion{
			{Version: "1.30.0", Replicas: 1},
			{Version: "v1.30.0", Replicas: 1},
			{Version: "v1.30.0+bbb", Replicas: 1},
			{Version: "v1.30.0+aaa", Replicas: 1},
		}))
	})

	t.Run("sums replicas by version", func(t *testing.T) {
		g := NewWithT(t)

		got := AggregateStatusVersions([]clusterv1.StatusVersion{
			{Version: "v1.31.1", Replicas: 1},
			{Version: "v1.31.1", Replicas: 2},
			{Version: "v1.32.0", Replicas: 3},
			{Version: "v1.33.0"},
		})

		g.Expect(got).To(Equal([]clusterv1.StatusVersion{
			{Version: "v1.31.1", Replicas: 3},
			{Version: "v1.32.0", Replicas: 3},
			{Version: "v1.33.0", Replicas: 0},
		}))
	})
}

func TestVersionsFromMachines(t *testing.T) {
	t.Run("returns sorted versions from machine spec versions", func(t *testing.T) {
		g := NewWithT(t)

		machines := []*clusterv1.Machine{
			nil,
			{},
			{Spec: clusterv1.MachineSpec{Version: "v1.32.0"}},
			{Spec: clusterv1.MachineSpec{Version: "v1.31.1"}},
			{Spec: clusterv1.MachineSpec{Version: "v1.31.1"}},
			{Spec: clusterv1.MachineSpec{Version: "v1.31.1"}, Status: clusterv1.MachineStatus{NodeInfo: &corev1.NodeSystemInfo{KubeletVersion: "v1.30.0"}}},
			{Spec: clusterv1.MachineSpec{Version: "v1.31.1"}, Status: clusterv1.MachineStatus{NodeInfo: &corev1.NodeSystemInfo{KubeletVersion: "v1.31.1"}}},
		}

		g.Expect(VersionsFromMachines(machines)).To(Equal([]clusterv1.StatusVersion{
			{Version: "v1.30.0", Replicas: 1},
			{Version: "v1.31.1", Replicas: 3},
			{Version: "v1.32.0", Replicas: 1},
		}))
	})

	t.Run("keeps the spec version if the kubelet version only differs in build metadata", func(t *testing.T) {
		g := NewWithT(t)

		machines := []*clusterv1.Machine{
			{Spec: clusterv1.MachineSpec{Version: "v1.33.1+test.0"}, Status: clusterv1.MachineStatus{NodeInfo: &corev1.NodeSystemInfo{KubeletVersion: "v1.33.1+test"}}},
			{Spec: clusterv1.MachineSpec{Version: "v1.33.1+test.0"}, Status: clusterv1.MachineStatus{NodeInfo: &corev1.NodeSystemInfo{KubeletVersion: "v1.33.0+test"}}},
		}

		g.Expect(VersionsFromMachines(machines)).To(Equal([]clusterv1.StatusVersion{
			{Version: "v1.33.0+test", Replicas: 1},
			{Version: "v1.33.1+test.0", Replicas: 1},
		}))
	})

	t.Run("orders non-sortable build metadata by machine creation timestamp", func(t *testing.T) {
		g := NewWithT(t)

		base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		machines := []*clusterv1.Machine{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "newer",
					CreationTimestamp: metav1.NewTime(base.Add(time.Minute)),
				},
				Spec: clusterv1.MachineSpec{Version: "v1.32.0+aaa"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "older",
					CreationTimestamp: metav1.NewTime(base),
				},
				Spec: clusterv1.MachineSpec{Version: "v1.32.0+bbb"},
			},
		}

		g.Expect(VersionsFromMachines(machines)).To(Equal([]clusterv1.StatusVersion{
			{Version: "v1.32.0+bbb", Replicas: 1},
			{Version: "v1.32.0+aaa", Replicas: 1},
		}))
	})
}
