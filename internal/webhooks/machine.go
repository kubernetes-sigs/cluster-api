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

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/labels"
	"sigs.k8s.io/cluster-api/util/version"
)

const defaultNodeDeletionTimeoutSeconds = int32(10)

func (webhook *Machine) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&clusterv1.Machine{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-cluster-x-k8s-io-v1beta2-machine,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machines,versions=v1beta2,name=validation.machine.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1beta2-machine,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=machines,versions=v1beta2,name=default.machine.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

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

	if m.Spec.Version != "" && !strings.HasPrefix(m.Spec.Version, "v") {
		normalizedVersion := "v" + m.Spec.Version
		m.Spec.Version = normalizedVersion
	}

	if m.Spec.Deletion.NodeDeletionTimeoutSeconds == nil {
		m.Spec.Deletion.NodeDeletionTimeoutSeconds = ptr.To(defaultNodeDeletionTimeoutSeconds)
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
	if !newM.Spec.Bootstrap.ConfigRef.IsDefined() && newM.Spec.Bootstrap.DataSecretName == nil {
		// MachinePool Machines don't have a bootstrap configRef, so don't require it. The bootstrap config is instead owned by the MachinePool.
		if !labels.IsMachinePoolOwned(newM) {
			allErrs = append(
				allErrs,
				field.Required(
					specPath.Child("bootstrap"),
					"expected either spec.bootstrap.dataSecretName or spec.bootstrap.configRef to be populated",
				),
			)
		}
	}

	if oldM != nil && oldM.Spec.ClusterName != newM.Spec.ClusterName {
		allErrs = append(
			allErrs,
			field.Forbidden(specPath.Child("clusterName"), "field is immutable"),
		)
	}

	if newM.Spec.Version != "" {
		if !version.KubeSemver.MatchString(newM.Spec.Version) {
			allErrs = append(allErrs, field.Invalid(specPath.Child("version"), newM.Spec.Version, "must be a valid semantic version"))
		}
	}

	allErrs = append(allErrs, validateMachineTaints(newM.Spec.Taints, specPath.Child("taints"))...)
	allErrs = append(allErrs, validateMachineTaintsForWorkers(newM.Spec.Taints, newM, specPath.Child("taints"))...)

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("Machine").GroupKind(), newM.Name, allErrs)
}

func validateMachineTaints(taints []clusterv1.MachineTaint, taintsPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if !feature.Gates.Enabled(feature.MachineTaintPropagation) {
		if len(taints) > 0 {
			allErrs = append(allErrs, field.Forbidden(taintsPath, "taints are not allowed to be set when the feature gate MachineTaintPropagation is disabled"))
		}
	}

	for i, taint := range taints {
		idxPath := taintsPath.Index(i)

		// The following validations uses a switch statement, because if one of them matches, then the others won't.

		switch {
		// Validate for keys which are reserved for usage by the cluster-api or providers.
		case taint.Key == clusterv1.NodeUninitializedTaint.Key:
			allErrs = append(allErrs, field.Invalid(idxPath.Child("key"), taint.Key, "taint key is not allowed"))
		case taint.Key == clusterv1.NodeOutdatedRevisionTaint.Key:
			allErrs = append(allErrs, field.Invalid(idxPath.Child("key"), taint.Key, "taint key is not allowed"))
		// Validate for keys which are reserved for usage by the node or node-lifecycle-controller, but allow `node.kubernetes.io/out-of-service`.
		case strings.HasPrefix(taint.Key, "node.kubernetes.io/") && taint.Key != "node.kubernetes.io/out-of-service":
			allErrs = append(allErrs, field.Invalid(idxPath.Child("key"), taint.Key, "taint key must not have the prefix node.kubernetes.io/, except for node.kubernetes.io/out-of-service"))
		// Validate for keys which are reserved for usage by the cloud-controller-manager or kubelet.
		case strings.HasPrefix(taint.Key, "node.cloudprovider.kubernetes.io/"):
			allErrs = append(allErrs, field.Invalid(idxPath.Child("key"), taint.Key, "taint key must not have the prefix node.cloudprovider.kubernetes.io/"))
		// Validate for the deprecated kubeadm node-role taint.
		case taint.Key == "node-role.kubernetes.io/master":
			allErrs = append(allErrs, field.Invalid(idxPath.Child("key"), taint.Key, "taint is deprecated since 1.24 and should not be used anymore"))
		}
	}

	return allErrs
}

func validateMachineTaintsForWorkers(taints []clusterv1.MachineTaint, machine *clusterv1.Machine, taintsPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if !feature.Gates.Enabled(feature.MachineTaintPropagation) {
		return allErrs
	}

	// Skip for control-plane machines.
	if machine != nil && machine.Labels != nil {
		if _, ok := machine.Labels[clusterv1.MachineControlPlaneLabel]; ok {
			return allErrs
		}
	}

	// Validate taints for worker machines.
	for i, taint := range taints {
		idxPath := taintsPath.Index(i)

		if taint.Key == "node-role.kubernetes.io/control-plane" {
			allErrs = append(allErrs, field.Invalid(idxPath.Child("key"), taint.Key, "taint is not allowed for worker Machines"))
		}
	}

	return allErrs
}
