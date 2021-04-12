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

// variableMetaData stores yaml value information.
type variableMetadata struct {
	name         string
	required     bool
	defaultValue string
}

// VariableMetadata ensures variableMetaData implement VariableMetadata.
type VariableMetadata interface {
	// configuration of the provider the provider components belongs to.
	Name() string
	Required() bool
	DefaultValue() string
}

var _ VariableMetadata = &variableMetadata{}

func (v *variableMetadata) Name() string {
	return v.name
}

func (v *variableMetadata) Required() bool {
	return v.required
}

func (v *variableMetadata) DefaultValue() string {
	return v.defaultValue
}

// implement sorting using name property.
type ByName []VariableMetadata

func (a ByName) Len() int           { return len(a) }
func (a ByName) Less(i, j int) bool { return a[i].Name() < a[j].Name() }
func (a ByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
