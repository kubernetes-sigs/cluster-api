/*
Copyright 2025 The Kubernetes Authors.

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

package test

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1 "sigs.k8s.io/cluster-api/api/addons/v1beta2"
)

func TestClusterResourceSetStrategyImmutable(t *testing.T) {
	g := NewWithT(t)

	crs := &addonsv1.ClusterResourceSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "crs-strategy-immutable",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: addonsv1.ClusterResourceSetSpec{
			ClusterSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"foo": "bar"},
			},
			Resources: []addonsv1.ResourceRef{
				{Name: "crs-strategy-immutable-resource", Kind: "Secret"},
			},
			Strategy: string(addonsv1.ClusterResourceSetStrategyApplyOnce),
		},
	}

	g.Expect(env.CreateAndWait(ctx, crs)).To(Succeed())
	t.Cleanup(func() {
		g.Expect(env.CleanupAndWait(ctx, crs)).To(Succeed())
	})

	actualCRS := &addonsv1.ClusterResourceSet{}
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: crs.Namespace, Name: crs.Name}, actualCRS)).To(Succeed())

	// Changing spec.strategy on update must be rejected by the CEL immutability rule.
	actualCRS.Spec.Strategy = string(addonsv1.ClusterResourceSetStrategyReconcile)
	err := env.Update(ctx, actualCRS)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("field is immutable"))

	// An update that leaves spec.strategy unchanged must still be allowed.
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: crs.Namespace, Name: crs.Name}, actualCRS)).To(Succeed())
	if actualCRS.Annotations == nil {
		actualCRS.Annotations = map[string]string{}
	}
	actualCRS.Annotations["test.cluster.x-k8s.io/unrelated"] = "value"
	g.Expect(env.Update(ctx, actualCRS)).To(Succeed())
}

func TestClusterResourceSetClusterSelectorImmutable(t *testing.T) {
	g := NewWithT(t)

	crs := &addonsv1.ClusterResourceSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "crs-clusterselector-immutable",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: addonsv1.ClusterResourceSetSpec{
			ClusterSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"foo": "bar"},
			},
			Resources: []addonsv1.ResourceRef{
				{Name: "crs-clusterselector-immutable-resource", Kind: "Secret"},
			},
			Strategy: string(addonsv1.ClusterResourceSetStrategyApplyOnce),
		},
	}

	g.Expect(env.CreateAndWait(ctx, crs)).To(Succeed())
	t.Cleanup(func() {
		g.Expect(env.CleanupAndWait(ctx, crs)).To(Succeed())
	})

	actualCRS := &addonsv1.ClusterResourceSet{}
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: crs.Namespace, Name: crs.Name}, actualCRS)).To(Succeed())

	// Changing spec.clusterSelector on update must be rejected by the CEL immutability rule.
	actualCRS.Spec.ClusterSelector = metav1.LabelSelector{MatchLabels: map[string]string{"foo": "different"}}
	err := env.Update(ctx, actualCRS)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("field is immutable"))

	// An update that leaves spec.clusterSelector unchanged must still be allowed.
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: crs.Namespace, Name: crs.Name}, actualCRS)).To(Succeed())
	if actualCRS.Annotations == nil {
		actualCRS.Annotations = map[string]string{}
	}
	actualCRS.Annotations["test.cluster.x-k8s.io/unrelated"] = "value"
	g.Expect(env.Update(ctx, actualCRS)).To(Succeed())
}
