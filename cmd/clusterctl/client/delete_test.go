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
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/sets"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

var namespace = "foobar"

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
		wantProviders sets.String
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
			wantProviders: sets.NewString(),
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
			wantProviders: sets.NewString(
				capiProviderConfig.Name(),
				clusterctlv1.ManifestLabel(controlPlaneProviderConfig.Name(), controlPlaneProviderConfig.Type()),
				clusterctlv1.ManifestLabel(infraProviderConfig.Name(), infraProviderConfig.Type())),
			wantErr: false,
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
			wantProviders: sets.NewString(
				clusterctlv1.ManifestLabel(controlPlaneProviderConfig.Name(), controlPlaneProviderConfig.Type()),
				clusterctlv1.ManifestLabel(infraProviderConfig.Name(), infraProviderConfig.Type())),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := tt.fields.client.Delete(tt.args.options)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			input := cluster.Kubeconfig(tt.args.options.Kubeconfig)
			proxy := tt.fields.client.clusters[input].Proxy()
			gotProviders := &clusterctlv1.ProviderList{}

			c, err := proxy.NewClient()
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(c.List(ctx, gotProviders)).To(Succeed())

			gotProvidersSet := sets.NewString()
			for _, gotProvider := range gotProviders.Items {
				gotProvidersSet.Insert(gotProvider.Name)
			}

			g.Expect(gotProvidersSet).To(Equal(tt.wantProviders))
		})
	}
}

// clusterctl client for a management cluster with capi and bootstrap provider.
func fakeClusterForDelete() *fakeClient {
	config1 := newFakeConfig().
		WithVar("var", "value").
		WithProvider(capiProviderConfig).
		WithProvider(bootstrapProviderConfig).
		WithProvider(controlPlaneProviderConfig).
		WithProvider(infraProviderConfig)

	repository1 := newFakeRepository(capiProviderConfig, config1)
	repository2 := newFakeRepository(bootstrapProviderConfig, config1)
	repository3 := newFakeRepository(controlPlaneProviderConfig, config1)
	repository4 := newFakeRepository(infraProviderConfig, config1)

	cluster1 := newFakeCluster(cluster.Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"}, config1)
	cluster1.fakeProxy.WithProviderInventory(capiProviderConfig.Name(), capiProviderConfig.Type(), "v1.0.0", "capi-system")
	cluster1.fakeProxy.WithProviderInventory(bootstrapProviderConfig.Name(), bootstrapProviderConfig.Type(), "v1.0.0", "capbpk-system")
	cluster1.fakeProxy.WithProviderInventory(controlPlaneProviderConfig.Name(), controlPlaneProviderConfig.Type(), "v1.0.0", namespace)
	cluster1.fakeProxy.WithProviderInventory(infraProviderConfig.Name(), infraProviderConfig.Type(), "v1.0.0", namespace)
	cluster1.fakeProxy.WithFakeCAPISetup()

	client := newFakeClient(config1).
		// fake repository for capi, bootstrap, controlplane and infra provider (matching provider's config)
		WithRepository(repository1).
		WithRepository(repository2).
		WithRepository(repository3).
		WithRepository(repository4).
		// fake empty cluster
		WithCluster(cluster1)

	return client
}
