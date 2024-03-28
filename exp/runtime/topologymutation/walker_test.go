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
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
)

var (
	testScheme = runtime.NewScheme()
)

func init() {
	_ = controlplanev1.AddToScheme(testScheme)
	_ = bootstrapv1.AddToScheme(testScheme)
}

func Test_WalkTemplates(t *testing.T) {
	g := NewWithT(t)

	decoder := serializer.NewCodecFactory(testScheme).UniversalDecoder(
		controlplanev1.GroupVersion,
		bootstrapv1.GroupVersion,
	)
	mutatingFunc := func(_ context.Context, obj runtime.Object, _ map[string]apiextensionsv1.JSON, _ runtimehooksv1.HolderReference) error {
		switch obj := obj.(type) {
		case *controlplanev1.KubeadmControlPlaneTemplate:
			obj.Annotations = map[string]string{"a": "a"}
		case *bootstrapv1.KubeadmConfigTemplate:
			obj.Annotations = map[string]string{"b": "b"}
		}
		return nil
	}
	kubeadmControlPlaneTemplate := controlplanev1.KubeadmControlPlaneTemplate{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmControlPlaneTemplate",
			APIVersion: controlplanev1.GroupVersion.String(),
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
		globalVariables  []runtimehooksv1.Variable
		requestItems     []runtimehooksv1.GeneratePatchesRequestItem
		expectedResponse *runtimehooksv1.GeneratePatchesResponse
		options          []WalkTemplatesOption
	}{
		{
			name: "Fails for invalid builtin variables",
			globalVariables: []runtimehooksv1.Variable{
				newVariable(runtimehooksv1.BuiltinsName, runtimehooksv1.Builtins{
					Cluster: &runtimehooksv1.ClusterBuiltins{
						Name: "test",
					},
				}),
			},
			requestItems: []runtimehooksv1.GeneratePatchesRequestItem{
				requestItem("1", kubeadmControlPlaneTemplate, []runtimehooksv1.Variable{
					newVariable(runtimehooksv1.BuiltinsName, "{invalid-builtin-value}"),
				}),
			},
			expectedResponse: &runtimehooksv1.GeneratePatchesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status:  runtimehooksv1.ResponseStatusFailure,
					Message: fmt.Sprintf("failed to merge builtin variables: failed to unmarshal builtin variable: json: cannot unmarshal string into Go value of type %s.Builtins", runtimehooksv1.GroupVersion.Version),
				},
			},
		},
		{
			name: "Silently ignore unknown types if FailForUnknownTypes option is not set",
			requestItems: []runtimehooksv1.GeneratePatchesRequestItem{
				{
					UID: types.UID("1"),
					Object: runtime.RawExtension{
						Raw: []byte("{\"kind\":\"Unknown\",\"apiVersion\":\"controlplane.cluster.x-k8s.io/v1beta1\"}"),
					},
				},
			},
			expectedResponse: &runtimehooksv1.GeneratePatchesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusSuccess,
				},
			},
		},
		{
			name: "Fails for unknown types if FailForUnknownTypes option is set",
			requestItems: []runtimehooksv1.GeneratePatchesRequestItem{
				{
					UID: types.UID("1"),
					Object: runtime.RawExtension{
						Raw: []byte("{\"kind\":\"Unknown\",\"apiVersion\":\"controlplane.cluster.x-k8s.io/v1beta1\"}"),
					},
				},
			},
			expectedResponse: &runtimehooksv1.GeneratePatchesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusFailure,
					Message: "no kind \"Unknown\" is registered for version \"controlplane.cluster.x-k8s." +
						"io/v1beta1\" in scheme",
				},
			},
			options: []WalkTemplatesOption{
				FailForUnknownTypes{},
			},
		},
		{
			name: "Fails for invalid holder ref",
			requestItems: []runtimehooksv1.GeneratePatchesRequestItem{
				{
					UID: types.UID("1"),
					Object: runtime.RawExtension{
						Raw: toJSON(kubeadmConfigTemplate),
					},
					HolderReference: runtimehooksv1.HolderReference{
						APIVersion: "invalid/cluster.x-k8s.io/v1beta1",
					},
				},
			},
			expectedResponse: &runtimehooksv1.GeneratePatchesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusFailure,
					Message: "error generating patches - HolderReference apiVersion \"invalid/cluster.x-k8s." +
						"io/v1beta1\" is not in valid format: unexpected GroupVersion string: invalid/cluster.x-k8s." +
						"io/v1beta1",
				},
			},
			options: []WalkTemplatesOption{
				FailForUnknownTypes{},
			},
		},
		{
			name: "Creates Json patches by default",
			requestItems: []runtimehooksv1.GeneratePatchesRequestItem{
				requestItem("1", kubeadmControlPlaneTemplate, nil),
				requestItem("2", kubeadmConfigTemplate, nil),
			},
			expectedResponse: &runtimehooksv1.GeneratePatchesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusSuccess,
				},
				Items: []runtimehooksv1.GeneratePatchesResponseItem{
					responseItem("1", "[{\"op\":\"add\",\"path\":\"/metadata/annotations\",\"value\":{\"a\":\"a\"}}]", runtimehooksv1.JSONPatchType),
					responseItem("2", "[{\"op\":\"add\",\"path\":\"/metadata/annotations\",\"value\":{\"b\":\"b\"}}]", runtimehooksv1.JSONPatchType),
				},
			},
		},
		{
			name: "Creates Json patches if option PatchFormat is set to JSONPatchType",
			requestItems: []runtimehooksv1.GeneratePatchesRequestItem{
				requestItem("1", kubeadmControlPlaneTemplate, nil),
				requestItem("2", kubeadmConfigTemplate, nil),
			},
			expectedResponse: &runtimehooksv1.GeneratePatchesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusSuccess,
				},
				Items: []runtimehooksv1.GeneratePatchesResponseItem{
					responseItem("1", "[{\"op\":\"add\",\"path\":\"/metadata/annotations\",\"value\":{\"a\":\"a\"}}]", runtimehooksv1.JSONPatchType),
					responseItem("2", "[{\"op\":\"add\",\"path\":\"/metadata/annotations\",\"value\":{\"b\":\"b\"}}]", runtimehooksv1.JSONPatchType),
				},
			},
			options: []WalkTemplatesOption{
				PatchFormat{Format: runtimehooksv1.JSONPatchType},
			},
		},
		{
			name: "Creates Json Merge patches if option PatchFormat is set to JSONMergePatchType",
			requestItems: []runtimehooksv1.GeneratePatchesRequestItem{
				requestItem("1", kubeadmControlPlaneTemplate, nil),
				requestItem("2", kubeadmConfigTemplate, nil),
			},
			expectedResponse: &runtimehooksv1.GeneratePatchesResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusSuccess,
				},
				Items: []runtimehooksv1.GeneratePatchesResponseItem{
					responseItem("1", "{\"metadata\":{\"annotations\":{\"a\":\"a\"}}}", runtimehooksv1.JSONMergePatchType),
					responseItem("2", "{\"metadata\":{\"annotations\":{\"b\":\"b\"}}}", runtimehooksv1.JSONMergePatchType),
				},
			},
			options: []WalkTemplatesOption{
				PatchFormat{Format: runtimehooksv1.JSONMergePatchType},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(*testing.T) {
			response := &runtimehooksv1.GeneratePatchesResponse{}
			request := &runtimehooksv1.GeneratePatchesRequest{Variables: tt.globalVariables, Items: tt.requestItems}

			WalkTemplates(context.Background(), decoder, request, response, mutatingFunc, tt.options...)

			g.Expect(response.Status).To(Equal(tt.expectedResponse.Status))
			g.Expect(response.Message).To(ContainSubstring(tt.expectedResponse.Message))
			for i, item := range response.Items {
				expectedItem := tt.expectedResponse.Items[i]
				g.Expect(item.PatchType).To(Equal(expectedItem.PatchType))
				g.Expect(item.UID).To(Equal(expectedItem.UID))

				switch item.PatchType {
				case runtimehooksv1.JSONPatchType:
					// Note; the order of the individual patch operations in Items[].Patch is not deterministic so we unmarshal
					// to an array and check that the arrays hold equivalent items.
					var actualPatchOps []map[string]interface{}
					var expectedPatchOps []map[string]interface{}
					g.Expect(json.Unmarshal(item.Patch, &actualPatchOps)).To(Succeed())
					g.Expect(json.Unmarshal(expectedItem.Patch, &expectedPatchOps)).To(Succeed())
					g.Expect(actualPatchOps).To(ConsistOf(expectedPatchOps))
				case runtimehooksv1.JSONMergePatchType:
					g.Expect(item.Patch).To(Equal(expectedItem.Patch))
				}
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
func responseItem(uid, patch string, format runtimehooksv1.PatchType) runtimehooksv1.GeneratePatchesResponseItem {
	return runtimehooksv1.GeneratePatchesResponseItem{
		UID:       types.UID(uid),
		PatchType: format,
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
