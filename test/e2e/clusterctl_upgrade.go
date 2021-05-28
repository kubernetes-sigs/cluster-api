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
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	clusterv1old "sigs.k8s.io/cluster-api/api/v1alpha3"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/test/e2e/internal/log"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/bootstrap"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const initWithBinaryVariableName = "INIT_WITH_BINARY"

// ClusterctlUpgradeSpecInput is the input for ClusterctlUpgradeSpec.
type ClusterctlUpgradeSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool
}

// ClusterctlUpgradeSpec implements a test that verifies clusterctl upgrade of a management cluster.
//
// NOTE: this test is designed to test v1alpha3 --> v1alpha4 upgrades.
func ClusterctlUpgradeSpec(ctx context.Context, inputGetter func() ClusterctlUpgradeSpecInput) {
	var (
		specName = "clusterctl-upgrade"
		input    ClusterctlUpgradeSpecInput

		testNamespace     *corev1.Namespace
		testCancelWatches context.CancelFunc

		managementClusterName          string
		managementClusterNamespace     *corev1.Namespace
		managementClusterCancelWatches context.CancelFunc
		managementClusterResources     *clusterctl.ApplyClusterTemplateAndWaitResult
		managementClusterProxy         framework.ClusterProxy

		workLoadClusterName string
	)

	BeforeEach(func() {
		Expect(ctx).NotTo(BeNil(), "ctx is required for %s spec", specName)
		input = inputGetter()
		Expect(input.E2EConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. input.BootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.E2EConfig.Variables).To(HaveKey(initWithBinaryVariableName), "Invalid argument. %s variable must be defined when calling %s spec", initWithBinaryVariableName, specName)
		Expect(input.E2EConfig.Variables[initWithBinaryVariableName]).ToNot(BeEmpty(), "Invalid argument. %s variable can't be empty when calling %s spec", initWithBinaryVariableName, specName)
		Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersion))
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		managementClusterNamespace, managementClusterCancelWatches = setupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder)
		managementClusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should create a management cluster and then upgrade all the providers", func() {
		By("Creating a workload cluster to be used as a new management cluster")
		// NOTE: given that the bootstrap cluster could be shared by several tests, it is not practical to use it for testing clusterctl upgrades.
		// So we are creating a workload cluster that will be used as a new management cluster where to install older version of providers
		managementClusterName = fmt.Sprintf("%s-%s", specName, util.RandomString(6))
		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   clusterctl.DefaultInfrastructureProvider,
				Flavor:                   clusterctl.DefaultFlavor,
				Namespace:                managementClusterNamespace.Name,
				ClusterName:              managementClusterName,
				KubernetesVersion:        input.E2EConfig.GetVariable(KubernetesVersion),
				ControlPlaneMachineCount: pointer.Int64Ptr(1),
				WorkerMachineCount:       pointer.Int64Ptr(1),
			},
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		}, managementClusterResources)

		By("Turning the workload cluster into a management cluster with older versions of providers")

		// If the cluster is a DockerCluster, we should load controller images into the nodes.
		// Nb. this can be achieved also by changing the DockerMachine spec, but for the time being we are using
		// this approach because this allows to have a single source of truth for images, the e2e config
		// Nb. the images for official version of the providers will be pulled from internet, but the latest images must be
		// built locally and loaded into kind
		cluster := managementClusterResources.Cluster
		if cluster.Spec.InfrastructureRef.Kind == "DockerCluster" {
			Expect(bootstrap.LoadImagesToKindCluster(ctx, bootstrap.LoadImagesToKindClusterInput{
				Name:   cluster.Name,
				Images: input.E2EConfig.Images,
			})).To(Succeed())
		}

		// Get a ClusterProxy so we can interact with the workload cluster
		managementClusterProxy = input.BootstrapClusterProxy.GetWorkloadCluster(ctx, cluster.Namespace, cluster.Name)

		// Download the v1alpha3 clusterctl version to be used for setting up the management cluster to be upgraded
		clusterctlBinaryURL := input.E2EConfig.GetVariable(initWithBinaryVariableName)
		clusterctlBinaryURL = strings.ReplaceAll(clusterctlBinaryURL, "{OS}", runtime.GOOS)
		clusterctlBinaryURL = strings.ReplaceAll(clusterctlBinaryURL, "{ARCH}", runtime.GOARCH)

		log.Logf("downloading clusterctl binary from %s", clusterctlBinaryURL)
		clusterctlBinaryPath := downloadToTmpFile(clusterctlBinaryURL)
		defer os.Remove(clusterctlBinaryPath) // clean up

		err := os.Chmod(clusterctlBinaryPath, 0744)
		Expect(err).ToNot(HaveOccurred(), "failed to chmod temporary file")

		By("Initializing the workload cluster with older versions of providers")
		clusterctl.InitManagementClusterAndWatchControllerLogs(ctx, clusterctl.InitManagementClusterAndWatchControllerLogsInput{
			ClusterctlBinaryPath:    clusterctlBinaryPath, // use older version of clusterctl to init the management cluster
			ClusterProxy:            managementClusterProxy,
			ClusterctlConfigPath:    input.ClusterctlConfigPath,
			CoreProvider:            input.E2EConfig.GetProvidersWithOldestVersion(config.ClusterAPIProviderName)[0],
			BootstrapProviders:      input.E2EConfig.GetProvidersWithOldestVersion(config.KubeadmBootstrapProviderName),
			ControlPlaneProviders:   input.E2EConfig.GetProvidersWithOldestVersion(config.KubeadmControlPlaneProviderName),
			InfrastructureProviders: input.E2EConfig.GetProvidersWithOldestVersion(input.E2EConfig.InfrastructureProviders()...),
			LogFolder:               filepath.Join(input.ArtifactFolder, "clusters", cluster.Name),
		}, input.E2EConfig.GetIntervals(specName, "wait-controllers")...)

		By("THE MANAGEMENT CLUSTER WITH THE OLDER VERSION OF PROVIDERS IS UP&RUNNING!")

		Byf("Creating a namespace for hosting the %s test workload cluster", specName)
		testNamespace, testCancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:   managementClusterProxy.GetClient(),
			ClientSet: managementClusterProxy.GetClientSet(),
			Name:      specName,
			LogFolder: filepath.Join(input.ArtifactFolder, "clusters", "bootstrap"),
		})

		By("Creating a test workload cluster")

		// NOTE: This workload cluster is used to check the old management cluster works fine.
		// In this case ApplyClusterTemplateAndWait can't be used because this helper is linked to the last version of the API;
		// so we are getting a template using the downloaded version of clusterctl, applying it, and wait for machines to be provisioned.

		workLoadClusterName = fmt.Sprintf("%s-%s", specName, util.RandomString(6))
		kubernetesVersion := input.E2EConfig.GetVariable(KubernetesVersion)
		controlPlaneMachineCount := pointer.Int64Ptr(1)
		workerMachineCount := pointer.Int64Ptr(1)

		log.Logf("Creating the workload cluster with name %q using the %q template (Kubernetes %s, %d control-plane machines, %d worker machines)",
			workLoadClusterName, "(default)", kubernetesVersion, *controlPlaneMachineCount, *workerMachineCount)

		log.Logf("Getting the cluster template yaml")
		workloadClusterTemplate := clusterctl.ConfigClusterWithBinary(ctx, clusterctlBinaryPath, clusterctl.ConfigClusterInput{
			// pass reference to the management cluster hosting this test
			KubeconfigPath: managementClusterProxy.GetKubeconfigPath(),
			// pass the clusterctl config file that points to the local provider repository created for this test,
			ClusterctlConfigPath: input.ClusterctlConfigPath,
			// select template
			Flavor: clusterctl.DefaultFlavor,
			// define template variables
			Namespace:                testNamespace.Name,
			ClusterName:              workLoadClusterName,
			KubernetesVersion:        kubernetesVersion,
			ControlPlaneMachineCount: controlPlaneMachineCount,
			WorkerMachineCount:       workerMachineCount,
			InfrastructureProvider:   clusterctl.DefaultInfrastructureProvider,
			// setup clusterctl logs folder
			LogFolder: filepath.Join(input.ArtifactFolder, "clusters", managementClusterProxy.GetName()),
		})
		Expect(workloadClusterTemplate).ToNot(BeNil(), "Failed to get the cluster template")

		log.Logf("Applying the cluster template yaml to the cluster")
		Expect(managementClusterProxy.Apply(ctx, workloadClusterTemplate)).To(Succeed())

		By("Waiting for the machines to exists")
		Eventually(func() (int64, error) {
			var n int64
			machineList := &clusterv1old.MachineList{}
			if err := managementClusterProxy.GetClient().List(ctx, machineList, client.InNamespace(testNamespace.Name), client.MatchingLabels{clusterv1.ClusterLabelName: workLoadClusterName}); err == nil {
				for _, machine := range machineList.Items {
					if machine.Status.NodeRef != nil {
						n++
					}
				}
			}
			return n, nil
		}, input.E2EConfig.GetIntervals(specName, "wait-worker-nodes")...).Should(Equal(*controlPlaneMachineCount + *workerMachineCount))

		By("THE MANAGEMENT CLUSTER WITH OLDER VERSION OF PROVIDERS WORKS!")

		By("Upgrading providers to the latest version available")
		clusterctl.UpgradeManagementClusterAndWait(ctx, clusterctl.UpgradeManagementClusterAndWaitInput{
			ClusterctlConfigPath: input.ClusterctlConfigPath,
			ClusterProxy:         managementClusterProxy,
			Contract:             clusterv1.GroupVersion.Version,
			LogFolder:            filepath.Join(input.ArtifactFolder, "clusters", cluster.Name),
		}, input.E2EConfig.GetIntervals(specName, "wait-controllers")...)

		By("THE MANAGEMENT CLUSTER WAS SUCCESSFULLY UPGRADED!")

		// After upgrading we are sure the version is the latest version of the API,
		// so it is possible to use the standard helpers

		testMachineDeployments := framework.GetMachineDeploymentsByCluster(ctx, framework.GetMachineDeploymentsByClusterInput{
			Lister:      managementClusterProxy.GetClient(),
			ClusterName: workLoadClusterName,
			Namespace:   testNamespace.Name,
		})

		framework.ScaleAndWaitMachineDeployment(ctx, framework.ScaleAndWaitMachineDeploymentInput{
			ClusterProxy:              managementClusterProxy,
			Cluster:                   &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace.Name}},
			MachineDeployment:         testMachineDeployments[0],
			Replicas:                  2,
			WaitForMachineDeployments: input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		})

		By("THE UPGRADED MANAGEMENT CLUSTER WORKS!")

		By("PASSED!")
	})

	AfterEach(func() {
		if testNamespace != nil {
			// Dump all the logs from the workload cluster before deleting them.
			managementClusterProxy.CollectWorkloadClusterLogs(ctx, testNamespace.Name, managementClusterName, filepath.Join(input.ArtifactFolder, "clusters", managementClusterName, "machines"))

			framework.DumpAllResources(ctx, framework.DumpAllResourcesInput{
				Lister:    managementClusterProxy.GetClient(),
				Namespace: managementClusterNamespace.Name,
				LogPath:   filepath.Join(input.ArtifactFolder, "clusters", managementClusterResources.Cluster.Name, "resources"),
			})

			if !input.SkipCleanup {
				Byf("Deleting cluster %s and %s", testNamespace.Name, managementClusterName)
				framework.DeleteAllClustersAndWait(ctx, framework.DeleteAllClustersAndWaitInput{
					Client:    managementClusterProxy.GetClient(),
					Namespace: testNamespace.Name,
				}, input.E2EConfig.GetIntervals(specName, "wait-delete-cluster")...)

				Byf("Deleting namespace used for hosting the %q test", specName)
				framework.DeleteNamespace(ctx, framework.DeleteNamespaceInput{
					Deleter: managementClusterProxy.GetClient(),
					Name:    testNamespace.Name,
				})
			}
			testCancelWatches()
		}

		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		dumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, managementClusterNamespace, managementClusterCancelWatches, managementClusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}

func downloadToTmpFile(url string) string {
	tmpFile, err := ioutil.TempFile("", "clusterctl")
	Expect(err).ToNot(HaveOccurred(), "failed to get temporary file")
	defer tmpFile.Close()

	// Get the data
	resp, err := http.Get(url) //nolint:gosec
	Expect(err).ToNot(HaveOccurred(), "failed to get clusterctl")
	defer resp.Body.Close()

	// Write the body to file
	_, err = io.Copy(tmpFile, resp.Body)
	Expect(err).ToNot(HaveOccurred(), "failed to write temporary file")

	return tmpFile.Name()
}
