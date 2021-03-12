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

	. "github.com/onsi/gomega"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

func Test_clusterctlClient_GetKubeconfig(t *testing.T) {

	configClient := newFakeConfig()
	kubeconfig := cluster.Kubeconfig{Path: "kubeconfig", Context: "mgmt-context"}
	clusterClient := newFakeCluster(cluster.Kubeconfig{Path: "cluster1"}, configClient)

	// create a clusterctl client where the proxy returns an empty namespace
	clusterClient.fakeProxy = test.NewFakeProxy().WithNamespace("").WithFakeCAPISetup()
	badClient := newFakeClient(configClient).WithCluster(clusterClient)

	tests := []struct {
		name      string
		client    *fakeClient
		options   GetKubeconfigOptions
		expectErr bool
	}{
		{
			name:      "returns error if unable to get client for mgmt cluster",
			client:    fakeEmptyCluster(),
			expectErr: true,
		},
		{
			name:      "returns error if unable namespace is empty",
			client:    badClient,
			options:   GetKubeconfigOptions{Kubeconfig: Kubeconfig(kubeconfig)},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			config, err := tt.client.GetKubeconfig(tt.options)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(config).ToNot(BeEmpty())
		})
	}
}
