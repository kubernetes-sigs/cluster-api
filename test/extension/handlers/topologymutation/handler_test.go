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
	"k8s.io/utils/ptr"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
)

var (
	testScheme = runtime.NewScheme()
)

func init() {
	_ = infrav1.AddToScheme(testScheme)
	_ = controlplanev1.AddToScheme(testScheme)
	_ = bootstrapv1.AddToScheme(testScheme)
}

func Test_patchDevClusterTemplate(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name             string
		template         *infrav1.DevClusterTemplate
		variables        map[string]apiextensionsv1.JSON
		expectedTemplate *infrav1.DevClusterTemplate
		expectedErr      bool
	}{
		{
			name:             "no op if imageRepository is not set",
			template:         &infrav1.DevClusterTemplate{},
			variables:        nil,
			expectedTemplate: &infrav1.DevClusterTemplate{},
		},
		{
			name: "set LoadBalancer.ImageRepository if imageRepository is set",
			template: &infrav1.DevClusterTemplate{
				Spec: infrav1.DevClusterTemplateSpec{
					Template: infrav1.DevClusterTemplateResource{
						Spec: infrav1.DevClusterSpec{
							Backend: infrav1.DevClusterBackendSpec{
								Docker: &infrav1.DockerClusterBackendSpec{},
							},
						},
					},
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				"imageRepository": {Raw: toJSON("testImage")},
			},
			expectedTemplate: &infrav1.DevClusterTemplate{
				Spec: infrav1.DevClusterTemplateSpec{
					Template: infrav1.DevClusterTemplateResource{
						Spec: infrav1.DevClusterSpec{
							Backend: infrav1.DevClusterBackendSpec{
								Docker: &infrav1.DockerClusterBackendSpec{
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
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(*testing.T) {
			err := patchDevClusterTemplate(context.Background(), tt.template, tt.variables)
			if tt.expectedErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(tt.template).To(BeComparableTo(tt.expectedTemplate))
		})
	}
}

func Test_patchKubeadmControlPlaneTemplate(t *testing.T) {
	tests := []struct {
		name             string
		template         *controlplanev1.KubeadmControlPlaneTemplate
		variables        map[string]apiextensionsv1.JSON
		expectedTemplate *controlplanev1.KubeadmControlPlaneTemplate
		expectedErr      bool
	}{
		{
			name:     "sets RolloutStrategy.RollingUpdate.MaxSurge if the kubeadmControlPlaneMaxSurge is provided",
			template: &controlplanev1.KubeadmControlPlaneTemplate{},
			variables: map[string]apiextensionsv1.JSON{
				runtimehooksv1.BuiltinsName: {Raw: toJSON(runtimehooksv1.Builtins{
					ControlPlane: &runtimehooksv1.ControlPlaneBuiltins{
						Version: "v1.24.0",
					},
				})},
				"kubeadmControlPlaneMaxSurge": {Raw: toJSON("1")},
			},
			expectedTemplate: &controlplanev1.KubeadmControlPlaneTemplate{
				Spec: controlplanev1.KubeadmControlPlaneTemplateSpec{
					Template: controlplanev1.KubeadmControlPlaneTemplateResource{
						Spec: controlplanev1.KubeadmControlPlaneTemplateResourceSpec{
							Rollout: controlplanev1.KubeadmControlPlaneRolloutSpec{
								Strategy: controlplanev1.KubeadmControlPlaneRolloutStrategy{
									Type: controlplanev1.RollingUpdateStrategyType,
									RollingUpdate: controlplanev1.KubeadmControlPlaneRolloutStrategyRollingUpdate{
										MaxSurge: &intstr.IntOrString{IntVal: 1},
									},
								},
							},
							KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
								ClusterConfiguration: bootstrapv1.ClusterConfiguration{
									APIServer: bootstrapv1.APIServer{
										ExtraArgs: []bootstrapv1.Arg{{Name: "v", Value: ptr.To("2")}},
									},
									ControllerManager: bootstrapv1.ControllerManager{
										ExtraArgs: []bootstrapv1.Arg{{Name: "v", Value: ptr.To("2")}},
									},
									Scheduler: bootstrapv1.Scheduler{
										ExtraArgs: []bootstrapv1.Arg{{Name: "v", Value: ptr.To("2")}},
									},
								},
								InitConfiguration: bootstrapv1.InitConfiguration{
									NodeRegistration: bootstrapv1.NodeRegistrationOptions{
										KubeletExtraArgs: []bootstrapv1.Arg{
											{Name: "v", Value: ptr.To("2")},
											{Name: "node-labels", Value: ptr.To("kubernetesVersion=v1.24.0")},
										},
									},
								},
								JoinConfiguration: bootstrapv1.JoinConfiguration{
									NodeRegistration: bootstrapv1.NodeRegistrationOptions{
										KubeletExtraArgs: []bootstrapv1.Arg{
											{Name: "v", Value: ptr.To("2")},
											{Name: "node-labels", Value: ptr.To("kubernetesVersion=v1.24.0")},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(*testing.T) {
			g := NewWithT(t)

			err := patchKubeadmControlPlaneTemplate(context.Background(), tt.template, tt.variables)
			if tt.expectedErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(tt.template).To(BeComparableTo(tt.expectedTemplate))
		})
	}
}

func Test_patchDevMachineTemplate(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name             string
		template         *infrav1.DevMachineTemplate
		variables        map[string]apiextensionsv1.JSON
		expectedTemplate *infrav1.DevMachineTemplate
		expectedErr      bool
	}{
		{
			name:             "fails if builtin.controlPlane.version nor builtin.machineDeployment.version is not set",
			template:         &infrav1.DevMachineTemplate{},
			variables:        nil,
			expectedTemplate: &infrav1.DevMachineTemplate{},
			expectedErr:      true,
		},
		{
			name: "sets customImage for templates linked to ControlPlane",
			template: &infrav1.DevMachineTemplate{
				Spec: infrav1.DevMachineTemplateSpec{
					Template: infrav1.DevMachineTemplateResource{
						Spec: infrav1.DevMachineSpec{
							Backend: infrav1.DevMachineBackendSpec{
								Docker: &infrav1.DockerMachineBackendSpec{},
							},
						},
					},
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				runtimehooksv1.BuiltinsName: {Raw: toJSON(runtimehooksv1.Builtins{
					ControlPlane: &runtimehooksv1.ControlPlaneBuiltins{
						Version: "v1.23.0",
					},
				})},
			},
			expectedTemplate: &infrav1.DevMachineTemplate{
				Spec: infrav1.DevMachineTemplateSpec{
					Template: infrav1.DevMachineTemplateResource{
						Spec: infrav1.DevMachineSpec{
							Backend: infrav1.DevMachineBackendSpec{
								Docker: &infrav1.DockerMachineBackendSpec{
									CustomImage: "kindest/node:v1.23.0",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "sets customImage for templates linked to ControlPlane for pre versions",
			template: &infrav1.DevMachineTemplate{
				Spec: infrav1.DevMachineTemplateSpec{
					Template: infrav1.DevMachineTemplateResource{
						Spec: infrav1.DevMachineSpec{
							Backend: infrav1.DevMachineBackendSpec{
								Docker: &infrav1.DockerMachineBackendSpec{},
							},
						},
					},
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				runtimehooksv1.BuiltinsName: {Raw: toJSON(runtimehooksv1.Builtins{
					ControlPlane: &runtimehooksv1.ControlPlaneBuiltins{
						Version: "v1.23.0-rc.0",
					},
				})},
			},
			expectedTemplate: &infrav1.DevMachineTemplate{
				Spec: infrav1.DevMachineTemplateSpec{
					Template: infrav1.DevMachineTemplateResource{
						Spec: infrav1.DevMachineSpec{
							Backend: infrav1.DevMachineBackendSpec{
								Docker: &infrav1.DockerMachineBackendSpec{
									CustomImage: "kindest/node:v1.23.0-rc.0",
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(*testing.T) {
			err := patchDevMachineTemplate(context.Background(), tt.template, tt.variables)
			if tt.expectedErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(tt.template).To(BeComparableTo(tt.expectedTemplate))
		})
	}
}

func Test_patchDevMachinePoolTemplate(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name             string
		template         *infrav1.DevMachinePoolTemplate
		variables        map[string]apiextensionsv1.JSON
		expectedTemplate *infrav1.DevMachinePoolTemplate
		expectedErr      bool
	}{
		{
			name:             "fails if builtin.controlPlane.version nor builtin.machinePool.version is not set",
			template:         &infrav1.DevMachinePoolTemplate{},
			variables:        nil,
			expectedTemplate: &infrav1.DevMachinePoolTemplate{},
			expectedErr:      true,
		},
		{
			name: "sets customImage for templates linked to ControlPlane",
			template: &infrav1.DevMachinePoolTemplate{
				Spec: infrav1.DevMachinePoolTemplateSpec{
					Template: infrav1.DevMachinePoolTemplateResource{
						Spec: infrav1.DevMachinePoolSpec{
							Backend: infrav1.DevMachinePoolBackendSpec{
								Docker: &infrav1.DockerMachinePoolBackendSpec{},
							},
						},
					},
				},
			},
			variables: map[string]apiextensionsv1.JSON{
				runtimehooksv1.BuiltinsName: {Raw: toJSON(runtimehooksv1.Builtins{
					ControlPlane: &runtimehooksv1.ControlPlaneBuiltins{
						Version: "v1.23.0",
					},
					MachinePool: &runtimehooksv1.MachinePoolBuiltins{
						Class:   "default-worker",
						Version: "v1.23.0",
					},
				})},
			},
			expectedTemplate: &infrav1.DevMachinePoolTemplate{
				Spec: infrav1.DevMachinePoolTemplateSpec{
					Template: infrav1.DevMachinePoolTemplateResource{
						Spec: infrav1.DevMachinePoolSpec{
							Backend: infrav1.DevMachinePoolBackendSpec{
								Docker: &infrav1.DockerMachinePoolBackendSpec{
									CustomImage: "kindest/node:v1.23.0",
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(*testing.T) {
			err := patchDevMachinePoolTemplate(context.Background(), tt.template, tt.variables)
			if tt.expectedErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(tt.template).To(BeComparableTo(tt.expectedTemplate))
		})
	}
}

// Note: given that we are testing functions used for modifying templates inside GeneratePatches it is not
// required to test GeneratePatches for all the sub-cases; we are only testing that everything comes together as expected.
// NOTE: custom RuntimeExtension must test specif logic added to GeneratePatches, if any.
func TestHandler_GeneratePatches(t *testing.T) {
	h := NewExtensionHandlers()
	controlPlaneVarsV123WithMaxSurge := []runtimehooksv1.Variable{
		newVariable(runtimehooksv1.BuiltinsName, runtimehooksv1.Builtins{
			ControlPlane: &runtimehooksv1.ControlPlaneBuiltins{
				Version: "v1.23.0",
			},
		}),
		newVariable("kubeadmControlPlaneMaxSurge", "3"),
	}
	imageRepositoryVar := []runtimehooksv1.Variable{
		newVariable("imageRepository", "docker.io"),
	}
	machineDeploymentVars123 := []runtimehooksv1.Variable{
		newVariable(runtimehooksv1.BuiltinsName, runtimehooksv1.Builtins{
			MachineDeployment: &runtimehooksv1.MachineDeploymentBuiltins{
				Class:   "default-worker",
				Version: "v1.23.0",
			},
		}),
	}
	machinePoolVars123 := []runtimehooksv1.Variable{
		newVariable(runtimehooksv1.BuiltinsName, runtimehooksv1.Builtins{
			MachinePool: &runtimehooksv1.MachinePoolBuiltins{
				Class:   "default-worker",
				Version: "v1.23.0",
			},
		}),
	}
	// Have to set GVK because we directly Marshal to JSON.
	kubeadmControlPlaneTemplate := controlplanev1.KubeadmControlPlaneTemplate{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmControlPlaneTemplate",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
	}
	devMachineTemplate := infrav1.DevMachineTemplate{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DevMachineTemplate",
			APIVersion: infrav1.GroupVersion.String(),
		},
		Spec: infrav1.DevMachineTemplateSpec{
			Template: infrav1.DevMachineTemplateResource{
				Spec: infrav1.DevMachineSpec{
					Backend: infrav1.DevMachineBackendSpec{
						Docker: &infrav1.DockerMachineBackendSpec{},
					},
				},
			},
		},
	}
	devMachinePoolTemplate := infrav1.DevMachinePoolTemplate{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DevMachinePoolTemplate",
			APIVersion: infrav1.GroupVersion.String(),
		},
		Spec: infrav1.DevMachinePoolTemplateSpec{
			Template: infrav1.DevMachinePoolTemplateResource{
				Spec: infrav1.DevMachinePoolSpec{
					Backend: infrav1.DevMachinePoolBackendSpec{
						Docker: &infrav1.DockerMachinePoolBackendSpec{},
					},
				},
			},
		},
	}
	devClusterTemplate := infrav1.DevClusterTemplate{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DevClusterTemplate",
			APIVersion: infrav1.GroupVersion.String(),
		},
		Spec: infrav1.DevClusterTemplateSpec{
			Template: infrav1.DevClusterTemplateResource{
				Spec: infrav1.DevClusterSpec{
					Backend: infrav1.DevClusterBackendSpec{
						Docker: &infrav1.DockerClusterBackendSpec{},
					},
				},
			},
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
				requestItem("2", devMachineTemplate, controlPlaneVarsV123WithMaxSurge),
				requestItem("3", devMachineTemplate, machineDeploymentVars123),
				requestItem("4", devClusterTemplate, imageRepositoryVar),
				requestItem("6", devMachinePoolTemplate, machinePoolVars123),
			},
			expectedResponse: &runtimehooksv1.GeneratePatchesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusSuccess,
				},
				Items: []runtimehooksv1.GeneratePatchesResponseItem{
					responseItem("1", `
[ {
  "op" : "add",
  "path" : "/spec",
  "value" : {
    "template" : {
      "spec" : {
        "kubeadmConfigSpec" : {
          "clusterConfiguration" : {
            "apiServer" : {
              "extraArgs" : [ {
                "name" : "v",
                "value" : "2"
              } ]
            },
            "controllerManager" : {
              "extraArgs" : [ {
                "name" : "v",
                "value" : "2"
              } ]
            },
            "scheduler" : {
              "extraArgs" : [ {
                "name" : "v",
                "value" : "2"
              } ]
            }
          },
          "initConfiguration" : {
            "nodeRegistration" : {
              "kubeletExtraArgs" : [ {
                "name" : "v",
                "value" : "2"
              },{
                "name" : "node-labels",
                "value" : "kubernetesVersion=v1.23.0"
              } ]
            }
          },
          "joinConfiguration" : {
            "nodeRegistration" : {
              "kubeletExtraArgs" : [ {
                "name" : "v",
                "value" : "2"
              },{
                "name" : "node-labels",
                "value" : "kubernetesVersion=v1.23.0"
              } ]
            }
          }
        },
        "rollout" : {
          "strategy" : {
            "rollingUpdate" : {
              "maxSurge" : 3
            },
            "type" : "RollingUpdate"
          }
        }
      }
    }
  }
} ]`),
					responseItem("2", `[
{"op":"add","path":"/spec/template/spec/backend/docker/customImage","value":"kindest/node:v1.23.0"}
]`),
					responseItem("3", `[
{"op":"add","path":"/spec/template/spec/backend/docker/customImage","value":"kindest/node:v1.23.0"}
]`),
					responseItem("4", `[
{"op":"add","path":"/spec/template/spec/backend/docker/loadBalancer/imageRepository","value":"docker.io"}
]`),
					responseItem("6", `[
{"op":"add","path":"/spec/template/spec/backend/docker/customImage","value":"kindest/node:v1.23.0"}
]`),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(*testing.T) {
			g := NewWithT(t)

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
