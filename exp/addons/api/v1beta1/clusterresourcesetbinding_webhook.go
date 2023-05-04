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

package v1beta1

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"sigs.k8s.io/cluster-api/feature"
)

func (c *ClusterResourceSetBinding) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(c).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-addons-cluster-x-k8s-io-v1beta1-clusterresourcesetbinding,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=addons.cluster.x-k8s.io,resources=clusterresourcesetbindings,versions=v1beta1,name=validation.clusterresourcesetbinding.addons.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.Validator = &ClusterResourceSetBinding{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (c *ClusterResourceSetBinding) ValidateCreate() (admission.Warnings, error) {
	return nil, c.validate(nil)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (c *ClusterResourceSetBinding) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	oldBinding, ok := old.(*ClusterResourceSetBinding)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterResourceSetBinding but got a %T", old))
	}
	return nil, c.validate(oldBinding)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (c *ClusterResourceSetBinding) ValidateDelete() (admission.Warnings, error) {
	return nil, nil
}

func (c *ClusterResourceSetBinding) validate(old *ClusterResourceSetBinding) error {
	// NOTE: ClusterResourceSet is behind ClusterResourceSet feature gate flag; the web hook
	// must prevent creating new objects in case the feature flag is disabled.
	if !feature.Gates.Enabled(feature.ClusterResourceSet) {
		return field.Forbidden(
			field.NewPath("spec"),
			"can be set only if the ClusterResourceSet feature flag is enabled",
		)
	}
	var allErrs field.ErrorList
	if old != nil && old.Spec.ClusterName != "" && old.Spec.ClusterName != c.Spec.ClusterName {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "clusterName"), c.Spec.ClusterName, "field is immutable"))
	}
	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(GroupVersion.WithKind("ClusterResourceSetBinding").GroupKind(), c.Name, allErrs)
}
