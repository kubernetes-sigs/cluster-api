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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestMetadata(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}

	t.Run("Manages metadata", func(t *testing.T) {
		g := NewWithT(t)

		metadata := &clusterv1.ObjectMeta{
			Labels: map[string]string{
				"label1": "labelValue1",
			},
			Annotations: map[string]string{
				"annotation1": "annotationValue1",
			},
		}

		m := Metadata{path: Path{"foo"}}
		g.Expect(m.Path()).To(Equal(Path{"foo"}))

		err := m.Set(obj, metadata)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := m.Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got).To(Equal(metadata))
	})
	t.Run("Manages empty metadata", func(t *testing.T) {
		g := NewWithT(t)

		metadata := &clusterv1.ObjectMeta{
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		}

		m := Metadata{path: Path{"foo"}}
		g.Expect(m.Path()).To(Equal(Path{"foo"}))

		err := m.Set(obj, metadata)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := m.Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got).To(Equal(metadata))
	})
}
