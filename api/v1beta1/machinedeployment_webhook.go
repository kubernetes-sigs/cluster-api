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

package v1beta1

import (
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
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"sigs.k8s.io/cluster-api/util/version"
)

// Deprecated: This file, including all public and private methods, will be removed in a future release.
// The MachineDeployment webhook validation implementation and API can now be found in the webhooks package.

// SetupWebhookWithManager sets up MachineDeployment webhooks.
// Deprecated: This method is going to be removed in a next release.
// Note: We're not using this method anymore and are using webhooks.MachineDeployment.SetupWebhookWithManager instead.
// Note: We don't have to call this func for the conversion webhook as there is only a single conversion webhook instance
// for all resources and we already register it through other types.
func (m *MachineDeployment) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(m).
		Complete()
}

var _ webhook.Defaulter = &MachineDeployment{}
var _ webhook.Validator = &MachineDeployment{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
// Deprecated: This method is going to be removed in a next release.
// Note: We're not using this method anymore and are using webhooks.MachineDeployment.Default instead.
func (m *MachineDeployment) Default() {
	PopulateDefaultsMachineDeployment(m)
	// tolerate version strings without a "v" prefix: prepend it if it's not there
	if m.Spec.Template.Spec.Version != nil && !strings.HasPrefix(*m.Spec.Template.Spec.Version, "v") {
		normalizedVersion := "v" + *m.Spec.Template.Spec.Version
		m.Spec.Template.Spec.Version = &normalizedVersion
	}
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
// Deprecated: This method is going to be removed in a next release.
// Note: We're not using this method anymore and are using webhooks.MachineDeployment.ValidateCreate instead.
func (m *MachineDeployment) ValidateCreate() error {
	return m.validate(nil)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
// Deprecated: This method is going to be removed in a next release.
// Note: We're not using this method anymore and are using webhooks.MachineDeployment.ValidateUpdate instead.
func (m *MachineDeployment) ValidateUpdate(old runtime.Object) error {
	oldMD, ok := old.(*MachineDeployment)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a MachineDeployment but got a %T", old))
	}
	return m.validate(oldMD)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
// Deprecated: This method is going to be removed in a next release.
// Note: We're not using this method anymore and are using webhooks.MachineDeployment.ValidateDelete instead.
func (m *MachineDeployment) ValidateDelete() error {
	return nil
}

func (m *MachineDeployment) validate(old *MachineDeployment) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")
	selector, err := metav1.LabelSelectorAsSelector(&m.Spec.Selector)
	if err != nil {
		allErrs = append(
			allErrs,
			field.Invalid(specPath.Child("selector"), m.Spec.Selector, err.Error()),
		)
	} else if !selector.Matches(labels.Set(m.Spec.Template.Labels)) {
		allErrs = append(
			allErrs,
			field.Forbidden(
				specPath.Child("template", "metadata", "labels"),
				fmt.Sprintf("must match spec.selector %q", selector.String()),
			),
		)
	}

	if old != nil && old.Spec.ClusterName != m.Spec.ClusterName {
		allErrs = append(
			allErrs,
			field.Forbidden(
				specPath.Child("clusterName"),
				"field is immutable",
			),
		)
	}

	if m.Spec.Strategy != nil && m.Spec.Strategy.RollingUpdate != nil {
		total := 1
		if m.Spec.Replicas != nil {
			total = int(*m.Spec.Replicas)
		}

		if m.Spec.Strategy.RollingUpdate.MaxSurge != nil {
			if _, err := intstr.GetScaledValueFromIntOrPercent(m.Spec.Strategy.RollingUpdate.MaxSurge, total, true); err != nil {
				allErrs = append(
					allErrs,
					field.Invalid(specPath.Child("strategy", "rollingUpdate", "maxSurge"),
						m.Spec.Strategy.RollingUpdate.MaxSurge, fmt.Sprintf("must be either an int or a percentage: %v", err.Error())),
				)
			}
		}

		if m.Spec.Strategy.RollingUpdate.MaxUnavailable != nil {
			if _, err := intstr.GetScaledValueFromIntOrPercent(m.Spec.Strategy.RollingUpdate.MaxUnavailable, total, true); err != nil {
				allErrs = append(
					allErrs,
					field.Invalid(specPath.Child("strategy", "rollingUpdate", "maxUnavailable"),
						m.Spec.Strategy.RollingUpdate.MaxUnavailable, fmt.Sprintf("must be either an int or a percentage: %v", err.Error())),
				)
			}
		}
	}

	if m.Spec.Template.Spec.Version != nil {
		if !version.KubeSemver.MatchString(*m.Spec.Template.Spec.Version) {
			allErrs = append(allErrs, field.Invalid(specPath.Child("template", "spec", "version"), *m.Spec.Template.Spec.Version, "must be a valid semantic version"))
		}
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(GroupVersion.WithKind("MachineDeployment").GroupKind(), m.Name, allErrs)
}

// PopulateDefaultsMachineDeployment fills in default field values.
// This is also called during MachineDeployment sync.
func PopulateDefaultsMachineDeployment(d *MachineDeployment) {
	if d.Labels == nil {
		d.Labels = make(map[string]string)
	}
	d.Labels[ClusterLabelName] = d.Spec.ClusterName

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
		d.Spec.Strategy = &MachineDeploymentStrategy{}
	}

	if d.Spec.Strategy.Type == "" {
		d.Spec.Strategy.Type = RollingUpdateMachineDeploymentStrategyType
	}

	if d.Spec.Template.Labels == nil {
		d.Spec.Template.Labels = make(map[string]string)
	}

	// Default RollingUpdate strategy only if strategy type is RollingUpdate.
	if d.Spec.Strategy.Type == RollingUpdateMachineDeploymentStrategyType {
		if d.Spec.Strategy.RollingUpdate == nil {
			d.Spec.Strategy.RollingUpdate = &MachineRollingUpdateDeployment{}
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
		d.Spec.Selector.MatchLabels[MachineDeploymentLabelName] = d.Name
		d.Spec.Template.Labels[MachineDeploymentLabelName] = d.Name
	}
	// Make sure selector and template to be in the same cluster.
	d.Spec.Selector.MatchLabels[ClusterLabelName] = d.Spec.ClusterName
	d.Spec.Template.Labels[ClusterLabelName] = d.Spec.ClusterName
}
