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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework/internal/log"
	"sigs.k8s.io/cluster-api/util/patch"
)

// CreateClusterInput is the input for CreateCluster.
type CreateClusterInput struct {
	Creator      Creator
	Cluster      *clusterv1.Cluster
	InfraCluster client.Object
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
			log.Logf("Failed to create a cluster: %+v", err)
			return err
		}
		return nil
	}, intervals...).Should(Succeed())
}

// GetAllClustersByNamespaceInput is the input for GetAllClustersByNamespace.
type GetAllClustersByNamespaceInput struct {
	Lister    Lister
	Namespace string
}

// GetAllClustersByNamespace returns the list of Cluster object in a namespace.
func GetAllClustersByNamespace(ctx context.Context, input GetAllClustersByNamespaceInput) []*clusterv1.Cluster {
	clusterList := &clusterv1.ClusterList{}
	Expect(input.Lister.List(ctx, clusterList, client.InNamespace(input.Namespace))).To(Succeed(), "Failed to list clusters in namespace %s", input.Namespace)

	clusters := make([]*clusterv1.Cluster, len(clusterList.Items))
	for i := range clusterList.Items {
		clusters[i] = &clusterList.Items[i]
	}
	return clusters
}

// GetClusterByNameInput is the input for GetClusterByName.
type GetClusterByNameInput struct {
	Getter    Getter
	Name      string
	Namespace string
}

// GetClusterByName returns a Cluster object given his name.
func GetClusterByName(ctx context.Context, input GetClusterByNameInput) *clusterv1.Cluster {
	cluster := &clusterv1.Cluster{}
	key := client.ObjectKey{
		Namespace: input.Namespace,
		Name:      input.Name,
	}
	Expect(input.Getter.Get(ctx, key, cluster)).To(Succeed(), "Failed to get Cluster object %s/%s", input.Namespace, input.Name)
	return cluster
}

// PatchClusterLabelInput is the input for PatchClusterLabel.
type PatchClusterLabelInput struct {
	ClusterProxy ClusterProxy
	Cluster      *clusterv1.Cluster
	Labels       map[string]string
}

// PatchClusterLabel patches labels to a cluster.
func PatchClusterLabel(ctx context.Context, input PatchClusterLabelInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for PatchClusterLabel")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling PatchClusterLabel")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling PatchClusterLabel")
	Expect(input.Labels).ToNot(BeEmpty(), "Invalid argument. input.Labels can't be empty when calling PatchClusterLabel")

	log.Logf("Patching the label to the cluster")
	patchHelper, err := patch.NewHelper(input.Cluster, input.ClusterProxy.GetClient())
	Expect(err).ToNot(HaveOccurred())
	input.Cluster.SetLabels(input.Labels)
	Expect(patchHelper.Patch(ctx, input.Cluster)).To(Succeed())
}

// WaitForClusterToProvisionInput is the input for WaitForClusterToProvision.
type WaitForClusterToProvisionInput struct {
	Getter  Getter
	Cluster *clusterv1.Cluster
}

// WaitForClusterToProvision will wait for a cluster to have a phase status of provisioned.
func WaitForClusterToProvision(ctx context.Context, input WaitForClusterToProvisionInput, intervals ...interface{}) *clusterv1.Cluster {
	cluster := &clusterv1.Cluster{}
	By("Waiting for cluster to enter the provisioned phase")
	Eventually(func() (string, error) {
		key := client.ObjectKey{
			Namespace: input.Cluster.GetNamespace(),
			Name:      input.Cluster.GetName(),
		}
		if err := input.Getter.Get(ctx, key, cluster); err != nil {
			return "", err
		}
		return cluster.Status.Phase, nil
	}, intervals...).Should(Equal(string(clusterv1.ClusterPhaseProvisioned)))
	return cluster
}

// DeleteClusterInput is the input for DeleteCluster.
type DeleteClusterInput struct {
	Deleter Deleter
	Cluster *clusterv1.Cluster
}

// DeleteCluster deletes the cluster and waits for everything the cluster owned to actually be gone.
func DeleteCluster(ctx context.Context, input DeleteClusterInput) {
	By(fmt.Sprintf("Deleting cluster %s", input.Cluster.GetName()))
	Expect(input.Deleter.Delete(ctx, input.Cluster)).To(Succeed())
}

// WaitForClusterDeletedInput is the input for WaitForClusterDeleted.
type WaitForClusterDeletedInput struct {
	Getter  Getter
	Cluster *clusterv1.Cluster
}

// WaitForClusterDeleted waits until the cluster object has been deleted.
func WaitForClusterDeleted(ctx context.Context, input WaitForClusterDeletedInput, intervals ...interface{}) {
	By(fmt.Sprintf("Waiting for cluster %s to be deleted", input.Cluster.GetName()))
	Eventually(func() bool {
		cluster := &clusterv1.Cluster{}
		key := client.ObjectKey{
			Namespace: input.Cluster.GetNamespace(),
			Name:      input.Cluster.GetName(),
		}
		return apierrors.IsNotFound(input.Getter.Get(ctx, key, cluster))
	}, intervals...).Should(BeTrue())
}

// DiscoveryAndWaitForClusterInput is the input type for DiscoveryAndWaitForCluster.
type DiscoveryAndWaitForClusterInput struct {
	Getter    Getter
	Namespace string
	Name      string
}

// DiscoveryAndWaitForCluster discovers a cluster object in a namespace and waits for the cluster infrastructure to be provisioned.
func DiscoveryAndWaitForCluster(ctx context.Context, input DiscoveryAndWaitForClusterInput, intervals ...interface{}) *clusterv1.Cluster {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DiscoveryAndWaitForCluster")
	Expect(input.Getter).ToNot(BeNil(), "Invalid argument. input.Getter can't be nil when calling DiscoveryAndWaitForCluster")
	Expect(input.Namespace).ToNot(BeNil(), "Invalid argument. input.Namespace can't be empty when calling DiscoveryAndWaitForCluster")
	Expect(input.Name).ToNot(BeNil(), "Invalid argument. input.Name can't be empty when calling DiscoveryAndWaitForCluster")

	cluster := GetClusterByName(ctx, GetClusterByNameInput{
		Getter:    input.Getter,
		Name:      input.Name,
		Namespace: input.Namespace,
	})
	Expect(cluster).ToNot(BeNil(), "Failed to get the Cluster object")

	// NOTE: We intentionally return the provisioned Cluster because it also contains
	// the reconciled ControlPlane ref and InfrastructureCluster ref when using a ClusterClass.
	cluster = WaitForClusterToProvision(ctx, WaitForClusterToProvisionInput{
		Getter:  input.Getter,
		Cluster: cluster,
	}, intervals...)

	return cluster
}

// DeleteClusterAndWaitInput is the input type for DeleteClusterAndWait.
type DeleteClusterAndWaitInput struct {
	Client  client.Client
	Cluster *clusterv1.Cluster
}

// DeleteClusterAndWait deletes a cluster object and waits for it to be gone.
func DeleteClusterAndWait(ctx context.Context, input DeleteClusterAndWaitInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DeleteClusterAndWait")
	Expect(input.Client).ToNot(BeNil(), "Invalid argument. input.Client can't be nil when calling DeleteClusterAndWait")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling DeleteClusterAndWait")

	DeleteCluster(ctx, DeleteClusterInput{
		Deleter: input.Client,
		Cluster: input.Cluster,
	})

	log.Logf("Waiting for the Cluster object to be deleted")
	WaitForClusterDeleted(ctx, WaitForClusterDeletedInput{
		Getter:  input.Client,
		Cluster: input.Cluster,
	}, intervals...)

	//TODO: consider if to move in another func (what if there are more than one cluster?)
	log.Logf("Check for all the Cluster API resources being deleted")
	resources := GetCAPIResources(ctx, GetCAPIResourcesInput{
		Lister:    input.Client,
		Namespace: input.Cluster.Namespace,
	})
	Expect(resources).To(BeEmpty(), "There are still Cluster API resources in the %q namespace", input.Cluster.Namespace)
}

// DeleteAllClustersAndWaitInput is the input type for DeleteAllClustersAndWait.
type DeleteAllClustersAndWaitInput struct {
	Client    client.Client
	Namespace string
}

// DeleteAllClustersAndWait deletes a cluster object and waits for it to be gone.
func DeleteAllClustersAndWait(ctx context.Context, input DeleteAllClustersAndWaitInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DeleteAllClustersAndWait")
	Expect(input.Client).ToNot(BeNil(), "Invalid argument. input.Client can't be nil when calling DeleteAllClustersAndWait")
	Expect(input.Namespace).ToNot(BeEmpty(), "Invalid argument. input.Namespace can't be empty when calling DeleteAllClustersAndWait")

	clusters := GetAllClustersByNamespace(ctx, GetAllClustersByNamespaceInput{
		Lister:    input.Client,
		Namespace: input.Namespace,
	})

	for _, c := range clusters {
		DeleteCluster(ctx, DeleteClusterInput{
			Deleter: input.Client,
			Cluster: c,
		})
	}

	for _, c := range clusters {
		log.Logf("Waiting for the Cluster %s/%s to be deleted", c.Namespace, c.Name)
		WaitForClusterDeleted(ctx, WaitForClusterDeletedInput{
			Getter:  input.Client,
			Cluster: c,
		}, intervals...)
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
