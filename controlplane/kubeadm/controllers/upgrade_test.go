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
	"testing"
	"time"

	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestKubeadmControlPlaneReconciler_upgradeControlPlane(t *testing.T) {
	g := NewWithT(t)

	cluster, kcp, genericMachineTemplate := createClusterWithControlPlane()
	kcp.Spec.Version = "v1.17.3"
	kcp.Spec.KubeadmConfigSpec.ClusterConfiguration = nil

	fakeClient := newFakeClient(g, cluster.DeepCopy(), kcp.DeepCopy(), genericMachineTemplate.DeepCopy())

	r := &KubeadmControlPlaneReconciler{
		Client:   fakeClient,
		Log:      log.Log,
		recorder: record.NewFakeRecorder(32),
		managementCluster: &fakeManagementCluster{
			Management: &internal.Management{Client: fakeClient},
			Workload:   fakeWorkloadCluster{},
		},
	}
	controlPlane := &internal.ControlPlane{
		KCP:      kcp,
		Cluster:  cluster,
		Machines: nil,
	}

	result, err := r.initializeControlPlane(context.Background(), cluster, kcp, controlPlane)
	g.Expect(result).To(Equal(ctrl.Result{Requeue: true}))
	g.Expect(err).NotTo(HaveOccurred())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(machineList.Items).NotTo(BeEmpty())
	g.Expect(machineList.Items).To(HaveLen(1))

	machineCollection := internal.NewFilterableMachineCollection(&machineList.Items[0])
	result, err = r.upgradeControlPlane(context.Background(), cluster, kcp, machineCollection, machineCollection, controlPlane)

	g.Expect(machineList.Items[0].Annotations).To(HaveKey(controlplanev1.SelectedForUpgradeAnnotation))
	g.Expect(result).To(Equal(ctrl.Result{Requeue: true}))
	g.Expect(err).ToNot(HaveOccurred())
}

func TestSelectMachineForUpgrade(t *testing.T) {
	g := NewWithT(t)

	cluster, kcp, genericMachineTemplate := createClusterWithControlPlane()
	kcp.Spec.KubeadmConfigSpec.ClusterConfiguration = nil

	m1 := machine("machine-1", withFailureDomain("one"))
	m2 := machine("machine-2", withFailureDomain("two"), withTimestamp(metav1.Time{Time: time.Date(1, 0, 0, 0, 0, 0, 0, time.UTC)}))
	m3 := machine("machine-3", withFailureDomain("two"), withTimestamp(metav1.Time{Time: time.Date(2, 0, 0, 0, 0, 0, 0, time.UTC)}))

	mc1 := internal.FilterableMachineCollection{
		"machine-1": m1,
		"machine-2": m2,
		"machine-3": m3,
	}
	fd1 := clusterv1.FailureDomains{
		"one":   failureDomain(true),
		"two":   failureDomain(true),
		"three": failureDomain(true),
		"four":  failureDomain(false),
	}

	controlPlane := &internal.ControlPlane{
		KCP:      &controlplanev1.KubeadmControlPlane{},
		Cluster:  &clusterv1.Cluster{Status: clusterv1.ClusterStatus{FailureDomains: fd1}},
		Machines: mc1,
	}

	fakeClient := newFakeClient(
		g,
		cluster.DeepCopy(),
		kcp.DeepCopy(),
		genericMachineTemplate.DeepCopy(),
		m2.DeepCopy(),
	)

	r := &KubeadmControlPlaneReconciler{
		Client:   fakeClient,
		Log:      log.Log,
		recorder: record.NewFakeRecorder(32),
		managementCluster: &fakeManagementCluster{
			Management: &internal.Management{Client: fakeClient},
			Workload:   fakeWorkloadCluster{},
		},
	}

	testCases := []struct {
		name            string
		upgradeMachines internal.FilterableMachineCollection
		cpMachines      internal.FilterableMachineCollection
		expectErr       bool
		expectedMachine clusterv1.Machine
	}{
		{
			name:            "matching controlplane machines and upgrade machines",
			upgradeMachines: mc1,
			cpMachines:      controlPlane.Machines,
			expectErr:       false,
			expectedMachine: clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-2"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())

			selectedMachine, err := r.selectMachineForUpgrade(context.Background(), cluster, controlPlane.Machines, controlPlane)

			if tc.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(selectedMachine.Name).To(Equal(tc.expectedMachine.Name))
		})
	}

}

func failureDomain(controlPlane bool) clusterv1.FailureDomainSpec {
	return clusterv1.FailureDomainSpec{
		ControlPlane: controlPlane,
	}
}

type machineOpt func(*clusterv1.Machine)

func withFailureDomain(fd string) machineOpt {
	return func(m *clusterv1.Machine) {
		m.Spec.FailureDomain = &fd
	}
}

func withTimestamp(t metav1.Time) machineOpt {
	return func(m *clusterv1.Machine) {
		m.CreationTimestamp = t
	}
}

func machine(name string, opts ...machineOpt) *clusterv1.Machine {
	m := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}
