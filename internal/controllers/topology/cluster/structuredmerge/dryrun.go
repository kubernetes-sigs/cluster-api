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
	"encoding/json"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/util/conversion"
)

type dryRunSSAPatchInput struct {
	client client.Client
	// originalUnstructured contains the current state of the object.
	// Note: We will run SSA dry-run on originalUnstructured and modifiedUnstructured and then compare them.
	originalUnstructured *unstructured.Unstructured
	// modifiedUnstructured contains the intended changes to the object.
	// Note: We will run SSA dry-run on originalUnstructured and modifiedUnstructured and then compare them.
	modifiedUnstructured *unstructured.Unstructured
	// helperOptions contains the helper options for filtering the intent.
	helperOptions *HelperOptions
}

// dryRunSSAPatch uses server side apply dry run to determine if the operation is going to change the actual object.
func dryRunSSAPatch(ctx context.Context, dryRunCtx *dryRunSSAPatchInput) (bool, bool, error) {
	// Add TopologyDryRunAnnotation to notify validation webhooks to skip immutability checks.
	if err := unstructured.SetNestedField(dryRunCtx.originalUnstructured.Object, "", "metadata", "annotations", clusterv1.TopologyDryRunAnnotation); err != nil {
		return false, false, errors.Wrap(err, "failed to add topology dry-run annotation to original object")
	}
	if err := unstructured.SetNestedField(dryRunCtx.modifiedUnstructured.Object, "", "metadata", "annotations", clusterv1.TopologyDryRunAnnotation); err != nil {
		return false, false, errors.Wrap(err, "failed to add topology dry-run annotation to modified object")
	}

	// Do a server-side apply dry-run with originalUnstructured to ensure the latest defaulting is applied.
	// Note: After a Cluster API upgrade there is no guarantee that defaulting has been run on existing objects.
	// We have to ensure defaulting has been applied before comparing original with modified, because otherwise
	// differences in defaulting would trigger rollouts.
	err := dryRunCtx.client.Patch(ctx, dryRunCtx.originalUnstructured, client.Apply, client.DryRunAll, client.FieldOwner(TopologyManagerName), client.ForceOwnership)
	if err != nil {
		// This catches errors like metadata.uid changes.
		return false, false, errors.Wrap(err, "server side apply dry-run failed for original object")
	}
	// Do a server-side apply dry-run with modifiedUnstructured to get the updated object.
	err = dryRunCtx.client.Patch(ctx, dryRunCtx.modifiedUnstructured, client.Apply, client.DryRunAll, client.FieldOwner(TopologyManagerName), client.ForceOwnership)
	if err != nil {
		// This catches errors like metadata.uid changes.
		return false, false, errors.Wrap(err, "server side apply dry-run failed for modified object")
	}

	// Cleanup the originalUnstructured and modifiedUnstructured objects to remove the added TopologyDryRunAnnotation
	// and remove all managedFields.
	// We can't compare managedFields as running dry-run with originalUnstructured will give us ownership of
	// all fields in originalUnstructured, while modifiedUnstructured only has the fields of our intent set.
	// We don't have to trigger a rollout for managed fields as they only reflect ownership of fields, as soon
	// as we actually want to change field values we will do a rollout anyway.
	cleanupManagedFieldsAndAnnotations(dryRunCtx.originalUnstructured)
	cleanupManagedFieldsAndAnnotations(dryRunCtx.modifiedUnstructured)

	// Drop all fields which are not part of our intent.
	filterObject(dryRunCtx.originalUnstructured, dryRunCtx.helperOptions)
	filterObject(dryRunCtx.modifiedUnstructured, dryRunCtx.helperOptions)

	// Compare the output of dry run to the original object.
	originalJSON, err := json.Marshal(dryRunCtx.originalUnstructured)
	if err != nil {
		return false, false, err
	}
	modifiedJSON, err := json.Marshal(dryRunCtx.modifiedUnstructured)
	if err != nil {
		return false, false, err
	}

	rawDiff, err := jsonpatch.CreateMergePatch(originalJSON, modifiedJSON)
	if err != nil {
		return false, false, err
	}

	// Determine if there are changes to the spec and object.
	diff := &unstructured.Unstructured{}
	if err := json.Unmarshal(rawDiff, &diff.Object); err != nil {
		return false, false, err
	}

	hasChanges := len(diff.Object) > 0
	_, hasSpecChanges := diff.Object["spec"]

	return hasChanges, hasSpecChanges, nil
}

// cleanupManagedFieldsAndAnnotations adjusts the obj to remove the topology.cluster.x-k8s.io/dry-run
// and cluster.x-k8s.io/conversion-data annotations as well as all managed fields.
func cleanupManagedFieldsAndAnnotations(obj *unstructured.Unstructured) {
	// Filter the annotations as well as leftover empty maps.
	filterIntent(&filterIntentInput{
		path:  contract.Path{},
		value: obj.Object,
		shouldFilter: isIgnorePath([]contract.Path{
			{"metadata", "annotations", clusterv1.TopologyDryRunAnnotation},
			// In case the ClusterClass we are reconciling is using not the latest apiVersion the conversion
			// annotation might be added to objects. As we don't care about differences in conversion as we
			// are working on the old apiVersion we want to ignore the annotation when diffing.
			{"metadata", "annotations", conversion.DataAnnotation},
		}),
	})

	// Drop managed fields.
	obj.SetManagedFields(nil)
}
