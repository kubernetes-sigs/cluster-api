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

package machine

import (
	"testing"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	corefake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1/testutil"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/fake"
	"sigs.k8s.io/cluster-api/pkg/util"
)

func TestMachineControllerReconcileHandler(t *testing.T) {
	tests := []struct {
		name                   string
		objExists              bool
		instanceExists         bool
		isDeleting             bool
		withFinalizer          bool
		ignoreDeleteCallCount  bool
		expectFinalizerRemoved bool
		numExpectedCreateCalls int64
		numExpectedDeleteCalls int64
		numExpectedUpdateCalls int64
		numExpectedExistsCalls int64
		nodeRef                *v1.ObjectReference
	}{
		{
			name:                   "Create machine",
			objExists:              false,
			instanceExists:         false,
			numExpectedCreateCalls: 1,
			numExpectedExistsCalls: 1,
		},
		{
			name:                   "Update machine",
			objExists:              true,
			instanceExists:         true,
			numExpectedUpdateCalls: 1,
			numExpectedExistsCalls: 1,
		},
		{
			name:                   "Delete machine, instance exists, with finalizer",
			objExists:              true,
			instanceExists:         true,
			isDeleting:             true,
			withFinalizer:          true,
			expectFinalizerRemoved: true,
			numExpectedDeleteCalls: 1,
		},
		{
			// This should not be possible. Here for completeness.
			name:           "Delete machine, instance exists without finalizer",
			objExists:      true,
			instanceExists: true,
			isDeleting:     true,
			withFinalizer:  false,
		},
		{
			name:                   "Delete machine, instance does not exist, with finalizer",
			objExists:              true,
			instanceExists:         false,
			isDeleting:             true,
			withFinalizer:          true,
			ignoreDeleteCallCount:  true,
			expectFinalizerRemoved: true,
		},
		{
			name:           "Delete machine, instance does not exist, without finalizer",
			objExists:      true,
			instanceExists: false,
			isDeleting:     true,
			withFinalizer:  false,
		},
		{
			name:                   "Delete machine, controller on different node with same name, but different UID",
			objExists:              true,
			instanceExists:         true,
			isDeleting:             true,
			withFinalizer:          true,
			numExpectedDeleteCalls: 1,
			expectFinalizerRemoved: true,
			nodeRef:                &v1.ObjectReference{Name: "controller-node-name", UID: "another-uid"},
		},
		{
			name:                   "Delete machine, controller on the same node",
			objExists:              true,
			instanceExists:         true,
			isDeleting:             true,
			withFinalizer:          true,
			expectFinalizerRemoved: false,
			nodeRef:                &v1.ObjectReference{Name: "controller-node-name", UID: "controller-node-uid"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			machineToTest := getMachine("bar", test.nodeRef, test.isDeleting, test.withFinalizer)
			knownObjects := []runtime.Object{}
			if test.objExists {
				knownObjects = append(knownObjects, machineToTest)
			}

			machineUpdated := false
			fakeClient := fake.NewSimpleClientset(knownObjects...)
			fakeClient.PrependReactor("update", "machines", func(action core.Action) (bool, runtime.Object, error) {
				machineUpdated = true
				return false, nil, nil
			})
			fakeKubernetesClient := corefake.NewSimpleClientset()
			node := v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "controller-node-name",
					UID:  "controller-node-uid",
				},
			}
			fakeKubernetesClient.CoreV1().Nodes().Create(&node)

			// When creating a new object, it should invoke the reconcile method.
			cluster := testutil.GetVanillaCluster()
			cluster.Name = "cluster-1"
			if _, err := fakeClient.ClusterV1alpha1().Clusters(metav1.NamespaceDefault).Create(&cluster); err != nil {
				t.Fatal(err)
			}

			actuator := NewTestActuator()
			actuator.ExistsValue = test.instanceExists

			target := &MachineControllerImpl{}
			target.actuator = actuator
			target.clientSet = fakeClient
			target.kubernetesClientSet = fakeKubernetesClient
			target.nodeName = "controller-node-name"

			var err error
			err = target.Reconcile(machineToTest)
			if err != nil {
				t.Fatal(err)
			}

			finalizerRemoved := false
			if machineUpdated {
				updatedMachine, err := fakeClient.ClusterV1alpha1().Machines(machineToTest.Namespace).Get(machineToTest.Name, metav1.GetOptions{})
				if err != nil {
					t.Fatalf("failed to get updated machine.")
				}
				finalizerRemoved = !util.Contains(updatedMachine.ObjectMeta.Finalizers, v1alpha1.MachineFinalizer)
			}

			if finalizerRemoved != test.expectFinalizerRemoved {
				t.Errorf("Got finalizer removed %v, expected finalizer removed %v", finalizerRemoved, test.expectFinalizerRemoved)
			}
			if actuator.CreateCallCount != test.numExpectedCreateCalls {
				t.Errorf("Got %v create calls, expected %v", actuator.CreateCallCount, test.numExpectedCreateCalls)
			}
			if actuator.DeleteCallCount != test.numExpectedDeleteCalls && !test.ignoreDeleteCallCount {
				t.Errorf("Got %v delete calls, expected %v", actuator.DeleteCallCount, test.numExpectedDeleteCalls)
			}
			if actuator.UpdateCallCount != test.numExpectedUpdateCalls {
				t.Errorf("Got %v update calls, expected %v", actuator.UpdateCallCount, test.numExpectedUpdateCalls)
			}
			if actuator.ExistsCallCount != test.numExpectedExistsCalls {
				t.Errorf("Got %v exists calls, expected %v", actuator.ExistsCallCount, test.numExpectedExistsCalls)
			}

		})
	}
}

func TestIsDeleteAllowed(t *testing.T) {
	testCases := []struct {
		name                  string
		setControllerNodeName bool
		nodeRef               *v1.ObjectReference
		expectedResult        bool
	}{
		{"empty controller node name should return true", false, nil, true},
		{"nil machine.Status.NodeRef should return true", true, nil, true},
		{"different node name should return true", true, &v1.ObjectReference{Name: "another-node", UID: "another-uid"}, true},
		{"same node name and different UID should return true", true, &v1.ObjectReference{Name: "controller-node-name", UID: "another-uid"}, true},
		{"same node name and same UID should return false", true, &v1.ObjectReference{Name: "controller-node-name", UID: "controller-node-uid"}, false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			controller, machine := createIsDeleteAllowedTestFixtures(t, tc.setControllerNodeName, "controller-node-name", "controller-node-uid", tc.nodeRef)
			result := controller.isDeleteAllowed(machine)
			if result != tc.expectedResult {
				t.Errorf("result mismatch: got '%v', want '%v'", result, tc.expectedResult)
			}
		})
	}
}

func createIsDeleteAllowedTestFixtures(t *testing.T, setControllerNodeNameVariable bool, controllerNodeName string, controllerNodeUid string, nodeRef *v1.ObjectReference) (*MachineControllerImpl, *v1alpha1.Machine) {
	controller := &MachineControllerImpl{}
	fakeKubernetesClient := corefake.NewSimpleClientset()
	node := v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: controllerNodeName,
			UID:  types.UID(controllerNodeUid),
		},
	}
	fakeKubernetesClient.CoreV1().Nodes().Create(&node)
	controller.kubernetesClientSet = fakeKubernetesClient
	controller.kubernetesClientSet = fakeKubernetesClient
	if setControllerNodeNameVariable {
		controller.nodeName = controllerNodeName
	}
	machine := getMachine("bar", nodeRef, true, false)
	return controller, machine
}

func getMachine(name string, nodeRef *v1.ObjectReference, isDeleting, hasFinalizer bool) *v1alpha1.Machine {
	m := &v1alpha1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Machine",
			APIVersion: v1alpha1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		Status: v1alpha1.MachineStatus{
			NodeRef: nodeRef,
		},
	}
	if isDeleting {
		now := metav1.NewTime(time.Now())
		m.ObjectMeta.SetDeletionTimestamp(&now)
	}
	if hasFinalizer {
		m.ObjectMeta.SetFinalizers([]string{v1alpha1.MachineFinalizer})
	}

	return m
}
