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

func TestClusterResourceSetBindingClusterNameImmutable(t *testing.T) {
	g := NewWithT(t)

	crsb := &addonsv1.ClusterResourceSetBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "crsb-clustername-immutable",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: addonsv1.ClusterResourceSetBindingSpec{
			ClusterName: "cluster1",
		},
	}

	g.Expect(env.CreateAndWait(ctx, crsb)).To(Succeed())
	t.Cleanup(func() {
		g.Expect(env.CleanupAndWait(ctx, crsb)).To(Succeed())
	})

	actualCRSB := &addonsv1.ClusterResourceSetBinding{}
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: crsb.Namespace, Name: crsb.Name}, actualCRSB)).To(Succeed())

	// Changing spec.clusterName on update must be rejected by the CEL immutability rule.
	actualCRSB.Spec.ClusterName = "cluster2"
	err := env.Update(ctx, actualCRSB)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("field is immutable"))

	// An update that leaves spec.clusterName unchanged must still be allowed.
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: crsb.Namespace, Name: crsb.Name}, actualCRSB)).To(Succeed())
	actualCRSB.Spec.Bindings = []addonsv1.ResourceSetBinding{
		{ClusterResourceSetName: "some-crs"},
	}
	g.Expect(env.Update(ctx, actualCRSB)).To(Succeed())
}
