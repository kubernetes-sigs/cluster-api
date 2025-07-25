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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/patch"
)

func TestControlPlane(t *testing.T) {
	t.Run("Failure domains", func(t *testing.T) {
		controlPlane := &ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{},
			Cluster: &clusterv1.Cluster{
				Status: clusterv1.ClusterStatus{
					FailureDomains: []clusterv1.FailureDomain{
						failureDomain("one", true),
						failureDomain("two", true),
						failureDomain("three", true),
						failureDomain("four", false),
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
			g.Expect(controlPlane.FailureDomainWithMostMachines(ctx, controlPlane.Machines)).To(Equal("two"))
		})

		t.Run("With some machines in non defined failure domains", func(*testing.T) {
			g := NewWithT(t)
			controlPlane.Machines.Insert(machine("machine-5", withFailureDomain("unknown")))
			g.Expect(controlPlane.FailureDomainWithMostMachines(ctx, controlPlane.Machines)).To(Equal("unknown"))
		})

		t.Run("With failure Domains is set empty", func(*testing.T) {
			g := NewWithT(t)
			controlPlane.Cluster.Status.FailureDomains = nil
			g.Expect(controlPlane.FailureDomainWithMostMachines(ctx, controlPlane.Machines)).To(Equal("one"))
		})
	})

	t.Run("MachinesUpToDate", func(t *testing.T) {
		g := NewWithT(t)
		cluster := &clusterv1.Cluster{
			Status: clusterv1.ClusterStatus{
				FailureDomains: []clusterv1.FailureDomain{
					failureDomain("one", true),
					failureDomain("two", true),
					failureDomain("three", true),
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
					Version:           "v1.31.0", // up-to-date
					FailureDomain:     "one",
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{Kind: "GenericInfrastructureMachine", APIGroup: clusterv1.GroupVersionInfrastructure.Group, Name: "m1"},
				}},
			"machine-2": &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "m2"},
				Spec: clusterv1.MachineSpec{
					Version:           "v1.29.0", // not up-to-date
					FailureDomain:     "two",
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{Kind: "GenericInfrastructureMachine", APIGroup: clusterv1.GroupVersionInfrastructure.Group, Name: "m2"},
				}},
			"machine-3": &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "m3", DeletionTimestamp: ptr.To(metav1.Now())}, // deleted
				Spec: clusterv1.MachineSpec{
					Version:           "v1.29.3", // not up-to-date
					FailureDomain:     "three",
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{Kind: "GenericInfrastructureMachine", APIGroup: clusterv1.GroupVersionInfrastructure.Group, Name: "m3"},
				}},
			"machine-4": &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "m4", DeletionTimestamp: ptr.To(metav1.Now())}, // deleted
				Spec: clusterv1.MachineSpec{
					Version:           "v1.31.0", // up-to-date
					FailureDomain:     "two",
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{Kind: "GenericInfrastructureMachine", APIGroup: clusterv1.GroupVersionInfrastructure.Group, Name: "m4"},
				}},
			"machine-5": &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "m5"},
				Spec: clusterv1.MachineSpec{
					Version:           "v1.31.0", // up-to-date
					FailureDomain:     "three",
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{Kind: "GenericInfrastructureMachine", APIGroup: clusterv1.GroupVersionInfrastructure.Group, Name: "m5"},
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
		g.Expect(fd).To(Equal("two")) // deleted up-to-date machines (m4) should not be counted when picking the next failure domain for scale up
	})

	t.Run("Next Failure Domains", func(t *testing.T) {
		g := NewWithT(t)
		cluster := clusterv1.Cluster{
			Status: clusterv1.ClusterStatus{
				FailureDomains: []clusterv1.FailureDomain{
					failureDomain("one", false),
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
				ObjectMeta: metav1.ObjectMeta{Name: "m1", DeletionTimestamp: ptr.To(metav1.Now())},
				Spec: clusterv1.MachineSpec{
					Version:           "v1.31.0",
					FailureDomain:     "one",
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{Kind: "GenericInfrastructureMachine", APIGroup: clusterv1.GroupVersionInfrastructure.Group, Name: "m1"},
				}},
		}
		controlPlane, err := NewControlPlane(ctx, nil, env.GetClient(), &cluster, kcp, machines)
		g.Expect(err).NotTo(HaveOccurred())
		fd, err := controlPlane.NextFailureDomainForScaleUp(ctx)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(fd).To(BeEmpty())
	})

	t.Run("ControlPlane returns error when getting infra resources", func(t *testing.T) {
		g := NewWithT(t)
		cluster := clusterv1.Cluster{
			Status: clusterv1.ClusterStatus{
				FailureDomains: []clusterv1.FailureDomain{
					failureDomain("one", true),
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
					Version:           "v1.31.0",
					FailureDomain:     "one",
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{Name: "m1"},
				}},
		}
		_, err := NewControlPlane(ctx, nil, env.GetClient(), &cluster, kcp, machines)
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("When infra and bootstrap config exists", func(t *testing.T) {
		g := NewWithT(t)
		ns, err := env.CreateNamespace(ctx, "test-machine-watches")
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				Version: "v1.31.0",
			},
		}

		g.Expect(err).ToNot(HaveOccurred())

		infraMachine := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       "GenericInfrastructureMachine",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"metadata": map[string]interface{}{
					"name":      "infra-config1",
					"namespace": ns.Name,
				},
				"spec": map[string]interface{}{
					"providerID": "test://id-1",
				},
				"status": map[string]interface{}{
					"ready": true,
					"addresses": []interface{}{
						map[string]interface{}{
							"type":    "InternalIP",
							"address": "10.0.0.1",
						},
					},
				},
			},
		}

		bootstrap := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       "KubeadmConfig",
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "bootstrap-config-machinereconcile",
					"namespace": ns.Name,
				},
				"spec": map[string]interface{}{
					"providerID": "test://id-1",
				},
				"status": map[string]interface{}{
					"ready": true,
				},
			},
		}

		testCluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: ns.Name},
			Status: clusterv1.ClusterStatus{
				FailureDomains: []clusterv1.FailureDomain{
					failureDomain("one", true),
					failureDomain("two", true),
					failureDomain("three", true),
				},
			},
		}

		g.Expect(env.Create(ctx, infraMachine)).To(Succeed())
		g.Expect(env.Create(ctx, bootstrap)).To(Succeed())

		defer func(do ...client.Object) {
			g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
		}(ns, bootstrap, infraMachine)

		// Patch infra machine ready
		patchHelper, err := patch.NewHelper(infraMachine, env)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(unstructured.SetNestedField(infraMachine.Object, true, "status", "ready")).To(Succeed())
		g.Expect(patchHelper.Patch(ctx, infraMachine, patch.WithStatusObservedGeneration{})).To(Succeed())

		// Patch bootstrap ready
		patchHelper, err = patch.NewHelper(bootstrap, env)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(unstructured.SetNestedField(bootstrap.Object, true, "status", "ready")).To(Succeed())
		g.Expect(patchHelper.Patch(ctx, bootstrap, patch.WithStatusObservedGeneration{})).To(Succeed())

		machines := collections.Machines{
			"machine-1": &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "m1",
					Namespace: ns.Name},
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: clusterv1.GroupVersionInfrastructure.Group,
						Kind:     "GenericInfrastructureMachine",
						Name:     "infra-config1",
					},
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &clusterv1.ContractVersionedObjectReference{
							APIGroup: clusterv1.GroupVersionBootstrap.Group,
							Kind:     "KubeadmConfig",
							Name:     "bootstrap-config-machinereconcile",
						},
					},
				},
			},
		}

		_, err = NewControlPlane(ctx, nil, env.GetClient(), testCluster, kcp, machines)
		g.Expect(err).NotTo(HaveOccurred())
	})
}

func TestHasMachinesToBeRemediated(t *testing.T) {
	// healthy machine (without MachineHealthCheckSucceded condition)
	healthyMachineNotProvisioned := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "healthyMachine1"}}
	// healthy machine (with MachineHealthCheckSucceded == true)
	healthyMachineProvisioned := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "healthyMachine2"}, Status: clusterv1.MachineStatus{NodeRef: &clusterv1.MachineNodeReference{Name: "node1"}}}
	healthyMachineProvisioned.SetConditions([]metav1.Condition{
		{
			Type:   clusterv1.MachineHealthCheckSucceededCondition,
			Status: metav1.ConditionTrue,
		},
	})
	// unhealthy machine NOT eligible for KCP remediation (with MachineHealthCheckSucceded == False, but without MachineOwnerRemediated condition)
	unhealthyMachineNOTOwnerRemediated := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "unhealthyMachineNOTOwnerRemediated"}, Status: clusterv1.MachineStatus{NodeRef: &clusterv1.MachineNodeReference{Name: "node2"}}}
	unhealthyMachineNOTOwnerRemediated.SetConditions([]metav1.Condition{
		{
			Type:   clusterv1.MachineHealthCheckSucceededCondition,
			Status: metav1.ConditionFalse,
		},
	})
	// unhealthy machine eligible for KCP remediation (with MachineHealthCheckSucceded == False, with MachineOwnerRemediated condition)
	unhealthyMachineOwnerRemediated := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "unhealthyMachineOwnerRemediated"}, Status: clusterv1.MachineStatus{NodeRef: &clusterv1.MachineNodeReference{Name: "node3"}}}
	unhealthyMachineOwnerRemediated.SetConditions([]metav1.Condition{
		{
			Type:   clusterv1.MachineHealthCheckSucceededCondition,
			Status: metav1.ConditionFalse,
		},
		{
			Type:   clusterv1.MachineOwnerRemediatedCondition,
			Status: metav1.ConditionFalse,
		},
	})

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
	healthyMachineProvisioned1.Status.NodeRef = &clusterv1.MachineNodeReference{}

	// unhealthy machine (with MachineHealthCheckSucceded condition) still provisioning (without NodeRef)
	unhealthyMachineStillProvisioning1 := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "unhealthyMachineStillProvisioning1"}}
	unhealthyMachineStillProvisioning1.SetConditions([]metav1.Condition{
		{
			Type:   clusterv1.MachineHealthCheckSucceededCondition,
			Status: metav1.ConditionFalse,
		},
		{
			Type:   clusterv1.MachineOwnerRemediatedCondition,
			Status: metav1.ConditionFalse,
		},
	})

	// unhealthy machine (with MachineHealthCheckSucceded condition) provisioned (with NodeRef)
	unhealthyMachineProvisioned1 := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "unhealthyMachineProvisioned1"}}
	unhealthyMachineProvisioned1.Status.NodeRef = &clusterv1.MachineNodeReference{}
	unhealthyMachineProvisioned1.SetConditions([]metav1.Condition{
		{
			Type:   clusterv1.MachineHealthCheckSucceededCondition,
			Status: metav1.ConditionFalse,
		},
		{
			Type:   clusterv1.MachineOwnerRemediatedCondition,
			Status: metav1.ConditionFalse,
		},
	})

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
			NodeRef: &clusterv1.MachineNodeReference{Name: "healthy-node"},
			Conditions: []metav1.Condition{
				{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionTrue},
				{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionTrue},
				{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionTrue},
				{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionTrue},
				{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue},
			},
		},
	}

	machineWithoutNode := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{Name: "without-node"},
		Status: clusterv1.MachineStatus{
			NodeRef: nil,
			Conditions: []metav1.Condition{
				{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionUnknown},
				{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionUnknown},
				{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionUnknown},
				{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionUnknown},
				{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionFalse}, // not a real use case, but used to test a code branch.
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
		metav1.Condition{Type: clusterv1.MachineHealthCheckSucceededCondition, Status: metav1.ConditionFalse},
		metav1.Condition{Type: clusterv1.MachineOwnerRemediatedCondition, Status: metav1.ConditionFalse},
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

func TestMachineInFailureDomainWithMostMachines(t *testing.T) {
	t.Run("Machines in Failure Domain", func(t *testing.T) {
		machines := collections.Machines{
			"machine-3": &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "m3"},
				Spec: clusterv1.MachineSpec{
					Version:           "v1.31.0",
					FailureDomain:     "three",
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{Kind: "GenericInfrastructureMachine", APIGroup: clusterv1.GroupVersionInfrastructure.Group, Name: "m3"},
				}},
		}

		c := &ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{},
			Cluster: &clusterv1.Cluster{
				Status: clusterv1.ClusterStatus{
					FailureDomains: []clusterv1.FailureDomain{
						failureDomain("three", false),
					},
				},
			},
			Machines: collections.Machines{
				"machine-3": machine("machine-3", withFailureDomain("three")),
			},
		}

		g := NewWithT(t)
		_, err := c.MachineInFailureDomainWithMostMachines(ctx, machines)
		g.Expect(err).NotTo(HaveOccurred())
	})
	t.Run("Return error when no controlplane machine found", func(t *testing.T) {
		machines := collections.Machines{}

		c := &ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{},
			Cluster: &clusterv1.Cluster{
				Status: clusterv1.ClusterStatus{
					FailureDomains: []clusterv1.FailureDomain{},
				},
			},
			Machines: collections.Machines{},
		}

		g := NewWithT(t)
		_, err := c.MachineInFailureDomainWithMostMachines(ctx, machines)
		g.Expect(err).To(HaveOccurred())
	})
}
func TestMachineWithDeleteAnnotation(t *testing.T) {
	t.Run("Machines having delete annotation set", func(t *testing.T) {
		machines := collections.Machines{
			"machine-1": &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "m1",
					Annotations: map[string]string{
						"cluster.x-k8s.io/delete-machine": "",
					},
				},
				Spec: clusterv1.MachineSpec{
					Version:           "v1.31.0",
					FailureDomain:     "one",
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{Kind: "GenericInfrastructureMachine", APIGroup: clusterv1.GroupVersionInfrastructure.Group, Name: "m1"},
				}},
			"machine-2": &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "m2",
					Annotations: map[string]string{
						"cluster.x-k8s.io/delete-machine": "",
					},
				},
				Spec: clusterv1.MachineSpec{
					Version:           "v1.31.0",
					FailureDomain:     "two",
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{Kind: "GenericInfrastructureMachine", APIGroup: clusterv1.GroupVersionInfrastructure.Group, Name: "m2"},
				}},
		}

		c := ControlPlane{
			Machines: machines,
			Cluster: &clusterv1.Cluster{
				Status: clusterv1.ClusterStatus{},
			},
		}

		g := NewWithT(t)
		annotatedMachines := c.MachineWithDeleteAnnotation(machines)
		g.Expect(annotatedMachines).NotTo(BeNil())
		g.Expect(annotatedMachines.Len()).To(BeEquivalentTo(2))
	})
}

type machineOpt func(*clusterv1.Machine)

func failureDomain(name string, controlPlane bool) clusterv1.FailureDomain {
	return clusterv1.FailureDomain{
		Name:         name,
		ControlPlane: ptr.To(controlPlane),
	}
}

func withFailureDomain(fd string) machineOpt {
	return func(m *clusterv1.Machine) {
		m.Spec.FailureDomain = fd
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
