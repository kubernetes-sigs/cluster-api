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
)

func Test_clusterctlClient_GetProvidersConfig(t *testing.T) {
	customProviderConfig := config.NewProvider("custom", "url", clusterctlv1.BootstrapProviderType)

	type field struct {
		client Client
	}
	tests := []struct {
		name          string
		field         field
		wantProviders []string
		wantErr       bool
	}{
		{
			name: "Returns default providers",
			field: field{
				client: newFakeClient(newFakeConfig()),
			},
			wantProviders: []string{
				"aws",
				config.ClusterAPIName,
				"docker",
				config.KubeadmBootstrapProviderName,
				"vsphere",
			},
			wantErr: false,
		},
		{
			name: "Returns default providers and custom providers if defined",
			field: field{
				client: newFakeClient(newFakeConfig().WithProvider(customProviderConfig)),
			},
			wantProviders: []string{
				"aws",
				config.ClusterAPIName,
				customProviderConfig.Name(),
				"docker",
				config.KubeadmBootstrapProviderName,
				"vsphere",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.field.client.GetProvidersConfig()
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}

			if len(got) != len(tt.wantProviders) {
				t.Errorf("got = %v items, want %v items", len(got), len(tt.wantProviders))
				return
			}

			for i, g := range got {
				w := tt.wantProviders[i]

				if g.Name() != w {
					t.Errorf("Item[%d].Name() got = %v, want = %v ", i, g.Name(), w)
				}
			}
		})
	}
}

func Test_clusterctlClient_GetProviderComponents(t *testing.T) {
	config1 := newFakeConfig().
		WithProvider(capiProviderConfig)

	repository1 := newFakeRepository(capiProviderConfig, config1.Variables()).
		WithPaths("root", "components.yaml").
		WithDefaultVersion("v1.0.0").
		WithFile("v1.0.0", "components.yaml", componentsYAML("ns1"))

	client := newFakeClient(config1).
		WithRepository(repository1)

	type args struct {
		provider          string
		targetNameSpace   string
		watchingNamespace string
	}
	type want struct {
		provider config.Provider
		version  string
	}
	tests := []struct {
		name    string
		args    args
		want    want
		wantErr bool
	}{
		{
			name: "Pass",
			args: args{
				provider:          capiProviderConfig.Name(),
				targetNameSpace:   "ns2",
				watchingNamespace: "",
			},
			want: want{
				provider: capiProviderConfig,
				version:  "v1.0.0",
			},
			wantErr: false,
		},
		{
			name: "Fail",
			args: args{
				provider:          fmt.Sprintf("%s:v0.2.0", capiProviderConfig.Name()),
				targetNameSpace:   "ns2",
				watchingNamespace: "",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := client.GetProviderComponents(tt.args.provider, tt.args.targetNameSpace, tt.args.watchingNamespace)
			if tt.wantErr != (err != nil) {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if got.Name() != tt.want.provider.Name() {
				t.Errorf("got.Name() = %v, want = %v ", got.Name(), tt.want.provider.Name())
			}

			if got.Version() != tt.want.version {
				t.Errorf("got.Version() = %v, want = %v ", got.Version(), tt.want.version)
			}
		})
	}
}
