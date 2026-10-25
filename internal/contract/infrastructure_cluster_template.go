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

package contract

import (
	"sync"
)

// InfrastructureClusterTemplateContract encodes information about the Cluster API contract for InfrastructureClusterTemplate objects
// like DockerClusterTemplates, AWSClusterTemplates, etc.
type InfrastructureClusterTemplateContract struct{}

var infrastructureClusterTemplate *InfrastructureClusterTemplateContract
var onceInfrastructureClusterTemplate sync.Once

// InfrastructureClusterTemplate provides access to the information about the Cluster API contract for InfrastructureClusterTemplate objects.
func InfrastructureClusterTemplate() *InfrastructureClusterTemplateContract {
	onceInfrastructureClusterTemplate.Do(func() {
		infrastructureClusterTemplate = &InfrastructureClusterTemplateContract{}
	})
	return infrastructureClusterTemplate
}

// Template provides access to the template.
func (c *InfrastructureClusterTemplateContract) Template() *InfrastructureClusterTemplateTemplate {
	return &InfrastructureClusterTemplateTemplate{}
}

// InfrastructureClusterTemplateTemplate provides a helper struct for working with the template in an InfrastructureClusterTemplate..
type InfrastructureClusterTemplateTemplate struct{}

// Metadata provides access to the metadata of a template.
func (c *InfrastructureClusterTemplateTemplate) Metadata() *Metadata {
	return &Metadata{
		path: Path{"spec", "template", "metadata"},
	}
}
