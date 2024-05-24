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
	"strconv"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework/internal/log"
	"sigs.k8s.io/cluster-api/util/patch"
)

// GetClusterClassByNameInput is the input for GetClusterClassByName.
type GetClusterClassByNameInput struct {
	Getter    Getter
	Name      string
	Namespace string
}

// GetClusterClassByName returns a ClusterClass object given his name and namespace.
func GetClusterClassByName(ctx context.Context, input GetClusterClassByNameInput) *clusterv1.ClusterClass {
	Expect(ctx).NotTo(BeNil(), "ctx is required for GetClusterClassByName")
	Expect(input.Getter).ToNot(BeNil(), "Invalid argument. input.Getter can't be nil when calling GetClusterClassByName")
	Expect(input.Namespace).ToNot(BeNil(), "Invalid argument. input.Namespace can't be empty when calling GetClusterClassByName")
	Expect(input.Name).ToNot(BeNil(), "Invalid argument. input.Name can't be empty when calling GetClusterClassByName")

	clusterClass := &clusterv1.ClusterClass{}
	key := client.ObjectKey{
		Namespace: input.Namespace,
		Name:      input.Name,
	}
	Eventually(func() error {
		return input.Getter.Get(ctx, key, clusterClass)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to get ClusterClass object %s", klog.KRef(input.Namespace, input.Name))
	return clusterClass
}

// UpgradeClusterTopologyAndWaitForUpgradeInput is the input type for UpgradeClusterTopologyAndWaitForUpgrade.
type UpgradeClusterTopologyAndWaitForUpgradeInput struct {
	ClusterProxy                       ClusterProxy
	Cluster                            *clusterv1.Cluster
	ControlPlane                       *controlplanev1.KubeadmControlPlane
	EtcdImageTag                       string
	DNSImageTag                        string
	MachineDeployments                 []*clusterv1.MachineDeployment
	MachinePools                       []*expv1.MachinePool
	KubernetesUpgradeVersion           string
	WaitForMachinesToBeUpgraded        []interface{}
	WaitForMachinePoolToBeUpgraded     []interface{}
	WaitForKubeProxyUpgrade            []interface{}
	WaitForDNSUpgrade                  []interface{}
	WaitForEtcdUpgrade                 []interface{}
	PreWaitForControlPlaneToBeUpgraded func()
	PreWaitForWorkersToBeUpgraded      func()
	SkipKubeProxyCheck                 bool
}

// UpgradeClusterTopologyAndWaitForUpgrade upgrades a Cluster topology and waits for it to be upgraded.
// NOTE: This func only works with KubeadmControlPlane.
func UpgradeClusterTopologyAndWaitForUpgrade(ctx context.Context, input UpgradeClusterTopologyAndWaitForUpgradeInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for UpgradeClusterTopologyAndWaitForUpgrade")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling UpgradeClusterTopologyAndWaitForUpgrade")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling UpgradeClusterTopologyAndWaitForUpgrade")
	Expect(input.ControlPlane).ToNot(BeNil(), "Invalid argument. input.ControlPlane can't be nil when calling UpgradeClusterTopologyAndWaitForUpgrade")
	Expect(input.KubernetesUpgradeVersion).ToNot(BeNil(), "Invalid argument. input.KubernetesUpgradeVersion can't be empty when calling UpgradeClusterTopologyAndWaitForUpgrade")

	mgmtClient := input.ClusterProxy.GetClient()

	log.Logf("Patching the new Kubernetes version to Cluster topology")
	patchHelper, err := patch.NewHelper(input.Cluster, mgmtClient)
	Expect(err).ToNot(HaveOccurred())

	input.Cluster.Spec.Topology.Version = input.KubernetesUpgradeVersion
	for i, variable := range input.Cluster.Spec.Topology.Variables {
		if input.EtcdImageTag != "" && variable.Name == "etcdImageTag" {
			// NOTE: strconv.Quote is used to produce a valid JSON string.
			input.Cluster.Spec.Topology.Variables[i].Value = apiextensionsv1.JSON{Raw: []byte(strconv.Quote(input.EtcdImageTag))}
		}
		if input.DNSImageTag != "" && variable.Name == "coreDNSImageTag" {
			// NOTE: strconv.Quote is used to produce a valid JSON string.
			input.Cluster.Spec.Topology.Variables[i].Value = apiextensionsv1.JSON{Raw: []byte(strconv.Quote(input.DNSImageTag))}
		}
	}
	Eventually(func() error {
		return patchHelper.Patch(ctx, input.Cluster)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to patch Cluster topology %s with version %s", klog.KObj(input.Cluster), input.KubernetesUpgradeVersion)

	// Once we have patched the Kubernetes Cluster we can run PreWaitForControlPlaneToBeUpgraded.
	// Note: This can e.g. be used to verify the BeforeClusterUpgrade lifecycle hook is executed
	// and blocking correctly.
	if input.PreWaitForControlPlaneToBeUpgraded != nil {
		log.Logf("Calling PreWaitForControlPlaneToBeUpgraded")
		input.PreWaitForControlPlaneToBeUpgraded()
	}

	log.Logf("Waiting for control-plane machines to have the upgraded Kubernetes version")
	WaitForControlPlaneMachinesToBeUpgraded(ctx, WaitForControlPlaneMachinesToBeUpgradedInput{
		Lister:                   mgmtClient,
		Cluster:                  input.Cluster,
		MachineCount:             int(*input.ControlPlane.Spec.Replicas),
		KubernetesUpgradeVersion: input.KubernetesUpgradeVersion,
	}, input.WaitForMachinesToBeUpgraded...)

	workloadCluster := input.ClusterProxy.GetWorkloadCluster(ctx, input.Cluster.Namespace, input.Cluster.Name)
	workloadClient := workloadCluster.GetClient()

	if !input.SkipKubeProxyCheck {
		log.Logf("Waiting for kube-proxy to have the upgraded Kubernetes version")
		WaitForKubeProxyUpgrade(ctx, WaitForKubeProxyUpgradeInput{
			Getter:            workloadClient,
			KubernetesVersion: input.KubernetesUpgradeVersion,
		}, input.WaitForKubeProxyUpgrade...)
	}

	// Wait for the CoreDNS upgrade if the DNSImageTag is set.
	if input.DNSImageTag != "" {
		log.Logf("Waiting for CoreDNS to have the upgraded image tag")
		WaitForDNSUpgrade(ctx, WaitForDNSUpgradeInput{
			Getter:     workloadClient,
			DNSVersion: input.DNSImageTag,
		}, input.WaitForDNSUpgrade...)
	}

	// Wait for the etcd upgrade if the EtcdImageTag is set.
	if input.EtcdImageTag != "" {
		log.Logf("Waiting for etcd to have the upgraded image tag")
		lblSelector, err := labels.Parse("component=etcd")
		Expect(err).ToNot(HaveOccurred())
		WaitForPodListCondition(ctx, WaitForPodListConditionInput{
			Lister:      workloadClient,
			ListOptions: &client.ListOptions{LabelSelector: lblSelector},
			Condition:   EtcdImageTagCondition(input.EtcdImageTag, int(*input.ControlPlane.Spec.Replicas)),
		}, input.WaitForEtcdUpgrade...)
	}

	// Once the ControlPlane is upgraded we can run PreWaitForWorkersToBeUpgraded.
	// Note: This can e.g. be used to verify the AfterControlPlaneUpgrade lifecycle hook is executed
	// and blocking correctly.
	if input.PreWaitForWorkersToBeUpgraded != nil {
		log.Logf("Calling PreWaitForWorkersToBeUpgraded")
		input.PreWaitForWorkersToBeUpgraded()
	}

	for _, deployment := range input.MachineDeployments {
		if *deployment.Spec.Replicas > 0 {
			log.Logf("Waiting for Kubernetes versions of machines in MachineDeployment %s to be upgraded to %s",
				klog.KObj(deployment), input.KubernetesUpgradeVersion)
			WaitForMachineDeploymentMachinesToBeUpgraded(ctx, WaitForMachineDeploymentMachinesToBeUpgradedInput{
				Lister:                   mgmtClient,
				Cluster:                  input.Cluster,
				MachineCount:             int(*deployment.Spec.Replicas),
				KubernetesUpgradeVersion: input.KubernetesUpgradeVersion,
				MachineDeployment:        *deployment,
			}, input.WaitForMachinesToBeUpgraded...)
		}
	}

	for _, pool := range input.MachinePools {
		if *pool.Spec.Replicas > 0 {
			log.Logf("Waiting for Kubernetes versions of machines in MachinePool %s to be upgraded to %s",
				klog.KObj(pool), input.KubernetesUpgradeVersion)
			WaitForMachinePoolInstancesToBeUpgraded(ctx, WaitForMachinePoolInstancesToBeUpgradedInput{
				Getter:                   mgmtClient,
				WorkloadClusterGetter:    input.ClusterProxy.GetWorkloadCluster(ctx, input.Cluster.Namespace, input.Cluster.Name).GetClient(),
				Cluster:                  input.Cluster,
				MachineCount:             int(*pool.Spec.Replicas),
				KubernetesUpgradeVersion: input.KubernetesUpgradeVersion,
				MachinePool:              pool,
			}, input.WaitForMachinePoolToBeUpgraded...)
		}
	}
}
