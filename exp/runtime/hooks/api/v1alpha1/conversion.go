/*
Copyright 2021 The Kubernetes Authors.

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
	conversion "k8s.io/apimachinery/pkg/conversion"
	v1alpha3 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha3"
)

func Convert_v1alpha1_DiscoveryHookResponse_To_v1alpha3_DiscoveryHookResponse(in *DiscoveryHookResponse, out *v1alpha3.DiscoveryHookResponse, s conversion.Scope) error {
	return autoConvert_v1alpha1_DiscoveryHookResponse_To_v1alpha3_DiscoveryHookResponse(in, out, s)
}
