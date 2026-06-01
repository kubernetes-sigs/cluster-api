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
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

// MachineHealthCheck is a HubSpokeConverter for the MachineHealthCheck API type.
var MachineHealthCheck = conversion.NewHubSpokeConverter(&clusterv1.MachineHealthCheck{},
	conversion.NewSpokeConverter(&clusterv1beta1.MachineHealthCheck{}, ConvertMachineHealthCheckHubToV1Beta1, ConvertMachineHealthCheckV1Beta1ToHub),
)

// ConvertMachineHealthCheckV1Beta1ToHub converts a v1beta1 MachineHealthCheck to a hub MachineHealthCheck.
func ConvertMachineHealthCheckV1Beta1ToHub(_ context.Context, src *clusterv1beta1.MachineHealthCheck, dst *clusterv1.MachineHealthCheck) error {
	if err := clusterv1beta1.Convert_v1beta1_MachineHealthCheck_To_v1beta2_MachineHealthCheck(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &clusterv1.MachineHealthCheck{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	clusterv1.Convert_int32_To_Pointer_int32(src.Status.ExpectedMachines, ok, restored.Status.ExpectedMachines, &dst.Status.ExpectedMachines)
	clusterv1.Convert_int32_To_Pointer_int32(src.Status.CurrentHealthy, ok, restored.Status.CurrentHealthy, &dst.Status.CurrentHealthy)
	clusterv1.Convert_int32_To_Pointer_int32(src.Status.RemediationsAllowed, ok, restored.Status.RemediationsAllowed, &dst.Status.RemediationsAllowed)

	return nil
}

// ConvertMachineHealthCheckHubToV1Beta1 converts a hub MachineHealthCheck to a v1beta1 MachineHealthCheck.
func ConvertMachineHealthCheckHubToV1Beta1(_ context.Context, src *clusterv1.MachineHealthCheck, dst *clusterv1beta1.MachineHealthCheck) error {
	if err := clusterv1beta1.Convert_v1beta2_MachineHealthCheck_To_v1beta1_MachineHealthCheck(src, dst, nil); err != nil {
		return err
	}

	if dst.Spec.RemediationTemplate != nil {
		dst.Spec.RemediationTemplate.Namespace = src.Namespace
	}

	return utilconversion.MarshalDataUnsafeNoCopy(src, dst)
}
