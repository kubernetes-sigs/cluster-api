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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/version"
)

func (webhook *MachinePool) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&expv1.MachinePool{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-cluster-x-k8s-io-v1beta1-machinepool,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machinepools,versions=v1beta1,name=validation.machinepool.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1beta1-machinepool,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machinepools,versions=v1beta1,name=default.machinepool.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// MachinePool implements a validation and defaulting webhook for MachinePool.
type MachinePool struct{}

var _ webhook.CustomValidator = &MachinePool{}
var _ webhook.CustomDefaulter = &MachinePool{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (webhook *MachinePool) Default(_ context.Context, obj runtime.Object) error {
	m, ok := obj.(*expv1.MachinePool)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a MachinePool but got a %T", obj))
	}

	if m.Labels == nil {
		m.Labels = make(map[string]string)
	}
	m.Labels[clusterv1.ClusterNameLabel] = m.Spec.ClusterName

	if m.Spec.Replicas == nil {
		m.Spec.Replicas = ptr.To[int32](1)
	}

	if m.Spec.MinReadySeconds == nil {
		m.Spec.MinReadySeconds = ptr.To[int32](0)
	}

	if m.Spec.Template.Spec.Bootstrap.ConfigRef != nil && m.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace == "" {
		m.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace = m.Namespace
	}

	if m.Spec.Template.Spec.InfrastructureRef.Namespace == "" {
		m.Spec.Template.Spec.InfrastructureRef.Namespace = m.Namespace
	}

	// tolerate version strings without a "v" prefix: prepend it if it's not there.
	if m.Spec.Template.Spec.Version != nil && !strings.HasPrefix(*m.Spec.Template.Spec.Version, "v") {
		normalizedVersion := "v" + *m.Spec.Template.Spec.Version
		m.Spec.Template.Spec.Version = &normalizedVersion
	}
	return nil
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *MachinePool) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	mp, ok := obj.(*expv1.MachinePool)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a MachinePool but got a %T", obj))
	}

	return nil, webhook.validate(nil, mp)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *MachinePool) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldMP, ok := oldObj.(*expv1.MachinePool)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a MachinePool but got a %T", oldObj))
	}
	newMP, ok := newObj.(*expv1.MachinePool)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a MachinePool but got a %T", newObj))
	}
	return nil, webhook.validate(oldMP, newMP)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (webhook *MachinePool) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	mp, ok := obj.(*expv1.MachinePool)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a MachinePool but got a %T", obj))
	}

	return nil, webhook.validate(nil, mp)
}

func (webhook *MachinePool) validate(oldObj, newObj *expv1.MachinePool) error {
	// NOTE: MachinePool is behind MachinePool feature gate flag; the web hook
	// must prevent creating newObj objects when the feature flag is disabled.
	specPath := field.NewPath("spec")
	if !feature.Gates.Enabled(feature.MachinePool) {
		return field.Forbidden(
			specPath,
			"can be set only if the MachinePool feature flag is enabled",
		)
	}
	var allErrs field.ErrorList
	if newObj.Spec.Template.Spec.Bootstrap.ConfigRef == nil && newObj.Spec.Template.Spec.Bootstrap.DataSecretName == nil {
		allErrs = append(
			allErrs,
			field.Required(
				specPath.Child("template", "spec", "bootstrap", "data"),
				"expected either spec.bootstrap.dataSecretName or spec.bootstrap.configRef to be populated",
			),
		)
	}

	if newObj.Spec.Template.Spec.Bootstrap.ConfigRef != nil && newObj.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace != newObj.Namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				specPath.Child("template", "spec", "bootstrap", "configRef", "namespace"),
				newObj.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace,
				"must match metadata.namespace",
			),
		)
	}

	if newObj.Spec.Template.Spec.InfrastructureRef.Namespace != newObj.Namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				specPath.Child("infrastructureRef", "namespace"),
				newObj.Spec.Template.Spec.InfrastructureRef.Namespace,
				"must match metadata.namespace",
			),
		)
	}

	if oldObj != nil && oldObj.Spec.ClusterName != newObj.Spec.ClusterName {
		allErrs = append(
			allErrs,
			field.Forbidden(
				specPath.Child("clusterName"),
				"field is immutable"),
		)
	}

	if newObj.Spec.Template.Spec.Version != nil {
		if !version.KubeSemver.MatchString(*newObj.Spec.Template.Spec.Version) {
			allErrs = append(allErrs, field.Invalid(specPath.Child("template", "spec", "version"), *newObj.Spec.Template.Spec.Version, "must be a valid semantic version"))
		}
	}

	// Validate the metadata of the MachinePool template.
	allErrs = append(allErrs, newObj.Spec.Template.ObjectMeta.Validate(specPath.Child("template", "metadata"))...)

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("MachinePool").GroupKind(), newObj.Name, allErrs)
}
