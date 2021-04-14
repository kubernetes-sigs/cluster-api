/*
Copyright 2019 The Kubernetes Authors.

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

package controllers

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// hasMatchingLabels verifies that the Label Selector matches the given Labels.
func hasMatchingLabels(matchSelector metav1.LabelSelector, matchLabels map[string]string) bool {
	// This should never fail, validating webhook should catch this first
	selector, err := metav1.LabelSelectorAsSelector(&matchSelector)
	if err != nil {
		return false
	}
	// If a nil or empty selector creeps in, it should match nothing, not everything.
	if selector.Empty() {
		return false
	}
	if !selector.Matches(labels.Set(matchLabels)) {
		return false
	}
	return true
}
