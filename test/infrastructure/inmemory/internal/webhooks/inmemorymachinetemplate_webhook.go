/*
Copyright 2023 The Kubernetes Authors.

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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/api/v1alpha1"
)

// InMemoryMachineTemplate implements a validating and defaulting webhook for InMemoryMachineTemplate.
type InMemoryMachineTemplate struct{}

func (webhook *InMemoryMachineTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&infrav1.InMemoryMachineTemplate{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-infrastructure-cluster-x-k8s-io-v1alpha1-inmemorymachinetemplate,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=inmemorymachinetemplates,versions=v1alpha1,name=default.inmemorymachinetemplate.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.CustomDefaulter = &InMemoryMachineTemplate{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (webhook *InMemoryMachineTemplate) Default(_ context.Context, _ runtime.Object) error {
	return nil
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1alpha1-inmemorymachinetemplate,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=inmemorymachinetemplates,versions=v1alpha1,name=validation.inmemorymachinetemplate.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.CustomValidator = &InMemoryMachineTemplate{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *InMemoryMachineTemplate) ValidateCreate(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *InMemoryMachineTemplate) ValidateUpdate(_ context.Context, _, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *InMemoryMachineTemplate) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
