/*
Copyright 2021 The Kubernetes Authors.

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

package cluster

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilfeature "k8s.io/component-base/featuregate/testing"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/exp/topology/scope"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/hooks"
	fakeruntimeclient "sigs.k8s.io/cluster-api/internal/runtime/client/fake"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

var (
	clusterName1                           = "cluster1"
	clusterName2                           = "cluster2"
	clusterName3                           = "cluster3"
	clusterClassName1                      = "class1"
	clusterClassName2                      = "class2"
	infrastructureMachineTemplateName1     = "inframachinetemplate1"
	infrastructureMachineTemplateName2     = "inframachinetemplate2"
	infrastructureMachinePoolTemplateName1 = "inframachinepooltemplate1"
	infrastructureMachinePoolTemplateName2 = "inframachinepooltemplate2"
)

func TestClusterReconciler_reconcileNewlyCreatedCluster(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	g := NewWithT(t)
	timeout := 5 * time.Second

	ns, err := env.CreateNamespace(ctx, "test-topology-cluster-reconcile")
	g.Expect(err).ToNot(HaveOccurred())

	// Create the objects needed for the integration test:
	// - a ClusterClass with all the related templates
	// - a Cluster using the above ClusterClass
	cleanup, err := setupTestEnvForIntegrationTests(ns)
	g.Expect(err).ToNot(HaveOccurred())

	// Defer a cleanup function that deletes each of the objects created during setupTestEnvForIntegrationTests.
	defer func() {
		g.Expect(cleanup()).To(Succeed())
	}()

	g.Eventually(func(g Gomega) error {
		// Get the cluster object.
		actualCluster := &clusterv1.Cluster{}
		if err := env.GetAPIReader().Get(ctx, client.ObjectKey{Name: clusterName1, Namespace: ns.Name}, actualCluster); err != nil {
			return err
		}

		// Check if Cluster has relevant Infrastructure and ControlPlane and labels and annotations.
		g.Expect(assertClusterReconcile(actualCluster)).Should(Succeed())

		// Check if InfrastructureCluster has been created and has the correct labels and annotations.
		g.Expect(assertInfrastructureClusterReconcile(actualCluster)).Should(Succeed())

		// Check if ControlPlane has been created and has the correct version, replicas, labels and annotations.
		g.Expect(assertControlPlaneReconcile(actualCluster)).Should(Succeed())

		// Check if MachineDeployments are created and have the correct version, replicas, labels annotations and templates.
		g.Expect(assertMachineDeploymentsReconcile(actualCluster)).Should(Succeed())

		// Check if MachinePools are created and have the correct version, replicas, labels annotations and templates.
		g.Expect(assertMachinePoolsReconcile(actualCluster)).Should(Succeed())

		// Check if the Cluster has the relevant TopologyReconciledCondition.
		g.Expect(assertClusterTopologyReconciledCondition(actualCluster)).Should(Succeed())

		return nil
	}, timeout).Should(Succeed())
}

func TestClusterReconciler_reconcileMultipleClustersFromOneClass(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)

	g := NewWithT(t)
	timeout := 5 * time.Second

	ns, err := env.CreateNamespace(ctx, "test-topology-cluster-reconcile")
	g.Expect(err).ToNot(HaveOccurred())

	// Create the objects needed for the integration test:
	// - a ClusterClass with all the related templates
	// - a Cluster using the above ClusterClass
	// - a second Cluster using the same ClusterClass
	cleanup, err := setupTestEnvForIntegrationTests(ns)
	g.Expect(err).ToNot(HaveOccurred())

	// Defer a cleanup function that deletes each of the objects created during setupTestEnvForIntegrationTests.
	defer func() {
		g.Expect(cleanup()).To(Succeed())
	}()

	// Check to see that both clusters were correctly created and reconciled using the existing clusterClass.
	g.Eventually(func(g Gomega) error {
		for _, name := range []string{clusterName1, clusterName2} {
			// Get the cluster object.
			actualCluster := &clusterv1.Cluster{}
			if err := env.Get(ctx, client.ObjectKey{Name: name, Namespace: ns.Name}, actualCluster); err != nil {
				return err
			}

			// Check if Cluster has relevant Infrastructure and ControlPlane and labels and annotations.
			g.Expect(assertClusterReconcile(actualCluster)).Should(Succeed())

			// Check if InfrastructureCluster has been created and has the correct labels and annotations.
			g.Expect(assertInfrastructureClusterReconcile(actualCluster)).Should(Succeed())

			// Check if ControlPlane has been created and has the correct version, replicas, labels and annotations.
			g.Expect(assertControlPlaneReconcile(actualCluster)).Should(Succeed())

			// Check if MachineDeployments are created and have the correct version, replicas, labels annotations and templates.
			g.Expect(assertMachineDeploymentsReconcile(actualCluster)).Should(Succeed())

			// Check if MachinePools are created and have the correct version, replicas, labels annotations and templates.
			g.Expect(assertMachinePoolsReconcile(actualCluster)).Should(Succeed())

			// Check if the Cluster has the relevant TopologyReconciledCondition.
			g.Expect(assertClusterTopologyReconciledCondition(actualCluster)).Should(Succeed())
		}
		return nil
	}, timeout).Should(Succeed())
}

func TestClusterReconciler_reconcileUpdateOnClusterTopology(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	g := NewWithT(t)
	timeout := 300 * time.Second

	ns, err := env.CreateNamespace(ctx, "test-topology-cluster-reconcile")
	g.Expect(err).ToNot(HaveOccurred())

	// Create the objects needed for the integration test:
	// - a ClusterClass with all the related templates
	// - a Cluster using the above ClusterClass
	cleanup, err := setupTestEnvForIntegrationTests(ns)
	g.Expect(err).ToNot(HaveOccurred())

	// Defer a cleanup function that deletes each of the objects created during setupTestEnvForIntegrationTests.
	defer func() {
		g.Expect(cleanup()).To(Succeed())
	}()

	actualCluster := &clusterv1.Cluster{}
	// First ensure that the initial cluster and other objects are created and populated as expected.
	g.Eventually(func(g Gomega) error {
		// Get the cluster object now including the updated replica number for the Machine deployment.
		if err := env.Get(ctx, client.ObjectKey{Name: clusterName1, Namespace: ns.Name}, actualCluster); err != nil {
			return err
		}

		// Check if Cluster has relevant Infrastructure and ControlPlane and labels and annotations.
		g.Expect(assertClusterReconcile(actualCluster)).Should(Succeed())

		// Check if InfrastructureCluster has been created and has the correct labels and annotations.
		g.Expect(assertInfrastructureClusterReconcile(actualCluster)).Should(Succeed())

		// Check if ControlPlane has been created and has the correct version, replicas, labels and annotations.
		g.Expect(assertControlPlaneReconcile(actualCluster)).Should(Succeed())

		// Check if MachineDeployments are created and have the correct version, replicas, labels annotations and templates.
		g.Expect(assertMachineDeploymentsReconcile(actualCluster)).Should(Succeed())

		// Check if MachinePools are created and have the correct version, replicas, labels annotations and templates.
		g.Expect(assertMachinePoolsReconcile(actualCluster)).Should(Succeed())

		// Check if the Cluster has the relevant TopologyReconciledCondition.
		g.Expect(assertClusterTopologyReconciledCondition(actualCluster)).Should(Succeed())
		return nil
	}, timeout).Should(Succeed())

	// Change the replicas field in the managed topology of our cluster and update the object in the API.
	replicas := int32(100)
	patchHelper, err := patch.NewHelper(actualCluster, env.Client)
	g.Expect(err).ToNot(HaveOccurred())
	clusterWithTopologyChange := actualCluster.DeepCopy()
	clusterWithTopologyChange.Spec.Topology.Workers.MachineDeployments[0].Replicas = &replicas
	clusterWithTopologyChange.Spec.Topology.Workers.MachinePools[0].Replicas = &replicas
	g.Expect(patchHelper.Patch(ctx, clusterWithTopologyChange)).Should(Succeed())

	// Check to ensure all objects are correctly reconciled with the new MachineDeployment and MachinePool replica count in Topology.
	g.Eventually(func(g Gomega) error {
		// Get the cluster object.
		updatedCluster := &clusterv1.Cluster{}
		if err := env.Get(ctx, client.ObjectKey{Name: clusterName1, Namespace: ns.Name}, updatedCluster); err != nil {
			return err
		}

		// Check to ensure the replica count has been successfully updated in the API server and cache.
		g.Expect(updatedCluster.Spec.Topology.Workers.MachineDeployments[0].Replicas).To(Equal(&replicas))

		// Check to ensure the replica count has been successfully updated in the API server and cache.
		g.Expect(updatedCluster.Spec.Topology.Workers.MachinePools[0].Replicas).To(Equal(&replicas))

		// Check if Cluster has relevant Infrastructure and ControlPlane and labels and annotations.
		g.Expect(assertClusterReconcile(updatedCluster)).Should(Succeed())

		// Check if InfrastructureCluster has been created and has the correct labels and annotations.
		g.Expect(assertInfrastructureClusterReconcile(updatedCluster)).Should(Succeed())

		// Check if ControlPlane has been created and has the correct version, replicas, labels and annotations.
		g.Expect(assertControlPlaneReconcile(updatedCluster)).Should(Succeed())

		// Check if MachineDeployments are created and have the correct version, replicas, labels annotations and templates.
		g.Expect(assertMachineDeploymentsReconcile(updatedCluster)).Should(Succeed())

		// Check if MachinePools are created and have the correct version, replicas, labels annotations and templates.
		g.Expect(assertMachinePoolsReconcile(updatedCluster)).Should(Succeed())

		// Check if the Cluster has the relevant TopologyReconciledCondition.
		g.Expect(assertClusterTopologyReconciledCondition(actualCluster)).Should(Succeed())
		return nil
	}, timeout).Should(Succeed())
}

func TestClusterReconciler_reconcileUpdatesOnClusterClass(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	g := NewWithT(t)
	timeout := 5 * time.Second

	ns, err := env.CreateNamespace(ctx, "test-topology-cluster-reconcile")
	g.Expect(err).ToNot(HaveOccurred())

	// Create the objects needed for the integration test:
	// - a ClusterClass with all the related templates
	// - a Cluster using the above ClusterClass
	// - a second Cluster using the same ClusterClass
	cleanup, err := setupTestEnvForIntegrationTests(ns)
	g.Expect(err).ToNot(HaveOccurred())

	// Defer a cleanup function that deletes each of the objects created during setupTestEnvForIntegrationTests.
	defer func() {
		g.Expect(cleanup()).To(Succeed())
	}()

	actualCluster := &clusterv1.Cluster{}

	g.Eventually(func(g Gomega) error {
		for _, name := range []string{clusterName1, clusterName2} {
			// Get the cluster object.
			if err := env.Get(ctx, client.ObjectKey{Name: name, Namespace: ns.Name}, actualCluster); err != nil {
				return err
			}

			// Check if Cluster has relevant Infrastructure and ControlPlane and labels and annotations.
			g.Expect(assertClusterReconcile(actualCluster)).Should(Succeed())

			// Check if InfrastructureCluster has been created and has the correct labels and annotations.
			g.Expect(assertInfrastructureClusterReconcile(actualCluster)).Should(Succeed())

			// Check if ControlPlane has been created and has the correct version, replicas, labels and annotations.
			g.Expect(assertControlPlaneReconcile(actualCluster)).Should(Succeed())

			// Check if MachineDeployments are created and have the correct version, replicas, labels annotations and templates.
			g.Expect(assertMachineDeploymentsReconcile(actualCluster)).Should(Succeed())

			// Check if MachinePools are created and have the correct version, replicas, labels annotations and templates.
			g.Expect(assertMachinePoolsReconcile(actualCluster)).Should(Succeed())

			// Check if the Cluster has the relevant TopologyReconciledCondition.
			g.Expect(assertClusterTopologyReconciledCondition(actualCluster)).Should(Succeed())
		}
		return nil
	}, timeout).Should(Succeed())

	// Get the clusterClass to update and check if clusterClass updates are being correctly reconciled.
	clusterClass := &clusterv1.ClusterClass{}
	g.Expect(env.Get(ctx, actualCluster.GetClassKey(), clusterClass)).To(Succeed())

	patchHelper, err := patch.NewHelper(clusterClass, env.Client)
	g.Expect(err).ToNot(HaveOccurred())
	// Change the infrastructureMachineTemplateName for the first of our MachineDeployments and update in the API.
	clusterClass.Spec.Workers.MachineDeployments[0].Template.Infrastructure.Ref.Name = infrastructureMachineTemplateName2
	// Change the infrastructureMachinePoolTemplateName for the first of our MachinePools and update in the API.
	clusterClass.Spec.Workers.MachinePools[0].Template.Infrastructure.Ref.Name = infrastructureMachinePoolTemplateName2
	g.Expect(patchHelper.Patch(ctx, clusterClass)).To(Succeed())

	g.Eventually(func(g Gomega) error {
		// Check that the clusterClass has been correctly updated to use the new infrastructure template.
		// This is necessary as sometimes the cache can take a little time to update.
		class := &clusterv1.ClusterClass{}
		g.Expect(env.Get(ctx, actualCluster.GetClassKey(), class)).To(Succeed())
		g.Expect(class.Spec.Workers.MachineDeployments[0].Template.Infrastructure.Ref.Name).To(Equal(infrastructureMachineTemplateName2))
		g.Expect(class.Spec.Workers.MachinePools[0].Template.Infrastructure.Ref.Name).To(Equal(infrastructureMachinePoolTemplateName2))

		// For each cluster check that the clusterClass changes have been correctly reconciled.
		for _, name := range []string{clusterName1, clusterName2} {
			// Get the cluster object.
			actualCluster = &clusterv1.Cluster{}

			if err := env.Get(ctx, client.ObjectKey{Name: name, Namespace: ns.Name}, actualCluster); err != nil {
				return err
			}
			// Check if Cluster has relevant Infrastructure and ControlPlane and labels and annotations.
			g.Expect(assertClusterReconcile(actualCluster)).Should(Succeed())

			// Check if InfrastructureCluster has been created and has the correct labels and annotations.
			g.Expect(assertInfrastructureClusterReconcile(actualCluster)).Should(Succeed())

			// Check if ControlPlane has been created and has the correct version, replicas, labels and annotations.
			g.Expect(assertControlPlaneReconcile(actualCluster)).Should(Succeed())

			// Check if MachineDeployments are created and have the correct version, replicas, labels annotations and templates.
			g.Expect(assertMachineDeploymentsReconcile(actualCluster)).Should(Succeed())

			// Check if MachinePools are created and have the correct version, replicas, labels annotations and templates.
			g.Expect(assertMachinePoolsReconcile(actualCluster)).Should(Succeed())

			// Check if the Cluster has the relevant TopologyReconciledCondition.
			g.Expect(assertClusterTopologyReconciledCondition(actualCluster)).Should(Succeed())
		}
		return nil
	}, timeout).Should(Succeed())
}

func TestClusterReconciler_reconcileClusterClassRebase(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	g := NewWithT(t)
	timeout := 30 * time.Second

	ns, err := env.CreateNamespace(ctx, "test-topology-cluster-reconcile")
	g.Expect(err).ToNot(HaveOccurred())

	// Create the objects needed for the integration test:
	// - a ClusterClass with all the related templates
	// - a Cluster using the first ClusterClass
	// - a compatible ClusterClass to rebase the Cluster to
	cleanup, err := setupTestEnvForIntegrationTests(ns)
	g.Expect(err).ToNot(HaveOccurred())

	// Defer a cleanup function that deletes each of the objects created during setupTestEnvForIntegrationTests.
	defer func() {
		g.Expect(cleanup()).To(Succeed())
	}()

	actualCluster := &clusterv1.Cluster{}
	// First ensure that the initial cluster and other objects are created and populated as expected.
	g.Eventually(func(g Gomega) error {
		// Get the cluster object.
		if err := env.Get(ctx, client.ObjectKey{Name: clusterName1, Namespace: ns.Name}, actualCluster); err != nil {
			return err
		}

		// Check if Cluster has relevant Infrastructure and ControlPlane and labels and annotations.
		g.Expect(assertClusterReconcile(actualCluster)).Should(Succeed())

		// Check if InfrastructureCluster has been created and has the correct labels and annotations.
		g.Expect(assertInfrastructureClusterReconcile(actualCluster)).Should(Succeed())

		// Check if ControlPlane has been created and has the correct version, replicas, labels and annotations.
		g.Expect(assertControlPlaneReconcile(actualCluster)).Should(Succeed())

		// Check if MachineDeployments are created and have the correct version, replicas, labels annotations and templates.
		g.Expect(assertMachineDeploymentsReconcile(actualCluster)).Should(Succeed())

		// Check if MachinePools are created and have the correct version, replicas, labels annotations and templates.
		g.Expect(assertMachinePoolsReconcile(actualCluster)).Should(Succeed())

		return nil
	}, timeout).Should(Succeed())

	patchHelper, err := patch.NewHelper(actualCluster, env.Client)
	g.Expect(err).ToNot(HaveOccurred())
	// Change the ClusterClass pointed to in the Cluster's Topology. This is a ClusterClass rebase operation.
	clusterWithRebase := actualCluster.DeepCopy()
	clusterWithRebase.Spec.Topology.Class = clusterClassName2
	g.Expect(patchHelper.Patch(ctx, clusterWithRebase)).Should(Succeed())

	// Check to ensure all objects are correctly reconciled with the new ClusterClass.
	g.Eventually(func(g Gomega) error {
		// Get the cluster object.
		updatedCluster := &clusterv1.Cluster{}
		if err := env.Get(ctx, client.ObjectKey{Name: clusterName1, Namespace: ns.Name}, updatedCluster); err != nil {
			return err
		}
		// Check to ensure the spec.topology.class has been successfully updated in the API server and cache.
		g.Expect(updatedCluster.GetClassKey().Name).To(Equal(clusterClassName2))
		// Check if Cluster has relevant Infrastructure and ControlPlane and labels and annotations.
		g.Expect(assertClusterReconcile(updatedCluster)).Should(Succeed())

		// Check if InfrastructureCluster has been created and has the correct labels and annotations.
		g.Expect(assertInfrastructureClusterReconcile(updatedCluster)).Should(Succeed())

		// Check if ControlPlane has been created and has the correct version, replicas, labels and annotations.
		g.Expect(assertControlPlaneReconcile(updatedCluster)).Should(Succeed())

		// Check if MachineDeployments are created and have the correct version, replicas, labels annotations and templates.
		g.Expect(assertMachineDeploymentsReconcile(updatedCluster)).Should(Succeed())

		// Check if MachinePools are created and have the correct version, replicas, labels annotations and templates.
		g.Expect(assertMachinePoolsReconcile(actualCluster)).Should(Succeed())

		return nil
	}, timeout).Should(Succeed())
}

func TestClusterReconciler_reconcileDelete(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, true)

	catalog := runtimecatalog.New()
	_ = runtimehooksv1.AddToCatalog(catalog)

	beforeClusterDeleteGVH, err := catalog.GroupVersionHook(runtimehooksv1.BeforeClusterDelete)
	if err != nil {
		panic(err)
	}

	blockingResponse := &runtimehooksv1.BeforeClusterDeleteResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			RetryAfterSeconds: int32(10),
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
		},
	}
	nonBlockingResponse := &runtimehooksv1.BeforeClusterDeleteResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			RetryAfterSeconds: int32(0),
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
		},
	}
	failureResponse := &runtimehooksv1.BeforeClusterDeleteResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusFailure,
			},
		},
	}

	tests := []struct {
		name               string
		cluster            *clusterv1.Cluster
		hookResponse       *runtimehooksv1.BeforeClusterDeleteResponse
		wantHookToBeCalled bool
		wantResult         ctrl.Result
		wantOkToDelete     bool
		wantErr            bool
	}{
		{
			name: "should apply the ok-to-delete annotation if the BeforeClusterDelete hook returns a non-blocking response",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
				},
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{},
				},
			},
			hookResponse:       nonBlockingResponse,
			wantResult:         ctrl.Result{},
			wantHookToBeCalled: true,
			wantOkToDelete:     true,
			wantErr:            false,
		},
		{
			name: "should requeue if the BeforeClusterDelete hook returns a blocking response",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
				},
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{},
				},
			},
			hookResponse:       blockingResponse,
			wantResult:         ctrl.Result{RequeueAfter: time.Duration(10) * time.Second},
			wantHookToBeCalled: true,
			wantOkToDelete:     false,
			wantErr:            false,
		},
		{
			name: "should fail if the BeforeClusterDelete hook returns a failure response",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
				},
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{},
				},
			},
			hookResponse:       failureResponse,
			wantResult:         ctrl.Result{},
			wantHookToBeCalled: true,
			wantOkToDelete:     false,
			wantErr:            true,
		},
		{
			name: "should succeed if the ok-to-delete annotation is already present",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
					Annotations: map[string]string{
						// If the hook is already marked the hook should not be called during cluster delete.
						runtimev1.OkToDeleteAnnotation: "",
					},
				},
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{},
				},
			},
			// Using a blocking response here should not matter as the hook should never be called.
			// Using a blocking response to enforce the point.
			hookResponse:       blockingResponse,
			wantResult:         ctrl.Result{},
			wantHookToBeCalled: false,
			wantOkToDelete:     true,
			wantErr:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeClient := fake.NewClientBuilder().WithObjects(tt.cluster).Build()
			fakeRuntimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
				WithCallAllExtensionResponses(map[runtimecatalog.GroupVersionHook]runtimehooksv1.ResponseObject{
					beforeClusterDeleteGVH: tt.hookResponse,
				}).
				WithCatalog(catalog).
				Build()

			r := &Reconciler{
				Client:        fakeClient,
				APIReader:     fakeClient,
				RuntimeClient: fakeRuntimeClient,
			}

			res, err := r.reconcileDelete(ctx, tt.cluster)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(res).To(BeComparableTo(tt.wantResult))
				g.Expect(hooks.IsOkToDelete(tt.cluster)).To(Equal(tt.wantOkToDelete))
				g.Expect(fakeRuntimeClient.CallAllCount(runtimehooksv1.BeforeClusterDelete) == 1).To(Equal(tt.wantHookToBeCalled))
			}
		})
	}
}

// TestClusterReconciler_deleteClusterClass tests the correct deletion behaviour for a ClusterClass with references in existing Clusters.
// In this case deletion of the ClusterClass should be blocked by the webhook.
func TestClusterReconciler_deleteClusterClass(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	g := NewWithT(t)
	timeout := 5 * time.Second

	ns, err := env.CreateNamespace(ctx, "test-topology-cluster-reconcile")
	g.Expect(err).ToNot(HaveOccurred())

	// Create the objects needed for the integration test:
	// - a ClusterClass with all the related templates
	// - a Cluster using the above ClusterClass
	cleanup, err := setupTestEnvForIntegrationTests(ns)
	g.Expect(err).ToNot(HaveOccurred())

	// Defer a cleanup function that deletes each of the objects created during setupTestEnvForIntegrationTests.
	defer func() {
		g.Expect(cleanup()).To(Succeed())
	}()

	actualCluster := &clusterv1.Cluster{}

	g.Eventually(func(g Gomega) error {
		for _, name := range []string{clusterName1, clusterName2} {
			// Get the cluster object.
			if err := env.Get(ctx, client.ObjectKey{Name: name, Namespace: ns.Name}, actualCluster); err != nil {
				return err
			}

			// Check if Cluster has relevant Infrastructure and ControlPlane and labels and annotations.
			g.Expect(assertClusterReconcile(actualCluster)).Should(Succeed())

			// Check if InfrastructureCluster has been created and has the correct labels and annotations.
			g.Expect(assertInfrastructureClusterReconcile(actualCluster)).Should(Succeed())

			// Check if ControlPlane has been created and has the correct version, replicas, labels and annotations.
			g.Expect(assertControlPlaneReconcile(actualCluster)).Should(Succeed())

			// Check if MachineDeployments are created and have the correct version, replicas, labels annotations and templates.
			g.Expect(assertMachineDeploymentsReconcile(actualCluster)).Should(Succeed())

			// Check if MachinePools are created and have the correct version, replicas, labels annotations and templates.
			g.Expect(assertMachinePoolsReconcile(actualCluster)).Should(Succeed())
		}
		return nil
	}, timeout).Should(Succeed())

	// Ensure the clusterClass is available in the API server .
	clusterClass := &clusterv1.ClusterClass{}
	g.Expect(env.Get(ctx, actualCluster.GetClassKey(), clusterClass)).To(Succeed())

	// Attempt to delete the ClusterClass. Expect an error here as the ClusterClass deletion is blocked by the webhook.
	g.Expect(env.Delete(ctx, clusterClass)).NotTo(Succeed())
}

func TestReconciler_callBeforeClusterCreateHook(t *testing.T) {
	catalog := runtimecatalog.New()
	_ = runtimehooksv1.AddToCatalog(catalog)
	gvh, err := catalog.GroupVersionHook(runtimehooksv1.BeforeClusterCreate)
	if err != nil {
		panic(err)
	}

	blockingResponse := &runtimehooksv1.BeforeClusterCreateResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
			RetryAfterSeconds: int32(10),
		},
	}
	nonBlockingResponse := &runtimehooksv1.BeforeClusterCreateResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
			RetryAfterSeconds: int32(0),
		},
	}
	failingResponse := &runtimehooksv1.BeforeClusterCreateResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusFailure,
			},
		},
	}

	tests := []struct {
		name         string
		hookResponse *runtimehooksv1.BeforeClusterCreateResponse
		wantResult   reconcile.Result
		wantErr      bool
	}{
		{
			name:         "should return a requeue response when the BeforeClusterCreate hook is blocking",
			hookResponse: blockingResponse,
			wantResult:   ctrl.Result{RequeueAfter: time.Duration(10) * time.Second},
			wantErr:      false,
		},
		{
			name:         "should return an empty response when the BeforeClusterCreate hook is not blocking",
			hookResponse: nonBlockingResponse,
			wantResult:   ctrl.Result{},
			wantErr:      false,
		},
		{
			name:         "should error when the BeforeClusterCreate hook returns a failure response",
			hookResponse: failingResponse,
			wantResult:   ctrl.Result{},
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			runtimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
				WithCatalog(catalog).
				WithCallAllExtensionResponses(map[runtimecatalog.GroupVersionHook]runtimehooksv1.ResponseObject{
					gvh: tt.hookResponse,
				}).
				Build()

			r := &Reconciler{
				RuntimeClient: runtimeClient,
			}
			s := &scope.Scope{
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{},
				},
				HookResponseTracker: scope.NewHookResponseTracker(),
			}
			res, err := r.callBeforeClusterCreateHook(ctx, s)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(res).To(BeComparableTo(tt.wantResult))
			}
		})
	}
}

// setupTestEnvForIntegrationTests builds and then creates in the envtest API server all objects required at init time for each of the
// integration tests in this file. This includes:
// - a first clusterClass with all the related templates
// - a second clusterClass, compatible with the first, used to test a ClusterClass rebase
// - a first Cluster using the above ClusterClass
// - a second Cluster using the above ClusterClass, but with different version/Machine deployment definition
// NOTE: The objects are created for every test, though some may not be used in every test.
func setupTestEnvForIntegrationTests(ns *corev1.Namespace) (func() error, error) {
	workerClassName1 := "linux-worker"
	workerClassName2 := "windows-worker"
	workerClassName3 := "solaris-worker"

	// The below objects are created in order to feed the reconcile loop all the information it needs to create a full
	// Cluster given a skeletal Cluster object and a ClusterClass. The objects include:

	// 1) Templates for Machine, Cluster, ControlPlane and Bootstrap.
	infrastructureMachineTemplate1 := builder.TestInfrastructureMachineTemplate(ns.Name, infrastructureMachineTemplateName1).Build()
	infrastructureMachineTemplate2 := builder.TestInfrastructureMachineTemplate(ns.Name, infrastructureMachineTemplateName2).
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()
	infrastructureMachinePoolTemplate1 := builder.TestInfrastructureMachinePoolTemplate(ns.Name, infrastructureMachinePoolTemplateName1).Build()
	infrastructureMachinePoolTemplate2 := builder.TestInfrastructureMachinePoolTemplate(ns.Name, infrastructureMachinePoolTemplateName2).
		WithSpecFields(map[string]interface{}{"spec.template.fakeSetting": true}).
		Build()
	infrastructureClusterTemplate1 := builder.TestInfrastructureClusterTemplate(ns.Name, "infraclustertemplate1").
		Build()
	infrastructureClusterTemplate2 := builder.TestInfrastructureClusterTemplate(ns.Name, "infraclustertemplate2").
		WithSpecFields(map[string]interface{}{"spec.template.spec.alteredSetting": true}).
		Build()
	controlPlaneTemplate := builder.TestControlPlaneTemplate(ns.Name, "cp1").
		WithInfrastructureMachineTemplate(infrastructureMachineTemplate1).
		Build()
	bootstrapTemplate := builder.TestBootstrapTemplate(ns.Name, "bootstraptemplate").Build()

	// 2) ClusterClass definitions including definitions of MachineDeploymentClasses and MachinePoolClasses used inside the ClusterClass.
	machineDeploymentClass1 := builder.MachineDeploymentClass(workerClassName1 + "-md").
		WithInfrastructureTemplate(infrastructureMachineTemplate1).
		WithBootstrapTemplate(bootstrapTemplate).
		WithLabels(map[string]string{"foo": "bar"}).
		WithAnnotations(map[string]string{"foo": "bar"}).
		Build()
	machineDeploymentClass2 := builder.MachineDeploymentClass(workerClassName2 + "-md").
		WithInfrastructureTemplate(infrastructureMachineTemplate1).
		WithBootstrapTemplate(bootstrapTemplate).
		Build()
	machineDeploymentClass3 := builder.MachineDeploymentClass(workerClassName3 + "-md").
		WithInfrastructureTemplate(infrastructureMachineTemplate2).
		WithBootstrapTemplate(bootstrapTemplate).
		Build()
	machinePoolClass1 := builder.MachinePoolClass(workerClassName1 + "-mp").
		WithInfrastructureTemplate(infrastructureMachinePoolTemplate1).
		WithBootstrapTemplate(bootstrapTemplate).
		WithLabels(map[string]string{"foo": "bar"}).
		WithAnnotations(map[string]string{"foo": "bar"}).
		Build()
	machinePoolClass2 := builder.MachinePoolClass(workerClassName2 + "-mp").
		WithInfrastructureTemplate(infrastructureMachinePoolTemplate1).
		WithBootstrapTemplate(bootstrapTemplate).
		Build()
	machinePoolClass3 := builder.MachinePoolClass(workerClassName3 + "-mp").
		WithInfrastructureTemplate(infrastructureMachinePoolTemplate2).
		WithBootstrapTemplate(bootstrapTemplate).
		Build()
	clusterClass := builder.ClusterClass(ns.Name, clusterClassName1).
		WithInfrastructureClusterTemplate(infrastructureClusterTemplate1).
		WithControlPlaneTemplate(controlPlaneTemplate).
		WithControlPlaneInfrastructureMachineTemplate(infrastructureMachineTemplate1).
		WithWorkerMachineDeploymentClasses(*machineDeploymentClass1, *machineDeploymentClass2).
		WithWorkerMachinePoolClasses(*machinePoolClass1, *machinePoolClass2).
		Build()

	// This ClusterClass changes a number of things in a ClusterClass in a way that is compatible for a ClusterClass rebase operation.
	// 1) It changes the controlPlaneMachineInfrastructureTemplate to a new template.
	// 2) It adds a new machineDeploymentClass and machinePoolClass
	// 3) It changes the infrastructureClusterTemplate.
	clusterClassForRebase := builder.ClusterClass(ns.Name, clusterClassName2).
		WithInfrastructureClusterTemplate(infrastructureClusterTemplate2).
		WithControlPlaneTemplate(controlPlaneTemplate).
		WithControlPlaneInfrastructureMachineTemplate(infrastructureMachineTemplate2).
		WithWorkerMachineDeploymentClasses(*machineDeploymentClass1, *machineDeploymentClass2, *machineDeploymentClass3).
		WithWorkerMachinePoolClasses(*machinePoolClass1, *machinePoolClass2, *machinePoolClass3).
		Build()

	// 3) Two Clusters including a Cluster Topology objects and the MachineDeploymentTopology and MachinePoolTopology objects used in the
	// Cluster Topology. The second cluster differs from the first both in version and in its MachineDeployment and MachinePool definitions.
	machineDeploymentTopology1 := builder.MachineDeploymentTopology("mdm1").
		WithClass(workerClassName1 + "-md").
		WithReplicas(3).
		Build()
	machineDeploymentTopology2 := builder.MachineDeploymentTopology("mdm2").
		WithClass(workerClassName2 + "-md").
		WithReplicas(1).
		Build()
	machinePoolTopology1 := builder.MachinePoolTopology("mp1").
		WithClass(workerClassName1 + "-mp").
		WithReplicas(3).
		Build()
	machinePoolTopology2 := builder.MachinePoolTopology("mp2").
		WithClass(workerClassName2 + "-mp").
		WithReplicas(1).
		Build()

	cluster1 := builder.Cluster(ns.Name, clusterName1).
		WithTopology(
			builder.ClusterTopology().
				WithClass(clusterClass.Name).
				WithMachineDeployment(machineDeploymentTopology1).
				WithMachineDeployment(machineDeploymentTopology2).
				WithMachinePool(machinePoolTopology1).
				WithMachinePool(machinePoolTopology2).
				WithVersion("1.22.2").
				WithControlPlaneReplicas(3).
				Build()).
		Build()

	cluster2 := builder.Cluster(ns.Name, clusterName2).
		WithTopology(
			builder.ClusterTopology().
				WithClass(clusterClass.Name).
				WithMachineDeployment(machineDeploymentTopology2).
				WithMachinePool(machinePoolTopology2).
				WithVersion("1.21.0").
				WithControlPlaneReplicas(1).
				Build()).
		Build()

	// Cross ns referencing cluster
	cluster3 := builder.Cluster(ns.Name, clusterName3).
		WithTopology(
			builder.ClusterTopology().
				WithClass(clusterClass.Name).
				WithClassNamespace("other").
				WithMachineDeployment(machineDeploymentTopology2).
				WithMachinePool(machinePoolTopology2).
				WithVersion("1.21.0").
				WithControlPlaneReplicas(1).
				Build()).
		Build()

	// Setup kubeconfig secrets for the clusters, so the ClusterCacheTracker works.
	cluster1Secret := kubeconfig.GenerateSecret(cluster1, kubeconfig.FromEnvTestConfig(env.Config, cluster1))
	cluster2Secret := kubeconfig.GenerateSecret(cluster2, kubeconfig.FromEnvTestConfig(env.Config, cluster2))
	// Unset the ownerrefs otherwise they are invalid because they contain an empty uid.
	cluster1Secret.ObjectMeta.OwnerReferences = nil
	cluster2Secret.ObjectMeta.OwnerReferences = nil

	// Create a set of setupTestEnvForIntegrationTests from the objects above to add to the API server when the test environment starts.
	// The objects are created for every test, though some e.g. infrastructureMachineTemplate2 may not be used in every test.
	initObjs := []client.Object{
		infrastructureClusterTemplate1,
		infrastructureClusterTemplate2,
		infrastructureMachineTemplate1,
		infrastructureMachineTemplate2,
		infrastructureMachinePoolTemplate1,
		infrastructureMachinePoolTemplate2,
		bootstrapTemplate,
		controlPlaneTemplate,
		clusterClass,
		clusterClassForRebase,
		cluster1,
		cluster2,
		cluster3,
		cluster1Secret,
		cluster2Secret,
	}
	cleanup := func() error {
		// Delete Objects in reverse, because we cannot delete a ClusterCLass if it is still used by a Cluster.
		for i := len(initObjs) - 1; i >= 0; i-- {
			if err := env.CleanupAndWait(ctx, initObjs[i]); err != nil {
				return err
			}
		}
		return nil
	}

	for _, obj := range initObjs {
		if err := env.CreateAndWait(ctx, obj); err != nil {
			return cleanup, err
		}
	}
	// Set InfrastructureReady to true so ClusterCache creates the clusterAccessors.
	patch := client.MergeFrom(cluster1.DeepCopy())
	cluster1.Status.InfrastructureReady = true
	if err := env.Status().Patch(ctx, cluster1, patch); err != nil {
		return nil, err
	}
	patch = client.MergeFrom(cluster2.DeepCopy())
	cluster2.Status.InfrastructureReady = true
	if err := env.Status().Patch(ctx, cluster2, patch); err != nil {
		return nil, err
	}

	return cleanup, nil
}

func assertClusterTopologyReconciledCondition(cluster *clusterv1.Cluster) error {
	if !conditions.Has(cluster, clusterv1.TopologyReconciledCondition) {
		return fmt.Errorf("cluster should have the TopologyReconciled condition set")
	}
	return nil
}

// assertClusterReconcile checks if the Cluster object:
// 1) Has its InfrastructureReference and ControlPlane reference added correctly.
// 2) InfrastructureReference and ControlPlaneRef have the expected Group, Version and Kind.
func assertClusterReconcile(cluster *clusterv1.Cluster) error {
	// Check if relevant managed topology labels are present.
	if err := assertClusterTopologyOwnedLabel(cluster); err != nil {
		return err
	}

	// Check if InfrastructureRef exists and is of the expected Kind and APIVersion.
	if err := referenceExistsWithCorrectKindAndAPIVersion(cluster.Spec.InfrastructureRef,
		builder.TestInfrastructureClusterKind,
		builder.InfrastructureGroupVersion); err != nil {
		return err
	}

	// Check if ControlPlaneRef exists is of the expected Kind and APIVersion.
	return referenceExistsWithCorrectKindAndAPIVersion(cluster.Spec.ControlPlaneRef,
		builder.TestControlPlaneKind,
		builder.ControlPlaneGroupVersion)
}

// assertInfrastructureClusterReconcile checks if the infrastructureCluster object:
// 1) Is created.
// 2) Has the correct labels and annotations.
func assertInfrastructureClusterReconcile(cluster *clusterv1.Cluster) error {
	_, err := getAndAssertLabelsAndAnnotations(*cluster.Spec.InfrastructureRef, cluster.Name)
	return err
}

// assertControlPlaneReconcile checks if the ControlPlane object:
//  1. Is created.
//  2. Has the correct labels and annotations.
//  3. If it requires ControlPlane Infrastructure and if so:
//     i) That the infrastructureMachineTemplate is created correctly.
//     ii) That the infrastructureMachineTemplate has the correct labels and annotations
func assertControlPlaneReconcile(cluster *clusterv1.Cluster) error {
	cp, err := getAndAssertLabelsAndAnnotations(*cluster.Spec.ControlPlaneRef, cluster.Name)
	if err != nil {
		return err
	}
	// Check if the ControlPlane Version matches the version in the Cluster's managed topology spec.
	version, err := contract.ControlPlane().Version().Get(cp)
	if err != nil {
		return err
	}

	if *version != cluster.Spec.Topology.Version {
		return fmt.Errorf("version %v does not match expected %v", *version, cluster.Spec.Topology.Version)
	}

	// Check for Control Plane replicase if it's set in the Cluster.Spec.Topology
	if cluster.Spec.Topology.ControlPlane.Replicas != nil {
		replicas, err := contract.ControlPlane().Replicas().Get(cp)
		if err != nil {
			return err
		}

		// Check for Control Plane replicase if it's set in the Cluster.Spec.Topology
		if int32(*replicas) != *cluster.Spec.Topology.ControlPlane.Replicas {
			return fmt.Errorf("replicas %v do not match expected %v", int32(*replicas), *cluster.Spec.Topology.ControlPlane.Replicas)
		}
	}
	clusterClass := &clusterv1.ClusterClass{}
	if err := env.Get(ctx, cluster.GetClassKey(), clusterClass); err != nil {
		return err
	}
	// Check for the ControlPlaneInfrastructure if it's referenced in the clusterClass.
	if clusterClass.Spec.ControlPlane.MachineInfrastructure != nil && clusterClass.Spec.ControlPlane.MachineInfrastructure.Ref != nil {
		cpInfra, err := contract.ControlPlane().MachineTemplate().InfrastructureRef().Get(cp)
		if err != nil {
			return err
		}
		if err := referenceExistsWithCorrectKindAndAPIVersion(cpInfra,
			builder.TestInfrastructureMachineTemplateKind,
			builder.InfrastructureGroupVersion); err != nil {
			return err
		}
		if _, err := getAndAssertLabelsAndAnnotations(*cpInfra, cluster.Name); err != nil {
			return err
		}
	}
	return nil
}

// assertMachineDeploymentsReconcile checks if the MachineDeployments:
// 1) Are created in the correct number.
// 2) Have the correct labels (TopologyOwned, ClusterName, MachineDeploymentName).
// 3) Have the correct replicas and version.
// 4) Have the correct Kind/APIVersion and Labels/Annotations for BoostrapRef and InfrastructureRef templates.
func assertMachineDeploymentsReconcile(cluster *clusterv1.Cluster) error {
	// List all created machine deployments to assert the expected numbers are created.
	machineDeployments := &clusterv1.MachineDeploymentList{}
	if err := env.List(ctx, machineDeployments, client.InNamespace(cluster.Namespace)); err != nil {
		return err
	}

	// clusterMDs will hold the MachineDeployments that have labels associating them with the cluster.
	clusterMDs := []clusterv1.MachineDeployment{}

	// Run through all machine deployments and add only those with the TopologyOwnedLabel and the correct
	// ClusterNameLabel to the items for further testing.
	for _, m := range machineDeployments.Items {
		// If the machineDeployment doesn't have the ClusterTopologyOwnedLabel and the ClusterNameLabel ignore.
		md := m
		if err := assertClusterTopologyOwnedLabel(&md); err != nil {
			continue
		}
		if err := assertClusterNameLabel(&md, cluster.Name); err != nil {
			continue
		}
		clusterMDs = append(clusterMDs, md)
	}

	// If the total number of machine deployments is not as expected return false.
	if len(clusterMDs) != len(cluster.Spec.Topology.Workers.MachineDeployments) {
		return fmt.Errorf("number of MachineDeployments %v does not match number expected %v", len(clusterMDs), len(cluster.Spec.Topology.Workers.MachineDeployments))
	}
	for _, m := range clusterMDs {
		for _, topologyMD := range cluster.Spec.Topology.Workers.MachineDeployments {
			md := m
			// use the ClusterTopologyMachineDeploymentLabel to get the specific machineDeployment to compare to.
			if topologyMD.Name != md.GetLabels()[clusterv1.ClusterTopologyMachineDeploymentNameLabel] {
				continue
			}

			// Check if the ClusterTopologyLabelName and ClusterTopologyOwnedLabel are set correctly.
			if err := assertClusterTopologyOwnedLabel(&md); err != nil {
				return err
			}

			if err := assertClusterNameLabel(&md, cluster.Name); err != nil {
				return err
			}

			// Check replicas and version for the MachineDeployment.
			if *md.Spec.Replicas != *topologyMD.Replicas {
				return fmt.Errorf("replicas %v does not match expected %v", *md.Spec.Replicas, *topologyMD.Replicas)
			}
			if *md.Spec.Template.Spec.Version != cluster.Spec.Topology.Version {
				return fmt.Errorf("version %v does not match expected %v", *md.Spec.Template.Spec.Version, cluster.Spec.Topology.Version)
			}

			// Check if the InfrastructureReference exists.
			if err := referenceExistsWithCorrectKindAndAPIVersion(&md.Spec.Template.Spec.InfrastructureRef,
				builder.TestInfrastructureMachineTemplateKind,
				builder.InfrastructureGroupVersion); err != nil {
				return err
			}

			// Check if the InfrastructureReference has the expected labels and annotations.
			if _, err := getAndAssertLabelsAndAnnotations(md.Spec.Template.Spec.InfrastructureRef, cluster.Name); err != nil {
				return err
			}

			// Check if the Bootstrap reference has the expected Kind and APIVersion.
			if err := referenceExistsWithCorrectKindAndAPIVersion(md.Spec.Template.Spec.Bootstrap.ConfigRef,
				builder.TestBootstrapConfigTemplateKind,
				builder.BootstrapGroupVersion); err != nil {
				return err
			}

			// Check if the Bootstrap reference has the expected labels and annotations.
			if _, err := getAndAssertLabelsAndAnnotations(*md.Spec.Template.Spec.Bootstrap.ConfigRef, cluster.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

// assertMachinePoolsReconcile checks if the MachinePools:
// 1) Are created in the correct number.
// 2) Have the correct labels (TopologyOwned, ClusterName, MachinePoolName).
// 3) Have the correct replicas and version.
// 4) Have the correct Kind/APIVersion and Labels/Annotations for BoostrapRef and InfrastructureRef templates.
func assertMachinePoolsReconcile(cluster *clusterv1.Cluster) error {
	// List all created machine pools to assert the expected numbers are created.
	machinePools := &expv1.MachinePoolList{}
	if err := env.List(ctx, machinePools, client.InNamespace(cluster.Namespace)); err != nil {
		return err
	}

	// clusterMPs will hold the MachinePools that have labels associating them with the cluster.
	clusterMPs := []expv1.MachinePool{}

	// Run through all machine pools and add only those with the TopologyOwnedLabel and the correct
	// ClusterNameLabel to the items for further testing.
	for _, m := range machinePools.Items {
		// If the machinePool doesn't have the ClusterTopologyOwnedLabel and the ClusterNameLabel ignore.
		mp := m
		if err := assertClusterTopologyOwnedLabel(&mp); err != nil {
			continue
		}
		if err := assertClusterNameLabel(&mp, cluster.Name); err != nil {
			continue
		}
		clusterMPs = append(clusterMPs, mp)
	}

	// If the total number of machine pools is not as expected return false.
	if len(clusterMPs) != len(cluster.Spec.Topology.Workers.MachinePools) {
		return fmt.Errorf("number of MachinePools %v does not match number expected %v", len(clusterMPs), len(cluster.Spec.Topology.Workers.MachinePools))
	}
	for _, m := range clusterMPs {
		for _, topologyMP := range cluster.Spec.Topology.Workers.MachinePools {
			mp := m
			// use the ClusterTopologyMachinePoolLabel to get the specific machinePool to compare to.
			if topologyMP.Name != mp.GetLabels()[clusterv1.ClusterTopologyMachinePoolNameLabel] {
				continue
			}

			// Check if the ClusterTopologyLabelName and ClusterTopologyOwnedLabel are set correctly.
			if err := assertClusterTopologyOwnedLabel(&mp); err != nil {
				return err
			}

			if err := assertClusterNameLabel(&mp, cluster.Name); err != nil {
				return err
			}

			// Check replicas and version for the MachinePool.
			if *mp.Spec.Replicas != *topologyMP.Replicas {
				return fmt.Errorf("replicas %v does not match expected %v", mp.Spec.Replicas, topologyMP.Replicas)
			}
			if *mp.Spec.Template.Spec.Version != cluster.Spec.Topology.Version {
				return fmt.Errorf("version %v does not match expected %v", *mp.Spec.Template.Spec.Version, cluster.Spec.Topology.Version)
			}

			// Check if the InfrastructureReference exists.
			if err := referenceExistsWithCorrectKindAndAPIVersion(&mp.Spec.Template.Spec.InfrastructureRef,
				builder.TestInfrastructureMachinePoolKind,
				builder.InfrastructureGroupVersion); err != nil {
				return err
			}

			// Check if the InfrastructureReference has the expected labels and annotations.
			if _, err := getAndAssertLabelsAndAnnotations(mp.Spec.Template.Spec.InfrastructureRef, cluster.Name); err != nil {
				return err
			}

			// Check if the Bootstrap reference has the expected Kind and APIVersion.
			if err := referenceExistsWithCorrectKindAndAPIVersion(mp.Spec.Template.Spec.Bootstrap.ConfigRef,
				builder.TestBootstrapConfigKind,
				builder.BootstrapGroupVersion); err != nil {
				return err
			}

			// Check if the Bootstrap reference has the expected labels and annotations.
			if _, err := getAndAssertLabelsAndAnnotations(*mp.Spec.Template.Spec.Bootstrap.ConfigRef, cluster.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

// getAndAssertLabelsAndAnnotations pulls the template referenced in the ObjectReference from the API server, checks for:
// 1) The ClusterTopologyOwnedLabel.
// 2) The correct ClusterNameLabel.
// 3) The annotation stating where the template was cloned from.
// The function returns the unstructured object and a bool indicating if it passed all tests.
func getAndAssertLabelsAndAnnotations(template corev1.ObjectReference, clusterName string) (*unstructured.Unstructured, error) {
	got := &unstructured.Unstructured{}
	got.SetKind(template.Kind)
	got.SetAPIVersion(template.APIVersion)

	if err := env.Get(ctx, client.ObjectKey{Name: template.Name, Namespace: template.Namespace}, got); err != nil {
		return nil, err
	}

	if err := assertLabelsAndAnnotations(got, clusterName); err != nil {
		return nil, err
	}
	return got, nil
}

// assertLabelsAndAnnotations runs the specific label checks required to assert that an unstructured object has been
// correctly created by a clusterClass reconciliation.
func assertLabelsAndAnnotations(got client.Object, clusterName string) error {
	if err := assertClusterTopologyOwnedLabel(got); err != nil {
		return err
	}
	if err := assertClusterNameLabel(got, clusterName); err != nil {
		return err
	}
	return assertTemplateClonedFromNameAnnotation(got)
}

// assertClusterTopologyOwnedLabel asserts the label exists.
func assertClusterTopologyOwnedLabel(got client.Object) error {
	_, ok := got.GetLabels()[clusterv1.ClusterTopologyOwnedLabel]
	if !ok {
		return fmt.Errorf("%v not found on %v: %v", clusterv1.ClusterTopologyOwnedLabel, got.GetObjectKind().GroupVersionKind().Kind, got.GetName())
	}
	return nil
}

// assertClusterNameLabel asserts the label exists and is set to the correct value.
func assertClusterNameLabel(got client.Object, clusterName string) error {
	v, ok := got.GetLabels()[clusterv1.ClusterNameLabel]
	if !ok {
		return fmt.Errorf("%v not found in %v: %v", clusterv1.ClusterNameLabel, got.GetObjectKind().GroupVersionKind().Kind, got.GetName())
	}
	if v != clusterName {
		return fmt.Errorf("%v %v does not match expected %v", clusterv1.ClusterNameLabel, v, clusterName)
	}
	return nil
}

// assertTemplateClonedFromNameAnnotation asserts the annotation exists. This check does not assert that the template
// named in the annotation is as expected.
func assertTemplateClonedFromNameAnnotation(got client.Object) error {
	_, ok := got.GetAnnotations()[clusterv1.TemplateClonedFromNameAnnotation]
	if !ok {
		return fmt.Errorf("%v not found in %v; %v", clusterv1.TemplateClonedFromNameAnnotation, got.GetObjectKind().GroupVersionKind().Kind, got.GetName())
	}
	return nil
}

// referenceExistsWithCorrectKindAndAPIVersion asserts that the passed ObjectReference is not nil and that it has the correct kind and apiVersion.
func referenceExistsWithCorrectKindAndAPIVersion(reference *corev1.ObjectReference, kind string, apiVersion schema.GroupVersion) error {
	if reference == nil {
		return fmt.Errorf("object reference passed was nil")
	}
	if reference.Kind != kind {
		return fmt.Errorf("object reference kind %v does not match expected %v", reference.Kind, kind)
	}
	if reference.APIVersion != apiVersion.String() {
		return fmt.Errorf("apiVersion %v does not match expected %v", reference.APIVersion, apiVersion.String())
	}
	return nil
}

func TestReconciler_DefaultCluster(t *testing.T) {
	g := NewWithT(t)
	classBuilder := builder.ClusterClass(metav1.NamespaceDefault, clusterClassName1)
	topologyBase := builder.ClusterTopology().
		WithClass(clusterClassName1).
		WithVersion("1.22.2").
		WithControlPlaneReplicas(3)
	mdClass1 := builder.MachineDeploymentClass("worker1").
		Build()
	mdTopologyBase := builder.MachineDeploymentTopology("md1").
		WithClass("worker1").
		WithReplicas(3)
	mpClass1 := builder.MachinePoolClass("worker1").
		Build()
	mpTopologyBase := builder.MachinePoolTopology("mp1").
		WithClass("worker1").
		WithReplicas(3)
	clusterBuilder := builder.Cluster(metav1.NamespaceDefault, clusterName1).
		WithTopology(topologyBase.DeepCopy().Build())

	tests := []struct {
		name           string
		clusterClass   *clusterv1.ClusterClass
		initialCluster *clusterv1.Cluster
		wantCluster    *clusterv1.Cluster
	}{
		{
			name: "Default Cluster variables with values from ClusterClass",
			clusterClass: classBuilder.DeepCopy().
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "location",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
								},
							},
						},
					},
				}).
				WithConditions(*conditions.TrueCondition(clusterv1.ClusterClassVariablesReconciledCondition)).
				Build(),
			initialCluster: clusterBuilder.DeepCopy().
				Build(),
			wantCluster: clusterBuilder.DeepCopy().
				WithTopology(topologyBase.DeepCopy().WithVariables(
					clusterv1.ClusterVariable{Name: "location", Value: apiextensionsv1.JSON{Raw: []byte(`"us-east"`)}}).
					Build()).
				Build(),
		},
		{
			name: "Do not default variable if a value is defined in the Cluster",
			clusterClass: classBuilder.DeepCopy().
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "location",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
								},
							},
						},
					},
				}).
				WithConditions(*conditions.TrueCondition(clusterv1.ClusterClassVariablesReconciledCondition)).
				Build(),
			initialCluster: clusterBuilder.DeepCopy().WithTopology(topologyBase.DeepCopy().WithVariables(
				clusterv1.ClusterVariable{Name: "location", Value: apiextensionsv1.JSON{Raw: []byte(`"us-west"`)}}).
				Build()).
				Build(),
			wantCluster: clusterBuilder.DeepCopy().WithTopology(topologyBase.DeepCopy().WithVariables(
				clusterv1.ClusterVariable{Name: "location", Value: apiextensionsv1.JSON{Raw: []byte(`"us-west"`)}}).
				Build()).
				Build(),
		},
		{
			name: "Default nested values of Cluster variables with values from ClusterClass",
			clusterClass: classBuilder.DeepCopy().
				WithWorkerMachineDeploymentClasses(*mdClass1).
				WithWorkerMachinePoolClasses(*mpClass1).
				WithStatusVariables([]clusterv1.ClusterClassStatusVariable{
					{
						Name: "location",
						Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
							{
								Required: true,
								From:     clusterv1.VariableDefinitionFromInline,
								Schema: clusterv1.VariableSchema{
									OpenAPIV3Schema: clusterv1.JSONSchemaProps{
										Type:    "string",
										Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
									},
								},
							},
						},
					},
					{
						Name: "httpProxy",
						Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
							{
								Required: true,
								From:     clusterv1.VariableDefinitionFromInline,
								Schema: clusterv1.VariableSchema{
									OpenAPIV3Schema: clusterv1.JSONSchemaProps{
										Type: "object",
										Properties: map[string]clusterv1.JSONSchemaProps{
											"enabled": {
												Type: "boolean",
											},
											"url": {
												Type:    "string",
												Default: &apiextensionsv1.JSON{Raw: []byte(`"http://localhost:3128"`)},
											},
										},
									},
								},
							},
						},
					},
				}...).
				WithConditions(*conditions.TrueCondition(clusterv1.ClusterClassVariablesReconciledCondition)).
				Build(),
			initialCluster: clusterBuilder.DeepCopy().
				WithTopology(topologyBase.DeepCopy().
					WithVariables(
						clusterv1.ClusterVariable{Name: "location", Value: apiextensionsv1.JSON{Raw: []byte(`"us-west"`)}},
						clusterv1.ClusterVariable{Name: "httpProxy", Value: apiextensionsv1.JSON{Raw: []byte(`{"enabled":true}`)}}).
					WithControlPlaneVariables(
						clusterv1.ClusterVariable{Name: "location", Value: apiextensionsv1.JSON{Raw: []byte(`"us-west"`)}},
						clusterv1.ClusterVariable{Name: "httpProxy", Value: apiextensionsv1.JSON{Raw: []byte(`{"enabled":true}`)}}).
					WithMachineDeployment(mdTopologyBase.DeepCopy().
						WithVariables(clusterv1.ClusterVariable{
							Name:  "httpProxy",
							Value: apiextensionsv1.JSON{Raw: []byte(`{"enabled":true}`)},
						}).Build()).
					WithMachinePool(mpTopologyBase.DeepCopy().
						WithVariables(clusterv1.ClusterVariable{
							Name:  "httpProxy",
							Value: apiextensionsv1.JSON{Raw: []byte(`{"enabled":true}`)},
						}).Build()).
					Build()).
				Build(),
			wantCluster: clusterBuilder.DeepCopy().WithTopology(
				topologyBase.DeepCopy().
					WithVariables(
						clusterv1.ClusterVariable{Name: "location", Value: apiextensionsv1.JSON{Raw: []byte(`"us-west"`)}},
						clusterv1.ClusterVariable{Name: "httpProxy", Value: apiextensionsv1.JSON{Raw: []byte(`{"enabled":true,"url":"http://localhost:3128"}`)}}).
					WithControlPlaneVariables(
						clusterv1.ClusterVariable{Name: "location", Value: apiextensionsv1.JSON{Raw: []byte(`"us-west"`)}},
						clusterv1.ClusterVariable{Name: "httpProxy", Value: apiextensionsv1.JSON{Raw: []byte(`{"enabled":true,"url":"http://localhost:3128"}`)}}).
					WithMachineDeployment(
						mdTopologyBase.DeepCopy().WithVariables(
							clusterv1.ClusterVariable{
								Name: "httpProxy",
								Value: apiextensionsv1.JSON{
									// url has been added by defaulting.
									Raw: []byte(`{"enabled":true,"url":"http://localhost:3128"}`),
								},
							}).
							Build()).
					WithMachinePool(
						mpTopologyBase.DeepCopy().WithVariables(
							clusterv1.ClusterVariable{
								Name: "httpProxy",
								Value: apiextensionsv1.JSON{
									// url has been added by defaulting.
									Raw: []byte(`{"enabled":true,"url":"http://localhost:3128"}`),
								},
							}).
							Build()).
					Build()).
				Build(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(*testing.T) {
			initObjects := []client.Object{tt.initialCluster, tt.clusterClass}
			fakeClient := fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(initObjects...).Build()
			r := &Reconciler{
				Client:    fakeClient,
				APIReader: fakeClient,
			}
			// Ignore the error here as we expect the ClusterClass to fail in reconciliation as its references do not exist.
			_, _ = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKey{Name: tt.initialCluster.Name, Namespace: tt.initialCluster.Namespace}})
			got := &clusterv1.Cluster{}
			g.Expect(fakeClient.Get(ctx, client.ObjectKey{Name: tt.initialCluster.Name, Namespace: tt.initialCluster.Namespace}, got)).To(Succeed())
			// Compare the spec of the two clusters to ensure that variables are defaulted correctly.
			g.Expect(got.Spec).To(BeComparableTo(tt.wantCluster.Spec))
		})
	}
}

func TestReconciler_ValidateCluster(t *testing.T) {
	g := NewWithT(t)
	mdTopologyBase := builder.MachineDeploymentTopology("md1").
		WithClass("worker1").
		WithReplicas(3)
	mpTopologyBase := builder.MachinePoolTopology("mp1").
		WithClass("worker1").
		WithReplicas(3)
	classBuilder := builder.ClusterClass(metav1.NamespaceDefault, clusterClassName1)
	topologyBase := builder.ClusterTopology().
		WithClass(clusterClassName1).
		WithVersion("1.22.2").
		WithControlPlaneReplicas(3)
	clusterBuilder := builder.Cluster(metav1.NamespaceDefault, clusterName1).
		WithTopology(
			topologyBase.Build())
	tests := []struct {
		name                     string
		clusterClass             *clusterv1.ClusterClass
		cluster                  *clusterv1.Cluster
		wantValidationErr        bool
		wantValidationErrMessage string
	}{
		{
			name: "Valid cluster should not throw validation error",
			clusterClass: classBuilder.DeepCopy().
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "httpProxy",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: false, // variable is not required.
							From:     clusterv1.VariableDefinitionFromInline,
						},
					},
				}).
				WithConditions(*conditions.TrueCondition(clusterv1.ClusterClassVariablesReconciledCondition)).
				Build(),
			cluster: clusterBuilder.DeepCopy().
				Build(),
			wantValidationErr: false,
		},
		{
			name: "Cluster invalid as it does not define a required variable",
			clusterClass: classBuilder.DeepCopy().
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "httpProxy",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
						},
					},
				}).
				WithConditions(*conditions.TrueCondition(clusterv1.ClusterClassVariablesReconciledCondition)).
				Build(),
			cluster: clusterBuilder.
				Build(),
			wantValidationErr: true,
		},
		{
			name: "Cluster cannot reconcile as the ClusterClass has not VariablesReconciled successfully",
			clusterClass: classBuilder.DeepCopy().
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "httpProxy",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
						},
					},
				}).
				WithConditions(*conditions.FalseCondition(clusterv1.ClusterClassVariablesReconciledCondition, clusterv1.VariableDiscoveryFailedReason, clusterv1.ConditionSeverityError, "error message")).
				Build(),
			cluster: clusterBuilder.
				Build(),
			wantValidationErr:        true,
			wantValidationErrMessage: "ClusterClass is not successfully reconciled: status of VariablesReconciled condition on ClusterClass must be \"True\"",
		},
		{
			name: "Cluster invalid as it defines an MDTopology without a corresponding MDClass",
			clusterClass: classBuilder.DeepCopy().
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "httpProxy",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
						},
					},
				}).
				WithConditions(*conditions.TrueCondition(clusterv1.ClusterClassVariablesReconciledCondition)).
				Build(),
			cluster: clusterBuilder.WithTopology(
				builder.ClusterTopology().DeepCopy().
					WithClass(clusterClassName1).
					WithVersion("1.22.2").
					WithControlPlaneReplicas(3).
					WithMachineDeployment(mdTopologyBase.Build()).
					WithMachinePool(mpTopologyBase.Build()).Build(),
			).
				Build(),
			wantValidationErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(*testing.T) {
			initObjects := []client.Object{tt.cluster, tt.clusterClass}
			fakeClient := fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(initObjects...).Build()
			r := &Reconciler{
				Client:    fakeClient,
				APIReader: fakeClient,
			}
			_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKey{Name: tt.cluster.Name, Namespace: tt.cluster.Namespace}})
			// Reconcile will always return an error here as the topology is incomplete. This test checks specifically for
			// validation errors.
			validationErrMessage := fmt.Sprintf("Cluster.cluster.x-k8s.io %q is invalid:", tt.cluster.Name)
			if tt.wantValidationErrMessage != "" {
				validationErrMessage = tt.wantValidationErrMessage
			}
			if tt.wantValidationErr {
				g.Expect(err.Error()).To(ContainSubstring(validationErrMessage))
				return
			}
			g.Expect(err.Error()).ToNot(ContainSubstring(validationErrMessage))
		})
	}
}

func TestClusterClassToCluster(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "cluster-reconcile-namespace")
	g.Expect(err).ToNot(HaveOccurred())

	// Create the objects needed for the integration test:
	cleanup, err := setupTestEnvForIntegrationTests(ns)
	g.Expect(err).ToNot(HaveOccurred())

	// Defer a cleanup function that deletes each of the objects created during setupTestEnvForIntegrationTests.
	defer func() {
		g.Expect(cleanup()).To(Succeed())
	}()

	tests := []struct {
		name         string
		clusterClass *clusterv1.ClusterClass
		expected     []reconcile.Request
	}{
		{
			name:         "ClusterClass change should request reconcile for the referenced class",
			clusterClass: builder.ClusterClass(ns.Name, clusterClassName1).Build(),
			expected: []reconcile.Request{
				{NamespacedName: client.ObjectKeyFromObject(builder.Cluster(ns.Name, clusterName1).Build())},
				{NamespacedName: client.ObjectKeyFromObject(builder.Cluster(ns.Name, clusterName2).Build())},
			},
		},
		{
			name:         "ClusterClass with no matching name and namespace should not trigger reconcile",
			clusterClass: builder.ClusterClass("other", clusterClassName2).Build(),
			expected:     []reconcile.Request{},
		},
		{
			name:         "Different ClusterClass with matching name and namespace should trigger reconcile",
			clusterClass: builder.ClusterClass("other", clusterClassName1).Build(),
			expected: []reconcile.Request{
				{NamespacedName: client.ObjectKeyFromObject(builder.Cluster(ns.Name, clusterName3).Build())},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(*testing.T) {
			r := &Reconciler{Client: env.GetClient()}

			requests := r.clusterClassToCluster(ctx, tt.clusterClass)
			g.Expect(requests).To(ConsistOf(tt.expected))
		})
	}
}
