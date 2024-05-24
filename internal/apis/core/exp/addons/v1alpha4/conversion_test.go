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

	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func TestFuzzyConversion(t *testing.T) {
	t.Run("for ClusterResourceSet", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &addonsv1.ClusterResourceSet{},
		Spoke: &ClusterResourceSet{},
	}))
	t.Run("for ClusterResourceSetBinding", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &addonsv1.ClusterResourceSetBinding{},
		Spoke: &ClusterResourceSetBinding{},
	}))
}
