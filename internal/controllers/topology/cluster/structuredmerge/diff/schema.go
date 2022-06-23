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
	"reflect"
)

// CRDSchema provides a simplified view of a CRD's Open API schema.
// Schema is a represented as map of field path, TypeDef.
type CRDSchema map[string]TypeDef

// TypeDef represents a named type in a schema.
// TypeDef contains only the info required for diff processing.
type TypeDef struct {
	Scalar *ScalarDef
	List   *ListDef
	Map    *MapDef
	Struct *StructDef
}

// ScalarDef represent a scalar type.
type ScalarDef struct {
}

// MapType enumerate the type if map.
type MapType string

const (
	// GranularMapType defines a map where each item should be processed.
	GranularMapType = MapType("granular")

	// AtomicMapType defines a map where all the items should be processed at once.
	AtomicMapType = MapType("atomic")
)

// MapDef represent a map type.
type MapDef struct {
	Type MapType
}

// StructType enumerate the type if struct.
type StructType string

const (
	// GranularStructType defines a map where each field should be processed.
	GranularStructType = StructType("granular")

	// AtomicStructType defines a map where all the field should be processed at once.
	AtomicStructType = StructType("atomic")
)

// StructDef represent a struct type.
type StructDef struct {
	Type StructType
}

// ListType enumerate the type if lists.
type ListType string

const (
	// AtomicListType defines a list where all the items should be processed at once.
	AtomicListType = ListType("atomic")

	// ListMapType defines a list where each item should be processed according to ListMapKeys.
	ListMapType = ListType("map")

	// ListSetType defines a list where each item should be processed like in a set of scalar values.
	ListSetType = ListType("set")
)

// ListDef represent a list type.
type ListDef struct {
	Type ListType
	Keys []string
}

// GetSameItem returns the element with the same keys of items from the given list.
func (ld *ListDef) GetSameItem(list []interface{}, item interface{}) map[string]interface{} {
	keys := ld.GetItemKeys(item)
	if keys == nil {
		return nil
	}

	return ld.GetItemWithKeys(list, keys)
}

// GetItemKeys return the keys for a given list item.
func (ld *ListDef) GetItemKeys(item interface{}) map[string]interface{} {
	itemMap, ok := item.(map[string]interface{})
	if !ok {
		return nil
	}

	keys := make(map[string]interface{})
	for _, k := range ld.Keys {
		itemValue, ok := itemMap[k]
		if !ok {
			return nil
		}
		keys[k] = itemValue
	}
	return keys
}

// GetItemWithKeys returns the element with the given keys/values from the list.
func (ld *ListDef) GetItemWithKeys(list []interface{}, keys map[string]interface{}) map[string]interface{} {
	for _, item := range list {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		if reflect.DeepEqual(ld.GetItemKeys(itemMap), keys) {
			return itemMap
		}
	}
	return nil
}
