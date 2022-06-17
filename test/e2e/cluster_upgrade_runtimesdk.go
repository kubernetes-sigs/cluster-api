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
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
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

		// Setup a Namespace where to host objects for this spec and create a watcher for the Namespace events.
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
		// Assert that each hook passed to this function is marked as "true" in the response configmap
		err = checkLifecycleHooks(ctx, input.BootstrapClusterProxy.GetClient(), namespace.Name, clusterName, map[string]string{
			"BeforeClusterCreate":          "",
			"BeforeClusterUpgrade":         "",
			"AfterControlPlaneInitialized": "",
			"AfterControlPlaneUpgrade":     "",
			"AfterClusterUpgrade":          "",
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
		// Every response contain only Status:Success. The test checks whether each handler has been called at least once.
		Data: map[string]string{
			"BeforeClusterCreate-response":          `{"Status": "Success"}`,
			"BeforeClusterUpgrade-response":         `{"Status": "Success"}`,
			"AfterControlPlaneInitialized-response": `{"Status": "Success"}`,
			"AfterControlPlaneUpgrade-response":     `{"Status": "Success"}`,
			"AfterClusterUpgrade-response":          `{"Status": "Success"}`,
		},
	}
}

func checkLifecycleHooks(ctx context.Context, c client.Client, namespace string, clusterName string, hooks map[string]string) error {
	configMap := &corev1.ConfigMap{}
	configMapName := clusterName + "-hookresponses"
	err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: configMapName}, configMap)
	Expect(err).ToNot(HaveOccurred(), "Failed to get the hook response configmap")
	for hook := range hooks {
		if _, ok := configMap.Data[hook+"-called"]; !ok {
			return errors.Errorf("hook %s call not recorded in configMap %s/%s", hook, namespace, configMapName)
		}
	}
	return nil
}
