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

	ipamv1alpha1 "sigs.k8s.io/cluster-api/api/ipam/v1alpha1"
	ipamv1beta1 "sigs.k8s.io/cluster-api/api/ipam/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
)

// IPAddress is a HubSpokeConverter for the IPAddress API type.
var IPAddress = conversion.NewHubSpokeConverter(&ipamv1.IPAddress{},
	conversion.NewSpokeConverter(&ipamv1alpha1.IPAddress{}, ConvertIPAddressHubToV1Alpha1, ConvertIPAddressV1Alpha1ToHub),
	conversion.NewSpokeConverter(&ipamv1beta1.IPAddress{}, ConvertIPAddressHubToV1Beta1, ConvertIPAddressV1Beta1ToHub),
)

// ConvertIPAddressV1Alpha1ToHub converts a v1alpha1 IPAddress to a hub IPAddress.
func ConvertIPAddressV1Alpha1ToHub(_ context.Context, src *ipamv1alpha1.IPAddress, dst *ipamv1.IPAddress) error {
	return ipamv1alpha1.Convert_v1alpha1_IPAddress_To_v1beta2_IPAddress(src, dst, nil)
}

// ConvertIPAddressHubToV1Alpha1 converts a hub IPAddress to a v1alpha1 IPAddress.
func ConvertIPAddressHubToV1Alpha1(_ context.Context, src *ipamv1.IPAddress, dst *ipamv1alpha1.IPAddress) error {
	return ipamv1alpha1.Convert_v1beta2_IPAddress_To_v1alpha1_IPAddress(src, dst, nil)
}

// ConvertIPAddressV1Beta1ToHub converts a v1beta1 IPAddress to a hub IPAddress.
func ConvertIPAddressV1Beta1ToHub(_ context.Context, src *ipamv1beta1.IPAddress, dst *ipamv1.IPAddress) error {
	return ipamv1beta1.Convert_v1beta1_IPAddress_To_v1beta2_IPAddress(src, dst, nil)
}

// ConvertIPAddressHubToV1Beta1 converts a hub IPAddress to a v1beta1 IPAddress.
func ConvertIPAddressHubToV1Beta1(_ context.Context, src *ipamv1.IPAddress, dst *ipamv1beta1.IPAddress) error {
	return ipamv1beta1.Convert_v1beta2_IPAddress_To_v1beta1_IPAddress(src, dst, nil)
}
