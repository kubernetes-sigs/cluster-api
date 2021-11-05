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
	clusterv1alpha3 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestMachinePoolConversion(t *testing.T) {
	g := NewWithT(t)
	ns, err := env.CreateNamespace(ctx, fmt.Sprintf("conversion-webhook-%s", util.RandomString(5)))
	g.Expect(err).ToNot(HaveOccurred())
	clusterName := fmt.Sprintf("test-cluster-%s", util.RandomString(5))
	machinePoolName := fmt.Sprintf("test-machinepool-%s", util.RandomString(5))
	machinePool := &MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      machinePoolName,
			Namespace: ns.Name,
		},
		Spec: MachinePoolSpec{
			ClusterName: clusterName,
			Replicas:    pointer.Int32(3),
			Template:    newFakeMachineTemplate(ns.Name, clusterName),
			Strategy: &clusterv1alpha3.MachineDeploymentStrategy{
				Type: clusterv1alpha3.RollingUpdateMachineDeploymentStrategyType,
			},
			MinReadySeconds: pointer.Int32(60),
			ProviderIDList:  []string{"cloud:////1111", "cloud:////1112", "cloud:////1113"},
			FailureDomains:  []string{"1", "3"},
		},
	}

	g.Expect(env.Create(ctx, machinePool)).To(Succeed())
	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(ns, machinePool)
}

func newFakeMachineTemplate(namespace, clusterName string) clusterv1alpha3.MachineTemplateSpec {
	return clusterv1alpha3.MachineTemplateSpec{
		Spec: clusterv1alpha3.MachineSpec{
			ClusterName: clusterName,
			Bootstrap: clusterv1alpha3.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha3",
					Kind:       "KubeadmConfigTemplate",
					Name:       fmt.Sprintf("%s-md-0", clusterName),
					Namespace:  namespace,
				},
			},
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: "exp.infrastructure.cluster.x-k8s.io/v1alpha3",
				Kind:       "FakeMachinePool",
				Name:       fmt.Sprintf("%s-md-0", clusterName),
				Namespace:  namespace,
			},
			Version: pointer.String("v1.20.2"),
		},
	}
}
