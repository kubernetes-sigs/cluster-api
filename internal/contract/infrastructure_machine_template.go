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

// InfrastructureMachineTemplateContract encodes information about the Cluster API contract for InfrastructureMachineTemplate objects
// like DockerMachineTemplates, AWSMachineTemplates, etc.
type InfrastructureMachineTemplateContract struct{}

var (
	infrastructureMachineTemplate     *InfrastructureMachineTemplateContract
	onceInfrastructureMachineTemplate sync.Once
)

// InfrastructureMachineTemplate provide access to the information about the Cluster API contract for InfrastructureMachineTemplate objects.
func InfrastructureMachineTemplate() *InfrastructureMachineTemplateContract {
	onceInfrastructureMachineTemplate.Do(func() {
		infrastructureMachineTemplate = &InfrastructureMachineTemplateContract{}
	})
	return infrastructureMachineTemplate
}

// Template provides access to the template.
func (c *InfrastructureMachineTemplateContract) Template() *InfrastructureMachineTemplateTemplate {
	return &InfrastructureMachineTemplateTemplate{}
}

// InfrastructureMachineTemplateTemplate provides a helper struct for working with the template in an InfrastructureMachineTemplate.
type InfrastructureMachineTemplateTemplate struct{}

// Metadata provides access to the metadata of a template.
func (c *InfrastructureMachineTemplateTemplate) Metadata() *Metadata {
	return &Metadata{
		path: Path{"spec", "template", "metadata"},
	}
}
