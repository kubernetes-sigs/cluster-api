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

package admission

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
)

func TestIPAddressClaimValidateCreate(t *testing.T) {
	getClaim := func(fn func(addr *ipamv1.IPAddressClaim)) ipamv1.IPAddressClaim {
		claim := ipamv1.IPAddressClaim{
			Spec: ipamv1.IPAddressClaimSpec{
				PoolRef: ipamv1.IPPoolReference{
					Name:     "identical",
					Kind:     "TestPool",
					APIGroup: "ipam.cluster.x-k8s.io",
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
