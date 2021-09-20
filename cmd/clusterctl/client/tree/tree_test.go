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
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"
)

func Test_hasSameReadyStatusSeverityAndReason(t *testing.T) {
	readyTrue := conditions.TrueCondition(clusterv1.ReadyCondition)
	readyFalseReasonInfo := conditions.FalseCondition(clusterv1.ReadyCondition, "Reason", clusterv1.ConditionSeverityInfo, "message falseInfo1")
	readyFalseAnotherReasonInfo := conditions.FalseCondition(clusterv1.ReadyCondition, "AnotherReason", clusterv1.ConditionSeverityInfo, "message falseInfo1")
	readyFalseReasonWarning := conditions.FalseCondition(clusterv1.ReadyCondition, "Reason", clusterv1.ConditionSeverityWarning, "message falseInfo1")

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
	now := &clusterv1.Condition{Type: "now", LastTransitionTime: metav1.Now()}
	beforeNow := &clusterv1.Condition{Type: "beforeNow", LastTransitionTime: metav1.Time{Time: now.LastTransitionTime.Time.Add(-1 * time.Hour)}}
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

			got := minLastTransitionTime(tt.args.a, tt.args.b)
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
	beforeNow := metav1.Time{Time: now.Time.Add(-1 * time.Hour)}

	obj := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "my-machine",
		},
		Status: clusterv1.MachineStatus{
			Conditions: clusterv1.Conditions{
				clusterv1.Condition{Type: clusterv1.ReadyCondition, LastTransitionTime: now},
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
			Conditions: clusterv1.Conditions{
				clusterv1.Condition{Type: clusterv1.ReadyCondition, LastTransitionTime: beforeNow},
			},
		},
	}

	want := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "virtual.cluster.x-k8s.io/v1beta1",
			"kind":       "MachineGroup",
			"metadata": map[string]interface{}{
				"namespace": "ns",
				"name":      "", // random string
				"annotations": map[string]interface{}{
					VirtualObjectAnnotation: "True",
					GroupObjectAnnotation:   "True",
					GroupItemsAnnotation:    "my-machine, sibling-machine",
				},
				"uid": "", // random string
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"status":             "",
						"lastTransitionTime": beforeNow.Time.UTC().Format(time.RFC3339),
						"type":               "Ready",
					},
				},
			},
		},
	}

	g := NewWithT(t)
	got := createGroupNode(sibling, GetReadyCondition(sibling), obj, GetReadyCondition(obj))

	// Some values are generated randomly, so pick up them.
	want.SetName(got.GetName())
	want.SetUID(got.GetUID())

	g.Expect(got).To(Equal(want))
}

func Test_updateGroupNode(t *testing.T) {
	now := metav1.Now()
	beforeNow := metav1.Time{Time: now.Time.Add(-1 * time.Hour)}

	group := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "virtual.cluster.x-k8s.io/v1beta1",
			"kind":       "MachineGroup",
			"metadata": map[string]interface{}{
				"namespace": "ns",
				"name":      "random-name",
				"annotations": map[string]interface{}{
					VirtualObjectAnnotation: "True",
					GroupObjectAnnotation:   "True",
					GroupItemsAnnotation:    "my-machine, sibling-machine",
				},
				"uid": "random-uid",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"status":             "",
						"lastTransitionTime": beforeNow.Time.UTC().Format(time.RFC3339),
						"type":               "Ready",
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
			Conditions: clusterv1.Conditions{
				clusterv1.Condition{Type: clusterv1.ReadyCondition, LastTransitionTime: now},
			},
		},
	}

	want := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "virtual.cluster.x-k8s.io/v1beta1",
			"kind":       "MachineGroup",
			"metadata": map[string]interface{}{
				"namespace": "ns",
				"name":      "random-name",
				"annotations": map[string]interface{}{
					VirtualObjectAnnotation: "True",
					GroupObjectAnnotation:   "True",
					GroupItemsAnnotation:    "another-machine, my-machine, sibling-machine",
				},
				"uid": "random-uid",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"status":             "",
						"lastTransitionTime": beforeNow.Time.UTC().Format(time.RFC3339),
						"type":               "Ready",
					},
				},
			},
		},
	}

	g := NewWithT(t)
	updateGroupNode(group, GetReadyCondition(group), obj, GetReadyCondition(obj))

	g.Expect(group).To(Equal(want))
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
		t.Run(tt.name, func(t *testing.T) {
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
			name: "should add the annotation if requested to",
			args: args{
				treeOptions: ObjectTreeOptions{},
				addOptions:  []AddObjectOption{GroupingObject(true)},
			},
			want: true,
		},
		{
			name: "should not add the annotation if requested to, but grouping is disabled",
			args: args{
				treeOptions: ObjectTreeOptions{DisableGrouping: true},
				addOptions:  []AddObjectOption{GroupingObject(true)},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
		t.Run(tt.name, func(t *testing.T) {
			root := parent.DeepCopy()
			tree := NewObjectTree(root, ObjectTreeOptions{})

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

func Test_Add_NoEcho(t *testing.T) {
	parent := fakeCluster("parent",
		withClusterCondition(conditions.TrueCondition(clusterv1.ReadyCondition)),
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
					withMachineCondition(conditions.TrueCondition(clusterv1.ReadyCondition)),
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
					withMachineCondition(conditions.TrueCondition(clusterv1.ReadyCondition)),
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
					withMachineCondition(conditions.FalseCondition(clusterv1.ReadyCondition, "", clusterv1.ConditionSeverityInfo, "")),
				),
			},
			wantNode: true,
		},
		{
			name: "should add if NoEcho option is present, objects have same ReadyCondition, but NoEcho is disabled",
			args: args{
				treeOptions: ObjectTreeOptions{DisableNoEcho: true},
				addOptions:  []AddObjectOption{NoEcho(true)},
				obj: fakeMachine("my-machine",
					withMachineCondition(conditions.TrueCondition(clusterv1.ReadyCondition)),
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

func Test_Add_Grouping(t *testing.T) {
	parent := fakeCluster("parent",
		withClusterAnnotation(GroupingObjectAnnotation, "True"),
	)

	type args struct {
		addOptions []AddObjectOption
		siblings   []*clusterv1.Machine
		obj        *clusterv1.Machine
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
			name: "should group child node if it has same conditions of an existing one",
			args: args{
				siblings: []*clusterv1.Machine{
					fakeMachine("first-machine",
						withMachineCondition(conditions.TrueCondition(clusterv1.ReadyCondition)),
					),
				},
				obj: fakeMachine("second-machine",
					withMachineCondition(conditions.TrueCondition(clusterv1.ReadyCondition)),
				),
			},
			wantNodesPrefix: []string{"zz_True"},
			wantVisible:     false,
			wantItems:       "first-machine, second-machine",
		},
		{
			name: "should group child node if it has same conditions of an existing group",
			args: args{
				siblings: []*clusterv1.Machine{
					fakeMachine("first-machine",
						withMachineCondition(conditions.TrueCondition(clusterv1.ReadyCondition)),
					),
					fakeMachine("second-machine",
						withMachineCondition(conditions.TrueCondition(clusterv1.ReadyCondition)),
					),
				},
				obj: fakeMachine("third-machine",
					withMachineCondition(conditions.TrueCondition(clusterv1.ReadyCondition)),
				),
			},
			wantNodesPrefix: []string{"zz_True"},
			wantVisible:     false,
			wantItems:       "first-machine, second-machine, third-machine",
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

func withClusterCondition(c *clusterv1.Condition) func(*clusterv1.Cluster) {
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

func withMachineCondition(c *clusterv1.Condition) func(*clusterv1.Machine) {
	return func(m *clusterv1.Machine) {
		conditions.Set(m, c)
	}
}
