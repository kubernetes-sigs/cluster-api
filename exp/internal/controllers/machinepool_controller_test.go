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
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/external"
	externalfake "sigs.k8s.io/cluster-api/controllers/external/fake"
	"sigs.k8s.io/cluster-api/util"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestMachinePoolFinalizer(t *testing.T) {
	bootstrapData := "some valid machinepool bootstrap data"
	clusterCorrectMeta := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceDefault,
			Name:      "valid-cluster",
		},
	}

	machinePoolValidCluster := &clusterv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machinePool1",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachinePoolSpec{
			Replicas: ptr.To[int32](1),
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						DataSecretName: &bootstrapData,
					},
				},
			},
			ClusterName: "valid-cluster",
		},
	}

	machinePoolWithFinalizer := &clusterv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "machinePool2",
			Namespace:  metav1.NamespaceDefault,
			Finalizers: []string{"some-other-finalizer"},
		},
		Spec: clusterv1.MachinePoolSpec{
			Replicas: ptr.To[int32](1),
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						DataSecretName: &bootstrapData,
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

			mr := &MachinePoolReconciler{
				Client: fake.NewClientBuilder().WithObjects(
					clusterCorrectMeta,
					machinePoolValidCluster,
					machinePoolWithFinalizer,
				).Build(),
			}

			_, _ = mr.Reconcile(ctx, tc.request)

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
		ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "test-cluster"},
	}

	machinePoolInvalidCluster := &clusterv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machinePool1",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachinePoolSpec{
			Replicas:    ptr.To[int32](1),
			ClusterName: "invalid",
		},
		Status: clusterv1.MachinePoolStatus{
			Conditions: []metav1.Condition{{
				Type:   clusterv1.PausedCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.NotPausedReason,
			}},
		},
	}

	machinePoolValidCluster := &clusterv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machinePool2",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachinePoolSpec{
			Replicas: ptr.To[int32](1),
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						DataSecretName: &bootstrapData,
					},
				},
			},
			ClusterName: "test-cluster",
		},
		Status: clusterv1.MachinePoolStatus{
			Conditions: []metav1.Condition{{
				Type:   clusterv1.PausedCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.NotPausedReason,
			}},
		},
	}

	machinePoolValidMachinePool := &clusterv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machinePool3",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "valid-cluster",
			},
		},
		Spec: clusterv1.MachinePoolSpec{
			Replicas: ptr.To[int32](1),
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						DataSecretName: &bootstrapData,
					},
				},
			},
			ClusterName: "test-cluster",
		},
		Status: clusterv1.MachinePoolStatus{
			Conditions: []metav1.Condition{{
				Type:   clusterv1.PausedCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.NotPausedReason,
			}},
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

			fakeClient := fake.NewClientBuilder().WithObjects(
				testCluster,
				machinePoolInvalidCluster,
				machinePoolValidCluster,
				machinePoolValidMachinePool,
			).WithStatusSubresource(&clusterv1.MachinePool{}).Build()
			mr := &MachinePoolReconciler{
				Client:    fakeClient,
				APIReader: fakeClient,
			}

			key := client.ObjectKey{Namespace: tc.m.Namespace, Name: tc.m.Name}
			var actual clusterv1.MachinePool

			// this first requeue is to add finalizer
			result, err := mr.Reconcile(ctx, tc.request)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(result).To(BeComparableTo(ctrl.Result{}))
			g.Expect(mr.Client.Get(ctx, key, &actual)).To(Succeed())
			g.Expect(actual.Finalizers).To(ContainElement(clusterv1.MachinePoolFinalizer))

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

func TestReconcileMachinePoolRequest(t *testing.T) {
	infraMachinePool := unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       builder.TestInfrastructureMachinePoolKind,
			"apiVersion": builder.InfrastructureGroupVersion.String(),
			"metadata": map[string]interface{}{
				"name":      "infra-config1",
				"namespace": metav1.NamespaceDefault,
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

	testCluster := clusterv1.Cluster{
		TypeMeta:   metav1.TypeMeta{Kind: "Cluster", APIVersion: clusterv1.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "test-cluster"},
	}

	bootstrapConfig := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       builder.TestBootstrapConfigKind,
			"apiVersion": builder.BootstrapGroupVersion.String(),
			"metadata": map[string]interface{}{
				"name":      "test-bootstrap",
				"namespace": metav1.NamespaceDefault,
			},
		},
	}

	timeNow := metav1.Now()
	type expected struct {
		mpExist bool
		result  reconcile.Result
		err     bool
	}
	testCases := []struct {
		name            string
		machinePool     clusterv1.MachinePool
		nodes           []corev1.Node
		errOnDeleteNode bool
		expected        expected
	}{
		{
			name: "Successfully reconcile MachinePool",
			machinePool: clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "created",
					Namespace:         metav1.NamespaceDefault,
					Finalizers:        []string{clusterv1.MachinePoolFinalizer},
					CreationTimestamp: timeNow,
				},
				Spec: clusterv1.MachinePoolSpec{
					ClusterName:    "test-cluster",
					ProviderIDList: []string{"test://id-1"},
					Replicas:       ptr.To[int32](1),
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{
								APIGroup: builder.InfrastructureGroupVersion.Group,
								Kind:     builder.TestInfrastructureMachinePoolKind,
								Name:     "infra-config1",
							},
							Bootstrap: clusterv1.Bootstrap{DataSecretName: ptr.To("data")},
						},
					},
				},
				Status: clusterv1.MachinePoolStatus{
					Replicas: ptr.To(int32(1)),
					Deprecated: &clusterv1.MachinePoolDeprecatedStatus{
						V1Beta1: &clusterv1.MachinePoolV1Beta1DeprecatedStatus{
							ReadyReplicas: 1,
						},
					},
					NodeRefs: []corev1.ObjectReference{
						{Name: "test"},
					},
					ObservedGeneration: 1,
					Conditions: []metav1.Condition{{
						Type:   clusterv1.PausedCondition,
						Status: metav1.ConditionFalse,
						Reason: clusterv1.NotPausedReason,
					}},
				},
			},
			expected: expected{
				mpExist: true,
				result:  reconcile.Result{},
				err:     false,
			},
		},
		{
			name: "Successfully reconcile MachinePool with deletionTimestamp & NodeDeletionTimeoutSeconds not passed when Nodes can be deleted (MP should go away)",
			machinePool: clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deleted",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.MachineControlPlaneLabel: "",
					},
					Finalizers:        []string{clusterv1.MachinePoolFinalizer},
					CreationTimestamp: timeNow,
					DeletionTimestamp: &timeNow,
				},
				Spec: clusterv1.MachinePoolSpec{
					ClusterName: "test-cluster",
					Replicas:    ptr.To[int32](1),
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{
								APIGroup: builder.InfrastructureGroupVersion.Group,
								Kind:     builder.TestInfrastructureMachinePoolKind,
								Name:     "infra-config1-already-deleted", // Use an InfrastructureMachinePool that doesn't exist, so reconcileDelete doesn't get stuck on deletion
							},
							Bootstrap: clusterv1.Bootstrap{DataSecretName: ptr.To("data")},
							Deletion: clusterv1.MachineDeletionSpec{
								NodeDeletionTimeoutSeconds: ptr.To(int32(10 * 60)),
							},
						},
					},
					ProviderIDList: []string{"aws:///us-test-2a/i-013ab00756982217f"},
				},
				Status: clusterv1.MachinePoolStatus{
					NodeRefs: []corev1.ObjectReference{
						{
							APIVersion: "v1",
							Kind:       "Node",
							Name:       "test-node",
						},
					},
					Conditions: []metav1.Condition{{
						Type:   clusterv1.PausedCondition,
						Status: metav1.ConditionFalse,
						Reason: clusterv1.NotPausedReason,
					}},
				},
			},
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
					},
					Spec: corev1.NodeSpec{
						// This providerID is not in the list above, so the reconciler will try (and fail) to delete it
						ProviderID: "aws:///us-test-2c/i-013ab00756982217f",
					},
				},
			},
			errOnDeleteNode: false, // Node can be deleted
			expected: expected{
				mpExist: false,
				result:  reconcile.Result{},
				err:     false,
			},
		},
		{
			name: "Fail reconcile MachinePool with deletionTimestamp & NodeDeletionTimeoutSeconds not passed when Nodes cannot be deleted (MP should stay around)",
			machinePool: clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deleted",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.MachineControlPlaneLabel: "",
					},
					Finalizers:        []string{clusterv1.MachinePoolFinalizer},
					CreationTimestamp: timeNow,
					DeletionTimestamp: &timeNow,
				},
				Spec: clusterv1.MachinePoolSpec{
					ClusterName: "test-cluster",
					Replicas:    ptr.To[int32](1),
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{
								APIGroup: builder.InfrastructureGroupVersion.Group,
								Kind:     builder.TestInfrastructureMachinePoolKind,
								Name:     "infra-config1-already-deleted", // Use an InfrastructureMachinePool that doesn't exist, so reconcileDelete doesn't get stuck on deletion
							},
							Bootstrap: clusterv1.Bootstrap{DataSecretName: ptr.To("data")},
							Deletion: clusterv1.MachineDeletionSpec{
								NodeDeletionTimeoutSeconds: ptr.To(int32(10 * 60)),
							},
						},
					},
					ProviderIDList: []string{"aws:///us-test-2a/i-013ab00756982217f"},
				},
				Status: clusterv1.MachinePoolStatus{
					NodeRefs: []corev1.ObjectReference{
						{
							APIVersion: "v1",
							Kind:       "Node",
							Name:       "test-node",
						},
					},
					Conditions: []metav1.Condition{{
						Type:   clusterv1.PausedCondition,
						Status: metav1.ConditionFalse,
						Reason: clusterv1.NotPausedReason,
					}},
				},
			},
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
					},
					Spec: corev1.NodeSpec{
						// This providerID is not in the list above, so the reconciler will try (and fail) to delete it
						ProviderID: "aws:///us-test-2c/i-013ab00756982217f",
					},
				},
			},
			errOnDeleteNode: true, // Node cannot be deleted
			expected: expected{
				mpExist: true,
				result:  reconcile.Result{},
				err:     true,
			},
		},
		{
			name: "Successfully reconcile MachinePool with deletionTimestamp & NodeDeletionTimeoutSeconds passed when Nodes cannot be deleted (MP should go away)",
			machinePool: clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deleted",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.MachineControlPlaneLabel: "",
					},
					Finalizers:        []string{clusterv1.MachinePoolFinalizer},
					CreationTimestamp: metav1.Time{Time: timeNow.Add(time.Minute * -2)},
					DeletionTimestamp: &metav1.Time{Time: timeNow.Add(time.Minute * -1)},
				},
				Spec: clusterv1.MachinePoolSpec{
					ClusterName: "test-cluster",
					Replicas:    ptr.To[int32](1),
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{
								APIGroup: builder.InfrastructureGroupVersion.Group,
								Kind:     builder.TestInfrastructureMachinePoolKind,
								Name:     "infra-config1-already-deleted", // Use an InfrastructureMachinePool that doesn't exist, so reconcileDelete doesn't get stuck on deletion
							},
							Bootstrap: clusterv1.Bootstrap{DataSecretName: ptr.To("data")},
							Deletion: clusterv1.MachineDeletionSpec{
								NodeDeletionTimeoutSeconds: ptr.To(int32(10)), // timeout passed
							},
						},
					},
					ProviderIDList: []string{"aws:///us-test-2a/i-013ab00756982217f"},
				},
				Status: clusterv1.MachinePoolStatus{
					NodeRefs: []corev1.ObjectReference{
						{
							APIVersion: "v1",
							Kind:       "Node",
							Name:       "test-node",
						},
					},
					Conditions: []metav1.Condition{{
						Type:   clusterv1.PausedCondition,
						Status: metav1.ConditionFalse,
						Reason: clusterv1.NotPausedReason,
					}},
				},
			},
			nodes: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
					},
					Spec: corev1.NodeSpec{
						// This providerID is not in the list above, so the reconciler will try (and fail) to delete it
						ProviderID: "aws:///us-test-2c/i-013ab00756982217f",
					},
				},
			},
			errOnDeleteNode: true, // Node cannot be deleted
			expected: expected{
				mpExist: false,
				result:  reconcile.Result{},
				err:     false,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			clientFake := fake.NewClientBuilder().WithObjects(
				&testCluster,
				&tc.machinePool,
				&infraMachinePool,
				bootstrapConfig,
				builder.TestBootstrapConfigCRD,
				builder.TestInfrastructureMachinePoolCRD,
			).WithStatusSubresource(&clusterv1.MachinePool{}).Build()

			trackerObjects := []client.Object{}
			for _, node := range tc.nodes {
				trackerObjects = append(trackerObjects, &node)
			}
			trackerClientFake := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
				Delete: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.DeleteOption) error {
					if tc.errOnDeleteNode {
						return fmt.Errorf("node deletion failed")
					}
					return nil
				},
			}).WithObjects(trackerObjects...).Build()

			r := &MachinePoolReconciler{
				Client:       clientFake,
				APIReader:    clientFake,
				ClusterCache: clustercache.NewFakeClusterCache(trackerClientFake, client.ObjectKey{Name: testCluster.Name, Namespace: testCluster.Namespace}),
				externalTracker: external.ObjectTracker{
					Controller:      externalfake.Controller{},
					Cache:           &informertest.FakeInformers{},
					Scheme:          clientFake.Scheme(),
					PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
				},
			}

			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: util.ObjectKey(&tc.machinePool)})
			if tc.expected.err {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(result).To(BeComparableTo(tc.expected.result))

			// Check machinePool test cases
			key := client.ObjectKey{Namespace: tc.machinePool.Namespace, Name: tc.machinePool.Name}
			if tc.expected.mpExist {
				g.Expect(r.Client.Get(ctx, key, &clusterv1.MachinePool{})).To(Succeed())
			} else {
				g.Expect(apierrors.IsNotFound(r.Client.Get(ctx, key, &clusterv1.MachinePool{}))).To(BeTrue())
			}
		})
	}
}

func TestMachinePoolNodeDeleteTimeoutPassed(t *testing.T) {
	timeNow := metav1.Now()
	testCases := []struct {
		name        string
		machinePool *clusterv1.MachinePool
		want        bool
	}{
		{
			name: "false if deletionTimestamp not set",
			machinePool: &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machinepool",
					Namespace: metav1.NamespaceDefault,
				},
			},
			want: false,
		},
		{
			name: "false if deletionTimestamp set to now and NodeDeletionTimeoutSeconds not set",
			machinePool: &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "machinepool",
					Namespace:         metav1.NamespaceDefault,
					DeletionTimestamp: &timeNow,
				},
			},
			want: false,
		},
		{
			name: "false if deletionTimestamp set to now and NodeDeletionTimeoutSeconds set to 0",
			machinePool: &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "machinepool",
					Namespace:         metav1.NamespaceDefault,
					DeletionTimestamp: &timeNow,
				},
				Spec: clusterv1.MachinePoolSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Deletion: clusterv1.MachineDeletionSpec{
								NodeDeletionTimeoutSeconds: ptr.To(int32(0)),
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "false if deletionTimestamp set to now and NodeDeletionTimeoutSeconds set to 1m",
			machinePool: &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "machinepool",
					Namespace:         metav1.NamespaceDefault,
					DeletionTimestamp: &timeNow,
				},
				Spec: clusterv1.MachinePoolSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Deletion: clusterv1.MachineDeletionSpec{
								NodeDeletionTimeoutSeconds: ptr.To(int32(60)),
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "true if deletionTimestamp set to now-1m and NodeDeletionTimeoutSeconds set to 10s",
			machinePool: &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "machinepool",
					Namespace:         metav1.NamespaceDefault,
					DeletionTimestamp: &metav1.Time{Time: timeNow.Add(time.Minute * -1)},
				},
				Spec: clusterv1.MachinePoolSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Deletion: clusterv1.MachineDeletionSpec{
								NodeDeletionTimeoutSeconds: ptr.To(int32(10)),
							},
						},
					},
				},
			},
			want: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			timeoutPassed := (&MachinePoolReconciler{}).isMachinePoolNodeDeleteTimeoutPassed(tc.machinePool)
			g.Expect(timeoutPassed).To(Equal(tc.want))
		})
	}
}

func TestReconcileMachinePoolDeleteExternal(t *testing.T) {
	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "test-cluster"},
	}

	bootstrapConfig := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       builder.TestBootstrapConfigKind,
			"apiVersion": builder.BootstrapGroupVersion.String(),
			"metadata": map[string]interface{}{
				"name":      "delete-bootstrap",
				"namespace": metav1.NamespaceDefault,
			},
		},
	}

	infraConfig := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       builder.TestInfrastructureMachinePoolKind,
			"apiVersion": builder.InfrastructureGroupVersion.String(),
			"metadata": map[string]interface{}{
				"name":      "delete-infra",
				"namespace": metav1.NamespaceDefault,
			},
		},
	}

	machinePool := &clusterv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delete",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachinePoolSpec{
			ClusterName: "test-cluster",
			Replicas:    ptr.To[int32](1),
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: builder.InfrastructureGroupVersion.Group,
						Kind:     builder.TestInfrastructureMachinePoolKind,
						Name:     "delete-infra",
					},
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: builder.BootstrapGroupVersion.Group,
							Kind:     builder.TestBootstrapConfigKind,
							Name:     "delete-bootstrap",
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
			objs := []client.Object{testCluster, machinePool, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachinePoolCRD}

			if tc.bootstrapExists {
				objs = append(objs, bootstrapConfig)
			}

			if tc.infraExists {
				objs = append(objs, infraConfig)
			}

			r := &MachinePoolReconciler{
				Client: fake.NewClientBuilder().WithObjects(objs...).Build(),
			}

			ok, err := r.reconcileDeleteExternal(ctx, machinePool)
			g.Expect(ok).To(Equal(tc.expected))
			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestRemoveMachinePoolFinalizerAfterDeleteReconcile(t *testing.T) {
	g := NewWithT(t)

	dt := metav1.Now()

	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "test-cluster"},
	}

	m := &clusterv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "delete123",
			Namespace:         metav1.NamespaceDefault,
			Finalizers:        []string{clusterv1.MachinePoolFinalizer, "test"},
			DeletionTimestamp: &dt,
		},
		Spec: clusterv1.MachinePoolSpec{
			ClusterName: "test-cluster",
			Replicas:    ptr.To[int32](1),
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: builder.InfrastructureGroupVersion.Group,
						Kind:     builder.TestInfrastructureMachinePoolKind,
						Name:     "infra-config1",
					},
					Bootstrap: clusterv1.Bootstrap{DataSecretName: ptr.To("data")},
				},
			},
		},
		Status: clusterv1.MachinePoolStatus{
			Conditions: []metav1.Condition{{
				Type:   clusterv1.PausedCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.NotPausedReason,
			}},
		},
	}
	key := client.ObjectKey{Namespace: m.Namespace, Name: m.Name}
	clientFake := fake.NewClientBuilder().WithObjects(testCluster, m, builder.TestInfrastructureMachinePoolCRD).WithStatusSubresource(&clusterv1.MachinePool{}).Build()
	mr := &MachinePoolReconciler{
		Client:       clientFake,
		ClusterCache: clustercache.NewFakeClusterCache(clientFake, client.ObjectKey{Name: testCluster.Name, Namespace: testCluster.Namespace}),
	}
	_, err := mr.Reconcile(ctx, reconcile.Request{NamespacedName: key})
	g.Expect(err).ToNot(HaveOccurred())

	var actual clusterv1.MachinePool
	g.Expect(mr.Client.Get(ctx, key, &actual)).To(Succeed())
	g.Expect(actual.ObjectMeta.Finalizers).To(Equal([]string{"test"}))
}

func TestMachinePoolConditions(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "test-cluster"},
	}

	bootstrapConfig := func(dataSecretCreated bool) *unstructured.Unstructured {
		return &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       builder.TestBootstrapConfigKind,
				"apiVersion": builder.BootstrapGroupVersion.String(),
				"metadata": map[string]interface{}{
					"name":      "bootstrap1",
					"namespace": metav1.NamespaceDefault,
				},
				"status": map[string]interface{}{
					"initialization": map[string]interface{}{
						"dataSecretCreated": dataSecretCreated,
					},
					"dataSecretName": "data",
				},
			},
		}
	}

	infraConfig := func(ready bool) *unstructured.Unstructured {
		return &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       builder.TestInfrastructureMachinePoolKind,
				"apiVersion": builder.InfrastructureGroupVersion.String(),
				"metadata": map[string]interface{}{
					"name":      "infra1",
					"namespace": metav1.NamespaceDefault,
				},
				"status": map[string]interface{}{
					"ready": ready,
				},
				"spec": map[string]interface{}{
					"providerIDList": []interface{}{
						"azure://westus2/id-node-4",
						"aws://us-east-1/id-node-1",
					},
				},
			},
		}
	}

	machinePool := &clusterv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "blah",
			Namespace:  metav1.NamespaceDefault,
			Finalizers: []string{clusterv1.MachinePoolFinalizer},
		},
		Spec: clusterv1.MachinePoolSpec{
			ClusterName: "test-cluster",
			Replicas:    ptr.To[int32](2),
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: builder.InfrastructureGroupVersion.Group,
						Kind:     builder.TestInfrastructureMachinePoolKind,
						Name:     "infra1",
					},
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: builder.BootstrapGroupVersion.Group,
							Kind:     builder.TestBootstrapConfigKind,
							Name:     "bootstrap1",
						},
					},
				},
			},
		},
		Status: clusterv1.MachinePoolStatus{
			Conditions: []metav1.Condition{{
				Type:   clusterv1.PausedCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.NotPausedReason,
			}},
		},
	}

	nodeList := corev1.NodeList{
		Items: []corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
				},
				Spec: corev1.NodeSpec{
					ProviderID: "aws://us-east-1/id-node-1",
				},
				Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady}}},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "azure-node-4",
				},
				Spec: corev1.NodeSpec{
					ProviderID: "azure://westus2/id-node-4",
				},
				Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady}}},
			},
		},
	}

	testcases := []struct {
		name                string
		dataSecretCreated   bool
		infrastructureReady bool
		expectError         bool
		beforeFunc          func(bootstrap, infra *unstructured.Unstructured, mp *clusterv1.MachinePool, nodeList *corev1.NodeList)
		conditionAssertFunc func(t *testing.T, getter v1beta1conditions.Getter)
	}{
		{
			name:                "all conditions true",
			dataSecretCreated:   true,
			infrastructureReady: true,
			beforeFunc: func(_, _ *unstructured.Unstructured, mp *clusterv1.MachinePool, _ *corev1.NodeList) {
				mp.Spec.ProviderIDList = []string{"azure://westus2/id-node-4", "aws://us-east-1/id-node-1"}
				mp.Status.NodeRefs = []corev1.ObjectReference{
					{Name: "node-1"},
					{Name: "azure-node-4"},
				}
				mp.Status.Replicas = ptr.To(int32(2))
				if mp.Status.Deprecated == nil {
					mp.Status.Deprecated = &clusterv1.MachinePoolDeprecatedStatus{}
				}
				if mp.Status.Deprecated.V1Beta1 == nil {
					mp.Status.Deprecated.V1Beta1 = &clusterv1.MachinePoolV1Beta1DeprecatedStatus{}
				}
				mp.Status.Deprecated.V1Beta1.ReadyReplicas = 2
			},
			conditionAssertFunc: func(t *testing.T, getter v1beta1conditions.Getter) {
				t.Helper()
				g := NewWithT(t)

				g.Expect(getter.GetV1Beta1Conditions()).NotTo(BeEmpty())
				for _, c := range getter.GetV1Beta1Conditions() {
					g.Expect(c.Status).To(Equal(corev1.ConditionTrue))
				}
			},
		},
		{
			name:                "boostrap not ready",
			dataSecretCreated:   false,
			infrastructureReady: true,
			beforeFunc: func(bootstrap, _ *unstructured.Unstructured, _ *clusterv1.MachinePool, _ *corev1.NodeList) {
				addConditionsToExternal(bootstrap, clusterv1.Conditions{
					{
						Type:     clusterv1.ReadyV1Beta1Condition,
						Status:   corev1.ConditionFalse,
						Severity: clusterv1.ConditionSeverityInfo,
						Reason:   "Custom reason",
					},
				})
			},
			conditionAssertFunc: func(t *testing.T, getter v1beta1conditions.Getter) {
				t.Helper()
				g := NewWithT(t)

				g.Expect(v1beta1conditions.Has(getter, clusterv1.BootstrapReadyV1Beta1Condition)).To(BeTrue())
				infraReadyCondition := v1beta1conditions.Get(getter, clusterv1.BootstrapReadyV1Beta1Condition)
				g.Expect(infraReadyCondition.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(infraReadyCondition.Reason).To(Equal("Custom reason"))
			},
		},
		{
			name:                "bootstrap not ready with fallback condition",
			dataSecretCreated:   false,
			infrastructureReady: true,
			conditionAssertFunc: func(t *testing.T, getter v1beta1conditions.Getter) {
				t.Helper()
				g := NewWithT(t)

				g.Expect(v1beta1conditions.Has(getter, clusterv1.BootstrapReadyV1Beta1Condition)).To(BeTrue())
				bootstrapReadyCondition := v1beta1conditions.Get(getter, clusterv1.BootstrapReadyV1Beta1Condition)
				g.Expect(bootstrapReadyCondition.Status).To(Equal(corev1.ConditionFalse))

				g.Expect(v1beta1conditions.Has(getter, clusterv1.ReadyV1Beta1Condition)).To(BeTrue())
				readyCondition := v1beta1conditions.Get(getter, clusterv1.ReadyV1Beta1Condition)
				g.Expect(readyCondition.Status).To(Equal(corev1.ConditionFalse))
			},
		},
		{
			name:                "infrastructure not ready",
			dataSecretCreated:   true,
			infrastructureReady: false,
			beforeFunc: func(_, infra *unstructured.Unstructured, _ *clusterv1.MachinePool, _ *corev1.NodeList) {
				addConditionsToExternal(infra, clusterv1.Conditions{
					{
						Type:     clusterv1.ReadyV1Beta1Condition,
						Status:   corev1.ConditionFalse,
						Severity: clusterv1.ConditionSeverityInfo,
						Reason:   "Custom reason",
					},
				})
			},
			conditionAssertFunc: func(t *testing.T, getter v1beta1conditions.Getter) {
				t.Helper()

				g := NewWithT(t)

				g.Expect(v1beta1conditions.Has(getter, clusterv1.InfrastructureReadyV1Beta1Condition)).To(BeTrue())
				infraReadyCondition := v1beta1conditions.Get(getter, clusterv1.InfrastructureReadyV1Beta1Condition)
				g.Expect(infraReadyCondition.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(infraReadyCondition.Reason).To(Equal("Custom reason"))
			},
		},
		{
			name:                "infrastructure not ready with fallback condition",
			dataSecretCreated:   true,
			infrastructureReady: false,
			conditionAssertFunc: func(t *testing.T, getter v1beta1conditions.Getter) {
				t.Helper()
				g := NewWithT(t)

				g.Expect(v1beta1conditions.Has(getter, clusterv1.InfrastructureReadyV1Beta1Condition)).To(BeTrue())
				infraReadyCondition := v1beta1conditions.Get(getter, clusterv1.InfrastructureReadyV1Beta1Condition)
				g.Expect(infraReadyCondition.Status).To(Equal(corev1.ConditionFalse))

				g.Expect(v1beta1conditions.Has(getter, clusterv1.ReadyV1Beta1Condition)).To(BeTrue())
				readyCondition := v1beta1conditions.Get(getter, clusterv1.ReadyV1Beta1Condition)
				g.Expect(readyCondition.Status).To(Equal(corev1.ConditionFalse))
			},
		},
		{
			name:              "incorrect infrastructure reference",
			dataSecretCreated: true,
			expectError:       true,
			beforeFunc: func(_, _ *unstructured.Unstructured, mp *clusterv1.MachinePool, _ *corev1.NodeList) {
				mp.Spec.Template.Spec.InfrastructureRef = clusterv1.ContractVersionedObjectReference{
					APIGroup: builder.InfrastructureGroupVersion.Group,
					Kind:     builder.TestInfrastructureMachinePoolKind,
					Name:     "does-not-exist",
				}
			},
			conditionAssertFunc: func(t *testing.T, getter v1beta1conditions.Getter) {
				t.Helper()
				g := NewWithT(t)

				g.Expect(v1beta1conditions.Has(getter, clusterv1.InfrastructureReadyV1Beta1Condition)).To(BeTrue())
				infraReadyCondition := v1beta1conditions.Get(getter, clusterv1.InfrastructureReadyV1Beta1Condition)
				g.Expect(infraReadyCondition.Status).To(Equal(corev1.ConditionFalse))
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// setup objects
			bootstrap := bootstrapConfig(tt.dataSecretCreated)
			infra := infraConfig(tt.infrastructureReady)
			mp := machinePool.DeepCopy()
			nodes := nodeList.DeepCopy()
			if tt.beforeFunc != nil {
				tt.beforeFunc(bootstrap, infra, mp, nodes)
			}

			clientFake := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
				testCluster,
				mp,
				infra,
				bootstrap,
				&nodes.Items[0],
				&nodes.Items[1],
				builder.TestBootstrapConfigCRD,
				builder.TestInfrastructureMachinePoolCRD,
			).WithStatusSubresource(&clusterv1.MachinePool{}).Build()

			r := &MachinePoolReconciler{
				Client:       clientFake,
				APIReader:    clientFake,
				ClusterCache: clustercache.NewFakeClusterCache(clientFake, client.ObjectKey{Name: testCluster.Name, Namespace: testCluster.Namespace}),
				externalTracker: external.ObjectTracker{
					Controller:      externalfake.Controller{},
					Cache:           &informertest.FakeInformers{},
					Scheme:          clientFake.Scheme(),
					PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
				},
			}

			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: util.ObjectKey(machinePool)})
			if !tt.expectError {
				g.Expect(err).ToNot(HaveOccurred())
			}

			m := &clusterv1.MachinePool{}
			machinePoolKey := client.ObjectKeyFromObject(machinePool)
			g.Expect(r.Client.Get(ctx, machinePoolKey, m)).ToNot(HaveOccurred())

			tt.conditionAssertFunc(t, m)
		})
	}
}

// adds a condition list to an external object.
func addConditionsToExternal(u *unstructured.Unstructured, newConditions clusterv1.Conditions) {
	existingConditions := clusterv1.Conditions{}
	if cs := v1beta1conditions.UnstructuredGetter(u).GetV1Beta1Conditions(); len(cs) != 0 {
		existingConditions = cs
	}
	existingConditions = append(existingConditions, newConditions...)
	v1beta1conditions.UnstructuredSetter(u).SetV1Beta1Conditions(existingConditions)
}
