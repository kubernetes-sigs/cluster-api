/*
Copyright 2024 The Kubernetes Authors.

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

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
)

// DevMachine implements a validating and defaulting webhook for DevMachine.
type DevMachine struct{}

func (webhook *DevMachine) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &infrav1.DevMachine{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-infrastructure-cluster-x-k8s-io-v1beta2-devmachine,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=devmachines,versions=v1beta2,name=default.devmachine.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ admission.Defaulter[*infrav1.DevMachine] = &DevMachine{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (webhook *DevMachine) Default(_ context.Context, machine *infrav1.DevMachine) error {
	defaultDevMachineSpec(&machine.Spec)
	return nil
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1beta2-devmachine,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=devmachines,versions=v1beta2,name=validation.devmachine.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ admission.Validator[*infrav1.DevMachine] = &DevMachine{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DevMachine) ValidateCreate(_ context.Context, _ *infrav1.DevMachine) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DevMachine) ValidateUpdate(_ context.Context, _, _ *infrav1.DevMachine) (admission.Warnings, error) {
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DevMachine) ValidateDelete(_ context.Context, _ *infrav1.DevMachine) (admission.Warnings, error) {
	return nil, nil
}

func defaultDevMachineSpec(s *infrav1.DevMachineSpec) {
	if s.Backend.InMemory == nil &&
		s.Backend.Docker == nil {
		s.Backend.Docker = &infrav1.DockerMachineBackendSpec{}
	}
}
