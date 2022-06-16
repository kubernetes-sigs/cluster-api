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

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/secret"

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
			options:   GetKubeconfigOptions{Kubeconfig: Kubeconfig(kubeconfig), UserKubeconfig: true},
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

const validUserKubeconfig = `
apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: randomstring
    server: https://test-cluster-api.us-east-1.eks.amazonaws.com
  name: default_mgmt-cluster-control-plane
contexts:
- context:
    cluster: default_mgmt-cluster-control-plane
    user: default_mgmt-cluster-control-plane-user
  name: default_mgmt-cluster-control-plane-user@default_mgmt-cluster-control-plane
current-context: default_mgmt-cluster-control-plane-user@default_mgmt-cluster-control-plane
preferences: {}
users
- name: default_mgmt-cluster-control-plane-user
  user:
    client-certificate-data: stuff-cert-data
    client-key-data: stuff-key-data
`

func Test_clusterctlClient_GetKubeconfig1(t *testing.T) {
	newconfigClient := newFakeConfig()
	newclusterClient := newFakeCluster(cluster.Kubeconfig{Path: "kubeconfig", Context: "default_mgmt-cluster-control-plane-user@default_mgmt-cluster-control-plane"}, newconfigClient)
	newclusterClient.fakeProxy = test.NewFakeProxy().WithObjs(&corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mgmt-cluster-user-kubeconfig",
			Namespace: "default",
			Labels:    map[string]string{clusterv1.ClusterLabelName: "mgmt-cluster-user"},
		},
		Data: map[string][]byte{
			secret.KubeconfigDataName: []byte(validUserKubeconfig),
		},
		Type: clusterv1.ClusterSecretType,
	}, &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiextensionsv1.SchemeGroupVersion.String(),
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("clusters.%s", clusterv1.GroupVersion.Group),
			Labels: map[string]string{
				fmt.Sprintf("clusterctl.%s", clusterv1.GroupVersion.String()): "",
				fmt.Sprintf("%s/provider", clusterv1.GroupVersion.String()):   "cluster-api",
			},
		},
	})
	client := newFakeClient(newconfigClient).WithCluster(newclusterClient)

	options := GetKubeconfigOptions{
		Kubeconfig:          Kubeconfig(cluster.Kubeconfig{Path: "kubeconfig", Context: "default_mgmt-cluster-control-plane-user@default_mgmt-cluster-control-plane"}),
		Namespace:           "default",
		WorkloadClusterName: "mgmt-cluster",
		UserKubeconfig:      true,
	}
	g := NewWithT(t)

	config, err := client.GetKubeconfig(options)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(config).ToNot(BeEmpty())
}
