/*
Copyright 2019 The Kubernetes Authors.

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

package v1alpha2

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/cluster-api/api/v1alpha3"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func TestConvertCluster(t *testing.T) {
	g := NewWithT(t)

	t.Run("to hub", func(t *testing.T) {
		t.Run("should convert the first value in Status.APIEndpoints to Spec.ControlPlaneEndpoint", func(t *testing.T) {
			src := &Cluster{
				Status: ClusterStatus{
					APIEndpoints: []APIEndpoint{
						{
							Host: "example.com",
							Port: 6443,
						},
					},
				},
			}
			dst := &v1alpha3.Cluster{}

			g.Expect(src.ConvertTo(dst)).To(Succeed())
			g.Expect(dst.Spec.ControlPlaneEndpoint.Host).To(Equal("example.com"))
			g.Expect(dst.Spec.ControlPlaneEndpoint.Port).To(BeEquivalentTo(6443))
		})
	})

	t.Run("from hub", func(t *testing.T) {
		t.Run("should convert Spec.ControlPlaneEndpoint to Status.APIEndpoints[0]", func(t *testing.T) {
			src := &v1alpha3.Cluster{
				Spec: v1alpha3.ClusterSpec{
					ControlPlaneEndpoint: v1alpha3.APIEndpoint{
						Host: "example.com",
						Port: 6443,
					},
				},
			}
			dst := &Cluster{}

			g.Expect(dst.ConvertFrom(src)).To(Succeed())
			g.Expect(dst.Status.APIEndpoints[0].Host).To(Equal("example.com"))
			g.Expect(dst.Status.APIEndpoints[0].Port).To(BeEquivalentTo(6443))
		})
	})
}

func TestConvertMachine(t *testing.T) {
	g := NewWithT(t)

	t.Run("from hub", func(t *testing.T) {
		t.Run("preserves Spec.Bootstrap.DataSecretName", func(t *testing.T) {
			src := &v1alpha3.Machine{
				Spec: v1alpha3.MachineSpec{
					Bootstrap: v1alpha3.Bootstrap{
						DataSecretName: pointer.StringPtr("secret-data"),
					},
				},
			}
			dst := &Machine{}

			g.Expect(dst.ConvertFrom(src)).To(Succeed())
			g.Expect(dst.GetAnnotations()[utilconversion.DataAnnotation]).To(ContainSubstring("secret-data"))
		})
	})

}
