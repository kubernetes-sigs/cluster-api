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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// SummaryOption is some configuration that modifies options for a summary call.
type SummaryOption interface {
	// ApplyToSummary applies this configuration to the given summary options.
	ApplyToSummary(*SummaryOptions)
}

// SummaryOptions allows to set options for the summary operation.
type SummaryOptions struct {
	mergeStrategy                  MergeStrategy
	conditionTypes                 []string
	negativePolarityConditionTypes []string
	ignoreTypesIfMissing           []string
	overrideConditions             []ConditionWithOwnerInfo
}

// ApplyOptions applies the given list options on these options,
// and then returns itself (for convenient chaining).
func (o *SummaryOptions) ApplyOptions(opts []SummaryOption) *SummaryOptions {
	for _, opt := range opts {
		opt.ApplyToSummary(o)
	}
	return o
}

// NewSummaryCondition creates a new condition by summarizing a set of conditions from an object; the list of
// types of the conditions to be summarized must be provided with the ForConditionTypes option;
// conditions types with negative polarity, should be indicated with the NegativePolarityConditionTypes option.
//
// If any of the condition in scope does not exist in the source object, missing conditions are considered Unknown, reason NotYetReported.
// Use the IgnoreTypesIfMissing to exclude types from this option.
//
// Additionally, it is possible to inject custom merge strategies using the CustomMergeStrategy option or
// to add a step counter to the generated message by using the StepCounter option.
func NewSummaryCondition(sourceObj Getter, targetConditionType string, opts ...SummaryOption) (*metav1.Condition, error) {
	summarizeOpt := &SummaryOptions{}
	summarizeOpt.ApplyOptions(opts)
	if summarizeOpt.mergeStrategy == nil {
		// Note. Summary always assume the target condition type has positive polarity.
		summarizeOpt.mergeStrategy = DefaultMergeStrategy(GetPriorityFunc(GetDefaultMergePriorityFunc(summarizeOpt.negativePolarityConditionTypes...)))
	}

	if len(summarizeOpt.conditionTypes) == 0 {
		return nil, errors.New("option ForConditionTypes not provided or empty")
	}

	for _, conditionType := range summarizeOpt.conditionTypes {
		if conditionType == targetConditionType {
			return nil, errors.Errorf("option ForConditionTypes cannot include %s (target condition type)", targetConditionType)
		}
	}

	expectedConditionTypes := sets.New[string](summarizeOpt.conditionTypes...)
	ignoreTypesIfMissing := sets.New[string](summarizeOpt.ignoreTypesIfMissing...)
	existingConditionTypes := sets.New[string]()

	conditions := getConditionsWithOwnerInfo(sourceObj)

	conditionsByType := map[string]ConditionWithOwnerInfo{}
	for _, c := range conditions {
		conditionsByType[c.Type] = c
	}
	overrideConditionsByType := map[string]ConditionWithOwnerInfo{}
	for _, c := range summarizeOpt.overrideConditions {
		if _, ok := overrideConditionsByType[c.Type]; ok {
			return nil, errors.Errorf("override condition %s specified multiple times", c.Type)
		}

		overrideConditionsByType[c.Type] = c

		if _, ok := conditionsByType[c.Type]; !ok {
			return nil, errors.Errorf("override condition %s must exist in source object", c.Type)
		}
	}

	conditionsInScope := make([]ConditionWithOwnerInfo, 0, len(expectedConditionTypes))
	for _, condition := range conditions {
		// Drops all the conditions not in scope for the merge operation
		if !expectedConditionTypes.Has(condition.Type) {
			continue
		}

		if overrideCondition, ok := overrideConditionsByType[condition.Type]; ok {
			conditionsInScope = append(conditionsInScope, overrideCondition)
		} else {
			conditionsInScope = append(conditionsInScope, condition)
		}

		existingConditionTypes.Insert(condition.Type)
	}

	// Add the expected conditions which do not exist, so we are compliant with K8s guidelines
	// (all missing conditions should be considered unknown).

	diff := expectedConditionTypes.Difference(existingConditionTypes).Difference(ignoreTypesIfMissing).UnsortedList()
	if len(diff) > 0 {
		conditionOwner := getConditionOwnerInfo(sourceObj)

		for _, c := range diff {
			conditionsInScope = append(conditionsInScope, ConditionWithOwnerInfo{
				OwnerResource: conditionOwner,
				Condition: metav1.Condition{
					Type:    c,
					Status:  metav1.ConditionUnknown,
					Reason:  NotYetReportedReason,
					Message: "Condition not yet reported",
					// NOTE: LastTransitionTime and ObservedGeneration are not relevant for merge.
				},
			})
		}
	}

	if len(conditionsInScope) == 0 {
		return nil, errors.New("summary can't be performed when the list of conditions to be summarized is empty")
	}

	status, reason, message, err := summarizeOpt.mergeStrategy.Merge(SummaryMergeOperation, conditionsInScope, summarizeOpt.conditionTypes)
	if err != nil {
		return nil, err
	}

	return &metav1.Condition{
		Type:    targetConditionType,
		Status:  status,
		Reason:  reason,
		Message: message,
		// NOTE: LastTransitionTime and ObservedGeneration will be set when this condition is added to an object by calling Set.
	}, nil
}

// SetSummaryCondition is a convenience method that calls NewSummaryCondition to create a summary condition from the source object,
// and then calls Set to add the new condition to the target object.
func SetSummaryCondition(sourceObj Getter, targetObj Setter, targetConditionType string, opts ...SummaryOption) error {
	summaryCondition, err := NewSummaryCondition(sourceObj, targetConditionType, opts...)
	if err != nil {
		return err
	}
	Set(targetObj, *summaryCondition)
	return nil
}
