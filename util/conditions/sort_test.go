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

package conditions

import (
	"slices"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestDefaultSortLessFunc(t *testing.T) {
	g := NewWithT(t)

	conditions := []metav1.Condition{
		{Type: clusterv1.DeletingCondition},
		{Type: "B"},
		{Type: clusterv1.AvailableCondition},
		{Type: "A"},
		{Type: clusterv1.PausedCondition},
		{Type: clusterv1.ReadyCondition},
		{Type: "C!"},
	}

	slices.SortFunc(conditions, func(i, j metav1.Condition) int {
		if defaultSortLessFunc(i, j) {
			return -1
		}
		return 1
	})

	g.Expect(conditions).To(Equal([]metav1.Condition{
		{Type: clusterv1.AvailableCondition},
		{Type: clusterv1.ReadyCondition},
		{Type: "A"},
		{Type: "B"},
		{Type: "C!"},
		{Type: clusterv1.PausedCondition},
		{Type: clusterv1.DeletingCondition},
	}))
}
