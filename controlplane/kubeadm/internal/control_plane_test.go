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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/klogr"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/hash"
	capierrors "sigs.k8s.io/cluster-api/errors"
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

	Describe("Generating components", func() {
		Context("That is after machine creation time", func() {
			BeforeEach(func() {
				controlPlane.KCP = &controlplanev1.KubeadmControlPlane{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cp",
						UID:  types.UID("test-uid"),
					},
				}
				controlPlane.Cluster = &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-cluster",
					},
				}
			})
			It("should generate kubeadmconfig without controller reference", func() {
				spec := &bootstrapv1.KubeadmConfigSpec{}
				kubeadmConfig := controlPlane.GenerateKubeadmConfig(spec)
				Expect(kubeadmConfig.Labels["cluster.x-k8s.io/cluster-name"]).To(Equal("test-cluster"))
				Expect(kubeadmConfig.Labels["kubeadm.controlplane.cluster.x-k8s.io/hash"]).ToNot(BeEmpty())
				Expect(kubeadmConfig.OwnerReferences[0].Controller).To(BeNil())
			})
			It("should generate new machine with controller reference", func() {
				machine := controlPlane.NewMachine(&corev1.ObjectReference{Namespace: "foobar"}, &corev1.ObjectReference{Namespace: "foobar"}, pointer.StringPtr("failureDomain"))
				Expect(machine.OwnerReferences[0].Controller).ToNot(BeNil())
			})
		})
	})

})

func TestNextMachineForScaleDown(t *testing.T) {
	kcp := controlplanev1.KubeadmControlPlane{
		Spec: controlplanev1.KubeadmControlPlaneSpec{},
	}
	startDate := time.Date(2000, 1, 1, 1, 0, 0, 0, time.UTC)
	m1 := machine("machine-1", withFailureDomain("one"), withTimestamp(startDate.Add(time.Hour)), withValidHash(kcp.Spec))
	m2 := machine("machine-2", withFailureDomain("one"), withTimestamp(startDate.Add(-3*time.Hour)), withValidHash(kcp.Spec))
	m3 := machine("machine-3", withFailureDomain("one"), withTimestamp(startDate.Add(-4*time.Hour)), withValidHash(kcp.Spec))
	m4 := machine("machine-4", withFailureDomain("two"), withTimestamp(startDate.Add(-time.Hour)), withValidHash(kcp.Spec))
	m5 := machine("machine-5", withFailureDomain("two"), withTimestamp(startDate.Add(-2*time.Hour)), withHash("shrug"))
	m6 := machine("machine-6", withFailureDomain("two"), withTimestamp(startDate.Add(-2*time.Hour)), withValidHash(kcp.Spec), withUnhealthyAnnotation())

	fd := clusterv1.FailureDomains{
		"one": failureDomain(true),
		"two": failureDomain(true),
	}
	upToDateControlPlane := &ControlPlane{
		KCP:      &kcp,
		Cluster:  &clusterv1.Cluster{Status: clusterv1.ClusterStatus{FailureDomains: fd}},
		Machines: NewFilterableMachineCollection(m1, m2, m3, m4),
		Logger:   klogr.New(),
	}
	needsUpgradeControlPlane := &ControlPlane{
		KCP:      &kcp,
		Cluster:  &clusterv1.Cluster{Status: clusterv1.ClusterStatus{FailureDomains: fd}},
		Machines: NewFilterableMachineCollection(m1, m2, m3, m4, m5),
		Logger:   klogr.New(),
	}
	unhealthyControlPlane := &ControlPlane{
		KCP:      &kcp,
		Cluster:  &clusterv1.Cluster{Status: clusterv1.ClusterStatus{FailureDomains: fd}},
		Machines: NewFilterableMachineCollection(m1, m2, m3, m4, m5, m6),
		Logger:   klogr.New(),
	}

	testCases := []struct {
		name            string
		cp              *ControlPlane
		expectedMachine clusterv1.Machine
	}{
		{
			name:            "when there are unhealthy machines, it returns the oldest unhealthy machine in the failure domain with the most unhealthy machines",
			cp:              unhealthyControlPlane,
			expectedMachine: clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-6"}},
		},
		{
			name:            "when there are are machines needing upgrade, it returns the oldest machine in the failure domain with the most machines needing upgrade",
			cp:              needsUpgradeControlPlane,
			expectedMachine: clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-5"}},
		},
		{
			name:            "when there are no outdated machines, it returns the oldest machine in the largest failure domain",
			cp:              upToDateControlPlane,
			expectedMachine: clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-3"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())

			selectedMachine := tc.cp.NextMachineForScaleDown()

			g.Expect(selectedMachine.Name).To(Equal(tc.expectedMachine.Name))
		})
	}
}

func TestControlPlaneScalingLogic(t *testing.T) {
	testCases := []struct {
		name string
		test func(g *WithT, cp *ControlPlane)
	}{
		{
			name: "it needs initialization when there are no machines and any desired replicas",
			test: func(g *WithT, cp *ControlPlane) {
				cp.KCP.Spec.Replicas = pointer.Int32Ptr(0)
				g.Expect(cp.NeedsInitialization()).To(BeFalse())

				cp.KCP.Spec.Replicas = pointer.Int32Ptr(1)
				g.Expect(cp.NeedsInitialization()).To(BeTrue())

				cp.Machines = NewFilterableMachineCollection(machine("1"))
				g.Expect(cp.NeedsInitialization()).To(BeFalse())
			},
		},
		{
			name: "it needs scale up when it is missing replicas",
			test: func(g *WithT, cp *ControlPlane) {
				cp.KCP.Spec.Replicas = pointer.Int32Ptr(0)
				cp.Machines = NewFilterableMachineCollection()
				g.Expect(cp.NeedsScaleUp()).To(BeFalse())
				g.Expect(cp.NeedsScaleDown()).To(BeFalse())

				cp.KCP.Spec.Replicas = pointer.Int32Ptr(1)
				g.Expect(cp.NeedsScaleUp()).To(BeTrue())
				g.Expect(cp.NeedsScaleDown()).To(BeFalse())
			},
		},
		{
			name: "it needs scale up one machine at a time when there are outdated machines",
			test: func(g *WithT, cp *ControlPlane) {
				cp.KCP.Spec.Replicas = pointer.Int32Ptr(1)
				cp.Machines = NewFilterableMachineCollection(machine("1", withValidHash(cp.KCP.Spec)))
				g.Expect(cp.NeedsScaleUp()).To(BeFalse())
				g.Expect(cp.NeedsScaleDown()).To(BeFalse())

				cp.KCP.Spec.Version = "1.2.0"
				g.Expect(cp.NeedsScaleUp()).To(BeTrue())
				g.Expect(cp.NeedsScaleDown()).To(BeFalse())

				cp.Machines = NewFilterableMachineCollection(machine("1", withValidHash(cp.KCP.Spec)), machine("2", withValidHash(cp.KCP.Spec)))
				g.Expect(cp.NeedsScaleUp()).To(BeFalse())
				g.Expect(cp.NeedsScaleDown()).To(BeTrue())
			},
		},
		{
			name: "it needs scale down when the cluster has at least 3 machines and any are unhealthy",
			test: func(g *WithT, cp *ControlPlane) {
				cp.KCP.Spec.Replicas = pointer.Int32Ptr(2)
				cp.Machines = NewFilterableMachineCollection(
					machine("1", withValidHash(cp.KCP.Spec), withUnhealthyAnnotation()),
					machine("2", withValidHash(cp.KCP.Spec)),
				)
				g.Expect(cp.NeedsScaleUp()).To(BeFalse())
				g.Expect(cp.NeedsScaleDown()).To(BeFalse())

				cp.KCP.Spec.Replicas = pointer.Int32Ptr(3)
				cp.Machines = NewFilterableMachineCollection(
					machine("1", withUnhealthyAnnotation()),
					machine("2", withValidHash(cp.KCP.Spec)),
					machine("3", withValidHash(cp.KCP.Spec)),
				)
				g.Expect(cp.NeedsScaleUp()).To(BeFalse())
				g.Expect(cp.NeedsScaleDown()).To(BeTrue())

				cp.Machines = NewFilterableMachineCollection(
					machine("1", withValidHash(cp.KCP.Spec)),
					machine("2", withValidHash(cp.KCP.Spec)),
					machine("3", withValidHash(cp.KCP.Spec)),
				)
				g.Expect(cp.NeedsScaleUp()).To(BeFalse())
				g.Expect(cp.NeedsScaleDown()).To(BeFalse())
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			kcp := &controlplanev1.KubeadmControlPlane{}
			cp := &ControlPlane{KCP: kcp, Logger: klogr.New()}
			tc.test(g, cp)
		})
	}
}

func TestMachinePhaseFilters(t *testing.T) {
	testCases := []struct {
		name string
		test func(g *WithT, cp *ControlPlane)
	}{
		{
			name: "machines without a node or infrastructure ready are provisioning",
			test: func(g *WithT, cp *ControlPlane) {
				cp.Machines = NewFilterableMachineCollection(
					machine("1", withNodeRef(), withInfrastructureReady()),
					machine("2"),
					machine("3", withNodeRef(), withInfrastructureReady()),
				)
				g.Expect(cp.ProvisioningMachines().Names()).To(ConsistOf("2"))
				g.Expect(cp.ReadyMachines().Names()).To(ConsistOf("1", "3"))
				g.Expect(cp.UnhealthyMachines().Names()).To(BeEmpty())
				g.Expect(cp.FailedMachines().Names()).To(BeEmpty())
			},
		},
		{
			name: "machines with a failure message or reason are not provisioning or ready",
			test: func(g *WithT, cp *ControlPlane) {
				cp.Machines = NewFilterableMachineCollection(
					machine("1", withNodeRef(), withFailureReason("foo")),
					machine("2", withNodeRef(), withFailureMessage("foo")),
					machine("3", withInfrastructureReady(), withFailureReason("bar")),
					machine("4", withInfrastructureReady(), withFailureMessage("bar")),
					machine("5", withInfrastructureReady(), withNodeRef(), withFailureMessage("baz")),
					machine("6", withInfrastructureReady(), withNodeRef(), withFailureReason("baz")),
				)
				g.Expect(cp.ProvisioningMachines().Names()).To(BeEmpty())
				g.Expect(cp.ReadyMachines().Names()).To(BeEmpty())
				g.Expect(cp.UnhealthyMachines().Names()).To(BeEmpty())
				g.Expect(cp.FailedMachines().Names()).To(ConsistOf("1", "2", "3", "4", "5", "6"))
			},
		},
		{
			name: "machines with an unhealthy annotation in clusters with at least 3 machines are unhealthy",
			test: func(g *WithT, cp *ControlPlane) {
				cp.Machines = NewFilterableMachineCollection(
					machine("1", withUnhealthyAnnotation()),
					machine("2", withNodeRef(), withInfrastructureReady()),
					machine("3"),
				)
				g.Expect(cp.ProvisioningMachines().Names()).To(ConsistOf("3"))
				g.Expect(cp.ReadyMachines().Names()).To(ConsistOf("2"))
				g.Expect(cp.UnhealthyMachines().Names()).To(ConsistOf("1"))
				g.Expect(cp.FailedMachines().Names()).To(BeEmpty())
			},
		},
		{
			name: "machines with an unhealthy annotation in clusters with less than 3 machines are not unhealthy",
			test: func(g *WithT, cp *ControlPlane) {
				cp.Machines = NewFilterableMachineCollection(
					machine("1", withUnhealthyAnnotation()),
					machine("2", withNodeRef(), withInfrastructureReady(), withUnhealthyAnnotation()),
				)
				g.Expect(cp.ProvisioningMachines().Names()).To(ConsistOf("1"))
				g.Expect(cp.ReadyMachines().Names()).To(ConsistOf("2"))
				g.Expect(cp.UnhealthyMachines().Names()).To(BeEmpty())
				g.Expect(cp.FailedMachines().Names()).To(BeEmpty())
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			kcp := &controlplanev1.KubeadmControlPlane{}
			cp := &ControlPlane{KCP: kcp, Logger: klogr.New()}
			tc.test(g, cp)
		})
	}
}

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

func withTimestamp(t time.Time) machineOpt {
	return func(m *clusterv1.Machine) {
		m.CreationTimestamp = metav1.NewTime(t)
	}
}

func withValidHash(kcp controlplanev1.KubeadmControlPlaneSpec) machineOpt {
	return func(m *clusterv1.Machine) {
		withHash(hash.Compute(&kcp))(m)
	}
}

func withUnhealthyAnnotation() machineOpt {
	return func(m *clusterv1.Machine) {
		m.Annotations = map[string]string{clusterv1.MachineUnhealthy: ""}
	}
}

func withNodeRef() machineOpt {
	return func(m *clusterv1.Machine) {
		m.Status.NodeRef = &corev1.ObjectReference{}
	}
}

func withInfrastructureReady() machineOpt {
	return func(m *clusterv1.Machine) {
		m.Status.InfrastructureReady = true
	}
}

func withFailureReason(reason string) machineOpt {
	return func(m *clusterv1.Machine) {
		failureReason := capierrors.MachineStatusError(reason)
		m.Status.FailureReason = &failureReason
	}
}
func withFailureMessage(msg string) machineOpt {
	return func(m *clusterv1.Machine) {
		m.Status.FailureMessage = pointer.StringPtr(msg)
	}
}
