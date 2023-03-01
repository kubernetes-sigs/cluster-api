/*
Copyright 2022 The Kubernetes Authors.

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

package topologymutation

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/patches/variables"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
)

var (
	testScheme = runtime.NewScheme()
)

func init() {
	_ = infrav1.AddToScheme(testScheme)
	_ = controlplanev1.AddToScheme(testScheme)
	_ = bootstrapv1.AddToScheme(testScheme)
}

func Test_patchDockerClusterTemplate(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name             string
		template         *infrav1.DockerClusterTemplate
		variables        map[string]apiextensionsv1.JSON
		expectedTemplate *infrav1.DockerClusterTemplate
		expectedErr      bool
	}{
		{
			name:             "no op if imageRepository is not set",
			template:         &infrav1.DockerClusterTemplate{},
			variables:        nil,
			expectedTemplate: &infrav1.DockerClusterTemplate{},
		},
		{
			name:     "set LoadBalancer.ImageRepository if imageRepository is set",
			template: &infrav1.DockerClusterTemplate{},
			variables: map[string]apiextensionsv1.JSON{
				"imageRepository": {Raw: toJSON("testImage")},
			},
			expectedTemplate: &infrav1.DockerClusterTemplate{
				Spec: infrav1.DockerClusterTemplateSpec{
					Template: infrav1.DockerClusterTemplateResource{
						Spec: infrav1.DockerClusterSpec{
							LoadBalancer: infrav1.DockerLoadBalancer{
								ImageMeta: infrav1.ImageMeta{
									ImageRepository: "testImage",
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := patchDockerClusterTemplate(context.Background(), tt.template, tt.variables)
			if tt.expectedErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(tt.template).To(Equal(tt.expectedTemplate))
		})
	}
}

func Test_patchKubeadmControlPlaneTemplate(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name             string
		template         *controlplanev1.KubeadmControlPlaneTemplate
		variables        map[string]apiextensionsv1.JSON
		expectedTemplate *controlplanev1.KubeadmControlPlaneTemplate
		expectedErr      bool
	}{
		{
			name:             "fails if builtin.controlPlane.version is not set",
			template:         &controlplanev1.KubeadmControlPlaneTemplate{},
			variables:        nil,
			expectedTemplate: &controlplanev1.KubeadmControlPlaneTemplate{},
			expectedErr:      true,
		},
		{
			name:     "sets KubeletExtraArgs[cgroup-driver] to cgroupfs for Kubernetes < 1.24",
			template: &controlplanev1.KubeadmControlPlaneTemplate{},
			variables: map[string]apiextensionsv1.JSON{
				variables.BuiltinsName: {Raw: toJSON(variables.Builtins{
					ControlPlane: &variables.ControlPlaneBuiltins{
						Version: "v1.23.0",
					},
				})},
			},
			expectedTemplate: &controlplanev1.KubeadmControlPlaneTemplate{
				Spec: controlplanev1.KubeadmControlPlaneTemplateSpec{
					Template: controlplanev1.KubeadmControlPlaneTemplateResource{
						Spec: controlplanev1.KubeadmControlPlaneTemplateResourceSpec{
							KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
								InitConfiguration: &bootstrapv1.InitConfiguration{
									NodeRegistration: bootstrapv1.NodeRegistrationOptions{
										KubeletExtraArgs: map[string]string{"cgroup-driver": "cgroupfs"},
									},
								},
								JoinConfiguration: &bootstrapv1.JoinConfiguration{
									NodeRegistration: bootstrapv1.NodeRegistrationOptions{
										KubeletExtraArgs: map[string]string{"cgroup-driver": "cgroupfs"},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:     "do not set KubeletExtraArgs[cgroup-driver] to cgroupfs for Kubernetes >= 1.24",
			template: &controlplanev1.KubeadmControlPlaneTemplate{},
			variables: map[string]apiextensionsv1.JSON{
				variables.BuiltinsName: {Raw: toJSON(variables.Builtins{
					ControlPlane: &variables.ControlPlaneBuiltins{
						Version: "v1.24.0",
					},
				})},
			},
			expectedTemplate: &controlplanev1.KubeadmControlPlaneTemplate{},
		},
		{
			name:     "sets RolloutStrategy.RollingUpdate.MaxSurge if the kubeadmControlPlaneMaxSurge is provided",
			template: &controlplanev1.KubeadmControlPlaneTemplate{},
			variables: map[string]apiextensionsv1.JSON{
				variables.BuiltinsName: {Raw: toJSON(variables.Builtins{
					ControlPlane: &variables.ControlPlaneBuiltins{
						Version: "v1.24.0",
					},
				})},
				"kubeadmControlPlaneMaxSurge": {Raw: toJSON("1")},
			},
			expectedTemplate: &controlplanev1.KubeadmControlPlaneTemplate{
				Spec: controlplanev1.KubeadmControlPlaneTemplateSpec{
					Template: controlplanev1.KubeadmControlPlaneTemplateResource{
						Spec: controlplanev1.KubeadmControlPlaneTemplateResourceSpec{
							RolloutStrategy: &controlplanev1.RolloutStrategy{
								RollingUpdate: &controlplanev1.RollingUpdate{MaxSurge: &intstr.IntOrString{IntVal: 1}},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := patchKubeadmControlPlaneTemplate(context.Background(), tt.template, tt.variables)
			if tt.expectedErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(tt.template).To(Equal(tt.expectedTemplate))
		})
	}
}

func Test_patchKubeadmConfigTemplate(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name             string
		template         *bootstrapv1.KubeadmConfigTemplate
		variables        map[string]apiextensionsv1.JSON
		expectedTemplate *bootstrapv1.KubeadmConfigTemplate
		expectedErr      bool
	}{
		{
			name:             "fails if builtin.machineDeployment.class is not set",
			template:         &bootstrapv1.KubeadmConfigTemplate{},
			variables:        nil,
			expectedTemplate: &bootstrapv1.KubeadmConfigTemplate{},
			expectedErr:      true,
		},
		{
			name:     "no op for MachineDeployment class != default-worker",
			template: &bootstrapv1.KubeadmConfigTemplate{},
			variables: map[string]apiextensionsv1.JSON{
				variables.BuiltinsName: {Raw: toJSON(variables.Builtins{
					MachineDeployment: &variables.MachineDeploymentBuiltins{
						Class: "another-class",
					},
				})},
			},
			expectedTemplate: &bootstrapv1.KubeadmConfigTemplate{},
		},
		{
			name:     "fails if builtin.machineDeployment.version is not set for MachineDeployment class == default-worker",
			template: &bootstrapv1.KubeadmConfigTemplate{},
			variables: map[string]apiextensionsv1.JSON{
				variables.BuiltinsName: {Raw: toJSON(variables.Builtins{
					MachineDeployment: &variables.MachineDeploymentBuiltins{
						Class: "default-worker",
					},
				})},
			},
			expectedTemplate: &bootstrapv1.KubeadmConfigTemplate{},
			expectedErr:      true,
		},
		{
			name:     "set KubeletExtraArgs[cgroup-driver] to cgroupfs for Kubernetes < 1.24 and MachineDeployment class == default-worker",
			template: &bootstrapv1.KubeadmConfigTemplate{},
			variables: map[string]apiextensionsv1.JSON{
				variables.BuiltinsName: {Raw: toJSON(variables.Builtins{
					MachineDeployment: &variables.MachineDeploymentBuiltins{
						Class:   "default-worker",
						Version: "v1.23.0",
					},
				})},
			},
			expectedTemplate: &bootstrapv1.KubeadmConfigTemplate{
				Spec: bootstrapv1.KubeadmConfigTemplateSpec{
					Template: bootstrapv1.KubeadmConfigTemplateResource{
						Spec: bootstrapv1.KubeadmConfigSpec{
							JoinConfiguration: &bootstrapv1.JoinConfiguration{
								NodeRegistration: bootstrapv1.NodeRegistrationOptions{
									KubeletExtraArgs: map[string]string{"cgroup-driver": "cgroupfs"},
								},
							},
						},
					},
				},
			},
		},
		{
			name:     "do not set KubeletExtraArgs[cgroup-driver] to cgroupfs for Kubernetes >= 1.24 and MachineDeployment class == default-worker",
			template: &bootstrapv1.KubeadmConfigTemplate{},
			variables: map[string]apiextensionsv1.JSON{
				variables.BuiltinsName: {Raw: toJSON(variables.Builtins{
					MachineDeployment: &variables.MachineDeploymentBuiltins{
						Class:   "default-worker",
						Version: "v1.24.0",
					},
				})},
			},
			expectedTemplate: &bootstrapv1.KubeadmConfigTemplate{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := patchKubeadmConfigTemplate(context.Background(), tt.template, tt.variables)
			if tt.expectedErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(tt.template).To(Equal(tt.expectedTemplate))
		})
	}
}

func Test_patchDockerMachineTemplate(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name             string
		template         *infrav1.DockerMachineTemplate
		variables        map[string]apiextensionsv1.JSON
		expectedTemplate *infrav1.DockerMachineTemplate
		expectedErr      bool
	}{
		{
			name:             "fails if builtin.controlPlane.version nor builtin.machineDeployment.version is not set",
			template:         &infrav1.DockerMachineTemplate{},
			variables:        nil,
			expectedTemplate: &infrav1.DockerMachineTemplate{},
			expectedErr:      true,
		},
		{
			name:     "sets customImage for templates linked to ControlPlane",
			template: &infrav1.DockerMachineTemplate{},
			variables: map[string]apiextensionsv1.JSON{
				variables.BuiltinsName: {Raw: toJSON(variables.Builtins{
					ControlPlane: &variables.ControlPlaneBuiltins{
						Version: "v1.23.0",
					},
				})},
			},
			expectedTemplate: &infrav1.DockerMachineTemplate{
				Spec: infrav1.DockerMachineTemplateSpec{
					Template: infrav1.DockerMachineTemplateResource{
						Spec: infrav1.DockerMachineSpec{
							CustomImage: "kindest/node:v1.23.0",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := patchDockerMachineTemplate(context.Background(), tt.template, tt.variables)
			if tt.expectedErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(tt.template).To(Equal(tt.expectedTemplate))
		})
	}
}

// Note: given that we are testing functions used for modifying templates inside GeneratePatches it is not
// required to test GeneratePatches for all the sub-cases; we are only testing that everything comes together as expected.
// NOTE: custom RuntimeExtension must test specif logic added to GeneratePatches, if any.
func TestHandler_GeneratePatches(t *testing.T) {
	g := NewWithT(t)
	h := NewExtensionHandlers(testScheme)
	controlPlaneVarsV123WithMaxSurge := []runtimehooksv1.Variable{
		newVariable(variables.BuiltinsName, variables.Builtins{
			ControlPlane: &variables.ControlPlaneBuiltins{
				Version: "v1.23.0",
			},
		}),
		newVariable("kubeadmControlPlaneMaxSurge", "3"),
	}
	imageRepositoryVar := []runtimehooksv1.Variable{
		newVariable("imageRepository", "docker.io"),
	}
	machineDeploymentVars123 := []runtimehooksv1.Variable{
		newVariable(variables.BuiltinsName, variables.Builtins{
			MachineDeployment: &variables.MachineDeploymentBuiltins{
				Class:   "default-worker",
				Version: "v1.23.0",
			},
		}),
	}
	kubeadmControlPlaneTemplate := controlplanev1.KubeadmControlPlaneTemplate{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmControlPlaneTemplate",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
	}
	dockerMachineTemplate := infrav1.DockerMachineTemplate{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DockerMachineTemplate",
			APIVersion: infrav1.GroupVersion.String(),
		},
	}
	dockerClusterTemplate := infrav1.DockerClusterTemplate{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DockerClusterTemplate",
			APIVersion: infrav1.GroupVersion.String(),
		},
	}
	kubeadmConfigTemplate := bootstrapv1.KubeadmConfigTemplate{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmConfigTemplate",
			APIVersion: bootstrapv1.GroupVersion.String(),
		},
	}
	tests := []struct {
		name             string
		requestItems     []runtimehooksv1.GeneratePatchesRequestItem
		expectedResponse *runtimehooksv1.GeneratePatchesResponse
	}{
		{
			name: "All the templates are patched",
			requestItems: []runtimehooksv1.GeneratePatchesRequestItem{
				requestItem("1", kubeadmControlPlaneTemplate, controlPlaneVarsV123WithMaxSurge),
				requestItem("2", dockerMachineTemplate, controlPlaneVarsV123WithMaxSurge),
				requestItem("3", dockerMachineTemplate, machineDeploymentVars123),
				requestItem("4", dockerClusterTemplate, imageRepositoryVar),
				requestItem("5", kubeadmConfigTemplate, machineDeploymentVars123),
			},
			expectedResponse: &runtimehooksv1.GeneratePatchesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusSuccess,
				},
				Items: []runtimehooksv1.GeneratePatchesResponseItem{
					responseItem("1", `[
{"op":"add","path":"/spec/template/spec/kubeadmConfigSpec/initConfiguration","value":{"localAPIEndpoint":{},"nodeRegistration":{"kubeletExtraArgs":{"cgroup-driver":"cgroupfs"}}}},
{"op":"add","path":"/spec/template/spec/kubeadmConfigSpec/joinConfiguration","value":{"discovery":{},"nodeRegistration":{"kubeletExtraArgs":{"cgroup-driver":"cgroupfs"}}}},
{"op":"add","path":"/spec/template/spec/rolloutStrategy","value":{"rollingUpdate":{"maxSurge":3}}}
]`),
					responseItem("2", `[
{"op":"add","path":"/spec/template/spec/customImage","value":"kindest/node:v1.23.0"}
]`),
					responseItem("3", `[
{"op":"add","path":"/spec/template/spec/customImage","value":"kindest/node:v1.23.0"}
]`),

					responseItem("4", `[
{"op":"add","path":"/spec/template/spec/loadBalancer/imageRepository","value":"docker.io"}
]`),
					responseItem("5", `[
{"op":"add","path":"/spec/template/spec/joinConfiguration","value":{"discovery":{},"nodeRegistration":{"kubeletExtraArgs":{"cgroup-driver":"cgroupfs"}}}}
]`),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := &runtimehooksv1.GeneratePatchesResponse{}
			request := &runtimehooksv1.GeneratePatchesRequest{Items: tt.requestItems}
			h.GeneratePatches(context.Background(), request, response)

			// Expect all response fields to be as expected. responseItems are ignored here and tested below.
			// Ignore the message to not compare error strings.
			g.Expect(tt.expectedResponse).To(EqualObject(response, IgnorePaths{"items", ".message"}))
			// For each item in the response check that the patches are the same.
			// Note that the order of the individual patch operations in Items[].Patch is not determinate so we unmarshal
			// to an array and check that the arrays hold equivalent items.
			for i, item := range response.Items {
				expectedItem := tt.expectedResponse.Items[i]
				g.Expect(item.PatchType).To(Equal(expectedItem.PatchType))
				g.Expect(item.UID).To(Equal(expectedItem.UID))

				var actualPatchOps []map[string]interface{}
				var expectedPatchOps []map[string]interface{}
				g.Expect(json.Unmarshal(item.Patch, &actualPatchOps)).To(Succeed())
				g.Expect(json.Unmarshal(expectedItem.Patch, &expectedPatchOps)).To(Succeed())
				g.Expect(actualPatchOps).To(ConsistOf(expectedPatchOps))
			}
		})
	}
}

// toJSONCompact is used to be able to write JSON values in a readable manner.
func toJSONCompact(value string) []byte {
	var compactValue bytes.Buffer
	if err := json.Compact(&compactValue, []byte(value)); err != nil {
		panic(err)
	}
	return compactValue.Bytes()
}

// toJSON marshals the object and returns a byte array. This function panics on any error.
func toJSON(val interface{}) []byte {
	jsonStr, err := json.Marshal(val)
	if err != nil {
		panic(err)
	}
	return jsonStr
}

// requestItem returns a GeneratePatchesRequestItem with the given uid, variables and object.
func requestItem(uid string, object interface{}, variables []runtimehooksv1.Variable) runtimehooksv1.GeneratePatchesRequestItem {
	return runtimehooksv1.GeneratePatchesRequestItem{
		UID:       types.UID(uid),
		Variables: variables,
		Object: runtime.RawExtension{
			Raw: toJSON(object),
		},
	}
}

// responseItem returns a GeneratePatchesResponseItem of PatchType JSONPatch with the passed uid and patch.
func responseItem(uid, patch string) runtimehooksv1.GeneratePatchesResponseItem {
	return runtimehooksv1.GeneratePatchesResponseItem{
		UID:       types.UID(uid),
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     toJSONCompact(patch),
	}
}

// newVariable returns a runtimehooksv1.Variable with the passed name and value.
func newVariable(name string, value interface{}) runtimehooksv1.Variable {
	return runtimehooksv1.Variable{
		Name:  name,
		Value: apiextensionsv1.JSON{Raw: toJSON(value)},
	}
}
