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
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimeclient "sigs.k8s.io/cluster-api/exp/runtime/client"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/feature"
	fakeruntimeclient "sigs.k8s.io/cluster-api/internal/runtime/client/fake"
	"sigs.k8s.io/cluster-api/util/cache"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestClusterClassReconciler_reconcile(t *testing.T) {
	g := NewWithT(t)
	timeout := 30 * time.Second

	ns, err := env.CreateNamespace(ctx, "test-topology-clusterclass-reconciler")
	g.Expect(err).ToNot(HaveOccurred())

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
	infraMachinePoolTemplateWorker := builder.InfrastructureMachinePoolTemplate(ns.Name, "inframachinepooltemplate-worker").Build()

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

	// MachinePoolClasses that will be part of the ClusterClass.
	machinePoolClass1 := builder.MachinePoolClass(workerClassName1).
		WithBootstrapTemplate(bootstrapTemplate).
		WithInfrastructureTemplate(infraMachinePoolTemplateWorker).
		Build()
	machinePoolClass2 := builder.MachinePoolClass(workerClassName2).
		WithBootstrapTemplate(bootstrapTemplate).
		WithInfrastructureTemplate(infraMachinePoolTemplateWorker).
		Build()

	// ClusterClass.
	clusterClass := builder.ClusterClass(ns.Name, clusterClassName).
		WithInfrastructureClusterTemplate(infraClusterTemplate).
		WithControlPlaneTemplate(controlPlaneTemplate).
		WithControlPlaneInfrastructureMachineTemplate(infraMachineTemplateControlPlane).
		WithWorkerMachineDeploymentClasses(*machineDeploymentClass1, *machineDeploymentClass2).
		WithWorkerMachinePoolClasses(*machinePoolClass1, *machinePoolClass2).
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
						XMetadata: &clusterv1.VariableSchemaMetadata{
							Labels: map[string]string{
								"some-label": "some-label-value",
							},
							Annotations: map[string]string{
								"some-annotation": "some-annotation-value",
							},
						},
					},
				},
				Metadata: clusterv1.ClusterClassVariableMetadata{
					Labels: map[string]string{
						"some-label": "some-label-value",
					},
					Annotations: map[string]string{
						"some-annotation": "some-annotation-value",
					},
				},
			}).
		Build()

	// Create the set of initObjects from the objects above to add to the API server when the test environment starts.
	initObjs := []client.Object{
		bootstrapTemplate,
		infraMachineTemplateWorker,
		infraMachinePoolTemplateWorker,
		infraMachineTemplateControlPlane,
		controlPlaneTemplate,
		infraClusterTemplate,
		clusterClass,
	}

	for _, obj := range initObjs {
		g.Expect(env.CreateAndWait(ctx, obj)).To(Succeed())
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

		g.Expect(assertMachinePoolClasses(ctx, actualClusterClass, ns)).Should(Succeed())

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
			if !cmp.Equal(specVar.Schema, statusVarDefinition.Schema) {
				return errors.Errorf("ClusterClass status variable %s schema does not match. Expected %v. Got %v", specVar.Name, specVar.Schema, statusVarDefinition.Schema)
			}
			if !cmp.Equal(specVar.Metadata, statusVarDefinition.Metadata) {
				return errors.Errorf("ClusterClass status variable %s metadata does not match. Expected %v. Got %v", specVar.Name, specVar.Metadata, statusVarDefinition.Metadata)
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
	if err := assertHasOwnerReference(actualInfraClusterTemplate, *ownerReferenceTo(actualClusterClass, clusterv1.GroupVersion.WithKind("ClusterClass"))); err != nil {
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
	if err := assertHasOwnerReference(actualControlPlaneTemplate, *ownerReferenceTo(actualClusterClass, clusterv1.GroupVersion.WithKind("ClusterClass"))); err != nil {
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
		if err := assertHasOwnerReference(actualInfrastructureMachineTemplate, *ownerReferenceTo(actualClusterClass, clusterv1.GroupVersion.WithKind("ClusterClass"))); err != nil {
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
	if err := assertHasOwnerReference(actualInfrastructureMachineTemplate, *ownerReferenceTo(actualClusterClass, clusterv1.GroupVersion.WithKind("ClusterClass"))); err != nil {
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
	if err := assertHasOwnerReference(actualBootstrapTemplate, *ownerReferenceTo(actualClusterClass, clusterv1.GroupVersion.WithKind("ClusterClass"))); err != nil {
		return err
	}

	// Assert the MachineDeploymentClass has the expected APIVersion and Kind to the bootstrap template
	return referenceExistsWithCorrectKindAndAPIVersion(mdClass.Template.Bootstrap.Ref,
		builder.GenericBootstrapConfigTemplateKind,
		builder.BootstrapGroupVersion)
}

func assertMachinePoolClasses(ctx context.Context, actualClusterClass *clusterv1.ClusterClass, ns *corev1.Namespace) error {
	for _, mpClass := range actualClusterClass.Spec.Workers.MachinePools {
		if err := assertMachinePoolClass(ctx, actualClusterClass, mpClass, ns); err != nil {
			return err
		}
	}
	return nil
}

func assertMachinePoolClass(ctx context.Context, actualClusterClass *clusterv1.ClusterClass, mpClass clusterv1.MachinePoolClass, ns *corev1.Namespace) error {
	// Assert the infrastructure machinepool template in the MachinePoolClass has an owner reference to the ClusterClass.
	actualInfrastructureMachinePoolTemplate := builder.InfrastructureMachinePoolTemplate("", "").Build()
	actualInfrastructureMachinePoolTemplateKey := client.ObjectKey{
		Namespace: ns.Name,
		Name:      mpClass.Template.Infrastructure.Ref.Name,
	}
	if err := env.Get(ctx, actualInfrastructureMachinePoolTemplateKey, actualInfrastructureMachinePoolTemplate); err != nil {
		return err
	}
	if err := assertHasOwnerReference(actualInfrastructureMachinePoolTemplate, *ownerReferenceTo(actualClusterClass, clusterv1.GroupVersion.WithKind("ClusterClass"))); err != nil {
		return err
	}

	// Assert the MachinePoolClass has the expected APIVersion and Kind to the infrastructure machinepool template
	if err := referenceExistsWithCorrectKindAndAPIVersion(mpClass.Template.Infrastructure.Ref,
		builder.GenericInfrastructureMachinePoolTemplateKind,
		builder.InfrastructureGroupVersion); err != nil {
		return err
	}

	// Assert the bootstrap template in the MachinePoolClass has an owner reference to the ClusterClass.
	actualBootstrapTemplate := builder.BootstrapTemplate("", "").Build()
	actualBootstrapTemplateKey := client.ObjectKey{
		Namespace: ns.Name,
		Name:      mpClass.Template.Bootstrap.Ref.Name,
	}
	if err := env.Get(ctx, actualBootstrapTemplateKey, actualBootstrapTemplate); err != nil {
		return err
	}
	if err := assertHasOwnerReference(actualBootstrapTemplate, *ownerReferenceTo(actualClusterClass, clusterv1.GroupVersion.WithKind("ClusterClass"))); err != nil {
		return err
	}

	// Assert the MachinePoolClass has the expected APIVersion and Kind to the bootstrap template
	return referenceExistsWithCorrectKindAndAPIVersion(mpClass.Template.Bootstrap.Ref,
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
		// Note: Kind might be empty here as it's usually only set on Unstructured objects.
		// But as this is just test code we don't care too much about it.
		return fmt.Errorf("%s %s does not have OwnerReference %s", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj), &ownerRef)
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
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, true)

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
							XMetadata: &clusterv1.VariableSchemaMetadata{
								Labels: map[string]string{
									"some-label": "some-label-value",
								},
								Annotations: map[string]string{
									"some-annotation": "some-annotation-value",
								},
							},
							XValidations: []clusterv1.ValidationRule{{
								Rule:    "self >= 1",
								Message: "integer must be greater or equal than 1",
								Reason:  clusterv1.FieldValueInvalid,
							}},
						},
					},
					Metadata: clusterv1.ClusterClassVariableMetadata{
						Labels: map[string]string{
							"some-label": "some-label-value",
						},
						Annotations: map[string]string{
							"some-annotation": "some-annotation-value",
						},
					},
				},
				{
					Name: "memory",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "string",
							XValidations: []clusterv1.ValidationRule{{
								Rule:              "true",
								MessageExpression: "'test error message, got value %s'.format([self])",
							}},
						},
					},
				},
			}...,
		)
	tests := []struct {
		name                              string
		clusterClass                      *clusterv1.ClusterClass
		want                              []clusterv1.ClusterClassStatusVariable
		patchResponse                     *runtimehooksv1.DiscoverVariablesResponse
		wantVariableDiscoveryErrorMessage string
		wantErrMessage                    string
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
									XMetadata: &clusterv1.VariableSchemaMetadata{
										Labels: map[string]string{
											"some-label": "some-label-value",
										},
										Annotations: map[string]string{
											"some-annotation": "some-annotation-value",
										},
									},
									XValidations: []clusterv1.ValidationRule{{
										Rule:    "self >= 1",
										Message: "integer must be greater or equal than 1",
										Reason:  clusterv1.FieldValueInvalid,
									}},
								},
							},
							Metadata: clusterv1.ClusterClassVariableMetadata{
								Labels: map[string]string{
									"some-label": "some-label-value",
								},
								Annotations: map[string]string{
									"some-annotation": "some-annotation-value",
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
									XValidations: []clusterv1.ValidationRule{{
										Rule:              "true",
										MessageExpression: "'test error message, got value %s'.format([self])",
									}},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Reconcile inline and external variables to ClusterClass status",
			clusterClass: clusterClassWithInlineVariables.DeepCopy().WithPatches(
				[]clusterv1.ClusterClassPatch{
					{
						Name: "patch1",
						External: &clusterv1.ExternalPatchDefinition{
							DiscoverVariablesExtension: ptr.To("variables-one"),
						},
					},
				}).
				Build(),
			patchResponse: &runtimehooksv1.DiscoverVariablesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusSuccess,
				},
				Variables: []clusterv1.ClusterClassVariable{
					{
						Name: "cpu",
						// Note: This schema must be exactly equal to the one in clusterClassWithInlineVariables to avoid conflicts.
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "integer",
								XMetadata: &clusterv1.VariableSchemaMetadata{
									Labels: map[string]string{
										"some-label": "some-label-value",
									},
									Annotations: map[string]string{
										"some-annotation": "some-annotation-value",
									},
								},
								XValidations: []clusterv1.ValidationRule{{
									Rule:    "self >= 1",
									Message: "integer must be greater or equal than 1",
									Reason:  clusterv1.FieldValueInvalid,
								}},
							},
						},
						Metadata: clusterv1.ClusterClassVariableMetadata{
							Labels: map[string]string{
								"some-label": "some-label-value",
							},
							Annotations: map[string]string{
								"some-annotation": "some-annotation-value",
							},
						},
					},
					{
						Name: "memory",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
								XValidations: []clusterv1.ValidationRule{{
									Rule:              "true",
									MessageExpression: "'test error message, got value %s'.format([self])",
								}},
							},
						},
					},
					{
						Name: "location",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
								XMetadata: &clusterv1.VariableSchemaMetadata{
									Labels: map[string]string{
										"some-label": "some-label-value",
									},
									Annotations: map[string]string{
										"some-annotation": "some-annotation-value",
									},
								},
							},
						},
						Metadata: clusterv1.ClusterClassVariableMetadata{
							Labels: map[string]string{
								"some-label": "some-label-value",
							},
							Annotations: map[string]string{
								"some-annotation": "some-annotation-value",
							},
						},
					},
				},
			},
			want: []clusterv1.ClusterClassStatusVariable{
				{
					Name:                "cpu",
					DefinitionsConflict: false,
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							From: clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
									XMetadata: &clusterv1.VariableSchemaMetadata{
										Labels: map[string]string{
											"some-label": "some-label-value",
										},
										Annotations: map[string]string{
											"some-annotation": "some-annotation-value",
										},
									},
									XValidations: []clusterv1.ValidationRule{{
										Rule:    "self >= 1",
										Message: "integer must be greater or equal than 1",
										Reason:  clusterv1.FieldValueInvalid,
									}},
								},
							},
							Metadata: clusterv1.ClusterClassVariableMetadata{
								Labels: map[string]string{
									"some-label": "some-label-value",
								},
								Annotations: map[string]string{
									"some-annotation": "some-annotation-value",
								},
							},
						},
						{
							From: "patch1",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
									XMetadata: &clusterv1.VariableSchemaMetadata{
										Labels: map[string]string{
											"some-label": "some-label-value",
										},
										Annotations: map[string]string{
											"some-annotation": "some-annotation-value",
										},
									},
									XValidations: []clusterv1.ValidationRule{{
										Rule:    "self >= 1",
										Message: "integer must be greater or equal than 1",
										Reason:  clusterv1.FieldValueInvalid,
									}},
								},
							},
							Metadata: clusterv1.ClusterClassVariableMetadata{
								Labels: map[string]string{
									"some-label": "some-label-value",
								},
								Annotations: map[string]string{
									"some-annotation": "some-annotation-value",
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
									XMetadata: &clusterv1.VariableSchemaMetadata{
										Labels: map[string]string{
											"some-label": "some-label-value",
										},
										Annotations: map[string]string{
											"some-annotation": "some-annotation-value",
										},
									},
								},
							},
							Metadata: clusterv1.ClusterClassVariableMetadata{
								Labels: map[string]string{
									"some-label": "some-label-value",
								},
								Annotations: map[string]string{
									"some-annotation": "some-annotation-value",
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
									XValidations: []clusterv1.ValidationRule{{
										Rule:              "true",
										MessageExpression: "'test error message, got value %s'.format([self])",
									}},
								},
							},
						},
						{
							From: "patch1",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
									XValidations: []clusterv1.ValidationRule{{
										Rule:              "true",
										MessageExpression: "'test error message, got value %s'.format([self])",
									}},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Error if reconciling inline and external variables to ClusterClass status with conflicts",
			clusterClass: clusterClassWithInlineVariables.DeepCopy().WithPatches(
				[]clusterv1.ClusterClassPatch{
					{
						Name: "patch1",
						External: &clusterv1.ExternalPatchDefinition{
							DiscoverVariablesExtension: ptr.To("variables-one"),
						},
					},
				}).
				Build(),
			patchResponse: &runtimehooksv1.DiscoverVariablesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusSuccess,
				},
				Variables: []clusterv1.ClusterClassVariable{
					{
						Name: "cpu",
						// Note: This schema conflicts with the schema in clusterClassWithInlineVariables.
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
							},
						},
					},
				},
			},
			wantErrMessage: "failed to discover variables for ClusterClass class1: " +
				"the following variables have conflicting schemas: cpu",
			wantVariableDiscoveryErrorMessage: "VariableDiscovery failed: the following variables have conflicting schemas: cpu",
		},
		{
			name: "Reconcile external variables to ClusterClass status",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").DeepCopy().WithPatches(
				[]clusterv1.ClusterClassPatch{
					{
						Name: "patch1",
						External: &clusterv1.ExternalPatchDefinition{
							DiscoverVariablesExtension: ptr.To("variables-one"),
						},
					},
				}).
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
								XValidations: []clusterv1.ValidationRule{{
									Rule:              "true",
									MessageExpression: "'test error message, got value %s'.format([self])",
								}},
							},
						},
					},
					{
						Name: "httpProxy",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"enabled": {
										Type: "boolean",
									},
								},
								XValidations: []clusterv1.ValidationRule{{
									Rule:              "true",
									MessageExpression: "'test error message, got value %s'.format([self.enabled])",
									FieldPath:         ".enabled",
								}},
							},
						},
					},
					{
						Name: "location",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
								XMetadata: &clusterv1.VariableSchemaMetadata{
									Labels: map[string]string{
										"some-label": "some-label-value",
									},
									Annotations: map[string]string{
										"some-annotation": "some-annotation-value",
									},
								},
							},
						},
						Metadata: clusterv1.ClusterClassVariableMetadata{
							Labels: map[string]string{
								"some-label": "some-label-value",
							},
							Annotations: map[string]string{
								"some-annotation": "some-annotation-value",
							},
						},
					},
				},
			},
			want: []clusterv1.ClusterClassStatusVariable{
				{
					Name:                "cpu",
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
					Name: "httpProxy",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							From: "patch1",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "object",
									Properties: map[string]clusterv1.JSONSchemaProps{
										"enabled": {
											Type: "boolean",
										},
									},
									XValidations: []clusterv1.ValidationRule{{
										Rule:              "true",
										MessageExpression: "'test error message, got value %s'.format([self.enabled])",
										FieldPath:         ".enabled",
									}},
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
									XMetadata: &clusterv1.VariableSchemaMetadata{
										Labels: map[string]string{
											"some-label": "some-label-value",
										},
										Annotations: map[string]string{
											"some-annotation": "some-annotation-value",
										},
									},
								},
							},
							Metadata: clusterv1.ClusterClassVariableMetadata{
								Labels: map[string]string{
									"some-label": "some-label-value",
								},
								Annotations: map[string]string{
									"some-annotation": "some-annotation-value",
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
							From: "patch1",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
									XValidations: []clusterv1.ValidationRule{{
										Rule:              "true",
										MessageExpression: "'test error message, got value %s'.format([self])",
									}},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Error if external patch defines a variable with same name multiple times",
			clusterClass: clusterClassWithInlineVariables.DeepCopy().WithPatches(
				[]clusterv1.ClusterClassPatch{
					{
						Name: "patch1",
						External: &clusterv1.ExternalPatchDefinition{
							DiscoverVariablesExtension: ptr.To("variables-one"),
						},
					},
				}).
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
			wantErrMessage: "failed to discover variables for ClusterClass class1: " +
				"patch1.variables[cpu].name: Invalid value: \"cpu\": variable name must be unique. " +
				"Variable with name \"cpu\" is defined more than once",
			wantVariableDiscoveryErrorMessage: "VariableDiscovery failed: patch1.variables[cpu].name: Invalid value: \"cpu\": variable name must be unique. Variable with name \"cpu\" is defined more than once",
		},
		{
			name: "Error if external patch returns an invalid variable (OpenAPI schema)",
			clusterClass: clusterClassWithInlineVariables.DeepCopy().WithPatches(
				[]clusterv1.ClusterClassPatch{
					{
						Name: "patch1",
						External: &clusterv1.ExternalPatchDefinition{
							DiscoverVariablesExtension: ptr.To("variables-one"),
						},
					},
				}).
				Build(),
			patchResponse: &runtimehooksv1.DiscoverVariablesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusSuccess,
				},
				Variables: []clusterv1.ClusterClassVariable{
					{
						Name: "httpProxy",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"enabled": {
										Type:    "boolean",
										Default: &apiextensionsv1.JSON{Raw: []byte(`false`)},
									},
									"url": {
										Type: "string",
									},
									"noProxy": {
										Type: "invalidType", // invalid type.
									},
								},
							},
						},
					},
				},
			},
			wantErrMessage: "failed to discover variables for ClusterClass class1: " +
				"patch1.variables[httpProxy].schema.openAPIV3Schema.properties[noProxy].type: Unsupported value: \"invalidType\": " +
				"supported values: \"array\", \"boolean\", \"integer\", \"number\", \"object\", \"string\"",
			wantVariableDiscoveryErrorMessage: "VariableDiscovery failed: patch1.variables[httpProxy].schema.openAPIV3Schema.properties[noProxy].type: Unsupported value: \"invalidType\": " +
				"supported values: \"array\", \"boolean\", \"integer\", \"number\", \"object\", \"string\"",
		},
		{
			name: "Error if external patch returns an invalid variable (CEL)",
			clusterClass: clusterClassWithInlineVariables.DeepCopy().WithPatches(
				[]clusterv1.ClusterClassPatch{
					{
						Name: "patch1",
						External: &clusterv1.ExternalPatchDefinition{
							DiscoverVariablesExtension: ptr.To("variables-one"),
						},
					},
				}).
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
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"nestedField": {
										Type: "integer",
										Default: &apiextensionsv1.JSON{
											Raw: []byte(`0`), // Default value is invalid according to CEL.
										},
										XValidations: []clusterv1.ValidationRule{{
											Rule: "self >= 1",
										}},
									},
								},
							},
						},
					},
					{
						Name: "anotherCPU",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "integer",
								XValidations: []clusterv1.ValidationRule{{
									Rule:              "self >= 1",
									MessageExpression: "'Expected integer greater or equal to 1, got ' + this does not compile", // does not compile
								}},
							},
						},
					},
				},
			},
			wantErrMessage: "failed to discover variables for ClusterClass class1: [" +
				"patch1.variables[cpu].schema.openAPIV3Schema.properties[nestedField].default: Invalid value: \"integer\": failed rule: self >= 1, " +
				"patch1.variables[anotherCPU].schema.openAPIV3Schema.x-kubernetes-validations[0].messageExpression: Invalid value: " +
				"apiextensions.ValidationRule{Rule:\"self >= 1\", Message:\"\", MessageExpression:\"'Expected integer greater or equal to 1, got ' + this does not compile\", " +
				"Reason:(*apiextensions.FieldValueErrorReason)(nil), FieldPath:\"\", OptionalOldSelf:(*bool)(nil)}: " +
				"messageExpression compilation failed: ERROR: <input>:1:55: Syntax error: mismatched input 'does' expecting <EOF>\n " +
				"| 'Expected integer greater or equal to 1, got ' + this does not compile\n " +
				"| ......................................................^]",
			wantVariableDiscoveryErrorMessage: "VariableDiscovery failed: [patch1.variables[cpu].schema.openAPIV3Schema.properties[nestedField].default: Invalid value: \"integer\": failed rule: self >= 1, " +
				"patch1.variables[anotherCPU].schema.openAPIV3Schema.x-kubernetes-validations[0].messageExpression: Invalid value: " +
				"apiextensions.ValidationRule{Rule:\"self >= 1\", Message:\"\", MessageExpression:\"'Expected integer greater or equal to 1, got ' + this does not compile\", " +
				"Reason:(*apiextensions.FieldValueErrorReason)(nil), FieldPath:\"\", OptionalOldSelf:(*bool)(nil)}: " +
				"messageExpression compilation failed: ERROR: <input>:1:55: Syntax error: mismatched input 'does' expecting <EOF>\n " +
				"| 'Expected integer greater or equal to 1, got ' + this does not compile\n " +
				"| ......................................................^]",
		},
		{
			name: "Error if external patch returns an invalid variable (CEL: using opts that are not available with the current compatibility version)",
			// Note: Because there is no "old" version of the variable schemas with RuntimeExtensions, they *always* can only use
			// the options available with the current compatibility version.
			// In contrast, inline variables that are already "stored" in etcd are able to use all options of the "max" env.
			clusterClass: clusterClassWithInlineVariables.DeepCopy().WithPatches(
				[]clusterv1.ClusterClassPatch{
					{
						Name: "patch1",
						External: &clusterv1.ExternalPatchDefinition{
							DiscoverVariablesExtension: ptr.To("variables-one"),
						},
					},
				}).
				Build(),
			patchResponse: &runtimehooksv1.DiscoverVariablesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusSuccess,
				},
				Variables: []clusterv1.ClusterClassVariable{
					{
						Name: "someIP",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
								XValidations: []clusterv1.ValidationRule{{
									// Note: IP will be only available if the compatibility version is 1.30
									Rule: "ip(self).family() == 6",
								}},
							},
						},
					},
				},
			},
			wantErrMessage: "failed to discover variables for ClusterClass class1: " +
				"patch1.variables[someIP].schema.openAPIV3Schema.x-kubernetes-validations[0].rule: Invalid value: " +
				"apiextensions.ValidationRule{Rule:\"ip(self).family() == 6\", Message:\"\", MessageExpression:\"\", Reason:(*apiextensions.FieldValueErrorReason)(nil), FieldPath:\"\", OptionalOldSelf:(*bool)(nil)}: compilation failed: " +
				"ERROR: <input>:1:3: undeclared reference to 'ip' (in container '')\n" +
				" | ip(self).family() == 6\n" +
				" | ..^\n" +
				"ERROR: <input>:1:16: undeclared reference to 'family' (in container '')\n" +
				" | ip(self).family() == 6\n" +
				" | ...............^",
			wantVariableDiscoveryErrorMessage: "VariableDiscovery failed: patch1.variables[someIP].schema.openAPIV3Schema.x-kubernetes-validations[0].rule: Invalid value: " +
				"apiextensions.ValidationRule{Rule:\"ip(self).family() == 6\", Message:\"\", MessageExpression:\"\", Reason:(*apiextensions.FieldValueErrorReason)(nil), FieldPath:\"\", OptionalOldSelf:(*bool)(nil)}: compilation failed: " +
				"ERROR: <input>:1:3: undeclared reference to 'ip' (in container '')\n" +
				" | ip(self).family() == 6\n" +
				" | ..^\n" +
				"ERROR: <input>:1:16: undeclared reference to 'family' (in container '')\n" +
				" | ip(self).family() == 6\n" +
				" | ...............^",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeRuntimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
				WithCallExtensionResponses(
					map[string]runtimehooksv1.ResponseObject{
						"variables-one": tt.patchResponse,
					}).
				WithCatalog(catalog).
				Build()

			r := &Reconciler{
				RuntimeClient:          fakeRuntimeClient,
				discoverVariablesCache: cache.New[runtimeclient.CallExtensionCacheEntry](),
			}

			// Pin the compatibility version used in variable CEL validation to 1.29, so we don't have to continuously refactor
			// the unit tests that verify that compatibility is handled correctly.
			// FIXME(sbueringer)
			//effectiveVer := utilversion.DefaultComponentGlobalsRegistry.EffectiveVersionFor(utilversion.DefaultKubeComponent)
			//if effectiveVer != nil {
			//	g.Expect(effectiveVer.MinCompatibilityVersion()).To(Equal(version.MustParse("v1.29")))
			//} else {
			//	v := utilversion.DefaultKubeEffectiveVersion()
			//	v.SetMinCompatibilityVersion(version.MustParse("v1.29"))
			//	g.Expect(utilversion.DefaultComponentGlobalsRegistry.Register(utilversion.DefaultKubeComponent, v, nil)).To(Succeed())
			//}

			s := &scope{
				clusterClass: tt.clusterClass,
			}
			_, err := r.reconcileVariables(ctx, s)

			if tt.wantVariableDiscoveryErrorMessage != "" {
				g.Expect(s.variableDiscoveryError).To(HaveOccurred())
				g.Expect(s.variableDiscoveryError.Error()).To(Equal(tt.wantVariableDiscoveryErrorMessage))
			} else {
				g.Expect(s.variableDiscoveryError).ToNot(HaveOccurred())
			}

			if tt.wantErrMessage != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.wantErrMessage))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(tt.clusterClass.Status.Variables).To(BeComparableTo(tt.want), cmp.Diff(tt.clusterClass.Status.Variables, tt.want))
		})
	}
}

func TestReconciler_extensionConfigToClusterClass(t *testing.T) {
	firstExtConfig := &runtimev1.ExtensionConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "runtime1",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "ExtensionConfig",
			APIVersion: runtimev1.GroupVersion.String(),
		},
		Spec: runtimev1.ExtensionConfigSpec{
			NamespaceSelector: &metav1.LabelSelector{},
		},
	}
	secondExtConfig := &runtimev1.ExtensionConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "runtime2",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "ExtensionConfig",
			APIVersion: runtimev1.GroupVersion.String(),
		},
		Spec: runtimev1.ExtensionConfigSpec{
			NamespaceSelector: &metav1.LabelSelector{},
		},
	}

	// These ClusterClasses will be reconciled as they both reference the passed ExtensionConfig `runtime1`.
	onePatchClusterClass := builder.ClusterClass(metav1.NamespaceDefault, "cc1").
		WithPatches([]clusterv1.ClusterClassPatch{
			{External: &clusterv1.ExternalPatchDefinition{DiscoverVariablesExtension: ptr.To("discover-variables.runtime1")}},
		}).
		Build()
	twoPatchClusterClass := builder.ClusterClass(metav1.NamespaceDefault, "cc2").
		WithPatches([]clusterv1.ClusterClassPatch{
			{External: &clusterv1.ExternalPatchDefinition{DiscoverVariablesExtension: ptr.To("discover-variables.runtime1")}},
			{External: &clusterv1.ExternalPatchDefinition{DiscoverVariablesExtension: ptr.To("discover-variables.runtime2")}},
		}).
		Build()

	// This ClusterClasses will not be reconciled as it does not reference the passed ExtensionConfig `runtime1`.
	notReconciledClusterClass := builder.ClusterClass(metav1.NamespaceDefault, "cc3").
		WithPatches([]clusterv1.ClusterClassPatch{
			{External: &clusterv1.ExternalPatchDefinition{DiscoverVariablesExtension: ptr.To("discover-variables.other-runtime-class")}},
		}).
		Build()

	t.Run("test", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithObjects(onePatchClusterClass, notReconciledClusterClass, twoPatchClusterClass).Build()
		r := &Reconciler{
			Client: fakeClient,
		}

		// Expect both onePatchClusterClass and twoPatchClusterClass to trigger a reconcile as both reference ExtensionCopnfig `runtime1`.
		firstExtConfigExpected := []reconcile.Request{
			{NamespacedName: types.NamespacedName{Namespace: onePatchClusterClass.Namespace, Name: onePatchClusterClass.Name}},
			{NamespacedName: types.NamespacedName{Namespace: twoPatchClusterClass.Namespace, Name: twoPatchClusterClass.Name}},
		}
		if got := r.extensionConfigToClusterClass(context.Background(), firstExtConfig); !cmp.Equal(got, firstExtConfigExpected) {
			t.Errorf("extensionConfigToClusterClass() = %v, want %v", got, firstExtConfigExpected)
		}

		// Expect only twoPatchClusterClass to trigger a reconcile as it's the only class with a reference to ExtensionCopnfig `runtime2`.
		secondExtConfigExpected := []reconcile.Request{
			{NamespacedName: types.NamespacedName{Namespace: twoPatchClusterClass.Namespace, Name: twoPatchClusterClass.Name}},
		}
		if got := r.extensionConfigToClusterClass(context.Background(), secondExtConfig); !cmp.Equal(got, secondExtConfigExpected) {
			t.Errorf("extensionConfigToClusterClass() = %v, want %v", got, secondExtConfigExpected)
		}
	})
}
