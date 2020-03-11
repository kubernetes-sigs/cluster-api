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

	. "github.com/onsi/gomega"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/util"
)

func Test_clusterctlClient_Init(t *testing.T) {
	g := NewWithT(t)

	type field struct {
		client *fakeClient
		hasCRD bool
	}

	type args struct {
		coreProvider           string
		bootstrapProvider      []string
		controlPlaneProvider   []string
		infrastructureProvider []string
		targetNameSpace        string
		watchingNamespace      string
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
				client: fakeEmptyCluster(), // clusterctl client for an empty management cluster (with repository setup for capi, bootstrap, control plane and infra provider)
				hasCRD: false,
			},
			args: args{
				coreProvider:           "",  // with an empty cluster, a core provider should be added automatically
				bootstrapProvider:      nil, // with an empty cluster, a bootstrap provider should be added automatically
				controlPlaneProvider:   nil, // with an empty cluster, a control plane provider should be added automatically
				infrastructureProvider: []string{"infra"},
				targetNameSpace:        "",
				watchingNamespace:      "",
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
					provider:          controlPlaneProviderConfig,
					version:           "v2.0.0",
					targetNamespace:   "ns3",
					watchingNamespace: "",
				},
				{
					provider:          infraProviderConfig,
					version:           "v3.0.0",
					targetNamespace:   "ns4",
					watchingNamespace: "",
				},
			},
			wantErr: false,
		},
		{
			name: "Init (with an empty cluster) opting out from automatic install of providers",
			field: field{
				client: fakeEmptyCluster(), // clusterctl client for an empty management cluster (with repository setup for capi, bootstrap, control plane and infra provider)
				hasCRD: false,
			},
			args: args{
				coreProvider:           "",            // with an empty cluster, a core provider should be added automatically
				bootstrapProvider:      []string{"-"}, // opt-out from the automatic bootstrap provider installation
				controlPlaneProvider:   []string{"-"}, // opt-out from the automatic control plane provider installation
				infrastructureProvider: []string{"infra"},
				targetNameSpace:        "",
				watchingNamespace:      "",
			},
			want: []want{
				{
					provider:          capiProviderConfig,
					version:           "v1.0.0",
					targetNamespace:   "ns1",
					watchingNamespace: "",
				},
				{
					provider:          infraProviderConfig,
					version:           "v3.0.0",
					targetNamespace:   "ns4",
					watchingNamespace: "",
				},
			},
			wantErr: false,
		},
		{
			name: "Init (with an empty cluster) with custom provider versions",
			field: field{
				client: fakeEmptyCluster(), // clusterctl client for an empty management cluster (with repository setup for capi, bootstrap, control plane and infra provider)
				hasCRD: false,
			},
			args: args{
				coreProvider:           fmt.Sprintf("%s:v1.1.0", config.ClusterAPIProviderName),
				bootstrapProvider:      []string{fmt.Sprintf("%s:v2.1.0", config.KubeadmBootstrapProviderName)},
				controlPlaneProvider:   []string{fmt.Sprintf("%s:v2.1.0", config.KubeadmControlPlaneProviderName)},
				infrastructureProvider: []string{"infra:v3.1.0"},
				targetNameSpace:        "",
				watchingNamespace:      "",
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
					provider:          controlPlaneProviderConfig,
					version:           "v2.1.0",
					targetNamespace:   "ns3",
					watchingNamespace: "",
				},
				{
					provider:          infraProviderConfig,
					version:           "v3.1.0",
					targetNamespace:   "ns4",
					watchingNamespace: "",
				},
			},
			wantErr: false,
		},
		{
			name: "Init (with an empty cluster) with target namespace",
			field: field{
				client: fakeEmptyCluster(), // clusterctl client for an empty management cluster (with repository setup for capi, bootstrap, control plane and infra provider)
				hasCRD: false,
			},
			args: args{
				coreProvider:           "", // with an empty cluster, a core provider should be added automatically
				bootstrapProvider:      []string{config.KubeadmBootstrapProviderName},
				infrastructureProvider: []string{"infra"},
				targetNameSpace:        "nsx",
				watchingNamespace:      "",
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
					provider:          controlPlaneProviderConfig,
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
				client: fakeInitializedCluster(), // clusterctl client for an management cluster with capi installed (with repository setup for capi, bootstrap, control plane and infra provider)
				hasCRD: true,
			},
			args: args{
				coreProvider:           "", // with a NOT empty cluster, a core provider should NOT be added automatically
				bootstrapProvider:      []string{config.KubeadmBootstrapProviderName},
				infrastructureProvider: []string{"infra"},
				targetNameSpace:        "",
				watchingNamespace:      "",
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
					targetNamespace:   "ns4",
					watchingNamespace: "",
				},
			},
			wantErr: false,
		},
		{
			name: "Fails when opting out from coreProvider automatic installation",
			field: field{
				client: fakeEmptyCluster(), // clusterctl client for an empty management cluster (with repository setup for capi, bootstrap, control plane and infra provider)
			},
			args: args{
				coreProvider:           "-", // not allowed
				bootstrapProvider:      nil,
				controlPlaneProvider:   nil,
				infrastructureProvider: nil,
				targetNameSpace:        "",
				watchingNamespace:      "",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Fails when coreProvider is a provider with the wrong type",
			field: field{
				client: fakeEmptyCluster(), // clusterctl client for an empty management cluster (with repository setup for capi, bootstrap, control plane and infra provider)
			},
			args: args{
				coreProvider:           "infra", // wrong
				bootstrapProvider:      nil,
				controlPlaneProvider:   nil,
				infrastructureProvider: nil,
				targetNameSpace:        "",
				watchingNamespace:      "",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Fails when bootstrapProvider list contains providers of the wrong type",
			field: field{
				client: fakeEmptyCluster(), // clusterctl client for an empty management cluster (with repository setup for capi, bootstrap, control plane and infra provider)
			},
			args: args{
				coreProvider:           "",
				bootstrapProvider:      []string{"infra"}, // wrong
				controlPlaneProvider:   nil,
				infrastructureProvider: nil,
				targetNameSpace:        "",
				watchingNamespace:      "",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Fails when controlPlaneProvider list contains providers of the wrong type",
			field: field{
				client: fakeEmptyCluster(), // clusterctl client for an empty management cluster (with repository setup for capi, bootstrap, control plane and infra provider)
			},
			args: args{
				coreProvider:           "",
				bootstrapProvider:      nil,
				controlPlaneProvider:   []string{"infra"}, // wrong
				infrastructureProvider: nil,
				targetNameSpace:        "",
				watchingNamespace:      "",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Fails when infrastructureProvider list contains providers of the wrong type",
			field: field{
				client: fakeEmptyCluster(), // clusterctl client for an empty management cluster (with repository setup for capi, bootstrap, control plane and infra provider)
			},
			args: args{
				coreProvider:           "",
				bootstrapProvider:      nil,
				controlPlaneProvider:   nil,
				infrastructureProvider: []string{config.KubeadmBootstrapProviderName}, // wrong
				targetNameSpace:        "",
				watchingNamespace:      "",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if tt.field.hasCRD {
				g.Expect(tt.field.client.clusters["kubeconfig"].ProviderInventory().EnsureCustomResourceDefinitions()).To(Succeed())
			}

			got, err := tt.field.client.Init(InitOptions{
				Kubeconfig:              "kubeconfig",
				CoreProvider:            tt.args.coreProvider,
				BootstrapProviders:      tt.args.bootstrapProvider,
				ControlPlaneProviders:   tt.args.controlPlaneProvider,
				InfrastructureProviders: tt.args.infrastructureProvider,
				TargetNamespace:         tt.args.targetNameSpace,
				WatchingNamespace:       tt.args.watchingNamespace,
			})
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(got).To(HaveLen(len(tt.want)))
			for i, gItem := range got {
				w := tt.want[i]
				g.Expect(gItem.Name()).To(Equal(w.provider.Name()))
				g.Expect(gItem.Type()).To(Equal(w.provider.Type()))
				g.Expect(gItem.Version()).To(Equal(w.version))
				g.Expect(gItem.TargetNamespace()).To(Equal(w.targetNamespace))
				g.Expect(gItem.WatchingNamespace()).To(Equal(w.watchingNamespace))
			}
		})
	}
}

var (
	capiProviderConfig         = config.NewProvider(config.ClusterAPIProviderName, "url", clusterctlv1.CoreProviderType)
	bootstrapProviderConfig    = config.NewProvider(config.KubeadmBootstrapProviderName, "url", clusterctlv1.BootstrapProviderType)
	controlPlaneProviderConfig = config.NewProvider(config.KubeadmControlPlaneProviderName, "url", clusterctlv1.ControlPlaneProviderType)
	infraProviderConfig        = config.NewProvider("infra", "url", clusterctlv1.InfrastructureProviderType)
)

// clusterctl client for an empty management cluster (with repository setup for capi, bootstrap and infra provider)
func fakeEmptyCluster() *fakeClient {
	config1 := newFakeConfig().
		WithVar("var", "value").
		WithProvider(capiProviderConfig).
		WithProvider(bootstrapProviderConfig).
		WithProvider(controlPlaneProviderConfig).
		WithProvider(infraProviderConfig)

	repository1 := newFakeRepository(capiProviderConfig, config1).
		WithPaths("root", "components.yaml").
		WithDefaultVersion("v1.0.0").
		WithFile("v1.0.0", "components.yaml", componentsYAML("ns1")).
		WithMetadata("v1.0.0", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 1, Minor: 0, Contract: "v1alpha3"},
			},
		}).
		WithFile("v1.1.0", "components.yaml", componentsYAML("ns1")).
		WithMetadata("v1.1.0", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 1, Minor: 1, Contract: "v1alpha3"},
			},
		})
	repository2 := newFakeRepository(bootstrapProviderConfig, config1).
		WithPaths("root", "components.yaml").
		WithDefaultVersion("v2.0.0").
		WithFile("v2.0.0", "components.yaml", componentsYAML("ns2")).
		WithMetadata("v2.0.0", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 2, Minor: 0, Contract: "v1alpha3"},
			},
		}).
		WithFile("v2.1.0", "components.yaml", componentsYAML("ns2")).
		WithMetadata("v2.1.0", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 2, Minor: 1, Contract: "v1alpha3"},
			},
		})
	repository3 := newFakeRepository(controlPlaneProviderConfig, config1).
		WithPaths("root", "components.yaml").
		WithDefaultVersion("v2.0.0").
		WithFile("v2.0.0", "components.yaml", componentsYAML("ns3")).
		WithMetadata("v2.0.0", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 2, Minor: 0, Contract: "v1alpha3"},
			},
		}).
		WithFile("v2.1.0", "components.yaml", componentsYAML("ns3")).
		WithMetadata("v2.1.0", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 2, Minor: 1, Contract: "v1alpha3"},
			},
		})
	repository4 := newFakeRepository(infraProviderConfig, config1).
		WithPaths("root", "components.yaml").
		WithDefaultVersion("v3.0.0").
		WithFile("v3.0.0", "components.yaml", componentsYAML("ns4")).
		WithMetadata("v3.0.0", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 3, Minor: 0, Contract: "v1alpha3"},
			},
		}).
		WithFile("v3.1.0", "components.yaml", componentsYAML("ns4")).
		WithMetadata("v3.1.0", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 3, Minor: 1, Contract: "v1alpha3"},
			},
		}).
		WithFile("v3.0.0", "cluster-template.yaml", templateYAML("ns4", "test"))

	cluster1 := newFakeCluster("kubeconfig", config1).
		// fake repository for capi, bootstrap and infra provider (matching provider's config)
		WithRepository(repository1).
		WithRepository(repository2).
		WithRepository(repository3).
		WithRepository(repository4)

	client := newFakeClient(config1).
		// fake repository for capi, bootstrap and infra provider (matching provider's config)
		WithRepository(repository1).
		WithRepository(repository2).
		WithRepository(repository3).
		WithRepository(repository4).
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

func templateYAML(ns string, clusterName string) []byte {
	var podYaml = []byte("apiVersion: v1\n" +
		"kind: Cluster\n" +
		"metadata:\n" +
		fmt.Sprintf("  name: %s\n", clusterName) +
		fmt.Sprintf("  namespace: %s", ns))

	return podYaml
}
