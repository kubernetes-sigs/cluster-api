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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
)

func TestControlPlane(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Control Plane Suite")
}

var _ = Describe("Control Plane", func() {
	var controlPlane *ControlPlane
	BeforeEach(func() {
		controlPlane = &ControlPlane{
			KCP:     &controlplanev1.KubeadmControlPlane{},
			Cluster: &clusterv1.Cluster{},
		}
	})

	Describe("Failure domains", func() {
		BeforeEach(func() {
			controlPlane.Machines = FilterableMachineCollection{
				"machine-1": machine("machine-1", withFailureDomain("one")),
				"machine-2": machine("machine-2", withFailureDomain("two")),
				"machine-3": machine("machine-3", withFailureDomain("two")),
			}
			controlPlane.Cluster.Status.FailureDomains = clusterv1.FailureDomains{
				"one":   failureDomain(true),
				"two":   failureDomain(true),
				"three": failureDomain(true),
				"four":  failureDomain(false),
			}
		})

		Describe("With most machines", func() {
			Context("With all machines in known failure domains", func() {
				It("should return the failure domain that has the most number of machines", func() {
					Expect(*controlPlane.FailureDomainWithMostMachines(controlPlane.Machines)).To(Equal("two"))
				})
			})
			Context("With some machines in non-defined failure domains", func() {
				JustBeforeEach(func() {
					controlPlane.Machines.Insert(machine("machine-5", withFailureDomain("unknown")))
				})
				It("should return machines in non-defined failure domains first", func() {
					Expect(*controlPlane.FailureDomainWithMostMachines(controlPlane.Machines)).To(Equal("unknown"))
				})
			})
		})

		Describe("With fewest machines", func() {
			Context("With machines all in the known failure domains", func() {
				It("should return the failure domain that has the fewest number of machines", func() {
					Expect(*controlPlane.FailureDomainWithFewestMachines()).To(Equal("three"))
				})
			})

		})
	})

	Describe("MachinesNeedingUpgrade", func() {
		Context("With no machines", func() {
			It("should return no machines", func() {
				Expect(controlPlane.MachinesNeedingUpgrade()).To(HaveLen(0))
			})
		})

		Context("With machines", func() {
			BeforeEach(func() {
				controlPlane.KCP.Spec.Version = "2"
				controlPlane.Machines = FilterableMachineCollection{
					"machine-1": machine("machine-1", withHash(controlPlane.SpecHash())),
					"machine-2": machine("machine-2", withHash(controlPlane.SpecHash())),
					"machine-3": machine("machine-3", withHash(controlPlane.SpecHash())),
				}
			})

			Context("That have an old configuration", func() {
				JustBeforeEach(func() {
					controlPlane.Machines.Insert(machine("machine-4", withHash(controlPlane.SpecHash()+"outdated")))
				})
				It("should return some machines", func() {
					Expect(controlPlane.MachinesNeedingUpgrade()).To(HaveLen(1))
				})
			})

			Context("That have an up-to-date configuration", func() {
				year := 2000
				BeforeEach(func() {
					controlPlane.Machines = FilterableMachineCollection{
						"machine-1": machine("machine-1",
							withCreationTimestamp(metav1.Time{Time: time.Date(year-1, 0, 0, 0, 0, 0, 0, time.UTC)}),
							withHash(controlPlane.SpecHash())),
						"machine-2": machine("machine-2",
							withCreationTimestamp(metav1.Time{Time: time.Date(year, 0, 0, 0, 0, 0, 0, time.UTC)}),
							withHash(controlPlane.SpecHash())),
						"machine-3": machine("machine-3",
							withCreationTimestamp(metav1.Time{Time: time.Date(year+1, 0, 0, 0, 0, 0, 0, time.UTC)}),
							withHash(controlPlane.SpecHash())),
					}
				})

				Context("That has no upgradeAfter value set", func() {
					It("should return no machines", func() {
						Expect(controlPlane.MachinesNeedingUpgrade()).To(HaveLen(0))
					})
				})

				Context("That has an upgradeAfter value set", func() {
					Context("That is in the future", func() {
						BeforeEach(func() {
							future := time.Date(year+1000, 0, 0, 0, 0, 0, 0, time.UTC)
							controlPlane.KCP.Spec.UpgradeAfter = &metav1.Time{Time: future}
						})
						It("should return no machines", func() {
							Expect(controlPlane.MachinesNeedingUpgrade()).To(HaveLen(0))
						})
					})

					Context("That is in the past", func() {
						Context("That is before machine creation time", func() {
							JustBeforeEach(func() {
								controlPlane.KCP.Spec.UpgradeAfter = &metav1.Time{Time: time.Date(year-2, 0, 0, 0, 0, 0, 0, time.UTC)}
							})
							It("should return no machines", func() {
								Expect(controlPlane.MachinesNeedingUpgrade()).To(HaveLen(0))
							})
						})

						Context("That is after machine creation time", func() {
							JustBeforeEach(func() {
								controlPlane.KCP.Spec.UpgradeAfter = &metav1.Time{Time: time.Date(year, 1, 0, 0, 0, 0, 0, time.UTC)}
							})
							It("should return all machines older than this date machines", func() {
								Expect(controlPlane.MachinesNeedingUpgrade()).To(HaveLen(2))
							})
						})
					})
				})
			})
		})
	})
})

func failureDomain(controlPlane bool) clusterv1.FailureDomainSpec {
	return clusterv1.FailureDomainSpec{
		ControlPlane: controlPlane,
	}
}

func withFailureDomain(fd string) machineOpt {
	return func(m *clusterv1.Machine) {
		m.Spec.FailureDomain = &fd
	}
}

func withHash(hash string) machineOpt {
	return func(m *clusterv1.Machine) {
		m.SetLabels(map[string]string{controlplanev1.KubeadmControlPlaneHashLabelKey: hash})
	}
}
