/*
Copyright 2020 The Kubernetes Authors.

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

package machinehealthcheck

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
)

func TestGetTargetsFromMHC(t *testing.T) {
	namespace := "test-mhc"
	clusterName := "test-cluster"

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      clusterName,
		},
	}

	mhcSelector := map[string]string{"cluster": clusterName, "machine-group": "foo"}

	// Create a namespace for the tests
	testNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "mhc-test"}}

	// Create a test MHC
	testMHC := &clusterv1.MachineHealthCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mhc",
			Namespace: namespace,
		},
		Spec: clusterv1.MachineHealthCheckSpec{
			ClusterName: clusterName,
			Selector: metav1.LabelSelector{
				MatchLabels: mhcSelector,
			},
			UnhealthyConditions: []clusterv1.UnhealthyCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionUnknown,
					Timeout: metav1.Duration{Duration: 5 * time.Minute},
				},
			},
		},
	}

	baseObjects := []client.Object{testNS, cluster, testMHC}

	// Initialise some test machines and nodes for use in the test cases

	testNode1 := newTestNode("node1")
	testMachine1 := newTestMachine("machine1", namespace, clusterName, testNode1.Name, mhcSelector)
	testNode2 := newTestNode("node2")
	testMachine2 := newTestMachine("machine2", namespace, clusterName, testNode2.Name, map[string]string{"cluster": clusterName})
	testNode3 := newTestNode("node3")
	testMachine3 := newTestMachine("machine3", namespace, clusterName, testNode3.Name, mhcSelector)
	testNode4 := newTestNode("node4")
	testMachine4 := newTestMachine("machine4", namespace, "other-cluster", testNode4.Name, mhcSelector)

	// machines for skip remediation
	testNode5 := newTestNode("node5")
	testMachine5 := newTestMachine("machine5", namespace, clusterName, testNode5.Name, mhcSelector)
	testMachine5.Annotations = map[string]string{clusterv1.MachineSkipRemediationAnnotation: ""}
	testNode6 := newTestNode("node6")
	testMachine6 := newTestMachine("machine6", namespace, clusterName, testNode6.Name, mhcSelector)
	testMachine6.Annotations = map[string]string{clusterv1.PausedAnnotation: ""}

	testCases := []struct {
		desc            string
		toCreate        []client.Object
		expectedTargets []healthCheckTarget
	}{
		{
			desc:            "with no matching machines",
			toCreate:        baseObjects,
			expectedTargets: nil,
		},
		{
			desc:     "when a machine's node is missing",
			toCreate: append(baseObjects, testMachine1),
			expectedTargets: []healthCheckTarget{
				{
					Machine:     testMachine1,
					MHC:         testMHC,
					Node:        nil,
					nodeMissing: true,
				},
			},
		},
		{
			desc:     "when a machine's labels do not match the selector",
			toCreate: append(baseObjects, testMachine1, testMachine2, testNode1),
			expectedTargets: []healthCheckTarget{
				{
					Machine: testMachine1,
					MHC:     testMHC,
					Node:    testNode1,
				},
			},
		},
		{
			desc:     "with multiple machines, should match correct nodes",
			toCreate: append(baseObjects, testNode1, testMachine1, testNode3, testMachine3, testNode4, testMachine4),
			expectedTargets: []healthCheckTarget{
				{
					Machine: testMachine1,
					MHC:     testMHC,
					Node:    testNode1,
				},
				{
					Machine: testMachine3,
					MHC:     testMHC,
					Node:    testNode3,
				},
			},
		},
		{
			desc:     "with machines having skip-remediation or paused annotation",
			toCreate: append(baseObjects, testNode1, testMachine1, testMachine5, testMachine6),
			expectedTargets: []healthCheckTarget{
				{
					Machine: testMachine1,
					MHC:     testMHC,
					Node:    testNode1,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			gs := NewGomegaWithT(t)

			k8sClient := fake.NewClientBuilder().WithObjects(tc.toCreate...).Build()

			// Create a test reconciler
			reconciler := &Reconciler{
				Client: k8sClient,
			}
			for _, t := range tc.expectedTargets {
				patchHelper, err := patch.NewHelper(t.Machine, k8sClient)
				gs.Expect(err).ToNot(HaveOccurred())
				t.patchHelper = patchHelper
			}

			targets, err := reconciler.getTargetsFromMHC(ctx, ctrl.LoggerFrom(ctx), k8sClient, cluster, testMHC)
			gs.Expect(err).ToNot(HaveOccurred())

			gs.Expect(targets).To(HaveLen(len(tc.expectedTargets)))
			for i, target := range targets {
				expectedTarget := tc.expectedTargets[i]
				gs.Expect(target.Machine).To(BeComparableTo(expectedTarget.Machine))
				gs.Expect(target.MHC).To(BeComparableTo(expectedTarget.MHC))
				gs.Expect(target.Node).To(BeComparableTo(expectedTarget.Node))
			}
		})
	}
}

func TestHealthCheckTargets(t *testing.T) {
	namespace := "test-mhc"
	clusterName := "test-cluster"

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      clusterName,
		},
	}
	conditions.MarkTrue(cluster, clusterv1.InfrastructureReadyCondition)
	conditions.MarkTrue(cluster, clusterv1.ControlPlaneInitializedCondition)

	// Ensure the control plane was initialized earlier to prevent it interfering with
	// NodeStartupTimeout testing.
	conds := clusterv1.Conditions{}
	for _, condition := range cluster.GetConditions() {
		condition.LastTransitionTime = metav1.NewTime(condition.LastTransitionTime.Add(-1 * time.Hour))
		conds = append(conds, condition)
	}
	cluster.SetConditions(conds)

	mhcSelector := map[string]string{"cluster": clusterName, "machine-group": "foo"}

	timeoutForMachineToHaveNode := 10 * time.Minute
	disabledTimeoutForMachineToHaveNode := time.Duration(0)
	timeoutForUnhealthyConditions := 5 * time.Minute

	// Create a test MHC
	testMHC := &clusterv1.MachineHealthCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mhc",
			Namespace: namespace,
		},
		Spec: clusterv1.MachineHealthCheckSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: mhcSelector,
			},
			ClusterName: clusterName,
			UnhealthyConditions: []clusterv1.UnhealthyCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionUnknown,
					Timeout: metav1.Duration{Duration: timeoutForUnhealthyConditions},
				},
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionFalse,
					Timeout: metav1.Duration{Duration: timeoutForUnhealthyConditions},
				},
			},
		},
	}

	testMachine := newTestMachine("machine1", namespace, clusterName, "node1", mhcSelector)

	// Targets for when the node has not yet been seen by the Machine controller
	testMachineCreated1200s := testMachine.DeepCopy()
	nowMinus1200s := metav1.NewTime(time.Now().Add(-1200 * time.Second))
	testMachineCreated1200s.ObjectMeta.CreationTimestamp = nowMinus1200s

	nodeNotYetStartedTarget1200s := healthCheckTarget{
		Cluster: cluster,
		MHC:     testMHC,
		Machine: testMachineCreated1200s,
		Node:    nil,
	}
	nodeNotYetStartedTarget1200sCondition := newFailedHealthCheckCondition(clusterv1.NodeStartupTimeoutReason, "Node failed to report startup in %s", timeoutForMachineToHaveNode)

	testMachineCreated400s := testMachine.DeepCopy()
	nowMinus400s := metav1.NewTime(time.Now().Add(-400 * time.Second))
	testMachineCreated400s.ObjectMeta.CreationTimestamp = nowMinus400s

	nodeNotYetStartedTarget400s := healthCheckTarget{
		Cluster: cluster,
		MHC:     testMHC,
		Machine: testMachineCreated400s,
		Node:    nil,
	}

	// Target for when the Node has been seen, but has now gone
	nodeGoneAway := healthCheckTarget{
		Cluster:     cluster,
		MHC:         testMHC,
		Machine:     testMachine.DeepCopy(),
		Node:        &corev1.Node{},
		nodeMissing: true,
	}
	nodeGoneAwayCondition := newFailedHealthCheckCondition(clusterv1.NodeNotFoundReason, "")

	// Create a test MHC without conditions
	testMHCEmptyConditions := &clusterv1.MachineHealthCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mhc",
			Namespace: namespace,
		},
		Spec: clusterv1.MachineHealthCheckSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: mhcSelector,
			},
			ClusterName: clusterName,
		},
	}
	// Target for when the Node has been seen, but has now gone
	// using MHC without unhealthyConditions
	nodeGoneAwayEmptyConditions := healthCheckTarget{
		Cluster: cluster,
		MHC:     testMHCEmptyConditions,
		Machine: testMachine.DeepCopy(),
		Node: &corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
		},
		nodeMissing: true,
	}
	nodeEmptyConditions := healthCheckTarget{
		Cluster: cluster,
		MHC:     testMHCEmptyConditions,
		Machine: testMachine.DeepCopy(),
		Node: &corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
		},
		nodeMissing: false,
	}

	// Target for when the node has been in an unknown state for shorter than the timeout
	testNodeUnknown200 := newTestUnhealthyNode("node1", corev1.NodeReady, corev1.ConditionUnknown, 200*time.Second)
	nodeUnknown200 := healthCheckTarget{
		Cluster:     cluster,
		MHC:         testMHC,
		Machine:     testMachine.DeepCopy(),
		Node:        testNodeUnknown200,
		nodeMissing: false,
	}

	// Second Target for when the node has been in an unknown state for shorter than the timeout
	testNodeUnknown100 := newTestUnhealthyNode("node1", corev1.NodeReady, corev1.ConditionUnknown, 100*time.Second)
	nodeUnknown100 := healthCheckTarget{
		Cluster:     cluster,
		MHC:         testMHC,
		Machine:     testMachine.DeepCopy(),
		Node:        testNodeUnknown100,
		nodeMissing: false,
	}

	// Target for when the node has been in an unknown state for longer than the timeout
	testNodeUnknown400 := newTestUnhealthyNode("node1", corev1.NodeReady, corev1.ConditionUnknown, 400*time.Second)
	nodeUnknown400 := healthCheckTarget{
		Cluster:     cluster,
		MHC:         testMHC,
		Machine:     testMachine.DeepCopy(),
		Node:        testNodeUnknown400,
		nodeMissing: false,
	}
	nodeUnknown400Condition := newFailedHealthCheckCondition(clusterv1.UnhealthyNodeConditionReason, "Condition Ready on node is reporting status Unknown for more than %s", timeoutForUnhealthyConditions)

	// Target for when a node is healthy
	testNodeHealthy := newTestNode("node1")
	testNodeHealthy.UID = "12345"
	nodeHealthy := healthCheckTarget{
		Cluster:     cluster,
		MHC:         testMHC,
		Machine:     testMachine.DeepCopy(),
		Node:        testNodeHealthy,
		nodeMissing: false,
	}

	// Target for when the machine has a failure reason
	failureReason := errors.UpdateMachineError
	testMachineFailureReason := testMachine.DeepCopy()
	testMachineFailureReason.Status.FailureReason = &failureReason
	machineFailureReason := healthCheckTarget{
		Cluster: cluster,
		MHC:     testMHC,
		Machine: testMachineFailureReason,
		Node:    nil,
	}
	machineFailureReasonCondition := newFailedHealthCheckCondition(clusterv1.MachineHasFailureReason, "FailureReason: %s", failureReason)

	// Target for when the machine has a failure message
	failureMsg := "some failure message"
	testMachineFailureMsg := testMachine.DeepCopy()
	testMachineFailureMsg.Status.FailureMessage = &failureMsg
	machineFailureMsg := healthCheckTarget{
		Cluster: cluster,
		MHC:     testMHC,
		Machine: testMachineFailureMsg,
		Node:    nil,
	}
	machineFailureMsgCondition := newFailedHealthCheckCondition(clusterv1.MachineHasFailureReason, "FailureMessage: %s", failureMsg)

	// Target for when the machine has the remediate machine annotation
	annotationRemediationMsg := "Marked for remediation via remediate-machine annotation"
	testMachineAnnotationRemediation := testMachine.DeepCopy()
	testMachineAnnotationRemediation.Annotations = map[string]string{clusterv1.RemediateMachineAnnotation: ""}
	machineAnnotationRemediation := healthCheckTarget{
		Cluster: cluster,
		MHC:     testMHC,
		Machine: testMachineAnnotationRemediation,
		Node:    nil,
	}
	machineAnnotationRemediationCondition := newFailedHealthCheckCondition(clusterv1.HasRemediateMachineAnnotationReason, annotationRemediationMsg)

	testCases := []struct {
		desc                              string
		targets                           []healthCheckTarget
		timeoutForMachineToHaveNode       *time.Duration
		expectedHealthy                   []healthCheckTarget
		expectedNeedsRemediation          []healthCheckTarget
		expectedNeedsRemediationCondition []clusterv1.Condition
		expectedNextCheckTimes            []time.Duration
	}{
		{
			desc:                     "when the node has not yet started for shorter than the timeout",
			targets:                  []healthCheckTarget{nodeNotYetStartedTarget400s},
			expectedHealthy:          []healthCheckTarget{},
			expectedNeedsRemediation: []healthCheckTarget{},
			expectedNextCheckTimes:   []time.Duration{timeoutForMachineToHaveNode - 400*time.Second},
		},
		{
			desc:                              "when the node has not yet started for longer than the timeout",
			targets:                           []healthCheckTarget{nodeNotYetStartedTarget1200s},
			expectedHealthy:                   []healthCheckTarget{},
			expectedNeedsRemediation:          []healthCheckTarget{nodeNotYetStartedTarget1200s},
			expectedNeedsRemediationCondition: []clusterv1.Condition{nodeNotYetStartedTarget1200sCondition},
			expectedNextCheckTimes:            []time.Duration{},
		},
		{
			desc:                              "when the node has gone away",
			targets:                           []healthCheckTarget{nodeGoneAway},
			expectedHealthy:                   []healthCheckTarget{},
			expectedNeedsRemediation:          []healthCheckTarget{nodeGoneAway},
			expectedNeedsRemediationCondition: []clusterv1.Condition{nodeGoneAwayCondition},
			expectedNextCheckTimes:            []time.Duration{},
		},
		{
			desc:                     "when the node has been in an unknown state for shorter than the timeout",
			targets:                  []healthCheckTarget{nodeUnknown200},
			expectedHealthy:          []healthCheckTarget{},
			expectedNeedsRemediation: []healthCheckTarget{},
			expectedNextCheckTimes:   []time.Duration{100 * time.Second},
		},
		{
			desc:                              "when the node has been in an unknown state for longer than the timeout",
			targets:                           []healthCheckTarget{nodeUnknown400},
			expectedHealthy:                   []healthCheckTarget{},
			expectedNeedsRemediation:          []healthCheckTarget{nodeUnknown400},
			expectedNeedsRemediationCondition: []clusterv1.Condition{nodeUnknown400Condition},
			expectedNextCheckTimes:            []time.Duration{},
		},
		{
			desc:                     "when the node is healthy",
			targets:                  []healthCheckTarget{nodeHealthy},
			expectedHealthy:          []healthCheckTarget{nodeHealthy},
			expectedNeedsRemediation: []healthCheckTarget{},
			expectedNextCheckTimes:   []time.Duration{},
		},
		{
			desc:                              "with a mix of healthy and unhealthy nodes",
			targets:                           []healthCheckTarget{nodeUnknown100, nodeUnknown200, nodeUnknown400, nodeHealthy},
			expectedHealthy:                   []healthCheckTarget{nodeHealthy},
			expectedNeedsRemediation:          []healthCheckTarget{nodeUnknown400},
			expectedNeedsRemediationCondition: []clusterv1.Condition{nodeUnknown400Condition},
			expectedNextCheckTimes:            []time.Duration{200 * time.Second, 100 * time.Second},
		},
		{
			desc:                        "when the node has not started for a long time but the startup timeout is disabled",
			targets:                     []healthCheckTarget{nodeNotYetStartedTarget400s},
			timeoutForMachineToHaveNode: &disabledTimeoutForMachineToHaveNode,
			expectedHealthy:             []healthCheckTarget{}, // The node is not healthy as it does not have a machine
			expectedNeedsRemediation:    []healthCheckTarget{},
			expectedNextCheckTimes:      []time.Duration{}, // We don't have a timeout so no way to know when to re-check
		},
		{
			desc:                              "when the machine has a failure reason",
			targets:                           []healthCheckTarget{machineFailureReason},
			expectedHealthy:                   []healthCheckTarget{},
			expectedNeedsRemediation:          []healthCheckTarget{machineFailureReason},
			expectedNeedsRemediationCondition: []clusterv1.Condition{machineFailureReasonCondition},
			expectedNextCheckTimes:            []time.Duration{},
		},
		{
			desc:                              "when the machine has a failure message",
			targets:                           []healthCheckTarget{machineFailureMsg},
			expectedHealthy:                   []healthCheckTarget{},
			expectedNeedsRemediation:          []healthCheckTarget{machineFailureMsg},
			expectedNeedsRemediationCondition: []clusterv1.Condition{machineFailureMsgCondition},
			expectedNextCheckTimes:            []time.Duration{},
		},
		{
			desc:                              "when the machine is manually marked for remediation",
			targets:                           []healthCheckTarget{machineAnnotationRemediation},
			expectedHealthy:                   []healthCheckTarget{},
			expectedNeedsRemediation:          []healthCheckTarget{machineAnnotationRemediation},
			expectedNeedsRemediationCondition: []clusterv1.Condition{machineAnnotationRemediationCondition},
			expectedNextCheckTimes:            []time.Duration{},
		},
		{
			desc:                              "health check with empty unhealthy conditions and missing node",
			targets:                           []healthCheckTarget{nodeGoneAwayEmptyConditions},
			expectedHealthy:                   []healthCheckTarget{},
			expectedNeedsRemediation:          []healthCheckTarget{nodeGoneAwayEmptyConditions},
			expectedNeedsRemediationCondition: []clusterv1.Condition{nodeGoneAwayCondition},
			expectedNextCheckTimes:            []time.Duration{},
		},
		{
			desc:                              "health check with empty unhealthy conditions and node",
			targets:                           []healthCheckTarget{nodeEmptyConditions},
			expectedHealthy:                   []healthCheckTarget{nodeEmptyConditions},
			expectedNeedsRemediation:          []healthCheckTarget{},
			expectedNeedsRemediationCondition: []clusterv1.Condition{},
			expectedNextCheckTimes:            []time.Duration{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			gs := NewWithT(t)

			// Create a test reconciler.
			reconciler := &Reconciler{
				recorder: record.NewFakeRecorder(5),
			}

			// Allow individual test cases to override the timeoutForMachineToHaveNode.
			timeout := metav1.Duration{Duration: timeoutForMachineToHaveNode}
			if tc.timeoutForMachineToHaveNode != nil {
				timeout.Duration = *tc.timeoutForMachineToHaveNode
			}

			healthy, unhealthy, nextCheckTimes := reconciler.healthCheckTargets(tc.targets, ctrl.LoggerFrom(ctx), timeout)

			// Round durations down to nearest second account for minute differences
			// in timing when running tests
			roundDurations := func(in []time.Duration) []time.Duration {
				out := []time.Duration{}
				for _, d := range in {
					out = append(out, d.Truncate(time.Second))
				}
				return out
			}

			// Remove the last transition time of the given conditions. Used for comparison with expected conditions.
			removeLastTransitionTimes := func(in clusterv1.Conditions) clusterv1.Conditions {
				out := clusterv1.Conditions{}
				for _, c := range in {
					withoutTime := c.DeepCopy()
					withoutTime.LastTransitionTime = metav1.Time{}
					out = append(out, *withoutTime)
				}
				return out
			}

			gs.Expect(healthy).To(ConsistOf(tc.expectedHealthy))
			gs.Expect(unhealthy).To(ConsistOf(tc.expectedNeedsRemediation))
			gs.Expect(nextCheckTimes).To(WithTransform(roundDurations, ConsistOf(tc.expectedNextCheckTimes)))
			for i, expectedMachineCondition := range tc.expectedNeedsRemediationCondition {
				actualConditions := unhealthy[i].Machine.GetConditions()
				conditionsMatcher := WithTransform(removeLastTransitionTimes, ContainElements(expectedMachineCondition))
				gs.Expect(actualConditions).To(conditionsMatcher)
			}
		})
	}
}

func newTestMachine(name, namespace, clusterName, nodeName string, labels map[string]string) *clusterv1.Machine {
	// Copy the labels so that the map is unique to each test Machine
	l := make(map[string]string)
	for k, v := range labels {
		l[k] = v
	}
	l[clusterv1.ClusterNameLabel] = clusterName

	bootstrap := "bootstrap"
	return &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    l,
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: clusterName,
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: &bootstrap,
			},
		},
		Status: clusterv1.MachineStatus{
			InfrastructureReady: true,
			BootstrapReady:      true,
			Phase:               string(clusterv1.MachinePhaseRunning),
			NodeRef: &corev1.ObjectReference{
				Name: nodeName,
			},
		},
	}
}

func newTestNode(name string) *corev1.Node {
	return &corev1.Node{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Node",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func newTestUnhealthyNode(name string, condition corev1.NodeConditionType, status corev1.ConditionStatus, unhealthyDuration time.Duration) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  "12345",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:               condition,
					Status:             status,
					LastTransitionTime: metav1.NewTime(time.Now().Add(-unhealthyDuration)),
				},
			},
		},
	}
}

func newFailedHealthCheckCondition(reason string, messageFormat string, messageArgs ...interface{}) clusterv1.Condition {
	return *conditions.FalseCondition(clusterv1.MachineHealthCheckSucceededCondition, reason, clusterv1.ConditionSeverityWarning, messageFormat, messageArgs...)
}
