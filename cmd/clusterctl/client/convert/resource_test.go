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

package convert

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime/schema"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/scheme"
)

func TestConvertResource(t *testing.T) {
	targetGV := clusterv1.GroupVersion

	t.Run("convert v1beta1 Cluster to v1beta2", func(t *testing.T) {
		g := NewWithT(t)

		cluster := &clusterv1beta1.Cluster{}
		cluster.SetName("test-cluster")
		cluster.SetNamespace("default")
		cluster.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "cluster.x-k8s.io",
			Version: "v1beta1",
			Kind:    "Cluster",
		})

		convertedObj, err := convertResource(cluster, targetGV, scheme.Scheme)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(convertedObj).ToNot(BeNil())

		convertedCluster, ok := convertedObj.(*clusterv1.Cluster)
		g.Expect(ok).To(BeTrue())
		g.Expect(convertedCluster.Name).To(Equal("test-cluster"))
		g.Expect(convertedCluster.GetObjectKind().GroupVersionKind().Version).To(Equal("v1beta2"))
	})

	t.Run("no-op for v1beta2 resource", func(t *testing.T) {
		g := NewWithT(t)

		cluster := &clusterv1.Cluster{}
		cluster.SetName("test-cluster")
		cluster.SetNamespace("default")
		cluster.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "cluster.x-k8s.io",
			Version: "v1beta2",
			Kind:    "Cluster",
		})

		convertedObj, err := convertResource(cluster, targetGV, scheme.Scheme)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(convertedObj).To(BeIdenticalTo(cluster))
	})

	t.Run("convert v1beta1 MachineDeployment to v1beta2", func(t *testing.T) {
		g := NewWithT(t)

		md := &clusterv1beta1.MachineDeployment{}
		md.SetName("test-md")
		md.SetNamespace("default")
		md.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "cluster.x-k8s.io",
			Version: "v1beta1",
			Kind:    "MachineDeployment",
		})

		convertedObj, err := convertResource(md, targetGV, scheme.Scheme)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(convertedObj).ToNot(BeNil())

		convertedMD, ok := convertedObj.(*clusterv1.MachineDeployment)
		g.Expect(ok).To(BeTrue())
		g.Expect(convertedMD.Name).To(Equal("test-md"))
	})
}
