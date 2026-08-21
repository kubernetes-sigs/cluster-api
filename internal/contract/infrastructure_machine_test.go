/*
Copyright 2022 The Kubernetes Authors.

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

func TestInfrastructureMachine(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}

	t.Run("Manages spec.providerID", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(InfrastructureMachine().ProviderID().Path()).To(Equal(Path{"spec", "providerID"}))

		err := InfrastructureMachine().ProviderID().Set(obj, "fake-provider-id")
		g.Expect(err).ToNot(HaveOccurred())

		got, err := InfrastructureMachine().ProviderID().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal("fake-provider-id"))
	})
	t.Run("Manages status.initialization.provisioned", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(InfrastructureMachine().Provisioned("v1beta2").Path()).To(Equal(Path{"status", "initialization", "provisioned"}))

		err := InfrastructureMachine().Provisioned("v1beta2").Set(obj, true)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := InfrastructureMachine().Provisioned("v1beta2").Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(BeTrue())

		g.Expect(InfrastructureMachine().Provisioned("v1beta1").Path()).To(Equal(Path{"status", "ready"}))

		objV1beta1 := &unstructured.Unstructured{Object: map[string]interface{}{}}
		err = InfrastructureMachine().Provisioned("v1beta1").Set(objV1beta1, true)
		g.Expect(err).ToNot(HaveOccurred())

		got, err = InfrastructureMachine().Provisioned("v1beta1").Get(objV1beta1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(BeTrue())
	})
}
