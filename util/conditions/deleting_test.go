/*
Copyright 2026 The Kubernetes Authors.

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
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestAppendDeletingMessage(t *testing.T) {
	fakeObj := func(conditions ...interface{}) *unstructured.Unstructured {
		obj := &unstructured.Unstructured{Object: map[string]interface{}{
			"kind": "FakeControlPlane",
			"metadata": map[string]interface{}{
				"name": "cp1",
			},
		}}
		if len(conditions) > 0 {
			_ = unstructured.SetNestedSlice(obj.Object, conditions, "status", "conditions")
		}
		return obj
	}
	fakeCondition := func(status metav1.ConditionStatus, message string) map[string]interface{} {
		return map[string]interface{}{
			"type":               clusterv1.DeletingCondition,
			"status":             string(status),
			"reason":             "SomeReason",
			"message":            message,
			"lastTransitionTime": "2026-08-06T10:00:00Z",
		}
	}

	const baseMsg = "Waiting for FakeControlPlane to be deleted"

	tests := []struct {
		name string
		obj  *unstructured.Unstructured
		want string
	}{
		{
			name: "obj is nil",
			obj:  nil,
			want: "Waiting for FakeControlPlane to be deleted",
		},
		{
			name: "obj does not report conditions",
			obj:  fakeObj(),
			want: "Waiting for FakeControlPlane to be deleted",
		},
		{
			name: "obj reports Deleting condition with a single line message",
			obj:  fakeObj(fakeCondition(metav1.ConditionTrue, "Deleting 3 Machines")),
			want: "Waiting for FakeControlPlane to be deleted: Deleting 3 Machines",
		},
		{
			name: "obj reports Deleting condition with a multiline message",
			obj:  fakeObj(fakeCondition(metav1.ConditionTrue, "FakeControlPlane deletion blocked because following objects still exist:\n* Machine m1\n* Machine m2")),
			want: "Waiting for FakeControlPlane to be deleted:\n  * FakeControlPlane deletion blocked because following objects still exist:\n    * Machine m1\n    * Machine m2",
		},
		{
			name: "obj reports Deleting condition with status False",
			obj:  fakeObj(fakeCondition(metav1.ConditionFalse, "some message")),
			want: "Waiting for FakeControlPlane to be deleted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(AppendDeletingMessage(baseMsg, tt.obj)).To(Equal(tt.want))
		})
	}
}
