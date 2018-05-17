/*
Copyright 2018 The Kubernetes Authors.

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

package v1alpha1_test

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1/testutil"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
)

func crudAccessToClusterClient(t *testing.T, cs *clientset.Clientset) {
	instance := testutil.GetVanillaCluster()
	instance.Name = "instance-1"

	expected := instance

	// When sending a storage request for a valid config,
	// it should provide CRUD access to the object.
	client := cs.ClusterV1alpha1().Clusters("cluster-test-valid")

	// Test that the create request returns success.
	actual, err := client.Create(&instance)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Delete(instance.Name, &metav1.DeleteOptions{})
	if !reflect.DeepEqual(actual.Spec, expected.Spec) {
		t.Fatalf(
			"Default fields were not set correctly.\nActual:\t%+v\nExpected:\t%+v",
			actual.Spec, expected.Spec)
	}

	// Test getting the created item for list requests.
	result, err := client.List(metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if itemLength := len(result.Items); itemLength != 1 {
		t.Fatalf("Number of items in Items list should be 1, but is %d.", itemLength)
	}
	if resultSpec := result.Items[0].Spec; !reflect.DeepEqual(resultSpec, expected.Spec) {
		t.Fatalf(
			"Item returned from list is not equal to the expected item.\nActual:\t%+v\nExpected:\t%+v",
			resultSpec, expected.Spec)
	}

	// Test getting the created item for get requests.
	actual, err = client.Get(instance.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual.Spec, expected.Spec) {
		t.Fatalf(
			"Item returned from get is not equal to the expected item.\nActual:\t%+v\nExpected:\t%+v",
			actual.Spec, expected.Spec)
	}

	// Test deleting the item for delete requests.
	actual.Finalizers = nil
	if _, updateErr := client.Update(actual); updateErr != nil {
		t.Fatal(updateErr)
	}
	if deleteErr := client.Delete(instance.Name, &metav1.DeleteOptions{}); deleteErr != nil {
		t.Fatal(deleteErr)
	}
	result, err = client.List(metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if itemLength := len(result.Items); itemLength != 0 {
		t.Fatalf("Number of items in Items list should be 0, but is %d.", itemLength)
	}
}

func clusterValidationTest(t *testing.T, cs *clientset.Clientset) {
	tests := []struct {
		name        string
		cluster     v1alpha1.Cluster
		errExpected bool
	}{
		{
			name: "missing services",
			cluster: v1alpha1.Cluster{
				Spec: v1alpha1.ClusterSpec{
					ClusterNetwork: v1alpha1.ClusterNetworkingConfig{
						Pods: v1alpha1.NetworkRanges{
							CIDRBlocks: []string{"192.168.0.0/16"},
						},
						ServiceDomain: "cluster.local",
					},
				},
			},
			errExpected: true,
		},
		{
			name: "missing pods",
			cluster: v1alpha1.Cluster{
				Spec: v1alpha1.ClusterSpec{
					ClusterNetwork: v1alpha1.ClusterNetworkingConfig{
						Services: v1alpha1.NetworkRanges{
							CIDRBlocks: []string{"10.96.0.0/12"},
						},
						ServiceDomain: "cluster.local",
					},
				},
			},
			errExpected: true,
		},
		{
			name: "missing ServiceDomain",
			cluster: v1alpha1.Cluster{
				Spec: v1alpha1.ClusterSpec{
					ClusterNetwork: v1alpha1.ClusterNetworkingConfig{
						Services: v1alpha1.NetworkRanges{
							CIDRBlocks: []string{"10.96.0.0/12"},
						},
						Pods: v1alpha1.NetworkRanges{
							CIDRBlocks: []string{"192.168.0.0/16"},
						},
					},
				},
			},
			errExpected: true,
		},
		{
			name: "positive test case",
			cluster: v1alpha1.Cluster{
				Spec: v1alpha1.ClusterSpec{
					ClusterNetwork: v1alpha1.ClusterNetworkingConfig{
						Services: v1alpha1.NetworkRanges{
							CIDRBlocks: []string{"10.96.0.0/12"},
						},
						Pods: v1alpha1.NetworkRanges{
							CIDRBlocks: []string{"192.168.0.0/16"},
						},
						ServiceDomain: "cluster.local",
					},
				},
			},
			errExpected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.cluster.Name = "cluster1"
			client := cs.ClusterV1alpha1().Clusters("cluster-test-valid")

			if _, err := client.Create(&tt.cluster); (err != nil) != tt.errExpected {
				t.Fatal(err)
			}
			client.Delete(tt.cluster.Name, &metav1.DeleteOptions{})
		})
	}
}
