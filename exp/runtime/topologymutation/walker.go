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
	"context"
	"encoding/json"

	mergepatch "github.com/evanphx/json-patch/v5"
	"github.com/pkg/errors"
	"gomodules.xyz/jsonpatch/v2"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"

	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
)

// WalkTemplatesOption is some configuration that modifies WalkTemplates behavior.
type WalkTemplatesOption interface {
	// ApplyToWalkTemplates applies this configuration to the given WalkTemplatesOptions.
	ApplyToWalkTemplates(*WalkTemplatesOptions)
}

// WalkTemplatesOptions contains options for WalkTemplates behavior.
type WalkTemplatesOptions struct {
	failForUnknownTypes bool
	patchFormat         runtimehooksv1.PatchType
	// TODO: add the possibility to set patchFormat for single patches only, eg. via a func(requestItem) format.
}

// newWalkTemplatesOptions returns a WalkTemplatesOptions with default values.
func newWalkTemplatesOptions() *WalkTemplatesOptions {
	return &WalkTemplatesOptions{
		patchFormat: runtimehooksv1.JSONPatchType,
	}
}

// FailForUnknownTypes defines if WalkTemplates should fail when processing unknown types.
// If not set unknown types will be silently ignored, which allows to the WalkTemplates decoder to
// be configured only the API types it cares about.
type FailForUnknownTypes struct{}

// ApplyToWalkTemplates applies this configuration to the given WalkTemplatesOptions.
func (f FailForUnknownTypes) ApplyToWalkTemplates(in *WalkTemplatesOptions) {
	in.failForUnknownTypes = true
}

// PatchFormat defines the patch format that WalkTemplates should generate.
// If not set, JSONPatchType will be used.
type PatchFormat struct {
	Format runtimehooksv1.PatchType
}

// ApplyToWalkTemplates applies this configuration to the given WalkTemplatesOptions.
func (d PatchFormat) ApplyToWalkTemplates(in *WalkTemplatesOptions) {
	in.patchFormat = d.Format
}

// WalkTemplates walks through all templates of a GeneratePatchesRequest and calls the mutateFunc.
// By using walk templates it is possible to implement patches using typed API objects, which makes code
// easier to read and less error prone than using unstructured or working with raw json/yaml.
// Also, by using this func it is possible to ignore most of the details of the GeneratePatchesRequest
// and GeneratePatchesResponse messages format and focus on writing patches/modifying the templates.
func WalkTemplates(ctx context.Context, decoder runtime.Decoder, req *runtimehooksv1.GeneratePatchesRequest,
	resp *runtimehooksv1.GeneratePatchesResponse, mutateFunc func(ctx context.Context, obj runtime.Object,
		variables map[string]apiextensionsv1.JSON, holderRef runtimehooksv1.HolderReference) error, opts ...WalkTemplatesOption) {
	log := ctrl.LoggerFrom(ctx)
	globalVariables := ToMap(req.Variables)

	options := newWalkTemplatesOptions()
	for _, o := range opts {
		o.ApplyToWalkTemplates(options)
	}

	// For all the templates in a request.
	// TODO: add a notion of ordering the patch implementers can rely on. Ideally ordering could be pluggable via options.
	for _, requestItem := range req.Items {
		// Computes the variables that apply to the template, by merging global and template variables.
		templateVariables, err := MergeVariableMaps(globalVariables, ToMap(requestItem.Variables))
		if err != nil {
			resp.Status = runtimehooksv1.ResponseStatusFailure
			resp.Message = err.Error()
			return
		}

		// Convert the template object into a typed object.
		original, _, err := decoder.Decode(requestItem.Object.Raw, nil, requestItem.Object.Object)
		if err != nil {
			if options.failForUnknownTypes {
				resp.Status = runtimehooksv1.ResponseStatusFailure
				resp.Message = err.Error()
				return
			}
			// Continue, object has a type which hasn't been registered with the scheme.
			continue
		}

		// Setup contextual logging for the requestItem.
		holderRefGV, err := schema.ParseGroupVersion(requestItem.HolderReference.APIVersion)
		if err != nil {
			resp.Status = runtimehooksv1.ResponseStatusFailure
			resp.Message = errors.Wrapf(err, "error generating patches - HolderReference apiVersion %q is not in valid format", requestItem.HolderReference.APIVersion).Error()
			return
		}
		requestItemLog := log.WithValues(
			"template", logRef{
				Group:   original.GetObjectKind().GroupVersionKind().Group,
				Version: original.GetObjectKind().GroupVersionKind().Version,
				Kind:    original.GetObjectKind().GroupVersionKind().Kind,
			},
			"holder", logRef{
				Group:     holderRefGV.Group,
				Version:   holderRefGV.Version,
				Kind:      requestItem.HolderReference.Kind,
				Namespace: requestItem.HolderReference.Namespace,
				Name:      requestItem.HolderReference.Name,
			},
		)
		requestItemCtx := ctrl.LoggerInto(ctx, requestItemLog)

		// Calls the mutateFunc.
		requestItemLog.V(4).Info("Generating patch for template")
		modified := original.DeepCopyObject()
		if err := mutateFunc(requestItemCtx, modified, templateVariables, requestItem.HolderReference); err != nil {
			resp.Status = runtimehooksv1.ResponseStatusFailure
			resp.Message = err.Error()
			return
		}

		// Generate the Patch by comparing original and modified object.
		var patch []byte
		switch options.patchFormat {
		case runtimehooksv1.JSONPatchType:
			patch, err = createJSONPatch(requestItem.Object.Raw, modified)
			if err != nil {
				resp.Status = runtimehooksv1.ResponseStatusFailure
				resp.Message = err.Error()
				return
			}
		case runtimehooksv1.JSONMergePatchType:
			patch, err = createJSONMergePatch(requestItem.Object.Raw, modified)
			if err != nil {
				resp.Status = runtimehooksv1.ResponseStatusFailure
				resp.Message = err.Error()
				return
			}
		}

		resp.Items = append(resp.Items, runtimehooksv1.GeneratePatchesResponseItem{
			UID:       requestItem.UID,
			PatchType: options.patchFormat,
			Patch:     patch,
		})
		requestItemLog.V(5).Info("Generated patch", "uid", requestItem.UID, "patch", string(patch))
	}

	resp.Status = runtimehooksv1.ResponseStatusSuccess
}

// createJSONPatch creates a RFC 6902 JSON patch from the original and the modified object.
func createJSONPatch(marshalledOriginal []byte, modified runtime.Object) ([]byte, error) {
	marshalledModified, err := json.Marshal(modified)
	if err != nil {
		return nil, errors.Errorf("failed to marshal modified object: %v", err)
	}

	patch, err := jsonpatch.CreatePatch(marshalledOriginal, marshalledModified)
	if err != nil {
		return nil, errors.Errorf("failed to create patch: %v", err)
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return nil, errors.Errorf("failed to marshal patch: %v", err)
	}

	return patchBytes, nil
}

// createJSONMergePatch creates a RFC 7396 JSON merge patch from the original and the modified object.
func createJSONMergePatch(marshalledOriginal []byte, modified runtime.Object) ([]byte, error) {
	marshalledModified, err := json.Marshal(modified)
	if err != nil {
		return nil, errors.Errorf("failed to marshal modified object: %v", err)
	}

	patch, err := mergepatch.CreateMergePatch(marshalledOriginal, marshalledModified)
	if err != nil {
		return nil, errors.Errorf("failed to create patch: %v", err)
	}

	return patch, nil
}
