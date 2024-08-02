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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
)

// NotYetReportedReason is set on missing conditions generated during mirror, aggregate or summary operations.
// Missing conditions are generated during the above operations when an expected condition does not exist on a object.
const NotYetReportedReason = "NotYetReported"

// MirrorOption is some configuration that modifies options for a mirrorInto request.
type MirrorOption interface {
	// ApplyToMirror applies this configuration to the given mirrorInto options.
	ApplyToMirror(*MirrorOptions)
}

// MirrorOptions allows to set options for the mirrorInto operation.
type MirrorOptions struct {
	overrideType string
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
// a new condition with status Unknown, reason NotYetReported is created.
//
// By default, the Mirror condition has the same type of the source condition, but this can be changed by using
// the OverrideType option.
func NewMirrorCondition(obj runtime.Object, conditionType string, opts ...MirrorOption) (*metav1.Condition, error) {
	mirrorOpt := &MirrorOptions{}
	mirrorOpt.ApplyOptions(opts)

	conditions, err := GetAll(obj)
	if err != nil {
		return nil, err
	}
	conditionOwner := getConditionOwnerInfo(obj)
	conditionOwnerString := strings.TrimSpace(fmt.Sprintf("%s %s", conditionOwner.Kind, klog.KRef(conditionOwner.Namespace, conditionOwner.Name)))

	var c *metav1.Condition
	for i := range conditions {
		if conditions[i].Type == conditionType {
			c = &metav1.Condition{
				Type:               conditions[i].Type,
				Status:             conditions[i].Status,
				ObservedGeneration: conditions[i].ObservedGeneration,
				LastTransitionTime: conditions[i].LastTransitionTime,
				Reason:             conditions[i].Reason,
				Message:            strings.TrimSpace(fmt.Sprintf("%s (from %s)", conditions[i].Message, conditionOwnerString)),
			}
		}
	}

	if c == nil {
		c = &metav1.Condition{
			Type:               conditionType,
			Status:             metav1.ConditionUnknown,
			LastTransitionTime: metav1.Now(),
			Reason:             NotYetReportedReason,
			Message:            fmt.Sprintf("Condition %s not yet reported from %s", conditionType, conditionOwnerString),
		}
	}

	if mirrorOpt.overrideType != "" {
		c.Type = mirrorOpt.overrideType
	}

	return c, nil
}

// SetMirrorCondition is a convenience method that calls NewMirrorCondition to create a mirror condition from the source object,
// and then calls Set to add the new condition to the target object.
func SetMirrorCondition(sourceObj, targetObj runtime.Object, conditionType string, opts ...MirrorOption) error {
	mirrorCondition, err := NewMirrorCondition(sourceObj, conditionType, opts...)
	if err != nil {
		return err
	}
	return Set(targetObj, *mirrorCondition)
}
