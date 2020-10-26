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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func init() {
	externalReadyWait = 1 * time.Second
}

var _ = Describe("Reconcile Machine Phases", func() {
	deletionTimestamp := metav1.Now()

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
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.MachineControlPlaneLabelName: "",
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: defaultCluster.Name,
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha3",
					Kind:       "BootstrapMachine",
					Name:       "bootstrap-config1",
				},
			},
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
				Kind:       "InfrastructureMachine",
				Name:       "infra-config1",
			},
		},
	}

	defaultBootstrap := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "BootstrapMachine",
			"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
			"metadata": map[string]interface{}{
				"name":      "bootstrap-config1",
				"namespace": "default",
			},
			"spec":   map[string]interface{}{},
			"status": map[string]interface{}{},
		},
	}

	defaultInfra := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "InfrastructureMachine",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha3",
			"metadata": map[string]interface{}{
				"name":      "infra-config1",
				"namespace": "default",
			},
			"spec":   map[string]interface{}{},
			"status": map[string]interface{}{},
		},
	}

	BeforeEach(func() {
		defaultKubeconfigSecret = kubeconfig.GenerateSecret(defaultCluster, kubeconfig.FromEnvTestConfig(&rest.Config{}, defaultCluster))
	})

	It("Should set OwnerReference and cluster name label on external objects", func() {
		machine := defaultMachine.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		r := &MachineReconciler{
			Client: fake.NewFakeClientWithScheme(scheme.Scheme,
				defaultCluster,
				defaultKubeconfigSecret,
				machine,
				external.TestGenericBootstrapCRD.DeepCopy(),
				external.TestGenericInfrastructureCRD.DeepCopy(),
				bootstrapConfig,
				infraConfig,
			),
			Log:    log.Log,
			scheme: scheme.Scheme,
		}

		res, err := r.reconcile(context.Background(), defaultCluster, machine)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.RequeueAfter).To(Equal(externalReadyWait))

		r.reconcilePhase(context.Background(), machine)

		Expect(r.Client.Get(ctx, types.NamespacedName{Name: bootstrapConfig.GetName(), Namespace: bootstrapConfig.GetNamespace()}, bootstrapConfig)).To(Succeed())

		Expect(bootstrapConfig.GetOwnerReferences()).To(HaveLen(1))
		Expect(bootstrapConfig.GetLabels()[clusterv1.ClusterLabelName]).To(BeEquivalentTo("test-cluster"))

		Expect(r.Client.Get(ctx, types.NamespacedName{Name: infraConfig.GetName(), Namespace: infraConfig.GetNamespace()}, infraConfig)).To(Succeed())

		Expect(infraConfig.GetOwnerReferences()).To(HaveLen(1))
		Expect(infraConfig.GetLabels()[clusterv1.ClusterLabelName]).To(BeEquivalentTo("test-cluster"))
	})

	It("Should set `Pending` with a new Machine", func() {
		machine := defaultMachine.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		r := &MachineReconciler{
			Client: fake.NewFakeClientWithScheme(scheme.Scheme,
				defaultCluster,
				defaultKubeconfigSecret,
				machine,
				external.TestGenericBootstrapCRD.DeepCopy(),
				external.TestGenericInfrastructureCRD.DeepCopy(),
				bootstrapConfig,
				infraConfig,
			),
			Log:    log.Log,
			scheme: scheme.Scheme,
		}

		res, err := r.reconcile(context.Background(), defaultCluster, machine)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.RequeueAfter).To(Equal(externalReadyWait))

		r.reconcilePhase(context.Background(), machine)
		Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhasePending))

		// LastUpdated should be set as the phase changes
		Expect(machine.Status.LastUpdated).ToNot(BeNil())
	})

	It("Should set `Provisioning` when bootstrap is ready", func() {
		machine := defaultMachine.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Set bootstrap ready.
		err := unstructured.SetNestedField(bootstrapConfig.Object, true, "status", "ready")
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		Expect(err).NotTo(HaveOccurred())

		// Set the LastUpdated to be able to verify it is updated when the phase changes
		lastUpdated := metav1.NewTime(time.Now().Add(-10 * time.Second))
		machine.Status.LastUpdated = &lastUpdated

		r := &MachineReconciler{
			Client: fake.NewFakeClientWithScheme(scheme.Scheme,
				defaultCluster,
				defaultKubeconfigSecret,
				machine,
				external.TestGenericBootstrapCRD.DeepCopy(),
				external.TestGenericInfrastructureCRD.DeepCopy(),
				bootstrapConfig,
				infraConfig,
			),
			Log:    log.Log,
			scheme: scheme.Scheme,
		}

		res, err := r.reconcile(context.Background(), defaultCluster, machine)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(context.Background(), machine)
		Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseProvisioning))

		// Verify that the LastUpdated timestamp was updated
		Expect(machine.Status.LastUpdated).ToNot(BeNil())
		Expect(machine.Status.LastUpdated.After(lastUpdated.Time)).To(BeTrue())
	})

	It("Should set `Running` when bootstrap and infra is ready", func() {
		machine := defaultMachine.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Set bootstrap ready.
		err := unstructured.SetNestedField(bootstrapConfig.Object, true, "status", "ready")
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		Expect(err).NotTo(HaveOccurred())

		// Set infra ready.
		err = unstructured.SetNestedField(infraConfig.Object, true, "status", "ready")
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, "test://id-1", "spec", "providerID")
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, "us-east-2a", "spec", "failureDomain")
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, []interface{}{
			map[string]interface{}{
				"type":    "InternalIP",
				"address": "10.0.0.1",
			},
			map[string]interface{}{
				"type":    "InternalIP",
				"address": "10.0.0.2",
			},
		}, "status", "addresses")
		Expect(err).NotTo(HaveOccurred())

		// Set NodeRef.
		machine.Status.NodeRef = &corev1.ObjectReference{Kind: "Node", Name: "machine-test-node"}

		// Set the LastUpdated to be able to verify it is updated when the phase changes
		lastUpdated := metav1.NewTime(time.Now().Add(-10 * time.Second))
		machine.Status.LastUpdated = &lastUpdated

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-test-node",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: corev1.NodeSpec{ProviderID: "test://id-1"},
		}
		cl := fake.NewFakeClientWithScheme(scheme.Scheme,
			defaultCluster,
			machine,
			node,
			external.TestGenericBootstrapCRD.DeepCopy(),
			external.TestGenericInfrastructureCRD.DeepCopy(),
			bootstrapConfig,
			infraConfig,
			defaultKubeconfigSecret,
		)
		r := &MachineReconciler{
			Client:  cl,
			Tracker: remote.NewTestClusterCacheTracker(cl, scheme.Scheme, client.ObjectKey{Name: defaultCluster.Name, Namespace: defaultCluster.Namespace}),
			Log:     log.Log,
			scheme:  scheme.Scheme,
		}

		res, err := r.reconcile(context.Background(), defaultCluster, machine)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Requeue).To(BeFalse())
		Expect(machine.Status.Addresses).To(HaveLen(2))
		Expect(*machine.Spec.FailureDomain).To(Equal("us-east-2a"))

		r.reconcilePhase(context.Background(), machine)
		Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseRunning))

		// Verify that the LastUpdated timestamp was updated
		Expect(machine.Status.LastUpdated).ToNot(BeNil())
		Expect(machine.Status.LastUpdated.After(lastUpdated.Time)).To(BeTrue())
	})

	It("Should set `Running` when bootstrap and infra is ready with no Status.Addresses", func() {
		machine := defaultMachine.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Set bootstrap ready.
		err := unstructured.SetNestedField(bootstrapConfig.Object, true, "status", "ready")
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		Expect(err).NotTo(HaveOccurred())

		// Set infra ready.
		err = unstructured.SetNestedField(infraConfig.Object, true, "status", "ready")
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, "test://id-1", "spec", "providerID")
		Expect(err).NotTo(HaveOccurred())

		// Set NodeRef.
		machine.Status.NodeRef = &corev1.ObjectReference{Kind: "Node", Name: "machine-test-node"}

		// Set the LastUpdated to be able to verify it is updated when the phase changes
		lastUpdated := metav1.NewTime(time.Now().Add(-10 * time.Second))
		machine.Status.LastUpdated = &lastUpdated

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-test-node",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: corev1.NodeSpec{ProviderID: "test://id-1"},
		}
		cl := fake.NewFakeClientWithScheme(scheme.Scheme,
			defaultCluster,
			machine,
			node,
			external.TestGenericBootstrapCRD.DeepCopy(),
			external.TestGenericInfrastructureCRD.DeepCopy(),
			bootstrapConfig,
			infraConfig,
			defaultKubeconfigSecret,
		)
		r := &MachineReconciler{
			Client:  cl,
			Tracker: remote.NewTestClusterCacheTracker(cl, scheme.Scheme, client.ObjectKey{Name: defaultCluster.Name, Namespace: defaultCluster.Namespace}),
			Log:     log.Log,
			scheme:  scheme.Scheme,
		}

		res, err := r.reconcile(context.Background(), defaultCluster, machine)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Requeue).To(BeFalse())
		Expect(machine.Status.Addresses).To(HaveLen(0))

		r.reconcilePhase(context.Background(), machine)
		Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseRunning))

		// Verify that the LastUpdated timestamp was updated
		Expect(machine.Status.LastUpdated).ToNot(BeNil())
		Expect(machine.Status.LastUpdated.After(lastUpdated.Time)).To(BeTrue())
	})

	It("Should set `Running` when bootstrap, infra, and NodeRef is ready", func() {
		machine := defaultMachine.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Set bootstrap ready.
		err := unstructured.SetNestedField(bootstrapConfig.Object, true, "status", "ready")
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		Expect(err).NotTo(HaveOccurred())

		// Set infra ready.
		err = unstructured.SetNestedField(infraConfig.Object, "test://id-1", "spec", "providerID")
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, true, "status", "ready")
		Expect(err).NotTo(HaveOccurred())

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
		Expect(err).NotTo(HaveOccurred())

		// Set NodeRef.
		machine.Status.NodeRef = &corev1.ObjectReference{Kind: "Node", Name: "machine-test-node"}

		// Set the LastUpdated to be able to verify it is updated when the phase changes
		lastUpdated := metav1.NewTime(time.Now().Add(-10 * time.Second))
		machine.Status.LastUpdated = &lastUpdated
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-test-node",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: corev1.NodeSpec{ProviderID: "test://id-1"},
		}
		cl := fake.NewFakeClientWithScheme(scheme.Scheme,
			defaultCluster,
			machine,
			node,
			external.TestGenericBootstrapCRD.DeepCopy(),
			external.TestGenericInfrastructureCRD.DeepCopy(),
			bootstrapConfig,
			infraConfig,
			defaultKubeconfigSecret,
		)
		r := &MachineReconciler{
			Client:  cl,
			Tracker: remote.NewTestClusterCacheTracker(cl, scheme.Scheme, client.ObjectKey{Name: defaultCluster.Name, Namespace: defaultCluster.Namespace}),
			Log:     log.Log,
			scheme:  scheme.Scheme,
		}

		res, err := r.reconcile(context.Background(), defaultCluster, machine)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(context.Background(), machine)
		Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseRunning))

		// Verify that the LastUpdated timestamp was updated
		Expect(machine.Status.LastUpdated).ToNot(BeNil())
		Expect(machine.Status.LastUpdated.After(lastUpdated.Time)).To(BeTrue())
	})

	It("Should set `Provisioned` when there is a NodeRef but infra is not ready ", func() {
		machine := defaultMachine.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Set bootstrap ready.
		err := unstructured.SetNestedField(bootstrapConfig.Object, true, "status", "ready")
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		Expect(err).NotTo(HaveOccurred())

		// Set NodeRef.
		machine.Status.NodeRef = &corev1.ObjectReference{Kind: "Node", Name: "machine-test-node"}

		// Set the LastUpdated to be able to verify it is updated when the phase changes
		lastUpdated := metav1.NewTime(time.Now().Add(-10 * time.Second))
		machine.Status.LastUpdated = &lastUpdated

		r := &MachineReconciler{
			Client: fake.NewFakeClientWithScheme(scheme.Scheme,
				defaultCluster,
				defaultKubeconfigSecret,
				machine,
				external.TestGenericBootstrapCRD.DeepCopy(),
				external.TestGenericInfrastructureCRD.DeepCopy(),
				bootstrapConfig,
				infraConfig,
			),
			Log:    log.Log,
			scheme: scheme.Scheme,
		}

		res, err := r.reconcile(context.Background(), defaultCluster, machine)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.RequeueAfter).To(Equal(externalReadyWait))

		r.reconcilePhase(context.Background(), machine)
		Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseProvisioned))

		// Verify that the LastUpdated timestamp was updated
		Expect(machine.Status.LastUpdated).ToNot(BeNil())
		Expect(machine.Status.LastUpdated.After(lastUpdated.Time)).To(BeTrue())
	})

	It("Should set `Deleting` when Machine is being deleted", func() {
		machine := defaultMachine.DeepCopy()
		// Need the second Machine to allow deletion of one.
		machineSecond := defaultMachine.DeepCopy()

		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		// Set bootstrap ready.
		err := unstructured.SetNestedField(bootstrapConfig.Object, true, "status", "ready")
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(bootstrapConfig.Object, "secret-data", "status", "dataSecretName")
		Expect(err).NotTo(HaveOccurred())

		// Set infra ready.
		err = unstructured.SetNestedField(infraConfig.Object, "test://id-1", "spec", "providerID")
		Expect(err).NotTo(HaveOccurred())

		err = unstructured.SetNestedField(infraConfig.Object, true, "status", "ready")
		Expect(err).NotTo(HaveOccurred())

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
		Expect(err).NotTo(HaveOccurred())

		// Set Cluster label.
		machine.Labels[clusterv1.ClusterLabelName] = machine.Spec.ClusterName
		machine.ResourceVersion = "1"
		machineSecond.Labels[clusterv1.ClusterLabelName] = machine.Spec.ClusterName
		machineSecond.Name = "SecondMachine"
		// Set NodeRef.
		machine.Status.NodeRef = &corev1.ObjectReference{Kind: "Node", Name: "machine-test-node"}

		// Set Deletion Timestamp.
		machine.SetDeletionTimestamp(&deletionTimestamp)

		// Set the LastUpdated to be able to verify it is updated when the phase changes
		lastUpdated := metav1.NewTime(time.Now().Add(-10 * time.Second))
		machine.Status.LastUpdated = &lastUpdated

		cl := fake.NewFakeClientWithScheme(scheme.Scheme,
			defaultCluster,
			defaultKubeconfigSecret,
			machine,
			machineSecond,
			external.TestGenericBootstrapCRD.DeepCopy(),
			external.TestGenericInfrastructureCRD.DeepCopy(),
			bootstrapConfig,
			infraConfig,
		)
		r := &MachineReconciler{
			Client:   cl,
			Tracker:  remote.NewTestClusterCacheTracker(cl, scheme.Scheme, client.ObjectKey{Name: defaultCluster.Name, Namespace: defaultCluster.Namespace}),
			Log:      log.Log,
			scheme:   scheme.Scheme,
			recorder: record.NewFakeRecorder(32),
		}

		res, err := r.reconcileDelete(context.Background(), defaultCluster, machine)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(context.Background(), machine)
		Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseDeleting))

		nodeHealthyCondition := conditions.Get(machine, clusterv1.MachineNodeHealthyCondition)
		Expect(nodeHealthyCondition.Status).To(Equal(corev1.ConditionFalse))
		Expect(nodeHealthyCondition.Reason).To(Equal(clusterv1.DeletingReason))

		// Verify that the LastUpdated timestamp was updated
		Expect(machine.Status.LastUpdated).ToNot(BeNil())
		Expect(machine.Status.LastUpdated.After(lastUpdated.Time)).To(BeTrue())
	})
})

func TestReconcileBootstrap(t *testing.T) {
	defaultMachine := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-test",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.ClusterLabelName: "test-cluster",
			},
		},
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha3",
					Kind:       "BootstrapMachine",
					Name:       "bootstrap-config1",
				},
			},
		},
	}

	defaultCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
	}

	testCases := []struct {
		name            string
		bootstrapConfig map[string]interface{}
		machine         *clusterv1.Machine
		expectError     bool
		expected        func(g *WithT, m *clusterv1.Machine)
		result          *ctrl.Result
	}{
		{
			name: "new machine, bootstrap config ready with data",
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapMachine",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "default",
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready":          true,
					"dataSecretName": "secret-data",
				},
			},
			expectError: false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.BootstrapReady).To(BeTrue())
				g.Expect(m.Spec.Bootstrap.DataSecretName).ToNot(BeNil())
				g.Expect(*m.Spec.Bootstrap.DataSecretName).To(ContainSubstring("secret-data"))
			},
		},
		{
			name: "new machine, bootstrap config ready with no data",
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapMachine",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "default",
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready": true,
				},
			},
			expectError: true,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.BootstrapReady).To(BeFalse())
				g.Expect(m.Spec.Bootstrap.Data).To(BeNil())
			},
		},
		{
			name: "new machine, bootstrap config not ready",
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapMachine",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "default",
				},
				"spec":   map[string]interface{}{},
				"status": map[string]interface{}{},
			},
			expectError: false,
			result:      &ctrl.Result{RequeueAfter: externalReadyWait},
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.BootstrapReady).To(BeFalse())
			},
		},
		{
			name: "new machine, bootstrap config is not found",
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapMachine",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "wrong-namespace",
				},
				"spec":   map[string]interface{}{},
				"status": map[string]interface{}{},
			},
			expectError: true,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.BootstrapReady).To(BeFalse())
			},
		},
		{
			name: "new machine, no bootstrap config or data",
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapMachine",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "wrong-namespace",
				},
				"spec":   map[string]interface{}{},
				"status": map[string]interface{}{},
			},
			expectError: true,
		},
		{
			name: "existing machine, bootstrap data should not change",
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapMachine",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "default",
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready":          true,
					"dataSecretName": "secret-data",
				},
			},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: "default",
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha3",
							Kind:       "BootstrapMachine",
							Name:       "bootstrap-config1",
						},
						Data: pointer.StringPtr("#!/bin/bash ... data"),
					},
				},
				Status: clusterv1.MachineStatus{
					BootstrapReady: true,
				},
			},
			expectError: false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.BootstrapReady).To(BeTrue())
				g.Expect(m.Spec.Bootstrap.Data).To(BeNil())
				g.Expect(*m.Spec.Bootstrap.DataSecretName).To(BeEquivalentTo("secret-data"))
			},
		},
		{
			name: "existing machine, bootstrap provider is not ready, and ownerref updated",
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapMachine",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "default",
					"ownerReferences": []interface{}{
						map[string]interface{}{
							"apiVersion": clusterv1.GroupVersion.String(),
							"kind":       "MachineSet",
							"name":       "ms",
							"uid":        "1",
							"controller": true,
						},
					},
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready": false,
				},
			},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: "default",
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha3",
							Kind:       "BootstrapMachine",
							Name:       "bootstrap-config1",
						},
					},
				},
				Status: clusterv1.MachineStatus{
					BootstrapReady: true,
				},
			},
			expectError: false,
			result:      &ctrl.Result{RequeueAfter: externalReadyWait},
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.GetOwnerReferences()).NotTo(ContainRefOfGroupKind("cluster.x-k8s.io", "MachineSet"))
			},
		},
		{
			name: "existing machine, machineset owner and version v1alpha2, and ownerref updated",
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapMachine",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "default",
					"ownerReferences": []interface{}{
						map[string]interface{}{
							"apiVersion": "cluster.x-k8s.io/v1alpha2",
							"kind":       "MachineSet",
							"name":       "ms",
							"uid":        "1",
							"controller": true,
						},
					},
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready": true,
				},
			},
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: "default",
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha2",
							Kind:       "BootstrapMachine",
							Name:       "bootstrap-config1",
						},
					},
				},
				Status: clusterv1.MachineStatus{
					BootstrapReady: true,
				},
			},
			expectError: true,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.GetOwnerReferences()).NotTo(ContainRefOfGroupKind("cluster.x-k8s.io", "MachineSet"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())

			if tc.machine == nil {
				tc.machine = defaultMachine.DeepCopy()
			}

			bootstrapConfig := &unstructured.Unstructured{Object: tc.bootstrapConfig}
			r := &MachineReconciler{
				Client: fake.NewFakeClientWithScheme(scheme.Scheme,
					tc.machine,
					external.TestGenericBootstrapCRD.DeepCopy(),
					external.TestGenericInfrastructureCRD.DeepCopy(),
					bootstrapConfig,
				),
				Log:    log.Log,
				scheme: scheme.Scheme,
			}

			res, err := r.reconcileBootstrap(context.Background(), defaultCluster, tc.machine)
			if tc.expectError {
				g.Expect(err).ToNot(BeNil())
			} else {
				g.Expect(err).To(BeNil())
			}

			if tc.expected != nil {
				tc.expected(g, tc.machine)
			}

			if tc.result != nil {
				g.Expect(res).To(Equal(*tc.result))
			}
		})
	}
}

func TestReconcileInfrastructure(t *testing.T) {
	defaultMachine := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-test",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.ClusterLabelName: "test-cluster",
			},
		},
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha3",
					Kind:       "BootstrapMachine",
					Name:       "bootstrap-config1",
				},
			},
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
				Kind:       "InfrastructureMachine",
				Name:       "infra-config1",
			},
		},
	}

	defaultCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
	}

	testCases := []struct {
		name               string
		bootstrapConfig    map[string]interface{}
		infraConfig        map[string]interface{}
		machine            *clusterv1.Machine
		expectError        bool
		expectChanged      bool
		expectRequeueAfter bool
		expected           func(g *WithT, m *clusterv1.Machine)
	}{
		{
			name: "new machine, infrastructure config ready",
			infraConfig: map[string]interface{}{
				"kind":       "InfrastructureMachine",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha3",
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": "default",
					"ownerReferences": []interface{}{
						map[string]interface{}{
							"apiVersion": clusterv1.GroupVersion.String(),
							"kind":       "MachineSet",
							"name":       "ms",
							"uid":        "1",
							"controller": true,
						},
					},
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
			expectError:   false,
			expectChanged: true,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.InfrastructureReady).To(BeTrue())
				g.Expect(m.GetOwnerReferences()).NotTo(ContainRefOfGroupKind("cluster.x-k8s.io", "MachineSet"))
			},
		},
		{
			name: "ready bootstrap, infra, and nodeRef, machine is running, infra object is deleted, expect failed",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: "default",
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha3",
							Kind:       "BootstrapMachine",
							Name:       "bootstrap-config1",
						},
					},
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
						Kind:       "InfrastructureMachine",
						Name:       "infra-config1",
					},
				},
				Status: clusterv1.MachineStatus{
					BootstrapReady:      true,
					InfrastructureReady: true,
					NodeRef:             &corev1.ObjectReference{Kind: "Node", Name: "machine-test-node"},
				},
			},
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapMachine",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "default",
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready":          true,
					"dataSecretName": "secret-data",
				},
			},
			infraConfig: map[string]interface{}{
				"kind":       "InfrastructureMachine",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha3",
				"metadata":   map[string]interface{}{},
			},
			expectError:        true,
			expectRequeueAfter: true,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.InfrastructureReady).To(BeTrue())
				g.Expect(m.Status.FailureMessage).ToNot(BeNil())
				g.Expect(m.Status.FailureReason).ToNot(BeNil())
				g.Expect(m.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseFailed))
			},
		},
		{
			name: "infrastructure ref is paused",
			infraConfig: map[string]interface{}{
				"kind":       "InfrastructureMachine",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha3",
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": "default",
					"annotations": map[string]interface{}{
						"cluster.x-k8s.io/paused": "true",
					},
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
			expectError:   false,
			expectChanged: false,
			expected: func(g *WithT, m *clusterv1.Machine) {
				g.Expect(m.Status.InfrastructureReady).To(BeFalse())
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())

			if tc.machine == nil {
				tc.machine = defaultMachine.DeepCopy()
			}

			infraConfig := &unstructured.Unstructured{Object: tc.infraConfig}
			r := &MachineReconciler{
				Client: fake.NewFakeClientWithScheme(scheme.Scheme,
					tc.machine,
					external.TestGenericBootstrapCRD.DeepCopy(),
					external.TestGenericInfrastructureCRD.DeepCopy(),
					infraConfig,
				),
				Log:    log.Log,
				scheme: scheme.Scheme,
			}

			_, err := r.reconcileInfrastructure(context.Background(), defaultCluster, tc.machine)
			r.reconcilePhase(context.Background(), tc.machine)
			if tc.expectError {
				g.Expect(err).ToNot(BeNil())
			} else {
				g.Expect(err).To(BeNil())
			}

			if tc.expected != nil {
				tc.expected(g, tc.machine)
			}
		})
	}
}
