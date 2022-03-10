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

package test

import (
	"testing"

	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/test/builder"
)

// TestClusterClassWebhook_Succeed_Create tests the correct creation behaviour for a valid ClusterClass.
func TestClusterClassWebhook_Succeed_Create(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "test-topology-clusterclass-webhook")
	g.Expect(err).ToNot(HaveOccurred())

	// Create the objects needed for the integration test:
	// - a ClusterClass with all the related templates
	clusterClass1 := builder.ClusterClass(ns.Name, "class1").
		WithInfrastructureClusterTemplate(
			builder.InfrastructureClusterTemplate(ns.Name, "inf").Build()).
		WithControlPlaneTemplate(
			builder.ControlPlaneTemplate(ns.Name, "cp1").
				Build()).
		WithWorkerMachineDeploymentClasses(
			*builder.MachineDeploymentClass("md1").
				WithInfrastructureTemplate(
					builder.InfrastructureMachineTemplate(ns.Name, "OLD_INFRA").Build()).
				WithBootstrapTemplate(
					builder.BootstrapTemplate(ns.Name, "bootstrap1").Build()).
				Build(),
			*builder.MachineDeploymentClass("md2").
				WithInfrastructureTemplate(
					builder.InfrastructureMachineTemplate(ns.Name, "infra1").Build()).
				WithBootstrapTemplate(
					builder.BootstrapTemplate(ns.Name, "bootstrap1").Build()).
				Build()).
		Build()

	// Clean up the resources once the test is finished.
	t.Cleanup(func() {
		g.Expect(env.Cleanup(ctx, clusterClass1))
		g.Expect(env.Cleanup(ctx, ns))
	})

	// Create the ClusterClass in the API server. Expect no error.
	g.Expect(env.Create(ctx, clusterClass1)).To(Succeed())
}

// TestClusterClassWebhook_Fail_Create tests the correct creation behaviour for an invalid ClusterClass with missing references.
// In this case creation of the ClusterClass should be blocked by the webhook.
func TestClusterClassWebhook_Fail_Create(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "test-topology-clusterclass-webhook")
	g.Expect(err).ToNot(HaveOccurred())

	// Create the objects needed for the integration test:
	// - a ClusterClass with all the related templates
	clusterClass1 := builder.ClusterClass(ns.Name, "class1").
		WithInfrastructureClusterTemplate(
			builder.InfrastructureClusterTemplate(ns.Name, "inf").Build()).
		// ControlPlaneTemplate not defined in this ClusterClass making it invalid.
		WithWorkerMachineDeploymentClasses(
			*builder.MachineDeploymentClass("md1").
				WithInfrastructureTemplate(
					builder.InfrastructureMachineTemplate(ns.Name, "OLD_INFRA").Build()).
				WithBootstrapTemplate(
					builder.BootstrapTemplate(ns.Name, "bootstrap1").Build()).
				Build(),
			*builder.MachineDeploymentClass("md2").
				WithInfrastructureTemplate(
					builder.InfrastructureMachineTemplate(ns.Name, "infra1").Build()).
				WithBootstrapTemplate(
					builder.BootstrapTemplate(ns.Name, "bootstrap1").Build()).
				Build()).
		Build()

	// Clean up the resources once the test is finished.
	t.Cleanup(func() {
		g.Expect(env.Cleanup(ctx, ns))
	})

	// Create the ClusterClass in the API server. Expect this to fail because of missing templates.
	g.Expect(env.Create(ctx, clusterClass1)).NotTo(Succeed())
}

// TestClusterWebhook_Succeed_Update tests the correct update behaviour for a ClusterClass with references in existing Clusters.
// In this case deletion of the ClusterClass should succeed by the webhook as the update is compatible with the existing Clusters using the ClusterClass.
func TestClusterWebhook_Succeed_Update(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "test-topology-clusterclass-webhook")
	g.Expect(err).ToNot(HaveOccurred())

	// Create the objects needed for the integration test:
	// - a ClusterClass with all the related templates
	// - a Cluster using the above ClusterClass
	clusterClass := builder.ClusterClass(ns.Name, "class1").
		WithInfrastructureClusterTemplate(
			builder.InfrastructureClusterTemplate(ns.Name, "inf").Build()).
		WithControlPlaneTemplate(
			builder.ControlPlaneTemplate(ns.Name, "cp1").
				Build()).
		WithWorkerMachineDeploymentClasses(
			*builder.MachineDeploymentClass("NOT_USED").
				WithInfrastructureTemplate(
					builder.InfrastructureMachineTemplate(ns.Name, "OLD_INFRA").Build()).
				WithBootstrapTemplate(
					builder.BootstrapTemplate(ns.Name, "bootstrap1").Build()).
				Build(),
			*builder.MachineDeploymentClass("IN_USE").
				WithInfrastructureTemplate(
					builder.InfrastructureMachineTemplate(ns.Name, "infra1").Build()).
				WithBootstrapTemplate(
					builder.BootstrapTemplate(ns.Name, "bootstrap1").Build()).
				Build()).
		Build()

	cluster := builder.Cluster(ns.Name, "cluster1").
		WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
		WithTopology(
			builder.ClusterTopology().
				WithVersion("v1.20.1").
				WithClass("class1").
				WithMachineDeployment(
					builder.MachineDeploymentTopology("workers1").
						WithClass("IN_USE").
						Build(),
				).
				Build()).
		Build()

	// Create the ClusterClass in the API server.
	g.Expect(env.CreateAndWait(ctx, clusterClass)).To(Succeed())

	// Create a Cluster using the ClusterClass.
	g.Expect(env.CreateAndWait(ctx, cluster)).To(Succeed())

	// Clean up the resources once the test is finished.
	t.Cleanup(func() {
		g.Expect(env.CleanupAndWait(ctx, cluster))
		g.Expect(env.Cleanup(ctx, clusterClass))
		g.Expect(env.Cleanup(ctx, ns))
	})

	// Retrieve the ClusterClass from the API server.
	actualCluster := &clusterv1.ClusterClass{}
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: "class1"}, actualCluster)).To(Succeed())

	// Remove MachineDeploymentClass "NOT_USED", which is not used by cluster1 from the clusterClass
	actualCluster.Spec.Workers.MachineDeployments = []clusterv1.MachineDeploymentClass{
		actualCluster.Spec.Workers.MachineDeployments[1],
	}
	// Change the template used in the ClusterClass to a compatible alternative (Only name is changed).
	actualCluster.Spec.Infrastructure.Ref.Name = "NEW_INFRA"

	// Attempt to update the ClusterClass with the above changes.
	// Expect no error here as the updates are compatible with the current Clusters using the ClusterClass.
	g.Expect(env.Update(ctx, actualCluster)).To(Succeed())
}

// TestClusterWebhook_Fail_Update tests the correct update behaviour for a ClusterClass with references in existing Clusters.
// In this case deletion of the ClusterClass should be blocked by the webhook as the update is incompatible with the existing Clusters using the ClusterClass.
func TestClusterWebhook_Fail_Update(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "test-topology-clusterclass-webhook")
	g.Expect(err).ToNot(HaveOccurred())

	// Create the objects needed for the integration test:
	// - a ClusterClass with all the related templates
	// - a Cluster using the above ClusterClass
	clusterClass := builder.ClusterClass(ns.Name, "class1").
		WithInfrastructureClusterTemplate(
			builder.InfrastructureClusterTemplate(ns.Name, "inf").Build()).
		WithControlPlaneTemplate(
			builder.ControlPlaneTemplate(ns.Name, "cp1").
				Build()).
		WithWorkerMachineDeploymentClasses(
			*builder.MachineDeploymentClass("NOT_USED").
				WithInfrastructureTemplate(
					builder.InfrastructureMachineTemplate(ns.Name, "infra1").Build()).
				WithBootstrapTemplate(
					builder.BootstrapTemplate(ns.Name, "bootstrap1").Build()).
				Build(),
			*builder.MachineDeploymentClass("IN_USE").
				WithInfrastructureTemplate(
					builder.InfrastructureMachineTemplate(ns.Name, "infra1").Build()).
				WithBootstrapTemplate(
					builder.BootstrapTemplate(ns.Name, "bootstrap1").Build()).
				Build()).
		Build()

	cluster := builder.Cluster(ns.Name, "cluster1").
		WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
		WithTopology(
			builder.ClusterTopology().
				WithVersion("v1.20.1").
				WithClass("class1").
				WithMachineDeployment(
					builder.MachineDeploymentTopology("workers1").
						WithClass("IN_USE").
						Build(),
				).
				Build()).
		Build()

	// Create the ClusterClass in the API server.
	g.Expect(env.CreateAndWait(ctx, clusterClass)).To(Succeed())

	// Create a cluster using the ClusterClass.
	g.Expect(env.CreateAndWait(ctx, cluster)).To(Succeed())

	// Clean up the resources once the test is finished.
	t.Cleanup(func() {
		g.Expect(env.CleanupAndWait(ctx, cluster))
		g.Expect(env.Cleanup(ctx, clusterClass))
		g.Expect(env.Cleanup(ctx, ns))
	})

	// Retrieve the clusterClass from the API server.
	actualCluster := &clusterv1.ClusterClass{}
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: "class1"}, actualCluster)).To(Succeed())

	// Remove MachineDeploymentClass "IN_USE", which is used by cluster1 from the clusterClass
	actualCluster.Spec.Workers.MachineDeployments = []clusterv1.MachineDeploymentClass{
		actualCluster.Spec.Workers.MachineDeployments[0],
	}

	// Attempt to update the ClusterClass with the above changes.
	// Expect an error here as the updates are incompatible with the Current Clusters using the ClusterClass.
	g.Expect(env.Update(ctx, actualCluster)).NotTo(Succeed())
}

// TestClusterClassWebhook_Fail_Delete tests the correct deletion behaviour for a ClusterClass with references in existing Clusters.
// In this case deletion of the ClusterClass should be blocked by the webhook.
func TestClusterClassWebhook_Delete(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "test-topology-clusterclass-webhook")
	g.Expect(err).ToNot(HaveOccurred())

	// Create the objects needed for the integration test:
	// - a two ClusterClasses with all the related templates
	// - a Cluster using one of the ClusterClasses
	clusterClass1 := builder.ClusterClass(ns.Name, "class1").
		WithInfrastructureClusterTemplate(
			builder.InfrastructureClusterTemplate(ns.Name, "inf").Build()).
		WithControlPlaneTemplate(
			builder.ControlPlaneTemplate(ns.Name, "cp1").
				Build()).
		WithWorkerMachineDeploymentClasses(
			*builder.MachineDeploymentClass("md1").
				WithInfrastructureTemplate(
					builder.InfrastructureMachineTemplate(ns.Name, "OLD_INFRA").Build()).
				WithBootstrapTemplate(
					builder.BootstrapTemplate(ns.Name, "bootstrap1").Build()).
				Build(),
			*builder.MachineDeploymentClass("md2").
				WithInfrastructureTemplate(
					builder.InfrastructureMachineTemplate(ns.Name, "infra1").Build()).
				WithBootstrapTemplate(
					builder.BootstrapTemplate(ns.Name, "bootstrap1").Build()).
				Build()).
		Build()

	clusterClass2 := builder.ClusterClass(ns.Name, "class2").
		WithInfrastructureClusterTemplate(
			builder.InfrastructureClusterTemplate(ns.Name, "inf").Build()).
		WithControlPlaneTemplate(
			builder.ControlPlaneTemplate(ns.Name, "cp1").
				Build()).
		WithWorkerMachineDeploymentClasses(
			*builder.MachineDeploymentClass("md1").
				WithInfrastructureTemplate(
					builder.InfrastructureMachineTemplate(ns.Name, "OLD_INFRA").Build()).
				WithBootstrapTemplate(
					builder.BootstrapTemplate(ns.Name, "bootstrap1").Build()).
				Build(),
			*builder.MachineDeploymentClass("md2").
				WithInfrastructureTemplate(
					builder.InfrastructureMachineTemplate(ns.Name, "infra1").Build()).
				WithBootstrapTemplate(
					builder.BootstrapTemplate(ns.Name, "bootstrap1").Build()).
				Build()).
		Build()

	cluster := builder.Cluster(ns.Name, "cluster2").
		WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
		WithTopology(
			builder.ClusterTopology().
				WithVersion("v1.20.1").
				WithClass("class1").
				WithMachineDeployment(
					builder.MachineDeploymentTopology("workers1").
						WithClass("md1").
						Build(),
				).
				Build()).
		Build()

	// Create the ClusterClasses in the API server.
	g.Expect(env.CreateAndWait(ctx, clusterClass1)).To(Succeed())
	g.Expect(env.CreateAndWait(ctx, clusterClass2)).To(Succeed())

	// Create a clusters.
	g.Expect(env.CreateAndWait(ctx, cluster)).To(Succeed())

	// Clean up the resources once the test is finished.
	t.Cleanup(func() {
		g.Expect(env.Cleanup(ctx, cluster))
		g.Expect(env.Cleanup(ctx, ns))
	})

	// Attempt to delete ClusterClass "class1" which is in use.
	// Expect no error here as the webhook should allow the deletion of an existing ClusterClass.
	g.Expect(env.Delete(ctx, clusterClass1)).To(Not(Succeed()))

	// Attempt to delete ClusterClass "class2" which is not in use.
	// Expect no error here as the webhook should allow the deletion of an existing ClusterClass.
	g.Expect(env.Delete(ctx, clusterClass2)).To(Succeed())
}

// TestClusterClassWebhook_Succeed_Delete tests the correct deletion behaviour for a ClusterClass with no references in existing Clusters.
// In this case deletion of the ClusterClass should succeed.
func TestClusterClassWebhook_Delete_MultipleExistingClusters(t *testing.T) {
	g := NewWithT(t)
	ns, err := env.CreateNamespace(ctx, "test-clusterclass-webhook")
	g.Expect(err).ToNot(HaveOccurred())

	// Create the objects needed for the integration test:
	// - Five ClusterClasses with all the related templates
	// - Five Clusters using all ClusterClass except "class1"
	clusterClasses := []client.Object{
		builder.ClusterClass(ns.Name, "class1").
			WithInfrastructureClusterTemplate(
				builder.InfrastructureClusterTemplate(ns.Name, "inf").Build()).
			WithControlPlaneTemplate(
				builder.ControlPlaneTemplate(ns.Name, "cp1").
					Build()).
			Build(),
		builder.ClusterClass(ns.Name, "class2").
			WithInfrastructureClusterTemplate(
				builder.InfrastructureClusterTemplate(ns.Name, "inf").Build()).
			WithControlPlaneTemplate(
				builder.ControlPlaneTemplate(ns.Name, "cp1").
					Build()).
			Build(),
		builder.ClusterClass(ns.Name, "class3").
			WithInfrastructureClusterTemplate(
				builder.InfrastructureClusterTemplate(ns.Name, "inf").Build()).
			WithControlPlaneTemplate(
				builder.ControlPlaneTemplate(ns.Name, "cp1").
					Build()).
			Build(),
		builder.ClusterClass(ns.Name, "class4").
			WithInfrastructureClusterTemplate(
				builder.InfrastructureClusterTemplate(ns.Name, "inf").Build()).
			WithControlPlaneTemplate(
				builder.ControlPlaneTemplate(ns.Name, "cp1").
					Build()).
			Build(),
		builder.ClusterClass(ns.Name, "class5").
			WithInfrastructureClusterTemplate(
				builder.InfrastructureClusterTemplate(ns.Name, "inf").Build()).
			WithControlPlaneTemplate(
				builder.ControlPlaneTemplate(ns.Name, "cp1").
					Build()).
			Build(),
	}
	clusters := []client.Object{
		// class1 is not referenced in any of the below Clusters
		builder.Cluster(ns.Name, "cluster1").
			WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
			WithTopology(
				builder.ClusterTopology().
					WithVersion("v1.20.1").
					WithClass("class5").
					Build()).
			Build(),
		builder.Cluster(ns.Name, "cluster2").
			WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
			WithTopology(
				builder.ClusterTopology().
					WithVersion("v1.20.1").
					WithClass("class2").
					Build()).
			Build(),
		builder.Cluster(ns.Name, "cluster3").
			WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
			WithTopology(
				builder.ClusterTopology().
					WithVersion("v1.20.1").
					WithClass("class3").
					Build()).
			Build(),
		builder.Cluster(ns.Name, "cluster4").
			WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
			WithTopology(
				builder.ClusterTopology().
					WithVersion("v1.20.1").
					WithClass("class4").
					Build()).
			Build(),
	}
	var classForDeletion string
	// Create the ClusterClasses in the API server.
	for _, class := range clusterClasses {
		g.Expect(env.CreateAndWait(ctx, class)).To(Succeed())
	}

	// Create each of the clusters.
	for _, c := range clusters {
		g.Expect(env.CreateAndWait(ctx, c)).To(Succeed())
	}

	// defer a function to clean up created resources.
	defer func() {
		// Delete each of the clusters.
		for _, c := range clusters {
			g.Expect(env.Delete(ctx, c)).To(Succeed())
		}

		// Delete the ClusterClasses in the API server.
		for _, class := range clusterClasses {
			// The classForDeletion should not exist at this point.
			if class.GetName() != classForDeletion {
				if err := env.Delete(ctx, class); err != nil {
					if apierrors.IsNotFound(err) {
						continue
					}
					g.Expect(err).To(BeNil())
				}
			}
		}
	}()

	// This is the unused clusterClass to be deleted.
	classForDeletion = "class1"

	class := &clusterv1.ClusterClass{}
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: classForDeletion}, class)).To(Succeed())

	// Attempt to delete ClusterClass "class1" which is not in use.
	// Expect no error here as the webhook should allow the deletion of an unused ClusterClass.
	g.Expect(env.Delete(ctx, class)).To(Succeed())

	// This is a clusterClass in use to be deleted.
	classForDeletion = "class3"

	class = &clusterv1.ClusterClass{}
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: classForDeletion}, class)).To(Succeed())

	// Attempt to delete ClusterClass "class3" which is in use.
	// Expect an error here as the webhook should not allow the deletion of an existing ClusterClass.
	g.Expect(env.Delete(ctx, class)).To(Not(Succeed()))
}
