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
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/defaulting"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/util/compare"
)

func (webhook *KubeadmControlPlaneTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&controlplanev1.KubeadmControlPlaneTemplate{}).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-controlplane-cluster-x-k8s-io-v1beta2-kubeadmcontrolplanetemplate,mutating=false,failurePolicy=fail,groups=controlplane.cluster.x-k8s.io,resources=kubeadmcontrolplanetemplates,versions=v1beta2,name=validation.kubeadmcontrolplanetemplate.controlplane.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// KubeadmControlPlaneTemplate implements a validation and defaulting webhook for KubeadmControlPlaneTemplate.
type KubeadmControlPlaneTemplate struct{}

var _ webhook.CustomValidator = &KubeadmControlPlaneTemplate{}

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
func (webhook *KubeadmControlPlaneTemplate) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList

	oldK, ok := oldObj.(*controlplanev1.KubeadmControlPlaneTemplate)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmControlPlaneTemplate but got a %T", oldObj))
	}

	newK, ok := newObj.(*controlplanev1.KubeadmControlPlaneTemplate)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmControlPlaneTemplate but got a %T", newObj))
	}

	// Apply defaults from older versions of CAPI so the following checks do not report differences when
	// dealing with objects created before dropping those defaults.
	defaulting.ApplyPreviousKubeadmConfigDefaults(&oldK.Spec.Template.Spec.KubeadmConfigSpec)
	defaulting.ApplyPreviousKubeadmConfigDefaults(&newK.Spec.Template.Spec.KubeadmConfigSpec)

	// In Cluster API < v1.11 the RolloutStrategy field was defaulted.
	// The defaulting was dropped with Cluster API v1.11.
	// To ensure users can still apply their KubeadmControlPlaneTemplate over pre-existing KubeadmControlPlaneTemplate
	// without setting rolloutStrategy we allow transitions from the old default value to unset.
	if reflect.DeepEqual(newK.Spec.Template.Spec.Rollout.Strategy, controlplanev1.KubeadmControlPlaneRolloutStrategy{}) &&
		oldK.Spec.Template.Spec.Rollout.Strategy.Type == controlplanev1.RollingUpdateStrategyType &&
		oldK.Spec.Template.Spec.Rollout.Strategy.RollingUpdate.MaxSurge != nil &&
		*oldK.Spec.Template.Spec.Rollout.Strategy.RollingUpdate.MaxSurge == intstr.FromInt32(1) {
		newK.Spec.Template.Spec.Rollout.Strategy = oldK.Spec.Template.Spec.Rollout.Strategy
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

	allErrs = append(allErrs, validateRolloutAndCertValidityFields(s.Rollout, s.KubeadmConfigSpec.ClusterConfiguration, nil, pathPrefix)...)
	allErrs = append(allErrs, validateNaming(s.MachineNaming, pathPrefix.Child("machineNaming"))...)

	// Validate the metadata of the MachineTemplate
	allErrs = append(allErrs, s.MachineTemplate.ObjectMeta.Validate(pathPrefix.Child("machineTemplate", "metadata"))...)

	return allErrs
}
