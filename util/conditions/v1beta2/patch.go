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
	"reflect"
	"sort"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/cluster-api/util"
)

// Patch defines a list of operations to change a list of conditions into another.
type Patch []PatchOperation

// PatchOperation defines an operation that changes a single condition.
type PatchOperation struct {
	Before *metav1.Condition
	After  *metav1.Condition
	Op     PatchOperationType
}

// PatchOperationType defines a condition patch operation type.
type PatchOperationType string

const (
	// AddConditionPatch defines an add condition patch operation.
	AddConditionPatch PatchOperationType = "Add"

	// ChangeConditionPatch defines an change condition patch operation.
	ChangeConditionPatch PatchOperationType = "Change"

	// RemoveConditionPatch defines a remove condition patch operation.
	RemoveConditionPatch PatchOperationType = "Remove"
)

// NewPatch returns the Patch required to align source conditions to after conditions.
func NewPatch(before, after Getter) (Patch, error) {
	var patch Patch

	if util.IsNil(before) {
		return nil, errors.New("error creating patch: before object is nil")
	}
	beforeConditions := before.GetV1Beta2Conditions()

	if util.IsNil(after) {
		return nil, errors.New("error creating patch: after object is nil")
	}
	afterConditions := after.GetV1Beta2Conditions()

	// Identify AddCondition and ModifyCondition changes.
	for i := range afterConditions {
		afterCondition := afterConditions[i]
		beforeCondition := meta.FindStatusCondition(beforeConditions, afterCondition.Type)
		if beforeCondition == nil {
			patch = append(patch, PatchOperation{Op: AddConditionPatch, After: &afterCondition})
			continue
		}

		if !reflect.DeepEqual(&afterCondition, beforeCondition) {
			patch = append(patch, PatchOperation{Op: ChangeConditionPatch, After: &afterCondition, Before: beforeCondition})
		}
	}

	// Identify RemoveCondition changes.
	for i := range beforeConditions {
		beforeCondition := beforeConditions[i]
		afterCondition := meta.FindStatusCondition(afterConditions, beforeCondition.Type)
		if afterCondition == nil {
			patch = append(patch, PatchOperation{Op: RemoveConditionPatch, Before: &beforeCondition})
		}
	}
	return patch, nil
}

// PatchApplyOption is some configuration that modifies options for a patch apply call.
type PatchApplyOption interface {
	// ApplyToPatchApply applies this configuration to the given patch apply options.
	ApplyToPatchApply(option *PatchApplyOptions)
}

// PatchApplyOptions allows to set strategies for patch apply.
type PatchApplyOptions struct {
	ownedConditionTypes []string
	forceOverwrite      bool
	conditionSortFunc   ConditionSortFunc
}

// ApplyOptions applies the given list options on these options,
// and then returns itself (for convenient chaining).
func (o *PatchApplyOptions) ApplyOptions(opts []PatchApplyOption) *PatchApplyOptions {
	for _, opt := range opts {
		if util.IsNil(opt) {
			continue
		}
		opt.ApplyToPatchApply(o)
	}
	return o
}

func (o *PatchApplyOptions) isOwnedConditionType(conditionType string) bool {
	for _, i := range o.ownedConditionTypes {
		if i == conditionType {
			return true
		}
	}
	return false
}

// Apply executes a three-way merge of a list of Patch.
// When merge conflicts are detected (latest deviated from before in an incompatible way), an error is returned.
func (p Patch) Apply(latest Setter, opts ...PatchApplyOption) error {
	if p.IsZero() {
		return nil
	}

	if util.IsNil(latest) {
		return errors.New("error patching conditions: latest object is nil")
	}
	latestConditions := latest.GetV1Beta2Conditions()

	applyOpt := &PatchApplyOptions{
		// By default, sort conditions by the default condition order: available and ready always first, deleting and paused always last, all the other conditions in alphabetical order.
		conditionSortFunc: defaultSortLessFunc,
	}
	applyOpt.ApplyOptions(opts)

	for _, conditionPatch := range p {
		switch conditionPatch.Op {
		case AddConditionPatch:
			// If the condition is owned, always keep the after value.
			if applyOpt.forceOverwrite || applyOpt.isOwnedConditionType(conditionPatch.After.Type) {
				setStatusCondition(&latestConditions, *conditionPatch.After)
				continue
			}

			// If the condition is already on latest, check if latest and after agree on the change; if not, this is a conflict.
			if latestCondition := meta.FindStatusCondition(latestConditions, conditionPatch.After.Type); latestCondition != nil {
				// If latest and after disagree on the change, then it is a conflict
				if !HasSameState(latestCondition, conditionPatch.After) {
					return errors.Errorf("error patching conditions: The condition %q was modified by a different process and this caused a merge/AddCondition conflict: %v", conditionPatch.After.Type, cmp.Diff(latestCondition, conditionPatch.After))
				}
				// otherwise, the latest is already as intended.
				// NOTE: We are preserving LastTransitionTime from the latest in order to avoid altering the existing value.
				continue
			}
			// If the condition does not exists on the latest, add the new after condition.
			setStatusCondition(&latestConditions, *conditionPatch.After)

		case ChangeConditionPatch:
			// If the conditions is owned, always keep the after value.
			if applyOpt.forceOverwrite || applyOpt.isOwnedConditionType(conditionPatch.After.Type) {
				setStatusCondition(&latestConditions, *conditionPatch.After)
				continue
			}

			latestCondition := meta.FindStatusCondition(latestConditions, conditionPatch.After.Type)

			// If the condition does not exist anymore on the latest, this is a conflict.
			if latestCondition == nil {
				return errors.Errorf("error patching conditions: The condition %q was deleted by a different process and this caused a merge/ChangeCondition conflict", conditionPatch.After.Type)
			}

			// If the condition on the latest is different from the base condition, check if
			// the after state corresponds to the desired value. If not this is a conflict (unless we should ignore conflicts for this condition type).
			if !reflect.DeepEqual(latestCondition, conditionPatch.Before) {
				if !HasSameState(latestCondition, conditionPatch.After) {
					return errors.Errorf("error patching conditions: The condition %q was modified by a different process and this caused a merge/ChangeCondition conflict: %v", conditionPatch.After.Type, cmp.Diff(latestCondition, conditionPatch.After))
				}
				// Otherwise the latest is already as intended.
				// NOTE: We are preserving LastTransitionTime from the latest in order to avoid altering the existing value.
				continue
			}
			// Otherwise apply the new after condition.
			setStatusCondition(&latestConditions, *conditionPatch.After)

		case RemoveConditionPatch:
			// If latestConditions is nil or empty, nothing to remove.
			if len(latestConditions) == 0 {
				continue
			}

			// If the conditions is owned, always keep the after value (condition should be deleted).
			if applyOpt.forceOverwrite || applyOpt.isOwnedConditionType(conditionPatch.Before.Type) {
				meta.RemoveStatusCondition(&latestConditions, conditionPatch.Before.Type)
				continue
			}

			// If the condition is still on the latest, check if it is changed in the meantime;
			// if so then this is a conflict.
			if latestCondition := meta.FindStatusCondition(latestConditions, conditionPatch.Before.Type); latestCondition != nil {
				if !HasSameState(latestCondition, conditionPatch.Before) {
					return errors.Errorf("error patching conditions: The condition %q was modified by a different process and this caused a merge/RemoveCondition conflict: %v", conditionPatch.Before.Type, cmp.Diff(latestCondition, conditionPatch.Before))
				}
			}
			// Otherwise the latest and after agreed on the delete operation, so there's nothing to change.
			meta.RemoveStatusCondition(&latestConditions, conditionPatch.Before.Type)
		}
	}

	if applyOpt.conditionSortFunc != nil {
		sort.SliceStable(latestConditions, func(i, j int) bool {
			return applyOpt.conditionSortFunc(latestConditions[i], latestConditions[j])
		})
	}

	latest.SetV1Beta2Conditions(latestConditions)
	return nil
}

// IsZero returns true if the patch is nil or has no changes.
func (p Patch) IsZero() bool {
	if p == nil {
		return true
	}
	return len(p) == 0
}

// HasSameState returns true if a condition has the same state of another; state is defined
// by the union of following fields: Type, Status, Reason, ObservedGeneration and Message (it excludes LastTransitionTime).
func HasSameState(i, j *metav1.Condition) bool {
	return i.Type == j.Type &&
		i.Status == j.Status &&
		i.ObservedGeneration == j.ObservedGeneration &&
		i.Reason == j.Reason &&
		i.Message == j.Message
}
