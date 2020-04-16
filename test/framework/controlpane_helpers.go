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

	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateKubeadmControlPlaneInput is the input for CreateKubeadmControlPlane.
type CreateKubeadmControlPlaneInput struct {
	Creator         Creator
	ControlPlane    *controlplanev1.KubeadmControlPlane
	MachineTemplate runtime.Object
}

// CreateKubeadmControlPlane creates the control plane object and necessary dependencies.
func CreateKubeadmControlPlane(ctx context.Context, input CreateKubeadmControlPlaneInput, intervals ...interface{}) {
	By("creating the machine template")
	Expect(input.Creator.Create(ctx, input.MachineTemplate)).To(Succeed())

	By("creating a KubeadmControlPlane")
	Eventually(func() error {
		err := input.Creator.Create(ctx, input.ControlPlane)
		if err != nil {
			fmt.Println(err)
		}
		return err
	}, intervals...).Should(Succeed())
}

// GetKubeadmControlPlaneByClusterInput is the input for GetKubeadmControlPlaneByCluster.
type GetKubeadmControlPlaneByClusterInput struct {
	Lister      Lister
	ClusterName string
	Namespace   string
}

// GetKubeadmControlPlaneByCluster returns the KubeadmControlPlane objects for a cluster.
// Important! this method relies on labels that are created by the CAPI controllers during the first reconciliation, so
// it is necessary to ensure this is already happened before calling it.
func GetKubeadmControlPlaneByCluster(ctx context.Context, input GetKubeadmControlPlaneByClusterInput) *controlplanev1.KubeadmControlPlane {
	controlPlaneList := &controlplanev1.KubeadmControlPlaneList{}
	Expect(input.Lister.List(ctx, controlPlaneList, byClusterOptions(input.ClusterName, input.Namespace)...)).To(Succeed(), "Failed to list KubeadmControlPlane object for Cluster %s/%s", input.Namespace, input.ClusterName)
	Expect(len(controlPlaneList.Items)).ToNot(BeNumerically(">", 1), "Cluster %s/%s should not have more than 1 KubeadmControlPlane object", input.Namespace, input.ClusterName)
	if len(controlPlaneList.Items) == 1 {
		return &controlPlaneList.Items[0]
	}
	return nil
}

// WaitForKubeadmControlPlaneMachinesToExistInput is the input for WaitForKubeadmControlPlaneMachinesToExist.
type WaitForKubeadmControlPlaneMachinesToExistInput struct {
	Lister       Lister
	Cluster      *clusterv1.Cluster
	ControlPlane *controlplanev1.KubeadmControlPlane
}

// WaitForKubeadmControlPlaneMachinesToExist will wait until all control plane machines have node refs.
func WaitForKubeadmControlPlaneMachinesToExist(ctx context.Context, input WaitForKubeadmControlPlaneMachinesToExistInput, intervals ...interface{}) {
	By("waiting for all control plane nodes to exist")
	inClustersNamespaceListOption := client.InNamespace(input.Cluster.Namespace)
	// ControlPlane labels
	matchClusterListOption := client.MatchingLabels{
		clusterv1.MachineControlPlaneLabelName: "",
		clusterv1.ClusterLabelName:             input.Cluster.Name,
	}

	Eventually(func() (int, error) {
		machineList := &clusterv1.MachineList{}
		if err := input.Lister.List(ctx, machineList, inClustersNamespaceListOption, matchClusterListOption); err != nil {
			fmt.Println(err)
			return 0, err
		}
		count := 0
		for _, machine := range machineList.Items {
			if machine.Status.NodeRef != nil {
				count++
			}
		}
		return count, nil
	}, intervals...).Should(Equal(int(*input.ControlPlane.Spec.Replicas)))
}

// WaitForKubeadmControlPlaneMachinesToExistInput is the input for WaitForKubeadmControlPlaneMachinesToExist.
type WaitForOneKubeadmControlPlaneMachineToExistInput struct {
	Lister       Lister
	Cluster      *clusterv1.Cluster
	ControlPlane *controlplanev1.KubeadmControlPlane
}

// WaitForKubeadmControlPlaneMachineToExist will wait until all control plane machines have node refs.
func WaitForOneKubeadmControlPlaneMachineToExist(ctx context.Context, input WaitForOneKubeadmControlPlaneMachineToExistInput, intervals ...interface{}) {
	By("waiting for one control plane node to exist")
	inClustersNamespaceListOption := client.InNamespace(input.Cluster.Namespace)
	// ControlPlane labels
	matchClusterListOption := client.MatchingLabels{
		clusterv1.MachineControlPlaneLabelName: "",
		clusterv1.ClusterLabelName:             input.Cluster.Name,
	}

	Eventually(func() (bool, error) {
		machineList := &clusterv1.MachineList{}
		if err := input.Lister.List(ctx, machineList, inClustersNamespaceListOption, matchClusterListOption); err != nil {
			fmt.Println(err)
			return false, err
		}
		count := 0
		for _, machine := range machineList.Items {
			if machine.Status.NodeRef != nil {
				count++
			}
		}
		return count > 0, nil
	}, intervals...).Should(BeTrue())
}

// WaitForControlPlaneToBeReadyInput is the input for WaitForControlPlaneToBeReady.
type WaitForControlPlaneToBeReadyInput struct {
	Getter       Getter
	ControlPlane *controlplanev1.KubeadmControlPlane
}

// WaitForControlPlaneToBeReady will wait for a control plane to be ready.
func WaitForControlPlaneToBeReady(ctx context.Context, input WaitForControlPlaneToBeReadyInput, intervals ...interface{}) {
	By("waiting for the control plane to be ready")
	Eventually(func() (bool, error) {
		controlplane := &controlplanev1.KubeadmControlPlane{}
		key := client.ObjectKey{
			Namespace: input.ControlPlane.GetNamespace(),
			Name:      input.ControlPlane.GetName(),
		}
		if err := input.Getter.Get(ctx, key, controlplane); err != nil {
			return false, err
		}
		return controlplane.Status.Ready, nil
	}, intervals...).Should(BeTrue())
}

// AssertControlPlaneFailureDomainsInput is the input for AssertControlPlaneFailureDomains.
type AssertControlPlaneFailureDomainsInput struct {
	GetLister  GetLister
	ClusterKey client.ObjectKey
	// ExpectedFailureDomains is required because this function cannot (easily) infer what success looks like.
	// In theory this field is not strictly necessary and could be replaced with enough clever logic/math.
	ExpectedFailureDomains map[string]int
}

// AssertControlPlaneFailureDomains will look at all control plane machines and see what failure domains they were
// placed in. If machines were placed in unexpected or wrong failure domains the expectation will fail.
func AssertControlPlaneFailureDomains(ctx context.Context, input AssertControlPlaneFailureDomainsInput) {
	failureDomainCounts := map[string]int{}

	// Look up the cluster object to find all known failure domains.
	cluster := &clusterv1.Cluster{}
	Expect(input.GetLister.Get(ctx, input.ClusterKey, cluster)).To(Succeed())

	for fd := range cluster.Status.FailureDomains {
		failureDomainCounts[fd] = 0
	}

	// Look up all the control plane machines.
	inClustersNamespaceListOption := client.InNamespace(input.ClusterKey.Namespace)
	matchClusterListOption := client.MatchingLabels{
		clusterv1.ClusterLabelName:             input.ClusterKey.Name,
		clusterv1.MachineControlPlaneLabelName: "",
	}

	machineList := &clusterv1.MachineList{}
	Expect(input.GetLister.List(ctx, machineList, inClustersNamespaceListOption, matchClusterListOption)).
		To(Succeed(), "Couldn't list machines for the cluster %q", input.ClusterKey.Name)

	// Count all control plane machine failure domains.
	for _, machine := range machineList.Items {
		if machine.Spec.FailureDomain == nil {
			continue
		}
		failureDomainCounts[*machine.Spec.FailureDomain]++
	}
	Expect(failureDomainCounts).To(Equal(input.ExpectedFailureDomains))
}

type GetMachinesByClusterInput struct {
	Lister      Lister
	ClusterName string
	Namespace   string
}

// GetControlPlaneMachinesByCluster returns the Machine objects for a cluster.
// Important! this method relies on labels that are created by the CAPI controllers during the first reconciliation, so
// it is necessary to ensure this is already happened before calling it.
func GetControlPlaneMachinesByCluster(ctx context.Context, input GetMachinesByClusterInput) []clusterv1.Machine {
	options := append(byClusterOptions(input.ClusterName, input.Namespace), controlPlaneMachineOptions()...)

	machineList := &clusterv1.MachineList{}
	Expect(input.Lister.List(ctx, machineList, options...)).To(Succeed(), "Failed to list MachineList object for Cluster %s/%s", input.Namespace, input.ClusterName)

	return machineList.Items
}

// controlPlaneMachineOptions returns a set of ListOptions that allows to get all machine objects belonging to control plane.
func controlPlaneMachineOptions() []client.ListOption {
	return []client.ListOption{
		client.HasLabels{clusterv1.MachineControlPlaneLabelName},
	}
}
