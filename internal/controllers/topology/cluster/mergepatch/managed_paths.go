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
	"sort"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/contract"
)

const (
	managedPathSeparator  = "."
	managedPathDotReplace = "%"
	managedFieldSeparator = ","
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

// getManagedPaths infers the list of paths managed by the topology controller in the previous patch operation
// by parsing the value of the managed field annotation.
// NOTE: if for any reason the annotation is missing, the patch helper will fall back on standard
// two-way merge behavior.
func getManagedPaths(obj client.Object) []contract.Path {
	// Gets the managed field annotation from the object.
	managedFieldAnnotation := obj.GetAnnotations()[clusterv1.ClusterTopologyManagedFieldsAnnotation]

	// Parses the managed field annotation value into a list of paths.
	// NOTE: we are prepending "spec" to the paths to restore the correct absolute path inside the object.
	managedFields := strings.Split(managedFieldAnnotation, managedFieldSeparator)
	paths := make([]contract.Path, 0, len(managedFields))
	for _, managedField := range managedFields {
		if strings.TrimSpace(managedField) == "" {
			continue
		}

		path := contract.Path{"spec"}
		for _, f := range strings.Split(managedField, managedPathSeparator) {
			path = append(path, strings.TrimSpace(strings.ReplaceAll(f, managedPathDotReplace, managedPathSeparator)))
		}
		paths = append(paths, path)
	}
	return paths
}

// Stores the list of paths managed by the topology controller into the managed field annotation.
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
	spec, ok, err := unstructured.NestedMap(u.UnstructuredContent(), "spec")
	if err != nil {
		return errors.Wrap(err, "failed to get object spec")
	}

	// Build a string representation of the paths under spec which are being managed by the object (the fields
	// a topology is expressing an opinion/value on, minus the ones we are explicitly ignoring).
	managedFields := ""
	if ok {
		s := []string{}
		paths := paths([]string{}, spec)
		for _, p := range paths {
			ignore := false
			pathString := strings.Join(p, managedPathSeparator)
			for _, i := range ignorePaths {
				if i[0] != "spec" {
					continue
				}
				ignorePathString := strings.Join(i[1:], managedPathSeparator)
				if pathString == ignorePathString || strings.HasPrefix(pathString, ignorePathString+managedPathSeparator) {
					ignore = true
					break
				}
			}
			if !ignore {
				s = append(s, strings.Join(p, managedPathSeparator))
			}
		}

		// Sort paths to get a predictable order (useful for readability and testing, not relevant at parse time).
		sort.Strings(s)

		// Concatenate all the paths in a single string.
		// NOTE: add an extra space between fields for better readability (not relevant at parse time).
		managedFields = strings.Join(s, managedFieldSeparator+" ")
	}

	// Stores the list of managed paths in an annotation for
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string, 1)
	}
	annotations[clusterv1.ClusterTopologyManagedFieldsAnnotation] = managedFields
	obj.SetAnnotations(annotations)

	return nil
}

// paths builds a slice of paths that exists in the unstructured content.
func paths(path contract.Path, unstructuredContent map[string]interface{}) []contract.Path {
	allPaths := []contract.Path{}
	for k, m := range unstructuredContent {
		key := strings.ReplaceAll(k, managedPathSeparator, managedPathDotReplace)
		nested, ok := m.(map[string]interface{})
		if !ok {
			// We have to use a copy of path, because otherwise the slice we append to
			// allPaths would be overwritten in another iteration.
			tmp := make([]string, len(path))
			copy(tmp, path)
			allPaths = append(allPaths, append(tmp, key))
			continue
		}
		allPaths = append(allPaths, paths(append(path, key), nested)...)
	}
	return allPaths
}
