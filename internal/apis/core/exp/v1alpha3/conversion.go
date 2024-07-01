/*
Copyright 2020 The Kubernetes Authors.

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

package v1alpha3

import (
	apimachineryconversion "k8s.io/apimachinery/pkg/conversion"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterv1alpha3 "sigs.k8s.io/cluster-api/internal/apis/core/v1alpha3"
)

func Convert_v1alpha3_MachineTemplateSpec_To_v1beta1_MachineTemplateSpec(in *clusterv1alpha3.MachineTemplateSpec, out *clusterv1.MachineTemplateSpec, s apimachineryconversion.Scope) error {
	return clusterv1alpha3.Convert_v1alpha3_MachineTemplateSpec_To_v1beta1_MachineTemplateSpec(in, out, s)
}

func Convert_v1beta1_MachineTemplateSpec_To_v1alpha3_MachineTemplateSpec(in *clusterv1.MachineTemplateSpec, out *clusterv1alpha3.MachineTemplateSpec, s apimachineryconversion.Scope) error {
	return clusterv1alpha3.Convert_v1beta1_MachineTemplateSpec_To_v1alpha3_MachineTemplateSpec(in, out, s)
}
