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
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

var (
	// DefaultNodeStartupTimeout is the time allowed for a node to start up.
	// Can be made longer as part of spec if required for particular provider.
	// 10 minutes should allow the instance to start and the node to join the
	// cluster on most providers.
	DefaultNodeStartupTimeout = metav1.Duration{Duration: 10 * time.Minute}
	// Minimum time allowed for a node to start up.
	minNodeStartupTimeout = metav1.Duration{Duration: 30 * time.Second}
	// We allow users to disable the nodeStartupTimeout by setting the duration to 0.
	disabledNodeStartupTimeout = clusterv1.ZeroDuration
)

// SetMinNodeStartupTimeout allows users to optionally set a custom timeout
// for the validation webhook.
//
// This function is mostly used within envtest (integration tests), and should
// never be used in a production environment.
func SetMinNodeStartupTimeout(d metav1.Duration) {
	minNodeStartupTimeout = d
}

func (webhook *MachineHealthCheck) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&clusterv1.MachineHealthCheck{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-cluster-x-k8s-io-v1beta1-machinehealthcheck,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machinehealthchecks,versions=v1beta1,name=validation.machinehealthcheck.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1beta1-machinehealthcheck,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machinehealthchecks,versions=v1beta1,name=default.machinehealthcheck.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// MachineHealthCheck implements a validating and defaulting webhook for MachineHealthCheck.
type MachineHealthCheck struct {
	Client client.Reader
}

var _ webhook.CustomDefaulter = &MachineHealthCheck{}
var _ webhook.CustomValidator = &MachineHealthCheck{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type.
func (webhook *MachineHealthCheck) Default(_ context.Context, obj runtime.Object) error {
	mhc, ok := obj.(*clusterv1.MachineHealthCheck)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a MachineHealthCheck but got a %T", obj))
	}
	if mhc.Labels == nil {
		mhc.Labels = make(map[string]string)
	}
	mhc.Labels[clusterv1.ClusterLabelName] = mhc.Spec.ClusterName

	if mhc.Spec.MaxUnhealthy == nil {
		defaultMaxUnhealthy := intstr.FromString("100%")
		mhc.Spec.MaxUnhealthy = &defaultMaxUnhealthy
	}

	if mhc.Spec.NodeStartupTimeout == nil {
		mhc.Spec.NodeStartupTimeout = &DefaultNodeStartupTimeout
	}

	if mhc.Spec.RemediationTemplate != nil && mhc.Spec.RemediationTemplate.Namespace == "" {
		mhc.Spec.RemediationTemplate.Namespace = mhc.Namespace
	}
	return nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *MachineHealthCheck) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	mhc, ok := obj.(*clusterv1.MachineHealthCheck)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a MachineHealthCheck but got a %T", obj))
	}
	return webhook.validate(ctx, nil, mhc)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *MachineHealthCheck) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	newMHC, ok := oldObj.(*clusterv1.MachineHealthCheck)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a MachineHealthCheck but got a %T", newObj))
	}
	oldMHC, ok := newObj.(*clusterv1.MachineHealthCheck)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a MachineHealthCheck but got a %T", oldObj))
	}
	return webhook.validate(ctx, oldMHC, newMHC)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *MachineHealthCheck) ValidateDelete(_ context.Context, _ runtime.Object) error {
	return nil
}

func (webhook *MachineHealthCheck) validate(_ context.Context, oldMachineHealthCheck, newMachineHealthCheck *clusterv1.MachineHealthCheck) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	// Validate selector parses as Selector
	selector, err := metav1.LabelSelectorAsSelector(&newMachineHealthCheck.Spec.Selector)
	if err != nil {
		allErrs = append(
			allErrs,
			field.Invalid(specPath.Child("selector"), newMachineHealthCheck.Spec.Selector, err.Error()),
		)
	}

	// Validate that the selector isn't empty.
	if selector != nil && selector.Empty() {
		allErrs = append(
			allErrs,
			field.Required(specPath.Child("selector"), "selector must not be empty"),
		)
	}

	if clusterName, ok := newMachineHealthCheck.Spec.Selector.MatchLabels[clusterv1.ClusterLabelName]; ok && clusterName != newMachineHealthCheck.Spec.ClusterName {
		allErrs = append(
			allErrs,
			field.Invalid(specPath.Child("selector"), newMachineHealthCheck.Spec.Selector, "cannot specify a cluster selector other than the one specified by ClusterName"))
	}

	if oldMachineHealthCheck != nil {
		if oldMachineHealthCheck.Spec.ClusterName != newMachineHealthCheck.Spec.ClusterName {
			allErrs = append(
				allErrs,
				field.Forbidden(specPath.Child("clusterName"), "field is immutable"),
			)
		}
	}

	if newMachineHealthCheck.Spec.NodeStartupTimeout != nil &&
		newMachineHealthCheck.Spec.NodeStartupTimeout.Seconds() != disabledNodeStartupTimeout.Seconds() &&
		newMachineHealthCheck.Spec.NodeStartupTimeout.Seconds() < minNodeStartupTimeout.Seconds() {
		allErrs = append(
			allErrs,
			field.Invalid(specPath.Child("nodeStartupTimeout"), newMachineHealthCheck.Spec.NodeStartupTimeout.Seconds(), "must be at least 30s"),
		)
	}

	if newMachineHealthCheck.Spec.MaxUnhealthy != nil {
		if _, err := intstr.GetScaledValueFromIntOrPercent(newMachineHealthCheck.Spec.MaxUnhealthy, 0, false); err != nil {
			allErrs = append(
				allErrs,
				field.Invalid(specPath.Child("maxUnhealthy"), newMachineHealthCheck.Spec.MaxUnhealthy, fmt.Sprintf("must be either an int or a percentage: %v", err.Error())),
			)
		}
	}

	if newMachineHealthCheck.Spec.RemediationTemplate != nil && newMachineHealthCheck.Spec.RemediationTemplate.Namespace != newMachineHealthCheck.Namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				specPath.Child("remediationTemplate", "namespace"),
				newMachineHealthCheck.Spec.RemediationTemplate.Namespace,
				"must match metadata.namespace",
			),
		)
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("MachineHealthCheck").GroupKind(), newMachineHealthCheck.Name, allErrs)
}
