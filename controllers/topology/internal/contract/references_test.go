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

package contract

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var fooRefBuilder = func() *unstructured.Unstructured {
	refObj := &unstructured.Unstructured{}
	refObj.SetAPIVersion("fooApiVersion")
	refObj.SetKind("fooKind")
	refObj.SetNamespace("fooNamespace")
	refObj.SetName("fooName")
	return refObj
}

func TestGetNestedRef(t *testing.T) {
	t.Run("Gets a nested ref if defined", func(t *testing.T) {
		g := NewWithT(t)

		refObj := fooRefBuilder()
		obj := &unstructured.Unstructured{Object: map[string]interface{}{}}

		err := SetNestedRef(obj, refObj, "spec", "machineTemplate", "infrastructureRef")
		g.Expect(err).To(BeNil())

		ref, err := GetNestedRef(obj, "spec", "machineTemplate", "infrastructureRef")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ref).ToNot(BeNil())
		g.Expect(ref.APIVersion).To(Equal(refObj.GetAPIVersion()))
		g.Expect(ref.Kind).To(Equal(refObj.GetKind()))
		g.Expect(ref.Name).To(Equal(refObj.GetName()))
		g.Expect(ref.Namespace).To(Equal(refObj.GetNamespace()))
	})
	t.Run("getNestedRef fails if the nested ref does not exist", func(t *testing.T) {
		g := NewWithT(t)

		obj := &unstructured.Unstructured{Object: map[string]interface{}{}}

		ref, err := GetNestedRef(obj, "spec", "machineTemplate", "infrastructureRef")
		g.Expect(err).To(HaveOccurred())
		g.Expect(ref).To(BeNil())
	})
	t.Run("getNestedRef fails if the nested ref exist but it is incomplete", func(t *testing.T) {
		g := NewWithT(t)

		obj := &unstructured.Unstructured{Object: map[string]interface{}{}}

		err := unstructured.SetNestedField(obj.UnstructuredContent(), "foo", "spec", "machineTemplate", "infrastructureRef", "kind")
		g.Expect(err).ToNot(HaveOccurred())
		err = unstructured.SetNestedField(obj.UnstructuredContent(), "bar", "spec", "machineTemplate", "infrastructureRef", "namespace")
		g.Expect(err).ToNot(HaveOccurred())
		err = unstructured.SetNestedField(obj.UnstructuredContent(), "baz", "spec", "machineTemplate", "infrastructureRef", "apiVersion")
		g.Expect(err).ToNot(HaveOccurred())
		// Reference name missing

		ref, err := GetNestedRef(obj, "spec", "machineTemplate", "infrastructureRef")
		g.Expect(err).To(HaveOccurred())
		g.Expect(ref).To(BeNil())
	})
}

func TestSetNestedRef(t *testing.T) {
	t.Run("Sets a nested ref", func(t *testing.T) {
		g := NewWithT(t)

		refObj := fooRefBuilder()
		obj := &unstructured.Unstructured{Object: map[string]interface{}{}}

		err := SetNestedRef(obj, refObj, "spec", "machineTemplate", "infrastructureRef")
		g.Expect(err).To(BeNil())

		ref, err := GetNestedRef(obj, "spec", "machineTemplate", "infrastructureRef")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ref).ToNot(BeNil())
		g.Expect(ref.APIVersion).To(Equal(refObj.GetAPIVersion()))
		g.Expect(ref.Kind).To(Equal(refObj.GetKind()))
		g.Expect(ref.Name).To(Equal(refObj.GetName()))
		g.Expect(ref.Namespace).To(Equal(refObj.GetNamespace()))
	})
}

func TestObjToRef(t *testing.T) {
	t.Run("Gets a ref from an obj", func(t *testing.T) {
		g := NewWithT(t)

		refObj := fooRefBuilder()
		ref := ObjToRef(refObj)

		g.Expect(ref).ToNot(BeNil())
		g.Expect(ref.APIVersion).To(Equal(refObj.GetAPIVersion()))
		g.Expect(ref.Kind).To(Equal(refObj.GetKind()))
		g.Expect(ref.Name).To(Equal(refObj.GetName()))
		g.Expect(ref.Namespace).To(Equal(refObj.GetNamespace()))
	})
}
