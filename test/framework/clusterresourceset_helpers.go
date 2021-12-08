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

package framework

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
)

// GetClusterResourceSetsInput is the input for GetClusterResourceSets.
type GetClusterResourceSetsInput struct {
	Lister    Lister
	Namespace string
}

// GetClusterResourceSets returns all ClusterResourceSet objects in a namespace.
func GetClusterResourceSets(ctx context.Context, input GetClusterResourceSetsInput) []*addonsv1.ClusterResourceSet {
	crsList := &addonsv1.ClusterResourceSetList{}
	Expect(input.Lister.List(ctx, crsList, client.InNamespace(input.Namespace))).To(Succeed(), "Failed to list ClusterResourceSet objects for namespace %s", input.Namespace)

	clusterResourceSets := make([]*addonsv1.ClusterResourceSet, len(crsList.Items))
	for i := range crsList.Items {
		clusterResourceSets[i] = &crsList.Items[i]
	}
	return clusterResourceSets
}

// GetClusterResourceSetBindingByClusterInput is the input for GetClusterResourceSetBindingByCluster.
type GetClusterResourceSetBindingByClusterInput struct {
	Getter      Getter
	ClusterName string
	Namespace   string
}

// GetClusterResourceSetBindingByCluster returns the ClusterResourceBinding objects for a cluster.
func GetClusterResourceSetBindingByCluster(ctx context.Context, input GetClusterResourceSetBindingByClusterInput) *addonsv1.ClusterResourceSetBinding {
	binding := &addonsv1.ClusterResourceSetBinding{}
	Expect(input.Getter.Get(ctx, client.ObjectKey{Namespace: input.Namespace, Name: input.ClusterName}, binding)).To(Succeed(), "Failed to list MachineDeployments object for Cluster %s/%s", input.Namespace, input.ClusterName)
	return binding
}

// DiscoverClusterResourceSetAndWaitForSuccessInput is the input for DiscoverClusterResourceSetAndWaitForSuccess.
type DiscoverClusterResourceSetAndWaitForSuccessInput struct {
	ClusterProxy ClusterProxy
	Cluster      *clusterv1.Cluster
}

// DiscoverClusterResourceSetAndWaitForSuccess patches a ClusterResourceSet label to the cluster and waits for resources to be created in that cluster.
func DiscoverClusterResourceSetAndWaitForSuccess(ctx context.Context, input DiscoverClusterResourceSetAndWaitForSuccessInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DiscoverClusterResourceSetAndWaitForSuccess")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling DiscoverClusterResourceSetAndWaitForSuccess")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling DiscoverClusterResourceSetAndWaitForSuccess")

	mgmtClient := input.ClusterProxy.GetClient()
	fmt.Fprintln(GinkgoWriter, "Discovering cluster resource set resources")
	clusterResourceSets := GetClusterResourceSets(ctx, GetClusterResourceSetsInput{
		Lister:    mgmtClient,
		Namespace: input.Cluster.Namespace,
	})

	Expect(clusterResourceSets).NotTo(BeEmpty())

	for _, crs := range clusterResourceSets {
		Expect(crs.Spec.ClusterSelector).NotTo(BeNil())

		PatchClusterLabel(ctx, PatchClusterLabelInput{
			ClusterProxy: input.ClusterProxy,
			Cluster:      input.Cluster,
			Labels:       crs.Spec.ClusterSelector.MatchLabels,
		})

		fmt.Fprintln(GinkgoWriter, "Waiting for ClusterResourceSet resources to be created in workload cluster")
		WaitForClusterResourceSetToApplyResources(ctx, WaitForClusterResourceSetToApplyResourcesInput{
			ClusterProxy:       input.ClusterProxy,
			Cluster:            input.Cluster,
			ClusterResourceSet: crs,
		}, intervals...)
	}
}

// WaitForClusterResourceSetToApplyResourcesInput is the input for WaitForClusterResourceSetToApplyResources.
type WaitForClusterResourceSetToApplyResourcesInput struct {
	ClusterProxy       ClusterProxy
	Cluster            *clusterv1.Cluster
	ClusterResourceSet *addonsv1.ClusterResourceSet
}

// WaitForClusterResourceSetToApplyResources wait until all ClusterResourceSet resources are created in the matching cluster.
func WaitForClusterResourceSetToApplyResources(ctx context.Context, input WaitForClusterResourceSetToApplyResourcesInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for WaitForClusterResourceSetToApplyResources")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling WaitForClusterResourceSetToApplyResources")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling WaitForClusterResourceSetToApplyResources")
	Expect(input.ClusterResourceSet).NotTo(BeNil(), "Invalid argument. input.ClusterResourceSet can't be nil when calling WaitForClusterResourceSetToApplyResources")

	fmt.Fprintln(GinkgoWriter, "Waiting until the binding is created for the workload cluster")
	Eventually(func() bool {
		binding := &addonsv1.ClusterResourceSetBinding{}
		err := input.ClusterProxy.GetClient().Get(ctx, types.NamespacedName{Name: input.Cluster.Name, Namespace: input.Cluster.Namespace}, binding)
		return err == nil
	}, intervals...).Should(BeTrue())

	fmt.Fprintln(GinkgoWriter, "Waiting until the resource is created in the workload cluster")
	Eventually(func() bool {
		binding := &addonsv1.ClusterResourceSetBinding{}
		Expect(input.ClusterProxy.GetClient().Get(ctx, types.NamespacedName{Name: input.Cluster.Name, Namespace: input.Cluster.Namespace}, binding)).To(Succeed())

		for _, resource := range input.ClusterResourceSet.Spec.Resources {
			var configSource client.Object

			switch resource.Kind {
			case string(addonsv1.SecretClusterResourceSetResourceKind):
				configSource = &corev1.Secret{}
			case string(addonsv1.ConfigMapClusterResourceSetResourceKind):
				configSource = &corev1.ConfigMap{}
			}

			if err := input.ClusterProxy.GetClient().Get(ctx, types.NamespacedName{Name: resource.Name, Namespace: input.ClusterResourceSet.Namespace}, configSource); err != nil {
				// If the resource is missing, CRS will not requeue but retry at each reconcile,
				// because this is not an error. So, we are only interested in seeing the resources that exist to be applied by CRS.
				continue
			}

			if len(binding.Spec.Bindings) == 0 || !binding.Spec.Bindings[0].IsApplied(resource) {
				return false
			}
		}
		return true
	}, intervals...).Should(BeTrue())
}
