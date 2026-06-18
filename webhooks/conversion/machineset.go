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

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// MachineSet is a HubSpokeConverter for the MachineSet API type.
var MachineSet = conversion.NewHubSpokeConverter(&clusterv1.MachineSet{},
	conversion.NewSpokeConverter(&clusterv1beta1.MachineSet{}, ConvertMachineSetHubToV1Beta1, ConvertMachineSetV1Beta1ToHub),
)

// ConvertMachineSetV1Beta1ToHub converts a v1beta1 MachineSet to a hub MachineSet.
func ConvertMachineSetV1Beta1ToHub(_ context.Context, src *clusterv1beta1.MachineSet, dst *clusterv1.MachineSet) error {
	if err := clusterv1beta1.Convert_v1beta1_MachineSet_To_v1beta2_MachineSet(src, dst, nil); err != nil {
		return err
	}

	if err := convertMachineSpecToContractVersionedObjectReference(&src.Spec.Template.Spec, &dst.Spec.Template.Spec); err != nil {
		return err
	}

	if src.Spec.MinReadySeconds == 0 {
		dst.Spec.Template.Spec.MinReadySeconds = nil
	} else {
		dst.Spec.Template.Spec.MinReadySeconds = &src.Spec.MinReadySeconds
	}

	return nil
}

// ConvertMachineSetHubToV1Beta1 converts a hub MachineSet to a v1beta1 MachineSet.
func ConvertMachineSetHubToV1Beta1(ctx context.Context, src *clusterv1.MachineSet, dst *clusterv1beta1.MachineSet) error {
	if err := clusterv1beta1.Convert_v1beta2_MachineSet_To_v1beta1_MachineSet(src, dst, nil); err != nil {
		return err
	}

	if err := convertMachineSpecToObjectReference(ctx, &src.Spec.Template.Spec, &dst.Spec.Template.Spec, src.Namespace); err != nil {
		return err
	}

	dst.Spec.MinReadySeconds = ptr.Deref(src.Spec.Template.Spec.MinReadySeconds, 0)

	dropEmptyStringsMachineSpec(&dst.Spec.Template.Spec)

	return nil
}
