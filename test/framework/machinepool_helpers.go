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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework/internal/log"
	"sigs.k8s.io/cluster-api/util/patch"
)

// GetMachinePoolsByClusterInput is the input for GetMachinePoolsByCluster.
type GetMachinePoolsByClusterInput struct {
	Lister      Lister
	ClusterName string
	Namespace   string
}

// GetMachinePoolsByCluster returns the MachinePools objects for a cluster.
// Important! this method relies on labels that are created by the CAPI controllers during the first reconciliation, so
// it is necessary to ensure this is already happened before calling it.
func GetMachinePoolsByCluster(ctx context.Context, input GetMachinePoolsByClusterInput) []*expv1.MachinePool {
	Expect(ctx).NotTo(BeNil(), "ctx is required for GetMachinePoolsByCluster")
	Expect(input.Lister).ToNot(BeNil(), "Invalid argument. input.Lister can't be nil when calling GetMachinePoolsByCluster")
	Expect(input.Namespace).ToNot(BeEmpty(), "Invalid argument. input.Namespace can't be empty when calling GetMachinePoolsByCluster")
	Expect(input.ClusterName).ToNot(BeEmpty(), "Invalid argument. input.ClusterName can't be empty when calling GetMachinePoolsByCluster")

	mpList := &expv1.MachinePoolList{}
	Expect(input.Lister.List(ctx, mpList, byClusterOptions(input.ClusterName, input.Namespace)...)).To(Succeed(), "Failed to list MachinePools object for Cluster %s/%s", input.Namespace, input.ClusterName)

	mps := make([]*expv1.MachinePool, len(mpList.Items))
	for i := range mpList.Items {
		mps[i] = &mpList.Items[i]
	}
	return mps
}

// WaitForMachinePoolNodesToExistInput is the input for WaitForMachinePoolNodesToExist.
type WaitForMachinePoolNodesToExistInput struct {
	Getter      Getter
	MachinePool *expv1.MachinePool
}

// WaitForMachinePoolNodesToExist waits until all nodes associated with a machine pool exist.
func WaitForMachinePoolNodesToExist(ctx context.Context, input WaitForMachinePoolNodesToExistInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for WaitForMachinePoolNodesToExist")
	Expect(input.Getter).ToNot(BeNil(), "Invalid argument. input.Getter can't be nil when calling WaitForMachinePoolNodesToExist")
	Expect(input.MachinePool).ToNot(BeNil(), "Invalid argument. input.MachinePool can't be nil when calling WaitForMachinePoolNodesToExist")

	By("Waiting for the machine pool workload nodes")
	Eventually(func() (int, error) {
		nn := client.ObjectKey{
			Namespace: input.MachinePool.Namespace,
			Name:      input.MachinePool.Name,
		}

		if err := input.Getter.Get(ctx, nn, input.MachinePool); err != nil {
			return 0, err
		}

		return int(input.MachinePool.Status.ReadyReplicas), nil
	}, intervals...).Should(Equal(int(*input.MachinePool.Spec.Replicas)))
}

// DiscoveryAndWaitForMachinePoolsInput is the input type for DiscoveryAndWaitForMachinePools.
type DiscoveryAndWaitForMachinePoolsInput struct {
	Getter  Getter
	Lister  Lister
	Cluster *clusterv1.Cluster
}

// DiscoveryAndWaitForMachinePools discovers the MachinePools existing in a cluster and waits for them to be ready (all the machines provisioned).
func DiscoveryAndWaitForMachinePools(ctx context.Context, input DiscoveryAndWaitForMachinePoolsInput, intervals ...interface{}) []*expv1.MachinePool {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DiscoveryAndWaitForMachinePools")
	Expect(input.Lister).ToNot(BeNil(), "Invalid argument. input.Lister can't be nil when calling DiscoveryAndWaitForMachinePools")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling DiscoveryAndWaitForMachinePools")

	machinePools := GetMachinePoolsByCluster(ctx, GetMachinePoolsByClusterInput{
		Lister:      input.Lister,
		ClusterName: input.Cluster.Name,
		Namespace:   input.Cluster.Namespace,
	})
	for _, machinepool := range machinePools {
		WaitForMachinePoolNodesToExist(ctx, WaitForMachinePoolNodesToExistInput{
			Getter:      input.Getter,
			MachinePool: machinepool,
		}, intervals...)
	}
	return machinePools
}

type UpgradeMachinePoolAndWaitInput struct {
	ClusterProxy                   ClusterProxy
	Cluster                        *clusterv1.Cluster
	UpgradeVersion                 string
	MachinePools                   []*expv1.MachinePool
	WaitForMachinePoolToBeUpgraded []interface{}
}

// UpgradeMachinePoolAndWait upgrades a machine pool and waits for its instances to be upgraded.
func UpgradeMachinePoolAndWait(ctx context.Context, input UpgradeMachinePoolAndWaitInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for UpgradeMachinePoolAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling UpgradeMachinePoolAndWait")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling UpgradeMachinePoolAndWait")
	Expect(input.UpgradeVersion).ToNot(BeNil(), "Invalid argument. input.UpgradeVersion can't be nil when calling UpgradeMachinePoolAndWait")
	Expect(input.MachinePools).ToNot(BeNil(), "Invalid argument. input.MachinePools can't be empty when calling UpgradeMachinePoolAndWait")

	mgmtClient := input.ClusterProxy.GetClient()
	for i := range input.MachinePools {
		mp := input.MachinePools[i]
		log.Logf("Patching the new Kubernetes version to Machine Pool %s/%s", mp.Namespace, mp.Name)
		patchHelper, err := patch.NewHelper(mp, mgmtClient)
		Expect(err).ToNot(HaveOccurred())

		oldVersion := mp.Spec.Template.Spec.Version
		mp.Spec.Template.Spec.Version = &input.UpgradeVersion
		Expect(patchHelper.Patch(ctx, mp)).To(Succeed())

		log.Logf("Waiting for Kubernetes versions of machines in MachinePool %s/%s to be upgraded from %s to %s",
			mp.Namespace, mp.Name, *oldVersion, input.UpgradeVersion)
		WaitForMachinePoolInstancesToBeUpgraded(ctx, WaitForMachinePoolInstancesToBeUpgradedInput{
			Getter:                   mgmtClient,
			WorkloadClusterGetter:    input.ClusterProxy.GetWorkloadCluster(ctx, input.Cluster.Namespace, input.Cluster.Name).GetClient(),
			Cluster:                  input.Cluster,
			MachineCount:             int(*mp.Spec.Replicas),
			KubernetesUpgradeVersion: input.UpgradeVersion,
			MachinePool:              mp,
		}, input.WaitForMachinePoolToBeUpgraded...)
	}
}

type ScaleMachinePoolAndWaitInput struct {
	ClusterProxy              ClusterProxy
	Cluster                   *clusterv1.Cluster
	Replicas                  int32
	MachinePools              []*expv1.MachinePool
	WaitForMachinePoolToScale []interface{}
}

// ScaleMachinePoolAndWait scales a machine pool and waits for its instances to scale up.
func ScaleMachinePoolAndWait(ctx context.Context, input ScaleMachinePoolAndWaitInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for UpgradeMachinePoolAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling UpgradeMachinePoolAndWait")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling UpgradeMachinePoolAndWait")
	Expect(input.MachinePools).ToNot(BeNil(), "Invalid argument. input.MachinePools can't be empty when calling UpgradeMachinePoolAndWait")

	mgmtClient := input.ClusterProxy.GetClient()
	for _, mp := range input.MachinePools {
		log.Logf("Patching the replica count in Machine Pool %s/%s", mp.Namespace, mp.Name)
		patchHelper, err := patch.NewHelper(mp, mgmtClient)
		Expect(err).ToNot(HaveOccurred())

		mp.Spec.Replicas = &input.Replicas
		Expect(patchHelper.Patch(ctx, mp)).To(Succeed())
	}

	for _, mp := range input.MachinePools {
		WaitForMachinePoolNodesToExist(ctx, WaitForMachinePoolNodesToExistInput{
			Getter:      mgmtClient,
			MachinePool: mp,
		}, input.WaitForMachinePoolToScale...)
	}
}

// WaitForMachinePoolInstancesToBeUpgradedInput is the input for WaitForMachinePoolInstancesToBeUpgraded.
type WaitForMachinePoolInstancesToBeUpgradedInput struct {
	Getter                   Getter
	WorkloadClusterGetter    Getter
	Cluster                  *clusterv1.Cluster
	KubernetesUpgradeVersion string
	MachineCount             int
	MachinePool              *expv1.MachinePool
}

// WaitForMachinePoolInstancesToBeUpgraded waits until all instances belonging to a MachinePool are upgraded to the correct kubernetes version.
func WaitForMachinePoolInstancesToBeUpgraded(ctx context.Context, input WaitForMachinePoolInstancesToBeUpgradedInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for WaitForMachinePoolInstancesToBeUpgraded")
	Expect(input.Getter).ToNot(BeNil(), "Invalid argument. input.Getter can't be nil when calling WaitForMachinePoolInstancesToBeUpgraded")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling WaitForMachinePoolInstancesToBeUpgraded")
	Expect(input.KubernetesUpgradeVersion).ToNot(BeNil(), "Invalid argument. input.KubernetesUpgradeVersion can't be nil when calling WaitForMachinePoolInstancesToBeUpgraded")
	Expect(input.MachinePool).ToNot(BeNil(), "Invalid argument. input.MachinePool can't be nil when calling WaitForMachinePoolInstancesToBeUpgraded")
	Expect(input.MachineCount).To(BeNumerically(">", 0), "Invalid argument. input.MachineCount can't be smaller than 1 when calling WaitForMachinePoolInstancesToBeUpgraded")

	log.Logf("Ensuring all MachinePool Instances have upgraded kubernetes version %s", input.KubernetesUpgradeVersion)
	Eventually(func() (int, error) {
		nn := client.ObjectKey{
			Namespace: input.MachinePool.Namespace,
			Name:      input.MachinePool.Name,
		}
		if err := input.Getter.Get(ctx, nn, input.MachinePool); err != nil {
			return 0, err
		}
		versions := getMachinePoolInstanceVersions(ctx, GetMachinesPoolInstancesInput{
			WorkloadClusterGetter: input.WorkloadClusterGetter,
			Namespace:             input.Cluster.Namespace,
			MachinePool:           input.MachinePool,
		})

		matches := 0
		for _, version := range versions {
			if version == input.KubernetesUpgradeVersion {
				matches++
			}
		}

		if matches != len(versions) {
			return 0, errors.New("old version instances remain")
		}

		return matches, nil
	}, intervals...).Should(Equal(input.MachineCount))
}

// GetMachinesPoolInstancesInput is the input for GetMachinesPoolInstances.
type GetMachinesPoolInstancesInput struct {
	WorkloadClusterGetter Getter
	Namespace             string
	MachinePool           *expv1.MachinePool
}

// getMachinePoolInstanceVersions returns the Kubernetes versions of the machine pool instances.
func getMachinePoolInstanceVersions(ctx context.Context, input GetMachinesPoolInstancesInput) []string {
	Expect(ctx).NotTo(BeNil(), "ctx is required for getMachinePoolInstanceVersions")
	Expect(input.WorkloadClusterGetter).ToNot(BeNil(), "Invalid argument. input.WorkloadClusterGetter can't be nil when calling getMachinePoolInstanceVersions")
	Expect(input.Namespace).ToNot(BeEmpty(), "Invalid argument. input.Namespace can't be empty when calling getMachinePoolInstanceVersions")
	Expect(input.MachinePool).ToNot(BeNil(), "Invalid argument. input.MachinePool can't be nil when calling getMachinePoolInstanceVersions")

	instances := input.MachinePool.Status.NodeRefs
	versions := make([]string, len(instances))
	for i, instance := range instances {
		node := &corev1.Node{}
		err := input.WorkloadClusterGetter.Get(ctx, client.ObjectKey{Name: instance.Name}, node)
		if err != nil {
			versions[i] = "unknown"
		} else {
			versions[i] = node.Status.NodeInfo.KubeletVersion
		}
	}

	return versions
}
