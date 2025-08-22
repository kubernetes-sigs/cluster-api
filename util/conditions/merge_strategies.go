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
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/gobuffalo/flect"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// ConditionWithOwnerInfo is a wrapper around metav1.Condition with additional ConditionOwnerInfo.
// These infos can be used when generating the message resulting from the merge operation.
type ConditionWithOwnerInfo struct {
	OwnerResource ConditionOwnerInfo
	metav1.Condition
}

// ConditionOwnerInfo contains infos about the object that owns the condition.
type ConditionOwnerInfo struct {
	Kind                  string
	Name                  string
	IsControlPlaneMachine bool
}

// String returns a string representation of the ConditionOwnerInfo.
func (o ConditionOwnerInfo) String() string {
	return fmt.Sprintf("%s %s", o.Kind, o.Name)
}

// MergeOperation defines merge operations.
type MergeOperation string

const (
	// SummaryMergeOperation defines a merge operation of type Summary.
	// Summary should merge different conditions from the same object.
	SummaryMergeOperation MergeOperation = "Summary"

	// AggregateMergeOperation defines a merge operation of type Aggregate.
	// Aggregate should merge the same condition across many objects.
	AggregateMergeOperation MergeOperation = "Aggregate"
)

// MergeStrategy defines a strategy used to merge conditions during the aggregate or summary operation.
type MergeStrategy interface {
	// Merge passed in conditions.
	//
	// It is up to the caller to ensure that all the expected conditions exist (e.g. by adding new conditions with status Unknown).
	// Conditions passed in must be of the given conditionTypes (other condition types must be discarded).
	//
	// The list of conditionTypes has an implicit order; it is up to the implementation of merge to use this info or not.
	Merge(operation MergeOperation, conditions []ConditionWithOwnerInfo, conditionTypes []string) (status metav1.ConditionStatus, reason, message string, err error)
}

// DefaultMergeStrategyOption is some configuration that modifies the DefaultMergeStrategy behaviour.
type DefaultMergeStrategyOption interface {
	// ApplyToDefaultMergeStrategy applies this configuration to the given DefaultMergeStrategy options.
	ApplyToDefaultMergeStrategy(option *DefaultMergeStrategyOptions)
}

// DefaultMergeStrategyOptions allows to set options for the DefaultMergeStrategy behaviour.
type DefaultMergeStrategyOptions struct {
	getPriorityFunc                    func(condition metav1.Condition) MergePriority
	targetConditionHasPositivePolarity bool
	computeReasonFunc                  func(issueConditions []ConditionWithOwnerInfo, unknownConditions []ConditionWithOwnerInfo, infoConditions []ConditionWithOwnerInfo) string
	summaryMessageTransformFunc        func([]string) []string
}

// ApplyOptions applies the given list options on these options,
// and then returns itself (for convenient chaining).
func (o *DefaultMergeStrategyOptions) ApplyOptions(opts []DefaultMergeStrategyOption) *DefaultMergeStrategyOptions {
	for _, opt := range opts {
		opt.ApplyToDefaultMergeStrategy(o)
	}
	return o
}

// DefaultMergeStrategy returns the default merge strategy.
//
// Use the GetPriorityFunc option to customize how the MergePriority for a given condition is computed.
// If not specified, conditions are considered issues or not if not to their normal state given the polarity
// (e.g. a positive polarity condition is considered to be reporting an issue when status is false,
// otherwise the condition is considered to be reporting an info unless status is unknown).
//
// Use the TargetConditionHasPositivePolarity to define the polarity of the condition returned by the DefaultMergeStrategy.
// If not specified, the generate condition will have positive polarity (status true = good).
//
// Use the ComputeReasonFunc to customize how the reason for the resulting condition will be computed.
// If not specified, generic reasons will be used.
func DefaultMergeStrategy(opts ...DefaultMergeStrategyOption) MergeStrategy {
	strategyOpt := &DefaultMergeStrategyOptions{
		targetConditionHasPositivePolarity: true,
		computeReasonFunc:                  GetDefaultComputeMergeReasonFunc(issuesReportedReason, unknownReportedReason, infoReportedReason), // NOTE: when no specific reason are provided, generic ones are used.
		getPriorityFunc:                    GetDefaultMergePriorityFunc(),
		summaryMessageTransformFunc:        nil,
	}
	strategyOpt.ApplyOptions(opts)

	return &defaultMergeStrategy{
		getPriorityFunc:                    strategyOpt.getPriorityFunc,
		computeReasonFunc:                  strategyOpt.computeReasonFunc,
		targetConditionHasPositivePolarity: strategyOpt.targetConditionHasPositivePolarity,
		summaryMessageTransformFunc:        strategyOpt.summaryMessageTransformFunc,
	}
}

// GetDefaultMergePriorityFunc returns the merge priority for each condition.
// It assigns following priority values to conditions:
// - issues: conditions with positive polarity (normal True) and status False or conditions with negative polarity (normal False) and status True.
// - unknown: conditions with status unknown.
// - info: conditions with positive polarity (normal True) and status True or conditions with negative polarity (normal False) and status False.
func GetDefaultMergePriorityFunc(negativePolarityConditionTypes ...string) func(condition metav1.Condition) MergePriority {
	negativePolarityConditionTypesSet := sets.New[string](negativePolarityConditionTypes...)
	return func(condition metav1.Condition) MergePriority {
		switch condition.Status {
		case metav1.ConditionTrue:
			if negativePolarityConditionTypesSet.Has(condition.Type) {
				return IssueMergePriority
			}
			return InfoMergePriority
		case metav1.ConditionFalse:
			if negativePolarityConditionTypesSet.Has(condition.Type) {
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

// GetDefaultComputeMergeReasonFunc return a function picking one of the three reasons in input depending on
// the status of the conditions being merged.
func GetDefaultComputeMergeReasonFunc(issueReason, unknownReason, infoReason string) func(issueConditions []ConditionWithOwnerInfo, unknownConditions []ConditionWithOwnerInfo, infoConditions []ConditionWithOwnerInfo) string {
	return func(issueConditions []ConditionWithOwnerInfo, unknownConditions []ConditionWithOwnerInfo, _ []ConditionWithOwnerInfo) string {
		switch {
		case len(issueConditions) > 0:
			return issueReason
		case len(unknownConditions) > 0:
			return unknownReason
		default:
			// Note: This func can assume that there is at least one condition, so this branch is equivalent to len(infoReason) > 0,
			// and it makes the linter happy.
			return infoReason
		}
	}
}

const (
	// issuesReportedReason is set on conditions generated during aggregate or summary operations when at least one conditions/objects are reporting issues.
	// NOTE: This const is used by GetDefaultComputeMergeReasonFunc if no specific reasons are provided.
	issuesReportedReason = "IssuesReported"

	// unknownReportedReason is set on conditions generated during aggregate or summary operations when at least one conditions/objects are reporting unknown.
	// NOTE: This const is used by GetDefaultComputeMergeReasonFunc if no specific reasons are provided.
	unknownReportedReason = "UnknownReported"

	// infoReportedReason is set on conditions generated during aggregate or summary operations when at least one conditions/objects are reporting info.
	// NOTE: This const is used by GetDefaultComputeMergeReasonFunc if no specific reasons are provided.
	infoReportedReason = "InfoReported"
)

// defaultMergeStrategy defines the default merge strategy for Cluster API conditions.
type defaultMergeStrategy struct {
	getPriorityFunc                    func(condition metav1.Condition) MergePriority
	targetConditionHasPositivePolarity bool
	computeReasonFunc                  func(issueConditions []ConditionWithOwnerInfo, unknownConditions []ConditionWithOwnerInfo, infoConditions []ConditionWithOwnerInfo) string
	summaryMessageTransformFunc        func([]string) []string
}

// Merge all conditions in input based on a strategy that surfaces issues first, then unknown conditions, then info (if none of issues and unknown condition exists).
// - issues: conditions with positive polarity (normal True) and status False or conditions with negative polarity (normal False) and status True.
// - unknown: conditions with status unknown.
// - info: conditions with positive polarity (normal True) and status True or conditions with negative polarity (normal False) and status False.
func (d *defaultMergeStrategy) Merge(operation MergeOperation, conditions []ConditionWithOwnerInfo, conditionTypes []string) (status metav1.ConditionStatus, reason, message string, err error) {
	if len(conditions) == 0 {
		return "", "", "", errors.New("can't merge an empty list of conditions")
	}

	if d.getPriorityFunc == nil {
		return "", "", "", errors.New("can't merge without a getPriority func")
	}

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
		if d.targetConditionHasPositivePolarity {
			status = metav1.ConditionFalse
		} else {
			status = metav1.ConditionTrue
		}
	case len(unknownConditions) > 0:
		status = metav1.ConditionUnknown
	case len(infoConditions) > 0:
		if d.targetConditionHasPositivePolarity {
			status = metav1.ConditionTrue
		} else {
			status = metav1.ConditionFalse
		}
	default:
		// NOTE: this is already handled above, but repeating also here for better readability.
		return "", "", "", errors.New("can't merge an empty list of conditions")
	}

	// Compute the reason for the target condition:
	// - In case there is only one condition in the top group, use the reason from this condition
	// - In case there are more than one condition in the top group, use a generic reason (for the target group)
	reason = d.computeReasonFunc(issueConditions, unknownConditions, infoConditions)

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
	if operation == SummaryMergeOperation {
		message = summaryMessage(conditions, d, status)
	}

	// When performing the aggregate operation, we are merging one single condition from potentially many objects.
	// Considering the high number of conditions involved, the messages from the conditions being merged must be filtered/summarized
	// using rules designed to surface the most important issues.
	//
	// Accordingly, the resulting message is composed by only three messages from conditions classified as issues/unknown;
	// instead three messages from conditions classified as info are included only if there are no issues/unknown.
	//
	// Three criteria are used to pick the messages to be shown
	// - Messages for control plane machines always go first
	// - Messages for issues always go before messages for unknown, info messages goes last
	// - The number of objects reporting the same message determine the order used to pick within the messages in the same bucket
	//
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
	if operation == AggregateMergeOperation {
		n := 3
		messages := []string{}

		// Get max n issue/unknown messages, decrement n, and track if there are other objects reporting issues/unknown not included in the messages.
		if len(issueConditions) > 0 || len(unknownConditions) > 0 {
			issueMessages := aggregateMessages(append(issueConditions, unknownConditions...), &n, false, d.getPriorityFunc, map[MergePriority]string{IssueMergePriority: "with other issues", UnknownMergePriority: "with status unknown"})
			messages = append(messages, issueMessages...)
		}

		// Only if there are no issue or unknown,
		// Get max n info messages, decrement n, and track if there are other objects reporting info not included in the messages.
		if len(issueConditions) == 0 && len(unknownConditions) == 0 && len(infoConditions) > 0 {
			infoMessages := aggregateMessages(infoConditions, &n, true, d.getPriorityFunc, map[MergePriority]string{InfoMergePriority: "with additional info"})
			messages = append(messages, infoMessages...)
		}

		message = strings.Join(messages, "\n")
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

// summaryMessage returns message for the summary operation.
func summaryMessage(conditions []ConditionWithOwnerInfo, d *defaultMergeStrategy, status metav1.ConditionStatus) string {
	messages := []string{}

	// Note: use conditions because we want to preserve the order of relevance defined by the users (the order of condition types).
	for _, condition := range conditions {
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

		m := fmt.Sprintf("* %s:", condition.Type)
		if condition.Message != "" {
			m += indentIfMultiline(condition.Message)
		} else {
			m += fmt.Sprintf(" %s", condition.Reason)
		}
		messages = append(messages, m)
	}

	if d.summaryMessageTransformFunc != nil {
		messages = d.summaryMessageTransformFunc(messages)
	}

	return strings.Join(messages, "\n")
}

// aggregateMessages returns messages for the aggregate operation.
func aggregateMessages(conditions []ConditionWithOwnerInfo, n *int, dropEmpty bool, getPriority func(condition metav1.Condition) MergePriority, otherMessages map[MergePriority]string) (messages []string) {
	// create a map with all the messages and the list of objects reporting the same message.
	messageObjMap := map[string]map[string][]string{}
	messagePriorityMap := map[string]MergePriority{}
	messageMustGoFirst := map[string]bool{}
	cpMachines := sets.Set[string]{}
	for _, condition := range conditions {
		if dropEmpty && condition.Message == "" {
			continue
		}

		// Keep track of the message and the list of objects it applies to.
		m := condition.Message
		if _, ok := messageObjMap[condition.OwnerResource.Kind]; !ok {
			messageObjMap[condition.OwnerResource.Kind] = map[string][]string{}
		}
		messageObjMap[condition.OwnerResource.Kind][m] = append(messageObjMap[condition.OwnerResource.Kind][m], condition.OwnerResource.Name)

		// Keep track of CP machines
		if condition.OwnerResource.IsControlPlaneMachine {
			cpMachines.Insert(condition.OwnerResource.Name)
		}

		// Keep track of the priority of the message.
		// In case the same message exists with different priorities, the highest according to issue/unknown/info applies.
		currentPriority, ok := messagePriorityMap[m]
		newPriority := getPriority(condition.Condition)
		switch {
		case !ok:
			messagePriorityMap[m] = newPriority
		case currentPriority == IssueMergePriority:
			// No-op, issue is already the highest priority.
		case currentPriority == UnknownMergePriority:
			// If current priority is unknown, use new one only if higher.
			if newPriority == IssueMergePriority {
				messagePriorityMap[m] = newPriority
			}
		case currentPriority == InfoMergePriority:
			// if current priority is info, new one can be equal or higher, use it.
			messagePriorityMap[m] = newPriority
		}

		// Keep track if this message belongs to control plane machines, and thus it should go first.
		// Note: it is enough that on object is a control plane machine to move the message as first.
		first, ok := messageMustGoFirst[m]
		if !ok || !first {
			if condition.OwnerResource.IsControlPlaneMachine {
				messageMustGoFirst[m] = true
			}
		}
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

		// compute the order of messages according to:
		// - message should go first (e.g. it applies to a control plane machine)
		// - message priority (e.g. first issues, then unknown)
		// - the number of objects reporting the same message.
		// Note: The list of object names is used as a secondary criteria to sort messages with the same number of objects.
		messageIndex := make([]string, 0, len(messageObjMapForKind))
		for m := range messageObjMapForKind {
			messageIndex = append(messageIndex, m)
		}

		sort.SliceStable(messageIndex, func(i, j int) bool {
			return sortMessage(messageIndex[i], messageIndex[j], messageMustGoFirst, messagePriorityMap, messageObjMapForKind)
		})

		// Pick the first n messages, decrement n.
		// For each message, add up to three objects; if more add the number of the remaining objects with the same message.
		// Count the number of objects reporting messages not included in the above.
		// Note: we are showing up to three objects because usually control plane has 3 machines, and we want to show all issues
		// to control plane machines if any,
		others := map[MergePriority]int{}
		for _, m := range messageIndex {
			if *n == 0 {
				others[messagePriorityMap[m]] += len(messageObjMapForKind[m])
				continue
			}

			msg := ""
			allObjects := messageObjMapForKind[m]
			sort.Slice(allObjects, func(i, j int) bool {
				return sortObj(allObjects[i], allObjects[j], cpMachines)
			})
			switch {
			case len(allObjects) == 0:
				// This should never happen, entry in the map exists only when an object reports a message.
			case len(allObjects) == 1:
				msg += fmt.Sprintf("* %s %s:", kind, strings.Join(allObjects, ", "))
			case len(allObjects) <= 3:
				msg += fmt.Sprintf("* %s %s:", kindPlural, strings.Join(allObjects, ", "))
			default:
				msg += fmt.Sprintf("* %s %s, ... (%d more):", kindPlural, strings.Join(allObjects[:3], ", "), len(allObjects)-3)
			}
			msg += indentIfMultiline(m)

			messages = append(messages, msg)
			*n--
		}

		for _, p := range []MergePriority{IssueMergePriority, UnknownMergePriority, InfoMergePriority} {
			other, ok := others[p]
			if !ok {
				continue
			}

			otherMessage, ok := otherMessages[p]
			if !ok {
				continue
			}
			if other == 1 {
				messages = append(messages, fmt.Sprintf("And %d %s %s", other, kind, otherMessage))
			}
			if other > 1 {
				messages = append(messages, fmt.Sprintf("And %d %s %s", other, kindPlural, otherMessage))
			}
		}
	}

	return messages
}

func sortMessage(i, j string, messageMustGoFirst map[string]bool, messagePriorityMap map[string]MergePriority, messageObjMapForKind map[string][]string) bool {
	if messageMustGoFirst[i] && !messageMustGoFirst[j] {
		return true
	}
	if !messageMustGoFirst[i] && messageMustGoFirst[j] {
		return false
	}

	if messagePriorityMap[i] < messagePriorityMap[j] {
		return true
	}
	if messagePriorityMap[i] > messagePriorityMap[j] {
		return false
	}

	if len(messageObjMapForKind[i]) > len(messageObjMapForKind[j]) {
		return true
	}
	if len(messageObjMapForKind[i]) < len(messageObjMapForKind[j]) {
		return false
	}

	return strings.Join(messageObjMapForKind[i], ",") < strings.Join(messageObjMapForKind[j], ",")
}

func sortObj(i, j string, cpMachines sets.Set[string]) bool {
	if cpMachines.Has(i) && !cpMachines.Has(j) {
		return true
	}
	if !cpMachines.Has(i) && cpMachines.Has(j) {
		return false
	}
	return i < j
}

var re = regexp.MustCompile(`\s*\*\s+`)

func indentIfMultiline(m string) string {
	msg := ""
	// If it is a multiline string or if it start with a bullet, indent the message.
	if strings.Contains(m, "\n") || re.MatchString(m) {
		msg += "\n"

		// Split the message in lines, and add a prefix; prefix can be
		// "  " when indenting a line starting in a bullet
		// "  * " when indenting a line starting without a bullet (indent + add a bullet)
		// "    " when indenting a line starting with a bullet, but other lines required adding a bullet
		lines := strings.Split(m, "\n")
		prefix := "  "
		hasLinesWithoutBullet := false
		for i := range lines {
			if !re.MatchString(lines[i]) {
				hasLinesWithoutBullet = true
				break
			}
		}
		for i, l := range lines {
			prefix := prefix
			if hasLinesWithoutBullet {
				if !re.MatchString(lines[i]) {
					prefix += "* "
				} else {
					prefix += "  "
				}
			}
			lines[i] = prefix + l
		}
		msg += strings.Join(lines, "\n")
	} else {
		msg += " " + m
	}
	return msg
}

// getConditionsWithOwnerInfo return all the conditions from an object each one with the corresponding ConditionOwnerInfo.
func getConditionsWithOwnerInfo(obj Getter) []ConditionWithOwnerInfo {
	ret := make([]ConditionWithOwnerInfo, 0, 10)
	conditions := obj.GetConditions()
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
	var isControlPlaneMachine bool
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

	if objMeta, ok := obj.(objectWithNameAndLabels); ok {
		name = objMeta.GetName()
		if kind == "Machine" {
			_, isControlPlaneMachine = objMeta.GetLabels()[clusterv1.MachineControlPlaneLabel]
		}
	}

	return ConditionOwnerInfo{
		Kind:                  kind,
		Name:                  name,
		IsControlPlaneMachine: isControlPlaneMachine,
	}
}

// objectWithNameAndLabels is a subset of metav1.Object.
type objectWithNameAndLabels interface {
	GetName() string
	GetLabels() map[string]string
}
