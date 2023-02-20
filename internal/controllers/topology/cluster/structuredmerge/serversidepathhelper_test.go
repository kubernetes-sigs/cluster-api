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

package structuredmerge

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/test/builder"
	"sigs.k8s.io/cluster-api/util/patch"
)

// NOTE: This test ensures the ServerSideApply works as expected when the object is co-authored by other controllers.
func TestServerSideApply(t *testing.T) {
	g := NewWithT(t)

	// Write the config file to access the test env for debugging.
	// g.Expect(os.WriteFile("test.conf", kubeconfig.FromEnvTestConfig(env.Config, &clusterv1.Cluster{
	// 	ObjectMeta: metav1.ObjectMeta{Name: "test"},
	// }), 0777)).To(Succeed())

	// Create a namespace for running the test
	ns, err := env.CreateNamespace(ctx, "ssa")
	g.Expect(err).ToNot(HaveOccurred())

	// Build the test object to work with.
	obj := builder.TestInfrastructureCluster(ns.Name, "obj1").WithSpecFields(map[string]interface{}{
		"spec.controlPlaneEndpoint.host": "1.2.3.4",
		"spec.controlPlaneEndpoint.port": int64(1234),
		"spec.foo":                       "", // this field is then explicitly ignored by the patch helper
	}).Build()
	g.Expect(unstructured.SetNestedField(obj.Object, "", "status", "foo")).To(Succeed()) // this field is then ignored by the patch helper (not allowed path).

	t.Run("Server side apply detect changes on object creation (unstructured)", func(t *testing.T) {
		g := NewWithT(t)

		var original *unstructured.Unstructured
		modified := obj.DeepCopy()

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient())
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeTrue())
		g.Expect(p0.HasSpecChanges()).To(BeTrue())
	})
	t.Run("Server side apply detect changes on object creation (typed)", func(t *testing.T) {
		g := NewWithT(t)

		var original *clusterv1.MachineDeployment
		modified := obj.DeepCopy()

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient())
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeTrue())
		g.Expect(p0.HasSpecChanges()).To(BeTrue())
	})
	t.Run("When creating an object using server side apply, it should track managed fields for the topology controller", func(t *testing.T) {
		g := NewWithT(t)

		// Create a patch helper with original == nil and modified == obj, ensure this is detected as operation that triggers changes.
		p0, err := NewServerSidePatchHelper(ctx, nil, obj.DeepCopy(), env.GetClient(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeTrue())
		g.Expect(p0.HasSpecChanges()).To(BeTrue())

		// Create the object using server side apply
		g.Expect(p0.Patch(ctx)).To(Succeed())

		// Check the object and verify managed field are properly set.
		got := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(got), got)).To(Succeed())
		fieldV1 := getTopologyManagedFields(got)
		g.Expect(fieldV1).ToNot(BeEmpty())
		g.Expect(fieldV1).To(HaveKey("f:spec"))      // topology controller should express opinions on spec.
		g.Expect(fieldV1).ToNot(HaveKey("f:status")) // topology controller should not express opinions on status/not allowed paths.

		specFieldV1 := fieldV1["f:spec"].(map[string]interface{})
		g.Expect(specFieldV1).ToNot(BeEmpty())
		g.Expect(specFieldV1).To(HaveKey("f:controlPlaneEndpoint")) // topology controller should express opinions on spec.controlPlaneEndpoint.
		g.Expect(specFieldV1).ToNot(HaveKey("f:foo"))               // topology controller should not express opinions on ignore paths.

		controlPlaneEndpointFieldV1 := specFieldV1["f:controlPlaneEndpoint"].(map[string]interface{})
		g.Expect(controlPlaneEndpointFieldV1).ToNot(BeEmpty())
		g.Expect(controlPlaneEndpointFieldV1).To(HaveKey("f:host")) // topology controller should express opinions on spec.controlPlaneEndpoint.host.
		g.Expect(controlPlaneEndpointFieldV1).To(HaveKey("f:port")) // topology controller should express opinions on spec.controlPlaneEndpoint.port.
	})
	t.Run("Server side apply patch helper detects no changes", func(t *testing.T) {
		g := NewWithT(t)

		// Get the current object (assumes tests to be run in sequence).
		original := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

		// Create a patch helper for a modified object with no changes.
		modified := obj.DeepCopy()
		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeFalse())
		g.Expect(p0.HasSpecChanges()).To(BeFalse())
	})

	t.Run("Server side apply patch helper discard changes in not allowed fields, e.g. status", func(t *testing.T) {
		g := NewWithT(t)

		// Get the current object (assumes tests to be run in sequence).
		original := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

		// Create a patch helper for a modified object with changes only in status.
		modified := obj.DeepCopy()
		g.Expect(unstructured.SetNestedField(modified.Object, "changed", "status", "foo")).To(Succeed())

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeFalse())
		g.Expect(p0.HasSpecChanges()).To(BeFalse())
	})

	t.Run("Server side apply patch helper detect changes", func(t *testing.T) {
		g := NewWithT(t)

		// Get the current object (assumes tests to be run in sequence).
		original := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

		// Create a patch helper for a modified object with changes in spec.
		modified := obj.DeepCopy()
		g.Expect(unstructured.SetNestedField(modified.Object, "changed", "spec", "bar")).To(Succeed())

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeTrue())
		g.Expect(p0.HasSpecChanges()).To(BeTrue())
	})

	t.Run("Server side apply patch helper detect changes impacting only metadata.labels", func(t *testing.T) {
		g := NewWithT(t)

		// Get the current object (assumes tests to be run in sequence).
		original := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

		// Create a patch helper for a modified object with changes only in metadata.
		modified := obj.DeepCopy()
		modified.SetLabels(map[string]string{"foo": "changed"})

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeTrue())
		g.Expect(p0.HasSpecChanges()).To(BeFalse())
	})

	t.Run("Server side apply patch helper detect changes impacting only metadata.annotations", func(t *testing.T) {
		g := NewWithT(t)

		// Get the current object (assumes tests to be run in sequence).
		original := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

		// Create a patch helper for a modified object with changes only in metadata.
		modified := obj.DeepCopy()
		modified.SetAnnotations(map[string]string{"foo": "changed"})

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeTrue())
		g.Expect(p0.HasSpecChanges()).To(BeFalse())
	})

	t.Run("Server side apply patch helper detect changes impacting only metadata.ownerReferences", func(t *testing.T) {
		g := NewWithT(t)

		// Get the current object (assumes tests to be run in sequence).
		original := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

		// Create a patch helper for a modified object with changes only in metadata.
		modified := obj.DeepCopy()
		modified.SetOwnerReferences([]metav1.OwnerReference{
			{
				APIVersion: "foo/v1alpha1",
				Kind:       "foo",
				Name:       "foo",
				UID:        "foo",
			},
		})

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeTrue())
		g.Expect(p0.HasSpecChanges()).To(BeFalse())
	})

	t.Run("Server side apply patch helper discard changes in ignore paths", func(t *testing.T) {
		g := NewWithT(t)

		// Get the current object (assumes tests to be run in sequence).
		original := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

		// Create a patch helper for a modified object with changes only in an ignoredField.
		modified := obj.DeepCopy()
		g.Expect(unstructured.SetNestedField(modified.Object, "changed", "spec", "foo")).To(Succeed())

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeFalse())
		g.Expect(p0.HasSpecChanges()).To(BeFalse())
	})

	t.Run("Another controller applies changes", func(t *testing.T) {
		g := NewWithT(t)

		// Get the current object (assumes tests to be run in sequence).
		obj := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())

		// Store object before another controller applies changes.
		original := obj.DeepCopy()

		// Create a patch helper like we do/recommend doing in the controllers and use it to apply some changes.
		p, err := patch.NewHelper(obj, env.Client)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(unstructured.SetNestedField(obj.Object, "changed", "spec", "foo")).To(Succeed())   // Controller sets a well known field ignored in the topology controller
		g.Expect(unstructured.SetNestedField(obj.Object, "changed", "spec", "bar")).To(Succeed())   // Controller sets an infra specific field the topology controller is not aware of
		g.Expect(unstructured.SetNestedField(obj.Object, "changed", "status", "foo")).To(Succeed()) // Controller sets something in status
		g.Expect(unstructured.SetNestedField(obj.Object, true, "status", "ready")).To(Succeed())    // Required field

		g.Expect(p.Patch(ctx, obj)).To(Succeed())

		// Verify that the topology controller detects no changes after another controller changed fields.
		// Note: We verify here that the ServerSidePatchHelper ignores changes in managed fields of other controllers.
		//       There's also a change in .spec.bar that is intentionally not ignored by the controller, we have to ignore
		//       it here to be able to verify that managed field changes are ignored. This is the same situation as when
		//       other controllers update .status (that is ignored) and the ServerSidePatchHelper then ignores the corresponding
		//       managed field changes.
		p0, err := NewServerSidePatchHelper(ctx, original, original, env.GetClient(), IgnorePaths{{"spec", "foo"}, {"spec", "bar"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeFalse())
		g.Expect(p0.HasSpecChanges()).To(BeFalse())
	})

	t.Run("Topology controller reconcile again with no changes on topology managed fields", func(t *testing.T) {
		g := NewWithT(t)

		// Get the current object (assumes tests to be run in sequence).
		original := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

		// Create a patch helper for a modified object with no changes to what previously applied by th topology manager.
		modified := obj.DeepCopy()

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeFalse())
		g.Expect(p0.HasSpecChanges()).To(BeFalse())

		// Change the object using server side apply
		g.Expect(p0.Patch(ctx)).To(Succeed())

		// Check the object and verify fields set by the other controller are preserved.
		got := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(got), got)).To(Succeed())

		// Check if resourceVersion stayed the same
		g.Expect(got.GetResourceVersion()).To(Equal(original.GetResourceVersion()))

		v1, _, _ := unstructured.NestedString(got.Object, "spec", "foo")
		g.Expect(v1).To(Equal("changed"))
		v2, _, _ := unstructured.NestedString(got.Object, "spec", "bar")
		g.Expect(v2).To(Equal("changed"))
		v3, _, _ := unstructured.NestedString(got.Object, "status", "foo")
		g.Expect(v3).To(Equal("changed"))
		v4, _, _ := unstructured.NestedBool(got.Object, "status", "ready")
		g.Expect(v4).To(BeTrue())

		fieldV1 := getTopologyManagedFields(got)
		g.Expect(fieldV1).ToNot(BeEmpty())
		g.Expect(fieldV1).To(HaveKey("f:spec"))      // topology controller should express opinions on spec.
		g.Expect(fieldV1).ToNot(HaveKey("f:status")) // topology controller should not express opinions on status/not allowed paths.

		specFieldV1 := fieldV1["f:spec"].(map[string]interface{})
		g.Expect(specFieldV1).ToNot(BeEmpty())
		g.Expect(specFieldV1).To(HaveKey("f:controlPlaneEndpoint")) // topology controller should express opinions on spec.controlPlaneEndpoint.
		g.Expect(specFieldV1).ToNot(HaveKey("f:foo"))               // topology controller should not express opinions on ignore paths.
		g.Expect(specFieldV1).ToNot(HaveKey("f:bar"))               // topology controller should not express opinions on fields managed by other controllers.
	})

	t.Run("Topology controller reconcile again with some changes on topology managed fields", func(t *testing.T) {
		g := NewWithT(t)

		// Get the current object (assumes tests to be run in sequence).
		original := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

		// Create a patch helper for a modified object with some changes to what previously applied by th topology manager.
		modified := obj.DeepCopy()
		g.Expect(unstructured.SetNestedField(modified.Object, "changed", "spec", "controlPlaneEndpoint", "host")).To(Succeed())

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeTrue())
		g.Expect(p0.HasSpecChanges()).To(BeTrue())

		// Create the object using server side apply
		g.Expect(p0.Patch(ctx)).To(Succeed())

		// Check the object and verify the change is applied as well as the fields set by the other controller are still preserved.
		got := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(got), got)).To(Succeed())

		// Check if resourceVersion did change
		g.Expect(got.GetResourceVersion()).ToNot(Equal(original.GetResourceVersion()))

		v0, _, _ := unstructured.NestedString(got.Object, "spec", "controlPlaneEndpoint", "host")
		g.Expect(v0).To(Equal("changed"))
		v1, _, _ := unstructured.NestedString(got.Object, "spec", "foo")
		g.Expect(v1).To(Equal("changed"))
		v2, _, _ := unstructured.NestedString(got.Object, "spec", "bar")
		g.Expect(v2).To(Equal("changed"))
		v3, _, _ := unstructured.NestedString(got.Object, "status", "foo")
		g.Expect(v3).To(Equal("changed"))
		v4, _, _ := unstructured.NestedBool(got.Object, "status", "ready")
		g.Expect(v4).To(BeTrue())
	})
	t.Run("Topology controller reconcile again with an opinion on a field managed by another controller (co-ownership)", func(t *testing.T) {
		g := NewWithT(t)

		// Get the current object (assumes tests to be run in sequence).
		original := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

		// Create a patch helper for a modified object with some changes to what previously applied by th topology manager.
		modified := obj.DeepCopy()
		g.Expect(unstructured.SetNestedField(modified.Object, "changed", "spec", "controlPlaneEndpoint", "host")).To(Succeed())
		g.Expect(unstructured.SetNestedField(modified.Object, "changed", "spec", "bar")).To(Succeed())

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeTrue())
		g.Expect(p0.HasSpecChanges()).To(BeFalse())

		// Create the object using server side apply
		g.Expect(p0.Patch(ctx)).To(Succeed())

		// Check the object and verify the change is applied as well as managed field updated accordingly.
		got := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(got), got)).To(Succeed())

		// Check if resourceVersion did change
		g.Expect(got.GetResourceVersion()).ToNot(Equal(original.GetResourceVersion()))

		v2, _, _ := unstructured.NestedString(got.Object, "spec", "bar")
		g.Expect(v2).To(Equal("changed"))

		fieldV1 := getTopologyManagedFields(got)
		g.Expect(fieldV1).ToNot(BeEmpty())
		g.Expect(fieldV1).To(HaveKey("f:spec")) // topology controller should express opinions on spec.

		specFieldV1 := fieldV1["f:spec"].(map[string]interface{})
		g.Expect(specFieldV1).ToNot(BeEmpty())
		g.Expect(specFieldV1).To(HaveKey("f:controlPlaneEndpoint")) // topology controller should express opinions on spec.controlPlaneEndpoint.
		g.Expect(specFieldV1).ToNot(HaveKey("f:foo"))               // topology controller should not express opinions on ignore paths.
		g.Expect(specFieldV1).To(HaveKey("f:bar"))                  // topology controller now has an opinion on a field previously managed by other controllers (force ownership).
	})
	t.Run("Topology controller reconcile again with an opinion on a field managed by another controller (force ownership)", func(t *testing.T) {
		g := NewWithT(t)

		// Get the current object (assumes tests to be run in sequence).
		original := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

		// Create a patch helper for a modified object with some changes to what previously applied by th topology manager.
		modified := obj.DeepCopy()
		g.Expect(unstructured.SetNestedField(modified.Object, "changed", "spec", "controlPlaneEndpoint", "host")).To(Succeed())
		g.Expect(unstructured.SetNestedField(modified.Object, "changed-by-topology-controller", "spec", "bar")).To(Succeed())

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeTrue())
		g.Expect(p0.HasSpecChanges()).To(BeTrue())

		// Create the object using server side apply
		g.Expect(p0.Patch(ctx)).To(Succeed())

		// Check the object and verify the change is applied as well as managed field updated accordingly.
		got := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(got), got)).To(Succeed())

		// Check if resourceVersion did change
		g.Expect(got.GetResourceVersion()).ToNot(Equal(original.GetResourceVersion()))

		v2, _, _ := unstructured.NestedString(got.Object, "spec", "bar")
		g.Expect(v2).To(Equal("changed-by-topology-controller"))

		fieldV1 := getTopologyManagedFields(got)
		g.Expect(fieldV1).ToNot(BeEmpty())
		g.Expect(fieldV1).To(HaveKey("f:spec")) // topology controller should express opinions on spec.

		specFieldV1 := fieldV1["f:spec"].(map[string]interface{})
		g.Expect(specFieldV1).ToNot(BeEmpty())
		g.Expect(specFieldV1).To(HaveKey("f:controlPlaneEndpoint")) // topology controller should express opinions on spec.controlPlaneEndpoint.
		g.Expect(specFieldV1).ToNot(HaveKey("f:foo"))               // topology controller should not express opinions on ignore paths.
		g.Expect(specFieldV1).To(HaveKey("f:bar"))                  // topology controller now has an opinion on a field previously managed by other controllers (force ownership).
	})
	t.Run("No-op on unstructured object having empty map[string]interface in spec", func(t *testing.T) {
		g := NewWithT(t)

		obj2 := builder.TestInfrastructureCluster(ns.Name, "obj2").
			WithSpecFields(map[string]interface{}{
				"spec.fooMap":  map[string]interface{}{},
				"spec.fooList": []interface{}{},
			}).
			Build()

		// create new object having an empty map[string]interface in spec and a copy of it for further testing
		original := obj2.DeepCopy()
		modified := obj2.DeepCopy()

		// Create the object using server side apply
		g.Expect(env.PatchAndWait(ctx, original, client.FieldOwner(TopologyManagerName))).To(Succeed())
		// Get created object to have managed fields
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

		// Create a patch helper for a modified object with which has no changes.
		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient())
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeFalse())
		g.Expect(p0.HasSpecChanges()).To(BeFalse())
	})
	t.Run("Error on object which has another uid due to immutability", func(t *testing.T) {
		g := NewWithT(t)

		// Get the current object (assumes tests to be run in sequence).
		original := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

		// Create a patch helper for a modified object with some changes to what previously applied by th topology manager.
		modified := obj.DeepCopy()
		g.Expect(unstructured.SetNestedField(modified.Object, "changed", "spec", "controlPlaneEndpoint", "host")).To(Succeed())
		g.Expect(unstructured.SetNestedField(modified.Object, "changed-by-topology-controller", "spec", "bar")).To(Succeed())

		// Set an other uid to original
		original.SetUID("a-wrong-one")
		modified.SetUID("")

		// Create a patch helper which should fail because original's real UID changed.
		_, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient())
		g.Expect(err).To(HaveOccurred())
	})
	t.Run("Error on object which does not exist (anymore) but was expected to get updated", func(t *testing.T) {
		original := builder.TestInfrastructureCluster(ns.Name, "obj3").WithSpecFields(map[string]interface{}{
			"spec.controlPlaneEndpoint.host": "1.2.3.4",
			"spec.controlPlaneEndpoint.port": int64(1234),
			"spec.foo":                       "", // this field is then explicitly ignored by the patch helper
		}).Build()

		modified := original.DeepCopy()
		g.Expect(unstructured.SetNestedField(modified.Object, "changed", "spec", "controlPlaneEndpoint", "host")).To(Succeed())

		// Set a not existing uid to the not existing original object
		original.SetUID("does-not-exist")

		// Create a patch helper which should fail because original does not exist.
		_, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient())
		g.Expect(err).To(HaveOccurred())
	})
}

// getTopologyManagedFields returns metadata.managedFields entry tracking
// server side apply operations for the topology controller.
func getTopologyManagedFields(original client.Object) map[string]interface{} {
	r := map[string]interface{}{}

	for _, m := range original.GetManagedFields() {
		if m.Operation == metav1.ManagedFieldsOperationApply &&
			m.Manager == TopologyManagerName &&
			m.APIVersion == original.GetObjectKind().GroupVersionKind().GroupVersion().String() {
			// NOTE: API server ensures this is a valid json.
			err := json.Unmarshal(m.FieldsV1.Raw, &r)
			if err != nil {
				continue
			}
			break
		}
	}
	return r
}
