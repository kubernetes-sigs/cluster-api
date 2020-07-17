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

package controllers

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	timeout              = time.Second * 10
	defaultNamespaceName = "default"
)

var _ = Describe("ClusterResourceSet Reconciler", func() {

	var testCluster *clusterv1.Cluster
	var clusterName string
	BeforeEach(func() {
		clusterName = fmt.Sprintf("cluster-%s", util.RandomString(6))
		testCluster = &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: defaultNamespaceName}}

		By("Creating the Cluster")
		Expect(testEnv.Create(ctx, testCluster)).To(Succeed())
		By("Creating the remote Cluster kubeconfig")
		Expect(testEnv.CreateKubeconfigSecret(testCluster)).To(Succeed())
	})
	AfterEach(func() {
		By("Deleting the Kubeconfigsecret")
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName + "-kubeconfig",
				Namespace: defaultNamespaceName,
			},
		}
		Expect(testEnv.Delete(ctx, secret)).To(Succeed())
	})

	It("Should reconcile a ClusterResourceSet when a cluster with matching label exists", func() {
		By("Creating the resource configmap")
		var configmapName = "test-configmap"
		testConfigmap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configmapName,
				Namespace: defaultNamespaceName,
			},
			Data: map[string]string{
				"cm": `metadata:
name: resource-configmap
namespace: default
kind: ConfigMap
apiVersion: v1
---
metadata:
name: resource-configmap2
namespace: default
kind: ConfigMap
apiVersion: v1`,
				"cm2": `metadata:
name: resource-configmap3
namespace: default
kind: ConfigMap
apiVersion: v1
---
metadata:
name: resource-configmap4
namespace: default
kind: ConfigMap
apiVersion: v1`,
			},
		}
		Expect(testEnv.Create(ctx, testConfigmap)).To(Succeed())

		By("Updating the cluster with labels")
		labels := map[string]string{"foo": "bar"}
		testCluster.SetLabels(labels)
		Expect(testEnv.Update(ctx, testCluster)).To(Succeed())

		By("Creating a ClusterResourceSet instance that has same labels as selector")
		clusterResourceSetInstance := &addonsv1.ClusterResourceSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-clusterresourceset",
				Namespace: defaultNamespaceName,
			},
			Spec: addonsv1.ClusterResourceSetSpec{
				ClusterSelector: metav1.LabelSelector{
					MatchLabels: labels,
				},
				Resources: []addonsv1.ResourceRef{{Name: configmapName, Kind: "ConfigMap"}},
			},
		}
		// Create the ClusterResourceSet.
		Expect(testEnv.Create(ctx, clusterResourceSetInstance)).To(Succeed())
		defer func() {
			Expect(testEnv.Delete(ctx, clusterResourceSetInstance)).To(Succeed())
		}()

		By("Verifying ClusterResourceSetBinding is created with cluster owner reference")
		Eventually(func() bool {
			binding := &addonsv1.ClusterResourceSetBinding{}
			clusterResourceSetBindingKey := client.ObjectKey{
				Namespace: testCluster.Namespace,
				Name:      testCluster.Name,
			}
			err := testEnv.Get(ctx, clusterResourceSetBindingKey, binding)
			if err != nil {
				return false
			}

			if len(binding.Spec.Bindings) != 1 {
				return false
			}
			if len(binding.Spec.Bindings[0].Resources) != 1 {
				return false
			}

			return util.HasOwnerRef(binding.GetOwnerReferences(), metav1.OwnerReference{
				APIVersion: clusterv1.GroupVersion.String(),
				Kind:       "Cluster",
				Name:       testCluster.Name,
				UID:        testCluster.UID,
			})
		}, timeout).Should(BeTrue())
		By("Deleting the Cluster")
		Expect(testEnv.Delete(ctx, testCluster)).To(Succeed())
	})

	It("Should reconcile a cluster when its labels are changed to match a ClusterResourceSet's selector", func() {

		labels := map[string]string{"foo": "bar"}

		clusterResourceSetInstance := &addonsv1.ClusterResourceSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-clusterresourceset",
				Namespace: defaultNamespaceName,
			},
			Spec: addonsv1.ClusterResourceSetSpec{
				ClusterSelector: metav1.LabelSelector{
					MatchLabels: labels,
				},
			},
		}
		// Create the ClusterResourceSet.
		Expect(testEnv.Create(ctx, clusterResourceSetInstance)).To(Succeed())
		defer func() {
			Expect(testEnv.Delete(ctx, clusterResourceSetInstance)).To(Succeed())
		}()

		testCluster.SetLabels(labels)
		Expect(testEnv.Update(ctx, testCluster)).To(Succeed())

		By("Verifying ClusterResourceSetBinding is created with cluster owner reference")
		Eventually(func() bool {
			binding := &addonsv1.ClusterResourceSetBinding{}
			clusterResourceSetBindingKey := client.ObjectKey{
				Namespace: testCluster.Namespace,
				Name:      testCluster.Name,
			}
			err := testEnv.Get(ctx, clusterResourceSetBindingKey, binding)
			if err != nil {
				return false
			}

			return util.HasOwnerRef(binding.GetOwnerReferences(), metav1.OwnerReference{
				APIVersion: clusterv1.GroupVersion.String(),
				Kind:       "Cluster",
				Name:       testCluster.Name,
				UID:        testCluster.UID,
			})
		}, timeout).Should(BeTrue())

		// Wait until ClusterResourceSetBinding is created for the Cluster
		clusterResourceSetBindingKey := client.ObjectKey{
			Namespace: testCluster.Namespace,
			Name:      testCluster.Name,
		}
		Eventually(func() bool {
			binding := &addonsv1.ClusterResourceSetBinding{}

			err := testEnv.Get(ctx, clusterResourceSetBindingKey, binding)
			return err == nil
		}, timeout).Should(BeTrue())

		By("Verifying ClusterResourceSetBinding is deleted when its cluster owner reference is deleted")
		Expect(testEnv.Delete(ctx, testCluster)).To(Succeed())

		Eventually(func() bool {
			binding := &addonsv1.ClusterResourceSetBinding{}

			err := testEnv.Get(ctx, clusterResourceSetBindingKey, binding)

			return apierrors.IsNotFound(err)
		}, timeout).Should(BeTrue())
	})
})
