/*
Copyright 2024 The Kubernetes Authors.

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

package experimental

import (
	"reflect"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// Get returns a conditions from the object.
//
// Get support retrieving conditions from objects at different stages of the transition to metav1.condition type:
// - Objects with metav1.condition in status.experimental conditions
// - Objects with metav1.condition in status.conditions
//
// In case the object does not have metav1.conditions, Get tries to read clusterv1.condition from status.conditions
// and convert them to metav1.conditions.
func Get(obj runtime.Object, conditionType string) (*metav1.Condition, error) {
	conditions, err := GetAll(obj)
	if err != nil {
		return nil, err
	}
	return meta.FindStatusCondition(conditions, conditionType), nil
}

// GetAll returns all the conditions from the object.
//
// GetAll support retrieving conditions from objects at different stages of the transition to metav1.condition type:
// - Objects with metav1.condition in status.experimental conditions
// - Objects with metav1.condition in status.conditions
//
// In case the object does not have metav1.conditions, GetAll tries to read clusterv1.condition from status.conditions
// and convert them to metav1.conditions.
func GetAll(obj runtime.Object) ([]metav1.Condition, error) {
	if obj == nil {
		return nil, errors.New("obj cannot be nil")
	}

	switch obj.(type) {
	case runtime.Unstructured:
		return getFromUnstructuredObject(obj)
	default:
		return getFromTypedObject(obj)
	}
}

func getFromUnstructuredObject(obj runtime.Object) ([]metav1.Condition, error) {
	u, ok := obj.(runtime.Unstructured)
	if !ok {
		// NOTE: this should not happen due to the type assertion before calling this fun
		return nil, errors.New("obj cannot be converted to runtime.Unstructured")
	}

	if !reflect.ValueOf(u).Elem().IsValid() {
		return nil, errors.New("obj cannot be nil")
	}

	value, _, _ := unstructured.NestedFieldNoCopy(u.UnstructuredContent(), "status", "experimentalConditions")
	if conditions, ok := value.([]interface{}); ok {
		return convertUnstructuredConditions(conditions), nil
	}

	value, _, _ = unstructured.NestedFieldNoCopy(u.UnstructuredContent(), "status", "conditions")
	if conditions, ok := value.([]interface{}); ok {
		return convertUnstructuredConditions(conditions), nil
	}

	return nil, errors.New("obj must have Status with one of Conditions or ExperimentalConditions")
}

func convertUnstructuredConditions(conditions []interface{}) []metav1.Condition {
	if conditions == nil {
		return nil
	}

	convertedConditions := make([]metav1.Condition, 0, len(conditions))
	for _, c := range conditions {
		cMap, ok := c.(map[string]interface{})
		if !ok || cMap == nil {
			// TODO: think about returning an error in this case and when type of status are not set (as a signal it is a condition type)
			continue
		}

		var conditionType string
		if v, ok := cMap["type"]; ok {
			conditionType = v.(string)
		}

		var status string
		if v, ok := cMap["status"]; ok {
			status = v.(string)
		}

		var observedGeneration int64
		if v, ok := cMap["observedGeneration"]; ok {
			observedGeneration = v.(int64)
		}

		var lastTransitionTime metav1.Time
		if v, ok := cMap["lastTransitionTime"]; ok {
			_ = lastTransitionTime.UnmarshalQueryParameter(v.(string))
		}

		var reason string
		if v, ok := cMap["reason"]; ok {
			reason = v.(string)
		}

		var message string
		if v, ok := cMap["message"]; ok {
			message = v.(string)
		}

		convertedConditions = append(convertedConditions, metav1.Condition{
			Type:               conditionType,
			Status:             metav1.ConditionStatus(status),
			ObservedGeneration: observedGeneration,
			LastTransitionTime: lastTransitionTime,
			Reason:             reason,
			Message:            message,
		})
	}
	return convertedConditions
}

func getFromTypedObject(obj runtime.Object) ([]metav1.Condition, error) {
	ptr := reflect.ValueOf(obj)
	if ptr.Kind() != reflect.Pointer {
		return nil, errors.New("obj must be a pointer")
	}

	elem := ptr.Elem()
	if !elem.IsValid() {
		return nil, errors.New("obj must be a valid value (non zero value of its type)")
	}

	statusField := elem.FieldByName("Status")
	if statusField == (reflect.Value{}) {
		return nil, errors.New("obj must have a Status field")
	}

	// Get conditions.
	// NOTE: Given that we allow providers to migrate at different speed, it is required to support objects at the different stage of the transition from legacy conditions to metav1.conditions.
	// In order to handle this, first try to read Status.ExperimentalConditions, then Status.Conditions; for Status.Conditions, also support conversion from legacy conditions.
	// The ExperimentalConditions branch and the conversion from legacy conditions should be dropped when v1beta1 API are removed.

	if conditionField := statusField.FieldByName("ExperimentalConditions"); conditionField != (reflect.Value{}) {
		v1beta2Conditions, ok := conditionField.Interface().([]metav1.Condition)
		if !ok {
			return nil, errors.New("obj.Status.ExperimentalConditions must be of type []metav1.conditions")
		}
		return v1beta2Conditions, nil
	}

	if conditionField := statusField.FieldByName("Conditions"); conditionField != (reflect.Value{}) {
		conditions, ok := conditionField.Interface().([]metav1.Condition)
		if ok {
			return conditions, nil
		}

		return convertFromV1Beta1Conditions(conditionField)
	}

	return nil, errors.New("obj.Status must have one of Conditions or ExperimentalConditions")
}

func convertFromV1Beta1Conditions(conditionField reflect.Value) ([]metav1.Condition, error) {
	v1betaConditions, ok := conditionField.Interface().(clusterv1.Conditions)
	if !ok {
		return nil, errors.New("obj.Status.Conditions must be of type []metav1.conditions or []clusterv1.Condition")
	}

	convertedConditions := make([]metav1.Condition, len(v1betaConditions))
	for i, c := range v1betaConditions {
		convertedConditions[i] = metav1.Condition{
			Type:               string(c.Type),
			Status:             metav1.ConditionStatus(c.Status),
			LastTransitionTime: c.LastTransitionTime,
			Reason:             c.Reason,
			Message:            c.Message,
		}
	}
	return convertedConditions, nil
}
