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
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlclient "sigs.k8s.io/cluster-api/cmd/clusterctl/client"
	cmdtree "sigs.k8s.io/cluster-api/internal/util/tree"
	. "sigs.k8s.io/cluster-api/test/framework/ginkgoextensions"
	"sigs.k8s.io/cluster-api/test/framework/internal/log"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
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
	Eventually(func() error {
		return input.Creator.Create(ctx, input.InfraCluster)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to create InfrastructureCluster %s", klog.KObj(input.InfraCluster))

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
	}, intervals...).Should(Succeed(), "Failed to create Cluster %s", klog.KObj(input.Cluster))
}

// GetAllClustersByNamespaceInput is the input for GetAllClustersByNamespace.
type GetAllClustersByNamespaceInput struct {
	Lister    Lister
	Namespace string
}

// GetAllClustersByNamespace returns the list of Cluster object in a namespace.
func GetAllClustersByNamespace(ctx context.Context, input GetAllClustersByNamespaceInput) []*clusterv1.Cluster {
	clusterList := &clusterv1.ClusterList{}
	Eventually(func() error {
		return input.Lister.List(ctx, clusterList, client.InNamespace(input.Namespace))
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to list clusters in namespace %s", input.Namespace)

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
	Eventually(func() error {
		return input.Getter.Get(ctx, key, cluster)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to get Cluster object %s", klog.KRef(input.Namespace, input.Name))
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
	Eventually(func() error {
		return patchHelper.Patch(ctx, input.Cluster)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to patch label to cluster %s", klog.KObj(input.Cluster))
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
	}, intervals...).Should(Equal(string(clusterv1.ClusterPhaseProvisioned)), "Timed out waiting for Cluster %s to provision", klog.KObj(input.Cluster))
	return cluster
}

// DeleteClusterInput is the input for DeleteCluster.
type DeleteClusterInput struct {
	Deleter Deleter
	Cluster *clusterv1.Cluster
}

// DeleteCluster deletes the cluster.
func DeleteCluster(ctx context.Context, input DeleteClusterInput) {
	Byf("Deleting cluster %s", klog.KObj(input.Cluster))
	Expect(input.Deleter.Delete(ctx, input.Cluster)).To(Succeed())
}

// WaitForClusterDeletedInput is the input for WaitForClusterDeleted.
type WaitForClusterDeletedInput struct {
	ClusterProxy         ClusterProxy
	ClusterctlConfigPath string
	Cluster              *clusterv1.Cluster
	// ArtifactFolder, if set, clusters will be dumped if deletion times out
	ArtifactFolder string
}

// WaitForClusterDeleted waits until the cluster object has been deleted.
func WaitForClusterDeleted(ctx context.Context, input WaitForClusterDeletedInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for WaitForClusterDeleted")
	Expect(input.ClusterProxy).NotTo(BeNil(), "input.ClusterProxy is required for WaitForClusterDeleted")
	Expect(input.ClusterctlConfigPath).NotTo(BeEmpty(), "input.ClusterctlConfigPath is required for WaitForClusterDeleted")
	Expect(input.Cluster).NotTo(BeNil(), "input.Cluster is required for WaitForClusterDeleted")

	Byf("Waiting for cluster %s to be deleted", klog.KObj(input.Cluster))
	// Note: dumpArtifactsOnDeletionTimeout is passed in as a func so it gets only executed if and after the Eventually failed.
	Eventually(func() bool {
		cluster := &clusterv1.Cluster{}
		key := client.ObjectKey{
			Namespace: input.Cluster.GetNamespace(),
			Name:      input.Cluster.GetName(),
		}
		return apierrors.IsNotFound(input.ClusterProxy.GetClient().Get(ctx, key, cluster))
	}, intervals...).Should(BeTrue(), func() string {
		return dumpArtifactsOnDeletionTimeout(ctx, input.ClusterProxy, input.Cluster, input.ClusterctlConfigPath, input.ArtifactFolder)
	})
}

func dumpArtifactsOnDeletionTimeout(ctx context.Context, clusterProxy ClusterProxy, cluster *clusterv1.Cluster, clusterctlConfigPath, artifactFolder string) string {
	if artifactFolder != "" {
		// Dump all Cluster API related resources to artifacts.
		DumpAllResources(ctx, DumpAllResourcesInput{
			Lister:               clusterProxy.GetClient(),
			KubeConfigPath:       clusterProxy.GetKubeconfigPath(),
			ClusterctlConfigPath: clusterctlConfigPath,
			Namespace:            cluster.Namespace,
			LogPath:              filepath.Join(artifactFolder, "clusters-afterDeletionTimedOut", cluster.Name, "resources"),
		})
	}

	// Try to get more details about why Cluster deletion timed out.
	if err := clusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(cluster), cluster); err == nil {
		if c := v1beta2conditions.Get(cluster, clusterv1.MachineDeletingV1Beta2Condition); c != nil {
			return fmt.Sprintf("waiting for cluster deletion timed out:\ncondition: %s\nmessage: %s", c.Type, c.Message)
		}
	}

	return "waiting for cluster deletion timed out"
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

	var cluster *clusterv1.Cluster
	Eventually(func() bool {
		cluster = GetClusterByName(ctx, GetClusterByNameInput{
			Getter:    input.Getter,
			Name:      input.Name,
			Namespace: input.Namespace,
		})
		return cluster != nil
	}, retryableOperationTimeout, retryableOperationInterval).Should(BeTrue(), "Failed to get Cluster object %s", klog.KRef(input.Namespace, input.Name))

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
	ClusterProxy         ClusterProxy
	ClusterctlConfigPath string
	Cluster              *clusterv1.Cluster
	// ArtifactFolder, if set, clusters will be dumped if deletion times out
	ArtifactFolder string
}

// DeleteClusterAndWait deletes a cluster object and waits for it to be gone.
func DeleteClusterAndWait(ctx context.Context, input DeleteClusterAndWaitInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DeleteClusterAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling DeleteClusterAndWait")
	Expect(input.ClusterctlConfigPath).ToNot(BeNil(), "Invalid argument. input.ClusterctlConfigPath can't be nil when calling DeleteClusterAndWait")
	Expect(input.Cluster).ToNot(BeEmpty(), "Invalid argument. input.Cluster can't be empty when calling DeleteClusterAndWait")

	DeleteCluster(ctx, DeleteClusterInput{
		Deleter: input.ClusterProxy.GetClient(),
		Cluster: input.Cluster,
	})

	log.Logf("Waiting for the Cluster object to be deleted")
	WaitForClusterDeleted(ctx, WaitForClusterDeletedInput(input), intervals...)

	// TODO: consider if to move in another func (what if there are more than one cluster?)
	log.Logf("Check for all the Cluster API resources being deleted")
	Eventually(func() []*unstructured.Unstructured {
		return GetCAPIResources(ctx, GetCAPIResourcesInput{
			Lister:    input.ClusterProxy.GetClient(),
			Namespace: input.Cluster.Namespace,
		})
	}, retryableOperationTimeout, retryableOperationInterval).Should(BeEmpty(), "There are still Cluster API resources in the %q namespace", input.Cluster.Namespace)
}

// DeleteAllClustersAndWaitInput is the input type for DeleteAllClustersAndWait.
type DeleteAllClustersAndWaitInput struct {
	ClusterProxy         ClusterProxy
	ClusterctlConfigPath string
	Namespace            string
	// ArtifactFolder, if set, clusters will be dumped if deletion times out
	ArtifactFolder string
}

// DeleteAllClustersAndWait deletes a cluster object and waits for it to be gone.
func DeleteAllClustersAndWait(ctx context.Context, input DeleteAllClustersAndWaitInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DeleteAllClustersAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling DeleteAllClustersAndWait")
	Expect(input.ClusterctlConfigPath).ToNot(BeNil(), "Invalid argument. input.ClusterctlConfigPath can't be nil when calling DeleteAllClustersAndWait")
	Expect(input.Namespace).ToNot(BeEmpty(), "Invalid argument. input.Namespace can't be empty when calling DeleteAllClustersAndWait")

	clusters := GetAllClustersByNamespace(ctx, GetAllClustersByNamespaceInput{
		Lister:    input.ClusterProxy.GetClient(),
		Namespace: input.Namespace,
	})

	for _, c := range clusters {
		DeleteCluster(ctx, DeleteClusterInput{
			Deleter: input.ClusterProxy.GetClient(),
			Cluster: c,
		})
	}

	for _, c := range clusters {
		log.Logf("Waiting for the Cluster %s to be deleted", klog.KObj(c))
		WaitForClusterDeleted(ctx, WaitForClusterDeletedInput{
			ClusterProxy:         input.ClusterProxy,
			ClusterctlConfigPath: input.ClusterctlConfigPath,
			Cluster:              c,
			ArtifactFolder:       input.ArtifactFolder,
		}, intervals...)
	}
}

// byClusterOptions returns a set of ListOptions that allows to identify all the objects belonging to a Cluster.
func byClusterOptions(name, namespace string) []client.ListOption {
	return []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{
			clusterv1.ClusterNameLabel: name,
		},
	}
}

type DescribeClusterInput struct {
	ClusterctlConfigPath string
	LogFolder            string
	KubeConfigPath       string
	Namespace            string
	Name                 string
}

func DescribeCluster(ctx context.Context, input DescribeClusterInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DescribeCluster")
	Expect(input.KubeConfigPath).ToNot(BeEmpty(), "Invalid argument. input.KubeConfigPath can't be empty when calling DescribeCluster")
	Expect(input.ClusterctlConfigPath).ToNot(BeNil(), "Invalid argument. input.ClusterctlConfigPath can't be nil when calling DescribeCluster")
	Expect(input.LogFolder).ToNot(BeEmpty(), "Invalid argument. input.LogFolder can't be empty when calling DescribeCluster")
	Expect(input.Namespace).ToNot(BeNil(), "Invalid argument. input.Namespace can't be nil when calling DescribeCluster")
	Expect(input.Name).ToNot(BeNil(), "Invalid argument. input.Name can't be nil when calling DescribeCluster")

	log.Logf("clusterctl describe cluster %s --show-conditions=all --show-machinesets=true --grouping=false --echo=true --v1beta2", input.Name)

	clusterctlClient, err := clusterctlclient.New(ctx, input.ClusterctlConfigPath)
	Expect(err).ToNot(HaveOccurred(), "Failed to create the clusterctl client library")

	tree, err := clusterctlClient.DescribeCluster(ctx, clusterctlclient.DescribeClusterOptions{
		Kubeconfig: clusterctlclient.Kubeconfig{
			Path:    input.KubeConfigPath,
			Context: "",
		},
		Namespace:               input.Namespace,
		ClusterName:             input.Name,
		ShowOtherConditions:     "all",
		ShowClusterResourceSets: false,
		ShowTemplates:           false,
		ShowMachineSets:         true,
		AddTemplateVirtualNode:  true,
		Echo:                    true,
		Grouping:                false,
		V1Beta2:                 true,
	})
	Expect(err).ToNot(HaveOccurred(), "Failed to run clusterctl describe")

	Expect(os.MkdirAll(input.LogFolder, 0750)).To(Succeed(), "Failed to create log folder %s", input.LogFolder)

	f, err := os.Create(filepath.Join(input.LogFolder, fmt.Sprintf("clusterctl-describe-cluster-%s.txt", input.Name)))
	Expect(err).ToNot(HaveOccurred(), "Failed to create a file for saving clusterctl describe output")

	defer f.Close()

	w := bufio.NewWriter(f)
	cmdtree.PrintObjectTreeV1Beta2(tree, w)
	if CurrentSpecReport().Failed() {
		cmdtree.PrintObjectTreeV1Beta2(tree, GinkgoWriter)
	}
	Expect(w.Flush()).To(Succeed(), "Failed to save clusterctl describe output")
}

type DescribeAllClusterInput struct {
	Lister               Lister
	KubeConfigPath       string
	ClusterctlConfigPath string
	LogFolder            string
	Namespace            string
}

func DescribeAllCluster(ctx context.Context, input DescribeAllClusterInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DescribeAllCluster")
	Expect(input.Lister).ToNot(BeNil(), "Invalid argument. input.Lister can't be nil when calling DescribeAllCluster")
	Expect(input.KubeConfigPath).ToNot(BeEmpty(), "Invalid argument. input.KubeConfigPath can't be empty when calling DescribeAllCluster")
	Expect(input.ClusterctlConfigPath).ToNot(BeNil(), "Invalid argument. input.ClusterctlConfigPath can't be nil when calling DescribeAllCluster")
	Expect(input.LogFolder).ToNot(BeEmpty(), "Invalid argument. input.LogFolder can't be empty when calling DescribeAllCluster")
	Expect(input.Namespace).ToNot(BeNil(), "Invalid argument. input.Namespace can't be nil when calling DescribeAllCluster")

	clusters := &clusterv1.ClusterList{}
	Eventually(func() error {
		return input.Lister.List(ctx, clusters, client.InNamespace(input.Namespace))
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to list clusters in namespace %s", input.Namespace)

	for _, c := range clusters.Items {
		DescribeCluster(ctx, DescribeClusterInput{
			ClusterctlConfigPath: input.ClusterctlConfigPath,
			LogFolder:            input.LogFolder,
			KubeConfigPath:       input.KubeConfigPath,
			Namespace:            c.Namespace,
			Name:                 c.Name,
		})
	}
}
