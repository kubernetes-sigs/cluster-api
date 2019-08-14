/*
Copyright 2019 The Kubernetes Authors.

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

package external

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Get uses the client and reference to get an external, unstructured object.
func Get(c client.Client, ref *corev1.ObjectReference, namespace string) (*unstructured.Unstructured, error) {
	obj := new(unstructured.Unstructured)
	obj.SetAPIVersion(ref.APIVersion)
	obj.SetKind(ref.Kind)
	obj.SetName(ref.Name)
	key := client.ObjectKey{Name: obj.GetName(), Namespace: namespace}
	if err := c.Get(context.Background(), key, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// CloneTemplate uses the client and the reference to create a new object from the template.
func CloneTemplate(c client.Client, ref *corev1.ObjectReference, namespace string) (*unstructured.Unstructured, error) {
	from, err := Get(c, ref, namespace)
	if err != nil {
		return nil, err
	}
	template, found, err := unstructured.NestedMap(from.Object, "spec", "template")
	if !found {
		return nil, errors.Errorf("missing Spec.Template on %v %q", from.GroupVersionKind(), from.GetName())
	} else if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve Spec.Template map on %v %q", from.GroupVersionKind(), from.GetName())
	}
	to := &unstructured.Unstructured{Object: template}
	to.SetResourceVersion("")
	to.SetOwnerReferences(nil)
	to.SetFinalizers(nil)
	to.SetUID("")
	to.SetSelfLink("")
	to.SetName("")
	to.SetGenerateName(fmt.Sprintf("%s-", from.GetName()))
	to.SetNamespace(namespace)
	if err := c.Create(context.Background(), to); err != nil {
		return nil, err
	}
	return to, nil
}

// ErrorsFrom returns the ErrorReason and ErrorMessage fields from the external object status.
func ErrorsFrom(obj *unstructured.Unstructured) (string, string, error) {
	errorReason, _, err := unstructured.NestedString(obj.Object, "status", "errorReason")
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to determine errorReason on %v %q",
			obj.GroupVersionKind(), obj.GetName())
	}
	errorMessage, _, err := unstructured.NestedString(obj.Object, "status", "errorMessage")
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to determine errorMessage on %v %q",
			obj.GroupVersionKind(), obj.GetName())
	}
	return errorReason, errorMessage, nil
}

// IsReady returns true if the Status.Ready field on an external object is true.
func IsReady(obj *unstructured.Unstructured) (bool, error) {
	ready, found, err := unstructured.NestedBool(obj.Object, "status", "ready")
	if err != nil {
		return false, errors.Wrapf(err, "failed to determine %v %q readiness",
			obj.GroupVersionKind(), obj.GetName())
	}
	return ready && found, nil
}
