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
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
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
					Namespace:  metav1.NamespaceDefault,
				},
			},
		},
	}

	testCases := []struct {
		name                      string
		machine                   *clusterv1.Machine
		bootstrapConfig           *unstructured.Unstructured
		bootstrapConfigIsNotFound bool
		expectCondition           metav1.Condition
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
			bootstrapConfigIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineBootstrapDataSecretProvidedV1Beta2Reason,
			},
		},
		{
			name:    "mirror Ready condition from bootstrap config (true)",
			machine: defaultMachine.DeepCopy(),
			bootstrapConfig: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "Ready",
							"status": "True",
							// reason not set for v1beta1 conditions
							"message": "some message",
						},
					},
				},
			}},
			bootstrapConfigIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineBootstrapConfigReadyV1Beta2Reason, // reason fixed up
				Message: "some message",
			},
		},
		{
			name:    "mirror Ready condition from bootstrap config",
			machine: defaultMachine.DeepCopy(),
			bootstrapConfig: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":    "Ready",
							"status":  "False",
							"reason":  "SomeReason",
							"message": "some message",
						},
					},
				},
			}},
			bootstrapConfigIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  "SomeReason",
				Message: "some message",
			},
		},
		{
			name:    "Use status.BoostrapReady flag as a fallback Ready condition from bootstrap config is missing",
			machine: defaultMachine.DeepCopy(),
			bootstrapConfig: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"status": map[string]interface{}{},
			}},
			bootstrapConfigIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineBootstrapConfigNotReadyV1Beta2Reason,
				Message: "GenericBootstrapConfig status.ready is false",
			},
		},
		{
			name: "Use status.BoostrapReady flag as a fallback Ready condition from bootstrap config is missing (ready true)",
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.Status.BootstrapReady = true
				return m
			}(),
			bootstrapConfig: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"status": map[string]interface{}{
					"ready": true,
				},
			}},
			bootstrapConfigIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineBootstrapConfigReadyV1Beta2Reason,
			},
		},
		{
			name:    "invalid Ready condition from bootstrap config",
			machine: defaultMachine.DeepCopy(),
			bootstrapConfig: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type": "Ready",
						},
					},
				},
			}},
			bootstrapConfigIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineBootstrapConfigInvalidConditionReportedV1Beta2Reason,
				Message: "failed to convert status.conditions from GenericBootstrapConfig to []metav1.Condition: status must be set for the Ready condition",
			},
		},
		{
			name:                      "failed to get bootstrap config",
			machine:                   defaultMachine.DeepCopy(),
			bootstrapConfig:           nil,
			bootstrapConfigIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineBootstrapConfigInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name: "bootstrap config that was ready not found while machine is deleting",
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.Status.BootstrapReady = true
				m.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
				return m
			}(),
			bootstrapConfig:           nil,
			bootstrapConfigIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineBootstrapConfigDeletedV1Beta2Reason,
				Message: "GenericBootstrapConfig has been deleted",
			},
		},
		{
			name: "bootstrap config not found while machine is deleting",
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
				return m
			}(),
			bootstrapConfig:           nil,
			bootstrapConfigIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineBootstrapConfigDoesNotExistV1Beta2Reason,
				Message: "GenericBootstrapConfig does not exist",
			},
		},
		{
			name:                      "bootstrap config not found",
			machine:                   defaultMachine.DeepCopy(),
			bootstrapConfig:           nil,
			bootstrapConfigIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineBootstrapConfigDoesNotExistV1Beta2Reason,
				Message: "GenericBootstrapConfig does not exist",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			setBootstrapReadyCondition(ctx, tc.machine, tc.bootstrapConfig, tc.bootstrapConfigIsNotFound)

			condition := v1beta2conditions.Get(tc.machine, clusterv1.MachineBootstrapConfigReadyV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tc.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetInfrastructureReadyCondition(t *testing.T) {
	defaultMachine := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-test",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "GenericInfrastructureMachine",
				Name:       "infra-machine1",
				Namespace:  metav1.NamespaceDefault,
			},
		},
	}

	testCases := []struct {
		name                   string
		machine                *clusterv1.Machine
		infraMachine           *unstructured.Unstructured
		infraMachineIsNotFound bool
		expectCondition        metav1.Condition
	}{
		{
			name:    "mirror Ready condition from infra machine (true)",
			machine: defaultMachine.DeepCopy(),
			infraMachine: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "infra-machine1",
					"namespace": metav1.NamespaceDefault,
				},
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "Ready",
							"status": "True",
							// reason not set for v1beta1 conditions
							"message": "some message",
						},
					},
				},
			}},
			infraMachineIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineInfrastructureReadyV1Beta2Reason, // reason fixed up
				Message: "some message",
			},
		},
		{
			name:    "mirror Ready condition from infra machine",
			machine: defaultMachine.DeepCopy(),
			infraMachine: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "infra-machine1",
					"namespace": metav1.NamespaceDefault,
				},
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":    "Ready",
							"status":  "False",
							"reason":  "SomeReason",
							"message": "some message",
						},
					},
				},
			}},
			infraMachineIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  "SomeReason",
				Message: "some message",
			},
		},
		{
			name:    "Use status.InfrastructureReady flag as a fallback Ready condition from infra machine is missing",
			machine: defaultMachine.DeepCopy(),
			infraMachine: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "infra-machine1",
					"namespace": metav1.NamespaceDefault,
				},
				"status": map[string]interface{}{},
			}},
			infraMachineIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineInfrastructureNotReadyV1Beta2Reason,
				Message: "GenericInfrastructureMachine status.ready is false",
			},
		},
		{
			name: "Use status.InfrastructureReady flag as a fallback Ready condition from infra machine is missing (ready true)",
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.Status.InfrastructureReady = true
				return m
			}(),
			infraMachine: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "infra-machine1",
					"namespace": metav1.NamespaceDefault,
				},
				"status": map[string]interface{}{
					"ready": true,
				},
			}},
			infraMachineIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineInfrastructureReadyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineInfrastructureReadyV1Beta2Reason,
			},
		},
		{
			name:    "invalid Ready condition from infra machine",
			machine: defaultMachine.DeepCopy(),
			infraMachine: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "infra-machine1",
					"namespace": metav1.NamespaceDefault,
				},
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type": "Ready",
						},
					},
				},
			}},
			infraMachineIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineInfrastructureInvalidConditionReportedV1Beta2Reason,
				Message: "failed to convert status.conditions from GenericInfrastructureMachine to []metav1.Condition: status must be set for the Ready condition",
			},
		},
		{
			name: "failed to get infra machine",
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
				return m
			}(),
			infraMachine:           nil,
			infraMachineIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineInfrastructureInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name: "infra machine that was ready not found while machine is deleting",
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
				m.Status.InfrastructureReady = true
				return m
			}(),
			infraMachine:           nil,
			infraMachineIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineInfrastructureDeletedV1Beta2Reason,
				Message: "GenericInfrastructureMachine has been deleted",
			},
		},
		{
			name: "infra machine not found while machine is deleting",
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
				return m
			}(),
			infraMachine:           nil,
			infraMachineIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineInfrastructureDoesNotExistV1Beta2Reason,
				Message: "GenericInfrastructureMachine does not exist",
			},
		},
		{
			name: "infra machine not found after the machine has been initialized",
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.Status.InfrastructureReady = true
				return m
			}(),
			infraMachine:           nil,
			infraMachineIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineInfrastructureDeletedV1Beta2Reason,
				Message: "GenericInfrastructureMachine has been deleted while the Machine still exists",
			},
		},
		{
			name:                   "infra machine not found",
			machine:                defaultMachine.DeepCopy(),
			infraMachine:           nil,
			infraMachineIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineInfrastructureDoesNotExistV1Beta2Reason,
				Message: "GenericInfrastructureMachine does not exist",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			setInfrastructureReadyCondition(ctx, tc.machine, tc.infraMachine, tc.infraMachineIsNotFound)

			condition := v1beta2conditions.Get(tc.machine, clusterv1.MachineInfrastructureReadyV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tc.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSummarizeNodeV1Beta2Conditions(t *testing.T) {
	testCases := []struct {
		name            string
		conditions      []corev1.NodeCondition
		expectedStatus  metav1.ConditionStatus
		expectedReason  string
		expectedMessage string
	}{
		{
			name: "node is healthy",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodePIDPressure, Status: corev1.ConditionFalse},
			},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: clusterv1.MachineNodeHealthyV1Beta2Reason,
		},
		{
			name: "all conditions are unknown",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionUnknown, Message: "Node is not reporting status"},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionUnknown, Message: "Node is not reporting status"},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionUnknown, Message: "Node is not reporting status"},
				{Type: corev1.NodePIDPressure, Status: corev1.ConditionUnknown, Message: "Node is not reporting status"},
			},
			expectedStatus:  metav1.ConditionUnknown,
			expectedReason:  clusterv1.MachineNodeHealthUnknownV1Beta2Reason,
			expectedMessage: "* Node.AllConditions: Node is not reporting status",
		},
		{
			name: "multiple semantically failed condition",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionUnknown, Message: "Node is not reporting status"},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue, Message: "kubelet has NOT sufficient memory available"},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue, Message: "kubelet has disk pressure"},
				{Type: corev1.NodePIDPressure, Status: corev1.ConditionTrue, Message: "kubelet has NOT sufficient PID available"},
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: clusterv1.MachineNodeNotHealthyV1Beta2Reason,
			expectedMessage: "* Node.Ready: Node is not reporting status\n" +
				"* Node.MemoryPressure: kubelet has NOT sufficient memory available\n" +
				"* Node.DiskPressure: kubelet has disk pressure\n" +
				"* Node.PIDPressure: kubelet has NOT sufficient PID available",
		},
		{
			name: "one semantically failed condition when the rest is healthy",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Reason: "SomeReason", Message: "kubelet is NOT ready"},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse, Message: "kubelet has sufficient memory available"},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse, Message: "kubelet has no disk pressure"},
				{Type: corev1.NodePIDPressure, Status: corev1.ConditionFalse, Message: "kubelet has sufficient PID available"},
			},
			expectedStatus:  metav1.ConditionFalse,
			expectedReason:  clusterv1.MachineNodeNotHealthyV1Beta2Reason,
			expectedMessage: "* Node.Ready: kubelet is NOT ready",
		},
		{
			name: "one unknown condition when the rest is healthy",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionUnknown, Reason: "SomeReason", Message: "Node is not reporting status"},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse, Message: "kubelet has sufficient memory available"},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse, Message: "kubelet has no disk pressure"},
				{Type: corev1.NodePIDPressure, Status: corev1.ConditionFalse, Message: "kubelet has sufficient PID available"},
			},
			expectedStatus:  metav1.ConditionUnknown,
			expectedReason:  clusterv1.MachineNodeHealthUnknownV1Beta2Reason,
			expectedMessage: "* Node.Ready: Node is not reporting status",
		},
		{
			name: "one condition missing",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse, Message: "kubelet has sufficient memory available"},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse, Message: "kubelet has no disk pressure"},
				{Type: corev1.NodePIDPressure, Status: corev1.ConditionFalse, Message: "kubelet has sufficient PID available"},
			},
			expectedStatus:  metav1.ConditionUnknown,
			expectedReason:  clusterv1.MachineNodeHealthUnknownV1Beta2Reason,
			expectedMessage: "* Node.Ready: Condition not yet reported",
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
				},
				Status: corev1.NodeStatus{
					Conditions: test.conditions,
				},
			}
			status, reason, message := summarizeNodeV1Beta2Conditions(ctx, node)
			g.Expect(status).To(Equal(test.expectedStatus))
			g.Expect(reason).To(Equal(test.expectedReason))
			g.Expect(message).To(Equal(test.expectedMessage))
		})
	}
}

func TestSetNodeHealthyAndReadyConditions(t *testing.T) {
	now := time.Now()

	defaultMachine := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-test",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "GenericInfrastructureMachine",
				Name:       "infra-machine1",
				Namespace:  metav1.NamespaceDefault,
			},
		},
	}

	defaultCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-test",
			Namespace: metav1.NamespaceDefault,
		},
		Status: clusterv1.ClusterStatus{
			InfrastructureReady: true,
			Conditions: clusterv1.Conditions{
				{Type: clusterv1.ControlPlaneInitializedCondition, Status: corev1.ConditionTrue,
					LastTransitionTime: metav1.Time{Time: now.Add(-5 * time.Second)}},
			},
		},
	}

	testCases := []struct {
		name                 string
		cluster              *clusterv1.Cluster
		machine              *clusterv1.Machine
		node                 *corev1.Node
		nodeGetErr           error
		lastProbeSuccessTime time.Time
		expectConditions     []metav1.Condition
	}{
		{
			name: "Cluster status.infrastructureReady is false",
			cluster: func() *clusterv1.Cluster {
				c := defaultCluster.DeepCopy()
				c.Status.InfrastructureReady = false
				return c
			}(),
			machine: defaultMachine.DeepCopy(),
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeInspectionFailedV1Beta2Reason,
					Message: "Waiting for Cluster status.infrastructureReady to be true",
				},
				{
					Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeInspectionFailedV1Beta2Reason,
					Message: "Waiting for Cluster status.infrastructureReady to be true",
				},
			},
		},
		{
			name: "Cluster control plane is not initialized",
			cluster: func() *clusterv1.Cluster {
				c := defaultCluster.DeepCopy()
				conditions.MarkFalse(c, clusterv1.ControlPlaneInitializedCondition, "", clusterv1.ConditionSeverityError, "")
				return c
			}(),
			machine: defaultMachine.DeepCopy(),
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeInspectionFailedV1Beta2Reason,
					Message: "Waiting for Cluster control plane to be initialized",
				},
				{
					Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeInspectionFailedV1Beta2Reason,
					Message: "Waiting for Cluster control plane to be initialized",
				},
			},
		},
		{
			name:    "get NodeHealthy and NodeReady from node",
			cluster: defaultCluster,
			machine: defaultMachine.DeepCopy(),
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Reason: "SomeReason", Message: "kubelet is NOT ready"},
						{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
						{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse},
						{Type: corev1.NodePIDPressure, Status: corev1.ConditionFalse},
					},
				},
			},
			lastProbeSuccessTime: now,
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineNodeNotHealthyV1Beta2Reason,
					Message: "* Node.Ready: kubelet is NOT ready",
				},
				{
					Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineNodeNotReadyV1Beta2Reason,
					Message: "* Node.Ready: kubelet is NOT ready",
				},
			},
		},
		{
			name:    "get NodeHealthy and NodeReady from node (Node is ready & healthy)",
			cluster: defaultCluster,
			machine: defaultMachine.DeepCopy(),
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue, Reason: "KubeletReady", Message: "kubelet is posting ready status"},
						{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
						{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse},
						{Type: corev1.NodePIDPressure, Status: corev1.ConditionFalse},
					},
				},
			},
			lastProbeSuccessTime: now,
			expectConditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineNodeHealthyV1Beta2Condition,
				},
				{
					Type:   clusterv1.MachineNodeReadyV1Beta2Condition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineNodeReadyV1Beta2Reason,
				},
			},
		},
		{
			name:    "NodeReady missing from node",
			cluster: defaultCluster,
			machine: defaultMachine.DeepCopy(),
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
						{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse},
						{Type: corev1.NodePIDPressure, Status: corev1.ConditionFalse},
					},
				},
			},
			lastProbeSuccessTime: now,
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeHealthUnknownV1Beta2Reason,
					Message: "* Node.Ready: Condition not yet reported",
				},
				{
					Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeReadyUnknownV1Beta2Reason,
					Message: "* Node.Ready: Condition not yet reported",
				},
			},
		},
		{
			name:    "node that existed not found while machine is deleting",
			cluster: defaultCluster,
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
				m.Status.NodeRef = &corev1.ObjectReference{
					Name: "test-node-1",
				}
				return m
			}(),
			node:                 nil,
			lastProbeSuccessTime: now,
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineNodeDeletedV1Beta2Reason,
					Message: "Node test-node-1 has been deleted",
				},
				{
					Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineNodeDeletedV1Beta2Reason,
					Message: "Node test-node-1 has been deleted",
				},
			},
		},
		{
			name:    "node not found while machine is deleting",
			cluster: defaultCluster,
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
				return m
			}(),
			node:                 nil,
			lastProbeSuccessTime: now,
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeDoesNotExistV1Beta2Reason,
					Message: "Node does not exist",
				},
				{
					Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeDoesNotExistV1Beta2Reason,
					Message: "Node does not exist",
				},
			},
		},
		{
			name:    "node missing while machine is still running",
			cluster: defaultCluster,
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.Status.NodeRef = &corev1.ObjectReference{
					Name: "test-node-1",
				}
				return m
			}(),
			node:                 nil,
			lastProbeSuccessTime: now,
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineNodeDeletedV1Beta2Reason,
					Message: "Node test-node-1 has been deleted while the Machine still exists",
				},
				{
					Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineNodeDeletedV1Beta2Reason,
					Message: "Node test-node-1 has been deleted while the Machine still exists",
				},
			},
		},
		{
			name:    "machine with ProviderID set, Node still missing",
			cluster: defaultCluster,
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.Spec.ProviderID = ptr.To("foo://test-node-1")
				return m
			}(),
			node:                 nil,
			lastProbeSuccessTime: now,
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeInspectionFailedV1Beta2Reason,
					Message: "Waiting for a Node with spec.providerID foo://test-node-1 to exist",
				},
				{
					Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeInspectionFailedV1Beta2Reason,
					Message: "Waiting for a Node with spec.providerID foo://test-node-1 to exist",
				},
			},
		},
		{
			name:                 "machine with ProviderID not yet set, waiting for it",
			cluster:              defaultCluster,
			machine:              defaultMachine.DeepCopy(),
			node:                 nil,
			lastProbeSuccessTime: now,
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeInspectionFailedV1Beta2Reason,
					Message: "Waiting for GenericInfrastructureMachine to report spec.providerID",
				},
				{
					Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeInspectionFailedV1Beta2Reason,
					Message: "Waiting for GenericInfrastructureMachine to report spec.providerID",
				},
			},
		},
		{
			name: "connection down, preserve conditions as they have been set before (remote conditions grace period not passed yet)",
			cluster: func() *clusterv1.Cluster {
				c := defaultCluster.DeepCopy()
				for i, condition := range c.Status.Conditions {
					if condition.Type == clusterv1.ControlPlaneInitializedCondition {
						c.Status.Conditions[i].LastTransitionTime.Time = now.Add(-4 * time.Minute)
					}
				}
				return c
			}(),
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				v1beta2conditions.Set(m, metav1.Condition{
					Type:   clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineNodeHealthyV1Beta2Reason,
				})
				v1beta2conditions.Set(m, metav1.Condition{
					Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineNodeReadyV1Beta2Reason,
					Message: "kubelet is posting ready status",
				})
				return m
			}(),
			node:                 nil,
			nodeGetErr:           errors.Wrapf(clustercache.ErrClusterNotConnected, "error getting client"),
			lastProbeSuccessTime: now.Add(-3 * time.Minute),
			// Conditions have not been updated.
			// remoteConditionsGracePeriod is 5m
			// control plane is initialized since 4m ago, last probe success was 3m ago.
			expectConditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineNodeHealthyV1Beta2Reason,
				},
				{
					Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineNodeReadyV1Beta2Reason,
					Message: "kubelet is posting ready status",
				},
			},
		},
		{
			name: "connection down, set conditions as they haven't been set before (remote conditions grace period not passed yet)",
			cluster: func() *clusterv1.Cluster {
				c := defaultCluster.DeepCopy()
				for i, condition := range c.Status.Conditions {
					if condition.Type == clusterv1.ControlPlaneInitializedCondition {
						c.Status.Conditions[i].LastTransitionTime.Time = now.Add(-4 * time.Minute)
					}
				}
				return c
			}(),
			machine:              defaultMachine.DeepCopy(),
			node:                 nil,
			nodeGetErr:           errors.Wrapf(clustercache.ErrClusterNotConnected, "error getting client"),
			lastProbeSuccessTime: now.Add(-3 * time.Minute),
			// Conditions have been set.
			// remoteConditionsGracePeriod is 5m
			// control plane is initialized since 4m ago, last probe success was 3m ago.
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeConnectionDownV1Beta2Reason,
					Message: fmt.Sprintf("Last successful probe at %s", now.Add(-3*time.Minute).Format(time.RFC3339)),
				},
				{
					Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeConnectionDownV1Beta2Reason,
					Message: fmt.Sprintf("Last successful probe at %s", now.Add(-3*time.Minute).Format(time.RFC3339)),
				},
			},
		},
		{
			name: "connection down, set conditions to unknown (remote conditions grace period passed)",
			cluster: func() *clusterv1.Cluster {
				c := defaultCluster.DeepCopy()
				for i, condition := range c.Status.Conditions {
					if condition.Type == clusterv1.ControlPlaneInitializedCondition {
						c.Status.Conditions[i].LastTransitionTime.Time = now.Add(-7 * time.Minute)
					}
				}
				return c
			}(),
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				v1beta2conditions.Set(m, metav1.Condition{
					Type:   clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineNodeHealthyV1Beta2Reason,
				})
				v1beta2conditions.Set(m, metav1.Condition{
					Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineNodeReadyV1Beta2Reason,
					Message: "kubelet is posting ready status",
				})
				return m
			}(),
			node:                 nil,
			nodeGetErr:           errors.Wrapf(clustercache.ErrClusterNotConnected, "error getting client"),
			lastProbeSuccessTime: now.Add(-6 * time.Minute),
			// Conditions have been updated.
			// remoteConditionsGracePeriod is 5m
			// control plane is initialized since 7m ago, last probe success was 6m ago.
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeConnectionDownV1Beta2Reason,
					Message: fmt.Sprintf("Last successful probe at %s", now.Add(-6*time.Minute).Format(time.RFC3339)),
				},
				{
					Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeConnectionDownV1Beta2Reason,
					Message: fmt.Sprintf("Last successful probe at %s", now.Add(-6*time.Minute).Format(time.RFC3339)),
				},
			},
		},
		{
			name:                 "internal error occurred when trying to get Node",
			cluster:              defaultCluster,
			machine:              defaultMachine.DeepCopy(),
			nodeGetErr:           errors.Errorf("error creating watch machine-watchNodes for *v1.Node"),
			lastProbeSuccessTime: now,
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeInternalErrorV1Beta2Reason,
					Message: "Please check controller logs for errors",
				},
				{
					Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeInternalErrorV1Beta2Reason,
					Message: "Please check controller logs for errors",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			setNodeHealthyAndReadyConditions(ctx, tc.cluster, tc.machine, tc.node, tc.nodeGetErr, tc.lastProbeSuccessTime, 5*time.Minute)
			g.Expect(tc.machine.GetV1Beta2Conditions()).To(v1beta2conditions.MatchConditions(tc.expectConditions, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestDeletingCondition(t *testing.T) {
	testCases := []struct {
		name                    string
		machine                 *clusterv1.Machine
		reconcileDeleteExecuted bool
		deletingReason          string
		deletingMessage         string
		expectCondition         metav1.Condition
	}{
		{
			name: "deletionTimestamp not set",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
			},
			reconcileDeleteExecuted: false,
			deletingReason:          "",
			deletingMessage:         "",
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeletingV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineNotDeletingV1Beta2Reason,
			},
		},
		{
			name: "deletionTimestamp set (waiting for pre-drain hooks)",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "machine-test",
					Namespace:         metav1.NamespaceDefault,
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
				},
			},
			reconcileDeleteExecuted: true,
			deletingReason:          clusterv1.MachineDeletingWaitingForPreDrainHookV1Beta2Reason,
			deletingMessage:         "Waiting for pre-drain hooks to succeed (hooks: test-hook)",
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeletingV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeletingWaitingForPreDrainHookV1Beta2Reason,
				Message: "Waiting for pre-drain hooks to succeed (hooks: test-hook)",
			},
		},
		{
			name: "deletionTimestamp set (waiting for Node drain)",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "machine-test",
					Namespace:         metav1.NamespaceDefault,
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
				},
			},
			reconcileDeleteExecuted: true,
			deletingReason:          clusterv1.MachineDeletingDrainingNodeV1Beta2Reason,
			deletingMessage: `Drain not completed yet (started at 2024-10-09T16:13:59Z):
* Pods with deletionTimestamp that still exist: pod-2-deletionTimestamp-set-1, pod-2-deletionTimestamp-set-2, pod-2-deletionTimestamp-set-3, pod-3-to-trigger-eviction-successfully-1, pod-3-to-trigger-eviction-successfully-2, ... (2 more)
* Pods with eviction failed:
  * Cannot evict pod as it would violate the pod's disruption budget. The disruption budget pod-5-pdb needs 20 healthy pods and has 20 currently: pod-5-to-trigger-eviction-pdb-violated-1, pod-5-to-trigger-eviction-pdb-violated-2, pod-5-to-trigger-eviction-pdb-violated-3, ... (3 more)
  * some other error 1: pod-6-to-trigger-eviction-some-other-error
  * some other error 2: pod-7-to-trigger-eviction-some-other-error
  * some other error 3: pod-8-to-trigger-eviction-some-other-error
  * some other error 4: pod-9-to-trigger-eviction-some-other-error
  * ... (1 more error applying to 1 Pod)`,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineDeletingV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineDeletingDrainingNodeV1Beta2Reason,
				Message: `Drain not completed yet (started at 2024-10-09T16:13:59Z):
* Pods with deletionTimestamp that still exist: pod-2-deletionTimestamp-set-1, pod-2-deletionTimestamp-set-2, pod-2-deletionTimestamp-set-3, pod-3-to-trigger-eviction-successfully-1, pod-3-to-trigger-eviction-successfully-2, ... (2 more)
* Pods with eviction failed:
  * Cannot evict pod as it would violate the pod's disruption budget. The disruption budget pod-5-pdb needs 20 healthy pods and has 20 currently: pod-5-to-trigger-eviction-pdb-violated-1, pod-5-to-trigger-eviction-pdb-violated-2, pod-5-to-trigger-eviction-pdb-violated-3, ... (3 more)
  * some other error 1: pod-6-to-trigger-eviction-some-other-error
  * some other error 2: pod-7-to-trigger-eviction-some-other-error
  * some other error 3: pod-8-to-trigger-eviction-some-other-error
  * some other error 4: pod-9-to-trigger-eviction-some-other-error
  * ... (1 more error applying to 1 Pod)`,
			},
		},
		{
			name: "deletionTimestamp set, reconcileDelete not executed",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "machine-test",
					Namespace:         metav1.NamespaceDefault,
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
				},
				Status: clusterv1.MachineStatus{
					V1Beta2: &clusterv1.MachineV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:    clusterv1.MachineDeletingV1Beta2Condition,
								Status:  metav1.ConditionTrue,
								Reason:  clusterv1.MachineDeletingWaitingForPreDrainHookV1Beta2Reason,
								Message: "Waiting for pre-drain hooks to succeed (hooks: test-hook)",
							},
						},
					},
				},
			},
			reconcileDeleteExecuted: false,
			deletingReason:          "",
			deletingMessage:         "",
			// Condition was not updated because reconcileDelete was not executed.
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeletingV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeletingWaitingForPreDrainHookV1Beta2Reason,
				Message: "Waiting for pre-drain hooks to succeed (hooks: test-hook)",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			setDeletingCondition(ctx, tc.machine, tc.reconcileDeleteExecuted, tc.deletingReason, tc.deletingMessage)

			deletingCondition := v1beta2conditions.Get(tc.machine, clusterv1.MachineDeletingV1Beta2Condition)
			g.Expect(deletingCondition).ToNot(BeNil())
			g.Expect(*deletingCondition).To(v1beta2conditions.MatchCondition(tc.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestTransformControlPlaneAndEtcdConditions(t *testing.T) {
	testCases := []struct {
		name           string
		messages       []string
		expectMessages []string
	}{
		{
			name: "no-op without control plane conditions",
			messages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyV1Beta2Condition),
				fmt.Sprintf("* %s: Foo", clusterv1.InfrastructureReadyV1Beta2Condition),
				fmt.Sprintf("* %s: Foo", clusterv1.MachineNodeHealthyV1Beta2Condition),
				fmt.Sprintf("* %s: Foo", clusterv1.MachineDeletingV1Beta2Condition),
				fmt.Sprintf("* %s: Foo", clusterv1.MachineHealthCheckSucceededV1Beta2Condition),
			},
			expectMessages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyV1Beta2Condition),
				fmt.Sprintf("* %s: Foo", clusterv1.InfrastructureReadyV1Beta2Condition),
				fmt.Sprintf("* %s: Foo", clusterv1.MachineNodeHealthyV1Beta2Condition),
				fmt.Sprintf("* %s: Foo", clusterv1.MachineDeletingV1Beta2Condition),
				fmt.Sprintf("* %s: Foo", clusterv1.MachineHealthCheckSucceededV1Beta2Condition),
			},
		},
		{
			name: "group control plane conditions when msg are all equal",
			messages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyV1Beta2Condition),
				"* APIServerSomething: Waiting for AWSMachine to report spec.providerID",
				"* ControllerManagerSomething: Waiting for AWSMachine to report spec.providerID",
				"* SchedulerSomething: Waiting for AWSMachine to report spec.providerID",
				"* EtcdSomething: Waiting for AWSMachine to report spec.providerID",
			},
			expectMessages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyV1Beta2Condition),
				"* Control plane components: Waiting for AWSMachine to report spec.providerID",
				"* EtcdSomething: Waiting for AWSMachine to report spec.providerID",
			},
		},
		{
			name: "don't group control plane conditions when msg are not all equal",
			messages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyV1Beta2Condition),
				"* APIServerSomething: Waiting for AWSMachine to report spec.providerID",
				"* ControllerManagerSomething: Some message",
				"* SchedulerSomething: Waiting for AWSMachine to report spec.providerID",
				"* EtcdSomething:Some message",
			},
			expectMessages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyV1Beta2Condition),
				"* APIServerSomething: Waiting for AWSMachine to report spec.providerID",
				"* ControllerManagerSomething: Some message",
				"* SchedulerSomething: Waiting for AWSMachine to report spec.providerID",
				"* EtcdSomething:Some message",
			},
		},
		{
			name: "don't group control plane conditions when there is only one condition",
			messages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyV1Beta2Condition),
				"* APIServerSomething: Waiting for AWSMachine to report spec.providerID",
				"* EtcdSomething:Some message",
			},
			expectMessages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyV1Beta2Condition),
				"* APIServerSomething: Waiting for AWSMachine to report spec.providerID",
				"* EtcdSomething:Some message",
			},
		},
		{
			name: "Treat EtcdPodHealthy as a special case (it groups with control plane components)",
			messages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyV1Beta2Condition),
				"* APIServerSomething: Waiting for AWSMachine to report spec.providerID",
				"* ControllerManagerSomething: Waiting for AWSMachine to report spec.providerID",
				"* SchedulerSomething: Waiting for AWSMachine to report spec.providerID",
				"* EtcdPodHealthy: Waiting for AWSMachine to report spec.providerID",
				"* EtcdSomething: Some message",
			},
			expectMessages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyV1Beta2Condition),
				"* Control plane components: Waiting for AWSMachine to report spec.providerID",
				"* EtcdSomething: Some message",
			},
		},
		{
			name: "Treat EtcdPodHealthy as a special case (it does not group with other etcd conditions)",
			messages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyV1Beta2Condition),
				"* APIServerSomething: Waiting for AWSMachine to report spec.providerID",
				"* ControllerManagerSomething: Waiting for AWSMachine to report spec.providerID",
				"* SchedulerSomething: Waiting for AWSMachine to report spec.providerID",
				"* EtcdPodHealthy: Some message",
				"* EtcdSomething: Some message",
			},
			expectMessages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyV1Beta2Condition),
				"* APIServerSomething: Waiting for AWSMachine to report spec.providerID",
				"* ControllerManagerSomething: Waiting for AWSMachine to report spec.providerID",
				"* SchedulerSomething: Waiting for AWSMachine to report spec.providerID",
				"* EtcdPodHealthy: Some message",
				"* EtcdSomething: Some message",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			got := transformControlPlaneAndEtcdConditions(tc.messages)
			g.Expect(got).To(Equal(tc.expectMessages))
		})
	}
}

func TestSetReadyCondition(t *testing.T) {
	testCases := []struct {
		name            string
		machine         *clusterv1.Machine
		expectCondition metav1.Condition
	}{
		{
			name: "Accepts HealthCheckSucceeded to be missing",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Status: clusterv1.MachineStatus{
					V1Beta2: &clusterv1.MachineV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.InfrastructureReadyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.MachineNodeHealthyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.MachineDeletingV1Beta2Condition,
								Status: metav1.ConditionFalse,
								Reason: clusterv1.MachineNotDeletingV1Beta2Reason,
							},
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineReadyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineReadyV1Beta2Reason,
			},
		},
		{
			name: "Tolerates BootstrapConfig, InfraMachine and Node do not exists while the machine is deleting (waiting for pre-drain hooks)",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "machine-test",
					Namespace:         metav1.NamespaceDefault,
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
				},
				Status: clusterv1.MachineStatus{
					V1Beta2: &clusterv1.MachineV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
								Status: metav1.ConditionFalse,
								Reason: clusterv1.MachineBootstrapConfigDoesNotExistV1Beta2Reason,
							},
							{
								Type:   clusterv1.InfrastructureReadyV1Beta2Condition,
								Status: metav1.ConditionFalse,
								Reason: clusterv1.MachineInfrastructureDeletedV1Beta2Reason,
							},
							{
								Type:   clusterv1.MachineNodeHealthyV1Beta2Condition,
								Status: metav1.ConditionFalse,
								Reason: clusterv1.MachineNodeDeletedV1Beta2Reason,
							},
							{
								Type:    clusterv1.MachineDeletingV1Beta2Condition,
								Status:  metav1.ConditionTrue,
								Reason:  clusterv1.MachineDeletingWaitingForPreDrainHookV1Beta2Reason,
								Message: "Waiting for pre-drain hooks to succeed (hooks: test-hook)",
							},
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineNotReadyV1Beta2Reason,
				Message: "* Deleting: Machine deletion in progress, stage: WaitingForPreDrainHook",
			},
		},
		{
			name: "Aggregates Ready condition correctly while the machine is deleting (waiting for Node drain)",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "machine-test",
					Namespace:         metav1.NamespaceDefault,
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
				},
				Status: clusterv1.MachineStatus{
					V1Beta2: &clusterv1.MachineV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.InfrastructureReadyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.MachineNodeHealthyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.MachineDeletingV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: clusterv1.MachineDeletingDrainingNodeV1Beta2Reason,
								Message: `Drain not completed yet (started at 2024-10-09T16:13:59Z):
* Pods with deletionTimestamp that still exist: pod-2-deletionTimestamp-set-1, pod-2-deletionTimestamp-set-2, pod-2-deletionTimestamp-set-3, pod-3-to-trigger-eviction-successfully-1, pod-3-to-trigger-eviction-successfully-2, ... (2 more)
* Pods with eviction failed:
  * Cannot evict pod as it would violate the pod's disruption budget. The disruption budget pod-5-pdb needs 20 healthy pods and has 20 currently: pod-5-to-trigger-eviction-pdb-violated-1, pod-5-to-trigger-eviction-pdb-violated-2, pod-5-to-trigger-eviction-pdb-violated-3, ... (3 more)
  * some other error 1: pod-6-to-trigger-eviction-some-other-error
  * some other error 2: pod-7-to-trigger-eviction-some-other-error
  * some other error 3: pod-8-to-trigger-eviction-some-other-error
  * some other error 4: pod-9-to-trigger-eviction-some-other-error
  * ... (1 more error applying to 1 Pod)`,
							},
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineNotReadyV1Beta2Reason,
				Message: "* Deleting: Machine deletion in progress, stage: DrainingNode",
			},
		},
		{
			name: "Takes into account HealthCheckSucceeded when it exists",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Status: clusterv1.MachineStatus{
					V1Beta2: &clusterv1.MachineV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.InfrastructureReadyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.MachineNodeHealthyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:    clusterv1.MachineHealthCheckSucceededV1Beta2Condition,
								Status:  metav1.ConditionFalse,
								Reason:  "SomeReason",
								Message: "Some message",
							},
							{
								Type:   clusterv1.MachineDeletingV1Beta2Condition,
								Status: metav1.ConditionFalse,
								Reason: clusterv1.MachineNotDeletingV1Beta2Reason,
							},
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineNotReadyV1Beta2Reason,
				Message: "* HealthCheckSucceeded: Some message",
			},
		},
		{
			name: "Takes into account Readiness gates when defined",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineSpec{
					ReadinessGates: []clusterv1.MachineReadinessGate{
						{ConditionType: "MyReadinessGate"},
					},
				},
				Status: clusterv1.MachineStatus{
					V1Beta2: &clusterv1.MachineV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.InfrastructureReadyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.MachineNodeHealthyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.MachineHealthCheckSucceededV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:    "MyReadinessGate",
								Status:  metav1.ConditionFalse,
								Reason:  "SomeReason",
								Message: "Some message",
							},
							{
								Type:   clusterv1.MachineDeletingV1Beta2Condition,
								Status: metav1.ConditionFalse,
								Reason: clusterv1.MachineNotDeletingV1Beta2Reason,
							},
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineNotReadyV1Beta2Reason,
				Message: "* MyReadinessGate: Some message",
			},
		},
		{
			name: "Groups readiness gates for control plane components and etcd member when possible and there is more than one condition for each category",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.MachineSpec{
					ReadinessGates: []clusterv1.MachineReadinessGate{
						{ConditionType: "APIServerSomething"},
						{ConditionType: "ControllerManagerSomething"},
						{ConditionType: "EtcdSomething"},
					},
				},
				Status: clusterv1.MachineStatus{
					V1Beta2: &clusterv1.MachineV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.InfrastructureReadyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.MachineNodeHealthyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:   clusterv1.MachineHealthCheckSucceededV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: "Foo",
							},
							{
								Type:    "APIServerSomething",
								Status:  metav1.ConditionFalse,
								Reason:  "SomeReason",
								Message: "Some control plane message",
							},
							{
								Type:    "ControllerManagerSomething",
								Status:  metav1.ConditionFalse,
								Reason:  "SomeReason",
								Message: "Some control plane message",
							},
							{
								Type:    "EtcdSomething",
								Status:  metav1.ConditionFalse,
								Reason:  "SomeReason",
								Message: "Some etcd message",
							},
							{
								Type:   clusterv1.MachineDeletingV1Beta2Condition,
								Status: metav1.ConditionFalse,
								Reason: clusterv1.MachineNotDeletingV1Beta2Reason,
							},
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineReadyV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineNotReadyV1Beta2Reason,
				Message: "* Control plane components: Some control plane message\n" +
					"* EtcdSomething: Some etcd message",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			setReadyCondition(ctx, tc.machine)

			condition := v1beta2conditions.Get(tc.machine, clusterv1.MachineReadyV1Beta2Condition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(v1beta2conditions.MatchCondition(tc.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestCalculateDeletingConditionForSummary(t *testing.T) {
	testCases := []struct {
		name            string
		machine         *clusterv1.Machine
		expectCondition v1beta2conditions.ConditionWithOwnerInfo
	}{
		{
			name: "No Deleting condition",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Status: clusterv1.MachineStatus{
					V1Beta2: &clusterv1.MachineV1Beta2Status{
						Conditions: []metav1.Condition{},
					},
				},
			},
			expectCondition: v1beta2conditions.ConditionWithOwnerInfo{
				OwnerResource: v1beta2conditions.ConditionOwnerInfo{
					Kind: "Machine",
					Name: "machine-test",
				},
				Condition: metav1.Condition{
					Type:    clusterv1.MachineDeletingV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineDeletingV1Beta2Reason,
					Message: "Machine deletion in progress",
				},
			},
		},
		{
			name: "Deleting condition with DrainingNode since more than 15m",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "machine-test",
					Namespace:         metav1.NamespaceDefault,
					DeletionTimestamp: &metav1.Time{Time: time.Now().Add(-16 * time.Minute)},
				},
				Status: clusterv1.MachineStatus{
					V1Beta2: &clusterv1.MachineV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.MachineDeletingV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: clusterv1.MachineDeletingDrainingNodeV1Beta2Reason,
								Message: `Drain not completed yet (started at 2024-10-09T16:13:59Z):
* Pods pod-2-deletionTimestamp-set-1, pod-3-to-trigger-eviction-successfully-1: deletionTimestamp set, but still not removed from the Node
* Pod pod-5-to-trigger-eviction-pdb-violated-1: cannot evict pod as it would violate the pod's disruption budget. The disruption budget pod-5-pdb needs 20 healthy pods and has 20 currently
* Pod pod-6-to-trigger-eviction-some-other-error: failed to evict Pod, some other error 1
After above Pods have been removed from the Node, the following Pods will be evicted: pod-7-eviction-later, pod-8-eviction-later`,
							},
						},
					},
					Deletion: &clusterv1.MachineDeletionStatus{
						NodeDrainStartTime: &metav1.Time{Time: time.Now().Add(-6 * time.Minute)},
					},
				},
			},
			expectCondition: v1beta2conditions.ConditionWithOwnerInfo{
				OwnerResource: v1beta2conditions.ConditionOwnerInfo{
					Kind: "Machine",
					Name: "machine-test",
				},
				Condition: metav1.Condition{
					Type:    clusterv1.MachineDeletingV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineDeletingV1Beta2Reason,
					Message: "Machine deletion in progress since more than 15m, stage: DrainingNode, delay likely due to PodDisruptionBudgets, Pods not terminating, Pod eviction errors",
				},
			},
		},
		{
			name: "Deleting condition with WaitingForVolumeDetach since more than 15m",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "machine-test",
					Namespace:         metav1.NamespaceDefault,
					DeletionTimestamp: &metav1.Time{Time: time.Now().Add(-16 * time.Minute)},
				},
				Status: clusterv1.MachineStatus{
					V1Beta2: &clusterv1.MachineV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:    clusterv1.MachineDeletingV1Beta2Condition,
								Status:  metav1.ConditionTrue,
								Reason:  clusterv1.MachineDeletingWaitingForVolumeDetachV1Beta2Reason,
								Message: "Waiting for Node volumes to be detached (started at 2024-10-09T16:13:59Z)",
							},
						},
					},
					Deletion: &clusterv1.MachineDeletionStatus{
						WaitForNodeVolumeDetachStartTime: &metav1.Time{Time: time.Now().Add(-6 * time.Minute)},
					},
				},
			},
			expectCondition: v1beta2conditions.ConditionWithOwnerInfo{
				OwnerResource: v1beta2conditions.ConditionOwnerInfo{
					Kind: "Machine",
					Name: "machine-test",
				},
				Condition: metav1.Condition{
					Type:    clusterv1.MachineDeletingV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineDeletingV1Beta2Reason,
					Message: "Machine deletion in progress since more than 15m, stage: WaitingForVolumeDetach",
				},
			},
		},
		{
			name: "Deleting condition with WaitingForPreDrainHook",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Status: clusterv1.MachineStatus{
					V1Beta2: &clusterv1.MachineV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:    clusterv1.MachineDeletingV1Beta2Condition,
								Status:  metav1.ConditionTrue,
								Reason:  clusterv1.MachineDeletingWaitingForPreDrainHookV1Beta2Reason,
								Message: "Waiting for pre-drain hooks to succeed (hooks: test-hook)",
							},
						},
					},
				},
			},
			expectCondition: v1beta2conditions.ConditionWithOwnerInfo{
				OwnerResource: v1beta2conditions.ConditionOwnerInfo{
					Kind: "Machine",
					Name: "machine-test",
				},
				Condition: metav1.Condition{
					Type:    clusterv1.MachineDeletingV1Beta2Condition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineDeletingV1Beta2Reason,
					Message: "Machine deletion in progress, stage: WaitingForPreDrainHook",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(calculateDeletingConditionForSummary(tc.machine)).To(BeComparableTo(tc.expectCondition))
		})
	}
}

func TestAvailableCondition(t *testing.T) {
	testCases := []struct {
		name            string
		machine         *clusterv1.Machine
		expectCondition metav1.Condition
	}{
		{
			name: "Not Ready",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Status: clusterv1.MachineStatus{
					V1Beta2: &clusterv1.MachineV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.MachineReadyV1Beta2Condition,
								Status: metav1.ConditionFalse,
								Reason: "SomeReason",
							},
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineAvailableV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineNotReadyV1Beta2Reason,
			},
		},
		{
			name: "Ready but still waiting for MinReadySeconds",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Status: clusterv1.MachineStatus{
					V1Beta2: &clusterv1.MachineV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:               clusterv1.MachineReadyV1Beta2Condition,
								Status:             metav1.ConditionTrue,
								Reason:             clusterv1.MachineReadyV1Beta2Reason,
								LastTransitionTime: metav1.Time{Time: time.Now().Add(10 * time.Second)},
							},
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineAvailableV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineWaitingForMinReadySecondsV1Beta2Reason,
			},
		},
		{
			name: "Ready and available",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Status: clusterv1.MachineStatus{
					V1Beta2: &clusterv1.MachineV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:               clusterv1.MachineReadyV1Beta2Condition,
								Status:             metav1.ConditionTrue,
								Reason:             clusterv1.MachineReadyV1Beta2Reason,
								LastTransitionTime: metav1.Time{Time: time.Now().Add(-10 * time.Second)},
							},
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineAvailableV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineAvailableV1Beta2Reason,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			setAvailableCondition(ctx, tc.machine)

			availableCondition := v1beta2conditions.Get(tc.machine, clusterv1.MachineAvailableV1Beta2Condition)
			g.Expect(availableCondition).ToNot(BeNil())
			g.Expect(*availableCondition).To(v1beta2conditions.MatchCondition(tc.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
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
					Namespace:  metav1.NamespaceDefault,
				},
			},
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "GenericInfrastructureMachine",
				Name:       "infra-config1",
				Namespace:  metav1.NamespaceDefault,
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
		machine.Spec.InfrastructureRef.Namespace = ns.Name
		machine.Spec.Bootstrap.ConfigRef.Namespace = ns.Name

		g.Expect(env.Create(ctx, cluster)).To(Succeed())
		defaultKubeconfigSecret = kubeconfig.GenerateSecret(cluster, kubeconfig.FromEnvTestConfig(env.Config, cluster))
		g.Expect(env.Create(ctx, defaultKubeconfigSecret)).To(Succeed())
		// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.InfrastructureReady = true
		g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())

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
		machine.Spec.InfrastructureRef.Namespace = ns.Name
		machine.Spec.Bootstrap.ConfigRef.Namespace = ns.Name

		g.Expect(env.Create(ctx, cluster)).To(Succeed())
		defaultKubeconfigSecret = kubeconfig.GenerateSecret(cluster, kubeconfig.FromEnvTestConfig(env.Config, cluster))
		g.Expect(env.Create(ctx, defaultKubeconfigSecret)).To(Succeed())
		// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.InfrastructureReady = true
		g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())

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
		machine.Spec.InfrastructureRef.Namespace = ns.Name
		machine.Spec.Bootstrap.ConfigRef.Namespace = ns.Name

		g.Expect(env.Create(ctx, cluster)).To(Succeed())
		defaultKubeconfigSecret = kubeconfig.GenerateSecret(cluster, kubeconfig.FromEnvTestConfig(env.Config, cluster))
		g.Expect(env.Create(ctx, defaultKubeconfigSecret)).To(Succeed())
		// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.InfrastructureReady = true
		g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())

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
		machine.Spec.InfrastructureRef.Namespace = ns.Name
		machine.Spec.Bootstrap.ConfigRef.Namespace = ns.Name

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
		// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.InfrastructureReady = true
		g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())

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
		machine.Spec.InfrastructureRef.Namespace = ns.Name
		machine.Spec.Bootstrap.ConfigRef.Namespace = ns.Name

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
		// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.InfrastructureReady = true
		g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())

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
		machine.Spec.InfrastructureRef.Namespace = ns.Name
		machine.Spec.Bootstrap.ConfigRef.Namespace = ns.Name

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
		// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.InfrastructureReady = true
		g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())

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
		machine.Spec.InfrastructureRef.Namespace = ns.Name
		machine.Spec.Bootstrap.ConfigRef.Namespace = ns.Name
		// Set Machine ProviderID.
		machine.Spec.ProviderID = ptr.To(nodeProviderID)

		g.Expect(env.Create(ctx, cluster)).To(Succeed())
		defaultKubeconfigSecret = kubeconfig.GenerateSecret(cluster, kubeconfig.FromEnvTestConfig(env.Config, cluster))
		g.Expect(env.Create(ctx, defaultKubeconfigSecret)).To(Succeed())
		// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.InfrastructureReady = true
		g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())

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
		machine.Spec.InfrastructureRef.Namespace = ns.Name
		machine.Spec.Bootstrap.ConfigRef.Namespace = ns.Name

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
		// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.InfrastructureReady = true
		g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())

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
			g.Expect(nodeHealthyCondition).ToNot(BeNil())
			g.Expect(nodeHealthyCondition.Status).To(Equal(corev1.ConditionFalse))
			g.Expect(nodeHealthyCondition.Reason).To(Equal(clusterv1.DeletingReason))
			// Verify that the LastUpdated timestamp was updated
			g.Expect(machine.Status.LastUpdated).NotTo(BeNil())
			g.Expect(machine.Status.LastUpdated.After(preUpdate)).To(BeTrue())
			return true
		}, 10*time.Second).Should(BeTrue())
	})
}
