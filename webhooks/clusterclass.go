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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/topology/check"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func (webhook *ClusterClass) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&clusterv1.ClusterClass{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-cluster-x-k8s-io-v1beta1-clusterclass,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=clusterclasses,versions=v1beta1,name=validation.clusterclass.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1beta1-clusterclass,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=clusterclasses,versions=v1beta1,name=default.clusterclass.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// ClusterClass implements a validation and defaulting webhook for ClusterClass.
type ClusterClass struct{}

var _ webhook.CustomDefaulter = &ClusterClass{}
var _ webhook.CustomValidator = &ClusterClass{}

// Default implements defaulting for ClusterClass create and update.
func (webhook *ClusterClass) Default(_ context.Context, obj runtime.Object) error {
	in, ok := obj.(*clusterv1.ClusterClass)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterClass but got a %T", obj))
	}
	// Default all namespaces in the references to the object namespace.
	defaultNamespace(in.Spec.Infrastructure.Ref, in.Namespace)
	defaultNamespace(in.Spec.ControlPlane.Ref, in.Namespace)

	if in.Spec.ControlPlane.MachineInfrastructure != nil {
		defaultNamespace(in.Spec.ControlPlane.MachineInfrastructure.Ref, in.Namespace)
	}

	for i := range in.Spec.Workers.MachineDeployments {
		defaultNamespace(in.Spec.Workers.MachineDeployments[i].Template.Bootstrap.Ref, in.Namespace)
		defaultNamespace(in.Spec.Workers.MachineDeployments[i].Template.Infrastructure.Ref, in.Namespace)
	}
	return nil
}

func defaultNamespace(ref *corev1.ObjectReference, namespace string) {
	if ref != nil && len(ref.Namespace) == 0 {
		ref.Namespace = namespace
	}
}

// ValidateCreate implements validation for ClusterClass create.
func (webhook *ClusterClass) ValidateCreate(_ context.Context, obj runtime.Object) error {
	in, ok := obj.(*clusterv1.ClusterClass)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterClass but got a %T", obj))
	}
	return webhook.validate(nil, in)
}

// ValidateUpdate implements validation for ClusterClass update.
func (webhook *ClusterClass) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) error {
	newClusterClass, ok := newObj.(*clusterv1.ClusterClass)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterClass but got a %T", newObj))
	}
	oldClusterClass, ok := oldObj.(*clusterv1.ClusterClass)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterClass but got a %T", oldObj))
	}
	return webhook.validate(oldClusterClass, newClusterClass)
}

// ValidateDelete implements validation for ClusterClass delete.
func (webhook *ClusterClass) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	return nil
}

func (webhook *ClusterClass) validate(old, new *clusterv1.ClusterClass) error {
	// NOTE: ClusterClass and managed topologies are behind ClusterTopology feature gate flag; the web hook
	// must prevent creating new objects new case the feature flag is disabled.
	if !feature.Gates.Enabled(feature.ClusterTopology) {
		return field.Forbidden(
			field.NewPath("spec"),
			"can be set only if the ClusterTopology feature flag is enabled",
		)
	}

	var allErrs field.ErrorList

	// Ensure all references are valid.
	allErrs = append(allErrs, webhook.validateAllRefs(new)...)

	// Ensure all MachineDeployment classes are unique.
	allErrs = append(allErrs, webhook.validateUniqueClasses(new.Spec.Workers, field.NewPath("spec", "workers"))...)

	// Ensure spec changes are compatible.
	allErrs = append(allErrs, webhook.validateCompatibleSpecChanges(old, new)...)

	if len(allErrs) > 0 {
		return apierrors.NewInvalid(clusterv1.GroupVersion.WithKind("ClusterClass").GroupKind(), new.Name, allErrs)
	}
	return nil
}

func (webhook *ClusterClass) validateAllRefs(clusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, check.LocalObjectTemplateIsValid(&clusterClass.Spec.Infrastructure, clusterClass.Namespace, field.NewPath("spec", "infrastructure"))...)
	allErrs = append(allErrs, check.LocalObjectTemplateIsValid(&clusterClass.Spec.ControlPlane.LocalObjectTemplate, clusterClass.Namespace, field.NewPath("spec", "controlPlane"))...)
	if clusterClass.Spec.ControlPlane.MachineInfrastructure != nil {
		allErrs = append(allErrs, check.LocalObjectTemplateIsValid(clusterClass.Spec.ControlPlane.MachineInfrastructure, clusterClass.Namespace, field.NewPath("spec", "controlPlane", "machineInfrastructure"))...)
	}

	for i, class := range clusterClass.Spec.Workers.MachineDeployments {
		allErrs = append(allErrs, check.LocalObjectTemplateIsValid(&class.Template.Bootstrap, clusterClass.Namespace, field.NewPath("spec", "workers", "machineDeployments").Index(i).Child("template", "bootstrap"))...)
		allErrs = append(allErrs, check.LocalObjectTemplateIsValid(&class.Template.Infrastructure, clusterClass.Namespace, field.NewPath("spec", "workers", "machineDeployments").Index(i).Child("template", "infrastructure"))...)
	}

	return allErrs
}

func (webhook *ClusterClass) validateCompatibleSpecChanges(old, new *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList

	// new case of create, no changes to verify
	// return early.
	if old == nil {
		return nil
	}

	// Validate changes to MachineDeployments.
	allErrs = append(allErrs, webhook.validateMachineDeploymentsCompatibleChanges(old, new)...)

	// Validate InfrastructureClusterTemplate changes new a compatible way.
	allErrs = append(allErrs, check.LocalObjectTemplatesAreCompatible(old.Spec.Infrastructure, new.Spec.Infrastructure)...)

	// Validate control plane changes new a compatible way.
	allErrs = append(allErrs, check.LocalObjectTemplatesAreCompatible(old.Spec.ControlPlane.LocalObjectTemplate, new.Spec.ControlPlane.LocalObjectTemplate)...)

	if new.Spec.ControlPlane.MachineInfrastructure != nil && old.Spec.ControlPlane.MachineInfrastructure != nil {
		allErrs = append(allErrs, check.LocalObjectTemplatesAreCompatible(*old.Spec.ControlPlane.MachineInfrastructure, *new.Spec.ControlPlane.MachineInfrastructure)...)
	}

	return allErrs
}

func (webhook *ClusterClass) validateMachineDeploymentsCompatibleChanges(old, new *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList

	// Ensure no MachineDeployment class was removed.
	classes := webhook.classNamesFromWorkerClass(new.Spec.Workers)
	for _, oldClass := range old.Spec.Workers.MachineDeployments {
		if !classes.Has(oldClass.Class) {
			allErrs = append(allErrs,
				field.Invalid(
					field.NewPath("spec", "workers", "machineDeployments"),
					new.Spec.Workers.MachineDeployments,
					fmt.Sprintf("The %q MachineDeployment class can't be removed.", oldClass.Class),
				),
			)
		}
	}

	// Ensure previous MachineDeployment class was modified new a compatible way.
	for _, class := range new.Spec.Workers.MachineDeployments {
		for _, oldClass := range old.Spec.Workers.MachineDeployments {
			if class.Class == oldClass.Class {
				// NOTE: class.Template.Metadata and class.Template.Bootstrap are allowed to change;
				// class.Template.Bootstrap are ensured syntactically correct by validateAllRefs.

				// Validates class.Template.Infrastructure template changes new a compatible way
				allErrs = append(allErrs, check.LocalObjectTemplatesAreCompatible(oldClass.Template.Infrastructure, class.Template.Infrastructure)...)
			}
		}
	}

	return allErrs
}

// classNames returns the set of MachineDeployment class names.
func (webhook *ClusterClass) classNamesFromWorkerClass(w clusterv1.WorkersClass) sets.String {
	classes := sets.NewString()
	for _, class := range w.MachineDeployments {
		classes.Insert(class.Class)
	}
	return classes
}

func (webhook *ClusterClass) validateUniqueClasses(w clusterv1.WorkersClass, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	classes := sets.NewString()
	for i, class := range w.MachineDeployments {
		if classes.Has(class.Class) {
			allErrs = append(allErrs,
				field.Invalid(
					pathPrefix.Child("machineDeployments").Index(i).Child("class"),
					class.Class,
					fmt.Sprintf("MachineDeployment class should be unique. MachineDeployment with class %q is defined more than once.", class.Class),
				),
			)
		}
		classes.Insert(class.Class)
	}

	return allErrs
}
