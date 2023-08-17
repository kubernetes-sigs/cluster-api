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
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
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
	return ctrl.NewWebhookManagedBy(mgr).
		For(&clusterv1.MachineSet{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-cluster-x-k8s-io-v1beta1-machineset,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machinesets,versions=v1beta1,name=validation.machineset.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1beta1-machineset,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machinesets,versions=v1beta1,name=default.machineset.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// MachineSet implements a validation and defaulting webhook for MachineSet.
type MachineSet struct{}

var _ webhook.CustomDefaulter = &MachineSet{}
var _ webhook.CustomValidator = &MachineSet{}

// Default sets default MachineSet field values.
func (webhook *MachineSet) Default(_ context.Context, obj runtime.Object) error {
	m, ok := obj.(*clusterv1.MachineSet)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a MachineSet but got a %T", obj))
	}

	if m.Labels == nil {
		m.Labels = make(map[string]string)
	}
	m.Labels[clusterv1.ClusterNameLabel] = m.Spec.ClusterName

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
