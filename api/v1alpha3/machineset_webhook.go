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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func (m *MachineSet) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(m).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-cluster-x-k8s-io-v1alpha3-machineset,mutating=false,failurePolicy=fail,groups=cluster.x-k8s.io,resources=machinesets,versions=v1alpha3,name=validation.machineset.cluster.x-k8s.io

var _ webhook.Validator = &MachineSet{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (m *MachineSet) ValidateCreate() error {
	return m.validate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (m *MachineSet) ValidateUpdate(old runtime.Object) error {
	return m.validate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (m *MachineSet) ValidateDelete() error {
	return nil
}

func (m *MachineSet) validate() error {
	var allErrs field.ErrorList
	selector, err := metav1.LabelSelectorAsSelector(&m.Spec.Selector)
	if err != nil {
		allErrs = append(
			allErrs,
			field.Invalid(field.NewPath("spec", "selector"), m.Spec.Selector, err.Error()),
		)
	} else if !selector.Matches(labels.Set(m.Spec.Template.Labels)) {
		allErrs = append(
			allErrs,
			field.Invalid(
				field.NewPath("spec", "template", "labels"),
				m.Spec.Template.Labels,
				fmt.Sprintf("must match spec.selector %q", selector.String()),
			),
		)
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(GroupVersion.WithKind("MachineSet").GroupKind(), m.Name, allErrs)
}
