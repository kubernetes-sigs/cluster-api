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
)

func (*Cluster) Hub()                {}
func (*ClusterList) Hub()            {}
func (*Machine) Hub()                {}
func (*MachineList) Hub()            {}
func (*MachineSet) Hub()             {}
func (*MachineSetList) Hub()         {}
func (*MachineDeployment) Hub()      {}
func (*MachineDeploymentList) Hub()  {}
func (*MachineHealthCheck) Hub()     {}
func (*MachineHealthCheckList) Hub() {}

func Convert_v1_ObjectReference_To_v1alpha4_LocalObjectReference(in *v1.ObjectReference, out *LocalObjectReference, s apiconversion.Scope) error {
	out.APIVersion = in.APIVersion
	out.Kind = in.Kind
	out.Name = in.Name
	return nil
}

func Convert_v1alpha4_LocalObjectReference_To_v1_ObjectReference(in *LocalObjectReference, out *v1.ObjectReference, s apiconversion.Scope) error {
	out.APIVersion = in.APIVersion
	out.Kind = in.Kind
	out.Name = in.Name
	return nil
}

func Convert_v1_ObjectReference_To_v1alpha4_ObjectReference(in *v1.ObjectReference, out *ObjectReference, s apiconversion.Scope) error {
	out.APIVersion = in.APIVersion
	out.Kind = in.Kind
	out.Name = in.Name
	out.Namespace = in.Namespace
	return nil
}

func Convert_v1alpha4_ObjectReference_To_v1_ObjectReference(in *ObjectReference, out *v1.ObjectReference, s apiconversion.Scope) error {
	out.APIVersion = in.APIVersion
	out.Kind = in.Kind
	out.Name = in.Name
	out.Namespace = in.Namespace
	return nil
}

func Convert_v1_ObjectReference_To_v1alpha4_PinnedObjectReference(in *v1.ObjectReference, out *PinnedObjectReference, s apiconversion.Scope) error {
	out.APIVersion = in.APIVersion
	out.Kind = in.Kind
	out.Name = in.Name
	out.Namespace = in.Namespace
	out.UID = in.UID
	return nil
}

func Convert_v1alpha4_PinnedObjectReference_To_v1_ObjectReference(in *PinnedObjectReference, out *v1.ObjectReference, s apiconversion.Scope) error {
	out.APIVersion = in.APIVersion
	out.Kind = in.Kind
	out.Name = in.Name
	out.Namespace = in.Namespace
	out.UID = in.UID
	return nil
}
