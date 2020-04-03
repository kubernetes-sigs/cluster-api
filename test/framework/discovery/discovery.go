// +build e2e

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

package discovery

import (
	"context"

	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Provides methods for discovering Cluster API objects existing in the management cluster.

// GetControllerDeploymentsInput is the input for GetControllerDeployments.
type GetControllerDeploymentsInput struct {
	Lister framework.Lister
}

// GetControllerDeployments returns all the deployment for the cluster API controllers existing in a management cluster.
func GetControllerDeployments(ctx context.Context, input GetControllerDeploymentsInput) []*appsv1.Deployment {
	deploymentList := &appsv1.DeploymentList{}
	Expect(input.Lister.List(ctx, deploymentList, capiProviderOptions()...)).To(Succeed(), "Failed to list deployments for the cluster API controllers")

	deployments := make([]*appsv1.Deployment, len(deploymentList.Items))
	for i := range deploymentList.Items {
		deployments[i] = &deploymentList.Items[i]
	}
	return deployments
}

// GetClusterByNameInput is the input for GetClusterByName.
type GetClusterByNameInput struct {
	Getter    framework.Getter
	Name      string
	Namespace string
}

// GetClusterByName returns a Cluster object given his name
func GetClusterByName(ctx context.Context, input GetClusterByNameInput) *clusterv1.Cluster {
	cluster := &clusterv1.Cluster{}
	key := client.ObjectKey{
		Namespace: input.Namespace,
		Name:      input.Name,
	}
	Expect(input.Getter.Get(ctx, key, cluster)).To(Succeed(), "Failed to get Cluster object %s/%s", input.Namespace, input.Name)
	return cluster
}

// GetKubeadmControlPlaneByClusterInput is the input for GetKubeadmControlPlaneByCluster.
type GetKubeadmControlPlaneByClusterInput struct {
	Lister      framework.Lister
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

// GetMachineDeploymentsByClusterInput is the input for GetMachineDeploymentsByCluster.
type GetMachineDeploymentsByClusterInput struct {
	Lister      framework.Lister
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
		deployments[i] = &deploymentList.Items[i]
	}
	return deployments
}

// capiProviderOptions returns a set of ListOptions that allows to identify all the objects belonging to Cluster API providers.
func capiProviderOptions() []client.ListOption {
	return []client.ListOption{
		client.HasLabels{clusterv1.ProviderLabelName},
	}
}

// byClusterOptions returns a set of ListOptions that allows to identify all the objects belonging to a Cluster.
func byClusterOptions(name, namespace string) []client.ListOption {
	return []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{
			clusterv1.ClusterLabelName: name,
		},
	}
}
