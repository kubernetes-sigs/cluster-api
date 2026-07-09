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

package patches

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/cluster-api/internal/contract"
	patchutil "sigs.k8s.io/cluster-api/internal/util/patch"
)

// PatchOption represents an option for the patchObject and patchTemplate funcs.
type PatchOption interface {
	// ApplyToHelper applies configuration to the given options.
	ApplyToHelper(*PatchOptions)
}

// PatchOptions contains options for patchObject and patchTemplate.
type PatchOptions struct {
	preserveFields []contract.Path
}

// ApplyOptions applies the given patch options.
func (o *PatchOptions) ApplyOptions(opts []PatchOption) {
	for _, opt := range opts {
		opt.ApplyToHelper(o)
	}
}

// PreserveFields instructs the patch func to preserve fields.
type PreserveFields []contract.Path

// ApplyToHelper applies this configuration to the given patch options.
func (i PreserveFields) ApplyToHelper(opts *PatchOptions) {
	opts.preserveFields = i
}

// patchObject overwrites spec in object with spec.template.spec of modifiedObject,
// while preserving the configured fields.
// For example, ControlPlane.spec will be overwritten with the patched
// ControlPlaneTemplate.spec.template.spec but preserving spec.version and spec.replicas
// which are previously set by the topology controller and shouldn't be overwritten.
func patchObject(ctx context.Context, object, modifiedObject *unstructured.Unstructured, opts ...PatchOption) error {
	return patchUnstructured(ctx, object, modifiedObject, "spec.template.spec", "spec", opts...)
}

// patchTemplate overwrites spec.template.spec in template with spec.template.spec of modifiedTemplate,
// while preserving the configured fields.
// For example, it's possible to patch BootstrapTemplate.spec.template.spec with a patched
// BootstrapTemplate.spec.template.spec while preserving fields configured via opts.fieldsToPreserve.
func patchTemplate(ctx context.Context, template, modifiedTemplate *unstructured.Unstructured, opts ...PatchOption) error {
	return patchUnstructured(ctx, template, modifiedTemplate, "spec.template.spec", "spec.template.spec", opts...)
}

// patchUnstructured overwrites original.destSpecPath with modified.srcSpecPath.
// NOTE: Original won't be changed at all, if there is no diff.
func patchUnstructured(ctx context.Context, original, modified *unstructured.Unstructured, srcSpecPath, destSpecPath string, opts ...PatchOption) error {
	log := ctrl.LoggerFrom(ctx)

	patchOptions := &PatchOptions{}
	patchOptions.ApplyOptions(opts)

	// Create a copy to store the result of the patching temporarily.
	patched := original.DeepCopy()

	// copySpec overwrites patched.destSpecPath with modified.srcSpecPath.
	if err := patchutil.CopySpec(patchutil.CopySpecInput{
		Src:              modified,
		Dest:             patched,
		SrcSpecPath:      srcSpecPath,
		DestSpecPath:     destSpecPath,
		FieldsToPreserve: patchOptions.preserveFields,
	}); err != nil {
		return errors.Wrapf(err, "failed to apply patch to %s %s", original.GetKind(), klog.KObj(original))
	}

	// Calculate diff.
	diff, err := calculateDiff(original, patched)
	if err != nil {
		return errors.Wrapf(err, "failed to apply patch to %s %s: failed to calculate diff", original.GetKind(), klog.KObj(original))
	}

	// Return if there is no diff.
	if bytes.Equal(diff, []byte("{}")) {
		return nil
	}

	// Log the delta between the object before and after applying the accumulated patches.
	log.V(4).Info(fmt.Sprintf("Applying accumulated patches to desired state of %s", original.GetKind()), original.GetKind(), klog.KObj(original), "patch", string(diff))

	// Overwrite original.
	*original = *patched
	return nil
}

// calculateDiff calculates the diff between two Unstructured objects.
func calculateDiff(original, patched *unstructured.Unstructured) ([]byte, error) {
	originalJSON, err := json.Marshal(original.Object["spec"])
	if err != nil {
		return nil, errors.Errorf("failed to marshal original object")
	}

	patchedJSON, err := json.Marshal(patched.Object["spec"])
	if err != nil {
		return nil, errors.Errorf("failed to marshal patched object")
	}

	diff, err := jsonpatch.CreateMergePatch(originalJSON, patchedJSON)
	if err != nil {
		return nil, errors.Errorf("failed to diff objects")
	}
	return diff, nil
}
