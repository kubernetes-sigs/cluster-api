/*
Copyright 2023 The Kubernetes Authors.

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

// Package taints implements taint helper functions.
package taints

import (
	corev1 "k8s.io/api/core/v1"
)

// RemoveNodeTaint drops the taint from the list of node taints.
// It returns true if the taints are modified, false otherwise.
func RemoveNodeTaint(node *corev1.Node, drop corev1.Taint) bool {
	droppedTaint := false
	taints := []corev1.Taint{}
	for _, taint := range node.Spec.Taints {
		if taint.MatchTaint(&drop) {
			droppedTaint = true
			continue
		}
		taints = append(taints, taint)
	}
	node.Spec.Taints = taints
	return droppedTaint
}

// HasTaint returns true if the targetTaint is in the list of taints.
func HasTaint(taints []corev1.Taint, targetTaint corev1.Taint) bool {
	for _, taint := range taints {
		if taint.MatchTaint(&targetTaint) {
			return true
		}
	}
	return false
}

// EnsureNodeTaint makes sure the node has the Taint.
// It returns true if the taints are modified, false otherwise.
func EnsureNodeTaint(node *corev1.Node, taint corev1.Taint) bool {
	if !HasTaint(node.Spec.Taints, taint) {
		node.Spec.Taints = append(node.Spec.Taints, taint)
		return true
	}
	return false
}
