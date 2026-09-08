/*
Copyright 2026 The Kubernetes Authors.

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

package cel

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/cache"
)

func TestCompile(t *testing.T) {
	tests := []struct {
		name                string
		expression          string
		expectedError       string
		expectedUsesNode    bool
		expectedUsesMachine bool
	}{
		{
			name:                "compiles has_condition",
			expression:          `node.has_condition("Ready", "True")`,
			expectedUsesNode:    true,
			expectedUsesMachine: false,
		},
		{
			name:                "compiles has_condition with reason",
			expression:          `machine.has_condition("Ready", "False", "Reason")`,
			expectedUsesNode:    false,
			expectedUsesMachine: true,
		},
		{
			name:                "compiles has_condition_since",
			expression:          `node.has_condition_since("Ready", "Unknown", "5m")`,
			expectedUsesNode:    true,
			expectedUsesMachine: false,
		},
		{
			name:                "compiles has_condition_since with reason",
			expression:          `machine.has_condition_since("Ready", "False", "Reason", "1h30m")`,
			expectedUsesNode:    false,
			expectedUsesMachine: true,
		},
		{
			name:                "compiles an expression using node and machine",
			expression:          `node.has_condition("Ready", "True") && machine.has_condition("Ready", "True")`,
			expectedUsesNode:    true,
			expectedUsesMachine: true,
		},
		{
			name:                "compiles an expression using neither node nor machine",
			expression:          `true`,
			expectedUsesNode:    false,
			expectedUsesMachine: false,
		},
		{
			name:          "returns an error if the expression does not evaluate to a bool",
			expression:    "node",
			expectedError: "expression must evaluate to a bool, got cel.nodeForCEL",
		},
		{
			name:          "returns an error for an invalid expression",
			expression:    "node.has_condition",
			expectedError: "ERROR: <input>:1:5: undefined field 'has_condition'\n | node.has_condition\n | ....^",
		},
		{
			name:          "returns an error for unknown condition fields",
			expression:    "node.status.conditions.exists(c, c.bogusField == 'Ready')",
			expectedError: "ERROR: <input>:1:5: undefined field 'status'\n | node.status.conditions.exists(c, c.bogusField == 'Ready')\n | ....^",
		},
		{
			name:          "returns an error for machine.spec which is not exposed to CEL",
			expression:    "machine.spec.clusterName == 'foo'",
			expectedError: "ERROR: <input>:1:8: undefined field 'spec'\n | machine.spec.clusterName == 'foo'\n | .......^",
		},
		{
			name:          "returns an error for an invalid status in node.has_condition",
			expression:    `node.has_condition("Ready", "Invalid")`,
			expectedError: "ERROR: <input>:1:29: invalid condition status \"Invalid\", must be one of True, False, Unknown\n | node.has_condition(\"Ready\", \"Invalid\")\n | ............................^",
		},
		{
			name:          "returns an error for an invalid status in node.has_condition_since",
			expression:    `node.has_condition_since("Ready", "Invalid", "5m")`,
			expectedError: "ERROR: <input>:1:35: invalid condition status \"Invalid\", must be one of True, False, Unknown\n | node.has_condition_since(\"Ready\", \"Invalid\", \"5m\")\n | ..................................^",
		},
		{
			name:          "returns an error for a status with wrong casing in machine.has_condition with reason",
			expression:    `machine.has_condition("Ready", "false", "Reason")`,
			expectedError: "ERROR: <input>:1:32: invalid condition status \"false\", must be one of True, False, Unknown\n | machine.has_condition(\"Ready\", \"false\", \"Reason\")\n | ...............................^",
		},
		{
			name:          "returns an error for an invalid status in machine.has_condition_since with reason",
			expression:    `machine.has_condition_since("Ready", "Invalid", "Reason", "5m")`,
			expectedError: "ERROR: <input>:1:38: invalid condition status \"Invalid\", must be one of True, False, Unknown\n | machine.has_condition_since(\"Ready\", \"Invalid\", \"Reason\", \"5m\")\n | .....................................^",
		},
		{
			name:          "returns an error for an invalid duration in node.has_condition_since",
			expression:    `node.has_condition_since("Ready", "False", "invalid")`,
			expectedError: "ERROR: <input>:1:44: invalid duration \"invalid\": time: invalid duration \"invalid\"\n | node.has_condition_since(\"Ready\", \"False\", \"invalid\")\n | ...........................................^",
		},
		{
			name:          "returns an error for an invalid duration in machine.has_condition_since with reason",
			expression:    `machine.has_condition_since("Ready", "False", "Reason", "5 minutes")`,
			expectedError: "ERROR: <input>:1:57: invalid duration \"5 minutes\": time: unknown unit \" minutes\" in duration \"5 minutes\"\n | machine.has_condition_since(\"Ready\", \"False\", \"Reason\", \"5 minutes\")\n | ........................................................^",
		},
		{
			name:          "returns an error for an invalid literal in a nested part of the expression",
			expression:    `node.has_condition("Ready", "True") && machine.has_condition_since("Ready", "Bad", "5m")`,
			expectedError: "ERROR: <input>:1:77: invalid condition status \"Bad\", must be one of True, False, Unknown\n | node.has_condition(\"Ready\", \"True\") && machine.has_condition_since(\"Ready\", \"Bad\", \"5m\")\n | ............................................................................^",
		},
	}

	expressionCache := cache.New[ProgramEntry](t.Context(), 30*time.Minute)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			_, err := Compile(expressionCache, tt.expression)
			if tt.expectedError != "" {
				g.Expect(err).To(MatchError(tt.expectedError))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			// Compiled programs are cached by expression.
			entry, ok := expressionCache.Has(tt.expression)
			g.Expect(ok).To(BeTrue())
			g.Expect(entry.usesNode).To(Equal(tt.expectedUsesNode))
			g.Expect(entry.usesMachine).To(Equal(tt.expectedUsesMachine))
		})
	}
}

func TestEvaluateExpressions(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name                               string
		node                               *corev1.Node
		machine                            *clusterv1.Machine
		expressions                        []clusterv1.UnhealthyExpression
		cacheEntry                         *ExpressionResultEntry
		expectedUnhealthyConditionMessages []string
		expectedNextCheck                  time.Duration
		expectedError                      bool
		expectedCacheEntry                 *ExpressionResultEntry
	}{
		{
			name:    "returns healthy with no expressions",
			node:    nodeWithConditions(),
			machine: machineWithConditions(),
		},
		{
			name:    "returns an error for an invalid expression",
			node:    nodeWithConditions(),
			machine: machineWithConditions(),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: "node.doesNotExist"},
			},
			expectedError: true,
		},
		{
			name:    "returns an error for an invalid expression (invalid status)",
			node:    nodeWithConditions(),
			machine: machineWithConditions(),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `node.has_condition_since("Ready", "Invalid", "5m")`, Message: "condition message"},
			},
			expectedError: true,
		},
		{
			name:    "returns an error for an invalid expression (invalid duration)",
			node:    nodeWithConditions(),
			machine: machineWithConditions(),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `node.has_condition_since("Ready", "Unknown", "invalid")`, Message: "condition message"},
			},
			expectedError: true,
		},
		{
			name:    "returns healthy if expression referencing node and node is nil",
			node:    nil,
			machine: machineWithConditions(),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `node.has_condition_since("Ready", "False", "5m")`, Message: "condition message"},
			},
		},
		{
			name: "return unhealthy matches using only the machine variable when the node is nil, e.g. before the node has been created",
			node: nil,
			machine: machineWithConditions(
				metav1.Condition{Type: "InfrastructureReady", Status: metav1.ConditionFalse, LastTransitionTime: metav1.NewTime(now)},
			),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `machine.has_condition("InfrastructureReady", "False")`, Message: "condition message"},
			},
			expectedUnhealthyConditionMessages: []string{"condition message"},
		},
		// Test all CEL methods to ensure they all work
		{
			name: "returns unhealthy if expressions match",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now.Add(-1 * time.Hour))},
			),
			machine: machineWithConditions(
				metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now.Add(-1 * time.Hour))},
			),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `node.has_condition("Ready", "False")`, Message: "node_has_condition message"},
				{Expression: `node.has_condition("Ready", "False", "SomeReason")`, Message: "node_has_condition_with_reason message"},
				{Expression: `node.has_condition_since("Ready", "False", "5m")`, Message: "node_has_condition_since message"},
				{Expression: `node.has_condition_since("Ready", "False", "SomeReason", "5m")`, Message: "node_has_condition_since_with_reason message"},
				{Expression: `machine.has_condition("Ready", "False")`, Message: "machine_has_condition message"},
				{Expression: `machine.has_condition("Ready", "False", "SomeReason")`, Message: "machine_has_condition_with_reason message"},
				{Expression: `machine.has_condition_since("Ready", "False", "5m")`, Message: "machine_has_condition_since message"},
				{Expression: `machine.has_condition_since("Ready", "False", "SomeReason", "5m")`, Message: "machine_has_condition_since_with_reason message"},
			},
			expectedUnhealthyConditionMessages: []string{
				"node_has_condition message",
				"node_has_condition_with_reason message",
				"node_has_condition_since message",
				"node_has_condition_since_with_reason message",
				"machine_has_condition message",
				"machine_has_condition_with_reason message",
				"machine_has_condition_since message",
				"machine_has_condition_since_with_reason message",
			},
		},
		{
			name: "returns healthy if condition type, status or reason do not match",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now)},
			),
			machine: machineWithConditions(),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `node.has_condition("Different", "False", "SomeReason")`, Message: "condition message"},
				{Expression: `node.has_condition("Ready", "True", "SomeReason")`, Message: "condition message"},
				{Expression: `node.has_condition("Ready", "False", "Different")`, Message: "condition message"},
			},
		},
		// Node
		{
			name: "returns healthy if expression referencing node and node is healthy",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(now)},
			),
			machine: machineWithConditions(),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `node.has_condition_since("Ready", "False", "5m")`, Message: "condition message"},
			},
		},
		{
			name: "returns healthy and next check if expression referencing node and node is becoming unhealthy",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now)},
			),
			machine: machineWithConditions(),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: "node.has_condition_since('Ready', 'False', 'SomeReason', '1h')", Message: "condition message"},
			},
			expectedNextCheck: 60 * time.Minute,
		},
		{
			name: "returns unhealthy if expression referencing node and node is unhealthy",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now.Add(-1 * time.Hour))},
			),
			machine: machineWithConditions(),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: "node.has_condition_since('Ready', 'False', 'SomeReason', '1h')", Message: "condition message"},
			},
			expectedUnhealthyConditionMessages: []string{"condition message"},
		},
		{
			name: "returns unhealthy if expression referencing node and node is unhealthy for a long time",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now.Add(-2 * time.Hour))},
			),
			machine: machineWithConditions(),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: "node.has_condition_since('Ready', 'False', 'SomeReason', '1h')", Message: "condition message"},
			},
			expectedUnhealthyConditionMessages: []string{"condition message"},
		},
		{
			name: "returns unhealthy if expression referencing node and node is becoming healthy",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now)},
			),
			machine: machineWithConditions(),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: "!node.has_condition_since('Ready', 'True', 'SomeReason', '30m')", Message: "condition message"},
			},
			expectedUnhealthyConditionMessages: []string{"condition message"},
			expectedNextCheck:                  30 * time.Minute,
		},
		// Machine
		{
			name: "returns healthy if expression referencing machine and machine is healthy",
			node: nodeWithConditions(),
			machine: machineWithConditions(
				metav1.Condition{Type: "InfrastructureReady", Status: metav1.ConditionTrue, LastTransitionTime: metav1.NewTime(now)},
			),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `machine.has_condition_since("InfrastructureReady", "False", "5m")`, Message: "condition message"},
			},
		},
		{
			name: "returns healthy and next check if expression referencing machine and machine is becoming unhealthy",
			node: nodeWithConditions(),
			machine: machineWithConditions(
				metav1.Condition{Type: "InfrastructureReady", Status: metav1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now)},
			),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: "machine.has_condition_since('InfrastructureReady', 'False', 'SomeReason', '1h')", Message: "condition message"},
			},
			expectedNextCheck: 60 * time.Minute,
		},
		{
			name: "returns unhealthy if expression referencing machine and machine is unhealthy",
			node: nodeWithConditions(),
			machine: machineWithConditions(
				metav1.Condition{Type: "InfrastructureReady", Status: metav1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now.Add(-1 * time.Hour))},
			),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: "machine.has_condition_since('InfrastructureReady', 'False', 'SomeReason', '1h')", Message: "condition message"},
			},
			expectedUnhealthyConditionMessages: []string{"condition message"},
		},
		{
			name: "returns unhealthy if expression referencing machine and machine is unhealthy for a long time",
			node: nodeWithConditions(),
			machine: machineWithConditions(
				metav1.Condition{Type: "InfrastructureReady", Status: metav1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now.Add(-2 * time.Hour))},
			),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: "machine.has_condition_since('InfrastructureReady', 'False', 'SomeReason', '1h')", Message: "condition message"},
			},
			expectedUnhealthyConditionMessages: []string{"condition message"},
		},
		{
			name: "returns unhealthy if expression referencing machine and machine is becoming healthy",
			node: nodeWithConditions(),
			machine: machineWithConditions(
				metav1.Condition{Type: "InfrastructureReady", Status: metav1.ConditionTrue, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now)},
			),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: "!machine.has_condition_since('InfrastructureReady', 'True', 'SomeReason', '30m')", Message: "condition message"},
			},
			expectedUnhealthyConditionMessages: []string{"condition message"},
			expectedNextCheck:                  30 * time.Minute,
		},
		// Node and Machine
		{
			name: "returns unhealthy when expression uses && and both Node and Machine expressions match",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now.Add(-2 * time.Hour))},
			),
			machine: machineWithConditions(
				metav1.Condition{Type: "InfrastructureReady", Status: metav1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now.Add(-45 * time.Minute))},
			),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: "node.has_condition_since('Ready', 'False', 'SomeReason', '1h') && machine.has_condition_since('InfrastructureReady', 'False', 'SomeReason', '30m')", Message: "condition message"},
			},
			expectedUnhealthyConditionMessages: []string{"condition message"},
		},
		{
			name: "returns healthy and next check based on the min next check from Node and Machine",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now)},
			),
			machine: machineWithConditions(
				metav1.Condition{Type: "InfrastructureReady", Status: metav1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now)},
			),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: "node.has_condition_since('Ready', 'False', 'SomeReason', '1h') || machine.has_condition_since('InfrastructureReady', 'False', 'SomeReason', '30m')", Message: "condition message"},
			},
			expectedNextCheck: 30 * time.Minute, // nextCheck is picked from Machine because it's lower than the one from Node
		},
		{
			name: "returns healthy and next check based on the min next check from Machine and Node",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now)},
			),
			machine: machineWithConditions(
				metav1.Condition{Type: "InfrastructureReady", Status: metav1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now)},
			),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: "machine.has_condition_since('InfrastructureReady', 'False', 'SomeReason', '30m') || node.has_condition_since('Ready', 'False', 'SomeReason', '1h')", Message: "condition message"},
			},
			expectedNextCheck: 30 * time.Minute, // nextCheck is picked from Machine because it's lower than the one from Node
		},
		// Short-circuiting
		{
			name: "returns healthy and next check based on Node because Machine is short-circuited",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now)},
			),
			machine: machineWithConditions(
				metav1.Condition{Type: "InfrastructureReady", Status: metav1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now)},
			),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: "node.has_condition_since('Ready', 'False', 'SomeReason', '1h') && machine.has_condition_since('InfrastructureReady', 'False', 'SomeReason', '30m')", Message: "condition message"},
			},
			expectedNextCheck: 60 * time.Minute, // nextCheck is picked from Node, Machine is short-circuited
		},
		{
			name: "returns healthy and next check based on Node because Machine is short-circuited",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now.Add(-2 * time.Hour))},
			),
			machine: machineWithConditions(
				metav1.Condition{Type: "InfrastructureReady", Status: metav1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now)},
			),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: "!node.has_condition_since('Ready', 'True', 'SomeReason', '1h') && machine.has_condition_since('InfrastructureReady', 'False', 'SomeReason', '30m')", Message: "condition message"},
			},
			// there is no nextCheck from Node because it won't flip in the future, Machine is short-circuited
		},
		{
			name: "returns unhealthy and no next check because the Node check won't flip in the future",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now.Add(-2 * time.Hour))},
			),
			machine: machineWithConditions(
				metav1.Condition{Type: "InfrastructureReady", Status: metav1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now)},
			),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: "node.has_condition_since('Ready', 'False', 'SomeReason', '1h') || machine.has_condition_since('InfrastructureReady', 'False', 'SomeReason', '30m')", Message: "condition message"},
			},
			expectedUnhealthyConditionMessages: []string{"condition message"},
			// there is no nextCheck from Node because it won't flip in the future, Machine is short-circuited
		},
		{
			name: "returns unhealthy and next check because Node condition might flip in the future",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now)},
			),
			machine: machineWithConditions(
				metav1.Condition{Type: "InfrastructureReady", Status: metav1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now)},
			),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: "!node.has_condition_since('Ready', 'True', 'SomeReason', '1h') || machine.has_condition_since('InfrastructureReady', 'False', 'SomeReason', '30m')", Message: "condition message"},
			},
			expectedUnhealthyConditionMessages: []string{"condition message"},
			expectedNextCheck:                  60 * time.Minute, // nextCheck is picked from Node, Machine is short-circuited
		},
		{
			name: "returns unhealthy and no next check because the Node check won't flip in the future (using !)",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now)},
			),
			machine: machineWithConditions(
				metav1.Condition{Type: "InfrastructureReady", Status: metav1.ConditionFalse, Reason: "SomeReason", LastTransitionTime: metav1.NewTime(now)},
			),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: "!node.has_condition_since('Ready', 'True', 'SomeReason', '1h') || machine.has_condition_since('InfrastructureReady', 'False', 'SomeReason', '30m')", Message: "condition message"},
			},
			expectedUnhealthyConditionMessages: []string{"condition message"},
			// there is no nextCheck from Node because it won't flip in the future, Machine is short-circuited
		},
		// Other use cases
		{
			name: "returns healthy when Node is not ready but Node is in maintenance",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionFalse, LastTransitionTime: metav1.NewTime(now.Add(-5 * time.Minute))},
				corev1.NodeCondition{Type: "MaintenanceInProgress", Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(now.Add(-10 * time.Minute))},
			),
			machine: machineWithConditions(),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `node.has_condition_since("Ready", "False", "5m") && !node.has_condition("MaintenanceInProgress", "True")`, Message: "condition message"},
			},
		},
		{
			name: "returns healthy when Node is not ready but Machine is in maintenance",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionFalse, LastTransitionTime: metav1.NewTime(now.Add(-10 * time.Minute))},
			),
			machine: machineWithConditions(
				metav1.Condition{Type: "InfrastructureReady", Status: metav1.ConditionFalse, Reason: "Maintenance", LastTransitionTime: metav1.NewTime(now.Add(-5 * time.Minute))},
			),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `node.has_condition_since("Ready", "False", "5m") && !machine.has_condition("InfrastructureReady", "False", "Maintenance")`, Message: "condition message"},
			},
		},
		{
			name: "returns healthy when Machine is not ready but Machine is in maintenance",
			node: nodeWithConditions(),
			machine: machineWithConditions(
				metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "Maintenance", LastTransitionTime: metav1.NewTime(now.Add(-5 * time.Minute))},
				metav1.Condition{Type: "InfrastructureReady", Status: metav1.ConditionFalse, Reason: "Maintenance", LastTransitionTime: metav1.NewTime(now.Add(-5 * time.Minute))},
			),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `machine.has_condition_since("Ready", "False", "5m") && !machine.has_condition("InfrastructureReady", "False", "Maintenance")`, Message: "condition message"},
			},
		},
		// Multiple expressions
		{
			name: "returns unhealthy with multiple expressions and picks the min nextCheck",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(now)},
			),
			machine: machineWithConditions(
				metav1.Condition{Type: "InfrastructureReady", Status: metav1.ConditionTrue, LastTransitionTime: metav1.NewTime(now)},
			),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `!node.has_condition_since("Ready", "True", "5m")`, Message: "Node condition message"},
				{Expression: `!machine.has_condition_since("InfrastructureReady", "True", "10m")`, Message: "Machine condition message"},
			},
			expectedUnhealthyConditionMessages: []string{"Node condition message", "Machine condition message"},
			expectedNextCheck:                  5 * time.Minute,
		},
		{
			name: "returns unhealthy with multiple expressions and it won't flip",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionFalse, LastTransitionTime: metav1.NewTime(now.Add(-10 * time.Minute))},
			),
			machine: machineWithConditions(
				metav1.Condition{Type: "InfrastructureReady", Status: metav1.ConditionFalse, LastTransitionTime: metav1.NewTime(now.Add(-15 * time.Minute))},
			),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `node.has_condition_since("Ready", "False", "5m")`, Message: "Node condition message"},
				{Expression: `machine.has_condition_since("InfrastructureReady", "False", "10m")`, Message: "Machine condition message"},
			},
			expectedUnhealthyConditionMessages: []string{"Node condition message", "Machine condition message"},
		},
		{
			name: "returns healthy with multiple expressions and picks the min nextCheck",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionFalse, LastTransitionTime: metav1.NewTime(now)},
			),
			machine: machineWithConditions(
				metav1.Condition{Type: "InfrastructureReady", Status: metav1.ConditionFalse, LastTransitionTime: metav1.NewTime(now)},
			),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `node.has_condition_since("Ready", "False", "5m")`, Message: "Node condition message"},
				{Expression: `machine.has_condition_since("InfrastructureReady", "False", "10m")`, Message: "Machine condition message"},
			},
			expectedNextCheck: 5 * time.Minute,
		},
		{
			name: "returns healthy with multiple expressions and it won't flip in the future",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeReady, Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(now.Add(-10 * time.Minute))},
			),
			machine: machineWithConditions(
				metav1.Condition{Type: "InfrastructureReady", Status: metav1.ConditionTrue, LastTransitionTime: metav1.NewTime(now.Add(-15 * time.Minute))},
			),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `!node.has_condition_since("Ready", "True", "5m")`, Message: "Node condition message"},
				{Expression: `!machine.has_condition_since("InfrastructureReady", "True", "10m")`, Message: "Machine condition message"},
			},
		},
		// Caching
		{
			name:    "evaluates expression if MHC generation changed",
			machine: machineWithConditions(),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `true`, Message: "condition message"},
			},
			cacheEntry: &ExpressionResultEntry{
				MachineHealthCheckGeneration: 99, // different from the generation used for MHC in this test
			},
			expectedUnhealthyConditionMessages: []string{"condition message"},
		},
		{
			name:    "evaluates expression if Node showed up",
			node:    nodeWithConditions(),
			machine: machineWithConditions(),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `true`, Message: "condition message"},
			},
			cacheEntry: &ExpressionResultEntry{
				MachineHealthCheckGeneration: 5,
				// Node was created, i.e. it is not set in the cache
			},
			expectedUnhealthyConditionMessages: []string{"condition message"},
		},
		{
			name: "evaluates expression if Node was deleted",
			// Node was deleted
			machine: machineWithConditions(),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `true`, Message: "condition message"},
			},
			cacheEntry: &ExpressionResultEntry{
				MachineHealthCheckGeneration: 5,
				NodeKey:                      client.ObjectKeyFromObject(nodeWithConditions()),
			},
			expectedUnhealthyConditionMessages: []string{"condition message"},
		},
		{
			name:    "evaluates expression if Node resourceVersion changed",
			node:    nodeWithConditions(),
			machine: machineWithConditions(),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `true`, Message: "condition message"},
			},
			cacheEntry: &ExpressionResultEntry{
				MachineHealthCheckGeneration: 5,
				NodeKey:                      client.ObjectKeyFromObject(nodeWithConditions()),
				NodeResourceVersion:          "99", // different from the Node resourceVersion
			},
			expectedUnhealthyConditionMessages: []string{"condition message"},
		},
		{
			name:    "evaluates expression if Machine resourceVersion changed",
			node:    nodeWithConditions(),
			machine: machineWithConditions(),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `true`, Message: "condition message"},
			},
			cacheEntry: &ExpressionResultEntry{
				MachineHealthCheckGeneration: 5,
				NodeKey:                      client.ObjectKeyFromObject(nodeWithConditions()),
				NodeResourceVersion:          "13",
				MachineResourceVersion:       "99", // different from the Node resourceVersion
			},
			expectedUnhealthyConditionMessages: []string{"condition message"},
		},
		{
			name:    "uses cache entry if NextCheckTime is not set",
			node:    nodeWithConditions(),
			machine: machineWithConditions(),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `true`, Message: "condition message"},
			},
			cacheEntry: &ExpressionResultEntry{
				MachineHealthCheckGeneration: 5,
				NodeKey:                      client.ObjectKeyFromObject(nodeWithConditions()),
				NodeResourceVersion:          "13",
				MachineResourceVersion:       "15",
				// NextCheckTime is not set
				UnhealthyConditionsMessages: []string{"condition message from cache"},
			},
			expectedUnhealthyConditionMessages: []string{"condition message from cache"},
		},
		{
			name:    "evaluates expression if NextCheckTime is in the past",
			node:    nodeWithConditions(),
			machine: machineWithConditions(),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `true`, Message: "condition message"},
			},
			cacheEntry: &ExpressionResultEntry{
				MachineHealthCheckGeneration: 5,
				NodeKey:                      client.ObjectKeyFromObject(nodeWithConditions()),
				NodeResourceVersion:          "13",
				MachineResourceVersion:       "15",
				NextCheckTime:                new(now.Add(-15 * time.Minute)),
			},
			expectedUnhealthyConditionMessages: []string{"condition message"},
		},
		{
			name:    "evaluates expression if NextCheckTime is now",
			node:    nodeWithConditions(),
			machine: machineWithConditions(),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `true`, Message: "condition message"},
			},
			cacheEntry: &ExpressionResultEntry{
				MachineHealthCheckGeneration: 5,
				NodeKey:                      client.ObjectKeyFromObject(nodeWithConditions()),
				NodeResourceVersion:          "13",
				MachineResourceVersion:       "15",
				NextCheckTime:                new(now),
			},
			expectedUnhealthyConditionMessages: []string{"condition message"},
		},
		{
			name:    "uses cache entry if NextCheckTime is in the future",
			node:    nodeWithConditions(),
			machine: machineWithConditions(),
			expressions: []clusterv1.UnhealthyExpression{
				{Expression: `true`, Message: "condition message"},
			},
			cacheEntry: &ExpressionResultEntry{
				MachineHealthCheckGeneration: 5,
				NodeKey:                      client.ObjectKeyFromObject(nodeWithConditions()),
				NodeResourceVersion:          "13",
				MachineResourceVersion:       "15",
				NextCheckTime:                new(now.Add(15 * time.Minute)),
				UnhealthyConditionsMessages:  []string{"condition message from cache"},
			},
			expectedUnhealthyConditionMessages: []string{"condition message from cache"},
			expectedNextCheck:                  15 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			c := cache.New[ExpressionResultEntry](t.Context(), 30*time.Minute)

			mhc := &clusterv1.MachineHealthCheck{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "mhc",
					Namespace:  metav1.NamespaceDefault,
					Generation: 5,
				},
				Spec: clusterv1.MachineHealthCheckSpec{
					Checks: clusterv1.MachineHealthCheckChecks{
						UnhealthyExpressions: tt.expressions,
					},
				},
			}

			if tt.cacheEntry != nil {
				tt.cacheEntry.MachineHealthCheckKey = client.ObjectKeyFromObject(mhc)
				tt.cacheEntry.MachineKey = client.ObjectKeyFromObject(tt.machine)
				c.Add(*tt.cacheEntry)
			}

			expressionCache := cache.New[ProgramEntry](t.Context(), 1*time.Hour)
			unhealthyConditionsMessages, nextCheck, err := EvaluateExpressions(expressionCache, c, mhc, tt.node, tt.machine, now)
			if tt.expectedError {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(unhealthyConditionsMessages).To(Equal(tt.expectedUnhealthyConditionMessages))
			g.Expect(nextCheck).To(Equal(tt.expectedNextCheck))

			if len(tt.expressions) == 0 {
				return
			}

			entry, ok := c.Has(ExpressionResultEntry{
				MachineHealthCheckKey: client.ObjectKeyFromObject(mhc),
				MachineKey:            client.ObjectKeyFromObject(tt.machine),
			}.Key())
			g.Expect(ok).To(BeTrue())

			if tt.expectedCacheEntry != nil {
				g.Expect(entry).To(Equal(*tt.expectedCacheEntry))
			} else {
				g.Expect(entry.MachineHealthCheckKey).To(Equal(client.ObjectKeyFromObject(mhc)))
				g.Expect(entry.MachineHealthCheckGeneration).To(Equal(mhc.Generation))
				g.Expect(entry.MachineKey).To(Equal(client.ObjectKeyFromObject(tt.machine)))
				g.Expect(entry.MachineResourceVersion).To(Equal(tt.machine.ResourceVersion))
				g.Expect(entry.UnhealthyConditionsMessages).To(Equal(tt.expectedUnhealthyConditionMessages))
				if tt.expectedNextCheck == 0 {
					g.Expect(entry.NextCheckTime).To(BeNil())
				} else {
					g.Expect(*entry.NextCheckTime).To(Equal(now.Add(tt.expectedNextCheck)))
				}
				if tt.node == nil {
					g.Expect(entry.NodeKey).To(Equal(client.ObjectKey{}))
					g.Expect(entry.NodeResourceVersion).To(BeEmpty())
				} else {
					g.Expect(entry.NodeKey).To(Equal(client.ObjectKeyFromObject(tt.node)))
					g.Expect(entry.NodeResourceVersion).To(Equal(tt.node.ResourceVersion))
				}
			}
		})
	}
}

func nodeWithConditions(conditions ...corev1.NodeCondition) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "node",
			Namespace:       metav1.NamespaceDefault,
			ResourceVersion: "13",
		},
		Status: corev1.NodeStatus{
			Conditions: conditions,
		},
	}
}

func machineWithConditions(conditions ...metav1.Condition) *clusterv1.Machine {
	return &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "machine",
			Namespace:       metav1.NamespaceDefault,
			ResourceVersion: "15",
		},
		Status: clusterv1.MachineStatus{
			Conditions: conditions,
		},
	}
}
