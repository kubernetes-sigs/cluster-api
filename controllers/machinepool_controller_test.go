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
	"sigs.k8s.io/cluster-api/util"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

func TestMachinePoolFinalizer(t *testing.T) {
	bootstrapData := "some valid machinepool bootstrap data"
	clusterCorrectMeta := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "valid-cluster",
		},
	}

	machinePoolValidCluster := &clusterv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machinePool1",
			Namespace: "default",
		},
		Spec: clusterv1.MachinePoolSpec{
			Replicas: pointer.Int32Ptr(1),
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						Data: &bootstrapData,
					},
				},
			},
			ClusterName: "valid-cluster",
		},
	}

	machinePoolWithFinalizer := &clusterv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "machinePool2",
			Namespace:  "default",
			Finalizers: []string{"some-other-finalizer"},
		},
		Spec: clusterv1.MachinePoolSpec{
			Replicas: pointer.Int32Ptr(1),
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						Data: &bootstrapData,
					},
				},
			},
			ClusterName: "valid-cluster",
		},
	}

	testCases := []struct {
		name               string
		request            reconcile.Request
		m                  *clusterv1.MachinePool
		expectedFinalizers []string
	}{
		{
			name: "should add a machinePool finalizer to the machinePool if it doesn't have one",
			request: reconcile.Request{
				NamespacedName: util.ObjectKey(machinePoolValidCluster),
			},
			m:                  machinePoolValidCluster,
			expectedFinalizers: []string{clusterv1.MachinePoolFinalizer},
		},
		{
			name: "should append the machinePool finalizer to the machinePool if it already has a finalizer",
			request: reconcile.Request{
				NamespacedName: util.ObjectKey(machinePoolWithFinalizer),
			},
			m:                  machinePoolWithFinalizer,
			expectedFinalizers: []string{"some-other-finalizer", clusterv1.MachinePoolFinalizer},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())

			mr := &MachinePoolReconciler{
				Client: fake.NewFakeClientWithScheme(
					scheme.Scheme,
					clusterCorrectMeta,
					machinePoolValidCluster,
					machinePoolWithFinalizer,
				),
				Log:    log.Log,
				scheme: scheme.Scheme,
			}

			_, _ = mr.Reconcile(tc.request)

			key := client.ObjectKey{Namespace: tc.m.Namespace, Name: tc.m.Name}
			var actual clusterv1.MachinePool
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

func TestMachinePoolOwnerReference(t *testing.T) {
	bootstrapData := "some valid machinepool bootstrap data"
	testCluster := &clusterv1.Cluster{
		TypeMeta:   metav1.TypeMeta{Kind: "Cluster", APIVersion: clusterv1.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-cluster"},
	}

	machinePoolInvalidCluster := &clusterv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machinePool1",
			Namespace: "default",
		},
		Spec: clusterv1.MachinePoolSpec{
			ClusterName: "invalid",
		},
	}

	machinePoolValidCluster := &clusterv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machinePool2",
			Namespace: "default",
		},
		Spec: clusterv1.MachinePoolSpec{
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						Data: &bootstrapData,
					},
				},
			},
			ClusterName: "test-cluster",
		},
	}

	machinePoolValidMachinePool := &clusterv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machinePool3",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.ClusterLabelName: "valid-cluster",
			},
		},
		Spec: clusterv1.MachinePoolSpec{
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						Data: &bootstrapData,
					},
				},
			},
			ClusterName: "test-cluster",
		},
	}

	testCases := []struct {
		name       string
		request    reconcile.Request
		m          *clusterv1.MachinePool
		expectedOR []metav1.OwnerReference
	}{
		{
			name: "should add owner reference to machinePool referencing a cluster with correct type meta",
			request: reconcile.Request{
				NamespacedName: util.ObjectKey(machinePoolValidCluster),
			},
			m: machinePoolValidCluster,
			expectedOR: []metav1.OwnerReference{
				{
					APIVersion: testCluster.APIVersion,
					Kind:       testCluster.Kind,
					Name:       testCluster.Name,
					UID:        testCluster.UID,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())

			mr := &MachinePoolReconciler{
				Client: fake.NewFakeClientWithScheme(
					scheme.Scheme,
					testCluster,
					machinePoolInvalidCluster,
					machinePoolValidCluster,
					machinePoolValidMachinePool,
				),
				Log:    log.Log,
				scheme: scheme.Scheme,
			}

			_, _ = mr.Reconcile(tc.request)

			key := client.ObjectKey{Namespace: tc.m.Namespace, Name: tc.m.Name}
			var actual clusterv1.MachinePool
			if len(tc.expectedOR) > 0 {
				g.Expect(mr.Client.Get(ctx, key, &actual)).To(Succeed())
				g.Expect(actual.OwnerReferences).To(Equal(tc.expectedOR))
			} else {
				g.Expect(actual.OwnerReferences).To(BeEmpty())
			}
		})
	}
}

func TestReconcileMachinePoolRequest(t *testing.T) {
	infraConfig := unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "InfrastructureConfig",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha3",
			"metadata": map[string]interface{}{
				"name":      "infra-config1",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"providerIDList": []interface{}{
					"test://id-1",
				},
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
		TypeMeta:   metav1.TypeMeta{Kind: "Cluster", APIVersion: clusterv1.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-cluster"},
	}

	bootstrapConfig := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "BootstrapConfig",
			"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
			"metadata": map[string]interface{}{
				"name":      "test-bootstrap",
				"namespace": "default",
			},
		},
	}

	type expected struct {
		result reconcile.Result
		err    bool
	}
	testCases := []struct {
		machinePool clusterv1.MachinePool
		expected    expected
	}{
		{
			machinePool: clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "created",
					Namespace:  "default",
					Finalizers: []string{clusterv1.MachinePoolFinalizer, metav1.FinalizerDeleteDependents},
				},
				Spec: clusterv1.MachinePoolSpec{
					ClusterName:    "test-cluster",
					ProviderIDList: []string{"test://id-1"},
					Replicas:       pointer.Int32Ptr(1),
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{

							InfrastructureRef: corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
								Kind:       "InfrastructureConfig",
								Name:       "infra-config1",
							},
							Bootstrap: clusterv1.Bootstrap{Data: pointer.StringPtr("data")},
						},
					},
				},
				Status: clusterv1.MachinePoolStatus{
					Replicas:      1,
					ReadyReplicas: 1,
					NodeRefs: []corev1.ObjectReference{
						{Name: "test"},
					},
				},
			},
			expected: expected{
				result: reconcile.Result{},
				err:    false,
			},
		},
		{
			machinePool: clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "updated",
					Namespace:  "default",
					Finalizers: []string{clusterv1.MachinePoolFinalizer, metav1.FinalizerDeleteDependents},
				},
				Spec: clusterv1.MachinePoolSpec{
					ClusterName:    "test-cluster",
					ProviderIDList: []string{"test://id-1"},
					Replicas:       pointer.Int32Ptr(1),
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							InfrastructureRef: corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
								Kind:       "InfrastructureConfig",
								Name:       "infra-config1",
							},
							Bootstrap: clusterv1.Bootstrap{Data: pointer.StringPtr("data")},
						},
					},
				},
				Status: clusterv1.MachinePoolStatus{
					Replicas:      1,
					ReadyReplicas: 1,
					NodeRefs: []corev1.ObjectReference{
						{Name: "test"},
					},
				},
			},
			expected: expected{
				result: reconcile.Result{},
				err:    false,
			},
		},
		{
			machinePool: clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deleted",
					Namespace: "default",
					Labels: map[string]string{
						clusterv1.MachineControlPlaneLabelName: "",
					},
					Finalizers:        []string{clusterv1.MachinePoolFinalizer, metav1.FinalizerDeleteDependents},
					DeletionTimestamp: &time,
				},
				Spec: clusterv1.MachinePoolSpec{
					ClusterName: "test-cluster",
					Replicas:    pointer.Int32Ptr(1),
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							InfrastructureRef: corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
								Kind:       "InfrastructureConfig",
								Name:       "infra-config1",
							},
							Bootstrap: clusterv1.Bootstrap{Data: pointer.StringPtr("data")},
						},
					},
				},
			},
			expected: expected{
				result: reconcile.Result{},
				err:    false,
			},
		},
	}

	for _, tc := range testCases {
		t.Run("machinePool should be "+tc.machinePool.Name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())

			clientFake := fake.NewFakeClientWithScheme(
				scheme.Scheme,
				&testCluster,
				&tc.machinePool,
				&infraConfig,
				bootstrapConfig,
			)

			r := &MachinePoolReconciler{
				Client: clientFake,
				Log:    log.Log,
				scheme: scheme.Scheme,
			}

			result, err := r.Reconcile(reconcile.Request{NamespacedName: util.ObjectKey(&tc.machinePool)})
			if tc.expected.err {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}

			g.Expect(result).To(Equal(tc.expected.result))
		})
	}
}

func TestReconcileMachinePoolDeleteExternal(t *testing.T) {
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

	infraConfig := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "InfrastructureConfig",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha3",
			"metadata": map[string]interface{}{
				"name":      "delete-infra",
				"namespace": "default",
			},
		},
	}

	machinePool := &clusterv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delete",
			Namespace: "default",
		},
		Spec: clusterv1.MachinePoolSpec{
			ClusterName: "test-cluster",
			Replicas:    pointer.Int32Ptr(1),
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
						Kind:       "InfrastructureConfig",
						Name:       "delete-infra",
					},
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha3",
							Kind:       "BootstrapConfig",
							Name:       "delete-bootstrap",
						},
					},
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
			g := NewWithT(t)

			g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())

			objs := []runtime.Object{testCluster, machinePool}

			if tc.bootstrapExists {
				objs = append(objs, bootstrapConfig)
			}

			if tc.infraExists {
				objs = append(objs, infraConfig)
			}

			r := &MachinePoolReconciler{
				Client: fake.NewFakeClientWithScheme(scheme.Scheme, objs...),
				Log:    log.Log,
				scheme: scheme.Scheme,
			}

			ok, err := r.reconcileDeleteExternal(ctx, machinePool)
			g.Expect(ok).To(Equal(tc.expected))
			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

func TestRemoveMachinePoolFinalizerAfterDeleteReconcile(t *testing.T) {
	g := NewWithT(t)

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())

	dt := metav1.Now()

	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-cluster"},
	}

	m := &clusterv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "delete123",
			Namespace:         "default",
			Finalizers:        []string{clusterv1.MachinePoolFinalizer, metav1.FinalizerDeleteDependents},
			DeletionTimestamp: &dt,
		},
		Spec: clusterv1.MachinePoolSpec{
			ClusterName: "test-cluster",
			Replicas:    pointer.Int32Ptr(1),
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
						Kind:       "InfrastructureConfig",
						Name:       "infra-config1",
					},
					Bootstrap: clusterv1.Bootstrap{Data: pointer.StringPtr("data")},
				},
			},
		},
	}
	key := client.ObjectKey{Namespace: m.Namespace, Name: m.Name}
	mr := &MachinePoolReconciler{
		Client: fake.NewFakeClientWithScheme(scheme.Scheme, testCluster, m),
		Log:    log.Log,
		scheme: scheme.Scheme,
	}
	_, err := mr.Reconcile(reconcile.Request{NamespacedName: key})
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(mr.Client.Get(ctx, key, m)).To(Succeed())
	g.Expect(m.ObjectMeta.Finalizers).To(Equal([]string{metav1.FinalizerDeleteDependents}))
}
