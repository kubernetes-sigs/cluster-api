//go:build !race

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

package converter

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/randfill"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// Test is disabled when the race detector is enabled (via "//go:build !race" above) because otherwise the fuzz tests would just time out.

func TestFuzzyConversion(t *testing.T) {
	SetAPIVersionGetter(func(_ context.Context, gk schema.GroupKind) (string, error) {
		for _, gvk := range testGVKs {
			if gvk.GroupKind() == gk {
				return schema.GroupVersion{
					Group:   gk.Group,
					Version: gvk.Version,
				}.String(), nil
			}
		}
		return "", fmt.Errorf("failed to map GroupKind %s to version", gk.String())
	})

	// FIXME: implement a new test util for the new conversion.
	//t.Run("for Cluster", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
	//	Hub:         &clusterv1.Cluster{},
	//	Spoke:       &Cluster{},
	//	FuzzerFuncs: []fuzzer.FuzzerFuncs{ClusterFuzzFuncs},
	//}))
	//t.Run("for ClusterClass", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
	//	Hub:         &clusterv1.ClusterClass{},
	//	Spoke:       &ClusterClass{},
	//	FuzzerFuncs: []fuzzer.FuzzerFuncs{ClusterClassFuncs},
	//}))
	//t.Run("for Machine", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
	//	Hub:         &clusterv1.Machine{},
	//	Spoke:       &Machine{},
	//	FuzzerFuncs: []fuzzer.FuzzerFuncs{MachineFuzzFuncs},
	//}))
	//t.Run("for MachineSet", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
	//	Hub:         &clusterv1.MachineSet{},
	//	Spoke:       &MachineSet{},
	//	FuzzerFuncs: []fuzzer.FuzzerFuncs{MachineSetFuzzFuncs},
	//}))
	//t.Run("for MachineDeployment", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
	//	Hub:         &clusterv1.MachineDeployment{},
	//	Spoke:       &MachineDeployment{},
	//	FuzzerFuncs: []fuzzer.FuzzerFuncs{MachineDeploymentFuzzFuncs},
	//}))
	//t.Run("for MachineHealthCheck", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
	//	Hub:         &clusterv1.MachineHealthCheck{},
	//	Spoke:       &MachineHealthCheck{},
	//	FuzzerFuncs: []fuzzer.FuzzerFuncs{MachineHealthCheckFuzzFuncs},
	//}))
	//t.Run("for MachinePool", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
	//	Hub:         &clusterv1.MachinePool{},
	//	Spoke:       &MachinePool{},
	//	FuzzerFuncs: []fuzzer.FuzzerFuncs{MachinePoolFuzzFuncs},
	//}))
}

func ClusterFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubClusterSpec,
		hubClusterStatus,
		hubClusterVariable,
		hubFailureDomain,
		hubUnhealthyNodeCondition,
		spokeCluster,
		spokeClusterTopology,
		spokeObjectReference,
		spokeClusterStatus,
		spokeClusterVariable,
		spokeControlPlaneTopology,
		spokeMachineDeploymentTopology,
		spokeMachinePoolTopology,
		spokeMachineHealthCheckClass,
		spokeUnhealthyCondition,
	}
}

func hubClusterSpec(in *clusterv1.ClusterSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	// Ensure ref fields are always set to realistic values.
	if in.InfrastructureRef.IsDefined() {
		gvk := testGVKs[c.Int31n(4)]
		in.InfrastructureRef.APIGroup = gvk.Group
		in.InfrastructureRef.Kind = gvk.Kind
	}
	if in.ControlPlaneRef.IsDefined() {
		gvk := testGVKs[c.Int31n(4)]
		in.ControlPlaneRef.APIGroup = gvk.Group
		in.ControlPlaneRef.Kind = gvk.Kind
	}
}

func hubClusterStatus(in *clusterv1.ClusterStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.Deprecated != nil {
		if in.Deprecated.V1Beta1 == nil || reflect.DeepEqual(in.Deprecated.V1Beta1, &clusterv1.ClusterV1Beta1DeprecatedStatus{}) {
			in.Deprecated = nil
		}
	}

	if len(in.FailureDomains) > 0 {
		in.FailureDomains = nil // Remove all pre-existing potentially invalid FailureDomains
		for i := range c.Int31n(20) {
			in.FailureDomains = append(in.FailureDomains,
				clusterv1.FailureDomain{
					Name:         fmt.Sprintf("%d-%s", i, c.String(255)), // Ensure valid unique non-empty names.
					ControlPlane: ptr.To(c.Bool()),
				},
			)
		}
		// The Cluster controller always ensures alphabetic sorting when writing this field.
		slices.SortFunc(in.FailureDomains, func(a, b clusterv1.FailureDomain) int {
			if a.Name < b.Name {
				return -1
			}
			return 1
		})
	}
}

func hubClusterVariable(in *clusterv1.ClusterVariable, c randfill.Continue) {
	c.FillNoCustom(in)

	in.Value = apiextensionsv1.JSON{Raw: []byte(strconv.FormatBool(c.Bool()))}
}

func hubFailureDomain(in *clusterv1.FailureDomain, c randfill.Continue) {
	c.FillNoCustom(in)

	in.ControlPlane = ptr.To(c.Bool())
}

func hubUnhealthyNodeCondition(in *clusterv1.UnhealthyNodeCondition, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.TimeoutSeconds == nil {
		in.TimeoutSeconds = ptr.To(int32(0)) // TimeoutSeconds is a required field and nil does not round trip
	}
}

func spokeCluster(in *clusterv1beta1.Cluster, c randfill.Continue) {
	c.FillNoCustom(in)

	in.Namespace = "foo"

	// Ensure ref fields are always set to realistic values.
	if in.Spec.ControlPlaneRef != nil {
		gvk := testGVKs[c.Int31n(4)]
		in.Spec.ControlPlaneRef.APIVersion = gvk.GroupVersion().String()
		in.Spec.ControlPlaneRef.Kind = gvk.Kind
		in.Spec.ControlPlaneRef.Namespace = in.Namespace
		in.Spec.ControlPlaneRef.UID = ""
		in.Spec.ControlPlaneRef.ResourceVersion = ""
		in.Spec.ControlPlaneRef.FieldPath = ""
	}
	if in.Spec.InfrastructureRef != nil {
		gvk := testGVKs[c.Int31n(4)]
		in.Spec.InfrastructureRef.APIVersion = gvk.GroupVersion().String()
		in.Spec.InfrastructureRef.Kind = gvk.Kind
		in.Spec.InfrastructureRef.Namespace = in.Namespace
		in.Spec.InfrastructureRef.UID = ""
		in.Spec.InfrastructureRef.ResourceVersion = ""
		in.Spec.InfrastructureRef.FieldPath = ""
	}

	dropEmptyStringsCluster(in)
	if in.Spec.Topology != nil {
		if in.Spec.Topology.ControlPlane.MachineHealthCheck != nil {
			dropEmptyString(&in.Spec.Topology.ControlPlane.MachineHealthCheck.UnhealthyRange)
		}
		if in.Spec.Topology.Workers != nil {
			for i, md := range in.Spec.Topology.Workers.MachineDeployments {
				dropEmptyString(&md.FailureDomain)
				if md.MachineHealthCheck != nil {
					dropEmptyString(&md.MachineHealthCheck.UnhealthyRange)
				}
				in.Spec.Topology.Workers.MachineDeployments[i] = md
			}
		}
	}

	if in.Spec.ClusterNetwork != nil {
		if in.Spec.ClusterNetwork.Services != nil && reflect.DeepEqual(in.Spec.ClusterNetwork.Services, &clusterv1beta1.NetworkRanges{}) {
			in.Spec.ClusterNetwork.Services = nil
		}
		if in.Spec.ClusterNetwork.Pods != nil && reflect.DeepEqual(in.Spec.ClusterNetwork.Pods, &clusterv1beta1.NetworkRanges{}) {
			in.Spec.ClusterNetwork.Pods = nil
		}
		if reflect.DeepEqual(in.Spec.ClusterNetwork, &clusterv1beta1.ClusterNetwork{}) {
			in.Spec.ClusterNetwork = nil
		}
	}

	if in.Spec.Topology != nil && reflect.DeepEqual(in.Spec.Topology, &clusterv1beta1.Topology{}) {
		in.Spec.Topology = nil
	}
}

func spokeClusterTopology(in *clusterv1beta1.Topology, c randfill.Continue) {
	c.FillNoCustom(in)

	// RolloutAfter was unused and has been removed in v1beta2.
	in.RolloutAfter = nil

	if in.Workers != nil && reflect.DeepEqual(in.Workers, &clusterv1beta1.WorkersTopology{}) {
		in.Workers = nil
	}
}

func spokeClusterStatus(in *clusterv1beta1.ClusterStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &clusterv1beta1.ClusterV1Beta2Status{}) {
			in.V1Beta2 = nil
		}
	}
}

func spokeClusterVariable(in *clusterv1beta1.ClusterVariable, c randfill.Continue) {
	c.FillNoCustom(in)

	in.Value = apiextensionsv1.JSON{Raw: []byte(strconv.FormatBool(c.Bool()))}

	// Drop DefinitionFrom as we intentionally don't preserve it.
	in.DefinitionFrom = ""
}

func ClusterClassFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubClusterClassVariable,
		hubClusterClassStatusVariableDefinition,
		hubClusterClassStatus,
		hubJSONPatch,
		hubJSONSchemaProps,
		hubUnhealthyNodeCondition,
		spokeClusterClass,
		spokeObjectReference,
		spokeClusterClassStatus,
		spokeSONPatch,
		spokeJSONSchemaProps,
		spokeControlPlaneClass,
		spokeMachineDeploymentClass,
		spokeMachinePoolClass,
		spokeMachineHealthCheckClass,
		spokeUnhealthyCondition,
		spokeLocalObjectTemplate,
	}
}

func hubClusterClassVariable(in *clusterv1.ClusterClassVariable, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.Required == nil {
		in.Required = ptr.To(false) // Required is a required field and nil does not round trip
	}
}

func hubClusterClassStatusVariableDefinition(in *clusterv1.ClusterClassStatusVariableDefinition, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.Required == nil {
		in.Required = ptr.To(false) // Required is a required field and nil does not round trip
	}
}

func hubClusterClassStatus(in *clusterv1.ClusterClassStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.Deprecated != nil {
		if in.Deprecated.V1Beta1 == nil || reflect.DeepEqual(in.Deprecated.V1Beta1, &clusterv1.ClusterClassV1Beta1DeprecatedStatus{}) {
			in.Deprecated = nil
		}
	}
}

func hubJSONPatch(in *clusterv1.JSONPatch, c randfill.Continue) {
	c.FillNoCustom(in)

	// Not every random byte array is valid JSON, e.g. a string without `""`,so we're setting a valid value.
	in.Value = &apiextensionsv1.JSON{Raw: []byte("5")}
}

func hubJSONSchemaProps(in *clusterv1.JSONSchemaProps, c randfill.Continue) {
	// NOTE: We have to fuzz the individual fields manually,
	// because we cannot call `FillNoCustom` as it would lead
	// to an infinite recursion.
	_ = fillHubJSONSchemaProps(in, c)

	// Fill one level recursion.
	in.AdditionalProperties = fillHubJSONSchemaProps(&clusterv1.JSONSchemaProps{}, c)
	in.Properties = map[string]clusterv1.JSONSchemaProps{}
	for range c.Intn(5) {
		in.Properties[c.String(0)] = *fillHubJSONSchemaProps(&clusterv1.JSONSchemaProps{}, c)
	}
	in.Items = fillHubJSONSchemaProps(&clusterv1.JSONSchemaProps{}, c)
	in.AllOf = []clusterv1.JSONSchemaProps{*fillHubJSONSchemaProps(&clusterv1.JSONSchemaProps{}, c)}
	in.OneOf = []clusterv1.JSONSchemaProps{*fillHubJSONSchemaProps(&clusterv1.JSONSchemaProps{}, c)}
	in.AnyOf = []clusterv1.JSONSchemaProps{*fillHubJSONSchemaProps(&clusterv1.JSONSchemaProps{}, c)}
	in.Not = fillHubJSONSchemaProps(&clusterv1.JSONSchemaProps{}, c)
}

func fillHubJSONSchemaProps(in *clusterv1.JSONSchemaProps, c randfill.Continue) *clusterv1.JSONSchemaProps {
	in.Type = c.String(0)
	for range c.Intn(10) {
		in.Required = append(in.Required, c.String(0))
	}
	in.Format = c.String(0)
	in.Pattern = c.String(0)
	if c.Bool() {
		in.MaxItems = ptr.To(c.Int63())
		in.MinItems = ptr.To(c.Int63())
		in.MaxLength = ptr.To(c.Int63())
		in.MinLength = ptr.To(c.Int63())
		in.Maximum = ptr.To(c.Int63())
		in.Maximum = ptr.To(c.Int63())
		in.Minimum = ptr.To(c.Int63())
		in.UniqueItems = ptr.To(c.Bool())
		in.ExclusiveMaximum = ptr.To(c.Bool())
		in.ExclusiveMinimum = ptr.To(c.Bool())
		in.XPreserveUnknownFields = ptr.To(c.Bool())
		in.XIntOrString = ptr.To(c.Bool())
	}

	// Not every random byte array is valid JSON, e.g. a string without `""`,so we're setting valid values.
	in.Enum = []apiextensionsv1.JSON{
		{Raw: []byte("\"a\"")},
		{Raw: []byte("\"b\"")},
		{Raw: []byte("\"c\"")},
	}
	in.Default = &apiextensionsv1.JSON{Raw: []byte(strconv.FormatBool(c.Bool()))}

	return in
}

func spokeClusterClass(in *clusterv1beta1.ClusterClass, c randfill.Continue) {
	c.FillNoCustom(in)

	in.Namespace = "foo"

	dropEmptyStringsClusterClass(in)
	if in.Spec.ControlPlane.MachineHealthCheck != nil {
		dropEmptyString(&in.Spec.ControlPlane.MachineHealthCheck.UnhealthyRange)
	}
	for i, md := range in.Spec.Workers.MachineDeployments {
		if md.MachineHealthCheck != nil {
			dropEmptyString(&md.MachineHealthCheck.UnhealthyRange)
		}
		in.Spec.Workers.MachineDeployments[i] = md
	}
	if reflect.DeepEqual(in.Spec.InfrastructureNamingStrategy, &clusterv1beta1.InfrastructureNamingStrategy{}) {
		in.Spec.InfrastructureNamingStrategy = nil
	}
	if reflect.DeepEqual(in.Spec.ControlPlane.NamingStrategy, &clusterv1beta1.ControlPlaneClassNamingStrategy{}) {
		in.Spec.ControlPlane.NamingStrategy = nil
	}
	for i, md := range in.Spec.Workers.MachineDeployments {
		if reflect.DeepEqual(md.NamingStrategy, &clusterv1beta1.MachineDeploymentClassNamingStrategy{}) {
			md.NamingStrategy = nil
		}
		in.Spec.Workers.MachineDeployments[i] = md
	}
	for i, mp := range in.Spec.Workers.MachinePools {
		if reflect.DeepEqual(mp.NamingStrategy, &clusterv1beta1.MachinePoolClassNamingStrategy{}) {
			mp.NamingStrategy = nil
		}
		in.Spec.Workers.MachinePools[i] = mp
	}
}

func spokeClusterClassStatus(in *clusterv1beta1.ClusterClassStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &clusterv1beta1.ClusterClassV1Beta2Status{}) {
			in.V1Beta2 = nil
		}
	}
}

func spokeSONPatch(in *clusterv1beta1.JSONPatch, c randfill.Continue) {
	c.FillNoCustom(in)

	// Not every random byte array is valid JSON, e.g. a string without `""`,so we're setting a valid value.
	in.Value = &apiextensionsv1.JSON{Raw: []byte("5")}
}

func spokeJSONSchemaProps(in *clusterv1beta1.JSONSchemaProps, c randfill.Continue) {
	// NOTE: We have to fuzz the individual fields manually,
	// because we cannot call `FillNoCustom` as it would lead
	// to an infinite recursion.
	_ = fillSpokeJSONSchemaProps(in, c)

	// Fill one level recursion.
	in.AdditionalProperties = fillSpokeJSONSchemaProps(&clusterv1beta1.JSONSchemaProps{}, c)
	in.Properties = map[string]clusterv1beta1.JSONSchemaProps{}
	for range c.Intn(5) {
		in.Properties[c.String(0)] = *fillSpokeJSONSchemaProps(&clusterv1beta1.JSONSchemaProps{}, c)
	}
	in.Items = fillSpokeJSONSchemaProps(&clusterv1beta1.JSONSchemaProps{}, c)
	in.AllOf = []clusterv1beta1.JSONSchemaProps{*fillSpokeJSONSchemaProps(&clusterv1beta1.JSONSchemaProps{}, c)}
	in.OneOf = []clusterv1beta1.JSONSchemaProps{*fillSpokeJSONSchemaProps(&clusterv1beta1.JSONSchemaProps{}, c)}
	in.AnyOf = []clusterv1beta1.JSONSchemaProps{*fillSpokeJSONSchemaProps(&clusterv1beta1.JSONSchemaProps{}, c)}
	in.Not = fillSpokeJSONSchemaProps(&clusterv1beta1.JSONSchemaProps{}, c)
}

func fillSpokeJSONSchemaProps(in *clusterv1beta1.JSONSchemaProps, c randfill.Continue) *clusterv1beta1.JSONSchemaProps {
	in.Format = c.String(0)
	in.Pattern = c.String(0)
	if c.Bool() {
		in.MaxItems = ptr.To(c.Int63())
		in.MinItems = ptr.To(c.Int63())
		in.MaxLength = ptr.To(c.Int63())
		in.MinLength = ptr.To(c.Int63())
		in.Maximum = ptr.To(c.Int63())
		in.Maximum = ptr.To(c.Int63())
		in.Minimum = ptr.To(c.Int63())
	}
	in.UniqueItems = c.Bool()
	in.ExclusiveMaximum = c.Bool()
	in.ExclusiveMinimum = c.Bool()
	in.XPreserveUnknownFields = c.Bool()
	in.XIntOrString = c.Bool()

	// Not every random byte array is valid JSON, e.g. a string without `""`,so we're setting valid values.
	in.Enum = []apiextensionsv1.JSON{
		{Raw: []byte("\"a\"")},
		{Raw: []byte("\"b\"")},
		{Raw: []byte("\"c\"")},
	}
	in.Default = &apiextensionsv1.JSON{Raw: []byte(strconv.FormatBool(c.Bool()))}

	return in
}

func spokeLocalObjectTemplate(in *clusterv1beta1.LocalObjectTemplate, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.Ref == nil {
		return
	}

	in.Ref = &corev1.ObjectReference{
		APIVersion: in.Ref.APIVersion,
		Kind:       in.Ref.Kind,
		Name:       in.Ref.Name,
		Namespace:  "foo",
	}
}

func MachineFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubMachineSpec,
		hubMachineStatus,
		spokeMachine,
		spokeMachineSpec,
		spokeMachineStatus,
	}
}

func hubMachineSpec(in *clusterv1.MachineSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	// Ensure ref fields are always set to realistic values.
	gvk := testGVKs[c.Int31n(4)]
	in.InfrastructureRef.APIGroup = gvk.Group
	in.InfrastructureRef.Kind = gvk.Kind

	if in.Bootstrap.ConfigRef.IsDefined() {
		gvk := testGVKs[c.Int31n(4)]
		in.Bootstrap.ConfigRef.APIGroup = gvk.Group
		in.Bootstrap.ConfigRef.Kind = gvk.Kind
	}
}

func hubMachineStatus(in *clusterv1.MachineStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.Deprecated != nil {
		if in.Deprecated.V1Beta1 == nil || reflect.DeepEqual(in.Deprecated.V1Beta1, &clusterv1.MachineV1Beta1DeprecatedStatus{}) {
			in.Deprecated = nil
		}
	}
}

func spokeMachine(in *clusterv1beta1.Machine, c randfill.Continue) {
	c.FillNoCustom(in)

	fillMachineSpec(&in.Spec, c, in.Namespace)

	dropEmptyStringsMachineSpec(&in.Spec)
}

func fillMachineSpec(spec *clusterv1beta1.MachineSpec, c randfill.Continue, namespace string) {
	// Ensure ref fields are always set to realistic values.
	if spec.Bootstrap.ConfigRef != nil {
		gvk := testGVKs[c.Int31n(4)]
		spec.Bootstrap.ConfigRef.APIVersion = gvk.GroupVersion().String()
		spec.Bootstrap.ConfigRef.Kind = gvk.Kind
		spec.Bootstrap.ConfigRef.Namespace = namespace
		spec.Bootstrap.ConfigRef.UID = ""
		spec.Bootstrap.ConfigRef.ResourceVersion = ""
		spec.Bootstrap.ConfigRef.FieldPath = ""
	}
	gvk := testGVKs[c.Int31n(4)]
	spec.InfrastructureRef.APIVersion = gvk.GroupVersion().String()
	spec.InfrastructureRef.Kind = gvk.Kind
	spec.InfrastructureRef.Namespace = namespace
	spec.InfrastructureRef.UID = ""
	spec.InfrastructureRef.ResourceVersion = ""
	spec.InfrastructureRef.FieldPath = ""
}

func spokeMachineSpec(in *clusterv1beta1.MachineSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.NodeDrainTimeout != nil {
		in.NodeDrainTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.NodeVolumeDetachTimeout != nil {
		in.NodeVolumeDetachTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.NodeDeletionTimeout != nil {
		in.NodeDeletionTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
}

func spokeMachineStatus(in *clusterv1beta1.MachineStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &clusterv1beta1.MachineV1Beta2Status{}) {
			in.V1Beta2 = nil
		}
	}

	if in.NodeRef != nil {
		// Drop everything except name
		in.NodeRef = &corev1.ObjectReference{
			Name:       "node-" + in.NodeRef.Name, // NodeRef's with empty Name's don't survive the round trip.
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Node",
		}
	}
	if reflect.DeepEqual(in.LastUpdated, &metav1.Time{}) {
		in.LastUpdated = nil
	}
	if reflect.DeepEqual(in.CertificatesExpiryDate, &metav1.Time{}) {
		in.CertificatesExpiryDate = nil
	}
	if in.Deletion != nil {
		if reflect.DeepEqual(in.Deletion.NodeDrainStartTime, &metav1.Time{}) {
			in.Deletion.NodeDrainStartTime = nil
		}
		if reflect.DeepEqual(in.Deletion.WaitForNodeVolumeDetachStartTime, &metav1.Time{}) {
			in.Deletion.WaitForNodeVolumeDetachStartTime = nil
		}
	}
}

func MachineSetFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubMachineSetStatus,
		hubMachineSpec,
		spokeMachineSet,
		spokeMachineSetStatus,
		spokeMachineSpec,
	}
}

func hubMachineSetStatus(in *clusterv1.MachineSetStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Always create struct with at least one mandatory fields.
	if in.Deprecated == nil {
		in.Deprecated = &clusterv1.MachineSetDeprecatedStatus{}
	}
	if in.Deprecated.V1Beta1 == nil {
		in.Deprecated.V1Beta1 = &clusterv1.MachineSetV1Beta1DeprecatedStatus{}
	}
	// nil becomes &0 after hub => spoke => hub conversion
	// This is acceptable as usually Replicas is set and controllers using older apiVersions are not writing MachineSet status.
	if in.Replicas == nil {
		in.Replicas = ptr.To(int32(0))
	}
}

func spokeMachineSet(in *clusterv1beta1.MachineSet, c randfill.Continue) {
	c.FillNoCustom(in)

	fillMachineSpec(&in.Spec.Template.Spec, c, in.Namespace)

	dropEmptyStringsMachineSpec(&in.Spec.Template.Spec)

	if reflect.DeepEqual(in.Spec.MachineNamingStrategy, &clusterv1beta1.MachineNamingStrategy{}) {
		in.Spec.MachineNamingStrategy = nil
	}
}

func spokeMachineSetStatus(in *clusterv1beta1.MachineSetStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &clusterv1beta1.MachineSetV1Beta2Status{}) {
			in.V1Beta2 = nil
		}
	}
}

func MachineDeploymentFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubMachineDeploymentStatus,
		hubMachineSpec,
		spokeMachineDeployment,
		spokeMachineDeploymentSpec,
		spokeMachineDeploymentStatus,
		spokeMachineSpec,
	}
}

func hubMachineDeploymentStatus(in *clusterv1.MachineDeploymentStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Always create struct with at least one mandatory fields.
	if in.Deprecated == nil {
		in.Deprecated = &clusterv1.MachineDeploymentDeprecatedStatus{}
	}
	if in.Deprecated.V1Beta1 == nil {
		in.Deprecated.V1Beta1 = &clusterv1.MachineDeploymentV1Beta1DeprecatedStatus{}
	}
	// nil becomes &0 after hub => spoke => hub conversion
	// This is acceptable as usually Replicas is set and controllers using older apiVersions are not writing MachineSet status.
	if in.Replicas == nil {
		in.Replicas = ptr.To(int32(0))
	}
}

func spokeMachineDeployment(in *clusterv1beta1.MachineDeployment, c randfill.Continue) {
	c.FillNoCustom(in)

	fillMachineSpec(&in.Spec.Template.Spec, c, in.Namespace)

	dropEmptyStringsMachineSpec(&in.Spec.Template.Spec)

	if reflect.DeepEqual(in.Spec.MachineNamingStrategy, &clusterv1beta1.MachineNamingStrategy{}) {
		in.Spec.MachineNamingStrategy = nil
	}
}

func spokeMachineDeploymentSpec(in *clusterv1beta1.MachineDeploymentSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	// Drop ProgressDeadlineSeconds as we intentionally don't preserve it.
	in.ProgressDeadlineSeconds = nil

	// Drop RevisionHistoryLimit as we intentionally don't preserve it.
	in.RevisionHistoryLimit = nil

	if in.Strategy != nil {
		if in.Strategy.RollingUpdate != nil {
			if in.Strategy.RollingUpdate.DeletePolicy != nil && *in.Strategy.RollingUpdate.DeletePolicy == "" {
				// &"" Is not a valid value for DeletePolicy as the enum validation enforces an enum value if DeletePolicy is set.
				in.Strategy.RollingUpdate.DeletePolicy = nil
			}
			if reflect.DeepEqual(in.Strategy.RollingUpdate, &clusterv1beta1.MachineRollingUpdateDeployment{}) {
				in.Strategy.RollingUpdate = nil
			}
		}
		if in.Strategy.Remediation != nil && reflect.DeepEqual(in.Strategy.Remediation, &clusterv1beta1.RemediationStrategy{}) {
			in.Strategy.Remediation = nil
		}
		if reflect.DeepEqual(in.Strategy, &clusterv1beta1.MachineDeploymentStrategy{}) {
			in.Strategy = nil
		}
	}
	if reflect.DeepEqual(in.RolloutAfter, &metav1.Time{}) {
		in.RolloutAfter = nil
	}
}

func spokeMachineDeploymentStatus(in *clusterv1beta1.MachineDeploymentStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &clusterv1beta1.MachineDeploymentV1Beta2Status{}) {
			in.V1Beta2 = nil
		}
	}
}

func MachineHealthCheckFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubUnhealthyNodeCondition,
		hubMachineHealthCheckStatus,
		spokeMachineHealthCheck,
		spokeMachineHealthCheckSpec,
		spokeObjectReference,
		spokeMachineHealthCheckStatus,
		spokeUnhealthyCondition,
	}
}

func hubMachineHealthCheckStatus(in *clusterv1.MachineHealthCheckStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.Deprecated != nil {
		if in.Deprecated.V1Beta1 == nil || reflect.DeepEqual(in.Deprecated.V1Beta1, &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{}) {
			in.Deprecated = nil
		}
	}
}

func spokeMachineHealthCheck(in *clusterv1beta1.MachineHealthCheck, c randfill.Continue) {
	c.FillNoCustom(in)

	in.Namespace = "foo"

	dropEmptyString(&in.Spec.UnhealthyRange)
}

func spokeMachineHealthCheckStatus(in *clusterv1beta1.MachineHealthCheckStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &clusterv1beta1.MachineHealthCheckV1Beta2Status{}) {
			in.V1Beta2 = nil
		}
	}
}

func spokeObjectReference(in *corev1.ObjectReference, c randfill.Continue) {
	c.FillNoCustom(in)

	if in == nil {
		return
	}

	in.Namespace = "foo"
	in.Name = "bar" // Also set Name, Namespace alone won't survive the round trip
	in.UID = types.UID("")
	in.ResourceVersion = ""
	in.FieldPath = ""
}

func MachinePoolFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubMachinePoolStatus,
		hubMachineSpec,
		spokeMachinePool,
		spokeMachinePoolStatus,
		spokeMachineSpec,
	}
}

func hubMachinePoolStatus(in *clusterv1.MachinePoolStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Always create struct with at least one mandatory fields.
	if in.Deprecated == nil {
		in.Deprecated = &clusterv1.MachinePoolDeprecatedStatus{}
	}
	if in.Deprecated.V1Beta1 == nil {
		in.Deprecated.V1Beta1 = &clusterv1.MachinePoolV1Beta1DeprecatedStatus{}
	}

	// nil becomes &0 after hub => spoke => hub conversion
	// This is acceptable as usually Replicas is set and controllers using older apiVersions are not writing MachineSet status.
	if in.Replicas == nil {
		in.Replicas = ptr.To(int32(0))
	}
}

func spokeMachinePool(in *clusterv1beta1.MachinePool, c randfill.Continue) {
	c.FillNoCustom(in)

	fillMachineSpec(&in.Spec.Template.Spec, c, in.Namespace)

	dropEmptyStringsMachineSpec(&in.Spec.Template.Spec)
}

func spokeMachinePoolStatus(in *clusterv1beta1.MachinePoolStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &clusterv1beta1.MachinePoolV1Beta2Status{}) {
			in.V1Beta2 = nil
		}
	}
}

func spokeControlPlaneTopology(in *clusterv1beta1.ControlPlaneTopology, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.NodeDrainTimeout != nil {
		in.NodeDrainTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.NodeVolumeDetachTimeout != nil {
		in.NodeVolumeDetachTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.NodeDeletionTimeout != nil {
		in.NodeDeletionTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}

	if in.Variables != nil && reflect.DeepEqual(in.Variables, &clusterv1beta1.ControlPlaneVariables{}) {
		in.Variables = nil
	}
}

func spokeMachineDeploymentTopology(in *clusterv1beta1.MachineDeploymentTopology, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.NodeDrainTimeout != nil {
		in.NodeDrainTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.NodeVolumeDetachTimeout != nil {
		in.NodeVolumeDetachTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.NodeDeletionTimeout != nil {
		in.NodeDeletionTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.Variables != nil && reflect.DeepEqual(in.Variables, &clusterv1beta1.MachineDeploymentVariables{}) {
		in.Variables = nil
	}
	if in.Strategy != nil {
		if in.Strategy.RollingUpdate != nil {
			if in.Strategy.RollingUpdate.DeletePolicy != nil && *in.Strategy.RollingUpdate.DeletePolicy == "" {
				// &"" Is not a valid value for DeletePolicy as the enum validation enforces an enum value if DeletePolicy is set.
				in.Strategy.RollingUpdate.DeletePolicy = nil
			}
			if reflect.DeepEqual(in.Strategy.RollingUpdate, &clusterv1beta1.MachineRollingUpdateDeployment{}) {
				in.Strategy.RollingUpdate = nil
			}
		}
		if in.Strategy.Remediation != nil && reflect.DeepEqual(in.Strategy.Remediation, &clusterv1beta1.RemediationStrategy{}) {
			in.Strategy.Remediation = nil
		}
		if reflect.DeepEqual(in.Strategy, &clusterv1beta1.MachineDeploymentStrategy{}) {
			in.Strategy = nil
		}
	}
}

func spokeMachinePoolTopology(in *clusterv1beta1.MachinePoolTopology, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.NodeDrainTimeout != nil {
		in.NodeDrainTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.NodeVolumeDetachTimeout != nil {
		in.NodeVolumeDetachTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.NodeDeletionTimeout != nil {
		in.NodeDeletionTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.Variables != nil && reflect.DeepEqual(in.Variables, &clusterv1beta1.MachinePoolVariables{}) {
		in.Variables = nil
	}
}

func spokeMachineHealthCheckClass(in *clusterv1beta1.MachineHealthCheckClass, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.NodeStartupTimeout != nil {
		in.NodeStartupTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
}

func spokeControlPlaneClass(in *clusterv1beta1.ControlPlaneClass, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.NodeDrainTimeout != nil {
		in.NodeDrainTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.NodeVolumeDetachTimeout != nil {
		in.NodeVolumeDetachTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.NodeDeletionTimeout != nil {
		in.NodeDeletionTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
}

func spokeMachineDeploymentClass(in *clusterv1beta1.MachineDeploymentClass, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.NodeDrainTimeout != nil {
		in.NodeDrainTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.NodeVolumeDetachTimeout != nil {
		in.NodeVolumeDetachTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.NodeDeletionTimeout != nil {
		in.NodeDeletionTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.Strategy != nil {
		if in.Strategy.RollingUpdate != nil {
			if in.Strategy.RollingUpdate.DeletePolicy != nil && *in.Strategy.RollingUpdate.DeletePolicy == "" {
				// &"" Is not a valid value for DeletePolicy as the enum validation enforces an enum value if DeletePolicy is set.
				in.Strategy.RollingUpdate.DeletePolicy = nil
			}
			if reflect.DeepEqual(in.Strategy.RollingUpdate, &clusterv1beta1.MachineRollingUpdateDeployment{}) {
				in.Strategy.RollingUpdate = nil
			}
		}
		if in.Strategy.Remediation != nil && reflect.DeepEqual(in.Strategy.Remediation, &clusterv1beta1.RemediationStrategy{}) {
			in.Strategy.Remediation = nil
		}
		if reflect.DeepEqual(in.Strategy, &clusterv1beta1.MachineDeploymentStrategy{}) {
			in.Strategy = nil
		}
	}
}

func spokeMachinePoolClass(in *clusterv1beta1.MachinePoolClass, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.NodeDrainTimeout != nil {
		in.NodeDrainTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.NodeVolumeDetachTimeout != nil {
		in.NodeVolumeDetachTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.NodeDeletionTimeout != nil {
		in.NodeDeletionTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
}

func spokeMachineHealthCheckSpec(in *clusterv1beta1.MachineHealthCheckSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.NodeStartupTimeout != nil {
		in.NodeStartupTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
}

func spokeUnhealthyCondition(in *clusterv1beta1.UnhealthyCondition, c randfill.Continue) {
	c.FillNoCustom(in)

	in.Timeout = metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second}
}

func convertMachineSpecToContractVersionedObjectReference(src *clusterv1beta1.MachineSpec, dst *clusterv1.MachineSpec) error {
	infraRef, err := convertToContractVersionedObjectReference(&src.InfrastructureRef)
	if err != nil {
		return err
	}
	dst.InfrastructureRef = infraRef

	if src.Bootstrap.ConfigRef != nil {
		bootstrapRef, err := convertToContractVersionedObjectReference(src.Bootstrap.ConfigRef)
		if err != nil {
			return err
		}
		dst.Bootstrap.ConfigRef = bootstrapRef
	}

	return nil
}

func convertMachineSpecToObjectReference(ctx context.Context, src *clusterv1.MachineSpec, dst *clusterv1beta1.MachineSpec, namespace string) error {
	if src.InfrastructureRef.IsDefined() {
		infraRef, err := convertToObjectReference(ctx, src.InfrastructureRef, namespace)
		if err != nil {
			return err
		}
		dst.InfrastructureRef = *infraRef
	}

	if src.Bootstrap.ConfigRef.IsDefined() {
		bootstrapRef, err := convertToObjectReference(ctx, src.Bootstrap.ConfigRef, namespace)
		if err != nil {
			return err
		}
		dst.Bootstrap.ConfigRef = bootstrapRef
	}

	return nil
}

var testGVKs = []schema.GroupVersionKind{
	{
		Group:   "controlplane.cluster.x-k8s.io",
		Version: "v1beta4",
		Kind:    "KubeadmControlPlane",
	},
	{
		Group:   "controlplane.cluster.x-k8s.io",
		Version: "v1beta7",
		Kind:    "AWSManagedControlPlane",
	},
	{
		Group:   "infrastructure.cluster.x-k8s.io",
		Version: "v1beta3",
		Kind:    "DockerCluster",
	},
	{
		Group:   "infrastructure.cluster.x-k8s.io",
		Version: "v1beta6",
		Kind:    "AWSCluster",
	},
}
