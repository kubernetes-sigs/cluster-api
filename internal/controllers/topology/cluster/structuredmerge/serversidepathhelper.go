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
	"context"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util"
)

// TopologyManagerName is the manager name in managed fields for the topology controller.
const TopologyManagerName = "capi-topology"

type serverSidePatchHelper struct {
	client         client.Client
	modified       *unstructured.Unstructured
	hasChanges     bool
	hasSpecChanges bool
}

// NewServerSidePatchHelper returns a new PatchHelper using server side apply.
func NewServerSidePatchHelper(ctx context.Context, original, modified client.Object, c client.Client, ssaCache ssa.Cache, opts ...HelperOption) (PatchHelper, error) {
	// Create helperOptions for filtering the original and modified objects to the desired intent.
	helperOptions := newHelperOptions(modified, opts...)

	// If required, convert the original and modified objects to unstructured and filter out all the information
	// not relevant for the topology controller.

	var originalUnstructured *unstructured.Unstructured
	if !util.IsNil(original) {
		originalUnstructured = &unstructured.Unstructured{}
		switch original.(type) {
		case *unstructured.Unstructured:
			originalUnstructured = original.DeepCopyObject().(*unstructured.Unstructured)
		default:
			if err := c.Scheme().Convert(original, originalUnstructured, nil); err != nil {
				return nil, errors.Wrap(err, "failed to convert original object to Unstructured")
			}
		}
	}

	modifiedUnstructured := &unstructured.Unstructured{}
	switch modified.(type) {
	case *unstructured.Unstructured:
		modifiedUnstructured = modified.DeepCopyObject().(*unstructured.Unstructured)
	default:
		if err := c.Scheme().Convert(modified, modifiedUnstructured, nil); err != nil {
			return nil, errors.Wrap(err, "failed to convert modified object to Unstructured")
		}
	}

	// Filter the modifiedUnstructured object to only contain changes intendet to be done.
	// The originalUnstructured object will be filtered in dryRunSSAPatch using other options.
	ssa.FilterObject(modifiedUnstructured, &ssa.FilterObjectInput{
		AllowedPaths: helperOptions.allowedPaths,
		IgnorePaths:  helperOptions.ignorePaths,
	})

	// Carry over uid to match the intent to:
	// * create (uid==""):
	//   * if object doesn't exist => create
	//   * if object already exists => update
	// * update (uid!=""):
	//   * if object doesn't exist => fail
	//   * if object already exists => update
	//   * This allows us to enforce that an update doesn't create an object which has been deleted.
	modifiedUnstructured.SetUID("")
	if originalUnstructured != nil {
		modifiedUnstructured.SetUID(originalUnstructured.GetUID())
	}

	// Determine if the intent defined in the modified object is going to trigger
	// an actual change when running server side apply, and if this change might impact the object spec or not.
	var hasChanges, hasSpecChanges bool
	switch {
	case util.IsNil(original):
		hasChanges, hasSpecChanges = true, true
	default:
		var err error
		hasChanges, hasSpecChanges, err = dryRunSSAPatch(ctx, &dryRunSSAPatchInput{
			client:               c,
			ssaCache:             ssaCache,
			originalUnstructured: originalUnstructured,
			modifiedUnstructured: modifiedUnstructured.DeepCopy(),
			helperOptions:        helperOptions,
		})
		if err != nil {
			return nil, err
		}
	}

	return &serverSidePatchHelper{
		client:         c,
		modified:       modifiedUnstructured,
		hasChanges:     hasChanges,
		hasSpecChanges: hasSpecChanges,
	}, nil
}

// HasSpecChanges return true if the patch has changes to the spec field.
func (h *serverSidePatchHelper) HasSpecChanges() bool {
	return h.hasSpecChanges
}

// HasChanges return true if the patch has changes.
func (h *serverSidePatchHelper) HasChanges() bool {
	return h.hasChanges
}

// Patch will server side apply the current intent (the modified object.
func (h *serverSidePatchHelper) Patch(ctx context.Context) error {
	if !h.HasChanges() {
		return nil
	}

	log := ctrl.LoggerFrom(ctx)
	log.V(5).Info("Patching object", "Intent", h.modified)

	options := []client.PatchOption{
		client.FieldOwner(TopologyManagerName),
		// NOTE: we are using force ownership so in case of conflicts the topology controller
		// overwrite values and become sole manager.
		client.ForceOwnership,
	}
	return h.client.Patch(ctx, h.modified, client.Apply, options...)
}
