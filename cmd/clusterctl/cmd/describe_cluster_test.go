/*
Copyright 2019 The Kubernetes Authors.

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

package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gosuri/uitable"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/util/conditions"

	"github.com/fatih/color"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/tree"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func Test_getRowName(t *testing.T) {
	tests := []struct {
		name   string
		object controllerutil.Object
		expect string
	}{
		{
			name:   "Row name for objects should be kind/name",
			object: fakeObject("c1"),
			expect: "Object/c1",
		},
		{
			name:   "Row name for a deleting object should have deleted prefix",
			object: fakeObject("c1", withDeletionTimestamp),
			expect: "!! DELETED !! Object/c1",
		},
		{
			name:   "Row name for objects with meta name should be meta-name - kind/name",
			object: fakeObject("c1", withAnnotation(tree.ObjectMetaNameAnnotation, "MetaName")),
			expect: "MetaName - Object/c1",
		},
		{
			name:   "Row name for virtual objects should be name",
			object: fakeObject("c1", withAnnotation(tree.VirtualObjectAnnotation, "True")),
			expect: "c1",
		},
		{
			name: "Row name for group objects should be #-of-items kind",
			object: fakeObject("c1",
				withAnnotation(tree.VirtualObjectAnnotation, "True"),
				withAnnotation(tree.GroupObjectAnnotation, "True"),
				withAnnotation(tree.GroupItemsAnnotation, "c1, c2, c3"),
			),
			expect: "3 Objects...",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got := getRowName(tt.object)
			g.Expect(got).To(Equal(tt.expect))
		})
	}
}

func Test_newConditionDescriptor_readyColor(t *testing.T) {
	tests := []struct {
		name             string
		condition        *clusterv1.Condition
		expectReadyColor *color.Color
	}{
		{
			name:             "True condition should be green",
			condition:        conditions.TrueCondition("C"),
			expectReadyColor: green,
		},
		{
			name:             "Unknown condition should be white",
			condition:        conditions.UnknownCondition("C", "", ""),
			expectReadyColor: white,
		},
		{
			name:             "False condition, severity error should be red",
			condition:        conditions.FalseCondition("C", "", clusterv1.ConditionSeverityError, ""),
			expectReadyColor: red,
		},
		{
			name:             "False condition, severity warning should be yellow",
			condition:        conditions.FalseCondition("C", "", clusterv1.ConditionSeverityWarning, ""),
			expectReadyColor: yellow,
		},
		{
			name:             "False condition, severity info should be white",
			condition:        conditions.FalseCondition("C", "", clusterv1.ConditionSeverityInfo, ""),
			expectReadyColor: white,
		},
		{
			name:             "Condition without status should be gray",
			condition:        &clusterv1.Condition{},
			expectReadyColor: gray,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got := newConditionDescriptor(tt.condition)
			g.Expect(got.readyColor).To(Equal(tt.expectReadyColor))
		})
	}
}

func Test_newConditionDescriptor_truncateMessages(t *testing.T) {
	tests := []struct {
		name          string
		condition     *clusterv1.Condition
		expectMessage string
	}{
		{
			name:          "Short messages are not changed",
			condition:     conditions.UnknownCondition("C", "", "short message"),
			expectMessage: "short message",
		},
		{
			name:          "Long message are truncated",
			condition:     conditions.UnknownCondition("C", "", strings.Repeat("s", 150)),
			expectMessage: fmt.Sprintf("%s ...", strings.Repeat("s", 100)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got := newConditionDescriptor(tt.condition)
			g.Expect(got.message).To(Equal(tt.expectMessage))
		})
	}
}

func Test_TreePrefix(t *testing.T) {
	tests := []struct {
		name         string
		objectTree   *tree.ObjectTree
		expectPrefix []string
	}{
		{
			name: "First level child should get the right prefix",
			objectTree: func() *tree.ObjectTree {
				root := fakeObject("root")
				obectjTree := tree.NewObjectTree(root, tree.ObjectTreeOptions{})

				o1 := fakeObject("child1")
				o2 := fakeObject("child2")
				obectjTree.Add(root, o1)
				obectjTree.Add(root, o2)
				return obectjTree
			}(),
			expectPrefix: []string{
				"Object/root",
				"├─Object/child1", // first objects gets ├─
				"└─Object/child2", // last objects gets └─
			},
		},
		{
			name: "Second level child should get the right prefix",
			objectTree: func() *tree.ObjectTree {
				root := fakeObject("root")
				obectjTree := tree.NewObjectTree(root, tree.ObjectTreeOptions{})

				o1 := fakeObject("child1")
				o1_1 := fakeObject("child1.1")
				o1_2 := fakeObject("child1.2")
				o2 := fakeObject("child2")
				o2_1 := fakeObject("child2.1")
				o2_2 := fakeObject("child2.2")

				obectjTree.Add(root, o1)
				obectjTree.Add(o1, o1_1)
				obectjTree.Add(o1, o1_2)
				obectjTree.Add(root, o2)
				obectjTree.Add(o2, o2_1)
				obectjTree.Add(o2, o2_2)
				return obectjTree
			}(),
			expectPrefix: []string{
				"Object/root",
				"├─Object/child1",
				"│ ├─Object/child1.1", // first second level child gets pipes and ├─
				"│ └─Object/child1.2", // last second level child gets pipes and └─
				"└─Object/child2",
				"  ├─Object/child2.1", // first second level child spaces and ├─
				"  └─Object/child2.2", // last second level child gets spaces and └─
			},
		},
		{
			name: "Conditions should get the right prefix",
			objectTree: func() *tree.ObjectTree {
				root := fakeObject("root")
				obectjTree := tree.NewObjectTree(root, tree.ObjectTreeOptions{})

				o1 := fakeObject("child1",
					withAnnotation(tree.ShowObjectConditionsAnnotation, "True"),
					withCondition(conditions.TrueCondition("C1.1")),
					withCondition(conditions.TrueCondition("C1.2")),
				)
				o2 := fakeObject("child2",
					withAnnotation(tree.ShowObjectConditionsAnnotation, "True"),
					withCondition(conditions.TrueCondition("C2.1")),
					withCondition(conditions.TrueCondition("C2.2")),
				)
				obectjTree.Add(root, o1)
				obectjTree.Add(root, o2)
				return obectjTree
			}(),
			expectPrefix: []string{
				"Object/root",
				"├─Object/child1",
				"│             ├─C1.1", // first condition child gets pipes and ├─
				"│             └─C1.2", // last condition child gets └─ and pipes and └─
				"└─Object/child2",
				"              ├─C2.1", // first condition child gets spaces and ├─
				"              └─C2.2", // last condition child gets spaces and └─
			},
		},
		{
			name: "Conditions should get the right prefix if the object has a child",
			objectTree: func() *tree.ObjectTree {
				root := fakeObject("root")
				obectjTree := tree.NewObjectTree(root, tree.ObjectTreeOptions{})

				o1 := fakeObject("child1",
					withAnnotation(tree.ShowObjectConditionsAnnotation, "True"),
					withCondition(conditions.TrueCondition("C1.1")),
					withCondition(conditions.TrueCondition("C1.2")),
				)
				o1_1 := fakeObject("child1.1")

				o2 := fakeObject("child2",
					withAnnotation(tree.ShowObjectConditionsAnnotation, "True"),
					withCondition(conditions.TrueCondition("C2.1")),
					withCondition(conditions.TrueCondition("C2.2")),
				)
				o2_1 := fakeObject("child2.1")
				obectjTree.Add(root, o1)
				obectjTree.Add(o1, o1_1)
				obectjTree.Add(root, o2)
				obectjTree.Add(o2, o2_1)
				return obectjTree
			}(),
			expectPrefix: []string{
				"Object/root",
				"├─Object/child1",
				"│ │           ├─C1.1", // first condition child gets pipes, children pipe and ├─
				"│ │           └─C1.2", // last condition child gets pipes, children pipe and └─
				"│ └─Object/child1.1",
				"└─Object/child2",
				"  │           ├─C2.1", // first condition child gets spaces, children pipe and ├─
				"  │           └─C2.2", // last condition child gets spaces, children pipe and └─
				"  └─Object/child2.1",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Creates the output table
			tbl := uitable.New()

			// Add row for the root object, the cluster, and recursively for all the nodes representing the cluster status.
			addObjectRow("", tbl, tt.objectTree, tt.objectTree.GetRoot())

			for i := range tt.expectPrefix {
				g.Expect(tbl.Rows[i].Cells[0].String()).To(Equal(tt.expectPrefix[i]))
			}

		})
	}
}

type objectOption func(object controllerutil.Object)

func fakeObject(name string, options ...objectOption) controllerutil.Object {
	c := &clusterv1.Cluster{ // suing type cluster for simplicity, but this could be any object
		TypeMeta: metav1.TypeMeta{
			Kind: "Object",
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

func withAnnotation(name, value string) func(controllerutil.Object) {
	return func(c controllerutil.Object) {
		if c.GetAnnotations() == nil {
			c.SetAnnotations(map[string]string{})
		}
		a := c.GetAnnotations()
		a[name] = value
		c.SetAnnotations(a)
	}
}

func withCondition(c *clusterv1.Condition) func(controllerutil.Object) {
	return func(m controllerutil.Object) {
		setter := m.(conditions.Setter)
		conditions.Set(setter, c)
	}
}

func withDeletionTimestamp(object controllerutil.Object) {
	now := metav1.Now()
	object.SetDeletionTimestamp(&now)
}
