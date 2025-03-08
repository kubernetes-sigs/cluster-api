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
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	addonsv1 "sigs.k8s.io/cluster-api/api/addons/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
)

// ClusterResourceSet implements a validation and defaulting webhook for ClusterResourceSet.
type ClusterResourceSet struct{}

func (webhook *ClusterResourceSet) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&addonsv1.ClusterResourceSet{}).
		WithDefaulter(webhook, admission.DefaulterRemoveUnknownOrOmitableFields).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-addons-cluster-x-k8s-io-v1beta1-clusterresourceset,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=addons.cluster.x-k8s.io,resources=clusterresourcesets,versions=v1beta1,name=validation.clusterresourceset.addons.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-addons-cluster-x-k8s-io-v1beta1-clusterresourceset,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=addons.cluster.x-k8s.io,resources=clusterresourcesets,versions=v1beta1,name=default.clusterresourceset.addons.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.CustomDefaulter = &ClusterResourceSet{}
var _ webhook.CustomValidator = &ClusterResourceSet{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (webhook *ClusterResourceSet) Default(_ context.Context, obj runtime.Object) error {
	crs, ok := obj.(*addonsv1.ClusterResourceSet)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterResourceSet but got a %T", obj))
	}
	// ClusterResourceSet Strategy defaults to ApplyOnce.
	if crs.Spec.Strategy == "" {
		crs.Spec.Strategy = string(addonsv1.ClusterResourceSetStrategyApplyOnce)
	}
	return nil
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *ClusterResourceSet) ValidateCreate(_ context.Context, newObj runtime.Object) (admission.Warnings, error) {
	newCRS, ok := newObj.(*addonsv1.ClusterResourceSet)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterResourceSet but got a %T", newObj))
	}
	return nil, webhook.validate(nil, newCRS)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *ClusterResourceSet) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldCRS, ok := oldObj.(*addonsv1.ClusterResourceSet)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterResourceSet but got a %T", oldObj))
	}
	newCRS, ok := newObj.(*addonsv1.ClusterResourceSet)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterResourceSet but got a %T", newObj))
	}
	return nil, webhook.validate(oldCRS, newCRS)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *ClusterResourceSet) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (webhook *ClusterResourceSet) validate(oldCRS, newCRS *addonsv1.ClusterResourceSet) error {
	var allErrs field.ErrorList

	// NOTE: ClusterResourceSet is behind ClusterResourceSet feature gate flag; the web hook
	// must prevent creating new objects when the feature flag is disabled.
	if !feature.Gates.Enabled(feature.ClusterResourceSet) {
		return field.Forbidden(
			field.NewPath("spec"),
			"can be set only if the ClusterResourceSet feature flag is enabled",
		)
	}

	// Validate selector parses as Selector
	selector, err := metav1.LabelSelectorAsSelector(&newCRS.Spec.ClusterSelector)
	if err != nil {
		allErrs = append(
			allErrs,
			field.Invalid(field.NewPath("spec", "clusterSelector"), newCRS.Spec.ClusterSelector, err.Error()),
		)
	}

	// Validate that the selector isn't empty as null selectors do not select any objects.
	if selector != nil && selector.Empty() {
		allErrs = append(
			allErrs,
			field.Invalid(field.NewPath("spec", "clusterSelector"), newCRS.Spec.ClusterSelector, "selector must not be empty"),
		)
	}

	if oldCRS != nil && oldCRS.Spec.Strategy != "" && oldCRS.Spec.Strategy != newCRS.Spec.Strategy {
		allErrs = append(
			allErrs,
			field.Invalid(field.NewPath("spec", "strategy"), newCRS.Spec.Strategy, "field is immutable"),
		)
	}

	if oldCRS != nil && !reflect.DeepEqual(oldCRS.Spec.ClusterSelector, newCRS.Spec.ClusterSelector) {
		allErrs = append(
			allErrs,
			field.Invalid(field.NewPath("spec", "clusterSelector"), newCRS.Spec.ClusterSelector, "field is immutable"),
		)
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(addonsv1.GroupVersion.WithKind("ClusterResourceSet").GroupKind(), newCRS.Name, allErrs)
}
