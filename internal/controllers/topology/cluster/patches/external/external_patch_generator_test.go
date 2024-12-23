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

package external

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimeclient "sigs.k8s.io/cluster-api/exp/runtime/client"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/feature"
)

func TestExternalPatchGenerator_Generate(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, true)

	ctx := context.Background()
	tests := []struct {
		name          string
		runtimeClient *fakeRuntimeClient
		patch         *clusterv1.ClusterClassPatch
		request       *runtimehooksv1.GeneratePatchesRequest
		assertRequest func(g *WithT, actualRequest runtimehooksv1.RequestObject)
	}{
		{
			name:          "Request should not have any settings if there are no settings in external patch definition",
			runtimeClient: &fakeRuntimeClient{},
			patch: &clusterv1.ClusterClassPatch{
				Name:        "",
				Description: "",
				EnabledIf:   nil,
				Definitions: nil,
				External: &clusterv1.ExternalPatchDefinition{
					GenerateExtension: ptr.To("test-generate-extension"),
					Settings:          nil,
				},
			},
			request: &runtimehooksv1.GeneratePatchesRequest{},
			assertRequest: func(g *WithT, actualRequest runtimehooksv1.RequestObject) {
				actualSettings := actualRequest.GetSettings()
				g.Expect(actualSettings).To(BeNil())
			},
		},
		{
			name:          "Request should have setting if setting are defined in external path definition",
			runtimeClient: &fakeRuntimeClient{},
			patch: &clusterv1.ClusterClassPatch{
				Name:        "",
				Description: "",
				EnabledIf:   nil,
				Definitions: nil,
				External: &clusterv1.ExternalPatchDefinition{
					GenerateExtension: ptr.To("test-generate-extension"),
					Settings: map[string]string{
						"key1": "value1",
					},
				},
			},
			request: &runtimehooksv1.GeneratePatchesRequest{},
			assertRequest: func(g *WithT, actualRequest runtimehooksv1.RequestObject) {
				actualSettings := actualRequest.GetSettings()
				expectedSettings := map[string]string{
					"key1": "value1",
				}
				g.Expect(actualSettings).To(Equal(expectedSettings))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			externalPatchGenerator := NewGenerator(tt.runtimeClient, tt.patch)
			_, _ = externalPatchGenerator.Generate(ctx, &clusterv1.Cluster{}, tt.request)
			tt.assertRequest(g, tt.runtimeClient.callExtensionRequest)
		})
	}
}

var _ runtimeclient.Client = &fakeRuntimeClient{}

type fakeRuntimeClient struct {
	callExtensionRequest runtimehooksv1.RequestObject
}

func (f *fakeRuntimeClient) WarmUp(_ *runtimev1.ExtensionConfigList) error {
	panic("implement me")
}

func (f *fakeRuntimeClient) IsReady() bool {
	panic("implement me")
}

func (f *fakeRuntimeClient) Discover(_ context.Context, _ *runtimev1.ExtensionConfig) (*runtimev1.ExtensionConfig, error) {
	panic("implement me")
}

func (f *fakeRuntimeClient) Register(_ *runtimev1.ExtensionConfig) error {
	panic("implement me")
}

func (f *fakeRuntimeClient) Unregister(_ *runtimev1.ExtensionConfig) error {
	panic("implement me")
}

func (f *fakeRuntimeClient) CallAllExtensions(_ context.Context, _ runtimecatalog.Hook, _ metav1.Object, _ runtimehooksv1.RequestObject, _ runtimehooksv1.ResponseObject) error {
	panic("implement me")
}

func (f *fakeRuntimeClient) CallExtension(_ context.Context, _ runtimecatalog.Hook, _ metav1.Object, _ string, request runtimehooksv1.RequestObject, _ runtimehooksv1.ResponseObject, _ ...runtimeclient.CallExtensionOption) error {
	// Keep a copy of the request object.
	// We keep a copy because the request is modified after the call is made. So we keep a copy to perform assertions.
	f.callExtensionRequest = request.DeepCopyObject().(runtimehooksv1.RequestObject)
	return nil
}
