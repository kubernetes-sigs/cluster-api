/*
Copyright 2021 The Kubernetes Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestMachineSetScheme(t *testing.T) {
	g := NewWithT(t)
	ns, err := env.CreateNamespace(ctx, "schema-test")
	g.Expect(err).ToNot(HaveOccurred())

	testMachineSet := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns.Name,
			Name:      "test-machineset",
		},
		Spec: clusterv1.MachineSetSpec{
			ClusterName: "test",
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					ClusterName: "test",
				},
			},
		},
	}

	g.Expect(env.Create(ctx, testMachineSet)).To(Succeed())

	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(ns, testMachineSet)

	g.Expect(testMachineSet.Spec.Replicas).To(Equal(pointer.Int32Ptr(1)))
}

func TestMachineDeploymentScheme(t *testing.T) {
	g := NewWithT(t)
	ns, err := env.CreateNamespace(ctx, "schema-test")
	g.Expect(err).ToNot(HaveOccurred())

	testMachineDeployment := &clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns.Name,
			Name:      "test-machinedeployment",
		},
		Spec: clusterv1.MachineDeploymentSpec{
			ClusterName: "test",
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					ClusterName: "test",
				},
			},
		},
	}

	g.Expect(env.Create(ctx, testMachineDeployment)).To(Succeed())

	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(ns, testMachineDeployment)

	g.Expect(testMachineDeployment.Spec.Replicas).To(Equal(pointer.Int32Ptr(1)))
}
