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

	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime"
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
	g.Expect(machineList.Items).NotTo(BeEmpty())
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
			fmc.Machines = fmc.Machines.Insert(m)
			initObjs = append(initObjs, m.DeepCopy())
		}

		fakeClient := newFakeClient(g, initObjs...)

		r := &KubeadmControlPlaneReconciler{
			Client:            fakeClient,
			managementCluster: fmc,
			Log:               log.Log,
			recorder:          record.NewFakeRecorder(32),
		}
		controlPlane := &internal.ControlPlane{
			KCP:      kcp,
			Cluster:  cluster,
			Machines: fmc.Machines,
		}

		result, err := r.scaleUpControlPlane(context.Background(), cluster, kcp, fmc.Machines.DeepCopy(), controlPlane)
		g.Expect(result).To(Equal(ctrl.Result{Requeue: true}))
		g.Expect(err).ToNot(HaveOccurred())

		controlPlaneMachines := clusterv1.MachineList{}
		g.Expect(fakeClient.List(context.Background(), &controlPlaneMachines)).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(HaveLen(3))
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

	_, err := r.scaleDownControlPlane(context.Background(), cluster, kcp, machines, machines, controlPlane)
	g.Expect(err).ToNot(HaveOccurred())
}
