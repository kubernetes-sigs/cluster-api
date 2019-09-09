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

package remote

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/util/secret"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	clusterWithValidKubeConfig = &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test1",
			Namespace: "test",
		},
	}

	clusterWithInvalidKubeConfig = &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test2",
			Namespace: "test",
		},
	}

	clusterWithNoKubeConfig = &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test3",
			Namespace: "test",
		},
	}

	validKubeConfig = `
clusters:
- cluster:
    server: %s
  name: test-cluster-api
contexts:
- context:
    cluster: test-cluster-api
    user: kubernetes-admin
  name: kubernetes-admin@test-cluster-api
current-context: kubernetes-admin@test-cluster-api
kind: Config
preferences: {}
users:
- name: kubernetes-admin
`
	invalidSecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test2-kubeconfig",
			Namespace: "test",
		},
		Data: map[string][]byte{
			secret.KubeconfigDataName: []byte("Not valid!!1"),
		},
	}
)

func kubeconfigSecret(apiendpoint string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test1-kubeconfig",
			Namespace: "test",
		},
		Data: map[string][]byte{
			secret.KubeconfigDataName: []byte(fmt.Sprintf(validKubeConfig, apiendpoint)),
		},
	}
}

func TestNewClusterClient(t *testing.T) {
	// The most minimal APIServer ever
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"apiVersion": "v1", "kind": "APIVersions"}`)
	})
	mux.HandleFunc("/apis", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"apiVersion": "v1", "kind": "APIGroupList"}`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	vs := kubeconfigSecret(ts.URL)
	t.Run("cluster with valid kubeconfig", func(t *testing.T) {
		client := fake.NewFakeClientWithScheme(scheme.Scheme, vs)
		c, err := NewClusterClient(client, clusterWithValidKubeConfig)
		if err != nil {
			t.Fatalf("Expected no errors, got %v", err)
		}

		if c == nil {
			t.Fatal("Expected actual client, got nil")
		}
	})

	t.Run("cluster with no kubeconfig", func(t *testing.T) {
		client := fake.NewFakeClientWithScheme(scheme.Scheme)
		_, err := NewClusterClient(client, clusterWithNoKubeConfig)
		if !strings.Contains(err.Error(), "not found") {
			t.Fatalf("Expected not found error, got %v", err)
		}
	})

	t.Run("cluster with invalid kubeconfig", func(t *testing.T) {
		client := fake.NewFakeClientWithScheme(scheme.Scheme, invalidSecret)
		_, err := NewClusterClient(client, clusterWithInvalidKubeConfig)
		if err == nil || apierrors.IsNotFound(err) {
			t.Fatalf("Expected error, got %v", err)
		}
	})

}
