/*
Copyright 2021 The Kubernetes Authors.

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

package v1beta1

import (
	"testing"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var exampleCluster = Cluster{
	TypeMeta: metav1.TypeMeta{
		Kind:       "Cluster",
		APIVersion: GroupVersion.String(),
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "testcluster",
		Namespace: "default",
		UID:       "2393f2c4-d63e-4c01-86b4-e62833391b39",
	},
}

func TestNewObjectReference(t *testing.T) {
	g := NewWithT(t)

	g.Expect(NewObjectReference(&exampleCluster)).To(PointTo(MatchAllFields(Fields{
		"Kind":      Equal("Cluster"),
		"Name":      Equal(exampleCluster.Name),
		"Namespace": Equal(exampleCluster.Namespace),
		"Group":     Equal(GroupVersion.Group),
	})))
}

func TestObjectReferenceNamespacedName(t *testing.T) {
	g := NewWithT(t)

	ref := ObjectReference{
		Name:      "test",
		Namespace: "default",
	}
	g.Expect(ref.NamespacedName()).To(MatchAllFields(Fields{
		"Name":      Equal("test"),
		"Namespace": Equal("default"),
	}))
}

var exampleObjRef = ObjectReference{
	Name:      "test",
	Namespace: "default",
	Kind:      "Cluster",
	Group:     GroupVersion.Group,
}

func TestObjectReferencePin(t *testing.T) {
	g := NewWithT(t)

	g.Expect(exampleObjRef.Pin(exampleCluster.UID)).To(PointTo(MatchAllFields(Fields{
		"Name":      Equal(exampleObjRef.Name),
		"Namespace": Equal(exampleObjRef.Namespace),
		"Kind":      Equal(exampleObjRef.Kind),
		"Group":     Equal(exampleObjRef.Group),
		"UID":       Equal(exampleCluster.UID),
	})))
}

func TestObjectReferenceToLocal(t *testing.T) {
	g := NewWithT(t)

	g.Expect(exampleObjRef.ToLocal()).To(PointTo(MatchAllFields(Fields{
		"Name":  Equal(exampleObjRef.Name),
		"Kind":  Equal(exampleObjRef.Kind),
		"Group": Equal(exampleObjRef.Group),
	})))
}

func TestNewPinnedObjectReference(t *testing.T) {
	g := NewWithT(t)

	g.Expect(NewPinnedObjectReference(&exampleCluster)).To(PointTo(MatchAllFields(Fields{
		"Kind":      Equal("Cluster"),
		"Name":      Equal(exampleCluster.Name),
		"Namespace": Equal(exampleCluster.Namespace),
		"Group":     Equal(GroupVersion.Group),
		"UID":       Equal(exampleCluster.UID),
	})))
}

func TestPinnedObjectReferenceNamespacedName(t *testing.T) {
	g := NewWithT(t)

	ref := PinnedObjectReference{
		Name:      "test",
		Namespace: "default",
	}
	g.Expect(ref.NamespacedName()).To(MatchAllFields(Fields{
		"Name":      Equal("test"),
		"Namespace": Equal("default"),
	}))
}

func TestPinnedObjectReferenceUnpin(t *testing.T) {
	g := NewWithT(t)

	var examplePinnedObjRef = PinnedObjectReference{
		Name:      "test",
		Namespace: "default",
		Kind:      "Cluster",
		Group:     GroupVersion.Group,
	}

	g.Expect(examplePinnedObjRef.Unpin()).To(PointTo(MatchAllFields(Fields{
		"Name":      Equal(examplePinnedObjRef.Name),
		"Namespace": Equal(examplePinnedObjRef.Namespace),
		"Kind":      Equal(examplePinnedObjRef.Kind),
		"Group":     Equal(examplePinnedObjRef.Group),
	})))
}

func TestNewLocalObjectReference(t *testing.T) {
	g := NewWithT(t)

	g.Expect(NewLocalObjectReference(&exampleCluster)).To(PointTo(MatchAllFields(Fields{
		"Kind":  Equal("Cluster"),
		"Name":  Equal(exampleCluster.Name),
		"Group": Equal(GroupVersion.Group),
	})))
}

func TestLocalObjectReferenceNamespacedName(t *testing.T) {
	g := NewWithT(t)

	ref := LocalObjectReference{
		Name: "test",
	}
	g.Expect(ref.NamespacedName("default")).To(MatchAllFields(Fields{
		"Name":      Equal("test"),
		"Namespace": Equal("default"),
	}))
}

var exampleLocalObjRef = LocalObjectReference{
	Name:  "test",
	Kind:  "Cluster",
	Group: GroupVersion.Group,
}

func TestLocalObjectReferencePin(t *testing.T) {
	g := NewWithT(t)

	g.Expect(exampleLocalObjRef.Pin(exampleCluster.UID)).To(PointTo(MatchAllFields(Fields{
		"Name":  Equal(exampleLocalObjRef.Name),
		"Kind":  Equal(exampleLocalObjRef.Kind),
		"Group": Equal(exampleLocalObjRef.Group),
		"UID":   Equal(exampleCluster.UID),
	})))
}

func TestNewPinnedLocalObjectReference(t *testing.T) {
	g := NewWithT(t)

	g.Expect(NewPinnedLocalObjectReference(&exampleCluster)).To(PointTo(MatchAllFields(Fields{
		"Kind":  Equal("Cluster"),
		"Name":  Equal(exampleCluster.Name),
		"Group": Equal(GroupVersion.Group),
		"UID":   Equal(exampleCluster.UID),
	})))
}

func TestPinnedLocalObjectReferenceNamespacedName(t *testing.T) {
	g := NewWithT(t)

	ref := PinnedLocalObjectReference{
		Name: "test",
	}
	g.Expect(ref.NamespacedName("default")).To(MatchAllFields(Fields{
		"Name":      Equal("test"),
		"Namespace": Equal("default"),
	}))
}

func TestPinnedLocalObjectReferenceUnpin(t *testing.T) {
	g := NewWithT(t)

	var examplePinnedObjRef = PinnedLocalObjectReference{
		Name:  "test",
		Kind:  "Cluster",
		Group: GroupVersion.Group,
	}

	g.Expect(examplePinnedObjRef.Unpin()).To(PointTo(MatchAllFields(Fields{
		"Name":  Equal(examplePinnedObjRef.Name),
		"Kind":  Equal(examplePinnedObjRef.Kind),
		"Group": Equal(examplePinnedObjRef.Group),
	})))
}
