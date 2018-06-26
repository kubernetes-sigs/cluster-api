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

package clusterdeployer_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/cluster-api/clusterctl/clusterdeployer"
	clustercommon "sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
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
	NewFromCoreclientsetPCStore          clusterdeployer.ProviderComponentsStore
	NewFromCoreclientsetError            error
	NewFromCoreclientsetCapturedArgument *kubernetes.Clientset
}

func (m *mockProviderComponentsStoreFactory) NewFromCoreClientset(clientset *kubernetes.Clientset) (clusterdeployer.ProviderComponentsStore, error) {
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

func (f *testClusterClientFactory) NewClusterClientFromKubeconfig(kubeconfig string) (clusterdeployer.ClusterClient, error) {
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
	const externalKubeconfig = "external"
	const internalKubeconfig = "internal"

	var testcases = []struct {
		name                     string
		provisionExternalErr     error
		factoryClusterClientErr  error
		externalClient           *testClusterClient
		internalClient           *testClusterClient
		cleanupExternal          bool
		expectErr                bool
		expectExternalExists     bool
		expectExternalCreated    bool
		expectedInternalClusters int
		expectedInternalMachines int
	}{
		{
			name:                     "success",
			internalClient:           &testClusterClient{},
			externalClient:           &testClusterClient{},
			cleanupExternal:          true,
			expectExternalExists:     false,
			expectExternalCreated:    true,
			expectedInternalClusters: 1,
			expectedInternalMachines: 2,
		},
		{
			name:                     "success no cleaning external",
			internalClient:           &testClusterClient{},
			externalClient:           &testClusterClient{},
			cleanupExternal:          false,
			expectExternalExists:     true,
			expectExternalCreated:    true,
			expectedInternalClusters: 1,
			expectedInternalMachines: 2,
		},
		{
			name:                 "fail provision external cluster",
			internalClient:       &testClusterClient{},
			externalClient:       &testClusterClient{},
			provisionExternalErr: fmt.Errorf("Test failure"),
			expectErr:            true,
		},
		{
			name:                    "fail create clients",
			internalClient:          &testClusterClient{},
			externalClient:          &testClusterClient{},
			cleanupExternal:         true,
			expectExternalCreated:   true,
			factoryClusterClientErr: fmt.Errorf("Test failure"),
			expectErr:               true,
		},
		{
			name:                  "fail apply yaml to external cluster",
			internalClient:        &testClusterClient{},
			externalClient:        &testClusterClient{ApplyErr: fmt.Errorf("Test failure")},
			cleanupExternal:       true,
			expectExternalCreated: true,
			expectErr:             true,
		},
		{
			name:                  "fail waiting for api ready on external cluster",
			internalClient:        &testClusterClient{},
			externalClient:        &testClusterClient{WaitForClusterV1alpha1ReadyErr: fmt.Errorf("Test failure")},
			cleanupExternal:       true,
			expectExternalCreated: true,
			expectErr:             true,
		},
		{
			name:                  "fail getting external cluster objects",
			internalClient:        &testClusterClient{},
			externalClient:        &testClusterClient{GetClusterObjectsErr: fmt.Errorf("Test failure")},
			cleanupExternal:       true,
			expectExternalCreated: true,
			expectErr:             true,
		},
		{
			name:                  "fail getting external machine objects",
			internalClient:        &testClusterClient{},
			externalClient:        &testClusterClient{GetMachineObjectsErr: fmt.Errorf("Test failure")},
			cleanupExternal:       true,
			expectExternalCreated: true,
			expectErr:             true,
		},
		{
			name:                  "fail create cluster",
			internalClient:        &testClusterClient{},
			externalClient:        &testClusterClient{CreateClusterObjectErr: fmt.Errorf("Test failure")},
			cleanupExternal:       true,
			expectExternalCreated: true,
			expectErr:             true,
		},
		{
			name:                  "fail create master",
			internalClient:        &testClusterClient{},
			externalClient:        &testClusterClient{CreateMachineObjectsErr: fmt.Errorf("Test failure")},
			cleanupExternal:       true,
			expectExternalCreated: true,
			expectErr:             true,
		},
		{
			name:                  "fail update external cluster endpoint",
			internalClient:        &testClusterClient{},
			externalClient:        &testClusterClient{UpdateClusterObjectEndpointErr: fmt.Errorf("Test failure")},
			cleanupExternal:       true,
			expectExternalCreated: true,
			expectErr:             true,
		},
		{
			name:                  "fail apply yaml to internal cluster",
			internalClient:        &testClusterClient{ApplyErr: fmt.Errorf("Test failure")},
			externalClient:        &testClusterClient{},
			cleanupExternal:       true,
			expectExternalCreated: true,
			expectErr:             true,
		},
		{
			name:                  "fail wait for api ready on internal cluster",
			internalClient:        &testClusterClient{WaitForClusterV1alpha1ReadyErr: fmt.Errorf("Test failure")},
			externalClient:        &testClusterClient{},
			cleanupExternal:       true,
			expectExternalCreated: true,
			expectErr:             true,
		},
		{
			name:                  "fail  create internal cluster",
			internalClient:        &testClusterClient{CreateClusterObjectErr: fmt.Errorf("Test failure")},
			externalClient:        &testClusterClient{},
			cleanupExternal:       true,
			expectExternalCreated: true,
			expectErr:             true,
		},
		{
			name:                     "fail create nodes",
			internalClient:           &testClusterClient{CreateMachineObjectsErr: fmt.Errorf("Test failure")},
			externalClient:           &testClusterClient{},
			cleanupExternal:          true,
			expectExternalCreated:    true,
			expectedInternalClusters: 1,
			expectErr:                true,
		},
		{
			name:                     "fail update cluster endpoint internal",
			internalClient:           &testClusterClient{UpdateClusterObjectEndpointErr: fmt.Errorf("Test failure")},
			externalClient:           &testClusterClient{},
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
				kubeconfig: externalKubeconfig,
			}
			pd := &testProviderDeployer{}
			pd.kubeconfig = internalKubeconfig
			f := newTestClusterClientFactory()
			f.clusterClients[externalKubeconfig] = testcase.externalClient
			f.clusterClients[internalKubeconfig] = testcase.internalClient
			f.ClusterClientErr = testcase.factoryClusterClientErr

			// Create
			inputCluster := &clusterv1.Cluster{}
			inputCluster.Name = "test-cluster"
			inputMachines := generateMachines()
			pcStore := mockProviderComponentsStore{}
			pcFactory := mockProviderComponentsStoreFactory{NewFromCoreclientsetPCStore: &pcStore}
			d := clusterdeployer.New(p, f, "", "", testcase.cleanupExternal)
			err := d.Create(inputCluster, inputMachines, pd, kubeconfigOut, &pcFactory)

			// Validate
			if (testcase.expectErr && err == nil) || (!testcase.expectErr && err != nil) {
				t.Fatalf("Unexpected returned error. Got: %v, Want Err: %v", err, testcase.expectErr)
			}
			if testcase.expectExternalExists != p.clusterExists {
				t.Errorf("Unexpected external cluster existance. Got: %v, Want: %v", p.clusterExists, testcase.expectExternalExists)
			}
			if testcase.expectExternalCreated != p.clusterCreated {
				t.Errorf("Unexpected external cluster provisioning. Got: %v, Want: %v", p.clusterCreated, testcase.expectExternalCreated)
			}
			if testcase.expectedInternalClusters != len(testcase.internalClient.clusters) {
				t.Fatalf("Unexpected cluster count. Got: %v, Want: %v", len(testcase.internalClient.clusters), testcase.expectedInternalClusters)
			}
			if testcase.expectedInternalClusters > 1 && inputCluster.Name != testcase.internalClient.clusters[0].Name {
				t.Errorf("Provisioned cluster has unexpeted name. Got: %v, Want: %v", testcase.internalClient.clusters[0].Name, inputCluster.Name)
			}

			if testcase.expectedInternalMachines != len(testcase.internalClient.machines) {
				t.Fatalf("Unexpected machine count. Got: %v, Want: %v", len(testcase.internalClient.machines), testcase.expectedInternalMachines)
			}
			if testcase.expectedInternalMachines == len(inputMachines) {
				for i := range inputMachines {
					if inputMachines[i].Name != testcase.internalClient.machines[i].Name {
						t.Fatalf("Unexpected machine name at %v. Got: %v, Want: %v", i, inputMachines[i].Name, testcase.internalClient.machines[i].Name)
					}
				}
			}
		})
	}
}

func TestCreateProviderComponentsScenarios(t *testing.T) {
	const externalKubeconfig = "external"
	const internalKubeconfig = "internal"
	testCases := []struct {
		name          string
		pcStore       mockProviderComponentsStore
		expectedError string
	}{
		{"success", mockProviderComponentsStore{SaveErr: nil}, ""},
		{"error when saving", mockProviderComponentsStore{SaveErr: fmt.Errorf("pcstore save error")}, "unable to save provider components to internal cluster: error saving provider components: pcstore save error"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			kubeconfigOut := newTempFile(t)
			defer os.Remove(kubeconfigOut)
			p := &testClusterProvisioner{
				kubeconfig: externalKubeconfig,
			}
			pd := &testProviderDeployer{}
			pd.kubeconfig = internalKubeconfig
			f := newTestClusterClientFactory()
			f.clusterClients[externalKubeconfig] = &testClusterClient{}
			f.clusterClients[internalKubeconfig] = &testClusterClient{}

			inputCluster := &clusterv1.Cluster{}
			inputCluster.Name = "test-cluster"
			inputMachines := generateMachines()
			pcFactory := mockProviderComponentsStoreFactory{NewFromCoreclientsetPCStore: &tc.pcStore}
			providerComponentsYaml := "-yaml\ndefinition"
			addonsYaml := "-yaml\ndefinition"
			d := clusterdeployer.New(p, f, providerComponentsYaml, addonsYaml, false)
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

func TestDeleteCleanupExternalCluster(t *testing.T) {
	const externalKubeconfig = "external"
	const internalKubeconfig = "internal"

	testCases := []struct {
		name                   string
		cleanupExternalCluster bool
		provisionExternalErr   error
		externalClient         *testClusterClient
		internalClient         *testClusterClient
		expectedErrorMessage   string
	}{
		{"success with cleanup", true, nil, &testClusterClient{}, &testClusterClient{}, ""},
		{"success without cleanup", false, nil, &testClusterClient{}, &testClusterClient{}, ""},
		{"error with cleanup", true, nil, &testClusterClient{}, &testClusterClient{GetMachineSetObjectsErr: fmt.Errorf("get machine sets error")}, "unable to copy objects from internal to external cluster: get machine sets error"},
		{"error without cleanup", true, nil, &testClusterClient{}, &testClusterClient{GetMachineSetObjectsErr: fmt.Errorf("get machine sets error")}, "unable to copy objects from internal to external cluster: get machine sets error"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			kubeconfigOut := newTempFile(t)
			defer os.Remove(kubeconfigOut)
			p := &testClusterProvisioner{err: tc.provisionExternalErr, kubeconfig: externalKubeconfig}
			f := newTestClusterClientFactory()
			f.clusterClients[externalKubeconfig] = tc.externalClient
			f.clusterClients[internalKubeconfig] = tc.internalClient
			d := clusterdeployer.New(p, f, "", "", tc.cleanupExternalCluster)
			err := d.Delete(tc.internalClient)
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
	const externalKubeconfig = "external"
	const internalKubeconfig = "internal"

	testCases := []struct {
		name                 string
		provisionExternalErr error
		NewCoreClientsetErr  error
		externalClient       *testClusterClient
		internalClient       *testClusterClient
		expectedErrorMessage string
	}{
		{"success", nil, nil, &testClusterClient{}, &testClusterClient{}, ""},
		{"error creating core client", nil, fmt.Errorf("error creating core client"), &testClusterClient{}, &testClusterClient{}, "could not create external cluster: unable to create external client: error creating core client"},
		{"fail provision external cluster", fmt.Errorf("minikube error"), nil, &testClusterClient{}, &testClusterClient{}, "could not create external cluster: could not create external control plane: minikube error"},
		{"fail apply yaml to external cluster", nil, nil, &testClusterClient{ApplyErr: fmt.Errorf("yaml apply error")}, &testClusterClient{}, "unable to apply cluster api stack to external cluster: unable to apply cluster apiserver: unable to apply apiserver yaml: yaml apply error"},
		{"fail delete provider components should succeed", nil, nil, &testClusterClient{}, &testClusterClient{DeleteErr: fmt.Errorf("kubectl delete error")}, ""},
		{"error listing machines", nil, nil, &testClusterClient{}, &testClusterClient{GetMachineObjectsErr: fmt.Errorf("get machines error")}, "unable to copy objects from internal to external cluster: get machines error"},
		{"error listing machine sets", nil, nil, &testClusterClient{}, &testClusterClient{GetMachineSetObjectsErr: fmt.Errorf("get machine sets error")}, "unable to copy objects from internal to external cluster: get machine sets error"},
		{"error listing machine deployments", nil, nil, &testClusterClient{}, &testClusterClient{GetMachineDeploymentObjectsErr: fmt.Errorf("get machine deployments error")}, "unable to copy objects from internal to external cluster: get machine deployments error"},
		{"error listing clusters", nil, nil, &testClusterClient{}, &testClusterClient{GetClusterObjectsErr: fmt.Errorf("get clusters error")}, "unable to copy objects from internal to external cluster: get clusters error"},
		{"error creating machines", nil, nil, &testClusterClient{CreateMachineObjectsErr: fmt.Errorf("create machines error")}, &testClusterClient{machines: generateMachines()}, "unable to copy objects from internal to external cluster: error moving Machine 'test-master': create machines error"},
		{"error creating machine sets", nil, nil, &testClusterClient{CreateMachineSetObjectsErr: fmt.Errorf("create machine sets error")}, &testClusterClient{machineSets: newMachineSetsFixture()}, "unable to copy objects from internal to external cluster: error moving MachineSet 'machine-set-name-1': create machine sets error"},
		{"error creating machine deployments", nil, nil, &testClusterClient{CreateMachineDeploymentsObjectsErr: fmt.Errorf("create machine deployments error")}, &testClusterClient{machineDeployments: newMachineDeploymentsFixture()}, "unable to copy objects from internal to external cluster: error moving MachineDeployment 'machine-deployment-name-1': create machine deployments error"},
		{"error creating cluster", nil, nil, &testClusterClient{CreateClusterObjectErr: fmt.Errorf("create cluster error")}, &testClusterClient{clusters: newClustersFixture()}, "unable to copy objects from internal to external cluster: error moving Cluster 'cluster-name-1': create cluster error"},
		{"error deleting machines", nil, nil, &testClusterClient{DeleteMachineObjectsErr: fmt.Errorf("delete machines error")}, &testClusterClient{}, "unable to finish deleting objects in external cluster, resources may have been leaked: error(s) encountered deleting objects from external cluster: [error deleting machines: delete machines error]"},
		{"error deleting machine sets", nil, nil, &testClusterClient{DeleteMachineSetObjectsErr: fmt.Errorf("delete machine sets error")}, &testClusterClient{}, "unable to finish deleting objects in external cluster, resources may have been leaked: error(s) encountered deleting objects from external cluster: [error deleting machine sets: delete machine sets error]"},
		{"error deleting machine deployments", nil, nil, &testClusterClient{DeleteMachineDeploymentsObjectsErr: fmt.Errorf("delete machine deployments error")}, &testClusterClient{}, "unable to finish deleting objects in external cluster, resources may have been leaked: error(s) encountered deleting objects from external cluster: [error deleting machine deployments: delete machine deployments error]"},
		{"error deleting clusters", nil, nil, &testClusterClient{DeleteClusterObjectsErr: fmt.Errorf("delete clusters error")}, &testClusterClient{}, "unable to finish deleting objects in external cluster, resources may have been leaked: error(s) encountered deleting objects from external cluster: [error deleting clusters: delete clusters error]"},
		{"error deleting machines and clusters", nil, nil, &testClusterClient{DeleteMachineObjectsErr: fmt.Errorf("delete machines error"), DeleteClusterObjectsErr: fmt.Errorf("delete clusters error")}, &testClusterClient{}, "unable to finish deleting objects in external cluster, resources may have been leaked: error(s) encountered deleting objects from external cluster: [error deleting machines: delete machines error, error deleting clusters: delete clusters error]"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			kubeconfigOut := newTempFile(t)
			defer os.Remove(kubeconfigOut)
			p := &testClusterProvisioner{err: tc.provisionExternalErr, kubeconfig: externalKubeconfig}
			f := newTestClusterClientFactory()
			f.clusterClients[externalKubeconfig] = tc.externalClient
			f.clusterClients[internalKubeconfig] = tc.internalClient
			f.ClusterClientErr = tc.NewCoreClientsetErr
			d := clusterdeployer.New(p, f, "", "", true)
			err := d.Delete(tc.internalClient)
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

func generateMachines() []*clusterv1.Machine {
	master := &clusterv1.Machine{}
	master.Name = "test-master"
	master.Spec.Roles = []clustercommon.MachineRole{clustercommon.MasterRole}
	node := &clusterv1.Machine{}
	node.Name = "test.Node"
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
