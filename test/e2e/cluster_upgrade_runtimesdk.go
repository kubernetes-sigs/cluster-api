/*
Copyright 2021 The Kubernetes Authors.

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

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/exp/topology/desiredstate"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/hooks"
	"sigs.k8s.io/cluster-api/test/e2e/internal/log"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
)

// The Cluster API test extension uses a ConfigMap named cluster-name + suffix to determine answers to the lifecycle hook calls;
// This test uses this ConfigMap to test hooks blocking first, and then hooks passing.

var hookResponsesConfigMapNameSuffix = "test-extension-hookresponses"

func hookResponsesConfigMapName(clusterName, extensionConfigName string) string {
	return fmt.Sprintf("%s-%s-%s", clusterName, extensionConfigName, hookResponsesConfigMapNameSuffix)
}

// ClusterUpgradeWithRuntimeSDKSpecInput is the input for clusterUpgradeWithRuntimeSDKSpec.
type ClusterUpgradeWithRuntimeSDKSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool

	// InfrastructureProviders specifies the infrastructure to use for clusterctl
	// operations (Example: get cluster templates).
	// Note: In most cases this need not be specified. It only needs to be specified when
	// multiple infrastructure providers are installed on the cluster as clusterctl will not be
	// able to identify the default.
	InfrastructureProvider *string

	// ControlPlaneMachineCount is used in `config cluster` to configure the count of the control plane machines used in the test.
	// Default is 1.
	ControlPlaneMachineCount *int64

	// WorkerMachineCount is used in `config cluster` to configure the count of the worker machines used in the test.
	// NOTE: If the WORKER_MACHINE_COUNT var is used multiple times in the cluster template, the absolute count of
	// worker machines is a multiple of WorkerMachineCount.
	// Default is 2.
	WorkerMachineCount *int64

	// Flavor to use when creating the cluster for testing, "upgrades" is used if not specified.
	Flavor *string

	// KubernetesVersionFrom allow to specify the Kubernetes version from for this test.
	// If not specified, the KUBERNETES_VERSION_UPGRADE_FROM env variable will be used
	KubernetesVersionFrom string

	// KubernetesVersions allows providing a list of Kubernetes version to be used for computing upgrade paths.
	// This list will be considered only if there is no a list in the ClusterClass created from the template.
	KubernetesVersions []string

	// Allows injecting a function to be run after test namespace is created.
	// If not specified, this is a no-op.
	PostNamespaceCreated func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace string)

	// Allows injecting a function to be run after the cluster is upgraded.
	// If not specified, this is a no-op.
	PostUpgrade func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace, workloadClusterName string)

	// ExtensionConfigName is the name of the ExtensionConfig. Defaults to "k8s-upgrade-with-runtimesdk".
	// This value is provided to clusterctl as "EXTENSION_CONFIG_NAME" variable and can be used to template the
	// name of the ExtensionConfig into the ClusterClass.
	ExtensionConfigName string

	// ExtensionServiceNamespace is the namespace where the service for the Runtime SDK is located
	// and is used to configure in the test-namespace scoped ExtensionConfig.
	ExtensionServiceNamespace string

	// ExtensionServiceName is the name of the service to configure in the test-namespace scoped ExtensionConfig.
	ExtensionServiceName string

	// DeployClusterClassInSeparateNamespace defines if the ClusterClass should be deployed in a separate namespace.
	DeployClusterClassInSeparateNamespace bool
}

// ClusterUpgradeWithRuntimeSDKSpec implements a spec that upgrades a cluster and runs the Kubernetes conformance suite.
// Upgrading a cluster refers to upgrading the control-plane and worker nodes (managed by MD and machine pools).
// NOTE: This test only works with a KubeadmControlPlane.
// NOTE: This test works with Clusters with and without ClusterClass.
// When using ClusterClass, the ClusterClass must have the variables "etcdImageTag" and "coreDNSImageTag" of type string.
// Those variables should have corresponding patches which set the etcd and CoreDNS tags in KCP.
func ClusterUpgradeWithRuntimeSDKSpec(ctx context.Context, inputGetter func() ClusterUpgradeWithRuntimeSDKSpecInput) {
	const (
		specName = "k8s-upgrade-with-runtimesdk"
	)

	var (
		input                            ClusterUpgradeWithRuntimeSDKSpecInput
		namespace, clusterClassNamespace *corev1.Namespace
		cancelWatches                    context.CancelFunc

		controlPlaneMachineCount int64
		workerMachineCount       int64

		clusterResources *clusterctl.ApplyClusterTemplateAndWaitResult
		clusterName      string
	)

	BeforeEach(func() {
		Expect(ctx).NotTo(BeNil(), "ctx is required for %s spec", specName)
		input = inputGetter()
		Expect(input.E2EConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. input.BootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0750)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		if input.KubernetesVersionFrom == "" {
			Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersionUpgradeFrom))
		}
		Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersionUpgradeTo))

		Expect(input.ExtensionServiceNamespace).ToNot(BeEmpty())
		Expect(input.ExtensionServiceName).ToNot(BeEmpty())
		if input.ExtensionConfigName == "" {
			input.ExtensionConfigName = specName
		}

		if input.ControlPlaneMachineCount == nil {
			controlPlaneMachineCount = 1
		} else {
			controlPlaneMachineCount = *input.ControlPlaneMachineCount
		}

		if input.WorkerMachineCount == nil {
			workerMachineCount = 2
		} else {
			workerMachineCount = *input.WorkerMachineCount
		}

		// Set up a Namespace where to host objects for this spec and create a watcher for the Namespace events.
		namespace, cancelWatches = framework.SetupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, input.PostNamespaceCreated)
		if input.DeployClusterClassInSeparateNamespace {
			clusterClassNamespace = framework.CreateNamespace(ctx, framework.CreateNamespaceInput{Creator: input.BootstrapClusterProxy.GetClient(), Name: fmt.Sprintf("%s-clusterclass", namespace.Name)}, "40s", "10s")
			Expect(clusterClassNamespace).ToNot(BeNil(), "Failed to create namespace")
		}
		clusterName = fmt.Sprintf("%s-%s", specName, util.RandomString(6))
		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should create, upgrade and delete a workload cluster", func() {
		// NOTE: test extension is already deployed in the management cluster. If for any reason in future we want
		// to make this test more self-contained, this test should be modified in order to create an additional
		// management cluster; also, the E2E test configuration should be modified introducing something like
		// optional:true allowing to define which providers should not be installed by default in
		// a management cluster.

		By("Deploy Test Extension ExtensionConfig")

		namespaces := []string{namespace.Name}
		if input.DeployClusterClassInSeparateNamespace {
			namespaces = append(namespaces, clusterClassNamespace.Name)
		}

		// In this test we are defaulting all handlers to blocking because we expect the handlers to block the
		// cluster lifecycle by default. Setting defaultAllHandlersToBlocking to true enforces that the test-extension
		// automatically creates the ConfigMap with blocking preloaded responses.
		Expect(input.BootstrapClusterProxy.GetClient().Create(ctx,
			extensionConfig(input.ExtensionConfigName, input.ExtensionServiceNamespace, input.ExtensionServiceName, true, true, namespaces...))).
			To(Succeed(), "Failed to create the extension config")

		By("Creating a workload cluster; creation waits for BeforeClusterCreateHook to gate the operation")

		infrastructureProvider := clusterctl.DefaultInfrastructureProvider
		if input.InfrastructureProvider != nil {
			infrastructureProvider = *input.InfrastructureProvider
		}

		variables := map[string]string{
			// This is used to template the name of the ExtensionConfig into the ClusterClass.
			"EXTENSION_CONFIG_NAME": input.ExtensionConfigName,
		}
		if input.DeployClusterClassInSeparateNamespace {
			variables["CLUSTER_CLASS_NAMESPACE"] = clusterClassNamespace.Name
		}

		fromVersion := input.KubernetesVersionFrom
		if fromVersion == "" {
			fromVersion = input.E2EConfig.MustGetVariable(KubernetesVersionUpgradeFrom)
		}
		toVersion := input.E2EConfig.MustGetVariable(KubernetesVersionUpgradeTo)

		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   infrastructureProvider,
				Flavor:                   ptr.Deref(input.Flavor, "upgrades"),
				Namespace:                namespace.Name,
				ClusterName:              clusterName,
				KubernetesVersion:        fromVersion,
				ControlPlaneMachineCount: ptr.To[int64](controlPlaneMachineCount),
				WorkerMachineCount:       ptr.To[int64](workerMachineCount),
				ClusterctlVariables:      variables,
			},
			PreWaitForCluster: func() {
				cluster := framework.GetClusterByName(ctx, framework.GetClusterByNameInput{
					Name: clusterName, Namespace: namespace.Name, Getter: input.BootstrapClusterProxy.GetClient()})

				// Check for the beforeClusterUpgrade being called, then unblock
				beforeClusterCreateTestHandler(ctx,
					input.BootstrapClusterProxy.GetClient(),
					cluster,
					input.ExtensionConfigName)
			},
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
			WaitForMachinePools:          input.E2EConfig.GetIntervals(specName, "wait-machine-pool-nodes"),
		}, clusterResources)

		// Compute the upgrade plan that must be followed during the upgrade, so it can be validated step by step in the second
		// part of the test.
		// By default, the test assumes we are upgrading by one minor, however:
		// - If the cluster class provides a list of Kubernetes versions, this list will be used to compute the
		//   upgrade plan, which can span multiple versions in this case.
		// - If the test has in input a list of Kubernetes versions, the cluster class will be update using it
		//   and used like in the example above.

		getUpgradePlanFunc := desiredstate.GetUpgradePlanOneMinor
		if len(clusterResources.ClusterClass.Spec.KubernetesVersions) != 0 {
			log.Logf("Using Kubernetes versions from cluster class: %s", clusterResources.ClusterClass.Spec.KubernetesVersions)
			getUpgradePlanFunc = desiredstate.GetUpgradePlanFromClusterClassVersions(clusterResources.ClusterClass.Spec.KubernetesVersions)
		} else if len(input.KubernetesVersions) != 0 {
			log.Logf("Using Kubernetes versions provided as input to the test: %s", input.KubernetesVersions)
			clusterResources.ClusterClass.Spec.KubernetesVersions = input.KubernetesVersions
			Expect(input.BootstrapClusterProxy.GetClient().Update(ctx, clusterResources.ClusterClass)).To(Succeed(), "Failed to update cluster class %s with Kubernetes versions", klog.KObj(clusterResources.ClusterClass))

			getUpgradePlanFunc = desiredstate.GetUpgradePlanFromClusterClassVersions(input.KubernetesVersions)
		}

		controlPlaneUpgradePlan, workersUpgradePlan, err := getUpgradePlanFunc(ctx, toVersion, fromVersion, fromVersion)
		Expect(err).ToNot(HaveOccurred(), "Failed to get the upgrade plan")

		workersUpgradePlan, err = desiredstate.DefaultAndValidateUpgradePlans(toVersion, fromVersion, fromVersion, controlPlaneUpgradePlan, workersUpgradePlan)
		Expect(err).ToNot(HaveOccurred(), "Failed to default and validate the upgrade plan")

		log.Logf("Control Plane upgrade plan: %s", controlPlaneUpgradePlan)
		log.Logf("Workers upgrade plan: %s", workersUpgradePlan)

		// Perform the upgrade and check everything is working as expected.
		// More specifically:
		// - After control plane upgrade steps
		// 	 - Check control plane machines are on the desired version, nodes are healthy
		// 	 - Workers machines are still on the old version, nodes are healthy
		//   - MachineSet preflight checks are preventing creation on machines in the old version (this is implemented before unblocking the afterControlPlaneUpgradeTestHandler)
		// - After workers upgrade steps
		// 	 - Check control plane machines are on the desired version, nodes are healthy
		// 	 - Workers machines are still are on the desired version, nodes are healthy
		// - Check lifecycle hooks before and after every step

		controlPlaneVersion := fromVersion
		workersVersion := fromVersion

		// wait for all Machines to exist before starting the upgrade.
		waitControlPlaneVersion(ctx, input.BootstrapClusterProxy.GetClient(), clusterResources.Cluster, controlPlaneVersion, input.E2EConfig.GetIntervals(specName, "wait-control-plane-upgrade"))
		waitWorkersVersions(ctx, input.BootstrapClusterProxy.GetClient(), clusterResources.Cluster, workersVersion, input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"))

		// Add a BeforeClusterUpgrade hook annotation to block via the annotation.
		beforeClusterUpgradeAnnotation := clusterv1.BeforeClusterUpgradeHookAnnotationPrefix + "/upgrade-test"
		patchHelper, err := patch.NewHelper(clusterResources.Cluster, input.BootstrapClusterProxy.GetClient())
		Expect(err).ToNot(HaveOccurred())
		clusterResources.Cluster.Annotations[beforeClusterUpgradeAnnotation] = ""
		Expect(patchHelper.Patch(ctx, clusterResources.Cluster)).To(Succeed())

		// Upgrade the Cluster topology to run through an entire cluster lifecycle to test the lifecycle hooks.
		By("Upgrading the Cluster topology")
		framework.UpgradeClusterTopologyAndWaitForUpgrade(ctx, framework.UpgradeClusterTopologyAndWaitForUpgradeInput{
			ClusterProxy:                         input.BootstrapClusterProxy,
			Cluster:                              clusterResources.Cluster,
			ControlPlane:                         clusterResources.ControlPlane,
			MachineDeployments:                   clusterResources.MachineDeployments,
			MachinePools:                         clusterResources.MachinePools,
			KubernetesUpgradeVersion:             toVersion,
			WaitForControlPlaneToBeUpgraded:      input.E2EConfig.GetIntervals(specName, "wait-control-plane-upgrade"),
			WaitForMachineDeploymentToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-machine-deployment-upgrade"),
			WaitForMachinePoolToBeUpgraded:       input.E2EConfig.GetIntervals(specName, "wait-machine-pool-upgrade"),
			WaitForKubeProxyUpgrade:              input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			WaitForDNSUpgrade:                    input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			WaitForEtcdUpgrade:                   input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			PreWaitForControlPlaneToBeUpgraded: func() {
				// WaitForControlPlaneToBeUpgraded inside UpgradeClusterTopologyAndWaitForUpgrade checks if all the machines are at the target
				// version; however, in this test we want a finer control of what happen during CP upgrade, so we use this func to go through
				// the control plane upgrade step by step using hooks to block/unblock every phase.

				// Check for the beforeClusterUpgrade being called, then unblock
				Expect(controlPlaneUpgradePlan).ToNot(BeEmpty())
				firstControlPlaneVersion := controlPlaneUpgradePlan[0]

				beforeClusterUpgradeTestHandler(ctx,
					input.BootstrapClusterProxy.GetClient(),
					clusterResources.Cluster,
					input.ExtensionConfigName,
					fromVersion,              // Cluster fromVersion
					toVersion,                // Cluster toVersion
					firstControlPlaneVersion, // firstControlPlaneVersion in the upgrade plan, used to check the BeforeControlPlaneUpgrade is not called before its time.
				)

				// Then check the upgrade is progressing step by step according to the upgrade plan
				for i, version := range controlPlaneUpgradePlan {
					// make sure beforeControlPlaneUpgrade still blocks, then unblock the upgrade.
					beforeControlPlaneUpgradeTestHandler(ctx,
						input.BootstrapClusterProxy.GetClient(),
						clusterResources.Cluster,
						input.ExtensionConfigName,
						controlPlaneVersion, // Control plane fromVersion for this upgrade step.
						version,             // Control plane toVersion for this upgrade step.
						workersVersion,      // Current workersVersion, used to check workers do not upgrade before its time.
					)

					// Wait CP to update to version
					controlPlaneVersion = version
					waitControlPlaneVersion(ctx, input.BootstrapClusterProxy.GetClient(), clusterResources.Cluster, controlPlaneVersion, input.E2EConfig.GetIntervals(specName, "wait-control-plane-upgrade"))

					// Check workers are not yet upgraded.
					checkWorkersVersions(ctx, input.BootstrapClusterProxy.GetClient(), clusterResources.Cluster, workersVersion)

					// make sure afterControlPlaneUpgrade still blocks, then unblock the upgrade.
					nextControlPlaneVersion := ""
					if i < len(controlPlaneUpgradePlan)-1 {
						nextControlPlaneVersion = controlPlaneUpgradePlan[i+1]
					}
					afterControlPlaneUpgradeTestHandler(ctx,
						input.BootstrapClusterProxy.GetClient(),
						clusterResources.Cluster,
						input.ExtensionConfigName,
						controlPlaneVersion,     // Current controlPlaneVersion for this upgrade step.
						workersVersion,          // Current workersVersion, used to check workers do not upgrade before its time.
						nextControlPlaneVersion, // nextControlPlaneVersion in the upgrade plan, used to check the BeforeControlPlaneUpgrade is not called before its time (in case workers do not perform this upgrade step).
						toVersion,               // toVersion of the upgrade, used to check the AfterClusterUpgrade is not called before its time (in case workers do not perform this upgrade step).
					)

					// If worker should not upgrade at this step, continue
					if !sets.New[string](workersUpgradePlan...).Has(version) {
						continue
					}

					// make sure beforeWorkersUpgrade still blocks, then unblock the upgrade.
					beforeWorkersUpgradeTestHandler(ctx,
						input.BootstrapClusterProxy.GetClient(),
						clusterResources.Cluster,
						input.ExtensionConfigName,
						workersVersion, // Current workersVersion for this upgrade step.
						version,        // Workers toVersion for this upgrade step.
					)

					// Wait for workers to update to version
					workersVersion = version
					waitWorkersVersions(ctx, input.BootstrapClusterProxy.GetClient(), clusterResources.Cluster, workersVersion, input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"))

					// make sure afterWorkersUpgradeTestHandler still blocks, then unblock the upgrade.
					afterWorkersUpgradeTestHandler(ctx,
						input.BootstrapClusterProxy.GetClient(),
						clusterResources.Cluster,
						input.ExtensionConfigName,
						controlPlaneVersion,     // Current controlPlaneVersion for this upgrade step.
						workersVersion,          // Current workersVersion for this upgrade step.
						nextControlPlaneVersion, // nextControlPlaneVersion in the upgrade plan, used to check the BeforeControlPlaneUpgrade is not called before its time (in case workers do not perform this upgrade step).
						toVersion,               // toVersion of the upgrade, used to check the AfterClusterUpgrade is not called before its time (in case workers do not perform this upgrade step).
					)
				}
			},
		})

		// Check if the AfterClusterUpgrade hook is actually called.
		afterAfterClusterUpgradeTestHandler(ctx,
			input.BootstrapClusterProxy.GetClient(),
			clusterResources.Cluster,
			input.ExtensionConfigName,
			toVersion, // toVersion of the upgrade.
		)

		By("Waiting until nodes are ready")
		workloadProxy := input.BootstrapClusterProxy.GetWorkloadCluster(ctx, namespace.Name, clusterResources.Cluster.Name)
		workloadClient := workloadProxy.GetClient()
		framework.WaitForNodesReady(ctx, framework.WaitForNodesReadyInput{
			Lister:            workloadClient,
			KubernetesVersion: toVersion,
			Count:             int(clusterResources.ExpectedTotalNodes()),
			WaitForNodesReady: input.E2EConfig.GetIntervals(specName, "wait-nodes-ready"),
		})

		if input.PostUpgrade != nil {
			log.Logf("Calling PostMachinesProvisioned for cluster %s", klog.KRef(namespace.Name, clusterResources.Cluster.Name))
			input.PostUpgrade(input.BootstrapClusterProxy, namespace.Name, clusterResources.Cluster.Name)
		}

		Byf("Verify Cluster Available condition is true")
		framework.VerifyClusterAvailable(ctx, framework.VerifyClusterAvailableInput{
			Getter:    input.BootstrapClusterProxy.GetClient(),
			Name:      clusterResources.Cluster.Name,
			Namespace: clusterResources.Cluster.Namespace,
		})

		Byf("Verify Machines Ready condition is true")
		framework.VerifyMachinesReady(ctx, framework.VerifyMachinesReadyInput{
			Lister:    input.BootstrapClusterProxy.GetClient(),
			Name:      clusterResources.Cluster.Name,
			Namespace: clusterResources.Cluster.Namespace,
		})

		By("Dumping resources and deleting the workload cluster; deletion waits for BeforeClusterDeleteHook to gate the operation")
		dumpAndDeleteCluster(ctx, input.BootstrapClusterProxy, input.ClusterctlConfigPath, namespace.Name, clusterName, input.ArtifactFolder)

		beforeClusterDeleteHandler(ctx, input.BootstrapClusterProxy.GetClient(), clusterResources.Cluster, input.ExtensionConfigName)

		Byf("Waiting for cluster to be deleted")
		framework.WaitForClusterDeleted(ctx, framework.WaitForClusterDeletedInput{
			ClusterProxy:         input.BootstrapClusterProxy,
			ClusterctlConfigPath: input.ClusterctlConfigPath,
			Cluster:              clusterResources.Cluster,
			ArtifactFolder:       input.ArtifactFolder,
		}, input.E2EConfig.GetIntervals(specName, "wait-delete-cluster")...)

		By("Checking all lifecycle hooks have been called")
		// Assert that each hook has been called and returned "Success" during the test.
		expectedHooks := map[string]string{
			computeHookName("BeforeClusterCreate", nil):                               "Status: Success, RetryAfterSeconds: 0",
			computeHookName("AfterControlPlaneInitialized", nil):                      "Success",
			computeHookName("BeforeClusterUpgrade", []string{fromVersion, toVersion}): "Status: Success, RetryAfterSeconds: 0",
			computeHookName("AfterClusterUpgrade", []string{toVersion}):               "Status: Success, RetryAfterSeconds: 0",
			computeHookName("BeforeClusterDelete", nil):                               "Status: Success, RetryAfterSeconds: 0",
		}
		fromv := fromVersion
		for _, v := range controlPlaneUpgradePlan {
			expectedHooks[computeHookName("BeforeControlPlaneUpgrade", []string{fromv, v})] = "Status: Success, RetryAfterSeconds: 0"
			expectedHooks[computeHookName("AfterControlPlaneUpgrade", []string{v})] = "Status: Success, RetryAfterSeconds: 0"
			fromv = v
		}
		fromv = fromVersion
		for _, v := range workersUpgradePlan {
			expectedHooks[computeHookName("BeforeWorkersUpgrade", []string{fromv, v})] = "Status: Success, RetryAfterSeconds: 0"
			expectedHooks[computeHookName("AfterWorkersUpgrade", []string{v})] = "Status: Success, RetryAfterSeconds: 0"
			fromv = v
		}

		checkLifecycleHookResponses(ctx, input.BootstrapClusterProxy.GetClient(), clusterResources.Cluster, input.ExtensionConfigName, expectedHooks)

		By("PASSED!")
	})

	AfterEach(func() {
		// Dump all the resources in the spec namespace and the workload cluster.
		framework.DumpAllResourcesAndLogs(ctx, input.BootstrapClusterProxy, input.ClusterctlConfigPath, input.ArtifactFolder, namespace, clusterResources.Cluster)

		if !input.SkipCleanup {
			// Delete the extensionConfig first to ensure the BeforeDeleteCluster hook doesn't block deletion.
			Eventually(func() error {
				return input.BootstrapClusterProxy.GetClient().Delete(ctx, &runtimev1.ExtensionConfig{ObjectMeta: metav1.ObjectMeta{Name: input.ExtensionConfigName}})
			}, 10*time.Second, 1*time.Second).Should(Succeed(), "Deleting ExtensionConfig failed")

			Byf("Deleting cluster %s", klog.KObj(clusterResources.Cluster))
			// While https://github.com/kubernetes-sigs/cluster-api/issues/2955 is addressed in future iterations, there is a chance
			// that cluster variable is not set even if the cluster exists, so we are calling DeleteAllClustersAndWait
			// instead of DeleteClusterAndWait
			framework.DeleteAllClustersAndWait(ctx, framework.DeleteAllClustersAndWaitInput{
				ClusterProxy:         input.BootstrapClusterProxy,
				ClusterctlConfigPath: input.ClusterctlConfigPath,
				Namespace:            namespace.Name,
				ArtifactFolder:       input.ArtifactFolder,
			}, input.E2EConfig.GetIntervals(specName, "wait-delete-cluster")...)

			Byf("Deleting namespace used for hosting the %q test spec", specName)
			framework.DeleteNamespace(ctx, framework.DeleteNamespaceInput{
				Deleter: input.BootstrapClusterProxy.GetClient(),
				Name:    namespace.Name,
			})

			if input.DeployClusterClassInSeparateNamespace {
				Byf("Deleting namespace used for hosting the %q test spec ClusterClass", specName)
				framework.DeleteNamespace(ctx, framework.DeleteNamespaceInput{
					Deleter: input.BootstrapClusterProxy.GetClient(),
					Name:    clusterClassNamespace.Name,
				})
			}
		}
		cancelWatches()
	})
}

// machineSetPreflightChecksTest verifies the MachineSet preflight checks.
// At this point in the test the ControlPlane is upgraded to the new version, and the upgrade to the MachineDeployments
// should be blocked by the AfterControlPlaneUpgrade hook.
// Test the MachineSet preflight checks by scaling up the MachineDeployment. The creation on the new Machine
// should be blocked because the preflight checks should not pass (kubeadm version skew preflight check should fail).
func machineSetPreflightChecksTest(ctx context.Context, c client.Client, cluster *clusterv1.Cluster) {
	log.Logf("Test the MachineSet preflight checks")

	// Scale up the MachineDeployment
	machineDeployments := framework.GetMachineDeploymentsByCluster(ctx, framework.GetMachineDeploymentsByClusterInput{
		Lister:      c,
		ClusterName: cluster.Name,
		Namespace:   cluster.Namespace,
	})
	md := machineDeployments[0]

	log.Logf("Scaling up MD %s", md.Name)

	// Note: It is fair to assume that the Cluster is ClusterClass based since RuntimeSDK
	// is only supported for ClusterClass based Clusters.
	patchHelper, err := patch.NewHelper(md, c)
	Expect(err).ToNot(HaveOccurred())

	// Scale up the MachineDeployment.
	// IMPORTANT: Since the MachineDeployment is pending an upgrade, at this point the topology controller will not push any changes
	// to the MachineDeployment. Therefore, the changes made to the MachineDeployment here will not be replaced
	// until the AfterControlPlaneUpgrade hook unblocks the upgrade.
	*md.Spec.Replicas++
	Eventually(func() error {
		return patchHelper.Patch(ctx, md)
	}).Should(Succeed(), "Failed to scale up the MachineDeployment %s", klog.KObj(md))
	// Verify the MachineDeployment updated replicas are not overridden by the topology controller.
	// Note: This verifies that the topology controller in fact holds any reconciliation of this MachineDeployment.
	Consistently(func(g Gomega) {
		// Get the updated MachineDeployment.
		targetMD := &clusterv1.MachineDeployment{}
		// Wrap in an Eventually block for additional safety. Since all of this is in a Consistently block it
		// will fail if we hit a transient error like a network flake.
		g.Eventually(func() error {
			return c.Get(ctx, client.ObjectKeyFromObject(md), targetMD)
		}).Should(Succeed(), "Failed to get MachineDeployment %s", klog.KObj(md))
		// Verify replicas are not overridden.
		g.Expect(targetMD.Spec.Replicas).To(Equal(md.Spec.Replicas))
	}, 10*time.Second, 1*time.Second).Should(Succeed())

	log.Logf("Checking that a new MachineSet is created, but no machine are created for MD %s (upgrade is still pending)", md.Name)

	// Since the MachineDeployment is scaled up (overriding the topology controller) at this point the MachineSet would
	// also scale up. However, a new Machine creation would be blocked by one of the MachineSet preflight checks (KubeadmVersionSkew).
	// Verify the MachineSet is blocking new Machine creation.
	Eventually(func(g Gomega) {
		machineSets := framework.GetMachineSetsByDeployment(ctx, framework.GetMachineSetsByDeploymentInput{
			Lister:    c,
			MDName:    md.Name,
			Namespace: md.Namespace,
		})
		// Check required replicas are like expected
		g.Expect(machineSets[0].Spec.Replicas).To(Equal(md.Spec.Replicas))

		// Check conditions to surface the MS cannot scale up due to preflight checks.
		g.Expect(conditions.IsTrue(machineSets[0], clusterv1.MachineSetScalingUpCondition)).To(BeTrue())
		scalingUpCondition := conditions.Get(machineSets[0], clusterv1.MachineSetScalingUpCondition)
		g.Expect(scalingUpCondition).NotTo(BeNil())
		g.Expect(scalingUpCondition.Reason).To(Equal(clusterv1.MachineSetScalingUpReason))
		g.Expect(scalingUpCondition.Message).To(ContainSubstring("\"KubeadmVersionSkew\" preflight check failed"))

		// Check v1beta1 conditions to surface the MS cannot scale up due to preflight checks.
		g.Expect(v1beta1conditions.IsFalse(machineSets[0], clusterv1.MachinesCreatedV1Beta1Condition)).To(BeTrue())
		machinesCreatedCondition := v1beta1conditions.Get(machineSets[0], clusterv1.MachinesCreatedV1Beta1Condition)
		g.Expect(machinesCreatedCondition).NotTo(BeNil())
		g.Expect(machinesCreatedCondition.Reason).To(Equal(clusterv1.PreflightCheckFailedV1Beta1Reason))
	}).Should(Succeed(), "New Machine creation not blocked by MachineSet preflight checks")

	// Verify that the MachineSet is not creating the new Machine.
	// No new machines should be created for this MachineDeployment even though it is scaled up.
	// Creation of new Machines will be blocked by MachineSet preflight checks (KubeadmVersionSkew).
	Consistently(func(g Gomega) {
		originalReplicas := int(*md.Spec.Replicas - 1)
		machines := framework.GetMachinesByMachineDeployments(ctx, framework.GetMachinesByMachineDeploymentsInput{
			Lister:            c,
			ClusterName:       cluster.Name,
			Namespace:         cluster.Namespace,
			MachineDeployment: *md,
		})
		g.Expect(machines).To(HaveLen(originalReplicas), "New Machines should not be created")
	}, 10*time.Second, time.Second).Should(Succeed())

	log.Logf("Scaling down MD %s to the original state", md.Name)

	// Scale down the MachineDeployment to the original replicas to restore to the state of the MachineDeployment
	// it existed in before this test block.
	patchHelper, err = patch.NewHelper(md, c)
	Expect(err).ToNot(HaveOccurred())
	*md.Spec.Replicas--
	Eventually(func() error {
		return patchHelper.Patch(ctx, md)
	}).Should(Succeed(), "Failed to scale down the MachineDeployment %s", klog.KObj(md))
}

// extensionConfig generates an ExtensionConfig.
// We make sure this cluster-wide object does not conflict with others by using a random generated
// name and a NamespaceSelector selecting on the namespace of the current test.
// Thus, this object is "namespaced" to the current test even though it's a cluster-wide object.
func extensionConfig(name, extensionServiceNamespace, extensionServiceName string, disableInPlaceUpdates, defaultAllHandlersToBlocking bool, namespaces ...string) *runtimev1.ExtensionConfig {
	cfg := &runtimev1.ExtensionConfig{
		ObjectMeta: metav1.ObjectMeta{
			// Note: We have to use a constant name here as we have to be able to reference it in the ClusterClass
			// when configuring external patches.
			Name: name,
			Annotations: map[string]string{
				// Note: this assumes the test extension gets deployed in the default namespace defined in its own runtime-extensions-components.yaml
				runtimev1.InjectCAFromSecretAnnotation: fmt.Sprintf("%s/%s-cert", extensionServiceNamespace, extensionServiceName),
			},
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				Service: runtimev1.ServiceReference{
					Name: extensionServiceName,
					// Note: this assumes the test extension gets deployed in the default namespace defined in its own runtime-extensions-components.yaml
					Namespace: extensionServiceNamespace,
				},
			},
			Settings: map[string]string{
				"extensionConfigName":          name,
				"disableInPlaceUpdates":        strconv.FormatBool(disableInPlaceUpdates),
				"defaultAllHandlersToBlocking": strconv.FormatBool(defaultAllHandlersToBlocking),
			},
		},
	}
	if len(namespaces) > 0 {
		cfg.Spec.NamespaceSelector = &metav1.LabelSelector{
			// Note: we are limiting the test extension to be used by the namespace where the test is run.
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "kubernetes.io/metadata.name",
					Operator: metav1.LabelSelectorOpIn,
					Values:   namespaces,
				},
			},
		}
	}
	return cfg
}

// Check that each hook in hooks has been called at least once by checking if its actualResponseStatus is in the hook response configmap.
// If the provided hooks have both keys and values, check that the values match those in the hook response configmap.
func checkLifecycleHookResponses(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, extensionConfigName string, expectedHookResponses map[string]string) {
	Eventually(func() error {
		responseData := getLifecycleHookResponsesFromConfigMap(ctx, c, cluster, extensionConfigName)
		for hookName, expectedResponse := range expectedHookResponses {
			actualResponse, ok := responseData[hookName+"-actualResponseStatus"]
			if !ok {
				return errors.Errorf("hook %s call not recorded in configMap %s", hookName, klog.KRef(cluster.Namespace, hookResponsesConfigMapName(cluster.Name, extensionConfigName)))
			}
			if expectedResponse != "" && expectedResponse != actualResponse {
				return errors.Errorf("hook %s was expected to be %s in configMap got %s", hookName, expectedResponse, actualResponse)
			}
		}
		return nil
	}, 30*time.Second).Should(Succeed(), "Lifecycle hook calls were not as expected")
}

// Check that each hook in expectedHooks has been called at least once by checking if its actualResponseStatus is in the hook response configmap.
func checkLifecycleHooksCalledAtLeastOnce(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, extensionConfigName string, hook string, attributes []string) error {
	responseData := getLifecycleHookResponsesFromConfigMap(ctx, c, cluster, extensionConfigName)
	hookName := computeHookName(hook, attributes)
	if _, ok := responseData[hookName+"-actualResponseStatus"]; !ok {
		return errors.Errorf("hook %s call not recorded in configMap %s", hookName, klog.KRef(cluster.Namespace, hookResponsesConfigMapName(cluster.Name, extensionConfigName)))
	}
	return nil
}

func getLifecycleHookResponsesFromConfigMap(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, extensionConfigName string) map[string]string {
	configMap := &corev1.ConfigMap{}
	Eventually(func() error {
		return c.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: hookResponsesConfigMapName(cluster.Name, extensionConfigName)}, configMap)
	}).Should(Succeed(), "Failed to get the hook response configmap")
	return configMap.Data
}

// beforeClusterCreateTestHandler calls runtimeHookTestHandler with a blockedCondition function which returns false if
// the Cluster has a control plane or an infrastructure reference.
func beforeClusterCreateTestHandler(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, extensionConfigName string) {
	hookName := "BeforeClusterCreate"

	// for BeforeClusterCreate, the hook is blocking if Get Cluster keep returning a cluster without infraCluster and controlPlaneRef set.
	isBlockingCreate := func() bool {
		blocked := true
		cluster := framework.GetClusterByName(ctx,
			framework.GetClusterByNameInput{Name: cluster.Name, Namespace: cluster.Namespace, Getter: c})

		if cluster.Spec.InfrastructureRef.IsDefined() || cluster.Spec.ControlPlaneRef.IsDefined() {
			blocked = false
		}
		return blocked
	}
	runtimeHookTestHandler(ctx, c, cluster, extensionConfigName, hookName, nil, isBlockingCreate, nil)

	Byf("BeforeClusterCreate unblocked")
}

// beforeClusterUpgradeTestHandler calls runtimeHookTestHandler with a blocking function which returns false if
// any of the control plane, machine deployments or machine pools has been updated from the initial Kubernetes version.
func beforeClusterUpgradeTestHandler(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, extensionConfigName string, fromVersion, toVersion, firstControlPlaneVersion string) {
	hookName := "BeforeClusterUpgrade"
	beforeClusterUpgradeAnnotation := clusterv1.BeforeClusterUpgradeHookAnnotationPrefix + "/upgrade-test"

	// for BeforeClusterUpgrade, the hook is blocking if both controlPlane and workers remain at the expected version (both controlPlane and workers should be at fromVersion)
	// and the BeforeControlPlaneUpgrade hook is not called yet.
	isBlockingUpgrade := func() bool {
		controlPlane, err := external.GetObjectFromContractVersionedRef(ctx, c, cluster.Spec.ControlPlaneRef, cluster.Namespace)
		if err != nil {
			return false
		}
		controlPlaneVersion, err := contract.ControlPlane().Version().Get(controlPlane)
		if err != nil {
			return false
		}
		if *controlPlaneVersion != fromVersion {
			return false
		}

		controlPlaneMachines := framework.GetControlPlaneMachinesByCluster(ctx,
			framework.GetControlPlaneMachinesByClusterInput{Lister: c, ClusterName: cluster.Name, Namespace: cluster.Namespace})
		for _, machine := range controlPlaneMachines {
			if machine.Spec.Version != fromVersion {
				return false
			}
		}

		mds := framework.GetMachineDeploymentsByCluster(ctx,
			framework.GetMachineDeploymentsByClusterInput{ClusterName: cluster.Name, Namespace: cluster.Namespace, Lister: c})
		for _, md := range mds {
			if md.Spec.Template.Spec.Version != fromVersion {
				return false
			}
		}

		mps := framework.GetMachinePoolsByCluster(ctx,
			framework.GetMachinePoolsByClusterInput{ClusterName: cluster.Name, Namespace: cluster.Namespace, Lister: c})
		for _, mp := range mps {
			if mp.Spec.Template.Spec.Version != fromVersion {
				return false
			}
		}

		// Check if the BeforeControlPlaneUpgrade hook has been called (this should not happen when BeforeClusterUpgrade is blocking).
		// Note: BeforeClusterUpgrade hook is followed by BeforeControlPlaneUpgrade hook.
		if err := checkLifecycleHooksCalledAtLeastOnce(ctx, c, cluster, extensionConfigName, "BeforeControlPlaneUpgrade", []string{fromVersion, firstControlPlaneVersion}); err == nil {
			return false
		}

		return true
	}

	// BeforeClusterUpgrade can be blocked via an annotation hook. Check it works
	annotationHookTestHandler(ctx, c, cluster, hookName, beforeClusterUpgradeAnnotation, isBlockingUpgrade)

	// Test the BeforeClusterUpgrade hook.
	runtimeHookTestHandler(ctx, c, cluster, extensionConfigName, hookName, []string{fromVersion, toVersion}, isBlockingUpgrade, nil)

	Byf("BeforeClusterUpgrade from %s to %s unblocked", fromVersion, toVersion)
}

// beforeControlPlaneUpgradeTestHandler calls runtimeHookTestHandler with a blocking function which returns false if
// any of the control plane, machine deployments or machine pools has been updated from the initial Kubernetes version.
func beforeControlPlaneUpgradeTestHandler(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, extensionConfigName string, fromVersion, toVersion, workersVersion string) {
	hookName := "BeforeControlPlaneUpgrade"

	// for BeforeControlPlaneUpgrade, the hook is blocking if both controlPlane and workers remain at the expected version (both controlPlane and workers should be at fromVersion)
	isBlockingUpgrade := func() bool {
		controlPlane, err := external.GetObjectFromContractVersionedRef(ctx, c, cluster.Spec.ControlPlaneRef, cluster.Namespace)
		if err != nil {
			return false
		}
		controlPlaneVersion, err := contract.ControlPlane().Version().Get(controlPlane)
		if err != nil {
			return false
		}
		if *controlPlaneVersion != fromVersion {
			return false
		}

		controlPlaneMachines := framework.GetControlPlaneMachinesByCluster(ctx,
			framework.GetControlPlaneMachinesByClusterInput{Lister: c, ClusterName: cluster.Name, Namespace: cluster.Namespace})
		for _, machine := range controlPlaneMachines {
			if machine.Spec.Version != fromVersion {
				return false
			}
		}

		mds := framework.GetMachineDeploymentsByCluster(ctx,
			framework.GetMachineDeploymentsByClusterInput{ClusterName: cluster.Name, Namespace: cluster.Namespace, Lister: c})
		for _, md := range mds {
			if md.Spec.Template.Spec.Version != workersVersion {
				return false
			}
		}

		mps := framework.GetMachinePoolsByCluster(ctx,
			framework.GetMachinePoolsByClusterInput{ClusterName: cluster.Name, Namespace: cluster.Namespace, Lister: c})
		for _, mp := range mps {
			if mp.Spec.Template.Spec.Version != workersVersion {
				return false
			}
		}

		return true
	}

	runtimeHookTestHandler(ctx, c, cluster, extensionConfigName, hookName, []string{fromVersion, toVersion}, isBlockingUpgrade, nil)

	Byf("BeforeControlPlaneUpgrade from %s to %s unblocked", fromVersion, toVersion)
}

// afterControlPlaneUpgradeTestHandler calls runtimeHookTestHandler with a blocking function which returns false if any
// MachineDeployment in the Cluster has upgraded to the target Kubernetes version.
func afterControlPlaneUpgradeTestHandler(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, extensionConfigName string, controlPlaneVersion, workersVersion, nextControlPlaneVersion, topologyVersion string) {
	hookName := "AfterControlPlaneUpgrade"

	// for AfterControlPlaneUpgrade, the hook is blocking if both controlPlane and workers remain at the expected version (controlPlaneVersion, workersVersion)
	// and the BeforeWorkersUpgrade hook or the BeforeControlPlaneUpgrade is not called yet.
	isBlockingUpgrade := func() bool {
		controlPlane, err := external.GetObjectFromContractVersionedRef(ctx, c, cluster.Spec.ControlPlaneRef, cluster.Namespace)
		if err != nil {
			return false
		}
		v, err := contract.ControlPlane().Version().Get(controlPlane)
		if err != nil {
			return false
		}
		if *v != controlPlaneVersion {
			return false
		}

		controlPlaneMachines := framework.GetControlPlaneMachinesByCluster(ctx,
			framework.GetControlPlaneMachinesByClusterInput{Lister: c, ClusterName: cluster.Name, Namespace: cluster.Namespace})
		for _, machine := range controlPlaneMachines {
			if machine.Spec.Version != controlPlaneVersion {
				return false
			}
		}

		mds := framework.GetMachineDeploymentsByCluster(ctx,
			framework.GetMachineDeploymentsByClusterInput{ClusterName: cluster.Name, Namespace: cluster.Namespace, Lister: c})
		for _, md := range mds {
			if md.Spec.Template.Spec.Version != workersVersion {
				return false
			}
		}

		mps := framework.GetMachinePoolsByCluster(ctx,
			framework.GetMachinePoolsByClusterInput{ClusterName: cluster.Name, Namespace: cluster.Namespace, Lister: c})
		for _, mp := range mps {
			if mp.Spec.Template.Spec.Version != workersVersion {
				return false
			}
		}

		// Check if the BeforeWorkersUpgrade hook has been called (this should not happen when AfterControlPlaneUpgrade is blocking).
		// Note: AfterControlPlaneUpgrade hook can be followed by BeforeWorkersUpgrade hook in case also workers are performing this upgrade step.
		if err := checkLifecycleHooksCalledAtLeastOnce(ctx, c, cluster, extensionConfigName, "BeforeWorkersUpgrade", []string{workersVersion, controlPlaneVersion}); err == nil {
			return false
		}

		// Check if the BeforeControlPlaneUpgrade hook has been called (this should not happen when AfterControlPlaneUpgrade is blocking).
		// Note: AfterControlPlaneUpgrade hook can be followed by BeforeControlPlaneUpgrade hook in case there are still upgrade steps to perform and workers are skipping this minor.
		// Note: nextControlPlaneVersion != "" when there are still upgrade steps to perform.
		if nextControlPlaneVersion != "" {
			if err := checkLifecycleHooksCalledAtLeastOnce(ctx, c, cluster, extensionConfigName, "BeforeControlPlaneUpgrade", []string{controlPlaneVersion, nextControlPlaneVersion}); err == nil {
				return false
			}
		}

		// Check if the AfterClusterUpgrade hook has been called (this should not happen when AfterControlPlaneUpgrade is blocking).
		// Note: AfterControlPlaneUpgrade hook can be followed by AfterClusterUpgrade hook in case the upgrade is completed.
		// Note: nextControlPlaneVersion == "" in case the upgrade is completed.
		if nextControlPlaneVersion == "" {
			if err := checkLifecycleHooksCalledAtLeastOnce(ctx, c, cluster, extensionConfigName, "AfterClusterUpgrade", []string{topologyVersion}); err == nil {
				return false
			}
		}

		return true
	}

	beforeUnblockingUpgrade := func() {
		machineSetPreflightChecksTest(ctx, c, cluster)
	}

	// Test the AfterControlPlaneUpgrade hook and perform machine set preflight checks before unblocking.
	runtimeHookTestHandler(ctx, c, cluster, extensionConfigName, hookName, []string{controlPlaneVersion}, isBlockingUpgrade, beforeUnblockingUpgrade)

	Byf("AfterControlPlaneUpgrade to %s unblocked", controlPlaneVersion)
}

// beforeWorkersUpgradeTestHandler calls runtimeHookTestHandler with a blocking function which returns false if
// any of the control plane, machine deployments or machine pools has been updated from the initial Kubernetes version.
func beforeWorkersUpgradeTestHandler(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, extensionConfigName string, fromVersion, toVersion string) {
	hookName := "BeforeWorkersUpgrade"

	// for BeforeWorkersUpgrade, the hook is blocking if both controlPlane and workers remain at the expected version (controlPlaneVersion should be already at toVersion, workers should be at fromVersion)
	isBlockingUpgrade := func() bool {
		controlPlane, err := external.GetObjectFromContractVersionedRef(ctx, c, cluster.Spec.ControlPlaneRef, cluster.Namespace)
		if err != nil {
			return false
		}
		controlPlaneVersion, err := contract.ControlPlane().Version().Get(controlPlane)
		if err != nil {
			return false
		}
		if *controlPlaneVersion != toVersion {
			return false
		}

		controlPlaneMachines := framework.GetControlPlaneMachinesByCluster(ctx,
			framework.GetControlPlaneMachinesByClusterInput{Lister: c, ClusterName: cluster.Name, Namespace: cluster.Namespace})
		for _, machine := range controlPlaneMachines {
			if machine.Spec.Version != toVersion {
				return false
			}
		}

		mds := framework.GetMachineDeploymentsByCluster(ctx,
			framework.GetMachineDeploymentsByClusterInput{ClusterName: cluster.Name, Namespace: cluster.Namespace, Lister: c})
		for _, md := range mds {
			if md.Spec.Template.Spec.Version != fromVersion {
				return false
			}
		}

		mps := framework.GetMachinePoolsByCluster(ctx,
			framework.GetMachinePoolsByClusterInput{ClusterName: cluster.Name, Namespace: cluster.Namespace, Lister: c})
		for _, mp := range mps {
			if mp.Spec.Template.Spec.Version != fromVersion {
				return false
			}
		}

		return true
	}

	runtimeHookTestHandler(ctx, c, cluster, extensionConfigName, hookName, []string{fromVersion, toVersion}, isBlockingUpgrade, nil)

	Byf("BeforeWorkersUpgrade from %s to %s unblocked", fromVersion, toVersion)
}

// afterWorkersUpgradeTestHandler calls runtimeHookTestHandler with a blocking function which returns false if any
// MachineDeployment in the Cluster has upgraded to the target Kubernetes version.
func afterWorkersUpgradeTestHandler(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, extensionConfigName string, controlPlaneVersion, workersVersion, nextControlPlaneVersion, topologyVersion string) {
	hookName := "AfterWorkersUpgrade"

	// for AfterWorkersUpgrade, the hook is blocking if both controlPlane and workers remain at the expected version (controlPlaneVersion, workersVersion)
	// and the BeforeControlPlaneUpgrade hook or the AfterClusterUpgrade are not called yet.
	isBlockingUpgrade := func() bool {
		controlPlane, err := external.GetObjectFromContractVersionedRef(ctx, c, cluster.Spec.ControlPlaneRef, cluster.Namespace)
		if err != nil {
			return false
		}
		v, err := contract.ControlPlane().Version().Get(controlPlane)
		if err != nil {
			return false
		}
		if *v != controlPlaneVersion {
			return false
		}

		controlPlaneMachines := framework.GetControlPlaneMachinesByCluster(ctx,
			framework.GetControlPlaneMachinesByClusterInput{Lister: c, ClusterName: cluster.Name, Namespace: cluster.Namespace})
		for _, machine := range controlPlaneMachines {
			if machine.Spec.Version != controlPlaneVersion {
				return false
			}
		}

		mds := framework.GetMachineDeploymentsByCluster(ctx,
			framework.GetMachineDeploymentsByClusterInput{ClusterName: cluster.Name, Namespace: cluster.Namespace, Lister: c})
		for _, md := range mds {
			if md.Spec.Template.Spec.Version != workersVersion {
				return false
			}
		}

		mps := framework.GetMachinePoolsByCluster(ctx,
			framework.GetMachinePoolsByClusterInput{ClusterName: cluster.Name, Namespace: cluster.Namespace, Lister: c})
		for _, mp := range mps {
			if mp.Spec.Template.Spec.Version != workersVersion {
				return false
			}
		}

		// Check if the BeforeControlPlaneUpgrade hook has been called (this should not happen when AfterWorkersUpgrade is blocking).
		// Note: AfterWorkersUpgrade hook can be followed by BeforeControlPlaneUpgrade hook in case there are still upgrade steps to perform.
		// Note: nextControlPlaneVersion != "" when there are still upgrade steps to perform.
		if nextControlPlaneVersion != "" {
			if err := checkLifecycleHooksCalledAtLeastOnce(ctx, c, cluster, extensionConfigName, "BeforeControlPlaneUpgrade", []string{controlPlaneVersion, nextControlPlaneVersion}); err == nil {
				return false
			}
		}

		// Check if the AfterClusterUpgrade hook has been called (this should not happen when AfterWorkersUpgrade is blocking).
		// Note: AfterWorkersUpgrade hook can be followed by AfterClusterUpgrade hook in case the upgrade is completed.
		// Note: nextControlPlaneVersion == "" in case the upgrade is completed.
		if nextControlPlaneVersion == "" {
			if err := checkLifecycleHooksCalledAtLeastOnce(ctx, c, cluster, extensionConfigName, "AfterClusterUpgrade", []string{topologyVersion}); err == nil {
				return false
			}
		}

		return true
	}

	runtimeHookTestHandler(ctx, c, cluster, extensionConfigName, hookName, []string{workersVersion}, isBlockingUpgrade, nil)

	Byf("AfterWorkersUpgrade to %s unblocked", workersVersion)
}

// afterAfterClusterUpgradeTestHandler calls runtimeHookTestHandler with a blocking function which returns false
// if it is possible to upgrade to the next K8s version.
func afterAfterClusterUpgradeTestHandler(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, extensionConfigName string, topologyVersion string) {
	hookName := "AfterClusterUpgrade"

	v, err := semver.ParseTolerant(topologyVersion)
	Expect(err).ToNot(HaveOccurred())

	nextUpgradeCluster := cluster.DeepCopy()
	v.Minor++
	nextUpgradeCluster.Spec.Topology.Version = fmt.Sprintf("v%s", v.String())

	isBlockingUpgrade := func() bool {
		if err := c.Patch(ctx, nextUpgradeCluster, client.MergeFrom(cluster), client.DryRunAll); err != nil {
			if apierrors.IsInvalid(err) && strings.Contains(err.Error(), fmt.Sprintf("%q hook is still blocking", hookName)) {
				return true
			}
		}
		return false
	}

	runtimeHookTestHandler(ctx, c, cluster, extensionConfigName, hookName, []string{topologyVersion}, isBlockingUpgrade, nil)

	Byf("AfterClusterUpgrade to %s unblocked", topologyVersion)
}

// beforeClusterDeleteHandler calls runtimeHookTestHandler with a blocking function which returns false if the Cluster
// cannot be found in the API server.
func beforeClusterDeleteHandler(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, extensionConfigName string) {
	hookName := "BeforeClusterDelete"
	clusterObjectKey := client.ObjectKeyFromObject(cluster)

	// for BeforeClusterDelete, the hook is blocking if Get Cluster keep returning something different from IsNotFound error.
	isBlockingDelete := func() bool {
		var blocked = true

		// BeforeClusterDelete is unblocked either if the Cluster is gone or if OkToDeleteAnnotation is set.
		cluster := &clusterv1.Cluster{}
		if err := c.Get(ctx, clusterObjectKey, cluster); err != nil {
			if apierrors.IsNotFound(err) {
				blocked = false
			}
		} else if hooks.IsOkToDelete(cluster) {
			blocked = false
		}
		return blocked
	}

	runtimeHookTestHandler(ctx, c, cluster, extensionConfigName, hookName, nil, isBlockingDelete, nil)
}

// annotationHookTestHandler runs a series of tests in sequence to check if the annotation hook can block.
// 1. Check if the annotation hook is blocking and if the TopologyReconciled condition reports if the annotation hook is blocking.
// 2. Remove the annotation hook.
// 3. Check if the TopologyReconciled condition stops reporting the annotation hook is blocking.
func annotationHookTestHandler(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, hook, annotation string, blockingCondition func() bool) {
	log.Logf("Blocking with the %s annotation hook for 60 seconds", hook)

	expectedBlockingMessage := fmt.Sprintf("annotation %s is set", annotation)

	// Check if TopologyReconciledCondition reports if the annotation hook is blocking
	topologyConditionCheck := func() bool {
		cluster = framework.GetClusterByName(ctx, framework.GetClusterByNameInput{
			Name: cluster.Name, Namespace: cluster.Namespace, Getter: c})

		return strings.Contains(conditions.GetMessage(cluster, clusterv1.ClusterTopologyReconciledCondition), expectedBlockingMessage)
	}

	Byf("Waiting for %s hook (via annotation %s) to start blocking", hook, annotation)

	// Check if the annotation hook is blocking.
	Eventually(func(_ Gomega) bool {
		if !topologyConditionCheck() {
			return false
		}
		if !blockingCondition() {
			return false
		}
		return true
	}, 20*time.Second, 2*time.Second).Should(BeTrue(), "%s (via annotation %s) did not block", hook, annotation)

	Byf("Validating %s hook (via annotation %s) consistently blocks progress in the upgrade", hook, annotation)

	// Check if the annotation hook keeps blocking.
	Consistently(func(_ Gomega) bool {
		if !topologyConditionCheck() {
			return false
		}
		if !blockingCondition() {
			return false
		}
		return true
	}, 30*time.Second, 2*time.Second).Should(BeTrue(),
		fmt.Sprintf("Cluster Topology reconciliation continued unexpectedly: hook %s (via annotation %s) is not blocking", hook, annotation))

	// Patch the Cluster to remove the LifecycleHook annotation hook and unblock.
	Byf("Removing the %s annotation", annotation)

	patchHelper, err := patch.NewHelper(cluster, c)
	Expect(err).ToNot(HaveOccurred())
	delete(cluster.Annotations, annotation)
	Expect(patchHelper.Patch(ctx, cluster)).To(Succeed())

	// Expect the LifecycleHook annotation to not block anymore.
	// NOTE: we check only the topology reconciled message and not that blockingCondition because a runtime hook will block progress on reconciliation.

	Byf("Waiting for %s hook (via annotation %s) to stop blocking", hook, annotation)

	Eventually(func() error {
		cluster = framework.GetClusterByName(ctx, framework.GetClusterByNameInput{
			Name: cluster.Name, Namespace: cluster.Namespace, Getter: c})

		if strings.Contains(conditions.GetMessage(cluster, clusterv1.ClusterTopologyReconciledCondition), expectedBlockingMessage) {
			return fmt.Errorf("hook %s (via annotation %s) should not be blocking anymore with message: %s", hook, annotation, expectedBlockingMessage)
		}
		return nil
	}, 20*time.Second, 2*time.Second).Should(Succeed(),
		fmt.Sprintf("ClusterTopology reconcile did not proceed as expected when unblocking hook %s (via annotation %s)", hook, annotation))
}

// runtimeHookTestHandler runs a series of tests in sequence to check if the runtimeHook passed in has been called, and it can block.
//  1. Check if the hook is actually called, it is blocking, and if the TopologyReconciled condition reports the hook is blocking.
//  2. Remove the block.
//  3. Check that hook is not blocking anymore.
//
// Note: runtimeHookTestHandler assumes that the hook passed to it is currently returning a blocking response.
// Updating the response to be non-blocking happens inline in the function.
func runtimeHookTestHandler(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, extensionConfigName string, hook string, attributes []string, blockingCondition func() bool, beforeUnblocking func()) {
	hookName := computeHookName(hook, attributes)
	log.Logf("Blocking with the %s hook for 60 seconds", hookName)

	Byf("Waiting for %s hook to be called and start blocking", hookName)

	// Check if TopologyReconciledCondition reports if the hook is blocking
	topologyConditionCheck := func() bool {
		if hook != "BeforeClusterDelete" {
			cluster := framework.GetClusterByName(ctx, framework.GetClusterByNameInput{
				Name: cluster.Name, Namespace: cluster.Namespace, Getter: c})

			if !clusterConditionShowsHookBlocking(cluster, hook) {
				return false
			}
		}
		return true
	}

	// Check if the hook is actually called, it is blocking, and if the TopologyReconciled condition reports the hook is blocking.
	Eventually(func(_ Gomega) error {
		if err := checkLifecycleHooksCalledAtLeastOnce(ctx, c, cluster, extensionConfigName, hook, attributes); err != nil {
			return err
		}
		if !topologyConditionCheck() {
			return errors.Errorf("Blocking condition for %s not found on Cluster object", hookName)
		}
		return nil
	}, 30*time.Second, 2*time.Second).Should(Succeed(), "%s has not been called", hookName)

	Byf("Validating %s hook consistently blocks progress in the upgrade", hookName)

	// Check if the hook keeps blocking.
	Consistently(func(_ Gomega) bool {
		return topologyConditionCheck() && blockingCondition()
	}, 30*time.Second, 5*time.Second).Should(BeTrue(),
		fmt.Sprintf("Cluster Topology reconciliation continued unexpectedly: hook %s not blocking", hookName))

	if beforeUnblocking != nil {
		beforeUnblocking()
	}

	// Patch the ConfigMap to set the hook response to "Success".
	Byf("Setting %s response to Status:Success to unblock the upgrade", hookName)

	configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: hookResponsesConfigMapName(cluster.Name, extensionConfigName), Namespace: cluster.Namespace}}
	Eventually(func() error {
		return c.Get(ctx, util.ObjectKey(configMap), configMap)
	}).Should(Succeed(), "Failed to get ConfigMap %s", klog.KObj(configMap))
	patchData := client.RawPatch(types.MergePatchType,
		[]byte(fmt.Sprintf(`{"data":{"%s-preloadedResponse":%s}}`, hookName, "\"{\\\"Status\\\": \\\"Success\\\"}\"")))
	Eventually(func() error {
		return c.Patch(ctx, configMap, patchData)
	}).Should(Succeed(), "Failed to set %s response to Status:Success to unblock the upgrade", hookName)

	// Check if the hook stops blocking and if the TopologyReconciled condition stops reporting the hook is blocking.

	Byf("Waiting for %s hook to stop blocking", hookName)

	Eventually(func(_ Gomega) bool {
		if hook == "BeforeClusterDelete" {
			// Only check blockingCondition for BeforeClusterDelete, because topologyConditionCheck
			// always returns true for the BeforeClusterDelete hook.
			return blockingCondition()
		}
		return topologyConditionCheck() || blockingCondition()
	}, 30*time.Second, 2*time.Second).Should(BeFalse(),
		fmt.Sprintf("ClusterTopology reconcile did not proceed as expected when calling %s", hookName))
}

func computeHookName(hook string, attributes []string) string {
	// Note: + is not a valid character for ConfigMap keys (only alphanumeric characters, '-', '_' or '.')
	return strings.ReplaceAll(strings.Join(append([]string{hook}, attributes...), "-"), "+", "_")
}

// clusterConditionShowsHookBlocking checks if the TopologyReconciled condition message contains both the hook name and hookFailedMessage.
func clusterConditionShowsHookBlocking(cluster *clusterv1.Cluster, hookName string) bool {
	return strings.Contains(conditions.GetMessage(cluster, clusterv1.ClusterTopologyReconciledCondition), hookName)
}

func waitControlPlaneVersion(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, version string, intervals []interface{}) {
	Byf("Waiting for control plane to have version %s", version)

	Eventually(func(_ Gomega) bool {
		controlPlane, err := external.GetObjectFromContractVersionedRef(ctx, c, cluster.Spec.ControlPlaneRef, cluster.Namespace)
		if err != nil {
			return false
		}
		v, err := contract.ControlPlane().Version().Get(controlPlane)
		if err != nil {
			return false
		}
		if *v != version {
			return false
		}

		sv, err := contract.ControlPlane().StatusVersion().Get(controlPlane)
		if err != nil {
			return false
		}
		if *sv != version {
			return false
		}

		machineList := &clusterv1.MachineList{}
		if err := c.List(ctx, machineList, client.InNamespace(cluster.Namespace), client.MatchingLabels{
			clusterv1.ClusterNameLabel:         cluster.Name,
			clusterv1.MachineControlPlaneLabel: "",
		}); err != nil {
			return false
		}

		for i := range machineList.Items {
			machine := &machineList.Items[i]
			if machine.Spec.Version != version {
				return false
			}

			if !conditions.IsTrue(machine, clusterv1.MachineNodeHealthyCondition) {
				return false
			}
		}

		return true
	}, intervals...).Should(BeTrue(), fmt.Sprintf("Failed to wait for ControlPlane to reach version %s and Nodes to become healthy", version))
}

func waitWorkersVersions(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, workersVersion string, intervals []interface{}) {
	Byf("Waiting for workers to have version %s", workersVersion)
	workersVersions(ctx, c, cluster, workersVersion, intervals...)
}

func checkWorkersVersions(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, workersVersion string) {
	Byf("Checking workers have version %s", workersVersion)
	workersVersions(ctx, c, cluster, workersVersion, "10s", "2s")
}

func workersVersions(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, workersVersion string, intervals ...interface{}) {
	Eventually(func(_ Gomega) bool {
		mds := framework.GetMachineDeploymentsByCluster(ctx,
			framework.GetMachineDeploymentsByClusterInput{ClusterName: cluster.Name, Namespace: cluster.Namespace, Lister: c})
		for _, md := range mds {
			if md.Spec.Template.Spec.Version != workersVersion {
				return false
			}
		}

		mps := framework.GetMachinePoolsByCluster(ctx,
			framework.GetMachinePoolsByClusterInput{ClusterName: cluster.Name, Namespace: cluster.Namespace, Lister: c})
		for _, mp := range mps {
			if mp.Spec.Template.Spec.Version != workersVersion {
				return false
			}
		}

		machineList := &clusterv1.MachineList{}
		if err := c.List(ctx, machineList, client.InNamespace(cluster.Namespace), client.MatchingLabels{
			clusterv1.ClusterNameLabel: cluster.Name,
		}); err != nil {
			return false
		}

		for i := range machineList.Items {
			machine := &machineList.Items[i]
			if util.IsControlPlaneMachine(machine) {
				continue
			}

			if machine.Spec.Version != workersVersion {
				return false
			}

			if !conditions.IsTrue(machine, clusterv1.MachineNodeHealthyCondition) {
				return false
			}
		}
		return true
	}, intervals...).Should(BeTrue(), fmt.Sprintf("Failed to wait for workers to reach version %s and Nodes to become healthy", workersVersion))
}

func dumpAndDeleteCluster(ctx context.Context, proxy framework.ClusterProxy, clusterctlConfigPath, namespace, clusterName, artifactFolder string) {
	By("Deleting the workload cluster")

	cluster := framework.GetClusterByName(ctx, framework.GetClusterByNameInput{
		Name: clusterName, Namespace: namespace, Getter: proxy.GetClient()})

	// Dump all the logs from the workload cluster before deleting them.
	proxy.CollectWorkloadClusterLogs(ctx,
		cluster.Namespace,
		cluster.Name,
		filepath.Join(artifactFolder, "clusters-beforeClusterDelete", cluster.Name))

	// Dump all Cluster API related resources to artifacts before deleting them.
	framework.DumpAllResources(ctx, framework.DumpAllResourcesInput{
		Lister:               proxy.GetClient(),
		KubeConfigPath:       proxy.GetKubeconfigPath(),
		ClusterctlConfigPath: clusterctlConfigPath,
		Namespace:            namespace,
		LogPath:              filepath.Join(artifactFolder, "clusters-beforeClusterDelete", proxy.GetName(), "resources")})

	By("Deleting the workload cluster")
	framework.DeleteCluster(ctx, framework.DeleteClusterInput{
		Deleter: proxy.GetClient(),
		Cluster: cluster,
	})
}
