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

package kubeadmcontrolplane

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	. "github.com/onsi/gomega"
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
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/pkg"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/pkg/desiredstate"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/pkg/etcd"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/setup"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/pkg/dynamiccache"
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
	setupEnv := func(t *testing.T, g *WithT) *corev1.Namespace {
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
	namespace := setupEnv(t, g)
	defer teardown(t, g, namespace)

	timeout := 30 * time.Second

	cluster, kcp, genericInfrastructureMachineTemplate := createClusterWithControlPlane(namespace.Name)
	g.Expect(env.CreateAndWait(ctx, genericInfrastructureMachineTemplate, client.FieldOwner("manager"))).To(Succeed())
	// Note: Wait additionally until dynamicCache is up-to-date, CreateAndWait above only waits until
	// the regular cache in the manager is up-to-date.
	g.Eventually(func(g Gomega) {
		_, err := dynamicCache.GetUnstructured(ctx, setup.DynamicCacheInfraMachineTemplateObjectType, genericInfrastructureMachineTemplate.GroupVersionKind().GroupKind(), client.ObjectKeyFromObject(genericInfrastructureMachineTemplate))
		g.Expect(err).ToNot(HaveOccurred())
	}).WithTimeout(5 * time.Second).To(Succeed())

	cluster.UID = types.UID(util.RandomString(10))
	cluster.Spec.ControlPlaneEndpoint.Host = Host
	cluster.Spec.ControlPlaneEndpoint.Port = 6443
	cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
	kcp.UID = types.UID(util.RandomString(10))
	kcp.Spec.Replicas = ptr.To[int32](1)
	setKCPHealthy(kcp)

	fc := capicontrollerutil.NewFakeController()

	r := &Reconciler{
		Client:              env,
		SecretCachingClient: secretCachingClient,
		DynamicCache:        dynamicCache,
		controller:          fc,
		recorder:            record.NewFakeRecorder(32),
		managementCluster: &fakeManagementCluster{
			Management: &pkg.Management{Client: env},
			Workload: &fakeWorkloadCluster{
				Workload: &pkg.Workload{
					Client: env,
				},
			},
		},
		ssaCache: ssa.NewCache("test-controller"),
	}
	var err error
	r.machineClientWithDeleteResponse, err = capicontrollerutil.NewClientWithDeleteResponse(&clusterv1.Machine{}, machineGR,
		env.GetScheme(), env.GetConfig(), env.GetHTTPClient())
	g.Expect(err).ToNot(HaveOccurred())
	controlPlane := &pkg.ControlPlane{
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
	machinesUpToDateResults := map[string]pkg.UpToDateResult{}
	for _, m := range needingUpgrade {
		machinesUpToDateResults[m.Name] = pkg.UpToDateResult{EligibleForInPlaceUpdate: false}
	}
	controlPlane.MachinesNotUpToDate = needingUpgrade
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
	controlPlane = &pkg.ControlPlane{
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
	for i := range bothMachines.Items {
		setMachineHealthy(&bothMachines.Items[i])
	}
	controlPlane.Machines = collections.FromMachineList(bothMachines)
	controlPlane.EtcdLeader = &etcd.Member{Name: controlPlane.Machines.UnsortedList()[0].Name}

	machinesRequireUpgrade := collections.Machines{}
	for i := range bothMachines.Items {
		if bothMachines.Items[i].Spec.Version != "" && bothMachines.Items[i].Spec.Version != UpdatedVersion {
			machinesRequireUpgrade[bothMachines.Items[i].Name] = &bothMachines.Items[i]
		}
	}
	machinesUpToDateResults = map[string]pkg.UpToDateResult{}
	for _, m := range machinesRequireUpgrade {
		machinesUpToDateResults[m.Name] = pkg.UpToDateResult{EligibleForInPlaceUpdate: false}
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
		Workload: &fakeWorkloadCluster{},
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
	fmc.Workload.Workload = &pkg.Workload{Client: fakeClient}
	r := &Reconciler{
		Client:                          fakeClient,
		SecretCachingClient:             fakeClient,
		DynamicCache:                    dynamiccache.NewFakeDynamicCache(fakeClient, setup.DynamicCacheOptions()),
		machineClientWithDeleteResponse: capicontrollerutil.NewClientWithDeleteResponseFromClient(fakeClient),
		managementCluster:               fmc,
	}

	controlPlane := &pkg.ControlPlane{
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
	controlPlane.EtcdLeader = &etcd.Member{Name: controlPlane.Machines.UnsortedList()[0].Name}
	machinesUpToDateResults := map[string]pkg.UpToDateResult{}
	for _, m := range needingUpgrade {
		machinesUpToDateResults[m.Name] = pkg.UpToDateResult{EligibleForInPlaceUpdate: false}
	}
	result, err = r.updateControlPlane(ctx, controlPlane, needingUpgrade, machinesUpToDateResults)
	g.Expect(result.IsZero()).To(BeTrue())
	g.Expect(err).ToNot(HaveOccurred())
	remainingMachines := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(ctx, remainingMachines, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(remainingMachines.Items).To(HaveLen(2))
}

func Test_rollingUpdate(t *testing.T) {
	preflightChecksFailedFunc := func(_ context.Context, _ *pkg.ControlPlane, _ ...*clusterv1.Machine) preflightChecksResult {
		return preflightChecksResult{succeeded: false, requeueAfter: 3 * time.Second}
	}
	preflightChecksSucceededFunc := func(_ context.Context, _ *pkg.ControlPlane, _ ...*clusterv1.Machine) preflightChecksResult {
		return preflightChecksResult{succeeded: true}
	}
	preflightChecksSucceededForSpecificMachineFunc := func(_ context.Context, _ *pkg.ControlPlane, excludeFor ...*clusterv1.Machine) preflightChecksResult {
		if len(excludeFor) > 0 {
			return preflightChecksResult{succeeded: true}
		}
		return preflightChecksResult{succeeded: false, requeueAfter: 3 * time.Second}
	}
	canUpdateMachineTrueFunc := func(_ context.Context, _ *clusterv1.Machine, _ pkg.UpToDateResult) (canUpdateMachineResult, error) {
		return canUpdateMachineResult{canUpdateMachine: true, affectsAvailability: true}, nil
	}
	canUpdateMachineTrueAndDoesNotAffectAvailabilityFunc := func(_ context.Context, _ *clusterv1.Machine, _ pkg.UpToDateResult) (canUpdateMachineResult, error) {
		return canUpdateMachineResult{canUpdateMachine: true, affectsAvailability: false}, nil
	}
	canUpdateMachineFalseFunc := func(_ context.Context, _ *clusterv1.Machine, _ pkg.UpToDateResult) (canUpdateMachineResult, error) {
		return canUpdateMachineResult{canUpdateMachine: false, affectsAvailability: false}, nil
	}

	tests := []struct {
		name                            string
		maxSurge                        int32
		currentReplicas                 int32
		currentUpToDateReplicas         int32
		desiredReplicas                 int32
		isRemediation                   bool
		enableInPlaceUpdatesFeatureGate bool
		machineEligibleForInPlaceUpdate bool
		preflightChecksFunc             func(ctx context.Context, controlPlane *pkg.ControlPlane, excludeFor ...*clusterv1.Machine) preflightChecksResult
		canUpdateMachineFunc            func(ctx context.Context, machine *clusterv1.Machine, machineUpToDateResult pkg.UpToDateResult) (canUpdateMachineResult, error)
		wantPreflightChecksFuncCalled   bool
		wantCanUpdateMachineCalled      bool
		wantScaleDownCalled             bool
		wantScaleUpCalled               bool
		wantTriggerInPlaceUpdateCalled  bool
		wantError                       bool
		wantErrorMessage                string
		wantRes                         ctrl.Result
	}{
		// Regular rollout (no in-place updates, enableInPlaceUpdatesFeatureGate: false)
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
			name:                    "Regular rollout: maxSurge 0: scale up (currentReplicas == minReplicas && currentReplicas < desiredReplicas)",
			maxSurge:                0,
			currentReplicas:         2,
			currentUpToDateReplicas: 0,
			desiredReplicas:         3,
			wantScaleUpCalled:       true,
		},
		{
			name:                    "Regular rollout: already enough up-to-date replicas: scale down",
			maxSurge:                1,
			currentReplicas:         4,
			currentUpToDateReplicas: 3,
			desiredReplicas:         3,
			wantScaleDownCalled:     true,
		},
		{
			name:                    "Remediation: scale up",
			maxSurge:                0,
			currentReplicas:         2,
			currentUpToDateReplicas: 0,
			desiredReplicas:         3,
			isRemediation:           true,
			wantScaleUpCalled:       true,
		},
		{
			name:                    "Below min replicas: scale up",
			maxSurge:                1,
			currentReplicas:         2,
			currentUpToDateReplicas: 0,
			desiredReplicas:         3,
			wantScaleUpCalled:       true,
		},
		{
			name:                    "Above max replicas: scale down",
			maxSurge:                1,
			currentReplicas:         5,
			currentUpToDateReplicas: 0,
			desiredReplicas:         3,
			wantScaleDownCalled:     true,
		},
		// In-place updates
		// Note: maxSurge 0 or 1 doesn't have an impact on the in-place code path so not testing permutations here.
		//
		// In-place updates: inPlaceUpdateOrScaleUpControlPlane
		{
			name:                            "In-place updates: Machine not eligible for in-place: scale up",
			maxSurge:                        1,
			currentReplicas:                 3,
			currentUpToDateReplicas:         0,
			desiredReplicas:                 3,
			enableInPlaceUpdatesFeatureGate: true,
			machineEligibleForInPlaceUpdate: false,
			wantTriggerInPlaceUpdateCalled:  false,
			wantScaleUpCalled:               true,
		},
		{
			name:                            "In-place updates: preflightChecks failed",
			maxSurge:                        1,
			currentReplicas:                 3,
			currentUpToDateReplicas:         0,
			desiredReplicas:                 3,
			enableInPlaceUpdatesFeatureGate: true,
			machineEligibleForInPlaceUpdate: true,
			preflightChecksFunc:             preflightChecksFailedFunc,
			wantPreflightChecksFuncCalled:   true,
			wantTriggerInPlaceUpdateCalled:  false,
			wantScaleUpCalled:               false,
			wantRes:                         ctrl.Result{RequeueAfter: 3 * time.Second},
		},
		{
			name:                            "In-place updates: preflightChecks succeeded, canUpdateMachine: true, affectsAvailability: false, triggerInPlaceUpdate called",
			maxSurge:                        1,
			currentReplicas:                 3,
			currentUpToDateReplicas:         0,
			desiredReplicas:                 3,
			enableInPlaceUpdatesFeatureGate: true,
			machineEligibleForInPlaceUpdate: true,
			preflightChecksFunc:             preflightChecksSucceededFunc,
			canUpdateMachineFunc:            canUpdateMachineTrueAndDoesNotAffectAvailabilityFunc,
			wantPreflightChecksFuncCalled:   true,
			wantCanUpdateMachineCalled:      true,
			wantTriggerInPlaceUpdateCalled:  true,
			wantScaleUpCalled:               false,
		},
		{
			name:                            "In-place updates: preflightChecks succeeded, canUpdateMachine: true, affectsAvailability: true, fallback to scale up",
			maxSurge:                        1,
			currentReplicas:                 3,
			currentUpToDateReplicas:         0,
			desiredReplicas:                 3,
			enableInPlaceUpdatesFeatureGate: true,
			machineEligibleForInPlaceUpdate: true,
			preflightChecksFunc:             preflightChecksSucceededFunc,
			canUpdateMachineFunc:            canUpdateMachineTrueFunc,
			wantPreflightChecksFuncCalled:   true,
			wantCanUpdateMachineCalled:      true,
			wantTriggerInPlaceUpdateCalled:  false,
			wantScaleUpCalled:               true,
		},
		{
			name:                            "In-place updates: preflightChecks succeeded, canUpdateMachine: false, fallback to scale up",
			maxSurge:                        1,
			currentReplicas:                 3,
			currentUpToDateReplicas:         0,
			desiredReplicas:                 3,
			enableInPlaceUpdatesFeatureGate: true,
			machineEligibleForInPlaceUpdate: true,
			preflightChecksFunc:             preflightChecksSucceededFunc,
			canUpdateMachineFunc:            canUpdateMachineFalseFunc,
			wantPreflightChecksFuncCalled:   true,
			wantCanUpdateMachineCalled:      true,
			wantTriggerInPlaceUpdateCalled:  false,
			wantScaleUpCalled:               true,
		},
		// In-place updates: inPlaceUpdateOrScaleDownControlPlane
		{
			name:                            "In-place updates: Machine not eligible for in-place: scale down",
			maxSurge:                        0,
			currentReplicas:                 3,
			currentUpToDateReplicas:         0,
			desiredReplicas:                 3,
			enableInPlaceUpdatesFeatureGate: true,
			machineEligibleForInPlaceUpdate: false,
			wantTriggerInPlaceUpdateCalled:  false,
			wantScaleDownCalled:             true,
		},
		{
			name:                            "In-place updates: preflightChecks failed",
			maxSurge:                        0,
			currentReplicas:                 3,
			currentUpToDateReplicas:         0,
			desiredReplicas:                 3,
			enableInPlaceUpdatesFeatureGate: true,
			machineEligibleForInPlaceUpdate: true,
			preflightChecksFunc:             preflightChecksFailedFunc,
			wantPreflightChecksFuncCalled:   true,
			wantTriggerInPlaceUpdateCalled:  false,
			wantScaleDownCalled:             false,
			wantRes:                         ctrl.Result{RequeueAfter: 3 * time.Second},
		},
		{
			name:                            "In-place updates: preflightChecks succeeded only for specific Machine, fallback to scale down",
			maxSurge:                        0,
			currentReplicas:                 3,
			currentUpToDateReplicas:         0,
			desiredReplicas:                 3,
			enableInPlaceUpdatesFeatureGate: true,
			machineEligibleForInPlaceUpdate: true,
			preflightChecksFunc:             preflightChecksSucceededForSpecificMachineFunc,
			wantPreflightChecksFuncCalled:   true,
			wantTriggerInPlaceUpdateCalled:  false,
			wantScaleDownCalled:             true,
		},
		{
			name:                            "In-place updates: preflightChecks succeeded, canUpdateMachine: true, triggerInPlaceUpdate called",
			maxSurge:                        0,
			currentReplicas:                 3,
			currentUpToDateReplicas:         0,
			desiredReplicas:                 3,
			enableInPlaceUpdatesFeatureGate: true,
			machineEligibleForInPlaceUpdate: true,
			preflightChecksFunc:             preflightChecksSucceededFunc,
			canUpdateMachineFunc:            canUpdateMachineTrueFunc,
			wantPreflightChecksFuncCalled:   true,
			wantCanUpdateMachineCalled:      true,
			wantTriggerInPlaceUpdateCalled:  true,
			wantScaleDownCalled:             false,
		},
		{
			name:                            "In-place updates: preflightChecks succeeded, canUpdateMachine: false, fallback to scale down",
			maxSurge:                        0,
			currentReplicas:                 3,
			currentUpToDateReplicas:         0,
			desiredReplicas:                 3,
			enableInPlaceUpdatesFeatureGate: true,
			machineEligibleForInPlaceUpdate: true,
			preflightChecksFunc:             preflightChecksSucceededFunc,
			canUpdateMachineFunc:            canUpdateMachineFalseFunc,
			wantPreflightChecksFuncCalled:   true,
			wantCanUpdateMachineCalled:      true,
			wantTriggerInPlaceUpdateCalled:  false,
			wantScaleDownCalled:             true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			if tt.enableInPlaceUpdatesFeatureGate {
				utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.InPlaceUpdates, true)
			}

			var preflightChecksFuncCalled bool
			var canUpdateMachineCalled bool
			var scaleDownCalled bool
			var scaleUpCalled bool
			var triggerInPlaceUpdateCalled bool
			r := &Reconciler{
				overridePreflightChecksFunc: func(ctx context.Context, controlPlane *pkg.ControlPlane, excludeFor ...*clusterv1.Machine) preflightChecksResult {
					preflightChecksFuncCalled = true
					return tt.preflightChecksFunc(ctx, controlPlane, excludeFor...)
				},
				overrideCanUpdateMachineFunc: func(ctx context.Context, machine *clusterv1.Machine, machineUpToDateResult pkg.UpToDateResult) (canUpdateMachineResult, error) {
					canUpdateMachineCalled = true
					return tt.canUpdateMachineFunc(ctx, machine, machineUpToDateResult)
				},
				overrideScaleDownControlPlaneFunc: func(_ context.Context, _ *pkg.ControlPlane, _ *clusterv1.Machine) (ctrl.Result, error) {
					scaleDownCalled = true
					return ctrl.Result{}, nil
				},
				overrideScaleUpControlPlaneFunc: func(_ context.Context, _ *pkg.ControlPlane) (ctrl.Result, error) {
					scaleUpCalled = true
					return ctrl.Result{}, nil
				},
				overrideTriggerInPlaceUpdate: func(_ context.Context, _ *pkg.ControlPlane, _ *clusterv1.Machine, _ pkg.UpToDateResult, _ bool) error {
					triggerInPlaceUpdateCalled = true
					return nil
				},
				controller: capicontrollerutil.NewFakeController(),
				recorder:   record.NewFakeRecorder(32),
			}

			machines := collections.Machines{}
			for i := range tt.currentReplicas {
				machines[fmt.Sprintf("machine-%d", i)] = machine(fmt.Sprintf("machine-%d", i))
			}
			machinesUpToDate := collections.Machines{}
			for i := range tt.currentUpToDateReplicas {
				machinesUpToDate[fmt.Sprintf("machine-%d", i)] = machine(fmt.Sprintf("machine-%d", i))
			}

			controlPlane := &pkg.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: func() map[string]string {
							annotations := map[string]string{}
							if tt.isRemediation {
								annotations[controlplanev1.RemediationInProgressAnnotation] = ""
							}
							return annotations
						}(),
					},
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
			machinesUpToDateResults := map[string]pkg.UpToDateResult{}
			for _, m := range machinesNeedingRollout {
				machinesUpToDateResults[m.Name] = pkg.UpToDateResult{EligibleForInPlaceUpdate: tt.machineEligibleForInPlaceUpdate}
			}
			res, err := r.rollingUpdate(ctx, controlPlane, machinesNeedingRollout, machinesUpToDateResults)
			if tt.wantError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.wantErrorMessage))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(res).To(Equal(tt.wantRes))

			g.Expect(preflightChecksFuncCalled).To(Equal(tt.wantPreflightChecksFuncCalled), "preflightChecksFuncCalled: actual: %t expected: %t", preflightChecksFuncCalled, tt.wantPreflightChecksFuncCalled)
			g.Expect(canUpdateMachineCalled).To(Equal(tt.wantCanUpdateMachineCalled), "canUpdateMachineCalled: actual: %t expected: %t", canUpdateMachineCalled, tt.wantCanUpdateMachineCalled)
			g.Expect(scaleDownCalled).To(Equal(tt.wantScaleDownCalled), "scaleDownCalled: actual: %t expected: %t", scaleDownCalled, tt.wantScaleDownCalled)
			g.Expect(scaleUpCalled).To(Equal(tt.wantScaleUpCalled), "scaleUpCalled: actual: %t expected: %t", scaleUpCalled, tt.wantScaleUpCalled)
			g.Expect(triggerInPlaceUpdateCalled).To(Equal(tt.wantTriggerInPlaceUpdateCalled), "triggerInPlaceUpdateCalled: actual: %t expected: %t", triggerInPlaceUpdateCalled, tt.wantTriggerInPlaceUpdateCalled)
		})
	}
}

func Test_rollingUpdateSequences(t *testing.T) {
	type machineAttr struct {
		Name                     string
		UpToDate                 bool
		EligibleForInPlaceUpdate bool
		CanUpdateMachine         bool
		AffectsAvailability      bool
	}
	newMachine := func(attr machineAttr) *clusterv1.Machine {
		return &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: attr.Name,
				Annotations: map[string]string{
					"upToDate":                 strconv.FormatBool(attr.UpToDate),
					"eligibleForInPlaceUpdate": strconv.FormatBool(attr.EligibleForInPlaceUpdate),
					"canUpdateMachine":         strconv.FormatBool(attr.CanUpdateMachine),
					"affectsAvailability":      strconv.FormatBool(attr.AffectsAvailability),
				},
			},
		}
	}
	mustParseBool := func(s string) bool {
		b, err := strconv.ParseBool(s)
		if err != nil {
			panic(err)
		}
		return b
	}
	attrFromMachine := func(machine *clusterv1.Machine) machineAttr {
		return machineAttr{
			Name:                     machine.Name,
			UpToDate:                 mustParseBool(machine.Annotations["upToDate"]),
			EligibleForInPlaceUpdate: mustParseBool(machine.Annotations["eligibleForInPlaceUpdate"]),
			CanUpdateMachine:         mustParseBool(machine.Annotations["canUpdateMachine"]),
			AffectsAvailability:      mustParseBool(machine.Annotations["affectsAvailability"]),
		}
	}

	tests := []struct {
		name                  string
		desiredReplicas       int32
		maxSurge              int32
		remediationInProgress bool
		// Machine names must match the pattern: machine-0, machine-1, ...
		// When scale up creates new Machines it will continue the sequence
		machines         []*clusterv1.Machine
		skipVerifyMinMax bool
		wantSequence     []string
	}{
		// Regular rollout (no in-place)
		{
			name:            "Regular rollout, 3 Replicas, maxSurge 1",
			desiredReplicas: 3,
			maxSurge:        1,
			machines: []*clusterv1.Machine{
				newMachine(machineAttr{Name: "machine-0", UpToDate: false, EligibleForInPlaceUpdate: false, CanUpdateMachine: false, AffectsAvailability: false}),
				newMachine(machineAttr{Name: "machine-1", UpToDate: false, EligibleForInPlaceUpdate: false, CanUpdateMachine: false, AffectsAvailability: false}),
				newMachine(machineAttr{Name: "machine-2", UpToDate: false, EligibleForInPlaceUpdate: false, CanUpdateMachine: false, AffectsAvailability: false}),
			},
			wantSequence: []string{
				"machine-3 created",
				"machine-0 deleted",
				"machine-4 created",
				"machine-1 deleted",
				"machine-5 created",
				"machine-2 deleted",
			},
		},
		{
			name:            "Regular rollout, 3 Replicas, maxSurge 0",
			desiredReplicas: 3,
			maxSurge:        0,
			machines: []*clusterv1.Machine{
				newMachine(machineAttr{Name: "machine-0", UpToDate: false, EligibleForInPlaceUpdate: false, CanUpdateMachine: false, AffectsAvailability: false}),
				newMachine(machineAttr{Name: "machine-1", UpToDate: false, EligibleForInPlaceUpdate: false, CanUpdateMachine: false, AffectsAvailability: false}),
				newMachine(machineAttr{Name: "machine-2", UpToDate: false, EligibleForInPlaceUpdate: false, CanUpdateMachine: false, AffectsAvailability: false}),
			},
			wantSequence: []string{
				"machine-0 deleted",
				"machine-3 created",
				"machine-1 deleted",
				"machine-4 created",
				"machine-2 deleted",
				// machine-5 will be created in the scaleUp code path, not in rollingUpdate
			},
		},
		{
			name:            "Regular rollout, 3 Replicas, maxSurge 1, scale up",
			desiredReplicas: 3, // scale up
			maxSurge:        1,
			machines: []*clusterv1.Machine{
				newMachine(machineAttr{Name: "machine-0", UpToDate: false, EligibleForInPlaceUpdate: false, CanUpdateMachine: false, AffectsAvailability: false}),
			},
			wantSequence: []string{
				"machine-1 created",
				"machine-2 created",
				"machine-3 created",
				"machine-0 deleted",
			},
		},
		{
			name:            "Regular rollout, 3 Replicas, maxSurge 1, scale down",
			desiredReplicas: 1, // scale down
			maxSurge:        1,
			machines: []*clusterv1.Machine{
				newMachine(machineAttr{Name: "machine-0", UpToDate: false, EligibleForInPlaceUpdate: false, CanUpdateMachine: false, AffectsAvailability: false}),
				newMachine(machineAttr{Name: "machine-1", UpToDate: false, EligibleForInPlaceUpdate: false, CanUpdateMachine: false, AffectsAvailability: false}),
				newMachine(machineAttr{Name: "machine-2", UpToDate: false, EligibleForInPlaceUpdate: false, CanUpdateMachine: false, AffectsAvailability: false}),
			},
			wantSequence: []string{
				"machine-0 deleted",
				"machine-1 deleted",
				"machine-3 created",
				"machine-2 deleted",
			},
		},
		// Rollout with In-place updates (AffectsAvailability: true)
		{
			name:            "In-place rollout, 3 Replicas, maxSurge 1 (AffectsAvailability: true)",
			desiredReplicas: 3,
			maxSurge:        1,
			machines: []*clusterv1.Machine{
				newMachine(machineAttr{Name: "machine-0", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: true}),
				newMachine(machineAttr{Name: "machine-1", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: true}),
				newMachine(machineAttr{Name: "machine-2", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: true}),
			},
			wantSequence: []string{
				"machine-3 created",
				"machine-0 updated",
				"machine-1 updated",
				"machine-2 deleted",
			},
		},
		{
			name:            "In-place rollout, 3 Replicas, maxSurge 0 (AffectsAvailability: true)",
			desiredReplicas: 3,
			maxSurge:        0,
			machines: []*clusterv1.Machine{
				newMachine(machineAttr{Name: "machine-0", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: true}),
				newMachine(machineAttr{Name: "machine-1", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: true}),
				newMachine(machineAttr{Name: "machine-2", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: true}),
			},
			wantSequence: []string{
				"machine-0 updated",
				"machine-1 updated",
				"machine-2 updated",
			},
		},
		{
			name:            "In-place rollout, 3 Replicas, maxSurge 1, scale up (AffectsAvailability: true)",
			desiredReplicas: 3, // scale up
			maxSurge:        1,
			machines: []*clusterv1.Machine{
				newMachine(machineAttr{Name: "machine-0", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: true}),
			},
			wantSequence: []string{
				"machine-1 created",
				"machine-2 created",
				"machine-3 created",
				"machine-0 deleted",
			},
		},
		{
			name:            "In-place rollout, 3 Replicas, maxSurge 1, scale down (AffectsAvailability: true)",
			desiredReplicas: 1, // scale down
			maxSurge:        1,
			machines: []*clusterv1.Machine{
				newMachine(machineAttr{Name: "machine-0", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: true}),
				newMachine(machineAttr{Name: "machine-1", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: true}),
				newMachine(machineAttr{Name: "machine-2", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: true}),
			},
			wantSequence: []string{
				"machine-0 deleted",
				"machine-1 updated",
				"machine-2 deleted",
			},
		},
		// Rollout with In-place updates (AffectsAvailability: false)
		{
			name:            "In-place rollout, 3 Replicas, maxSurge 1 (AffectsAvailability: false)",
			desiredReplicas: 3,
			maxSurge:        1,
			machines: []*clusterv1.Machine{
				newMachine(machineAttr{Name: "machine-0", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: false}),
				newMachine(machineAttr{Name: "machine-1", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: false}),
				newMachine(machineAttr{Name: "machine-2", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: false}),
			},
			wantSequence: []string{
				"machine-0 updated",
				"machine-1 updated",
				"machine-2 updated",
			},
		},
		{
			name:            "In-place rollout, 3 Replicas, maxSurge 0 (AffectsAvailability: false)",
			desiredReplicas: 3,
			maxSurge:        0,
			machines: []*clusterv1.Machine{
				newMachine(machineAttr{Name: "machine-0", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: false}),
				newMachine(machineAttr{Name: "machine-1", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: false}),
				newMachine(machineAttr{Name: "machine-2", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: false}),
			},
			wantSequence: []string{
				"machine-0 updated",
				"machine-1 updated",
				"machine-2 updated",
			},
		},
		{
			name:            "In-place rollout, 3 Replicas, maxSurge 1, scale up (AffectsAvailability: false)",
			desiredReplicas: 3, // scale up
			maxSurge:        1,
			machines: []*clusterv1.Machine{
				newMachine(machineAttr{Name: "machine-0", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: false}),
			},
			wantSequence: []string{
				"machine-1 created",
				"machine-2 created",
				"machine-0 updated",
			},
		},
		{
			name:            "In-place rollout, 3 Replicas, maxSurge 1, scale down (AffectsAvailability: false)",
			desiredReplicas: 1, // scale down
			maxSurge:        1,
			machines: []*clusterv1.Machine{
				newMachine(machineAttr{Name: "machine-0", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: false}),
				newMachine(machineAttr{Name: "machine-1", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: false}),
				newMachine(machineAttr{Name: "machine-2", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: false}),
			},
			wantSequence: []string{
				"machine-0 deleted",
				"machine-1 updated",
				"machine-2 deleted",
			},
		},
		{
			name:            "In-place rollout, 3 Replicas, maxSurge 0 (AffectsAvailability: false), 2 current Replicas",
			desiredReplicas: 3,
			maxSurge:        0,
			machines: []*clusterv1.Machine{
				newMachine(machineAttr{Name: "machine-0", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: false}),
				newMachine(machineAttr{Name: "machine-1", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: false}),
			},
			wantSequence: []string{
				"machine-2 created",
				"machine-0 updated",
				"machine-1 updated",
			},
		},
		// Rollout with In-place updates: 1 CP Machine
		{
			name:            "In-place rollout, 1 Replicas, maxSurge 1 (AffectsAvailability: true)",
			desiredReplicas: 1,
			maxSurge:        1,
			machines: []*clusterv1.Machine{
				newMachine(machineAttr{Name: "machine-0", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: true}),
			},
			wantSequence: []string{
				"machine-1 created",
				"machine-0 deleted",
			},
		},
		{
			name:            "In-place rollout, 1 Replicas, maxSurge 1 (AffectsAvailability: false)",
			desiredReplicas: 1,
			maxSurge:        1,
			machines: []*clusterv1.Machine{
				newMachine(machineAttr{Name: "machine-0", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: false}),
			},
			wantSequence: []string{
				"machine-0 updated",
			},
		},
		// Rollout with both regular rollout & in-place updates
		{
			name:            "Regular & In-place rollout, 3 Replicas, maxSurge 1",
			desiredReplicas: 3,
			maxSurge:        1,
			machines: []*clusterv1.Machine{
				newMachine(machineAttr{Name: "machine-0", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: false}),
				newMachine(machineAttr{Name: "machine-1", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: false, AffectsAvailability: false}),
				newMachine(machineAttr{Name: "machine-2", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: true}),
			},
			wantSequence: []string{
				"machine-0 updated",
				"machine-3 created",
				"machine-1 deleted",
				"machine-4 created",
				"machine-2 deleted",
			},
		},
		{
			name:            "Regular & In-place rollout, 5 Replicas, maxSurge 1",
			desiredReplicas: 5,
			maxSurge:        1,
			machines: []*clusterv1.Machine{
				newMachine(machineAttr{Name: "machine-0", UpToDate: true}),
				newMachine(machineAttr{Name: "machine-1", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: false, AffectsAvailability: false}),
				newMachine(machineAttr{Name: "machine-2", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: true}),
				newMachine(machineAttr{Name: "machine-3", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: false}),
				newMachine(machineAttr{Name: "machine-4", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: true}),
			},
			wantSequence: []string{
				"machine-5 created",
				"machine-1 deleted",
				"machine-6 created",
				"machine-2 updated",
				"machine-3 updated",
				"machine-4 deleted",
			},
		},
		// Remediation
		{
			name:                  "In-place rollout, 3 Replicas, maxSurge 1, remediation",
			desiredReplicas:       3,
			maxSurge:              1,
			remediationInProgress: true,
			machines: []*clusterv1.Machine{
				newMachine(machineAttr{Name: "machine-0", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: true}),
				newMachine(machineAttr{Name: "machine-1", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: true}),
				// one Machine got remediated (i.e. deleted)
			},
			wantSequence: []string{
				"machine-2 created",
				"machine-3 created",
				"machine-0 updated",
				"machine-1 deleted",
			},
		},
		{
			name:                  "In-place rollout, 3 Replicas, maxSurge 1, scale up, remediation",
			desiredReplicas:       5, // scale up
			maxSurge:              1,
			remediationInProgress: true,
			machines: []*clusterv1.Machine{
				newMachine(machineAttr{Name: "machine-0", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: true}),
				newMachine(machineAttr{Name: "machine-1", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: true}),
				// one Machine got remediated (i.e. deleted)
			},
			wantSequence: []string{
				"machine-2 created",
				"machine-3 created",
				"machine-4 created",
				"machine-5 created",
				"machine-0 updated",
				"machine-1 deleted",
			},
		},
		{
			name:                  "In-place rollout, 3 Replicas, maxSurge 1, scale down, remediation",
			desiredReplicas:       1, // scale down
			maxSurge:              1,
			remediationInProgress: true,
			machines: []*clusterv1.Machine{
				newMachine(machineAttr{Name: "machine-0", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: true}),
				newMachine(machineAttr{Name: "machine-1", UpToDate: false, EligibleForInPlaceUpdate: true, CanUpdateMachine: true, AffectsAvailability: true}),
				// one Machine got remediated (i.e. deleted)
			},
			// As we first scale up after remediation we are intentionally violating the min/max range.
			// Note: In reality this should never happen because reconcileUnhealthyMachines should clean up
			// the remediation annotation in this case.
			skipVerifyMinMax: true,
			wantSequence: []string{
				"machine-2 created",
				"machine-0 deleted",
				"machine-1 deleted",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.InPlaceUpdates, true)

			g := NewWithT(t)

			machines := collections.Machines{}
			machinesNotUpToDate := collections.Machines{}
			for _, m := range tt.machines {
				machines[m.Name] = m
				if v, ok := m.Annotations["upToDate"]; ok && v == "false" {
					machinesNotUpToDate[m.Name] = m
				}
			}

			controlPlane := &pkg.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: func() map[string]string {
							annotations := map[string]string{}
							if tt.remediationInProgress {
								annotations[controlplanev1.RemediationInProgressAnnotation] = ""
							}
							return annotations
						}(),
					},
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
				MachinesNotUpToDate: machinesNotUpToDate,
			}

			var sequence []string
			machineCounter := len(tt.machines)
			r := &Reconciler{
				overridePreflightChecksFunc: func(_ context.Context, _ *pkg.ControlPlane, _ ...*clusterv1.Machine) preflightChecksResult {
					// Assuming for this test that preflightChecks always succeed.
					return preflightChecksResult{succeeded: true}
				},
				overrideCanUpdateMachineFunc: func(_ context.Context, m *clusterv1.Machine, _ pkg.UpToDateResult) (canUpdateMachineResult, error) {
					attr := attrFromMachine(m)
					return canUpdateMachineResult{canUpdateMachine: attr.CanUpdateMachine, affectsAvailability: attr.AffectsAvailability}, nil
				},
				overrideScaleDownControlPlaneFunc: func(_ context.Context, _ *pkg.ControlPlane, m *clusterv1.Machine) (ctrl.Result, error) {
					delete(controlPlane.Machines, m.Name)
					delete(controlPlane.MachinesNotUpToDate, m.Name)
					sequence = append(sequence, fmt.Sprintf("%s deleted", m.Name))
					return ctrl.Result{}, nil
				},
				overrideScaleUpControlPlaneFunc: func(_ context.Context, _ *pkg.ControlPlane) (ctrl.Result, error) {
					m := newMachine(machineAttr{Name: fmt.Sprintf("machine-%d", machineCounter), UpToDate: true})
					controlPlane.Machines[m.Name] = m
					delete(controlPlane.KCP.Annotations, controlplanev1.RemediationInProgressAnnotation)
					machineCounter++
					sequence = append(sequence, fmt.Sprintf("%s created", m.Name))
					return ctrl.Result{}, nil
				},
				overrideTriggerInPlaceUpdate: func(_ context.Context, _ *pkg.ControlPlane, m *clusterv1.Machine, _ pkg.UpToDateResult, _ bool) error {
					m.Annotations["upToDate"] = "true"
					delete(controlPlane.MachinesNotUpToDate, m.Name)
					sequence = append(sequence, fmt.Sprintf("%s updated", m.Name))
					return nil
				},
				controller: capicontrollerutil.NewFakeController(),
				recorder:   record.NewFakeRecorder(32),
			}

			minReplicas := tt.desiredReplicas + tt.maxSurge - 1
			maxReplicas := tt.desiredReplicas + tt.maxSurge
			// Verify min/max range from the beginning if we are in the range at the beginning of the rollout.
			verifyMinMax := replicasInMinMaxRange(minReplicas, maxReplicas, int32(len(controlPlane.Machines)))

			machinesNeedingRollout, _ := controlPlane.MachinesNeedingRollout()
			machinesUpToDateResults := map[string]pkg.UpToDateResult{}
			for _, m := range controlPlane.Machines {
				machinesUpToDateResults[m.Name] = pkg.UpToDateResult{EligibleForInPlaceUpdate: attrFromMachine(m).EligibleForInPlaceUpdate}
			}

			for len(machinesNeedingRollout) > 0 {
				// Execute rollingUpdate.
				res, err := r.rollingUpdate(ctx, controlPlane, machinesNeedingRollout, machinesUpToDateResults)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(res.IsZero()).To(BeTrue())

				if !tt.skipVerifyMinMax {
					// Verify that we stay in the min/max range once we are in the range.
					if verifyMinMax {
						g.Expect(replicasInMinMaxRange(minReplicas, maxReplicas, int32(len(controlPlane.Machines)))).To(BeTrue())
					} else if replicasInMinMaxRange(minReplicas, maxReplicas, int32(len(controlPlane.Machines))) {
						// Start verifying min/max as soon as we get into the min/max range.
						verifyMinMax = true
					}
				}

				// Update machinesNeedingRollout, machinesUpToDateResults for next iteration.
				// Note: The override funcs above already updated the Machine maps inside of controlPlane.
				machinesNeedingRollout, _ = controlPlane.MachinesNeedingRollout()
				machinesUpToDateResults = map[string]pkg.UpToDateResult{}
				for _, m := range controlPlane.Machines {
					machinesUpToDateResults[m.Name] = pkg.UpToDateResult{EligibleForInPlaceUpdate: attrFromMachine(m).EligibleForInPlaceUpdate}
				}
			}
			g.Expect(sequence).To(Equal(tt.wantSequence))
		})
	}
}

func replicasInMinMaxRange(minReplicas, maxReplicas, currentReplicas int32) bool {
	return currentReplicas >= minReplicas && currentReplicas <= maxReplicas
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
