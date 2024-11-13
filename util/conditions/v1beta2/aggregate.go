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
	"fmt"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// AggregateOption is some configuration that modifies options for a aggregate call.
type AggregateOption interface {
	// ApplyToAggregate applies this configuration to the given aggregate options.
	ApplyToAggregate(option *AggregateOptions)
}

// AggregateOptions allows to set options for the aggregate operation.
type AggregateOptions struct {
	mergeStrategy                  MergeStrategy
	targetConditionType            string
	negativePolarityConditionTypes []string
}

// ApplyOptions applies the given list options on these options,
// and then returns itself (for convenient chaining).
func (o *AggregateOptions) ApplyOptions(opts []AggregateOption) *AggregateOptions {
	for _, opt := range opts {
		opt.ApplyToAggregate(o)
	}
	return o
}

// NewAggregateCondition aggregates a condition from a list of objects; the given condition must have positive polarity;
// if the given condition does not exist in one of the source objects, missing conditions are considered Unknown, reason NotYetReported.
//
// If the source conditions type has negative polarity, it should be indicated with the NegativePolarityConditionTypes option;
// in this case NegativePolarityConditionTypes must match the sourceConditionType, and by default also the resulting condition
// will have negative polarity.
//
// By default, the Aggregate condition has the same type of the source condition, but this can be changed by using
// the TargetConditionType option.
//
// Additionally, it is possible to inject custom merge strategies using the CustomMergeStrategy option.
func NewAggregateCondition[T Getter](sourceObjs []T, sourceConditionType string, opts ...AggregateOption) (*metav1.Condition, error) {
	if len(sourceObjs) == 0 {
		return nil, errors.New("sourceObjs can't be empty")
	}

	aggregateOpt := &AggregateOptions{
		targetConditionType: sourceConditionType,
	}
	aggregateOpt.ApplyOptions(opts)

	if len(aggregateOpt.negativePolarityConditionTypes) != 0 && (aggregateOpt.negativePolarityConditionTypes[0] != sourceConditionType || len(aggregateOpt.negativePolarityConditionTypes) > 1) {
		return nil, errors.Errorf("negativePolarityConditionTypes can only be set to [%q]", sourceConditionType)
	}

	if aggregateOpt.mergeStrategy == nil {
		// Note: If mergeStrategy is not explicitly set, target condition has negative polarity if source condition has negative polarity
		targetConditionHasPositivePolarity := !sets.New[string](aggregateOpt.negativePolarityConditionTypes...).Has(sourceConditionType)
		aggregateOpt.mergeStrategy = DefaultMergeStrategy(TargetConditionHasPositivePolarity(targetConditionHasPositivePolarity), GetPriorityFunc(GetDefaultMergePriorityFunc(aggregateOpt.negativePolarityConditionTypes...)))
	}

	conditionsInScope := make([]ConditionWithOwnerInfo, 0, len(sourceObjs))
	for _, obj := range sourceObjs {
		conditions := getConditionsWithOwnerInfo(obj)

		// Drops all the conditions not in scope for the merge operation
		hasConditionType := false
		for _, condition := range conditions {
			if condition.Type != sourceConditionType {
				continue
			}
			conditionsInScope = append(conditionsInScope, condition)
			hasConditionType = true
			break
		}

		// Add the expected conditions if it does not exist, so we are compliant with K8s guidelines
		// (all missing conditions should be considered unknown).
		if !hasConditionType {
			conditionOwner := getConditionOwnerInfo(obj)

			conditionsInScope = append(conditionsInScope, ConditionWithOwnerInfo{
				OwnerResource: conditionOwner,
				Condition: metav1.Condition{
					Type:    sourceConditionType,
					Status:  metav1.ConditionUnknown,
					Reason:  NotYetReportedReason,
					Message: fmt.Sprintf("Condition %s not yet reported", sourceConditionType),
					// NOTE: LastTransitionTime and ObservedGeneration are not relevant for merge.
				},
			})
		}
	}

	status, reason, message, err := aggregateOpt.mergeStrategy.Merge(conditionsInScope, []string{sourceConditionType})
	if err != nil {
		return nil, err
	}

	c := &metav1.Condition{
		Type:    aggregateOpt.targetConditionType,
		Status:  status,
		Reason:  reason,
		Message: message,
		// NOTE: LastTransitionTime and ObservedGeneration will be set when this condition is added to an object by calling Set.
	}

	return c, err
}

// SetAggregateCondition is a convenience method that calls NewAggregateCondition to create an aggregate condition from the source objects,
// and then calls Set to add the new condition to the target object.
func SetAggregateCondition[T Getter](sourceObjs []T, targetObj Setter, conditionType string, opts ...AggregateOption) error {
	aggregateCondition, err := NewAggregateCondition(sourceObjs, conditionType, opts...)
	if err != nil {
		return err
	}
	Set(targetObj, *aggregateCondition)
	return nil
}
