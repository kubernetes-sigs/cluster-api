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
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
)

func (src *IPAddress) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*ipamv1.IPAddress)

	return Convert_v1alpha1_IPAddress_To_v1beta1_IPAddress(src, dst, nil)
}

func (dst *IPAddress) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*ipamv1.IPAddress)

	return Convert_v1beta1_IPAddress_To_v1alpha1_IPAddress(src, dst, nil)
}

func (src *IPAddressList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*ipamv1.IPAddressList)

	return Convert_v1alpha1_IPAddressList_To_v1beta1_IPAddressList(src, dst, nil)
}

func (dst *IPAddressList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*ipamv1.IPAddressList)

	return Convert_v1beta1_IPAddressList_To_v1alpha1_IPAddressList(src, dst, nil)
}

func (src *IPAddressClaim) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*ipamv1.IPAddressClaim)

	if err := Convert_v1alpha1_IPAddressClaim_To_v1beta1_IPAddressClaim(src, dst, nil); err != nil {
		return err
	}

	if src.ObjectMeta.Labels != nil {
		dst.Spec.ClusterName = src.ObjectMeta.Labels[clusterv1.ClusterNameLabel]
		if dst.ObjectMeta.Annotations != nil {
			if clusterNameLabelWasSet, ok := dst.ObjectMeta.Annotations["conversion.cluster.x-k8s.io/cluster-name-label-set"]; ok {
				if clusterNameLabelWasSet == "false" {
					delete(dst.ObjectMeta.Labels, clusterv1.ClusterNameLabel)
				}
				delete(dst.ObjectMeta.Annotations, "conversion.cluster.x-k8s.io/cluster-name-label-set")
			}
		}
	}

	return nil
}

func (dst *IPAddressClaim) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*ipamv1.IPAddressClaim)

	if err := Convert_v1beta1_IPAddressClaim_To_v1alpha1_IPAddressClaim(src, dst, nil); err != nil {
		return err
	}

	if src.Spec.ClusterName != "" {
		if dst.ObjectMeta.Labels == nil {
			dst.ObjectMeta.Labels = map[string]string{}
		}
		if _, ok := dst.ObjectMeta.Labels[clusterv1.ClusterNameLabel]; !ok {
			if dst.ObjectMeta.Annotations == nil {
				dst.ObjectMeta.Annotations = map[string]string{}
			}
			dst.ObjectMeta.Annotations["conversion.cluster.x-k8s.io/cluster-name-label-set"] = "false"
		}
		dst.ObjectMeta.Labels[clusterv1.ClusterNameLabel] = src.Spec.ClusterName
	}

	return nil
}

func (src *IPAddressClaimList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*ipamv1.IPAddressClaimList)

	return Convert_v1alpha1_IPAddressClaimList_To_v1beta1_IPAddressClaimList(src, dst, nil)
}

func (dst *IPAddressClaimList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*ipamv1.IPAddressClaimList)

	return Convert_v1beta1_IPAddressClaimList_To_v1alpha1_IPAddressClaimList(src, dst, nil)
}

func Convert_v1beta1_IPAddressClaimSpec_To_v1alpha1_IPAddressClaimSpec(from *ipamv1.IPAddressClaimSpec, to *IPAddressClaimSpec, scope apiconversion.Scope) error {
	return autoConvert_v1beta1_IPAddressClaimSpec_To_v1alpha1_IPAddressClaimSpec(from, to, scope)
}
