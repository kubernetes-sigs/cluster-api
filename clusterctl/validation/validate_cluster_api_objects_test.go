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
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/kubernetes-incubator/apiserver-builder/pkg/test"
	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/cluster-api/pkg/apis"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1/testutil"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	"sigs.k8s.io/cluster-api/pkg/openapi"
)

var clusterApiClient *clientset.Clientset
var k8sClient kubernetes.Interface

func TestMain(m *testing.M) {
	testenv := test.NewTestEnvironment()
	config := testenv.Start(apis.GetAllApiBuilders(), openapi.GetOpenAPIDefinitions)
	clusterApiClient = clientset.NewForConfigOrDie(config)
	k8sClient = fake.NewSimpleClientset()

	code := m.Run()

	testenv.Stop()
	os.Exit(code)
}

func getClusterWithError(clusterName string, errorReason common.ClusterStatusError, errorMessage string) v1alpha1.Cluster {
	return v1alpha1.Cluster{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: clusterName,
		},
		Spec: v1alpha1.ClusterSpec{
			ClusterNetwork: v1alpha1.ClusterNetworkingConfig{
				Services: v1alpha1.NetworkRanges{
					CIDRBlocks: []string{"10.96.0.0/12"},
				},
				Pods: v1alpha1.NetworkRanges{
					CIDRBlocks: []string{"192.168.0.0/16"},
				},
				ServiceDomain: "cluster.local",
			},
		},
		Status: v1alpha1.ClusterStatus{
			ErrorReason:  errorReason,
			ErrorMessage: errorMessage,
		},
	}
}

func getMachineWithError(machineName string, nodeRef *v1.ObjectReference, errorReason *common.MachineStatusError, errorMessage *string) v1alpha1.Machine {
	return v1alpha1.Machine{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: machineName,
		},
		Status: v1alpha1.MachineStatus{
			NodeRef:      nodeRef,
			ErrorReason:  errorReason,
			ErrorMessage: errorMessage,
		},
	}
}

func getNodeWithReadyStatus(nodeName string, nodeReadyStatus v1.ConditionStatus) v1.Node {
	return v1.Node{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: nodeName,
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				v1.NodeCondition{
					Type: v1.NodeReady,
					Status: nodeReadyStatus,
				},
			},
		},
	}
}

func TestGetClusterObjectWithNoCluster(t *testing.T) {
	t.Run("Get cluster", func(t *testing.T) {
		_, err := getClusterObject(clusterApiClient, "test-cluster", "get-cluster-object-with-no-cluster")
		if err == nil {
			t.Fatalf("Expect to get error, but got no returned error.")
		}
	})
}

func TestGetClusterObjectWithOneCluster(t *testing.T) {
	testClusterName := "test-cluster"
	testNamespace := "get-cluster-object-with-one-cluster"
	clusterClient := clusterApiClient.ClusterV1alpha1().Clusters(testNamespace)
	cluster := testutil.GetVanillaCluster()
	cluster.Name = testClusterName
	actualCluster, err := clusterClient.Create(&cluster)
	if err != nil {
		t.Fatal(err)
	}
	defer clusterClient.Delete(actualCluster.Name, &meta_v1.DeleteOptions{})
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
			cluster, err := getClusterObject(clusterApiClient, testcase.clusterName, testcase.namespace)
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
	testNamespace := "get-cluster-object-with-more-than-one-cluster"
	clusterClient := clusterApiClient.ClusterV1alpha1().Clusters(testNamespace)

	testClusterName1 := "test-cluster1"
	cluster1 := testutil.GetVanillaCluster()
	cluster1.Name = testClusterName1
	actualCluster1, err := clusterClient.Create(&cluster1)
	if err != nil {
		t.Fatal(err)
	}
	defer clusterClient.Delete(actualCluster1.Name, &meta_v1.DeleteOptions{})

	testClusterName2 := "test-cluster2"
	cluster2 := testutil.GetVanillaCluster()
	cluster2.Name = testClusterName2
	actualCluster2, err := clusterClient.Create(&cluster2)

	if err != nil {
		t.Fatal(err)
	}
	defer clusterClient.Delete(actualCluster2.Name, &meta_v1.DeleteOptions{})

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
			cluster, err := getClusterObject(clusterApiClient, testcase.clusterName, testNamespace)
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
			cluster := getClusterWithError("test-cluster", testcase.errorReason, testcase.errorMessage)
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
	testNodeName := "test-node"
	testNode := getNodeWithReadyStatus(testNodeName, v1.ConditionTrue)
	actualNode, err := k8sClient.CoreV1().Nodes().Create(&testNode)
	if err != nil {
		t.Fatal(err)
	}
	defer k8sClient.CoreV1().Nodes().Delete(actualNode.Name, &meta_v1.DeleteOptions{})

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
					getMachineWithError("test-machine-with-no-error", &testNodeRef, nil, nil),
					getMachineWithError("test-machine", testcase.nodeRef, testcase.errorReason, testcase.errorMessage),
				},
			}
			var b bytes.Buffer
			err := validateMachineObjects(&b, &machines, k8sClient)
			if testcase.expectErr && err == nil {
				t.Fatalf("Expect to get error, but got no returned error.")
			}
			if !testcase.expectErr && err != nil {
				t.Fatalf("Expect to get no error, but got returned error: %v", err)
			}
		})
	}
}

func TestValidateMachineObjectWithReferredNode(t *testing.T) {
	testNodeReadyName := "test-node-ready"
	testNodeReady := getNodeWithReadyStatus(testNodeReadyName, v1.ConditionTrue)
	actualTestNodeReady, err := k8sClient.CoreV1().Nodes().Create(&testNodeReady)
	if err != nil {
		t.Fatal(err)
	}
	defer k8sClient.CoreV1().Nodes().Delete(actualTestNodeReady.Name, &meta_v1.DeleteOptions{})

	testNodeNotReadyName := "test-node-not-ready"
	testNodeNotReady := getNodeWithReadyStatus(testNodeNotReadyName, v1.ConditionFalse)
	actualTestNodeNotReady, err := k8sClient.CoreV1().Nodes().Create(&testNodeNotReady)
	if err != nil {
		t.Fatal(err)
	}
	defer k8sClient.CoreV1().Nodes().Delete(actualTestNodeNotReady.Name, &meta_v1.DeleteOptions{})

	testNodeNotExistName := "test-node-not-exist"

	var testcases = []struct {
		name         string
		nodeRef      v1.ObjectReference
		expectErr    bool
	}{
		{
			name:         "Machine's ref node is ready",
			nodeRef:      v1.ObjectReference{Kind: "Node", Name: testNodeReadyName},
			expectErr:    false,
		},
		{
			name:         "Machine's ref node is not ready",
			nodeRef:      v1.ObjectReference{Kind: "Node", Name: testNodeNotReadyName},
			expectErr:    true,
		},
		{
			name:         "Machine's ref node does not exist",
			nodeRef:      v1.ObjectReference{Kind: "Node", Name: testNodeNotExistName},
			expectErr:    true,
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			machines := v1alpha1.MachineList{
				Items: []v1alpha1.Machine{
					getMachineWithError("test-machine", &testcase.nodeRef, nil, nil),
				},
			}
			var b bytes.Buffer
			err := validateMachineObjects(&b, &machines, k8sClient)
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
	testNamespace := "validate-cluster-api-object-output"
	clusterClient := clusterApiClient.ClusterV1alpha1().Clusters(testNamespace)

	testClusterName := "test-cluster"
	cluster := testutil.GetVanillaCluster()
	cluster.Name = testClusterName
	actualCluster, err := clusterClient.Create(&cluster)
	if err != nil {
		t.Fatal(err)
	}
	defer clusterClient.Delete(actualCluster.Name, &meta_v1.DeleteOptions{})

	machineClient := clusterApiClient.ClusterV1alpha1().Machines(testNamespace)
	testMachine1Name := "test-machine1"
	machine1 := getMachineWithError(testMachine1Name, nil, nil, nil) // machine with no error
	_, err = machineClient.Create(&machine1)
	if err != nil {
		t.Fatal(err)
	}
	defer machineClient.Delete(machine1.Name, &meta_v1.DeleteOptions{})
	testMachine2Name := "test-machine2"
	machine2 := getMachineWithError(testMachine2Name, nil, nil, nil) // machine with no error
	_, err = machineClient.Create(&machine2)
	if err != nil {
		t.Fatal(err)
	}
	defer machineClient.Delete(machine2.Name, &meta_v1.DeleteOptions{})

	testNode1Name := "test-node1"
	testNode1 := getNodeWithReadyStatus(testNode1Name, v1.ConditionTrue)
	actualNode1, err := k8sClient.CoreV1().Nodes().Create(&testNode1)
	if err != nil {
		t.Fatal(err)
	}
	defer k8sClient.CoreV1().Nodes().Delete(actualNode1.Name, &meta_v1.DeleteOptions{})

	testNode2Name := "test-node2"
	testNode2 := getNodeWithReadyStatus(testNode2Name, v1.ConditionTrue)
	actualNode2, err := k8sClient.CoreV1().Nodes().Create(&testNode2)
	if err != nil {
		t.Fatal(err)
	}
	defer k8sClient.CoreV1().Nodes().Delete(actualNode2.Name, &meta_v1.DeleteOptions{})

	testNodeNotReadyName := "test-node-not-ready"
	testNodeNotReady := getNodeWithReadyStatus(testNodeNotReadyName, v1.ConditionFalse)
	actualTestNodeNotReady, err := k8sClient.CoreV1().Nodes().Create(&testNodeNotReady)
	if err != nil {
		t.Fatal(err)
	}
	defer k8sClient.CoreV1().Nodes().Delete(actualTestNodeNotReady.Name, &meta_v1.DeleteOptions{})

	testNodeRef1 := v1.ObjectReference{Kind: "Node", Name: testNode1Name}
	testNodeRef2 := v1.ObjectReference{Kind: "Node", Name: testNode2Name}
	testNodeRefNotReady := v1.ObjectReference{Kind: "Node", Name: testNodeNotReadyName}
	testNodeRefNotExist := v1.ObjectReference{Kind: "Node", Name: "test-node-not-exist"}
	machineErrorReason := common.CreateMachineError
	machineErrorMessage := "Failed to create machine"
	var testcases = []struct {
		name               string
		clusterWithStatus  v1alpha1.Cluster
		machine1WithStatus v1alpha1.Machine
		machine2WithStatus v1alpha1.Machine
		expectErr          bool
		outputFileName     string
	}{
		{
			name:               "Pass",
			clusterWithStatus:  getClusterWithError(testClusterName, "", ""),
			machine1WithStatus: getMachineWithError(testMachine1Name, &testNodeRef1, nil, nil),
			machine2WithStatus: getMachineWithError(testMachine2Name, &testNodeRef2, nil, nil),
			expectErr:          false,
			outputFileName:     "validate-cluster-api-object-output-pass.golden",
		},
		{
			name:               "Failed to validate cluster object",
			clusterWithStatus:  getClusterWithError(testClusterName, common.CreateClusterError, "Failed to create cluster"),
			machine1WithStatus: getMachineWithError(testMachine1Name, &testNodeRef1, nil, nil),
			machine2WithStatus: getMachineWithError(testMachine2Name, &testNodeRef2, nil, nil),
			expectErr:          true,
			outputFileName:     "fail-to-validate-cluster-object.golden",
		},
		{
			name:               "Failed to validate machine objects with errors",
			clusterWithStatus:  getClusterWithError(testClusterName, "", ""),
			machine1WithStatus: getMachineWithError(testMachine1Name, &testNodeRef1, &machineErrorReason, &machineErrorMessage),
			machine2WithStatus: getMachineWithError(testMachine2Name, nil, nil, nil),
			expectErr:          true,
			outputFileName:     "fail-to-validate-machine-objects-with-errors.golden",
		},
		{
			name:               "Failed to validate machine objects with node ref errors",
			clusterWithStatus:  getClusterWithError(testClusterName, "", ""),
			machine1WithStatus: getMachineWithError(testMachine1Name, &testNodeRefNotReady, nil, nil),
			machine2WithStatus: getMachineWithError(testMachine2Name, &testNodeRefNotExist, nil, nil),
			expectErr:          true,
			outputFileName:     "fail-to-validate-machine-objects-with-noderef-errors.golden",
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			_, err = clusterClient.UpdateStatus(&testcase.clusterWithStatus)
			if err != nil {
				t.Fatal(err)
			}
			testcase.machine1WithStatus.Name = testMachine1Name
			_, err = machineClient.UpdateStatus(&testcase.machine1WithStatus)
			if err != nil {
				t.Fatal(err)
			}
			testcase.machine2WithStatus.Name = testMachine2Name
			_, err = machineClient.UpdateStatus(&testcase.machine2WithStatus)
			if err != nil {
				t.Fatal(err)
			}

			var output bytes.Buffer
			err = ValidateClusterAPIObjects(&output, clusterApiClient, k8sClient, testClusterName, testNamespace)

			if testcase.expectErr && err == nil {
				t.Fatalf("Expect to get error, but got no returned error.")
			}
			if !testcase.expectErr && err != nil {
				t.Fatalf("Expect to get no error, but got returned error: %v", err)
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
