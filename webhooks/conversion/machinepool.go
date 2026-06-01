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
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

// MachinePool is a HubSpokeConverter for the MachinePool API type.
var MachinePool = conversion.NewHubSpokeConverter(&clusterv1.MachinePool{},
	conversion.NewSpokeConverter(&clusterv1beta1.MachinePool{}, ConvertMachinePoolHubToV1Beta1, ConvertMachinePoolV1Beta1ToHub),
)

// ConvertMachinePoolV1Beta1ToHub converts a v1beta1 MachinePool to a hub MachinePool.
func ConvertMachinePoolV1Beta1ToHub(_ context.Context, src *clusterv1beta1.MachinePool, dst *clusterv1.MachinePool) error {
	if err := clusterv1beta1.Convert_v1beta1_MachinePool_To_v1beta2_MachinePool(src, dst, nil); err != nil {
		return err
	}

	if err := convertMachineSpecToContractVersionedObjectReference(&src.Spec.Template.Spec, &dst.Spec.Template.Spec); err != nil {
		return err
	}

	dst.Spec.Template.Spec.MinReadySeconds = src.Spec.MinReadySeconds

	restored := &clusterv1.MachinePool{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Recover intent for bool values converted to *bool.
	initialization := clusterv1.MachinePoolInitializationStatus{}
	restoredBootstrapDataSecretCreated := restored.Status.Initialization.BootstrapDataSecretCreated
	restoredInfrastructureProvisioned := restored.Status.Initialization.InfrastructureProvisioned
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.BootstrapReady, ok, restoredBootstrapDataSecretCreated, &initialization.BootstrapDataSecretCreated)
	clusterv1.Convert_bool_To_Pointer_bool(src.Status.InfrastructureReady, ok, restoredInfrastructureProvisioned, &initialization.InfrastructureProvisioned)
	if !reflect.DeepEqual(initialization, clusterv1.MachinePoolInitializationStatus{}) {
		dst.Status.Initialization = initialization
	}

	return nil
}

// ConvertMachinePoolHubToV1Beta1 converts a hub MachinePool to a v1beta1 MachinePool.
func ConvertMachinePoolHubToV1Beta1(ctx context.Context, src *clusterv1.MachinePool, dst *clusterv1beta1.MachinePool) error {
	if err := clusterv1beta1.Convert_v1beta2_MachinePool_To_v1beta1_MachinePool(src, dst, nil); err != nil {
		return err
	}

	if err := convertMachineSpecToObjectReference(ctx, &src.Spec.Template.Spec, &dst.Spec.Template.Spec, src.Namespace); err != nil {
		return err
	}

	dst.Spec.MinReadySeconds = src.Spec.Template.Spec.MinReadySeconds

	dropEmptyStringsMachineSpec(&dst.Spec.Template.Spec)

	return utilconversion.MarshalDataUnsafeNoCopy(src, dst)
}
