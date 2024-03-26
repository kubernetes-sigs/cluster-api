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
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/blang/semver/v4"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/internal/log"
)

// InitManagementClusterAndWatchControllerLogsInput is the input type for InitManagementClusterAndWatchControllerLogs.
type InitManagementClusterAndWatchControllerLogsInput struct {
	ClusterProxy              framework.ClusterProxy
	ClusterctlConfigPath      string
	CoreProvider              string
	BootstrapProviders        []string
	ControlPlaneProviders     []string
	InfrastructureProviders   []string
	IPAMProviders             []string
	RuntimeExtensionProviders []string
	AddonProviders            []string
	LogFolder                 string
	DisableMetricsCollection  bool
	ClusterctlBinaryPath      string
}

// InitManagementClusterAndWatchControllerLogs initializes a management using clusterctl and setup watches for controller logs.
// Important: Considering we want to support test suites using existing clusters, clusterctl init is executed only in case
// there are no provider controllers in the cluster; but controller logs watchers are created regardless of the pre-existing providers.
func InitManagementClusterAndWatchControllerLogs(ctx context.Context, input InitManagementClusterAndWatchControllerLogsInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for InitManagementClusterAndWatchControllerLogs")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling InitManagementClusterAndWatchControllerLogs")
	Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling InitManagementClusterAndWatchControllerLogs")
	Expect(input.InfrastructureProviders).ToNot(BeEmpty(), "Invalid argument. input.InfrastructureProviders can't be empty when calling InitManagementClusterAndWatchControllerLogs")
	Expect(os.MkdirAll(input.LogFolder, 0750)).To(Succeed(), "Invalid argument. input.LogFolder can't be created for InitManagementClusterAndWatchControllerLogs")

	if input.CoreProvider == "" {
		input.CoreProvider = config.ClusterAPIProviderName
	}
	if len(input.BootstrapProviders) == 0 {
		input.BootstrapProviders = []string{config.KubeadmBootstrapProviderName}
	}
	if len(input.ControlPlaneProviders) == 0 {
		input.ControlPlaneProviders = []string{config.KubeadmControlPlaneProviderName}
	}

	client := input.ClusterProxy.GetClient()
	controllersDeployments := framework.GetControllerDeployments(ctx, framework.GetControllerDeploymentsInput{
		Lister: client,
	})
	if len(controllersDeployments) == 0 {
		initInput := InitInput{
			// pass reference to the management cluster hosting this test
			KubeconfigPath: input.ClusterProxy.GetKubeconfigPath(),
			// pass the clusterctl config file that points to the local provider repository created for this test
			ClusterctlConfigPath: input.ClusterctlConfigPath,
			// setup the desired list of providers for a single-tenant management cluster
			CoreProvider:              input.CoreProvider,
			BootstrapProviders:        input.BootstrapProviders,
			ControlPlaneProviders:     input.ControlPlaneProviders,
			InfrastructureProviders:   input.InfrastructureProviders,
			IPAMProviders:             input.IPAMProviders,
			RuntimeExtensionProviders: input.RuntimeExtensionProviders,
			AddonProviders:            input.AddonProviders,
			// setup clusterctl logs folder
			LogFolder: input.LogFolder,
		}

		if input.ClusterctlBinaryPath != "" {
			InitWithBinary(ctx, input.ClusterctlBinaryPath, initInput)
			// Old versions of clusterctl may deploy CRDs, Mutating- and/or ValidatingWebhookConfigurations
			// before creating the new Certificate objects. This check ensures the CA's are up to date before
			// continuing.
			clusterctlVersion, err := getClusterCtlVersion(input.ClusterctlBinaryPath)
			Expect(err).ToNot(HaveOccurred())
			if clusterctlVersion.LT(semver.MustParse("1.7.2")) {
				Eventually(func() error {
					return verifyCAInjection(ctx, client)
				}, time.Minute*5, time.Second*10).Should(Succeed(), "Failed to verify CA injection")
			}
		} else {
			Init(ctx, initInput)
		}
	}

	log.Logf("Waiting for provider controllers to be running")
	controllersDeployments = framework.GetControllerDeployments(ctx, framework.GetControllerDeploymentsInput{
		Lister: client,
	})
	Expect(controllersDeployments).ToNot(BeEmpty(), "The list of controller deployments should not be empty")
	for _, deployment := range controllersDeployments {
		framework.WaitForDeploymentsAvailable(ctx, framework.WaitForDeploymentsAvailableInput{
			Getter:     client,
			Deployment: deployment,
		}, intervals...)

		// Start streaming logs from all controller providers
		framework.WatchDeploymentLogsByName(ctx, framework.WatchDeploymentLogsByNameInput{
			GetLister:  client,
			Cache:      input.ClusterProxy.GetCache(ctx),
			ClientSet:  input.ClusterProxy.GetClientSet(),
			Deployment: deployment,
			LogPath:    filepath.Join(input.LogFolder, "logs", deployment.GetNamespace()),
		})

		if !input.DisableMetricsCollection {
			framework.WatchPodMetrics(ctx, framework.WatchPodMetricsInput{
				GetLister:   client,
				ClientSet:   input.ClusterProxy.GetClientSet(),
				Deployment:  deployment,
				MetricsPath: filepath.Join(input.LogFolder, "metrics", deployment.GetNamespace()),
			})
		}
	}
}

// UpgradeManagementClusterAndWaitInput is the input type for UpgradeManagementClusterAndWait.
type UpgradeManagementClusterAndWaitInput struct {
	ClusterProxy              framework.ClusterProxy
	ClusterctlConfigPath      string
	ClusterctlVariables       map[string]string
	Contract                  string
	CoreProvider              string
	BootstrapProviders        []string
	ControlPlaneProviders     []string
	InfrastructureProviders   []string
	IPAMProviders             []string
	RuntimeExtensionProviders []string
	AddonProviders            []string
	LogFolder                 string
	ClusterctlBinaryPath      string
}

// UpgradeManagementClusterAndWait upgrades provider a management cluster using clusterctl, and waits for the cluster to be ready.
func UpgradeManagementClusterAndWait(ctx context.Context, input UpgradeManagementClusterAndWaitInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for UpgradeManagementClusterAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling UpgradeManagementClusterAndWait")
	Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling UpgradeManagementClusterAndWait")
	// Check if the user want a custom upgrade
	isCustomUpgrade := input.CoreProvider != "" ||
		len(input.BootstrapProviders) > 0 ||
		len(input.ControlPlaneProviders) > 0 ||
		len(input.InfrastructureProviders) > 0 ||
		len(input.IPAMProviders) > 0 ||
		len(input.RuntimeExtensionProviders) > 0 ||
		len(input.AddonProviders) > 0

	Expect((input.Contract != "" && !isCustomUpgrade) || (input.Contract == "" && isCustomUpgrade)).To(BeTrue(), `Invalid argument. Either the input.Contract parameter or at least one of the following providers has to be set:
		input.CoreProvider, input.BootstrapProviders, input.ControlPlaneProviders, input.InfrastructureProviders, input.IPAMProviders, input.RuntimeExtensionProviders, input.AddonProviders`)

	Expect(os.MkdirAll(input.LogFolder, 0750)).To(Succeed(), "Invalid argument. input.LogFolder can't be created for UpgradeManagementClusterAndWait")

	upgradeInput := UpgradeInput{
		ClusterctlConfigPath:      input.ClusterctlConfigPath,
		ClusterctlVariables:       input.ClusterctlVariables,
		ClusterName:               input.ClusterProxy.GetName(),
		KubeconfigPath:            input.ClusterProxy.GetKubeconfigPath(),
		Contract:                  input.Contract,
		CoreProvider:              input.CoreProvider,
		BootstrapProviders:        input.BootstrapProviders,
		ControlPlaneProviders:     input.ControlPlaneProviders,
		InfrastructureProviders:   input.InfrastructureProviders,
		IPAMProviders:             input.IPAMProviders,
		RuntimeExtensionProviders: input.RuntimeExtensionProviders,
		AddonProviders:            input.AddonProviders,
		LogFolder:                 input.LogFolder,
	}

	client := input.ClusterProxy.GetClient()

	if input.ClusterctlBinaryPath != "" {
		UpgradeWithBinary(ctx, input.ClusterctlBinaryPath, upgradeInput)
		// Old versions of clusterctl may deploy CRDs, Mutating- and/or ValidatingWebhookConfigurations
		// before creating the new Certificate objects. This check ensures the CA's are up to date before
		// continuing.
		clusterctlVersion, err := getClusterCtlVersion(input.ClusterctlBinaryPath)
		Expect(err).ToNot(HaveOccurred())
		if clusterctlVersion.LT(semver.MustParse("1.7.2")) {
			Eventually(func() error {
				return verifyCAInjection(ctx, client)
			}, time.Minute*5, time.Second*10).Should(Succeed(), "Failed to verify CA injection")
		}
	} else {
		Upgrade(ctx, upgradeInput)
	}

	log.Logf("Waiting for provider controllers to be running")
	controllersDeployments := framework.GetControllerDeployments(ctx, framework.GetControllerDeploymentsInput{
		Lister: client,
		// This namespace has been dropped in v0.4.x.
		// We have to exclude this namespace here as after an upgrade from v0.3x there won't
		// be a controller in this namespace anymore and if we wait for it to come up the test would fail.
		// Note: We can drop this as soon as we don't have a test upgrading from v0.3.x anymore.
		ExcludeNamespaces: []string{"capi-webhook-system"},
	})
	Expect(controllersDeployments).ToNot(BeEmpty(), "The list of controller deployments should not be empty")
	for _, deployment := range controllersDeployments {
		framework.WaitForDeploymentsAvailable(ctx, framework.WaitForDeploymentsAvailableInput{
			Getter:     client,
			Deployment: deployment,
		}, intervals...)

		framework.WatchPodMetrics(ctx, framework.WatchPodMetricsInput{
			GetLister:   client,
			ClientSet:   input.ClusterProxy.GetClientSet(),
			Deployment:  deployment,
			MetricsPath: filepath.Join(input.LogFolder, "metrics", deployment.GetNamespace()),
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
	CreateOrUpdateOpts           []framework.CreateOrUpdateOption // options to be passed to CreateOrUpdate function config
	PreWaitForCluster            func()
	PostMachinesProvisioned      func()
	ControlPlaneWaiters
}

// Waiter is a function that runs and waits for a long-running operation to finish and updates the result.
type Waiter func(ctx context.Context, input ApplyCustomClusterTemplateAndWaitInput, result *ApplyCustomClusterTemplateAndWaitResult)

// ControlPlaneWaiters are Waiter functions for the control plane.
type ControlPlaneWaiters struct {
	WaitForControlPlaneInitialized   Waiter
	WaitForControlPlaneMachinesReady Waiter
}

// ApplyClusterTemplateAndWaitResult is the output type for ApplyClusterTemplateAndWait.
type ApplyClusterTemplateAndWaitResult struct {
	ClusterClass       *clusterv1.ClusterClass
	Cluster            *clusterv1.Cluster
	ControlPlane       *controlplanev1.KubeadmControlPlane
	MachineDeployments []*clusterv1.MachineDeployment
	MachinePools       []*expv1.MachinePool
}

// ExpectedWorkerNodes returns the expected number of worker nodes that will
// be provisioned by the given cluster template.
func (r *ApplyClusterTemplateAndWaitResult) ExpectedWorkerNodes() int32 {
	expectedWorkerNodes := int32(0)

	for _, md := range r.MachineDeployments {
		if md.Spec.Replicas != nil {
			expectedWorkerNodes += *md.Spec.Replicas
		}
	}
	for _, mp := range r.MachinePools {
		if mp.Spec.Replicas != nil {
			expectedWorkerNodes += *mp.Spec.Replicas
		}
	}

	return expectedWorkerNodes
}

// ExpectedTotalNodes returns the expected number of nodes that will
// be provisioned by the given cluster template.
func (r *ApplyClusterTemplateAndWaitResult) ExpectedTotalNodes() int32 {
	expectedNodes := r.ExpectedWorkerNodes()

	if r.ControlPlane != nil && r.ControlPlane.Spec.Replicas != nil {
		expectedNodes += *r.ControlPlane.Spec.Replicas
	}

	return expectedNodes
}

// ApplyClusterTemplateAndWait gets a cluster template using clusterctl, and waits for the cluster to be ready.
// Important! this method assumes the cluster uses a KubeadmControlPlane and MachineDeployments.
func ApplyClusterTemplateAndWait(ctx context.Context, input ApplyClusterTemplateAndWaitInput, result *ApplyClusterTemplateAndWaitResult) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for ApplyClusterTemplateAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling ApplyClusterTemplateAndWait")
	Expect(result).ToNot(BeNil(), "Invalid argument. result can't be nil when calling ApplyClusterTemplateAndWait")
	Expect(input.ConfigCluster.ControlPlaneMachineCount).ToNot(BeNil())

	var workerMachinesCount string
	if input.ConfigCluster.WorkerMachineCount != nil {
		workerMachinesCount = fmt.Sprintf("%d", *input.ConfigCluster.WorkerMachineCount)
	} else {
		workerMachinesCount = "(unset)"
	}
	log.Logf("Creating the workload cluster with name %q using the %q template (Kubernetes %s, %d control-plane machines, %s worker machines)",
		input.ConfigCluster.ClusterName, valueOrDefault(input.ConfigCluster.Flavor), input.ConfigCluster.KubernetesVersion, *input.ConfigCluster.ControlPlaneMachineCount, workerMachinesCount)

	// Ensure we have a Cluster for dump and cleanup steps in AfterEach even if ApplyClusterTemplateAndWait fails.
	result.Cluster = &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      input.ConfigCluster.ClusterName,
			Namespace: input.ConfigCluster.Namespace,
		},
	}

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
		LogFolder:           input.ConfigCluster.LogFolder,
		ClusterctlVariables: input.ConfigCluster.ClusterctlVariables,
	})
	Expect(workloadClusterTemplate).ToNot(BeNil(), "Failed to get the cluster template")

	ApplyCustomClusterTemplateAndWait(ctx, ApplyCustomClusterTemplateAndWaitInput{
		ClusterProxy:                 input.ClusterProxy,
		CustomTemplateYAML:           workloadClusterTemplate,
		ClusterName:                  input.ConfigCluster.ClusterName,
		Namespace:                    input.ConfigCluster.Namespace,
		CNIManifestPath:              input.CNIManifestPath,
		Flavor:                       input.ConfigCluster.Flavor,
		WaitForClusterIntervals:      input.WaitForClusterIntervals,
		WaitForControlPlaneIntervals: input.WaitForControlPlaneIntervals,
		WaitForMachineDeployments:    input.WaitForMachineDeployments,
		WaitForMachinePools:          input.WaitForMachinePools,
		CreateOrUpdateOpts:           input.CreateOrUpdateOpts,
		PreWaitForCluster:            input.PreWaitForCluster,
		PostMachinesProvisioned:      input.PostMachinesProvisioned,
		ControlPlaneWaiters:          input.ControlPlaneWaiters,
	}, (*ApplyCustomClusterTemplateAndWaitResult)(result))
}

// ApplyCustomClusterTemplateAndWaitInput is the input type for ApplyCustomClusterTemplateAndWait.
type ApplyCustomClusterTemplateAndWaitInput struct {
	ClusterProxy                 framework.ClusterProxy
	CustomTemplateYAML           []byte
	ClusterName                  string
	Namespace                    string
	CNIManifestPath              string
	Flavor                       string
	WaitForClusterIntervals      []interface{}
	WaitForControlPlaneIntervals []interface{}
	WaitForMachineDeployments    []interface{}
	WaitForMachinePools          []interface{}
	CreateOrUpdateOpts           []framework.CreateOrUpdateOption // options to be passed to CreateOrUpdate function config
	PreWaitForCluster            func()
	PostMachinesProvisioned      func()
	ControlPlaneWaiters
}

type ApplyCustomClusterTemplateAndWaitResult struct {
	ClusterClass       *clusterv1.ClusterClass
	Cluster            *clusterv1.Cluster
	ControlPlane       *controlplanev1.KubeadmControlPlane
	MachineDeployments []*clusterv1.MachineDeployment
	MachinePools       []*expv1.MachinePool
}

func ApplyCustomClusterTemplateAndWait(ctx context.Context, input ApplyCustomClusterTemplateAndWaitInput, result *ApplyCustomClusterTemplateAndWaitResult) {
	setDefaults(&input)
	Expect(ctx).NotTo(BeNil(), "ctx is required for ApplyCustomClusterTemplateAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling ApplyCustomClusterTemplateAndWait")
	Expect(input.CustomTemplateYAML).NotTo(BeEmpty(), "Invalid argument. input.CustomTemplateYAML can't be empty when calling ApplyCustomClusterTemplateAndWait")
	Expect(input.ClusterName).NotTo(BeEmpty(), "Invalid argument. input.ClusterName can't be empty when calling ApplyCustomClusterTemplateAndWait")
	Expect(input.Namespace).NotTo(BeEmpty(), "Invalid argument. input.Namespace can't be empty when calling ApplyCustomClusterTemplateAndWait")
	Expect(result).ToNot(BeNil(), "Invalid argument. result can't be nil when calling ApplyClusterTemplateAndWait")

	log.Logf("Creating the workload cluster with name %q from the provided yaml", input.ClusterName)

	// Ensure we have a Cluster for dump and cleanup steps in AfterEach even if ApplyClusterTemplateAndWait fails.
	result.Cluster = &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      input.ClusterName,
			Namespace: input.Namespace,
		},
	}

	log.Logf("Applying the cluster template yaml of cluster %s", klog.KRef(input.Namespace, input.ClusterName))
	Eventually(func() error {
		return input.ClusterProxy.CreateOrUpdate(ctx, input.CustomTemplateYAML, input.CreateOrUpdateOpts...)
	}, 1*time.Minute).Should(Succeed(), "Failed to apply the cluster template")

	// Once we applied the cluster template we can run PreWaitForCluster.
	// Note: This can e.g. be used to verify the BeforeClusterCreate lifecycle hook is executed
	// and blocking correctly.
	if input.PreWaitForCluster != nil {
		log.Logf("Calling PreWaitForCluster for cluster %s", klog.KRef(input.Namespace, input.ClusterName))
		input.PreWaitForCluster()
	}

	log.Logf("Waiting for the cluster infrastructure of cluster %s to be provisioned", klog.KRef(input.Namespace, input.ClusterName))
	result.Cluster = framework.DiscoveryAndWaitForCluster(ctx, framework.DiscoveryAndWaitForClusterInput{
		Getter:    input.ClusterProxy.GetClient(),
		Namespace: input.Namespace,
		Name:      input.ClusterName,
	}, input.WaitForClusterIntervals...)

	if result.Cluster.Spec.Topology != nil {
		result.ClusterClass = framework.GetClusterClassByName(ctx, framework.GetClusterClassByNameInput{
			Getter:    input.ClusterProxy.GetClient(),
			Namespace: input.Namespace,
			Name:      result.Cluster.Spec.Topology.Class,
		})
	}

	log.Logf("Waiting for control plane of cluster %s to be initialized", klog.KRef(input.Namespace, input.ClusterName))
	input.WaitForControlPlaneInitialized(ctx, input, result)

	if input.CNIManifestPath != "" {
		log.Logf("Installing a CNI plugin to the workload cluster %s", klog.KRef(input.Namespace, input.ClusterName))
		workloadCluster := input.ClusterProxy.GetWorkloadCluster(ctx, result.Cluster.Namespace, result.Cluster.Name)

		cniYaml, err := os.ReadFile(input.CNIManifestPath)
		Expect(err).ShouldNot(HaveOccurred())

		Expect(workloadCluster.CreateOrUpdate(ctx, cniYaml)).ShouldNot(HaveOccurred())
	}

	log.Logf("Waiting for control plane of cluster %s to be ready", klog.KRef(input.Namespace, input.ClusterName))
	input.WaitForControlPlaneMachinesReady(ctx, input, result)

	log.Logf("Waiting for the machine deployments of cluster %s to be provisioned", klog.KRef(input.Namespace, input.ClusterName))
	result.MachineDeployments = framework.DiscoveryAndWaitForMachineDeployments(ctx, framework.DiscoveryAndWaitForMachineDeploymentsInput{
		Lister:  input.ClusterProxy.GetClient(),
		Cluster: result.Cluster,
	}, input.WaitForMachineDeployments...)

	log.Logf("Waiting for the machine pools of cluster %s to be provisioned", klog.KRef(input.Namespace, input.ClusterName))
	result.MachinePools = framework.DiscoveryAndWaitForMachinePools(ctx, framework.DiscoveryAndWaitForMachinePoolsInput{
		Getter:  input.ClusterProxy.GetClient(),
		Lister:  input.ClusterProxy.GetClient(),
		Cluster: result.Cluster,
	}, input.WaitForMachinePools...)

	if input.PostMachinesProvisioned != nil {
		log.Logf("Calling PostMachinesProvisioned for cluster %s", klog.KRef(input.Namespace, input.ClusterName))
		input.PostMachinesProvisioned()
	}
}

// setDefaults sets the default values for ApplyCustomClusterTemplateAndWaitInput if not set.
// Currently, we set the default ControlPlaneWaiters here, which are implemented for KubeadmControlPlane.
func setDefaults(input *ApplyCustomClusterTemplateAndWaitInput) {
	if input.WaitForControlPlaneInitialized == nil {
		input.WaitForControlPlaneInitialized = func(ctx context.Context, input ApplyCustomClusterTemplateAndWaitInput, result *ApplyCustomClusterTemplateAndWaitResult) {
			result.ControlPlane = framework.DiscoveryAndWaitForControlPlaneInitialized(ctx, framework.DiscoveryAndWaitForControlPlaneInitializedInput{
				Lister:  input.ClusterProxy.GetClient(),
				Cluster: result.Cluster,
			}, input.WaitForControlPlaneIntervals...)
		}
	}

	if input.WaitForControlPlaneMachinesReady == nil {
		input.WaitForControlPlaneMachinesReady = func(ctx context.Context, input ApplyCustomClusterTemplateAndWaitInput, result *ApplyCustomClusterTemplateAndWaitResult) {
			framework.WaitForControlPlaneAndMachinesReady(ctx, framework.WaitForControlPlaneAndMachinesReadyInput{
				GetLister:    input.ClusterProxy.GetClient(),
				Cluster:      result.Cluster,
				ControlPlane: result.ControlPlane,
			}, input.WaitForControlPlaneIntervals...)
		}
	}
}
