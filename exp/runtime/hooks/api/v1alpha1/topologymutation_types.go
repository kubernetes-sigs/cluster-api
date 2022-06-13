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

package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	runtimecatalog "sigs.k8s.io/cluster-api/internal/runtime/catalog"
)

// GeneratePatchesRequest is the request of the GeneratePatches hook.
// +kubebuilder:object:root=true
type GeneratePatchesRequest struct {
	metav1.TypeMeta `json:",inline"`

	// Variables are global variables for all templates.
	Variables []Variable `json:"variables"`

	// Items is the list of templates to generate patches for.
	Items []GeneratePatchesRequestItem `json:"items"`
}

// GeneratePatchesRequestItem represents a template to generate patches for.
type GeneratePatchesRequestItem struct {
	// UID is an identifier for this template. It allows us to correlate the template in the request
	// with the corresponding generates patches in the response.
	UID types.UID `json:"uid"`

	// HolderReference is a reference to the object where the template is used.
	HolderReference HolderReference `json:"holderReference"`

	// Object contains the template as a raw object.
	Object runtime.RawExtension `json:"object"`

	// Variables are variables specific for the current template.
	// For example some builtin variables like MachineDeployment replicas and version are context-sensitive
	// and thus are only added to templates for MachineDeployments and with values which correspond to the
	// current MachineDeployment.
	Variables []Variable `json:"variables"`
}

var _ ResponseObject = &GeneratePatchesResponse{}

// GeneratePatchesResponse is the response of the GeneratePatches hook.
// NOTE: The patches in GeneratePatchesResponse will be applied in the order in which they are defined to the
// templates of the request. Thus applying changes consecutively when iterating through internal and external patches.
// +kubebuilder:object:root=true
type GeneratePatchesResponse struct {
	metav1.TypeMeta `json:",inline"`

	// CommonResponse contains Status and Message fields common to all response types.
	CommonResponse `json:",inline"`

	// Items is the list of generated patches.
	Items []GeneratePatchesResponseItem `json:"items"`
}

// GeneratePatchesResponseItem is a generated patch.
type GeneratePatchesResponseItem struct {
	// UID identifies the corresponding template in the request on which
	// the patch should be applied.
	UID types.UID `json:"uid"`

	// PatchType defines the type of the patch.
	// One of: "JSONPatch" or "JSONMergePatch".
	PatchType PatchType `json:"patchType"`

	// Patch contains the patch which should be applied to the template.
	// It must be of the corresponding PatchType.
	Patch []byte `json:"patch"`
}

// PatchType defines the supported patch types.
// +enum
type PatchType string

const (
	// JSONPatchType identifies a https://datatracker.ietf.org/doc/html/rfc6902 JSON patch.
	JSONPatchType PatchType = "JSONPatch"

	// JSONMergePatchType identifies a https://datatracker.ietf.org/doc/html/rfc7386 JSON merge patch.
	JSONMergePatchType PatchType = "JSONMergePatch"
)

// GeneratePatches generates patches during topology reconciliation for the entire Cluster topology.
func GeneratePatches(*GeneratePatchesRequest, *GeneratePatchesResponse) {}

// ValidateTopologyRequest is the request of the ValidateTopology hook.
// +kubebuilder:object:root=true
type ValidateTopologyRequest struct {
	metav1.TypeMeta `json:",inline"`

	// Variables are global variables for all templates.
	Variables []Variable `json:"variables"`

	// Items is the list of templates to validate.
	Items []*ValidateTopologyRequestItem `json:"items"`
}

// ValidateTopologyRequestItem represents a template to validate.
type ValidateTopologyRequestItem struct {
	// HolderReference is a reference to the object where the template is used.
	HolderReference HolderReference `json:"holderReference"`

	// Object contains the template as a raw object.
	Object runtime.RawExtension `json:"object"`

	// Variables are variables specific for the current template.
	// For example some builtin variables like MachineDeployment replicas and version are context-sensitive
	// and thus are only added to templates for MachineDeployments and with values which correspond to the
	// current MachineDeployment.
	Variables []Variable `json:"variables"`
}

var _ ResponseObject = &ValidateTopologyResponse{}

// ValidateTopologyResponse is the response of the ValidateTopology hook.
// +kubebuilder:object:root=true
type ValidateTopologyResponse struct {
	metav1.TypeMeta `json:",inline"`

	// CommonResponse contains Status and Message fields common to all response types.
	CommonResponse `json:",inline"`
}

// Variable represents a variable value.
type Variable struct {
	// Name of the variable.
	Name string `json:"name"`

	// Value of the variable.
	Value apiextensionsv1.JSON `json:"value"`
}

// HolderReference represents a reference to an object which holds a template.
type HolderReference struct {
	// API version of the referent.
	APIVersion string `json:"apiVersion"`

	// Kind of the referent.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	Kind string `json:"kind"`

	// Namespace of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/
	Namespace string `json:"namespace"`

	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	Name string `json:"name"`

	// FieldPath is the path to the field of the object which references the template.
	FieldPath string `json:"fieldPath"`
}

// ValidateTopology validates the Cluster topology after all patches have been applied.
func ValidateTopology(*ValidateTopologyRequest, *ValidateTopologyResponse) {}

func init() {
	catalogBuilder.RegisterHook(GeneratePatches, &runtimecatalog.HookMeta{
		Tags:        []string{"Topology Mutation Hook"},
		Summary:     "GeneratePatches generates patches during topology reconciliation for the entire Cluster topology.",
		Description: "A GeneratePatches call generates patches for the entire Cluster topology. Accordingly the request contains all templates, the global variables and the template-specific variables. The response contains generated patches.",
	})

	catalogBuilder.RegisterHook(ValidateTopology, &runtimecatalog.HookMeta{
		Tags:        []string{"Topology Mutation Hook"},
		Summary:     "ValidateTopology validates the Cluster topology after all patches have been applied.",
		Description: "A ValidateTopology call validates the Cluster topology after all patches have been applied. The request contains all templates of the Cluster topology, the global variables and the template-specific variables. The response contains the result of the validation.",
	})
}
