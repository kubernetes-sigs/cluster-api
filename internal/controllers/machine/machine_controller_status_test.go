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
				Reason: clusterv1.MachineBootstrapDataSecretDataSecretUserProvidedV1Beta2Reason,
			},
		},
		{
			name: "machine without bootstrap config ref and with dataSecretName not set",
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
			bootstrapConfigIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineBootstrapInvalidConfigV1Beta2Reason,
				Message: "Either spec.bootstrap.configRef must be set or spec.bootstrap.dataSecretName must not be empty",
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
							"message": "some message",
						},
					},
				},
			}},
			bootstrapConfigIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineBootstrapConfigReadyNoV1Beta2ReasonReported,
				Message: "some message (from GenericBootstrapConfig)",
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
				"status": map[string]interface{}{
					"conditions": []interface{}{},
				},
			}},
			bootstrapConfigIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineBootstrapConfigReadyNoV1Beta2ReasonReported,
				Message: "GenericBootstrapConfig status.ready is false",
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
				Status:  metav1.ConditionFalse,
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
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineBootstrapConfigDeletedV1Beta2Reason,
				Message: "GenericBootstrapConfig has been deleted",
			},
		},
		{
			name:                      "bootstrap config not found",
			machine:                   defaultMachine.DeepCopy(),
			bootstrapConfig:           nil,
			bootstrapConfigIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
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
							"message": "some message",
						},
					},
				},
			}},
			infraMachineIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineInfrastructureReadyNoV1Beta2ReasonReported,
				Message: "some message (from GenericInfrastructureMachine)",
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
				"status": map[string]interface{}{
					"conditions": []interface{}{},
				},
			}},
			infraMachineIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineInfrastructureReadyNoV1Beta2ReasonReported,
				Message: "GenericInfrastructureMachine status.ready is false",
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
				Status:  metav1.ConditionFalse,
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
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineInfrastructureDeletedV1Beta2Reason,
				Message: "GenericInfrastructureMachine has been deleted",
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
				Message: "GenericInfrastructureMachine has been deleted while the machine still exist",
			},
		},
		{
			name:                   "infra machine not found",
			machine:                defaultMachine.DeepCopy(),
			infraMachine:           nil,
			infraMachineIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
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
			expectedReason: v1beta2conditions.MultipleInfoReportedReason,
		},
		{
			name: "all conditions are unknown",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionUnknown},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionUnknown},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionUnknown},
				{Type: corev1.NodePIDPressure, Status: corev1.ConditionUnknown},
			},
			expectedStatus:  metav1.ConditionUnknown,
			expectedReason:  v1beta2conditions.MultipleUnknownReportedReason,
			expectedMessage: "Node Ready: condition is Unknown; Node MemoryPressure: condition is Unknown; Node DiskPressure: condition is Unknown; Node PIDPressure: condition is Unknown",
		},
		{
			name: "multiple semantically failed condition",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionUnknown},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue},
				{Type: corev1.NodePIDPressure, Status: corev1.ConditionTrue},
			},
			expectedStatus:  metav1.ConditionFalse,
			expectedReason:  v1beta2conditions.MultipleIssuesReportedReason,
			expectedMessage: "Node Ready: condition is Unknown; Node MemoryPressure: condition is True; Node DiskPressure: condition is True; Node PIDPressure: condition is True",
		},
		{
			name: "one semantically failed condition when the rest is healthy",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Reason: "SomeReason"},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodePIDPressure, Status: corev1.ConditionFalse},
			},
			expectedStatus:  metav1.ConditionFalse,
			expectedReason:  "SomeReason",
			expectedMessage: "Node Ready: condition is False",
		},
		{
			name: "one unknown condition when the rest is healthy",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionUnknown, Reason: "SomeReason"},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodePIDPressure, Status: corev1.ConditionFalse},
			},
			expectedStatus:  metav1.ConditionUnknown,
			expectedReason:  "SomeReason",
			expectedMessage: "Node Ready: condition is Unknown",
		},
		{
			name: "one condition missing",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodePIDPressure, Status: corev1.ConditionFalse},
			},
			expectedStatus:  metav1.ConditionUnknown,
			expectedReason:  clusterv1.MachineNodeConditionNotYetReportedV1Beta2Reason,
			expectedMessage: "Node Ready: condition not yet reported",
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
			},
		},
	}

	testCases := []struct {
		name             string
		machine          *clusterv1.Machine
		node             *corev1.Node
		expectConditions []metav1.Condition
	}{
		{
			name:    "get NodeHealthy and NodeReady from node",
			machine: defaultMachine.DeepCopy(),
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Reason: "SomeReason", Message: "Some message"},
						{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
						{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse},
						{Type: corev1.NodePIDPressure, Status: corev1.ConditionFalse},
					},
				},
			},
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  "SomeReason",
					Message: "Node Ready: condition is False",
				},
				{
					Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  "SomeReason",
					Message: "Some message (from Node)",
				},
			},
		},
		{
			// TODO: handle missing conditions in summarize node conditions.
			name:    "NodeReady missing from node",
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
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeConditionNotYetReportedV1Beta2Reason,
					Message: "Node Ready: condition not yet reported",
				},
				{
					Type:   clusterv1.MachineNodeReadyV1Beta2Condition,
					Status: metav1.ConditionUnknown,
					Reason: clusterv1.MachineNodeConditionNotYetReportedV1Beta2Reason,
				},
			},
		},
		{
			name: "node not found while machine is deleting",
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
				return m
			}(),
			node: nil,
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeDeletedV1Beta2Reason,
					Message: "Node has been deleted",
				},
				{
					Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeDeletedV1Beta2Reason,
					Message: "Node has been deleted",
				},
			},
		},
		{
			name: "node missing while machine is still running",
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.Status.NodeRef = &corev1.ObjectReference{
					Name: "test-node-1",
				}
				return m
			}(),
			node: nil,
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineNodeDeletedV1Beta2Reason,
					Message: "Node test-node-1 has been deleted while the machine still exist",
				},
				{
					Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineNodeDeletedV1Beta2Reason,
					Message: "Node test-node-1 has been deleted while the machine still exist",
				},
			},
		},
		{
			name: "machine with ProviderID set, Node still missing",
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.Spec.ProviderID = ptr.To("foo://test-node-1")
				return m
			}(),
			node: nil,
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeNotFoundV1Beta2Reason,
					Message: "Waiting for a node with Provider ID foo://test-node-1 to exist",
				},
				{
					Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeNotFoundV1Beta2Reason,
					Message: "Waiting for a node with Provider ID foo://test-node-1 to exist",
				},
			},
		},
		{
			name:    "machine with ProviderID not yet set, waiting for it",
			machine: defaultMachine.DeepCopy(),
			node:    nil,
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeNotFoundV1Beta2Reason,
					Message: "Waiting for GenericInfrastructureMachine to report spec.providerID",
				},
				{
					Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeNotFoundV1Beta2Reason,
					Message: "Waiting for GenericInfrastructureMachine to report spec.providerID",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			setNodeHealthyAndReadyConditions(ctx, tc.machine, tc.node)
			g.Expect(tc.machine.GetV1Beta2Conditions()).To(v1beta2conditions.MatchConditions(tc.expectConditions, v1beta2conditions.IgnoreLastTransitionTime(true)))
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
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineReadyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: v1beta2conditions.MultipleInfoReportedReason,
			},
		},
		{
			name: "Tolerates BootstrapConfig, InfraMachine and Node do not exists while the machine is deleting",
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
								Status: metav1.ConditionUnknown,
								Reason: clusterv1.MachineInfrastructureDeletedV1Beta2Reason,
							},
							{
								Type:   clusterv1.MachineNodeHealthyV1Beta2Condition,
								Status: metav1.ConditionTrue,
								Reason: clusterv1.MachineNodeDeletedV1Beta2Reason,
							},
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineReadyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: v1beta2conditions.MultipleInfoReportedReason,
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
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  "SomeReason",
				Message: "HealthCheckSucceeded: Some message",
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
						{
							ConditionType: "MyReadinessGate",
						},
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
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  "SomeReason",
				Message: "MyReadinessGate: Some message",
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
								Reason:             v1beta2conditions.MultipleInfoReportedReason,
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
								Reason:             v1beta2conditions.MultipleInfoReportedReason,
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

			readyCondition := v1beta2conditions.Get(tc.machine, clusterv1.MachineAvailableV1Beta2Condition)
			g.Expect(readyCondition).ToNot(BeNil())
			g.Expect(*readyCondition).To(v1beta2conditions.MatchCondition(tc.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
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
		}, 100*time.Second).Should(BeTrue())
	})
}
