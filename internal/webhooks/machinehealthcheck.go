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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

var (
	// Minimum time allowed for a node to start up.
	minNodeStartupTimeoutSeconds = int32(30)
	// We allow users to disable the nodeStartupTimeout by setting the duration to 0.
	disabledNodeStartupTimeoutSeconds = int32(0)
)

// SetMinNodeStartupTimeoutSeconds allows users to optionally set a custom timeout
// for the validation webhook.
//
// This function is mostly used within envtest (integration tests), and should
// never be used in a production environment.
func SetMinNodeStartupTimeoutSeconds(d int32) {
	minNodeStartupTimeoutSeconds = d
}

func (webhook *MachineHealthCheck) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&clusterv1.MachineHealthCheck{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-cluster-x-k8s-io-v1beta2-machinehealthcheck,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machinehealthchecks,versions=v1beta2,name=validation.machinehealthcheck.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1beta2-machinehealthcheck,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machinehealthchecks,versions=v1beta2,name=default.machinehealthcheck.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// MachineHealthCheck implements a validation and defaulting webhook for MachineHealthCheck.
type MachineHealthCheck struct{}

var _ webhook.CustomDefaulter = &MachineHealthCheck{}
var _ webhook.CustomValidator = &MachineHealthCheck{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type.
func (webhook *MachineHealthCheck) Default(_ context.Context, obj runtime.Object) error {
	m, ok := obj.(*clusterv1.MachineHealthCheck)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a MachineHealthCheck but got a %T", obj))
	}

	if m.Labels == nil {
		m.Labels = make(map[string]string)
	}
	m.Labels[clusterv1.ClusterNameLabel] = m.Spec.ClusterName

	if m.Spec.Checks.NodeStartupTimeoutSeconds == nil {
		m.Spec.Checks.NodeStartupTimeoutSeconds = &clusterv1.DefaultNodeStartupTimeoutSeconds
	}

	return nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *MachineHealthCheck) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	m, ok := obj.(*clusterv1.MachineHealthCheck)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a MachineHealthCheck but got a %T", obj))
	}

	return nil, webhook.validate(nil, m)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *MachineHealthCheck) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldM, ok := oldObj.(*clusterv1.MachineHealthCheck)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a MachineHealthCheck but got a %T", oldObj))
	}
	newM, ok := newObj.(*clusterv1.MachineHealthCheck)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a MachineHealthCheck but got a %T", newObj))
	}

	return nil, webhook.validate(oldM, newM)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *MachineHealthCheck) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (webhook *MachineHealthCheck) validate(oldMHC, newMHC *clusterv1.MachineHealthCheck) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	// Validate selector parses as Selector
	selector, err := metav1.LabelSelectorAsSelector(&newMHC.Spec.Selector)
	if err != nil {
		allErrs = append(
			allErrs,
			field.Invalid(specPath.Child("selector"), newMHC.Spec.Selector, err.Error()),
		)
	}

	// Validate that the selector isn't empty.
	if selector != nil && selector.Empty() {
		allErrs = append(
			allErrs,
			field.Required(specPath.Child("selector"), "selector must not be empty"),
		)
	}

	if clusterName, ok := newMHC.Spec.Selector.MatchLabels[clusterv1.ClusterNameLabel]; ok && clusterName != newMHC.Spec.ClusterName {
		allErrs = append(
			allErrs,
			field.Invalid(specPath.Child("selector"), newMHC.Spec.Selector, "cannot specify a cluster selector other than the one specified by ClusterName"))
	}

	if oldMHC != nil && oldMHC.Spec.ClusterName != newMHC.Spec.ClusterName {
		allErrs = append(
			allErrs,
			field.Forbidden(specPath.Child("clusterName"), "field is immutable"),
		)
	}

	allErrs = append(allErrs, validateMachineHealthCheckNodeStartupTimeoutSeconds(specPath, newMHC.Spec.Checks.NodeStartupTimeoutSeconds)...)
	allErrs = append(allErrs, validateMachineHealthCheckUnhealthyLessThanOrEqualTo(specPath, newMHC.Spec.Remediation.TriggerIf.UnhealthyLessThanOrEqualTo)...)

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("MachineHealthCheck").GroupKind(), newMHC.Name, allErrs)
}

func validateMachineHealthCheckNodeStartupTimeoutSeconds(fldPath *field.Path, nodeStartupTimeoutSeconds *int32) field.ErrorList {
	var allErrs field.ErrorList
	if nodeStartupTimeoutSeconds != nil &&
		*nodeStartupTimeoutSeconds != disabledNodeStartupTimeoutSeconds &&
		*nodeStartupTimeoutSeconds < minNodeStartupTimeoutSeconds {
		allErrs = append(
			allErrs,
			field.Invalid(fldPath.Child("checks", "nodeStartupTimeoutSeconds"), *nodeStartupTimeoutSeconds, "must be at least 30s"),
		)
	}
	return allErrs
}

func validateMachineHealthCheckUnhealthyLessThanOrEqualTo(fldPath *field.Path, unhealthyLessThanOrEqualTo *intstr.IntOrString) field.ErrorList {
	var allErrs field.ErrorList
	if unhealthyLessThanOrEqualTo != nil {
		// Note: total and roundUp parameters don't matter for validation.
		if _, err := intstr.GetScaledValueFromIntOrPercent(unhealthyLessThanOrEqualTo, 0, false); err != nil {
			allErrs = append(
				allErrs,
				field.Invalid(fldPath.Child("remediation", "triggerIf", "unhealthyLessThanOrEqualTo"), unhealthyLessThanOrEqualTo.String(), fmt.Sprintf("must be either an int or a percentage: %v", err.Error())),
			)
		}
	}
	return allErrs
}
