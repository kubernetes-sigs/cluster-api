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
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
)

const (
	clusterName = "test-cluster"
)

func TestDeleteMachinePoolMachine(t *testing.T) {
	defaultMachinePool := &expv1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machinepool-test",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: clusterName,
			},
		},
		Spec: expv1.MachinePoolSpec{
			ClusterName: clusterName,
			Replicas:    pointer.Int32(2),
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
							Kind:       "BootstrapConfig",
							Name:       "bootstrap-config1",
						},
					},
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
						Kind:       "InfrastructureConfig",
						Name:       "infra-config1",
					},
				},
			},
		},
	}

	machine := &clusterv1.Machine{
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
				Name:       "docker-machine",
				Namespace:  metav1.NamespaceDefault,
			},
		},
	}

	dockerMachine := &infrav1.DockerMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "docker-machine",
			Namespace:       metav1.NamespaceDefault,
			OwnerReferences: []metav1.OwnerReference{},
		},
	}

	testCases := []struct {
		name          string
		dockerMachine *infrav1.DockerMachine
		machine       *clusterv1.Machine
		machinePool   *expv1.MachinePool
		expectedError string
	}{
		{
			name:          "delete owner Machine for DockerMachine",
			dockerMachine: dockerMachine.DeepCopy(),
			machine:       machine.DeepCopy(),
			machinePool:   defaultMachinePool.DeepCopy(),
			expectedError: "",
		},
		{
			name:          "delete DockerMachine directly when MachinePool is not found",
			dockerMachine: dockerMachine.DeepCopy(),
			machine:       nil,
			machinePool:   nil,
			expectedError: "",
		},
		{
			name:          "return error to requeue when owner Machine doesn't exist but MachinePool does",
			dockerMachine: dockerMachine.DeepCopy(),
			machine:       nil,
			machinePool:   defaultMachinePool.DeepCopy(),
			expectedError: fmt.Sprintf("DockerMachine %s/%s has no owner Machine, will reattempt deletion once owner Machine is present", dockerMachine.Namespace, dockerMachine.Name),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			objs := []client.Object{tc.dockerMachine}
			if tc.machine != nil {
				objs = append(objs, tc.machine)
			}
			mp := expv1.MachinePool{}
			if tc.machinePool != nil {
				objs = append(objs, tc.machinePool)
				mp = *tc.machinePool
			}

			r := &DockerMachinePoolReconciler{
				Client: fake.NewClientBuilder().WithObjects(objs...).Build(),
			}

			if tc.machine != nil {
				machine := &clusterv1.Machine{}
				g.Expect(r.Client.Get(context.Background(), client.ObjectKeyFromObject(tc.machine), machine)).To(Succeed())
				tc.dockerMachine.OwnerReferences = append(tc.dockerMachine.OwnerReferences, *metav1.NewControllerRef(machine, machine.GroupVersionKind()))
			}

			err := r.deleteMachinePoolMachine(context.Background(), *tc.dockerMachine, mp)
			if tc.expectedError != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tc.expectedError))
			} else {
				g.Expect(err).NotTo(HaveOccurred())

				// If Machine exists, expect delete call to go to Machine, which triggers DockerMachine deletion eventually (though we're not running it here).
				if tc.machine != nil {
					g.Expect(r.Client.Get(context.Background(), client.ObjectKeyFromObject(tc.machine), tc.machine)).ToNot(Succeed())
				} else { // Otherwise, expect delete call to go to DockerMachine directly.
					g.Expect(r.Client.Get(context.Background(), client.ObjectKeyFromObject(tc.dockerMachine), tc.dockerMachine)).ToNot(Succeed())
				}
			}
		})
	}
}
