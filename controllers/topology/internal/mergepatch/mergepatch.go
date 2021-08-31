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

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/contract"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var allowedPaths = []contract.Path{
	{"metadata", "labels"},
	{"metadata", "annotations"},
	{"spec"},
}

// Helper helps with a patch that yields the modified document when applied to the original document.
type Helper struct {
	client client.Client

	// original holds the object to which the patch should apply to, to be used in the Patch method.
	original client.Object

	// patch holds the merge patch in json format.
	patch []byte
}

// NewHelper will return a patch that yields the modified document when applied to the original document.
// NOTE: In the case of ClusterTopologyReconciler, original is the current object, modified is the desired object, and
// the patch returns all the changes required to align current to what is defined in desired; fields not defined in desired
// are going to be preserved without changes.
func NewHelper(original, modified client.Object, c client.Client, opts ...HelperOption) (*Helper, error) {
	helperOptions := &HelperOptions{}
	helperOptions = helperOptions.ApplyOptions(opts)

	// Convert the input objects to json.
	originalJSON, err := json.Marshal(original)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal original object to json")
	}

	modifiedJSON, err := json.Marshal(modified)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal modified object to json")
	}

	// Apply the modified object to the original one, merging the values of both;
	// in case of conflicts, values from the modified object are preserved.
	originalWithModifiedJSON, err := jsonpatch.MergePatch(originalJSON, modifiedJSON)
	if err != nil {
		return nil, errors.Wrap(err, "failed to apply modified json to original json")
	}

	// Compute the merge patch that will align the original object to the target
	// state defined above.
	rawPatch, err := jsonpatch.CreateMergePatch(originalJSON, originalWithModifiedJSON)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create merge patch")
	}

	// We should consider only the changes that are relevant for the topology, removing
	// changes for metadata fields computed by the system or changes to the  status.
	patch, err := filterPatch(rawPatch, allowedPaths, helperOptions.ignorePaths)
	if err != nil {
		return nil, errors.Wrap(err, "failed to remove fields merge patch")
	}

	return &Helper{
		client:   c,
		patch:    patch,
		original: original,
	}, nil
}

// filterPatch removes from the patch diffs not in the allowed paths.
func filterPatch(patch []byte, allowedPaths, ignorePaths []contract.Path) ([]byte, error) {
	// converts the patch into a Map
	patchMap := make(map[string]interface{})
	err := json.Unmarshal(patch, &patchMap)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal merge patch")
	}

	// Removes from diffs everything not in the allowed paths.
	filterPatchMap(patchMap, allowedPaths)

	// Removes from diffs everything in the ignore paths.
	for _, path := range ignorePaths {
		removePath(patchMap, path)
	}

	// converts Map back into the patch
	patch, err = json.Marshal(&patchMap)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal merge patch")
	}
	return patch, nil
}

// filterPatch removes from the patchMap diffs not in the allowed paths.
func filterPatchMap(patchMap map[string]interface{}, allowedPaths []contract.Path) {
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
		filterPatchMap(nestedMap, nestedPaths)

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
		// if path is empty, no-op.
		return
	case 1:
		// if we are at the end of a path, remove the corresponding entry.
		delete(patchMap, path[0])
	default:
		// if in the middle of a path, go into the nested map,
		nestedMap, ok := patchMap[path[0]].(map[string]interface{})
		if !ok {
			return
		}
		removePath(nestedMap, path[1:])

		// Ensure we are not leaving empty maps around.
		if len(nestedMap) == 0 {
			delete(patchMap, path[0])
		}
	}
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
