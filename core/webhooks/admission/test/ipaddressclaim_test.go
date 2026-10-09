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

	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
)

func TestIPAddressClaimSpecImmutable(t *testing.T) {
	g := NewWithT(t)

	claim := &ipamv1.IPAddressClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ipaddressclaim-spec-immutable",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: ipamv1.IPAddressClaimSpec{
			PoolRef: ipamv1.IPPoolReference{
				Name:     "identical",
				Kind:     "TestPool",
				APIGroup: "ipam.cluster.x-k8s.io",
			},
		},
	}

	g.Expect(env.CreateAndWait(ctx, claim)).To(Succeed())
	t.Cleanup(func() {
		g.Expect(env.CleanupAndWait(ctx, claim)).To(Succeed())
	})

	actualClaim := &ipamv1.IPAddressClaim{}
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: claim.Namespace, Name: claim.Name}, actualClaim)).To(Succeed())

	// Changing spec.poolRef on update must be rejected by the CEL immutability rule.
	actualClaim.Spec.PoolRef.Name = "different"
	err := env.Update(ctx, actualClaim)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("field is immutable"))

	// An update that leaves spec unchanged must still be allowed.
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: claim.Namespace, Name: claim.Name}, actualClaim)).To(Succeed())
	if actualClaim.Annotations == nil {
		actualClaim.Annotations = map[string]string{}
	}
	actualClaim.Annotations["test.cluster.x-k8s.io/unrelated"] = "value"
	g.Expect(env.Update(ctx, actualClaim)).To(Succeed())
}
