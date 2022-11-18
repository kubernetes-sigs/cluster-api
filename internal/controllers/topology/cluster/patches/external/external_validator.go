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
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/patches/api"
	runtimeclient "sigs.k8s.io/cluster-api/internal/runtime/client"
)

// externalValidator validates templates.
type externalValidator struct {
	runtimeClient runtimeclient.Client
	patch         *clusterv1.ClusterClassPatch
}

// NewValidator returns a new external Validator from a given ClusterClassPatch object.
func NewValidator(runtimeClient runtimeclient.Client, patch *clusterv1.ClusterClassPatch) api.Validator {
	return &externalValidator{
		runtimeClient: runtimeClient,
		patch:         patch,
	}
}

func (e externalValidator) Validate(ctx context.Context, forObject client.Object, req *runtimehooksv1.ValidateTopologyRequest) (*runtimehooksv1.ValidateTopologyResponse, error) {
	if !feature.Gates.Enabled(feature.RuntimeSDK) {
		return nil, errors.Errorf("can not use external patch %q if RuntimeSDK feature flag is disabled", *e.patch.External.ValidateExtension)
	}

	// Set the settings defined in external patch definition on the request object.
	// These settings will override overlapping keys defined in ExtensionConfig settings.
	req.Settings = e.patch.External.Settings
	// The req object is re-used across multiple calls to the patch generator.
	// Reset the settings so that the request does not remain modified here.
	defer func() {
		req.Settings = nil
	}()

	resp := &runtimehooksv1.ValidateTopologyResponse{}
	err := e.runtimeClient.CallExtension(ctx, runtimehooksv1.ValidateTopology, forObject, *e.patch.External.ValidateExtension, req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
