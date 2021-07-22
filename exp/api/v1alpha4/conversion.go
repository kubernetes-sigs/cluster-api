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

package v1alpha4

import (
	v1 "k8s.io/api/core/v1"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/cluster-api/api/v1alpha4"
)

func Convert_v1_ObjectReference_To_v1alpha4_LocalObjectReference(in *v1.ObjectReference, out *v1alpha4.LocalObjectReference, s apiconversion.Scope) error {
	return v1alpha4.Convert_v1_ObjectReference_To_v1alpha4_LocalObjectReference(in, out, s)
}

func Convert_v1alpha4_LocalObjectReference_To_v1_ObjectReference(in *v1alpha4.LocalObjectReference, out *v1.ObjectReference, s apiconversion.Scope) error {
	return v1alpha4.Convert_v1alpha4_LocalObjectReference_To_v1_ObjectReference(in, out, s)
}
