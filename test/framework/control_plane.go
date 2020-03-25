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

package framework

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	cabpkv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1alpha3"
	"sigs.k8s.io/cluster-api/test/framework/options"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Interfaces to scope down client.Client

// Getter can get resources.
type Getter interface {
	Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error
}

// Creator can creates resources.
type Creator interface {
	Create(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error
}

// Lister can lists resources.
type Lister interface {
	List(ctx context.Context, list runtime.Object, opts ...client.ListOption) error
}

// Deleter can delete resources.
type Deleter interface {
	Delete(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error
}

// GetLister can get and list resources.
type GetLister interface {
	Getter
	Lister
}

// CreateRelatedResourcesInput is the input type for CreateRelatedResources.
type CreateRelatedResourcesInput struct {
	Creator          Creator
	RelatedResources []runtime.Object
}

// CreateRelatedResources is used to create runtime.Objects.
func CreateRelatedResources(ctx context.Context, input CreateRelatedResourcesInput, intervals ...interface{}) {
	By("creating related resources")
	for i := range input.RelatedResources {
		obj := input.RelatedResources[i]
		By(fmt.Sprintf("creating a/an %s resource", obj.GetObjectKind().GroupVersionKind()))
		Eventually(func() error {
			return input.Creator.Create(ctx, obj)
		}, intervals...).Should(Succeed())
	}
}

// CreateClusterInput is the input for CreateCluster.
type CreateClusterInput struct {
	Creator      Creator
	Cluster      *clusterv1.Cluster
	InfraCluster runtime.Object
}

// CreateCluster will create the Cluster and InfraCluster objects.
func CreateCluster(ctx context.Context, input CreateClusterInput, intervals ...interface{}) {
	By("creating an InfrastructureCluster resource")
	Expect(input.Creator.Create(ctx, input.InfraCluster)).To(Succeed())

	// This call happens in an eventually because of a race condition with the
	// webhook server. If the latter isn't fully online then this call will
	// fail.
	By("creating a Cluster resource linked to the InfrastructureCluster resource")
	Eventually(func() error {
		if err := input.Creator.Create(ctx, input.Cluster); err != nil {
			fmt.Printf("%+v\n", err)
			return err
		}
		return nil
	}, intervals...).Should(Succeed())
}

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

// CreateMachineDeploymentInput is the input for CreateMachineDeployment.
type CreateMachineDeploymentInput struct {
	Creator                 Creator
	MachineDeployment       *clusterv1.MachineDeployment
	BootstrapConfigTemplate runtime.Object
	InfraMachineTemplate    runtime.Object
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

// WaitForMachineDeploymentNodesToExistInput is the input for WaitForMachineDeploymentNodesToExist.
type WaitForMachineDeploymentNodesToExistInput struct {
	Lister            Lister
	Cluster           *clusterv1.Cluster
	MachineDeployment *clusterv1.MachineDeployment
}

// WaitForMachineDeploymentNodesToExist waits until all nodes associated with a machine deployment exist.
func WaitForMachineDeploymentNodesToExist(ctx context.Context, input WaitForMachineDeploymentNodesToExistInput, intervals ...interface{}) {
	By("waiting for the workload nodes to exist")
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

// WaitForClusterToProvisionInput is the input for WaitForClusterToProvision.
type WaitForClusterToProvisionInput struct {
	Getter  Getter
	Cluster *clusterv1.Cluster
}

// WaitForClusterToProvision will wait for a cluster to have a phase status of provisioned.
func WaitForClusterToProvision(ctx context.Context, input WaitForClusterToProvisionInput, intervals ...interface{}) {
	By("waiting for cluster to enter the provisioned phase")
	Eventually(func() (string, error) {
		cluster := &clusterv1.Cluster{}
		key := client.ObjectKey{
			Namespace: input.Cluster.GetNamespace(),
			Name:      input.Cluster.GetName(),
		}
		if err := input.Getter.Get(ctx, key, cluster); err != nil {
			return "", err
		}
		return cluster.Status.Phase, nil
	}, intervals...).Should(Equal(string(clusterv1.ClusterPhaseProvisioned)))
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

// DeleteClusterInput is the input for DeleteCluster.
type DeleteClusterInput struct {
	Deleter Deleter
	Cluster *clusterv1.Cluster
}

// DeleteCluster deletes the cluster and waits for everything the cluster owned to actually be gone.
func DeleteCluster(ctx context.Context, input DeleteClusterInput) {
	if options.SkipResourceCleanup {
		return
	}
	By(fmt.Sprintf("deleting cluster %s", input.Cluster.GetName()))
	Expect(input.Deleter.Delete(ctx, input.Cluster)).To(Succeed())
}

// WaitForClusterDeletedInput is the input for WaitForClusterDeleted.
type WaitForClusterDeletedInput struct {
	Getter  Getter
	Cluster *clusterv1.Cluster
}

// WaitForClusterDeleted waits until the cluster object has been deleted.
func WaitForClusterDeleted(ctx context.Context, input WaitForClusterDeletedInput, intervals ...interface{}) {
	if options.SkipResourceCleanup {
		return
	}
	By(fmt.Sprintf("waiting for cluster %s to be deleted", input.Cluster.GetName()))
	Eventually(func() bool {
		cluster := &clusterv1.Cluster{}
		key := client.ObjectKey{
			Namespace: input.Cluster.GetNamespace(),
			Name:      input.Cluster.GetName(),
		}
		return apierrors.IsNotFound(input.Getter.Get(ctx, key, cluster))
	}, intervals...).Should(BeTrue())
}

// AssertAllClusterAPIResourcesAreGoneInput is the input for AssertAllClusterAPIResourcesAreGone.
type AssertAllClusterAPIResourcesAreGoneInput struct {
	Lister  Lister
	Cluster *clusterv1.Cluster
}

// AssertAllClusterAPIResourcesAreGone ensures that all known Cluster API resources have been remvoed.
func AssertAllClusterAPIResourcesAreGone(ctx context.Context, input AssertAllClusterAPIResourcesAreGoneInput) {
	if options.SkipResourceCleanup {
		return
	}
	lbl, err := labels.Parse(fmt.Sprintf("%s=%s", clusterv1.ClusterLabelName, input.Cluster.GetClusterName()))
	Expect(err).ToNot(HaveOccurred())
	opt := &client.ListOptions{LabelSelector: lbl}

	By("ensuring all CAPI artifacts have been deleted")

	ml := &clusterv1.MachineList{}
	Expect(input.Lister.List(ctx, ml, opt)).To(Succeed())
	Expect(ml.Items).To(HaveLen(0))

	msl := &clusterv1.MachineSetList{}
	Expect(input.Lister.List(ctx, msl, opt)).To(Succeed())
	Expect(msl.Items).To(HaveLen(0))

	mdl := &clusterv1.MachineDeploymentList{}
	Expect(input.Lister.List(ctx, mdl, opt)).To(Succeed())
	Expect(mdl.Items).To(HaveLen(0))

	mpl := &expv1.MachinePoolList{}
	Expect(input.Lister.List(ctx, mpl, opt)).To(Succeed())
	Expect(mpl.Items).To(HaveLen(0))

	kcpl := &controlplanev1.KubeadmControlPlaneList{}
	Expect(input.Lister.List(ctx, kcpl, opt)).To(Succeed())
	Expect(kcpl.Items).To(HaveLen(0))

	kcl := &cabpkv1.KubeadmConfigList{}
	Expect(input.Lister.List(ctx, kcl, opt)).To(Succeed())
	Expect(kcl.Items).To(HaveLen(0))

	sl := &corev1.SecretList{}
	Expect(input.Lister.List(ctx, sl, opt)).To(Succeed())
	Expect(sl.Items).To(HaveLen(0))
}
