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

// Package yamlprocessor implements YAML processing.
package yamlprocessor

// Processor defines the methods necessary for creating a specific yaml
// processor.
type Processor interface {
	// GetTemplateName returns the file name of the template that needs to be
	// retrieved from the source.
	GetTemplateName(version, flavor string) string

	// GetClusterClassTemplateName returns the file name of the cluster class
	// template that needs to be retrieved from the source.
	GetClusterClassTemplateName(version, name string) string

	// GetVariables parses the template blob of bytes and provides a
	// list of variables that the template uses.
	GetVariables([]byte) ([]string, error)

	// GetVariableMap parses the template blob of bytes and provides a
	// map of variables that the template uses with their default values.
	GetVariableMap([]byte) (map[string]*string, error)

	// Process processes the template blob of bytes and will return the final
	// yaml with values retrieved from the values getter
	Process([]byte, func(string) (string, error)) ([]byte, error)
}
