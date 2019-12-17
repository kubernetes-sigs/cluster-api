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

package generator

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

func TestGenerateMachineWithOwner(t *testing.T) {
	g := gomega.NewWithT(t)
	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme)

	namespace := "test"
	namePrefix := "generate2"
	clusterName := "testCluster"
	version := "my-version"
	infraRef := &corev1.ObjectReference{
		Kind:       "InfraKind",
		APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
		Name:       "infra",
		Namespace:  namespace,
	}
	bootstrapRef := &corev1.ObjectReference{
		Kind:       "BootstrapKind",
		APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha3",
		Name:       "bootstrap",
		Namespace:  namespace,
	}
	owner := &metav1.OwnerReference{
		Kind:       "Cluster",
		APIVersion: clusterv1.GroupVersion.String(),
		Name:       clusterName,
	}
	labels := map[string]string{"color": "blue"}
	expectedMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName:    namePrefix + "-",
			Namespace:       namespace,
			ResourceVersion: "1",
			Labels:          labels,
			OwnerReferences: []metav1.OwnerReference{*owner.DeepCopy()},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: clusterName,
			Version:     utilpointer.StringPtr(version),
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: bootstrapRef.DeepCopy(),
			},
			InfrastructureRef: *infraRef.DeepCopy(),
		},
	}

	mgg := &MachineGenerator{}
	err := mgg.GenerateMachine(
		context.Background(),
		fakeClient,
		namespace,
		namePrefix,
		clusterName,
		version,
		infraRef,
		bootstrapRef,
		labels,
		owner,
	)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace(namespace))).To(gomega.Succeed())
	g.Expect(machineList.Items).NotTo(gomega.BeEmpty())
	g.Expect(machineList.Items).To(gomega.HaveLen(1))
	g.Expect(machineList.Items[0]).To(gomega.Equal(*expectedMachine.DeepCopy()))
}

func TestGenerateMachineWithoutOwner(t *testing.T) {
	g := gomega.NewWithT(t)
	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme)

	namespace := "test"
	namePrefix := "generate1"
	clusterName := "testCluster"
	version := "my-version"
	infraRef := &corev1.ObjectReference{
		Kind:       "InfraKind",
		APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
		Name:       "infra",
		Namespace:  namespace,
	}
	bootstrapRef := &corev1.ObjectReference{
		Kind:       "BootstrapKind",
		APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha3",
		Name:       "bootstrap",
		Namespace:  namespace,
	}
	labels := map[string]string{"color": "blue"}
	expectedMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName:    namePrefix + "-",
			Namespace:       namespace,
			ResourceVersion: "1",
			Labels:          labels,
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: clusterName,
			Version:     utilpointer.StringPtr(version),
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: bootstrapRef.DeepCopy(),
			},
			InfrastructureRef: *infraRef.DeepCopy(),
		},
	}

	mgg := &MachineGenerator{}
	err := mgg.GenerateMachine(
		context.Background(),
		fakeClient,
		namespace,
		namePrefix,
		clusterName,
		version,
		infraRef,
		bootstrapRef,
		labels,
		nil,
	)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace(namespace))).To(gomega.Succeed())
	g.Expect(machineList.Items).NotTo(gomega.BeEmpty())
	g.Expect(machineList.Items).To(gomega.HaveLen(1))
	g.Expect(machineList.Items[0]).To(gomega.Equal(*expectedMachine.DeepCopy()))
}
