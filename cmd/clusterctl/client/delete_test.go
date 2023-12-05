/*
Copyright 2020 The Kubernetes Authors.

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
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/sets"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

const (
	namespace       = "foobar"
	providerVersion = "v1.0.0"
)

func Test_clusterctlClient_Delete(t *testing.T) {
	type fields struct {
		client *fakeClient
	}
	type args struct {
		options DeleteOptions
	}
	tests := []struct {
		name          string
		fields        fields
		args          args
		wantProviders sets.Set[string]
		wantErr       bool
	}{
		{
			name: "Delete all the providers",
			fields: fields{
				client: fakeClusterForDelete(),
			},
			args: args{
				options: DeleteOptions{
					Kubeconfig:              Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					IncludeNamespace:        false,
					IncludeCRDs:             false,
					SkipInventory:           false,
					CoreProvider:            "",
					BootstrapProviders:      nil,
					InfrastructureProviders: nil,
					ControlPlaneProviders:   nil,
					DeleteAll:               true, // delete all the providers
				},
			},
			wantProviders: sets.Set[string]{},
			wantErr:       false,
		},
		{
			name: "Delete all the providers including CRDs",
			fields: fields{
				client: fakeClusterForDelete(),
			},
			args: args{
				options: DeleteOptions{
					Kubeconfig:              Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					IncludeNamespace:        false,
					IncludeCRDs:             true,
					SkipInventory:           false,
					CoreProvider:            "",
					BootstrapProviders:      nil,
					InfrastructureProviders: nil,
					ControlPlaneProviders:   nil,
					DeleteAll:               true, // delete all the providers
				},
			},
			wantProviders: sets.Set[string]{},
			wantErr:       false,
		},
		{
			name: "Delete single provider auto-detect namespace",
			fields: fields{
				client: fakeClusterForDelete(),
			},
			args: args{
				options: DeleteOptions{
					Kubeconfig:              Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					IncludeNamespace:        false,
					IncludeCRDs:             false,
					SkipInventory:           false,
					CoreProvider:            "",
					BootstrapProviders:      []string{bootstrapProviderConfig.Name()},
					InfrastructureProviders: nil,
					ControlPlaneProviders:   nil,
					DeleteAll:               false,
				},
			},
			wantProviders: sets.Set[string]{}.Insert(
				capiProviderConfig.Name(),
				clusterctlv1.ManifestLabel(controlPlaneProviderConfig.Name(), controlPlaneProviderConfig.Type()),
				clusterctlv1.ManifestLabel(infraProviderConfig.Name(), infraProviderConfig.Type())),
			wantErr: false,
		},
		{
			name: "Delete a provider with version mentioned",
			fields: fields{
				client: fakeClusterForDelete(),
			},
			args: args{
				options: DeleteOptions{
					Kubeconfig:              Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					IncludeNamespace:        false,
					IncludeCRDs:             false,
					SkipInventory:           false,
					CoreProvider:            "",
					BootstrapProviders:      nil,
					InfrastructureProviders: []string{infraProviderConfig.Name() + ":" + providerVersion},
					ControlPlaneProviders:   nil,
					DeleteAll:               false,
				},
			},
			wantProviders: sets.Set[string]{}.Insert(
				capiProviderConfig.Name(),
				clusterctlv1.ManifestLabel(bootstrapProviderConfig.Name(), bootstrapProviderConfig.Type()),
				clusterctlv1.ManifestLabel(controlPlaneProviderConfig.Name(), controlPlaneProviderConfig.Type())),
			wantErr: false,
		},
		{
			name: "Delete a provider with wrong version",
			fields: fields{
				client: fakeClusterForDelete(),
			},
			args: args{
				options: DeleteOptions{
					Kubeconfig:              Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					IncludeNamespace:        false,
					IncludeCRDs:             false,
					SkipInventory:           false,
					CoreProvider:            "",
					BootstrapProviders:      nil,
					InfrastructureProviders: []string{infraProviderConfig.Name() + ":v99.99.99"},
					ControlPlaneProviders:   nil,
					DeleteAll:               false,
				},
			},
			wantProviders: sets.Set[string]{}.Insert(
				capiProviderConfig.Name(),
				clusterctlv1.ManifestLabel(bootstrapProviderConfig.Name(), bootstrapProviderConfig.Type()),
				clusterctlv1.ManifestLabel(controlPlaneProviderConfig.Name(), controlPlaneProviderConfig.Type()),
				clusterctlv1.ManifestLabel(infraProviderConfig.Name(), infraProviderConfig.Type())),
			wantErr: true,
		},
		{
			name: "Delete multiple providers of different type",
			fields: fields{
				client: fakeClusterForDelete(),
			},
			args: args{
				options: DeleteOptions{
					Kubeconfig:              Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"},
					IncludeNamespace:        false,
					IncludeCRDs:             false,
					SkipInventory:           false,
					CoreProvider:            capiProviderConfig.Name(),
					BootstrapProviders:      []string{bootstrapProviderConfig.Name()},
					InfrastructureProviders: nil,
					ControlPlaneProviders:   nil,
					DeleteAll:               false,
				},
			},
			wantProviders: sets.Set[string]{}.Insert(
				clusterctlv1.ManifestLabel(controlPlaneProviderConfig.Name(), controlPlaneProviderConfig.Type()),
				clusterctlv1.ManifestLabel(infraProviderConfig.Name(), infraProviderConfig.Type())),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			err := tt.fields.client.Delete(ctx, tt.args.options)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			input := cluster.Kubeconfig(tt.args.options.Kubeconfig)
			proxy := tt.fields.client.clusters[input].Proxy()
			gotProviders := &clusterctlv1.ProviderList{}

			c, err := proxy.NewClient(ctx)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(c.List(ctx, gotProviders)).To(Succeed())

			gotProvidersSet := sets.Set[string]{}
			for _, gotProvider := range gotProviders.Items {
				gotProvidersSet.Insert(gotProvider.Name)
			}

			g.Expect(gotProvidersSet).To(BeComparableTo(tt.wantProviders))
		})
	}
}

// clusterctl client for a management cluster with capi and bootstrap provider.
func fakeClusterForDelete() *fakeClient {
	ctx := context.Background()

	config1 := newFakeConfig(ctx).
		WithVar("var", "value").
		WithProvider(capiProviderConfig).
		WithProvider(bootstrapProviderConfig).
		WithProvider(controlPlaneProviderConfig).
		WithProvider(infraProviderConfig)

	repository1 := newFakeRepository(ctx, capiProviderConfig, config1)
	repository2 := newFakeRepository(ctx, bootstrapProviderConfig, config1)
	repository3 := newFakeRepository(ctx, controlPlaneProviderConfig, config1)
	repository4 := newFakeRepository(ctx, infraProviderConfig, config1)

	cluster1 := newFakeCluster(cluster.Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"}, config1)
	cluster1.fakeProxy.WithProviderInventory(capiProviderConfig.Name(), capiProviderConfig.Type(), providerVersion, "capi-system")
	cluster1.fakeProxy.WithProviderInventory(bootstrapProviderConfig.Name(), bootstrapProviderConfig.Type(), providerVersion, "capi-kubeadm-bootstrap-system")
	cluster1.fakeProxy.WithProviderInventory(controlPlaneProviderConfig.Name(), controlPlaneProviderConfig.Type(), providerVersion, "capi-kubeadm-control-plane-system")
	cluster1.fakeProxy.WithProviderInventory(infraProviderConfig.Name(), infraProviderConfig.Type(), providerVersion, namespace)
	cluster1.fakeProxy.WithFakeCAPISetup()

	client := newFakeClient(ctx, config1).
		// fake repository for capi, bootstrap, controlplane and infra provider (matching provider's config)
		WithRepository(repository1).
		WithRepository(repository2).
		WithRepository(repository3).
		WithRepository(repository4).
		// fake empty cluster
		WithCluster(cluster1)

	return client
}
