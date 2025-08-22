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

package conditions

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// NotYetReportedReason is set on missing conditions generated during mirror, aggregate or summary operations.
// Missing conditions are generated during the above operations when an expected condition does not exist on a object.
// TODO: Move to the API package.
const NotYetReportedReason = "NotYetReported"

// MirrorOption is some configuration that modifies options for a mirror call.
type MirrorOption interface {
	// ApplyToMirror applies this configuration to the given mirror options.
	ApplyToMirror(*MirrorOptions)
}

// MirrorOptions allows to set options for the mirror operation.
type MirrorOptions struct {
	targetConditionType string
	fallbackStatus      metav1.ConditionStatus
	fallbackReason      string
	fallbackMessage     string
}

// ApplyOptions applies the given list options on these options,
// and then returns itself (for convenient chaining).
func (o *MirrorOptions) ApplyOptions(opts []MirrorOption) *MirrorOptions {
	for _, opt := range opts {
		opt.ApplyToMirror(o)
	}
	return o
}

// NewMirrorCondition create a mirror of the given condition from obj; if the given condition does not exist in the source obj,
// the condition specified in the FallbackCondition is used; if this option is not set, a new condition with status Unknown
// and reason NotYetReported is created.
//
// By default, the Mirror condition has the same type as the source condition, but this can be changed by using
// the TargetConditionType option.
func NewMirrorCondition(sourceObj Getter, sourceConditionType string, opts ...MirrorOption) *metav1.Condition {
	condition := Get(sourceObj, sourceConditionType)

	return newMirrorCondition(condition, sourceConditionType, opts)
}

func newMirrorCondition(sourceCondition *metav1.Condition, sourceConditionType string, opts []MirrorOption) *metav1.Condition {
	mirrorOpt := &MirrorOptions{
		targetConditionType: sourceConditionType,
	}
	mirrorOpt.ApplyOptions(opts)

	if sourceCondition != nil {
		return &metav1.Condition{
			Type:   mirrorOpt.targetConditionType,
			Status: sourceCondition.Status,
			// NOTE: we are preserving the original transition time (when the underlying condition changed)
			LastTransitionTime: sourceCondition.LastTransitionTime,
			Reason:             sourceCondition.Reason,
			Message:            sourceCondition.Message,
			// NOTE: ObservedGeneration will be set when this condition is added to an object by calling Set
			// (also preserving ObservedGeneration from the source object will be confusing when the mirror conditions shows up in the target object).
		}
	}

	if mirrorOpt.fallbackStatus != "" {
		return &metav1.Condition{
			Type:    mirrorOpt.targetConditionType,
			Status:  mirrorOpt.fallbackStatus,
			Reason:  mirrorOpt.fallbackReason,
			Message: mirrorOpt.fallbackMessage,
			// NOTE: LastTransitionTime and ObservedGeneration will be set when this condition is added to an object by calling Set.
		}
	}

	return &metav1.Condition{
		Type:    mirrorOpt.targetConditionType,
		Status:  metav1.ConditionUnknown,
		Reason:  NotYetReportedReason,
		Message: fmt.Sprintf("Condition %s not yet reported", sourceConditionType),
		// NOTE: LastTransitionTime and ObservedGeneration will be set when this condition is added to an object by calling Set.
	}
}

// SetMirrorCondition is a convenience method that calls NewMirrorCondition to create a mirror condition from the source object,
// and then calls Set to add the new condition to the target object.
func SetMirrorCondition(sourceObj Getter, targetObj Setter, sourceConditionType string, opts ...MirrorOption) {
	mirrorCondition := NewMirrorCondition(sourceObj, sourceConditionType, opts...)
	Set(targetObj, *mirrorCondition)
}

// NewMirrorConditionFromUnstructured is a convenience method create a mirror of the given condition from the unstructured source obj.
// It combines, UnstructuredGet, NewMirrorCondition (most specifically it uses only the logic to
// create a mirror condition).
func NewMirrorConditionFromUnstructured(sourceObj runtime.Unstructured, sourceConditionType string, opts ...MirrorOption) (*metav1.Condition, error) {
	condition, err := UnstructuredGet(sourceObj, sourceConditionType)
	if err != nil {
		return nil, err
	}
	return newMirrorCondition(condition, sourceConditionType, opts), nil
}

// SetMirrorConditionFromUnstructured is a convenience method that mirror of the given condition from the unstructured source obj
// into the target object. It combines, NewMirrorConditionFromUnstructured, and Set.
func SetMirrorConditionFromUnstructured(sourceObj runtime.Unstructured, targetObj Setter, sourceConditionType string, opts ...MirrorOption) error {
	condition, err := NewMirrorConditionFromUnstructured(sourceObj, sourceConditionType, opts...)
	if err != nil {
		return err
	}
	Set(targetObj, *condition)
	return nil
}

// BoolToStatus converts a bool to either metav1.ConditionTrue or metav1.ConditionFalse.
func BoolToStatus(status bool) metav1.ConditionStatus {
	if status {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}
