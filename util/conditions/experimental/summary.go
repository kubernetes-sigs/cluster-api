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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

// SummaryOption is some configuration that modifies options for a summary request.
type SummaryOption interface {
	// ApplyToSummary applies this configuration to the given summary options.
	ApplyToSummary(*SummaryOptions)
}

// SummaryOptions allows to set options for the summary operation.
type SummaryOptions struct {
	mergeStrategy                  MergeStrategy
	conditionTypes                 []string
	negativePolarityConditionTypes []string
	stepCounter                    bool
}

// ApplyOptions applies the given list options on these options,
// and then returns itself (for convenient chaining).
func (o *SummaryOptions) ApplyOptions(opts []SummaryOption) *SummaryOptions {
	for _, opt := range opts {
		opt.ApplyToSummary(o)
	}
	return o
}

// NewSummaryCondition creates a new condition by summarizing a set of conditions from an object.
// If any of the condition in scope does not exist in the source object, missing conditions are considered Unknown, reason NotYetReported.
//
// By default, the Aggregate condition has the same type of the source condition, but this can be changed by using
// the OverrideType option.
//
// Additionally, it is possible to inject custom merge strategies using the WithMergeStrategy option or
// to add a step counter to the generated message by using the WithStepCounter option.
func NewSummaryCondition(obj runtime.Object, conditionType string, opts ...SummaryOption) (*metav1.Condition, error) {
	summarizeOpt := &SummaryOptions{
		mergeStrategy: newDefaultMergeStrategy(),
	}
	summarizeOpt.ApplyOptions(opts)

	conditions, err := getConditionsWithOwnerInfo(obj)
	if err != nil {
		return nil, err
	}

	expectedConditionTypes := sets.New[string](summarizeOpt.conditionTypes...)
	existingConditionTypes := sets.New[string]()

	// Drops all the conditions not in scope for the merge operation
	conditionsInScope := make([]ConditionWithOwnerInfo, 0, len(expectedConditionTypes))
	for _, condition := range conditions {
		if !expectedConditionTypes.Has(condition.Type) {
			continue
		}
		conditionsInScope = append(conditionsInScope, condition)
		existingConditionTypes.Insert(condition.Type)
	}

	// Add the expected conditions which do net exists, so we are compliant with K8s guidelines
	// (all missing conditions should be considered unknown).
	// TODO: consider if we want to allow exception to this rule. e.g it is ok that HealthCheckSucceeded is missing.

	diff := expectedConditionTypes.Difference(existingConditionTypes).UnsortedList()
	if len(diff) > 0 {
		conditionOwner := getConditionOwnerInfo(obj)

		for _, conditionType := range diff {
			conditionsInScope = append(conditionsInScope, ConditionWithOwnerInfo{
				OwnerResource: conditionOwner,
				Condition: metav1.Condition{
					Type:    conditionType,
					Status:  metav1.ConditionUnknown,
					Reason:  NotYetReportedReason,
					Message: fmt.Sprintf("Condition %s not yet reported", conditionType),
				},
			})
		}
	}

	status, reason, message, err := summarizeOpt.mergeStrategy.Merge(
		conditionsInScope,
		summarizeOpt.conditionTypes,
		sets.New[string](summarizeOpt.negativePolarityConditionTypes...),
		summarizeOpt.stepCounter,
	)
	if err != nil {
		return nil, err
	}

	return &metav1.Condition{
		Type:    conditionType,
		Status:  status,
		Reason:  reason,
		Message: message,
	}, err
}

// SetSummaryCondition is a convenience method that calls NewSummaryCondition to create a summary condition from the source object,
// and then calls Set to add the new condition to the target object.
func SetSummaryCondition(sourceObj, targetObj runtime.Object, conditionType string, opts ...MirrorOption) error {
	mirrorCondition, err := NewMirrorCondition(sourceObj, conditionType, opts...)
	if err != nil {
		return err
	}
	return Set(targetObj, *mirrorCondition)
}
