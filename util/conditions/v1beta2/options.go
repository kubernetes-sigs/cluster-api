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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ConditionSortFunc defines the sort order when conditions are assigned to an object.
type ConditionSortFunc func(i, j metav1.Condition) bool

// ApplyToSet applies this configuration to the given Set options.
func (f ConditionSortFunc) ApplyToSet(opts *SetOptions) {
	opts.conditionSortFunc = f
}

// ApplyToPatchApply applies this configuration to the given patch apply options.
func (f ConditionSortFunc) ApplyToPatchApply(opts *PatchApplyOptions) {
	opts.conditionSortFunc = f
}

// TargetConditionType allows to specify the type of new mirror or aggregate conditions.
type TargetConditionType string

// ApplyToMirror applies this configuration to the given mirror options.
func (t TargetConditionType) ApplyToMirror(opts *MirrorOptions) {
	opts.targetConditionType = string(t)
}

// ApplyToAggregate applies this configuration to the given aggregate options.
func (t TargetConditionType) ApplyToAggregate(opts *AggregateOptions) {
	opts.targetConditionType = string(t)
}

// FallbackCondition defines the condition that should be returned by mirror if the source condition
// does not exist.
type FallbackCondition struct {
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

// ApplyToMirror applies this configuration to the given mirror options.
func (f FallbackCondition) ApplyToMirror(opts *MirrorOptions) {
	opts.fallbackStatus = f.Status
	opts.fallbackReason = f.Reason
	opts.fallbackMessage = f.Message
}

// ForConditionTypes allows to define the set of conditions in scope for a summary operation.
// Please note that condition types have an implicit order that can be used by the summary operation to determine relevance of the different conditions.
type ForConditionTypes []string

// ApplyToSummary applies this configuration to the given summary options.
func (t ForConditionTypes) ApplyToSummary(opts *SummaryOptions) {
	opts.conditionTypes = t
}

// NegativePolarityConditionTypes allows to define polarity for some of the conditions in scope for a summary operation.
type NegativePolarityConditionTypes []string

// ApplyToSummary applies this configuration to the given summary options.
func (t NegativePolarityConditionTypes) ApplyToSummary(opts *SummaryOptions) {
	opts.negativePolarityConditionTypes = t
}

// IgnoreTypesIfMissing allows to define conditions types that should be ignored (not defaulted to unknown) when performing a summary operation.
type IgnoreTypesIfMissing []string

// ApplyToSummary applies this configuration to the given summary options.
func (t IgnoreTypesIfMissing) ApplyToSummary(opts *SummaryOptions) {
	opts.ignoreTypesIfMissing = t
}

// OverrideConditions allows to override conditions read from the source object only for the scope of a summary operation.
// The condition on the source object will preserve the original value.
type OverrideConditions []ConditionWithOwnerInfo

// ApplyToSummary applies this configuration to the given summary options.
func (t OverrideConditions) ApplyToSummary(opts *SummaryOptions) {
	opts.overrideConditions = t
}

// CustomMergeStrategy allows to define a custom merge strategy when creating new summary or aggregate conditions.
type CustomMergeStrategy struct {
	MergeStrategy
}

// ApplyToSummary applies this configuration to the given summary options.
func (t CustomMergeStrategy) ApplyToSummary(opts *SummaryOptions) {
	opts.mergeStrategy = t
}

// ApplyToAggregate applies this configuration to the given aggregate options.
func (t CustomMergeStrategy) ApplyToAggregate(opts *AggregateOptions) {
	opts.mergeStrategy = t
}

// OwnedConditionTypes allows to define condition types owned by the controller when performing patch apply.
// In case of conflicts for the owned conditions, the patch helper will always use the value provided by the controller.
type OwnedConditionTypes []string

// ApplyToPatchApply applies this configuration to the given patch apply options.
func (o OwnedConditionTypes) ApplyToPatchApply(opts *PatchApplyOptions) {
	opts.ownedConditionTypes = o
}

// ForceOverwrite instructs patch apply to always use the value provided by the controller (no matter of what value exists currently).
type ForceOverwrite bool

// ApplyToPatchApply applies this configuration to the given patch apply options.
func (f ForceOverwrite) ApplyToPatchApply(opts *PatchApplyOptions) {
	opts.forceOverwrite = bool(f)
}
