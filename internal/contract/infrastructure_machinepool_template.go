/*
Copyright 2022 The Kubernetes Authors.

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

package contract

import (
	"sync"
)

// InfrastructureMachinePoolTemplateContract encodes information about the Cluster API contract for InfrastructureMachinePoolTemplate objects
// like DockerMachinePoolTemplates, AWSMachinePoolTemplates, etc.
type InfrastructureMachinePoolTemplateContract struct{}

var infrastructureMachinePoolTemplate *InfrastructureMachinePoolTemplateContract
var onceInfrastructureMachinePoolTemplate sync.Once

// InfrastructureMachinePoolTemplate provide access to the information about the Cluster API contract for InfrastructureMachinePoolTemplate objects.
func InfrastructureMachinePoolTemplate() *InfrastructureMachinePoolTemplateContract {
	onceInfrastructureMachinePoolTemplate.Do(func() {
		infrastructureMachinePoolTemplate = &InfrastructureMachinePoolTemplateContract{}
	})
	return infrastructureMachinePoolTemplate
}

// Template provides access to the template.
func (c *InfrastructureMachinePoolTemplateContract) Template() *InfrastructureMachinePoolTemplateTemplate {
	return &InfrastructureMachinePoolTemplateTemplate{}
}

// InfrastructureMachinePoolTemplateTemplate provides a helper struct for working with the template in an InfrastructureMachinePoolTemplate.
type InfrastructureMachinePoolTemplateTemplate struct{}

// Metadata provides access to the metadata of a template.
func (c *InfrastructureMachinePoolTemplateTemplate) Metadata() *Metadata {
	return &Metadata{
		path: Path{"spec", "template", "metadata"},
	}
}
