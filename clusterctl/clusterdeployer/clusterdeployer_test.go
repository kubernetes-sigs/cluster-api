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

package clusterdeployer

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

type testClusterProvisioner struct {
	err            error
	clusterCreated bool
	clusterExists  bool
	kubeconfig     string
}

func (p *testClusterProvisioner) Create() error {
	if p.err != nil {
		return p.err
	}
	p.clusterCreated = true
	p.clusterExists = true
	return nil
}

func (p *testClusterProvisioner) Delete() error {
	if p.err != nil {
		return p.err
	}
	p.clusterExists = false
	return nil
}

func (p *testClusterProvisioner) GetKubeconfig() (string, error) {
	return p.kubeconfig, p.err
}

type mockProviderComponentsStoreFactory struct {
	NewFromCoreclientsetPCStore          ProviderComponentsStore
	NewFromCoreclientsetError            error
	NewFromCoreclientsetCapturedArgument *kubernetes.Clientset
}

func (m *mockProviderComponentsStoreFactory) NewFromCoreClientset(clientset *kubernetes.Clientset) (ProviderComponentsStore, error) {
	m.NewFromCoreclientsetCapturedArgument = clientset
	return m.NewFromCoreclientsetPCStore, m.NewFromCoreclientsetError
}

type mockProviderComponentsStore struct {
	SaveErr                        error
	SaveCapturedProviderComponents string
	LoadResult                     string
	LoadErr                        error
}

func (m *mockProviderComponentsStore) Save(providerComponents string) error {
	m.SaveCapturedProviderComponents = providerComponents
	return m.SaveErr
}

func (m *mockProviderComponentsStore) Load() (string, error) {
	return m.LoadResult, m.LoadErr
}

type testClusterClient struct {
	ApplyErr                           error
	DeleteErr                          error
	WaitForClusterV1alpha1ReadyErr     error
	GetClusterObjectsErr               error
	GetMachineDeploymentObjectsErr     error
	GetMachineSetObjectsErr            error
	GetMachineObjectsErr               error
	CreateClusterObjectErr             error
	CreateMachineObjectsErr            error
	CreateMachineSetObjectsErr         error
	CreateMachineDeploymentsObjectsErr error
	DeleteClusterObjectsErr            error
	DeleteMachineObjectsErr            error
	DeleteMachineSetObjectsErr         error
	DeleteMachineDeploymentsObjectsErr error
	UpdateClusterObjectEndpointErr     error
	CloseErr                           error

	clusters           []*clusterv1.Cluster
	machineDeployments []*clusterv1.MachineDeployment
	machineSets        []*clusterv1.MachineSet
	machines           []*clusterv1.Machine
}

func (c *testClusterClient) Apply(string) error {
	return c.ApplyErr
}

func (c *testClusterClient) Delete(string) error {
	return c.DeleteErr
}

func (c *testClusterClient) WaitForClusterV1alpha1Ready() error {
	return c.WaitForClusterV1alpha1ReadyErr
}

func (c *testClusterClient) GetClusterObjects() ([]*clusterv1.Cluster, error) {
	return c.clusters, c.GetClusterObjectsErr
}

func (c *testClusterClient) GetMachineDeploymentObjects() ([]*clusterv1.MachineDeployment, error) {
	return c.machineDeployments, c.GetMachineDeploymentObjectsErr
}

func (c *testClusterClient) GetMachineSetObjects() ([]*clusterv1.MachineSet, error) {
	return c.machineSets, c.GetMachineSetObjectsErr
}

func (c *testClusterClient) GetMachineObjects() ([]*clusterv1.Machine, error) {
	return c.machines, c.GetMachineObjectsErr
}

func (c *testClusterClient) CreateClusterObject(cluster *clusterv1.Cluster) error {
	if c.CreateClusterObjectErr == nil {
		c.clusters = append(c.clusters, cluster)
		return nil
	}
	return c.CreateClusterObjectErr
}

func (c *testClusterClient) CreateMachineDeploymentObjects(deployments []*clusterv1.MachineDeployment) error {
	if c.CreateMachineDeploymentsObjectsErr == nil {
		c.machineDeployments = append(c.machineDeployments, deployments...)
		return nil
	}
	return c.CreateMachineDeploymentsObjectsErr
}

func (c *testClusterClient) CreateMachineSetObjects(machineSets []*clusterv1.MachineSet) error {
	if c.CreateMachineSetObjectsErr == nil {
		c.machineSets = append(c.machineSets, machineSets...)
		return nil
	}
	return c.CreateMachineSetObjectsErr
}

func (c *testClusterClient) CreateMachineObjects(machines []*clusterv1.Machine) error {
	if c.CreateMachineObjectsErr == nil {
		c.machines = append(c.machines, machines...)
		return nil
	}
	return c.CreateMachineObjectsErr
}

func (c *testClusterClient) DeleteClusterObjects() error {
	return c.DeleteClusterObjectsErr
}

func (c *testClusterClient) DeleteMachineDeploymentObjects() error {
	return c.DeleteMachineDeploymentsObjectsErr
}

func (c *testClusterClient) DeleteMachineSetObjects() error {
	return c.DeleteMachineSetObjectsErr
}

func (c *testClusterClient) DeleteMachineObjects() error {
	return c.DeleteMachineObjectsErr
}

func (c *testClusterClient) UpdateClusterObjectEndpoint(string) error {
	return c.UpdateClusterObjectEndpointErr
}
func (c *testClusterClient) Close() error {
	return c.CloseErr
}

type testClusterClientFactory struct {
	ClusterClientErr    error
	clusterClients      map[string]*testClusterClient
	NewCoreClientsetErr error
	CoreClientsets      map[string]*kubernetes.Clientset
}

func newTestClusterClientFactory() *testClusterClientFactory {
	return &testClusterClientFactory{
		clusterClients: map[string]*testClusterClient{},
	}
}

func (f *testClusterClientFactory) NewCoreClientsetFromKubeconfigFile(kubeconfigPath string) (*kubernetes.Clientset, error) {
	return f.CoreClientsets[kubeconfigPath], f.NewCoreClientsetErr
}

func (f *testClusterClientFactory) NewClusterClientFromKubeconfig(kubeconfig string) (ClusterClient, error) {
	if f.ClusterClientErr != nil {
		return nil, f.ClusterClientErr
	}
	return f.clusterClients[kubeconfig], nil
}

type testProviderDeployer struct {
	GetIPErr         error
	GetKubeConfigErr error
	ip               string
	kubeconfig       string
}

func (d *testProviderDeployer) GetIP(_ *clusterv1.Cluster, _ *clusterv1.Machine) (string, error) {
	return d.ip, d.GetIPErr
}
func (d *testProviderDeployer) GetKubeConfig(_ *clusterv1.Cluster, _ *clusterv1.Machine) (string, error) {
	return d.kubeconfig, d.GetKubeConfigErr
}

func TestCreate(t *testing.T) {
	const bootstrapKubeconfig = "bootstrap"
	const targetKubeconfig = "target"

	var testcases = []struct {
		name                     string
		provisionExternalErr     error
		factoryClusterClientErr  error
		bootstrapClient          *testClusterClient
		targetClient             *testClusterClient
		cleanupExternal          bool
		expectErr                bool
		expectExternalExists     bool
		expectExternalCreated    bool
		expectedInternalClusters int
		expectedInternalMachines int
	}{
		{
			name:                     "success",
			targetClient:             &testClusterClient{},
			bootstrapClient:          &testClusterClient{},
			cleanupExternal:          true,
			expectExternalExists:     false,
			expectExternalCreated:    true,
			expectedInternalClusters: 1,
			expectedInternalMachines: 2,
		},
		{
			name:                     "success no cleaning bootstrap",
			targetClient:             &testClusterClient{},
			bootstrapClient:          &testClusterClient{},
			cleanupExternal:          false,
			expectExternalExists:     true,
			expectExternalCreated:    true,
			expectedInternalClusters: 1,
			expectedInternalMachines: 2,
		},
		{
			name:                 "fail provision bootstrap cluster",
			targetClient:         &testClusterClient{},
			bootstrapClient:      &testClusterClient{},
			provisionExternalErr: fmt.Errorf("Test failure"),
			expectErr:            true,
		},
		{
			name:                    "fail create clients",
			targetClient:            &testClusterClient{},
			bootstrapClient:         &testClusterClient{},
			cleanupExternal:         true,
			expectExternalCreated:   true,
			factoryClusterClientErr: fmt.Errorf("Test failure"),
			expectErr:               true,
		},
		{
			name:                  "fail apply yaml to bootstrap cluster",
			targetClient:          &testClusterClient{},
			bootstrapClient:       &testClusterClient{ApplyErr: fmt.Errorf("Test failure")},
			cleanupExternal:       true,
			expectExternalCreated: true,
			expectErr:             true,
		},
		{
			name:                  "fail waiting for api ready on bootstrap cluster",
			targetClient:          &testClusterClient{},
			bootstrapClient:       &testClusterClient{WaitForClusterV1alpha1ReadyErr: fmt.Errorf("Test failure")},
			cleanupExternal:       true,
			expectExternalCreated: true,
			expectErr:             true,
		},
		{
			name:                  "fail getting bootstrap cluster objects",
			targetClient:          &testClusterClient{},
			bootstrapClient:       &testClusterClient{GetClusterObjectsErr: fmt.Errorf("Test failure")},
			cleanupExternal:       true,
			expectExternalCreated: true,
			expectErr:             true,
		},
		{
			name:                  "fail getting bootstrap machine objects",
			targetClient:          &testClusterClient{},
			bootstrapClient:       &testClusterClient{GetMachineObjectsErr: fmt.Errorf("Test failure")},
			cleanupExternal:       true,
			expectExternalCreated: true,
			expectErr:             true,
		},
		{
			name:                  "fail create cluster",
			targetClient:          &testClusterClient{},
			bootstrapClient:       &testClusterClient{CreateClusterObjectErr: fmt.Errorf("Test failure")},
			cleanupExternal:       true,
			expectExternalCreated: true,
			expectErr:             true,
		},
		{
			name:                  "fail create master",
			targetClient:          &testClusterClient{},
			bootstrapClient:       &testClusterClient{CreateMachineObjectsErr: fmt.Errorf("Test failure")},
			cleanupExternal:       true,
			expectExternalCreated: true,
			expectErr:             true,
		},
		{
			name:                  "fail update bootstrap cluster endpoint",
			targetClient:          &testClusterClient{},
			bootstrapClient:       &testClusterClient{UpdateClusterObjectEndpointErr: fmt.Errorf("Test failure")},
			cleanupExternal:       true,
			expectExternalCreated: true,
			expectErr:             true,
		},
		{
			name:                  "fail apply yaml to target cluster",
			targetClient:          &testClusterClient{ApplyErr: fmt.Errorf("Test failure")},
			bootstrapClient:       &testClusterClient{},
			cleanupExternal:       true,
			expectExternalCreated: true,
			expectErr:             true,
		},
		{
			name:                  "fail wait for api ready on target cluster",
			targetClient:          &testClusterClient{WaitForClusterV1alpha1ReadyErr: fmt.Errorf("Test failure")},
			bootstrapClient:       &testClusterClient{},
			cleanupExternal:       true,
			expectExternalCreated: true,
			expectErr:             true,
		},
		{
			name:                  "fail  create target cluster",
			targetClient:          &testClusterClient{CreateClusterObjectErr: fmt.Errorf("Test failure")},
			bootstrapClient:       &testClusterClient{},
			cleanupExternal:       true,
			expectExternalCreated: true,
			expectErr:             true,
		},
		{
			name:                     "fail create nodes",
			targetClient:             &testClusterClient{CreateMachineObjectsErr: fmt.Errorf("Test failure")},
			bootstrapClient:          &testClusterClient{},
			cleanupExternal:          true,
			expectExternalCreated:    true,
			expectedInternalClusters: 1,
			expectErr:                true,
		},
		{
			name:                     "fail update cluster endpoint target",
			targetClient:             &testClusterClient{UpdateClusterObjectEndpointErr: fmt.Errorf("Test failure")},
			bootstrapClient:          &testClusterClient{},
			cleanupExternal:          true,
			expectExternalCreated:    true,
			expectedInternalClusters: 1,
			expectedInternalMachines: 1,
			expectErr:                true,
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			kubeconfigOut := newTempFile(t)
			defer os.Remove(kubeconfigOut)

			// Create provisioners & clients and hook them up
			p := &testClusterProvisioner{
				err:        testcase.provisionExternalErr,
				kubeconfig: bootstrapKubeconfig,
			}
			pd := &testProviderDeployer{}
			pd.kubeconfig = targetKubeconfig
			f := newTestClusterClientFactory()
			f.clusterClients[bootstrapKubeconfig] = testcase.bootstrapClient
			f.clusterClients[targetKubeconfig] = testcase.targetClient
			f.ClusterClientErr = testcase.factoryClusterClientErr

			// Create
			inputCluster := &clusterv1.Cluster{}
			inputCluster.Name = "test-cluster"
			inputMachines := generateMachines()
			pcStore := mockProviderComponentsStore{}
			pcFactory := mockProviderComponentsStoreFactory{NewFromCoreclientsetPCStore: &pcStore}
			d := New(p, f, "", "", testcase.cleanupExternal)
			err := d.Create(inputCluster, inputMachines, pd, kubeconfigOut, &pcFactory)

			// Validate
			if (testcase.expectErr && err == nil) || (!testcase.expectErr && err != nil) {
				t.Fatalf("Unexpected returned error. Got: %v, Want Err: %v", err, testcase.expectErr)
			}
			if testcase.expectExternalExists != p.clusterExists {
				t.Errorf("Unexpected bootstrap cluster existance. Got: %v, Want: %v", p.clusterExists, testcase.expectExternalExists)
			}
			if testcase.expectExternalCreated != p.clusterCreated {
				t.Errorf("Unexpected bootstrap cluster provisioning. Got: %v, Want: %v", p.clusterCreated, testcase.expectExternalCreated)
			}
			if testcase.expectedInternalClusters != len(testcase.targetClient.clusters) {
				t.Fatalf("Unexpected cluster count. Got: %v, Want: %v", len(testcase.targetClient.clusters), testcase.expectedInternalClusters)
			}
			if testcase.expectedInternalClusters > 1 && inputCluster.Name != testcase.targetClient.clusters[0].Name {
				t.Errorf("Provisioned cluster has unexpected name. Got: %v, Want: %v", testcase.targetClient.clusters[0].Name, inputCluster.Name)
			}

			if testcase.expectedInternalMachines != len(testcase.targetClient.machines) {
				t.Fatalf("Unexpected machine count. Got: %v, Want: %v", len(testcase.targetClient.machines), testcase.expectedInternalMachines)
			}
			if testcase.expectedInternalMachines == len(inputMachines) {
				for i := range inputMachines {
					if inputMachines[i].Name != testcase.targetClient.machines[i].Name {
						t.Fatalf("Unexpected machine name at %v. Got: %v, Want: %v", i, inputMachines[i].Name, testcase.targetClient.machines[i].Name)
					}
				}
			}
		})
	}
}

func TestCreateProviderComponentsScenarios(t *testing.T) {
	const bootstrapKubeconfig = "bootstrap"
	const targetKubeconfig = "target"
	testCases := []struct {
		name          string
		pcStore       mockProviderComponentsStore
		expectedError string
	}{
		{"success", mockProviderComponentsStore{SaveErr: nil}, ""},
		{"error when saving", mockProviderComponentsStore{SaveErr: fmt.Errorf("pcstore save error")}, "unable to save provider components to target cluster: error saving provider components: pcstore save error"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			kubeconfigOut := newTempFile(t)
			defer os.Remove(kubeconfigOut)
			p := &testClusterProvisioner{
				kubeconfig: bootstrapKubeconfig,
			}
			pd := &testProviderDeployer{}
			pd.kubeconfig = targetKubeconfig
			f := newTestClusterClientFactory()
			f.clusterClients[bootstrapKubeconfig] = &testClusterClient{}
			f.clusterClients[targetKubeconfig] = &testClusterClient{}

			inputCluster := &clusterv1.Cluster{}
			inputCluster.Name = "test-cluster"
			inputMachines := generateMachines()
			pcFactory := mockProviderComponentsStoreFactory{NewFromCoreclientsetPCStore: &tc.pcStore}
			providerComponentsYaml := "-yaml\ndefinition"
			addonsYaml := "-yaml\ndefinition"
			d := New(p, f, providerComponentsYaml, addonsYaml, false)
			err := d.Create(inputCluster, inputMachines, pd, kubeconfigOut, &pcFactory)
			if err == nil && tc.expectedError != "" {
				t.Fatalf("error mismatch: got '%v', want '%v'", err, tc.expectedError)
			}
			if err != nil && err.Error() != tc.expectedError {
				t.Errorf("error message mismatch: got '%v', want '%v'", err, tc.expectedError)
			}
			if tc.pcStore.SaveCapturedProviderComponents != providerComponentsYaml {
				t.Errorf("provider components mismatch: got '%v', want '%v'", tc.pcStore.SaveCapturedProviderComponents, providerComponentsYaml)
			}
		})
	}
}

func TestExtractMasterMachine(t *testing.T) {
	const singleMasterName = "test-master"
	multpleMasterNames := []string{"test-master-1", "test-master-2"}
	const singleNodeName = "test-node"
	multipleNodeNames := []string{"test-node-1", "test-node-2", "test-node-3"}

	testCases := []struct {
		name            string
		inputMachines   []*clusterv1.Machine
		expectedMasters *clusterv1.Machine
		expectedNodes   []*clusterv1.Machine
		expectedError   error
	}{
		{
			name:            "success_1_master_1_node",
			inputMachines:   generateMachines(),
			expectedMasters: generateTestMasterMachine(singleMasterName),
			expectedNodes:   generateTestNodeMachines([]string{singleNodeName}),
			expectedError:   nil,
		},
		{
			name:            "success_1_master_multiple_nodes",
			inputMachines:   generateValidExtractMasterMachineInput([]string{singleMasterName}, multipleNodeNames),
			expectedMasters: generateTestMasterMachine(singleMasterName),
			expectedNodes:   generateTestNodeMachines(multipleNodeNames),
			expectedError:   nil,
		},
		{
			name:            "fail_more_than_1_master_not_allowed",
			inputMachines:   generateInvalidExtractMasterMachine(multpleMasterNames, multipleNodeNames),
			expectedMasters: nil,
			expectedNodes:   nil,
			expectedError:   fmt.Errorf("expected one master, got: 2"),
		},
		{
			name:            "fail_0_master_not_allowed",
			inputMachines:   generateTestNodeMachines(multipleNodeNames),
			expectedMasters: nil,
			expectedNodes:   nil,
			expectedError:   fmt.Errorf("expected one master, got: 0"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualMasters, actualNodes, actualError := extractMasterMachine(tc.inputMachines)

			if tc.expectedError == nil && actualError != nil {
				t.Fatalf("%s: extractMasterMachine(%q): gotError %q; wantError [nil]", tc.name, tc.inputMachines, actualError)
			}

			if tc.expectedError != nil && tc.expectedError.Error() != actualError.Error() {
				t.Fatalf("%s: extractMasterMachine(%q): gotError %q; wantError %q", tc.name, tc.inputMachines, actualError, tc.expectedError)
			}

			if (tc.expectedMasters == nil && actualMasters != nil) ||
				(tc.expectedMasters != nil && actualMasters == nil) {
				t.Fatalf("%s: extractMasterMachine(%q): gotMasters = %q; wantMasters = %q", tc.name, tc.inputMachines, actualMasters, tc.expectedMasters)
			}

			if len(tc.expectedNodes) != len(actualNodes) {
				t.Fatalf("%s: extractMasterMachine(%q): gotNodes = %q; wantNodes = %q", tc.name, tc.inputMachines, actualNodes, tc.expectedNodes)
			}
		})
	}
}

func TestDeleteCleanupExternalCluster(t *testing.T) {
	const bootstrapKubeconfig = "bootstrap"
	const targetKubeconfig = "target"

	testCases := []struct {
		name                   string
		cleanupExternalCluster bool
		provisionExternalErr   error
		bootstrapClient        *testClusterClient
		targetClient           *testClusterClient
		expectedErrorMessage   string
	}{
		{"success with cleanup", true, nil, &testClusterClient{}, &testClusterClient{}, ""},
		{"success without cleanup", false, nil, &testClusterClient{}, &testClusterClient{}, ""},
		{"error with cleanup", true, nil, &testClusterClient{}, &testClusterClient{GetMachineSetObjectsErr: fmt.Errorf("get machine sets error")}, "unable to copy objects from target to bootstrap cluster: get machine sets error"},
		{"error without cleanup", true, nil, &testClusterClient{}, &testClusterClient{GetMachineSetObjectsErr: fmt.Errorf("get machine sets error")}, "unable to copy objects from target to bootstrap cluster: get machine sets error"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			kubeconfigOut := newTempFile(t)
			defer os.Remove(kubeconfigOut)
			p := &testClusterProvisioner{err: tc.provisionExternalErr, kubeconfig: bootstrapKubeconfig}
			f := newTestClusterClientFactory()
			f.clusterClients[bootstrapKubeconfig] = tc.bootstrapClient
			f.clusterClients[targetKubeconfig] = tc.targetClient
			d := New(p, f, "", "", tc.cleanupExternalCluster)
			err := d.Delete(tc.targetClient)
			if err != nil || tc.expectedErrorMessage != "" {
				if err == nil {
					t.Errorf("expected error")
				} else if err.Error() != tc.expectedErrorMessage {
					t.Errorf("Unexpected error: got '%v', want: '%v'", err, tc.expectedErrorMessage)
				}
			}
			if !tc.cleanupExternalCluster != p.clusterExists {
				t.Errorf("cluster existence mismatch: got: '%v', want: '%v'", p.clusterExists, !tc.cleanupExternalCluster)
			}
		})
	}
}

func TestDeleteBasicScenarios(t *testing.T) {
	const bootstrapKubeconfig = "bootstrap"
	const targetKubeconfig = "target"

	testCases := []struct {
		name                 string
		provisionExternalErr error
		NewCoreClientsetErr  error
		bootstrapClient      *testClusterClient
		targetClient         *testClusterClient
		expectedErrorMessage string
	}{
		{"success", nil, nil, &testClusterClient{}, &testClusterClient{}, ""},
		{"error creating core client", nil, fmt.Errorf("error creating core client"), &testClusterClient{}, &testClusterClient{}, "could not create bootstrap cluster: unable to create bootstrap client: error creating core client"},
		{"fail provision bootstrap cluster", fmt.Errorf("minikube error"), nil, &testClusterClient{}, &testClusterClient{}, "could not create bootstrap cluster: could not create bootstrap control plane: minikube error"},
		{"fail apply yaml to bootstrap cluster", nil, nil, &testClusterClient{ApplyErr: fmt.Errorf("yaml apply error")}, &testClusterClient{}, "unable to apply cluster api stack to bootstrap cluster: unable to apply cluster apiserver: unable to apply apiserver yaml: yaml apply error"},
		{"fail delete provider components should succeed", nil, nil, &testClusterClient{}, &testClusterClient{DeleteErr: fmt.Errorf("kubectl delete error")}, ""},
		{"error listing machines", nil, nil, &testClusterClient{}, &testClusterClient{GetMachineObjectsErr: fmt.Errorf("get machines error")}, "unable to copy objects from target to bootstrap cluster: get machines error"},
		{"error listing machine sets", nil, nil, &testClusterClient{}, &testClusterClient{GetMachineSetObjectsErr: fmt.Errorf("get machine sets error")}, "unable to copy objects from target to bootstrap cluster: get machine sets error"},
		{"error listing machine deployments", nil, nil, &testClusterClient{}, &testClusterClient{GetMachineDeploymentObjectsErr: fmt.Errorf("get machine deployments error")}, "unable to copy objects from target to bootstrap cluster: get machine deployments error"},
		{"error listing clusters", nil, nil, &testClusterClient{}, &testClusterClient{GetClusterObjectsErr: fmt.Errorf("get clusters error")}, "unable to copy objects from target to bootstrap cluster: get clusters error"},
		{"error creating machines", nil, nil, &testClusterClient{CreateMachineObjectsErr: fmt.Errorf("create machines error")}, &testClusterClient{machines: generateMachines()}, "unable to copy objects from target to bootstrap cluster: error moving Machine 'test-master': create machines error"},
		{"error creating machine sets", nil, nil, &testClusterClient{CreateMachineSetObjectsErr: fmt.Errorf("create machine sets error")}, &testClusterClient{machineSets: newMachineSetsFixture()}, "unable to copy objects from target to bootstrap cluster: error moving MachineSet 'machine-set-name-1': create machine sets error"},
		{"error creating machine deployments", nil, nil, &testClusterClient{CreateMachineDeploymentsObjectsErr: fmt.Errorf("create machine deployments error")}, &testClusterClient{machineDeployments: newMachineDeploymentsFixture()}, "unable to copy objects from target to bootstrap cluster: error moving MachineDeployment 'machine-deployment-name-1': create machine deployments error"},
		{"error creating cluster", nil, nil, &testClusterClient{CreateClusterObjectErr: fmt.Errorf("create cluster error")}, &testClusterClient{clusters: newClustersFixture()}, "unable to copy objects from target to bootstrap cluster: error moving Cluster 'cluster-name-1': create cluster error"},
		{"error deleting machines", nil, nil, &testClusterClient{DeleteMachineObjectsErr: fmt.Errorf("delete machines error")}, &testClusterClient{}, "unable to finish deleting objects in bootstrap cluster, resources may have been leaked: error(s) encountered deleting objects from bootstrap cluster: [error deleting machines: delete machines error]"},
		{"error deleting machine sets", nil, nil, &testClusterClient{DeleteMachineSetObjectsErr: fmt.Errorf("delete machine sets error")}, &testClusterClient{}, "unable to finish deleting objects in bootstrap cluster, resources may have been leaked: error(s) encountered deleting objects from bootstrap cluster: [error deleting machine sets: delete machine sets error]"},
		{"error deleting machine deployments", nil, nil, &testClusterClient{DeleteMachineDeploymentsObjectsErr: fmt.Errorf("delete machine deployments error")}, &testClusterClient{}, "unable to finish deleting objects in bootstrap cluster, resources may have been leaked: error(s) encountered deleting objects from bootstrap cluster: [error deleting machine deployments: delete machine deployments error]"},
		{"error deleting clusters", nil, nil, &testClusterClient{DeleteClusterObjectsErr: fmt.Errorf("delete clusters error")}, &testClusterClient{}, "unable to finish deleting objects in bootstrap cluster, resources may have been leaked: error(s) encountered deleting objects from bootstrap cluster: [error deleting clusters: delete clusters error]"},
		{"error deleting machines and clusters", nil, nil, &testClusterClient{DeleteMachineObjectsErr: fmt.Errorf("delete machines error"), DeleteClusterObjectsErr: fmt.Errorf("delete clusters error")}, &testClusterClient{}, "unable to finish deleting objects in bootstrap cluster, resources may have been leaked: error(s) encountered deleting objects from bootstrap cluster: [error deleting machines: delete machines error, error deleting clusters: delete clusters error]"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			kubeconfigOut := newTempFile(t)
			defer os.Remove(kubeconfigOut)
			p := &testClusterProvisioner{err: tc.provisionExternalErr, kubeconfig: bootstrapKubeconfig}
			f := newTestClusterClientFactory()
			f.clusterClients[bootstrapKubeconfig] = tc.bootstrapClient
			f.clusterClients[targetKubeconfig] = tc.targetClient
			f.ClusterClientErr = tc.NewCoreClientsetErr
			d := New(p, f, "", "", true)
			err := d.Delete(tc.targetClient)
			if err != nil || tc.expectedErrorMessage != "" {
				if err == nil {
					t.Errorf("expected error")
				} else if err.Error() != tc.expectedErrorMessage {
					t.Errorf("Unexpected error: got '%v', want: '%v'", err, tc.expectedErrorMessage)
				}
			}
		})
	}
}

func generateTestMasterMachine(name string) *clusterv1.Machine {
	return &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: clusterv1.MachineSpec{
			Versions: clusterv1.MachineVersionInfo{
				ControlPlane: "1.10.1",
			},
		},
	}
}

func generateTestNodeMachine(name string) *clusterv1.Machine {
	return &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func generateTestMasterMachines(masterNames []string) []*clusterv1.Machine {
	var masters []*clusterv1.Machine
	for _, mn := range masterNames {
		masters = append(masters, generateTestMasterMachine(mn))
	}
	return masters
}

func generateTestNodeMachines(nodeNames []string) []*clusterv1.Machine {
	var nodes []*clusterv1.Machine
	for _, nn := range nodeNames {
		nodes = append(nodes, generateTestNodeMachine(nn))
	}
	return nodes
}

func generateInvalidExtractMasterMachine(masterNames, nodeNames []string) []*clusterv1.Machine {
	masters := generateTestMasterMachines(masterNames)
	nodes := generateTestNodeMachines(nodeNames)

	return append(masters, nodes...)
}

func generateValidExtractMasterMachineInput(masterNames, nodeNames []string) []*clusterv1.Machine {
	masters := generateTestMasterMachines(masterNames)
	nodes := generateTestNodeMachines(nodeNames)

	return append(masters, nodes...)
}

func generateMachines() []*clusterv1.Machine {
	master := generateTestMasterMachine("test-master")
	node := generateTestNodeMachine("test-node")
	return []*clusterv1.Machine{master, node}
}

func newMachineSetsFixture() []*clusterv1.MachineSet {
	return []*clusterv1.MachineSet{
		&clusterv1.MachineSet{ObjectMeta: metav1.ObjectMeta{Name: "machine-set-name-1"}},
		&clusterv1.MachineSet{ObjectMeta: metav1.ObjectMeta{Name: "machine-set-name-2"}},
	}
}

func newMachineDeploymentsFixture() []*clusterv1.MachineDeployment {
	return []*clusterv1.MachineDeployment{
		&clusterv1.MachineDeployment{ObjectMeta: metav1.ObjectMeta{Name: "machine-deployment-name-1"}},
		&clusterv1.MachineDeployment{ObjectMeta: metav1.ObjectMeta{Name: "machine-deployment-name-2"}},
	}
}

func newClustersFixture() []*clusterv1.Cluster {
	return []*clusterv1.Cluster{
		&clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-name-1"}},
		&clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-name-2"}},
	}
}

func newTempFile(t *testing.T) string {
	kubeconfigOutFile, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatalf("could not provision temp file:%v", err)
	}
	kubeconfigOutFile.Close()
	return kubeconfigOutFile.Name()
}
