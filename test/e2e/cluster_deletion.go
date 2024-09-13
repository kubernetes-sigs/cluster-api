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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
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
	// Kinds are the Kinds which are supposed to be deleted in this phase.
	Kinds sets.Set[string]
	// KindsWithFinalizer are the kinds which are considered to be deleted in this phase, but need to be blocking.
	KindsWithFinalizer sets.Set[string]
	// objects will be filled later by objects which match a kind given in one of the above sets.
	objects []client.Object
}

// ClusterDeletionSpec implements a test that verifies that MachineDeployment rolling updates are successful.
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

		relevantKinds := sets.Set[string]{}

		for _, phaseInput := range input.ClusterDeletionPhases {
			relevantKinds = relevantKinds.
				Union(phaseInput.KindsWithFinalizer).
				Union(phaseInput.Kinds)
		}

		// Get all objects relevant to the test by filtering for the kinds of the phases.
		graph, err := clusterctlcluster.GetOwnerGraph(ctx, namespace.GetName(), input.BootstrapClusterProxy.GetKubeconfigPath(), func(u unstructured.Unstructured) bool {
			return relevantKinds.Has(u.GetKind())
		})
		Expect(err).ToNot(HaveOccurred())

		for i, phase := range input.ClusterDeletionPhases {
			// Iterate over the graph and find all objects relevant for the phase.
			// * Objects matching the kind except for Machines (see below)
			// * Machines which are owned by one of the kinds which are part of the phase
			allKinds := phase.KindsWithFinalizer.Union(phase.Kinds)
			allKindsWithoutMachine := allKinds.Clone().Delete("Machine")

			for _, node := range graph {
				// Only add objects which match a defined kind for the phase.
				if !allKinds.Has(node.Object.Kind) {
					continue
				}

				// Special handling for machines: only add machines which are owned by one of the given kinds.
				if node.Object.Kind == "Machine" {
					isOwnedByKind := false
					for _, owner := range node.Owners {
						if allKindsWithoutMachine.Has(owner.Kind) {
							isOwnedByKind = true
							break
						}
					}
					// Ignore Machines which are not owned by one of the given kinds.
					if !isOwnedByKind {
						continue
					}
				}

				obj := new(unstructured.Unstructured)
				obj.SetAPIVersion(node.Object.APIVersion)
				obj.SetKind(node.Object.Kind)
				obj.SetName(node.Object.Name)
				obj.SetNamespace(namespace.GetName())

				phase.objects = append(phase.objects, obj)

				// If the object's kind is part of kindsWithFinalizer, then additionally the object to the finalizedObjs slice.
				if phase.KindsWithFinalizer.Has(obj.GetKind()) {
					objectsWithFinalizer = append(objectsWithFinalizer, obj)
				}
			}

			// Update the phase in input.ClusterDeletionPhases
			input.ClusterDeletionPhases[i] = phase
		}

		// Update all objects in objectsWithFinalizers and add the finalizer for this test to control when these objects vanish.
		addFinalizer(ctx, input.BootstrapClusterProxy.GetClient(), finalizer, objectsWithFinalizer...)

		// Trigger the deletion of the Cluster.
		framework.DeleteCluster(ctx, framework.DeleteClusterInput{
			Cluster: clusterResources.Cluster,
			Deleter: input.BootstrapClusterProxy.GetClient(),
		})

		var objectsDeleted, objectsInDeletion []client.Object

		for i, phase := range input.ClusterDeletionPhases {
			// Objects which previously were in deletion because they waited for our finalizer being removed are now checked to actually had been removed.
			objectsDeleted = objectsInDeletion
			// All objects for this phase are expected to get into deletion (still exist and have deletionTimestamp set).
			objectsInDeletion = phase.objects
			// All objects which are part of future phases are expected to not yet be in deletion.
			objectsNotInDeletion := []client.Object{}
			for j := i + 1; j < len(input.ClusterDeletionPhases); j++ {
				objectsNotInDeletion = append(objectsNotInDeletion, input.ClusterDeletionPhases[j].objects...)
			}

			Eventually(func(g Gomega) {
				// Ensure expected objects are in deletion
				for _, obj := range objectsInDeletion {
					g.Expect(input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(obj), obj)).ToNot(HaveOccurred())
					g.Expect(obj.GetDeletionTimestamp().IsZero()).To(BeFalse())
				}

				// Ensure deleted objects are gone
				for _, obj := range objectsDeleted {
					err := input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(obj), obj)
					g.Expect(err).To(HaveOccurred())
					g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
				}

				// Ensure other objects are not in deletion.
				for _, obj := range objectsNotInDeletion {
					g.Expect(input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(obj), obj)).ToNot(HaveOccurred())
					g.Expect(obj.GetDeletionTimestamp().IsZero()).To(BeTrue())
				}
			}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

			// Remove the test's finalizer from all objects which had been expected to be in deletion to unblock the next phase.
			removeFinalizer(ctx, input.BootstrapClusterProxy.GetClient(), finalizer, objectsInDeletion...)
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
		Expect(c.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())

		helper, err := patch.NewHelper(obj, c)
		Expect(err).ToNot(HaveOccurred())

		obj.SetFinalizers(append(obj.GetFinalizers(), finalizer))

		Expect(helper.Patch(ctx, obj)).ToNot(HaveOccurred())
	}
}

func removeFinalizer(ctx context.Context, c client.Client, finalizer string, objs ...client.Object) {
	for _, obj := range objs {
		err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		if apierrors.IsNotFound(err) {
			continue
		}
		Expect(err).ToNot(HaveOccurred())

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
		Expect(helper.Patch(ctx, obj)).ToNot(HaveOccurred())
	}
}
