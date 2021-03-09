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
	"testing"

	fuzz "github.com/google/gofuzz"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	kubeadmv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha4"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func TestFuzzyConversion(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(AddToScheme(scheme)).To(Succeed())
	g.Expect(v1alpha4.AddToScheme(scheme)).To(Succeed())

	t.Run("for KubeadmControlPLane", utilconversion.FuzzTestFunc(
		scheme, &v1alpha4.KubeadmControlPlane{}, &KubeadmControlPlane{},
		func(codecs runtimeserializer.CodecFactory) []interface{} {
			return []interface{}{
				// This custom function is needed when ConvertTo/ConvertFrom functions
				// uses the json package to unmarshal the bootstrap token string.
				//
				// The Kubeadm v1beta1.BootstrapTokenString type ships with a custom
				// json string representation, in particular it supplies a customized
				// UnmarshalJSON function that can return an error if the string
				// isn't in the correct form.
				//
				// This function effectively disables any fuzzing for the token by setting
				// the values for ID and Secret to working alphanumeric values.
				func(in *kubeadmv1.BootstrapTokenString, c fuzz.Continue) {
					in.ID = "abcdef"
					in.Secret = "abcdef0123456789"
				},
			}
		},
	))
}
