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

package cluster

import (
	"context"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"sigs.k8s.io/cluster-api/controllers/external"
)

// getReference gets the object referenced in ref.
func (r *Reconciler) getReference(ctx context.Context, ref *corev1.ObjectReference) (*unstructured.Unstructured, error) {
	if ref == nil {
		return nil, errors.New("reference is not set")
	}

	obj, err := external.Get(ctx, r.Client, ref, ref.Namespace)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve %s %q in namespace %q", ref.Kind, ref.Name, ref.Namespace)
	}
	return obj, nil
}

// refToUnstructured returns an unstructured object with details from an ObjectReference.
func refToUnstructured(ref *corev1.ObjectReference) *unstructured.Unstructured {
	uns := &unstructured.Unstructured{}
	uns.SetAPIVersion(ref.APIVersion)
	uns.SetKind(ref.Kind)
	uns.SetNamespace(ref.Namespace)
	uns.SetName(ref.Name)
	return uns
}
