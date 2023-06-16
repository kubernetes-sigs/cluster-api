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
	"os"
	"path/filepath"
	"strconv"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
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
	Flavor *string

	// ClusterCount is the number of target workload clusters.
	// If unspecified, defaults to 10.
	// Can be overridden by variable CAPI_SCALE_CLUSTER_COUNT.
	ClusterCount *int64

	// Concurrency is the maximum concurrency of each of the scale operations.
	// If unspecified it defaults to 5.
	// Can be overridden by variable CAPI_SCALE_CONCURRENCY.
	Concurrency *int64

	// ControlPlaneMachineCount defines the number of control plane machines to be added to each workload cluster.
	// If not specified, 1 will be used.
	// Can be overridden by variable CAPI_SCALE_CONTROLPLANE_MACHINE_COUNT.
	ControlPlaneMachineCount *int64

	// WorkerMachineCount defines number of worker machines to be added to each workload cluster.
	// If not specified, 1 will be used.
	// Can be overridden by variable CAPI_SCALE_WORKER_MACHINE_COUNT.
	WorkerMachineCount *int64

	// FailFast if set to true will return immediately after the first cluster operation fails.
	// If set to false, the test suite will not exit immediately after the first cluster operation fails.
	// Example: When creating clusters from c1 to c20 consider c6 fails creation. If FailFast is set to true
	// the suit will exit immediately after receiving the c6 creation error. If set to false, cluster creations
	// of the other clusters will continue and all the errors are collected before the test exists.
	// Note: Please note that the test suit will still fail since c6 creation failed. FailFast will determine
	// if the test suit should fail as soon as c6 fails or if it should fail after all cluster creations are done.
	FailFast bool
}

// scaleSpec implements a scale test.
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
		namespace, cancelWatches = setupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder)
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

		controlPlaneMachineCount := pointer.Int64(1)
		if input.ControlPlaneMachineCount != nil {
			controlPlaneMachineCount = input.ControlPlaneMachineCount
		}
		// If variable is defined that will take precedence.
		if input.E2EConfig.HasVariable(scaleControlPlaneMachineCount) {
			controlPlaneMachineCountStr := input.E2EConfig.GetVariable(scaleControlPlaneMachineCount)
			controlPlaneMachineCountInt, err := strconv.Atoi(controlPlaneMachineCountStr)
			Expect(err).NotTo(HaveOccurred())
			controlPlaneMachineCount = pointer.Int64(int64(controlPlaneMachineCountInt))
		}

		workerMachineCount := pointer.Int64(1)
		if input.WorkerMachineCount != nil {
			workerMachineCount = input.WorkerMachineCount
		}
		// If variable is defined that will take precedence.
		if input.E2EConfig.HasVariable(scaleWorkerMachineCount) {
			workerMachineCountStr := input.E2EConfig.GetVariable(scaleWorkerMachineCount)
			workerMachineCountInt, err := strconv.Atoi(workerMachineCountStr)
			Expect(err).NotTo(HaveOccurred())
			workerMachineCount = pointer.Int64(int64(workerMachineCountInt))
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
		baseClusterName := fmt.Sprintf("%s-base", specName)
		baseWorkloadClusterTemplate := clusterctl.ConfigCluster(ctx, clusterctl.ConfigClusterInput{
			LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
			ClusterctlConfigPath:     input.ClusterctlConfigPath,
			KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
			InfrastructureProvider:   infrastructureProvider,
			Flavor:                   flavor,
			Namespace:                namespace.Name,
			ClusterName:              baseClusterName,
			KubernetesVersion:        input.E2EConfig.GetVariable(KubernetesVersion),
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
		clusterClassYAML, baseClusterTemplateYAML := extractClusterClassAndClusterFromTemplate(baseWorkloadClusterTemplate)

		// Apply the ClusterClass.
		log.Logf("Create ClusterClass")
		Eventually(func() error {
			return input.BootstrapClusterProxy.Apply(ctx, clusterClassYAML, "-n", namespace.Name)
		}).Should(Succeed())

		By("Create workload clusters concurrently")
		// Create multiple clusters concurrently from the same base cluster template.

		clusterNames := make([]string, 0, clusterCount)
		for i := int64(1); i <= clusterCount; i++ {
			name := fmt.Sprintf("%s-%d", specName, i)
			clusterNames = append(clusterNames, name)
		}

		clusterCreateResults, err := workConcurrentlyAndWait(ctx, workConcurrentlyAndWaitInput{
			ClusterNames: clusterNames,
			Concurrency:  concurrency,
			FailFast:     input.FailFast,
			WorkerFunc: func(ctx context.Context, inputChan chan string, resultChan chan workResult, wg *sync.WaitGroup) {
				createClusterAndWaitWorker(ctx, inputChan, resultChan, wg, baseClusterTemplateYAML, baseClusterName, clusterctl.ApplyCustomClusterTemplateAndWaitInput{
					ClusterProxy:                 input.BootstrapClusterProxy,
					Namespace:                    namespace.Name,
					WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
					WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
					WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
				})
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

		// TODO(ykakarap): Follow-up: Dump resources for the failed clusters (creation).

		clusterNamesToDelete := []string{}
		for _, result := range clusterCreateResults {
			clusterNamesToDelete = append(clusterNamesToDelete, result.clusterName)
		}

		By("Delete the workload clusters concurrently")
		// Now delete all the workload clusters.
		_, err = workConcurrentlyAndWait(ctx, workConcurrentlyAndWaitInput{
			ClusterNames: clusterNamesToDelete,
			Concurrency:  concurrency,
			FailFast:     input.FailFast,
			WorkerFunc: func(ctx context.Context, inputChan chan string, resultChan chan workResult, wg *sync.WaitGroup) {
				deleteClusterAndWaitWorker(ctx, inputChan, resultChan, wg, input.BootstrapClusterProxy.GetClient(), namespace.Name)
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
	Expect(err).NotTo(HaveOccurred())
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
	Expect(err).NotTo(HaveOccurred())
	clusterClassYAML, err := yaml.FromUnstructured(clusterClassAndTemplates)
	Expect(err).NotTo(HaveOccurred())
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
	for i := int64(0); i < input.Concurrency; i++ {
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

func createClusterAndWaitWorker(ctx context.Context, inputChan <-chan string, resultChan chan<- workResult, wg *sync.WaitGroup, baseTemplate []byte, baseClusterName string, input clusterctl.ApplyCustomClusterTemplateAndWaitInput) {
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

				// Create the cluster template YAML with the target cluster name.
				clusterTemplateYAML := bytes.Replace(baseTemplate, []byte(baseClusterName), []byte(clusterName), -1)
				// Nb. Input is passed as a copy therefore we can safely update the value here, and it won't affect other
				// workers.
				input.CustomTemplateYAML = clusterTemplateYAML
				input.ClusterName = clusterName

				clusterResources := &clusterctl.ApplyCustomClusterTemplateAndWaitResult{}
				clusterctl.ApplyCustomClusterTemplateAndWait(ctx, input, clusterResources)
				return false
			}
		}()
		if done {
			break
		}
	}
}

func deleteClusterAndWaitWorker(ctx context.Context, inputChan <-chan string, resultChan chan<- workResult, wg *sync.WaitGroup, c client.Client, namespace string) {
	defer wg.Done()

	for {
		done := func() bool {
			select {
			case <-ctx.Done():
				// If the context is cancelled, return and shutdown the worker.
				return true
			case clusterName, open := <-inputChan:
				// Read the cluster name from the channel.
				// If the channel is closed it implies there is not more work to be done. Return.
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

				cluster := &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      clusterName,
						Namespace: namespace,
					},
				}
				framework.DeleteCluster(ctx, framework.DeleteClusterInput{
					Deleter: c,
					Cluster: cluster,
				})
				framework.WaitForClusterDeleted(ctx, framework.WaitForClusterDeletedInput{
					Getter:  c,
					Cluster: cluster,
				})
				return false
			}
		}()
		if done {
			break
		}
	}
}

type workResult struct {
	clusterName string
	err         any
}
