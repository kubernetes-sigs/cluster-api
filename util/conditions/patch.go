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

// NewPatch returns the list of Patch required to align source conditions to after conditions.
func NewPatch(before Getter, after Getter) Patch {
	var patch Patch

	// Identify AddCondition and ModifyCondition changes.
	targetConditions := after.GetConditions()
	for i := range targetConditions {
		targetCondition := targetConditions[i]
		currentCondition := Get(before, targetCondition.Type)
		if currentCondition == nil {
			patch = append(patch, PatchOperation{Op: AddConditionPatch, Target: &targetCondition})
			continue
		}

		if !reflect.DeepEqual(&targetCondition, currentCondition) {
			patch = append(patch, PatchOperation{Op: ChangeConditionPatch, Target: &targetCondition, Base: currentCondition})
		}
	}

	// Identify RemoveCondition changes.
	baseConditions := before.GetConditions()
	for i := range baseConditions {
		baseCondition := baseConditions[i]
		targetCondition := Get(after, baseCondition.Type)
		if targetCondition == nil {
			patch = append(patch, PatchOperation{Op: RemoveConditionPatch, Base: &baseCondition})
		}
	}
	return patch
}

// Apply executes a three-way merge of a list of Patch.
// When merge conflicts are detected (latest deviated from before in an incompatible way), an error is returned.
func (p Patch) Apply(latest Setter) error {
	if len(p) == 0 {
		return nil
	}

	for _, conditionPatch := range p {
		switch conditionPatch.Op {
		case AddConditionPatch:
			// If the condition is already on latest, check if latest and after agree on the change; if not, this is a conflict.
			if sourceCondition := Get(latest, conditionPatch.Target.Type); sourceCondition != nil {
				// If latest and after agree on the change, then it is a conflict.
				if !hasSameState(sourceCondition, conditionPatch.Target) {
					return errors.Errorf("error patching conditions: The condition %q on was modified by a different process and this caused a merge/AddCondition conflict", conditionPatch.Target.Type)
				}
				// otherwise, the latest is already as intended.
				// NOTE: We are preserving LastTransitionTime from the latest in order to avoid altering the existing value.
				continue
			}
			// If the condition does not exists on the latest, add the new after condition.
			Set(latest, conditionPatch.Target)

		case ChangeConditionPatch:
			sourceCondition := Get(latest, conditionPatch.Target.Type)

			// If the condition does not exist anymore on the latest, this is a conflict.
			if sourceCondition == nil {
				return errors.Errorf("error patching conditions: The condition %q on was deleted by a different process and this caused a merge/ChangeCondition conflict", conditionPatch.Target.Type)
			}

			// If the condition on the latest is different from the base condition, check if
			// the after state corresponds to the desired value. If not this is a conflict.
			if !reflect.DeepEqual(sourceCondition, conditionPatch.Base) {
				if !hasSameState(sourceCondition, conditionPatch.Target) {
					return errors.Errorf("error patching conditions: The condition %q on was modified by a different process and this caused a merge/ChangeCondition conflict", conditionPatch.Target.Type)
				}
				// Otherwise the latest is already as intended.
				// NOTE: We are preserving LastTransitionTime from the latest in order to avoid altering the existing value.
				continue
			}

			// Otherwise apply the new after condition.
			Set(latest, conditionPatch.Target)

		case RemoveConditionPatch:
			// If the condition is still on the latest, check if it is changed in the meantime;
			// if so then this is a conflict.
			if sourceCondition := Get(latest, conditionPatch.Base.Type); sourceCondition != nil {
				if !hasSameState(sourceCondition, conditionPatch.Base) {
					return errors.Errorf("error patching conditions: The condition %q on was modified by a different process and this caused a merge/RemoveCondition conflict", conditionPatch.Base.Type)
				}
			}
			// Otherwise the latest and after agreed on the delete operation, so there's nothing to change.
			Delete(latest, conditionPatch.Base.Type)
		}
	}
	return nil
}

// IsZero returns true if the patch has no changes.
func (p Patch) IsZero() bool {
	return len(p) == 0
}
