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
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/labels"
	"sigs.k8s.io/cluster-api/util/version"
)

const defaultNodeDeletionTimeout = 10 * time.Second

func (webhook *Machine) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&clusterv1.Machine{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-cluster-x-k8s-io-v1beta1-machine,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machines,versions=v1beta1,name=validation.machine.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1beta1-machine,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machines,versions=v1beta1,name=default.machine.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// Machine implements a validation and defaulting webhook for Machine.
type Machine struct{}

var _ webhook.CustomValidator = &Machine{}
var _ webhook.CustomDefaulter = &Machine{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (webhook *Machine) Default(_ context.Context, obj runtime.Object) error {
	m, ok := obj.(*clusterv1.Machine)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Machine but got a %T", obj))
	}

	if m.Labels == nil {
		m.Labels = make(map[string]string)
	}
	m.Labels[clusterv1.ClusterNameLabel] = m.Spec.ClusterName

	if m.Spec.Bootstrap.ConfigRef != nil && m.Spec.Bootstrap.ConfigRef.Namespace == "" {
		m.Spec.Bootstrap.ConfigRef.Namespace = m.Namespace
	}

	if m.Spec.InfrastructureRef.Namespace == "" {
		m.Spec.InfrastructureRef.Namespace = m.Namespace
	}

	if m.Spec.Version != nil && !strings.HasPrefix(*m.Spec.Version, "v") {
		normalizedVersion := "v" + *m.Spec.Version
		m.Spec.Version = &normalizedVersion
	}

	if m.Spec.NodeDeletionTimeout == nil {
		m.Spec.NodeDeletionTimeout = &metav1.Duration{Duration: defaultNodeDeletionTimeout}
	}

	return nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Machine) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	m, ok := obj.(*clusterv1.Machine)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a Machine but got a %T", obj))
	}

	return nil, webhook.validate(nil, m)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (webhook *Machine) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldM, ok := oldObj.(*clusterv1.Machine)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a Machine but got a %T", oldObj))
	}

	newM, ok := newObj.(*clusterv1.Machine)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a Machine but got a %T", newObj))
	}

	return nil, webhook.validate(oldM, newM)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *Machine) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (webhook *Machine) validate(oldM, newM *clusterv1.Machine) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")
	if newM.Spec.Bootstrap.ConfigRef == nil && newM.Spec.Bootstrap.DataSecretName == nil {
		// MachinePool Machines don't have a bootstrap configRef, so don't require it. The bootstrap config is instead owned by the MachinePool.
		if !labels.IsMachinePoolOwned(newM) {
			allErrs = append(
				allErrs,
				field.Required(
					specPath.Child("bootstrap", "data"),
					"expected either spec.bootstrap.dataSecretName or spec.bootstrap.configRef to be populated",
				),
			)
		}
	}

	if newM.Spec.Bootstrap.ConfigRef != nil && newM.Spec.Bootstrap.ConfigRef.Namespace != newM.Namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				specPath.Child("bootstrap", "configRef", "namespace"),
				newM.Spec.Bootstrap.ConfigRef.Namespace,
				"must match metadata.namespace",
			),
		)
	}

	if newM.Spec.InfrastructureRef.Namespace != newM.Namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				specPath.Child("infrastructureRef", "namespace"),
				newM.Spec.InfrastructureRef.Namespace,
				"must match metadata.namespace",
			),
		)
	}

	if oldM != nil && oldM.Spec.ClusterName != newM.Spec.ClusterName {
		allErrs = append(
			allErrs,
			field.Forbidden(specPath.Child("clusterName"), "field is immutable"),
		)
	}

	if newM.Spec.Version != nil {
		if !version.KubeSemver.MatchString(*newM.Spec.Version) {
			allErrs = append(allErrs, field.Invalid(specPath.Child("version"), *newM.Spec.Version, "must be a valid semantic version"))
		}
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("Machine").GroupKind(), newM.Name, allErrs)
}
