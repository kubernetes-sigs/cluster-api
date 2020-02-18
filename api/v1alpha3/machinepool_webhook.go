/*
Copyright 2019 The Kubernetes Authors.

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

package v1alpha3

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func (m *MachinePool) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(m).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-cluster-x-k8s-io-v1alpha3-machinepool,mutating=false,failurePolicy=fail,groups=cluster.x-k8s.io,resources=machinepools,versions=v1alpha3,name=validation.machinepool.cluster.x-k8s.io
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1alpha3-machinepool,mutating=true,failurePolicy=fail,groups=cluster.x-k8s.io,resources=machinepools,versions=v1alpha3,name=default.machinepool.cluster.x-k8s.io

var _ webhook.Defaulter = &MachinePool{}
var _ webhook.Validator = &MachinePool{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (m *MachinePool) Default() {
	if m.Spec.Template.Spec.Bootstrap.ConfigRef != nil && len(m.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace) == 0 {
		m.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace = m.Namespace
	}

	if len(m.Spec.Template.Spec.InfrastructureRef.Namespace) == 0 {
		m.Spec.Template.Spec.InfrastructureRef.Namespace = m.Namespace
	}
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (m *MachinePool) ValidateCreate() error {
	return m.validate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (m *MachinePool) ValidateUpdate(old runtime.Object) error {
	return m.validate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (m *MachinePool) ValidateDelete() error {
	return m.validate()
}

func (m *MachinePool) validate() error {
	var allErrs field.ErrorList
	if m.Spec.Template.Spec.Bootstrap.ConfigRef == nil && m.Spec.Template.Spec.Bootstrap.DataSecretName == nil {
		allErrs = append(
			allErrs,
			field.Required(
				field.NewPath("spec", "template", "spec", "bootstrap", "data"),
				"expected either spec.bootstrap.dataSecretName or spec.bootstrap.configRef to be populated",
			),
		)
	}

	if m.Spec.Template.Spec.Bootstrap.ConfigRef != nil && m.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace != m.Namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				field.NewPath("spec", "template", "spec", "bootstrap", "configRef", "namespace"),
				m.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace,
				"must match metadata.namespace",
			),
		)
	}

	if m.Spec.Template.Spec.InfrastructureRef.Namespace != m.Namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				field.NewPath("spec", "infrastructureRef", "namespace"),
				m.Spec.Template.Spec.InfrastructureRef.Namespace,
				"must match metadata.namespace",
			),
		)
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(GroupVersion.WithKind("MachinePool").GroupKind(), m.Name, allErrs)
}
