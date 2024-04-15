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

package variables

import (
	"context"
	"fmt"
	"math"
	"strings"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsvalidation "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/validation"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel"
	structuraldefaulting "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/defaulting"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1validation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
	apiservercel "k8s.io/apiserver/pkg/cel"
	"k8s.io/apiserver/pkg/cel/environment"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	// builtinsName is the name of the builtin variable.
	builtinsName = "builtin"

	// StaticEstimatedCostLimit represents the largest-allowed static CEL cost on a per-expression basis.
	StaticEstimatedCostLimit = apiextensionsvalidation.StaticEstimatedCostLimit
)

// ValidateClusterClassVariables validates clusterClassVariable.
func ValidateClusterClassVariables(ctx context.Context, clusterClassVariables []clusterv1.ClusterClassVariable, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, validateClusterClassVariableNamesUnique(clusterClassVariables, fldPath)...)

	for i := range clusterClassVariables {
		allErrs = append(allErrs, validateClusterClassVariable(ctx, &clusterClassVariables[i], fldPath.Index(i))...)
	}

	return allErrs
}

// validateClusterClassVariableNamesUnique validates that ClusterClass variable names are unique.
func validateClusterClassVariableNamesUnique(clusterClassVariables []clusterv1.ClusterClassVariable, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	variableNames := sets.Set[string]{}
	for i, clusterClassVariable := range clusterClassVariables {
		if variableNames.Has(clusterClassVariable.Name) {
			allErrs = append(allErrs,
				field.Invalid(
					pathPrefix.Index(i).Child("name"),
					clusterClassVariable.Name,
					fmt.Sprintf("variable name must be unique. Variable with name %q is defined more than once", clusterClassVariable.Name),
				),
			)
		}
		variableNames.Insert(clusterClassVariable.Name)
	}

	return allErrs
}

// validateClusterClassVariable validates a ClusterClassVariable.
func validateClusterClassVariable(ctx context.Context, variable *clusterv1.ClusterClassVariable, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate variable name.
	allErrs = append(allErrs, validateClusterClassVariableName(variable.Name, fldPath.Child("name"))...)

	// Validate variable metadata.
	allErrs = append(allErrs, validateClusterClassVariableMetadata(variable.Metadata, fldPath.Child("metadata"))...)

	// Validate schema.
	allErrs = append(allErrs, validateRootSchema(ctx, variable, fldPath.Child("schema", "openAPIV3Schema"))...)

	return allErrs
}

// validateClusterClassVariableName validates a variable name.
func validateClusterClassVariableName(variableName string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if variableName == "" {
		allErrs = append(allErrs, field.Required(fldPath, "variable name must be defined"))
	}

	if variableName == builtinsName {
		allErrs = append(allErrs, field.Invalid(fldPath, variableName, fmt.Sprintf("%q is a reserved variable name", builtinsName)))
	}
	if strings.Contains(variableName, ".") {
		allErrs = append(allErrs, field.Invalid(fldPath, variableName, "variable name cannot contain \".\"")) // TODO: consider if to restrict variable names to RFC 1123
	}

	return allErrs
}

// validateClusterClassVariableMetadata validates a variable metadata.
func validateClusterClassVariableMetadata(metadata clusterv1.ClusterClassVariableMetadata, fldPath *field.Path) field.ErrorList {
	allErrs := metav1validation.ValidateLabels(
		metadata.Labels,
		fldPath.Child("labels"),
	)
	allErrs = append(allErrs, apivalidation.ValidateAnnotations(
		metadata.Annotations,
		fldPath.Child("annotations"),
	)...)
	return allErrs
}

var validVariableTypes = sets.Set[string]{}.Insert("object", "array", "string", "number", "integer", "boolean")

// validateRootSchema validates the schema.
func validateRootSchema(ctx context.Context, clusterClassVariable *clusterv1.ClusterClassVariable, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	apiExtensionsSchema, allErrs := convertToAPIExtensionsJSONSchemaProps(&clusterClassVariable.Schema.OpenAPIV3Schema, field.NewPath("schema"))
	if len(allErrs) > 0 {
		return field.ErrorList{field.InternalError(fldPath,
			fmt.Errorf("failed to convert schema definition for variable %q; ClusterClass should be checked: %v", clusterClassVariable.Name, allErrs))} // TODO: consider if to add ClusterClass name
	}

	// Validate structural schema.
	// Note: structural schema only allows `type: object` on the root level, so we wrap the schema with:
	// type: object
	// properties:
	//   variableSchema: <variable-schema>
	wrappedSchema := &apiextensions.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextensions.JSONSchemaProps{
			"variableSchema": *apiExtensionsSchema,
		},
	}

	// Get the structural schema for the variable.
	ss, err := structuralschema.NewStructural(wrappedSchema)
	if err != nil {
		return append(allErrs, field.Invalid(fldPath.Child("schema"), "", err.Error()))
	}

	// Validate the schema.
	if validationErrors := structuralschema.ValidateStructural(fldPath.Child("schema"), ss); len(validationErrors) > 0 {
		return append(allErrs, validationErrors...)
	}

	// Validate defaults in the structural schema.
	validationErrors, err := structuraldefaulting.ValidateDefaults(ctx, fldPath.Child("schema"), ss, true, true)
	if err != nil {
		return append(allErrs, field.Invalid(fldPath.Child("schema"), "", err.Error()))
	}
	if len(validationErrors) > 0 {
		return append(allErrs, validationErrors...)
	}

	// If the structural schema is valid, ensure a schema validator can be constructed.
	if _, _, err := validation.NewSchemaValidator(apiExtensionsSchema); err != nil {
		return append(allErrs, field.Invalid(fldPath, "", fmt.Sprintf("failed to build validator: %v", err)))
	}

	allErrs = append(allErrs, validateSchema(apiExtensionsSchema, fldPath)...)
	return allErrs
}

func validateSchema(schema *apiextensions.JSONSchemaProps, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// Validate that type is one of the validVariableTypes.
	switch {
	case schema.Type == "":
		return field.ErrorList{field.Required(fldPath.Child("type"), "type cannot be empty")}
	case !validVariableTypes.Has(schema.Type):
		return field.ErrorList{field.NotSupported(fldPath.Child("type"), schema.Type, sets.List(validVariableTypes))}
	}

	// If the structural schema is valid, ensure a schema validator can be constructed.
	validator, _, err := validation.NewSchemaValidator(schema)
	if err != nil {
		return append(allErrs, field.Invalid(fldPath, "", fmt.Sprintf("failed to build validator: %v", err)))
	}

	if schema.Example != nil {
		if errs := validation.ValidateCustomResource(fldPath, *schema.Example, validator); len(errs) > 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("example"), schema.Example, fmt.Sprintf("invalid value in example: %v", errs)))
		}
	}

	for i, enum := range schema.Enum {
		if enum != nil {
			if errs := validation.ValidateCustomResource(fldPath, enum, validator); len(errs) > 0 {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("enum").Index(i), enum, fmt.Sprintf("invalid value in enum: %v", errs)))
			}
		}
	}

	if schema.AdditionalProperties != nil {
		if len(schema.Properties) > 0 {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("additionalProperties"), "additionalProperties and properties are mutual exclusive"))
		}
		allErrs = append(allErrs, validateSchema(schema.AdditionalProperties.Schema, fldPath.Child("additionalProperties"))...)
	}

	for propertyName, propertySchema := range schema.Properties {
		p := propertySchema
		allErrs = append(allErrs, validateSchema(&p, fldPath.Child("properties").Key(propertyName))...)
	}

	// If any schema related validation errors have been found at this level or deeper, skip CEL expression validation.
	// Invalid OpenAPISchemas are not always possible to convert into valid CEL DeclTypes, and can lead to CEL
	// validation error messages that are not actionable (will go away once the schema errors are resolved) and that
	// are difficult for CEL expression authors to understand.
	if len(allErrs) > 0 {
		return allErrs
	}

	celContext := apiextensionsvalidation.RootCELContext(schema)
	allErrs = append(allErrs, validateCELExpressions(schema, fldPath, celContext)...)

	return allErrs
}

func validateCELExpressions(schema *apiextensions.JSONSchemaProps, fldPath *field.Path, celContext *apiextensionsvalidation.CELSchemaContext) field.ErrorList {
	var allErrs field.ErrorList

	if celContext == nil {
		return allErrs
	}

	if schema.AdditionalProperties != nil {
		allErrs = append(allErrs, validateCELExpressions(schema.AdditionalProperties.Schema, fldPath.Child("additionalProperties"), celContext.ChildAdditionalPropertiesContext(schema.AdditionalProperties.Schema))...)
	}

	if len(schema.Properties) != 0 {
		for property, jsonSchema := range schema.Properties {
			propertySchema := jsonSchema
			allErrs = append(allErrs, validateCELExpressions(&propertySchema, fldPath.Child("properties").Key(property), celContext.ChildPropertyContext(&propertySchema, property))...)
		}
	}

	if schema.Items != nil {
		allErrs = append(allErrs, validateCELExpressions(schema.Items.Schema, fldPath.Child("items"), celContext.ChildItemsContext(schema.Items.Schema))...)
		if len(schema.Items.JSONSchemas) != 0 {
			for i, jsonSchema := range schema.Items.JSONSchemas {
				itemsSchema := jsonSchema
				allErrs = append(allErrs, validateCELExpressions(&itemsSchema, fldPath.Child("items").Index(i), celContext.ChildItemsContext(&itemsSchema))...)
			}
		}
	}

	typeInfo, err := celContext.TypeInfo()
	if err != nil {
		return append(allErrs, field.InternalError(fldPath.Child("x-kubernetes-validations"), fmt.Errorf("internal error: failed to construct type information for x-kubernetes-validations rules: %w", err)))
	}
	if typeInfo == nil {
		return allErrs
	}

	compResults, err := cel.Compile(
		typeInfo.Schema,
		typeInfo.DeclType,
		celconfig.PerCallLimit,
		environment.MustBaseEnvSet(environment.DefaultCompatibilityVersion()),
		cel.NewExpressionsEnvLoader(),
	)
	if err != nil {
		return append(allErrs, field.InternalError(fldPath.Child("x-kubernetes-validations"), err))
	}

	for i, cr := range compResults {
		expressionCost := getExpressionCost(cr, celContext)
		if expressionCost > StaticEstimatedCostLimit {
			costErrorMsg := getCostErrorMessage("estimated rule cost", expressionCost, StaticEstimatedCostLimit)
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("x-kubernetes-validations").Index(i).Child("rule"), costErrorMsg))
		}
		if celContext.TotalCost != nil {
			celContext.TotalCost.ObserveExpressionCost(fldPath.Child("x-kubernetes-validations").Index(i).Child("rule"), expressionCost)
		}
		if cr.Error != nil {
			if cr.Error.Type == apiservercel.ErrorTypeRequired {
				allErrs = append(allErrs, field.Required(fldPath.Child("x-kubernetes-validations").Index(i).Child("rule"), cr.Error.Detail))
			} else {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("x-kubernetes-validations").Index(i).Child("rule"), schema.XValidations[i], cr.Error.Detail))
			}
		}
		if cr.MessageExpressionError != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("x-kubernetes-validations").Index(i).Child("messageExpression"), schema.XValidations[i], cr.MessageExpressionError.Detail))
		} else if cr.MessageExpression != nil {
			if cr.MessageExpressionMaxCost > StaticEstimatedCostLimit {
				costErrorMsg := getCostErrorMessage("estimated messageExpression cost", cr.MessageExpressionMaxCost, StaticEstimatedCostLimit)
				allErrs = append(allErrs, field.Forbidden(fldPath.Child("x-kubernetes-validations").Index(i).Child("messageExpression"), costErrorMsg))
			}
			if celContext.TotalCost != nil {
				celContext.TotalCost.ObserveExpressionCost(fldPath.Child("x-kubernetes-validations").Index(i).Child("messageExpression"), cr.MessageExpressionMaxCost)
			}
		}
	}
	return allErrs
}

// multiplyWithOverflowGuard returns the product of baseCost and cardinality unless that product
// would exceed math.MaxUint, in which case math.MaxUint is returned.
func multiplyWithOverflowGuard(baseCost, cardinality uint64) uint64 {
	if baseCost == 0 {
		// an empty rule can return 0, so guard for that here
		return 0
	} else if math.MaxUint/baseCost < cardinality {
		return math.MaxUint
	}
	return baseCost * cardinality
}

// unbounded uses nil to represent an unbounded cardinality value.
var unbounded *uint64 = nil //nolint:revive // Using as a named variable to provide the meaning of nil in this context.

func getExpressionCost(cr cel.CompilationResult, cardinalityCost *apiextensionsvalidation.CELSchemaContext) uint64 {
	if cardinalityCost.MaxCardinality != unbounded {
		return multiplyWithOverflowGuard(cr.MaxCost, *cardinalityCost.MaxCardinality)
	}
	return multiplyWithOverflowGuard(cr.MaxCost, cr.MaxCardinality)
}

func getCostErrorMessage(costName string, expressionCost, costLimit uint64) string {
	exceedFactor := float64(expressionCost) / float64(costLimit)
	var factor string
	if exceedFactor > 100.0 {
		// if exceedFactor is greater than 2 orders of magnitude, the rule is likely O(n^2) or worse
		// and will probably never validate without some set limits
		// also in such cases the cost estimation is generally large enough to not add any value
		factor = "more than 100x"
	} else if exceedFactor < 1.5 {
		factor = fmt.Sprintf("%fx", exceedFactor) // avoid reporting "exceeds budge by a factor of 1.0x"
	} else {
		factor = fmt.Sprintf("%.1fx", exceedFactor)
	}
	return fmt.Sprintf("%s exceeds budget by factor of %s (try simplifying the rule, or adding maxItems, maxProperties, and maxLength where arrays, maps, and strings are declared)", costName, factor)
}
