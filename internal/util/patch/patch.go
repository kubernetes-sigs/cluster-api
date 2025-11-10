/*
Copyright 2025 The Kubernetes Authors.

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

// Package patch contains patch utils.
package patch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"strings"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/internal/contract"
)

// ApplyPatchToTypedObject applies the patch to a typed obj.
func ApplyPatchToTypedObject[T any](ctx context.Context, currentMachine *T, machinePath runtimehooksv1.Patch, patchPath string) error {
	// Note: Machine needs special handling because it is not a runtime.RawExtension. Simply converting it here to
	//       a runtime.RawExtension so we can avoid making the code in applyPatchToObject more complex.
	currentMachineRaw, err := ConvertToRawExtension(currentMachine)
	if err != nil {
		return err
	}

	machineChanged, err := ApplyPatchToObject(ctx, &currentMachineRaw, machinePath, patchPath)
	if err != nil {
		return err
	}

	if !machineChanged {
		return nil
	}

	// Note: json.Unmarshal can't be used directly on *currentMachine as json.Unmarshal does not unset fields.
	patchedCurrentMachine := new(T)
	if err := json.Unmarshal(currentMachineRaw.Raw, patchedCurrentMachine); err != nil {
		return err
	}
	*currentMachine = *patchedCurrentMachine
	return nil
}

// ApplyPatchToObject applies the patch to the obj.
// Note: This is following the same general structure that is used in the applyPatchToRequest func in
// internal/controllers/topology/cluster/patches/engine.go.
func ApplyPatchToObject(ctx context.Context, obj *runtime.RawExtension, patch runtimehooksv1.Patch, patchPath string) (objChanged bool, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	if patch.PatchType == "" {
		return false, errors.Errorf("failed to apply patch: patchType is not set")
	}

	defer func() {
		if r := recover(); r != nil {
			log.Info(fmt.Sprintf("Observed a panic when applying patch: %v\n%s", r, string(debug.Stack())))
			reterr = kerrors.NewAggregate([]error{reterr, fmt.Errorf("observed a panic when applying patch: %v", r)})
		}
	}()

	// Create a copy of obj.Raw.
	// The patches will be applied to the copy and then only spec changes will be copied back to the obj.
	patchedObject := bytes.Clone(obj.Raw)
	var err error

	switch patch.PatchType {
	case runtimehooksv1.JSONPatchType:
		log.V(5).Info("Accumulating JSON patch", "patch", string(patch.Patch))
		jsonPatch, err := jsonpatch.DecodePatch(patch.Patch)
		if err != nil {
			log.Error(err, "Failed to apply patch: error decoding json patch (RFC6902)", "patch", string(patch.Patch))
			return false, errors.Wrap(err, "failed to apply patch: error decoding json patch (RFC6902)")
		}

		if len(jsonPatch) == 0 {
			// Return if there are no patches, nothing to do.
			return false, nil
		}

		patchedObject, err = jsonPatch.Apply(patchedObject)
		if err != nil {
			log.Error(err, "Failed to apply patch: error applying json patch (RFC6902)", "patch", string(patch.Patch))
			return false, errors.Wrap(err, "failed to apply patch: error applying json patch (RFC6902)")
		}
	case runtimehooksv1.JSONMergePatchType:
		if len(patch.Patch) == 0 || bytes.Equal(patch.Patch, []byte("{}")) {
			// Return if there are no patches, nothing to do.
			return false, nil
		}

		log.V(5).Info("Accumulating JSON merge patch", "patch", string(patch.Patch))
		patchedObject, err = jsonpatch.MergePatch(patchedObject, patch.Patch)
		if err != nil {
			log.Error(err, "Failed to apply patch: error applying json merge patch (RFC7386)", "patch", string(patch.Patch))
			return false, errors.Wrap(err, "failed to apply patch: error applying json merge patch (RFC7386)")
		}
	default:
		return false, errors.Errorf("failed to apply patch: unknown patchType %s", patch.PatchType)
	}

	// Overwrite the spec of obj with the spec of the patchedObject,
	// to ensure that we only pick up changes to the spec.
	if err := Patch(obj, patchedObject, patchPath); err != nil {
		return false, errors.Wrap(err, "failed to apply patch to object")
	}

	return true, nil
}

// ConvertToRawExtension converts any object to a runtime.RawExtension.
func ConvertToRawExtension(object any) (runtime.RawExtension, error) {
	objectBytes, err := json.Marshal(object)
	if err != nil {
		return runtime.RawExtension{}, errors.Wrap(err, "failed to marshal object to JSON")
	}

	objectUnstructured, ok := object.(*unstructured.Unstructured)
	if !ok {
		objectUnstructured = &unstructured.Unstructured{}
		// Note: This only succeeds if object has apiVersion & kind set (which is always the case).
		if err := json.Unmarshal(objectBytes, objectUnstructured); err != nil {
			return runtime.RawExtension{}, errors.Wrap(err, "failed to Unmarshal object into Unstructured")
		}
	}

	// Note: Raw and Object are always both set and Object is always set as an Unstructured
	//       to simplify subsequent code in matchesUnstructuredSpec.
	return runtime.RawExtension{
		Raw:    objectBytes,
		Object: objectUnstructured,
	}, nil
}

// Patch overwrites spec in object with spec of patchedObjectBytes.
func Patch(object *runtime.RawExtension, patchedObjectBytes []byte, patchPath string) error {
	objectUnstructured, err := bytesToUnstructured(object.Raw)
	if err != nil {
		return errors.Wrap(err, "failed to convert object to Unstructured")
	}
	patchedObjectUnstructured, err := bytesToUnstructured(patchedObjectBytes)
	if err != nil {
		return errors.Wrap(err, "failed to convert patched object to Unstructured")
	}

	// Copy spec from patchedObjectUnstructured to objectUnstructured.
	if err := CopySpec(CopySpecInput{
		Src:          patchedObjectUnstructured,
		Dest:         objectUnstructured,
		SrcSpecPath:  patchPath,
		DestSpecPath: patchPath,
	}); err != nil {
		return errors.Wrap(err, "failed to apply patch to object")
	}

	// Marshal objectUnstructured and store it in object.
	objectBytes, err := objectUnstructured.MarshalJSON()
	if err != nil {
		return errors.Wrapf(err, "failed to marshal patched object")
	}
	object.Object = objectUnstructured
	object.Raw = objectBytes
	return nil
}

// CopySpecInput is a struct containing the input parameters of CopySpec.
type CopySpecInput struct {
	Src              *unstructured.Unstructured
	Dest             *unstructured.Unstructured
	SrcSpecPath      string
	DestSpecPath     string
	FieldsToPreserve []contract.Path
}

// CopySpec copies a field from a srcSpecPath in src to a destSpecPath in dest,
// while preserving fieldsToPreserve.
func CopySpec(in CopySpecInput) error {
	// Backup fields that should be preserved from dest.
	preservedFields := map[string]interface{}{}
	for _, field := range in.FieldsToPreserve {
		value, found, err := unstructured.NestedFieldNoCopy(in.Dest.Object, field...)
		if !found {
			// Continue if the field does not exist in src. fieldsToPreserve don't have to exist.
			continue
		} else if err != nil {
			return errors.Wrapf(err, "failed to get field %q from %s %s", strings.Join(field, "."), in.Dest.GetKind(), klog.KObj(in.Dest))
		}
		preservedFields[strings.Join(field, ".")] = value
	}

	// Get spec from src.
	srcSpec, found, err := unstructured.NestedFieldNoCopy(in.Src.Object, strings.Split(in.SrcSpecPath, ".")...)
	if !found {
		// Return if srcSpecPath does not exist in src, nothing to do.
		return nil
	} else if err != nil {
		return errors.Wrapf(err, "failed to get field %q from %s %s", in.SrcSpecPath, in.Src.GetKind(), klog.KObj(in.Src))
	}

	// Set spec in dest.
	if err := unstructured.SetNestedField(in.Dest.Object, srcSpec, strings.Split(in.DestSpecPath, ".")...); err != nil {
		return errors.Wrapf(err, "failed to set field %q on %s %s", in.DestSpecPath, in.Dest.GetKind(), klog.KObj(in.Dest))
	}

	// Restore preserved fields.
	for path, value := range preservedFields {
		if err := unstructured.SetNestedField(in.Dest.Object, value, strings.Split(path, ".")...); err != nil {
			return errors.Wrapf(err, "failed to set field %q on %s %s", path, in.Dest.GetKind(), klog.KObj(in.Dest))
		}
	}
	return nil
}

// unstructuredDecoder is used to decode byte arrays into Unstructured objects.
var unstructuredDecoder = serializer.NewCodecFactory(nil).UniversalDeserializer()

// bytesToUnstructured provides a utility method that converts a (JSON) byte array into an Unstructured object.
func bytesToUnstructured(b []byte) (*unstructured.Unstructured, error) {
	// Unmarshal the JSON.
	u := &unstructured.Unstructured{}
	if _, _, err := unstructuredDecoder.Decode(b, nil, u); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal object from json")
	}

	return u, nil
}
