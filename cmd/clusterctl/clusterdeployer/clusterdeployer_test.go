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

type stringCheckFunc func(string) error

type testClusterClient struct {
	ApplyErr                              error
	DeleteErr                             error
	WaitForClusterV1alpha1ReadyErr        error
	GetClustersErr                        error
	GetClusterErr                         error
	GetMachineClassesErr                  error
	GetMachineDeploymentsErr              error
	GetMachineSetsErr                     error
	GetMachineSetsForMachineDeploymentErr error
	GetMachinesForMachineSetErr           error
	GetMachinesErr                        error
	CreateClusterObjectErr                error
	CreateMachinesErr                     error
	CreateMachineSetsErr                  error
	CreateMachineDeploymentsErr           error
	DeleteClustersErr                     error
	DeleteMachineClassesErr               error
	DeleteMachineDeploymentsErr           error
	DeleteMachinesErr                     error
	DeleteMachineSetsErr                  error
	UpdateClusterObjectEndpointErr        error
	EnsureNamespaceErr                    error
	DeleteNamespaceErr                    error
	CloseErr                              error

	ApplyFunc stringCheckFunc

	clusters           map[string][]*clusterv1.Cluster
	machineClasses     map[string][]*clusterv1.MachineClass
	machineDeployments map[string][]*clusterv1.MachineDeployment
	machineSets        map[string][]*clusterv1.MachineSet
	machines           map[string][]*clusterv1.Machine
	namespaces         []string
	contextNamespace   string
}

func (c *testClusterClient) Apply(yaml string) error {
	if c.ApplyFunc != nil {
		return c.ApplyFunc(yaml)
	}
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

func (c *testClusterClient) GetCluster(clusterName, namespace string) (*clusterv1.Cluster, error) {
	if c.GetClusterErr != nil {
		return nil, c.GetClusterErr
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

func (c *testClusterClient) GetClusters(namespace string) ([]*clusterv1.Cluster, error) {
	if c.GetClustersErr != nil {
		return nil, c.GetClustersErr
	}
	if namespace != "" {
		return c.clusters[namespace], nil
	}
	var result []*clusterv1.Cluster // nolint
	for _, clusters := range c.clusters {
		result = append(result, clusters...)
	}
	return result, nil
}

func (c *testClusterClient) GetMachineDeployments(namespace string) ([]*clusterv1.MachineDeployment, error) {
	if c.GetMachineDeploymentsErr != nil {
		return nil, c.GetMachineDeploymentsErr
	}
	if namespace != "" {
		return c.machineDeployments[namespace], nil
	}
	var result []*clusterv1.MachineDeployment // nolint
	for _, machineDeployments := range c.machineDeployments {
		result = append(result, machineDeployments...)
	}
	return result, nil
}

func (c *testClusterClient) GetMachineSets(namespace string) ([]*clusterv1.MachineSet, error) {
	if c.GetMachineSetsErr != nil {
		return nil, c.GetMachineSetsErr
	}
	if namespace != "" {
		return c.machineSets[namespace], nil
	}
	var result []*clusterv1.MachineSet // nolint
	for _, machineSets := range c.machineSets {
		result = append(result, machineSets...)
	}
	return result, nil
}

func (c *testClusterClient) GetMachines(namespace string) ([]*clusterv1.Machine, error) {
	if c.GetMachinesErr != nil {
		return nil, c.GetMachinesErr
	}
	if namespace != "" {
		return c.machines[namespace], nil
	}
	var result []*clusterv1.Machine // nolint
	for _, machines := range c.machines {
		result = append(result, machines...)
	}
	return result, nil
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

func (c *testClusterClient) CreateMachineDeployments(deployments []*clusterv1.MachineDeployment, namespace string) error {
	if c.CreateMachineDeploymentsErr == nil {
		if c.machineDeployments == nil {
			c.machineDeployments = make(map[string][]*clusterv1.MachineDeployment)
		}
		c.machineDeployments[namespace] = append(c.machineDeployments[namespace], deployments...)
		return nil
	}
	return c.CreateMachineDeploymentsErr
}

func (c *testClusterClient) CreateMachineSets(machineSets []*clusterv1.MachineSet, namespace string) error {
	if c.CreateMachineSetsErr == nil {
		if c.machineSets == nil {
			c.machineSets = make(map[string][]*clusterv1.MachineSet)
		}
		c.machineSets[namespace] = append(c.machineSets[namespace], machineSets...)
		return nil
	}
	return c.CreateMachineSetsErr
}

func (c *testClusterClient) CreateMachines(machines []*clusterv1.Machine, namespace string) error {
	if c.CreateMachinesErr == nil {
		if c.machines == nil {
			c.machines = make(map[string][]*clusterv1.Machine)
		}
		c.machines[namespace] = append(c.machines[namespace], machines...)
		return nil
	}
	return c.CreateMachinesErr
}

func (c *testClusterClient) DeleteClusters(ns string) error {
	if c.DeleteClustersErr != nil {
		return c.DeleteClustersErr
	}
	if ns == "" {
		c.clusters = make(map[string][]*clusterv1.Cluster)
	} else {
		delete(c.clusters, ns)
	}
	return nil
}

func (c *testClusterClient) DeleteMachineClasses(ns string) error {
	if c.DeleteMachineClassesErr != nil {
		return c.DeleteMachineClassesErr
	}
	if ns == "" {
		c.machineClasses = make(map[string][]*clusterv1.MachineClass)
	} else {
		delete(c.machineClasses, ns)
	}
	return nil
}

func (c *testClusterClient) DeleteMachineDeployments(ns string) error {
	if c.DeleteMachineDeploymentsErr != nil {
		return c.DeleteMachineDeploymentsErr
	}
	if ns == "" {
		c.machineDeployments = make(map[string][]*clusterv1.MachineDeployment)
	} else {
		delete(c.machineDeployments, ns)
	}
	return nil
}

func (c *testClusterClient) DeleteMachineSets(ns string) error {
	if c.DeleteMachineSetsErr != nil {
		return c.DeleteMachineSetsErr
	}
	if ns == "" {
		c.machineSets = make(map[string][]*clusterv1.MachineSet)
	} else {
		delete(c.machineSets, ns)
	}
	return nil
}

func (c *testClusterClient) DeleteMachines(ns string) error {
	if c.DeleteMachinesErr != nil {
		return c.DeleteMachinesErr
	}
	if ns == "" {
		c.machines = make(map[string][]*clusterv1.Machine)
	} else {
		delete(c.machines, ns)
	}
	return nil
}

func (c *testClusterClient) UpdateClusterObjectEndpoint(string, string, string) error {
	return c.UpdateClusterObjectEndpointErr
}
func (c *testClusterClient) Close() error {
	return c.CloseErr
}

func (c *testClusterClient) EnsureNamespace(nsName string) error {
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

func (c *testClusterClient) ScaleStatefulSet(ns string, name string, scale int32) error {
	return nil
}

func (c *testClusterClient) GetMachineSetsForCluster(cluster *clusterv1.Cluster) ([]*clusterv1.MachineSet, error) {
	var result []*clusterv1.MachineSet
	for _, ms := range c.machineSets[cluster.Namespace] {
		if ms.Labels[clusterv1.MachineClusterLabelName] == cluster.Name {
			result = append(result, ms)
		}
	}
	return result, nil
}

func (c *testClusterClient) GetMachineDeploymentsForCluster(cluster *clusterv1.Cluster) ([]*clusterv1.MachineDeployment, error) {
	var result []*clusterv1.MachineDeployment
	for _, md := range c.machineDeployments[cluster.Namespace] {
		if md.Labels[clusterv1.MachineClusterLabelName] == cluster.Name {
			result = append(result, md)
		}
	}
	return result, nil
}

func (c *testClusterClient) GetMachinesForCluster(cluster *clusterv1.Cluster) ([]*clusterv1.Machine, error) {
	var result []*clusterv1.Machine
	for _, m := range c.machines[cluster.Namespace] {
		if m.Labels[clusterv1.MachineClusterLabelName] == cluster.Name {
			result = append(result, m)
		}
	}
	return result, nil
}

func (c *testClusterClient) GetMachineDeployment(namespace, name string) (*clusterv1.MachineDeployment, error) {
	for _, md := range c.machineDeployments[namespace] {
		if md.Name == name {
			return md, nil
		}
	}
	return nil, nil
}

func (c *testClusterClient) GetMachineSet(namespace, name string) (*clusterv1.MachineSet, error) {
	for _, ms := range c.machineSets[namespace] {
		if ms.Name == name {
			return ms, nil
		}
	}
	return nil, nil
}

func (c *testClusterClient) ForceDeleteCluster(namespace, name string) error {
	var newClusters []*clusterv1.Cluster
	for _, cluster := range c.clusters[namespace] {
		if cluster.Name != name {
			newClusters = append(newClusters, cluster)
		}
	}
	c.clusters[namespace] = newClusters
	return nil
}

func (c *testClusterClient) ForceDeleteMachine(namespace, name string) error {
	var newMachines []*clusterv1.Machine
	for _, machine := range c.machines[namespace] {
		if machine.Name != name {
			newMachines = append(newMachines, machine)
		}
	}
	c.machines[namespace] = newMachines
	return nil
}

func (c *testClusterClient) ForceDeleteMachineSet(namespace, name string) error {
	var newMachineSets []*clusterv1.MachineSet
	for _, ms := range c.machineSets[namespace] {
		if ms.Name != name {
			newMachineSets = append(newMachineSets, ms)
		}
	}
	c.machineSets[namespace] = newMachineSets
	return nil
}

func (c *testClusterClient) ForceDeleteMachineDeployment(namespace, name string) error {
	var newMachineDeployments []*clusterv1.MachineDeployment
	for _, md := range c.machineDeployments[namespace] {
		if md.Name != name {
			newMachineDeployments = append(newMachineDeployments, md)
		}
	}
	c.machineDeployments[namespace] = newMachineDeployments
	return nil
}

func (c *testClusterClient) GetMachineSetsForMachineDeployment(md *clusterv1.MachineDeployment) ([]*clusterv1.MachineSet, error) {
	if c.GetMachineSetsForMachineDeploymentErr != nil {
		return nil, c.GetMachineSetsForMachineDeploymentErr
	}
	var results []*clusterv1.MachineSet
	machineSets, err := c.GetMachineSets(md.Namespace)
	if err != nil {
		return nil, err
	}
	for _, ms := range machineSets {
		for _, or := range ms.OwnerReferences {
			if or.APIVersion == md.APIVersion && or.Kind == md.Kind && or.Name == md.Name {
				results = append(results, ms)
			}
		}
	}
	return results, nil
}

func (c *testClusterClient) GetMachinesForMachineSet(ms *clusterv1.MachineSet) ([]*clusterv1.Machine, error) {
	if c.GetMachinesForMachineSetErr != nil {
		return nil, c.GetMachinesForMachineSetErr
	}
	var results []*clusterv1.Machine
	machines, err := c.GetMachines(ms.Namespace)
	if err != nil {
		return nil, err
	}
	for _, m := range machines {
		for _, or := range m.OwnerReferences {
			if or.APIVersion == ms.APIVersion && or.Kind == ms.Kind && or.Name == ms.Name {
				results = append(results, m)
			}
		}
	}
	return results, nil
}

func (c *testClusterClient) WaitForResourceStatuses() error {
	return nil
}

// TODO: implement GetMachineClasses for testClusterClient and add tests
func (c *testClusterClient) GetMachineClasses(namespace string) ([]*clusterv1.MachineClass, error) {
	return c.machineClasses[namespace], c.GetMachineClassesErr
}

// TODO: implement CreateMachineClass for testClusterClient and add tests
func (c *testClusterClient) CreateMachineClass(*clusterv1.MachineClass) error {
	return errors.Errorf("CreateMachineClass Not yet implemented.")
}

// TODO: implement DeleteMachineClass for testClusterClient and add tests
func (c *testClusterClient) DeleteMachineClass(namespace, name string) error {
	return errors.Errorf("DeleteMachineClass Not yet implemented.")
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
	const bootstrapComponent = "bootstrap-only"

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
			targetClient:                        &testClusterClient{ApplyFunc: func(yaml string) error { return nil }},
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
			targetClient:                        &testClusterClient{ApplyFunc: func(yaml string) error { return nil }},
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
			targetClient:                        &testClusterClient{ApplyFunc: func(yaml string) error { return nil }},
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
			targetClient:                        &testClusterClient{ApplyFunc: func(yaml string) error { return nil }},
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
			name:                                "fail provision bootstrap cluster",
			targetClient:                        &testClusterClient{ApplyFunc: func(yaml string) error { return nil }},
			bootstrapClient:                     &testClusterClient{},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			provisionExternalErr:                errors.New("Test failure"),
			expectErr:                           true,
		},
		{
			name:                                "fail provision bootstrap cluster",
			targetClient:                        &testClusterClient{ApplyFunc: func(yaml string) error { return nil }},
			bootstrapClient:                     &testClusterClient{},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			provisionExternalErr:                errors.New("Test failure"),
			expectErr:                           true,
		},
		{
			name:                                "fail create clients",
			targetClient:                        &testClusterClient{ApplyFunc: func(yaml string) error { return nil }},
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
			targetClient:                        &testClusterClient{ApplyFunc: func(yaml string) error { return nil }},
			bootstrapClient:                     &testClusterClient{ApplyErr: errors.New("Test failure")},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			cleanupExternal:                     true,
			expectExternalCreated:               true,
			expectErr:                           true,
		},
		{
			name:                                "fail waiting for api ready on bootstrap cluster",
			targetClient:                        &testClusterClient{ApplyFunc: func(yaml string) error { return nil }},
			bootstrapClient:                     &testClusterClient{WaitForClusterV1alpha1ReadyErr: errors.New("Test failure")},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			cleanupExternal:                     true,
			expectExternalCreated:               true,
			expectErr:                           true,
		},
		{
			name:                                "fail getting bootstrap cluster objects",
			targetClient:                        &testClusterClient{ApplyFunc: func(yaml string) error { return nil }},
			bootstrapClient:                     &testClusterClient{GetClustersErr: errors.New("Test failure")},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			cleanupExternal:                     true,
			expectExternalCreated:               true,
			expectErr:                           true,
		},
		{
			name:                                "fail getting bootstrap machine objects",
			targetClient:                        &testClusterClient{ApplyFunc: func(yaml string) error { return nil }},
			bootstrapClient:                     &testClusterClient{GetMachinesErr: errors.New("Test failure")},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			cleanupExternal:                     true,
			expectExternalCreated:               true,
			expectErr:                           true,
		},
		{
			name:                                "fail create cluster",
			targetClient:                        &testClusterClient{ApplyFunc: func(yaml string) error { return nil }},
			bootstrapClient:                     &testClusterClient{CreateClusterObjectErr: errors.New("Test failure")},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			cleanupExternal:                     true,
			expectExternalCreated:               true,
			expectErr:                           true,
		},
		{
			name:                                "fail create control plane",
			targetClient:                        &testClusterClient{ApplyFunc: func(yaml string) error { return nil }},
			bootstrapClient:                     &testClusterClient{CreateMachinesErr: errors.New("Test failure")},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			cleanupExternal:                     true,
			expectExternalCreated:               true,
			expectErr:                           true,
		},
		{
			name:                                "fail update bootstrap cluster endpoint",
			targetClient:                        &testClusterClient{ApplyFunc: func(yaml string) error { return nil }},
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
			name:                                "fail create target cluster",
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
			targetClient:                        &testClusterClient{CreateMachinesErr: errors.New("Test failure")},
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
		{
			name: "success bootstrap_only components not applied to target cluster",
			targetClient: &testClusterClient{ApplyFunc: func(yaml string) error {
				if yaml == bootstrapComponent {
					return errors.New("Test failure. Bootstrap component should not be applied to target cluster")
				}
				return nil
			}},
			bootstrapClient:                     &testClusterClient{},
			namespaceToExpectedInternalMachines: make(map[string]int),
			namespaceToInputCluster:             map[string][]*clusterv1.Cluster{metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1)},
			cleanupExternal:                     true,
			expectExternalCreated:               true,
			expectErr:                           false,
			expectedTotalInternalClustersCount:  1,
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
			d := New(p, f, "", "", bootstrapComponent, testcase.cleanupExternal)

			inputMachines := make(map[string][]*clusterv1.Machine)

			for namespace, inputClusters := range testcase.namespaceToInputCluster {
				ns := namespace
				if ns == "" {
					ns = testcase.bootstrapClient.GetContextNamespace()
				}

				var err error
				for _, inputCluster := range inputClusters {
					inputCluster.Name = fmt.Sprintf("%s-cluster", ns)
					inputMachines[inputCluster.Name] = generateMachines(inputCluster, ns)
					err = d.Create(inputCluster, inputMachines[inputCluster.Name], pd, kubeconfigOut, &pcFactory)
					if err != nil {
						break
					}
					testcase.namespaceToExpectedInternalMachines[ns] = testcase.namespaceToExpectedInternalMachines[ns] + len(inputMachines[inputCluster.Name])
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

					clusterMachines := inputMachines[expectedClusterName]
					for _, m := range clusterMachines {
						found := false
						for _, wm := range testcase.targetClient.machines[ns] {
							if m.Name == wm.Name {
								found = true
							}
						}
						if !found {
							t.Fatalf("Unexpected machine name: %s/%s", ns, m.Name)
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

			inputCluster := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: metav1.NamespaceDefault,
				},
			}
			inputMachines := generateMachines(inputCluster, metav1.NamespaceDefault)
			pcFactory := mockProviderComponentsStoreFactory{NewFromCoreclientsetPCStore: &tc.pcStore}
			providerComponentsYaml := "---\nyaml: definition"
			addonsYaml := "---\nyaml: definition"
			d := New(p, f, providerComponentsYaml, addonsYaml, "", false)
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
		expectedCPMachines   []*clusterv1.Machine
		expectedNodeMachines []*clusterv1.Machine
		expectedError        error
	}{
		{
			name:                 "success_1_control_plane_1_node",
			inputMachines:        generateMachines(nil, metav1.NamespaceDefault),
			expectedCPMachines:   generateTestControlPlaneMachines(nil, metav1.NamespaceDefault, []string{singleControlPlaneName}),
			expectedNodeMachines: generateTestNodeMachines(nil, metav1.NamespaceDefault, []string{singleNodeName}),
			expectedError:        nil,
		},
		{
			name:                 "success_1_control_plane_multiple_nodes",
			inputMachines:        generateValidExtractControlPlaneMachineInput(nil, metav1.NamespaceDefault, []string{singleControlPlaneName}, multipleNodeNames),
			expectedCPMachines:   generateTestControlPlaneMachines(nil, metav1.NamespaceDefault, []string{singleControlPlaneName}),
			expectedNodeMachines: generateTestNodeMachines(nil, metav1.NamespaceDefault, multipleNodeNames),
			expectedError:        nil,
		},
		{
			name:                 "success_2_control_planes_1_node",
			inputMachines:        generateValidExtractControlPlaneMachineInput(nil, metav1.NamespaceDefault, multipleControlPlaneNames, []string{singleNodeName}),
			expectedCPMachines:   generateTestControlPlaneMachines(nil, metav1.NamespaceDefault, multipleControlPlaneNames),
			expectedNodeMachines: generateTestNodeMachines(nil, metav1.NamespaceDefault, []string{singleNodeName}),
			expectedError:        nil,
		},
		{
			name:                 "success_2_control_planes_multiple_nodes",
			inputMachines:        generateValidExtractControlPlaneMachineInput(nil, metav1.NamespaceDefault, multipleControlPlaneNames, multipleNodeNames),
			expectedCPMachines:   generateTestControlPlaneMachines(nil, metav1.NamespaceDefault, multipleControlPlaneNames),
			expectedNodeMachines: generateTestNodeMachines(nil, metav1.NamespaceDefault, multipleNodeNames),
			expectedError:        nil,
		},
		{
			name:                 "fail_0_control_plane_not_allowed",
			inputMachines:        generateTestNodeMachines(nil, metav1.NamespaceDefault, multipleNodeNames),
			expectedCPMachines:   nil,
			expectedNodeMachines: nil,
			expectedError:        errors.New("expected one or more control plane machines, got: 0"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualCPMachines, actualNodes, actualError := clusterclient.ExtractControlPlaneMachines(tc.inputMachines)

			if tc.expectedError == nil && actualError != nil {
				t.Fatalf("%s: extractControlPlaneMachine(%q): gotError %q; wantError [nil]", tc.name, len(tc.inputMachines), actualError)
			}

			if tc.expectedError != nil && tc.expectedError.Error() != actualError.Error() {
				t.Fatalf("%s: extractControlPlaneMachine(%q): gotError %q; wantError %q", tc.name, len(tc.inputMachines), actualError, tc.expectedError)
			}

			if (tc.expectedCPMachines == nil && actualCPMachines != nil) ||
				(tc.expectedCPMachines != nil && actualCPMachines == nil) {
				t.Fatalf("%s: extractControlPlaneMachine(%q): gotControlPlane = %v; wantControlPlane = %v", tc.name, len(tc.inputMachines), actualCPMachines[0] != nil, tc.expectedCPMachines != nil)
			}

			if len(tc.expectedNodeMachines) != len(actualNodes) {
				t.Fatalf("%s: extractControlPlaneMachine(%q): gotNodes = %q; wantNodes = %q", tc.name, len(tc.inputMachines), len(actualNodes), len(tc.expectedNodeMachines))
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
		{
			name:                   "success with cleanup",
			cleanupExternalCluster: true,
			provisionExternalErr:   nil,
			bootstrapClient:        &testClusterClient{},
			targetClient:           &testClusterClient{},
			expectedErrorMessage:   "",
		},
		{
			name:                   "success without cleanup",
			cleanupExternalCluster: false,
			provisionExternalErr:   nil,
			bootstrapClient:        &testClusterClient{},
			targetClient:           &testClusterClient{},
			expectedErrorMessage:   "",
		},
		{
			name:                   "error with cleanup",
			cleanupExternalCluster: true,
			provisionExternalErr:   nil,
			bootstrapClient:        &testClusterClient{},
			targetClient:           &testClusterClient{GetMachineSetsErr: errors.New("get machine sets error")},
			expectedErrorMessage:   "get machine sets error",
		},
		{
			name:                   "error without cleanup",
			cleanupExternalCluster: true,
			provisionExternalErr:   nil,
			bootstrapClient:        &testClusterClient{},
			targetClient:           &testClusterClient{GetMachineSetsErr: errors.New("get machine sets error")},
			expectedErrorMessage:   "get machine sets error",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			kubeconfigOut := newTempFile(t)
			defer os.Remove(kubeconfigOut)
			p := &testClusterProvisioner{err: tc.provisionExternalErr, kubeconfig: bootstrapKubeconfig}
			f := newTestClusterClientFactory()
			f.clusterClients[bootstrapKubeconfig] = tc.bootstrapClient
			f.clusterClients[targetKubeconfig] = tc.targetClient
			d := New(p, f, "", "", "", tc.cleanupExternalCluster)
			err := d.Delete(tc.targetClient)
			if err != nil || tc.expectedErrorMessage != "" {
				if err == nil {
					t.Errorf("expected error %q", tc.expectedErrorMessage)
				} else if errors.Cause(err).Error() != tc.expectedErrorMessage {
					t.Errorf("Unexpected error: got %q, want: %q", errors.Cause(err).Error(), tc.expectedErrorMessage)
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
		name                                   string
		provisionExternalErr                   error
		NewCoreClientsetErr                    error
		bootstrapClient                        *testClusterClient
		targetClient                           *testClusterClient
		expectedErrorMessage                   string
		expectedExternalClusterCount           int
		expectedExternalMachineCount           int
		expectedExternalMachineSetCount        int
		expectedExternalMachineDeploymentCount int
		expectedExternalMachineClassCount      int
		expectError                            bool
	}{
		{
			name:            "success delete 1/1 cluster",
			bootstrapClient: &testClusterClient{},
			targetClient: &testClusterClient{
				clusters: map[string][]*clusterv1.Cluster{
					metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1),
				},
				machines: map[string][]*clusterv1.Machine{
					metav1.NamespaceDefault: generateMachines(nil, metav1.NamespaceDefault),
				},
				machineSets: map[string][]*clusterv1.MachineSet{
					metav1.NamespaceDefault: newMachineSetsFixture(metav1.NamespaceDefault),
				},
				machineDeployments: map[string][]*clusterv1.MachineDeployment{
					metav1.NamespaceDefault: newMachineDeploymentsFixture(metav1.NamespaceDefault),
				},
			},
		},
		{
			name:            "success delete 3/3 clusters",
			bootstrapClient: &testClusterClient{},
			targetClient: &testClusterClient{
				clusters: map[string][]*clusterv1.Cluster{
					"foo": getClustersForNamespace("foo", 1),
					"bar": getClustersForNamespace("bar", 1),
					"baz": getClustersForNamespace("baz", 1),
				},
				machines: map[string][]*clusterv1.Machine{
					"foo": generateMachines(nil, "foo"),
					"bar": generateMachines(nil, "bar"),
					"baz": generateMachines(nil, "baz"),
				},
				machineSets: map[string][]*clusterv1.MachineSet{
					"foo": newMachineSetsFixture("foo"),
					"bar": newMachineSetsFixture("bar"),
					"baz": newMachineSetsFixture("baz"),
				},
				machineDeployments: map[string][]*clusterv1.MachineDeployment{
					"foo": newMachineDeploymentsFixture("foo"),
					"bar": newMachineDeploymentsFixture("bar"),
					"baz": newMachineDeploymentsFixture("baz"),
				},
			},
		},
		{
			name:                 "error creating core client",
			provisionExternalErr: nil,
			NewCoreClientsetErr:  errors.New("error creating core client"),
			bootstrapClient:      &testClusterClient{},
			targetClient:         &testClusterClient{},
			expectedErrorMessage: "error creating core client",
		},
		{
			name:                 "fail provision bootstrap cluster",
			provisionExternalErr: errors.New("minikube error"),
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{},
			targetClient:         &testClusterClient{},
			expectedErrorMessage: "minikube error",
		},
		{
			name:                 "fail apply yaml to bootstrap cluster",
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{ApplyErr: errors.New("yaml apply error")},
			targetClient:         &testClusterClient{},
			expectedErrorMessage: "yaml apply error",
		},
		{
			name:                 "fail delete provider components should succeed",
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{},
			targetClient:         &testClusterClient{DeleteErr: errors.New("kubectl delete error")},
			expectedErrorMessage: "",
		},
		{
			name:                 "error listing machines",
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{},
			targetClient:         &testClusterClient{GetMachinesErr: errors.New("get machines error")},
			expectedErrorMessage: "get machines error",
		},
		{
			name:                 "error listing machine sets",
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{},
			targetClient:         &testClusterClient{GetMachineSetsErr: errors.New("get machine sets error")},
			expectedErrorMessage: "get machine sets error",
		},
		{
			name:                 "error listing machine deployments",
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{},
			targetClient:         &testClusterClient{GetMachineDeploymentsErr: errors.New("get machine deployments error")},
			expectedErrorMessage: "get machine deployments error",
		},
		{
			name:                 "error listing clusters",
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{},
			targetClient:         &testClusterClient{GetClustersErr: errors.New("get clusters error")},
			expectedErrorMessage: "get clusters error",
		},
		{
			name:                 "error creating machines",
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{CreateMachinesErr: errors.New("create machines error")},
			targetClient: &testClusterClient{
				machines: map[string][]*clusterv1.Machine{
					metav1.NamespaceDefault: generateMachines(nil, metav1.NamespaceDefault),
				},
			},
			expectedErrorMessage:         "create machines error",
			expectedExternalMachineCount: 2,
		},
		{
			name:                 "error creating machine sets",
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{CreateMachineSetsErr: errors.New("create machine sets error")},
			targetClient: &testClusterClient{
				machineSets: map[string][]*clusterv1.MachineSet{
					metav1.NamespaceDefault: newMachineSetsFixture(metav1.NamespaceDefault),
				},
			},
			expectedErrorMessage:            "create machine sets error",
			expectedExternalMachineSetCount: 2,
		},
		{
			name:                 "error creating machine deployments",
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{CreateMachineDeploymentsErr: errors.New("create machine deployments error")},
			targetClient: &testClusterClient{
				machineDeployments: map[string][]*clusterv1.MachineDeployment{
					metav1.NamespaceDefault: newMachineDeploymentsFixture(metav1.NamespaceDefault),
				}},
			expectedErrorMessage:                   "create machine deployments error",
			expectedExternalMachineDeploymentCount: 2,
		},
		{
			name:                 "error creating cluster",
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{CreateClusterObjectErr: errors.New("create cluster error")},
			targetClient: &testClusterClient{
				clusters: map[string][]*clusterv1.Cluster{
					metav1.NamespaceDefault: getClustersForNamespace(metav1.NamespaceDefault, 1),
				},
			},
			expectedErrorMessage:         "create cluster error",
			expectedExternalClusterCount: 1,
		},
		{
			name:                 "error deleting machines",
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{DeleteMachinesErr: errors.New("delete machines error")},
			targetClient:         &testClusterClient{},
			expectedErrorMessage: "error(s) encountered deleting objects from bootstrap cluster: [error deleting Machines: delete machines error]",
		},
		{
			name:                 "error deleting machine sets",
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{DeleteMachineSetsErr: errors.New("delete machine sets error")},
			targetClient:         &testClusterClient{},
			expectedErrorMessage: "error(s) encountered deleting objects from bootstrap cluster: [error deleting MachineSets: delete machine sets error]",
		},
		{
			name:                 "error deleting machine deployments",
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{DeleteMachineDeploymentsErr: errors.New("delete machine deployments error")},
			targetClient:         &testClusterClient{},
			expectedErrorMessage: "error(s) encountered deleting objects from bootstrap cluster: [error deleting MachineDeployments: delete machine deployments error]",
		},
		{
			name:                 "error deleting clusters",
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{DeleteClustersErr: errors.New("delete clusters error")},
			targetClient:         &testClusterClient{},
			expectedErrorMessage: "error(s) encountered deleting objects from bootstrap cluster: [error deleting Clusters: delete clusters error]",
		},
		{
			name:                 "error deleting machines and clusters",
			provisionExternalErr: nil,
			NewCoreClientsetErr:  nil,
			bootstrapClient:      &testClusterClient{DeleteMachinesErr: errors.New("delete machines error"), DeleteClustersErr: errors.New("delete clusters error")},
			targetClient:         &testClusterClient{},
			expectedErrorMessage: "error(s) encountered deleting objects from bootstrap cluster: [error deleting Machines: delete machines error, error deleting Clusters: delete clusters error]",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			kubeconfigOut := newTempFile(t)
			defer os.Remove(kubeconfigOut)
			p := &testClusterProvisioner{
				err:        testCase.provisionExternalErr,
				kubeconfig: bootstrapKubeconfig,
			}
			f := newTestClusterClientFactory()
			f.clusterClients[bootstrapKubeconfig] = testCase.bootstrapClient
			f.ClusterClientErr = testCase.NewCoreClientsetErr
			d := New(p, f, "", "", "", true)

			err := d.Delete(testCase.targetClient)
			if err != nil || testCase.expectedErrorMessage != "" {
				if err == nil {
					t.Errorf("expected error %q", testCase.expectedErrorMessage)
				} else if errors.Cause(err).Error() != testCase.expectedErrorMessage {
					t.Errorf("Unexpected error: got %q, want: %q", errors.Cause(err).Error(), testCase.expectedErrorMessage)
				}
			}

			if !testCase.expectError {
				var (
					bootstrapClusters, bootstrapMachines, bootstrapMachineDeployments, bootstrapMachineSets, bootstrapMachineClasses int
					targetClusters, targetMachines, targetMachineDeployments, targetMachineSets, targetMachineClasses                int
				)
				for _, clusters := range testCase.bootstrapClient.clusters {
					bootstrapClusters = bootstrapClusters + len(clusters)
				}
				for _, machines := range testCase.bootstrapClient.machines {
					bootstrapMachines = bootstrapMachines + len(machines)
				}
				for _, machineDeployments := range testCase.bootstrapClient.machineDeployments {
					bootstrapMachineDeployments = bootstrapMachineDeployments + len(machineDeployments)
				}
				for _, machineSets := range testCase.bootstrapClient.machineSets {
					bootstrapMachineSets = bootstrapMachineSets + len(machineSets)
				}
				for _, machineClasses := range testCase.bootstrapClient.machineClasses {
					bootstrapMachineClasses = bootstrapMachineClasses + len(machineClasses)
				}
				for _, clusters := range testCase.targetClient.clusters {
					targetClusters = targetClusters + len(clusters)
				}
				for _, machines := range testCase.targetClient.machines {
					targetMachines = targetMachines + len(machines)
				}
				for _, machineDeployments := range testCase.targetClient.machineDeployments {
					targetMachineDeployments = targetMachineDeployments + len(machineDeployments)
				}
				for _, machineSets := range testCase.targetClient.machineSets {
					targetMachineSets = targetMachineSets + len(machineSets)
				}
				for _, machineClasses := range testCase.targetClient.machineClasses {
					targetMachineClasses = targetMachineClasses + len(machineClasses)
				}

				if bootstrapClusters != 0 {
					t.Fatalf("Unexpected Cluster count in bootstrap cluster. Got: %d, Want: 0", bootstrapClusters)
				}
				if bootstrapMachines != 0 {
					t.Fatalf("Unexpected Machine count in bootstrap cluster. Got: %d, Want: 0", bootstrapMachines)
				}
				if bootstrapMachineSets != 0 {
					t.Fatalf("Unexpected MachineSet count in bootstrap cluster. Got: %d, Want: 0", bootstrapMachineSets)
				}
				if bootstrapMachineDeployments != 0 {
					t.Fatalf("Unexpected MachineDeployment count in bootstrap cluster. Got: %d, Want: 0", bootstrapMachineDeployments)
				}
				if bootstrapMachineClasses != 0 {
					t.Fatalf("Unexpected MachineClass count in bootstrap cluster. Got: %d, Want: 0", bootstrapMachineClasses)
				}
				if targetClusters != testCase.expectedExternalClusterCount {
					t.Fatalf("Unexpected Cluster count in target cluster. Got: %d, Want: %d", targetClusters, testCase.expectedExternalClusterCount)
				}
				if targetMachines != testCase.expectedExternalMachineCount {
					t.Fatalf("Unexpected Machine count in target cluster. Got: %d, Want: %d", targetMachines, testCase.expectedExternalMachineCount)
				}
				if targetMachineSets != testCase.expectedExternalMachineSetCount {
					t.Fatalf("Unexpected MachineSet count in target cluster. Got: %d, Want: %d", targetMachineSets, testCase.expectedExternalMachineSetCount)
				}
				if targetMachineDeployments != testCase.expectedExternalMachineDeploymentCount {
					t.Fatalf("Unexpected MachineDeployment count in target cluster. Got: %d, Want: %d", targetMachineDeployments, testCase.expectedExternalMachineDeploymentCount)
				}
				if targetMachineClasses != testCase.expectedExternalMachineClassCount {
					t.Fatalf("Unexpected MachineClass count in target cluster. Got: %d, Want: %d", targetMachineClasses, testCase.expectedExternalMachineClassCount)
				}
			}
		})
	}
}

func generateTestControlPlaneMachines(cluster *clusterv1.Cluster, ns string, names []string) []*clusterv1.Machine {
	machines := make([]*clusterv1.Machine, 0, len(names))
	for _, name := range names {
		machine := generateTestNodeMachine(cluster, ns, name)
		machine.Spec = clusterv1.MachineSpec{
			Versions: clusterv1.MachineVersionInfo{
				ControlPlane: "1.10.1",
			},
		}
		machines = append(machines, machine)
	}
	return machines
}

func generateTestNodeMachine(cluster *clusterv1.Cluster, ns, name string) *clusterv1.Machine {
	machine := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
	if cluster != nil {
		machine.Labels = map[string]string{clusterv1.MachineClusterLabelName: cluster.Name}
		machine.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: cluster.APIVersion,
				Kind:       cluster.Kind,
				Name:       cluster.Name,
				UID:        cluster.UID,
			},
		}
	}
	return &machine
}

func generateTestNodeMachines(cluster *clusterv1.Cluster, ns string, nodeNames []string) []*clusterv1.Machine {
	nodes := make([]*clusterv1.Machine, 0, len(nodeNames))
	for _, nn := range nodeNames {
		nodes = append(nodes, generateTestNodeMachine(cluster, ns, nn))
	}
	return nodes
}

func generateValidExtractControlPlaneMachineInput(cluster *clusterv1.Cluster, ns string, controlPlaneName []string, nodeNames []string) []*clusterv1.Machine {
	var machines []*clusterv1.Machine
	machines = append(machines, generateTestControlPlaneMachines(cluster, ns, controlPlaneName)...)
	machines = append(machines, generateTestNodeMachines(cluster, ns, nodeNames)...)
	return machines
}

func generateMachines(cluster *clusterv1.Cluster, ns string) []*clusterv1.Machine {
	var machines []*clusterv1.Machine
	controlPlaneName := "control-plane"
	workerName := "node"
	if cluster != nil {
		controlPlaneName = cluster.Name + controlPlaneName
		workerName = cluster.Name + workerName
	}
	machines = append(machines, generateTestControlPlaneMachines(cluster, ns, []string{controlPlaneName})...)
	machines = append(machines, generateTestNodeMachine(cluster, ns, workerName))
	return machines
}

func newMachineSetsFixture(ns string) []*clusterv1.MachineSet {
	return []*clusterv1.MachineSet{
		{ObjectMeta: metav1.ObjectMeta{Name: "machine-set-name-1", Namespace: ns}},
		{ObjectMeta: metav1.ObjectMeta{Name: "machine-set-name-2", Namespace: ns}},
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

func newMachineDeploymentsFixture(ns string) []*clusterv1.MachineDeployment {
	return []*clusterv1.MachineDeployment{
		{ObjectMeta: metav1.ObjectMeta{Name: "machine-deployment-name-1", Namespace: ns}},
		{ObjectMeta: metav1.ObjectMeta{Name: "machine-deployment-name-2", Namespace: ns}},
	}
}

func newClustersFixture(ns string) []*clusterv1.Cluster {
	return []*clusterv1.Cluster{
		{ObjectMeta: metav1.ObjectMeta{Name: "cluster-name-1", Namespace: ns}},
		{ObjectMeta: metav1.ObjectMeta{Name: "cluster-name-2", Namespace: ns}},
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
