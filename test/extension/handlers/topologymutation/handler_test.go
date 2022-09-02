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

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	. "sigs.k8s.io/cluster-api/internal/test/matchers"
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

func TestHandler_GeneratePatches(t *testing.T) {
	g := NewWithT(t)
	h := NewHandler(testScheme)
	emptyVars := []runtimehooksv1.Variable{}
	controlPlaneVarsV123WithMaxSurge := []runtimehooksv1.Variable{
		builtInVariablesForControlPlane("v1.23.0"),
		newVariable("kubeadmControlPlaneMaxSurge", "3"),
	}
	controlPlaneVarsV123 := []runtimehooksv1.Variable{
		builtInVariablesForControlPlane("v1.23.0"),
	}
	controlPlaneVarsInvalidVersion := []runtimehooksv1.Variable{
		builtInVariablesForControlPlane("v1.23.x"),
	}
	machineDeploymentVarsInvalidVersion := []runtimehooksv1.Variable{
		builtInVariablesForControlPlane("v1.23.x"),
	}
	lbImageRepositoryVar := []runtimehooksv1.Variable{
		newVariable("lbImageRepository", "docker.io"),
	}
	controlPlaneVarsV124 := []runtimehooksv1.Variable{
		builtInVariablesForControlPlane("v1.24.0"),
	}
	machineDeploymentVars123 := []runtimehooksv1.Variable{
		builtInVariablesForMachineDeployment("default-worker", "v1.23.0"),
	}
	machineDeploymentVars124 := []runtimehooksv1.Variable{
		builtInVariablesForMachineDeployment("default-worker", "v1.24.0"),
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
			name: "With version 1.23 for all objects KubeadmControlPlane, KubeadmConfigTemplate, DockerClusterTemplate and DockerMachineTemplate",
			requestItems: []runtimehooksv1.GeneratePatchesRequestItem{
				requestItem("1", kubeadmControlPlaneTemplate, controlPlaneVarsV123WithMaxSurge),
				requestItem("2", dockerMachineTemplate, controlPlaneVarsV123WithMaxSurge),
				requestItem("3", dockerMachineTemplate, machineDeploymentVars123),
				requestItem("4", dockerClusterTemplate, lbImageRepositoryVar),
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
{"op":"add","path":"/spec/template/spec/joinConfiguration","value":{"discovery":{},"nodeRegistration":{"kubeletExtraArgs":{"cgroup-driver":"cgroupfs"}}}},
{"op":"add","path":"/spec/template/spec/initConfiguration","value":{"localAPIEndpoint":{},"nodeRegistration":{"kubeletExtraArgs":{"cgroup-driver":"cgroupfs"}}}}
]`),
				},
			},
		},
		{
			name: "With version 1.24 for CP and MD no patch for KubeadmControlPlane, KubeadmConfigTemplate",
			requestItems: []runtimehooksv1.GeneratePatchesRequestItem{
				requestItem("1", kubeadmControlPlaneTemplate, controlPlaneVarsV124),
				requestItem("4", kubeadmConfigTemplate, machineDeploymentVars124),
			},
			expectedResponse: &runtimehooksv1.GeneratePatchesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusSuccess,
				},
				Items: []runtimehooksv1.GeneratePatchesResponseItem{
					responseItem("1", `[]`),
					responseItem("4", `[]`),
				},
			},
		},
		{
			name: "With version 1.24 for CP and v1.23 for MD no patch for KubeadmControlPlane, patch KubeadmConfigTemplate",
			requestItems: []runtimehooksv1.GeneratePatchesRequestItem{
				requestItem("1", kubeadmControlPlaneTemplate, controlPlaneVarsV124),
				requestItem("4", kubeadmConfigTemplate, machineDeploymentVars123),
			},
			expectedResponse: &runtimehooksv1.GeneratePatchesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusSuccess,
				},
				Items: []runtimehooksv1.GeneratePatchesResponseItem{
					responseItem("1", `[]`),
					responseItem("4", `[
{"op":"add","path":"/spec/template/spec/joinConfiguration","value":{"discovery":{},"nodeRegistration":{"kubeletExtraArgs":{"cgroup-driver":"cgroupfs"}}}},
{"op":"add","path":"/spec/template/spec/initConfiguration","value":{"localAPIEndpoint":{},"nodeRegistration":{"kubeletExtraArgs":{"cgroup-driver":"cgroupfs"}}}}
]`),
				},
			},
		},
		{
			name: "With version 1.23 for CP and v1.24 for MD patch KubeadmControlPlane, no patch for KubeadmConfigTemplate",
			requestItems: []runtimehooksv1.GeneratePatchesRequestItem{
				requestItem("1", kubeadmControlPlaneTemplate, controlPlaneVarsV123),
				requestItem("4", kubeadmConfigTemplate, machineDeploymentVars124),
			},
			expectedResponse: &runtimehooksv1.GeneratePatchesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusSuccess,
				},
				Items: []runtimehooksv1.GeneratePatchesResponseItem{
					responseItem("1", `[
{"op":"add","path":"/spec/template/spec/kubeadmConfigSpec/initConfiguration","value":{"localAPIEndpoint":{},"nodeRegistration":{"kubeletExtraArgs":{"cgroup-driver":"cgroupfs"}}}},
{"op":"add","path":"/spec/template/spec/kubeadmConfigSpec/joinConfiguration","value":{"discovery":{},"nodeRegistration":{"kubeletExtraArgs":{"cgroup-driver":"cgroupfs"}}}}
]`),
					responseItem("4", `[]`),
				},
			},
		},
		{
			name: "Error with an invalid version for the KubeadmControlPlane patch",
			requestItems: []runtimehooksv1.GeneratePatchesRequestItem{
				requestItem("2", kubeadmControlPlaneTemplate, controlPlaneVarsInvalidVersion),
			},
			expectedResponse: &runtimehooksv1.GeneratePatchesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusFailure,
				},
			},
		},
		{
			name: "Error with an invalid version for the machine deployment KubeadmConfigTemplate patch",
			requestItems: []runtimehooksv1.GeneratePatchesRequestItem{
				requestItem("1", kubeadmConfigTemplate, machineDeploymentVarsInvalidVersion),
			},
			expectedResponse: &runtimehooksv1.GeneratePatchesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusFailure,
				},
			},
		},
		{
			name: "Error with an invalid version for the machine deployment DockerMachineTemplate patch",
			requestItems: []runtimehooksv1.GeneratePatchesRequestItem{
				requestItem("1", dockerMachineTemplate, machineDeploymentVarsInvalidVersion),
			},
			expectedResponse: &runtimehooksv1.GeneratePatchesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusFailure,
				},
			},
		},
		{
			name: "Error with an invalid version for the control plane DockerMachineTemplate patch",
			requestItems: []runtimehooksv1.GeneratePatchesRequestItem{
				requestItem("1", dockerMachineTemplate, controlPlaneVarsInvalidVersion),
			},
			expectedResponse: &runtimehooksv1.GeneratePatchesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusFailure,
				},
			},
		},
		{
			name: "Error with no variables found the control plane DockerMachineTemplate patch",
			requestItems: []runtimehooksv1.GeneratePatchesRequestItem{
				requestItem("1", dockerMachineTemplate, emptyVars),
			},
			expectedResponse: &runtimehooksv1.GeneratePatchesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusFailure,
				},
			},
		},
		{
			name: "Error with no variables found for the MachineDeployment DockerMachineTemplate patch",
			requestItems: []runtimehooksv1.GeneratePatchesRequestItem{
				requestItem("1", dockerMachineTemplate, emptyVars),
			},
			expectedResponse: &runtimehooksv1.GeneratePatchesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusFailure,
				},
			},
		},
		{
			name: "Error with no variables found for the KubeadmControlPlane patch",
			requestItems: []runtimehooksv1.GeneratePatchesRequestItem{
				requestItem("1", kubeadmControlPlaneTemplate, emptyVars),
			},
			expectedResponse: &runtimehooksv1.GeneratePatchesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusFailure,
				},
			},
		},
		{
			name: "Error with no variables found for the KubeadmConfigTemplate patch",
			requestItems: []runtimehooksv1.GeneratePatchesRequestItem{
				requestItem("1", kubeadmConfigTemplate, emptyVars),
			},
			expectedResponse: &runtimehooksv1.GeneratePatchesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusFailure,
				},
			},
		},
		{
			name: "No error and no patch with no variables found for the DockerCluster patch",
			requestItems: []runtimehooksv1.GeneratePatchesRequestItem{
				requestItem("1", dockerClusterTemplate, emptyVars),
			},
			expectedResponse: &runtimehooksv1.GeneratePatchesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusSuccess,
				},
				Items: []runtimehooksv1.GeneratePatchesResponseItem{
					responseItem("1", `[]`),
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
			g.Expect(tt.expectedResponse).To(EqualObject(response, IgnorePaths{{"items"}, {"message"}}))
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

// builtInVariablesForControlPlane returns a runtimehooksv1.Variable for a control plane with the passed version.
func builtInVariablesForControlPlane(version string) runtimehooksv1.Variable {
	vars := map[string]interface{}{
		"controlPlane": map[string]interface{}{
			"version": version,
		},
	}
	return newVariable("builtin", vars)
}

// newVariable returns a runtimehooksv1.Variable with the passed name and value.
func newVariable(name string, value interface{}) runtimehooksv1.Variable {
	return runtimehooksv1.Variable{
		Name:  name,
		Value: apiextensionsv1.JSON{Raw: toJSON(value)},
	}
}

// builtInVariablesForMachineDeployment returns a runtimehooksv1.Variable for a machineDeployment with the passed class and version.
func builtInVariablesForMachineDeployment(class, version string) runtimehooksv1.Variable {
	vars := map[string]interface{}{
		"machineDeployment": map[string]interface{}{
			"version": version,
			"class":   class,
		},
	}
	return newVariable("builtin", vars)
}
