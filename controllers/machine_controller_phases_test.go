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
	dto "github.com/prometheus/client_model/go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
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
					Kind:       "BootstrapConfig",
					Name:       "bootstrap-config1",
				},
			},
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
				Kind:       "InfrastructureConfig",
				Name:       "infra-config1",
			},
		},
	}

	defaultBootstrap := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "BootstrapConfig",
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
			"kind":       "InfrastructureConfig",
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
		defaultKubeconfigSecret = kubeconfig.GenerateSecret(defaultCluster, kubeconfig.FromEnvTestConfig(cfg, defaultCluster))
	})

	It("Should set OwnerReference and cluster name label on external objects", func() {
		machine := defaultMachine.DeepCopy()
		bootstrapConfig := defaultBootstrap.DeepCopy()
		infraConfig := defaultInfra.DeepCopy()

		r := &MachineReconciler{
			Client: fake.NewFakeClientWithScheme(scheme.Scheme, defaultCluster, defaultKubeconfigSecret, machine, bootstrapConfig, infraConfig),
			Log:    log.Log,
		}

		res, err := r.reconcile(context.Background(), defaultCluster, machine)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Requeue).To(BeTrue())

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
			Client: fake.NewFakeClientWithScheme(scheme.Scheme, defaultCluster, defaultKubeconfigSecret, machine, bootstrapConfig, infraConfig),
			Log:    log.Log,
		}

		res, err := r.reconcile(context.Background(), defaultCluster, machine)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Requeue).To(BeTrue())

		r.reconcilePhase(context.Background(), machine)
		Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhasePending))
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

		r := &MachineReconciler{
			Client: fake.NewFakeClientWithScheme(scheme.Scheme, defaultCluster, defaultKubeconfigSecret, machine, bootstrapConfig, infraConfig),
			Log:    log.Log,
		}

		res, err := r.reconcile(context.Background(), defaultCluster, machine)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Requeue).To(BeTrue())

		r.reconcilePhase(context.Background(), machine)
		Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseProvisioning))
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

		r := &MachineReconciler{
			Client: fake.NewFakeClientWithScheme(scheme.Scheme, defaultCluster, defaultKubeconfigSecret, machine, bootstrapConfig, infraConfig),
			Log:    log.Log,
		}

		res, err := r.reconcile(context.Background(), defaultCluster, machine)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Requeue).To(BeFalse())
		Expect(machine.Status.Addresses).To(HaveLen(2))

		r.reconcilePhase(context.Background(), machine)
		Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseRunning))
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

		r := &MachineReconciler{
			Client: fake.NewFakeClientWithScheme(scheme.Scheme, defaultCluster, defaultKubeconfigSecret, machine, bootstrapConfig, infraConfig),
			Log:    log.Log,
		}

		res, err := r.reconcile(context.Background(), defaultCluster, machine)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Requeue).To(BeFalse())
		Expect(machine.Status.Addresses).To(HaveLen(0))

		r.reconcilePhase(context.Background(), machine)
		Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseRunning))
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

		r := &MachineReconciler{
			Client: fake.NewFakeClientWithScheme(scheme.Scheme, defaultCluster, defaultKubeconfigSecret, machine, bootstrapConfig, infraConfig),
			Log:    log.Log,
		}

		res, err := r.reconcile(context.Background(), defaultCluster, machine)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(context.Background(), machine)
		Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseRunning))
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

		r := &MachineReconciler{
			Client: fake.NewFakeClientWithScheme(scheme.Scheme, defaultCluster, defaultKubeconfigSecret, machine, bootstrapConfig, infraConfig),
			Log:    log.Log,
		}

		res, err := r.reconcile(context.Background(), defaultCluster, machine)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Requeue).To(BeTrue())

		r.reconcilePhase(context.Background(), machine)
		Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseProvisioned))
	})

	It("Should set `Deleting` when Machine is being deleted", func() {
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

		// Set Deletion Timestamp.
		machine.SetDeletionTimestamp(&deletionTimestamp)

		r := &MachineReconciler{
			Client: fake.NewFakeClientWithScheme(scheme.Scheme, defaultCluster, defaultKubeconfigSecret, machine, bootstrapConfig, infraConfig),
			Log:    log.Log,
		}

		res, err := r.reconcile(context.Background(), defaultCluster, machine)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Requeue).To(BeFalse())

		r.reconcilePhase(context.Background(), machine)
		Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseDeleting))
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
					Kind:       "BootstrapConfig",
					Name:       "bootstrap-config1",
				},
			},
		},
	}

	testCases := []struct {
		name            string
		bootstrapConfig map[string]interface{}
		machine         *clusterv1.Machine
		expectError     bool
		expected        func(g *WithT, m *clusterv1.Machine)
	}{
		{
			name: "new machine, bootstrap config ready with data",
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapConfig",
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
				"kind":       "BootstrapConfig",
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
				"kind":       "BootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "default",
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
			name: "new machine, bootstrap config is not found",
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapConfig",
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
				"kind":       "BootstrapConfig",
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
				"kind":       "BootstrapConfig",
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
							Kind:       "BootstrapConfig",
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
				g.Expect(*m.Spec.Bootstrap.Data).To(Equal("#!/bin/bash ... data"))
			},
		},
		{
			name: "existing machine, bootstrap provider is to not ready",
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "default",
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready": false,
					"data":  "#!/bin/bash ... data",
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
							Kind:       "BootstrapConfig",
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
				Client: fake.NewFakeClientWithScheme(scheme.Scheme, tc.machine, bootstrapConfig),
				Log:    log.Log,
			}

			err := r.reconcileBootstrap(context.Background(), tc.machine)
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
					Kind:       "BootstrapConfig",
					Name:       "bootstrap-config1",
				},
			},
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
				Kind:       "InfrastructureConfig",
				Name:       "infra-config1",
			},
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
				"kind":       "InfrastructureConfig",
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
							Kind:       "BootstrapConfig",
							Name:       "bootstrap-config1",
						},
					},
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
						Kind:       "InfrastructureConfig",
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
				"kind":       "BootstrapConfig",
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
				"kind":       "InfrastructureConfig",
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
				Client: fake.NewFakeClientWithScheme(scheme.Scheme, tc.machine, infraConfig),
				Log:    log.Log,
			}

			err := r.reconcileInfrastructure(context.Background(), tc.machine)
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

func getMetricFamily(list []*dto.MetricFamily, metricName string) *dto.MetricFamily {
	for _, mf := range list {
		if mf.GetName() == metricName {
			return mf
		}
	}
	return nil
}
