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
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/cluster-api/util"
)

// Setter interface defines methods that a Cluster API object should implement in order to
// use the conditions package for setting conditions.
type Setter interface {
	Getter

	// SetV1Beta2Conditions sets conditions for an API object.
	// Note: SetV1Beta2Conditions will be renamed to SetConditions in a later stage of the transition to V1Beta2.
	SetV1Beta2Conditions([]metav1.Condition)
}

// SetOption is some configuration that modifies options for a Set request.
type SetOption interface {
	// ApplyToSet applies this configuration to the given Set options.
	ApplyToSet(option *SetOptions)
}

// SetOptions allows to define options for the set operation.
type SetOptions struct {
	conditionSortFunc ConditionSortFunc
}

// ApplyOptions applies the given list options on these options,
// and then returns itself (for convenient chaining).
func (o *SetOptions) ApplyOptions(opts []SetOption) *SetOptions {
	for _, opt := range opts {
		opt.ApplyToSet(o)
	}
	return o
}

// Set a condition on the given object implementing the Setter interface; if the object is nil, the operation is a no-op.
//
// When setting a condition:
// - condition.ObservedGeneration will be set to object.Metadata.Generation if targetObj is a metav1.Object.
// - If the condition does not exist and condition.LastTransitionTime is not set, time.Now is used.
// - If the condition already exists, condition.Status is changing and condition.LastTransitionTime is not set, time.Now is used.
// - If the condition already exists, condition.Status is NOT changing, all the fields can be changed except for condition.LastTransitionTime.
//
// Additionally, Set enforces a default condition order (Available and Ready fist, everything else in alphabetical order),
// but this can be changed by using the ConditionSortFunc option.
//
// Please note that Set does not support setting conditions to an unstructured object nor to API types not implementing
// the Setter interface. Eventually, users can implement wrappers on those types implementing this interface and
// taking care of aligning the condition format if necessary.
func Set(targetObj Setter, condition metav1.Condition, opts ...SetOption) {
	if util.IsNil(targetObj) {
		return
	}

	setOpt := &SetOptions{
		// By default, sort conditions by the default condition order: available and ready always first, deleting and paused always last, all the other conditions in alphabetical order.
		conditionSortFunc: defaultSortLessFunc,
	}
	setOpt.ApplyOptions(opts)

	if objMeta, ok := targetObj.(metav1.Object); ok {
		condition.ObservedGeneration = objMeta.GetGeneration()
	}

	conditions := targetObj.GetV1Beta2Conditions()
	if changed := setStatusCondition(&conditions, condition); !changed {
		return
	}

	if setOpt.conditionSortFunc != nil {
		sort.SliceStable(conditions, func(i, j int) bool {
			return setOpt.conditionSortFunc(conditions[i], conditions[j])
		})
	}

	targetObj.SetV1Beta2Conditions(conditions)
}

func setStatusCondition(conditions *[]metav1.Condition, condition metav1.Condition) bool {
	// Truncate last transition time to seconds.
	// This prevents inconsistencies from what we have in objects in memory and what Marshal/Unmarshal
	// will do while the data is sent to/read from the API server.
	if condition.LastTransitionTime.IsZero() {
		condition.LastTransitionTime = metav1.Now()
	}
	condition.LastTransitionTime.Time = condition.LastTransitionTime.Truncate(1 * time.Second)
	return meta.SetStatusCondition(conditions, condition)
}

// Delete deletes the condition with the given type.
func Delete(to Setter, conditionType string) {
	if to == nil {
		return
	}

	conditions := to.GetV1Beta2Conditions()
	newConditions := make([]metav1.Condition, 0, len(conditions)-1)
	for _, condition := range conditions {
		if condition.Type != conditionType {
			newConditions = append(newConditions, condition)
		}
	}
	to.SetV1Beta2Conditions(newConditions)
}
