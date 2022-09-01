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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// Deprecated: This file, including all public and private methods, will be removed in a future release.
// The KubeadmConfig webhook validation implementation and API can now be found in the webhooks package.

// SetupWebhookWithManager sets up KubeadmConfigTemplate webhooks.
// Deprecated: This method is going to be removed in a next release.
// Note: We're not using this method anymore and are using webhooks.KubeadmConfigTemplate.SetupWebhookWithManager instead.
// Note: We don't have to call this func for the conversion webhook as there is only a single conversion webhook instance
// for all resources and we already register it through other types.
func (r *KubeadmConfigTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

var _ webhook.Defaulter = &KubeadmConfigTemplate{}
var _ webhook.Validator = &KubeadmConfigTemplate{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
// Deprecated: This method is going to be removed in a next release.
// Note: We're not using this method anymore and are using webhooks.KubeadmConfigTemplate.Default instead.
func (r *KubeadmConfigTemplate) Default() {
	DefaultKubeadmConfigSpec(&r.Spec.Template.Spec)
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
// Deprecated: This method is going to be removed in a next release.
// Note: We're not using this method anymore and are using webhooks.KubeadmConfigTemplate.ValidateCreate instead.
func (r *KubeadmConfigTemplate) ValidateCreate() error {
	return r.Spec.validate(r.Name)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
// Deprecated: This method is going to be removed in a next release.
// Note: We're not using this method anymore and are using webhooks.KubeadmConfigTemplate.ValidateUpdate instead.
func (r *KubeadmConfigTemplate) ValidateUpdate(old runtime.Object) error {
	return r.Spec.validate(r.Name)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
// Deprecated: This method is going to be removed in a next release.
// Note: We're not using this method anymore and are using webhooks.KubeadmConfigTemplate.ValidateDelete instead.
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
