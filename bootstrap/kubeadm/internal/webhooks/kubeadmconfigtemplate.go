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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
)

func (webhook *KubeadmConfigTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&bootstrapv1.KubeadmConfigTemplate{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-bootstrap-cluster-x-k8s-io-v1beta1-kubeadmconfigtemplate,mutating=true,failurePolicy=fail,groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigtemplates,versions=v1beta1,name=default.kubeadmconfigtemplate.bootstrap.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/validate-bootstrap-cluster-x-k8s-io-v1beta1-kubeadmconfigtemplate,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigtemplates,versions=v1beta1,name=validation.kubeadmconfigtemplate.bootstrap.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// KubeadmConfigTemplate implements a validating and defaulting webhook for KubeadmConfigTemplate.
type KubeadmConfigTemplate struct {
	Client client.Reader
}

var _ webhook.CustomDefaulter = &KubeadmConfigTemplate{}
var _ webhook.CustomValidator = &KubeadmConfigTemplate{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type.
func (webhook *KubeadmConfigTemplate) Default(_ context.Context, obj runtime.Object) error {
	tpl, ok := obj.(*bootstrapv1.KubeadmConfigTemplate)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmConfigTemplate but got a %T", obj))
	}
	bootstrapv1.DefaultKubeadmConfigSpec(&tpl.Spec.Template.Spec)

	return nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *KubeadmConfigTemplate) ValidateCreate(_ context.Context, obj runtime.Object) error {
	tpl, ok := obj.(*bootstrapv1.KubeadmConfigTemplate)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmConfigTemplate but got a %T", obj))
	}
	return webhook.validate(tpl)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *KubeadmConfigTemplate) ValidateUpdate(_ context.Context, _, newObj runtime.Object) error {
	tpl, ok := newObj.(*bootstrapv1.KubeadmConfigTemplate)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmConfigTemplate but got a %T", newObj))
	}
	return webhook.validate(tpl)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *KubeadmConfigTemplate) ValidateDelete(_ context.Context, _ runtime.Object) error {
	return nil
}

func (webhook *KubeadmConfigTemplate) validate(tpl *bootstrapv1.KubeadmConfigTemplate) error {
	var allErrs field.ErrorList

	allErrs = append(allErrs, ValidateKubeadmConfigSpec(&tpl.Spec.Template.Spec, field.NewPath("spec", "template", "spec"))...)

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(bootstrapv1.GroupVersion.WithKind("KubeadmConfigTemplate").GroupKind(), tpl.Name, allErrs)
}
