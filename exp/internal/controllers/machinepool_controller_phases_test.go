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
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/test/builder"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
)

const (
	timeout        = time.Second * 30
	clusterName    = "test-cluster"
	wrongNamespace = "wrong-namespace"
)

func init() {
	externalReadyWait = 1 * time.Second
}

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
			Replicas:    pointer.Int32(1),
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: builder.BootstrapGroupVersion.String(),
							Kind:       builder.TestBootstrapConfigKind,
							Name:       "bootstrap-config1",
						},
					},
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: builder.InfrastructureGroupVersion.String(),
						Kind:       builder.TestInfrastructureMachineTemplateKind,
						Name:       "infra-config1",
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

		r := &MachinePoolReconciler{
			Client: fake.NewClientBuilder().WithObjects(defaultCluster, defaultKubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build(),
		}

		res, err := r.reconcile(ctx, defaultCluster, machinepool)
		g.Expect(err).NotTo(HaveOccurred())
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

		r := &MachinePoolReconciler{
			Client: fake.NewClientBuilder().WithObjects(defaultCluster, defaultKubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build(),
		}

		res, err := r.reconcile(ctx, defaultCluster, machinepool)
		g.Expect(err).NotTo(HaveOccurred())
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
		g.Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		g.Expect(err).NotTo(HaveOccurred())

		r := &MachinePoolReconciler{
			Client: fake.NewClientBuilder().WithObjects(defaultCluster, defaultKubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build(),
		}

		res, err := r.reconcile(ctx, defaultCluster, machinepool)
		g.Expect(err).NotTo(HaveOccurred())
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
		g.Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		g.Expect(err).NotTo(HaveOccurred())

		// Set infra ready.
		err = unstructured.SetNestedField(infraConfig.Object, true, "status", "ready")
		g.Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, int64(1), "status", "replicas")
		g.Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedStringSlice(infraConfig.Object, []string{"test://machinepool-test-node"}, "spec", "providerIDList")
		g.Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, "us-east-2a", "spec", "failureDomain")
		g.Expect(err).NotTo(HaveOccurred())

		// Set NodeRef.
		machinepool.Status.NodeRefs = []corev1.ObjectReference{{Kind: "Node", Name: "machinepool-test-node"}}

		r := &MachinePoolReconciler{
			Client: fake.NewClientBuilder().WithObjects(defaultCluster, defaultKubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build(),
		}

		res, err := r.reconcile(ctx, defaultCluster, machinepool)
		g.Expect(err).NotTo(HaveOccurred())
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
		g.Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		g.Expect(err).NotTo(HaveOccurred())

		// Set infra ready.
		err = unstructured.SetNestedStringSlice(infraConfig.Object, []string{"test://id-1"}, "spec", "providerIDList")
		g.Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, true, "status", "ready")
		g.Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, int64(1), "status", "replicas")
		g.Expect(err).NotTo(HaveOccurred())

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
		g.Expect(err).NotTo(HaveOccurred())

		// Set NodeRef.
		machinepool.Status.NodeRefs = []corev1.ObjectReference{{Kind: "Node", Name: "machinepool-test-node"}}

		r := &MachinePoolReconciler{
			Client: fake.NewClientBuilder().WithObjects(defaultCluster, defaultKubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build(),
		}

		res, err := r.reconcile(ctx, defaultCluster, machinepool)
		g.Expect(err).NotTo(HaveOccurred())
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
		g.Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		g.Expect(err).NotTo(HaveOccurred())

		// Set NodeRef.
		machinepool.Status.NodeRefs = []corev1.ObjectReference{{Kind: "Node", Name: "machinepool-test-node"}}

		r := &MachinePoolReconciler{
			Client: fake.NewClientBuilder().WithObjects(defaultCluster, defaultKubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build(),
		}

		res, err := r.reconcile(ctx, defaultCluster, machinepool)
		g.Expect(err).NotTo(HaveOccurred())
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
		g.Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		g.Expect(err).NotTo(HaveOccurred())

		// Set infra ready.
		err = unstructured.SetNestedStringSlice(infraConfig.Object, []string{"test://id-1"}, "spec", "providerIDList")
		g.Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, true, "status", "ready")
		g.Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, int64(1), "status", "replicas")
		g.Expect(err).NotTo(HaveOccurred())

		// Set NodeRef.
		machinepool.Status.NodeRefs = []corev1.ObjectReference{{Kind: "Node", Name: "machinepool-test-node"}}

		r := &MachinePoolReconciler{
			Client: fake.NewClientBuilder().WithObjects(defaultCluster, defaultKubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build(),
		}

		res, err := r.reconcile(ctx, defaultCluster, machinepool)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		// Set ReadyReplicas
		machinepool.Status.ReadyReplicas = 1

		// Scale up
		machinepool.Spec.Replicas = pointer.Int32(5)

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
		g.Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		g.Expect(err).NotTo(HaveOccurred())

		// Set infra ready.
		err = unstructured.SetNestedStringSlice(infraConfig.Object, []string{"test://id-1"}, "spec", "providerIDList")
		g.Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, true, "status", "ready")
		g.Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, int64(4), "status", "replicas")
		g.Expect(err).NotTo(HaveOccurred())

		machinepool.Spec.Replicas = pointer.Int32(4)

		// Set NodeRef.
		machinepool.Status.NodeRefs = []corev1.ObjectReference{
			{Kind: "Node", Name: "machinepool-test-node-0"},
			{Kind: "Node", Name: "machinepool-test-node-1"},
			{Kind: "Node", Name: "machinepool-test-node-2"},
			{Kind: "Node", Name: "machinepool-test-node-3"},
		}

		r := &MachinePoolReconciler{
			Client: fake.NewClientBuilder().WithObjects(defaultCluster, defaultKubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build(),
		}

		res, err := r.reconcile(ctx, defaultCluster, machinepool)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		// Set ReadyReplicas
		machinepool.Status.ReadyReplicas = 4

		// Scale down
		machinepool.Spec.Replicas = pointer.Int32(1)

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
		g.Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		g.Expect(err).NotTo(HaveOccurred())

		// Set infra ready.
		err = unstructured.SetNestedStringSlice(infraConfig.Object, []string{"test://id-1"}, "spec", "providerIDList")
		g.Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, true, "status", "ready")
		g.Expect(err).NotTo(HaveOccurred())

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
		g.Expect(err).NotTo(HaveOccurred())

		// Set NodeRef.
		machinepool.Status.NodeRefs = []corev1.ObjectReference{{Kind: "Node", Name: "machinepool-test-node"}}

		// Set Deletion Timestamp.
		machinepool.SetDeletionTimestamp(&deletionTimestamp)
		machinepool.Finalizers = []string{expv1.MachinePoolFinalizer}

		r := &MachinePoolReconciler{
			Client: fake.NewClientBuilder().WithObjects(defaultCluster, defaultKubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build(),
		}

		res, err := r.reconcile(ctx, defaultCluster, machinepool)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(machinepool)
		g.Expect(machinepool.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseDeleting))
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
			expectResult: ctrl.Result{RequeueAfter: externalReadyWait},
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
								},
								DataSecretName: pointer.String("data"),
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
								DataSecretName: pointer.String("data"),
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
								},
								DataSecretName: pointer.String("data"),
							},
						},
					},
				},
				Status: expv1.MachinePoolStatus{
					BootstrapReady: false,
				},
			},
			expectError:  false,
			expectResult: ctrl.Result{RequeueAfter: externalReadyWait},
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
			r := &MachinePoolReconciler{
				Client: fake.NewClientBuilder().WithObjects(tc.machinepool, bootstrapConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build(),
			}

			res, err := r.reconcileBootstrap(ctx, defaultCluster, tc.machinepool)
			g.Expect(res).To(Equal(tc.expectResult))
			if tc.expectError {
				g.Expect(err).ToNot(BeNil())
			} else {
				g.Expect(err).To(BeNil())
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
			Replicas: pointer.Int32(1),
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: builder.BootstrapGroupVersion.String(),
							Kind:       builder.TestBootstrapConfigKind,
							Name:       "bootstrap-config1",
						},
					},
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: builder.InfrastructureGroupVersion.String(),
						Kind:       builder.TestInfrastructureMachineTemplateKind,
						Name:       "infra-config1",
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
					Replicas: pointer.Int32(1),
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Bootstrap: clusterv1.Bootstrap{
								ConfigRef: &corev1.ObjectReference{
									APIVersion: builder.BootstrapGroupVersion.String(),
									Kind:       builder.TestBootstrapConfigKind,
									Name:       "bootstrap-config1",
								},
							},
							InfrastructureRef: corev1.ObjectReference{
								APIVersion: builder.InfrastructureGroupVersion.String(),
								Kind:       builder.TestInfrastructureMachineTemplateKind,
								Name:       "infra-config1",
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
			name: "infrastructure ref is paused",
			infraConfig: map[string]interface{}{
				"kind":       builder.TestInfrastructureMachineTemplateKind,
				"apiVersion": builder.InfrastructureGroupVersion.String(),
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": metav1.NamespaceDefault,
					"annotations": map[string]interface{}{
						"cluster.x-k8s.io/paused": "true",
					},
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
			expectChanged: false,
			expected: func(g *WithT, m *expv1.MachinePool) {
				g.Expect(m.Status.InfrastructureReady).To(BeFalse())
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
					Replicas: pointer.Int32(0),
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Bootstrap: clusterv1.Bootstrap{
								ConfigRef: &corev1.ObjectReference{
									APIVersion: builder.BootstrapGroupVersion.String(),
									Kind:       builder.TestBootstrapConfigKind,
									Name:       "bootstrap-config1",
								},
							},
							InfrastructureRef: corev1.ObjectReference{
								APIVersion: builder.InfrastructureGroupVersion.String(),
								Kind:       builder.TestInfrastructureMachineTemplateKind,
								Name:       "infra-config1",
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
			r := &MachinePoolReconciler{
				Client: fake.NewClientBuilder().WithObjects(tc.machinepool, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build(),
			}

			res, err := r.reconcileInfrastructure(ctx, defaultCluster, tc.machinepool)
			if tc.expectRequeueAfter {
				g.Expect(res.RequeueAfter).To(BeNumerically(">=", 0))
			} else {
				g.Expect(res.RequeueAfter).To(Equal(time.Duration(0)))
			}
			r.reconcilePhase(tc.machinepool)
			if tc.expectError {
				g.Expect(err).ToNot(BeNil())
			} else {
				g.Expect(err).To(BeNil())
			}

			if tc.expected != nil {
				tc.expected(g, tc.machinepool)
			}
		})
	}
}

func TestReconcileMachinePoolMachines(t *testing.T) {
	defaultCluster := clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: metav1.NamespaceDefault,
		},
	}

	defaultMachinePool := expv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machinepool-test",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: defaultCluster.Name,
			},
		},
		Spec: expv1.MachinePoolSpec{
			ClusterName: defaultCluster.Name,
			Replicas:    pointer.Int32(2),
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
							Kind:       "BootstrapConfig",
							Name:       "bootstrap-config1",
						},
					},
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
						Kind:       "InfrastructureConfig",
						Name:       "infra-config1",
					},
				},
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
					clusterv1.ClusterNameLabel:     defaultCluster.Name,
					clusterv1.MachinePoolNameLabel: defaultMachinePool.Name,
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
					clusterv1.ClusterNameLabel:     defaultCluster.Name,
					clusterv1.MachinePoolNameLabel: defaultMachinePool.Name,
				},
			},
		},
	}

	machine1 := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine1",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:     defaultCluster.Name,
				clusterv1.MachinePoolNameLabel: "machinepool-test",
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: clusterName,
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureMachine",
				Name:       "infra-machine1",
				Namespace:  metav1.NamespaceDefault,
			},
		},
	}

	machine2 := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine2",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:     defaultCluster.Name,
				clusterv1.MachinePoolNameLabel: "machinepool-test",
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: clusterName,
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureMachine",
				Name:       "infra-machine2",
				Namespace:  metav1.NamespaceDefault,
			},
		},
	}

	machine3 := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine3",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:     defaultCluster.Name,
				clusterv1.MachinePoolNameLabel: "machinepool-test",
				clusterv1.FallbackMachineLabel: "true",
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: clusterName,
		},
	}

	machine4 := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine4",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:     defaultCluster.Name,
				clusterv1.MachinePoolNameLabel: "machinepool-test",
				clusterv1.FallbackMachineLabel: "true",
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: clusterName,
		},
	}

	machine5 := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine5",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:     defaultCluster.Name,
				clusterv1.MachinePoolNameLabel: "machinepool-test",
				clusterv1.FallbackMachineLabel: "true",
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: clusterName,
		},
	}

	testCases := []struct {
		name            string
		bootstrapConfig map[string]interface{}
		infraConfig     map[string]interface{}
		machines        []clusterv1.Machine
		infraMachines   []unstructured.Unstructured
		machinepool     *expv1.MachinePool
		expectError     bool
		expectFallback  bool
	}{
		{
			name: "two infra machines, should create two machinepool machines",
			infraConfig: map[string]interface{}{
				"kind":       "InfrastructureConfig",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
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
					"infrastructureMachineKind": "InfrastructureMachine",
					"infrastructureMachineSelector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							clusterv1.ClusterNameLabel:     defaultCluster.Name,
							clusterv1.MachinePoolNameLabel: defaultMachinePool.Name,
						},
					},
				},
			},
			infraMachines: []unstructured.Unstructured{
				infraMachine1,
				infraMachine2,
			},
			expectError: false,
		},
		{
			name: "two infra machines and two machinepool machines, nothing to do",
			infraConfig: map[string]interface{}{
				"kind":       "InfrastructureConfig",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
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
					"infrastructureMachineKind": "InfrastructureMachine",
					"infrastructureMachineSelector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							clusterv1.ClusterNameLabel:     defaultCluster.Name,
							clusterv1.MachinePoolNameLabel: defaultMachinePool.Name,
						},
					},
				},
			},
			machines: []clusterv1.Machine{
				machine1,
				machine2,
			},
			infraMachines: []unstructured.Unstructured{
				infraMachine1,
				infraMachine2,
			},
			expectError: false,
		},
		{
			name: "one extra machine, should delete",
			infraConfig: map[string]interface{}{
				"kind":       "InfrastructureConfig",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
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
					"infrastructureMachineKind": "InfrastructureMachine",
					"infrastructureMachineSelector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							clusterv1.ClusterNameLabel:     defaultCluster.Name,
							clusterv1.MachinePoolNameLabel: defaultMachinePool.Name,
						},
					},
				},
			},
			machines: []clusterv1.Machine{
				machine1,
				machine2,
			},
			infraMachines: []unstructured.Unstructured{
				infraMachine1,
			},
			expectError: false,
		},
		{
			name: "fallback machinepool, nothing to do",
			infraConfig: map[string]interface{}{
				"kind":       "InfrastructureConfig",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
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
			machines: []clusterv1.Machine{
				machine3,
				machine4,
			},
			expectError:    false,
			expectFallback: true,
		},
		{
			name: "fallback machinepool, create one more machine",
			infraConfig: map[string]interface{}{
				"kind":       "InfrastructureConfig",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
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
			machines: []clusterv1.Machine{
				machine3,
			},
			expectError:    false,
			expectFallback: true,
		},
		{
			name: "fallback machinepool, delete extra machine",
			infraConfig: map[string]interface{}{
				"kind":       "InfrastructureConfig",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
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
			machines: []clusterv1.Machine{
				machine3,
				machine4,
				machine5,
			},
			expectError:    false,
			expectFallback: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			if tc.machinepool == nil {
				tc.machinepool = defaultMachinePool.DeepCopy()
			}

			objs := []client.Object{defaultCluster.DeepCopy()}

			infraConfig := &unstructured.Unstructured{Object: tc.infraConfig}

			objs = append(objs, tc.machinepool, infraConfig.DeepCopy())

			for _, infraMachine := range tc.infraMachines {
				objs = append(objs, infraMachine.DeepCopy())
			}

			for _, machine := range tc.machines {
				objs = append(objs, machine.DeepCopy())
			}

			r := &MachinePoolReconciler{
				Client: fake.NewClientBuilder().WithObjects(objs...).Build(),
			}

			err := r.reconcileMachinePoolMachines(ctx, tc.machinepool, infraConfig)

			r.reconcilePhase(tc.machinepool)
			if tc.expectError {
				g.Expect(err).ToNot(BeNil())
			} else {
				g.Expect(err).To(BeNil())

				g.Eventually(func() bool {
					machineList := &clusterv1.MachineList{}
					labels := map[string]string{
						clusterv1.ClusterNameLabel:     defaultCluster.Name,
						clusterv1.MachinePoolNameLabel: tc.machinepool.Name,
					}
					if err := r.Client.List(ctx, machineList, client.InNamespace(tc.machinepool.Namespace), client.MatchingLabels(labels)); err != nil {
						t.Log("Failed to list machines with error:", err)
						return false
					}

					if tc.expectFallback {
						if len(machineList.Items) != int(*tc.machinepool.Spec.Replicas) {
							t.Logf("Machine list length %d != replicas %d", len(machineList.Items), *tc.machinepool.Spec.Replicas)

							return false
						}
					} else {
						if len(machineList.Items) != len(tc.infraMachines) {
							t.Logf("Machine list length %d != infraMachine list length %d", len(machineList.Items), len(tc.infraMachines))

							return false
						}

						for i := range machineList.Items {
							machine := &machineList.Items[i]
							infraMachine, err := external.Get(ctx, r.Client, &machine.Spec.InfrastructureRef, machine.Namespace)
							if err != nil {
								t.Log("Failed to get infraMachine with error:", err)
								return false
							}

							if util.IsControlledBy(infraMachine, machine) {
								t.Log("InfraMachine is not controlled by machine")
								return false
							}
						}
					}

					return true
				}, timeout).Should(BeTrue())
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
	g.Expect(env.Create(ctx, testCluster)).To(BeNil())
	g.Expect(env.CreateKubeconfigSecret(ctx, testCluster)).To(Succeed())
	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(ns, testCluster)

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
						},
					},
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: builder.InfrastructureGroupVersion.String(),
						Kind:       builder.TestInfrastructureMachineTemplateKind,
						Name:       "infra-config1",
					},
				},
			},
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
		g.Expect(env.Create(ctx, node)).To(BeNil())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(node)

		kubeconfigSecret := kubeconfig.GenerateSecret(testCluster, kubeconfig.FromEnvTestConfig(env.Config, testCluster))
		machinepool := defaultMachinePool.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Setup prerequisites - a running MachinePool with one instance and user sets Replicas to 0

		// set replicas to 0
		machinepool.Spec.Replicas = pointer.Int32(0)

		// set nodeRefs to one instance
		machinepool.Status.NodeRefs = []corev1.ObjectReference{{Kind: "Node", Name: "machinepool-test-node"}}

		// set infra providerIDList
		err = unstructured.SetNestedStringSlice(infraConfig.Object, []string{"test://machinepool-test-node"}, "spec", "providerIDList")
		g.Expect(err).NotTo(HaveOccurred())

		// set infra replicas
		err = unstructured.SetNestedField(infraConfig.Object, int64(1), "status", "replicas")
		g.Expect(err).NotTo(HaveOccurred())

		r := &MachinePoolReconciler{
			Client:   fake.NewClientBuilder().WithObjects(testCluster, kubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build(),
			recorder: record.NewFakeRecorder(32),
		}

		res, err := r.reconcile(ctx, testCluster, machinepool)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(machinepool)

		g.Expect(machinepool.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseScalingDown))

		delNode := &corev1.Node{}
		g.Expect(env.Get(ctx, client.ObjectKeyFromObject(node), delNode)).To(BeNil())
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
		g.Expect(env.Create(ctx, node)).To(BeNil())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(node)

		kubeconfigSecret := kubeconfig.GenerateSecret(testCluster, kubeconfig.FromEnvTestConfig(env.Config, testCluster))
		machinepool := defaultMachinePool.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Setup prerequisites - a running MachinePool with one instance and user sets Replicas to 0

		// set replicas to 0
		machinepool.Spec.Replicas = pointer.Int32(0)

		// set nodeRefs to one instance
		machinepool.Status.NodeRefs = []corev1.ObjectReference{{Kind: "Node", Name: "machinepool-test-node"}}

		// set infra replicas
		err = unstructured.SetNestedField(infraConfig.Object, int64(0), "status", "replicas")
		g.Expect(err).NotTo(HaveOccurred())

		r := &MachinePoolReconciler{
			Client:   fake.NewClientBuilder().WithObjects(testCluster, kubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build(),
			recorder: record.NewFakeRecorder(32),
		}

		res, err := r.reconcile(ctx, testCluster, machinepool)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(machinepool)

		g.Expect(machinepool.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseRunning))

		delNode := &corev1.Node{}
		err = env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(node), delNode)
		g.Expect(err).ToNot(BeNil())
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
		machinepool.Spec.Replicas = pointer.Int32(0)

		// set nodeRefs to no instance
		machinepool.Status.NodeRefs = []corev1.ObjectReference{}

		// set infra replicas
		err := unstructured.SetNestedField(infraConfig.Object, int64(0), "status", "replicas")
		g.Expect(err).NotTo(HaveOccurred())

		r := &MachinePoolReconciler{
			Client:   fake.NewClientBuilder().WithObjects(testCluster, kubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build(),
			recorder: record.NewFakeRecorder(32),
		}

		res, err := r.reconcile(ctx, testCluster, machinepool)
		g.Expect(err).NotTo(HaveOccurred())
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
		machinepool.Spec.Replicas = pointer.Int32(1)

		// set nodeRefs to no instance
		machinepool.Status.NodeRefs = []corev1.ObjectReference{}

		// set infra replicas
		err := unstructured.SetNestedField(infraConfig.Object, int64(0), "status", "replicas")
		g.Expect(err).NotTo(HaveOccurred())

		r := &MachinePoolReconciler{
			Client:   fake.NewClientBuilder().WithObjects(testCluster, kubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build(),
			recorder: record.NewFakeRecorder(32),
		}

		res, err := r.reconcile(ctx, testCluster, machinepool)
		g.Expect(err).NotTo(HaveOccurred())
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
		g.Expect(env.Create(ctx, node)).To(BeNil())
		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(node)

		kubeconfigSecret := kubeconfig.GenerateSecret(testCluster, kubeconfig.FromEnvTestConfig(env.Config, testCluster))
		machinepool := defaultMachinePool.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Setup prerequisites - a running MachinePool with no refs but providerIDList and replicas set to 1

		// set replicas to 1
		machinepool.Spec.Replicas = pointer.Int32(1)

		// set nodeRefs to no instance
		machinepool.Status.NodeRefs = []corev1.ObjectReference{}

		// set infra providerIDList
		err = unstructured.SetNestedStringSlice(infraConfig.Object, []string{"test://machinepool-test-node"}, "spec", "providerIDList")
		g.Expect(err).NotTo(HaveOccurred())

		// set infra replicas
		err = unstructured.SetNestedField(infraConfig.Object, int64(1), "status", "replicas")
		g.Expect(err).NotTo(HaveOccurred())

		r := &MachinePoolReconciler{
			Client:   fake.NewClientBuilder().WithObjects(testCluster, kubeconfigSecret, machinepool, bootstrapConfig, infraConfig, builder.TestBootstrapConfigCRD, builder.TestInfrastructureMachineTemplateCRD).Build(),
			recorder: record.NewFakeRecorder(32),
		}

		res, err := r.reconcile(ctx, testCluster, machinepool)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(machinepool)

		g.Expect(machinepool.Status.GetTypedPhase()).To(Equal(expv1.MachinePoolPhaseRunning))

		delNode := &corev1.Node{}
		g.Expect(env.Get(ctx, client.ObjectKeyFromObject(node), delNode)).To(BeNil())
	})
}
