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
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
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
			Checks: clusterv1.MachineHealthCheckChecks{
				UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
					{
						Type:           corev1.NodeReady,
						Status:         corev1.ConditionUnknown,
						TimeoutSeconds: ptr.To(int32(5 * 60)),
					},
				},
				UnhealthyMachineConditions: []clusterv1.UnhealthyMachineCondition{
					{
						Type:           controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
						Status:         metav1.ConditionUnknown,
						TimeoutSeconds: ptr.To(int32(5 * 60)),
					},
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
	conditions.Set(cluster, metav1.Condition{Type: clusterv1.ClusterInfrastructureReadyCondition, Status: metav1.ConditionTrue})
	conditions.Set(cluster, metav1.Condition{Type: clusterv1.ClusterControlPlaneInitializedCondition, Status: metav1.ConditionTrue})

	// Ensure the control plane was initialized earlier to prevent it interfering with
	// NodeStartupTimeoutSeconds testing.
	conds := []metav1.Condition{}
	for _, condition := range cluster.GetConditions() {
		condition.LastTransitionTime = metav1.NewTime(condition.LastTransitionTime.Add(-1 * time.Hour))
		conds = append(conds, condition)
	}
	cluster.SetConditions(conds)

	mhcSelector := map[string]string{"cluster": clusterName, "machine-group": "foo"}

	timeoutForMachineToHaveNode := 10 * time.Minute
	disabledTimeoutForMachineToHaveNode := time.Duration(0)
	timeoutForUnhealthyNodeConditions := int32(5 * 60)
	timeoutForUnhealthyMachineConditions := int32(5 * 60)

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
			Checks: clusterv1.MachineHealthCheckChecks{
				UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
					{
						Type:           corev1.NodeReady,
						Status:         corev1.ConditionUnknown,
						TimeoutSeconds: ptr.To(timeoutForUnhealthyNodeConditions),
					},
					{
						Type:           corev1.NodeReady,
						Status:         corev1.ConditionFalse,
						TimeoutSeconds: ptr.To(timeoutForUnhealthyNodeConditions),
					},
				},
				UnhealthyMachineConditions: []clusterv1.UnhealthyMachineCondition{
					{
						Type:           controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
						Status:         metav1.ConditionUnknown,
						TimeoutSeconds: ptr.To(timeoutForUnhealthyMachineConditions),
					},
					{
						Type:           controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
						Status:         metav1.ConditionFalse,
						TimeoutSeconds: ptr.To(timeoutForUnhealthyMachineConditions),
					},
				},
			},
		},
	}

	testMachine := newTestMachine("machine1", namespace, clusterName, "node1", mhcSelector)
	testMachineWithInfraReady := testMachine.DeepCopy()
	testMachineWithInfraReady.CreationTimestamp = metav1.NewTime(time.Now().Add(-100 * time.Second))
	conditions.Set(testMachineWithInfraReady, metav1.Condition{Type: clusterv1.MachineInfrastructureReadyCondition, Status: metav1.ConditionTrue, LastTransitionTime: metav1.NewTime(testMachineWithInfraReady.CreationTimestamp.Add(50 * time.Second))})

	nodeNotYetStartedTargetAndInfraReady := healthCheckTarget{
		Cluster: cluster,
		MHC:     testMHC,
		Machine: testMachineWithInfraReady,
		Node:    nil,
	}

	// Targets for when the node has not yet been seen by the Machine controller
	testMachineCreated1200s := testMachine.DeepCopy()
	nowMinus1200s := metav1.NewTime(time.Now().Add(-1200 * time.Second))
	testMachineCreated1200s.CreationTimestamp = nowMinus1200s

	nodeNotYetStartedTarget1200s := healthCheckTarget{
		Cluster: cluster,
		MHC:     testMHC,
		Machine: testMachineCreated1200s,
		Node:    nil,
	}
	nodeNotYetStartedTarget1200sCondition := newFailedHealthCheckV1Beta1Condition(clusterv1.NodeStartupTimeoutV1Beta1Reason, "Node failed to report startup in %s", timeoutForMachineToHaveNode)
	nodeNotYetStartedTarget1200sV1Beta2Condition := newFailedHealthCheckCondition(clusterv1.MachineHealthCheckNodeStartupTimeoutReason, "Health check failed: Node failed to report startup in %s", timeoutForMachineToHaveNode)

	testMachineCreated400s := testMachine.DeepCopy()
	nowMinus400s := metav1.NewTime(time.Now().Add(-400 * time.Second))
	testMachineCreated400s.CreationTimestamp = nowMinus400s

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
	nodeGoneAwayCondition := newFailedHealthCheckV1Beta1Condition(clusterv1.NodeNotFoundV1Beta1Reason, "")
	nodeGoneAwayV1Beta2Condition := newFailedHealthCheckCondition(clusterv1.MachineHealthCheckNodeDeletedReason, "Health check failed: Node %s has been deleted", testMachine.Status.NodeRef.Name)

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
	// using MHC without unhealthyNodeConditions
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
	nodeUnknown400Condition := newFailedHealthCheckV1Beta1Condition(clusterv1.UnhealthyNodeConditionV1Beta1Reason, "Condition Ready on node is reporting status Unknown for more than %s", (time.Duration(timeoutForUnhealthyNodeConditions) * time.Second).String())
	nodeUnknown400V1Beta2Condition := newFailedHealthCheckCondition(clusterv1.MachineHealthCheckUnhealthyNodeReason, "Health check failed: Condition Ready on Node is reporting status Unknown for more than %s", (time.Duration(timeoutForUnhealthyMachineConditions) * time.Second).String())

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

	// Machine unhealthy for shorter than timeout
	testMachineUnhealthy200 := newTestUnhealthyMachine("machine1", namespace, clusterName, "node1", mhcSelector, controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, metav1.ConditionFalse, 200*time.Second)
	machineUnhealthy200 := healthCheckTarget{
		Cluster:     cluster,
		MHC:         testMHC,
		Machine:     testMachineUnhealthy200,
		Node:        testNodeHealthy,
		nodeMissing: false,
	}

	// Machine unhealthy for longer than timeout
	testMachineUnhealthy400 := newTestUnhealthyMachine("machine1", namespace, clusterName, "node1", mhcSelector, controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, metav1.ConditionFalse, 400*time.Second)
	machineUnhealthy400 := healthCheckTarget{
		Cluster:     cluster,
		MHC:         testMHC,
		Machine:     testMachineUnhealthy400,
		Node:        testNodeHealthy,
		nodeMissing: false,
	}
	machineUnhealthy400Condition := newFailedHealthCheckV1Beta1Condition(
		clusterv1.UnhealthyMachineConditionV1Beta1Reason,
		"Condition EtcdPodHealthy on machine is reporting status False for more than %s",
		(time.Duration(timeoutForUnhealthyMachineConditions) * time.Second).String(),
	)
	machineUnhealthy400V1Beta2Condition := newFailedHealthCheckCondition(
		clusterv1.MachineHealthCheckUnhealthyMachineReason,
		"Health check failed: Condition EtcdPodHealthy on Machine is reporting status False for more than %s",
		(time.Duration(timeoutForUnhealthyMachineConditions) * time.Second).String(),
	)

	// Target for when the machine has the remediate machine annotation
	const annotationRemediationMsg = "Marked for remediation via remediate-machine annotation"
	const annotationRemediationV1Beta2Msg = "Health check failed: marked for remediation via cluster.x-k8s.io/remediate-machine annotation"
	testMachineAnnotationRemediation := testMachine.DeepCopy()
	testMachineAnnotationRemediation.Annotations = map[string]string{clusterv1.RemediateMachineAnnotation: ""}
	machineAnnotationRemediation := healthCheckTarget{
		Cluster: cluster,
		MHC:     testMHC,
		Machine: testMachineAnnotationRemediation,
		Node:    nil,
	}
	machineAnnotationRemediationCondition := newFailedHealthCheckV1Beta1Condition(clusterv1.HasRemediateMachineAnnotationV1Beta1Reason, annotationRemediationMsg)
	machineAnnotationRemediationV1Beta2Condition := newFailedHealthCheckCondition(clusterv1.MachineHealthCheckHasRemediateAnnotationReason, annotationRemediationV1Beta2Msg)

	testCases := []struct {
		desc                                     string
		targets                                  []healthCheckTarget
		timeoutForMachineToHaveNode              *time.Duration
		expectedHealthy                          []healthCheckTarget
		expectedNeedsRemediation                 []healthCheckTarget
		expectedNeedsRemediationCondition        []clusterv1.Condition
		expectedNeedsRemediationV1Beta2Condition []metav1.Condition
		expectedNextCheckTimes                   []time.Duration
	}{
		{
			desc:                     "when the node has not yet started for shorter than the timeout",
			targets:                  []healthCheckTarget{nodeNotYetStartedTarget400s},
			expectedHealthy:          []healthCheckTarget{},
			expectedNeedsRemediation: []healthCheckTarget{},
			expectedNextCheckTimes:   []time.Duration{timeoutForMachineToHaveNode - 400*time.Second},
		},
		{
			desc:                     "when the node has not yet started for shorter than the timeout, and infra is ready",
			targets:                  []healthCheckTarget{nodeNotYetStartedTargetAndInfraReady},
			expectedHealthy:          []healthCheckTarget{},
			expectedNeedsRemediation: []healthCheckTarget{},
			expectedNextCheckTimes:   []time.Duration{timeoutForMachineToHaveNode - 50*time.Second},
		},
		{
			desc:                                     "when the node has not yet started for longer than the timeout",
			targets:                                  []healthCheckTarget{nodeNotYetStartedTarget1200s},
			expectedHealthy:                          []healthCheckTarget{},
			expectedNeedsRemediation:                 []healthCheckTarget{nodeNotYetStartedTarget1200s},
			expectedNeedsRemediationCondition:        []clusterv1.Condition{nodeNotYetStartedTarget1200sCondition},
			expectedNeedsRemediationV1Beta2Condition: []metav1.Condition{nodeNotYetStartedTarget1200sV1Beta2Condition},
			expectedNextCheckTimes:                   []time.Duration{},
		},
		{
			desc:                                     "when the node has gone away",
			targets:                                  []healthCheckTarget{nodeGoneAway},
			expectedHealthy:                          []healthCheckTarget{},
			expectedNeedsRemediation:                 []healthCheckTarget{nodeGoneAway},
			expectedNeedsRemediationCondition:        []clusterv1.Condition{nodeGoneAwayCondition},
			expectedNeedsRemediationV1Beta2Condition: []metav1.Condition{nodeGoneAwayV1Beta2Condition},
			expectedNextCheckTimes:                   []time.Duration{},
		},
		{
			desc:                     "when the node has been in an unknown state for shorter than the timeout",
			targets:                  []healthCheckTarget{nodeUnknown200},
			expectedHealthy:          []healthCheckTarget{},
			expectedNeedsRemediation: []healthCheckTarget{},
			expectedNextCheckTimes:   []time.Duration{100 * time.Second},
		},
		{
			desc:                                     "when the node has been in an unknown state for longer than the timeout",
			targets:                                  []healthCheckTarget{nodeUnknown400},
			expectedHealthy:                          []healthCheckTarget{},
			expectedNeedsRemediation:                 []healthCheckTarget{nodeUnknown400},
			expectedNeedsRemediationCondition:        []clusterv1.Condition{nodeUnknown400Condition},
			expectedNeedsRemediationV1Beta2Condition: []metav1.Condition{nodeUnknown400V1Beta2Condition},
			expectedNextCheckTimes:                   []time.Duration{},
		},
		{
			desc:                     "when the machine condition has been unhealthy for shorter than the timeout",
			targets:                  []healthCheckTarget{machineUnhealthy200},
			expectedHealthy:          []healthCheckTarget{},
			expectedNeedsRemediation: []healthCheckTarget{},
			expectedNextCheckTimes:   []time.Duration{100 * time.Second}, // 300s timeout - 200s elapsed = 100s remaining
		},
		{
			desc:                                     "when the machine condition has been unhealthy for longer than the timeout",
			targets:                                  []healthCheckTarget{machineUnhealthy400},
			expectedHealthy:                          []healthCheckTarget{},
			expectedNeedsRemediation:                 []healthCheckTarget{machineUnhealthy400},
			expectedNeedsRemediationCondition:        []clusterv1.Condition{machineUnhealthy400Condition},
			expectedNeedsRemediationV1Beta2Condition: []metav1.Condition{machineUnhealthy400V1Beta2Condition},
			expectedNextCheckTimes:                   []time.Duration{},
		},
		{
			desc:                     "when the node is healthy",
			targets:                  []healthCheckTarget{nodeHealthy},
			expectedHealthy:          []healthCheckTarget{nodeHealthy},
			expectedNeedsRemediation: []healthCheckTarget{},
			expectedNextCheckTimes:   []time.Duration{},
		},
		{
			desc:                                     "with a mix of healthy and unhealthy nodes",
			targets:                                  []healthCheckTarget{nodeUnknown100, nodeUnknown200, nodeUnknown400, nodeHealthy},
			expectedHealthy:                          []healthCheckTarget{nodeHealthy},
			expectedNeedsRemediation:                 []healthCheckTarget{nodeUnknown400},
			expectedNeedsRemediationCondition:        []clusterv1.Condition{nodeUnknown400Condition},
			expectedNeedsRemediationV1Beta2Condition: []metav1.Condition{nodeUnknown400V1Beta2Condition},
			expectedNextCheckTimes:                   []time.Duration{200 * time.Second, 100 * time.Second},
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
			desc:                                     "when the machine is manually marked for remediation",
			targets:                                  []healthCheckTarget{machineAnnotationRemediation},
			expectedHealthy:                          []healthCheckTarget{},
			expectedNeedsRemediation:                 []healthCheckTarget{machineAnnotationRemediation},
			expectedNeedsRemediationCondition:        []clusterv1.Condition{machineAnnotationRemediationCondition},
			expectedNeedsRemediationV1Beta2Condition: []metav1.Condition{machineAnnotationRemediationV1Beta2Condition},
			expectedNextCheckTimes:                   []time.Duration{},
		},
		{
			desc:                                     "health check with empty unhealthy conditions and missing node",
			targets:                                  []healthCheckTarget{nodeGoneAwayEmptyConditions},
			expectedHealthy:                          []healthCheckTarget{},
			expectedNeedsRemediation:                 []healthCheckTarget{nodeGoneAwayEmptyConditions},
			expectedNeedsRemediationCondition:        []clusterv1.Condition{nodeGoneAwayCondition},
			expectedNeedsRemediationV1Beta2Condition: []metav1.Condition{nodeGoneAwayV1Beta2Condition},
			expectedNextCheckTimes:                   []time.Duration{},
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

			removeLastTransitionTimesV1Beta2 := func(in []metav1.Condition) []metav1.Condition {
				out := []metav1.Condition{}
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
				actualConditions := unhealthy[i].Machine.GetV1Beta1Conditions()
				conditionsMatcher := WithTransform(removeLastTransitionTimes, ContainElements(expectedMachineCondition))
				gs.Expect(actualConditions).To(conditionsMatcher)
			}
			for i, expectedMachineCondition := range tc.expectedNeedsRemediationV1Beta2Condition {
				actualConditions := unhealthy[i].Machine.GetConditions()
				conditionsMatcher := WithTransform(removeLastTransitionTimesV1Beta2, ContainElements(expectedMachineCondition))
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
			Initialization: clusterv1.MachineInitializationStatus{
				InfrastructureProvisioned:  ptr.To(true),
				BootstrapDataSecretCreated: ptr.To(true),
			},
			Phase: string(clusterv1.MachinePhaseRunning),
			NodeRef: clusterv1.MachineNodeReference{
				Name: nodeName,
			},
		},
	}
}

func newTestNode(name string) *corev1.Node {
	return &corev1.Node{
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

func newTestUnhealthyMachine(name, namespace, clusterName, nodeName string, labels map[string]string, condition string, status metav1.ConditionStatus, unhealthyDuration time.Duration) *clusterv1.Machine {
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
			Conditions: []metav1.Condition{
				{
					Type:               condition,
					Status:             status,
					LastTransitionTime: metav1.NewTime(time.Now().Add(-unhealthyDuration)),
				},
			},
			Initialization: clusterv1.MachineInitializationStatus{
				InfrastructureProvisioned:  ptr.To(true),
				BootstrapDataSecretCreated: ptr.To(true),
			},
			Phase: string(clusterv1.MachinePhaseRunning),
			NodeRef: clusterv1.MachineNodeReference{
				Name: nodeName,
			},
		},
	}
}

func newFailedHealthCheckV1Beta1Condition(reason string, messageFormat string, messageArgs ...interface{}) clusterv1.Condition {
	return *v1beta1conditions.FalseCondition(clusterv1.MachineHealthCheckSucceededV1Beta1Condition, reason, clusterv1.ConditionSeverityWarning, messageFormat, messageArgs...)
}

func newFailedHealthCheckCondition(reason string, messageFormat string, messageArgs ...interface{}) metav1.Condition {
	return metav1.Condition{
		Type:    clusterv1.MachineHealthCheckSucceededCondition,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: fmt.Sprintf(messageFormat, messageArgs...),
	}
}
