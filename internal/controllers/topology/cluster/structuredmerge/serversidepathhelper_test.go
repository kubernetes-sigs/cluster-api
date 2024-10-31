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
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/test/builder"
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

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), ssa.NewCache())
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeTrue())
		g.Expect(p0.HasSpecChanges()).To(BeTrue())
		g.Expect(p0.Changes()).To(BeNil()) // changes are expected to be nil on create.
	})
	t.Run("Server side apply detect changes on object creation (typed)", func(t *testing.T) {
		g := NewWithT(t)

		var original *clusterv1.MachineDeployment
		modified := obj.DeepCopy()

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), ssa.NewCache())
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeTrue())
		g.Expect(p0.HasSpecChanges()).To(BeTrue())
		g.Expect(p0.Changes()).To(BeNil()) // changes are expected to be nil on create.
	})
	t.Run("When creating an object using server side apply, it should track managed fields for the topology controller", func(t *testing.T) {
		g := NewWithT(t)

		// Create a patch helper with original == nil and modified == obj, ensure this is detected as operation that triggers changes.
		p0, err := NewServerSidePatchHelper(ctx, nil, obj.DeepCopy(), env.GetClient(), ssa.NewCache(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeTrue())
		g.Expect(p0.HasSpecChanges()).To(BeTrue())
		g.Expect(p0.Changes()).To(BeNil()) // changes are expected to be nil on create.

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
		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), ssa.NewCache(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeFalse())
		g.Expect(p0.HasSpecChanges()).To(BeFalse())
		g.Expect(p0.Changes()).To(BeNil())
	})

	t.Run("Server side apply patch helper discard changes in not allowed fields, e.g. status", func(t *testing.T) {
		g := NewWithT(t)

		// Get the current object (assumes tests to be run in sequence).
		original := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

		// Create a patch helper for a modified object with changes only in status.
		modified := obj.DeepCopy()
		g.Expect(unstructured.SetNestedField(modified.Object, "changed", "status", "foo")).To(Succeed())

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), ssa.NewCache(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeFalse())
		g.Expect(p0.HasSpecChanges()).To(BeFalse())
		g.Expect(p0.Changes()).To(BeNil())
	})

	t.Run("Server side apply patch helper detect changes", func(t *testing.T) {
		g := NewWithT(t)

		// Get the current object (assumes tests to be run in sequence).
		original := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

		// Create a patch helper for a modified object with changes in spec.
		modified := obj.DeepCopy()
		g.Expect(unstructured.SetNestedField(modified.Object, "changed", "spec", "bar")).To(Succeed())

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), ssa.NewCache(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeTrue())
		g.Expect(p0.HasSpecChanges()).To(BeTrue())
		g.Expect(p0.Changes()).To(Equal([]byte(`{"spec":{"bar":"changed"}}`)))
	})

	t.Run("Server side apply patch helper detect changes impacting only metadata.labels", func(t *testing.T) {
		g := NewWithT(t)

		// Get the current object (assumes tests to be run in sequence).
		original := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

		// Create a patch helper for a modified object with changes only in metadata.
		modified := obj.DeepCopy()
		modified.SetLabels(map[string]string{"foo": "changed"})

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), ssa.NewCache(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeTrue())
		g.Expect(p0.HasSpecChanges()).To(BeFalse())
		g.Expect(p0.Changes()).To(Equal([]byte(`{"metadata":{"labels":{"foo":"changed"}}}`)))
	})

	t.Run("Server side apply patch helper detect changes impacting only metadata.annotations", func(t *testing.T) {
		g := NewWithT(t)

		// Get the current object (assumes tests to be run in sequence).
		original := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

		// Create a patch helper for a modified object with changes only in metadata.
		modified := obj.DeepCopy()
		modified.SetAnnotations(map[string]string{"foo": "changed"})

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), ssa.NewCache(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeTrue())
		g.Expect(p0.HasSpecChanges()).To(BeFalse())
		g.Expect(p0.Changes()).To(Equal([]byte(`{"metadata":{"annotations":{"foo":"changed"}}}`)))
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

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), ssa.NewCache(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeTrue())
		g.Expect(p0.HasSpecChanges()).To(BeFalse())
		g.Expect(p0.Changes()).To(Equal([]byte(`{"metadata":{"ownerReferences":[{"apiVersion":"foo/v1alpha1","kind":"foo","name":"foo","uid":"foo"}]}}`)))
	})

	t.Run("Server side apply patch helper discard changes in ignore paths", func(t *testing.T) {
		g := NewWithT(t)

		// Get the current object (assumes tests to be run in sequence).
		original := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

		// Create a patch helper for a modified object with changes only in an ignoredField.
		modified := obj.DeepCopy()
		g.Expect(unstructured.SetNestedField(modified.Object, "changed", "spec", "foo")).To(Succeed())

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), ssa.NewCache(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeFalse())
		g.Expect(p0.HasSpecChanges()).To(BeFalse())
		g.Expect(p0.Changes()).To(BeNil())
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
		p0, err := NewServerSidePatchHelper(ctx, original, original, env.GetClient(), ssa.NewCache(), IgnorePaths{{"spec", "foo"}, {"spec", "bar"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeFalse())
		g.Expect(p0.HasSpecChanges()).To(BeFalse())
		g.Expect(p0.Changes()).To(BeNil())
	})

	t.Run("Topology controller reconcile again with no changes on topology managed fields", func(t *testing.T) {
		g := NewWithT(t)

		// Get the current object (assumes tests to be run in sequence).
		original := obj.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

		// Create a patch helper for a modified object with no changes to what previously applied by th topology manager.
		modified := obj.DeepCopy()

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), ssa.NewCache(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeFalse())
		g.Expect(p0.HasSpecChanges()).To(BeFalse())
		g.Expect(p0.Changes()).To(BeNil())

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

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), ssa.NewCache(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeTrue())
		g.Expect(p0.HasSpecChanges()).To(BeTrue())
		g.Expect(p0.Changes()).To(Equal([]byte(`{"spec":{"controlPlaneEndpoint":{"host":"changed"}}}`)))

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

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), ssa.NewCache(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeTrue())
		g.Expect(p0.HasSpecChanges()).To(BeFalse())
		g.Expect(p0.Changes()).To(BeEmpty()) // Note: metadata.managedFields have been removed from the diff to reduce log verbosity.

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

		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), ssa.NewCache(), IgnorePaths{{"spec", "foo"}})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeTrue())
		g.Expect(p0.HasSpecChanges()).To(BeTrue())
		g.Expect(p0.Changes()).To(Equal([]byte(`{"spec":{"bar":"changed-by-topology-controller"}}`)))

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
		p0, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), ssa.NewCache())
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(p0.HasChanges()).To(BeFalse())
		g.Expect(p0.HasSpecChanges()).To(BeFalse())
		g.Expect(p0.Changes()).To(BeNil())
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
		_, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), ssa.NewCache())
		g.Expect(err).To(HaveOccurred())
	})
	t.Run("Error on object which does not exist (anymore) but was expected to get updated", func(*testing.T) {
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
		_, err := NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), ssa.NewCache())
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

// NOTE: This test ensures that ServerSideApply works as expected when new defaulting logic is introduced by a Cluster API update.
func TestServerSideApplyWithDefaulting(t *testing.T) {
	g := NewWithT(t)

	// Create a namespace for running the test
	ns, err := env.CreateNamespace(ctx, "ssa-defaulting")
	g.Expect(err).ToNot(HaveOccurred())

	// Setup webhook with the manager.
	// Note: The webhooks is not active yet, as the MutatingWebhookConfiguration will be deployed later.
	defaulter, mutatingWebhookConfiguration, err := setupWebhookWithManager(ns)
	g.Expect(err).ToNot(HaveOccurred())

	// Calculate KubeadmConfigTemplate.
	kct := &bootstrapv1.KubeadmConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kct",
			Namespace: ns.Name,
		},
		Spec: bootstrapv1.KubeadmConfigTemplateSpec{
			Template: bootstrapv1.KubeadmConfigTemplateResource{
				Spec: bootstrapv1.KubeadmConfigSpec{
					JoinConfiguration: &bootstrapv1.JoinConfiguration{
						NodeRegistration: bootstrapv1.NodeRegistrationOptions{
							KubeletExtraArgs: map[string]string{
								"eviction-hard": "nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%",
							},
						},
					},
				},
			},
		},
	}

	// The test does the following.
	// 1. Create KubeadmConfigTemplate
	// 2. Activate the new defaulting logic via the webhook
	//   * This simulates the deployment of a new Cluster API version with new defaulting
	// 3. Simulate defaulting on original and/or modified
	//   * defaultOriginal will add a label to the KubeadmConfigTemplate which will trigger defaulting
	//     * original is the KubeadmConfigTemplate referenced in a MachineDeployment of the Cluster topology
	//   * defaultModified will simulate that defaulting was run on the KubeadmConfigTemplate referenced in the ClusterClass
	//     * modified is the desired state calculated based on the KubeadmConfigTemplate referenced in the ClusterClass
	//   * We are testing through all permutations as we don't want to assume on which objects defaulting was run.
	// 4. Check patch helper results

	// We have the following test cases:
	// | original   | modified    | expect behavior                                           |
	// |            |             | no-op                                                     |
	// | defaulted  |             | no-op                                                     |
	// |            | defaulted   | no spec changes, only take ownership of defaulted fields  |
	// | defaulted  | defaulted   | no spec changes, only take ownership of defaulted fields  |
	tests := []struct {
		name                 string
		defaultOriginal      bool
		defaultModified      bool
		expectChanges        bool
		expectSpecChanges    bool
		expectFieldOwnership bool
	}{
		{
			name:            "no-op if neither is defaulted",
			defaultOriginal: false,
			defaultModified: false,
			// Dry run results:
			// * original: field will be defaulted by the webhook, capi-topology doesn't get ownership.
			// * modified: field will be defaulted by the webhook, capi-topology doesn't get ownership.
			expectChanges:        false,
			expectSpecChanges:    false,
			expectFieldOwnership: false,
		},
		{
			name:            "no-op if original is defaulted",
			defaultOriginal: true,
			defaultModified: false,
			// Dry run results:
			// * original: no defaulting in dry run, as field has already been defaulted before, capi-topology doesn't get ownership.
			// * modified: field will be defaulted by the webhook, capi-topology doesn't get ownership.
			expectChanges:        false,
			expectSpecChanges:    false,
			expectFieldOwnership: false,
		},
		{
			name:            "no spec changes, only take ownership of defaulted fields if modified is defaulted",
			defaultOriginal: false,
			defaultModified: true,
			// Dry run results:
			// * original: field will be defaulted by the webhook, capi-topology doesn't get ownership.
			// * original: no defaulting in dry run, as field has already been defaulted before, capi-topology does get ownership as we explicitly set the field.
			// => capi-topology takes ownership during Patch
			expectChanges:        true,
			expectSpecChanges:    false,
			expectFieldOwnership: true,
		},
		{
			name:            "no spec changes, only take ownership of defaulted fields if both are defaulted",
			defaultOriginal: true,
			defaultModified: true,
			// Dry run results:
			// * original: no defaulting in dry run, as field has already been defaulted before, capi-topology doesn't get ownership.
			// * original: no defaulting in dry run, as field has already been defaulted before, capi-topology does get ownership as we explicitly set the field.
			// => capi-topology takes ownership during Patch
			expectChanges:        true,
			expectSpecChanges:    false,
			expectFieldOwnership: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			// Note: This is necessary because otherwise we could not create the webhook config
			// in multiple test runs, because after the first test run it has a resourceVersion set.
			mutatingWebhookConfiguration := mutatingWebhookConfiguration.DeepCopy()

			// Create a cache to cache SSA requests.
			ssaCache := ssa.NewCache()

			// Create the initial KubeadmConfigTemplate (with the old defaulting logic).
			p0, err := NewServerSidePatchHelper(ctx, nil, kct.DeepCopy(), env.GetClient(), ssaCache)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(p0.HasChanges()).To(BeTrue())
			g.Expect(p0.HasSpecChanges()).To(BeTrue())
			g.Expect(p0.Patch(ctx)).To(Succeed())
			defer func() {
				g.Expect(env.CleanupAndWait(ctx, kct.DeepCopy())).To(Succeed())
			}()

			// Wait until the initial KubeadmConfigTemplate is visible in the local cache. Otherwise the test fails below.
			g.Eventually(ctx, func(g Gomega) {
				g.Expect(env.Get(ctx, client.ObjectKeyFromObject(kct), &bootstrapv1.KubeadmConfigTemplate{})).To(Succeed())
			}, 5*time.Second).Should(Succeed())

			// Enable the new defaulting logic (i.e. simulate the Cluster API update).
			// The webhook will default the users field to `[{Name: "default-user"}]`.
			g.Expect(env.Create(ctx, mutatingWebhookConfiguration)).To(Succeed())
			defer func() {
				g.Expect(env.CleanupAndWait(ctx, mutatingWebhookConfiguration)).To(Succeed())
			}()

			// Test if web hook is working
			// NOTE: we are doing this no matter of defaultOriginal to make sure that the web hook is up and running
			// before calling NewServerSidePatchHelper down below.
			g.Eventually(ctx, func(g Gomega) {
				kct2 := kct.DeepCopy()
				kct2.Name = fmt.Sprintf("%s-2", kct.Name)
				g.Expect(env.Create(ctx, kct2, client.DryRunAll)).To(Succeed())
				g.Expect(kct2.Spec.Template.Spec.Users).To(BeComparableTo([]bootstrapv1.User{{Name: "default-user"}}))
			}, 5*time.Second).Should(Succeed())

			// Run defaulting on the KubeadmConfigTemplate (triggered by an "external controller")
			if tt.defaultOriginal {
				patchKCT := &bootstrapv1.KubeadmConfigTemplate{}
				g.Expect(env.Get(ctx, client.ObjectKeyFromObject(kct), patchKCT)).To(Succeed())

				if patchKCT.Labels == nil {
					patchKCT.Labels = map[string]string{}
				}
				patchKCT.Labels["trigger"] = "update"

				g.Expect(env.Patch(ctx, patchKCT, client.MergeFrom(kct))).To(Succeed())

				// Wait for cache to be updated with the defaulted object
				g.Eventually(ctx, func(g Gomega) {
					g.Expect(env.Get(ctx, client.ObjectKeyFromObject(kct), patchKCT)).To(Succeed())
					g.Expect(patchKCT.Spec.Template.Spec.Users).To(BeComparableTo([]bootstrapv1.User{{Name: "default-user"}}))
				}, 5*time.Second).Should(Succeed())
			}
			// Get original for the update.
			original := kct.DeepCopy()
			g.Expect(env.Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

			// Calculate modified for the update.
			modified := kct.DeepCopy()
			// Run defaulting on modified
			// Note: We just default the modified / desired locally as we are not simulating
			// an entire ClusterClass. Defaulting on the template of the ClusterClass would
			// lead to the modified object having the defaults.
			if tt.defaultModified {
				defaultKubeadmConfigTemplate(modified)
			}

			// Apply modified.
			p0, err = NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), ssaCache)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(p0.HasChanges()).To(Equal(tt.expectChanges), fmt.Sprintf("changes: %s", string(p0.Changes())))
			g.Expect(p0.HasSpecChanges()).To(Equal(tt.expectSpecChanges))
			g.Expect(p0.Patch(ctx)).To(Succeed())

			// Verify field ownership
			// Note: It might take a bit for the cache to be up-to-date.
			g.Eventually(func(g Gomega) {
				got := original.DeepCopy()
				g.Expect(env.Get(ctx, client.ObjectKeyFromObject(got), got)).To(Succeed())

				// topology controller should express opinions on spec.template.spec.
				fieldV1 := getTopologyManagedFields(got)
				g.Expect(fieldV1).ToNot(BeEmpty())
				g.Expect(fieldV1).To(HaveKey("f:spec"))
				specFieldV1 := fieldV1["f:spec"].(map[string]interface{})
				g.Expect(specFieldV1).ToNot(BeEmpty())
				g.Expect(specFieldV1).To(HaveKey("f:template"))
				specTemplateFieldV1 := specFieldV1["f:template"].(map[string]interface{})
				g.Expect(specTemplateFieldV1).ToNot(BeEmpty())
				g.Expect(specTemplateFieldV1).To(HaveKey("f:spec"))

				specTemplateSpecFieldV1 := specTemplateFieldV1["f:spec"].(map[string]interface{})
				if tt.expectFieldOwnership {
					// topology controller should express opinions on spec.template.spec.users.
					g.Expect(specTemplateSpecFieldV1).To(HaveKey("f:users"))
				} else {
					// topology controller should not express opinions on spec.template.spec.users.
					g.Expect(specTemplateSpecFieldV1).ToNot(HaveKey("f:users"))
				}
			}, 2*time.Second).Should(Succeed())

			if p0.HasChanges() {
				// If there were changes the request should not be cached.
				// Which means on the next call we should not hit the cache and thus
				// send a request to the server.
				// We verify this by checking the webhook call counter.

				// Get original.
				original = kct.DeepCopy()
				g.Expect(env.Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

				countBefore := defaulter.Counter

				// Apply modified again.
				p0, err = NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), ssaCache)
				g.Expect(err).ToNot(HaveOccurred())

				// Expect no changes.
				g.Expect(p0.HasChanges()).To(BeFalse())
				g.Expect(p0.HasSpecChanges()).To(BeFalse())
				g.Expect(p0.Patch(ctx)).To(Succeed())

				// Expect webhook to be called.
				g.Expect(defaulter.Counter).To(Equal(countBefore+2),
					"request should not have been cached and thus we expect the webhook to be called twice (once for original and once for modified)")

				// Note: Now the request is also cached, which we verify below.
			}

			// If there were no changes the request is now cached.
			// Which means on the next call we should only hit the cache and thus
			// don't send a request to the server.
			// We verify this by checking the webhook call counter.

			// Get original.
			original = kct.DeepCopy()
			g.Expect(env.Get(ctx, client.ObjectKeyFromObject(original), original)).To(Succeed())

			countBefore := defaulter.Counter

			// Apply modified again.
			p0, err = NewServerSidePatchHelper(ctx, original, modified, env.GetClient(), ssaCache)
			g.Expect(err).ToNot(HaveOccurred())

			// Expect no changes.
			g.Expect(p0.HasChanges()).To(BeFalse())
			g.Expect(p0.HasSpecChanges()).To(BeFalse())
			g.Expect(p0.Patch(ctx)).To(Succeed())

			// Expect webhook to not be called.
			g.Expect(defaulter.Counter).To(Equal(countBefore),
				"request should have been cached and thus the webhook not called")
		})
	}
}

// setupWebhookWithManager configures the envtest manager / webhook server to serve the webhook.
// It also calculates and returns the corresponding MutatingWebhookConfiguration.
// Note: To activate the webhook, the MutatingWebhookConfiguration has to be deployed.
func setupWebhookWithManager(ns *corev1.Namespace) (*KubeadmConfigTemplateTestDefaulter, *admissionv1.MutatingWebhookConfiguration, error) {
	webhookServer := env.Manager.GetWebhookServer().(*webhook.DefaultServer)

	// Calculate webhook host and path.
	// Note: This is done the same way as in our envtest package.
	webhookPath := fmt.Sprintf("/%s/ssa-defaulting-webhook", ns.Name)
	webhookHost := "127.0.0.1"
	if host := os.Getenv("CAPI_WEBHOOK_HOSTNAME"); host != "" {
		webhookHost = host
	}

	// Serve KubeadmConfigTemplateTestDefaulter on the webhook server.
	// Note: This should only ever be called once with the same path, otherwise we get a panic.
	defaulter := &KubeadmConfigTemplateTestDefaulter{}
	webhookServer.Register(webhookPath,
		admission.WithCustomDefaulter(env.Manager.GetScheme(), &bootstrapv1.KubeadmConfigTemplate{}, defaulter))

	// Calculate the MutatingWebhookConfiguration
	caBundle, err := os.ReadFile(filepath.Join(webhookServer.Options.CertDir, webhookServer.Options.CertName))
	if err != nil {
		return nil, nil, err
	}

	sideEffectNone := admissionv1.SideEffectClassNone
	webhookConfig := &admissionv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: ns.Name + "-webhook-config",
		},
		Webhooks: []admissionv1.MutatingWebhook{
			{
				Name: ns.Name + ".kubeadmconfigtemplate.bootstrap.cluster.x-k8s.io",
				ClientConfig: admissionv1.WebhookClientConfig{
					URL:      ptr.To(fmt.Sprintf("https://%s%s", net.JoinHostPort(webhookHost, strconv.Itoa(webhookServer.Options.Port)), webhookPath)),
					CABundle: caBundle,
				},
				Rules: []admissionv1.RuleWithOperations{
					{
						Operations: []admissionv1.OperationType{
							admissionv1.Create,
							admissionv1.Update,
						},
						Rule: admissionv1.Rule{
							APIGroups:   []string{bootstrapv1.GroupVersion.Group},
							APIVersions: []string{bootstrapv1.GroupVersion.Version},
							Resources:   []string{"kubeadmconfigtemplates"},
						},
					},
				},
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						corev1.LabelMetadataName: ns.Name,
					},
				},
				AdmissionReviewVersions: []string{"v1"},
				SideEffects:             &sideEffectNone,
			},
		},
	}
	return defaulter, webhookConfig, nil
}

var _ webhook.CustomDefaulter = &KubeadmConfigTemplateTestDefaulter{}

type KubeadmConfigTemplateTestDefaulter struct {
	Counter int
}

func (d *KubeadmConfigTemplateTestDefaulter) Default(_ context.Context, obj runtime.Object) error {
	kct, ok := obj.(*bootstrapv1.KubeadmConfigTemplate)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Cluster but got a %T", obj))
	}

	d.Counter++

	defaultKubeadmConfigTemplate(kct)
	return nil
}

func defaultKubeadmConfigTemplate(kct *bootstrapv1.KubeadmConfigTemplate) {
	if len(kct.Spec.Template.Spec.Users) == 0 {
		kct.Spec.Template.Spec.Users = []bootstrapv1.User{
			{
				Name: "default-user",
			},
		}
	}
}
