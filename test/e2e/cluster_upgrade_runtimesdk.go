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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	"sigs.k8s.io/cluster-api/test/e2e/internal/log"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
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

	// Allows to inject a function to be run after test namespace is created.
	// If not specified, this is a no-op.
	PostNamespaceCreated func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace string)

	// Allows to inject a function to be run after the cluster is upgraded.
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
// When using ClusterClass the ClusterClass must have the variables "etcdImageTag" and "coreDNSImageTag" of type string.
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

		Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersionUpgradeFrom))
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
		// to make this test more self-contained this test should be modified in order to create an additional
		// management cluster; also the E2E test configuration should be modified introducing something like
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
			extensionConfig(input.ExtensionConfigName, input.ExtensionServiceNamespace, input.ExtensionServiceName, true, namespaces...))).
			To(Succeed(), "Failed to create the extension config")

		By("Creating a workload cluster; creation waits for BeforeClusterCreateHook to gate the operation")

		clusterRef := types.NamespacedName{
			Name:      clusterName,
			Namespace: namespace.Name,
		}

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
				KubernetesVersion:        input.E2EConfig.MustGetVariable(KubernetesVersionUpgradeFrom),
				ControlPlaneMachineCount: ptr.To[int64](controlPlaneMachineCount),
				WorkerMachineCount:       ptr.To[int64](workerMachineCount),
				ClusterctlVariables:      variables,
			},
			PreWaitForCluster: func() {
				beforeClusterCreateTestHandler(ctx,
					input.BootstrapClusterProxy.GetClient(),
					clusterRef,
					input.ExtensionConfigName,
					input.E2EConfig.GetIntervals(specName, "wait-cluster"))
			},
			PostMachinesProvisioned: func() {
				Eventually(func() error {
					// Before running the BeforeClusterUpgrade hook, the topology controller
					// checks if the ControlPlane `IsScaling()` and for MachineDeployments if
					// `IsAnyRollingOut()`.
					// This PostMachineProvisioned function ensures that the clusters machines
					// are healthy by checking the MachineNodeHealthyCondition, so the upgrade
					// below does not get delayed or runs into timeouts before even reaching
					// the BeforeClusterUpgrade hook.
					machineList := &clusterv1.MachineList{}
					if err := input.BootstrapClusterProxy.GetClient().List(ctx, machineList, client.InNamespace(namespace.Name)); err != nil {
						return errors.Wrap(err, "list machines")
					}

					for i := range machineList.Items {
						machine := &machineList.Items[i]
						if !conditions.IsTrue(machine, clusterv1.MachineNodeHealthyCondition) {
							return errors.Errorf("machine %q does not have %q condition set to true", machine.GetName(), clusterv1.MachineNodeHealthyCondition)
						}
					}

					return nil
				}, 5*time.Minute, 15*time.Second).Should(Succeed(), "Waiting for rollouts to finish")
			},
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
			WaitForMachinePools:          input.E2EConfig.GetIntervals(specName, "wait-machine-pool-nodes"),
		}, clusterResources)

		// TODO: check if AfterControlPlaneInitialized has been called (or add this check to the operation above)

		// Add a BeforeClusterUpgrade hook annotation to block via the annotation.
		beforeClusterUpgradeAnnotation := clusterv1.BeforeClusterUpgradeHookAnnotationPrefix + "/upgrade-test"
		patchHelper, err := patch.NewHelper(clusterResources.Cluster, input.BootstrapClusterProxy.GetClient())
		Expect(err).ToNot(HaveOccurred())
		clusterResources.Cluster.Annotations[beforeClusterUpgradeAnnotation] = ""
		Expect(patchHelper.Patch(ctx, clusterResources.Cluster)).To(Succeed())

		// Upgrade the Cluster topology to run through an entire cluster lifecycle to test the lifecycle hooks.
		By("Upgrading the Cluster topology; creation waits for BeforeClusterUpgradeHook and AfterControlPlaneUpgradeHook to gate the operation")
		framework.UpgradeClusterTopologyAndWaitForUpgrade(ctx, framework.UpgradeClusterTopologyAndWaitForUpgradeInput{
			ClusterProxy:                   input.BootstrapClusterProxy,
			Cluster:                        clusterResources.Cluster,
			ControlPlane:                   clusterResources.ControlPlane,
			MachineDeployments:             clusterResources.MachineDeployments,
			MachinePools:                   clusterResources.MachinePools,
			KubernetesUpgradeVersion:       input.E2EConfig.MustGetVariable(KubernetesVersionUpgradeTo),
			WaitForMachinesToBeUpgraded:    input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			WaitForMachinePoolToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-machine-pool-upgrade"),
			WaitForKubeProxyUpgrade:        input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			WaitForDNSUpgrade:              input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			WaitForEtcdUpgrade:             input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			PreWaitForControlPlaneToBeUpgraded: func() {
				beforeClusterUpgradeAnnotationIsBlocking(ctx,
					input.BootstrapClusterProxy.GetClient(),
					clusterRef,
					input.E2EConfig.MustGetVariable(KubernetesVersionUpgradeTo),
					beforeClusterUpgradeAnnotation,
					input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"))

				beforeClusterUpgradeTestHandler(ctx,
					input.BootstrapClusterProxy.GetClient(),
					clusterRef,
					input.ExtensionConfigName,
					input.E2EConfig.MustGetVariable(KubernetesVersionUpgradeTo),
					input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"))
			},
			PreWaitForWorkersToBeUpgraded: func() {
				machineSetPreflightChecksTestHandler(ctx,
					input.BootstrapClusterProxy.GetClient(),
					clusterRef,
					input.ExtensionConfigName)

				afterControlPlaneUpgradeTestHandler(ctx,
					input.BootstrapClusterProxy.GetClient(),
					clusterRef,
					input.ExtensionConfigName,
					input.E2EConfig.MustGetVariable(KubernetesVersionUpgradeTo),
					input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"))
			},
		})

		By("Waiting until nodes are ready")
		workloadProxy := input.BootstrapClusterProxy.GetWorkloadCluster(ctx, namespace.Name, clusterResources.Cluster.Name)
		workloadClient := workloadProxy.GetClient()
		framework.WaitForNodesReady(ctx, framework.WaitForNodesReadyInput{
			Lister:            workloadClient,
			KubernetesVersion: input.E2EConfig.MustGetVariable(KubernetesVersionUpgradeTo),
			Count:             int(clusterResources.ExpectedTotalNodes()),
			WaitForNodesReady: input.E2EConfig.GetIntervals(specName, "wait-nodes-ready"),
		})

		if input.PostUpgrade != nil {
			log.Logf("Calling PostMachinesProvisioned for cluster %s", klog.KRef(namespace.Name, clusterResources.Cluster.Name))
			input.PostUpgrade(input.BootstrapClusterProxy, namespace.Name, clusterResources.Cluster.Name)
		}

		By("Dumping resources and deleting the workload cluster; deletion waits for BeforeClusterDeleteHook to gate the operation")
		dumpAndDeleteCluster(ctx, input.BootstrapClusterProxy, input.ClusterctlConfigPath, namespace.Name, clusterName, input.ArtifactFolder)

		beforeClusterDeleteHandler(ctx, input.BootstrapClusterProxy.GetClient(), clusterRef, input.ExtensionConfigName, input.E2EConfig.GetIntervals(specName, "wait-delete-cluster"))

		By("Checking all lifecycle hooks have been called")
		// Assert that each hook has been called and returned "Success" during the test.
		Expect(checkLifecycleHookResponses(ctx, input.BootstrapClusterProxy.GetClient(), clusterRef, input.ExtensionConfigName, map[string]string{
			"BeforeClusterCreate":          "Status: Success, RetryAfterSeconds: 0",
			"BeforeClusterUpgrade":         "Status: Success, RetryAfterSeconds: 0",
			"BeforeClusterDelete":          "Status: Success, RetryAfterSeconds: 0",
			"AfterControlPlaneUpgrade":     "Status: Success, RetryAfterSeconds: 0",
			"AfterControlPlaneInitialized": "Success",
			"AfterClusterUpgrade":          "Success",
		})).To(Succeed(), "Lifecycle hook calls were not as expected")

		By("PASSED!")
	})

	AfterEach(func() {
		// Dump all the resources in the spec namespace and the workload cluster.
		framework.DumpAllResourcesAndLogs(ctx, input.BootstrapClusterProxy, input.ClusterctlConfigPath, input.ArtifactFolder, namespace, clusterResources.Cluster)

		if !input.SkipCleanup {
			// Delete the extensionConfig first to ensure the BeforeDeleteCluster hook doesn't block deletion.
			Eventually(func() error {
				return input.BootstrapClusterProxy.GetClient().Delete(ctx, extensionConfig(input.ExtensionConfigName, input.ExtensionServiceNamespace, input.ExtensionServiceName, true, namespace.Name))
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

// machineSetPreflightChecksTestHandler verifies the MachineSet preflight checks.
// At this point in the test the ControlPlane is upgraded to the new version and the upgrade to the MachineDeployments
// should be blocked by the AfterControlPlaneUpgrade hook.
// Test the MachineSet preflight checks by scaling up the MachineDeployment. The creation on the new Machine
// should be blocked because the preflight checks should not pass (kubeadm version skew preflight check should fail).
func machineSetPreflightChecksTestHandler(ctx context.Context, c client.Client, clusterRef types.NamespacedName, extensionConfigName string) {
	// Verify that the hook is called and the topology reconciliation is blocked.
	hookName := "AfterControlPlaneUpgrade"
	Eventually(func() error {
		if err := checkLifecycleHooksCalledAtLeastOnce(ctx, c, clusterRef, extensionConfigName, []string{hookName}); err != nil {
			return err
		}

		cluster := framework.GetClusterByName(ctx, framework.GetClusterByNameInput{
			Name: clusterRef.Name, Namespace: clusterRef.Namespace, Getter: c})

		if !clusterConditionShowsHookBlocking(cluster, hookName) {
			return errors.Errorf("Blocking condition for %s not found on Cluster object", hookName)
		}

		return nil
	}, 30*time.Second).Should(Succeed(), "%s has not been called", hookName)

	// Scale up the MachineDeployment
	machineDeployments := framework.GetMachineDeploymentsByCluster(ctx, framework.GetMachineDeploymentsByClusterInput{
		Lister:      c,
		ClusterName: clusterRef.Name,
		Namespace:   clusterRef.Namespace,
	})
	md := machineDeployments[0]

	// Note: It is fair to assume that the Cluster is ClusterClass based since RuntimeSDK
	// is only supported for ClusterClass based Clusters.
	patchHelper, err := patch.NewHelper(md, c)
	Expect(err).ToNot(HaveOccurred())

	// Scale up the MachineDeployment.
	// IMPORTANT: Since the MachineDeployment is pending an upgrade at this point the topology controller will not push any changes
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

	// Since the MachineDeployment is scaled up (overriding the topology controller) at this point the MachineSet would
	// also scale up. However, a new Machine creation would be blocked by one of the MachineSet preflight checks (KubeadmVersionSkew).
	// Verify the MachineSet is blocking new Machine creation.
	Eventually(func(g Gomega) {
		machineSets := framework.GetMachineSetsByDeployment(ctx, framework.GetMachineSetsByDeploymentInput{
			Lister:    c,
			MDName:    md.Name,
			Namespace: md.Namespace,
		})
		g.Expect(conditions.IsFalse(machineSets[0], clusterv1.MachinesCreatedCondition)).To(BeTrue())
		machinesCreatedCondition := conditions.Get(machineSets[0], clusterv1.MachinesCreatedCondition)
		g.Expect(machinesCreatedCondition).NotTo(BeNil())
		g.Expect(machinesCreatedCondition.Reason).To(Equal(clusterv1.PreflightCheckFailedReason))
		g.Expect(machineSets[0].Spec.Replicas).To(Equal(md.Spec.Replicas))
	}).Should(Succeed(), "New Machine creation not blocked by MachineSet preflight checks")

	// Verify that the MachineSet is not creating the new Machine.
	// No new machines should be created for this MachineDeployment even though it is scaled up.
	// Creation of new Machines will be blocked by MachineSet preflight checks (KubeadmVersionSkew).
	Consistently(func(g Gomega) {
		originalReplicas := int(*md.Spec.Replicas - 1)
		machines := framework.GetMachinesByMachineDeployments(ctx, framework.GetMachinesByMachineDeploymentsInput{
			Lister:            c,
			ClusterName:       clusterRef.Name,
			Namespace:         clusterRef.Namespace,
			MachineDeployment: *md,
		})
		g.Expect(machines).To(HaveLen(originalReplicas), "New Machines should not be created")
	}, 10*time.Second, time.Second).Should(Succeed())

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
func extensionConfig(name, extensionServiceNamespace, extensionServiceName string, defaultAllHandlersToBlocking bool, namespaces ...string) *runtimev1.ExtensionConfig {
	cfg := &runtimev1.ExtensionConfig{
		ObjectMeta: metav1.ObjectMeta{
			// Note: We have to use a constant name here as we have to be able to reference it in the ClusterClass
			// when configuring external patches.
			Name: name,
			Annotations: map[string]string{
				// Note: this assumes the test extension get deployed in the default namespace defined in its own runtime-extensions-components.yaml
				runtimev1.InjectCAFromSecretAnnotation: fmt.Sprintf("%s/%s-cert", extensionServiceNamespace, extensionServiceName),
			},
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				Service: &runtimev1.ServiceReference{
					Name: extensionServiceName,
					// Note: this assumes the test extension get deployed in the default namespace defined in its own runtime-extensions-components.yaml
					Namespace: extensionServiceNamespace,
				},
			},
			Settings: map[string]string{
				"extensionConfigName":          name,
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
// If the provided hooks have both keys and values check that the values match those in the hook response configmap.
func checkLifecycleHookResponses(ctx context.Context, c client.Client, cluster types.NamespacedName, extensionConfigName string, expectedHookResponses map[string]string) error {
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
}

// Check that each hook in expectedHooks has been called at least once by checking if its actualResponseStatus is in the hook response configmap.
func checkLifecycleHooksCalledAtLeastOnce(ctx context.Context, c client.Client, cluster types.NamespacedName, extensionConfigName string, expectedHooks []string) error {
	responseData := getLifecycleHookResponsesFromConfigMap(ctx, c, cluster, extensionConfigName)
	for _, hookName := range expectedHooks {
		if _, ok := responseData[hookName+"-actualResponseStatus"]; !ok {
			return errors.Errorf("hook %s call not recorded in configMap %s", hookName, klog.KRef(cluster.Namespace, hookResponsesConfigMapName(cluster.Name, extensionConfigName)))
		}
	}
	return nil
}

func getLifecycleHookResponsesFromConfigMap(ctx context.Context, c client.Client, cluster types.NamespacedName, extensionConfigName string) map[string]string {
	configMap := &corev1.ConfigMap{}
	Eventually(func() error {
		return c.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: hookResponsesConfigMapName(cluster.Name, extensionConfigName)}, configMap)
	}).Should(Succeed(), "Failed to get the hook response configmap")
	return configMap.Data
}

// beforeClusterCreateTestHandler calls runtimeHookTestHandler with a blockedCondition function which returns false if
// the Cluster has entered ClusterPhaseProvisioned.
func beforeClusterCreateTestHandler(ctx context.Context, c client.Client, cluster types.NamespacedName, extensionConfigName string, intervals []interface{}) {
	hookName := "BeforeClusterCreate"
	runtimeHookTestHandler(ctx, c, cluster, hookName, extensionConfigName, true, func() bool {
		blocked := true
		// This hook should block the Cluster from entering the "Provisioned" state.
		cluster := framework.GetClusterByName(ctx,
			framework.GetClusterByNameInput{Name: cluster.Name, Namespace: cluster.Namespace, Getter: c})

		if cluster.Status.Phase == string(clusterv1.ClusterPhaseProvisioned) {
			blocked = false
		}
		return blocked
	}, intervals)
}

// beforeClusterUpgradeAnnotationIsBlocking checks if the cluster is successfully blocking due to the given BeforeClusterUpgrade
// hook annotation by checking for the right condition message and that none of the machines in the control plane has been
// updated to the target Kubernetes version.
func beforeClusterUpgradeAnnotationIsBlocking(ctx context.Context, c client.Client, clusterRef types.NamespacedName, toVersion, annotation string, intervals []interface{}) {
	hookName := "BeforeClusterUpgrade"
	log.Logf("Blocking with %s hook for 60 seconds with the annotation", hookName)

	expectedBlockingMessage := fmt.Sprintf("hook %q is blocking: annotation [%s] is set", hookName, annotation)

	blockingConditionCheck := func() error {
		cluster := framework.GetClusterByName(ctx, framework.GetClusterByNameInput{
			Name: clusterRef.Name, Namespace: clusterRef.Namespace, Getter: c})

		if conditions.GetReason(cluster, clusterv1.TopologyReconciledCondition) != clusterv1.TopologyReconciledHookBlockingReason {
			return fmt.Errorf("hook %s (via annotation) should lead to LifecycleHookBlocking reason", hookName)
		}
		if !strings.Contains(conditions.GetMessage(cluster, clusterv1.TopologyReconciledCondition), expectedBlockingMessage) {
			return fmt.Errorf("hook %[1]s (via annotation) should show hook %[1]s is blocking as message with: %[2]s", hookName, expectedBlockingMessage)
		}

		controlPlaneMachines := framework.GetControlPlaneMachinesByCluster(ctx,
			framework.GetControlPlaneMachinesByClusterInput{Lister: c, ClusterName: clusterRef.Name, Namespace: clusterRef.Namespace})
		for _, machine := range controlPlaneMachines {
			if *machine.Spec.Version == toVersion {
				return errors.Errorf("Machine's %s version (%s) does match %s", klog.KObj(&machine), *machine.Spec.Version, toVersion)
			}
		}

		return nil
	}

	// Check that the LifecycleHook annotation is blocking at least once with the expected blocking reason, message and none of the CP machines being upgraded.
	Eventually(blockingConditionCheck, 30*time.Second).Should(Succeed(), "%s (via annotation %s) did not block", hookName, annotation)

	// The check  should consistently succeed.
	Consistently(blockingConditionCheck, 60*time.Second).Should(Succeed(),
		fmt.Sprintf("Cluster Topology reconciliation continued unexpectedly: hook %s (via annotation %s) is not blocking", hookName, annotation))

	// Patch the Cluster to remove the LifecycleHook annotation hook and unblock.
	cluster := framework.GetClusterByName(ctx, framework.GetClusterByNameInput{
		Name: clusterRef.Name, Namespace: clusterRef.Namespace, Getter: c})
	patchHelper, err := patch.NewHelper(cluster, c)
	Expect(err).ToNot(HaveOccurred())
	delete(cluster.Annotations, annotation)
	Expect(patchHelper.Patch(ctx, cluster)).To(Succeed())

	// Expect the LifecycleHook annotation to not block anymore.
	Eventually(func() error {
		cluster := framework.GetClusterByName(ctx, framework.GetClusterByNameInput{
			Name: clusterRef.Name, Namespace: clusterRef.Namespace, Getter: c})

		if strings.Contains(conditions.GetMessage(cluster, clusterv1.TopologyReconciledCondition), expectedBlockingMessage) {
			return fmt.Errorf("hook %s (via annotation %s) should not be blocking anymore with message: %s", hookName, annotation, expectedBlockingMessage)
		}

		return nil
	}, intervals...).Should(Succeed(),
		fmt.Sprintf("ClusterTopology reconcile did not proceed as expected when unblocking hook %s (via annotation %s)", hookName, annotation))
}

// beforeClusterUpgradeTestHandler calls runtimeHookTestHandler with a blocking function which returns false if
// any of the machines in the control plane has been updated to the target Kubernetes version.
func beforeClusterUpgradeTestHandler(ctx context.Context, c client.Client, cluster types.NamespacedName, extensionConfigName string, toVersion string, intervals []interface{}) {
	hookName := "BeforeClusterUpgrade"
	runtimeHookTestHandler(ctx, c, cluster, hookName, extensionConfigName, true, func() bool {
		var blocked = true

		controlPlaneMachines := framework.GetControlPlaneMachinesByCluster(ctx,
			framework.GetControlPlaneMachinesByClusterInput{Lister: c, ClusterName: cluster.Name, Namespace: cluster.Namespace})
		for _, machine := range controlPlaneMachines {
			if *machine.Spec.Version == toVersion {
				blocked = false
			}
		}
		return blocked
	}, intervals)
}

// afterControlPlaneUpgradeTestHandler calls runtimeHookTestHandler with a blocking function which returns false if any
// MachineDeployment in the Cluster has upgraded to the target Kubernetes version.
func afterControlPlaneUpgradeTestHandler(ctx context.Context, c client.Client, cluster types.NamespacedName, extensionConfigName string, version string, intervals []interface{}) {
	hookName := "AfterControlPlaneUpgrade"
	runtimeHookTestHandler(ctx, c, cluster, hookName, extensionConfigName, true, func() bool {
		var blocked = true

		mds := framework.GetMachineDeploymentsByCluster(ctx,
			framework.GetMachineDeploymentsByClusterInput{ClusterName: cluster.Name, Namespace: cluster.Namespace, Lister: c})
		// If any of the MachineDeployments have the target Kubernetes Version, the hook is unblocked.
		for _, md := range mds {
			if *md.Spec.Template.Spec.Version == version {
				blocked = false
			}
		}
		return blocked
	}, intervals)
}

// beforeClusterDeleteHandler calls runtimeHookTestHandler with a blocking function which returns false if the Cluster
// can not be found in the API server.
func beforeClusterDeleteHandler(ctx context.Context, c client.Client, cluster types.NamespacedName, extensionConfigName string, intervals []interface{}) {
	hookName := "BeforeClusterDelete"
	runtimeHookTestHandler(ctx, c, cluster, hookName, extensionConfigName, false, func() bool {
		var blocked = true

		// If the Cluster is not found it has been deleted and the hook is unblocked.
		if apierrors.IsNotFound(c.Get(ctx, client.ObjectKey{Name: cluster.Name, Namespace: cluster.Namespace}, &clusterv1.Cluster{})) {
			blocked = false
		}
		return blocked
	}, intervals)
}

// runtimeHookTestHandler runs a series of tests in sequence to check if the runtimeHook passed to it succeeds.
//  1. Checks that the hook has been called at least once and, if withTopologyReconciledCondition is set, checks that the TopologyReconciled condition is a Failure.
//  2. Check that the hook's blockingCondition is consistently true.
//     - At this point the function sets the hook's response to be non-blocking.
//  3. Check that the hook's blocking condition becomes false.
//
// Note: runtimeHookTestHandler assumes that the hook passed to it is currently returning a blocking response.
// Updating the response to be non-blocking happens inline in the function.
func runtimeHookTestHandler(ctx context.Context, c client.Client, cluster types.NamespacedName, hookName, extensionConfigName string, withTopologyReconciledCondition bool, blockingCondition func() bool, intervals []interface{}) {
	log.Logf("Blocking with %s hook for 60 seconds after the hook has been called for the first time", hookName)

	// Check that the LifecycleHook has been called at least once and - when required - that the TopologyReconciled condition is a Failure.
	Eventually(func() error {
		if err := checkLifecycleHooksCalledAtLeastOnce(ctx, c, cluster, extensionConfigName, []string{hookName}); err != nil {
			return err
		}

		// Check for the existence of the condition if withTopologyReconciledCondition is true.
		if withTopologyReconciledCondition {
			cluster := framework.GetClusterByName(ctx, framework.GetClusterByNameInput{
				Name: cluster.Name, Namespace: cluster.Namespace, Getter: c})

			if !clusterConditionShowsHookBlocking(cluster, hookName) {
				return errors.Errorf("Blocking condition for %s not found on Cluster object", hookName)
			}
		}
		return nil
	}, 30*time.Second).Should(Succeed(), "%s has not been called", hookName)

	// blockingCondition should consistently be true as the Runtime hook is returning "Failure".
	Consistently(func() bool {
		return blockingCondition()
	}, 60*time.Second).Should(BeTrue(),
		fmt.Sprintf("Cluster Topology reconciliation continued unexpectedly: hook %s not blocking", hookName))

	// Patch the ConfigMap to set the hook response to "Success".
	Byf("Setting %s response to Status:Success to unblock the reconciliation", hookName)

	configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: hookResponsesConfigMapName(cluster.Name, extensionConfigName), Namespace: cluster.Namespace}}
	Eventually(func() error {
		return c.Get(ctx, util.ObjectKey(configMap), configMap)
	}).Should(Succeed(), "Failed to get ConfigMap %s", klog.KObj(configMap))
	patch := client.RawPatch(types.MergePatchType,
		[]byte(fmt.Sprintf(`{"data":{"%s-preloadedResponse":%s}}`, hookName, "\"{\\\"Status\\\": \\\"Success\\\"}\"")))
	Eventually(func() error {
		return c.Patch(ctx, configMap, patch)
	}).Should(Succeed(), "Failed to set %s response to Status:Success to unblock the reconciliation", hookName)

	// Expect the Hook to pass, setting the blockingCondition to false before the timeout ends.
	Eventually(func() bool {
		return blockingCondition()
	}, intervals...).Should(BeFalse(),
		fmt.Sprintf("ClusterTopology reconcile did not proceed as expected when calling %s", hookName))
}

// clusterConditionShowsHookBlocking checks if the TopologyReconciled condition message contains both the hook name and hookFailedMessage.
func clusterConditionShowsHookBlocking(cluster *clusterv1.Cluster, hookName string) bool {
	return conditions.GetReason(cluster, clusterv1.TopologyReconciledCondition) == clusterv1.TopologyReconciledHookBlockingReason &&
		strings.Contains(conditions.GetMessage(cluster, clusterv1.TopologyReconciledCondition), hookName)
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
