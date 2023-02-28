/*
Copyright 2023 The Kubernetes Authors.

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

package ssa

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/internal/test/builder"
)

func TestPatch(t *testing.T) {
	g := NewWithT(t)

	// Create a namespace for running the test
	ns, err := env.CreateNamespace(ctx, "ssa")
	g.Expect(err).ToNot(HaveOccurred())

	// Build the test object to work with.
	initialObject := builder.TestInfrastructureCluster(ns.Name, "obj1").WithSpecFields(map[string]interface{}{
		"spec.controlPlaneEndpoint.host": "1.2.3.4",
		"spec.controlPlaneEndpoint.port": int64(1234),
		"spec.foo":                       "bar",
	}).Build()

	fieldManager := "test-manager"
	ssaCache := NewCache(env.GetScheme())

	// Create the object
	createObject := initialObject.DeepCopy()
	_, err = Patch(ctx, env.GetClient(), fieldManager, createObject)
	g.Expect(err).ToNot(HaveOccurred())

	// Update the object and verify that the request was not cached as the object was changed.
	// Get the original object.
	originalObject := initialObject.DeepCopy()
	g.Expect(env.GetClient().Get(ctx, client.ObjectKeyFromObject(originalObject), originalObject))
	// Modify the object
	modifiedObject := initialObject.DeepCopy()
	g.Expect(unstructured.SetNestedField(modifiedObject.Object, "baz", "spec", "foo")).To(Succeed())
	// Compute request identifier, so we can later verify that the update call was not cached.
	requestIdentifier, err := ComputeRequestIdentifier(env.GetScheme(), originalObject, modifiedObject)
	g.Expect(err).ToNot(HaveOccurred())
	// Update the object
	_, err = Patch(ctx, env.GetClient(), fieldManager, modifiedObject, WithCachingProxy{Cache: ssaCache, Original: originalObject})
	g.Expect(err).ToNot(HaveOccurred())
	// Verify that request was not cached (as it changed the object)
	g.Expect(ssaCache.Has(requestIdentifier)).To(BeFalse())

	// Repeat the same update and verify that the request was cached as the object was not changed.
	// Get the original object.
	originalObject = initialObject.DeepCopy()
	g.Expect(env.GetClient().Get(ctx, client.ObjectKeyFromObject(originalObject), originalObject))
	// Modify the object
	modifiedObject = initialObject.DeepCopy()
	g.Expect(unstructured.SetNestedField(modifiedObject.Object, "baz", "spec", "foo")).To(Succeed())
	// Compute request identifier, so we can later verify that the update call was not cached.
	requestIdentifier, err = ComputeRequestIdentifier(env.GetScheme(), originalObject, modifiedObject)
	g.Expect(err).ToNot(HaveOccurred())
	// Update the object
	_, err = Patch(ctx, env.GetClient(), fieldManager, modifiedObject, WithCachingProxy{Cache: ssaCache, Original: originalObject})
	g.Expect(err).ToNot(HaveOccurred())
	// Verify that request was not cached (as it changed the object)
	g.Expect(ssaCache.Has(requestIdentifier)).To(BeTrue())
}
