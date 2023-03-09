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
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/test/builder"
)

func TestPatch(t *testing.T) {
	g := NewWithT(t)

	// Create a namespace for running the test
	ns, err := env.CreateNamespace(ctx, "ssa")
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("Test patch with unstructured", func(t *testing.T) {
		// Build the test object to work with.
		initialObject := builder.TestInfrastructureCluster(ns.Name, "obj1").WithSpecFields(map[string]interface{}{
			"spec.controlPlaneEndpoint.host": "1.2.3.4",
			"spec.controlPlaneEndpoint.port": int64(1234),
			"spec.foo":                       "bar",
		}).Build()

		fieldManager := "test-manager"
		ssaCache := NewCache()

		// 1. Create the object
		createObject := initialObject.DeepCopy()
		g.Expect(Patch(ctx, env.GetClient(), fieldManager, createObject)).To(Succeed())

		// 2. Update the object and verify that the request was not cached as the object was changed.
		// Get the original object.
		originalObject := initialObject.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(originalObject), originalObject))
		// Modify the object
		modifiedObject := initialObject.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedObject.Object, "baz", "spec", "foo")).To(Succeed())
		// Compute request identifier, so we can later verify that the update call was not cached.
		modifiedUnstructured, err := prepareModified(env.Scheme(), modifiedObject)
		g.Expect(err).ToNot(HaveOccurred())
		requestIdentifier, err := ComputeRequestIdentifier(env.GetScheme(), originalObject, modifiedUnstructured)
		g.Expect(err).ToNot(HaveOccurred())
		// Update the object
		g.Expect(Patch(ctx, env.GetClient(), fieldManager, modifiedObject, WithCachingProxy{Cache: ssaCache, Original: originalObject})).To(Succeed())
		// Verify that request was not cached (as it changed the object)
		g.Expect(ssaCache.Has(requestIdentifier)).To(BeFalse())

		// 3. Repeat the same update and verify that the request was cached as the object was not changed.
		// Get the original object.
		originalObject = initialObject.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(originalObject), originalObject))
		// Modify the object
		modifiedObject = initialObject.DeepCopy()
		g.Expect(unstructured.SetNestedField(modifiedObject.Object, "baz", "spec", "foo")).To(Succeed())
		// Compute request identifier, so we can later verify that the update call was cached.
		modifiedUnstructured, err = prepareModified(env.Scheme(), modifiedObject)
		g.Expect(err).ToNot(HaveOccurred())
		requestIdentifier, err = ComputeRequestIdentifier(env.GetScheme(), originalObject, modifiedUnstructured)
		g.Expect(err).ToNot(HaveOccurred())
		// Update the object
		g.Expect(Patch(ctx, env.GetClient(), fieldManager, modifiedObject, WithCachingProxy{Cache: ssaCache, Original: originalObject})).To(Succeed())
		// Verify that request was cached (as it did not change the object)
		g.Expect(ssaCache.Has(requestIdentifier)).To(BeTrue())
	})

	t.Run("Test patch with Machine", func(t *testing.T) {
		// Build the test object to work with.
		initialObject := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{
				APIVersion: clusterv1.GroupVersion.String(),
				Kind:       "Machine",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-1",
				Namespace: ns.Name,
				Labels: map[string]string{
					"label": "labelValue",
				},
				Annotations: map[string]string{
					"annotation": "annotationValue",
				},
			},
			Spec: clusterv1.MachineSpec{
				ClusterName:      "cluster-1",
				Version:          pointer.String("v1.25.0"),
				NodeDrainTimeout: &metav1.Duration{Duration: 10 * time.Second},
				Bootstrap: clusterv1.Bootstrap{
					DataSecretName: pointer.String("data-secret"),
				},
			},
		}
		fieldManager := "test-manager"
		ssaCache := NewCache()

		// 1. Create the object
		createObject := initialObject.DeepCopy()
		g.Expect(Patch(ctx, env.GetClient(), fieldManager, createObject)).To(Succeed())
		// Note: We have to patch the status here to explicitly set these two status fields.
		// If we don't do it the Machine defaulting webhook will try to set the two fields to false.
		// For an unknown reason this will happen with the 2nd update call (3.) below and not before.
		// This means that this call would unexpectedly not cache the object because the resourceVersion
		// is changed because the fields are set.
		// It's unclear why those status fields are not already set during create (1.) or the first update (2.)
		// (the webhook is returning patches for the two fields in those requests as well).
		// To further investigate this behavior it would be necessary to debug the kube-apiserver.
		// Fortunately, in reality this is not an issue as the fields will be set sooner or later and then
		// the requests are cached.
		createObjectWithStatus := createObject.DeepCopy()
		createObjectWithStatus.Status.BootstrapReady = false
		createObjectWithStatus.Status.InfrastructureReady = false
		g.Expect(env.Status().Patch(ctx, createObjectWithStatus, client.MergeFrom(createObject)))

		// 2. Update the object and verify that the request was not cached as the object was changed.
		// Get the original object.
		originalObject := initialObject.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(originalObject), originalObject))
		// Modify the object
		modifiedObject := initialObject.DeepCopy()
		modifiedObject.Spec.NodeDrainTimeout = &metav1.Duration{Duration: 5 * time.Second}
		// Compute request identifier, so we can later verify that the update call was not cached.
		modifiedUnstructured, err := prepareModified(env.Scheme(), modifiedObject)
		g.Expect(err).ToNot(HaveOccurred())
		requestIdentifier, err := ComputeRequestIdentifier(env.GetScheme(), originalObject, modifiedUnstructured)
		g.Expect(err).ToNot(HaveOccurred())
		// Update the object
		g.Expect(Patch(ctx, env.GetClient(), fieldManager, modifiedObject, WithCachingProxy{Cache: ssaCache, Original: originalObject})).To(Succeed())
		// Verify that request was not cached (as it changed the object)
		g.Expect(ssaCache.Has(requestIdentifier)).To(BeFalse())

		// 3. Repeat the same update and verify that the request was cached as the object was not changed.
		// Get the original object.
		originalObject = initialObject.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(originalObject), originalObject))
		// Modify the object
		modifiedObject = initialObject.DeepCopy()
		modifiedObject.Spec.NodeDrainTimeout = &metav1.Duration{Duration: 5 * time.Second}
		// Compute request identifier, so we can later verify that the update call was cached.
		modifiedUnstructured, err = prepareModified(env.Scheme(), modifiedObject)
		g.Expect(err).ToNot(HaveOccurred())
		requestIdentifier, err = ComputeRequestIdentifier(env.GetScheme(), originalObject, modifiedUnstructured)
		g.Expect(err).ToNot(HaveOccurred())
		// Update the object
		g.Expect(Patch(ctx, env.GetClient(), fieldManager, modifiedObject, WithCachingProxy{Cache: ssaCache, Original: originalObject})).To(Succeed())
		// Verify that request was cached (as it did not change the object)
		g.Expect(ssaCache.Has(requestIdentifier)).To(BeTrue())
	})
}
