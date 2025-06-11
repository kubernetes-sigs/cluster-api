/*
Copyright 2025 The Kubernetes Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
)

// CanUpdateMachineRequest is the request of the CanUpdateMachine hook.
// +kubebuilder:object:root=true
type CanUpdateMachineRequest struct {
	metav1.TypeMeta `json:",inline"`

	// CommonRequest contains fields common to all request types.
	CommonRequest `json:",inline"`

	// currentMachineObjects contains the current state of the Machine and related objects.
	// +required
	CurrentMachineObjects UpdateMachineObjects `json:"currentMachineObjects"`

	// desiredMachineObjects contains the desired state of the Machine and related objects.
	// +required
	DesiredMachineObjects UpdateMachineObjects `json:"desiredMachineObjects"`
}

// UpdateMachineObjects groups Machine and related objects.
type UpdateMachineObjects struct {
	// machineSpec is the Machine spec.
	// +required
	MachineSpec clusterv1beta1.MachineSpec `json:"machineSpec"`

	// infrastructureMachineSpec contains the infrastructure Machine object spec.
	// +required
	InfrastructureMachineSpec runtime.RawExtension `json:"infrastructureMachineSpec"`

	// bootstrapConfigSpec contains the bootstrap configuration object spec.
	// +optional
	BootstrapConfigSpec *runtime.RawExtension `json:"bootstrapConfigSpec,omitempty"`
}

var _ ResponseObject = &CanUpdateMachineResponse{}

// CanUpdateMachineResponse is the response of the CanUpdateMachine hook.
// +kubebuilder:object:root=true
type CanUpdateMachineResponse struct {
	metav1.TypeMeta `json:",inline"`

	// CommonResponse contains Status and Message fields common to all response types.
	CommonResponse `json:",inline"`

	// machinePatches are patches which, when applied to the current Machine spec from the request,
	// indicate the subset of spec changes this extension can handle in-place.
	// +optional
	MachinePatches []PatchItem `json:"machinePatches,omitempty"`

	// infrastructureMachinePatches are patches which, when applied to the current InfrastructureMachine spec
	// from the request, indicate the subset of spec changes this extension can handle in-place.
	// +optional
	InfrastructureMachinePatches []PatchItem `json:"infrastructureMachinePatches,omitempty"`

	// bootstrapConfigPatches are patches which, when applied to the current BootstrapConfig spec from the request,
	// indicate the subset of spec changes this extension can handle in-place.
	// +optional
	BootstrapConfigPatches []PatchItem `json:"bootstrapConfigPatches,omitempty"`
}

// CanUpdateMachine is the hook that will be called to determine if an extension
// can handle specific machine changes for in-place updates.
func CanUpdateMachine(*CanUpdateMachineRequest, *CanUpdateMachineResponse) {}

// CanUpdateMachineSetRequest is the request of the CanUpdateMachineSet hook.
// +kubebuilder:object:root=true
type CanUpdateMachineSetRequest struct {
	metav1.TypeMeta `json:",inline"`

	// CommonRequest contains fields common to all request types.
	CommonRequest `json:",inline"`

	// currentUpdateMachineSetObjects contains the current state of the MachineSet spec and related objects.
	// +required
	CurrentUpdateMachineSetObjects UpdateMachineSetObjects `json:"currentUpdateMachineSetObjects"`

	// desiredUpdateMachineSetObjects contains the desired state of the MachineSet spec and related objects.
	// +required
	DesiredUpdateMachineSetObjects UpdateMachineSetObjects `json:"desiredUpdateMachineSetObjects"`
}

// UpdateMachineSetObjects groups MachineSet and related template objects.
type UpdateMachineSetObjects struct {
	// machineSetSpec is the MachineSet spec.
	// +required
	MachineSetSpec clusterv1beta1.MachineSetSpec `json:"machineSetSpec"`

	// infrastructureMachineTemplateSpec contains the provider-specific infrastructure Machine template spec object.
	// +required
	InfrastructureMachineTemplateSpec runtime.RawExtension `json:"infrastructureMachineTemplateSpec"`

	// bootstrapConfigTemplateSpec contains the bootstrap configuration template spec object.
	// +optional
	BootstrapConfigTemplateSpec *runtime.RawExtension `json:"bootstrapConfigTemplateSpec,omitempty"`
}

// CanUpdateMachineSetResponse is the response of the CanUpdateMachineSet hook.
// +kubebuilder:object:root=true
type CanUpdateMachineSetResponse struct {
	metav1.TypeMeta `json:",inline"`

	// CommonResponse contains Status and Message fields common to all response types.
	CommonResponse `json:",inline"`

	// machineSetPatches are patches which, when applied to the current MachineSet spec from the request,
	// indicate the subset of spec changes this extension can handle in-place.
	// +optional
	MachineSetPatches []PatchItem `json:"machineSetPatches,omitempty"`

	// infrastructureMachineTemplateSpecPatches are patches which, when applied to the current InfrastructureMachineTemplateSpec
	// from the request, indicate the subset of spec changes this extension can handle in-place.
	// +optional
	InfrastructureMachineTemplateSpecPatches []PatchItem `json:"infrastructureMachineTemplateSpecPatches,omitempty"`

	// bootstrapConfigTemplateSpecPatches are patches which, when applied to the current BootstrapConfigTemplateSpec from the request,
	// indicate the subset of spec changes this extension can handle in-place.
	// +optional
	BootstrapConfigTemplateSpecPatches []PatchItem `json:"bootstrapConfigTemplateSpecPatches,omitempty"`
}

var _ ResponseObject = &CanUpdateMachineSetResponse{}

// CanUpdateMachineSet is the hook that will be called to determine if an extension
// can handle specific MachineSet changes for in-place updates.
func CanUpdateMachineSet(*CanUpdateMachineSetRequest, *CanUpdateMachineSetResponse) {}

// UpdateMachineRequest is the request of the UpdateMachine hook.
// +kubebuilder:object:root=true
type UpdateMachineRequest struct {
	metav1.TypeMeta `json:",inline"`

	// CommonRequest contains fields common to all request types.
	CommonRequest `json:",inline"`

	// desiredUpdateMachineObjects contains the desired state of the Machine and related objects.
	// +required
	DesiredUpdateMachineObjects UpdateMachineObjects `json:"desiredUpdateMachineObjects"`
}

var _ RetryResponseObject = &UpdateMachineResponse{}

// UpdateMachineResponse is the response of the UpdateMachine hook.
// The status of the update operation is determined by the CommonRetryResponse fields:
// - Status=Success + RetryAfterSeconds > 0: update is in progress
// - Status=Success + RetryAfterSeconds = 0: update completed successfully
// - Status=Failure: update failed
// +kubebuilder:object:root=true
type UpdateMachineResponse struct {
	metav1.TypeMeta `json:",inline"`

	// CommonRetryResponse contains Status, Message and RetryAfterSeconds fields.
	CommonRetryResponse `json:",inline"`
}

// UpdateMachine is the hook that will be called to perform in-place updates on a machine.
// This hook should be idempotent and can be called multiple times for the same machine
// until it reports Done or Failed status.
func UpdateMachine(*UpdateMachineRequest, *UpdateMachineResponse) {}

func init() {
	catalogBuilder.RegisterHook(CanUpdateMachine, &runtimecatalog.HookMeta{
		Tags:    []string{"In-Place Update Hooks"},
		Summary: "Cluster API Runtime will call this hook to determine if an extension can handle specific machine changes",
		Description: "Called during update planning to determine if an extension can handle machine changes. " +
			"The request contains current and desired state spec for Machine, InfraMachine and optionally BootstrapConfig. " +
			"Extensions should return per-object patches to be applied on current objects to indicate which changes they can handle in-place.\n" +
			"\n" +
			"Notes:\n" +
			"- This hook is called during the planning phase of updates\n" +
			"- Only spec is provided, status fields are not included\n" +
			"- If no extension can cover the required changes, CAPI will fallback to rolling updates\n",
	})

	catalogBuilder.RegisterHook(CanUpdateMachineSet, &runtimecatalog.HookMeta{
		Tags:    []string{"In-Place Update Hooks"},
		Summary: "Cluster API Runtime will call this hook to determine if an extension can handle specific MachineSet changes",
		Description: "Called during update planning to determine if an extension can handle MachineSet changes. " +
			"The request contains current and desired state spec for MachineSet, InfraMachineTemplate and optionally BootstrapConfigTemplate. " +
			"Extensions should return per-object patches to be applied on current objects to indicate which changes they can handle in-place.",
	})

	catalogBuilder.RegisterHook(UpdateMachine, &runtimecatalog.HookMeta{
		Tags:    []string{"In-Place Update Hooks"},
		Summary: "Cluster API Runtime will call this hook to perform in-place updates on a machine",
		Description: "Cluster API Runtime will call this hook to perform the actual in-place update on a machine. " +
			"The request contains the desired state spec for Machine, InfraMachine and optionally BootstrapConfig. " +
			"The hook will be called repeatedly until it reports Done or Failed status.\n" +
			"\n" +
			"Notes:\n" +
			"- This hook must be idempotent - it can be called multiple times for the same machine\n",
	})
}
