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

package topology

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilfeature "k8s.io/component-base/featuregate/testing"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/contract"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestClusterReconciler_reconcile(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()
	g := NewWithT(t)
	timeout := 5 * time.Second

	ns, err := env.CreateNamespace(ctx, "test-topology-cluster-reconcile")
	g.Expect(err).ToNot(HaveOccurred())

	clusterName := "cluster1"
	clusterClassName := "class1"
	workerClassName1 := "linux-worker"
	workerClassName2 := "windows-worker"

	// The below objects are created in order to feed the reconcile loop all the information it needs to create a full
	// Cluster given a skeletal Cluster object and a ClusterClass. The objects include:

	// 1) Templates for Machine, Cluster, ControlPlane and Bootstrap.
	infrastructureMachineTemplate := builder.InfrastructureMachineTemplate(ns.Name, "inframachinetemplate").Build()
	infrastructureClusterTemplate := builder.InfrastructureClusterTemplate(ns.Name, "infraclustertemplate").
		// Create spec fake setting to assert that template spec is non-empty for tests.
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()
	controlPlaneTemplate := builder.ControlPlaneTemplate(ns.Name, "cp1").
		WithInfrastructureMachineTemplate(infrastructureMachineTemplate).
		Build()
	bootstrapTemplate := builder.BootstrapTemplate(ns.Name, "bootstraptemplate").Build()

	// 2) ClusterClass definitions including definitions of MachineDeploymentClasses used inside the ClusterClass.
	machineDeploymentClass1 := builder.MachineDeploymentClass(ns.Name, "md1").
		WithClass(workerClassName1).
		WithInfrastructureTemplate(infrastructureMachineTemplate).
		WithBootstrapTemplate(bootstrapTemplate).
		WithLabels(map[string]string{"foo": "bar"}).
		WithAnnotations(map[string]string{"foo": "bar"}).
		Build()
	machineDeploymentClass2 := builder.MachineDeploymentClass(ns.Name, "md2").
		WithClass(workerClassName2).
		WithInfrastructureTemplate(infrastructureMachineTemplate).
		WithBootstrapTemplate(bootstrapTemplate).
		Build()
	clusterClass := builder.ClusterClass(ns.Name, clusterClassName).
		WithInfrastructureClusterTemplate(infrastructureClusterTemplate).
		WithControlPlaneTemplate(controlPlaneTemplate).
		WithControlPlaneInfrastructureMachineTemplate(infrastructureMachineTemplate).
		WithWorkerMachineDeploymentClasses([]clusterv1.MachineDeploymentClass{*machineDeploymentClass1, *machineDeploymentClass2}).
		Build()

	// 3) A Cluster object including a Cluster Topology Object and the MachineDeploymentTopology objects used in the
	// Cluster Topology.
	machineDeploymentTopology1 := builder.MachineDeploymentTopology("mdm1").
		WithClass(workerClassName1).
		WithReplicas(3).
		Build()
	machineDeploymentTopology2 := builder.MachineDeploymentTopology("mdm2").
		WithClass(workerClassName2).
		WithReplicas(1).
		Build()
	cluster := builder.Cluster(ns.Name, clusterName).
		WithTopology(
			builder.ClusterTopology().
				WithClass(clusterClass.Name).
				WithMachineDeployment(machineDeploymentTopology1).
				WithMachineDeployment(machineDeploymentTopology2).
				WithVersion("1.22.2").
				WithControlPlaneReplicas(3).
				Build()).
		Build()

	// Create a set of initObjects from the objects above to add to the API server when the test environment starts.
	initObjs := []client.Object{
		clusterClass,
		cluster,
		infrastructureClusterTemplate,
		infrastructureMachineTemplate,
		bootstrapTemplate,
		controlPlaneTemplate,
	}

	for _, obj := range initObjs {
		g.Expect(env.Create(ctx, obj)).To(Succeed())
	}
	defer func() {
		for _, obj := range initObjs {
			g.Expect(env.Delete(ctx, obj)).To(Succeed())
		}
	}()

	g.Eventually(func(g Gomega) error {
		// Get the cluster object.
		key := client.ObjectKey{Name: clusterName, Namespace: ns.Name}
		actualCluster := &clusterv1.Cluster{}
		if err := env.Get(ctx, key, actualCluster); err != nil {
			return err
		}

		// Check if Cluster has relevant Infrastructure and ControlPlane and labels and annotations.
		g.Expect(assertClusterReconcile(actualCluster)).Should(Succeed())

		// Check if InfrastructureCluster has been created and has the correct labels and annotations.
		g.Expect(assertInfrastructureClusterReconcile(actualCluster)).Should(Succeed())

		// Check if ControlPlane has been created and has the correct version, replicas, labels and annotations.
		g.Expect(assertControlPlaneReconcile(actualCluster, clusterClass)).Should(Succeed())

		// Check if MachineDeployments are created and have the correct version, replicas, labels annotations and templates.
		g.Expect(assertMachineDeploymentsReconcile(actualCluster)).Should(Succeed())

		return nil
	}, timeout).Should(Succeed())
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
		builder.GenericInfrastructureClusterKind,
		builder.InfrastructureGroupVersion); err != nil {
		return err
	}

	// Check if ControlPlaneRef exists is of the expected Kind and APIVersion.
	if err := referenceExistsWithCorrectKindAndAPIVersion(cluster.Spec.ControlPlaneRef,
		builder.GenericControlPlaneKind,
		builder.ControlPlaneGroupVersion); err != nil {
		return err
	}
	return nil
}

// assertInfrastructureClusterReconcile checks if the infrastructureCluster object:
// 1) Is created.
// 2) Has the correct labels and annotations.
func assertInfrastructureClusterReconcile(cluster *clusterv1.Cluster) error {
	_, err := getAndAssertLabelsAndAnnotations(*cluster.Spec.InfrastructureRef, cluster.Name)
	return err
}

// assertControlPlaneReconcile checks if the ControlPlane object:
// 1) Is created.
// 2) Has the correct labels and annotations.
// 3) If it requires ControlPlane Infrastructure and if so:
//		i) That the infrastructureMachineTemplate is created correctly.
//      ii) That the infrastructureMachineTemplate has the correct labels and annotations
func assertControlPlaneReconcile(cluster *clusterv1.Cluster, clusterClass *clusterv1.ClusterClass) error {
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

	// Check for the ControlPlaneInfrastructure if it's referenced in the clusterClass.
	if clusterClass.Spec.ControlPlane.MachineInfrastructure != nil && clusterClass.Spec.ControlPlane.MachineInfrastructure.Ref != nil {
		cpInfra, err := contract.ControlPlane().MachineTemplate().InfrastructureRef().Get(cp)
		if err != nil {
			return err
		}
		if err := referenceExistsWithCorrectKindAndAPIVersion(cpInfra,
			builder.GenericInfrastructureMachineTemplateKind,
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
// 3) Have the correct finalizer applied.
// 4) Have the correct replicas and version.
// 6) Have the correct Kind/APIVersion and Labels/Annotations for BoostrapRef and InfrastructureRef templates.
func assertMachineDeploymentsReconcile(cluster *clusterv1.Cluster) error {
	// List all created machine deployments to assert the expected numbers are c*reated.
	machineDeployments := &clusterv1.MachineDeploymentList{}
	if err := env.List(ctx, machineDeployments); err != nil {
		return err
	}

	// clusterMDs will hold the MachineDeployments that have labels associating them with the cluster.
	clusterMDs := []clusterv1.MachineDeployment{}

	// Run through all machine deployments and add only those with the TopologyOwnedLabel and the correct
	// ClusterLabelName to the items for further testing.
	for _, m := range machineDeployments.Items {
		// If the machineDeployment doesn't have the ClusterTopologyOwnedLabel and the ClusterLabelName ignore.
		md := m
		if err := assertClusterTopologyOwnedLabel(&md); err != nil {
			continue
		}
		if err := assertClusterLabelName(&md, cluster.Name); err != nil {
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
			if topologyMD.Name != md.GetLabels()[clusterv1.ClusterTopologyMachineDeploymentLabelName] {
				continue
			}

			// Assert that the correct Finalizer has been added to the MachineDeployment.
			for _, f := range md.Finalizers {
				// Break as soon as we find a matching finalizer.
				if f == clusterv1.MachineDeploymentTopologyFinalizer {
					break
				}
				// False if the finalizer is not present on the MachineDeployment.
				return fmt.Errorf("finalizer %v not found on MachineDeployment", clusterv1.MachineDeploymentTopologyFinalizer)
			}

			// Check if the ClusterTopologyLabelName and ClusterTopologyOwnedLabel are set correctly.
			if err := assertClusterTopologyOwnedLabel(&md); err != nil {
				return err
			}

			if err := assertClusterLabelName(&md, cluster.Name); err != nil {
				return err
			}

			// Check replicas and version for the MachineDeployment.
			if *md.Spec.Replicas != *topologyMD.Replicas {
				return fmt.Errorf("replicas %v does not match expected %v", md.Spec.Replicas, topologyMD.Replicas)
			}
			if *md.Spec.Template.Spec.Version != cluster.Spec.Topology.Version {
				return fmt.Errorf("version %v does not match expecteed %v", md.Spec.Template.Spec.Version, cluster.Spec.Topology.Version)
			}

			// Check if the InfrastructureReference exists.
			if err := referenceExistsWithCorrectKindAndAPIVersion(&md.Spec.Template.Spec.InfrastructureRef,
				builder.GenericInfrastructureMachineTemplateKind,
				builder.InfrastructureGroupVersion); err != nil {
				return err
			}

			// Check if the InfrastructureReference has the expected labels and annotations.
			if _, err := getAndAssertLabelsAndAnnotations(md.Spec.Template.Spec.InfrastructureRef, cluster.Name); err != nil {
				return err
			}

			// Check if the Bootstrap reference has the expected Kind and APIVersion.
			if err := referenceExistsWithCorrectKindAndAPIVersion(md.Spec.Template.Spec.Bootstrap.ConfigRef,
				builder.GenericBootstrapConfigTemplateKind,
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

// getAndAssertLabelsAndAnnotations pulls the template referenced in the ObjectReference from the API server, checks for:
// 1) The ClusterTopologyOwnedLabel.
// 2) The correct ClusterLabelName.
// 3) The annotation stating where the template was cloned from.
// The function returns the unstructured object and a bool indicating if it passed all tests.
func getAndAssertLabelsAndAnnotations(template corev1.ObjectReference, clusterName string) (*unstructured.Unstructured, error) {
	key := client.ObjectKey{Name: template.Name, Namespace: template.Namespace}
	got := &unstructured.Unstructured{}
	got.SetKind(template.Kind)
	got.SetAPIVersion(template.APIVersion)

	if err := env.Get(ctx, key, got); err != nil {
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
	if err := assertClusterLabelName(got, clusterName); err != nil {
		return err
	}
	if err := assertTemplateClonedFromNameAnnotation(got); err != nil {
		return err
	}
	return nil
}

// assertClusterTopologyOwnedLabel  asserts the label exists.
func assertClusterTopologyOwnedLabel(got client.Object) error {
	_, ok := got.GetLabels()[clusterv1.ClusterTopologyOwnedLabel]
	if !ok {
		return fmt.Errorf("%v not found on %v: %v", clusterv1.ClusterTopologyOwnedLabel, got.GetObjectKind().GroupVersionKind().Kind, got.GetName())
	}
	return nil
}

// assertClusterTopologyOwnedLabel asserts the label exists and is set to the correct value.
func assertClusterLabelName(got client.Object, clusterName string) error {
	v, ok := got.GetLabels()[clusterv1.ClusterLabelName]
	if !ok {
		return fmt.Errorf("%v not found in %v: %v", clusterv1.ClusterLabelName, got.GetObjectKind().GroupVersionKind().Kind, got.GetName())
	}
	if v != clusterName {
		return fmt.Errorf("%v %v does not match expected %v", clusterv1.ClusterLabelName, v, clusterName)
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
