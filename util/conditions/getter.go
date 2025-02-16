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

package conditions

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// Getter interface defines methods that a Cluster API object should implement in order to
// use the conditions package for getting conditions.
type Getter interface {
	client.Object

	// GetConditions returns the list of conditions for a cluster API object.
	GetConditions() clusterv1.Conditions
}

// Get returns the condition with the given type, if the condition does not exist,
// it returns nil.
func Get(from Getter, t clusterv1.ConditionType) *clusterv1.Condition {
	conditions := from.GetConditions()
	if conditions == nil {
		return nil
	}

	for _, condition := range conditions {
		if condition.Type == t {
			return &condition
		}
	}
	return nil
}

// Has returns true if a condition with the given type exists.
func Has(from Getter, t clusterv1.ConditionType) bool {
	return Get(from, t) != nil
}

// IsTrue is true if the condition with the given type is True, otherwise it returns false
// if the condition is not True or if the condition does not exist (is nil).
func IsTrue(from Getter, t clusterv1.ConditionType) bool {
	if c := Get(from, t); c != nil {
		return c.Status == corev1.ConditionTrue
	}
	return false
}

// IsFalse is true if the condition with the given type is False, otherwise it returns false
// if the condition is not False or if the condition does not exist (is nil).
func IsFalse(from Getter, t clusterv1.ConditionType) bool {
	if c := Get(from, t); c != nil {
		return c.Status == corev1.ConditionFalse
	}
	return false
}

// IsUnknown is true if the condition with the given type is Unknown or if the condition
// does not exist (is nil).
func IsUnknown(from Getter, t clusterv1.ConditionType) bool {
	if c := Get(from, t); c != nil {
		return c.Status == corev1.ConditionUnknown
	}
	return true
}

// GetReason returns a nil safe string of Reason for the condition with the given type.
func GetReason(from Getter, t clusterv1.ConditionType) string {
	if c := Get(from, t); c != nil {
		return c.Reason
	}
	return ""
}

// GetMessage returns a nil safe string of Message.
func GetMessage(from Getter, t clusterv1.ConditionType) string {
	if c := Get(from, t); c != nil {
		return c.Message
	}
	return ""
}

// GetSeverity returns the condition Severity or nil if the condition
// does not exist (is nil).
func GetSeverity(from Getter, t clusterv1.ConditionType) *clusterv1.ConditionSeverity {
	if c := Get(from, t); c != nil {
		return &c.Severity
	}
	return nil
}

// GetLastTransitionTime returns the condition Severity or nil if the condition
// does not exist (is nil).
func GetLastTransitionTime(from Getter, t clusterv1.ConditionType) *metav1.Time {
	if c := Get(from, t); c != nil {
		return &c.LastTransitionTime
	}
	return nil
}

// summary returns a Ready condition with the summary of all the conditions existing
// on an object. If the object does not have other conditions, no summary condition is generated.
// NOTE: The resulting Ready condition will have positive polarity; the conditions we are starting from might have positive or negative polarity.
func summary(from Getter, options ...MergeOption) *clusterv1.Condition {
	conditions := from.GetConditions()

	mergeOpt := &mergeOptions{}
	for _, o := range options {
		o(mergeOpt)
	}

	// Identifies the conditions in scope for the Summary by taking all the existing conditions except Ready,
	// or, if a list of conditions types is specified, only the conditions the condition in that list.
	conditionsInScope := make([]localizedCondition, 0, len(conditions))
	for i := range conditions {
		c := conditions[i]
		if c.Type == clusterv1.ReadyCondition {
			continue
		}

		if mergeOpt.conditionTypes != nil {
			found := false
			for _, t := range mergeOpt.conditionTypes {
				if c.Type == t {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Keep track of the polarity of the condition we are starting from.
		polarity := PositivePolarity
		for _, t := range mergeOpt.negativeConditionTypes {
			if c.Type == t {
				polarity = NegativePolarity
				break
			}
		}

		conditionsInScope = append(conditionsInScope, localizedCondition{
			Condition: &c,
			Polarity:  polarity,
			Getter:    from,
		})
	}

	// If it is required to add a step counter only if a subset of condition exists, check if the conditions
	// in scope are included in this subset or not.
	if mergeOpt.addStepCounterIfOnlyConditionTypes != nil {
		for _, c := range conditionsInScope {
			found := false
			for _, t := range mergeOpt.addStepCounterIfOnlyConditionTypes {
				if c.Type == t {
					found = true
					break
				}
			}
			if !found {
				mergeOpt.addStepCounter = false
				break
			}
		}
	}

	// If it is required to add a step counter, determine the total number of conditions defaulting
	// to the selected conditions or, if defined, to the total number of conditions type to be considered.
	if mergeOpt.addStepCounter {
		mergeOpt.stepCounter = len(conditionsInScope)
		if mergeOpt.conditionTypes != nil {
			mergeOpt.stepCounter = len(mergeOpt.conditionTypes)
		}
		if mergeOpt.addStepCounterIfOnlyConditionTypes != nil {
			mergeOpt.stepCounter = len(mergeOpt.addStepCounterIfOnlyConditionTypes)
		}
	}

	return merge(conditionsInScope, clusterv1.ReadyCondition, mergeOpt)
}

// mirrorOptions allows to set options for the mirror operation.
type mirrorOptions struct {
	fallbackTo       *bool
	fallbackReason   string
	fallbackSeverity clusterv1.ConditionSeverity
	fallbackMessage  string
}

// MirrorOptions defines an option for mirroring conditions.
type MirrorOptions func(*mirrorOptions)

// WithFallbackValue specify a fallback value to use in case the mirrored condition does not exist;
// in case the fallbackValue is false, given values for reason, severity and message will be used.
func WithFallbackValue(fallbackValue bool, reason string, severity clusterv1.ConditionSeverity, message string) MirrorOptions {
	return func(c *mirrorOptions) {
		c.fallbackTo = &fallbackValue
		c.fallbackReason = reason
		c.fallbackSeverity = severity
		c.fallbackMessage = message
	}
}

// mirror mirrors the Ready condition from a dependent object into the target condition;
// if the Ready condition does not exist in the source object, no target conditions is generated.
// NOTE: Considering that we are mirroring Ready conditions with positive polarity, also the resulting condition will have positive polarity.
func mirror(from Getter, targetCondition clusterv1.ConditionType, options ...MirrorOptions) *clusterv1.Condition {
	mirrorOpt := &mirrorOptions{}
	for _, o := range options {
		o(mirrorOpt)
	}

	condition := Get(from, clusterv1.ReadyCondition)

	if mirrorOpt.fallbackTo != nil && condition == nil {
		switch *mirrorOpt.fallbackTo {
		case true:
			condition = TrueCondition(targetCondition)
		case false:
			condition = FalseCondition(targetCondition, mirrorOpt.fallbackReason, mirrorOpt.fallbackSeverity, mirrorOpt.fallbackMessage)
		}
	}

	if condition != nil {
		condition.Type = targetCondition
	}

	return condition
}

// Aggregates all the Ready condition from a list of dependent objects into the target object;
// if the Ready condition does not exist in one of the source object, the object is excluded from
// the aggregation; if none of the source object have ready condition, no target conditions is generated.
// NOTE: Considering that we are aggregating Ready conditions with positive polarity, also the resulting condition will have positive polarity.
func aggregate(from []Getter, targetCondition clusterv1.ConditionType, options ...MergeOption) *clusterv1.Condition {
	conditionsInScope := make([]localizedCondition, 0, len(from))
	for i := range from {
		condition := Get(from[i], clusterv1.ReadyCondition)

		conditionsInScope = append(conditionsInScope, localizedCondition{
			Condition: condition,
			Polarity:  PositivePolarity,
			Getter:    from[i],
		})
	}

	mergeOpt := &mergeOptions{
		addStepCounter: true,
		stepCounter:    len(from),
	}
	for _, o := range options {
		o(mergeOpt)
	}
	return merge(conditionsInScope, targetCondition, mergeOpt)
}
