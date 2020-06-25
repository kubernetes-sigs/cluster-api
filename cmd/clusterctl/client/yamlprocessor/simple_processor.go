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

package yamlprocessor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
)

// SimpleProcessor is a yaml processor that does simple variable substitution.
// It implements the YamlProcessor interface. The variables are defined in
// the following format ${variable_name}
type SimpleProcessor struct{}

var _ Processor = &SimpleProcessor{}

func NewSimpleProcessor() *SimpleProcessor {
	return &SimpleProcessor{}
}

// GetTemplateName returns the name of the template that the simple processor
// uses. It follows the cluster template naming convention of
// "cluster-template<-flavor>.yaml".
func (tp *SimpleProcessor) GetTemplateName(_, flavor string) string {
	name := "cluster-template"
	if flavor != "" {
		name = fmt.Sprintf("%s-%s", name, flavor)
	}
	name = fmt.Sprintf("%s.yaml", name)

	return name
}

// GetVariables returns a list of the variables specified in the yaml
// manifest. The format of the variables being parsed is ${VAR}
func (tp *SimpleProcessor) GetVariables(rawArtifact []byte) ([]string, error) {
	return inspectVariables(rawArtifact), nil
}

// Process returns the final yaml with all the variables replaced with their
// respective values. If there are variables without corresponding values, it
// will return the yaml along with an error.
func (tp *SimpleProcessor) Process(rawArtifact []byte, variablesGetter func(string) (string, error)) ([]byte, error) {
	// Inspect the yaml read from the repository for variables.
	variables := inspectVariables(rawArtifact)

	// Replace variables with corresponding values read from the config
	tmp := string(rawArtifact)
	var err error
	var missingVariables []string
	for _, key := range variables {
		val, err := variablesGetter(key)
		if err != nil {
			missingVariables = append(missingVariables, key)
			continue
		}
		exp := regexp.MustCompile(`\$\{\s*` + regexp.QuoteMeta(key) + `\s*\}`)
		tmp = exp.ReplaceAllLiteralString(tmp, val)
	}
	if len(missingVariables) > 0 {
		err = errors.Errorf("value for variables [%s] is not set. Please set the value using os environment variables or the clusterctl config file", strings.Join(missingVariables, ", "))
	}

	return []byte(tmp), err
}

// variableRegEx defines the regexp used for searching variables inside a YAML
var variableRegEx = regexp.MustCompile(`\${\s*([A-Z0-9_$]+)\s*}`)

func inspectVariables(data []byte) []string {
	variables := sets.NewString()
	match := variableRegEx.FindAllStringSubmatch(string(data), -1)

	for _, m := range match {
		submatch := m[1]
		if !variables.Has(submatch) {
			variables.Insert(submatch)
		}
	}

	return variables.List()
}
