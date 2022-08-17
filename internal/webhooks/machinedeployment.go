/*
Copyright 2022 The Kubernetes Authors.

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
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/version"
)

func (webhook *MachineDeployment) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&clusterv1.MachineDeployment{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-cluster-x-k8s-io-v1beta1-machinedeployment,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machinedeployments,versions=v1beta1,name=validation.machinedeployment.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1beta1-machinedeployment,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machinedeployments,versions=v1beta1,name=default.machinedeployment.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// MachineDeployment implements a validating and defaulting webhook for MachineDeployment.
type MachineDeployment struct {
	Client client.Reader
}

var _ webhook.CustomDefaulter = &MachineDeployment{}
var _ webhook.CustomValidator = &MachineDeployment{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type.
func (webhook *MachineDeployment) Default(_ context.Context, obj runtime.Object) error {
	machineDeployment, ok := obj.(*clusterv1.MachineDeployment)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a MachineDeployment but got a %T", obj))
	}
	PopulateDefaultsMachineDeployment(machineDeployment)
	// tolerate version strings without a "v" prefix: prepend it if it's not there
	if machineDeployment.Spec.Template.Spec.Version != nil && !strings.HasPrefix(*machineDeployment.Spec.Template.Spec.Version, "v") {
		normalizedVersion := "v" + *machineDeployment.Spec.Template.Spec.Version
		machineDeployment.Spec.Template.Spec.Version = &normalizedVersion
	}
	return nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *MachineDeployment) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	machineDeployment, ok := obj.(*clusterv1.MachineDeployment)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a MachineDeployment but got a %T", obj))
	}
	return webhook.validate(ctx, nil, machineDeployment)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *MachineDeployment) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	newMD, ok := newObj.(*clusterv1.MachineDeployment)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a MachineDeployment but got a %T", newObj))
	}
	oldMD, ok := oldObj.(*clusterv1.MachineDeployment)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a MachineDeployment but got a %T", oldObj))
	}
	return webhook.validate(ctx, oldMD, newMD)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *MachineDeployment) ValidateDelete(_ context.Context, _ runtime.Object) error {
	return nil
}

func (webhook *MachineDeployment) validate(_ context.Context, oldMachineDeployment, newMachineDeployment *clusterv1.MachineDeployment) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")
	selector, err := metav1.LabelSelectorAsSelector(&newMachineDeployment.Spec.Selector)
	if err != nil {
		allErrs = append(
			allErrs,
			field.Invalid(specPath.Child("selector"), newMachineDeployment.Spec.Selector, err.Error()),
		)
	} else if !selector.Matches(labels.Set(newMachineDeployment.Spec.Template.Labels)) {
		allErrs = append(
			allErrs,
			field.Forbidden(
				specPath.Child("template", "metadata", "labels"),
				fmt.Sprintf("must match spec.selector %q", selector.String()),
			),
		)
	}

	if oldMachineDeployment != nil {
		if oldMachineDeployment.Spec.ClusterName != newMachineDeployment.Spec.ClusterName {
			allErrs = append(
				allErrs,
				field.Forbidden(
					specPath.Child("clusterName"),
					"field is immutable",
				),
			)
		}
	}

	if newMachineDeployment.Spec.Strategy != nil && newMachineDeployment.Spec.Strategy.RollingUpdate != nil {
		total := 1
		if newMachineDeployment.Spec.Replicas != nil {
			total = int(*newMachineDeployment.Spec.Replicas)
		}

		if newMachineDeployment.Spec.Strategy.RollingUpdate.MaxSurge != nil {
			if _, err := intstr.GetScaledValueFromIntOrPercent(newMachineDeployment.Spec.Strategy.RollingUpdate.MaxSurge, total, true); err != nil {
				allErrs = append(
					allErrs,
					field.Invalid(specPath.Child("strategy", "rollingUpdate", "maxSurge"),
						newMachineDeployment.Spec.Strategy.RollingUpdate.MaxSurge, fmt.Sprintf("must be either an int or a percentage: %v", err.Error())),
				)
			}
		}

		if newMachineDeployment.Spec.Strategy.RollingUpdate.MaxUnavailable != nil {
			if _, err := intstr.GetScaledValueFromIntOrPercent(newMachineDeployment.Spec.Strategy.RollingUpdate.MaxUnavailable, total, true); err != nil {
				allErrs = append(
					allErrs,
					field.Invalid(specPath.Child("strategy", "rollingUpdate", "maxUnavailable"),
						newMachineDeployment.Spec.Strategy.RollingUpdate.MaxUnavailable, fmt.Sprintf("must be either an int or a percentage: %v", err.Error())),
				)
			}
		}
	}

	if newMachineDeployment.Spec.Template.Spec.Version != nil {
		if !version.KubeSemver.MatchString(*newMachineDeployment.Spec.Template.Spec.Version) {
			allErrs = append(allErrs, field.Invalid(specPath.Child("template", "spec", "version"), *newMachineDeployment.Spec.Template.Spec.Version, "must be a valid semantic version"))
		}
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("MachineDeployment").GroupKind(), newMachineDeployment.Name, allErrs)
}

// PopulateDefaultsMachineDeployment fills in default field values.
// This is also called during MachineDeployment sync.
func PopulateDefaultsMachineDeployment(d *clusterv1.MachineDeployment) {
	if d.Labels == nil {
		d.Labels = make(map[string]string)
	}
	d.Labels[clusterv1.ClusterLabelName] = d.Spec.ClusterName

	if d.Spec.MinReadySeconds == nil {
		d.Spec.MinReadySeconds = pointer.Int32Ptr(0)
	}

	if d.Spec.RevisionHistoryLimit == nil {
		d.Spec.RevisionHistoryLimit = pointer.Int32Ptr(1)
	}

	if d.Spec.ProgressDeadlineSeconds == nil {
		d.Spec.ProgressDeadlineSeconds = pointer.Int32Ptr(600)
	}

	if d.Spec.Selector.MatchLabels == nil {
		d.Spec.Selector.MatchLabels = make(map[string]string)
	}

	if d.Spec.Strategy == nil {
		d.Spec.Strategy = &clusterv1.MachineDeploymentStrategy{}
	}

	if d.Spec.Strategy.Type == "" {
		d.Spec.Strategy.Type = clusterv1.RollingUpdateMachineDeploymentStrategyType
	}

	if d.Spec.Template.Labels == nil {
		d.Spec.Template.Labels = make(map[string]string)
	}

	// Default RollingUpdate strategy only if strategy type is RollingUpdate.
	if d.Spec.Strategy.Type == clusterv1.RollingUpdateMachineDeploymentStrategyType {
		if d.Spec.Strategy.RollingUpdate == nil {
			d.Spec.Strategy.RollingUpdate = &clusterv1.MachineRollingUpdateDeployment{}
		}
		if d.Spec.Strategy.RollingUpdate.MaxSurge == nil {
			ios1 := intstr.FromInt(1)
			d.Spec.Strategy.RollingUpdate.MaxSurge = &ios1
		}
		if d.Spec.Strategy.RollingUpdate.MaxUnavailable == nil {
			ios0 := intstr.FromInt(0)
			d.Spec.Strategy.RollingUpdate.MaxUnavailable = &ios0
		}
	}

	// If no selector has been provided, add label and selector for the
	// MachineDeployment's name as a default way of providing uniqueness.
	if len(d.Spec.Selector.MatchLabels) == 0 && len(d.Spec.Selector.MatchExpressions) == 0 {
		d.Spec.Selector.MatchLabels[clusterv1.MachineDeploymentLabelName] = d.Name
		d.Spec.Template.Labels[clusterv1.MachineDeploymentLabelName] = d.Name
	}
	// Make sure selector and template to be in the same cluster.
	d.Spec.Selector.MatchLabels[clusterv1.ClusterLabelName] = d.Spec.ClusterName
	d.Spec.Template.Labels[clusterv1.ClusterLabelName] = d.Spec.ClusterName
}
