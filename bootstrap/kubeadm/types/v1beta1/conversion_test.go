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

package v1beta1

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha4"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func TestFuzzyConversion(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha4.AddToScheme(scheme)).To(Succeed())

	t.Run("for ClusterConfiguration", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: scheme,
		Hub:    &v1alpha4.ClusterConfiguration{},
		Spoke:  &ClusterConfiguration{},
		// NOTE: Kubeadm types does not have ObjectMeta, so we are required to skip data annotation cleanup in the spoke-hub-spoke round trip test.
		SkipSpokeAnnotationCleanup: true,
	}))
	t.Run("for ClusterStatus", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: scheme,
		Hub:    &v1alpha4.ClusterStatus{},
		Spoke:  &ClusterStatus{},
		// NOTE: Kubeadm types does not have ObjectMeta, so we are required to skip data annotation cleanup in the spoke-hub-spoke round trip test.
		SkipSpokeAnnotationCleanup: true,
	}))
	t.Run("for InitConfiguration", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: scheme,
		Hub:    &v1alpha4.InitConfiguration{},
		Spoke:  &InitConfiguration{},
		// NOTE: Kubeadm types does not have ObjectMeta, so we are required to skip data annotation cleanup in the spoke-hub-spoke round trip test.
		SkipSpokeAnnotationCleanup: true,
	}))
	t.Run("for JoinConfiguration", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: scheme,
		Hub:    &v1alpha4.JoinConfiguration{},
		Spoke:  &JoinConfiguration{},
		// NOTE: Kubeadm types does not have ObjectMeta, so we are required to skip data annotation cleanup in the spoke-hub-spoke round trip test.
		SkipSpokeAnnotationCleanup: true,
	}))
}
