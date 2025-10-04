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

package webhooks

import (
	runtimev1alpha1 "sigs.k8s.io/cluster-api/api/runtime/v1alpha1"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
)

func ConvertExtensionConfigV1Alpha1ToHub(src *runtimev1alpha1.ExtensionConfig, dst *runtimev1.ExtensionConfig) error {
	return runtimev1alpha1.Convert_v1alpha1_ExtensionConfig_To_v1beta2_ExtensionConfig(src, dst, nil)
}

func ConvertExtensionConfigHubToV1Alpha1(src *runtimev1.ExtensionConfig, dst *runtimev1alpha1.ExtensionConfig) error {
	if err := runtimev1alpha1.Convert_v1beta2_ExtensionConfig_To_v1alpha1_ExtensionConfig(src, dst, nil); err != nil {
		return err
	}

	dropEmptyStringsExtensionConfig(dst)
	for i, h := range dst.Status.Handlers {
		if h.TimeoutSeconds != nil && *h.TimeoutSeconds == 0 {
			h.TimeoutSeconds = nil
		}
		dst.Status.Handlers[i] = h
	}
	return nil
}

func dropEmptyStringsExtensionConfig(dst *runtimev1alpha1.ExtensionConfig) {
	dropEmptyString(&dst.Spec.ClientConfig.URL)
	if dst.Spec.ClientConfig.Service != nil {
		dropEmptyString(&dst.Spec.ClientConfig.Service.Path)
	}
}
