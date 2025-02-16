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

package webhooks

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestValidatePatches(t *testing.T) {
	tests := []struct {
		name         string
		clusterClass clusterv1.ClusterClass
		runtimeSDK   bool
		wantErr      bool
	}{
		{
			name: "pass multiple patches that are correctly formatted",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},

					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op:   "add",
											Path: "/spec/template/spec/variableSetting/variableValue1",
											ValueFrom: &clusterv1.JSONPatchValue{
												Variable: ptr.To("variableName1"),
											},
										},
									},
								},
							},
						},
						{
							Name: "patch2",
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
											Op:   "add",
											Path: "/spec/template/spec/variableSetting/variableValue2",
											ValueFrom: &clusterv1.JSONPatchValue{
												Variable: ptr.To("variableName2"),
											},
										},
									},
								},
							},
						},
					},
					Variables: []clusterv1.ClusterClassVariable{
						{
							Name:     "variableName1",
							Required: true,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
						{
							Name:     "variableName2",
							Required: true,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},

		// Patch name validation
		{
			name: "error if patch name is empty",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},

					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "",
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
											Op:   "add",
											Path: "/spec/template/spec/kubeadmConfigSpec/clusterConfiguration/controllerManager/extraArgs/cluster-name",
											ValueFrom: &clusterv1.JSONPatchValue{
												Variable: ptr.To("variableName"),
											},
										},
									},
								},
							},
						},
					},
					Variables: []clusterv1.ClusterClassVariable{
						{
							Name:     "variableName",
							Required: true,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "error if patches name is not unique",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},

					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op:   "add",
											Path: "/spec/template/spec/kubeadmConfigSpec/clusterConfiguration/controllerManager/extraArgs/cluster-name",
											ValueFrom: &clusterv1.JSONPatchValue{
												Variable: ptr.To("variableName1"),
											},
										},
									},
								},
							},
						},
						{
							Name: "patch1",
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
											Op:   "add",
											Path: "/spec/template/spec/variableSetting/variableValue",
											ValueFrom: &clusterv1.JSONPatchValue{
												Variable: ptr.To("variableName2"),
											},
										},
									},
								},
							},
						},
					},
					Variables: []clusterv1.ClusterClassVariable{
						{
							Name:     "variableName1",
							Required: true,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
						{
							Name:     "variableName2",
							Required: true,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},

		// enabledIf validation
		{
			name: "pass if enabledIf is a valid Go template",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},
					Patches: []clusterv1.ClusterClassPatch{
						{
							Name:        "patch1",
							EnabledIf:   ptr.To(`template {{ .variableB }}`),
							Definitions: []clusterv1.PatchDefinition{},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "error if enabledIf is an invalid Go template",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},
					Patches: []clusterv1.ClusterClassPatch{
						{
							Name:      "patch1",
							EnabledIf: ptr.To(`template {{{{{{{{ .variableB }}`),
						},
					},
				},
			},
			wantErr: true,
		},
		// Patch "op" (operation) validation
		{
			name: "error if patch op is not \"add\" \"remove\" or \"replace\"",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},

					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											// OP is set to an unrecognized value here.
											Op:   "drop",
											Path: "/spec/template/spec/variableSetting/variableValue2",
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},

		// Patch path validation
		{
			name: "error if jsonPath does not begin with \"/spec/\"",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},

					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op: "remove",
											// Path is set to status.
											Path: "/status/template/spec/variableSetting/variableValue2",
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "pass if jsonPatch path uses a valid index for add i.e. 0",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},

					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op:   "add",
											Path: "/spec/template/0/",
											ValueFrom: &clusterv1.JSONPatchValue{
												Variable: ptr.To("variableName"),
											},
										},
									},
								},
							},
						},
					},
					Variables: []clusterv1.ClusterClassVariable{
						{
							Name:     "variableName",
							Required: true,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "error if jsonPatch path uses an invalid index for add i.e. a number greater than 0.",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},

					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op:   "add",
											Path: "/spec/template/1/",
											ValueFrom: &clusterv1.JSONPatchValue{
												Variable: ptr.To("variableName"),
											},
										},
									},
								},
							},
						},
					},
					Variables: []clusterv1.ClusterClassVariable{
						{
							Name:     "variableName",
							Required: true,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "error if jsonPatch path uses an invalid index for add i.e. 01",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},

					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op:   "add",
											Path: "/spec/template/01/",
											ValueFrom: &clusterv1.JSONPatchValue{
												Variable: ptr.To("variableName"),
											},
										},
									},
								},
							},
						},
					},
					Variables: []clusterv1.ClusterClassVariable{
						{
							Name:     "variableName",
							Required: true,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "error if jsonPatch path uses any index for remove i.e. 0 or -.",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},

					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op:   "remove",
											Path: "/spec/template/0/",
											ValueFrom: &clusterv1.JSONPatchValue{
												Variable: ptr.To("variableName"),
											},
										},
									},
								},
							},
						},
					},
					Variables: []clusterv1.ClusterClassVariable{
						{
							Name:     "variableName",
							Required: true,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "error if jsonPatch path uses any index for replace i.e. 0",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},

					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Path: "/spec/template/0/",
											ValueFrom: &clusterv1.JSONPatchValue{
												Variable: ptr.To("variableName"),
											},
										},
									},
								},
							},
						},
					},
					Variables: []clusterv1.ClusterClassVariable{
						{
							Name:     "variableName",
							Required: true,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},

		// Patch Value/ValueFrom validation
		{
			name: "error if jsonPatch has neither Value nor ValueFrom",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},

					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op:   "add",
											Path: "/spec/template/spec/",
											// Value and ValueFrom not defined.
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "error if jsonPatch has both Value and ValueFrom",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},
					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op:   "add",
											Path: "/spec/template/spec/",
											ValueFrom: &clusterv1.JSONPatchValue{
												Variable: ptr.To("variableName"),
											},
											Value: &apiextensionsv1.JSON{Raw: []byte("1")},
										},
									},
								},
							},
						},
					},
					Variables: []clusterv1.ClusterClassVariable{
						{
							Name:     "variableName",
							Required: true,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},

		// Patch value validation
		{
			name: "pass if jsonPatch value is valid json literal",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},
					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op:    "add",
											Path:  "/spec/template/spec/",
											Value: &apiextensionsv1.JSON{Raw: []byte(`"stringValue"`)},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "pass if jsonPatch value is valid json",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},
					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op:   "add",
											Path: "/spec/template/spec/",
											Value: &apiextensionsv1.JSON{Raw: []byte(
												"{\"id\": \"file\"" +
													"," +
													"\"value\": \"File\"}")},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "pass if jsonPatch value is nil",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},
					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op:   "add",
											Path: "/spec/template/spec/",
											Value: &apiextensionsv1.JSON{
												Raw: nil,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "error if jsonPatch value is invalid json",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},

					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op:   "add",
											Path: "/spec/template/spec/",
											Value: &apiextensionsv1.JSON{Raw: []byte(
												"{\"id\": \"file\"" +
													// missing comma here +
													"\"value\": \"File\"}")},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},

		// Patch valueFrom validation
		{
			name: "error if jsonPatch defines neither ValueFrom.Template nor ValueFrom.Variable",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},
					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op:        "add",
											Path:      "/spec/template/spec/",
											ValueFrom: &clusterv1.JSONPatchValue{},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "error if jsonPatch has both ValueFrom.Template and ValueFrom.Variable",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},
					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op:   "add",
											Path: "/spec/template/spec/",
											ValueFrom: &clusterv1.JSONPatchValue{
												Variable: ptr.To("variableName"),
												Template: ptr.To(`template {{ .variableB }}`),
											},
										},
									},
								},
							},
						},
					},
					Variables: []clusterv1.ClusterClassVariable{
						{
							Name:     "variableName",
							Required: true,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},

		// Patch valueFrom.Template validation
		{
			name: "pass if jsonPatch defines a valid ValueFrom.Template",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},
					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op:   "add",
											Path: "/spec/template/spec/",
											ValueFrom: &clusterv1.JSONPatchValue{
												Template: ptr.To(`template {{ .variableB }}`),
											},
										},
									},
								},
							},
						},
					},
					Variables: []clusterv1.ClusterClassVariable{
						{
							Name:     "variableName",
							Required: true,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "error if jsonPatch defines an invalid ValueFrom.Template",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},
					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op:   "add",
											Path: "/spec/template/spec/",
											ValueFrom: &clusterv1.JSONPatchValue{
												// Template is invalid - too many leading curly braces.
												Template: ptr.To(`template {{{{{{{{ .variableB }}`),
											},
										},
									},
								},
							},
						},
					},
					Variables: []clusterv1.ClusterClassVariable{
						{
							Name:     "variableName",
							Required: true,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},

		// Patch valueFrom.Variable validation
		{
			name: "error if jsonPatch valueFrom uses a variable which is not defined",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},
					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op:   "add",
											Path: "/spec/template/spec/",
											ValueFrom: &clusterv1.JSONPatchValue{
												Variable: ptr.To("undefinedVariable"),
											},
										},
									},
								},
							},
						},
					},
					Variables: []clusterv1.ClusterClassVariable{
						{
							Name:     "variableName",
							Required: true,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "pass if jsonPatch uses a user-defined variable which is defined",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},
					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op:   "add",
											Path: "/spec/template/spec/",
											ValueFrom: &clusterv1.JSONPatchValue{
												Variable: ptr.To("variableName"),
											},
										},
									},
								},
							},
						},
					},
					Variables: []clusterv1.ClusterClassVariable{
						{
							Name:     "variableName",
							Required: true,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "pass if jsonPatch uses a nested user-defined variable which is defined",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},
					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op:   "add",
											Path: "/spec/template/spec/",
											ValueFrom: &clusterv1.JSONPatchValue{
												Variable: ptr.To("variableName.nestedField"),
											},
										},
									},
								},
							},
						},
					},
					Variables: []clusterv1.ClusterClassVariable{
						{
							Name:     "variableName",
							Required: true,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "object",
									Properties: map[string]clusterv1.JSONSchemaProps{
										"nestedField": {
											Type: "string",
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "error if jsonPatch uses a builtin variable which is not defined",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},
					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op:   "add",
											Path: "/spec/template/spec/",
											ValueFrom: &clusterv1.JSONPatchValue{
												Variable: ptr.To("builtin.notDefined"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "pass if jsonPatch uses a builtin variable which is defined",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},

					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
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
											Op:   "add",
											Path: "/spec/template/spec/",
											ValueFrom: &clusterv1.JSONPatchValue{
												Variable: ptr.To("builtin.machineDeployment.version"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},

		// Patch with External
		{
			name: "pass if patch defines both external.generateExtension and external.validateExtension",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},

					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
							External: &clusterv1.ExternalPatchDefinition{
								GenerateExtension: ptr.To("generate-extension"),
								ValidateExtension: ptr.To("generate-extension"),
							},
						},
					},
				},
			},
			runtimeSDK: true,
			wantErr:    false,
		},
		{
			name: "error if patch defines both external and RuntimeSDK is not enabled",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},

					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
							External: &clusterv1.ExternalPatchDefinition{
								GenerateExtension: ptr.To("generate-extension"),
								ValidateExtension: ptr.To("generate-extension"),
							},
						},
					},
				},
			},
			runtimeSDK: false,
			wantErr:    true,
		},
		{
			name: "error if patch defines neither external.generateExtension nor external.validateExtension",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},

					Patches: []clusterv1.ClusterClassPatch{
						{
							Name:     "patch1",
							External: &clusterv1.ExternalPatchDefinition{},
						},
					},
				},
			},
			runtimeSDK: true,
			wantErr:    true,
		},
		{
			name: "error if patch defines both external and definitions",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},

					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
							External: &clusterv1.ExternalPatchDefinition{
								GenerateExtension: ptr.To("generate-extension"),
								ValidateExtension: ptr.To("generate-extension"),
							},
							Definitions: []clusterv1.PatchDefinition{},
						},
					},
				},
			},
			runtimeSDK: true,
			wantErr:    true,
		},
		{
			name: "error if neither external nor definitions is defined",
			clusterClass: clusterv1.ClusterClass{
				Spec: clusterv1.ClusterClassSpec{
					ControlPlane: clusterv1.ControlPlaneClass{
						LocalObjectTemplate: clusterv1.LocalObjectTemplate{
							Ref: &corev1.ObjectReference{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "ControlPlaneTemplate",
							},
						},
					},

					Patches: []clusterv1.ClusterClassPatch{
						{
							Name: "patch1",
						},
					},
				},
			},
			runtimeSDK: true,
			wantErr:    true,
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, tt.runtimeSDK)

			g := NewWithT(t)

			errList := validatePatches(&tt.clusterClass)
			if tt.wantErr {
				g.Expect(errList).NotTo(BeEmpty())
				return
			}
			g.Expect(errList).To(BeEmpty())
		})
	}
}

func Test_validateSelectors(t *testing.T) {
	tests := []struct {
		name         string
		selector     clusterv1.PatchSelector
		clusterClass *clusterv1.ClusterClass
		wantErr      bool
	}{
		{
			name: "error if selectors are all set to false or empty",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureClusterTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					ControlPlane:           false,
					InfrastructureCluster:  false,
					MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{},
					MachinePoolClass:       &clusterv1.PatchSelectorMatchMachinePoolClass{},
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithControlPlaneTemplate(
					refToUnstructured(
						&corev1.ObjectReference{
							APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
							Kind:       "InfrastructureClusterTemplate",
						}),
				).
				Build(),
			wantErr: true,
		},
		{
			name: "pass if selector targets an existing infrastructureCluster reference",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureClusterTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					InfrastructureCluster: true,
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(
						&corev1.ObjectReference{
							APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
							Kind:       "InfrastructureClusterTemplate",
						}),
				).
				Build(),
		},
		{
			name: "error if selector targets a non-existing infrastructureCluster APIVersion reference",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureClusterTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					InfrastructureCluster: true,
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(
						&corev1.ObjectReference{
							APIVersion: "nonmatchinginfrastructure.cluster.x-k8s.io/v1beta1",
							Kind:       "InfrastructureClusterTemplate",
						}),
				).
				Build(),
			wantErr: true,
		},
		{
			name: "pass if selector targets an existing controlPlane reference",
			selector: clusterv1.PatchSelector{
				APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
				Kind:       "ControlPlaneTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					ControlPlane: true,
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithControlPlaneTemplate(
					refToUnstructured(
						&corev1.ObjectReference{
							APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
							Kind:       "ControlPlaneTemplate",
						}),
				).
				Build(),
		},
		{
			name: "error if selector targets a non-existing controlPlane Kind reference",
			selector: clusterv1.PatchSelector{
				APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
				Kind:       "ControlPlaneTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					ControlPlane: true,
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithControlPlaneTemplate(
					refToUnstructured(
						&corev1.ObjectReference{
							APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
							Kind:       "NonMatchingControlPlaneTemplate",
						}),
				).
				Build(),
			wantErr: true,
		},
		{
			name: "pass if selector targets an existing controlPlane machineInfrastructure reference",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureMachineTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					ControlPlane: true,
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithControlPlaneTemplate(
					refToUnstructured(
						&corev1.ObjectReference{
							APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
							Kind:       "NonMatchingControlPlaneTemplate",
						}),
				).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(
						&corev1.ObjectReference{
							APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
							Kind:       "InfrastructureMachineTemplate",
						}),
				).
				Build(),
		},
		{
			name: "error if selector targets a non-existing controlPlane machineInfrastructure reference",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureMachineTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					ControlPlane: true,
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithControlPlaneTemplate(
					refToUnstructured(
						&corev1.ObjectReference{
							APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
							Kind:       "NonMatchingControlPlaneTemplate",
						}),
				).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(
						&corev1.ObjectReference{
							APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
							Kind:       "NonMatchingInfrastructureMachineTemplate",
						}),
				).
				Build(),
			wantErr: true,
		},
		{
			name: "pass if selector targets an existing MachineDeploymentClass and MachinePoolClass BootstrapTemplate",
			selector: clusterv1.PatchSelector{
				APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
				Kind:       "BootstrapTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
						Names: []string{"aa"},
					},
					MachinePoolClass: &clusterv1.PatchSelectorMatchMachinePoolClass{
						Names: []string{"aa"},
					},
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "InfrastructureMachineTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
				).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "InfrastructureMachinePoolTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
				).
				Build(),
		},
		{
			name: "pass if selector targets an existing MachineDeploymentClass InfrastructureTemplate",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureMachineTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
						Names: []string{"aa"},
					},
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "InfrastructureMachineTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
				).
				Build(),
		},
		{
			name: "pass if selector targets an existing MachinePoolClass InfrastructureTemplate",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureMachinePoolTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					MachinePoolClass: &clusterv1.PatchSelectorMatchMachinePoolClass{
						Names: []string{"aa"},
					},
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "InfrastructureMachinePoolTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
				).
				Build(),
		},
		{
			name: "error if selector targets a non-existing MachineDeploymentClass InfrastructureTemplate",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureMachineTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
						Names: []string{"bb"},
					},
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "InfrastructureMachineTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "NonMatchingInfrastructureMachineTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
				).
				Build(),
			wantErr: true,
		},
		{
			name: "error if selector targets a non-existing MachinePoolClass InfrastructureTemplate",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureMachinePoolTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					MachinePoolClass: &clusterv1.PatchSelectorMatchMachinePoolClass{
						Names: []string{"bb"},
					},
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "InfrastructureMachinePoolTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
					*builder.MachinePoolClass("bb").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "NonMatchingInfrastructureMachinePoolTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
				).
				Build(),
			wantErr: true,
		},
		{
			name: "fail if selector targets ControlPlane Machine Infrastructure but does not have MatchResources.ControlPlane enabled",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureMachineTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
						Names: []string{"bb"},
					},
					ControlPlane: false,
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(
						&corev1.ObjectReference{
							APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
							Kind:       "InfrastructureMachineTemplate",
						}),
				).
				Build(),
			wantErr: true,
		},
		{
			name: "error if selector targets an empty MachineDeploymentClass InfrastructureTemplate",
			selector: clusterv1.PatchSelector{
				APIVersion:     "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:           "InfrastructureMachineTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "InfrastructureMachineTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "NonMatchingInfrastructureMachineTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
				).
				Build(),
			wantErr: true,
		},
		{
			name: "error if selector targets an empty MachinePoolClass InfrastructureTemplate",
			selector: clusterv1.PatchSelector{
				APIVersion:     "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:           "InfrastructureMachinePoolTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "InfrastructureMachinePoolTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
					*builder.MachinePoolClass("bb").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "NonMatchingInfrastructureMachinePoolTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
				).
				Build(),
			wantErr: true,
		},
		{
			name: "error if selector targets a bad pattern for matching MachineDeploymentClass InfrastructureTemplate",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureMachineTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
						Names: []string{"a*a"},
					},
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("a-something-a").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "InfrastructureMachineTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
				).
				Build(),
			wantErr: true,
		},
		{
			name: "error if selector targets a bad pattern for matching MachinePoolClass InfrastructureTemplate",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureMachinePoolTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					MachinePoolClass: &clusterv1.PatchSelectorMatchMachinePoolClass{
						Names: []string{"a*a"},
					},
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("a-something-a").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "InfrastructureMachinePoolTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
				).
				Build(),
			wantErr: true,
		},
		{
			name: "pass if selector targets an existing MachineDeploymentClass InfrastructureTemplate with prefix *",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureMachineTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
						Names: []string{"a-*"},
					},
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("a-something-a").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "InfrastructureMachineTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
				).
				Build(),
		},
		{
			name: "pass if selector targets an existing MachinePoolClass InfrastructureTemplate with prefix *",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureMachinePoolTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					MachinePoolClass: &clusterv1.PatchSelectorMatchMachinePoolClass{
						Names: []string{"a-*"},
					},
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("a-something-a").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "InfrastructureMachinePoolTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
				).
				Build(),
		},
		{
			name: "pass if selector targets an existing MachineDeploymentClass InfrastructureTemplate with suffix *",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureMachineTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
						Names: []string{"*-a"},
					},
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("a-something-a").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "InfrastructureMachineTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
				).
				Build(),
		},
		{
			name: "pass if selector targets an existing MachinePoolClass InfrastructureTemplate with suffix *",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureMachinePoolTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					MachinePoolClass: &clusterv1.PatchSelectorMatchMachinePoolClass{
						Names: []string{"*-a"},
					},
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("a-something-a").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "InfrastructureMachinePoolTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
				).
				Build(),
		},
		{
			name: "pass if selector targets all existing MachineDeploymentClass InfrastructureTemplate with *",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureMachineTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
						Names: []string{"*"},
					},
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("a-something-a").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "InfrastructureMachineTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
				).
				Build(),
		},
		{
			name: "pass if selector targets all existing MachinePoolClass InfrastructureTemplate with *",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureMachinePoolTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					MachinePoolClass: &clusterv1.PatchSelectorMatchMachinePoolClass{
						Names: []string{"*"},
					},
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("a-something-a").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "InfrastructureMachinePoolTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
				).
				Build(),
		},
		// The following tests have selectors which match multiple resources at the same time.
		{
			name: "fail if selector targets a matching infrastructureCluster reference and a not matching control plane",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureClusterTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					InfrastructureCluster: true,
					ControlPlane:          true,
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(
						&corev1.ObjectReference{
							APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
							Kind:       "InfrastructureClusterTemplate",
						}),
				).
				WithControlPlaneTemplate(
					refToUnstructured(
						&corev1.ObjectReference{
							APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
							Kind:       "NonMatchingControlPlaneTemplate",
						}),
				).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(
						&corev1.ObjectReference{
							APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
							Kind:       "NonMatchingInfrastructureMachineTemplate",
						}),
				).
				Build(),
			wantErr: true,
		},
		{
			name: "pass if selector targets BOTH an existing ControlPlane MachineInfrastructureTemplate and an existing MachineDeploymentClass InfrastructureTemplate",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureMachineTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
						Names: []string{"bb"},
					},
					ControlPlane: true,
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(
						&corev1.ObjectReference{
							APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
							Kind:       "InfrastructureMachineTemplate",
						}),
				).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "InfrastructureMachineTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "InfrastructureMachineTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
				).
				Build(),
			wantErr: false,
		},
		{
			name: "fail if selector targets BOTH an existing ControlPlane MachineInfrastructureTemplate and an existing MachineDeploymentClass InfrastructureTemplate but does not match all MachineDeployment classes",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureMachineTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
						Names: []string{"aa", "bb"},
					},
					ControlPlane: true,
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithControlPlaneTemplate(
					refToUnstructured(
						&corev1.ObjectReference{
							APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
							Kind:       "NonMatchingControlPlaneTemplate",
						}),
				).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(
						&corev1.ObjectReference{
							APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
							Kind:       "InfrastructureMachineTemplate",
						}),
				).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "NonMatchingInfrastructureMachineTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "InfrastructureMachineTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
				).
				Build(),
			wantErr: true,
		},
		{
			name: "fail if selector targets BOTH an existing ControlPlane MachineInfrastructureTemplate and an existing MachineDeploymentClass InfrastructureTemplate but matches only one",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "InfrastructureMachineTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
						Names: []string{"bb"},
					},
					ControlPlane: true,
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithControlPlaneTemplate(
					refToUnstructured(
						&corev1.ObjectReference{
							APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
							Kind:       "NonMatchingControlPlaneTemplate",
						}),
				).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(
						&corev1.ObjectReference{
							APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
							Kind:       "InfrastructureMachineTemplate",
						}),
				).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "OtherInfrastructureMachineTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
				).
				Build(),
			wantErr: true,
		},
		{
			name: "fail if selector targets everything but nothing matches",
			selector: clusterv1.PatchSelector{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "NotMatchingInfrastructureMachineTemplate",
				MatchResources: clusterv1.PatchSelectorMatch{
					ControlPlane:          true,
					InfrastructureCluster: true,
					MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
						Names: []string{"bb"},
					},
				},
			},
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(
						&corev1.ObjectReference{
							APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
							Kind:       "InfrastructureClusterTemplate",
						}),
				).
				WithControlPlaneTemplate(
					refToUnstructured(
						&corev1.ObjectReference{
							APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
							Kind:       "NonMatchingControlPlaneTemplate",
						}),
				).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(
						&corev1.ObjectReference{
							APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
							Kind:       "InfrastructureMachineTemplate",
						}),
				).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
								Kind:       "OtherInfrastructureMachineTemplate",
							})).
						WithBootstrapTemplate(
							refToUnstructured(&corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "BootstrapTemplate",
							})).
						Build(),
				).
				Build(),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := validateSelectors(tt.selector, tt.clusterClass, field.NewPath(""))

			if tt.wantErr {
				g.Expect(err.ToAggregate()).To(HaveOccurred())
				return
			}
			g.Expect(err.ToAggregate()).ToNot(HaveOccurred())
		})
	}
}

func TestGetVariableName(t *testing.T) {
	tests := []struct {
		name         string
		variable     string
		variableName string
	}{
		{
			name:         "simple variable",
			variable:     "variableA",
			variableName: "variableA",
		},
		{
			name:         "variable object",
			variable:     "variableObject.field",
			variableName: "variableObject",
		},
		{
			name:         "variable array",
			variable:     "variableArray[0]",
			variableName: "variableArray",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(getVariableName(tt.variable)).To(Equal(tt.variableName))
		})
	}
}
