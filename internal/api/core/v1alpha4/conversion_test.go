//go:build !race

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

package v1alpha4

import (
	"reflect"
	"strconv"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/randfill"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

// Test is disabled when the race detector is enabled (via "//go:build !race" above) because otherwise the fuzz tests would just time out.

func TestFuzzyConversion(t *testing.T) {
	t.Run("for Cluster", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.Cluster{},
		Spoke:       &Cluster{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{ClusterFuzzFuncs},
	}))
	t.Run("for ClusterClass", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.ClusterClass{},
		Spoke:       &ClusterClass{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{ClusterClassFuzzFuncs},
	}))
	t.Run("for Machine", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.Machine{},
		Spoke:       &Machine{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{MachineFuzzFunc},
	}))
	t.Run("for MachineSet", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.MachineSet{},
		Spoke:       &MachineSet{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{MachineSetFuzzFuncs},
	}))
	t.Run("for MachineDeployment", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.MachineDeployment{},
		Spoke:       &MachineDeployment{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{MachineDeploymentFuzzFuncs},
	}))
	t.Run("for MachineHealthCheck", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.MachineHealthCheck{},
		Spoke:       &MachineHealthCheck{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{MachineHealthCheckFuzzFunc},
	}))
	t.Run("for MachinePool", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.MachinePool{},
		Spoke:       &MachinePool{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{MachinePoolFuzzFuncs},
	}))
}

func MachineFuzzFunc(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubMachineStatus,
		spokeMachineSpec,
		spokeMachineStatus,
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

	// Drop empty structs with only omit empty fields.
	if in.Initialization != nil {
		if reflect.DeepEqual(in.Initialization, &clusterv1.MachineInitializationStatus{}) {
			in.Initialization = nil
		}
	}
}

func spokeMachineSpec(in *MachineSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.NodeDrainTimeout != nil {
		in.NodeDrainTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
}

func spokeMachineStatus(in *MachineStatus, c randfill.Continue) {
	c.FillNoCustom(in)

	// These fields have been removed in v1beta1
	// data is going to be lost, so we're forcing zero values to avoid round trip errors.
	in.Version = nil

	if in.NodeRef != nil {
		// Drop everything except name
		in.NodeRef = &corev1.ObjectReference{
			Name:       in.NodeRef.Name,
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Node",
		}
	}
}

func ClusterFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubClusterVariable,
		hubClusterStatus,
		spokeClusterTopology,
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

	// Drop empty structs with only omit empty fields.
	if in.Initialization != nil {
		if reflect.DeepEqual(in.Initialization, &clusterv1.ClusterInitializationStatus{}) {
			in.Initialization = nil
		}
	}
}

func hubClusterVariable(in *clusterv1.ClusterVariable, c randfill.Continue) {
	c.FillNoCustom(in)

	// Not every random byte array is valid JSON, e.g. a string without `""`,so we're setting a valid value.
	in.Value = apiextensionsv1.JSON{Raw: []byte("\"test-string\"")}
}

func spokeClusterTopology(in *Topology, c randfill.Continue) {
	c.FillNoCustom(in)

	// RolloutAfter was unused and has been removed in v1beta2.
	in.RolloutAfter = nil
}

func ClusterClassFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubJSONPatch,
		hubJSONSchemaProps,
		spokeClusterClass,
		spokeLocalObjectTemplate,
	}
}

func spokeClusterClass(in *ClusterClass, c randfill.Continue) {
	c.FillNoCustom(in)

	in.Namespace = "foo"
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
	in.Type = c.String(0)
	for i := 0; i < c.Intn(10); i++ {
		in.Required = append(in.Required, c.String(0))
	}
	in.MaxItems = ptr.To(c.Int63())
	in.MinItems = ptr.To(c.Int63())
	in.UniqueItems = c.Bool()
	in.Format = c.String(0)
	in.MaxLength = ptr.To(c.Int63())
	in.MinLength = ptr.To(c.Int63())
	in.Pattern = c.String(0)
	in.Maximum = ptr.To(c.Int63())
	in.Maximum = ptr.To(c.Int63())
	in.ExclusiveMaximum = c.Bool()
	in.Minimum = ptr.To(c.Int63())
	in.ExclusiveMinimum = c.Bool()

	// Not every random byte array is valid JSON, e.g. a string without `""`,so we're setting valid values.
	in.Enum = []apiextensionsv1.JSON{
		{Raw: []byte("\"a\"")},
		{Raw: []byte("\"b\"")},
		{Raw: []byte("\"c\"")},
	}
	in.Default = &apiextensionsv1.JSON{Raw: []byte(strconv.FormatBool(c.Bool()))}

	// We're using a copy of the current JSONSchemaProps,
	// because we cannot recursively fuzz new schemas.
	in2 := in.DeepCopy()
	in.AdditionalProperties = in2

	// We're using a copy of the current JSONSchemaProps,
	// because we cannot recursively fuzz new schemas.
	in.Properties = map[string]clusterv1.JSONSchemaProps{}
	for i := 0; i < c.Intn(10); i++ {
		in.Properties[c.String(0)] = *in2
	}
	in.Items = in2
}

func spokeLocalObjectTemplate(in *LocalObjectTemplate, c randfill.Continue) {
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

func MachineSetFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		spokeMachineSpec,
		hubMachineSetStatus,
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

func MachineDeploymentFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubMachineDeploymentStatus,
		spokeMachineDeploymentSpec,
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

func spokeMachineDeploymentSpec(in *MachineDeploymentSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	// Drop ProgressDeadlineSeconds as we intentionally don't preserve it.
	in.ProgressDeadlineSeconds = nil

	// Drop RevisionHistoryLimit as we intentionally don't preserve it.
	in.RevisionHistoryLimit = nil
}

func MachineHealthCheckFuzzFunc(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubMachineHealthCheckStatus,
		spokeMachineHealthCheck,
		spokeMachineHealthCheckSpec,
		spokeObjectReference,
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

func spokeMachineHealthCheck(in *MachineHealthCheck, c randfill.Continue) {
	c.FillNoCustom(in)

	in.Namespace = "foo"
}

func spokeObjectReference(in *corev1.ObjectReference, c randfill.Continue) {
	c.FillNoCustom(in)

	if in == nil {
		return
	}

	in.Namespace = "foo"
	in.UID = types.UID("")
	in.ResourceVersion = ""
	in.FieldPath = ""
}

func MachinePoolFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubMachinePoolStatus,
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

	// Drop empty structs with only omit empty fields.
	if in.Initialization != nil {
		if reflect.DeepEqual(in.Initialization, &clusterv1.MachinePoolInitializationStatus{}) {
			in.Initialization = nil
		}
	}

	// nil becomes &0 after hub => spoke => hub conversion
	// This is acceptable as usually Replicas is set and controllers using older apiVersions are not writing MachineSet status.
	if in.Replicas == nil {
		in.Replicas = ptr.To(int32(0))
	}
}

func spokeMachineHealthCheckSpec(in *MachineHealthCheckSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.NodeStartupTimeout != nil {
		in.NodeStartupTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
}

func spokeUnhealthyCondition(in *UnhealthyCondition, c randfill.Continue) {
	c.FillNoCustom(in)

	in.Timeout = metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second}
}
