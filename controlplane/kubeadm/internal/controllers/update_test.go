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
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/desiredstate"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/collections"
	capicontrollerutil "sigs.k8s.io/cluster-api/util/controller"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

const (
	UpdatedVersion string = "v1.17.4"
	Host           string = "nodomain.example.com"
)

func TestKubeadmControlPlaneReconciler_RolloutStrategy_ScaleUp(t *testing.T) {
	setup := func(t *testing.T, g *WithT) *corev1.Namespace {
		t.Helper()

		t.Log("Creating the namespace")
		ns, err := env.CreateNamespace(ctx, "test-kcp-reconciler-rollout-scaleup")
		g.Expect(err).ToNot(HaveOccurred())

		return ns
	}

	teardown := func(t *testing.T, g *WithT, ns *corev1.Namespace) {
		t.Helper()

		t.Log("Deleting the namespace")
		g.Expect(env.Delete(ctx, ns)).To(Succeed())
	}

	g := NewWithT(t)
	namespace := setup(t, g)
	defer teardown(t, g, namespace)

	timeout := 30 * time.Second

	cluster, kcp, genericInfrastructureMachineTemplate := createClusterWithControlPlane(namespace.Name)
	g.Expect(env.CreateAndWait(ctx, genericInfrastructureMachineTemplate, client.FieldOwner("manager"))).To(Succeed())
	cluster.UID = types.UID(util.RandomString(10))
	cluster.Spec.ControlPlaneEndpoint.Host = Host
	cluster.Spec.ControlPlaneEndpoint.Port = 6443
	cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
	kcp.UID = types.UID(util.RandomString(10))
	kcp.Spec.Replicas = ptr.To[int32](1)
	setKCPHealthy(kcp)

	fc := capicontrollerutil.NewFakeController()

	r := &KubeadmControlPlaneReconciler{
		Client:              env,
		SecretCachingClient: secretCachingClient,
		controller:          fc,
		recorder:            record.NewFakeRecorder(32),
		managementCluster: &fakeManagementCluster{
			Management: &internal.Management{Client: env},
			Workload: &fakeWorkloadCluster{
				Status: internal.ClusterStatus{Nodes: 1},
			},
		},
		managementClusterUncached: &fakeManagementCluster{
			Management: &internal.Management{Client: env},
			Workload: &fakeWorkloadCluster{
				Status: internal.ClusterStatus{Nodes: 1},
			},
		},
		ssaCache: ssa.NewCache("test-controller"),
	}
	controlPlane := &internal.ControlPlane{
		KCP:      kcp,
		Cluster:  cluster,
		Machines: nil,
	}
	controlPlane.InjectTestManagementCluster(r.managementCluster)

	result, err := r.initializeControlPlane(ctx, controlPlane)
	g.Expect(result.RequeueAfter).To(Equal(time.Duration(0)))
	g.Expect(err).ToNot(HaveOccurred())

	// initial setup
	initialMachine := &clusterv1.MachineList{}
	g.Eventually(func(g Gomega) {
		// Nb. This Eventually block also forces the cache to update so that subsequent
		// reconcile and updateControlPlane calls use the updated cache and avoids flakiness in the test.
		g.Expect(env.List(ctx, initialMachine, client.InNamespace(cluster.Namespace))).To(Succeed())
		g.Expect(initialMachine.Items).To(HaveLen(1))
	}, timeout).Should(Succeed())
	for i := range initialMachine.Items {
		setMachineHealthy(&initialMachine.Items[i])
	}

	// change the KCP spec so the machine becomes outdated
	kcp.Spec.Version = UpdatedVersion

	// run upgrade the first time, expect we scale up
	needingUpgrade := collections.FromMachineList(initialMachine)
	controlPlane.Machines = needingUpgrade
	machinesUpToDateResults := map[string]internal.UpToDateResult{}
	for _, m := range needingUpgrade {
		machinesUpToDateResults[m.Name] = internal.UpToDateResult{EligibleForInPlaceUpdate: false}
	}
	result, err = r.updateControlPlane(ctx, controlPlane, needingUpgrade, machinesUpToDateResults)
	g.Expect(result.IsZero()).To(BeTrue())
	g.Expect(err).ToNot(HaveOccurred())
	bothMachines := &clusterv1.MachineList{}
	g.Eventually(func(g Gomega) {
		g.Expect(env.List(ctx, bothMachines, client.InNamespace(cluster.Namespace))).To(Succeed())
		g.Expect(bothMachines.Items).To(HaveLen(2))
	}, timeout).Should(Succeed())

	// run upgrade a second time, simulate that the node has not appeared yet but the machine exists

	// Unhealthy control plane will be detected during reconcile loop and upgrade will never be called.
	controlPlane = &internal.ControlPlane{
		KCP:      kcp,
		Cluster:  cluster,
		Machines: collections.FromMachineList(bothMachines),
	}
	controlPlane.InjectTestManagementCluster(r.managementCluster)

	result, err = r.reconcile(context.Background(), controlPlane)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(BeComparableTo(ctrl.Result{RequeueAfter: preflightFailedRequeueAfter}))
	g.Expect(fc.Deferrals).To(HaveKeyWithValue(
		reconcile.Request{NamespacedName: client.ObjectKeyFromObject(kcp)},
		BeTemporally("~", time.Now().Add(5*time.Second), 1*time.Second)),
	)
	g.Eventually(func(g Gomega) {
		g.Expect(env.List(context.Background(), bothMachines, client.InNamespace(cluster.Namespace))).To(Succeed())
		g.Expect(bothMachines.Items).To(HaveLen(2))
	}, timeout).Should(Succeed())

	// manually increase number of nodes, make control plane healthy again
	r.managementCluster.(*fakeManagementCluster).Workload.Status.Nodes++
	for i := range bothMachines.Items {
		setMachineHealthy(&bothMachines.Items[i])
	}
	controlPlane.Machines = collections.FromMachineList(bothMachines)

	machinesRequireUpgrade := collections.Machines{}
	for i := range bothMachines.Items {
		if bothMachines.Items[i].Spec.Version != "" && bothMachines.Items[i].Spec.Version != UpdatedVersion {
			machinesRequireUpgrade[bothMachines.Items[i].Name] = &bothMachines.Items[i]
		}
	}
	machinesUpToDateResults = map[string]internal.UpToDateResult{}
	for _, m := range machinesRequireUpgrade {
		machinesUpToDateResults[m.Name] = internal.UpToDateResult{EligibleForInPlaceUpdate: false}
	}

	// run upgrade the second time, expect we scale down
	result, err = r.updateControlPlane(ctx, controlPlane, machinesRequireUpgrade, machinesUpToDateResults)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue())
	finalMachine := &clusterv1.MachineList{}
	g.Eventually(func(g Gomega) {
		g.Expect(env.List(ctx, finalMachine, client.InNamespace(cluster.Namespace))).To(Succeed())
		g.Expect(finalMachine.Items).To(HaveLen(1))
		// assert that the deleted machine is the initial machine
		g.Expect(finalMachine.Items[0].Name).ToNot(Equal(initialMachine.Items[0].Name))
	}, timeout).Should(Succeed())
}

func TestKubeadmControlPlaneReconciler_RolloutStrategy_ScaleDown(t *testing.T) {
	version := "v1.17.3"
	g := NewWithT(t)

	cluster, kcp, tmpl := createClusterWithControlPlane(metav1.NamespaceDefault)
	cluster.Spec.ControlPlaneEndpoint.Host = "nodomain.example.com1"
	cluster.Spec.ControlPlaneEndpoint.Port = 6443
	kcp.Spec.Replicas = ptr.To[int32](3)
	kcp.Spec.Rollout.Strategy.RollingUpdate.MaxSurge.IntVal = 0
	setKCPHealthy(kcp)

	fmc := &fakeManagementCluster{
		Machines: collections.Machines{},
		Workload: &fakeWorkloadCluster{
			Status: internal.ClusterStatus{Nodes: 3},
		},
	}
	objs := []client.Object{builder.GenericInfrastructureMachineTemplateCRD, cluster.DeepCopy(), kcp.DeepCopy(), tmpl.DeepCopy()}
	for i := range 3 {
		name := fmt.Sprintf("test-%d", i)
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cluster.Namespace,
				Name:      name,
				Labels:    desiredstate.ControlPlaneMachineLabels(kcp, cluster.Name),
			},
			Spec: clusterv1.MachineSpec{
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: bootstrapv1.GroupVersion.Group,
						Kind:     "KubeadmConfig",
						Name:     name,
					},
				},
				Version: version,
			},
		}
		cfg := &bootstrapv1.KubeadmConfig{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: cluster.Namespace,
				Name:      name,
			},
		}
		objs = append(objs, m, cfg)
		fmc.Machines.Insert(m)
	}
	fakeClient := newFakeClient(objs...)
	fmc.Reader = fakeClient
	r := &KubeadmControlPlaneReconciler{
		Client:                    fakeClient,
		SecretCachingClient:       fakeClient,
		managementCluster:         fmc,
		managementClusterUncached: fmc,
	}

	controlPlane := &internal.ControlPlane{
		KCP:      kcp,
		Cluster:  cluster,
		Machines: nil,
	}
	controlPlane.InjectTestManagementCluster(r.managementCluster)

	result, err := r.reconcile(ctx, controlPlane)
	g.Expect(result).To(BeComparableTo(ctrl.Result{}))
	g.Expect(err).ToNot(HaveOccurred())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(machineList.Items).To(HaveLen(3))
	for i := range machineList.Items {
		setMachineHealthy(&machineList.Items[i])
	}

	// change the KCP spec so the machine becomes outdated
	kcp.Spec.Version = UpdatedVersion

	// run upgrade, expect we scale down
	needingUpgrade := collections.FromMachineList(machineList)
	controlPlane.Machines = needingUpgrade
	machinesUpToDateResults := map[string]internal.UpToDateResult{}
	for _, m := range needingUpgrade {
		machinesUpToDateResults[m.Name] = internal.UpToDateResult{EligibleForInPlaceUpdate: false}
	}
	result, err = r.updateControlPlane(ctx, controlPlane, needingUpgrade, machinesUpToDateResults)
	g.Expect(result.IsZero()).To(BeTrue())
	g.Expect(err).ToNot(HaveOccurred())
	remainingMachines := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(ctx, remainingMachines, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(remainingMachines.Items).To(HaveLen(2))
}

func Test_rollingUpdate(t *testing.T) {
	tests := []struct {
		name                            string
		maxSurge                        int32
		currentReplicas                 int32
		currentUpToDateReplicas         int32
		desiredReplicas                 int32
		enableInPlaceUpdatesFeatureGate bool
		machineEligibleForInPlaceUpdate bool
		tryInPlaceUpdateFunc            func(ctx context.Context, controlPlane *internal.ControlPlane, machineToInPlaceUpdate *clusterv1.Machine, machineUpToDateResult internal.UpToDateResult) (bool, ctrl.Result, error)
		wantTryInPlaceUpdateCalled      bool
		wantScaleDownCalled             bool
		wantScaleUpCalled               bool
		wantError                       bool
		wantErrorMessage                string
		wantRes                         ctrl.Result
	}{
		// Regular rollout (no in-place updates)
		{
			name:                    "Regular rollout: maxSurge 1: scale up",
			maxSurge:                1,
			currentReplicas:         3,
			currentUpToDateReplicas: 0,
			desiredReplicas:         3,
			wantScaleUpCalled:       true,
		},
		{
			name:                    "Regular rollout: maxSurge 1: scale down",
			maxSurge:                1,
			currentReplicas:         4,
			currentUpToDateReplicas: 1,
			desiredReplicas:         3,
			wantScaleDownCalled:     true,
		},
		{
			name:                    "Regular rollout: maxSurge 0: scale down",
			maxSurge:                0,
			currentReplicas:         3,
			currentUpToDateReplicas: 0,
			desiredReplicas:         3,
			wantScaleDownCalled:     true,
		},
		{
			name:                    "Regular rollout: maxSurge 0: scale up",
			maxSurge:                0,
			currentReplicas:         2,
			currentUpToDateReplicas: 0,
			desiredReplicas:         3,
			wantScaleUpCalled:       true,
		},
		// In-place updates
		// Note: maxSurge 0 or 1 doesn't have an impact on the in-place code path so not testing permutations here.
		// Note: Scale up works the same way as for regular rollouts so not testing it here again.
		//
		// In-place updates: tryInPlaceUpdate not called
		{
			name:                            "In-place updates: feature gate disabled: scale down (tryInPlaceUpdate not called)",
			maxSurge:                        0,
			currentReplicas:                 3,
			currentUpToDateReplicas:         0,
			desiredReplicas:                 3,
			enableInPlaceUpdatesFeatureGate: false,
			wantTryInPlaceUpdateCalled:      false,
			wantScaleDownCalled:             true,
		},
		{
			name:                            "In-place updates: Machine not eligible for in-place: scale down (tryInPlaceUpdate not called)",
			maxSurge:                        0,
			currentReplicas:                 3,
			currentUpToDateReplicas:         0,
			desiredReplicas:                 3,
			enableInPlaceUpdatesFeatureGate: true,
			machineEligibleForInPlaceUpdate: false,
			wantTryInPlaceUpdateCalled:      false,
			wantScaleDownCalled:             true,
		},
		{
			name:                            "In-place updates: already enough up-to-date replicas: scale down (tryInPlaceUpdate not called)",
			maxSurge:                        1,
			currentReplicas:                 4,
			currentUpToDateReplicas:         3,
			desiredReplicas:                 3,
			enableInPlaceUpdatesFeatureGate: true,
			machineEligibleForInPlaceUpdate: true,
			wantTryInPlaceUpdateCalled:      false,
			wantScaleDownCalled:             true,
		},
		// In-place updates: tryInPlaceUpdate called
		{
			name:                            "In-place updates: tryInPlaceUpdate returns error",
			maxSurge:                        0,
			currentReplicas:                 3,
			currentUpToDateReplicas:         0,
			desiredReplicas:                 3,
			enableInPlaceUpdatesFeatureGate: true,
			machineEligibleForInPlaceUpdate: true,
			tryInPlaceUpdateFunc: func(_ context.Context, _ *internal.ControlPlane, _ *clusterv1.Machine, _ internal.UpToDateResult) (bool, ctrl.Result, error) {
				return false, ctrl.Result{}, errors.New("in-place update error")
			},
			wantTryInPlaceUpdateCalled: true,
			wantScaleDownCalled:        false,
			wantError:                  true,
			wantErrorMessage:           "in-place update error",
		},
		{
			name:                            "In-place updates: tryInPlaceUpdate returns Requeue",
			maxSurge:                        0,
			currentReplicas:                 3,
			currentUpToDateReplicas:         0,
			desiredReplicas:                 3,
			enableInPlaceUpdatesFeatureGate: true,
			machineEligibleForInPlaceUpdate: true,
			tryInPlaceUpdateFunc: func(_ context.Context, _ *internal.ControlPlane, _ *clusterv1.Machine, _ internal.UpToDateResult) (fallbackToScaleDown bool, _ ctrl.Result, _ error) {
				return false, ctrl.Result{RequeueAfter: preflightFailedRequeueAfter}, nil
			},
			wantTryInPlaceUpdateCalled: true,
			wantScaleDownCalled:        false,
			wantRes:                    ctrl.Result{RequeueAfter: preflightFailedRequeueAfter},
		},
		{
			name:                            "In-place updates: tryInPlaceUpdate returns fallback to scale down",
			maxSurge:                        0,
			currentReplicas:                 3,
			currentUpToDateReplicas:         0,
			desiredReplicas:                 3,
			enableInPlaceUpdatesFeatureGate: true,
			machineEligibleForInPlaceUpdate: true,
			tryInPlaceUpdateFunc: func(_ context.Context, _ *internal.ControlPlane, _ *clusterv1.Machine, _ internal.UpToDateResult) (fallbackToScaleDown bool, _ ctrl.Result, _ error) {
				return true, ctrl.Result{}, nil
			},
			wantTryInPlaceUpdateCalled: true,
			wantScaleDownCalled:        true,
		},
		{
			name:                            "In-place updates: tryInPlaceUpdate returns nothing (in-place update triggered)",
			maxSurge:                        0,
			currentReplicas:                 3,
			currentUpToDateReplicas:         0,
			desiredReplicas:                 3,
			enableInPlaceUpdatesFeatureGate: true,
			machineEligibleForInPlaceUpdate: true,
			tryInPlaceUpdateFunc: func(_ context.Context, _ *internal.ControlPlane, _ *clusterv1.Machine, _ internal.UpToDateResult) (fallbackToScaleDown bool, _ ctrl.Result, _ error) {
				return false, ctrl.Result{}, nil
			},
			wantTryInPlaceUpdateCalled: true,
			wantScaleDownCalled:        false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			if tt.enableInPlaceUpdatesFeatureGate {
				utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.InPlaceUpdates, true)
			}

			var inPlaceUpdateCalled bool
			var scaleDownCalled bool
			var scaleUpCalled bool
			r := &KubeadmControlPlaneReconciler{
				overrideTryInPlaceUpdateFunc: func(ctx context.Context, controlPlane *internal.ControlPlane, machineToInPlaceUpdate *clusterv1.Machine, machineUpToDateResult internal.UpToDateResult) (bool, ctrl.Result, error) {
					inPlaceUpdateCalled = true
					return tt.tryInPlaceUpdateFunc(ctx, controlPlane, machineToInPlaceUpdate, machineUpToDateResult)
				},
				overrideScaleDownControlPlaneFunc: func(_ context.Context, _ *internal.ControlPlane, _ *clusterv1.Machine) (ctrl.Result, error) {
					scaleDownCalled = true
					return ctrl.Result{}, nil
				},
				overrideScaleUpControlPlaneFunc: func(_ context.Context, _ *internal.ControlPlane) (ctrl.Result, error) {
					scaleUpCalled = true
					return ctrl.Result{}, nil
				},
			}

			machines := collections.Machines{}
			for i := range tt.currentReplicas {
				machines[fmt.Sprintf("machine-%d", i)] = machine(fmt.Sprintf("machine-%d", i))
			}
			machinesUpToDate := collections.Machines{}
			for i := range tt.currentUpToDateReplicas {
				machinesUpToDate[fmt.Sprintf("machine-%d", i)] = machine(fmt.Sprintf("machine-%d", i))
			}

			controlPlane := &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec: controlplanev1.KubeadmControlPlaneSpec{
						Replicas: ptr.To(tt.desiredReplicas),
						Rollout: controlplanev1.KubeadmControlPlaneRolloutSpec{
							Strategy: controlplanev1.KubeadmControlPlaneRolloutStrategy{
								RollingUpdate: controlplanev1.KubeadmControlPlaneRolloutStrategyRollingUpdate{
									MaxSurge: ptr.To(intstr.FromInt32(tt.maxSurge)),
								},
							},
						},
					},
				},
				Cluster:             &clusterv1.Cluster{},
				Machines:            machines,
				MachinesNotUpToDate: machines.Difference(machinesUpToDate),
			}
			machinesNeedingRollout, _ := controlPlane.MachinesNeedingRollout()
			machinesUpToDateResults := map[string]internal.UpToDateResult{}
			for _, m := range machinesNeedingRollout {
				machinesUpToDateResults[m.Name] = internal.UpToDateResult{EligibleForInPlaceUpdate: tt.machineEligibleForInPlaceUpdate}
			}
			res, err := r.rollingUpdate(ctx, controlPlane, machinesNeedingRollout, machinesUpToDateResults)
			if tt.wantError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.wantErrorMessage))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(res).To(Equal(tt.wantRes))

			g.Expect(inPlaceUpdateCalled).To(Equal(tt.wantTryInPlaceUpdateCalled), "inPlaceUpdateCalled: actual: %t expected: %t", inPlaceUpdateCalled, tt.wantTryInPlaceUpdateCalled)
			g.Expect(scaleDownCalled).To(Equal(tt.wantScaleDownCalled), "scaleDownCalled: actual: %t expected: %t", scaleDownCalled, tt.wantScaleDownCalled)
			g.Expect(scaleUpCalled).To(Equal(tt.wantScaleUpCalled), "scaleUpCalled: actual: %t expected: %t", scaleUpCalled, tt.wantScaleUpCalled)
		})
	}
}

type machineOpt func(*clusterv1.Machine)

func machine(name string, opts ...machineOpt) *clusterv1.Machine {
	m := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}
