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
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/test/helpers"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMachineFinalizer(t *testing.T) {
	bootstrapData := "some valid data"
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
		},
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				Data: &bootstrapData,
			},
			ClusterName: "valid-cluster",
		},
	}

	machineWithFinalizer := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "machine2",
			Namespace:  "default",
			Finalizers: []string{"some-other-finalizer"},
		},
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				Data: &bootstrapData,
			},
			ClusterName: "valid-cluster",
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
				NamespacedName: util.ObjectKey(machineValidCluster),
			},
			m:                  machineValidCluster,
			expectedFinalizers: []string{clusterv1.MachineFinalizer},
		},
		{
			name: "should append the machine finalizer to the machine if it already has a finalizer",
			request: reconcile.Request{
				NamespacedName: util.ObjectKey(machineWithFinalizer),
			},
			m:                  machineWithFinalizer,
			expectedFinalizers: []string{"some-other-finalizer", clusterv1.MachineFinalizer},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			mr := &MachineReconciler{
				Client: helpers.NewFakeClientWithScheme(
					scheme.Scheme,
					clusterCorrectMeta,
					machineValidCluster,
					machineWithFinalizer,
				),
				Log: log.Log,
			}

			_, _ = mr.Reconcile(tc.request)

			key := client.ObjectKey{Namespace: tc.m.Namespace, Name: tc.m.Name}
			var actual clusterv1.Machine
			if len(tc.expectedFinalizers) > 0 {
				g.Expect(mr.Client.Get(ctx, key, &actual)).To(Succeed())
				g.Expect(actual.Finalizers).ToNot(BeEmpty())
				g.Expect(actual.Finalizers).To(Equal(tc.expectedFinalizers))
			} else {
				g.Expect(actual.Finalizers).To(BeEmpty())
			}
		})
	}
}

func TestMachineOwnerReference(t *testing.T) {
	bootstrapData := "some valid data"
	testCluster := &clusterv1.Cluster{
		TypeMeta:   metav1.TypeMeta{Kind: "Cluster", APIVersion: clusterv1.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-cluster"},
	}

	machineInvalidCluster := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine1",
			Namespace: "default",
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: "invalid",
		},
	}

	machineValidCluster := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine2",
			Namespace: "default",
		},
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				Data: &bootstrapData,
			},
			ClusterName: "test-cluster",
		},
	}

	machineValidMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine3",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.ClusterLabelName: "valid-cluster",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "MachineSet",
					Name:       "valid-machineset",
					Controller: pointer.BoolPtr(true),
				},
			},
		},
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				Data: &bootstrapData,
			},
			ClusterName: "test-cluster",
		},
	}

	machineValidControlled := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine4",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.ClusterLabelName:             "valid-cluster",
				clusterv1.MachineControlPlaneLabelName: "",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "test.group",
					Kind:       "KubeadmControlPlane",
					Name:       "valid-controlplane",
					Controller: pointer.BoolPtr(true),
				},
			},
		},
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				Data: &bootstrapData,
			},
			ClusterName: "test-cluster",
		},
	}

	testCases := []struct {
		name       string
		request    reconcile.Request
		m          *clusterv1.Machine
		expectedOR []metav1.OwnerReference
	}{
		{
			name: "should add owner reference to machine referencing a cluster with correct type meta",
			request: reconcile.Request{
				NamespacedName: util.ObjectKey(machineValidCluster),
			},
			m: machineValidCluster,
			expectedOR: []metav1.OwnerReference{
				{
					APIVersion: testCluster.APIVersion,
					Kind:       testCluster.Kind,
					Name:       testCluster.Name,
					UID:        testCluster.UID,
				},
			},
		},
		{
			name: "should not add cluster owner reference if machine is owned by a machine set",
			request: reconcile.Request{
				NamespacedName: util.ObjectKey(machineValidMachine),
			},
			m: machineValidMachine,
			expectedOR: []metav1.OwnerReference{
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "MachineSet",
					Name:       "valid-machineset",
					Controller: pointer.BoolPtr(true),
				},
			},
		},
		{
			name: "should not add cluster owner reference if machine has a controller owner",
			request: reconcile.Request{
				NamespacedName: util.ObjectKey(machineValidControlled),
			},
			m: machineValidControlled,
			expectedOR: []metav1.OwnerReference{
				{
					APIVersion: "test.group",
					Kind:       "KubeadmControlPlane",
					Name:       "valid-controlplane",
					Controller: pointer.BoolPtr(true),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			mr := &MachineReconciler{
				Client: helpers.NewFakeClientWithScheme(
					scheme.Scheme,
					testCluster,
					machineInvalidCluster,
					machineValidCluster,
					machineValidMachine,
					machineValidControlled,
				),
				Log:    log.Log,
				scheme: scheme.Scheme,
			}

			key := client.ObjectKey{Namespace: tc.m.Namespace, Name: tc.m.Name}
			var actual clusterv1.Machine

			// this first requeue is to add finalizer
			result, err := mr.Reconcile(tc.request)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(result).To(Equal(ctrl.Result{}))
			g.Expect(mr.Client.Get(ctx, key, &actual)).To(Succeed())
			g.Expect(actual.Finalizers).To(ContainElement(clusterv1.MachineFinalizer))

			_, _ = mr.Reconcile(tc.request)

			if len(tc.expectedOR) > 0 {
				g.Expect(mr.Client.Get(ctx, key, &actual)).To(Succeed())
				g.Expect(actual.OwnerReferences).To(Equal(tc.expectedOR))
			} else {
				g.Expect(actual.OwnerReferences).To(BeEmpty())
			}
		})
	}
}

func TestReconcileRequest(t *testing.T) {
	infraConfig := unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "InfrastructureMachine",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha3",
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

	time := metav1.Now()

	testCluster := clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
	}

	type expected struct {
		result reconcile.Result
		err    bool
	}
	testCases := []struct {
		machine  clusterv1.Machine
		expected expected
	}{
		{
			machine: clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "created",
					Namespace:  "default",
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName: "test-cluster",
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
						Kind:       "InfrastructureMachine",
						Name:       "infra-config1",
					},
					Bootstrap: clusterv1.Bootstrap{DataSecretName: pointer.StringPtr("data")},
				},
				Status: clusterv1.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						Name: "test",
					},
					ObservedGeneration: 1,
				},
			},
			expected: expected{
				result: reconcile.Result{},
				err:    false,
			},
		},
		{
			machine: clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "updated",
					Namespace:  "default",
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName: "test-cluster",
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
						Kind:       "InfrastructureMachine",
						Name:       "infra-config1",
					},
					Bootstrap: clusterv1.Bootstrap{DataSecretName: pointer.StringPtr("data")},
				},
				Status: clusterv1.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						Name: "test",
					},
					ObservedGeneration: 1,
				},
			},
			expected: expected{
				result: reconcile.Result{},
				err:    false,
			},
		},
		{
			machine: clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deleted",
					Namespace: "default",
					Labels: map[string]string{
						clusterv1.MachineControlPlaneLabelName: "",
					},
					Finalizers:        []string{clusterv1.MachineFinalizer},
					DeletionTimestamp: &time,
				},
				Spec: clusterv1.MachineSpec{
					ClusterName: "test-cluster",
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
						Kind:       "InfrastructureMachine",
						Name:       "infra-config1",
					},
					Bootstrap: clusterv1.Bootstrap{DataSecretName: pointer.StringPtr("data")},
				},
			},
			expected: expected{
				result: reconcile.Result{},
				err:    false,
			},
		},
	}

	for _, tc := range testCases {
		t.Run("machine should be "+tc.machine.Name, func(t *testing.T) {
			g := NewWithT(t)

			clientFake := helpers.NewFakeClientWithScheme(
				scheme.Scheme,
				&testCluster,
				&tc.machine,
				external.TestGenericInfrastructureCRD.DeepCopy(),
				&infraConfig,
			)

			r := &MachineReconciler{
				Client: clientFake,
				Log:    log.Log,
				scheme: scheme.Scheme,
			}

			result, err := r.Reconcile(reconcile.Request{NamespacedName: util.ObjectKey(&tc.machine)})
			if tc.expected.err {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}

			g.Expect(result).To(Equal(tc.expected.result))
		})
	}
}

func TestMachineConditions(t *testing.T) {
	infraConfig := func(ready bool) *unstructured.Unstructured {
		return &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       "InfrastructureMachine",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha3",
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": "default",
				},
				"spec": map[string]interface{}{
					"providerID": "test://id-1",
				},
				"status": map[string]interface{}{
					"ready": ready,
					"addresses": []interface{}{
						map[string]interface{}{
							"type":    "InternalIP",
							"address": "10.0.0.1",
						},
					},
				},
			},
		}
	}

	boostrapConfig := func(ready bool) *unstructured.Unstructured {
		status := map[string]interface{}{
			"ready": ready,
		}
		if ready {
			status["dataSecretName"] = "data"
		}
		return &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       "BootstrapMachine",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "default",
				},
				"status": status,
			},
		}
	}

	testCluster := clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
	}

	machine := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blah",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.MachineControlPlaneLabelName: "",
			},
			Finalizers: []string{clusterv1.MachineFinalizer},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: "test-cluster",
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
				Kind:       "InfrastructureMachine",
				Name:       "infra-config1",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha3",
					Kind:       "BootstrapMachine",
					Name:       "bootstrap-config1",
				},
			},
		},
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Name: "test",
			},
			ObservedGeneration: 1,
		},
	}

	testcases := []struct {
		name                string
		infraReady          bool
		bootstrapReady      bool
		beforeFunc          func(bootstrap, infra *unstructured.Unstructured, m *clusterv1.Machine)
		conditionAssertFunc func(t *testing.T, getter conditions.Getter)
	}{
		{
			name:           "all conditions true",
			infraReady:     true,
			bootstrapReady: true,
			conditionAssertFunc: func(t *testing.T, getter conditions.Getter) {
				g := NewWithT(t)
				g.Expect(getter.GetConditions()).NotTo(HaveLen(0))
				for _, c := range getter.GetConditions() {
					g.Expect(c.Status).To(Equal(corev1.ConditionTrue))
				}
			},
		},
		{
			name:           "infra condition consumes reason from the infra config",
			infraReady:     false,
			bootstrapReady: true,
			beforeFunc: func(bootstrap, infra *unstructured.Unstructured, m *clusterv1.Machine) {
				addConditionsToExternal(infra, clusterv1.Conditions{
					{
						Type:     clusterv1.ReadyCondition,
						Status:   corev1.ConditionFalse,
						Severity: clusterv1.ConditionSeverityInfo,
						Reason:   "Custom reason",
					},
				})
			},
			conditionAssertFunc: func(t *testing.T, getter conditions.Getter) {
				g := NewWithT(t)

				g.Expect(conditions.Has(getter, clusterv1.InfrastructureReadyCondition)).To(BeTrue())
				infraReadyCondition := conditions.Get(getter, clusterv1.InfrastructureReadyCondition)
				g.Expect(infraReadyCondition.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(infraReadyCondition.Reason).To(Equal("Custom reason"))
			},
		},
		{
			name:           "infra condition consumes the fallback reason",
			infraReady:     false,
			bootstrapReady: true,
			conditionAssertFunc: func(t *testing.T, getter conditions.Getter) {
				g := NewWithT(t)

				g.Expect(conditions.Has(getter, clusterv1.InfrastructureReadyCondition)).To(BeTrue())
				infraReadyCondition := conditions.Get(getter, clusterv1.InfrastructureReadyCondition)
				g.Expect(infraReadyCondition.Status).To(Equal(corev1.ConditionFalse))

				g.Expect(conditions.Has(getter, clusterv1.ReadyCondition)).To(BeTrue())
				readyCondition := conditions.Get(getter, clusterv1.ReadyCondition)
				g.Expect(readyCondition.Status).To(Equal(corev1.ConditionFalse))
			},
		},
		{
			name:           "bootstrap condition consumes reason from the bootstrap config",
			infraReady:     true,
			bootstrapReady: false,
			beforeFunc: func(bootstrap, infra *unstructured.Unstructured, m *clusterv1.Machine) {
				addConditionsToExternal(bootstrap, clusterv1.Conditions{
					{
						Type:     clusterv1.ReadyCondition,
						Status:   corev1.ConditionFalse,
						Severity: clusterv1.ConditionSeverityInfo,
						Reason:   "Custom reason",
					},
				})
			},
			conditionAssertFunc: func(t *testing.T, getter conditions.Getter) {
				g := NewWithT(t)

				g.Expect(conditions.Has(getter, clusterv1.BootstrapReadyCondition)).To(BeTrue())
				infraReadyCondition := conditions.Get(getter, clusterv1.BootstrapReadyCondition)
				g.Expect(infraReadyCondition.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(infraReadyCondition.Reason).To(Equal("Custom reason"))
			},
		},
		{
			name:           "bootstrap condition consumes the fallback reason",
			infraReady:     true,
			bootstrapReady: false,
			conditionAssertFunc: func(t *testing.T, getter conditions.Getter) {
				g := NewWithT(t)

				g.Expect(conditions.Has(getter, clusterv1.BootstrapReadyCondition)).To(BeTrue())
				infraReadyCondition := conditions.Get(getter, clusterv1.BootstrapReadyCondition)
				g.Expect(infraReadyCondition.Status).To(Equal(corev1.ConditionFalse))

				g.Expect(conditions.Has(getter, clusterv1.ReadyCondition)).To(BeTrue())
				readyCondition := conditions.Get(getter, clusterv1.ReadyCondition)
				g.Expect(readyCondition.Status).To(Equal(corev1.ConditionFalse))
			},
		},
		// infra condition takes precedence over bootstrap condition in generating summary
		{
			name:           "ready condition summary consumes reason from the infra condition",
			infraReady:     false,
			bootstrapReady: false,
			conditionAssertFunc: func(t *testing.T, getter conditions.Getter) {
				g := NewWithT(t)

				g.Expect(conditions.Has(getter, clusterv1.ReadyCondition)).To(BeTrue())
				readyCondition := conditions.Get(getter, clusterv1.ReadyCondition)
				g.Expect(readyCondition.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(readyCondition.Reason).To(Equal(clusterv1.WaitingForInfrastructureFallbackReason))
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// setup objects
			bootstrap := boostrapConfig(tt.bootstrapReady)
			infra := infraConfig(tt.infraReady)
			m := machine.DeepCopy()
			if tt.beforeFunc != nil {
				tt.beforeFunc(bootstrap, infra, m)
			}

			clientFake := helpers.NewFakeClientWithScheme(
				scheme.Scheme,
				&testCluster,
				m,
				external.TestGenericInfrastructureCRD.DeepCopy(),
				infra,
				external.TestGenericBootstrapCRD.DeepCopy(),
				bootstrap,
			)

			r := &MachineReconciler{
				Client: clientFake,
				Log:    log.Log,
				scheme: scheme.Scheme,
			}

			_, err := r.Reconcile(reconcile.Request{NamespacedName: util.ObjectKey(&machine)})
			g.Expect(err).NotTo(HaveOccurred())

			m = &clusterv1.Machine{}
			machineKey, _ := client.ObjectKeyFromObject(&machine)
			g.Expect(r.Client.Get(ctx, machineKey, m)).NotTo(HaveOccurred())

			tt.conditionAssertFunc(t, m)
		})
	}
}

func TestReconcileDeleteExternal(t *testing.T) {
	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-cluster"},
	}

	bootstrapConfig := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "BootstrapConfig",
			"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
			"metadata": map[string]interface{}{
				"name":      "delete-bootstrap",
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
			ClusterName: "test-cluster",
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha3",
					Kind:       "BootstrapConfig",
					Name:       "delete-bootstrap",
				},
			},
		},
	}

	testCases := []struct {
		name            string
		bootstrapExists bool
		expectError     bool
		expected        *unstructured.Unstructured
	}{
		{
			name:            "should continue to reconcile delete of external refs if exists",
			bootstrapExists: true,
			expected: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
					"kind":       "BootstrapConfig",
					"metadata": map[string]interface{}{
						"name":            "delete-bootstrap",
						"namespace":       "default",
						"resourceVersion": "1",
					},
				},
			},
			expectError: false,
		},
		{
			name:            "should no longer reconcile deletion of external refs since it doesn't exist",
			bootstrapExists: false,
			expected:        nil,
			expectError:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			objs := []runtime.Object{testCluster, machine}

			if tc.bootstrapExists {
				objs = append(objs, bootstrapConfig)
			}

			r := &MachineReconciler{
				Client: helpers.NewFakeClientWithScheme(scheme.Scheme, objs...),
				Log:    log.Log,
				scheme: scheme.Scheme,
			}

			obj, err := r.reconcileDeleteExternal(ctx, machine, machine.Spec.Bootstrap.ConfigRef)
			g.Expect(obj).To(Equal(tc.expected))
			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

func TestRemoveMachineFinalizerAfterDeleteReconcile(t *testing.T) {
	g := NewWithT(t)

	dt := metav1.Now()

	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-cluster"},
	}

	m := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "delete123",
			Namespace:         "default",
			Finalizers:        []string{clusterv1.MachineFinalizer},
			DeletionTimestamp: &dt,
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: "test-cluster",
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
				Kind:       "InfrastructureMachine",
				Name:       "infra-config1",
			},
			Bootstrap: clusterv1.Bootstrap{Data: pointer.StringPtr("data")},
		},
	}
	key := client.ObjectKey{Namespace: m.Namespace, Name: m.Name}
	mr := &MachineReconciler{
		Client: helpers.NewFakeClientWithScheme(scheme.Scheme, testCluster, m),
		Log:    log.Log,
		scheme: scheme.Scheme,
	}
	_, err := mr.Reconcile(reconcile.Request{NamespacedName: key})
	g.Expect(err).ToNot(HaveOccurred())

	var actual clusterv1.Machine
	g.Expect(mr.Client.Get(ctx, key, &actual)).To(Succeed())
	g.Expect(actual.ObjectMeta.Finalizers).To(BeEmpty())
}

func Test_clusterToActiveMachines(t *testing.T) {
	testCluster2Machines := &clusterv1.Cluster{
		TypeMeta:   metav1.TypeMeta{Kind: "Cluster", APIVersion: clusterv1.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-cluster-2"},
	}
	testCluster0Machines := &clusterv1.Cluster{
		TypeMeta:   metav1.TypeMeta{Kind: "Cluster", APIVersion: clusterv1.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-cluster-0"},
	}

	tests := []struct {
		name    string
		cluster handler.MapObject
		want    []reconcile.Request
	}{
		{
			name: "cluster with two machines",
			cluster: handler.MapObject{
				Meta: &metav1.ObjectMeta{
					Name:      "test-cluster-2",
					Namespace: "default",
				},
				Object: testCluster2Machines,
			},
			want: []reconcile.Request{
				{
					NamespacedName: client.ObjectKey{
						Name:      "m1",
						Namespace: "default",
					},
				},
				{
					NamespacedName: client.ObjectKey{
						Name:      "m2",
						Namespace: "default",
					},
				},
			},
		},
		{
			name: "cluster with zero machines",
			cluster: handler.MapObject{
				Meta: &metav1.ObjectMeta{
					Name:      "test-cluster-0",
					Namespace: "default",
				},
				Object: testCluster0Machines,
			},
			want: []reconcile.Request{},
		},
	}
	for _, tt := range tests {
		g := NewWithT(t)

		var objs []runtime.Object
		objs = append(objs, testCluster2Machines)
		objs = append(objs, testCluster0Machines)

		m1 := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind: "Machine",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "m1",
				Namespace: "default",
				Labels: map[string]string{
					clusterv1.ClusterLabelName: "test-cluster-2",
				},
			},
		}
		objs = append(objs, m1)
		m2 := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				Kind: "Machine",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "m2",
				Namespace: "default",
				Labels: map[string]string{
					clusterv1.ClusterLabelName: "test-cluster-2",
				},
			},
		}
		objs = append(objs, m2)

		r := &MachineReconciler{
			Client: helpers.NewFakeClientWithScheme(scheme.Scheme, objs...),
			Log:    log.Log,
			scheme: scheme.Scheme,
		}

		got := r.clusterToActiveMachines(tt.cluster)
		g.Expect(got).To(Equal(tt.want))
	}
}

func TestIsNodeDrainedAllowed(t *testing.T) {
	testCluster := &clusterv1.Cluster{
		TypeMeta:   metav1.TypeMeta{Kind: "Cluster", APIVersion: clusterv1.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-cluster"},
	}

	tests := []struct {
		name     string
		machine  *clusterv1.Machine
		expected bool
	}{
		{
			name: "Exclude node draining annotation exists",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-machine",
					Namespace:   "default",
					Finalizers:  []string{clusterv1.MachineFinalizer},
					Annotations: map[string]string{clusterv1.ExcludeNodeDrainingAnnotation: "existed!!"},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{Data: pointer.StringPtr("data")},
				},
				Status: clusterv1.MachineStatus{},
			},
			expected: false,
		},
		{
			name: "Node draining timeout is over",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-machine",
					Namespace:  "default",
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{Data: pointer.StringPtr("data")},
					NodeDrainTimeout:  &metav1.Duration{Duration: time.Second * 60},
				},

				Status: clusterv1.MachineStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:               clusterv1.DrainingSucceededCondition,
							Status:             corev1.ConditionFalse,
							LastTransitionTime: metav1.Time{Time: time.Now().Add(-(time.Second * 70)).UTC()},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "Node draining timeout is not yet over",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-machine",
					Namespace:  "default",
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{Data: pointer.StringPtr("data")},
					NodeDrainTimeout:  &metav1.Duration{Duration: time.Second * 60},
				},
				Status: clusterv1.MachineStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:               clusterv1.DrainingSucceededCondition,
							Status:             corev1.ConditionFalse,
							LastTransitionTime: metav1.Time{Time: time.Now().Add(-(time.Second * 30)).UTC()},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "NodeDrainTimeout option is set to its default value 0",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-machine",
					Namespace:  "default",
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{Data: pointer.StringPtr("data")},
				},
				Status: clusterv1.MachineStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:               clusterv1.DrainingSucceededCondition,
							Status:             corev1.ConditionFalse,
							LastTransitionTime: metav1.Time{Time: time.Now().Add(-(time.Second * 1000)).UTC()},
						},
					},
				},
			},
			expected: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			var objs []runtime.Object
			objs = append(objs, testCluster, tt.machine)

			r := &MachineReconciler{
				Client: helpers.NewFakeClientWithScheme(scheme.Scheme, objs...),
				Log:    log.Log,
				scheme: scheme.Scheme,
			}

			got := r.isNodeDrainAllowed(tt.machine)
			g.Expect(got).To(Equal(tt.expected))
		})
	}
}

func TestIsDeleteNodeAllowed(t *testing.T) {
	deletionts := metav1.Now()

	testCases := []struct {
		name          string
		cluster       *clusterv1.Cluster
		machine       *clusterv1.Machine
		expectedError error
	}{
		{
			name:    "machine without nodeRef",
			cluster: &clusterv1.Cluster{},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "created",
					Namespace:  "default",
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{Data: pointer.StringPtr("data")},
				},
				Status: clusterv1.MachineStatus{},
			},
			expectedError: errNilNodeRef,
		},
		{
			name:    "no control plane members",
			cluster: &clusterv1.Cluster{},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "created",
					Namespace:  "default",
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{Data: pointer.StringPtr("data")},
				},
				Status: clusterv1.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						Name: "test",
					},
				},
			},
			expectedError: errNoControlPlaneNodes,
		},
		{
			name:    "is last control plane member",
			cluster: &clusterv1.Cluster{},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "created",
					Namespace: "default",
					Labels: map[string]string{
						clusterv1.ClusterLabelName:             "test",
						clusterv1.MachineControlPlaneLabelName: "",
					},
					Finalizers:        []string{clusterv1.MachineFinalizer},
					DeletionTimestamp: &metav1.Time{Time: time.Now().UTC()},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{Data: pointer.StringPtr("data")},
				},
				Status: clusterv1.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						Name: "test",
					},
				},
			},
			expectedError: errNoControlPlaneNodes,
		},
		{
			name:    "has nodeRef and control plane is healthy",
			cluster: &clusterv1.Cluster{},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "created",
					Namespace: "default",
					Labels: map[string]string{
						clusterv1.ClusterLabelName: "test",
					},
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{Data: pointer.StringPtr("data")},
				},
				Status: clusterv1.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						Name: "test",
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "has nodeRef and cluster is being deleted",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &deletionts,
				},
			},
			machine:       &clusterv1.Machine{},
			expectedError: errClusterIsBeingDeleted,
		},
		{
			name: "has nodeRef and control plane is healthy and externally managed",
			cluster: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					ControlPlaneRef: &corev1.ObjectReference{
						APIVersion: "controlplane.cluster.x-k8s.io/v1alpha3",
						Kind:       "AWSManagedControlPlane",
						Name:       "test-cluster",
						Namespace:  "test-cluster",
					},
				},
			},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "created",
					Namespace: "default",
					Labels: map[string]string{
						clusterv1.ClusterLabelName: "test",
					},
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{Data: pointer.StringPtr("data")},
				},
				Status: clusterv1.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						Name: "test",
					},
				},
			},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			m1 := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cp1",
					Namespace: "default",
					Labels: map[string]string{
						clusterv1.ClusterLabelName: "test",
					},
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{Data: pointer.StringPtr("data")},
				},
				Status: clusterv1.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						Name: "test1",
					},
				},
			}
			m2 := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cp2",
					Namespace: "default",
					Labels: map[string]string{
						clusterv1.ClusterLabelName: "test",
					},
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{Data: pointer.StringPtr("data")},
				},
				Status: clusterv1.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						Name: "test2",
					},
				},
			}
			// For isDeleteNodeAllowed to be true we assume a healthy control plane.
			if tc.expectedError == nil {
				m1.Labels[clusterv1.MachineControlPlaneLabelName] = ""
				m2.Labels[clusterv1.MachineControlPlaneLabelName] = ""
			}

			emp := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"externalManagedControlPlane": true,
					},
				},
			}
			emp.SetAPIVersion("controlplane.cluster.x-k8s.io/v1alpha3")
			emp.SetKind("AWSManagedControlPlane")
			emp.SetName("test-cluster")
			emp.SetNamespace("test-cluster")

			mr := &MachineReconciler{
				Client: helpers.NewFakeClientWithScheme(
					scheme.Scheme,
					tc.cluster,
					tc.machine,
					m1,
					m2,
					emp,
				),
				Log:    log.Log,
				scheme: scheme.Scheme,
			}

			err := mr.isDeleteNodeAllowed(context.TODO(), tc.cluster, tc.machine)
			if tc.expectedError == nil {
				g.Expect(err).To(BeNil())
			} else {
				g.Expect(err).To(Equal(tc.expectedError))
			}
		})
	}
}

// adds a condition list to an external object
func addConditionsToExternal(u *unstructured.Unstructured, newConditions clusterv1.Conditions) {
	existingConditions := clusterv1.Conditions{}
	if cs := conditions.UnstructuredGetter(u).GetConditions(); len(cs) != 0 {
		existingConditions = cs
	}
	existingConditions = append(existingConditions, newConditions...)
	conditions.UnstructuredSetter(u).SetConditions(existingConditions)
}
