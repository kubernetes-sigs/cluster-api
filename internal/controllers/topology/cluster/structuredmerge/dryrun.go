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

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/contract"
)

type dryRunSSAPatchInput struct {
	client client.Client
	// originalUnstructured contains the current state of the object
	originalUnstructured *unstructured.Unstructured
	// dryRunUnstructured contains the intended changes to the object and will be used to
	// compare to the originalUnstructured object afterwards.
	dryRunUnstructured *unstructured.Unstructured
	// helperOptions contains the helper options for filtering the intent.
	helperOptions *HelperOptions
}

// dryRunSSAPatch uses server side apply dry run to determine if the operation is going to change the actual object.
func dryRunSSAPatch(ctx context.Context, dryRunCtx *dryRunSSAPatchInput) (bool, bool, error) {
	// For dry run we use the same options as for the intent but with adding metadata.managedFields
	// to ensure that changes to ownership are detected.
	dryRunHelperOptions := &HelperOptions{
		allowedPaths: append(dryRunCtx.helperOptions.allowedPaths, []string{"metadata", "managedFields"}),
		ignorePaths:  dryRunCtx.helperOptions.ignorePaths,
	}

	// Add TopologyDryRunAnnotation to notify validation webhooks to skip immutability checks.
	if err := unstructured.SetNestedField(dryRunCtx.dryRunUnstructured.Object, "", "metadata", "annotations", clusterv1.TopologyDryRunAnnotation); err != nil {
		return false, false, errors.Wrap(err, "failed to add topology dry-run annotation")
	}

	// Do a server-side apply dry-run request to get the updated object.
	err := dryRunCtx.client.Patch(ctx, dryRunCtx.dryRunUnstructured, client.Apply, client.DryRunAll, client.FieldOwner(TopologyManagerName), client.ForceOwnership)
	if err != nil {
		// This catches errors like metadata.uid changes.
		return false, false, errors.Wrap(err, "failed to request dry-run server side apply")
	}

	// Cleanup the dryRunUnstructured object to remove the added TopologyDryRunAnnotation
	// and remove the affected managedFields for `manager=capi-topology` which would
	// otherwise show the additional field ownership for the annotation we added and
	// the changed managedField timestamp.
	// We also drop managedFields of other managers as we don't care about if other managers
	// made changes to the object. We only want to trigger a ServerSideApply if we would make
	// changes to the object.
	// Please note that if other managers made changes to fields that we care about and thus ownership changed,
	// this would affect our managed fields as well and we would still detect it by diffing our managed fields.
	if err := cleanupManagedFieldsAndAnnotation(dryRunCtx.dryRunUnstructured); err != nil {
		return false, false, errors.Wrap(err, "failed to filter topology dry-run annotation on dryRunUnstructured")
	}

	// Also run the function for the originalUnstructured to remove the managedField
	// timestamp for `manager=capi-topology`.
	// We also drop managedFields of other managers as we don't care about if other managers
	// made changes to the object. We only want to trigger a ServerSideApply if we would make
	// changes to the object.
	// Please note that if other managers made changes to fields that we care about and thus ownership changed,
	// this would affect our managed fields as well and we would still detect it by diffing our managed fields.
	if err := cleanupManagedFieldsAndAnnotation(dryRunCtx.originalUnstructured); err != nil {
		return false, false, errors.Wrap(err, "failed to filter topology dry-run annotation on originalUnstructured")
	}

	// Drop the other fields which are not part of our intent.
	filterObject(dryRunCtx.dryRunUnstructured, dryRunHelperOptions)
	filterObject(dryRunCtx.originalUnstructured, dryRunHelperOptions)

	// Compare the output of dry run to the original object.
	originalJSON, err := json.Marshal(dryRunCtx.originalUnstructured)
	if err != nil {
		return false, false, err
	}
	dryRunJSON, err := json.Marshal(dryRunCtx.dryRunUnstructured)
	if err != nil {
		return false, false, err
	}

	rawDiff, err := jsonpatch.CreateMergePatch(originalJSON, dryRunJSON)
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

// cleanupManagedFieldsAndAnnotation adjusts the obj to remove the topology.cluster.x-k8s.io/dry-run
// annotation as well as the field ownership reference in managedFields. It does
// also remove the timestamp of the managedField for `manager=capi-topology` because
// it is expected to change due to the additional annotation.
func cleanupManagedFieldsAndAnnotation(obj *unstructured.Unstructured) error {
	// Filter the topology.cluster.x-k8s.io/dry-run annotation as well as leftover empty maps.
	filterIntent(&filterIntentInput{
		path:  contract.Path{},
		value: obj.Object,
		shouldFilter: isIgnorePath([]contract.Path{
			{"metadata", "annotations", clusterv1.TopologyDryRunAnnotation},
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
		filterIntent(&filterIntentInput{
			path:  contract.Path{},
			value: fieldsV1,
			shouldFilter: isIgnorePath([]contract.Path{
				{"f:metadata", "f:annotations", "f:" + clusterv1.TopologyDryRunAnnotation},
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
