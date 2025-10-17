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

package migrate

import (
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/scheme"
)

// Converter handles conversion of individual CAPI resources between API versions.
type Converter struct {
	scheme       *runtime.Scheme
	targetGV     schema.GroupVersion
	targetGVKMap gvkConversionMap
}

// gvkConversionMap caches conversions from a source GroupVersionKind to its target GroupVersionKind.
type gvkConversionMap map[schema.GroupVersionKind]schema.GroupVersionKind

// ConversionResult represents the outcome of converting a single resource.
type ConversionResult struct {
	Object runtime.Object
	// Converted indicates whether the object was actually converted
	Converted bool
	Error     error
	Warnings  []string
}

// NewConverter creates a new resource converter using the clusterctl scheme.
func NewConverter(targetGV schema.GroupVersion) (*Converter, error) {
	return &Converter{
		scheme:       scheme.Scheme,
		targetGV:     targetGV,
		targetGVKMap: make(gvkConversionMap),
	}, nil
}

// ConvertResource converts a single resource to the target version.
// Returns the converted object, or the original if no conversion is needed.
func (c *Converter) ConvertResource(info ResourceInfo, obj runtime.Object) ConversionResult {
	gvk := info.GroupVersionKind

	if gvk.Group == clusterv1.GroupVersion.Group && gvk.Version == c.targetGV.Version {
		return ConversionResult{
			Object:    obj,
			Converted: false,
			Warnings:  []string{fmt.Sprintf("Resource %s/%s is already at version %s", gvk.Kind, info.Name, c.targetGV.Version)},
		}
	}

	if gvk.Group != clusterv1.GroupVersion.Group {
		return ConversionResult{
			Object:    obj,
			Converted: false,
			Warnings:  []string{fmt.Sprintf("Skipping non-%s resource: %s", clusterv1.GroupVersion.Group, gvk.String())},
		}
	}

	targetGVK, err := c.getTargetGVK(gvk)
	if err != nil {
		return ConversionResult{
			Object:    obj,
			Converted: false,
			Error:     errors.Wrapf(err, "failed to determine target GVK for %s", gvk.String()),
		}
	}

	// Check if the object is already typed
	// If it's typed and implements conversion.Convertible, use the custom ConvertTo method
	if convertible, ok := obj.(conversion.Convertible); ok {
		// Create a new instance of the target type
		targetObj, err := c.scheme.New(targetGVK)
		if err != nil {
			return ConversionResult{
				Object:    obj,
				Converted: false,
				Error:     errors.Wrapf(err, "failed to create target object for %s", targetGVK.String()),
			}
		}

		// Check if the target object is a Hub
		if hub, ok := targetObj.(conversion.Hub); ok {
			if err := convertible.ConvertTo(hub); err != nil {
				return ConversionResult{
					Object:    obj,
					Converted: false,
					Error:     errors.Wrapf(err, "failed to convert %s from %s to %s", gvk.Kind, gvk.Version, c.targetGV.Version),
				}
			}

			// Ensure the GVK is set on the converted object
			hubObj := hub.(runtime.Object)
			hubObj.GetObjectKind().SetGroupVersionKind(targetGVK)

			return ConversionResult{
				Object:    hubObj,
				Converted: true,
				Error:     nil,
				Warnings:  nil,
			}
		}
	}

	// Use scheme-based conversion for all remaining cases
	convertedObj, err := c.scheme.ConvertToVersion(obj, targetGVK.GroupVersion())
	if err != nil {
		return ConversionResult{
			Object:    obj,
			Converted: false,
			Error:     errors.Wrapf(err, "failed to convert %s from %s to %s", gvk.Kind, gvk.Version, c.targetGV.Version),
		}
	}

	return ConversionResult{
		Object:    convertedObj,
		Converted: true,
		Error:     nil,
		Warnings:  nil,
	}
}

// getTargetGVK returns the target GroupVersionKind for a given source GVK.
func (c *Converter) getTargetGVK(sourceGVK schema.GroupVersionKind) (schema.GroupVersionKind, error) {
	// Check cache first
	if targetGVK, ok := c.targetGVKMap[sourceGVK]; ok {
		return targetGVK, nil
	}

	// Create target GVK with same kind but target version
	targetGVK := schema.GroupVersionKind{
		Group:   c.targetGV.Group,
		Version: c.targetGV.Version,
		Kind:    sourceGVK.Kind,
	}

	// Verify the target type exists in the scheme
	if !c.scheme.Recognizes(targetGVK) {
		return schema.GroupVersionKind{}, errors.Errorf("target GVK %s not recognized by scheme", targetGVK.String())
	}

	// Cache for future use
	c.targetGVKMap[sourceGVK] = targetGVK

	return targetGVK, nil
}
