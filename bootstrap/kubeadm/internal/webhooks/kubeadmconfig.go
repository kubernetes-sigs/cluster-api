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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
)

func (webhook *KubeadmConfig) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &bootstrapv1.KubeadmConfig{}).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-bootstrap-cluster-x-k8s-io-v1beta2-kubeadmconfig,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigs,versions=v1beta2,name=validation.kubeadmconfig.bootstrap.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1

// KubeadmConfig implements a validation and defaulting webhook for KubeadmConfig.
type KubeadmConfig struct{}

var _ admission.Validator[*bootstrapv1.KubeadmConfig] = &KubeadmConfig{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *KubeadmConfig) ValidateCreate(_ context.Context, c *bootstrapv1.KubeadmConfig) (admission.Warnings, error) {
	return nil, webhook.validate(c.Spec, c.Name)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *KubeadmConfig) ValidateUpdate(_ context.Context, _, newC *bootstrapv1.KubeadmConfig) (admission.Warnings, error) {
	return nil, webhook.validate(newC.Spec, newC.Name)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *KubeadmConfig) ValidateDelete(_ context.Context, _ *bootstrapv1.KubeadmConfig) (admission.Warnings, error) {
	return nil, nil
}

func (webhook *KubeadmConfig) validate(c bootstrapv1.KubeadmConfigSpec, name string) error {
	allErrs := c.Validate(false, field.NewPath("spec"))

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(bootstrapv1.GroupVersion.WithKind("KubeadmConfig").GroupKind(), name, allErrs)
}
