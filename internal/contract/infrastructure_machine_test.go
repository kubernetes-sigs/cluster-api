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

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
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
	t.Run("Manages status.ready", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(InfrastructureMachine().Ready().Path()).To(Equal(Path{"status", "ready"}))

		err := InfrastructureMachine().Ready().Set(obj, true)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := InfrastructureMachine().Ready().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(BeTrue())
	})
	t.Run("Manages optional status.failureReason", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(InfrastructureMachine().FailureReason().Path()).To(Equal(Path{"status", "failureReason"}))

		err := InfrastructureMachine().FailureReason().Set(obj, "fake-reason")
		g.Expect(err).ToNot(HaveOccurred())

		got, err := InfrastructureMachine().FailureReason().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal("fake-reason"))
	})
	t.Run("Manages optional status.failureMessage", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(InfrastructureMachine().FailureMessage().Path()).To(Equal(Path{"status", "failureMessage"}))

		err := InfrastructureMachine().FailureMessage().Set(obj, "fake-message")
		g.Expect(err).ToNot(HaveOccurred())

		got, err := InfrastructureMachine().FailureMessage().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal("fake-message"))
	})
	t.Run("Manages optional status.addresses", func(t *testing.T) {
		g := NewWithT(t)

		addresses := []clusterv1.MachineAddress{
			{
				Type:    clusterv1.MachineHostName,
				Address: "fake-address-1",
			},
			{
				Type:    clusterv1.MachineInternalIP,
				Address: "fake-address-2",
			},
		}
		g.Expect(InfrastructureMachine().Addresses().Path()).To(Equal(Path{"status", "addresses"}))

		err := InfrastructureMachine().Addresses().Set(obj, addresses)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := InfrastructureMachine().Addresses().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(addresses))
	})
	t.Run("Manages optional spec.failureDomain", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(InfrastructureMachine().FailureDomain().Path()).To(Equal(Path{"spec", "failureDomain"}))

		err := InfrastructureMachine().FailureDomain().Set(obj, "fake-failure-domain")
		g.Expect(err).ToNot(HaveOccurred())

		got, err := InfrastructureMachine().FailureDomain().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal("fake-failure-domain"))
	})
}
