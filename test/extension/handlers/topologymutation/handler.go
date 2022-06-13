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

// Package topologymutation contains the handlers for the topologymutation webhook.
package topologymutation

import (
	"context"
	"strconv"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	ctrl "sigs.k8s.io/controller-runtime"

	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	patchvariables "sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/patches/variables"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
)

// NewHandler returns a new topology mutation Handler.
func NewHandler(scheme *runtime.Scheme) *Handler {
	return &Handler{
		decoder: serializer.NewCodecFactory(scheme).UniversalDecoder(
			infrav1.GroupVersion,
		),
	}
}

// Handler is a topology mutation handler.
type Handler struct {
	decoder runtime.Decoder
}

// GeneratePatches returns a function that generates patches for the given request.
func (h *Handler) GeneratePatches(ctx context.Context, req *runtimehooksv1.GeneratePatchesRequest, resp *runtimehooksv1.GeneratePatchesResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("GeneratePatches called")

	walkTemplates(h.decoder, req, resp, func(obj runtime.Object, variables map[string]apiextensionsv1.JSON) error {
		if dockerClusterTemplate, ok := obj.(*infrav1.DockerClusterTemplate); ok {
			if err := patchDockerClusterTemplate(dockerClusterTemplate, variables); err != nil {
				return err
			}
		}

		return nil
	})
}

// patchDockerClusterTemplate patches the DockerClusterTepmlate.
func patchDockerClusterTemplate(dockerClusterTemplate *infrav1.DockerClusterTemplate, templateVariables map[string]apiextensionsv1.JSON) error {
	// Get the variable value as JSON string.
	value, err := patchvariables.GetVariableValue(templateVariables, "lbImageRepository")
	if err != nil {
		return err
	}

	// Unquote the JSON string.
	stringValue, err := strconv.Unquote(string(value.Raw))
	if err != nil {
		return err
	}

	dockerClusterTemplate.Spec.Template.Spec.LoadBalancer.ImageRepository = stringValue
	return nil
}

// ValidateTopology returns a function that validates the given request.
func (h *Handler) ValidateTopology(ctx context.Context, _ *runtimehooksv1.ValidateTopologyRequest, resp *runtimehooksv1.ValidateTopologyResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("ValidateTopology called")

	resp.Status = runtimehooksv1.ResponseStatusSuccess
}
