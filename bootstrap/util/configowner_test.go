/*
Copyright 2019 The Kubernetes Authors.

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

package util

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

func TestGetConfigOwnerSuccess(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clusterv1.AddToScheme(scheme); err != nil {
		t.Fatal("failed to register Cluster API objects to scheme")
	}

	myMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-machine",
			Namespace: "my-ns",
			Labels: map[string]string{
				clusterv1.MachineControlPlaneLabelName: "",
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: "my-cluster",
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: pointer.StringPtr("my-data-secret"),
			},
		},
		Status: clusterv1.MachineStatus{
			InfrastructureReady: true,
		},
	}

	c := fake.NewFakeClientWithScheme(scheme, myMachine)
	objm := metav1.ObjectMeta{
		OwnerReferences: []metav1.OwnerReference{
			{
				Kind:       "Machine",
				APIVersion: clusterv1.GroupVersion.String(),
				Name:       "my-machine",
			},
		},
		Namespace: "my-ns",
		Name:      "my-resource-owned-by-machine",
	}
	configOwner, err := GetConfigOwner(context.TODO(), c, objm)
	if err != nil {
		t.Fatalf("did not expect an error but found one: %v", err)
	}
	if configOwner == nil {
		t.Fatal("expected a configOwner but got nil")
	}
	if configOwner.ClusterName() != "my-cluster" {
		t.Fatalf("did not expect ClusterName: %q", configOwner.ClusterName())
	}
	if !configOwner.IsInfrastructureReady() {
		t.Fatalf("did not expect InfrastructureReady: %v", configOwner.IsInfrastructureReady())
	}
	if configOwner.DataSecretName() == nil {
		t.Fatalf("did not expect DataSecretName: %v", configOwner.DataSecretName())
	}
	if !configOwner.IsControlPlaneMachine() {
		t.Fatalf("did not expect IsControlPlane: %v", configOwner.IsControlPlaneMachine())
	}
}

func TestGetConfigOwnerNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clusterv1.AddToScheme(scheme); err != nil {
		t.Fatal("failed to register cluster api objects to scheme")
	}

	c := fake.NewFakeClientWithScheme(scheme)
	objm := metav1.ObjectMeta{
		OwnerReferences: []metav1.OwnerReference{
			{
				Kind:       "Machine",
				APIVersion: clusterv1.GroupVersion.String(),
				Name:       "my-machine",
			},
		},
		Namespace: "my-ns",
		Name:      "my-resource-owned-by-machine",
	}
	_, err := GetConfigOwner(context.TODO(), c, objm)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetConfigOwnerNoOwner(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clusterv1.AddToScheme(scheme); err != nil {
		t.Fatal("failed to register cluster api objects to scheme")
	}

	c := fake.NewFakeClientWithScheme(scheme)
	objm := metav1.ObjectMeta{
		OwnerReferences: []metav1.OwnerReference{},
		Namespace:       "my-ns",
		Name:            "my-resource-owned-by-machine",
	}
	configOwner, err := GetConfigOwner(context.TODO(), c, objm)
	if err != nil {
		t.Fatalf("did not expect an error but found one: %v", err)
	}
	if configOwner != nil {
		t.Fatalf("expected nil, but got %v", configOwner)
	}
}
