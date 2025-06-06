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

package contract

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestInfrastructureCluster(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}

	t.Run("Manages spec.ControlPlaneEndpoint", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(InfrastructureCluster().ControlPlaneEndpoint().Path()).To(Equal(Path{"spec", "controlPlaneEndpoint"}))

		endpoint := clusterv1.APIEndpoint{
			Host: "example.com",
			Port: 1234,
		}

		err := InfrastructureCluster().ControlPlaneEndpoint().Set(obj, endpoint)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := InfrastructureCluster().ControlPlaneEndpoint().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal(endpoint))
	})
	t.Run("Manages status.initialization.provisioned", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(InfrastructureCluster().Provisioned("v1beta2").Path()).To(Equal(Path{"status", "initialization", "provisioned"}))

		err := InfrastructureCluster().Provisioned("v1beta2").Set(obj, true)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := InfrastructureCluster().Provisioned("v1beta2").Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(BeTrue())

		g.Expect(InfrastructureCluster().Provisioned("v1beta1").Path()).To(Equal(Path{"status", "ready"}))

		objV1beta1 := &unstructured.Unstructured{Object: map[string]interface{}{}}
		err = InfrastructureCluster().Provisioned("v1beta1").Set(objV1beta1, true)
		g.Expect(err).ToNot(HaveOccurred())

		got, err = InfrastructureCluster().Provisioned("v1beta1").Get(objV1beta1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(BeTrue())
	})
	t.Run("Manages optional status.failureReason", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(InfrastructureCluster().FailureReason().Path()).To(Equal(Path{"status", "failureReason"}))

		err := InfrastructureCluster().FailureReason().Set(obj, "fake-reason")
		g.Expect(err).ToNot(HaveOccurred())

		got, err := InfrastructureCluster().FailureReason().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal("fake-reason"))
	})
	t.Run("Manages optional status.failureMessage", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(InfrastructureCluster().FailureMessage().Path()).To(Equal(Path{"status", "failureMessage"}))

		err := InfrastructureCluster().FailureMessage().Set(obj, "fake-message")
		g.Expect(err).ToNot(HaveOccurred())

		got, err := InfrastructureCluster().FailureMessage().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(Equal("fake-message"))
	})
	t.Run("Manages optional status.failureDomains", func(t *testing.T) {
		g := NewWithT(t)

		failureDomains := []clusterv1.FailureDomain{
			{
				Name:         "domain1",
				ControlPlane: true,
				Attributes: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
			{
				Name:         "domain2",
				ControlPlane: false,
				Attributes: map[string]string{
					"key3": "value3",
					"key4": "value4",
				},
			},
		}
		g.Expect(InfrastructureCluster().FailureDomains("v1beta2").Path()).To(Equal(Path{"status", "failureDomains"}))

		err := InfrastructureCluster().FailureDomains("v1beta2").Set(obj, failureDomains)
		g.Expect(err).ToNot(HaveOccurred())

		got, err := InfrastructureCluster().FailureDomains("v1beta2").Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got).To(BeComparableTo(failureDomains))

		infraClusterV1Beta1 := &unstructured.Unstructured{Object: map[string]interface{}{
			"kind":       "GenericInfrastructureCluster",
			"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
			"status": map[string]interface{}{
				"failureDomains": map[string]interface{}{
					"oldDomain": map[string]interface{}{
						"controlPlane": true,
						"attributes": map[string]interface{}{
							"attribute1": "value1",
						},
					},
					"newdomain": map[string]interface{}{
						"controlPlane": true,
						"attributes": map[string]interface{}{
							"attribute2": "value2",
						},
					},
				},
			},
		}}
		got, err = InfrastructureCluster().FailureDomains("v1beta1").Get(infraClusterV1Beta1)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		// Note: Also validates the failureDomains are sorted.
		g.Expect(got).To(BeComparableTo([]clusterv1.FailureDomain{
			{
				Name:         "newdomain",
				ControlPlane: true,
				Attributes: map[string]string{
					"attribute2": "value2",
				},
			},
			{
				Name:         "oldDomain",
				ControlPlane: true,
				Attributes: map[string]string{
					"attribute1": "value1",
				},
			},
		}))

		infraClusterV1Beta2 := &unstructured.Unstructured{Object: map[string]interface{}{
			"kind":       "GenericInfrastructureCluster",
			"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
			"status": map[string]interface{}{
				"failureDomains": []interface{}{
					map[string]interface{}{
						"name":         "newdomain",
						"controlPlane": true,
						"attributes": map[string]interface{}{
							"attribute1": "value1",
						},
					},
				},
			},
		}}
		got, err = InfrastructureCluster().FailureDomains("v1beta2").Get(infraClusterV1Beta2)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got).To(BeComparableTo([]clusterv1.FailureDomain{
			{
				Name:         "newdomain",
				ControlPlane: true,
				Attributes: map[string]string{
					"attribute1": "value1",
				},
			},
		}))
	})
}

func TestInfrastructureClusterIgnorePaths(t *testing.T) {
	tests := []struct {
		name                  string
		infrastructureCluster *unstructured.Unstructured
		want                  []Path
		expectErr             bool
	}{
		{
			name: "No ignore paths when controlPlaneEndpoint is not set",
			infrastructureCluster: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"server": "1.2.3.4",
					},
				},
			},
			want: nil,
		},
		{
			name: "No ignore paths when controlPlaneEndpoint is nil",
			infrastructureCluster: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": nil,
					},
				},
			},
			want: nil,
		},
		{
			name: "No ignore paths when controlPlaneEndpoint is an empty object",
			infrastructureCluster: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{},
					},
				},
			},
			want: nil,
		},
		{
			name: "Don't ignore host when controlPlaneEndpoint.host is set",
			infrastructureCluster: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "example.com",
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "Ignore host when controlPlaneEndpoint.host is set to its zero value",
			infrastructureCluster: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "",
						},
					},
				},
			},
			want: []Path{
				{"spec", "controlPlaneEndpoint", "host"},
			},
		},
		{
			name: "Don't ignore port when controlPlaneEndpoint.port is set",
			infrastructureCluster: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"port": int64(6443),
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "Ignore port when controlPlaneEndpoint.port is set to its zero value",
			infrastructureCluster: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"port": int64(0),
						},
					},
				},
			},
			want: []Path{
				{"spec", "controlPlaneEndpoint", "port"},
			},
		},
		{
			name: "Ignore host and port when controlPlaneEndpoint host and port are set to their zero values",
			infrastructureCluster: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "",
							"port": int64(0),
						},
					},
				},
			},
			want: []Path{
				{"spec", "controlPlaneEndpoint", "host"},
				{"spec", "controlPlaneEndpoint", "port"},
			},
		},
		{
			name: "Ignore host when controlPlaneEndpoint host is to its zero values, even if port is set",
			infrastructureCluster: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "",
							"port": int64(6443),
						},
					},
				},
			},
			want: []Path{
				{"spec", "controlPlaneEndpoint", "host"},
			},
		},
		{
			name: "Ignore port when controlPlaneEndpoint port is to its zero values, even if host is set",
			infrastructureCluster: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "example.com",
							"port": int64(0),
						},
					},
				},
			},
			want: []Path{
				{"spec", "controlPlaneEndpoint", "port"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := InfrastructureCluster().IgnorePaths(tt.infrastructureCluster)

			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}
