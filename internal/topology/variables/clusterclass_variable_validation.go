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
	"regexp"
	"strings"

	celgo "github.com/google/cel-go/cel"
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
	// StaticEstimatedCRDCostLimit represents the largest-allowed total cost for the x-kubernetes-validations rules of a variable.
	StaticEstimatedCRDCostLimit = apiextensionsvalidation.StaticEstimatedCRDCostLimit
)

// ValidateClusterClassVariables validates clusterClassVariable.
func ValidateClusterClassVariables(ctx context.Context, oldClusterClassVariables, clusterClassVariables []clusterv1.ClusterClassVariable, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, validateClusterClassVariableNamesUnique(clusterClassVariables, fldPath)...)

	oldClusterClassVariablesMap := map[string]clusterv1.ClusterClassVariable{}
	for _, variable := range oldClusterClassVariables {
		oldClusterClassVariablesMap[variable.Name] = variable
	}

	for _, clusterClassVariable := range clusterClassVariables {
		// Add variable name as key, this makes it easier to read the field path.
		fldPath := fldPath.Key(clusterClassVariable.Name)

		var oldClusterClassVariable *clusterv1.ClusterClassVariable
		if v, exists := oldClusterClassVariablesMap[clusterClassVariable.Name]; exists {
			oldClusterClassVariable = &v
		}
		allErrs = append(allErrs, validateClusterClassVariable(ctx, oldClusterClassVariable, &clusterClassVariable, fldPath)...)
	}

	return allErrs
}

// validateClusterClassVariableNamesUnique validates that ClusterClass variable names are unique.
func validateClusterClassVariableNamesUnique(clusterClassVariables []clusterv1.ClusterClassVariable, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	variableNames := sets.Set[string]{}
	for _, clusterClassVariable := range clusterClassVariables {
		// Add variable name as key, this makes it easier to read the field path.
		fldPath := fldPath.Key(clusterClassVariable.Name)

		if variableNames.Has(clusterClassVariable.Name) {
			allErrs = append(allErrs,
				field.Invalid(
					fldPath.Child("name"),
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
func validateClusterClassVariable(ctx context.Context, oldVariable, variable *clusterv1.ClusterClassVariable, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate variable name.
	allErrs = append(allErrs, validateClusterClassVariableName(variable.Name, fldPath.Child("name"))...)

	// Validate variable metadata.
	allErrs = append(allErrs, validateClusterClassVariableMetadata(variable.Metadata, fldPath.Child("metadata"))...)

	// Validate variable XMetadata.
	allErrs = append(allErrs, validateClusterClassXVariableMetadata(&variable.Schema.OpenAPIV3Schema, fldPath.Child("schema", "openAPIV3Schema"))...)

	// Validate schema.
	allErrs = append(allErrs, validateRootSchema(ctx, oldVariable, variable, fldPath.Child("schema", "openAPIV3Schema"))...)

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

// validateClusterClassXVariableMetadata validates XMetadata recursively across the entire schema.
// Note: This cannot be done within validateSchema because XMetadata does not exist in apiextensions.JSONSchemaProps.
func validateClusterClassXVariableMetadata(schema *clusterv1.JSONSchemaProps, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	if schema.XMetadata != nil {
		allErrs = metav1validation.ValidateLabels(
			schema.XMetadata.Labels,
			fldPath.Child("x-metadata", "labels"),
		)
		allErrs = append(allErrs, apivalidation.ValidateAnnotations(
			schema.XMetadata.Annotations,
			fldPath.Child("x-metadata", "annotations"),
		)...)
	}

	if schema.AdditionalProperties != nil {
		allErrs = append(allErrs, validateClusterClassXVariableMetadata(schema.AdditionalProperties, fldPath.Child("additionalProperties"))...)
	}

	for propertyName, propertySchema := range schema.Properties {
		p := propertySchema
		allErrs = append(allErrs, validateClusterClassXVariableMetadata(&p, fldPath.Child("properties").Key(propertyName))...)
	}

	if schema.Items != nil {
		allErrs = append(allErrs, validateClusterClassXVariableMetadata(schema.Items, fldPath.Child("items"))...)
	}

	return allErrs
}

var validVariableTypes = sets.Set[string]{}.Insert("object", "array", "string", "number", "integer", "boolean")

// validateRootSchema validates the schema.
func validateRootSchema(ctx context.Context, oldClusterClassVariables, clusterClassVariable *clusterv1.ClusterClassVariable, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	apiExtensionsSchema, validationErrors := convertToAPIExtensionsJSONSchemaProps(&clusterClassVariable.Schema.OpenAPIV3Schema, fldPath)
	if len(validationErrors) > 0 {
		for _, validationError := range validationErrors {
			// Add context to the error message.
			validationError.Detail = fmt.Sprintf("failed to convert schema definition for variable %q; ClusterClass should be checked: %s", clusterClassVariable.Name, validationError.Detail) // TODO: consider if to add ClusterClass name
			allErrs = append(allErrs, validationError)
		}
		return allErrs
	}

	var oldAPIExtensionsSchema *apiextensions.JSONSchemaProps
	if oldClusterClassVariables != nil {
		oldAPIExtensionsSchema, validationErrors = convertToAPIExtensionsJSONSchemaProps(&oldClusterClassVariables.Schema.OpenAPIV3Schema, fldPath)
		if len(validationErrors) > 0 {
			for _, validationError := range validationErrors {
				// Add context to the error message.
				validationError.Detail = fmt.Sprintf("failed to convert old schema definition for variable %q; ClusterClass should be checked: %s", clusterClassVariable.Name, validationError.Detail) // TODO: consider if to add ClusterClass name
				allErrs = append(allErrs, validationError)
			}
			return allErrs
		}
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
	wrappedStructural, err := structuralschema.NewStructural(wrappedSchema)
	if err != nil {
		return append(allErrs, field.Invalid(fldPath, "", err.Error()))
	}

	// Validate the schema.
	if validationErrors := structuralschema.ValidateStructural(fldPath, wrappedStructural); len(validationErrors) > 0 {
		for _, validationError := range validationErrors {
			// Remove .properties[variableSchema], this only exists because we had to wrap via wrappedSchema.
			validationError.Field = strings.Replace(validationError.Field, "openAPIV3Schema.properties[variableSchema]", "openAPIV3Schema", 1)
			allErrs = append(allErrs, validationError)
		}
		return allErrs
	}
	ss := wrappedStructural.Properties["variableSchema"]

	// Validate defaults in the structural schema.
	// Note: Since convertToAPIExtensionsJSONSchemaProps converts the XValidations,
	// this func internally now also uses CEL to validate the defaults values.
	validationErrors, err = structuraldefaulting.ValidateDefaults(ctx, fldPath, &ss, false, true)
	if err != nil {
		return append(allErrs, field.Invalid(fldPath, "", err.Error()))
	}
	if len(validationErrors) > 0 {
		return append(allErrs, validationErrors...)
	}

	celContext := apiextensionsvalidation.RootCELContext(apiExtensionsSchema)

	// Context:
	// * EnvSets are CEL environments configured with various options (https://github.com/kubernetes/kubernetes/blob/v1.30.0/staging/src/k8s.io/apiserver/pkg/cel/environment/base.go#L49)
	// * Options can be e.g. library functions (string, regex) or types (IP, CIDR)
	// * Over time / Kubernetes versions new options might be added and older ones removed
	// * When creating an EnvSet two environments are created:
	//   * one environment based on the passed in Kubernetes version. We use environment.DefaultCompatibilityVersion(),
	//     which is either n-1 or an even earlier version (called "n-1" in the following)
	//   * one environment with vMaxUint.MaxUint (called "max" in the following")
	// * The following is implemented similar to how Kubernetes implements it for CRD create/update and CR validation.
	// * CAPI validation:
	//   * If a ClusterClass create or update introduces a new CEL expression, we will validate it with the "n-1" env
	//   * If a ClusterClass update keeps a CEL expression unchanged, it will be validated with the "max" env
	//   * On Cluster create or update, variable values will always be validated with the "max" env
	// * The goal is to only allow adding new CEL expressions that are compatible with "n-1",
	//   while allowing to use pre-existing CEL expressions that are compatible with "max".
	// * We have to do this because:
	//   * This makes it possible to rollback Cluster API from "m" to "m-1", because Cluster API "m-1" is
	//     able to use CEL expressions created with Cluster API "m" (because "m" only allows to add CEL expressions
	//     that can be used by "m-1").
	//   * The Kubernetes CEL library assumes this behavior and it's baked into cel.NewValidator (they use n to validate)
	// * If the DefaultCompatibilityVersion is an earlier version than "n-1", it means we could roll back even more than 1 version.
	opts := &validationOptions{
		celEnvironmentSet: environment.MustBaseEnvSet(environment.DefaultCompatibilityVersion(), true),
	}
	if oldAPIExtensionsSchema != nil {
		opts.preexistingExpressions = findPreexistingExpressions(oldAPIExtensionsSchema)
	}

	allErrs = append(allErrs, validateSchema(ctx, apiExtensionsSchema, fldPath, opts, celContext, nil).AllErrors()...)
	if celContext != nil && celContext.TotalCost != nil {
		if celContext.TotalCost.Total > StaticEstimatedCRDCostLimit {
			for _, expensive := range celContext.TotalCost.MostExpensive {
				costErrorMsg := "contributed to estimated rule & messageExpression cost total exceeding cost limit for entire OpenAPIv3 schema"
				allErrs = append(allErrs, field.Forbidden(expensive.Path, costErrorMsg))
			}

			costErrorMsg := getCostErrorMessage("x-kubernetes-validations estimated rule & messageExpression cost total for entire OpenAPIv3 schema", celContext.TotalCost.Total, StaticEstimatedCRDCostLimit)
			allErrs = append(allErrs, field.Forbidden(fldPath, costErrorMsg))
		}
	}

	// If the structural schema is valid, ensure a schema validator can be constructed.
	if len(allErrs) == 0 {
		if _, _, err := validation.NewSchemaValidator(apiExtensionsSchema); err != nil {
			return append(allErrs, field.Invalid(fldPath, "", fmt.Sprintf("failed to build validator: %v", err)))
		}
	}
	return allErrs
}

var supportedValidationReason = sets.NewString(
	string(clusterv1.FieldValueRequired),
	string(clusterv1.FieldValueForbidden),
	string(clusterv1.FieldValueInvalid),
	string(clusterv1.FieldValueDuplicate),
)

func validateSchema(ctx context.Context, schema *apiextensions.JSONSchemaProps, fldPath *field.Path, opts *validationOptions, celContext *apiextensionsvalidation.CELSchemaContext, uncorrelatablePath *field.Path) *OpenAPISchemaErrorList {
	allErrs := &OpenAPISchemaErrorList{SchemaErrors: field.ErrorList{}, CELErrors: field.ErrorList{}}

	// Validate that type is one of the validVariableTypes.
	switch {
	case schema.Type == "":
		allErrs.SchemaErrors = append(allErrs.SchemaErrors, field.Required(fldPath.Child("type"), "type cannot be empty"))
		return allErrs
	case !validVariableTypes.Has(schema.Type):
		allErrs.SchemaErrors = append(allErrs.SchemaErrors, field.NotSupported(fldPath.Child("type"), schema.Type, sets.List(validVariableTypes)))
		return allErrs
	}

	// If the structural schema is valid, ensure a schema validator can be constructed.
	validator, _, err := validation.NewSchemaValidator(schema)
	if err != nil {
		allErrs.SchemaErrors = append(allErrs.SchemaErrors, field.Invalid(fldPath, "", fmt.Sprintf("failed to build schema validator: %v", err)))
		return allErrs
	}

	if schema.Example != nil {
		if errs := validation.ValidateCustomResource(fldPath.Child("example"), *schema.Example, validator); len(errs) > 0 {
			allErrs.SchemaErrors = append(allErrs.SchemaErrors, errs...)
		}
	}

	for i, enum := range schema.Enum {
		if enum != nil {
			if errs := validation.ValidateCustomResource(fldPath.Child("enum").Index(i), enum, validator); len(errs) > 0 {
				allErrs.SchemaErrors = append(allErrs.SchemaErrors, errs...)
			}
		}
	}

	if schema.AdditionalProperties != nil {
		if len(schema.Properties) > 0 {
			allErrs.SchemaErrors = append(allErrs.SchemaErrors, field.Forbidden(fldPath, "additionalProperties and properties are mutually exclusive"))
		}
		allErrs.AppendErrors(validateSchema(ctx, schema.AdditionalProperties.Schema, fldPath.Child("additionalProperties"), opts, celContext.ChildAdditionalPropertiesContext(schema.AdditionalProperties.Schema), uncorrelatablePath))
	}

	for propertyName, propertySchema := range schema.Properties {
		p := propertySchema
		allErrs.AppendErrors(validateSchema(ctx, &p, fldPath.Child("properties").Key(propertyName), opts, celContext.ChildPropertyContext(&p, propertyName), uncorrelatablePath))
	}

	if schema.Items != nil {
		// We cannot correlate old/new items on atomic list types, which is the only list type supported in ClusterClass variable schema.
		if uncorrelatablePath == nil {
			uncorrelatablePath = fldPath.Child("items")
		}
		allErrs.AppendErrors(validateSchema(ctx, schema.Items.Schema, fldPath.Child("items"), opts, celContext.ChildItemsContext(schema.Items.Schema), uncorrelatablePath))
	}

	// This validation is duplicated from upstream CRD validation at
	// https://github.com/kubernetes/apiextensions-apiserver/blob/v0.30.0/pkg/apis/apiextensions/validation/validation.go#L1178.
	if len(schema.XValidations) > 0 {
		// Return if schema is not a structural schema (this should enver happen, but let's handle it anyway).
		ss, err := structuralschema.NewStructural(schema)
		if err != nil {
			allErrs.SchemaErrors = append(allErrs.SchemaErrors, field.Invalid(fldPath, "", fmt.Sprintf("schema is not a structural schema: %v", err)))
			return allErrs
		}

		for i, rule := range schema.XValidations {
			trimmedRule := strings.TrimSpace(rule.Rule)
			trimmedMsg := strings.TrimSpace(rule.Message)
			trimmedMsgExpr := strings.TrimSpace(rule.MessageExpression)
			if len(trimmedRule) == 0 {
				allErrs.SchemaErrors = append(allErrs.SchemaErrors, field.Required(fldPath.Child("x-kubernetes-validations").Index(i).Child("rule"), "rule is not specified"))
			} else if len(rule.Message) > 0 && len(trimmedMsg) == 0 {
				allErrs.SchemaErrors = append(allErrs.SchemaErrors, field.Invalid(fldPath.Child("x-kubernetes-validations").Index(i).Child("message"), rule.Message, "message must be non-empty if specified"))
			} else if len(rule.Message) > 2048 {
				allErrs.SchemaErrors = append(allErrs.SchemaErrors, field.Invalid(fldPath.Child("x-kubernetes-validations").Index(i).Child("message"), rule.Message[0:10]+"...", "message must have a maximum length of 2048 characters"))
			} else if hasNewlines(trimmedMsg) {
				allErrs.SchemaErrors = append(allErrs.SchemaErrors, field.Invalid(fldPath.Child("x-kubernetes-validations").Index(i).Child("message"), rule.Message, "message must not contain line breaks"))
			} else if hasNewlines(trimmedRule) && len(trimmedMsg) == 0 {
				allErrs.SchemaErrors = append(allErrs.SchemaErrors, field.Required(fldPath.Child("x-kubernetes-validations").Index(i).Child("message"), "message must be specified if rule contains line breaks"))
			}
			if len(rule.MessageExpression) > 0 && len(trimmedMsgExpr) == 0 {
				allErrs.SchemaErrors = append(allErrs.SchemaErrors, field.Required(fldPath.Child("x-kubernetes-validations").Index(i).Child("messageExpression"), "messageExpression must be non-empty if specified"))
			}
			if rule.Reason != nil && !supportedValidationReason.Has(string(*rule.Reason)) {
				allErrs.SchemaErrors = append(allErrs.SchemaErrors, field.NotSupported(fldPath.Child("x-kubernetes-validations").Index(i).Child("reason"), *rule.Reason, supportedValidationReason.List()))
			}
			trimmedFieldPath := strings.TrimSpace(rule.FieldPath)
			if len(rule.FieldPath) > 0 && len(trimmedFieldPath) == 0 {
				allErrs.SchemaErrors = append(allErrs.SchemaErrors, field.Invalid(fldPath.Child("x-kubernetes-validations").Index(i).Child("fieldPath"), rule.FieldPath, "fieldPath must be non-empty if specified"))
			}
			if hasNewlines(rule.FieldPath) {
				allErrs.SchemaErrors = append(allErrs.SchemaErrors, field.Invalid(fldPath.Child("x-kubernetes-validations").Index(i).Child("fieldPath"), rule.FieldPath, "fieldPath must not contain line breaks"))
			}
			if len(rule.FieldPath) > 0 {
				if _, _, err := cel.ValidFieldPath(rule.FieldPath, ss); err != nil {
					allErrs.SchemaErrors = append(allErrs.SchemaErrors, field.Invalid(fldPath.Child("x-kubernetes-validations").Index(i).Child("fieldPath"), rule.FieldPath, fmt.Sprintf("fieldPath must be a valid path: %v", err)))
				}
			}
		}

		// If any schema related validation errors have been found at this level or deeper, skip CEL expression validation.
		// Invalid OpenAPISchemas are not always possible to convert into valid CEL DeclTypes, and can lead to CEL
		// validation error messages that are not actionable (will go away once the schema errors are resolved) and that
		// are difficult for CEL expression authors to understand.
		if len(allErrs.SchemaErrors) == 0 && celContext != nil {
			typeInfo, err := celContext.TypeInfo()
			if err != nil {
				allErrs.CELErrors = append(allErrs.CELErrors, field.InternalError(fldPath.Child("x-kubernetes-validations"), fmt.Errorf("internal error: failed to construct type information for x-kubernetes-validations rules: %s", err)))
			} else if typeInfo == nil {
				allErrs.CELErrors = append(allErrs.CELErrors, field.InternalError(fldPath.Child("x-kubernetes-validations"), fmt.Errorf("internal error: failed to retrieve type information for x-kubernetes-validations")))
			} else {
				// Note: k/k CRD validation also uses celconfig.PerCallLimit when creating the validator.
				// The current PerCallLimit gives roughly 0.1 second for each expression validation call.
				compResults, err := cel.Compile(typeInfo.Schema, typeInfo.DeclType, celconfig.PerCallLimit, opts.celEnvironmentSet, opts.preexistingExpressions)
				if err != nil {
					allErrs.CELErrors = append(allErrs.CELErrors, field.InternalError(fldPath.Child("x-kubernetes-validations"), err))
				} else {
					for i, cr := range compResults {
						expressionCost := getExpressionCost(cr, celContext)
						if expressionCost > StaticEstimatedCostLimit {
							costErrorMsg := getCostErrorMessage("estimated rule cost", expressionCost, StaticEstimatedCostLimit)
							allErrs.CELErrors = append(allErrs.CELErrors, field.Forbidden(fldPath.Child("x-kubernetes-validations").Index(i).Child("rule"), costErrorMsg))
						}
						if celContext.TotalCost != nil {
							celContext.TotalCost.ObserveExpressionCost(fldPath.Child("x-kubernetes-validations").Index(i).Child("rule"), expressionCost)
						}
						if cr.Error != nil {
							if cr.Error.Type == apiservercel.ErrorTypeRequired {
								allErrs.CELErrors = append(allErrs.CELErrors, field.Required(fldPath.Child("x-kubernetes-validations").Index(i).Child("rule"), cr.Error.Detail))
							} else {
								allErrs.CELErrors = append(allErrs.CELErrors, field.Invalid(fldPath.Child("x-kubernetes-validations").Index(i).Child("rule"), schema.XValidations[i], cr.Error.Detail))
							}
						}
						if cr.MessageExpressionError != nil {
							allErrs.CELErrors = append(allErrs.CELErrors, field.Invalid(fldPath.Child("x-kubernetes-validations").Index(i).Child("messageExpression"), schema.XValidations[i], cr.MessageExpressionError.Detail))
						} else if cr.MessageExpression != nil {
							if cr.MessageExpressionMaxCost > StaticEstimatedCostLimit {
								costErrorMsg := getCostErrorMessage("estimated messageExpression cost", cr.MessageExpressionMaxCost, StaticEstimatedCostLimit)
								allErrs.CELErrors = append(allErrs.CELErrors, field.Forbidden(fldPath.Child("x-kubernetes-validations").Index(i).Child("messageExpression"), costErrorMsg))
							}
							if celContext.TotalCost != nil {
								celContext.TotalCost.ObserveExpressionCost(fldPath.Child("x-kubernetes-validations").Index(i).Child("messageExpression"), cr.MessageExpressionMaxCost)
							}
						}
						if cr.UsesOldSelf {
							if uncorrelatablePath != nil {
								allErrs.CELErrors = append(allErrs.CELErrors, field.Invalid(fldPath.Child("x-kubernetes-validations").Index(i).Child("rule"), schema.XValidations[i].Rule, fmt.Sprintf("oldSelf cannot be used on the uncorrelatable portion of the schema within %v", uncorrelatablePath)))
							}
						}
					}
				}
			}
		}

		// Return if we found some errors in the CEL validations.
		if len(allErrs.AllErrors()) > 0 {
			return allErrs
		}

		// Validate example & enum values via CEL.
		// OpenAPI validation for example & enum values was already done via: validation.ValidateCustomResource
		// CEL validation for Default values was already done via: structuraldefaulting.ValidateDefaults
		celValidator := cel.NewValidator(ss, false, celconfig.PerCallLimit)
		if celValidator == nil {
			allErrs.CELErrors = append(allErrs.CELErrors, field.Invalid(fldPath.Child("x-kubernetes-validations"), "", "failed to create CEL validator"))
			return allErrs
		}
		if schema.Example != nil {
			errs, _ := celValidator.Validate(ctx, fldPath.Child("example"), ss, *schema.Example, nil, celconfig.RuntimeCELCostBudget)
			allErrs.CELErrors = append(allErrs.CELErrors, errs...)
		}
		for i, enum := range schema.Enum {
			if enum != nil {
				errs, _ := celValidator.Validate(ctx, fldPath.Child("enum").Index(i), ss, enum, nil, celconfig.RuntimeCELCostBudget)
				allErrs.CELErrors = append(allErrs.CELErrors, errs...)
			}
		}
	}

	return allErrs
}

// The following funcs are all duplicated from upstream CRD validation at
// https://github.com/kubernetes/apiextensions-apiserver/blob/v0.30.0/pkg/apis/apiextensions/validation/validation.go.

// OpenAPISchemaErrorList tracks validation errors reported
// with CEL related errors kept separate from schema related errors.
type OpenAPISchemaErrorList struct {
	SchemaErrors field.ErrorList
	CELErrors    field.ErrorList
}

// AppendErrors appends all errors in the provided list with the errors of this list.
func (o *OpenAPISchemaErrorList) AppendErrors(list *OpenAPISchemaErrorList) {
	if o == nil || list == nil {
		return
	}
	o.SchemaErrors = append(o.SchemaErrors, list.SchemaErrors...)
	o.CELErrors = append(o.CELErrors, list.CELErrors...)
}

// AllErrors returns a list containing both schema and CEL errors.
func (o *OpenAPISchemaErrorList) AllErrors() field.ErrorList {
	if o == nil {
		return field.ErrorList{}
	}
	return append(o.SchemaErrors, o.CELErrors...)
}

// validationOptions groups several validation options, to avoid passing multiple bool parameters to methods.
type validationOptions struct {
	preexistingExpressions preexistingExpressions

	celEnvironmentSet *environment.EnvSet
}

type preexistingExpressions struct {
	rules              sets.Set[string]
	messageExpressions sets.Set[string]
}

func (pe preexistingExpressions) RuleEnv(envSet *environment.EnvSet, expression string) *celgo.Env {
	if pe.rules.Has(expression) {
		return envSet.StoredExpressionsEnv()
	}
	return envSet.NewExpressionsEnv()
}

func (pe preexistingExpressions) MessageExpressionEnv(envSet *environment.EnvSet, expression string) *celgo.Env {
	if pe.messageExpressions.Has(expression) {
		return envSet.StoredExpressionsEnv()
	}
	return envSet.NewExpressionsEnv()
}

func findPreexistingExpressions(schema *apiextensions.JSONSchemaProps) preexistingExpressions {
	expressions := preexistingExpressions{rules: sets.New[string](), messageExpressions: sets.New[string]()}
	findPreexistingExpressionsInSchema(schema, expressions)
	return expressions
}

func findPreexistingExpressionsInSchema(schema *apiextensions.JSONSchemaProps, expressions preexistingExpressions) {
	apiextensionsvalidation.SchemaHas(schema, func(s *apiextensions.JSONSchemaProps) bool {
		for _, v := range s.XValidations {
			expressions.rules.Insert(v.Rule)
			if len(v.MessageExpression) > 0 {
				expressions.messageExpressions.Insert(v.MessageExpression)
			}
		}
		return false
	})
}

var newlineMatcher = regexp.MustCompile(`[\n\r]+`) // valid newline chars in CEL grammar
func hasNewlines(s string) bool {
	return newlineMatcher.MatchString(s)
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
