/*
Copyright 2025 The Kubernetes Authors.

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

package convert

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/scheme"
)

func TestConvertResource(t *testing.T) {
	targetGV := clusterv1.GroupVersion

	t.Run("convert v1beta1 Cluster to v1beta2", func(t *testing.T) {
		cluster := &clusterv1beta1.Cluster{}
		cluster.SetName("test-cluster")
		cluster.SetNamespace("default")
		cluster.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "cluster.x-k8s.io",
			Version: "v1beta1",
			Kind:    "Cluster",
		})

		convertedObj, wasConverted, err := convertResource(cluster, targetGV, scheme.Scheme, "cluster.x-k8s.io")

		if err != nil {
			t.Fatalf("convertResource() failed: %v", err)
		}
		if !wasConverted {
			t.Error("Expected resource to be converted")
		}
		if convertedObj == nil {
			t.Fatal("Converted object is nil")
		}

		// Verify the converted object is v1beta2.
		convertedCluster, ok := convertedObj.(*clusterv1.Cluster)
		if !ok {
			t.Fatalf("Expected *clusterv1.Cluster, got %T", convertedObj)
		}
		if convertedCluster.Name != "test-cluster" {
			t.Errorf("Expected name test-cluster, got %s", convertedCluster.Name)
		}

		// Verify GVK is set correctly.
		gvk := convertedCluster.GetObjectKind().GroupVersionKind()
		if gvk.Version != "v1beta2" {
			t.Errorf("Expected version v1beta2, got %s", gvk.Version)
		}
	})

	t.Run("no-op for v1beta2 resource", func(t *testing.T) {
		cluster := &clusterv1.Cluster{}
		cluster.SetName("test-cluster")
		cluster.SetNamespace("default")
		cluster.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "cluster.x-k8s.io",
			Version: "v1beta2",
			Kind:    "Cluster",
		})

		convertedObj, wasConverted, err := convertResource(cluster, targetGV, scheme.Scheme, "cluster.x-k8s.io")

		if err != nil {
			t.Fatalf("convertResource() failed: %v", err)
		}
		if wasConverted {
			t.Error("Expected resource not to be converted")
		}
		if convertedObj != cluster {
			t.Error("Expected original object to be returned")
		}
	})

	t.Run("convert v1beta1 MachineDeployment to v1beta2", func(t *testing.T) {
		md := &clusterv1beta1.MachineDeployment{}
		md.SetName("test-md")
		md.SetNamespace("default")
		md.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "cluster.x-k8s.io",
			Version: "v1beta1",
			Kind:    "MachineDeployment",
		})

		convertedObj, wasConverted, err := convertResource(md, targetGV, scheme.Scheme, "cluster.x-k8s.io")

		if err != nil {
			t.Fatalf("convertResource() failed: %v", err)
		}
		if !wasConverted {
			t.Error("Expected resource to be converted")
		}
		if convertedObj == nil {
			t.Fatal("Converted object is nil")
		}

		// Verify the converted object is v1beta2.
		convertedMD, ok := convertedObj.(*clusterv1.MachineDeployment)
		if !ok {
			t.Fatalf("Expected *clusterv1.MachineDeployment, got %T", convertedObj)
		}
		if convertedMD.Name != "test-md" {
			t.Errorf("Expected name test-md, got %s", convertedMD.Name)
		}
	})
}

func TestShouldConvert(t *testing.T) {
	tests := []struct {
		name          string
		gvk           schema.GroupVersionKind
		targetVersion string
		want          bool
	}{
		{
			name: "should convert v1beta1 to v1beta2",
			gvk: schema.GroupVersionKind{
				Group:   "cluster.x-k8s.io",
				Version: "v1beta1",
				Kind:    "Cluster",
			},
			targetVersion: "v1beta2",
			want:          true,
		},
		{
			name: "should not convert v1beta2 to v1beta2",
			gvk: schema.GroupVersionKind{
				Group:   "cluster.x-k8s.io",
				Version: "v1beta2",
				Kind:    "Cluster",
			},
			targetVersion: "v1beta2",
			want:          false,
		},
		{
			name: "should not convert non-CAPI resource",
			gvk: schema.GroupVersionKind{
				Group:   "infrastructure.cluster.x-k8s.io",
				Version: "v1beta1",
				Kind:    "DockerCluster",
			},
			targetVersion: "v1beta2",
			want:          false,
		},
		{
			name: "should not convert bootstrap resource",
			gvk: schema.GroupVersionKind{
				Group:   "bootstrap.cluster.x-k8s.io",
				Version: "v1beta1",
				Kind:    "KubeadmConfig",
			},
			targetVersion: "v1beta2",
			want:          false,
		},
		{
			name: "should not convert controlplane resource",
			gvk: schema.GroupVersionKind{
				Group:   "controlplane.cluster.x-k8s.io",
				Version: "v1beta1",
				Kind:    "KubeadmControlPlane",
			},
			targetVersion: "v1beta2",
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldConvert(tt.gvk, tt.targetVersion, "cluster.x-k8s.io")
			if got != tt.want {
				t.Errorf("shouldConvert() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetInfoMessage(t *testing.T) {
	tests := []struct {
		name          string
		gvk           schema.GroupVersionKind
		targetVersion string
		wantContains  string
	}{
		{
			name: "already at target version",
			gvk: schema.GroupVersionKind{
				Group:   "cluster.x-k8s.io",
				Version: "v1beta2",
				Kind:    "Cluster",
			},
			targetVersion: "v1beta2",
			wantContains:  "already at version",
		},
		{
			name: "non-CAPI resource",
			gvk: schema.GroupVersionKind{
				Group:   "infrastructure.cluster.x-k8s.io",
				Version: "v1beta1",
				Kind:    "DockerCluster",
			},
			targetVersion: "v1beta2",
			wantContains:  "Skipping non-cluster.x-k8s.io",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getInfoMessage(tt.gvk, tt.targetVersion, "cluster.x-k8s.io")
			if got == "" {
				t.Error("Expected non-empty info message")
			}
			if tt.wantContains != "" && !contains(got, tt.wantContains) {
				t.Errorf("getInfoMessage() = %q, want to contain %q", got, tt.wantContains)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
