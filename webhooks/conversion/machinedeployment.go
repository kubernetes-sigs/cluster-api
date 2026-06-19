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

	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	conversionutil "sigs.k8s.io/cluster-api/util/conversion"
)

// MachineDeployment is a HubSpokeConverter for the MachineDeployment API type.
var MachineDeployment = conversion.NewHubSpokeConverter(&clusterv1.MachineDeployment{},
	conversion.NewSpokeConverter(&clusterv1beta1.MachineDeployment{}, ConvertMachineDeploymentHubToV1Beta1, ConvertMachineDeploymentV1Beta1ToHub),
)

// ConvertMachineDeploymentV1Beta1ToHub converts a v1beta1 MachineDeployment to a hub MachineDeployment.
func ConvertMachineDeploymentV1Beta1ToHub(_ context.Context, src *clusterv1beta1.MachineDeployment, dst *clusterv1.MachineDeployment) error {
	if err := clusterv1beta1.Convert_v1beta1_MachineDeployment_To_v1beta2_MachineDeployment(src, dst, nil); err != nil {
		return err
	}

	if err := convertMachineSpecToContractVersionedObjectReference(&src.Spec.Template.Spec, &dst.Spec.Template.Spec); err != nil {
		return err
	}

	dst.Spec.Template.Spec.MinReadySeconds = src.Spec.MinReadySeconds

	restored := &clusterv1.MachineDeployment{}
	ok, err := conversionutil.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Recover intent for bool values converted to *bool.
	clusterv1.Convert_bool_To_Pointer_bool(src.Spec.Paused, ok, restored.Spec.Paused, &dst.Spec.Paused)

	return nil
}

// ConvertMachineDeploymentHubToV1Beta1 converts a hub MachineDeployment to a v1beta1 MachineDeployment.
func ConvertMachineDeploymentHubToV1Beta1(ctx context.Context, src *clusterv1.MachineDeployment, dst *clusterv1beta1.MachineDeployment) error {
	if err := clusterv1beta1.Convert_v1beta2_MachineDeployment_To_v1beta1_MachineDeployment(src, dst, nil); err != nil {
		return err
	}

	if err := convertMachineSpecToObjectReference(ctx, &src.Spec.Template.Spec, &dst.Spec.Template.Spec, src.Namespace); err != nil {
		return err
	}

	dst.Spec.MinReadySeconds = src.Spec.Template.Spec.MinReadySeconds

	dropEmptyStringsMachineSpec(&dst.Spec.Template.Spec)

	return conversionutil.MarshalDataUnsafeNoCopy(src, dst)
}
