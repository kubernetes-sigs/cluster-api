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

package webhooks

import (
	"context"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
)

func TestDockerMachineTemplateInvalid(t *testing.T) {
	oldTemplate := infrav1.DockerMachineTemplate{
		ObjectMeta: metav1.ObjectMeta{},
		Spec: infrav1.DockerMachineTemplateSpec{
			Template: infrav1.DockerMachineTemplateResource{},
		},
	}

	newTemplate := oldTemplate.DeepCopy()
	newTemplate.Spec.Template.Spec.ExtraMounts = append(newTemplate.Spec.Template.Spec.ExtraMounts, []infrav1.Mount{{ContainerPath: "/var/run/docker.sock", HostPath: "/var/run/docker.sock"}}...)
	newTemplateSkipImmutabilityAnnotationSet := newTemplate.DeepCopy()
	newTemplateSkipImmutabilityAnnotationSet.SetAnnotations(map[string]string{clusterv1.TopologyDryRunAnnotation: ""})

	newTemplateWithInvalidMetadata := newTemplate.DeepCopy()
	newTemplateWithInvalidMetadata.Spec.Template.ObjectMeta = clusterv1.ObjectMeta{
		Labels: map[string]string{
			"foo":          "$invalid-key",
			"bar":          strings.Repeat("a", 64) + "too-long-value",
			"/invalid-key": "foo",
		},
		Annotations: map[string]string{
			"/invalid-key": "foo",
		},
	}

	tests := []struct {
		name        string
		newTemplate *infrav1.DockerMachineTemplate
		oldTemplate *infrav1.DockerMachineTemplate
		req         *admission.Request
		wantError   bool
	}{
		{
			name:        "return no error if no modification",
			newTemplate: newTemplate,
			oldTemplate: newTemplate,
			req:         &admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{DryRun: ptr.To(false)}},
			wantError:   false,
		},
		{
			name:        "don't allow modification",
			newTemplate: newTemplate,
			oldTemplate: &oldTemplate,
			req:         &admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{DryRun: ptr.To(false)}},
			wantError:   true,
		},
		{
			name:        "don't allow modification, skip immutability annotation set",
			newTemplate: newTemplateSkipImmutabilityAnnotationSet,
			oldTemplate: &oldTemplate,
			req:         &admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{DryRun: ptr.To(false)}},
			wantError:   true,
		},
		{
			name:        "don't allow modification, dry run, no skip immutability annotation set",
			newTemplate: newTemplate,
			oldTemplate: &oldTemplate,
			req:         &admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{DryRun: ptr.To(true)}},
			wantError:   true,
		},
		{
			name:        "skip immutability check",
			newTemplate: newTemplateSkipImmutabilityAnnotationSet,
			oldTemplate: &oldTemplate,
			req:         &admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{DryRun: ptr.To(true)}},
			wantError:   false,
		},
		{
			name:        "don't allow invalid metadata",
			newTemplate: newTemplateWithInvalidMetadata,
			oldTemplate: newTemplate,
			req:         &admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{DryRun: ptr.To(true)}},
			wantError:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			wh := &DockerMachineTemplate{}
			ctx := context.Background()
			if tt.req != nil {
				ctx = admission.NewContextWithRequest(ctx, *tt.req)
			}
			warnings, err := wh.ValidateUpdate(ctx, tt.oldTemplate, tt.newTemplate)
			if tt.wantError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(warnings).To(BeEmpty())
		})
	}
}
