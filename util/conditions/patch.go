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
	"reflect"

	"github.com/pkg/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

// Patch defines a list of operations to change a list of conditions into another
type Patch []PatchOperation

// PatchOperation define an operation that changes a single condition.
type PatchOperation struct {
	Target *clusterv1.Condition
	Base   *clusterv1.Condition
	Op     PatchOperationType
}

// PatchOperationType defines patch operation types.
type PatchOperationType string

const (
	// AddConditionPatch defines an add condition patch operation.
	AddConditionPatch PatchOperationType = "Add"

	// ChangeConditionPatch defines an change condition patch operation.
	ChangeConditionPatch PatchOperationType = "Change"

	// RemoveConditionPatch defines a remove condition patch operation.
	RemoveConditionPatch PatchOperationType = "Remove"
)

// NewPatch returns the list of Patch required to align source conditions to target conditions.
func NewPatch(base Getter, target Getter) Patch {
	var patch Patch

	// Identify AddCondition and ModifyCondition changes.
	targetConditions := target.GetConditions()
	for i := range targetConditions {
		targetCondition := targetConditions[i]
		currentCondition := Get(base, targetCondition.Type)
		if currentCondition == nil {
			patch = append(patch, PatchOperation{Op: AddConditionPatch, Target: &targetCondition})
			continue
		}

		if !reflect.DeepEqual(&targetCondition, currentCondition) {
			patch = append(patch, PatchOperation{Op: ChangeConditionPatch, Target: &targetCondition, Base: currentCondition})
		}
	}

	// Identify RemoveCondition changes.
	baseConditions := base.GetConditions()
	for i := range baseConditions {
		baseCondition := baseConditions[i]
		targetCondition := Get(target, baseCondition.Type)
		if targetCondition == nil {
			patch = append(patch, PatchOperation{Op: RemoveConditionPatch, Base: &baseCondition})
		}
	}
	return patch
}

// Apply executes a three-way merge of a list of Patch.
// When merge conflicts are detected (the source deviated from the original base recorded when creating the patch
// in an incompatible way), an error is returned.
func (p Patch) Apply(source Setter) error {
	if len(p) == 0 {
		return nil
	}

	for _, conditionPatch := range p {
		switch conditionPatch.Op {
		case AddConditionPatch:
			// If the condition is already on source, check if source and target agree on the change;
			// if not, this is a conflict.
			if sourceCondition := Get(source, conditionPatch.Target.Type); sourceCondition != nil {
				// If source and target agree on the change, then it is a conflict.
				if !hasSameState(sourceCondition, conditionPatch.Target) {
					return errors.Errorf("error patching conditions: The condition %q on was modified by a different process and this caused a merge/AddCondition conflict", conditionPatch.Target.Type)
				}
				// otherwise, the source is already as intended.
				// NOTE: We are preserving LastTransitionTime from the source in order to avoid altering the existing value.
				// XXX Set(source, sourceCondition)
				continue
			}
			// If the condition does not exists on the source, add the new target condition.
			Set(source, conditionPatch.Target)

		case ChangeConditionPatch:
			sourceCondition := Get(source, conditionPatch.Target.Type)

			// If the condition does not exist anymore on the source, this is a conflict.
			if sourceCondition == nil {
				return errors.Errorf("error patching conditions: The condition %q on was deleted by a different process and this caused a merge/ChangeCondition conflict", conditionPatch.Target.Type)
			}

			// If the condition on the source is different from the base condition, check if
			// the target state corresponds to the desired value. If not this is a conflict.
			if !reflect.DeepEqual(sourceCondition, conditionPatch.Base) {
				if !hasSameState(sourceCondition, conditionPatch.Target) {
					return errors.Errorf("error patching conditions: The condition %q on was modified by a different process and this caused a merge/ChangeCondition conflict", conditionPatch.Target.Type)
				}
				// Otherwise the source is already as intended.
				// NOTE: We are preserving LastTransitionTime from the source in order to avoid altering the existing value.
				// XXX Set(source, sourceCondition)
				continue
			}

			// Otherwise apply the new target condition.
			Set(source, conditionPatch.Target)

		case RemoveConditionPatch:
			// If the condition is still on the source, check if it is changed in the meantime;
			// if so then this is a conflict.
			if sourceCondition := Get(source, conditionPatch.Base.Type); sourceCondition != nil {
				if !hasSameState(sourceCondition, conditionPatch.Base) {
					return errors.Errorf("error patching conditions: The condition %q on was modified by a different process and this caused a merge/RemoveCondition conflict", conditionPatch.Base.Type)
				}
			}
			// Otherwise the source and target agreed on the delete operation, so there's nothing to change.
			Delete(source, conditionPatch.Base.Type)
		}
	}
	return nil
}
