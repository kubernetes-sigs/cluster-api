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
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
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
					Kubeconfig:           "kubeconfig",
					ForceDeleteNamespace: false,
					ForceDeleteCRD:       false,
					Namespace:            "",
					Providers:            nil, // nil means all the providers
				},
			},
			wantProviders: sets.NewString(),
			wantErr:       false,
		},
		{
			name: "Delete single provider",
			fields: fields{
				client: fakeClusterForDelete(),
			},
			args: args{
				options: DeleteOptions{
					Kubeconfig:           "kubeconfig",
					ForceDeleteNamespace: false,
					ForceDeleteCRD:       false,
					Namespace:            "capbpk-system",
					Providers:            []string{bootstrapProviderConfig.Name()},
				},
			},
			wantProviders: sets.NewString(capiProviderConfig.Name()),
			wantErr:       false,
		},
		{
			name: "Delete single provider auto-detect namespace",
			fields: fields{
				client: fakeClusterForDelete(),
			},
			args: args{
				options: DeleteOptions{
					Kubeconfig:           "kubeconfig",
					ForceDeleteNamespace: false,
					ForceDeleteCRD:       false,
					Namespace:            "", // empty namespace triggers namespace auto detection
					Providers:            []string{bootstrapProviderConfig.Name()},
				},
			},
			wantProviders: sets.NewString(capiProviderConfig.Name()),
			wantErr:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fields.client.Delete(tt.args.options); (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			proxy := tt.fields.client.clusters["kubeconfig"].Proxy()
			gotProviders := &clusterctlv1.ProviderList{}

			c, err := proxy.NewClient()
			if err != nil {
				t.Fatalf("failed to create client %v", err)
			}

			if err := c.List(context.TODO(), gotProviders); err != nil {
				t.Fatalf("failed to read providers %v", err)
			}

			gotProvidersSet := sets.NewString()
			for _, gotProvider := range gotProviders.Items {
				gotProvidersSet.Insert(gotProvider.Name)
			}

			if !reflect.DeepEqual(gotProvidersSet, tt.wantProviders) {
				t.Errorf("got = %v providers, want %v", len(gotProviders.Items), len(tt.wantProviders))
			}
		})
	}
}

// clusterctl client for a management cluster with capi and bootstrap provider
func fakeClusterForDelete() *fakeClient {
	config1 := newFakeConfig().
		WithVar("var", "value").
		WithProvider(capiProviderConfig).
		WithProvider(bootstrapProviderConfig)

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

	cluster1 := newFakeCluster("kubeconfig", config1)
	cluster1.fakeProxy.WithProviderInventory(capiProviderConfig.Name(), capiProviderConfig.Type(), "v1.0.0", "capi-system", "")
	cluster1.fakeProxy.WithProviderInventory(bootstrapProviderConfig.Name(), bootstrapProviderConfig.Type(), "v1.0.0", "capbpk-system", "")

	client := newFakeClient(config1).
		// fake repository for capi, bootstrap and infra provider (matching provider's config)
		WithRepository(repository1).
		WithRepository(repository2).
		// fake empty cluster
		WithCluster(cluster1)

	return client
}
