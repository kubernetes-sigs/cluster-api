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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
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
	// ObjectFilter identifies if an object should be considered to get deleted during this phase.
	// During the test the identified objects are expected to only have an deletionTimestamp when this phase is tested
	// and be gone after unblocking this phases blocking objects.
	ObjectFilter func(objectReference corev1.ObjectReference, objectOwnerReferences []metav1.OwnerReference) bool
	// BlockingObjectFilter is a filter on top of the ObjectFilter to identify which objects should block the deletion in
	// this phase. The identified objects will get a finalizer added before the deletion of the cluster starts.
	// After a successful verification that all objects are in the correct state the finalizer gets removed to unblock
	// the deletion and go to the next phase.
	BlockingObjectFilter func(objectReference corev1.ObjectReference, objectOwnerReferences []metav1.OwnerReference) bool
}

// ClusterDeletionSpec goes through the following steps:
// * Create a cluster.
// * Add a finalizer to the Cluster object.
// * Add a finalizer to objects identified by input.ClusterDeletionPhases.BlockingObjectFilter.
// * Trigger deletion for the cluster.
// * For each phase:
//   - Verify objects of the previous phase are gone.
//   - Verify objects to be deleted in this phase have a deletionTimestamp set but still exist.
//   - Verify objects expected to be deleted in a later phase don't have a deletionTimestamp set.
//   - Remove finalizers for this phase's BlockingObjects to unblock deletion of them.
//
// * Do a final verification:
//   - Verify objects expected to get deleted in the last phase are gone.
//   - Verify the Cluster object has a deletionTimestamp set but still exists.
//   - Remove finalizer for the Cluster object.
//
// * Verify the Cluster object is gone.
func ClusterDeletionSpec(ctx context.Context, inputGetter func() ClusterDeletionSpecInput) {
	var (
		specName         = "cluster-deletion"
		input            ClusterDeletionSpecInput
		namespace        *corev1.Namespace
		cancelWatches    context.CancelFunc
		clusterResources *clusterctl.ApplyClusterTemplateAndWaitResult
		finalizer        = "test.cluster.x-k8s.io/cluster-deletion"
		blockingObjects  []client.Object
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

		// Get all objects per deletion phase and the list of blocking objects.
		var objectsPerPhase [][]client.Object
		objectsPerPhase, blockingObjects = getDeletionPhaseObjects(ctx, input.BootstrapClusterProxy, clusterResources.Cluster, input.ClusterDeletionPhases)

		// Add a finalizer to all objects in blockingObjects to control when these objects are gone.
		By(fmt.Sprintf("Adding finalizer %s on objects to block during cluster deletion", finalizer))
		addFinalizer(ctx, input.BootstrapClusterProxy.GetClient(), finalizer, blockingObjects...)

		// Trigger the deletion of the Cluster.
		framework.DeleteCluster(ctx, framework.DeleteClusterInput{
			Cluster: clusterResources.Cluster,
			Deleter: input.BootstrapClusterProxy.GetClient(),
		})

		var expectedObjectsDeleted, expectedObjectsInDeletion []client.Object

		// Verify deletion for each phase.
		for i, phaseObjects := range objectsPerPhase {
			By(fmt.Sprintf("Verify deletion phase %d/%d", i+1, len(objectsPerPhase)))
			// Expect the objects of the previous phase to be deleted.
			expectedObjectsDeleted = expectedObjectsInDeletion
			// Expect the objects of this phase to have a deletionTimestamp set, but still exist due to the blocking objects having the finalizer.
			expectedObjectsInDeletion = phaseObjects
			// Expect the objects of upcoming phases to not yet have a deletionTimestamp set.
			expectedObjectsNotInDeletion := []client.Object{}
			for j := i + 1; j < len(objectsPerPhase); j++ {
				expectedObjectsNotInDeletion = append(expectedObjectsNotInDeletion, objectsPerPhase[j]...)
			}

			assertDeletionPhase(ctx, input.BootstrapClusterProxy.GetClient(), finalizer,
				expectedObjectsDeleted,
				expectedObjectsInDeletion,
				expectedObjectsNotInDeletion,
			)

			// Remove the test's finalizer from all objects which had been expected to be in deletion to unblock the next phase.
			// Note: this only removes the finalizer from objects which have it set.
			By(fmt.Sprintf("Removing finalizers for phase %d/%d", i+1, len(input.ClusterDeletionPhases)))
			removeFinalizer(ctx, input.BootstrapClusterProxy.GetClient(), finalizer, expectedObjectsInDeletion...)
		}

		By("Final deletion verification")
		// Verify that all objects of the last phase are gone and the cluster does still exist.
		expectedObjectsDeleted = expectedObjectsInDeletion
		assertDeletionPhase(ctx, input.BootstrapClusterProxy.GetClient(), finalizer,
			expectedObjectsDeleted,
			[]client.Object{clusterResources.Cluster},
			nil,
		)

		// Remove the test's finalizer from the cluster object.
		By("Removing finalizers for Cluster")
		removeFinalizer(ctx, input.BootstrapClusterProxy.GetClient(), finalizer, clusterResources.Cluster)

		log.Logf("Waiting for the Cluster %s to be deleted", klog.KObj(clusterResources.Cluster))
		framework.WaitForClusterDeleted(ctx, framework.WaitForClusterDeletedInput{
			ClusterProxy:         input.BootstrapClusterProxy,
			ClusterctlConfigPath: input.ClusterctlConfigPath,
			Cluster:              clusterResources.Cluster,
			ArtifactFolder:       input.ArtifactFolder,
		}, input.E2EConfig.GetIntervals(specName, "wait-delete-cluster")...)

		By("PASSED!")
	})

	AfterEach(func() {
		// Dump all the resources in the spec namespace and the workload cluster.
		framework.DumpAllResourcesAndLogs(ctx, input.BootstrapClusterProxy, input.ClusterctlConfigPath, input.ArtifactFolder, namespace, clusterResources.Cluster)

		if !input.SkipCleanup {
			// Remove finalizers we added to block normal deletion.
			removeFinalizer(ctx, input.BootstrapClusterProxy.GetClient(), finalizer, blockingObjects...)

			By(fmt.Sprintf("Deleting cluster %s", klog.KObj(clusterResources.Cluster)))
			// While https://github.com/kubernetes-sigs/cluster-api/issues/2955 is addressed in future iterations, there is a chance
			// that cluster variable is not set even if the cluster exists, so we are calling DeleteAllClustersAndWait
			// instead of DeleteClusterAndWait
			framework.DeleteAllClustersAndWait(ctx, framework.DeleteAllClustersAndWaitInput{
				ClusterProxy:         input.BootstrapClusterProxy,
				ClusterctlConfigPath: input.ClusterctlConfigPath,
				Namespace:            namespace.Name,
				ArtifactFolder:       input.ArtifactFolder,
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

// getDeletionPhaseObjects gets the OwnerGraph for the cluster and returns all objects per phase and an additional array
// with all objects which should block during the cluster deletion.
func getDeletionPhaseObjects(ctx context.Context, bootstrapClusterProxy framework.ClusterProxy, cluster *clusterv1.Cluster, phases []ClusterDeletionPhase) (objectsPerPhase [][]client.Object, blockingObjects []client.Object) {
	// Add Cluster object to blockingObjects to control when it gets actually removed during the test
	// and to be able to cleanup the Cluster during Teardown on failures.
	blockingObjects = append(blockingObjects, cluster)

	// Get all objects relevant to the test by filtering for the kinds of the phases.
	graph, err := clusterctlcluster.GetOwnerGraph(ctx, cluster.GetNamespace(), bootstrapClusterProxy.GetKubeconfigPath(), clusterctlcluster.FilterClusterObjectsWithNameFilter(cluster.GetName()))
	Expect(err).ToNot(HaveOccurred())

	for _, phase := range phases {
		objects := []client.Object{}
		// Iterate over the graph and find all objects relevant for the phase and which of them should be blocking.
		for _, node := range graph {
			// Check if the node should be part of the phase.
			if !phase.ObjectFilter(node.Object, node.Owners) {
				continue
			}

			obj := new(unstructured.Unstructured)
			obj.SetAPIVersion(node.Object.APIVersion)
			obj.SetKind(node.Object.Kind)
			obj.SetName(node.Object.Name)
			obj.SetNamespace(cluster.GetNamespace())

			objects = append(objects, obj)

			// Add the object to the objectsWithFinalizer array if it should be blocking.
			if phase.BlockingObjectFilter(node.Object, node.Owners) {
				blockingObjects = append(blockingObjects, obj)
			}
		}

		objectsPerPhase = append(objectsPerPhase, objects)
	}
	return objectsPerPhase, blockingObjects
}

func addFinalizer(ctx context.Context, c client.Client, finalizer string, objs ...client.Object) {
	for _, obj := range objs {
		log.Logf("Adding finalizer for %s %s", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj))
		Expect(c.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed(), fmt.Sprintf("Failed to get %s %s", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj)))

		helper, err := patch.NewHelper(obj, c)
		Expect(err).ToNot(HaveOccurred())

		if updated := controllerutil.AddFinalizer(obj, finalizer); !updated {
			continue
		}

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

		if updated := controllerutil.RemoveFinalizer(obj, finalizer); !updated {
			continue
		}

		Expect(helper.Patch(ctx, obj)).ToNot(HaveOccurred(), fmt.Sprintf("Failed to remove finalizer from %s %s", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj)))
	}
}

func assertDeletionPhase(ctx context.Context, c client.Client, finalizer string, expectedObjectsDeleted, expectedObjectsInDeletion, expectedObjectsNotInDeletion []client.Object) {
	Eventually(func() error {
		var errs []error
		// Ensure deleted objects are gone
		for _, obj := range expectedObjectsDeleted {
			err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj)
			if err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				errs = append(errs, errors.Wrapf(err, "expected %s %s to be deleted", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj)))
				continue
			}
			errs = append(errs, errors.Errorf("expected %s %s to be deleted but it still exists", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj)))
		}

		// Ensure expected objects are in deletion
		for _, obj := range expectedObjectsInDeletion {
			if err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
				errs = append(errs, errors.Wrapf(err, "checking %s %s is in deletion", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj)))
				continue
			}
			if obj.GetDeletionTimestamp().IsZero() {
				errs = append(errs, errors.Errorf("expected %s %s to be in deletion but deletionTimestamp is not set", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj)))
				continue
			}

			// Blocking objects have our finalizer set. When being excepted to be in deletion,
			// blocking objects should only have our finalizer, otherwise they are getting blocked by something else.
			if sets.New[string](obj.GetFinalizers()...).Has(finalizer) && len(obj.GetFinalizers()) > 1 {
				errs = append(errs, errors.Errorf("expected blocking %s %s to only have %s as finalizer, got [%s]", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj), finalizer, strings.Join(obj.GetFinalizers(), ", ")))
				continue
			}
		}

		// Ensure other objects are not in deletion.
		for _, obj := range expectedObjectsNotInDeletion {
			if err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
				errs = append(errs, errors.Wrapf(err, "checking %s %s is not in deletion", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj)))
				continue
			}

			if !obj.GetDeletionTimestamp().IsZero() {
				errs = append(errs, errors.Errorf("expected %s %s to not be in deletion but deletionTimestamp is set", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj)))
				continue
			}
		}

		return kerrors.NewAggregate(errs)
	}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
}
