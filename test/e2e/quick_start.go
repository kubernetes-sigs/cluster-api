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

package e2e

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
)

// QuickStartSpecInput is the input for QuickStartSpec.
type QuickStartSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool

	// Cluster name allows to specify a deterministic clusterName.
	// If not set, a random one will be generated.
	ClusterName *string

	// DeployClusterClassInSeparateNamespace defines if the ClusterClass should be deployed in a separate namespace.
	DeployClusterClassInSeparateNamespace bool

	// InfrastructureProvider allows to specify the infrastructure provider to be used when looking for
	// cluster templates.
	// If not set, clusterctl will look at the infrastructure provider installed in the management cluster;
	// if only one infrastructure provider exists, it will be used, otherwise the operation will fail if more than one exists.
	InfrastructureProvider *string

	// Flavor, if specified is the template flavor used to create the cluster for testing.
	// If not specified, the default flavor for the selected infrastructure provider is used.
	Flavor *string

	// ControlPlaneMachineCount defines the number of control plane machines to be added to the workload cluster.
	// If not specified, 1 will be used.
	ControlPlaneMachineCount *int64

	// WorkerMachineCount defines number of worker machines to be added to the workload cluster.
	// If not specified, 1 will be used.
	WorkerMachineCount *int64

	// Allows to inject functions to be run while waiting for the control plane to be initialized,
	// which unblocks CNI installation, and for the control plane machines to be ready (after CNI installation).
	ControlPlaneWaiters clusterctl.ControlPlaneWaiters

	// ExtensionConfigName is the name of the ExtensionConfig. Defaults to "quick-start".
	// This value is provided to clusterctl as "EXTENSION_CONFIG_NAME" variable and can be used to template the
	// name of the ExtensionConfig into the ClusterClass.
	ExtensionConfigName string

	// ExtensionServiceNamespace is the namespace where the service for the Runtime Extension is located.
	// Note: This should only be set if a Runtime Extension is used.
	ExtensionServiceNamespace string

	// ExtensionServiceNamespace is the name where the service for the Runtime Extension is located.
	// Note: This should only be set if a Runtime Extension is used.
	ExtensionServiceName string

	// Allows to inject a function to be run after test namespace is created.
	// If not specified, this is a no-op.
	PostNamespaceCreated func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace string)

	// Allows to inject a function to be run after machines are provisioned.
	// If not specified, this is a no-op.
	PostMachinesProvisioned func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace, workloadClusterName string)

	// ClusterctlVariables allows injecting variables to the cluster template.
	// If not specified, this is a no-op.
	ClusterctlVariables map[string]string
}

// QuickStartSpec implements a spec that mimics the operation described in the Cluster API quick start, that is
// creating a workload cluster.
// This test is meant to provide a first, fast signal to detect regression; it is recommended to use it as a PR blocker test.
// NOTE: This test works with Clusters with and without ClusterClass.
func QuickStartSpec(ctx context.Context, inputGetter func() QuickStartSpecInput) {
	var (
		specName              = "quick-start"
		input                 QuickStartSpecInput
		namespace             *corev1.Namespace
		clusterClassNamespace *corev1.Namespace
		cancelWatches         context.CancelFunc
		clusterResources      *clusterctl.ApplyClusterTemplateAndWaitResult
	)

	BeforeEach(func() {
		Expect(ctx).NotTo(BeNil(), "ctx is required for %s spec", specName)
		input = inputGetter()
		Expect(input.E2EConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. input.BootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0750)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersion))

		if input.ExtensionServiceNamespace != "" && input.ExtensionServiceName != "" {
			if input.ExtensionConfigName == "" {
				input.ExtensionConfigName = specName
			}
		}

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = framework.SetupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, input.PostNamespaceCreated)

		if input.DeployClusterClassInSeparateNamespace {
			clusterClassNamespace = framework.CreateNamespace(ctx, framework.CreateNamespaceInput{Creator: input.BootstrapClusterProxy.GetClient(), Name: fmt.Sprintf("%s-clusterclass", namespace.Name)}, "40s", "10s")
			Expect(clusterClassNamespace).ToNot(BeNil(), "Failed to create namespace")
		}

		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should create a workload cluster", func() {
		By("Creating a workload cluster")

		infrastructureProvider := clusterctl.DefaultInfrastructureProvider
		if input.InfrastructureProvider != nil {
			infrastructureProvider = *input.InfrastructureProvider
		}

		flavor := clusterctl.DefaultFlavor
		if input.Flavor != nil {
			flavor = *input.Flavor
		}

		controlPlaneMachineCount := ptr.To[int64](1)
		if input.ControlPlaneMachineCount != nil {
			controlPlaneMachineCount = input.ControlPlaneMachineCount
		}

		workerMachineCount := ptr.To[int64](1)
		if input.WorkerMachineCount != nil {
			workerMachineCount = input.WorkerMachineCount
		}

		clusterName := fmt.Sprintf("%s-%s", specName, util.RandomString(6))
		if input.ClusterName != nil {
			clusterName = *input.ClusterName
		}

		if input.ExtensionServiceNamespace != "" && input.ExtensionServiceName != "" {
			// NOTE: test extension is already deployed in the management cluster. If for any reason in future we want
			// to make this test more self-contained this test should be modified in order to create an additional
			// management cluster; also the E2E test configuration should be modified introducing something like
			// optional:true allowing to define which providers should not be installed by default in
			// a management cluster.
			By("Deploy Test Extension ExtensionConfig")

			// In this test we are defaulting all handlers to non-blocking because we don't expect the handlers to block the
			// cluster lifecycle by default. Setting defaultAllHandlersToBlocking to false enforces that the test-extension
			// automatically creates the ConfigMap with non-blocking preloaded responses.
			defaultAllHandlersToBlocking := false
			// select on the current namespace
			// This is necessary so in CI this test doesn't influence other tests by enabling lifecycle hooks
			// in other test namespaces.
			namespaces := []string{namespace.Name}
			if input.DeployClusterClassInSeparateNamespace {
				// Add the ClusterClass namespace, if the ClusterClass is deployed in a separate namespace.
				namespaces = append(namespaces, clusterClassNamespace.Name)
			}
			extensionConfig := extensionConfig(input.ExtensionConfigName, input.ExtensionServiceNamespace, input.ExtensionServiceName, defaultAllHandlersToBlocking, namespaces...)
			Expect(input.BootstrapClusterProxy.GetClient().Create(ctx,
				extensionConfig)).
				To(Succeed(), "Failed to create the ExtensionConfig")
		}

		variables := map[string]string{
			// This is used to template the name of the ExtensionConfig into the ClusterClass.
			"EXTENSION_CONFIG_NAME": input.ExtensionConfigName,
		}
		maps.Copy(variables, input.ClusterctlVariables)

		if input.DeployClusterClassInSeparateNamespace {
			variables["CLUSTER_CLASS_NAMESPACE"] = clusterClassNamespace.Name
			By("Creating a cluster referencing a ClusterClass from another namespace")
		}

		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				ClusterctlVariables:      variables,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   infrastructureProvider,
				Flavor:                   flavor,
				Namespace:                namespace.Name,
				ClusterName:              clusterName,
				KubernetesVersion:        input.E2EConfig.MustGetVariable(KubernetesVersion),
				ControlPlaneMachineCount: controlPlaneMachineCount,
				WorkerMachineCount:       workerMachineCount,
			},
			ControlPlaneWaiters:          input.ControlPlaneWaiters,
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
			PostMachinesProvisioned: func() {
				if input.PostMachinesProvisioned != nil {
					input.PostMachinesProvisioned(input.BootstrapClusterProxy, namespace.Name, clusterName)
				}
			},
		}, clusterResources)

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

		By("PASSED!")
	})

	AfterEach(func() {
		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		framework.DumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ClusterctlConfigPath, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
		if !input.SkipCleanup {
			if input.ExtensionServiceNamespace != "" && input.ExtensionServiceName != "" {
				Eventually(func() error {
					return input.BootstrapClusterProxy.GetClient().Delete(ctx, extensionConfig(input.ExtensionConfigName, input.ExtensionServiceNamespace, input.ExtensionServiceName, true))
				}, 10*time.Second, 1*time.Second).Should(Succeed(), "Deleting ExtensionConfig failed")
			}
			if input.DeployClusterClassInSeparateNamespace {
				framework.DeleteNamespace(ctx, framework.DeleteNamespaceInput{
					Deleter: input.BootstrapClusterProxy.GetClient(),
					Name:    clusterClassNamespace.Name,
				})
			}
		}
	})
}
