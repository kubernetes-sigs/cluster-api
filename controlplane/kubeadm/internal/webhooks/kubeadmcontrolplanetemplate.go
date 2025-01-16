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

package webhooks

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/util/compare"
)

func (webhook *KubeadmControlPlaneTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&controlplanev1.KubeadmControlPlaneTemplate{}).
		WithDefaulter(webhook, admission.DefaulterRemoveUnknownOrOmitableFields).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-controlplane-cluster-x-k8s-io-v1beta1-kubeadmcontrolplanetemplate,mutating=false,failurePolicy=fail,groups=controlplane.cluster.x-k8s.io,resources=kubeadmcontrolplanetemplates,versions=v1beta1,name=validation.kubeadmcontrolplanetemplate.controlplane.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-controlplane-cluster-x-k8s-io-v1beta1-kubeadmcontrolplanetemplate,mutating=true,failurePolicy=fail,groups=controlplane.cluster.x-k8s.io,resources=kubeadmcontrolplanetemplates,versions=v1beta1,name=default.kubeadmcontrolplanetemplate.controlplane.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// KubeadmControlPlaneTemplate implements a validation and defaulting webhook for KubeadmControlPlaneTemplate.
type KubeadmControlPlaneTemplate struct{}

var _ webhook.CustomValidator = &KubeadmControlPlaneTemplate{}
var _ webhook.CustomDefaulter = &KubeadmControlPlaneTemplate{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (webhook *KubeadmControlPlaneTemplate) Default(_ context.Context, obj runtime.Object) error {
	k, ok := obj.(*controlplanev1.KubeadmControlPlaneTemplate)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmControlPlaneTemplate but got a %T", obj))
	}

	k.Spec.Template.Spec.KubeadmConfigSpec.Default()

	k.Spec.Template.Spec.RolloutStrategy = defaultRolloutStrategy(k.Spec.Template.Spec.RolloutStrategy)

	return nil
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *KubeadmControlPlaneTemplate) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	k, ok := obj.(*controlplanev1.KubeadmControlPlaneTemplate)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmControlPlaneTemplate but got a %T", obj))
	}

	// NOTE: KubeadmControlPlaneTemplate is behind ClusterTopology feature gate flag; the web hook
	// must prevent creating new objects in case the feature flag is disabled.
	if !feature.Gates.Enabled(feature.ClusterTopology) {
		return nil, field.Forbidden(
			field.NewPath("spec"),
			"can be set only if the ClusterTopology feature flag is enabled",
		)
	}

	spec := k.Spec.Template.Spec
	allErrs := validateKubeadmControlPlaneTemplateResourceSpec(spec, field.NewPath("spec", "template", "spec"))
	allErrs = append(allErrs, validateClusterConfiguration(nil, spec.KubeadmConfigSpec.ClusterConfiguration, field.NewPath("spec", "template", "spec", "kubeadmConfigSpec", "clusterConfiguration"))...)
	allErrs = append(allErrs, spec.KubeadmConfigSpec.Validate(field.NewPath("spec", "template", "spec", "kubeadmConfigSpec"))...)
	// Validate the metadata of the KubeadmControlPlaneTemplateResource
	allErrs = append(allErrs, k.Spec.Template.ObjectMeta.Validate(field.NewPath("spec", "template", "metadata"))...)
	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("KubeadmControlPlaneTemplate").GroupKind(), k.Name, allErrs)
	}
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *KubeadmControlPlaneTemplate) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList

	oldK, ok := oldObj.(*controlplanev1.KubeadmControlPlaneTemplate)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmControlPlaneTemplate but got a %T", oldObj))
	}

	newK, ok := newObj.(*controlplanev1.KubeadmControlPlaneTemplate)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmControlPlaneTemplate but got a %T", newObj))
	}

	if err := webhook.Default(ctx, oldK); err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("failed to compare old and new KubeadmControlPlaneTemplate: failed to default old object: %v", err))
	}
	if err := webhook.Default(ctx, newK); err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("failed to compare old and new KubeadmControlPlaneTemplate: failed to default new object: %v", err))
	}

	equal, diff, err := compare.Diff(oldK.Spec.Template.Spec, newK.Spec.Template.Spec)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("failed to compare old and new KubeadmControlPlaneTemplate: %v", err))
	}
	if !equal {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "template", "spec"), newK, fmt.Sprintf("KubeadmControlPlaneTemplate spec.template.spec field is immutable. Please create new resource instead. Diff: %s", diff)),
		)
	}

	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("KubeadmControlPlaneTemplate").GroupKind(), newK.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *KubeadmControlPlaneTemplate) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// validateKubeadmControlPlaneTemplateResourceSpec is a copy of validateKubeadmControlPlaneSpec which
// only validates the fields in KubeadmControlPlaneTemplateResourceSpec we care about.
func validateKubeadmControlPlaneTemplateResourceSpec(s controlplanev1.KubeadmControlPlaneTemplateResourceSpec, pathPrefix *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, validateRolloutBefore(s.RolloutBefore, pathPrefix.Child("rolloutBefore"))...)
	allErrs = append(allErrs, validateRolloutStrategy(s.RolloutStrategy, nil, pathPrefix.Child("rolloutStrategy"))...)
	if s.MachineNamingStrategy != nil {
		allErrs = append(allErrs, validateNamingStrategy(s.MachineNamingStrategy, pathPrefix.Child("machineNamingStrategy"))...)
	}

	if s.MachineTemplate != nil {
		// Validate the metadata of the MachineTemplate
		allErrs = append(allErrs, s.MachineTemplate.ObjectMeta.Validate(pathPrefix.Child("machineTemplate", "metadata"))...)
	}

	return allErrs
}
