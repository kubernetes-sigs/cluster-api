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

package diff

import (
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"

	"sigs.k8s.io/cluster-api/internal/contract"
)

// NOTE: We can assume that current intent and current object always have the same gvk,
// given that we are always calling UpdateReferenceAPIContract when reading both of them.

// DryRunDiff determine if the current intent is going to determine changes in the object.
func DryRunDiff(in *DryRunDiffInput) (hasChanges, hasSpecChanges bool) {
	// Check if the current intent is going to actually change something in the current object
	hasChanges, hasSpecChanges = isChangingAnything(&isChangingAnythingInput{
		path:          contract.Path{},
		currentIntent: in.CurrentIntent.Object,
		currentObject: in.CurrentObject.Object,
		schema:        in.Schema,
	})

	// If we already found a diff, we can return here without processing following checks.
	if hasChanges && hasSpecChanges {
		return hasChanges, hasSpecChanges
	}

	// compares previous intent and current intent to determine if the topology
	// controller stopped to express opinion on some fields.
	hasDeletion, hasSpecDeletion := isDroppingAnyIntent(&isDroppingAnyIntentInput{
		path:           contract.Path{},
		previousIntent: in.PreviousIntent.Object,
		currentIntent:  in.CurrentIntent.Object,
		currentObject:  in.CurrentObject.Object,
		schema:         in.Schema,
	})
	hasChanges = hasChanges || hasDeletion
	hasSpecChanges = hasSpecChanges || hasSpecDeletion

	return hasChanges, hasSpecChanges
}

// DryRunDiffInput is the input of the DryRunDiff func.
type DryRunDiffInput struct {
	// the previous intent, that is the values the topology controller enforced at the previous reconcile.
	PreviousIntent *unstructured.Unstructured
	// the current intent, that is the values the topology controller is enforcing at this reconcile.
	CurrentIntent *unstructured.Unstructured

	// the current object read at the beginning of the reconcile process.
	CurrentObject *unstructured.Unstructured

	Schema CRDSchema
}

// isChangingAnything checks if the current intent is going to actually change something in the current object.
// NOTE: We can assume that current intent and current object always have the same gvk,
// given that we are always calling UpdateReferenceAPIContract when reading both of them.
func isChangingAnything(in *isChangingAnythingInput) (hasChanges, hasSpecChanges bool) {
	typeDef, ok := in.schema[in.path.String()]
	if !ok {
		// TODO: log that the operation is done assuming atomic value.
	}

	if typeDef.Struct != nil || typeDef.Map != nil {
		if (typeDef.Struct != nil && typeDef.Struct.Type == GranularStructType) || (typeDef.Map != nil && typeDef.Map.Type == GranularMapType) {
			currentIntentMap, ok := in.currentIntent.(map[string]interface{})
			if !ok {
				// NOTE: this condition should never happen because this func is called while walking through values in the current intent.
				// However, for extra caution, it is handled nevertheless.
				return hasChanges, hasSpecChanges
			}

			// Get the corresponding struct/map in the current object.
			// If the corresponding struct/map does not exist in the current object, it means it is being added by the intent (it is a change).
			currentObjectMap, ok := in.currentObject.(map[string]interface{})
			if !ok {
				return pathToResult(in.path)
			}

			// For each field in the current intent.
			for field, currentIntentValue := range currentIntentMap {
				// Compute the path to get access to the field schema; in case of struct, it is defined by the current path + the field name.
				// For maps instead every item has the same schema defined at current path + [].
				fieldPath := in.path.Append(field)
				if typeDef.Map != nil {
					fieldPath = in.path.Item()
				}

				// Get the corresponding value in the current object.
				// If the corresponding value does not exist in the current object, it means it is being added by the intent (it is a change).
				currentObjectValue, ok := currentObjectMap[field]
				if !ok {
					fieldHasChanges, fieldHasSpecChanges := pathToResult(fieldPath)
					hasChanges = hasChanges || fieldHasChanges
					hasSpecChanges = hasSpecChanges || fieldHasSpecChanges

					// If we already detected a spec change, no need to look further, otherwise
					// we should keep going to process remaining entries in the struct/map.
					if hasSpecChanges {
						return hasChanges, hasSpecChanges
					}
					continue
				}

				// Otherwise, if there is a corresponding value, check for changes recursively.
				fieldHasChanges, fieldHasSpecChanges := isChangingAnything(&isChangingAnythingInput{
					path:          fieldPath,
					currentIntent: currentIntentValue,
					currentObject: currentObjectValue,
					schema:        in.schema,
				})
				hasChanges = hasChanges || fieldHasChanges
				hasSpecChanges = hasSpecChanges || fieldHasSpecChanges

				// If we already detected a spec change, no need to look further, otherwise
				// we should keep going to process remaining entries in the struct/map.
				if hasSpecChanges {
					return hasChanges, hasSpecChanges
				}
			}

			return hasChanges, hasSpecChanges
		}

		// NOTE: struct/maps of Type atomic are treated below.
	}

	if typeDef.List != nil {
		// Get the corresponding list in the current intent.
		currentIntentList, ok := in.currentIntent.([]interface{})
		if !ok {
			// NOTE: this condition should never happen, because entire list deletion is caught when processing fields in a map, and the list is always a field in a map.
			// However, for extra caution, it is handled nevertheless.
			return hasChanges, hasSpecChanges
		}

		// Get the corresponding list in the current object.
		// If the corresponding list does not exist in the current object, it means it is being added by the intent (it is a change).
		currentObjectList, ok := in.currentObject.([]interface{})
		if !ok {
			return pathToResult(in.path)
		}

		if typeDef.List.Type == ListMapType {
			// If it is ListMap, we know that the list includes only map items and those items must have a set of unique listMapKeys values.
			// Accordingly, we detect changes by looking for items (maps identified by a listMapKeys values) both in current object and current intent and checking for differences.
			// e.g. previous intent [{"id": "1", "foo": "foo", ...}, {...}], current intent [{"id": "1", "foo": "foo-changed", ...}, {...}], item with key "id": "1" has been changed.

			// Check if items in the current intent have changes from the current object.
			for _, currentIntentItem := range currentIntentList {
				// Get the corresponding item from the current object, which is the item with the same ListMap keys values.
				// If the corresponding item does not exist in the current object, it means it is being added by the intent (it is a change).
				currentObjectItem := typeDef.List.GetSameItem(currentObjectList, currentIntentItem)
				if currentObjectItem == nil {
					return pathToResult(in.path)
				}

				// Otherwise, if there is a corresponding item, check for deletion recursively.
				if itemHasChanges, itemHasSpecChanges := isChangingAnything(&isChangingAnythingInput{
					path:          in.path.Item(),
					currentIntent: currentIntentItem,
					currentObject: currentObjectItem,
					schema:        in.schema,
				}); itemHasChanges || itemHasSpecChanges {
					return itemHasChanges, itemHasSpecChanges
				}
			}

			return hasChanges, hasSpecChanges
		}

		if typeDef.List.Type == ListSetType {
			// If it is ListSet, we know that the list includes only scalar items and those items must be unique.
			// Accordingly, we detect changes by looking for items (scalar values) present in the current intent but not in the current object.
			// e.g. previous intent ["a", "b"], current intent ["a"], item "b" has been deleted.

			// Get the corresponding set of list items from the current object.
			currentObjectSet := sets.NewString()
			for _, currentObjectItem := range currentObjectList {
				currentObjectSet.Insert(fmt.Sprintf("%s", currentObjectItem))
			}

			// Check if items in the current intent have changes from the current object;
			// being the List a set of scalar values, we are only concerned about additions.
			for _, currentIntentItem := range currentIntentList {
				// If the corresponding item in the corresponding does not exist in the current object, it means it is being added by the intent (it is a change).
				if !currentObjectSet.Has(fmt.Sprintf("%s", currentIntentItem)) {
					return pathToResult(in.path)
				}
			}

			return hasChanges, hasSpecChanges
		}

		// NOTE: lists of Type atomic are treated below.
	}

	// Otherwise we consider the field a Scalar or an atomic value.

	// Compare current intent and current object, if they are different it means it is being modified by the intent (it is a change).
	// NOTE: we use reflect.DeepEqual because the value can be a nested struct.
	if !reflect.DeepEqual(in.currentIntent, in.currentObject) {
		return pathToResult(in.path)
	}

	return hasChanges, hasSpecChanges
}

// isChangingAnythingInput is the input of the isChangingAnything func.
type isChangingAnythingInput struct {
	path contract.Path

	currentIntent interface{}
	currentObject interface{}

	schema CRDSchema
}

// isDroppingAnyIntent compares previous intent and current intent to determine if the topology
// controller stopped to express opinion on some fields; as further optimization,
// detected deletion from the intent are ignored in case the corresponding value
// has been already removed from the current object.
// NOTE: We can assume that current intent and current object always have the same gvk,
// given that we are always calling UpdateReferenceAPIContract when reading both of them.
// NOTE: Previous intent instead could have an older gvk, and in that case we look for dropped
// intent at best effort, using the latest gvk schema.
// A possible improvements will be for the providers to convert the content of the previous intent
// while converting the object to a newer version.
func isDroppingAnyIntent(in *isDroppingAnyIntentInput) (hasDeletion, hasSpecDeletion bool) {
	typeDef, ok := in.schema[in.path.String()]
	if !ok {
		// TODO: log that the operation is done assuming atomic value.
	}

	if typeDef.Struct != nil || typeDef.Map != nil {
		if (typeDef.Struct != nil && typeDef.Struct.Type == GranularStructType) || (typeDef.Map != nil && typeDef.Map.Type == GranularMapType) {
			// Get the corresponding struct/map in the current object.
			// NOTE: The conversion error is ignored because the struct/map could have been already removed from the current object.
			currentObjectStruct, _ := in.currentObject.(map[string]interface{})

			// Get the struct/map from the previous intent.
			previousIntentStruct, ok := in.previousIntent.(map[string]interface{})
			if !ok {
				// NOTE: this condition should never happen because this func is called while walking through values in the previous intent.
				// However, for extra caution, it is handled nevertheless.
				return hasDeletion, hasSpecDeletion
			}

			// Get the corresponding struct/map from the current intent.
			currentIntentStruct, ok := in.currentIntent.(map[string]interface{})
			if !ok {
				// NOTE: this condition should never happen, because entire struct/map deletion is caught when processing fields in a struct, and the struct/map is always a field in a parent struct.
				// However, for extra caution, it is handled nevertheless.

				// If the struct/map still is in the current object, then there is a deletion the topology controller should reconcile.
				if currentObjectStruct != nil {
					return pathToResult(in.path)
				}
				return hasDeletion, hasSpecDeletion
			}

			// For each field in the previous intent.
			for field, previousIntentValue := range previousIntentStruct {
				// Compute the path to get access to the field schema; in case of struct, it is defined by the current path + the field name.
				// For maps instead every item has the same schema defined at current path + [].
				fieldPath := in.path.Append(field)
				if typeDef.Map != nil {
					fieldPath = in.path.Item()
				}

				// Get the corresponding value in the current object.
				// NOTE: The item could have been already removed from the current object.
				currentObjectValue := currentObjectStruct[field]

				// Get the corresponding value in the current intent; if there is no corresponding value,
				// the field has been deleted from the current intent.
				currentIntentValue, ok := currentIntentStruct[field]
				if !ok {
					if currentObjectValue != nil {
						fieldHasDeletion, fieldHasSpecDeletion := pathToResult(fieldPath)
						hasDeletion = hasDeletion || fieldHasDeletion
						hasSpecDeletion = hasSpecDeletion || fieldHasSpecDeletion

						// If we already detected a spec deletion, no need to look further, otherwise
						// we should keep going to process remaining entries in the struct/map.
						if hasSpecDeletion {
							return hasDeletion, hasSpecDeletion
						}
					}
					continue
				}

				// Otherwise, there is a corresponding value in the current intent, then check for deletion recursively.
				fieldHasDeletion, fieldHasSpecDeletion := isDroppingAnyIntent(&isDroppingAnyIntentInput{
					path:           fieldPath,
					previousIntent: previousIntentValue,
					currentIntent:  currentIntentValue,
					currentObject:  currentObjectValue,
					schema:         in.schema,
				})
				hasDeletion = hasDeletion || fieldHasDeletion
				hasSpecDeletion = hasSpecDeletion || fieldHasSpecDeletion

				// If we already detected a spec deletion, no need to look further, otherwise
				// we should keep going to process remaining entries in the struct/map.
				if hasSpecDeletion {
					return hasDeletion, hasSpecDeletion
				}
			}

			return hasDeletion, hasSpecDeletion
		}

		// NOTE: struct of Type atomic are treated below.
	}

	if typeDef.List != nil {
		// Get the corresponding list in the current object.
		// NOTE: The conversion error is ignored because the map could have been already removed from the current object.
		currentObjectList, _ := in.currentObject.([]interface{})

		// Get the corresponding list in the current intent; if there is no corresponding list,
		// the list has been deleted from the current intent.
		currentIntentList, ok := in.currentIntent.([]interface{})
		if !ok {
			// NOTE: this condition should never happen, because entire list deletion is caught when processing fields in a map, and the list is always a field in a map.
			// However, for extra caution, it is handled nevertheless.

			// If the list still is in the current object, then there is a deletion the topology controller should reconcile.
			if currentObjectList != nil {
				return pathToResult(in.path)
			}
			return hasDeletion, hasSpecDeletion
		}

		// Get the corresponding value in the current intent; if there is no corresponding value,
		previousIntentList, ok := in.previousIntent.([]interface{})
		if !ok {
			// NOTE: this condition should never happen because this func is called while walking through values in the previous intent.
			// However, for extra caution, it is handled nevertheless.
			return hasDeletion, hasSpecDeletion
		}

		if typeDef.List.Type == ListMapType {
			// If it is ListMap, we know that the list includes only map items and those items must have a set of unique listMapKeys values.
			// Accordingly, we detect deletions by looking for items (maps identified by a listMapKeys values) present in the previous intent but not in the current intent.
			// e.g. previous intent [{"id": "1", ...}, {"id": "2", ...}], current intent [{"id": "1", ...}], item with key "id": "2" has been deleted.
			// If instead an item exists both in previousIntent and current intent, we look for deletions recursively inside the item.

			// Check if items in the previous intent has been removed from the current intent.
			for _, previousIntentItem := range previousIntentList {
				// Get the corresponding item from the current object, which is the item with the same ListMap keys values.
				currentObjectItem := typeDef.List.GetSameItem(currentObjectList, previousIntentItem)

				// Get the corresponding item from the current intent, which is the item with the same ListMap keys values.
				currentIntentItem := typeDef.List.GetSameItem(currentIntentList, previousIntentItem)

				// If there is no corresponding value in the current intent, the item has been deleted.
				if currentIntentItem == nil {
					// If the list item still is in the current object, then there is a deletion the topology controller should reconcile,
					// otherwise we can continue processing remaining items.
					if currentObjectItem != nil {
						return pathToResult(in.path)
					}
					continue
				}

				// Otherwise, if there is a corresponding item, check for deletion recursively.
				if itemHasDeletion, itemHasSpecDeletion := isDroppingAnyIntent(&isDroppingAnyIntentInput{
					path:           in.path.Item(),
					previousIntent: previousIntentItem,
					currentIntent:  currentIntentItem,
					currentObject:  currentObjectItem,
					schema:         in.schema,
				}); itemHasDeletion || itemHasSpecDeletion {
					return itemHasDeletion, itemHasSpecDeletion
				}
			}

			return hasDeletion, hasSpecDeletion
		}

		if typeDef.List.Type == ListSetType {
			// If it is ListSet, we know that the list includes only scalar items and those items must be unique.
			// Accordingly, we detect deletions by looking for items (scalar values) present in the previous intent but not in the current intent.
			// e.g. previous intent ["a", "b"], current intent ["a"], item "b" has been deleted.

			// Get the corresponding set of list items from the current object.
			currentObjectSet := sets.NewString()
			for _, currentObjectItem := range currentObjectList {
				currentObjectSet.Insert(fmt.Sprintf("%s", currentObjectItem))
			}

			// Get the corresponding set of list items from the current intent.
			currentIntentSet := sets.NewString()
			for _, currentIntentItem := range currentIntentList {
				currentIntentSet.Insert(fmt.Sprintf("%s", currentIntentItem))
			}

			// Check if items in the previous intent has been removed from the current intent.
			for _, previousIntentItem := range previousIntentList {
				// if there is no corresponding item in the current intent, the item has been deleted.
				if !currentIntentSet.Has(fmt.Sprintf("%s", previousIntentItem)) {
					// If the item still is in the current object, then there is a deletion the topology controller should reconcile,
					// otherwise we can continue processing the remaining items.
					if currentObjectSet.Has(fmt.Sprintf("%s", previousIntentItem)) {
						return pathToResult(in.path)
					}
					continue
				}
			}

			return hasDeletion, hasSpecDeletion
		}

		// NOTE: lists of Type atomic are treated below.
	}

	// Otherwise we consider the field a Scalar or an atomic value.

	// If there is no corresponding field in the current intent, and it still is in the current object,
	// then there is a deletion the topology controller should reconcile.
	if in.currentIntent == nil && in.currentObject != nil {
		return pathToResult(in.path)
	}

	return hasDeletion, hasSpecDeletion
}

// isDroppingAnyIntentInput is the input of the isDroppingAnyIntent func.
type isDroppingAnyIntentInput struct {
	// the path of the field being processed.
	path contract.Path

	// the previous intent, that is the values the topology controller enforced at the previous reconcile.
	previousIntent interface{}
	// the current intent, that is the values the topology controller is enforcing at this reconcile.
	currentIntent interface{}

	// the current object read at the beginning of the reconcile process.
	currentObject interface{}

	schema CRDSchema
}

// pathToResult determine if a change in a path impact the spec.
// We assume there is always a change when this call is called; additionally
// we determine the change impacts spec when the path is the root of the object
// or the path starts with spec.
func pathToResult(p contract.Path) (hasChanges, hasSpecChanges bool) {
	return true, len(p) == 0 || (len(p) > 0 && p[0] == "spec")
}
