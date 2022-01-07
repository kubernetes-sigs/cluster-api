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
	"strings"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"sigs.k8s.io/cluster-api/internal/contract"
	tlog "sigs.k8s.io/cluster-api/internal/log"
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

// patchObject overwrites spec in object with spec.template.spec of patchedTemplate,
// while preserving the configured fields.
// For example, ControlPlane.spec will be overwritten with the patched
// ControlPlaneTemplate.spec.template.spec but preserving spec.version and spec.replicas
// which are previously set by the topology controller and shouldn't be overwritten.
func patchObject(ctx context.Context, object, modifiedObject *unstructured.Unstructured, opts ...PatchOption) error {
	return patchUnstructured(ctx, object, modifiedObject, "spec.template.spec", "spec", opts...)
}

// patchTemplate overwrites spec.template.spec in template with spec.template.spec of patchedTemplate,
// while preserving the configured fields.
// For example, it's possible to patch BootstrapTemplate.spec.template.spec with a patched
// BootstrapTemplate.spec.template.spec while preserving fields configured via opts.fieldsToPreserve.
func patchTemplate(ctx context.Context, template, modifiedTemplate *unstructured.Unstructured, opts ...PatchOption) error {
	return patchUnstructured(ctx, template, modifiedTemplate, "spec.template.spec", "spec.template.spec", opts...)
}

// patchUnstructured overwrites original.destSpecPath with modified.srcSpecPath.
// NOTE: Original won't be changed at all, if there is no diff.
func patchUnstructured(ctx context.Context, original, modified *unstructured.Unstructured, srcSpecPath, destSpecPath string, opts ...PatchOption) error {
	log := tlog.LoggerFrom(ctx)

	patchOptions := &PatchOptions{}
	patchOptions.ApplyOptions(opts)

	// Create a copy to store the result of the patching temporarily.
	patched := original.DeepCopy()

	// copySpec overwrites patched.destSpecPath with modified.srcSpecPath.
	if err := copySpec(copySpecInput{
		src:              modified,
		dest:             patched,
		srcSpecPath:      srcSpecPath,
		destSpecPath:     destSpecPath,
		fieldsToPreserve: patchOptions.preserveFields,
	}); err != nil {
		return errors.Wrapf(err, "failed to apply patch to %s", tlog.KObj{Obj: original})
	}

	// Calculate diff.
	diff, err := calculateDiff(original, patched)
	if err != nil {
		return errors.Wrapf(err, "failed to apply patch to %s: failed to calculate diff", tlog.KObj{Obj: original})
	}

	// Return if there is no diff.
	if bytes.Equal(diff, []byte("{}")) {
		return nil
	}

	// Log the delta between the object before and after applying the accumulated patches.
	log.V(4).WithObject(original).Infof("Applying accumulated patches to desired state: %s", string(diff))

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

// patchTemplateSpec overwrites spec in templateJSON with spec of patchedTemplateBytes.
func patchTemplateSpec(templateJSON *apiextensionsv1.JSON, patchedTemplateBytes []byte) error {
	// Convert templates to Unstructured.
	template, err := bytesToUnstructured(templateJSON.Raw)
	if err != nil {
		return errors.Wrap(err, "failed to convert template to Unstructured")
	}
	patchedTemplate, err := bytesToUnstructured(patchedTemplateBytes)
	if err != nil {
		return errors.Wrap(err, "failed to convert patched template to Unstructured")
	}

	// Copy spec from patchedTemplate to template.
	if err := copySpec(copySpecInput{
		src:          patchedTemplate,
		dest:         template,
		srcSpecPath:  "spec",
		destSpecPath: "spec",
	}); err != nil {
		return errors.Wrap(err, "failed to apply patch to template")
	}

	// Marshal template and store it in templateJSON.
	templateBytes, err := template.MarshalJSON()
	if err != nil {
		return errors.Wrapf(err, "failed to marshal patched template")
	}
	templateJSON.Raw = templateBytes
	return nil
}

type copySpecInput struct {
	src              *unstructured.Unstructured
	dest             *unstructured.Unstructured
	srcSpecPath      string
	destSpecPath     string
	fieldsToPreserve []contract.Path
}

// copySpec copies a field from a srcSpecPath in src to a destSpecPath in dest,
// while preserving fieldsToPreserve.
func copySpec(in copySpecInput) error {
	// Backup fields that should be preserved from dest.
	preservedFields := map[string]interface{}{}
	for _, field := range in.fieldsToPreserve {
		value, found, err := unstructured.NestedFieldNoCopy(in.dest.Object, field...)
		if !found {
			// Continue if the field does not exist in src. fieldsToPreserve don't have to exist.
			continue
		} else if err != nil {
			return errors.Wrapf(err, "failed to get field %q from %s", strings.Join(field, "."), tlog.KObj{Obj: in.dest})
		}
		preservedFields[strings.Join(field, ".")] = value
	}

	// Get spec from src.
	srcSpec, found, err := unstructured.NestedFieldNoCopy(in.src.Object, strings.Split(in.srcSpecPath, ".")...)
	if !found {
		return errors.Errorf("missing field %q in %s", in.srcSpecPath, tlog.KObj{Obj: in.src})
	} else if err != nil {
		return errors.Wrapf(err, "failed to get field %q from %s", in.srcSpecPath, tlog.KObj{Obj: in.src})
	}

	// Set spec in dest.
	if err := unstructured.SetNestedField(in.dest.Object, srcSpec, strings.Split(in.destSpecPath, ".")...); err != nil {
		return errors.Wrapf(err, "failed to set field %q on %s", in.destSpecPath, tlog.KObj{Obj: in.dest})
	}

	// Restore preserved fields.
	for path, value := range preservedFields {
		if err := unstructured.SetNestedField(in.dest.Object, value, strings.Split(path, ".")...); err != nil {
			return errors.Wrapf(err, "failed to set field %q on %s", path, tlog.KObj{Obj: in.dest})
		}
	}
	return nil
}
