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

package controllers

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	utilfeature "k8s.io/component-base/featuregate/testing"
	utilptr "k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestGetMachineToBeRemediated(t *testing.T) {
	t.Run("returns the machine with RemediateMachineAnnotation first", func(t *testing.T) {
		g := NewWithT(t)

		ns, err := env.CreateNamespace(ctx, "ns1")
		g.Expect(err).ToNot(HaveOccurred())
		defer func() {
			g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
		}()

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed())
		m2 := createMachine(ctx, g, ns.Name, "m2-unhealthy-", withMachineHealthCheckFailed(), withoutNodeRef(), withUnhealthyEtcdMember(), withUnhealthyAPIServerPod())

		m1.SetAnnotations(map[string]string{clusterv1.RemediateMachineAnnotation: ""})

		unhealthyMachines := collections.FromMachines(m2, m1)

		g.Expect(getMachineToBeRemediated(unhealthyMachines, true).Name).To(HavePrefix("m1-unhealthy-"))
	})

	t.Run("returns provisioning machines first", func(t *testing.T) {
		g := NewWithT(t)

		ns, err := env.CreateNamespace(ctx, "ns1")
		g.Expect(err).ToNot(HaveOccurred())
		defer func() {
			g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
		}()

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed(), withUnhealthyEtcdMember(), withUnhealthyAPIServerPod()) // Note issue on etcd / API server have lower priority than the lack of node.
		m2 := createMachine(ctx, g, ns.Name, "m2-unhealthy-", withMachineHealthCheckFailed(), withoutNodeRef())

		unhealthyMachines := collections.FromMachines(m1, m2)

		g.Expect(getMachineToBeRemediated(unhealthyMachines, true).Name).To(HavePrefix("m2-unhealthy-"))
	})

	t.Run("returns the machines with etcd errors first (if there are no provisioning machines)", func(t *testing.T) {
		g := NewWithT(t)

		ns, err := env.CreateNamespace(ctx, "ns1")
		g.Expect(err).ToNot(HaveOccurred())
		defer func() {
			g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
		}()

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed(), withHealthyEtcdMember(), withUnhealthyAPIServerPod()) // Note issue on API server have lower priority than issue on etcd
		m2 := createMachine(ctx, g, ns.Name, "m2-unhealthy-", withMachineHealthCheckFailed(), withUnhealthyEtcdMember())

		unhealthyMachines := collections.FromMachines(m1, m2)

		g.Expect(getMachineToBeRemediated(unhealthyMachines, true).Name).To(HavePrefix("m2-unhealthy-"))
	})

	t.Run("returns the machines with API server errors first (if there are no provisioning machines and no etcd issues)", func(t *testing.T) {
		g := NewWithT(t)

		ns, err := env.CreateNamespace(ctx, "ns1")
		g.Expect(err).ToNot(HaveOccurred())
		defer func() {
			g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
		}()

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed(), withHealthyAPIServerPod())
		m2 := createMachine(ctx, g, ns.Name, "m2-unhealthy-", withMachineHealthCheckFailed(), withUnhealthyAPIServerPod())

		unhealthyMachines := collections.FromMachines(m1, m2)

		g.Expect(getMachineToBeRemediated(unhealthyMachines, true).Name).To(HavePrefix("m2-unhealthy-"))
	})
	t.Run("returns the oldest machine if there are no provisioning machines/no other elements affecting priority", func(t *testing.T) {
		g := NewWithT(t)

		ns, err := env.CreateNamespace(ctx, "ns1")
		g.Expect(err).ToNot(HaveOccurred())
		defer func() {
			g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
		}()

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed())
		m2 := createMachine(ctx, g, ns.Name, "m2-unhealthy-", withMachineHealthCheckFailed())

		unhealthyMachines := collections.FromMachines(m1, m2)

		g.Expect(getMachineToBeRemediated(unhealthyMachines, true).Name).To(HavePrefix("m1-unhealthy-"))
	})
}

func TestReconcileUnhealthyMachines(t *testing.T) {
	g := NewWithT(t)

	r := &KubeadmControlPlaneReconciler{
		Client:   env.GetClient(),
		recorder: record.NewFakeRecorder(32),
	}
	ns, err := env.CreateNamespace(ctx, "ns1")
	g.Expect(err).ToNot(HaveOccurred())
	defer func() {
		g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
	}()

	var removeFinalizer = func(g *WithT, m *clusterv1.Machine) {
		patchHelper, err := patch.NewHelper(m, env.GetClient())
		g.Expect(err).ToNot(HaveOccurred())
		m.Finalizers = nil
		g.Expect(patchHelper.Patch(ctx, m)).To(Succeed())
	}

	t.Run("It cleans up stuck remediation on previously unhealthy machines", func(t *testing.T) {
		g := NewWithT(t)

		m := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withStuckRemediation())

		controlPlane := &internal.ControlPlane{
			KCP:      &controlplanev1.KubeadmControlPlane{},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m),
		}
		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeTrue()) // Remediation skipped
		g.Expect(err).ToNot(HaveOccurred())

		g.Eventually(func() error {
			if err := env.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: m.Name}, m); err != nil {
				return err
			}
			if c := v1beta1conditions.Get(m, clusterv1.MachineOwnerRemediatedV1Beta1Condition); c != nil {
				return errors.Errorf("condition %s still exists", clusterv1.MachineOwnerRemediatedV1Beta1Condition)
			}
			if c := conditions.Get(m, clusterv1.MachineOwnerRemediatedCondition); c != nil {
				return errors.Errorf("condition %s still exists", clusterv1.MachineOwnerRemediatedCondition)
			}
			return nil
		}, 10*time.Second).Should(Succeed())
	})

	// Generic preflight checks
	// Those are ore flight checks that happen no matter if the control plane has been already initialized or not.

	t.Run("Remediation does not happen if there are no unhealthy machines", func(t *testing.T) {
		g := NewWithT(t)

		controlPlane := &internal.ControlPlane{
			KCP:      &controlplanev1.KubeadmControlPlane{},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.New(),
		}
		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeTrue()) // Remediation skipped
		g.Expect(err).ToNot(HaveOccurred())
	})
	t.Run("reconcileUnhealthyMachines return early if another remediation is in progress", func(t *testing.T) {
		g := NewWithT(t)

		m := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed())
		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						controlplanev1.RemediationInProgressAnnotation: MustMarshalRemediationData(&RemediationData{
							Machine:    "foo",
							Timestamp:  metav1.Time{Time: time.Now().UTC()},
							RetryCount: 0,
						}),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m),
		}
		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeTrue()) // Remediation skipped
		g.Expect(err).ToNot(HaveOccurred())
	})
	t.Run("remediation in progress is ignored when stale", func(t *testing.T) {
		g := NewWithT(t)

		m := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer())
		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						controlplanev1.RemediationInProgressAnnotation: MustMarshalRemediationData(&RemediationData{
							Machine:    "foo",
							Timestamp:  metav1.Time{Time: time.Now().Add(-1 * time.Hour).UTC()},
							RetryCount: 0,
						}),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m),
		}
		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeFalse()) // Remediation completed, requeue
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).To(HaveKey(controlplanev1.RemediationInProgressAnnotation))
		remediationData, err := RemediationDataFromAnnotation(controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation])
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(remediationData.Machine).To(Equal(m.Name))
		g.Expect(remediationData.RetryCount).To(Equal(0))

		assertMachineV1beta1Condition(ctx, g, m, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.RemediationInProgressV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")
		assertMachineCondition(ctx, g, m, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationMachineDeletingReason, "Machine is deleting")

		err = env.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: m.Name}, m)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(m.ObjectMeta.DeletionTimestamp.IsZero()).To(BeFalse())

		removeFinalizer(g, m)
		g.Expect(env.Cleanup(ctx, m)).To(Succeed())
	})
	t.Run("reconcileUnhealthyMachines return early if the machine to be remediated is already being deleted", func(t *testing.T) {
		g := NewWithT(t)

		m := getDeletingMachine(ns.Name, "m1-unhealthy-deleting-", withMachineHealthCheckFailed())
		controlPlane := &internal.ControlPlane{
			KCP:      &controlplanev1.KubeadmControlPlane{},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m),
		}
		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(controlPlane.KCP.Annotations).ToNot(HaveKey(controlplanev1.RemediationInProgressAnnotation))

		g.Expect(ret.IsZero()).To(BeTrue()) // Remediation skipped
		g.Expect(err).ToNot(HaveOccurred())
	})
	t.Run("reconcileUnhealthyMachines return early if there is a pending topology upgrade", func(t *testing.T) {
		utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)

		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer())
		m2 := createMachine(ctx, g, ns.Name, "m2-healthy-", withHealthyEtcdMember())
		m3 := createMachine(ctx, g, ns.Name, "m3-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To[int32](3),
					Version:  "v1.19.1",
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Initialization: controlplanev1.KubeadmControlPlaneInitializationStatus{
						ControlPlaneInitialized: utilptr.To(true),
					},
				},
			},
			Cluster: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					Topology: clusterv1.Topology{
						Version: "v1.20.1",
					},
				},
			},
			Machines: collections.FromMachines(m1, m2, m3),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)
		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeTrue()) // Remediation skipped
		g.Expect(err).ToNot(HaveOccurred())

		assertMachineV1beta1Condition(ctx, g, m1, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.WaitingForRemediationV1Beta1Reason, clusterv1.ConditionSeverityWarning, "KubeadmControlPlane can't remediate while waiting for a version upgrade to v1.20.1 to be propagated from Cluster.spec.topology")
		assertMachineCondition(ctx, g, m1, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationDeferredReason, "KubeadmControlPlane can't remediate while waiting for a version upgrade to v1.20.1 to be propagated from Cluster.spec.topology")

		err = env.Get(ctx, client.ObjectKey{Namespace: m1.Namespace, Name: m1.Name}, m1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(m1.ObjectMeta.DeletionTimestamp.IsZero()).To(BeTrue())

		g.Expect(env.Cleanup(ctx, m1, m2, m3)).To(Succeed())
	})
	t.Run("Remediation does not happen if MaxRetry is reached", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer(), withRemediateForAnnotation(MustMarshalRemediationData(&RemediationData{
			Machine:    "m0",
			Timestamp:  metav1.Time{Time: time.Now().Add(-time.Duration(controlplanev1.DefaultMinHealthyPeriodSeconds) * time.Second / 2).UTC()}, // minHealthy not expired yet.
			RetryCount: 3,
		})))
		m2 := createMachine(ctx, g, ns.Name, "m2-healthy-", withHealthyEtcdMember())
		m3 := createMachine(ctx, g, ns.Name, "m3-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To[int32](3),
					Version:  "v1.19.1",
					Remediation: controlplanev1.KubeadmControlPlaneRemediationSpec{
						MaxRetry: utilptr.To[int32](3),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}

		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeTrue()) // Remediation skipped
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).ToNot(HaveKey(controlplanev1.RemediationInProgressAnnotation))

		assertMachineV1beta1Condition(ctx, g, m1, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.WaitingForRemediationV1Beta1Reason, clusterv1.ConditionSeverityWarning, "KubeadmControlPlane can't remediate this machine because the operation already failed 3 times (MaxRetry)")
		assertMachineCondition(ctx, g, m1, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineCannotBeRemediatedReason, "KubeadmControlPlane can't remediate this machine because the operation already failed 3 times (MaxRetry)")

		err = env.Get(ctx, client.ObjectKey{Namespace: m1.Namespace, Name: m1.Name}, m1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(m1.ObjectMeta.DeletionTimestamp.IsZero()).To(BeTrue())

		removeFinalizer(g, m1)
		g.Expect(env.Cleanup(ctx, m1, m2, m3)).To(Succeed())
	})
	t.Run("Retry history is ignored if min healthy period is expired, default min healthy period", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer(), withRemediateForAnnotation(MustMarshalRemediationData(&RemediationData{
			Machine:    "m0",
			Timestamp:  metav1.Time{Time: time.Now().Add(-2 * time.Duration(controlplanev1.DefaultMinHealthyPeriodSeconds) * time.Second).UTC()}, // minHealthyPeriod already expired.
			RetryCount: 3,
		})))
		m2 := createMachine(ctx, g, ns.Name, "m2-healthy-", withHealthyEtcdMember())
		m3 := createMachine(ctx, g, ns.Name, "m3-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To[int32](3),
					Version:  "v1.19.1",
					Remediation: controlplanev1.KubeadmControlPlaneRemediationSpec{
						MaxRetry: utilptr.To[int32](3),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}

		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeFalse()) // Remediation completed, requeue
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).To(HaveKey(controlplanev1.RemediationInProgressAnnotation))
		remediationData, err := RemediationDataFromAnnotation(controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation])
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(remediationData.Machine).To(Equal(m1.Name))
		g.Expect(remediationData.RetryCount).To(Equal(0))

		assertMachineV1beta1Condition(ctx, g, m1, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.RemediationInProgressV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")
		assertMachineCondition(ctx, g, m1, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationMachineDeletingReason, "Machine is deleting")

		err = env.Get(ctx, client.ObjectKey{Namespace: m1.Namespace, Name: m1.Name}, m1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(m1.ObjectMeta.DeletionTimestamp.IsZero()).To(BeFalse())

		removeFinalizer(g, m1)
		g.Expect(env.Cleanup(ctx, m1, m2, m3)).To(Succeed())
	})
	t.Run("Retry history is ignored if min healthy period is expired", func(t *testing.T) {
		g := NewWithT(t)

		minHealthyPeriod := 4 * controlplanev1.DefaultMinHealthyPeriodSeconds // big min healthy period, so we are user that we are not using DefaultMinHealthyPeriodSeconds.

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer(), withRemediateForAnnotation(MustMarshalRemediationData(&RemediationData{
			Machine:    "m0",
			Timestamp:  metav1.Time{Time: time.Now().Add(-2 * time.Duration(minHealthyPeriod) * time.Second).UTC()}, // minHealthyPeriod already expired.
			RetryCount: 3,
		})))
		m2 := createMachine(ctx, g, ns.Name, "m2-healthy-", withHealthyEtcdMember())
		m3 := createMachine(ctx, g, ns.Name, "m3-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To[int32](3),
					Version:  "v1.19.1",
					Remediation: controlplanev1.KubeadmControlPlaneRemediationSpec{
						MaxRetry:                utilptr.To[int32](3),
						MinHealthyPeriodSeconds: utilptr.To(minHealthyPeriod),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}

		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeFalse()) // Remediation completed, requeue
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).To(HaveKey(controlplanev1.RemediationInProgressAnnotation))
		remediationData, err := RemediationDataFromAnnotation(controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation])
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(remediationData.Machine).To(Equal(m1.Name))
		g.Expect(remediationData.RetryCount).To(Equal(0))

		assertMachineV1beta1Condition(ctx, g, m1, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.RemediationInProgressV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")
		assertMachineCondition(ctx, g, m1, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationMachineDeletingReason, "Machine is deleting")

		err = env.Get(ctx, client.ObjectKey{Namespace: m1.Namespace, Name: m1.Name}, m1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(m1.ObjectMeta.DeletionTimestamp.IsZero()).To(BeFalse())

		removeFinalizer(g, m1)
		g.Expect(env.Cleanup(ctx, m1, m2, m3)).To(Succeed())
	})
	t.Run("Remediation does not happen if RetryPeriodSeconds is not yet passed", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer(), withRemediateForAnnotation(MustMarshalRemediationData(&RemediationData{
			Machine:    "m0",
			Timestamp:  metav1.Time{Time: time.Now().Add(-time.Duration(controlplanev1.DefaultMinHealthyPeriodSeconds) * time.Second / 2).UTC()}, // minHealthyPeriod not yet expired.
			RetryCount: 2,
		})))
		m2 := createMachine(ctx, g, ns.Name, "m2-healthy-", withHealthyEtcdMember())
		m3 := createMachine(ctx, g, ns.Name, "m3-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To[int32](3),
					Version:  "v1.19.1",
					Remediation: controlplanev1.KubeadmControlPlaneRemediationSpec{
						MaxRetry:           utilptr.To[int32](3),
						RetryPeriodSeconds: utilptr.To(controlplanev1.DefaultMinHealthyPeriodSeconds), // RetryPeriodSeconds not yet expired.
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}

		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeTrue()) // Remediation skipped
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).ToNot(HaveKey(controlplanev1.RemediationInProgressAnnotation))

		assertMachineV1beta1Condition(ctx, g, m1, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.WaitingForRemediationV1Beta1Reason, clusterv1.ConditionSeverityWarning, "KubeadmControlPlane can't remediate this machine because the operation already failed in the latest 1h0m0s (RetryPeriodSeconds)")
		assertMachineCondition(ctx, g, m1, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationDeferredReason, "KubeadmControlPlane can't remediate this machine because the operation already failed in the latest 1h0m0s (RetryPeriodSeconds)")

		err = env.Get(ctx, client.ObjectKey{Namespace: m1.Namespace, Name: m1.Name}, m1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(m1.ObjectMeta.DeletionTimestamp.IsZero()).To(BeTrue())

		removeFinalizer(g, m1)
		g.Expect(env.Cleanup(ctx, m1, m2, m3)).To(Succeed())
	})

	// There are no preflight checks for when control plane is not yet initialized
	// (it is the first CP, we can nuke it).

	// Preflight checks for when control plane is already initialized.

	t.Run("Remediation does not happen if desired replicas <= 1", func(t *testing.T) {
		g := NewWithT(t)

		m := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed())
		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To[int32](1),
					Rollout: controlplanev1.KubeadmControlPlaneRolloutSpec{
						Strategy: controlplanev1.KubeadmControlPlaneRolloutStrategy{
							RollingUpdate: controlplanev1.KubeadmControlPlaneRolloutStrategyRollingUpdate{
								MaxSurge: &intstr.IntOrString{
									IntVal: 1,
								},
							},
						},
					},
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Initialization: controlplanev1.KubeadmControlPlaneInitializationStatus{
						ControlPlaneInitialized: utilptr.To(true),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m),
		}
		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeTrue()) // Remediation skipped
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).ToNot(HaveKey(controlplanev1.RemediationInProgressAnnotation))

		assertMachineV1beta1Condition(ctx, g, m, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.WaitingForRemediationV1Beta1Reason, clusterv1.ConditionSeverityWarning, "KubeadmControlPlane can't remediate if current replicas are less or equal to 1")
		assertMachineCondition(ctx, g, m, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineCannotBeRemediatedReason, "KubeadmControlPlane can't remediate if current replicas are less or equal to 1")

		g.Expect(env.Cleanup(ctx, m)).To(Succeed())
	})
	t.Run("Remediation does not happen if there is another machine being deleted (not the one to be remediated)", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed())
		m2 := createMachine(ctx, g, ns.Name, "m2-healthy-")
		m3 := getDeletingMachine(ns.Name, "m3-deleting") // NB. This machine is not created, it gets only added to control plane
		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To[int32](3),
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Initialization: controlplanev1.KubeadmControlPlaneInitializationStatus{
						ControlPlaneInitialized: utilptr.To(true),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3),
		}
		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeTrue()) // Remediation skipped
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).ToNot(HaveKey(controlplanev1.RemediationInProgressAnnotation))

		assertMachineV1beta1Condition(ctx, g, m1, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.WaitingForRemediationV1Beta1Reason, clusterv1.ConditionSeverityWarning, "KubeadmControlPlane waiting for control plane machine deletion to complete before triggering remediation")
		assertMachineCondition(ctx, g, m1, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationDeferredReason, "KubeadmControlPlane waiting for control plane Machine deletion to complete before triggering remediation")

		g.Expect(env.Cleanup(ctx, m1, m2)).To(Succeed())
	})
	t.Run("Remediation does not happen if there is an healthy machine being provisioned", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed())
		m2 := createMachine(ctx, g, ns.Name, "m2-healthy-")
		m3 := createMachine(ctx, g, ns.Name, "m3-healthy-", withoutNodeRef()) // Provisioning
		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To(int32(3)),
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Initialization: controlplanev1.KubeadmControlPlaneInitializationStatus{
						ControlPlaneInitialized: utilptr.To(true),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3),
		}
		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeTrue()) // Remediation skipped
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).ToNot(HaveKey(controlplanev1.RemediationInProgressAnnotation))

		assertMachineV1beta1Condition(ctx, g, m1, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.WaitingForRemediationV1Beta1Reason, clusterv1.ConditionSeverityWarning, "KubeadmControlPlane waiting for control plane machine provisioning to complete before triggering remediation")
		assertMachineCondition(ctx, g, m1, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationDeferredReason, "KubeadmControlPlane waiting for control plane Machine provisioning to complete before triggering remediation")

		g.Expect(env.Cleanup(ctx, m1, m2)).To(Succeed())
	})
	t.Run("Remediation does not happen if there is an healthy machine being provisioned - 4 CP (during 3 CP rolling upgrade)", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed())
		m2 := createMachine(ctx, g, ns.Name, "m2-healthy-")
		m3 := createMachine(ctx, g, ns.Name, "m3-healthy-")
		m4 := createMachine(ctx, g, ns.Name, "m4-healthy-", withoutNodeRef()) // Provisioning
		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To(int32(3)),
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Initialization: controlplanev1.KubeadmControlPlaneInitializationStatus{
						ControlPlaneInitialized: utilptr.To(true),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3, m4),
		}
		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeTrue()) // Remediation skipped
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).ToNot(HaveKey(controlplanev1.RemediationInProgressAnnotation))

		assertMachineV1beta1Condition(ctx, g, m1, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.WaitingForRemediationV1Beta1Reason, clusterv1.ConditionSeverityWarning, "KubeadmControlPlane waiting for control plane machine provisioning to complete before triggering remediation")
		assertMachineCondition(ctx, g, m1, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationDeferredReason, "KubeadmControlPlane waiting for control plane Machine provisioning to complete before triggering remediation")

		g.Expect(env.Cleanup(ctx, m1, m2)).To(Succeed())
	})
	t.Run("Remediation does not happen if there is at least one additional unhealthy etcd member on a 3 machine CP", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-mhc-unhealthy-", withMachineHealthCheckFailed())
		m2 := createMachine(ctx, g, ns.Name, "m2-etcd-unhealthy-", withUnhealthyEtcdMember())
		m3 := createMachine(ctx, g, ns.Name, "m3-etcd-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To[int32](3),
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Initialization: controlplanev1.KubeadmControlPlaneInitializationStatus{
						ControlPlaneInitialized: utilptr.To(true),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeTrue()) // Remediation skipped
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).ToNot(HaveKey(controlplanev1.RemediationInProgressAnnotation))

		assertMachineV1beta1Condition(ctx, g, m1, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.WaitingForRemediationV1Beta1Reason, clusterv1.ConditionSeverityWarning, "KubeadmControlPlane can't remediate this machine because this could result in etcd loosing quorum")
		assertMachineCondition(ctx, g, m1, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineCannotBeRemediatedReason, "KubeadmControlPlane can't remediate this Machine because this could result in etcd loosing quorum")

		g.Expect(env.Cleanup(ctx, m1, m2, m3)).To(Succeed())
	})
	t.Run("Remediation does not happen if there are at least two additional unhealthy etcd member on a 5 machine CP", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-mhc-unhealthy-", withMachineHealthCheckFailed())
		m2 := createMachine(ctx, g, ns.Name, "m2-etcd-unhealthy-", withUnhealthyEtcdMember())
		m3 := createMachine(ctx, g, ns.Name, "m3-etcd-unhealthy-", withUnhealthyEtcdMember())
		m4 := createMachine(ctx, g, ns.Name, "m4-etcd-healthy-", withHealthyEtcdMember())
		m5 := createMachine(ctx, g, ns.Name, "m5-etcd-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To[int32](5),
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Initialization: controlplanev1.KubeadmControlPlaneInitializationStatus{
						ControlPlaneInitialized: utilptr.To(true),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3, m4, m5),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeTrue()) // Remediation skipped
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).ToNot(HaveKey(controlplanev1.RemediationInProgressAnnotation))

		assertMachineV1beta1Condition(ctx, g, m1, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.WaitingForRemediationV1Beta1Reason, clusterv1.ConditionSeverityWarning, "KubeadmControlPlane can't remediate this machine because this could result in etcd loosing quorum")
		assertMachineCondition(ctx, g, m1, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineCannotBeRemediatedReason, "KubeadmControlPlane can't remediate this Machine because this could result in etcd loosing quorum")

		g.Expect(env.Cleanup(ctx, m1, m2, m3, m4, m5)).To(Succeed())
	})

	// Remediation for when control plane is not yet initialized

	t.Run("Remediation deletes unhealthy machine - 1 CP not initialized", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To[int32](1),
					Version:  "v1.19.1",
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Initialization: controlplanev1.KubeadmControlPlaneInitializationStatus{
						ControlPlaneInitialized: utilptr.To(false),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}

		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeFalse()) // Remediation completed, requeue
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).To(HaveKey(controlplanev1.RemediationInProgressAnnotation))
		remediationData, err := RemediationDataFromAnnotation(controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation])
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(remediationData.Machine).To(Equal(m1.Name))
		g.Expect(remediationData.RetryCount).To(Equal(0))

		assertMachineV1beta1Condition(ctx, g, m1, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.RemediationInProgressV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")
		assertMachineCondition(ctx, g, m1, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationMachineDeletingReason, "Machine is deleting")

		err = env.Get(ctx, client.ObjectKey{Namespace: m1.Namespace, Name: m1.Name}, m1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(m1.ObjectMeta.DeletionTimestamp.IsZero()).To(BeFalse())

		removeFinalizer(g, m1)
		g.Expect(env.Cleanup(ctx, m1)).To(Succeed())
	})
	t.Run("Subsequent remediation of the same machine increase retry count - 1 CP not initialized", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To[int32](1),
					Version:  "v1.19.1",
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Initialization: controlplanev1.KubeadmControlPlaneInitializationStatus{
						ControlPlaneInitialized: utilptr.To(false),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		// First reconcile, remediate machine m1 for the first time
		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}

		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeFalse()) // Remediation completed, requeue
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).To(HaveKey(controlplanev1.RemediationInProgressAnnotation))
		remediationData, err := RemediationDataFromAnnotation(controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation])
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(remediationData.Machine).To(Equal(m1.Name))
		g.Expect(remediationData.RetryCount).To(Equal(0))

		assertMachineV1beta1Condition(ctx, g, m1, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.RemediationInProgressV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")
		assertMachineCondition(ctx, g, m1, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationMachineDeletingReason, "Machine is deleting")

		err = env.Get(ctx, client.ObjectKey{Namespace: m1.Namespace, Name: m1.Name}, m1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(m1.ObjectMeta.DeletionTimestamp.IsZero()).To(BeFalse())

		removeFinalizer(g, m1)
		g.Expect(env.CleanupAndWait(ctx, m1)).To(Succeed())

		for i := 2; i < 4; i++ {
			// Simulate the creation of a replacement for 0.
			mi := createMachine(ctx, g, ns.Name, fmt.Sprintf("m%d-unhealthy-", i), withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer(), withRemediateForAnnotation(MustMarshalRemediationData(remediationData)))

			// Simulate KCP dropping RemediationInProgressAnnotation after creating the replacement machine.
			delete(controlPlane.KCP.Annotations, controlplanev1.RemediationInProgressAnnotation)

			controlPlane.Machines = collections.FromMachines(mi)
			controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

			// Reconcile unhealthy replacements for m1.
			r.managementCluster = &fakeManagementCluster{Workload: &fakeWorkloadCluster{}}
			ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

			g.Expect(ret.IsZero()).To(BeFalse()) // Remediation completed, requeue
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(controlPlane.KCP.Annotations).To(HaveKey(controlplanev1.RemediationInProgressAnnotation))
			remediationData, err = RemediationDataFromAnnotation(controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation])
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(remediationData.Machine).To(Equal(mi.Name))
			g.Expect(remediationData.RetryCount).To(Equal(i - 1))

			assertMachineV1beta1Condition(ctx, g, mi, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.RemediationInProgressV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")
			assertMachineCondition(ctx, g, mi, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationMachineDeletingReason, "Machine is deleting")

			err = env.Get(ctx, client.ObjectKey{Namespace: mi.Namespace, Name: mi.Name}, mi)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(mi.ObjectMeta.DeletionTimestamp.IsZero()).To(BeFalse())

			removeFinalizer(g, mi)
			g.Expect(env.CleanupAndWait(ctx, mi)).To(Succeed())
		}
	})

	// Remediation for when control plane is already initialized

	t.Run("Remediation deletes unhealthy machine - 2 CP (during 1 CP rolling upgrade)", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer())
		m2 := createMachine(ctx, g, ns.Name, "m2-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To[int32](2),
					Version:  "v1.19.1",
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Initialization: controlplanev1.KubeadmControlPlaneInitializationStatus{
						ControlPlaneInitialized: utilptr.To(true),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeFalse()) // Remediation completed, requeue
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).To(HaveKey(controlplanev1.RemediationInProgressAnnotation))
		remediationData, err := RemediationDataFromAnnotation(controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation])
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(remediationData.Machine).To(Equal(m1.Name))
		g.Expect(remediationData.RetryCount).To(Equal(0))

		assertMachineV1beta1Condition(ctx, g, m1, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.RemediationInProgressV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")
		assertMachineCondition(ctx, g, m1, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationMachineDeletingReason, "Machine is deleting")

		err = env.Get(ctx, client.ObjectKey{Namespace: m1.Namespace, Name: m1.Name}, m1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(m1.ObjectMeta.DeletionTimestamp.IsZero()).To(BeFalse())

		removeFinalizer(g, m1)
		g.Expect(env.Cleanup(ctx, m1, m2)).To(Succeed())
	})
	t.Run("Remediation deletes unhealthy machine - 3 CP", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer())
		m2 := createMachine(ctx, g, ns.Name, "m2-healthy-", withHealthyEtcdMember())
		m3 := createMachine(ctx, g, ns.Name, "m3-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To[int32](3),
					Version:  "v1.19.1",
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Initialization: controlplanev1.KubeadmControlPlaneInitializationStatus{
						ControlPlaneInitialized: utilptr.To(true),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeFalse()) // Remediation completed, requeue
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).To(HaveKey(controlplanev1.RemediationInProgressAnnotation))
		remediationData, err := RemediationDataFromAnnotation(controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation])
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(remediationData.Machine).To(Equal(m1.Name))
		g.Expect(remediationData.RetryCount).To(Equal(0))

		assertMachineV1beta1Condition(ctx, g, m1, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.RemediationInProgressV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")
		assertMachineCondition(ctx, g, m1, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationMachineDeletingReason, "Machine is deleting")

		err = env.Get(ctx, client.ObjectKey{Namespace: m1.Namespace, Name: m1.Name}, m1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(m1.ObjectMeta.DeletionTimestamp.IsZero()).To(BeFalse())

		removeFinalizer(g, m1)
		g.Expect(env.Cleanup(ctx, m1, m2, m3)).To(Succeed())
	})
	t.Run("Remediation deletes unhealthy machine failed to provision - 3 CP", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer(), withoutNodeRef())
		m2 := createMachine(ctx, g, ns.Name, "m2-healthy-", withHealthyEtcdMember())
		m3 := createMachine(ctx, g, ns.Name, "m3-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To(int32(3)),
					Version:  "v1.19.1",
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Initialization: controlplanev1.KubeadmControlPlaneInitializationStatus{
						ControlPlaneInitialized: utilptr.To(true),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeFalse()) // Remediation completed, requeue
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).To(HaveKey(controlplanev1.RemediationInProgressAnnotation))
		remediationData, err := RemediationDataFromAnnotation(controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation])
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(remediationData.Machine).To(Equal(m1.Name))
		g.Expect(remediationData.RetryCount).To(Equal(0))

		assertMachineV1beta1Condition(ctx, g, m1, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.RemediationInProgressV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")
		assertMachineCondition(ctx, g, m1, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationMachineDeletingReason, "Machine is deleting")

		err = env.Get(ctx, client.ObjectKey{Namespace: m1.Namespace, Name: m1.Name}, m1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(m1.ObjectMeta.DeletionTimestamp.IsZero()).To(BeFalse())

		removeFinalizer(g, m1)
		g.Expect(env.Cleanup(ctx, m1, m2, m3)).To(Succeed())
	})
	t.Run("Remediation deletes unhealthy machine - 4 CP (during 3 CP rolling upgrade)", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer())
		m2 := createMachine(ctx, g, ns.Name, "m2-healthy-", withHealthyEtcdMember())
		m3 := createMachine(ctx, g, ns.Name, "m3-healthy-", withHealthyEtcdMember())
		m4 := createMachine(ctx, g, ns.Name, "m4-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To[int32](4),
					Version:  "v1.19.1",
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Initialization: controlplanev1.KubeadmControlPlaneInitializationStatus{
						ControlPlaneInitialized: utilptr.To(true),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3, m4),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeFalse()) // Remediation completed, requeue
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).To(HaveKey(controlplanev1.RemediationInProgressAnnotation))
		remediationData, err := RemediationDataFromAnnotation(controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation])
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(remediationData.Machine).To(Equal(m1.Name))
		g.Expect(remediationData.RetryCount).To(Equal(0))

		assertMachineV1beta1Condition(ctx, g, m1, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.RemediationInProgressV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")
		assertMachineCondition(ctx, g, m1, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationMachineDeletingReason, "Machine is deleting")

		err = env.Get(ctx, client.ObjectKey{Namespace: m1.Namespace, Name: m1.Name}, m1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(m1.ObjectMeta.DeletionTimestamp.IsZero()).To(BeFalse())

		removeFinalizer(g, m1)
		g.Expect(env.Cleanup(ctx, m1, m2, m3, m4)).To(Succeed())
	})
	t.Run("Remediation deletes unhealthy machine failed to provision - 4 CP (during 3 CP rolling upgrade)", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer(), withoutNodeRef())
		m2 := createMachine(ctx, g, ns.Name, "m2-healthy-", withHealthyEtcdMember())
		m3 := createMachine(ctx, g, ns.Name, "m3-healthy-", withHealthyEtcdMember())
		m4 := createMachine(ctx, g, ns.Name, "m4-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To(int32(4)),
					Version:  "v1.19.1",
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Initialization: controlplanev1.KubeadmControlPlaneInitializationStatus{
						ControlPlaneInitialized: utilptr.To(true),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3, m4),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeFalse()) // Remediation completed, requeue
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).To(HaveKey(controlplanev1.RemediationInProgressAnnotation))
		remediationData, err := RemediationDataFromAnnotation(controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation])
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(remediationData.Machine).To(Equal(m1.Name))
		g.Expect(remediationData.RetryCount).To(Equal(0))

		assertMachineV1beta1Condition(ctx, g, m1, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.RemediationInProgressV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")
		assertMachineCondition(ctx, g, m1, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationMachineDeletingReason, "Machine is deleting")

		err = env.Get(ctx, client.ObjectKey{Namespace: m1.Namespace, Name: m1.Name}, m1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(m1.ObjectMeta.DeletionTimestamp.IsZero()).To(BeFalse())

		removeFinalizer(g, m1)
		g.Expect(env.Cleanup(ctx, m1, m2, m3, m4)).To(Succeed())
	})
	t.Run("Remediation fails gracefully if no healthy Control Planes are available to become etcd leader", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer())
		m2 := createMachine(ctx, g, ns.Name, "m2-healthy-", withMachineHealthCheckFailed(), withHealthyEtcdMember())
		m3 := createMachine(ctx, g, ns.Name, "m3-healthy-", withMachineHealthCheckFailed(), withHealthyEtcdMember())
		m4 := createMachine(ctx, g, ns.Name, "m4-healthy-", withMachineHealthCheckFailed(), withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To[int32](4),
					Version:  "v1.19.1",
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Initialization: controlplanev1.KubeadmControlPlaneInitializationStatus{
						ControlPlaneInitialized: utilptr.To(true),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3, m4),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		_, err = r.reconcileUnhealthyMachines(ctx, controlPlane)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).ToNot(HaveKey(controlplanev1.RemediationInProgressAnnotation))

		assertMachineV1beta1Condition(ctx, g, m1, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.RemediationFailedV1Beta1Reason, clusterv1.ConditionSeverityWarning,
			"A control plane machine needs remediation, but there is no healthy machine to forward etcd leadership to. Skipping remediation")
		assertMachineCondition(ctx, g, m1, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineCannotBeRemediatedReason,
			"KubeadmControlPlane can't remediate this Machine because there is no healthy Machine to forward etcd leadership to")

		removeFinalizer(g, m1)
		g.Expect(env.Cleanup(ctx, m1, m2, m3, m4)).To(Succeed())
	})
	t.Run("Subsequent remediation of the same machine increase retry count - 3 CP", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer())
		m2 := createMachine(ctx, g, ns.Name, "m2-healthy-", withHealthyEtcdMember())
		m3 := createMachine(ctx, g, ns.Name, "m3-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To[int32](1),
					Version:  "v1.19.1",
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Initialization: controlplanev1.KubeadmControlPlaneInitializationStatus{
						ControlPlaneInitialized: utilptr.To(false),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		// First reconcile, remediate machine m1 for the first time
		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}

		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeFalse()) // Remediation completed, requeue
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).To(HaveKey(controlplanev1.RemediationInProgressAnnotation))
		remediationData, err := RemediationDataFromAnnotation(controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation])
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(remediationData.Machine).To(Equal(m1.Name))
		g.Expect(remediationData.RetryCount).To(Equal(0))

		assertMachineV1beta1Condition(ctx, g, m1, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.RemediationInProgressV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")
		assertMachineCondition(ctx, g, m1, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationMachineDeletingReason, "Machine is deleting")

		err = env.Get(ctx, client.ObjectKey{Namespace: m1.Namespace, Name: m1.Name}, m1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(m1.ObjectMeta.DeletionTimestamp.IsZero()).To(BeFalse())

		removeFinalizer(g, m1)
		g.Expect(env.CleanupAndWait(ctx, m1)).To(Succeed())

		for i := 5; i < 6; i++ {
			// Simulate the creation of a replacement for m1.
			mi := createMachine(ctx, g, ns.Name, fmt.Sprintf("m%d-unhealthy-", i), withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer(), withRemediateForAnnotation(MustMarshalRemediationData(remediationData)))

			// Simulate KCP dropping RemediationInProgressAnnotation after creating the replacement machine.
			delete(controlPlane.KCP.Annotations, controlplanev1.RemediationInProgressAnnotation)
			controlPlane.Machines = collections.FromMachines(mi, m2, m3)
			controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

			// Reconcile unhealthy replacements for m1.
			r.managementCluster = &fakeManagementCluster{Workload: &fakeWorkloadCluster{}}

			ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

			g.Expect(ret.IsZero()).To(BeFalse()) // Remediation completed, requeue
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(controlPlane.KCP.Annotations).To(HaveKey(controlplanev1.RemediationInProgressAnnotation))
			remediationData, err := RemediationDataFromAnnotation(controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation])
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(remediationData.Machine).To(Equal(mi.Name))
			g.Expect(remediationData.RetryCount).To(Equal(i - 4))

			assertMachineV1beta1Condition(ctx, g, mi, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.RemediationInProgressV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")
			assertMachineCondition(ctx, g, mi, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationMachineDeletingReason, "Machine is deleting")

			err = env.Get(ctx, client.ObjectKey{Namespace: mi.Namespace, Name: mi.Name}, mi)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(mi.ObjectMeta.DeletionTimestamp.IsZero()).To(BeFalse())

			removeFinalizer(g, mi)
			g.Expect(env.CleanupAndWait(ctx, mi)).To(Succeed())
		}

		g.Expect(env.CleanupAndWait(ctx, m2, m3)).To(Succeed())
	})
}

func TestReconcileUnhealthyMachinesSequences(t *testing.T) {
	var removeFinalizer = func(g *WithT, m *clusterv1.Machine) {
		patchHelper, err := patch.NewHelper(m, env.GetClient())
		g.Expect(err).ToNot(HaveOccurred())
		m.Finalizers = nil
		g.Expect(patchHelper.Patch(ctx, m)).To(Succeed())
	}

	t.Run("Remediates the first CP machine having problems to come up", func(t *testing.T) {
		g := NewWithT(t)

		ns, err := env.CreateNamespace(ctx, "ns1")
		g.Expect(err).ToNot(HaveOccurred())
		defer func() {
			g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
		}()

		// Control plane not initialized yet, First CP is unhealthy and gets remediated:

		m1 := createMachine(ctx, g, ns.Name, "m1-unhealthy-", withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To[int32](3),
					Version:  "v1.19.1",
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Initialization: controlplanev1.KubeadmControlPlaneInitializationStatus{
						ControlPlaneInitialized: utilptr.To(false),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}

		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeFalse()) // Remediation completed, requeue
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).To(HaveKey(controlplanev1.RemediationInProgressAnnotation))
		remediationData, err := RemediationDataFromAnnotation(controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation])
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(remediationData.Machine).To(Equal(m1.Name))
		g.Expect(remediationData.RetryCount).To(Equal(0))

		assertMachineV1beta1Condition(ctx, g, m1, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.RemediationInProgressV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")
		assertMachineCondition(ctx, g, m1, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationMachineDeletingReason, "Machine is deleting")

		err = env.Get(ctx, client.ObjectKey{Namespace: m1.Namespace, Name: m1.Name}, m1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(m1.ObjectMeta.DeletionTimestamp.IsZero()).To(BeFalse())

		removeFinalizer(g, m1)
		g.Expect(env.Cleanup(ctx, m1)).To(Succeed())

		// Fake scaling up, which creates a remediation machine, fast forwards to when also the replacement machine is marked unhealthy.
		// NOTE: scale up also resets remediation in progress and remediation counts.

		m2 := createMachine(ctx, g, ns.Name, "m2-unhealthy-", withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer(), withRemediateForAnnotation(MustMarshalRemediationData(remediationData)))
		delete(controlPlane.KCP.Annotations, controlplanev1.RemediationInProgressAnnotation)

		// Control plane not initialized yet, Second CP is unhealthy and gets remediated (retry 2)

		controlPlane.Machines = collections.FromMachines(m2)
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)
		r.managementCluster = &fakeManagementCluster{Workload: &fakeWorkloadCluster{}}

		ret, err = r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeFalse()) // Remediation completed, requeue
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).To(HaveKey(controlplanev1.RemediationInProgressAnnotation))
		remediationData, err = RemediationDataFromAnnotation(controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation])
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(remediationData.Machine).To(Equal(m2.Name))
		g.Expect(remediationData.RetryCount).To(Equal(1))

		assertMachineV1beta1Condition(ctx, g, m2, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.RemediationInProgressV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")
		assertMachineCondition(ctx, g, m2, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationMachineDeletingReason, "Machine is deleting")

		err = env.Get(ctx, client.ObjectKey{Namespace: m2.Namespace, Name: m2.Name}, m1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(m2.ObjectMeta.DeletionTimestamp.IsZero()).To(BeFalse())

		removeFinalizer(g, m2)
		g.Expect(env.Cleanup(ctx, m2)).To(Succeed())

		// Fake scaling up, which creates a remediation machine, which is healthy.
		// NOTE: scale up also resets remediation in progress and remediation counts.

		m3 := createMachine(ctx, g, ns.Name, "m3-healthy-", withHealthyEtcdMember(), withRemediateForAnnotation(MustMarshalRemediationData(remediationData)))
		delete(controlPlane.KCP.Annotations, controlplanev1.RemediationInProgressAnnotation)

		g.Expect(env.Cleanup(ctx, m3)).To(Succeed())
	})

	t.Run("Remediates the second CP machine having problems to come up", func(t *testing.T) {
		g := NewWithT(t)

		ns, err := env.CreateNamespace(ctx, "ns1")
		g.Expect(err).ToNot(HaveOccurred())
		defer func() {
			g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
		}()

		// Control plane initialized yet, First CP healthy, second CP is unhealthy and gets remediated:

		m1 := createMachine(ctx, g, ns.Name, "m1-healthy-", withHealthyEtcdMember())
		m2 := createMachine(ctx, g, ns.Name, "m2-unhealthy-", withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To[int32](3),
					Version:  "v1.19.1",
					Rollout: controlplanev1.KubeadmControlPlaneRolloutSpec{
						Strategy: controlplanev1.KubeadmControlPlaneRolloutStrategy{
							RollingUpdate: controlplanev1.KubeadmControlPlaneRolloutStrategyRollingUpdate{
								MaxSurge: &intstr.IntOrString{
									IntVal: 1,
								},
							},
						},
					},
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Initialization: controlplanev1.KubeadmControlPlaneInitializationStatus{
						ControlPlaneInitialized: utilptr.To(true),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeFalse()) // Remediation completed, requeue
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).To(HaveKey(controlplanev1.RemediationInProgressAnnotation))
		remediationData, err := RemediationDataFromAnnotation(controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation])
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(remediationData.Machine).To(Equal(m2.Name))
		g.Expect(remediationData.RetryCount).To(Equal(0))

		assertMachineV1beta1Condition(ctx, g, m2, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.RemediationInProgressV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")
		assertMachineCondition(ctx, g, m2, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationMachineDeletingReason, "Machine is deleting")

		err = env.Get(ctx, client.ObjectKey{Namespace: m2.Namespace, Name: m2.Name}, m2)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(m2.ObjectMeta.DeletionTimestamp.IsZero()).To(BeFalse())

		removeFinalizer(g, m2)
		g.Expect(env.Cleanup(ctx, m2)).To(Succeed())

		// Fake scaling up, which creates a remediation machine, fast forwards to when also the replacement machine is marked unhealthy.
		// NOTE: scale up also resets remediation in progress and remediation counts.

		m3 := createMachine(ctx, g, ns.Name, "m3-unhealthy-", withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer(), withRemediateForAnnotation(MustMarshalRemediationData(remediationData)))
		delete(controlPlane.KCP.Annotations, controlplanev1.RemediationInProgressAnnotation)

		// Control plane not initialized yet, Second CP is unhealthy and gets remediated (retry 2)

		controlPlane.Machines = collections.FromMachines(m1, m3)
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)
		r.managementCluster = &fakeManagementCluster{Workload: &fakeWorkloadCluster{}}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		ret, err = r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeFalse()) // Remediation completed, requeue
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).To(HaveKey(controlplanev1.RemediationInProgressAnnotation))
		remediationData, err = RemediationDataFromAnnotation(controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation])
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(remediationData.Machine).To(Equal(m3.Name))
		g.Expect(remediationData.RetryCount).To(Equal(1))

		assertMachineV1beta1Condition(ctx, g, m3, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.RemediationInProgressV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")
		assertMachineCondition(ctx, g, m3, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationMachineDeletingReason, "Machine is deleting")

		err = env.Get(ctx, client.ObjectKey{Namespace: m3.Namespace, Name: m3.Name}, m3)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(m3.ObjectMeta.DeletionTimestamp.IsZero()).To(BeFalse())

		removeFinalizer(g, m3)
		g.Expect(env.Cleanup(ctx, m3)).To(Succeed())

		// Fake scaling up, which creates a remediation machine, which is healthy.
		// NOTE: scale up also resets remediation in progress and remediation counts.

		m4 := createMachine(ctx, g, ns.Name, "m4-healthy-", withHealthyEtcdMember(), withRemediateForAnnotation(MustMarshalRemediationData(remediationData)))
		delete(controlPlane.KCP.Annotations, controlplanev1.RemediationInProgressAnnotation)

		g.Expect(env.Cleanup(ctx, m1, m4)).To(Succeed())
	})

	t.Run("Remediates only one CP machine in case of multiple failures", func(t *testing.T) {
		g := NewWithT(t)

		ns, err := env.CreateNamespace(ctx, "ns1")
		g.Expect(err).ToNot(HaveOccurred())
		defer func() {
			g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
		}()

		// Control plane initialized yet, First CP healthy, second and third CP are unhealthy. second gets remediated:

		m1 := createMachine(ctx, g, ns.Name, "m1-healthy-", withHealthyEtcdMember())
		m2 := createMachine(ctx, g, ns.Name, "m2-unhealthy-", withHealthyEtcdMember(), withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer())
		m3 := createMachine(ctx, g, ns.Name, "m3-unhealthy-", withHealthyEtcdMember(), withMachineHealthCheckFailed(), withWaitBeforeDeleteFinalizer())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: utilptr.To[int32](3),
					Version:  "v1.19.1",
					Rollout: controlplanev1.KubeadmControlPlaneRolloutSpec{
						Strategy: controlplanev1.KubeadmControlPlaneRolloutStrategy{
							RollingUpdate: controlplanev1.KubeadmControlPlaneRolloutStrategyRollingUpdate{
								MaxSurge: &intstr.IntOrString{
									IntVal: 1,
								},
							},
						},
					},
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Initialization: controlplanev1.KubeadmControlPlaneInitializationStatus{
						ControlPlaneInitialized: utilptr.To(true),
					},
				},
			},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		ret, err := r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeFalse()) // Remediation completed, requeue
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controlPlane.KCP.Annotations).To(HaveKey(controlplanev1.RemediationInProgressAnnotation))
		remediationData, err := RemediationDataFromAnnotation(controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation])
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(remediationData.Machine).To(Equal(m2.Name))
		g.Expect(remediationData.RetryCount).To(Equal(0))

		assertMachineV1beta1Condition(ctx, g, m2, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.RemediationInProgressV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")
		assertMachineV1beta1Condition(ctx, g, m3, clusterv1.MachineOwnerRemediatedV1Beta1Condition, corev1.ConditionFalse, clusterv1.WaitingForRemediationV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")

		assertMachineCondition(ctx, g, m2, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, controlplanev1.KubeadmControlPlaneMachineRemediationMachineDeletingReason, "Machine is deleting")
		assertMachineCondition(ctx, g, m3, clusterv1.MachineOwnerRemediatedCondition, metav1.ConditionFalse, clusterv1.MachineOwnerRemediatedWaitingForRemediationReason, "Waiting for remediation")

		err = env.Get(ctx, client.ObjectKey{Namespace: m2.Namespace, Name: m2.Name}, m2)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(m2.ObjectMeta.DeletionTimestamp.IsZero()).To(BeFalse())

		removeFinalizer(g, m2)
		g.Expect(env.Cleanup(ctx, m2)).To(Succeed())

		// Check next reconcile does not further remediate

		controlPlane.Machines = collections.FromMachines(m1, m3)
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)
		r.managementCluster = &fakeManagementCluster{Workload: &fakeWorkloadCluster{}}

		ret, err = r.reconcileUnhealthyMachines(ctx, controlPlane)

		g.Expect(ret.IsZero()).To(BeTrue()) // Remediation skipped
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(env.Cleanup(ctx, m1)).To(Succeed())
	})
}

func TestCanSafelyRemoveEtcdMember(t *testing.T) {
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "ns1")
	g.Expect(err).ToNot(HaveOccurred())
	defer func() {
		g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
	}()

	t.Run("Can't safely remediate 1 machine CP", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-mhc-unhealthy-", withMachineHealthCheckFailed())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{Spec: controlplanev1.KubeadmControlPlaneSpec{
				Replicas: utilptr.To[int32](1),
			}},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		ret, err := r.canSafelyRemoveEtcdMember(ctx, controlPlane, m1)
		g.Expect(ret).To(BeFalse())
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(env.Cleanup(ctx, m1)).To(Succeed())
	})

	t.Run("Can safely remediate 2 machine CP without additional etcd member failures", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-mhc-unhealthy-", withMachineHealthCheckFailed())
		m2 := createMachine(ctx, g, ns.Name, "m2-etcd-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{Spec: controlplanev1.KubeadmControlPlaneSpec{
				Replicas: utilptr.To[int32](3),
			}},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		ret, err := r.canSafelyRemoveEtcdMember(ctx, controlPlane, m1)
		g.Expect(ret).To(BeTrue())
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(env.Cleanup(ctx, m1, m2)).To(Succeed())
	})
	t.Run("Can safely remediate 2 machines CP when the etcd member being remediated is missing", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-mhc-unhealthy-", withMachineHealthCheckFailed())
		m2 := createMachine(ctx, g, ns.Name, "m2-etcd-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{Spec: controlplanev1.KubeadmControlPlaneSpec{
				Replicas: utilptr.To[int32](3),
			}},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2),
		}

		members := make([]*etcd.Member, 0, len(controlPlane.Machines)-1)
		for _, n := range nodes(controlPlane.Machines) {
			if !strings.Contains(n, "m1-mhc-unhealthy-") {
				members = append(members, &etcd.Member{Name: n})
			}
		}
		controlPlane.EtcdMembers = members

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		ret, err := r.canSafelyRemoveEtcdMember(ctx, controlPlane, m1)
		g.Expect(ret).To(BeTrue())
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(env.Cleanup(ctx, m1, m2)).To(Succeed())
	})
	t.Run("Can't safely remediate 2 machines CP with one additional etcd member failure", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-mhc-unhealthy-", withMachineHealthCheckFailed())
		m2 := createMachine(ctx, g, ns.Name, "m2-etcd-unhealthy-", withUnhealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{Spec: controlplanev1.KubeadmControlPlaneSpec{
				Replicas: utilptr.To[int32](3),
			}},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		ret, err := r.canSafelyRemoveEtcdMember(ctx, controlPlane, m1)
		g.Expect(ret).To(BeFalse())
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(env.Cleanup(ctx, m1, m2)).To(Succeed())
	})
	t.Run("Can safely remediate 3 machines CP without additional etcd member failures", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-mhc-unhealthy-", withMachineHealthCheckFailed())
		m2 := createMachine(ctx, g, ns.Name, "m2-etcd-healthy-", withHealthyEtcdMember())
		m3 := createMachine(ctx, g, ns.Name, "m3-etcd-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{Spec: controlplanev1.KubeadmControlPlaneSpec{
				Replicas: utilptr.To[int32](3),
			}},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		ret, err := r.canSafelyRemoveEtcdMember(ctx, controlPlane, m1)
		g.Expect(ret).To(BeTrue())
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(env.Cleanup(ctx, m1, m2, m3)).To(Succeed())
	})
	t.Run("Can safely remediate 3 machines CP when the etcd member being remediated is missing", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-mhc-unhealthy-", withMachineHealthCheckFailed())
		m2 := createMachine(ctx, g, ns.Name, "m2-etcd-healthy-", withHealthyEtcdMember())
		m3 := createMachine(ctx, g, ns.Name, "m3-etcd-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{Spec: controlplanev1.KubeadmControlPlaneSpec{
				Replicas: utilptr.To[int32](3),
			}},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3),
		}

		members := make([]*etcd.Member, 0, len(controlPlane.Machines)-1)
		for _, n := range nodes(controlPlane.Machines) {
			if !strings.Contains(n, "m1-mhc-unhealthy-") {
				members = append(members, &etcd.Member{Name: n})
			}
		}
		controlPlane.EtcdMembers = members

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		ret, err := r.canSafelyRemoveEtcdMember(ctx, controlPlane, m1)
		g.Expect(ret).To(BeTrue())
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(env.Cleanup(ctx, m1, m2, m3)).To(Succeed())
	})
	t.Run("Can't safely remediate 3 machines CP with one additional etcd member failure", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-mhc-unhealthy-", withMachineHealthCheckFailed())
		m2 := createMachine(ctx, g, ns.Name, "m2-etcd-unhealthy-", withUnhealthyEtcdMember())
		m3 := createMachine(ctx, g, ns.Name, "m3-etcd-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{Spec: controlplanev1.KubeadmControlPlaneSpec{
				Replicas: utilptr.To[int32](3),
			}},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		ret, err := r.canSafelyRemoveEtcdMember(ctx, controlPlane, m1)
		g.Expect(ret).To(BeFalse())
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(env.Cleanup(ctx, m1, m2, m3)).To(Succeed())
	})
	t.Run("Can safely remediate 5 machines CP less than 2 additional etcd member failures", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-mhc-unhealthy-", withMachineHealthCheckFailed())
		m2 := createMachine(ctx, g, ns.Name, "m2-etcd-unhealthy-", withUnhealthyEtcdMember())
		m3 := createMachine(ctx, g, ns.Name, "m3-etcd-healthy-", withHealthyEtcdMember())
		m4 := createMachine(ctx, g, ns.Name, "m4-etcd-healthy-", withHealthyEtcdMember())
		m5 := createMachine(ctx, g, ns.Name, "m5-etcd-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{Spec: controlplanev1.KubeadmControlPlaneSpec{
				Replicas: utilptr.To[int32](5),
			}},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3, m4, m5),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		ret, err := r.canSafelyRemoveEtcdMember(ctx, controlPlane, m1)
		g.Expect(ret).To(BeTrue())
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(env.Cleanup(ctx, m1, m2, m3, m4, m5)).To(Succeed())
	})
	t.Run("Can't safely remediate 5 machines CP with 2 additional etcd member failures", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-mhc-unhealthy-", withMachineHealthCheckFailed())
		m2 := createMachine(ctx, g, ns.Name, "m2-etcd-unhealthy-", withUnhealthyEtcdMember())
		m3 := createMachine(ctx, g, ns.Name, "m3-etcd-unhealthy-", withUnhealthyEtcdMember())
		m4 := createMachine(ctx, g, ns.Name, "m4-etcd-healthy-", withHealthyEtcdMember())
		m5 := createMachine(ctx, g, ns.Name, "m5-etcd-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{Spec: controlplanev1.KubeadmControlPlaneSpec{
				Replicas: utilptr.To[int32](7),
			}},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3, m4, m5),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		ret, err := r.canSafelyRemoveEtcdMember(ctx, controlPlane, m1)
		g.Expect(ret).To(BeFalse())
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(env.Cleanup(ctx, m1, m2, m3, m4, m5)).To(Succeed())
	})
	t.Run("Can safely remediate 7 machines CP with less than 3 additional etcd member failures", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-mhc-unhealthy-", withMachineHealthCheckFailed())
		m2 := createMachine(ctx, g, ns.Name, "m2-etcd-unhealthy-", withUnhealthyEtcdMember())
		m3 := createMachine(ctx, g, ns.Name, "m3-etcd-unhealthy-", withUnhealthyEtcdMember())
		m4 := createMachine(ctx, g, ns.Name, "m4-etcd-healthy-", withHealthyEtcdMember())
		m5 := createMachine(ctx, g, ns.Name, "m5-etcd-healthy-", withHealthyEtcdMember())
		m6 := createMachine(ctx, g, ns.Name, "m6-etcd-healthy-", withHealthyEtcdMember())
		m7 := createMachine(ctx, g, ns.Name, "m7-etcd-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{Spec: controlplanev1.KubeadmControlPlaneSpec{
				Replicas: utilptr.To[int32](7),
			}},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3, m4, m5, m6, m7),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		ret, err := r.canSafelyRemoveEtcdMember(ctx, controlPlane, m1)
		g.Expect(ret).To(BeTrue())
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(env.Cleanup(ctx, m1, m2, m3, m4, m5, m6, m7)).To(Succeed())
	})
	t.Run("Can't safely remediate 7 machines CP with 3 additional etcd member failures", func(t *testing.T) {
		g := NewWithT(t)

		m1 := createMachine(ctx, g, ns.Name, "m1-mhc-unhealthy-", withMachineHealthCheckFailed())
		m2 := createMachine(ctx, g, ns.Name, "m2-etcd-unhealthy-", withUnhealthyEtcdMember())
		m3 := createMachine(ctx, g, ns.Name, "m3-etcd-unhealthy-", withUnhealthyEtcdMember())
		m4 := createMachine(ctx, g, ns.Name, "m4-etcd-unhealthy-", withUnhealthyEtcdMember())
		m5 := createMachine(ctx, g, ns.Name, "m5-etcd-healthy-", withHealthyEtcdMember())
		m6 := createMachine(ctx, g, ns.Name, "m6-etcd-healthy-", withHealthyEtcdMember())
		m7 := createMachine(ctx, g, ns.Name, "m7-etcd-healthy-", withHealthyEtcdMember())

		controlPlane := &internal.ControlPlane{
			KCP: &controlplanev1.KubeadmControlPlane{Spec: controlplanev1.KubeadmControlPlaneSpec{
				Replicas: utilptr.To[int32](5),
			}},
			Cluster:  &clusterv1.Cluster{},
			Machines: collections.FromMachines(m1, m2, m3, m4, m5, m6, m7),
		}
		controlPlane.EtcdMembers = etcdMembers(controlPlane.Machines)

		r := &KubeadmControlPlaneReconciler{
			Client:            env.GetClient(),
			recorder:          record.NewFakeRecorder(32),
			managementCluster: &fakeManagementCluster{Workload: &fakeWorkloadCluster{}},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		ret, err := r.canSafelyRemoveEtcdMember(ctx, controlPlane, m1)
		g.Expect(ret).To(BeFalse())
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(env.Cleanup(ctx, m1, m2, m3, m4, m5, m6, m7)).To(Succeed())
	})
}

func nodes(machines collections.Machines) []string {
	nodes := make([]string, 0, machines.Len())
	for _, m := range machines {
		if m.Status.NodeRef.IsDefined() {
			nodes = append(nodes, m.Status.NodeRef.Name)
		}
	}
	return nodes
}

func etcdMembers(machines collections.Machines) []*etcd.Member {
	members := make([]*etcd.Member, 0, machines.Len())
	for _, m := range machines {
		if m.Status.NodeRef.IsDefined() {
			members = append(members, &etcd.Member{Name: m.Status.NodeRef.Name})
		}
	}
	return members
}

type machineOption func(*clusterv1.Machine)

func withMachineHealthCheckFailed() machineOption {
	return func(machine *clusterv1.Machine) {
		v1beta1conditions.MarkFalse(machine, clusterv1.MachineHealthCheckSucceededV1Beta1Condition, clusterv1.MachineHasFailureV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")
		v1beta1conditions.MarkFalse(machine, clusterv1.MachineOwnerRemediatedV1Beta1Condition, clusterv1.WaitingForRemediationV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")

		conditions.Set(machine, metav1.Condition{
			Type:   clusterv1.MachineHealthCheckSucceededCondition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.MachineHealthCheckNodeDeletedReason,
		})
		conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineOwnerRemediatedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationReason,
			Message: "Waiting for remediation",
		})
	}
}

func withStuckRemediation() machineOption {
	return func(machine *clusterv1.Machine) {
		v1beta1conditions.MarkTrue(machine, clusterv1.MachineHealthCheckSucceededV1Beta1Condition)
		v1beta1conditions.MarkFalse(machine, clusterv1.MachineOwnerRemediatedV1Beta1Condition, clusterv1.WaitingForRemediationV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")

		conditions.Set(machine, metav1.Condition{
			Type:   clusterv1.MachineHealthCheckSucceededCondition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachineHealthCheckSucceededReason,
		})
		conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineOwnerRemediatedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationReason,
			Message: "Waiting for remediation",
		})
	}
}

func withHealthyEtcdMember() machineOption {
	return func(machine *clusterv1.Machine) {
		conditions.Set(machine, metav1.Condition{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyReason})
	}
}

func withUnhealthyEtcdMember() machineOption {
	return func(machine *clusterv1.Machine) {
		conditions.Set(machine, metav1.Condition{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionFalse, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberNotHealthyReason})
	}
}

func withHealthyAPIServerPod() machineOption {
	return func(machine *clusterv1.Machine) {
		conditions.Set(machine, metav1.Condition{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason})
	}
}

func withUnhealthyAPIServerPod() machineOption {
	return func(machine *clusterv1.Machine) {
		conditions.Set(machine, metav1.Condition{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionFalse, Reason: controlplanev1.KubeadmControlPlaneMachinePodFailedReason})
	}
}

func withNodeRef(ref string) machineOption {
	return func(machine *clusterv1.Machine) {
		machine.Status.NodeRef = clusterv1.MachineNodeReference{
			Name: ref,
		}
	}
}

func withoutNodeRef() machineOption {
	return func(machine *clusterv1.Machine) {
		machine.Status.NodeRef = clusterv1.MachineNodeReference{}
	}
}

func withRemediateForAnnotation(remediatedFor string) machineOption {
	return func(machine *clusterv1.Machine) {
		if machine.Annotations == nil {
			machine.Annotations = map[string]string{}
		}
		machine.Annotations[controlplanev1.RemediationForAnnotation] = remediatedFor
	}
}

func withWaitBeforeDeleteFinalizer() machineOption {
	return func(machine *clusterv1.Machine) {
		machine.Finalizers = []string{"wait-before-delete"}
	}
}

func createMachine(ctx context.Context, g *WithT, namespace, name string, options ...machineOption) *clusterv1.Machine {
	m := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    namespace,
			GenerateName: name,
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: "cluster",
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: builder.InfrastructureGroupVersion.Group,
				Kind:     builder.GenericInfrastructureMachineKind,
				Name:     "inframachine",
			},
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: utilptr.To("secret"),
			},
		},
	}
	g.Expect(env.CreateAndWait(ctx, m)).To(Succeed())

	patchHelper, err := patch.NewHelper(m, env.GetClient())
	g.Expect(err).ToNot(HaveOccurred())

	for _, opt := range append([]machineOption{withNodeRef(fmt.Sprintf("node-%s", m.Name))}, options...) {
		opt(m)
	}

	g.Expect(patchHelper.Patch(ctx, m)).To(Succeed())
	return m
}

func getDeletingMachine(namespace, name string, options ...machineOption) *clusterv1.Machine {
	deletionTime := metav1.Now()
	m := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         namespace,
			Name:              name,
			DeletionTimestamp: &deletionTime,
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: "cluster",
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: utilptr.To("secret"),
			},
		},
	}

	for _, opt := range append([]machineOption{withNodeRef(fmt.Sprintf("node-%s", m.Name))}, options...) {
		opt(m)
	}
	return m
}

func assertMachineV1beta1Condition(ctx context.Context, g *WithT, m *clusterv1.Machine, t clusterv1.ConditionType, status corev1.ConditionStatus, reason string, severity clusterv1.ConditionSeverity, message string) {
	g.Eventually(func() error {
		if err := env.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: m.Name}, m); err != nil {
			return err
		}
		c := v1beta1conditions.Get(m, t)
		if c == nil {
			return errors.Errorf("condition %q was nil", t)
		}
		if c.Status != status {
			return errors.Errorf("condition %q status %q did not match %q", t, c.Status, status)
		}
		if c.Reason != reason {
			return errors.Errorf("condition %q reason %q did not match %q", t, c.Reason, reason)
		}
		if c.Severity != severity {
			return errors.Errorf("condition %q severity %q did not match %q", t, c.Status, status)
		}
		if c.Message != message {
			return errors.Errorf("condition %q message %q did not match %q", t, c.Message, message)
		}
		return nil
	}, 10*time.Second).Should(Succeed())
}

func assertMachineCondition(ctx context.Context, g *WithT, m *clusterv1.Machine, t string, status metav1.ConditionStatus, reason string, message string) {
	g.Eventually(func() error {
		if err := env.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: m.Name}, m); err != nil {
			return err
		}
		c := conditions.Get(m, t)
		if c == nil {
			return errors.Errorf("condition %q was nil", t)
		}
		if c.Status != status {
			return errors.Errorf("condition %q status %q did not match %q", t, c.Status, status)
		}
		if c.Reason != reason {
			return errors.Errorf("condition %q reason %q did not match %q", t, c.Reason, reason)
		}
		if c.Message != message {
			return errors.Errorf("condition %q message %q did not match %q", t, c.Message, message)
		}
		return nil
	}, 10*time.Second).Should(Succeed())
}

func MustMarshalRemediationData(r *RemediationData) string {
	s, err := r.Marshal()
	if err != nil {
		panic("failed to marshal remediation data")
	}
	return s
}
