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

package controllers

import (
	"reflect"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ reconcile.Reconciler = &MachineDeploymentReconciler{}

var _ = Describe("MachineDeployment Reconciler", func() {
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "md-test"}}

	BeforeEach(func() {
		Expect(k8sClient.Create(ctx, namespace)).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, namespace)).NotTo(HaveOccurred())
	})

	It("Should reconcile a MachineDeployment", func() {
		labels := map[string]string{"foo": "bar"}
		version := "1.10.3"
		deployment := &clusterv1.MachineDeployment{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "md-",
				Namespace:    namespace.Name,
			},
			Spec: clusterv1.MachineDeploymentSpec{
				MinReadySeconds:      pointer.Int32Ptr(0),
				Replicas:             pointer.Int32Ptr(2),
				RevisionHistoryLimit: pointer.Int32Ptr(0),
				Selector:             metav1.LabelSelector{MatchLabels: labels},
				Strategy: &clusterv1.MachineDeploymentStrategy{
					Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
					RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
						MaxUnavailable: intOrStrPtr(0),
						MaxSurge:       intOrStrPtr(1),
					},
				},
				Template: clusterv1.MachineTemplateSpec{
					ObjectMeta: clusterv1.ObjectMeta{
						Labels: labels,
					},
					Spec: clusterv1.MachineSpec{
						Version: &version,
						InfrastructureRef: corev1.ObjectReference{
							APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha2",
							Kind:       "InfrastructureMachineTemplate",
							Name:       "md-template",
						},
						Bootstrap: clusterv1.Bootstrap{
							Data: pointer.StringPtr("data"),
						},
					},
				},
			},
		}
		msListOpts := []client.ListOption{
			client.InNamespace(namespace.Name),
			client.MatchingLabels(labels),
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
		infraTmpl.SetName("md-template")
		infraTmpl.SetNamespace(namespace.Name)
		Expect(k8sClient.Create(ctx, infraTmpl)).To(BeNil())

		// Create the MachineDeployment object and expect Reconcile to be called.
		Expect(k8sClient.Create(ctx, deployment)).To(BeNil())
		defer k8sClient.Delete(ctx, deployment)

		// Verify that the MachineSet was created.
		machineSets := &clusterv1.MachineSetList{}
		Eventually(func() int {
			if err := k8sClient.List(ctx, machineSets, msListOpts...); err != nil {
				return -1
			}
			return len(machineSets.Items)
		}, timeout).Should(BeEquivalentTo(1))

		firstMachineSet := machineSets.Items[0]
		Expect(*firstMachineSet.Spec.Replicas).To(BeEquivalentTo(2))
		Expect(*firstMachineSet.Spec.Template.Spec.Version).To(BeEquivalentTo("1.10.3"))

		//
		// Delete firstMachineSet and expect Reconcile to be called to replace it.
		//
		Expect(k8sClient.Delete(ctx, &firstMachineSet)).To(BeNil())
		Eventually(func() bool {
			if err := k8sClient.List(ctx, machineSets, msListOpts...); err != nil {
				return false
			}
			for _, ms := range machineSets.Items {
				if ms.UID == firstMachineSet.UID {
					return false
				}
			}
			return len(machineSets.Items) > 0
		}, timeout).Should(BeTrue())

		//
		// Scale the MachineDeployment and expect Reconcile to be called.
		//
		secondMachineSet := machineSets.Items[0]
		err := updateMachineDeployment(k8sClient, deployment, func(d *clusterv1.MachineDeployment) { d.Spec.Replicas = pointer.Int32Ptr(3) })
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() int {
			key := client.ObjectKey{Name: secondMachineSet.Name, Namespace: secondMachineSet.Namespace}
			if err := k8sClient.Get(ctx, key, &secondMachineSet); err != nil {
				return -1
			}
			return int(*secondMachineSet.Spec.Replicas)
		}, timeout).Should(BeEquivalentTo(3))

		//
		// Update a MachineDeployment, expect Reconcile to be called and a new MachineSet to appear.
		//
		err = updateMachineDeployment(k8sClient, deployment, func(d *clusterv1.MachineDeployment) { d.Spec.Template.Labels["updated"] = "true" })
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() int {
			if err := k8sClient.List(ctx, machineSets, msListOpts...); err != nil {
				return -1
			}
			return len(machineSets.Items)
		}, timeout).Should(BeEquivalentTo(2))

		Eventually(func() int {
			// Set the all non-deleted machines as ready with a NodeRef, so the MachineSet controller can proceed
			// to properly set AvailableReplicas.
			machines := &clusterv1.MachineList{}
			Expect(k8sClient.List(ctx, machines, client.InNamespace(namespace.Name))).NotTo(HaveOccurred())
			for _, m := range machines.Items {
				if !m.DeletionTimestamp.IsZero() {
					continue
				}
				fakeInfrastructureRefReady(m.Spec.InfrastructureRef, infraResource)
				fakeMachineNodeRef(&m)
			}

			if err := k8sClient.List(ctx, machineSets, msListOpts...); err != nil {
				return -1
			}
			return len(machineSets.Items)
		}, timeout*3).Should(BeEquivalentTo(1))

	})
})

func TestMachineSetToDeployments(t *testing.T) {
	machineDeployment := clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withMatchingLabels",
			Namespace: "test",
		},
		Spec: clusterv1.MachineDeploymentSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo":                             "bar",
					clusterv1.MachineClusterLabelName: "test-cluster",
				},
			},
		},
	}

	machineDeplopymentList := &clusterv1.MachineDeploymentList{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineDeploymentList",
		},
		Items: []clusterv1.MachineDeployment{machineDeployment},
	}

	ms1 := clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withOwnerRef",
			Namespace: "test",
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(&machineDeployment, controllerKind),
			},
			Labels: map[string]string{
				clusterv1.MachineClusterLabelName: "test-cluster",
			},
		},
	}
	ms2 := clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "noOwnerRefNoLabels",
			Namespace: "test",
			Labels: map[string]string{
				clusterv1.MachineClusterLabelName: "test-cluster",
			},
		},
	}
	ms3 := clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
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
		machineSet clusterv1.MachineSet
		mapObject  handler.MapObject
		expected   []reconcile.Request
	}{
		{
			machineSet: ms1,
			mapObject: handler.MapObject{
				Meta:   ms1.GetObjectMeta(),
				Object: &ms1,
			},
			expected: []reconcile.Request{},
		},
		{
			machineSet: ms2,
			mapObject: handler.MapObject{
				Meta:   ms2.GetObjectMeta(),
				Object: &ms2,
			},
			expected: nil,
		},
		{
			machineSet: ms3,
			mapObject: handler.MapObject{
				Meta:   ms3.GetObjectMeta(),
				Object: &ms3,
			},
			expected: []reconcile.Request{
				{NamespacedName: client.ObjectKey{Namespace: "test", Name: "withMatchingLabels"}},
			},
		},
	}

	clusterv1.AddToScheme(scheme.Scheme)
	r := &MachineDeploymentReconciler{
		Client:   fake.NewFakeClient(machineDeplopymentList),
		Log:      log.Log,
		recorder: record.NewFakeRecorder(32),
	}

	for _, tc := range testsCases {
		got := r.MachineSetToDeployments(tc.mapObject)
		if !reflect.DeepEqual(got, tc.expected) {
			t.Errorf("Case %s. Got: %v, expected: %v", tc.machineSet.Name, got, tc.expected)
		}
	}
}

func TestGetMachineDeploymentsForMachineSet(t *testing.T) {
	machineDeployment := clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withLabels",
			Namespace: "test",
		},
		Spec: clusterv1.MachineDeploymentSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			},
		},
	}
	machineDeplopymentList := &clusterv1.MachineDeploymentList{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineDeploymentList",
		},
		Items: []clusterv1.MachineDeployment{
			machineDeployment,
		},
	}
	ms1 := clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "NoMatchingLabels",
			Namespace: "test",
		},
	}
	ms2 := clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withMatchingLabels",
			Namespace: "test",
			Labels: map[string]string{
				"foo": "bar",
			},
		},
	}

	testCases := []struct {
		machineDeploymentList clusterv1.MachineDeploymentList
		machineSet            clusterv1.MachineSet
		expected              []*clusterv1.MachineDeployment
	}{
		{
			machineDeploymentList: *machineDeplopymentList,
			machineSet:            ms1,
			expected:              nil,
		},
		{
			machineDeploymentList: *machineDeplopymentList,
			machineSet:            ms2,
			expected:              []*clusterv1.MachineDeployment{&machineDeployment},
		},
	}
	clusterv1.AddToScheme(scheme.Scheme)
	r := &MachineDeploymentReconciler{
		Client:   fake.NewFakeClient(&ms1, &ms2, machineDeplopymentList),
		Log:      log.Log,
		recorder: record.NewFakeRecorder(32),
	}

	for _, tc := range testCases {
		got := r.getMachineDeploymentsForMachineSet(&tc.machineSet)
		if !reflect.DeepEqual(got, tc.expected) {
			t.Errorf("Case %s. Got: %v, expected %v", tc.machineSet.Name, got, tc.expected)
		}
	}
}

func TestGetMachineSetsForDeployment(t *testing.T) {
	machineDeployment1 := clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withMatchingOwnerRefAndLabels",
			Namespace: "test",
			UID:       "UID",
		},
		Spec: clusterv1.MachineDeploymentSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			},
		},
	}
	machineDeployment2 := clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withNoMatchingOwnerRef",
			Namespace: "test",
			UID:       "unMatchingUID",
		},
		Spec: clusterv1.MachineDeploymentSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar2",
				},
			},
		},
	}

	ms1 := clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withNoOwnerRefShouldBeAdopted2",
			Namespace: "test",
			Labels: map[string]string{
				"foo": "bar2",
			},
		},
	}
	ms2 := clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withOwnerRefAndLabels",
			Namespace: "test",
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(&machineDeployment1, controllerKind),
			},
			Labels: map[string]string{
				"foo": "bar",
			},
		},
	}
	ms3 := clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withNoOwnerRefShouldBeAdopted1",
			Namespace: "test",
			Labels: map[string]string{
				"foo": "bar",
			},
		},
	}
	ms4 := clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withNoOwnerRefNoMatch",
			Namespace: "test",
			Labels: map[string]string{
				"foo": "nomatch",
			},
		},
	}
	machineSetList := &clusterv1.MachineSetList{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSetList",
		},
		Items: []clusterv1.MachineSet{
			ms1,
			ms2,
			ms3,
			ms4,
		},
	}

	testCases := []struct {
		machineDeployment clusterv1.MachineDeployment
		expected          []*clusterv1.MachineSet
	}{
		{
			machineDeployment: machineDeployment1,
			expected:          []*clusterv1.MachineSet{&ms2, &ms3},
		},
		{
			machineDeployment: machineDeployment2,
			expected:          []*clusterv1.MachineSet{&ms1},
		},
	}

	clusterv1.AddToScheme(scheme.Scheme)
	r := &MachineDeploymentReconciler{
		Client:   fake.NewFakeClient(machineSetList),
		Log:      log.Log,
		recorder: record.NewFakeRecorder(32),
	}
	for _, tc := range testCases {
		got, err := r.getMachineSetsForDeployment(&tc.machineDeployment)
		if err != nil {
			t.Errorf("Failed running getMachineSetsForDeployment: %v", err)
		}

		if len(tc.expected) != len(got) {
			t.Errorf("Case %s. Expected to get %d MachineSets but got %d", tc.machineDeployment.Name, len(tc.expected), len(got))
		}

		for idx, res := range got {
			if res.Name != tc.expected[idx].Name || res.Namespace != tc.expected[idx].Namespace {
				t.Errorf("Case %s. Expected %q found %q", tc.machineDeployment.Name, res.Name, tc.expected[idx].Name)
			}
		}
	}
}
