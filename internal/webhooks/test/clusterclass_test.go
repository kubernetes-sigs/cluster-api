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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

const (
	workerClassName1 = "linux-worker"
	workerClassName2 = "windows-worker"

	infrastructureClusterTemplateName1 = "infrastructureclustertemplate1"
	infrastructureClusterTemplateName2 = "infracluster2"

	controlPlaneTemplateName1 = "controlplanetemplate1"

	infrastructureMachineTemplateName1 = "infrastructuremachinetemplate1"
	infrastructureMachineTemplateName2 = "infrastructuremachinetemplate2"

	clusterClassName1 = "clusterclass1"
	clusterClassName2 = "clusterclass2"
	clusterClassName3 = "clusterclass3"
	clusterClassName4 = "clusterclass4"
	clusterClassName5 = "clusterclass5"

	clusterName1 = "cluster1"
	clusterName2 = "cluster2"
	clusterName3 = "cluster3"
	clusterName4 = "cluster4"

	bootstrapTemplateName1 = "bootstraptemplate1"
)

// TestClusterClassWebhook_Succeed_Create tests the correct creation behaviour for a valid ClusterClass.
func TestClusterClassWebhook_Succeed_Create(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "test-topology-clusterclass-webhook")
	g.Expect(err).ToNot(HaveOccurred())
	// Create the objects needed for the integration test:
	// - a ClusterClass with all the related templates
	clusterClass1 := builder.ClusterClass(ns.Name, clusterClassName1).
		WithInfrastructureClusterTemplate(
			builder.InfrastructureClusterTemplate(ns.Name, infrastructureClusterTemplateName1).Build()).
		WithControlPlaneTemplate(
			builder.ControlPlaneTemplate(ns.Name, controlPlaneTemplateName1).
				Build()).
		WithWorkerMachineDeploymentClasses(
			*builder.MachineDeploymentClass("md1").
				WithInfrastructureTemplate(
					builder.InfrastructureMachineTemplate(ns.Name, "OLD_INFRA").Build()).
				WithBootstrapTemplate(
					builder.BootstrapTemplate(ns.Name, bootstrapTemplateName1).Build()).
				Build(),
			*builder.MachineDeploymentClass("md2").
				WithInfrastructureTemplate(
					builder.InfrastructureMachineTemplate(ns.Name, infrastructureMachineTemplateName1).Build()).
				WithBootstrapTemplate(
					builder.BootstrapTemplate(ns.Name, bootstrapTemplateName1).Build()).
				Build()).
		Build()

	// Clean up the resources once the test is finished.
	t.Cleanup(func() {
		g.Expect(env.Cleanup(ctx, clusterClass1)).To(Succeed())
		g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
	})

	// Create the ClusterClass in the API server. Expect no error.
	g.Expect(env.Create(ctx, clusterClass1)).To(Succeed())
}

// TestClusterClassWebhook_Fail_Create tests the correct creation behaviour for an invalid ClusterClass with missing references.
// In this case creation of the ClusterClass should be blocked by the webhook.
func TestClusterClassWebhook_Fail_Create(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "test-topology-clusterclass-webhook")
	g.Expect(err).ToNot(HaveOccurred())

	cleanup, err := createTemplates(ns)
	g.Expect(err).ToNot(HaveOccurred())

	// Create the objects needed for the integration test:
	// - a ClusterClass with all the related templates
	clusterClass1 := builder.ClusterClass(ns.Name, clusterClassName1).
		WithInfrastructureClusterTemplate(
			builder.TestInfrastructureClusterTemplate(ns.Name, infrastructureClusterTemplateName1).Build()).
		// ControlPlaneTemplate not defined in this ClusterClass making it invalid.
		WithWorkerMachineDeploymentClasses(
			*builder.MachineDeploymentClass("md1").
				WithInfrastructureTemplate(
					builder.TestInfrastructureMachineTemplate(ns.Name, infrastructureMachineTemplateName1).Build()).
				WithBootstrapTemplate(
					builder.TestBootstrapTemplate(ns.Name, bootstrapTemplateName1).Build()).
				Build(),
			*builder.MachineDeploymentClass("md2").
				WithInfrastructureTemplate(
					builder.TestInfrastructureMachineTemplate(ns.Name, infrastructureMachineTemplateName1).Build()).
				WithBootstrapTemplate(
					builder.TestBootstrapTemplate(ns.Name, bootstrapTemplateName1).Build()).
				Build()).
		Build()

	// Clean up the resources once the test is finished.
	t.Cleanup(func() {
		g.Expect(cleanup()).To(Succeed())
		g.Expect(env.Cleanup(ctx, clusterClass1)).To(Succeed())
		g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
	})

	// Create the ClusterClass in the API server. Expect this to fail because of missing templates.
	g.Expect(env.Create(ctx, clusterClass1)).NotTo(Succeed())
}

// TestClusterWebhook_Succeed_Update tests the correct update behaviour for a ClusterClass with references in existing Clusters.
// In this case deletion of the ClusterClass should succeed by the webhook as the update is compatible with the existing Clusters using the ClusterClass.
func TestClusterWebhook_Succeed_Update(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "test-topology-clusterclass-webhook")
	g.Expect(err).ToNot(HaveOccurred())

	cleanup, err := createTemplates(ns)
	g.Expect(err).ToNot(HaveOccurred())

	// Create the objects needed for the integration test:
	// - a ClusterClass with all the related templates
	// - a Cluster using the above ClusterClass
	clusterClass := builder.ClusterClass(ns.Name, clusterClassName1).
		WithInfrastructureClusterTemplate(
			builder.TestInfrastructureClusterTemplate(ns.Name, infrastructureClusterTemplateName1).Build()).
		WithControlPlaneTemplate(
			builder.TestControlPlaneTemplate(ns.Name, controlPlaneTemplateName1).
				Build()).
		WithWorkerMachineDeploymentClasses(
			*builder.MachineDeploymentClass("NOT_USED").
				WithInfrastructureTemplate(
					builder.TestInfrastructureMachineTemplate(ns.Name, infrastructureMachineTemplateName1).Build()).
				WithBootstrapTemplate(
					builder.TestBootstrapTemplate(ns.Name, bootstrapTemplateName1).Build()).
				Build(),
			*builder.MachineDeploymentClass(workerClassName1).
				WithInfrastructureTemplate(
					builder.TestInfrastructureMachineTemplate(ns.Name, infrastructureMachineTemplateName1).Build()).
				WithBootstrapTemplate(
					builder.TestBootstrapTemplate(ns.Name, bootstrapTemplateName1).Build()).
				Build()).
		Build()

	cluster := builder.Cluster(ns.Name, clusterName1).
		WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
		WithTopology(
			builder.ClusterTopology().
				WithVersion("v1.20.1").
				WithClass(clusterClassName1).
				WithMachineDeployment(
					builder.MachineDeploymentTopology("md1").
						WithClass(workerClassName1).
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
		g.Expect(cleanup()).To(Succeed())
		g.Expect(env.CleanupAndWait(ctx, cluster)).To(Succeed())
		g.Expect(env.Cleanup(ctx, clusterClass)).To(Succeed())
		g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
	})

	// Retrieve the ClusterClass from the API server.
	actualClusterClass := &clusterv1.ClusterClass{}
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: clusterClassName1}, actualClusterClass)).To(Succeed())

	// Remove MachineDeploymentClass , which is not used by cluster1 from the clusterClass
	actualClusterClass.Spec.Workers.MachineDeployments = []clusterv1.MachineDeploymentClass{
		actualClusterClass.Spec.Workers.MachineDeployments[1],
	}
	// Change the template used in the ClusterClass to a compatible alternative (Only name is changed).
	actualClusterClass.Spec.Infrastructure.Ref.Name = infrastructureClusterTemplateName2

	// Attempt to update the ClusterClass with the above changes.
	// Expect no error here as the updates are compatible with the current Clusters using the ClusterClass.
	g.Expect(env.Update(ctx, actualClusterClass)).To(Succeed())
}

// TestClusterWebhook_Fail_Update tests the correct update behaviour for a ClusterClass with references in existing Clusters.
// In this case deletion of the ClusterClass should be blocked by the webhook as the update is incompatible with the existing Clusters using the ClusterClass.
func TestClusterWebhook_Fail_Update(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "test-topology-clusterclass-webhook")
	g.Expect(err).ToNot(HaveOccurred())

	cleanup, err := createTemplates(ns)
	g.Expect(err).ToNot(HaveOccurred())

	// Create the objects needed for the integration test:
	// - a ClusterClass with all the related templates
	// - a Cluster using the above ClusterClass
	clusterClass := builder.ClusterClass(ns.Name, clusterClassName1).
		WithInfrastructureClusterTemplate(
			builder.TestInfrastructureClusterTemplate(ns.Name, infrastructureClusterTemplateName1).Build()).
		WithControlPlaneTemplate(
			builder.TestControlPlaneTemplate(ns.Name, controlPlaneTemplateName1).
				Build()).
		WithWorkerMachineDeploymentClasses(
			*builder.MachineDeploymentClass("NOT_USED").
				WithInfrastructureTemplate(
					builder.TestInfrastructureMachineTemplate(ns.Name, infrastructureMachineTemplateName1).Build()).
				WithBootstrapTemplate(
					builder.TestBootstrapTemplate(ns.Name, bootstrapTemplateName1).Build()).
				Build(),
			*builder.MachineDeploymentClass("IN_USE").
				WithInfrastructureTemplate(
					builder.TestInfrastructureMachineTemplate(ns.Name, infrastructureMachineTemplateName1).Build()).
				WithBootstrapTemplate(
					builder.TestBootstrapTemplate(ns.Name, bootstrapTemplateName1).Build()).
				Build()).
		Build()

	cluster := builder.Cluster(ns.Name, clusterName1).
		WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
		WithTopology(
			builder.ClusterTopology().
				WithVersion("v1.20.1").
				WithClass(clusterClassName1).
				WithMachineDeployment(
					builder.MachineDeploymentTopology("md1").
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
		g.Expect(cleanup()).To(Succeed())
		g.Expect(env.CleanupAndWait(ctx, cluster)).To(Succeed())
		g.Expect(env.Cleanup(ctx, clusterClass)).To(Succeed())
		g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
	})

	// Retrieve the clusterClass from the API server.
	actualClusterClass := &clusterv1.ClusterClass{}
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: clusterClassName1}, actualClusterClass)).To(Succeed())

	// Remove MachineDeploymentClass "IN_USE", which is used by cluster1 from the clusterClass
	actualClusterClass.Spec.Workers.MachineDeployments = []clusterv1.MachineDeploymentClass{
		actualClusterClass.Spec.Workers.MachineDeployments[0],
	}

	// Attempt to update the ClusterClass with the above changes.
	// Expect an error here as the updates are incompatible with the Current Clusters using the ClusterClass.
	g.Expect(env.Update(ctx, actualClusterClass)).NotTo(Succeed())
}

// TestClusterClassWebhook_Fail_Delete tests the correct deletion behaviour for a ClusterClass with references in existing Clusters.
// In this case deletion of the ClusterClass should be blocked by the webhook.
func TestClusterClassWebhook_Delete(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "test-topology-clusterclass-webhook")
	g.Expect(err).ToNot(HaveOccurred())

	cleanup, err := createTemplates(ns)
	g.Expect(err).ToNot(HaveOccurred())

	// Create the objects needed for the integration test:
	// - a two ClusterClasses with all the related templates
	// - a Cluster using one of the ClusterClasses
	clusterClass1 := builder.ClusterClass(ns.Name, clusterClassName1).
		WithInfrastructureClusterTemplate(
			builder.TestInfrastructureClusterTemplate(ns.Name, infrastructureClusterTemplateName1).Build()).
		WithControlPlaneTemplate(
			builder.TestControlPlaneTemplate(ns.Name, controlPlaneTemplateName1).
				Build()).
		WithWorkerMachineDeploymentClasses(
			*builder.MachineDeploymentClass(workerClassName1).
				WithInfrastructureTemplate(
					builder.TestInfrastructureMachineTemplate(ns.Name, infrastructureMachineTemplateName1).Build()).
				WithBootstrapTemplate(
					builder.TestBootstrapTemplate(ns.Name, bootstrapTemplateName1).Build()).
				Build(),
			*builder.MachineDeploymentClass(workerClassName2).
				WithInfrastructureTemplate(
					builder.TestInfrastructureMachineTemplate(ns.Name, infrastructureMachineTemplateName1).Build()).
				WithBootstrapTemplate(
					builder.TestBootstrapTemplate(ns.Name, bootstrapTemplateName1).Build()).
				Build()).
		Build()

	clusterClass2 := builder.ClusterClass(ns.Name, clusterClassName2).
		WithInfrastructureClusterTemplate(
			builder.TestInfrastructureClusterTemplate(ns.Name, infrastructureClusterTemplateName1).Build()).
		WithControlPlaneTemplate(
			builder.TestControlPlaneTemplate(ns.Name, controlPlaneTemplateName1).
				Build()).
		WithWorkerMachineDeploymentClasses(
			*builder.MachineDeploymentClass(workerClassName1).
				WithInfrastructureTemplate(
					builder.TestInfrastructureMachineTemplate(ns.Name, infrastructureMachineTemplateName1).Build()).
				WithBootstrapTemplate(
					builder.TestBootstrapTemplate(ns.Name, bootstrapTemplateName1).Build()).
				Build(),
			*builder.MachineDeploymentClass(workerClassName2).
				WithInfrastructureTemplate(
					builder.TestInfrastructureMachineTemplate(ns.Name, infrastructureMachineTemplateName1).Build()).
				WithBootstrapTemplate(
					builder.TestBootstrapTemplate(ns.Name, bootstrapTemplateName1).Build()).
				Build()).
		Build()

	cluster := builder.Cluster(ns.Name, clusterName2).
		WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
		WithTopology(
			builder.ClusterTopology().
				WithVersion("v1.20.1").
				WithClass(clusterClassName1).
				WithMachineDeployment(
					builder.MachineDeploymentTopology("md1").
						WithClass(workerClassName1).
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
		g.Expect(cleanup()).To(Succeed())
		g.Expect(env.Cleanup(ctx, cluster)).To(Succeed())
		g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
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

	cleanup, err := createTemplates(ns)
	g.Expect(err).ToNot(HaveOccurred())
	// Create the objects needed for the integration test:
	// - Five ClusterClasses with all the related templates
	// - Five Clusters using all ClusterClass except "class1"
	clusterClasses := []client.Object{
		builder.ClusterClass(ns.Name, clusterClassName1).
			WithInfrastructureClusterTemplate(
				builder.TestInfrastructureClusterTemplate(ns.Name, infrastructureClusterTemplateName1).Build()).
			WithControlPlaneTemplate(
				builder.ControlPlaneTemplate(ns.Name, controlPlaneTemplateName1).
					Build()).
			Build(),
		builder.ClusterClass(ns.Name, clusterClassName2).
			WithInfrastructureClusterTemplate(
				builder.TestInfrastructureClusterTemplate(ns.Name, infrastructureClusterTemplateName1).Build()).
			WithControlPlaneTemplate(
				builder.TestControlPlaneTemplate(ns.Name, controlPlaneTemplateName1).
					Build()).
			Build(),
		builder.ClusterClass(ns.Name, clusterClassName3).
			WithInfrastructureClusterTemplate(
				builder.TestInfrastructureClusterTemplate(ns.Name, infrastructureClusterTemplateName1).Build()).
			WithControlPlaneTemplate(
				builder.TestControlPlaneTemplate(ns.Name, controlPlaneTemplateName1).
					Build()).
			Build(),
		builder.ClusterClass(ns.Name, clusterClassName4).
			WithInfrastructureClusterTemplate(
				builder.TestInfrastructureClusterTemplate(ns.Name, infrastructureClusterTemplateName1).Build()).
			WithControlPlaneTemplate(
				builder.TestControlPlaneTemplate(ns.Name, controlPlaneTemplateName1).
					Build()).
			Build(),
		builder.ClusterClass(ns.Name, clusterClassName5).
			WithInfrastructureClusterTemplate(
				builder.TestInfrastructureClusterTemplate(ns.Name, infrastructureClusterTemplateName1).Build()).
			WithControlPlaneTemplate(
				builder.TestControlPlaneTemplate(ns.Name, controlPlaneTemplateName1).
					Build()).
			Build(),
	}
	clusters := []client.Object{
		// class1 is not referenced in any of the below Clusters
		builder.Cluster(ns.Name, clusterName1).
			WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
			WithTopology(
				builder.ClusterTopology().
					WithVersion("v1.20.1").
					WithClass(clusterClassName5).
					Build()).
			Build(),
		builder.Cluster(ns.Name, clusterName2).
			WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
			WithTopology(
				builder.ClusterTopology().
					WithVersion("v1.20.1").
					WithClass(clusterClassName2).
					Build()).
			Build(),
		builder.Cluster(ns.Name, clusterName3).
			WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
			WithTopology(
				builder.ClusterTopology().
					WithVersion("v1.20.1").
					WithClass(clusterClassName3).
					Build()).
			Build(),
		builder.Cluster(ns.Name, clusterName4).
			WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
			WithTopology(
				builder.ClusterTopology().
					WithVersion("v1.20.1").
					WithClass(clusterClassName4).
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
		g.Expect(cleanup()).To(Succeed())
		// Delete each of the clusters.
		for _, c := range clusters {
			g.Expect(env.CleanupAndWait(ctx, c)).To(Succeed())
		}

		// Delete the ClusterClasses in the API server.
		for _, class := range clusterClasses {
			// The classForDeletion should not exist at this point.
			if class.GetName() != classForDeletion {
				if err := env.Delete(ctx, class); err != nil {
					if apierrors.IsNotFound(err) {
						continue
					}
					g.Expect(err).ToNot(HaveOccurred())
				}
			}
		}
	}()

	// This is the unused clusterClass to be deleted.
	classForDeletion = clusterClassName1

	class := &clusterv1.ClusterClass{}
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: classForDeletion}, class)).To(Succeed())

	// Attempt to delete ClusterClass "class1" which is not in use.
	// Expect no error here as the webhook should allow the deletion of an unused ClusterClass.
	g.Expect(env.Delete(ctx, class)).To(Succeed())

	// This is a clusterClass in use to be deleted.
	classForDeletion = clusterClassName3

	class = &clusterv1.ClusterClass{}
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: classForDeletion}, class)).To(Succeed())

	// Attempt to delete ClusterClass "class3" which is in use.
	// Expect an error here as the webhook should not allow the deletion of an existing ClusterClass.
	g.Expect(env.Delete(ctx, class)).To(Not(Succeed()))
}

// createTemplates builds and then creates all required ClusterClass templates in the envtest API server.
func createTemplates(ns *corev1.Namespace) (func() error, error) {
	// Templates for MachineInfrastructure, ClusterInfrastructure, ControlPlane and Bootstrap.
	infrastructureMachineTemplate1 := builder.TestInfrastructureMachineTemplate(ns.Name, infrastructureMachineTemplateName1).Build()
	infrastructureMachineTemplate2 := builder.TestInfrastructureMachineTemplate(ns.Name, infrastructureMachineTemplateName2).Build()
	infrastructureClusterTemplate1 := builder.TestInfrastructureClusterTemplate(ns.Name, infrastructureClusterTemplateName1).Build()
	infrastructureClusterTemplate2 := builder.TestInfrastructureClusterTemplate(ns.Name, infrastructureClusterTemplateName2).Build()
	controlPlaneTemplate := builder.TestControlPlaneTemplate(ns.Name, controlPlaneTemplateName1).Build()
	bootstrapTemplate := builder.TestBootstrapTemplate(ns.Name, bootstrapTemplateName1).Build()

	// Create a set of templates from the objects above to add to the API server when the test environment starts.
	initObjs := []client.Object{
		infrastructureClusterTemplate1,
		infrastructureClusterTemplate2,
		infrastructureMachineTemplate1,
		infrastructureMachineTemplate2,
		bootstrapTemplate,
		controlPlaneTemplate,
	}
	cleanup := func() error {
		for _, o := range initObjs {
			if err := env.CleanupAndWait(ctx, o); err != nil {
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
	return cleanup, nil
}
