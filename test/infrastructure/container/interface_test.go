/*
Copyright 2021 The Kubernetes Authors.

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

package container

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestFilterBuildKeyValue(t *testing.T) {
	g := NewWithT(t)

	filters := FilterBuilder{}
	filters.AddKeyValue("key1", "value1")

	g.Expect(filters).To(Equal(FilterBuilder{"key1": {"value1": []string{""}}}))
}

func TestFilterBuildKeyNameValue(t *testing.T) {
	g := NewWithT(t)

	filters := FilterBuilder{}
	filters.AddKeyNameValue("key1", "name1", "value1")

	g.Expect(filters).To(Equal(FilterBuilder{"key1": {"name1": []string{"value1"}}}))
}

func TestFakeContext(t *testing.T) {
	g := NewWithT(t)
	fake := FakeRuntime{}
	ctx := RuntimeInto(context.Background(), &fake)
	rtc, err := RuntimeFrom(ctx)

	g.Expect(err).ShouldNot(HaveOccurred())

	_, ok := rtc.(*FakeRuntime)
	g.Expect(ok).To(BeTrue())
}

func TestDockerContext(t *testing.T) {
	g := NewWithT(t)
	docker := dockerRuntime{}
	ctx := RuntimeInto(context.Background(), &docker)
	rtc, err := RuntimeFrom(ctx)

	g.Expect(err).ShouldNot(HaveOccurred())

	_, ok := rtc.(*dockerRuntime)
	g.Expect(ok).To(BeTrue())
}

func TestInvalidContext(t *testing.T) {
	g := NewWithT(t)
	_, err := RuntimeFrom(context.Background())
	g.Expect(err).Should(HaveOccurred())
}

func TestGetClusterIPFamily(t *testing.T) {
	clusterWithNetwork := func(podCIDRs, serviceCIDRs []string) *clusterv1.Cluster {
		return &clusterv1.Cluster{
			Spec: clusterv1.ClusterSpec{
				ClusterNetwork: &clusterv1.ClusterNetwork{
					Pods: &clusterv1.NetworkRanges{
						CIDRBlocks: podCIDRs,
					},
					Services: &clusterv1.NetworkRanges{
						CIDRBlocks: serviceCIDRs,
					},
				},
			},
		}
	}

	validAndUnambiguous := []struct {
		name      string
		expectRes ClusterIPFamily
		c         *clusterv1.Cluster
	}{
		{
			name:      "pods: ipv4, services: ipv4",
			expectRes: IPv4IPFamily,
			c:         clusterWithNetwork([]string{"192.168.0.0/16"}, []string{"10.128.0.0/12"}),
		},
		{
			name:      "pods: ipv4, services: nil",
			expectRes: IPv4IPFamily,
			c:         clusterWithNetwork([]string{"192.168.0.0/16"}, nil),
		},
		{
			name:      "pods: ipv6, services: nil",
			expectRes: IPv6IPFamily,
			c:         clusterWithNetwork([]string{"fd00:100:96::/48"}, nil),
		},
		{
			name:      "pods: ipv6, services: ipv6",
			expectRes: IPv6IPFamily,
			c:         clusterWithNetwork([]string{"fd00:100:96::/48"}, []string{"fd00:100:64::/108"}),
		},
		{
			name:      "pods: dual-stack, services: nil",
			expectRes: DualStackIPFamily,
			c:         clusterWithNetwork([]string{"192.168.0.0/16", "fd00:100:96::/48"}, nil),
		},
		{
			name:      "pods: dual-stack, services: ipv4",
			expectRes: DualStackIPFamily,
			c:         clusterWithNetwork([]string{"192.168.0.0/16", "fd00:100:96::/48"}, []string{"10.128.0.0/12"}),
		},
		{
			name:      "pods: dual-stack, services: ipv6",
			expectRes: DualStackIPFamily,
			c:         clusterWithNetwork([]string{"192.168.0.0/16", "fd00:100:96::/48"}, []string{"fd00:100:64::/108"}),
		},
		{
			name:      "pods: dual-stack, services: dual-stack",
			expectRes: DualStackIPFamily,
			c:         clusterWithNetwork([]string{"192.168.0.0/16", "fd00:100:96::/48"}, []string{"10.128.0.0/12", "fd00:100:64::/108"}),
		},
		{
			name:      "pods: nil, services: dual-stack",
			expectRes: DualStackIPFamily,
			c:         clusterWithNetwork(nil, []string{"10.128.0.0/12", "fd00:100:64::/108"}),
		},
	}

	for _, tt := range validAndUnambiguous {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ipFamily, err := GetClusterIPFamily(tt.c)
			g.Expect(ipFamily).To(Equal(tt.expectRes))
			g.Expect(err).ToNot(HaveOccurred())
		})
	}

	validButAmbiguous := []struct {
		name      string
		expectRes ClusterIPFamily
		c         *clusterv1.Cluster
	}{
		{
			name: "pods: nil, services: nil",
			// this could  be ipv4, ipv6, or dual-stack; assume ipv4 for now though
			expectRes: IPv4IPFamily,
			c:         clusterWithNetwork(nil, nil),
		},
		{
			name: "pods: nil, services: ipv4",
			// this could be a dual-stack; assume ipv4 for now though
			expectRes: IPv4IPFamily,
			c:         clusterWithNetwork(nil, []string{"10.128.0.0/12"}),
		},
		{
			name: "pods: nil, services: ipv6",
			// this could be dual-stack; assume ipv6 for now though
			expectRes: IPv6IPFamily,
			c:         clusterWithNetwork(nil, []string{"fd00:100:64::/108"}),
		},
	}

	for _, tt := range validButAmbiguous {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ipFamily, err := GetClusterIPFamily(tt.c)
			g.Expect(ipFamily).To(Equal(tt.expectRes))
			g.Expect(err).ToNot(HaveOccurred())
		})
	}

	invalid := []struct {
		name      string
		expectErr string
		c         *clusterv1.Cluster
	}{
		{
			name:      "pods: ipv4, services: ipv6",
			expectErr: "pods and services IP family mismatch",
			c:         clusterWithNetwork([]string{"192.168.0.0/16"}, []string{"fd00:100:64::/108"}),
		},
		{
			name:      "pods: ipv6, services: ipv4",
			expectErr: "pods and services IP family mismatch",
			c:         clusterWithNetwork([]string{"fd00:100:96::/48"}, []string{"10.128.0.0/12"}),
		},
		{
			name:      "pods: ipv6, services: dual-stack",
			expectErr: "pods and services IP family mismatch",
			c:         clusterWithNetwork([]string{"fd00:100:96::/48"}, []string{"10.128.0.0/12", "fd00:100:64::/108"}),
		},
		{
			name:      "pods: ipv4, services: dual-stack",
			expectErr: "pods and services IP family mismatch",
			c:         clusterWithNetwork([]string{"192.168.0.0/16"}, []string{"10.128.0.0/12", "fd00:100:64::/108"}),
		},
		{
			name:      "pods: ipv4, services: dual-stack",
			expectErr: "pods and services IP family mismatch",
			c:         clusterWithNetwork([]string{"192.168.0.0/16"}, []string{"10.128.0.0/12", "fd00:100:64::/108"}),
		},
		{
			name:      "pods: bad cidr",
			expectErr: "pods: could not parse CIDR",
			c:         clusterWithNetwork([]string{"foo"}, nil),
		},
		{
			name:      "services: bad cidr",
			expectErr: "services: could not parse CIDR",
			c:         clusterWithNetwork([]string{"192.168.0.0/16"}, []string{"foo"}),
		},
		{
			name:      "pods: too many cidrs",
			expectErr: "pods: too many CIDRs specified",
			c:         clusterWithNetwork([]string{"192.168.0.0/16", "fd00:100:96::/48", "10.128.0.0/12"}, nil),
		},
		{
			name:      "services: too many cidrs",
			expectErr: "services: too many CIDRs specified",
			c:         clusterWithNetwork(nil, []string{"192.168.0.0/16", "fd00:100:96::/48", "10.128.0.0/12"}),
		},
	}

	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ipFamily, err := GetClusterIPFamily(tt.c)
			g.Expect(err).To(HaveOccurred())
			g.Expect(err).To(MatchError(ContainSubstring(tt.expectErr)))
			g.Expect(ipFamily).To(Equal(InvalidIPFamily))
		})
	}
}
