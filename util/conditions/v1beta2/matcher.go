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

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IgnoreLastTransitionTime instructs MatchConditions and MatchCondition to ignore the LastTransitionTime field.
type IgnoreLastTransitionTime bool

// ApplyMatch applies this configuration to the given Match options.
func (f IgnoreLastTransitionTime) ApplyMatch(opts *MatchOptions) {
	opts.ignoreLastTransitionTime = bool(f)
}

// MatchOption is some configuration that modifies options for a match call.
type MatchOption interface {
	// ApplyMatch applies this configuration to the given match options.
	ApplyMatch(option *MatchOptions)
}

// MatchOptions allows to set options for the match operation.
type MatchOptions struct {
	ignoreLastTransitionTime bool
}

// ApplyOptions applies the given list options on these options,
// and then returns itself (for convenient chaining).
func (o *MatchOptions) ApplyOptions(opts []MatchOption) *MatchOptions {
	for _, opt := range opts {
		opt.ApplyMatch(o)
	}
	return o
}

// MatchConditions returns a custom matcher to check equality of []metav1.Condition.
func MatchConditions(expected []metav1.Condition, opts ...MatchOption) types.GomegaMatcher {
	return &matchConditions{
		opts:     opts,
		expected: expected,
	}
}

type matchConditions struct {
	opts     []MatchOption
	expected []metav1.Condition
}

func (m matchConditions) Match(actual interface{}) (success bool, err error) {
	elems := []interface{}{}
	for _, condition := range m.expected {
		elems = append(elems, MatchCondition(condition, m.opts...))
	}

	return gomega.ConsistOf(elems...).Match(actual)
}

func (m matchConditions) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("expected\n\t%#v\nto match\n\t%#v\n", actual, m.expected)
}

func (m matchConditions) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("expected\n\t%#v\nto not match\n\t%#v\n", actual, m.expected)
}

// MatchCondition returns a custom matcher to check equality of metav1.Condition.
func MatchCondition(expected metav1.Condition, opts ...MatchOption) types.GomegaMatcher {
	return &matchCondition{
		opts:     opts,
		expected: expected,
	}
}

type matchCondition struct {
	opts     []MatchOption
	expected metav1.Condition
}

func (m matchCondition) Match(actual interface{}) (success bool, err error) {
	matchOpt := &MatchOptions{
		ignoreLastTransitionTime: false,
	}
	matchOpt.ApplyOptions(m.opts)

	actualCondition, ok := actual.(metav1.Condition)
	if !ok {
		return false, fmt.Errorf("actual should be of type metav1.Condition")
	}

	ok, err = gomega.Equal(m.expected.Type).Match(actualCondition.Type)
	if !ok {
		return ok, err
	}
	ok, err = gomega.Equal(m.expected.Status).Match(actualCondition.Status)
	if !ok {
		return ok, err
	}
	ok, err = gomega.Equal(m.expected.ObservedGeneration).Match(actualCondition.ObservedGeneration)
	if !ok {
		return ok, err
	}
	ok, err = gomega.Equal(m.expected.Reason).Match(actualCondition.Reason)
	if !ok {
		return ok, err
	}
	ok, err = gomega.Equal(m.expected.Message).Match(actualCondition.Message)
	if !ok {
		return ok, err
	}

	if !matchOpt.ignoreLastTransitionTime {
		ok, err = gomega.BeTemporally("==", m.expected.LastTransitionTime.Time).Match(actualCondition.LastTransitionTime.Time)
		if !ok {
			return ok, err
		}
	}

	return ok, err
}

func (m matchCondition) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("expected\n\t%#v\nto match\n\t%#v\n", actual, m.expected)
}

func (m matchCondition) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("expected\n\t%#v\nto not match\n\t%#v\n", actual, m.expected)
}
