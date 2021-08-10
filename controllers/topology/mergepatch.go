/*
Copyright 2021 The Kubernetes Authors.

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

package topology

import (
	"bytes"
	"context"
	"encoding/json"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type mergePatchHelper struct {
	client client.Client

	// original holds the object to which the patch should apply to, to be used in the Patch method.
	original client.Object

	// data holds the raw merge patch in json format.
	data []byte
}

// newMergePatchHelper will return a patch that yields the modified document when applied to the original document.
// NOTE: In the case of ClusterTopologyReconciler, original is the current object, modified is the desired object, and
// the patch returns all the changes required to align current to what is defined in desired; fields not defined in desired
// are going to be preserved without changes.
func newMergePatchHelper(original, modified client.Object, c client.Client) (*mergePatchHelper, error) {
	originalJSON, err := json.Marshal(original)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal original object to json")
	}

	modifiedJSON, err := json.Marshal(modified)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal modified object to json")
	}

	originalWithModifiedJSON, err := jsonpatch.MergePatch(originalJSON, modifiedJSON)
	if err != nil {
		return nil, errors.Wrap(err, "failed to apply modified json to original json")
	}

	data, err := jsonpatch.CreateMergePatch(originalJSON, originalWithModifiedJSON)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create merge patch")
	}

	return &mergePatchHelper{
		client:   c,
		data:     data,
		original: original,
	}, nil
}

// HasChanges return true if the patch has changes.
func (h *mergePatchHelper) HasChanges() bool {
	return !bytes.Equal(h.data, []byte("{}"))
}

// Patch will attempt to apply the twoWaysPatch to the original object.
func (h *mergePatchHelper) Patch(ctx context.Context) error {
	if !h.HasChanges() {
		return nil
	}
	return h.client.Patch(ctx, h.original, client.RawPatch(types.MergePatchType, h.data))
}
