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
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util/secret"
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
    server: https://test-cluster-api:6443
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

	validSecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test1-kubeconfig",
			Namespace: "test",
		},
		Data: map[string][]byte{
			secret.KubeconfigDataName: []byte(validKubeConfig),
		},
	}

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

func TestNewClusterClient(t *testing.T) {
	g := NewWithT(t)

	testScheme := runtime.NewScheme()
	g.Expect(scheme.AddToScheme(testScheme)).To(Succeed())

	t.Run("cluster with valid kubeconfig", func(t *testing.T) {
		client := fake.NewFakeClientWithScheme(testScheme, validSecret)
		c, err := NewClusterClient(client, clusterWithValidKubeConfig, testScheme)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(c).NotTo(BeNil())

		restConfig, err := RESTConfig(client, clusterWithValidKubeConfig)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(restConfig.Host).To(Equal("https://test-cluster-api:6443"))
	})

	t.Run("cluster with no kubeconfig", func(t *testing.T) {
		client := fake.NewFakeClientWithScheme(testScheme)
		_, err := NewClusterClient(client, clusterWithNoKubeConfig, testScheme)
		g.Expect(err).To(MatchError(ContainSubstring("not found")))
	})

	t.Run("cluster with invalid kubeconfig", func(t *testing.T) {
		client := fake.NewFakeClientWithScheme(testScheme, invalidSecret)
		_, err := NewClusterClient(client, clusterWithInvalidKubeConfig, testScheme)
		g.Expect(err).To(HaveOccurred())
		g.Expect(apierrors.IsNotFound(err)).To(BeFalse())
	})

}
