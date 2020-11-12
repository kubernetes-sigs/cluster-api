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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util/conditions"
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
				Expect(kubeadmConfig.OwnerReferences[0].Controller).To(BeNil())
			})
			It("should generate new machine with controller reference", func() {
				machine := controlPlane.NewMachine(&corev1.ObjectReference{Namespace: "foobar"}, &corev1.ObjectReference{Namespace: "foobar"}, pointer.StringPtr("failureDomain"))
				Expect(machine.OwnerReferences[0].Controller).ToNot(BeNil())
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

func TestHasUnhealthyMachine(t *testing.T) {
	// healthy machine (without MachineHealthCheckSucceded condition)
	healthyMachine1 := &clusterv1.Machine{}
	// healthy machine (with MachineHealthCheckSucceded == true)
	healthyMachine2 := &clusterv1.Machine{}
	conditions.MarkTrue(healthyMachine2, clusterv1.MachineHealthCheckSuccededCondition)
	// unhealthy machine NOT eligible for KCP remediation (with MachineHealthCheckSucceded == False, but without MachineOwnerRemediated condition)
	unhealthyMachineNOTOwnerRemediated := &clusterv1.Machine{}
	conditions.MarkFalse(unhealthyMachineNOTOwnerRemediated, clusterv1.MachineHealthCheckSuccededCondition, clusterv1.MachineHasFailureReason, clusterv1.ConditionSeverityWarning, "")
	// unhealthy machine eligible for KCP remediation (with MachineHealthCheckSucceded == False, with MachineOwnerRemediated condition)
	unhealthyMachineOwnerRemediated := &clusterv1.Machine{}
	conditions.MarkFalse(unhealthyMachineOwnerRemediated, clusterv1.MachineHealthCheckSuccededCondition, clusterv1.MachineHasFailureReason, clusterv1.ConditionSeverityWarning, "")
	conditions.MarkFalse(unhealthyMachineOwnerRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "")

	c := ControlPlane{
		Machines: NewFilterableMachineCollection(
			healthyMachine1,
			healthyMachine2,
			unhealthyMachineNOTOwnerRemediated,
			unhealthyMachineOwnerRemediated,
		),
	}

	g := NewWithT(t)
	g.Expect(c.HasUnhealthyMachine()).To(BeTrue())
}
