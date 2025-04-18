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
	"k8s.io/client-go/tools/record"
	utilfeature "k8s.io/component-base/featuregate/testing"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta2"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/collections"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
)

func TestKubeadmControlPlaneReconciler_initializeControlPlane(t *testing.T) {
	setup := func(t *testing.T, g *WithT) *corev1.Namespace {
		t.Helper()

		t.Log("Creating the namespace")
		ns, err := env.CreateNamespace(ctx, "test-kcp-reconciler-initializecontrolplane")
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

	cluster, kcp, genericInfrastructureMachineTemplate := createClusterWithControlPlane(namespace.Name)
	g.Expect(env.CreateAndWait(ctx, genericInfrastructureMachineTemplate, client.FieldOwner("manager"))).To(Succeed())
	kcp.UID = types.UID(util.RandomString(10))

	r := &KubeadmControlPlaneReconciler{
		Client:   env,
		recorder: record.NewFakeRecorder(32),
		managementClusterUncached: &fakeManagementCluster{
			Management: &internal.Management{Client: env},
			Workload:   &fakeWorkloadCluster{},
		},
	}
	controlPlane := &internal.ControlPlane{
		Cluster: cluster,
		KCP:     kcp,
	}

	result, err := r.initializeControlPlane(ctx, controlPlane)
	g.Expect(result).To(BeComparableTo(ctrl.Result{Requeue: true}))
	g.Expect(err).ToNot(HaveOccurred())

	machineList := &clusterv1.MachineList{}
	g.Expect(env.GetAPIReader().List(ctx, machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(machineList.Items).To(HaveLen(1))

	res, err := collections.GetFilteredMachinesForCluster(ctx, env.GetAPIReader(), cluster, collections.OwnedMachines(kcp))
	g.Expect(res).To(HaveLen(1))
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(machineList.Items[0].Namespace).To(Equal(cluster.Namespace))
	g.Expect(machineList.Items[0].Name).To(HavePrefix(kcp.Name))

	g.Expect(machineList.Items[0].Spec.InfrastructureRef.Namespace).To(Equal(cluster.Namespace))
	g.Expect(machineList.Items[0].Spec.InfrastructureRef.Name).To(Equal(machineList.Items[0].Name))
	g.Expect(machineList.Items[0].Spec.InfrastructureRef.APIVersion).To(Equal(genericInfrastructureMachineTemplate.GetAPIVersion()))
	g.Expect(machineList.Items[0].Spec.InfrastructureRef.Kind).To(Equal("GenericInfrastructureMachine"))

	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.Namespace).To(Equal(cluster.Namespace))
	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.Name).To(Equal(machineList.Items[0].Name))
	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.APIVersion).To(Equal(bootstrapv1.GroupVersion.String()))
	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.Kind).To(Equal("KubeadmConfig"))

	kubeadmConfig := &bootstrapv1.KubeadmConfig{}
	bootstrapRef := machineList.Items[0].Spec.Bootstrap.ConfigRef
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKey{Namespace: bootstrapRef.Namespace, Name: bootstrapRef.Name}, kubeadmConfig)).To(Succeed())
	g.Expect(kubeadmConfig.Spec.ClusterConfiguration.FeatureGates).To(BeComparableTo(map[string]bool{internal.ControlPlaneKubeletLocalMode: true}))
}

func TestKubeadmControlPlaneReconciler_initializeControlPlane_Error(t *testing.T) {
	t.Run("Return error when parsing kubernetes version", func(t *testing.T) {
		setup := func(t *testing.T, g *WithT) *corev1.Namespace {
			t.Helper()

			t.Log("Creating the namespace")
			ns, err := env.CreateNamespace(ctx, "test-kcp-reconciler-initializecontrolplane")
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

		cluster, kcp, genericInfrastructureMachineTemplate := createClusterWithControlPlane(namespace.Name)
		// Try to break version
		kcp.Spec.Version = "1+foobar-0"
		g.Expect(env.CreateAndWait(ctx, genericInfrastructureMachineTemplate, client.FieldOwner("manager"))).To(Succeed())
		kcp.UID = types.UID(util.RandomString(10))

		r := &KubeadmControlPlaneReconciler{
			Client:   env,
			recorder: record.NewFakeRecorder(32),
			managementClusterUncached: &fakeManagementCluster{
				Management: &internal.Management{Client: env},
				Workload:   &fakeWorkloadCluster{},
			},
		}
		controlPlane := &internal.ControlPlane{
			Cluster: cluster,
			KCP:     kcp,
		}

		result, err := r.initializeControlPlane(ctx, controlPlane)
		g.Expect(result).To(BeComparableTo(ctrl.Result{Requeue: false}))
		g.Expect(err).To(HaveOccurred())
	})
	t.Run("Return error when cloning control plane Machine", func(t *testing.T) {
		setup := func(t *testing.T, g *WithT) *corev1.Namespace {
			t.Helper()

			t.Log("Creating the namespace")
			ns, err := env.CreateNamespace(ctx, "test-kcp-reconciler-initializecontrolplane")
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

		cluster, kcp, genericInfrastructureMachineTemplate := createClusterWithControlPlane(namespace.Name)
		// Try to break Infra Cloning
		kcp.Spec.MachineTemplate.InfrastructureRef.Name = "something_invalid"
		g.Expect(env.CreateAndWait(ctx, genericInfrastructureMachineTemplate, client.FieldOwner("manager"))).To(Succeed())
		kcp.UID = types.UID(util.RandomString(10))

		r := &KubeadmControlPlaneReconciler{
			Client:   env,
			recorder: record.NewFakeRecorder(32),
			managementClusterUncached: &fakeManagementCluster{
				Management: &internal.Management{Client: env},
				Workload:   &fakeWorkloadCluster{},
			},
		}
		controlPlane := &internal.ControlPlane{
			Cluster: cluster,
			KCP:     kcp,
		}

		result, err := r.initializeControlPlane(ctx, controlPlane)
		g.Expect(result).To(BeComparableTo(ctrl.Result{Requeue: false}))
		g.Expect(err).To(HaveOccurred())
	})
}

func TestKubeadmControlPlaneReconciler_scaleUpControlPlane(t *testing.T) {
	t.Run("creates a control plane Machine if preflight checks pass", func(t *testing.T) {
		setup := func(t *testing.T, g *WithT) *corev1.Namespace {
			t.Helper()

			t.Log("Creating the namespace")
			ns, err := env.CreateNamespace(ctx, "test-kcp-reconciler-scaleupcontrolplane")
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

		cluster, kcp, genericInfrastructureMachineTemplate := createClusterWithControlPlane(namespace.Name)
		g.Expect(env.CreateAndWait(ctx, genericInfrastructureMachineTemplate, client.FieldOwner("manager"))).To(Succeed())
		kcp.UID = types.UID(util.RandomString(10))
		setKCPHealthy(kcp)

		fmc := &fakeManagementCluster{
			Machines: collections.New(),
			Workload: &fakeWorkloadCluster{},
		}

		for i := range 2 {
			m, _ := createMachineNodePair(fmt.Sprintf("test-%d", i), cluster, kcp, true)
			setMachineHealthy(m)
			fmc.Machines.Insert(m)
		}

		r := &KubeadmControlPlaneReconciler{
			Client:                    env,
			managementCluster:         fmc,
			managementClusterUncached: fmc,
			recorder:                  record.NewFakeRecorder(32),
		}
		controlPlane := &internal.ControlPlane{
			KCP:      kcp,
			Cluster:  cluster,
			Machines: fmc.Machines,
		}

		result, err := r.scaleUpControlPlane(ctx, controlPlane)
		g.Expect(result).To(BeComparableTo(ctrl.Result{Requeue: true}))
		g.Expect(err).ToNot(HaveOccurred())

		controlPlaneMachines := clusterv1.MachineList{}
		g.Expect(env.GetAPIReader().List(ctx, &controlPlaneMachines, client.InNamespace(namespace.Name))).To(Succeed())
		// A new machine should have been created.
		// Note: expected length is 1 because only the newly created machine is on API server. Other machines are
		// in-memory only during the test.
		g.Expect(controlPlaneMachines.Items).To(HaveLen(1))

		kubeadmConfig := &bootstrapv1.KubeadmConfig{}
		bootstrapRef := controlPlaneMachines.Items[0].Spec.Bootstrap.ConfigRef
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKey{Namespace: bootstrapRef.Namespace, Name: bootstrapRef.Name}, kubeadmConfig)).To(Succeed())
		g.Expect(kubeadmConfig.Spec.ClusterConfiguration.FeatureGates).To(BeComparableTo(map[string]bool{internal.ControlPlaneKubeletLocalMode: true}))
	})
	t.Run("does not create a control plane Machine if preflight checks fail", func(t *testing.T) {
		setup := func(t *testing.T, g *WithT) *corev1.Namespace {
			t.Helper()

			t.Log("Creating the namespace")
			ns, err := env.CreateNamespace(ctx, "test-kcp-reconciler-scaleupcontrolplane")
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

		cluster, kcp, genericInfrastructureMachineTemplate := createClusterWithControlPlane(namespace.Name)
		g.Expect(env.Create(ctx, genericInfrastructureMachineTemplate, client.FieldOwner("manager"))).To(Succeed())
		kcp.UID = types.UID(util.RandomString(10))
		cluster.UID = types.UID(util.RandomString(10))
		cluster.Spec.ControlPlaneEndpoint.Host = "nodomain.example.com"
		cluster.Spec.ControlPlaneEndpoint.Port = 6443
		cluster.Status.Initialization = &clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: true}

		beforeMachines := collections.New()
		for i := range 2 {
			m, _ := createMachineNodePair(fmt.Sprintf("test-%d", i), cluster.DeepCopy(), kcp.DeepCopy(), true)
			beforeMachines.Insert(m)
		}

		fmc := &fakeManagementCluster{
			Machines: beforeMachines.DeepCopy(),
			Workload: &fakeWorkloadCluster{},
		}

		r := &KubeadmControlPlaneReconciler{
			Client:                    env,
			SecretCachingClient:       secretCachingClient,
			managementCluster:         fmc,
			managementClusterUncached: fmc,
			recorder:                  record.NewFakeRecorder(32),
		}

		controlPlane, adoptableMachineFound, err := r.initControlPlaneScope(ctx, cluster, kcp)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(adoptableMachineFound).To(BeFalse())

		result, err := r.scaleUpControlPlane(context.Background(), controlPlane)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(BeComparableTo(ctrl.Result{RequeueAfter: preflightFailedRequeueAfter}))

		// scaleUpControlPlane is never called due to health check failure and new machine is not created to scale up.
		controlPlaneMachines := &clusterv1.MachineList{}
		g.Expect(env.GetAPIReader().List(context.Background(), controlPlaneMachines, client.InNamespace(namespace.Name))).To(Succeed())
		// No new machine should be created.
		// Note: expected length is 0 because no machine is created and hence no machine is on the API server.
		// Other machines are in-memory only during the test.
		g.Expect(controlPlaneMachines.Items).To(BeEmpty())

		endMachines := collections.FromMachineList(controlPlaneMachines)
		for _, m := range endMachines {
			bm, ok := beforeMachines[m.Name]
			g.Expect(ok).To(BeTrue())
			g.Expect(m).To(BeComparableTo(bm))
		}
	})
	t.Run("Return error when parsing kubernetes version", func(t *testing.T) {
		setup := func(t *testing.T, g *WithT) *corev1.Namespace {
			t.Helper()

			t.Log("Creating the namespace")
			ns, err := env.CreateNamespace(ctx, "test-kcp-reconciler-scaleupcontrolplane")
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

		cluster, kcp, genericInfrastructureMachineTemplate := createClusterWithControlPlane(namespace.Name)
		// Try to break version
		kcp.Spec.Version = "1+foobar-0"
		g.Expect(env.CreateAndWait(ctx, genericInfrastructureMachineTemplate, client.FieldOwner("manager"))).To(Succeed())
		kcp.UID = types.UID(util.RandomString(10))
		setKCPHealthy(kcp)

		fmc := &fakeManagementCluster{
			Machines: collections.New(),
			Workload: &fakeWorkloadCluster{},
		}

		for i := range 2 {
			m, _ := createMachineNodePair(fmt.Sprintf("test-%d", i), cluster, kcp, true)
			setMachineHealthy(m)
			fmc.Machines.Insert(m)
		}

		r := &KubeadmControlPlaneReconciler{
			Client:                    env,
			managementCluster:         fmc,
			managementClusterUncached: fmc,
			recorder:                  record.NewFakeRecorder(32),
		}
		controlPlane := &internal.ControlPlane{
			KCP:      kcp,
			Cluster:  cluster,
			Machines: fmc.Machines,
		}

		result, err := r.scaleUpControlPlane(ctx, controlPlane)
		g.Expect(result).To(BeComparableTo(ctrl.Result{Requeue: false}))
		g.Expect(err).To(HaveOccurred())
	})
	t.Run("Return error when cloning control plane Machine", func(t *testing.T) {
		setup := func(t *testing.T, g *WithT) *corev1.Namespace {
			t.Helper()

			t.Log("Creating the namespace")
			ns, err := env.CreateNamespace(ctx, "test-kcp-reconciler-scaleupcontrolplane")
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

		cluster, kcp, genericInfrastructureMachineTemplate := createClusterWithControlPlane(namespace.Name)
		// Try to break Infra Cloning
		kcp.Spec.MachineTemplate.InfrastructureRef.Name = "something_invalid"
		g.Expect(env.CreateAndWait(ctx, genericInfrastructureMachineTemplate, client.FieldOwner("manager"))).To(Succeed())
		kcp.UID = types.UID(util.RandomString(10))
		setKCPHealthy(kcp)

		fmc := &fakeManagementCluster{
			Machines: collections.New(),
			Workload: &fakeWorkloadCluster{},
		}

		for i := range 2 {
			m, _ := createMachineNodePair(fmt.Sprintf("test-%d", i), cluster, kcp, true)
			setMachineHealthy(m)
			fmc.Machines.Insert(m)
		}

		r := &KubeadmControlPlaneReconciler{
			Client:                    env,
			managementCluster:         fmc,
			managementClusterUncached: fmc,
			recorder:                  record.NewFakeRecorder(32),
		}
		controlPlane := &internal.ControlPlane{
			KCP:      kcp,
			Cluster:  cluster,
			Machines: fmc.Machines,
		}

		result, err := r.scaleUpControlPlane(ctx, controlPlane)
		g.Expect(result).To(BeComparableTo(ctrl.Result{Requeue: false}))
		g.Expect(err).To(HaveOccurred())
	})
}

func TestKubeadmControlPlaneReconciler_scaleDownControlPlane_Error(t *testing.T) {
	t.Run("Return error failed to select machine for scale down", func(t *testing.T) {
		g := NewWithT(t)
		fakeClient := newFakeClient()

		r := &KubeadmControlPlaneReconciler{
			recorder:            record.NewFakeRecorder(32),
			Client:              fakeClient,
			SecretCachingClient: fakeClient,
			managementCluster: &fakeManagementCluster{
				Workload: &fakeWorkloadCluster{},
			},
		}

		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				Version: "v1.19.1",
			},
		}
		setKCPHealthy(kcp)
		controlPlane := &internal.ControlPlane{
			KCP:     kcp,
			Cluster: &clusterv1.Cluster{},
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		result, err := r.scaleDownControlPlane(context.Background(), controlPlane, controlPlane.Machines)
		g.Expect(err).To(HaveOccurred())
		g.Expect(result).To(BeComparableTo(ctrl.Result{Requeue: false}))
	})
	t.Run("Return error when deleting control plane machine", func(t *testing.T) {
		g := NewWithT(t)

		machines := map[string]*clusterv1.Machine{
			"two":   machine("two", withTimestamp(time.Now())),
			"three": machine("three", withTimestamp(time.Now())),
		}
		setMachineHealthy(machines["two"])
		setMachineHealthy(machines["three"])
		fakeClient := newFakeClient(machines["two"], machines["three"])

		r := &KubeadmControlPlaneReconciler{
			recorder: record.NewFakeRecorder(32),
			Client: &fClient{Client: fakeClient,
				deleteError: errors.New("delete error")},
			SecretCachingClient: fakeClient,
			managementCluster: &fakeManagementCluster{
				Workload: &fakeWorkloadCluster{},
			},
		}

		cluster := &clusterv1.Cluster{}
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				Version: "v1.19.1",
			},
		}
		controlPlane := &internal.ControlPlane{
			KCP:      kcp,
			Cluster:  cluster,
			Machines: machines,
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		result, err := r.scaleDownControlPlane(context.Background(), controlPlane, machines)
		g.Expect(err).To(HaveOccurred())
		g.Expect(result).To(BeComparableTo(ctrl.Result{Requeue: false}))
	})

	t.Run("Return error when create client for workload cluster", func(t *testing.T) {
		g := NewWithT(t)

		machines := map[string]*clusterv1.Machine{
			"two":   machine("two", withTimestamp(time.Now())),
			"three": machine("three", withTimestamp(time.Now())),
		}
		setMachineHealthy(machines["two"])
		setMachineHealthy(machines["three"])
		fakeClient := newFakeClient(machines["two"], machines["three"])

		r := &KubeadmControlPlaneReconciler{
			recorder:            record.NewFakeRecorder(32),
			Client:              &fClient{Client: fakeClient},
			SecretCachingClient: fakeClient,
			managementCluster: &fakeManagementCluster{
				Workload:    &fakeWorkloadCluster{},
				WorkloadErr: errors.New("get error"),
			},
		}

		cluster := &clusterv1.Cluster{}
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				Version: "v1.19.1",
			},
		}
		controlPlane := &internal.ControlPlane{
			KCP:      kcp,
			Cluster:  cluster,
			Machines: machines,
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		result, err := r.scaleDownControlPlane(context.Background(), controlPlane, machines)
		g.Expect(err).To(HaveOccurred())
		g.Expect(result).To(BeComparableTo(ctrl.Result{Requeue: false}))
	})
}

func TestKubeadmControlPlaneReconciler_scaleDownControlPlane_NoError(t *testing.T) {
	t.Run("deletes control plane Machine if preflight checks pass", func(t *testing.T) {
		g := NewWithT(t)

		machines := map[string]*clusterv1.Machine{
			"one": machine("one"),
		}
		setMachineHealthy(machines["one"])
		fakeClient := newFakeClient(machines["one"])

		r := &KubeadmControlPlaneReconciler{
			recorder:            record.NewFakeRecorder(32),
			Client:              fakeClient,
			SecretCachingClient: fakeClient,
			managementCluster: &fakeManagementCluster{
				Workload: &fakeWorkloadCluster{},
			},
		}

		cluster := &clusterv1.Cluster{}
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				Version: "v1.19.1",
			},
		}
		setKCPHealthy(kcp)
		controlPlane := &internal.ControlPlane{
			KCP:      kcp,
			Cluster:  cluster,
			Machines: machines,
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		result, err := r.scaleDownControlPlane(context.Background(), controlPlane, controlPlane.Machines)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(BeComparableTo(ctrl.Result{Requeue: true}))

		controlPlaneMachines := clusterv1.MachineList{}
		g.Expect(fakeClient.List(context.Background(), &controlPlaneMachines)).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(BeEmpty())
	})
	t.Run("deletes the oldest control plane Machine even if preflight checks fails", func(t *testing.T) {
		g := NewWithT(t)

		machines := map[string]*clusterv1.Machine{
			"one":   machine("one", withTimestamp(time.Now().Add(-1*time.Minute))),
			"two":   machine("two", withTimestamp(time.Now())),
			"three": machine("three", withTimestamp(time.Now())),
		}
		setMachineHealthy(machines["two"])
		setMachineHealthy(machines["three"])
		fakeClient := newFakeClient(machines["one"], machines["two"], machines["three"])

		r := &KubeadmControlPlaneReconciler{
			recorder:            record.NewFakeRecorder(32),
			Client:              fakeClient,
			SecretCachingClient: fakeClient,
			managementCluster: &fakeManagementCluster{
				Workload: &fakeWorkloadCluster{},
			},
		}

		cluster := &clusterv1.Cluster{}
		kcp := &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{
				Version: "v1.19.1",
			},
		}
		controlPlane := &internal.ControlPlane{
			KCP:      kcp,
			Cluster:  cluster,
			Machines: machines,
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		result, err := r.scaleDownControlPlane(context.Background(), controlPlane, controlPlane.Machines)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(BeComparableTo(ctrl.Result{Requeue: true}))

		controlPlaneMachines := clusterv1.MachineList{}
		g.Expect(fakeClient.List(context.Background(), &controlPlaneMachines)).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(HaveLen(2))
	})

	t.Run("does not scale down if preflight checks fail on any machine other than the one being deleted", func(t *testing.T) {
		g := NewWithT(t)

		machines := map[string]*clusterv1.Machine{
			"one":   machine("one", withTimestamp(time.Now().Add(-1*time.Minute))),
			"two":   machine("two", withTimestamp(time.Now())),
			"three": machine("three", withTimestamp(time.Now())),
		}
		setMachineHealthy(machines["three"])
		fakeClient := newFakeClient(machines["one"], machines["two"], machines["three"])

		r := &KubeadmControlPlaneReconciler{
			recorder:            record.NewFakeRecorder(32),
			Client:              fakeClient,
			SecretCachingClient: fakeClient,
			managementCluster: &fakeManagementCluster{
				Workload: &fakeWorkloadCluster{},
			},
		}

		cluster := &clusterv1.Cluster{}
		kcp := &controlplanev1.KubeadmControlPlane{}
		controlPlane := &internal.ControlPlane{
			KCP:      kcp,
			Cluster:  cluster,
			Machines: machines,
		}
		controlPlane.InjectTestManagementCluster(r.managementCluster)

		result, err := r.scaleDownControlPlane(context.Background(), controlPlane, controlPlane.Machines)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(BeComparableTo(ctrl.Result{RequeueAfter: preflightFailedRequeueAfter}))

		controlPlaneMachines := clusterv1.MachineList{}
		g.Expect(fakeClient.List(context.Background(), &controlPlaneMachines)).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(HaveLen(3))
	})
}

func TestSelectMachineForScaleDown(t *testing.T) {
	kcp := controlplanev1.KubeadmControlPlane{
		Spec: controlplanev1.KubeadmControlPlaneSpec{},
	}
	startDate := time.Date(2000, 1, 1, 1, 0, 0, 0, time.UTC)
	m1 := machine("machine-1", withFailureDomain("one"), withTimestamp(startDate.Add(time.Hour)), machineOpt(withNodeRef("machine-1")))
	m2 := machine("machine-2", withFailureDomain("one"), withTimestamp(startDate.Add(-3*time.Hour)), machineOpt(withNodeRef("machine-2")))
	m3 := machine("machine-3", withFailureDomain("one"), withTimestamp(startDate.Add(-4*time.Hour)), machineOpt(withNodeRef("machine-3")))
	m4 := machine("machine-4", withFailureDomain("two"), withTimestamp(startDate.Add(-time.Hour)), machineOpt(withNodeRef("machine-4")))
	m5 := machine("machine-5", withFailureDomain("two"), withTimestamp(startDate.Add(-2*time.Hour)), machineOpt(withNodeRef("machine-5")))
	m6 := machine("machine-6", withFailureDomain("two"), withTimestamp(startDate.Add(-7*time.Hour)), machineOpt(withNodeRef("machine-6")))
	m7 := machine("machine-7", withFailureDomain("two"), withTimestamp(startDate.Add(-5*time.Hour)),
		withAnnotation("cluster.x-k8s.io/delete-machine"), machineOpt(withNodeRef("machine-7")))
	m8 := machine("machine-8", withFailureDomain("two"), withTimestamp(startDate.Add(-6*time.Hour)),
		withAnnotation("cluster.x-k8s.io/delete-machine"), machineOpt(withNodeRef("machine-8")))
	m9 := machine("machine-9", withFailureDomain("two"), withTimestamp(startDate.Add(-5*time.Hour)),
		machineOpt(withNodeRef("machine-9")))
	m10 := machine("machine-10", withFailureDomain("two"), withTimestamp(startDate.Add(-4*time.Hour)),
		machineOpt(withNodeRef("machine-10")), machineOpt(withUnhealthyAPIServerPod()))
	m11 := machine("machine-11", withFailureDomain("two"), withTimestamp(startDate.Add(-3*time.Hour)),
		machineOpt(withNodeRef("machine-11")), machineOpt(withUnhealthyEtcdMember()))

	mc3 := collections.FromMachines(m1, m2, m3, m4, m5)
	mc6 := collections.FromMachines(m6, m7, m8)
	mc9 := collections.FromMachines(m9, m10, m11)
	fd := clusterv1.FailureDomains{
		"one": failureDomain(true),
		"two": failureDomain(true),
	}

	needsUpgradeControlPlane := &internal.ControlPlane{
		KCP:      &kcp,
		Cluster:  &clusterv1.Cluster{Status: clusterv1.ClusterStatus{FailureDomains: fd}},
		Machines: mc3,
	}
	needsUpgradeControlPlane1 := &internal.ControlPlane{
		KCP:      &kcp,
		Cluster:  &clusterv1.Cluster{Status: clusterv1.ClusterStatus{FailureDomains: fd}},
		Machines: mc9,
	}
	upToDateControlPlane := &internal.ControlPlane{
		KCP:     &kcp,
		Cluster: &clusterv1.Cluster{Status: clusterv1.ClusterStatus{FailureDomains: fd}},
		Machines: mc3.Filter(func(m *clusterv1.Machine) bool {
			return m.Name != "machine-5"
		}),
	}
	annotatedControlPlane := &internal.ControlPlane{
		KCP:      &kcp,
		Cluster:  &clusterv1.Cluster{Status: clusterv1.ClusterStatus{FailureDomains: fd}},
		Machines: mc6,
	}

	testCases := []struct {
		name             string
		cp               *internal.ControlPlane
		outDatedMachines collections.Machines
		expectErr        bool
		expectedMachine  clusterv1.Machine
	}{
		{
			name:             "when there are machines needing upgrade, it returns the oldest machine in the failure domain with the most machines needing upgrade",
			cp:               needsUpgradeControlPlane,
			outDatedMachines: collections.FromMachines(m5),
			expectErr:        false,
			expectedMachine:  clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-5"}},
		},
		{
			name:             "when there are no outdated machines, it returns the oldest machine in the largest failure domain",
			cp:               upToDateControlPlane,
			outDatedMachines: collections.New(),
			expectErr:        false,
			expectedMachine:  clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-3"}},
		},
		{
			name:             "when there is a single machine marked with delete annotation key in machine collection, it returns only that marked machine",
			cp:               annotatedControlPlane,
			outDatedMachines: collections.FromMachines(m6, m7),
			expectErr:        false,
			expectedMachine:  clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-7"}},
		},
		{
			name:             "when there are machines marked with delete annotation key in machine collection, it returns the oldest marked machine first",
			cp:               annotatedControlPlane,
			outDatedMachines: collections.FromMachines(m7, m8),
			expectErr:        false,
			expectedMachine:  clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-8"}},
		},
		{
			name:             "when there are annotated machines which are part of the annotatedControlPlane but not in outdatedMachines, it returns the oldest marked machine first",
			cp:               annotatedControlPlane,
			outDatedMachines: collections.New(),
			expectErr:        false,
			expectedMachine:  clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-8"}},
		},
		{
			name:             "when there are machines needing upgrade, it returns the oldest machine in the failure domain with the most machines needing upgrade",
			cp:               needsUpgradeControlPlane,
			outDatedMachines: collections.FromMachines(m7, m3),
			expectErr:        false,
			expectedMachine:  clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-7"}},
		},
		{
			name:             "when there is an up to date machine with delete annotation, while there are any outdated machines without annotation that still exist, it returns oldest marked machine first",
			cp:               upToDateControlPlane,
			outDatedMachines: collections.FromMachines(m5, m3, m8, m7, m6, m1, m2),
			expectErr:        false,
			expectedMachine:  clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-8"}},
		},
		{
			name:             "when there are machines needing upgrade, it returns the single unhealthy machine with MachineAPIServerPodHealthyCondition set to False",
			cp:               needsUpgradeControlPlane1,
			outDatedMachines: collections.FromMachines(m9, m10),
			expectErr:        false,
			expectedMachine:  clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-10"}},
		},
		{
			name:             "when there are machines needing upgrade, it returns the oldest unhealthy machine with MachineEtcdMemberHealthyCondition set to False",
			cp:               needsUpgradeControlPlane1,
			outDatedMachines: collections.FromMachines(m9, m10, m11),
			expectErr:        false,
			expectedMachine:  clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-10"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			selectedMachine, err := selectMachineForScaleDown(ctx, tc.cp, tc.outDatedMachines)

			if tc.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(tc.expectedMachine.Name).To(Equal(selectedMachine.Name))
		})
	}
}

func TestPreflightChecks(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	testCases := []struct {
		name            string
		cluster         *clusterv1.Cluster
		kcp             *controlplanev1.KubeadmControlPlane
		machines        []*clusterv1.Machine
		expectResult    ctrl.Result
		expectPreflight internal.PreflightCheckResults
	}{
		{
			name:         "control plane without machines (not initialized) should pass",
			kcp:          &controlplanev1.KubeadmControlPlane{},
			expectResult: ctrl.Result{},
			expectPreflight: internal.PreflightCheckResults{
				HasDeletingMachine:               false,
				ControlPlaneComponentsNotHealthy: false,
				EtcdClusterNotHealthy:            false,
				TopologyVersionMismatch:          false,
			},
		},
		{
			name: "control plane with a pending upgrade should requeue",
			cluster: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{
						Version: "v1.33.0",
					},
				},
			},
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.32.0",
				},
			},
			machines: []*clusterv1.Machine{
				{},
			},

			expectResult: ctrl.Result{RequeueAfter: preflightFailedRequeueAfter},
			expectPreflight: internal.PreflightCheckResults{
				HasDeletingMachine:               false,
				ControlPlaneComponentsNotHealthy: false,
				EtcdClusterNotHealthy:            false,
				TopologyVersionMismatch:          true,
			},
		},
		{
			name: "control plane with a deleting machine should requeue",
			kcp:  &controlplanev1.KubeadmControlPlane{},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
					},
				},
			},
			expectResult: ctrl.Result{RequeueAfter: deleteRequeueAfter},
			expectPreflight: internal.PreflightCheckResults{
				HasDeletingMachine:               true,
				ControlPlaneComponentsNotHealthy: false,
				EtcdClusterNotHealthy:            false,
				TopologyVersionMismatch:          false,
			},
		},
		{
			name: "control plane without a nodeRef should requeue",
			kcp:  &controlplanev1.KubeadmControlPlane{},
			machines: []*clusterv1.Machine{
				{
					Status: clusterv1.MachineStatus{
						NodeRef: nil,
						// Note: with v1beta1 no conditions are applied to machine when NodeRef is not set, this will change with v1beta2.
					},
				},
			},
			expectResult: ctrl.Result{RequeueAfter: preflightFailedRequeueAfter},
			expectPreflight: internal.PreflightCheckResults{
				HasDeletingMachine:               false,
				ControlPlaneComponentsNotHealthy: true,
				EtcdClusterNotHealthy:            true,
				TopologyVersionMismatch:          false,
			},
		},
		{
			name: "control plane with an unhealthy machine condition should requeue",
			kcp:  &controlplanev1.KubeadmControlPlane{},
			machines: []*clusterv1.Machine{
				{
					Status: clusterv1.MachineStatus{
						NodeRef: &corev1.ObjectReference{
							Kind: "Node",
							Name: "node-1",
						},
						Deprecated: &clusterv1.MachineDeprecatedStatus{V1Beta1: &clusterv1.MachineV1Beta1DeprecatedStatus{
							Conditions: clusterv1.Conditions{
								*v1beta1conditions.FalseCondition(controlplanev1.MachineAPIServerPodHealthyV1Beta1Condition, "fooReason", clusterv1.ConditionSeverityError, ""),
								*v1beta1conditions.TrueCondition(controlplanev1.MachineControllerManagerPodHealthyV1Beta1Condition),
								*v1beta1conditions.TrueCondition(controlplanev1.MachineSchedulerPodHealthyV1Beta1Condition),
								*v1beta1conditions.TrueCondition(controlplanev1.MachineEtcdPodHealthyV1Beta1Condition),
								*v1beta1conditions.TrueCondition(controlplanev1.MachineEtcdMemberHealthyV1Beta1Condition),
							},
						}},
					},
				},
			},
			expectResult: ctrl.Result{RequeueAfter: preflightFailedRequeueAfter},
			expectPreflight: internal.PreflightCheckResults{
				HasDeletingMachine:               false,
				ControlPlaneComponentsNotHealthy: true,
				EtcdClusterNotHealthy:            false,
				TopologyVersionMismatch:          false,
			},
		},
		{
			name: "control plane with an unhealthy machine condition should requeue",
			kcp:  &controlplanev1.KubeadmControlPlane{},
			machines: []*clusterv1.Machine{
				{
					Status: clusterv1.MachineStatus{
						NodeRef: &corev1.ObjectReference{
							Kind: "Node",
							Name: "node-1",
						},
						Deprecated: &clusterv1.MachineDeprecatedStatus{V1Beta1: &clusterv1.MachineV1Beta1DeprecatedStatus{
							Conditions: clusterv1.Conditions{
								*v1beta1conditions.TrueCondition(controlplanev1.MachineAPIServerPodHealthyV1Beta1Condition),
								*v1beta1conditions.TrueCondition(controlplanev1.MachineControllerManagerPodHealthyV1Beta1Condition),
								*v1beta1conditions.TrueCondition(controlplanev1.MachineSchedulerPodHealthyV1Beta1Condition),
								*v1beta1conditions.TrueCondition(controlplanev1.MachineEtcdPodHealthyV1Beta1Condition),
								*v1beta1conditions.FalseCondition(controlplanev1.MachineEtcdMemberHealthyV1Beta1Condition, "fooReason", clusterv1.ConditionSeverityError, ""),
							},
						}},
					},
				},
			},
			expectResult: ctrl.Result{RequeueAfter: preflightFailedRequeueAfter},
			expectPreflight: internal.PreflightCheckResults{
				HasDeletingMachine:               false,
				ControlPlaneComponentsNotHealthy: false,
				EtcdClusterNotHealthy:            true,
				TopologyVersionMismatch:          false,
			},
		},
		{
			name: "control plane with an healthy machine and an healthy kcp condition should pass",
			kcp: &controlplanev1.KubeadmControlPlane{
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Deprecated: &controlplanev1.KubeadmControlPlaneDeprecatedStatus{V1Beta1: &controlplanev1.KubeadmControlPlaneV1Beta1DeprecatedStatus{
						Conditions: clusterv1.Conditions{
							*v1beta1conditions.TrueCondition(controlplanev1.ControlPlaneComponentsHealthyV1Beta1Condition),
							*v1beta1conditions.TrueCondition(controlplanev1.EtcdClusterHealthyV1Beta1Condition),
						},
					}},
				},
			},
			machines: []*clusterv1.Machine{
				{
					Status: clusterv1.MachineStatus{
						NodeRef: &corev1.ObjectReference{
							Kind: "Node",
							Name: "node-1",
						},
						Deprecated: &clusterv1.MachineDeprecatedStatus{V1Beta1: &clusterv1.MachineV1Beta1DeprecatedStatus{
							Conditions: clusterv1.Conditions{
								*v1beta1conditions.TrueCondition(controlplanev1.MachineAPIServerPodHealthyV1Beta1Condition),
								*v1beta1conditions.TrueCondition(controlplanev1.MachineControllerManagerPodHealthyV1Beta1Condition),
								*v1beta1conditions.TrueCondition(controlplanev1.MachineSchedulerPodHealthyV1Beta1Condition),
								*v1beta1conditions.TrueCondition(controlplanev1.MachineEtcdPodHealthyV1Beta1Condition),
								*v1beta1conditions.TrueCondition(controlplanev1.MachineEtcdMemberHealthyV1Beta1Condition),
							},
						}},
					},
				},
			},
			expectResult: ctrl.Result{},
			expectPreflight: internal.PreflightCheckResults{
				HasDeletingMachine:               false,
				ControlPlaneComponentsNotHealthy: false,
				EtcdClusterNotHealthy:            false,
				TopologyVersionMismatch:          false,
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			r := &KubeadmControlPlaneReconciler{
				recorder: record.NewFakeRecorder(32),
			}
			cluster := &clusterv1.Cluster{}
			if tt.cluster != nil {
				cluster = tt.cluster
			}
			controlPlane := &internal.ControlPlane{
				Cluster:  cluster,
				KCP:      tt.kcp,
				Machines: collections.FromMachines(tt.machines...),
			}
			result, err := r.preflightChecks(context.TODO(), controlPlane)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(result).To(BeComparableTo(tt.expectResult))
			g.Expect(controlPlane.PreflightCheckResults).To(Equal(tt.expectPreflight))
		})
	}
}

func TestPreflightCheckCondition(t *testing.T) {
	condition := clusterv1.ConditionType("fooCondition")
	testCases := []struct {
		name      string
		machine   *clusterv1.Machine
		expectErr bool
	}{
		{
			name:      "missing condition should return error",
			machine:   &clusterv1.Machine{},
			expectErr: true,
		},
		{
			name: "false condition should return error",
			machine: &clusterv1.Machine{
				Status: clusterv1.MachineStatus{
					Deprecated: &clusterv1.MachineDeprecatedStatus{V1Beta1: &clusterv1.MachineV1Beta1DeprecatedStatus{
						Conditions: clusterv1.Conditions{
							*v1beta1conditions.FalseCondition(condition, "fooReason", clusterv1.ConditionSeverityError, ""),
						},
					}},
				},
			},
			expectErr: true,
		},
		{
			name: "unknown condition should return error",
			machine: &clusterv1.Machine{
				Status: clusterv1.MachineStatus{
					Deprecated: &clusterv1.MachineDeprecatedStatus{V1Beta1: &clusterv1.MachineV1Beta1DeprecatedStatus{
						Conditions: clusterv1.Conditions{
							*v1beta1conditions.UnknownCondition(condition, "fooReason", ""),
						},
					}},
				},
			},
			expectErr: true,
		},
		{
			name: "true condition should not return error",
			machine: &clusterv1.Machine{
				Status: clusterv1.MachineStatus{
					Deprecated: &clusterv1.MachineDeprecatedStatus{V1Beta1: &clusterv1.MachineV1Beta1DeprecatedStatus{
						Conditions: clusterv1.Conditions{
							*v1beta1conditions.TrueCondition(condition),
						},
					}},
				},
			},
			expectErr: false,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := preflightCheckCondition("machine", tt.machine, condition)

			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

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

func withAnnotation(annotation string) machineOpt {
	return func(m *clusterv1.Machine) {
		m.ObjectMeta.Annotations = map[string]string{annotation: ""}
	}
}

func withLabels(labels map[string]string) machineOpt {
	return func(m *clusterv1.Machine) {
		m.ObjectMeta.Labels = labels
	}
}

func withTimestamp(t time.Time) machineOpt {
	return func(m *clusterv1.Machine) {
		m.CreationTimestamp = metav1.NewTime(t)
	}
}

type fClient struct {
	client.Client
	getError    error
	listError   error
	deleteError error
}

func (fc *fClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if fc.getError != nil {
		return fc.getError
	}
	return fc.Client.Get(ctx, key, obj, opts...)
}

func (fc *fClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if fc.listError != nil {
		return fc.listError
	}
	return fc.Client.List(ctx, list, opts...)
}

func (fc *fClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if fc.deleteError != nil {
		return fc.deleteError
	}
	return fc.Client.Delete(ctx, obj, opts...)
}
