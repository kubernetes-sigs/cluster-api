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
	apimachineryconversion "k8s.io/apimachinery/pkg/conversion"
	"k8s.io/utils/ptr"

	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
)

func Convert_v1alpha1_IPAddressSpec_To_v1beta2_IPAddressSpec(in *IPAddressSpec, out *ipamv1.IPAddressSpec, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1alpha1_IPAddressSpec_To_v1beta2_IPAddressSpec(in, out, s); err != nil {
		return err
	}
	out.Prefix = ptr.To(int32(in.Prefix))
	return nil
}

func Convert_v1beta2_IPAddressSpec_To_v1alpha1_IPAddressSpec(in *ipamv1.IPAddressSpec, out *IPAddressSpec, s apimachineryconversion.Scope) error {
	if err := autoConvert_v1beta2_IPAddressSpec_To_v1alpha1_IPAddressSpec(in, out, s); err != nil {
		return err
	}
	out.Prefix = int(ptr.Deref(in.Prefix, 0))
	return nil
}

func Convert_v1beta2_IPAddressClaimSpec_To_v1alpha1_IPAddressClaimSpec(from *ipamv1.IPAddressClaimSpec, to *IPAddressClaimSpec, scope apimachineryconversion.Scope) error {
	return autoConvert_v1beta2_IPAddressClaimSpec_To_v1alpha1_IPAddressClaimSpec(from, to, scope)
}

func Convert_v1beta2_IPAddressClaimStatus_To_v1alpha1_IPAddressClaimStatus(from *ipamv1.IPAddressClaimStatus, to *IPAddressClaimStatus, scope apimachineryconversion.Scope) error {
	return autoConvert_v1beta2_IPAddressClaimStatus_To_v1alpha1_IPAddressClaimStatus(from, to, scope)
}
