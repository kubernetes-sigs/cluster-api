/*
Copyright 2026 The Kubernetes Authors.

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
	"errors"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
	dockerbackend "sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/controllers/backends/docker"
	capicontrollerutil "sigs.k8s.io/cluster-api/util/controller"
)

func TestDevMachineReconciler_ActiveMissingDevClusterFailsClosed(t *testing.T) {
	g := NewWithT(t)
	s := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(s)).To(Succeed())
	g.Expect(infrav1.AddToScheme(s)).To(Succeed())

	const namespace = "default"
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: namespace},
		Spec: clusterv1.ClusterSpec{
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: infrav1.GroupVersion.Group,
				Kind:     "DevCluster",
				Name:     "test-cluster",
			},
		},
	}
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: namespace,
			Labels:    map[string]string{clusterv1.ClusterNameLabel: cluster.Name},
		},
	}
	devMachine := &infrav1.DevMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      machine.Name,
			Namespace: namespace,
			Labels:    map[string]string{clusterv1.ClusterNameLabel: cluster.Name},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: clusterv1.GroupVersion.String(),
				Kind:       "Machine",
				Name:       machine.Name,
			}},
		},
		Spec: infrav1.DevMachineSpec{
			Backend: infrav1.DevMachineBackendSpec{
				Docker: &infrav1.DockerMachineBackendSpec{},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(devMachine).WithObjects(cluster, machine, devMachine).Build()
	r := &DevMachineReconciler{
		Client:           c,
		ContainerRuntime: &container.FakeRuntime{},
	}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Name: devMachine.Name, Namespace: namespace}}

	_, err := r.Reconcile(context.Background(), request)
	g.Expect(err).NotTo(HaveOccurred())
	_, err = r.Reconcile(context.Background(), request)
	g.Expect(err).NotTo(HaveOccurred())

	updated := &infrav1.DevMachine{}
	g.Expect(c.Get(context.Background(), clientKey(devMachine), updated)).To(Succeed())
	g.Expect(updated.Finalizers).To(ConsistOf(infrav1.MachineFinalizer))
}

func TestDevMachineReconciler_DeletingMissingDevClusterDoesNotReturnDependencyError(t *testing.T) {
	g := NewWithT(t)
	s := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(s)).To(Succeed())
	g.Expect(infrav1.AddToScheme(s)).To(Succeed())

	const namespace = "default"
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "delete-cluster", Namespace: namespace},
		Spec: clusterv1.ClusterSpec{
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: infrav1.GroupVersion.Group,
				Kind:     "DevCluster",
				Name:     "delete-cluster",
			},
		},
	}
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "delete-machine",
			Namespace: namespace,
			Labels:    map[string]string{clusterv1.ClusterNameLabel: cluster.Name},
		},
	}
	devMachine := &infrav1.DevMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:              machine.Name,
			Namespace:         namespace,
			Labels:            map[string]string{clusterv1.ClusterNameLabel: cluster.Name},
			Finalizers:        []string{infrav1.MachineFinalizer},
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: clusterv1.GroupVersion.String(),
				Kind:       "Machine",
				Name:       machine.Name,
			}},
		},
		Spec: infrav1.DevMachineSpec{
			Backend: infrav1.DevMachineBackendSpec{
				Docker: &infrav1.DockerMachineBackendSpec{},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(devMachine).WithObjects(cluster, machine, devMachine).Build()
	r := &DevMachineReconciler{
		Client:                   c,
		controller:               &capicontrollerutil.FakeController{},
		ContainerRuntime:         &container.FakeRuntime{},
		DockerMachineTaskManager: dockerbackend.NewTaskManager(),
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: clientKey(devMachine)})
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(c.Get(context.Background(), clientKey(devMachine), &infrav1.DevMachine{})).To(Succeed())
}

func TestDockerMachineBackend_ReconcileDeleteMissingDevCluster(t *testing.T) {
	g := NewWithT(t)
	runtimeClient := &container.FakeRuntime{}
	ctx := container.RuntimeInto(context.Background(), runtimeClient)
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "default"},
	}
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: cluster.Namespace,
			Labels:    map[string]string{clusterv1.MachineControlPlaneLabel: ""},
		},
	}
	devMachine := &infrav1.DevMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:       machine.Name,
			Namespace:  machine.Namespace,
			Finalizers: []string{infrav1.MachineFinalizer},
		},
		Spec: infrav1.DevMachineSpec{
			Backend: infrav1.DevMachineBackendSpec{
				Docker: &infrav1.DockerMachineBackendSpec{},
			},
		},
	}
	r := &dockerbackend.MachineBackendReconciler{
		ContainerRuntime: runtimeClient,
		TaskManager:      dockerbackend.NewTaskManager(),
	}

	_, err := r.ReconcileDelete(ctx, cluster, nil, machine, devMachine)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(devMachine.Finalizers).To(BeEmpty())
	g.Expect(runtimeClient.DeleteContainerCalls()).To(BeEmpty())

	_, err = r.ReconcileDelete(ctx, cluster, nil, machine, devMachine)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(devMachine.Finalizers).To(BeEmpty())
}

func TestDockerMachineBackend_ReconcileDeleteMissingDevClusterUsesExpectedContainer(t *testing.T) {
	tests := []struct {
		name             string
		machineName      string
		devMachineName   string
		machinePoolOwned bool
	}{
		{
			name:             "machine pool",
			machineName:      "generated-machine",
			devMachineName:   "pool-container",
			machinePoolOwned: true,
		},
		{
			name:             "regular machine",
			machineName:      "regular-machine",
			devMachineName:   "different-devmachine",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			runtimeClient := &recordingContainerRuntime{
				FakeRuntime: &container.FakeRuntime{},
			}
			runtimeClient.ResetDeleteContainerCallLogs()
			cluster := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "default"},
			}
			machine := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.machineName,
					Namespace: cluster.Namespace,
				},
			}
			if tt.machinePoolOwned {
				machine.Labels = map[string]string{clusterv1.MachinePoolNameLabel: "test-pool"}
			}
			devMachine := &infrav1.DevMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:       tt.devMachineName,
					Namespace:  machine.Namespace,
					Finalizers: []string{infrav1.MachineFinalizer},
					Labels:     machine.Labels,
				},
				Spec: infrav1.DevMachineSpec{
					Backend: infrav1.DevMachineBackendSpec{
						Docker: &infrav1.DockerMachineBackendSpec{},
					},
				},
			}
			expectedMachineName := tt.machineName
			if tt.machinePoolOwned {
				expectedMachineName = tt.devMachineName
			}
			expectedContainerName := fmt.Sprintf("%s-%s", cluster.Name, expectedMachineName)
			runtimeClient.containers = []container.Container{{Name: expectedContainerName, Status: "Up 1 second"}}

			r := &dockerbackend.MachineBackendReconciler{
				ContainerRuntime: runtimeClient,
				TaskManager:      dockerbackend.NewTaskManager(),
			}

			_, err := r.ReconcileDelete(
				container.RuntimeInto(context.Background(), runtimeClient),
				cluster,
				nil,
				machine,
				devMachine,
			)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(devMachine.Finalizers).To(BeEmpty())
			g.Expect(runtimeClient.listedFilters).To(HaveLen(1))
			g.Expect(runtimeClient.listedFilters[0]["name"]).To(HaveKey(fmt.Sprintf("^%s$", expectedContainerName)))
			g.Expect(runtimeClient.DeleteContainerCalls()).To(Equal([]string{expectedContainerName}))
		})
	}
}

func TestDockerMachineBackend_ReconcileDeleteMissingDevClusterErrorKeepsFinalizer(t *testing.T) {
	g := NewWithT(t)
	runtimeClient := &errorContainerRuntime{
		FakeRuntime: &container.FakeRuntime{},
	}
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "default"},
	}
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "generated-machine",
			Namespace: cluster.Namespace,
			Labels:    map[string]string{clusterv1.MachinePoolNameLabel: "test-pool"},
		},
	}
	devMachine := &infrav1.DevMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "pool-container",
			Namespace:  machine.Namespace,
			Finalizers: []string{infrav1.MachineFinalizer},
			Labels:     machine.Labels,
		},
		Spec: infrav1.DevMachineSpec{
			Backend: infrav1.DevMachineBackendSpec{
				Docker: &infrav1.DockerMachineBackendSpec{},
			},
		},
	}
	r := &dockerbackend.MachineBackendReconciler{
		ContainerRuntime: runtimeClient,
		TaskManager:      dockerbackend.NewTaskManager(),
	}

	_, err := r.ReconcileDelete(
		container.RuntimeInto(context.Background(), runtimeClient),
		cluster,
		nil,
		machine,
		devMachine,
	)
	g.Expect(err).To(MatchError("failed to create helper for managing the externalMachine: failed to list containers: failed to list containers: container lookup failed"))
	g.Expect(devMachine.Finalizers).To(ConsistOf(infrav1.MachineFinalizer))
}

type recordingContainerRuntime struct {
	*container.FakeRuntime
	containers    []container.Container
	listedFilters []container.FilterBuilder
}

func (r *recordingContainerRuntime) ListContainers(_ context.Context, filters container.FilterBuilder) ([]container.Container, error) {
	r.listedFilters = append(r.listedFilters, filters)
	return r.containers, nil
}

type errorContainerRuntime struct {
	*container.FakeRuntime
}

func (r *errorContainerRuntime) ListContainers(_ context.Context, _ container.FilterBuilder) ([]container.Container, error) {
	return nil, errors.New("container lookup failed")
}

func clientKey(obj client.Object) types.NamespacedName {
	return types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
}
