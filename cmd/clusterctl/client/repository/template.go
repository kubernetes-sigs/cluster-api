/*
Copyright 2020 The Kubernetes Authors.

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

package repository

import (
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	yaml "sigs.k8s.io/cluster-api/cmd/clusterctl/client/yamlprocessor"
	utilyaml "sigs.k8s.io/cluster-api/util/yaml"
)

// Template wraps a YAML file that defines the cluster objects (Cluster, Machines etc.).
// It is important to notice that clusterctl applies a set of processing steps to the “raw” cluster template YAML read
// from the provider repositories:
// 1. Checks for all the variables in the cluster template YAML file and replace with corresponding config values
// 2. Ensure all the cluster objects are deployed in the target namespace.
type Template interface {
	// Variables used by the template.
	// This value is derived from the template YAML.
	Variables() []string

	// VariableMap used by the template with their default values. If the value is `nil`, there is no
	// default and the variable is required.
	// This value is derived from the template YAML.
	VariableMap() map[string]*string

	// TargetNamespace where the template objects will be installed.
	TargetNamespace() string

	// Yaml returns yaml defining all the cluster template objects as a byte array.
	Yaml() ([]byte, error)

	// Objs returns the cluster template as a list of Unstructured objects.
	Objs() []unstructured.Unstructured
}

// template implements Template.
type template struct {
	variables       []string
	variableMap     map[string]*string
	targetNamespace string
	objs            []unstructured.Unstructured
}

// Ensures template implements the Template interface.
var _ Template = &template{}

func (t *template) Variables() []string {
	return t.variables
}

func (t *template) VariableMap() map[string]*string {
	return t.variableMap
}

func (t *template) TargetNamespace() string {
	return t.targetNamespace
}

func (t *template) Objs() []unstructured.Unstructured {
	return t.objs
}

func (t *template) Yaml() ([]byte, error) {
	return utilyaml.FromUnstructured(t.objs)
}

// TemplateInput is an input struct for NewTemplate.
type TemplateInput struct {
	RawArtifact           []byte
	ConfigVariablesClient config.VariablesClient
	Processor             yaml.Processor
	TargetNamespace       string
	SkipTemplateProcess   bool
}

// NewTemplate returns a new objects embedding a cluster template YAML file.
func NewTemplate(input TemplateInput) (Template, error) {
	variables, err := input.Processor.GetVariables(input.RawArtifact)
	if err != nil {
		return nil, err
	}

	variableMap, err := input.Processor.GetVariableMap(input.RawArtifact)
	if err != nil {
		return nil, err
	}

	if input.SkipTemplateProcess {
		return &template{
			variables:       variables,
			variableMap:     variableMap,
			targetNamespace: input.TargetNamespace,
		}, nil
	}

	processedYaml, err := input.Processor.Process(input.RawArtifact, input.ConfigVariablesClient.Get)
	if err != nil {
		return nil, err
	}

	// Transform the yaml in a list of objects, so following transformation can work on typed objects (instead of working on a string/slice of bytes).
	objs, err := utilyaml.ToUnstructured(processedYaml)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse yaml")
	}

	if input.TargetNamespace != "" {
		// Ensures all the template components are deployed in the target namespace (applies only to namespaced objects)
		// This is required in order to ensure a cluster and all the related objects are in a single namespace, that is a requirement for
		// the clusterctl move operation (and also for many controller reconciliation loops).
		objs, err = fixTargetNamespace(objs, input.TargetNamespace)
		if err != nil {
			return nil, errors.Wrap(err, "failed to set the TargetNamespace in the template")
		}
	}

	return &template{
		variables:       variables,
		variableMap:     variableMap,
		targetNamespace: input.TargetNamespace,
		objs:            objs,
	}, nil
}

// MergeTemplates merges the provided Templates into one Template.
// Notes on the merge operation:
// - The merge operation returns an error if all the templates do not have the same TargetNamespace.
// - The Variables of the resulting template is a union of all Variables in the templates.
// - The default value is picked from the first template that defines it.
//    The defaults of the same variable in the subsequent templates will be ignored.
//    (e.g when merging a cluster template and its ClusterClass, the default value from the template takes precedence)
// - The Objs of the final template will be a union of all the Objs in the templates.
func MergeTemplates(templates ...Template) (Template, error) {
	templates = filterNilTemplates(templates...)
	if len(templates) == 0 {
		return nil, nil
	}

	merged := &template{
		variables:       []string{},
		variableMap:     map[string]*string{},
		objs:            []unstructured.Unstructured{},
		targetNamespace: templates[0].TargetNamespace(),
	}

	for _, tmpl := range templates {
		merged.variables = sets.NewString(merged.variables...).Union(sets.NewString(tmpl.Variables()...)).List()

		for key, val := range tmpl.VariableMap() {
			if v, ok := merged.variableMap[key]; !ok || v == nil {
				merged.variableMap[key] = val
			}
		}

		if merged.targetNamespace != tmpl.TargetNamespace() {
			return nil, fmt.Errorf("cannot merge templates with different targetNamespaces")
		}

		merged.objs = append(merged.objs, tmpl.Objs()...)
	}

	return merged, nil
}

func filterNilTemplates(templates ...Template) []Template {
	res := []Template{}
	for _, tmpl := range templates {
		if tmpl != nil {
			res = append(res, tmpl)
		}
	}
	return res
}
