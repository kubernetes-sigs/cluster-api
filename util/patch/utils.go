/*
Copyright 2020 The Kubernetes Authors.

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

package patch

import (
	"reflect"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

type patchType string

const (
	specPatch   patchType = "spec"
	statusPatch patchType = "status"
)

var (
	preserveUnstructuredKeys = map[string]bool{
		"kind":       true,
		"apiVersion": true,
		"metadata":   true,
	}
)

// toUnstructured converts an object to Unstructured.
// We have to pass in a gvk as we can't rely on GVK being set in a runtime.Object.
func toUnstructured(obj runtime.Object, gvk schema.GroupVersionKind) (*unstructured.Unstructured, error) {
	// If the incoming object is already unstructured, perform a deep copy first
	// otherwise DefaultUnstructuredConverter ends up returning the inner map without
	// making a copy.
	if _, ok := obj.(runtime.Unstructured); ok {
		obj = obj.DeepCopyObject()
	}
	rawMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}
	u := &unstructured.Unstructured{Object: rawMap}
	u.SetGroupVersionKind(gvk)

	return u, nil
}

// unsafeUnstructuredCopy returns a shallow copy of the unstructured object given as input.
// It copies the common fields such as `kind`, `apiVersion`, `metadata` and the patchType specified.
//
// It's not safe to modify any of the keys in the returned unstructured object, the result should be treated as read-only.
func unsafeUnstructuredCopy(obj *unstructured.Unstructured, focus patchType, clusterv1ConditionsFieldPath, metav1ConditionsFieldPath []string) *unstructured.Unstructured {
	// Create the return focused-unstructured object with a preallocated map.
	res := &unstructured.Unstructured{Object: make(map[string]interface{}, len(obj.Object))}

	// Ranges over the keys of the unstructured object, think of this as the very top level of an object
	// when submitting a yaml to kubectl or a client.
	// These would be keys like `apiVersion`, `kind`, `metadata`, `spec`, `status`, etc.
	for key := range obj.Object {
		value := obj.Object[key]

		preserve := false
		switch focus {
		case specPatch:
			// For what we define as `spec` fields, we should preserve everything
			// that's not `status`.
			preserve = key != string(statusPatch)
		case statusPatch:
			// For status, only preserve the status fields.
			preserve = key == string(focus)
		}

		// Perform a shallow copy only for the keys we're interested in,
		// or the ones that should be always preserved (like metadata).
		if preserve || preserveUnstructuredKeys[key] {
			res.Object[key] = value
		}
	}

	// If we've determined that new or old condition must be set,
	// when dealing with the status patch, remove corresponding sub-fields from the object.
	// NOTE: Removing conditions sub-fields changes the incoming object! This is safe because the condition patch
	// doesn't use the unstructured fields, and it runs before any other patch.
	//
	// If we want to be 100% safe, we could make a copy of the incoming object before modifying it, although
	// copies have a high cpu and high memory usage, therefore we intentionally choose to avoid extra copies
	// given that the ordering of operations and safety is handled internally by the patch helper.
	if focus == statusPatch {
		if len(clusterv1ConditionsFieldPath) > 0 {
			unstructured.RemoveNestedField(res.Object, clusterv1ConditionsFieldPath...)
		}
		if len(metav1ConditionsFieldPath) > 0 {
			unstructured.RemoveNestedField(res.Object, metav1ConditionsFieldPath...)
		}
	}

	return res
}

var (
	clusterv1ConditionsType = reflect.TypeOf(clusterv1.Conditions{})
	metav1ConditionsType    = reflect.TypeOf([]metav1.Condition{})
)

func identifyConditionsFieldsPath(obj runtime.Object) ([]string, []string, error) {
	if obj == nil {
		return nil, nil, errors.New("cannot identify conditions on a nil object")
	}

	ptr := reflect.ValueOf(obj)
	if ptr.Kind() != reflect.Pointer {
		return nil, nil, errors.New("cannot identify conditions on a object that is not a pointer")
	}

	elem := ptr.Elem()
	if !elem.IsValid() {
		return nil, nil, errors.New("obj must be a valid value (non zero value of its type)")
	}

	statusField := elem.FieldByName("Status")
	if statusField == (reflect.Value{}) {
		return nil, nil, nil
	}

	// NOTE: Given that we allow providers to migrate at different speed, it is required to support objects at the different stage of the transition from clusterv1.conditions to metav1.conditions.
	// In order to handle this, it is required to identify where conditions are supported (metav1 or clusterv1 and where they are located.

	var metav1ConditionsFields []string
	var clusterv1ConditionsFields []string

	if v1beta2Field := statusField.FieldByName("V1Beta2"); v1beta2Field != (reflect.Value{}) {
		if v1beta2Field.Kind() != reflect.Pointer {
			return nil, nil, errors.New("obj.status.v1beta2 must be a pointer")
		}

		v1beta2Elem := v1beta2Field.Elem()
		if !v1beta2Elem.IsValid() {
			// If the field is a zero value of its type, we can't further investigate type struct.
			// We assume the type is implemented according to transition guidelines
			metav1ConditionsFields = []string{"status", "v1beta2", "conditions"}
		} else {
			if conditionField := v1beta2Elem.FieldByName("Conditions"); conditionField != (reflect.Value{}) {
				metav1ConditionsFields = []string{"status", "v1beta2", "conditions"}
			}
		}
	}

	if conditionField := statusField.FieldByName("Conditions"); conditionField != (reflect.Value{}) {
		if conditionField.Type() == metav1ConditionsType {
			metav1ConditionsFields = []string{"status", "conditions"}
		}
		if conditionField.Type() == clusterv1ConditionsType {
			clusterv1ConditionsFields = []string{"status", "conditions"}
		}
	}

	if deprecatedField := statusField.FieldByName("Deprecated"); deprecatedField != (reflect.Value{}) {
		if deprecatedField.Kind() != reflect.Pointer {
			return nil, nil, errors.New("obj.status.deprecated must be a pointer")
		}

		deprecatedElem := deprecatedField.Elem()
		if !deprecatedElem.IsValid() {
			// If the field is a zero value of its type, we can't further investigate type struct.
			// We assume the type is implemented according to transition guidelines
			clusterv1ConditionsFields = []string{"status", "deprecated", "v1beta1", "conditions"}
		} else {
			if v1Beta1Field := deprecatedElem.FieldByName("V1Beta1"); deprecatedField != (reflect.Value{}) {
				if v1Beta1Field.Kind() != reflect.Pointer {
					return nil, nil, errors.New("obj.status.deprecated.v1beta1 must be a pointer")
				}

				v1Beta1Elem := v1Beta1Field.Elem()
				if !v1Beta1Elem.IsValid() {
					// If the field is a zero value of its type, we can't further investigate type struct.
					// We assume the type is implemented according to transition guidelines
					clusterv1ConditionsFields = []string{"status", "deprecated", "v1beta1", "conditions"}
				} else {
					if conditionField := v1Beta1Elem.FieldByName("Conditions"); conditionField != (reflect.Value{}) {
						clusterv1ConditionsFields = []string{"status", "deprecated", "v1beta1", "conditions"}
					}
				}
			}
		}
	}

	return metav1ConditionsFields, clusterv1ConditionsFields, nil
}
