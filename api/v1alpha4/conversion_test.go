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
	"testing"

	"sigs.k8s.io/cluster-api/api/v1beta1"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func TestFuzzyConversion(t *testing.T) {
	t.Run("for Cluster", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &v1beta1.Cluster{},
		Spoke: &Cluster{},
	}))
	t.Run("for ClusterClass", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &v1beta1.ClusterClass{},
		Spoke: &ClusterClass{},
	}))

	t.Run("for Machine", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &v1beta1.Machine{},
		Spoke: &Machine{},
	}))

	t.Run("for MachineSet", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &v1beta1.MachineSet{},
		Spoke: &MachineSet{},
	}))

	t.Run("for MachineDeployment", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &v1beta1.MachineDeployment{},
		Spoke: &MachineDeployment{},
	}))

	t.Run("for MachineHealthCheck", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &v1beta1.MachineHealthCheck{},
		Spoke: &MachineHealthCheck{},
	}))
}
