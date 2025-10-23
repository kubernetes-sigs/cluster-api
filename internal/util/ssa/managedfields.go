/*
Copyright 2023 The Kubernetes Authors.

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

// Package ssa contains utils related to Server-Side-Apply.
package ssa

import (
	"context"
	"encoding/json"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/contract"
)

const classicManager = "manager"

// RemoveManagedFieldsForLabelsAndAnnotations removes labels and annotations from managedFields entries.
// This func is e.g. used in the following context:
// * First a full object is created with fieldManager.
// * Then this func is called to drop ownership for labels and annotations
// * And then in a subsequent syncMachines call a metadataFieldManager can take ownership for labels and annotations.
func RemoveManagedFieldsForLabelsAndAnnotations(ctx context.Context, c client.Client, apiReader client.Reader, object client.Object, fieldManager string) error {
	objectKey := client.ObjectKeyFromObject(object)
	objectGVK, err := apiutil.GVKForObject(object, c.Scheme())
	if err != nil {
		return errors.Wrapf(err, "failed to remove managedFields for labels and annotations from object %s",
			klog.KRef(objectKey.Namespace, objectKey.Name))
	}

	var getObjectFromAPIServer bool
	err = retry.RetryOnConflict(wait.Backoff{
		Steps:    3,
		Duration: 10 * time.Millisecond,
		Factor:   1.0,
		Jitter:   0.1,
	}, func() error {
		// First try is with the passed in object. After that get the object from the apiserver.
		// Note: This is done so that this func does not rely on managedFields being stored in the cache, so we can optimize
		// memory usage by dropping managedFields before storing objects in the cache.
		if getObjectFromAPIServer {
			if err := apiReader.Get(ctx, objectKey, object); err != nil {
				return err
			}
		} else {
			getObjectFromAPIServer = true
		}

		base := object.DeepCopyObject().(client.Object)

		// Modify managedFields for manager=fieldManager and operation=Apply to drop ownership for labels and annotations.
		originalManagedFields := object.GetManagedFields()
		managedFields := make([]metav1.ManagedFieldsEntry, 0, len(originalManagedFields))
		for _, managedField := range originalManagedFields {
			if managedField.Manager == fieldManager &&
				managedField.Operation == metav1.ManagedFieldsOperationApply &&
				managedField.Subresource == "" {
				// Unmarshal the managed fields into a map[string]interface{}
				fieldsV1 := map[string]interface{}{}
				if err := json.Unmarshal(managedField.FieldsV1.Raw, &fieldsV1); err != nil {
					return errors.Wrap(err, "failed to unmarshal managed fields")
				}

				// Filter out the ownership for labels and annotations.
				deletedFields := FilterIntent(&FilterIntentInput{
					Path:  contract.Path{},
					Value: fieldsV1,
					ShouldFilter: IsPathIgnored([]contract.Path{
						{"f:metadata", "f:annotations"},
						{"f:metadata", "f:labels"},
					}),
				})
				if !deletedFields {
					// If fieldManager did not own labels and annotations there's nothing to do.
					return nil
				}

				fieldsV1Raw, err := json.Marshal(fieldsV1)
				if err != nil {
					return errors.Wrap(err, "failed to marshal managed fields")
				}
				managedField.FieldsV1.Raw = fieldsV1Raw

				managedFields = append(managedFields, managedField)
			} else {
				// Do not modify the entry. Use as is.
				managedFields = append(managedFields, managedField)
			}
		}
		object.SetManagedFields(managedFields)

		// Use optimistic locking to avoid accidentally rolling back managedFields.
		return c.Patch(ctx, object, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}))
	})
	if err != nil {
		return errors.Wrapf(err, "failed to remove managedFields for labels and annotations from %s %s",
			objectGVK.Kind, klog.KRef(objectKey.Namespace, objectKey.Name))
	}
	return nil
}

// MigrateManagedFields migrates managedFields.
// ManagedFields are only migrated if fieldManager owns labels+annotations
// The migration deletes all non-status managedField entries for fieldManager:Apply and manager:Update.
// Note: We have to call this func for every Machine created with CAPI <= v1.11 once.
// Given that this was introduced in CAPI v1.12 and our n-3 upgrade policy this can
// be removed with CAPI v1.15.
func MigrateManagedFields(ctx context.Context, c client.Client, object client.Object, fieldManager, metadataFieldManager string) error {
	objectKey := client.ObjectKeyFromObject(object)
	objectGVK, err := apiutil.GVKForObject(object, c.Scheme())
	if err != nil {
		return errors.Wrapf(err, "failed to migrate managedFields for object %s",
			klog.KRef(objectKey.Namespace, objectKey.Name))
	}

	// Check if a migration is still needed. This should be only done once per object.
	needsMigration, err := needsMigration(object, fieldManager)
	if err != nil {
		return errors.Wrapf(err, "failed to migrate managedFields for %s %s",
			objectGVK.Kind, klog.KRef(objectKey.Namespace, objectKey.Name))
	}
	if !needsMigration {
		return nil
	}

	base := object.DeepCopyObject().(client.Object)

	// Remove managedFields for fieldManager:Apply and manager:Update.
	originalManagedFields := object.GetManagedFields()
	managedFields := make([]metav1.ManagedFieldsEntry, 0, len(originalManagedFields))
	for _, managedField := range originalManagedFields {
		if managedField.Manager == fieldManager &&
			managedField.Operation == metav1.ManagedFieldsOperationApply &&
			managedField.Subresource == "" {
			continue
		}
		if managedField.Manager == classicManager &&
			managedField.Operation == metav1.ManagedFieldsOperationUpdate &&
			managedField.Subresource == "" {
			continue
		}
		managedFields = append(managedFields, managedField)
	}

	// Add a seeding managedField entry to prevent SSA to create/infer a default managedField entry when the
	// first SSA is applied.
	// If an existing object doesn't have managedFields when applying the first SSA the API server
	// creates an entry with operation=Update (guessing where the object comes from), but this entry ends up
	// acting as a co-ownership and we want to prevent this.
	// NOTE: fieldV1Map cannot be empty, so we add metadata.name which will be cleaned up at the first
	//       SSA patch of the same manager (updateLabelsAndAnnotations directly after calling this func).
	managedFields = append(managedFields, metav1.ManagedFieldsEntry{
		Manager:    metadataFieldManager,
		Operation:  metav1.ManagedFieldsOperationApply,
		APIVersion: objectGVK.GroupVersion().String(),
		Time:       ptr.To(metav1.Now()),
		FieldsType: "FieldsV1",
		FieldsV1:   &metav1.FieldsV1{Raw: []byte(`{"f:metadata":{"f:name":{}}}`)},
	})

	object.SetManagedFields(managedFields)

	// Use optimistic locking to avoid accidentally rolling back managedFields.
	if err := c.Patch(ctx, object, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
		return errors.Wrapf(err, "failed to migrate managedFields for %s %s",
			objectGVK.Kind, klog.KRef(objectKey.Namespace, objectKey.Name))
	}
	return nil
}

// needsMigration returns true if fieldManager:Apply owns the clusterv1.ClusterNameLabel.
func needsMigration(object client.Object, fieldManager string) (bool, error) {
	for _, managedField := range object.GetManagedFields() {
		//nolint:gocritic // much easier to read this way, not going to use continue instead
		if managedField.Manager == fieldManager &&
			managedField.Operation == metav1.ManagedFieldsOperationApply &&
			managedField.Subresource == "" {
			// Unmarshal the managed fields into a map[string]interface{}
			fieldsV1 := map[string]interface{}{}
			if err := json.Unmarshal(managedField.FieldsV1.Raw, &fieldsV1); err != nil {
				return false, errors.Wrap(err, "failed to determine if migration is needed: failed to unmarshal managed fields")
			}

			// Note: MigrateManagedFields is only called for BootstrapConfig/InfraMachine and both always have the cluster-name label set.
			_, containsClusterNameLabel, err := unstructured.NestedFieldNoCopy(fieldsV1, "f:metadata", "f:labels", "f:cluster.x-k8s.io/cluster-name")
			if err != nil {
				return false, errors.Wrapf(err, "failed to determine if migration is needed: failed to determine if managed field contains the %s label", clusterv1.ClusterNameLabel)
			}
			return containsClusterNameLabel, nil
		}
	}
	return false, nil
}
