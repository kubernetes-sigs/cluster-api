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
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/e2e/internal/log"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util/yaml"
)

const (
	scaleClusterCount             = "CAPI_SCALE_CLUSTER_COUNT"
	scaleConcurrency              = "CAPI_SCALE_CONCURRENCY"
	scaleControlPlaneMachineCount = "CAPI_SCALE_CONTROL_PLANE_MACHINE_COUNT"
	scaleWorkerMachineCount       = "CAPI_SCALE_WORKER_MACHINE_COUNT"
	scaleMachineDeploymentCount   = "CAPI_SCALE_MACHINE_DEPLOYMENT_COUNT"

	// Note: Names must consist of lower case alphanumeric characters or '-'.
	scaleClusterNamePlaceholder      = "scale-cluster-name-placeholder"
	scaleClusterNamespacePlaceholder = "scale-cluster-namespace-placeholder"
)

// scaleSpecInput is the input for scaleSpec.
type scaleSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string

	// InfrastructureProviders specifies the infrastructure to use for clusterctl
	// operations (Example: get cluster templates).
	// Note: In most cases this need not be specified. It only needs to be specified when
	// multiple infrastructure providers (ex: CAPD + in-memory) are installed on the cluster as clusterctl will not be
	// able to identify the default.
	InfrastructureProvider *string

	// Flavor, if specified is the template flavor used to create the cluster for testing.
	// If not specified, the default flavor for the selected infrastructure provider is used.
	// The ClusterTopology of this flavor should have exactly one MachineDeployment.
	Flavor *string

	// ClusterCount is the number of target workload clusters.
	// If unspecified, defaults to 10.
	// Can be overridden by variable CAPI_SCALE_CLUSTER_COUNT.
	ClusterCount *int64

	// DeployClusterInSeparateNamespaces defines if each cluster should be deployed into its separate namespace.
	// In this case The namespace name will be the name of the cluster.
	DeployClusterInSeparateNamespaces bool

	// Concurrency is the maximum concurrency of each of the scale operations.
	// If unspecified it defaults to 5.
	// Can be overridden by variable CAPI_SCALE_CONCURRENCY.
	Concurrency *int64

	// ControlPlaneMachineCount defines the number of control plane machines to be added to each workload cluster.
	// If not specified, 1 will be used.
	// Can be overridden by variable CAPI_SCALE_CONTROLPLANE_MACHINE_COUNT.
	ControlPlaneMachineCount *int64

	// WorkerMachineCount defines number of worker machines per machine deployment of the workload cluster.
	// If not specified, 1 will be used.
	// Can be overridden by variable CAPI_SCALE_WORKER_MACHINE_COUNT.
	// The resulting number of worker nodes for each of the workload cluster will
	// be MachineDeploymentCount*WorkerMachineCount (CAPI_SCALE_MACHINE_DEPLOYMENT_COUNT*CAPI_SCALE_WORKER_MACHINE_COUNT).
	WorkerMachineCount *int64

	// MachineDeploymentCount defines the number of MachineDeployments to be used per workload cluster.
	// If not specified, 1 will be used.
	// Can be overridden by variable CAPI_SCALE_MACHINE_DEPLOYMENT_COUNT.
	// Note: This assumes that the cluster template of the specified flavor has exactly one machine deployment.
	// It uses this machine deployment to create additional copies.
	// Names of the MachineDeployments will be overridden to "md-1", "md-2", etc.
	// The resulting number of worker nodes for each of the workload cluster will
	// be MachineDeploymentCount*WorkerMachineCount (CAPI_SCALE_MACHINE_DEPLOYMENT_COUNT*CAPI_SCALE_WORKER_MACHINE_COUNT).
	MachineDeploymentCount *int64

	// Allows to inject a function to be run after test namespace is created.
	// If not specified, this is a no-op.
	PostNamespaceCreated func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace string)

	// FailFast if set to true will return immediately after the first cluster operation fails.
	// If set to false, the test suite will not exit immediately after the first cluster operation fails.
	// Example: When creating clusters from c1 to c20 consider c6 fails creation. If FailFast is set to true
	// the suit will exit immediately after receiving the c6 creation error. If set to false, cluster creations
	// of the other clusters will continue and all the errors are collected before the test exists.
	// Note: Please note that the test suit will still fail since c6 creation failed. FailFast will determine
	// if the test suit should fail as soon as c6 fails or if it should fail after all cluster creations are done.
	FailFast bool

	// SkipUpgrade if set to true will skip upgrading the workload clusters.
	SkipUpgrade bool

	// SkipCleanup if set to true will skip deleting the workload clusters.
	SkipCleanup bool

	// SkipWaitForCreation defines if the test should wait for the workload clusters to be fully provisioned
	// before moving on.
	// If set to true, the test will create the workload clusters and immediately continue without waiting
	// for the clusters to be fully provisioned.
	SkipWaitForCreation bool
}

// scaleSpec implements a scale test for clusters with MachineDeployments.
func scaleSpec(ctx context.Context, inputGetter func() scaleSpecInput) {
	var (
		specName      = "scale"
		input         scaleSpecInput
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
	)

	BeforeEach(func() {
		Expect(ctx).NotTo(BeNil(), "ctx is required for %s spec", specName)
		input = inputGetter()
		Expect(input.E2EConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. input.BootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0750)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersion))

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		// We are pinning the namespace for the test to help with debugging and testing.
		// Example: Queries to look up state of the clusters can be re-used.
		// Since we don't run multiple instances of this test concurrently on a management cluster it is okay to pin the namespace.
		Byf("Creating a namespace for hosting the %q test spec", specName)
		namespace, cancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:             input.BootstrapClusterProxy.GetClient(),
			ClientSet:           input.BootstrapClusterProxy.GetClientSet(),
			Name:                specName,
			LogFolder:           filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
			IgnoreAlreadyExists: true,
		})

		if input.PostNamespaceCreated != nil {
			log.Logf("Calling postNamespaceCreated for namespace %s", namespace.Name)
			input.PostNamespaceCreated(input.BootstrapClusterProxy, namespace.Name)
		}
	})

	It("Should create and delete workload clusters", func() {
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
		// If variable is defined that will take precedence.
		if input.E2EConfig.HasVariable(scaleControlPlaneMachineCount) {
			controlPlaneMachineCountStr := input.E2EConfig.GetVariable(scaleControlPlaneMachineCount)
			controlPlaneMachineCountInt, err := strconv.Atoi(controlPlaneMachineCountStr)
			Expect(err).ToNot(HaveOccurred())
			controlPlaneMachineCount = ptr.To[int64](int64(controlPlaneMachineCountInt))
		}

		workerMachineCount := ptr.To[int64](1)
		if input.WorkerMachineCount != nil {
			workerMachineCount = input.WorkerMachineCount
		}
		// If variable is defined that will take precedence.
		if input.E2EConfig.HasVariable(scaleWorkerMachineCount) {
			workerMachineCountStr := input.E2EConfig.GetVariable(scaleWorkerMachineCount)
			workerMachineCountInt, err := strconv.Atoi(workerMachineCountStr)
			Expect(err).ToNot(HaveOccurred())
			workerMachineCount = ptr.To[int64](int64(workerMachineCountInt))
		}

		machineDeploymentCount := ptr.To[int64](1)
		if input.MachineDeploymentCount != nil {
			machineDeploymentCount = input.MachineDeploymentCount
		}
		// If variable is defined that will take precedence.
		if input.E2EConfig.HasVariable(scaleMachineDeploymentCount) {
			machineDeploymentCountStr := input.E2EConfig.GetVariable(scaleMachineDeploymentCount)
			machineDeploymentCountInt, err := strconv.Atoi(machineDeploymentCountStr)
			Expect(err).ToNot(HaveOccurred())
			machineDeploymentCount = ptr.To[int64](int64(machineDeploymentCountInt))
		}

		clusterCount := int64(10)
		if input.ClusterCount != nil {
			clusterCount = *input.ClusterCount
		}
		// If variable is defined that will take precedence.
		if input.E2EConfig.HasVariable(scaleClusterCount) {
			clusterCountStr := input.E2EConfig.GetVariable(scaleClusterCount)
			var err error
			clusterCount, err = strconv.ParseInt(clusterCountStr, 10, 64)
			Expect(err).NotTo(HaveOccurred(), "%q value should be integer", scaleClusterCount)
		}

		concurrency := int64(5)
		if input.Concurrency != nil {
			concurrency = *input.Concurrency
		}
		// If variable is defined that will take precedence.
		if input.E2EConfig.HasVariable(scaleConcurrency) {
			concurrencyStr := input.E2EConfig.GetVariable(scaleConcurrency)
			var err error
			concurrency, err = strconv.ParseInt(concurrencyStr, 10, 64)
			Expect(err).NotTo(HaveOccurred(), "%q value should be integer", scaleConcurrency)
		}

		// TODO(ykakarap): Follow-up: Add support for legacy cluster templates.

		By("Create the ClusterClass to be used by all workload clusters")

		// IMPORTANT: ConfigCluster function in the test framework is currently not concurrency safe.
		// Therefore, it is not advised to call this functions across all the concurrency workers.
		// To avoid this problem we chose to run ConfigCluster once and reuse its output across all the workers.
		log.Logf("Generating YAML for base Cluster and ClusterClass")
		baseWorkloadClusterTemplate := clusterctl.ConfigCluster(ctx, clusterctl.ConfigClusterInput{
			LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
			ClusterctlConfigPath:     input.ClusterctlConfigPath,
			KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
			InfrastructureProvider:   infrastructureProvider,
			Flavor:                   flavor,
			Namespace:                scaleClusterNamespacePlaceholder,
			ClusterName:              scaleClusterNamePlaceholder,
			KubernetesVersion:        input.E2EConfig.GetVariable(KubernetesVersionUpgradeFrom),
			ControlPlaneMachineCount: controlPlaneMachineCount,
			WorkerMachineCount:       workerMachineCount,
		})
		Expect(baseWorkloadClusterTemplate).ToNot(BeNil(), "Failed to get the cluster template")

		// Separate the Cluster YAML and the ClusterClass YAML so that we can apply the ClusterCLass ahead of time
		// to avoid race conditions while applying the ClusterClass when trying to create multiple clusters concurrently.
		// Nb. Apply function in the test framework uses `kubectl apply` internally. `kubectl apply` detects
		// if the resource has to be created or updated before actually executing the operation. If another worker changes
		// the status of the cluster during this timeframe the operation will fail.
		log.Logf("Extract ClusterClass and Cluster from template YAML")
		baseClusterClassYAML, baseClusterTemplateYAML := extractClusterClassAndClusterFromTemplate(baseWorkloadClusterTemplate)

		// Modify the baseClusterTemplateYAML so that it has the desired number of machine deployments.
		baseClusterTemplateYAML = modifyMachineDeployments(baseClusterTemplateYAML, int(*machineDeploymentCount))

		// If all clusters should be deployed in the same namespace (namespace.Name),
		// then deploy the ClusterClass in this namespace.
		if !input.DeployClusterInSeparateNamespaces {
			if len(baseClusterClassYAML) > 0 {
				clusterClassYAML := bytes.Replace(baseClusterClassYAML, []byte(scaleClusterNamespacePlaceholder), []byte(namespace.Name), -1)
				log.Logf("Apply ClusterClass")
				Eventually(func() error {
					return input.BootstrapClusterProxy.CreateOrUpdate(ctx, clusterClassYAML)
				}, 1*time.Minute).Should(Succeed())
			} else {
				log.Logf("ClusterClass already exists. Skipping creation.")
			}
		}

		By("Create workload clusters concurrently")
		// Create multiple clusters concurrently from the same base cluster template.

		clusterNames := make([]string, 0, clusterCount)
		clusterNameDigits := 1 + int(math.Log10(float64(clusterCount)))
		for i := int64(1); i <= clusterCount; i++ {
			// This ensures we always have the right number of leading zeros in our cluster names, e.g.
			// clusterCount=1000 will lead to cluster names like scale-0001, scale-0002, ... .
			// This makes it possible to have correct ordering of clusters in diagrams in tools like Grafana.
			name := fmt.Sprintf("%s-%0*d", specName, clusterNameDigits, i)
			clusterNames = append(clusterNames, name)
		}

		// Use the correct creator function for creating the workload clusters.
		// Default to using the "create and wait" creator function. If SkipWaitForCreation=true then
		// use the "create only" creator function.
		creator := getClusterCreateAndWaitFn(clusterctl.ApplyCustomClusterTemplateAndWaitInput{
			ClusterProxy:                 input.BootstrapClusterProxy,
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		})
		if input.SkipWaitForCreation {
			if !input.SkipCleanup {
				log.Logf("WARNING! Using SkipWaitForCreation=true while SkipCleanup=false can lead to workload clusters getting deleted before they are fully provisioned.")
			}
			creator = getClusterCreateFn(input.BootstrapClusterProxy)
		}

		clusterCreateResults, err := workConcurrentlyAndWait(ctx, workConcurrentlyAndWaitInput{
			ClusterNames: clusterNames,
			Concurrency:  concurrency,
			FailFast:     input.FailFast,
			WorkerFunc: func(ctx context.Context, inputChan chan string, resultChan chan workResult, wg *sync.WaitGroup) {
				createClusterWorker(ctx, input.BootstrapClusterProxy, inputChan, resultChan, wg, namespace.Name, input.DeployClusterInSeparateNamespaces, baseClusterClassYAML, baseClusterTemplateYAML, creator)
			},
		})
		if err != nil {
			// Call Fail to notify ginkgo that the suit has failed.
			// Ginkgo will print the first observed error failure in this case.
			// Example: If cluster c1, c2 and c3 failed then ginkgo will only print the first
			// observed failure among the these 3 clusters.
			// Since ginkgo only captures one failure, to help with this we are logging the error
			// that will contain the full stack trace of failure for each cluster to help with debugging.
			// TODO(ykakarap): Follow-up: Explore options for improved error reporting.
			log.Logf("Failed to create clusters. Error: %s", err.Error())
			Fail("")
		}

		if !input.SkipUpgrade {
			By("Upgrade the workload clusters concurrently")
			// Get the upgrade function for upgrading the workload clusters.
			upgrader := getClusterUpgradeAndWaitFn(framework.UpgradeClusterTopologyAndWaitForUpgradeInput{
				ClusterProxy:                input.BootstrapClusterProxy,
				KubernetesUpgradeVersion:    input.E2EConfig.GetVariable(KubernetesVersionUpgradeTo),
				EtcdImageTag:                input.E2EConfig.GetVariable(EtcdVersionUpgradeTo),
				DNSImageTag:                 input.E2EConfig.GetVariable(CoreDNSVersionUpgradeTo),
				WaitForMachinesToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
				WaitForKubeProxyUpgrade:     input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
				WaitForDNSUpgrade:           input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
				WaitForEtcdUpgrade:          input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			})

			clusterNamesToUpgrade := []string{}
			for _, result := range clusterCreateResults {
				clusterNamesToUpgrade = append(clusterNamesToUpgrade, result.clusterName)
			}

			// Upgrade all the workload clusters.
			_, err = workConcurrentlyAndWait(ctx, workConcurrentlyAndWaitInput{
				ClusterNames: clusterNamesToUpgrade,
				Concurrency:  concurrency,
				FailFast:     input.FailFast,
				WorkerFunc: func(ctx context.Context, inputChan chan string, resultChan chan workResult, wg *sync.WaitGroup) {
					upgradeClusterAndWaitWorker(ctx, inputChan, resultChan, wg, namespace.Name, input.DeployClusterInSeparateNamespaces, baseClusterTemplateYAML, upgrader)
				},
			})
			if err != nil {
				// Call Fail to notify ginkgo that the suit has failed.
				// Ginkgo will print the first observed error failure in this case.
				// Example: If cluster c1, c2 and c3 failed then ginkgo will only print the first
				// observed failure among the these 3 clusters.
				// Since ginkgo only captures one failure, to help with this we are logging the error
				// that will contain the full stack trace of failure for each cluster to help with debugging.
				// TODO(ykakarap): Follow-up: Explore options for improved error reporting.
				log.Logf("Failed to upgrade clusters. Error: %s", err.Error())
				Fail("")
			}
		}

		// TODO(ykakarap): Follow-up: Dump resources for the failed clusters (creation).

		clusterNamesToDelete := []string{}
		for _, result := range clusterCreateResults {
			clusterNamesToDelete = append(clusterNamesToDelete, result.clusterName)
		}

		if input.SkipCleanup {
			return
		}

		By("Delete the workload clusters concurrently")
		// Now delete all the workload clusters.
		_, err = workConcurrentlyAndWait(ctx, workConcurrentlyAndWaitInput{
			ClusterNames: clusterNamesToDelete,
			Concurrency:  concurrency,
			FailFast:     input.FailFast,
			WorkerFunc: func(ctx context.Context, inputChan chan string, resultChan chan workResult, wg *sync.WaitGroup) {
				deleteClusterAndWaitWorker(ctx, inputChan, resultChan, wg, input.BootstrapClusterProxy.GetClient(), namespace.Name, input.DeployClusterInSeparateNamespaces)
			},
		})
		if err != nil {
			// Call Fail to notify ginkgo that the suit has failed.
			// Ginkgo will print the first observed error failure in this case.
			// Example: If cluster c1, c2 and c3 failed then ginkgo will only print the first
			// observed failure among the these 3 clusters.
			// Since ginkgo only captures one failure, to help with this we are logging the error
			// that will contain the full stack trace of failure for each cluster to help with debugging.
			// TODO(ykakarap): Follow-up: Explore options for improved error reporting.
			log.Logf("Failed to delete clusters. Error: %s", err.Error())
			Fail("")
		}

		// TODO(ykakarap): Follow-up: Dump resources for the failed clusters (deletion).

		By("PASSED!")
	})

	AfterEach(func() {
		cancelWatches()
	})
}

func extractClusterClassAndClusterFromTemplate(rawYAML []byte) ([]byte, []byte) {
	objs, err := yaml.ToUnstructured(rawYAML)
	Expect(err).ToNot(HaveOccurred())
	clusterObj := unstructured.Unstructured{}
	clusterClassAndTemplates := []unstructured.Unstructured{}
	for _, obj := range objs {
		if obj.GroupVersionKind().GroupKind() == clusterv1.GroupVersion.WithKind("Cluster").GroupKind() {
			clusterObj = obj
		} else {
			clusterClassAndTemplates = append(clusterClassAndTemplates, obj)
		}
	}
	clusterYAML, err := yaml.FromUnstructured([]unstructured.Unstructured{clusterObj})
	Expect(err).ToNot(HaveOccurred())
	clusterClassYAML, err := yaml.FromUnstructured(clusterClassAndTemplates)
	Expect(err).ToNot(HaveOccurred())
	return clusterClassYAML, clusterYAML
}

type workConcurrentlyAndWaitInput struct {
	// ClusterNames is the names of clusters to work on.
	ClusterNames []string

	// Concurrency is the maximum number of clusters to be created concurrently.
	// NB. This also includes waiting for the clusters to be up and running.
	// Example: If the concurrency is 2. It would create 2 clusters concurrently and wait
	// till at least one of the clusters is up and running before it starts creating another
	// cluster.
	Concurrency int64

	FailFast bool

	WorkerFunc func(ctx context.Context, inputChan chan string, errChan chan workResult, wg *sync.WaitGroup)
}

func workConcurrentlyAndWait(ctx context.Context, input workConcurrentlyAndWaitInput) ([]workResult, error) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for workConcurrentlyAndWait")
	Expect(input.Concurrency).To(BeNumerically(">", 0), "Invalid argument. input.Concurrency should be greater that 0")

	// Start a channel. This channel will be used to coordinate work with the workers.
	// The channel is used to communicate the name of the cluster.
	// Adding a new name to the channel implies that a new cluster of the given names needs to be processed.
	inputChan := make(chan string)
	wg := &sync.WaitGroup{}
	doneChan := make(chan bool)
	resultChan := make(chan workResult)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start the workers.
	for range input.Concurrency {
		wg.Add(1)
		go input.WorkerFunc(ctx, inputChan, resultChan, wg)
	}

	// Adding the cluster names into the input channel.
	go func() {
		for _, name := range input.ClusterNames {
			inputChan <- name
		}
		// All the clusters are requested.
		// Close the channel to shut down workers as they become unused.
		close(inputChan)
	}()

	go func() {
		// Wait for all the workers to shut down.
		wg.Wait()
		close(doneChan)
	}()

	results := []workResult{}

outer:
	for {
		select {
		case result := <-resultChan:
			results = append(results, result)
			if result.err != nil && input.FailFast {
				cancel()
			}
		case <-doneChan:
			break outer
		}
	}

	// Clean up. All the workers are shut down.
	// Close the result channel.
	close(resultChan)

	errs := []error{}
	for _, result := range results {
		if result.err != nil {
			if e, ok := result.err.(types.GinkgoError); ok {
				errs = append(errs, errors.Errorf("[clusterName: %q] Stack trace: \n %s", result.clusterName, e.CodeLocation.FullStackTrace))
			} else {
				errs = append(errs, errors.Errorf("[clusterName: %q] Error: %v", result.clusterName, result.err))
			}
		}
	}

	return results, kerrors.NewAggregate(errs)
}

type clusterCreator func(ctx context.Context, namespace, clusterName string, clusterTemplateYAML []byte)

func getClusterCreateAndWaitFn(input clusterctl.ApplyCustomClusterTemplateAndWaitInput) clusterCreator {
	return func(ctx context.Context, namespace, clusterName string, clusterTemplateYAML []byte) {
		clusterResources := &clusterctl.ApplyCustomClusterTemplateAndWaitResult{}
		// Nb. We cannot directly modify and use `input` in this closure function because this function
		// will be called multiple times and this closure will keep modifying the same `input` multiple
		// times. It is safer to pass the values explicitly into `ApplyCustomClusterTemplateAndWait`.
		clusterctl.ApplyCustomClusterTemplateAndWait(ctx, clusterctl.ApplyCustomClusterTemplateAndWaitInput{
			ClusterProxy:                 input.ClusterProxy,
			CustomTemplateYAML:           clusterTemplateYAML,
			ClusterName:                  clusterName,
			Namespace:                    namespace,
			WaitForClusterIntervals:      input.WaitForClusterIntervals,
			WaitForControlPlaneIntervals: input.WaitForControlPlaneIntervals,
			WaitForMachineDeployments:    input.WaitForMachineDeployments,
			CreateOrUpdateOpts:           input.CreateOrUpdateOpts,
			PreWaitForCluster:            input.PreWaitForCluster,
			PostMachinesProvisioned:      input.PostMachinesProvisioned,
			ControlPlaneWaiters:          input.ControlPlaneWaiters,
		}, clusterResources)
	}
}

func getClusterCreateFn(clusterProxy framework.ClusterProxy) clusterCreator {
	return func(ctx context.Context, namespace, clusterName string, clusterTemplateYAML []byte) {
		log.Logf("Applying the cluster template yaml of cluster %s", klog.KRef(namespace, clusterName))
		Eventually(func() error {
			return clusterProxy.CreateOrUpdate(ctx, clusterTemplateYAML)
		}, 1*time.Minute).Should(Succeed(), "Failed to apply the cluster template of cluster %s", klog.KRef(namespace, clusterName))
	}
}

func createClusterWorker(ctx context.Context, clusterProxy framework.ClusterProxy, inputChan <-chan string, resultChan chan<- workResult, wg *sync.WaitGroup, defaultNamespace string, deployClusterInSeparateNamespaces bool, baseClusterClassYAML, baseClusterTemplateYAML []byte, create clusterCreator) {
	defer wg.Done()

	for {
		done := func() bool {
			select {
			case <-ctx.Done():
				// If the context is cancelled, return and shutdown the worker.
				return true
			case clusterName, open := <-inputChan:
				// Read the cluster name from the channel.
				// If the channel is closed it implies there is no more work to be done. Return.
				if !open {
					return true
				}
				log.Logf("Creating cluster %s", clusterName)

				// This defer will catch ginkgo failures and record them.
				// The recorded panics are then handled by the parent goroutine.
				defer func() {
					e := recover()
					resultChan <- workResult{
						clusterName: clusterName,
						err:         e,
					}
				}()

				// Calculate namespace.
				namespaceName := defaultNamespace
				if deployClusterInSeparateNamespaces {
					namespaceName = clusterName
				}

				// If every cluster should be deployed in a separate namespace:
				// * Adjust namespace in ClusterClass YAML.
				// * Create new namespace.
				// * Deploy ClusterClass in new namespace.
				if deployClusterInSeparateNamespaces {
					log.Logf("Create namespace %", namespaceName)
					_ = framework.CreateNamespace(ctx, framework.CreateNamespaceInput{
						Creator:             clusterProxy.GetClient(),
						Name:                namespaceName,
						IgnoreAlreadyExists: true,
					}, "40s", "10s")

					log.Logf("Apply ClusterClass in namespace %", namespaceName)
					clusterClassYAML := bytes.Replace(baseClusterClassYAML, []byte(scaleClusterNamespacePlaceholder), []byte(namespaceName), -1)
					Eventually(func() error {
						return clusterProxy.CreateOrUpdate(ctx, clusterClassYAML)
					}, 1*time.Minute).Should(Succeed())
				}

				// Adjust namespace and name in Cluster YAML
				clusterTemplateYAML := bytes.Replace(baseClusterTemplateYAML, []byte(scaleClusterNamespacePlaceholder), []byte(namespaceName), -1)
				clusterTemplateYAML = bytes.Replace(clusterTemplateYAML, []byte(scaleClusterNamePlaceholder), []byte(clusterName), -1)

				// Deploy Cluster.
				create(ctx, namespaceName, clusterName, clusterTemplateYAML)
				return false
			}
		}()
		if done {
			break
		}
	}
}

func deleteClusterAndWaitWorker(ctx context.Context, inputChan <-chan string, resultChan chan<- workResult, wg *sync.WaitGroup, c client.Client, defaultNamespace string, deployClusterInSeparateNamespaces bool) {
	defer wg.Done()

	for {
		done := func() bool {
			select {
			case <-ctx.Done():
				// If the context is cancelled, return and shutdown the worker.
				return true
			case clusterName, open := <-inputChan:
				// Read the cluster name from the channel.
				// If the channel is closed it implies there is no more work to be done. Return.
				if !open {
					return true
				}
				log.Logf("Deleting cluster %s", clusterName)

				// This defer will catch ginkgo failures and record them.
				// The recorded panics are then handled by the parent goroutine.
				defer func() {
					e := recover()
					resultChan <- workResult{
						clusterName: clusterName,
						err:         e,
					}
				}()

				// Calculate namespace.
				namespaceName := defaultNamespace
				if deployClusterInSeparateNamespaces {
					namespaceName = clusterName
				}

				cluster := &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      clusterName,
						Namespace: namespaceName,
					},
				}
				framework.DeleteCluster(ctx, framework.DeleteClusterInput{
					Deleter: c,
					Cluster: cluster,
				})
				framework.WaitForClusterDeleted(ctx, framework.WaitForClusterDeletedInput{
					Client:  c,
					Cluster: cluster,
				})

				// Note: We only delete the namespace in this case because in the case where all clusters are deployed
				// to the same namespace deleting the Namespace will lead to deleting all clusters.
				if deployClusterInSeparateNamespaces {
					framework.DeleteNamespace(ctx, framework.DeleteNamespaceInput{
						Deleter: c,
						Name:    namespaceName,
					})
				}
				return false
			}
		}()
		if done {
			break
		}
	}
}

type clusterUpgrader func(ctx context.Context, namespace, clusterName string, clusterTemplateYAML []byte)

func getClusterUpgradeAndWaitFn(input framework.UpgradeClusterTopologyAndWaitForUpgradeInput) clusterUpgrader {
	return func(ctx context.Context, namespace, clusterName string, _ []byte) {
		resources := getClusterResourcesForUpgrade(ctx, input.ClusterProxy.GetClient(), namespace, clusterName)
		// Nb. We cannot directly modify and use `input` in this closure function because this function
		// will be called multiple times and this closure will keep modifying the same `input` multiple
		// times. It is safer to pass the values explicitly into `UpgradeClusterTopologyAndWaitForUpgradeInput`.
		framework.UpgradeClusterTopologyAndWaitForUpgrade(ctx, framework.UpgradeClusterTopologyAndWaitForUpgradeInput{
			ClusterProxy:                input.ClusterProxy,
			Cluster:                     resources.cluster,
			ControlPlane:                resources.controlPlane,
			MachineDeployments:          resources.machineDeployments,
			KubernetesUpgradeVersion:    input.KubernetesUpgradeVersion,
			WaitForMachinesToBeUpgraded: input.WaitForMachinesToBeUpgraded,
			WaitForKubeProxyUpgrade:     input.WaitForKubeProxyUpgrade,
			WaitForDNSUpgrade:           input.WaitForDNSUpgrade,
			WaitForEtcdUpgrade:          input.WaitForEtcdUpgrade,
			// TODO: (killianmuldoon) Checking the kube-proxy, etcd and DNS version doesn't work as we can't access the control plane endpoint for the workload cluster
			// from the host. Need to figure out a way to route the calls to the workload Cluster correctly.
			EtcdImageTag:       "",
			DNSImageTag:        "",
			SkipKubeProxyCheck: true,
		})
	}
}

func upgradeClusterAndWaitWorker(ctx context.Context, inputChan <-chan string, resultChan chan<- workResult, wg *sync.WaitGroup, defaultNamespace string, deployClusterInSeparateNamespaces bool, clusterTemplateYAML []byte, upgrade clusterUpgrader) {
	defer wg.Done()

	for {
		done := func() bool {
			select {
			case <-ctx.Done():
				// If the context is cancelled, return and shutdown the worker.
				return true
			case clusterName, open := <-inputChan:
				// Read the cluster name from the channel.
				// If the channel is closed it implies there is no more work to be done. Return.
				if !open {
					return true
				}
				log.Logf("Upgrading cluster %s", clusterName)

				// This defer will catch ginkgo failures and record them.
				// The recorded panics are then handled by the parent goroutine.
				defer func() {
					e := recover()
					resultChan <- workResult{
						clusterName: clusterName,
						err:         e,
					}
				}()

				// Calculate namespace.
				namespaceName := defaultNamespace
				if deployClusterInSeparateNamespaces {
					namespaceName = clusterName
				}
				upgrade(ctx, namespaceName, clusterName, clusterTemplateYAML)
				return false
			}
		}()
		if done {
			break
		}
	}
}

type clusterResources struct {
	cluster            *clusterv1.Cluster
	machineDeployments []*clusterv1.MachineDeployment
	controlPlane       *controlplanev1.KubeadmControlPlane
}

func getClusterResourcesForUpgrade(ctx context.Context, c client.Client, namespace, clusterName string) clusterResources {
	cluster := &clusterv1.Cluster{}
	err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: clusterName}, cluster)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error getting Cluster %s: %s", klog.KRef(namespace, clusterName), err))

	controlPlane := &controlplanev1.KubeadmControlPlane{}
	err = c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: cluster.Spec.ControlPlaneRef.Name}, controlPlane)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error getting ControlPlane for Cluster %s: %s,", klog.KObj(cluster), err))

	mds := []*clusterv1.MachineDeployment{}
	machineDeployments := &clusterv1.MachineDeploymentList{}
	err = c.List(ctx, machineDeployments,
		client.MatchingLabels{
			clusterv1.ClusterNameLabel:          cluster.Name,
			clusterv1.ClusterTopologyOwnedLabel: "",
		},
		client.InNamespace(cluster.Namespace),
	)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error getting MachineDeployments for Cluster %s: %s", klog.KObj(cluster), err))
	for _, md := range machineDeployments.Items {
		mds = append(mds, md.DeepCopy())
	}

	return clusterResources{
		cluster:            cluster,
		machineDeployments: mds,
		controlPlane:       controlPlane,
	}
}

type workResult struct {
	clusterName string
	err         any
}

func modifyMachineDeployments(baseClusterTemplateYAML []byte, count int) []byte {
	Expect(baseClusterTemplateYAML).NotTo(BeEmpty(), "Invalid argument. baseClusterTemplateYAML cannot be empty when calling modifyMachineDeployments")
	Expect(count).To(BeNumerically(">=", 0), "Invalid argument. count cannot be less than 0 when calling modifyMachineDeployments")

	objs, err := yaml.ToUnstructured(baseClusterTemplateYAML)
	Expect(err).ToNot(HaveOccurred())
	Expect(objs).To(HaveLen(1), "Unexpected number of objects found in baseClusterTemplateYAML")

	scheme := runtime.NewScheme()
	framework.TryAddDefaultSchemes(scheme)
	cluster := &clusterv1.Cluster{}
	Expect(scheme.Convert(&objs[0], cluster, nil)).Should(Succeed())
	// Verify the Cluster Topology.
	Expect(cluster.Spec.Topology).NotTo(BeNil(), "Should be a ClusterClass based Cluster")
	Expect(cluster.Spec.Topology.Workers).NotTo(BeNil(), "ClusterTopology should have exactly one MachineDeployment. Cannot be empty")
	Expect(cluster.Spec.Topology.Workers.MachineDeployments).To(HaveLen(1), "ClusterTopology should have exactly one MachineDeployment")

	baseMD := cluster.Spec.Topology.Workers.MachineDeployments[0]
	allMDs := make([]clusterv1.MachineDeploymentTopology, count)
	allMDDigits := 1 + int(math.Log10(float64(count)))
	for i := 1; i <= count; i++ {
		md := baseMD.DeepCopy()
		// This ensures we always have the right number of leading zeros in our machine deployment names, e.g.
		// count=1000 will lead to machine deployment names like md-0001, md-0002, so on.
		md.Name = fmt.Sprintf("md-%0*d", allMDDigits, i)
		allMDs[i-1] = *md
	}
	cluster.Spec.Topology.Workers.MachineDeployments = allMDs
	u := &unstructured.Unstructured{}
	Expect(scheme.Convert(cluster, u, nil)).To(Succeed())
	modifiedClusterYAML, err := yaml.FromUnstructured([]unstructured.Unstructured{*u})
	Expect(err).ToNot(HaveOccurred())

	return modifiedClusterYAML
}
