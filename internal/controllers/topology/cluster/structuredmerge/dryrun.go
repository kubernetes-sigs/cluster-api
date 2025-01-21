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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util/conversion"
)

type dryRunSSAPatchInput struct {
	client client.Client
	// ssaCache caches SSA request to allow us to only run SSA when we actually have to.
	ssaCache ssa.Cache
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
func dryRunSSAPatch(ctx context.Context, dryRunCtx *dryRunSSAPatchInput) (bool, bool, []byte, error) {
	// Compute a request identifier.
	// The identifier is unique for a specific request to ensure we don't have to re-run the request
	// once we found out that it would not produce a diff.
	// The identifier consists of: gvk, namespace, name and resourceVersion of originalUnstructured
	// and a hash of modifiedUnstructured.
	// This ensures that we re-run the request as soon as either original or modified changes.
	requestIdentifier, err := ssa.ComputeRequestIdentifier(dryRunCtx.client.Scheme(), dryRunCtx.originalUnstructured, dryRunCtx.modifiedUnstructured)
	if err != nil {
		return false, false, nil, err
	}

	// Check if we already ran this request before by checking if the cache already contains this identifier.
	// Note: We only add an identifier to the cache if the result of the dry run was no diff.
	if exists := dryRunCtx.ssaCache.Has(requestIdentifier, dryRunCtx.originalUnstructured.GetKind()); exists {
		return false, false, nil, nil
	}

	// For dry run we use the same options as for the intent but with adding metadata.managedFields
	// to ensure that changes to ownership are detected.
	filterObjectInput := &ssa.FilterObjectInput{
		AllowedPaths: append(dryRunCtx.helperOptions.AllowedPaths, []string{"metadata", "managedFields"}),
		IgnorePaths:  dryRunCtx.helperOptions.IgnorePaths,
	}

	// Add TopologyDryRunAnnotation to notify validation webhooks to skip immutability checks.
	if err := unstructured.SetNestedField(dryRunCtx.originalUnstructured.Object, "", "metadata", "annotations", clusterv1.TopologyDryRunAnnotation); err != nil {
		return false, false, nil, errors.Wrap(err, "failed to add topology dry-run annotation to original object")
	}
	if err := unstructured.SetNestedField(dryRunCtx.modifiedUnstructured.Object, "", "metadata", "annotations", clusterv1.TopologyDryRunAnnotation); err != nil {
		return false, false, nil, errors.Wrap(err, "failed to add topology dry-run annotation to modified object")
	}

	// Do a server-side apply dry-run with modifiedUnstructured to get the updated object.
	err = dryRunCtx.client.Patch(ctx, dryRunCtx.modifiedUnstructured, client.Apply, client.DryRunAll, client.FieldOwner(TopologyManagerName), client.ForceOwnership)
	if err != nil {
		// This catches errors like metadata.uid changes.
		return false, false, nil, errors.Wrap(err, "server side apply dry-run failed for modified object")
	}

	// Do a server-side apply dry-run with originalUnstructured to ensure the latest defaulting is applied.
	// Note: After a Cluster API upgrade there is no guarantee that defaulting has been run on existing objects.
	// We have to ensure defaulting has been applied before comparing original with modified, because otherwise
	// differences in defaulting would trigger rollouts.
	// Note: We cannot use the managed fields of originalUnstructured after SSA dryrun, because applying
	// the whole originalUnstructured will give capi-topology ownership of all fields. Thus, we back up the
	// managed fields and restore them after the dry run.
	// It's fine to compare the managed fields of modifiedUnstructured after dry-run with originalUnstructured
	// before dry-run as we want to know if applying modifiedUnstructured would change managed fields on original.

	// Filter object to drop fields which are not part of our intent.
	// Note: It's especially important to also drop metadata.resourceVersion, otherwise we could get the following
	// error: "the object has been modified; please apply your changes to the latest version and try again"
	ssa.FilterObject(dryRunCtx.originalUnstructured, filterObjectInput)
	// Backup managed fields.
	originalUnstructuredManagedFieldsBeforeSSA := dryRunCtx.originalUnstructured.GetManagedFields()
	// Set managed fields to nil.
	// Note: Otherwise we would get the following error:
	// "failed to request dry-run server side apply: metadata.managedFields must be nil"
	dryRunCtx.originalUnstructured.SetManagedFields(nil)
	err = dryRunCtx.client.Patch(ctx, dryRunCtx.originalUnstructured, client.Apply, client.DryRunAll, client.FieldOwner(TopologyManagerName), client.ForceOwnership)
	if err != nil {
		return false, false, nil, errors.Wrap(err, "server side apply dry-run failed for original object")
	}
	// Restore managed fields.
	dryRunCtx.originalUnstructured.SetManagedFields(originalUnstructuredManagedFieldsBeforeSSA)

	// Cleanup the dryRunUnstructured object to remove the added TopologyDryRunAnnotation
	// and remove the affected managedFields for `manager=capi-topology` which would
	// otherwise show the additional field ownership for the annotation we added and
	// the changed managedField timestamp.
	// We also drop managedFields of other managers as we don't care about if other managers
	// made changes to the object. We only want to trigger a ServerSideApply if we would make
	// changes to the object.
	// Please note that if other managers made changes to fields that we care about and thus ownership changed,
	// this would affect our managed fields as well and we would still detect it by diffing our managed fields.
	if err := cleanupManagedFieldsAndAnnotation(dryRunCtx.modifiedUnstructured); err != nil {
		return false, false, nil, errors.Wrap(err, "failed to filter topology dry-run annotation on modified object")
	}

	// Also run the function for the originalUnstructured to remove the managedField
	// timestamp for `manager=capi-topology`.
	// We also drop managedFields of other managers as we don't care about if other managers
	// made changes to the object. We only want to trigger a ServerSideApply if we would make
	// changes to the object.
	// Please note that if other managers made changes to fields that we care about and thus ownership changed,
	// this would affect our managed fields as well and we would still detect it by diffing our managed fields.
	if err := cleanupManagedFieldsAndAnnotation(dryRunCtx.originalUnstructured); err != nil {
		return false, false, nil, errors.Wrap(err, "failed to filter topology dry-run annotation on original object")
	}

	// Drop the other fields which are not part of our intent.
	ssa.FilterObject(dryRunCtx.modifiedUnstructured, filterObjectInput)
	ssa.FilterObject(dryRunCtx.originalUnstructured, filterObjectInput)

	// Compare the output of dry run to the original object.
	originalJSON, err := json.Marshal(dryRunCtx.originalUnstructured)
	if err != nil {
		return false, false, nil, err
	}
	modifiedJSON, err := json.Marshal(dryRunCtx.modifiedUnstructured)
	if err != nil {
		return false, false, nil, err
	}

	rawDiff, err := jsonpatch.CreateMergePatch(originalJSON, modifiedJSON)
	if err != nil {
		return false, false, nil, err
	}

	// Determine if there are changes to the spec and object.
	diff := &unstructured.Unstructured{}
	if err := json.Unmarshal(rawDiff, &diff.Object); err != nil {
		return false, false, nil, err
	}

	hasChanges := len(diff.Object) > 0
	_, hasSpecChanges := diff.Object["spec"]

	var changes []byte
	if hasChanges {
		// Cleanup diff by dropping .metadata.managedFields.
		ssa.FilterIntent(&ssa.FilterIntentInput{
			Path:         contract.Path{},
			Value:        diff.Object,
			ShouldFilter: ssa.IsPathIgnored([]contract.Path{[]string{"metadata", "managedFields"}}),
		})

		// changes should be empty (not "{}") if diff.Object is empty
		if len(diff.Object) != 0 {
			changes, err = json.Marshal(diff.Object)
			if err != nil {
				return false, false, nil, errors.Wrapf(err, "failed to marshal diff")
			}
		}
	}

	// If there is no diff add the request identifier to the cache.
	if !hasChanges {
		dryRunCtx.ssaCache.Add(requestIdentifier)
	}

	return hasChanges, hasSpecChanges, changes, nil
}

// cleanupManagedFieldsAndAnnotation adjusts the obj to remove the topology.cluster.x-k8s.io/dry-run
// and cluster.x-k8s.io/conversion-data annotations as well as the field ownership reference in managedFields. It does
// also remove the timestamp of the managedField for `manager=capi-topology` because
// it is expected to change due to the additional annotation.
func cleanupManagedFieldsAndAnnotation(obj *unstructured.Unstructured) error {
	// Filter the topology.cluster.x-k8s.io/dry-run annotation as well as leftover empty maps.
	ssa.FilterIntent(&ssa.FilterIntentInput{
		Path:  contract.Path{},
		Value: obj.Object,
		ShouldFilter: ssa.IsPathIgnored([]contract.Path{
			{"metadata", "annotations", clusterv1.TopologyDryRunAnnotation},
			// In case the ClusterClass we are reconciling is using not the latest apiVersion the conversion
			// annotation might be added to objects. As we don't care about differences in conversion as we
			// are working on the old apiVersion we want to ignore the annotation when diffing.
			{"metadata", "annotations", conversion.DataAnnotation},
		}),
	})

	// Adjust the managed field for Manager=TopologyManagerName, Subresource="", Operation="Apply" and
	// drop managed fields of other controllers.
	oldManagedFields := obj.GetManagedFields()
	newManagedFields := []metav1.ManagedFieldsEntry{}
	for _, managedField := range oldManagedFields {
		if managedField.Manager != TopologyManagerName {
			continue
		}
		if managedField.Subresource != "" {
			continue
		}
		if managedField.Operation != metav1.ManagedFieldsOperationApply {
			continue
		}

		// Unset the managedField timestamp because managedFields are treated as atomic map.
		managedField.Time = nil

		// Unmarshal the managed fields into a map[string]interface{}
		fieldsV1 := map[string]interface{}{}
		if err := json.Unmarshal(managedField.FieldsV1.Raw, &fieldsV1); err != nil {
			return errors.Wrap(err, "failed to unmarshal managed fields")
		}

		// Filter out the annotation ownership as well as leftover empty maps.
		ssa.FilterIntent(&ssa.FilterIntentInput{
			Path:  contract.Path{},
			Value: fieldsV1,
			ShouldFilter: ssa.IsPathIgnored([]contract.Path{
				{"f:metadata", "f:annotations", "f:" + clusterv1.TopologyDryRunAnnotation},
				{"f:metadata", "f:annotations", "f:" + conversion.DataAnnotation},
			}),
		})

		fieldsV1Raw, err := json.Marshal(fieldsV1)
		if err != nil {
			return errors.Wrap(err, "failed to marshal managed fields")
		}
		managedField.FieldsV1.Raw = fieldsV1Raw

		newManagedFields = append(newManagedFields, managedField)
	}

	obj.SetManagedFields(newManagedFields)

	return nil
}
