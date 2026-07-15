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
func patchObject(ctx context.Context, dest, src *unstructured.Unstructured, opts ...PatchOption) error {
	return patchUnstructured(ctx, dest, src, []patchUnstructuredFields{
		{Src: "spec.template.spec", Dest: "spec"},
		{Src: "spec.template.metadata.labels", Dest: "metadata.labels"},
		{Src: "spec.template.metadata.annotations", Dest: "metadata.annotations"},
	}, opts...)
}

// patchTemplate overwrites spec.template.spec in template with spec.template.spec of modifiedTemplate,
// while preserving the configured fields.
// For example, it's possible to patch BootstrapTemplate.spec.template.spec with a patched
// BootstrapTemplate.spec.template.spec while preserving fields configured via opts.fieldsToPreserve.
func patchTemplate(ctx context.Context, dest, src *unstructured.Unstructured, opts ...PatchOption) error {
	return patchUnstructured(ctx, dest, src, []patchUnstructuredFields{
		{Src: "metadata.labels", Dest: "metadata.labels"},
		{Src: "metadata.annotations", Dest: "metadata.annotations"},
		{Src: "spec.template.spec", Dest: "spec.template.spec"},
		{Src: "spec.template.metadata.labels", Dest: "spec.template.metadata.labels"},
		{Src: "spec.template.metadata.annotations", Dest: "spec.template.metadata.annotations"},
	}, opts...)
}

type patchUnstructuredFields struct {
	Src  string
	Dest string
}

// patchUnstructured overwrites original.destSpecPath with modified.srcSpecPath.
// NOTE: Original won't be changed at all, if there is no diff.
func patchUnstructured(ctx context.Context, dest, src *unstructured.Unstructured, patchFields []patchUnstructuredFields, opts ...PatchOption) error {
	log := ctrl.LoggerFrom(ctx)

	patchOptions := &PatchOptions{}
	patchOptions.ApplyOptions(opts)

	// Create a copy to store the result of the patching temporarily.
	patched := dest.DeepCopy()

	fields := make([]patchutil.CopyFieldsInputField, 0, len(patchFields))
	for _, patchField := range patchFields {
		fields = append(fields, patchutil.CopyFieldsInputField{
			Src:  patchField.Src,
			Dest: patchField.Dest,
		})
	}

	// copySpec overwrites patched.destSpecPath with modified.srcSpecPath.
	if err := patchutil.CopyFields(patchutil.CopyFieldsInput{
		Src:              src,
		Dest:             patched,
		Fields:           fields,
		FieldsToPreserve: patchOptions.preserveFields,
	}); err != nil {
		return errors.Wrapf(err, "failed to apply patch to %s %s", dest.GetKind(), klog.KObj(dest))
	}

	// Calculate diff.
	diff, err := calculateDiff(dest, patched)
	if err != nil {
		return errors.Wrapf(err, "failed to apply patch to %s %s: failed to calculate diff", dest.GetKind(), klog.KObj(dest))
	}

	// Return if there is no diff.
	if bytes.Equal(diff, []byte("{}")) {
		return nil
	}

	// Log the delta between the object before and after applying the accumulated patches.
	log.V(4).Info(fmt.Sprintf("Applying accumulated patches to desired state of %s", dest.GetKind()), dest.GetKind(), klog.KObj(dest), "patch", string(diff))

	// Overwrite original.
	*dest = *patched
	return nil
}

// calculateDiff calculates the diff between two Unstructured objects.
func calculateDiff(original, patched *unstructured.Unstructured) ([]byte, error) {
	originalDiffObject := map[string]any{
		"metadata": map[string]any{
			"annotations": original.GetAnnotations(),
			"labels":      original.GetLabels(),
		},
		"spec": original.Object["spec"],
	}
	patchedDiffObject := map[string]any{
		"metadata": map[string]any{
			"annotations": patched.GetAnnotations(),
			"labels":      patched.GetLabels(),
		},
		"spec": patched.Object["spec"],
	}

	originalJSON, err := json.Marshal(originalDiffObject)
	if err != nil {
		return nil, errors.Errorf("failed to marshal original object")
	}

	patchedJSON, err := json.Marshal(patchedDiffObject)
	if err != nil {
		return nil, errors.Errorf("failed to marshal patched object")
	}

	diff, err := jsonpatch.CreateMergePatch(originalJSON, patchedJSON)
	if err != nil {
		return nil, errors.Errorf("failed to diff objects")
	}
	return diff, nil
}
