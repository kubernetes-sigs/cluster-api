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

package internal

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGetMachinesForCluster(t *testing.T) {
	m := ManagementCluster{Client: &fakeClient{}}
	clusterKey := types.NamespacedName{
		Namespace: "my-namespace",
		Name:      "my-cluster",
	}
	machines, err := m.GetMachinesForCluster(context.Background(), clusterKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(machines) != 3 {
		t.Fatalf("expected 3 machines but found %d", len(machines))
	}

	// Test the OwnedControlPlaneMachines works
	machines, err = m.GetMachinesForCluster(context.Background(), clusterKey, OwnedControlPlaneMachines("my-control-plane"))
	if err != nil {
		t.Fatal(err)
	}
	if len(machines) != 1 {
		t.Fatalf("expected 1 control plane machine but got %d", len(machines))
	}

	// Test that the filters use AND logic instead of OR logic
	nameFilter := func(cluster clusterv1.Machine) bool {
		return cluster.Name == "first-machine"
	}
	machines, err = m.GetMachinesForCluster(context.Background(), clusterKey, OwnedControlPlaneMachines("my-control-plane"), nameFilter)
	if err != nil {
		t.Fatal(err)
	}
	if len(machines) != 1 {
		t.Fatalf("expected 1 control plane machine but got %d", len(machines))
	}
}

type fakeClient struct {
	client.Client
}

func (f *fakeClient) List(_ context.Context, list runtime.Object, opts ...client.ListOption) error {
	owned := true
	ownerRefs := []metav1.OwnerReference{
		{
			Kind:       "KubeadmControlPlane",
			Name:       "my-control-plane",
			Controller: &owned,
		},
	}
	myList := &clusterv1.MachineList{
		Items: []clusterv1.Machine{
			{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "first-machine",
					Namespace: "my-namespace",
					Labels: map[string]string{
						clusterv1.ClusterLabelName:             "my-cluster",
						clusterv1.MachineControlPlaneLabelName: "",
					},
					OwnerReferences: ownerRefs,
				},
			},
			{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "second-machine",
					Namespace: "my-namespace",
					Labels: map[string]string{
						clusterv1.ClusterLabelName: "my-cluster",
					},
				},
			},
			{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "third-machine",
					Namespace: "my-namespace",
					Labels: map[string]string{
						clusterv1.ClusterLabelName: "my-cluster",
					},
				},
			},
		},
	}
	myList.DeepCopyInto(list.(*clusterv1.MachineList))
	return nil
}
