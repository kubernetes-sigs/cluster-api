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
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	utilyaml "sigs.k8s.io/cluster-api/util/yaml"
)

var (
	ctx = ctrl.SetupSignalHandler()
)

func Test_clusterctlClient_InitImages(t *testing.T) {
	type field struct {
		client *fakeClient
	}

	type args struct {
		kubeconfigContext      string
		coreProvider           string
		bootstrapProvider      []string
		controlPlaneProvider   []string
		infrastructureProvider []string
	}

	tests := []struct {
		name                 string
		field                field
		args                 args
		additionalProviders  []Provider
		expectedImages       []string
		wantErr              bool
		expectedErrorMessage string
		certManagerImages    []string
		certManagerImagesErr error
	}{
		{
			name: "returns error if cannot find cluster client",
			field: field{
				client: fakeEmptyCluster(),
			},
			args: args{
				kubeconfigContext: "does-not-exist",
			},
			expectedImages: []string{},
			wantErr:        true,
		},
		{
			name: "returns list of images even if component variable values are not found",
			args: args{
				coreProvider:           "",  // with an empty cluster, a core provider should be added automatically
				bootstrapProvider:      nil, // with an empty cluster, a bootstrap provider should be added automatically
				controlPlaneProvider:   nil, // with an empty cluster, a control plane provider should be added automatically
				infrastructureProvider: []string{"infra"},
				kubeconfigContext:      "mgmt-context",
			},
			expectedImages: []string{
				"registry.k8s.io/cluster-api-aws/cluster-api-aws-controller:v0.5.3",
			},
			wantErr: false,
		},
		{
			name: "returns error when core provider name is invalid",
			args: args{
				coreProvider:      "some-core-provider",
				kubeconfigContext: "mgmt-context",
			},
			additionalProviders: []Provider{
				config.NewProvider("some-core-provider", "some-core-url", clusterctlv1.CoreProviderType),
			},
			wantErr:              true,
			expectedErrorMessage: "name cluster-api must be used with the CoreProvider type",
		},
		{
			name: "return no error when core provider as the correct name",
			args: args{
				coreProvider:           config.ClusterAPIProviderName,
				bootstrapProvider:      nil,
				controlPlaneProvider:   nil,
				infrastructureProvider: nil,
				kubeconfigContext:      "mgmt-context",
			},
			expectedImages: []string{},
			wantErr:        false,
		},
		{
			name: "returns error when a bootstrap provider is not present",
			args: args{
				bootstrapProvider: []string{"not-provided"},
				kubeconfigContext: "mgmt-context",
			},
			wantErr:              true,
			expectedErrorMessage: "failed to get configuration for the BootstrapProvider with name not-provided",
		},
		{
			name: "returns error when a control plane provider is not present",
			args: args{
				controlPlaneProvider: []string{"not-provided"},
				kubeconfigContext:    "mgmt-context",
			},
			wantErr:              true,
			expectedErrorMessage: "failed to get configuration for the ControlPlaneProvider with name not-provided",
		},
		{
			name: "returns error when a infrastructure provider is not present",
			args: args{
				infrastructureProvider: []string{"not-provided"},
				kubeconfigContext:      "mgmt-context",
			},
			wantErr:              true,
			expectedErrorMessage: "failed to get configuration for the InfrastructureProvider with name not-provided",
		},
		{
			name: "returns certificate manager images when required",
			args: args{
				kubeconfigContext: "mgmt-context",
			},
			wantErr: false,
			certManagerImages: []string{
				"some.registry.com/cert-image-1:latest",
				"some.registry.com/cert-image-2:some-tag",
			},
			expectedImages: []string{
				"some.registry.com/cert-image-1:latest",
				"some.registry.com/cert-image-2:some-tag",
			},
		},
		{
			name: "returns error when cert-manager client cannot retrieve the image list",
			args: args{
				kubeconfigContext: "mgmt-context",
			},
			wantErr:              true,
			certManagerImagesErr: errors.New("failed to get cert images"),
		},
	}

	for _, tt := range tests {
		_, fc := setupCluster(tt.additionalProviders, newFakeCertManagerClient(tt.certManagerImages, tt.certManagerImagesErr))
		if tt.field.client == nil {
			tt.field.client = fc
		}

		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := tt.field.client.InitImages(ctx, InitOptions{
				Kubeconfig:              Kubeconfig{Path: "kubeconfig", Context: tt.args.kubeconfigContext},
				CoreProvider:            tt.args.coreProvider,
				BootstrapProviders:      tt.args.bootstrapProvider,
				ControlPlaneProviders:   tt.args.controlPlaneProvider,
				InfrastructureProviders: tt.args.infrastructureProvider,
			})

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				if tt.expectedErrorMessage == "" {
					return
				}
				g.Expect(err.Error()).To(ContainSubstring(tt.expectedErrorMessage))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(HaveLen(len(tt.expectedImages)))
			g.Expect(got).To(ConsistOf(tt.expectedImages))
		})
	}
}

func Test_clusterctlClient_Init(t *testing.T) {
	// create a config variables client which does not have the value for
	// SOME_VARIABLE as expected in the infra components YAML
	fconfig := newFakeConfig(ctx).
		WithVar("ANOTHER_VARIABLE", "value").
		WithProvider(capiProviderConfig).
		WithProvider(infraProviderConfig)
	frepositories := fakeRepositories(fconfig, nil)
	fcluster := fakeCluster(fconfig, frepositories, newFakeCertManagerClient(nil, nil))
	fclient := fakeClusterCtlClient(fconfig, frepositories, []*fakeClusterClient{fcluster})

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
	}
	type want struct {
		provider        Provider
		version         string
		targetNamespace string
	}

	tests := []struct {
		name    string
		field   field
		args    args
		want    []want
		wantErr bool
	}{
		{
			name: "returns error if variables are not available",
			field: field{
				client: fclient,
			},
			args: args{
				coreProvider:           "",  // with an empty cluster, a core provider should be added automatically
				bootstrapProvider:      nil, // with an empty cluster, a bootstrap provider should be added automatically
				controlPlaneProvider:   nil, // with an empty cluster, a control plane provider should be added automatically
				infrastructureProvider: []string{"infra"},
			},
			wantErr: true,
		},
		{
			name: "Init (with an empty cluster) with default provider versions/current contract",
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
			},
			want: []want{
				{
					provider:        capiProviderConfig,
					version:         "v1.0.0",
					targetNamespace: "ns1",
				},
				{
					provider:        bootstrapProviderConfig,
					version:         "v2.0.0",
					targetNamespace: "ns2",
				},
				{
					provider:        controlPlaneProviderConfig,
					version:         "v2.0.0",
					targetNamespace: "ns3",
				},
				{
					provider:        infraProviderConfig,
					version:         "v3.0.0",
					targetNamespace: "ns4",
				},
			},
			wantErr: false,
		},
		{
			name: "Init (with an empty cluster) opting out from automatic install of providers/current contract",
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
			},
			want: []want{
				{
					provider:        capiProviderConfig,
					version:         "v1.0.0",
					targetNamespace: "ns1",
				},
				{
					provider:        infraProviderConfig,
					version:         "v3.0.0",
					targetNamespace: "ns4",
				},
			},
			wantErr: false,
		},
		{
			name: "Init (with an empty cluster) with custom provider versions/current contract",
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
			},
			want: []want{
				{
					provider:        capiProviderConfig,
					version:         "v1.1.0",
					targetNamespace: "ns1",
				},
				{
					provider:        bootstrapProviderConfig,
					version:         "v2.1.0",
					targetNamespace: "ns2",
				},
				{
					provider:        controlPlaneProviderConfig,
					version:         "v2.1.0",
					targetNamespace: "ns3",
				},
				{
					provider:        infraProviderConfig,
					version:         "v3.1.0",
					targetNamespace: "ns4",
				},
			},
			wantErr: false,
		},
		{
			name: "Init (with an empty cluster) with target namespace/current contract",
			field: field{
				client: fakeEmptyCluster(), // clusterctl client for an empty management cluster (with repository setup for capi, bootstrap, control plane and infra provider)
				hasCRD: false,
			},
			args: args{
				coreProvider:           "", // with an empty cluster, a core provider should be added automatically
				bootstrapProvider:      []string{config.KubeadmBootstrapProviderName},
				infrastructureProvider: []string{"infra"},
				targetNameSpace:        "nsx",
			},
			want: []want{
				{
					provider:        capiProviderConfig,
					version:         "v1.0.0",
					targetNamespace: "nsx",
				},
				{
					provider:        bootstrapProviderConfig,
					version:         "v2.0.0",
					targetNamespace: "nsx",
				},
				{
					provider:        controlPlaneProviderConfig,
					version:         "v2.0.0",
					targetNamespace: "nsx",
				},
				{
					provider:        infraProviderConfig,
					version:         "v3.0.0",
					targetNamespace: "nsx",
				},
			},
			wantErr: false,
		},
		{
			name: "Init (with a NOT empty cluster) adds a provider/current contract",
			field: field{
				client: fakeInitializedCluster(), // clusterctl client for an management cluster with capi installed (with repository setup for capi, bootstrap, control plane and infra provider)
				hasCRD: true,
			},
			args: args{
				coreProvider:           "", // with a NOT empty cluster, a core provider should NOT be added automatically
				bootstrapProvider:      []string{config.KubeadmBootstrapProviderName},
				infrastructureProvider: []string{"infra"},
				targetNameSpace:        "",
			},
			want: []want{
				{
					provider:        bootstrapProviderConfig,
					version:         "v2.0.0",
					targetNamespace: "ns2",
				},
				{
					provider:        infraProviderConfig,
					version:         "v3.0.0",
					targetNamespace: "ns4",
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
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Init (with an empty cluster) with custom provider versions/previous contract, not supported",
			field: field{
				client: fakeEmptyCluster(), // clusterctl client for an empty management cluster (with repository setup for capi, bootstrap, control plane and infra provider)
				hasCRD: false,
			},
			args: args{
				coreProvider:           fmt.Sprintf("%s:v0.9.0", config.ClusterAPIProviderName),
				bootstrapProvider:      []string{fmt.Sprintf("%s:v0.9.0", config.KubeadmBootstrapProviderName)},
				controlPlaneProvider:   []string{fmt.Sprintf("%s:v0.9.0", config.KubeadmControlPlaneProviderName)},
				infrastructureProvider: []string{"infra:v0.9.0"},
				targetNameSpace:        "",
			},
			wantErr: true,
		},
		{
			name: "Init (with a NOT empty cluster) adds a provider/previous contract, not supported",
			field: field{
				client: fakeInitializedCluster(), // clusterctl client for an management cluster with capi installed (with repository setup for capi, bootstrap, control plane and infra provider)
				hasCRD: true,
			},
			args: args{
				coreProvider:           "", // with a NOT empty cluster, a core provider should NOT be added automatically
				bootstrapProvider:      []string{fmt.Sprintf("%s:v0.9.0", config.KubeadmBootstrapProviderName)},
				infrastructureProvider: []string{"infra:v0.9.0"},
				targetNameSpace:        "",
			},
			wantErr: true,
		},
		{
			name: "Init (with an NOT empty cluster) adds the same core providers version again - should ignore duplicate",
			field: field{
				client: fakeClusterWithCoreProvider(), // clusterctl client for an management cluster with CoreProvider cluster-api already installed.
				hasCRD: true,
			},
			args: args{
				coreProvider:           "cluster-api:v1.0.0", // core provider of the same version is already installed on the cluster. should be skipped.
				bootstrapProvider:      []string{},
				infrastructureProvider: []string{"infra"},
				targetNameSpace:        "",
			},
			want: []want{
				// Only the infra provider should be installed. Core provider should be skipped.
				{
					provider:        infraProviderConfig,
					version:         "v3.0.0",
					targetNamespace: "ns4",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			if tt.field.hasCRD {
				input := cluster.Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"}
				g.Expect(tt.field.client.clusters[input].ProviderInventory().EnsureCustomResourceDefinitions(ctx)).To(Succeed())
			}

			got, err := tt.field.client.Init(ctx, InitOptions{
				Kubeconfig:              Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
				CoreProvider:            tt.args.coreProvider,
				BootstrapProviders:      tt.args.bootstrapProvider,
				ControlPlaneProviders:   tt.args.controlPlaneProvider,
				InfrastructureProviders: tt.args.infrastructureProvider,
				TargetNamespace:         tt.args.targetNameSpace,
			})
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(HaveLen(len(tt.want)))
			for i, gItem := range got {
				w := tt.want[i]
				g.Expect(gItem.Name()).To(Equal(w.provider.Name()))
				g.Expect(gItem.Type()).To(Equal(w.provider.Type()))
				g.Expect(gItem.Version()).To(Equal(w.version))
				g.Expect(gItem.TargetNamespace()).To(Equal(w.targetNamespace))
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

// setup a cluster client and the fake configuration for testing.
func setupCluster(providers []Provider, certManagerClient cluster.CertManagerClient) (*fakeConfigClient, *fakeClient) {
	// create a config variables client which does not have the value for
	// SOME_VARIABLE as expected in the infra components YAML
	cfg := newFakeConfig(ctx).
		WithVar("ANOTHER_VARIABLE", "value").
		WithProvider(capiProviderConfig).
		WithProvider(infraProviderConfig)

	for _, provider := range providers {
		cfg.WithProvider(provider)
	}

	frepositories := fakeRepositories(cfg, providers)
	cluster := fakeCluster(cfg, frepositories, certManagerClient)
	fc := fakeClusterCtlClient(cfg, frepositories, []*fakeClusterClient{cluster})
	return cfg, fc
}

// clusterctl client for an empty management cluster (with repository setup for capi, bootstrap and infra provider).
func fakeEmptyCluster() *fakeClient {
	// create a config variables client which contains the value for the
	// variable required
	config1 := fakeConfig(
		[]config.Provider{capiProviderConfig, bootstrapProviderConfig, controlPlaneProviderConfig, infraProviderConfig},
		map[string]string{"SOME_VARIABLE": "value"},
	)

	// fake repository for capi, bootstrap and infra provider (matching provider's config)
	repositories := fakeRepositories(config1, nil)
	// fake empty cluster from fake repository for capi, bootstrap and infra
	// provider (matching provider's config)
	cluster1 := fakeCluster(config1, repositories, newFakeCertManagerClient(nil, nil))

	client := fakeClusterCtlClient(config1, repositories, []*fakeClusterClient{cluster1})
	return client
}

func fakeConfig(providers []config.Provider, variables map[string]string) *fakeConfigClient {
	config := newFakeConfig(ctx)
	for _, p := range providers {
		config = config.WithProvider(p)
	}
	for k, v := range variables {
		config = config.WithVar(k, v)
	}
	return config
}

func fakeCluster(config *fakeConfigClient, repos []*fakeRepositoryClient, certManagerClient cluster.CertManagerClient) *fakeClusterClient {
	cluster := newFakeCluster(cluster.Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"}, config)
	for _, r := range repos {
		cluster = cluster.WithRepository(r)
	}
	cluster.WithCertManagerClient(certManagerClient)
	return cluster
}

// fakeRepositories returns a base set of repositories for the different types
// of providers.
func fakeRepositories(config *fakeConfigClient, providers []Provider) []*fakeRepositoryClient {
	repository1 := newFakeRepository(ctx, capiProviderConfig, config).
		WithPaths("root", "components.yaml").
		WithDefaultVersion("v1.0.0").
		WithFile("v0.9.0", "components.yaml", componentsYAML("ns1")).
		WithMetadata("v0.9.0", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 0, Minor: 9, Contract: test.PreviousCAPIContractNotSupported},
			},
		}).
		WithFile("v1.0.0", "components.yaml", componentsYAML("ns1")).
		WithMetadata("v1.0.0", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 0, Minor: 9, Contract: test.PreviousCAPIContractNotSupported},
				{Major: 1, Minor: 0, Contract: test.CurrentCAPIContract},
			},
		}).
		WithFile("v1.1.0", "components.yaml", componentsYAML("ns1")).
		WithMetadata("v1.1.0", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 0, Minor: 9, Contract: test.PreviousCAPIContractNotSupported},
				{Major: 1, Minor: 1, Contract: test.CurrentCAPIContract},
			},
		})
	repository2 := newFakeRepository(ctx, bootstrapProviderConfig, config).
		WithPaths("root", "components.yaml").
		WithDefaultVersion("v2.0.0").
		WithFile("v0.9.0", "components.yaml", componentsYAML("ns1")).
		WithMetadata("v0.9.0", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 0, Minor: 9, Contract: test.PreviousCAPIContractNotSupported},
			},
		}).
		WithFile("v2.0.0", "components.yaml", componentsYAML("ns2")).
		WithMetadata("v2.0.0", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 0, Minor: 9, Contract: test.PreviousCAPIContractNotSupported},
				{Major: 2, Minor: 0, Contract: test.CurrentCAPIContract},
			},
		}).
		WithFile("v2.1.0", "components.yaml", componentsYAML("ns2")).
		WithMetadata("v2.1.0", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 0, Minor: 9, Contract: test.PreviousCAPIContractNotSupported},
				{Major: 2, Minor: 1, Contract: test.CurrentCAPIContract},
			},
		})
	repository3 := newFakeRepository(ctx, controlPlaneProviderConfig, config).
		WithPaths("root", "components.yaml").
		WithDefaultVersion("v2.0.0").
		WithFile("v0.9.0", "components.yaml", componentsYAML("ns1")).
		WithMetadata("v0.9.0", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 0, Minor: 9, Contract: test.PreviousCAPIContractNotSupported},
			},
		}).
		WithFile("v2.0.0", "components.yaml", componentsYAML("ns3")).
		WithMetadata("v2.0.0", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 0, Minor: 9, Contract: test.PreviousCAPIContractNotSupported},
				{Major: 2, Minor: 0, Contract: test.CurrentCAPIContract},
			},
		}).
		WithFile("v2.1.0", "components.yaml", componentsYAML("ns3")).
		WithMetadata("v2.1.0", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 0, Minor: 9, Contract: test.PreviousCAPIContractNotSupported},
				{Major: 2, Minor: 1, Contract: test.CurrentCAPIContract},
			},
		})
	repository4 := newFakeRepository(ctx, infraProviderConfig, config).
		WithPaths("root", "components.yaml").
		WithDefaultVersion("v3.0.0").
		WithFile("v0.9.0", "components.yaml", componentsYAML("ns1")).
		WithMetadata("v0.9.0", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 0, Minor: 9, Contract: test.PreviousCAPIContractNotSupported},
			},
		}).
		WithFile("v3.0.0", "components.yaml", infraComponentsYAML("ns4")).
		WithMetadata("v3.0.0", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 0, Minor: 9, Contract: test.PreviousCAPIContractNotSupported},
				{Major: 3, Minor: 0, Contract: test.CurrentCAPIContract},
			},
		}).
		WithFile("v3.1.0", "components.yaml", infraComponentsYAML("ns4")).
		WithMetadata("v3.1.0", &clusterctlv1.Metadata{
			ReleaseSeries: []clusterctlv1.ReleaseSeries{
				{Major: 0, Minor: 9, Contract: test.PreviousCAPIContractNotSupported},
				{Major: 3, Minor: 1, Contract: test.CurrentCAPIContract},
			},
		}).
		WithFile("v3.0.0", "cluster-template.yaml", templateYAML("ns4", "test"))

	var providerRepositories = []*fakeRepositoryClient{repository1, repository2, repository3, repository4}

	for _, provider := range providers {
		providerRepositories = append(providerRepositories,
			newFakeRepository(ctx, provider, config).
				WithPaths("root", "components.yaml").
				WithDefaultVersion("v2.0.0").
				WithFile("v2.0.0", "components.yaml", componentsYAML("ns2")).
				WithMetadata("v2.0.0", &clusterctlv1.Metadata{
					ReleaseSeries: []clusterctlv1.ReleaseSeries{
						{Major: 2, Minor: 0, Contract: test.CurrentCAPIContract},
					},
				}))
	}

	return providerRepositories
}

func fakeClusterCtlClient(config *fakeConfigClient, repos []*fakeRepositoryClient, clusters []*fakeClusterClient) *fakeClient {
	client := newFakeClient(ctx, config)
	for _, r := range repos {
		client = client.WithRepository(r)
	}
	for _, c := range clusters {
		client = client.WithCluster(c)
	}
	return client
}

// clusterctl client for a management cluster with capi installed (with repository setup for capi, bootstrap and infra provider)
// It references a cluster client that corresponds to the mgmt-context in the
// kubeconfig file.
func fakeInitializedCluster() *fakeClient {
	client := fakeEmptyCluster()

	input := cluster.Kubeconfig{
		Path:    "kubeconfig",
		Context: "mgmt-context",
	}
	p := client.clusters[input].Proxy()
	fp := p.(*test.FakeProxy)

	fp.WithProviderInventory(capiProviderConfig.Name(), capiProviderConfig.Type(), "v1.0.0", "capi-system")

	return client
}

func fakeClusterWithCoreProvider() *fakeClient {
	client := fakeEmptyCluster()

	input := cluster.Kubeconfig{
		Path:    "kubeconfig",
		Context: "mgmt-context",
	}
	p := client.clusters[input].Proxy()
	fp := p.(*test.FakeProxy)

	fp.WithProviderInventory(capiProviderConfig.Name(), capiProviderConfig.Type(), "v1.0.0", "ns1")

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

	return utilyaml.JoinYaml(namespaceYaml, podYaml)
}

func templateYAML(ns string, clusterName string) []byte {
	var podYaml = []byte("apiVersion: v1\n" +
		"kind: Cluster\n" +
		"metadata:\n" +
		fmt.Sprintf("  name: %s\n", clusterName) +
		fmt.Sprintf("  namespace: %s", ns))

	return podYaml
}

func mangedTopologyTemplateYAML(ns, clusterName, clusterClassName string) []byte {
	return []byte(fmt.Sprintf("apiVersion: %s\n", clusterv1.GroupVersion.String()) +
		"kind: Cluster\n" +
		"metadata:\n" +
		fmt.Sprintf("  name: %s\n", clusterName) +
		fmt.Sprintf("  namespace: %s\n", ns) +
		"spec:\n" +
		"  topology:\n" +
		fmt.Sprintf("    class: %s", clusterClassName))
}

func clusterClassYAML(ns, clusterClassName string) []byte {
	var podYaml = []byte(fmt.Sprintf("apiVersion: %s\n", clusterv1.GroupVersion.String()) +
		"kind: ClusterClass\n" +
		"metadata:\n" +
		fmt.Sprintf("  name: %s\n", clusterClassName) +
		fmt.Sprintf("  namespace: %s", ns))

	return podYaml
}

// infraComponentsYAML defines a namespace and deployment with container
// images and a variable.
func infraComponentsYAML(namespace string) []byte {
	var infraComponentsYAML = `---
apiVersion: v1
kind: Namespace
metadata:
  name: %[1]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: capa-controller-manager
  namespace: %[1]s
spec:
  template:
    spec:
      containers:
      - image: registry.k8s.io/cluster-api-aws/cluster-api-aws-controller:v0.5.3
        name: manager
        volumeMounts:
        - mountPath: /home/.aws
          name: credentials
      volumes:
      - name: credentials
        secret:
          secretName: ${SOME_VARIABLE}
`
	return []byte(fmt.Sprintf(infraComponentsYAML, namespace))
}
