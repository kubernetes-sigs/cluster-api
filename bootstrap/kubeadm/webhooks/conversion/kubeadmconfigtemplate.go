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

	bootstrapv1beta1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	conversionutil "sigs.k8s.io/cluster-api/util/conversion"
)

// KubeadmConfigTemplate is a HubSpokeConverter for the KubeadmConfigTemplate API type.
var KubeadmConfigTemplate = conversion.NewHubSpokeConverter(&bootstrapv1.KubeadmConfigTemplate{},
	conversion.NewSpokeConverter(&bootstrapv1beta1.KubeadmConfigTemplate{}, ConvertKubeadmConfigTemplateHubToV1Beta1, ConvertKubeadmConfigTemplateV1Beta1ToHub),
)

// ConvertKubeadmConfigTemplateV1Beta1ToHub converts a v1beta1 KubeadmConfigTemplate to a hub KubeadmConfigTemplate.
func ConvertKubeadmConfigTemplateV1Beta1ToHub(ctx context.Context, src *bootstrapv1beta1.KubeadmConfigTemplate, dst *bootstrapv1.KubeadmConfigTemplate) error {
	if err := bootstrapv1beta1.Convert_v1beta1_KubeadmConfigTemplate_To_v1beta2_KubeadmConfigTemplate(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &bootstrapv1.KubeadmConfigTemplate{}
	ok, err := conversionutil.UnmarshalData(src, restored)
	if err != nil {
		return err
	}
	// Recover intent for bool values converted to *bool.
	if err := RestoreBoolIntentKubeadmConfigSpec(&src.Spec.Template.Spec, &dst.Spec.Template.Spec, ok, &restored.Spec.Template.Spec); err != nil {
		return err
	}

	// Recover other values
	if ok {
		RestoreKubeadmConfigSpec(&restored.Spec.Template.Spec, &dst.Spec.Template.Spec)
	}

	// Override restored data with timeouts values already existing in v1beta1 but in other structs.
	ConvertKubeadmConfigSpecV1Beta1ToHub(ctx, &src.Spec.Template.Spec, &dst.Spec.Template.Spec)
	return nil
}

// ConvertKubeadmConfigTemplateHubToV1Beta1 converts a hub KubeadmConfigTemplate to a v1beta1 KubeadmConfigTemplate.
func ConvertKubeadmConfigTemplateHubToV1Beta1(ctx context.Context, src *bootstrapv1.KubeadmConfigTemplate, dst *bootstrapv1beta1.KubeadmConfigTemplate) error {
	if err := bootstrapv1beta1.Convert_v1beta2_KubeadmConfigTemplate_To_v1beta1_KubeadmConfigTemplate(src, dst, nil); err != nil {
		return err
	}

	// Convert timeouts moved from one struct to another.
	ConvertKubeadmConfigSpecHubToV1Beta1(ctx, &src.Spec.Template.Spec, &dst.Spec.Template.Spec)

	dropEmptyStringsKubeadmConfigSpec(&dst.Spec.Template.Spec)

	// Preserve Hub data on down-conversion except for metadata.
	return conversionutil.MarshalDataUnsafeNoCopy(src, dst)
}
