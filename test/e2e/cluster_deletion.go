/*
Copyright 2024 The Kubernetes Authors.

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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterctlcluster "sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/test/e2e/internal/log"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
)

// ClusterDeletionSpecInput is the input for ClusterDeletionSpec.
type ClusterDeletionSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool

	// Cluster name allows to specify a deterministic clusterName.
	// If not set, a random one will be generated.
	ClusterName *string

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

	// Allows to inject a function to be run after test namespace is created.
	// If not specified, this is a no-op.
	PostNamespaceCreated func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace string)

	// Allows to inject a function to be run after machines are provisioned.
	// If not specified, this is a no-op.
	PostMachinesProvisioned func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace, workloadClusterName string)

	// ClusterDeletionPhases define kinds which are expected to get deleted in phases.
	ClusterDeletionPhases []ClusterDeletionPhase
}

type ClusterDeletionPhase struct {
	// DeletionSelector identifies if a node should be considered to get deleted during this phase.
	DeletionSelector func(node clusterctlcluster.OwnerGraphNode) bool
	// IsBlocking is a filter on top of the DeletionSelector to identify which nodes should block the deletion in this phase.
	// The deletion of all objects in this phase gets blocked by adding a finalizer to this nodes to ensure the test can
	// assert when the objects should be in deletion and actually get removed by control the removal of the finalizer.
	IsBlocking func(node clusterctlcluster.OwnerGraphNode) bool

	// objects will be filled later by objects which matched the DeletionSelector
	objects []client.Object
}

// ClusterDeletionSpec goes through the following steps:
// * Create a cluster.
// * Add finalizer to objects defined in input.ClusterDeletionPhases.DeleteBlockKinds.
// * Trigger deletion for the cluster.
// * For each phase:
//   - Verify objects expected to get deleted in previous phases are gone.
//   - Verify objects expected to be in deletion have a deletionTimestamp set but still exist.
//   - Verify objects expected to be deleted in a later phase don't have a deletionTimestamp set.
//   - Remove finalizers for objects in deletion.
//
// * Verify the Cluster object is gone.
func ClusterDeletionSpec(ctx context.Context, inputGetter func() ClusterDeletionSpecInput) {
	var (
		specName             = "cluster-deletion"
		input                ClusterDeletionSpecInput
		namespace            *corev1.Namespace
		cancelWatches        context.CancelFunc
		clusterResources     *clusterctl.ApplyClusterTemplateAndWaitResult
		finalizer            = "test.cluster.x-k8s.io/cluster-deletion"
		objectsWithFinalizer []client.Object
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
		namespace, cancelWatches = framework.SetupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, input.PostNamespaceCreated)
		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should successfully create and delete a Cluster", func() {
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
		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   infrastructureProvider,
				Flavor:                   flavor,
				Namespace:                namespace.Name,
				ClusterName:              clusterName,
				KubernetesVersion:        input.E2EConfig.GetVariable(KubernetesVersion),
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

		// Get all objects relevant to the test by filtering for the kinds of the phases.
		graph, err := clusterctlcluster.GetOwnerGraph(ctx, namespace.GetName(), input.BootstrapClusterProxy.GetKubeconfigPath(), clusterctlcluster.FilterClusterObjectsWithNameFilter(clusterName))
		Expect(err).ToNot(HaveOccurred())

		for i, phase := range input.ClusterDeletionPhases {
			// Iterate over the graph and find all objects relevant for the phase.
			// * Objects matching the kind except for Machines (see below)
			// * Machines which are owned by one of the kinds which are part of the phase
			for _, node := range graph {
				if !phase.DeletionSelector(node) {
					continue
				}

				obj := new(unstructured.Unstructured)
				obj.SetAPIVersion(node.Object.APIVersion)
				obj.SetKind(node.Object.Kind)
				obj.SetName(node.Object.Name)
				obj.SetNamespace(namespace.GetName())

				phase.objects = append(phase.objects, obj)

				// Add the object to the objectsWithFinalizer array if it should be blocking.
				if phase.IsBlocking(node) {
					objectsWithFinalizer = append(objectsWithFinalizer, obj)
				}
			}

			// Update the phase in input.ClusterDeletionPhases
			input.ClusterDeletionPhases[i] = phase
		}

		// Update all objects in objectsWithFinalizers and add the finalizer for this test to control when these objects vanish.
		By(fmt.Sprintf("Adding finalizer %s on objects to block during cluster deletion", finalizer))
		addFinalizer(ctx, input.BootstrapClusterProxy.GetClient(), finalizer, objectsWithFinalizer...)

		// Trigger the deletion of the Cluster.
		framework.DeleteCluster(ctx, framework.DeleteClusterInput{
			Cluster: clusterResources.Cluster,
			Deleter: input.BootstrapClusterProxy.GetClient(),
		})

		var expectedObjectsDeleted, expectedObjectsInDeletion []client.Object

		for i, phase := range input.ClusterDeletionPhases {
			By(fmt.Sprintf("Verify deletion phase %d/%d", i+1, len(input.ClusterDeletionPhases)))
			// Objects which previously were in deletion because they waited for our finalizer being removed are now checked to actually had been removed.
			expectedObjectsDeleted = expectedObjectsInDeletion
			// All objects for this phase are expected to get into deletion (still exist and have deletionTimestamp set).
			expectedObjectsInDeletion = phase.objects
			// All objects which are part of future phases are expected to not yet be in deletion.
			expectedObjectsNotInDeletion := []client.Object{}
			for j := i + 1; j < len(input.ClusterDeletionPhases); j++ {
				expectedObjectsNotInDeletion = append(expectedObjectsNotInDeletion, input.ClusterDeletionPhases[j].objects...)
			}

			Eventually(func() error {
				var errs []error
				// Ensure deleted objects are gone
				for _, obj := range expectedObjectsDeleted {
					err := input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(obj), obj)
					if err != nil {
						if apierrors.IsNotFound(err) {
							continue
						}
						errs = append(errs, errors.Wrapf(err, "expected %s %s to be deleted", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj)))
						continue
					}
					errs = append(errs, fmt.Errorf("expected %s %s to be deleted but it still exists", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj)))
				}

				// Ensure expected objects are in deletion
				for _, obj := range expectedObjectsInDeletion {
					if err := input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
						errs = append(errs, errors.Wrapf(err, "checking %s %s is in deletion", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj)))
						continue
					}
					if obj.GetDeletionTimestamp().IsZero() {
						errs = append(errs, errors.Wrapf(err, "expected %s %s to be in deletion", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj)))
						continue
					}
				}

				// Ensure other objects are not in deletion.
				for _, obj := range expectedObjectsNotInDeletion {
					if err := input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
						errs = append(errs, errors.Wrapf(err, "checking %s %s is not in deletion", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj)))
						continue
					}

					if !obj.GetDeletionTimestamp().IsZero() {
						errs = append(errs, errors.Wrapf(err, "expected %s %s to not be in deletion", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj)))
						continue
					}
				}

				return kerrors.NewAggregate(errs)
			}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

			// Remove the test's finalizer from all objects which had been expected to be in deletion to unblock the next phase.
			By(fmt.Sprintf("Removing finalizers for phase %d/%d", i+1, len(input.ClusterDeletionPhases)))
			removeFinalizer(ctx, input.BootstrapClusterProxy.GetClient(), finalizer, expectedObjectsInDeletion...)
		}

		// The last phase should unblock the cluster to actually have been removed.
		log.Logf("Waiting for the Cluster %s to be deleted", klog.KObj(clusterResources.Cluster))
		framework.WaitForClusterDeleted(ctx, framework.WaitForClusterDeletedInput{
			Client:         input.BootstrapClusterProxy.GetClient(),
			Cluster:        clusterResources.Cluster,
			ArtifactFolder: input.ArtifactFolder,
		}, input.E2EConfig.GetIntervals(specName, "wait-delete-cluster")...)

		By("PASSED!")
	})

	AfterEach(func() {
		// Dump all the resources in the spec namespace and the workload cluster.
		framework.DumpAllResourcesAndLogs(ctx, input.BootstrapClusterProxy, input.ArtifactFolder, namespace, clusterResources.Cluster)

		if !input.SkipCleanup {
			// Remove finalizers we added to block normal deletion.
			removeFinalizer(ctx, input.BootstrapClusterProxy.GetClient(), finalizer, objectsWithFinalizer...)

			By(fmt.Sprintf("Deleting cluster %s", klog.KObj(clusterResources.Cluster)))
			// While https://github.com/kubernetes-sigs/cluster-api/issues/2955 is addressed in future iterations, there is a chance
			// that cluster variable is not set even if the cluster exists, so we are calling DeleteAllClustersAndWait
			// instead of DeleteClusterAndWait
			framework.DeleteAllClustersAndWait(ctx, framework.DeleteAllClustersAndWaitInput{
				Client:         input.BootstrapClusterProxy.GetClient(),
				Namespace:      namespace.Name,
				ArtifactFolder: input.ArtifactFolder,
			}, input.E2EConfig.GetIntervals(specName, "wait-delete-cluster")...)

			By(fmt.Sprintf("Deleting namespace used for hosting the %q test spec", specName))
			framework.DeleteNamespace(ctx, framework.DeleteNamespaceInput{
				Deleter: input.BootstrapClusterProxy.GetClient(),
				Name:    namespace.Name,
			})
		}
		cancelWatches()
	})
}

func addFinalizer(ctx context.Context, c client.Client, finalizer string, objs ...client.Object) {
	for _, obj := range objs {
		log.Logf("Adding finalizer for %s %s", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj))
		Expect(c.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed(), fmt.Sprintf("Failed to get %s %s", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj)))

		helper, err := patch.NewHelper(obj, c)
		Expect(err).ToNot(HaveOccurred())

		obj.SetFinalizers(append(obj.GetFinalizers(), finalizer))

		Expect(helper.Patch(ctx, obj)).ToNot(HaveOccurred(), fmt.Sprintf("Failed to add finalizer to %s %s", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj)))
	}
}

func removeFinalizer(ctx context.Context, c client.Client, finalizer string, objs ...client.Object) {
	for _, obj := range objs {
		log.Logf("Removing finalizer for %s %s", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj))
		err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		if apierrors.IsNotFound(err) {
			continue
		}
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to get %s %s", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj)))

		helper, err := patch.NewHelper(obj, c)
		Expect(err).ToNot(HaveOccurred())

		var finalizers []string
		for _, f := range obj.GetFinalizers() {
			if f == finalizer {
				continue
			}
			finalizers = append(finalizers, f)
		}

		obj.SetFinalizers(finalizers)
		Expect(helper.Patch(ctx, obj)).ToNot(HaveOccurred(), fmt.Sprintf("Failed to remove finalizer from %s %s", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj)))
	}
}
