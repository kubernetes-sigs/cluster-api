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

// Package api contains the API definition for the patch engine.
// NOTE: We are introducing this API as a decoupling layer between the patch engine and the concrete components
// responsible for generating patches, because we aim to provide support for external patches in a future iteration.
// We also assume that this API and all the related types will be moved in a separate (versioned) package thus
// providing a versioned contract between Cluster API and the components implementing external patch extensions.
package api

import (
	"context"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// Generator defines a component that can generate patches for ClusterClass templates.
type Generator interface {
	// Generate generates patches for templates.
	// GenerateRequest contains templates and the corresponding variables.
	// GenerateResponse contains the patches which should be applied to the templates of the GenerateRequest.
	Generate(ctx context.Context, request *GenerateRequest) (*GenerateResponse, error)
}

// GenerateRequest defines the input for a Generate request.
type GenerateRequest struct {
	// Variables is a name/value map containing variables.
	Variables map[string]apiextensionsv1.JSON

	// Items contains the list of templates to generate patches for.
	Items []*GenerateRequestTemplate
}

// GenerateRequestTemplate defines one of the ClusterClass templates to generate patches for.
type GenerateRequestTemplate struct {
	// TemplateRef identifies a template to generate patches for;
	// the same TemplateRef must be used when specifying to which template a generated patch should be applied to.
	TemplateRef TemplateRef

	// Variables is a name/value map containing variables specifically for the current template.
	// For example some builtin variables like MachineDeployment replicas and version are context-sensitive
	// and thus are only added to templates for MachineDeployments and with values which correspond to the
	// current MachineDeployment.
	Variables map[string]apiextensionsv1.JSON

	// Template contains the template.
	Template apiextensionsv1.JSON
}

// TemplateRef identifies one of the ClusterClass templates to generate patches for;
// the same TemplateRef must be used when specifying where a generated patch should apply to.
type TemplateRef struct {
	// APIVersion of the current template.
	APIVersion string

	// Kind of the current template.
	Kind string

	// TemplateType defines where the template is used.
	TemplateType TemplateType

	// MachineDeployment specifies the MachineDeployment in which the template is used.
	// This field is only set if the template is used in the context of a MachineDeployment.
	MachineDeploymentRef MachineDeploymentRef
}

func (t TemplateRef) String() string {
	ret := fmt.Sprintf("%s %s/%s", t.TemplateType, t.APIVersion, t.Kind)
	if t.MachineDeploymentRef.TopologyName != "" {
		ret = fmt.Sprintf("%s, MachineDeployment topology %s", ret, t.MachineDeploymentRef.TopologyName)
	}
	if t.MachineDeploymentRef.Class != "" {
		ret = fmt.Sprintf("%s, MachineDeployment class %s", ret, t.MachineDeploymentRef.Class)
	}
	return ret
}

// MachineDeploymentRef specifies the MachineDeployment in which the template is used.
type MachineDeploymentRef struct {
	// TopologyName is the name of the MachineDeploymentTopology.
	TopologyName string

	// Class is the name of the MachineDeploymentClass.
	Class string
}

// TemplateType define the type for target types enum.
type TemplateType string

const (
	// InfrastructureClusterTemplateType identifies a template for the InfrastructureCluster object.
	InfrastructureClusterTemplateType TemplateType = "InfrastructureClusterTemplate"

	// ControlPlaneTemplateType identifies a template for the ControlPlane object.
	ControlPlaneTemplateType TemplateType = "ControlPlaneTemplate"

	// ControlPlaneInfrastructureMachineTemplateType identifies a template for the InfrastructureMachines to be used for the ControlPlane object.
	ControlPlaneInfrastructureMachineTemplateType TemplateType = "ControlPlane/InfrastructureMachineTemplate"

	// MachineDeploymentBootstrapConfigTemplateType identifies a template for the BootstrapConfig to be used for a MachineDeployment object.
	MachineDeploymentBootstrapConfigTemplateType TemplateType = "MachineDeployment/BootstrapConfigTemplate"

	// MachineDeploymentInfrastructureMachineTemplateType identifies a template for the InfrastructureMachines to be used for a MachineDeployment object.
	MachineDeploymentInfrastructureMachineTemplateType TemplateType = "MachineDeployment/InfrastructureMachineTemplate"
)

// PatchType define the type for patch types enum.
type PatchType string

const (
	// JSONPatchType identifies a https://datatracker.ietf.org/doc/html/rfc6902 json patch.
	JSONPatchType PatchType = "JSONPatch"

	// JSONMergePatchType identifies a https://datatracker.ietf.org/doc/html/rfc7386 json merge patch.
	JSONMergePatchType PatchType = "JSONMergePatch"
)

// GenerateResponse defines the response of a Generate request.
// NOTE: Patches defined in GenerateResponse will be applied in the same order to the original
// GenerateRequest object, thus adding changes on templates across all the subsequent Generate calls.
type GenerateResponse struct {
	// Items contains the list of generated patches.
	Items []GenerateResponsePatch
}

// GenerateResponsePatch defines a Patch targeting a specific GenerateRequestTemplate.
type GenerateResponsePatch struct {
	// TemplateRef identifies the template the patch should apply to.
	TemplateRef TemplateRef

	// Patch contains the patch.
	Patch apiextensionsv1.JSON

	// Patch defines the type of the JSON patch.
	PatchType PatchType
}
