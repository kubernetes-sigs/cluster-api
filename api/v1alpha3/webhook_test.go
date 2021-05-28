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

package v1alpha3

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/util"
)

func TestClusterConversion(t *testing.T) {
	g := NewWithT(t)
	ns, err := env.CreateNamespace(ctx, fmt.Sprintf("conversion-webhook-%s", util.RandomString(5)))
	g.Expect(err).ToNot(HaveOccurred())
	clusterName := fmt.Sprintf("test-cluster-%s", util.RandomString(5))
	cluster := &Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: ns.Name,
		},
	}

	g.Expect(env.Create(ctx, cluster)).To(Succeed())
	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(ns, cluster)
}

func TestMachineSetConversion(t *testing.T) {
	g := NewWithT(t)
	ns, err := env.CreateNamespace(ctx, fmt.Sprintf("conversion-webhook-%s", util.RandomString(5)))
	g.Expect(err).ToNot(HaveOccurred())

	clusterName := fmt.Sprintf("test-cluster-%s", util.RandomString(5))
	machineSetName := fmt.Sprintf("test-machineset-%s", util.RandomString(5))
	machineSet := &MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns.Name,
			Name:      machineSetName,
		},
		Spec: MachineSetSpec{
			ClusterName:     clusterName,
			Template:        newFakeMachineTemplate(ns.Name, clusterName),
			MinReadySeconds: 10,
			Replicas:        pointer.Int32Ptr(1),
			DeletePolicy:    "Random",
		},
	}

	g.Expect(env.Create(ctx, machineSet)).To(Succeed())
	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(ns, machineSet)
}

func TestMachineDeploymentConversion(t *testing.T) {
	g := NewWithT(t)
	ns, err := env.CreateNamespace(ctx, fmt.Sprintf("conversion-webhook-%s", util.RandomString(5)))
	g.Expect(err).ToNot(HaveOccurred())

	clusterName := fmt.Sprintf("test-cluster-%s", util.RandomString(5))
	machineDeploymentName := fmt.Sprintf("test-machinedeployment-%s", util.RandomString(5))
	machineDeployment := &MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      machineDeploymentName,
			Namespace: ns.Name,
		},
		Spec: MachineDeploymentSpec{
			ClusterName: clusterName,
			Template:    newFakeMachineTemplate(ns.Name, clusterName),
			Replicas:    pointer.Int32Ptr(0),
		},
	}

	g.Expect(env.Create(ctx, machineDeployment)).To(Succeed())
	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(ns, machineDeployment)
}

func newFakeMachineTemplate(namespace, clusterName string) MachineTemplateSpec {
	return MachineTemplateSpec{
		Spec: MachineSpec{
			ClusterName: clusterName,
			Bootstrap: Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha3",
					Kind:       "KubeadmConfigTemplate",
					Name:       fmt.Sprintf("%s-md-0", clusterName),
					Namespace:  namespace,
				},
			},
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
				Kind:       "FakeMachineTemplate",
				Name:       fmt.Sprintf("%s-md-0", clusterName),
				Namespace:  namespace,
			},
			Version: pointer.String("v1.20.2"),
		},
	}
}
