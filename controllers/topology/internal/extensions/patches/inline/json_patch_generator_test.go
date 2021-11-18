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

package inline

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/extensions/patches/api"
	patchvariables "sigs.k8s.io/cluster-api/controllers/topology/internal/extensions/patches/variables"
)

func TestGenerate(t *testing.T) {
	tests := []struct {
		name    string
		patch   *clusterv1.ClusterClassPatch
		req     *api.GenerateRequest
		want    *api.GenerateResponse
		wantErr bool
	}{
		{
			name: "Should generate JSON Patches with correct variable values",
			patch: &clusterv1.ClusterClassPatch{
				Name: "clusterName",
				Definitions: []clusterv1.PatchDefinition{
					{
						Selector: clusterv1.PatchSelector{
							APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
							Kind:       "ControlPlaneTemplate",
							MatchResources: clusterv1.PatchSelectorMatch{
								ControlPlane: pointer.Bool(true),
							},
						},
						JSONPatches: []clusterv1.JSONPatch{
							// .value
							{
								Op:    "replace",
								Path:  "/spec/value",
								Value: &apiextensionsv1.JSON{Raw: []byte("1")},
							},
							// .valueFrom.variable
							{
								Op:   "replace",
								Path: "/spec/valueFrom/variable",
								ValueFrom: &clusterv1.JSONPatchValue{
									Variable: pointer.String("variableA"),
								},
							},
							// .valueFrom.template
							{
								Op:   "replace",
								Path: "/spec/valueFrom/template",
								ValueFrom: &clusterv1.JSONPatchValue{
									Template: pointer.String(`template {{ .variableB }}`),
								},
							},
							// template-specific variable takes precedent, if the same variable exists
							// in the global and template-specific variables.
							{
								Op:   "replace",
								Path: "/spec/templatePrecedent",
								ValueFrom: &clusterv1.JSONPatchValue{
									Variable: pointer.String("variableC"),
								},
							},
							// global builtin variable should work.
							// (verify that merging builtin variables works)
							{
								Op:   "replace",
								Path: "/spec/builtinClusterName",
								ValueFrom: &clusterv1.JSONPatchValue{
									Variable: pointer.String("builtin.cluster.name"),
								},
							},
							// template-specific builtin variable should work.
							// (verify that merging builtin variables works)
							{
								Op:   "replace",
								Path: "/spec/builtinControlPlaneReplicas",
								ValueFrom: &clusterv1.JSONPatchValue{
									Variable: pointer.String("builtin.controlPlane.replicas"),
								},
							},
						},
					},
				},
			},
			req: &api.GenerateRequest{
				Variables: map[string]apiextensionsv1.JSON{
					"builtin":   {Raw: []byte(`{"cluster":{"name":"cluster-name","namespace":"default","topology":{"class":"clusterClass1","version":"v1.21.1"}}}`)},
					"variableA": {Raw: []byte(`"A"`)},
					"variableB": {Raw: []byte(`"B"`)},
					"variableC": {Raw: []byte(`"C"`)},
				},
				Items: []*api.GenerateRequestTemplate{
					{
						TemplateRef: api.TemplateRef{
							APIVersion:   "controlplane.cluster.x-k8s.io/v1beta1",
							Kind:         "ControlPlaneTemplate",
							TemplateType: api.ControlPlaneTemplateType,
						},
						Variables: map[string]apiextensionsv1.JSON{
							"builtin":   {Raw: []byte(`{"controlPlane":{"replicas":3}}`)},
							"variableC": {Raw: []byte(`"C-template"`)},
						},
					},
				},
			},
			want: &api.GenerateResponse{
				Items: []api.GenerateResponsePatch{
					{
						TemplateRef: api.TemplateRef{
							APIVersion:   "controlplane.cluster.x-k8s.io/v1beta1",
							Kind:         "ControlPlaneTemplate",
							TemplateType: api.ControlPlaneTemplateType,
						},
						Patch: toJSONCompact(`[
{"op":"replace","path":"/spec/value","value":1},
{"op":"replace","path":"/spec/valueFrom/variable","value":"A"},
{"op":"replace","path":"/spec/valueFrom/template","value":"template B"},
{"op":"replace","path":"/spec/templatePrecedent","value":"C-template"},
{"op":"replace","path":"/spec/builtinClusterName","value":"cluster-name"},
{"op":"replace","path":"/spec/builtinControlPlaneReplicas","value":3}
]`),
						PatchType: api.JSONPatchType,
					},
				},
			},
		},
		{
			name: "Should generate JSON Patches (multiple PatchDefinitions)",
			patch: &clusterv1.ClusterClassPatch{
				Name: "clusterName",
				Definitions: []clusterv1.PatchDefinition{
					{
						Selector: clusterv1.PatchSelector{
							APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
							Kind:       "ControlPlaneTemplate",
							MatchResources: clusterv1.PatchSelectorMatch{
								ControlPlane: pointer.Bool(true),
							},
						},
						JSONPatches: []clusterv1.JSONPatch{
							{
								Op:   "replace",
								Path: "/spec/template/spec/kubeadmConfigSpec/clusterConfiguration/controllerManager/extraArgs/cluster-name",
								ValueFrom: &clusterv1.JSONPatchValue{
									Variable: pointer.String("builtin.cluster.name"),
								},
							},
							{
								Op:   "replace",
								Path: "/spec/template/spec/kubeadmConfigSpec/files",
								ValueFrom: &clusterv1.JSONPatchValue{
									Template: pointer.String(`
- contentFrom:
    secret:
      key: control-plane-azure.json
      name: "{{ .builtin.cluster.name }}-control-plane-azure-json"
  owner: root:root
`),
								},
							},
						},
					},
					{
						Selector: clusterv1.PatchSelector{
							APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
							Kind:       "BootstrapTemplate",
							MatchResources: clusterv1.PatchSelectorMatch{
								MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
									Names: []string{"default-worker"},
								},
							},
						},
						JSONPatches: []clusterv1.JSONPatch{
							{
								Op:   "replace",
								Path: "/spec/template/spec/kubeadmConfigSpec/clusterConfiguration/controllerManager/extraArgs/cluster-name",
								ValueFrom: &clusterv1.JSONPatchValue{
									Template: pointer.String(`
[{
	"contentFrom":{
		"secret":{
			"key":"worker-node-azure.json",
			"name":"{{ .builtin.cluster.name }}-md-0-azure-json"
		}
	},
	"owner":"root:root"
}]`),
								},
							},
						},
					},
				},
			},
			req: &api.GenerateRequest{
				Variables: map[string]apiextensionsv1.JSON{
					"builtin": {Raw: []byte(`{"cluster":{"name":"cluster-name","namespace":"default","topology":{"class":"clusterClass1","version":"v1.21.1"}}}`)},
				},
				Items: []*api.GenerateRequestTemplate{
					{
						TemplateRef: api.TemplateRef{
							APIVersion:   "controlplane.cluster.x-k8s.io/v1beta1",
							Kind:         "ControlPlaneTemplate",
							TemplateType: api.ControlPlaneTemplateType,
						},
					},
					{
						TemplateRef: api.TemplateRef{
							APIVersion:   "bootstrap.cluster.x-k8s.io/v1beta1",
							Kind:         "BootstrapTemplate",
							TemplateType: api.MachineDeploymentBootstrapConfigTemplateType,
							MachineDeploymentRef: api.MachineDeploymentRef{
								Class: "default-worker",
							},
						},
					},
				},
			},
			want: &api.GenerateResponse{
				Items: []api.GenerateResponsePatch{
					{
						TemplateRef: api.TemplateRef{
							APIVersion:   "controlplane.cluster.x-k8s.io/v1beta1",
							Kind:         "ControlPlaneTemplate",
							TemplateType: api.ControlPlaneTemplateType,
						},
						Patch: toJSONCompact(`[
{"op":"replace","path":"/spec/template/spec/kubeadmConfigSpec/clusterConfiguration/controllerManager/extraArgs/cluster-name","value":"cluster-name"},
{"op":"replace","path":"/spec/template/spec/kubeadmConfigSpec/files","value":[{"contentFrom":{"secret":{"key":"control-plane-azure.json","name":"cluster-name-control-plane-azure-json"}},"owner":"root:root"}]}
]`),
						PatchType: api.JSONPatchType,
					},
					{
						TemplateRef: api.TemplateRef{
							APIVersion:   "bootstrap.cluster.x-k8s.io/v1beta1",
							Kind:         "BootstrapTemplate",
							TemplateType: api.MachineDeploymentBootstrapConfigTemplateType,
							MachineDeploymentRef: api.MachineDeploymentRef{
								Class: "default-worker",
							},
						},
						Patch: toJSONCompact(`[
{"op":"replace","path":"/spec/template/spec/kubeadmConfigSpec/clusterConfiguration/controllerManager/extraArgs/cluster-name","value":[{"contentFrom":{"secret":{"key":"worker-node-azure.json","name":"cluster-name-md-0-azure-json"}},"owner":"root:root"}]}
]`),
						PatchType: api.JSONPatchType,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := New(tt.patch).Generate(context.Background(), tt.req)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestTemplateMatchesSelector(t *testing.T) {
	tests := []struct {
		name        string
		templateRef *api.TemplateRef
		selector    clusterv1.PatchSelector
		match       bool
	}{
		{
			name: "Don't match: apiVersion mismatch",
			templateRef: &api.TemplateRef{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "AzureMachineTemplate",
			},
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha4",
				Kind:       "AzureMachineTemplate",
			},
			match: false,
		},
		{
			name: "Don't match: kind mismatch",
			templateRef: &api.TemplateRef{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "AzureMachineTemplate",
			},
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "AzureClusterTemplate",
			},
			match: false,
		},
		{
			name: "Match InfrastructureClusterTemplate",
			templateRef: &api.TemplateRef{
				APIVersion:   "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:         "AzureClusterTemplate",
				TemplateType: api.InfrastructureClusterTemplateType,
			},
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "AzureClusterTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					InfrastructureCluster: pointer.Bool(true),
				},
			},
			match: true,
		},
		{
			name: "Don't match InfrastructureClusterTemplate, .matchResources.infrastructureCluster not set",
			templateRef: &api.TemplateRef{
				APIVersion:   "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:         "AzureClusterTemplate",
				TemplateType: api.InfrastructureClusterTemplateType,
			},
			selector: clusterv1.PatchSelector{
				APIVersion:     "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:           "AzureClusterTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{},
			},
			match: false,
		},
		{
			name: "Don't match InfrastructureClusterTemplate, .matchResources.infrastructureCluster false",
			templateRef: &api.TemplateRef{
				APIVersion:   "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:         "AzureClusterTemplate",
				TemplateType: api.InfrastructureClusterTemplateType,
			},
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "AzureClusterTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					InfrastructureCluster: pointer.Bool(false),
				},
			},
			match: false,
		},
		{
			name: "Match ControlPlaneTemplate",
			templateRef: &api.TemplateRef{
				APIVersion:   "controlplane.cluster.x-k8s.io/v1beta1",
				Kind:         "ControlPlaneTemplate",
				TemplateType: api.ControlPlaneTemplateType,
			},
			selector: clusterv1.PatchSelector{
				APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
				Kind:       "ControlPlaneTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					ControlPlane: pointer.Bool(true),
				},
			},
			match: true,
		},
		{
			name: "Don't match ControlPlaneTemplate, .matchResources.controlPlane not set",
			templateRef: &api.TemplateRef{
				APIVersion:   "controlplane.cluster.x-k8s.io/v1beta1",
				Kind:         "ControlPlaneTemplate",
				TemplateType: api.ControlPlaneTemplateType,
			},
			selector: clusterv1.PatchSelector{
				APIVersion:     "controlplane.cluster.x-k8s.io/v1beta1",
				Kind:           "ControlPlaneTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{},
			},
			match: false,
		},
		{
			name: "Don't match ControlPlaneTemplate, .matchResources.controlPlane false",
			templateRef: &api.TemplateRef{
				APIVersion:   "controlplane.cluster.x-k8s.io/v1beta1",
				Kind:         "ControlPlaneTemplate",
				TemplateType: api.ControlPlaneTemplateType,
			},
			selector: clusterv1.PatchSelector{
				APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
				Kind:       "ControlPlaneTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					ControlPlane: pointer.Bool(false),
				},
			},
			match: false,
		},
		{
			name: "Match ControlPlane InfrastructureMachineTemplate",
			templateRef: &api.TemplateRef{
				APIVersion:   "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:         "AzureMachineTemplate",
				TemplateType: api.ControlPlaneInfrastructureMachineTemplateType,
			},
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "AzureMachineTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					ControlPlane: pointer.Bool(true),
				},
			},
			match: true,
		},
		{
			name: "Match MD BootstrapTemplate",
			templateRef: &api.TemplateRef{
				APIVersion:   "bootstrap.cluster.x-k8s.io/v1beta1",
				Kind:         "BootstrapTemplate",
				TemplateType: api.MachineDeploymentBootstrapConfigTemplateType,
				MachineDeploymentRef: api.MachineDeploymentRef{
					Class: "classA",
				},
			},
			selector: clusterv1.PatchSelector{
				APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
				Kind:       "BootstrapTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
						Names: []string{"classA"},
					},
				},
			},
			match: true,
		},
		{
			name: "Don't match BootstrapTemplate, .matchResources.machineDeploymentClass not set",
			templateRef: &api.TemplateRef{
				APIVersion:   "bootstrap.cluster.x-k8s.io/v1beta1",
				Kind:         "BootstrapTemplate",
				TemplateType: api.MachineDeploymentBootstrapConfigTemplateType,
				MachineDeploymentRef: api.MachineDeploymentRef{
					Class: "classA",
				},
			},
			selector: clusterv1.PatchSelector{
				APIVersion:     "bootstrap.cluster.x-k8s.io/v1beta1",
				Kind:           "BootstrapTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{},
			},
			match: false,
		},
		{
			name: "Don't match BootstrapTemplate, .matchResources.machineDeploymentClass does not match",
			templateRef: &api.TemplateRef{
				APIVersion:   "bootstrap.cluster.x-k8s.io/v1beta1",
				Kind:         "BootstrapTemplate",
				TemplateType: api.MachineDeploymentBootstrapConfigTemplateType,
				MachineDeploymentRef: api.MachineDeploymentRef{
					Class: "classA",
				},
			},
			selector: clusterv1.PatchSelector{
				APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
				Kind:       "BootstrapTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
						Names: []string{"classB"},
					},
				},
			},
			match: false,
		},
		{
			name: "Match MD InfrastructureMachineTemplate",
			templateRef: &api.TemplateRef{
				APIVersion:   "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:         "AzureMachineTemplate",
				TemplateType: api.MachineDeploymentInfrastructureMachineTemplateType,
				MachineDeploymentRef: api.MachineDeploymentRef{
					Class: "classA",
				},
			},
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "AzureMachineTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
						Names: []string{"classA"},
					},
				},
			},
			match: true,
		},
		{
			name: "Don't match: unknown target type",
			templateRef: &api.TemplateRef{
				APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
				Kind:       "ControlPlaneTemplate",
				// invalid is an invalid TemplateType.
				TemplateType: "invalid",
			},
			selector: clusterv1.PatchSelector{
				APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
				Kind:       "ControlPlaneTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					ControlPlane: pointer.Bool(true),
				},
			},
			match: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(templateMatchesSelector(tt.templateRef, tt.selector)).To(Equal(tt.match))
		})
	}
}

func TestCalculateValue(t *testing.T) {
	tests := []struct {
		name      string
		patch     clusterv1.JSONPatch
		variables map[string]apiextensionsv1.JSON
		want      *apiextensionsv1.JSON
		wantErr   bool
	}{
		{
			name:    "Fails if neither .value nor .valueFrom are set",
			patch:   clusterv1.JSONPatch{},
			wantErr: true,
		},
		{
			name: "Fails if both .value and .valueFrom are set",
			patch: clusterv1.JSONPatch{
				Value: &apiextensionsv1.JSON{Raw: []byte(`"value"`)},
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableA"),
				},
			},
			wantErr: true,
		},
		{
			name: "Fails if .valueFrom.variable and .valueFrom.template are set",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableA"),
					Template: pointer.String("template"),
				},
			},
			wantErr: true,
		},
		{
			name: "Fails if .valueFrom is set, but .valueFrom.variable and .valueFrom.template are both not set",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{},
			},
			wantErr: true,
		},
		{
			name: "Should return .value if set",
			patch: clusterv1.JSONPatch{
				Value: &apiextensionsv1.JSON{Raw: []byte(`"value"`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`"value"`)},
		},
		{
			name: "Should return .valueFrom.variable if set",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableA"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableA": {Raw: []byte(`"value"`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`"value"`)},
		},
		{
			name: "Fails if .valueFrom.variable is set but variable does not exist",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableA"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableB": {Raw: []byte(`"value"`)},
			},
			wantErr: true,
		},
		{
			name: "Should return .valueFrom.variable if set: builtinVariable int",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("builtin.controlPlane.replicas"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				patchvariables.BuiltinsName: {Raw: []byte(`{"controlPlane":{"replicas":3}}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`3`)},
		},
		{
			name: "Should return .valueFrom.variable if set: builtinVariable string",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("builtin.cluster.topology.version"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				patchvariables.BuiltinsName: {Raw: []byte(`{"cluster":{"name":"cluster-name","namespace":"default","topology":{"class":"clusterClass1","version":"v1.21.1"}}}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`"v1.21.1"`)},
		},
		{
			name: "Should return .valueFrom.variable if set: variable 'builtin'",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("builtin"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				patchvariables.BuiltinsName: {Raw: []byte(`{"cluster":{"name":"cluster-name","namespace":"default","topology":{"class":"clusterClass1","version":"v1.21.1"}}}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`{"cluster":{"name":"cluster-name","namespace":"default","topology":{"class":"clusterClass1","version":"v1.21.1"}}}`)},
		},
		{
			name: "Should return .valueFrom.variable if set: variable 'builtin.cluster'",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("builtin.cluster"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				patchvariables.BuiltinsName: {Raw: []byte(`{"cluster":{"name":"cluster-name","namespace":"default","topology":{"class":"clusterClass1","version":"v1.21.1"}}}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`{"name":"cluster-name","namespace":"default","topology":{"class":"clusterClass1","version":"v1.21.1"}}`)},
		},
		{
			name: "Should return .valueFrom.variable if set: variable 'builtin.cluster.topology'",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("builtin.cluster.topology"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				patchvariables.BuiltinsName: {Raw: []byte(`{"cluster":{"name":"cluster-name","namespace":"default","topology":{"class":"clusterClass1","version":"v1.21.1"}}}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`{"class":"clusterClass1","version":"v1.21.1"}`)},
		},
		{
			// NOTE: Template rendering is tested more extensively in TestRenderValueTemplate
			name: "Should return rendered .valueFrom.template if set",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Template: pointer.String("{{ .variableA }}"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableA": {Raw: []byte(`"value"`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`"value"`)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := calculateValue(tt.patch, tt.variables)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestRenderValueTemplate(t *testing.T) {
	tests := []struct {
		name      string
		template  string
		variables map[string]apiextensionsv1.JSON
		want      *apiextensionsv1.JSON
		wantErr   bool
	}{
		// Basic types
		{
			name:     "Should render a string variable",
			template: `{{ .stringVariable }}`,
			variables: map[string]apiextensionsv1.JSON{
				"stringVariable": {Raw: []byte(`"bar"`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`"bar"`)},
		},
		{
			name:     "Should render an integer variable",
			template: `{{ .integerVariable }}`,
			variables: map[string]apiextensionsv1.JSON{
				"integerVariable": {Raw: []byte("3")},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`3`)},
		},
		{
			name:     "Should render a number variable",
			template: `{{ .numberVariable }}`,
			variables: map[string]apiextensionsv1.JSON{
				"numberVariable": {Raw: []byte("2.5")},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`2.5`)},
		},
		{
			name:     "Should render a boolean variable",
			template: `{{ .booleanVariable }}`,
			variables: map[string]apiextensionsv1.JSON{
				"booleanVariable": {Raw: []byte("true")},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`true`)},
		},
		// YAML
		{
			name: "Should render a YAML array",
			template: `
- contentFrom:
    secret:
      key: control-plane-azure.json
      name: "{{ .builtin.cluster.name }}-control-plane-azure-json"
  owner: root:root
`,
			variables: map[string]apiextensionsv1.JSON{
				patchvariables.BuiltinsName: {Raw: []byte(`{"cluster":{"name":"cluster1"}}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`
[{
	"contentFrom":{
		"secret":{
			"key":"control-plane-azure.json",
			"name":"cluster1-control-plane-azure-json"
		}
	},
	"owner":"root:root"
}]`),
			},
		},
		{
			name: "Should render a YAML object",
			template: `
contentFrom:
  secret:
    key: control-plane-azure.json
    name: "{{ .builtin.cluster.name }}-control-plane-azure-json"
owner: root:root
`,
			variables: map[string]apiextensionsv1.JSON{
				patchvariables.BuiltinsName: {Raw: []byte(`{"cluster":{"name":"cluster1"}}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`
{
	"contentFrom":{
		"secret":{
			"key":"control-plane-azure.json",
			"name":"cluster1-control-plane-azure-json"
		}
	},
	"owner":"root:root"
}`),
			},
		},
		// JSON
		{
			name: "Should render a JSON array",
			template: `
[{
	"contentFrom":{
		"secret":{
			"key":"control-plane-azure.json",
			"name":"{{ .builtin.cluster.name }}-control-plane-azure-json"
		}
	},
	"owner":"root:root"
}]`,
			variables: map[string]apiextensionsv1.JSON{
				patchvariables.BuiltinsName: {Raw: []byte(`{"cluster":{"name":"cluster1"}}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`
[{
	"contentFrom":{
		"secret":{
			"key":"control-plane-azure.json",
			"name":"cluster1-control-plane-azure-json"
		}
	},
	"owner":"root:root"
}]`),
			},
		},
		{
			name: "Should render a JSON object",
			template: `
{
	"contentFrom":{
		"secret":{
			"key":"control-plane-azure.json",
			"name":"{{ .builtin.cluster.name }}-control-plane-azure-json"
		}
	},
	"owner":"root:root"
}`,
			variables: map[string]apiextensionsv1.JSON{
				patchvariables.BuiltinsName: {Raw: []byte(`{"cluster":{"name":"cluster1"}}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`
{
	"contentFrom":{
		"secret":{
			"key":"control-plane-azure.json",
			"name":"cluster1-control-plane-azure-json"
		}
	},
	"owner":"root:root"
}`),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := renderValueTemplate(tt.template, tt.variables)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			// Compact tt.want so we can use easily readable multi-line
			// strings in the test definition.
			var compactWant bytes.Buffer
			g.Expect(json.Compact(&compactWant, tt.want.Raw)).To(Succeed())

			g.Expect(string(got.Raw)).To(Equal(compactWant.String()))
		})
	}
}

func TestCalculateTemplateData(t *testing.T) {
	tests := []struct {
		name      string
		variables map[string]apiextensionsv1.JSON
		want      map[string]interface{}
		wantErr   bool
	}{
		{
			name: "Fails for invalid JSON value (missing closing quote)",
			variables: map[string]apiextensionsv1.JSON{
				"stringVariable": {Raw: []byte(`"cluster-name`)},
			},
			wantErr: true,
		},
		{
			name: "Fails for invalid JSON value (string without quotes)",
			variables: map[string]apiextensionsv1.JSON{
				"stringVariable": {Raw: []byte(`cluster-name`)},
			},
			wantErr: true,
		},
		{
			name: "Should convert basic types",
			variables: map[string]apiextensionsv1.JSON{
				"stringVariable":  {Raw: []byte(`"cluster-name"`)},
				"integerVariable": {Raw: []byte("4")},
				"numberVariable":  {Raw: []byte("2.5")},
				"booleanVariable": {Raw: []byte("true")},
			},
			want: map[string]interface{}{
				"stringVariable":  "cluster-name",
				"integerVariable": float64(4),
				"numberVariable":  float64(2.5),
				"booleanVariable": true,
			},
		},
		{
			name: "Should handle nested variables correctly",
			variables: map[string]apiextensionsv1.JSON{
				"builtin":      {Raw: []byte(`{"cluster":{"name":"cluster-name","namespace":"default","topology":{"class":"clusterClass1","version":"v1.22.0"}},"controlPlane":{"replicas":3},"machineDeployment":{"version":"v1.21.2"}}`)},
				"userVariable": {Raw: []byte(`"value"`)},
			},
			want: map[string]interface{}{
				"builtin": map[string]interface{}{
					"cluster": map[string]interface{}{
						"name":      "cluster-name",
						"namespace": "default",
						"topology": map[string]interface{}{
							"class":   "clusterClass1",
							"version": "v1.22.0",
						},
					},
					"controlPlane": map[string]interface{}{
						"replicas": float64(3),
					},
					"machineDeployment": map[string]interface{}{
						"version": "v1.21.2",
					},
				},
				"userVariable": "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := calculateTemplateData(tt.variables)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestMergeVariables(t *testing.T) {
	t.Run("Merge variables", func(t *testing.T) {
		g := NewWithT(t)

		m, err := mergeVariableMaps(
			map[string]apiextensionsv1.JSON{
				patchvariables.BuiltinsName: {Raw: []byte(`{"cluster":{"name":"cluster-name","namespace":"default","topology":{"class":"clusterClass1","version":"v1.21.1"}}}`)},
				"a":                         {Raw: []byte("a-different")},
				"c":                         {Raw: []byte("c")},
			},
			map[string]apiextensionsv1.JSON{
				// Verify that builtin variables are merged correctly and
				// the latter variables take precedent ("cluster-name-overwrite").
				patchvariables.BuiltinsName: {Raw: []byte(`{"controlPlane":{"replicas":3},"cluster":{"name":"cluster-name-overwrite"}}`)},
				"a":                         {Raw: []byte("a")},
				"b":                         {Raw: []byte("b")},
			},
		)
		g.Expect(err).To(BeNil())

		g.Expect(m).To(HaveKeyWithValue(patchvariables.BuiltinsName, apiextensionsv1.JSON{Raw: []byte(`{"cluster":{"name":"cluster-name-overwrite","namespace":"default","topology":{"version":"v1.21.1","class":"clusterClass1"}},"controlPlane":{"replicas":3}}`)}))
		g.Expect(m).To(HaveKeyWithValue("a", apiextensionsv1.JSON{Raw: []byte("a")}))
		g.Expect(m).To(HaveKeyWithValue("b", apiextensionsv1.JSON{Raw: []byte("b")}))
		g.Expect(m).To(HaveKeyWithValue("c", apiextensionsv1.JSON{Raw: []byte("c")}))
	})
}

// toJSONCompact is used to be able to write JSON values in a readable manner.
func toJSONCompact(value string) apiextensionsv1.JSON {
	var compactValue bytes.Buffer
	if err := json.Compact(&compactValue, []byte(value)); err != nil {
		panic(err)
	}
	return apiextensionsv1.JSON{Raw: compactValue.Bytes()}
}
