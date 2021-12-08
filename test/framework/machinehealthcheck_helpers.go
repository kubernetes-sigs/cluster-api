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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// DiscoverMachineHealthCheckAndWaitForRemediationInput is the input for DiscoverMachineHealthCheckAndWait.
type DiscoverMachineHealthCheckAndWaitForRemediationInput struct {
	ClusterProxy              ClusterProxy
	Cluster                   *clusterv1.Cluster
	WaitForMachineRemediation []interface{}
}

// DiscoverMachineHealthChecksAndWaitForRemediation patches an unhealthy node condition to one node observed by the Machine Health Check and then wait for remediation.
func DiscoverMachineHealthChecksAndWaitForRemediation(ctx context.Context, input DiscoverMachineHealthCheckAndWaitForRemediationInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DiscoverMachineHealthChecksAndWaitForRemediation")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling DiscoverMachineHealthChecksAndWaitForRemediation")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling DiscoverMachineHealthChecksAndWaitForRemediation")

	mgmtClient := input.ClusterProxy.GetClient()
	fmt.Fprintln(GinkgoWriter, "Discovering machine health check resources")
	machineHealthChecks := GetMachineHealthChecksForCluster(ctx, GetMachineHealthChecksForClusterInput{
		Lister:      mgmtClient,
		ClusterName: input.Cluster.Name,
		Namespace:   input.Cluster.Namespace,
	})

	Expect(machineHealthChecks).NotTo(BeEmpty())

	for _, mhc := range machineHealthChecks {
		Expect(mhc.Spec.UnhealthyConditions).NotTo(BeEmpty())

		fmt.Fprintln(GinkgoWriter, "Ensuring there is at least 1 Machine that MachineHealthCheck is matching")
		machines := GetMachinesByMachineHealthCheck(ctx, GetMachinesByMachineHealthCheckInput{
			Lister:             mgmtClient,
			ClusterName:        input.Cluster.Name,
			MachineHealthCheck: mhc,
		})

		Expect(machines).NotTo(BeEmpty())

		fmt.Fprintln(GinkgoWriter, "Patching MachineHealthCheck unhealthy condition to one of the nodes")
		unhealthyNodeCondition := corev1.NodeCondition{
			Type:               mhc.Spec.UnhealthyConditions[0].Type,
			Status:             mhc.Spec.UnhealthyConditions[0].Status,
			LastTransitionTime: metav1.Time{Time: time.Now()},
		}
		PatchNodeCondition(ctx, PatchNodeConditionInput{
			ClusterProxy:  input.ClusterProxy,
			Cluster:       input.Cluster,
			NodeCondition: unhealthyNodeCondition,
			Machine:       machines[0],
		})

		fmt.Fprintln(GinkgoWriter, "Waiting for remediation")
		WaitForMachineHealthCheckToRemediateUnhealthyNodeCondition(ctx, WaitForMachineHealthCheckToRemediateUnhealthyNodeConditionInput{
			ClusterProxy:       input.ClusterProxy,
			Cluster:            input.Cluster,
			MachineHealthCheck: mhc,
			MachinesCount:      len(machines),
		}, input.WaitForMachineRemediation...)
	}
}

// GetMachineHealthChecksForClusterInput is the input for GetMachineHealthChecksForCluster.
type GetMachineHealthChecksForClusterInput struct {
	Lister      Lister
	ClusterName string
	Namespace   string
}

// GetMachineHealthChecksForCluster returns the MachineHealthCheck objects for a cluster.
// Important! this method relies on labels that are created by the CAPI controllers during the first reconciliation, so
// it is necessary to ensure this is already happened before calling it.
func GetMachineHealthChecksForCluster(ctx context.Context, input GetMachineHealthChecksForClusterInput) []*clusterv1.MachineHealthCheck {
	machineHealthCheckList := &clusterv1.MachineHealthCheckList{}
	Expect(input.Lister.List(ctx, machineHealthCheckList, byClusterOptions(input.ClusterName, input.Namespace)...)).To(Succeed(), "Failed to list MachineDeployments object for Cluster %s/%s", input.Namespace, input.ClusterName)

	machineHealthChecks := make([]*clusterv1.MachineHealthCheck, len(machineHealthCheckList.Items))
	for i := range machineHealthCheckList.Items {
		machineHealthChecks[i] = &machineHealthCheckList.Items[i]
	}
	return machineHealthChecks
}

// machineHealthCheckOptions returns a set of ListOptions that allows to get all machine objects belonging to a MachineHealthCheck.
func machineHealthCheckOptions(machineHealthCheck clusterv1.MachineHealthCheck) []client.ListOption {
	return []client.ListOption{
		client.MatchingLabels(machineHealthCheck.Spec.Selector.MatchLabels),
	}
}

// WaitForMachineHealthCheckToRemediateUnhealthyNodeConditionInput is the input for WaitForMachineHealthCheckToRemediateUnhealthyNodeCondition.
type WaitForMachineHealthCheckToRemediateUnhealthyNodeConditionInput struct {
	ClusterProxy       ClusterProxy
	Cluster            *clusterv1.Cluster
	MachineHealthCheck *clusterv1.MachineHealthCheck
	MachinesCount      int
}

// WaitForMachineHealthCheckToRemediateUnhealthyNodeCondition patches a node condition to any one of the machines with a node ref.
func WaitForMachineHealthCheckToRemediateUnhealthyNodeCondition(ctx context.Context, input WaitForMachineHealthCheckToRemediateUnhealthyNodeConditionInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for WaitForMachineHealthCheckToRemediateUnhealthyNodeCondition")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling WaitForMachineHealthCheckToRemediateUnhealthyNodeCondition")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling WaitForMachineHealthCheckToRemediateUnhealthyNodeCondition")
	Expect(input.MachineHealthCheck).NotTo(BeNil(), "Invalid argument. input.MachineHealthCheck can't be nil when calling WaitForMachineHealthCheckToRemediateUnhealthyNodeCondition")
	Expect(input.MachinesCount).NotTo(BeZero(), "Invalid argument. input.MachinesCount can't be zero when calling WaitForMachineHealthCheckToRemediateUnhealthyNodeCondition")

	fmt.Fprintln(GinkgoWriter, "Waiting until the node with unhealthy node condition is remediated")
	Eventually(func() bool {
		machines := GetMachinesByMachineHealthCheck(ctx, GetMachinesByMachineHealthCheckInput{
			Lister:             input.ClusterProxy.GetClient(),
			ClusterName:        input.Cluster.Name,
			MachineHealthCheck: input.MachineHealthCheck,
		})
		// Wait for all the machines to exists.
		// NOTE: this is required given that this helper is called after a remediation
		// and we want to make sure all the machine are back in place before testing for unhealthyCondition being fixed.
		if len(machines) < input.MachinesCount {
			return false
		}

		for _, machine := range machines {
			if machine.Status.NodeRef == nil {
				return false
			}
			node := &corev1.Node{}
			// This should not be an Expect(), because it may return error during machine deletion.
			err := input.ClusterProxy.GetWorkloadCluster(ctx, input.Cluster.Namespace, input.Cluster.Name).GetClient().Get(ctx, types.NamespacedName{Name: machine.Status.NodeRef.Name, Namespace: machine.Status.NodeRef.Namespace}, node)
			if err != nil {
				return false
			}
			if hasMatchingUnhealthyConditions(input.MachineHealthCheck, node.Status.Conditions) {
				return false
			}
		}
		return true
	}, intervals...).Should(BeTrue())
}

// hasMatchingUnhealthyConditions returns true if any node condition matches with machine health check unhealthy conditions.
func hasMatchingUnhealthyConditions(machineHealthCheck *clusterv1.MachineHealthCheck, nodeConditions []corev1.NodeCondition) bool {
	for _, unhealthyCondition := range machineHealthCheck.Spec.UnhealthyConditions {
		for _, nodeCondition := range nodeConditions {
			if nodeCondition.Type == unhealthyCondition.Type && nodeCondition.Status == unhealthyCondition.Status {
				return true
			}
		}
	}
	return false
}
