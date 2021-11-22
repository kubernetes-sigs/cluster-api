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

// Package inline implements the inline JSON patch generator.
package inline

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"text/template"

	"github.com/pkg/errors"
	"github.com/valyala/fastjson"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/extensions/patches/api"
	patchvariables "sigs.k8s.io/cluster-api/controllers/topology/internal/extensions/patches/variables"
	"sigs.k8s.io/yaml"
)

// jsonPatchGenerator generates JSON patches for a GenerateRequest based on a ClusterClassPatch.
type jsonPatchGenerator struct {
	patch *clusterv1.ClusterClassPatch
}

// New returns a new inline Generator from a given ClusterClassPatch object.
func New(patch *clusterv1.ClusterClassPatch) api.Generator {
	return &jsonPatchGenerator{
		patch: patch,
	}
}

// Generate generates JSON patches for the given GenerateRequest based on a ClusterClassPatch.
func (j *jsonPatchGenerator) Generate(_ context.Context, req *api.GenerateRequest) (*api.GenerateResponse, error) {
	resp := &api.GenerateResponse{}

	// Loop over all templates.
	errs := []error{}
	for _, template := range req.Items {
		// Calculate the list of patches which match the current template.
		matchingPatches := []clusterv1.PatchDefinition{}
		for _, patch := range j.patch.Definitions {
			// Add the patch to the list, if it matches the template.
			if templateMatchesSelector(&template.TemplateRef, patch.Selector) {
				matchingPatches = append(matchingPatches, patch)
			}
		}

		// Continue if there are no matching patches.
		if len(matchingPatches) == 0 {
			continue
		}

		// Merge template-specific and global variables.
		variables, err := mergeVariableMaps(req.Variables, template.Variables)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "failed to merge global and template-specific variables for template %s", template.TemplateRef))
			continue
		}

		// Loop over all PatchDefinitions.
		for _, patch := range matchingPatches {
			// Generate JSON patches.
			jsonPatches, err := generateJSONPatches(patch.JSONPatches, variables)
			if err != nil {
				errs = append(errs, errors.Wrapf(err, "failed to generate JSON patches for template %s", template.TemplateRef))
				continue
			}

			// Add jsonPatches to the response.
			resp.Items = append(resp.Items, api.GenerateResponsePatch{
				TemplateRef: template.TemplateRef,
				Patch:       *jsonPatches,
				PatchType:   api.JSONPatchType,
			})
		}
	}

	return resp, kerrors.NewAggregate(errs)
}

// templateMatchesSelector returns true if the template matches the selector.
func templateMatchesSelector(templateRef *api.TemplateRef, selector clusterv1.PatchSelector) bool {
	// Check if the apiVersion and kind are matching.
	if templateRef.APIVersion != selector.APIVersion {
		return false
	}
	if templateRef.Kind != selector.Kind {
		return false
	}

	// Check if target matches.
	switch templateRef.TemplateType {
	case api.InfrastructureClusterTemplateType:
		// Check if matchSelector.infrastructureCluster is true.
		if selector.MatchResources.InfrastructureCluster == nil {
			return false
		}
		return *selector.MatchResources.InfrastructureCluster
	case api.ControlPlaneTemplateType, api.ControlPlaneInfrastructureMachineTemplateType:
		// Check if matchSelector.controlPlane is true.
		if selector.MatchResources.ControlPlane == nil {
			return false
		}
		return *selector.MatchResources.ControlPlane
	case api.MachineDeploymentBootstrapConfigTemplateType, api.MachineDeploymentInfrastructureMachineTemplateType:
		// Check if matchSelector.machineDeploymentClass.names contains the
		// MachineDeployment.Class of the template.
		if selector.MatchResources.MachineDeploymentClass == nil {
			return false
		}
		for _, name := range selector.MatchResources.MachineDeploymentClass.Names {
			if name == templateRef.MachineDeploymentRef.Class {
				return true
			}
		}
		return false
	default:
		// Return false if the TargetType is unknown.
		return false
	}
}

// jsonPatchRFC6902 is used to render the generated JSONPatches.
type jsonPatchRFC6902 struct {
	Op    string               `json:"op"`
	Path  string               `json:"path"`
	Value apiextensionsv1.JSON `json:"value"`
}

// generateJSONPatches generates JSON patches based on the given JSONPatches and variables.
func generateJSONPatches(jsonPatches []clusterv1.JSONPatch, variables map[string]apiextensionsv1.JSON) (*apiextensionsv1.JSON, error) {
	res := []jsonPatchRFC6902{}

	for _, jsonPatch := range jsonPatches {
		value, err := calculateValue(jsonPatch, variables)
		if err != nil {
			return nil, err
		}

		res = append(res, jsonPatchRFC6902{
			Op:    jsonPatch.Op,
			Path:  jsonPatch.Path,
			Value: *value,
		})
	}

	// Render JSON Patches.
	resJSON, err := json.Marshal(res)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal JSON Patch %v", jsonPatches)
	}

	return &apiextensionsv1.JSON{Raw: resJSON}, nil
}

// calculateValue calculates a value for a JSON patch.
func calculateValue(patch clusterv1.JSONPatch, variables map[string]apiextensionsv1.JSON) (*apiextensionsv1.JSON, error) {
	// Return if values are set incorrectly.
	if patch.Value == nil && patch.ValueFrom == nil {
		return nil, errors.Errorf("failed to calculate value: neither .value nor .valueFrom are set")
	}
	if patch.Value != nil && patch.ValueFrom != nil {
		return nil, errors.Errorf("failed to calculate value: both .value and .valueFrom are set")
	}
	if patch.ValueFrom != nil && patch.ValueFrom.Variable == nil && patch.ValueFrom.Template == nil {
		return nil, errors.Errorf("failed to calculate value: .valueFrom is set, but neither .valueFrom.variable nor .valueFrom.template are set")
	}
	if patch.ValueFrom != nil && patch.ValueFrom.Variable != nil && patch.ValueFrom.Template != nil {
		return nil, errors.Errorf("failed to calculate value: .valueFrom is set, but both .valueFrom.variable and .valueFrom.template are set")
	}

	// Return raw value.
	if patch.Value != nil {
		return patch.Value, nil
	}

	// Return variable.
	if patch.ValueFrom.Variable != nil {
		value, err := getVariableValue(variables, *patch.ValueFrom.Variable)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to calculate value")
		}
		return value, nil
	}

	// Return rendered value template.
	value, err := renderValueTemplate(*patch.ValueFrom.Template, variables)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to calculate value")
	}
	return value, nil
}

const (
	leftArrayDelim  = "["
	rightArrayDelim = "]"
)

// getVariableValue returns a variable from the variables map.
func getVariableValue(variables map[string]apiextensionsv1.JSON, variableName string) (*apiextensionsv1.JSON, error) {
	// If the variable is a top-level variable, just do a simple lookup.
	if !strings.Contains(variableName, ".") {
		value, ok := variables[variableName]
		if !ok {
			return nil, errors.Errorf("variable %q does not exist", variableName)
		}
		return &value, nil
	}

	// If the variable is a nested variable we inspect the variable object.

	// Get the variable object.
	value, ok := variables[strings.Split(variableName, ".")[0]]
	if !ok {
		return nil, errors.Errorf("variable %q does not exist", variableName)
	}

	// Parse the variable object.
	variable, err := fastjson.ParseBytes(value.Raw)
	if err != nil {
		return nil, errors.Errorf("cannot parse variable %q", variableName)
	}

	// Split the variable name and exclude the first part ("<variableName>.")
	relativePath := strings.Split(variableName, ".")[1:]

	for _, pathSegment := range relativePath {
		if (strings.Contains(pathSegment, leftArrayDelim) && !strings.Contains(pathSegment, rightArrayDelim)) ||
			(!strings.Contains(pathSegment, leftArrayDelim) && strings.Contains(pathSegment, rightArrayDelim)) {
			return nil, errors.Errorf("variable name %q has invalid syntax", variableName)
		}

		// TODO: move this into a separate func + more unit tests
		arrayIndex := -1
		if strings.Contains(pathSegment, leftArrayDelim) && strings.Contains(pathSegment, rightArrayDelim) {
			arrayIndexStr := pathSegment[strings.Index(pathSegment, leftArrayDelim)+1 : strings.Index(pathSegment, rightArrayDelim)]
			index, err := strconv.Atoi(arrayIndexStr)
			if err != nil {
				return nil, errors.Wrapf(err, "variable name %q has invalid syntax: failed to parse array index", variableName)
			}
			if index < 0 {
				return nil, errors.Wrapf(err, "variable name %q has invalid syntax: invalid array index %d", variableName, index)
			}

			arrayIndex = index
			pathSegment = pathSegment[:strings.Index(pathSegment, leftArrayDelim)] //nolint:gocritic // We already check above that pathSegment contains leftArrayDelim
		}

		// Return if the variable does not exist.
		if !variable.Exists(pathSegment) {
			return nil, errors.Errorf("variable %q does not exist", variableName)
		}

		// Get the variable from the variable object.
		if arrayIndex == -1 {
			// arrayIndex not set.
			variable = variable.Get(pathSegment)
			continue
		}

		// arrayIndex is set.
		arr, err := variable.Get(pathSegment).Array()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to calculate variable %q", variableName)
		}

		if len(arr) < arrayIndex+1 {
			return nil, errors.Errorf("failed to calculate variable: array does not have index %d", arrayIndex)
		}

		variable = arr[arrayIndex]
	}

	// Return the marshalled value of the variable.
	return &apiextensionsv1.JSON{
		Raw: variable.MarshalTo([]byte{}),
	}, nil
}

// renderValueTemplate renders a template with the given variables as data.
func renderValueTemplate(valueTemplate string, variables map[string]apiextensionsv1.JSON) (*apiextensionsv1.JSON, error) {
	// Parse the template.
	tpl, err := template.New("tpl").Parse(valueTemplate)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse template: %q", valueTemplate)
	}

	// Convert the flat variables map in a nested map, so that variables can be
	// consumed in templates like this: `{{ .builtin.cluster.name }}`
	// NOTE: Variable values are also converted to their Go types as
	// they cannot be directly consumed as byte arrays.
	data, err := calculateTemplateData(variables)
	if err != nil {
		return nil, errors.Wrap(err, "failed to calculate template data")
	}

	// Render the template.
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return nil, errors.Wrapf(err, "failed to render template: %q", valueTemplate)
	}

	// Unmarshal the rendered template.
	// NOTE: The YAML library is used for unmarshalling, to be able to handle YAML and JSON.
	value := apiextensionsv1.JSON{}
	if err := yaml.Unmarshal(buf.Bytes(), &value); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal rendered template: %q", buf.String())
	}

	return &value, nil
}

// calculateTemplateData calculates data for the template, by converting
// the variables to their Go types.
// Example:
// * Input:
//   map[string]apiextensionsv1.JSON{
//     "builtin": {Raw: []byte(`{"cluster":{"name":"cluster-name"}}`},
//     "integerVariable": {Raw: []byte("4")},
//     "numberVariable": {Raw: []byte("2.5")},
//     "booleanVariable": {Raw: []byte("true")},
//   }
// * Output:
//   map[string]interface{}{
//     "builtin": map[string]interface{}{
//       "cluster": map[string]interface{}{
//         "name": <string>"cluster-name"
//       }
//     },
//     "integerVariable": <float64>4,
//     "numberVariable": <float64>2.5,
//     "booleanVariable": <bool>true,
//   }
func calculateTemplateData(variables map[string]apiextensionsv1.JSON) (map[string]interface{}, error) {
	res := make(map[string]interface{}, len(variables))

	// Marshal the variables into a byte array.
	tmp, err := json.Marshal(variables)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert variables: failed to marshal variables")
	}

	// Unmarshal the byte array back.
	// NOTE: This converts the "leaf nodes" of the nested map
	// from apiextensionsv1.JSON to their Go types.
	if err := json.Unmarshal(tmp, &res); err != nil {
		return nil, errors.Wrapf(err, "failed to convert variables: failed to unmarshal variables")
	}

	return res, nil
}

// mergeVariableMaps merges variables.
// NOTE: In case a variable exists in multiple maps, the variable from the latter map is preserved.
// NOTE: The builtin variable object is merged instead of simply overwritten.
func mergeVariableMaps(variableMaps ...map[string]apiextensionsv1.JSON) (map[string]apiextensionsv1.JSON, error) {
	res := make(map[string]apiextensionsv1.JSON)

	for _, variableMap := range variableMaps {
		for variableName, variableValue := range variableMap {
			// If the variable already exits and is the builtin variable, merge it.
			if _, ok := res[variableName]; ok && variableName == patchvariables.BuiltinsName {
				mergedV, err := mergeBuiltinVariables(res[variableName], variableValue)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to merge builtin variables")
				}
				res[variableName] = *mergedV
				continue
			}
			res[variableName] = variableValue
		}
	}

	return res, nil
}

// mergeBuiltinVariables merges builtin variable objects.
// NOTE: In case a variable exists in multiple builtin variables, the variable from the latter map is preserved.
func mergeBuiltinVariables(variableList ...apiextensionsv1.JSON) (*apiextensionsv1.JSON, error) {
	builtins := &patchvariables.Builtins{}

	// Unmarshal all variables into builtins.
	// NOTE: This accumulates the fields on the builtins.
	// Fields will be overwritten by later Unmarshals if fields are
	// set on multiple variables.
	for _, variable := range variableList {
		if err := json.Unmarshal(variable.Raw, builtins); err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal builtin variable")
		}
	}

	// Marshal builtins to JSON.
	builtinVariableJSON, err := json.Marshal(builtins)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal builtin variable")
	}

	return &apiextensionsv1.JSON{
		Raw: builtinVariableJSON,
	}, nil
}
