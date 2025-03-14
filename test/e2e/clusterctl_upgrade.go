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
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/controllers/external"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/e2e/internal/log"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/bootstrap"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
)

// ClusterctlUpgradeSpecInput is the input for ClusterctlUpgradeSpec.
type ClusterctlUpgradeSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string

	// UseKindForManagementCluster instruct the test to use kind for creating the management cluster (instead to use the actual infrastructure provider).
	// NOTE: given that the bootstrap cluster could be shared by several tests, it is not practical to use it for testing clusterctl upgrades.
	// So we are creating a new management cluster where to install older version of providers
	UseKindForManagementCluster bool
	// KindManagementClusterNewClusterProxyFunc is used to create the ClusterProxy used in the test after creating the kind based management cluster.
	// This allows to use a custom ClusterProxy implementation or create a ClusterProxy with a custom scheme and options.
	KindManagementClusterNewClusterProxyFunc func(name string, kubeconfigPath string) framework.ClusterProxy

	// InitWithBinary must be used to specify the URL of the clusterctl binary of the old version of Cluster API. The spec will interpolate the
	// strings `{OS}` and `{ARCH}` to `runtime.GOOS` and `runtime.GOARCH` respectively, e.g. https://github.com/kubernetes-sigs/cluster-api/releases/download/v0.3.23/clusterctl-{OS}-{ARCH}
	InitWithBinary string
	// InitWithProvidersContract can be used to set the contract used to initialise the secondary management cluster, e.g. `v1alpha3`
	InitWithProvidersContract string
	// InitWithKubernetesVersion must be used to set a Kubernetes version to use to create the secondary management cluster, e.g. `v1.25.0`
	InitWithKubernetesVersion string
	// InitWithCoreProvider specifies the core provider version to use when initializing the secondary management cluster, e.g. `cluster-api:v1.3.0`.
	// If not set, the core provider version is calculated based on the contract.
	InitWithCoreProvider string
	// InitWithBootstrapProviders specifies the bootstrap provider versions to use when initializing the secondary management cluster, e.g. `kubeadm:v1.3.0`.
	// If not set, the bootstrap provider version is calculated based on the contract.
	InitWithBootstrapProviders []string
	// InitWithControlPlaneProviders specifies the control plane provider versions to use when initializing the secondary management cluster, e.g. `kubeadm:v1.3.0`.
	// If not set, the control plane provider version is calculated based on the contract.
	InitWithControlPlaneProviders []string
	// InitWithInfrastructureProviders specifies the infrastructure provider versions to add to the secondary management cluster, e.g. `aws:v2.0.0`.
	// If not set, the infrastructure provider version is calculated based on the contract.
	InitWithInfrastructureProviders []string
	// InitWithIPAMProviders specifies the IPAM provider versions to add to the secondary management cluster, e.g. `infoblox:v0.0.1`.
	// If not set, the IPAM provider version is calculated based on the contract.
	InitWithIPAMProviders []string
	// InitWithRuntimeExtensionProviders specifies the runtime extension provider versions to add to the secondary management cluster, e.g. `test:v0.0.1`.
	// If not set, the runtime extension provider version is calculated based on the contract.
	InitWithRuntimeExtensionProviders []string
	// InitWithAddonProviders specifies the add-on provider versions to add to the secondary management cluster, e.g. `helm:v0.0.1`.
	// If not set, the add-on provider version is calculated based on the contract.
	InitWithAddonProviders []string
	// UpgradeClusterctlVariables can be used to set additional variables for clusterctl upgrade.
	UpgradeClusterctlVariables map[string]string
	SkipCleanup                bool

	// InfrastructureProviders specifies the infrastructure to use for clusterctl
	// operations (Example: get cluster templates).
	// Note: In most cases this need not be specified. It only needs to be specified when
	// multiple infrastructure providers are installed on the cluster as clusterctl will not be
	// able to identify the default.
	InfrastructureProvider *string
	// Allows to inject a function to be run after test namespace is created.
	// If not specified, this is a no-op.
	PostNamespaceCreated func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace string)
	// PreWaitForCluster is a function that can be used as a hook to apply extra resources (that cannot be part of the template) in the generated namespace hosting the cluster
	// This function is called after applying the cluster template and before waiting for the cluster resources.
	PreWaitForCluster   func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace string, workloadClusterName string)
	ControlPlaneWaiters clusterctl.ControlPlaneWaiters
	PreInit             func(managementClusterProxy framework.ClusterProxy)
	PreUpgrade          func(managementClusterProxy framework.ClusterProxy)
	PostUpgrade         func(managementClusterProxy framework.ClusterProxy, clusterNamespace, clusterName string)
	// PreCleanupManagementCluster hook can be used for extra steps that might be required from providers, for example, remove conflicting service (such as DHCP) running on
	// the target management cluster and run it on bootstrap (before the latter resumes LCM) if both clusters share the same LAN
	PreCleanupManagementCluster func(managementClusterProxy framework.ClusterProxy)
	MgmtFlavor                  string
	CNIManifestPath             string
	WorkloadFlavor              string
	// WorkloadKubernetesVersion is Kubernetes version used to create the workload cluster, e.g. `v1.25.0`
	WorkloadKubernetesVersion string

	// Upgrades allows to define upgrade sequences.
	// If not set, the test will upgrade once to the v1beta1 contract.
	// For some examples see clusterctl_upgrade_test.go
	Upgrades []ClusterctlUpgradeSpecInputUpgrade

	// ControlPlaneMachineCount specifies the number of control plane machines to create in the workload cluster.
	ControlPlaneMachineCount *int64
}

// ClusterctlUpgradeSpecInputUpgrade defines an upgrade.
type ClusterctlUpgradeSpecInputUpgrade struct {
	// UpgradeWithBinary can be used to set the clusterctl binary to use for the provider upgrade. The spec will interpolate the
	// strings `{OS}` and `{ARCH}` to `runtime.GOOS` and `runtime.GOARCH` respectively, e.g. https://github.com/kubernetes-sigs/cluster-api/releases/download/v0.3.23/clusterctl-{OS}-{ARCH}
	// If not set, the test will use the ApplyUpgrade function of the clusterctl library.
	WithBinary string
	// Contract defines the contract to upgrade to.
	// Either the contract *or* the *Provider fields should be defined
	// For some examples see clusterctl_upgrade_test.go
	Contract                  string
	CoreProvider              string
	BootstrapProviders        []string
	ControlPlaneProviders     []string
	InfrastructureProviders   []string
	IPAMProviders             []string
	RuntimeExtensionProviders []string
	AddonProviders            []string

	// PostUpgrade is called after the upgrade is completed.
	PostUpgrade func(proxy framework.ClusterProxy, namespace string, clusterName string)
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
// To use this spec the fields InitWithBinary and InitWithKubernetesVersion must be specified
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
//	preKubeadmCommands:
//	- mkdir -p /opt/cluster-api
//	- aws s3 cp "s3://${S3_BUCKET}/${E2E_IMAGE_SHA}" /opt/cluster-api/image.tar
//	- ctr -n k8s.io images import /opt/cluster-api/image.tar # The image must be imported into the k8s.io namespace
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
		managementClusterProvider      bootstrap.ClusterProvider
		managementClusterProxy         framework.ClusterProxy

		initClusterctlBinaryURL string
		initContract            string
		initKubernetesVersion   string

		workloadClusterName string
	)

	BeforeEach(func() {
		Expect(ctx).NotTo(BeNil(), "ctx is required for %s spec", specName)
		input = inputGetter()
		Expect(input.E2EConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. input.BootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.InitWithBinary).ToNot(BeEmpty(), "Invalid argument. input.InitWithBinary can't be empty when calling %s spec", specName)
		Expect(input.InitWithKubernetesVersion).ToNot(BeEmpty(), "Invalid argument. input.InitWithKubernetesVersion can't be empty when calling %s spec", specName)
		if input.KindManagementClusterNewClusterProxyFunc == nil {
			input.KindManagementClusterNewClusterProxyFunc = func(name string, kubeconfigPath string) framework.ClusterProxy {
				scheme := apiruntime.NewScheme()
				framework.TryAddDefaultSchemes(scheme)
				return framework.NewClusterProxy(name, kubeconfigPath, scheme)
			}
		}

		clusterctlBinaryURLTemplate := input.InitWithBinary
		clusterctlBinaryURLReplacer := strings.NewReplacer("{OS}", runtime.GOOS, "{ARCH}", runtime.GOARCH)
		initClusterctlBinaryURL = clusterctlBinaryURLReplacer.Replace(clusterctlBinaryURLTemplate)

		// NOTE: by default we are considering all the providers, no matter of the contract.
		// However, given that we want to test both v1alpha3 --> v1beta1 and v1alpha4 --> v1beta1,
		// InitWithProvidersContract can be used to select versions with a specific contract.
		initContract = "*"
		if input.InitWithProvidersContract != "" {
			initContract = input.InitWithProvidersContract
		}

		initKubernetesVersion = input.InitWithKubernetesVersion

		if len(input.Upgrades) == 0 {
			// Upgrade once to v1beta1 if no upgrades are specified.
			input.Upgrades = []ClusterctlUpgradeSpecInputUpgrade{
				{
					Contract: clusterv1.GroupVersion.Version,
				},
			}
		}

		Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersion))
		Expect(os.MkdirAll(input.ArtifactFolder, 0750)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		// If the test is not being run in a separated kind cluster, setup a Namespace in the current bootstrap cluster where to host objects for this spec and create a watcher for the namespace events.
		if !input.UseKindForManagementCluster {
			managementClusterNamespace, managementClusterCancelWatches = framework.SetupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, input.PostNamespaceCreated)
		}
		managementClusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should create a management cluster and then upgrade all the providers", func() {
		infrastructureProvider := clusterctl.DefaultInfrastructureProvider
		if input.InfrastructureProvider != nil {
			infrastructureProvider = *input.InfrastructureProvider
		}
		// NOTE: given that the bootstrap cluster could be shared by several tests, it is not practical to use it for testing clusterctl upgrades.
		// So we are creating a workload cluster that will be used as a new management cluster where to install older version of providers
		managementClusterName = fmt.Sprintf("%s-management-%s", specName, util.RandomString(6))
		managementClusterLogFolder := filepath.Join(input.ArtifactFolder, "clusters", managementClusterName)
		if input.UseKindForManagementCluster {
			By("Creating a kind cluster to be used as a new management cluster")

			managementClusterProvider = bootstrap.CreateKindBootstrapClusterAndLoadImages(ctx, bootstrap.CreateKindBootstrapClusterAndLoadImagesInput{
				Name:               managementClusterName,
				KubernetesVersion:  initKubernetesVersion,
				RequiresDockerSock: input.E2EConfig.HasDockerProvider(),
				// Note: most of this images won't be used while starting the controllers, because it is used to spin up older versions of CAPI. Those images will be eventually used when upgrading to current.
				Images:    input.E2EConfig.Images,
				IPFamily:  input.E2EConfig.MustGetVariable(IPFamily),
				LogFolder: filepath.Join(managementClusterLogFolder, "logs-kind"),
			})
			Expect(managementClusterProvider).ToNot(BeNil(), "Failed to create a kind cluster")

			kubeconfigPath := managementClusterProvider.GetKubeconfigPath()
			Expect(kubeconfigPath).To(BeAnExistingFile(), "Failed to get the kubeconfig file for the kind cluster")

			managementClusterProxy = input.KindManagementClusterNewClusterProxyFunc(managementClusterName, kubeconfigPath)
			Expect(managementClusterProxy).ToNot(BeNil(), "Failed to get a kind cluster proxy")

			managementClusterResources.Cluster = &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: managementClusterName,
				},
			}
		} else {
			By("Creating a workload cluster to be used as a new management cluster")

			clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
				ClusterProxy: input.BootstrapClusterProxy,
				ConfigCluster: clusterctl.ConfigClusterInput{
					LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
					ClusterctlConfigPath:     input.ClusterctlConfigPath,
					KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
					InfrastructureProvider:   infrastructureProvider,
					Flavor:                   input.MgmtFlavor,
					Namespace:                managementClusterNamespace.Name,
					ClusterName:              managementClusterName,
					KubernetesVersion:        initKubernetesVersion,
					ControlPlaneMachineCount: ptr.To[int64](1),
					WorkerMachineCount:       ptr.To[int64](1),
				},
				PreWaitForCluster: func() {
					if input.PreWaitForCluster != nil {
						input.PreWaitForCluster(input.BootstrapClusterProxy, managementClusterNamespace.Name, managementClusterName)
					}
				},
				CNIManifestPath:              input.CNIManifestPath,
				ControlPlaneWaiters:          input.ControlPlaneWaiters,
				WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
				WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
				WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
			}, managementClusterResources)

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
			managementClusterProxy = input.BootstrapClusterProxy.GetWorkloadCluster(ctx, cluster.Namespace, cluster.Name, framework.WithMachineLogCollector(input.BootstrapClusterProxy.GetLogCollector()))
		}

		By("Turning the new cluster into a management cluster with older versions of providers")

		// Download the clusterctl version that should be used to initially set up the management cluster (which is later upgraded).
		Byf("Downloading clusterctl binary from %s", initClusterctlBinaryURL)
		clusterctlBinaryPath, clusterctlConfigPath := setupClusterctl(ctx, initClusterctlBinaryURL, input.ClusterctlConfigPath)
		defer os.Remove(clusterctlBinaryPath) // clean up

		By("Initializing the new management cluster with older versions of providers")

		if input.PreInit != nil {
			By("Running Pre-init steps against the new management cluster")
			input.PreInit(managementClusterProxy)
		}

		var (
			coreProvider              string
			bootstrapProviders        []string
			controlPlaneProviders     []string
			infrastructureProviders   []string
			ipamProviders             []string
			runtimeExtensionProviders []string
			addonProviders            []string
		)

		coreProvider = input.E2EConfig.GetProviderLatestVersionsByContract(initContract, config.ClusterAPIProviderName)[0]
		if input.InitWithCoreProvider != "" {
			coreProvider = input.InitWithCoreProvider
		}

		bootstrapProviders = getValueOrFallback(input.InitWithBootstrapProviders,
			input.E2EConfig.GetProviderLatestVersionsByContract(initContract, config.KubeadmBootstrapProviderName))
		controlPlaneProviders = getValueOrFallback(input.InitWithControlPlaneProviders,
			input.E2EConfig.GetProviderLatestVersionsByContract(initContract, config.KubeadmControlPlaneProviderName))
		infrastructureProviders = getValueOrFallback(input.InitWithInfrastructureProviders,
			input.E2EConfig.GetProviderLatestVersionsByContract(initContract, input.E2EConfig.InfrastructureProviders()...))
		ipamProviders = getValueOrFallback(input.InitWithIPAMProviders,
			input.E2EConfig.GetProviderLatestVersionsByContract(initContract, input.E2EConfig.IPAMProviders()...))
		runtimeExtensionProviders = getValueOrFallback(input.InitWithRuntimeExtensionProviders,
			input.E2EConfig.GetProviderLatestVersionsByContract(initContract, input.E2EConfig.RuntimeExtensionProviders()...))
		addonProviders = getValueOrFallback(input.InitWithAddonProviders,
			input.E2EConfig.GetProviderLatestVersionsByContract(initContract, input.E2EConfig.AddonProviders()...))

		clusterctl.InitManagementClusterAndWatchControllerLogs(ctx, clusterctl.InitManagementClusterAndWatchControllerLogsInput{
			ClusterctlBinaryPath:      clusterctlBinaryPath, // use older version of clusterctl to init the management cluster
			ClusterProxy:              managementClusterProxy,
			ClusterctlConfigPath:      clusterctlConfigPath,
			CoreProvider:              coreProvider,
			BootstrapProviders:        bootstrapProviders,
			ControlPlaneProviders:     controlPlaneProviders,
			InfrastructureProviders:   infrastructureProviders,
			IPAMProviders:             ipamProviders,
			RuntimeExtensionProviders: runtimeExtensionProviders,
			AddonProviders:            addonProviders,
			LogFolder:                 managementClusterLogFolder,
		}, input.E2EConfig.GetIntervals(specName, "wait-controllers")...)

		By("THE MANAGEMENT CLUSTER WITH THE OLDER VERSION OF PROVIDERS IS UP&RUNNING!")

		Byf("Creating a namespace for hosting the %s test workload cluster", specName)
		testNamespace, testCancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:   managementClusterProxy.GetClient(),
			ClientSet: managementClusterProxy.GetClientSet(),
			Name:      specName,
			LogFolder: filepath.Join(input.ArtifactFolder, "clusters", "bootstrap"),
		})

		if input.PostNamespaceCreated != nil {
			log.Logf("Calling postNamespaceCreated for namespace %s", testNamespace.Name)
			input.PostNamespaceCreated(managementClusterProxy, testNamespace.Name)
		}

		By("Creating a test workload cluster")

		// NOTE: This workload cluster is used to check the old management cluster works fine.
		// In this case ApplyClusterTemplateAndWait can't be used because this helper is linked to the last version of the API;
		// so we are getting a template using the downloaded version of clusterctl, applying it, and wait for machines to be provisioned.

		workloadClusterName = fmt.Sprintf("%s-workload-%s", specName, util.RandomString(6))
		workloadClusterNamespace := testNamespace.Name
		kubernetesVersion := input.WorkloadKubernetesVersion
		if kubernetesVersion == "" {
			kubernetesVersion = input.E2EConfig.MustGetVariable(KubernetesVersion)
		}
		controlPlaneMachineCount := ptr.To[int64](1)
		if input.ControlPlaneMachineCount != nil {
			controlPlaneMachineCount = input.ControlPlaneMachineCount
		}
		workerMachineCount := ptr.To[int64](1)

		log.Logf("Creating the workload cluster with name %q using the %q template (Kubernetes %s, %d control-plane machines, %d worker machines)",
			workloadClusterName, "(default)", kubernetesVersion, *controlPlaneMachineCount, *workerMachineCount)

		log.Logf("Getting the cluster template yaml")
		workloadClusterTemplate := clusterctl.ConfigClusterWithBinary(ctx, clusterctlBinaryPath, clusterctl.ConfigClusterInput{
			// pass reference to the management cluster hosting this test
			KubeconfigPath: managementClusterProxy.GetKubeconfigPath(),
			// pass the clusterctl config file that points to the local provider repository created for this test,
			ClusterctlConfigPath: clusterctlConfigPath,
			// select template
			Flavor: input.WorkloadFlavor,
			// define template variables
			Namespace:                workloadClusterNamespace,
			ClusterName:              workloadClusterName,
			KubernetesVersion:        kubernetesVersion,
			ControlPlaneMachineCount: controlPlaneMachineCount,
			WorkerMachineCount:       workerMachineCount,
			InfrastructureProvider:   infrastructureProvider,
			// setup clusterctl logs folder
			LogFolder: filepath.Join(input.ArtifactFolder, "clusters", managementClusterProxy.GetName()),
		})
		Expect(workloadClusterTemplate).ToNot(BeNil(), "Failed to get the cluster template")

		// Applying the cluster template in dry-run to ensure mgmt cluster webhooks are up and available
		log.Logf("Applying the cluster template yaml to the cluster in dry-run")
		Eventually(func() error {
			return managementClusterProxy.CreateOrUpdate(ctx, workloadClusterTemplate, framework.WithCreateOpts([]client.CreateOption{client.DryRunAll}...), framework.WithUpdateOpts([]client.UpdateOption{client.DryRunAll}...))
		}, "1m", "10s").ShouldNot(HaveOccurred())

		log.Logf("Applying the cluster template yaml to the cluster")
		Expect(managementClusterProxy.CreateOrUpdate(ctx, workloadClusterTemplate)).To(Succeed())

		if input.PreWaitForCluster != nil {
			By("Running PreWaitForCluster steps against the management cluster")
			input.PreWaitForCluster(managementClusterProxy, workloadClusterNamespace, workloadClusterName)
		}

		coreCAPIStorageVersion := getCoreCAPIStorageVersion(ctx, managementClusterProxy.GetClient())

		// Note: We have to use unstructured here as the Cluster could be e.g. v1alpha3 / v1alpha4 / v1beta1.
		workloadClusterUnstructured := discoveryAndWaitForCluster(ctx, discoveryAndWaitForClusterInput{
			Client:                 managementClusterProxy.GetClient(),
			CoreCAPIStorageVersion: coreCAPIStorageVersion,
			Namespace:              workloadClusterNamespace,
			Name:                   workloadClusterName,
		}, input.E2EConfig.GetIntervals(specName, "wait-cluster")...)

		By("Calculating expected MachineDeployment and MachinePool Machine and Node counts")
		expectedMachineDeploymentMachineCount := calculateExpectedMachineDeploymentMachineCount(ctx, managementClusterProxy.GetClient(), workloadClusterUnstructured, coreCAPIStorageVersion)
		expectedMachinePoolNodeCount := calculateExpectedMachinePoolNodeCount(ctx, managementClusterProxy.GetClient(), workloadClusterUnstructured, coreCAPIStorageVersion)
		expectedMachinePoolMachineCount, err := calculateExpectedMachinePoolMachineCount(ctx, managementClusterProxy.GetClient(), workloadClusterNamespace, workloadClusterName)
		Expect(err).ToNot(HaveOccurred())

		expectedMachineCount := *controlPlaneMachineCount + expectedMachineDeploymentMachineCount + expectedMachinePoolMachineCount

		Byf("Expect %d Machines and %d MachinePool replicas to exist", expectedMachineCount, expectedMachinePoolNodeCount)
		By("Waiting for the machines to exist")
		Eventually(func() (int64, error) {
			var n int64
			machineList := &unstructured.UnstructuredList{}
			machineList.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   clusterv1.GroupVersion.Group,
				Version: coreCAPIStorageVersion,
				Kind:    "MachineList",
			})
			if err := managementClusterProxy.GetClient().List(
				ctx,
				machineList,
				client.InNamespace(workloadClusterNamespace),
				client.MatchingLabels{clusterv1.ClusterNameLabel: workloadClusterName},
			); err == nil {
				for _, m := range machineList.Items {
					_, found, err := unstructured.NestedMap(m.Object, "status", "nodeRef")
					if err == nil && found {
						n++
					}
				}
			}
			return n, nil
		}, input.E2EConfig.GetIntervals(specName, "wait-worker-nodes")...).Should(Equal(expectedMachineCount), "Timed out waiting for all Machines to exist")

		By("Waiting for MachinePool to be ready with correct number of replicas")
		Eventually(func() (int64, error) {
			var n int64
			machinePoolList := &expv1.MachinePoolList{}
			if err := managementClusterProxy.GetClient().List(
				ctx,
				machinePoolList,
				client.InNamespace(workloadClusterNamespace),
				client.MatchingLabels{clusterv1.ClusterNameLabel: workloadClusterName},
			); err == nil {
				for _, mp := range machinePoolList.Items {
					if mp.Status.Phase == string(expv1.MachinePoolPhaseRunning) {
						n += int64(mp.Status.ReadyReplicas)
					}
				}
			}

			return n, nil
		}, input.E2EConfig.GetIntervals(specName, "wait-worker-nodes")...).Should(Equal(expectedMachinePoolNodeCount), "Timed out waiting for all MachinePool replicas to be ready")

		By("THE MANAGEMENT CLUSTER WITH OLDER VERSION OF PROVIDERS WORKS!")

		for i, upgrade := range input.Upgrades {
			Byf("[%d] Starting upgrade", i)
			if input.PreUpgrade != nil {
				Byf("[%d] Running Pre-upgrade steps against the management cluster", i)
				input.PreUpgrade(managementClusterProxy)
			}

			// Get the workloadCluster before the management cluster is upgraded to make sure that the upgrade did not trigger
			// any unexpected rollouts.
			preUpgradeMachineList := &unstructured.UnstructuredList{}
			preUpgradeMachineList.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   clusterv1.GroupVersion.Group,
				Version: coreCAPIStorageVersion,
				Kind:    "MachineList",
			})
			err := managementClusterProxy.GetClient().List(
				ctx,
				preUpgradeMachineList,
				client.InNamespace(workloadClusterNamespace),
				client.MatchingLabels{clusterv1.ClusterNameLabel: workloadClusterName},
			)
			Expect(err).ToNot(HaveOccurred())

			clusterctlUpgradeBinaryPath := ""
			// TODO: While it is generally fine to use this clusterctl config for upgrades as well,
			// it is not ideal because it points to the latest repositories (e.g. _artifacts/repository/cluster-api/latest/components.yaml)
			// For example this means if we upgrade to v1.5 the upgrade won't use the metadata.yaml from v1.5 it will use the one from latest.
			clusterctlUpgradeConfigPath := input.ClusterctlConfigPath
			if upgrade.WithBinary != "" {
				// Download the clusterctl version to be used to upgrade the management cluster
				upgradeClusterctlBinaryURL := strings.NewReplacer("{OS}", runtime.GOOS, "{ARCH}", runtime.GOARCH).Replace(upgrade.WithBinary)
				Byf("[%d] Downloading clusterctl binary from %s", i, upgradeClusterctlBinaryURL)
				clusterctlUpgradeBinaryPath, clusterctlUpgradeConfigPath = setupClusterctl(ctx, upgradeClusterctlBinaryURL, input.ClusterctlConfigPath)
				defer os.Remove(clusterctlBinaryPath) //nolint:gocritic // Resource leakage is not a concern here.
			}

			// Check if the user want a custom upgrade
			isCustomUpgrade := upgrade.CoreProvider != "" ||
				len(upgrade.BootstrapProviders) > 0 ||
				len(upgrade.ControlPlaneProviders) > 0 ||
				len(upgrade.InfrastructureProviders) > 0 ||
				len(upgrade.IPAMProviders) > 0 ||
				len(upgrade.RuntimeExtensionProviders) > 0 ||
				len(upgrade.AddonProviders) > 0

			if isCustomUpgrade {
				Byf("[%d] Upgrading providers to custom versions", i)
				clusterctl.UpgradeManagementClusterAndWait(ctx, clusterctl.UpgradeManagementClusterAndWaitInput{
					ClusterctlBinaryPath:      clusterctlUpgradeBinaryPath, // use specific version of clusterctl to upgrade the management cluster (if set)
					ClusterctlConfigPath:      clusterctlUpgradeConfigPath,
					ClusterctlVariables:       input.UpgradeClusterctlVariables,
					ClusterProxy:              managementClusterProxy,
					CoreProvider:              upgrade.CoreProvider,
					BootstrapProviders:        upgrade.BootstrapProviders,
					ControlPlaneProviders:     upgrade.ControlPlaneProviders,
					InfrastructureProviders:   upgrade.InfrastructureProviders,
					IPAMProviders:             upgrade.IPAMProviders,
					RuntimeExtensionProviders: upgrade.RuntimeExtensionProviders,
					AddonProviders:            upgrade.AddonProviders,
					LogFolder:                 managementClusterLogFolder,
				}, input.E2EConfig.GetIntervals(specName, "wait-controllers")...)
			} else {
				Byf("[%d] Upgrading providers to the latest version available", i)
				clusterctl.UpgradeManagementClusterAndWait(ctx, clusterctl.UpgradeManagementClusterAndWaitInput{
					ClusterctlBinaryPath: clusterctlUpgradeBinaryPath, // use specific version of clusterctl to upgrade the management cluster (if set)
					ClusterctlConfigPath: clusterctlUpgradeConfigPath,
					ClusterctlVariables:  input.UpgradeClusterctlVariables,
					ClusterProxy:         managementClusterProxy,
					Contract:             upgrade.Contract,
					LogFolder:            managementClusterLogFolder,
				}, input.E2EConfig.GetIntervals(specName, "wait-controllers")...)
			}

			Byf("[%d] THE MANAGEMENT CLUSTER WAS SUCCESSFULLY UPGRADED!", i)

			// We have to get the core CAPI storage version again as the upgrade might have stopped serving v1alpha3/v1alpha4.
			coreCAPIStorageVersion = getCoreCAPIStorageVersion(ctx, managementClusterProxy.GetClient())

			// Note: Currently we only support v1beta1 core CAPI apiVersion after upgrades.
			// This seems a reasonable simplification as we don't want to test upgrades to v1alpha3 / v1alpha4.
			workloadCluster := framework.DiscoveryAndWaitForCluster(ctx, framework.DiscoveryAndWaitForClusterInput{
				Getter:    managementClusterProxy.GetClient(),
				Namespace: workloadClusterNamespace,
				Name:      workloadClusterName,
			}, input.E2EConfig.GetIntervals(specName, "wait-cluster")...)

			if input.PostUpgrade != nil {
				Byf("[%d] Running Post-upgrade steps against the management cluster", i)
				input.PostUpgrade(managementClusterProxy, workloadClusterNamespace, managementClusterName)
			}

			// After the upgrade: wait for MachineList to be available after the upgrade.
			Eventually(func() error {
				postUpgradeMachineList := &unstructured.UnstructuredList{}
				postUpgradeMachineList.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   clusterv1.GroupVersion.Group,
					Version: coreCAPIStorageVersion,
					Kind:    "MachineList",
				})
				return managementClusterProxy.GetClient().List(
					ctx,
					postUpgradeMachineList,
					client.InNamespace(workloadCluster.GetNamespace()),
					client.MatchingLabels{clusterv1.ClusterNameLabel: workloadCluster.GetName()},
				)
			}, "3m", "30s").ShouldNot(HaveOccurred(), "MachineList should be available after the upgrade")

			Byf("[%d] Waiting for three minutes before checking if an unexpected rollout happened", i)
			time.Sleep(time.Minute * 3)

			// After the upgrade: check that there were no unexpected rollouts.
			postUpgradeMachineList := &unstructured.UnstructuredList{}
			Byf("[%d] Verifing there are no unexpected rollouts", i)
			Eventually(func() error {
				postUpgradeMachineList.SetGroupVersionKind(schema.GroupVersionKind{
					Group:   clusterv1.GroupVersion.Group,
					Version: coreCAPIStorageVersion,
					Kind:    "MachineList",
				})
				return managementClusterProxy.GetClient().List(
					ctx,
					postUpgradeMachineList,
					client.InNamespace(workloadCluster.GetNamespace()),
					client.MatchingLabels{clusterv1.ClusterNameLabel: workloadCluster.GetName()},
				)
			}, "3m", "30s").ShouldNot(HaveOccurred(), "MachineList should be available after the upgrade")
			Expect(validateMachineRollout(preUpgradeMachineList, postUpgradeMachineList)).To(BeTrue(), "Machines should remain the same after the upgrade")

			// Scale up to 2 and back down to 1 so we can repeat this multiple times.
			Byf("[%d] Scale MachineDeployment to ensure the providers work", i)
			if workloadCluster.Spec.Topology != nil {
				// Cluster is using ClusterClass, scale up via topology.
				framework.ScaleAndWaitMachineDeploymentTopology(ctx, framework.ScaleAndWaitMachineDeploymentTopologyInput{
					ClusterProxy:              managementClusterProxy,
					Cluster:                   workloadCluster,
					Replicas:                  2,
					WaitForMachineDeployments: input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
				})
				framework.ScaleAndWaitMachineDeploymentTopology(ctx, framework.ScaleAndWaitMachineDeploymentTopologyInput{
					ClusterProxy:              managementClusterProxy,
					Cluster:                   workloadCluster,
					Replicas:                  1,
					WaitForMachineDeployments: input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
				})
			} else {
				// Cluster is not using ClusterClass, scale up via MachineDeployment.
				testMachineDeployments := framework.GetMachineDeploymentsByCluster(ctx, framework.GetMachineDeploymentsByClusterInput{
					Lister:      managementClusterProxy.GetClient(),
					ClusterName: workloadClusterName,
					Namespace:   workloadClusterNamespace,
				})
				if len(testMachineDeployments) > 0 {
					framework.ScaleAndWaitMachineDeployment(ctx, framework.ScaleAndWaitMachineDeploymentInput{
						ClusterProxy:              managementClusterProxy,
						Cluster:                   workloadCluster,
						MachineDeployment:         testMachineDeployments[0],
						Replicas:                  2,
						WaitForMachineDeployments: input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
					})
					framework.ScaleAndWaitMachineDeployment(ctx, framework.ScaleAndWaitMachineDeploymentInput{
						ClusterProxy:              managementClusterProxy,
						Cluster:                   workloadCluster,
						MachineDeployment:         testMachineDeployments[0],
						Replicas:                  1,
						WaitForMachineDeployments: input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
					})
				} else {
					Byf("[%d] No MachineDeployments found to scale", i)
				}
			}

			if upgrade.PostUpgrade != nil {
				upgrade.PostUpgrade(managementClusterProxy, workloadCluster.Namespace, workloadCluster.Name)
			}

			Byf("[%d] Verify v1beta2 Available and Ready conditions (if exist) to be true for Cluster and Machines", i)
			verifyV1Beta2ConditionsTrue(ctx, managementClusterProxy.GetClient(), workloadCluster.Name, workloadCluster.Namespace,
				[]string{clusterv1.AvailableV1Beta2Condition, clusterv1.ReadyV1Beta2Condition})

			Byf("[%d] Verify client-side SSA still works", i)
			clusterUpdate := &unstructured.Unstructured{}
			clusterUpdate.SetGroupVersionKind(clusterv1.GroupVersion.WithKind("Cluster"))
			clusterUpdate.SetNamespace(workloadCluster.Namespace)
			clusterUpdate.SetName(workloadCluster.Name)
			clusterUpdate.SetLabels(map[string]string{
				fmt.Sprintf("test-label-upgrade-%d", i): "test-label-value",
			})
			err = managementClusterProxy.GetClient().Patch(ctx, clusterUpdate, client.Apply, client.FieldOwner("e2e-test-client"))
			Expect(err).ToNot(HaveOccurred())

			Byf("[%d] THE UPGRADED MANAGEMENT CLUSTER WORKS!", i)
		}

		By("PASSED!")
	})

	AfterEach(func() {
		if testNamespace != nil {
			// Dump all the logs from the workload cluster before deleting them.
			framework.DumpAllResourcesAndLogs(ctx, managementClusterProxy, input.ClusterctlConfigPath, input.ArtifactFolder, testNamespace, managementClusterResources.Cluster)

			if !input.SkipCleanup {
				Byf("Deleting all clusters in namespace %s in management cluster %s", testNamespace.Name, managementClusterName)
				deleteAllClustersAndWait(ctx, deleteAllClustersAndWaitInput{
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

		if input.PreCleanupManagementCluster != nil {
			By("Running PreCleanupManagementCluster steps against the management cluster")
			input.PreCleanupManagementCluster(managementClusterProxy)
		}

		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		if input.UseKindForManagementCluster {
			if !input.SkipCleanup {
				managementClusterProxy.Dispose(ctx)
				managementClusterProvider.Dispose(ctx)
			}
		} else {
			framework.DumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ClusterctlConfigPath, input.ArtifactFolder, managementClusterNamespace, managementClusterCancelWatches, managementClusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
		}
	})
}

// verifyV1Beta2ConditionsTrue checks the Cluster and Machines of a Cluster that
// the given v1beta2 condition types are set to true without a message, if they exist.
func verifyV1Beta2ConditionsTrue(ctx context.Context, c client.Client, clusterName, clusterNamespace string, v1beta2conditionTypes []string) {
	cluster := framework.GetClusterByName(ctx, framework.GetClusterByNameInput{
		Getter:    c,
		Name:      clusterName,
		Namespace: clusterNamespace,
	})
	for _, conditionType := range v1beta2conditionTypes {
		if v1beta2conditions.Has(cluster, conditionType) {
			condition := v1beta2conditions.Get(cluster, conditionType)
			Expect(condition.Status).To(Equal(metav1.ConditionTrue), "The v1beta2 condition %q on the Cluster should be set to true", conditionType)
			Expect(condition.Message).To(BeEmpty(), "The v1beta2 condition %q on the Cluster should have an empty message", conditionType)
		}
	}

	machines := framework.GetMachinesByCluster(ctx, framework.GetMachinesByClusterInput{
		Lister:      c,
		ClusterName: clusterName,
		Namespace:   clusterNamespace,
	})
	for _, machine := range machines {
		for _, conditionType := range v1beta2conditionTypes {
			if v1beta2conditions.Has(&machine, conditionType) {
				condition := v1beta2conditions.Get(&machine, conditionType)
				Expect(condition.Status).To(Equal(metav1.ConditionTrue), "The v1beta2 condition %q on the Machine %q should be set to true", conditionType, machine.Name)
				Expect(condition.Message).To(BeEmpty(), "The v1beta2 condition %q on the Machine %q should have an empty message", conditionType, machine.Name)
			}
		}
	}
}

func setupClusterctl(ctx context.Context, clusterctlBinaryURL, clusterctlConfigPath string) (string, string) {
	clusterctlBinaryPath := downloadToTmpFile(ctx, clusterctlBinaryURL)

	err := os.Chmod(clusterctlBinaryPath, 0744) //nolint:gosec
	Expect(err).ToNot(HaveOccurred(), "failed to chmod temporary file")

	// Adjusts the clusterctlConfigPath in case the clusterctl version <= v1.3 (thus using a config file with only the providers supported in those versions)
	clusterctlConfigPath = clusterctl.AdjustConfigPathForBinary(clusterctlBinaryPath, clusterctlConfigPath)

	return clusterctlBinaryPath, clusterctlConfigPath
}

func downloadToTmpFile(ctx context.Context, url string) string {
	tmpFile, err := os.CreateTemp("", "clusterctl")
	Expect(err).ToNot(HaveOccurred(), "failed to get temporary file")
	defer tmpFile.Close()

	// Get the data
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	Expect(err).ToNot(HaveOccurred(), "failed to get clusterctl: failed to create request")

	resp, err := http.DefaultClient.Do(req)
	Expect(err).ToNot(HaveOccurred(), "failed to get clusterctl")
	defer resp.Body.Close()

	// Write the body to file
	_, err = io.Copy(tmpFile, resp.Body)
	Expect(err).ToNot(HaveOccurred(), "failed to write temporary file")

	return tmpFile.Name()
}

func getCoreCAPIStorageVersion(ctx context.Context, c client.Client) string {
	clusterCRD := &apiextensionsv1.CustomResourceDefinition{}
	if err := c.Get(ctx, client.ObjectKey{Name: "clusters.cluster.x-k8s.io"}, clusterCRD); err != nil {
		Expect(err).ToNot(HaveOccurred(), "failed to retrieve a machine CRD")
	}
	// Pick the storage version
	for _, version := range clusterCRD.Spec.Versions {
		if version.Storage {
			return version.Name
		}
	}
	Fail("Cluster CRD has no storage version")
	return ""
}

// discoveryAndWaitForClusterInput is the input type for DiscoveryAndWaitForCluster.
type discoveryAndWaitForClusterInput struct {
	Client                 client.Client
	CoreCAPIStorageVersion string
	Namespace              string
	Name                   string
}

// discoveryAndWaitForCluster discovers a cluster object in a namespace and waits for the cluster infrastructure to be provisioned.
func discoveryAndWaitForCluster(ctx context.Context, input discoveryAndWaitForClusterInput, intervals ...interface{}) *unstructured.Unstructured {
	Expect(ctx).NotTo(BeNil(), "ctx is required for discoveryAndWaitForCluster")
	Expect(input.Client).ToNot(BeNil(), "Invalid argument. input.Client can't be nil when calling discoveryAndWaitForCluster")
	Expect(input.Namespace).ToNot(BeNil(), "Invalid argument. input.Namespace can't be empty when calling discoveryAndWaitForCluster")
	Expect(input.Name).ToNot(BeNil(), "Invalid argument. input.Name can't be empty when calling discoveryAndWaitForCluster")

	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   clusterv1.GroupVersion.Group,
		Version: input.CoreCAPIStorageVersion,
		Kind:    "Cluster",
	})
	Eventually(func(g Gomega) {
		key := client.ObjectKey{
			Namespace: input.Namespace,
			Name:      input.Name,
		}
		g.Expect(input.Client.Get(ctx, key, cluster)).To(Succeed())

		clusterPhase, ok, err := unstructured.NestedString(cluster.Object, "status", "phase")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ok).To(BeTrue(), "could not get status.phase field")
		g.Expect(clusterPhase).To(Equal(string(clusterv1.ClusterPhaseProvisioned)), "Timed out waiting for Cluster %s to provision", klog.KObj(cluster))
	}, intervals...).Should(Succeed(), "Failed to get Cluster object %s", klog.KRef(input.Namespace, input.Name))

	return cluster
}

func calculateExpectedMachineDeploymentMachineCount(ctx context.Context, c client.Client, unstructuredCluster *unstructured.Unstructured, coreCAPIStorageVersion string) int64 {
	var expectedMachineDeploymentWorkerCount int64

	// Convert v1beta1 unstructured Cluster to clusterv1.Cluster
	// Only v1beta1 Cluster support ClusterClass (i.e. have cluster.spec.topology).
	if unstructuredCluster.GroupVersionKind().Version == clusterv1.GroupVersion.Version {
		cluster := &clusterv1.Cluster{}
		Expect(apiruntime.DefaultUnstructuredConverter.FromUnstructured(unstructuredCluster.Object, cluster)).To(Succeed())

		if cluster.Spec.Topology != nil {
			if cluster.Spec.Topology.Workers != nil {
				for _, md := range cluster.Spec.Topology.Workers.MachineDeployments {
					if md.Replicas == nil {
						continue
					}
					expectedMachineDeploymentWorkerCount += int64(*md.Replicas)
				}
			}
			return expectedMachineDeploymentWorkerCount
		}
	}

	byClusterOptions := []client.ListOption{
		client.InNamespace(unstructuredCluster.GetNamespace()),
		client.MatchingLabels{clusterv1.ClusterNameLabel: unstructuredCluster.GetName()},
	}

	machineDeploymentList := &unstructured.UnstructuredList{}
	machineDeploymentList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   clusterv1.GroupVersion.Group,
		Version: coreCAPIStorageVersion,
		Kind:    "MachineDeploymentList",
	})
	Eventually(func() error {
		return c.List(ctx, machineDeploymentList, byClusterOptions...)
	}, 3*time.Minute, 3*time.Second).Should(Succeed(), "Failed to list MachineDeployments object for Cluster %s", klog.KObj(unstructuredCluster))
	for _, md := range machineDeploymentList.Items {
		replicas, ok, err := unstructured.NestedInt64(md.Object, "spec", "replicas")
		Expect(err).ToNot(HaveOccurred())
		if !ok {
			continue
		}
		expectedMachineDeploymentWorkerCount += replicas
	}

	return expectedMachineDeploymentWorkerCount
}

func calculateExpectedMachinePoolMachineCount(ctx context.Context, c client.Client, workloadClusterNamespace, workloadClusterName string) (int64, error) {
	expectedMachinePoolMachineCount := int64(0)

	machinePoolList := &expv1.MachinePoolList{}
	if err := c.List(
		ctx,
		machinePoolList,
		client.InNamespace(workloadClusterNamespace),
		client.MatchingLabels{clusterv1.ClusterNameLabel: workloadClusterName},
	); err == nil {
		for _, mp := range machinePoolList.Items {
			infraMachinePool, err := external.Get(ctx, c, &mp.Spec.Template.Spec.InfrastructureRef)
			if err != nil {
				return 0, err
			}
			// Check if the InfraMachinePool has an infrastructureMachineKind field. If it does not, we should skip checking for MachinePool machines.
			err = util.UnstructuredUnmarshalField(infraMachinePool, ptr.To(""), "status", "infrastructureMachineKind")
			if err != nil && !errors.Is(err, util.ErrUnstructuredFieldNotFound) {
				return 0, err
			}
			if err == nil {
				expectedMachinePoolMachineCount += int64(*mp.Spec.Replicas)
			}
		}
	}

	return expectedMachinePoolMachineCount, nil
}

func calculateExpectedMachinePoolNodeCount(ctx context.Context, c client.Client, unstructuredCluster *unstructured.Unstructured, coreCAPIStorageVersion string) int64 {
	var expectedMachinePoolWorkerCount int64

	// Convert v1beta1 unstructured Cluster to clusterv1.Cluster
	// Only v1beta1 Cluster support ClusterClass (i.e. have cluster.spec.topology).
	if unstructuredCluster.GroupVersionKind().Version == clusterv1.GroupVersion.Version {
		cluster := &clusterv1.Cluster{}
		Expect(apiruntime.DefaultUnstructuredConverter.FromUnstructured(unstructuredCluster.Object, cluster)).To(Succeed())

		if cluster.Spec.Topology != nil {
			if cluster.Spec.Topology.Workers != nil {
				for _, mp := range cluster.Spec.Topology.Workers.MachinePools {
					if mp.Replicas == nil {
						continue
					}
					expectedMachinePoolWorkerCount += int64(*mp.Replicas)
				}
			}
			return expectedMachinePoolWorkerCount
		}
	}

	byClusterOptions := []client.ListOption{
		client.InNamespace(unstructuredCluster.GetNamespace()),
		client.MatchingLabels{clusterv1.ClusterNameLabel: unstructuredCluster.GetName()},
	}

	machinePoolList := &unstructured.UnstructuredList{}
	machinePoolGroup := clusterv1.GroupVersion.Group
	if coreCAPIStorageVersion == "v1alpha3" {
		machinePoolGroup = "exp.cluster.x-k8s.io"
	}
	machinePoolList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   machinePoolGroup,
		Version: coreCAPIStorageVersion,
		Kind:    "MachinePoolList",
	})
	Eventually(func() error {
		return c.List(ctx, machinePoolList, byClusterOptions...)
	}, 3*time.Minute, 3*time.Second).Should(Succeed(), "Failed to list MachinePool object for Cluster %s", klog.KObj(unstructuredCluster))
	for _, mp := range machinePoolList.Items {
		replicas, ok, err := unstructured.NestedInt64(mp.Object, "spec", "replicas")
		Expect(err).ToNot(HaveOccurred())
		if !ok {
			continue
		}
		expectedMachinePoolWorkerCount += replicas
	}

	return expectedMachinePoolWorkerCount
}

// deleteAllClustersAndWaitInput is the input type for deleteAllClustersAndWait.
type deleteAllClustersAndWaitInput struct {
	Client    client.Client
	Namespace string
}

// deleteAllClustersAndWait deletes all cluster resources in the given namespace and waits for them to be gone.
func deleteAllClustersAndWait(ctx context.Context, input deleteAllClustersAndWaitInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for deleteAllClustersAndWaitOldAPI")
	Expect(input.Client).ToNot(BeNil(), "Invalid argument. input.Client can't be nil when calling deleteAllClustersAndWaitOldAPI")
	Expect(input.Namespace).ToNot(BeEmpty(), "Invalid argument. input.Namespace can't be empty when calling deleteAllClustersAndWaitOldAPI")

	coreCAPIStorageVersion := getCoreCAPIStorageVersion(ctx, input.Client)

	clusterList := &unstructured.UnstructuredList{}
	clusterList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   clusterv1.GroupVersion.Group,
		Version: coreCAPIStorageVersion,
		Kind:    "ClusterList",
	})
	Expect(input.Client.List(ctx, clusterList, client.InNamespace(input.Namespace))).To(Succeed(), "Failed to list clusters in namespace %s", input.Namespace)

	// Enforce restart of kube-controller-manager by stealing its lease.
	// Note: Due to a known issue in the kube-controller-manager we have to restart it
	// in case the kube-controller-manager internally caches ownerRefs of apiVersions
	// which now don't exist anymore (e.g. v1alpha3/v1alpha4).
	// Alternatives to this would be:
	// * some other way to restart the kube-controller-manager (e.g. control plane node rollout)
	// * removing ownerRefs from (at least) MachineDeployments
	Eventually(func(g Gomega) {
		kubeControllerManagerLease := &coordinationv1.Lease{}
		g.Expect(input.Client.Get(ctx, client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: "kube-controller-manager"}, kubeControllerManagerLease)).To(Succeed())
		// As soon as the kube-controller-manager detects it doesn't own the lease anymore it will restart.
		// Once the current lease times out the kube-controller-manager will become leader again.
		kubeControllerManagerLease.Spec.HolderIdentity = ptr.To("e2e-test-client")
		g.Expect(input.Client.Update(ctx, kubeControllerManagerLease)).To(Succeed())
	}, 3*time.Minute, 3*time.Second).Should(Succeed(), "failed to steal lease from kube-controller-manager to trigger restart")

	for _, c := range clusterList.Items {
		Byf("Deleting cluster %s", c.GetName())
		Expect(input.Client.Delete(ctx, c.DeepCopy())).To(Succeed())
	}

	for _, c := range clusterList.Items {
		Byf("Waiting for cluster %s to be deleted", c.GetName())
		Eventually(func() bool {
			cluster := c.DeepCopy()
			key := client.ObjectKey{
				Namespace: c.GetNamespace(),
				Name:      c.GetName(),
			}
			return apierrors.IsNotFound(input.Client.Get(ctx, key, cluster))
		}, intervals...).Should(BeTrue())
	}
}

// validateMachineRollout compares preMachineList and postMachineList to detect a rollout.
// Note: we are using unstructured lists because the Machines have different apiVersions.
func validateMachineRollout(preMachineList, postMachineList *unstructured.UnstructuredList) bool {
	if preMachineList == nil && postMachineList == nil {
		return true
	}
	if preMachineList == nil || postMachineList == nil {
		return false
	}

	if names(preMachineList).Equal(names(postMachineList)) {
		return true
	}

	log.Logf("Rollout detected (%d Machines before, %d Machines after)", len(preMachineList.Items), len(postMachineList.Items))
	newMachines := names(postMachineList).Difference(names(preMachineList))
	deletedMachines := names(preMachineList).Difference(names(postMachineList))

	if len(newMachines) > 0 {
		log.Logf("Detected new Machines")
		for _, obj := range postMachineList.Items {
			if newMachines.Has(obj.GetName()) {
				resourceYAML, err := yaml.Marshal(obj)
				Expect(err).ToNot(HaveOccurred())
				log.Logf("New Machine %s:\n%s", klog.KObj(&obj), resourceYAML)
			}
		}
	}

	if len(deletedMachines) > 0 {
		log.Logf("Detected deleted Machines")
		for _, obj := range preMachineList.Items {
			if deletedMachines.Has(obj.GetName()) {
				resourceYAML, err := yaml.Marshal(obj)
				Expect(err).ToNot(HaveOccurred())
				log.Logf("Deleted Machine %s:\n%s", klog.KObj(&obj), resourceYAML)
			}
		}
	}
	return false
}

func names(objs *unstructured.UnstructuredList) sets.Set[string] {
	ret := sets.Set[string]{}
	for _, obj := range objs.Items {
		ret.Insert(obj.GetName())
	}
	return ret
}

// getValueOrFallback returns the input value unless it is empty, then it returns the fallback input.
func getValueOrFallback(value []string, fallback []string) []string {
	if value != nil {
		return value
	}
	return fallback
}
