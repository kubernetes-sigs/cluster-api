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

package resource

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestSortForCreate(t *testing.T) {
	g := NewWithT(t)

	cm := unstructured.Unstructured{}
	cm.SetKind("ConfigMap")

	ns := unstructured.Unstructured{}
	ns.SetKind("Namespace")

	ep := unstructured.Unstructured{}
	ep.SetKind("Endpoint")

	resources := []unstructured.Unstructured{ep, cm, ns}
	sorted := SortForCreate(resources)
	g.Expect(len(sorted)).To(Equal(3))
	g.Expect(sorted[0].GetKind()).To(BeIdenticalTo("Namespace"))
	g.Expect(sorted[1].GetKind()).To(BeIdenticalTo("ConfigMap"))
}
