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
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/external"
	externalfake "sigs.k8s.io/cluster-api/controllers/external/fake"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/labels/format"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

const (
	clusterName    = "test-cluster"
	wrongNamespace = "wrong-namespace"
)

func TestReconcileMachinePoolPhases(t *testing.T) {
	deletionTimestamp := metav1.Now()

	var defaultKubeconfigSecret *corev1.Secret
	defaultCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: metav1.NamespaceDefault,
		},
	}

	defaultMachinePool := expv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machinepool-test",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: expv1.MachinePoolSpec{
			ClusterName: defaultCluster.Name,
			Replicas:    ptr.To[int32](1),
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: builder.BootstrapGroupVersion.String(),
							Kind:       builder.TestBootstrapConfigKind,
							Name:       "bootstrap-config1",
							Namespace:  metav1.NamespaceDefault,
						},
					},
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: builder.InfrastructureGroupVersion.String(),
						Kind:       builder.TestInfrastructureMachineTemplateKind,
						Name:       "infra-config1",
						Namespace:  metav1.NamespaceDefault,
					},
				},
			},
		},
	}

	defaultBootstrap := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       builder.TestBootstrapConfigKind,
			"apiVersion": builder.BootstrapGroupVersion.String(),
			"metadata": map[string]interface{}{
				"name":      "bootstrap-config1",
				"namespace": metav1.NamespaceDefault,
			},
			"spec":   map[string]interface{}{},
			"status": map[string]interface{}{},
		},
	}

	defaultInfra := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       builder.TestInfrastructureMachineTemplateKind,
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
			"status": map[string]interface{}{},
		},
	}

	t.Run("Should set OwnerReference and cluster name label on external objects", func(t *testing.T) {
		g := NewWithT(t)

		defaultKubeconfigSecret = kubeconfig.GenerateSecret(defaultCluster, kubeconfig.FromEnvTestConfig(env.Config, defaultCluster))
		machinepool := defaultMachinePool.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		fakeClient := fake.NewClientBuilder().WithObjects(defaultCluster, defaultKubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build()
		r := &MachinePoolReconciler{
			Client:       fakeClient,
			ClusterCache: clustercache.NewFakeClusterCache(fakeClient, client.ObjectKey{Name: defaultCluster.Name, Namespace: defaultCluster.Namespace}),
			externalTracker: external.ObjectTracker{
				Controller:      externalfake.Controller{},
				Cache:           &informertest.FakeInformers{},
				Scheme:          fakeClient.Scheme(),
				PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
			},
		}

		scope := &scope{
			cluster:     defaultCluster,
			machinePool: machinepool,
		}

		res, err := r.reconcile(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(machinepool)

		g.Expect(r.Client.Get(ctx, types.NamespacedName{Name: bootstrapConfig.GetName(), Namespace: bootstrapConfig.GetNamespace()}, bootstrapConfig)).To(Succeed())

		g.Expect(bootstrapConfig.GetOwnerReferences()).To(HaveLen(1))
		g.Expect(bootstrapConfig.GetLabels()[clusterv1.ClusterNameLabel]).To(BeEquivalentTo(clusterName))

		g.Expect(r.Client.Get(ctx, types.NamespacedName{Name: infraConfig.GetName(), Namespace: infraConfig.GetNamespace()}, infraConfig)).To(Succeed())

		g.Expect(infraConfig.GetOwnerReferences()).To(HaveLen(1))
		g.Expect(infraConfig.GetLabels()[clusterv1.ClusterNameLabel]).To(BeEquivalentTo(clusterName))
	})

	t.Run("Should set `Pending` with a new MachinePool", func(t *testing.T) {
		g := NewWithT(t)

		defaultKubeconfigSecret = kubeconfig.GenerateSecret(defaultCluster, kubeconfig.FromEnvTestConfig(env.Config, defaultCluster))
		machinepool := defaultMachinePool.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		fakeClient := fake.NewClientBuilder().WithObjects(defaultCluster, defaultKubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build()

		r := &MachinePoolReconciler{
			Client:       fakeClient,
			ClusterCache: clustercache.NewFakeClusterCache(fakeClient, client.ObjectKey{Name: defaultCluster.Name, Namespace: defaultCluster.Namespace}),
			externalTracker: external.ObjectTracker{
				Controller:      externalfake.Controller{},
				Cache:           &informertest.FakeInformers{},
				Scheme:          fakeClient.Scheme(),
				PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
			},
		}

		scope := &scope{
			cluster:     defaultCluster,
			machinePool: machinepool,
		}

		res, err := r.reconcile(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(machinepool)
		g.Expect(machinepool.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhasePending))
	})

	t.Run("Should set `Provisioning` when bootstrap is ready", func(t *testing.T) {
		g := NewWithT(t)

		defaultKubeconfigSecret = kubeconfig.GenerateSecret(defaultCluster, kubeconfig.FromEnvTestConfig(env.Config, defaultCluster))
		machinepool := defaultMachinePool.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Set bootstrap ready.
		err := unstructured.SetNestedField(bootstrapConfig.Object, true, "status", "ready")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		g.Expect(err).ToNot(HaveOccurred())

		fakeClient := fake.NewClientBuilder().WithObjects(defaultCluster, defaultKubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build()
		r := &MachinePoolReconciler{
			Client:       fakeClient,
			ClusterCache: clustercache.NewFakeClusterCache(fakeClient, client.ObjectKey{Name: defaultCluster.Name, Namespace: defaultCluster.Namespace}),
			externalTracker: external.ObjectTracker{
				Controller:      externalfake.Controller{},
				Cache:           &informertest.FakeInformers{},
				Scheme:          fakeClient.Scheme(),
				PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
			},
		}

		scope := &scope{
			cluster:     defaultCluster,
			machinePool: machinepool,
		}

		res, err := r.reconcile(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(machinepool)
		g.Expect(machinepool.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseProvisioning))
	})

	t.Run("Should set `Running` when bootstrap and infra is ready", func(t *testing.T) {
		g := NewWithT(t)

		defaultKubeconfigSecret = kubeconfig.GenerateSecret(defaultCluster, kubeconfig.FromEnvTestConfig(env.Config, defaultCluster))
		machinepool := defaultMachinePool.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Set bootstrap ready.
		err := unstructured.SetNestedField(bootstrapConfig.Object, true, "status", "ready")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		g.Expect(err).ToNot(HaveOccurred())

		// Set infra ready.
		err = unstructured.SetNestedField(infraConfig.Object, true, "status", "ready")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, int64(1), "status", "replicas")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedStringSlice(infraConfig.Object, []string{"test://machinepool-test-node"}, "spec", "providerIDList")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, "us-east-2a", "spec", "failureDomain")
		g.Expect(err).ToNot(HaveOccurred())

		// Set NodeRef.
		machinepool.Status.NodeRefs = []corev1.ObjectReference{{Kind: "Node", Name: "machinepool-test-node"}}

		fakeClient := fake.NewClientBuilder().WithObjects(defaultCluster, defaultKubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build()
		r := &MachinePoolReconciler{
			Client:       fakeClient,
			ClusterCache: clustercache.NewFakeClusterCache(fakeClient, client.ObjectKey{Name: defaultCluster.Name, Namespace: defaultCluster.Namespace}),
			externalTracker: external.ObjectTracker{
				Controller:      externalfake.Controller{},
				Cache:           &informertest.FakeInformers{},
				Scheme:          fakeClient.Scheme(),
				PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
			},
		}

		scope := &scope{
			cluster:     defaultCluster,
			machinePool: machinepool,
		}

		res, err := r.reconcile(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		// Set ReadyReplicas
		machinepool.Status.ReadyReplicas = 1

		r.reconcilePhase(machinepool)
		g.Expect(machinepool.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseRunning))
	})

	t.Run("Should set `Running` when bootstrap, infra, and ready replicas equals spec replicas", func(t *testing.T) {
		g := NewWithT(t)

		defaultKubeconfigSecret = kubeconfig.GenerateSecret(defaultCluster, kubeconfig.FromEnvTestConfig(env.Config, defaultCluster))
		machinepool := defaultMachinePool.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Set bootstrap ready.
		err := unstructured.SetNestedField(bootstrapConfig.Object, true, "status", "ready")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		g.Expect(err).ToNot(HaveOccurred())

		// Set infra ready.
		err = unstructured.SetNestedStringSlice(infraConfig.Object, []string{"test://id-1"}, "spec", "providerIDList")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, true, "status", "ready")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, int64(1), "status", "replicas")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, []interface{}{
			map[string]interface{}{
				"type":    "InternalIP",
				"address": "10.0.0.1",
			},
			map[string]interface{}{
				"type":    "InternalIP",
				"address": "10.0.0.2",
			},
		}, "addresses")
		g.Expect(err).ToNot(HaveOccurred())

		// Set NodeRef.
		machinepool.Status.NodeRefs = []corev1.ObjectReference{{Kind: "Node", Name: "machinepool-test-node"}}

		fakeClient := fake.NewClientBuilder().WithObjects(defaultCluster, defaultKubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build()
		r := &MachinePoolReconciler{
			Client:       fakeClient,
			ClusterCache: clustercache.NewFakeClusterCache(fakeClient, client.ObjectKey{Name: defaultCluster.Name, Namespace: defaultCluster.Namespace}),
			externalTracker: external.ObjectTracker{
				Controller:      externalfake.Controller{},
				Cache:           &informertest.FakeInformers{},
				Scheme:          fakeClient.Scheme(),
				PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
			},
		}

		scope := &scope{
			cluster:     defaultCluster,
			machinePool: machinepool,
		}

		res, err := r.reconcile(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		// Set ReadyReplicas
		machinepool.Status.ReadyReplicas = 1

		r.reconcilePhase(machinepool)
		g.Expect(machinepool.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseRunning))
	})

	t.Run("Should set `Provisioned` when there is a NodeRef but infra is not ready ", func(t *testing.T) {
		g := NewWithT(t)

		defaultKubeconfigSecret = kubeconfig.GenerateSecret(defaultCluster, kubeconfig.FromEnvTestConfig(env.Config, defaultCluster))
		machinepool := defaultMachinePool.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Set bootstrap ready.
		err := unstructured.SetNestedField(bootstrapConfig.Object, true, "status", "ready")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		g.Expect(err).ToNot(HaveOccurred())

		// Set NodeRef.
		machinepool.Status.NodeRefs = []corev1.ObjectReference{{Kind: "Node", Name: "machinepool-test-node"}}

		fakeClient := fake.NewClientBuilder().WithObjects(defaultCluster, defaultKubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build()
		r := &MachinePoolReconciler{
			Client:       fakeClient,
			ClusterCache: clustercache.NewFakeClusterCache(fakeClient, client.ObjectKey{Name: defaultCluster.Name, Namespace: defaultCluster.Namespace}),
			externalTracker: external.ObjectTracker{
				Controller:      externalfake.Controller{},
				Cache:           &informertest.FakeInformers{},
				Scheme:          fakeClient.Scheme(),
				PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
			},
		}

		scope := &scope{
			cluster:     defaultCluster,
			machinePool: machinepool,
		}

		res, err := r.reconcile(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(machinepool)
		g.Expect(machinepool.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseProvisioned))
	})

	t.Run("Should set `ScalingUp` when infra is scaling up", func(t *testing.T) {
		g := NewWithT(t)

		defaultKubeconfigSecret = kubeconfig.GenerateSecret(defaultCluster, kubeconfig.FromEnvTestConfig(env.Config, defaultCluster))
		machinepool := defaultMachinePool.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Set bootstrap ready.
		err := unstructured.SetNestedField(bootstrapConfig.Object, true, "status", "ready")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		g.Expect(err).ToNot(HaveOccurred())

		// Set infra ready.
		err = unstructured.SetNestedStringSlice(infraConfig.Object, []string{"test://id-1"}, "spec", "providerIDList")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, true, "status", "ready")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, int64(1), "status", "replicas")
		g.Expect(err).ToNot(HaveOccurred())

		// Set NodeRef.
		machinepool.Status.NodeRefs = []corev1.ObjectReference{{Kind: "Node", Name: "machinepool-test-node"}}

		fakeClient := fake.NewClientBuilder().WithObjects(defaultCluster, defaultKubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build()
		r := &MachinePoolReconciler{
			Client:       fakeClient,
			ClusterCache: clustercache.NewFakeClusterCache(fakeClient, client.ObjectKey{Name: defaultCluster.Name, Namespace: defaultCluster.Namespace}),
			externalTracker: external.ObjectTracker{
				Controller:      externalfake.Controller{},
				Cache:           &informertest.FakeInformers{},
				Scheme:          fakeClient.Scheme(),
				PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
			},
		}

		scope := &scope{
			cluster:     defaultCluster,
			machinePool: machinepool,
		}

		res, err := r.reconcile(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		// Set ReadyReplicas
		machinepool.Status.ReadyReplicas = 1

		// Scale up
		machinepool.Spec.Replicas = ptr.To[int32](5)

		r.reconcilePhase(machinepool)
		g.Expect(machinepool.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseScalingUp))
	})

	t.Run("Should set `ScalingDown` when infra is scaling down", func(t *testing.T) {
		g := NewWithT(t)

		defaultKubeconfigSecret = kubeconfig.GenerateSecret(defaultCluster, kubeconfig.FromEnvTestConfig(env.Config, defaultCluster))
		machinepool := defaultMachinePool.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Set bootstrap ready.
		err := unstructured.SetNestedField(bootstrapConfig.Object, true, "status", "ready")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		g.Expect(err).ToNot(HaveOccurred())

		// Set infra ready.
		err = unstructured.SetNestedStringSlice(infraConfig.Object, []string{"test://id-1"}, "spec", "providerIDList")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, true, "status", "ready")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, int64(4), "status", "replicas")
		g.Expect(err).ToNot(HaveOccurred())

		machinepool.Spec.Replicas = ptr.To[int32](4)

		// Set NodeRef.
		machinepool.Status.NodeRefs = []corev1.ObjectReference{
			{Kind: "Node", Name: "machinepool-test-node-0"},
			{Kind: "Node", Name: "machinepool-test-node-1"},
			{Kind: "Node", Name: "machinepool-test-node-2"},
			{Kind: "Node", Name: "machinepool-test-node-3"},
		}

		fakeClient := fake.NewClientBuilder().WithObjects(defaultCluster, defaultKubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build()
		r := &MachinePoolReconciler{
			Client:       fakeClient,
			ClusterCache: clustercache.NewFakeClusterCache(fakeClient, client.ObjectKey{Name: defaultCluster.Name, Namespace: defaultCluster.Namespace}),
			externalTracker: external.ObjectTracker{
				Controller:      externalfake.Controller{},
				Cache:           &informertest.FakeInformers{},
				Scheme:          fakeClient.Scheme(),
				PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
			},
		}

		scope := &scope{
			cluster:     defaultCluster,
			machinePool: machinepool,
		}

		res, err := r.reconcile(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		// Set ReadyReplicas
		machinepool.Status.ReadyReplicas = 4

		// Scale down
		machinepool.Spec.Replicas = ptr.To[int32](1)

		r.reconcilePhase(machinepool)
		g.Expect(machinepool.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseScalingDown))
	})

	t.Run("Should set `Deleting` when MachinePool is being deleted", func(t *testing.T) {
		g := NewWithT(t)

		defaultKubeconfigSecret = kubeconfig.GenerateSecret(defaultCluster, kubeconfig.FromEnvTestConfig(env.Config, defaultCluster))
		machinepool := defaultMachinePool.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Set bootstrap ready.
		err := unstructured.SetNestedField(bootstrapConfig.Object, true, "status", "ready")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		g.Expect(err).ToNot(HaveOccurred())

		// Set infra ready.
		err = unstructured.SetNestedStringSlice(infraConfig.Object, []string{"test://id-1"}, "spec", "providerIDList")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, true, "status", "ready")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, []interface{}{
			map[string]interface{}{
				"type":    "InternalIP",
				"address": "10.0.0.1",
			},
			map[string]interface{}{
				"type":    "InternalIP",
				"address": "10.0.0.2",
			},
		}, "addresses")
		g.Expect(err).ToNot(HaveOccurred())

		// Set NodeRef.
		machinepool.Status.NodeRefs = []corev1.ObjectReference{{Kind: "Node", Name: "machinepool-test-node"}}

		// Set Deletion Timestamp.
		machinepool.SetDeletionTimestamp(&deletionTimestamp)
		machinepool.Finalizers = []string{expv1.MachinePoolFinalizer}

		fakeClient := fake.NewClientBuilder().WithObjects(defaultCluster, defaultKubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build()
		r := &MachinePoolReconciler{
			Client:       fakeClient,
			ClusterCache: clustercache.NewFakeClusterCache(fakeClient, client.ObjectKey{Name: defaultCluster.Name, Namespace: defaultCluster.Namespace}),
			externalTracker: external.ObjectTracker{
				Controller:      externalfake.Controller{},
				Cache:           &informertest.FakeInformers{},
				Scheme:          fakeClient.Scheme(),
				PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
			},
		}

		scope := &scope{
			cluster:     defaultCluster,
			machinePool: machinepool,
		}

		res, err := r.reconcile(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(machinepool)
		g.Expect(machinepool.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseDeleting))
	})

	t.Run("Should keep `Running` when MachinePool bootstrap config is changed to another ready one", func(t *testing.T) {
		g := NewWithT(t)

		defaultKubeconfigSecret = kubeconfig.GenerateSecret(defaultCluster, kubeconfig.FromEnvTestConfig(env.Config, defaultCluster))
		machinePool := defaultMachinePool.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Set bootstrap ready.
		err := unstructured.SetNestedField(bootstrapConfig.Object, true, "status", "ready")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		g.Expect(err).ToNot(HaveOccurred())

		// Set infra ready.
		err = unstructured.SetNestedStringSlice(infraConfig.Object, []string{"test://id-1"}, "spec", "providerIDList")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, true, "status", "ready")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, []interface{}{
			map[string]interface{}{
				"type":    "InternalIP",
				"address": "10.0.0.1",
			},
			map[string]interface{}{
				"type":    "InternalIP",
				"address": "10.0.0.2",
			},
		}, "addresses")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, int64(1), "status", "replicas")
		g.Expect(err).ToNot(HaveOccurred())

		// Set NodeRef.
		machinePool.Status.NodeRefs = []corev1.ObjectReference{{Kind: "Node", Name: "machinepool-test-node"}}

		// Set replicas to fully reconciled
		machinePool.Spec.ProviderIDList = []string{"test://id-1"}
		machinePool.Status.ReadyReplicas = 1
		machinePool.Status.Replicas = 1

		fakeClient := fake.NewClientBuilder().WithObjects(defaultCluster, defaultKubeconfigSecret, machinePool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build()
		r := &MachinePoolReconciler{
			Client:       fakeClient,
			ClusterCache: clustercache.NewFakeClusterCache(fakeClient, client.ObjectKey{Name: defaultCluster.Name, Namespace: defaultCluster.Namespace}),
			externalTracker: external.ObjectTracker{
				Controller:      externalfake.Controller{},
				Cache:           &informertest.FakeInformers{},
				Scheme:          fakeClient.Scheme(),
				PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
			},
		}

		scope := &scope{
			cluster:     defaultCluster,
			machinePool: machinePool,
		}

		res, err := r.reconcile(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(machinePool)
		g.Expect(machinePool.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseRunning))
		g.Expect(*machinePool.Spec.Template.Spec.Bootstrap.DataSecretName).To(Equal("secret-data"))

		// Change bootstrap reference.
		newBootstrapConfig := defaultBootstrap.DeepCopy()
		newBootstrapConfig.SetName("bootstrap-config2")
		err = unstructured.SetNestedField(newBootstrapConfig.Object, true, "status", "ready")
		g.Expect(err).ToNot(HaveOccurred())
		err = unstructured.SetNestedField(newBootstrapConfig.Object, "secret-data-new", "status", "dataSecretName")
		g.Expect(err).ToNot(HaveOccurred())
		err = r.Client.Create(ctx, newBootstrapConfig)
		g.Expect(err).ToNot(HaveOccurred())
		machinePool.Spec.Template.Spec.Bootstrap.ConfigRef.Name = newBootstrapConfig.GetName()

		// Reconcile again. The new bootstrap config should be used.
		res, err = r.reconcile(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(machinePool)
		g.Expect(*machinePool.Spec.Template.Spec.Bootstrap.DataSecretName).To(Equal("secret-data-new"))
		g.Expect(machinePool.Status.BootstrapReady).To(BeTrue())
		g.Expect(machinePool.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseRunning))
	})

	t.Run("Should keep `Running` when MachinePool bootstrap config is changed to a non-ready one", func(t *testing.T) {
		g := NewWithT(t)

		defaultKubeconfigSecret = kubeconfig.GenerateSecret(defaultCluster, kubeconfig.FromEnvTestConfig(env.Config, defaultCluster))
		machinePool := defaultMachinePool.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Set bootstrap ready
		err := unstructured.SetNestedField(bootstrapConfig.Object, true, "status", "ready")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		g.Expect(err).ToNot(HaveOccurred())

		// Set infra ready
		err = unstructured.SetNestedStringSlice(infraConfig.Object, []string{"test://id-1"}, "spec", "providerIDList")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, true, "status", "ready")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, []interface{}{
			map[string]interface{}{
				"type":    "InternalIP",
				"address": "10.0.0.1",
			},
			map[string]interface{}{
				"type":    "InternalIP",
				"address": "10.0.0.2",
			},
		}, "addresses")
		g.Expect(err).ToNot(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, int64(1), "status", "replicas")
		g.Expect(err).ToNot(HaveOccurred())

		// Set NodeRef
		machinePool.Status.NodeRefs = []corev1.ObjectReference{{Kind: "Node", Name: "machinepool-test-node"}}

		// Set replicas to fully reconciled
		machinePool.Spec.ProviderIDList = []string{"test://id-1"}
		machinePool.Status.ReadyReplicas = 1
		machinePool.Status.Replicas = 1

		fakeClient := fake.NewClientBuilder().WithObjects(defaultCluster, defaultKubeconfigSecret, machinePool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build()
		r := &MachinePoolReconciler{
			Client:       fakeClient,
			ClusterCache: clustercache.NewFakeClusterCache(fakeClient, client.ObjectKey{Name: defaultCluster.Name, Namespace: defaultCluster.Namespace}),
			externalTracker: external.ObjectTracker{
				Controller:      externalfake.Controller{},
				Cache:           &informertest.FakeInformers{},
				Scheme:          fakeClient.Scheme(),
				PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
			},
		}

		scope := &scope{
			cluster:     defaultCluster,
			machinePool: machinePool,
		}

		res, err := r.reconcile(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(machinePool)
		g.Expect(machinePool.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseRunning))
		g.Expect(*machinePool.Spec.Template.Spec.Bootstrap.DataSecretName).To(Equal("secret-data"))

		// Change bootstrap reference
		newBootstrapConfig := defaultBootstrap.DeepCopy()
		newBootstrapConfig.SetName("bootstrap-config2")
		err = unstructured.SetNestedField(newBootstrapConfig.Object, false, "status", "ready")
		g.Expect(err).ToNot(HaveOccurred())
		// Fill the `dataSecretName` so we can check if the machine pool uses the non-ready secret immediately or,
		// as it should, not yet
		err = unstructured.SetNestedField(newBootstrapConfig.Object, "secret-data-new", "status", "dataSecretName")
		g.Expect(err).ToNot(HaveOccurred())
		err = r.Client.Create(ctx, newBootstrapConfig)
		g.Expect(err).ToNot(HaveOccurred())
		machinePool.Spec.Template.Spec.Bootstrap.ConfigRef.Name = newBootstrapConfig.GetName()

		// Reconcile again. The new bootstrap config should be used
		res, err = r.reconcile(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())

		// Controller should wait until bootstrap provider reports ready bootstrap config
		g.Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(machinePool)

		// The old secret should still be used, as the new bootstrap config is not marked ready
		g.Expect(*machinePool.Spec.Template.Spec.Bootstrap.DataSecretName).To(Equal("secret-data"))
		g.Expect(machinePool.Status.BootstrapReady).To(BeFalse())

		// There is no phase defined for "changing to new bootstrap config", so it should still be `Running` the
		// old configuration
		g.Expect(machinePool.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseRunning))
	})
}

func TestReconcileMachinePoolBootstrap(t *testing.T) {
	defaultMachinePool := expv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machinepool-test",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: clusterName,
			},
		},
		Spec: expv1.MachinePoolSpec{
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: builder.BootstrapGroupVersion.String(),
							Kind:       builder.TestBootstrapConfigKind,
							Name:       "bootstrap-config1",
							Namespace:  metav1.NamespaceDefault,
						},
					},
				},
			},
		},
	}

	defaultCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: metav1.NamespaceDefault,
		},
	}

	testCases := []struct {
		name            string
		bootstrapConfig map[string]interface{}
		machinepool     *expv1.MachinePool
		expectError     bool
		expectResult    ctrl.Result
		expected        func(g *WithT, m *expv1.MachinePool)
	}{
		{
			name: "new machinepool, bootstrap config ready with data",
			bootstrapConfig: map[string]interface{}{
				"kind":       builder.TestBootstrapConfigKind,
				"apiVersion": builder.BootstrapGroupVersion.String(),
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready":          true,
					"dataSecretName": "secret-data",
				},
			},
			expectError: false,
			expected: func(g *WithT, m *expv1.MachinePool) {
				g.Expect(m.Status.BootstrapReady).To(BeTrue())
				g.Expect(m.Spec.Template.Spec.Bootstrap.DataSecretName).ToNot(BeNil())
				g.Expect(*m.Spec.Template.Spec.Bootstrap.DataSecretName).To(ContainSubstring("secret-data"))
			},
		},
		{
			name: "new machinepool, bootstrap config ready with no data",
			bootstrapConfig: map[string]interface{}{
				"kind":       builder.TestBootstrapConfigKind,
				"apiVersion": builder.BootstrapGroupVersion.String(),
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready": true,
				},
			},
			expectError: true,
			expected: func(g *WithT, m *expv1.MachinePool) {
				g.Expect(m.Status.BootstrapReady).To(BeFalse())
				g.Expect(m.Spec.Template.Spec.Bootstrap.DataSecretName).To(BeNil())
			},
		},
		{
			name: "new machinepool, bootstrap config not ready",
			bootstrapConfig: map[string]interface{}{
				"kind":       builder.TestBootstrapConfigKind,
				"apiVersion": builder.BootstrapGroupVersion.String(),
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec":   map[string]interface{}{},
				"status": map[string]interface{}{},
			},
			expectError:  false,
			expectResult: ctrl.Result{},
			expected: func(g *WithT, m *expv1.MachinePool) {
				g.Expect(m.Status.BootstrapReady).To(BeFalse())
			},
		},
		{
			name: "new machinepool, bootstrap config is not found",
			bootstrapConfig: map[string]interface{}{
				"kind":       builder.TestBootstrapConfigKind,
				"apiVersion": builder.BootstrapGroupVersion.String(),
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": wrongNamespace,
				},
				"spec":   map[string]interface{}{},
				"status": map[string]interface{}{},
			},
			expectError: true,
			expected: func(g *WithT, m *expv1.MachinePool) {
				g.Expect(m.Status.BootstrapReady).To(BeFalse())
			},
		},
		{
			name: "new machinepool, no bootstrap config or data",
			bootstrapConfig: map[string]interface{}{
				"kind":       builder.TestBootstrapConfigKind,
				"apiVersion": builder.BootstrapGroupVersion.String(),
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": wrongNamespace,
				},
				"spec":   map[string]interface{}{},
				"status": map[string]interface{}{},
			},
			expectError: true,
		},
		{
			name: "existing machinepool with config ref, update data secret name",
			bootstrapConfig: map[string]interface{}{
				"kind":       builder.TestBootstrapConfigKind,
				"apiVersion": builder.BootstrapGroupVersion.String(),
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready":          true,
					"dataSecretName": "secret-data",
				},
			},
			machinepool: &expv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: expv1.MachinePoolSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Bootstrap: clusterv1.Bootstrap{
								ConfigRef: &corev1.ObjectReference{
									APIVersion: builder.BootstrapGroupVersion.String(),
									Kind:       builder.TestBootstrapConfigKind,
									Name:       "bootstrap-config1",
									Namespace:  metav1.NamespaceDefault,
								},
								DataSecretName: ptr.To("data"),
							},
						},
					},
				},
				Status: expv1.MachinePoolStatus{
					BootstrapReady: true,
				},
			},
			expectError: false,
			expected: func(g *WithT, m *expv1.MachinePool) {
				g.Expect(m.Status.BootstrapReady).To(BeTrue())
				g.Expect(*m.Spec.Template.Spec.Bootstrap.DataSecretName).To(Equal("secret-data"))
			},
		},
		{
			name: "existing machinepool without config ref, do not update data secret name",
			bootstrapConfig: map[string]interface{}{
				"kind":       builder.TestBootstrapConfigKind,
				"apiVersion": builder.BootstrapGroupVersion.String(),
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready":          true,
					"dataSecretName": "secret-data",
				},
			},
			machinepool: &expv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: expv1.MachinePoolSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Bootstrap: clusterv1.Bootstrap{
								DataSecretName: ptr.To("data"),
							},
						},
					},
				},
				Status: expv1.MachinePoolStatus{
					BootstrapReady: true,
				},
			},
			expectError: false,
			expected: func(g *WithT, m *expv1.MachinePool) {
				g.Expect(m.Status.BootstrapReady).To(BeTrue())
				g.Expect(*m.Spec.Template.Spec.Bootstrap.DataSecretName).To(Equal("data"))
			},
		},
		{
			name: "existing machinepool, bootstrap provider is not ready",
			bootstrapConfig: map[string]interface{}{
				"kind":       builder.TestBootstrapConfigKind,
				"apiVersion": builder.BootstrapGroupVersion.String(),
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready": false,
					"data":  "#!/bin/bash ... data",
				},
			},
			machinepool: &expv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: expv1.MachinePoolSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Bootstrap: clusterv1.Bootstrap{
								ConfigRef: &corev1.ObjectReference{
									APIVersion: builder.BootstrapGroupVersion.String(),
									Kind:       builder.TestBootstrapConfigKind,
									Name:       "bootstrap-config1",
									Namespace:  metav1.NamespaceDefault,
								},
								DataSecretName: ptr.To("data"),
							},
						},
					},
				},
				Status: expv1.MachinePoolStatus{
					BootstrapReady: false,
				},
			},
			expectError:  false,
			expectResult: ctrl.Result{},
			expected: func(g *WithT, m *expv1.MachinePool) {
				g.Expect(m.Status.BootstrapReady).To(BeFalse())
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			if tc.machinepool == nil {
				tc.machinepool = defaultMachinePool.DeepCopy()
			}

			bootstrapConfig := &unstructured.Unstructured{Object: tc.bootstrapConfig}
			fakeClient := fake.NewClientBuilder().WithObjects(tc.machinepool, bootstrapConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build()
			r := &MachinePoolReconciler{
				Client: fakeClient,
				externalTracker: external.ObjectTracker{
					Controller:      externalfake.Controller{},
					Cache:           &informertest.FakeInformers{},
					Scheme:          fakeClient.Scheme(),
					PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
				},
			}

			scope := &scope{
				cluster:     defaultCluster,
				machinePool: tc.machinepool,
			}

			res, err := r.reconcileBootstrap(ctx, scope)
			g.Expect(res).To(BeComparableTo(tc.expectResult))
			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			if tc.expected != nil {
				tc.expected(g, tc.machinepool)
			}
		})
	}
}

func TestReconcileMachinePoolInfrastructure(t *testing.T) {
	defaultMachinePool := expv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machinepool-test",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: clusterName,
			},
		},
		Spec: expv1.MachinePoolSpec{
			Replicas: ptr.To[int32](1),
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: builder.BootstrapGroupVersion.String(),
							Kind:       builder.TestBootstrapConfigKind,
							Name:       "bootstrap-config1",
							Namespace:  metav1.NamespaceDefault,
						},
					},
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: builder.InfrastructureGroupVersion.String(),
						Kind:       builder.TestInfrastructureMachineTemplateKind,
						Name:       "infra-config1",
						Namespace:  metav1.NamespaceDefault,
					},
				},
			},
		},
	}

	defaultCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: metav1.NamespaceDefault,
		},
	}

	testCases := []struct {
		name               string
		bootstrapConfig    map[string]interface{}
		infraConfig        map[string]interface{}
		machinepool        *expv1.MachinePool
		expectError        bool
		expectChanged      bool
		expectRequeueAfter bool
		expected           func(g *WithT, m *expv1.MachinePool)
	}{
		{
			name: "new machinepool, infrastructure config ready",
			infraConfig: map[string]interface{}{
				"kind":       builder.TestInfrastructureMachineTemplateKind,
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
						map[string]interface{}{
							"type":    "InternalIP",
							"address": "10.0.0.2",
						},
					},
				},
			},
			expectError:   false,
			expectChanged: true,
			expected: func(g *WithT, m *expv1.MachinePool) {
				g.Expect(m.Status.InfrastructureReady).To(BeTrue())
			},
		},
		{
			name: "ready bootstrap, infra, and nodeRef, machinepool is running, infra object is deleted, expect failed",
			machinepool: &expv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machinepool-test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: expv1.MachinePoolSpec{
					Replicas: ptr.To[int32](1),
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Bootstrap: clusterv1.Bootstrap{
								ConfigRef: &corev1.ObjectReference{
									APIVersion: builder.BootstrapGroupVersion.String(),
									Kind:       builder.TestBootstrapConfigKind,
									Name:       "bootstrap-config1",
									Namespace:  metav1.NamespaceDefault,
								},
							},
							InfrastructureRef: corev1.ObjectReference{
								APIVersion: builder.InfrastructureGroupVersion.String(),
								Kind:       builder.TestInfrastructureMachineTemplateKind,
								Name:       "infra-config1",
								Namespace:  metav1.NamespaceDefault,
							},
						},
					},
				},
				Status: expv1.MachinePoolStatus{
					BootstrapReady:      true,
					InfrastructureReady: true,
					NodeRefs:            []corev1.ObjectReference{{Kind: "Node", Name: "machinepool-test-node"}},
				},
			},
			bootstrapConfig: map[string]interface{}{
				"kind":       builder.TestBootstrapConfigKind,
				"apiVersion": builder.BootstrapGroupVersion.String(),
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready":          true,
					"dataSecretName": "secret-data",
				},
			},
			infraConfig: map[string]interface{}{
				"kind":       builder.TestInfrastructureMachineTemplateKind,
				"apiVersion": builder.InfrastructureGroupVersion.String(),
				"metadata":   map[string]interface{}{},
			},
			expectError:        true,
			expectRequeueAfter: false,
			expected: func(g *WithT, m *expv1.MachinePool) {
				g.Expect(m.Status.InfrastructureReady).To(BeTrue())
				g.Expect(m.Status.FailureMessage).ToNot(BeNil())
				g.Expect(m.Status.FailureReason).ToNot(BeNil())
				g.Expect(m.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseFailed))
			},
		},
		{
			name: "ready bootstrap, infra, and nodeRef, machinepool is running, replicas 0, providerIDList not set",
			machinepool: &expv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machinepool-test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: expv1.MachinePoolSpec{
					Replicas: ptr.To[int32](0),
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Bootstrap: clusterv1.Bootstrap{
								ConfigRef: &corev1.ObjectReference{
									APIVersion: builder.BootstrapGroupVersion.String(),
									Kind:       builder.TestBootstrapConfigKind,
									Name:       "bootstrap-config1",
									Namespace:  metav1.NamespaceDefault,
								},
							},
							InfrastructureRef: corev1.ObjectReference{
								APIVersion: builder.InfrastructureGroupVersion.String(),
								Kind:       builder.TestInfrastructureMachineTemplateKind,
								Name:       "infra-config1",
								Namespace:  metav1.NamespaceDefault,
							},
						},
					},
				},
				Status: expv1.MachinePoolStatus{
					BootstrapReady:      true,
					InfrastructureReady: true,
					NodeRefs:            []corev1.ObjectReference{{Kind: "Node", Name: "machinepool-test-node"}},
				},
			},
			bootstrapConfig: map[string]interface{}{
				"kind":       builder.TestBootstrapConfigKind,
				"apiVersion": builder.BootstrapGroupVersion.String(),
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready":          true,
					"dataSecretName": "secret-data",
				},
			},
			infraConfig: map[string]interface{}{
				"kind":       builder.TestInfrastructureMachineTemplateKind,
				"apiVersion": builder.InfrastructureGroupVersion.String(),
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{
					"providerIDList": []interface{}{},
				},
				"status": map[string]interface{}{
					"ready": true,
					"addresses": []interface{}{
						map[string]interface{}{
							"type":    "InternalIP",
							"address": "10.0.0.1",
						},
						map[string]interface{}{
							"type":    "InternalIP",
							"address": "10.0.0.2",
						},
					},
				},
			},
			expectError:        false,
			expectRequeueAfter: false,
			expected: func(g *WithT, m *expv1.MachinePool) {
				g.Expect(m.Status.InfrastructureReady).To(BeTrue())
				g.Expect(m.Status.ReadyReplicas).To(Equal(int32(0)))
				g.Expect(m.Status.AvailableReplicas).To(Equal(int32(0)))
				g.Expect(m.Status.UnavailableReplicas).To(Equal(int32(0)))
				g.Expect(m.Status.FailureMessage).To(BeNil())
				g.Expect(m.Status.FailureReason).To(BeNil())
				g.Expect(m.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseRunning))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			if tc.machinepool == nil {
				tc.machinepool = defaultMachinePool.DeepCopy()
			}

			infraConfig := &unstructured.Unstructured{Object: tc.infraConfig}
			fakeClient := fake.NewClientBuilder().WithObjects(tc.machinepool, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build()
			r := &MachinePoolReconciler{
				Client:       fakeClient,
				ClusterCache: clustercache.NewFakeClusterCache(fakeClient, client.ObjectKey{Name: defaultCluster.Name, Namespace: defaultCluster.Namespace}),
				externalTracker: external.ObjectTracker{
					Controller:      externalfake.Controller{},
					Cache:           &informertest.FakeInformers{},
					Scheme:          fakeClient.Scheme(),
					PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
				},
			}

			scope := &scope{
				cluster:     defaultCluster,
				machinePool: tc.machinepool,
			}

			res, err := r.reconcileInfrastructure(ctx, scope)
			if tc.expectRequeueAfter {
				g.Expect(res.RequeueAfter).To(BeNumerically(">=", 0))
			} else {
				g.Expect(res.RequeueAfter).To(Equal(time.Duration(0)))
			}
			r.reconcilePhase(tc.machinepool)
			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			if tc.expected != nil {
				tc.expected(g, tc.machinepool)
			}
		})
	}
}

func TestReconcileMachinePoolMachines(t *testing.T) {
	t.Run("Reconcile MachinePool Machines", func(t *testing.T) {
		g := NewWithT(t)

		ns, err := env.CreateNamespace(ctx, "test-machinepool-machines")
		g.Expect(err).ToNot(HaveOccurred())

		cluster := builder.Cluster(ns.Name, clusterName).Build()
		g.Expect(env.CreateAndWait(ctx, cluster)).To(Succeed())

		t.Run("Should do nothing if machines already exist", func(*testing.T) {
			machinePool := getMachinePool(2, "machinepool-test-1", clusterName, ns.Name)
			g.Expect(env.CreateAndWait(ctx, &machinePool)).To(Succeed())

			infraMachines := getInfraMachines(2, machinePool.Name, clusterName, ns.Name)
			for i := range infraMachines {
				g.Expect(env.CreateAndWait(ctx, &infraMachines[i])).To(Succeed())
			}

			machines := getMachines(2, machinePool.Name, clusterName, ns.Name)
			for i := range machines {
				g.Expect(env.CreateAndWait(ctx, &machines[i])).To(Succeed())
			}

			infraConfig := map[string]interface{}{
				"kind":       builder.GenericInfrastructureMachinePoolKind,
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": ns.Name,
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
						map[string]interface{}{
							"type":    "InternalIP",
							"address": "10.0.0.2",
						},
					},
					"infrastructureMachineKind": builder.GenericInfrastructureMachineKind,
				},
			}
			g.Expect(env.CreateAndWait(ctx, &unstructured.Unstructured{Object: infraConfig})).To(Succeed())

			r := &MachinePoolReconciler{
				Client:   env,
				ssaCache: ssa.NewCache(),
				externalTracker: external.ObjectTracker{
					Controller:      externalfake.Controller{},
					Cache:           &informertest.FakeInformers{},
					Scheme:          env.Scheme(),
					PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
				},
			}
			scope := &scope{
				machinePool: &machinePool,
			}
			err = r.reconcileMachines(ctx, scope, &unstructured.Unstructured{Object: infraConfig})
			r.reconcilePhase(&machinePool)
			g.Expect(err).ToNot(HaveOccurred())

			machineList := &clusterv1.MachineList{}
			labels := map[string]string{
				clusterv1.ClusterNameLabel:     clusterName,
				clusterv1.MachinePoolNameLabel: machinePool.Name,
			}
			g.Expect(env.GetAPIReader().List(ctx, machineList, client.InNamespace(cluster.Namespace), client.MatchingLabels(labels))).To(Succeed())
			g.Expect(machineList.Items).To(HaveLen(2))
			for i := range machineList.Items {
				machine := &machineList.Items[i]
				_, err := external.Get(ctx, r.Client, &machine.Spec.InfrastructureRef)
				g.Expect(err).ToNot(HaveOccurred())
			}
		})

		t.Run("Should create two machines if two infra machines exist", func(*testing.T) {
			machinePool := getMachinePool(2, "machinepool-test-2", clusterName, ns.Name)
			g.Expect(env.CreateAndWait(ctx, &machinePool)).To(Succeed())

			infraMachines := getInfraMachines(2, machinePool.Name, clusterName, ns.Name)
			for i := range infraMachines {
				g.Expect(env.CreateAndWait(ctx, &infraMachines[i])).To(Succeed())
			}

			infraConfig := map[string]interface{}{
				"kind":       builder.GenericInfrastructureMachinePoolKind,
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "infra-config2",
					"namespace": ns.Name,
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
						map[string]interface{}{
							"type":    "InternalIP",
							"address": "10.0.0.2",
						},
					},
					"infrastructureMachineKind": builder.GenericInfrastructureMachineKind,
				},
			}
			g.Expect(env.CreateAndWait(ctx, &unstructured.Unstructured{Object: infraConfig})).To(Succeed())

			r := &MachinePoolReconciler{
				Client:   env,
				ssaCache: ssa.NewCache(),
				externalTracker: external.ObjectTracker{
					Controller:      externalfake.Controller{},
					Cache:           &informertest.FakeInformers{},
					Scheme:          env.Scheme(),
					PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
				},
			}

			scope := &scope{
				machinePool: &machinePool,
			}

			err = r.reconcileMachines(ctx, scope, &unstructured.Unstructured{Object: infraConfig})
			r.reconcilePhase(&machinePool)
			g.Expect(err).ToNot(HaveOccurred())

			machineList := &clusterv1.MachineList{}
			labels := map[string]string{
				clusterv1.ClusterNameLabel:     clusterName,
				clusterv1.MachinePoolNameLabel: machinePool.Name,
			}
			g.Expect(env.GetAPIReader().List(ctx, machineList, client.InNamespace(cluster.Namespace), client.MatchingLabels(labels))).To(Succeed())
			g.Expect(machineList.Items).To(HaveLen(2))
			for i := range machineList.Items {
				machine := &machineList.Items[i]
				_, err := external.Get(ctx, r.Client, &machine.Spec.InfrastructureRef)
				g.Expect(err).ToNot(HaveOccurred())
			}
		})

		t.Run("Should do nothing if machinepool does not support machinepool machines", func(*testing.T) {
			machinePool := getMachinePool(2, "machinepool-test-3", clusterName, ns.Name)
			g.Expect(env.Create(ctx, &machinePool)).To(Succeed())

			infraConfig := map[string]interface{}{
				"kind":       builder.GenericInfrastructureMachinePoolKind,
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "infra-config3",
					"namespace": ns.Name,
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
						map[string]interface{}{
							"type":    "InternalIP",
							"address": "10.0.0.2",
						},
					},
				},
			}
			g.Expect(env.CreateAndWait(ctx, &unstructured.Unstructured{Object: infraConfig})).To(Succeed())

			r := &MachinePoolReconciler{
				Client:   env,
				ssaCache: ssa.NewCache(),
			}

			scope := &scope{
				machinePool: &machinePool,
			}

			err = r.reconcileMachines(ctx, scope, &unstructured.Unstructured{Object: infraConfig})
			r.reconcilePhase(&machinePool)
			g.Expect(err).ToNot(HaveOccurred())

			machineList := &clusterv1.MachineList{}
			labels := map[string]string{
				clusterv1.ClusterNameLabel:     clusterName,
				clusterv1.MachinePoolNameLabel: machinePool.Name,
			}
			g.Expect(env.GetAPIReader().List(ctx, machineList, client.InNamespace(cluster.Namespace), client.MatchingLabels(labels))).To(Succeed())
			g.Expect(machineList.Items).To(BeEmpty())
		})
	})
}

func TestInfraMachineToMachinePoolMapper(t *testing.T) {
	machinePool1 := expv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machinepool-1",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: clusterName,
			},
		},
	}

	machinePool2 := expv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machinepool-2",
			Namespace: "other-namespace",
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: clusterName,
			},
		},
	}

	machinePool3 := expv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machinepool-3",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "other-cluster",
			},
		},
	}

	machinePoolLongName := expv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machinepool-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-long", // Use a name longer than 64 characters to trigger a hash
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "other-cluster",
			},
		},
	}

	infraMachine1 := unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "InfrastructureMachine",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"name":      "infra-machine1",
				"namespace": metav1.NamespaceDefault,
				"labels": map[string]interface{}{
					clusterv1.ClusterNameLabel:     clusterName,
					clusterv1.MachinePoolNameLabel: format.MustFormatValue(machinePool1.Name),
				},
			},
		},
	}

	infraMachine2 := unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "InfrastructureMachine",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"name":      "infra-machine2",
				"namespace": metav1.NamespaceDefault,
				"labels": map[string]interface{}{
					clusterv1.ClusterNameLabel:     "other-cluster",
					clusterv1.MachinePoolNameLabel: format.MustFormatValue(machinePoolLongName.Name),
				},
			},
		},
	}

	infraMachine3 := unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "InfrastructureMachine",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"name":      "infra-machine3",
				"namespace": metav1.NamespaceDefault,
				"labels": map[string]interface{}{
					clusterv1.ClusterNameLabel:     "other-cluster",
					clusterv1.MachinePoolNameLabel: format.MustFormatValue("missing-machinepool"),
				},
			},
		},
	}

	testCases := []struct {
		name                string
		infraMachine        *unstructured.Unstructured
		machinepools        []expv1.MachinePool
		expectedMachinePool *expv1.MachinePool
	}{
		{
			name:         "match machinePool name with label value",
			infraMachine: &infraMachine1,
			machinepools: []expv1.MachinePool{
				machinePool1,
				machinePool2,
				machinePool3,
				machinePoolLongName,
			},
			expectedMachinePool: &machinePool1,
		},
		{
			name:         "match hash of machinePool name with label hash",
			infraMachine: &infraMachine2,
			machinepools: []expv1.MachinePool{
				machinePool1,
				machinePool2,
				machinePool3,
				machinePoolLongName,
			},
			expectedMachinePool: &machinePoolLongName,
		},
		{
			name:         "return nil if no machinePool matches",
			infraMachine: &infraMachine3,
			machinepools: []expv1.MachinePool{
				machinePool1,
				machinePool2,
				machinePool3,
				machinePoolLongName,
			},
			expectedMachinePool: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			objs := []client.Object{tc.infraMachine.DeepCopy()}

			for _, mp := range tc.machinepools {
				objs = append(objs, mp.DeepCopy())
			}

			r := &MachinePoolReconciler{
				Client: fake.NewClientBuilder().WithObjects(objs...).Build(),
			}

			result := r.infraMachineToMachinePoolMapper(ctx, tc.infraMachine)
			if tc.expectedMachinePool == nil {
				g.Expect(result).To(BeNil())
			} else {
				g.Expect(result).To(HaveLen(1))
				g.Expect(result[0].Name).To(Equal(tc.expectedMachinePool.Name))
				g.Expect(result[0].Namespace).To(Equal(tc.expectedMachinePool.Namespace))
			}
		})
	}
}

func TestReconcileMachinePoolScaleToFromZero(t *testing.T) {
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "machinepool-scale-zero")
	g.Expect(err).ToNot(HaveOccurred())

	// Set up cluster to test against.
	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "machinepool-scale-zero-",
			Namespace:    ns.Name,
		},
	}
	g.Expect(env.CreateAndWait(ctx, testCluster)).To(Succeed())
	g.Expect(env.CreateKubeconfigSecret(ctx, testCluster)).To(Succeed())
	defer func(do ...client.Object) {
		g.Expect(env.CleanupAndWait(ctx, do...)).To(Succeed())
	}(testCluster)

	defaultMachinePool := expv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machinepool-test",
			Namespace: ns.Name,
		},
		Spec: expv1.MachinePoolSpec{
			ClusterName: testCluster.Name,
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: builder.BootstrapGroupVersion.String(),
							Kind:       builder.TestBootstrapConfigKind,
							Name:       "bootstrap-config1",
							Namespace:  ns.Name,
						},
					},
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: builder.InfrastructureGroupVersion.String(),
						Kind:       builder.TestInfrastructureMachineTemplateKind,
						Name:       "infra-config1",
						Namespace:  ns.Name,
					},
				},
			},
			MinReadySeconds: ptr.To[int32](0),
		},
		Status: expv1.MachinePoolStatus{},
	}

	defaultBootstrap := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       builder.TestBootstrapConfigKind,
			"apiVersion": builder.BootstrapGroupVersion.String(),
			"metadata": map[string]interface{}{
				"name":      "bootstrap-config1",
				"namespace": ns.Name,
			},
			"spec": map[string]interface{}{},
			"status": map[string]interface{}{
				"ready":          true,
				"dataSecretName": "secret-data",
			},
		},
	}

	defaultInfra := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       builder.TestInfrastructureMachineTemplateKind,
			"apiVersion": builder.InfrastructureGroupVersion.String(),
			"metadata": map[string]interface{}{
				"name":      "infra-config1",
				"namespace": ns.Name,
			},
			"spec": map[string]interface{}{},
			"status": map[string]interface{}{
				"ready": true,
			},
		},
	}

	t.Run("Should set `ScalingDown` when scaling to zero", func(t *testing.T) {
		g := NewWithT(t)

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "machinepool-test-node",
			},
			Spec: corev1.NodeSpec{
				ProviderID: "test://machinepool-test-node",
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		}
		g.Expect(env.CreateAndWait(ctx, node)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.CleanupAndWait(ctx, do...)).To(Succeed())
		}(node)

		kubeconfigSecret := kubeconfig.GenerateSecret(testCluster, kubeconfig.FromEnvTestConfig(env.Config, testCluster))
		machinepool := defaultMachinePool.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Setup prerequisites - a running MachinePool with one instance and user sets Replicas to 0

		// set replicas to 0
		machinepool.Spec.Replicas = ptr.To[int32](0)

		// set nodeRefs to one instance
		machinepool.Status.NodeRefs = []corev1.ObjectReference{{Kind: "Node", Name: "machinepool-test-node"}}

		// set infra providerIDList
		err = unstructured.SetNestedStringSlice(infraConfig.Object, []string{"test://machinepool-test-node"}, "spec", "providerIDList")
		g.Expect(err).ToNot(HaveOccurred())

		// set infra replicas
		err = unstructured.SetNestedField(infraConfig.Object, int64(1), "status", "replicas")
		g.Expect(err).ToNot(HaveOccurred())

		fakeClient := fake.NewClientBuilder().WithObjects(testCluster, kubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build()
		r := &MachinePoolReconciler{
			Client:       fakeClient,
			ClusterCache: clustercache.NewFakeClusterCache(env.GetClient(), client.ObjectKey{Name: testCluster.Name, Namespace: testCluster.Namespace}),
			recorder:     record.NewFakeRecorder(32),
			externalTracker: external.ObjectTracker{
				Controller:      externalfake.Controller{},
				Cache:           &informertest.FakeInformers{},
				Scheme:          fakeClient.Scheme(),
				PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
			},
		}

		scope := &scope{
			cluster:     testCluster,
			machinePool: machinepool,
		}

		res, err := r.reconcile(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(machinepool)

		g.Expect(machinepool.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseScalingDown))

		delNode := &corev1.Node{}
		g.Expect(env.Get(ctx, client.ObjectKeyFromObject(node), delNode)).To(Succeed())
	})

	t.Run("Should delete retired nodes when scaled to zero", func(t *testing.T) {
		g := NewWithT(t)

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "machinepool-test-node",
			},
			Spec: corev1.NodeSpec{
				ProviderID: "test://machinepool-test-node",
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		}
		g.Expect(env.CreateAndWait(ctx, node)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.CleanupAndWait(ctx, do...)).To(Succeed())
		}(node)

		kubeconfigSecret := kubeconfig.GenerateSecret(testCluster, kubeconfig.FromEnvTestConfig(env.Config, testCluster))
		machinepool := defaultMachinePool.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Setup prerequisites - a running MachinePool with one instance and user sets Replicas to 0

		// set replicas to 0
		machinepool.Spec.Replicas = ptr.To[int32](0)

		// set nodeRefs to one instance
		machinepool.Status.NodeRefs = []corev1.ObjectReference{{Kind: "Node", Name: "machinepool-test-node"}}

		// set infra replicas
		err = unstructured.SetNestedField(infraConfig.Object, int64(0), "status", "replicas")
		g.Expect(err).ToNot(HaveOccurred())

		fakeClient := fake.NewClientBuilder().WithObjects(testCluster, kubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build()
		r := &MachinePoolReconciler{
			Client:       fakeClient,
			ClusterCache: clustercache.NewFakeClusterCache(env.GetClient(), client.ObjectKey{Name: testCluster.Name, Namespace: testCluster.Namespace}),
			recorder:     record.NewFakeRecorder(32),
			externalTracker: external.ObjectTracker{
				Controller:      externalfake.Controller{},
				Cache:           &informertest.FakeInformers{},
				Scheme:          fakeClient.Scheme(),
				PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
			},
		}

		scope := &scope{
			cluster:     testCluster,
			machinePool: machinepool,
		}

		res, err := r.reconcile(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(machinepool)
		g.Expect(machinepool.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseRunning))

		delNode := &corev1.Node{}
		err = env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(node), delNode)
		g.Expect(err).To(HaveOccurred())
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	t.Run("Should set `Running` when scaled to zero", func(t *testing.T) {
		g := NewWithT(t)

		kubeconfigSecret := kubeconfig.GenerateSecret(testCluster, kubeconfig.FromEnvTestConfig(env.Config, testCluster))
		machinepool := defaultMachinePool.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Setup prerequisites - a running MachinePool with no instances and replicas set to 0

		// set replicas to 0
		machinepool.Spec.Replicas = ptr.To[int32](0)

		// set nodeRefs to no instance
		machinepool.Status.NodeRefs = []corev1.ObjectReference{}

		// set infra replicas
		err := unstructured.SetNestedField(infraConfig.Object, int64(0), "status", "replicas")
		g.Expect(err).ToNot(HaveOccurred())

		fakeClient := fake.NewClientBuilder().WithObjects(testCluster, kubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build()
		r := &MachinePoolReconciler{
			Client:       fakeClient,
			recorder:     record.NewFakeRecorder(32),
			ClusterCache: clustercache.NewFakeClusterCache(fakeClient, client.ObjectKey{Name: testCluster.Name, Namespace: testCluster.Namespace}),
			externalTracker: external.ObjectTracker{
				Controller:      externalfake.Controller{},
				Cache:           &informertest.FakeInformers{},
				Scheme:          fakeClient.Scheme(),
				PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
			},
		}

		scope := &scope{
			cluster:     testCluster,
			machinePool: machinepool,
		}

		res, err := r.reconcile(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(machinepool)

		g.Expect(machinepool.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseRunning))
	})

	t.Run("Should set `ScalingUp` when scaling from zero to one", func(t *testing.T) {
		g := NewWithT(t)

		kubeconfigSecret := kubeconfig.GenerateSecret(testCluster, kubeconfig.FromEnvTestConfig(env.Config, testCluster))
		machinepool := defaultMachinePool.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Setup prerequisites - a running MachinePool with no instances and replicas set to 1

		// set replicas to 1
		machinepool.Spec.Replicas = ptr.To[int32](1)

		// set nodeRefs to no instance
		machinepool.Status.NodeRefs = []corev1.ObjectReference{}

		// set infra replicas
		err := unstructured.SetNestedField(infraConfig.Object, int64(0), "status", "replicas")
		g.Expect(err).ToNot(HaveOccurred())

		fakeClient := fake.NewClientBuilder().WithObjects(testCluster, kubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build()
		r := &MachinePoolReconciler{
			Client:       fakeClient,
			recorder:     record.NewFakeRecorder(32),
			ClusterCache: clustercache.NewFakeClusterCache(fakeClient, client.ObjectKey{Name: testCluster.Name, Namespace: testCluster.Namespace}),
			externalTracker: external.ObjectTracker{
				Controller:      externalfake.Controller{},
				Cache:           &informertest.FakeInformers{},
				Scheme:          fakeClient.Scheme(),
				PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
			},
		}

		scope := &scope{
			cluster:     testCluster,
			machinePool: machinepool,
		}

		res, err := r.reconcile(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(machinepool)

		g.Expect(machinepool.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseScalingUp))
	})

	t.Run("Should set `Running` when scaled from zero to one", func(t *testing.T) {
		g := NewWithT(t)

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "machinepool-test-node",
			},
			Spec: corev1.NodeSpec{
				ProviderID: "test://machinepool-test-node",
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		}
		g.Expect(env.CreateAndWait(ctx, node)).To(Succeed())
		defer func(do ...client.Object) {
			g.Expect(env.CleanupAndWait(ctx, do...)).To(Succeed())
		}(node)

		kubeconfigSecret := kubeconfig.GenerateSecret(testCluster, kubeconfig.FromEnvTestConfig(env.Config, testCluster))
		machinepool := defaultMachinePool.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Setup prerequisites - a running MachinePool with no refs but providerIDList and replicas set to 1

		// set replicas to 1
		machinepool.Spec.Replicas = ptr.To[int32](1)

		// set nodeRefs to no instance
		machinepool.Status.NodeRefs = []corev1.ObjectReference{}

		// set infra providerIDList
		err = unstructured.SetNestedStringSlice(infraConfig.Object, []string{"test://machinepool-test-node"}, "spec", "providerIDList")
		g.Expect(err).ToNot(HaveOccurred())

		// set infra replicas
		err = unstructured.SetNestedField(infraConfig.Object, int64(1), "status", "replicas")
		g.Expect(err).ToNot(HaveOccurred())

		fakeClient := fake.NewClientBuilder().WithObjects(testCluster, kubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build()
		r := &MachinePoolReconciler{
			Client:       fakeClient,
			ClusterCache: clustercache.NewFakeClusterCache(env.GetClient(), client.ObjectKey{Name: testCluster.Name, Namespace: testCluster.Namespace}),
			recorder:     record.NewFakeRecorder(32),
			externalTracker: external.ObjectTracker{
				Controller:      externalfake.Controller{},
				Cache:           &informertest.FakeInformers{},
				Scheme:          fakeClient.Scheme(),
				PredicateLogger: ptr.To(logr.New(log.NullLogSink{})),
			},
		}

		scope := &scope{
			cluster:     testCluster,
			machinePool: machinepool,
		}

		res, err := r.reconcile(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(machinepool)

		g.Expect(machinepool.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseRunning))

		delNode := &corev1.Node{}
		g.Expect(env.Get(ctx, client.ObjectKeyFromObject(node), delNode)).To(Succeed())
	})
}

func getInfraMachines(replicas int, mpName, clusterName, nsName string) []unstructured.Unstructured {
	infraMachines := make([]unstructured.Unstructured, replicas)
	for i := range replicas {
		infraMachines[i] = unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       builder.GenericInfrastructureMachineKind,
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      fmt.Sprintf("%s-infra-%d", mpName, i),
					"namespace": nsName,
					"labels": map[string]interface{}{
						clusterv1.ClusterNameLabel:     clusterName,
						clusterv1.MachinePoolNameLabel: mpName,
					},
				},
			},
		}
	}
	return infraMachines
}

func getMachines(replicas int, mpName, clusterName, nsName string) []clusterv1.Machine {
	machines := make([]clusterv1.Machine, replicas)
	for i := range replicas {
		machines[i] = clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-machine-%d", mpName, i),
				Namespace: nsName,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel:     clusterName,
					clusterv1.MachinePoolNameLabel: mpName,
				},
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: clusterName,
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &corev1.ObjectReference{
						APIVersion: builder.BootstrapGroupVersion.String(),
						Kind:       builder.GenericBootstrapConfigKind,
						Name:       fmt.Sprintf("bootstrap-config-%d", i),
					},
				},
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: builder.InfrastructureGroupVersion.String(),
					Kind:       builder.GenericInfrastructureMachineKind,
					Name:       fmt.Sprintf("%s-infra-%d", mpName, i),
				},
			},
		}
	}
	return machines
}

func getMachinePool(replicas int, mpName, clusterName, nsName string) expv1.MachinePool {
	return expv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mpName,
			Namespace: nsName,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: clusterName,
			},
		},
		Spec: expv1.MachinePoolSpec{
			ClusterName: clusterName,
			Replicas:    ptr.To[int32](int32(replicas)),
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					ClusterName: clusterName,
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: builder.BootstrapGroupVersion.String(),
							Kind:       builder.GenericBootstrapConfigKind,
							Name:       "bootstrap-config1",
							Namespace:  nsName,
						},
					},
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: builder.InfrastructureGroupVersion.String(),
						Kind:       builder.GenericInfrastructureMachineKind,
						Name:       "infra-config1",
						Namespace:  nsName,
					},
				},
			},
		},
	}
}

func TestMachinePoolReconciler_getNodeRefMap(t *testing.T) {
	testCases := []struct {
		name     string
		nodeList []client.Object
		expected map[string]*corev1.Node
		err      error
	}{
		{
			name: "all valid provider ids",
			nodeList: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
					},
					Spec: corev1.NodeSpec{
						ProviderID: "aws://us-east-1/id-node-1",
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gce-node-2",
					},
					Spec: corev1.NodeSpec{
						ProviderID: "gce://us-central1/gce-id-node-2",
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "azure-node-4",
					},
					Spec: corev1.NodeSpec{
						ProviderID: "azure://westus2/id-node-4",
					},
				},
			},
			expected: map[string]*corev1.Node{
				"aws://us-east-1/id-node-1": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
					},
					Spec: corev1.NodeSpec{
						ProviderID: "aws://us-east-1/id-node-1",
					},
				},
				"gce://us-central1/gce-id-node-2": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "gce-node-2",
					},
					Spec: corev1.NodeSpec{
						ProviderID: "gce://us-central1/gce-id-node-2",
					},
				},
				"azure://westus2/id-node-4": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "azure-node-4",
					},
					Spec: corev1.NodeSpec{
						ProviderID: "azure://westus2/id-node-4",
					},
				},
			},
		},
		{
			name: "missing provider id",
			nodeList: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
					},
					Spec: corev1.NodeSpec{
						ProviderID: "aws://us-east-1/id-node-1",
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gce-node-2",
					},
					Spec: corev1.NodeSpec{
						ProviderID: "gce://us-central1/gce-id-node-2",
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "azure-node-4",
					},
				},
			},
			expected: map[string]*corev1.Node{
				"aws://us-east-1/id-node-1": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "node-1",
					},
					Spec: corev1.NodeSpec{
						ProviderID: "aws://us-east-1/id-node-1",
					},
				},
				"gce://us-central1/gce-id-node-2": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "gce-node-2",
					},
					Spec: corev1.NodeSpec{
						ProviderID: "gce://us-central1/gce-id-node-2",
					},
				},
			},
		},
		{
			name:     "empty node list",
			nodeList: []client.Object{},
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			r := &MachinePoolReconciler{
				Client:   fake.NewClientBuilder().Build(),
				recorder: record.NewFakeRecorder(32),
			}
			client := fake.NewClientBuilder().WithObjects(tt.nodeList...).Build()
			result, err := r.getNodeRefMap(ctx, client)
			if tt.err == nil {
				g.Expect(err).ToNot(HaveOccurred())
			} else {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(Equal(tt.err), "Expected error %v, got %v", tt.err, err)
			}
			g.Expect(result).To(HaveLen(len(tt.expected)), "Expected NodeRef count to be %v, got %v", len(result), len(tt.expected))
			for providerID, node := range result {
				g.Expect(node).ToNot(BeNil())
				g.Expect(node.Spec).Should(Equal(tt.expected[providerID].Spec))
				g.Expect(node.GetObjectMeta().GetName()).Should(Equal(tt.expected[providerID].GetObjectMeta().GetName()))
			}
		})
	}
}
