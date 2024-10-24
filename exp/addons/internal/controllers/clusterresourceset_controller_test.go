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
	"crypto/sha1" //nolint: gosec
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/test/envtest"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
)

const (
	timeout = time.Second * 15
)

func TestClusterResourceSetReconciler(t *testing.T) {
	var (
		clusterResourceSetName      string
		testCluster                 *clusterv1.Cluster
		clusterName                 string
		labels                      map[string]string
		configmapName               = "test-configmap"
		secretName                  = "test-secret"
		namespacePrefix             = "test-cluster-resource-set"
		resourceConfigMap1Name      = "resource-configmap"
		resourceConfigMap2Name      = "resource-configmap-2"
		resourceConfigMapsNamespace = "default"
	)

	createConfigMapAndSecret := func(g Gomega, namespaceName, configmapName, secretName string) {
		resourceConfigMap1Content := fmt.Sprintf(`metadata:
 name: %s
 namespace: %s
kind: ConfigMap
apiVersion: v1`, resourceConfigMap1Name, resourceConfigMapsNamespace)

		testConfigmap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configmapName,
				Namespace: namespaceName,
			},
			Data: map[string]string{
				"cm": resourceConfigMap1Content,
			},
		}

		resourceConfigMap2Content := fmt.Sprintf(`metadata:
kind: ConfigMap
apiVersion: v1
metadata:
 name: %s
 namespace: %s`, resourceConfigMap2Name, resourceConfigMapsNamespace)
		testSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespaceName,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: clusterName,
				},
			},
			Type: "addons.cluster.x-k8s.io/resource-set",
			StringData: map[string]string{
				"cm": resourceConfigMap2Content,
			},
		}
		t.Log("Creating a Secret and a ConfigMap with ConfigMap in their data field")
		g.Expect(env.Create(ctx, testConfigmap)).To(Succeed())
		g.Expect(env.Create(ctx, testSecret)).To(Succeed())
	}

	setup := func(t *testing.T, g *WithT) *corev1.Namespace {
		t.Helper()

		clusterResourceSetName = fmt.Sprintf("clusterresourceset-%s", util.RandomString(6))
		labels = map[string]string{clusterResourceSetName: "bar"}

		ns, err := env.CreateNamespace(ctx, namespacePrefix)
		g.Expect(err).ToNot(HaveOccurred())

		clusterName = fmt.Sprintf("cluster-%s", util.RandomString(6))
		testCluster = &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: ns.Name}}

		t.Log("Creating the Cluster")
		g.Expect(env.CreateAndWait(ctx, testCluster)).To(Succeed())
		t.Log("Creating the remote Cluster kubeconfig")
		g.Expect(env.CreateKubeconfigSecret(ctx, testCluster)).To(Succeed())
		// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
		patch := client.MergeFrom(testCluster.DeepCopy())
		testCluster.Status.InfrastructureReady = true
		g.Expect(env.Status().Patch(ctx, testCluster, patch)).To(Succeed())

		g.Eventually(func(g Gomega) {
			_, err = clusterCache.GetClient(ctx, client.ObjectKeyFromObject(testCluster))
			g.Expect(err).ToNot(HaveOccurred())
		}, 1*time.Minute, 5*time.Second).Should(Succeed())

		createConfigMapAndSecret(g, ns.Name, configmapName, secretName)
		return ns
	}

	teardown := func(t *testing.T, g *WithT, ns *corev1.Namespace) {
		t.Helper()

		t.Log("Deleting the Kubeconfig Secret")
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName + "-kubeconfig",
				Namespace: ns.Name,
			},
		}
		g.Expect(env.Delete(ctx, secret)).To(Succeed())

		t.Log("Deleting the ClusterResourceSet")
		clusterResourceSetInstance := &addonsv1.ClusterResourceSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterResourceSetName,
				Namespace: ns.Name,
			},
		}
		g.Expect(client.IgnoreNotFound(env.Delete(ctx, clusterResourceSetInstance))).To(Succeed())
		g.Eventually(func() bool {
			return apierrors.IsNotFound(env.Get(ctx, client.ObjectKeyFromObject(clusterResourceSetInstance), &addonsv1.ClusterResourceSet{}))
		}, timeout).Should(BeTrue())

		g.Expect(env.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name:      configmapName,
			Namespace: ns.Name,
		}})).To(Succeed())
		g.Expect(env.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: ns.Name,
		}})).To(Succeed())

		cm1 := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceConfigMap1Name,
				Namespace: resourceConfigMapsNamespace,
			},
		}
		g.Expect(client.IgnoreNotFound(env.Delete(ctx, cm1))).To(Succeed())

		cm2 := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceConfigMap2Name,
				Namespace: resourceConfigMapsNamespace,
			},
		}
		g.Expect(client.IgnoreNotFound(env.Delete(ctx, cm2))).To(Succeed())

		g.Expect(env.Delete(ctx, ns)).To(Succeed())

		g.Expect(client.IgnoreNotFound(env.Delete(ctx, testCluster))).To(Succeed())
	}

	t.Run("Should reconcile a ClusterResourceSet with multiple resources when a cluster with matching label exists", func(t *testing.T) {
		g := NewWithT(t)
		ns := setup(t, g)
		defer teardown(t, g, ns)

		t.Log("Updating the cluster with labels")
		testCluster.SetLabels(labels)
		g.Expect(env.Update(ctx, testCluster)).To(Succeed())

		t.Log("Creating a ClusterResourceSet instance that has same labels as selector")
		clusterResourceSetInstance := &addonsv1.ClusterResourceSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterResourceSetName,
				Namespace: ns.Name,
			},
			Spec: addonsv1.ClusterResourceSetSpec{
				ClusterSelector: metav1.LabelSelector{
					MatchLabels: labels,
				},
				Resources: []addonsv1.ResourceRef{{Name: configmapName, Kind: "ConfigMap"}, {Name: secretName, Kind: "Secret"}},
			},
		}
		// Create the ClusterResourceSet.
		g.Expect(env.Create(ctx, clusterResourceSetInstance)).To(Succeed())

		t.Log("Verifying ClusterResourceSetBinding is created with cluster owner reference")
		g.Eventually(clusterResourceSetBindingReady(env, testCluster), timeout).Should(BeTrue())
		t.Log("Deleting the Cluster")
		g.Expect(env.Delete(ctx, testCluster)).To(Succeed())
	})

	t.Run("Should reconcile a cluster when its labels are changed to match a ClusterResourceSet's selector", func(t *testing.T) {
		g := NewWithT(t)
		ns := setup(t, g)
		defer teardown(t, g, ns)

		clusterResourceSetInstance := &addonsv1.ClusterResourceSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterResourceSetName,
				Namespace: ns.Name,
			},
			Spec: addonsv1.ClusterResourceSetSpec{
				ClusterSelector: metav1.LabelSelector{
					MatchLabels: labels,
				},
			},
		}
		// Create the ClusterResourceSet.
		g.Expect(env.Create(ctx, clusterResourceSetInstance)).To(Succeed())

		testCluster.SetLabels(labels)
		g.Expect(env.Update(ctx, testCluster)).To(Succeed())

		t.Log("Verifying ClusterResourceSetBinding is created with cluster name")
		g.Eventually(func() bool {
			binding := &addonsv1.ClusterResourceSetBinding{}
			clusterResourceSetBindingKey := client.ObjectKey{
				Namespace: testCluster.Namespace,
				Name:      testCluster.Name,
			}
			err := env.Get(ctx, clusterResourceSetBindingKey, binding)
			if err != nil {
				return false
			}

			return binding.Spec.ClusterName == testCluster.Name
		}, timeout).Should(BeTrue())

		// Wait until ClusterResourceSetBinding is created for the Cluster
		clusterResourceSetBindingKey := client.ObjectKey{
			Namespace: testCluster.Namespace,
			Name:      testCluster.Name,
		}
		g.Eventually(func() bool {
			binding := &addonsv1.ClusterResourceSetBinding{}

			err := env.Get(ctx, clusterResourceSetBindingKey, binding)
			return err == nil
		}, timeout).Should(BeTrue())

		t.Log("Verifying ClusterResourceSetBinding is deleted when its cluster owner reference is deleted")
		g.Expect(env.Delete(ctx, testCluster)).To(Succeed())

		g.Eventually(func() bool {
			binding := &addonsv1.ClusterResourceSetBinding{}
			err := env.Get(ctx, clusterResourceSetBindingKey, binding)
			return apierrors.IsNotFound(err)
		}, timeout).Should(BeTrue())
	})

	t.Run("Should reconcile a ClusterResourceSet when a ConfigMap resource is created that is part of ClusterResourceSet resources", func(t *testing.T) {
		g := NewWithT(t)
		ns := setup(t, g)
		defer teardown(t, g, ns)

		newCMName := fmt.Sprintf("test-configmap-%s", util.RandomString(6))

		crsInstance := &addonsv1.ClusterResourceSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterResourceSetName,
				Namespace: ns.Name,
			},
			Spec: addonsv1.ClusterResourceSetSpec{
				ClusterSelector: metav1.LabelSelector{
					MatchLabels: labels,
				},
				Resources: []addonsv1.ResourceRef{{Name: newCMName, Kind: "ConfigMap"}},
			},
		}
		// Create the ClusterResourceSet.
		g.Expect(env.Create(ctx, crsInstance)).To(Succeed())

		testCluster.SetLabels(labels)
		g.Expect(env.Update(ctx, testCluster)).To(Succeed())

		t.Log("Verifying ClusterResourceSetBinding is created with cluster owner reference")
		// Wait until ClusterResourceSetBinding is created for the Cluster
		clusterResourceSetBindingKey := client.ObjectKey{
			Namespace: testCluster.Namespace,
			Name:      testCluster.Name,
		}
		g.Eventually(func() bool {
			binding := &addonsv1.ClusterResourceSetBinding{}

			err := env.Get(ctx, clusterResourceSetBindingKey, binding)
			return err == nil
		}, timeout).Should(BeTrue())

		// Initially ConfigMap is missing, so no resources in the binding.
		g.Eventually(func() bool {
			binding := &addonsv1.ClusterResourceSetBinding{}

			err := env.Get(ctx, clusterResourceSetBindingKey, binding)
			if err == nil {
				if len(binding.Spec.Bindings) > 0 && len(binding.Spec.Bindings[0].Resources) == 0 {
					return true
				}
			}
			return false
		}, timeout).Should(BeTrue())

		// Must sleep here to make sure resource is created after the previous reconcile.
		// If the resource is created in between, predicates are not used as intended in this test.
		time.Sleep(2 * time.Second)

		newConfigmap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      newCMName,
				Namespace: ns.Name,
			},
			Data: map[string]string{},
		}
		g.Expect(env.Create(ctx, newConfigmap)).To(Succeed())
		defer func() {
			g.Expect(env.Delete(ctx, newConfigmap)).To(Succeed())
		}()

		cmKey := client.ObjectKey{
			Namespace: ns.Name,
			Name:      newCMName,
		}
		g.Eventually(func() bool {
			m := &corev1.ConfigMap{}
			if err := env.Get(ctx, cmKey, m); err != nil {
				return false
			}
			if len(m.OwnerReferences) != 1 || m.OwnerReferences[0].Name != crsInstance.Name {
				return false
			}
			return true
		}, timeout).Should(BeTrue())

		// When the ConfigMap resource is created, CRS should get reconciled immediately.
		g.Eventually(func() error {
			binding := &addonsv1.ClusterResourceSetBinding{}
			if err := env.Get(ctx, clusterResourceSetBindingKey, binding); err != nil {
				return err
			}
			if len(binding.Spec.Bindings[0].Resources) > 0 && binding.Spec.Bindings[0].Resources[0].Name == newCMName {
				return nil
			}
			return errors.Errorf("ClusterResourceSet binding does not have any resources matching %q: %v", newCMName, binding.Spec.Bindings)
		}, timeout).Should(Succeed())

		t.Log("Verifying ClusterResourceSetBinding is deleted when its cluster owner reference is deleted")
		g.Expect(env.Delete(ctx, testCluster)).To(Succeed())

		g.Eventually(func() bool {
			binding := &addonsv1.ClusterResourceSetBinding{}
			err := env.Get(ctx, clusterResourceSetBindingKey, binding)
			return apierrors.IsNotFound(err)
		}, timeout).Should(BeTrue())
	})

	t.Run("Should reconcile a ClusterResourceSet when a Secret resource is created that is part of ClusterResourceSet resources", func(t *testing.T) {
		g := NewWithT(t)
		ns := setup(t, g)
		defer teardown(t, g, ns)

		newSecretName := fmt.Sprintf("test-secret-%s", util.RandomString(6))

		crsInstance := &addonsv1.ClusterResourceSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterResourceSetName,
				Namespace: ns.Name,
			},
			Spec: addonsv1.ClusterResourceSetSpec{
				ClusterSelector: metav1.LabelSelector{
					MatchLabels: labels,
				},
				Resources: []addonsv1.ResourceRef{{Name: newSecretName, Kind: "Secret"}},
			},
		}
		// Create the ClusterResourceSet.
		g.Expect(env.Create(ctx, crsInstance)).To(Succeed())

		testCluster.SetLabels(labels)
		g.Expect(env.Update(ctx, testCluster)).To(Succeed())

		// Must sleep here to make sure resource is created after the previous reconcile.
		// If the resource is created in between, predicates are not used as intended in this test.
		time.Sleep(2 * time.Second)

		t.Log("Verifying ClusterResourceSetBinding is created with cluster owner reference")
		// Wait until ClusterResourceSetBinding is created for the Cluster
		clusterResourceSetBindingKey := client.ObjectKey{
			Namespace: testCluster.Namespace,
			Name:      testCluster.Name,
		}
		g.Eventually(func() bool {
			binding := &addonsv1.ClusterResourceSetBinding{}

			err := env.Get(ctx, clusterResourceSetBindingKey, binding)
			return err == nil
		}, timeout).Should(BeTrue())

		// Initially Secret is missing, so no resources in the binding.
		g.Eventually(func() bool {
			binding := &addonsv1.ClusterResourceSetBinding{}

			err := env.Get(ctx, clusterResourceSetBindingKey, binding)
			if err == nil {
				if len(binding.Spec.Bindings) > 0 && len(binding.Spec.Bindings[0].Resources) == 0 {
					return true
				}
			}
			return false
		}, timeout).Should(BeTrue())

		newSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      newSecretName,
				Namespace: ns.Name,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: testCluster.Name,
				},
			},
			Type: addonsv1.ClusterResourceSetSecretType,
			Data: map[string][]byte{},
		}
		g.Expect(env.Create(ctx, newSecret)).To(Succeed())
		defer func() {
			g.Expect(env.Delete(ctx, newSecret)).To(Succeed())
		}()

		cmKey := client.ObjectKey{
			Namespace: ns.Name,
			Name:      newSecretName,
		}
		g.Eventually(func() bool {
			m := &corev1.Secret{}
			if err := env.Get(ctx, cmKey, m); err != nil {
				return false
			}
			if len(m.OwnerReferences) != 1 || m.OwnerReferences[0].Name != crsInstance.Name {
				return false
			}
			return true
		}, timeout).Should(BeTrue())

		// When the Secret resource is created, CRS should get reconciled immediately.
		g.Eventually(func() error {
			binding := &addonsv1.ClusterResourceSetBinding{}
			if err := env.Get(ctx, clusterResourceSetBindingKey, binding); err != nil {
				return err
			}
			if len(binding.Spec.Bindings[0].Resources) > 0 && binding.Spec.Bindings[0].Resources[0].Name == newSecretName {
				return nil
			}
			return errors.Errorf("ClusterResourceSet binding does not have any resources matching %q: %v", newSecretName, binding.Spec.Bindings)
		}, timeout).Should(Succeed())

		t.Log("Verifying ClusterResourceSetBinding is deleted when its cluster owner reference is deleted")
		g.Expect(env.Delete(ctx, testCluster)).To(Succeed())

		g.Eventually(func() bool {
			binding := &addonsv1.ClusterResourceSetBinding{}
			err := env.Get(ctx, clusterResourceSetBindingKey, binding)
			return apierrors.IsNotFound(err)
		}, timeout).Should(BeTrue())
	})

	t.Run("Should delete ClusterResourceSet from the bindings list when ClusterResourceSet is deleted", func(t *testing.T) {
		g := NewWithT(t)
		ns := setup(t, g)
		defer teardown(t, g, ns)

		t.Log("Updating the cluster with labels")
		testCluster.SetLabels(labels)
		g.Expect(env.Update(ctx, testCluster)).To(Succeed())

		t.Log("Creating a ClusterResourceSet instance that has same labels as selector")
		clusterResourceSetInstance2 := &addonsv1.ClusterResourceSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterResourceSetName,
				Namespace: ns.Name,
			},
			Spec: addonsv1.ClusterResourceSetSpec{
				ClusterSelector: metav1.LabelSelector{
					MatchLabels: labels,
				},
				Resources: []addonsv1.ResourceRef{{Name: configmapName, Kind: "ConfigMap"}},
			},
		}
		// Create the ClusterResourceSet.
		g.Expect(env.Create(ctx, clusterResourceSetInstance2)).To(Succeed())

		t.Log("Creating a second ClusterResourceSet instance that has same labels as selector")
		clusterResourceSetInstance3 := &addonsv1.ClusterResourceSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-clusterresourceset2",
				Namespace: ns.Name,
			},
			Spec: addonsv1.ClusterResourceSetSpec{
				ClusterSelector: metav1.LabelSelector{
					MatchLabels: labels,
				},
				Resources: []addonsv1.ResourceRef{{Name: configmapName, Kind: "ConfigMap"}, {Name: secretName, Kind: "Secret"}},
			},
		}
		// Create the ClusterResourceSet.
		g.Expect(env.Create(ctx, clusterResourceSetInstance3)).To(Succeed())

		t.Log("Verifying ClusterResourceSetBinding is created with 2 ClusterResourceSets")
		g.Eventually(func() bool {
			binding := &addonsv1.ClusterResourceSetBinding{}
			clusterResourceSetBindingKey := client.ObjectKey{
				Namespace: testCluster.Namespace,
				Name:      testCluster.Name,
			}
			err := env.Get(ctx, clusterResourceSetBindingKey, binding)
			if err != nil {
				return false
			}
			return len(binding.Spec.Bindings) == 2 && len(binding.OwnerReferences) == 2
		}, timeout).Should(BeTrue(), "Expected 2 ClusterResourceSets and 2 OwnerReferences")

		t.Log("Verifying deleted CRS is deleted from ClusterResourceSetBinding")
		// Delete one of the CRS instances and wait until it is removed from the binding list.
		g.Expect(env.Delete(ctx, clusterResourceSetInstance2)).To(Succeed())
		g.Eventually(func() bool {
			binding := &addonsv1.ClusterResourceSetBinding{}
			clusterResourceSetBindingKey := client.ObjectKey{
				Namespace: testCluster.Namespace,
				Name:      testCluster.Name,
			}
			err := env.Get(ctx, clusterResourceSetBindingKey, binding)
			if err != nil {
				return false
			}
			return len(binding.Spec.Bindings) == 1 && len(binding.OwnerReferences) == 1
		}, timeout).Should(BeTrue(), "ClusterResourceSetBinding should have 1 ClusterResourceSet and 1 OwnerReferences")

		t.Log("Verifying ClusterResourceSetBinding is deleted after deleting all matching CRS objects")
		// Delete one of the CRS instances and wait until it is removed from the binding list.
		g.Expect(env.Delete(ctx, clusterResourceSetInstance3)).To(Succeed())
		g.Eventually(func() bool {
			binding := &addonsv1.ClusterResourceSetBinding{}
			clusterResourceSetBindingKey := client.ObjectKey{
				Namespace: testCluster.Namespace,
				Name:      testCluster.Name,
			}
			return env.Get(ctx, clusterResourceSetBindingKey, binding) != nil
		}, timeout).Should(BeTrue())

		t.Log("Deleting the Cluster")
		g.Expect(env.Delete(ctx, testCluster)).To(Succeed())
	})

	t.Run("Should add finalizer after reconcile", func(t *testing.T) {
		g := NewWithT(t)
		ns := setup(t, g)
		defer teardown(t, g, ns)

		dt := metav1.Now()
		clusterResourceSetInstance := &addonsv1.ClusterResourceSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:              clusterResourceSetName,
				Namespace:         ns.Name,
				Finalizers:        []string{addonsv1.ClusterResourceSetFinalizer},
				DeletionTimestamp: &dt,
			},
			Spec: addonsv1.ClusterResourceSetSpec{
				ClusterSelector: metav1.LabelSelector{
					MatchLabels: labels,
				},
			},
		}
		// Create the ClusterResourceSet.
		g.Expect(env.Create(ctx, clusterResourceSetInstance)).To(Succeed())
		g.Eventually(func() bool {
			crsKey := client.ObjectKey{
				Namespace: clusterResourceSetInstance.Namespace,
				Name:      clusterResourceSetInstance.Name,
			}
			crs := &addonsv1.ClusterResourceSet{}

			err := env.Get(ctx, crsKey, crs)
			if err == nil {
				return len(crs.Finalizers) > 0
			}
			return false
		}, timeout).Should(BeTrue())
	})

	t.Run("Should reconcile a ClusterResourceSet with Reconcile strategy after the resources have already been created", func(t *testing.T) {
		g := NewWithT(t)
		ns := setup(t, g)
		defer teardown(t, g, ns)

		t.Log("Updating the cluster with labels")
		testCluster.SetLabels(labels)
		g.Expect(env.Update(ctx, testCluster)).To(Succeed())

		t.Log("Creating a ClusterResourceSet instance that has same labels as selector")
		clusterResourceSet := &addonsv1.ClusterResourceSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterResourceSetName,
				Namespace: ns.Name,
			},
			Spec: addonsv1.ClusterResourceSetSpec{
				Strategy: string(addonsv1.ClusterResourceSetStrategyReconcile),
				ClusterSelector: metav1.LabelSelector{
					MatchLabels: labels,
				},
				Resources: []addonsv1.ResourceRef{{Name: configmapName, Kind: "ConfigMap"}, {Name: secretName, Kind: "Secret"}},
			},
		}

		g.Expect(env.Create(ctx, clusterResourceSet)).To(Succeed())

		t.Log("Verifying ClusterResourceSetBinding is created with cluster owner reference")
		clusterResourceSetBindingKey := client.ObjectKey{
			Namespace: testCluster.Namespace,
			Name:      testCluster.Name,
		}
		g.Eventually(clusterResourceSetBindingReady(env, testCluster), timeout).Should(BeTrue())

		binding := &addonsv1.ClusterResourceSetBinding{}
		err := env.Get(ctx, clusterResourceSetBindingKey, binding)
		g.Expect(err).ToNot(HaveOccurred())
		resourceHashes := map[string]string{}
		for _, r := range binding.Spec.Bindings[0].Resources {
			resourceHashes[r.Name] = r.Hash
		}

		t.Log("Verifying resource ConfigMap 1 has been created")
		resourceConfigMap1Key := client.ObjectKey{
			Namespace: resourceConfigMapsNamespace,
			Name:      resourceConfigMap1Name,
		}
		g.Eventually(func() error {
			cm := &corev1.ConfigMap{}
			return env.Get(ctx, resourceConfigMap1Key, cm)
		}, timeout).Should(Succeed())

		t.Log("Verifying resource ConfigMap 2 has been created")
		resourceConfigMap2Key := client.ObjectKey{
			Namespace: resourceConfigMapsNamespace,
			Name:      resourceConfigMap2Name,
		}
		g.Eventually(func() error {
			cm := &corev1.ConfigMap{}
			return env.Get(ctx, resourceConfigMap2Key, cm)
		}, timeout).Should(Succeed())

		resourceConfigMap1 := configMap(
			resourceConfigMap1Name,
			resourceConfigMapsNamespace,
			map[string]string{
				"my_new_config": "some_value",
			},
		)

		resourceConfigMap1Content, err := yaml.Marshal(resourceConfigMap1)
		g.Expect(err).ToNot(HaveOccurred())

		testConfigmap := configMap(
			configmapName,
			ns.Name,
			map[string]string{
				"cm": string(resourceConfigMap1Content),
			},
		)

		resourceConfigMap2 := configMap(
			resourceConfigMap2Name,
			resourceConfigMapsNamespace,
			map[string]string{
				"my_new_secret_config": "some_secret_value",
			},
		)

		resourceConfigMap2Content, err := yaml.Marshal(resourceConfigMap2)
		g.Expect(err).ToNot(HaveOccurred())

		testSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: ns.Name,
			},
			Type: "addons.cluster.x-k8s.io/resource-set",
			StringData: map[string]string{
				"cm": string(resourceConfigMap2Content),
			},
		}
		t.Log("Updating the Secret and a ConfigMap with updated ConfigMaps in their data field")
		g.Expect(env.Update(ctx, testConfigmap)).To(Succeed())
		g.Expect(env.Update(ctx, testSecret)).To(Succeed())

		t.Log("Verifying ClusterResourceSetBinding has been updated with new hashes")
		g.Eventually(func() error {
			binding := &addonsv1.ClusterResourceSetBinding{}
			if err := env.Get(ctx, clusterResourceSetBindingKey, binding); err != nil {
				return err
			}

			for _, r := range binding.Spec.Bindings[0].Resources {
				if resourceHashes[r.Name] == r.Hash {
					return errors.Errorf("resource binding for %s hasn't been updated with new hash", r.Name)
				}
			}

			return nil
		}, timeout).Should(Succeed())

		t.Log("Checking resource ConfigMap 1 has been updated")
		g.Eventually(configMapHasBeenUpdated(env, resourceConfigMap1Key, resourceConfigMap1), timeout).Should(Succeed())

		t.Log("Checking resource ConfigMap 2 has been updated")
		g.Eventually(configMapHasBeenUpdated(env, resourceConfigMap2Key, resourceConfigMap2), timeout).Should(Succeed())
	})

	t.Run("Should reconcile a ClusterResourceSet with ApplyOnce strategy even when one of the resources already exist", func(t *testing.T) {
		g := NewWithT(t)
		ns := setup(t, g)
		defer teardown(t, g, ns)

		t.Log("Updating the cluster with labels")
		testCluster.SetLabels(labels)
		g.Expect(env.Update(ctx, testCluster)).To(Succeed())

		t.Log("Creating resource CM before creating CRS")
		// This CM is defined in the data of "configmapName", which is included in the
		// CRS we will create in this test
		resourceConfigMap1 := configMap(
			resourceConfigMap1Name,
			resourceConfigMapsNamespace,
			map[string]string{
				"created": "before CRS",
			},
		)
		g.Expect(env.Create(ctx, resourceConfigMap1)).To(Succeed())

		t.Log("Creating a ClusterResourceSet instance that has same labels as selector")
		clusterResourceSet := &addonsv1.ClusterResourceSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterResourceSetName,
				Namespace: ns.Name,
			},
			Spec: addonsv1.ClusterResourceSetSpec{
				Strategy: string(addonsv1.ClusterResourceSetStrategyApplyOnce),
				ClusterSelector: metav1.LabelSelector{
					MatchLabels: labels,
				},
				Resources: []addonsv1.ResourceRef{{Name: configmapName, Kind: "ConfigMap"}, {Name: secretName, Kind: "Secret"}},
			},
		}

		g.Expect(env.Create(ctx, clusterResourceSet)).To(Succeed())

		t.Log("Checking resource ConfigMap 1 hasn't been updated")
		resourceConfigMap1Key := client.ObjectKey{
			Namespace: resourceConfigMapsNamespace,
			Name:      resourceConfigMap1Name,
		}
		g.Eventually(configMapHasBeenUpdated(env, resourceConfigMap1Key, resourceConfigMap1), timeout).Should(Succeed())

		t.Log("Verifying resource ConfigMap 2 has been created")
		resourceConfigMap2Key := client.ObjectKey{
			Namespace: resourceConfigMapsNamespace,
			Name:      resourceConfigMap2Name,
		}
		g.Eventually(func() error {
			cm := &corev1.ConfigMap{}
			return env.Get(ctx, resourceConfigMap2Key, cm)
		}, timeout).Should(Succeed())
	})

	t.Run("Should reconcile a ClusterResourceSet with ApplyOnce strategy even when there is an error, after the error has been corrected", func(t *testing.T) {
		// To trigger an error in the middle of the reconciliation, we'll define an object in a namespace that doesn't yet exist.
		// We'll expect the CRS to reconcile all other objects except that one and bubble up the error.
		// Once that happens, we'll go ahead and create the namespace. Then we'll expect the CRS to, eventually, create that remaining object.

		g := NewWithT(t)
		ns := setup(t, g)
		defer teardown(t, g, ns)

		t.Log("Updating the cluster with labels")
		testCluster.SetLabels(labels)
		g.Expect(env.Update(ctx, testCluster)).To(Succeed())

		t.Log("Updating the test config map with the missing namespace resource")
		missingNamespace := randomNamespaceForTest(t)

		resourceConfigMap1 := configMap(
			resourceConfigMap1Name,
			resourceConfigMapsNamespace,
			map[string]string{
				"my_new_config": "some_value",
			},
		)

		resourceConfigMap1Content, err := yaml.Marshal(resourceConfigMap1)
		g.Expect(err).ToNot(HaveOccurred())

		resourceConfigMapWithMissingNamespace := configMap(
			"cm-missing-namespace",
			missingNamespace,
			map[string]string{
				"my_new_config": "this is all new",
			},
		)

		resourceConfigMapMissingNamespaceContent, err := yaml.Marshal(resourceConfigMapWithMissingNamespace)
		g.Expect(err).ToNot(HaveOccurred())

		testConfigmap := configMap(
			configmapName,
			ns.Name,
			map[string]string{
				"cm":             string(resourceConfigMap1Content),
				"problematic_cm": string(resourceConfigMapMissingNamespaceContent),
			},
		)

		g.Expect(env.Update(ctx, testConfigmap)).To(Succeed())

		t.Log("Creating a ClusterResourceSet instance that has same labels as selector")
		clusterResourceSet := &addonsv1.ClusterResourceSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterResourceSetName,
				Namespace: ns.Name,
			},
			Spec: addonsv1.ClusterResourceSetSpec{
				Strategy: string(addonsv1.ClusterResourceSetStrategyApplyOnce),
				ClusterSelector: metav1.LabelSelector{
					MatchLabels: labels,
				},
				Resources: []addonsv1.ResourceRef{{Name: testConfigmap.Name, Kind: "ConfigMap"}, {Name: secretName, Kind: "Secret"}},
			},
		}

		g.Expect(env.Create(ctx, clusterResourceSet)).To(Succeed())

		t.Log("Verifying resource ConfigMap 1 has been created")
		resourceConfigMap1Key := client.ObjectKeyFromObject(resourceConfigMap1)
		g.Eventually(configMapHasBeenUpdated(env, resourceConfigMap1Key, resourceConfigMap1), timeout).Should(Succeed())

		t.Log("Verifying resource ConfigMap 2 has been created")
		resourceConfigMap2Key := client.ObjectKey{
			Namespace: resourceConfigMapsNamespace,
			Name:      resourceConfigMap2Name,
		}
		g.Eventually(func() error {
			cm := &corev1.ConfigMap{}
			return env.Get(ctx, resourceConfigMap2Key, cm)
		}, timeout).Should(Succeed())

		t.Log("Verifying CRS Binding failed marked the resource as not applied")
		g.Eventually(func(g Gomega) {
			clusterResourceSetBindingKey := client.ObjectKey{
				Namespace: testCluster.Namespace,
				Name:      testCluster.Name,
			}
			binding := &addonsv1.ClusterResourceSetBinding{}
			g.Expect(env.Get(ctx, clusterResourceSetBindingKey, binding)).To(Succeed())

			g.Expect(binding.Spec.Bindings).To(HaveLen(1))
			g.Expect(binding.Spec.Bindings[0].Resources).To(HaveLen(2))

			for _, r := range binding.Spec.Bindings[0].Resources {
				switch r.ResourceRef.Name {
				case testConfigmap.Name:
					g.Expect(r.Applied).To(BeFalse(), "test-configmap should be not applied bc of missing namespace")
				case secretName:
					g.Expect(r.Applied).To(BeTrue(), "test-secret should be applied")
				}
			}
		}, timeout).Should(Succeed())

		t.Log("Verifying CRS has a false ResourcesApplied condition")
		g.Eventually(func(g Gomega) {
			clusterResourceSetKey := client.ObjectKeyFromObject(clusterResourceSet)
			crs := &addonsv1.ClusterResourceSet{}
			g.Expect(env.Get(ctx, clusterResourceSetKey, crs)).To(Succeed())

			appliedCondition := conditions.Get(crs, addonsv1.ResourcesAppliedCondition)
			g.Expect(appliedCondition).NotTo(BeNil())
			g.Expect(appliedCondition.Status).To(Equal(corev1.ConditionFalse))
			g.Expect(appliedCondition.Reason).To(Equal(addonsv1.ApplyFailedReason))
			g.Expect(appliedCondition.Message).To(ContainSubstring("creating object /v1, Kind=ConfigMap %s/cm-missing-namespace", missingNamespace))
		}, timeout).Should(Succeed())

		t.Log("Creating missing namespace")
		missingNs := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: missingNamespace,
			},
		}
		g.Expect(env.Create(ctx, missingNs)).To(Succeed())

		t.Log("Verifying CRS Binding has all resources applied")
		g.Eventually(clusterResourceSetBindingReady(env, testCluster), timeout).Should(BeTrue())

		t.Log("Verifying resource ConfigMap with previously missing namespace has been created")
		g.Eventually(configMapHasBeenUpdated(env, client.ObjectKeyFromObject(resourceConfigMapWithMissingNamespace), resourceConfigMapWithMissingNamespace), timeout).Should(Succeed())

		g.Expect(env.Delete(ctx, resourceConfigMapWithMissingNamespace)).To(Succeed())
		g.Expect(env.Delete(ctx, missingNs)).To(Succeed())
	})

	t.Run("Should only apply resources after the remote cluster's Kubernetes API Server Service has been created", func(t *testing.T) {
		g := NewWithT(t)
		ns := setup(t, g)
		defer teardown(t, g, ns)

		fakeService := &corev1.Service{
			TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fake",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name: "https",
						Port: 443,
					},
				},
				Type: "ClusterIP",
			},
		}

		kubernetesAPIServerService := &corev1.Service{}
		t.Log("Verifying Kubernetes API Server Service has been created")
		g.Expect(env.Get(ctx, client.ObjectKey{Name: "kubernetes", Namespace: metav1.NamespaceDefault}, kubernetesAPIServerService)).To(Succeed())

		fakeService.Spec.ClusterIP = kubernetesAPIServerService.Spec.ClusterIP

		t.Log("Let Kubernetes API Server Service fail to create by occupying its IP")
		g.Eventually(func() error {
			err := env.Delete(ctx, kubernetesAPIServerService)
			if err != nil {
				return err
			}
			err = env.Create(ctx, fakeService)
			if err != nil {
				return err
			}
			return nil
		}, timeout).Should(Succeed())
		g.Expect(apierrors.IsNotFound(env.Get(ctx, client.ObjectKeyFromObject(kubernetesAPIServerService), &corev1.Service{}))).To(BeTrue())

		clusterResourceSetInstance := &addonsv1.ClusterResourceSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterResourceSetName,
				Namespace: ns.Name,
			},
			Spec: addonsv1.ClusterResourceSetSpec{
				ClusterSelector: metav1.LabelSelector{
					MatchLabels: labels,
				},
				Resources: []addonsv1.ResourceRef{{Name: secretName, Kind: "Secret"}},
			},
		}
		// Create the ClusterResourceSet.
		g.Expect(env.Create(ctx, clusterResourceSetInstance)).To(Succeed())

		testCluster.SetLabels(labels)
		g.Expect(env.Update(ctx, testCluster)).To(Succeed())

		// Resources are not applied because the Kubernetes API Server Service doesn't exist.
		clusterResourceSetBindingKey := client.ObjectKey{Namespace: testCluster.Namespace, Name: testCluster.Name}
		g.Consistently(func() bool {
			binding := &addonsv1.ClusterResourceSetBinding{}

			if err := env.Get(ctx, clusterResourceSetBindingKey, binding); err != nil {
				// either the binding is not there
				return true
			}
			// or the binding is there but resources are not applied
			for _, b := range binding.Spec.Bindings {
				if len(b.Resources) > 0 {
					return false
				}
			}
			return true
		}, timeout).Should(BeTrue())

		t.Log("Create Kubernetes API Server Service")
		g.Expect(env.Delete(ctx, fakeService)).Should(Succeed())
		kubernetesAPIServerService.ResourceVersion = ""
		g.Expect(env.Create(ctx, kubernetesAPIServerService)).Should(Succeed())

		// Label the CRS to trigger reconciliation.
		labels["new"] = ""
		clusterResourceSetInstance.SetLabels(labels)
		g.Expect(env.Patch(ctx, clusterResourceSetInstance, client.MergeFrom(clusterResourceSetInstance.DeepCopy()))).To(Succeed())

		// Wait until ClusterResourceSetBinding is created for the Cluster
		g.Eventually(func() bool {
			// the binding must exists and track resource being applied
			binding := &addonsv1.ClusterResourceSetBinding{}
			if err := env.Get(ctx, clusterResourceSetBindingKey, binding); err != nil {
				return false
			}
			for _, b := range binding.Spec.Bindings {
				if len(b.Resources) == 0 {
					return false
				}
			}
			return len(binding.Spec.Bindings) != 0
		}, timeout).Should(BeTrue())
	})

	t.Run("Should handle applying multiple ClusterResourceSets concurrently to the same cluster", func(t *testing.T) {
		g := NewWithT(t)
		ns := setup(t, g)
		defer teardown(t, g, ns)

		t.Log("Creating ClusterResourceSet instances that have same labels as selector")
		for i := range 10 {
			configmapName := fmt.Sprintf("%s-%d", configmapName, i)
			secretName := fmt.Sprintf("%s-%d", secretName, i)
			createConfigMapAndSecret(g, ns.Name, configmapName, secretName)

			clusterResourceSetInstance := &addonsv1.ClusterResourceSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("clusterresourceset-%d", i),
					Namespace: ns.Name,
				},
				Spec: addonsv1.ClusterResourceSetSpec{
					ClusterSelector: metav1.LabelSelector{
						MatchLabels: labels,
					},
					Resources: []addonsv1.ResourceRef{{Name: configmapName, Kind: "ConfigMap"}, {Name: secretName, Kind: "Secret"}},
				},
			}
			// Create the ClusterResourceSet.
			g.Expect(env.Create(ctx, clusterResourceSetInstance)).To(Succeed())
		}

		t.Log("Updating the cluster with labels to trigger cluster resource sets to be applied")
		testCluster.SetLabels(labels)
		g.Expect(env.Update(ctx, testCluster)).To(Succeed())

		t.Log("Verifying ClusterResourceSetBinding shows that all CRS have been applied")
		g.Eventually(func(g Gomega) {
			clusterResourceSetBindingKey := client.ObjectKey{Namespace: testCluster.Namespace, Name: testCluster.Name}
			binding := &addonsv1.ClusterResourceSetBinding{}
			g.Expect(env.Get(ctx, clusterResourceSetBindingKey, binding)).Should(Succeed())
			g.Expect(binding.Spec.Bindings).To(HaveLen(10))
			for _, b := range binding.Spec.Bindings {
				g.Expect(b.Resources).To(HaveLen(2))
				for _, r := range b.Resources {
					g.Expect(r.Applied).To(BeTrue())
				}
			}
			g.Expect(binding.OwnerReferences).To(HaveLen(10))
		}, 4*timeout).Should(Succeed())
		t.Log("Deleting the created ClusterResourceSet instances")
		g.Expect(env.DeleteAllOf(ctx, &addonsv1.ClusterResourceSet{}, client.InNamespace(ns.Name))).To(Succeed())
		g.Expect(env.DeleteAllOf(ctx, &addonsv1.ClusterResourceSetBinding{}, client.InNamespace(ns.Name))).To(Succeed())
	})
}

func clusterResourceSetBindingReady(env *envtest.Environment, cluster *clusterv1.Cluster) func() bool {
	return func() bool {
		clusterResourceSetBindingKey := client.ObjectKey{
			Namespace: cluster.Namespace,
			Name:      cluster.Name,
		}
		binding := &addonsv1.ClusterResourceSetBinding{}
		err := env.Get(ctx, clusterResourceSetBindingKey, binding)
		if err != nil {
			return false
		}

		if len(binding.Spec.Bindings) != 1 {
			return false
		}
		if len(binding.Spec.Bindings[0].Resources) != 2 {
			return false
		}

		if !binding.Spec.Bindings[0].Resources[0].Applied || !binding.Spec.Bindings[0].Resources[1].Applied {
			return false
		}

		return binding.Spec.ClusterName == cluster.Name
	}
}

func configMapHasBeenUpdated(env *envtest.Environment, key client.ObjectKey, newState *corev1.ConfigMap) func() error {
	return func() error {
		cm := &corev1.ConfigMap{}
		if err := env.Get(ctx, key, cm); err != nil {
			return err
		}

		if !cmp.Equal(cm.Data, newState.Data) {
			return errors.Errorf("configMap %s hasn't been updated yet", key.Name)
		}

		return nil
	}
}

func configMap(name, namespace string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}
}

func randomNamespaceForTest(tb testing.TB) string {
	tb.Helper()
	// This is just to get a short form of the test name
	// sha1 is totally fine
	h := sha1.New() //nolint: gosec
	h.Write([]byte(tb.Name()))
	testNameHash := fmt.Sprintf("%x", h.Sum(nil))
	return "ns-" + testNameHash[:7] + "-" + util.RandomString(6)
}
