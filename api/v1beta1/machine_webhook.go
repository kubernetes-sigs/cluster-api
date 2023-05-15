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
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"sigs.k8s.io/cluster-api/util/version"
)

const defaultNodeDeletionTimeout = 10 * time.Second

func (m *Machine) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&Machine{}).
		WithValidator(MachineValidator(mgr.GetScheme())).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-cluster-x-k8s-io-v1beta1-machine,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machines,versions=v1beta1,name=validation.machine.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1beta1-machine,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machines,versions=v1beta1,name=default.machine.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.CustomValidator = &machineValidator{}
var _ webhook.Defaulter = &Machine{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (m *Machine) Default() {
	if m.Labels == nil {
		m.Labels = make(map[string]string)
	}
	m.Labels[ClusterNameLabel] = m.Spec.ClusterName

	if m.Spec.Bootstrap.ConfigRef != nil && m.Spec.Bootstrap.ConfigRef.Namespace == "" {
		m.Spec.Bootstrap.ConfigRef.Namespace = m.Namespace
	}

	if m.Spec.InfrastructureRef.Namespace == "" {
		// Don't autofill namespace for MachinePool Machines since the infraRef will be populated by the MachinePool controller.
		if !isMachinePoolMachine(m) {
			m.Spec.InfrastructureRef.Namespace = m.Namespace
		}
	}

	if m.Spec.Version != nil && !strings.HasPrefix(*m.Spec.Version, "v") {
		normalizedVersion := "v" + *m.Spec.Version
		m.Spec.Version = &normalizedVersion
	}

	if m.Spec.NodeDeletionTimeout == nil {
		m.Spec.NodeDeletionTimeout = &metav1.Duration{Duration: defaultNodeDeletionTimeout}
	}
}

// MachineValidator creates a new CustomValidator for Machines.
func MachineValidator(_ *runtime.Scheme) webhook.CustomValidator {
	return &machineValidator{}
}

// machineValidator implements a defaulting webhook for Machine.
type machineValidator struct{}

// // ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
// func (m *Machine) ValidateCreate() (admission.Warnings, error) {
// 	return nil, m.validate(nil)
// }

// // ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
// func (m *Machine) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
// 	oldM, ok := old.(*Machine)
// 	if !ok {
// 		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a Machine but got a %T", old))
// 	}
// 	return nil, m.validate(oldM)
// }

// // ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
//
//	func (m *Machine) ValidateDelete() (admission.Warnings, error) {
//		return nil, nil
func (*machineValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	m, ok := obj.(*Machine)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a Machine but got a %T", obj))
	}

	return nil, m.validate(nil)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (*machineValidator) ValidateUpdate(_ context.Context, oldObj runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	newM, ok := newObj.(*Machine)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a Machine but got a %T", newObj))
	}

	oldM, ok := oldObj.(*Machine)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a Machine but got a %T", oldObj))
	}
	return nil, newM.validate(oldM)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (*machineValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	m, ok := obj.(*Machine)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a Machine but got a %T", obj))
	}

	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a admission.Request inside context: %v", err))
	}

	// Fallback machines are placeholders for InfraMachinePools that do not support MachinePool Machines. These have
	// no bootstrap or infrastructure data and cannot be deleted by users. They instead exist to provide a consistent
	// user experience for MachinePool Machines.
	if _, isFallbackMachine := m.Labels[FallbackMachineLabel]; isFallbackMachine {
		// Only allow the request if it is coming from the CAPI controller service account.
		if req.UserInfo.Username != "system:serviceaccount:"+os.Getenv("POD_NAMESPACE")+":"+os.Getenv("POD_SERVICE_ACCOUNT") {
			return nil, apierrors.NewBadRequest("this Machine is a placeholder for InfraMachinePools that do not support MachinePool Machines and cannot be deleted by users, scale down the MachinePool instead to delete")
		}
	}

	return nil, nil
}

func (m *Machine) validate(old *Machine) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")
	if m.Spec.Bootstrap.ConfigRef == nil && m.Spec.Bootstrap.DataSecretName == nil {
		// MachinePool Machines don't have a bootstrap configRef, so don't require it. The bootstrap config is instead owned by the MachinePool.
		if !isMachinePoolMachine(m) {
			allErrs = append(
				allErrs,
				field.Required(
					specPath.Child("bootstrap", "data"),
					"expected either spec.bootstrap.dataSecretName or spec.bootstrap.configRef to be populated",
				),
			)
		}
	}

	if m.Spec.Bootstrap.ConfigRef != nil && m.Spec.Bootstrap.ConfigRef.Namespace != m.Namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				specPath.Child("bootstrap", "configRef", "namespace"),
				m.Spec.Bootstrap.ConfigRef.Namespace,
				"must match metadata.namespace",
			),
		)
	}

	// InfraRef can be empty for MachinePool Machines so skip the check in that case.
	if !isMachinePoolMachine(m) {
		if m.Spec.InfrastructureRef.Namespace != m.Namespace {
			allErrs = append(
				allErrs,
				field.Invalid(
					specPath.Child("infrastructureRef", "namespace"),
					m.Spec.InfrastructureRef.Namespace,
					"must match metadata.namespace",
				),
			)
		}
	}

	if old != nil && old.Spec.ClusterName != m.Spec.ClusterName {
		allErrs = append(
			allErrs,
			field.Forbidden(specPath.Child("clusterName"), "field is immutable"),
		)
	}

	if m.Spec.Version != nil {
		if !version.KubeSemver.MatchString(*m.Spec.Version) {
			allErrs = append(allErrs, field.Invalid(specPath.Child("version"), *m.Spec.Version, "must be a valid semantic version"))
		}
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(GroupVersion.WithKind("Machine").GroupKind(), m.Name, allErrs)
}

func isMachinePoolMachine(m *Machine) bool {
	if m.OwnerReferences == nil {
		return false
	}

	for _, owner := range m.OwnerReferences {
		if owner.Kind == "MachinePool" {
			return true
		}
	}

	return false
}
