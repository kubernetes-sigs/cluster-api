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

import "sync"

// ControlPlaneTemplateContract encodes information about the Cluster API contract for ControlPlaneTemplate objects
// like e.g. the KubeadmControlPlane etc.
type ControlPlaneTemplateContract struct{}

var controlPlaneTemplate *ControlPlaneTemplateContract
var onceControlPlaneTemplate sync.Once

// ControlPlaneTemplate provide access to the information about the Cluster API contract for ControlPlaneTemplate objects.
func ControlPlaneTemplate() *ControlPlaneTemplateContract {
	onceControlPlaneTemplate.Do(func() {
		controlPlaneTemplate = &ControlPlaneTemplateContract{}
	})
	return controlPlaneTemplate
}

// Template provides access to the template.
func (c *ControlPlaneTemplateContract) Template() *ControlPlaneTemplateTemplate {
	return &ControlPlaneTemplateTemplate{}
}

// ControlPlaneTemplateTemplate provides a helper struct for working with the template in an ControlPlaneTemplate.
type ControlPlaneTemplateTemplate struct{}

// Metadata provides access to the metadata of a template.
func (c *ControlPlaneTemplateTemplate) Metadata() *Metadata {
	return &Metadata{
		path: Path{"spec", "template", "metadata"},
	}
}

// MachineTemplate provides access to MachineTemplate in a ControlPlaneTemplate object, if any.
func (c *ControlPlaneTemplateTemplate) MachineTemplate() *ControlPlaneTemplateMachineTemplate {
	return &ControlPlaneTemplateMachineTemplate{}
}

// ControlPlaneTemplateMachineTemplate provides a helper struct for working with MachineTemplate.
type ControlPlaneTemplateMachineTemplate struct{}

// Metadata provides access to the metadata of the MachineTemplate of a ControlPlaneTemplate.
func (c *ControlPlaneTemplateMachineTemplate) Metadata() *Metadata {
	return &Metadata{
		path: Path{"spec", "template", "spec", "machineTemplate", "metadata"},
	}
}

// NodeDrainTimeout provides access to the nodeDrainTimeout of a MachineTemplate.
func (c *ControlPlaneTemplateMachineTemplate) NodeDrainTimeout() *Duration {
	return &Duration{
		path: Path{"spec", "template", "spec", "machineTemplate", "nodeDrainTimeout"},
	}
}

// NodeVolumeDetachTimeout provides access to the nodeVolumeDetachTimeout of a MachineTemplate.
func (c *ControlPlaneTemplateMachineTemplate) NodeVolumeDetachTimeout() *Duration {
	return &Duration{
		path: Path{"spec", "template", "spec", "machineTemplate", "nodeVolumeDetachTimeout"},
	}
}

// NodeDeletionTimeout provides access to the nodeDeletionTimeout of a MachineTemplate.
func (c *ControlPlaneTemplateMachineTemplate) NodeDeletionTimeout() *Duration {
	return &Duration{
		path: Path{"spec", "template", "spec", "machineTemplate", "nodeDeletionTimeout"},
	}
}
