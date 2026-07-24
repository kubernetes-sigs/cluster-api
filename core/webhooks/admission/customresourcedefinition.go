/*
Copyright 2025 The Kubernetes Authors.

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

package admission

import (
	"context"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// +kubebuilder:webhook:verbs=update,path=/validate-apiextensions-k8s-io-v1-customresourcedefinition,mutating=false,failurePolicy=ignore,groups=apiextensions.k8s.io,resources=customresourcedefinitions,versions=v1,name=validation.customresourcedefinition.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1

// CustomResourceDefinitionWebhook validates CRD updates to prevent removal of
// apiVersions that are still referenced by ClusterClass templateRefs.
type CustomResourceDefinitionWebhook struct {
	Client client.Reader
}

// SetupWebhookWithManager registers the webhook with the manager.
func (crdWebhook *CustomResourceDefinitionWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &apiextensionsv1.CustomResourceDefinition{}).
		WithValidator(crdWebhook).
		Complete()
}

var _ admission.Validator[*apiextensionsv1.CustomResourceDefinition] = &CustomResourceDefinitionWebhook{}

// ValidateCreate is a no-op; a new CRD cannot drop versions.
func (crdWebhook *CustomResourceDefinitionWebhook) ValidateCreate(_ context.Context, _ *apiextensionsv1.CustomResourceDefinition) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate rejects the update if it drops an apiVersion still pinned in any ClusterClass.
func (crdWebhook *CustomResourceDefinitionWebhook) ValidateUpdate(ctx context.Context, oldCRD, newCRD *apiextensionsv1.CustomResourceDefinition) (admission.Warnings, error) {
	dropped := droppedCRDVersions(oldCRD, newCRD)
	if dropped.Len() == 0 {
		return nil, nil
	}

	affected, err := crdWebhook.clusterClassesReferencingDroppedVersions(ctx, oldCRD.Spec.Group, oldCRD.Spec.Names.Kind, dropped)
	if err != nil {
		return nil, err
	}
	if len(affected) > 0 {
		return nil, fmt.Errorf(
			"cannot remove apiVersion(s) %v from %s/%s: still referenced by ClusterClass(es): %v",
			sets.List(dropped), oldCRD.Spec.Group, oldCRD.Spec.Names.Kind, affected,
		)
	}
	return nil, nil
}

// ValidateDelete is a no-op; we're only concerned with dropped versions (on updates), while the associated GKs are still present.
func (crdWebhook *CustomResourceDefinitionWebhook) ValidateDelete(_ context.Context, _ *apiextensionsv1.CustomResourceDefinition) (admission.Warnings, error) {
	return nil, nil
}

// droppedCRDVersions returns the set of versions present in old but absent in new.
func droppedCRDVersions(oldCRD, newCRD *apiextensionsv1.CustomResourceDefinition) sets.Set[string] {
	oldVersions := sets.New[string]()
	for _, v := range oldCRD.Spec.Versions {
		oldVersions.Insert(v.Name)
	}
	newVersions := sets.New[string]()
	for _, v := range newCRD.Spec.Versions {
		newVersions.Insert(v.Name)
	}
	return oldVersions.Difference(newVersions)
}

// clusterClassesReferencingDroppedVersions returns the namespaced names of all ClusterClasses
// that pin the given GK at one of the dropped versions.
func (crdWebhook *CustomResourceDefinitionWebhook) clusterClassesReferencingDroppedVersions(
	ctx context.Context, group, kind string, dropped sets.Set[string],
) ([]string, error) {
	list := &clusterv1.ClusterClassList{}
	if err := crdWebhook.Client.List(ctx, list); err != nil {
		return nil, err
	}
	var affected []string
	for i := range list.Items {
		if clusterClassReferencesDroppedVersion(&list.Items[i], group, kind, dropped) {
			affected = append(affected, list.Items[i].Namespace+"/"+list.Items[i].Name)
		}
	}
	return affected, nil
}

// clusterClassReferencesDroppedVersion reports whether any templateRef in the ClusterClass
// points to the given group/kind at one of the dropped versions.
func clusterClassReferencesDroppedVersion(clusterClass *clusterv1.ClusterClass, group, kind string, dropped sets.Set[string]) bool {
	for _, ref := range clusterClassTemplateRefs(clusterClass) {
		if ref.apiVersion == "" || ref.kind == "" {
			continue
		}
		gv, err := schema.ParseGroupVersion(ref.apiVersion)
		if err != nil {
			continue
		}
		if gv.Group == group && ref.kind == kind && dropped.Has(gv.Version) {
			return true
		}
	}
	return false
}

// templateRef is a minimal common representation of ClusterClassTemplateReference
// and MachineHealthCheckRemediationTemplateReference (same fields, different Go types).
type templateRef struct {
	apiVersion string
	kind       string
}

// clusterClassTemplateRefs collects all templateRef locations from a ClusterClass.
func clusterClassTemplateRefs(clusterClass *clusterv1.ClusterClass) []templateRef {
	var refs []templateRef
	refsAdder := func(apiVersion, kind string) {
		refs = append(refs, templateRef{apiVersion: apiVersion, kind: kind})
	}

	refsAdder(clusterClass.Spec.Infrastructure.TemplateRef.APIVersion, clusterClass.Spec.Infrastructure.TemplateRef.Kind)
	refsAdder(clusterClass.Spec.ControlPlane.TemplateRef.APIVersion, clusterClass.Spec.ControlPlane.TemplateRef.Kind)
	refsAdder(clusterClass.Spec.ControlPlane.MachineInfrastructure.TemplateRef.APIVersion, clusterClass.Spec.ControlPlane.MachineInfrastructure.TemplateRef.Kind)
	refsAdder(clusterClass.Spec.ControlPlane.HealthCheck.Remediation.TemplateRef.APIVersion, clusterClass.Spec.ControlPlane.HealthCheck.Remediation.TemplateRef.Kind)

	for _, md := range clusterClass.Spec.Workers.MachineDeployments {
		refsAdder(md.Bootstrap.TemplateRef.APIVersion, md.Bootstrap.TemplateRef.Kind)
		refsAdder(md.Infrastructure.TemplateRef.APIVersion, md.Infrastructure.TemplateRef.Kind)
		refsAdder(md.HealthCheck.Remediation.TemplateRef.APIVersion, md.HealthCheck.Remediation.TemplateRef.Kind)
	}
	for _, mp := range clusterClass.Spec.Workers.MachinePools {
		refsAdder(mp.Bootstrap.TemplateRef.APIVersion, mp.Bootstrap.TemplateRef.Kind)
		refsAdder(mp.Infrastructure.TemplateRef.APIVersion, mp.Infrastructure.TemplateRef.Kind)
	}

	return refs
}
