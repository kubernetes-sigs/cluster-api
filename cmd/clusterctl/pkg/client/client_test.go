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

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
)

// dummy test to document fakeClient usage
func TestNewFakeClient(t *testing.T) {
	// create a fake config with a provider named P1 and a variable named var
	repository1Config := config.NewProvider("p1", "url", clusterctlv1.CoreProviderType)

	config1 := newFakeConfig().
		WithVar("var", "value").
		WithProvider(repository1Config)

	// create a new fakeClient using the fake config
	newFakeClient(config1)
}

type fakeClient struct {
	configClient   config.Client
	internalclient *clusterctlClient
}

var _ Client = &fakeClient{}

func (f fakeClient) GetProvidersConfig() ([]Provider, error) {
	return f.internalclient.GetProvidersConfig()
}

// newFakeClient return a fake implementation of the client for high-level clusterctl library, based on th given config.
func newFakeClient(configClient config.Client) *fakeClient {

	fake := &fakeClient{}

	fake.configClient = configClient
	if fake.configClient == nil {
		fake.configClient = newFakeConfig()
	}

	fake.internalclient, _ = newClusterctlClient("fake-config",
		InjectConfig(fake.configClient),
	)

	return fake
}

// newFakeConfig return a fake implementation of the client for low-level config library.
// The implementation uses a FakeReader that stores configuration settings in a config map; you can use
// the WithVar or WithProvider methods to set the config map values.
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

var (
	bootstrapProviderConfig = config.NewProvider("bootstrap", "url", clusterctlv1.BootstrapProviderType)
)
