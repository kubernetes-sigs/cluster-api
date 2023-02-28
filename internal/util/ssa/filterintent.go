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

package ssa

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"sigs.k8s.io/cluster-api/internal/contract"
)

// FilterObjectInput holds info required while filtering the object.
type FilterObjectInput struct {
	// AllowedPaths instruct FilterObject to ignore everything except given paths.
	AllowedPaths []contract.Path

	// IgnorePaths instruct FilterObject to ignore given paths.
	// NOTE: IgnorePaths are used to filter out fields nested inside AllowedPaths, e.g.
	// spec.ControlPlaneEndpoint.
	// NOTE: ignore paths which point to an array are not supported by the current implementation.
	IgnorePaths []contract.Path
}

// FilterObject filter out changes not relevant for the controller.
func FilterObject(obj *unstructured.Unstructured, input *FilterObjectInput) {
	// filter out changes not in the allowed paths (fields to not consider, e.g. status);
	if len(input.AllowedPaths) > 0 {
		FilterIntent(&FilterIntentInput{
			Path:         contract.Path{},
			Value:        obj.Object,
			ShouldFilter: IsNotAllowedPath(input.AllowedPaths),
		})
	}

	// filter out changes for ignore paths (well known fields owned by other controllers, e.g.
	//   spec.controlPlaneEndpoint in the InfrastructureCluster object);
	if len(input.IgnorePaths) > 0 {
		FilterIntent(&FilterIntentInput{
			Path:         contract.Path{},
			Value:        obj.Object,
			ShouldFilter: IsIgnorePath(input.IgnorePaths),
		})
	}
}

// FilterIntent ensures that object only includes the fields and values for which the controller has an opinion,
// and filter out everything else by removing it from the Value.
// NOTE: This func is called recursively only for fields of type Map, but this is ok given the current use cases
// this func has to address. More specifically, we are using this func for filtering out not allowed paths and for ignore paths;
// all of them are defined in reconcile_state.go and are targeting well-known fields inside nested maps.
// Allowed paths / ignore paths which point to an array are not supported by the current implementation.
func FilterIntent(ctx *FilterIntentInput) bool {
	value, ok := ctx.Value.(map[string]interface{})
	if !ok {
		return false
	}

	gotDeletions := false
	for field := range value {
		fieldCtx := &FilterIntentInput{
			// Compose the Path for the nested field.
			Path: ctx.Path.Append(field),
			// Gets the original and the modified Value for the field.
			Value: value[field],
			// Carry over global values from the context.
			ShouldFilter: ctx.ShouldFilter,
		}

		// If the field should be filtered out, delete it from the modified object.
		if fieldCtx.ShouldFilter(fieldCtx.Path) {
			delete(value, field)
			gotDeletions = true
			continue
		}

		// Process nested fields and get in return if FilterIntent removed fields.
		if FilterIntent(fieldCtx) {
			// Ensure we are not leaving empty maps around.
			if v, ok := fieldCtx.Value.(map[string]interface{}); ok && len(v) == 0 {
				delete(value, field)
				gotDeletions = true
			}
		}
	}
	return gotDeletions
}

// FilterIntentInput holds info required while filtering the intent for server side apply.
// NOTE: in server side apply an intent is a partial object that only includes the fields and values for which the user has an opinion.
type FilterIntentInput struct {
	// the Path of the field being processed.
	Path contract.Path

	// the Value for the current Path.
	Value interface{}

	// ShouldFilter handle the func that determine if the current Path should be dropped or not.
	ShouldFilter func(path contract.Path) bool
}

// IsAllowedPath returns true when the Path is one of the AllowedPaths.
func IsAllowedPath(allowedPaths []contract.Path) func(path contract.Path) bool {
	return func(path contract.Path) bool {
		for _, p := range allowedPaths {
			// NOTE: we allow everything Equal or one IsParentOf one of the allowed paths.
			// e.g. if allowed Path is metadata.labels, we allow both metadata and metadata.labels;
			// this is required because allowed Path is called recursively.
			if path.Overlaps(p) {
				return true
			}
		}
		return false
	}
}

// IsNotAllowedPath returns true when the Path is NOT one of the AllowedPaths.
func IsNotAllowedPath(allowedPaths []contract.Path) func(path contract.Path) bool {
	return func(path contract.Path) bool {
		isAllowed := IsAllowedPath(allowedPaths)
		return !isAllowed(path)
	}
}

// IsIgnorePath returns true when the Path is one of the IgnorePaths.
func IsIgnorePath(ignorePaths []contract.Path) func(path contract.Path) bool {
	return func(path contract.Path) bool {
		for _, p := range ignorePaths {
			if path.Equal(p) {
				return true
			}
		}
		return false
	}
}
