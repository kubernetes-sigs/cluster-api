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

func TestControlPlaneTemplate(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}

	t.Run("Manages spec.template.spec.machineTemplate.infrastructureRef", func(t *testing.T) {
		g := NewWithT(t)

		refObj := fooRefBuilder()

		g.Expect(ControlPlaneTemplate().InfrastructureMachineTemplate().Path()).To(Equal(Path{"spec", "template", "spec", "machineTemplate", "infrastructureRef"}))

		err := ControlPlaneTemplate().InfrastructureMachineTemplate().Set(obj, refObj)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := ControlPlaneTemplate().InfrastructureMachineTemplate().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.APIVersion).To(Equal(refObj.GetAPIVersion()))
		g.Expect(got.Kind).To(Equal(refObj.GetKind()))
		g.Expect(got.Name).To(Equal(refObj.GetName()))
		g.Expect(got.Namespace).To(Equal(refObj.GetNamespace()))
	})
}
