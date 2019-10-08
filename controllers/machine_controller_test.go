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
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMachineFinalizer(t *testing.T) {
	RegisterTestingT(t)
	clusterCorrectMeta := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "valid-cluster",
		},
	}

	machineValidCluster := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine1",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.MachineClusterLabelName: "valid-cluster",
			},
		},
	}

	machineWithFinalizer := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine2",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.MachineClusterLabelName: "valid-cluster",
			},
			Finalizers: []string{"some-other-finalizer"},
		},
	}

	testCases := []struct {
		name               string
		request            reconcile.Request
		m                  *clusterv1.Machine
		expectedFinalizers []string
	}{
		{
			name: "should add a machine finalizer to the machine if it doesn't have one",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      machineValidCluster.Name,
					Namespace: machineValidCluster.Namespace,
				},
			},
			m:                  machineValidCluster,
			expectedFinalizers: []string{clusterv1.MachineFinalizer},
		},
		{
			name: "should append the machine finalizer to the machine if it already has a finalizer",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      machineWithFinalizer.Name,
					Namespace: machineWithFinalizer.Namespace,
				},
			},
			m:                  machineWithFinalizer,
			expectedFinalizers: []string{"some-other-finalizer", clusterv1.MachineFinalizer},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clusterv1.AddToScheme(scheme.Scheme)
			mr := &MachineReconciler{
				Client: fake.NewFakeClientWithScheme(
					scheme.Scheme,
					clusterCorrectMeta,
					machineValidCluster,
					machineWithFinalizer,
				),
				Log: log.Log,
			}

			mr.Reconcile(tc.request)

			key := client.ObjectKey{Namespace: tc.m.Namespace, Name: tc.m.Name}
			var actual clusterv1.Machine
			if len(tc.expectedFinalizers) > 0 {
				Expect(mr.Client.Get(ctx, key, &actual)).ToNot(HaveOccurred())
				Expect(actual.Finalizers).ToNot(BeEmpty())
				Expect(actual.Finalizers).To(Equal(tc.expectedFinalizers))
			} else {
				Expect(actual.Finalizers).To(BeEmpty())
			}
		})
	}
}

func TestMachineOwnerReference(t *testing.T) {
	RegisterTestingT(t)
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

	machineInvalidCluster := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine1",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.MachineClusterLabelName: "invalid-cluster",
			},
		},
	}

	machineValidCluster := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine2",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.MachineClusterLabelName: "valid-cluster",
			},
		},
		Spec: clusterv1.MachineSpec{},
	}

	machineValidMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine3",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.MachineClusterLabelName: "valid-cluster",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "MachineSet",
					Name:       "valid-machineset",
				},
			},
		},
		Spec: clusterv1.MachineSpec{},
	}

	testCases := []struct {
		name       string
		request    reconcile.Request
		m          *clusterv1.Machine
		expectedOR []metav1.OwnerReference
	}{
		{
			name: "should not add owner reference to machine referencing a cluster with incorrect type meta",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      machineInvalidCluster.Name,
					Namespace: machineInvalidCluster.Namespace,
				},
			},
			m: machineInvalidCluster,
		},
		{
			name: "should add owner reference to machine referencing a cluster with correct type meta",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      machineValidCluster.Name,
					Namespace: machineValidCluster.Namespace,
				},
			},
			m: machineValidCluster,
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
			name: "should not add cluster owner reference if machine is owned by a machine set",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      machineValidMachine.Name,
					Namespace: machineValidMachine.Namespace,
				},
			},
			m: machineValidMachine,
			expectedOR: []metav1.OwnerReference{
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "MachineSet",
					Name:       "valid-machineset",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clusterv1.AddToScheme(scheme.Scheme)
			mr := &MachineReconciler{
				Client: fake.NewFakeClientWithScheme(
					scheme.Scheme,
					clusterIncorrectMeta,
					machineInvalidCluster,
					clusterCorrectMeta,
					machineValidCluster,
					machineValidMachine,
				),
				Log: log.Log,
			}

			mr.Reconcile(tc.request)

			key := client.ObjectKey{Namespace: tc.m.Namespace, Name: tc.m.Name}
			var actual clusterv1.Machine
			if len(tc.expectedOR) > 0 {
				Expect(mr.Client.Get(ctx, key, &actual)).ToNot(HaveOccurred())
				Expect(actual.OwnerReferences).To(Equal(tc.expectedOR))
			} else {
				Expect(actual.OwnerReferences).To(BeEmpty())
			}
		})
	}
}

func TestReconcileRequest(t *testing.T) {
	RegisterTestingT(t)

	infraConfig := unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "InfrastructureConfig",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha2",
			"metadata": map[string]interface{}{
				"name":      "infra-config1",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"providerID": "test://id-1",
			},
			"status": map[string]interface{}{
				"ready": true,
				"addresses": []interface{}{
					map[string]interface{}{
						"type":    "InternalIP",
						"address": "10.0.0.1",
					},
				},
			},
		},
	}

	machine1 := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "create",
			Namespace:  "default",
			Finalizers: []string{clusterv1.MachineFinalizer, metav1.FinalizerDeleteDependents},
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha2",
				Kind:       "InfrastructureConfig",
				Name:       "infra-config1",
			},
			Bootstrap: clusterv1.Bootstrap{Data: pointer.StringPtr("data")},
		},
	}

	machine2 := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "update",
			Namespace:  "default",
			Finalizers: []string{clusterv1.MachineFinalizer, metav1.FinalizerDeleteDependents},
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha2",
				Kind:       "InfrastructureConfig",
				Name:       "infra-config1",
			},
			Bootstrap: clusterv1.Bootstrap{Data: pointer.StringPtr("data")},
		},
	}

	time := metav1.Now()
	machine3 := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "delete",
			Namespace:         "default",
			Finalizers:        []string{clusterv1.MachineFinalizer, metav1.FinalizerDeleteDependents},
			DeletionTimestamp: &time,
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha2",
				Kind:       "InfrastructureConfig",
				Name:       "infra-config1",
			},
			Bootstrap: clusterv1.Bootstrap{Data: pointer.StringPtr("data")},
		},
	}

	clusterList := clusterv1.ClusterList{
		Items: []clusterv1.Cluster{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testcluster",
					Namespace: "default",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rainbow",
					Namespace: "foo",
				},
			},
		},
	}

	type expected struct {
		result reconcile.Result
		err    bool
	}
	testCases := []struct {
		request  reconcile.Request
		expected expected
	}{
		{
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: machine1.Name, Namespace: machine1.Namespace}},
			expected: expected{
				result: reconcile.Result{},
				err:    false,
			},
		},
		{
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: machine2.Name, Namespace: machine2.Namespace}},
			expected: expected{
				result: reconcile.Result{},
				err:    false,
			},
		},
		{
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: machine3.Name, Namespace: machine3.Namespace}},
			expected: expected{
				result: reconcile.Result{},
				err:    false,
			},
		},
	}

	for _, tc := range testCases {
		clusterv1.AddToScheme(scheme.Scheme)
		r := &MachineReconciler{
			Client: fake.NewFakeClient(
				&clusterList,
				&machine1,
				&machine2,
				&machine3,
				&infraConfig,
			),
			Log: log.Log,
		}

		result, err := r.Reconcile(tc.request)
		if tc.expected.err {
			Expect(err).ToNot(BeNil())
		} else {
			Expect(err).To(BeNil())
		}

		Expect(result).To(Equal(tc.expected.result))
	}
}

func TestReconcileDeleteExternal(t *testing.T) {
	RegisterTestingT(t)

	bootstrapConfig := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "BootstrapConfig",
			"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha2",
			"metadata": map[string]interface{}{
				"name":      "delete-bootstrap",
				"namespace": "default",
			},
		},
	}

	infraConfig := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "InfrastructureConfig",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha2",
			"metadata": map[string]interface{}{
				"name":      "delete-infra",
				"namespace": "default",
			},
		},
	}

	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delete",
			Namespace: "default",
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha2",
				Kind:       "InfrastructureConfig",
				Name:       "delete-infra",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha2",
					Kind:       "BootstrapConfig",
					Name:       "delete-bootstrap",
				},
			},
		},
	}

	testCases := []struct {
		name            string
		bootstrapExists bool
		infraExists     bool
		expected        bool
		expectError     bool
	}{
		{
			name:            "should continue to reconcile delete of external refs since both refs exists",
			bootstrapExists: true,
			infraExists:     true,
			expected:        false,
			expectError:     false,
		},
		{
			name:            "should continue to reconcile delete of external refs since infra ref exist",
			bootstrapExists: false,
			infraExists:     true,
			expected:        false,
			expectError:     false,
		},
		{
			name:            "should continue to reconcile delete of external refs since bootstrap ref exist",
			bootstrapExists: true,
			infraExists:     false,
			expected:        false,
			expectError:     false,
		},
		{
			name:            "should no longer reconcile deletion of external refs since both don't exist",
			bootstrapExists: false,
			infraExists:     false,
			expected:        true,
			expectError:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clusterv1.AddToScheme(scheme.Scheme)

			objs := []runtime.Object{machine}

			if tc.bootstrapExists {
				objs = append(objs, bootstrapConfig)
			}

			if tc.infraExists {
				objs = append(objs, infraConfig)
			}

			r := &MachineReconciler{
				Client: fake.NewFakeClientWithScheme(scheme.Scheme, objs...),
				Log:    log.Log,
			}

			ok, err := r.reconcileDeleteExternal(ctx, machine)
			Expect(ok).To(Equal(tc.expected))
			if tc.expectError {
				Expect(err).ToNot(BeNil())
			} else {
				Expect(err).To(BeNil())
			}
		})
	}
}

func TestRemoveMachineFinalizerAfterDeleteReconcile(t *testing.T) {
	RegisterTestingT(t)
	clusterv1.AddToScheme(scheme.Scheme)
	dt := metav1.Now()
	m := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "delete123",
			Namespace:         "default",
			Finalizers:        []string{clusterv1.MachineFinalizer, metav1.FinalizerDeleteDependents},
			DeletionTimestamp: &dt,
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha2",
				Kind:       "InfrastructureConfig",
				Name:       "infra-config1",
			},
			Bootstrap: clusterv1.Bootstrap{Data: pointer.StringPtr("data")},
		},
	}
	key := client.ObjectKey{Namespace: m.Namespace, Name: m.Name}
	mr := &MachineReconciler{
		Client: fake.NewFakeClientWithScheme(scheme.Scheme, m),
		Log:    log.Log,
	}
	_, err := mr.Reconcile(reconcile.Request{NamespacedName: key})
	Expect(err).ToNot(HaveOccurred())

	Expect(mr.Client.Get(ctx, key, m)).ToNot(HaveOccurred())
	Expect(m.ObjectMeta.Finalizers).To(Equal([]string{metav1.FinalizerDeleteDependents}))
}
