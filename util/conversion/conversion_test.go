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

package conversion

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

var (
	oldMachineGVK = schema.GroupVersionKind{
		Group:   clusterv1.GroupVersion.Group,
		Version: "v1old",
		Kind:    "Machine",
	}
)

func TestMarshalData(t *testing.T) {
	g := NewWithT(t)

	t.Run("should write source object to destination", func(t *testing.T) {
		version := "v1.16.4"
		providerID := "aws://some-id"
		src := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
				Labels: map[string]string{
					"label1": "",
				},
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: "test-cluster",
				Version:     &version,
				ProviderID:  &providerID,
			},
		}

		dst := &unstructured.Unstructured{}
		dst.SetGroupVersionKind(oldMachineGVK)
		dst.SetName("test-1")

		g.Expect(MarshalData(src, dst)).To(Succeed())
		// ensure the src object is not modified
		g.Expect(src.GetLabels()).ToNot(BeEmpty())

		g.Expect(dst.GetAnnotations()[DataAnnotation]).ToNot(BeEmpty())
		g.Expect(dst.GetAnnotations()[DataAnnotation]).To(ContainSubstring("test-cluster"))
		g.Expect(dst.GetAnnotations()[DataAnnotation]).To(ContainSubstring("v1.16.4"))
		g.Expect(dst.GetAnnotations()[DataAnnotation]).To(ContainSubstring("aws://some-id"))
		g.Expect(dst.GetAnnotations()[DataAnnotation]).ToNot(ContainSubstring("metadata"))
		g.Expect(dst.GetAnnotations()[DataAnnotation]).ToNot(ContainSubstring("label1"))
	})

	t.Run("should append the annotation", func(t *testing.T) {
		src := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
			},
		}
		dst := &unstructured.Unstructured{}
		dst.SetGroupVersionKind(clusterv1.GroupVersion.WithKind("Machine"))
		dst.SetName("test-1")
		dst.SetAnnotations(map[string]string{
			"annotation": "1",
		})

		g.Expect(MarshalData(src, dst)).To(Succeed())
		g.Expect(len(dst.GetAnnotations())).To(Equal(2))
	})
}

func TestUnmarshalData(t *testing.T) {
	g := NewWithT(t)

	t.Run("should return false without errors if annotation doesn't exist", func(t *testing.T) {
		src := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
			},
		}
		dst := &unstructured.Unstructured{}
		dst.SetGroupVersionKind(oldMachineGVK)
		dst.SetName("test-1")

		ok, err := UnmarshalData(src, dst)
		g.Expect(ok).To(BeFalse())
		g.Expect(err).To(BeNil())
	})

	t.Run("should return true when a valid annotation with data exists", func(t *testing.T) {
		src := &unstructured.Unstructured{}
		src.SetGroupVersionKind(oldMachineGVK)
		src.SetName("test-1")
		src.SetAnnotations(map[string]string{
			DataAnnotation: "{\"metadata\":{\"name\":\"test-1\",\"creationTimestamp\":null,\"labels\":{\"label1\":\"\"}},\"spec\":{\"clusterName\":\"\",\"bootstrap\":{},\"infrastructureRef\":{}},\"status\":{\"bootstrapReady\":true,\"infrastructureReady\":true}}",
		})

		dst := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
			},
		}

		ok, err := UnmarshalData(src, dst)
		g.Expect(err).To(BeNil())
		g.Expect(ok).To(BeTrue())

		g.Expect(len(dst.GetLabels())).To(Equal(1))
		g.Expect(dst.GetName()).To(Equal("test-1"))
		g.Expect(dst.GetLabels()).To(HaveKeyWithValue("label1", ""))
		g.Expect(dst.GetAnnotations()).To(BeEmpty())
	})

	t.Run("should clean the annotation on successful unmarshal", func(t *testing.T) {
		src := &unstructured.Unstructured{}
		src.SetGroupVersionKind(oldMachineGVK)
		src.SetName("test-1")
		src.SetAnnotations(map[string]string{
			"annotation-1": "",
			DataAnnotation: "{\"metadata\":{\"name\":\"test-1\",\"creationTimestamp\":null,\"labels\":{\"label1\":\"\"}},\"spec\":{\"clusterName\":\"\",\"bootstrap\":{},\"infrastructureRef\":{}},\"status\":{\"bootstrapReady\":true,\"infrastructureReady\":true}}",
		})

		dst := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
			},
		}

		ok, err := UnmarshalData(src, dst)
		g.Expect(err).To(BeNil())
		g.Expect(ok).To(BeTrue())

		g.Expect(src.GetAnnotations()).ToNot(HaveKey(DataAnnotation))
		g.Expect(len(src.GetAnnotations())).To(Equal(1))
	})
}
