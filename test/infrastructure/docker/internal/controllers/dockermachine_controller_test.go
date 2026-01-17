/*
Copyright 2019 The Kubernetes Authors.

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

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
)

var (
	clusterName   = "my-cluster"
	dockerCluster = newDockerCluster(clusterName, "my-docker-cluster")
	cluster       = newCluster(clusterName, dockerCluster)

	dockerMachine = newDockerMachine("my-docker-machine-0", "my-machine-0")
	machine       = newMachine(clusterName, "my-machine-0", dockerMachine)

	anotherDockerMachine = newDockerMachine("my-docker-machine-1", "my-machine-1")
	anotherMachine       = newMachine(clusterName, "my-machine-1", anotherDockerMachine)
)

func TestDockerMachineReconciler_DockerClusterToDockerMachines(t *testing.T) {
	g := NewWithT(t)

	objects := []client.Object{
		cluster,
		dockerCluster,
		machine,
		anotherMachine,
		// Intentionally omitted
		newMachine(clusterName, "my-machine-2", nil),
	}
	c := fake.NewClientBuilder().WithObjects(objects...).Build()
	r := DockerMachineReconciler{
		Client: c,
	}
	out := r.dockerClusterToDockerMachines(context.Background(), dockerCluster)
	machineNames := make([]string, len(out))
	for i := range out {
		machineNames[i] = out[i].Name
	}
	g.Expect(out).To(HaveLen(2))
	g.Expect(machineNames).To(ConsistOf("my-machine-0", "my-machine-1"))
}

func newCluster(clusterName string, dockerCluster *infrav1.DockerCluster) *clusterv1.Cluster {
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterName,
		},
	}
	if dockerCluster != nil {
		cluster.Spec.InfrastructureRef = clusterv1.ContractVersionedObjectReference{
			APIGroup: infrav1.GroupVersion.Group,
			Kind:     "DockerCluster",
			Name:     dockerCluster.Name,
		}
	}
	return cluster
}

func newDockerCluster(clusterName, dockerName string) *infrav1.DockerCluster {
	return &infrav1.DockerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: dockerName,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Cluster",
					Name:       clusterName,
				},
			},
		},
	}
}

func newMachine(clusterName, machineName string, dockerMachine *infrav1.DockerMachine) *clusterv1.Machine {
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: machineName,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: clusterName,
			},
		},
	}
	if dockerMachine != nil {
		machine.Spec.InfrastructureRef = clusterv1.ContractVersionedObjectReference{
			APIGroup: infrav1.GroupVersion.Group,
			Kind:     "DockerMachine",
			Name:     dockerMachine.Name,
		}
	}
	return machine
}

func newDockerMachine(dockerMachineName, machineName string) *infrav1.DockerMachine {
	return &infrav1.DockerMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:            dockerMachineName,
			ResourceVersion: "999",
			Finalizers:      []string{infrav1.MachineFinalizer},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Machine",
					Name:       machineName,
				},
			},
		},
		Spec:   infrav1.DockerMachineSpec{},
		Status: infrav1.DockerMachineStatus{},
	}
}

func TestDockerClusterReconciler_ReconcileOrphanedDeletion(t *testing.T) {
	g := NewWithT(t)

	// Create a DockerCluster without owner reference and with deletion timestamp
	now := metav1.Now()
	dockerCluster := &infrav1.DockerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "orphaned-docker-cluster",
			Namespace:         "default",
			DeletionTimestamp: &now,
			Finalizers:        []string{infrav1.ClusterFinalizer},
		},
		Spec: infrav1.DockerClusterSpec{},
	}

	// Create a fake client with the DockerCluster - using status subresource
	c := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(dockerCluster).
		WithStatusSubresource(dockerCluster).
		Build()

	// Create the reconciler
	r := &DockerClusterReconciler{
		Client: c,
	}

	// Reconcile the DockerCluster
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{
			Name:      dockerCluster.Name,
			Namespace: dockerCluster.Namespace,
		},
	})

	// Verify no error occurred
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))

	// Verify the finalizer was removed
	updatedDockerCluster := &infrav1.DockerCluster{}
	err = c.Get(context.Background(), client.ObjectKey{
		Name:      dockerCluster.Name,
		Namespace: dockerCluster.Namespace,
	}, updatedDockerCluster)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(controllerutil.ContainsFinalizer(updatedDockerCluster, infrav1.ClusterFinalizer)).To(BeFalse())
}

func TestDockerClusterReconciler_ReconcileOrphanedWithoutDeletion(t *testing.T) {
	g := NewWithT(t)

	// Create a DockerCluster without owner reference and WITHOUT deletion timestamp
	dockerCluster := &infrav1.DockerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "orphaned-docker-cluster-no-deletion",
			Namespace:  "default",
			Finalizers: []string{infrav1.ClusterFinalizer},
		},
		Spec: infrav1.DockerClusterSpec{},
	}

	// Create a fake client with the DockerCluster - using status subresource
	c := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(dockerCluster).
		WithStatusSubresource(dockerCluster).
		Build()

	// Create the reconciler
	r := &DockerClusterReconciler{
		Client: c,
	}

	// Reconcile the DockerCluster
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{
			Name:      dockerCluster.Name,
			Namespace: dockerCluster.Namespace,
		},
	})

	// Verify no error occurred
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))

	// Verify the finalizer was NOT removed (since it's not being deleted)
	updatedDockerCluster := &infrav1.DockerCluster{}
	err = c.Get(context.Background(), client.ObjectKey{
		Name:      dockerCluster.Name,
		Namespace: dockerCluster.Namespace,
	}, updatedDockerCluster)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(controllerutil.ContainsFinalizer(updatedDockerCluster, infrav1.ClusterFinalizer)).To(BeTrue())
}

func TestDevClusterReconciler_ReconcileOrphanedDeletion(t *testing.T) {
	g := NewWithT(t)

	// Create a DevCluster without owner reference and with deletion timestamp
	now := metav1.Now()
	devCluster := &infrav1.DevCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "orphaned-dev-cluster",
			Namespace:         "default",
			DeletionTimestamp: &now,
			Finalizers:        []string{infrav1.ClusterFinalizer},
		},
		Spec: infrav1.DevClusterSpec{},
	}

	// Create a fake client with the DevCluster - using status subresource
	c := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(devCluster).
		WithStatusSubresource(devCluster).
		Build()

	// Create the reconciler
	r := &DevClusterReconciler{
		Client: c,
	}

	// Reconcile the DevCluster
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{
			Name:      devCluster.Name,
			Namespace: devCluster.Namespace,
		},
	})

	// Verify no error occurred
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))

	// Verify the finalizer was removed
	updatedDevCluster := &infrav1.DevCluster{}
	err = c.Get(context.Background(), client.ObjectKey{
		Name:      devCluster.Name,
		Namespace: devCluster.Namespace,
	}, updatedDevCluster)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(controllerutil.ContainsFinalizer(updatedDevCluster, infrav1.ClusterFinalizer)).To(BeFalse())
}

func TestDevClusterReconciler_ReconcileOrphanedWithoutDeletion(t *testing.T) {
	g := NewWithT(t)

	// Create a DevCluster without owner reference and WITHOUT deletion timestamp
	devCluster := &infrav1.DevCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "orphaned-dev-cluster-no-deletion",
			Namespace:  "default",
			Finalizers: []string{infrav1.ClusterFinalizer},
		},
		Spec: infrav1.DevClusterSpec{},
	}

	// Create a fake client with the DevCluster - using status subresource
	c := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(devCluster).
		WithStatusSubresource(devCluster).
		Build()

	// Create the reconciler
	r := &DevClusterReconciler{
		Client: c,
	}

	// Reconcile the DevCluster
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{
			Name:      devCluster.Name,
			Namespace: devCluster.Namespace,
		},
	})

	// Verify no error occurred
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))

	// Verify the finalizer was NOT removed (since it's not being deleted)
	updatedDevCluster := &infrav1.DevCluster{}
	err = c.Get(context.Background(), client.ObjectKey{
		Name:      devCluster.Name,
		Namespace: devCluster.Namespace,
	}, updatedDevCluster)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(controllerutil.ContainsFinalizer(updatedDevCluster, infrav1.ClusterFinalizer)).To(BeTrue())
}
