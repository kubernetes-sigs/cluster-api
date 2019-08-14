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
	"testing"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/cluster-api/api/v1alpha2"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcilePhase(t *testing.T) {
	deletionTimestamp := metav1.Now()

	defaultMachine := v1alpha2.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-test",
			Namespace: "default",
		},
		Spec: v1alpha2.MachineSpec{
			Bootstrap: v1alpha2.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha2",
					Kind:       "BootstrapConfig",
					Name:       "bootstrap-config1",
				},
			},
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha2",
				Kind:       "InfrastructureConfig",
				Name:       "infra-config1",
			},
		},
	}

	testCases := []struct {
		name               string
		bootstrapConfig    map[string]interface{}
		infraConfig        map[string]interface{}
		machine            *v1alpha2.Machine
		expectError        bool
		expectRequeueAfter bool
		expected           func(g *gomega.WithT, m *v1alpha2.Machine)
	}{
		{
			name: "new machine, expect pending",
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha2",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "default",
				},
				"spec":   map[string]interface{}{},
				"status": map[string]interface{}{},
			},
			infraConfig: map[string]interface{}{
				"kind":       "InfrastructureConfig",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha2",
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": "default",
				},
				"spec":   map[string]interface{}{},
				"status": map[string]interface{}{},
			},
			expectError:        false,
			expectRequeueAfter: true,
			expected: func(g *gomega.WithT, m *v1alpha2.Machine) {
				g.Expect(m.Status.GetTypedPhase()).To(gomega.Equal(v1alpha2.MachinePhasePending))
			},
		},
		{
			name: "ready bootstrap, expect provisioning",
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha2",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "default",
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready":         true,
					"bootstrapData": "...",
				},
			},
			infraConfig: map[string]interface{}{
				"kind":       "InfrastructureConfig",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha2",
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": "default",
				},
				"spec":   map[string]interface{}{},
				"status": map[string]interface{}{},
			},
			expectError:        false,
			expectRequeueAfter: true,
			expected: func(g *gomega.WithT, m *v1alpha2.Machine) {
				g.Expect(m.Status.GetTypedPhase()).To(gomega.Equal(v1alpha2.MachinePhaseProvisioning))
			},
		},
		{
			name: "ready bootstrap and infra, expect provisioned",
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha2",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "default",
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready":         true,
					"bootstrapData": "...",
				},
			},
			infraConfig: map[string]interface{}{
				"kind":       "InfrastructureConfig",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha2",
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
			expectError:        false,
			expectRequeueAfter: false,
			expected: func(g *gomega.WithT, m *v1alpha2.Machine) {
				g.Expect(m.Status.GetTypedPhase()).To(gomega.Equal(v1alpha2.MachinePhaseProvisioned))
				g.Expect(m.Status.Addresses).To(gomega.HaveLen(2))
			},
		},
		{
			name: "ready bootstrap and infra, allow nil addresses as they are optional",
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha2",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "default",
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready":         true,
					"bootstrapData": "...",
				},
			},
			infraConfig: map[string]interface{}{
				"kind":       "InfrastructureConfig",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha2",
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": "default",
				},
				"spec": map[string]interface{}{
					"providerID": "test://id-1",
				},
				"status": map[string]interface{}{
					"ready": true,
				},
			},
			expectError:        false,
			expectRequeueAfter: false,
			expected: func(g *gomega.WithT, m *v1alpha2.Machine) {
				g.Expect(m.Status.GetTypedPhase()).To(gomega.Equal(v1alpha2.MachinePhaseProvisioned))
				g.Expect(m.Status.Addresses).To(gomega.HaveLen(0))
			},
		},
		{
			name: "ready bootstrap, infra, and nodeRef, expect running",
			machine: &v1alpha2.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: "default",
				},
				Spec: v1alpha2.MachineSpec{
					Bootstrap: v1alpha2.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha2",
							Kind:       "BootstrapConfig",
							Name:       "bootstrap-config1",
						},
					},
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha2",
						Kind:       "InfrastructureConfig",
						Name:       "infra-config1",
					},
				},
				Status: v1alpha2.MachineStatus{
					BootstrapReady:      true,
					InfrastructureReady: true,
					NodeRef:             &corev1.ObjectReference{Kind: "Node", Name: "machine-test-node"},
				},
			},
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha2",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "default",
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready":         true,
					"bootstrapData": "...",
				},
			},
			infraConfig: map[string]interface{}{
				"kind":       "InfrastructureConfig",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha2",
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
			expectError:        false,
			expectRequeueAfter: false,
			expected: func(g *gomega.WithT, m *v1alpha2.Machine) {
				g.Expect(m.Status.GetTypedPhase()).To(gomega.Equal(v1alpha2.MachinePhaseRunning))
			},
		},
		{
			name: "ready bootstrap, infra, and nodeRef, machine being deleted, expect deleting",
			machine: &v1alpha2.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "machine-test",
					Namespace:         "default",
					DeletionTimestamp: &deletionTimestamp,
				},
				Spec: v1alpha2.MachineSpec{
					Bootstrap: v1alpha2.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha2",
							Kind:       "BootstrapConfig",
							Name:       "bootstrap-config1",
						},
					},
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha2",
						Kind:       "InfrastructureConfig",
						Name:       "infra-config1",
					},
				},
				Status: v1alpha2.MachineStatus{
					BootstrapReady:      true,
					InfrastructureReady: true,
					NodeRef:             &corev1.ObjectReference{Kind: "Node", Name: "machine-test-node"},
				},
			},
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha2",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "default",
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready":         true,
					"bootstrapData": "...",
				},
			},
			infraConfig: map[string]interface{}{
				"kind":       "InfrastructureConfig",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha2",
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
			expectError:        false,
			expectRequeueAfter: false,
			expected: func(g *gomega.WithT, m *v1alpha2.Machine) {
				g.Expect(m.Status.GetTypedPhase()).To(gomega.Equal(v1alpha2.MachinePhaseDeleting))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := gomega.NewGomegaWithT(t)

			if tc.machine == nil {
				tc.machine = defaultMachine.DeepCopy()
			}

			bootstrapConfig := &unstructured.Unstructured{Object: tc.bootstrapConfig}
			infraConfig := &unstructured.Unstructured{Object: tc.infraConfig}
			r := &ReconcileMachine{
				Client: fake.NewFakeClient(tc.machine, bootstrapConfig, infraConfig),
				scheme: scheme.Scheme,
			}

			err := r.reconcile(context.Background(), nil, tc.machine)
			if tc.expectError {
				g.Expect(err).ToNot(gomega.BeNil())
			} else if tc.expectRequeueAfter {
				g.Expect(capierrors.IsRequeueAfter(err)).To(gomega.BeTrue())
			} else if !tc.expectError {
				g.Expect(err).To(gomega.BeNil())
			}

			if tc.expected != nil {
				tc.expected(g, tc.machine)
			}

			// Test that externalWatchers detected the new kinds.
			if tc.machine.DeletionTimestamp.IsZero() {
				_, ok := r.externalWatchers.Load(bootstrapConfig.GroupVersionKind().String())
				g.Expect(ok).To(gomega.BeTrue())
				_, ok = r.externalWatchers.Load(infraConfig.GroupVersionKind().String())
				g.Expect(ok).To(gomega.BeTrue())
			}
		})

	}

}

func TestReconcileBootstrap(t *testing.T) {
	defaultMachine := v1alpha2.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-test",
			Namespace: "default",
			Labels: map[string]string{
				v1alpha2.MachineClusterLabelName: "test-cluster",
			},
		},
		Spec: v1alpha2.MachineSpec{
			Bootstrap: v1alpha2.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha2",
					Kind:       "BootstrapConfig",
					Name:       "bootstrap-config1",
				},
			},
		},
	}

	testCases := []struct {
		name            string
		bootstrapConfig map[string]interface{}
		machine         *v1alpha2.Machine
		expectError     bool
		expected        func(g *gomega.WithT, m *v1alpha2.Machine)
	}{
		{
			name: "new machine, bootstrap config ready with data",
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha2",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "default",
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready":         true,
					"bootstrapData": "#!/bin/bash ... data",
				},
			},
			expectError: false,
			expected: func(g *gomega.WithT, m *v1alpha2.Machine) {
				g.Expect(m.Status.BootstrapReady).To(gomega.BeTrue())
				g.Expect(m.Spec.Bootstrap.Data).ToNot(gomega.BeNil())
				g.Expect(*m.Spec.Bootstrap.Data).To(gomega.ContainSubstring("#!/bin/bash"))
			},
		},
		{
			name: "new machine, bootstrap config ready with no data",
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha2",
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
			expected: func(g *gomega.WithT, m *v1alpha2.Machine) {
				g.Expect(m.Status.BootstrapReady).To(gomega.BeFalse())
				g.Expect(m.Spec.Bootstrap.Data).To(gomega.BeNil())
			},
		},
		{
			name: "new machine, bootstrap config not ready",
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha2",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "default",
				},
				"spec":   map[string]interface{}{},
				"status": map[string]interface{}{},
			},
			expectError: true,
			expected: func(g *gomega.WithT, m *v1alpha2.Machine) {
				g.Expect(m.Status.BootstrapReady).To(gomega.BeFalse())
			},
		},
		{
			name: "new machine, bootstrap config is not found",
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha2",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "wrong-namespace",
				},
				"spec":   map[string]interface{}{},
				"status": map[string]interface{}{},
			},
			expectError: true,
			expected: func(g *gomega.WithT, m *v1alpha2.Machine) {
				g.Expect(m.Status.BootstrapReady).To(gomega.BeFalse())
			},
		},
		{
			name: "new machine, no bootstrap config or data",
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha2",
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
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha2",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": "default",
				},
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"ready": true,
					"data":  "#!/bin/bash ... data with change",
				},
			},
			machine: &v1alpha2.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: "default",
				},
				Spec: v1alpha2.MachineSpec{
					Bootstrap: v1alpha2.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha2",
							Kind:       "BootstrapConfig",
							Name:       "bootstrap-config1",
						},
						Data: pointer.StringPtr("#!/bin/bash ... data"),
					},
				},
				Status: v1alpha2.MachineStatus{
					BootstrapReady: true,
				},
			},
			expectError: false,
			expected: func(g *gomega.WithT, m *v1alpha2.Machine) {
				g.Expect(m.Status.BootstrapReady).To(gomega.BeTrue())
				g.Expect(*m.Spec.Bootstrap.Data).To(gomega.Equal("#!/bin/bash ... data"))
			},
		},
		{
			name: "existing machine, bootstrap provider is to not ready",
			bootstrapConfig: map[string]interface{}{
				"kind":       "BootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha2",
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
			machine: &v1alpha2.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bootstrap-test-existing",
					Namespace: "default",
				},
				Spec: v1alpha2.MachineSpec{
					Bootstrap: v1alpha2.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha2",
							Kind:       "BootstrapConfig",
							Name:       "bootstrap-config1",
						},
						Data: pointer.StringPtr("#!/bin/bash ... data"),
					},
				},
				Status: v1alpha2.MachineStatus{
					BootstrapReady: true,
				},
			},
			expectError: false,
			expected: func(g *gomega.WithT, m *v1alpha2.Machine) {
				g.Expect(m.Status.BootstrapReady).To(gomega.BeTrue())
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := gomega.NewGomegaWithT(t)

			if tc.machine == nil {
				tc.machine = defaultMachine.DeepCopy()
			}

			bootstrapConfig := &unstructured.Unstructured{Object: tc.bootstrapConfig}
			r := &ReconcileMachine{
				Client: fake.NewFakeClient(tc.machine, bootstrapConfig),
				scheme: scheme.Scheme,
			}

			err := r.reconcileBootstrap(context.Background(), tc.machine)
			if tc.expectError {
				g.Expect(err).ToNot(gomega.BeNil())
			} else {
				g.Expect(err).To(gomega.BeNil())
			}

			if tc.expected != nil {
				tc.expected(g, tc.machine)
			}
		})

	}

}

func TestReconcileInfrastructure(t *testing.T) {
	defaultMachine := v1alpha2.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-test",
			Namespace: "default",
			Labels: map[string]string{
				v1alpha2.MachineClusterLabelName: "test-cluster",
			},
		},
		Spec: v1alpha2.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha2",
				Kind:       "InfrastructureConfig",
				Name:       "infra-config1",
			},
		},
	}

	testCases := []struct {
		name          string
		infraConfig   map[string]interface{}
		machine       *v1alpha2.Machine
		expectError   bool
		expectChanged bool
		expected      func(g *gomega.WithT, m *v1alpha2.Machine)
	}{
		{
			name: "new machine, infrastructure config ready",
			infraConfig: map[string]interface{}{
				"kind":       "InfrastructureConfig",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha2",
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
			expected: func(g *gomega.WithT, m *v1alpha2.Machine) {
				g.Expect(m.Status.InfrastructureReady).To(gomega.BeTrue())
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := gomega.NewGomegaWithT(t)

			if tc.machine == nil {
				tc.machine = defaultMachine.DeepCopy()
			}

			infraConfig := &unstructured.Unstructured{Object: tc.infraConfig}
			r := &ReconcileMachine{
				Client: fake.NewFakeClient(tc.machine, infraConfig),
				scheme: scheme.Scheme,
			}

			err := r.reconcileInfrastructure(context.Background(), tc.machine)
			if tc.expectError {
				g.Expect(err).ToNot(gomega.BeNil())
			} else {
				g.Expect(err).To(gomega.BeNil())
			}

			if tc.expected != nil {
				tc.expected(g, tc.machine)
			}
		})

	}

}
