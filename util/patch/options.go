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

package patch

import clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

// Option is some configuration that modifies options for a patch request.
type Option interface {
	// ApplyToHelper applies this configuration to the given Helper options.
	ApplyToHelper(*HelperOptions)
}

// HelperOptions contains options for patch options.
type HelperOptions struct {
	// IncludeStatusObservedGeneration sets the status.observedGeneration field
	// on the incoming object to match metadata.generation, only if there is a change.
	IncludeStatusObservedGeneration bool

	// ForceOverwriteConditions allows the patch helper to overwrite conditions in case of conflicts.
	// This option should only ever be set in controller managing the object being patched.
	ForceOverwriteConditions bool

	// OwnedConditions defines condition types owned by the controller.
	// In case of conflicts for the owned conditions, the patch helper will always use the value provided by the controller.
	OwnedConditions []clusterv1.ConditionType

	// OwnedV1Beta2Conditions defines condition types owned by the controller.
	// In case of conflicts for the owned conditions, the patch helper will always use the value provided by the controller.
	OwnedV1Beta2Conditions []string

	// Metav1ConditionsFields allows to override the path for the field hosting []metav1.Condition.
	// Please note that the default value for this option is inferred from the object struct.
	// This means, that if the correct path cannot be detected, this option has to be specified. One example
	// is if you pass a wrapper to unstructured.
	// The override for this option is considered only if the object implements the conditions.Setter interface.
	Metav1ConditionsFieldPath []string

	// Clusterv1ConditionsFieldPath allows to override the path for the field hosting clusterv1.Conditions.
	// Please note that the default value for this option is inferred from the object struct.
	// This means, that if the correct path cannot be detected, this option has to be specified. One example
	// is if you pass a wrapper to unstructured.
	// The override for this option is considered only if the object implements the conditions.Setter interface.
	Clusterv1ConditionsFieldPath []string
}

// WithForceOverwriteConditions allows the patch helper to overwrite conditions in case of conflicts.
// This option should only ever be set in controller managing the object being patched.
type WithForceOverwriteConditions struct{}

// ApplyToHelper applies this configuration to the given HelperOptions.
func (w WithForceOverwriteConditions) ApplyToHelper(in *HelperOptions) {
	in.ForceOverwriteConditions = true
}

// WithStatusObservedGeneration sets the status.observedGeneration field
// on the incoming object to match metadata.generation, only if there is a change.
type WithStatusObservedGeneration struct{}

// ApplyToHelper applies this configuration to the given HelperOptions.
func (w WithStatusObservedGeneration) ApplyToHelper(in *HelperOptions) {
	in.IncludeStatusObservedGeneration = true
}

// WithOwnedV1Beta1Conditions allows to define condition types owned by the controller.
// In case of conflicts for the owned conditions, the patch helper will always use the value provided by the controller.
type WithOwnedV1Beta1Conditions struct {
	Conditions []clusterv1.ConditionType
}

// ApplyToHelper applies this configuration to the given HelperOptions.
func (w WithOwnedV1Beta1Conditions) ApplyToHelper(in *HelperOptions) {
	in.OwnedConditions = w.Conditions
}

// WithOwnedConditions allows to define condition types owned by the controller.
// In case of conflicts for the owned conditions, the patch helper will always use the value provided by the controller.
type WithOwnedConditions struct {
	Conditions []string
}

// ApplyToHelper applies this configuration to the given HelperOptions.
func (w WithOwnedConditions) ApplyToHelper(in *HelperOptions) {
	in.OwnedV1Beta2Conditions = w.Conditions
}

// Metav1ConditionsFieldPath allows to override the path for the field hosting []metav1.Condition.
// Please note that the default value for this option is inferred from the object struct.
// The override for this option is considered only if the object implements the conditions.Setter interface.
type Metav1ConditionsFieldPath []string

// ApplyToHelper applies this configuration to the given HelperOptions.
func (w Metav1ConditionsFieldPath) ApplyToHelper(in *HelperOptions) {
	in.Metav1ConditionsFieldPath = w
}

// Clusterv1ConditionsFieldPath allows to override the path for the field hosting clusterv1.Conditions.
// Please note that the default value for this option is inferred from the object struct.
// The override for this option is considered only if the object implements the conditions.Setter interface.
type Clusterv1ConditionsFieldPath []string

// ApplyToHelper applies this configuration to the given HelperOptions.
func (w Clusterv1ConditionsFieldPath) ApplyToHelper(in *HelperOptions) {
	in.Clusterv1ConditionsFieldPath = w
}
