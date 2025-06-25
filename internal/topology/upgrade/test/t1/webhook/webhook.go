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

// Package webhook define webhooks for the v1beta1 API.
package webhook

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	testv1 "sigs.k8s.io/cluster-api/internal/topology/upgrade/test/t1/v1beta1"
)

// TestResourceTemplate defines CustomDefaulter and CustomValidator for a TestResourceTemplate.
type TestResourceTemplate struct{}

var _ webhook.CustomDefaulter = &TestResourceTemplate{}
var _ webhook.CustomValidator = &TestResourceTemplate{}

// Default a TestResourceTemplate.
func (webhook *TestResourceTemplate) Default(_ context.Context, obj runtime.Object) error {
	r := obj.(*testv1.TestResourceTemplate)
	if r.Annotations == nil {
		r.Annotations = map[string]string{}
	}
	r.Annotations["default-t1"] = ""
	return nil
}

// ValidateCreate a TestResourceTemplate.
func (webhook *TestResourceTemplate) ValidateCreate(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate a TestResourceTemplate.
func (webhook *TestResourceTemplate) ValidateUpdate(_ context.Context, _, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// ValidateDelete a TestResourceTemplate.
func (webhook *TestResourceTemplate) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// TestResource defines CustomDefaulter and CustomValidator for a TestResource.
type TestResource struct{}

var _ webhook.CustomDefaulter = &TestResource{}
var _ webhook.CustomValidator = &TestResource{}

// Default a TestResource.
func (webhook *TestResource) Default(_ context.Context, obj runtime.Object) error {
	r := obj.(*testv1.TestResource)
	if r.Annotations == nil {
		r.Annotations = map[string]string{}
	}
	r.Annotations["default-t1"] = ""
	return nil
}

// ValidateCreate a TestResource.
func (webhook *TestResource) ValidateCreate(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate a TestResource.
func (webhook *TestResource) ValidateUpdate(_ context.Context, _, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// ValidateDelete a TestResource.
func (webhook *TestResource) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
