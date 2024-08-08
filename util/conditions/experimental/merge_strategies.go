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
	"reflect"
	"sort"
	"strings"

	"github.com/gobuffalo/flect"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
)

const (
	// ManyIssuesReason is set on conditions generated during aggregate or summary operations when many conditions/objects are reporting issues.
	ManyIssuesReason = "MoreThanOneIssuesReported"

	// ManyUnknownsReason is set on conditions generated during aggregate or summary operations when many conditions/objects are reporting unknown.
	ManyUnknownsReason = "MoreThanOneUnknownReported"

	// ManyInfoReason is set on conditions generated during aggregate or summary operations when many conditions/objects are reporting info.
	ManyInfoReason = "MoreThanOneInfoReported"
)

// MergeStrategy defines a strategy used to merge conditions during the aggregate or summary operation.
type MergeStrategy interface {
	// Merge all the condition in input.
	//
	// It is up to the caller to ensure that all the expected conditions exist (e.g. by adding new conditions with status Unknown).
	// Condition in input must be of the given conditionTypes (other condition types must be discarded);
	//
	// The list of conditionTypes has an implicit order; it is up to the implementation of merge to use this info or not.
	// If negativeConditionTypes are in scope, the implementation of merge should treat them accordingly.
	//
	// It required, the implementation of merge must add a stepCounter to the output message.
	Merge(conditions []ConditionWithOwnerInfo, conditionTypes []string, negativeConditionTypes sets.Set[string], stepCounter bool) (status metav1.ConditionStatus, reason, message string, err error)
}

// ConditionWithOwnerInfo is a wrapper  metav1.Condition with addition ConditionOwnerInfo.
// Those info can be used when generating the message resulting from the merge operation.
type ConditionWithOwnerInfo struct {
	OwnerResource ConditionOwnerInfo
	metav1.Condition
}

// ConditionOwnerInfo contains info about the object that owns the condition.
type ConditionOwnerInfo struct {
	metav1.TypeMeta
	Namespace string
	Name      string
}

// defaultMergeStrategy defines the default merge strategy for Cluster API conditions.
type defaultMergeStrategy struct{}

type mergePriority uint8

const (
	issueMergePriority mergePriority = iota
	unknownMergePriority
	infoMergePriority
)

func newDefaultMergeStrategy() MergeStrategy {
	return &defaultMergeStrategy{}
}

// Merge all the condition in input based on a strategy that surfaces issue first, then unknown conditions, the info (if none of issues and unknown condition exists).
// - issues: conditions with positive polarity (normal True) and status False or conditions with negative polarity (normal False) and status True.
// - unknown: conditions with status unknown.
// - info: conditions with positive polarity (normal True) and status True or conditions with negative polarity (normal False) and status False.
func (d *defaultMergeStrategy) Merge(conditions []ConditionWithOwnerInfo, conditionTypes []string, negativeConditionTypes sets.Set[string], stepCounter bool) (status metav1.ConditionStatus, reason, message string, err error) {
	// Infer which operation is calling this func, so it is possible to use different strategies for computing the message for the target condition.
	// - When merge should consider a single condition type, we can assume this func is called within an aggregate operation
	//   (Aggregate should merge the same condition across many objects)
	isAggregateOperation := len(conditionTypes) == 1

	// - Otherwise we can assume this func is called within an summary operation
	//   (Aggregate should merge the same condition across many objects)
	isSummaryOperation := !isAggregateOperation

	// sortConditions the relevance defined by the users (the order of condition types), LastTransition time (older first).
	sortConditions(conditions, conditionTypes)

	issueConditions, unknownConditions, infoConditions := splitConditionsByPriority(conditions, negativeConditionTypes)

	// Compute the status for the target condition:
	// Note: This function always return a condition with positive polarity.
	// - If the top group are issues, use false
	// - If the top group are unknown, use unknown
	// - If the top group are info, use true
	switch {
	case len(issueConditions) > 0:
		status = metav1.ConditionFalse
	case len(unknownConditions) > 0:
		status = metav1.ConditionUnknown
	case len(infoConditions) > 0:
		status = metav1.ConditionTrue
	}

	// Compute the reason for the target condition:
	// - In case there is only one condition in the top group, use the reason from this condition
	// - In case there are more than one condition in the top group, use a generic reason (for the target group)
	switch {
	case len(issueConditions) == 1:
		reason = issueConditions[0].Reason
	case len(issueConditions) > 1:
		reason = ManyIssuesReason
	case len(unknownConditions) == 1:
		reason = unknownConditions[0].Reason
	case len(unknownConditions) > 1:
		reason = ManyUnknownsReason
	case len(infoConditions) == 1:
		reason = infoConditions[0].Reason
	case len(infoConditions) > 1:
		reason = ManyInfoReason
	}

	// Compute the message for the target condition, which is optimized for the operation being performed.

	// When performing the summary operation, usually we are merging a small set of conditions form the same object,
	// Considering the small number of conditions, involved it is acceptable/preferred to provide as much detail
	// as possible about the messages from the conditions being merged.
	//
	// Accordingly, the resulting message is composed by all the messages from conditions classified as issues/unknown;
	// messages from conditions classified as info are included only if there are no issues/unknown.
	//
	// e.g. Condition-B (False): Message-B; Condition-!C (True): Message-!C; Condition-A (Unknown): Message-A
	//
	// When including messages from conditions, they are sorted by issue/unknown and by the implicit order of condition types
	// provided by the user (it is consider as order of relevance).
	if isSummaryOperation {
		messages := []string{}
		for _, condition := range append(issueConditions, append(unknownConditions, infoConditions...)...) {
			priority := getPriority(condition.Condition, negativeConditionTypes)
			if priority == infoMergePriority {
				// Drop info messages when we are surfacing issues on unknown.
				if status != metav1.ConditionTrue {
					continue
				}
				// Drop info conditions with empty messages.
				if condition.Message == "" {
					continue
				}
			}

			m := fmt.Sprintf("%s (%s)", condition.Type, condition.Status)
			if condition.Message != "" {
				m += fmt.Sprintf(": %s", condition.Message)
			}
			messages = append(messages, m)
		}

		// Prepend the step counter if required.
		if stepCounter {
			totalSteps := len(conditionTypes)
			stepsCompleted := len(infoConditions)

			messages = append([]string{fmt.Sprintf("%d of %d completed", stepsCompleted, totalSteps)}, messages...)
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
	// e.g. (False): Message-1 from default/obj0, default/obj1, default/obj2 and 2 other Objects
	//
	// If there are other objects - objects not included in the list above - reporting issues/unknown (or info there no issues/unknown),
	// the number of those objects is surfaced.
	//
	// e.g. ...; other 2 Objects with issues; other 1 Objects unknown
	//
	if isAggregateOperation {
		// Gets the kind to be used when composing messages.
		// NOTE: This assume all the objects in an aggregate operation are of the same kind.
		kind := ""
		if len(conditions) > 0 {
			kind = flect.Pluralize(conditions[0].OwnerResource.Kind)
		}

		n := 3
		messages := []string{}

		// Get max n issue messages, decrement n, and track if there are other objects reporting issues not included in the messages.
		if len(issueConditions) > 0 {
			issueMessages, otherIssues := aggregateMessages(issueConditions, &n, kind, false)

			messages = append(messages, issueMessages...)
			if otherIssues > 0 {
				messages = append(messages, fmt.Sprintf("other %d %s with issues", otherIssues, kind))
			}
		}

		// Get max n unknown messages, decrement n, and track if there are other objects reporting unknown not included in the messages.
		if len(unknownConditions) > 0 {
			unknownMessages, otherUnknown := aggregateMessages(unknownConditions, &n, kind, false)

			messages = append(messages, unknownMessages...)
			if otherUnknown > 0 {
				messages = append(messages, fmt.Sprintf("other %d %s unknown", otherUnknown, kind))
			}
		}

		// Only if there are no issue or unknown,
		// Get max n info messages, decrement n, and track if there are other objects reporting info not included in the messages.
		if len(issueConditions) == 0 && len(unknownConditions) == 0 && len(infoConditions) > 0 {
			infoMessages, infoUnknown := aggregateMessages(infoConditions, &n, kind, true)

			messages = append(messages, infoMessages...)
			if infoUnknown > 0 {
				messages = append(messages, fmt.Sprintf("other %d %s with info messages", infoUnknown, kind))
			}
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
func splitConditionsByPriority(conditions []ConditionWithOwnerInfo, negativePolarityConditionTypes sets.Set[string]) (issueConditions, unknownConditions, infoConditions []ConditionWithOwnerInfo) {
	for _, condition := range conditions {
		switch getPriority(condition.Condition, negativePolarityConditionTypes) {
		case issueMergePriority:
			issueConditions = append(issueConditions, condition)
		case unknownMergePriority:
			unknownConditions = append(unknownConditions, condition)
		case infoMergePriority:
			infoConditions = append(infoConditions, condition)
		}
	}
	return issueConditions, unknownConditions, infoConditions
}

// getPriority returns the merge priority for each condition.
// The default implementation identifies as:
// - issues: conditions with positive polarity (normal True) and status False or conditions with negative polarity (normal False) and status True.
// - unknown: conditions with status unknown.
// - info: conditions with positive polarity (normal True) and status True or conditions with negative polarity (normal False) and status False.
func getPriority(condition metav1.Condition, negativePolarityConditionTypes sets.Set[string]) mergePriority {
	switch condition.Status {
	case metav1.ConditionTrue:
		if negativePolarityConditionTypes.Has(condition.Type) {
			return issueMergePriority
		}
		return infoMergePriority
	case metav1.ConditionFalse:
		if negativePolarityConditionTypes.Has(condition.Type) {
			return infoMergePriority
		}
		return issueMergePriority
	case metav1.ConditionUnknown:
		return unknownMergePriority
	}

	// Note: this should never happen. In case, those conditions are considered like condition with unknown status.
	return unknownMergePriority
}

// aggregateMessages return messages for the aggregate operation.
func aggregateMessages(conditions []ConditionWithOwnerInfo, n *int, kind string, dropEmpty bool) (messages []string, other int) {
	// create a map with all the messages and the list of objects reporting the same message.
	messageObjMap := map[string][]string{}
	for _, condition := range conditions {
		if dropEmpty && condition.Message == "" {
			continue
		}

		m := fmt.Sprintf("(%s)", condition.Status)
		if condition.Message != "" {
			m += fmt.Sprintf(": %s", condition.Message)
		}
		if _, ok := messageObjMap[m]; !ok {
			messageObjMap[m] = []string{}
		}
		messageObjMap[m] = append(messageObjMap[m], klog.KRef(condition.OwnerResource.Namespace, condition.OwnerResource.Name).String())
	}

	// compute the order of messages according to the number of objects reporting the same message.
	// Note: The message text is used as a secondary criteria to sort messages with the same number of objects.
	messageIndex := make([]string, 0, len(messageObjMap))
	for m := range messageObjMap {
		messageIndex = append(messageIndex, m)
	}

	sort.SliceStable(messageIndex, func(i, j int) bool {
		return len(messageObjMap[messageIndex[i]]) > len(messageObjMap[messageIndex[j]]) ||
			(len(messageObjMap[messageIndex[i]]) == len(messageObjMap[messageIndex[j]]) && messageIndex[i] < messageIndex[j])
	})

	// Pick the first n messages, decrement n.
	// For each message, add up to three objects; if more add the number of the remaining objects with the same message.
	// Count the number of objects reporting messages not included in the above.
	// Note: we are showing up to three objects because usually control plane has 3 machines, and we want to show all issues
	// to control plane machines if any,
	for i := range len(messageIndex) {
		if *n == 0 {
			other += len(messageObjMap[messageIndex[i]])
			continue
		}

		m := messageIndex[i]
		allObjects := messageObjMap[messageIndex[i]]
		if len(allObjects) <= 3 {
			m += fmt.Sprintf(" from %s", strings.Join(allObjects, ", "))
		} else {
			m += fmt.Sprintf(" from %s and %d other %s", strings.Join(allObjects[:3], ", "), len(allObjects)-3, kind)
		}

		messages = append(messages, m)
		*n--
	}

	return messages, other
}

// getConditionsWithOwnerInfo return all the conditions from an object each one with the corresponding ConditionOwnerInfo.
func getConditionsWithOwnerInfo(obj runtime.Object) ([]ConditionWithOwnerInfo, error) {
	ret := make([]ConditionWithOwnerInfo, 0, 10)
	conditions, err := GetAll(obj)
	if err != nil {
		return nil, err
	}
	for _, condition := range conditions {
		ret = append(ret, ConditionWithOwnerInfo{
			OwnerResource: getConditionOwnerInfo(obj),
			Condition:     condition,
		})
	}
	return ret, nil
}

// getConditionOwnerInfo return the ConditionOwnerInfo for the given object.
func getConditionOwnerInfo(obj runtime.Object) ConditionOwnerInfo {
	var apiVersion, kind, name, namespace string
	kind = obj.GetObjectKind().GroupVersionKind().Kind
	apiVersion = obj.GetObjectKind().GroupVersionKind().GroupVersion().String()

	if kind == "" {
		t := reflect.TypeOf(obj)
		if t.Kind() == reflect.Pointer {
			kind = t.Elem().Name()
		}
	}

	if objMeta, ok := obj.(metav1.Object); ok {
		name = objMeta.GetName()
		namespace = objMeta.GetNamespace()
	}

	return ConditionOwnerInfo{
		TypeMeta: metav1.TypeMeta{
			Kind:       kind,
			APIVersion: apiVersion,
		},
		Namespace: namespace,
		Name:      name,
	}
}
