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

package collections_test

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func falseFilter(_ *clusterv1.Machine) bool {
	return false
}

func trueFilter(_ *clusterv1.Machine) bool {
	return true
}

func TestNot(t *testing.T) {
	t.Run("returns false given a machine filter that returns true", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(collections.Not(trueFilter)(m)).To(BeFalse())
	})
	t.Run("returns true given a machine filter that returns false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(collections.Not(falseFilter)(m)).To(BeTrue())
	})
}

func TestAnd(t *testing.T) {
	t.Run("returns true if both given machine filters return true", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(collections.And(trueFilter, trueFilter)(m)).To(BeTrue())
	})
	t.Run("returns false if either given machine filter returns false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(collections.And(trueFilter, falseFilter)(m)).To(BeFalse())
	})
}

func TestOr(t *testing.T) {
	t.Run("returns true if either given machine filters return true", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(collections.Or(trueFilter, falseFilter)(m)).To(BeTrue())
	})
	t.Run("returns false if both given machine filter returns false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(collections.Or(falseFilter, falseFilter)(m)).To(BeFalse())
	})
}

func TestHasUnhealthyCondition(t *testing.T) {
	t.Run("healthy machine (without HealthCheckSucceeded condition) should return false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(collections.HasUnhealthyCondition(m)).To(BeFalse())
	})
	t.Run("healthy machine (with HealthCheckSucceeded condition == True) should return false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		conditions.MarkTrue(m, clusterv1.MachineHealthCheckSuccededCondition)
		g.Expect(collections.HasUnhealthyCondition(m)).To(BeFalse())
	})
	t.Run("unhealthy machine NOT eligible for KCP remediation (with withHealthCheckSucceeded condition == False but without OwnerRemediated) should return false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		conditions.MarkFalse(m, clusterv1.MachineHealthCheckSuccededCondition, clusterv1.MachineHasFailureReason, clusterv1.ConditionSeverityWarning, "")
		g.Expect(collections.HasUnhealthyCondition(m)).To(BeFalse())
	})
	t.Run("unhealthy machine eligible for KCP (with HealthCheckSucceeded condition == False and with OwnerRemediated) should return true", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		conditions.MarkFalse(m, clusterv1.MachineHealthCheckSuccededCondition, clusterv1.MachineHasFailureReason, clusterv1.ConditionSeverityWarning, "")
		conditions.MarkFalse(m, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "")
		g.Expect(collections.HasUnhealthyCondition(m)).To(BeTrue())
	})
}

func TestHasDeletionTimestamp(t *testing.T) {
	t.Run("machine with deletion timestamp returns true", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		now := metav1.Now()
		m.SetDeletionTimestamp(&now)
		g.Expect(collections.HasDeletionTimestamp(m)).To(BeTrue())
	})
	t.Run("machine with nil deletion timestamp returns false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(collections.HasDeletionTimestamp(m)).To(BeFalse())
	})
	t.Run("machine with zero deletion timestamp returns false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		zero := metav1.NewTime(time.Time{})
		m.SetDeletionTimestamp(&zero)
		g.Expect(collections.HasDeletionTimestamp(m)).To(BeFalse())
	})
}

func TestShouldRolloutAfter(t *testing.T) {
	reconciliationTime := metav1.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	t.Run("if the machine is nil it returns false", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(collections.ShouldRolloutAfter(&reconciliationTime, &reconciliationTime)(nil)).To(BeFalse())
	})
	t.Run("if the reconciliationTime is nil it returns false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(collections.ShouldRolloutAfter(nil, &reconciliationTime)(m)).To(BeFalse())
	})
	t.Run("if the rolloutAfter is nil it returns false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(collections.ShouldRolloutAfter(&reconciliationTime, nil)(m)).To(BeFalse())
	})
	t.Run("if rolloutAfter is after the reconciliation time, return false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		rolloutAfter := metav1.NewTime(reconciliationTime.Add(+1 * time.Hour))
		g.Expect(collections.ShouldRolloutAfter(&reconciliationTime, &rolloutAfter)(m)).To(BeFalse())
	})
	t.Run("if rolloutAfter is before the reconciliation time and the machine was created before rolloutAfter, return true", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		m.SetCreationTimestamp(metav1.NewTime(reconciliationTime.Add(-2 * time.Hour)))
		rolloutAfter := metav1.NewTime(reconciliationTime.Add(-1 * time.Hour))
		g.Expect(collections.ShouldRolloutAfter(&reconciliationTime, &rolloutAfter)(m)).To(BeTrue())
	})
	t.Run("if rolloutAfter is before the reconciliation time and the machine was created after rolloutAfter, return false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		m.SetCreationTimestamp(metav1.NewTime(reconciliationTime.Add(+1 * time.Hour)))
		rolloutAfter := metav1.NewTime(reconciliationTime.Add(-1 * time.Hour))
		g.Expect(collections.ShouldRolloutAfter(&reconciliationTime, &rolloutAfter)(m)).To(BeFalse())
	})
}

func TestHashAnnotationKey(t *testing.T) {
	t.Run("machine with specified annotation returns true", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		m.SetAnnotations(map[string]string{"test": ""})
		g.Expect(collections.HasAnnotationKey("test")(m)).To(BeTrue())
	})
	t.Run("machine with specified annotation with non-empty value returns true", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		m.SetAnnotations(map[string]string{"test": "blue"})
		g.Expect(collections.HasAnnotationKey("test")(m)).To(BeTrue())
	})
	t.Run("machine without specified annotation returns false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(collections.HasAnnotationKey("foo")(m)).To(BeFalse())
	})
}

func TestInFailureDomain(t *testing.T) {
	t.Run("nil machine returns false", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(collections.InFailureDomains(pointer.StringPtr("test"))(nil)).To(BeFalse())
	})
	t.Run("machine with given failure domain returns true", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: pointer.StringPtr("test")}}
		g.Expect(collections.InFailureDomains(pointer.StringPtr("test"))(m)).To(BeTrue())
	})
	t.Run("machine with a different failure domain returns false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: pointer.StringPtr("notTest")}}
		g.Expect(collections.InFailureDomains(
			pointer.StringPtr("test"),
			pointer.StringPtr("test2"),
			pointer.StringPtr("test3"),
			nil,
			pointer.StringPtr("foo"))(m)).To(BeFalse())
	})
	t.Run("machine without failure domain returns false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(collections.InFailureDomains(pointer.StringPtr("test"))(m)).To(BeFalse())
	})
	t.Run("machine without failure domain returns true, when nil used for failure domain", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(collections.InFailureDomains(nil)(m)).To(BeTrue())
	})
	t.Run("machine with failure domain returns true, when one of multiple failure domains match", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: pointer.StringPtr("test")}}
		g.Expect(collections.InFailureDomains(pointer.StringPtr("foo"), pointer.StringPtr("test"))(m)).To(BeTrue())
	})
}

func TestActiveMachinesInCluster(t *testing.T) {
	t.Run("machine with deletion timestamp returns false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		now := metav1.Now()
		m.SetDeletionTimestamp(&now)
		g.Expect(collections.ActiveMachines(m)).To(BeFalse())
	})
	t.Run("machine with nil deletion timestamp returns true", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(collections.ActiveMachines(m)).To(BeTrue())
	})
	t.Run("machine with zero deletion timestamp returns true", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		zero := metav1.NewTime(time.Time{})
		m.SetDeletionTimestamp(&zero)
		g.Expect(collections.ActiveMachines(m)).To(BeTrue())
	})
}

func TestMatchesKubernetesVersion(t *testing.T) {
	t.Run("nil machine returns false", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(collections.MatchesKubernetesVersion("some_ver")(nil)).To(BeFalse())
	})

	t.Run("nil machine.Spec.Version returns false", func(t *testing.T) {
		g := NewWithT(t)
		machine := &clusterv1.Machine{
			Spec: clusterv1.MachineSpec{
				Version: nil,
			},
		}
		g.Expect(collections.MatchesKubernetesVersion("some_ver")(machine)).To(BeFalse())
	})

	t.Run("machine.Spec.Version returns true if matches", func(t *testing.T) {
		g := NewWithT(t)
		kversion := "some_ver"
		machine := &clusterv1.Machine{
			Spec: clusterv1.MachineSpec{
				Version: &kversion,
			},
		}
		g.Expect(collections.MatchesKubernetesVersion("some_ver")(machine)).To(BeTrue())
	})

	t.Run("machine.Spec.Version returns false if does not match", func(t *testing.T) {
		g := NewWithT(t)
		kversion := "some_ver_2"
		machine := &clusterv1.Machine{
			Spec: clusterv1.MachineSpec{
				Version: &kversion,
			},
		}
		g.Expect(collections.MatchesKubernetesVersion("some_ver")(machine)).To(BeFalse())
	})
}

func TestWithVersion(t *testing.T) {
	t.Run("nil machine returns false", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(collections.WithVersion()(nil)).To(BeFalse())
	})

	t.Run("nil machine.Spec.Version returns false", func(t *testing.T) {
		g := NewWithT(t)
		machine := &clusterv1.Machine{
			Spec: clusterv1.MachineSpec{
				Version: nil,
			},
		}
		g.Expect(collections.WithVersion()(machine)).To(BeFalse())
	})

	t.Run("empty machine.Spec.Version returns false", func(t *testing.T) {
		g := NewWithT(t)
		machine := &clusterv1.Machine{
			Spec: clusterv1.MachineSpec{
				Version: pointer.String(""),
			},
		}
		g.Expect(collections.WithVersion()(machine)).To(BeFalse())
	})

	t.Run("invalid machine.Spec.Version returns false", func(t *testing.T) {
		g := NewWithT(t)
		machine := &clusterv1.Machine{
			Spec: clusterv1.MachineSpec{
				Version: pointer.String("1..20"),
			},
		}
		g.Expect(collections.WithVersion()(machine)).To(BeFalse())
	})

	t.Run("valid machine.Spec.Version returns true", func(t *testing.T) {
		g := NewWithT(t)
		machine := &clusterv1.Machine{
			Spec: clusterv1.MachineSpec{
				Version: pointer.String("1.20"),
			},
		}
		g.Expect(collections.WithVersion()(machine)).To(BeTrue())
	})
}

func TestHealtyAPIServer(t *testing.T) {
	t.Run("nil machine returns false", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(collections.HealthyAPIServer()(nil)).To(BeFalse())
	})

	t.Run("unhealthy machine returns false", func(t *testing.T) {
		g := NewWithT(t)
		machine := &clusterv1.Machine{}
		g.Expect(collections.HealthyAPIServer()(machine)).To(BeFalse())
	})

	t.Run("healthy machine returns true", func(t *testing.T) {
		g := NewWithT(t)
		machine := &clusterv1.Machine{}
		conditions.Set(machine, conditions.TrueCondition(controlplanev1.MachineAPIServerPodHealthyCondition))
		g.Expect(collections.HealthyAPIServer()(machine)).To(BeTrue())
	})
}

func TestGetFilteredMachinesForCluster(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "my-namespace",
			Name:      "my-cluster",
		},
	}

	c := fake.NewClientBuilder().
		WithObjects(cluster,
			testControlPlaneMachine("first-machine"),
			testMachine("second-machine"),
			testMachine("third-machine")).
		Build()

	machines, err := collections.GetFilteredMachinesForCluster(ctx, c, cluster)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(machines).To(HaveLen(3))

	// Test the ControlPlaneMachines works
	machines, err = collections.GetFilteredMachinesForCluster(ctx, c, cluster, collections.ControlPlaneMachines("my-cluster"))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(machines).To(HaveLen(1))

	// Test that the filters use AND logic instead of OR logic
	nameFilter := func(cluster *clusterv1.Machine) bool {
		return cluster.Name == "first-machine"
	}
	machines, err = collections.GetFilteredMachinesForCluster(ctx, c, cluster, collections.ControlPlaneMachines("my-cluster"), nameFilter)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(machines).To(HaveLen(1))
}

func testControlPlaneMachine(name string) *clusterv1.Machine {
	owned := true
	ownedRef := []metav1.OwnerReference{
		{
			Kind:       "KubeadmControlPlane",
			Name:       "my-control-plane",
			Controller: &owned,
		},
	}
	controlPlaneMachine := testMachine(name)
	controlPlaneMachine.ObjectMeta.Labels[clusterv1.MachineControlPlaneLabelName] = ""
	controlPlaneMachine.OwnerReferences = ownedRef

	return controlPlaneMachine
}

func testMachine(name string) *clusterv1.Machine {
	return &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "my-namespace",
			Labels: map[string]string{
				clusterv1.ClusterLabelName: "my-cluster",
			},
		},
	}
}
