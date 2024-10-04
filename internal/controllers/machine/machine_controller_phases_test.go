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
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/test/builder"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
)

func init() {
	externalReadyWait = 1 * time.Second
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

func TestReconcileBootstrap(t *testing.T) {
	defaultMachine := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-test",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "test-cluster",
			},
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

	defaultCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
		},
	}

	testCases := []struct {
		name                    string
		machine                 *clusterv1.Machine
		bootstrapConfig         map[string]interface{}
		bootstrapConfigGetError error
		expectResult            ctrl.Result
		expectError             bool
		expected                func(g *WithT, m *clusterv1.Machine)
	}{
		{
			name:                    "err reading bootstrap config (something different than not found), it should return error",
			machine:                 defaultMachine.DeepCopy(),
			bootstrapConfig:         nil,
			bootstrapConfigGetError: errors.New("some error"),
			expectResult:            ctrl.Result{},
			expectError:             true,
		},
		{
			name:                    "bootstrap config is not found, it should requeue",
			machine:                 defaultMachine.DeepCopy(),
			bootstrapConfig:         nil,
			bootstrapConfigGetError: nil,
			expectResult:            ctrl.Result{RequeueAfter: externalReadyWait},
			expectError:             false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.BootstrapReady).To(BeFalse())
			},
		},
		{
			name:    "bootstrap config not ready, it should reconcile but no data should surface on the machine",
			machine: defaultMachine.DeepCopy(),
			bootstrapConfig: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready":          false,
					"dataSecretName": "secret-data",
				},
			},
			bootstrapConfigGetError: nil,
			expectResult:            ctrl.Result{},
			expectError:             false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.BootstrapReady).To(BeFalse())
				g.Expect(m.Spec.Bootstrap.DataSecretName).To(BeNil())
			},
		},
		{
			name:    "bootstrap config ready with data, it should reconcile and data should surface on the machine",
			machine: defaultMachine.DeepCopy(),
			bootstrapConfig: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
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
			bootstrapConfigGetError: nil,
			expectResult:            ctrl.Result{},
			expectError:             false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.BootstrapReady).To(BeTrue())
				g.Expect(m.Spec.Bootstrap.DataSecretName).NotTo(BeNil())
				g.Expect(*m.Spec.Bootstrap.DataSecretName).To(Equal("secret-data"))
			},
		},
		{
			name:    "bootstrap config ready and paused, it should reconcile and data should surface on the machine",
			machine: defaultMachine.DeepCopy(),
			bootstrapConfig: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
					"annotations": map[string]interface{}{
						"cluster.x-k8s.io/paused": "true",
					},
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready":          true,
					"dataSecretName": "secret-data",
				},
			},
			bootstrapConfigGetError: nil,
			expectResult:            ctrl.Result{},
			expectError:             false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.BootstrapReady).To(BeTrue())
				g.Expect(m.Spec.Bootstrap.DataSecretName).NotTo(BeNil())
				g.Expect(*m.Spec.Bootstrap.DataSecretName).To(Equal("secret-data"))
			},
		},
		{
			name:    "bootstrap config ready with no bootstrap secret",
			machine: defaultMachine.DeepCopy(),
			bootstrapConfig: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready": true,
				},
			},
			bootstrapConfigGetError: nil,
			expectResult:            ctrl.Result{},
			expectError:             true,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.BootstrapReady).To(BeFalse())
				g.Expect(m.Spec.Bootstrap.DataSecretName).To(BeNil())
			},
		},
		{
			name: "bootstrap data secret and bootstrap ready should not change after bootstrap config is set",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
							Kind:       "GenericBootstrapConfig",
							Name:       "bootstrap-config1",
						},
						DataSecretName: ptr.To("secret-data"),
					},
				},
				Status: clusterv1.MachineStatus{
					BootstrapReady: true,
				},
			},
			bootstrapConfig: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready":          false,
					"dataSecretName": "secret-data-changed",
				},
			},
			bootstrapConfigGetError: nil,
			expectResult:            ctrl.Result{},
			expectError:             false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.BootstrapReady).To(BeTrue())
				g.Expect(*m.Spec.Bootstrap.DataSecretName).To(Equal("secret-data"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			if tc.machine == nil {
				tc.machine = defaultMachine.DeepCopy()
			}

			var bootstrapConfig *unstructured.Unstructured
			if tc.bootstrapConfig != nil {
				bootstrapConfig = &unstructured.Unstructured{Object: tc.bootstrapConfig}
			}

			c := fake.NewClientBuilder().
				WithObjects(tc.machine).Build()

			if tc.bootstrapConfigGetError == nil {
				g.Expect(c.Create(ctx, builder.GenericBootstrapConfigCRD.DeepCopy())).To(Succeed())
			}

			if bootstrapConfig != nil {
				g.Expect(c.Create(ctx, bootstrapConfig)).To(Succeed())
			}

			r := &Reconciler{
				Client: c,
			}
			s := &scope{cluster: defaultCluster, machine: tc.machine}
			res, err := r.reconcileBootstrap(ctx, s)
			g.Expect(res).To(BeComparableTo(tc.expectResult))
			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			if tc.expected != nil {
				tc.expected(g, tc.machine)
			}
		})
	}
}

func TestReconcileInfrastructure(t *testing.T) {
	defaultMachine := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-test",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "test-cluster",
			},
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "GenericInfrastructureMachine",
				Name:       "infra-config1",
			},
		},
	}

	defaultCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: metav1.NamespaceDefault,
		},
	}

	testCases := []struct {
		name                 string
		machine              *clusterv1.Machine
		infraMachine         map[string]interface{}
		infraMachineGetError error
		expectResult         ctrl.Result
		expectError          bool
		expected             func(g *WithT, m *clusterv1.Machine)
	}{
		{
			name:                 "err reading infra machine (something different than not found), it should return error",
			machine:              defaultMachine.DeepCopy(),
			infraMachine:         nil,
			infraMachineGetError: errors.New("some error"),
			expectResult:         ctrl.Result{},
			expectError:          true,
		},
		{
			name:                 "infra machine not found and infrastructure not yet ready, it should requeue",
			machine:              defaultMachine.DeepCopy(),
			infraMachine:         nil,
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{RequeueAfter: externalReadyWait},
			expectError:          false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.InfrastructureReady).To(BeFalse())
				g.Expect(m.Status.FailureMessage).To(BeNil())
				g.Expect(m.Status.FailureReason).To(BeNil())
			},
		},
		{
			name:    "infra machine not ready, it should reconcile but no data should surface on the machine",
			machine: defaultMachine.DeepCopy(),
			infraMachine: map[string]interface{}{
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
					"ready": false,
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
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.InfrastructureReady).To(BeFalse())
				g.Expect(m.Spec.ProviderID).To(BeNil())
				g.Expect(m.Spec.FailureDomain).To(BeNil())
				g.Expect(m.Status.Addresses).To(BeNil())
			},
		},
		{
			name:    "infra machine ready and without optional fields, it should reconcile and data should surface on the machine",
			machine: defaultMachine.DeepCopy(),
			infraMachine: map[string]interface{}{
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
				},
			},
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.InfrastructureReady).To(BeTrue())
				g.Expect(ptr.Deref(m.Spec.ProviderID, "")).To(Equal("test://id-1"))
				g.Expect(m.Spec.FailureDomain).To(BeNil())
				g.Expect(m.Status.Addresses).To(BeNil())
			},
		},
		{
			name:    "infra machine ready and with optional failure domain, it should reconcile and data should surface on the machine",
			machine: defaultMachine.DeepCopy(),
			infraMachine: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{
					"providerID":    "test://id-1",
					"failureDomain": "foo",
				},
				"status": map[string]interface{}{
					"ready": true,
				},
			},
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.InfrastructureReady).To(BeTrue())
				g.Expect(ptr.Deref(m.Spec.ProviderID, "")).To(Equal("test://id-1"))
				g.Expect(ptr.Deref(m.Spec.FailureDomain, "")).To(Equal("foo"))
				g.Expect(m.Status.Addresses).To(BeNil())
			},
		},
		{
			name:    "infra machine ready and with optional addresses, it should reconcile and data should surface on the machine",
			machine: defaultMachine.DeepCopy(),
			infraMachine: map[string]interface{}{
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
						map[string]interface{}{
							"type":    "InternalIP",
							"address": "10.0.0.2",
						},
					},
				},
			},
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.InfrastructureReady).To(BeTrue())
				g.Expect(ptr.Deref(m.Spec.ProviderID, "")).To(Equal("test://id-1"))
				g.Expect(m.Spec.FailureDomain).To(BeNil())
				g.Expect(m.Status.Addresses).To(HaveLen(2))
			},
		},
		{
			name:    "infra machine ready and with all the optional fields, it should reconcile and data should surface on the machine",
			machine: defaultMachine.DeepCopy(),
			infraMachine: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{
					"providerID":    "test://id-1",
					"failureDomain": "foo",
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
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.InfrastructureReady).To(BeTrue())
				g.Expect(ptr.Deref(m.Spec.ProviderID, "")).To(Equal("test://id-1"))
				g.Expect(ptr.Deref(m.Spec.FailureDomain, "")).To(Equal("foo"))
				g.Expect(m.Status.Addresses).To(HaveLen(2))
			},
		},
		{
			name:    "infra machine ready and paused, it should reconcile and data should surface on the machine",
			machine: defaultMachine.DeepCopy(),
			infraMachine: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": metav1.NamespaceDefault,
					"annotations": map[string]interface{}{
						"cluster.x-k8s.io/paused": "true",
					},
				},
				"spec": map[string]interface{}{
					"providerID":    "test://id-1",
					"failureDomain": "foo",
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
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.InfrastructureReady).To(BeTrue())
				g.Expect(ptr.Deref(m.Spec.ProviderID, "")).To(Equal("test://id-1"))
				g.Expect(ptr.Deref(m.Spec.FailureDomain, "")).To(Equal("foo"))
				g.Expect(m.Status.Addresses).To(HaveLen(2))
			},
		},
		{
			name:    "infra machine ready and no provider ID, it should fail",
			machine: defaultMachine.DeepCopy(),
			infraMachine: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready": true,
				},
			},
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          true,
		},
		{
			name: "should never revert back to infrastructure not ready",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
						Kind:       "GenericInfrastructureMachine",
						Name:       "infra-config1",
					},
					ProviderID:    ptr.To("test://something"),
					FailureDomain: ptr.To("something"),
				},
				Status: clusterv1.MachineStatus{
					InfrastructureReady: true,
					Addresses: []clusterv1.MachineAddress{
						{
							Type:    clusterv1.MachineExternalIP,
							Address: "1.2.3.4",
						},
					},
				},
			},
			infraMachine: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{
					"providerID":    "test://id-1",
					"failureDomain": "foo",
				},
				"status": map[string]interface{}{
					"ready": false,
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
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.InfrastructureReady).To(BeTrue())
				g.Expect(ptr.Deref(m.Spec.ProviderID, "")).To(Equal("test://id-1"))
				g.Expect(ptr.Deref(m.Spec.FailureDomain, "")).To(Equal("foo"))
				g.Expect(m.Status.Addresses).To(HaveLen(2))
			},
		},
		{
			name: "should change data also after infrastructure ready is set",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
						Kind:       "GenericInfrastructureMachine",
						Name:       "infra-config1",
					},
					ProviderID:    ptr.To("test://something"),
					FailureDomain: ptr.To("something"),
				},
				Status: clusterv1.MachineStatus{
					InfrastructureReady: true,
					Addresses: []clusterv1.MachineAddress{
						{
							Type:    clusterv1.MachineExternalIP,
							Address: "1.2.3.4",
						},
					},
				},
			},
			infraMachine: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"spec": map[string]interface{}{
					"providerID":    "test://id-1",
					"failureDomain": "foo",
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
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.InfrastructureReady).To(BeTrue())
				g.Expect(ptr.Deref(m.Spec.ProviderID, "")).To(Equal("test://id-1"))
				g.Expect(ptr.Deref(m.Spec.FailureDomain, "")).To(Equal("foo"))
				g.Expect(m.Status.Addresses).To(HaveLen(2))
			},
		},
		{
			name: "err reading infra machine when infrastructure have been ready (something different than not found), it should return error",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
						Kind:       "GenericInfrastructureMachine",
						Name:       "infra-config1",
					},
				},
				Status: clusterv1.MachineStatus{
					InfrastructureReady: true,
				},
			},
			infraMachine:         nil,
			infraMachineGetError: errors.New("some error"),
			expectResult:         ctrl.Result{},
			expectError:          true,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.InfrastructureReady).To(BeTrue())
				g.Expect(m.Status.FailureMessage).To(BeNil())
				g.Expect(m.Status.FailureReason).To(BeNil())
			},
		},
		{
			name: "infra machine not found when infrastructure have been ready, should be treated as terminal error",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
						Kind:       "GenericInfrastructureMachine",
						Name:       "infra-config1",
					},
				},
				Status: clusterv1.MachineStatus{
					InfrastructureReady: true,
				},
			},
			infraMachine:         nil,
			infraMachineGetError: nil,
			expectResult:         ctrl.Result{},
			expectError:          true,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.InfrastructureReady).To(BeTrue())
				g.Expect(m.Status.FailureMessage).ToNot(BeNil())
				g.Expect(m.Status.FailureReason).ToNot(BeNil())
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			var infraMachine *unstructured.Unstructured
			if tc.infraMachine != nil {
				infraMachine = &unstructured.Unstructured{Object: tc.infraMachine}
			}
			c := fake.NewClientBuilder().
				WithObjects(tc.machine).Build()

			if tc.infraMachineGetError == nil {
				g.Expect(c.Create(ctx, builder.GenericInfrastructureMachineCRD.DeepCopy())).To(Succeed())
			}

			if infraMachine != nil {
				g.Expect(c.Create(ctx, infraMachine)).To(Succeed())
			}

			r := &Reconciler{
				Client: c,
			}
			s := &scope{cluster: defaultCluster, machine: tc.machine}
			result, err := r.reconcileInfrastructure(ctx, s)
			r.reconcilePhase(ctx, tc.machine)
			g.Expect(result).To(BeComparableTo(tc.expectResult))
			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			if tc.expected != nil {
				tc.expected(g, tc.machine)
			}
		})
	}
}

func TestReconcileCertificateExpiry(t *testing.T) {
	fakeTimeString := "2020-01-01T00:00:00Z"
	fakeTime, _ := time.Parse(time.RFC3339, fakeTimeString)
	fakeMetaTime := &metav1.Time{Time: fakeTime}

	fakeTimeString2 := "2020-02-02T00:00:00Z"
	fakeTime2, _ := time.Parse(time.RFC3339, fakeTimeString2)
	fakeMetaTime2 := &metav1.Time{Time: fakeTime2}

	bootstrapConfigWithExpiry := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericBootstrapConfig",
			"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"name":      "bootstrap-config-with-expiry",
				"namespace": metav1.NamespaceDefault,
				"annotations": map[string]interface{}{
					clusterv1.MachineCertificatesExpiryDateAnnotation: fakeTimeString,
				},
			},
			"spec": map[string]interface{}{},
			"status": map[string]interface{}{
				"ready":          true,
				"dataSecretName": "secret-data",
			},
		},
	}

	bootstrapConfigWithoutExpiry := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericBootstrapConfig",
			"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"name":      "bootstrap-config-without-expiry",
				"namespace": metav1.NamespaceDefault,
			},
			"spec": map[string]interface{}{},
			"status": map[string]interface{}{
				"ready":          true,
				"dataSecretName": "secret-data",
			},
		},
	}

	tests := []struct {
		name            string
		machine         *clusterv1.Machine
		bootstrapConfig *unstructured.Unstructured
		expected        func(g *WithT, m *clusterv1.Machine)
	}{
		{
			name: "worker machine with certificate expiry annotation should not update expiry date",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: metav1.NamespaceDefault,
					Annotations: map[string]string{
						clusterv1.MachineCertificatesExpiryDateAnnotation: fakeTimeString,
					},
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{},
				},
			},
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.CertificatesExpiryDate).To(BeNil())
			},
		},
		{
			name: "control plane machine with no bootstrap config and no certificate expiry annotation should not set expiry date",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.MachineControlPlaneLabel: "",
					},
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{},
				},
			},
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.CertificatesExpiryDate).To(BeNil())
			},
		},
		{
			name: "control plane machine with bootstrap config and no certificate expiry annotation should not set expiry date",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.MachineControlPlaneLabel: "",
					},
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
							Kind:       "GenericBootstrapConfig",
							Name:       "bootstrap-config-without-expiry",
						},
					},
				},
			},
			bootstrapConfig: bootstrapConfigWithoutExpiry,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.CertificatesExpiryDate).To(BeNil())
			},
		},
		{
			name: "control plane machine with certificate expiry annotation in bootstrap config should set expiry date",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.MachineControlPlaneLabel: "",
					},
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
							Kind:       "GenericBootstrapConfig",
							Name:       "bootstrap-config-with-expiry",
						},
					},
				},
			},
			bootstrapConfig: bootstrapConfigWithExpiry,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.CertificatesExpiryDate).To(Equal(fakeMetaTime))
			},
		},
		{
			name: "control plane machine with certificate expiry annotation and no certificate expiry annotation on bootstrap config should set expiry date",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.MachineControlPlaneLabel: "",
					},
					Annotations: map[string]string{
						clusterv1.MachineCertificatesExpiryDateAnnotation: fakeTimeString,
					},
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
							Kind:       "GenericBootstrapConfig",
							Name:       "bootstrap-config-without-expiry",
						},
					},
				},
			},
			bootstrapConfig: bootstrapConfigWithoutExpiry,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.CertificatesExpiryDate).To(Equal(fakeMetaTime))
			},
		},
		{
			name: "control plane machine with certificate expiry annotation in machine should take precedence over bootstrap config and should set expiry date",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.MachineControlPlaneLabel: "",
					},
					Annotations: map[string]string{
						clusterv1.MachineCertificatesExpiryDateAnnotation: fakeTimeString2,
					},
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
							Kind:       "GenericBootstrapConfig",
							Name:       "bootstrap-config-with-expiry",
						},
					},
				},
			},
			bootstrapConfig: bootstrapConfigWithExpiry,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.CertificatesExpiryDate).To(Equal(fakeMetaTime2))
			},
		},
		{
			name: "reset certificates expiry information in machine status if the information is not available on the machine and the bootstrap config",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						clusterv1.MachineControlPlaneLabel: "",
					},
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
							Kind:       "GenericBootstrapConfig",
							Name:       "bootstrap-config-without-expiry",
						},
					},
				},
				Status: clusterv1.MachineStatus{
					CertificatesExpiryDate: fakeMetaTime,
				},
			},
			bootstrapConfig: bootstrapConfigWithoutExpiry,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.CertificatesExpiryDate).To(BeNil())
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			r := &Reconciler{}
			s := &scope{machine: tc.machine, bootstrapConfig: tc.bootstrapConfig}
			_, _ = r.reconcileCertificateExpiry(ctx, s)
			if tc.expected != nil {
				tc.expected(g, tc.machine)
			}
		})
	}
}
