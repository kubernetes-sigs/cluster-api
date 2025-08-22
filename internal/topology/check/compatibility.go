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
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// ObjectsAreStrictlyCompatible checks if two referenced objects are strictly compatible, meaning that
// they are compatible and the name of the objects do not change.
func ObjectsAreStrictlyCompatible(current, desired client.Object) field.ErrorList {
	var allErrs field.ErrorList
	if current.GetName() != desired.GetName() {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("metadata", "name"),
			fmt.Sprintf("metadata.name of %s/%s cannot be changed from %q to %q to prevent incompatible changes in the Cluster",
				current.GetObjectKind().GroupVersionKind().GroupKind().String(), current.GetName(), current.GetName(), desired.GetName()),
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
			fmt.Sprintf("apiVersion.group of %s/%s cannot be changed from %q to %q to prevent incompatible changes in the Cluster",
				currentGK.String(), current.GetName(), currentGK.Group, desiredGK.Group),
		))
	}
	if currentGK.Kind != desiredGK.Kind {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("metadata", "kind"),
			fmt.Sprintf("apiVersion.kind of %s/%s cannot be changed from %q to %q to prevent incompatible changes in the Cluster",
				currentGK.String(), current.GetName(), currentGK.Kind, desiredGK.Kind),
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
			fmt.Sprintf("metadata.namespace of %s/%s cannot be changed from %q to %q because templates must be in the same namespace as the Cluster",
				current.GetObjectKind().GroupVersionKind().GroupKind().String(), current.GetName(), current.GetNamespace(), desired.GetNamespace()),
		))
	}
	return allErrs
}

// ClusterClassTemplateAreCompatible checks if two referenced objects are compatible, meaning that
// they are of the same GroupKind.
func ClusterClassTemplateAreCompatible(current, desired clusterv1.ClusterClassTemplateReference, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	currentGK := current.GroupVersionKind().GroupKind()
	desiredGK := desired.GroupVersionKind().GroupKind()

	if currentGK.Group != desiredGK.Group {
		allErrs = append(allErrs, field.Forbidden(
			pathPrefix.Child("apiVersion"),
			fmt.Sprintf("group of apiVersion cannot be changed from %q to %q to prevent incompatible changes in the Clusters",
				currentGK.Group, desiredGK.Group),
		))
	}
	if currentGK.Kind != desiredGK.Kind {
		allErrs = append(allErrs, field.Forbidden(
			pathPrefix.Child("kind"),
			fmt.Sprintf("kind cannot be changed from %q to %q to prevent incompatible changes in the Clusters",
				currentGK.Kind, desiredGK.Kind),
		))
	}
	return allErrs
}

// ClusterClassTemplateIsValid ensures the template has no nil references, and has a valid Kind and GroupVersion.
func ClusterClassTemplateIsValid(templateRef clusterv1.ClusterClassTemplateReference, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// check if a name is provided
	if templateRef.Name == "" {
		allErrs = append(allErrs,
			field.Required(
				pathPrefix.Child("ref", "name"),
				"template name must be defined",
			),
		)
	}

	// check if kind is a template
	if len(templateRef.Kind) <= len(clusterv1.TemplateSuffix) || !strings.HasSuffix(templateRef.Kind, clusterv1.TemplateSuffix) {
		allErrs = append(allErrs,
			field.Invalid(
				pathPrefix.Child("ref", "kind"),
				templateRef.Kind,
				fmt.Sprintf("template kind must be of form \"<name>%s\"", clusterv1.TemplateSuffix),
			),
		)
	}

	// check if apiVersion is valid
	gv, err := schema.ParseGroupVersion(templateRef.APIVersion)
	if err != nil {
		allErrs = append(allErrs,
			field.Invalid(
				pathPrefix.Child("ref", "apiVersion"),
				templateRef.APIVersion,
				fmt.Sprintf("template apiVersion must be a valid Kubernetes apiVersion: %v", err),
			),
		)
	}
	if err == nil && gv.Empty() {
		allErrs = append(allErrs,
			field.Required(
				pathPrefix.Child("ref", "apiVersion"),
				"template apiVersion must be defined",
			),
		)
	}
	return allErrs
}

// ClusterClassesAreCompatible checks the compatibility between new and old versions of a Cluster Class.
// It checks that:
// 1) InfrastructureCluster Templates are compatible.
// 2) ControlPlane Templates are compatible.
// 3) ControlPlane InfrastructureMachineTemplates are compatible.
// 4) MachineDeploymentClasses are compatible.
// 5) MachinePoolClasses are compatible.
func ClusterClassesAreCompatible(current, desired *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList
	if current == nil {
		return append(allErrs, field.Invalid(field.NewPath(""), "", "could not compare ClusterClass compatibility: current ClusterClass must not be nil"))
	}
	if desired == nil {
		return append(allErrs, field.Invalid(field.NewPath(""), "", "could not compare ClusterClass compatibility: desired ClusterClass must not be nil"))
	}

	// Validate InfrastructureClusterTemplate changes desired a compatible way.
	allErrs = append(allErrs, ClusterClassTemplateAreCompatible(current.Spec.Infrastructure.TemplateRef, desired.Spec.Infrastructure.TemplateRef,
		field.NewPath("spec", "infrastructure", "templateRef"))...)

	// Validate control plane changes desired a compatible way.
	allErrs = append(allErrs, ClusterClassTemplateAreCompatible(current.Spec.ControlPlane.TemplateRef, desired.Spec.ControlPlane.TemplateRef,
		field.NewPath("spec", "controlPlane", "templateRef"))...)
	if desired.Spec.ControlPlane.MachineInfrastructure.TemplateRef.IsDefined() && current.Spec.ControlPlane.MachineInfrastructure.TemplateRef.IsDefined() {
		allErrs = append(allErrs, ClusterClassTemplateAreCompatible(current.Spec.ControlPlane.MachineInfrastructure.TemplateRef, desired.Spec.ControlPlane.MachineInfrastructure.TemplateRef,
			field.NewPath("spec", "controlPlane", "machineInfrastructure", "templateRef"))...)
	}

	// Validate changes to MachineDeployments.
	allErrs = append(allErrs, MachineDeploymentClassesAreCompatible(current, desired)...)

	// Validate changes to MachinePools.
	allErrs = append(allErrs, MachinePoolClassesAreCompatible(current, desired)...)

	return allErrs
}

// MachineDeploymentClassesAreCompatible checks if each MachineDeploymentClass in the new ClusterClass is a compatible change from the previous ClusterClass.
// It checks if the MachineDeploymentClass.Template.Infrastructure reference has changed its Group or Kind.
func MachineDeploymentClassesAreCompatible(current, desired *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList

	// Ensure previous MachineDeployment class was modified in a compatible way.
	for i, class := range desired.Spec.Workers.MachineDeployments {
		for _, oldClass := range current.Spec.Workers.MachineDeployments {
			if class.Class == oldClass.Class {
				// NOTE: class.Template.Metadata and class.Template.Bootstrap are allowed to change;

				// class.Template.Bootstrap is ensured syntactically correct by LocalObjectTemplateIsValid.

				// Validates class.Template.Infrastructure template changes in a compatible way
				allErrs = append(allErrs, ClusterClassTemplateAreCompatible(oldClass.Infrastructure.TemplateRef, class.Infrastructure.TemplateRef,
					field.NewPath("spec", "workers", "machineDeployments").Index(i).Child("infrastructure", "templateRef"))...)
			}
		}
	}
	return allErrs
}

// MachineDeploymentClassesAreUnique checks that no two MachineDeploymentClasses in a ClusterClass share a name.
func MachineDeploymentClassesAreUnique(clusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList
	classes := sets.Set[string]{}
	for i, class := range clusterClass.Spec.Workers.MachineDeployments {
		if classes.Has(class.Class) {
			allErrs = append(allErrs,
				field.Invalid(
					field.NewPath("spec", "workers", "machineDeployments").Index(i).Child("class"),
					class.Class,
					fmt.Sprintf("MachineDeployment class must be unique. MachineDeployment with class %q is defined more than once", class.Class),
				),
			)
		}
		classes.Insert(class.Class)
	}
	return allErrs
}

// MachinePoolClassesAreCompatible checks if each MachinePoolClass in the new ClusterClass is a compatible change from the previous ClusterClass.
// It checks if the MachinePoolClass.Template.Infrastructure reference has changed its Group or Kind.
func MachinePoolClassesAreCompatible(current, desired *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList

	// Ensure previous MachinePool class was modified in a compatible way.
	for i, class := range desired.Spec.Workers.MachinePools {
		for _, oldClass := range current.Spec.Workers.MachinePools {
			if class.Class == oldClass.Class {
				// NOTE: class.Template.Metadata and class.Template.Bootstrap are allowed to change;

				// class.Template.Bootstrap is ensured syntactically correct by LocalObjectTemplateIsValid.

				// Validates class.Template.Infrastructure template changes in a compatible way
				allErrs = append(allErrs, ClusterClassTemplateAreCompatible(oldClass.Infrastructure.TemplateRef, class.Infrastructure.TemplateRef,
					field.NewPath("spec", "workers", "machinePools").Index(i).Child("infrastructure", "templateRef"))...)
			}
		}
	}
	return allErrs
}

// MachinePoolClassesAreUnique checks that no two MachinePoolClasses in a ClusterClass share a name.
func MachinePoolClassesAreUnique(clusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList
	classes := sets.Set[string]{}
	for i, class := range clusterClass.Spec.Workers.MachinePools {
		if classes.Has(class.Class) {
			allErrs = append(allErrs,
				field.Invalid(
					field.NewPath("spec", "workers", "machinePools").Index(i).Child("class"),
					class.Class,
					fmt.Sprintf("MachinePool class must be unique. MachinePool with class %q is defined more than once", class.Class),
				),
			)
		}
		classes.Insert(class.Class)
	}
	return allErrs
}

// MachineDeploymentTopologiesAreValidAndDefinedInClusterClass checks that each MachineDeploymentTopology name is not empty
// and unique, and each class in use is defined in ClusterClass.spec.Workers.MachineDeployments.
func MachineDeploymentTopologiesAreValidAndDefinedInClusterClass(desired *clusterv1.Cluster, clusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList
	if len(desired.Spec.Topology.Workers.MachineDeployments) == 0 {
		return nil
	}
	// MachineDeployment clusterClass must be defined in the ClusterClass.
	machineDeploymentClasses := mdClassNamesFromWorkerClass(clusterClass.Spec.Workers)
	names := sets.Set[string]{}
	for i, md := range desired.Spec.Topology.Workers.MachineDeployments {
		if !machineDeploymentClasses.Has(md.Class) {
			allErrs = append(allErrs,
				field.Invalid(
					field.NewPath("spec", "topology", "workers", "machineDeployments").Index(i).Child("class"),
					md.Class,
					fmt.Sprintf("MachineDeploymentClass with name %q does not exist in ClusterClass %q",
						md.Class, clusterClass.Name),
				),
			)
		}

		// MachineDeploymentTopology name should not be empty.
		if md.Name == "" {
			allErrs = append(
				allErrs,
				field.Required(
					field.NewPath("spec", "topology", "workers", "machineDeployments").Index(i).Child("name"),
					"name must not be empty",
				),
			)
			continue
		}

		if names.Has(md.Name) {
			allErrs = append(allErrs,
				field.Invalid(
					field.NewPath("spec", "topology", "workers", "machineDeployments").Index(i).Child("name"),
					md.Name,
					fmt.Sprintf("name must be unique. MachineDeployment with name %q is defined more than once", md.Name),
				),
			)
		}
		names.Insert(md.Name)
	}
	return allErrs
}

// MachinePoolTopologiesAreValidAndDefinedInClusterClass checks that each MachinePoolTopology name is not empty
// and unique, and each class in use is defined in ClusterClass.spec.Workers.MachinePools.
func MachinePoolTopologiesAreValidAndDefinedInClusterClass(desired *clusterv1.Cluster, clusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList
	if len(desired.Spec.Topology.Workers.MachinePools) == 0 {
		return nil
	}
	// MachinePool clusterClass must be defined in the ClusterClass.
	machinePoolClasses := mpClassNamesFromWorkerClass(clusterClass.Spec.Workers)
	names := sets.Set[string]{}
	for i, mp := range desired.Spec.Topology.Workers.MachinePools {
		if !machinePoolClasses.Has(mp.Class) {
			allErrs = append(allErrs,
				field.Invalid(
					field.NewPath("spec", "topology", "workers", "machinePools").Index(i).Child("class"),
					mp.Class,
					fmt.Sprintf("MachinePoolClass with name %q does not exist in ClusterClass %q",
						mp.Class, clusterClass.Name),
				),
			)
		}

		// MachinePoolTopology name should not be empty.
		if mp.Name == "" {
			allErrs = append(
				allErrs,
				field.Required(
					field.NewPath("spec", "topology", "workers", "machinePools").Index(i).Child("name"),
					"name must not be empty",
				),
			)
			continue
		}

		if names.Has(mp.Name) {
			allErrs = append(allErrs,
				field.Invalid(
					field.NewPath("spec", "topology", "workers", "machinePools").Index(i).Child("name"),
					mp.Name,
					fmt.Sprintf("name must be unique. MachinePool with name %q is defined more than once", mp.Name),
				),
			)
		}
		names.Insert(mp.Name)
	}
	return allErrs
}

// ClusterClassTemplatesAreValid checks that each template reference in the ClusterClass is valid .
func ClusterClassTemplatesAreValid(clusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, ClusterClassTemplateIsValid(clusterClass.Spec.Infrastructure.TemplateRef, field.NewPath("spec", "infrastructure"))...)
	allErrs = append(allErrs, ClusterClassTemplateIsValid(clusterClass.Spec.ControlPlane.TemplateRef, field.NewPath("spec", "controlPlane"))...)
	if clusterClass.Spec.ControlPlane.MachineInfrastructure.TemplateRef.IsDefined() {
		allErrs = append(allErrs, ClusterClassTemplateIsValid(clusterClass.Spec.ControlPlane.MachineInfrastructure.TemplateRef, field.NewPath("spec", "controlPlane", "machineInfrastructure"))...)
	}

	for i := range clusterClass.Spec.Workers.MachineDeployments {
		mdc := clusterClass.Spec.Workers.MachineDeployments[i]
		allErrs = append(allErrs, ClusterClassTemplateIsValid(mdc.Bootstrap.TemplateRef, field.NewPath("spec", "workers", "machineDeployments").Index(i).Child("template", "bootstrap"))...)
		allErrs = append(allErrs, ClusterClassTemplateIsValid(mdc.Infrastructure.TemplateRef, field.NewPath("spec", "workers", "machineDeployments").Index(i).Child("template", "infrastructure"))...)
	}

	for i := range clusterClass.Spec.Workers.MachinePools {
		mpc := clusterClass.Spec.Workers.MachinePools[i]
		allErrs = append(allErrs, ClusterClassTemplateIsValid(mpc.Bootstrap.TemplateRef, field.NewPath("spec", "workers", "machinePools").Index(i).Child("template", "bootstrap"))...)
		allErrs = append(allErrs, ClusterClassTemplateIsValid(mpc.Infrastructure.TemplateRef, field.NewPath("spec", "workers", "machinePools").Index(i).Child("template", "infrastructure"))...)
	}

	return allErrs
}

// mdClassNamesFromWorkerClass returns the set of MachineDeployment class names.
func mdClassNamesFromWorkerClass(w clusterv1.WorkersClass) sets.Set[string] {
	classes := sets.Set[string]{}
	for _, class := range w.MachineDeployments {
		classes.Insert(class.Class)
	}
	return classes
}

// mpClassNamesFromWorkerClass returns the set of MachinePool class names.
func mpClassNamesFromWorkerClass(w clusterv1.WorkersClass) sets.Set[string] {
	classes := sets.Set[string]{}
	for _, class := range w.MachinePools {
		classes.Insert(class.Class)
	}
	return classes
}
