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
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	"sigs.k8s.io/cluster-api/test/e2e/internal/log"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
)

// clusterUpgradeWithRuntimeSDKSpecInput is the input for clusterUpgradeWithRuntimeSDKSpec.
type clusterUpgradeWithRuntimeSDKSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool

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
}

// clusterUpgradeWithRuntimeSDKSpec implements a spec that upgrades a cluster and runs the Kubernetes conformance suite.
// Upgrading a cluster refers to upgrading the control-plane and worker nodes (managed by MD and machine pools).
// NOTE: This test only works with a KubeadmControlPlane.
// NOTE: This test works with Clusters with and without ClusterClass.
// When using ClusterClass the ClusterClass must have the variables "etcdImageTag" and "coreDNSImageTag" of type string.
// Those variables should have corresponding patches which set the etcd and CoreDNS tags in KCP.
func clusterUpgradeWithRuntimeSDKSpec(ctx context.Context, inputGetter func() clusterUpgradeWithRuntimeSDKSpecInput) {
	const (
		textExtensionPathVariable = "TEST_EXTENSION"
		specName                  = "k8s-upgrade-with-runtimesdk"
	)

	var (
		input         clusterUpgradeWithRuntimeSDKSpecInput
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc

		controlPlaneMachineCount int64
		workerMachineCount       int64

		clusterResources  *clusterctl.ApplyClusterTemplateAndWaitResult
		testExtensionPath string
		clusterName       string
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
		Expect(input.E2EConfig.Variables).To(HaveKey(EtcdVersionUpgradeTo))
		Expect(input.E2EConfig.Variables).To(HaveKey(CoreDNSVersionUpgradeTo))

		testExtensionPath = input.E2EConfig.GetVariable(textExtensionPathVariable)
		Expect(testExtensionPath).To(BeAnExistingFile(), "The %s variable should resolve to an existing file", textExtensionPathVariable)

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
		namespace, cancelWatches = setupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder)
		clusterName = fmt.Sprintf("%s-%s", specName, util.RandomString(6))

		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should create, upgrade and delete a workload cluster", func() {
		By("Deploy Test Extension")
		testExtensionDeploymentTemplate, err := os.ReadFile(testExtensionPath) //nolint:gosec
		Expect(err).ToNot(HaveOccurred(), "Failed to read the extension deployment manifest file")

		// Set the SERVICE_NAMESPACE, which is used in the cert-manager Certificate CR.
		// We have to dynamically set the namespace here, because it depends on the test run and thus
		// cannot be set when rendering the test extension YAML with kustomize.
		testExtensionDeployment := strings.ReplaceAll(string(testExtensionDeploymentTemplate), "${SERVICE_NAMESPACE}", namespace.Name)

		Expect(testExtensionDeployment).ToNot(BeEmpty(), "Test Extension deployment manifest file should not be empty")
		Expect(input.BootstrapClusterProxy.Apply(ctx, []byte(testExtensionDeployment), "--namespace", namespace.Name)).To(Succeed())

		By("Deploy Test Extension ExtensionConfig and ConfigMap")

		Expect(input.BootstrapClusterProxy.GetClient().Create(ctx,
			extensionConfig(specName, namespace))).
			To(Succeed(), "Failed to create the extension config")

		Expect(input.BootstrapClusterProxy.GetClient().Create(ctx,
			responsesConfigMap(clusterName, namespace))).
			To(Succeed(), "Failed to create the responses configMap")

		By("Wait for test extension deployment to be availabel")
		framework.WaitForDeploymentsAvailable(ctx, framework.WaitForDeploymentsAvailableInput{
			Getter:     input.BootstrapClusterProxy.GetClient(),
			Deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "test-extension", Namespace: namespace.Name}},
		})

		By("Watch Deployment logs of test extension")
		framework.WatchDeploymentLogs(ctx, framework.WatchDeploymentLogsInput{
			GetLister:  input.BootstrapClusterProxy.GetClient(),
			ClientSet:  input.BootstrapClusterProxy.GetClientSet(),
			Deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "test-extension", Namespace: namespace.Name}},
			LogPath:    filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName(), "logs", namespace.Name),
		})

		By("Creating a workload cluster")

		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   clusterctl.DefaultInfrastructureProvider,
				Flavor:                   pointer.StringDeref(input.Flavor, "upgrades"),
				Namespace:                namespace.Name,
				ClusterName:              clusterName,
				KubernetesVersion:        input.E2EConfig.GetVariable(KubernetesVersionUpgradeFrom),
				ControlPlaneMachineCount: pointer.Int64Ptr(controlPlaneMachineCount),
				WorkerMachineCount:       pointer.Int64Ptr(workerMachineCount),
			},
			PreWaitForCluster: func() {
				beforeClusterCreateTestHandler(ctx,
					input.BootstrapClusterProxy.GetClient(),
					namespace.Name, clusterName,
					input.E2EConfig.GetIntervals(specName, "wait-cluster"))
			},
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
			WaitForMachinePools:          input.E2EConfig.GetIntervals(specName, "wait-machine-pool-nodes"),
		}, clusterResources)

		// Upgrade the Cluster topology to run through an entire cluster lifecycle to test the lifecycle hooks.
		By("Upgrading the Cluster topology")
		framework.UpgradeClusterTopologyAndWaitForUpgrade(ctx, framework.UpgradeClusterTopologyAndWaitForUpgradeInput{
			ClusterProxy:                input.BootstrapClusterProxy,
			Cluster:                     clusterResources.Cluster,
			ControlPlane:                clusterResources.ControlPlane,
			MachineDeployments:          clusterResources.MachineDeployments,
			KubernetesUpgradeVersion:    input.E2EConfig.GetVariable(KubernetesVersionUpgradeTo),
			WaitForMachinesToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			WaitForKubeProxyUpgrade:     input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			WaitForDNSUpgrade:           input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			WaitForEtcdUpgrade:          input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			PreWaitForControlPlaneToBeUpgraded: func() {
				beforeClusterUpgradeTestHandler(ctx,
					input.BootstrapClusterProxy.GetClient(),
					namespace.Name,
					clusterName,
					input.E2EConfig.GetVariable(KubernetesVersionUpgradeTo),
					input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"))
			},
			PreWaitForMachineDeploymentToBeUpgraded: func() {
				afterControlPlaneUpgradeTestHandler(ctx,
					input.BootstrapClusterProxy.GetClient(),
					namespace.Name,
					clusterName,
					input.E2EConfig.GetVariable(KubernetesVersionUpgradeTo),
					input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"))
			},
		})

		// Only attempt to upgrade MachinePools if they were provided in the template.
		if len(clusterResources.MachinePools) > 0 && workerMachineCount > 0 {
			By("Upgrading the machinepool instances")
			framework.UpgradeMachinePoolAndWait(ctx, framework.UpgradeMachinePoolAndWaitInput{
				ClusterProxy:                   input.BootstrapClusterProxy,
				Cluster:                        clusterResources.Cluster,
				UpgradeVersion:                 input.E2EConfig.GetVariable(KubernetesVersionUpgradeTo),
				WaitForMachinePoolToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-machine-pool-upgrade"),
				MachinePools:                   clusterResources.MachinePools,
			})
		}

		By("Waiting until nodes are ready")
		workloadProxy := input.BootstrapClusterProxy.GetWorkloadCluster(ctx, namespace.Name, clusterResources.Cluster.Name)
		workloadClient := workloadProxy.GetClient()
		framework.WaitForNodesReady(ctx, framework.WaitForNodesReadyInput{
			Lister:            workloadClient,
			KubernetesVersion: input.E2EConfig.GetVariable(KubernetesVersionUpgradeTo),
			Count:             int(clusterResources.ExpectedTotalNodes()),
			WaitForNodesReady: input.E2EConfig.GetIntervals(specName, "wait-nodes-ready"),
		})

		By("Dumping resources and deleting the workload cluster")
		dumpAndDeleteCluster(ctx, input.BootstrapClusterProxy, namespace.Name, clusterName, input.ArtifactFolder)

		beforeClusterDeleteHandler(ctx, input.BootstrapClusterProxy.GetClient(), namespace.Name, clusterName, input.E2EConfig.GetIntervals(specName, "wait-delete-cluster"))

		By("Checking all lifecycle hooks have been called")
		// Assert that each hook has been called and returned "Success" during the test.
		Expect(checkLifecycleHookResponses(ctx, input.BootstrapClusterProxy.GetClient(), namespace.Name, clusterName, map[string]string{
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
		// Delete the extensionConfig first to ensure the BeforeDeleteCluster hook doesn't block deletion.
		Eventually(func() error {
			return input.BootstrapClusterProxy.GetClient().Delete(ctx, extensionConfig(specName, namespace))
		}, 10*time.Second, 1*time.Second).Should(Succeed(), "delete extensionConfig failed")

		// Dumps all the resources in the spec Namespace, then cleanups the cluster object and the spec Namespace itself.
		dumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}

// extensionConfig generates an ExtensionConfig.
// We make sure this cluster-wide object does not conflict with others by using a random generated
// name and a NamespaceSelector selecting on the namespace of the current test.
// Thus, this object is "namespaced" to the current test even though it's a cluster-wide object.
func extensionConfig(specName string, namespace *corev1.Namespace) *runtimev1.ExtensionConfig {
	return &runtimev1.ExtensionConfig{
		ObjectMeta: metav1.ObjectMeta{
			// FIXME(sbueringer): use constant name for now as we have to be able to reference it in the ClusterClass.
			// Random generate later on again when Yuvaraj's PR has split up the cluster lifecycle.
			//Name: fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
			Name: specName,
			Annotations: map[string]string{
				runtimev1.InjectCAFromSecretAnnotation: fmt.Sprintf("%s/webhook-service-cert", namespace.Name),
			},
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				Service: &runtimev1.ServiceReference{
					Name:      "webhook-service",
					Namespace: namespace.Name,
				},
			},
			NamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "kubernetes.io/metadata.name",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{namespace.Name},
					},
				},
			},
		},
	}
}

// responsesConfigMap generates a ConfigMap with preloaded responses for the test extension.
func responsesConfigMap(name string, namespace *corev1.Namespace) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-hookresponses", name),
			Namespace: namespace.Name,
		},
		// Set the initial preloadedResponses for each of the tested hooks.
		Data: map[string]string{
			// Blocking hooks are set to return RetryAfterSeconds initially. These will be changed during the test.
			"BeforeClusterCreate-preloadedResponse":      `{"Status": "Success", "RetryAfterSeconds": 5}`,
			"BeforeClusterUpgrade-preloadedResponse":     `{"Status": "Success", "RetryAfterSeconds": 5}`,
			"AfterControlPlaneUpgrade-preloadedResponse": `{"Status": "Success", "RetryAfterSeconds": 5}`,
			"BeforeClusterDelete-preloadedResponse":      `{"Status": "Success", "RetryAfterSeconds": 5}`,

			// Non-blocking hooks are set to Status:Success.
			"AfterControlPlaneInitialized-preloadedResponse": `{"Status": "Success"}`,
			"AfterClusterUpgrade-preloadedResponse":          `{"Status": "Success"}`,
		},
	}
}

// Check that each hook in hooks has been called at least once by checking if its actualResponseStatus is in the hook response configmap.
// If the provided hooks have both keys and values check that the values match those in the hook response configmap.
func checkLifecycleHookResponses(ctx context.Context, c client.Client, namespace string, clusterName string, expectedHookResponses map[string]string) error {
	responseData := getLifecycleHookResponsesFromConfigMap(ctx, c, namespace, clusterName)
	for hookName, expectedResponse := range expectedHookResponses {
		actualResponse, ok := responseData[hookName+"-actualResponseStatus"]
		if !ok {
			return errors.Errorf("hook %s call not recorded in configMap %s", hookName, klog.KRef(namespace, clusterName+"-hookresponses"))
		}
		if expectedResponse != "" && expectedResponse != actualResponse {
			return errors.Errorf("hook %s was expected to be %s in configMap got %s", hookName, expectedResponse, actualResponse)
		}
	}
	return nil
}

// Check that each hook in expectedHooks has been called at least once by checking if its actualResponseStatus is in the hook response configmap.
func checkLifecycleHooksCalledAtLeastOnce(ctx context.Context, c client.Client, namespace string, clusterName string, expectedHooks []string) error {
	responseData := getLifecycleHookResponsesFromConfigMap(ctx, c, namespace, clusterName)
	for _, hookName := range expectedHooks {
		if _, ok := responseData[hookName+"-actualResponseStatus"]; !ok {
			return errors.Errorf("hook %s call not recorded in configMap %s", hookName, klog.KRef(namespace, clusterName+"-hookresponses"))
		}
	}
	return nil
}

func getLifecycleHookResponsesFromConfigMap(ctx context.Context, c client.Client, namespace string, clusterName string) map[string]string {
	configMap := &corev1.ConfigMap{}
	configMapName := clusterName + "-hookresponses"
	Eventually(func() error {
		return c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: configMapName}, configMap)
	}).Should(Succeed(), "Failed to get the hook response configmap")
	return configMap.Data
}

// beforeClusterCreateTestHandler calls runtimeHookTestHandler with a blockedCondition function which returns false if
// the Cluster has entered ClusterPhaseProvisioned.
func beforeClusterCreateTestHandler(ctx context.Context, c client.Client, namespace, clusterName string, intervals []interface{}) {
	hookName := "BeforeClusterCreate"
	runtimeHookTestHandler(ctx, c, namespace, clusterName, hookName, true, func() bool {
		blocked := true
		// This hook should block the Cluster from entering the "Provisioned" state.
		cluster := framework.GetClusterByName(ctx,
			framework.GetClusterByNameInput{Name: clusterName, Namespace: namespace, Getter: c})

		if cluster.Status.Phase == string(clusterv1.ClusterPhaseProvisioned) {
			blocked = false
		}
		return blocked
	}, intervals)
}

// beforeClusterUpgradeTestHandler calls runtimeHookTestHandler with a blocking function which returns false if
// any of the machines in the control plane has been updated to the target Kubernetes version.
func beforeClusterUpgradeTestHandler(ctx context.Context, c client.Client, namespace, clusterName, toVersion string, intervals []interface{}) {
	hookName := "BeforeClusterUpgrade"
	runtimeHookTestHandler(ctx, c, namespace, clusterName, hookName, true, func() bool {
		var blocked = true

		controlPlaneMachines := framework.GetControlPlaneMachinesByCluster(ctx,
			framework.GetControlPlaneMachinesByClusterInput{Lister: c, ClusterName: clusterName, Namespace: namespace})
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
func afterControlPlaneUpgradeTestHandler(ctx context.Context, c client.Client, namespace, clusterName, version string, intervals []interface{}) {
	hookName := "AfterControlPlaneUpgrade"
	runtimeHookTestHandler(ctx, c, namespace, clusterName, hookName, true, func() bool {
		var blocked = true

		mds := framework.GetMachineDeploymentsByCluster(ctx,
			framework.GetMachineDeploymentsByClusterInput{ClusterName: clusterName, Namespace: namespace, Lister: c})
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
func beforeClusterDeleteHandler(ctx context.Context, c client.Client, namespace, clusterName string, intervals []interface{}) {
	hookName := "BeforeClusterDelete"
	runtimeHookTestHandler(ctx, c, namespace, clusterName, hookName, false, func() bool {
		var blocked = true

		// If the Cluster is not found it has been deleted and the hook is unblocked.
		if apierrors.IsNotFound(c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: clusterName}, &clusterv1.Cluster{})) {
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
func runtimeHookTestHandler(ctx context.Context, c client.Client, namespace, clusterName, hookName string, withTopologyReconciledCondition bool, blockingCondition func() bool, intervals []interface{}) {
	log.Logf("Blocking with %s hook", hookName)

	// Check that the LifecycleHook has been called at least once and - when required - that the TopologyReconciled condition is a Failure.
	Eventually(func() error {
		if err := checkLifecycleHooksCalledAtLeastOnce(ctx, c, namespace, clusterName, []string{hookName}); err != nil {
			return err
		}

		// Check for the existence of the condition if withTopologyReconciledCondition is true.
		if withTopologyReconciledCondition {
			cluster := framework.GetClusterByName(ctx, framework.GetClusterByNameInput{
				Name: clusterName, Namespace: namespace, Getter: c})

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

	configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: clusterName + "-hookresponses", Namespace: namespace}}
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
		fmt.Sprintf("ClusterTopology reconcile did proceed as expected when calling %s", hookName))
}

// clusterConditionShowsHookBlocking checks if the TopologyReconciled condition message contains both the hook name and hookFailedMessage.
func clusterConditionShowsHookBlocking(cluster *clusterv1.Cluster, hookName string) bool {
	return conditions.GetReason(cluster, clusterv1.TopologyReconciledCondition) == clusterv1.TopologyReconciledHookBlockingReason &&
		strings.Contains(conditions.GetMessage(cluster, clusterv1.TopologyReconciledCondition), hookName)
}

func dumpAndDeleteCluster(ctx context.Context, proxy framework.ClusterProxy, namespace, clusterName, artifactFolder string) {
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
		Lister:    proxy.GetClient(),
		Namespace: namespace,
		LogPath:   filepath.Join(artifactFolder, "clusters-beforeClusterDelete", proxy.GetName(), "resources")})

	By("Deleting the workload cluster")
	framework.DeleteCluster(ctx, framework.DeleteClusterInput{
		Deleter: proxy.GetClient(),
		Cluster: cluster,
	})
}
