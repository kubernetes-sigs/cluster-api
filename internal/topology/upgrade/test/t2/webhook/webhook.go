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

// Package webhook define webhooks for the v1beta2 API.
package webhook

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	testv1 "sigs.k8s.io/cluster-api/internal/topology/upgrade/test/t2/v1beta2"
)

// TestResourceTemplate defines CustomDefaulter and CustomValidator for a TestResourceTemplate.
type TestResourceTemplate struct{}

var _ admission.Defaulter[*testv1.TestResourceTemplate] = &TestResourceTemplate{}
var _ admission.Validator[*testv1.TestResourceTemplate] = &TestResourceTemplate{}

// Default a TestResourceTemplate.
func (webhook *TestResourceTemplate) Default(_ context.Context, r *testv1.TestResourceTemplate) error {
	if r.Annotations == nil {
		r.Annotations = map[string]string{}
	}
	r.Annotations["default-t2"] = ""
	return nil
}

// ValidateCreate a TestResourceTemplate.
func (webhook *TestResourceTemplate) ValidateCreate(_ context.Context, _ *testv1.TestResourceTemplate) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate a TestResourceTemplate.
func (webhook *TestResourceTemplate) ValidateUpdate(_ context.Context, _, _ *testv1.TestResourceTemplate) (admission.Warnings, error) {
	return nil, nil
}

// ValidateDelete a TestResourceTemplate.
func (webhook *TestResourceTemplate) ValidateDelete(_ context.Context, _ *testv1.TestResourceTemplate) (admission.Warnings, error) {
	return nil, nil
}

// TestResource defines CustomDefaulter and CustomValidator for a TestResource.
type TestResource struct{}

var _ admission.Defaulter[*testv1.TestResource] = &TestResource{}
var _ admission.Validator[*testv1.TestResource] = &TestResource{}

// Default a TestResource.
func (webhook *TestResource) Default(_ context.Context, r *testv1.TestResource) error {
	if r.Annotations == nil {
		r.Annotations = map[string]string{}
	}
	r.Annotations["default-t2"] = ""
	return nil
}

// ValidateCreate a TestResource.
func (webhook *TestResource) ValidateCreate(_ context.Context, _ *testv1.TestResource) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate a TestResource.
func (webhook *TestResource) ValidateUpdate(_ context.Context, _, _ *testv1.TestResource) (admission.Warnings, error) {
	return nil, nil
}

// ValidateDelete a TestResource.
func (webhook *TestResource) ValidateDelete(_ context.Context, _ *testv1.TestResource) (admission.Warnings, error) {
	return nil, nil
}
