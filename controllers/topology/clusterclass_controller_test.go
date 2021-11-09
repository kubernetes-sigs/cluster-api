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
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/component-base/featuregate/testing"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	tlog "sigs.k8s.io/cluster-api/controllers/topology/internal/log"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestClusterClassReconciler_reconcile(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()
	g := NewWithT(t)
	timeout := 30 * time.Second

	ns, err := env.CreateNamespace(ctx, "test-topology-clusterclass-reconciler")
	g.Expect(err).NotTo(HaveOccurred())

	clusterClassName := "class1"
	workerClassName1 := "linux-worker-1"
	workerClassName2 := "linux-worker-2"

	// The below objects are created in order to feed the reconcile loop all the information it needs to create a
	// full tree of ClusterClass objects (the objects should have owner referecen to the ClusterClass).

	// Bootstrap templates for the workers.
	bootstrapTemplate := builder.BootstrapTemplate(ns.Name, "bootstraptemplate").Build()

	// InfraMachineTemplates for the workers and the control plane.
	infraMachineTemplateControlPlane := builder.InfrastructureMachineTemplate(ns.Name, "inframachinetemplate-control-plane").Build()
	infraMachineTemplateWorker := builder.InfrastructureMachineTemplate(ns.Name, "inframachinetemplate-worker").Build()

	// Control plane template.
	controlPlaneTemplate := builder.ControlPlaneTemplate(ns.Name, "controlplanetemplate").Build()

	// InfraClusterTemplate.
	infraClusterTemplate := builder.InfrastructureClusterTemplate(ns.Name, "infraclustertemplate").Build()

	// MachineDeploymentClasses that will be part of the ClusterClass.
	machineDeploymentClass1 := builder.MachineDeploymentClass(workerClassName1).
		WithBootstrapTemplate(bootstrapTemplate).
		WithInfrastructureTemplate(infraMachineTemplateWorker).
		Build()
	machineDeploymentClass2 := builder.MachineDeploymentClass(workerClassName2).
		WithBootstrapTemplate(bootstrapTemplate).
		WithInfrastructureTemplate(infraMachineTemplateWorker).
		Build()

	// ClusterClass.
	clusterClass := builder.ClusterClass(ns.Name, clusterClassName).
		WithInfrastructureClusterTemplate(infraClusterTemplate).
		WithControlPlaneTemplate(controlPlaneTemplate).
		WithControlPlaneInfrastructureMachineTemplate(infraMachineTemplateControlPlane).
		WithWorkerMachineDeploymentClasses(*machineDeploymentClass1, *machineDeploymentClass2).
		Build()

	// Create the set of initObjects from the objects above to add to the API server when the test environment starts.
	initObjs := []client.Object{
		bootstrapTemplate,
		infraMachineTemplateWorker,
		infraMachineTemplateControlPlane,
		controlPlaneTemplate,
		infraClusterTemplate,
		clusterClass,
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
		actualClusterClass := &clusterv1.ClusterClass{}
		g.Expect(env.Get(ctx, client.ObjectKey{Name: clusterClassName, Namespace: ns.Name}, actualClusterClass)).To(Succeed())

		g.Expect(assertInfrastructureClusterTemplate(ctx, actualClusterClass, ns)).Should(Succeed())

		g.Expect(assertControlPlaneTemplate(ctx, actualClusterClass, ns)).Should(Succeed())

		g.Expect(assertMachineDeploymentClasses(ctx, actualClusterClass, ns)).Should(Succeed())

		return nil
	}, timeout).Should(Succeed())
}

func assertInfrastructureClusterTemplate(ctx context.Context, actualClusterClass *clusterv1.ClusterClass, ns *corev1.Namespace) error {
	// Assert the infrastructure cluster template has the correct owner reference.
	actualInfraClusterTemplate := builder.InfrastructureClusterTemplate("", "").Build()
	actualInfraClusterTemplateKey := client.ObjectKey{
		Namespace: ns.Name,
		Name:      actualClusterClass.Spec.Infrastructure.Ref.Name,
	}
	if err := env.Get(ctx, actualInfraClusterTemplateKey, actualInfraClusterTemplate); err != nil {
		return err
	}
	if err := assertHasOwnerReference(actualInfraClusterTemplate, ownerRefereceTo(actualClusterClass)); err != nil {
		return err
	}

	// Assert the ClusterClass has the expected APIVersion and Kind of to the infrastructure cluster template
	if err := referenceExistsWithCorrectKindAndAPIVersion(actualClusterClass.Spec.Infrastructure.Ref,
		builder.GenericInfrastructureClusterTemplateKind,
		builder.InfrastructureGroupVersion); err != nil {
		return err
	}

	return nil
}

func assertControlPlaneTemplate(ctx context.Context, actualClusterClass *clusterv1.ClusterClass, ns *corev1.Namespace) error {
	// Assert the control plane template has the correct owner reference.
	actualControlPlaneTemplate := builder.ControlPlaneTemplate("", "").Build()
	actualControlPlaneTemplateKey := client.ObjectKey{
		Namespace: ns.Name,
		Name:      actualClusterClass.Spec.ControlPlane.Ref.Name,
	}
	if err := env.Get(ctx, actualControlPlaneTemplateKey, actualControlPlaneTemplate); err != nil {
		return err
	}
	if err := assertHasOwnerReference(actualControlPlaneTemplate, ownerRefereceTo(actualClusterClass)); err != nil {
		return err
	}

	// Assert the ClusterClass has the expected APIVersion and Kind to the control plane template
	if err := referenceExistsWithCorrectKindAndAPIVersion(actualClusterClass.Spec.ControlPlane.Ref,
		builder.GenericControlPlaneTemplateKind,
		builder.ControlPlaneGroupVersion); err != nil {
		return err
	}

	// If the control plane has machine infra assert that the infra machine template has the correct owner reference.
	if actualClusterClass.Spec.ControlPlane.MachineInfrastructure != nil && actualClusterClass.Spec.ControlPlane.MachineInfrastructure.Ref != nil {
		actualInfrastructureMachineTemplate := builder.InfrastructureMachineTemplate("", "").Build()
		actualInfrastructureMachineTemplateKey := client.ObjectKey{
			Namespace: ns.Name,
			Name:      actualClusterClass.Spec.ControlPlane.MachineInfrastructure.Ref.Name,
		}
		if err := env.Get(ctx, actualInfrastructureMachineTemplateKey, actualInfrastructureMachineTemplate); err != nil {
			return err
		}
		if err := assertHasOwnerReference(actualInfrastructureMachineTemplate, ownerRefereceTo(actualClusterClass)); err != nil {
			return err
		}

		// Assert the ClusterClass has the expected APIVersion and Kind to the infrastructure machine template
		if err := referenceExistsWithCorrectKindAndAPIVersion(actualClusterClass.Spec.ControlPlane.MachineInfrastructure.Ref,
			builder.GenericInfrastructureMachineTemplateKind,
			builder.InfrastructureGroupVersion); err != nil {
			return err
		}
	}

	return nil
}

func assertMachineDeploymentClasses(ctx context.Context, actualClusterClass *clusterv1.ClusterClass, ns *corev1.Namespace) error {
	for _, mdClass := range actualClusterClass.Spec.Workers.MachineDeployments {
		if err := assertMachineDeploymentClass(ctx, actualClusterClass, mdClass, ns); err != nil {
			return err
		}
	}
	return nil
}

func assertMachineDeploymentClass(ctx context.Context, actualClusterClass *clusterv1.ClusterClass, mdClass clusterv1.MachineDeploymentClass, ns *corev1.Namespace) error {
	// Assert the infrastructure machine template in the MachineDeploymentClass has an owner reference to the ClusterClass.
	actualInfrastructureMachineTemplate := builder.InfrastructureMachineTemplate("", "").Build()
	actualInfrastructureMachineTemplateKey := client.ObjectKey{
		Namespace: ns.Name,
		Name:      mdClass.Template.Infrastructure.Ref.Name,
	}
	if err := env.Get(ctx, actualInfrastructureMachineTemplateKey, actualInfrastructureMachineTemplate); err != nil {
		return err
	}
	if err := assertHasOwnerReference(actualInfrastructureMachineTemplate, ownerRefereceTo(actualClusterClass)); err != nil {
		return err
	}

	// Assert the MachineDeploymentClass has the expected APIVersion and Kind to the infrastructure machine template
	if err := referenceExistsWithCorrectKindAndAPIVersion(mdClass.Template.Infrastructure.Ref,
		builder.GenericInfrastructureMachineTemplateKind,
		builder.InfrastructureGroupVersion); err != nil {
		return err
	}

	// Assert the bootstrap template in the MachineDeploymentClass has an owner reference to the ClusterClass.
	actualBootstrapTemplate := builder.BootstrapTemplate("", "").Build()
	actualBootstrapTemplateKey := client.ObjectKey{
		Namespace: ns.Name,
		Name:      mdClass.Template.Bootstrap.Ref.Name,
	}
	if err := env.Get(ctx, actualBootstrapTemplateKey, actualBootstrapTemplate); err != nil {
		return err
	}
	if err := assertHasOwnerReference(actualBootstrapTemplate, ownerRefereceTo(actualClusterClass)); err != nil {
		return err
	}

	// Assert the MachineDeploymentClass has the expected APIVersion and Kind to the bootstrap template
	if err := referenceExistsWithCorrectKindAndAPIVersion(mdClass.Template.Bootstrap.Ref,
		builder.GenericBootstrapConfigTemplateKind,
		builder.BootstrapGroupVersion); err != nil {
		return err
	}

	return nil
}

func assertHasOwnerReference(obj client.Object, ownerRef metav1.OwnerReference) error {
	found := false
	for _, ref := range obj.GetOwnerReferences() {
		if isOwnerReferenceEqual(ref, ownerRef) {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("Object %s does not have OwnerReference %s", tlog.KObj{Obj: obj}, &ownerRef)
	}
	return nil
}

func isOwnerReferenceEqual(a, b metav1.OwnerReference) bool {
	if a.APIVersion != b.APIVersion {
		return false
	}
	if a.Kind != b.Kind {
		return false
	}
	if a.Name != b.Name {
		return false
	}
	if a.UID != b.UID {
		return false
	}
	return true
}

func ownerRefereceTo(obj client.Object) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: obj.GetObjectKind().GroupVersionKind().GroupVersion().String(),
		Kind:       obj.GetObjectKind().GroupVersionKind().Kind,
		Name:       obj.GetName(),
		UID:        obj.GetUID(),
	}
}
