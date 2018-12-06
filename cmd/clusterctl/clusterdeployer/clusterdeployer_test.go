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

	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/clusterclient"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/provider"
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
	NewFromCoreclientsetPCStore          provider.ComponentsStore
	NewFromCoreclientsetError            error
	NewFromCoreclientsetCapturedArgument *kubernetes.Clientset
}

func (m *mockProviderComponentsStoreFactory) NewFromCoreClientset(clientset *kubernetes.Clientset) (provider.ComponentsStore, error) {
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
	ApplyErr                                      error
	DeleteErr                                     error
	WaitForClusterV1alpha1ReadyErr                error
	GetClusterObjectsErr                          error
	GetClusterObjectErr                           error
	GetClusterObjectsInNamespaceErr               error
	GetMachineDeploymentObjectsErr                error
	GetMachineDeploymentObjectsInNamespaceErr     error
	GetMachineSetObjectsErr                       error
	GetMachineSetObjectsInNamespaceErr            error
	GetMachineObjectsErr                          error
	GetMachineObjectsInNamespaceErr               error
	CreateClusterObjectErr                        error
	CreateMachineObjectsErr                       error
	CreateMachineSetObjectsErr                    error
	CreateMachineDeploymentsObjectsErr            error
	DeleteClusterObjectsErr                       error
	DeleteClusterObjectsInNamespaceErr            error
	DeleteMachineObjectsErr                       error
	DeleteMachineObjectsInNamespaceErr            error
	DeleteMachineSetObjectsErr                    error
	DeleteMachineSetObjectsInNamespaceErr         error
	DeleteMachineDeploymentsObjectsErr            error
	DeleteMachineDeploymentsObjectsInNamespaceErr error
	UpdateClusterObjectEndpointErr                error
	EnsureNamespaceErr                            error
	DeleteNamespaceErr                            error
	CloseErr                                      error

	clusters           map[string][]*clusterv1.Cluster
	machineDeployments map[string][]*clusterv1.MachineDeployment
	machineSets        map[string][]*clusterv1.MachineSet
	machines           map[string][]*clusterv1.Machine
	namespaces         []string
	contextNamespace   string
}

func (c *testClusterClient) Apply(string) error {
	return c.ApplyErr
}

func (c *testClusterClient) Delete(string) error {
	return c.DeleteErr
}

func (c *testClusterClient) GetContextNamespace() string {
	if c.contextNamespace == "" {
		return "foo"
	}
	return c.contextNamespace
}

func (c *testClusterClient) WaitForClusterV1alpha1Ready() error {
	return c.WaitForClusterV1alpha1ReadyErr
}

func (c *testClusterClient) GetClusterObjects() ([]*clusterv1.Cluster, error) {
	return c.clusters[metav1.NamespaceDefault], c.GetClusterObjectsErr
}

func (c *testClusterClient) GetClusterObject(clusterName, namespace string) (*clusterv1.Cluster, error) {
	if c.GetClusterObjectErr != nil {
		return nil, c.GetClusterObjectErr
	}
	var cluster *clusterv1.Cluster
	for _, nc := range c.clusters[namespace] {
		if nc.Name == clusterName {
			cluster = nc
			break
		}
	}
	return cluster, nil
}

func (c *testClusterClient) GetClusterObjectsInNamespace(namespace string) ([]*clusterv1.Cluster, error) {
	if c.GetClusterObjectsInNamespaceErr != nil {
		return nil, c.GetClusterObjectsInNamespaceErr
	}
	return c.clusters[namespace], nil
}

func (c *testClusterClient) GetMachineDeploymentObjects() ([]*clusterv1.MachineDeployment, error) {
	return c.machineDeployments[metav1.NamespaceDefault], c.GetMachineDeploymentObjectsErr
}

func (c *testClusterClient) GetMachineDeploymentObjectsInNamespace(namespace string) ([]*clusterv1.MachineDeployment, error) {
	return c.machineDeployments[namespace], c.GetMachineDeploymentObjectsInNamespaceErr
}

func (c *testClusterClient) GetMachineSetObjects() ([]*clusterv1.MachineSet, error) {
	return c.machineSets[metav1.NamespaceDefault], c.GetMachineSetObjectsErr
}

func (c *testClusterClient) GetMachineSetObjectsInNamespace(namespace string) ([]*clusterv1.MachineSet, error) {
	return c.machineSets[namespace], c.GetMachineSetObjectsInNamespaceErr
}

func (c *testClusterClient) GetMachineObjects() ([]*clusterv1.Machine, error) {
	return c.machines[metav1.NamespaceDefault], c.GetMachineObjectsErr
}

func (c *testClusterClient) GetMachineObjectsInNamespace(namespace string) ([]*clusterv1.Machine, error) {
	return c.machines[namespace], c.GetMachineObjectsInNamespaceErr
}

func (c *testClusterClient) CreateClusterObject(cluster *clusterv1.Cluster) error {
	if c.CreateClusterObjectErr == nil {
		if c.clusters == nil {
			c.clusters = make(map[string][]*clusterv1.Cluster)
		}
		c.clusters[cluster.Namespace] = append(c.clusters[cluster.Namespace], cluster)
		return nil
	}
	return c.CreateClusterObjectErr
}

func (c *testClusterClient) CreateMachineDeploymentObjects(deployments []*clusterv1.MachineDeployment, namespace string) error {
	if c.CreateMachineDeploymentsObjectsErr == nil {
		if c.machineDeployments == nil {
			c.machineDeployments = make(map[string][]*clusterv1.MachineDeployment)
		}
		c.machineDeployments[namespace] = append(c.machineDeployments[namespace], deployments...)
		return nil
	}
	return c.CreateMachineDeploymentsObjectsErr
}

func (c *testClusterClient) CreateMachineSetObjects(machineSets []*clusterv1.MachineSet, namespace string) error {
	if c.CreateMachineSetObjectsErr == nil {
		if c.machineSets == nil {
			c.machineSets = make(map[string][]*clusterv1.MachineSet)
		}
		c.machineSets[namespace] = append(c.machineSets[namespace], machineSets...)
		return nil
	}
	return c.CreateMachineSetObjectsErr
}

func (c *testClusterClient) CreateMachineObjects(machines []*clusterv1.Machine, namespace string) error {
	if c.CreateMachineObjectsErr == nil {
		if c.machines == nil {
			c.machines = make(map[string][]*clusterv1.Machine)
		}
		c.machines[namespace] = append(c.machines[namespace], machines...)
		return nil
	}
	return c.CreateMachineObjectsErr
}

func (c *testClusterClient) DeleteClusterObjectsInNamespace(ns string) error {
	if c.DeleteClusterObjectsInNamespaceErr == nil {
		delete(c.clusters, ns)
	}
	return c.DeleteClusterObjectsInNamespaceErr
}

func (c *testClusterClient) DeleteClusterObjects() error {
	return c.DeleteClusterObjectsErr
}

func (c *testClusterClient) DeleteMachineDeploymentObjectsInNamespace(ns string) error {
	if c.DeleteMachineDeploymentsObjectsInNamespaceErr == nil {
		delete(c.machineDeployments, ns)
	}
	return c.DeleteMachineDeploymentsObjectsInNamespaceErr
}

func (c *testClusterClient) DeleteMachineDeploymentObjects() error {
	return c.DeleteMachineDeploymentsObjectsErr
}

func (c *testClusterClient) DeleteMachineSetObjectsInNamespace(ns string) error {
	if c.DeleteMachineSetObjectsInNamespaceErr == nil {
		delete(c.machineSets, ns)
	}
	return c.DeleteMachineSetObjectsInNamespaceErr
}

func (c *testClusterClient) DeleteMachineSetObjects() error {
	return c.DeleteMachineSetObjectsErr
}

func (c *testClusterClient) DeleteMachineObjectsInNamespace(ns string) error {
	if c.DeleteMachineObjectsInNamespaceErr == nil {
		delete(c.machines, ns)
	}
	return c.DeleteMachineObjectsInNamespaceErr
}

func (c *testClusterClient) DeleteMachineObjects() error {
	return c.DeleteMachineObjectsErr
}

func (c *testClusterClient) UpdateClusterObjectEndpoint(string, string, string) error {
	return c.UpdateClusterObjectEndpointErr
}
func (c *testClusterClient) Close() error {
	return c.CloseErr
}

func (c *testClusterClient) EnsureNamespace(nsName string) error {
	if len(c.namespaces) == 0 {
		c.namespaces = append(c.namespaces, nsName)
	}
	if exists := contains(c.namespaces, nsName); !exists {
		c.namespaces = append(c.namespaces, nsName)
	}
	return c.EnsureNamespaceErr
}

func (c *testClusterClient) DeleteNamespace(namespaceName string) error {
	if namespaceName == apiv1.NamespaceDefault {
		return nil
	}

	ns := make([]string, 0, len(c.namespaces))
	for _, n := range c.namespaces {
		if n == namespaceName {
			continue
		}
		ns = append(ns, n)
	}
	c.namespaces = ns

	return c.DeleteNamespaceErr
}

func contains(s []string, e string) bool {
	exists := false
	for _, existingNs := range s {
		if existingNs == e {
			exists = true
			break
		}
	}
	return exists
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

func (f *testClusterClientFactory) NewClientFromKubeconfig(kubeconfig string) (clusterclient.Client, error) {
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

func TestClusterCreate(t *testing.T) {
	const bootstrapKubeconfig = "bootstrap"
	const targetKubeconfig = "target"

	testcases := []struct {
		name                                string
		provisionExternalErr                error
		factoryClusterClientErr             error
		bootstrapClient                     *testClusterClient
		targetClient                        *testClusterClient
		namespaceToInputCluster             map[string][]*clusterv1.Cluster
		cleanupExternal                     bool
		expectErr                           bool
		expectExternalExists                bool
		expectExternalCreated               bool
		namespaceToExpectedInternalMachines map[string]int
		expectedTotalInternalClustersCount  int // across all namespaces
	}{
		{
			name:                                "success one cluster one namespace",
			bootstrapClient:                     &testClusterClient{},
			targetClient:                        &testClusterClient{},
			cleanupExternal:                     true,
			expectExternalExists:                false,
			expectExternalCreated:               true,
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster: map[string][]*clusterv1.Cluster{
				"foo": getClustersForNamespace("foo", 1),
			},
			expectedTotalInternalClustersCount: 1,
		},
		{
			name:                                "success 1 clusters per namespace with 3 namespaces",
			bootstrapClient:                     &testClusterClient{},
			targetClient:                        &testClusterClient{},
			cleanupExternal:                     true,
			expectExternalExists:                false,
			expectExternalCreated:               true,
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster: map[string][]*clusterv1.Cluster{
				"foo": getClustersForNamespace("foo", 1),
				"bar": getClustersForNamespace("bar", 1),
				"baz": getClustersForNamespace("baz", 1),
			},
			expectedTotalInternalClustersCount: 3,
		},
		{
			name:                                "success no cleaning bootstrap",
			targetClient:                        &testClusterClient{},
			bootstrapClient:                     &testClusterClient{},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			cleanupExternal:                     false,
			expectExternalExists:                true,
			expectExternalCreated:               true,
			expectedTotalInternalClustersCount:  1,
		},
		{
			name:                                "success create cluster with \"\" namespace and bootstrapClientContext namespace",
			bootstrapClient:                     &testClusterClient{contextNamespace: "foo"},
			targetClient:                        &testClusterClient{},
			cleanupExternal:                     true,
			expectExternalExists:                false,
			expectExternalCreated:               true,
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster: map[string][]*clusterv1.Cluster{
				"": getClustersForNamespace("", 1),
			},
			expectedTotalInternalClustersCount: 1,
		},
		{
			name:                                "success cluster with \"\" namespace and \"\" bootstrapClientContext namespace",
			bootstrapClient:                     &testClusterClient{},
			targetClient:                        &testClusterClient{},
			cleanupExternal:                     true,
			expectExternalExists:                false,
			expectExternalCreated:               true,
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster: map[string][]*clusterv1.Cluster{
				"": getClustersForNamespace("", 1),
			},
			expectedTotalInternalClustersCount: 1,
		},
		{
			name:                                "fail ensureNamespace in bootstrap cluster",
			targetClient:                        &testClusterClient{},
			bootstrapClient:                     &testClusterClient{EnsureNamespaceErr: errors.New("Test failure")},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{"foo": getClustersForNamespace("foo", 3)},
			expectErr:                           true,
			cleanupExternal:                     true,
			expectExternalExists:                false,
			expectExternalCreated:               true,
		},
		{
			name:                                "fail ensureNamespace in target cluster",
			targetClient:                        &testClusterClient{EnsureNamespaceErr: errors.New("Test failure")},
			bootstrapClient:                     &testClusterClient{},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{"foo": getClustersForNamespace("foo", 3)},
			expectErr:                           true,
			cleanupExternal:                     true,
			expectExternalExists:                false,
			expectExternalCreated:               true,
		},
		{
			name:                                "fail provision multiple clusters in a namespace",
			targetClient:                        &testClusterClient{},
			bootstrapClient:                     &testClusterClient{},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{"foo": getClustersForNamespace("foo", 3)},
			expectErr:                           true,
			cleanupExternal:                     true,
			expectExternalExists:                false,
			expectExternalCreated:               true,
		},
		{
			name:                                "fail provision bootstrap cluster",
			targetClient:                        &testClusterClient{},
			bootstrapClient:                     &testClusterClient{},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			provisionExternalErr:                errors.New("Test failure"),
			expectErr:                           true,
		},
		{
			name:                                "fail provision bootstrap cluster",
			targetClient:                        &testClusterClient{},
			bootstrapClient:                     &testClusterClient{},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			provisionExternalErr:                errors.New("Test failure"),
			expectErr:                           true,
		},
		{
			name:                                "fail create clients",
			targetClient:                        &testClusterClient{},
			bootstrapClient:                     &testClusterClient{},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			cleanupExternal:                     true,
			expectExternalCreated:               true,
			factoryClusterClientErr:             errors.New("Test failure"),
			expectErr:                           true,
		},
		{
			name:                                "fail apply yaml to bootstrap cluster",
			targetClient:                        &testClusterClient{},
			bootstrapClient:                     &testClusterClient{ApplyErr: errors.New("Test failure")},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			cleanupExternal:                     true,
			expectExternalCreated:               true,
			expectErr:                           true,
		},
		{
			name:                                "fail waiting for api ready on bootstrap cluster",
			targetClient:                        &testClusterClient{},
			bootstrapClient:                     &testClusterClient{WaitForClusterV1alpha1ReadyErr: errors.New("Test failure")},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			cleanupExternal:                     true,
			expectExternalCreated:               true,
			expectErr:                           true,
		},
		{
			name:                                "fail getting bootstrap cluster objects",
			targetClient:                        &testClusterClient{},
			bootstrapClient:                     &testClusterClient{GetClusterObjectsInNamespaceErr: errors.New("Test failure")},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			cleanupExternal:                     true,
			expectExternalCreated:               true,
			expectErr:                           true,
		},
		{
			name:                                "fail getting bootstrap machine objects",
			targetClient:                        &testClusterClient{},
			bootstrapClient:                     &testClusterClient{GetMachineObjectsInNamespaceErr: errors.New("Test failure")},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			cleanupExternal:                     true,
			expectExternalCreated:               true,
			expectErr:                           true,
		},
		{
			name:                                "fail create cluster",
			targetClient:                        &testClusterClient{},
			bootstrapClient:                     &testClusterClient{CreateClusterObjectErr: errors.New("Test failure")},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			cleanupExternal:                     true,
			expectExternalCreated:               true,
			expectErr:                           true,
		},
		{
			name:                                "fail create control plane",
			targetClient:                        &testClusterClient{},
			bootstrapClient:                     &testClusterClient{CreateMachineObjectsErr: errors.New("Test failure")},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			cleanupExternal:                     true,
			expectExternalCreated:               true,
			expectErr:                           true,
		},
		{
			name:                                "fail update bootstrap cluster endpoint",
			targetClient:                        &testClusterClient{},
			bootstrapClient:                     &testClusterClient{UpdateClusterObjectEndpointErr: errors.New("Test failure")},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			cleanupExternal:                     true,
			expectExternalCreated:               true,
			expectErr:                           true,
		},
		{
			name:                                "fail apply yaml to target cluster",
			targetClient:                        &testClusterClient{ApplyErr: errors.New("Test failure")},
			bootstrapClient:                     &testClusterClient{},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			cleanupExternal:                     true,
			expectExternalCreated:               true,
			expectErr:                           true,
		},
		{
			name:                                "fail wait for api ready on target cluster",
			targetClient:                        &testClusterClient{WaitForClusterV1alpha1ReadyErr: errors.New("Test failure")},
			bootstrapClient:                     &testClusterClient{},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			cleanupExternal:                     true,
			expectExternalCreated:               true,
			expectErr:                           true,
		},
		{
			name:                                "fail  create target cluster",
			targetClient:                        &testClusterClient{CreateClusterObjectErr: errors.New("Test failure")},
			bootstrapClient:                     &testClusterClient{},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			cleanupExternal:                     true,
			expectExternalCreated:               true,
			expectErr:                           true,
		},
		{
			name:                                "fail create nodes",
			targetClient:                        &testClusterClient{CreateMachineObjectsErr: errors.New("Test failure")},
			bootstrapClient:                     &testClusterClient{},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			cleanupExternal:                     true,
			expectExternalCreated:               true,
			expectErr:                           true,
		},
		{
			name:                                "fail update cluster endpoint target",
			targetClient:                        &testClusterClient{UpdateClusterObjectEndpointErr: errors.New("Test failure")},
			bootstrapClient:                     &testClusterClient{},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			cleanupExternal:                     true,
			expectExternalCreated:               true,
			expectErr:                           true,
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

			pcStore := mockProviderComponentsStore{}
			pcFactory := mockProviderComponentsStoreFactory{NewFromCoreclientsetPCStore: &pcStore}
			d := New(p, f, "", "", testcase.cleanupExternal)
			inputMachines := generateMachines()

			for namespace, inputClusters := range testcase.namespaceToInputCluster {
				ns := namespace
				if ns == "" {
					ns = testcase.bootstrapClient.GetContextNamespace()
				}

				var err error
				for _, inputCluster := range inputClusters {
					inputCluster.Name = fmt.Sprintf("%s-cluster", ns)
					err = d.Create(inputCluster, inputMachines, pd, kubeconfigOut, &pcFactory)
					if err != nil {
						break
					}
				}
				if (testcase.expectErr && err == nil) || (!testcase.expectErr && err != nil) {
					t.Fatalf("Unexpected error returned. Got: %v, Want Err: %v", err, testcase.expectErr)
				}
				if testcase.expectExternalExists != p.clusterExists {
					t.Errorf("Unexpected bootstrap cluster existence. Got: %v, Want: %v", p.clusterExists, testcase.expectExternalExists)
				}
				if testcase.expectExternalCreated != p.clusterCreated {
					t.Errorf("Unexpected bootstrap cluster provisioning. Got: %v, Want: %v", p.clusterCreated, testcase.expectExternalCreated)
				}
				testcase.namespaceToExpectedInternalMachines[ns] = len(inputMachines)
			}

			if !testcase.expectErr {
				// Validate namespaces
				for namespace := range testcase.namespaceToInputCluster {
					ns := namespace
					if ns == "" {
						ns = testcase.bootstrapClient.GetContextNamespace()
					}
					if len(testcase.targetClient.clusters[ns]) != 1 {
						t.Fatalf("Unexpected cluster count in namespace %q. Got: %d, Want: %d", ns, len(testcase.targetClient.clusters[ns]), 1)
					}

					expectedClusterName := fmt.Sprintf("%s-cluster", ns)
					if testcase.targetClient.clusters[ns][0].Name != expectedClusterName {
						t.Fatalf("Unexpected cluster name in namespace %q. Got: %q, Want: %q", ns, testcase.targetClient.clusters[ns][0].Name, expectedClusterName)
					}

					if len(testcase.targetClient.machines[ns]) != testcase.namespaceToExpectedInternalMachines[ns] {
						t.Fatalf("Unexpected machine count in namespace %q. Got: %d, Want: %d", ns, len(testcase.targetClient.machines[ns]), testcase.namespaceToExpectedInternalMachines[ns])
					}

					for i := range inputMachines {
						if inputMachines[i].Name != testcase.targetClient.machines[ns][i].Name {
							t.Fatalf("Unexpected machine name at %v in namespace %q. Got: %v, Want: %v", i, ns, inputMachines[i].Name, testcase.targetClient.machines[ns][i].Name)
						}
					}

					if !contains(testcase.targetClient.namespaces, ns) {
						t.Fatalf("Expected namespace %q in target namespace not found. Got: NotFound, Want: Found", ns)
					}

					if !contains(testcase.bootstrapClient.namespaces, ns) {
						t.Fatalf("Expected namespace %q in bootstrap namespace not found. Got: NotFound, Want: Found", ns)
					}
				}
				// Validate across all namespaces
				if len(testcase.targetClient.clusters) != testcase.expectedTotalInternalClustersCount {
					t.Fatalf("Unexpected cluster count across all namespaces. Got: %d, Want: %d", len(testcase.targetClient.clusters), testcase.expectedTotalInternalClustersCount)
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
		{"error when saving", mockProviderComponentsStore{SaveErr: errors.New("pcstore save error")}, "unable to save provider components to target cluster: error saving provider components: pcstore save error"},
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

func TestExtractControlPlaneMachine(t *testing.T) {
	const singleControlPlaneName = "test-control-plane"
	multipleControlPlaneNames := []string{"test-control-plane-1", "test-control-plane-2"}
	const singleNodeName = "test-node"
	multipleNodeNames := []string{"test-node-1", "test-node-2", "test-node-3"}

	testCases := []struct {
		name                 string
		inputMachines        []*clusterv1.Machine
		expectedControlPlane *clusterv1.Machine
		expectedNodes        []*clusterv1.Machine
		expectedError        error
	}{
		{
			name:                 "success_1_control_plane_1_node",
			inputMachines:        generateMachines(),
			expectedControlPlane: generateTestControlPlaneMachine(singleControlPlaneName),
			expectedNodes:        generateTestNodeMachines([]string{singleNodeName}),
			expectedError:        nil,
		},
		{
			name:                 "success_1_control_plane_multiple_nodes",
			inputMachines:        generateValidExtractControlPlaneMachineInput([]string{singleControlPlaneName}, multipleNodeNames),
			expectedControlPlane: generateTestControlPlaneMachine(singleControlPlaneName),
			expectedNodes:        generateTestNodeMachines(multipleNodeNames),
			expectedError:        nil,
		},
		{
			name:                 "fail_more_than_1_control_plane_not_allowed",
			inputMachines:        generateInvalidExtractControlPlaneMachine(multipleControlPlaneNames, multipleNodeNames),
			expectedControlPlane: nil,
			expectedNodes:        nil,
			expectedError:        errors.New("expected one control plane machine, got: 2"),
		},
		{
			name:                 "fail_0_control_plane_not_allowed",
			inputMachines:        generateTestNodeMachines(multipleNodeNames),
			expectedControlPlane: nil,
			expectedNodes:        nil,
			expectedError:        errors.New("expected one control plane machine, got: 0"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualControlPlane, actualNodes, actualError := clusterclient.ExtractControlPlaneMachine(tc.inputMachines)

			if tc.expectedError == nil && actualError != nil {
				t.Fatalf("%s: extractControlPlaneMachine(%q): gotError %q; wantError [nil]", tc.name, len(tc.inputMachines), actualError)
			}

			if tc.expectedError != nil && tc.expectedError.Error() != actualError.Error() {
				t.Fatalf("%s: extractControlPlaneMachine(%q): gotError %q; wantError %q", tc.name, len(tc.inputMachines), actualError, tc.expectedError)
			}

			if (tc.expectedControlPlane == nil && actualControlPlane != nil) ||
				(tc.expectedControlPlane != nil && actualControlPlane == nil) {
				t.Fatalf("%s: extractControlPlaneMachine(%q): gotControlPlane = %v; wantControlPlane = %v", tc.name, len(tc.inputMachines), actualControlPlane != nil, tc.expectedControlPlane != nil)
			}

			if len(tc.expectedNodes) != len(actualNodes) {
				t.Fatalf("%s: extractControlPlaneMachine(%q): gotNodes = %q; wantNodes = %q", tc.name, len(tc.inputMachines), len(actualNodes), len(tc.expectedNodes))
			}
		})
	}
}

func TestDeleteCleanupExternalCluster(t *testing.T) {
	const bootstrapKubeconfig = "bootstrap"
	const targetKubeconfig = "target"

	testCases := []struct {
		name                   string
		namespace              string
		cleanupExternalCluster bool
		provisionExternalErr   error
		bootstrapClient        *testClusterClient
		targetClient           *testClusterClient
		expectedErrorMessage   string
	}{
		{"success with cleanup", metav1.NamespaceDefault, true, nil, &testClusterClient{}, &testClusterClient{}, ""},
		{"success without cleanup", metav1.NamespaceDefault, false, nil, &testClusterClient{}, &testClusterClient{}, ""},
		{"error with cleanup", metav1.NamespaceDefault, true, nil, &testClusterClient{}, &testClusterClient{GetMachineSetObjectsInNamespaceErr: errors.New("get machine sets error")}, "unable to copy objects from target to bootstrap cluster: get machine sets error"},
		{"error without cleanup", metav1.NamespaceDefault, true, nil, &testClusterClient{}, &testClusterClient{GetMachineSetObjectsInNamespaceErr: errors.New("get machine sets error")}, "unable to copy objects from target to bootstrap cluster: get machine sets error"},
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
			err := d.Delete(tc.targetClient, tc.namespace)
			if err != nil || tc.expectedErrorMessage != "" {
				if err == nil {
					t.Errorf("expected error %q", tc.expectedErrorMessage)
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

func TestClusterDelete(t *testing.T) {
	const bootstrapKubeconfig = "bootstrap"
	const targetKubeconfig = "target"

	testCases := []struct {
		name                         string
		namespace                    string
		provisionExternalErr         error
		NewCoreClientsetErr          error
		bootstrapClient              *testClusterClient
		targetClient                 *testClusterClient
		expectedErrorMessage         string
		expectedExternalClusterCount int
		expectError                  bool
	}{
		{
			name:      "success delete 1/1 cluster, 0 clusters remaining",
			namespace: metav1.NamespaceDefault,
			bootstrapClient: &testClusterClient{
				clusters: map[string][]*clusterv1.Cluster{
					metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1),
				},
				machines: map[string][]*clusterv1.Machine{
					metav1.NamespaceDefault: generateMachines(),
				},
				machineSets: map[string][]*clusterv1.MachineSet{
					metav1.NamespaceDefault: newMachineSetsFixture(),
				},
				machineDeployments: map[string][]*clusterv1.MachineDeployment{
					metav1.NamespaceDefault: newMachineDeploymentsFixture(),
				},
			},
			targetClient: &testClusterClient{
				clusters: map[string][]*clusterv1.Cluster{
					metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1),
				},
				machines: map[string][]*clusterv1.Machine{
					metav1.NamespaceDefault: generateMachines(),
				},
				machineSets: map[string][]*clusterv1.MachineSet{
					metav1.NamespaceDefault: newMachineSetsFixture(),
				},
				machineDeployments: map[string][]*clusterv1.MachineDeployment{
					metav1.NamespaceDefault: newMachineDeploymentsFixture(),
				},
			},
			expectedExternalClusterCount: 0,
		},
		{
			name:      "success delete 1/3 clusters, 2 clusters remaining",
			namespace: "foo",
			bootstrapClient: &testClusterClient{
				clusters: map[string][]*clusterv1.Cluster{
					"foo": getClustersForNamespace("foo", 1),
					"bar": getClustersForNamespace("bar", 1),
					"baz": getClustersForNamespace("baz", 1),
				},
				machines: map[string][]*clusterv1.Machine{
					"foo": generateMachines(),
					"bar": generateMachines(),
					"baz": generateMachines(),
				},
				machineSets: map[string][]*clusterv1.MachineSet{
					"foo": newMachineSetsFixture(),
					"bar": newMachineSetsFixture(),
					"baz": newMachineSetsFixture(),
				},
				machineDeployments: map[string][]*clusterv1.MachineDeployment{
					"foo": newMachineDeploymentsFixture(),
					"bar": newMachineDeploymentsFixture(),
					"baz": newMachineDeploymentsFixture(),
				},
			},
			targetClient: &testClusterClient{
				clusters: map[string][]*clusterv1.Cluster{
					"foo": getClustersForNamespace("foo", 1),
					"bar": getClustersForNamespace("bar", 1),
					"baz": getClustersForNamespace("baz", 1),
				},
				machines: map[string][]*clusterv1.Machine{
					"foo": generateMachines(),
					"bar": generateMachines(),
					"baz": generateMachines(),
				},
				machineSets: map[string][]*clusterv1.MachineSet{
					"foo": newMachineSetsFixture(),
					"bar": newMachineSetsFixture(),
					"baz": newMachineSetsFixture(),
				},
				machineDeployments: map[string][]*clusterv1.MachineDeployment{
					"foo": newMachineDeploymentsFixture(),
					"bar": newMachineDeploymentsFixture(),
					"baz": newMachineDeploymentsFixture(),
				},
			},
			expectedExternalClusterCount: 2,
		},
		{
			name:                 "error creating core client",
			namespace:            metav1.NamespaceDefault,
			provisionExternalErr: nil,
			NewCoreClientsetErr:  errors.New("error creating core client"),
			bootstrapClient:      &testClusterClient{},
			targetClient:         &testClusterClient{},
			expectedErrorMessage: "could not create bootstrap cluster: unable to create bootstrap client: error creating core client",
		},
		{
			name:                 "fail provision bootstrap cluster",
			namespace:            metav1.NamespaceDefault,
			provisionExternalErr: errors.New("minikube error"),
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{},
			targetClient:         &testClusterClient{},
			expectedErrorMessage: "could not create bootstrap cluster: could not create bootstrap control plane: minikube error",
		},
		{
			name:                 "fail apply yaml to bootstrap cluster",
			namespace:            metav1.NamespaceDefault,
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{ApplyErr: errors.New("yaml apply error")},
			targetClient:         &testClusterClient{},
			expectedErrorMessage: "unable to apply cluster api stack to bootstrap cluster: unable to apply cluster api controllers: yaml apply error",
		},
		{
			name:                 "fail delete provider components should succeed",
			namespace:            metav1.NamespaceDefault,
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{},
			targetClient:         &testClusterClient{DeleteErr: errors.New("kubectl delete error")},
			expectedErrorMessage: "",
		},
		{
			name:                 "error listing machines",
			namespace:            metav1.NamespaceDefault,
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{},
			targetClient:         &testClusterClient{GetMachineObjectsInNamespaceErr: errors.New("get machines error")},
			expectedErrorMessage: "unable to copy objects from target to bootstrap cluster: get machines error",
		},
		{
			name:                 "error listing machine sets",
			namespace:            metav1.NamespaceDefault,
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{},
			targetClient:         &testClusterClient{GetMachineSetObjectsInNamespaceErr: errors.New("get machine sets error")},
			expectedErrorMessage: "unable to copy objects from target to bootstrap cluster: get machine sets error",
		},
		{
			name:                 "error listing machine deployments",
			namespace:            metav1.NamespaceDefault,
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{},
			targetClient:         &testClusterClient{GetMachineDeploymentObjectsInNamespaceErr: errors.New("get machine deployments error")},
			expectedErrorMessage: "unable to copy objects from target to bootstrap cluster: get machine deployments error",
		},
		{
			name:                 "error listing clusters",
			namespace:            metav1.NamespaceDefault,
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{},
			targetClient:         &testClusterClient{GetClusterObjectsInNamespaceErr: errors.New("get clusters error")},
			expectedErrorMessage: "unable to copy objects from target to bootstrap cluster: get clusters error",
		},
		{
			name:                 "error creating machines",
			namespace:            metav1.NamespaceDefault,
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{CreateMachineObjectsErr: errors.New("create machines error")},
			targetClient: &testClusterClient{
				machines: map[string][]*clusterv1.Machine{
					metav1.NamespaceDefault: generateMachines(),
				},
			},
			expectedErrorMessage: "unable to copy objects from target to bootstrap cluster: error moving Machine \"test-control-plane\": create machines error",
		},
		{
			name:                 "error creating machine sets",
			namespace:            metav1.NamespaceDefault,
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{CreateMachineSetObjectsErr: errors.New("create machine sets error")},
			targetClient: &testClusterClient{
				machineSets: map[string][]*clusterv1.MachineSet{
					metav1.NamespaceDefault: newMachineSetsFixture(),
				},
			},
			expectedErrorMessage: "unable to copy objects from target to bootstrap cluster: error moving MachineSet \"machine-set-name-1\": create machine sets error",
		},
		{
			name:                 "error creating machine deployments",
			namespace:            metav1.NamespaceDefault,
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{CreateMachineDeploymentsObjectsErr: errors.New("create machine deployments error")},
			targetClient: &testClusterClient{
				machineDeployments: map[string][]*clusterv1.MachineDeployment{
					metav1.NamespaceDefault: newMachineDeploymentsFixture(),
				}},
			expectedErrorMessage: "unable to copy objects from target to bootstrap cluster: error moving MachineDeployment \"machine-deployment-name-1\": create machine deployments error",
		},
		{
			name:                 "error creating cluster",
			namespace:            metav1.NamespaceDefault,
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{CreateClusterObjectErr: errors.New("create cluster error")},
			targetClient: &testClusterClient{
				clusters: map[string][]*clusterv1.Cluster{
					metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1),
				},
			},
			expectedErrorMessage: "unable to copy objects from target to bootstrap cluster: error moving Cluster \"default-cluster\": create cluster error",
		},

		{
			name:                 "error deleting machines",
			namespace:            metav1.NamespaceDefault,
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{DeleteMachineObjectsInNamespaceErr: errors.New("delete machines error")},
			targetClient:         &testClusterClient{},
			expectedErrorMessage: "unable to finish deleting objects in bootstrap cluster, resources may have been leaked: error(s) encountered deleting objects from bootstrap cluster: [error deleting machines: delete machines error]",
		},
		{
			name:                 "error deleting machine sets",
			namespace:            metav1.NamespaceDefault,
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{DeleteMachineSetObjectsInNamespaceErr: errors.New("delete machine sets error")},
			targetClient:         &testClusterClient{},
			expectedErrorMessage: "unable to finish deleting objects in bootstrap cluster, resources may have been leaked: error(s) encountered deleting objects from bootstrap cluster: [error deleting machine sets: delete machine sets error]",
		},
		{
			name:                 "error deleting machine deployments",
			namespace:            metav1.NamespaceDefault,
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{DeleteMachineDeploymentsObjectsInNamespaceErr: errors.New("delete machine deployments error")},
			targetClient:         &testClusterClient{},
			expectedErrorMessage: "unable to finish deleting objects in bootstrap cluster, resources may have been leaked: error(s) encountered deleting objects from bootstrap cluster: [error deleting machine deployments: delete machine deployments error]",
		},
		{
			name:                 "error deleting clusters",
			namespace:            metav1.NamespaceDefault,
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{DeleteClusterObjectsInNamespaceErr: errors.New("delete clusters error")},
			targetClient:         &testClusterClient{},
			expectedErrorMessage: "unable to finish deleting objects in bootstrap cluster, resources may have been leaked: error(s) encountered deleting objects from bootstrap cluster: [error deleting clusters: delete clusters error]",
		},
		{
			name:                 "error deleting machines and clusters",
			namespace:            metav1.NamespaceDefault,
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{DeleteMachineObjectsInNamespaceErr: errors.New("delete machines error"), DeleteClusterObjectsInNamespaceErr: errors.New("delete clusters error")},
			targetClient:         &testClusterClient{},
			expectedErrorMessage: "unable to finish deleting objects in bootstrap cluster, resources may have been leaked: error(s) encountered deleting objects from bootstrap cluster: [error deleting machines: delete machines error, error deleting clusters: delete clusters error]",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			kubeconfigOut := newTempFile(t)
			defer os.Remove(kubeconfigOut)
			p := &testClusterProvisioner{err: testCase.provisionExternalErr, kubeconfig: bootstrapKubeconfig}
			f := newTestClusterClientFactory()
			f.clusterClients[bootstrapKubeconfig] = testCase.bootstrapClient
			f.clusterClients[targetKubeconfig] = testCase.targetClient
			f.ClusterClientErr = testCase.NewCoreClientsetErr
			d := New(p, f, "", "", true)

			err := d.Delete(testCase.targetClient, testCase.namespace)
			if err != nil || testCase.expectedErrorMessage != "" {
				if err == nil {
					t.Errorf("expected error %q", testCase.expectedErrorMessage)
				} else if err.Error() != testCase.expectedErrorMessage {
					t.Errorf("Unexpected error: got %q, want: %q", err, testCase.expectedErrorMessage)
				}
			}

			if !testCase.expectError {
				if len(testCase.bootstrapClient.clusters[testCase.namespace]) != 0 {
					t.Fatalf("Unexpected cluster count in namespace %q. Got: %d, Want: 0", testCase.namespace, len(testCase.targetClient.clusters[testCase.namespace]))
				}
				if len(testCase.bootstrapClient.machines[testCase.namespace]) != 0 {
					t.Fatalf("Unexpected machine count in namespace %q. Got: %d, Want: 0", testCase.namespace, len(testCase.targetClient.machines[testCase.namespace]))
				}
				if len(testCase.bootstrapClient.machineSets[testCase.namespace]) != 0 {
					t.Fatalf("Unexpected machineSets count in namespace %q. Got: %d, Want: 0", testCase.namespace, len(testCase.targetClient.machineSets[testCase.namespace]))
				}
				if len(testCase.bootstrapClient.machineDeployments[testCase.namespace]) != 0 {
					t.Fatalf("Unexpected machineDeployments count in namespace %q. Got: %d, Want: 0", testCase.namespace, len(testCase.targetClient.machineDeployments[testCase.namespace]))
				}
				if len(testCase.bootstrapClient.clusters) != testCase.expectedExternalClusterCount {
					t.Fatalf("Unexpected remaining cluster count. Got: %d, Want: %d", len(testCase.bootstrapClient.clusters), testCase.expectedExternalClusterCount)
				}
				if contains(testCase.bootstrapClient.namespaces, testCase.namespace) {
					t.Fatalf("Unexpected remaining namespace %q in bootstrap cluster. Got: Found, Want: NotFound", testCase.namespace)
				}
				if testCase.namespace != apiv1.NamespaceDefault && contains(testCase.targetClient.namespaces, testCase.namespace) {
					t.Fatalf("Unexpected remaining namespace %q in target cluster. Got: Found, Want: NotFound", testCase.namespace)
				}
			}
		})
	}
}

func generateTestControlPlaneMachine(name string) *clusterv1.Machine {
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

func generateTestControlPlaneMachines(controlPlaneNames []string) []*clusterv1.Machine {
	controlPlanes := make([]*clusterv1.Machine, 0, len(controlPlaneNames))
	for _, mn := range controlPlaneNames {
		controlPlanes = append(controlPlanes, generateTestControlPlaneMachine(mn))
	}
	return controlPlanes
}

func generateTestNodeMachines(nodeNames []string) []*clusterv1.Machine {
	nodes := make([]*clusterv1.Machine, 0, len(nodeNames))
	for _, nn := range nodeNames {
		nodes = append(nodes, generateTestNodeMachine(nn))
	}
	return nodes
}

func generateInvalidExtractControlPlaneMachine(controlPlaneNames, nodeNames []string) []*clusterv1.Machine {
	controlPlanes := generateTestControlPlaneMachines(controlPlaneNames)
	nodes := generateTestNodeMachines(nodeNames)

	return append(controlPlanes, nodes...)
}

func generateValidExtractControlPlaneMachineInput(controlPlaneNames, nodeNames []string) []*clusterv1.Machine {
	controlPlanes := generateTestControlPlaneMachines(controlPlaneNames)
	nodes := generateTestNodeMachines(nodeNames)

	return append(controlPlanes, nodes...)
}

func generateMachines() []*clusterv1.Machine {
	controlPlaneMachine := generateTestControlPlaneMachine("test-control-plane")
	node := generateTestNodeMachine("test-node")
	return []*clusterv1.Machine{controlPlaneMachine, node}
}

func newMachineSetsFixture() []*clusterv1.MachineSet {
	return []*clusterv1.MachineSet{
		{ObjectMeta: metav1.ObjectMeta{Name: "machine-set-name-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "machine-set-name-2"}},
	}
}

func getClustersForNamespace(namespace string, count int) []*clusterv1.Cluster {
	var clusters []*clusterv1.Cluster
	for i := 0; i < count; i++ {
		clusters = append(clusters, &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-cluster", namespace),
				Namespace: namespace,
			},
		})
	}
	return clusters
}

func newMachineDeploymentsFixture() []*clusterv1.MachineDeployment {
	return []*clusterv1.MachineDeployment{
		{ObjectMeta: metav1.ObjectMeta{Name: "machine-deployment-name-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "machine-deployment-name-2"}},
	}
}

func newClustersFixture() []*clusterv1.Cluster {
	return []*clusterv1.Cluster{
		{ObjectMeta: metav1.ObjectMeta{Name: "cluster-name-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "cluster-name-2"}},
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
