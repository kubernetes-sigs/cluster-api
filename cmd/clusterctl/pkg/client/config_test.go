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

	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
)

func Test_clusterctlClient_GetProvidersConfig(t *testing.T) {
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
				"kubeadm",
				"vsphere",
			},
			wantErr: false,
		},
		{
			name: "Returns default providers and custom providers if defined",
			field: field{
				client: newFakeClient(newFakeConfig().WithProvider(bootstrapProviderConfig)),
			},
			wantProviders: []string{
				"aws",
				bootstrapProviderConfig.Name(),
				config.ClusterAPIName,
				"docker",
				"kubeadm",
				"vsphere",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.field.client.GetProvidersConfig()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetProvidersConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(got) != len(tt.wantProviders) {
				t.Errorf("Init() got = %v items, want %v items", len(got), len(tt.wantProviders))
				return
			}

			for i, g := range got {
				w := tt.wantProviders[i]

				if g.Name() != w {
					t.Errorf("GetProvidersConfig(), Item[%d].Name() got = %v, want = %v ", i, g.Name(), w)
				}
			}
		})
	}
}
