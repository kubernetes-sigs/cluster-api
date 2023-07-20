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

package machine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/internal/test/builder"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
)

func TestWatches(t *testing.T) {
	g := NewWithT(t)
	ns, err := env.CreateNamespace(ctx, "test-machine-watches")
	g.Expect(err).ToNot(HaveOccurred())

	infraMachine := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericInfrastructureMachine",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"name":      "infra-config1",
				"namespace": ns.Name,
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

	defaultBootstrap := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericBootstrapConfig",
			"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"name":      "bootstrap-config-machinereconcile",
				"namespace": ns.Name,
			},
			"spec":   map[string]interface{}{},
			"status": map[string]interface{}{},
		},
	}

	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "machine-reconcile-",
			Namespace:    ns.Name,
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node-1",
			Namespace: ns.Name,
		},
		Spec: corev1.NodeSpec{
			ProviderID: "test://id-1",
		},
	}

	g.Expect(env.Create(ctx, testCluster)).To(Succeed())
	g.Expect(env.CreateKubeconfigSecret(ctx, testCluster)).To(Succeed())
	g.Expect(env.Create(ctx, defaultBootstrap)).To(Succeed())
	g.Expect(env.Create(ctx, node)).To(Succeed())
	g.Expect(env.Create(ctx, infraMachine)).To(Succeed())

	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(ns, testCluster, defaultBootstrap)

	// Patch infra machine ready
	patchHelper, err := patch.NewHelper(infraMachine, env)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(unstructured.SetNestedField(infraMachine.Object, true, "status", "ready")).To(Succeed())
	g.Expect(patchHelper.Patch(ctx, infraMachine, patch.WithStatusObservedGeneration{})).To(Succeed())

	// Patch bootstrap ready
	patchHelper, err = patch.NewHelper(defaultBootstrap, env)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(unstructured.SetNestedField(defaultBootstrap.Object, true, "status", "ready")).To(Succeed())
	g.Expect(unstructured.SetNestedField(defaultBootstrap.Object, "secretData", "status", "dataSecretName")).To(Succeed())
	g.Expect(patchHelper.Patch(ctx, defaultBootstrap, patch.WithStatusObservedGeneration{})).To(Succeed())

	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "machine-created-",
			Namespace:    ns.Name,
			Labels: map[string]string{
				clusterv1.MachineControlPlaneLabel: "",
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: testCluster.Name,
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "GenericInfrastructureMachine",
				Name:       "infra-config1",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
					Kind:       "GenericBootstrapConfig",
					Name:       "bootstrap-config-machinereconcile",
				},
			}},
	}

	g.Expect(env.Create(ctx, machine)).To(Succeed())
	defer func() {
		g.Expect(env.Cleanup(ctx, machine)).To(Succeed())
	}()

	// Wait for reconciliation to happen.
	// Since infra and bootstrap objects are ready, a nodeRef will be assigned during node reconciliation.
	key := client.ObjectKey{Name: machine.Name, Namespace: machine.Namespace}
	g.Eventually(func() bool {
		if err := env.Get(ctx, key, machine); err != nil {
			return false
		}
		return machine.Status.NodeRef != nil
	}, timeout).Should(BeTrue())

	// Node deletion will trigger node watchers and a request will be added to the queue.
	g.Expect(env.Delete(ctx, node)).To(Succeed())
	// TODO: Once conditions are in place, check if node deletion triggered a reconcile.

	// Delete infra machine, external tracker will trigger reconcile
	// and machine Status.FailureReason should be non-nil after reconcileInfrastructure
	g.Expect(env.Delete(ctx, infraMachine)).To(Succeed())
	g.Eventually(func() bool {
		if err := env.Get(ctx, key, machine); err != nil {
			return false
		}
		return machine.Status.FailureMessage != nil
	}, timeout).Should(BeTrue())
}

func TestMachine_Reconcile(t *testing.T) {
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "test-machine-reconcile")
	g.Expect(err).ToNot(HaveOccurred())

	infraMachine := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericInfrastructureMachine",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"name":      "infra-config1",
				"namespace": ns.Name,
			},
			"spec": map[string]interface{}{
				"providerID": "test://id-1",
			},
		},
	}

	defaultBootstrap := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericBootstrapConfig",
			"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"name":      "bootstrap-config-machinereconcile",
				"namespace": ns.Name,
			},
			"spec":   map[string]interface{}{},
			"status": map[string]interface{}{},
		},
	}

	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "machine-reconcile-",
			Namespace:    ns.Name,
		},
	}

	g.Expect(env.Create(ctx, testCluster)).To(Succeed())
	g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
	g.Expect(env.Create(ctx, defaultBootstrap)).To(Succeed())

	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(ns, testCluster, defaultBootstrap)

	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "machine-created-",
			Namespace:    ns.Name,
			Finalizers:   []string{clusterv1.MachineFinalizer},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: testCluster.Name,
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "GenericInfrastructureMachine",
				Name:       "infra-config1",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
					Kind:       "GenericBootstrapConfig",
					Name:       "bootstrap-config-machinereconcile",
				},
			}},
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Name: "test",
			},
		},
	}
	g.Expect(env.Create(ctx, machine)).To(Succeed())

	key := client.ObjectKey{Name: machine.Name, Namespace: machine.Namespace}

	// Wait for reconciliation to happen when infra and bootstrap objects are not ready.
	g.Eventually(func() bool {
		if err := env.Get(ctx, key, machine); err != nil {
			return false
		}
		return len(machine.Finalizers) > 0
	}, timeout).Should(BeTrue())

	// Set bootstrap ready.
	bootstrapPatch := client.MergeFrom(defaultBootstrap.DeepCopy())
	g.Expect(unstructured.SetNestedField(defaultBootstrap.Object, true, "status", "ready")).ToNot(HaveOccurred())
	g.Expect(env.Status().Patch(ctx, defaultBootstrap, bootstrapPatch)).To(Succeed())

	// Set infrastructure ready.
	infraMachinePatch := client.MergeFrom(infraMachine.DeepCopy())
	g.Expect(unstructured.SetNestedField(infraMachine.Object, true, "status", "ready")).To(Succeed())
	g.Expect(env.Status().Patch(ctx, infraMachine, infraMachinePatch)).To(Succeed())

	// Wait for Machine Ready Condition to become True.
	g.Eventually(func() bool {
		if err := env.Get(ctx, key, machine); err != nil {
			return false
		}
		if !conditions.Has(machine, clusterv1.InfrastructureReadyCondition) {
			return false
		}
		readyCondition := conditions.Get(machine, clusterv1.ReadyCondition)
		return readyCondition.Status == corev1.ConditionTrue
	}, timeout).Should(BeTrue())

	g.Expect(env.Delete(ctx, machine)).ToNot(HaveOccurred())
	// Wait for Machine to be deleted.
	g.Eventually(func() bool {
		if err := env.Get(ctx, key, machine); err != nil {
			if apierrors.IsNotFound(err) {
				return true
			}
		}
		return false
	}, timeout).Should(BeTrue())

	// Check if Machine deletion successfully deleted infrastructure external reference.
	keyInfra := client.ObjectKey{Name: infraMachine.GetName(), Namespace: infraMachine.GetNamespace()}
	g.Eventually(func() bool {
		if err := env.Get(ctx, keyInfra, infraMachine); err != nil {
			if apierrors.IsNotFound(err) {
				return true
			}
		}
		return false
	}, timeout).Should(BeTrue())

	// Check if Machine deletion successfully deleted bootstrap external reference.
	keyBootstrap := client.ObjectKey{Name: defaultBootstrap.GetName(), Namespace: defaultBootstrap.GetNamespace()}
	g.Eventually(func() bool {
		if err := env.Get(ctx, keyBootstrap, defaultBootstrap); err != nil {
			if apierrors.IsNotFound(err) {
				return true
			}
		}
		return false
	}, timeout).Should(BeTrue())
}

func TestMachineFinalizer(t *testing.T) {
	bootstrapData := "some valid data"
	clusterCorrectMeta := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "valid-cluster",
			Namespace: metav1.NamespaceDefault,
		},
	}

	machineValidCluster := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine1",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: &bootstrapData,
			},
			ClusterName: "valid-cluster",
		},
	}

	machineWithFinalizer := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "machine2",
			Namespace:  metav1.NamespaceDefault,
			Finalizers: []string{"some-other-finalizer"},
		},
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: &bootstrapData,
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

			c := fake.NewClientBuilder().WithObjects(
				clusterCorrectMeta,
				machineValidCluster,
				machineWithFinalizer,
			).Build()
			mr := &Reconciler{
				Client:                    c,
				UnstructuredCachingClient: c,
			}

			_, _ = mr.Reconcile(ctx, tc.request)

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
		ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "test-cluster"},
	}

	machineInvalidCluster := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine1",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: "invalid",
		},
	}

	machineValidCluster := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine2",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: &bootstrapData,
			},
			ClusterName: "test-cluster",
		},
	}

	machineValidMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine3",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "valid-cluster",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "MachineSet",
					Name:       "valid-machineset",
					Controller: pointer.Bool(true),
				},
			},
		},
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: &bootstrapData,
			},
			ClusterName: "test-cluster",
		},
	}

	machineValidControlled := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine4",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:         "valid-cluster",
				clusterv1.MachineControlPlaneLabel: "",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "test.group",
					Kind:       "KubeadmControlPlane",
					Name:       "valid-controlplane",
					Controller: pointer.Bool(true),
				},
			},
		},
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: &bootstrapData,
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
					Controller: pointer.Bool(true),
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
					Controller: pointer.Bool(true),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			c := fake.NewClientBuilder().WithObjects(
				testCluster,
				machineInvalidCluster,
				machineValidCluster,
				machineValidMachine,
				machineValidControlled,
			).WithStatusSubresource(&clusterv1.Machine{}).Build()
			mr := &Reconciler{
				Client:                    c,
				UnstructuredCachingClient: c,
				APIReader:                 c,
			}

			key := client.ObjectKey{Namespace: tc.m.Namespace, Name: tc.m.Name}
			var actual clusterv1.Machine

			// this first requeue is to add finalizer
			result, err := mr.Reconcile(ctx, tc.request)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(result).To(BeComparableTo(ctrl.Result{}))
			g.Expect(mr.Client.Get(ctx, key, &actual)).To(Succeed())
			g.Expect(actual.Finalizers).To(ContainElement(clusterv1.MachineFinalizer))

			_, _ = mr.Reconcile(ctx, tc.request)

			if len(tc.expectedOR) > 0 {
				g.Expect(mr.Client.Get(ctx, key, &actual)).To(Succeed())
				g.Expect(actual.OwnerReferences).To(BeComparableTo(tc.expectedOR))
			} else {
				g.Expect(actual.OwnerReferences).To(BeEmpty())
			}
		})
	}
}

func TestReconcileRequest(t *testing.T) {
	infraConfig := unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericInfrastructureMachine",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"name":      "infra-config1",
				"namespace": metav1.NamespaceDefault,
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
			Namespace: metav1.NamespaceDefault,
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: corev1.NodeSpec{ProviderID: "test://id-1"},
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
					Namespace:  metav1.NamespaceDefault,
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName: "test-cluster",
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
						Kind:       "GenericInfrastructureMachine",
						Name:       "infra-config1",
					},
					Bootstrap: clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
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
					Namespace:  metav1.NamespaceDefault,
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName: "test-cluster",
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
						Kind:       "GenericInfrastructureMachine",
						Name:       "infra-config1",
					},
					Bootstrap: clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
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
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.MachineControlPlaneLabel: "",
					},
					Finalizers:        []string{clusterv1.MachineFinalizer},
					DeletionTimestamp: &time,
				},
				Spec: clusterv1.MachineSpec{
					ClusterName: "test-cluster",
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
						Kind:       "GenericInfrastructureMachine",
						Name:       "infra-config1",
					},
					Bootstrap: clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
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

			clientFake := fake.NewClientBuilder().WithObjects(
				node,
				&testCluster,
				&tc.machine,
				builder.GenericInfrastructureMachineCRD.DeepCopy(),
				&infraConfig,
			).WithStatusSubresource(&clusterv1.Machine{}).WithIndex(&corev1.Node{}, index.NodeProviderIDField, index.NodeByProviderID).Build()

			r := &Reconciler{
				Client:                    clientFake,
				UnstructuredCachingClient: clientFake,
				Tracker:                   remote.NewTestClusterCacheTracker(logr.New(log.NullLogSink{}), clientFake, scheme.Scheme, client.ObjectKey{Name: testCluster.Name, Namespace: testCluster.Namespace}),
				ssaCache:                  ssa.NewCache(),
			}

			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: util.ObjectKey(&tc.machine)})
			if tc.expected.err {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			g.Expect(result).To(BeComparableTo(tc.expected.result))
		})
	}
}

func TestMachineConditions(t *testing.T) {
	infraConfig := func(ready bool) *unstructured.Unstructured {
		return &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": metav1.NamespaceDefault,
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
				"kind":       "GenericBootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"status": status,
			},
		}
	}

	testCluster := clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
		},
	}

	machine := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "blah",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.MachineControlPlaneLabel: "",
			},
			Finalizers: []string{clusterv1.MachineFinalizer},
		},
		Spec: clusterv1.MachineSpec{
			ProviderID:  pointer.String("test://id-1"),
			ClusterName: "test-cluster",
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "GenericInfrastructureMachine",
				Name:       "infra-config1",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
					Kind:       "GenericBootstrapConfig",
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

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: corev1.NodeSpec{ProviderID: "test://id-1"},
	}

	testcases := []struct {
		name               string
		infraReady         bool
		bootstrapReady     bool
		beforeFunc         func(bootstrap, infra *unstructured.Unstructured, m *clusterv1.Machine)
		conditionsToAssert []*clusterv1.Condition
	}{
		{
			name:           "all conditions true",
			infraReady:     true,
			bootstrapReady: true,
			beforeFunc: func(bootstrap, infra *unstructured.Unstructured, m *clusterv1.Machine) {
				// since these conditions are set by an external controller
				conditions.MarkTrue(m, clusterv1.MachineHealthCheckSucceededCondition)
				conditions.MarkTrue(m, clusterv1.MachineOwnerRemediatedCondition)
			},
			conditionsToAssert: []*clusterv1.Condition{
				conditions.TrueCondition(clusterv1.InfrastructureReadyCondition),
				conditions.TrueCondition(clusterv1.BootstrapReadyCondition),
				conditions.TrueCondition(clusterv1.MachineOwnerRemediatedCondition),
				conditions.TrueCondition(clusterv1.MachineHealthCheckSucceededCondition),
				conditions.TrueCondition(clusterv1.ReadyCondition),
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
			conditionsToAssert: []*clusterv1.Condition{
				conditions.FalseCondition(clusterv1.InfrastructureReadyCondition, "Custom reason", clusterv1.ConditionSeverityInfo, ""),
			},
		},
		{
			name:           "infra condition consumes the fallback reason",
			infraReady:     false,
			bootstrapReady: true,
			conditionsToAssert: []*clusterv1.Condition{
				conditions.FalseCondition(clusterv1.InfrastructureReadyCondition, clusterv1.WaitingForInfrastructureFallbackReason, clusterv1.ConditionSeverityInfo, ""),
				conditions.FalseCondition(clusterv1.ReadyCondition, clusterv1.WaitingForInfrastructureFallbackReason, clusterv1.ConditionSeverityInfo, ""),
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
			conditionsToAssert: []*clusterv1.Condition{
				conditions.FalseCondition(clusterv1.BootstrapReadyCondition, "Custom reason", clusterv1.ConditionSeverityInfo, ""),
			},
		},
		{
			name:           "bootstrap condition consumes the fallback reason",
			infraReady:     true,
			bootstrapReady: false,
			conditionsToAssert: []*clusterv1.Condition{
				conditions.FalseCondition(clusterv1.BootstrapReadyCondition, clusterv1.WaitingForDataSecretFallbackReason, clusterv1.ConditionSeverityInfo, ""),
				conditions.FalseCondition(clusterv1.ReadyCondition, clusterv1.WaitingForDataSecretFallbackReason, clusterv1.ConditionSeverityInfo, ""),
			},
		},
		// Assert summary conditions
		// infra condition takes precedence over bootstrap condition in generating summary
		{
			name:           "ready condition summary consumes reason from the infra condition",
			infraReady:     false,
			bootstrapReady: false,
			conditionsToAssert: []*clusterv1.Condition{
				conditions.FalseCondition(clusterv1.ReadyCondition, clusterv1.WaitingForInfrastructureFallbackReason, clusterv1.ConditionSeverityInfo, ""),
			},
		},
		{
			name:           "ready condition summary consumes reason from the machine owner remediated condition",
			infraReady:     true,
			bootstrapReady: true,
			beforeFunc: func(bootstrap, infra *unstructured.Unstructured, m *clusterv1.Machine) {
				conditions.MarkFalse(m, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "MHC failed")
			},
			conditionsToAssert: []*clusterv1.Condition{
				conditions.FalseCondition(clusterv1.ReadyCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "MHC failed"),
			},
		},
		{
			name:           "ready condition summary consumes reason from the MHC succeeded condition",
			infraReady:     true,
			bootstrapReady: true,
			beforeFunc: func(bootstrap, infra *unstructured.Unstructured, m *clusterv1.Machine) {
				conditions.MarkFalse(m, clusterv1.MachineHealthCheckSucceededCondition, clusterv1.NodeNotFoundReason, clusterv1.ConditionSeverityWarning, "")
			},
			conditionsToAssert: []*clusterv1.Condition{
				conditions.FalseCondition(clusterv1.ReadyCondition, clusterv1.NodeNotFoundReason, clusterv1.ConditionSeverityWarning, ""),
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

			clientFake := fake.NewClientBuilder().WithObjects(
				&testCluster,
				m,
				builder.GenericInfrastructureMachineCRD.DeepCopy(),
				infra,
				builder.GenericBootstrapConfigCRD.DeepCopy(),
				bootstrap,
				node,
			).
				WithIndex(&corev1.Node{}, index.NodeProviderIDField, index.NodeByProviderID).
				WithStatusSubresource(&clusterv1.Machine{}).
				Build()

			r := &Reconciler{
				Client:                    clientFake,
				UnstructuredCachingClient: clientFake,
				Tracker:                   remote.NewTestClusterCacheTracker(logr.New(log.NullLogSink{}), clientFake, scheme.Scheme, client.ObjectKey{Name: testCluster.Name, Namespace: testCluster.Namespace}),
				ssaCache:                  ssa.NewCache(),
			}

			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: util.ObjectKey(&machine)})
			g.Expect(err).ToNot(HaveOccurred())

			m = &clusterv1.Machine{}
			g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(&machine), m)).ToNot(HaveOccurred())

			assertConditions(t, m, tt.conditionsToAssert...)
		})
	}
}

func TestReconcileDeleteExternal(t *testing.T) {
	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "test-cluster"},
	}

	bootstrapConfig := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "BootstrapConfig",
			"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"name":      "delete-bootstrap",
				"namespace": metav1.NamespaceDefault,
			},
		},
	}

	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delete",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: "test-cluster",
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
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
					"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
					"kind":       "BootstrapConfig",
					"metadata": map[string]interface{}{
						"name":            "delete-bootstrap",
						"namespace":       metav1.NamespaceDefault,
						"resourceVersion": "999",
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

			objs := []client.Object{testCluster, machine}

			if tc.bootstrapExists {
				objs = append(objs, bootstrapConfig)
			}

			c := fake.NewClientBuilder().WithObjects(objs...).Build()
			r := &Reconciler{
				Client:                    c,
				UnstructuredCachingClient: c,
			}

			obj, err := r.reconcileDeleteExternal(ctx, machine, machine.Spec.Bootstrap.ConfigRef)
			g.Expect(obj).To(BeComparableTo(tc.expected))
			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestRemoveMachineFinalizerAfterDeleteReconcile(t *testing.T) {
	g := NewWithT(t)

	dt := metav1.Now()

	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "test-cluster"},
	}

	m := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "delete123",
			Namespace:         metav1.NamespaceDefault,
			Finalizers:        []string{clusterv1.MachineFinalizer, "test"},
			DeletionTimestamp: &dt,
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: "test-cluster",
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "GenericInfrastructureMachine",
				Name:       "infra-config1",
			},
			Bootstrap: clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
		},
	}
	key := client.ObjectKey{Namespace: m.Namespace, Name: m.Name}
	c := fake.NewClientBuilder().WithObjects(testCluster, m).WithStatusSubresource(&clusterv1.Machine{}).Build()
	mr := &Reconciler{
		Client:                    c,
		UnstructuredCachingClient: c,
	}
	_, err := mr.Reconcile(ctx, reconcile.Request{NamespacedName: key})
	g.Expect(err).ToNot(HaveOccurred())

	var actual clusterv1.Machine
	g.Expect(mr.Client.Get(ctx, key, &actual)).To(Succeed())
	g.Expect(actual.ObjectMeta.Finalizers).To(Equal([]string{"test"}))
}

func TestIsNodeDrainedAllowed(t *testing.T) {
	testCluster := &clusterv1.Cluster{
		TypeMeta:   metav1.TypeMeta{Kind: "Cluster", APIVersion: clusterv1.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "test-cluster"},
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
					Namespace:   metav1.NamespaceDefault,
					Finalizers:  []string{clusterv1.MachineFinalizer},
					Annotations: map[string]string{clusterv1.ExcludeNodeDrainingAnnotation: "existed!!"},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
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
					Namespace:  metav1.NamespaceDefault,
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
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
					Namespace:  metav1.NamespaceDefault,
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
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
					Namespace:  metav1.NamespaceDefault,
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
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

			var objs []client.Object
			objs = append(objs, testCluster, tt.machine)

			c := fake.NewClientBuilder().WithObjects(objs...).Build()
			r := &Reconciler{
				Client:                    c,
				UnstructuredCachingClient: c,
			}

			got := r.isNodeDrainAllowed(tt.machine)
			g.Expect(got).To(Equal(tt.expected))
		})
	}
}

func TestIsNodeVolumeDetachingAllowed(t *testing.T) {
	testCluster := &clusterv1.Cluster{
		TypeMeta:   metav1.TypeMeta{Kind: "Cluster", APIVersion: clusterv1.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "test-cluster"},
	}

	tests := []struct {
		name     string
		machine  *clusterv1.Machine
		expected bool
	}{
		{
			name: "Exclude wait node volume detaching annotation exists",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-machine",
					Namespace:   metav1.NamespaceDefault,
					Finalizers:  []string{clusterv1.MachineFinalizer},
					Annotations: map[string]string{clusterv1.ExcludeWaitForNodeVolumeDetachAnnotation: "existed!!"},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
				},
				Status: clusterv1.MachineStatus{},
			},
			expected: false,
		},
		{
			name: "Volume detach timeout is over",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-machine",
					Namespace:  metav1.NamespaceDefault,
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:             "test-cluster",
					InfrastructureRef:       corev1.ObjectReference{},
					Bootstrap:               clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
					NodeVolumeDetachTimeout: &metav1.Duration{Duration: time.Second * 30},
				},

				Status: clusterv1.MachineStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:               clusterv1.VolumeDetachSucceededCondition,
							Status:             corev1.ConditionFalse,
							LastTransitionTime: metav1.Time{Time: time.Now().Add(-(time.Second * 60)).UTC()},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "Volume detach timeout is not yet over",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-machine",
					Namespace:  metav1.NamespaceDefault,
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:             "test-cluster",
					InfrastructureRef:       corev1.ObjectReference{},
					Bootstrap:               clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
					NodeVolumeDetachTimeout: &metav1.Duration{Duration: time.Second * 60},
				},
				Status: clusterv1.MachineStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:               clusterv1.VolumeDetachSucceededCondition,
							Status:             corev1.ConditionFalse,
							LastTransitionTime: metav1.Time{Time: time.Now().Add(-(time.Second * 30)).UTC()},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "Volume detach timeout option is set to it's default value 0",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-machine",
					Namespace:  metav1.NamespaceDefault,
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
				},
				Status: clusterv1.MachineStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:               clusterv1.VolumeDetachSucceededCondition,
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

			var objs []client.Object
			objs = append(objs, testCluster, tt.machine)

			c := fake.NewClientBuilder().WithObjects(objs...).Build()
			r := &Reconciler{
				Client:                    c,
				UnstructuredCachingClient: c,
			}

			got := r.isNodeVolumeDetachingAllowed(tt.machine)
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
			name: "machine without nodeRef",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: metav1.NamespaceDefault,
				},
			},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "created",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: "test-cluster",
					},
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
				},
				Status: clusterv1.MachineStatus{},
			},
			expectedError: errNilNodeRef,
		},
		{
			name: "no control plane members",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: metav1.NamespaceDefault,
				},
			},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "created",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: "test-cluster",
					},
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
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
			name: "is last control plane member",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: metav1.NamespaceDefault,
				},
			},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "created",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.ClusterNameLabel:         "test-cluster",
						clusterv1.MachineControlPlaneLabel: "",
					},
					Finalizers:        []string{clusterv1.MachineFinalizer},
					DeletionTimestamp: &metav1.Time{Time: time.Now().UTC()},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
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
			name: "has nodeRef and control plane is healthy",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: metav1.NamespaceDefault,
				},
			},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "created",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: "test-cluster",
					},
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
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
					Name:              "test-cluster",
					Namespace:         metav1.NamespaceDefault,
					DeletionTimestamp: &deletionts,
					Finalizers:        []string{clusterv1.ClusterFinalizer},
				},
			},
			machine:       &clusterv1.Machine{},
			expectedError: errClusterIsBeingDeleted,
		},
		{
			name: "has nodeRef and control plane is healthy and externally managed",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.ClusterSpec{
					ControlPlaneRef: &corev1.ObjectReference{
						APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
						Kind:       "AWSManagedControlPlane",
						Name:       "test-cluster",
						Namespace:  "test-cluster",
					},
				},
			},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "created",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: "test-cluster",
					},
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
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
			name: "has nodeRef, control plane is being deleted and not externally managed",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.ClusterSpec{
					ControlPlaneRef: &corev1.ObjectReference{
						APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
						Kind:       "AWSManagedControlPlane",
						Name:       "test-cluster-2",
						Namespace:  "test-cluster",
					},
				},
			},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "created",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: "test-cluster",
					},
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
				},
				Status: clusterv1.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						Name: "test",
					},
				},
			},
			expectedError: errControlPlaneIsBeingDeleted,
		},
		{
			name: "has nodeRef, control plane is being deleted and is externally managed",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.ClusterSpec{
					ControlPlaneRef: &corev1.ObjectReference{
						APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
						Kind:       "AWSManagedControlPlane",
						Name:       "test-cluster-3",
						Namespace:  "test-cluster",
					},
				},
			},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "created",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: "test-cluster",
					},
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
				},
				Status: clusterv1.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						Name: "test",
					},
				},
			},
			expectedError: errControlPlaneIsBeingDeleted,
		},
	}

	emp := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"externalManagedControlPlane": true,
			},
		},
	}
	emp.SetAPIVersion("controlplane.cluster.x-k8s.io/v1beta1")
	emp.SetKind("AWSManagedControlPlane")
	emp.SetName("test-cluster")
	emp.SetNamespace("test-cluster")

	mcpBeingDeleted := &unstructured.Unstructured{
		Object: map[string]interface{}{},
	}
	mcpBeingDeleted.SetAPIVersion("controlplane.cluster.x-k8s.io/v1beta1")
	mcpBeingDeleted.SetKind("AWSManagedControlPlane")
	mcpBeingDeleted.SetName("test-cluster-2")
	mcpBeingDeleted.SetNamespace("test-cluster")
	mcpBeingDeleted.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
	mcpBeingDeleted.SetFinalizers([]string{"block-deletion"})

	empBeingDeleted := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"externalManagedControlPlane": true,
			},
		},
	}
	empBeingDeleted.SetAPIVersion("controlplane.cluster.x-k8s.io/v1beta1")
	empBeingDeleted.SetKind("AWSManagedControlPlane")
	empBeingDeleted.SetName("test-cluster-3")
	empBeingDeleted.SetNamespace("test-cluster")
	empBeingDeleted.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
	empBeingDeleted.SetFinalizers([]string{"block-deletion"})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			m1 := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cp1",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: "test-cluster",
					},
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
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
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: "test-cluster",
					},
					Finalizers: []string{clusterv1.MachineFinalizer},
				},
				Spec: clusterv1.MachineSpec{
					ClusterName:       "test-cluster",
					InfrastructureRef: corev1.ObjectReference{},
					Bootstrap:         clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
				},
				Status: clusterv1.MachineStatus{
					NodeRef: &corev1.ObjectReference{
						Name: "test2",
					},
				},
			}
			// For isDeleteNodeAllowed to be true we assume a healthy control plane.
			if tc.expectedError == nil {
				m1.Labels[clusterv1.MachineControlPlaneLabel] = ""
				m2.Labels[clusterv1.MachineControlPlaneLabel] = ""
			}

			c := fake.NewClientBuilder().WithObjects(
				tc.cluster,
				tc.machine,
				m1,
				m2,
				emp,
				mcpBeingDeleted,
				empBeingDeleted,
			).Build()
			mr := &Reconciler{
				Client:                    c,
				UnstructuredCachingClient: c,
			}

			err := mr.isDeleteNodeAllowed(ctx, tc.cluster, tc.machine)
			if tc.expectedError == nil {
				g.Expect(err).ToNot(HaveOccurred())
			} else {
				g.Expect(err).To(Equal(tc.expectedError))
			}
		})
	}
}

func TestNodeToMachine(t *testing.T) {
	g := NewWithT(t)
	ns, err := env.CreateNamespace(ctx, "test-node-to-machine")
	g.Expect(err).ToNot(HaveOccurred())

	// Set up cluster, machines and nodes to test against.
	infraMachine := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericInfrastructureMachine",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"name":      "infra-config1",
				"namespace": ns.Name,
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

	infraMachine2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericInfrastructureMachine",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"name":      "infra-config2",
				"namespace": ns.Name,
			},
			"spec": map[string]interface{}{
				"providerID": "test://id-2",
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

	defaultBootstrap := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericBootstrapConfig",
			"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"name":      "bootstrap-config-machinereconcile",
				"namespace": ns.Name,
			},
			"spec":   map[string]interface{}{},
			"status": map[string]interface{}{},
		},
	}

	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "machine-reconcile-",
			Namespace:    ns.Name,
		},
	}

	targetNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-to-machine-1",
		},
		Spec: corev1.NodeSpec{
			ProviderID: "test://id-1",
		},
	}

	randomNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-to-machine-node-2",
		},
		Spec: corev1.NodeSpec{
			ProviderID: "test://id-2",
		},
	}

	g.Expect(env.Create(ctx, testCluster)).To(Succeed())
	g.Expect(env.CreateKubeconfigSecret(ctx, testCluster)).To(Succeed())
	g.Expect(env.Create(ctx, defaultBootstrap)).To(Succeed())
	g.Expect(env.Create(ctx, targetNode)).To(Succeed())
	g.Expect(env.Create(ctx, randomNode)).To(Succeed())
	g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
	g.Expect(env.Create(ctx, infraMachine2)).To(Succeed())

	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(ns, testCluster, defaultBootstrap)

	// Patch infra expectedMachine ready
	patchHelper, err := patch.NewHelper(infraMachine, env)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(unstructured.SetNestedField(infraMachine.Object, true, "status", "ready")).To(Succeed())
	g.Expect(patchHelper.Patch(ctx, infraMachine, patch.WithStatusObservedGeneration{})).To(Succeed())

	// Patch infra randomMachine ready
	patchHelper, err = patch.NewHelper(infraMachine2, env)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(unstructured.SetNestedField(infraMachine2.Object, true, "status", "ready")).To(Succeed())
	g.Expect(patchHelper.Patch(ctx, infraMachine2, patch.WithStatusObservedGeneration{})).To(Succeed())

	// Patch bootstrap ready
	patchHelper, err = patch.NewHelper(defaultBootstrap, env)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(unstructured.SetNestedField(defaultBootstrap.Object, true, "status", "ready")).To(Succeed())
	g.Expect(unstructured.SetNestedField(defaultBootstrap.Object, "secretData", "status", "dataSecretName")).To(Succeed())
	g.Expect(patchHelper.Patch(ctx, defaultBootstrap, patch.WithStatusObservedGeneration{})).To(Succeed())

	expectedMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "machine-created-",
			Namespace:    ns.Name,
			Labels: map[string]string{
				clusterv1.MachineControlPlaneLabel: "",
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: testCluster.Name,
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "GenericInfrastructureMachine",
				Name:       "infra-config1",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
					Kind:       "GenericBootstrapConfig",
					Name:       "bootstrap-config-machinereconcile",
				},
			}},
	}

	g.Expect(env.Create(ctx, expectedMachine)).To(Succeed())
	defer func() {
		g.Expect(env.Cleanup(ctx, expectedMachine)).To(Succeed())
	}()

	// Wait for reconciliation to happen.
	// Since infra and bootstrap objects are ready, a nodeRef will be assigned during node reconciliation.
	key := client.ObjectKey{Name: expectedMachine.Name, Namespace: expectedMachine.Namespace}
	g.Eventually(func() bool {
		if err := env.Get(ctx, key, expectedMachine); err != nil {
			return false
		}
		return expectedMachine.Status.NodeRef != nil
	}, timeout).Should(BeTrue())

	randomMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "machine-created-",
			Namespace:    ns.Name,
			Labels: map[string]string{
				clusterv1.MachineControlPlaneLabel: "",
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: testCluster.Name,
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "GenericInfrastructureMachine",
				Name:       "infra-config2",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
					Kind:       "GenericBootstrapConfig",
					Name:       "bootstrap-config-machinereconcile",
				},
			}},
	}

	g.Expect(env.Create(ctx, randomMachine)).To(Succeed())
	defer func() {
		g.Expect(env.Cleanup(ctx, randomMachine)).To(Succeed())
	}()

	// Wait for reconciliation to happen.
	// Since infra and bootstrap objects are ready, a nodeRef will be assigned during node reconciliation.
	key = client.ObjectKey{Name: randomMachine.Name, Namespace: randomMachine.Namespace}
	g.Eventually(func() bool {
		if err := env.Get(ctx, key, randomMachine); err != nil {
			return false
		}
		return randomMachine.Status.NodeRef != nil
	}, timeout).Should(BeTrue())

	// Fake nodes for actual test of nodeToMachine.
	fakeNodes := []*corev1.Node{
		// None annotations.
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: targetNode.GetName(),
			},
			Spec: corev1.NodeSpec{
				ProviderID: targetNode.Spec.ProviderID,
			},
		},
		// ClusterNameAnnotation annotation.
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: targetNode.GetName(),
				Annotations: map[string]string{
					clusterv1.ClusterNameAnnotation: testCluster.GetName(),
				},
			},
			Spec: corev1.NodeSpec{
				ProviderID: targetNode.Spec.ProviderID,
			},
		},
		// ClusterNamespaceAnnotation annotation.
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: targetNode.GetName(),
				Annotations: map[string]string{
					clusterv1.ClusterNamespaceAnnotation: ns.GetName(),
				},
			},
			Spec: corev1.NodeSpec{
				ProviderID: targetNode.Spec.ProviderID,
			},
		},
		// Both annotations.
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: targetNode.GetName(),
				Annotations: map[string]string{
					clusterv1.ClusterNameAnnotation:      testCluster.GetName(),
					clusterv1.ClusterNamespaceAnnotation: ns.GetName(),
				},
			},
			Spec: corev1.NodeSpec{
				ProviderID: targetNode.Spec.ProviderID,
			},
		},
	}

	r := &Reconciler{
		Client:                    env,
		UnstructuredCachingClient: env,
	}
	for _, node := range fakeNodes {
		request := r.nodeToMachine(ctx, node)
		g.Expect(request).To(BeEquivalentTo([]reconcile.Request{
			{
				NamespacedName: client.ObjectKeyFromObject(expectedMachine),
			},
		}))
	}
}

type fakeClientWithNodeDeletionErr struct {
	client.Client
}

func (fc fakeClientWithNodeDeletionErr) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	gvk, err := apiutil.GVKForObject(obj, fakeScheme)
	if err == nil && gvk.Kind == "Node" {
		return fmt.Errorf("fake error")
	}
	return fc.Client.Delete(ctx, obj, opts...)
}

func TestNodeDeletion(t *testing.T) {
	g := NewWithT(t)

	deletionTime := metav1.Now().Add(-1 * time.Second)

	testCluster := clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: corev1.NodeSpec{ProviderID: "test://id-1"},
	}

	testMachine := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.MachineControlPlaneLabel: "",
			},
			Annotations: map[string]string{
				"machine.cluster.x-k8s.io/exclude-node-draining": "",
			},
			Finalizers:        []string{clusterv1.MachineFinalizer},
			DeletionTimestamp: &metav1.Time{Time: deletionTime},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: "test-cluster",
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "GenericInfrastructureMachine",
				Name:       "infra-config1",
			},
			Bootstrap: clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
		},
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Name: "test",
			},
		},
	}

	cpmachine1 := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cp1",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:         "test-cluster",
				clusterv1.MachineControlPlaneLabel: "",
			},
			Finalizers: []string{clusterv1.MachineFinalizer},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName:       "test-cluster",
			InfrastructureRef: corev1.ObjectReference{},
			Bootstrap:         clusterv1.Bootstrap{DataSecretName: pointer.String("data")},
		},
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Name: "cp1",
			},
		},
	}

	testCases := []struct {
		name               string
		deletionTimeout    *metav1.Duration
		resultErr          bool
		clusterDeleted     bool
		expectNodeDeletion bool
		createFakeClient   func(...client.Object) client.Client
	}{
		{
			name:               "should return no error when deletion is successful",
			deletionTimeout:    &metav1.Duration{Duration: time.Second},
			resultErr:          false,
			expectNodeDeletion: true,
			createFakeClient: func(initObjs ...client.Object) client.Client {
				return fake.NewClientBuilder().
					WithObjects(initObjs...).
					WithStatusSubresource(&clusterv1.Machine{}).
					Build()
			},
		},
		{
			name:               "should return an error when timeout is not expired and node deletion fails",
			deletionTimeout:    &metav1.Duration{Duration: time.Hour},
			resultErr:          true,
			expectNodeDeletion: false,
			createFakeClient: func(initObjs ...client.Object) client.Client {
				fc := fake.NewClientBuilder().
					WithObjects(initObjs...).
					WithStatusSubresource(&clusterv1.Machine{}).
					Build()
				return fakeClientWithNodeDeletionErr{fc}
			},
		},
		{
			name:               "should return an error when timeout is infinite and node deletion fails",
			deletionTimeout:    &metav1.Duration{Duration: 0}, // should lead to infinite timeout
			resultErr:          true,
			expectNodeDeletion: false,
			createFakeClient: func(initObjs ...client.Object) client.Client {
				fc := fake.NewClientBuilder().
					WithObjects(initObjs...).
					WithStatusSubresource(&clusterv1.Machine{}).
					Build()
				return fakeClientWithNodeDeletionErr{fc}
			},
		},
		{
			name:               "should not return an error when timeout is expired and node deletion fails",
			deletionTimeout:    &metav1.Duration{Duration: time.Millisecond},
			resultErr:          false,
			expectNodeDeletion: false,
			createFakeClient: func(initObjs ...client.Object) client.Client {
				fc := fake.NewClientBuilder().
					WithObjects(initObjs...).
					WithStatusSubresource(&clusterv1.Machine{}).
					Build()
				return fakeClientWithNodeDeletionErr{fc}
			},
		},
		{
			name:               "should not delete the node or return an error when the cluster is marked for deletion",
			deletionTimeout:    nil, // should lead to infinite timeout
			resultErr:          false,
			clusterDeleted:     true,
			expectNodeDeletion: false,
			createFakeClient: func(initObjs ...client.Object) client.Client {
				fc := fake.NewClientBuilder().
					WithObjects(initObjs...).
					WithStatusSubresource(&clusterv1.Machine{}).
					Build()
				return fakeClientWithNodeDeletionErr{fc}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := testMachine.DeepCopy()
			m.Spec.NodeDeletionTimeout = tc.deletionTimeout

			fakeClient := tc.createFakeClient(node, m, cpmachine1)
			tracker := remote.NewTestClusterCacheTracker(ctrl.Log, fakeClient, fakeScheme, client.ObjectKeyFromObject(&testCluster))

			r := &Reconciler{
				Client:                    fakeClient,
				UnstructuredCachingClient: fakeClient,
				Tracker:                   tracker,
				recorder:                  record.NewFakeRecorder(10),
				nodeDeletionRetryTimeout:  10 * time.Millisecond,
			}

			cluster := testCluster.DeepCopy()
			if tc.clusterDeleted {
				cluster.DeletionTimestamp = &metav1.Time{Time: deletionTime.Add(time.Hour)}
			}

			_, err := r.reconcileDelete(context.Background(), cluster, m)

			if tc.resultErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				if tc.expectNodeDeletion {
					n := &corev1.Node{}
					g.Expect(fakeClient.Get(context.Background(), client.ObjectKeyFromObject(node), n)).NotTo(Succeed())
				}
			}
		})
	}
}

// adds a condition list to an external object.
func addConditionsToExternal(u *unstructured.Unstructured, newConditions clusterv1.Conditions) {
	existingConditions := clusterv1.Conditions{}
	if cs := conditions.UnstructuredGetter(u).GetConditions(); len(cs) != 0 {
		existingConditions = cs
	}
	existingConditions = append(existingConditions, newConditions...)
	conditions.UnstructuredSetter(u).SetConditions(existingConditions)
}

// asserts the conditions set on the Getter object.
// TODO: replace this with util.condition.MatchConditions (or a new matcher in controller runtime komega).
func assertConditions(t *testing.T, from conditions.Getter, conditions ...*clusterv1.Condition) {
	t.Helper()

	for _, condition := range conditions {
		assertCondition(t, from, condition)
	}
}

// asserts whether a condition of type is set on the Getter object
// when the condition is true, asserting the reason/severity/message
// for the condition are avoided.
func assertCondition(t *testing.T, from conditions.Getter, condition *clusterv1.Condition) {
	t.Helper()

	g := NewWithT(t)
	g.Expect(conditions.Has(from, condition.Type)).To(BeTrue())

	if condition.Status == corev1.ConditionTrue {
		conditions.IsTrue(from, condition.Type)
	} else {
		conditionToBeAsserted := conditions.Get(from, condition.Type)
		g.Expect(conditionToBeAsserted.Status).To(Equal(condition.Status))
		g.Expect(conditionToBeAsserted.Severity).To(Equal(condition.Severity))
		g.Expect(conditionToBeAsserted.Reason).To(Equal(condition.Reason))
		if condition.Message != "" {
			g.Expect(conditionToBeAsserted.Message).To(Equal(condition.Message))
		}
	}
}
