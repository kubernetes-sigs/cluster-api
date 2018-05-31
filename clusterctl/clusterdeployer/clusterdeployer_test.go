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
	"sigs.k8s.io/cluster-api/clusterctl/clusterdeployer"
	"testing"

	"io/ioutil"
	"os"
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

type testClusterClient struct {
	ApplyErr                       error
	WaitForClusterV1alpha1ReadyErr error
	GetClusterObjectsErr           error
	GetMachineObjectsErr           error
	CreateClusterObjectErr         error
	CreateMachineObjectsErr        error
	UpdateClusterObjectEndpointErr error
	CloseErr                       error

	clusters []*clusterv1.Cluster
	machines []*clusterv1.Machine
}

func (c *testClusterClient) Apply(string) error {
	return c.ApplyErr
}

func (c *testClusterClient) WaitForClusterV1alpha1Ready() error {
	return c.WaitForClusterV1alpha1ReadyErr
}

func (c *testClusterClient) GetClusterObjects() ([]*clusterv1.Cluster, error) {
	return c.clusters, c.GetClusterObjectsErr
}

func (c *testClusterClient) GetMachineObjects() ([]*clusterv1.Machine, error) {
	return c.machines, c.GetMachineObjectsErr
}

func (c *testClusterClient) CreateClusterObject(cluster *clusterv1.Cluster) error {
	if c.CreateClusterObjectErr != nil {
		return c.CreateClusterObjectErr
	}
	c.clusters = append(c.clusters, cluster)
	return nil
}
func (c *testClusterClient) CreateMachineObjects(machines []*clusterv1.Machine) error {
	if c.CreateMachineObjectsErr != nil {
		return c.CreateMachineObjectsErr
	}
	c.machines = append(c.machines, machines...)
	return nil
}
func (c *testClusterClient) UpdateClusterObjectEndpoint(string) error {
	return c.UpdateClusterObjectEndpointErr
}
func (c *testClusterClient) Close() error {
	return c.CloseErr
}

type testClusterClientFactory struct {
	ClusterClientErr error
	clients          map[string]*testClusterClient
}

func newTestClusterClientFactory() *testClusterClientFactory {
	return &testClusterClientFactory{
		clients: map[string]*testClusterClient{},
	}
}

func (f *testClusterClientFactory) ClusterClient(kubeconfig string) (clusterdeployer.ClusterClient, error) {
	if f.ClusterClientErr != nil {
		return nil, f.ClusterClientErr
	}
	return f.clients[kubeconfig], nil
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
			kubeconfigOutFile, err := ioutil.TempFile("", "")
			if err != nil {
				t.Fatalf("could not provision temp file:%v", err)
			}
			kubeconfigOutFile.Close()
			kubeconfigOut := kubeconfigOutFile.Name()
			defer os.Remove(kubeconfigOut)

			// Create provisioners & clients and hook them up
			p := &testClusterProvisioner{
				err:        testcase.provisionExternalErr,
				kubeconfig: externalKubeconfig,
			}
			pd := &testProviderDeployer{}
			pd.kubeconfig = internalKubeconfig
			f := newTestClusterClientFactory()
			f.clients[externalKubeconfig] = testcase.externalClient
			f.clients[internalKubeconfig] = testcase.internalClient
			f.ClusterClientErr = testcase.factoryClusterClientErr

			// Create
			inputCluster := &clusterv1.Cluster{}
			inputCluster.Name = "test-cluster"
			inputMachines := generateMachines()
			d := clusterdeployer.New(p, f, pd, "", kubeconfigOut, testcase.cleanupExternal)
			err = d.Create(inputCluster, inputMachines)

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

func generateMachines() []*clusterv1.Machine {
	master := &clusterv1.Machine{}
	master.Name = "test-master"
	master.Spec.Roles = []clustercommon.MachineRole{clustercommon.MasterRole}
	node := &clusterv1.Machine{}
	node.Name = "test.Node"
	return []*clusterv1.Machine{master, node}
}
