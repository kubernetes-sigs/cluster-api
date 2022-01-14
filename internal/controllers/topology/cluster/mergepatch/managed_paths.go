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

package mergepatch

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/contract"
)

// DeepCopyWithManagedFieldAnnotation returns a copy of the object with an annotation
// Keeping track of the fields the object is setting.
func DeepCopyWithManagedFieldAnnotation(obj client.Object) (client.Object, error) {
	return deepCopyWithManagedFieldAnnotation(obj, nil)
}

func deepCopyWithManagedFieldAnnotation(obj client.Object, ignorePaths []contract.Path) (client.Object, error) {
	// Store the list of paths managed by the topology controller in the current patch operation;
	// this information will be used by the next patch operation.
	objWithManagedFieldAnnotation := obj.DeepCopyObject().(client.Object)
	if err := storeManagedPaths(objWithManagedFieldAnnotation, ignorePaths); err != nil {
		return nil, err
	}
	return objWithManagedFieldAnnotation, nil
}

// storeManagedPaths stores the list of paths managed by the topology controller into the managed field annotation.
// NOTE: The topology controller is only concerned about managed paths in the spec; given that
// we are dropping spec. from the result to reduce verbosity of the generated annotation.
// NOTE: Managed paths are relevant only for unstructured objects where it is not possible
// to easily discover which fields have been set by templates + patches/variables at a given reconcile;
// instead, it is not necessary to store managed paths for typed objets (e.g. Cluster, MachineDeployments)
// given that the topology controller explicitly sets a well-known, immutable list of fields at every reconcile.
func storeManagedPaths(obj client.Object, ignorePaths []contract.Path) error {
	// Return early if the object is not unstructured.
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil
	}

	// Gets the object spec.
	spec, _, err := unstructured.NestedMap(u.UnstructuredContent(), "spec")
	if err != nil {
		return errors.Wrap(err, "failed to get object spec")
	}

	// Gets a map with the key of the fields we are going to set into spec.
	managedFieldsMap := toManagedFieldsMap(spec, specIgnorePaths(ignorePaths))

	// Gets the annotation for the given map.
	managedFieldAnnotation, err := toManagedFieldAnnotation(managedFieldsMap)
	if err != nil {
		return err
	}

	// Store the managed paths in an annotation.
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string, 1)
	}
	annotations[clusterv1.ClusterTopologyManagedFieldsAnnotation] = managedFieldAnnotation
	obj.SetAnnotations(annotations)

	return nil
}

// specIgnorePaths returns ignore paths that apply to spec.
func specIgnorePaths(ignorePaths []contract.Path) []contract.Path {
	specPaths := make([]contract.Path, 0, len(ignorePaths))
	for _, i := range ignorePaths {
		if i[0] == "spec" && len(i) > 1 {
			specPaths = append(specPaths, i[1:])
		}
	}
	return specPaths
}

// toManagedFieldsMap returns a map with the key of the fields we are going to set into spec.
// Note: we are dropping ignorePaths.
func toManagedFieldsMap(m map[string]interface{}, ignorePaths []contract.Path) map[string]interface{} {
	r := make(map[string]interface{})
	for k, v := range m {
		// Drop the key if it matches ignore paths.
		ignore := false
		for _, i := range ignorePaths {
			if i[0] == k && len(i) == 1 {
				ignore = true
			}
		}
		if ignore {
			continue
		}

		// If the field has nested values (it is an object/map), process them.
		if nestedM, ok := v.(map[string]interface{}); ok {
			nestedIgnorePaths := make([]contract.Path, 0)
			for _, i := range ignorePaths {
				if i[0] == k && len(i) > 1 {
					nestedIgnorePaths = append(nestedIgnorePaths, i[1:])
				}
			}
			nestedV := toManagedFieldsMap(nestedM, nestedIgnorePaths)

			// Note: we are considering the object managed only if it is setting a value for one of the nested fields.
			// This prevents the topology controller to become authoritative on all the empty maps generated due to
			// how serialization works.
			if len(nestedV) > 0 {
				r[k] = nestedV
			}
			continue
		}

		// Otherwise, it is a "simple" field so mark it as managed
		r[k] = make(map[string]interface{})
	}
	return r
}

// managedFieldAnnotation returns a managed field annotation for a given managedFieldsMap.
func toManagedFieldAnnotation(managedFieldsMap map[string]interface{}) (string, error) {
	if len(managedFieldsMap) == 0 {
		return "", nil
	}

	// Converts to json.
	managedFieldsJSON, err := json.Marshal(managedFieldsMap)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal managed fields")
	}

	// gzip and base64 encode
	var managedFieldsJSONGZIP bytes.Buffer
	zw := gzip.NewWriter(&managedFieldsJSONGZIP)
	if _, err := zw.Write(managedFieldsJSON); err != nil {
		return "", errors.Wrap(err, "failed to write managed fields to gzip writer")
	}
	if err := zw.Close(); err != nil {
		return "", errors.Wrap(err, "failed to close gzip writer for managed fields")
	}
	managedFields := base64.StdEncoding.EncodeToString(managedFieldsJSONGZIP.Bytes())
	return managedFields, nil
}

// getManagedPaths infers the list of paths managed by the topology controller in the previous patch operation
// by parsing the value of the managed field annotation.
// NOTE: if for any reason the annotation is missing, the patch helper will fall back on standard
// two-way merge behavior.
func getManagedPaths(obj client.Object) ([]contract.Path, error) {
	// Gets the managed field annotation from the object.
	managedFieldAnnotation := obj.GetAnnotations()[clusterv1.ClusterTopologyManagedFieldsAnnotation]

	if managedFieldAnnotation == "" {
		return nil, nil
	}

	managedFieldsJSONGZIP, err := base64.StdEncoding.DecodeString(managedFieldAnnotation)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode managed fields")
	}

	var managedFieldsJSON bytes.Buffer
	zr, err := gzip.NewReader(bytes.NewReader(managedFieldsJSONGZIP))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create gzip reader for managed fields")
	}

	if _, err := io.Copy(&managedFieldsJSON, zr); err != nil { //nolint:gosec
		return nil, errors.Wrap(err, "failed to copy from gzip reader")
	}

	if err := zr.Close(); err != nil {
		return nil, errors.Wrap(err, "failed to close gzip reader for managed fields")
	}

	managedFieldsMap := make(map[string]interface{})
	if err := json.Unmarshal(managedFieldsJSON.Bytes(), &managedFieldsMap); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal managed fields")
	}

	paths := flattenManagePaths([]string{"spec"}, managedFieldsMap)

	return paths, nil
}

// flattenManagePaths builds a slice of paths from a managedFieldMap.
func flattenManagePaths(path contract.Path, unstructuredContent map[string]interface{}) []contract.Path {
	allPaths := []contract.Path{}
	for k, m := range unstructuredContent {
		nested, ok := m.(map[string]interface{})
		if ok && len(nested) == 0 {
			// We have to use a copy of path, because otherwise the slice we append to
			// allPaths would be overwritten in another iteration.
			tmp := make([]string, len(path))
			copy(tmp, path)
			allPaths = append(allPaths, append(tmp, k))
			continue
		}
		allPaths = append(allPaths, flattenManagePaths(append(path, k), nested)...)
	}
	return allPaths
}
