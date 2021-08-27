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

// InfrastructureMachineTemplate provide access to InfrastructureMachineTemplate reference, if any.
// NOTE: When working with unstructured there is no way to understand if the ControlPlane provider
// do support a field in the type definition from the fact that a field is not set in a given instance.
// This is why in we are deriving if this field is required from the ClusterClass in the topology reconciler code.
func (c *ControlPlaneTemplateContract) InfrastructureMachineTemplate() *Ref {
	return &Ref{
		path: Path{"spec", "template", "spec", "machineTemplate", "infrastructureRef"},
	}
}
