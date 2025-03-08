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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	addonsv1 "sigs.k8s.io/cluster-api/api/addons/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
)

func (webhook *ClusterResourceSetBinding) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&addonsv1.ClusterResourceSetBinding{}).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-addons-cluster-x-k8s-io-v1beta1-clusterresourcesetbinding,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=addons.cluster.x-k8s.io,resources=clusterresourcesetbindings,versions=v1beta1,name=validation.clusterresourcesetbinding.addons.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// ClusterResourceSetBinding implements a validation webhook for ClusterResourceSetBinding.
type ClusterResourceSetBinding struct{}

var _ webhook.CustomValidator = &ClusterResourceSetBinding{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *ClusterResourceSetBinding) ValidateCreate(_ context.Context, newObj runtime.Object) (admission.Warnings, error) {
	newBinding, ok := newObj.(*addonsv1.ClusterResourceSetBinding)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterResourceSetBinding but got a %T", newObj))
	}
	return nil, webhook.validate(nil, newBinding)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *ClusterResourceSetBinding) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldBinding, ok := oldObj.(*addonsv1.ClusterResourceSetBinding)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterResourceSetBinding but got a %T", oldObj))
	}
	newBinding, ok := newObj.(*addonsv1.ClusterResourceSetBinding)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterResourceSetBinding but got a %T", newObj))
	}
	return nil, webhook.validate(oldBinding, newBinding)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *ClusterResourceSetBinding) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (webhook *ClusterResourceSetBinding) validate(oldCRSB, newCRSB *addonsv1.ClusterResourceSetBinding) error {
	var allErrs field.ErrorList

	// NOTE: ClusterResourceSet is behind ClusterResourceSet feature gate flag; the web hook
	// must prevent creating new objects in case the feature flag is disabled.
	if !feature.Gates.Enabled(feature.ClusterResourceSet) {
		return field.Forbidden(
			field.NewPath("spec"),
			"can be set only if the ClusterResourceSet feature flag is enabled",
		)
	}

	if oldCRSB != nil && oldCRSB.Spec.ClusterName != "" && oldCRSB.Spec.ClusterName != newCRSB.Spec.ClusterName {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "clusterName"), newCRSB.Spec.ClusterName, "field is immutable"))
	}
	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(addonsv1.GroupVersion.WithKind("ClusterResourceSetBinding").GroupKind(), newCRSB.Name, allErrs)
}
