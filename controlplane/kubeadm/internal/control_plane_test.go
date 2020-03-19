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

package internal

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
)

func TestControlPlane(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Control Plane Suite")
}

var _ = Describe("Control Plane", func() {
	Describe("MachinesNeedingUpgrade", func() {
		var controlPlane *ControlPlane
		BeforeEach(func() {
			controlPlane = &ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{},
			}
		})

		Context("With no machines", func() {
			It("should return no machines", func() {
				Expect(controlPlane.MachinesNeedingUpgrade()).To(HaveLen(0))
			})
		})

		Context("With machines", func() {
			BeforeEach(func() {
				controlPlane.Machines = FilterableMachineCollection{
					"machine-1": machine("machine-1"),
				}
			})
			Context("That have an old configuration", func() {
				It("should return some machines", func() {
					Expect(controlPlane.MachinesNeedingUpgrade()).ToNot(HaveLen(0))
				})
			})

			Context("That have an up-to-date configuration", func() {
				Context("That has no upgradeAfter value set", func() {
					PIt("should return no machines", func() {})
				})

				Context("That has an upgradeAfter value set", func() {
					Context("That is in the future", func() {
						PIt("should return no machines", func() {})
					})

					Context("That is in the past", func() {
						Context("That is before machine creation time", func() {
							PIt("should return no machines", func() {})
						})
						Context("That is after machine creation time", func() {
							PIt("should return no machines", func() {})
						})
					})
				})
			})
		})

	})
})
