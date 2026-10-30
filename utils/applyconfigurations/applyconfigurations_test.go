/*
Copyright 2026 The Kubernetes Authors.

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

// Package applyconfigurations_test verifies the generated ApplyConfigurations against the API types.
package applyconfigurations_test

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"

	addonsv1 "sigs.k8s.io/cluster-api/api/addons/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	addonsac "sigs.k8s.io/cluster-api/utils/applyconfigurations/addons/addons/v1beta2"
	clusterac "sigs.k8s.io/cluster-api/utils/applyconfigurations/core/core/v1beta2"
	ipamac "sigs.k8s.io/cluster-api/utils/applyconfigurations/ipam/ipam/v1beta2"
	runtimeac "sigs.k8s.io/cluster-api/utils/applyconfigurations/runtime/runtime/v1beta2"
)

const fieldManager = "test-manager"

// roundTrip marshals the apply configuration and unmarshals it into the typed object out.
func roundTrip(g *WithT, in, out any) {
	g.THelper()
	raw, err := json.Marshal(in)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(json.Unmarshal(raw, out)).To(Succeed())
}

// ownedFields returns managed fields where fieldManager applied the given FieldsV1 JSON.
func ownedFields(apiVersion, fieldsV1 string) []metav1.ManagedFieldsEntry {
	return []metav1.ManagedFieldsEntry{{
		Manager:    fieldManager,
		Operation:  metav1.ManagedFieldsOperationApply,
		APIVersion: apiVersion,
		FieldsType: "FieldsV1",
		FieldsV1:   &metav1.FieldsV1{Raw: []byte(fieldsV1)},
	}}
}

func TestClusterApplyConfiguration(t *testing.T) {
	g := NewWithT(t)

	ac := clusterac.Cluster("my-cluster", "my-ns").
		WithSpec(clusterac.ClusterSpec().
			WithPaused(true).
			WithControlPlaneRef(clusterac.ContractVersionedObjectReference().
				WithAPIGroup("controlplane.cluster.x-k8s.io").
				WithKind("KubeadmControlPlane").
				WithName("my-cp")).
			WithInfrastructureRef(clusterac.ContractVersionedObjectReference().
				WithAPIGroup("infrastructure.cluster.x-k8s.io").
				WithKind("DockerCluster").
				WithName("my-infra")).
			WithTopology(clusterac.Topology().
				WithClassRef(clusterac.ClusterClassRef().WithName("my-class")).
				WithVersion("v1.30.0")))

	got := &clusterv1.Cluster{}
	roundTrip(g, ac, got)

	g.Expect(got.APIVersion).To(Equal(clusterv1.GroupVersion.String()))
	g.Expect(got.Kind).To(Equal("Cluster"))
	g.Expect(got.Name).To(Equal("my-cluster"))
	g.Expect(got.Namespace).To(Equal("my-ns"))
	g.Expect(got.Spec.Paused).To(Equal(ptrTo(true)))
	g.Expect(got.Spec.ControlPlaneRef).To(Equal(clusterv1.ContractVersionedObjectReference{
		APIGroup: "controlplane.cluster.x-k8s.io",
		Kind:     "KubeadmControlPlane",
		Name:     "my-cp",
	}))
	g.Expect(got.Spec.InfrastructureRef).To(Equal(clusterv1.ContractVersionedObjectReference{
		APIGroup: "infrastructure.cluster.x-k8s.io",
		Kind:     "DockerCluster",
		Name:     "my-infra",
	}))
	g.Expect(got.Spec.Topology.ClassRef.Name).To(Equal("my-class"))
	g.Expect(got.Spec.Topology.Version).To(Equal("v1.30.0"))
}

func TestExtractCluster(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{APIVersion: clusterv1.GroupVersion.String(), Kind: "Cluster"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cluster",
			Namespace: "my-ns",
			ManagedFields: ownedFields(clusterv1.GroupVersion.String(),
				`{"f:spec":{"f:paused":{},"f:topology":{"f:version":{}}}}`),
		},
		Spec: clusterv1.ClusterSpec{
			Paused: ptrTo(true),
			Topology: clusterv1.Topology{
				Version: "v1.30.0",
				// Not owned by fieldManager, so it must not be extracted.
				ClassRef: clusterv1.ClusterClassRef{Name: "other-owner"},
			},
		},
	}

	ac, err := clusterac.ExtractCluster(cluster, fieldManager)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(ac.Name).To(HaveValue(Equal("my-cluster")))
	g.Expect(ac.Namespace).To(HaveValue(Equal("my-ns")))
	g.Expect(ac.Spec).ToNot(BeNil())
	g.Expect(ac.Spec.Paused).To(HaveValue(BeTrue()))
	g.Expect(ac.Spec.Topology).ToNot(BeNil())
	g.Expect(ac.Spec.Topology.Version).To(HaveValue(Equal("v1.30.0")))
	g.Expect(ac.Spec.Topology.ClassRef).To(BeNil())
}

func TestClusterResourceSetApplyConfiguration(t *testing.T) {
	g := NewWithT(t)

	ac := addonsac.ClusterResourceSet("my-crs", "my-ns").
		WithSpec(addonsac.ClusterResourceSetSpec().
			WithClusterSelector(metav1ac.LabelSelector().WithMatchLabels(map[string]string{"cni": "calico"})).
			WithStrategy(string(addonsv1.ClusterResourceSetStrategyReconcile)).
			WithResources(addonsac.ResourceRef().WithName("calico").WithKind(string(addonsv1.ConfigMapClusterResourceSetResourceKind))))

	got := &addonsv1.ClusterResourceSet{}
	roundTrip(g, ac, got)

	g.Expect(got.APIVersion).To(Equal(addonsv1.GroupVersion.String()))
	g.Expect(got.Kind).To(Equal("ClusterResourceSet"))
	g.Expect(got.Name).To(Equal("my-crs"))
	g.Expect(got.Namespace).To(Equal("my-ns"))
	g.Expect(got.Spec.ClusterSelector.MatchLabels).To(Equal(map[string]string{"cni": "calico"}))
	g.Expect(got.Spec.Strategy).To(Equal(string(addonsv1.ClusterResourceSetStrategyReconcile)))
	g.Expect(got.Spec.Resources).To(Equal([]addonsv1.ResourceRef{{Name: "calico", Kind: "ConfigMap"}}))
}

func TestExtractClusterResourceSet(t *testing.T) {
	g := NewWithT(t)

	crs := &addonsv1.ClusterResourceSet{
		TypeMeta: metav1.TypeMeta{APIVersion: addonsv1.GroupVersion.String(), Kind: "ClusterResourceSet"},
		ObjectMeta: metav1.ObjectMeta{
			Name:          "my-crs",
			Namespace:     "my-ns",
			ManagedFields: ownedFields(addonsv1.GroupVersion.String(), `{"f:spec":{"f:strategy":{}}}`),
		},
		Spec: addonsv1.ClusterResourceSetSpec{Strategy: "Reconcile"},
	}

	ac, err := addonsac.ExtractClusterResourceSet(crs, fieldManager)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ac.Spec).ToNot(BeNil())
	g.Expect(ac.Spec.Strategy).To(HaveValue(Equal("Reconcile")))
}

func TestIPAddressApplyConfiguration(t *testing.T) {
	g := NewWithT(t)

	ac := ipamac.IPAddress("my-ip", "my-ns").
		WithSpec(ipamac.IPAddressSpec().
			WithAddress("10.0.0.10").
			WithPrefix(24).
			WithGateway("10.0.0.1").
			WithClaimRef(ipamac.IPAddressClaimReference().WithName("my-claim")).
			WithPoolRef(ipamac.IPPoolReference().
				WithAPIGroup("ipam.example.com").
				WithKind("Pool").
				WithName("my-pool")))

	got := &ipamv1.IPAddress{}
	roundTrip(g, ac, got)

	g.Expect(got.APIVersion).To(Equal(ipamv1.GroupVersion.String()))
	g.Expect(got.Kind).To(Equal("IPAddress"))
	g.Expect(got.Name).To(Equal("my-ip"))
	g.Expect(got.Namespace).To(Equal("my-ns"))
	g.Expect(got.Spec.Address).To(Equal("10.0.0.10"))
	g.Expect(got.Spec.Prefix).To(Equal(ptrTo(int32(24))))
	g.Expect(got.Spec.Gateway).To(Equal("10.0.0.1"))
	g.Expect(got.Spec.ClaimRef.Name).To(Equal("my-claim"))
	g.Expect(got.Spec.PoolRef).To(Equal(ipamv1.IPPoolReference{
		APIGroup: "ipam.example.com",
		Kind:     "Pool",
		Name:     "my-pool",
	}))
}

func TestExtensionConfigApplyConfiguration(t *testing.T) {
	g := NewWithT(t)

	ac := runtimeac.ExtensionConfig("my-ext").
		WithSpec(runtimeac.ExtensionConfigSpec().
			WithSettings(map[string]string{"key": "value"}))

	got := &runtimev1.ExtensionConfig{}
	roundTrip(g, ac, got)

	g.Expect(got.APIVersion).To(Equal(runtimev1.GroupVersion.String()))
	g.Expect(got.Kind).To(Equal("ExtensionConfig"))
	g.Expect(got.Name).To(Equal("my-ext"))
	g.Expect(got.Spec.Settings).To(Equal(map[string]string{"key": "value"}))
}

func ptrTo[T any](v T) *T { return &v }
