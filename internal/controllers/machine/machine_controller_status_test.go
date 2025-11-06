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
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestSetBootstrapReadyCondition(t *testing.T) {
	defaultMachine := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine-test",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: clusterv1.ContractVersionedObjectReference{
					APIGroup: clusterv1.GroupVersionBootstrap.Group,
					Kind:     "GenericBootstrapConfig",
					Name:     "bootstrap-config1",
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
				m.Spec.Bootstrap.ConfigRef = clusterv1.ContractVersionedObjectReference{} // Overwriting ConfigRef to empty.
				m.Spec.Bootstrap.DataSecretName = ptr.To("foo")
				return m
			}(),
			bootstrapConfig: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": clusterv1.GroupVersionBootstrap.String(),
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"status": map[string]interface{}{},
			}},
			bootstrapConfigIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineBootstrapConfigReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineBootstrapDataSecretProvidedReason,
			},
		},
		{
			name:    "mirror Ready condition from bootstrap config (true)",
			machine: defaultMachine.DeepCopy(),
			bootstrapConfig: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": clusterv1.GroupVersionBootstrap.String(),
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
				Type:    clusterv1.MachineBootstrapConfigReadyCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineBootstrapConfigReadyReason, // reason fixed up
				Message: "some message",
			},
		},
		{
			name:    "mirror Ready condition from bootstrap config",
			machine: defaultMachine.DeepCopy(),
			bootstrapConfig: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": clusterv1.GroupVersionBootstrap.String(),
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
				Type:    clusterv1.MachineBootstrapConfigReadyCondition,
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
				"apiVersion": clusterv1.GroupVersionBootstrap.String(),
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config1",
					"namespace": metav1.NamespaceDefault,
				},
				"status": map[string]interface{}{},
			}},
			bootstrapConfigIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineBootstrapConfigReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineBootstrapConfigNotReadyReason,
				Message: "GenericBootstrapConfig status.initialization.dataSecretCreated is false",
			},
		},
		{
			name: "Use status.BoostrapReady flag as a fallback Ready condition from bootstrap config is missing (ready true)",
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.Status.Initialization.BootstrapDataSecretCreated = ptr.To(true)
				return m
			}(),
			bootstrapConfig: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": clusterv1.GroupVersionBootstrap.String(),
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
				Type:   clusterv1.MachineBootstrapConfigReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineBootstrapConfigReadyReason,
			},
		},
		{
			name:    "invalid Ready condition from bootstrap config",
			machine: defaultMachine.DeepCopy(),
			bootstrapConfig: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind":       "GenericBootstrapConfig",
				"apiVersion": clusterv1.GroupVersionBootstrap.String(),
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
				Type:    clusterv1.MachineBootstrapConfigReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineBootstrapConfigInvalidConditionReportedReason,
				Message: "failed to convert status.conditions from GenericBootstrapConfig to []metav1.Condition: status must be set for the Ready condition",
			},
		},
		{
			name:                      "failed to get bootstrap config",
			machine:                   defaultMachine.DeepCopy(),
			bootstrapConfig:           nil,
			bootstrapConfigIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineBootstrapConfigReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineBootstrapConfigInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name: "bootstrap config that was ready not found while machine is deleting",
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.Status.Initialization.BootstrapDataSecretCreated = ptr.To(true)
				m.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
				return m
			}(),
			bootstrapConfig:           nil,
			bootstrapConfigIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineBootstrapConfigReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineBootstrapConfigDeletedReason,
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
				Type:    clusterv1.MachineBootstrapConfigReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineBootstrapConfigDoesNotExistReason,
				Message: "GenericBootstrapConfig does not exist",
			},
		},
		{
			name:                      "bootstrap config not found",
			machine:                   defaultMachine.DeepCopy(),
			bootstrapConfig:           nil,
			bootstrapConfigIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineBootstrapConfigReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineBootstrapConfigDoesNotExistReason,
				Message: "GenericBootstrapConfig does not exist",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			setBootstrapReadyCondition(ctx, tc.machine, tc.bootstrapConfig, tc.bootstrapConfigIsNotFound)

			condition := conditions.Get(tc.machine, clusterv1.MachineBootstrapConfigReadyCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tc.expectCondition, conditions.IgnoreLastTransitionTime(true)))
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
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: clusterv1.GroupVersionInfrastructure.Group,
				Kind:     "GenericInfrastructureMachine",
				Name:     "infra-machine1",
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
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
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
				Type:    clusterv1.MachineInfrastructureReadyCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineInfrastructureReadyReason, // reason fixed up
				Message: "some message",
			},
		},
		{
			name:    "mirror Ready condition from infra machine",
			machine: defaultMachine.DeepCopy(),
			infraMachine: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
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
				Type:    clusterv1.MachineInfrastructureReadyCondition,
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
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"metadata": map[string]interface{}{
					"name":      "infra-machine1",
					"namespace": metav1.NamespaceDefault,
				},
				"status": map[string]interface{}{},
			}},
			infraMachineIsNotFound: false,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineInfrastructureNotReadyReason,
				Message: "GenericInfrastructureMachine status.initialization.provisioned is false",
			},
		},
		{
			name: "Use status.InfrastructureReady flag as a fallback Ready condition from infra machine is missing (ready true)",
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
				return m
			}(),
			infraMachine: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
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
				Type:   clusterv1.MachineInfrastructureReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineInfrastructureReadyReason,
			},
		},
		{
			name:    "invalid Ready condition from infra machine",
			machine: defaultMachine.DeepCopy(),
			infraMachine: &unstructured.Unstructured{Object: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
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
				Type:    clusterv1.MachineInfrastructureReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineInfrastructureInvalidConditionReportedReason,
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
				Type:    clusterv1.MachineInfrastructureReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineInfrastructureInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		},
		{
			name: "infra machine that was ready not found while machine is deleting",
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
				m.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
				return m
			}(),
			infraMachine:           nil,
			infraMachineIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineInfrastructureDeletedReason,
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
				Type:    clusterv1.MachineInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineInfrastructureDoesNotExistReason,
				Message: "GenericInfrastructureMachine does not exist",
			},
		},
		{
			name: "infra machine not found after the machine has been initialized",
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
				return m
			}(),
			infraMachine:           nil,
			infraMachineIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineInfrastructureDeletedReason,
				Message: "GenericInfrastructureMachine has been deleted while the Machine still exists",
			},
		},
		{
			name:                   "infra machine not found",
			machine:                defaultMachine.DeepCopy(),
			infraMachine:           nil,
			infraMachineIsNotFound: true,
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineInfrastructureDoesNotExistReason,
				Message: "GenericInfrastructureMachine does not exist",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			setInfrastructureReadyCondition(ctx, tc.machine, tc.infraMachine, tc.infraMachineIsNotFound)

			condition := conditions.Get(tc.machine, clusterv1.MachineInfrastructureReadyCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tc.expectCondition, conditions.IgnoreLastTransitionTime(true)))
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
			expectedReason: clusterv1.MachineNodeHealthyReason,
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
			expectedReason:  clusterv1.MachineNodeHealthUnknownReason,
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
			expectedReason: clusterv1.MachineNodeNotHealthyReason,
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
			expectedReason:  clusterv1.MachineNodeNotHealthyReason,
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
			expectedReason:  clusterv1.MachineNodeHealthUnknownReason,
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
			expectedReason:  clusterv1.MachineNodeHealthUnknownReason,
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
			status, reason, message := summarizeNodeConditions(ctx, node)
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
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: clusterv1.GroupVersionInfrastructure.Group,
				Kind:     "GenericInfrastructureMachine",
				Name:     "infra-machine1",
			},
		},
	}

	defaultCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-test",
			Namespace: metav1.NamespaceDefault,
		},
		Status: clusterv1.ClusterStatus{
			Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)},
			Conditions: []metav1.Condition{
				{Type: clusterv1.ClusterControlPlaneInitializedCondition, Status: metav1.ConditionTrue, LastTransitionTime: metav1.Time{Time: now.Add(-5 * time.Second)}},
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
				c.Status.Initialization.InfrastructureProvisioned = ptr.To(false)
				return c
			}(),
			machine: defaultMachine.DeepCopy(),
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeInspectionFailedReason,
					Message: "Waiting for Cluster status.initialization.infrastructureProvisioned to be true",
				},
				{
					Type:    clusterv1.MachineNodeReadyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeInspectionFailedReason,
					Message: "Waiting for Cluster status.initialization.infrastructureProvisioned to be true",
				},
			},
		},
		{
			name: "Cluster control plane is not initialized",
			cluster: func() *clusterv1.Cluster {
				c := defaultCluster.DeepCopy()
				conditions.Set(c, metav1.Condition{Type: clusterv1.ClusterControlPlaneInitializedCondition, Status: metav1.ConditionFalse})
				return c
			}(),
			machine: defaultMachine.DeepCopy(),
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeInspectionFailedReason,
					Message: "Waiting for Cluster control plane to be initialized",
				},
				{
					Type:    clusterv1.MachineNodeReadyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeInspectionFailedReason,
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
					Type:    clusterv1.MachineNodeHealthyCondition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineNodeNotHealthyReason,
					Message: "* Node.Ready: kubelet is NOT ready",
				},
				{
					Type:    clusterv1.MachineNodeReadyCondition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineNodeNotReadyReason,
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
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineNodeHealthyCondition,
				},
				{
					Type:   clusterv1.MachineNodeReadyCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineNodeReadyReason,
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
					Type:    clusterv1.MachineNodeHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeHealthUnknownReason,
					Message: "* Node.Ready: Condition not yet reported",
				},
				{
					Type:    clusterv1.MachineNodeReadyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeReadyUnknownReason,
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
				m.Status.NodeRef = clusterv1.MachineNodeReference{
					Name: "test-node-1",
				}
				return m
			}(),
			node:                 nil,
			lastProbeSuccessTime: now,
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeHealthyCondition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineNodeDeletedReason,
					Message: "Node test-node-1 has been deleted",
				},
				{
					Type:    clusterv1.MachineNodeReadyCondition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineNodeDeletedReason,
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
					Type:    clusterv1.MachineNodeHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeDoesNotExistReason,
					Message: "Node does not exist",
				},
				{
					Type:    clusterv1.MachineNodeReadyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeDoesNotExistReason,
					Message: "Node does not exist",
				},
			},
		},
		{
			name:    "node missing while machine is still running",
			cluster: defaultCluster,
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.Status.NodeRef = clusterv1.MachineNodeReference{
					Name: "test-node-1",
				}
				return m
			}(),
			node:                 nil,
			lastProbeSuccessTime: now,
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeHealthyCondition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineNodeDeletedReason,
					Message: "Node test-node-1 has been deleted while the Machine still exists",
				},
				{
					Type:    clusterv1.MachineNodeReadyCondition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineNodeDeletedReason,
					Message: "Node test-node-1 has been deleted while the Machine still exists",
				},
			},
		},
		{
			name:    "machine with ProviderID set, Node still missing",
			cluster: defaultCluster,
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				m.Spec.ProviderID = "foo://test-node-1"
				return m
			}(),
			node:                 nil,
			lastProbeSuccessTime: now,
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeInspectionFailedReason,
					Message: "Waiting for a Node with spec.providerID foo://test-node-1 to exist",
				},
				{
					Type:    clusterv1.MachineNodeReadyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeInspectionFailedReason,
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
					Type:    clusterv1.MachineNodeHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeInspectionFailedReason,
					Message: "Waiting for GenericInfrastructureMachine to report spec.providerID",
				},
				{
					Type:    clusterv1.MachineNodeReadyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeInspectionFailedReason,
					Message: "Waiting for GenericInfrastructureMachine to report spec.providerID",
				},
			},
		},
		{
			name: "connection down, preserve conditions as they have been set before (probe did not succeed yet)",
			cluster: func() *clusterv1.Cluster {
				c := defaultCluster.DeepCopy()
				for i, condition := range c.Status.Conditions {
					if condition.Type == clusterv1.ClusterControlPlaneInitializedCondition {
						c.Status.Conditions[i].LastTransitionTime.Time = now.Add(-4 * time.Minute)
					}
				}
				return c
			}(),
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				conditions.Set(m, metav1.Condition{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineNodeHealthyReason,
				})
				conditions.Set(m, metav1.Condition{
					Type:    clusterv1.MachineNodeReadyCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineNodeReadyReason,
					Message: "kubelet is posting ready status",
				})
				return m
			}(),
			node:                 nil,
			nodeGetErr:           errors.Wrapf(clustercache.ErrClusterNotConnected, "error getting client"),
			lastProbeSuccessTime: time.Time{}, // probe did not succeed yet
			expectConditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineNodeHealthyReason,
				},
				{
					Type:    clusterv1.MachineNodeReadyCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineNodeReadyReason,
					Message: "kubelet is posting ready status",
				},
			},
		},
		{
			name: "connection down, set conditions as they haven't been set before (probe did not succeed yet)",
			cluster: func() *clusterv1.Cluster {
				c := defaultCluster.DeepCopy()
				for i, condition := range c.Status.Conditions {
					if condition.Type == clusterv1.ClusterControlPlaneInitializedCondition {
						c.Status.Conditions[i].LastTransitionTime.Time = now.Add(-4 * time.Minute)
					}
				}
				return c
			}(),
			machine:              defaultMachine.DeepCopy(),
			node:                 nil,
			nodeGetErr:           errors.Wrapf(clustercache.ErrClusterNotConnected, "error getting client"),
			lastProbeSuccessTime: time.Time{}, // probe did not succeed yet
			expectConditions: []metav1.Condition{
				{
					Type:    clusterv1.MachineNodeReadyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeConnectionDownReason,
					Message: "Remote connection not established yet",
				},
				{
					Type:    clusterv1.MachineNodeHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeConnectionDownReason,
					Message: "Remote connection not established yet",
				},
			},
		},
		{
			name: "connection down, preserve conditions as they have been set before (remote conditions grace period not passed yet)",
			cluster: func() *clusterv1.Cluster {
				c := defaultCluster.DeepCopy()
				for i, condition := range c.Status.Conditions {
					if condition.Type == clusterv1.ClusterControlPlaneInitializedCondition {
						c.Status.Conditions[i].LastTransitionTime.Time = now.Add(-4 * time.Minute)
					}
				}
				return c
			}(),
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				conditions.Set(m, metav1.Condition{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineNodeHealthyReason,
				})
				conditions.Set(m, metav1.Condition{
					Type:    clusterv1.MachineNodeReadyCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineNodeReadyReason,
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
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineNodeHealthyReason,
				},
				{
					Type:    clusterv1.MachineNodeReadyCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineNodeReadyReason,
					Message: "kubelet is posting ready status",
				},
			},
		},
		{
			name: "connection down, set conditions as they haven't been set before (remote conditions grace period not passed yet)",
			cluster: func() *clusterv1.Cluster {
				c := defaultCluster.DeepCopy()
				for i, condition := range c.Status.Conditions {
					if condition.Type == clusterv1.ClusterControlPlaneInitializedCondition {
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
					Type:    clusterv1.MachineNodeReadyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeConnectionDownReason,
					Message: fmt.Sprintf("Last successful probe at %s", now.Add(-3*time.Minute).Format(time.RFC3339)),
				},
				{
					Type:    clusterv1.MachineNodeHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeConnectionDownReason,
					Message: fmt.Sprintf("Last successful probe at %s", now.Add(-3*time.Minute).Format(time.RFC3339)),
				},
			},
		},
		{
			name: "connection down, set conditions to unknown (remote conditions grace period passed)",
			cluster: func() *clusterv1.Cluster {
				c := defaultCluster.DeepCopy()
				for i, condition := range c.Status.Conditions {
					if condition.Type == clusterv1.ClusterControlPlaneInitializedCondition {
						c.Status.Conditions[i].LastTransitionTime.Time = now.Add(-7 * time.Minute)
					}
				}
				return c
			}(),
			machine: func() *clusterv1.Machine {
				m := defaultMachine.DeepCopy()
				conditions.Set(m, metav1.Condition{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineNodeHealthyReason,
				})
				conditions.Set(m, metav1.Condition{
					Type:    clusterv1.MachineNodeReadyCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineNodeReadyReason,
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
					Type:    clusterv1.MachineNodeReadyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeConnectionDownReason,
					Message: fmt.Sprintf("Last successful probe at %s", now.Add(-6*time.Minute).Format(time.RFC3339)),
				},
				{
					Type:    clusterv1.MachineNodeHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeConnectionDownReason,
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
					Type:    clusterv1.MachineNodeReadyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeInternalErrorReason,
					Message: "Please check controller logs for errors",
				},
				{
					Type:    clusterv1.MachineNodeHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.MachineNodeInternalErrorReason,
					Message: "Please check controller logs for errors",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			healthCheckingState := clustercache.HealthCheckingState{
				LastProbeSuccessTime: tc.lastProbeSuccessTime,
			}
			setNodeHealthyAndReadyConditions(ctx, tc.cluster, tc.machine, tc.node, tc.nodeGetErr, healthCheckingState, 5*time.Minute)
			g.Expect(tc.machine.GetConditions()).To(conditions.MatchConditions(tc.expectConditions, conditions.IgnoreLastTransitionTime(true)))
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
				Type:   clusterv1.MachineDeletingCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineNotDeletingReason,
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
			deletingReason:          clusterv1.MachineDeletingWaitingForPreDrainHookReason,
			deletingMessage:         "Waiting for pre-drain hooks to succeed (hooks: test-hook)",
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeletingCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeletingWaitingForPreDrainHookReason,
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
			deletingReason:          clusterv1.MachineDeletingDrainingNodeReason,
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
				Type:   clusterv1.MachineDeletingCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineDeletingDrainingNodeReason,
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
					Conditions: []metav1.Condition{
						{
							Type:    clusterv1.MachineDeletingCondition,
							Status:  metav1.ConditionTrue,
							Reason:  clusterv1.MachineDeletingWaitingForPreDrainHookReason,
							Message: "Waiting for pre-drain hooks to succeed (hooks: test-hook)",
						},
					},
				},
			},
			reconcileDeleteExecuted: false,
			deletingReason:          "",
			deletingMessage:         "",
			// Condition was not updated because reconcileDelete was not executed.
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineDeletingCondition,
				Status:  metav1.ConditionTrue,
				Reason:  clusterv1.MachineDeletingWaitingForPreDrainHookReason,
				Message: "Waiting for pre-drain hooks to succeed (hooks: test-hook)",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			setDeletingCondition(ctx, tc.machine, tc.reconcileDeleteExecuted, tc.deletingReason, tc.deletingMessage)

			deletingCondition := conditions.Get(tc.machine, clusterv1.MachineDeletingCondition)
			g.Expect(deletingCondition).ToNot(BeNil())
			g.Expect(*deletingCondition).To(conditions.MatchCondition(tc.expectCondition, conditions.IgnoreLastTransitionTime(true)))
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
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyCondition),
				fmt.Sprintf("* %s: Foo", clusterv1.InfrastructureReadyCondition),
				fmt.Sprintf("* %s: Foo", clusterv1.MachineNodeHealthyCondition),
				fmt.Sprintf("* %s: Foo", clusterv1.MachineDeletingCondition),
				fmt.Sprintf("* %s: Foo", clusterv1.MachineHealthCheckSucceededCondition),
			},
			expectMessages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyCondition),
				fmt.Sprintf("* %s: Foo", clusterv1.InfrastructureReadyCondition),
				fmt.Sprintf("* %s: Foo", clusterv1.MachineNodeHealthyCondition),
				fmt.Sprintf("* %s: Foo", clusterv1.MachineDeletingCondition),
				fmt.Sprintf("* %s: Foo", clusterv1.MachineHealthCheckSucceededCondition),
			},
		},
		{
			name: "group control plane conditions when msg are all equal",
			messages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyCondition),
				"* APIServerSomething: Waiting for AWSMachine to report spec.providerID",
				"* ControllerManagerSomething: Waiting for AWSMachine to report spec.providerID",
				"* SchedulerSomething: Waiting for AWSMachine to report spec.providerID",
				"* EtcdSomething: Waiting for AWSMachine to report spec.providerID",
			},
			expectMessages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyCondition),
				"* Control plane components: Waiting for AWSMachine to report spec.providerID",
				"* EtcdSomething: Waiting for AWSMachine to report spec.providerID",
			},
		},
		{
			name: "don't group control plane conditions when msg are not all equal",
			messages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyCondition),
				"* APIServerSomething: Waiting for AWSMachine to report spec.providerID",
				"* ControllerManagerSomething: Some message",
				"* SchedulerSomething: Waiting for AWSMachine to report spec.providerID",
				"* EtcdSomething:Some message",
			},
			expectMessages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyCondition),
				"* APIServerSomething: Waiting for AWSMachine to report spec.providerID",
				"* ControllerManagerSomething: Some message",
				"* SchedulerSomething: Waiting for AWSMachine to report spec.providerID",
				"* EtcdSomething:Some message",
			},
		},
		{
			name: "don't group control plane conditions when there is only one condition",
			messages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyCondition),
				"* APIServerSomething: Waiting for AWSMachine to report spec.providerID",
				"* EtcdSomething:Some message",
			},
			expectMessages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyCondition),
				"* APIServerSomething: Waiting for AWSMachine to report spec.providerID",
				"* EtcdSomething:Some message",
			},
		},
		{
			name: "Treat EtcdPodHealthy as a special case (it groups with control plane components)",
			messages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyCondition),
				"* APIServerSomething: Waiting for AWSMachine to report spec.providerID",
				"* ControllerManagerSomething: Waiting for AWSMachine to report spec.providerID",
				"* SchedulerSomething: Waiting for AWSMachine to report spec.providerID",
				"* EtcdPodHealthy: Waiting for AWSMachine to report spec.providerID",
				"* EtcdSomething: Some message",
			},
			expectMessages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyCondition),
				"* Control plane components: Waiting for AWSMachine to report spec.providerID",
				"* EtcdSomething: Some message",
			},
		},
		{
			name: "Treat EtcdPodHealthy as a special case (it does not group with other etcd conditions)",
			messages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyCondition),
				"* APIServerSomething: Waiting for AWSMachine to report spec.providerID",
				"* ControllerManagerSomething: Waiting for AWSMachine to report spec.providerID",
				"* SchedulerSomething: Waiting for AWSMachine to report spec.providerID",
				"* EtcdPodHealthy: Some message",
				"* EtcdSomething: Some message",
			},
			expectMessages: []string{
				fmt.Sprintf("* %s: Foo", clusterv1.MachineBootstrapConfigReadyCondition),
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

func TestSetUpdatingCondition(t *testing.T) {
	tests := []struct {
		name            string
		machine         *clusterv1.Machine
		updatingReason  string
		updatingMessage string
		expectCondition *metav1.Condition
	}{
		{
			name:            "A machine not in-place updating is not updating",
			machine:         &clusterv1.Machine{},
			updatingReason:  "foo",
			updatingMessage: "bar",
			expectCondition: &metav1.Condition{
				Type:   clusterv1.MachineUpdatingCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineNotUpdatingReason,
			},
		},
		{
			name: "A machine starting in-place update is updating",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.UpdateInProgressAnnotation: "",
					},
				},
			},
			updatingReason:  "foo",
			updatingMessage: "bar",
			expectCondition: &metav1.Condition{
				Type:    clusterv1.MachineUpdatingCondition,
				Status:  metav1.ConditionTrue,
				Reason:  "foo",
				Message: "bar",
			},
		},
		{
			name: "A machine in-place updating is updating",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.UpdateInProgressAnnotation: "",
						runtimev1.PendingHooksAnnotation:     "UpdateMachine",
					},
				},
			},
			updatingReason:  "foo",
			updatingMessage: "bar",
			expectCondition: &metav1.Condition{
				Type:    clusterv1.MachineUpdatingCondition,
				Status:  metav1.ConditionTrue,
				Reason:  "foo",
				Message: "bar",
			},
		},
		{
			name: "A machine stopping in-place updating is updating",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						runtimev1.PendingHooksAnnotation: "UpdateMachine",
					},
				},
			},
			updatingReason:  "foo",
			updatingMessage: "bar",
			expectCondition: &metav1.Condition{
				Type:    clusterv1.MachineUpdatingCondition,
				Status:  metav1.ConditionTrue,
				Reason:  "foo",
				Message: "bar",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setUpdatingCondition(ctx, tt.machine, tt.updatingReason, tt.updatingMessage)

			condition := conditions.Get(tt.machine, clusterv1.MachineUpdatingCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(*tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestSetUpToDateCondition(t *testing.T) {
	reconciliationTime := time.Now()
	tests := []struct {
		name              string
		machineDeployment *clusterv1.MachineDeployment
		machineSet        *clusterv1.MachineSet
		machine           *clusterv1.Machine
		expectCondition   *metav1.Condition
	}{
		{
			name:              "no condition returned for stand-alone Machines",
			machineDeployment: nil,
			machineSet:        nil,
			machine:           &clusterv1.Machine{},
			expectCondition:   nil,
		},
		{
			name:              "no condition returned for stand-alone MachineSet",
			machineDeployment: nil,
			machineSet:        &clusterv1.MachineSet{},
			machine:           &clusterv1.Machine{},
			expectCondition:   nil,
		},
		{
			name: "up-to-date",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: "v1.31.0",
						},
					},
				},
			},
			machineSet: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: "v1.31.0",
						},
					},
				},
			},
			machine: &clusterv1.Machine{},
			expectCondition: &metav1.Condition{
				Type:   clusterv1.MachineUpToDateCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineUpToDateReason,
			},
		},
		{
			name: "updating",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: "v1.31.0",
						},
					},
				},
			},
			machineSet: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: "v1.31.0",
						},
					},
				},
			},
			machine: &clusterv1.Machine{
				Status: clusterv1.MachineStatus{
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.MachineUpdatingCondition,
							Status: metav1.ConditionTrue,
							Reason: clusterv1.MachineInPlaceUpdatingReason,
						},
					},
				},
			},
			expectCondition: &metav1.Condition{
				Type:   clusterv1.MachineUpToDateCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineUpToDateUpdatingReason,
			},
		},
		{
			name: "not up-to-date",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: "v1.31.0",
						},
					},
				},
			},
			machineSet: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: "v1.30.0",
						},
					},
				},
			},
			machine: &clusterv1.Machine{},
			expectCondition: &metav1.Condition{
				Type:    clusterv1.MachineUpToDateCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineNotUpToDateReason,
				Message: "* Version v1.30.0, v1.31.0 required",
			},
		},
		{
			name: "up-to-date, spec.rolloutAfter not expired",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Rollout: clusterv1.MachineDeploymentRolloutSpec{
						After: metav1.Time{Time: reconciliationTime.Add(1 * time.Hour)}, // rollout after not yet expired
					},
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: "v1.31.0",
						},
					},
				},
			},
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Time{Time: reconciliationTime.Add(-1 * time.Hour)}, // MS created before rollout after
				},
				Spec: clusterv1.MachineSetSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: "v1.31.0",
						},
					},
				},
			},
			machine: &clusterv1.Machine{},
			expectCondition: &metav1.Condition{
				Type:   clusterv1.MachineUpToDateCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineUpToDateReason,
			},
		},
		{
			name: "not up-to-date, rollout After expired",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Rollout: clusterv1.MachineDeploymentRolloutSpec{
						After: metav1.Time{Time: reconciliationTime.Add(-1 * time.Hour)}, // rollout after expired
					},
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: "v1.31.0",
						},
					},
				},
			},
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Time{Time: reconciliationTime.Add(-2 * time.Hour)}, // MS created before rollout after
				},
				Spec: clusterv1.MachineSetSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: "v1.31.0",
						},
					},
				},
			},
			machine: &clusterv1.Machine{},
			expectCondition: &metav1.Condition{
				Type:    clusterv1.MachineUpToDateCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineNotUpToDateReason,
				Message: "* MachineDeployment spec.rolloutAfter expired",
			},
		},
		{
			name: "not up-to-date, rollout After expired and a new MS created",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Rollout: clusterv1.MachineDeploymentRolloutSpec{
						After: metav1.Time{Time: reconciliationTime.Add(-2 * time.Hour)}, // rollout after expired
					},
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: "v1.31.0",
						},
					},
				},
			},
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Time{Time: reconciliationTime.Add(-1 * time.Hour)}, // MS created after rollout after
				},
				Spec: clusterv1.MachineSetSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: "v1.31.0",
						},
					},
				},
			},
			machine: &clusterv1.Machine{},
			expectCondition: &metav1.Condition{
				Type:   clusterv1.MachineUpToDateCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineUpToDateReason,
			},
		},
		{
			name: "not up-to-date, version changed, rollout After expired",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Rollout: clusterv1.MachineDeploymentRolloutSpec{
						After: metav1.Time{Time: reconciliationTime.Add(-1 * time.Hour)}, // rollout after expired
					},
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: "v1.30.0",
						},
					},
				},
			},
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Time{Time: reconciliationTime.Add(-2 * time.Hour)}, // MS created before rollout after
				},
				Spec: clusterv1.MachineSetSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: "v1.31.0",
						},
					},
				},
			},
			machine: &clusterv1.Machine{},
			expectCondition: &metav1.Condition{
				Type:   clusterv1.MachineUpToDateCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineNotUpToDateReason,
				Message: "* Version v1.31.0, v1.30.0 required\n" +
					"* MachineDeployment spec.rolloutAfter expired",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			setUpToDateCondition(ctx, tt.machine, tt.machineSet, tt.machineDeployment)

			condition := conditions.Get(tt.machine, clusterv1.MachineUpToDateCondition)
			if tt.expectCondition != nil {
				g.Expect(condition).ToNot(BeNil())
				g.Expect(*condition).To(conditions.MatchCondition(*tt.expectCondition, conditions.IgnoreLastTransitionTime(true)))
			} else {
				g.Expect(condition).To(BeNil())
			}
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
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.MachineBootstrapConfigReadyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.InfrastructureReadyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.MachineNodeHealthyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.MachineDeletingCondition,
							Status: metav1.ConditionFalse,
							Reason: clusterv1.MachineNotDeletingReason,
						},
						{
							Type:   clusterv1.MachineUpdatingCondition,
							Status: metav1.ConditionFalse,
							Reason: clusterv1.MachineNotUpdatingReason,
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineReadyReason,
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
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.MachineBootstrapConfigReadyCondition,
							Status: metav1.ConditionFalse,
							Reason: clusterv1.MachineBootstrapConfigDoesNotExistReason,
						},
						{
							Type:   clusterv1.InfrastructureReadyCondition,
							Status: metav1.ConditionFalse,
							Reason: clusterv1.MachineInfrastructureDeletedReason,
						},
						{
							Type:   clusterv1.MachineNodeHealthyCondition,
							Status: metav1.ConditionFalse,
							Reason: clusterv1.MachineNodeDeletedReason,
						},
						{
							Type:    clusterv1.MachineDeletingCondition,
							Status:  metav1.ConditionTrue,
							Reason:  clusterv1.MachineDeletingWaitingForPreDrainHookReason,
							Message: "Waiting for pre-drain hooks to succeed (hooks: test-hook)",
						},
						{
							Type:   clusterv1.MachineUpdatingCondition,
							Status: metav1.ConditionFalse,
							Reason: clusterv1.MachineNotUpdatingReason,
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineNotReadyReason,
				Message: "* Deleting: Machine deletion in progress, stage: WaitingForPreDrainHook",
			},
		},
		{
			name: "Aggregates Ready condition correctly while the machine is updating",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Status: clusterv1.MachineStatus{
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.MachineBootstrapConfigReadyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.InfrastructureReadyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.MachineNodeHealthyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.MachineDeletingCondition,
							Status: metav1.ConditionFalse,
							Reason: clusterv1.MachineNotDeletingReason,
						},
						{
							Type:    clusterv1.MachineUpdatingCondition,
							Status:  metav1.ConditionTrue,
							Reason:  clusterv1.MachineInPlaceUpdatingReason,
							Message: "In place update in progress",
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineNotReadyReason,
				Message: "* Updating: In place update in progress",
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
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.MachineBootstrapConfigReadyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.InfrastructureReadyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.MachineNodeHealthyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.MachineDeletingCondition,
							Status: metav1.ConditionTrue,
							Reason: clusterv1.MachineDeletingDrainingNodeReason,
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
						{
							Type:   clusterv1.MachineUpdatingCondition,
							Status: metav1.ConditionFalse,
							Reason: clusterv1.MachineNotUpdatingReason,
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineNotReadyReason,
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
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.MachineBootstrapConfigReadyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.InfrastructureReadyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.MachineNodeHealthyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:    clusterv1.MachineHealthCheckSucceededCondition,
							Status:  metav1.ConditionFalse,
							Reason:  "SomeReason",
							Message: "Some message",
						},
						{
							Type:   clusterv1.MachineDeletingCondition,
							Status: metav1.ConditionFalse,
							Reason: clusterv1.MachineNotDeletingReason,
						},
						{
							Type:   clusterv1.MachineUpdatingCondition,
							Status: metav1.ConditionFalse,
							Reason: clusterv1.MachineNotUpdatingReason,
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:    clusterv1.MachineReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineNotReadyReason,
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
						{
							ConditionType: "MyReadinessGate",
						},
						{
							ConditionType: "MyReadinessGateGateWithNegativePolarity",
							Polarity:      clusterv1.NegativePolarityCondition,
						},
					},
				},
				Status: clusterv1.MachineStatus{
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.MachineBootstrapConfigReadyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.InfrastructureReadyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.MachineNodeHealthyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.MachineHealthCheckSucceededCondition,
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
							Type:    "MyReadinessGateGateWithNegativePolarity",
							Status:  metav1.ConditionTrue,
							Reason:  "SomeReason",
							Message: "Some other message",
						},
						{
							Type:   clusterv1.MachineDeletingCondition,
							Status: metav1.ConditionFalse,
							Reason: clusterv1.MachineNotDeletingReason,
						},
						{
							Type:   clusterv1.MachineUpdatingCondition,
							Status: metav1.ConditionFalse,
							Reason: clusterv1.MachineNotUpdatingReason,
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineReadyCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineNotReadyReason,
				Message: "* MyReadinessGate: Some message\n" +
					"* MyReadinessGateGateWithNegativePolarity: Some other message",
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
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.MachineBootstrapConfigReadyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.InfrastructureReadyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.MachineNodeHealthyCondition,
							Status: metav1.ConditionTrue,
							Reason: "Foo",
						},
						{
							Type:   clusterv1.MachineHealthCheckSucceededCondition,
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
							Type:   clusterv1.MachineDeletingCondition,
							Status: metav1.ConditionFalse,
							Reason: clusterv1.MachineNotDeletingReason,
						},
						{
							Type:   clusterv1.MachineUpdatingCondition,
							Status: metav1.ConditionFalse,
							Reason: clusterv1.MachineNotUpdatingReason,
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineReadyCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineNotReadyReason,
				Message: "* Control plane components: Some control plane message\n" +
					"* EtcdSomething: Some etcd message",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			setReadyCondition(ctx, tc.machine)

			condition := conditions.Get(tc.machine, clusterv1.MachineReadyCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(*condition).To(conditions.MatchCondition(tc.expectCondition, conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestCalculateDeletingConditionForSummary(t *testing.T) {
	testCases := []struct {
		name            string
		machine         *clusterv1.Machine
		expectCondition conditions.ConditionWithOwnerInfo
	}{
		{
			name: "No Deleting condition",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Status: clusterv1.MachineStatus{
					Conditions: []metav1.Condition{},
				},
			},
			expectCondition: conditions.ConditionWithOwnerInfo{
				OwnerResource: conditions.ConditionOwnerInfo{
					Kind: "Machine",
					Name: "machine-test",
				},
				Condition: metav1.Condition{
					Type:    clusterv1.MachineDeletingCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineDeletingReason,
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
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.MachineDeletingCondition,
							Status: metav1.ConditionTrue,
							Reason: clusterv1.MachineDeletingDrainingNodeReason,
							Message: `Drain not completed yet (started at 2024-10-09T16:13:59Z):
* Pods pod-2-deletionTimestamp-set-1, pod-3-to-trigger-eviction-successfully-1: deletionTimestamp set, but still not removed from the Node
* Pod pod-5-to-trigger-eviction-pdb-violated-1: cannot evict pod as it would violate the pod's disruption budget. The disruption budget pod-5-pdb needs 20 healthy pods and has 20 currently
* Pod pod-6-to-trigger-eviction-some-other-error: failed to evict Pod, some other error 1
* Pod pod-9-wait-completed: waiting for completion
After above Pods have been removed from the Node, the following Pods will be evicted: pod-7-eviction-later, pod-8-eviction-later`,
						},
					},
					Deletion: &clusterv1.MachineDeletionStatus{
						NodeDrainStartTime: metav1.Time{Time: time.Now().Add(-6 * time.Minute)},
					},
				},
			},
			expectCondition: conditions.ConditionWithOwnerInfo{
				OwnerResource: conditions.ConditionOwnerInfo{
					Kind: "Machine",
					Name: "machine-test",
				},
				Condition: metav1.Condition{
					Type:    clusterv1.MachineDeletingCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineDeletingReason,
					Message: "Machine deletion in progress since more than 15m, stage: DrainingNode, delay likely due to PodDisruptionBudgets, Pods not terminating, Pod eviction errors, Pods not completed yet",
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
					Conditions: []metav1.Condition{
						{
							Type:    clusterv1.MachineDeletingCondition,
							Status:  metav1.ConditionTrue,
							Reason:  clusterv1.MachineDeletingWaitingForVolumeDetachReason,
							Message: "Waiting for Node volumes to be detached (started at 2024-10-09T16:13:59Z)",
						},
					},
					Deletion: &clusterv1.MachineDeletionStatus{
						WaitForNodeVolumeDetachStartTime: metav1.Time{Time: time.Now().Add(-6 * time.Minute)},
					},
				},
			},
			expectCondition: conditions.ConditionWithOwnerInfo{
				OwnerResource: conditions.ConditionOwnerInfo{
					Kind: "Machine",
					Name: "machine-test",
				},
				Condition: metav1.Condition{
					Type:    clusterv1.MachineDeletingCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineDeletingReason,
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
					Conditions: []metav1.Condition{
						{
							Type:    clusterv1.MachineDeletingCondition,
							Status:  metav1.ConditionTrue,
							Reason:  clusterv1.MachineDeletingWaitingForPreDrainHookReason,
							Message: "Waiting for pre-drain hooks to succeed (hooks: test-hook)",
						},
					},
				},
			},
			expectCondition: conditions.ConditionWithOwnerInfo{
				OwnerResource: conditions.ConditionOwnerInfo{
					Kind: "Machine",
					Name: "machine-test",
				},
				Condition: metav1.Condition{
					Type:    clusterv1.MachineDeletingCondition,
					Status:  metav1.ConditionTrue,
					Reason:  clusterv1.MachineDeletingReason,
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
		expectRes       ctrl.Result
	}{
		{
			name: "Not Ready",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Status: clusterv1.MachineStatus{
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.MachineReadyCondition,
							Status: metav1.ConditionFalse,
							Reason: "SomeReason",
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineAvailableCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineNotReadyReason,
			},
			expectRes: ctrl.Result{},
		},
		{
			name: "Ready but still waiting for MinReadySeconds",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Status: clusterv1.MachineStatus{
					Conditions: []metav1.Condition{
						{
							Type:               clusterv1.MachineReadyCondition,
							Status:             metav1.ConditionTrue,
							Reason:             clusterv1.MachineReadyReason,
							LastTransitionTime: metav1.Time{Time: time.Now().Add(10 * time.Second)},
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineAvailableCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineWaitingForMinReadySecondsReason,
			},
			expectRes: ctrl.Result{RequeueAfter: 10 * time.Second},
		},
		{
			name: "Ready and available",
			machine: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-test",
					Namespace: metav1.NamespaceDefault,
				},
				Status: clusterv1.MachineStatus{
					Conditions: []metav1.Condition{
						{
							Type:               clusterv1.MachineReadyCondition,
							Status:             metav1.ConditionTrue,
							Reason:             clusterv1.MachineReadyReason,
							LastTransitionTime: metav1.Time{Time: time.Now().Add(-10 * time.Second)},
						},
					},
				},
			},
			expectCondition: metav1.Condition{
				Type:   clusterv1.MachineAvailableCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineAvailableReason,
			},
			expectRes: ctrl.Result{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			res := setAvailableCondition(ctx, tc.machine)

			availableCondition := conditions.Get(tc.machine, clusterv1.MachineAvailableCondition)
			g.Expect(availableCondition).ToNot(BeNil())
			g.Expect(*availableCondition).To(conditions.MatchCondition(tc.expectCondition, conditions.IgnoreLastTransitionTime(true)))

			g.Expect(res.IsZero()).To(Equal(tc.expectRes.IsZero()))
			if !tc.expectRes.IsZero() {
				g.Expect(res.RequeueAfter).To(BeNumerically("~", tc.expectRes.RequeueAfter, 100*time.Millisecond))
			}
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
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: builder.ControlPlaneGroupVersion.Group,
				Kind:     builder.GenericControlPlaneKind,
				Name:     "cp1",
			},
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
				ConfigRef: clusterv1.ContractVersionedObjectReference{
					APIGroup: clusterv1.GroupVersionBootstrap.Group,
					Kind:     "GenericBootstrapConfig",
					Name:     "bootstrap-config1",
				},
			},
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: clusterv1.GroupVersionInfrastructure.Group,
				Kind:     "GenericInfrastructureMachine",
				Name:     "infra-config1",
			},
		},
	}

	defaultBootstrap := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericBootstrapConfig",
			"apiVersion": clusterv1.GroupVersionBootstrap.String(),
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
			"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
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
		// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
		g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())

		g.Expect(env.Create(ctx, bootstrapConfig)).To(Succeed())
		g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
		// Create and wait on machine to make sure caches sync and reconciliation triggers.
		g.Expect(env.CreateAndWait(ctx, machine)).To(Succeed())

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
			if err := env.DirectAPIServerGet(ctx, client.ObjectKeyFromObject(infraMachine), infraMachine); err != nil {
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
		// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
		g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())

		g.Expect(env.Create(ctx, bootstrapConfig)).To(Succeed())
		g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
		// Create and wait on machine to make sure caches sync and reconciliation triggers.
		g.Expect(env.CreateAndWait(ctx, machine)).To(Succeed())

		// Wait until Machine was reconciled.
		g.Eventually(func(g Gomega) bool {
			if err := env.DirectAPIServerGet(ctx, client.ObjectKeyFromObject(machine), machine); err != nil {
				return false
			}
			g.Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhasePending))
			// LastUpdated should be set as the phase changes
			g.Expect(machine.Status.LastUpdated.IsZero()).To(BeFalse())
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
		// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
		g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())

		g.Expect(env.Create(ctx, bootstrapConfig)).To(Succeed())
		g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
		// We have to subtract 2 seconds, because .status.lastUpdated does not contain milliseconds.
		preUpdate := time.Now().Add(-2 * time.Second)
		g.Expect(env.CreateAndWait(ctx, machine)).To(Succeed())

		// Set the LastUpdated to be able to verify it is updated when the phase changes
		modifiedMachine := machine.DeepCopy()
		g.Expect(env.Status().Patch(ctx, modifiedMachine, client.MergeFrom(machine))).To(Succeed())

		// Set bootstrap ready.
		modifiedBootstrapConfig := bootstrapConfig.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, true, "status", "initialization", "dataSecretCreated")).To(Succeed())
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, "secret-data", "status", "dataSecretName")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedBootstrapConfig, client.MergeFrom(bootstrapConfig))).To(Succeed())

		// Wait until Machine was reconciled.
		g.Eventually(func(g Gomega) bool {
			if err := env.DirectAPIServerGet(ctx, client.ObjectKeyFromObject(machine), machine); err != nil {
				return false
			}
			g.Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseProvisioning))
			// Verify that the LastUpdated timestamp was updated
			g.Expect(machine.Status.LastUpdated.IsZero()).To(BeFalse())
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
		// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
		g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())

		g.Expect(env.Create(ctx, bootstrapConfig)).To(Succeed())
		g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
		// We have to subtract 2 seconds, because .status.lastUpdated does not contain milliseconds.
		preUpdate := time.Now().Add(-2 * time.Second)

		// Create and wait on machine to make sure caches sync and reconciliation triggers.
		g.Expect(env.CreateAndWait(ctx, machine)).To(Succeed())

		modifiedMachine := machine.DeepCopy()
		// Set NodeRef.
		machine.Status.NodeRef = clusterv1.MachineNodeReference{Name: node.Name}
		g.Expect(env.Status().Patch(ctx, modifiedMachine, client.MergeFrom(machine))).To(Succeed())

		// Set bootstrap ready.
		modifiedBootstrapConfig := bootstrapConfig.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, true, "status", "initialization", "dataSecretCreated")).To(Succeed())
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, "secret-data", "status", "dataSecretName")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedBootstrapConfig, client.MergeFrom(bootstrapConfig))).To(Succeed())

		// Set infra ready.
		modifiedInfraMachine := infraMachine.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedInfraMachine.Object, true, "status", "initialization", "provisioned")).To(Succeed())
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
			if err := env.DirectAPIServerGet(ctx, client.ObjectKeyFromObject(machine), machine); err != nil {
				return false
			}
			g.Expect(machine.Status.Addresses).To(HaveLen(2))
			g.Expect(machine.Spec.FailureDomain).To(Equal("us-east-2a"))
			g.Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseRunning))
			// Verify that the LastUpdated timestamp was updated
			g.Expect(machine.Status.LastUpdated.IsZero()).To(BeFalse())
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
		// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
		g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())

		g.Expect(env.Create(ctx, bootstrapConfig)).To(Succeed())
		g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
		// We have to subtract 2 seconds, because .status.lastUpdated does not contain milliseconds.
		preUpdate := time.Now().Add(-2 * time.Second)
		// Create and wait on machine to make sure caches sync and reconciliation triggers.
		g.Expect(env.CreateAndWait(ctx, machine)).To(Succeed())

		modifiedMachine := machine.DeepCopy()
		// Set NodeRef.
		machine.Status.NodeRef = clusterv1.MachineNodeReference{Name: node.Name}
		g.Expect(env.Status().Patch(ctx, modifiedMachine, client.MergeFrom(machine))).To(Succeed())

		// Set bootstrap ready.
		modifiedBootstrapConfig := bootstrapConfig.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, true, "status", "initialization", "dataSecretCreated")).To(Succeed())
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, "secret-data", "status", "dataSecretName")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedBootstrapConfig, client.MergeFrom(bootstrapConfig))).To(Succeed())

		// Set infra ready.
		modifiedInfraMachine := infraMachine.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedInfraMachine.Object, true, "status", "initialization", "provisioned")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedInfraMachine, client.MergeFrom(infraMachine))).To(Succeed())

		// Wait until Machine was reconciled.
		g.Eventually(func(g Gomega) bool {
			if err := env.DirectAPIServerGet(ctx, client.ObjectKeyFromObject(machine), machine); err != nil {
				return false
			}
			g.Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseRunning))
			g.Expect(machine.Status.Addresses).To(BeEmpty())
			// Verify that the LastUpdated timestamp was updated
			g.Expect(machine.Status.LastUpdated.IsZero()).To(BeFalse())
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
		// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
		g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())

		g.Expect(env.Create(ctx, bootstrapConfig)).To(Succeed())
		g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
		// We have to subtract 2 seconds, because .status.lastUpdated does not contain milliseconds.
		preUpdate := time.Now().Add(-2 * time.Second)
		// Create and wait on machine to make sure caches sync and reconciliation triggers.
		g.Expect(env.CreateAndWait(ctx, machine)).To(Succeed())

		modifiedMachine := machine.DeepCopy()
		// Set NodeRef.
		machine.Status.NodeRef = clusterv1.MachineNodeReference{Name: node.Name}
		g.Expect(env.Status().Patch(ctx, modifiedMachine, client.MergeFrom(machine))).To(Succeed())

		// Set bootstrap ready.
		modifiedBootstrapConfig := bootstrapConfig.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, true, "status", "initialization", "dataSecretCreated")).To(Succeed())
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, "secret-data", "status", "dataSecretName")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedBootstrapConfig, client.MergeFrom(bootstrapConfig))).To(Succeed())

		// Set infra ready.
		modifiedInfraMachine := infraMachine.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedInfraMachine.Object, true, "status", "initialization", "provisioned")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedInfraMachine, client.MergeFrom(infraMachine))).To(Succeed())

		// Wait until Machine was reconciled.
		g.Eventually(func(g Gomega) bool {
			if err := env.DirectAPIServerGet(ctx, client.ObjectKeyFromObject(machine), machine); err != nil {
				return false
			}
			g.Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseRunning))
			// Verify that the LastUpdated timestamp was updated
			g.Expect(machine.Status.LastUpdated.IsZero()).To(BeFalse())
			g.Expect(machine.Status.LastUpdated.After(preUpdate)).To(BeTrue())
			return true
		}, 10*time.Second).Should(BeTrue())
	})

	t.Run("Should set `Updating` when Machine is in-place updating", func(t *testing.T) {
		utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.InPlaceUpdates, true)

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
		// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
		g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())

		g.Expect(env.Create(ctx, bootstrapConfig)).To(Succeed())
		g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
		// We have to subtract 2 seconds, because .status.lastUpdated does not contain milliseconds.
		preUpdate := time.Now().Add(-2 * time.Second)
		// Create and wait on machine to make sure caches sync and reconciliation triggers.
		g.Expect(env.CreateAndWait(ctx, machine)).To(Succeed())

		modifiedMachine := machine.DeepCopy()
		// Set NodeRef.
		machine.Status.NodeRef = clusterv1.MachineNodeReference{Name: node.Name}
		g.Expect(env.Status().Patch(ctx, modifiedMachine, client.MergeFrom(machine))).To(Succeed())

		// Set bootstrap ready.
		modifiedBootstrapConfig := bootstrapConfig.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, true, "status", "initialization", "dataSecretCreated")).To(Succeed())
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, "secret-data", "status", "dataSecretName")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedBootstrapConfig, client.MergeFrom(bootstrapConfig))).To(Succeed())

		// Set infra ready.
		modifiedInfraMachine := infraMachine.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedInfraMachine.Object, true, "status", "initialization", "provisioned")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedInfraMachine, client.MergeFrom(infraMachine))).To(Succeed())

		// Set annotations on Machine, BootstrapConfig and InfraMachine to trigger an in-place update.
		orig := modifiedBootstrapConfig.DeepCopy()
		annotations.AddAnnotations(modifiedBootstrapConfig, map[string]string{
			clusterv1.UpdateInProgressAnnotation: "",
		})
		g.Expect(env.Patch(ctx, modifiedBootstrapConfig, client.MergeFrom(orig))).To(Succeed())
		orig = modifiedInfraMachine.DeepCopy()
		annotations.AddAnnotations(modifiedInfraMachine, map[string]string{
			clusterv1.UpdateInProgressAnnotation: "",
		})
		g.Expect(env.Patch(ctx, modifiedInfraMachine, client.MergeFrom(orig))).To(Succeed())
		origMachine := modifiedMachine.DeepCopy()
		annotations.AddAnnotations(modifiedMachine, map[string]string{
			runtimev1.PendingHooksAnnotation:     "UpdateMachine",
			clusterv1.UpdateInProgressAnnotation: "",
		})
		g.Expect(env.Patch(ctx, modifiedMachine, client.MergeFrom(origMachine))).To(Succeed())

		// Wait until Machine was reconciled.
		g.Eventually(func(g Gomega) bool {
			if err := env.DirectAPIServerGet(ctx, client.ObjectKeyFromObject(machine), machine); err != nil {
				return false
			}
			g.Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseUpdating))
			// Verify that the LastUpdated timestamp was updated
			g.Expect(machine.Status.LastUpdated.IsZero()).To(BeFalse())
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
		machine.Spec.ProviderID = nodeProviderID

		g.Expect(env.Create(ctx, cluster)).To(Succeed())
		defaultKubeconfigSecret = kubeconfig.GenerateSecret(cluster, kubeconfig.FromEnvTestConfig(env.Config, cluster))
		g.Expect(env.Create(ctx, defaultKubeconfigSecret)).To(Succeed())
		// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
		g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())

		g.Expect(env.Create(ctx, bootstrapConfig)).To(Succeed())
		g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
		// We have to subtract 2 seconds, because .status.lastUpdated does not contain milliseconds.
		preUpdate := time.Now().Add(-2 * time.Second)
		// Create and wait on machine to make sure caches sync and reconciliation triggers.
		g.Expect(env.CreateAndWait(ctx, machine)).To(Succeed())

		// Set bootstrap ready.
		modifiedBootstrapConfig := bootstrapConfig.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, true, "status", "initialization", "dataSecretCreated")).To(Succeed())
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, "secret-data", "status", "dataSecretName")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedBootstrapConfig, client.MergeFrom(bootstrapConfig))).To(Succeed())

		// Set infra ready.
		modifiedInfraMachine := infraMachine.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedInfraMachine.Object, true, "status", "initialization", "provisioned")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedInfraMachine, client.MergeFrom(infraMachine))).To(Succeed())

		// Wait until Machine was reconciled.
		g.Eventually(func(g Gomega) bool {
			if err := env.DirectAPIServerGet(ctx, client.ObjectKeyFromObject(machine), machine); err != nil {
				return false
			}
			g.Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseProvisioned))
			// Verify that the LastUpdated timestamp was updated
			g.Expect(machine.Status.LastUpdated.IsZero()).To(BeFalse())
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
		g.Expect(env.CreateAndWait(ctx, node)).To(Succeed())
		defer func() {
			g.Expect(env.Cleanup(ctx, node)).To(Succeed())
		}()

		g.Expect(env.Create(ctx, cluster)).To(Succeed())
		defaultKubeconfigSecret = kubeconfig.GenerateSecret(cluster, kubeconfig.FromEnvTestConfig(env.Config, cluster))
		g.Expect(env.Create(ctx, defaultKubeconfigSecret)).To(Succeed())
		// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
		g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())

		g.Expect(env.Create(ctx, bootstrapConfig)).To(Succeed())
		g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
		// We have to subtract 2 seconds, because .status.lastUpdated does not contain milliseconds.
		preUpdate := time.Now().Add(-2 * time.Second)
		// Create and wait on machine to make sure caches sync and reconciliation triggers.
		g.Expect(env.CreateAndWait(ctx, machine)).To(Succeed())

		// Set bootstrap ready.
		modifiedBootstrapConfig := bootstrapConfig.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, true, "status", "initialization", "dataSecretCreated")).To(Succeed())
		g.Expect(unstructured.SetNestedField(modifiedBootstrapConfig.Object, "secret-data", "status", "dataSecretName")).To(Succeed())
		g.Expect(env.Status().Patch(ctx, modifiedBootstrapConfig, client.MergeFrom(bootstrapConfig))).To(Succeed())

		// Set infra ready.
		modifiedInfraMachine := infraMachine.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedInfraMachine.Object, true, "status", "initialization", "provisioned")).To(Succeed())
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
		machine.Status.NodeRef = clusterv1.MachineNodeReference{Name: node.Name}
		g.Expect(env.Status().Patch(ctx, modifiedMachine, client.MergeFrom(machine))).To(Succeed())

		modifiedMachine = machine.DeepCopy()
		// Set finalizer so we can check the Machine later, otherwise it would be already gone.
		modifiedMachine.Finalizers = append(modifiedMachine.Finalizers, "test")
		g.Expect(env.Patch(ctx, modifiedMachine, client.MergeFrom(machine))).To(Succeed())

		// Delete Machine
		g.Expect(env.DeleteAndWait(ctx, machine)).To(Succeed())

		// Wait until Machine was reconciled.
		g.Eventually(func(g Gomega) bool {
			if err := env.DirectAPIServerGet(ctx, client.ObjectKeyFromObject(machine), machine); err != nil {
				return false
			}
			g.Expect(machine.Status.GetTypedPhase()).To(Equal(clusterv1.MachinePhaseDeleting))
			nodeHealthyCondition := v1beta1conditions.Get(machine, clusterv1.MachineNodeHealthyV1Beta1Condition)
			g.Expect(nodeHealthyCondition).ToNot(BeNil())
			g.Expect(nodeHealthyCondition.Status).To(Equal(corev1.ConditionFalse))
			g.Expect(nodeHealthyCondition.Reason).To(Equal(clusterv1.DeletingV1Beta1Reason))
			// Verify that the LastUpdated timestamp was updated
			g.Expect(machine.Status.LastUpdated.IsZero()).To(BeFalse())
			g.Expect(machine.Status.LastUpdated.After(preUpdate)).To(BeTrue())
			return true
		}, 10*time.Second).Should(BeTrue())
	})
}
