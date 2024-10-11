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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// defaultSortLessFunc returns true if a condition is less than another with regards to the
// order of conditions designed for convenience of the consumer, i.e. kubectl get.
// According to this order the Available and the Ready condition always goes first, Deleting and Paused always goes last,
// and all the other conditions are sorted by Type.
func defaultSortLessFunc(i, j metav1.Condition) bool {
	fi, oki := first[i.Type]
	fj, okj := first[j.Type]
	switch {
	case oki && !okj:
		return true
	case !oki && okj:
		return false
	case oki && okj:
		return fi < fj
	}

	li, oki := last[i.Type]
	lj, okj := last[j.Type]
	switch {
	case oki && !okj:
		return false
	case !oki && okj:
		return true
	case oki && okj:
		return li < lj
	}

	return i.Type < j.Type
}

var first = map[string]int{
	clusterv1.AvailableV1Beta2Condition: 0,
	clusterv1.ReadyV1Beta2Condition:     1,
}

var last = map[string]int{
	clusterv1.PausedV1Beta2Condition:   0,
	clusterv1.DeletingV1Beta2Condition: 1,
}
