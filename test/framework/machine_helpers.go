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
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework/internal/log"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
)

// GetMachinesByMachineDeploymentsInput is the input for GetMachinesByMachineDeployments.
type GetMachinesByMachineDeploymentsInput struct {
	Lister            Lister
	ClusterName       string
	Namespace         string
	MachineDeployment clusterv1.MachineDeployment
}

// GetMachinesByMachineDeployments returns Machine objects for a cluster belonging to a machine deployment.
// Important! this method relies on labels that are created by the CAPI controllers during the first reconciliation, so
// it is necessary to ensure this is already happened before calling it.
func GetMachinesByMachineDeployments(ctx context.Context, input GetMachinesByMachineDeploymentsInput) []clusterv1.Machine {
	Expect(ctx).NotTo(BeNil(), "ctx is required for GetMachinesByMachineDeployments")
	Expect(input.Lister).ToNot(BeNil(), "Invalid argument. input.Lister can't be nil when calling GetMachinesByMachineDeployments")
	Expect(input.ClusterName).ToNot(BeEmpty(), "Invalid argument. input.ClusterName can't be empty when calling GetMachinesByMachineDeployments")
	Expect(input.Namespace).ToNot(BeEmpty(), "Invalid argument. input.Namespace can't be empty when calling GetMachinesByMachineDeployments")
	Expect(input.MachineDeployment).ToNot(BeNil(), "Invalid argument. input.MachineDeployment can't be nil when calling GetMachinesByMachineDeployments")

	opts := byClusterOptions(input.ClusterName, input.Namespace)
	opts = append(opts, machineDeploymentOptions(input.MachineDeployment)...)

	machineList := &clusterv1.MachineList{}
	Expect(input.Lister.List(ctx, machineList, opts...)).To(Succeed(), "Failed to list MachineList object for Cluster %s/%s", input.Namespace, input.ClusterName)

	return machineList.Items
}

// GetMachinesByMachineHealthCheckInput is the input for GetMachinesByMachineHealthCheck.
type GetMachinesByMachineHealthCheckInput struct {
	Lister             Lister
	ClusterName        string
	MachineHealthCheck *clusterv1.MachineHealthCheck
}

// GetMachinesByMachineHealthCheck returns Machine objects for a cluster that match with MachineHealthCheck selector.
func GetMachinesByMachineHealthCheck(ctx context.Context, input GetMachinesByMachineHealthCheckInput) []clusterv1.Machine {
	Expect(ctx).NotTo(BeNil(), "ctx is required for GetMachinesByMachineDeployments")
	Expect(input.Lister).ToNot(BeNil(), "Invalid argument. input.Lister can't be nil when calling GetMachinesByMachineHealthCheck")
	Expect(input.ClusterName).ToNot(BeEmpty(), "Invalid argument. input.ClusterName can't be empty when calling GetMachinesByMachineHealthCheck")
	Expect(input.MachineHealthCheck).ToNot(BeNil(), "Invalid argument. input.MachineHealthCheck can't be nil when calling GetMachinesByMachineHealthCheck")

	opts := byClusterOptions(input.ClusterName, input.MachineHealthCheck.Namespace)
	opts = append(opts, machineHealthCheckOptions(*input.MachineHealthCheck)...)

	machineList := &clusterv1.MachineList{}
	Expect(input.Lister.List(ctx, machineList, opts...)).To(Succeed(), "Failed to list MachineList object for Cluster %s/%s", input.MachineHealthCheck.Namespace, input.ClusterName)

	return machineList.Items
}

// GetControlPlaneMachinesByClusterInput is the input for GetControlPlaneMachinesByCluster.
type GetControlPlaneMachinesByClusterInput struct {
	Lister      Lister
	ClusterName string
	Namespace   string
}

// GetControlPlaneMachinesByCluster returns the Machine objects for a cluster.
// Important! this method relies on labels that are created by the CAPI controllers during the first reconciliation, so
// it is necessary to ensure this is already happened before calling it.
func GetControlPlaneMachinesByCluster(ctx context.Context, input GetControlPlaneMachinesByClusterInput) []clusterv1.Machine {
	Expect(ctx).NotTo(BeNil(), "ctx is required for GetControlPlaneMachinesByCluster")
	Expect(input.Lister).ToNot(BeNil(), "Invalid argument. input.Lister can't be nil when calling GetControlPlaneMachinesByCluster")
	Expect(input.ClusterName).ToNot(BeEmpty(), "Invalid argument. input.ClusterName can't be empty when calling GetControlPlaneMachinesByCluster")
	Expect(input.Namespace).ToNot(BeEmpty(), "Invalid argument. input.Namespace can't be empty when calling GetControlPlaneMachinesByCluster")

	options := append(byClusterOptions(input.ClusterName, input.Namespace), controlPlaneMachineOptions()...)

	machineList := &clusterv1.MachineList{}
	Expect(input.Lister.List(ctx, machineList, options...)).To(Succeed(), "Failed to list MachineList object for Cluster %s/%s", input.Namespace, input.ClusterName)

	return machineList.Items
}

// WaitForControlPlaneMachinesToBeUpgradedInput is the input for WaitForControlPlaneMachinesToBeUpgraded.
type WaitForControlPlaneMachinesToBeUpgradedInput struct {
	Lister                   Lister
	Cluster                  *clusterv1.Cluster
	KubernetesUpgradeVersion string
	MachineCount             int
}

// WaitForControlPlaneMachinesToBeUpgraded waits until all machines are upgraded to the correct Kubernetes version.
func WaitForControlPlaneMachinesToBeUpgraded(ctx context.Context, input WaitForControlPlaneMachinesToBeUpgradedInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for WaitForControlPlaneMachinesToBeUpgraded")
	Expect(input.Lister).ToNot(BeNil(), "Invalid argument. input.Lister can't be nil when calling WaitForControlPlaneMachinesToBeUpgraded")
	Expect(input.KubernetesUpgradeVersion).ToNot(BeEmpty(), "Invalid argument. input.KubernetesUpgradeVersion can't be empty when calling WaitForControlPlaneMachinesToBeUpgraded")
	Expect(input.MachineCount).To(BeNumerically(">", 0), "Invalid argument. input.MachineCount can't be smaller than 1 when calling WaitForControlPlaneMachinesToBeUpgraded")

	By(fmt.Sprintf("Ensuring all control-plane machines have upgraded kubernetes version %s", input.KubernetesUpgradeVersion))

	Eventually(func() (int, error) {
		machines := GetControlPlaneMachinesByCluster(ctx, GetControlPlaneMachinesByClusterInput{
			Lister:      input.Lister,
			ClusterName: input.Cluster.Name,
			Namespace:   input.Cluster.Namespace,
		})

		upgraded := 0
		for _, machine := range machines {
			m := machine
			if *m.Spec.Version == input.KubernetesUpgradeVersion && conditions.IsTrue(&m, clusterv1.MachineNodeHealthyCondition) {
				upgraded++
			}
		}
		if len(machines) > upgraded {
			return 0, errors.New("old nodes remain")
		}
		return upgraded, nil
	}, intervals...).Should(Equal(input.MachineCount))
}

// WaitForMachineDeploymentMachinesToBeUpgradedInput is the input for WaitForMachineDeploymentMachinesToBeUpgraded.
type WaitForMachineDeploymentMachinesToBeUpgradedInput struct {
	Lister                   Lister
	Cluster                  *clusterv1.Cluster
	KubernetesUpgradeVersion string
	MachineCount             int
	MachineDeployment        clusterv1.MachineDeployment
}

// WaitForMachineDeploymentMachinesToBeUpgraded waits until all machines belonging to a MachineDeployment are upgraded to the correct kubernetes version.
func WaitForMachineDeploymentMachinesToBeUpgraded(ctx context.Context, input WaitForMachineDeploymentMachinesToBeUpgradedInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for WaitForMachineDeploymentMachinesToBeUpgraded")
	Expect(input.Lister).ToNot(BeNil(), "Invalid argument. input.Getter can't be nil when calling WaitForMachineDeploymentMachinesToBeUpgraded")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling WaitForMachineDeploymentMachinesToBeUpgraded")
	Expect(input.KubernetesUpgradeVersion).ToNot(BeNil(), "Invalid argument. input.KubernetesUpgradeVersion can't be nil when calling WaitForMachineDeploymentMachinesToBeUpgraded")
	Expect(input.MachineDeployment).ToNot(BeNil(), "Invalid argument. input.MachineDeployment can't be nil when calling WaitForMachineDeploymentMachinesToBeUpgraded")
	Expect(input.MachineCount).To(BeNumerically(">", 0), "Invalid argument. input.MachineCount can't be smaller than 1 when calling WaitForMachineDeploymentMachinesToBeUpgraded")

	log.Logf("Ensuring all MachineDeployment Machines have upgraded kubernetes version %s", input.KubernetesUpgradeVersion)
	Eventually(func() (int, error) {
		machines := GetMachinesByMachineDeployments(ctx, GetMachinesByMachineDeploymentsInput{
			Lister:            input.Lister,
			ClusterName:       input.Cluster.Name,
			Namespace:         input.Cluster.Namespace,
			MachineDeployment: input.MachineDeployment,
		})

		upgraded := 0
		for _, machine := range machines {
			if *machine.Spec.Version == input.KubernetesUpgradeVersion {
				upgraded++
			}
		}
		if len(machines) > upgraded {
			return 0, errors.New("old nodes remain")
		}
		return upgraded, nil
	}, intervals...).Should(Equal(input.MachineCount))
}

// PatchNodeConditionInput is the input for PatchNodeCondition.
type PatchNodeConditionInput struct {
	ClusterProxy  ClusterProxy
	Cluster       *clusterv1.Cluster
	NodeCondition corev1.NodeCondition
	Machine       clusterv1.Machine
}

// PatchNodeCondition patches a node condition to any one of the machines with a node ref.
func PatchNodeCondition(ctx context.Context, input PatchNodeConditionInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for PatchNodeConditions")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling PatchNodeConditions")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling PatchNodeConditions")
	Expect(input.NodeCondition).ToNot(BeNil(), "Invalid argument. input.NodeCondition can't be nil when calling PatchNodeConditions")
	Expect(input.Machine).ToNot(BeNil(), "Invalid argument. input.Machine can't be nil when calling PatchNodeConditions")

	log.Logf("Patching the node condition to the node")
	Expect(input.Machine.Status.NodeRef).ToNot(BeNil())
	node := &corev1.Node{}
	Expect(input.ClusterProxy.GetWorkloadCluster(ctx, input.Cluster.Namespace, input.Cluster.Name).GetClient().Get(ctx, types.NamespacedName{Name: input.Machine.Status.NodeRef.Name, Namespace: input.Machine.Status.NodeRef.Namespace}, node)).To(Succeed())
	patchHelper, err := patch.NewHelper(node, input.ClusterProxy.GetWorkloadCluster(ctx, input.Cluster.Namespace, input.Cluster.Name).GetClient())
	Expect(err).ToNot(HaveOccurred())
	node.Status.Conditions = append(node.Status.Conditions, input.NodeCondition)
	Expect(patchHelper.Patch(ctx, node)).To(Succeed())
}

// MachineStatusCheck is a type that operates a status check on a Machine.
type MachineStatusCheck func(p *clusterv1.Machine) error

// WaitForMachineStatusCheckInput is the input for WaitForMachineStatusCheck.
type WaitForMachineStatusCheckInput struct {
	Getter       Getter
	Machine      *clusterv1.Machine
	StatusChecks []MachineStatusCheck
}

// WaitForMachineStatusCheck waits for the specified status to be true for the machine.
func WaitForMachineStatusCheck(ctx context.Context, input WaitForMachineStatusCheckInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for WaitForMachineStatusCheck")
	Expect(input.Machine).ToNot(BeNil(), "Invalid argument. input.Machine can't be nil when calling WaitForMachineStatusCheck")
	Expect(input.StatusChecks).ToNot(BeEmpty(), "Invalid argument. input.StatusCheck can't be empty when calling WaitForMachineStatusCheck")

	Eventually(func() (bool, error) {
		machine := &clusterv1.Machine{}
		key := client.ObjectKey{
			Namespace: input.Machine.Namespace,
			Name:      input.Machine.Name,
		}
		err := input.Getter.Get(ctx, key, machine)
		Expect(err).NotTo(HaveOccurred())

		for _, statusCheck := range input.StatusChecks {
			err := statusCheck(machine)
			if err != nil {
				return false, err
			}
		}
		return true, nil
	}, intervals...).Should(BeTrue())
}

// MachineNodeRefCheck is a MachineStatusCheck ensuring that a NodeRef is assigned to the machine.
func MachineNodeRefCheck() MachineStatusCheck {
	return func(machine *clusterv1.Machine) error {
		if machine.Status.NodeRef == nil {
			return errors.Errorf("NodeRef is not assigned to the machine %s/%s", machine.Namespace, machine.Name)
		}
		return nil
	}
}

// MachinePhaseCheck is a MachineStatusCheck ensuring that a machines is in the expected phase.
func MachinePhaseCheck(expectedPhase string) MachineStatusCheck {
	return func(machine *clusterv1.Machine) error {
		if machine.Status.Phase != expectedPhase {
			return errors.Errorf("Machine %s/%s is not in phase %s", machine.Namespace, machine.Name, expectedPhase)
		}
		return nil
	}
}
