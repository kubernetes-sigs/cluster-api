//go:build !race

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

	addonsv1beta1 "sigs.k8s.io/cluster-api/api/addons/v1beta1"
	addonsv1 "sigs.k8s.io/cluster-api/api/addons/v1beta2"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ipamv1alpha1 "sigs.k8s.io/cluster-api/api/ipam/v1alpha1"
	ipamv1beta1 "sigs.k8s.io/cluster-api/api/ipam/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	runtimev1alpha1 "sigs.k8s.io/cluster-api/api/runtime/v1alpha1"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	conversionutil "sigs.k8s.io/cluster-api/util/conversion"
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

	t.Run("for Cluster (v1beta1)", conversionutil.SpokeConverterFuzzTestFunc(
		conversionutil.SpokeConverterFuzzTestFuncInput[*clusterv1.Cluster, *clusterv1beta1.Cluster]{
			ConvertSpokeToHubFunc: ConvertClusterV1Beta1ToHub,
			ConvertHubToSpokeFunc: ConvertClusterHubToV1Beta1,
			FuzzerFuncs:           []fuzzer.FuzzerFuncs{ClusterFuzzFuncs},
		}),
	)
	t.Run("for ClusterClass (v1beta1)", conversionutil.SpokeConverterFuzzTestFunc(
		conversionutil.SpokeConverterFuzzTestFuncInput[*clusterv1.ClusterClass, *clusterv1beta1.ClusterClass]{
			ConvertSpokeToHubFunc: ConvertClusterClassV1Beta1ToHub,
			ConvertHubToSpokeFunc: ConvertClusterClassHubToV1Beta1,
			FuzzerFuncs:           []fuzzer.FuzzerFuncs{ClusterClassFuzzFuncs},
		}),
	)
	t.Run("for Machine (v1beta1)", conversionutil.SpokeConverterFuzzTestFunc(
		conversionutil.SpokeConverterFuzzTestFuncInput[*clusterv1.Machine, *clusterv1beta1.Machine]{
			ConvertSpokeToHubFunc: ConvertMachineV1Beta1ToHub,
			ConvertHubToSpokeFunc: ConvertMachineHubToV1Beta1,
			FuzzerFuncs:           []fuzzer.FuzzerFuncs{MachineFuzzFuncs},
		}),
	)
	t.Run("for MachineSet (v1beta1)", conversionutil.SpokeConverterFuzzTestFunc(
		conversionutil.SpokeConverterFuzzTestFuncInput[*clusterv1.MachineSet, *clusterv1beta1.MachineSet]{
			ConvertSpokeToHubFunc: ConvertMachineSetV1Beta1ToHub,
			ConvertHubToSpokeFunc: ConvertMachineSetHubToV1Beta1,
			FuzzerFuncs:           []fuzzer.FuzzerFuncs{MachineSetFuzzFuncs},
		}),
	)
	t.Run("for MachineDeployment (v1beta1)", conversionutil.SpokeConverterFuzzTestFunc(
		conversionutil.SpokeConverterFuzzTestFuncInput[*clusterv1.MachineDeployment, *clusterv1beta1.MachineDeployment]{
			ConvertSpokeToHubFunc: ConvertMachineDeploymentV1Beta1ToHub,
			ConvertHubToSpokeFunc: ConvertMachineDeploymentHubToV1Beta1,
			FuzzerFuncs:           []fuzzer.FuzzerFuncs{MachineDeploymentFuzzFuncs},
		}),
	)
	t.Run("for MachineHealthCheck (v1beta1)", conversionutil.SpokeConverterFuzzTestFunc(
		conversionutil.SpokeConverterFuzzTestFuncInput[*clusterv1.MachineHealthCheck, *clusterv1beta1.MachineHealthCheck]{
			ConvertSpokeToHubFunc: ConvertMachineHealthCheckV1Beta1ToHub,
			ConvertHubToSpokeFunc: ConvertMachineHealthCheckHubToV1Beta1,
			FuzzerFuncs:           []fuzzer.FuzzerFuncs{MachineHealthCheckFuzzFuncs},
		}),
	)
	t.Run("for MachinePool (v1beta1)", conversionutil.SpokeConverterFuzzTestFunc(
		conversionutil.SpokeConverterFuzzTestFuncInput[*clusterv1.MachinePool, *clusterv1beta1.MachinePool]{
			ConvertSpokeToHubFunc: ConvertMachinePoolV1Beta1ToHub,
			ConvertHubToSpokeFunc: ConvertMachinePoolHubToV1Beta1,
			FuzzerFuncs:           []fuzzer.FuzzerFuncs{MachinePoolFuzzFuncs},
		}),
	)
	t.Run("for MachineDrainRule (v1beta1)", conversionutil.SpokeConverterFuzzTestFunc(
		conversionutil.SpokeConverterFuzzTestFuncInput[*clusterv1.MachineDrainRule, *clusterv1beta1.MachineDrainRule]{
			ConvertSpokeToHubFunc: ConvertMachineDrainRuleV1Beta1ToHub,
			ConvertHubToSpokeFunc: ConvertMachineDrainRuleHubToV1Beta1,
			FuzzerFuncs:           []fuzzer.FuzzerFuncs{},
		}),
	)
	t.Run("for ClusterResourceSet (v1beta1)", conversionutil.SpokeConverterFuzzTestFunc(
		conversionutil.SpokeConverterFuzzTestFuncInput[*addonsv1.ClusterResourceSet, *addonsv1beta1.ClusterResourceSet]{
			ConvertSpokeToHubFunc: ConvertClusterResourceSetV1Beta1ToHub,
			ConvertHubToSpokeFunc: ConvertClusterResourceSetHubToV1Beta1,
			FuzzerFuncs:           []fuzzer.FuzzerFuncs{ClusterResourceSetFuzzFuncs},
		}),
	)
	t.Run("for ClusterResourceSetBinding (v1beta1)", conversionutil.SpokeConverterFuzzTestFunc(
		conversionutil.SpokeConverterFuzzTestFuncInput[*addonsv1.ClusterResourceSetBinding, *addonsv1beta1.ClusterResourceSetBinding]{
			ConvertSpokeToHubFunc: ConvertClusterResourceSetBindingV1Beta1ToHub,
			ConvertHubToSpokeFunc: ConvertClusterResourceSetBindingHubToV1Beta1,
			FuzzerFuncs:           []fuzzer.FuzzerFuncs{ClusterResourceSetBindingFuzzFuncs},
		}),
	)
	t.Run("for IPAddress (v1alpha1)", conversionutil.SpokeConverterFuzzTestFunc(
		conversionutil.SpokeConverterFuzzTestFuncInput[*ipamv1.IPAddress, *ipamv1alpha1.IPAddress]{
			ConvertSpokeToHubFunc: ConvertIPAddressV1Alpha1ToHub,
			ConvertHubToSpokeFunc: ConvertIPAddressHubToV1Alpha1,
			FuzzerFuncs:           []fuzzer.FuzzerFuncs{IPAddressFuzzFuncs},
		}),
	)
	t.Run("for IPAddress (v1beta1)", conversionutil.SpokeConverterFuzzTestFunc(
		conversionutil.SpokeConverterFuzzTestFuncInput[*ipamv1.IPAddress, *ipamv1beta1.IPAddress]{
			ConvertSpokeToHubFunc: ConvertIPAddressV1Beta1ToHub,
			ConvertHubToSpokeFunc: ConvertIPAddressHubToV1Beta1,
			FuzzerFuncs:           []fuzzer.FuzzerFuncs{IPAddressFuzzFuncs},
		}),
	)
	t.Run("for IPAddressClaim (v1alpha1)", conversionutil.SpokeConverterFuzzTestFunc(
		conversionutil.SpokeConverterFuzzTestFuncInput[*ipamv1.IPAddressClaim, *ipamv1alpha1.IPAddressClaim]{
			ConvertSpokeToHubFunc: ConvertIPAddressClaimV1Alpha1ToHub,
			ConvertHubToSpokeFunc: ConvertIPAddressClaimHubToV1Alpha1,
			FuzzerFuncs:           []fuzzer.FuzzerFuncs{IPAddressClaimFuzzFuncs},
		}),
	)
	t.Run("for IPAddressClaim (v1beta1)", conversionutil.SpokeConverterFuzzTestFunc(
		conversionutil.SpokeConverterFuzzTestFuncInput[*ipamv1.IPAddressClaim, *ipamv1beta1.IPAddressClaim]{
			ConvertSpokeToHubFunc: ConvertIPAddressClaimV1Beta1ToHub,
			ConvertHubToSpokeFunc: ConvertIPAddressClaimHubToV1Beta1,
			FuzzerFuncs:           []fuzzer.FuzzerFuncs{IPAddressClaimFuzzFuncs},
		}),
	)
	t.Run("for ExtensionConfig (v1alpha1)", conversionutil.SpokeConverterFuzzTestFunc(
		conversionutil.SpokeConverterFuzzTestFuncInput[*runtimev1.ExtensionConfig, *runtimev1alpha1.ExtensionConfig]{
			ConvertSpokeToHubFunc: ConvertExtensionConfigV1Alpha1ToHub,
			ConvertHubToSpokeFunc: ConvertExtensionConfigHubToV1Alpha1,
			FuzzerFuncs:           []fuzzer.FuzzerFuncs{ExtensionConfigFuzzFuncs},
		}),
	)
}

func ClusterFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubClusterSpec,
		hubClusterStatus,
		hubClusterVariable,
		hubFailureDomain,
		hubUnhealthyNodeCondition,
		hubUnhealthyMachineCondition,
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
		spokeUnhealthyMachineCondition,
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

func hubUnhealthyMachineCondition(in *clusterv1.UnhealthyMachineCondition, c randfill.Continue) {
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

func ClusterClassFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubClusterClassVariable,
		hubClusterClassStatusVariableDefinition,
		hubClusterClassStatus,
		hubJSONPatch,
		hubJSONSchemaProps,
		hubUnhealthyNodeCondition,
		hubUnhealthyMachineCondition,
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
		spokeUnhealthyMachineCondition,
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

	in.Phase = []string{"Updating", "Running"}[c.Intn(2)]

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
		if reflect.DeepEqual(in.Deletion.WaitForPreDrainHookStartTime, &metav1.Time{}) {
			in.Deletion.WaitForPreDrainHookStartTime = nil
		}
		if reflect.DeepEqual(in.Deletion.WaitForPreTerminateHookStartTime, &metav1.Time{}) {
			in.Deletion.WaitForPreTerminateHookStartTime = nil
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
		hubUnhealthyMachineCondition,
		hubMachineHealthCheckStatus,
		spokeMachineHealthCheck,
		spokeMachineHealthCheckSpec,
		spokeObjectReference,
		spokeMachineHealthCheckStatus,
		spokeUnhealthyCondition,
		spokeUnhealthyMachineCondition,
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

func spokeUnhealthyMachineCondition(in *clusterv1beta1.UnhealthyMachineCondition, c randfill.Continue) {
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

func ClusterResourceSetFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubClusterResourceSetStatus,
		spokeClusterResourceSetStatus,
	}
}

func hubClusterResourceSetStatus(in *addonsv1.ClusterResourceSetStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.Deprecated != nil {
		if in.Deprecated.V1Beta1 == nil || reflect.DeepEqual(in.Deprecated.V1Beta1, &addonsv1.ClusterResourceSetV1Beta1DeprecatedStatus{}) {
			in.Deprecated = nil
		}
	}
}

func spokeClusterResourceSetStatus(in *addonsv1beta1.ClusterResourceSetStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &addonsv1beta1.ClusterResourceSetV1Beta2Status{}) {
			in.V1Beta2 = nil
		}
	}
}

func ClusterResourceSetBindingFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubResourceBinding,
		hubClusterResourceSetStatus,
		spokeClusterResourceSetBindingSpec,
	}
}

func hubResourceBinding(in *addonsv1.ResourceBinding, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.Applied == nil {
		in.Applied = ptr.To(false) // Applied is a required field and nil does not round trip
	}
}

func spokeClusterResourceSetBindingSpec(in *addonsv1beta1.ClusterResourceSetBindingSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	for i, b := range in.Bindings {
		if b != nil && reflect.DeepEqual(*b, addonsv1beta1.ResourceSetBinding{}) {
			in.Bindings[i] = nil
		}
		if in.Bindings[i] != nil {
			for j, r := range in.Bindings[i].Resources {
				if reflect.DeepEqual(r.LastAppliedTime, &metav1.Time{}) {
					in.Bindings[i].Resources[j].LastAppliedTime = nil
				}
			}
		}
	}
}

func IPAddressFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubIPAddressSpec,
		spokeTypedLocalObjectReference,
	}
}

func hubIPAddressSpec(in *ipamv1.IPAddressSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.Prefix == nil {
		in.Prefix = ptr.To(int32(0)) // Prefix is a required field and nil does not round trip
	}
}

func spokeTypedLocalObjectReference(in *corev1.TypedLocalObjectReference, c randfill.Continue) {
	c.FillNoCustom(in)
	if in.APIGroup != nil && *in.APIGroup == "" {
		in.APIGroup = nil
	}
}

func IPAddressClaimFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubIPAddressClaimStatus,
		spokeIPAddressClaimStatus,
		spokeTypedLocalObjectReference,
	}
}

func hubIPAddressClaimStatus(in *ipamv1.IPAddressClaimStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.Deprecated != nil {
		if in.Deprecated.V1Beta1 == nil || reflect.DeepEqual(in.Deprecated.V1Beta1, &ipamv1.IPAddressClaimV1Beta1DeprecatedStatus{}) {
			in.Deprecated = nil
		}
	}
}

func spokeIPAddressClaimStatus(in *ipamv1beta1.IPAddressClaimStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &ipamv1beta1.IPAddressClaimV1Beta2Status{}) {
			in.V1Beta2 = nil
		}
	}
}

func ExtensionConfigFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubExtensionConfigStatus,
		spokeExtensionConfig,
		spokeExtensionConfigStatus,
	}
}

func hubExtensionConfigStatus(in *runtimev1.ExtensionConfigStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.Deprecated != nil {
		if in.Deprecated.V1Beta1 == nil || reflect.DeepEqual(in.Deprecated.V1Beta1, &runtimev1.ExtensionConfigV1Beta1DeprecatedStatus{}) {
			in.Deprecated = nil
		}
	}
}

func spokeExtensionConfig(in *runtimev1alpha1.ExtensionConfig, c randfill.Continue) {
	c.FillNoCustom(in)

	dropEmptyStringsExtensionConfig(in)

	if in.Spec.ClientConfig.Service != nil && reflect.DeepEqual(in.Spec.ClientConfig.Service, &runtimev1alpha1.ServiceReference{}) {
		in.Spec.ClientConfig.Service = nil
	}
}

func spokeExtensionConfigStatus(in *runtimev1alpha1.ExtensionConfigStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &runtimev1alpha1.ExtensionConfigV1Beta2Status{}) {
			in.V1Beta2 = nil
		}
	}

	for i, h := range in.Handlers {
		if h.FailurePolicy != nil && *h.FailurePolicy == "" {
			// &"" Is not a valid value for FailurePolicy as the enum validation enforces an enum value if FailurePolicy is set.
			h.FailurePolicy = nil
		}
		in.Handlers[i] = h
	}
}
