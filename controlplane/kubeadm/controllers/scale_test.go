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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestKubeadmControlPlaneReconciler_initializeControlPlane(t *testing.T) {
	g := NewWithT(t)

	cluster, kcp, genericMachineTemplate := createClusterWithControlPlane()

	fakeClient := newFakeClient(g, cluster.DeepCopy(), kcp.DeepCopy(), genericMachineTemplate.DeepCopy())

	r := &KubeadmControlPlaneReconciler{
		Client:   fakeClient,
		Log:      log.Log,
		recorder: record.NewFakeRecorder(32),
		managementClusterUncached: &fakeManagementCluster{
			Management: &internal.Management{Client: fakeClient},
			Workload:   fakeWorkloadCluster{},
		},
	}
	controlPlane := &internal.ControlPlane{
		Cluster: cluster,
		KCP:     kcp,
	}

	result, err := r.initializeControlPlane(context.Background(), cluster, kcp, controlPlane)
	g.Expect(result).To(Equal(ctrl.Result{Requeue: true}))
	g.Expect(err).NotTo(HaveOccurred())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(machineList.Items).To(HaveLen(1))

	g.Expect(machineList.Items[0].Namespace).To(Equal(cluster.Namespace))
	g.Expect(machineList.Items[0].Name).To(HavePrefix(kcp.Name))

	g.Expect(machineList.Items[0].Spec.InfrastructureRef.Namespace).To(Equal(cluster.Namespace))
	g.Expect(machineList.Items[0].Spec.InfrastructureRef.Name).To(HavePrefix(genericMachineTemplate.GetName()))
	g.Expect(machineList.Items[0].Spec.InfrastructureRef.APIVersion).To(Equal(genericMachineTemplate.GetAPIVersion()))
	g.Expect(machineList.Items[0].Spec.InfrastructureRef.Kind).To(Equal("GenericMachine"))

	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.Namespace).To(Equal(cluster.Namespace))
	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.Name).To(HavePrefix(kcp.Name))
	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.APIVersion).To(Equal(bootstrapv1.GroupVersion.String()))
	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.Kind).To(Equal("KubeadmConfig"))
}

func TestKubeadmControlPlaneReconciler_scaleUpControlPlane(t *testing.T) {
	t.Run("creates a control plane Machine if health checks pass", func(t *testing.T) {
		g := NewWithT(t)

		cluster, kcp, genericMachineTemplate := createClusterWithControlPlane()
		initObjs := []runtime.Object{cluster.DeepCopy(), kcp.DeepCopy(), genericMachineTemplate.DeepCopy()}

		fmc := &fakeManagementCluster{
			Machines:            internal.NewFilterableMachineCollection(),
			ControlPlaneHealthy: true,
			EtcdHealthy:         true,
		}

		for i := 0; i < 2; i++ {
			m, _ := createMachineNodePair(fmt.Sprintf("test-%d", i), cluster, kcp, true)
			fmc.Machines.Insert(m)
			initObjs = append(initObjs, m.DeepCopy())
		}

		fakeClient := newFakeClient(g, initObjs...)

		r := &KubeadmControlPlaneReconciler{
			Client:                    fakeClient,
			managementCluster:         fmc,
			managementClusterUncached: fmc,
			Log:                       log.Log,
			recorder:                  record.NewFakeRecorder(32),
		}
		controlPlane := &internal.ControlPlane{
			KCP:      kcp,
			Cluster:  cluster,
			Machines: fmc.Machines,
		}

		result, err := r.scaleUpControlPlane(context.Background(), cluster, kcp, controlPlane)
		g.Expect(result).To(Equal(ctrl.Result{Requeue: true}))
		g.Expect(err).ToNot(HaveOccurred())

		controlPlaneMachines := clusterv1.MachineList{}
		g.Expect(fakeClient.List(context.Background(), &controlPlaneMachines)).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(HaveLen(3))
	})
	t.Run("does not create a control plane Machine if health checks fail", func(t *testing.T) {
		cluster, kcp, genericMachineTemplate := createClusterWithControlPlane()
		initObjs := []runtime.Object{cluster.DeepCopy(), kcp.DeepCopy(), genericMachineTemplate.DeepCopy()}

		beforeMachines := internal.NewFilterableMachineCollection()
		for i := 0; i < 2; i++ {
			m, _ := createMachineNodePair(fmt.Sprintf("test-%d", i), cluster.DeepCopy(), kcp.DeepCopy(), true)
			beforeMachines.Insert(m)
			initObjs = append(initObjs, m.DeepCopy())
		}

		testCases := []struct {
			name                  string
			etcdUnHealthy         bool
			controlPlaneUnHealthy bool
			expectErr             bool
			expectResult          ctrl.Result
		}{
			{
				name:          "etcd health check fails",
				etcdUnHealthy: true,
				expectErr:     true,
				expectResult:  ctrl.Result{},
			},
			{
				name:                  "controlplane component health check fails",
				controlPlaneUnHealthy: true,
				expectErr:             false,
				expectResult:          ctrl.Result{RequeueAfter: healthCheckFailedRequeueAfter},
			},
		}
		for _, tc := range testCases {
			g := NewWithT(t)

			fakeClient := newFakeClient(g, initObjs...)
			fmc := &fakeManagementCluster{
				Machines:            beforeMachines.DeepCopy(),
				ControlPlaneHealthy: !tc.controlPlaneUnHealthy,
				EtcdHealthy:         !tc.etcdUnHealthy,
			}

			r := &KubeadmControlPlaneReconciler{
				Client:                    fakeClient,
				managementCluster:         fmc,
				managementClusterUncached: fmc,
				Log:                       log.Log,
				recorder:                  record.NewFakeRecorder(32),
			}
			controlPlane := &internal.ControlPlane{
				KCP:      kcp,
				Cluster:  cluster,
				Machines: beforeMachines,
			}

			result, err := r.scaleUpControlPlane(context.Background(), cluster.DeepCopy(), kcp.DeepCopy(), controlPlane)
			if tc.expectErr {
				g.Expect(err).To(HaveOccurred())
			}
			g.Expect(result).To(Equal(tc.expectResult))

			controlPlaneMachines := &clusterv1.MachineList{}
			g.Expect(fakeClient.List(context.Background(), controlPlaneMachines)).To(Succeed())
			g.Expect(controlPlaneMachines.Items).To(HaveLen(len(beforeMachines)))

			endMachines := internal.NewFilterableMachineCollectionFromMachineList(controlPlaneMachines)
			for _, m := range endMachines {
				bm, ok := beforeMachines[m.Name]
				bm.SetResourceVersion("1")
				g.Expect(ok).To(BeTrue())
				g.Expect(m).To(Equal(bm))
			}
		}
	})
}

func TestKubeadmControlPlaneReconciler_scaleDownControlPlane_NoError(t *testing.T) {
	g := NewWithT(t)

	machines := map[string]*clusterv1.Machine{
		"one": machine("one"),
	}

	r := &KubeadmControlPlaneReconciler{
		Log:      log.Log,
		recorder: record.NewFakeRecorder(32),
		Client:   newFakeClient(g, machines["one"]),
		managementCluster: &fakeManagementCluster{
			EtcdHealthy:         true,
			ControlPlaneHealthy: true,
		},
	}
	cluster := &clusterv1.Cluster{}
	kcp := &controlplanev1.KubeadmControlPlane{}
	controlPlane := &internal.ControlPlane{
		KCP:      kcp,
		Cluster:  cluster,
		Machines: machines,
	}

	_, err := r.scaleDownControlPlane(context.Background(), cluster, kcp, controlPlane, controlPlane.Machines)
	g.Expect(err).ToNot(HaveOccurred())
}

func TestSelectMachineForScaleDown(t *testing.T) {
	kcp := controlplanev1.KubeadmControlPlane{
		Spec: controlplanev1.KubeadmControlPlaneSpec{},
	}
	startDate := time.Date(2000, 1, 1, 1, 0, 0, 0, time.UTC)
	m1 := machine("machine-1", withFailureDomain("one"), withTimestamp(startDate.Add(time.Hour)))
	m2 := machine("machine-2", withFailureDomain("one"), withTimestamp(startDate.Add(-3*time.Hour)))
	m3 := machine("machine-3", withFailureDomain("one"), withTimestamp(startDate.Add(-4*time.Hour)))
	m4 := machine("machine-4", withFailureDomain("two"), withTimestamp(startDate.Add(-time.Hour)))
	m5 := machine("machine-5", withFailureDomain("two"), withTimestamp(startDate.Add(-2*time.Hour)))

	mc3 := internal.NewFilterableMachineCollection(m1, m2, m3, m4, m5)
	fd := clusterv1.FailureDomains{
		"one": failureDomain(true),
		"two": failureDomain(true),
	}

	needsUpgradeControlPlane := &internal.ControlPlane{
		KCP:      &kcp,
		Cluster:  &clusterv1.Cluster{Status: clusterv1.ClusterStatus{FailureDomains: fd}},
		Machines: mc3,
	}
	upToDateControlPlane := &internal.ControlPlane{
		KCP:     &kcp,
		Cluster: &clusterv1.Cluster{Status: clusterv1.ClusterStatus{FailureDomains: fd}},
		Machines: mc3.Filter(func(m *clusterv1.Machine) bool {
			return m.Name != "machine-5"
		}),
	}

	testCases := []struct {
		name             string
		cp               *internal.ControlPlane
		outDatedMachines internal.FilterableMachineCollection
		expectErr        bool
		expectedMachine  clusterv1.Machine
	}{
		{
			name:             "when there are are machines needing upgrade, it returns the oldest machine in the failure domain with the most machines needing upgrade",
			cp:               needsUpgradeControlPlane,
			outDatedMachines: internal.NewFilterableMachineCollection(m5),
			expectErr:        false,
			expectedMachine:  clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-5"}},
		},
		{
			name:             "when there are no outdated machines, it returns the oldest machine in the largest failure domain",
			cp:               upToDateControlPlane,
			outDatedMachines: internal.NewFilterableMachineCollection(),
			expectErr:        false,
			expectedMachine:  clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-3"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())

			selectedMachine, err := selectMachineForScaleDown(tc.cp, tc.outDatedMachines)

			if tc.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(tc.expectedMachine.Name).To(Equal(selectedMachine.Name))
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

func withTimestamp(t time.Time) machineOpt {
	return func(m *clusterv1.Machine) {
		m.CreationTimestamp = metav1.NewTime(t)
	}
}
