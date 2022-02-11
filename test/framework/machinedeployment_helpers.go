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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework/internal/log"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
)

// CreateMachineDeploymentInput is the input for CreateMachineDeployment.
type CreateMachineDeploymentInput struct {
	Creator                 Creator
	MachineDeployment       *clusterv1.MachineDeployment
	BootstrapConfigTemplate client.Object
	InfraMachineTemplate    client.Object
}

// CreateMachineDeployment creates the machine deployment and dependencies.
func CreateMachineDeployment(ctx context.Context, input CreateMachineDeploymentInput) {
	By("creating a core MachineDeployment resource")
	Expect(input.Creator.Create(ctx, input.MachineDeployment)).To(Succeed())

	By("creating a BootstrapConfigTemplate resource")
	Expect(input.Creator.Create(ctx, input.BootstrapConfigTemplate)).To(Succeed())

	By("creating an InfrastructureMachineTemplate resource")
	Expect(input.Creator.Create(ctx, input.InfraMachineTemplate)).To(Succeed())
}

// GetMachineDeploymentsByClusterInput is the input for GetMachineDeploymentsByCluster.
type GetMachineDeploymentsByClusterInput struct {
	Lister      Lister
	ClusterName string
	Namespace   string
}

// GetMachineDeploymentsByCluster returns the MachineDeployments objects for a cluster.
// Important! this method relies on labels that are created by the CAPI controllers during the first reconciliation, so
// it is necessary to ensure this is already happened before calling it.
func GetMachineDeploymentsByCluster(ctx context.Context, input GetMachineDeploymentsByClusterInput) []*clusterv1.MachineDeployment {
	deploymentList := &clusterv1.MachineDeploymentList{}
	Expect(input.Lister.List(ctx, deploymentList, byClusterOptions(input.ClusterName, input.Namespace)...)).To(Succeed(), "Failed to list MachineDeployments object for Cluster %s/%s", input.Namespace, input.ClusterName)

	deployments := make([]*clusterv1.MachineDeployment, len(deploymentList.Items))
	for i := range deploymentList.Items {
		Expect(deploymentList.Items[i].Spec.Replicas).ToNot(BeNil())
		deployments[i] = &deploymentList.Items[i]
	}
	return deployments
}

// WaitForMachineDeploymentNodesToExistInput is the input for WaitForMachineDeploymentNodesToExist.
type WaitForMachineDeploymentNodesToExistInput struct {
	Lister            Lister
	Cluster           *clusterv1.Cluster
	MachineDeployment *clusterv1.MachineDeployment
}

// WaitForMachineDeploymentNodesToExist waits until all nodes associated with a machine deployment exist.
func WaitForMachineDeploymentNodesToExist(ctx context.Context, input WaitForMachineDeploymentNodesToExistInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for WaitForMachineDeploymentNodesToExist")
	Expect(input.Lister).ToNot(BeNil(), "Invalid argument. input.Lister can't be nil when calling WaitForMachineDeploymentNodesToExist")
	Expect(input.MachineDeployment).ToNot(BeNil(), "Invalid argument. input.MachineDeployment can't be nil when calling WaitForMachineDeploymentNodesToExist")

	By("Waiting for the workload nodes to exist")
	Eventually(func() (int, error) {
		selectorMap, err := metav1.LabelSelectorAsMap(&input.MachineDeployment.Spec.Selector)
		if err != nil {
			return 0, err
		}
		ms := &clusterv1.MachineSetList{}
		if err := input.Lister.List(ctx, ms, client.InNamespace(input.Cluster.Namespace), client.MatchingLabels(selectorMap)); err != nil {
			return 0, err
		}
		if len(ms.Items) == 0 {
			return 0, errors.New("no machinesets were found")
		}
		machineSet := ms.Items[0]
		selectorMap, err = metav1.LabelSelectorAsMap(&machineSet.Spec.Selector)
		if err != nil {
			return 0, err
		}
		machines := &clusterv1.MachineList{}
		if err := input.Lister.List(ctx, machines, client.InNamespace(machineSet.Namespace), client.MatchingLabels(selectorMap)); err != nil {
			return 0, err
		}
		count := 0
		for _, machine := range machines.Items {
			if machine.Status.NodeRef != nil {
				count++
			}
		}
		return count, nil
	}, intervals...).Should(Equal(int(*input.MachineDeployment.Spec.Replicas)))
}

// DiscoveryAndWaitForMachineDeploymentsInput is the input type for DiscoveryAndWaitForMachineDeployments.
type DiscoveryAndWaitForMachineDeploymentsInput struct {
	Lister  Lister
	Cluster *clusterv1.Cluster
}

// DiscoveryAndWaitForMachineDeployments discovers the MachineDeployments existing in a cluster and waits for them to be ready (all the machine provisioned).
func DiscoveryAndWaitForMachineDeployments(ctx context.Context, input DiscoveryAndWaitForMachineDeploymentsInput, intervals ...interface{}) []*clusterv1.MachineDeployment {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DiscoveryAndWaitForMachineDeployments")
	Expect(input.Lister).ToNot(BeNil(), "Invalid argument. input.Lister can't be nil when calling DiscoveryAndWaitForMachineDeployments")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling DiscoveryAndWaitForMachineDeployments")

	machineDeployments := GetMachineDeploymentsByCluster(ctx, GetMachineDeploymentsByClusterInput{
		Lister:      input.Lister,
		ClusterName: input.Cluster.Name,
		Namespace:   input.Cluster.Namespace,
	})
	for _, deployment := range machineDeployments {
		WaitForMachineDeploymentNodesToExist(ctx, WaitForMachineDeploymentNodesToExistInput{
			Lister:            input.Lister,
			Cluster:           input.Cluster,
			MachineDeployment: deployment,
		}, intervals...)
	}
	return machineDeployments
}

// UpgradeMachineDeploymentsAndWaitInput is the input type for UpgradeMachineDeploymentsAndWait.
type UpgradeMachineDeploymentsAndWaitInput struct {
	ClusterProxy                ClusterProxy
	Cluster                     *clusterv1.Cluster
	UpgradeVersion              string
	UpgradeMachineTemplate      *string
	MachineDeployments          []*clusterv1.MachineDeployment
	WaitForMachinesToBeUpgraded []interface{}
}

// UpgradeMachineDeploymentsAndWait upgrades a machine deployment and waits for its machines to be upgraded.
func UpgradeMachineDeploymentsAndWait(ctx context.Context, input UpgradeMachineDeploymentsAndWaitInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for UpgradeMachineDeploymentsAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling UpgradeMachineDeploymentsAndWait")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling UpgradeMachineDeploymentsAndWait")
	Expect(input.UpgradeVersion).ToNot(BeNil(), "Invalid argument. input.UpgradeVersion can't be nil when calling UpgradeMachineDeploymentsAndWait")
	Expect(input.MachineDeployments).ToNot(BeEmpty(), "Invalid argument. input.MachineDeployments can't be empty when calling UpgradeMachineDeploymentsAndWait")

	mgmtClient := input.ClusterProxy.GetClient()

	for _, deployment := range input.MachineDeployments {
		log.Logf("Patching the new kubernetes version to Machine Deployment %s/%s", deployment.Namespace, deployment.Name)
		patchHelper, err := patch.NewHelper(deployment, mgmtClient)
		Expect(err).ToNot(HaveOccurred())

		oldVersion := deployment.Spec.Template.Spec.Version
		deployment.Spec.Template.Spec.Version = &input.UpgradeVersion
		if input.UpgradeMachineTemplate != nil {
			deployment.Spec.Template.Spec.InfrastructureRef.Name = *input.UpgradeMachineTemplate
		}
		Expect(patchHelper.Patch(ctx, deployment)).To(Succeed())

		log.Logf("Waiting for Kubernetes versions of machines in MachineDeployment %s/%s to be upgraded from %s to %s",
			deployment.Namespace, deployment.Name, *oldVersion, input.UpgradeVersion)
		WaitForMachineDeploymentMachinesToBeUpgraded(ctx, WaitForMachineDeploymentMachinesToBeUpgradedInput{
			Lister:                   mgmtClient,
			Cluster:                  input.Cluster,
			MachineCount:             int(*deployment.Spec.Replicas),
			KubernetesUpgradeVersion: input.UpgradeVersion,
			MachineDeployment:        *deployment,
		}, input.WaitForMachinesToBeUpgraded...)
	}
}

// WaitForMachineDeploymentRollingUpgradeToStartInput is the input for WaitForMachineDeploymentRollingUpgradeToStart.
type WaitForMachineDeploymentRollingUpgradeToStartInput struct {
	Getter            Getter
	MachineDeployment *clusterv1.MachineDeployment
}

// WaitForMachineDeploymentRollingUpgradeToStart waits until rolling upgrade starts.
func WaitForMachineDeploymentRollingUpgradeToStart(ctx context.Context, input WaitForMachineDeploymentRollingUpgradeToStartInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for WaitForMachineDeploymentRollingUpgradeToStart")
	Expect(input.Getter).ToNot(BeNil(), "Invalid argument. input.Getter can't be nil when calling WaitForMachineDeploymentRollingUpgradeToStart")
	Expect(input.MachineDeployment).ToNot(BeNil(), "Invalid argument. input.MachineDeployment can't be nil when calling WaitForMachineDeploymentRollingUpgradeToStarts")

	log.Logf("Waiting for MachineDeployment rolling upgrade to start")
	Eventually(func() bool {
		md := &clusterv1.MachineDeployment{}
		Expect(input.Getter.Get(ctx, client.ObjectKey{Namespace: input.MachineDeployment.Namespace, Name: input.MachineDeployment.Name}, md)).To(Succeed())
		return md.Status.Replicas != md.Status.AvailableReplicas
	}, intervals...).Should(BeTrue())
}

// WaitForMachineDeploymentRollingUpgradeToCompleteInput is the input for WaitForMachineDeploymentRollingUpgradeToComplete.
type WaitForMachineDeploymentRollingUpgradeToCompleteInput struct {
	Getter            Getter
	MachineDeployment *clusterv1.MachineDeployment
}

// WaitForMachineDeploymentRollingUpgradeToComplete waits until rolling upgrade is complete.
func WaitForMachineDeploymentRollingUpgradeToComplete(ctx context.Context, input WaitForMachineDeploymentRollingUpgradeToCompleteInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for WaitForMachineDeploymentRollingUpgradeToComplete")
	Expect(input.Getter).ToNot(BeNil(), "Invalid argument. input.Getter can't be nil when calling WaitForMachineDeploymentRollingUpgradeToComplete")
	Expect(input.MachineDeployment).ToNot(BeNil(), "Invalid argument. input.MachineDeployment can't be nil when calling WaitForMachineDeploymentRollingUpgradeToComplete")

	log.Logf("Waiting for MachineDeployment rolling upgrade to complete")
	Eventually(func() bool {
		md := &clusterv1.MachineDeployment{}
		Expect(input.Getter.Get(ctx, client.ObjectKey{Namespace: input.MachineDeployment.Namespace, Name: input.MachineDeployment.Name}, md)).To(Succeed())
		return md.Status.Replicas == md.Status.AvailableReplicas
	}, intervals...).Should(BeTrue())
}

// UpgradeMachineDeploymentInfrastructureRefAndWaitInput is the input type for UpgradeMachineDeploymentInfrastructureRefAndWait.
type UpgradeMachineDeploymentInfrastructureRefAndWaitInput struct {
	ClusterProxy                ClusterProxy
	Cluster                     *clusterv1.Cluster
	MachineDeployments          []*clusterv1.MachineDeployment
	WaitForMachinesToBeUpgraded []interface{}
}

// UpgradeMachineDeploymentInfrastructureRefAndWait upgrades a machine deployment infrastructure ref and waits for its machines to be upgraded.
func UpgradeMachineDeploymentInfrastructureRefAndWait(ctx context.Context, input UpgradeMachineDeploymentInfrastructureRefAndWaitInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for UpgradeMachineDeploymentInfrastructureRefAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling UpgradeMachineDeploymentInfrastructureRefAndWait")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling UpgradeMachineDeploymentInfrastructureRefAndWait")
	Expect(input.MachineDeployments).ToNot(BeEmpty(), "Invalid argument. input.MachineDeployments can't be empty when calling UpgradeMachineDeploymentInfrastructureRefAndWait")

	mgmtClient := input.ClusterProxy.GetClient()

	for _, deployment := range input.MachineDeployments {
		log.Logf("Patching the new infrastructure ref to Machine Deployment %s/%s", deployment.Namespace, deployment.Name)
		// Retrieve infra object
		infraRef := deployment.Spec.Template.Spec.InfrastructureRef
		infraObj := &unstructured.Unstructured{}
		infraObj.SetGroupVersionKind(infraRef.GroupVersionKind())
		key := client.ObjectKey{
			Namespace: input.Cluster.Namespace,
			Name:      infraRef.Name,
		}
		Expect(mgmtClient.Get(ctx, key, infraObj)).NotTo(HaveOccurred())

		// Creates a new infra object
		newInfraObj := infraObj
		newInfraObjName := fmt.Sprintf("%s-%s", infraRef.Name, util.RandomString(6))
		newInfraObj.SetName(newInfraObjName)
		newInfraObj.SetResourceVersion("")
		Expect(mgmtClient.Create(ctx, newInfraObj)).NotTo(HaveOccurred())

		// Patch the new infra object's ref to the machine deployment
		patchHelper, err := patch.NewHelper(deployment, mgmtClient)
		Expect(err).ToNot(HaveOccurred())
		infraRef.Name = newInfraObjName
		deployment.Spec.Template.Spec.InfrastructureRef = infraRef
		Expect(patchHelper.Patch(ctx, deployment)).To(Succeed())

		log.Logf("Waiting for rolling upgrade to start.")
		WaitForMachineDeploymentRollingUpgradeToStart(ctx, WaitForMachineDeploymentRollingUpgradeToStartInput{
			Getter:            mgmtClient,
			MachineDeployment: deployment,
		}, input.WaitForMachinesToBeUpgraded...)

		log.Logf("Waiting for rolling upgrade to complete.")
		WaitForMachineDeploymentRollingUpgradeToComplete(ctx, WaitForMachineDeploymentRollingUpgradeToCompleteInput{
			Getter:            mgmtClient,
			MachineDeployment: deployment,
		}, input.WaitForMachinesToBeUpgraded...)
	}
}

// machineDeploymentOptions returns a set of ListOptions that allows to get all machine objects belonging to a machine deployment.
func machineDeploymentOptions(deployment clusterv1.MachineDeployment) []client.ListOption {
	return []client.ListOption{
		client.MatchingLabels(deployment.Spec.Selector.MatchLabels),
	}
}

// ScaleAndWaitMachineDeploymentInput is the input for ScaleAndWaitMachineDeployment.
type ScaleAndWaitMachineDeploymentInput struct {
	ClusterProxy              ClusterProxy
	Cluster                   *clusterv1.Cluster
	MachineDeployment         *clusterv1.MachineDeployment
	Replicas                  int32
	WaitForMachineDeployments []interface{}
}

// ScaleAndWaitMachineDeployment scales MachineDeployment and waits until all machines have node ref and equal to Replicas.
func ScaleAndWaitMachineDeployment(ctx context.Context, input ScaleAndWaitMachineDeploymentInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for ScaleAndWaitMachineDeployment")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling ScaleAndWaitMachineDeployment")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling ScaleAndWaitMachineDeployment")

	log.Logf("Scaling machine deployment %s/%s from %d to %d replicas", input.MachineDeployment.Namespace, input.MachineDeployment.Name, *input.MachineDeployment.Spec.Replicas, input.Replicas)
	patchHelper, err := patch.NewHelper(input.MachineDeployment, input.ClusterProxy.GetClient())
	Expect(err).ToNot(HaveOccurred())
	input.MachineDeployment.Spec.Replicas = pointer.Int32Ptr(input.Replicas)
	Expect(patchHelper.Patch(ctx, input.MachineDeployment)).To(Succeed())

	log.Logf("Waiting for correct number of replicas to exist")
	Eventually(func() (int, error) {
		selectorMap, err := metav1.LabelSelectorAsMap(&input.MachineDeployment.Spec.Selector)
		if err != nil {
			return -1, err
		}
		ms := &clusterv1.MachineSetList{}
		if err := input.ClusterProxy.GetClient().List(ctx, ms, client.InNamespace(input.Cluster.Namespace), client.MatchingLabels(selectorMap)); err != nil {
			return -1, err
		}
		if len(ms.Items) == 0 {
			return -1, errors.New("no machinesets were found")
		}
		machineSet := ms.Items[0]
		selectorMap, err = metav1.LabelSelectorAsMap(&machineSet.Spec.Selector)
		if err != nil {
			return -1, err
		}
		machines := &clusterv1.MachineList{}
		if err := input.ClusterProxy.GetClient().List(ctx, machines, client.InNamespace(machineSet.Namespace), client.MatchingLabels(selectorMap)); err != nil {
			return -1, err
		}
		nodeRefCount := 0
		for _, machine := range machines.Items {
			if machine.Status.NodeRef != nil {
				nodeRefCount++
			}
		}
		if len(machines.Items) != nodeRefCount {
			return -1, errors.New("Machine count does not match existing nodes count")
		}
		return nodeRefCount, nil
	}, input.WaitForMachineDeployments...).Should(Equal(int(*input.MachineDeployment.Spec.Replicas)))
}
