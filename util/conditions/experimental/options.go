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

// ConditionFields defines the path where conditions are defined in Unstructured objects.
type ConditionFields []string

// ApplyToSet applies this configuration to the given Set options.
func (f ConditionFields) ApplyToSet(opts *SetOptions) {
	opts.conditionsFields = f
}

// TODO: Think about using Less directly instead of defining a new option (might be only useful for the UX).

// ConditionSortFunc defines the sort order when conditions are assigned to an object.
type ConditionSortFunc Less

// ApplyToSet applies this configuration to the given Set options.
func (f ConditionSortFunc) ApplyToSet(opts *SetOptions) {
	opts.less = Less(f)
}

// OverrideType allows to override the type of new mirror or aggregate conditions.
type OverrideType string

// ApplyToMirror applies this configuration to the given mirrorInto options.
func (t OverrideType) ApplyToMirror(opts *MirrorOptions) {
	opts.overrideType = string(t)
}

// ApplyToAggregate applies this configuration to the given aggregate options.
func (t OverrideType) ApplyToAggregate(opts *AggregateOptions) {
	opts.overrideType = string(t)
}

// ForConditionTypes allows to define the set of conditions in scope for a summary operation.
// Please note that condition types have an implicit order that can be used by the summary operation to determine relevance of the different conditions.
type ForConditionTypes []string

// ApplyToSummary applies this configuration to the given summary options.
func (t ForConditionTypes) ApplyToSummary(opts *SummaryOptions) {
	opts.conditionTypes = t
}

// WithNegativeConditionTypes allows to define polarity for some of the conditions in scope for a summary operation.
type WithNegativeConditionTypes []string

// ApplyToSummary applies this configuration to the given summary options.
func (t WithNegativeConditionTypes) ApplyToSummary(opts *SummaryOptions) {
	opts.negativePolarityConditionTypes = t
}

// WithMergeStrategy allows to define a custom merge strategy when creating new summary or aggregate conditions.
type WithMergeStrategy struct {
	MergeStrategy
}

// ApplyToSummary applies this configuration to the given summary options.
func (t WithMergeStrategy) ApplyToSummary(opts *SummaryOptions) {
	opts.mergeStrategy = t
}

// ApplyToAggregate applies this configuration to the given aggregate options.
func (t WithMergeStrategy) ApplyToAggregate(opts *AggregateOptions) {
	opts.mergeStrategy = t
}

// WithStepCounter adds a step counter message to new summary conditions.
type WithStepCounter bool

// ApplyToSummary applies this configuration to the given summary options.
func (t WithStepCounter) ApplyToSummary(opts *SummaryOptions) {
	opts.stepCounter = bool(t)
}
