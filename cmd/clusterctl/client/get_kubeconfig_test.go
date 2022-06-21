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
	clusterName := "workload-cluster"
	configClient := newFakeConfig()
	objects := test.FakeCAPISetupObjects()
	kubeconfig := cluster.Kubeconfig{Path: "kubeconfig"}

	tests := []struct {
		name      string
		client    *fakeClient
		options   GetKubeconfigOptions
		expectErr bool
		want      string
	}{
		{
			name:      "returns error if unable to get client for workload cluster",
			client:    fakeEmptyCluster(),
			expectErr: true,
		},
		{
			name: "returns error if namespace is empty",
			client: func() *fakeClient {
				clusterClient := newFakeCluster(kubeconfig, configClient)

				// create a clusterctl client where the proxy returns an empty namespace
				clusterClient.fakeProxy = test.NewFakeProxy().WithNamespace("").WithFakeCAPISetup()
				return newFakeClient(configClient).WithCluster(clusterClient)
			}(),
			options:   GetKubeconfigOptions{Kubeconfig: Kubeconfig(kubeconfig), UserKubeconfig: true},
			expectErr: true,
		},
		{
			name: "returns the default user kubeconfig",
			client: func() *fakeClient {
				clusterClient := newFakeCluster(kubeconfig, configClient).
					WithObjs(append(objects, getKubeconfigSecret(true, clusterName))...)
				return newFakeClient(configClient).WithCluster(clusterClient)
			}(),
			options: GetKubeconfigOptions{
				Kubeconfig:          Kubeconfig(kubeconfig),
				Namespace:           "default",
				WorkloadClusterName: clusterName,
				UserKubeconfig:      true,
			},
			expectErr: false,
			want:      getKubeconfig(clusterName, true),
		},
		{
			name: "returns the system kubeconfig when the secret for user kubeconfig is not available",
			client: func() *fakeClient {
				clusterClient := newFakeCluster(kubeconfig, configClient).
					WithObjs(append(objects, getKubeconfigSecret(false, clusterName))...)
				return newFakeClient(configClient).WithCluster(clusterClient)
			}(),
			options: GetKubeconfigOptions{
				Kubeconfig:          Kubeconfig(kubeconfig),
				Namespace:           "default",
				WorkloadClusterName: clusterName,
				UserKubeconfig:      true,
			},
			expectErr: false,
			want:      getKubeconfig(clusterName, false),
		},
		{
			name: "returns the system kubeconfig when GetKubeconfigOptions.UserKubeconfig is set to false",
			client: func() *fakeClient {
				clusterClient := newFakeCluster(kubeconfig, configClient).
					WithObjs(append(objects, getKubeconfigSecret(false, clusterName))...)
				return newFakeClient(configClient).WithCluster(clusterClient)
			}(),
			options: GetKubeconfigOptions{
				Kubeconfig:          Kubeconfig(kubeconfig),
				Namespace:           "default",
				WorkloadClusterName: clusterName,
				UserKubeconfig:      false,
			},
			expectErr: false,
			want:      getKubeconfig(clusterName, false),
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
  name: %[2]s
contexts:
- context:
    cluster: %[2]s
    user: %[1]s
  name: %[2]s
current-context: %[2]s
preferences: {}
users
- name: %[1]s
  user:
    token: sometoken
`, username, clusterName)
}

//getKubeconfigSecret returns user/system specific kubeconfig secret
func getKubeconfigSecret(isUser bool, clusterName string) *corev1.Secret {
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
