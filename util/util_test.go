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

package util

import (
	"context"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMachineToInfrastructureMapFunc(t *testing.T) {
	var testcases = []struct {
		name    string
		input   schema.GroupVersionKind
		request handler.MapObject
		output  []reconcile.Request
	}{
		{
			name: "should reconcile infra-1",
			input: schema.GroupVersionKind{
				Group:   "foo.cluster.x-k8s.io",
				Version: "v1alpha3",
				Kind:    "TestMachine",
			},
			request: handler.MapObject{
				Object: &clusterv1.Machine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-1",
					},
					Spec: clusterv1.MachineSpec{
						InfrastructureRef: corev1.ObjectReference{
							APIVersion: "foo.cluster.x-k8s.io/v1alpha3",
							Kind:       "TestMachine",
							Name:       "infra-1",
						},
					},
				},
			},
			output: []reconcile.Request{
				{
					NamespacedName: client.ObjectKey{
						Namespace: "default",
						Name:      "infra-1",
					},
				},
			},
		},
		{
			name: "should return no matching reconcile requests",
			input: schema.GroupVersionKind{
				Group:   "foo.cluster.x-k8s.io",
				Version: "v1alpha3",
				Kind:    "TestMachine",
			},
			request: handler.MapObject{
				Object: &clusterv1.Machine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-1",
					},
					Spec: clusterv1.MachineSpec{
						InfrastructureRef: corev1.ObjectReference{
							APIVersion: "bar.cluster.x-k8s.io/v1alpha3",
							Kind:       "TestMachine",
							Name:       "bar-1",
						},
					},
				},
			},
			output: nil,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			fn := MachineToInfrastructureMapFunc(tc.input)
			out := fn(tc.request)
			if !reflect.DeepEqual(out, tc.output) {
				t.Fatalf("Unexpected output. Got: %v, Want: %v", out, tc.output)
			}
		})
	}
}

func TestClusterToInfrastructureMapFunc(t *testing.T) {
	var testcases = []struct {
		name    string
		input   schema.GroupVersionKind
		request handler.MapObject
		output  []reconcile.Request
	}{
		{
			name: "should reconcile infra-1",
			input: schema.GroupVersionKind{
				Group:   "foo.cluster.x-k8s.io",
				Version: "v1alpha3",
				Kind:    "TestCluster",
			},
			request: handler.MapObject{
				Object: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-1",
					},
					Spec: clusterv1.ClusterSpec{
						InfrastructureRef: &corev1.ObjectReference{
							APIVersion: "foo.cluster.x-k8s.io/v1alpha3",
							Kind:       "TestCluster",
							Name:       "infra-1",
						},
					},
				},
			},
			output: []reconcile.Request{
				{
					NamespacedName: client.ObjectKey{
						Namespace: "default",
						Name:      "infra-1",
					},
				},
			},
		},
		{
			name: "should return no matching reconcile requests",
			input: schema.GroupVersionKind{
				Group:   "foo.cluster.x-k8s.io",
				Version: "v1alpha3",
				Kind:    "TestCluster",
			},
			request: handler.MapObject{
				Object: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "test-1",
					},
					Spec: clusterv1.ClusterSpec{
						InfrastructureRef: &corev1.ObjectReference{
							APIVersion: "bar.cluster.x-k8s.io/v1alpha3",
							Kind:       "TestCluster",
							Name:       "bar-1",
						},
					},
				},
			},
			output: nil,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			fn := ClusterToInfrastructureMapFunc(tc.input)
			out := fn(tc.request)
			if !reflect.DeepEqual(out, tc.output) {
				t.Fatalf("Unexpected output. Got: %v, Want: %v", out, tc.output)
			}
		})
	}
}

func TestHasOwner(t *testing.T) {
	tests := []struct {
		name     string
		refList  []metav1.OwnerReference
		expected bool
	}{
		{
			name: "no ownership",
		},
		{
			name: "owned by cluster",
			refList: []metav1.OwnerReference{
				{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
				},
			},
			expected: true,
		},
		{
			name: "owned by something else",
			refList: []metav1.OwnerReference{
				{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
				},
			},
		},
		{
			name: "owner by a deployment",
			refList: []metav1.OwnerReference{
				{
					Kind:       "MachineDeployment",
					APIVersion: clusterv1.GroupVersion.String(),
				},
			},
			expected: true,
		},
		{
			name: "right kind, wrong apiversion",
			refList: []metav1.OwnerReference{
				{
					Kind:       "MachineDeployment",
					APIVersion: "wrong/v2",
				},
			},
		},
		{
			name: "right apiversion, wrong kind",
			refList: []metav1.OwnerReference{
				{
					Kind:       "Machine",
					APIVersion: clusterv1.GroupVersion.String(),
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := HasOwner(
				test.refList,
				clusterv1.GroupVersion.String(),
				[]string{"MachineDeployment", "Cluster"},
			)
			if test.expected != result {
				t.Errorf("expected hasOwner to be %v, got %v", test.expected, result)
			}
		})
	}
}

func TestPointsTo(t *testing.T) {
	targetID := "fri3ndsh1p"

	meta := metav1.ObjectMeta{
		UID: types.UID(targetID),
	}

	tests := []struct {
		name     string
		refIDs   []string
		expected bool
	}{
		{
			name: "empty owner list",
		},
		{
			name:   "single wrong owner ref",
			refIDs: []string{"m4g1c"},
		},
		{
			name:     "single right owner ref",
			refIDs:   []string{targetID},
			expected: true,
		},
		{
			name:   "multiple wrong refs",
			refIDs: []string{"m4g1c", "h4rm0ny"},
		},
		{
			name:     "multiple refs one right",
			refIDs:   []string{"m4g1c", targetID},
			expected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pointer := &metav1.ObjectMeta{}

			for _, ref := range test.refIDs {
				pointer.OwnerReferences = append(pointer.OwnerReferences, metav1.OwnerReference{
					UID: types.UID(ref),
				})
			}

			result := PointsTo(pointer.OwnerReferences, &meta)
			if result != test.expected {
				t.Errorf("expected %v, got %v", test.expected, result)
			}
		})
	}
}

func TestGetOwnerClusterSuccessByName(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clusterv1.AddToScheme(scheme); err != nil {
		t.Fatal("failed to register cluster api objects to scheme")
	}

	myCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cluster",
			Namespace: "my-ns",
		},
	}

	c := fake.NewFakeClientWithScheme(scheme, myCluster)
	objm := metav1.ObjectMeta{
		OwnerReferences: []metav1.OwnerReference{
			{
				Kind:       "Cluster",
				APIVersion: clusterv1.GroupVersion.String(),
				Name:       "my-cluster",
			},
		},
		Namespace: "my-ns",
		Name:      "my-resource-owned-by-cluster",
	}
	cluster, err := GetOwnerCluster(context.TODO(), c, objm)
	if err != nil {
		t.Fatalf("did not expect an error but found one: %v", err)
	}
	if cluster == nil {
		t.Fatal("expected a cluster but got nil")
	}
}

func TestGetOwnerMachineSuccessByName(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clusterv1.AddToScheme(scheme); err != nil {
		t.Fatal("failed to register cluster api objects to scheme")
	}

	myMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-machine",
			Namespace: "my-ns",
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
	machine, err := GetOwnerMachine(context.TODO(), c, objm)
	if err != nil {
		t.Fatalf("did not expect an error but found one: %v", err)
	}
	if machine == nil {
		t.Fatal("expected a machine but got nil")
	}
}
