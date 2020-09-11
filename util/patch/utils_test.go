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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
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
		newObj, err := toUnstructured(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(newObj.GetName()).To(Equal(obj.Name))
		g.Expect(newObj.GetNamespace()).To(Equal(obj.Namespace))

		// Change a spec field and validate that it stays the same in the incoming object.
		g.Expect(unstructured.SetNestedField(newObj.Object, false, "spec", "paused")).To(Succeed())
		g.Expect(obj.Spec.Paused).To(BeTrue())
	})

	t.Run("with an unstructured object", func(t *testing.T) {
		g := NewWithT(t)

		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "test.x.y.z/v1",
				"metadata": map[string]interface{}{
					"name":      "test-1",
					"namespace": "namespace-1",
				},
				"spec": map[string]interface{}{
					"paused": true,
				},
			},
		}

		newObj, err := toUnstructured(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(newObj.GetName()).To(Equal(obj.GetName()))
		g.Expect(newObj.GetNamespace()).To(Equal(obj.GetNamespace()))

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
