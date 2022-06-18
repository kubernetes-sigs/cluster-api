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
	objects := test.FakeCAPISetupObjects()
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
		want      string
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
		{
			name: "returns the default user kubeconfig",
			client: func() *fakeClient {
				newclusterClient := newFakeCluster(kubeconfig, configClient).
					WithObjs(append(objects, getKubeconfigSecret(true))...)
				return newFakeClient(configClient).WithCluster(newclusterClient)
			}(),
			options: GetKubeconfigOptions{
				Kubeconfig:          Kubeconfig(kubeconfig),
				Namespace:           "default",
				WorkloadClusterName: "mgmt-cluster",
				UserKubeconfig:      true,
			},
			expectErr: false,
			want:      getKubeconfig("mgmt-cluster", true),
		},
		{
			name: "returns the system kubeconfig when the secret for user kubeconfig is not available",
			client: func() *fakeClient {
				newclusterClient := newFakeCluster(kubeconfig, configClient).
					WithObjs(append(objects, getKubeconfigSecret(false))...)
				return newFakeClient(configClient).WithCluster(newclusterClient)
			}(),
			options: GetKubeconfigOptions{
				Kubeconfig:          Kubeconfig(kubeconfig),
				Namespace:           "default",
				WorkloadClusterName: "mgmt-cluster",
				UserKubeconfig:      true,
			},
			expectErr: false,
			want:      getKubeconfig("mgmt-cluster", false),
		},
		{
			name: "returns the system kubeconfig when GetKubeconfigOptions.UserKubeconfig is set to false",
			client: func() *fakeClient {
				newclusterClient := newFakeCluster(kubeconfig, configClient).
					WithObjs(append(objects, getKubeconfigSecret(false))...)
				return newFakeClient(configClient).WithCluster(newclusterClient)
			}(),
			options: GetKubeconfigOptions{
				Kubeconfig:          Kubeconfig(kubeconfig),
				Namespace:           "default",
				WorkloadClusterName: "mgmt-cluster",
				UserKubeconfig:      false,
			},
			expectErr: false,
			want:      getKubeconfig("mgmt-cluster", false),
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
			g.Expect(config).To(BeEquivalentTo(tt.want))
		})
	}
}

//getKubeconfig returns a user/system specific kubeconfig with given cluster name
func getKubeconfig(clusterName string, isUser bool) string {
	var username string
	if isUser {
		username += fmt.Sprintf("%s-user", clusterName)
	} else {
		username += fmt.Sprintf("%s-admin", clusterName)
	}
	return fmt.Sprintf(`
apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: randomstring
    server: https://test-cluster-api.us-east-1.eks.amazonaws.com
  name: mgmt-cluster
contexts:
- context:
    cluster: mgmt-cluster
    user: %[1]s
  name: mgmt-cluster
current-context: mgmt-cluster
preferences: {}
users
- name: %[1]s
  user:
    token: sometoken
`, username)
}

//getKubeconfigSecret returns user/system specific kubeconfig secret
func getKubeconfigSecret(isUser bool) *corev1.Secret {
	clusterName := "mgmt-cluster"
	var secretName string
	if isUser {
		secretName = fmt.Sprintf("%s-user-kubeconfig", clusterName)
	} else {
		secretName = fmt.Sprintf("%s-kubeconfig", clusterName)
	}
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: "default",
			Labels:    map[string]string{clusterv1.ClusterLabelName: clusterName},
		},
		Data: map[string][]byte{
			secret.KubeconfigDataName: []byte(getKubeconfig(clusterName, isUser)),
		},
		Type: clusterv1.ClusterSecretType,
	}
}
