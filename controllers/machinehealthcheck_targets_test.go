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

package controllers

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	gtypes "github.com/onsi/gomega/types"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestGetTargetsFromMHC(t *testing.T) {
	namespace := "test-mhc"
	clusterName := "test-cluster"
	mhcSelector := map[string]string{"cluster": clusterName, "machine-group": "foo"}

	// Create a namespace and cluster for the tests
	testNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "mhc-test"}}
	testCluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: clusterName}}

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
			ClusterName: testCluster.ObjectMeta.Name,
			UnhealthyConditions: []clusterv1.UnhealthyCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionUnknown,
					Timeout: metav1.Duration{Duration: 5 * time.Minute},
				},
			},
		},
	}

	baseObjects := []runtime.Object{testNS, testCluster, testMHC}

	// Initialise some test machines and nodes for use in the test cases

	testNode1 := newTestNode("node1")
	testMachine1 := newTestMachine("machine1", namespace, clusterName, testNode1.Name, mhcSelector)
	testNode2 := newTestNode("node2")
	testMachine2 := newTestMachine("machine2", namespace, clusterName, testNode2.Name, map[string]string{"cluster": clusterName})
	testNode3 := newTestNode("node3")
	testMachine3 := newTestMachine("machine3", namespace, clusterName, testNode3.Name, mhcSelector)
	testNode4 := newTestNode("node4")
	testMachine4 := newTestMachine("machine4", namespace, clusterName, testNode4.Name, mhcSelector)

	testCases := []struct {
		desc            string
		toCreate        []runtime.Object
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
					Node:        &corev1.Node{},
					nodeMissing: true,
				},
			},
		},
		{
			desc:     "when a machine's labels do not match",
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
				{
					Machine: testMachine4,
					MHC:     testMHC,
					Node:    testNode4,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			gs := NewGomegaWithT(t)

			gs.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
			k8sClient := fake.NewFakeClientWithScheme(scheme.Scheme, tc.toCreate...)

			// Create a test reconciler
			reconciler := &MachineHealthCheckReconciler{
				Client: k8sClient,
				Log:    log.Log,
				scheme: scheme.Scheme,
			}

			targets, err := reconciler.getTargetsFromMHC(k8sClient, testCluster, testMHC)
			gs.Expect(err).ToNot(HaveOccurred())

			gs.Expect(targets).To(ConsistOf(tc.expectedTargets))
		})
	}
}

func TestHealthCheckTargets(t *testing.T) {
	namespace := "test-mhc"
	clusterName := "test-cluster"
	mhcSelector := map[string]string{"cluster": clusterName, "machine-group": "foo"}

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
					Timeout: metav1.Duration{Duration: 5 * time.Minute},
				},
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionFalse,
					Timeout: metav1.Duration{Duration: 5 * time.Minute},
				},
			},
		},
	}

	testMachine := newTestMachine("machine1", namespace, clusterName, "node1", mhcSelector)

	// Target for when the node has not yet been seen by the Machine controller
	testMachineLastUpdated400s := testMachine.DeepCopy()
	nowMinus400s := metav1.NewTime(time.Now().Add(-400 * time.Second))
	testMachineLastUpdated400s.Status.LastUpdated = &nowMinus400s

	nodeNotYetStartedTarget := healthCheckTarget{
		MHC:     testMHC,
		Machine: testMachineLastUpdated400s,
		Node:    nil,
	}

	// Target for when the Node has been seen, but has now gone
	nodeGoneAway := healthCheckTarget{
		MHC:         testMHC,
		Machine:     testMachine,
		Node:        &corev1.Node{},
		nodeMissing: true,
	}

	// Target for when the node has been in an unknown state for shorter than the timeout
	testNodeUnknown200 := newTestUnhealthyNode("node1", corev1.NodeReady, corev1.ConditionUnknown, 200*time.Second)
	nodeUnknown200 := healthCheckTarget{
		MHC:         testMHC,
		Machine:     testMachine,
		Node:        testNodeUnknown200,
		nodeMissing: false,
	}

	// Second Target for when the node has been in an unknown state for shorter than the timeout
	testNodeUnknown100 := newTestUnhealthyNode("node1", corev1.NodeReady, corev1.ConditionUnknown, 100*time.Second)
	nodeUnknown100 := healthCheckTarget{
		MHC:         testMHC,
		Machine:     testMachine,
		Node:        testNodeUnknown100,
		nodeMissing: false,
	}

	// Target for when the node has been in an unknown state for longer than the timeout
	testNodeUnknown400 := newTestUnhealthyNode("node1", corev1.NodeReady, corev1.ConditionUnknown, 400*time.Second)
	nodeUnknown400 := healthCheckTarget{
		MHC:         testMHC,
		Machine:     testMachine,
		Node:        testNodeUnknown400,
		nodeMissing: false,
	}

	// Target for when a node is healthy
	testNodeHealthy := newTestNode("node1")
	testNodeHealthy.UID = "12345"
	nodeHealthy := healthCheckTarget{
		MHC:         testMHC,
		Machine:     testMachine,
		Node:        testNodeHealthy,
		nodeMissing: false,
	}

	testCases := []struct {
		desc                     string
		targets                  []healthCheckTarget
		expectedHealthy          int
		expectedNeedsRemediation []healthCheckTarget
		expectedNextCheckTimes   []time.Duration
	}{
		{
			desc:                     "when the node has not yet started",
			targets:                  []healthCheckTarget{nodeNotYetStartedTarget},
			expectedHealthy:          0,
			expectedNeedsRemediation: []healthCheckTarget{},
			expectedNextCheckTimes:   []time.Duration{200 * time.Second},
		},
		{
			desc:                     "when the node has gone away",
			targets:                  []healthCheckTarget{nodeGoneAway},
			expectedHealthy:          0,
			expectedNeedsRemediation: []healthCheckTarget{nodeGoneAway},
			expectedNextCheckTimes:   []time.Duration{},
		},
		{
			desc:                     "when the node has been in an unknown state for shorter than the timeout",
			targets:                  []healthCheckTarget{nodeUnknown200},
			expectedHealthy:          0,
			expectedNeedsRemediation: []healthCheckTarget{},
			expectedNextCheckTimes:   []time.Duration{100 * time.Second},
		},
		{
			desc:                     "when the node has been in an unknown state for longer than the timeout",
			targets:                  []healthCheckTarget{nodeUnknown400},
			expectedHealthy:          0,
			expectedNeedsRemediation: []healthCheckTarget{nodeUnknown400},
			expectedNextCheckTimes:   []time.Duration{},
		},
		{
			desc:                     "when the node is healthy",
			targets:                  []healthCheckTarget{nodeHealthy},
			expectedHealthy:          1,
			expectedNeedsRemediation: []healthCheckTarget{},
			expectedNextCheckTimes:   []time.Duration{},
		},
		{
			desc:                     "with a mix of healthy and unhealthy nodes",
			targets:                  []healthCheckTarget{nodeUnknown100, nodeUnknown200, nodeUnknown400, nodeHealthy},
			expectedHealthy:          1,
			expectedNeedsRemediation: []healthCheckTarget{nodeUnknown400},
			expectedNextCheckTimes:   []time.Duration{200 * time.Second, 100 * time.Second},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			gs := NewGomegaWithT(t)

			gs.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
			k8sClient := fake.NewFakeClientWithScheme(scheme.Scheme)

			// Create a test reconciler
			reconciler := &MachineHealthCheckReconciler{
				Client:   k8sClient,
				Log:      log.Log,
				scheme:   scheme.Scheme,
				recorder: record.NewFakeRecorder(5),
			}

			timeoutForMachineToHaveNode := 10 * time.Minute
			currentHealthy, needRemediationTargets, nextCheckTimes := reconciler.healthCheckTargets(tc.targets, reconciler.Log, timeoutForMachineToHaveNode)

			// Round durations down to nearest second account for minute differences
			// in timing when running tests
			roundDurations := func(in []time.Duration) []time.Duration {
				out := []time.Duration{}
				for _, d := range in {
					out = append(out, d.Truncate(time.Second))
				}
				return out
			}

			gs.Expect(currentHealthy).To(Equal(tc.expectedHealthy))
			gs.Expect(needRemediationTargets).To(ConsistOf(tc.expectedNeedsRemediation))
			gs.Expect(nextCheckTimes).To(WithTransform(roundDurations, ConsistOf(tc.expectedNextCheckTimes)))
		})
	}
}

func TestRemediate(t *testing.T) {
	namespace := "test-mhc"
	clusterName := "test-cluster"
	labels := map[string]string{"cluster": clusterName, "machine-group": "foo"}

	machineSetORs := []metav1.OwnerReference{{APIVersion: clusterv1.GroupVersion.String(), Kind: "MachineSet", Controller: pointer.BoolPtr(true)}}
	kubeadmControlPlaneORs := []metav1.OwnerReference{{APIVersion: controlplanev1.GroupVersion.String(), Kind: "KubeadmControlPlane", Controller: pointer.BoolPtr(true)}}

	workerNode := newTestNode("worker-node")
	workerMachine := newTestMachine("worker-machine", namespace, clusterName, workerNode.Name, labels)
	workerMachine.SetOwnerReferences(machineSetORs)
	workerMachineUnowned := newTestMachine("worker-machine", namespace, clusterName, workerNode.Name, labels)

	controlPlaneNode := newTestNode("control-plane-node")
	if controlPlaneNode.Labels == nil {
		controlPlaneNode.Labels = make(map[string]string)
	}
	controlPlaneNode.Labels[nodeControlPlaneLabel] = ""

	controlPlaneMachine := newTestMachine("control-plane-machine", namespace, clusterName, controlPlaneNode.Name, labels)
	controlPlaneMachine.SetOwnerReferences(kubeadmControlPlaneORs)
	if controlPlaneMachine.Labels == nil {
		controlPlaneMachine.Labels = make(map[string]string)
	}
	controlPlaneMachine.Labels[clusterv1.MachineControlPlaneLabelName] = ""

	testCases := []struct {
		name          string
		node          *corev1.Node
		machine       *clusterv1.Machine
		expectErr     bool
		expectDeleted bool
		expectEvents  []string
	}{
		{
			name:          "when the machine is not owned by a machineset",
			node:          workerNode,
			machine:       workerMachineUnowned,
			expectErr:     false,
			expectDeleted: false,
			expectEvents:  []string{},
		},
		{
			name:          "when the node is a worker with a machine owned by a machineset",
			node:          workerNode,
			machine:       workerMachine,
			expectErr:     false,
			expectDeleted: true,
			expectEvents:  []string{EventMachineDeleted},
		},
		{
			name:          "when the node is a control plane node",
			node:          controlPlaneNode,
			machine:       controlPlaneMachine,
			expectErr:     false,
			expectDeleted: true,
			expectEvents:  []string{EventMachineDeleted},
		},
		{
			name:          "when the machine is a control plane machine",
			node:          nil,
			machine:       controlPlaneMachine,
			expectErr:     false,
			expectDeleted: true,
			expectEvents:  []string{EventMachineDeleted},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gs := NewGomegaWithT(t)

			target := &healthCheckTarget{
				Node:    tc.node,
				Machine: tc.machine,
				MHC:     newTestMachineHealthCheck("mhc", namespace, clusterName, labels),
			}

			fakeRecorder := record.NewFakeRecorder(2)
			gs.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
			k8sClient := fake.NewFakeClientWithScheme(scheme.Scheme, target.Machine)
			if target.Node != nil {
				gs.Expect(k8sClient.Create(context.Background(), target.Node)).To(Succeed())
			}

			// Run remediation
			err := target.remediate(context.Background(), log.Log, k8sClient, fakeRecorder)
			gs.Expect(err != nil).To(Equal(tc.expectErr))

			machine := &clusterv1.Machine{}
			key := types.NamespacedName{Namespace: tc.machine.Namespace, Name: tc.machine.Name}
			err = k8sClient.Get(context.Background(), key, machine)

			// Check if the machine was deleted or not
			if tc.expectDeleted {
				gs.Expect(errors.IsNotFound(err)).To(BeTrue())
			} else {
				gs.Expect(err).ToNot(HaveOccurred())
				gs.Expect(machine).To(Equal(target.Machine))
			}

			// Check which event types were sent
			gs.Expect(fakeRecorder.Events).To(HaveLen(len(tc.expectEvents)))
			receivedEvents := []string{}
			eventMatchers := []gtypes.GomegaMatcher{}
			for _, ev := range tc.expectEvents {
				receivedEvents = append(receivedEvents, <-fakeRecorder.Events)
				eventMatchers = append(eventMatchers, ContainSubstring(fmt.Sprintf(" %s ", ev)))
			}
			gs.Expect(receivedEvents).To(ConsistOf(eventMatchers))
		})
	}
}

func TestHasControllerOwner(t *testing.T) {
	machineSetOR := metav1.OwnerReference{
		Kind:       "MachineSet",
		APIVersion: clusterv1.GroupVersion.String(),
		Controller: pointer.BoolPtr(true),
	}

	machineDeploymentOR := metav1.OwnerReference{
		Kind:       "MachineDeployment",
		APIVersion: clusterv1.GroupVersion.String(),
		Controller: pointer.BoolPtr(true),
	}

	controlPlaneOR := metav1.OwnerReference{
		Kind:       "KubeadmControlPlane",
		APIVersion: controlplanev1.GroupVersion.String(),
		Controller: pointer.BoolPtr(true),
	}

	differentGroupOR := metav1.OwnerReference{
		Kind:       "MachineSet",
		APIVersion: "different-group/v1",
		Controller: pointer.BoolPtr(true),
	}

	invalidGroupOR := metav1.OwnerReference{
		Kind:       "MachineSet",
		APIVersion: "invalid/group/",
		Controller: pointer.BoolPtr(true),
	}

	noControllerOwner := metav1.OwnerReference{
		Kind:       "noControllerOwner",
		APIVersion: clusterv1.GroupVersion.String(),
		Controller: pointer.BoolPtr(false),
	}

	testCases := []struct {
		name            string
		ownerReferences []metav1.OwnerReference
		hasOwner        bool
		expectErr       bool
	}{
		{
			name:            "with no owner references",
			ownerReferences: []metav1.OwnerReference{},
			hasOwner:        false,
			expectErr:       false,
		},
		{
			name:            "with a MachineDeployment owner reference",
			ownerReferences: []metav1.OwnerReference{machineDeploymentOR},
			hasOwner:        true,
			expectErr:       false,
		},
		{
			name:            "with a MachineSet owner reference",
			ownerReferences: []metav1.OwnerReference{machineSetOR},
			hasOwner:        true,
			expectErr:       false,
		},
		{
			name:            "with a MachineSet and MachineDeployment owner reference",
			ownerReferences: []metav1.OwnerReference{machineSetOR, machineDeploymentOR},
			hasOwner:        true,
			expectErr:       false,
		},
		{
			name:            "with a MachineSet owner reference from a different API Group",
			ownerReferences: []metav1.OwnerReference{differentGroupOR},
			hasOwner:        false,
			expectErr:       false,
		},
		{
			name:            "with a MachineSet owner reference from an invalid API Group",
			ownerReferences: []metav1.OwnerReference{invalidGroupOR},
			hasOwner:        false,
			expectErr:       true,
		},
		{
			name:            "with MachineSet owner references from two API Groups",
			ownerReferences: []metav1.OwnerReference{differentGroupOR, machineSetOR},
			hasOwner:        true,
			expectErr:       false,
		},
		{
			name:            "with a control plane controller",
			ownerReferences: []metav1.OwnerReference{controlPlaneOR},
			hasOwner:        true,
			expectErr:       false,
		},
		{
			name:            "with an owner which is not a controller",
			ownerReferences: []metav1.OwnerReference{noControllerOwner},
			hasOwner:        false,
			expectErr:       false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gs := NewGomegaWithT(t)

			machine := newTestMachine("machine", "test-mhc", "test-cluster", "node", map[string]string{})
			machine.SetOwnerReferences(tc.ownerReferences)

			target := &healthCheckTarget{
				Machine: machine,
			}

			hasControllerOwner, err := target.hasControllerOwner()
			gs.Expect(err != nil).To(Equal(tc.expectErr))
			gs.Expect(hasControllerOwner).To(Equal(tc.hasOwner))
		})
	}
}

func newTestMachine(name, namespace, clusterName, nodeName string, labels map[string]string) *clusterv1.Machine {
	// Copy the labels so that the map is unique to each test Machine
	l := make(map[string]string)
	for k, v := range labels {
		l[k] = v
	}

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
				Data: &bootstrap,
			},
		},
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Name:      nodeName,
				Namespace: namespace,
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
