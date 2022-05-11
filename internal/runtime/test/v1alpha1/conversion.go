/*
Copyright 2022 The Kubernetes Authors.

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

	clusterv1alpha4 "sigs.k8s.io/cluster-api/api/v1alpha4"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	v1alpha2 "sigs.k8s.io/cluster-api/internal/runtime/test/v1alpha2"
)

func Convert_v1alpha1_FakeResponse_To_v1alpha2_FakeResponse(in *FakeResponse, out *v1alpha2.FakeResponse, s conversion.Scope) error {
	return autoConvert_v1alpha1_FakeResponse_To_v1alpha2_FakeResponse(in, out, s)
}

func Convert_v1alpha4_Cluster_To_v1beta1_Cluster(in *clusterv1alpha4.Cluster, out *clusterv1.Cluster, s conversion.Scope) error {
	return clusterv1alpha4.Convert_v1alpha4_Cluster_To_v1beta1_Cluster(in, out, s)
}

func Convert_v1beta1_Cluster_To_v1alpha4_Cluster(in *clusterv1.Cluster, out *clusterv1alpha4.Cluster, s conversion.Scope) error {
	return clusterv1alpha4.Convert_v1beta1_Cluster_To_v1alpha4_Cluster(in, out, s)
}
