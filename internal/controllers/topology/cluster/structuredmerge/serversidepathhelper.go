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
	"encoding/json"

	"github.com/pkg/errors"
	"golang.org/x/net/context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/structuredmerge/diff"
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
func NewServerSidePatchHelper(ctx context.Context, original, modified client.Object, c client.Client, crdSchemaCache diff.CRDSchemaCache, opts ...HelperOption) (PatchHelper, error) {
	helperOptions := &HelperOptions{}
	helperOptions = helperOptions.ApplyOptions(opts)
	helperOptions.allowedPaths = []contract.Path{
		// apiVersion, kind, name and namespace are required field for a server side apply intent.
		{"apiVersion"},
		{"kind"},
		{"metadata", "name"},
		{"metadata", "namespace"},
		// the topology controller controls/has an opinion for labels, annotation, ownerReferences and spec only.
		{"metadata", "labels"},
		{"metadata", "annotations"},
		{"metadata", "ownerReferences"},
		{"spec"},
	}

	// If required, convert the original and modified objects to unstructured and filter out all the information
	// not relevant for the topology controller.

	var originalUnstructured, previousIntent *unstructured.Unstructured
	if !isNil(original) {
		// Gets the previousIntent from the original object, to be used later for DryRunDiff.
		var err error
		previousIntent, err = diff.GetPreviousIntent(original)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get last applied intent")
		}

		originalUnstructured = &unstructured.Unstructured{}
		switch original.(type) {
		case *unstructured.Unstructured:
			originalUnstructured = original.DeepCopyObject().(*unstructured.Unstructured)
		default:
			if err := c.Scheme().Convert(original, originalUnstructured, nil); err != nil {
				return nil, errors.Wrap(err, "failed to convert original object to Unstructured")
			}
		}

		// If the object has been created with previous custom approach for tracking managed fields, cleanup the object.
		if _, ok := original.GetAnnotations()[clusterv1.ClusterTopologyManagedFieldsAnnotation]; ok {
			if err := cleanupLegacyManagedFields(originalUnstructured, c); err != nil {
				return nil, errors.Wrap(err, "failed to cleanup legacy managed fields from original object")
			}
		}

		filterObject(originalUnstructured, helperOptions)
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
	filterObject(modifiedUnstructured, helperOptions)

	// Determine if the intent defined in the modified object is going to trigger
	// an actual change when running server side apply, and if this change might impact the object spec or not.
	var hasChanges, hasSpecChanges bool
	switch {
	case isNil(original):
		hasChanges, hasSpecChanges = true, true
	default:
		// Gets the schema for the modified object gvk.
		// NOTE: this schema drives DryRunDiff operations; modified (current intent) and original (current object)
		// are of the same gvk, given that we are always calling UpdateReferenceAPIContract when reading both of them.
		// previousIntent instead could be of an older version, but this impacts dryRun only partially (see diff.isDroppingAnyIntent for more details)
		schema, err := crdSchemaCache.LoadOrStore(ctx, modifiedUnstructured.GroupVersionKind())
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get schema for %s", modifiedUnstructured.GroupVersionKind().String())
		}

		hasChanges, hasSpecChanges = diff.DryRunDiff(&diff.DryRunDiffInput{
			PreviousIntent: previousIntent,
			CurrentIntent:  modifiedUnstructured,
			CurrentObject:  originalUnstructured,
			Schema:         schema,
		})
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

	// Stores che current intent as last applied intent.
	// NOTE: we are doing this at this stage so it won't impact the dryRunDiff logic.
	if err := diff.AddCurrentIntentAnnotation(h.modified); err != nil {
		return errors.Wrap(err, "failed to add last applied intent annotation")
	}

	options := []client.PatchOption{
		client.FieldOwner(TopologyManagerName),
		// NOTE: we are using force ownership so in case of conflicts the topology controller
		// overwrite values and become sole manager.
		client.ForceOwnership,
	}
	return h.client.Patch(ctx, h.modified, client.Apply, options...)
}

// cleanupLegacyManagedFields cleanups managed field management in place before introducing SSA.
// NOTE: this operation can trigger a machine rollout, but this is considered acceptable given that ClusterClass is still alpha
// and SSA adoption align the topology controller with K8s recommended solution for many controllers authoring the same object.
func cleanupLegacyManagedFields(obj *unstructured.Unstructured, c client.Client) error {
	base := obj.DeepCopyObject().(*unstructured.Unstructured)

	// Remove the topology.cluster.x-k8s.io/managed-field-paths annotation
	annotations := obj.GetAnnotations()
	delete(annotations, clusterv1.ClusterTopologyManagedFieldsAnnotation)
	obj.SetAnnotations(annotations)

	// Remove managedFieldEntry for manager=manager and operation=update to prevent having two managers holding values set by the topology controller.
	originalManagedFields := obj.GetManagedFields()
	managedFields := make([]metav1.ManagedFieldsEntry, 0, len(originalManagedFields))
	for i := range originalManagedFields {
		if originalManagedFields[i].Manager == "manager" &&
			originalManagedFields[i].Operation == metav1.ManagedFieldsOperationUpdate {
			continue
		}
		managedFields = append(managedFields, originalManagedFields[i])
	}

	// Add a seeding managedFieldEntry for SSA executed by the management controller, to prevent SSA to create/infer
	// a default managedFieldEntry when the first SSA is applied.
	// More specifically, if an existing object doesn't have managedFields when applying the first SSA the API server
	// creates an entry with operation=Update (kind of guessing where the object comes from), but this entry ends up
	// acting as a co-ownership and we want to prevent this.
	// NOTE: fieldV1Map cannot be empty, so we add metadata.name which will be cleaned up at the first SSA patch.
	fieldV1Map := map[string]interface{}{
		"f:metadata": map[string]interface{}{
			"f:name": map[string]interface{}{},
		},
	}
	fieldV1, err := json.Marshal(fieldV1Map)
	if err != nil {
		return errors.Wrap(err, "failed to create seeding fieldV1Map for cleaning up legacy managed fields")
	}
	now := metav1.Now()
	managedFields = append(managedFields, metav1.ManagedFieldsEntry{
		Manager:    TopologyManagerName,
		Operation:  metav1.ManagedFieldsOperationApply,
		APIVersion: obj.GetAPIVersion(),
		Time:       &now,
		FieldsType: "FieldsV1",
		FieldsV1:   &metav1.FieldsV1{Raw: fieldV1},
	})

	obj.SetManagedFields(managedFields)

	return c.Patch(context.TODO(), obj, client.MergeFrom(base))
}
