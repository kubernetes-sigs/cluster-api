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
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/machineset"
	. "sigs.k8s.io/cluster-api/test/framework/ginkgoextensions"
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
	Eventually(func() error {
		return input.Creator.Create(ctx, input.MachineDeployment)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to create MachineDeployment %s", klog.KObj(input.MachineDeployment))

	By("creating a BootstrapConfigTemplate resource")
	Eventually(func() error {
		return input.Creator.Create(ctx, input.BootstrapConfigTemplate)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to create BootstrapConfigTemplate %s", klog.KObj(input.BootstrapConfigTemplate))

	By("creating an InfrastructureMachineTemplate resource")
	Eventually(func() error {
		return input.Creator.Create(ctx, input.InfraMachineTemplate)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to create InfrastructureMachineTemplate %s", klog.KObj(input.InfraMachineTemplate))
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
	Eventually(func() error {
		return input.Lister.List(ctx, deploymentList, byClusterOptions(input.ClusterName, input.Namespace)...)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to list MachineDeployments object for Cluster %s", klog.KRef(input.Namespace, input.ClusterName))

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
	Eventually(func(g Gomega) {
		selectorMap, err := metav1.LabelSelectorAsMap(&input.MachineDeployment.Spec.Selector)
		g.Expect(err).NotTo(HaveOccurred())
		ms := &clusterv1.MachineSetList{}
		err = input.Lister.List(ctx, ms, client.InNamespace(input.Cluster.Namespace), client.MatchingLabels(selectorMap))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(ms.Items).NotTo(BeEmpty())
		machineSet := ms.Items[0]
		selectorMap, err = metav1.LabelSelectorAsMap(&machineSet.Spec.Selector)
		g.Expect(err).NotTo(HaveOccurred())
		machines := &clusterv1.MachineList{}
		err = input.Lister.List(ctx, machines, client.InNamespace(machineSet.Namespace), client.MatchingLabels(selectorMap))
		g.Expect(err).NotTo(HaveOccurred())
		count := 0
		for _, machine := range machines.Items {
			if machine.Status.NodeRef != nil {
				count++
			}
		}
		g.Expect(count).To(Equal(int(*input.MachineDeployment.Spec.Replicas)))
	}, intervals...).Should(Succeed(), "Timed out waiting for %d nodes to be created for MachineDeployment %s", int(*input.MachineDeployment.Spec.Replicas), klog.KObj(input.MachineDeployment))
}

// AssertMachineDeploymentFailureDomainsInput is the input for AssertMachineDeploymentFailureDomains.
type AssertMachineDeploymentFailureDomainsInput struct {
	Lister            Lister
	Cluster           *clusterv1.Cluster
	MachineDeployment *clusterv1.MachineDeployment
}

// AssertMachineDeploymentFailureDomains will look at all MachineDeployment machines and see what failure domains they were
// placed in. If machines were placed in unexpected or wrong failure domains the expectation will fail.
func AssertMachineDeploymentFailureDomains(ctx context.Context, input AssertMachineDeploymentFailureDomainsInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for AssertMachineDeploymentFailureDomains")
	Expect(input.Lister).ToNot(BeNil(), "Invalid argument. input.Lister can't be nil when calling AssertMachineDeploymentFailureDomains")
	Expect(input.MachineDeployment).ToNot(BeNil(), "Invalid argument. input.MachineDeployment can't be nil when calling AssertMachineDeploymentFailureDomains")

	machineDeploymentFD := pointer.StringDeref(input.MachineDeployment.Spec.Template.Spec.FailureDomain, "<None>")

	Byf("Checking all the machines controlled by %s are in the %q failure domain", input.MachineDeployment.Name, machineDeploymentFD)
	selectorMap, err := metav1.LabelSelectorAsMap(&input.MachineDeployment.Spec.Selector)
	Expect(err).NotTo(HaveOccurred())

	ms := &clusterv1.MachineSetList{}
	Eventually(func() error {
		return input.Lister.List(ctx, ms, client.InNamespace(input.Cluster.Namespace), client.MatchingLabels(selectorMap))
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to list MachineSets for Cluster %s", klog.KObj(input.Cluster))

	for _, machineSet := range ms.Items {
		machineSetFD := pointer.StringDeref(machineSet.Spec.Template.Spec.FailureDomain, "<None>")
		Expect(machineSetFD).To(Equal(machineDeploymentFD), "MachineSet %s is in the %q failure domain, expecting %q", machineSet.Name, machineSetFD, machineDeploymentFD)

		selectorMap, err = metav1.LabelSelectorAsMap(&machineSet.Spec.Selector)
		Expect(err).NotTo(HaveOccurred())

		machines := &clusterv1.MachineList{}
		Eventually(func() error {
			return input.Lister.List(ctx, machines, client.InNamespace(machineSet.Namespace), client.MatchingLabels(selectorMap))
		}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to list Machines for Cluster %s", klog.KObj(input.Cluster))

		for _, machine := range machines.Items {
			machineFD := pointer.StringDeref(machine.Spec.FailureDomain, "<None>")
			Expect(machineFD).To(Equal(machineDeploymentFD), "Machine %s is in the %q failure domain, expecting %q", machine.Name, machineFD, machineDeploymentFD)
		}
	}
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

		AssertMachineDeploymentFailureDomains(ctx, AssertMachineDeploymentFailureDomainsInput{
			Lister:            input.Lister,
			Cluster:           input.Cluster,
			MachineDeployment: deployment,
		})
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
		log.Logf("Patching the new kubernetes version to Machine Deployment %s", klog.KObj(deployment))
		patchHelper, err := patch.NewHelper(deployment, mgmtClient)
		Expect(err).ToNot(HaveOccurred())

		oldVersion := deployment.Spec.Template.Spec.Version
		deployment.Spec.Template.Spec.Version = &input.UpgradeVersion
		if input.UpgradeMachineTemplate != nil {
			deployment.Spec.Template.Spec.InfrastructureRef.Name = *input.UpgradeMachineTemplate
		}
		Eventually(func() error {
			return patchHelper.Patch(ctx, deployment)
		}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to patch Kubernetes version on MachineDeployment %s", klog.KObj(deployment))

		log.Logf("Waiting for Kubernetes versions of machines in MachineDeployment %s to be upgraded from %s to %s",
			klog.KObj(deployment), *oldVersion, input.UpgradeVersion)
		WaitForMachineDeploymentMachinesToBeUpgraded(ctx, WaitForMachineDeploymentMachinesToBeUpgradedInput{
			Lister:                   mgmtClient,
			Cluster:                  input.Cluster,
			MachineCount:             int(*deployment.Spec.Replicas),
			KubernetesUpgradeVersion: input.UpgradeVersion,
			MachineDeployment:        *deployment,
		}, input.WaitForMachinesToBeUpgraded...)
	}
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
		// Get MachineSets of the MachineDeployment before the upgrade.
		msListBeforeUpgrade, err := machineset.GetMachineSetsForDeployment(ctx, mgmtClient, client.ObjectKeyFromObject(deployment))
		Expect(err).ToNot(HaveOccurred())

		log.Logf("Patching the new infrastructure ref to Machine Deployment %s", klog.KObj(deployment))
		// Retrieve infra object.
		infraRef := deployment.Spec.Template.Spec.InfrastructureRef
		infraObj := &unstructured.Unstructured{}
		infraObj.SetGroupVersionKind(infraRef.GroupVersionKind())
		key := client.ObjectKey{
			Namespace: input.Cluster.Namespace,
			Name:      infraRef.Name,
		}
		Eventually(func() error {
			return mgmtClient.Get(ctx, key, infraObj)
		}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to get infra object %s for MachineDeployment %s", klog.KRef(key.Namespace, key.Name), klog.KObj(deployment))

		// Create a new infra object.
		newInfraObj := infraObj
		newInfraObjName := fmt.Sprintf("%s-%s", infraRef.Name, util.RandomString(6))
		newInfraObj.SetName(newInfraObjName)
		newInfraObj.SetResourceVersion("")
		Eventually(func() error {
			return mgmtClient.Create(ctx, newInfraObj)
		}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to create new infrastructure object %s for MachineDeployment %s", klog.KObj(infraObj), klog.KObj(deployment))

		// Patch the new infra object's ref to the MachineDeployment.
		patchHelper, err := patch.NewHelper(deployment, mgmtClient)
		Expect(err).ToNot(HaveOccurred())
		infraRef.Name = newInfraObjName
		deployment.Spec.Template.Spec.InfrastructureRef = infraRef
		Eventually(func() error {
			return patchHelper.Patch(ctx, deployment)
		}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to patch new infrastructure ref to MachineDeployment %s", klog.KObj(deployment))

		Eventually(func(g Gomega) {
			// Get MachineSets of the MachineDeployment after the upgrade.
			msListAfterUpgrade, err := machineset.GetMachineSetsForDeployment(ctx, mgmtClient, client.ObjectKeyFromObject(deployment))
			g.Expect(err).ToNot(HaveOccurred())

			// Find the new MachineSet.
			newMachineSets := machineSetNames(msListAfterUpgrade).Difference(machineSetNames(msListBeforeUpgrade))
			g.Expect(newMachineSets).To(HaveLen(1), "expecting exactly 1 MachineSet after rollout")
			var newMachineSet *clusterv1.MachineSet
			for _, ms := range msListAfterUpgrade {
				if newMachineSets.Has(ms.Name) {
					newMachineSet = ms
				}
			}

			// MachineSet should be rolled out.
			g.Expect(newMachineSet.Spec.Replicas).To(Equal(deployment.Spec.Replicas))
			g.Expect(*newMachineSet.Spec.Replicas).To(Equal(newMachineSet.Status.Replicas))
			g.Expect(*newMachineSet.Spec.Replicas).To(Equal(newMachineSet.Status.ReadyReplicas))
			g.Expect(*newMachineSet.Spec.Replicas).To(Equal(newMachineSet.Status.AvailableReplicas))

			// MachineSet should have the same infrastructureRef as the MachineDeployment.
			g.Expect(newMachineSet.Spec.Template.Spec.InfrastructureRef).To(Equal(deployment.Spec.Template.Spec.InfrastructureRef))
		}, input.WaitForMachinesToBeUpgraded...).Should(Succeed())
	}
}

// UpgradeMachineDeploymentInPlaceMutableFieldsAndWaitInput is the input type for UpgradeMachineDeploymentInPlaceMutableFieldsAndWait.
type UpgradeMachineDeploymentInPlaceMutableFieldsAndWaitInput struct {
	ClusterProxy                ClusterProxy
	Cluster                     *clusterv1.Cluster
	MachineDeployments          []*clusterv1.MachineDeployment
	WaitForMachinesToBeUpgraded []interface{}
}

// UpgradeMachineDeploymentInPlaceMutableFieldsAndWait upgrades in-place mutable fields in a MachineDeployment
// and waits for them to be in-place propagated to MachineSets and Machines.
func UpgradeMachineDeploymentInPlaceMutableFieldsAndWait(ctx context.Context, input UpgradeMachineDeploymentInPlaceMutableFieldsAndWaitInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for UpgradeMachineDeploymentInPlaceMutableFieldsAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling UpgradeMachineDeploymentInPlaceMutableFieldsAndWait")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling UpgradeMachineDeploymentInPlaceMutableFieldsAndWait")
	Expect(input.MachineDeployments).ToNot(BeEmpty(), "Invalid argument. input.MachineDeployments can't be empty when calling UpgradeMachineDeploymentInPlaceMutableFieldsAndWait")

	mgmtClient := input.ClusterProxy.GetClient()

	for _, deployment := range input.MachineDeployments {
		// Get MachineSet and Machines of the MachineDeployment before the upgrade.
		msListBeforeUpgrade, err := machineset.GetMachineSetsForDeployment(ctx, mgmtClient, client.ObjectKeyFromObject(deployment))
		Expect(err).ToNot(HaveOccurred())
		Expect(msListBeforeUpgrade).To(HaveLen(1), "expecting exactly 1 MachineSet")
		machineSetBeforeUpgrade := msListBeforeUpgrade[0]
		machinesBeforeUpgrade := GetMachinesByMachineDeployments(ctx, GetMachinesByMachineDeploymentsInput{
			Lister:            mgmtClient,
			ClusterName:       input.Cluster.Name,
			Namespace:         input.Cluster.Namespace,
			MachineDeployment: *deployment,
		})

		log.Logf("Patching in-place mutable fields of MachineDeployment %s", klog.KObj(deployment))
		patchHelper, err := patch.NewHelper(deployment, mgmtClient)
		Expect(err).ToNot(HaveOccurred())
		if deployment.Spec.Template.Labels == nil {
			deployment.Spec.Template.Labels = map[string]string{}
		}
		deployment.Spec.Template.Labels["new-label"] = "new-label-value"
		if deployment.Spec.Template.Annotations == nil {
			deployment.Spec.Template.Annotations = map[string]string{}
		}
		deployment.Spec.Template.Annotations["new-annotation"] = "new-annotation-value"
		deployment.Spec.Template.Spec.NodeDrainTimeout = &metav1.Duration{Duration: time.Duration(rand.Intn(20)) * time.Second}        //nolint:gosec
		deployment.Spec.Template.Spec.NodeDeletionTimeout = &metav1.Duration{Duration: time.Duration(rand.Intn(20)) * time.Second}     //nolint:gosec
		deployment.Spec.Template.Spec.NodeVolumeDetachTimeout = &metav1.Duration{Duration: time.Duration(rand.Intn(20)) * time.Second} //nolint:gosec
		Eventually(func() error {
			return patchHelper.Patch(ctx, deployment)
		}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to patch in-place mutable fields of MachineDeployment %s", klog.KObj(deployment))

		log.Logf("Waiting for in-place propagation to complete")
		Eventually(func(g Gomega) {
			// Get MachineSet and Machines of the MachineDeployment after the upgrade.
			msListAfterUpgrade, err := machineset.GetMachineSetsForDeployment(ctx, mgmtClient, client.ObjectKeyFromObject(deployment))
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(msListAfterUpgrade).To(HaveLen(1), "expecting exactly 1 MachineSet")
			machineSetAfterUpgrade := msListAfterUpgrade[0]
			machinesAfterUpgrade := GetMachinesByMachineDeployments(ctx, GetMachinesByMachineDeploymentsInput{
				Lister:            mgmtClient,
				ClusterName:       input.Cluster.Name,
				Namespace:         input.Cluster.Namespace,
				MachineDeployment: *deployment,
			})

			log.Logf("Verify MachineSet and Machines have not been replaced")
			g.Expect(machineSetAfterUpgrade.Name).To(Equal(machineSetBeforeUpgrade.Name))
			g.Expect(machineNames(machinesAfterUpgrade).Equal(machineNames(machinesBeforeUpgrade))).To(BeTrue())

			log.Logf("Verify fields have been propagated to MachineSet")
			// Label should be propagated to MS.Labels.
			g.Expect(machineSetAfterUpgrade.Labels).To(HaveKeyWithValue("new-label", "new-label-value"))
			// Annotation should not be propagated to MS.Annotations.
			g.Expect(machineSetAfterUpgrade.Annotations).ToNot(HaveKeyWithValue("new-annotation", "new-annotation-value"))
			// Label and annotation should be propagated to MS.Spec.Template.{Labels,Annotations}.
			g.Expect(machineSetAfterUpgrade.Spec.Template.Labels).To(HaveKeyWithValue("new-label", "new-label-value"))
			g.Expect(machineSetAfterUpgrade.Spec.Template.Annotations).To(HaveKeyWithValue("new-annotation", "new-annotation-value"))
			// Timeouts should be propagated.
			g.Expect(machineSetAfterUpgrade.Spec.Template.Spec.NodeDrainTimeout).To(Equal(deployment.Spec.Template.Spec.NodeDrainTimeout))
			g.Expect(machineSetAfterUpgrade.Spec.Template.Spec.NodeDeletionTimeout).To(Equal(deployment.Spec.Template.Spec.NodeDeletionTimeout))
			g.Expect(machineSetAfterUpgrade.Spec.Template.Spec.NodeVolumeDetachTimeout).To(Equal(deployment.Spec.Template.Spec.NodeVolumeDetachTimeout))

			log.Logf("Verify fields have been propagated to Machines")
			for _, m := range machinesAfterUpgrade {
				// Label and annotation should be propagated to MS.{Labels,Annotations}.
				g.Expect(m.Labels).To(HaveKeyWithValue("new-label", "new-label-value"))
				g.Expect(m.Annotations).To(HaveKeyWithValue("new-annotation", "new-annotation-value"))
				// Timeouts should be propagated.
				g.Expect(m.Spec.NodeDrainTimeout).To(Equal(deployment.Spec.Template.Spec.NodeDrainTimeout))
				g.Expect(m.Spec.NodeDeletionTimeout).To(Equal(deployment.Spec.Template.Spec.NodeDeletionTimeout))
				g.Expect(m.Spec.NodeVolumeDetachTimeout).To(Equal(deployment.Spec.Template.Spec.NodeVolumeDetachTimeout))
			}
		}, input.WaitForMachinesToBeUpgraded...).Should(Succeed())
	}
}

func machineNames(machines []clusterv1.Machine) sets.Set[string] {
	ret := sets.Set[string]{}
	for _, m := range machines {
		ret.Insert(m.Name)
	}
	return ret
}

func machineSetNames(machineSets []*clusterv1.MachineSet) sets.Set[string] {
	ret := sets.Set[string]{}
	for _, ms := range machineSets {
		ret.Insert(ms.Name)
	}
	return ret
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

	log.Logf("Scaling machine deployment %s from %d to %d replicas", klog.KObj(input.MachineDeployment), *input.MachineDeployment.Spec.Replicas, input.Replicas)
	patchHelper, err := patch.NewHelper(input.MachineDeployment, input.ClusterProxy.GetClient())
	Expect(err).ToNot(HaveOccurred())
	input.MachineDeployment.Spec.Replicas = pointer.Int32(input.Replicas)
	Eventually(func() error {
		return patchHelper.Patch(ctx, input.MachineDeployment)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to scale machine deployment %s", klog.KObj(input.MachineDeployment))

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
	}, input.WaitForMachineDeployments...).Should(Equal(int(*input.MachineDeployment.Spec.Replicas)), "Timed out waiting for Machine Deployment %s to have %d replicas", klog.KObj(input.MachineDeployment), *input.MachineDeployment.Spec.Replicas)
}

// ScaleAndWaitMachineDeploymentTopologyInput is the input for ScaleAndWaitMachineDeployment.
type ScaleAndWaitMachineDeploymentTopologyInput struct {
	ClusterProxy              ClusterProxy
	Cluster                   *clusterv1.Cluster
	Replicas                  int32
	WaitForMachineDeployments []interface{}
}

// ScaleAndWaitMachineDeploymentTopology scales MachineDeployment topology and waits until all machines have node ref and equal to Replicas.
func ScaleAndWaitMachineDeploymentTopology(ctx context.Context, input ScaleAndWaitMachineDeploymentTopologyInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for ScaleAndWaitMachineDeployment")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling ScaleAndWaitMachineDeployment")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling ScaleAndWaitMachineDeployment")
	Expect(input.Cluster.Spec.Topology.Workers).ToNot(BeNil(), "Invalid argument. input.Cluster must have MachineDeployment topologies")
	Expect(len(input.Cluster.Spec.Topology.Workers.MachineDeployments) >= 1).To(BeTrue(), "Invalid argument. input.Cluster must have at least one MachineDeployment topology")

	mdTopology := input.Cluster.Spec.Topology.Workers.MachineDeployments[0]
	log.Logf("Scaling machine deployment topology %s from %d to %d replicas", mdTopology.Name, *mdTopology.Replicas, input.Replicas)
	patchHelper, err := patch.NewHelper(input.Cluster, input.ClusterProxy.GetClient())
	Expect(err).ToNot(HaveOccurred())
	mdTopology.Replicas = pointer.Int32(input.Replicas)
	input.Cluster.Spec.Topology.Workers.MachineDeployments[0] = mdTopology
	Eventually(func() error {
		return patchHelper.Patch(ctx, input.Cluster)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to scale machine deployment topology %s", mdTopology.Name)

	log.Logf("Waiting for correct number of replicas to exist")
	deploymentList := &clusterv1.MachineDeploymentList{}
	Eventually(func() error {
		return input.ClusterProxy.GetClient().List(ctx, deploymentList,
			client.InNamespace(input.Cluster.Namespace),
			client.MatchingLabels{
				clusterv1.ClusterNameLabel:                          input.Cluster.Name,
				clusterv1.ClusterTopologyMachineDeploymentNameLabel: mdTopology.Name,
			},
		)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to list MachineDeployments object for Cluster %s", klog.KRef(input.Cluster.Namespace, input.Cluster.Name))

	Expect(deploymentList.Items).To(HaveLen(1))
	md := deploymentList.Items[0]

	Eventually(func() (int, error) {
		selectorMap, err := metav1.LabelSelectorAsMap(&md.Spec.Selector)
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
	}, input.WaitForMachineDeployments...).Should(Equal(int(*md.Spec.Replicas)), "Timed out waiting for Machine Deployment %s to have %d replicas", klog.KObj(&md), *md.Spec.Replicas)
}
