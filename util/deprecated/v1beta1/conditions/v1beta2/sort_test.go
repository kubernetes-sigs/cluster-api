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
	"sort"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestDefaultSortLessFunc(t *testing.T) {
	g := NewWithT(t)

	conditions := []metav1.Condition{
		{Type: clusterv1.DeletingV1Beta2Condition},
		{Type: "B"},
		{Type: clusterv1.AvailableV1Beta2Condition},
		{Type: "A"},
		{Type: clusterv1.PausedV1Beta2Condition},
		{Type: clusterv1.ReadyV1Beta2Condition},
		{Type: "C!"},
	}

	sort.Slice(conditions, func(i, j int) bool {
		return defaultSortLessFunc(conditions[i], conditions[j])
	})

	g.Expect(conditions).To(Equal([]metav1.Condition{
		{Type: clusterv1.AvailableV1Beta2Condition},
		{Type: clusterv1.ReadyV1Beta2Condition},
		{Type: "A"},
		{Type: "B"},
		{Type: "C!"},
		{Type: clusterv1.PausedV1Beta2Condition},
		{Type: clusterv1.DeletingV1Beta2Condition},
	}))
}
