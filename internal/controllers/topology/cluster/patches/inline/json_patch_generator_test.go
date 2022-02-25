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
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/patches/api"
	patchvariables "sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/patches/variables"
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
								ControlPlane: true,
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
							// .valueFrom.template using sprig functions
							{
								Op:   "replace",
								Path: "/spec/valueFrom/template",
								ValueFrom: &clusterv1.JSONPatchValue{
									Template: pointer.String(`template {{ .variableB | lower | repeat 5 }}`),
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
							// test .builtin.controlPlane.machineTemplate.InfrastructureRef.name var.
							{
								Op:   "replace",
								Path: "/spec/template/spec/files",
								ValueFrom: &clusterv1.JSONPatchValue{
									Template: pointer.String(`[{"contentFrom":{"secret":{"key":"control-plane-azure.json","name":"{{ .builtin.controlPlane.machineTemplate.infrastructureRef.name }}-azure-json"}}}]`),
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
							"builtin":   {Raw: []byte(`{"controlPlane":{"replicas":3,"machineTemplate":{"infrastructureRef":{"name":"controlPlaneInfrastructureMachineTemplate1"}}}}`)},
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
{"op":"replace","path":"/spec/valueFrom/template","value":"template bbbbb"},
{"op":"replace","path":"/spec/templatePrecedent","value":"C-template"},
{"op":"replace","path":"/spec/builtinClusterName","value":"cluster-name"},
{"op":"replace","path":"/spec/builtinControlPlaneReplicas","value":3},
{"op":"replace","path":"/spec/template/spec/files","value":[{
  "contentFrom":{
    "secret":{
      "key":"control-plane-azure.json",
      "name":"controlPlaneInfrastructureMachineTemplate1-azure-json"
    }
  }
}]}]`),
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
								ControlPlane: true,
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
							{
								Op:   "remove",
								Path: "/spec/template/spec/kubeadmConfigSpec/clusterConfiguration/apiServer/extraArgs",
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
{"op":"replace","path":"/spec/template/spec/kubeadmConfigSpec/files","value":[{"contentFrom":{"secret":{"key":"control-plane-azure.json","name":"cluster-name-control-plane-azure-json"}},"owner":"root:root"}]},
{"op":"remove","path":"/spec/template/spec/kubeadmConfigSpec/clusterConfiguration/apiServer/extraArgs"}
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
					InfrastructureCluster: true,
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
					InfrastructureCluster: false,
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
					ControlPlane: true,
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
					ControlPlane: false,
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
					ControlPlane: true,
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
					ControlPlane: true,
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

func TestPatchIsEnabled(t *testing.T) {
	tests := []struct {
		name      string
		enabledIf *string
		variables map[string]apiextensionsv1.JSON
		want      bool
		wantErr   bool
	}{
		{
			name:      "Enabled if enabledIf is not set",
			enabledIf: nil,
			want:      true,
		},
		{
			name:      "Fail if template is invalid",
			enabledIf: pointer.String(`{{ variable }}`), // . is missing
			wantErr:   true,
		},
		// Hardcoded value.
		{
			name:      "Enabled if template is true ",
			enabledIf: pointer.String(`true`),
			want:      true,
		},
		{
			name: "Enabled if template is true (even with leading and trailing new line)",
			enabledIf: pointer.String(`
true
`),
			want: true,
		},
		{
			name:      "Disabled if template is false",
			enabledIf: pointer.String(`false`),
			want:      false,
		},
		// Boolean variable.
		{
			name:      "Enabled if simple template with boolean variable evaluates to true",
			enabledIf: pointer.String(`{{ .httpProxyEnabled }}`),
			variables: map[string]apiextensionsv1.JSON{
				"httpProxyEnabled": {Raw: []byte(`true`)},
			},
			want: true,
		},
		{
			name: "Enabled if simple template with boolean variable evaluates to true (even with leading and trailing new line",
			enabledIf: pointer.String(`
{{ .httpProxyEnabled }}
`),
			variables: map[string]apiextensionsv1.JSON{
				"httpProxyEnabled": {Raw: []byte(`true`)},
			},
			want: true,
		},
		{
			name:      "Disabled if simple template with boolean variable evaluates to false",
			enabledIf: pointer.String(`{{ .httpProxyEnabled }}`),
			variables: map[string]apiextensionsv1.JSON{
				"httpProxyEnabled": {Raw: []byte(`false`)},
			},
			want: false,
		},
		// Render value with if/else.
		{
			name: "Enabled if template with if evaluates to true",
			// Else is not needed because we check if the result is equal to true.
			enabledIf: pointer.String(`{{ if eq "v1.21.1" .builtin.cluster.topology.version }}true{{end}}`),
			variables: map[string]apiextensionsv1.JSON{
				"builtin": {Raw: []byte(`{"cluster":{"name":"cluster-name","namespace":"default","topology":{"class":"clusterClass1","version":"v1.21.1"}}}`)},
			},
			want: true,
		},
		{
			name:      "Disabled if template with if evaluates to false",
			enabledIf: pointer.String(`{{ if eq "v1.21.2" .builtin.cluster.topology.version }}true{{end}}`),
			variables: map[string]apiextensionsv1.JSON{
				"builtin": {Raw: []byte(`{"cluster":{"name":"cluster-name","namespace":"default","topology":{"class":"clusterClass1","version":"v1.21.1"}}}`)},
			},
			want: false,
		},
		{
			name:      "Enabled if template with if/else evaluates to true",
			enabledIf: pointer.String(`{{ if eq "v1.21.1" .builtin.cluster.topology.version }}true{{else}}false{{end}}`),
			variables: map[string]apiextensionsv1.JSON{
				"builtin": {Raw: []byte(`{"cluster":{"name":"cluster-name","namespace":"default","topology":{"class":"clusterClass1","version":"v1.21.1"}}}`)},
			},
			want: true,
		},
		{
			name:      "Disabled if template with if/else evaluates to false",
			enabledIf: pointer.String(`{{ if eq "v1.21.2" .builtin.cluster.topology.version }}true{{else}}false{{end}}`),
			variables: map[string]apiextensionsv1.JSON{
				"builtin": {Raw: []byte(`{"cluster":{"name":"cluster-name","namespace":"default","topology":{"class":"clusterClass1","version":"v1.21.1"}}}`)},
			},
			want: false,
		},
		// Render value with if to check if var is not empty.
		{
			name:      "Enabled if template which checks if variable is set evaluates to true",
			enabledIf: pointer.String(`{{ if .variableA }}true{{end}}`),
			variables: map[string]apiextensionsv1.JSON{
				"variableA": {Raw: []byte(`"abc"`)},
			},
			want: true,
		},
		{
			name:      "Disabled if template which checks if variable is set evaluates to false (variable empty)",
			enabledIf: pointer.String(`{{ if .variableA }}true{{end}}`),
			variables: map[string]apiextensionsv1.JSON{
				"variableA": {Raw: []byte(``)},
			},
			want: false,
		},
		{
			name:      "Disabled if template which checks if variable is set evaluates to false (variable empty string)",
			enabledIf: pointer.String(`{{ if .variableA }}true{{end}}`),
			variables: map[string]apiextensionsv1.JSON{
				"variableA": {Raw: []byte(`""`)},
			},
			want: false,
		},
		{
			name:      "Disabled if template which checks if variable is set evaluates to false (variable does not exist)",
			enabledIf: pointer.String(`{{ if .variableA }}true{{end}}`),
			variables: map[string]apiextensionsv1.JSON{
				"variableB": {Raw: []byte(``)},
			},
			want: false,
		},
		// Render value with object variable.
		// NOTE: the builtin variable tests above test something very similar, so this
		// test mostly exists to visualize how user-defined object variables can be used.
		{
			name:      "Enabled if template with complex variable evaluates to true",
			enabledIf: pointer.String(`{{ if .httpProxy.enabled }}true{{end}}`),
			variables: map[string]apiextensionsv1.JSON{
				"httpProxy": {Raw: []byte(`{"enabled": true, "url": "localhost:3128", "noProxy": "internal.example.com"}`)},
			},
			want: true,
		},
		{
			name:      "Disabled if template with complex variable evaluates to false",
			enabledIf: pointer.String(`{{ if .httpProxy.enabled }}true{{end}}`),
			variables: map[string]apiextensionsv1.JSON{
				"httpProxy": {Raw: []byte(`{"enabled": false, "url": "localhost:3128", "noProxy": "internal.example.com"}`)},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := patchIsEnabled(tt.enabledIf, tt.variables)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
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
		// Objects
		{
			name: "Should return .valueFrom.variable if set: whole object",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableObject"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableObject": {Raw: []byte(`{"requiredProperty":false,"boolProperty":true,"integerProperty":1,"enumProperty":"enumValue2"}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`{"requiredProperty":false,"boolProperty":true,"integerProperty":1,"enumProperty":"enumValue2"}`)},
		},
		{
			name: "Should return .valueFrom.variable if set: nested bool property",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableObject.boolProperty"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableObject": {Raw: []byte(`{"boolProperty":true,"integerProperty":1,"enumProperty":"enumValue2"}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`true`)},
		},
		{
			name: "Should return .valueFrom.variable if set: nested integer property",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableObject.integerProperty"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableObject": {Raw: []byte(`{"boolProperty":true,"integerProperty":1,"enumProperty":"enumValue2"}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`1`)},
		},
		{
			name: "Should return .valueFrom.variable if set: nested string property",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableObject.enumProperty"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableObject": {Raw: []byte(`{"boolProperty":true,"integerProperty":1,"enumProperty":"enumValue2"}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`"enumValue2"`)},
		},
		{
			name: "Fails if .valueFrom.variable object variable does not exist",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableObject.enumProperty"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"anotherObject": {Raw: []byte(`{"boolProperty":true,"integerProperty":1,"enumProperty":"enumValue2"}`)},
			},
			wantErr: true,
		},
		{
			name: "Fails if .valueFrom.variable nested object property does not exist",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableObject.nonExistingProperty"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"anotherObject": {Raw: []byte(`{"boolProperty":true,"integerProperty":1}`)},
			},
			wantErr: true,
		},
		{
			name: "Fails if .valueFrom.variable nested object property is an array instead",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					// NOTE: it's not possible to access a property of an array element without index.
					Variable: pointer.String("variableObject.nonExistingProperty"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"anotherObject": {Raw: []byte(`[{"boolProperty":true,"integerProperty":1}]`)},
			},
			wantErr: true,
		},
		// Deeper nested Objects
		{
			name: "Should return .valueFrom.variable if set: nested object property top-level",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableObject"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableObject": {Raw: []byte(`{"firstLevel":{"secondLevel":{"leaf":"value"}}}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`{"firstLevel":{"secondLevel":{"leaf":"value"}}}`)},
		},
		{
			name: "Should return .valueFrom.variable if set: nested object property firstLevel",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableObject.firstLevel"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableObject": {Raw: []byte(`{"firstLevel":{"secondLevel":{"leaf":"value"}}}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`{"secondLevel":{"leaf":"value"}}`)},
		},
		{
			name: "Should return .valueFrom.variable if set: nested object property secondLevel",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableObject.firstLevel.secondLevel"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableObject": {Raw: []byte(`{"firstLevel":{"secondLevel":{"leaf":"value"}}}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`{"leaf":"value"}`)},
		},
		{
			name: "Should return .valueFrom.variable if set: nested object property leaf",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableObject.firstLevel.secondLevel.leaf"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableObject": {Raw: []byte(`{"firstLevel":{"secondLevel":{"leaf":"value"}}}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`"value"`)},
		},
		// Array
		{
			name: "Should return .valueFrom.variable if set: array",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableArray"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableArray": {Raw: []byte(`["abc","def"]`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`["abc","def"]`)},
		},
		{
			name: "Should return .valueFrom.variable if set: array element",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableArray[0]"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableArray": {Raw: []byte(`["abc","def"]`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`"abc"`)},
		},
		{
			name: "Should return .valueFrom.variable if set: nested array",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableArray.firstLevel"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableArray": {Raw: []byte(`{"firstLevel":["abc","def"]}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`["abc","def"]`)},
		},
		{
			name: "Should return .valueFrom.variable if set: nested array element",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableArray.firstLevel[1]"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableArray": {Raw: []byte(`{"firstLevel":[{"secondLevel":"firstElement"},{"secondLevel":"secondElement"}]}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`{"secondLevel":"secondElement"}`)},
		},
		{
			name: "Should return .valueFrom.variable if set: nested field of nested array element",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableArray.firstLevel[1].secondLevel"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableArray": {Raw: []byte(`{"firstLevel":[{"secondLevel":"firstElement"},{"secondLevel":"secondElement"}]}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`"secondElement"`)},
		},
		{
			name: "Fails if .valueFrom.variable array path is invalid: only left delimiter",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableArray.firstLevel["),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableArray": {Raw: []byte(`{"firstLevel":[{"secondLevel":"firstElement"}]}`)},
			},
			wantErr: true,
		},
		{
			name: "Fails if .valueFrom.variable array path is invalid: only right delimiter",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableArray.firstLevel]"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableArray": {Raw: []byte(`{"firstLevel":[{"secondLevel":"firstElement"}]}`)},
			},
			wantErr: true,
		},
		{
			name: "Fails if .valueFrom.variable array path is invalid: no index",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableArray.firstLevel[]"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableArray": {Raw: []byte(`{"firstLevel":[{"secondLevel":"firstElement"}]}`)},
			},
			wantErr: true,
		},
		{
			name: "Fails if .valueFrom.variable array path is invalid: text index",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableArray.firstLevel[someText]"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableArray": {Raw: []byte(`{"firstLevel":[{"secondLevel":"firstElement"}]}`)},
			},
			wantErr: true,
		},
		{
			name: "Fails if .valueFrom.variable array path is invalid: negative index",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableArray.firstLevel[-1]"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableArray": {Raw: []byte(`{"firstLevel":[{"secondLevel":"firstElement"}]}`)},
			},
			wantErr: true,
		},
		{
			name: "Fails if .valueFrom.variable array path is invalid: index out of bounds",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableArray.firstLevel[1]"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableArray": {Raw: []byte(`{"firstLevel":[{"secondLevel":"firstElement"}]}`)},
			},
			wantErr: true,
		},
		{
			name: "Fails if .valueFrom.variable array path is invalid: variable is an object instead",
			patch: clusterv1.JSONPatch{
				ValueFrom: &clusterv1.JSONPatchValue{
					Variable: pointer.String("variableArray.firstLevel[1]"),
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"variableArray": {Raw: []byte(`{"firstLevel":{"secondLevel":"firstElement"}}`)},
			},
			wantErr: true,
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

func TestParsePathSegment(t *testing.T) {
	tests := []struct {
		name            string
		segment         string
		wantPathSegment *pathSegment
		wantErr         bool
	}{
		{
			name:    "parse basic segment",
			segment: "propertyName",
			wantPathSegment: &pathSegment{
				path:  "propertyName",
				index: nil,
			},
		},
		{
			name:    "parse segment with index",
			segment: "arrayProperty[5]",
			wantPathSegment: &pathSegment{
				path:  "arrayProperty",
				index: pointer.Int(5),
			},
		},
		{
			name:    "fail invalid syntax: only left delimiter",
			segment: "arrayProperty[",
			wantErr: true,
		},
		{
			name:    "fail invalid syntax: only right delimiter",
			segment: "arrayProperty]",
			wantErr: true,
		},
		{
			name:    "fail invalid syntax: both delimiter but no index",
			segment: "arrayProperty[]",
			wantErr: true,
		},
		{
			name:    "fail invalid syntax: negative index",
			segment: "arrayProperty[-1]",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := parsePathSegment(tt.segment)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(Equal(tt.wantPathSegment))
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
		{
			name:     "Fails if the template is invalid",
			template: `{{ booleanVariable }}`,
			variables: map[string]apiextensionsv1.JSON{
				"booleanVariable": {Raw: []byte("true")},
			},
			wantErr: true,
		},
		// Default variables via template
		{
			name:     "Should render depending on variable existence: variable is set",
			template: `{{ if .vnetName }}{{.vnetName}}{{else}}{{.builtin.cluster.name}}-vnet{{end}}`,
			variables: map[string]apiextensionsv1.JSON{
				patchvariables.BuiltinsName: {Raw: []byte(`{"cluster":{"name":"cluster1"}}`)},
				"vnetName":                  {Raw: []byte(`"custom-network"`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`"custom-network"`)},
		},
		{
			name:     "Should render depending on variable existence: variable is not set",
			template: `{{ if .vnetName }}{{.vnetName}}{{else}}{{.builtin.cluster.name}}-vnet{{end}}`,
			variables: map[string]apiextensionsv1.JSON{
				patchvariables.BuiltinsName: {Raw: []byte(`{"cluster":{"name":"cluster1"}}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`"cluster1-vnet"`)},
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
		// Object types
		{
			name:     "Should render a object property top-level",
			template: `{{ .variableObject }}`,
			variables: map[string]apiextensionsv1.JSON{
				"variableObject": {Raw: []byte(`{"firstLevel":{"secondLevel":{"leaf":"value"}}}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`"map[firstLevel:map[secondLevel:map[leaf:value]]]"`)}, // Not ideal but that's go templating.
		},
		{
			name:     "Should render a object property firstLevel",
			template: `{{ .variableObject.firstLevel }}`,
			variables: map[string]apiextensionsv1.JSON{
				"variableObject": {Raw: []byte(`{"firstLevel":{"secondLevel":{"leaf":"value"}}}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`"map[secondLevel:map[leaf:value]]"`)}, // Not ideal but that's go templating.
		},
		{
			name:     "Should render a object property secondLevel",
			template: `{{ .variableObject.firstLevel.secondLevel }}`,
			variables: map[string]apiextensionsv1.JSON{
				"variableObject": {Raw: []byte(`{"firstLevel":{"secondLevel":{"leaf":"value"}}}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`"map[leaf:value]"`)}, // Not ideal but that's go templating.
		},
		{
			name:     "Should render a object property leaf",
			template: `{{ .variableObject.firstLevel.secondLevel.leaf }}`,
			variables: map[string]apiextensionsv1.JSON{
				"variableObject": {Raw: []byte(`{"firstLevel":{"secondLevel":{"leaf":"value"}}}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`"value"`)},
		},
		{
			name:     "Should render even if object property leaf does not exist",
			template: `{{ .variableObject.firstLevel.secondLevel.anotherLeaf }}`,
			variables: map[string]apiextensionsv1.JSON{
				"variableObject": {Raw: []byte(`{"firstLevel":{"secondLevel":{"leaf":"value"}}}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`"\u003cno value\u003e"`)},
		},
		{
			name: "Should render a object with range",
			template: `
{
{{ range $key, $value := .variableObject }}
 "{{$key}}-modified": "{{$value}}",
{{end}}
}
`,
			variables: map[string]apiextensionsv1.JSON{
				"variableObject": {Raw: []byte(`{"key1":"value1","key2":"value2"}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`{"key1-modified":"value1","key2-modified":"value2"}`)},
		},
		// Arrays
		{
			name:     "Should render an array property",
			template: `{{ .variableArray }}`,
			variables: map[string]apiextensionsv1.JSON{
				"variableArray": {Raw: []byte(`["string1","string2","string3"]`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`["string1 string2 string3"]`)}, // // Not ideal but that's go templating.
		},
		{
			name: "Should render an array property with range",
			template: `
{
{{ range .variableArray }}
 "{{.}}-modified": "value",
{{end}}
}
`,
			variables: map[string]apiextensionsv1.JSON{
				"variableArray": {Raw: []byte(`["string1","string2","string3"]`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`{"string1-modified":"value","string2-modified":"value","string3-modified":"value"}`)},
		},
		{
			name:     "Should render an array property: array element",
			template: `{{ index .variableArray 1 }}`,
			variables: map[string]apiextensionsv1.JSON{
				"variableArray": {Raw: []byte(`["string1","string2","string3"]`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`"string2"`)},
		},
		{
			name:     "Should render an array property: array object element field",
			template: `{{ (index .variableArray 1).propertyA }}`,
			variables: map[string]apiextensionsv1.JSON{
				"variableArray": {Raw: []byte(`[{"propertyA":"A0","propertyB":"B0"},{"propertyA":"A1","propertyB":"B1"}]`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`"A1"`)},
		},
		// Pick up config for a specific MD Class
		{
			name:     "Should render a object property with a lookup based on a builtin variable",
			template: `{{ (index .mdConfig .builtin.machineDeployment.class).config }}`,
			variables: map[string]apiextensionsv1.JSON{
				"mdConfig": {Raw: []byte(`{
"mdClass1":{
	"config":"configValue1"
},
"mdClass2":{
	"config":"configValue2"
}
}`)},
				// Schema must either support complex objects with predefined keys/mdClasses or maps with additionalProperties.
				patchvariables.BuiltinsName: {Raw: []byte(`{"machineDeployment":{"version":"v1.21.1","class":"mdClass2","name":"md1","topologyName":"md-topology","replicas":3}}`)},
			},
			want: &apiextensionsv1.JSON{Raw: []byte(`"configValue2"`)},
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
