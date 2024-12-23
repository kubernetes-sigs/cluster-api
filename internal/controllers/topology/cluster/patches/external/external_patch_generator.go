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

// Package external implements the external patch generator.
package external

import (
	"context"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	runtimeclient "sigs.k8s.io/cluster-api/exp/runtime/client"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/patches/api"
)

// externalPatchGenerator generates JSON patches for a GeneratePatchesRequest based on a ClusterClassPatch.
type externalPatchGenerator struct {
	runtimeClient runtimeclient.Client
	patch         *clusterv1.ClusterClassPatch
}

// NewGenerator returns a new external Generator from a given ClusterClassPatch object.
func NewGenerator(runtimeClient runtimeclient.Client, patch *clusterv1.ClusterClassPatch) api.Generator {
	return &externalPatchGenerator{
		runtimeClient: runtimeClient,
		patch:         patch,
	}
}

func (e externalPatchGenerator) Generate(ctx context.Context, forObject client.Object, req *runtimehooksv1.GeneratePatchesRequest) (*runtimehooksv1.GeneratePatchesResponse, error) {
	if !feature.Gates.Enabled(feature.RuntimeSDK) {
		return nil, errors.Errorf("can not use external patch %q if RuntimeSDK feature flag is disabled", *e.patch.External.GenerateExtension)
	}

	// Set the settings defined in external patch definition on the request object.
	// These settings will override overlapping keys defined in ExtensionConfig settings.
	req.Settings = e.patch.External.Settings
	// The req object is re-used across multiple calls to the patch generator.
	// Reset the settings so that the request does not remain modified here.
	defer func() {
		req.Settings = nil
	}()

	resp := &runtimehooksv1.GeneratePatchesResponse{}
	err := e.runtimeClient.CallExtension(ctx, runtimehooksv1.GeneratePatches, forObject, *e.patch.External.GenerateExtension, req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
