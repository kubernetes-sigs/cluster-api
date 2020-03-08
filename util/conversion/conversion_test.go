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
	clusterv1a2 "sigs.k8s.io/cluster-api/api/v1alpha3"
	clusterv1a3 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

func TestMarshalData(t *testing.T) {
	g := NewWithT(t)

	t.Run("should write source object to destination", func(t *testing.T) {
		version := "v1.16.4"
		providerID := "aws://some-id"
		src := &clusterv1a3.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
				Labels: map[string]string{
					"label1": "",
				},
			},
			Spec: clusterv1a3.MachineSpec{
				ClusterName: "test-cluster",
				Version:     &version,
				ProviderID:  &providerID,
			},
		}

		dst := &clusterv1a2.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
			},
		}

		g.Expect(MarshalData(src, dst)).To(Succeed())
		// ensure the src object is not modified
		g.Expect(src.GetLabels()).ToNot(BeEmpty())

		g.Expect(dst.Annotations[DataAnnotation]).ToNot(BeEmpty())
		g.Expect(dst.Annotations[DataAnnotation]).To(ContainSubstring("test-cluster"))
		g.Expect(dst.Annotations[DataAnnotation]).To(ContainSubstring("v1.16.4"))
		g.Expect(dst.Annotations[DataAnnotation]).To(ContainSubstring("aws://some-id"))
		g.Expect(dst.Annotations[DataAnnotation]).ToNot(ContainSubstring("metadata"))
		g.Expect(dst.Annotations[DataAnnotation]).ToNot(ContainSubstring("label1"))
	})

	t.Run("should append the annotation", func(t *testing.T) {
		src := &clusterv1a3.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
			},
		}
		dst := &clusterv1a2.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
				Annotations: map[string]string{
					"annotation": "1",
				},
			},
		}

		g.Expect(MarshalData(src, dst)).To(Succeed())
		g.Expect(len(dst.Annotations)).To(Equal(2))
	})
}

func TestUnmarshalData(t *testing.T) {
	g := NewWithT(t)

	t.Run("should return false without errors if annotation doesn't exist", func(t *testing.T) {
		src := &clusterv1a3.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
			},
		}
		dst := &clusterv1a2.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
			},
		}

		ok, err := UnmarshalData(src, dst)
		g.Expect(ok).To(BeFalse())
		g.Expect(err).To(BeNil())
	})

	t.Run("should return true when a valid annotation with data exists", func(t *testing.T) {
		src := &clusterv1a3.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
				Annotations: map[string]string{
					DataAnnotation: "{\"metadata\":{\"name\":\"test-1\",\"creationTimestamp\":null,\"labels\":{\"label1\":\"\"}},\"spec\":{\"clusterName\":\"\",\"bootstrap\":{},\"infrastructureRef\":{}},\"status\":{\"bootstrapReady\":true,\"infrastructureReady\":true}}",
				},
			},
		}
		dst := &clusterv1a2.Machine{}

		ok, err := UnmarshalData(src, dst)
		g.Expect(ok).To(BeTrue())
		g.Expect(err).To(BeNil())

		g.Expect(len(dst.Labels)).To(Equal(1))
		g.Expect(dst.Name).To(Equal("test-1"))
		g.Expect(dst.Labels).To(HaveKeyWithValue("label1", ""))
		g.Expect(dst.Annotations).To(BeEmpty())
	})

	t.Run("should clean the annotation on successful unmarshal", func(t *testing.T) {
		src := &clusterv1a3.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
				Annotations: map[string]string{
					"annotation-1": "",
					DataAnnotation: "{\"metadata\":{\"name\":\"test-1\",\"creationTimestamp\":null,\"labels\":{\"label1\":\"\"}},\"spec\":{\"clusterName\":\"\",\"bootstrap\":{},\"infrastructureRef\":{}},\"status\":{\"bootstrapReady\":true,\"infrastructureReady\":true}}",
				},
			},
		}
		dst := &clusterv1a2.Machine{}

		ok, err := UnmarshalData(src, dst)
		g.Expect(ok).To(BeTrue())
		g.Expect(err).To(BeNil())

		g.Expect(src.Annotations).ToNot(HaveKey(DataAnnotation))
		g.Expect(len(src.Annotations)).To(Equal(1))
	})
}
