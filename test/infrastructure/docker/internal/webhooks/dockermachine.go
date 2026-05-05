/*
Copyright 2026 The Kubernetes Authors.

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

	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
)

// DockerMachine implements a custom validation webhook for DockerMachine.
type DockerMachine struct{}

func (webhook *DockerMachine) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &infrav1.DockerMachine{}).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1beta2-dockermachine,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=dockermachines,versions=v1beta2,name=validation.dockermachine.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1

var _ admission.Validator[*infrav1.DockerMachine] = &DockerMachine{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DockerMachine) ValidateCreate(_ context.Context, _ *infrav1.DockerMachine) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DockerMachine) ValidateUpdate(_ context.Context, oldObj, newObj *infrav1.DockerMachine) (admission.Warnings, error) {
	var allErrs field.ErrorList

	if oldObj.Spec.CustomImage != newObj.Spec.CustomImage {
		allErrs = append(allErrs, field.Forbidden(field.NewPath("spec", "customImage"), "cannot be modified"))
	}

	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(infrav1.GroupVersion.WithKind("DockerMachine").GroupKind(), newObj.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DockerMachine) ValidateDelete(_ context.Context, _ *infrav1.DockerMachine) (admission.Warnings, error) {
	return nil, nil
}
