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
	"fmt"
	"testing"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/util"
)

func Test_clusterctlClient_Init(t *testing.T) {
	type field struct {
		client *fakeClient
		hasCRD bool
	}

	type args struct {
		coreProvider           string
		bootstrapProvider      []string
		infrastructureProvider []string
		targetNameSpace        string
		watchingNamespace      string
		force                  bool
	}
	type want struct {
		provider          Provider
		version           string
		targetNamespace   string
		watchingNamespace string
	}

	tests := []struct {
		name    string
		field   field
		args    args
		want    []want
		wantErr bool
	}{
		{
			name: "Init (with an empty cluster) with default provider versions",
			field: field{
				client: fakeEmptyCluster(), // clusterctl client for an empty management cluster (with repository setup for capi, bootstrap and infra provider)
				hasCRD: false,
			},
			args: args{
				coreProvider:           "",  // with an empty cluster, a core provider should be added automatically
				bootstrapProvider:      nil, // with an empty cluster, a bootstrap provider should be added automatically
				infrastructureProvider: []string{"infra"},
				targetNameSpace:        "",
				watchingNamespace:      "",
				force:                  false,
			},
			want: []want{
				{
					provider:          capiProviderConfig,
					version:           "v1.0.0",
					targetNamespace:   "ns1",
					watchingNamespace: "",
				},
				{
					provider:          bootstrapProviderConfig,
					version:           "v2.0.0",
					targetNamespace:   "ns2",
					watchingNamespace: "",
				},
				{
					provider:          infraProviderConfig,
					version:           "v3.0.0",
					targetNamespace:   "ns3",
					watchingNamespace: "",
				},
			},
			wantErr: false,
		},
		{
			name: "Init (with an empty cluster) with custom provider versions",
			field: field{
				client: fakeEmptyCluster(), // clusterctl client for an empty management cluster (with repository setup for capi, bootstrap and infra provider)
				hasCRD: false,
			},
			args: args{
				coreProvider:           fmt.Sprintf("%s:v1.1.0", config.ClusterAPIName),
				bootstrapProvider:      []string{fmt.Sprintf("%s:v2.1.0", config.KubeadmBootstrapProviderName)},
				infrastructureProvider: []string{"infra:v3.1.0"},
				targetNameSpace:        "",
				watchingNamespace:      "",
				force:                  false,
			},
			want: []want{
				{
					provider:          capiProviderConfig,
					version:           "v1.1.0",
					targetNamespace:   "ns1",
					watchingNamespace: "",
				},
				{
					provider:          bootstrapProviderConfig,
					version:           "v2.1.0",
					targetNamespace:   "ns2",
					watchingNamespace: "",
				},
				{
					provider:          infraProviderConfig,
					version:           "v3.1.0",
					targetNamespace:   "ns3",
					watchingNamespace: "",
				},
			},
			wantErr: false,
		},
		{
			name: "Init (with an empty cluster) with target namespace",
			field: field{
				client: fakeEmptyCluster(), // clusterctl client for an empty management cluster (with repository setup for capi, bootstrap and infra provider)
				hasCRD: false,
			},
			args: args{
				coreProvider:           "", // with an empty cluster, a core provider should be added automatically
				bootstrapProvider:      []string{config.KubeadmBootstrapProviderName},
				infrastructureProvider: []string{"infra"},
				targetNameSpace:        "nsx",
				watchingNamespace:      "",
				force:                  false,
			},
			want: []want{
				{
					provider:          capiProviderConfig,
					version:           "v1.0.0",
					targetNamespace:   "nsx",
					watchingNamespace: "",
				},
				{
					provider:          bootstrapProviderConfig,
					version:           "v2.0.0",
					targetNamespace:   "nsx",
					watchingNamespace: "",
				},
				{
					provider:          infraProviderConfig,
					version:           "v3.0.0",
					targetNamespace:   "nsx",
					watchingNamespace: "",
				},
			},
			wantErr: false,
		},
		{
			name: "Init (with a NOT empty cluster) adds a provider",
			field: field{
				client: fakeInitializedCluster(), // clusterctl client for an management cluster with capi installed (with repository setup for capi, bootstrap and infra provider)
				hasCRD: true,
			},
			args: args{
				coreProvider:           "", // with a NOT empty cluster, a core provider should NOT be added automatically
				bootstrapProvider:      []string{config.KubeadmBootstrapProviderName},
				infrastructureProvider: []string{"infra"},
				targetNameSpace:        "",
				watchingNamespace:      "",
				force:                  false,
			},
			want: []want{
				{
					provider:          bootstrapProviderConfig,
					version:           "v2.0.0",
					targetNamespace:   "ns2",
					watchingNamespace: "",
				},
				{
					provider:          infraProviderConfig,
					version:           "v3.0.0",
					targetNamespace:   "ns3",
					watchingNamespace: "",
				},
			},
			wantErr: false,
		},
		{
			name: "Fails when coreProvider is a provider with the wrong type",
			field: field{
				client: fakeEmptyCluster(), // clusterctl client for an empty management cluster (with repository setup for capi, bootstrap and infra provider)
			},
			args: args{
				coreProvider:           "infra",
				bootstrapProvider:      []string{"infra"},
				infrastructureProvider: []string{"infra"},
				targetNameSpace:        "",
				watchingNamespace:      "",
				force:                  false,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Fails when bootstrapProvider list contains providers of the wrong type",
			field: field{
				client: fakeEmptyCluster(), // clusterctl client for an empty management cluster (with repository setup for capi, bootstrap and infra provider)
			},
			args: args{
				coreProvider:           "",
				bootstrapProvider:      []string{"infra"},
				infrastructureProvider: []string{"infra"},
				targetNameSpace:        "",
				watchingNamespace:      "",
				force:                  false,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Fails when infrastructureProvider  list contains providers of the wrong type",
			field: field{
				client: fakeEmptyCluster(), // clusterctl client for an empty management cluster (with repository setup for capi, bootstrap and infra provider)
			},
			args: args{
				coreProvider:           "",
				bootstrapProvider:      []string{config.KubeadmBootstrapProviderName},
				infrastructureProvider: []string{config.KubeadmBootstrapProviderName},
				targetNameSpace:        "",
				watchingNamespace:      "",
				force:                  false,
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if tt.field.hasCRD {
				if err := tt.field.client.clusters["kubeconfig"].ProviderInventory().EnsureCustomResourceDefinitions(); err != nil {
					t.Fatalf("EnsureMetadata() error = %v", err)
				}
			}

			got, _, err := tt.field.client.Init(InitOptions{
				Kubeconfig:              "kubeconfig",
				CoreProvider:            tt.args.coreProvider,
				BootstrapProviders:      tt.args.bootstrapProvider,
				InfrastructureProviders: tt.args.infrastructureProvider,
				TargetNameSpace:         tt.args.targetNameSpace,
				WatchingNamespace:       tt.args.watchingNamespace,
				Force:                   tt.args.force,
			})

			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}

			if len(got) != len(tt.want) {
				t.Errorf("got = %v items, want %v items", len(got), len(tt.want))
				return
			}

			for i, g := range got {
				w := tt.want[i]

				if g.Name() != w.provider.Name() {
					t.Errorf("Item[%d].Name() got = %v, want = %v ", i, g.Name(), w.provider.Name())
				}

				if g.Type() != w.provider.Type() {
					t.Errorf("Item[%d].Type() got = %v, want = %v ", i, g.Type(), w.provider.Type())
				}

				if g.Version() != w.version {
					t.Errorf("Item[%d].Version() got = %v, want = %v ", i, g.Version(), w.version)
				}

				if g.TargetNamespace() != w.targetNamespace {
					t.Errorf("Item[%d].TargetNamespace() got = %v, want = %v ", i, g.TargetNamespace(), w.targetNamespace)
				}

				if g.WatchingNamespace() != w.watchingNamespace {
					t.Errorf("Item[%d].WatchingNamespace() got = %v, want = %v ", i, g.WatchingNamespace(), w.watchingNamespace)
				}
			}
		})
	}
}

var (
	capiProviderConfig      = config.NewProvider(config.ClusterAPIName, "url", clusterctlv1.CoreProviderType)
	bootstrapProviderConfig = config.NewProvider(config.KubeadmBootstrapProviderName, "url", clusterctlv1.BootstrapProviderType)
	infraProviderConfig     = config.NewProvider("infra", "url", clusterctlv1.InfrastructureProviderType)
)

// clusterctl client for an empty management cluster (with repository setup for capi, bootstrap and infra provider)
func fakeEmptyCluster() *fakeClient {
	config1 := newFakeConfig().
		WithVar("var", "value").
		WithProvider(capiProviderConfig).
		WithProvider(bootstrapProviderConfig).
		WithProvider(infraProviderConfig)

	repository1 := newFakeRepository(capiProviderConfig, config1.Variables()).
		WithPaths("root", "components.yaml").
		WithDefaultVersion("v1.0.0").
		WithFile("v1.0.0", "components.yaml", componentsYAML("ns1")).
		WithFile("v1.1.0", "components.yaml", componentsYAML("ns1"))
	repository2 := newFakeRepository(bootstrapProviderConfig, config1.Variables()).
		WithPaths("root", "components.yaml").
		WithDefaultVersion("v2.0.0").
		WithFile("v2.0.0", "components.yaml", componentsYAML("ns2")).
		WithFile("v2.1.0", "components.yaml", componentsYAML("ns2"))
	repository3 := newFakeRepository(infraProviderConfig, config1.Variables()).
		WithPaths("root", "components.yaml").
		WithDefaultVersion("v3.0.0").
		WithFile("v3.0.0", "components.yaml", componentsYAML("ns3")).
		WithFile("v3.1.0", "components.yaml", componentsYAML("ns3")).
		WithFile("v3.0.0", "config-kubeadm.yaml", templateYAML("ns3"))

	cluster1 := newFakeCluster("kubeconfig")

	client := newFakeClient(config1).
		// fake repository for capi, bootstrap and infra provider (matching provider's config)
		WithRepository(repository1).
		WithRepository(repository2).
		WithRepository(repository3).
		// fake empty cluster
		WithCluster(cluster1)

	return client
}

// clusterctl client for an management cluster with capi installed (with repository setup for capi, bootstrap and infra provider)
func fakeInitializedCluster() *fakeClient {
	client := fakeEmptyCluster()

	p := client.clusters["kubeconfig"].Proxy()
	fp := p.(*test.FakeProxy)

	fp.WithProviderInventory(capiProviderConfig.Name(), capiProviderConfig.Type(), "v1.0.0", "capi-system", "")

	return client
}

func componentsYAML(ns string) []byte {
	var namespaceYaml = []byte("apiVersion: v1\n" +
		"kind: Namespace\n" +
		"metadata:\n" +
		fmt.Sprintf("  name: %s", ns))

	var podYaml = []byte("apiVersion: v1\n" +
		"kind: Pod\n" +
		"metadata:\n" +
		"  name: manager")

	return util.JoinYaml(namespaceYaml, podYaml)
}

func templateYAML(ns string) []byte {
	var podYaml = []byte("apiVersion: v1\n" +
		"kind: Pod\n" +
		"metadata:\n" +
		"  name: manager\n" +
		fmt.Sprintf("  namespace: %s", ns))

	return podYaml
}
