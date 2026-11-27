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
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestAppendDeletingMessage(t *testing.T) {
	fakeUnstructured := func(conditions ...interface{}) *unstructured.Unstructured {
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
	unstructuredCondition := func(status metav1.ConditionStatus, message string) map[string]interface{} {
		return map[string]interface{}{
			"type":               clusterv1.DeletingCondition,
			"status":             string(status),
			"reason":             "SomeReason",
			"message":            message,
			"lastTransitionTime": "2026-08-06T10:00:00Z",
		}
	}
	fakeTyped := func(conditions ...metav1.Condition) *builder.Phase3Obj {
		return &builder.Phase3Obj{
			TypeMeta:   metav1.TypeMeta{Kind: "FakeControlPlane"},
			ObjectMeta: metav1.ObjectMeta{Name: "cp1"},
			Status:     builder.Phase3ObjStatus{Conditions: conditions},
		}
	}
	typedCondition := func(status metav1.ConditionStatus, message string) metav1.Condition {
		return metav1.Condition{
			Type:    clusterv1.DeletingCondition,
			Status:  status,
			Reason:  "SomeReason",
			Message: message,
		}
	}

	const (
		baseMsg   = "Waiting for FakeControlPlane to be deleted"
		multiline = "FakeControlPlane deletion blocked because following objects still exist:\n* Machine m1\n* Machine m2"
		// wantMultiline is baseMsg followed by multiline, indented as a nested bullet list.
		wantMultiline = baseMsg + ":\n  * FakeControlPlane deletion blocked because following objects still exist:\n    * Machine m1\n    * Machine m2"
	)

	tests := []struct {
		name string
		obj  client.Object
		want string
	}{
		{
			name: "nil interface",
			obj:  nil,
			want: baseMsg,
		},
		{
			name: "unstructured nil pointer",
			obj:  (*unstructured.Unstructured)(nil),
			want: baseMsg,
		},
		{
			name: "typed nil pointer",
			obj:  (*builder.Phase3Obj)(nil),
			want: baseMsg,
		},
		{
			name: "object implementing neither unstructured nor Getter",
			obj:  &metav1.PartialObjectMetadata{},
			want: baseMsg,
		},
		{
			name: "unstructured without conditions",
			obj:  fakeUnstructured(),
			want: baseMsg,
		},
		{
			name: "unstructured with Deleting condition status False",
			obj:  fakeUnstructured(unstructuredCondition(metav1.ConditionFalse, "some message")),
			want: baseMsg,
		},
		{
			name: "unstructured with Deleting condition and empty message",
			obj:  fakeUnstructured(unstructuredCondition(metav1.ConditionTrue, "")),
			want: baseMsg,
		},
		{
			name: "unstructured with Deleting condition and single line message",
			obj:  fakeUnstructured(unstructuredCondition(metav1.ConditionTrue, "Deleting 3 Machines")),
			want: baseMsg + ": Deleting 3 Machines",
		},
		{
			name: "unstructured with Deleting condition and multiline message",
			obj:  fakeUnstructured(unstructuredCondition(metav1.ConditionTrue, multiline)),
			want: wantMultiline,
		},
		{
			name: "typed without conditions",
			obj:  fakeTyped(),
			want: baseMsg,
		},
		{
			name: "typed with Deleting condition status False",
			obj:  fakeTyped(typedCondition(metav1.ConditionFalse, "some message")),
			want: baseMsg,
		},
		{
			name: "typed with Deleting condition and empty message",
			obj:  fakeTyped(typedCondition(metav1.ConditionTrue, "")),
			want: baseMsg,
		},
		{
			name: "typed with Deleting condition and single line message",
			obj:  fakeTyped(typedCondition(metav1.ConditionTrue, "Deleting 3 Machines")),
			want: baseMsg + ": Deleting 3 Machines",
		},
		{
			name: "typed with Deleting condition and multiline message",
			obj:  fakeTyped(typedCondition(metav1.ConditionTrue, multiline)),
			want: wantMultiline,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(AppendDeletingMessage(baseMsg, tt.obj)).To(Equal(tt.want))
		})
	}
}
