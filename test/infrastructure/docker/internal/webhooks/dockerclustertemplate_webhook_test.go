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
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/component-base/featuregate/testing"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
)

var ctx = ctrl.SetupSignalHandler()

func TestDockerClusterTemplateValidationFeatureGateEnabled(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)

	t.Run("create dockerclustertemplate should pass if gate enabled and valid dockerclustertemplate", func(t *testing.T) {
		g := NewWithT(t)
		dct := &infrav1.DockerClusterTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dockerclustertemplate-test",
				Namespace: "test-namespace",
			},
			Spec: infrav1.DockerClusterTemplateSpec{
				Template: infrav1.DockerClusterTemplateResource{
					Spec: infrav1.DockerClusterSpec{},
				},
			},
		}
		webhook := DockerClusterTemplate{}
		warnings, err := webhook.ValidateCreate(ctx, dct)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(warnings).To(BeEmpty())
	})
}

func TestDockerClusterTemplateValidationFeatureGateDisabled(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to create DockerClusterTemplate.
	t.Run("create dockerclustertemplate should not pass if gate disabled and valid dockerclustertemplate", func(t *testing.T) {
		g := NewWithT(t)
		dct := &infrav1.DockerClusterTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dockerclustertemplate-test",
				Namespace: "test-namespace",
			},
			Spec: infrav1.DockerClusterTemplateSpec{
				Template: infrav1.DockerClusterTemplateResource{
					Spec: infrav1.DockerClusterSpec{},
				},
			},
		}
		webhook := DockerClusterTemplate{}
		warnings, err := webhook.ValidateCreate(ctx, dct)
		g.Expect(err).To(HaveOccurred())
		g.Expect(warnings).To(BeEmpty())
	})
}

func TestDockerClusterTemplateValidationMetadata(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)

	tests := []struct {
		name        string
		labels      map[string]string
		annotations map[string]string
		expectErr   bool
	}{
		{
			name: "should return error for invalid labels and annotations",
			labels: map[string]string{
				"foo":          "$invalid-key",
				"bar":          strings.Repeat("a", 64) + "too-long-value",
				"/invalid-key": "foo",
			},
			annotations: map[string]string{
				"/invalid-key": "foo",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			dct := &infrav1.DockerClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dockerclustertemplate-test",
					Namespace: "test-namespace",
				},
				Spec: infrav1.DockerClusterTemplateSpec{
					Template: infrav1.DockerClusterTemplateResource{
						ObjectMeta: clusterv1.ObjectMeta{
							Labels: map[string]string{
								"foo":          "$invalid-key",
								"bar":          strings.Repeat("a", 64) + "too-long-value",
								"/invalid-key": "foo",
							},
							Annotations: map[string]string{
								"/invalid-key": "foo",
							},
						},
						Spec: infrav1.DockerClusterSpec{},
					},
				},
			}
			webhook := DockerClusterTemplate{}
			warnings, err := webhook.ValidateCreate(ctx, dct)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			}
		})
	}
}
