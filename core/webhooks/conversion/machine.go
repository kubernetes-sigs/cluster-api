/*
Copyright 2026 The Kubernetes Authors.

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

package conversion

import (
	"context"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	conversionutil "sigs.k8s.io/cluster-api/util/conversion"
)

// Machine is a HubSpokeConverter for the Machine API type.
var Machine = conversion.NewHubSpokeConverter(&clusterv1.Machine{},
	conversion.NewSpokeConverter(&clusterv1beta1.Machine{}, ConvertMachineHubToV1Beta1, ConvertMachineV1Beta1ToHub),
)

// ConvertMachineV1Beta1ToHub converts a v1beta1 Machine to a hub Machine.
func ConvertMachineV1Beta1ToHub(_ context.Context, src *clusterv1beta1.Machine, dst *clusterv1.Machine) error {
	if err := clusterv1beta1.Convert_v1beta1_Machine_To_v1beta2_Machine(src, dst, nil); err != nil {
		return err
	}

	if err := convertMachineSpecToContractVersionedObjectReference(&src.Spec, &dst.Spec); err != nil {
		return err
	}

	restored := &clusterv1.Machine{}
	ok, err := conversionutil.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Recover intent for bool values converted to *bool.
	initialization := clusterv1.MachineInitializationStatus{}
	restoredBootstrapDataSecretCreated := restored.Status.Initialization.BootstrapDataSecretCreated
	restoredInfrastructureProvisioned := restored.Status.Initialization.InfrastructureProvisioned
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.BootstrapReady, ok, restoredBootstrapDataSecretCreated, &initialization.BootstrapDataSecretCreated)
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.InfrastructureReady, ok, restoredInfrastructureProvisioned, &initialization.InfrastructureProvisioned)
	if !reflect.DeepEqual(initialization, clusterv1.MachineInitializationStatus{}) {
		dst.Status.Initialization = initialization
	}

	// Recover other values.
	if ok {
		// Note: Using the conversion annotation here because backporting the field into MachineSpec
		// would mean that it also shows up as a duplicate MinReadySeconds field in MachineSet & MachineDeployment.
		dst.Spec.MinReadySeconds = restored.Spec.MinReadySeconds
		// Restore the phase, this also means that any client using v1beta1 during a round-trip
		// won't be able to write the Phase field. But that's okay as the only client writing the Phase
		// field should be the Machine controller.
		dst.Status.Phase = restored.Status.Phase
		dst.Status.FailureDomain = restored.Status.FailureDomain
	}

	return nil
}

// ConvertMachineHubToV1Beta1 converts a hub Machine to a v1beta1 Machine.
func ConvertMachineHubToV1Beta1(ctx context.Context, src *clusterv1.Machine, dst *clusterv1beta1.Machine) error {
	if err := clusterv1beta1.Convert_v1beta2_Machine_To_v1beta1_Machine(src, dst, nil); err != nil {
		return err
	}

	if err := convertMachineSpecToObjectReference(ctx, &src.Spec, &dst.Spec, src.Namespace); err != nil {
		return err
	}

	dropEmptyStringsMachineSpec(&dst.Spec)

	return conversionutil.MarshalDataUnsafeNoCopy(src, dst)
}
