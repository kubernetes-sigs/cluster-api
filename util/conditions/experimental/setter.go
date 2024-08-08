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
	"fmt"
	"reflect"
	"sort"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// SetOption is some configuration that modifies options for a Set request.
type SetOption interface {
	// ApplyToSet applies this configuration to the given Set options.
	ApplyToSet(option *SetOptions)
}

// SetOptions allows to define options for the set operation.
type SetOptions struct {
	conditionsFields []string
	less             Less
}

// ApplyOptions applies the given list options on these options,
// and then returns itself (for convenient chaining).
func (o *SetOptions) ApplyOptions(opts []SetOption) *SetOptions {
	for _, opt := range opts {
		opt.ApplyToSet(o)
	}
	return o
}

// Set a condition on the given object.
//
// Set support adding conditions to objects at different stages of the transition to metav1.condition type:
// - Objects with metav1.condition in status.experimental conditions
// - Objects with metav1.condition in status.conditions
//
// In case the object is unstructured, it is required to provide the path where conditions are defined using
// the ConditionFields option (because it is not possible to infer where conditions are by looking at the UnstructuredContent only).
//
// Additionally, Set enforce the lexicographic condition order (Available and Ready fist, everything else in alphabetical order),
// but this can be changed by using the ConditionSortFunc option.
func Set(obj runtime.Object, condition metav1.Condition, opts ...SetOption) error {
	conditions, err := GetAll(obj)
	if err != nil {
		return err
	}

	if changed := meta.SetStatusCondition(&conditions, condition); !changed {
		return nil
	}

	return SetAll(obj, conditions, opts...)
}

// SetAll the conditions on the given object.
//
// SetAll support adding conditions to objects at different stages of the transition to metav1.condition type:
// - Objects with metav1.condition in status.experimental conditions
// - Objects with metav1.condition in status.conditions
//
// In case the object is unstructured, it is required to provide the path where conditions are defined using
// the ConditionFields option (because it is not possible to infer where conditions are by looking at the UnstructuredContent only).
//
// Additionally, SetAll enforce the lexicographic condition order (Available and Ready fist, everything else in alphabetical order),
// but this can be changed by using the ConditionSortFunc option.
func SetAll(obj runtime.Object, conditions []metav1.Condition, opts ...SetOption) error {
	setOpt := &SetOptions{
		// By default sort condition by lexicographicLess order (first available, then ready, then the other conditions if alphabetical order.
		less: lexicographicLess,
	}
	setOpt.ApplyOptions(opts)

	// TODO: think about setting ObservedGeneration from obj
	// TODO: think about setting LastTransition Time with reconcile time --> All the conditions will flit at the same time, but we loose correlation with logs

	if setOpt.less != nil {
		sort.SliceStable(conditions, func(i, j int) bool {
			return setOpt.less(conditions[i], conditions[j])
		})
	}

	switch obj.(type) {
	case runtime.Unstructured:
		return setToUnstructuredObject(obj, conditions, setOpt.conditionsFields)
	default:
		return setToTypedObject(obj, conditions)
	}
}

func setToUnstructuredObject(obj runtime.Object, conditions []metav1.Condition, conditionsFields []string) error {
	if obj == nil {
		return errors.New("cannot set conditions on a nil object")
	}

	// NOTE: Given that we allow providers to migrate at different speed, it is required to support objects at the different stage of the transition from legacy conditions to metav1.conditions.
	// In order to handle this with Unstructured, it is required to ask the path for the conditions field in the given object (it cannot be inferred).
	// conditionsFields should be dropped when v1beta1 API are removed because new conditions will always be in Status.Conditions.

	if len(conditionsFields) == 0 {
		return errors.New("while transition from legacy conditions types to metav1.conditions types is in progress, cannot set conditions on Unstructured object without conditionFields path being set")
	}

	u, ok := obj.(runtime.Unstructured)
	if !ok {
		return errors.New("cannot call setToUnstructuredObject on a object which is not Unstructured")
	}

	if !reflect.ValueOf(u).Elem().IsValid() {
		return errors.New("obj cannot be nil")
	}

	v := make([]interface{}, 0, len(conditions))
	for i := range conditions {
		c, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&conditions[i])
		if err != nil {
			return errors.Wrapf(err, "failed to convert conditions %s to Unstructured", conditions[i].Type)
		}
		v = append(v, c)
	}

	if err := unstructured.SetNestedField(u.UnstructuredContent(), v, conditionsFields...); err != nil {
		return errors.Wrap(err, "failed to set conditions into Unstructured")
	}
	return nil
}

var metav1ConditionsType = reflect.TypeOf([]metav1.Condition{})

func setToTypedObject(obj runtime.Object, conditions []metav1.Condition) error {
	if obj == nil {
		return errors.New("cannot set conditions on a nil object")
	}

	ptr := reflect.ValueOf(obj)
	if ptr.Kind() != reflect.Pointer {
		return errors.New("cannot set conditions on a object that is not a pointer")
	}

	elem := ptr.Elem()
	if !elem.IsValid() {
		return errors.New("obj must be a valid value (non zero value of its type)")
	}

	statusField := elem.FieldByName("Status")
	if statusField == (reflect.Value{}) {
		return errors.New("cannot set conditions on a object without a status field")
	}

	// Set conditions.
	// NOTE: Given that we allow providers to migrate at different speed, it is required to support objects at the different stage of the transition from legacy conditions to metav1.conditions.
	// In order to handle this, first try to set Status.ExperimentalConditions, then Status.Conditions.
	// The ExperimentalConditions branch should be dropped when v1beta1 API are removed.

	if conditionField := statusField.FieldByName("ExperimentalConditions"); conditionField != (reflect.Value{}) {
		fmt.Println("Status.ExperimentalConditions is a", reflect.TypeOf(conditionField.Interface()).String())

		if conditionField.Type() != metav1ConditionsType {
			return errors.Errorf("cannot set conditions on Status.ExperimentalConditions field if it isn't %s: %s type detected", metav1ConditionsType.String(), reflect.TypeOf(conditionField.Interface()).String())
		}

		setToTypedField(conditionField, conditions)
		return nil
	}

	if conditionField := statusField.FieldByName("Conditions"); conditionField != (reflect.Value{}) {
		fmt.Println("Status.Conditions is a", reflect.TypeOf(conditionField.Interface()).String())

		if conditionField.Type() != metav1ConditionsType {
			return errors.Errorf("cannot set conditions on Status.Conditions field if it isn't %s: %s type detected", metav1ConditionsType.String(), reflect.TypeOf(conditionField.Interface()).String())
		}

		setToTypedField(conditionField, conditions)
		return nil
	}

	return errors.New("cannot set conditions on a object without Status.ExperimentalConditions or Status.Conditions")
}

func setToTypedField(conditionField reflect.Value, conditions []metav1.Condition) {
	n := len(conditions)
	conditionField.Set(reflect.MakeSlice(conditionField.Type(), n, n))
	for i := range n {
		itemField := conditionField.Index(i)
		itemField.Set(reflect.ValueOf(conditions[i]))
	}
}
