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
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	clusterWithValidKubeConfig = &v1alpha2.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test1",
			Namespace: "test",
		},
	}

	clusterWithInvalidKubeConfig = &v1alpha2.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test2",
			Namespace: "test",
		},
	}

	clusterWithNoKubeConfig = &v1alpha2.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test3",
			Namespace: "test",
		},
	}
)

func TestNewClusterClient(t *testing.T) {
	t.Run("cluster with valid kubeconfig", func(t *testing.T) {
		client := fake.NewFakeClient(validSecret)
		c, err := NewClusterClient(client, clusterWithValidKubeConfig)
		if err != nil {
			t.Fatalf("Expected no errors, got %v", err)
		}

		if c == nil {
			t.Fatal("Expected actual client, got nil")
		}

		restConfig := c.RESTConfig()
		if restConfig.Host != "https://test-cluster-api:6443" {
			t.Fatalf("Unexpected Host value in RESTConfig: %q", restConfig.Host)
		}
	})

	t.Run("cluster with no kubeconfig", func(t *testing.T) {
		client := fake.NewFakeClient()
		_, err := NewClusterClient(client, clusterWithNoKubeConfig)
		if !strings.Contains(err.Error(), "secret not found") {
			t.Fatalf("Expected not found error, got %v", err)
		}
	})

	t.Run("cluster with invalid kubeconfig", func(t *testing.T) {
		client := fake.NewFakeClient(invalidSecret)
		_, err := NewClusterClient(client, clusterWithInvalidKubeConfig)
		if err == nil || apierrors.IsNotFound(err) {
			t.Fatalf("Expected error, got %v", err)
		}
	})

}
