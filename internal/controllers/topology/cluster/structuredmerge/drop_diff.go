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

import "sigs.k8s.io/cluster-api/internal/contract"

// dropDiff allow to change the modified object so the generated patch will not contain changes
// that match the shouldDropDiff criteria.
// NOTE: This func is called recursively only for fields of type Map, but this is ok given the current use cases
// this func has to address. More specifically, we are using only for not allowed paths and for ignore paths;
// all of them are defined in reconcile_state.go and are targeting well-known fields inside nested maps.
// Allowed paths / ignore paths which point to an array are not supported by the current implementation.
func dropDiff(ctx *dropDiffInput) {
	original, _ := ctx.original.(map[string]interface{})
	modified, _ := ctx.modified.(map[string]interface{})
	for field := range modified {
		fieldCtx := &dropDiffInput{
			// Compose the path for the nested field.
			path: ctx.path.Append(field),
			// Gets the original and the modified value for the field.
			original: original[field],
			modified: modified[field],
			// Carry over global values from the context.
			shouldDropDiffFunc: ctx.shouldDropDiffFunc,
		}

		// Note: for everything we should drop changes we are making modified equal to original, so the generated patch doesn't include this change
		if fieldCtx.shouldDropDiffFunc(fieldCtx.path) {
			// If original exists, make modified equal to original, otherwise if original does not exist, drop the change.
			if o, ok := original[field]; ok {
				modified[field] = o
			} else {
				delete(modified, field)
			}
			continue
		}

		// Process nested fields.
		dropDiff(fieldCtx)

		// Ensure we are not leaving empty maps around.
		if v, ok := fieldCtx.modified.(map[string]interface{}); ok && len(v) == 0 {
			delete(modified, field)
		}
	}
}

// dropDiffInput holds info required while computing dropDiff.
type dropDiffInput struct {
	// the path of the field being processed.
	path contract.Path

	// the original and the modified value for the current path.
	original interface{}
	modified interface{}

	// shouldDropDiffFunc handle the func that determine if the current path should be dropped or not.
	shouldDropDiffFunc func(path contract.Path) bool
}
