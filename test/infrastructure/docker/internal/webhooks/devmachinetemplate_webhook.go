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
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"sigs.k8s.io/cluster-api/internal/util/compare"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
	"sigs.k8s.io/cluster-api/util/topology"
)

// DevMachineTemplate implements a custom validation and defaulting webhook for DevMachineTemplate.
// +kubebuilder:object:generate=false
type DevMachineTemplate struct{}

func (webhook *DevMachineTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&infrav1.DevMachineTemplate{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-infrastructure-cluster-x-k8s-io-v1beta2-devmachinetemplate,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=devmachinetemplates,versions=v1beta2,name=default.devmachinetemplate.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta2

var _ webhook.CustomDefaulter = &DevMachineTemplate{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (webhook *DevMachineTemplate) Default(_ context.Context, obj runtime.Object) error {
	machineTemplate, ok := obj.(*infrav1.DevMachineTemplate)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a DevMachineTemplate but got a %T", obj))
	}
	defaultDevMachineSpec(&machineTemplate.Spec.Template.Spec)
	return nil
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1beta2-devmachinetemplate,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=devmachinetemplates,versions=v1beta2,name=validation.devmachinetemplate.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta2

var _ webhook.CustomValidator = &DevMachineTemplate{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DevMachineTemplate) ValidateCreate(_ context.Context, raw runtime.Object) (admission.Warnings, error) {
	obj, ok := raw.(*infrav1.DevMachineTemplate)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a DevMachineTemplate but got a %T", raw))
	}
	// Validate the metadata of the template.
	allErrs := obj.Spec.Template.ObjectMeta.Validate(field.NewPath("spec", "template", "metadata"))
	if len(allErrs) > 0 {
		return nil, apierrors.NewInvalid(infrav1.GroupVersion.WithKind("DevMachineTemplate").GroupKind(), obj.Name, allErrs)
	}
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DevMachineTemplate) ValidateUpdate(ctx context.Context, oldRaw runtime.Object, newRaw runtime.Object) (admission.Warnings, error) {
	newObj, ok := newRaw.(*infrav1.DevMachineTemplate)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a DevMachineTemplate but got a %T", newRaw))
	}
	oldObj, ok := oldRaw.(*infrav1.DevMachineTemplate)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a DevMachineTemplate but got a %T", oldRaw))
	}

	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a admission.Request inside context: %v", err))
	}

	var allErrs field.ErrorList
	if !topology.ShouldSkipImmutabilityChecks(req, newObj) {
		equal, diff, err := compare.Diff(oldObj.Spec.Template.Spec, newObj.Spec.Template.Spec)
		if err != nil {
			return nil, apierrors.NewBadRequest(fmt.Sprintf("failed to compare old and new DevMachineTemplate: %v", err))
		}
		if !equal {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec", "template", "spec"), newObj, fmt.Sprintf("DevMachineTemplate spec.template.spec field is immutable. Please create a new resource instead. Diff: %s", diff)),
			)
		}
	}

	// Validate the metadata of the template.
	allErrs = append(allErrs, newObj.Spec.Template.ObjectMeta.Validate(field.NewPath("spec", "template", "metadata"))...)

	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(infrav1.GroupVersion.WithKind("DevMachineTemplate").GroupKind(), newObj.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *DevMachineTemplate) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
