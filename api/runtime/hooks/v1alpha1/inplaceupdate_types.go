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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
)

// CanUpdateMachineRequest is the request of the CanUpdateMachine hook.
// This hook is called to determine if an extension can handle specific changes.
// +kubebuilder:object:root=true
type CanUpdateMachineRequest struct {
	metav1.TypeMeta `json:",inline"`

	// CommonRequest contains fields common to all request types.
	CommonRequest `json:",inline"`

	// changes is a list of field paths that need to be updated on the machine.
	// Examples: ["spec.version", "spec.infrastructureRef.spec.memoryMiB"]
	// +required
	Changes []string `json:"changes"`
}

var _ ResponseObject = &CanUpdateMachineResponse{}

// CanUpdateMachineResponse is the response of the CanUpdateMachine hook.
// +kubebuilder:object:root=true
type CanUpdateMachineResponse struct {
	metav1.TypeMeta `json:",inline"`

	// CommonResponse contains Status and Message fields common to all response types.
	CommonResponse `json:",inline"`

	// acceptedChanges is the subset of requested changes that this extension can handle.
	// If empty, the extension cannot handle any of the requested changes.
	// +optional
	AcceptedChanges []string `json:"acceptedChanges,omitempty"`
}

// CanUpdateMachine is the hook that will be called to determine if an extension
// can handle specific machine changes for in-place updates.
func CanUpdateMachine(*CanUpdateMachineRequest, *CanUpdateMachineResponse) {}

// UpdateMachineRequest is the request of the UpdateMachine hook.
// This hook is called to perform the actual in-place update on a machine.
// +kubebuilder:object:root=true
type UpdateMachineRequest struct {
	metav1.TypeMeta `json:",inline"`

	// CommonRequest contains fields common to all request types.
	CommonRequest `json:",inline"`

	// machineRef is a reference to the machine object the in-place update hook corresponds to.
	// Updaters should fetch the latest machine state using this reference.
	// +required
	MachineRef corev1.ObjectReference `json:"machineRef"`
}

var _ RetryResponseObject = &UpdateMachineResponse{}

// UpdateMachineStatus represents the status of an in-place update operation.
// +kubebuilder:validation:Enum=InProgress;Done;Failed
type UpdateMachineStatus string

const (
	// UpdateMachineStatusInProgress indicates the update is still in progress.
	UpdateMachineStatusInProgress UpdateMachineStatus = "InProgress"
	// UpdateMachineStatusDone indicates the update has completed successfully.
	UpdateMachineStatusDone UpdateMachineStatus = "Done"
	// UpdateMachineStatusFailed indicates the update has failed.
	UpdateMachineStatusFailed UpdateMachineStatus = "Failed"
)

// UpdateMachineResponse is the response of the UpdateMachine hook.
// +kubebuilder:object:root=true
type UpdateMachineResponse struct {
	metav1.TypeMeta `json:",inline"`

	// CommonRetryResponse contains Status, Message and RetryAfterSeconds fields.
	CommonRetryResponse `json:",inline"`

	// updateStatus indicates the current status of the update operation.
	// +required
	UpdateStatus UpdateMachineStatus `json:"updateStatus"`
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
			"The extension should respond with the subset of changes it can handle for in-place updates.\n" +
			"\n" +
			"Notes:\n" +
			"- This hook is called during the planning phase of updates\n" +
			"- The request contains a list of required changes (field paths)\n" +
			"- Extensions should return only the changes they can confidently handle\n" +
			"- If no extension can cover all changes, CAPI will fallback to rolling updates\n",
	})

	catalogBuilder.RegisterHook(UpdateMachine, &runtimecatalog.HookMeta{
		Tags:    []string{"In-Place Update Hooks"},
		Summary: "Cluster API Runtime will call this hook to perform in-place updates on a machine",
		Description: "Cluster API Runtime will call this hook to perform the actual in-place update on a machine. " +
			"The hook will be called repeatedly until it reports Done or Failed status.\n" +
			"\n" +
			"Notes:\n" +
			"- This hook must be idempotent - it can be called multiple times for the same machine\n" +
			"- Extensions should fetch the latest machine state using the provided reference\n" +
			"- The hook should return InProgress status while the update is ongoing\n" +
			"- Use RetryAfterSeconds to control polling frequency\n" +
			"- The hook should perform updates based on the current machine spec\n",
	})
}
