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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/internal/contract"
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
// the patch returns all the changes required to align current to what is defined in desired; fields not managed
// by the topology controller are going to be preserved without changes.
func NewHelper(original, modified client.Object, c client.Client, opts ...HelperOption) (*Helper, error) {
	helperOptions := &HelperOptions{}
	helperOptions = helperOptions.ApplyOptions(opts)
	helperOptions.allowedPaths = []contract.Path{
		{"metadata", "labels"},
		{"metadata", "annotations"},
		{"spec"}, // NOTE: The handling of managed path requires/assumes spec to be within allowed path.
	}

	// Infer the list of paths managed by the topology controller in the previous patch operation;
	// changes to those paths are going to be considered authoritative.
	managedPaths, err := getManagedPaths(original)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get managed paths")
	}
	helperOptions.managedPaths = managedPaths

	// Convert the input objects to json.
	originalJSON, err := json.Marshal(original)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal original object to json")
	}

	// Store the list of paths managed by the topology controller in the current patch operation;
	// this information will be used by the next patch operation.
	modifiedWithManagedFieldAnnotation, err := deepCopyWithManagedFieldAnnotation(modified, helperOptions.ignorePaths)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a copy of object with the managed field annotation")
	}

	modifiedJSON, err := json.Marshal(modifiedWithManagedFieldAnnotation)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal modified object to json")
	}

	// Compute the merge patch that will align the original object to the target
	// state defined above; this patch overrides the two-way merge patch for both the
	// authoritative and the managed paths, if any.
	var authoritativePatch []byte
	if len(helperOptions.authoritativePaths) > 0 || len(helperOptions.managedPaths) > 0 {
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
		modified:           modifiedJSON,
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
	modified           []byte
	options            *HelperOptions
}

type applyPathOptionsOutput struct {
	patch          []byte
	hasSpecChanges bool
}

// applyPathOptions applies all the options acting on path level; currently it removes from the patch diffs not
// in the allowed paths, filters out path to be ignored and enforce authoritative and managed paths.
// It also returns a flag indicating if the resulting patch has spec changes or not.
func applyPathOptions(in *applyPathOptionsInput) (*applyPathOptionsOutput, error) {
	twoWayPatchMap := make(map[string]interface{})
	if err := json.Unmarshal(in.twoWayPatch, &twoWayPatchMap); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal two way merge patch")
	}

	// Enforce changes from authoritative patch when required (authoritative and managed paths).
	// This will override instance specific fields for a subset of fields,
	// e.g. machine template metadata changes should be reflected into generated objects without
	// accounting for instance specific changes like we do for other maps into spec.
	if len(in.options.authoritativePaths) > 0 || len(in.options.managedPaths) > 0 {
		authoritativePatchMap := make(map[string]interface{})
		if err := json.Unmarshal(in.authoritativePatch, &authoritativePatchMap); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal authoritative merge patch")
		}

		modifiedMap := make(map[string]interface{})
		if err := json.Unmarshal(in.modified, &modifiedMap); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal modified")
		}

		for _, path := range append(in.options.managedPaths, in.options.authoritativePaths...) {
			enforcePath(authoritativePatchMap, modifiedMap, twoWayPatchMap, path)
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
func enforcePath(authoritative, modified, twoWay map[string]interface{}, path contract.Path) {
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
		var nestedSimpleMap map[string]interface{}
		switch v, ok := authoritative[path[0]]; {
		case ok && v == nil:
			// If the nested map is nil, it means that the authoritative patch is trying to delete a parent object
			// in the middle of the enforced path.

			// If the parent object has been intentionally deleted (the corresponding parent object value in the modified object is null),
			// then we should enforce the deletion of the parent object including everything below it.
			if m, ok := modified[path[0]]; ok && m == nil {
				twoWay[path[0]] = nil
				return
			}

			// Otherwise, we continue processing the enforced path, thus deleting only what
			// is explicitly enforced.
			nestedSimpleMap = map[string]interface{}{path[1]: nil}
		default:
			nestedSimpleMap, ok = v.(map[string]interface{})

			// NOTE: This should never happen given how Unmarshal works
			// when generating the authoritative, but adding this as an extra safety
			if !ok {
				return
			}
		}

		// Get the corresponding map in the two-way patch.
		nestedTwoWayMap, ok := twoWay[path[0]].(map[string]interface{})
		if !ok {
			// If the path is empty, we need to fill it with unstructured maps.
			nestedTwoWayMap = map[string]interface{}{}
			twoWay[path[0]] = nestedTwoWayMap
		}

		// Get the corresponding value in modified.
		nestedModified, _ := modified[path[0]].(map[string]interface{})

		// Enforce the nested path.
		enforcePath(nestedSimpleMap, nestedModified, nestedTwoWayMap, path[1:])

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

	// Note: deepcopy before patching in order to avoid modifications to the original object.
	return h.client.Patch(ctx, h.original.DeepCopyObject().(client.Object), client.RawPatch(types.MergePatchType, h.patch))
}
