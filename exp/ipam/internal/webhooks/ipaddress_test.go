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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
)

func TestIPAddressValidateCreate(t *testing.T) {
	claim := &ipamv1.IPAddressClaim{
		TypeMeta: metav1.TypeMeta{
			APIVersion: ipamv1.GroupVersion.String(),
			Kind:       "IPAddressClaim",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "claim",
			Namespace: "default",
		},
		Spec: ipamv1.IPAddressClaimSpec{
			PoolRef: corev1.TypedLocalObjectReference{
				Kind:     "TestPool",
				Name:     "pool",
				APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
			},
		},
	}

	getAddress := func(v6 bool, fn func(addr *ipamv1.IPAddress)) ipamv1.IPAddress {
		addr := ipamv1.IPAddress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
			},
			Spec: ipamv1.IPAddressSpec{
				ClaimRef: corev1.LocalObjectReference{Name: claim.Name},
				PoolRef:  claim.Spec.PoolRef,
				Address:  "10.0.0.1",
				Prefix:   24,
				Gateway:  "10.0.0.254",
			},
		}
		if v6 {
			addr.Spec.Address = "42::1"
			addr.Spec.Prefix = 64
			addr.Spec.Gateway = "42::ffff"
		}
		fn(&addr)
		return addr
	}

	tests := []struct {
		name      string
		ip        ipamv1.IPAddress
		extraObjs []client.Object
		expectErr bool
	}{
		{
			name:      "a valid IPv4 Address should be accepted",
			ip:        getAddress(false, func(*ipamv1.IPAddress) {}),
			extraObjs: []client.Object{claim},
			expectErr: false,
		},
		{
			name:      "a valid IPv6 Address should be accepted",
			ip:        getAddress(true, func(*ipamv1.IPAddress) {}),
			extraObjs: []client.Object{claim},
			expectErr: false,
		},
		{
			name: "a prefix that is negative should be rejected",
			ip: getAddress(false, func(addr *ipamv1.IPAddress) {
				addr.Spec.Prefix = -1
			}),
			extraObjs: []client.Object{claim},
			expectErr: true,
		},
		{
			name: "a prefix that is too large for v4 should be rejected",
			ip: getAddress(false, func(addr *ipamv1.IPAddress) {
				addr.Spec.Prefix = 64
			}),
			extraObjs: []client.Object{claim},
			expectErr: true,
		},
		{
			name: "a prefix that is too large for v6 should be rejected",
			ip: getAddress(true, func(addr *ipamv1.IPAddress) {
				addr.Spec.Prefix = 256
			}),
			extraObjs: []client.Object{claim},
			expectErr: true,
		},
		{
			name: "an invalid address should be rejected",
			ip: getAddress(false, func(addr *ipamv1.IPAddress) {
				addr.Spec.Address = "42"
			}),
			extraObjs: []client.Object{claim},
			expectErr: true,
		},
		{
			name: "an invalid gateway should be rejected",
			ip: getAddress(false, func(addr *ipamv1.IPAddress) {
				addr.Spec.Gateway = "42"
			}),
			extraObjs: []client.Object{claim},
			expectErr: true,
		},
		{
			name: "an empty gateway should be allowed",
			ip: getAddress(false, func(addr *ipamv1.IPAddress) {
				addr.Spec.Gateway = ""
			}),
			extraObjs: []client.Object{claim},
			expectErr: false,
		},
		{
			name: "a pool reference that does not match the claim should be rejected",
			ip: getAddress(false, func(addr *ipamv1.IPAddress) {
				addr.Spec.PoolRef.Name = "nothing"
			}),
			extraObjs: []client.Object{claim},
			expectErr: true,
		},
		{
			name: "a pool reference that does not contain a group should be rejected",
			ip: getAddress(false, func(addr *ipamv1.IPAddress) {
				addr.Spec.PoolRef.APIGroup = nil
			}),
			extraObjs: []client.Object{claim},
			expectErr: true,
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			scheme := runtime.NewScheme()
			g.Expect(ipamv1.AddToScheme(scheme)).To(Succeed())
			wh := IPAddress{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.extraObjs...).Build(),
			}
			if tt.expectErr {
				g.Expect(wh.validate(context.Background(), &tt.ip)).NotTo(Succeed())
			} else {
				g.Expect(wh.validate(context.Background(), &tt.ip)).To(Succeed())
			}
		})
	}
}

func TestIPAddressValidateUpdate(t *testing.T) {
	getAddress := func(fn func(addr *ipamv1.IPAddress)) ipamv1.IPAddress {
		addr := ipamv1.IPAddress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
			},
			Spec: ipamv1.IPAddressSpec{
				ClaimRef: corev1.LocalObjectReference{},
				PoolRef:  corev1.TypedLocalObjectReference{},
				Address:  "10.0.0.1",
				Prefix:   24,
				Gateway:  "10.0.0.254",
			},
		}
		fn(&addr)
		return addr
	}

	tests := []struct {
		name      string
		oldIP     ipamv1.IPAddress
		newIP     ipamv1.IPAddress
		extraObjs []client.Object
		expectErr bool
	}{
		{
			name:      "should accept objects with identical spec",
			oldIP:     getAddress(func(*ipamv1.IPAddress) {}),
			newIP:     getAddress(func(*ipamv1.IPAddress) {}),
			expectErr: false,
		},
		{
			name:  "should reject objects with different spec",
			oldIP: getAddress(func(*ipamv1.IPAddress) {}),
			newIP: getAddress(func(addr *ipamv1.IPAddress) {
				addr.Spec.Address = "10.0.0.2"
			}),
			expectErr: true,
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			scheme := runtime.NewScheme()
			g.Expect(ipamv1.AddToScheme(scheme)).To(Succeed())
			wh := IPAddress{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.extraObjs...).Build(),
			}
			warnings, err := wh.ValidateUpdate(context.Background(), &tt.oldIP, &tt.newIP)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(warnings).To(BeEmpty())
		})
	}
}
