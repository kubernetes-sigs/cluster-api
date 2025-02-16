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
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
)

func TestControlPlane(t *testing.T) {
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

		t.Run("With all machines in known failure domain, should return the FD with most number of machines", func(*testing.T) {
			g := NewWithT(t)
			g.Expect(*controlPlane.FailureDomainWithMostMachines(ctx, controlPlane.Machines)).To(Equal("two"))
		})

		t.Run("With some machines in non defined failure domains", func(*testing.T) {
			g := NewWithT(t)
			controlPlane.Machines.Insert(machine("machine-5", withFailureDomain("unknown")))
			g.Expect(*controlPlane.FailureDomainWithMostMachines(ctx, controlPlane.Machines)).To(Equal("unknown"))
		})
	})

	t.Run("MachinesUpToDate", func(t *testing.T) {
		g := NewWithT(t)
		cluster := &clusterv1.Cluster{
			Status: clusterv1.ClusterStatus{
				FailureDomains: clusterv1.FailureDomains{
					"one":   failureDomain(true),
					"two":   failureDomain(true),
					"three": failureDomain(true),
				},
			},
		}
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				Version: "v1.31.0",
			},
		}
		machines := collections.Machines{
			"machine-1": &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "m1"},
				Spec: clusterv1.MachineSpec{
					Version:           ptr.To("v1.31.0"), // up-to-date
					FailureDomain:     ptr.To("one"),
					InfrastructureRef: corev1.ObjectReference{Kind: "GenericInfrastructureMachine", APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1", Name: "m1"},
				}},
			"machine-2": &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "m2"},
				Spec: clusterv1.MachineSpec{
					Version:           ptr.To("v1.29.0"), // not up-to-date
					FailureDomain:     ptr.To("two"),
					InfrastructureRef: corev1.ObjectReference{Kind: "GenericInfrastructureMachine", APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1", Name: "m2"},
				}},
			"machine-3": &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "m3", DeletionTimestamp: ptr.To(metav1.Now())}, // deleted
				Spec: clusterv1.MachineSpec{
					Version:           ptr.To("v1.29.3"), // not up-to-date
					FailureDomain:     ptr.To("three"),
					InfrastructureRef: corev1.ObjectReference{Kind: "GenericInfrastructureMachine", APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1", Name: "m3"},
				}},
			"machine-4": &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "m4", DeletionTimestamp: ptr.To(metav1.Now())}, // deleted
				Spec: clusterv1.MachineSpec{
					Version:           ptr.To("v1.31.0"), // up-to-date
					FailureDomain:     ptr.To("two"),
					InfrastructureRef: corev1.ObjectReference{Kind: "GenericInfrastructureMachine", APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1", Name: "m4"},
				}},
			"machine-5": &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "m5"},
				Spec: clusterv1.MachineSpec{
					Version:           ptr.To("v1.31.0"), // up-to-date
					FailureDomain:     ptr.To("three"),
					InfrastructureRef: corev1.ObjectReference{Kind: "GenericInfrastructureMachine", APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1", Name: "m5"},
				}},
		}
		controlPlane, err := NewControlPlane(ctx, nil, env.GetClient(), cluster, kcp, machines)
		g.Expect(err).NotTo(HaveOccurred())

		g.Expect(controlPlane.Machines).To(HaveLen(5))

		machinesNotUptoDate, machinesNotUptoDateConditionMessages := controlPlane.NotUpToDateMachines()
		g.Expect(machinesNotUptoDate.Names()).To(ConsistOf("m2", "m3"))
		g.Expect(machinesNotUptoDateConditionMessages).To(HaveLen(2))
		g.Expect(machinesNotUptoDateConditionMessages).To(HaveKeyWithValue("m2", []string{"Version v1.29.0, v1.31.0 required"}))
		g.Expect(machinesNotUptoDateConditionMessages).To(HaveKeyWithValue("m3", []string{"Version v1.29.3, v1.31.0 required"}))

		machinesNeedingRollout, machinesNotUptoDateLogMessages := controlPlane.MachinesNeedingRollout()
		g.Expect(machinesNeedingRollout.Names()).To(ConsistOf("m2"))
		g.Expect(machinesNotUptoDateLogMessages).To(HaveLen(2))
		g.Expect(machinesNotUptoDateLogMessages).To(HaveKeyWithValue("m2", []string{"Machine version \"v1.29.0\" is not equal to KCP version \"v1.31.0\""}))
		g.Expect(machinesNotUptoDateLogMessages).To(HaveKeyWithValue("m3", []string{"Machine version \"v1.29.3\" is not equal to KCP version \"v1.31.0\""}))

		upToDateMachines := controlPlane.UpToDateMachines()
		g.Expect(upToDateMachines).To(HaveLen(3))
		g.Expect(upToDateMachines.Names()).To(ConsistOf("m1", "m4", "m5"))

		fd, err := controlPlane.NextFailureDomainForScaleUp(ctx)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(fd).To(Equal(ptr.To("two"))) // deleted up-to-date machines (m4) should not be counted when picking the next failure domain for scale up
	})
}

func TestHasMachinesToBeRemediated(t *testing.T) {
	// healthy machine (without MachineHealthCheckSucceded condition)
	healthyMachineNotProvisioned := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "healthyMachine1"}}
	// healthy machine (with MachineHealthCheckSucceded == true)
	healthyMachineProvisioned := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "healthyMachine2"}, Status: clusterv1.MachineStatus{NodeRef: &corev1.ObjectReference{Kind: "Node", Name: "node1"}}}
	conditions.MarkTrue(healthyMachineProvisioned, clusterv1.MachineHealthCheckSucceededCondition)
	// unhealthy machine NOT eligible for KCP remediation (with MachineHealthCheckSucceded == False, but without MachineOwnerRemediated condition)
	unhealthyMachineNOTOwnerRemediated := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "unhealthyMachineNOTOwnerRemediated"}, Status: clusterv1.MachineStatus{NodeRef: &corev1.ObjectReference{Kind: "Node", Name: "node2"}}}
	conditions.MarkFalse(unhealthyMachineNOTOwnerRemediated, clusterv1.MachineHealthCheckSucceededCondition, clusterv1.MachineHasFailureReason, clusterv1.ConditionSeverityWarning, "Something is wrong")
	// unhealthy machine eligible for KCP remediation (with MachineHealthCheckSucceded == False, with MachineOwnerRemediated condition)
	unhealthyMachineOwnerRemediated := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "unhealthyMachineOwnerRemediated"}, Status: clusterv1.MachineStatus{NodeRef: &corev1.ObjectReference{Kind: "Node", Name: "node3"}}}
	conditions.MarkFalse(unhealthyMachineOwnerRemediated, clusterv1.MachineHealthCheckSucceededCondition, clusterv1.MachineHasFailureReason, clusterv1.ConditionSeverityWarning, "Something is wrong")
	conditions.MarkFalse(unhealthyMachineOwnerRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "KCP should remediate this issue")

	t.Run("One unhealthy machine to be remediated by KCP", func(t *testing.T) {
		c := ControlPlane{
			Machines: collections.FromMachines(
				healthyMachineNotProvisioned,       // healthy machine, should be ignored
				healthyMachineProvisioned,          // healthy machine, should be ignored (the MachineHealthCheckSucceededCondition is true)
				unhealthyMachineNOTOwnerRemediated, // unhealthy machine, but KCP should not remediate it, should be ignored.
				unhealthyMachineOwnerRemediated,
			),
		}

		g := NewWithT(t)
		g.Expect(c.MachinesToBeRemediatedByKCP()).To(ConsistOf(unhealthyMachineOwnerRemediated))
		g.Expect(c.UnhealthyMachines()).To(ConsistOf(unhealthyMachineOwnerRemediated, unhealthyMachineNOTOwnerRemediated))
		g.Expect(c.HealthyMachines()).To(ConsistOf(healthyMachineNotProvisioned, healthyMachineProvisioned))
		g.Expect(c.HasHealthyMachineStillProvisioning()).To(BeTrue())
	})

	t.Run("No unhealthy machine to be remediated by KCP", func(t *testing.T) {
		c := ControlPlane{
			Machines: collections.FromMachines(
				healthyMachineNotProvisioned,       // healthy machine, should be ignored
				healthyMachineProvisioned,          // healthy machine, should be ignored (the MachineHealthCheckSucceededCondition is true)
				unhealthyMachineNOTOwnerRemediated, // unhealthy machine, but KCP should not remediate it, should be ignored.
			),
		}

		g := NewWithT(t)
		g.Expect(c.MachinesToBeRemediatedByKCP()).To(BeEmpty())
		g.Expect(c.UnhealthyMachines()).To(ConsistOf(unhealthyMachineNOTOwnerRemediated))
		g.Expect(c.HealthyMachines()).To(ConsistOf(healthyMachineNotProvisioned, healthyMachineProvisioned))
		g.Expect(c.HasHealthyMachineStillProvisioning()).To(BeTrue())
	})

	t.Run("No unhealthy machine to be remediated by KCP", func(t *testing.T) {
		c := ControlPlane{
			Machines: collections.FromMachines(
				healthyMachineProvisioned,          // healthy machine, should be ignored (the MachineHealthCheckSucceededCondition is true)
				unhealthyMachineNOTOwnerRemediated, // unhealthy machine, but KCP should not remediate it, should be ignored.
			),
		}

		g := NewWithT(t)
		g.Expect(c.MachinesToBeRemediatedByKCP()).To(BeEmpty())
		g.Expect(c.UnhealthyMachines()).To(ConsistOf(unhealthyMachineNOTOwnerRemediated))
		g.Expect(c.HealthyMachines()).To(ConsistOf(healthyMachineProvisioned))
		g.Expect(c.HasHealthyMachineStillProvisioning()).To(BeFalse())
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

func TestStatusToLogKeyAndValues(t *testing.T) {
	healthyMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{Name: "healthy"},
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{Name: "healthy-node"},
			Conditions: []clusterv1.Condition{
				{Type: controlplanev1.MachineAPIServerPodHealthyCondition, Status: corev1.ConditionTrue},
				{Type: controlplanev1.MachineControllerManagerPodHealthyCondition, Status: corev1.ConditionTrue},
				{Type: controlplanev1.MachineSchedulerPodHealthyCondition, Status: corev1.ConditionTrue},
				{Type: controlplanev1.MachineEtcdPodHealthyCondition, Status: corev1.ConditionTrue},
				{Type: controlplanev1.MachineEtcdMemberHealthyCondition, Status: corev1.ConditionTrue},
			},
		},
	}

	machineWithoutNode := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{Name: "without-node"},
		Status: clusterv1.MachineStatus{
			NodeRef: nil,
			Conditions: []clusterv1.Condition{
				{Type: controlplanev1.MachineAPIServerPodHealthyCondition, Status: corev1.ConditionUnknown},
				{Type: controlplanev1.MachineControllerManagerPodHealthyCondition, Status: corev1.ConditionUnknown},
				{Type: controlplanev1.MachineSchedulerPodHealthyCondition, Status: corev1.ConditionUnknown},
				{Type: controlplanev1.MachineEtcdPodHealthyCondition, Status: corev1.ConditionUnknown},
				{Type: controlplanev1.MachineEtcdMemberHealthyCondition, Status: corev1.ConditionFalse}, // not a real use case, but used to test a code branch.
			},
		},
	}

	machineJustCreated := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "just-created"}}

	machineJustDeleted := healthyMachine.DeepCopy()
	machineJustDeleted.Name = "just-deleted"

	machineNotUpToDate := healthyMachine.DeepCopy()
	machineNotUpToDate.Name = "not-up-to-date"

	machineMarkedForRemediation := healthyMachine.DeepCopy()
	machineMarkedForRemediation.Name = "marked-for-remediation"
	machineMarkedForRemediation.Status.Conditions = append(machineMarkedForRemediation.Status.Conditions,
		clusterv1.Condition{Type: clusterv1.MachineHealthCheckSucceededCondition, Status: corev1.ConditionFalse},
		clusterv1.Condition{Type: clusterv1.MachineOwnerRemediatedCondition, Status: corev1.ConditionFalse},
	)

	g := NewWithT(t)
	c := &ControlPlane{
		KCP:                 &controlplanev1.KubeadmControlPlane{},
		Machines:            collections.FromMachines(healthyMachine, machineWithoutNode, machineJustDeleted, machineNotUpToDate, machineMarkedForRemediation),
		machinesNotUptoDate: collections.FromMachines(machineNotUpToDate),
		EtcdMembers:         []*etcd.Member{{Name: "m1"}, {Name: "m2"}, {Name: "m3"}},
	}

	got := c.StatusToLogKeyAndValues(machineJustCreated, machineJustDeleted)

	g.Expect(got).To(HaveLen(4))
	g.Expect(got[0]).To(Equal("machines"))
	machines := strings.Join([]string{
		"healthy",
		"just-created (just created)",
		"just-deleted (just deleted)",
		"marked-for-remediation (marked for remediation)",
		"not-up-to-date (not up-to-date)",
		"without-node (status.nodeRef not set, APIServerPod health unknown, ControllerManagerPod health unknown, SchedulerPod health unknown, EtcdPod health unknown, EtcdMember not healthy)",
	}, ", ")
	g.Expect(got[1]).To(Equal(machines), cmp.Diff(got[1], machines))
	g.Expect(got[2]).To(Equal("etcdMembers"))
	g.Expect(got[3]).To(Equal("m1, m2, m3"))
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
