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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/discovery"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

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
	// InitWithKubernetesVersion can be used to override the INIT_WITH_KUBERNETES_VERSION e2e config variable with a specific
	// Kubernetes version to use to create the secondary management cluster, e.g. `v1.25.0`
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
	// multiple infrastructure providers (ex: CAPD + in-memory) are installed on the cluster as clusterctl will not be
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
	// Custom providers can be specified to upgrade to a pre-release or a custom version instead of upgrading to the latest using contact
	CoreProvider              string
	BootstrapProviders        []string
	ControlPlaneProviders     []string
	InfrastructureProviders   []string
	IPAMProviders             []string
	RuntimeExtensionProviders []string
	AddonProviders            []string
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

		clusterctlBinaryURLTemplate := input.InitWithBinary
		if clusterctlBinaryURLTemplate == "" {
			Expect(input.E2EConfig.ResolveReleases(ctx)).To(Succeed(), "Failed to resolve release markers in e2e test config file")
			Expect(input.E2EConfig.Variables).To(HaveKey(initWithBinaryVariableName), "Invalid argument. %s variable must be defined when calling %s spec", initWithBinaryVariableName, specName)
			Expect(input.E2EConfig.Variables[initWithBinaryVariableName]).ToNot(BeEmpty(), "Invalid argument. %s variable can't be empty when calling %s spec", initWithBinaryVariableName, specName)
			clusterctlBinaryURLTemplate = input.E2EConfig.GetVariable(initWithBinaryVariableName)
		}
		clusterctlBinaryURLReplacer := strings.NewReplacer("{OS}", runtime.GOOS, "{ARCH}", runtime.GOARCH)
		initClusterctlBinaryURL = clusterctlBinaryURLReplacer.Replace(clusterctlBinaryURLTemplate)

		// NOTE: by default we are considering all the providers, no matter of the contract.
		// However, given that we want to test both v1alpha3 --> v1beta1 and v1alpha4 --> v1beta1, the INIT_WITH_PROVIDERS_CONTRACT
		// variable can be used to select versions with a specific contract.
		initContract = "*"
		if input.E2EConfig.HasVariable(initWithProvidersContract) {
			initContract = input.E2EConfig.GetVariable(initWithProvidersContract)
		}
		if input.InitWithProvidersContract != "" {
			initContract = input.InitWithProvidersContract
		}

		initKubernetesVersion = input.InitWithKubernetesVersion
		if initKubernetesVersion == "" {
			Expect(input.E2EConfig.Variables).To(HaveKey(initWithKubernetesVersion), "Invalid argument. %s variable must be defined when calling %s spec", initWithKubernetesVersion, specName)
			Expect(input.E2EConfig.Variables[initWithKubernetesVersion]).ToNot(BeEmpty(), "Invalid argument. %s variable can't be empty when calling %s spec", initWithKubernetesVersion, specName)
			initKubernetesVersion = input.E2EConfig.GetVariable(initWithKubernetesVersion)
		}

		Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersion))
		Expect(os.MkdirAll(input.ArtifactFolder, 0750)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		managementClusterNamespace, managementClusterCancelWatches = setupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, input.PostNamespaceCreated)
		managementClusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should create a management cluster and then upgrade all the providers", func() {
		By("Creating a workload cluster to be used as a new management cluster")
		// NOTE: given that the bootstrap cluster could be shared by several tests, it is not practical to use it for testing clusterctl upgrades.
		// So we are creating a workload cluster that will be used as a new management cluster where to install older version of providers
		infrastructureProvider := clusterctl.DefaultInfrastructureProvider
		if input.InfrastructureProvider != nil {
			infrastructureProvider = *input.InfrastructureProvider
		}
		managementClusterName = fmt.Sprintf("%s-%s", specName, util.RandomString(6))
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
		managementClusterProxy = input.BootstrapClusterProxy.GetWorkloadCluster(ctx, cluster.Namespace, cluster.Name, framework.WithMachineLogCollector(input.BootstrapClusterProxy.GetLogCollector()))

		// Download the older clusterctl version to be used for setting up the management cluster to be upgraded
		log.Logf("Downloading clusterctl binary from %s", initClusterctlBinaryURL)
		clusterctlBinaryPath := downloadToTmpFile(ctx, initClusterctlBinaryURL)
		defer os.Remove(clusterctlBinaryPath) // clean up

		err := os.Chmod(clusterctlBinaryPath, 0744) //nolint:gosec
		Expect(err).ToNot(HaveOccurred(), "failed to chmod temporary file")

		// Adjusts the clusterctlConfigPath in case the clusterctl version <= v1.3 (thus using a config file with only the providers supported in those versions)
		clusterctlConfigPath := clusterctl.AdjustConfigPathForBinary(clusterctlBinaryPath, input.ClusterctlConfigPath)

		By("Initializing the workload cluster with older versions of providers")

		if input.PreInit != nil {
			By("Running Pre-init steps against the management cluster")
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
			LogFolder:                 filepath.Join(input.ArtifactFolder, "clusters", cluster.Name),
		}, input.E2EConfig.GetIntervals(specName, "wait-controllers")...)

		By("THE MANAGEMENT CLUSTER WITH THE OLDER VERSION OF PROVIDERS IS UP&RUNNING!")

		machineCRD := &apiextensionsv1.CustomResourceDefinition{}
		if err := managementClusterProxy.GetClient().Get(ctx, client.ObjectKey{Name: "machines.cluster.x-k8s.io"}, machineCRD); err != nil {
			Expect(err).ToNot(HaveOccurred(), "failed to retrieve a machine CRD")
		}

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

		workloadClusterName = fmt.Sprintf("%s-%s", specName, util.RandomString(6))
		kubernetesVersion := input.WorkloadKubernetesVersion
		if kubernetesVersion == "" {
			kubernetesVersion = input.E2EConfig.GetVariable(KubernetesVersion)
		}
		controlPlaneMachineCount := ptr.To[int64](1)
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
			Namespace:                testNamespace.Name,
			ClusterName:              workloadClusterName,
			KubernetesVersion:        kubernetesVersion,
			ControlPlaneMachineCount: controlPlaneMachineCount,
			WorkerMachineCount:       workerMachineCount,
			InfrastructureProvider:   infrastructureProvider,
			// setup clusterctl logs folder
			LogFolder: filepath.Join(input.ArtifactFolder, "clusters", managementClusterProxy.GetName()),
		})
		Expect(workloadClusterTemplate).ToNot(BeNil(), "Failed to get the cluster template")

		log.Logf("Applying the cluster template yaml to the cluster")
		Expect(managementClusterProxy.Apply(ctx, workloadClusterTemplate)).To(Succeed())

		if input.PreWaitForCluster != nil {
			By("Running PreWaitForCluster steps against the management cluster")
			input.PreWaitForCluster(managementClusterProxy, testNamespace.Name, workloadClusterName)
		}

		workloadCluster := framework.DiscoveryAndWaitForCluster(ctx, framework.DiscoveryAndWaitForClusterInput{
			Getter:    managementClusterProxy.GetClient(),
			Namespace: testNamespace.Name,
			Name:      workloadClusterName,
		}, input.E2EConfig.GetIntervals(specName, "wait-cluster")...)

		expectedMachineCount := *controlPlaneMachineCount + calculateExpectedWorkerCount(ctx, managementClusterProxy.GetClient(), input.E2EConfig, specName, workloadCluster)

		// Build GroupVersionKind for Machine resources
		machineListGVK := schema.GroupVersionKind{
			Group: machineCRD.Spec.Group,
			Kind:  machineCRD.Spec.Names.ListKind,
		}

		// Pick the storage version
		for _, version := range machineCRD.Spec.Versions {
			if version.Storage {
				machineListGVK.Version = version.Name
				break
			}
		}

		By("Waiting for the machines to exist")
		Eventually(func() (int64, error) {
			var n int64
			machineList := &unstructured.UnstructuredList{}
			machineList.SetGroupVersionKind(machineListGVK)
			if err := managementClusterProxy.GetClient().List(
				ctx,
				machineList,
				client.InNamespace(workloadCluster.Namespace),
				client.MatchingLabels{clusterv1.ClusterNameLabel: workloadCluster.Name},
			); err == nil {
				for _, m := range machineList.Items {
					_, found, err := unstructured.NestedMap(m.Object, "status", "nodeRef")
					if err == nil && found {
						n++
					}
				}
			}
			return n, nil
		}, input.E2EConfig.GetIntervals(specName, "wait-worker-nodes")...).Should(Equal(expectedMachineCount), "Timed out waiting for all machines to be exist")

		By("THE MANAGEMENT CLUSTER WITH OLDER VERSION OF PROVIDERS WORKS!")

		if input.PreUpgrade != nil {
			By("Running Pre-upgrade steps against the management cluster")
			input.PreUpgrade(managementClusterProxy)
		}

		// Get the workloadCluster before the management cluster is upgraded to make sure that the upgrade did not trigger
		// any unexpected rollouts.
		preUpgradeMachineList := &unstructured.UnstructuredList{}
		preUpgradeMachineList.SetGroupVersionKind(machineListGVK)
		err = managementClusterProxy.GetClient().List(
			ctx,
			preUpgradeMachineList,
			client.InNamespace(workloadCluster.Namespace),
			client.MatchingLabels{clusterv1.ClusterNameLabel: workloadCluster.Name},
		)
		Expect(err).ToNot(HaveOccurred())
		// Check if the user want a custom upgrade
		isCustomUpgrade := input.CoreProvider != "" ||
			len(input.BootstrapProviders) > 0 ||
			len(input.ControlPlaneProviders) > 0 ||
			len(input.InfrastructureProviders) > 0 ||
			len(input.IPAMProviders) > 0 ||
			len(input.RuntimeExtensionProviders) > 0 ||
			len(input.AddonProviders) > 0

		if isCustomUpgrade {
			By("Upgrading providers to custom versions")
			clusterctl.UpgradeManagementClusterAndWait(ctx, clusterctl.UpgradeManagementClusterAndWaitInput{
				ClusterctlConfigPath:      input.ClusterctlConfigPath,
				ClusterctlVariables:       input.UpgradeClusterctlVariables,
				ClusterProxy:              managementClusterProxy,
				CoreProvider:              input.CoreProvider,
				BootstrapProviders:        input.BootstrapProviders,
				ControlPlaneProviders:     input.ControlPlaneProviders,
				InfrastructureProviders:   input.InfrastructureProviders,
				IPAMProviders:             input.IPAMProviders,
				RuntimeExtensionProviders: input.RuntimeExtensionProviders,
				AddonProviders:            input.AddonProviders,
				LogFolder:                 filepath.Join(input.ArtifactFolder, "clusters", cluster.Name),
			}, input.E2EConfig.GetIntervals(specName, "wait-controllers")...)
		} else {
			By("Upgrading providers to the latest version available")
			clusterctl.UpgradeManagementClusterAndWait(ctx, clusterctl.UpgradeManagementClusterAndWaitInput{
				ClusterctlConfigPath: input.ClusterctlConfigPath,
				ClusterctlVariables:  input.UpgradeClusterctlVariables,
				ClusterProxy:         managementClusterProxy,
				Contract:             clusterv1.GroupVersion.Version,
				LogFolder:            filepath.Join(input.ArtifactFolder, "clusters", cluster.Name),
			}, input.E2EConfig.GetIntervals(specName, "wait-controllers")...)
		}

		By("THE MANAGEMENT CLUSTER WAS SUCCESSFULLY UPGRADED!")

		if input.PostUpgrade != nil {
			By("Running Post-upgrade steps against the management cluster")
			input.PostUpgrade(managementClusterProxy, testNamespace.Name, managementClusterName)
		}

		// After the upgrade check that there were no unexpected rollouts.
		log.Logf("Verify there are no unexpected rollouts")
		Consistently(func() bool {
			postUpgradeMachineList := &unstructured.UnstructuredList{}
			postUpgradeMachineList.SetGroupVersionKind(clusterv1.GroupVersion.WithKind("MachineList"))
			err = managementClusterProxy.GetClient().List(
				ctx,
				postUpgradeMachineList,
				client.InNamespace(workloadCluster.Namespace),
				client.MatchingLabels{clusterv1.ClusterNameLabel: workloadCluster.Name},
			)
			Expect(err).ToNot(HaveOccurred())
			return validateMachineRollout(preUpgradeMachineList, postUpgradeMachineList)
		}, "3m", "30s").Should(BeTrue(), "Machines should remain the same after the upgrade")

		if workloadCluster.Spec.Topology != nil {
			// Cluster is using ClusterClass, scale up via topology.
			framework.ScaleAndWaitMachineDeploymentTopology(ctx, framework.ScaleAndWaitMachineDeploymentTopologyInput{
				ClusterProxy:              managementClusterProxy,
				Cluster:                   workloadCluster,
				Replicas:                  2,
				WaitForMachineDeployments: input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
			})
		} else {
			// Cluster is not using ClusterClass, scale up via MachineDeployment.
			testMachineDeployments := framework.GetMachineDeploymentsByCluster(ctx, framework.GetMachineDeploymentsByClusterInput{
				Lister:      managementClusterProxy.GetClient(),
				ClusterName: workloadCluster.Name,
				Namespace:   workloadCluster.Namespace,
			})
			framework.ScaleAndWaitMachineDeployment(ctx, framework.ScaleAndWaitMachineDeploymentInput{
				ClusterProxy:              managementClusterProxy,
				Cluster:                   workloadCluster,
				MachineDeployment:         testMachineDeployments[0],
				Replicas:                  2,
				WaitForMachineDeployments: input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
			})
		}

		By("THE UPGRADED MANAGEMENT CLUSTER WORKS!")

		By("PASSED!")
	})

	AfterEach(func() {
		if testNamespace != nil {
			// Dump all the logs from the workload cluster before deleting them.
			dumpAllResources(ctx, managementClusterProxy, input.ArtifactFolder, testNamespace, managementClusterResources.Cluster)

			if !input.SkipCleanup {
				switch {
				case discovery.ServerSupportsVersion(managementClusterProxy.GetClientSet().DiscoveryClient, clusterv1.GroupVersion) == nil:
					Byf("Deleting all %s clusters in namespace %s in management cluster %s", clusterv1.GroupVersion, testNamespace.Name, managementClusterName)
					framework.DeleteAllClustersAndWait(ctx, framework.DeleteAllClustersAndWaitInput{
						Client:    managementClusterProxy.GetClient(),
						Namespace: testNamespace.Name,
					}, input.E2EConfig.GetIntervals(specName, "wait-delete-cluster")...)
				default:
					log.Logf("Management Cluster does not appear to support CAPI resources.")
				}

				Byf("Deleting cluster %s", klog.KRef(testNamespace.Name, managementClusterName))
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

		if input.PreCleanupManagementCluster != nil {
			By("Running PreCleanupManagementCluster steps against the management cluster")
			input.PreCleanupManagementCluster(managementClusterProxy)
		}
		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		dumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, managementClusterNamespace, managementClusterCancelWatches, managementClusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
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

func calculateExpectedWorkerCount(ctx context.Context, c client.Client, e2eConfig *clusterctl.E2EConfig, specName string, cluster *clusterv1.Cluster) int64 {
	var expectedWorkerCount int64

	if cluster.Spec.Topology != nil {
		if cluster.Spec.Topology.Workers != nil {
			for _, md := range cluster.Spec.Topology.Workers.MachineDeployments {
				if md.Replicas == nil {
					continue
				}
				expectedWorkerCount += int64(*md.Replicas)
			}
			for _, mp := range cluster.Spec.Topology.Workers.MachinePools {
				if mp.Replicas == nil {
					continue
				}
				expectedWorkerCount += int64(*mp.Replicas)
			}
		}
		return expectedWorkerCount
	}

	machineDeployments := framework.DiscoveryAndWaitForMachineDeployments(ctx, framework.DiscoveryAndWaitForMachineDeploymentsInput{
		Lister:  c,
		Cluster: cluster,
	}, e2eConfig.GetIntervals(specName, "wait-worker-nodes")...)
	for _, md := range machineDeployments {
		if md.Spec.Replicas == nil {
			continue
		}
		expectedWorkerCount += int64(*md.Spec.Replicas)
	}

	machinePools := framework.DiscoveryAndWaitForMachinePools(ctx, framework.DiscoveryAndWaitForMachinePoolsInput{
		Getter:  c,
		Lister:  c,
		Cluster: cluster,
	}, e2eConfig.GetIntervals(specName, "wait-machine-pool-nodes")...)
	for _, mp := range machinePools {
		if mp.Spec.Replicas == nil {
			continue
		}
		expectedWorkerCount += int64(*mp.Spec.Replicas)
	}
	return expectedWorkerCount
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
			obj := obj
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
			obj := obj
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
