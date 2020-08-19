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

package clusterctl

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"

	. "github.com/onsi/gomega"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	clusterv1exp "sigs.k8s.io/cluster-api/exp/api/v1alpha3"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/internal/log"
)

// InitManagementClusterAndWatchControllerLogsInput is the input type for InitManagementClusterAndWatchControllerLogs.
type InitManagementClusterAndWatchControllerLogsInput struct {
	ClusterProxy             framework.ClusterProxy
	ClusterctlConfigPath     string
	InfrastructureProviders  []string
	LogFolder                string
	DisableMetricsCollection bool
}

// InitManagementClusterAndWatchControllerLogs initializes a management using clusterctl and setup watches for controller logs.
// Important: Considering we want to support test suites using existing clusters, clusterctl init is executed only in case
// there are no provider controllers in the cluster; but controller logs watchers are created regardless of the pre-existing providers.
func InitManagementClusterAndWatchControllerLogs(ctx context.Context, input InitManagementClusterAndWatchControllerLogsInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for InitManagementClusterAndWatchControllerLogs")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling InitManagementClusterAndWatchControllerLogs")
	Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling InitManagementClusterAndWatchControllerLogs")
	Expect(input.InfrastructureProviders).ToNot(BeEmpty(), "Invalid argument. input.InfrastructureProviders can't be empty when calling InitManagementClusterAndWatchControllerLogs")
	Expect(os.MkdirAll(input.LogFolder, 0755)).To(Succeed(), "Invalid argument. input.LogFolder can't be created for InitManagementClusterAndWatchControllerLogs")

	client := input.ClusterProxy.GetClient()
	controllersDeployments := framework.GetControllerDeployments(context.TODO(), framework.GetControllerDeploymentsInput{
		Lister: client,
	})
	if len(controllersDeployments) == 0 {
		Init(context.TODO(), InitInput{
			// pass reference to the management cluster hosting this test
			KubeconfigPath: input.ClusterProxy.GetKubeconfigPath(),
			// pass the clusterctl config file that points to the local provider repository created for this test
			ClusterctlConfigPath: input.ClusterctlConfigPath,
			// setup the desired list of providers for a single-tenant management cluster
			CoreProvider:            config.ClusterAPIProviderName,
			BootstrapProviders:      []string{config.KubeadmBootstrapProviderName},
			ControlPlaneProviders:   []string{config.KubeadmControlPlaneProviderName},
			InfrastructureProviders: input.InfrastructureProviders,
			// setup clusterctl logs folder
			LogFolder: input.LogFolder,
		})
	}

	log.Logf("Waiting for provider controllers to be running")
	controllersDeployments = framework.GetControllerDeployments(context.TODO(), framework.GetControllerDeploymentsInput{
		Lister: client,
	})
	Expect(controllersDeployments).ToNot(BeEmpty(), "The list of controller deployments should not be empty")
	for _, deployment := range controllersDeployments {
		framework.WaitForDeploymentsAvailable(context.TODO(), framework.WaitForDeploymentsAvailableInput{
			Getter:     client,
			Deployment: deployment,
		}, intervals...)

		// Start streaming logs from all controller providers
		framework.WatchDeploymentLogs(context.TODO(), framework.WatchDeploymentLogsInput{
			GetLister:  client,
			ClientSet:  input.ClusterProxy.GetClientSet(),
			Deployment: deployment,
			LogPath:    filepath.Join(input.LogFolder, "controllers"),
		})

		if input.DisableMetricsCollection {
			return
		}
		framework.WatchPodMetrics(ctx, framework.WatchPodMetricsInput{
			GetLister:   client,
			ClientSet:   input.ClusterProxy.GetClientSet(),
			Deployment:  deployment,
			MetricsPath: filepath.Join(input.LogFolder, "controllers"),
		})
	}
}

// ApplyClusterTemplateAndWaitInput is the input type for ApplyClusterTemplateAndWait.
type ApplyClusterTemplateAndWaitInput struct {
	ClusterProxy                 framework.ClusterProxy
	ConfigCluster                ConfigClusterInput
	CNIManifestPath              string
	WaitForClusterIntervals      []interface{}
	WaitForControlPlaneIntervals []interface{}
	WaitForMachineDeployments    []interface{}
	WaitForMachinePools          []interface{}
}

type ApplyClusterTemplateAndWaitResult struct {
	Cluster            *clusterv1.Cluster
	ControlPlane       *controlplanev1.KubeadmControlPlane
	MachineDeployments []*clusterv1.MachineDeployment
	MachinePools       []*clusterv1exp.MachinePool
}

// ApplyClusterTemplateAndWait gets a cluster template using clusterctl, and waits for the cluster to be ready.
// Important! this method assumes the cluster uses a KubeadmControlPlane and MachineDeployments.
func ApplyClusterTemplateAndWait(ctx context.Context, input ApplyClusterTemplateAndWaitInput) *ApplyClusterTemplateAndWaitResult {
	Expect(ctx).NotTo(BeNil(), "ctx is required for ApplyClusterTemplateAndWait")

	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling ApplyClusterTemplateAndWait")

	log.Logf("Creating the workload cluster with name %q using the %q template (Kubernetes %s, %d control-plane machines, %d worker machines)",
		input.ConfigCluster.ClusterName, valueOrDefault(input.ConfigCluster.Flavor), input.ConfigCluster.KubernetesVersion, input.ConfigCluster.ControlPlaneMachineCount, input.ConfigCluster.WorkerMachineCount)

	log.Logf("Getting the cluster template yaml")
	workloadClusterTemplate := ConfigCluster(ctx, ConfigClusterInput{
		// pass reference to the management cluster hosting this test
		KubeconfigPath: input.ConfigCluster.KubeconfigPath,
		// pass the clusterctl config file that points to the local provider repository created for this test,
		ClusterctlConfigPath: input.ConfigCluster.ClusterctlConfigPath,
		// select template
		Flavor: input.ConfigCluster.Flavor,
		// define template variables
		Namespace:                input.ConfigCluster.Namespace,
		ClusterName:              input.ConfigCluster.ClusterName,
		KubernetesVersion:        input.ConfigCluster.KubernetesVersion,
		ControlPlaneMachineCount: input.ConfigCluster.ControlPlaneMachineCount,
		WorkerMachineCount:       input.ConfigCluster.WorkerMachineCount,
		InfrastructureProvider:   input.ConfigCluster.InfrastructureProvider,
		// setup clusterctl logs folder
		LogFolder: input.ConfigCluster.LogFolder,
	})
	Expect(workloadClusterTemplate).ToNot(BeNil(), "Failed to get the cluster template")

	log.Logf("Applying the cluster template yaml to the cluster")
	Expect(input.ClusterProxy.Apply(ctx, workloadClusterTemplate)).ShouldNot(HaveOccurred())

	log.Logf("Waiting for the cluster infrastructure to be provisioned")
	cluster := framework.DiscoveryAndWaitForCluster(ctx, framework.DiscoveryAndWaitForClusterInput{
		Getter:    input.ClusterProxy.GetClient(),
		Namespace: input.ConfigCluster.Namespace,
		Name:      input.ConfigCluster.ClusterName,
	}, input.WaitForClusterIntervals...)

	log.Logf("Waiting for control plane to be initialized")
	controlPlane := framework.DiscoveryAndWaitForControlPlaneInitialized(ctx, framework.DiscoveryAndWaitForControlPlaneInitializedInput{
		Lister:  input.ClusterProxy.GetClient(),
		Cluster: cluster,
	}, input.WaitForControlPlaneIntervals...)

	if input.CNIManifestPath != "" {
		log.Logf("Installing a CNI plugin to the workload cluster")
		workloadCluster := input.ClusterProxy.GetWorkloadCluster(context.TODO(), cluster.Namespace, cluster.Name)

		cniYaml, err := ioutil.ReadFile(input.CNIManifestPath)
		Expect(err).ShouldNot(HaveOccurred())

		Expect(workloadCluster.Apply(context.TODO(), cniYaml)).ShouldNot(HaveOccurred())
	}

	log.Logf("Waiting for control plane to be ready")
	framework.WaitForControlPlaneAndMachinesReady(ctx, framework.WaitForControlPlaneAndMachinesReadyInput{
		GetLister:    input.ClusterProxy.GetClient(),
		Cluster:      cluster,
		ControlPlane: controlPlane,
	}, input.WaitForControlPlaneIntervals...)

	log.Logf("Waiting for the machine deployments to be provisioned")
	machineDeployments := framework.DiscoveryAndWaitForMachineDeployments(ctx, framework.DiscoveryAndWaitForMachineDeploymentsInput{
		Lister:  input.ClusterProxy.GetClient(),
		Cluster: cluster,
	}, input.WaitForMachineDeployments...)

	log.Logf("Waiting for the machine pools to be provisioned")
	machinePools := framework.DiscoveryAndWaitForMachinePools(ctx, framework.DiscoveryAndWaitForMachinePoolsInput{
		Getter:  input.ClusterProxy.GetClient(),
		Lister:  input.ClusterProxy.GetClient(),
		Cluster: cluster,
	}, input.WaitForMachineDeployments...)

	return &ApplyClusterTemplateAndWaitResult{
		Cluster:            cluster,
		ControlPlane:       controlPlane,
		MachineDeployments: machineDeployments,
		MachinePools:       machinePools,
	}
}
