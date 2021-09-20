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

	"sigs.k8s.io/cluster-api/util/collections"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
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
			g.Expect(*controlPlane.FailureDomainWithMostMachines(controlPlane.Machines)).To(Equal("two"))
		})

		t.Run(("With some machines in non defined failure domains"), func(t *testing.T) {
			controlPlane.Machines.Insert(machine("machine-5", withFailureDomain("unknown")))
			g.Expect(*controlPlane.FailureDomainWithMostMachines(controlPlane.Machines)).To(Equal("unknown"))
		})
	})

	t.Run("Generating components", func(t *testing.T) {
		controlPlane := &ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cp",
					UID:  types.UID("test-uid"),
				},
			},
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
				},
			},
		}

		t.Run("Should generate KubeadmConfig without a controller reference", func(t *testing.T) {
			spec := &bootstrapv1.KubeadmConfigSpec{}
			kubeadmConfig := controlPlane.GenerateKubeadmConfig(spec)
			g.Expect(kubeadmConfig.Labels["cluster.x-k8s.io/cluster-name"]).To(Equal("test-cluster"))
			g.Expect(kubeadmConfig.OwnerReferences[0].Controller).To(BeNil())
		})

		t.Run("Should generate a new machine with a controller reference", func(t *testing.T) {
			machine := controlPlane.NewMachine(&corev1.ObjectReference{Namespace: "foobar"}, &corev1.ObjectReference{Namespace: "foobar"}, pointer.StringPtr("failureDomain"))
			g.Expect(machine.OwnerReferences[0].Controller).ToNot(BeNil())
		})
	})
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
		Machines: collections.FromMachines(
			healthyMachine1,
			healthyMachine2,
			unhealthyMachineNOTOwnerRemediated,
			unhealthyMachineOwnerRemediated,
		),
	}

	g := NewWithT(t)
	g.Expect(c.HasUnhealthyMachine()).To(BeTrue())
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
