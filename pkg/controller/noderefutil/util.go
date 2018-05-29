/*
Copyright 2018 The Kubernetes Authors.

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

package noderefutil

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IsNodeAvailable returns true if the node is ready and minReadySeconds have elapsed or is 0. False otherwise.
func IsNodeAvailable(node *corev1.Node, minReadySeconds int32, now metav1.Time) bool {
	if !IsNodeReady(node) {
		return false
	}

	if minReadySeconds == 0 {
		return true
	}

	minReadySecondsDuration := time.Duration(minReadySeconds) * time.Second
	readyCondition := GetReadyCondition(&node.Status)

	if !readyCondition.LastTransitionTime.IsZero() &&
		readyCondition.LastTransitionTime.Add(minReadySecondsDuration).Before(now.Time) {
		return true
	}

	return false
}

// GetReadyCondition extracts the ready condition from the given status and returns that.
// Returns nil and -1 if the condition is not present, and the index of the located condition.
func GetReadyCondition(status *corev1.NodeStatus) *corev1.NodeCondition {
	if status == nil {
		return nil
	}
	for i := range status.Conditions {
		if status.Conditions[i].Type == corev1.NodeReady {
			return &status.Conditions[i]
		}
	}
	return nil
}

// IsNodeReady returns true if a node is ready; false otherwise.
func IsNodeReady(node *corev1.Node) bool {
	if node == nil || &node.Status == nil {
		return false
	}
	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}
