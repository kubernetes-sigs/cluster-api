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
	"reflect"
	"sort"
	"strings"

	"github.com/gobuffalo/flect"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

// TODO: Move to the API package.
const (
	// MultipleIssuesReportedReason is set on conditions generated during aggregate or summary operations when multiple conditions/objects are reporting issues.
	// NOTE: If a custom merge strategy is used for the aggregate or summary operations, this might not be true anymore.
	MultipleIssuesReportedReason = "MultipleIssuesReported"

	// MultipleUnknownReportedReason is set on conditions generated during aggregate or summary operations when multiple conditions/objects are reporting unknown.
	// NOTE: If a custom merge strategy is used for the aggregate or summary operations, this might not be true anymore.
	MultipleUnknownReportedReason = "MultipleUnknownReported"

	// MultipleInfoReportedReason is set on conditions generated during aggregate or summary operations when multiple conditions/objects are reporting info.
	// NOTE: If a custom merge strategy is used for the aggregate or summary operations, this might not be true anymore.
	MultipleInfoReportedReason = "MultipleInfoReported"
)

// ConditionWithOwnerInfo is a wrapper around metav1.Condition with additional ConditionOwnerInfo.
// These infos can be used when generating the message resulting from the merge operation.
type ConditionWithOwnerInfo struct {
	OwnerResource ConditionOwnerInfo
	metav1.Condition
}

// ConditionOwnerInfo contains infos about the object that owns the condition.
type ConditionOwnerInfo struct {
	Kind string
	Name string
}

// String returns a string representation of the ConditionOwnerInfo.
func (o ConditionOwnerInfo) String() string {
	return fmt.Sprintf("%s %s", o.Kind, o.Name)
}

// MergeStrategy defines a strategy used to merge conditions during the aggregate or summary operation.
type MergeStrategy interface {
	// Merge passed in conditions.
	//
	// It is up to the caller to ensure that all the expected conditions exist (e.g. by adding new conditions with status Unknown).
	// Conditions passed in must be of the given conditionTypes (other condition types must be discarded).
	//
	// The list of conditionTypes has an implicit order; it is up to the implementation of merge to use this info or not.
	Merge(conditions []ConditionWithOwnerInfo, conditionTypes []string) (status metav1.ConditionStatus, reason, message string, err error)
}

// DefaultMergeStrategyWithCustomPriority is the default merge strategy with a customized getPriority function.
func DefaultMergeStrategyWithCustomPriority(getPriorityFunc func(condition metav1.Condition) MergePriority) MergeStrategy {
	return &defaultMergeStrategy{
		getPriorityFunc: getPriorityFunc,
	}
}

func newDefaultMergeStrategy(negativePolarityConditionTypes sets.Set[string]) MergeStrategy {
	return &defaultMergeStrategy{
		getPriorityFunc: GetDefaultMergePriorityFunc(negativePolarityConditionTypes),
	}
}

// GetDefaultMergePriorityFunc returns the merge priority for each condition.
// It assigns following priority values to conditions:
// - issues: conditions with positive polarity (normal True) and status False or conditions with negative polarity (normal False) and status True.
// - unknown: conditions with status unknown.
// - info: conditions with positive polarity (normal True) and status True or conditions with negative polarity (normal False) and status False.
func GetDefaultMergePriorityFunc(negativePolarityConditionTypes sets.Set[string]) func(condition metav1.Condition) MergePriority {
	return func(condition metav1.Condition) MergePriority {
		switch condition.Status {
		case metav1.ConditionTrue:
			if negativePolarityConditionTypes.Has(condition.Type) {
				return IssueMergePriority
			}
			return InfoMergePriority
		case metav1.ConditionFalse:
			if negativePolarityConditionTypes.Has(condition.Type) {
				return InfoMergePriority
			}
			return IssueMergePriority
		case metav1.ConditionUnknown:
			return UnknownMergePriority
		}

		// Note: this should never happen. In case, those conditions are considered like conditions with unknown status.
		return UnknownMergePriority
	}
}

// MergePriority defines the priority for a condition during a merge operation.
type MergePriority uint8

const (
	// IssueMergePriority is the merge priority used by GetDefaultMergePriority in case the condition state is considered an issue.
	IssueMergePriority MergePriority = iota

	// UnknownMergePriority is the merge priority used by GetDefaultMergePriority in case of unknown conditions.
	UnknownMergePriority

	// InfoMergePriority is the merge priority used by GetDefaultMergePriority in case the condition state is not considered an issue.
	InfoMergePriority
)

// defaultMergeStrategy defines the default merge strategy for Cluster API conditions.
type defaultMergeStrategy struct {
	getPriorityFunc func(condition metav1.Condition) MergePriority
}

// Merge all conditions in input based on a strategy that surfaces issues first, then unknown conditions, then info (if none of issues and unknown condition exists).
// - issues: conditions with positive polarity (normal True) and status False or conditions with negative polarity (normal False) and status True.
// - unknown: conditions with status unknown.
// - info: conditions with positive polarity (normal True) and status True or conditions with negative polarity (normal False) and status False.
func (d *defaultMergeStrategy) Merge(conditions []ConditionWithOwnerInfo, conditionTypes []string) (status metav1.ConditionStatus, reason, message string, err error) {
	if len(conditions) == 0 {
		return "", "", "", errors.New("can't merge an empty list of conditions")
	}

	if d.getPriorityFunc == nil {
		return "", "", "", errors.New("can't merge without a getPriority func")
	}

	// Infer which operation is calling this func, so it is possible to use different strategies for computing the message for the target condition.
	// - When merge should consider a single condition type, we can assume this func is called within an aggregate operation
	//   (Aggregate should merge the same condition across many objects)
	isAggregateOperation := len(conditionTypes) == 1

	// - Otherwise we can assume this func is called within a summary operation
	//   (Summary should merge different conditions from the same object)
	isSummaryOperation := !isAggregateOperation

	// sortConditions the relevance defined by the users (the order of condition types), LastTransition time (older first).
	sortConditions(conditions, conditionTypes)

	issueConditions, unknownConditions, infoConditions := splitConditionsByPriority(conditions, d.getPriorityFunc)

	// Compute the status for the target condition:
	// Note: This function always returns a condition with positive polarity.
	// - if there are issues, use false
	// - else if there are unknown, use unknown
	// - else if there are info, use true
	switch {
	case len(issueConditions) > 0:
		status = metav1.ConditionFalse
	case len(unknownConditions) > 0:
		status = metav1.ConditionUnknown
	case len(infoConditions) > 0:
		status = metav1.ConditionTrue
	default:
		// NOTE: this is already handled above, but repeating also here for better readability.
		return "", "", "", errors.New("can't merge an empty list of conditions")
	}

	// Compute the reason for the target condition:
	// - In case there is only one condition in the top group, use the reason from this condition
	// - In case there are more than one condition in the top group, use a generic reason (for the target group)
	switch {
	case len(issueConditions) == 1:
		reason = issueConditions[0].Reason
	case len(issueConditions) > 1:
		reason = MultipleIssuesReportedReason
	case len(unknownConditions) == 1:
		reason = unknownConditions[0].Reason
	case len(unknownConditions) > 1:
		reason = MultipleUnknownReportedReason
	case len(infoConditions) == 1:
		reason = infoConditions[0].Reason
	case len(infoConditions) > 1:
		reason = MultipleInfoReportedReason
	default:
		// NOTE: this is already handled above, but repeating also here for better readability.
		return "", "", "", errors.New("can't merge an empty list of conditions")
	}

	// Compute the message for the target condition, which is optimized for the operation being performed.

	// When performing the summary operation, usually we are merging a small set of conditions from the same object,
	// Considering the small number of conditions, involved it is acceptable/preferred to provide as much detail
	// as possible about the messages from the conditions being merged.
	//
	// Accordingly, the resulting message is composed by all the messages from conditions classified as issues/unknown;
	// messages from conditions classified as info are included only if there are no issues/unknown.
	//
	// e.g. Condition-B (False): Message-B; Condition-!C (True): Message-!C; Condition-A (Unknown): Message-A
	//
	// When including messages from conditions, they are sorted by issue/unknown and by the implicit order of condition types
	// provided by the user (it is considered as order of relevance).
	if isSummaryOperation {
		messages := []string{}
		for _, condition := range append(issueConditions, append(unknownConditions, infoConditions...)...) {
			priority := d.getPriorityFunc(condition.Condition)
			if priority == InfoMergePriority {
				// Drop info messages when we are surfacing issues or unknown.
				if status != metav1.ConditionTrue {
					continue
				}
				// Drop info conditions with empty messages.
				if condition.Message == "" {
					continue
				}
			}

			var m string
			if condition.Message != "" {
				m = fmt.Sprintf("%s: %s", condition.Type, condition.Message)
			} else {
				m = fmt.Sprintf("%s: No additional info provided", condition.Type)
			}
			messages = append(messages, m)
		}

		message = strings.Join(messages, "; ")
	}

	// When performing the aggregate operation, we are merging one single condition from potentially many objects.
	// Considering the high number of conditions involved, the messages from the conditions being merged must be filtered/summarized
	// using rules designed to surface the most important issues.
	//
	// Accordingly, the resulting message is composed by only three messages from conditions classified as issues/unknown;
	// instead three messages from conditions classified as info are included only if there are no issues/unknown.
	//
	// The number of objects reporting the same message determine the order used to pick the messages to be shown;
	// For each message it is reported a list of max 3 objects reporting the message; if more objects are reporting the same
	// message, the number of those objects is surfaced.
	//
	// e.g. (False): Message-1 from obj0, obj1, obj2 and 2 more Objects
	//
	// If there are other objects - objects not included in the list above - reporting issues/unknown (or info there no issues/unknown),
	// the number of those objects is surfaced.
	//
	// e.g. ...; 2 more Objects with issues; 1 more Objects with unknown status
	//
	if isAggregateOperation {
		n := 3
		messages := []string{}

		// Get max n issue messages, decrement n, and track if there are other objects reporting issues not included in the messages.
		if len(issueConditions) > 0 {
			issueMessages := aggregateMessages(issueConditions, &n, false, "with other issues")
			messages = append(messages, issueMessages...)
		}

		// Get max n unknown messages, decrement n, and track if there are other objects reporting unknown not included in the messages.
		if len(unknownConditions) > 0 {
			unknownMessages := aggregateMessages(unknownConditions, &n, false, "with status unknown")
			messages = append(messages, unknownMessages...)
		}

		// Only if there are no issue or unknown,
		// Get max n info messages, decrement n, and track if there are other objects reporting info not included in the messages.
		if len(issueConditions) == 0 && len(unknownConditions) == 0 && len(infoConditions) > 0 {
			infoMessages := aggregateMessages(infoConditions, &n, true, "with additional info")
			messages = append(messages, infoMessages...)
		}

		message = strings.Join(messages, "; ")
	}

	return status, reason, message, nil
}

// sortConditions by condition types order, LastTransitionTime
// (the order of relevance defined by the users, the oldest first).
func sortConditions(conditions []ConditionWithOwnerInfo, orderedConditionTypes []string) {
	conditionOrder := make(map[string]int, len(orderedConditionTypes))
	for i, conditionType := range orderedConditionTypes {
		conditionOrder[conditionType] = i
	}

	sort.SliceStable(conditions, func(i, j int) bool {
		// Sort by condition order (user defined, useful when computing summary of different conditions from the same object)
		return conditionOrder[conditions[i].Type] < conditionOrder[conditions[j].Type] ||
			// If same condition order, sort by last transition time (useful when computing aggregation of the same conditions from different objects)
			(conditionOrder[conditions[i].Type] == conditionOrder[conditions[j].Type] && conditions[i].LastTransitionTime.Before(&conditions[j].LastTransitionTime))
	})
}

// splitConditionsByPriority split conditions in 3 groups:
// - conditions representing an issue.
// - conditions with status unknown.
// - conditions representing an info.
// NOTE: The order of conditions is preserved in each group.
func splitConditionsByPriority(conditions []ConditionWithOwnerInfo, getPriority func(condition metav1.Condition) MergePriority) (issueConditions, unknownConditions, infoConditions []ConditionWithOwnerInfo) {
	for _, condition := range conditions {
		switch getPriority(condition.Condition) {
		case IssueMergePriority:
			issueConditions = append(issueConditions, condition)
		case UnknownMergePriority:
			unknownConditions = append(unknownConditions, condition)
		case InfoMergePriority:
			infoConditions = append(infoConditions, condition)
		}
	}
	return issueConditions, unknownConditions, infoConditions
}

// aggregateMessages returns messages for the aggregate operation.
func aggregateMessages(conditions []ConditionWithOwnerInfo, n *int, dropEmpty bool, otherMessage string) (messages []string) {
	// create a map with all the messages and the list of objects reporting the same message.
	messageObjMap := map[string]map[string][]string{}
	for _, condition := range conditions {
		if dropEmpty && condition.Message == "" {
			continue
		}

		m := condition.Message
		if _, ok := messageObjMap[condition.OwnerResource.Kind]; !ok {
			messageObjMap[condition.OwnerResource.Kind] = map[string][]string{}
		}
		messageObjMap[condition.OwnerResource.Kind][m] = append(messageObjMap[condition.OwnerResource.Kind][m], condition.OwnerResource.Name)
	}

	// Gets the objects kind (with a stable order).
	kinds := make([]string, 0, len(messageObjMap))
	for kind := range messageObjMap {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)

	// Aggregate messages for each object kind.
	for _, kind := range kinds {
		kindPlural := flect.Pluralize(kind)
		messageObjMapForKind := messageObjMap[kind]

		// compute the order of messages according to the number of objects reporting the same message.
		// Note: The message text is used as a secondary criteria to sort messages with the same number of objects.
		messageIndex := make([]string, 0, len(messageObjMapForKind))
		for m := range messageObjMapForKind {
			messageIndex = append(messageIndex, m)
		}

		sort.SliceStable(messageIndex, func(i, j int) bool {
			return len(messageObjMapForKind[messageIndex[i]]) > len(messageObjMapForKind[messageIndex[j]]) ||
				(len(messageObjMapForKind[messageIndex[i]]) == len(messageObjMapForKind[messageIndex[j]]) && messageIndex[i] < messageIndex[j])
		})

		// Pick the first n messages, decrement n.
		// For each message, add up to three objects; if more add the number of the remaining objects with the same message.
		// Count the number of objects reporting messages not included in the above.
		// Note: we are showing up to three objects because usually control plane has 3 machines, and we want to show all issues
		// to control plane machines if any,
		var other = 0
		for _, m := range messageIndex {
			if *n == 0 {
				other += len(messageObjMapForKind[m])
				continue
			}

			msg := m
			allObjects := messageObjMapForKind[m]
			sort.Strings(allObjects)
			switch {
			case len(allObjects) == 0:
				// This should never happen, entry in the map exists only when an object reports a message.
			case len(allObjects) == 1:
				msg += fmt.Sprintf(" from %s %s", kind, strings.Join(allObjects, ", "))
			case len(allObjects) <= 3:
				msg += fmt.Sprintf(" from %s %s", kindPlural, strings.Join(allObjects, ", "))
			default:
				msg += fmt.Sprintf(" from %s %s and %d more", kindPlural, strings.Join(allObjects[:3], ", "), len(allObjects)-3)
			}

			messages = append(messages, msg)
			*n--
		}

		if other == 1 {
			messages = append(messages, fmt.Sprintf("%d %s %s", other, kind, otherMessage))
		}
		if other > 1 {
			messages = append(messages, fmt.Sprintf("%d %s %s", other, kindPlural, otherMessage))
		}
	}

	return messages
}

// getConditionsWithOwnerInfo return all the conditions from an object each one with the corresponding ConditionOwnerInfo.
func getConditionsWithOwnerInfo(obj Getter) []ConditionWithOwnerInfo {
	ret := make([]ConditionWithOwnerInfo, 0, 10)
	conditions := obj.GetV1Beta2Conditions()
	ownerInfo := getConditionOwnerInfo(obj)
	for _, condition := range conditions {
		ret = append(ret, ConditionWithOwnerInfo{
			OwnerResource: ownerInfo,
			Condition:     condition,
		})
	}
	return ret
}

// getConditionOwnerInfo return the ConditionOwnerInfo for the given object.
// Note: Given that controller runtime often does not set typeMeta for objects,
// in case kind is missing we are falling back to the type name, which in most cases
// is the same as kind.
func getConditionOwnerInfo(obj any) ConditionOwnerInfo {
	var kind, name string
	if runtimeObject, ok := obj.(runtime.Object); ok {
		kind = runtimeObject.GetObjectKind().GroupVersionKind().Kind
	}

	if kind == "" {
		t := reflect.TypeOf(obj)
		if t.Kind() == reflect.Pointer {
			kind = t.Elem().Name()
		} else {
			kind = t.Name()
		}
	}

	if objMeta, ok := obj.(metav1.Object); ok {
		name = objMeta.GetName()
	}

	return ConditionOwnerInfo{
		Kind: kind,
		Name: name,
	}
}
