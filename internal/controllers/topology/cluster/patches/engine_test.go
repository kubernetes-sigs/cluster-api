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

package patches

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/exp/topology/scope"
	"sigs.k8s.io/cluster-api/feature"
	fakeruntimeclient "sigs.k8s.io/cluster-api/internal/runtime/client/fake"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestApply(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, true)
	type expectedFields struct {
		infrastructureCluster                          map[string]interface{}
		controlPlane                                   map[string]interface{}
		controlPlaneInfrastructureMachineTemplate      map[string]interface{}
		machineDeploymentBootstrapTemplate             map[string]map[string]interface{}
		machineDeploymentInfrastructureMachineTemplate map[string]map[string]interface{}
		machinePoolBootstrapConfig                     map[string]map[string]interface{}
		machinePoolInfrastructureMachinePool           map[string]map[string]interface{}
	}

	tests := []struct {
		name                   string
		patches                []clusterv1.ClusterClassPatch
		varDefinitions         []clusterv1.ClusterClassStatusVariable
		externalPatchResponses map[string]runtimehooksv1.ResponseObject
		expectedFields         expectedFields
		wantErr                bool
	}{
		{
			name: "Should preserve desired state, if there are no patches",
			// No changes expected.
			expectedFields: expectedFields{},
		},
		{
			name: "Should apply JSON patches to InfraCluster, ControlPlane and ControlPlaneInfrastructureMachineTemplate",
			patches: []clusterv1.ClusterClassPatch{
				{
					Name: "fake-patch1",
					Definitions: []clusterv1.PatchDefinition{
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.InfrastructureGroupVersion.String(),
								Kind:       builder.GenericInfrastructureClusterTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									InfrastructureCluster: true,
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:    "add",
									Path:  "/spec/template/spec/resource",
									Value: &apiextensionsv1.JSON{Raw: []byte(`"infraCluster"`)},
								},
							},
						},
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.ControlPlaneGroupVersion.String(),
								Kind:       builder.GenericControlPlaneTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									ControlPlane: true,
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:    "add",
									Path:  "/spec/template/spec/resource",
									Value: &apiextensionsv1.JSON{Raw: []byte(`"controlPlane"`)},
								},
							},
						},
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.InfrastructureGroupVersion.String(),
								Kind:       builder.GenericInfrastructureMachineTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									ControlPlane: true,
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:    "add",
									Path:  "/spec/template/spec/resource",
									Value: &apiextensionsv1.JSON{Raw: []byte(`"controlPlaneInfrastructureMachineTemplate"`)},
								},
							},
						},
					},
				},
			},
			expectedFields: expectedFields{
				infrastructureCluster: map[string]interface{}{
					"spec.resource": "infraCluster",
				},
				controlPlane: map[string]interface{}{
					"spec.resource": "controlPlane",
				},
				controlPlaneInfrastructureMachineTemplate: map[string]interface{}{
					"spec.template.spec.resource": "controlPlaneInfrastructureMachineTemplate",
				},
			},
		},
		{
			name: "Should apply JSON patches to MachineDeployment and MachinePool templates",
			patches: []clusterv1.ClusterClassPatch{
				{
					Name: "fake-patch1",
					Definitions: []clusterv1.PatchDefinition{
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.InfrastructureGroupVersion.String(),
								Kind:       builder.GenericInfrastructureMachineTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
										Names: []string{"default-worker"},
									},
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:    "add",
									Path:  "/spec/template/spec/resource",
									Value: &apiextensionsv1.JSON{Raw: []byte(`"default-worker-infra"`)},
								},
							},
						},
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.BootstrapGroupVersion.String(),
								Kind:       builder.GenericBootstrapConfigTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
										Names: []string{"default-worker"},
									},
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:    "add",
									Path:  "/spec/template/spec/resource",
									Value: &apiextensionsv1.JSON{Raw: []byte(`"default-worker-bootstrap"`)},
								},
							},
						},
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.InfrastructureGroupVersion.String(),
								Kind:       builder.GenericInfrastructureMachinePoolTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									MachinePoolClass: &clusterv1.PatchSelectorMatchMachinePoolClass{
										Names: []string{"default-mp-worker"},
									},
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:    "add",
									Path:  "/spec/template/spec/resource",
									Value: &apiextensionsv1.JSON{Raw: []byte(`"default-mp-worker-infra"`)},
								},
							},
						},
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.BootstrapGroupVersion.String(),
								Kind:       builder.GenericBootstrapConfigTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									MachinePoolClass: &clusterv1.PatchSelectorMatchMachinePoolClass{
										Names: []string{"default-mp-worker"},
									},
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:    "add",
									Path:  "/spec/template/spec/resource",
									Value: &apiextensionsv1.JSON{Raw: []byte(`"default-mp-worker-bootstrap"`)},
								},
							},
						},
					},
				},
			},
			expectedFields: expectedFields{
				machineDeploymentBootstrapTemplate: map[string]map[string]interface{}{
					"default-worker-topo1": {"spec.template.spec.resource": "default-worker-bootstrap"},
					"default-worker-topo2": {"spec.template.spec.resource": "default-worker-bootstrap"},
				},
				machineDeploymentInfrastructureMachineTemplate: map[string]map[string]interface{}{
					"default-worker-topo1": {"spec.template.spec.resource": "default-worker-infra"},
					"default-worker-topo2": {"spec.template.spec.resource": "default-worker-infra"},
				},
				machinePoolBootstrapConfig: map[string]map[string]interface{}{
					"default-mp-worker-topo1": {"spec.resource": "default-mp-worker-bootstrap"},
					"default-mp-worker-topo2": {"spec.resource": "default-mp-worker-bootstrap"},
				},
				machinePoolInfrastructureMachinePool: map[string]map[string]interface{}{
					"default-mp-worker-topo1": {"spec.resource": "default-mp-worker-infra"},
					"default-mp-worker-topo2": {"spec.resource": "default-mp-worker-infra"},
				},
			},
		},
		{
			name: "Should apply JSON patches in the correct order",
			patches: []clusterv1.ClusterClassPatch{
				{
					Name: "fake-patch1",
					Definitions: []clusterv1.PatchDefinition{
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.ControlPlaneGroupVersion.String(),
								Kind:       builder.GenericControlPlaneTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									ControlPlane: true,
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:    "add",
									Path:  "/spec/template/spec/clusterName",
									Value: &apiextensionsv1.JSON{Raw: []byte(`"cluster1"`)},
								},
								{
									Op:    "add",
									Path:  "/spec/template/spec/files",
									Value: &apiextensionsv1.JSON{Raw: []byte(`[{"key1":"value1"}]`)},
								},
							},
						},
					},
				},
				{
					Name: "fake-patch2",
					Definitions: []clusterv1.PatchDefinition{
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.ControlPlaneGroupVersion.String(),
								Kind:       builder.GenericControlPlaneTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									ControlPlane: true,
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:    "replace",
									Path:  "/spec/template/spec/clusterName",
									Value: &apiextensionsv1.JSON{Raw: []byte(`"cluster1-overwritten"`)},
								},
							},
						},
					},
				},
			},
			expectedFields: expectedFields{
				controlPlane: map[string]interface{}{
					"spec.clusterName": "cluster1-overwritten",
					"spec.files": []interface{}{
						map[string]interface{}{
							"key1": "value1",
						},
					},
				},
			},
		},
		{
			name: "Should apply JSON patches and preserve ControlPlane fields",
			patches: []clusterv1.ClusterClassPatch{
				{
					Name: "fake-patch1",
					Definitions: []clusterv1.PatchDefinition{
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.ControlPlaneGroupVersion.String(),
								Kind:       builder.GenericControlPlaneTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									ControlPlane: true,
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:    "add",
									Path:  "/spec/template/spec/replicas",
									Value: &apiextensionsv1.JSON{Raw: []byte(`1`)},
								},
								{
									Op:    "add",
									Path:  "/spec/template/spec/version",
									Value: &apiextensionsv1.JSON{Raw: []byte(`"v1.15.0"`)},
								},
								{
									Op:    "add",
									Path:  "/spec/template/spec/machineTemplate/infrastructureRef",
									Value: &apiextensionsv1.JSON{Raw: []byte(`{"apiVersion":"invalid","kind":"invalid","namespace":"invalid","name":"invalid"}`)},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Should apply JSON patches without metadata",
			patches: []clusterv1.ClusterClassPatch{
				{
					Name: "fake-patch1",
					Definitions: []clusterv1.PatchDefinition{
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.InfrastructureGroupVersion.String(),
								Kind:       builder.GenericInfrastructureClusterTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									InfrastructureCluster: true,
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:    "add",
									Path:  "/spec/template/spec/clusterName",
									Value: &apiextensionsv1.JSON{Raw: []byte(`"cluster1"`)},
								},
								{
									Op:    "replace",
									Path:  "/metadata/name",
									Value: &apiextensionsv1.JSON{Raw: []byte(`"overwrittenName"`)},
								},
							},
						},
					},
				},
			},
			expectedFields: expectedFields{
				infrastructureCluster: map[string]interface{}{
					"spec.clusterName": "cluster1",
				},
			},
		},
		{
			name: "Should apply JSON merge patches",
			patches: []clusterv1.ClusterClassPatch{
				{
					Name: "fake-patch1",
					Definitions: []clusterv1.PatchDefinition{
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.InfrastructureGroupVersion.String(),
								Kind:       builder.GenericInfrastructureClusterTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									InfrastructureCluster: true,
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:    "add",
									Path:  "/spec/template/spec/resource",
									Value: &apiextensionsv1.JSON{Raw: []byte(`"infraCluster"`)},
								},
							},
						},
					},
				},
			},
			expectedFields: expectedFields{
				infrastructureCluster: map[string]interface{}{
					"spec.resource": "infraCluster",
				},
			},
		},
		{
			name: "Successfully apply external jsonPatch with generate and validate",
			patches: []clusterv1.ClusterClassPatch{
				{
					Name: "fake-patch1",
					External: &clusterv1.ExternalPatchDefinition{
						GenerateExtension: ptr.To("patch-infrastructureCluster"),
						ValidateExtension: ptr.To("validate-infrastructureCluster"),
					},
				},
			},
			externalPatchResponses: map[string]runtimehooksv1.ResponseObject{
				"patch-infrastructureCluster": &runtimehooksv1.GeneratePatchesResponse{
					Items: []runtimehooksv1.GeneratePatchesResponseItem{
						{
							UID:       "1",
							PatchType: runtimehooksv1.JSONPatchType,
							Patch: bytesPatch([]jsonPatchRFC6902{{
								Op:    "add",
								Path:  "/spec/template/spec/resource",
								Value: &apiextensionsv1.JSON{Raw: []byte(`"infraCluster"`)},
							}}),
						},
					},
				},
				"validate-infrastructureCluster": &runtimehooksv1.ValidateTopologyResponse{
					CommonResponse: runtimehooksv1.CommonResponse{
						Status: runtimehooksv1.ResponseStatusSuccess,
					},
				},
			},
			expectedFields: expectedFields{
				infrastructureCluster: map[string]interface{}{
					"spec.resource": "infraCluster",
				},
			},
		},
		{
			name: "error on failed validation with external jsonPatch",
			patches: []clusterv1.ClusterClassPatch{
				{
					Name: "fake-patch1",
					External: &clusterv1.ExternalPatchDefinition{
						GenerateExtension: ptr.To("patch-infrastructureCluster"),
						ValidateExtension: ptr.To("validate-infrastructureCluster"),
					},
				},
			},
			externalPatchResponses: map[string]runtimehooksv1.ResponseObject{
				"patch-infrastructureCluster": &runtimehooksv1.GeneratePatchesResponse{
					Items: []runtimehooksv1.GeneratePatchesResponseItem{
						{
							UID:       "1",
							PatchType: runtimehooksv1.JSONPatchType,
							Patch: bytesPatch([]jsonPatchRFC6902{{
								Op:    "add",
								Path:  "/spec/template/spec/resource",
								Value: &apiextensionsv1.JSON{Raw: []byte(`"invalid-infraCluster"`)},
							}}),
						},
					},
				},
				"validate-infrastructureCluster": &runtimehooksv1.ValidateTopologyResponse{
					CommonResponse: runtimehooksv1.CommonResponse{
						Status:  runtimehooksv1.ResponseStatusFailure,
						Message: "not a valid infrastructureCluster",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Successfully apply multiple external jsonPatch",
			patches: []clusterv1.ClusterClassPatch{
				{
					Name: "fake-patch1",
					External: &clusterv1.ExternalPatchDefinition{
						GenerateExtension: ptr.To("patch-infrastructureCluster"),
					},
				},
				{
					Name: "fake-patch2",
					External: &clusterv1.ExternalPatchDefinition{
						GenerateExtension: ptr.To("patch-controlPlane"),
					},
				},
			},

			externalPatchResponses: map[string]runtimehooksv1.ResponseObject{
				"patch-infrastructureCluster": &runtimehooksv1.GeneratePatchesResponse{
					Items: []runtimehooksv1.GeneratePatchesResponseItem{
						{
							UID:       "1",
							PatchType: runtimehooksv1.JSONPatchType,
							Patch: bytesPatch([]jsonPatchRFC6902{{
								Op:    "add",
								Path:  "/spec/template/spec/resource",
								Value: &apiextensionsv1.JSON{Raw: []byte(`"infraCluster"`)},
							}}),
						},
						{
							UID:       "1",
							PatchType: runtimehooksv1.JSONPatchType,
							Patch: bytesPatch([]jsonPatchRFC6902{{
								Op:    "add",
								Path:  "/spec/template/spec/another",
								Value: &apiextensionsv1.JSON{Raw: []byte(`"resource"`)},
							}}),
						},
					},
				},
				"patch-controlPlane": &runtimehooksv1.GeneratePatchesResponse{
					Items: []runtimehooksv1.GeneratePatchesResponseItem{
						{
							UID:       "2",
							PatchType: runtimehooksv1.JSONMergePatchType,
							Patch:     []byte(`{"spec":{"template":{"spec":{"resource": "controlPlane"}}}}`),
						},
					},
				},
			},
			expectedFields: expectedFields{
				infrastructureCluster: map[string]interface{}{
					"spec.resource": "infraCluster",
					"spec.another":  "resource",
				},
				controlPlane: map[string]interface{}{
					"spec.resource": "controlPlane",
				},
			},
		},
		{
			name: "Should correctly apply patches with builtin variables",
			patches: []clusterv1.ClusterClassPatch{
				{
					Name: "fake-patch1",
					Definitions: []clusterv1.PatchDefinition{
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.InfrastructureGroupVersion.String(),
								Kind:       builder.GenericInfrastructureClusterTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									InfrastructureCluster: true,
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:   "add",
									Path: "/spec/template/spec/clusterName",
									ValueFrom: &clusterv1.JSONPatchValue{
										Variable: ptr.To("builtin.cluster.name"),
									},
								},
							},
						},
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.ControlPlaneGroupVersion.String(),
								Kind:       builder.GenericControlPlaneTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									ControlPlane: true,
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:   "add",
									Path: "/spec/template/spec/controlPlaneName",
									ValueFrom: &clusterv1.JSONPatchValue{
										Variable: ptr.To("builtin.controlPlane.name"),
									},
								},
							},
						},
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.InfrastructureGroupVersion.String(),
								Kind:       builder.GenericInfrastructureMachineTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									ControlPlane: true,
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:   "add",
									Path: "/spec/template/spec/controlPlaneName",
									ValueFrom: &clusterv1.JSONPatchValue{
										Variable: ptr.To("builtin.controlPlane.name"),
									},
								},
							},
						},
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.BootstrapGroupVersion.String(),
								Kind:       builder.GenericBootstrapConfigTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
										Names: []string{"default-worker"},
									},
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:   "add",
									Path: "/spec/template/spec/machineDeploymentTopologyName",
									ValueFrom: &clusterv1.JSONPatchValue{
										Variable: ptr.To("builtin.machineDeployment.topologyName"),
									},
								},
							},
						},
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.InfrastructureGroupVersion.String(),
								Kind:       builder.GenericInfrastructureMachineTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
										Names: []string{"default-worker"},
									},
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:   "add",
									Path: "/spec/template/spec/machineDeploymentTopologyName",
									ValueFrom: &clusterv1.JSONPatchValue{
										Variable: ptr.To("builtin.machineDeployment.topologyName"),
									},
								},
							},
						},
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.BootstrapGroupVersion.String(),
								Kind:       builder.GenericBootstrapConfigTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									MachinePoolClass: &clusterv1.PatchSelectorMatchMachinePoolClass{
										Names: []string{"default-mp-worker"},
									},
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:   "add",
									Path: "/spec/template/spec/machinePoolTopologyName",
									ValueFrom: &clusterv1.JSONPatchValue{
										Variable: ptr.To("builtin.machinePool.topologyName"),
									},
								},
							},
						},
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.InfrastructureGroupVersion.String(),
								Kind:       builder.GenericInfrastructureMachinePoolTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									MachinePoolClass: &clusterv1.PatchSelectorMatchMachinePoolClass{
										Names: []string{"default-mp-worker"},
									},
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:   "add",
									Path: "/spec/template/spec/machinePoolTopologyName",
									ValueFrom: &clusterv1.JSONPatchValue{
										Variable: ptr.To("builtin.machinePool.topologyName"),
									},
								},
							},
						},
					},
				},
			},
			expectedFields: expectedFields{
				infrastructureCluster: map[string]interface{}{
					"spec.clusterName": "cluster1",
				},
				controlPlane: map[string]interface{}{
					"spec.controlPlaneName": "controlPlane1",
				},
				controlPlaneInfrastructureMachineTemplate: map[string]interface{}{
					"spec.template.spec.controlPlaneName": "controlPlane1",
				},
				machineDeploymentInfrastructureMachineTemplate: map[string]map[string]interface{}{
					"default-worker-topo1": {"spec.template.spec.machineDeploymentTopologyName": "default-worker-topo1"},
					"default-worker-topo2": {"spec.template.spec.machineDeploymentTopologyName": "default-worker-topo2"},
				},
				machineDeploymentBootstrapTemplate: map[string]map[string]interface{}{
					"default-worker-topo1": {"spec.template.spec.machineDeploymentTopologyName": "default-worker-topo1"},
					"default-worker-topo2": {"spec.template.spec.machineDeploymentTopologyName": "default-worker-topo2"},
				},
				machinePoolInfrastructureMachinePool: map[string]map[string]interface{}{
					"default-mp-worker-topo1": {"spec.machinePoolTopologyName": "default-mp-worker-topo1"},
					"default-mp-worker-topo2": {"spec.machinePoolTopologyName": "default-mp-worker-topo2"},
				},
				machinePoolBootstrapConfig: map[string]map[string]interface{}{
					"default-mp-worker-topo1": {"spec.machinePoolTopologyName": "default-mp-worker-topo1"},
					"default-mp-worker-topo2": {"spec.machinePoolTopologyName": "default-mp-worker-topo2"},
				},
			},
		},
		{
			name: "Should correctly apply variables with variables without conflicts",
			varDefinitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "controlPlaneVariable",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							From: "inline",
						},
					},
				},
				{
					Name: "defaultMDWorkerVariable",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							From: "inline",
						},
					},
				},
				{
					Name: "defaultMPWorkerVariable",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							From: "inline",
						},
					},
				},
				{
					Name: "infraCluster",
					// Note: This variable is defined by multiple patches, but without conflicts.
					DefinitionsConflict: false,
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							From: "inline",
						},
						{
							From: "not-used-patch",
						},
					},
				},
			},
			patches: []clusterv1.ClusterClassPatch{
				{
					Name: "fake-patch1",
					Definitions: []clusterv1.PatchDefinition{
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.InfrastructureGroupVersion.String(),
								Kind:       builder.GenericInfrastructureClusterTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									InfrastructureCluster: true,
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:   "add",
									Path: "/spec/template/spec/resource",
									ValueFrom: &clusterv1.JSONPatchValue{
										Variable: ptr.To("infraCluster"),
									},
								},
							},
						},
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.ControlPlaneGroupVersion.String(),
								Kind:       builder.GenericControlPlaneTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									ControlPlane: true,
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:   "add",
									Path: "/spec/template/spec/controlPlaneField",
									ValueFrom: &clusterv1.JSONPatchValue{
										Variable: ptr.To("controlPlaneVariable"),
									},
								},
							},
						},
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.InfrastructureGroupVersion.String(),
								Kind:       builder.GenericInfrastructureMachineTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									ControlPlane: true,
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:   "add",
									Path: "/spec/template/spec/controlPlaneInfraMachineTemplateField",
									ValueFrom: &clusterv1.JSONPatchValue{
										Variable: ptr.To("controlPlaneVariable"),
									},
								},
							},
						},
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.BootstrapGroupVersion.String(),
								Kind:       builder.GenericBootstrapConfigTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
										Names: []string{"default-worker"},
									},
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:   "add",
									Path: "/spec/template/spec/resource",
									ValueFrom: &clusterv1.JSONPatchValue{
										Variable: ptr.To("defaultMDWorkerVariable"),
									},
								},
							},
						},
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.InfrastructureGroupVersion.String(),
								Kind:       builder.GenericInfrastructureMachineTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									MachineDeploymentClass: &clusterv1.PatchSelectorMatchMachineDeploymentClass{
										Names: []string{"default-worker"},
									},
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:   "add",
									Path: "/spec/template/spec/resource",
									ValueFrom: &clusterv1.JSONPatchValue{
										Variable: ptr.To("defaultMDWorkerVariable"),
									},
								},
							},
						},
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.BootstrapGroupVersion.String(),
								Kind:       builder.GenericBootstrapConfigTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									MachinePoolClass: &clusterv1.PatchSelectorMatchMachinePoolClass{
										Names: []string{"default-mp-worker"},
									},
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:   "add",
									Path: "/spec/template/spec/resource",
									ValueFrom: &clusterv1.JSONPatchValue{
										Variable: ptr.To("defaultMPWorkerVariable"),
									},
								},
							},
						},
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: builder.InfrastructureGroupVersion.String(),
								Kind:       builder.GenericInfrastructureMachinePoolTemplateKind,
								MatchResources: clusterv1.PatchSelectorMatch{
									MachinePoolClass: &clusterv1.PatchSelectorMatchMachinePoolClass{
										Names: []string{"default-mp-worker"},
									},
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:   "add",
									Path: "/spec/template/spec/resource",
									ValueFrom: &clusterv1.JSONPatchValue{
										Variable: ptr.To("defaultMPWorkerVariable"),
									},
								},
							},
						},
					},
				},
			},
			expectedFields: expectedFields{
				infrastructureCluster: map[string]interface{}{
					"spec.resource": "value99",
				},
				controlPlane: map[string]interface{}{
					"spec.controlPlaneField": "control-plane-override-value",
				},
				controlPlaneInfrastructureMachineTemplate: map[string]interface{}{
					"spec.template.spec.controlPlaneInfraMachineTemplateField": "control-plane-override-value",
				},
				machineDeploymentInfrastructureMachineTemplate: map[string]map[string]interface{}{
					"default-worker-topo1": {"spec.template.spec.resource": "default-worker-topo1-override-value"},
					"default-worker-topo2": {"spec.template.spec.resource": "default-md-cluster-wide-value"},
				},
				machineDeploymentBootstrapTemplate: map[string]map[string]interface{}{
					"default-worker-topo1": {"spec.template.spec.resource": "default-worker-topo1-override-value"},
					"default-worker-topo2": {"spec.template.spec.resource": "default-md-cluster-wide-value"},
				},
				machinePoolInfrastructureMachinePool: map[string]map[string]interface{}{
					"default-mp-worker-topo1": {"spec.resource": "default-mp-worker-topo1-override-value"},
					"default-mp-worker-topo2": {"spec.resource": "default-mp-cluster-wide-value"},
				},
				machinePoolBootstrapConfig: map[string]map[string]interface{}{
					"default-mp-worker-topo1": {"spec.resource": "default-mp-worker-topo1-override-value"},
					"default-mp-worker-topo2": {"spec.resource": "default-mp-cluster-wide-value"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Set up test objects, which are:
			// * blueprint:
			//   * A ClusterClass with its corresponding templates:
			//     * ControlPlaneTemplate with a corresponding ControlPlane InfrastructureMachineTemplate.
			//     * MachineDeploymentClass "default-worker" with corresponding BootstrapTemplate and InfrastructureMachineTemplate.
			//     * MachinePoolClass "default-mp-worker" with corresponding BootstrapTemplate and InfrastructureMachinePoolTemplate.
			//   * The corresponding Cluster.spec.topology:
			//     * with 3 ControlPlane replicas
			//     * with a "default-worker-topo1" MachineDeploymentTopology without replicas (based on "default-worker")
			//     * with a "default-mp-worker-topo1" MachinePoolTopology without replicas (based on "default-mp-worker")
			//     * with a "default-worker-topo2" MachineDeploymentTopology with 3 replicas (based on "default-worker")
			//     * with a "default-mp-worker-topo2" MachinePoolTopology with 3 replicas (based on "default-mp-worker")
			// * desired: essentially the corresponding desired objects.
			blueprint, desired := setupTestObjects()

			// If there are patches, set up patch generators.
			cat := runtimecatalog.New()
			g.Expect(runtimehooksv1.AddToCatalog(cat)).To(Succeed())

			runtimeClient := fakeruntimeclient.NewRuntimeClientBuilder().WithCatalog(cat).Build()

			if tt.externalPatchResponses != nil {
				// replace the package variable uuidGenerator with one that returns an incremented integer.
				// each patch will have a new uuid in the order in which they're defined and called.
				var uuid int32
				uuidGenerator = func() types.UID {
					uuid++
					return types.UID(fmt.Sprintf("%d", uuid))
				}
				runtimeClient = fakeruntimeclient.NewRuntimeClientBuilder().
					WithCallExtensionResponses(tt.externalPatchResponses).
					WithCatalog(cat).
					Build()
			}
			patchEngine := NewEngine(runtimeClient)

			if len(tt.patches) > 0 {
				// Add the patches.
				blueprint.ClusterClass.Spec.Patches = tt.patches
			}
			if len(tt.varDefinitions) > 0 {
				// If there are variable definitions in the test add them to the ClusterClass.
				blueprint.ClusterClass.Status.Variables = tt.varDefinitions
			}

			// Copy the desired objects before applying patches.
			expectedCluster := desired.Cluster.DeepCopy()
			expectedInfrastructureCluster := desired.InfrastructureCluster.DeepCopy()
			expectedControlPlane := desired.ControlPlane.Object.DeepCopy()
			expectedControlPlaneInfrastructureMachineTemplate := desired.ControlPlane.InfrastructureMachineTemplate.DeepCopy()
			expectedBootstrapTemplates := map[string]*unstructured.Unstructured{}
			expectedInfrastructureMachineTemplate := map[string]*unstructured.Unstructured{}
			for mdTopology, md := range desired.MachineDeployments {
				expectedBootstrapTemplates[mdTopology] = md.BootstrapTemplate.DeepCopy()
				expectedInfrastructureMachineTemplate[mdTopology] = md.InfrastructureMachineTemplate.DeepCopy()
			}
			expectedBootstrapConfig := map[string]*unstructured.Unstructured{}
			expectedInfrastructureMachinePool := map[string]*unstructured.Unstructured{}
			for mpTopology, mp := range desired.MachinePools {
				expectedBootstrapConfig[mpTopology] = mp.BootstrapObject.DeepCopy()
				expectedInfrastructureMachinePool[mpTopology] = mp.InfrastructureMachinePoolObject.DeepCopy()
			}

			// Set expected fields on the copy of the objects, so they can be used for comparison with the result of Apply.
			if tt.expectedFields.infrastructureCluster != nil {
				setSpecFields(expectedInfrastructureCluster, tt.expectedFields.infrastructureCluster)
			}
			if tt.expectedFields.controlPlane != nil {
				setSpecFields(expectedControlPlane, tt.expectedFields.controlPlane)
			}
			if tt.expectedFields.controlPlaneInfrastructureMachineTemplate != nil {
				setSpecFields(expectedControlPlaneInfrastructureMachineTemplate, tt.expectedFields.controlPlaneInfrastructureMachineTemplate)
			}
			for mdTopology, expectedFields := range tt.expectedFields.machineDeploymentBootstrapTemplate {
				setSpecFields(expectedBootstrapTemplates[mdTopology], expectedFields)
			}
			for mdTopology, expectedFields := range tt.expectedFields.machineDeploymentInfrastructureMachineTemplate {
				setSpecFields(expectedInfrastructureMachineTemplate[mdTopology], expectedFields)
			}
			for mpTopology, expectedFields := range tt.expectedFields.machinePoolBootstrapConfig {
				setSpecFields(expectedBootstrapConfig[mpTopology], expectedFields)
			}
			for mpTopology, expectedFields := range tt.expectedFields.machinePoolInfrastructureMachinePool {
				setSpecFields(expectedInfrastructureMachinePool[mpTopology], expectedFields)
			}

			// Apply patches.
			if err := patchEngine.Apply(context.Background(), blueprint, desired); err != nil {
				if !tt.wantErr {
					t.Fatal(err)
				}
				return
			}

			// Compare the patched desired objects with the expected desired objects.
			g.Expect(desired.Cluster).To(EqualObject(expectedCluster))
			g.Expect(desired.InfrastructureCluster).To(EqualObject(expectedInfrastructureCluster))
			g.Expect(desired.ControlPlane.Object).To(EqualObject(expectedControlPlane))
			g.Expect(desired.ControlPlane.InfrastructureMachineTemplate).To(EqualObject(expectedControlPlaneInfrastructureMachineTemplate))
			for mdTopology, bootstrapTemplate := range expectedBootstrapTemplates {
				g.Expect(desired.MachineDeployments[mdTopology].BootstrapTemplate).To(EqualObject(bootstrapTemplate))
			}
			for mdTopology, infrastructureMachineTemplate := range expectedInfrastructureMachineTemplate {
				g.Expect(desired.MachineDeployments[mdTopology].InfrastructureMachineTemplate).To(EqualObject(infrastructureMachineTemplate))
			}
			for mpTopology, bootstrapConfig := range expectedBootstrapConfig {
				g.Expect(desired.MachinePools[mpTopology].BootstrapObject).To(EqualObject(bootstrapConfig))
			}
			for mpTopology, infrastructureMachinePool := range expectedInfrastructureMachinePool {
				g.Expect(desired.MachinePools[mpTopology].InfrastructureMachinePoolObject).To(EqualObject(infrastructureMachinePool))
			}
		})
	}
}

func setupTestObjects() (*scope.ClusterBlueprint, *scope.ClusterState) {
	infrastructureClusterTemplate := builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infraClusterTemplate1").
		Build()

	controlPlaneInfrastructureMachineTemplate := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "controlplaneinframachinetemplate1").
		Build()
	controlPlaneTemplate := builder.ControlPlaneTemplate(metav1.NamespaceDefault, "controlPlaneTemplate1").
		WithInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
		Build()

	workerInfrastructureMachineTemplate := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "linux-worker-inframachinetemplate").
		Build()
	workerInfrastructureMachinePoolTemplate := builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "linux-worker-inframachinepooltemplate").
		Build()
	workerInfrastructureMachinePool := builder.InfrastructureMachinePool(metav1.NamespaceDefault, "linux-worker-inframachinepool").
		Build()
	workerBootstrapTemplate := builder.BootstrapTemplate(metav1.NamespaceDefault, "linux-worker-bootstrapconfigtemplate").
		Build()
	workerBootstrapConfig := builder.BootstrapConfig(metav1.NamespaceDefault, "linux-worker-bootstrapconfig").
		Build()
	mdClass1 := builder.MachineDeploymentClass("default-worker").
		WithInfrastructureTemplate(workerInfrastructureMachineTemplate).
		WithBootstrapTemplate(workerBootstrapTemplate).
		Build()
	mpClass1 := builder.MachinePoolClass("default-mp-worker").
		WithInfrastructureTemplate(workerInfrastructureMachinePoolTemplate).
		WithBootstrapTemplate(workerBootstrapTemplate).
		Build()

	clusterClass := builder.ClusterClass(metav1.NamespaceDefault, "clusterClass1").
		WithInfrastructureClusterTemplate(infrastructureClusterTemplate).
		WithControlPlaneTemplate(controlPlaneTemplate).
		WithControlPlaneInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
		WithWorkerMachineDeploymentClasses(*mdClass1).
		WithWorkerMachinePoolClasses(*mpClass1).
		Build()

	// Note: we depend on TypeMeta being set to calculate HolderReferences correctly.
	// We also set TypeMeta explicitly in the topology/cluster/cluster_controller.go.
	cluster := &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: metav1.NamespaceDefault,
			UID:       uuid.NewUUID(),
		},
		Spec: clusterv1.ClusterSpec{
			Paused: false,
			ClusterNetwork: &clusterv1.ClusterNetwork{
				APIServerPort: ptr.To[int32](8),
				Services: &clusterv1.NetworkRanges{
					CIDRBlocks: []string{"10.10.10.1/24"},
				},
				Pods: &clusterv1.NetworkRanges{
					CIDRBlocks: []string{"11.10.10.1/24"},
				},
				ServiceDomain: "lark",
			},
			ControlPlaneRef:   nil,
			InfrastructureRef: nil,
			Topology: &clusterv1.Topology{
				Version: "v1.21.2",
				Class:   clusterClass.Name,
				ControlPlane: clusterv1.ControlPlaneTopology{
					Replicas: ptr.To[int32](3),
					Variables: &clusterv1.ControlPlaneVariables{
						Overrides: []clusterv1.ClusterVariable{
							{
								Name:  "controlPlaneVariable",
								Value: apiextensionsv1.JSON{Raw: []byte(`"control-plane-override-value"`)},
							},
						},
					},
				},
				Variables: []clusterv1.ClusterVariable{
					{
						Name:  "infraCluster",
						Value: apiextensionsv1.JSON{Raw: []byte(`"value99"`)},
					},
					{
						Name: "controlPlaneVariable",
						// This value should be overwritten for the control plane.
						Value: apiextensionsv1.JSON{Raw: []byte(`"control-plane-cluster-wide-value"`)},
					},
					{
						Name: "defaultMDWorkerVariable",
						// This value should be overwritten for the default-worker-topo1 MachineDeployment.
						Value: apiextensionsv1.JSON{Raw: []byte(`"default-md-cluster-wide-value"`)},
					},
					{
						Name: "defaultMPWorkerVariable",
						// This value should be overwritten for the default-mp-worker-topo1 MachinePool.
						Value: apiextensionsv1.JSON{Raw: []byte(`"default-mp-cluster-wide-value"`)},
					},
				},
				Workers: &clusterv1.WorkersTopology{
					MachineDeployments: []clusterv1.MachineDeploymentTopology{
						{
							Metadata: clusterv1.ObjectMeta{Labels: map[string]string{"foo": "bar"}, Annotations: map[string]string{"fizz": "buzz"}},
							Class:    "default-worker",
							Name:     "default-worker-topo1",
							Variables: &clusterv1.MachineDeploymentVariables{
								Overrides: []clusterv1.ClusterVariable{
									{
										Name:  "defaultMDWorkerVariable",
										Value: apiextensionsv1.JSON{Raw: []byte(`"default-worker-topo1-override-value"`)},
									},
								},
							},
						},
						{
							Metadata: clusterv1.ObjectMeta{},
							Class:    "default-worker",
							Name:     "default-worker-topo2",
							Replicas: ptr.To[int32](5),
						},
					},
					MachinePools: []clusterv1.MachinePoolTopology{
						{
							Metadata: clusterv1.ObjectMeta{Labels: map[string]string{"foo": "bar"}, Annotations: map[string]string{"fizz": "buzz"}},
							Class:    "default-mp-worker",
							Name:     "default-mp-worker-topo1",
							Variables: &clusterv1.MachinePoolVariables{
								Overrides: []clusterv1.ClusterVariable{
									{
										Name:  "defaultMPWorkerVariable",
										Value: apiextensionsv1.JSON{Raw: []byte(`"default-mp-worker-topo1-override-value"`)},
									},
								},
							},
						},
						{
							Metadata: clusterv1.ObjectMeta{},
							Class:    "default-mp-worker",
							Name:     "default-mp-worker-topo2",
							Replicas: ptr.To[int32](5),
						},
					},
				},
			},
		},
	}

	// Aggregating Cluster, Templates and ClusterClass into a blueprint.
	blueprint := &scope.ClusterBlueprint{
		Topology:                      cluster.Spec.Topology,
		ClusterClass:                  clusterClass,
		InfrastructureClusterTemplate: infrastructureClusterTemplate,
		ControlPlane: &scope.ControlPlaneBlueprint{
			Template:                      controlPlaneTemplate,
			InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
		},
		MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{
			"default-worker": {
				InfrastructureMachineTemplate: workerInfrastructureMachineTemplate,
				BootstrapTemplate:             workerBootstrapTemplate,
			},
		},
		MachinePools: map[string]*scope.MachinePoolBlueprint{
			"default-mp-worker": {
				InfrastructureMachinePoolTemplate: workerInfrastructureMachinePoolTemplate,
				BootstrapTemplate:                 workerBootstrapTemplate,
			},
		},
	}

	// Create a Cluster using the ClusterClass from above with multiple MachineDeployments
	// using the same MachineDeployment class.
	desiredCluster := cluster.DeepCopy()

	infrastructureCluster := builder.InfrastructureCluster(metav1.NamespaceDefault, "infraClusterTemplate1").
		WithSpecFields(map[string]interface{}{
			// Add an empty spec field, to make sure the InfrastructureCluster matches
			// the one calculated by computeInfrastructureCluster.
			"spec": map[string]interface{}{},
		}).
		Build()

	controlPlane := builder.ControlPlane(metav1.NamespaceDefault, "controlPlane1").
		WithVersion("v1.21.2").
		WithReplicas(3).
		// Make sure we're using an independent instance of the template.
		WithInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate.DeepCopy()).
		Build()

	desired := &scope.ClusterState{
		Cluster:               desiredCluster,
		InfrastructureCluster: infrastructureCluster,
		ControlPlane: &scope.ControlPlaneState{
			Object: controlPlane,
			// Make sure we're using an independent instance of the template.
			InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate.DeepCopy(),
		},
		MachineDeployments: map[string]*scope.MachineDeploymentState{
			"default-worker-topo1": {
				Object: builder.MachineDeployment(metav1.NamespaceDefault, "md1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyMachineDeploymentNameLabel: "default-worker-topo1"}).
					WithVersion("v1.21.2").
					Build(),
				// Make sure we're using an independent instance of the template.
				InfrastructureMachineTemplate: workerInfrastructureMachineTemplate.DeepCopy(),
				BootstrapTemplate:             workerBootstrapTemplate.DeepCopy(),
			},
			"default-worker-topo2": {
				Object: builder.MachineDeployment(metav1.NamespaceDefault, "md2").
					WithLabels(map[string]string{clusterv1.ClusterTopologyMachineDeploymentNameLabel: "default-worker-topo2"}).
					WithVersion("v1.20.6").
					WithReplicas(5).
					Build(),
				// Make sure we're using an independent instance of the template.
				InfrastructureMachineTemplate: workerInfrastructureMachineTemplate.DeepCopy(),
				BootstrapTemplate:             workerBootstrapTemplate.DeepCopy(),
			},
		},
		MachinePools: map[string]*scope.MachinePoolState{
			"default-mp-worker-topo1": {
				Object: builder.MachinePool(metav1.NamespaceDefault, "mp1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyMachinePoolNameLabel: "default-mp-worker-topo1"}).
					WithVersion("v1.21.2").
					Build(),
				// Make sure we're using an independent instance of the template.
				InfrastructureMachinePoolObject: workerInfrastructureMachinePool.DeepCopy(),
				BootstrapObject:                 workerBootstrapConfig.DeepCopy(),
			},
			"default-mp-worker-topo2": {
				Object: builder.MachinePool(metav1.NamespaceDefault, "mp2").
					WithLabels(map[string]string{clusterv1.ClusterTopologyMachinePoolNameLabel: "default-mp-worker-topo2"}).
					WithVersion("v1.20.6").
					WithReplicas(5).
					Build(),
				// Make sure we're using an independent instance of the template.
				InfrastructureMachinePoolObject: workerInfrastructureMachinePool.DeepCopy(),
				BootstrapObject:                 workerBootstrapConfig.DeepCopy(),
			},
		},
	}
	return blueprint, desired
}

// setSpecFields sets fields on an unstructured object from a map.
func setSpecFields(obj *unstructured.Unstructured, fields map[string]interface{}) {
	for k, v := range fields {
		fieldParts := strings.Split(k, ".")
		if len(fieldParts) == 0 {
			panic(fmt.Errorf("fieldParts invalid"))
		}
		if fieldParts[0] != "spec" {
			panic(fmt.Errorf("can not set fields outside spec"))
		}
		if err := unstructured.SetNestedField(obj.UnstructuredContent(), v, strings.Split(k, ".")...); err != nil {
			panic(err)
		}
	}
}

// jsonPatchRFC6902 represents a jsonPatch.
type jsonPatchRFC6902 struct {
	Op    string                `json:"op"`
	Path  string                `json:"path"`
	Value *apiextensionsv1.JSON `json:"value,omitempty"`
}

func bytesPatch(patch []jsonPatchRFC6902) []byte {
	out, err := json.Marshal(patch)
	if err != nil {
		panic(err)
	}
	return out
}
