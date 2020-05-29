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
	"fmt"
	"strings"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

// mergeOptions allows to set strategies for merging a set of conditions into a single condition,
// and more specifically for computing the target Reason and the target Message.
type mergeOptions struct {
	conditionOrder []clusterv1.ConditionType
	addSourceRef   bool
	stepCounter    int
}

// MergeOption defines an option for computing a summary of conditions.
type MergeOption func(*mergeOptions)

// WithConditionOrder instructs merge about the condition order to be used when
// merging conditions Reason and Message into the Target Condition.
// The remaining conditions (not included in this list) will be sorted by type, and in case of
// two conditions with the same type, by the source object name.
func WithConditionOrder(t ...clusterv1.ConditionType) MergeOption {
	return func(c *mergeOptions) {
		c.conditionOrder = t
	}
}

// WithStepCounter instructs merge to add a "x of y completed" string to the message,
// where x is the number of conditions with Status=true and y is the number passed to this method.
func WithStepCounter(to int) MergeOption {
	return func(c *mergeOptions) {
		c.stepCounter = to
	}
}

// AddSourceRef instructs merge to add info about the originating object to the target Reason.
func AddSourceRef() MergeOption {
	return func(c *mergeOptions) {
		c.addSourceRef = true
	}
}

// getReason returns the reason to be applied to the condition resulting by merging a set of condition groups.
// The reason is computed according to the given mergeOptions.
func getReason(groups conditionGroups, options *mergeOptions) string {
	return getFirstReason(groups, options.conditionOrder, options.addSourceRef)
}

// getFirstReason returns the first reason from the ordered list of conditions in the top group.
// If required, the reason gets localized with the source object reference.
func getFirstReason(g conditionGroups, order []clusterv1.ConditionType, addSourceRef bool) string {
	if condition := getFirstCondition(g, order); condition != nil {
		reason := condition.Reason
		if addSourceRef {
			return localizeReason(reason, condition.Getter)
		}
		return reason
	}
	return ""
}

// localizeReason adds info about the originating object to the target Reason.
func localizeReason(reason string, from Getter) string {
	if strings.Contains(reason, "@") {
		return reason
	}
	return fmt.Sprintf("%s@%s/%s", reason, from.GetObjectKind().GroupVersionKind().Kind, from.GetName())
}

// getMessage returns the message to be applied to the condition resulting by merging a set of condition groups.
// The message is computed according to the given mergeOptions, but in case of errors or warning a
// summary of existing errors is automatically added.
func getMessage(groups conditionGroups, options *mergeOptions) string {
	errorAndWarningSummary := getErrorAndWarningSummary(groups)
	if options.stepCounter > 0 {
		counter := getStepCounterMessage(groups, options.stepCounter)
		if errorAndWarningSummary != "" {
			return fmt.Sprintf("%s, %s", counter, errorAndWarningSummary)
		}
		return counter

	}

	firstMessage := getFirstMessage(groups, options.conditionOrder)
	if errorAndWarningSummary != "" {
		return fmt.Sprintf("%s (%s)", firstMessage, errorAndWarningSummary)
	}
	return firstMessage
}

func getErrorAndWarningSummary(groups conditionGroups) string {
	msgs := []string{}
	if errorGroup := groups.ErrorGroup(); errorGroup != nil {
		switch l := len(errorGroup.conditions); l {
		case 1:
			msgs = append(msgs, "1 error")
		default:
			msgs = append(msgs, fmt.Sprintf("%d errors", len(errorGroup.conditions)))
		}
	}
	if warningGroup := groups.WarningGroup(); warningGroup != nil {
		switch l := len(warningGroup.conditions); l {
		case 1:
			msgs = append(msgs, "1 warning")
		default:
			msgs = append(msgs, fmt.Sprintf("%d warnings", len(warningGroup.conditions)))
		}
	}
	if len(msgs) > 0 {
		return strings.Join(msgs, ", ")
	}
	return ""
}

// getStepCounterMessage returns a message "x of y completed", where x is the number of conditions
// with Status=true and y is the number passed to this method.
func getStepCounterMessage(groups conditionGroups, to int) string {
	ct := 0
	if trueGroup := groups.TrueGroup(); trueGroup != nil {
		ct = len(trueGroup.conditions)
	}
	return fmt.Sprintf("%d of %d completed", ct, to)
}

// getFirstMessage returns the message from the ordered list of conditions in the top group.
func getFirstMessage(groups conditionGroups, order []clusterv1.ConditionType) string {
	if condition := getFirstCondition(groups, order); condition != nil {
		return condition.Message
	}
	return ""
}

// getFirstCondition returns a first condition from the ordered list of conditions in the top group.
func getFirstCondition(g conditionGroups, priority []clusterv1.ConditionType) *localizedCondition {
	topGroup := g.TopGroup()
	if topGroup == nil {
		return nil
	}

	switch len(topGroup.conditions) {
	case 0:
		return nil
	case 1:
		return &topGroup.conditions[0]
	default:
		for _, p := range priority {
			for _, c := range topGroup.conditions {
				if c.Type == p {
					return &c
				}
			}
		}
		return &topGroup.conditions[0]
	}
}
