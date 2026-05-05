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
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// convertResource converts a single resource to the target GroupVersion.
func convertResource(obj runtime.Object, targetGV schema.GroupVersion, scheme *runtime.Scheme) (runtime.Object, error) {
	gvk := obj.GetObjectKind().GroupVersionKind()

	// Already at target version, nothing to do.
	if gvk.Version == targetGV.Version {
		return obj, nil
	}

	targetGVK := schema.GroupVersionKind{
		Group:   targetGV.Group,
		Version: targetGV.Version,
		Kind:    gvk.Kind,
	}

	if !scheme.Recognizes(targetGVK) {
		return nil, errors.Errorf("target GVK %s not recognized by scheme", targetGVK.String())
	}

	// Use the Convertible/Hub interface if available (preferred path for CAPI types).
	if convertible, ok := obj.(conversion.Convertible); ok {
		targetObj, err := scheme.New(targetGVK)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create target object for %s", targetGVK.String())
		}

		if hub, ok := targetObj.(conversion.Hub); ok {
			if err := convertible.ConvertTo(hub); err != nil {
				return nil, errors.Wrapf(err, "failed to convert %s from %s to %s", gvk.Kind, gvk.Version, targetGV.Version)
			}

			hubObj := hub.(runtime.Object)
			hubObj.GetObjectKind().SetGroupVersionKind(targetGVK)
			return hubObj, nil
		}
	}

	// Fallback to scheme conversion.
	convertedObj, err := scheme.ConvertToVersion(obj, targetGVK.GroupVersion())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert %s from %s to %s", gvk.Kind, gvk.Version, targetGV.Version)
	}

	return convertedObj, nil
}
