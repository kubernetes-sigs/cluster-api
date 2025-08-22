/*
Copyright 2024 The Kubernetes Authors.

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

// Package finalizers implements finalizer helper functions.
package finalizers

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"sigs.k8s.io/cluster-api/util/patch"
)

// EnsureFinalizer adds a finalizer if the object doesn't have a deletionTimestamp set
// and if the finalizer is not already set.
// This util is usually used in reconcilers directly after the reconciled object was retrieved
// and before pause is handled or "defer patch" with the patch helper.
func EnsureFinalizer(ctx context.Context, c client.Client, o client.Object, finalizer string) (finalizerAdded bool, err error) {
	// Finalizers can only be added when the deletionTimestamp is not set.
	if !o.GetDeletionTimestamp().IsZero() {
		return false, nil
	}

	if controllerutil.ContainsFinalizer(o, finalizer) {
		return false, nil
	}

	patchHelper, err := patch.NewHelper(o, c)
	if err != nil {
		return false, err
	}

	controllerutil.AddFinalizer(o, finalizer)

	if err := patchHelper.Patch(ctx, o); err != nil {
		return false, err
	}

	return true, nil
}
