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

package v1beta2

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/cluster-api/util"
)

// TODO: Move to the API package.
const (
	// NoReasonReported identifies a clusterv1.Condition that reports no reason.
	NoReasonReported = "NoReasonReported"
)

// Getter interface defines methods that an API object should implement in order to
// use the conditions package for getting conditions.
type Getter interface {
	// GetV1Beta2Conditions returns the list of conditions for a cluster API object.
	// Note: GetV1Beta2Conditions will be renamed to GetConditions in a later stage of the transition to V1Beta2.
	GetV1Beta2Conditions() []metav1.Condition
}

// Get returns a condition from the object implementing the Getter interface.
//
// Please note that Get does not support reading conditions from unstructured objects nor from API types not implementing
// the Getter interface. Eventually, users can implement wrappers on those types implementing this interface and
// taking care of aligning the condition format if necessary.
func Get(sourceObj Getter, sourceConditionType string) *metav1.Condition {
	// if obj is nil, the requested condition does not exist.
	if util.IsNil(sourceObj) {
		return nil
	}

	// Otherwise get the requested condition.
	return meta.FindStatusCondition(sourceObj.GetV1Beta2Conditions(), sourceConditionType)
}

// UnstructuredGet returns a condition from an Unstructured object.
//
// UnstructuredGet supports retrieving conditions from objects at different stages of the transition from
// clusterv1.conditions to the metav1.Condition type:
//   - Objects with clusterv1.Conditions in status.conditions; in this case a best effort conversion
//     to metav1.Condition is performed, just enough to allow surfacing a condition from a provider object with Mirror
//   - Objects with metav1.Condition in status.v1beta2.conditions
//   - Objects with metav1.Condition in status.conditions
func UnstructuredGet(sourceObj runtime.Unstructured, sourceConditionType string) (*metav1.Condition, error) {
	if util.IsNil(sourceObj) {
		return nil, errors.New("sourceObj is nil")
	}

	ownerInfo := getConditionOwnerInfo(sourceObj)

	value, exists, err := unstructured.NestedFieldNoCopy(sourceObj.UnstructuredContent(), "status", "v1beta2", "conditions")
	if exists && err == nil {
		if conditions, ok := value.([]interface{}); ok {
			r, err := convertFromUnstructuredConditions(conditions)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to convert %s.status.v1beta2.conditions to []metav1.Condition", ownerInfo)
			}
			return meta.FindStatusCondition(r, sourceConditionType), nil
		}
	}

	value, exists, err = unstructured.NestedFieldNoCopy(sourceObj.UnstructuredContent(), "status", "conditions")
	if exists && err == nil {
		if conditions, ok := value.([]interface{}); ok {
			r, err := convertFromUnstructuredConditions(conditions)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to convert %s.status.conditions to []metav1.Condition", ownerInfo)
			}
			return meta.FindStatusCondition(r, sourceConditionType), nil
		}
	}

	// With unstructured, it is not possible to detect if conditions are not set if the type is wrongly defined.
	// This methods assume condition are not set.
	return nil, nil
}

// convertFromUnstructuredConditions converts []interface{} to []metav1.Condition; this operation must account for
// objects which are not transitioning to metav1.Condition, or not yet fully transitioned, and thus a best
// effort conversion of values to metav1.Condition is performed.
func convertFromUnstructuredConditions(conditions []interface{}) ([]metav1.Condition, error) {
	if conditions == nil {
		return nil, nil
	}

	convertedConditions := make([]metav1.Condition, 0, len(conditions))
	for _, c := range conditions {
		cMap, ok := c.(map[string]interface{})
		if !ok || cMap == nil {
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
		if v, ok := cMap["lastTransitionTime"]; ok && v != nil && v.(string) != "" {
			if err := lastTransitionTime.UnmarshalQueryParameter(v.(string)); err != nil {
				return nil, errors.Wrapf(err, "failed to unmarshal lastTransitionTime value: %s", v)
			}
		}

		var reason string
		if v, ok := cMap["reason"]; ok {
			reason = v.(string)
		}

		var message string
		if v, ok := cMap["message"]; ok {
			message = v.(string)
		}

		c := metav1.Condition{
			Type:               conditionType,
			Status:             metav1.ConditionStatus(status),
			ObservedGeneration: observedGeneration,
			LastTransitionTime: lastTransitionTime,
			Reason:             reason,
			Message:            message,
		}
		if err := validateAndFixConvertedCondition(&c); err != nil {
			return nil, err
		}

		convertedConditions = append(convertedConditions, c)
	}
	return convertedConditions, nil
}

// validateAndFixConvertedCondition validates and fixes a clusterv1.Condition converted to a metav1.Condition.
// this operation assumes conditions have been set using Cluster API condition utils;
// also, only a few, minimal rules are enforced, just enough to allow surfacing a condition from a providers object with Mirror.
func validateAndFixConvertedCondition(c *metav1.Condition) error {
	if c.Type == "" {
		return errors.New("condition type must be set")
	}
	if c.Status == "" {
		return errors.New("condition status must be set")
	}
	if c.Reason == "" {
		switch c.Status {
		case metav1.ConditionFalse: // When using old Cluster API condition utils, for conditions with Status false, Reason can be empty only when a condition has negative polarity (means "good")
			c.Reason = NoReasonReported
		case metav1.ConditionTrue: // When using old Cluster API condition utils, for conditions with Status true, Reason can be empty only when a condition has positive polarity (means "good").
			c.Reason = NoReasonReported
		case metav1.ConditionUnknown:
			return errors.New("condition reason must be set when a condition is unknown")
		}
	}

	// NOTE: Empty LastTransitionTime is tolerated because it will be set when assigning the newly generated mirror condition to an object.
	// NOTE: Other metav1.Condition validations rules, e.g. regex, are not enforced at this stage; they will be enforced by the API server at a later stage.

	return nil
}
