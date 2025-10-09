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

package migrate

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestConverter_ConvertResource(t *testing.T) {
	converter, err := NewConverter(clusterv1.GroupVersion)
	if err != nil {
		t.Fatalf("NewConverter() failed: %v", err)
	}

	t.Run("convert v1beta1 Cluster to v1beta2", func(t *testing.T) {
		cluster := &clusterv1beta1.Cluster{}
		cluster.SetName("test-cluster")
		cluster.SetNamespace("default")

		info := ResourceInfo{
			GroupVersionKind: schema.GroupVersionKind{
				Group:   "cluster.x-k8s.io",
				Version: "v1beta1",
				Kind:    "Cluster",
			},
			Name:      "test-cluster",
			Namespace: "default",
		}

		result := converter.ConvertResource(info, cluster)

		if result.Error != nil {
			t.Fatalf("ConvertResource() failed: %v", result.Error)
		}
		if !result.Converted {
			t.Error("Expected resource to be converted")
		}
		if result.Object == nil {
			t.Fatal("Converted object is nil")
		}

		// Verify the converted object is v1beta2
		convertedCluster, ok := result.Object.(*clusterv1.Cluster)
		if !ok {
			t.Fatalf("Expected *clusterv1beta2.Cluster, got %T", result.Object)
		}
		if convertedCluster.Name != "test-cluster" {
			t.Errorf("Expected name test-cluster, got %s", convertedCluster.Name)
		}
	})

	t.Run("no-op for v1beta2 resource", func(t *testing.T) {
		cluster := &clusterv1.Cluster{}
		cluster.SetName("test-cluster")
		cluster.SetNamespace("default")

		info := ResourceInfo{
			GroupVersionKind: schema.GroupVersionKind{
				Group:   "cluster.x-k8s.io",
				Version: "v1beta2",
				Kind:    "Cluster",
			},
			Name:      "test-cluster",
			Namespace: "default",
		}

		result := converter.ConvertResource(info, cluster)

		if result.Error != nil {
			t.Fatalf("ConvertResource() failed: %v", result.Error)
		}
		if result.Converted {
			t.Error("Expected resource not to be converted")
		}
		if len(result.Warnings) == 0 {
			t.Error("Expected warning for already-converted resource")
		}
	})

	t.Run("convert v1beta1 MachineDeployment to v1beta2", func(t *testing.T) {
		md := &clusterv1beta1.MachineDeployment{}
		md.SetName("test-md")
		md.SetNamespace("default")

		info := ResourceInfo{
			GroupVersionKind: schema.GroupVersionKind{
				Group:   "cluster.x-k8s.io",
				Version: "v1beta1",
				Kind:    "MachineDeployment",
			},
			Name:      "test-md",
			Namespace: "default",
		}

		result := converter.ConvertResource(info, md)

		if result.Error != nil {
			t.Fatalf("ConvertResource() failed: %v", result.Error)
		}
		if !result.Converted {
			t.Error("Expected resource to be converted")
		}
		if result.Object == nil {
			t.Fatal("Converted object is nil")
		}

		// Verify the converted object is v1beta2
		convertedMD, ok := result.Object.(*clusterv1.MachineDeployment)
		if !ok {
			t.Fatalf("Expected *clusterv1beta2.MachineDeployment, got %T", result.Object)
		}
		if convertedMD.Name != "test-md" {
			t.Errorf("Expected name test-md, got %s", convertedMD.Name)
		}
	})
}
