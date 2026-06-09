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

	controlplanev1beta1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	bootstrapconversion "sigs.k8s.io/cluster-api/bootstrap/kubeadm/webhooks/conversion"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

// KubeadmControlPlaneTemplate is a HubSpokeConverter for the KubeadmControlPlaneTemplate API type.
var KubeadmControlPlaneTemplate = conversion.NewHubSpokeConverter(&controlplanev1.KubeadmControlPlaneTemplate{},
	conversion.NewSpokeConverter(&controlplanev1beta1.KubeadmControlPlaneTemplate{}, ConvertKubeadmControlPlaneTemplateHubToV1Beta1, ConvertKubeadmControlPlaneTemplateV1Beta1ToHub),
)

// ConvertKubeadmControlPlaneTemplateV1Beta1ToHub converts a v1beta1 KubeadmControlPlaneTemplate to a hub KubeadmControlPlaneTemplate.
func ConvertKubeadmControlPlaneTemplateV1Beta1ToHub(ctx context.Context, src *controlplanev1beta1.KubeadmControlPlaneTemplate, dst *controlplanev1.KubeadmControlPlaneTemplate) error {
	if err := controlplanev1beta1.Convert_v1beta1_KubeadmControlPlaneTemplate_To_v1beta2_KubeadmControlPlaneTemplate(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &controlplanev1.KubeadmControlPlaneTemplate{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return err
	}

	// Recover intent for bool values converted to *bool.
	if err := bootstrapconversion.RestoreBoolIntentKubeadmConfigSpec(&src.Spec.Template.Spec.KubeadmConfigSpec, &dst.Spec.Template.Spec.KubeadmConfigSpec, ok, &restored.Spec.Template.Spec.KubeadmConfigSpec); err != nil {
		return err
	}

	// Recover other values
	if ok {
		bootstrapconversion.RestoreKubeadmConfigSpec(&restored.Spec.Template.Spec.KubeadmConfigSpec, &dst.Spec.Template.Spec.KubeadmConfigSpec)
	}

	if src.Spec.Template.Spec.RemediationStrategy != nil {
		clusterv1.Convert_Duration_To_Pointer_int32(src.Spec.Template.Spec.RemediationStrategy.RetryPeriod, ok, restored.Spec.Template.Spec.Remediation.RetryPeriodSeconds, &dst.Spec.Template.Spec.Remediation.RetryPeriodSeconds)
	}

	// Override restored data with timeouts values already existing in v1beta1 but in other structs.
	bootstrapconversion.ConvertKubeadmConfigSpecV1Beta1ToHub(ctx, &src.Spec.Template.Spec.KubeadmConfigSpec, &dst.Spec.Template.Spec.KubeadmConfigSpec)
	return nil
}

// ConvertKubeadmControlPlaneTemplateHubToV1Beta1 converts a hub KubeadmControlPlaneTemplate to a v1beta1 KubeadmControlPlaneTemplate.
func ConvertKubeadmControlPlaneTemplateHubToV1Beta1(ctx context.Context, src *controlplanev1.KubeadmControlPlaneTemplate, dst *controlplanev1beta1.KubeadmControlPlaneTemplate) error {
	if err := controlplanev1beta1.Convert_v1beta2_KubeadmControlPlaneTemplate_To_v1beta1_KubeadmControlPlaneTemplate(src, dst, nil); err != nil {
		return err
	}

	// Convert timeouts moved from one struct to another.
	bootstrapconversion.ConvertKubeadmConfigSpecHubToV1Beta1(ctx, &src.Spec.Template.Spec.KubeadmConfigSpec, &dst.Spec.Template.Spec.KubeadmConfigSpec)

	dropEmptyStringsKubeadmConfigSpec(&dst.Spec.Template.Spec.KubeadmConfigSpec)

	// Preserve Hub data on down-conversion except for metadata.
	return utilconversion.MarshalDataUnsafeNoCopy(src, dst)
}
