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
	"sort"
	"strings"

	"github.com/drone/envsubst"
	"github.com/drone/envsubst/parse"
)

// SimpleProcessor is a yaml processor that uses envsubst to substitute values
// for variables in the format ${var}. It also allows default values if
// specified in the format ${var:=default}.
// See https://github.com/drone/envsubst for more details.
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

// GetVariables returns a list of the variables specified in the yaml.
func (tp *SimpleProcessor) GetVariables(rawArtifact []byte) ([]string, error) {
	strArtifact := convertLegacyVars(string(rawArtifact))

	variables, err := inspectVariables(strArtifact)
	if err != nil {
		return nil, err
	}

	varNames := make([]string, 0, len(variables))
	for k := range variables {
		varNames = append(varNames, k)
	}
	sort.Strings(varNames)
	return varNames, nil
}

// Process returns the final yaml with all the variables replaced with their
// respective values. If there are variables without corresponding values, it
// will return the raw yaml along with an error.
func (tp *SimpleProcessor) Process(rawArtifact []byte, variablesClient func(string) (string, error)) ([]byte, error) {
	tmp := convertLegacyVars(string(rawArtifact))
	// Inspect the yaml read from the repository for variables.
	variables, err := inspectVariables(tmp)
	if err != nil {
		return rawArtifact, err
	}

	var missingVariables []string
	// keep track of missing variables to return as error later
	for name, hasDefault := range variables {
		_, err := variablesClient(name)
		// add to missingVariables list if the variable does not exist in the
		// variablesClient AND it does not have a default value
		if err != nil && !hasDefault {
			missingVariables = append(missingVariables, name)
			continue
		}
	}

	if len(missingVariables) > 0 {
		return rawArtifact, &errMissingVariables{missingVariables}
	}

	tmp, err = envsubst.Eval(tmp, func(in string) string {
		v, _ := variablesClient(in)
		return v
	})
	if err != nil {
		return rawArtifact, err
	}

	return []byte(tmp), err
}

type errMissingVariables struct {
	Missing []string
}

func (e *errMissingVariables) Error() string {
	sort.Strings(e.Missing)
	return fmt.Sprintf(
		"value for variables [%s] is not set. Please set the value using os environment variables or the clusterctl config file",
		strings.Join(e.Missing, ", "),
	)
}

// inspectVariables parses through the yaml and returns a map of the variable
// names and if they have default values. It returns an error if it cannot
// parse the yaml.
func inspectVariables(data string) (map[string]bool, error) {
	variables := make(map[string]bool)
	t, err := parse.Parse(data)
	if err != nil {
		return nil, err
	}
	traverse(t.Root, variables)
	return variables, nil
}

// traverse recursively walks down the root node and tracks the variables
// which are FuncNodes and if the variables have default values.
func traverse(root parse.Node, variables map[string]bool) {
	switch v := root.(type) {
	case *parse.ListNode:
		// iterate through the list node
		for _, ln := range v.Nodes {
			traverse(ln, variables)
		}
	case *parse.FuncNode:
		if _, ok := variables[v.Param]; !ok {
			// if there are args, then the variable has a default value
			variables[v.Param] = len(v.Args) > 0
		}
	}
}

// legacyVariableRegEx defines the regexp used for searching variables inside a YAML.
// It searches for variables with the format ${ VAR}, ${ VAR }, ${VAR }
var legacyVariableRegEx = regexp.MustCompile(`(\${(\s+([A-Za-z0-9_$]+)\s+)})|(\${(\s+([A-Za-z0-9_$]+))})|(\${(([A-Za-z0-9_$]+)\s+)})`)
var whitespaceRegEx = regexp.MustCompile(`\s`)

// convertLegacyVars parses through the yaml string and modifies it replacing
// variables with the format ${ VAR}, ${ VAR }, ${VAR } to ${VAR}. This is
// done to maintain backwards compatibility.
func convertLegacyVars(data string) string {
	return legacyVariableRegEx.ReplaceAllStringFunc(data, func(o string) string {
		return whitespaceRegEx.ReplaceAllString(o, "")
	})
}
