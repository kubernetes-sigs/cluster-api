/*
Copyright 2024 The Kubernetes Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func (webhook *MachineDrainRule) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&clusterv1.MachineDrainRule{}).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-cluster-x-k8s-io-v1beta1-machinedrainrule,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machinedrainrules,versions=v1beta1,name=validation.machinedrainrule.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// MachineDrainRule implements a validation webhook for MachineDrainRule.
type MachineDrainRule struct{}

var _ webhook.CustomValidator = &MachineDrainRule{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *MachineDrainRule) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	mdr, ok := obj.(*clusterv1.MachineDrainRule)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a MachineDrainRule but got a %T", obj))
	}

	return nil, webhook.validate(mdr)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *MachineDrainRule) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	newMDR, ok := newObj.(*clusterv1.MachineDrainRule)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a MachineDrainRule but got a %T", newObj))
	}

	return nil, webhook.validate(newMDR)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *MachineDrainRule) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (webhook *MachineDrainRule) validate(newMDR *clusterv1.MachineDrainRule) error {
	var allErrs field.ErrorList

	if newMDR.Spec.Drain.Behavior == clusterv1.MachineDrainRuleDrainBehaviorSkip ||
		newMDR.Spec.Drain.Behavior == clusterv1.MachineDrainRuleDrainBehaviorWaitCompleted {
		if newMDR.Spec.Drain.Order != nil {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec", "drain", "order"),
					*newMDR.Spec.Drain.Order,
					fmt.Sprintf("order must not be set if drain behavior is %q or %q",
						clusterv1.MachineDrainRuleDrainBehaviorSkip, clusterv1.MachineDrainRuleDrainBehaviorWaitCompleted),
				),
			)
		}
	}

	allErrs = append(allErrs, ValidateMachineDrainRulesSelectors(newMDR)...)

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("MachineDrainRule").GroupKind(), newMDR.Name, allErrs)
}

// ValidateMachineDrainRulesSelectors validate the selectors of a MachineDrainRule.
// Note: This func is exported so it can be also used to validate selectors in the Machine controller.
func ValidateMachineDrainRulesSelectors(machineDrainRule *clusterv1.MachineDrainRule) field.ErrorList {
	var allErrs field.ErrorList

	for i, machineSelector := range machineDrainRule.Spec.Machines {
		if machineSelector.Selector != nil {
			if _, err := metav1.LabelSelectorAsSelector(machineSelector.Selector); err != nil {
				allErrs = append(allErrs,
					field.Invalid(field.NewPath("spec", "machines").Index(i).Child("selector"), machineSelector.Selector, err.Error()),
				)
			}
		}
		if machineSelector.ClusterSelector != nil {
			if _, err := metav1.LabelSelectorAsSelector(machineSelector.ClusterSelector); err != nil {
				allErrs = append(allErrs,
					field.Invalid(field.NewPath("spec", "machines").Index(i).Child("clusterSelector"), machineSelector.ClusterSelector, err.Error()),
				)
			}
		}
	}

	for i, podSelector := range machineDrainRule.Spec.Pods {
		if podSelector.Selector != nil {
			if _, err := metav1.LabelSelectorAsSelector(podSelector.Selector); err != nil {
				allErrs = append(allErrs,
					field.Invalid(field.NewPath("spec", "pods").Index(i).Child("selector"), podSelector.Selector, err.Error()),
				)
			}
		}
		if podSelector.NamespaceSelector != nil {
			if _, err := metav1.LabelSelectorAsSelector(podSelector.NamespaceSelector); err != nil {
				allErrs = append(allErrs,
					field.Invalid(field.NewPath("spec", "pods").Index(i).Child("namespaceSelector"), podSelector.NamespaceSelector, err.Error()),
				)
			}
		}
	}

	return allErrs
}
