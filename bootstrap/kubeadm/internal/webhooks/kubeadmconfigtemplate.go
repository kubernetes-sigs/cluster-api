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
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
)

func (webhook *KubeadmConfigTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&bootstrapv1.KubeadmConfigTemplate{}).
		WithDefaulter(webhook, admission.DefaulterRemoveUnknownOrOmitableFields).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-bootstrap-cluster-x-k8s-io-v1beta1-kubeadmconfigtemplate,mutating=true,failurePolicy=fail,groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigtemplates,versions=v1beta1,name=default.kubeadmconfigtemplate.bootstrap.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/validate-bootstrap-cluster-x-k8s-io-v1beta1-kubeadmconfigtemplate,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigtemplates,versions=v1beta1,name=validation.kubeadmconfigtemplate.bootstrap.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// KubeadmConfigTemplate implements a validation and defaulting webhook for KubeadmConfigTemplate.
type KubeadmConfigTemplate struct{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (webhook *KubeadmConfigTemplate) Default(_ context.Context, obj runtime.Object) error {
	c, ok := obj.(*bootstrapv1.KubeadmConfigTemplate)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmConfigTemplate but got a %T", obj))
	}

	c.Spec.Template.Spec.Default()

	return nil
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *KubeadmConfigTemplate) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	c, ok := obj.(*bootstrapv1.KubeadmConfigTemplate)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmConfigTemplate but got a %T", obj))
	}

	return nil, webhook.validate(&c.Spec, c.Name)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *KubeadmConfigTemplate) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	newC, ok := newObj.(*bootstrapv1.KubeadmConfigTemplate)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmConfigTemplate but got a %T", newObj))
	}

	return nil, webhook.validate(&newC.Spec, newC.Name)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *KubeadmConfigTemplate) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (webhook *KubeadmConfigTemplate) validate(r *bootstrapv1.KubeadmConfigTemplateSpec, name string) error {
	var allErrs field.ErrorList

	allErrs = append(allErrs, r.Template.Spec.Validate(field.NewPath("spec", "template", "spec"))...)
	// Validate the metadata of the template.
	allErrs = append(allErrs, r.Template.ObjectMeta.Validate(field.NewPath("spec", "template", "metadata"))...)

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(bootstrapv1.GroupVersion.WithKind("KubeadmConfigTemplate").GroupKind(), name, allErrs)
}
