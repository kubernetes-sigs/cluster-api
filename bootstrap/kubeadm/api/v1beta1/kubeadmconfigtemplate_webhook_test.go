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

package v1beta1_test

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
)

func TestKubeadmConfigTemplateValidation(t *testing.T) {
	cases := map[string]struct {
		in *bootstrapv1.KubeadmConfigTemplate
	}{
		"valid configuration": {
			in: &bootstrapv1.KubeadmConfigTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: bootstrapv1.KubeadmConfigTemplateSpec{
					Template: bootstrapv1.KubeadmConfigTemplateResource{
						Spec: bootstrapv1.KubeadmConfigSpec{},
					},
				},
			},
		},
	}

	for name, tt := range cases {
		tt := tt

		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(tt.in.ValidateCreate()).To(Succeed())
			g.Expect(tt.in.ValidateUpdate(nil)).To(Succeed())
		})
	}
}
