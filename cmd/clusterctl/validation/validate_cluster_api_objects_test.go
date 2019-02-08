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

package validation

import (
	"bytes"
	"context"
	"io/ioutil"
	"path"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1/testutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var c client.Client

func newClusterStatus(errorReason common.ClusterStatusError, errorMessage string) v1alpha1.ClusterStatus {
	return v1alpha1.ClusterStatus{
		ErrorReason:  errorReason,
		ErrorMessage: errorMessage,
	}
}

func newMachineStatus(nodeRef *v1.ObjectReference, errorReason *common.MachineStatusError, errorMessage *string) v1alpha1.MachineStatus {
	return v1alpha1.MachineStatus{
		NodeRef:      nodeRef,
		ErrorReason:  errorReason,
		ErrorMessage: errorMessage,
	}
}

func getMachineWithError(machineName, namespace string, nodeRef *v1.ObjectReference, errorReason *common.MachineStatusError, errorMessage *string) v1alpha1.Machine {
	return v1alpha1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      machineName,
			Namespace: namespace,
		},
		Status: newMachineStatus(nodeRef, errorReason, errorMessage),
	}
}

func getNodeWithReadyStatus(nodeName string, nodeReadyStatus v1.ConditionStatus) v1.Node {
	return v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{Type: v1.NodeReady, Status: nodeReadyStatus},
			},
		},
	}
}

func TestGetClusterObjectWithNoCluster(t *testing.T) {
	// Setup the Manager and Controller.
	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		t.Fatalf("error creating new manager: %v", err)
	}
	c = mgr.GetClient()
	defer close(StartTestManager(mgr, t))

	if _, err := getClusterObject(context.TODO(), c, "test-cluster", "get-cluster-object-with-no-cluster"); err == nil {
		t.Error("expected error but didn't get one")
	}
}

func TestGetClusterObjectWithOneCluster(t *testing.T) {
	// Setup the Manager and Controller.
	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		t.Fatalf("error creating new manager: %v", err)
	}
	c = mgr.GetClient()
	defer close(StartTestManager(mgr, t))

	const testClusterName = "test-cluster"
	const testNamespace = "get-cluster-object-with-one-cluster"
	cluster := testutil.GetVanillaCluster()
	cluster.Name = testClusterName
	cluster.Namespace = testNamespace
	if err := c.Create(context.TODO(), &cluster); err != nil {
		t.Fatalf("error creating cluster: %v", err)
	}
	defer c.Delete(context.TODO(), &cluster)

	var testcases = []struct {
		name        string
		clusterName string
		namespace   string
		expectErr   bool
	}{
		{
			name:        "Get cluster with name",
			clusterName: testClusterName,
			namespace:   testNamespace,
			expectErr:   false,
		},
		{
			name:        "Get cluster without name",
			clusterName: "",
			namespace:   testNamespace,
			expectErr:   false,
		},
		{
			name:        "Get cluster with name in different namespace",
			clusterName: testClusterName,
			namespace:   "different-namespace",
			expectErr:   true,
		},
		{
			name:        "Get cluster without name in different namespace",
			clusterName: "",
			namespace:   "different-namespace",
			expectErr:   true,
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			cluster, err := getClusterObject(context.TODO(), c, testcase.clusterName, testcase.namespace)
			if testcase.expectErr && err == nil {
				t.Fatalf("Expect to get error, but got no returned error.")
			}
			if !testcase.expectErr && err != nil {
				t.Fatalf("Expect to get no error, but got returned error: %v", err)
			}
			if err == nil && cluster.Name != testClusterName {
				t.Fatalf("Unexpected cluster name. Expect: %s, Got: %s", testClusterName, cluster.Name)
			}
		})
	}
}

func TestGetClusterObjectWithMoreThanOneCluster(t *testing.T) {
	// Setup the Manager and Controller.
	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		t.Fatalf("error creating new manager: %v", err)
	}
	c = mgr.GetClient()
	defer close(StartTestManager(mgr, t))

	const testNamespace = "get-cluster-object-with-more-than-one-cluster"

	const testClusterName1 = "test-cluster1"
	cluster1 := testutil.GetVanillaCluster()
	cluster1.Name = testClusterName1
	cluster1.Namespace = testNamespace
	if err := c.Create(context.TODO(), &cluster1); err != nil {
		t.Fatalf("error creating cluster1: %v", err)
	}
	defer c.Delete(context.TODO(), &cluster1)

	const testClusterName2 = "test-cluster2"
	cluster2 := testutil.GetVanillaCluster()
	cluster2.Name = testClusterName2
	cluster2.Namespace = testNamespace
	if err := c.Create(context.TODO(), &cluster2); err != nil {
		t.Fatalf("error creating cluster2: %v", err)
	}
	defer c.Delete(context.TODO(), &cluster2)

	var testcases = []struct {
		name        string
		clusterName string
		expectErr   bool
	}{
		{
			name:        "Get cluster with name",
			clusterName: testClusterName1,
			expectErr:   false,
		},
		{
			name:        "Get cluster without name",
			clusterName: "",
			expectErr:   true,
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			cluster, err := getClusterObject(context.TODO(), c, testcase.clusterName, testNamespace)
			if testcase.expectErr && err == nil {
				t.Fatalf("Expect to get error, but got no returned error.")
			}
			if !testcase.expectErr && err != nil {
				t.Fatalf("Expect to get no error, but got returned error: %v", err)
			}
			if err == nil && cluster.Name != testClusterName1 {
				t.Fatalf("Unexpected cluster name. Expect: %s, Got: %s", testClusterName1, cluster.Name)
			}
		})
	}
}

func TestValidateClusterObject(t *testing.T) {
	var testcases = []struct {
		name         string
		errorReason  common.ClusterStatusError
		errorMessage string
		expectErr    bool
	}{
		{
			name:         "Cluster has no error",
			errorReason:  "",
			errorMessage: "",
			expectErr:    false,
		},
		{
			name:         "Cluster has error reason",
			errorReason:  common.CreateClusterError,
			errorMessage: "",
			expectErr:    true,
		},
		{
			name:         "Cluster has error message",
			errorReason:  "",
			errorMessage: "Failed to create cluster",
			expectErr:    true,
		},
		{
			name:         "Cluster has error reason and message",
			errorReason:  common.CreateClusterError,
			errorMessage: "Failed to create cluster",
			expectErr:    true,
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			cluster := testutil.GetVanillaCluster()
			cluster.Name = "test-cluster"
			cluster.Namespace = "default"
			cluster.Status = newClusterStatus(testcase.errorReason, testcase.errorMessage)
			var b bytes.Buffer
			err := validateClusterObject(&b, &cluster)
			if testcase.expectErr && err == nil {
				t.Fatalf("Expect to get error, but got no returned error.")
			}
			if !testcase.expectErr && err != nil {
				t.Fatalf("Expect to get no error, but got returned error: %v", err)
			}
		})
	}
}

func TestValidateMachineObjects(t *testing.T) {
	// Setup the Manager and Controller.
	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		t.Fatalf("error creating new manager: %v", err)
	}
	c = mgr.GetClient()
	defer close(StartTestManager(mgr, t))

	const testNodeName = "test-node"
	testNode := getNodeWithReadyStatus(testNodeName, v1.ConditionTrue)
	if err := c.Create(context.TODO(), &testNode); err != nil {
		t.Fatalf("error creating node: %v", err)
	}
	defer c.Delete(context.TODO(), &testNode)

	testNodeRef := v1.ObjectReference{Kind: "Node", Name: testNodeName}
	machineErrorReason := common.CreateMachineError
	machineErrorMessage := "Failed to create machine"
	var testcases = []struct {
		name         string
		nodeRef      *v1.ObjectReference
		errorReason  *common.MachineStatusError
		errorMessage *string
		expectErr    bool
	}{
		{
			name:         "Machine has no error",
			nodeRef:      &testNodeRef,
			errorReason:  nil,
			errorMessage: nil,
			expectErr:    false,
		},
		{
			name:         "Machine has no node reference",
			nodeRef:      nil,
			errorReason:  nil,
			errorMessage: nil,
			expectErr:    true,
		},
		{
			name:         "Machine has error reason",
			nodeRef:      &testNodeRef,
			errorReason:  &machineErrorReason,
			errorMessage: nil,
			expectErr:    true,
		},
		{
			name:         "Machine has error message",
			nodeRef:      &testNodeRef,
			errorReason:  nil,
			errorMessage: &machineErrorMessage,
			expectErr:    true,
		},
		{
			name:         "Machine has error reason and message",
			nodeRef:      &testNodeRef,
			errorReason:  &machineErrorReason,
			errorMessage: &machineErrorMessage,
			expectErr:    true,
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			machines := v1alpha1.MachineList{
				Items: []v1alpha1.Machine{
					getMachineWithError("test-machine-with-no-error", "default", &testNodeRef, nil, nil),
					getMachineWithError("test-machine", "default", testcase.nodeRef, testcase.errorReason, testcase.errorMessage),
				},
			}
			var b bytes.Buffer
			err := validateMachineObjects(context.TODO(), &b, &machines, c)
			if testcase.expectErr && err == nil {
				t.Errorf("Expect to get error, but got no returned error: %v", b.String())
			}
			if !testcase.expectErr && err != nil {
				t.Errorf("Expect to get no error, but got returned error: %v: %v", err, b.String())
			}
		})
	}
}

func TestValidateMachineObjectWithReferredNode(t *testing.T) {
	// Setup the Manager and Controller.
	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		t.Fatalf("error creating new manager: %v", err)
	}
	c = mgr.GetClient()
	defer close(StartTestManager(mgr, t))

	const testNodeReadyName = "test-node-ready"
	testNodeReady := getNodeWithReadyStatus(testNodeReadyName, v1.ConditionTrue)
	if err := c.Create(context.TODO(), &testNodeReady); err != nil {
		t.Fatalf("error creating node: %v", err)
	}
	defer c.Delete(context.TODO(), &testNodeReady)

	const testNodeNotReadyName = "test-node-not-ready"
	testNodeNotReady := getNodeWithReadyStatus(testNodeNotReadyName, v1.ConditionFalse)
	if err := c.Create(context.TODO(), &testNodeNotReady); err != nil {
		t.Fatalf("error creating node: %v", err)
	}
	defer c.Delete(context.TODO(), &testNodeNotReady)

	const testNodeNotExistName = "test-node-not-exist"

	var testcases = []struct {
		name      string
		nodeRef   v1.ObjectReference
		expectErr bool
	}{
		{
			name:      "Machine's ref node is ready",
			nodeRef:   v1.ObjectReference{Kind: "Node", Name: testNodeReadyName},
			expectErr: false,
		},
		{
			name:      "Machine's ref node is not ready",
			nodeRef:   v1.ObjectReference{Kind: "Node", Name: testNodeNotReadyName},
			expectErr: true,
		},
		{
			name:      "Machine's ref node does not exist",
			nodeRef:   v1.ObjectReference{Kind: "Node", Name: testNodeNotExistName},
			expectErr: true,
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			machines := v1alpha1.MachineList{
				Items: []v1alpha1.Machine{
					getMachineWithError("test-machine", "default", &testcase.nodeRef, nil, nil),
				},
			}
			var b bytes.Buffer
			err := validateMachineObjects(context.TODO(), &b, &machines, c)
			if testcase.expectErr && err == nil {
				t.Fatalf("Expect to get error, but got no returned error.")
			}
			if !testcase.expectErr && err != nil {
				t.Fatalf("Expect to get no error, but got returned error: %v", err)
			}
		})
	}
}

func TestValidateClusterAPIObjectsOutput(t *testing.T) {
	// Setup the Manager and Controller.
	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		t.Fatalf("error creating new manager: %v", err)
	}
	defer close(StartTestManager(mgr, t))

	const testClusterName = "test-cluster"
	const testMachine1Name = "test-machine1"
	const testMachine2Name = "test-machine2"
	const testNode1Name = "test-node1"
	const testNode2Name = "test-node2"
	const testNodeNotReadyName = "test-node-not-ready"

	c, err := client.New(cfg, client.Options{Scheme: mgr.GetScheme(), Mapper: mgr.GetRESTMapper()})
	if err != nil {
		t.Fatalf("error creating client: %v", err)
	}

	testNodeRef1 := v1.ObjectReference{Kind: "Node", Name: testNode1Name}
	testNodeRef2 := v1.ObjectReference{Kind: "Node", Name: testNode2Name}
	testNodeRefNotReady := v1.ObjectReference{Kind: "Node", Name: testNodeNotReadyName}
	testNodeRefNotExist := v1.ObjectReference{Kind: "Node", Name: "test-node-not-exist"}
	machineErrorReason := common.CreateMachineError
	machineErrorMessage := "Failed to create machine"

	var testcases = []struct {
		name           string
		namespace      string
		clusterStatus  v1alpha1.ClusterStatus
		machine1Status v1alpha1.MachineStatus
		machine2Status v1alpha1.MachineStatus
		expectErr      bool
		outputFileName string
	}{
		{
			name:           "Pass",
			namespace:      "validate-cluster-objects",
			clusterStatus:  v1alpha1.ClusterStatus{},
			machine1Status: newMachineStatus(&testNodeRef1, nil, nil),
			machine2Status: newMachineStatus(&testNodeRef2, nil, nil),
			expectErr:      false,
			outputFileName: "validate-cluster-api-object-output-pass.golden",
		},
		{
			name:           "Failed to validate cluster object",
			namespace:      "validate-cluster-objects-errors",
			clusterStatus:  newClusterStatus(common.CreateClusterError, "Failed to create cluster"),
			machine1Status: newMachineStatus(&testNodeRef1, nil, nil),
			machine2Status: newMachineStatus(&testNodeRef2, nil, nil),
			expectErr:      true,
			outputFileName: "fail-to-validate-cluster-object.golden",
		},
		{
			name:           "Failed to validate machine objects with errors",
			namespace:      "validate-machine-objects-errors",
			clusterStatus:  v1alpha1.ClusterStatus{},
			machine1Status: newMachineStatus(&testNodeRef1, &machineErrorReason, &machineErrorMessage),
			machine2Status: v1alpha1.MachineStatus{}, // newMachineStatus(nil, nil, nil),
			expectErr:      true,
			outputFileName: "fail-to-validate-machine-objects-with-errors.golden",
		},
		{
			name:           "Failed to validate machine objects with node ref errors",
			namespace:      "validate-machine-objects-node-ref-errors",
			clusterStatus:  v1alpha1.ClusterStatus{},
			machine1Status: newMachineStatus(&testNodeRefNotReady, nil, nil),
			machine2Status: newMachineStatus(&testNodeRefNotExist, nil, nil),
			expectErr:      true,
			outputFileName: "fail-to-validate-machine-objects-with-noderef-errors.golden",
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			cluster := testutil.GetVanillaCluster()
			cluster.Name = testClusterName
			cluster.Namespace = testcase.namespace
			if err := c.Create(context.TODO(), &cluster); err != nil {
				t.Fatalf("error creating cluster: %v", err)
			}
			defer c.Delete(context.TODO(), &cluster)

			machine1 := getMachineWithError(testMachine1Name, testcase.namespace, nil, nil, nil) // machine with no error
			if err := c.Create(context.TODO(), &machine1); err != nil {
				t.Fatalf("error creating machine1: %v", err)
			}
			defer c.Delete(context.TODO(), &machine1)

			machine2 := getMachineWithError(testMachine2Name, testcase.namespace, nil, nil, nil) // machine with no error
			if err := c.Create(context.TODO(), &machine2); err != nil {
				t.Fatalf("error creating machine2: %v", err)
			}
			defer c.Delete(context.TODO(), &machine2)

			testNode1 := getNodeWithReadyStatus(testNode1Name, v1.ConditionTrue)
			if err := c.Create(context.TODO(), &testNode1); err != nil {
				t.Fatalf("error creating node1: %v", err)
			}
			defer c.Delete(context.TODO(), &testNode1)

			testNode2 := getNodeWithReadyStatus(testNode2Name, v1.ConditionTrue)
			if err := c.Create(context.TODO(), &testNode2); err != nil {
				t.Fatalf("error creating node2: %v", err)
			}
			defer c.Delete(context.TODO(), &testNode2)

			testNodeNotReady := getNodeWithReadyStatus(testNodeNotReadyName, v1.ConditionFalse)
			if err := c.Create(context.TODO(), &testNodeNotReady); err != nil {
				t.Fatalf("error creating node: %v", err)
			}
			defer c.Delete(context.TODO(), &testNodeNotReady)

			if err := c.Get(context.TODO(), types.NamespacedName{Name: testClusterName, Namespace: testcase.namespace}, &cluster); err != nil {
				t.Fatalf("Unable to get cluster: %v", err)
			}
			cluster.Status = testcase.clusterStatus
			if err := c.Status().Update(context.TODO(), &cluster); err != nil {
				t.Fatalf("Unable to update cluster with status: %v", err)
			}
			if err := c.Get(context.TODO(), types.NamespacedName{Name: testMachine1Name, Namespace: testcase.namespace}, &machine1); err != nil {
				t.Fatalf("Unable to get machine 1: %v", err)
			}

			machine1.Status = testcase.machine1Status
			if err := c.Status().Update(context.TODO(), &machine1); err != nil {
				t.Fatalf("Unable to update machine 1 with status: %v", err)
			}

			if err := c.Get(context.TODO(), types.NamespacedName{Name: testMachine2Name, Namespace: testcase.namespace}, &machine2); err != nil {
				t.Fatalf("Unable to get machine 2: %v", err)
			}
			machine2.Status = testcase.machine2Status
			if err := c.Status().Update(context.TODO(), &machine2); err != nil {
				t.Fatalf("Unable to update machine 2 with status: %v", err)
			}

			var output bytes.Buffer
			err = ValidateClusterAPIObjects(context.TODO(), &output, c, testClusterName, testcase.namespace)

			if testcase.expectErr && err == nil {
				t.Fatalf("Expect to get error, but got no returned error: %v", output.String())
			}
			if !testcase.expectErr && err != nil {
				t.Fatalf("Expect to get no error, but got returned error: %v: %v", err, output.String())
			}

			outputFilePath := path.Join("testdata", testcase.outputFileName)
			expectedOutput, err := ioutil.ReadFile(outputFilePath)
			if err != nil {
				t.Fatalf("Unable to read output file '%v': %v", outputFilePath, err)
			}
			if !bytes.Equal(expectedOutput, output.Bytes()) {
				t.Errorf("Unexpected output. Expect %v, Got %v", string(expectedOutput), output.String())
			}
		})
	}

}
