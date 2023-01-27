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

// main is the main package for openapi-gen.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/pkg/errors"
	flag "github.com/spf13/pflag"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-tools/pkg/crd"
	"sigs.k8s.io/controller-tools/pkg/loader"
	"sigs.k8s.io/controller-tools/pkg/markers"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

var (
	paths      = flag.String("paths", "", "Paths with the variable types.")
	outputFile = flag.String("output-file", "zz_generated.variables.json", "Output file name.")
)

// FIXME: re-evaluate if we should still use openapi-gen in the other case
func main() {
	flag.Parse()

	if *paths == "" {
		klog.Exit("--paths must be specified")
	}

	if *outputFile == "" {
		klog.Exit("--output-file must be specified")
	}

	outputFileExt := path.Ext(*outputFile)
	if outputFileExt != ".json" {
		klog.Exit("--output-file must have 'json' extension")
	}

	// FIXME:
	//  * compare clusterv1.JsonSchemaProps vs kubebuilder marker if something is missing
	//   * example marker
	//  * cleanup code here

	if err := run(*paths, *outputFile); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// FIXME: Current state
	// * variable go type => apiextensionsv1.CustomResourceDefinition
	// * apiextensionsv1.CustomResourceDefinition => clusterv1.JsonSchemaProps
	// * Write schema as go structs to a file
	// * Validate: existing util (clusterv1.JsonSchemaProps) => validation result
}

func run(paths, outputFile string) error {
	crdGen := crd.Generator{}

	roots, err := loader.LoadRoots(paths)
	if err != nil {
		fmt.Println(err)
	}

	collector := &markers.Collector{
		Registry: &markers.Registry{},
	}
	if err = crdGen.RegisterMarkers(collector.Registry); err != nil {
		return err
	}
	def, err := markers.MakeAnyTypeDefinition("kubebuilder:example", markers.DescribesField, Example{})
	if err != nil {
		return err
	}
	if err := collector.Registry.Register(def); err != nil {
		return err
	}

	parser := &crd.Parser{
		Collector: collector,
		Checker: &loader.TypeChecker{
			NodeFilters: []loader.NodeFilter{crdGen.CheckFilter()},
		},
		IgnoreUnexportedFields:     true,
		AllowDangerousTypes:        false,
		GenerateEmbeddedObjectMeta: false,
	}

	crd.AddKnownTypes(parser)
	for _, root := range roots {
		parser.NeedPackage(root)
	}

	kubeKinds := []schema.GroupKind{}
	for typeIdent, _ := range parser.Types {
		// If we need another way to identify "variable structs": look at: crd.FindKubeKinds(parser, metav1Pkg)
		if typeIdent.Name == "Variables" {
			kubeKinds = append(kubeKinds, schema.GroupKind{
				Group: parser.GroupVersions[typeIdent.Package].Group,
				Kind:  typeIdent.Name,
			})
		}
	}

	// For inspiration: parser.NeedCRDFor(groupKind, nil)
	var variables []clusterv1.ClusterClassVariable
	for _, groupKind := range kubeKinds {

		// Get package for the current GroupKind
		var packages []*loader.Package
		for pkg, gv := range parser.GroupVersions {
			if gv.Group != groupKind.Group {
				continue
			}
			packages = append(packages, pkg)
		}

		var apiExtensionsSchema *apiextensionsv1.JSONSchemaProps
		for _, pkg := range packages {
			typeIdent := crd.TypeIdent{Package: pkg, Name: groupKind.Kind}
			typeInfo := parser.Types[typeIdent]

			// Didn't find type in pkg.
			if typeInfo == nil {
				continue
			}

			parser.NeedFlattenedSchemaFor(typeIdent)
			fullSchema := parser.FlattenedSchemata[typeIdent]
			apiExtensionsSchema = &*fullSchema.DeepCopy() // don't mutate the cache (we might be truncating description, etc)
		}

		if apiExtensionsSchema == nil {
			return errors.Errorf("Couldn't find schema for %s", groupKind)
		}

		for variableName, variableSchema := range apiExtensionsSchema.Properties {

			openAPIV3Schema, errs := convertToJSONSchemaProps(&variableSchema, field.NewPath("schema"))
			if len(errs) > 0 {
				// FIXME
			}

			variable := clusterv1.ClusterClassVariable{
				Name: variableName,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: *openAPIV3Schema,
				},
			}

			for _, requiredVariable := range apiExtensionsSchema.Required {
				if variableName == requiredVariable {
					variable.Required = true
				}
			}

			variables = append(variables, variable)
		}
	}

	res, err := json.Marshal(variables)
	if err != nil {
		return err
	}

	if err := os.WriteFile(outputFile, res, 0600); err != nil {
		return err
	}

	return nil
}

// JSONSchemaProps converts a apiextensions.JSONSchemaProp to a clusterv1.JSONSchemaProps.
func convertToJSONSchemaProps(schema *apiextensionsv1.JSONSchemaProps, fldPath *field.Path) (*clusterv1.JSONSchemaProps, field.ErrorList) {
	var allErrs field.ErrorList

	props := &clusterv1.JSONSchemaProps{
		Type:             schema.Type,
		Required:         schema.Required,
		MaxItems:         schema.MaxItems,
		MinItems:         schema.MinItems,
		UniqueItems:      schema.UniqueItems,
		Format:           schema.Format,
		MaxLength:        schema.MaxLength,
		MinLength:        schema.MinLength,
		Pattern:          schema.Pattern,
		ExclusiveMaximum: schema.ExclusiveMaximum,
		ExclusiveMinimum: schema.ExclusiveMinimum,
		Default:          schema.Default,
		Enum:             schema.Enum,
		Example:          schema.Example,
	}

	if schema.Maximum != nil {
		f := int64(*schema.Maximum)
		props.Maximum = &f
	}

	if schema.Minimum != nil {
		f := int64(*schema.Minimum)
		props.Minimum = &f
	}

	if schema.AdditionalProperties != nil {
		jsonSchemaProps, err := convertToJSONSchemaProps(schema.AdditionalProperties.Schema, fldPath.Child("additionalProperties"))
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("additionalProperties"), "",
				fmt.Sprintf("failed to convert schema: %v", err)))
		} else {
			props.AdditionalProperties = jsonSchemaProps
		}
	}

	if len(schema.Properties) > 0 {
		props.Properties = map[string]clusterv1.JSONSchemaProps{}
		for propertyName, propertySchema := range schema.Properties {
			p := propertySchema
			jsonSchemaProps, err := convertToJSONSchemaProps(&p, fldPath.Child("properties").Key(propertyName))
			if err != nil {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("properties").Key(propertyName), "",
					fmt.Sprintf("failed to convert schema: %v", err)))
			} else {
				props.Properties[propertyName] = *jsonSchemaProps
			}
		}
	}

	if schema.Items != nil {
		jsonSchemaProps, err := convertToJSONSchemaProps(schema.Items.Schema, fldPath.Child("items"))
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("items"), "",
				fmt.Sprintf("failed to convert schema: %v", err)))
		} else {
			props.Items = jsonSchemaProps
		}
	}

	return props, allErrs
}

type Example struct {
	Value interface{}
}
