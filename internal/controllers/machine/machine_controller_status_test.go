/*
Copyright 2024 The Kubernetes Authors.

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
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
)

func TestSetBootstrapReadyCondition(t *testing.T) {
	defaultMachine := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-test",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
					Kind:       "GenericBootstrapConfig",
					Name:       "bootstrap-config1",
				},
			},
		},
	}

	testCases := []struct {
		name             string
		machine          *clusterv1.Machine
		bootstrapConfig  *unstructured.Unstructured
		expectConditions []metav1.Condition
	}{
		{
			name: "boostrap data secret provided by user/operator",
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.Spec.Bootstrap.ConfigRef = nil
				m.Spec.Bootstrap.DataSecretName = ptr.To("foo")
				return m
			}(),
			bootstrapConfig: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"status": map[string]interface{}{},
			}},
			expectConditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineReadyV1Beta2Condition,
					Status: metav1.ConditionTrue,
					Reason: v1beta2conditions.MultipleInfoReportedReason,
				},
				{
					Type:   clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineBootstrapDataSecretDataSecretUserProvidedV1Beta2Reason,
				},
			},
		},
		{
			name: "InvalidConfig: machine without bootstrap config ref and with dataSecretName not set",
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.Spec.Bootstrap.ConfigRef = nil
				return m
			}(),
			bootstrapConfig: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"status": map[string]interface{}{},
			}},
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineReadyV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineBootstrapInvalidConfigV1Beta2Reason,
					Message: "BootstrapConfigReady: either spec.bootstrap.configRef must be set or spec.bootstrap.dataSecretName must not be empty",
				},
				{
					Type:    clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineBootstrapInvalidConfigV1Beta2Reason,
					Message: "either spec.bootstrap.configRef must be set or spec.bootstrap.dataSecretName must not be empty",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			setBootstrapReadyCondition(ctx, tc.machine, tc.bootstrapConfig)

			// Compute ready by ensuring no other conditions influence the result.
			tc.machine.Status.V1Beta2.Conditions = append(tc.machine.Status.V1Beta2.Conditions,
				metav1.Condition{
					Type:   clusterv1.MachineInfrastructureReadyV1Beta2Condition,
					Status: metav1.ConditionTrue,
				},
				metav1.Condition{
					Type:   clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status: metav1.ConditionTrue,
				},
			)
			setReadyCondition(ctx, tc.machine)
			meta.RemoveStatusCondition(&tc.machine.Status.V1Beta2.Conditions, clusterv1.MachineInfrastructureReadyV1Beta2Condition)
			meta.RemoveStatusCondition(&tc.machine.Status.V1Beta2.Conditions, clusterv1.MachineNodeHealthyV1Beta2Condition)

			g.Expect(tc.machine.GetV1Beta2Conditions()).To(v1beta2conditions.MatchConditions(tc.expectConditions, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestReconcileMachinePhases(t *testing.T) {
	var defaultKubeconfigSecret *corev1.Secret
	defaultCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
		},
	}

	defaultMachine := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-test",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.MachineControlPlaneLabel: "",
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: defaultCluster.Name,
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
					Kind:       "GenericBootstrapConfig",
					Name:       "bootstrap-config1",
				},
			},
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "GenericInfrastructureMachine",
				Name:       "infra-config1",
			},
		},
	}

	defaultBootstrap := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericBootstrapConfig",
			"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
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
			"kind":       "GenericInfrastructureMachine",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"name":      "infra-config1",
				"namespace": metav1.NamespaceDefault,
			},
			"spec":   map[string]interface{}{},
			"status": map[string]interface{}{},
		},
	}

	t.Run("Should set OwnerReference and cluster name label on external objects", func(t *testing.T) {
		g := NewWithT(t)

		ns, err := env.CreateNamespace(ctx, "test-reconcile-machine-phases")
		g.Expect(err).ToNot(HaveOccurred())
		defer func() {
			g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
		}()

		cluster := defaultCluster.DeepCopy()
		cluster.Namespace = ns.Name

		bootstrapConfig := defaultBootstrap.DeepCopy()
		bootstrapConfig.SetNamespace(ns.Name)
		infraMachine := defaultInfra.DeepCopy()
		infraMachine.SetNamespace(ns.Name)
		machine := defaultMachine.DeepCopy()
		machine.Namespace = ns.Name

		g.Expect(env.Create(ctx, cluster)).To(Succeed())
		defaultKubeconfigSecret = kubeconfig.GenerateSecret(cluster, kubeconfig.FromEnvTestConfig(env.Config, cluster))
		g.Expect(env.Create(ctx, defaultKubeconfigSecret)).To(Succeed())

		g.Expect(env.Create(ctx, bootstrapConfig)).To(Succeed())
		g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
		g.Expect(env.Create(ctx, machine)).To(Succeed())

		// Wait until BootstrapConfig has the ownerReference.
		g.Eventually(func(g Gomega) bool {
			if err := env.Get(ctx, client.ObjectKeyFromObject(bootstrapConfig), bootstrapConfig); err != nil {
				return false
			}
			g.Expect(bootstrapConfig.GetOwnerReferences()).To(HaveLen(1))
			g.Expect(bootstrapConfig.GetLabels()[clusterv1.ClusterNameLabel]).To(Equal("test-cluster"))
			return true
		}, 10*time.Second).Should(BeTrue())

		// Wait until InfraMachine has the ownerReference.
		g.Eventually(func(g Gomega) bool {
			if err := env.Get(ctx, client.ObjectKeyFromObject(infraMachine), infraMachine); err != nil {
				return false
			}
			g.Expect(infraMachine.GetOwnerReferences()).To(HaveLen(1))
			g.Expect(infraMachine.GetLabels()[clusterv1.ClusterNameLabel]).To(Equal("test-cluster"))
			return true
		}, 10*time.Second).Should(BeTrue())
	})

	t.Run("Should set `Pending` with a new Machine", func(t *testing.T) {
		g := NewWithT(t)

		ns, err := env.CreateNamespace(ctx, "test-reconcile-machine-phases")
		g.Expect(err).ToNot(HaveOccurred())
		defer func() {
			g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
		}()

		cluster := defaultCluster.DeepCopy()
		cluster.Namespace = ns.Name

		bootstrapConfig := defaultBootstrap.DeepCopy()
		bootstrapConfig.SetNamespace(ns.Name)
		infraMachine := defaultInfra.DeepCopy()
		infraMachine.SetNamespace(ns.Name)
		machine := defaultMachine.DeepCopy()
		machine.Namespace = ns.Name

		g.Expect(env.Create(ctx, cluster)).To(Succeed())
		defaultKubeconfigSecret = kubeconfig.GenerateSecret(cluster, kubeconfig.FromEnvTestConfig(env.Config, cluster))
		g.Expect(env.Create(ctx, defaultKubeconfigSecret)).To(Succeed())

		g.Expect(env.Create(ctx, bootstrapConfig)).To(Succeed())
		g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
		g.Expect(env.Create(ctx, machine)).To(Succeed())

		// Wait until Machine was reconciled.
		g.Eventually(func(g Gomega) bool {
			if err := env.Get(ctx, client.ObjectKeyFromObject(machine), machine); err != nil {
				return false
			}
			g.Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhasePending))
			// LastUpdated should be set as the phase changes
			g.Expect(machine.Status.LastUpdated).NotTo(BeNil())
			return true
		}, 10*time.Second).Should(BeTrue())
	})

	t.Run("Should set `Provisioning` when bootstrap is ready", func(t *testing.T) {
		g := NewWithT(t)

		ns, err := env.CreateNamespace(ctx, "test-reconcile-machine-phases")
		g.Expect(err).ToNot(HaveOccurred())
		defer func() {
			g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
		}()

		cluster := defaultCluster.DeepCopy()
		cluster.Namespace = ns.Name

		bootstrapConfig := defaultBootstrap.DeepCopy()
		bootstrapConfig.SetNamespace(ns.Name)
		infraMachine := defaultInfra.DeepCopy()
		infraMachine.SetNamespace(ns.Name)
		machine := defaultMachine.DeepCopy()
		machine.Namespace = ns.Name

		g.Expect(env.Create(ctx, cluster)).To(Succeed())
		defaultKubeconfigSecret = kubeconfig.GenerateSecret(cluster, kubeconfig.FromEnvTestConfig(env.Config, cluster))
		g.Expect(env.Create(ctx, defaultKubeconfigSecret)).To(Succeed())

		g.Expect(env.Create(ctx, bootstrapConfig)).To(Succeed())
		g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
		// We have to subtract 2 seconds, because .status.lastUpdated does not contain miliseconds.
		preUpdate := time.Now().Add(-2 * time.Second)
		g.Expect(env.Create(ctx, machine)).To(Succeed())

		// Set the LastUpdated to be able to verify it is updated when the phase changes
		modifiedMachine := machine.DeepCopy()
		g.Expect(env.Status().Patch(ctx, modifiedMachine, client.MergeFrom(machine))).To(Succeed())

		// Set bootstrap ready.
		modifiedBootstrapConfig := bootstrapConfig.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, true, "status", "ready")).To(Succeed())
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, "secret-data", "status", "dataSecretName")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedBootstrapConfig, client.MergeFrom(bootstrapConfig))).To(Succeed())

		// Wait until Machine was reconciled.
		g.Eventually(func(g Gomega) bool {
			if err := env.Get(ctx, client.ObjectKeyFromObject(machine), machine); err != nil {
				return false
			}
			g.Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseProvisioning))
			// Verify that the LastUpdated timestamp was updated
			g.Expect(machine.Status.LastUpdated).NotTo(BeNil())
			g.Expect(machine.Status.LastUpdated.After(preUpdate)).To(BeTrue())
			return true
		}, 10*time.Second).Should(BeTrue())
	})

	t.Run("Should set `Running` when bootstrap and infra is ready", func(t *testing.T) {
		g := NewWithT(t)

		ns, err := env.CreateNamespace(ctx, "test-reconcile-machine-phases")
		g.Expect(err).ToNot(HaveOccurred())
		defer func() {
			g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
		}()

		nodeProviderID := fmt.Sprintf("test://%s", util.RandomString(6))

		cluster := defaultCluster.DeepCopy()
		cluster.Namespace = ns.Name

		bootstrapConfig := defaultBootstrap.DeepCopy()
		bootstrapConfig.SetNamespace(ns.Name)
		infraMachine := defaultInfra.DeepCopy()
		infraMachine.SetNamespace(ns.Name)
		g.Expect(unstructured.SetNestedField(infraMachine.Object, nodeProviderID, "spec", "providerID")).To(Succeed())
		g.Expect(unstructured.SetNestedField(infraMachine.Object, "us-east-2a", "spec", "failureDomain")).To(Succeed())
		machine := defaultMachine.DeepCopy()
		machine.Namespace = ns.Name

		// Create Node.
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "machine-test-node-",
			},
			Spec: corev1.NodeSpec{ProviderID: nodeProviderID},
		}
		g.Expect(env.Create(ctx, node)).To(Succeed())
		defer func() {
			g.Expect(env.Cleanup(ctx, node)).To(Succeed())
		}()

		g.Expect(env.Create(ctx, cluster)).To(Succeed())
		defaultKubeconfigSecret = kubeconfig.GenerateSecret(cluster, kubeconfig.FromEnvTestConfig(env.Config, cluster))
		g.Expect(env.Create(ctx, defaultKubeconfigSecret)).To(Succeed())

		g.Expect(env.Create(ctx, bootstrapConfig)).To(Succeed())
		g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
		// We have to subtract 2 seconds, because .status.lastUpdated does not contain miliseconds.
		preUpdate := time.Now().Add(-2 * time.Second)
		g.Expect(env.Create(ctx, machine)).To(Succeed())

		modifiedMachine := machine.DeepCopy()
		// Set NodeRef.
		machine.Status.NodeRef = &corev1.ObjectReference{Kind: "Node", Name: node.Name}
		g.Expect(env.Status().Patch(ctx, modifiedMachine, client.MergeFrom(machine))).To(Succeed())

		// Set bootstrap ready.
		modifiedBootstrapConfig := bootstrapConfig.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, true, "status", "ready")).To(Succeed())
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, "secret-data", "status", "dataSecretName")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedBootstrapConfig, client.MergeFrom(bootstrapConfig))).To(Succeed())

		// Set infra ready.
		modifiedInfraMachine := infraMachine.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedInfraMachine.Object, true, "status", "ready")).To(Succeed())
		g.Expect(unstructured.SetNestedField(modifiedInfraMachine.Object, []interface{}{
			map[string]interface{}{
				"type":    "InternalIP",
				"address": "10.0.0.1",
			},
			map[string]interface{}{
				"type":    "InternalIP",
				"address": "10.0.0.2",
			},
		}, "status", "addresses")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedInfraMachine, client.MergeFrom(infraMachine))).To(Succeed())

		// Wait until Machine was reconciled.
		g.Eventually(func(g Gomega) bool {
			if err := env.Get(ctx, client.ObjectKeyFromObject(machine), machine); err != nil {
				return false
			}
			g.Expect(machine.Status.Addresses).To(HaveLen(2))
			g.Expect(*machine.Spec.FailureDomain).To(Equal("us-east-2a"))
			g.Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseRunning))
			// Verify that the LastUpdated timestamp was updated
			g.Expect(machine.Status.LastUpdated).NotTo(BeNil())
			g.Expect(machine.Status.LastUpdated.After(preUpdate)).To(BeTrue())
			return true
		}, 10*time.Second).Should(BeTrue())
	})

	t.Run("Should set `Running` when bootstrap and infra is ready with no Status.Addresses", func(t *testing.T) {
		g := NewWithT(t)

		ns, err := env.CreateNamespace(ctx, "test-reconcile-machine-phases")
		g.Expect(err).ToNot(HaveOccurred())
		defer func() {
			g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
		}()

		nodeProviderID := fmt.Sprintf("test://%s", util.RandomString(6))

		cluster := defaultCluster.DeepCopy()
		cluster.Namespace = ns.Name

		bootstrapConfig := defaultBootstrap.DeepCopy()
		bootstrapConfig.SetNamespace(ns.Name)
		infraMachine := defaultInfra.DeepCopy()
		infraMachine.SetNamespace(ns.Name)
		g.Expect(unstructured.SetNestedField(infraMachine.Object, nodeProviderID, "spec", "providerID")).To(Succeed())
		machine := defaultMachine.DeepCopy()
		machine.Namespace = ns.Name

		// Create Node.
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "machine-test-node-",
			},
			Spec: corev1.NodeSpec{ProviderID: nodeProviderID},
		}
		g.Expect(env.Create(ctx, node)).To(Succeed())
		defer func() {
			g.Expect(env.Cleanup(ctx, node)).To(Succeed())
		}()

		g.Expect(env.Create(ctx, cluster)).To(Succeed())
		defaultKubeconfigSecret = kubeconfig.GenerateSecret(cluster, kubeconfig.FromEnvTestConfig(env.Config, cluster))
		g.Expect(env.Create(ctx, defaultKubeconfigSecret)).To(Succeed())

		g.Expect(env.Create(ctx, bootstrapConfig)).To(Succeed())
		g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
		// We have to subtract 2 seconds, because .status.lastUpdated does not contain miliseconds.
		preUpdate := time.Now().Add(-2 * time.Second)
		g.Expect(env.Create(ctx, machine)).To(Succeed())

		modifiedMachine := machine.DeepCopy()
		// Set NodeRef.
		machine.Status.NodeRef = &corev1.ObjectReference{Kind: "Node", Name: node.Name}
		g.Expect(env.Status().Patch(ctx, modifiedMachine, client.MergeFrom(machine))).To(Succeed())

		// Set bootstrap ready.
		modifiedBootstrapConfig := bootstrapConfig.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, true, "status", "ready")).To(Succeed())
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, "secret-data", "status", "dataSecretName")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedBootstrapConfig, client.MergeFrom(bootstrapConfig))).To(Succeed())

		// Set infra ready.
		modifiedInfraMachine := infraMachine.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedInfraMachine.Object, true, "status", "ready")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedInfraMachine, client.MergeFrom(infraMachine))).To(Succeed())

		// Wait until Machine was reconciled.
		g.Eventually(func(g Gomega) bool {
			if err := env.Get(ctx, client.ObjectKeyFromObject(machine), machine); err != nil {
				return false
			}
			g.Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseRunning))
			g.Expect(machine.Status.Addresses).To(BeEmpty())
			// Verify that the LastUpdated timestamp was updated
			g.Expect(machine.Status.LastUpdated).NotTo(BeNil())
			g.Expect(machine.Status.LastUpdated.After(preUpdate)).To(BeTrue())
			return true
		}, 10*time.Second).Should(BeTrue())
	})

	t.Run("Should set `Running` when bootstrap, infra, and NodeRef is ready", func(t *testing.T) {
		g := NewWithT(t)

		ns, err := env.CreateNamespace(ctx, "test-reconcile-machine-phases")
		g.Expect(err).ToNot(HaveOccurred())
		defer func() {
			g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
		}()

		nodeProviderID := fmt.Sprintf("test://%s", util.RandomString(6))

		cluster := defaultCluster.DeepCopy()
		cluster.Namespace = ns.Name

		bootstrapConfig := defaultBootstrap.DeepCopy()
		bootstrapConfig.SetNamespace(ns.Name)
		infraMachine := defaultInfra.DeepCopy()
		infraMachine.SetNamespace(ns.Name)
		g.Expect(unstructured.SetNestedField(infraMachine.Object, nodeProviderID, "spec", "providerID")).To(Succeed())
		machine := defaultMachine.DeepCopy()
		machine.Namespace = ns.Name

		// Create Node.
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "machine-test-node-",
			},
			Spec: corev1.NodeSpec{ProviderID: nodeProviderID},
		}
		g.Expect(env.Create(ctx, node)).To(Succeed())
		defer func() {
			g.Expect(env.Cleanup(ctx, node)).To(Succeed())
		}()

		g.Expect(env.Create(ctx, cluster)).To(Succeed())
		defaultKubeconfigSecret = kubeconfig.GenerateSecret(cluster, kubeconfig.FromEnvTestConfig(env.Config, cluster))
		g.Expect(env.Create(ctx, defaultKubeconfigSecret)).To(Succeed())

		g.Expect(env.Create(ctx, bootstrapConfig)).To(Succeed())
		g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
		// We have to subtract 2 seconds, because .status.lastUpdated does not contain miliseconds.
		preUpdate := time.Now().Add(-2 * time.Second)
		g.Expect(env.Create(ctx, machine)).To(Succeed())

		modifiedMachine := machine.DeepCopy()
		// Set NodeRef.
		machine.Status.NodeRef = &corev1.ObjectReference{Kind: "Node", Name: node.Name}
		g.Expect(env.Status().Patch(ctx, modifiedMachine, client.MergeFrom(machine))).To(Succeed())

		// Set bootstrap ready.
		modifiedBootstrapConfig := bootstrapConfig.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, true, "status", "ready")).To(Succeed())
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, "secret-data", "status", "dataSecretName")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedBootstrapConfig, client.MergeFrom(bootstrapConfig))).To(Succeed())

		// Set infra ready.
		modifiedInfraMachine := infraMachine.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedInfraMachine.Object, true, "status", "ready")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedInfraMachine, client.MergeFrom(infraMachine))).To(Succeed())

		// Wait until Machine was reconciled.
		g.Eventually(func(g Gomega) bool {
			if err := env.Get(ctx, client.ObjectKeyFromObject(machine), machine); err != nil {
				return false
			}
			g.Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseRunning))
			// Verify that the LastUpdated timestamp was updated
			g.Expect(machine.Status.LastUpdated).NotTo(BeNil())
			g.Expect(machine.Status.LastUpdated.After(preUpdate)).To(BeTrue())
			return true
		}, 10*time.Second).Should(BeTrue())
	})

	t.Run("Should set `Provisioned` when there is a ProviderID and there is no Node", func(t *testing.T) {
		g := NewWithT(t)

		ns, err := env.CreateNamespace(ctx, "test-reconcile-machine-phases")
		g.Expect(err).ToNot(HaveOccurred())
		defer func() {
			g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
		}()

		nodeProviderID := fmt.Sprintf("test://%s", util.RandomString(6))

		cluster := defaultCluster.DeepCopy()
		cluster.Namespace = ns.Name

		bootstrapConfig := defaultBootstrap.DeepCopy()
		bootstrapConfig.SetNamespace(ns.Name)
		infraMachine := defaultInfra.DeepCopy()
		infraMachine.SetNamespace(ns.Name)
		g.Expect(unstructured.SetNestedField(infraMachine.Object, nodeProviderID, "spec", "providerID")).To(Succeed())
		machine := defaultMachine.DeepCopy()
		machine.Namespace = ns.Name
		// Set Machine ProviderID.
		machine.Spec.ProviderID = ptr.To(nodeProviderID)

		g.Expect(env.Create(ctx, cluster)).To(Succeed())
		defaultKubeconfigSecret = kubeconfig.GenerateSecret(cluster, kubeconfig.FromEnvTestConfig(env.Config, cluster))
		g.Expect(env.Create(ctx, defaultKubeconfigSecret)).To(Succeed())

		g.Expect(env.Create(ctx, bootstrapConfig)).To(Succeed())
		g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
		// We have to subtract 2 seconds, because .status.lastUpdated does not contain miliseconds.
		preUpdate := time.Now().Add(-2 * time.Second)
		g.Expect(env.Create(ctx, machine)).To(Succeed())

		// Set bootstrap ready.
		modifiedBootstrapConfig := bootstrapConfig.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, true, "status", "ready")).To(Succeed())
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, "secret-data", "status", "dataSecretName")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedBootstrapConfig, client.MergeFrom(bootstrapConfig))).To(Succeed())

		// Set infra ready.
		modifiedInfraMachine := infraMachine.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedInfraMachine.Object, true, "status", "ready")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedInfraMachine, client.MergeFrom(infraMachine))).To(Succeed())

		// Wait until Machine was reconciled.
		g.Eventually(func(g Gomega) bool {
			if err := env.Get(ctx, client.ObjectKeyFromObject(machine), machine); err != nil {
				return false
			}
			g.Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseProvisioned))
			// Verify that the LastUpdated timestamp was updated
			g.Expect(machine.Status.LastUpdated).NotTo(BeNil())
			g.Expect(machine.Status.LastUpdated.After(preUpdate)).To(BeTrue())
			return true
		}, 10*time.Second).Should(BeTrue())
	})

	t.Run("Should set `Deleting` when Machine is being deleted", func(t *testing.T) {
		g := NewWithT(t)

		ns, err := env.CreateNamespace(ctx, "test-reconcile-machine-phases")
		g.Expect(err).ToNot(HaveOccurred())
		defer func() {
			g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
		}()

		nodeProviderID := fmt.Sprintf("test://%s", util.RandomString(6))

		cluster := defaultCluster.DeepCopy()
		cluster.Namespace = ns.Name

		bootstrapConfig := defaultBootstrap.DeepCopy()
		bootstrapConfig.SetNamespace(ns.Name)
		infraMachine := defaultInfra.DeepCopy()
		infraMachine.SetNamespace(ns.Name)
		g.Expect(unstructured.SetNestedField(infraMachine.Object, nodeProviderID, "spec", "providerID")).To(Succeed())
		machine := defaultMachine.DeepCopy()
		machine.Namespace = ns.Name

		// Create Node.
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "machine-test-node-",
			},
			Spec: corev1.NodeSpec{ProviderID: nodeProviderID},
		}
		g.Expect(env.Create(ctx, node)).To(Succeed())
		defer func() {
			g.Expect(env.Cleanup(ctx, node)).To(Succeed())
		}()

		g.Expect(env.Create(ctx, cluster)).To(Succeed())
		defaultKubeconfigSecret = kubeconfig.GenerateSecret(cluster, kubeconfig.FromEnvTestConfig(env.Config, cluster))
		g.Expect(env.Create(ctx, defaultKubeconfigSecret)).To(Succeed())

		g.Expect(env.Create(ctx, bootstrapConfig)).To(Succeed())
		g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
		// We have to subtract 2 seconds, because .status.lastUpdated does not contain miliseconds.
		preUpdate := time.Now().Add(-2 * time.Second)
		g.Expect(env.Create(ctx, machine)).To(Succeed())

		// Set bootstrap ready.
		modifiedBootstrapConfig := bootstrapConfig.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, true, "status", "ready")).To(Succeed())
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, "secret-data", "status", "dataSecretName")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedBootstrapConfig, client.MergeFrom(bootstrapConfig))).To(Succeed())

		// Set infra ready.
		modifiedInfraMachine := infraMachine.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedInfraMachine.Object, true, "status", "ready")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedInfraMachine, client.MergeFrom(infraMachine))).To(Succeed())

		// Wait until the Machine has the Machine finalizer
		g.Eventually(func() []string {
			if err := env.Get(ctx, client.ObjectKeyFromObject(machine), machine); err != nil {
				return nil
			}
			return machine.Finalizers
		}, 10*time.Second).Should(HaveLen(1))

		modifiedMachine := machine.DeepCopy()
		// Set NodeRef.
		machine.Status.NodeRef = &corev1.ObjectReference{Kind: "Node", Name: node.Name}
		g.Expect(env.Status().Patch(ctx, modifiedMachine, client.MergeFrom(machine))).To(Succeed())

		modifiedMachine = machine.DeepCopy()
		// Set finalizer so we can check the Machine later, otherwise it would be already gone.
		modifiedMachine.Finalizers = append(modifiedMachine.Finalizers, "test")
		g.Expect(env.Patch(ctx, modifiedMachine, client.MergeFrom(machine))).To(Succeed())

		// Delete Machine
		g.Expect(env.Delete(ctx, machine)).To(Succeed())

		// Wait until Machine was reconciled.
		g.Eventually(func(g Gomega) bool {
			if err := env.Get(ctx, client.ObjectKeyFromObject(machine), machine); err != nil {
				return false
			}
			g.Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseDeleting))
			nodeHealthyCondition := conditions.Get(machine, clusterv1.MachineNodeHealthyCondition)
			g.Expect(nodeHealthyCondition.Status).To(Equal(corev1.ConditionFalse))
			g.Expect(nodeHealthyCondition.Reason).To(Equal(clusterv1.DeletingReason))
			// Verify that the LastUpdated timestamp was updated
			g.Expect(machine.Status.LastUpdated).NotTo(BeNil())
			g.Expect(machine.Status.LastUpdated.After(preUpdate)).To(BeTrue())
			return true
		}, 10*time.Second).Should(BeTrue())
	})
}
