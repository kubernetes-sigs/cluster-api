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

package patch

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestToUnstructured(t *testing.T) {
	t.Run("with a typed object", func(t *testing.T) {
		g := NewWithT(t)
		// Test with a typed object.
		obj := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-1",
				Namespace: "namespace-1",
			},
			Spec: clusterv1.ClusterSpec{
				Paused: true,
			},
		}
		gvk := schema.GroupVersionKind{
			Group:   clusterv1.GroupVersion.Group,
			Kind:    "Cluster",
			Version: clusterv1.GroupVersion.Version,
		}
		newObj, err := toUnstructured(obj, gvk)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(newObj.GetName()).To(Equal(obj.Name))
		g.Expect(newObj.GetNamespace()).To(Equal(obj.Namespace))
		g.Expect(newObj.GetAPIVersion()).To(Equal(clusterv1.GroupVersion.String()))
		g.Expect(newObj.GetKind()).To(Equal("Cluster"))

		// Change a spec field and validate that it stays the same in the incoming object.
		g.Expect(unstructured.SetNestedField(newObj.Object, false, "spec", "paused")).To(Succeed())
		g.Expect(obj.Spec.Paused).To(BeTrue())
	})

	t.Run("with an unstructured object", func(t *testing.T) {
		g := NewWithT(t)

		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "test.x.y.z/v1",
				"kind":       "TestKind",
				"metadata": map[string]interface{}{
					"name":      "test-1",
					"namespace": "namespace-1",
				},
				"spec": map[string]interface{}{
					"paused": true,
				},
			},
		}
		gvk := schema.GroupVersionKind{
			Group:   "test.x.y.z",
			Kind:    "TestKind",
			Version: "v1",
		}

		newObj, err := toUnstructured(obj, gvk)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(newObj.GetName()).To(Equal(obj.GetName()))
		g.Expect(newObj.GetNamespace()).To(Equal(obj.GetNamespace()))
		g.Expect(newObj.GetAPIVersion()).To(Equal("test.x.y.z/v1"))
		g.Expect(newObj.GetKind()).To(Equal("TestKind"))

		// Validate that the maps point to different addresses.
		g.Expect(obj.Object).ToNot(BeIdenticalTo(newObj.Object))

		// Change a spec field and validate that it stays the same in the incoming object.
		g.Expect(unstructured.SetNestedField(newObj.Object, false, "spec", "paused")).To(Succeed())
		pausedValue, _, err := unstructured.NestedBool(obj.Object, "spec", "paused")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(pausedValue).To(BeTrue())

		// Change the name of the new object and make sure it doesn't change it the old one.
		newObj.SetName("test-2")
		g.Expect(obj.GetName()).To(Equal("test-1"))
	})
}

func TestUnsafeFocusedUnstructured(t *testing.T) {
	t.Run("focus=spec, should only return spec and common fields", func(t *testing.T) {
		g := NewWithT(t)

		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "test.x.y.z/v1",
				"kind":       "TestCluster",
				"metadata": map[string]interface{}{
					"name":      "test-1",
					"namespace": "namespace-1",
				},
				"spec": map[string]interface{}{
					"paused": true,
				},
				"status": map[string]interface{}{
					"infrastructureReady": true,
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "Ready",
							"status": "True",
						},
					},
				},
			},
		}

		newObj := unsafeUnstructuredCopy(obj, specPatch, nil, []string{"status", "conditions"})

		// Validate that common fields are always preserved.
		g.Expect(newObj.Object["apiVersion"]).To(Equal(obj.Object["apiVersion"]))
		g.Expect(newObj.Object["kind"]).To(Equal(obj.Object["kind"]))
		g.Expect(newObj.Object["metadata"]).To(Equal(obj.Object["metadata"]))

		// Validate that the spec has been preserved.
		g.Expect(newObj.Object["spec"]).To(Equal(obj.Object["spec"]))

		// Validate that the status is nil, but preserved in the original object.
		g.Expect(newObj.Object["status"]).To(BeNil())
		g.Expect(obj.Object["status"]).ToNot(BeNil())
		g.Expect(obj.Object["status"].(map[string]interface{})["conditions"]).ToNot(BeNil())
	})

	t.Run("focus=status w/ condition-setter object, should only return status (without conditions) and common fields", func(t *testing.T) {
		g := NewWithT(t)

		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "test.x.y.z/v1",
				"kind":       "TestCluster",
				"metadata": map[string]interface{}{
					"name":      "test-1",
					"namespace": "namespace-1",
				},
				"spec": map[string]interface{}{
					"paused": true,
				},
				"status": map[string]interface{}{
					"infrastructureReady": true,
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "Ready",
							"status": "True",
						},
					},
				},
			},
		}

		newObj := unsafeUnstructuredCopy(obj, statusPatch, nil, []string{"status", "conditions"})

		// Validate that common fields are always preserved.
		g.Expect(newObj.Object["apiVersion"]).To(Equal(obj.Object["apiVersion"]))
		g.Expect(newObj.Object["kind"]).To(Equal(obj.Object["kind"]))
		g.Expect(newObj.Object["metadata"]).To(Equal(obj.Object["metadata"]))

		// Validate that spec is nil in the new object, but still exists in the old copy.
		g.Expect(newObj.Object["spec"]).To(BeNil())
		g.Expect(obj.Object["spec"]).To(Equal(map[string]interface{}{
			"paused": true,
		}))

		// Validate that the status has been copied, without conditions.
		g.Expect(newObj.Object["status"]).To(HaveLen(1))
		g.Expect(newObj.Object["status"].(map[string]interface{})["infrastructureReady"]).To(BeTrue())
		g.Expect(newObj.Object["status"].(map[string]interface{})["conditions"]).To(BeNil())

		// When working with conditions, the inner map is going to be removed from the original object.
		g.Expect(obj.Object["status"].(map[string]interface{})["conditions"]).To(BeNil())
	})

	t.Run("focus=status w/o condition-setter object, should only return status and common fields", func(t *testing.T) {
		g := NewWithT(t)

		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "test.x.y.z/v1",
				"kind":       "TestCluster",
				"metadata": map[string]interface{}{
					"name":      "test-1",
					"namespace": "namespace-1",
				},
				"spec": map[string]interface{}{
					"paused": true,
					"other":  "field",
				},
				"status": map[string]interface{}{
					"infrastructureReady": true,
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "Ready",
							"status": "True",
						},
					},
				},
			},
		}

		newObj := unsafeUnstructuredCopy(obj, statusPatch, nil, nil)

		// Validate that spec is nil in the new object, but still exists in the old copy.
		g.Expect(newObj.Object["spec"]).To(BeNil())
		g.Expect(obj.Object["spec"]).To(Equal(map[string]interface{}{
			"paused": true,
			"other":  "field",
		}))

		// Validate that common fields are always preserved.
		g.Expect(newObj.Object["apiVersion"]).To(Equal(obj.Object["apiVersion"]))
		g.Expect(newObj.Object["kind"]).To(Equal(obj.Object["kind"]))
		g.Expect(newObj.Object["metadata"]).To(Equal(obj.Object["metadata"]))

		// Validate that the status has been copied, without conditions.
		g.Expect(newObj.Object["status"]).To(HaveLen(2))
		g.Expect(newObj.Object["status"]).To(Equal(obj.Object["status"]))

		// Make sure that we didn't modify the incoming object if this object isn't a condition setter.
		g.Expect(obj.Object["status"].(map[string]interface{})["conditions"]).ToNot(BeNil())
	})
}

func TestIdentifyConditionsFieldsPath(t *testing.T) {
	t.Run("v1beta1 object with conditions (phase 0)", func(t *testing.T) {
		g := NewWithT(t)

		obj := &builder.Phase0Obj{}
		metav1ConditionsFields, clusterv1ConditionsFields, err := identifyConditionsFieldsPath(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(metav1ConditionsFields).To(BeEmpty())
		g.Expect(clusterv1ConditionsFields).To(Equal([]string{"status", "conditions"}))
	})
	t.Run("v1beta1 object with both clusterv1.conditions and metav1.conditions (phase 1)", func(t *testing.T) {
		g := NewWithT(t)

		tests := []struct {
			obj runtime.Object
		}{
			{obj: &builder.Phase1Obj{}},
			{obj: &builder.Phase1Obj{Status: builder.Phase1ObjStatus{V1Beta2: nil}}},
			{obj: &builder.Phase1Obj{Status: builder.Phase1ObjStatus{V1Beta2: &builder.Phase1ObjStatusV1Beta2{Conditions: nil}}}},
		}
		for _, tt := range tests {
			metav1ConditionsFields, clusterv1ConditionsFields, err := identifyConditionsFieldsPath(tt.obj)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(metav1ConditionsFields).To(Equal([]string{"status", "v1beta2", "conditions"}))
			g.Expect(clusterv1ConditionsFields).To(Equal([]string{"status", "conditions"}))
		}
	})
	t.Run("v1beta2 object with both clusterv1.conditions and metav1.conditions conditions (phase 2)", func(t *testing.T) {
		g := NewWithT(t)

		tests := []struct {
			obj runtime.Object
		}{
			{obj: &builder.Phase2Obj{}},
			{obj: &builder.Phase2Obj{Status: builder.Phase2ObjStatus{Deprecated: nil}}},
			{obj: &builder.Phase2Obj{Status: builder.Phase2ObjStatus{Deprecated: &builder.Phase2ObjStatusDeprecated{V1Beta1: nil}}}},
			{obj: &builder.Phase2Obj{Status: builder.Phase2ObjStatus{Deprecated: &builder.Phase2ObjStatusDeprecated{V1Beta1: &builder.Phase2ObjStatusDeprecatedV1Beta1{Conditions: nil}}}}},
		}
		for _, tt := range tests {
			metav1ConditionsFields, clusterv1ConditionsFields, err := identifyConditionsFieldsPath(tt.obj)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(metav1ConditionsFields).To(Equal([]string{"status", "conditions"}))
			g.Expect(clusterv1ConditionsFields).To(Equal([]string{"status", "deprecated", "v1beta1", "conditions"}))
		}
	})
	t.Run("v1beta2 object with metav1.conditions (phase 3)", func(t *testing.T) {
		g := NewWithT(t)

		obj := &builder.Phase3Obj{}
		metav1ConditionsFields, clusterv1ConditionsFields, err := identifyConditionsFieldsPath(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(metav1ConditionsFields).To(Equal([]string{"status", "conditions"}))
		g.Expect(clusterv1ConditionsFields).To(BeEmpty())
	})
}
