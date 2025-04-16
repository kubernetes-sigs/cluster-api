//go:build !race

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
	"reflect"
	"testing"

	fuzz "github.com/google/gofuzz"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

// Test is disabled when the race detector is enabled (via "//go:build !race" above) because otherwise the fuzz tests would just time out.

func TestFuzzyConversion(t *testing.T) {
	t.Run("for Cluster", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:                &clusterv1.Cluster{},
		Spoke:              &Cluster{},
		SpokeAfterMutation: clusterSpokeAfterMutation,
		FuzzerFuncs:        []fuzzer.FuzzerFuncs{ClusterFuncs},
	}))

	t.Run("for Machine", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.Machine{},
		Spoke:       &Machine{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{MachineFuzzFunc},
	}))

	t.Run("for MachineSet", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.MachineSet{},
		Spoke:       &MachineSet{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{MachineSetFuzzFunc},
	}))

	t.Run("for MachineDeployment", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.MachineDeployment{},
		Spoke:       &MachineDeployment{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{MachineDeploymentFuzzFunc},
	}))

	t.Run("for MachineHealthCheck", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &clusterv1.MachineHealthCheck{},
		Spoke:       &MachineHealthCheck{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{MachineHealthCheckFuzzFunc},
	}))
}

func MachineFuzzFunc(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubMachineStatus,
		spokeMachineStatus,
		spokeBootstrap,
	}
}

func hubMachineStatus(in *clusterv1.MachineStatus, c fuzz.Continue) {
	c.FuzzNoCustom(in)
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

func spokeMachineStatus(in *MachineStatus, c fuzz.Continue) {
	c.FuzzNoCustom(in)

	// These fields have been removed in v1beta1
	// data is going to be lost, so we're forcing zero values to avoid round trip errors.
	in.Version = nil
}

func MachineSetFuzzFunc(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubMachineSetStatus,
		spokeObjectMeta,
		spokeBootstrap,
	}
}

func hubMachineSetStatus(in *clusterv1.MachineSetStatus, c fuzz.Continue) {
	c.FuzzNoCustom(in)
	// Always create struct with at least one mandatory fields.
	if in.Deprecated == nil {
		in.Deprecated = &clusterv1.MachineSetDeprecatedStatus{}
	}
	if in.Deprecated.V1Beta1 == nil {
		in.Deprecated.V1Beta1 = &clusterv1.MachineSetV1Beta1DeprecatedStatus{}
	}
}

func MachineDeploymentFuzzFunc(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubMachineDeploymentStatus,
		spokeObjectMeta,
		spokeBootstrap,
	}
}

func hubMachineDeploymentStatus(in *clusterv1.MachineDeploymentStatus, c fuzz.Continue) {
	c.FuzzNoCustom(in)
	// Always create struct with at least one mandatory fields.
	if in.Deprecated == nil {
		in.Deprecated = &clusterv1.MachineDeploymentDeprecatedStatus{}
	}
	if in.Deprecated.V1Beta1 == nil {
		in.Deprecated.V1Beta1 = &clusterv1.MachineDeploymentV1Beta1DeprecatedStatus{}
	}
}

func spokeObjectMeta(in *ObjectMeta, c fuzz.Continue) {
	c.FuzzNoCustom(in)

	// These fields have been removed in v1alpha4
	// data is going to be lost, so we're forcing zero values here.
	in.Name = ""
	in.GenerateName = ""
	in.Namespace = ""
	in.OwnerReferences = nil
}

func spokeBootstrap(obj *Bootstrap, c fuzz.Continue) {
	c.FuzzNoCustom(obj)

	// Bootstrap.Data has been removed in v1alpha4, so setting it to nil in order to avoid v1alpha3 --> <hub> --> v1alpha3 round trip errors.
	obj.Data = nil
}

// clusterSpokeAfterMutation modifies the spoke version of the Cluster such that it can pass an equality test in the
// spoke-hub-spoke conversion scenario.
func clusterSpokeAfterMutation(c conversion.Convertible) {
	cluster := c.(*Cluster)

	// Create a temporary 0-length slice using the same underlying array as cluster.Status.Conditions to avoid
	// allocations.
	tmp := cluster.Status.Conditions[:0]

	for i := range cluster.Status.Conditions {
		condition := cluster.Status.Conditions[i]

		// Keep everything that is not ControlPlaneInitializedCondition
		if condition.Type != ConditionType(clusterv1.ControlPlaneInitializedV1Beta1Condition) {
			tmp = append(tmp, condition)
		}
	}

	// Point cluster.Status.Conditions and our slice that does not have ControlPlaneInitializedCondition
	cluster.Status.Conditions = tmp
}

func ClusterFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubClusterStatus,
		hubClusterVariable,
	}
}

func hubClusterStatus(in *clusterv1.ClusterStatus, c fuzz.Continue) {
	c.FuzzNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.Deprecated != nil {
		if in.Deprecated.V1Beta1 == nil || reflect.DeepEqual(in.Deprecated.V1Beta1, &clusterv1.ClusterV1Beta1DeprecatedStatus{}) {
			in.Deprecated = nil
		}
	}
}

func hubClusterVariable(in *clusterv1.ClusterVariable, c fuzz.Continue) {
	c.FuzzNoCustom(in)

	// Not every random byte array is valid JSON, e.g. a string without `""`,so we're setting a valid value.
	in.Value = apiextensionsv1.JSON{Raw: []byte("\"test-string\"")}
}

func MachineHealthCheckFuzzFunc(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubMachineHealthCheckStatus,
	}
}

func hubMachineHealthCheckStatus(in *clusterv1.MachineHealthCheckStatus, c fuzz.Continue) {
	c.FuzzNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.Deprecated != nil {
		if in.Deprecated.V1Beta1 == nil || reflect.DeepEqual(in.Deprecated.V1Beta1, &clusterv1.MachineHealthCheckV1Beta1DeprecatedStatus{}) {
			in.Deprecated = nil
		}
	}
}
