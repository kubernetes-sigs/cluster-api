// +build e2e

/*
Copyright 2018 The Kubernetes Authors.

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

package e2e_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capiv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Cluster API Validation", func() {
	It("verifies validation is setup correctly and works", func() {
		bootstrapData := "some valid data"
		name := "capi-test-machine-" + util.RandomString(6)
		machine := &capiv1.Machine{
			ObjectMeta: v1.ObjectMeta{
				Name:      name,
				Namespace: capiTestNamespace,
			},
			Spec: capiv1.MachineSpec{
				ClusterName: name,
				Bootstrap: capiv1.Bootstrap{
					Data: &bootstrapData,
				},
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
					Kind:       "InfrastructureConfig",
					Name:       "infra-config1",
				},
			},
		}

		err := kindClient.Create(ctx, machine)
		Expect(err).ToNot(HaveOccurred())

		// Confirm that object exists
		Eventually(func() error {
			m := &capiv1.Machine{}
			key := client.ObjectKey{
				Namespace: capiTestNamespace,
				Name:      name,
			}
			if err := kindClient.Get(ctx, key, m); err != nil {
				return err
			}
			return nil
		}, 10*time.Second).ShouldNot(HaveOccurred())
	})

	It("returns error when validation fails", func() {
		// Create invalid machine object - no bootstrap config or data.
		name := "capi-test-machine-" + util.RandomString(6)
		machine := &capiv1.Machine{
			ObjectMeta: v1.ObjectMeta{
				Name:      name,
				Namespace: capiTestNamespace,
			},
			Spec: capiv1.MachineSpec{
				ClusterName: name,
				Bootstrap:   capiv1.Bootstrap{},
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
					Kind:       "InfrastructureConfig",
					Name:       "infra-config1",
				},
			},
		}

		err := kindClient.Create(ctx, machine)
		// TODO: Should we assert the error contents to ensure it is a
		// validation error?
		Expect(err).To(HaveOccurred())
	})
})
