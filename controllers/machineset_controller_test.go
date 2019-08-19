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

package controllers

import (
	"reflect"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ reconcile.Reconciler = &MachineSetReconciler{}

var _ = Describe("MachineSet Reconciler", func() {
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ms-test"}}

	BeforeEach(func() {
		Expect(k8sClient.Create(ctx, namespace)).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, namespace)).NotTo(HaveOccurred())
	})

	It("Should reconcile a MachineSet", func() {
		replicas := int32(2)
		version := "1.14.2"
		instance := &v1alpha2.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "ms-",
				Namespace:    namespace.Name,
			},
			Spec: v1alpha2.MachineSetSpec{
				Replicas: &replicas,
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"label-1": "true",
					},
				},
				Template: v1alpha2.MachineTemplateSpec{
					ObjectMeta: v1alpha2.ObjectMeta{
						Labels: map[string]string{
							"label-1": "true",
						},
					},
					Spec: v1alpha2.MachineSpec{
						Version: &version,
						Bootstrap: v1alpha2.Bootstrap{
							Data: pointer.StringPtr("x"),
						},
						InfrastructureRef: corev1.ObjectReference{
							APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha2",
							Kind:       "InfrastructureMachineTemplate",
							Name:       "ms-template",
						},
					},
				},
			},
		}

		// Create infrastructure template resource.
		infraResource := map[string]interface{}{
			"kind":       "InfrastructureMachine",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha2",
			"metadata":   map[string]interface{}{},
			"spec": map[string]interface{}{
				"size":       "3xlarge",
				"providerID": "test:////id",
			},
		}
		infraTmpl := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": infraResource,
				},
			},
		}
		infraTmpl.SetKind("InfrastructureMachineTemplate")
		infraTmpl.SetAPIVersion("infrastructure.cluster.x-k8s.io/v1alpha2")
		infraTmpl.SetName("ms-template")
		infraTmpl.SetNamespace(namespace.Name)
		Expect(k8sClient.Create(ctx, infraTmpl)).To(BeNil())

		// Create the MachineSet.
		Expect(k8sClient.Create(ctx, instance)).To(BeNil())
		defer k8sClient.Delete(ctx, instance)

		machines := &v1alpha2.MachineList{}

		// Verify that we have 2 replicas.
		Eventually(func() int {
			if err := k8sClient.List(ctx, machines, client.InNamespace(namespace.Name)); err != nil {
				return -1
			}
			return len(machines.Items)
		}, timeout).Should(BeEquivalentTo(replicas))

		// Set the infrastructure reference as ready.
		for _, m := range machines.Items {
			fakeInfrastructureRefReady(m.Spec.InfrastructureRef, infraResource)
		}

		// Try to delete 1 machine and check the MachineSet scales back up.
		machineToBeDeleted := machines.Items[0]
		Expect(k8sClient.Delete(ctx, &machineToBeDeleted)).To(BeNil())

		// Verify that the Machine has been deleted.
		Eventually(func() bool {
			key := client.ObjectKey{Name: machineToBeDeleted.Name, Namespace: machineToBeDeleted.Namespace}
			if err := k8sClient.Get(ctx, key, &machineToBeDeleted); apierrors.IsNotFound(err) || !machineToBeDeleted.DeletionTimestamp.IsZero() {
				return true
			}
			return false
		}, timeout).Should(BeTrue())

		// Verify that we have 2 replicas.
		Eventually(func() (ready int) {
			if err := k8sClient.List(ctx, machines, client.InNamespace(namespace.Name)); err != nil {
				return -1
			}
			for _, m := range machines.Items {
				if !m.DeletionTimestamp.IsZero() {
					continue
				}
				ready++
			}
			return
		}, timeout*3).Should(BeEquivalentTo(replicas))

		// Verify that each machine has the desired kubelet version,
		// create a fake node in Ready state, update NodeRef, and wait for a reconciliation request.
		for _, m := range machines.Items {
			Expect(m.Spec.Version).ToNot(BeNil())
			Expect(*m.Spec.Version).To(BeEquivalentTo("1.14.2"))
			fakeInfrastructureRefReady(m.Spec.InfrastructureRef, infraResource)
			fakeMachineNodeRef(&m)
		}

		// Verify that all Machines are Ready.
		Eventually(func() int32 {
			key := client.ObjectKey{Name: instance.Name, Namespace: instance.Namespace}
			if err := k8sClient.Get(ctx, key, instance); err != nil {
				return -1
			}
			return instance.Status.AvailableReplicas
		}, timeout).Should(BeEquivalentTo(replicas))
	})
})

func TestMachineSetToMachines(t *testing.T) {
	machineSetList := &v1alpha2.MachineSetList{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSetList",
		},
		Items: []v1alpha2.MachineSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "withMatchingLabels",
					Namespace: "test",
				},
				Spec: v1alpha2.MachineSetSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"foo":                            "bar",
							v1alpha2.MachineClusterLabelName: "test-cluster",
						},
					},
				},
			},
		},
	}
	controller := true
	m := v1alpha2.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withOwnerRef",
			Namespace: "test",
			Labels: map[string]string{
				v1alpha2.MachineClusterLabelName: "test-cluster",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Name:       "Owner",
					Kind:       "MachineSet",
					Controller: &controller,
				},
			},
		},
	}
	m2 := v1alpha2.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "noOwnerRefNoLabels",
			Namespace: "test",
			Labels: map[string]string{
				v1alpha2.MachineClusterLabelName: "test-cluster",
			},
		},
	}
	m3 := v1alpha2.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withMatchingLabels",
			Namespace: "test",
			Labels: map[string]string{
				"foo":                            "bar",
				v1alpha2.MachineClusterLabelName: "test-cluster",
			},
		},
	}
	testsCases := []struct {
		machine   v1alpha2.Machine
		mapObject handler.MapObject
		expected  []reconcile.Request
	}{
		{
			machine: m,
			mapObject: handler.MapObject{
				Meta:   m.GetObjectMeta(),
				Object: &m,
			},
			expected: []reconcile.Request{},
		},
		{
			machine: m2,
			mapObject: handler.MapObject{
				Meta:   m2.GetObjectMeta(),
				Object: &m2,
			},
			expected: nil,
		},
		{
			machine: m3,
			mapObject: handler.MapObject{
				Meta:   m3.GetObjectMeta(),
				Object: &m3,
			},
			expected: []reconcile.Request{
				{NamespacedName: client.ObjectKey{Namespace: "test", Name: "withMatchingLabels"}},
			},
		},
	}

	v1alpha2.AddToScheme(scheme.Scheme)
	r := &MachineSetReconciler{
		Client: fake.NewFakeClient(&m, &m2, &m3, machineSetList),
		Log:    log.Log,
	}

	for _, tc := range testsCases {
		got := r.MachineToMachineSets(tc.mapObject)
		if !reflect.DeepEqual(got, tc.expected) {
			t.Errorf("Case %s. Got: %v, expected: %v", tc.machine.Name, got, tc.expected)
		}
	}
}

func TestShouldExcludeMachine(t *testing.T) {
	controller := true
	testCases := []struct {
		machineSet v1alpha2.MachineSet
		machine    v1alpha2.Machine
		expected   bool
	}{
		{
			machineSet: v1alpha2.MachineSet{
				ObjectMeta: metav1.ObjectMeta{UID: "1"},
			},
			machine: v1alpha2.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "withNoMatchingOwnerRef",
					Namespace: "test",
					OwnerReferences: []metav1.OwnerReference{
						{
							Name:       "Owner",
							Kind:       "MachineSet",
							Controller: &controller,
							UID:        "not-1",
						},
					},
				},
			},
			expected: true,
		},
		{
			machineSet: v1alpha2.MachineSet{
				ObjectMeta: metav1.ObjectMeta{UID: "1"},
			},
			machine: v1alpha2.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "withMatchingOwnerRef",
					Namespace: "test",
					OwnerReferences: []metav1.OwnerReference{
						{
							Name:       "Owner",
							Kind:       "MachineSet",
							Controller: &controller,
							UID:        "1",
						},
					},
				},
			},
			expected: false,
		},
		{
			machineSet: v1alpha2.MachineSet{
				Spec: v1alpha2.MachineSetSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
			machine: v1alpha2.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "withMatchingLabels",
					Namespace: "test",
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			},
			expected: false,
		},
		{
			machineSet: v1alpha2.MachineSet{},
			machine: v1alpha2.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "withDeletionTimestamp",
					Namespace:         "test",
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		got := shouldExcludeMachine(&tc.machineSet, &tc.machine)
		if got != tc.expected {
			t.Errorf("Case %s. Got: %v, expected: %v", tc.machine.Name, got, tc.expected)
		}
	}
}

func TestAdoptOrphan(t *testing.T) {
	m := v1alpha2.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "orphanMachine",
		},
	}
	ms := v1alpha2.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "adoptOrphanMachine",
		},
	}
	controller := true
	blockOwnerDeletion := true
	testCases := []struct {
		machineSet v1alpha2.MachineSet
		machine    v1alpha2.Machine
		expected   []metav1.OwnerReference
	}{
		{
			machine:    m,
			machineSet: ms,
			expected: []metav1.OwnerReference{
				{
					APIVersion:         v1alpha2.GroupVersion.String(),
					Kind:               "MachineSet",
					Name:               "adoptOrphanMachine",
					UID:                "",
					Controller:         &controller,
					BlockOwnerDeletion: &blockOwnerDeletion,
				},
			},
		},
	}

	v1alpha2.AddToScheme(scheme.Scheme)
	r := &MachineSetReconciler{
		Client: fake.NewFakeClient(&m),
		Log:    log.Log,
	}
	for _, tc := range testCases {
		r.adoptOrphan(&tc.machineSet, &tc.machine)
		got := tc.machine.GetOwnerReferences()
		if !reflect.DeepEqual(got, tc.expected) {
			t.Errorf("Case %s. Got: %+v, expected: %+v", tc.machine.Name, got, tc.expected)
		}
	}
}

func TestHasMatchingLabels(t *testing.T) {
	r := &MachineSetReconciler{}

	testCases := []struct {
		machineSet v1alpha2.MachineSet
		machine    v1alpha2.Machine
		expected   bool
	}{
		{
			machineSet: v1alpha2.MachineSet{
				Spec: v1alpha2.MachineSetSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
			machine: v1alpha2.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "matchSelector",
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			},
			expected: true,
		},
		{
			machineSet: v1alpha2.MachineSet{
				Spec: v1alpha2.MachineSetSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
			machine: v1alpha2.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "doesNotMatchSelector",
					Labels: map[string]string{
						"no": "match",
					},
				},
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		got := r.hasMatchingLabels(&tc.machineSet, &tc.machine)
		if tc.expected != got {
			t.Errorf("Case %s. Got: %v, expected %v", tc.machine.Name, got, tc.expected)
		}
	}
}
