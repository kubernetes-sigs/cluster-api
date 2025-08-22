/*
Copyright 2020 The Kubernetes Authors.

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

package tree

import (
	"fmt"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
)

func Test_hasSameAvailableReadyUptoDateStatusAndReason(t *testing.T) {
	conditionTrue := &metav1.Condition{Status: metav1.ConditionTrue}
	conditionFalse := &metav1.Condition{Status: metav1.ConditionFalse, Reason: "Reason", Message: "message false"}
	conditionFalseAnotherReason := &metav1.Condition{Status: metav1.ConditionFalse, Reason: "AnotherReason", Message: "message false"}

	type conditionPair struct {
		a *metav1.Condition
		b *metav1.Condition
	}
	tests := []struct {
		name string
		args map[string]conditionPair
		want bool
	}{
		{
			name: "Objects without conditions are the same",
			args: map[string]conditionPair{
				"available":  {a: nil, b: nil},
				"ready":      {a: nil, b: nil},
				"up-tp-date": {a: nil, b: nil},
			},
			want: true,
		},
		{
			name: "Objects with same Available condition are the same",
			args: map[string]conditionPair{
				"available": {
					a: conditionTrue,
					b: conditionTrue,
				},
				"ready":      {a: nil, b: nil},
				"up-tp-date": {a: nil, b: nil},
			},
			want: true,
		},
		{
			name: "Objects with different Available.Status are not the same",
			args: map[string]conditionPair{
				"available": {
					a: conditionTrue,
					b: conditionFalse,
				},
				"ready":      {a: nil, b: nil},
				"up-tp-date": {a: nil, b: nil},
			},
			want: false,
		},
		{
			name: "Objects with different Available.Reason are not the same",
			args: map[string]conditionPair{
				"available": {
					a: conditionFalse,
					b: conditionFalseAnotherReason,
				},
				"ready":      {a: nil, b: nil},
				"up-tp-date": {a: nil, b: nil},
			},
			want: false,
		},
		{
			name: "Objects with same Ready condition are the same",
			args: map[string]conditionPair{
				"available": {a: nil, b: nil},
				"ready": {
					a: conditionTrue,
					b: conditionTrue,
				},
				"up-tp-date": {a: nil, b: nil},
			},
			want: true,
		},
		{
			name: "Objects with different Ready.Status are not the same",
			args: map[string]conditionPair{
				"available": {a: nil, b: nil},
				"ready": {
					a: conditionTrue,
					b: conditionFalse,
				},
				"up-tp-date": {a: nil, b: nil},
			},
			want: false,
		},
		{
			name: "Objects with different Ready.Reason are not the same",
			args: map[string]conditionPair{
				"available": {a: nil, b: nil},
				"ready": {
					a: conditionFalse,
					b: conditionFalseAnotherReason,
				},
				"up-tp-date": {a: nil, b: nil},
			},
			want: false,
		},
		{
			name: "Objects with same UpToDate condition are the same",
			args: map[string]conditionPair{
				"available": {a: nil, b: nil},
				"ready":     {a: nil, b: nil},
				"up-tp-date": {
					a: conditionTrue,
					b: conditionTrue,
				},
			},
			want: true,
		},
		{
			name: "Objects with different UpToDate.Status are not the same",
			args: map[string]conditionPair{
				"available": {a: nil, b: nil},
				"ready":     {a: nil, b: nil},
				"up-tp-date": {
					a: conditionTrue,
					b: conditionFalse,
				},
			},
			want: false,
		},
		{
			name: "Objects with different UpToDate.Reason are not the same",
			args: map[string]conditionPair{
				"available": {a: nil, b: nil},
				"ready":     {a: nil, b: nil},
				"up-tp-date": {
					a: conditionFalse,
					b: conditionFalseAnotherReason,
				},
			},
			want: false,
		},
		{
			name: "Objects with same conditions are the same",
			args: map[string]conditionPair{
				"available": {
					a: conditionTrue,
					b: conditionTrue,
				},
				"ready": {
					a: conditionTrue,
					b: conditionTrue,
				},
				"up-tp-date": {
					a: conditionTrue,
					b: conditionTrue,
				},
			},
			want: true,
		},
		{
			name: "Objects with at least one condition different are not the same",
			args: map[string]conditionPair{
				"available": {
					a: conditionFalse,
					b: conditionTrue,
				},
				"ready": {
					a: conditionTrue,
					b: conditionTrue,
				},
				"up-tp-date": {
					a: conditionTrue,
					b: conditionTrue,
				},
			},
			want: false,
		},
		{
			name: "Objects with at least one condition different are not the same",
			args: map[string]conditionPair{
				"available": {
					a: conditionTrue,
					b: conditionTrue,
				},
				"ready": {
					a: conditionFalse,
					b: conditionTrue,
				},
				"up-tp-date": {
					a: conditionTrue,
					b: conditionTrue,
				},
			},
			want: false,
		},
		{
			name: "Objects with at least one condition different are not the same",
			args: map[string]conditionPair{
				"available": {
					a: conditionTrue,
					b: conditionTrue,
				},
				"ready": {
					a: conditionTrue,
					b: conditionTrue,
				},
				"up-tp-date": {
					a: conditionFalse,
					b: conditionTrue,
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := hasSameAvailableReadyUptoDateStatusAndReason(tt.args["available"].a, tt.args["available"].b, tt.args["ready"].a, tt.args["ready"].b, tt.args["up-tp-date"].a, tt.args["up-tp-date"].b)
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_hasSameReadyStatusSeverityAndReason(t *testing.T) {
	readyTrue := v1beta1conditions.TrueCondition(clusterv1.ReadyV1Beta1Condition)
	readyFalseReasonInfo := v1beta1conditions.FalseCondition(clusterv1.ReadyV1Beta1Condition, "Reason", clusterv1.ConditionSeverityInfo, "message falseInfo1")
	readyFalseAnotherReasonInfo := v1beta1conditions.FalseCondition(clusterv1.ReadyV1Beta1Condition, "AnotherReason", clusterv1.ConditionSeverityInfo, "message falseInfo1")
	readyFalseReasonWarning := v1beta1conditions.FalseCondition(clusterv1.ReadyV1Beta1Condition, "Reason", clusterv1.ConditionSeverityWarning, "message falseInfo1")

	type args struct {
		a *clusterv1.Condition
		b *clusterv1.Condition
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Objects without conditions are the same",
			args: args{
				a: nil,
				b: nil,
			},
			want: true,
		},
		{
			name: "Objects with same Ready condition are the same",
			args: args{
				a: readyTrue,
				b: readyTrue,
			},
			want: true,
		},
		{
			name: "Objects with different Ready.Status are not the same",
			args: args{
				a: readyTrue,
				b: readyFalseReasonInfo,
			},
			want: false,
		},
		{
			name: "Objects with different Ready.Reason are not the same",
			args: args{
				a: readyFalseReasonInfo,
				b: readyFalseAnotherReasonInfo,
			},
			want: false,
		},
		{
			name: "Objects with different Ready.Severity are not the same",
			args: args{
				a: readyFalseReasonInfo,
				b: readyFalseReasonWarning,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := hasSameReadyStatusSeverityAndReason(tt.args.a, tt.args.b)
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_minLastTransitionTime(t *testing.T) {
	now := &metav1.Condition{Type: "now", LastTransitionTime: metav1.Now()}
	beforeNow := &metav1.Condition{Type: "beforeNow", LastTransitionTime: metav1.Time{Time: now.LastTransitionTime.Add(-1 * time.Hour)}}
	type args struct {
		a *metav1.Condition
		b *metav1.Condition
	}
	tests := []struct {
		name string
		args args
		want metav1.Time
	}{
		{
			name: "nil, nil should return empty time",
			args: args{
				a: nil,
				b: nil,
			},
			want: metav1.Time{},
		},
		{
			name: "nil, now should return now",
			args: args{
				a: nil,
				b: now,
			},
			want: now.LastTransitionTime,
		},
		{
			name: "now, nil should return now",
			args: args{
				a: now,
				b: nil,
			},
			want: now.LastTransitionTime,
		},
		{
			name: "now, beforeNow should return beforeNow",
			args: args{
				a: now,
				b: beforeNow,
			},
			want: beforeNow.LastTransitionTime,
		},
		{
			name: "beforeNow, now should return beforeNow",
			args: args{
				a: now,
				b: beforeNow,
			},
			want: beforeNow.LastTransitionTime,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := minLastTransitionTime(tt.args.a, tt.args.b)
			g.Expect(got.Time).To(BeTemporally("~", tt.want.Time))
		})
	}
}

func Test_minLastTransitionTimeV1Beta1(t *testing.T) {
	now := &clusterv1.Condition{Type: "now", LastTransitionTime: metav1.Now()}
	beforeNow := &clusterv1.Condition{Type: "beforeNow", LastTransitionTime: metav1.Time{Time: now.LastTransitionTime.Add(-1 * time.Hour)}}
	type args struct {
		a *clusterv1.Condition
		b *clusterv1.Condition
	}
	tests := []struct {
		name string
		args args
		want metav1.Time
	}{
		{
			name: "nil, nil should return empty time",
			args: args{
				a: nil,
				b: nil,
			},
			want: metav1.Time{},
		},
		{
			name: "nil, now should return now",
			args: args{
				a: nil,
				b: now,
			},
			want: now.LastTransitionTime,
		},
		{
			name: "now, nil should return now",
			args: args{
				a: now,
				b: nil,
			},
			want: now.LastTransitionTime,
		},
		{
			name: "now, beforeNow should return beforeNow",
			args: args{
				a: now,
				b: beforeNow,
			},
			want: beforeNow.LastTransitionTime,
		},
		{
			name: "beforeNow, now should return beforeNow",
			args: args{
				a: now,
				b: beforeNow,
			},
			want: beforeNow.LastTransitionTime,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := minLastTransitionTimeV1Beta1(tt.args.a, tt.args.b)
			g.Expect(got.Time).To(BeTemporally("~", tt.want.Time))
		})
	}
}

func Test_isObjDebug(t *testing.T) {
	obj := fakeMachine("my-machine")
	type args struct {
		filter string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "empty filter should return false",
			args: args{
				filter: "",
			},
			want: false,
		},
		{
			name: "all filter should return true",
			args: args{
				filter: "all",
			},
			want: true,
		},
		{
			name: "kind filter should return true",
			args: args{
				filter: "Machine",
			},
			want: true,
		},
		{
			name: "another kind filter should return false",
			args: args{
				filter: "AnotherKind",
			},
			want: false,
		},
		{
			name: "kind/name filter should return true",
			args: args{
				filter: "Machine/my-machine",
			},
			want: true,
		},
		{
			name: "kind/wrong name filter should return false",
			args: args{
				filter: "Cluster/another-cluster",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := isObjDebug(obj, tt.args.filter)
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_createGroupNode(t *testing.T) {
	now := metav1.Now()
	beforeNow := metav1.Time{Time: now.Time.Add(-1 * time.Hour)}.Rfc3339Copy()

	obj := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "my-machine",
		},
		Status: clusterv1.MachineStatus{
			Conditions: []metav1.Condition{
				{Type: clusterv1.AvailableCondition, Status: metav1.ConditionTrue},
				{Type: clusterv1.ReadyCondition, Status: metav1.ConditionTrue, LastTransitionTime: now},
				{Type: clusterv1.MachineUpToDateCondition, Status: metav1.ConditionFalse},
			},
		},
	}

	sibling := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "sibling-machine",
		},
		Status: clusterv1.MachineStatus{
			Conditions: []metav1.Condition{
				{Type: clusterv1.ReadyCondition, LastTransitionTime: beforeNow},
			},
		},
	}

	want := &NodeObject{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachineGroup",
			APIVersion: GroupVersionVirtualObject.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "", // random string
			Namespace: "ns",
			Annotations: map[string]string{
				VirtualObjectAnnotation:    "True",
				GroupObjectAnnotation:      "True",
				GroupItemsAnnotation:       "my-machine, sibling-machine",
				GroupItemsReadyCounter:     "2",
				GroupItemsAvailableCounter: "2",
				GroupItemsUpToDateCounter:  "0",
			},
			UID: types.UID(""), // random string
		},
		Status: NodeStatus{
			Conditions: []metav1.Condition{
				{Type: clusterv1.AvailableCondition, Status: metav1.ConditionTrue},
				{Type: clusterv1.ReadyCondition, Status: metav1.ConditionTrue, LastTransitionTime: beforeNow},
				{Type: clusterv1.MachineUpToDateCondition, Status: metav1.ConditionFalse},
			},
		},
	}

	g := NewWithT(t)
	got := createGroupNode(sibling, GetReadyCondition(sibling), obj, GetAvailableCondition(obj), GetReadyCondition(obj), GetMachineUpToDateCondition(obj))

	// Some values are generated randomly, so pick up them.
	want.SetName(got.GetName())
	want.SetUID(got.GetUID())
	for i := range got.Status.Conditions {
		if got.Status.Conditions[i].Type == clusterv1.ReadyCondition {
			continue
		}
		got.Status.Conditions[i].LastTransitionTime = metav1.Time{}
	}

	g.Expect(got).To(BeComparableTo(want))
}

func Test_createV1Beta1GroupNode(t *testing.T) {
	now := metav1.Now()
	beforeNow := metav1.Time{Time: now.Add(-1 * time.Hour)}

	obj := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "my-machine",
		},
		Status: clusterv1.MachineStatus{
			Deprecated: &clusterv1.MachineDeprecatedStatus{
				V1Beta1: &clusterv1.MachineV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						clusterv1.Condition{Type: clusterv1.ReadyV1Beta1Condition, LastTransitionTime: now},
					},
				},
			},
		},
	}

	sibling := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "sibling-machine",
		},
		Status: clusterv1.MachineStatus{
			Deprecated: &clusterv1.MachineDeprecatedStatus{
				V1Beta1: &clusterv1.MachineV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						clusterv1.Condition{Type: clusterv1.ReadyV1Beta1Condition, LastTransitionTime: beforeNow},
					},
				},
			},
		},
	}

	want := &NodeObject{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachineGroup",
			APIVersion: GroupVersionVirtualObject.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "", // random string
			Namespace: "ns",
			Annotations: map[string]string{
				VirtualObjectAnnotation: "True",
				GroupObjectAnnotation:   "True",
				GroupItemsAnnotation:    "my-machine, sibling-machine",
			},
			UID: types.UID(""), // random string
		},
		Status: NodeStatus{
			Deprecated: &NodeDeprecatedStatus{
				V1Beta1: &NodeV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:               "Ready",
							Status:             "",
							LastTransitionTime: beforeNow,
						},
					},
				},
			},
		},
	}

	g := NewWithT(t)
	got := createV1Beta1GroupNode(sibling, GetV1Beta1ReadyCondition(sibling), obj, GetV1Beta1ReadyCondition(obj))

	// Some values are generated randomly, so pick up them.
	want.SetName(got.GetName())
	want.SetUID(got.GetUID())

	g.Expect(got).To(BeComparableTo(want))
}

func Test_updateGroupNode(t *testing.T) {
	now := metav1.Now()
	beforeNow := metav1.Time{Time: now.Add(-1 * time.Hour)}

	group := &NodeObject{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachineGroup",
			APIVersion: GroupVersionVirtualObject.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "", // random string
			Namespace: "ns",
			Annotations: map[string]string{
				VirtualObjectAnnotation:    "True",
				GroupObjectAnnotation:      "True",
				GroupItemsAnnotation:       "my-machine, sibling-machine",
				GroupItemsReadyCounter:     "2",
				GroupItemsAvailableCounter: "2",
				GroupItemsUpToDateCounter:  "0",
			},
			UID: types.UID(""), // random string
		},
		Status: NodeStatus{
			Conditions: []metav1.Condition{
				{Type: clusterv1.AvailableCondition, Status: metav1.ConditionTrue},
				{Type: clusterv1.ReadyCondition, LastTransitionTime: beforeNow},
				{Type: clusterv1.MachineUpToDateCondition, Status: metav1.ConditionFalse},
			},
		},
	}

	obj := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "another-machine",
		},
		Status: clusterv1.MachineStatus{
			Conditions: []metav1.Condition{
				{Type: clusterv1.AvailableCondition, Status: metav1.ConditionTrue},
				{Type: clusterv1.ReadyCondition, Status: metav1.ConditionTrue, LastTransitionTime: now},
				{Type: clusterv1.MachineUpToDateCondition, Status: metav1.ConditionFalse},
			},
		},
	}

	want := &NodeObject{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachineGroup",
			APIVersion: GroupVersionVirtualObject.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "", // random string
			Namespace: "ns",
			Annotations: map[string]string{
				VirtualObjectAnnotation:    "True",
				GroupObjectAnnotation:      "True",
				GroupItemsAnnotation:       "another-machine, my-machine, sibling-machine",
				GroupItemsReadyCounter:     "3",
				GroupItemsAvailableCounter: "3",
				GroupItemsUpToDateCounter:  "0",
			},
			UID: types.UID(""), // random string
		},
		Status: NodeStatus{
			Conditions: []metav1.Condition{
				{Type: clusterv1.AvailableCondition, Status: metav1.ConditionTrue},
				{Type: clusterv1.ReadyCondition, LastTransitionTime: beforeNow},
				{Type: clusterv1.MachineUpToDateCondition, Status: metav1.ConditionFalse},
			},
		},
	}

	g := NewWithT(t)
	updateGroupNode(group, GetReadyCondition(group), obj, GetAvailableCondition(obj), GetReadyCondition(obj), GetMachineUpToDateCondition(obj))

	g.Expect(group).To(BeComparableTo(want))
}

func Test_updateV1Beta1GroupNode(t *testing.T) {
	now := metav1.Now()
	beforeNow := metav1.Time{Time: now.Add(-1 * time.Hour)}

	group := &NodeObject{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachineGroup",
			APIVersion: GroupVersionVirtualObject.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "", // random string
			Namespace: "ns",
			Annotations: map[string]string{
				VirtualObjectAnnotation: "True",
				GroupObjectAnnotation:   "True",
				GroupItemsAnnotation:    "my-machine, sibling-machine",
			},
			UID: types.UID(""), // random string
		},
		Status: NodeStatus{
			Deprecated: &NodeDeprecatedStatus{
				V1Beta1: &NodeV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:               "Ready",
							Status:             "",
							LastTransitionTime: beforeNow,
						},
					},
				},
			},
		},
	}

	obj := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "another-machine",
		},
		Status: clusterv1.MachineStatus{
			Deprecated: &clusterv1.MachineDeprecatedStatus{
				V1Beta1: &clusterv1.MachineV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						clusterv1.Condition{Type: clusterv1.ReadyV1Beta1Condition, LastTransitionTime: now},
					},
				},
			},
		},
	}

	want := &NodeObject{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachineGroup",
			APIVersion: GroupVersionVirtualObject.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "", // random string
			Namespace: "ns",
			Annotations: map[string]string{
				VirtualObjectAnnotation: "True",
				GroupObjectAnnotation:   "True",
				GroupItemsAnnotation:    "another-machine, my-machine, sibling-machine",
			},
			UID: types.UID(""), // random string
		},
		Status: NodeStatus{
			Deprecated: &NodeDeprecatedStatus{
				V1Beta1: &NodeV1Beta1DeprecatedStatus{
					Conditions: clusterv1.Conditions{
						{
							Type:               "Ready",
							Status:             "",
							LastTransitionTime: beforeNow,
						},
					},
				},
			},
		},
	}

	g := NewWithT(t)
	updateV1Beta1GroupNode(group, GetV1Beta1ReadyCondition(group), obj, GetV1Beta1ReadyCondition(obj))

	g.Expect(group).To(BeComparableTo(want))
}

func Test_Add_setsShowObjectConditionsAnnotation(t *testing.T) {
	parent := fakeCluster("parent")
	obj := fakeMachine("my-machine")

	type args struct {
		treeOptions ObjectTreeOptions
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "filter selecting my machine should not add the annotation",
			args: args{
				treeOptions: ObjectTreeOptions{ShowOtherConditions: "all"},
			},
			want: true,
		},
		{
			name: "filter not selecting my machine should not add the annotation",
			args: args{
				treeOptions: ObjectTreeOptions{ShowOtherConditions: ""},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		for _, v1beta1 := range []bool{true, false} {
			tt.args.treeOptions.V1Beta1 = v1beta1

			t.Run(tt.name+" v1beta1: "+fmt.Sprintf("%t", v1beta1), func(t *testing.T) {
				root := parent.DeepCopy()
				tree := NewObjectTree(root, tt.args.treeOptions)

				g := NewWithT(t)
				getAdded, gotVisible := tree.Add(root, obj.DeepCopy())
				g.Expect(getAdded).To(BeTrue())
				g.Expect(gotVisible).To(BeTrue())

				gotObj := tree.GetObject("my-machine")
				g.Expect(gotObj).ToNot(BeNil())
				switch tt.want {
				case true:
					g.Expect(gotObj.GetAnnotations()).To(HaveKey(ShowObjectConditionsAnnotation))
					g.Expect(gotObj.GetAnnotations()[ShowObjectConditionsAnnotation]).To(Equal("True"))
				case false:
					g.Expect(gotObj.GetAnnotations()).ToNot(HaveKey(ShowObjectConditionsAnnotation))
				}
			})
		}
	}
}

func Test_Add_setsGroupingObjectAnnotation(t *testing.T) {
	parent := fakeCluster("parent")
	obj := fakeMachine("my-machine")

	type args struct {
		treeOptions ObjectTreeOptions
		addOptions  []AddObjectOption
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "should not add the annotation if not requested to",
			args: args{
				treeOptions: ObjectTreeOptions{},
				addOptions:  nil, // without GroupingObject option
			},
			want: false,
		},
		{
			name: "should add the annotation if requested to and grouping is enabled",
			args: args{
				treeOptions: ObjectTreeOptions{Grouping: true},
				addOptions:  []AddObjectOption{GroupingObject(true)},
			},
			want: true,
		},
		{
			name: "should not add the annotation if requested to, but grouping is disabled",
			args: args{
				treeOptions: ObjectTreeOptions{Grouping: false},
				addOptions:  []AddObjectOption{GroupingObject(true)},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		for _, v1beta1 := range []bool{true, false} {
			tt.args.treeOptions.V1Beta1 = v1beta1

			t.Run(tt.name+" v1beta1: "+fmt.Sprintf("%t", v1beta1), func(t *testing.T) {
				root := parent.DeepCopy()
				tree := NewObjectTree(root, tt.args.treeOptions)

				g := NewWithT(t)
				getAdded, gotVisible := tree.Add(root, obj.DeepCopy(), tt.args.addOptions...)
				g.Expect(getAdded).To(BeTrue())
				g.Expect(gotVisible).To(BeTrue())

				gotObj := tree.GetObject("my-machine")
				g.Expect(gotObj).ToNot(BeNil())
				switch tt.want {
				case true:
					g.Expect(gotObj.GetAnnotations()).To(HaveKey(GroupingObjectAnnotation))
					g.Expect(gotObj.GetAnnotations()[GroupingObjectAnnotation]).To(Equal("True"))
				case false:
					g.Expect(gotObj.GetAnnotations()).ToNot(HaveKey(GroupingObjectAnnotation))
				}
			})
		}
	}
}

func Test_Add_setsObjectMetaNameAnnotation(t *testing.T) {
	parent := fakeCluster("parent")
	obj := fakeMachine("my-machine")

	type args struct {
		addOptions []AddObjectOption
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "should not add the annotation if not requested to",
			args: args{
				addOptions: nil, // without ObjectMetaName option
			},
			want: false,
		},
		{
			name: "should add the annotation if requested to",
			args: args{
				addOptions: []AddObjectOption{ObjectMetaName("MetaName")},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		for _, v1beta1 := range []bool{true, false} {
			treeOptions := ObjectTreeOptions{V1Beta1: v1beta1}

			t.Run(tt.name+" v1beta1: "+fmt.Sprintf("%t", v1beta1), func(t *testing.T) {
				root := parent.DeepCopy()
				tree := NewObjectTree(root, treeOptions)

				g := NewWithT(t)
				getAdded, gotVisible := tree.Add(root, obj.DeepCopy(), tt.args.addOptions...)
				g.Expect(getAdded).To(BeTrue())
				g.Expect(gotVisible).To(BeTrue())

				gotObj := tree.GetObject("my-machine")
				g.Expect(gotObj).ToNot(BeNil())
				switch tt.want {
				case true:
					g.Expect(gotObj.GetAnnotations()).To(HaveKey(ObjectMetaNameAnnotation))
					g.Expect(gotObj.GetAnnotations()[ObjectMetaNameAnnotation]).To(Equal("MetaName"))
				case false:
					g.Expect(gotObj.GetAnnotations()).ToNot(HaveKey(ObjectMetaNameAnnotation))
				}
			})
		}
	}
}

func Test_Add_NoEcho(t *testing.T) {
	parent := fakeCluster("parent",
		withClusterCondition(metav1.Condition{Type: clusterv1.ReadyCondition, Status: metav1.ConditionTrue}),
	)

	type args struct {
		treeOptions ObjectTreeOptions
		addOptions  []AddObjectOption
		obj         *clusterv1.Machine
	}
	tests := []struct {
		name     string
		args     args
		wantNode bool
	}{
		{
			name: "should always add if NoEcho option is not present",
			args: args{
				treeOptions: ObjectTreeOptions{},
				addOptions:  nil,
				obj: fakeMachine("my-machine",
					withMachineCondition(metav1.Condition{Type: clusterv1.ReadyCondition, Status: metav1.ConditionTrue}),
				),
			},
			wantNode: true,
		},
		{
			name: "should not add if NoEcho option is present and objects have same ReadyCondition",
			args: args{
				treeOptions: ObjectTreeOptions{},
				addOptions:  []AddObjectOption{NoEcho(true)},
				obj: fakeMachine("my-machine",
					withMachineCondition(metav1.Condition{Type: clusterv1.ReadyCondition, Status: metav1.ConditionTrue}),
				),
			},
			wantNode: false,
		},
		{
			name: "should add if NoEcho option is present but objects have not same ReadyCondition",
			args: args{
				treeOptions: ObjectTreeOptions{},
				addOptions:  []AddObjectOption{NoEcho(true)},
				obj: fakeMachine("my-machine",
					withMachineCondition(metav1.Condition{Type: clusterv1.ReadyCondition, Status: metav1.ConditionFalse}),
				),
			},
			wantNode: true,
		},
		{
			name: "should add if NoEcho option is present, objects have same ReadyCondition, but NoEcho is disabled",
			args: args{
				treeOptions: ObjectTreeOptions{Echo: true},
				addOptions:  []AddObjectOption{NoEcho(true)},
				obj: fakeMachine("my-machine",
					withMachineCondition(metav1.Condition{Type: clusterv1.ReadyCondition, Status: metav1.ConditionTrue}),
				),
			},
			wantNode: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := parent.DeepCopy()
			tree := NewObjectTree(root, tt.args.treeOptions)

			g := NewWithT(t)
			getAdded, gotVisible := tree.Add(root, tt.args.obj, tt.args.addOptions...)
			g.Expect(getAdded).To(Equal(tt.wantNode))
			g.Expect(gotVisible).To(Equal(tt.wantNode))

			gotObj := tree.GetObject("my-machine")
			switch tt.wantNode {
			case true:
				g.Expect(gotObj).ToNot(BeNil())
			case false:
				g.Expect(gotObj).To(BeNil())
			}
		})
	}
}

func Test_Add_NoEcho_V1Beta1(t *testing.T) {
	parent := fakeCluster("parent",
		withClusterV1Beta1Condition(v1beta1conditions.TrueCondition(clusterv1.ReadyV1Beta1Condition)),
	)

	type args struct {
		treeOptions ObjectTreeOptions
		addOptions  []AddObjectOption
		obj         *clusterv1.Machine
	}
	tests := []struct {
		name     string
		args     args
		wantNode bool
	}{
		{
			name: "should always add if NoEcho option is not present",
			args: args{
				treeOptions: ObjectTreeOptions{},
				addOptions:  nil,
				obj: fakeMachine("my-machine",
					withMachineV1Beta1Condition(v1beta1conditions.TrueCondition(clusterv1.ReadyV1Beta1Condition)),
				),
			},
			wantNode: true,
		},
		{
			name: "should not add if NoEcho option is present and objects have same ReadyCondition",
			args: args{
				treeOptions: ObjectTreeOptions{},
				addOptions:  []AddObjectOption{NoEcho(true)},
				obj: fakeMachine("my-machine",
					withMachineV1Beta1Condition(v1beta1conditions.TrueCondition(clusterv1.ReadyV1Beta1Condition)),
				),
			},
			wantNode: false,
		},
		{
			name: "should add if NoEcho option is present but objects have not same ReadyCondition",
			args: args{
				treeOptions: ObjectTreeOptions{},
				addOptions:  []AddObjectOption{NoEcho(true)},
				obj: fakeMachine("my-machine",
					withMachineV1Beta1Condition(v1beta1conditions.FalseCondition(clusterv1.ReadyV1Beta1Condition, "", clusterv1.ConditionSeverityInfo, "")),
				),
			},
			wantNode: true,
		},
		{
			name: "should add if NoEcho option is present, objects have same ReadyCondition, but NoEcho is disabled",
			args: args{
				treeOptions: ObjectTreeOptions{Echo: true},
				addOptions:  []AddObjectOption{NoEcho(true)},
				obj: fakeMachine("my-machine",
					withMachineV1Beta1Condition(v1beta1conditions.TrueCondition(clusterv1.ReadyV1Beta1Condition)),
				),
			},
			wantNode: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.args.treeOptions.V1Beta1 = true

			root := parent.DeepCopy()
			tree := NewObjectTree(root, tt.args.treeOptions)

			g := NewWithT(t)
			getAdded, gotVisible := tree.Add(root, tt.args.obj, tt.args.addOptions...)
			g.Expect(getAdded).To(Equal(tt.wantNode))
			g.Expect(gotVisible).To(Equal(tt.wantNode))

			gotObj := tree.GetObject("my-machine")
			switch tt.wantNode {
			case true:
				g.Expect(gotObj).ToNot(BeNil())
			case false:
				g.Expect(gotObj).To(BeNil())
			}
		})
	}
}

func Test_Add_Grouping(t *testing.T) {
	parent := fakeCluster("parent",
		withClusterAnnotation(GroupingObjectAnnotation, "True"),
	)

	type args struct {
		addOptions []AddObjectOption
		siblings   []client.Object
		obj        client.Object
	}
	tests := []struct {
		name            string
		args            args
		wantNodesPrefix []string
		wantVisible     bool
		wantItems       string
	}{
		{
			name: "should never group the first child object",
			args: args{
				obj: fakeMachine("my-machine"),
			},
			wantNodesPrefix: []string{"my-machine"},
			wantVisible:     true,
		},
		{
			name: "should group child node if it has same kind and conditions of an existing one",
			args: args{
				siblings: []client.Object{
					fakeMachine("first-machine",
						withMachineCondition(metav1.Condition{Type: clusterv1.ReadyCondition, Status: metav1.ConditionTrue}),
					),
				},
				obj: fakeMachine("second-machine",
					withMachineCondition(metav1.Condition{Type: clusterv1.ReadyCondition, Status: metav1.ConditionTrue}),
				),
			},
			wantNodesPrefix: []string{"zz_True"},
			wantVisible:     false,
			wantItems:       "first-machine, second-machine",
		},
		{
			name: "should group child node if it has same kind and conditions of an existing group",
			args: args{
				siblings: []client.Object{
					fakeMachine("first-machine",
						withMachineCondition(metav1.Condition{Type: clusterv1.ReadyCondition, Status: metav1.ConditionTrue}),
					),
					fakeMachine("second-machine",
						withMachineCondition(metav1.Condition{Type: clusterv1.ReadyCondition, Status: metav1.ConditionTrue}),
					),
				},
				obj: fakeMachine("third-machine",
					withMachineCondition(metav1.Condition{Type: clusterv1.ReadyCondition, Status: metav1.ConditionTrue}),
				),
			},
			wantNodesPrefix: []string{"zz_True"},
			wantVisible:     false,
			wantItems:       "first-machine, second-machine, third-machine",
		},
		{
			name: "should not group child node if it has different kind",
			args: args{
				siblings: []client.Object{
					fakeMachine("first-machine",
						withMachineCondition(metav1.Condition{Type: clusterv1.ReadyCondition, Status: metav1.ConditionTrue}),
					),
					fakeMachine("second-machine",
						withMachineCondition(metav1.Condition{Type: clusterv1.ReadyCondition, Status: metav1.ConditionTrue}),
					),
				},
				obj: VirtualObject("ns", "NotAMachine", "other-object"),
			},
			wantNodesPrefix: []string{"zz_True", "other-object"},
			wantVisible:     true,
			wantItems:       "first-machine, second-machine",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := parent.DeepCopy()
			tree := NewObjectTree(root, ObjectTreeOptions{})

			for i := range tt.args.siblings {
				tree.Add(parent, tt.args.siblings[i], tt.args.addOptions...)
			}

			g := NewWithT(t)
			getAdded, gotVisible := tree.Add(root, tt.args.obj, tt.args.addOptions...)
			g.Expect(getAdded).To(BeTrue())
			g.Expect(gotVisible).To(Equal(tt.wantVisible))

			gotObjs := tree.GetObjectsByParent("parent")
			g.Expect(gotObjs).To(HaveLen(len(tt.wantNodesPrefix)))
			for _, obj := range gotObjs {
				found := false
				for _, prefix := range tt.wantNodesPrefix {
					if strings.HasPrefix(obj.GetName(), prefix) {
						found = true
						break
					}
				}
				g.Expect(found).To(BeTrue(), "Found object with name %q, waiting for one of %s", obj.GetName(), tt.wantNodesPrefix)

				if strings.HasPrefix(obj.GetName(), "zz_") {
					g.Expect(GetGroupItems(obj)).To(Equal(tt.wantItems))
				}
			}
		})
	}
}

func Test_Add_Grouping_V1Beta1(t *testing.T) {
	parent := fakeCluster("parent",
		withClusterAnnotation(GroupingObjectAnnotation, "True"),
	)

	type args struct {
		addOptions []AddObjectOption
		siblings   []client.Object
		obj        client.Object
	}
	tests := []struct {
		name            string
		args            args
		wantNodesPrefix []string
		wantVisible     bool
		wantItems       string
	}{
		{
			name: "should never group the first child object",
			args: args{
				obj: fakeMachine("my-machine"),
			},
			wantNodesPrefix: []string{"my-machine"},
			wantVisible:     true,
		},
		{
			name: "should group child node if it has same kind and conditions of an existing one",
			args: args{
				siblings: []client.Object{
					fakeMachine("first-machine",
						withMachineV1Beta1Condition(v1beta1conditions.TrueCondition(clusterv1.ReadyV1Beta1Condition)),
					),
				},
				obj: fakeMachine("second-machine",
					withMachineV1Beta1Condition(v1beta1conditions.TrueCondition(clusterv1.ReadyV1Beta1Condition)),
				),
			},
			wantNodesPrefix: []string{"zz_True"},
			wantVisible:     false,
			wantItems:       "first-machine, second-machine",
		},
		{
			name: "should group child node if it has same kind and conditions of an existing group",
			args: args{
				siblings: []client.Object{
					fakeMachine("first-machine",
						withMachineV1Beta1Condition(v1beta1conditions.TrueCondition(clusterv1.ReadyV1Beta1Condition)),
					),
					fakeMachine("second-machine",
						withMachineV1Beta1Condition(v1beta1conditions.TrueCondition(clusterv1.ReadyV1Beta1Condition)),
					),
				},
				obj: fakeMachine("third-machine",
					withMachineV1Beta1Condition(v1beta1conditions.TrueCondition(clusterv1.ReadyV1Beta1Condition)),
				),
			},
			wantNodesPrefix: []string{"zz_True"},
			wantVisible:     false,
			wantItems:       "first-machine, second-machine, third-machine",
		},
		{
			name: "should not group child node if it has different kind",
			args: args{
				siblings: []client.Object{
					fakeMachine("first-machine",
						withMachineV1Beta1Condition(v1beta1conditions.TrueCondition(clusterv1.ReadyV1Beta1Condition)),
					),
					fakeMachine("second-machine",
						withMachineV1Beta1Condition(v1beta1conditions.TrueCondition(clusterv1.ReadyV1Beta1Condition)),
					),
				},
				obj: VirtualObject("ns", "NotAMachine", "other-object"),
			},
			wantNodesPrefix: []string{"zz_True", "other-object"},
			wantVisible:     true,
			wantItems:       "first-machine, second-machine",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := parent.DeepCopy()
			tree := NewObjectTree(root, ObjectTreeOptions{V1Beta1: true})

			for i := range tt.args.siblings {
				tree.Add(parent, tt.args.siblings[i], tt.args.addOptions...)
			}

			g := NewWithT(t)
			getAdded, gotVisible := tree.Add(root, tt.args.obj, tt.args.addOptions...)
			g.Expect(getAdded).To(BeTrue())
			g.Expect(gotVisible).To(Equal(tt.wantVisible))

			gotObjs := tree.GetObjectsByParent("parent")
			g.Expect(gotObjs).To(HaveLen(len(tt.wantNodesPrefix)))
			for _, obj := range gotObjs {
				found := false
				for _, prefix := range tt.wantNodesPrefix {
					if strings.HasPrefix(obj.GetName(), prefix) {
						found = true
						break
					}
				}
				g.Expect(found).To(BeTrue(), "Found object with name %q, waiting for one of %s", obj.GetName(), tt.wantNodesPrefix)

				if strings.HasPrefix(obj.GetName(), "zz_") {
					g.Expect(GetGroupItems(obj)).To(Equal(tt.wantItems))
				}
			}
		})
	}
}

type clusterOption func(*clusterv1.Cluster)

func fakeCluster(name string, options ...clusterOption) *clusterv1.Cluster {
	c := &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind: "Cluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      name,
			UID:       types.UID(name),
		},
	}
	for _, opt := range options {
		opt(c)
	}
	return c
}

func withClusterAnnotation(name, value string) func(*clusterv1.Cluster) {
	return func(c *clusterv1.Cluster) {
		if c.Annotations == nil {
			c.Annotations = map[string]string{}
		}
		c.Annotations[name] = value
	}
}

func withClusterV1Beta1Condition(c *clusterv1.Condition) func(*clusterv1.Cluster) {
	return func(m *clusterv1.Cluster) {
		v1beta1conditions.Set(m, c)
	}
}

func withClusterCondition(c metav1.Condition) func(*clusterv1.Cluster) {
	return func(m *clusterv1.Cluster) {
		conditions.Set(m, c)
	}
}

type machineOption func(*clusterv1.Machine)

func fakeMachine(name string, options ...machineOption) *clusterv1.Machine {
	m := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      name,
			UID:       types.UID(name),
		},
	}
	for _, opt := range options {
		opt(m)
	}
	return m
}

func withMachineV1Beta1Condition(c *clusterv1.Condition) func(*clusterv1.Machine) {
	return func(m *clusterv1.Machine) {
		v1beta1conditions.Set(m, c)
	}
}

func withMachineCondition(c metav1.Condition) func(*clusterv1.Machine) {
	return func(m *clusterv1.Machine) {
		conditions.Set(m, c)
	}
}
