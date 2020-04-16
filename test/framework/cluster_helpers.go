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
	"sigs.k8s.io/cluster-api/test/framework/options"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

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

// GetClusterByNameInput is the input for GetClusterByName.
type GetClusterByNameInput struct {
	Getter    Getter
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

// byClusterOptions returns a set of ListOptions that allows to identify all the objects belonging to a Cluster.
func byClusterOptions(name, namespace string) []client.ListOption {
	return []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{
			clusterv1.ClusterLabelName: name,
		},
	}
}
