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
	"fmt"
	"k8s.io/utils/pointer"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func (m *MachinePool) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(m).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-exp-cluster-x-k8s-io-v1alpha3-machinepool,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=exp.cluster.x-k8s.io,resources=machinepools,versions=v1alpha3,name=validation.exp.machinepool.cluster.x-k8s.io,sideEffects=None
// +kubebuilder:webhook:verbs=create;update,path=/mutate-exp-cluster-x-k8s-io-v1alpha3-machinepool,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=exp.cluster.x-k8s.io,resources=machinepools,versions=v1alpha3,name=default.exp.machinepool.cluster.x-k8s.io,sideEffects=None

var _ webhook.Defaulter = &MachinePool{}
var _ webhook.Validator = &MachinePool{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (m *MachinePool) Default() {
	if m.Labels == nil {
		m.Labels = make(map[string]string)
	}
	m.Labels[clusterv1.ClusterLabelName] = m.Spec.ClusterName

	if m.Spec.Replicas == nil {
		m.Spec.Replicas = pointer.Int32Ptr(1)
	}

	if m.Spec.MinReadySeconds == nil {
		m.Spec.MinReadySeconds = pointer.Int32Ptr(0)
	}

	if m.Spec.Template.Spec.Bootstrap.ConfigRef != nil && len(m.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace) == 0 {
		m.Spec.Template.Spec.Bootstrap.ConfigRef.Namespace = m.Namespace
	}

	if len(m.Spec.Template.Spec.InfrastructureRef.Namespace) == 0 {
		m.Spec.Template.Spec.InfrastructureRef.Namespace = m.Namespace
	}
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (m *MachinePool) ValidateCreate() error {
	return m.validate(nil)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (m *MachinePool) ValidateUpdate(old runtime.Object) error {
	oldMP, ok := old.(*MachinePool)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a MachinePool but got a %T", old))
	}
	return m.validate(oldMP)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (m *MachinePool) ValidateDelete() error {
	return m.validate(nil)
}

func (m *MachinePool) validate(old *MachinePool) error {
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

	if old != nil && old.Spec.ClusterName != m.Spec.ClusterName {
		allErrs = append(
			allErrs,
			field.Invalid(field.NewPath("spec", "clusterName"), m.Spec.ClusterName, "field is immutable"),
		)
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(GroupVersion.WithKind("MachinePool").GroupKind(), m.Name, allErrs)
}
