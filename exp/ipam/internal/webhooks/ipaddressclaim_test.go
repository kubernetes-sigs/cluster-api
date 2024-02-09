/*
Copyright 2022 The Kubernetes Authors.

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

package webhooks

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
)

func TestIPAddressClaimValidateCreate(t *testing.T) {
	getClaim := func(fn func(addr *ipamv1.IPAddressClaim)) ipamv1.IPAddressClaim {
		claim := ipamv1.IPAddressClaim{
			Spec: ipamv1.IPAddressClaimSpec{
				PoolRef: corev1.TypedLocalObjectReference{
					Name:     "identical",
					Kind:     "TestPool",
					APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
				},
			},
		}
		fn(&claim)
		return claim
	}

	tests := []struct {
		name      string
		claim     ipamv1.IPAddressClaim
		expectErr bool
	}{
		{
			name:      "should accept a valid claim",
			claim:     getClaim(func(*ipamv1.IPAddressClaim) {}),
			expectErr: false,
		},
		{
			name: "should reject a pool reference without a group",
			claim: getClaim(func(addr *ipamv1.IPAddressClaim) {
				addr.Spec.PoolRef.APIGroup = nil
			}),
			expectErr: true,
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			wh := IPAddressClaim{}
			warnings, err := wh.ValidateCreate(context.Background(), &tt.claim)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(warnings).To(BeEmpty())
		})
	}
}

func TestIPAddressClaimValidateUpdate(t *testing.T) {
	getClaim := func(fn func(addr *ipamv1.IPAddressClaim)) ipamv1.IPAddressClaim {
		claim := ipamv1.IPAddressClaim{
			Spec: ipamv1.IPAddressClaimSpec{
				PoolRef: corev1.TypedLocalObjectReference{
					Name: "identical",
				},
			},
		}
		fn(&claim)
		return claim
	}

	tests := []struct {
		name      string
		oldClaim  ipamv1.IPAddressClaim
		newClaim  ipamv1.IPAddressClaim
		expectErr bool
	}{
		{
			name:      "should accept objects with identical spec",
			oldClaim:  getClaim(func(*ipamv1.IPAddressClaim) {}),
			newClaim:  getClaim(func(*ipamv1.IPAddressClaim) {}),
			expectErr: false,
		},
		{
			name:     "should reject objects with different spec",
			oldClaim: getClaim(func(*ipamv1.IPAddressClaim) {}),
			newClaim: getClaim(func(addr *ipamv1.IPAddressClaim) {
				addr.Spec.PoolRef.Name = "different"
			}),
			expectErr: true,
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			wh := IPAddressClaim{}
			warnings, err := wh.ValidateUpdate(context.Background(), &tt.oldClaim, &tt.newClaim)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(warnings).To(BeEmpty())
		})
	}
}
