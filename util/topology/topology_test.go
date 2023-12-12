/*
Copyright 2022 The Kubernetes Authors.

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

// Package topology implements topology utility functions.
package topology

import (
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestShouldSkipImmutabilityChecks(t *testing.T) {
	tests := []struct {
		name string
		req  admission.Request
		obj  metav1.Object
		want bool
	}{
		{
			name: "false - dryRun pointer is nil",
			req:  admission.Request{},
			obj:  nil,
			want: false,
		},
		{
			name: "false - dryRun pointer is false",
			req:  admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{DryRun: ptr.To(false)}},
			obj:  nil,
			want: false,
		},
		{
			name: "false - nil obj",
			req:  admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{DryRun: ptr.To(true)}},
			obj:  nil,
			want: false,
		},
		{
			name: "false - no annotations",
			req:  admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{DryRun: ptr.To(true)}},
			obj:  &unstructured.Unstructured{},
			want: false,
		},
		{
			name: "false - annotation not set",
			req:  admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{DryRun: ptr.To(true)}},
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{},
				},
			}},
			want: false,
		},
		{
			name: "true",
			req:  admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{DryRun: ptr.To(true)}},
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						clusterv1.TopologyDryRunAnnotation: "",
					},
				},
			}},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldSkipImmutabilityChecks(tt.req, tt.obj); got != tt.want {
				t.Errorf("ShouldSkipImmutabilityChecks() = %v, want %v", got, tt.want)
			}
		})
	}
}
