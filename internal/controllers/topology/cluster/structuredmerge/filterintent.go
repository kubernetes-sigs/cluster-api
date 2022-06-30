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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"sigs.k8s.io/cluster-api/internal/contract"
)

// filterObject filter out changes not relevant for the topology controller.
func filterObject(obj *unstructured.Unstructured, helperOptions *HelperOptions) {
	// filter out changes not in the allowed paths (fields to not consider, e.g. status);
	if len(helperOptions.allowedPaths) > 0 {
		filterIntent(&filterIntentInput{
			path:         contract.Path{},
			value:        obj.Object,
			shouldFilter: isNotAllowedPath(helperOptions.allowedPaths),
		})
	}

	// filter out changes for ignore paths (well known fields owned by other controllers, e.g.
	//   spec.controlPlaneEndpoint in the InfrastructureCluster object);
	if len(helperOptions.ignorePaths) > 0 {
		filterIntent(&filterIntentInput{
			path:         contract.Path{},
			value:        obj.Object,
			shouldFilter: isIgnorePath(helperOptions.ignorePaths),
		})
	}
}

// filterIntent ensures that object only includes the fields and values for which the topology controller has an opinion,
// and filter out everything else by removing it from the unstructured object.
// NOTE: This func is called recursively only for fields of type Map, but this is ok given the current use cases
// this func has to address. More specifically, we are using this func for filtering out not allowed paths and for ignore paths;
// all of them are defined in reconcile_state.go and are targeting well-known fields inside nested maps.
// Allowed paths / ignore paths which point to an array are not supported by the current implementation.
func filterIntent(ctx *filterIntentInput) bool {
	value, ok := ctx.value.(map[string]interface{})
	if !ok {
		return false
	}

	gotDeletions := false
	for field := range value {
		fieldCtx := &filterIntentInput{
			// Compose the path for the nested field.
			path: ctx.path.Append(field),
			// Gets the original and the modified value for the field.
			value: value[field],
			// Carry over global values from the context.
			shouldFilter: ctx.shouldFilter,
		}

		// If the field should be filtered out, delete it from the modified object.
		if fieldCtx.shouldFilter(fieldCtx.path) {
			delete(value, field)
			gotDeletions = true
			continue
		}

		// Process nested fields and get in return if filterIntent removed fields.
		if filterIntent(fieldCtx) {
			// Ensure we are not leaving empty maps around.
			if v, ok := fieldCtx.value.(map[string]interface{}); ok && len(v) == 0 {
				delete(value, field)
				gotDeletions = true
			}
		}
	}
	return gotDeletions
}

// filterIntentInput holds info required while filtering the intent for server side apply.
// NOTE: in server side apply an intent is a partial object that only includes the fields and values for which the user has an opinion.
type filterIntentInput struct {
	// the path of the field being processed.
	path contract.Path

	// the value for the current path.
	value interface{}

	// shouldFilter handle the func that determine if the current path should be dropped or not.
	shouldFilter func(path contract.Path) bool
}

// isAllowedPath returns true when the path is one of the allowedPaths.
func isAllowedPath(allowedPaths []contract.Path) func(path contract.Path) bool {
	return func(path contract.Path) bool {
		for _, p := range allowedPaths {
			// NOTE: we allow everything Equal or one IsParentOf one of the allowed paths.
			// e.g. if allowed path is metadata.labels, we allow both metadata and metadata.labels;
			// this is required because allowed path is called recursively.
			if path.Overlaps(p) {
				return true
			}
		}
		return false
	}
}

// isNotAllowedPath returns true when the path is NOT one of the allowedPaths.
func isNotAllowedPath(allowedPaths []contract.Path) func(path contract.Path) bool {
	return func(path contract.Path) bool {
		isAllowed := isAllowedPath(allowedPaths)
		return !isAllowed(path)
	}
}

// isIgnorePath returns true when the path is one of the ignorePaths.
func isIgnorePath(ignorePaths []contract.Path) func(path contract.Path) bool {
	return func(path contract.Path) bool {
		for _, p := range ignorePaths {
			if path.Equal(p) {
				return true
			}
		}
		return false
	}
}
