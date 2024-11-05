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
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/labels/format"
	"sigs.k8s.io/cluster-api/util/version"
)

func (webhook *MachineSet) SetupWebhookWithManager(mgr ctrl.Manager) error {
	if webhook.decoder == nil {
		webhook.decoder = admission.NewDecoder(mgr.GetScheme())
	}

	return ctrl.NewWebhookManagedBy(mgr).
		For(&clusterv1.MachineSet{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-cluster-x-k8s-io-v1beta1-machineset,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machinesets,versions=v1beta1,name=validation.machineset.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1beta1-machineset,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machinesets,versions=v1beta1,name=default.machineset.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// MachineSet implements a validation and defaulting webhook for MachineSet.
type MachineSet struct {
	decoder admission.Decoder
}

var _ webhook.CustomDefaulter = &MachineSet{}
var _ webhook.CustomValidator = &MachineSet{}

// Default sets default MachineSet field values.
func (webhook *MachineSet) Default(ctx context.Context, obj runtime.Object) error {
	m, ok := obj.(*clusterv1.MachineSet)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a MachineSet but got a %T", obj))
	}

	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return err
	}

	dryRun := false
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}

	var oldMS *clusterv1.MachineSet
	if req.Operation == v1.Update {
		oldMS = &clusterv1.MachineSet{}
		if err := webhook.decoder.DecodeRaw(req.OldObject, oldMS); err != nil {
			return errors.Wrapf(err, "failed to decode oldObject to MachineSet")
		}
	}

	if m.Labels == nil {
		m.Labels = make(map[string]string)
	}
	m.Labels[clusterv1.ClusterNameLabel] = m.Spec.ClusterName

	replicas, err := calculateMachineSetReplicas(ctx, oldMS, m, dryRun)
	if err != nil {
		return err
	}
	m.Spec.Replicas = ptr.To[int32](replicas)

	if m.Spec.DeletePolicy == "" {
		randomPolicy := string(clusterv1.RandomMachineSetDeletePolicy)
		m.Spec.DeletePolicy = randomPolicy
	}

	if m.Spec.Selector.MatchLabels == nil {
		m.Spec.Selector.MatchLabels = make(map[string]string)
	}

	if m.Spec.Template.Labels == nil {
		m.Spec.Template.Labels = make(map[string]string)
	}

	if len(m.Spec.Selector.MatchLabels) == 0 && len(m.Spec.Selector.MatchExpressions) == 0 {
		// Note: MustFormatValue is used here as the value of this label will be a hash if the MachineSet name is longer than 63 characters.
		m.Spec.Selector.MatchLabels[clusterv1.MachineSetNameLabel] = format.MustFormatValue(m.Name)
		m.Spec.Template.Labels[clusterv1.MachineSetNameLabel] = format.MustFormatValue(m.Name)
	}

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

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *MachineSet) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	m, ok := obj.(*clusterv1.MachineSet)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a MachineSet but got a %T", obj))
	}

	return nil, webhook.validate(nil, m)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *MachineSet) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldMS, ok := oldObj.(*clusterv1.MachineSet)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a MachineSet but got a %T", oldObj))
	}
	newMS, ok := newObj.(*clusterv1.MachineSet)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a MachineSet but got a %T", newObj))
	}

	return nil, webhook.validate(oldMS, newMS)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *MachineSet) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (webhook *MachineSet) validate(oldMS, newMS *clusterv1.MachineSet) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")
	selector, err := metav1.LabelSelectorAsSelector(&newMS.Spec.Selector)
	if err != nil {
		allErrs = append(
			allErrs,
			field.Invalid(
				specPath.Child("selector"),
				newMS.Spec.Selector,
				err.Error(),
			),
		)
	} else if !selector.Matches(labels.Set(newMS.Spec.Template.Labels)) {
		allErrs = append(
			allErrs,
			field.Invalid(
				specPath.Child("template", "metadata", "labels"),
				newMS.Spec.Template.ObjectMeta.Labels,
				fmt.Sprintf("must match spec.selector %q", selector.String()),
			),
		)
	}

	if feature.Gates.Enabled(feature.MachineSetPreflightChecks) {
		if err := validateSkippedMachineSetPreflightChecks(newMS); err != nil {
			allErrs = append(allErrs, err)
		}
	}

	if oldMS != nil && oldMS.Spec.ClusterName != newMS.Spec.ClusterName {
		allErrs = append(
			allErrs,
			field.Forbidden(
				specPath.Child("clusterName"),
				"field is immutable",
			),
		)
	}

	if newMS.Spec.Template.Spec.Version != nil {
		if !version.KubeSemver.MatchString(*newMS.Spec.Template.Spec.Version) {
			allErrs = append(
				allErrs,
				field.Invalid(
					specPath.Child("template", "spec", "version"),
					*newMS.Spec.Template.Spec.Version,
					"must be a valid semantic version",
				),
			)
		}
	}

	// Validate the metadata of the template.
	allErrs = append(allErrs, newMS.Spec.Template.ObjectMeta.Validate(specPath.Child("template", "metadata"))...)

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("MachineSet").GroupKind(), newMS.Name, allErrs)
}

func validateSkippedMachineSetPreflightChecks(o client.Object) *field.Error {
	if o == nil {
		return nil
	}
	skip := o.GetAnnotations()[clusterv1.MachineSetSkipPreflightChecksAnnotation]
	if skip == "" {
		return nil
	}

	supported := sets.New[clusterv1.MachineSetPreflightCheck](
		clusterv1.MachineSetPreflightCheckAll,
		clusterv1.MachineSetPreflightCheckKubeadmVersionSkew,
		clusterv1.MachineSetPreflightCheckKubernetesVersionSkew,
		clusterv1.MachineSetPreflightCheckControlPlaneIsStable,
	)

	skippedList := strings.Split(skip, ",")
	invalid := []clusterv1.MachineSetPreflightCheck{}
	for i := range skippedList {
		skipped := clusterv1.MachineSetPreflightCheck(strings.TrimSpace(skippedList[i]))
		if !supported.Has(skipped) {
			invalid = append(invalid, skipped)
		}
	}
	if len(invalid) > 0 {
		return field.Invalid(
			field.NewPath("metadata", "annotations", clusterv1.MachineSetSkipPreflightChecksAnnotation),
			invalid,
			fmt.Sprintf("skipped preflight check(s) must be among: %v", sets.List(supported)),
		)
	}
	return nil
}

// calculateMachineSetReplicas calculates the default value of the replicas field.
// The value will be calculated based on the following logic:
// * if replicas is already set on newMS, keep the current value
// * if the autoscaler min size and max size annotations are set:
//   - if it's a new MachineSet, use min size
//   - if the replicas field of the old MachineSet is < min size, use min size
//   - if the replicas field of the old MachineSet is > max size, use max size
//   - if the replicas field of the old MachineSet is in the (min size, max size) range, keep the value from the oldMS
//
// * otherwise use 1
//
// The goal of this logic is to provide a smoother UX for clusters using the Kubernetes autoscaler.
// Note: Autoscaler only takes over control of the replicas field if the replicas value is in the (min size, max size) range.
//
// We are supporting the following use cases:
// * A new MS is created and replicas should be managed by the autoscaler
//   - If the min size and max size annotations are set, the replicas field is defaulted to the value of the min size
//     annotation so the autoscaler can take control.
//
// * An existing MS which initially wasn't controlled by the autoscaler should be later controlled by the autoscaler
//   - To adopt an existing MS users can use the min size and max size annotations to enable the autoscaler
//     and to ensure the replicas field is within the (min size, max size) range. Without defaulting based on the annotations, handing over
//     control to the autoscaler by unsetting the replicas field would lead to the field being set to 1. This could be
//     very disruptive if the previous value of the replica field is greater than 1.
func calculateMachineSetReplicas(ctx context.Context, oldMS *clusterv1.MachineSet, newMS *clusterv1.MachineSet, dryRun bool) (int32, error) {
	// If replicas is already set => Keep the current value.
	if newMS.Spec.Replicas != nil {
		return *newMS.Spec.Replicas, nil
	}

	log := ctrl.LoggerFrom(ctx)

	// If both autoscaler annotations are set, use them to calculate the default value.
	minSizeString, hasMinSizeAnnotation := newMS.Annotations[clusterv1.AutoscalerMinSizeAnnotation]
	maxSizeString, hasMaxSizeAnnotation := newMS.Annotations[clusterv1.AutoscalerMaxSizeAnnotation]
	if hasMinSizeAnnotation && hasMaxSizeAnnotation {
		minSize, err := strconv.ParseInt(minSizeString, 10, 32)
		if err != nil {
			return 0, errors.Wrapf(err, "failed to caculate MachineSet replicas value: could not parse the value of the %q annotation", clusterv1.AutoscalerMinSizeAnnotation)
		}
		maxSize, err := strconv.ParseInt(maxSizeString, 10, 32)
		if err != nil {
			return 0, errors.Wrapf(err, "failed to caculate MachineSet replicas value: could not parse the value of the %q annotation", clusterv1.AutoscalerMaxSizeAnnotation)
		}

		// If it's a new MachineSet => Use the min size.
		// Note: This will result in a scale up to get into the range where autoscaler takes over.
		if oldMS == nil {
			if !dryRun {
				log.V(2).Info(fmt.Sprintf("Replica field has been defaulted to %d based on the %s annotation (MS is a new MS)", minSize, clusterv1.AutoscalerMinSizeAnnotation))
			}
			return int32(minSize), nil
		}

		// Otherwise we are handing over the control for the replicas field for an existing MachineSet
		// to the autoscaler.

		switch {
		// If the old MachineSet doesn't have replicas set => Use the min size.
		// Note: As defaulting always sets the replica field, this case should not be possible
		// We only have this handling to be 100% safe against panics.
		case oldMS.Spec.Replicas == nil:
			if !dryRun {
				log.V(2).Info(fmt.Sprintf("Replica field has been defaulted to %d based on the %s annotation (old MS didn't have replicas set)", minSize, clusterv1.AutoscalerMinSizeAnnotation))
			}
			return int32(minSize), nil
		// If the old MachineSet replicas are lower than min size => Use the min size.
		// Note: This will result in a scale up to get into the range where autoscaler takes over.
		case *oldMS.Spec.Replicas < int32(minSize):
			if !dryRun {
				log.V(2).Info(fmt.Sprintf("Replica field has been defaulted to %d based on the %s annotation (old MS had replicas below min size)", minSize, clusterv1.AutoscalerMinSizeAnnotation))
			}
			return int32(minSize), nil
		// If the old MachineSet replicas are higher than max size => Use the max size.
		// Note: This will result in a scale down to get into the range where autoscaler takes over.
		case *oldMS.Spec.Replicas > int32(maxSize):
			if !dryRun {
				log.V(2).Info(fmt.Sprintf("Replica field has been defaulted to %d based on the %s annotation (old MS had replicas above max size)", maxSize, clusterv1.AutoscalerMaxSizeAnnotation))
			}
			return int32(maxSize), nil
		// If the old MachineSet replicas are between min and max size => Keep the current value.
		default:
			if !dryRun {
				log.V(2).Info(fmt.Sprintf("Replica field has been defaulted to %d based on replicas of the old MachineSet (old MS had replicas within min size / max size range)", *oldMS.Spec.Replicas))
			}
			return *oldMS.Spec.Replicas, nil
		}
	}

	// If neither the default nor the autoscaler annotations are set => Default to 1.
	if !dryRun {
		log.V(2).Info("Replica field has been defaulted to 1")
	}
	return 1, nil
}
