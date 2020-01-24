/*
Copyright 2019 The Kubernetes Authors.

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

package client

import (
	"testing"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/repository"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
)

// dummy test to document fakeClient usage
func TestNewFakeClient(t *testing.T) {
	// create a fake config with a provider named P1 and a variable named var
	repository1Config := config.NewProvider("p1", "url", clusterctlv1.CoreProviderType)

	config1 := newFakeConfig().
		WithVar("var", "value").
		WithProvider(repository1Config)

	// create a fake repository with some YAML files in it (usually matching the list of providers defined in the config)
	repository1 := newFakeRepository(repository1Config, config1.Variables()).
		WithPaths("root", "components").
		WithDefaultVersion("v1.0").
		WithFile("v1.0", "components.yaml", []byte("content"))

	// create a fake cluster, eventually adding some existing runtime objects to it
	cluster1 := newFakeCluster("cluster1").
		WithObjs()

	// create a new fakeClient that allows to execute tests on the fake config, the fake repositories and the fake cluster.
	newFakeClient(config1).
		WithRepository(repository1).
		WithCluster(cluster1)
}

type fakeClient struct {
	configClient   config.Client
	clusters       map[string]cluster.Client
	repositories   map[string]repository.Client
	internalClient *clusterctlClient
}

var _ Client = &fakeClient{}

func (f fakeClient) GetProvidersConfig() ([]Provider, error) {
	return f.internalClient.GetProvidersConfig()
}

func (f fakeClient) GetProviderComponents(provider, targetNameSpace, watchingNamespace string) (Components, error) {
	return f.internalClient.GetProviderComponents(provider, targetNameSpace, watchingNamespace)
}

func (f fakeClient) GetClusterTemplate(options GetClusterTemplateOptions) (Template, error) {
	return f.internalClient.GetClusterTemplate(options)
}

func (f fakeClient) Init(options InitOptions) ([]Components, bool, error) {
	return f.internalClient.Init(options)
}

func (f fakeClient) Delete(options DeleteOptions) error {
	return f.internalClient.Delete(options)
}

// newFakeClient returns a clusterctl client that allows to execute tests on a set of fake config, fake repositories and fake clusters.
// you can use WithCluster and WithRepository to prepare for the test case.
func newFakeClient(configClient config.Client) *fakeClient {

	fake := &fakeClient{
		clusters:     map[string]cluster.Client{},
		repositories: map[string]repository.Client{},
	}

	fake.configClient = configClient
	if fake.configClient == nil {
		fake.configClient = newFakeConfig()
	}

	var clusterClientFactory = func(kubeconfig string) (cluster.Client, error) {
		if _, ok := fake.clusters[kubeconfig]; !ok {
			return nil, errors.Errorf("Cluster for kubeconfig %q does not exists.", kubeconfig)
		}
		return fake.clusters[kubeconfig], nil
	}

	fake.internalClient, _ = newClusterctlClient("fake-config",
		InjectConfig(fake.configClient),
		InjectClusterClientFactory(clusterClientFactory),
		InjectRepositoryFactory(func(provider config.Provider) (repository.Client, error) {
			if _, ok := fake.repositories[provider.Name()]; !ok {
				return nil, errors.Errorf("Repository for kubeconfig %q does not exists.", provider.Name())
			}
			return fake.repositories[provider.Name()], nil
		}),
	)

	return fake
}

func (f *fakeClient) WithCluster(clusterClient cluster.Client) *fakeClient {
	f.clusters[clusterClient.Kubeconfig()] = clusterClient
	return f
}

func (f *fakeClient) WithRepository(repositoryClient repository.Client) *fakeClient {
	if fc, ok := f.configClient.(fakeConfigClient); ok {
		fc.WithProvider(repositoryClient)
	}
	f.repositories[repositoryClient.Name()] = repositoryClient
	return f
}

// newFakeCluster returns a fakeClusterClient that
// internally uses a FakeProxy (based on the controller-runtime FakeClient).
// You can use WithObjs to pre-load a set of runtime objects in the cluster.
func newFakeCluster(kubeconfig string) *fakeClusterClient {
	fakeProxy := test.NewFakeProxy()

	options := cluster.Options{
		InjectProxy: fakeProxy,
	}

	client := cluster.New("", options)

	return &fakeClusterClient{
		kubeconfig:     kubeconfig,
		fakeProxy:      fakeProxy,
		internalclient: client,
	}
}

type fakeCertMangerClient struct {
}

var _ cluster.CertMangerClient = &fakeCertMangerClient{}

func (p *fakeCertMangerClient) EnsureWebHook() error {
	// For unit test, we are not installing the cert-manager WebHook so we always return no error without doing additional steps.
	return nil
}

type fakeClusterClient struct {
	kubeconfig     string
	fakeProxy      *test.FakeProxy
	internalclient cluster.Client
}

var _ cluster.Client = &fakeClusterClient{}

func (f fakeClusterClient) Kubeconfig() string {
	return f.kubeconfig
}

func (f fakeClusterClient) Proxy() cluster.Proxy {
	return f.fakeProxy
}

func (f *fakeClusterClient) CertManger() cluster.CertMangerClient {
	return &fakeCertMangerClient{}
}

func (f fakeClusterClient) ProviderComponents() cluster.ComponentsClient {
	return f.internalclient.ProviderComponents()
}

func (f fakeClusterClient) ProviderInventory() cluster.InventoryClient {
	return f.internalclient.ProviderInventory()
}

func (f fakeClusterClient) ProviderObjects() cluster.ObjectsClient {
	return f.internalclient.ProviderObjects()
}

func (f fakeClusterClient) ProviderInstaller() cluster.ProviderInstaller {
	return f.internalclient.ProviderInstaller()
}

func (f *fakeClusterClient) WithObjs(objs ...runtime.Object) *fakeClusterClient {
	f.fakeProxy.WithObjs(objs...)
	return f
}

func (f *fakeClusterClient) WithProviderInventory(name string, providerType clusterctlv1.ProviderType, version, targetNamespace, watchingNamespace string) *fakeClusterClient {
	f.fakeProxy.WithProviderInventory(name, providerType, version, targetNamespace, watchingNamespace)
	return f
}

// newFakeConfig return a fake implementation of the client for low-level config library.
// The implementation uses a FakeReader that stores configuration settings in a map; you can use
// the WithVar or WithProvider methods to set the map values.
func newFakeConfig() *fakeConfigClient {
	fakeReader := test.NewFakeReader()

	client, _ := config.New("fake-config", config.InjectReader(fakeReader))

	return &fakeConfigClient{
		fakeReader:     fakeReader,
		internalclient: client,
	}
}

type fakeConfigClient struct {
	fakeReader     *test.FakeReader
	internalclient config.Client
}

var _ config.Client = &fakeConfigClient{}

func (f fakeConfigClient) Providers() config.ProvidersClient {
	return f.internalclient.Providers()
}

func (f fakeConfigClient) Variables() config.VariablesClient {
	return f.internalclient.Variables()
}

func (f *fakeConfigClient) WithVar(key, value string) *fakeConfigClient {
	f.fakeReader.WithVar(key, value)
	return f
}

func (f *fakeConfigClient) WithProvider(provider config.Provider) *fakeConfigClient {
	f.fakeReader.WithProvider(provider.Name(), provider.Type(), provider.URL())
	return f
}

// newFakeRepository return a fake implementation of the client for low-level repository library.
// The implementation stores configuration settings in a map; you can use
// the WithPaths or WithDefaultVersion methods to configure the repository and WithFile to set the map values.
func newFakeRepository(provider config.Provider, configVariablesClient config.VariablesClient) *fakeRepositoryClient {
	fakeRepository := test.NewFakeRepository()
	options := repository.Options{
		InjectRepository: fakeRepository,
	}

	if configVariablesClient == nil {
		configVariablesClient = newFakeConfig().Variables()
	}

	client, _ := repository.New(provider, configVariablesClient, options)

	return &fakeRepositoryClient{
		Provider:       provider,
		fakeRepository: fakeRepository,
		client:         client,
	}
}

type fakeRepositoryClient struct {
	config.Provider
	fakeRepository *test.FakeRepository
	client         repository.Client
}

var _ repository.Client = &fakeRepositoryClient{}

func (f fakeRepositoryClient) DefaultVersion() string {
	return f.fakeRepository.DefaultVersion()
}

func (f fakeRepositoryClient) GetVersions() ([]string, error) {
	return f.fakeRepository.GetVersions()
}

func (f fakeRepositoryClient) Components() repository.ComponentsClient {
	return f.client.Components()
}

func (f fakeRepositoryClient) Templates(version string) repository.TemplateClient {
	return f.client.Templates(version)
}

func (f fakeRepositoryClient) Metadata(version string) repository.MetadataClient {
	return f.client.Metadata(version)
}

func (f *fakeRepositoryClient) WithPaths(rootPath, componentsPath string) *fakeRepositoryClient {
	f.fakeRepository.WithPaths(rootPath, componentsPath)
	return f
}

func (f *fakeRepositoryClient) WithDefaultVersion(version string) *fakeRepositoryClient {
	f.fakeRepository.WithDefaultVersion(version)
	return f
}

func (f *fakeRepositoryClient) WithFile(version, path string, content []byte) *fakeRepositoryClient {
	f.fakeRepository.WithFile(version, path, content)
	return f
}
