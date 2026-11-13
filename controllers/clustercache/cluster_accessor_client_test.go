/*
Copyright 2024 The Kubernetes Authors.

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

package clustercache

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"sigs.k8s.io/cluster-api/util/secret"
)

func TestDynamicBearerTokenRoundTripper(t *testing.T) {
	g := NewWithT(t)

	var authorizationHeaders []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorizationHeaders = append(authorizationHeaders, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	holder := newTokenHolder("stale-token")
	config := &rest.Config{
		Host:        server.URL,
		BearerToken: "stale-token",
	}
	holder.originalWrapTransport = config.WrapTransport
	holder.setConfigWrapTransport(config)

	httpClient, err := rest.HTTPClientFor(config)
	g.Expect(err).ToNot(HaveOccurred())

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, http.NoBody)
	g.Expect(err).ToNot(HaveOccurred())
	resp, err := httpClient.Do(req)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(resp.Body.Close()).To(Succeed())
	g.Expect(authorizationHeaders).To(Equal([]string{"Bearer stale-token"}))

	holder.store("rotated-token")

	req, err = http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, http.NoBody)
	g.Expect(err).ToNot(HaveOccurred())
	resp, err = httpClient.Do(req)
	g.Expect(err).ToNot(HaveOccurred())
	body, err := io.ReadAll(resp.Body)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(resp.Body.Close()).To(Succeed())
	g.Expect(body).To(BeEmpty())
	g.Expect(authorizationHeaders).To(Equal([]string{"Bearer stale-token", "Bearer rotated-token"}))

	req, err = http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, http.NoBody)
	g.Expect(err).ToNot(HaveOccurred())
	req.Header.Set("Authorization", "Bearer request-token")
	resp, err = httpClient.Do(req)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(resp.Body.Close()).To(Succeed())
	g.Expect(authorizationHeaders).To(Equal([]string{"Bearer stale-token", "Bearer rotated-token", "Bearer request-token"}))
}

func TestRunningOnWorkloadCluster(t *testing.T) {
	tests := []struct {
		name                      string
		currentControllerMetadata *metav1.ObjectMeta
		clusterObjects            []client.Object
		expected                  bool
	}{
		{
			name: "should return true if the controller is running on the workload cluster",
			currentControllerMetadata: &metav1.ObjectMeta{
				Name:      "controller-pod",
				Namespace: "controller-pod-namespace",
				UID:       types.UID("controller-pod-uid"),
			},
			clusterObjects: []client.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "controller-pod",
						Namespace: "controller-pod-namespace",
						UID:       types.UID("controller-pod-uid"),
					},
				},
			},
			expected: true,
		},
		{
			name: "should return false if the controller is not running on the workload cluster: name mismatch",
			currentControllerMetadata: &metav1.ObjectMeta{
				Name:      "controller-pod",
				Namespace: "controller-pod-namespace",
				UID:       types.UID("controller-pod-uid"),
			},
			clusterObjects: []client.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "controller-pod-mismatch",
						Namespace: "controller-pod-namespace",
						UID:       types.UID("controller-pod-uid"),
					},
				},
			},
			expected: false,
		},
		{
			name: "should return false if the controller is not running on the workload cluster: namespace mismatch",
			currentControllerMetadata: &metav1.ObjectMeta{
				Name:      "controller-pod",
				Namespace: "controller-pod-namespace",
				UID:       types.UID("controller-pod-uid"),
			},
			clusterObjects: []client.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "controller-pod",
						Namespace: "controller-pod-namespace-mismatch",
						UID:       types.UID("controller-pod-uid"),
					},
				},
			},
			expected: false,
		},
		{
			name: "should return false if the controller is not running on the workload cluster: uid mismatch",
			currentControllerMetadata: &metav1.ObjectMeta{
				Name:      "controller-pod",
				Namespace: "controller-pod-namespace",
				UID:       types.UID("controller-pod-uid"),
			},
			clusterObjects: []client.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "controller-pod",
						Namespace: "controller-pod-namespace",
						UID:       types.UID("controller-pod-uid-mismatch"),
					},
				},
			},
			expected: false,
		},
		{
			name: "should return false if the controller is not running on the workload cluster: no pod in cluster",
			currentControllerMetadata: &metav1.ObjectMeta{
				Name:      "controller-pod",
				Namespace: "controller-pod-namespace",
				UID:       types.UID("controller-pod-uid"),
			},
			clusterObjects: []client.Object{},
			expected:       false,
		},
		{
			name:                      "should return false if the controller is not running on the workload cluster: no controller metadata",
			currentControllerMetadata: nil,
			clusterObjects: []client.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "controller-pod",
						Namespace: "controller-pod-namespace",
						UID:       types.UID("controller-pod-uid"),
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := fake.NewClientBuilder().WithObjects(tt.clusterObjects...).Build()

			found, err := runningOnWorkloadCluster(ctx, tt.currentControllerMetadata, c)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(found).To(Equal(tt.expected))
		})
	}
}

func TestCreateRESTConfig(t *testing.T) {
	const (
		namespace   = "default"
		clusterName = "test-cluster"
	)

	kubeconfig := func(clusterFields, userFields string) string {
		return fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: c
  cluster:
    server: https://test-cluster.example.com:6443
%s
contexts:
- name: c
  context:
    cluster: c
    user: u
current-context: c
users:
- name: u
  user:
%s
`, clusterFields, userFields)
	}

	// client-go verifies that referenced files exist when parsing the kubeconfig.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "file")
	if err := os.WriteFile(filePath, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		kubeconfig string
		wantErr    string
	}{
		{
			name:       "inline credentials are accepted",
			kubeconfig: kubeconfig("    insecure-skip-tls-verify: true", "    token: token"),
		},
		{
			name:       "tokenFile is rejected",
			kubeconfig: kubeconfig("    insecure-skip-tls-verify: true", "    tokenFile: "+filePath),
			wantErr:    "BearerTokenFile is not supported in ClusterCache",
		},
		{
			name:       "client-certificate file is rejected",
			kubeconfig: kubeconfig("    insecure-skip-tls-verify: true", "    client-certificate: "+filePath+"\n    client-key: "+filePath),
			wantErr:    "TLSClientConfig.CertFile is not supported in ClusterCache",
		},
		{
			name:       "client-key file is rejected",
			kubeconfig: kubeconfig("    insecure-skip-tls-verify: true", "    client-certificate-data: ZGF0YQ==\n    client-key: "+filePath),
			wantErr:    "TLSClientConfig.KeyFile is not supported in ClusterCache",
		},
		{
			name:       "certificate-authority file is rejected",
			kubeconfig: kubeconfig("    certificate-authority: "+filePath, "    token: token"),
			wantErr:    "TLSClientConfig.CAFile is not supported in ClusterCache",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			kubeconfigSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secret.Name(clusterName, secret.Kubeconfig),
					Namespace: namespace,
				},
				Data: map[string][]byte{
					secret.KubeconfigDataName: []byte(tt.kubeconfig),
				},
			}
			c := fake.NewClientBuilder().WithObjects(kubeconfigSecret).Build()

			restConfig, _, _, _, err := createRESTConfig(ctx, &clusterAccessorClientConfig{}, c, client.ObjectKey{Namespace: namespace, Name: clusterName})
			if tt.wantErr != "" {
				g.Expect(err).To(MatchError(ContainSubstring(tt.wantErr)))
				g.Expect(restConfig).To(BeNil())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(restConfig).ToNot(BeNil())
		})
	}
}
