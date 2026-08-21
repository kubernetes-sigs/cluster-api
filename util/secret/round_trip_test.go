/*
Copyright 2026 The Kubernetes Authors.

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

package secret

import "testing"

func TestParseSecretName_RoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		cluster string
		purpose Purpose
	}{
		{
			name:    "simple cluster + Kubeconfig",
			cluster: "my-cluster",
			purpose: Kubeconfig,
		},
		{
			name:    "cluster with hyphens + ClusterCA",
			cluster: "my-test-cluster",
			purpose: ClusterCA,
		},
		{
			name:    "APIServerEtcdClient with simple cluster",
			cluster: "cluster",
			purpose: APIServerEtcdClient,
		},
		{
			name:    "APIServerEtcdClient with cluster containing hyphens",
			cluster: "my-cluster",
			purpose: APIServerEtcdClient,
		},
		{
			name:    "cluster name ending with purpose string + Kubeconfig",
			cluster: "my-kubeconfig",
			purpose: Kubeconfig,
		},
		{
			name:    "simple cluster + EtcdCA",
			cluster: "my-cluster",
			purpose: EtcdCA,
		},
		{
			name:    "simple cluster + ServiceAccount",
			cluster: "my-cluster",
			purpose: ServiceAccount,
		},
		{
			name:    "simple cluster + FrontProxyCA",
			cluster: "my-cluster",
			purpose: FrontProxyCA,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName := Name(tt.cluster, tt.purpose)
			gotCluster, gotPurpose, err := ParseSecretName(gotName)
			if err != nil {
				t.Fatalf("ParseSecretName(%q) unexpected error: %v", gotName, err)
			}
			if gotCluster != tt.cluster {
				t.Errorf("ParseSecretName(%q) cluster = %q, want %q", gotName, gotCluster, tt.cluster)
			}
			if gotPurpose != tt.purpose {
				t.Errorf("ParseSecretName(%q) purpose = %q, want %q", gotName, gotPurpose, tt.purpose)
			}
		})
	}
}
