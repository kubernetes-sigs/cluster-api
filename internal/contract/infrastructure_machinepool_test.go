/*
Copyright 2026 The Kubernetes Authors.

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

func TestInfrastructureMachinePool(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}

	t.Run("Manages status.initialization.provisioned", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(InfrastructureMachinePool().Provisioned("v1beta2").Path()).To(Equal(Path{"status", "initialization", "provisioned"}))

		err := InfrastructureMachinePool().Provisioned("v1beta2").Set(obj, true)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := InfrastructureMachinePool().Provisioned("v1beta2").Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(BeTrue())

		g.Expect(InfrastructureMachinePool().Provisioned("v1beta1").Path()).To(Equal(Path{"status", "ready"}))

		objV1beta1 := &unstructured.Unstructured{Object: map[string]interface{}{}}
		err = InfrastructureMachinePool().Provisioned("v1beta1").Set(objV1beta1, true)
		g.Expect(err).ToNot(HaveOccurred())

		got, err = InfrastructureMachinePool().Provisioned("v1beta1").Get(objV1beta1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(BeTrue())
	})
	t.Run("Falls back to status.ready when status.initialization.provisioned is not set", func(t *testing.T) {
		g := NewWithT(t)

		objWithFallback := &unstructured.Unstructured{Object: map[string]interface{}{}}
		err := unstructured.SetNestedField(objWithFallback.Object, true, "status", "ready")
		g.Expect(err).ToNot(HaveOccurred())

		got, err := InfrastructureMachinePool().Provisioned("v1beta2").Get(objWithFallback)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(BeTrue())
	})
	t.Run("Does not fall back to status.ready when status.initialization.provisioned is set", func(t *testing.T) {
		g := NewWithT(t)

		objWithBoth := &unstructured.Unstructured{Object: map[string]interface{}{}}
		err := unstructured.SetNestedField(objWithBoth.Object, false, "status", "initialization", "provisioned")
		g.Expect(err).ToNot(HaveOccurred())
		err = unstructured.SetNestedField(objWithBoth.Object, true, "status", "ready")
		g.Expect(err).ToNot(HaveOccurred())

		got, err := InfrastructureMachinePool().Provisioned("v1beta2").Get(objWithBoth)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(BeFalse())
	})
	t.Run("Returns ErrFieldNotFound when neither status.initialization.provisioned nor status.ready are set", func(t *testing.T) {
		g := NewWithT(t)

		emptyObj := &unstructured.Unstructured{Object: map[string]interface{}{}}

		_, err := InfrastructureMachinePool().Provisioned("v1beta2").Get(emptyObj)
		g.Expect(err).To(MatchError(ErrFieldNotFound))
	})
	t.Run("Manages spec.providerIDList", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(InfrastructureMachinePool().ProviderIDList().Path()).To(Equal(Path{"spec", "providerIDList"}))

		err := InfrastructureMachinePool().ProviderIDList().Set(obj, []string{"fake-provider-id-1", "fake-provider-id-2"})
		g.Expect(err).ToNot(HaveOccurred())

		got, err := InfrastructureMachinePool().ProviderIDList().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal([]string{"fake-provider-id-1", "fake-provider-id-2"}))
	})
	t.Run("Manages status.replicas", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(InfrastructureMachinePool().Replicas().Path()).To(Equal(Path{"status", "replicas"}))

		err := InfrastructureMachinePool().Replicas().Set(obj, 3)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := InfrastructureMachinePool().Replicas().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(int32(3)))
	})
	t.Run("Manages optional status.infrastructureMachineKind", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(InfrastructureMachinePool().InfrastructureMachineKind().Path()).To(Equal(Path{"status", "infrastructureMachineKind"}))

		err := InfrastructureMachinePool().InfrastructureMachineKind().Set(obj, "FakeMachine")
		g.Expect(err).ToNot(HaveOccurred())

		got, err := InfrastructureMachinePool().InfrastructureMachineKind().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal("FakeMachine"))
	})
}
