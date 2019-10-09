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
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
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
		instance := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "ms-",
				Namespace:    namespace.Name,
			},
			Spec: clusterv1.MachineSetSpec{
				Replicas: &replicas,
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"label-1": "true",
					},
				},
				Template: clusterv1.MachineTemplateSpec{
					ObjectMeta: clusterv1.ObjectMeta{
						Labels: map[string]string{
							"label-1": "true",
						},
					},
					Spec: clusterv1.MachineSpec{
						Version: &version,
						Bootstrap: clusterv1.Bootstrap{
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

		machines := &clusterv1.MachineList{}

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
			if !m.DeletionTimestamp.IsZero() {
				// Skip deleted Machines
				continue
			}

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

func TestMachineSetOwnerReference(t *testing.T) {
	ml := &clusterv1.MachineList{}

	clusterIncorrectMeta := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "my-kind",
			"apiVersion": "my-api-version",
			"metadata": map[string]interface{}{
				"name":      "invalid-cluster",
				"namespace": "default",
			},
		},
	}

	clusterCorrectMeta := &clusterv1.Cluster{
		TypeMeta:   metav1.TypeMeta{Kind: "Cluster", APIVersion: clusterv1.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "valid-cluster"},
	}

	ms1 := newMachineSet("machineset1", "valid-cluster")
	ms2 := newMachineSet("machineset2", "invalid-cluster")
	ms3 := newMachineSet("machineset3", "valid-cluster")
	ms3.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "MachineDeployment",
			Name:       "valid-machinedeployment",
		},
	}

	testCases := []struct {
		name               string
		request            reconcile.Request
		ms                 *clusterv1.MachineSet
		expectReconcileErr bool
		expectedOR         []metav1.OwnerReference
	}{
		{
			name: "should add cluster owner reference to machine set",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ms1.Name,
					Namespace: ms1.Namespace,
				},
			},
			ms: ms1,
			expectedOR: []metav1.OwnerReference{
				{
					APIVersion: clusterCorrectMeta.APIVersion,
					Kind:       clusterCorrectMeta.Kind,
					Name:       clusterCorrectMeta.Name,
					UID:        clusterCorrectMeta.UID,
				},
			},
		},
		{
			name: "should add not add cluster owner reference with incorrect meta",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ms2.Name,
					Namespace: ms2.Namespace,
				},
			},
			ms:                 ms2,
			expectReconcileErr: true,
		},
		{
			name: "should not add cluster owner reference if machine is owned by a machine deployment",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ms3.Name,
					Namespace: ms3.Namespace,
				},
			},
			ms: ms3,
			expectedOR: []metav1.OwnerReference{
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "MachineDeployment",
					Name:       "valid-machinedeployment",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			clusterv1.AddToScheme(scheme.Scheme)
			msr := &MachineSetReconciler{
				Client: fake.NewFakeClientWithScheme(
					scheme.Scheme,
					ml,
					clusterCorrectMeta,
					clusterIncorrectMeta,
					ms1,
					ms2,
					ms3,
				),
				Log:      log.Log,
				recorder: record.NewFakeRecorder(32),
			}

			_, err := msr.Reconcile(tc.request)
			if tc.expectReconcileErr {
				Expect(err).ToNot(BeNil())
			} else {
				Expect(err).To(BeNil())
			}

			key := client.ObjectKey{Namespace: tc.ms.Namespace, Name: tc.ms.Name}
			var actual clusterv1.MachineSet
			if len(tc.expectedOR) > 0 {
				Expect(msr.Client.Get(ctx, key, &actual)).ToNot(HaveOccurred())
				Expect(actual.OwnerReferences).To(Equal(tc.expectedOR))
			} else {
				Expect(actual.OwnerReferences).To(BeEmpty())
			}
		})
	}
}

func TestMachineSetReconcile(t *testing.T) {
	t.Run("ignore machine sets marked for deletion", func(t *testing.T) {
		RegisterTestingT(t)
		dt := metav1.Now()
		ms := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "machineset1",
				Namespace:         "default",
				DeletionTimestamp: &dt,
			},
		}
		request := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      ms.Name,
				Namespace: ms.Namespace,
			},
		}
		clusterv1.AddToScheme(scheme.Scheme)
		msr := &MachineSetReconciler{
			Client:   fake.NewFakeClientWithScheme(scheme.Scheme, ms),
			Log:      log.Log,
			recorder: record.NewFakeRecorder(32),
		}
		result, err := msr.Reconcile(request)
		Expect(err).To(BeNil())
		Expect(result).To(Equal(reconcile.Result{}))
	})

	t.Run("records event if reconcile fails", func(t *testing.T) {
		RegisterTestingT(t)
		ms := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machineset1",
				Namespace: "default",
			},
		}
		request := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      ms.Name,
				Namespace: ms.Namespace,
			},
		}
		clusterv1.AddToScheme(scheme.Scheme)
		rec := record.NewFakeRecorder(32)
		msr := &MachineSetReconciler{
			Client:   fake.NewFakeClientWithScheme(scheme.Scheme, ms),
			Log:      log.Log,
			recorder: rec,
		}
		msr.Reconcile(request)
		Eventually(rec.Events).Should(Receive())
	})
}

func TestMachineSetToMachines(t *testing.T) {
	machineSetList := &clusterv1.MachineSetList{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSetList",
		},
		Items: []clusterv1.MachineSet{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "withMatchingLabels",
					Namespace: "test",
				},
				Spec: clusterv1.MachineSetSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"foo":                             "bar",
							clusterv1.MachineClusterLabelName: "test-cluster",
						},
					},
				},
			},
		},
	}
	controller := true
	m := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withOwnerRef",
			Namespace: "test",
			Labels: map[string]string{
				clusterv1.MachineClusterLabelName: "test-cluster",
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
	m2 := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "noOwnerRefNoLabels",
			Namespace: "test",
			Labels: map[string]string{
				clusterv1.MachineClusterLabelName: "test-cluster",
			},
		},
	}
	m3 := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withMatchingLabels",
			Namespace: "test",
			Labels: map[string]string{
				"foo":                             "bar",
				clusterv1.MachineClusterLabelName: "test-cluster",
			},
		},
	}
	testsCases := []struct {
		name      string
		mapObject handler.MapObject
		expected  []reconcile.Request
	}{
		{
			name: "should return empty request when controller is set",
			mapObject: handler.MapObject{
				Meta:   m.GetObjectMeta(),
				Object: &m,
			},
			expected: []reconcile.Request{},
		},
		{
			name: "should return nil if machine has no owner reference",
			mapObject: handler.MapObject{
				Meta:   m2.GetObjectMeta(),
				Object: &m2,
			},
			expected: nil,
		},
		{
			name: "should return request if machine set's labels matches machine's labels",
			mapObject: handler.MapObject{
				Meta:   m3.GetObjectMeta(),
				Object: &m3,
			},
			expected: []reconcile.Request{
				{NamespacedName: client.ObjectKey{Namespace: "test", Name: "withMatchingLabels"}},
			},
		},
	}

	clusterv1.AddToScheme(scheme.Scheme)
	r := &MachineSetReconciler{
		Client: fake.NewFakeClient(&m, &m2, &m3, machineSetList),
		Log:    log.Log,
	}

	for _, tc := range testsCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			got := r.MachineToMachineSets(tc.mapObject)
			Expect(got).To(Equal(tc.expected))
		})
	}
}

func TestShouldExcludeMachine(t *testing.T) {
	RegisterTestingT(t)
	controller := true
	testCases := []struct {
		machineSet clusterv1.MachineSet
		machine    clusterv1.Machine
		expected   bool
	}{
		{
			machineSet: clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{UID: "1"},
			},
			machine: clusterv1.Machine{
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
			machineSet: clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{UID: "1"},
			},
			machine: clusterv1.Machine{
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
			machineSet: clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
			machine: clusterv1.Machine{
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
			machineSet: clusterv1.MachineSet{},
			machine: clusterv1.Machine{
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
		Expect(got).To(Equal(tc.expected))
	}
}

func TestAdoptOrphan(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()
	m := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "orphanMachine",
		},
	}
	ms := clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "adoptOrphanMachine",
		},
	}
	controller := true
	blockOwnerDeletion := true
	testCases := []struct {
		machineSet clusterv1.MachineSet
		machine    clusterv1.Machine
		expected   []metav1.OwnerReference
	}{
		{
			machine:    m,
			machineSet: ms,
			expected: []metav1.OwnerReference{
				{
					APIVersion:         clusterv1.GroupVersion.String(),
					Kind:               "MachineSet",
					Name:               "adoptOrphanMachine",
					UID:                "",
					Controller:         &controller,
					BlockOwnerDeletion: &blockOwnerDeletion,
				},
			},
		},
	}

	clusterv1.AddToScheme(scheme.Scheme)
	r := &MachineSetReconciler{
		Client: fake.NewFakeClient(&m),
		Log:    log.Log,
	}
	for _, tc := range testCases {
		err := r.adoptOrphan(ctx, tc.machineSet.DeepCopy(), tc.machine.DeepCopy())
		Expect(err).ToNot(HaveOccurred())

		key := client.ObjectKey{Namespace: tc.machine.Namespace, Name: tc.machine.Name}
		err = r.Client.Get(ctx, key, &tc.machine)
		Expect(err).ToNot(HaveOccurred())

		got := tc.machine.GetOwnerReferences()
		Expect(got).To(Equal(tc.expected))
	}
}

func TestHasMatchingLabels(t *testing.T) {
	r := &MachineSetReconciler{}

	testCases := []struct {
		name       string
		machineSet clusterv1.MachineSet
		machine    clusterv1.Machine
		expected   bool
	}{
		{
			name: "machine set and machine have matching labels",
			machineSet: clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
			machine: clusterv1.Machine{
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
			name: "machine set and machine do not have matching labels",
			machineSet: clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
			machine: clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "doesNotMatchSelector",
					Labels: map[string]string{
						"no": "match",
					},
				},
			},
			expected: false,
		},
		{
			name: "machine set has empty selector",
			machineSet: clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Selector: metav1.LabelSelector{},
				},
			},
			machine: clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "doesNotMatter",
				},
			},
			expected: false,
		},
		{
			name: "machine set has bad selector",
			machineSet: clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"foo": "bar",
						},
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Operator: "bad-operator",
							},
						},
					},
				},
			},
			machine: clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "match",
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			got := r.hasMatchingLabels(&tc.machineSet, &tc.machine)
			Expect(got).To(Equal(tc.expected))
		})
	}
}

func newMachineSet(name, cluster string) *clusterv1.MachineSet {
	var replicas int32
	return &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.MachineClusterLabelName: cluster,
			},
		},
		Spec: clusterv1.MachineSetSpec{
			Replicas: &replicas,
			Template: clusterv1.MachineTemplateSpec{
				ObjectMeta: clusterv1.ObjectMeta{
					Labels: map[string]string{
						clusterv1.MachineClusterLabelName: cluster,
					},
				},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					clusterv1.MachineClusterLabelName: cluster,
				},
			},
		},
	}
}
