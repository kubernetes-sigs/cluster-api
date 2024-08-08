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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	// AvailableCondition documents availability for an object.
	AvailableCondition = "Available"

	// ReadyCondition documents readiness for an object.
	ReadyCondition = "Ready"
)

// Less reports whether the i condition must sort before j condition.
type Less func(i, j metav1.Condition) bool

// lexicographicLess returns true if a condition is less than another with regards to the
// to order of conditions designed for convenience of the consumer, i.e. kubectl get.
// According to this order the Available and the Ready condition always goes first, followed by all the other
// conditions sorted by Type.
func lexicographicLess(i, j metav1.Condition) bool {
	return (i.Type == AvailableCondition || (i.Type == ReadyCondition || i.Type < j.Type) && j.Type != ReadyCondition) && j.Type != AvailableCondition
}
