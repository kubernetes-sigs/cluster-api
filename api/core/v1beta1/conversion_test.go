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

package v1beta1

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/randfill"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

// Test is disabled when the race detector is enabled (via "//go:build !race" above) because otherwise the fuzz tests would just time out.

func TestFuzzyConversion(t *testing.T) {
	SetAPIVersionGetter(func(gk schema.GroupKind) (string, error) {
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

	t.Run("for Cluster", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.Cluster{},
		Spoke:       &Cluster{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{ClusterFuzzFuncs},
	}))
	t.Run("for ClusterClass", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.ClusterClass{},
		Spoke:       &ClusterClass{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{ClusterClassFuncs},
	}))
	t.Run("for Machine", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.Machine{},
		Spoke:       &Machine{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{MachineFuzzFuncs},
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
		FuzzerFuncs: []fuzzer.FuzzerFuncs{MachineHealthCheckFuzzFuncs},
	}))
	t.Run("for MachinePool", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.MachinePool{},
		Spoke:       &MachinePool{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{MachinePoolFuzzFuncs},
	}))
}

func ClusterFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubClusterSpec,
		hubClusterStatus,
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
	if in.InfrastructureRef != nil {
		gvk := testGVKs[c.Int31n(4)]
		in.InfrastructureRef.APIGroup = gvk.Group
		in.InfrastructureRef.Kind = gvk.Kind
	}
	if in.ControlPlaneRef != nil {
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

	// Drop empty structs with only omit empty fields.
	if in.Initialization != nil {
		if reflect.DeepEqual(in.Initialization, &clusterv1.ClusterInitializationStatus{}) {
			in.Initialization = nil
		}
	}

	if len(in.FailureDomains) > 0 {
		in.FailureDomains = nil // Remove all pre-existing potentially invalid FailureDomains
		for i := range c.Int31n(20) {
			in.FailureDomains = append(in.FailureDomains,
				clusterv1.FailureDomain{
					Name:         fmt.Sprintf("%d-%s", i, c.String(255)), // Ensure valid unique non-empty names.
					ControlPlane: c.Bool(),
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

func spokeCluster(in *Cluster, c randfill.Continue) {
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
}

func spokeClusterTopology(in *Topology, c randfill.Continue) {
	c.FillNoCustom(in)

	// RolloutAfter was unused and has been removed in v1beta2.
	in.RolloutAfter = nil
}

func spokeClusterStatus(in *ClusterStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &ClusterV1Beta2Status{}) {
			in.V1Beta2 = nil
		}
	}
}

func spokeClusterVariable(in *ClusterVariable, c randfill.Continue) {
	c.FillNoCustom(in)

	// Drop DefinitionFrom as we intentionally don't preserve it.
	in.DefinitionFrom = ""
}

func ClusterClassFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubClusterClassStatus,
		hubJSONSchemaProps,
		spokeClusterClass,
		spokeObjectReference,
		spokeClusterClassStatus,
		spokeJSONSchemaProps,
		spokeControlPlaneClass,
		spokeMachineDeploymentClass,
		spokeMachinePoolClass,
		spokeMachineHealthCheckClass,
		spokeUnhealthyCondition,
		spokeLocalObjectTemplate,
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

func hubJSONSchemaProps(in *clusterv1.JSONSchemaProps, c randfill.Continue) {
	// NOTE: We have to fuzz the individual fields manually,
	// because we cannot call `FillNoCustom` as it would lead
	// to an infinite recursion.
	in.Type = c.String(0)
	for range c.Intn(10) {
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
	for range c.Intn(10) {
		in.Properties[c.String(0)] = *in2
	}
	in.Items = in2
}

func spokeClusterClass(in *ClusterClass, c randfill.Continue) {
	c.FillNoCustom(in)

	in.Namespace = "foo"
}

func spokeClusterClassStatus(in *ClusterClassStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &ClusterClassV1Beta2Status{}) {
			in.V1Beta2 = nil
		}
	}
}

func spokeJSONSchemaProps(in *JSONSchemaProps, c randfill.Continue) {
	// NOTE: We have to fuzz the individual fields manually,
	// because we cannot call `FillNoCustom` as it would lead
	// to an infinite recursion.
	in.Type = c.String(0)
	for range c.Intn(10) {
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
	in.Properties = map[string]JSONSchemaProps{}
	for range c.Intn(10) {
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

	if in.Bootstrap.ConfigRef != nil {
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

	// Drop empty structs with only omit empty fields.
	if in.Initialization != nil {
		if reflect.DeepEqual(in.Initialization, &clusterv1.MachineInitializationStatus{}) {
			in.Initialization = nil
		}
	}
}

func spokeMachine(in *Machine, c randfill.Continue) {
	c.FillNoCustom(in)

	fillMachineSpec(&in.Spec, c, in.Namespace)
}

func fillMachineSpec(spec *MachineSpec, c randfill.Continue, namespace string) {
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

func spokeMachineSpec(in *MachineSpec, c randfill.Continue) {
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

func spokeMachineStatus(in *MachineStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &MachineV1Beta2Status{}) {
			in.V1Beta2 = nil
		}
	}

	if in.NodeRef != nil {
		// Drop everything except name
		in.NodeRef = &corev1.ObjectReference{
			Name:       in.NodeRef.Name,
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Node",
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

func spokeMachineSet(in *MachineSet, c randfill.Continue) {
	c.FillNoCustom(in)

	fillMachineSpec(&in.Spec.Template.Spec, c, in.Namespace)
}

func spokeMachineSetStatus(in *MachineSetStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &MachineSetV1Beta2Status{}) {
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

func spokeMachineDeployment(in *MachineDeployment, c randfill.Continue) {
	c.FillNoCustom(in)

	fillMachineSpec(&in.Spec.Template.Spec, c, in.Namespace)
}

func spokeMachineDeploymentSpec(in *MachineDeploymentSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	// Drop ProgressDeadlineSeconds as we intentionally don't preserve it.
	in.ProgressDeadlineSeconds = nil

	// Drop RevisionHistoryLimit as we intentionally don't preserve it.
	in.RevisionHistoryLimit = nil
}

func spokeMachineDeploymentStatus(in *MachineDeploymentStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &MachineDeploymentV1Beta2Status{}) {
			in.V1Beta2 = nil
		}
	}
}

func MachineHealthCheckFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
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

func spokeMachineHealthCheck(in *MachineHealthCheck, c randfill.Continue) {
	c.FillNoCustom(in)

	in.Namespace = "foo"
}

func spokeMachineHealthCheckStatus(in *MachineHealthCheckStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &MachineHealthCheckV1Beta2Status{}) {
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

func spokeMachinePool(in *MachinePool, c randfill.Continue) {
	c.FillNoCustom(in)

	fillMachineSpec(&in.Spec.Template.Spec, c, in.Namespace)
}

func spokeMachinePoolStatus(in *MachinePoolStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &MachinePoolV1Beta2Status{}) {
			in.V1Beta2 = nil
		}
	}
}

func spokeControlPlaneTopology(in *ControlPlaneTopology, c randfill.Continue) {
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

func spokeMachineDeploymentTopology(in *MachineDeploymentTopology, c randfill.Continue) {
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

func spokeMachinePoolTopology(in *MachinePoolTopology, c randfill.Continue) {
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

func spokeMachineHealthCheckClass(in *MachineHealthCheckClass, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.NodeStartupTimeout != nil {
		in.NodeStartupTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
}

func spokeControlPlaneClass(in *ControlPlaneClass, c randfill.Continue) {
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

func spokeMachineDeploymentClass(in *MachineDeploymentClass, c randfill.Continue) {
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

func spokeMachinePoolClass(in *MachinePoolClass, c randfill.Continue) {
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
