/*
Copyright 2021 The Kubernetes Authors.

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

package mergepatch

import (
	"bytes"
	"context"
	"encoding/json"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/contract"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Helper helps with a patch that yields the modified document when applied to the original document.
type Helper struct {
	client client.Client

	// original holds the object to which the patch should apply to, to be used in the Patch method.
	original client.Object

	// patch holds the merge patch in json format.
	patch []byte

	// hasSpecChanges documents if the patch impacts the object spec
	hasSpecChanges bool
}

// NewHelper will return a patch that yields the modified document when applied to the original document.
// NOTE: patch helper consider changes only in metadata.labels, metadata.annotation and spec.
// NOTE: In the case of ClusterTopologyReconciler, original is the current object, modified is the desired object, and
// the patch returns all the changes required to align current to what is defined in desired; fields not defined in desired
// are going to be preserved without changes.
func NewHelper(original, modified client.Object, c client.Client, opts ...HelperOption) (*Helper, error) {
	helperOptions := &HelperOptions{}
	helperOptions = helperOptions.ApplyOptions(opts)
	helperOptions.allowedPaths = []contract.Path{
		{"metadata", "labels"},
		{"metadata", "annotations"},
		{"spec"},
	}

	// Convert the input objects to json.
	originalJSON, err := json.Marshal(original)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal original object to json")
	}

	modifiedJSON, err := json.Marshal(modified)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal modified object to json")
	}

	// Compute the merge patch that will align the original object to the target
	// state defined above; this patch overrides the two-way merge patch for the
	// authoritative paths, if any.
	var authoritativePatch []byte
	if helperOptions.authoritativePaths != nil {
		authoritativePatch, err = jsonpatch.CreateMergePatch(originalJSON, modifiedJSON)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create merge patch for authoritative paths")
		}
	}

	// Apply the modified object to the original one, merging the values of both;
	// in case of conflicts, values from the modified object are preserved.
	originalWithModifiedJSON, err := jsonpatch.MergePatch(originalJSON, modifiedJSON)
	if err != nil {
		return nil, errors.Wrap(err, "failed to apply modified json to original json")
	}

	// Compute the merge patch that will align the original object to the target
	// state defined above.
	twoWayPatch, err := jsonpatch.CreateMergePatch(originalJSON, originalWithModifiedJSON)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create merge patch")
	}

	// We should consider only the changes that are relevant for the topology, removing
	// changes for metadata fields computed by the system or changes to the  status.
	ret, err := applyPathOptions(&applyPathOptionsInput{
		authoritativePatch: authoritativePatch,
		twoWayPatch:        twoWayPatch,
		options:            helperOptions,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to applyPathOptions")
	}

	return &Helper{
		client:         c,
		patch:          ret.patch,
		hasSpecChanges: ret.hasSpecChanges,
		original:       original,
	}, nil
}

type applyPathOptionsInput struct {
	authoritativePatch []byte
	twoWayPatch        []byte
	options            *HelperOptions
}

type applyPathOptionsOutput struct {
	patch          []byte
	hasSpecChanges bool
}

// applyPathOptions applies all the options acting on path level; currently it removes from the patch diffs not
// in the allowed paths, filters out path to be ignored and enforce authoritative paths.
// It also returns a flag indicating if the resulting patch has spec changes or not.
func applyPathOptions(in *applyPathOptionsInput) (*applyPathOptionsOutput, error) {
	twoWayPatchMap := make(map[string]interface{})
	if err := json.Unmarshal(in.twoWayPatch, &twoWayPatchMap); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal two way merge patch")
	}

	// Enforce changes from the authoritative patch when required.
	// This will override instance specific fields for a subset of fields,
	// e.g. machine template metadata changes should be reflected into generated objects without
	// accounting for instance specific changes like we do for other maps into spec.
	if len(in.options.authoritativePaths) > 0 {
		authoritativePatchMap := make(map[string]interface{})
		if err := json.Unmarshal(in.authoritativePatch, &authoritativePatchMap); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal authoritative merge patch")
		}

		for _, path := range in.options.authoritativePaths {
			enforcePath(authoritativePatchMap, twoWayPatchMap, path)
		}
	}

	// Removes from diffs everything not in allowed paths.
	filterPaths(twoWayPatchMap, in.options.allowedPaths)

	// Removes from diffs everything in ignore paths.
	for _, path := range in.options.ignorePaths {
		removePath(twoWayPatchMap, path)
	}

	// check if the changes impact the spec field.
	hasSpecChanges := twoWayPatchMap["spec"] != nil

	// converts Map back into the patch
	filteredPatch, err := json.Marshal(&twoWayPatchMap)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal merge patch")
	}
	return &applyPathOptionsOutput{
		patch:          filteredPatch,
		hasSpecChanges: hasSpecChanges,
	}, nil
}

// filterPaths removes from the patchMap diffs not in the allowed paths.
func filterPaths(patchMap map[string]interface{}, allowedPaths []contract.Path) {
	// Loop through the entries in the map.
	for k, m := range patchMap {
		// Check if item is in the allowed paths.
		allowed := false
		for _, path := range allowedPaths {
			if k == path[0] {
				allowed = true
				break
			}
		}

		// If the items isn't in the allowed paths, remove it from the map.
		if !allowed {
			delete(patchMap, k)
			continue
		}

		// If the item is allowed, process then nested map with the subset of
		// allowed paths relevant for this context
		nestedMap, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		nestedPaths := make([]contract.Path, 0)
		for _, path := range allowedPaths {
			if k == path[0] && len(path) > 1 {
				nestedPaths = append(nestedPaths, path[1:])
			}
		}
		if len(nestedPaths) == 0 {
			continue
		}
		filterPaths(nestedMap, nestedPaths)

		// Ensure we are not leaving empty maps around.
		if len(nestedMap) == 0 {
			delete(patchMap, k)
		}
	}
}

// removePath removes from the patchMap diffs a given path.
func removePath(patchMap map[string]interface{}, path contract.Path) {
	switch len(path) {
	case 0:
		// If path is empty, no-op.
		return
	case 1:
		// If we are at the end of a path, remove the corresponding entry.
		delete(patchMap, path[0])
	default:
		// If in the middle of a path, go into the nested map.
		nestedMap, ok := patchMap[path[0]].(map[string]interface{})
		if !ok {
			// If the path is not a map, return (not a full match).
			return
		}
		removePath(nestedMap, path[1:])

		// Ensure we are not leaving empty maps around.
		if len(nestedMap) == 0 {
			delete(patchMap, path[0])
		}
	}
}

// enforcePath enforces a path from authoritativeMap into the twoWayMap thus
// enforcing changes aligned to the modified object for the authoritative paths.
func enforcePath(authoritative, twoWay map[string]interface{}, path contract.Path) {
	switch len(path) {
	case 0:
		// If path is empty, no-op.
		return
	case 1:
		// If we are at the end of a path, enforce the value.

		// If there is an authoritative change for the value, apply it.
		if authoritativeChange, authoritativeHasChange := authoritative[path[0]]; authoritativeHasChange {
			twoWay[path[0]] = authoritativeChange
			return
		}

		// Else, if there is no authoritative change but there is a twoWays change for the value, blank it out.
		delete(twoWay, path[0])

	default:
		// If in the middle of a path, go into the nested map,
		nestedSimpleMap, ok := authoritative[path[0]].(map[string]interface{})

		// If the value is not a map (is a value or a list), return (not a full match).
		if !ok {
			return
		}

		// Get the corresponding map in the two-way patch.
		nestedTwoWayMap, ok := twoWay[path[0]].(map[string]interface{})
		if !ok {
			// If the path is empty, we need to fill it with unstructured maps.
			nestedTwoWayMap = map[string]interface{}{}
			twoWay[path[0]] = nestedTwoWayMap
		}

		// Enforce the nested path.
		enforcePath(nestedSimpleMap, nestedTwoWayMap, path[1:])

		// Ensure we are not leaving empty maps around.
		if len(nestedTwoWayMap) == 0 {
			delete(twoWay, path[0])
		}
	}
}

// HasSpecChanges return true if the patch has changes to the spec field.
func (h *Helper) HasSpecChanges() bool {
	return h.hasSpecChanges
}

// HasChanges return true if the patch has changes.
func (h *Helper) HasChanges() bool {
	return !bytes.Equal(h.patch, []byte("{}"))
}

// Patch will attempt to apply the twoWaysPatch to the original object.
func (h *Helper) Patch(ctx context.Context) error {
	if !h.HasChanges() {
		return nil
	}

	log := ctrl.LoggerFrom(ctx)
	log.V(5).Info("Patching object", "Patch", string(h.patch))

	return h.client.Patch(ctx, h.original, client.RawPatch(types.MergePatchType, h.patch))
}
