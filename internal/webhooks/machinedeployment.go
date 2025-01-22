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
	"strconv"
	"strings"

	"github.com/pkg/errors"
	v1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	topologynames "sigs.k8s.io/cluster-api/internal/topology/names"
	"sigs.k8s.io/cluster-api/util/version"
)

func (webhook *MachineDeployment) SetupWebhookWithManager(mgr ctrl.Manager) error {
	if webhook.decoder == nil {
		webhook.decoder = admission.NewDecoder(mgr.GetScheme())
	}

	return ctrl.NewWebhookManagedBy(mgr).
		For(&clusterv1.MachineDeployment{}).
		WithDefaulter(webhook, admission.DefaulterRemoveUnknownOrOmitableFields).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-cluster-x-k8s-io-v1beta1-machinedeployment,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machinedeployments,versions=v1beta1,name=validation.machinedeployment.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1beta1-machinedeployment,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machinedeployments,versions=v1beta1,name=default.machinedeployment.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// MachineDeployment implements a validation and defaulting webhook for MachineDeployment.
type MachineDeployment struct {
	decoder admission.Decoder
}

var _ webhook.CustomDefaulter = &MachineDeployment{}
var _ webhook.CustomValidator = &MachineDeployment{}

// Default implements webhook.CustomDefaulter.
func (webhook *MachineDeployment) Default(ctx context.Context, obj runtime.Object) error {
	m, ok := obj.(*clusterv1.MachineDeployment)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a MachineDeployment but got a %T", obj))
	}

	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return err
	}
	dryRun := false
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}

	var oldMD *clusterv1.MachineDeployment
	if req.Operation == v1.Update {
		oldMD = &clusterv1.MachineDeployment{}
		if err := webhook.decoder.DecodeRaw(req.OldObject, oldMD); err != nil {
			return errors.Wrapf(err, "failed to decode oldObject to MachineDeployment")
		}
	}

	if m.Labels == nil {
		m.Labels = make(map[string]string)
	}
	m.Labels[clusterv1.ClusterNameLabel] = m.Spec.ClusterName

	replicas, err := calculateMachineDeploymentReplicas(ctx, oldMD, m, dryRun)
	if err != nil {
		return err
	}
	m.Spec.Replicas = ptr.To[int32](replicas)

	if m.Spec.MinReadySeconds == nil {
		m.Spec.MinReadySeconds = ptr.To[int32](0)
	}

	if m.Spec.RevisionHistoryLimit == nil {
		m.Spec.RevisionHistoryLimit = ptr.To[int32](1)
	}

	if m.Spec.ProgressDeadlineSeconds == nil {
		m.Spec.ProgressDeadlineSeconds = ptr.To[int32](600)
	}

	if m.Spec.Selector.MatchLabels == nil {
		m.Spec.Selector.MatchLabels = make(map[string]string)
	}

	if m.Spec.Strategy == nil {
		m.Spec.Strategy = &clusterv1.MachineDeploymentStrategy{}
	}

	if m.Spec.Strategy.Type == "" {
		m.Spec.Strategy.Type = clusterv1.RollingUpdateMachineDeploymentStrategyType
	}

	if m.Spec.Template.Labels == nil {
		m.Spec.Template.Labels = make(map[string]string)
	}

	// Default RollingUpdate strategy only if strategy type is RollingUpdate.
	if m.Spec.Strategy.Type == clusterv1.RollingUpdateMachineDeploymentStrategyType {
		if m.Spec.Strategy.RollingUpdate == nil {
			m.Spec.Strategy.RollingUpdate = &clusterv1.MachineRollingUpdateDeployment{}
		}
		if m.Spec.Strategy.RollingUpdate.MaxSurge == nil {
			ios1 := intstr.FromInt(1)
			m.Spec.Strategy.RollingUpdate.MaxSurge = &ios1
		}
		if m.Spec.Strategy.RollingUpdate.MaxUnavailable == nil {
			ios0 := intstr.FromInt(0)
			m.Spec.Strategy.RollingUpdate.MaxUnavailable = &ios0
		}
	}

	// If no selector has been provided, add label and selector for the
	// MachineDeployment's name as a default way of providing uniqueness.
	if len(m.Spec.Selector.MatchLabels) == 0 && len(m.Spec.Selector.MatchExpressions) == 0 {
		m.Spec.Selector.MatchLabels[clusterv1.MachineDeploymentNameLabel] = m.Name
		m.Spec.Template.Labels[clusterv1.MachineDeploymentNameLabel] = m.Name
	}
	// Make sure selector and template to be in the same cluster.
	m.Spec.Selector.MatchLabels[clusterv1.ClusterNameLabel] = m.Spec.ClusterName
	m.Spec.Template.Labels[clusterv1.ClusterNameLabel] = m.Spec.ClusterName

	// tolerate version strings without a "v" prefix: prepend it if it's not there
	if m.Spec.Template.Spec.Version != nil && !strings.HasPrefix(*m.Spec.Template.Spec.Version, "v") {
		normalizedVersion := "v" + *m.Spec.Template.Spec.Version
		m.Spec.Template.Spec.Version = &normalizedVersion
	}

	// Make sure the namespace of the referent is populated
	if m.Spec.Template.Spec.Bootstrap.ConfigRef != nil && m.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace == "" {
		m.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace = m.Namespace
	}

	if m.Spec.Template.Spec.InfrastructureRef.Namespace == "" {
		m.Spec.Template.Spec.InfrastructureRef.Namespace = m.Namespace
	}

	return nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *MachineDeployment) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	m, ok := obj.(*clusterv1.MachineDeployment)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a MachineDeployment but got a %T", obj))
	}

	return nil, webhook.validate(nil, m)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *MachineDeployment) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldMD, ok := oldObj.(*clusterv1.MachineDeployment)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a MachineDeployment but got a %T", oldObj))
	}

	newMD, ok := newObj.(*clusterv1.MachineDeployment)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a MachineDeployment but got a %T", newObj))
	}

	return nil, webhook.validate(oldMD, newMD)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *MachineDeployment) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (webhook *MachineDeployment) validate(oldMD, newMD *clusterv1.MachineDeployment) error {
	var allErrs field.ErrorList
	// The MachineDeployment name is used as a label value. This check ensures names which are not be valid label values are rejected.
	if errs := validation.IsValidLabelValue(newMD.Name); len(errs) != 0 {
		for _, err := range errs {
			allErrs = append(
				allErrs,
				field.Invalid(
					field.NewPath("metadata", "name"),
					newMD.Name,
					fmt.Sprintf("must be a valid label value: %s", err),
				),
			)
		}
	}
	specPath := field.NewPath("spec")
	selector, err := metav1.LabelSelectorAsSelector(&newMD.Spec.Selector)
	if err != nil {
		allErrs = append(
			allErrs,
			field.Invalid(specPath.Child("selector"), newMD.Spec.Selector, err.Error()),
		)
	} else if !selector.Matches(labels.Set(newMD.Spec.Template.Labels)) {
		allErrs = append(
			allErrs,
			field.Forbidden(
				specPath.Child("template", "metadata", "labels"),
				fmt.Sprintf("must match spec.selector %q", selector.String()),
			),
		)
	}

	// MachineSet preflight checks that should be skipped could also be set as annotation on the MachineDeployment
	// since MachineDeployment annotations are synced to the MachineSet.
	if feature.Gates.Enabled(feature.MachineSetPreflightChecks) {
		if err := validateSkippedMachineSetPreflightChecks(newMD); err != nil {
			allErrs = append(allErrs, err)
		}
	}

	if oldMD != nil && oldMD.Spec.ClusterName != newMD.Spec.ClusterName {
		allErrs = append(
			allErrs,
			field.Forbidden(
				specPath.Child("clusterName"),
				"field is immutable",
			),
		)
	}

	if newMD.Spec.Strategy != nil && newMD.Spec.Strategy.RollingUpdate != nil {
		total := 1
		if newMD.Spec.Replicas != nil {
			total = int(*newMD.Spec.Replicas)
		}

		if newMD.Spec.Strategy.RollingUpdate.MaxSurge != nil {
			if _, err := intstr.GetScaledValueFromIntOrPercent(newMD.Spec.Strategy.RollingUpdate.MaxSurge, total, true); err != nil {
				allErrs = append(
					allErrs,
					field.Invalid(specPath.Child("strategy", "rollingUpdate", "maxSurge"),
						newMD.Spec.Strategy.RollingUpdate.MaxSurge, fmt.Sprintf("must be either an int or a percentage: %v", err.Error())),
				)
			}
		}

		if newMD.Spec.Strategy.RollingUpdate.MaxUnavailable != nil {
			if _, err := intstr.GetScaledValueFromIntOrPercent(newMD.Spec.Strategy.RollingUpdate.MaxUnavailable, total, true); err != nil {
				allErrs = append(
					allErrs,
					field.Invalid(specPath.Child("strategy", "rollingUpdate", "maxUnavailable"),
						newMD.Spec.Strategy.RollingUpdate.MaxUnavailable, fmt.Sprintf("must be either an int or a percentage: %v", err.Error())),
				)
			}
		}
	}

	if newMD.Spec.Strategy != nil && newMD.Spec.Strategy.Remediation != nil {
		total := 1
		if newMD.Spec.Replicas != nil {
			total = int(*newMD.Spec.Replicas)
		}

		if newMD.Spec.Strategy.Remediation.MaxInFlight != nil {
			if _, err := intstr.GetScaledValueFromIntOrPercent(newMD.Spec.Strategy.Remediation.MaxInFlight, total, true); err != nil {
				allErrs = append(
					allErrs,
					field.Invalid(specPath.Child("strategy", "remediation", "maxInFlight"),
						newMD.Spec.Strategy.Remediation.MaxInFlight.String(), fmt.Sprintf("must be either an int or a percentage: %v", err.Error())),
				)
			}
		}
	}

	if newMD.Spec.Template.Spec.Version != nil {
		if !version.KubeSemver.MatchString(*newMD.Spec.Template.Spec.Version) {
			allErrs = append(allErrs, field.Invalid(specPath.Child("template", "spec", "version"), *newMD.Spec.Template.Spec.Version, "must be a valid semantic version"))
		}
	}

	if newMD.Spec.MachineNamingStrategy != nil {
		allErrs = append(allErrs, validateMDMachineNamingStrategy(newMD.Spec.MachineNamingStrategy, specPath.Child("machineNamingStrategy"))...)
	}

	// Validate the metadata of the template.
	allErrs = append(allErrs, newMD.Spec.Template.ObjectMeta.Validate(specPath.Child("template", "metadata"))...)

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("MachineDeployment").GroupKind(), newMD.Name, allErrs)
}

func validateMDMachineNamingStrategy(machineNamingStrategy *clusterv1.MachineNamingStrategy, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if machineNamingStrategy.Template != "" {
		if !strings.Contains(machineNamingStrategy.Template, "{{ .random }}") {
			allErrs = append(allErrs,
				field.Invalid(
					pathPrefix.Child("template"),
					machineNamingStrategy.Template,
					"invalid template, {{ .random }} is missing",
				))
		}
		name, err := topologynames.MachineSetMachineNameGenerator(machineNamingStrategy.Template, "cluster", "machineset").GenerateName()
		if err != nil {
			allErrs = append(allErrs,
				field.Invalid(
					pathPrefix.Child("template"),
					machineNamingStrategy.Template,
					fmt.Sprintf("invalid template: %v", err),
				))
		} else {
			for _, err := range validation.IsDNS1123Subdomain(name) {
				allErrs = append(allErrs,
					field.Invalid(
						pathPrefix.Child("template"),
						machineNamingStrategy.Template,
						fmt.Sprintf("invalid template, generated names would not be valid Kubernetes object names: %v", err),
					))
			}
		}
	}

	return allErrs
}

// calculateMachineDeploymentReplicas calculates the default value of the replicas field.
// The value will be calculated based on the following logic:
// * if replicas is already set on newMD, keep the current value
// * if the autoscaler min size and max size annotations are set:
//   - if it's a new MachineDeployment, use min size
//   - if the replicas field of the old MachineDeployment is < min size, use min size
//   - if the replicas field of the old MachineDeployment is > max size, use max size
//   - if the replicas field of the old MachineDeployment is in the (min size, max size) range, keep the value from the oldMD
//
// * otherwise use 1
//
// The goal of this logic is to provide a smoother UX for clusters using the Kubernetes autoscaler.
// Note: Autoscaler only takes over control of the replicas field if the replicas value is in the (min size, max size) range.
//
// We are supporting the following use cases:
// * A new MD is created and replicas should be managed by the autoscaler
//   - If the min size and max size annotations are set, the replicas field is defaulted to the value of the min size
//     annotation so the autoscaler can take control.
//
// * An existing MD which initially wasn't controlled by the autoscaler should be later controlled by the autoscaler
//   - To adopt an existing MD users can use the min size and max size annotations to enable the autoscaler
//     and to ensure the replicas field is within the (min size, max size) range. Without defaulting based on the annotations, handing over
//     control to the autoscaler by unsetting the replicas field would lead to the field being set to 1. This could be
//     very disruptive if the previous value of the replica field is greater than 1.
func calculateMachineDeploymentReplicas(ctx context.Context, oldMD *clusterv1.MachineDeployment, newMD *clusterv1.MachineDeployment, dryRun bool) (int32, error) {
	// If replicas is already set => Keep the current value.
	if newMD.Spec.Replicas != nil {
		return *newMD.Spec.Replicas, nil
	}

	log := ctrl.LoggerFrom(ctx)

	// If both autoscaler annotations are set, use them to calculate the default value.
	minSizeString, hasMinSizeAnnotation := newMD.Annotations[clusterv1.AutoscalerMinSizeAnnotation]
	maxSizeString, hasMaxSizeAnnotation := newMD.Annotations[clusterv1.AutoscalerMaxSizeAnnotation]
	if hasMinSizeAnnotation && hasMaxSizeAnnotation {
		minSize, err := strconv.ParseInt(minSizeString, 10, 32)
		if err != nil {
			return 0, errors.Wrapf(err, "failed to caculate MachineDeployment replicas value: could not parse the value of the %q annotation", clusterv1.AutoscalerMinSizeAnnotation)
		}
		maxSize, err := strconv.ParseInt(maxSizeString, 10, 32)
		if err != nil {
			return 0, errors.Wrapf(err, "failed to caculate MachineDeployment replicas value: could not parse the value of the %q annotation", clusterv1.AutoscalerMaxSizeAnnotation)
		}

		// If it's a new MachineDeployment => Use the min size.
		// Note: This will result in a scale up to get into the range where autoscaler takes over.
		if oldMD == nil {
			if !dryRun {
				log.V(2).Info(fmt.Sprintf("Replica field has been defaulted to %d based on the %s annotation (MD is a new MD)", minSize, clusterv1.AutoscalerMinSizeAnnotation))
			}
			return int32(minSize), nil
		}

		// Otherwise we are handing over the control for the replicas field for an existing MachineDeployment
		// to the autoscaler.

		switch {
		// If the old MachineDeployment doesn't have replicas set => Use the min size.
		// Note: As defaulting always sets the replica field, this case should not be possible
		// We only have this handling to be 100% safe against panics.
		case oldMD.Spec.Replicas == nil:
			if !dryRun {
				log.V(2).Info(fmt.Sprintf("Replica field has been defaulted to %d based on the %s annotation (old MD didn't have replicas set)", minSize, clusterv1.AutoscalerMinSizeAnnotation))
			}
			return int32(minSize), nil
		// If the old MachineDeployment replicas are lower than min size => Use the min size.
		// Note: This will result in a scale up to get into the range where autoscaler takes over.
		case *oldMD.Spec.Replicas < int32(minSize):
			if !dryRun {
				log.V(2).Info(fmt.Sprintf("Replica field has been defaulted to %d based on the %s annotation (old MD had replicas below min size)", minSize, clusterv1.AutoscalerMinSizeAnnotation))
			}
			return int32(minSize), nil
		// If the old MachineDeployment replicas are higher than max size => Use the max size.
		// Note: This will result in a scale down to get into the range where autoscaler takes over.
		case *oldMD.Spec.Replicas > int32(maxSize):
			if !dryRun {
				log.V(2).Info(fmt.Sprintf("Replica field has been defaulted to %d based on the %s annotation (old MD had replicas above max size)", maxSize, clusterv1.AutoscalerMaxSizeAnnotation))
			}
			return int32(maxSize), nil
		// If the old MachineDeployment replicas are between min and max size => Keep the current value.
		default:
			if !dryRun {
				log.V(2).Info(fmt.Sprintf("Replica field has been defaulted to %d based on replicas of the old MachineDeployment (old MD had replicas within min size / max size range)", *oldMD.Spec.Replicas))
			}
			return *oldMD.Spec.Replicas, nil
		}
	}

	// If neither the default nor the autoscaler annotations are set => Default to 1.
	if !dryRun {
		log.V(2).Info("Replica field has been defaulted to 1")
	}
	return 1, nil
}
