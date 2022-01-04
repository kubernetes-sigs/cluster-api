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
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1alpha3 "sigs.k8s.io/cluster-api/api/v1alpha3"
	clusterv1alpha4 "sigs.k8s.io/cluster-api/api/v1alpha4"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/test/e2e/internal/log"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/bootstrap"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
)

const (
	initWithBinaryVariableName = "INIT_WITH_BINARY"
	initWithProvidersContract  = "INIT_WITH_PROVIDERS_CONTRACT"
	initWithKubernetesVersion  = "INIT_WITH_KUBERNETES_VERSION"
)

// ClusterctlUpgradeSpecInput is the input for ClusterctlUpgradeSpec.
type ClusterctlUpgradeSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	// InitWithBinary can be used to override the INIT_WITH_BINARY e2e config variable with the URL of the clusterctl binary of the old version of Cluster API. The spec will interpolate the
	// strings `{OS}` and `{ARCH}` to `runtime.GOOS` and `runtime.GOARCH` respectively, e.g. https://github.com/kubernetes-sigs/cluster-api/releases/download/v0.3.23/clusterctl-{OS}-{ARCH}
	InitWithBinary string
	// InitWithProvidersContract can be used to override the INIT_WITH_PROVIDERS_CONTRACT e2e config variable with a specific
	// provider contract to use to initialise the secondary management cluster, e.g. `v1alpha3`
	InitWithProvidersContract string
	SkipCleanup               bool
	PreInit                   func(managementClusterProxy framework.ClusterProxy)
	PreUpgrade                func(managementClusterProxy framework.ClusterProxy)
	PostUpgrade               func(managementClusterProxy framework.ClusterProxy)
	MgmtFlavor                string
	WorkloadFlavor            string
}

// ClusterctlUpgradeSpec implements a test that verifies clusterctl upgrade of a management cluster.
//
// NOTE: this test is designed to test older versions of Cluster API --> v1beta1 upgrades.
// This spec will create a workload cluster, which will be converted into a new management cluster (henceforth called secondary
// managemnet cluster)
// with the older version of Cluster API and infrastructure provider. It will then create an additional
// workload cluster (henceforth called secondary workload cluster) from the new management cluster using the default cluster template of the old release
// then run clusterctl upgrade to the latest version of Cluster API and ensure correct operation by
// scaling a MachineDeployment.
//
// To use this spec the variables INIT_WITH_BINARY and INIT_WITH_PROVIDERS_CONTRACT must be set or specified directly
// in the spec input. See ClusterctlUpgradeSpecInput for further information.
//
// In order to get this to work, infrastructure providers need to implement a mechanism to stage
// the locally compiled OCI image of their infrastructure provider and have it downloaded and available
// on the secondary management cluster. It is recommended that infrastructure providers use `docker save` and output
// the local image to a tar file, upload it to object storage, and then use preKubeadmCommands to pre-load the image
// before Kubernetes starts.
//
// For example, for Cluster API Provider AWS, the docker image is stored in an s3 bucket with a unique name for the
// account-region pair, so as to not clash with any other AWS user / account, with the object key being the sha256sum of the
// image digest.
//
// The following commands are then added to preKubeadmCommands:
//
//   preKubeadmCommands:
//   - mkdir -p /opt/cluster-api
//   - aws s3 cp "s3://${S3_BUCKET}/${E2E_IMAGE_SHA}" /opt/cluster-api/image.tar
//   - ctr -n k8s.io images import /opt/cluster-api/image.tar # The image must be imported into the k8s.io namespace
func ClusterctlUpgradeSpec(ctx context.Context, inputGetter func() ClusterctlUpgradeSpecInput) {
	var (
		specName = "clusterctl-upgrade"
		input    ClusterctlUpgradeSpecInput

		testNamespace     *corev1.Namespace
		testCancelWatches context.CancelFunc

		managementClusterName          string
		clusterctlBinaryURL            string
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
		var clusterctlBinaryURLTemplate string
		if input.InitWithBinary == "" {
			Expect(input.E2EConfig.Variables).To(HaveKey(initWithBinaryVariableName), "Invalid argument. %s variable must be defined when calling %s spec", initWithBinaryVariableName, specName)
			Expect(input.E2EConfig.Variables[initWithBinaryVariableName]).ToNot(BeEmpty(), "Invalid argument. %s variable can't be empty when calling %s spec", initWithBinaryVariableName, specName)
			clusterctlBinaryURLTemplate = input.E2EConfig.GetVariable(initWithBinaryVariableName)
		} else {
			clusterctlBinaryURLTemplate = input.InitWithBinary
		}
		clusterctlBinaryURLReplacer := strings.NewReplacer("{OS}", runtime.GOOS, "{ARCH}", runtime.GOARCH)
		clusterctlBinaryURL = clusterctlBinaryURLReplacer.Replace(clusterctlBinaryURLTemplate)
		Expect(input.E2EConfig.Variables).To(HaveKey(initWithKubernetesVersion))
		Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersion))
		Expect(os.MkdirAll(input.ArtifactFolder, 0750)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

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
				Flavor:                   input.MgmtFlavor,
				Namespace:                managementClusterNamespace.Name,
				ClusterName:              managementClusterName,
				KubernetesVersion:        input.E2EConfig.GetVariable(initWithKubernetesVersion),
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

		// Download the older clusterctl version to be used for setting up the management cluster to be upgraded

		log.Logf("Downloading clusterctl binary from %s", clusterctlBinaryURL)
		clusterctlBinaryPath := downloadToTmpFile(clusterctlBinaryURL)
		defer os.Remove(clusterctlBinaryPath) // clean up

		err := os.Chmod(clusterctlBinaryPath, 0744) //nolint:gosec
		Expect(err).ToNot(HaveOccurred(), "failed to chmod temporary file")

		By("Initializing the workload cluster with older versions of providers")

		// NOTE: by default we are considering all the providers, no matter of the contract.
		// However, given that we want to test both v1alpha3 --> v1beta1 and v1alpha4 --> v1beta1, the INIT_WITH_PROVIDERS_CONTRACT
		// variable can be used to select versions with a specific contract.
		contract := "*"
		if input.E2EConfig.HasVariable(initWithProvidersContract) {
			contract = input.E2EConfig.GetVariable(initWithProvidersContract)
		}
		if input.InitWithProvidersContract != "" {
			contract = input.InitWithProvidersContract
		}

		if input.PreInit != nil {
			By("Running Pre-init steps against the management cluster")
			input.PreInit(managementClusterProxy)
		}

		clusterctl.InitManagementClusterAndWatchControllerLogs(ctx, clusterctl.InitManagementClusterAndWatchControllerLogsInput{
			ClusterctlBinaryPath:    clusterctlBinaryPath, // use older version of clusterctl to init the management cluster
			ClusterProxy:            managementClusterProxy,
			ClusterctlConfigPath:    input.ClusterctlConfigPath,
			CoreProvider:            input.E2EConfig.GetProviderLatestVersionsByContract(contract, config.ClusterAPIProviderName)[0],
			BootstrapProviders:      input.E2EConfig.GetProviderLatestVersionsByContract(contract, config.KubeadmBootstrapProviderName),
			ControlPlaneProviders:   input.E2EConfig.GetProviderLatestVersionsByContract(contract, config.KubeadmControlPlaneProviderName),
			InfrastructureProviders: input.E2EConfig.GetProviderLatestVersionsByContract(contract, input.E2EConfig.InfrastructureProviders()...),
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
			Flavor: input.WorkloadFlavor,
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
			machineList := &clusterv1alpha3.MachineList{}
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

		if input.PreUpgrade != nil {
			By("Running Pre-upgrade steps against the management cluster")
			input.PreUpgrade(managementClusterProxy)
		}

		By("Upgrading providers to the latest version available")
		clusterctl.UpgradeManagementClusterAndWait(ctx, clusterctl.UpgradeManagementClusterAndWaitInput{
			ClusterctlConfigPath: input.ClusterctlConfigPath,
			ClusterProxy:         managementClusterProxy,
			Contract:             clusterv1.GroupVersion.Version,
			LogFolder:            filepath.Join(input.ArtifactFolder, "clusters", cluster.Name),
		}, input.E2EConfig.GetIntervals(specName, "wait-controllers")...)

		By("THE MANAGEMENT CLUSTER WAS SUCCESSFULLY UPGRADED!")

		if input.PostUpgrade != nil {
			By("Running Post-upgrade steps against the management cluster")
			input.PostUpgrade(managementClusterProxy)
		}

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
				Namespace: testNamespace.Name,
				LogPath:   filepath.Join(input.ArtifactFolder, "clusters", managementClusterResources.Cluster.Name, "resources"),
			})

			if !input.SkipCleanup {
				switch {
				case discovery.ServerSupportsVersion(managementClusterProxy.GetClientSet().DiscoveryClient, clusterv1.GroupVersion) == nil:
					Byf("Deleting all %s clusters in namespace %s in management cluster %s", clusterv1.GroupVersion, testNamespace.Name, managementClusterName)
					framework.DeleteAllClustersAndWait(ctx, framework.DeleteAllClustersAndWaitInput{
						Client:    managementClusterProxy.GetClient(),
						Namespace: testNamespace.Name,
					}, input.E2EConfig.GetIntervals(specName, "wait-delete-cluster")...)
				case discovery.ServerSupportsVersion(managementClusterProxy.GetClientSet().DiscoveryClient, clusterv1alpha4.GroupVersion) == nil:
					Byf("Deleting all %s clusters in namespace %s in management cluster %s", clusterv1alpha4.GroupVersion, testNamespace.Name, managementClusterName)
					deleteAllClustersAndWaitV1alpha4(ctx, framework.DeleteAllClustersAndWaitInput{
						Client:    managementClusterProxy.GetClient(),
						Namespace: testNamespace.Name,
					}, input.E2EConfig.GetIntervals(specName, "wait-delete-cluster")...)
				case discovery.ServerSupportsVersion(managementClusterProxy.GetClientSet().DiscoveryClient, clusterv1alpha3.GroupVersion) == nil:
					Byf("Deleting all %s clusters in namespace %s in management cluster %s", clusterv1alpha3.GroupVersion, testNamespace.Name, managementClusterName)
					deleteAllClustersAndWaitV1alpha3(ctx, framework.DeleteAllClustersAndWaitInput{
						Client:    managementClusterProxy.GetClient(),
						Namespace: testNamespace.Name,
					}, input.E2EConfig.GetIntervals(specName, "wait-delete-cluster")...)
				default:
					log.Logf("Management Cluster does not appear to support CAPI resources.")
				}

				Byf("Deleting cluster %s/%s", testNamespace.Name, managementClusterName)
				framework.DeleteAllClustersAndWait(ctx, framework.DeleteAllClustersAndWaitInput{
					Client:    managementClusterProxy.GetClient(),
					Namespace: testNamespace.Name,
				}, input.E2EConfig.GetIntervals(specName, "wait-delete-cluster")...)

				Byf("Deleting namespace %s used for hosting the %q test", testNamespace.Name, specName)
				framework.DeleteNamespace(ctx, framework.DeleteNamespaceInput{
					Deleter: managementClusterProxy.GetClient(),
					Name:    testNamespace.Name,
				})

				Byf("Deleting providers")
				clusterctl.Delete(ctx, clusterctl.DeleteInput{
					LogFolder:            filepath.Join(input.ArtifactFolder, "clusters", managementClusterResources.Cluster.Name),
					ClusterctlConfigPath: input.ClusterctlConfigPath,
					KubeconfigPath:       managementClusterProxy.GetKubeconfigPath(),
				})
			}
			testCancelWatches()
		}

		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		dumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, managementClusterNamespace, managementClusterCancelWatches, managementClusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}

func downloadToTmpFile(url string) string {
	tmpFile, err := os.CreateTemp("", "clusterctl")
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

// deleteAllClustersAndWaitV1alpha3 deletes all cluster resources in the given namespace and waits for them to be gone using the older API.
func deleteAllClustersAndWaitV1alpha3(ctx context.Context, input framework.DeleteAllClustersAndWaitInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for deleteAllClustersAndWaitOldAPI")
	Expect(input.Client).ToNot(BeNil(), "Invalid argument. input.Client can't be nil when calling deleteAllClustersAndWaitOldAPI")
	Expect(input.Namespace).ToNot(BeEmpty(), "Invalid argument. input.Namespace can't be empty when calling deleteAllClustersAndWaitOldAPI")

	clusters := getAllClustersByNamespaceV1alpha3(ctx, framework.GetAllClustersByNamespaceInput{
		Lister:    input.Client,
		Namespace: input.Namespace,
	})

	for _, c := range clusters {
		deleteClusterV1alpha3(ctx, deleteClusterV1alpha3Input{
			Deleter: input.Client,
			Cluster: c,
		})
	}

	for _, c := range clusters {
		log.Logf("Waiting for the Cluster %s/%s to be deleted", c.Namespace, c.Name)
		waitForClusterDeletedV1alpha3(ctx, waitForClusterDeletedV1alpha3Input{
			Getter:  input.Client,
			Cluster: c,
		}, intervals...)
	}
}

// getAllClustersByNamespaceV1alpha3 returns the list of Cluster objects in a namespace using the older API.
func getAllClustersByNamespaceV1alpha3(ctx context.Context, input framework.GetAllClustersByNamespaceInput) []*clusterv1alpha3.Cluster {
	clusterList := &clusterv1alpha3.ClusterList{}
	Expect(input.Lister.List(ctx, clusterList, client.InNamespace(input.Namespace))).To(Succeed(), "Failed to list clusters in namespace %s", input.Namespace)

	clusters := make([]*clusterv1alpha3.Cluster, len(clusterList.Items))
	for i := range clusterList.Items {
		clusters[i] = &clusterList.Items[i]
	}
	return clusters
}

// deleteClusterV1alpha3Input is the input for deleteClusterV1alpha3.
type deleteClusterV1alpha3Input struct {
	Deleter framework.Deleter
	Cluster *clusterv1alpha3.Cluster
}

// deleteClusterV1alpha3 deletes the cluster and waits for everything the cluster owned to actually be gone using the older API.
func deleteClusterV1alpha3(ctx context.Context, input deleteClusterV1alpha3Input) {
	By(fmt.Sprintf("Deleting cluster %s", input.Cluster.GetName()))
	Expect(input.Deleter.Delete(ctx, input.Cluster)).To(Succeed())
}

// waitForClusterDeletedV1alpha3Input is the input for waitForClusterDeletedV1alpha3.
type waitForClusterDeletedV1alpha3Input struct {
	Getter  framework.Getter
	Cluster *clusterv1alpha3.Cluster
}

// waitForClusterDeletedV1alpha3 waits until the cluster object has been deleted using the older API.
func waitForClusterDeletedV1alpha3(ctx context.Context, input waitForClusterDeletedV1alpha3Input, intervals ...interface{}) {
	By(fmt.Sprintf("Waiting for cluster %s to be deleted", input.Cluster.GetName()))
	Eventually(func() bool {
		cluster := &clusterv1alpha3.Cluster{}
		key := client.ObjectKey{
			Namespace: input.Cluster.GetNamespace(),
			Name:      input.Cluster.GetName(),
		}
		return apierrors.IsNotFound(input.Getter.Get(ctx, key, cluster))
	}, intervals...).Should(BeTrue())
}

// deleteAllClustersAndWaitV1alpha4 deletes all cluster resources in the given namespace and waits for them to be gone using the older API.
func deleteAllClustersAndWaitV1alpha4(ctx context.Context, input framework.DeleteAllClustersAndWaitInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for deleteAllClustersAndWaitOldAPI")
	Expect(input.Client).ToNot(BeNil(), "Invalid argument. input.Client can't be nil when calling deleteAllClustersAndWaitOldAPI")
	Expect(input.Namespace).ToNot(BeEmpty(), "Invalid argument. input.Namespace can't be empty when calling deleteAllClustersAndWaitOldAPI")

	clusters := getAllClustersByNamespaceV1alpha4(ctx, framework.GetAllClustersByNamespaceInput{
		Lister:    input.Client,
		Namespace: input.Namespace,
	})

	for _, c := range clusters {
		deleteClusterV1alpha4(ctx, deleteClusterV1alpha4Input{
			Deleter: input.Client,
			Cluster: c,
		})
	}

	for _, c := range clusters {
		log.Logf("Waiting for the Cluster %s/%s to be deleted", c.Namespace, c.Name)
		waitForClusterDeletedV1alpha4(ctx, waitForClusterDeletedV1alpha4Input{
			Getter:  input.Client,
			Cluster: c,
		}, intervals...)
	}
}

// getAllClustersByNamespaceV1alpha4 returns the list of Cluster objects in a namespace using the older API.
func getAllClustersByNamespaceV1alpha4(ctx context.Context, input framework.GetAllClustersByNamespaceInput) []*clusterv1alpha4.Cluster {
	clusterList := &clusterv1alpha4.ClusterList{}
	Expect(input.Lister.List(ctx, clusterList, client.InNamespace(input.Namespace))).To(Succeed(), "Failed to list clusters in namespace %s", input.Namespace)

	clusters := make([]*clusterv1alpha4.Cluster, len(clusterList.Items))
	for i := range clusterList.Items {
		clusters[i] = &clusterList.Items[i]
	}
	return clusters
}

// deleteClusterV1alpha4Input is the input for deleteClusterV1alpha4.
type deleteClusterV1alpha4Input struct {
	Deleter framework.Deleter
	Cluster *clusterv1alpha4.Cluster
}

// deleteClusterV1alpha4 deletes the cluster and waits for everything the cluster owned to actually be gone using the older API.
func deleteClusterV1alpha4(ctx context.Context, input deleteClusterV1alpha4Input) {
	By(fmt.Sprintf("Deleting cluster %s", input.Cluster.GetName()))
	Expect(input.Deleter.Delete(ctx, input.Cluster)).To(Succeed())
}

// waitForClusterDeletedV1alpha4Input is the input for waitForClusterDeletedV1alpha4.
type waitForClusterDeletedV1alpha4Input struct {
	Getter  framework.Getter
	Cluster *clusterv1alpha4.Cluster
}

// waitForClusterDeletedV1alpha4 waits until the cluster object has been deleted using the older API.
func waitForClusterDeletedV1alpha4(ctx context.Context, input waitForClusterDeletedV1alpha4Input, intervals ...interface{}) {
	By(fmt.Sprintf("Waiting for cluster %s to be deleted", input.Cluster.GetName()))
	Eventually(func() bool {
		cluster := &clusterv1alpha4.Cluster{}
		key := client.ObjectKey{
			Namespace: input.Cluster.GetNamespace(),
			Name:      input.Cluster.GetName(),
		}
		return apierrors.IsNotFound(input.Getter.Get(ctx, key, cluster))
	}, intervals...).Should(BeTrue())
}
