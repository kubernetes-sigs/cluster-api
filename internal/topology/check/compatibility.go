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

// Package check implements checks for managed topology.
package check

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ObjectsAreStrictlyCompatible checks if two referenced objects are strictly compatible, meaning that
// they are compatible and the name of the objects do not change.
func ObjectsAreStrictlyCompatible(current, desired client.Object) field.ErrorList {
	var allErrs field.ErrorList
	if current.GetName() != desired.GetName() {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("metadata", "name"),
			fmt.Sprintf("cannot be changed from %v to %v to prevent incompatible changes in %v/%v", current.GetName(), desired.GetName(), current.GetObjectKind().GroupVersionKind().GroupKind().String(), current.GetName()),
		))
	}
	allErrs = append(allErrs, ObjectsAreCompatible(current, desired)...)
	return allErrs
}

// ObjectsAreCompatible checks if two referenced objects are compatible, meaning that
// they are of the same GroupKind and in the same namespace.
func ObjectsAreCompatible(current, desired client.Object) field.ErrorList {
	var allErrs field.ErrorList

	currentGK := current.GetObjectKind().GroupVersionKind().GroupKind()
	desiredGK := desired.GetObjectKind().GroupVersionKind().GroupKind()
	if currentGK.Group != desiredGK.Group {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("metadata", "apiVersion"),
			fmt.Sprintf("group cannot be changed from %v to %v to prevent incompatible changes in %v/%v", currentGK.Group, desiredGK.Group, currentGK.String(), current.GetName()),
		))
	}
	if currentGK.Kind != desiredGK.Kind {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("metadata", "kind"),
			fmt.Sprintf("cannot be changed from %v to %v to prevent incompatible changes in %v/%v", currentGK.Kind, desiredGK.Kind, currentGK.String(), current.GetName()),
		))
	}
	allErrs = append(allErrs, ObjectsAreInTheSameNamespace(current, desired)...)
	return allErrs
}

// ObjectsAreInTheSameNamespace checks if two referenced objects are in the same namespace.
func ObjectsAreInTheSameNamespace(current, desired client.Object) field.ErrorList {
	var allErrs field.ErrorList

	// NOTE: this should never happen (webhooks prevent it), but checking for extra safety.
	if current.GetNamespace() != desired.GetNamespace() {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("metadata", "namespace"),
			fmt.Sprintf("cannot be changed from %v to %v to prevent incompatible changes in %v/%v", current.GetNamespace(), desired.GetNamespace(), current.GetObjectKind().GroupVersionKind().GroupKind().String(), current.GetName()),
		))
	}
	return allErrs
}

// LocalObjectTemplatesAreCompatible checks if two referenced objects are compatible, meaning that
// they are of the same GroupKind and in the same namespace.
func LocalObjectTemplatesAreCompatible(current, desired clusterv1.LocalObjectTemplate) field.ErrorList {
	var allErrs field.ErrorList

	currentGK := current.Ref.GetObjectKind().GroupVersionKind().GroupKind()
	desiredGK := desired.Ref.GetObjectKind().GroupVersionKind().GroupKind()

	if currentGK.Group != desiredGK.Group {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("metadata", "apiVersion"),
			fmt.Sprintf("group cannot be changed from %v to %v to prevent incompatible changes in %v/%v", currentGK.Group, desiredGK.Group, currentGK.String(), current.Ref.Name),
		))
	}
	if currentGK.Kind != desiredGK.Kind {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("metadata", "kind"),
			fmt.Sprintf("cannot be changed from %v to %v to prevent incompatible changes in %v/%v", currentGK.Kind, desiredGK.Kind, currentGK.String(), current.Ref.Name),
		))
	}
	allErrs = append(allErrs, LocalObjectTemplatesAreInSameNamespace(current, desired)...)
	return allErrs
}

// LocalObjectTemplatesAreInSameNamespace checks if two referenced objects are in the same namespace.
func LocalObjectTemplatesAreInSameNamespace(current, desired clusterv1.LocalObjectTemplate) field.ErrorList {
	var allErrs field.ErrorList
	if current.Ref.Namespace != desired.Ref.Namespace {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("metadata", "namespace"),
			fmt.Sprintf("cannot be changed from %v to %v to prevent incompatible changes in %v/%v", current.Ref.Namespace, desired.Ref.Namespace, current.Ref.GetObjectKind().GroupVersionKind().GroupKind().String(), current.Ref.Name),
		))
	}
	return allErrs
}

// LocalObjectTemplateIsValid ensures the template is in the correct namespace, has no nil references, and has a valid Kind and GroupVersion.
func LocalObjectTemplateIsValid(in *clusterv1.LocalObjectTemplate, namespace string, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// check if ref is not nil.
	if in.Ref == nil {
		return field.ErrorList{field.Invalid(
			pathPrefix.Child("ref"),
			"nil",
			"cannot be nil",
		)}
	}

	// check if a name is provided
	if in.Ref.Name == "" {
		allErrs = append(allErrs,
			field.Invalid(
				pathPrefix.Child("ref", "name"),
				in.Ref.Name,
				"cannot be empty",
			),
		)
	}

	// validate if namespace matches the provided namespace
	if namespace != "" && in.Ref.Namespace != namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				pathPrefix.Child("ref", "namespace"),
				in.Ref.Namespace,
				fmt.Sprintf("must be '%s'", namespace),
			),
		)
	}

	// check if kind is a template
	if len(in.Ref.Kind) <= len(clusterv1.TemplateSuffix) || !strings.HasSuffix(in.Ref.Kind, clusterv1.TemplateSuffix) {
		allErrs = append(allErrs,
			field.Invalid(
				pathPrefix.Child("ref", "kind"),
				in.Ref.Kind,
				fmt.Sprintf("kind must be of form '<name>%s'", clusterv1.TemplateSuffix),
			),
		)
	}

	// check if apiVersion is valid
	gv, err := schema.ParseGroupVersion(in.Ref.APIVersion)
	if err != nil {
		allErrs = append(allErrs,
			field.Invalid(
				pathPrefix.Child("ref", "apiVersion"),
				in.Ref.APIVersion,
				fmt.Sprintf("must be a valid apiVersion: %v", err),
			),
		)
	}
	if err == nil && gv.Empty() {
		allErrs = append(allErrs,
			field.Invalid(
				pathPrefix.Child("ref", "apiVersion"),
				in.Ref.APIVersion,
				"value cannot be empty",
			),
		)
	}
	return allErrs
}
