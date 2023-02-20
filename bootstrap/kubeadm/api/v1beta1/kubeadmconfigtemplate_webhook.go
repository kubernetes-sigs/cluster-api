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

package v1beta1

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func (r *KubeadmConfigTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithDefaulter(&KubeadmConfigTemplateWebhook{}).
		Complete()
}

// KubeadmConfigTemplateWebhook implements a custom defaulting webhook for KubeadmConfigTemplate.
// +kubebuilder:object:generate=false
type KubeadmConfigTemplateWebhook struct{}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-bootstrap-cluster-x-k8s-io-v1beta1-kubeadmconfigtemplate,mutating=true,failurePolicy=fail,groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigtemplates,versions=v1beta1,name=default.kubeadmconfigtemplate.bootstrap.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.CustomDefaulter = &KubeadmConfigTemplateWebhook{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (k KubeadmConfigTemplateWebhook) Default(ctx context.Context, raw runtime.Object) error {
	obj, ok := raw.(*KubeadmConfigTemplate)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmConfigTemplate but got a %T", obj))
	}

	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a admission.Request inside context: %v", err))
	}

	if req.UserInfo.Username != "kubernetes-admin" {
		DefaultKubeadmConfigSpec(&obj.Spec.Template.Spec)
	}

	return nil
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-bootstrap-cluster-x-k8s-io-v1beta1-kubeadmconfigtemplate,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigtemplates,versions=v1beta1,name=validation.kubeadmconfigtemplate.bootstrap.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.Validator = &KubeadmConfigTemplate{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (r *KubeadmConfigTemplate) ValidateCreate() error {
	return r.Spec.validate(r.Name)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (r *KubeadmConfigTemplate) ValidateUpdate(_ runtime.Object) error {
	return r.Spec.validate(r.Name)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (r *KubeadmConfigTemplate) ValidateDelete() error {
	return nil
}

func (r *KubeadmConfigTemplateSpec) validate(name string) error {
	var allErrs field.ErrorList

	allErrs = append(allErrs, r.Template.Spec.Validate(field.NewPath("spec", "template", "spec"))...)

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(GroupVersion.WithKind("KubeadmConfigTemplate").GroupKind(), name, allErrs)
}
