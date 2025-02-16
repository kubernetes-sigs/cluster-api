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

	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/util/compare"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
)

// DockerClusterTemplate implements a validating and defaulting webhook for DockerClusterTemplate.
type DockerClusterTemplate struct{}

func (webhook *DockerClusterTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&infrav1.DockerClusterTemplate{}).
		WithDefaulter(webhook, admission.DefaulterRemoveUnknownOrOmitableFields).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-infrastructure-cluster-x-k8s-io-v1beta1-dockerclustertemplate,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=dockerclustertemplates,versions=v1beta1,name=default.dockerclustertemplate.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.CustomDefaulter = &DockerClusterTemplate{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (webhook *DockerClusterTemplate) Default(_ context.Context, obj runtime.Object) error {
	clusterTemplate, ok := obj.(*infrav1.DockerClusterTemplate)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a DockerClusterTemplate but got a %T", obj))
	}
	defaultDockerClusterSpec(&clusterTemplate.Spec.Template.Spec)
	return nil
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1beta1-dockerclustertemplate,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=dockerclustertemplates,versions=v1beta1,name=validation.dockerclustertemplate.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.CustomValidator = &DockerClusterTemplate{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DockerClusterTemplate) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	// NOTE: DockerClusterTemplate is behind ClusterTopology feature gate flag; the web hook
	// must prevent creating new objects in case the feature flag is disabled.
	if !feature.Gates.Enabled(feature.ClusterTopology) {
		return nil, field.Forbidden(
			field.NewPath("spec"),
			"can be set only if the ClusterTopology feature flag is enabled",
		)
	}

	clusterTemplate, ok := obj.(*infrav1.DockerClusterTemplate)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a DockerClusterTemplate but got a %T", obj))
	}

	allErrs := validateDockerClusterSpec(clusterTemplate.Spec.Template.Spec)

	// Validate the metadata of the template.
	allErrs = append(allErrs, clusterTemplate.Spec.Template.ObjectMeta.Validate(field.NewPath("spec", "template", "metadata"))...)

	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(infrav1.GroupVersion.WithKind("DockerClusterTemplate").GroupKind(), clusterTemplate.Name, allErrs)
	}
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DockerClusterTemplate) ValidateUpdate(ctx context.Context, oldRaw, newRaw runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList
	oldTemplate, ok := oldRaw.(*infrav1.DockerClusterTemplate)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a DockerClusterTemplate but got a %T", oldRaw))
	}
	newTemplate, ok := newRaw.(*infrav1.DockerClusterTemplate)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a DockerClusterTemplate but got a %T", newRaw))
	}

	if err := webhook.Default(ctx, oldTemplate); err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("failed to compare old and new DockerClusterTemplate: failed to default old object: %v", err))
	}
	if err := webhook.Default(ctx, newTemplate); err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("failed to compare old and new DockerClusterTemplate: failed to default new object: %v", err))
	}

	equal, diff, err := compare.Diff(oldTemplate.Spec.Template.Spec, newTemplate.Spec.Template.Spec)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("failed to compare old and new DockerClusterTemplate: %v", err))
	}
	if !equal {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "template", "spec"), newTemplate, fmt.Sprintf("DockerClusterTemplate spec.template.spec field is immutable. Please create a new resource instead. Diff: %s", diff)),
		)
	}

	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(infrav1.GroupVersion.WithKind("DockerClusterTemplate").GroupKind(), newTemplate.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DockerClusterTemplate) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
