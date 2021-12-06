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
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func (m *ClusterResourceSet) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(m).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-addons-cluster-x-k8s-io-v1beta1-clusterresourceset,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=addons.cluster.x-k8s.io,resources=clusterresourcesets,versions=v1beta1,name=validation.clusterresourceset.addons.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-addons-cluster-x-k8s-io-v1beta1-clusterresourceset,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=addons.cluster.x-k8s.io,resources=clusterresourcesets,versions=v1beta1,name=default.clusterresourceset.addons.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.Defaulter = &ClusterResourceSet{}
var _ webhook.Validator = &ClusterResourceSet{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (m *ClusterResourceSet) Default() {
	// ClusterResourceSet Strategy defaults to ApplyOnce.
	if m.Spec.Strategy == "" {
		m.Spec.Strategy = string(ClusterResourceSetStrategyApplyOnce)
	}
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (m *ClusterResourceSet) ValidateCreate() error {
	return m.validate(nil)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (m *ClusterResourceSet) ValidateUpdate(old runtime.Object) error {
	oldCRS, ok := old.(*ClusterResourceSet)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterResourceSet but got a %T", old))
	}
	return m.validate(oldCRS)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (m *ClusterResourceSet) ValidateDelete() error {
	return nil
}

func (m *ClusterResourceSet) validate(old *ClusterResourceSet) error {
	var allErrs field.ErrorList

	// Validate selector parses as Selector
	selector, err := metav1.LabelSelectorAsSelector(&m.Spec.ClusterSelector)
	if err != nil {
		allErrs = append(
			allErrs,
			field.Invalid(field.NewPath("spec", "clusterSelector"), m.Spec.ClusterSelector, err.Error()),
		)
	}

	// Validate that the selector isn't empty as null selectors do not select any objects.
	if selector != nil && selector.Empty() {
		allErrs = append(
			allErrs,
			field.Invalid(field.NewPath("spec", "clusterSelector"), m.Spec.ClusterSelector, "selector must not be empty"),
		)
	}

	if old != nil && old.Spec.Strategy != "" && old.Spec.Strategy != m.Spec.Strategy {
		allErrs = append(
			allErrs,
			field.Invalid(field.NewPath("spec", "strategy"), m.Spec.Strategy, "field is immutable"),
		)
	}

	if old != nil && !reflect.DeepEqual(old.Spec.ClusterSelector, m.Spec.ClusterSelector) {
		allErrs = append(
			allErrs,
			field.Invalid(field.NewPath("spec", "clusterSelector"), m.Spec.ClusterSelector, "field is immutable"),
		)
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(GroupVersion.WithKind("ClusterResourceSet").GroupKind(), m.Name, allErrs)
}
