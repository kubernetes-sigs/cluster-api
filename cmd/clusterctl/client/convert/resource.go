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
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// convertResource converts a single resource to the target GroupVersion.
func convertResource(obj runtime.Object, targetGV schema.GroupVersion, scheme *runtime.Scheme, targetAPIGroup string) (runtime.Object, bool, error) {
	gvk := obj.GetObjectKind().GroupVersionKind()

	if !shouldConvert(gvk, targetGV.Version, targetAPIGroup) {
		return obj, false, nil
	}

	targetGVK := schema.GroupVersionKind{
		Group:   targetGV.Group,
		Version: targetGV.Version,
		Kind:    gvk.Kind,
	}

	// Verify the target type exists in the scheme.
	if !scheme.Recognizes(targetGVK) {
		return nil, false, errors.Errorf("target GVK %s not recognized by scheme", targetGVK.String())
	}

	if convertible, ok := obj.(conversion.Convertible); ok {
		// Create a new instance of the target type.
		targetObj, err := scheme.New(targetGVK)
		if err != nil {
			return nil, false, errors.Wrapf(err, "failed to create target object for %s", targetGVK.String())
		}

		// Check if the target object is a Hub.
		if hub, ok := targetObj.(conversion.Hub); ok {
			if err := convertible.ConvertTo(hub); err != nil {
				return nil, false, errors.Wrapf(err, "failed to convert %s from %s to %s", gvk.Kind, gvk.Version, targetGV.Version)
			}

			// Ensure the GVK is set on the converted object.
			hubObj := hub.(runtime.Object)
			hubObj.GetObjectKind().SetGroupVersionKind(targetGVK)

			return hubObj, true, nil
		}
	}

	convertedObj, err := scheme.ConvertToVersion(obj, targetGVK.GroupVersion())
	if err != nil {
		return nil, false, errors.Wrapf(err, "failed to convert %s from %s to %s", gvk.Kind, gvk.Version, targetGV.Version)
	}

	return convertedObj, true, nil
}

// shouldConvert determines if a resource needs conversion based on its GVK and target version.
func shouldConvert(gvk schema.GroupVersionKind, targetVersion string, targetAPIGroup string) bool {
	// Only convert resources from the target API group.
	if gvk.Group != targetAPIGroup {
		return false
	}

	// Don't convert if already at target version.
	if gvk.Version == targetVersion {
		return false
	}

	return true
}

// getInfoMessage returns an informational message for resources that don't need conversion.
func getInfoMessage(gvk schema.GroupVersionKind, targetVersion string, targetAPIGroup string) string {
	if gvk.Group == targetAPIGroup && gvk.Version == targetVersion {
		return fmt.Sprintf("Resource %s is already at version %s", gvk.Kind, targetVersion)
	}

	// All other resources are from different API groups (pass-through).
	return fmt.Sprintf("Skipping non-%s resource: %s", targetAPIGroup, gvk.String())
}
