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
)

func TestInfrastructureCluster(t *testing.T) {
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
