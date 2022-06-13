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
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"gomodules.xyz/jsonpatch/v2"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"

	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	patchvariables "sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/patches/variables"
)

// walk walks through all templates of a GeneratePatchesRequest and calls the mutateFunc.
func walk(decoder runtime.Decoder, req *runtimehooksv1.GeneratePatchesRequest, resp *runtimehooksv1.GeneratePatchesResponse, mutateFunc func(obj runtime.Object, variables map[string]apiextensionsv1.JSON) error) {
	globalVariables := patchvariables.ToMap(req.Variables)

	for _, requestItem := range req.Items {
		templateVariables, err := patchvariables.MergeVariableMaps(globalVariables, patchvariables.ToMap(requestItem.Variables))
		if err != nil {
			resp.Status = runtimehooksv1.ResponseStatusFailure
			resp.Message = err.Error()
			return
		}

		obj, _, err := decoder.Decode(requestItem.Object.Raw, nil, requestItem.Object.Object)
		if err != nil {
			// Continue, object has a type which hasn't been registered with the scheme.
			continue
		}

		original := obj.DeepCopyObject()

		if err := mutateFunc(obj, templateVariables); err != nil {
			resp.Status = runtimehooksv1.ResponseStatusFailure
			resp.Message = err.Error()
			return
		}

		patch, err := createPatch(original, obj)
		if err != nil {
			resp.Status = runtimehooksv1.ResponseStatusFailure
			resp.Message = err.Error()
			return
		}

		resp.Items = append(resp.Items, runtimehooksv1.GeneratePatchesResponseItem{
			UID:       requestItem.UID,
			PatchType: runtimehooksv1.JSONPatchType,
			Patch:     patch,
		})

		fmt.Printf("Generated patch (uid: %q): %q\n", requestItem.UID, string(patch))
	}

	resp.Status = runtimehooksv1.ResponseStatusSuccess
}

// createPatch creates a JSON patch from the original and the modified object.
func createPatch(original, modified runtime.Object) ([]byte, error) {
	marshalledOriginal, err := json.Marshal(original)
	if err != nil {
		return nil, errors.Errorf("failed to marshal original object: %v", err)
	}

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
