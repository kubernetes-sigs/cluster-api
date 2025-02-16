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

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
)

func (webhook *KubeadmConfig) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&bootstrapv1.KubeadmConfig{}).
		WithDefaulter(webhook, admission.DefaulterRemoveUnknownOrOmitableFields).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-bootstrap-cluster-x-k8s-io-v1beta1-kubeadmconfig,mutating=true,failurePolicy=fail,groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigs,versions=v1beta1,name=default.kubeadmconfig.bootstrap.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/validate-bootstrap-cluster-x-k8s-io-v1beta1-kubeadmconfig,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigs,versions=v1beta1,name=validation.kubeadmconfig.bootstrap.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// KubeadmConfig implements a validation and defaulting webhook for KubeadmConfig.
type KubeadmConfig struct{}

var _ webhook.CustomValidator = &KubeadmConfig{}
var _ webhook.CustomDefaulter = &KubeadmConfig{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (webhook *KubeadmConfig) Default(_ context.Context, obj runtime.Object) error {
	c, ok := obj.(*bootstrapv1.KubeadmConfig)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmConfig but got a %T", obj))
	}

	c.Spec.Default()

	return nil
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *KubeadmConfig) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	c, ok := obj.(*bootstrapv1.KubeadmConfig)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmConfig but got a %T", obj))
	}

	return nil, webhook.validate(c.Spec, c.Name)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *KubeadmConfig) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	newC, ok := newObj.(*bootstrapv1.KubeadmConfig)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmConfig but got a %T", newObj))
	}

	return nil, webhook.validate(newC.Spec, newC.Name)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *KubeadmConfig) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (webhook *KubeadmConfig) validate(c bootstrapv1.KubeadmConfigSpec, name string) error {
	allErrs := c.Validate(field.NewPath("spec"))

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(bootstrapv1.GroupVersion.WithKind("KubeadmConfig").GroupKind(), name, allErrs)
}
