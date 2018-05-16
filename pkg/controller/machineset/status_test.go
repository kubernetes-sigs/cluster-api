/*
Copyright 2018 The Kubernetes Authors.

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

package machineset

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/fake"
)

func TestMachineSetController_calculateStatus(t *testing.T) {
	readyNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "rNode"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				corev1.NodeCondition{
					Type:               corev1.NodeReady,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: metav1.Time{Time: time.Now()},
				},
			},
		},
	}
	notReadyNode := readyNode.DeepCopy()
	notReadyNode.Name = "nrNode"
	notReadyNode.Status.Conditions[0].Status = corev1.ConditionFalse
	unknownStatusNode := readyNode.DeepCopy()
	unknownStatusNode.Name = "usNode"
	unknownStatusNode.Status.Conditions[0].Status = corev1.ConditionUnknown
	noConditionNode := readyNode.DeepCopy()
	noConditionNode.Name = "ncNode"
	noConditionNode.Status.Conditions = []corev1.NodeCondition{}
	availableNode := readyNode.DeepCopy()
	availableNode.Name = "aNode"
	availableNode.Status.Conditions[0].LastTransitionTime.Time = time.Now().Add(time.Duration(-6) * time.Minute)

	tests := []struct {
		name                      string
		machines                  []*v1alpha1.Machine
		setMinReadySeconds        bool
		minReadySeconds           int32
		expectedReplicas          int32
		expectedLabeledReplicas   int32
		expectedReadyReplicas     int32
		expectedAvailableReplicas int32
	}{
		{
			name: "scenario 1: empty machinset.",
		},
		{
			name: "scenario 2: 1 replica, 1 labeled machine",
			machines: []*v1alpha1.Machine{
				machineFromMachineSet(createMachineSet(1, "foo", "bar1", "acme"), "bar1"),
			},
			expectedReplicas:        1,
			expectedLabeledReplicas: 1,
		},
		{
			name: "scenario 3: 1 replica, 0 labeled machine",
			machines: []*v1alpha1.Machine{
				setDifferentLabels(machineFromMachineSet(createMachineSet(1, "foo", "bar1", "acme"), "bar1")),
			},
			expectedReplicas: 1,
		},
		{
			name: "scenario 4: 1 replica, 1 ready machine",
			machines: []*v1alpha1.Machine{
				setNode(machineFromMachineSet(createMachineSet(1, "foo", "bar1", "acme"), "bar1"), readyNode),
			},
			expectedReplicas:        1,
			expectedLabeledReplicas: 1,
			expectedReadyReplicas:   1,
		},
		{
			name: "scenario 5: 1 replica, 0 ready machine, not ready node",
			machines: []*v1alpha1.Machine{
				setNode(machineFromMachineSet(createMachineSet(1, "foo", "bar1", "acme"), "bar1"), notReadyNode),
			},
			expectedReplicas:        1,
			expectedLabeledReplicas: 1,
		},
		{
			name: "scenario 6: 1 replica, 0 ready machine, unknown node",
			machines: []*v1alpha1.Machine{
				setNode(machineFromMachineSet(createMachineSet(1, "foo", "bar1", "acme"), "bar1"), unknownStatusNode),
			},
			expectedReplicas:        1,
			expectedLabeledReplicas: 1,
		},
		{
			name: "scenario 7: 1 replica, 0 ready machine, missing condition node",
			machines: []*v1alpha1.Machine{
				setNode(machineFromMachineSet(createMachineSet(1, "foo", "bar1", "acme"), "bar1"), noConditionNode),
			},
			expectedReplicas:        1,
			expectedLabeledReplicas: 1,
		},
		{
			name: "scenario 8: 1 replica, 1 available machine, minReadySeconds = 0",
			machines: []*v1alpha1.Machine{
				setNode(machineFromMachineSet(createMachineSet(1, "foo", "bar1", "acme"), "bar1"), readyNode),
			},
			setMinReadySeconds:        true,
			minReadySeconds:           0,
			expectedReplicas:          1,
			expectedLabeledReplicas:   1,
			expectedReadyReplicas:     1,
			expectedAvailableReplicas: 1,
		},
		{
			name: "scenario 9: 1 replica, 1 available machine, 360s elapsed, need 300s",
			machines: []*v1alpha1.Machine{
				setNode(machineFromMachineSet(createMachineSet(1, "foo", "bar1", "acme"), "bar1"), availableNode),
			},
			setMinReadySeconds:        true,
			minReadySeconds:           300,
			expectedReplicas:          1,
			expectedLabeledReplicas:   1,
			expectedReadyReplicas:     1,
			expectedAvailableReplicas: 1,
		},
		{
			name: "scenario 10: 4 replicas, 3 labeled, 2 ready, 1 available machine",
			machines: []*v1alpha1.Machine{
				setDifferentLabels(setNode(machineFromMachineSet(createMachineSet(1, "foo", "bar1", "acme"), "bar1"), noConditionNode)),
				setNode(machineFromMachineSet(createMachineSet(1, "foo", "bar1", "acme"), "bar1"), notReadyNode),
				setNode(machineFromMachineSet(createMachineSet(1, "foo", "bar1", "acme"), "bar1"), readyNode),
				setNode(machineFromMachineSet(createMachineSet(1, "foo", "bar1", "acme"), "bar1"), availableNode),
			},
			setMinReadySeconds:        true,
			minReadySeconds:           300,
			expectedReplicas:          4,
			expectedLabeledReplicas:   3,
			expectedReadyReplicas:     2,
			expectedAvailableReplicas: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rObjects := []runtime.Object{readyNode, notReadyNode, unknownStatusNode, noConditionNode, availableNode}
			k8sClient := kubefake.NewSimpleClientset(rObjects...)

			c := &MachineSetControllerImpl{}
			c.kubernetesClient = k8sClient

			ms := createMachineSet(len(test.machines), "foo", "bar1", "acme")
			if test.setMinReadySeconds {
				ms.Spec.MinReadySeconds = test.minReadySeconds
			}

			status := c.calculateStatus(ms, test.machines)

			if status.Replicas != test.expectedReplicas {
				t.Errorf("got %v replicas, expected %v replicas", status.Replicas, test.expectedReplicas)
			}

			if status.FullyLabeledReplicas != test.expectedLabeledReplicas {
				t.Errorf("got %v fully labeled replicas, expected %v fully labeled replicas", status.FullyLabeledReplicas, test.expectedLabeledReplicas)
			}

			if status.ReadyReplicas != test.expectedReadyReplicas {
				t.Errorf("got %v ready replicas, expected %v ready replicas", status.ReadyReplicas, test.expectedReadyReplicas)
			}

			if status.AvailableReplicas != test.expectedAvailableReplicas {
				t.Errorf("got %v available replicas, expected %v available replicas", status.AvailableReplicas, test.expectedAvailableReplicas)
			}

		})
	}
}

func setNode(machine *v1alpha1.Machine, node *corev1.Node) *v1alpha1.Machine {
	machine.Status.NodeRef = getNodeRef(node)
	return machine
}

func getNodeRef(node *corev1.Node) *corev1.ObjectReference {
	return &corev1.ObjectReference{
		Kind: "Node",
		Name: node.ObjectMeta.Name,
	}
}

func TestMachineSetController_updateMachineSetStatus(t *testing.T) {
	tests := []struct {
		name            string
		attemptOutcomes []bool
		newStatus       v1alpha1.MachineSetStatus
		expectErr       bool
	}{
		{
			name:      "no change in status, noop",
			newStatus: getMachineSetStatus(1, 1, 1, 1),
		},
		{
			name:            "machine set status update success first try",
			attemptOutcomes: []bool{true},
			newStatus:       getMachineSetStatus(2, 1, 1, 1),
		},
		{
			name:            "machine set status update fail first try, success second try",
			attemptOutcomes: []bool{false, true},
			newStatus:       getMachineSetStatus(1, 2, 1, 1),
		},
		{
			name:            "machine set status update fail second try",
			attemptOutcomes: []bool{false, false},
			newStatus:       getMachineSetStatus(1, 1, 2, 1),
			expectErr:       true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ms := createMachineSet(1, "foo", "bar1", "acme")
			ms.Status = getMachineSetStatus(1, 1, 1, 1)

			rObjects := []runtime.Object{ms}
			fakeClient := fake.NewSimpleClientset(rObjects...)

			numCalls := 0
			fakeClient.PrependReactor("update", "machinesets", func(action core.Action) (bool, runtime.Object, error) {
				success := test.attemptOutcomes[numCalls]
				numCalls++
				if !success {
					return true, nil, fmt.Errorf("error")
				}
				newMS := ms.DeepCopy()
				newMS.Status = test.newStatus
				return true, newMS, nil
			})

			updatedMS, err := updateMachineSetStatus(fakeClient.ClusterV1alpha1().MachineSets(ms.Namespace), ms, test.newStatus)

			if numCalls != len(test.attemptOutcomes) {
				t.Fatalf("got %v update calls, expected %v update calls", numCalls, len(test.attemptOutcomes))
			}

			if (err != nil) != test.expectErr {
				t.Fatalf("got %v err, expected %v err, %v", err != nil, test.expectErr, err)
			}
			if !test.expectErr && len(test.attemptOutcomes) > 0 {
				if !reflect.DeepEqual(updatedMS.Status, test.newStatus) {
					t.Fatalf("got %v status, expected %v status", updatedMS.Status, test.newStatus)
				}
			}
		})
	}
}

func getMachineSetStatus(replicas, labeled, ready, available int32) v1alpha1.MachineSetStatus {
	return v1alpha1.MachineSetStatus{
		Replicas:             replicas,
		FullyLabeledReplicas: labeled,
		ReadyReplicas:        ready,
		AvailableReplicas:    available,
	}
}
