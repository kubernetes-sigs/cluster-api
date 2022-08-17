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
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
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

// MachineSet implements a validating and defaulting webhook for MachineSet.
type MachineSet struct {
	Client client.Reader
}

var _ webhook.CustomDefaulter = &MachineSet{}
var _ webhook.CustomValidator = &MachineSet{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type.
func (webhook *MachineSet) Default(_ context.Context, obj runtime.Object) error {
	machineSet, ok := obj.(*clusterv1.MachineSet)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a MachineSet but got a %T", obj))
	}
	if machineSet.Labels == nil {
		machineSet.Labels = make(map[string]string)
	}
	machineSet.Labels[clusterv1.ClusterLabelName] = machineSet.Spec.ClusterName

	if machineSet.Spec.DeletePolicy == "" {
		randomPolicy := string(clusterv1.RandomMachineSetDeletePolicy)
		machineSet.Spec.DeletePolicy = randomPolicy
	}

	if machineSet.Spec.Selector.MatchLabels == nil {
		machineSet.Spec.Selector.MatchLabels = make(map[string]string)
	}

	if machineSet.Spec.Template.Labels == nil {
		machineSet.Spec.Template.Labels = make(map[string]string)
	}

	if len(machineSet.Spec.Selector.MatchLabels) == 0 && len(machineSet.Spec.Selector.MatchExpressions) == 0 {
		machineSet.Spec.Selector.MatchLabels[clusterv1.MachineSetLabelName] = machineSet.Name
		machineSet.Spec.Template.Labels[clusterv1.MachineSetLabelName] = machineSet.Name
	}

	if machineSet.Spec.Template.Spec.Version != nil && !strings.HasPrefix(*machineSet.Spec.Template.Spec.Version, "v") {
		normalizedVersion := "v" + *machineSet.Spec.Template.Spec.Version
		machineSet.Spec.Template.Spec.Version = &normalizedVersion
	}
	return nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *MachineSet) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	machineSet, ok := obj.(*clusterv1.MachineSet)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a MachineSet but got a %T", obj))
	}
	return webhook.validate(ctx, nil, machineSet)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *MachineSet) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	newMS, ok := newObj.(*clusterv1.MachineSet)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a MachineSet but got a %T", newObj))
	}
	oldMS, ok := oldObj.(*clusterv1.MachineSet)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a MachineSet but got a %T", oldObj))
	}
	return webhook.validate(ctx, oldMS, newMS)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *MachineSet) ValidateDelete(_ context.Context, _ runtime.Object) error {
	return nil
}

func (webhook *MachineSet) validate(_ context.Context, oldMachineSet, newMachineSet *clusterv1.MachineSet) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")
	selector, err := metav1.LabelSelectorAsSelector(&newMachineSet.Spec.Selector)
	if err != nil {
		allErrs = append(
			allErrs,
			field.Invalid(
				specPath.Child("selector"),
				newMachineSet.Spec.Selector,
				err.Error(),
			),
		)
	} else if !selector.Matches(labels.Set(newMachineSet.Spec.Template.Labels)) {
		allErrs = append(
			allErrs,
			field.Invalid(
				specPath.Child("template", "metadata", "labels"),
				newMachineSet.Spec.Template.ObjectMeta.Labels,
				fmt.Sprintf("must match spec.selector %q", selector.String()),
			),
		)
	}

	if oldMachineSet != nil {
		if oldMachineSet.Spec.ClusterName != newMachineSet.Spec.ClusterName {
			allErrs = append(
				allErrs,
				field.Forbidden(
					specPath.Child("clusterName"),
					"field is immutable",
				),
			)
		}
	}

	if newMachineSet.Spec.Template.Spec.Version != nil {
		if !version.KubeSemver.MatchString(*newMachineSet.Spec.Template.Spec.Version) {
			allErrs = append(
				allErrs,
				field.Invalid(
					specPath.Child("template", "spec", "version"),
					*newMachineSet.Spec.Template.Spec.Version,
					"must be a valid semantic version",
				),
			)
		}
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("MachineSet").GroupKind(), newMachineSet.Name, allErrs)
}
