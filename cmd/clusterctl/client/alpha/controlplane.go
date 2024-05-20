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

package alpha

import (
	"context"
	"fmt"
	"strings"
	"time"

	openapi_v2 "github.com/google/gnostic-models/openapiv2"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

func getUnstructuredControlPlane(ctx context.Context, proxy cluster.Proxy, ref corev1.ObjectReference) (*unstructured.Unstructured, error) {
	c, err := proxy.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	if ref.APIVersion == "" {
		ref.APIVersion = DefaultAPIVersion
	}
	gvk := schema.GroupVersionKind{
		Group:   "controlplane.cluster.x-k8s.io",
		Version: ref.APIVersion,
		Kind:    ref.Kind,
	}

	// Create an unstructured object
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetNamespace(ref.Namespace)
	obj.SetName(ref.Name)

	// Fetch the resource
	err = c.Get(ctx, client.ObjectKey{
		Namespace: ref.Namespace,
		Name:      ref.Name,
	}, obj)

	if err != nil {
		return nil, fmt.Errorf("failed to get unstructured object: %v", err)
	}

	return obj, nil
}

// checkResourceConditions checks for specific conditions on the fetched resource.
func checkControlPlaneRolloutAfter(obj *unstructured.Unstructured) error {
	rolloutAfter, _, err := unstructured.NestedString(obj.Object, "spec", "rolloutAfter")

	if err != nil {
		return errors.Wrapf(err, "error accessing rolloutAfter in spec: %s/%s",
			obj.GetNamespace(), obj.GetName())
	}
	if rolloutAfter == "" {
		return nil
	}
	rolloutTime, err := time.Parse(time.RFC3339, rolloutAfter)
	if err != nil {
		return errors.Wrapf(err, "invalid rolloutAfter format: %s/%s",
			obj.GetNamespace(), obj.GetName())
	}
	if rolloutTime.After(time.Now()) {
		return errors.Errorf("can't update KubeadmControlPlane (remove 'spec.rolloutAfter' first): %v/%v", obj.GetKind(), obj.GetName())
	}

	return nil
}

// setRolloutAfterOnControlPlane sets the rolloutAfter field on a generic resource.
func setRolloutAfterOnControlPlane(ctx context.Context, proxy cluster.Proxy, ref corev1.ObjectReference) error {
	patch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf(`{"spec":{"rolloutAfter":"%v"}}`, time.Now().Format(time.RFC3339))))
	return patchControlPlane(ctx, proxy, ref, patch)
}

// patchControlPlane applies a patch to an unstructured controlplane.
func patchControlPlane(ctx context.Context, proxy cluster.Proxy, ref corev1.ObjectReference, patch client.Patch) error {
	c, err := proxy.NewClient(ctx)
	if err != nil {
		return err
	}
	if ref.APIVersion == "" {
		ref.APIVersion = DefaultAPIVersion
	}
	gvk := schema.GroupVersionKind{
		Group:   "controlplane.cluster.x-k8s.io",
		Version: ref.APIVersion,
		Kind:    ref.Kind,
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	objKey := client.ObjectKey{
		Namespace: ref.Namespace,
		Name:      ref.Name,
	}

	// Get the current state of the resource
	if err := c.Get(ctx, objKey, obj); err != nil {
		return errors.Wrapf(err, "failed to get ControlPlane %s/%s",
			obj.GetKind(), obj.GetName())
	}

	// Apply the patch
	if err := c.Patch(ctx, obj, patch); err != nil {
		return errors.Wrapf(err, "failed while patching ControlPlane %s/%s",
			obj.GetKind(), obj.GetName())
	}

	return nil
}

func resourceHasRolloutAfter(proxy cluster.Proxy, ref corev1.ObjectReference) (bool, error) {
	if ref.APIVersion == "" {
		ref.APIVersion = DefaultAPIVersion
	}

	config, err := proxy.GetConfig()
	if err != nil {
		return false, err
	}

	if config == nil {
		return false, nil
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return false, err
	}

	//Fetch the OpenAPI schema
	openAPISchema, err := discoveryClient.OpenAPISchema()
	if err != nil {
		return false, err
	}

	// Iterate over the schema definitions to find the resource
	if openAPISchema == nil {
		return false, fmt.Errorf("openAPI schema is nil")
	}

	for _, definition := range openAPISchema.GetDefinitions().AdditionalProperties {
		// Ensure the definition value is not nil
		if definition == nil || definition.Value == nil {
			continue
		}

		// Check if the definition matches the resource we are looking for
		resourceDefName := fmt.Sprintf("%s.%s.%s", "io.x-k8s.cluster.controlplane", ref.APIVersion, ref.Kind)
		if findSpecPropertyForResource(definition, resourceDefName, "rolloutAfter") {
			return true, nil
		}

	}

	return false, fmt.Errorf("resource definition for %s.%s.%s not found", "io.x-k8s.cluster.controlplane", ref.APIVersion, ref.Kind)
}

func findSpecPropertyForResource(resourceDefinition *openapi_v2.NamedSchema, resourceDefName, field string) bool {
	if !strings.HasSuffix(strings.ToLower(resourceDefinition.Name), resourceDefName) {
		return false
	}

	// Find spec field in crd properties
	properties := resourceDefinition.Value.GetProperties().AdditionalProperties
	if properties == nil {
		return false
	}

	for _, property := range properties {
		if property.GetName() == "spec" {
			// Check if rolloutAfter exists in spec properties
			specProperties := property.GetValue().GetProperties().AdditionalProperties
			if specProperties == nil {
				return false
			}
			for _, specProperty := range specProperties {
				if specProperty.GetName() == field {
					return true
				}
			}
		}
	}

	return false
}
