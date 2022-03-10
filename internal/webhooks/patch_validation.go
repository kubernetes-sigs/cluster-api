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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"text/template"

	sprig "github.com/Masterminds/sprig/v3"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// validatePatches returns errors if the Patches in the ClusterClass violate any validation rules.
func validatePatches(clusterClass *clusterv1.ClusterClass) field.ErrorList {
	var allErrs field.ErrorList
	names := sets.String{}
	for i, patch := range clusterClass.Spec.Patches {
		allErrs = append(
			allErrs,
			validatePatch(patch, names, clusterClass, field.NewPath("spec", "patches").Index(i))...,
		)
		names.Insert(patch.Name)
	}
	return allErrs
}

func validatePatch(patch clusterv1.ClusterClassPatch, names sets.String, clusterClass *clusterv1.ClusterClass, path *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs,
		validatePatchName(patch, names, path)...,
	)
	allErrs = append(allErrs,
		validatePatchDefinitions(patch, clusterClass, path)...,
	)
	return allErrs
}

func validatePatchName(patch clusterv1.ClusterClassPatch, names sets.String, path *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	if patch.Name == "" {
		allErrs = append(allErrs,
			field.Required(
				path.Child("name"),
				"patch name must be defined",
			),
		)
	}

	if names.Has(patch.Name) {
		allErrs = append(allErrs,
			field.Invalid(
				path.Child("name"),
				patch.Name,
				fmt.Sprintf("patch names must be unique. Patch with name %q is defined more than once", patch.Name),
			),
		)
	}
	return allErrs
}

func validatePatchDefinitions(patch clusterv1.ClusterClassPatch, clusterClass *clusterv1.ClusterClass, path *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, validateEnabledIf(patch.EnabledIf, path.Child("enabledIf"))...)

	for i, definition := range patch.Definitions {
		allErrs = append(allErrs,
			validateJSONPatches(definition.JSONPatches, clusterClass.Spec.Variables, path.Child("definitions").Index(i).Child("jsonPatches"))...)
		allErrs = append(allErrs,
			validateSelectors(definition.Selector, clusterClass, path.Child("definitions").Index(i).Child("selector"))...)
	}
	return allErrs
}

// validateSelectors validates if enabledIf is a valid template if it is set.
func validateEnabledIf(enabledIf *string, path *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if enabledIf != nil {
		// Error if template can not be parsed.
		_, err := template.New("enabledIf").Funcs(sprig.HermeticTxtFuncMap()).Parse(*enabledIf)
		if err != nil {
			allErrs = append(allErrs,
				field.Invalid(
					path,
					*enabledIf,
					fmt.Sprintf("template can not be parsed: %v", err),
				))
		}
	}

	return allErrs
}

// validateSelectors tests to see if the selector matches any template in the ClusterClass.
// It returns nil as soon as it finds any matching template and an error if there is no match.
func validateSelectors(selector clusterv1.PatchSelector, class *clusterv1.ClusterClass, path *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// Return an error if none of the possible selectors are enabled.
	if !(selector.MatchResources.InfrastructureCluster || selector.MatchResources.ControlPlane ||
		(selector.MatchResources.MachineDeploymentClass != nil && len(selector.MatchResources.MachineDeploymentClass.Names) > 0)) {
		return append(allErrs,
			field.Invalid(
				path,
				prettyPrint(selector),
				"no selector enabled",
			))
	}
	if selector.MatchResources.InfrastructureCluster {
		if selectorMatchTemplate(selector, class.Spec.Infrastructure.Ref) {
			return nil
		}
	}
	if selector.MatchResources.ControlPlane {
		if selectorMatchTemplate(selector, class.Spec.ControlPlane.Ref) {
			return nil
		}
	}

	if selector.MatchResources.ControlPlane && class.Spec.ControlPlane.MachineInfrastructure != nil {
		if selectorMatchTemplate(selector, class.Spec.ControlPlane.MachineInfrastructure.Ref) {
			return nil
		}
	}

	if selector.MatchResources.MachineDeploymentClass != nil && len(selector.MatchResources.MachineDeploymentClass.Names) > 0 {
		for _, name := range selector.MatchResources.MachineDeploymentClass.Names {
			for _, md := range class.Spec.Workers.MachineDeployments {
				if md.Class == name {
					if selectorMatchTemplate(selector, md.Template.Infrastructure.Ref) {
						return nil
					}
					if selectorMatchTemplate(selector, md.Template.Bootstrap.Ref) {
						return nil
					}
				}
			}
		}
	}
	// if the code has not returned at this point there is no matching template in the ClusterClass. Return an error.
	return append(allErrs,
		field.Invalid(
			path,
			prettyPrint(selector),
			"selector did not match any template in the ClusterClass",
		))
}

// selectorMatchTemplate returns true if APIVersion and Kind for the given selector match the reference.
func selectorMatchTemplate(selector clusterv1.PatchSelector, reference *corev1.ObjectReference) bool {
	if reference == nil {
		return false
	}
	return selector.Kind == reference.Kind && selector.APIVersion == reference.APIVersion
}

var validOps = sets.NewString("add", "replace", "remove")

func validateJSONPatches(jsonPatches []clusterv1.JSONPatch, variables []clusterv1.ClusterClassVariable, path *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	variableSet, _ := getClusterClassVariablesMapWithReverseIndex(variables)

	for i, jsonPatch := range jsonPatches {
		if !validOps.Has(jsonPatch.Op) {
			allErrs = append(allErrs,
				field.NotSupported(
					path.Index(i).Child("op"),
					prettyPrint(jsonPatch),
					validOps.List(),
				))
		}

		if !strings.HasPrefix(jsonPatch.Path, "/spec/") {
			allErrs = append(allErrs,
				field.Invalid(
					path.Index(i).Child("path"),
					prettyPrint(jsonPatch),
					"jsonPatch path must start with \"/spec/\"",
				))
		}

		// Validate that array access is only prepend or append for add and not allowed for replace or remove.
		allErrs = append(allErrs,
			validateIndexAccess(jsonPatch, path.Index(i).Child("path"))...,
		)

		// Validate the value and valueFrom fields for the patch.
		allErrs = append(allErrs,
			validateJSONPatchValues(jsonPatch, variableSet, path.Index(i))...,
		)
	}
	return allErrs
}

func validateJSONPatchValues(jsonPatch clusterv1.JSONPatch, variableSet map[string]*clusterv1.ClusterClassVariable, path *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// move to the next variable if the jsonPatch does not have "replace" or "add" op. Additional validation is not needed.
	if jsonPatch.Op != "add" && jsonPatch.Op != "replace" {
		return allErrs
	}

	if jsonPatch.Value == nil && jsonPatch.ValueFrom == nil {
		allErrs = append(allErrs,
			field.Invalid(
				path,
				prettyPrint(jsonPatch),
				"jsonPatch must define one of value or valueFrom",
			))
	}

	if jsonPatch.Value != nil && jsonPatch.ValueFrom != nil {
		allErrs = append(allErrs,
			field.Invalid(
				path,
				prettyPrint(jsonPatch),
				"jsonPatch can not define both value and valueFrom",
			))
	}

	// Attempt to marshal the JSON to discover if it  is valid. If jsonPatch.Value.Raw is set to nil skip this check
	// and accept the nil value.
	if jsonPatch.Value != nil && jsonPatch.Value.Raw != nil {
		var v interface{}
		if err := json.Unmarshal(jsonPatch.Value.Raw, &v); err != nil {
			allErrs = append(allErrs,
				field.Invalid(
					path.Child("value"),
					string(jsonPatch.Value.Raw),
					"jsonPatch Value is invalid JSON",
				))
		}
	}
	if jsonPatch.ValueFrom != nil && jsonPatch.ValueFrom.Template == nil && jsonPatch.ValueFrom.Variable == nil {
		allErrs = append(allErrs,
			field.Invalid(
				path.Child("valueFrom"),
				prettyPrint(jsonPatch.ValueFrom),
				"valueFrom must set either template or variable",
			))
	}
	if jsonPatch.ValueFrom != nil && jsonPatch.ValueFrom.Template != nil && jsonPatch.ValueFrom.Variable != nil {
		allErrs = append(allErrs,
			field.Invalid(
				path.Child("valueFrom"),
				prettyPrint(jsonPatch.ValueFrom),
				"valueFrom can not set both template and variable",
			))
	}

	if jsonPatch.ValueFrom != nil && jsonPatch.ValueFrom.Template != nil {
		// Error if template can not be parsed.
		_, err := template.New("valueFrom.template").Funcs(sprig.HermeticTxtFuncMap()).Parse(*jsonPatch.ValueFrom.Template)
		if err != nil {
			allErrs = append(allErrs,
				field.Invalid(
					path.Child("valueFrom", "template"),
					*jsonPatch.ValueFrom.Template,
					fmt.Sprintf("template can not be parsed: %v", err),
				))
		}
	}

	// If set validate that the variable is valid.
	if jsonPatch.ValueFrom != nil && jsonPatch.ValueFrom.Variable != nil {
		// If the variable is one of the list of builtin variables it's valid.
		if strings.HasPrefix(*jsonPatch.ValueFrom.Variable, "builtin.") {
			if _, ok := builtinVariables[*jsonPatch.ValueFrom.Variable]; !ok {
				allErrs = append(allErrs,
					field.Invalid(
						path.Child("valueFrom", "variable"),
						*jsonPatch.ValueFrom.Variable,
						"not a defined builtin variable",
					))
			}
		} else {
			// Note: We're only validating if the variable name exists without
			// validating if the whole path is an existing variable.
			// This could be done by re-using getVariableValue of the json patch
			// generator but requires a refactoring first.
			variableName := getVariableName(*jsonPatch.ValueFrom.Variable)
			if _, ok := variableSet[variableName]; !ok {
				allErrs = append(allErrs,
					field.Invalid(
						path.Child("valueFrom", "variable"),
						*jsonPatch.ValueFrom.Variable,
						fmt.Sprintf("variable with name %s cannot be found", *jsonPatch.ValueFrom.Variable),
					))
			}
		}
	}
	return allErrs
}

func getVariableName(variable string) string {
	return strings.FieldsFunc(variable, func(r rune) bool {
		return r == '[' || r == '.'
	})[0]
}

// This contains a list of all of the valid builtin variables.
// TODO(killianmuldoon): Match this list to controllers/topology/internal/extensions/patches/variables as those structs become available across the code base i.e. public or top-level internal.
var builtinVariables = sets.NewString(
	"builtin",

	// Cluster builtins.
	"builtin.cluster",
	"builtin.cluster.name",
	"builtin.cluster.namespace",

	// ClusterTopology builtins.
	"builtin.cluster.topology",
	"builtin.cluster.topology.class",
	"builtin.cluster.topology.version",

	// ClusterNetwork builtins
	"builtin.cluster.network",
	"builtin.cluster.network.serviceDomain",
	"builtin.cluster.network.services",
	"builtin.cluster.network.pods",
	"builtin.cluster.network.ipFamily",

	// ControlPlane builtins.
	"builtin.controlPlane",
	"builtin.controlPlane.name",
	"builtin.controlPlane.replicas",
	"builtin.controlPlane.version",
	// ControlPlane ref builtins.
	"builtin.controlPlane.machineTemplate.infrastructureRef.name",

	// MachineDeployment builtins.
	"builtin.machineDeployment",
	"builtin.machineDeployment.class",
	"builtin.machineDeployment.name",
	"builtin.machineDeployment.replicas",
	"builtin.machineDeployment.topologyName",
	"builtin.machineDeployment.version",
	// MachineDeployment ref builtins.
	"builtin.machineDeployment.bootstrap.configRef.name",
	"builtin.machineDeployment.infrastructureRef.name",
)

// validateIndexAccess checks to see if the jsonPath is attempting to add an element in the array i.e. access by number
// If the operation is add an error is thrown if a number greater than 0 is used as an index.
// If the operation is replace an error is thrown if an index is used.
func validateIndexAccess(jsonPatch clusterv1.JSONPatch, path *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	pathParts := strings.Split(jsonPatch.Path, "/")
	for _, part := range pathParts {
		// Check if the path segment is a valid number. If an error is thrown continue to the next segment.
		index, err := strconv.Atoi(part)
		if err != nil {
			continue
		}

		// If the operation is add an error is thrown if a number greater than 0 is used as an index.
		if jsonPatch.Op == "add" && index != 0 {
			allErrs = append(allErrs,
				field.Invalid(path,
					jsonPatch.Path,
					"arrays can only be accessed using \"0\" (prepend) or \"-\" (append)",
				))
		}

		// If the jsonPatch operation is replace or remove disallow any number as an element in the path.
		if jsonPatch.Op == "replace" || jsonPatch.Op == "remove" {
			allErrs = append(allErrs,
				field.Invalid(path,
					jsonPatch.Path,
					fmt.Sprintf("elements in arrays can not be accessed in a %s operation", jsonPatch.Op),
				))
		}
	}
	return allErrs
}

func prettyPrint(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errors.Wrapf(err, "failed to marshal field value").Error()
	}
	return string(b)
}
