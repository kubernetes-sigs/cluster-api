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

package cluster

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	"sigs.k8s.io/cluster-api/util/secret"
)

func Test_WorkloadCluster_GetKubeconfig(t *testing.T) {
	var (
		validKubeConfig = `
clusters:
- cluster:
    certificate-authority-data: stuff
    server: https://test-cluster-api:6443
  name: test1
contexts:
- context:
    cluster: test1
    user: test1-admin
  name: test1-admin@test1
current-context: test1-admin@test1
kind: Config
preferences: {}
users:
- name: test1-admin
  user:
    client-certificate-data: stuff-cert-data
    client-key-data: stuff-key-data
`

		validSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test1-kubeconfig",
				Namespace: "test",
				Labels:    map[string]string{clusterv1.ClusterLabelName: "test1"},
			},
			Data: map[string][]byte{
				secret.KubeconfigDataName: []byte(validKubeConfig),
			},
		}
	)

	tests := []struct {
		name           string
		expectErr      bool
		proxy          Proxy
		userKubeConfig bool
		want           string
	}{
		{
			name:      "return secret data",
			expectErr: false,
			proxy:     test.NewFakeProxy().WithObjs(validSecret),
			want:      string(validSecret.Data[secret.KubeconfigDataName]),
		},
		{
			name:      "return error if cannot find secret",
			expectErr: true,
			proxy:     test.NewFakeProxy(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			wc := newWorkloadCluster(tt.proxy)
			data, err := wc.GetKubeconfig("test1", "test")

			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(data).To(Equal(tt.want))
		})
	}
}

func Test_WorkloadCluster_GetUserKubeconfig(t *testing.T) {
	var (
		validUserKubeconfig = `
clusters:
- cluster:
    certificate-authority-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUN5RENDQWJDZ0F3SUJBZ0lCQURBTkJna3Foa2lHOXcwQkFRc0ZBREFWTVJNd0VRWURWUVFERXdwcmRXSmwKY201bGRHVnpNQjRYRFRFNU1ERXhNREU0TURBME1Gb1hEVEk1TURFd056RTRNREEwTUZvd0ZURVRNQkVHQTFVRQpBeE1LYTNWaVpYSnVaWFJsY3pDQ0FTSXdEUVlKS29aSWh2Y05BUUVCQlFBRGdnRVBBRENDQVFvQ2dnRUJBT1EvCmVndmViNk1qMHkzM3hSbGFjczd6OXE4QTNDajcrdnRrZ0pUSjRUSVB4TldRTEd0Q0dmL0xadzlHMW9zNmRKUHgKZFhDNmkwaHA5czJuT0Y2VjQvREUrYUNTTU45VDYzckdWb2s0TkcwSlJPYmlRWEtNY1VQakpiYm9PTXF2R2lLaAoyMGlFY0h5K3B4WkZOb3FzdnlaRGM5L2dRSHJVR1FPNXp6TDNHZGhFL0k1Nkczek9JaWhhbFRHSTNaakRRS05CCmVFV3FONTVDcHZzT3I1b0ZnTmZZTXk2YzZ4WXlUTlhWSUkwNFN0Z2xBbUk4bzZWaTNUVEJhZ1BWaldIVnRha1EKU2w3VGZtVUlIdndKZUo3b2hxbXArVThvaGVTQUIraHZSbDIrVHE5NURTemtKcmRjNmphcyswd2FWaEJydEh1agpWMU15NlNvV2VVUlkrdW5VVFgwQ0F3RUFBYU1qTUNFd0RnWURWUjBQQVFIL0JBUURBZ0trTUE4R0ExVWRFd0VCCi93UUZNQU1CQWY4d0RRWUpLb1pJaHZjTkFRRUxCUUFEZ2dFQkFIT2thSXNsd0pCOE5PaENUZkF4UWlnaUc1bEMKQlo0LytGeHZ3Y1pnWGhlL0IyUWo1UURMNWlRUU1VL2NqQ0tyYUxkTFFqM1o1aHA1dzY0K2NWRUg3Vm9PSTFCaQowMm13YTc4eWo4aDNzQ2lLQXJiU21kKzNld1QrdlNpWFMzWk9EYWRHVVRRa1BnUHB0THlaMlRGakF0SW43WjcyCmpnYlVnT2FXaklKbnlwRVJ5UmlSKzBvWlk4SUlmWWFsTHUwVXlXcmkwaVhNRmZqQUQ1UVNURy8zRGN5djhEN1UKZHBxU2l5ekJkZVRjSExyenpEbktBeXhQWWgvcWpKZ0tRdEdIakhjY0FCSE1URlFtRy9Ea1pNTnZWL2FZSnMrKwp0aVJCbHExSFhlQ0d4aExFcGdQcGxVb3IrWmVYTGF2WUo0Z2dMVmIweGl2QTF2RUtyaUUwak1Wd2lQaz0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=
    server: https://test-cluster-api.us-east-1.eks.amazonaws.com
  name: test1
contexts:
- context:
    cluster: test1
    user: test1-admin-user
  name: test1-admin@test1
current-context: test1-admin@test1
kind: Config
preferences: {}
- name: test1-admin-user
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1alpha1
      args:
      - token
      - -i
      - test1-admin-user
      command: aws-iam-authenticator
      env: null
      provideClusterInfo: false
`
		validUserSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test1-user-kubeconfig",
				Namespace: "test",
				Labels:    map[string]string{clusterv1.ClusterLabelName: "test1-user"},
			},
			Data: map[string][]byte{
				secret.KubeconfigDataName: []byte(validUserKubeconfig),
			},
			Type: clusterv1.ClusterSecretType,
		}
	)

	tests := []struct {
		name           string
		expectErr      bool
		proxy          Proxy
		userKubeConfig bool
		want           string
	}{
		{
			name:      "return error if cannot find secret",
			expectErr: true,
			proxy:     test.NewFakeProxy(),
		},
		{
			name:           "return user secret data ",
			expectErr:      false,
			proxy:          test.NewFakeProxy().WithObjs(validUserSecret),
			userKubeConfig: true,
			want:           string(validUserSecret.Data[secret.KubeconfigDataName]),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			wc := newWorkloadCluster(tt.proxy)
			data, err := GetUserKubeconfig(wc.proxy, "test1", "test")

			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(data).To(Equal(tt.want))
		})
	}
}
