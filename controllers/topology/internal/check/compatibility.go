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

// Package check implements checks for managed topology.
package check

import (
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReferencedObjectsAreStrictlyCompatible checks if two referenced objects are strictly compatible, meaning that
// they are compatible and the name of the objects do not change.
func ReferencedObjectsAreStrictlyCompatible(current, desired client.Object) error {
	if current.GetName() != desired.GetName() {
		return errors.Errorf("invalid operation: it is not possible to change the name of %s/%s from %s to %s",
			current.GetObjectKind().GroupVersionKind(), current.GetName(), current.GetName(), desired.GetName())
	}
	return ReferencedObjectsAreCompatible(current, desired)
}

// ReferencedObjectsAreCompatible checks if two referenced objects are compatible, meaning that
// they are of the same GroupKind and in the same namespace.
func ReferencedObjectsAreCompatible(current, desired client.Object) error {
	currentGK := current.GetObjectKind().GroupVersionKind().GroupKind()
	desiredGK := desired.GetObjectKind().GroupVersionKind().GroupKind()

	if currentGK.String() != desiredGK.String() {
		return errors.Errorf("invalid operation: it is not possible to change the GroupKind of %s/%s from %s to %s",
			current.GetObjectKind().GroupVersionKind(), current.GetName(), currentGK, desiredGK)
	}
	return ObjectsAreInTheSameNamespace(current, desired)
}

// ObjectsAreInTheSameNamespace checks if two referenced objects are in the same namespace.
func ObjectsAreInTheSameNamespace(current, desired client.Object) error {
	// NOTE: this should never happen (webhooks prevent it), but checking for extra safety.
	if current.GetNamespace() != desired.GetNamespace() {
		return errors.Errorf("invalid operation: it is not possible to change the namespace of %s/%s from %s to %s",
			current.GetObjectKind().GroupVersionKind(), current.GetName(), current.GetNamespace(), desired.GetNamespace())
	}
	return nil
}
