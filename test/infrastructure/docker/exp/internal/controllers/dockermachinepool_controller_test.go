/*
Copyright 2023 The Kubernetes Authors.

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

// Package controllers implements controller functionality.
package controllers

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	infraexpv1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/internal/docker"
)

const (
	clusterName = "test-cluster"
)

func TestInitNodePoolMachineStatuses(t *testing.T) {
	machine1 := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine1",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:     clusterName,
				clusterv1.MachinePoolNameLabel: "machinepool-test",
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: clusterName,
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "DockerMachine",
				Name:       "docker-machine1",
				Namespace:  metav1.NamespaceDefault,
			},
		},
	}

	machine2 := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine2",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:     clusterName,
				clusterv1.MachinePoolNameLabel: "machinepool-test",
			},
			Annotations: map[string]string{
				clusterv1.DeleteMachineAnnotation: "true",
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: clusterName,
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "DockerMachine",
				Name:       "docker-machine2",
				Namespace:  metav1.NamespaceDefault,
			},
		},
	}

	dockerMachine1 := infrav1.DockerMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "docker-machine1",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:     clusterName,
				clusterv1.MachinePoolNameLabel: "machinepool-test",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "cluster.x-k8s.io/v1beta1",
					Kind:       "Machine",
					Name:       machine1.Name,
				},
			},
		},
		Spec: infrav1.DockerMachineSpec{
			ProviderID: pointer.String("docker:////docker-machine1"),
		},
		Status: infrav1.DockerMachineStatus{
			Addresses: []clusterv1.MachineAddress{
				{
					Type:    clusterv1.MachineInternalIP,
					Address: "test-address-1",
				},
			},
			Conditions: clusterv1.Conditions{
				{
					Type:   infrav1.BootstrapExecSucceededCondition,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	dockerMachine2 := infrav1.DockerMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "docker-machine2",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:     clusterName,
				clusterv1.MachinePoolNameLabel: "machinepool-test",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "cluster.x-k8s.io/v1beta1",
					Kind:       "Machine",
					Name:       machine2.Name,
				},
			},
		},
		Spec: infrav1.DockerMachineSpec{
			ProviderID: pointer.String("docker:////docker-machine2"),
		},
		Status: infrav1.DockerMachineStatus{
			Addresses: []clusterv1.MachineAddress{
				{
					Type:    clusterv1.MachineInternalIP,
					Address: "test-address-2",
				},
			},
		},
	}

	dockerMachine3 := infrav1.DockerMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "docker-machine3",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:     clusterName,
				clusterv1.MachinePoolNameLabel: "machinepool-test",
			},
		},
		Spec: infrav1.DockerMachineSpec{
			ProviderID: pointer.String("docker:////docker-machine3"),
		},
		Status: infrav1.DockerMachineStatus{
			Addresses: []clusterv1.MachineAddress{
				{
					Type:    clusterv1.MachineInternalIP,
					Address: "test-address-2",
				},
			},
		},
	}

	testCases := []struct {
		name              string
		dockerMachines    []infrav1.DockerMachine
		machines          []clusterv1.Machine
		expectedInstances []docker.NodePoolMachineStatus
		expectErr         bool
	}{
		{
			name: "should return a list of instances",
			dockerMachines: []infrav1.DockerMachine{
				dockerMachine1,
				dockerMachine2,
			},
			machines: []clusterv1.Machine{
				machine1,
				machine2,
			},
			expectedInstances: []docker.NodePoolMachineStatus{
				{
					Name:             dockerMachine1.Name,
					PrioritizeDelete: false,
					ProviderID:       dockerMachine1.Spec.ProviderID,
				},
				{
					Name:             dockerMachine2.Name,
					PrioritizeDelete: true,
					ProviderID:       dockerMachine2.Spec.ProviderID,
				},
			},
		},
		{
			name: "do not return error if owner is missing",
			dockerMachines: []infrav1.DockerMachine{
				dockerMachine1,
				dockerMachine2,
				dockerMachine3,
			},
			machines: []clusterv1.Machine{
				machine1,
				machine2,
			},
			expectedInstances: []docker.NodePoolMachineStatus{
				{
					Name:             dockerMachine1.Name,
					PrioritizeDelete: false,
					ProviderID:       dockerMachine1.Spec.ProviderID,
				},
				{
					Name:             dockerMachine2.Name,
					PrioritizeDelete: true,
					ProviderID:       dockerMachine2.Spec.ProviderID,
				},
				{
					Name:             dockerMachine3.Name,
					PrioritizeDelete: false,
					ProviderID:       dockerMachine3.Spec.ProviderID,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			objs := []client.Object{}

			for _, ma := range tc.machines {
				objs = append(objs, ma.DeepCopy())
			}

			for _, dm := range tc.dockerMachines {
				objs = append(objs, dm.DeepCopy())
			}

			r := &DockerMachinePoolReconciler{
				Client: fake.NewClientBuilder().WithObjects(objs...).Build(),
			}

			result, err := r.initNodePoolMachineStatusList(context.Background(), tc.dockerMachines, &infraexpv1.DockerMachinePool{})
			if tc.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(result).To(HaveLen(len(tc.expectedInstances)))
				for i, instance := range result {
					g.Expect(instance).To(Equal(tc.expectedInstances[i]))
				}
			}
		})
	}
}
