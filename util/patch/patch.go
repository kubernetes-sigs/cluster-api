/*
Copyright 2017 The Kubernetes Authors.

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

package patch

import (
	"context"
	"reflect"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Helper is a utility for ensuring the proper Patching of resources
// and their status
type Helper struct {
	client        client.Client
	before        map[string]interface{}
	hasStatus     bool
	beforeStatus  interface{}
	resourcePatch client.Patch
	statusPatch   client.Patch
}

// NewHelper returns an initialized Helper
func NewHelper(resource runtime.Object, crClient client.Client) (*Helper, error) {
	if resource == nil {
		return nil, errors.Errorf("expected non-nil resource")
	}

	// convert the resource to unstructured for easier comparison later
	before, err := runtime.DefaultUnstructuredConverter.ToUnstructured(resource)
	if err != nil {
		return nil, err
	}

	hasStatus := false
	// attempt to extract the status from the resource for easier comparison later
	beforeStatus, ok, err := unstructured.NestedFieldCopy(before, "status")
	if err != nil {
		return nil, err
	}
	if ok {
		hasStatus = true
		// if the resource contains a status remove it from our unstructured copy
		// to avoid unnecessary patching later
		unstructured.RemoveNestedField(before, "status")
	}

	return &Helper{
		client:        crClient,
		before:        before,
		beforeStatus:  beforeStatus,
		hasStatus:     hasStatus,
		resourcePatch: client.MergeFrom(resource.DeepCopyObject()),
		statusPatch:   client.MergeFrom(resource.DeepCopyObject()),
	}, nil
}

// Patch will attempt to patch the given resource and its status
func (h *Helper) Patch(ctx context.Context, resource runtime.Object) error {
	if resource == nil {
		return errors.Errorf("expected non-nil resource")
	}

	// convert the resource to unstructured to compare against our before copy
	after, err := runtime.DefaultUnstructuredConverter.ToUnstructured(resource)
	if err != nil {
		return err
	}

	hasStatus := false
	// attempt to extract the status from the resource to compare against our
	// beforeStatus copy
	afterStatus, ok, err := unstructured.NestedFieldCopy(after, "status")
	if err != nil {
		return err
	}
	if ok {
		hasStatus = true
		// if the resource contains a status remove it from our unstructured copy
		// to avoid uneccsary patching.
		unstructured.RemoveNestedField(after, "status")
	}

	var errs []error

	if !reflect.DeepEqual(h.before, after) {
		// only issue a Patch if the before and after resources (minus status) differ
		if err := h.client.Patch(ctx, resource.DeepCopyObject(), h.resourcePatch); err != nil {
			errs = append(errs, err)
		}
	}

	if (h.hasStatus || hasStatus) && !reflect.DeepEqual(h.beforeStatus, afterStatus) {
		// only issue a Status Patch if the resource has a status and the beforeStatus
		// and afterStatus copies differ
		if err := h.client.Status().Patch(ctx, resource.DeepCopyObject(), h.statusPatch); err != nil {
			errs = append(errs, err)
		}
	}

	return kerrors.NewAggregate(errs)
}
