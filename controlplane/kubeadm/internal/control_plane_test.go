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

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
)

func TestControlPlane(t *testing.T) {
	g := NewWithT(t)

	t.Run("Failure domains", func(t *testing.T) {
		controlPlane := &ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{},
			Cluster: &clusterv1.Cluster{
				Status: clusterv1.ClusterStatus{
					FailureDomains: clusterv1.FailureDomains{
						"one":   failureDomain(true),
						"two":   failureDomain(true),
						"three": failureDomain(true),
						"four":  failureDomain(false),
					},
				},
			},
			Machines: collections.Machines{
				"machine-1": machine("machine-1", withFailureDomain("one")),
				"machine-2": machine("machine-2", withFailureDomain("two")),
				"machine-3": machine("machine-3", withFailureDomain("two")),
			},
		}

		t.Run("With all machines in known failure domain, should return the FD with most number of machines", func(t *testing.T) {
			g.Expect(*controlPlane.FailureDomainWithMostMachines(ctx, controlPlane.Machines)).To(Equal("two"))
		})

		t.Run("With some machines in non defined failure domains", func(t *testing.T) {
			controlPlane.Machines.Insert(machine("machine-5", withFailureDomain("unknown")))
			g.Expect(*controlPlane.FailureDomainWithMostMachines(ctx, controlPlane.Machines)).To(Equal("unknown"))
		})
	})
}

func TestHasUnhealthyMachine(t *testing.T) {
	// healthy machine (without MachineHealthCheckSucceded condition)
	healthyMachine1 := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "healthyMachine1"}}
	// healthy machine (with MachineHealthCheckSucceded == true)
	healthyMachine2 := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "healthyMachine2"}}
	conditions.MarkTrue(healthyMachine2, clusterv1.MachineHealthCheckSucceededCondition)
	// unhealthy machine NOT eligible for KCP remediation (with MachineHealthCheckSucceded == False, but without MachineOwnerRemediated condition)
	unhealthyMachineNOTOwnerRemediated := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "unhealthyMachineNOTOwnerRemediated"}}
	conditions.MarkFalse(unhealthyMachineNOTOwnerRemediated, clusterv1.MachineHealthCheckSucceededCondition, clusterv1.MachineHasFailureReason, clusterv1.ConditionSeverityWarning, "Something is wrong")
	// unhealthy machine eligible for KCP remediation (with MachineHealthCheckSucceded == False, with MachineOwnerRemediated condition)
	unhealthyMachineOwnerRemediated := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "unhealthyMachineOwnerRemediated"}}
	conditions.MarkFalse(unhealthyMachineOwnerRemediated, clusterv1.MachineHealthCheckSucceededCondition, clusterv1.MachineHasFailureReason, clusterv1.ConditionSeverityWarning, "Something is wrong")
	conditions.MarkFalse(unhealthyMachineOwnerRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "KCP should remediate this issue")

	t.Run("One unhealthy machine to be remediated by KCP", func(t *testing.T) {
		c := ControlPlane{
			Machines: collections.FromMachines(
				healthyMachine1,                    // healthy machine, should be ignored
				healthyMachine2,                    // healthy machine, should be ignored (the MachineHealthCheckSucceededCondition is true)
				unhealthyMachineNOTOwnerRemediated, // unhealthy machine, but KCP should not remediate it, should be ignored.
				unhealthyMachineOwnerRemediated,
			),
		}

		g := NewWithT(t)
		g.Expect(c.HasUnhealthyMachine()).To(BeTrue())
	})

	t.Run("No unhealthy machine to be remediated by KCP", func(t *testing.T) {
		c := ControlPlane{
			Machines: collections.FromMachines(
				healthyMachine1,                    // healthy machine, should be ignored
				healthyMachine2,                    // healthy machine, should be ignored (the MachineHealthCheckSucceededCondition is true)
				unhealthyMachineNOTOwnerRemediated, // unhealthy machine, but KCP should not remediate it, should be ignored.
			),
		}

		g := NewWithT(t)
		g.Expect(c.HasUnhealthyMachine()).To(BeFalse())
	})
}

func TestHasHealthyMachineStillProvisioning(t *testing.T) {
	// healthy machine (without MachineHealthCheckSucceded condition) still provisioning (without NodeRef)
	healthyMachineStillProvisioning1 := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "healthyMachineStillProvisioning1"}}

	// healthy machine (without MachineHealthCheckSucceded condition) provisioned (with NodeRef)
	healthyMachineProvisioned1 := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "healthyMachineProvisioned1"}}
	healthyMachineProvisioned1.Status.NodeRef = &corev1.ObjectReference{}

	// unhealthy machine (with MachineHealthCheckSucceded condition) still provisioning (without NodeRef)
	unhealthyMachineStillProvisioning1 := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "unhealthyMachineStillProvisioning1"}}
	conditions.MarkFalse(unhealthyMachineStillProvisioning1, clusterv1.MachineHealthCheckSucceededCondition, clusterv1.MachineHasFailureReason, clusterv1.ConditionSeverityWarning, "Something is wrong")
	conditions.MarkFalse(unhealthyMachineStillProvisioning1, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "KCP should remediate this issue")

	// unhealthy machine (with MachineHealthCheckSucceded condition) provisioned (with NodeRef)
	unhealthyMachineProvisioned1 := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "unhealthyMachineProvisioned1"}}
	unhealthyMachineProvisioned1.Status.NodeRef = &corev1.ObjectReference{}
	conditions.MarkFalse(unhealthyMachineProvisioned1, clusterv1.MachineHealthCheckSucceededCondition, clusterv1.MachineHasFailureReason, clusterv1.ConditionSeverityWarning, "Something is wrong")
	conditions.MarkFalse(unhealthyMachineProvisioned1, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "KCP should remediate this issue")

	t.Run("Healthy machine still provisioning", func(t *testing.T) {
		c := ControlPlane{
			Machines: collections.FromMachines(
				healthyMachineStillProvisioning1,
				unhealthyMachineStillProvisioning1, // unhealthy, should be ignored
				healthyMachineProvisioned1,         // already provisioned, should be ignored
				unhealthyMachineProvisioned1,       // unhealthy and already provisioned, should be ignored
			),
		}

		g := NewWithT(t)
		g.Expect(c.HasHealthyMachineStillProvisioning()).To(BeTrue())
	})
	t.Run("No machines still provisioning", func(t *testing.T) {
		c := ControlPlane{
			Machines: collections.FromMachines(
				unhealthyMachineStillProvisioning1, // unhealthy, should be ignored
				healthyMachineProvisioned1,         // already provisioned, should be ignored
				unhealthyMachineProvisioned1,       // unhealthy and already provisioned, should be ignored
			),
		}

		g := NewWithT(t)
		g.Expect(c.HasHealthyMachineStillProvisioning()).To(BeFalse())
	})
}

type machineOpt func(*clusterv1.Machine)

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

func machine(name string, opts ...machineOpt) *clusterv1.Machine {
	m := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}
