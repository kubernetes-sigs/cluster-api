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
	"fmt"
	"reflect"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/internal/contract"
)

// getTopologyManagedFields returns metadata.managedFields entry tracking
// server side apply operations for the topology controller.
func getTopologyManagedFields(original client.Object) map[string]interface{} {
	r := map[string]interface{}{}

	for _, m := range original.GetManagedFields() {
		if m.Operation == metav1.ManagedFieldsOperationApply &&
			m.Manager == TopologyManagerName &&
			m.APIVersion == original.GetObjectKind().GroupVersionKind().GroupVersion().String() {
			// NOTE: API server ensures this is a valid json.
			err := json.Unmarshal(m.FieldsV1.Raw, &r)
			if err != nil {
				continue
			}
			break
		}
	}
	return r
}

// dryRunPatch determine if the intent defined in the modified object is going to trigger
// an actual change when running server side apply, and if this change might impact the object spec or not.
// NOTE: This func checks if:
// - something previously managed is missing from intent (a field has been deleted from modified)
// - the value for a field previously managed is changing in the intent (a field has been changed in modified)
// - the intent contains something not previously managed (a field has been added to modified).
func dryRunPatch(ctx *dryRunInput) (hasChanges, hasSpecChanges bool) {
	// If the func is processing a modified element of type map
	if modifiedMap, ok := ctx.modified.(map[string]interface{}); ok {
		// NOTE: ignoring the error in case the element wasn't in original.
		originalMap, _ := ctx.original.(map[string]interface{})

		// Process mapType/structType = granular, previously not empty.
		// NOTE: mapType/structType = atomic is managed like scalar values down in the func;
		// a map is atomic when the corresponding FieldV1 doesn't have info for the nested fields.
		if len(ctx.fieldsV1) > 0 {
			// Process all the fields the modified map
			keys := sets.NewString()
			hasChanges, hasSpecChanges = false, false
			for field, fieldValue := range modifiedMap {
				// Skip apiVersion, kind, metadata.name and metadata.namespace which are required field for a
				// server side apply intent but not tracked into metadata.managedFields, otherwise they will be
				// considered as a new field added to modified because not previously managed.
				if len(ctx.path) == 0 && (field == "apiVersion" || field == "kind") {
					continue
				}
				if len(ctx.path) == 1 && ctx.path[0] == "metadata" && (field == "name" || field == "namespace") {
					continue
				}

				keys.Insert(field)

				// If this isn't the root of the object and there are already changes detected, it is possible
				// to skip processing sibling fields.
				if len(ctx.path) > 0 && hasChanges {
					continue
				}

				// Compute the field path.
				fieldPath := ctx.path.Append(field)

				// Get the managed field for this key.
				// NOTE: ignoring the conversion error that could happen when modified has a field not previously managed
				fieldV1, _ := ctx.fieldsV1[fmt.Sprintf("f:%s", field)].(map[string]interface{})

				// Get the original value.
				fieldOriginalValue := originalMap[field]

				// Check for changes in the field value.
				fieldHasChanges, fieldHasSpecChanges := dryRunPatch(&dryRunInput{
					path:     fieldPath,
					fieldsV1: fieldV1,
					modified: fieldValue,
					original: fieldOriginalValue,
				})
				hasChanges = hasChanges || fieldHasChanges
				hasSpecChanges = hasSpecChanges || fieldHasSpecChanges
			}

			// Process all the fields the corresponding managed field to identify fields previously managed being
			// dropped from modified.
			for fieldV1 := range ctx.fieldsV1 {
				// Drops "." as it represent the parent field.
				if fieldV1 == "." {
					continue
				}
				field := strings.TrimPrefix(fieldV1, "f:")
				if !keys.Has(field) {
					fieldPath := ctx.path.Append(field)
					return pathToResult(fieldPath)
				}
			}
			return
		}
	}

	// If the func is processing a modified element of type list
	if modifiedList, ok := ctx.modified.([]interface{}); ok {
		// NOTE: ignoring the error in case the element wasn't in original.
		originalList, _ := ctx.original.([]interface{})

		// Process listType = map/set, previously not empty.
		// NOTE: listType = map/set but previously empty is managed like scalar values down in the func.
		if len(ctx.fieldsV1) != 0 {
			// If the number of items is changed from the previous reconcile it is already clear that
			// something is changed without checking all the items.
			// NOTE: this assumes the root of the object isn't a list, which is true for all the K8s objects.
			if len(modifiedList) != len(ctx.fieldsV1) || len(modifiedList) != len(originalList) {
				return pathToResult(ctx.path)
			}

			// Otherwise, check the item in the list one by one.

			// if the list is a listMap
			if isListMap(ctx.fieldsV1) {
				for itemKeys, itemFieldsV1 := range ctx.fieldsV1 {
					// Get the keys for the current item.
					keys := getFieldV1Keys(itemKeys)

					// Get the corresponding original and modified item.
					modifiedItem := getItemWithKeys(modifiedList, keys)
					originalItem := getItemWithKeys(originalList, keys)

					// Get the managed field for this item.
					// NOTE: ignoring conversion failures because itemFieldsV1 are always of this type.
					fieldV1Map, _ := itemFieldsV1.(map[string]interface{})

					// Check for changes in the item value.
					itemHasChanges, itemHasSpecChanges := dryRunPatch(&dryRunInput{
						path:     ctx.path,
						fieldsV1: fieldV1Map,
						modified: modifiedItem,
						original: originalItem,
					})
					hasChanges = hasChanges || itemHasChanges
					hasSpecChanges = hasSpecChanges || itemHasSpecChanges

					// If there are already changes detected, it is possible to skip processing other items.
					if hasChanges {
						break
					}
				}
				return
			}

			if isListSet(ctx.fieldsV1) {
				s := sets.NewString()
				for v := range ctx.fieldsV1 {
					s.Insert(strings.TrimPrefix(v, "v:"))
				}

				for _, v := range modifiedList {
					// NOTE: ignoring this error because API server ensures the keys in listMap are scalars value.
					vString, _ := v.(string)
					if !s.Has(vString) {
						return pathToResult(ctx.path)
					}
				}
				return
			}
		}

		// NOTE: listType = atomic is managed like scalar values down in the func;
		// a list is atomic when the corresponding FieldV1 doesn't have info for the list items.
	}

	// Otherwise, the func is processing scalar or atomic values.

	// Check if the field has been added (it wasn't managed before).
	// NOTE: This prevents false positive when handling metadata, because it is required to have metadata.name and metadata.namespace
	// in modified, but they are not tracked as managed field.
	notManagedBefore := ctx.fieldsV1 == nil
	if len(ctx.path) == 1 && ctx.path[0] == "metadata" {
		notManagedBefore = false
	}

	// Check if the field value is changed.
	// NOTE: it is required to use reflect.DeepEqual because in case of atomic map or lists the value is not a scalar value.
	valueChanged := !reflect.DeepEqual(ctx.modified, ctx.original)

	if notManagedBefore || valueChanged {
		return pathToResult(ctx.path)
	}
	return false, false
}

type dryRunInput struct {
	// the path of the field being processed.
	path contract.Path
	// fieldsV1 for the current path.
	fieldsV1 map[string]interface{}

	// the original and the modified value for the current path.
	modified interface{}
	original interface{}
}

// pathToResult determine if a change in a path impact the spec.
// We assume there is always a change when this call is called; additionally
// we determine the change impacts spec when the path is the root of the object
// or the path starts with spec.
func pathToResult(p contract.Path) (hasChanges, hasSpecChanges bool) {
	return true, len(p) == 0 || (len(p) > 0 && p[0] == "spec")
}

// getFieldV1Keys returns the keys for a listMap item in metadata.managedFields;
// e.g. given the `"k:{\"field1\":\"id1\"}": {...}` item in a ListMap it returns {field1:id1}.
func getFieldV1Keys(v string) map[string]string {
	keys := map[string]string{}
	keysJSON := strings.TrimPrefix(v, "k:")
	// NOTE: ignoring this error because API server ensures this is a valid yaml.
	_ = json.Unmarshal([]byte(keysJSON), &keys)
	return keys
}

// getItemKeys returns the keys value pairs for an item in the list.
// e.g. given keys {field1:id1} and values `"{field1:id2, foo:foo}"` it returns {field1:id2}.
// NOTE: keys comes for managedFields, while values comes from the actual object.
func getItemKeys(keys map[string]string, values map[string]interface{}) map[string]string {
	keyValues := map[string]string{}
	for k := range keys {
		if v, ok := values[k]; ok {
			// NOTE: API server ensures the keys in listMap are scalars value.
			vString, ok := v.(string)
			if !ok {
				continue
			}
			keyValues[k] = vString
		}
	}
	return keyValues
}

// getItemWithKeys return the item in the list with the given keys or nil if any.
// e.g. given l `"[{field1:id1, foo:foo}, {field1:id2, bar:bar}]"` and keys {field1:id1} it returns {field1:id1, foo:foo}.
func getItemWithKeys(l []interface{}, keys map[string]string) map[string]interface{} {
	for _, i := range l {
		// NOTE: API server ensures the item in a listMap is a map.
		iMap, ok := i.(map[string]interface{})
		if !ok {
			continue
		}
		iKeys := getItemKeys(keys, iMap)
		if reflect.DeepEqual(iKeys, keys) {
			return iMap
		}
	}
	return nil
}

// isListMap returns true if the fieldsV1 value represent a listMap.
// NOTE: a listMap has elements in the form of `"k:{...}": {...}`.
func isListMap(fieldsV1 map[string]interface{}) bool {
	for k := range fieldsV1 {
		if strings.HasPrefix(k, "k:") {
			return true
		}
	}
	return false
}

// isListSet returns true if the fieldsV1 value represent a listSet.
// NOTE: a listMap has elements in the form of `"v:..": {}`.
func isListSet(fieldsV1 map[string]interface{}) bool {
	for k := range fieldsV1 {
		if strings.HasPrefix(k, "v:") {
			return true
		}
	}
	return false
}
