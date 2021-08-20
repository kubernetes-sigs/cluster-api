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

package v1alpha4

import (
	"fmt"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/cluster-api/feature"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func (in *ClusterClass) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(in).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-cluster-x-k8s-io-v1alpha4-clusterclass,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=clusterclasses,versions=v1alpha4,name=validation.clusterclass.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-cluster-x-k8s-io-v1alpha4-clusterclass,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=cluster.x-k8s.io,resources=clusterclasses,versions=v1alpha4,name=default.clusterclass.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.Validator = &ClusterClass{}
var _ webhook.Defaulter = &ClusterClass{}

// Default satisfies the defaulting webhook interface.
func (in *ClusterClass) Default() {
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
}

func defaultNamespace(ref *corev1.ObjectReference, namespace string) {
	if ref != nil && len(ref.Namespace) == 0 {
		ref.Namespace = namespace
	}
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (in *ClusterClass) ValidateCreate() error {
	return in.validate(nil)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (in *ClusterClass) ValidateUpdate(old runtime.Object) error {
	oldClusterClass, ok := old.(*ClusterClass)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a ClusterClass but got a %T", old))
	}
	return in.validate(oldClusterClass)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (in *ClusterClass) ValidateDelete() error {
	return nil
}

func (in *ClusterClass) validate(old *ClusterClass) error {
	// NOTE: ClusterClass and managed topologies are behind ClusterTopology feature gate flag; the web hook
	// must prevent creating new objects in case the feature flag is disabled.
	if !feature.Gates.Enabled(feature.ClusterTopology) {
		return field.Forbidden(
			field.NewPath("spec"),
			"can be set only if the ClusterTopology feature flag is enabled",
		)
	}

	var allErrs field.ErrorList

	// Ensure all references are valid.
	allErrs = append(allErrs, in.validateAllRefs()...)

	// Ensure all MachineDeployment classes are unique.
	allErrs = append(allErrs, in.Spec.Workers.validateUniqueClasses(field.NewPath("spec", "workers"))...)

	// Ensure spec changes are compatible.
	allErrs = append(allErrs, in.validateCompatibleSpecChanges(old)...)

	if len(allErrs) > 0 {
		return apierrors.NewInvalid(GroupVersion.WithKind("ClusterClass").GroupKind(), in.Name, allErrs)
	}
	return nil
}

func (in ClusterClass) validateAllRefs() field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, in.Spec.Infrastructure.validate(in.Namespace, field.NewPath("spec", "infrastructure"))...)
	allErrs = append(allErrs, in.Spec.ControlPlane.LocalObjectTemplate.validate(in.Namespace, field.NewPath("spec", "controlPlane"))...)
	if in.Spec.ControlPlane.MachineInfrastructure != nil {
		allErrs = append(allErrs, in.Spec.ControlPlane.MachineInfrastructure.validate(in.Namespace, field.NewPath("spec", "controlPlane", "machineInfrastructure"))...)
	}

	for i, class := range in.Spec.Workers.MachineDeployments {
		allErrs = append(allErrs, class.Template.Bootstrap.validate(in.Namespace, field.NewPath("spec", "workers", fmt.Sprintf("machineDeployments[%v]", i), "template", "bootstrap"))...)
		allErrs = append(allErrs, class.Template.Infrastructure.validate(in.Namespace, field.NewPath("spec", "workers", fmt.Sprintf("machineDeployments[%v]", i), "template", "infrastructure"))...)
	}

	return allErrs
}

func (in ClusterClass) validateCompatibleSpecChanges(old *ClusterClass) field.ErrorList {
	var allErrs field.ErrorList

	// in case of create, no changes to verify
	// return early.
	if old == nil {
		return nil
	}

	// Ensure that the old MachineDeployments still exist.
	allErrs = append(allErrs, in.validateMachineDeploymentsChanges(old)...)

	if !reflect.DeepEqual(in.Spec.Infrastructure, old.Spec.Infrastructure) {
		allErrs = append(allErrs,
			field.Invalid(
				field.NewPath("spec", "infrastructure"),
				in.Spec.Infrastructure,
				"cannot be changed.",
			),
		)
	}

	if !reflect.DeepEqual(in.Spec.ControlPlane, old.Spec.ControlPlane) {
		allErrs = append(allErrs,
			field.Invalid(
				field.NewPath("spec", "controlPlane"),
				in.Spec.Infrastructure,
				"cannot be changed.",
			),
		)
	}

	return allErrs
}

func (in ClusterClass) validateMachineDeploymentsChanges(old *ClusterClass) field.ErrorList {
	var allErrs field.ErrorList

	// Ensure no MachineDeployment class was removed.
	classes := in.Spec.Workers.classNames()
	for _, oldClass := range old.Spec.Workers.MachineDeployments {
		if !classes.Has(oldClass.Class) {
			allErrs = append(allErrs,
				field.Invalid(
					field.NewPath("spec", "workers", "machineDeployments"),
					in.Spec.Workers.MachineDeployments,
					fmt.Sprintf("The %q MachineDeployment class can't be removed.", oldClass.Class),
				),
			)
		}
	}

	// Ensure no previous MachineDeployment class was modified.
	for _, class := range in.Spec.Workers.MachineDeployments {
		for _, oldClass := range old.Spec.Workers.MachineDeployments {
			if class.Class == oldClass.Class && !reflect.DeepEqual(class, oldClass) {
				allErrs = append(allErrs,
					field.Invalid(
						field.NewPath("spec", "workers", "machineDeployments"),
						class,
						"cannot be changed.",
					),
				)
			}
		}
	}

	return allErrs
}

func (r LocalObjectTemplate) validate(namespace string, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// check if ref is not nil.
	if r.Ref == nil {
		return field.ErrorList{field.Invalid(
			pathPrefix.Child("ref"),
			r.Ref.Name,
			"cannot be nil",
		)}
	}

	// check if a name is provided
	if r.Ref.Name == "" {
		allErrs = append(allErrs,
			field.Invalid(
				pathPrefix.Child("ref", "name"),
				r.Ref.Name,
				"cannot be empty",
			),
		)
	}

	// validate if namespace matches the provided namespace
	if namespace != "" && r.Ref.Namespace != namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				pathPrefix.Child("ref", "namespace"),
				r.Ref.Namespace,
				fmt.Sprintf("must be '%s'", namespace),
			),
		)
	}

	// check if kind is a template
	if len(r.Ref.Kind) <= len(TemplateSuffix) || !strings.HasSuffix(r.Ref.Kind, TemplateSuffix) {
		allErrs = append(allErrs,
			field.Invalid(
				pathPrefix.Child("ref", "kind"),
				r.Ref.Kind,
				fmt.Sprintf("kind must be of form '<name>%s'", TemplateSuffix),
			),
		)
	}

	// check if apiVersion is valid
	gv, err := schema.ParseGroupVersion(r.Ref.APIVersion)
	if err != nil {
		allErrs = append(allErrs,
			field.Invalid(
				pathPrefix.Child("ref", "apiVersion"),
				r.Ref.APIVersion,
				fmt.Sprintf("must be a valid apiVersion: %v", err),
			),
		)
	}
	if err == nil && gv.Empty() {
		allErrs = append(allErrs,
			field.Invalid(
				pathPrefix.Child("ref", "apiVersion"),
				r.Ref.APIVersion,
				"cannot be empty",
			),
		)
	}

	return allErrs
}

// classNames returns the set of MachineDeployment class names.
func (w WorkersClass) classNames() sets.String {
	classes := sets.NewString()
	for _, class := range w.MachineDeployments {
		classes.Insert(class.Class)
	}
	return classes
}

func (w WorkersClass) validateUniqueClasses(pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	classes := sets.NewString()
	for i, class := range w.MachineDeployments {
		if classes.Has(class.Class) {
			allErrs = append(allErrs,
				field.Invalid(
					pathPrefix.Child(fmt.Sprintf("machineDeployments[%v]", i), "class"),
					class.Class,
					fmt.Sprintf("MachineDeployment class should be unique. MachineDeployment with class %q is defined more than once.", class.Class),
				),
			)
		}
		classes.Insert(class.Class)
	}

	return allErrs
}
