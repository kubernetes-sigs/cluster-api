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
	"testing"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
	out := r.DockerClusterToDockerMachines(dockerCluster)
	machineNames := make([]string, len(out))
	for i := range out {
		machineNames[i] = out[i].Name
	}
	g.Expect(out).To(HaveLen(2))
	g.Expect(machineNames).To(ConsistOf("my-machine-0", "my-machine-1"))
}

func newCluster(clusterName string, dockerCluster *infrav1.DockerCluster) *clusterv1.Cluster {
	cluster := &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterName,
		},
	}
	if dockerCluster != nil {
		cluster.Spec.InfrastructureRef = &corev1.ObjectReference{
			Name:       dockerCluster.Name,
			Namespace:  dockerCluster.Namespace,
			Kind:       dockerCluster.Kind,
			APIVersion: dockerCluster.GroupVersionKind().GroupVersion().String(),
		}
	}
	return cluster
}

func newDockerCluster(clusterName, dockerName string) *infrav1.DockerCluster {
	return &infrav1.DockerCluster{
		TypeMeta: metav1.TypeMeta{},
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
				clusterv1.ClusterLabelName: clusterName,
			},
		},
	}
	if dockerMachine != nil {
		machine.Spec.InfrastructureRef = corev1.ObjectReference{
			Name:       dockerMachine.Name,
			Namespace:  dockerMachine.Namespace,
			Kind:       dockerMachine.Kind,
			APIVersion: dockerMachine.GroupVersionKind().GroupVersion().String(),
		}
	}
	return machine
}

func newDockerMachine(dockerMachineName, machineName string) *infrav1.DockerMachine {
	return &infrav1.DockerMachine{
		TypeMeta: metav1.TypeMeta{},
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
