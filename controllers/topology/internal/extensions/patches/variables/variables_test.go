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

package variables

import (
	"testing"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/builder"
)

func TestGlobal(t *testing.T) {
	tests := []struct {
		name            string
		clusterTopology *clusterv1.Topology
		cluster         *clusterv1.Cluster
		want            VariableMap
	}{
		{
			name: "Should calculate global variables",
			clusterTopology: &clusterv1.Topology{
				Variables: []clusterv1.ClusterVariable{
					{
						Name:  "location",
						Value: toJSON("\"us-central\""),
					},
					{
						Name:  "cpu",
						Value: toJSON("8"),
					},
					{
						// This is blocked by a webhook, but let's make sure we overwrite
						// the user-defined variable with the builtin variable anyway.
						Name:  "builtin.cluster.name",
						Value: toJSON("8"),
					},
				},
			},
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster1",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: clusterv1.ClusterSpec{
					Topology: &clusterv1.Topology{
						Class:   "clusterClass1",
						Version: "v1.21.1",
					},
				},
			},
			want: VariableMap{
				"location":                    toJSON("\"us-central\""),
				"cpu":                         toJSON("8"),
				BuiltinClusterName:            toJSON("\"cluster1\""),
				BuiltinClusterNamespace:       toJSON("\"default\""),
				BuiltinClusterTopologyVersion: toJSON("\"v1.21.1\""),
				BuiltinClusterTopologyClass:   toJSON("\"clusterClass1\""),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := Global(tt.clusterTopology, tt.cluster)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestControlPlane(t *testing.T) {
	tests := []struct {
		name                 string
		controlPlaneTopology *clusterv1.ControlPlaneTopology
		controlPlane         *unstructured.Unstructured
		want                 VariableMap
	}{
		{
			name: "Should calculate ControlPlane variables",
			controlPlaneTopology: &clusterv1.ControlPlaneTopology{
				Replicas: pointer.Int32(3),
			},
			controlPlane: builder.ControlPlane(metav1.NamespaceDefault, "controlPlane1").
				WithReplicas(3).
				WithVersion("v1.21.1").
				Build(),
			want: VariableMap{
				BuiltinControlPlaneReplicas: toJSON("3"),
				BuiltinControlPlaneVersion:  toJSON("\"v1.21.1\""),
			},
		},
		{
			name:                 "Should calculate ControlPlane variables, replicas not set",
			controlPlaneTopology: &clusterv1.ControlPlaneTopology{},
			controlPlane: builder.ControlPlane(metav1.NamespaceDefault, "controlPlane1").
				WithVersion("v1.21.1").
				Build(),
			want: VariableMap{
				BuiltinControlPlaneVersion: toJSON("\"v1.21.1\""),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := ControlPlane(tt.controlPlaneTopology, tt.controlPlane)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestMachineDeployment(t *testing.T) {
	tests := []struct {
		name       string
		mdTopology *clusterv1.MachineDeploymentTopology
		md         *clusterv1.MachineDeployment
		want       VariableMap
	}{
		{
			name: "Should calculate MachineDeployment variables",
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Replicas: pointer.Int32(3),
				Name:     "md-topology",
				Class:    "md-class",
			},
			md: builder.MachineDeployment(metav1.NamespaceDefault, "md1").
				WithReplicas(3).
				WithVersion("v1.21.1").
				Build(),
			want: VariableMap{
				BuiltinMachineDeploymentReplicas:     toJSON("3"),
				BuiltinMachineDeploymentVersion:      toJSON("\"v1.21.1\""),
				BuiltinMachineDeploymentClass:        toJSON("\"md-class\""),
				BuiltinMachineDeploymentName:         toJSON("\"md1\""),
				BuiltinMachineDeploymentTopologyName: toJSON("\"md-topology\""),
			},
		},
		{
			name: "Should calculate MachineDeployment variables, replicas not set",
			mdTopology: &clusterv1.MachineDeploymentTopology{
				Name:  "md-topology",
				Class: "md-class",
			},
			md: builder.MachineDeployment(metav1.NamespaceDefault, "md1").
				WithVersion("v1.21.1").
				Build(),
			want: VariableMap{
				BuiltinMachineDeploymentVersion:      toJSON("\"v1.21.1\""),
				BuiltinMachineDeploymentClass:        toJSON("\"md-class\""),
				BuiltinMachineDeploymentName:         toJSON("\"md1\""),
				BuiltinMachineDeploymentTopologyName: toJSON("\"md-topology\""),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := MachineDeployment(tt.mdTopology, tt.md)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func toJSON(value string) apiextensionsv1.JSON {
	return apiextensionsv1.JSON{Raw: []byte(value)}
}
