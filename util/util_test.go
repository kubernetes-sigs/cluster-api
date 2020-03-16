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
	"testing"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMachineToInfrastructureMapFunc(t *testing.T) {
	g := NewWithT(t)

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
			g.Expect(out).To(Equal(tc.output))
		})
	}
}

func TestClusterToInfrastructureMapFunc(t *testing.T) {
	g := NewWithT(t)

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
			g.Expect(out).To(Equal(tc.output))
		})
	}
}

func TestHasOwner(t *testing.T) {
	g := NewWithT(t)

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
			g.Expect(result).To(Equal(test.expected))
		})
	}
}

func TestPointsTo(t *testing.T) {
	g := NewWithT(t)

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

			g.Expect(PointsTo(pointer.OwnerReferences, &meta)).To(Equal(test.expected))
		})
	}
}

func TestGetOwnerClusterSuccessByName(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

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
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cluster).NotTo(BeNil())
}

func TestGetOwnerMachineSuccessByName(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

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
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(machine).NotTo(BeNil())
}

func TestGetMachinesForCluster(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cluster",
			Namespace: "my-ns",
		},
	}

	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-machine",
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				clusterv1.ClusterLabelName: cluster.Name,
			},
		},
	}

	machineDifferentClusterNameSameNamespace := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-machine",
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				clusterv1.ClusterLabelName: "other-cluster",
			},
		},
	}

	machineSameClusterNameDifferentNamespace := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-machine",
			Namespace: "other-ns",
			Labels: map[string]string{
				clusterv1.ClusterLabelName: cluster.Name,
			},
		},
	}

	c := fake.NewFakeClientWithScheme(
		scheme,
		machine,
		machineDifferentClusterNameSameNamespace,
		machineSameClusterNameDifferentNamespace,
	)

	machines, err := GetMachinesForCluster(context.Background(), c, cluster)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(machines.Items).To(HaveLen(1))
	g.Expect(machines.Items[0].Labels[clusterv1.ClusterLabelName]).To(Equal(cluster.Name))
}

func TestModifyImageTag(t *testing.T) {
	g := NewGomegaWithT(t)
	t.Run("should ensure image is a docker compatible tag", func(t *testing.T) {
		testTag := "v1.17.4+build1"
		image := "example.com/image:1.17.3"
		res, err := ModifyImageTag(image, testTag)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res).To(Equal("example.com/image:v1.17.4_build1"))
	})
}

func TestEnsureOwnerRef(t *testing.T) {
	g := NewGomegaWithT(t)

	t.Run("should set ownerRef on an empty list", func(t *testing.T) {
		obj := &clusterv1.Machine{}
		ref := metav1.OwnerReference{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
			Name:       "test-cluster",
		}
		obj.OwnerReferences = EnsureOwnerRef(obj.OwnerReferences, ref)
		g.Expect(obj.OwnerReferences).Should(ContainElement(ref))
	})

	t.Run("should not duplicate owner references", func(t *testing.T) {
		obj := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "Cluster",
						Name:       "test-cluster",
					},
				},
			},
		}
		ref := metav1.OwnerReference{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
			Name:       "test-cluster",
		}
		obj.OwnerReferences = EnsureOwnerRef(obj.OwnerReferences, ref)
		g.Expect(obj.OwnerReferences).Should(ContainElement(ref))
		g.Expect(obj.OwnerReferences).Should(HaveLen(1))
	})

	t.Run("should update the APIVersion if duplicate", func(t *testing.T) {
		oldgvk := schema.GroupVersion{
			Group:   clusterv1.GroupVersion.Group,
			Version: "v1alpha2",
		}
		obj := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: oldgvk.String(),
						Kind:       "Cluster",
						Name:       "test-cluster",
					},
				},
			},
		}
		ref := metav1.OwnerReference{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
			Name:       "test-cluster",
		}
		obj.OwnerReferences = EnsureOwnerRef(obj.OwnerReferences, ref)
		g.Expect(obj.OwnerReferences).Should(ContainElement(ref))
		g.Expect(obj.OwnerReferences).Should(HaveLen(1))
	})
}
