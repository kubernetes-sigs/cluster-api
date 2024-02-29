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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
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
		conditions.MarkTrue(m, clusterv1.MachineHealthCheckSucceededCondition)
		g.Expect(collections.HasUnhealthyCondition(m)).To(BeFalse())
	})
	t.Run("unhealthy machine NOT eligible for KCP remediation (with withHealthCheckSucceeded condition == False but without OwnerRemediated) should return false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		conditions.MarkFalse(m, clusterv1.MachineHealthCheckSucceededCondition, clusterv1.MachineHasFailureReason, clusterv1.ConditionSeverityWarning, "")
		g.Expect(collections.HasUnhealthyCondition(m)).To(BeFalse())
	})
	t.Run("unhealthy machine eligible for KCP (with HealthCheckSucceeded condition == False and with OwnerRemediated) should return true", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		conditions.MarkFalse(m, clusterv1.MachineHealthCheckSucceededCondition, clusterv1.MachineHasFailureReason, clusterv1.ConditionSeverityWarning, "")
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

func TestShouldRolloutBeforeCertificatesExpire(t *testing.T) {
	reconciliationTime := &metav1.Time{Time: time.Now()}
	t.Run("if rolloutBefore is nil it should return false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(collections.ShouldRolloutBefore(reconciliationTime, nil)(m)).To(BeFalse())
	})
	t.Run("if rolloutBefore.certificatesExpiryDays is nil it should return false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(collections.ShouldRolloutBefore(reconciliationTime, &controlplanev1.RolloutBefore{})(m)).To(BeFalse())
	})
	t.Run("if machine is nil it should return false", func(t *testing.T) {
		g := NewWithT(t)
		rb := &controlplanev1.RolloutBefore{CertificatesExpiryDays: ptr.To[int32](10)}
		g.Expect(collections.ShouldRolloutBefore(reconciliationTime, rb)(nil)).To(BeFalse())
	})
	t.Run("if the machine certificate expiry information is not available it should return false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		rb := &controlplanev1.RolloutBefore{CertificatesExpiryDays: ptr.To[int32](10)}
		g.Expect(collections.ShouldRolloutBefore(reconciliationTime, rb)(m)).To(BeFalse())
	})
	t.Run("if the machine certificates are not going to expire within the expiry time it should return false", func(t *testing.T) {
		g := NewWithT(t)
		certificateExpiryTime := reconciliationTime.Add(60 * 24 * time.Hour) // certificates will expire in 60 days from 'now'.
		m := &clusterv1.Machine{
			Status: clusterv1.MachineStatus{
				CertificatesExpiryDate: &metav1.Time{Time: certificateExpiryTime},
			},
		}
		rb := &controlplanev1.RolloutBefore{CertificatesExpiryDays: ptr.To[int32](10)}
		g.Expect(collections.ShouldRolloutBefore(reconciliationTime, rb)(m)).To(BeFalse())
	})
	t.Run("if machine certificates will expire within the expiry time then it should return true", func(t *testing.T) {
		g := NewWithT(t)
		certificateExpiryTime := reconciliationTime.Add(5 * 24 * time.Hour) // certificates will expire in 5 days from 'now'.
		m := &clusterv1.Machine{
			Status: clusterv1.MachineStatus{
				CertificatesExpiryDate: &metav1.Time{Time: certificateExpiryTime},
			},
		}
		rb := &controlplanev1.RolloutBefore{CertificatesExpiryDays: ptr.To[int32](10)}
		g.Expect(collections.ShouldRolloutBefore(reconciliationTime, rb)(m)).To(BeTrue())
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
		g.Expect(collections.InFailureDomains(ptr.To("test"))(nil)).To(BeFalse())
	})
	t.Run("machine with given failure domain returns true", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: ptr.To("test")}}
		g.Expect(collections.InFailureDomains(ptr.To("test"))(m)).To(BeTrue())
	})
	t.Run("machine with a different failure domain returns false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: ptr.To("notTest")}}
		g.Expect(collections.InFailureDomains(
			ptr.To("test"),
			ptr.To("test2"),
			ptr.To("test3"),
			nil,
			ptr.To("foo"))(m)).To(BeFalse())
	})
	t.Run("machine without failure domain returns false", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(collections.InFailureDomains(ptr.To("test"))(m)).To(BeFalse())
	})
	t.Run("machine without failure domain returns true, when nil used for failure domain", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{}
		g.Expect(collections.InFailureDomains(nil)(m)).To(BeTrue())
	})
	t.Run("machine with failure domain returns true, when one of multiple failure domains match", func(t *testing.T) {
		g := NewWithT(t)
		m := &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: ptr.To("test")}}
		g.Expect(collections.InFailureDomains(ptr.To("foo"), ptr.To("test"))(m)).To(BeTrue())
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
				Version: ptr.To(""),
			},
		}
		g.Expect(collections.WithVersion()(machine)).To(BeFalse())
	})

	t.Run("invalid machine.Spec.Version returns false", func(t *testing.T) {
		g := NewWithT(t)
		machine := &clusterv1.Machine{
			Spec: clusterv1.MachineSpec{
				Version: ptr.To("1..20"),
			},
		}
		g.Expect(collections.WithVersion()(machine)).To(BeFalse())
	})

	t.Run("valid machine.Spec.Version returns true", func(t *testing.T) {
		g := NewWithT(t)
		machine := &clusterv1.Machine{
			Spec: clusterv1.MachineSpec{
				Version: ptr.To("1.20"),
			},
		}
		g.Expect(collections.WithVersion()(machine)).To(BeTrue())
	})
}

func TestHealthyAPIServer(t *testing.T) {
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
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(machines).To(HaveLen(3))

	// Test the ControlPlaneMachines works
	machines, err = collections.GetFilteredMachinesForCluster(ctx, c, cluster, collections.ControlPlaneMachines("my-cluster"))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(machines).To(HaveLen(1))

	// Test that the filters use AND logic instead of OR logic
	nameFilter := func(cluster *clusterv1.Machine) bool {
		return cluster.Name == "first-machine"
	}
	machines, err = collections.GetFilteredMachinesForCluster(ctx, c, cluster, collections.ControlPlaneMachines("my-cluster"), nameFilter)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(machines).To(HaveLen(1))
}

func TestHasNode(t *testing.T) {
	t.Run("nil machine returns false", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(collections.HasNode()(nil)).To(BeFalse())
	})

	t.Run("machine without node returns false", func(t *testing.T) {
		g := NewWithT(t)
		machine := &clusterv1.Machine{}
		g.Expect(collections.HasNode()(machine)).To(BeFalse())
	})

	t.Run("machine with node returns true", func(t *testing.T) {
		g := NewWithT(t)
		machine := &clusterv1.Machine{
			Status: clusterv1.MachineStatus{NodeRef: &corev1.ObjectReference{Name: "foo"}},
		}
		g.Expect(collections.HasNode()(machine)).To(BeTrue())
	})
}

func TestHasUnhealthyControlPlaneComponentCondition(t *testing.T) {
	t.Run("nil machine returns true", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(collections.HasUnhealthyControlPlaneComponentCondition(false)(nil)).To(BeTrue())
	})

	t.Run("machine without node returns true", func(t *testing.T) {
		g := NewWithT(t)
		machine := &clusterv1.Machine{}
		g.Expect(collections.HasUnhealthyControlPlaneComponentCondition(false)(machine)).To(BeTrue())
	})

	t.Run("machine with all healthy controlPlane component conditions returns false when the Etcd is not managed", func(t *testing.T) {
		g := NewWithT(t)
		machine := &clusterv1.Machine{}
		machine.Status.NodeRef = &corev1.ObjectReference{
			Name: "node1",
		}
		machine.Status.Conditions = clusterv1.Conditions{
			*conditions.TrueCondition(controlplanev1.MachineAPIServerPodHealthyCondition),
			*conditions.TrueCondition(controlplanev1.MachineControllerManagerPodHealthyCondition),
			*conditions.TrueCondition(controlplanev1.MachineSchedulerPodHealthyCondition),
		}
		g.Expect(collections.HasUnhealthyControlPlaneComponentCondition(false)(machine)).To(BeFalse())
	})

	t.Run("machine with unhealthy 'APIServerPodHealthy' condition returns true when the Etcd is not managed", func(t *testing.T) {
		g := NewWithT(t)
		machine := &clusterv1.Machine{}
		machine.Status.NodeRef = &corev1.ObjectReference{
			Name: "node1",
		}
		machine.Status.Conditions = clusterv1.Conditions{
			*conditions.TrueCondition(controlplanev1.MachineControllerManagerPodHealthyCondition),
			*conditions.TrueCondition(controlplanev1.MachineSchedulerPodHealthyCondition),
			*conditions.FalseCondition(controlplanev1.MachineAPIServerPodHealthyCondition, "",
				clusterv1.ConditionSeverityWarning, ""),
		}
		g.Expect(collections.HasUnhealthyControlPlaneComponentCondition(false)(machine)).To(BeTrue())
	})

	t.Run("machine with unhealthy etcd component conditions returns false when Etcd is not managed", func(t *testing.T) {
		g := NewWithT(t)
		machine := &clusterv1.Machine{}
		machine.Status.NodeRef = &corev1.ObjectReference{
			Name: "node1",
		}
		machine.Status.Conditions = clusterv1.Conditions{
			*conditions.TrueCondition(controlplanev1.MachineAPIServerPodHealthyCondition),
			*conditions.TrueCondition(controlplanev1.MachineControllerManagerPodHealthyCondition),
			*conditions.TrueCondition(controlplanev1.MachineSchedulerPodHealthyCondition),
			*conditions.FalseCondition(controlplanev1.MachineEtcdPodHealthyCondition, "",
				clusterv1.ConditionSeverityWarning, ""),
			*conditions.FalseCondition(controlplanev1.MachineEtcdMemberHealthyCondition, "",
				clusterv1.ConditionSeverityWarning, ""),
		}
		g.Expect(collections.HasUnhealthyControlPlaneComponentCondition(false)(machine)).To(BeFalse())
	})

	t.Run("machine with unhealthy etcd conditions returns true when Etcd is managed", func(t *testing.T) {
		g := NewWithT(t)
		machine := &clusterv1.Machine{}
		machine.Status.NodeRef = &corev1.ObjectReference{
			Name: "node1",
		}
		machine.Status.Conditions = clusterv1.Conditions{
			*conditions.TrueCondition(controlplanev1.MachineAPIServerPodHealthyCondition),
			*conditions.TrueCondition(controlplanev1.MachineControllerManagerPodHealthyCondition),
			*conditions.TrueCondition(controlplanev1.MachineSchedulerPodHealthyCondition),
			*conditions.FalseCondition(controlplanev1.MachineEtcdPodHealthyCondition, "",
				clusterv1.ConditionSeverityWarning, ""),
			*conditions.FalseCondition(controlplanev1.MachineEtcdMemberHealthyCondition, "",
				clusterv1.ConditionSeverityWarning, ""),
		}
		g.Expect(collections.HasUnhealthyControlPlaneComponentCondition(true)(machine)).To(BeTrue())
	})

	t.Run("machine with all healthy controlPlane and the Etcd component conditions returns false when Etcd is managed", func(t *testing.T) {
		g := NewWithT(t)
		machine := &clusterv1.Machine{}
		machine.Status.NodeRef = &corev1.ObjectReference{
			Name: "node1",
		}
		machine.Status.Conditions = clusterv1.Conditions{
			*conditions.TrueCondition(controlplanev1.MachineAPIServerPodHealthyCondition),
			*conditions.TrueCondition(controlplanev1.MachineControllerManagerPodHealthyCondition),
			*conditions.TrueCondition(controlplanev1.MachineSchedulerPodHealthyCondition),
			*conditions.TrueCondition(controlplanev1.MachineEtcdPodHealthyCondition),
			*conditions.TrueCondition(controlplanev1.MachineEtcdMemberHealthyCondition),
		}
		g.Expect(collections.HasUnhealthyControlPlaneComponentCondition(true)(machine)).To(BeFalse())
	})
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
	controlPlaneMachine.ObjectMeta.Labels[clusterv1.MachineControlPlaneLabel] = ""
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
				clusterv1.ClusterNameLabel: "my-cluster",
			},
		},
	}
}
