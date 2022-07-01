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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	"sigs.k8s.io/cluster-api/test/e2e/internal/log"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
)

var hookFailedMessage = "hook failed"

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
		ext           *runtimev1.ExtensionConfig
		cancelWatches context.CancelFunc

		controlPlaneMachineCount int64
		workerMachineCount       int64

		clusterResources  *clusterctl.ApplyClusterTemplateAndWaitResult
		testExtensionPath string
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
		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should create and upgrade a workload cluster", func() {
		clusterName := fmt.Sprintf("%s-%s", specName, util.RandomString(6))
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
		ext = extensionConfig(specName, namespace)
		err = input.BootstrapClusterProxy.GetClient().Create(ctx, ext)
		Expect(err).ToNot(HaveOccurred(), "Failed to create the extension config")
		responses := responsesConfigMap(clusterName, namespace)
		err = input.BootstrapClusterProxy.GetClient().Create(ctx, responses)
		Expect(err).ToNot(HaveOccurred(), "Failed to create the responses configmap")

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
			EtcdImageTag:                input.E2EConfig.GetVariable(EtcdVersionUpgradeTo),
			DNSImageTag:                 input.E2EConfig.GetVariable(CoreDNSVersionUpgradeTo),
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

		By("Checking all lifecycle hooks have been called")
		// Assert that each hook has been called and returned "Success" during the test.
		err = checkLifecycleHookResponses(ctx, input.BootstrapClusterProxy.GetClient(), namespace.Name, clusterName, map[string]string{
			"BeforeClusterCreate":          "Success",
			"BeforeClusterUpgrade":         "Success",
			"AfterControlPlaneInitialized": "Success",
			"AfterControlPlaneUpgrade":     "Success",
			"AfterClusterUpgrade":          "Success",
		})
		Expect(err).ToNot(HaveOccurred(), "Lifecycle hook calls were not as expected")

		By("PASSED!")
	})

	AfterEach(func() {
		// Dumps all the resources in the spec Namespace, then cleanups the cluster object and the spec Namespace itself.
		dumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)

		Eventually(func() error {
			return input.BootstrapClusterProxy.GetClient().Delete(ctx, ext)
		}, 10*time.Second, 1*time.Second).Should(Succeed())
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
			// Blocking hooks are set to Status:Failure initially. These will be changed during the test.
			"BeforeClusterCreate-preloadedResponse":      fmt.Sprintf(`{"Status": "Failure", "Message": %q}`, hookFailedMessage),
			"BeforeClusterUpgrade-preloadedResponse":     fmt.Sprintf(`{"Status": "Failure", "Message": %q}`, hookFailedMessage),
			"AfterControlPlaneUpgrade-preloadedResponse": fmt.Sprintf(`{"Status": "Failure", "Message": %q}`, hookFailedMessage),

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
			return errors.Errorf("hook %s call not recorded in configMap %s/%s", hookName, namespace, clusterName+"-hookresponses")
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
			return errors.Errorf("hook %s call not recorded in configMap %s/%s", hookName, namespace, clusterName+"-hookresponses")
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
	log.Logf("Blocking with BeforeClusterCreate hook")
	hookName := "BeforeClusterCreate"
	runtimeHookTestHandler(ctx, c, namespace, clusterName, hookName, func() bool {
		blocked := true
		// This hook should block the Cluster from entering the "Provisioned" state.
		cluster := &clusterv1.Cluster{}
		Eventually(func() error {
			return c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: clusterName}, cluster)
		}).Should(Succeed())

		// Check if the TopologyReconciled condition message contains both the hook name and hookFailedMessage.
		if !clusterConditionShowsHookFailed(cluster, hookName) {
			blocked = false
		}
		if cluster.Status.Phase == string(clusterv1.ClusterPhaseProvisioned) {
			blocked = false
		}
		return blocked
	}, intervals)
}

// beforeClusterUpgradeTestHandler calls runtimeHookTestHandler with a blocking function which returns false if the
// Cluster has controlplanev1.RollingUpdateInProgressReason in its ReadyCondition.
func beforeClusterUpgradeTestHandler(ctx context.Context, c client.Client, namespace, clusterName string, intervals []interface{}) {
	log.Logf("Blocking with BeforeClusterUpgrade hook")
	hookName := "BeforeClusterUpgrade"
	runtimeHookTestHandler(ctx, c, namespace, clusterName, hookName, func() bool {
		var blocked = true

		cluster := &clusterv1.Cluster{}
		Eventually(func() error {
			return c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: clusterName}, cluster)
		}).Should(Succeed())

		// Check if the TopologyReconciled condition message contains both the hook name and hookFailedMessage.
		if !clusterConditionShowsHookFailed(cluster, hookName) {
			blocked = false
		}
		// Check if the Cluster is showing the RollingUpdateInProgress condition reason. If it has the update process is unblocked.
		if conditions.IsFalse(cluster, clusterv1.ReadyCondition) &&
			conditions.GetReason(cluster, clusterv1.ReadyCondition) == controlplanev1.RollingUpdateInProgressReason {
			blocked = false
		}
		return blocked
	}, intervals)
}

// afterControlPlaneUpgradeTestHandler calls runtimeHookTestHandler with a blocking function which returns false if any
// MachineDeployment in the Cluster has upgraded to the target Kubernetes version.
func afterControlPlaneUpgradeTestHandler(ctx context.Context, c client.Client, namespace, clusterName, version string, intervals []interface{}) {
	log.Logf("Blocking with AfterControlPlaneUpgrade hook")
	hookName := "AfterControlPlaneUpgrade"
	runtimeHookTestHandler(ctx, c, namespace, clusterName, hookName, func() bool {
		var blocked = true
		cluster := &clusterv1.Cluster{}
		Eventually(func() error {
			return c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: clusterName}, cluster)
		}).Should(Succeed())

		// Check if the TopologyReconciled condition message contains both the hook name and hookFailedMessage.
		if !clusterConditionShowsHookFailed(cluster, hookName) {
			blocked = false
		}

		mds := &clusterv1.MachineDeploymentList{}
		Eventually(func() error {
			return c.List(ctx, mds, client.MatchingLabels{
				clusterv1.ClusterLabelName:          clusterName,
				clusterv1.ClusterTopologyOwnedLabel: "",
			})
		}).Should(Succeed())

		// If any of the MachineDeployments have the target Kubernetes Version, the hook is unblocked.
		for _, md := range mds.Items {
			if *md.Spec.Template.Spec.Version == version {
				blocked = false
			}
		}
		return blocked
	}, intervals)
}

// runtimeHookTestHandler runs a series of tests in sequence to check if the runtimeHook passed to it succeeds.
//	1) Checks that the hook has been called at least once the TopologyReconciled condition is a Failure.
//	2) Check that the hook's blockingCondition is consistently true.
//	- At this point the function sets the hook's response to be non-blocking.
//	3) Check that the hook's blocking condition becomes false.
// Note: runtimeHookTestHandler assumes that the hook passed to it is currently returning a blocking response.
// Updating the response to be non-blocking happens inline in the function.
func runtimeHookTestHandler(ctx context.Context, c client.Client, namespace, clusterName, hookName string, blockingCondition func() bool, intervals []interface{}) {
	// Check that the LifecycleHook has been called at least once and the TopologyReconciled condition is a Failure.
	Eventually(func() error {
		if err := checkLifecycleHooksCalledAtLeastOnce(ctx, c, namespace, clusterName, []string{hookName}); err != nil {
			return err
		}
		cluster := &clusterv1.Cluster{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: clusterName}, cluster); err != nil {
			return err
		}
		if !(conditions.GetReason(cluster, clusterv1.TopologyReconciledCondition) == clusterv1.TopologyReconcileFailedReason) {
			return errors.New("Condition not found on Cluster object")
		}
		return nil
	}, 60*time.Second).Should(Succeed(), "%s has not been called", hookName)

	// blockingCondition should consistently be true as the Runtime hook is returning "Failure".
	Consistently(func() bool {
		return blockingCondition()
	}, 30*time.Second).Should(BeTrue(),
		fmt.Sprintf("Cluster Topology reconciliation continued unexpectedly: hook %s not blocking", hookName))

	// Patch the ConfigMap to set the hook response to "Success".
	Byf("Setting %s response to Status:Success to unblock the reconciliation", hookName)

	configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: clusterName + "-hookresponses", Namespace: namespace}}
	Eventually(func() error {
		return c.Get(ctx, util.ObjectKey(configMap), configMap)
	}).Should(Succeed())
	patch := client.RawPatch(types.MergePatchType,
		[]byte(fmt.Sprintf(`{"data":{"%s-preloadedResponse":%s}}`, hookName, "\"{\\\"Status\\\": \\\"Success\\\"}\"")))
	Eventually(func() error {
		return c.Patch(ctx, configMap, patch)
	}).Should(Succeed())

	// Expect the Hook to pass, setting the blockingCondition to false before the timeout ends.
	Eventually(func() bool {
		return blockingCondition()
	}, intervals...).Should(BeFalse(),
		fmt.Sprintf("ClusterTopology reconcile did not unblock after updating hook response: hook %s still blocking", hookName))
}

// clusterConditionShowsHookFailed checks if the TopologyReconciled condition message contains both the hook name and hookFailedMessage.
func clusterConditionShowsHookFailed(cluster *clusterv1.Cluster, hookName string) bool {
	return conditions.GetReason(cluster, clusterv1.TopologyReconciledCondition) == clusterv1.TopologyReconcileFailedReason &&
		strings.Contains(conditions.GetMessage(cluster, clusterv1.TopologyReconciledCondition), hookFailedMessage) &&
		strings.Contains(conditions.GetMessage(cluster, clusterv1.TopologyReconciledCondition), hookName)
}
