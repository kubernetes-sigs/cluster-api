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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
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
	Eventually(func() error {
		return input.Lister.List(ctx, mpList, byClusterOptions(input.ClusterName, input.Namespace)...)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to list MachinePools object for Cluster %s", klog.KRef(input.Namespace, input.ClusterName))

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
	}, intervals...).Should(Equal(int(*input.MachinePool.Spec.Replicas)), "Timed out waiting for %v ready replicas for MachinePool %s", *input.MachinePool.Spec.Replicas, klog.KObj(input.MachinePool))
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

		// TODO: check for failure domains; currently MP doesn't provide a way to check where Machine are placed
		//  (checking infrastructure is the only alternative, but this makes test not portable)
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
		log.Logf("Patching the new Kubernetes version to Machine Pool %s", klog.KObj(mp))
		patchHelper, err := patch.NewHelper(mp, mgmtClient)
		Expect(err).ToNot(HaveOccurred())

		// Store old version.
		oldVersion := mp.Spec.Template.Spec.Version

		// Upgrade to new Version.
		mp.Spec.Template.Spec.Version = &input.UpgradeVersion

		Eventually(func() error {
			return patchHelper.Patch(ctx, mp)
		}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to patch the new Kubernetes version to Machine Pool %s", klog.KObj(mp))

		log.Logf("Waiting for Kubernetes versions of machines in MachinePool %s to be upgraded from %s to %s",
			klog.KObj(mp), *oldVersion, input.UpgradeVersion)
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
		log.Logf("Patching the replica count in Machine Pool %s", klog.KObj(mp))
		patchHelper, err := patch.NewHelper(mp, mgmtClient)
		Expect(err).ToNot(HaveOccurred())

		mp.Spec.Replicas = &input.Replicas
		Eventually(func() error {
			return patchHelper.Patch(ctx, mp)
		}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to patch MachinePool %s", klog.KObj(mp))
	}

	for _, mp := range input.MachinePools {
		WaitForMachinePoolNodesToExist(ctx, WaitForMachinePoolNodesToExistInput{
			Getter:      mgmtClient,
			MachinePool: mp,
		}, input.WaitForMachinePoolToScale...)
	}
}

type ScaleMachinePoolTopologyAndWaitInput struct {
	ClusterProxy        ClusterProxy
	Cluster             *clusterv1.Cluster
	Replicas            int32
	WaitForMachinePools []interface{}
	Getter              Getter
}

// ScaleMachinePoolTopologyAndWait scales a machine pool and waits for its instances to scale up.
func ScaleMachinePoolTopologyAndWait(ctx context.Context, input ScaleMachinePoolTopologyAndWaitInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for ScaleMachinePoolTopologyAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling ScaleMachinePoolTopologyAndWait")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling ScaleMachinePoolTopologyAndWait")
	Expect(input.Cluster.Spec.Topology.Workers).ToNot(BeNil(), "Invalid argument. input.Cluster must have MachinePool topologies")
	Expect(input.Cluster.Spec.Topology.Workers.MachinePools).NotTo(BeEmpty(), "Invalid argument. input.Cluster must have at least one MachinePool topology")

	mpTopology := input.Cluster.Spec.Topology.Workers.MachinePools[0]
	if mpTopology.Replicas != nil {
		log.Logf("Scaling machine pool topology %s from %d to %d replicas", mpTopology.Name, *mpTopology.Replicas, input.Replicas)
	} else {
		log.Logf("Scaling machine pool topology %s to %d replicas", mpTopology.Name, input.Replicas)
	}
	patchHelper, err := patch.NewHelper(input.Cluster, input.ClusterProxy.GetClient())
	Expect(err).ToNot(HaveOccurred())
	mpTopology.Replicas = ptr.To[int32](input.Replicas)
	input.Cluster.Spec.Topology.Workers.MachinePools[0] = mpTopology
	Eventually(func() error {
		return patchHelper.Patch(ctx, input.Cluster)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to scale machine pool topology %s", mpTopology.Name)

	log.Logf("Waiting for correct number of replicas to exist and have correct number for .spec.replicas")
	mpList := &expv1.MachinePoolList{}
	mp := expv1.MachinePool{}
	Eventually(func(g Gomega) int32 {
		g.Expect(input.ClusterProxy.GetClient().List(ctx, mpList,
			client.InNamespace(input.Cluster.Namespace),
			client.MatchingLabels{
				clusterv1.ClusterNameLabel:                    input.Cluster.Name,
				clusterv1.ClusterTopologyMachinePoolNameLabel: mpTopology.Name,
			},
		)).ToNot(HaveOccurred())
		g.Expect(mpList.Items).To(HaveLen(1))
		mp = mpList.Items[0]
		return *mp.Spec.Replicas
	}, retryableOperationTimeout, retryableOperationInterval).Should(Equal(input.Replicas), "MachinePool replicas for Cluster %s does not match set topology replicas", klog.KRef(input.Cluster.Namespace, input.Cluster.Name))

	WaitForMachinePoolNodesToExist(ctx, WaitForMachinePoolNodesToExistInput{
		Getter:      input.Getter,
		MachinePool: &mp,
	}, input.WaitForMachinePools...)
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
		mpKey := client.ObjectKey{
			Namespace: input.MachinePool.Namespace,
			Name:      input.MachinePool.Name,
		}
		if err := input.Getter.Get(ctx, mpKey, input.MachinePool); err != nil {
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
			return 0, errors.Errorf("old version instances remain. Expected %d instances at version %v. Got version list: %v", len(versions), input.KubernetesUpgradeVersion, versions)
		}

		return matches, nil
	}, intervals...).Should(Equal(input.MachineCount), "Timed out waiting for all MachinePool %s instances to be upgraded to Kubernetes version %s", klog.KObj(input.MachinePool), input.KubernetesUpgradeVersion)
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
		var nodeGetError error
		err := wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 10*time.Second, true, func(ctx context.Context) (bool, error) {
			nodeGetError = input.WorkloadClusterGetter.Get(ctx, client.ObjectKey{Name: instance.Name}, node)
			if nodeGetError != nil {
				return false, nil //nolint:nilerr
			}
			return true, nil
		})
		if err != nil {
			versions[i] = "unknown"
			if nodeGetError != nil {
				// Dump the instance name and error here so that we can log it as part of the version array later on.
				versions[i] = fmt.Sprintf("%s error: %s", instance.Name, errors.Wrap(err, nodeGetError.Error()))
			}
		} else {
			versions[i] = node.Status.NodeInfo.KubeletVersion
		}
	}

	return versions
}

type AssertMachinePoolReplicasInput struct {
	Getter             Getter
	MachinePool        *expv1.MachinePool
	Replicas           int32
	WaitForMachinePool []interface{}
}

func AssertMachinePoolReplicas(ctx context.Context, input AssertMachinePoolReplicasInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for AssertMachinePoolReplicas")
	Expect(input.Getter).ToNot(BeNil(), "Invalid argument. input.Getter can't be nil when calling AssertMachinePoolReplicas")
	Expect(input.MachinePool).ToNot(BeNil(), "Invalid argument. input.MachinePool can't be nil when calling AssertMachinePoolReplicas")

	Eventually(func(g Gomega) {
		// Get the MachinePool
		mp := &expv1.MachinePool{}
		key := client.ObjectKey{
			Namespace: input.MachinePool.Namespace,
			Name:      input.MachinePool.Name,
		}
		g.Expect(input.Getter.Get(ctx, key, mp)).To(Succeed(), fmt.Sprintf("failed to get MachinePool %s", klog.KObj(input.MachinePool)))
		g.Expect(mp.Spec.Replicas).Should(Not(BeNil()), fmt.Sprintf("MachinePool %s replicas should not be nil", klog.KObj(mp)))
		g.Expect(*mp.Spec.Replicas).Should(Equal(input.Replicas), fmt.Sprintf("MachinePool %s replicas should match expected replicas", klog.KObj(mp)))
		g.Expect(mp.Status.Replicas).Should(Equal(input.Replicas), fmt.Sprintf("MachinePool %s status.replicas should match expected replicas", klog.KObj(mp)))
	}, input.WaitForMachinePool...).Should(Succeed())
}
