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

package clusterclass

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/feature"
	tlog "sigs.k8s.io/cluster-api/internal/log"
	fakeruntimeclient "sigs.k8s.io/cluster-api/internal/runtime/client/fake"
	"sigs.k8s.io/cluster-api/internal/test/builder"
)

func TestClusterClassReconciler_reconcile(t *testing.T) {
	g := NewWithT(t)
	timeout := 30 * time.Second

	ns, err := env.CreateNamespace(ctx, "test-topology-clusterclass-reconciler")
	g.Expect(err).NotTo(HaveOccurred())

	clusterClassName := "class1"
	workerClassName1 := "linux-worker-1"
	workerClassName2 := "linux-worker-2"

	// The below objects are created in order to feed the reconcile loop all the information it needs to create a
	// full tree of ClusterClass objects (the objects should have owner references to the ClusterClass).

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
		WithVariables(
			clusterv1.ClusterClassVariable{
				Name:     "hdd",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "string",
					},
				},
			},
			clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
					},
				},
			}).
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

		g.Expect(assertStatusVariables(actualClusterClass)).Should(Succeed())
		return nil
	}, timeout).Should(Succeed())
}

func assertStatusVariables(actualClusterClass *clusterv1.ClusterClass) error {
	// Assert that each inline variable definition has been exposed in the ClusterClass status.
	for _, specVar := range actualClusterClass.Spec.Variables {
		var found bool
		for _, statusVar := range actualClusterClass.Status.Variables {
			if specVar.Name != statusVar.Name {
				continue
			}
			found = true
			if statusVar.DefinitionsConflict {
				return errors.Errorf("ClusterClass status %s variable DefinitionsConflict does not match. Expected %v , got %v", specVar.Name, false, statusVar.DefinitionsConflict)
			}
			if len(statusVar.Definitions) != 1 {
				return errors.Errorf("ClusterClass status has multiple definitions for variable %s. Expected a single definition", specVar.Name)
			}
			// For this test assume there is only one status variable definition, and that it should match the spec.
			statusVarDefinition := statusVar.Definitions[0]
			if statusVarDefinition.From != clusterv1.VariableDefinitionFromInline {
				return errors.Errorf("ClusterClass status variable %s from field does not match. Expected %s. Got %s", statusVar.Name, clusterv1.VariableDefinitionFromInline, statusVarDefinition.From)
			}
			if specVar.Required != statusVarDefinition.Required {
				return errors.Errorf("ClusterClass status variable %s required field does not match. Expecte %v. Got %v", specVar.Name, statusVarDefinition.Required, statusVarDefinition.Required)
			}
			if !reflect.DeepEqual(specVar.Schema, statusVarDefinition.Schema) {
				return errors.Errorf("ClusterClass status variable %s schema does not match. Expected %v. Got %v", specVar.Name, specVar.Schema, statusVarDefinition.Schema)
			}
		}
		if !found {
			return errors.Errorf("ClusterClass does not have status for variable %s", specVar.Name)
		}
	}
	return nil
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
	if err := assertHasOwnerReference(actualInfraClusterTemplate, *ownerReferenceTo(actualClusterClass)); err != nil {
		return err
	}

	// Assert the ClusterClass has the expected APIVersion and Kind of to the infrastructure cluster template
	return referenceExistsWithCorrectKindAndAPIVersion(actualClusterClass.Spec.Infrastructure.Ref,
		builder.GenericInfrastructureClusterTemplateKind,
		builder.InfrastructureGroupVersion)
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
	if err := assertHasOwnerReference(actualControlPlaneTemplate, *ownerReferenceTo(actualClusterClass)); err != nil {
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
		if err := assertHasOwnerReference(actualInfrastructureMachineTemplate, *ownerReferenceTo(actualClusterClass)); err != nil {
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
	if err := assertHasOwnerReference(actualInfrastructureMachineTemplate, *ownerReferenceTo(actualClusterClass)); err != nil {
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
	if err := assertHasOwnerReference(actualBootstrapTemplate, *ownerReferenceTo(actualClusterClass)); err != nil {
		return err
	}

	// Assert the MachineDeploymentClass has the expected APIVersion and Kind to the bootstrap template
	return referenceExistsWithCorrectKindAndAPIVersion(mdClass.Template.Bootstrap.Ref,
		builder.GenericBootstrapConfigTemplateKind,
		builder.BootstrapGroupVersion)
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
		return fmt.Errorf("object %s does not have OwnerReference %s", tlog.KObj{Obj: obj}, &ownerRef)
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

func TestReconciler_reconcileVariables(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, true)()

	g := NewWithT(t)
	catalog := runtimecatalog.New()
	_ = runtimehooksv1.AddToCatalog(catalog)

	clusterClassWithInlineVariables := builder.ClusterClass(metav1.NamespaceDefault, "class1").
		WithVariables(
			[]clusterv1.ClusterClassVariable{
				{
					Name: "cpu",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "integer",
						},
					},
				},
				{
					Name: "memory",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "string",
						},
					},
				},
			}...,
		)
	tests := []struct {
		name          string
		clusterClass  *clusterv1.ClusterClass
		want          []clusterv1.ClusterClassStatusVariable
		patchResponse *runtimehooksv1.DiscoverVariablesResponse
		wantErr       bool
	}{
		{
			name:         "Reconcile inline variables to ClusterClass status",
			clusterClass: clusterClassWithInlineVariables.DeepCopy().Build(),
			want: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							From: clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
						},
					},
				},
				{
					Name: "memory",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							From: clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Reconcile variables from inline and external variables to ClusterClass status",
			clusterClass: clusterClassWithInlineVariables.DeepCopy().WithPatches(
				[]clusterv1.ClusterClassPatch{
					{
						Name: "patch1",
						External: &clusterv1.ExternalPatchDefinition{
							DiscoverVariablesExtension: pointer.String("variables-one"),
						}}}).
				Build(),
			patchResponse: &runtimehooksv1.DiscoverVariablesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusSuccess,
				},
				Variables: []clusterv1.ClusterClassVariable{
					{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
							},
						},
					},
					{
						Name: "memory",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
							},
						},
					},
					{
						Name: "location",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
							},
						},
					},
				},
			},
			want: []clusterv1.ClusterClassStatusVariable{
				{
					Name:                "cpu",
					DefinitionsConflict: true,
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							From: clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
						},
						{
							From: "patch1",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
					},
				},
				{
					Name:                "location",
					DefinitionsConflict: false,
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							From: "patch1",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
					},
				},
				{
					Name:                "memory",
					DefinitionsConflict: false,
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							From: clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
						{
							From: "patch1",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
					},
				},
			},
		},
		{
			name:    "Error if external patch defines a variable with same name multiple times",
			wantErr: true,
			clusterClass: clusterClassWithInlineVariables.DeepCopy().WithPatches(
				[]clusterv1.ClusterClassPatch{
					{
						Name: "patch1",
						External: &clusterv1.ExternalPatchDefinition{
							DiscoverVariablesExtension: pointer.String("variables-one"),
						}}}).
				Build(),
			patchResponse: &runtimehooksv1.DiscoverVariablesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusSuccess,
				},
				Variables: []clusterv1.ClusterClassVariable{
					{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
							},
						},
					},
					{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "integer",
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeRuntimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
				WithCallExtensionResponses(
					map[string]runtimehooksv1.ResponseObject{
						"variables-one": tt.patchResponse,
					}).
				WithCatalog(catalog).
				Build()

			r := &Reconciler{
				RuntimeClient: fakeRuntimeClient,
			}

			err := r.reconcileVariables(ctx, tt.clusterClass)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(tt.clusterClass.Status.Variables).To(Equal(tt.want), cmp.Diff(tt.clusterClass.Status.Variables, tt.want))
		})
	}
}
