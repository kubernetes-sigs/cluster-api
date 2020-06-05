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

package v1alpha3

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cabpkv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
)

// These tests are written in BDD-style using Ginkgo framework. Refer to
// http://onsi.github.io/ginkgo to learn more.

var _ = Describe("KubeadmControlPlane", func() {
	var (
		key              types.NamespacedName
		created, fetched *KubeadmControlPlane
		ctx              = context.TODO()
	)

	// Add Tests for OpenAPI validation (or additional CRD features) specified in
	// your API definition.
	// Avoid adding tests for vanilla CRUD operations because they would
	// test Kubernetes API server, which isn't the goal here.
	Context("Create API", func() {

		It("should create an object successfully", func() {

			key = types.NamespacedName{
				Name:      "foo",
				Namespace: "default",
			}

			// missing field
			created2 := map[string]interface{}{
				"kind":       "KubeadmControlPlane",
				"apiVersion": "controlplane.cluster.x-k8s.io/v1alpha3",
				"metadata": map[string]interface{}{
					"name":      "foo",
					"namespace": "default",
				},
				"spec": map[string]interface{}{
					"version": "v1.1.1",
				},
			}
			createdUnstructured := &unstructured.Unstructured{Object: created2}

			By("creating an API obj with missing field")
			Expect(k8sClient.Create(ctx, createdUnstructured)).NotTo(Succeed())

			created = &KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: KubeadmControlPlaneSpec{
					InfrastructureTemplate: corev1.ObjectReference{},
					Version:                "v1.1.1",
					KubeadmConfigSpec:      cabpkv1.KubeadmConfigSpec{},
				},
			}

			By("creating an API obj")
			Expect(k8sClient.Create(ctx, created)).To(Succeed())

			fetched = &KubeadmControlPlane{}
			Expect(k8sClient.Get(ctx, key, fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			By("deleting the created object")
			Expect(k8sClient.Delete(ctx, created)).To(Succeed())
			Expect(k8sClient.Get(ctx, key, created)).ToNot(Succeed())
		})

	})
})
