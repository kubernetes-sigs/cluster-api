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

package contract

import (
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// V1Beta1Ref provide a helper struct for working with references in Unstructured objects.
//
// Deprecated: Will be removed when v1beta1 will be removed.
type V1Beta1Ref struct {
	path Path
}

// Path returns the path of the reference.
func (r *V1Beta1Ref) Path() Path {
	return r.path
}

// Get gets the reference value from the Unstructured object.
func (r *V1Beta1Ref) Get(obj *unstructured.Unstructured) (*corev1.ObjectReference, error) {
	return getNestedV1Beta1Ref(obj, r.path...)
}

// Set sets the reference value in the Unstructured object.
func (r *V1Beta1Ref) Set(obj *unstructured.Unstructured, ref *corev1.ObjectReference) error {
	return setNestedV1Beta1Ref(obj, ref, r.path...)
}

// getNestedV1Beta1Ref returns the ref value from a nested field in an Unstructured object.
func getNestedV1Beta1Ref(obj *unstructured.Unstructured, fields ...string) (*corev1.ObjectReference, error) {
	ref := &corev1.ObjectReference{}
	if v, ok, err := unstructured.NestedString(obj.UnstructuredContent(), append(fields, "apiVersion")...); ok && err == nil {
		ref.APIVersion = v
	} else {
		return nil, errors.Errorf("failed to get %s.apiVersion from %s", strings.Join(fields, "."), obj.GetKind())
	}
	if v, ok, err := unstructured.NestedString(obj.UnstructuredContent(), append(fields, "kind")...); ok && err == nil {
		ref.Kind = v
	} else {
		return nil, errors.Errorf("failed to get %s.kind from %s", strings.Join(fields, "."), obj.GetKind())
	}
	if v, ok, err := unstructured.NestedString(obj.UnstructuredContent(), append(fields, "name")...); ok && err == nil {
		ref.Name = v
	} else {
		return nil, errors.Errorf("failed to get %s.name from %s", strings.Join(fields, "."), obj.GetKind())
	}
	if v, ok, err := unstructured.NestedString(obj.UnstructuredContent(), append(fields, "namespace")...); ok && err == nil {
		ref.Namespace = v
	} else {
		return nil, errors.Errorf("failed to get %s.namespace from %s", strings.Join(fields, "."), obj.GetKind())
	}
	return ref, nil
}

// setNestedV1Beta1Ref sets the value of a nested field in an Unstructured to a reference to the refObj provided.
func setNestedV1Beta1Ref(obj *unstructured.Unstructured, ref *corev1.ObjectReference, fields ...string) error {
	r := map[string]interface{}{
		"kind":       ref.Kind,
		"namespace":  ref.Namespace,
		"name":       ref.Name,
		"apiVersion": ref.APIVersion,
	}
	if err := unstructured.SetNestedField(obj.UnstructuredContent(), r, fields...); err != nil {
		return errors.Wrapf(err, "failed to set object reference on object %v %s",
			obj.GroupVersionKind(), klog.KObj(obj))
	}
	return nil
}

// ObjToRef returns a reference to the given object.
// Note: This function only operates on Unstructured instead of client.Object
// because it is only safe to assume for Unstructured that the GVK is set.
func ObjToRef(obj *unstructured.Unstructured) *corev1.ObjectReference {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return &corev1.ObjectReference{
		Kind:       gvk.Kind,
		APIVersion: gvk.GroupVersion().String(),
		Namespace:  obj.GetNamespace(),
		Name:       obj.GetName(),
	}
}

// ObjToContractVersionedObjectReference returns a reference to the given object.
// Note: This function only operates on Unstructured instead of client.Object
// because it is only safe to assume for Unstructured that the GVK is set.
func ObjToContractVersionedObjectReference(obj *unstructured.Unstructured) clusterv1.ContractVersionedObjectReference {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return clusterv1.ContractVersionedObjectReference{
		APIGroup: gvk.Group,
		Kind:     gvk.Kind,
		Name:     obj.GetName(),
	}
}
