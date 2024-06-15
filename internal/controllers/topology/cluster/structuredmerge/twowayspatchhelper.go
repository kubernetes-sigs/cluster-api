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

package structuredmerge

import (
	"bytes"
	"context"
	"encoding/json"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util"
)

// TwoWaysPatchHelper helps with a patch that yields the modified document when applied to the original document.
type TwoWaysPatchHelper struct {
	client client.Client

	// original holds the object to which the patch should apply to, to be used in the Patch method.
	original client.Object

	// patch holds the merge patch in json format.
	patch []byte

	// hasSpecChanges documents if the patch impacts the object spec
	hasSpecChanges bool
	changes        []byte
}

// NewTwoWaysPatchHelper will return a patch that yields the modified document when applied to the original document
// using the two-ways merge algorithm.
// NOTE: In the case of ClusterTopologyReconciler, original is the current object, modified is the desired object, and
// the patch returns all the changes required to align current to what is defined in desired; fields not managed
// by the topology controller are going to be preserved without changes.
// NOTE: TwoWaysPatch is considered a minimal viable replacement for server side apply during topology dry run, with
// the following limitations:
//   - TwoWaysPatch doesn't consider OpenAPI schema extension like +ListMap this can lead to false positive when topology
//     dry run is simulating a change to an existing slice
//     (TwoWaysPatch always revert external changes, like server side apply when +ListMap=atomic).
//   - TwoWaysPatch doesn't consider existing metadata.managedFields, and this can lead to false negative when topology dry run
//     is simulating a change to an existing object where the topology controller is dropping an opinion for a field
//     (TwoWaysPatch always preserve dropped fields, like server side apply when the field has more than one manager).
//   - TwoWaysPatch doesn't generate metadata.managedFields as server side apply does.
//
// NOTE: NewTwoWaysPatchHelper consider changes only in metadata.labels, metadata.annotation and spec; it also respects
// the ignorePath option (same as the server side apply helper).
func NewTwoWaysPatchHelper(original, modified client.Object, c client.Client, opts ...HelperOption) (*TwoWaysPatchHelper, error) {
	helperOptions := &HelperOptions{}
	helperOptions = helperOptions.ApplyOptions(opts)
	helperOptions.AllowedPaths = []contract.Path{
		{"metadata", "labels"},
		{"metadata", "annotations"},
		{"spec"}, // NOTE: The handling of managed path requires/assumes spec to be within allowed path.
	}
	// In case we are creating an object, we extend the set of allowed fields adding apiVersion, Kind
	// metadata.name, metadata.namespace (who are required by the API server) and metadata.ownerReferences
	// that gets set to avoid orphaned objects.
	if util.IsNil(original) {
		helperOptions.AllowedPaths = append(helperOptions.AllowedPaths,
			contract.Path{"apiVersion"},
			contract.Path{"kind"},
			contract.Path{"metadata", "name"},
			contract.Path{"metadata", "namespace"},
			contract.Path{"metadata", "ownerReferences"},
		)
	}

	// Convert the input objects to json; if original is nil, use empty object so the
	// following logic works without panicking.
	originalJSON, err := json.Marshal(original)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal original object to json")
	}
	if util.IsNil(original) {
		originalJSON = []byte("{}")
	}

	modifiedJSON, err := json.Marshal(modified)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal modified object to json")
	}

	// Apply patch options including:
	// - exclude paths (fields to not consider, e.g. status);
	// - ignore paths (well known fields owned by something else, e.g. spec.controlPlaneEndpoint in the
	//   InfrastructureCluster object);
	// NOTE: All the above options trigger changes in the modified object so the resulting two ways patch
	// includes or not the specific change.
	modifiedJSON, err = applyOptions(&applyOptionsInput{
		original: originalJSON,
		modified: modifiedJSON,
		options:  helperOptions,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to apply options to modified")
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

	twoWayPatchMap := make(map[string]interface{})
	if err := json.Unmarshal(twoWayPatch, &twoWayPatchMap); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal two way merge patch")
	}

	hasChanges := len(twoWayPatchMap) > 0
	// check if the changes impact the spec field.
	hasSpecChanges := twoWayPatchMap["spec"] != nil

	var changes []byte
	if hasChanges {
		// Cleanup diff by dropping .metadata.managedFields.
		ssa.FilterIntent(&ssa.FilterIntentInput{
			Path:         contract.Path{},
			Value:        twoWayPatchMap,
			ShouldFilter: ssa.IsPathIgnored([]contract.Path{[]string{"metadata", "managedFields"}}),
		})

		changes, err = json.Marshal(twoWayPatchMap)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to marshal diff")
		}
	}

	return &TwoWaysPatchHelper{
		client:         c,
		patch:          twoWayPatch,
		hasSpecChanges: hasSpecChanges,
		changes:        changes,
		original:       original,
	}, nil
}

type applyOptionsInput struct {
	original []byte
	modified []byte
	options  *HelperOptions
}

// Apply patch options changing the modified object so the resulting two ways patch
// includes or not the specific change.
func applyOptions(in *applyOptionsInput) ([]byte, error) {
	originalMap := make(map[string]interface{})
	if err := json.Unmarshal(in.original, &originalMap); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal original")
	}

	modifiedMap := make(map[string]interface{})
	if err := json.Unmarshal(in.modified, &modifiedMap); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal modified")
	}

	// drop changes for exclude paths (fields to not consider, e.g. status);
	// Note: for everything not allowed it sets modified equal to original, so the generated patch doesn't include this change
	if len(in.options.AllowedPaths) > 0 {
		dropDiff(&dropDiffInput{
			path:               contract.Path{},
			original:           originalMap,
			modified:           modifiedMap,
			shouldDropDiffFunc: ssa.IsPathNotAllowed(in.options.AllowedPaths),
		})
	}

	// drop changes for ignore paths (well known fields owned by something else, e.g.
	//   spec.controlPlaneEndpoint in the InfrastructureCluster object);
	// Note: for everything ignored it sets  modified equal to original, so the generated patch doesn't include this change
	if len(in.options.IgnorePaths) > 0 {
		dropDiff(&dropDiffInput{
			path:               contract.Path{},
			original:           originalMap,
			modified:           modifiedMap,
			shouldDropDiffFunc: ssa.IsPathIgnored(in.options.IgnorePaths),
		})
	}

	modified, err := json.Marshal(&modifiedMap)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal modified")
	}

	return modified, nil
}

// HasSpecChanges return true if the patch has changes to the spec field.
func (h *TwoWaysPatchHelper) HasSpecChanges() bool {
	return h.hasSpecChanges
}

// Changes return the changes.
func (h *TwoWaysPatchHelper) Changes() []byte {
	return h.changes
}

// HasChanges return true if the patch has changes.
func (h *TwoWaysPatchHelper) HasChanges() bool {
	return !bytes.Equal(h.patch, []byte("{}"))
}

// Patch will attempt to apply the twoWaysPatch to the original object.
func (h *TwoWaysPatchHelper) Patch(ctx context.Context) error {
	if !h.HasChanges() {
		return nil
	}
	log := ctrl.LoggerFrom(ctx)

	if util.IsNil(h.original) {
		modifiedMap := make(map[string]interface{})
		if err := json.Unmarshal(h.patch, &modifiedMap); err != nil {
			return errors.Wrap(err, "failed to unmarshal two way merge patch")
		}

		obj := &unstructured.Unstructured{
			Object: modifiedMap,
		}
		return h.client.Create(ctx, obj)
	}

	// Note: deepcopy before patching in order to avoid modifications to the original object.
	log.V(5).Info("Patching object", "patch", string(h.patch))
	return h.client.Patch(ctx, h.original.DeepCopyObject().(client.Object), client.RawPatch(types.MergePatchType, h.patch))
}
