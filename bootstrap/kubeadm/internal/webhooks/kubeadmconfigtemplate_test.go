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
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/webhooks/util"
)

func TestKubeadmConfigTemplateDefault(t *testing.T) {
	g := NewWithT(t)

	kubeadmConfigTemplate := &bootstrapv1.KubeadmConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo",
		},
	}
	updateDefaultingKubeadmConfigTemplate := kubeadmConfigTemplate.DeepCopy()
	updateDefaultingKubeadmConfigTemplate.Spec.Template.Spec.Verbosity = ptr.To[int32](4)
	webhook := &KubeadmConfigTemplate{}
	t.Run("for KubeadmConfigTemplate", util.CustomDefaultValidateTest(ctx, updateDefaultingKubeadmConfigTemplate, webhook))

	g.Expect(webhook.Default(ctx, kubeadmConfigTemplate)).To(Succeed())

	g.Expect(kubeadmConfigTemplate.Spec.Template.Spec.Format).To(Equal(bootstrapv1.CloudConfig))
}

func TestKubeadmConfigTemplateValidation(t *testing.T) {
	cases := map[string]struct {
		in        *bootstrapv1.KubeadmConfigTemplate
		expectErr bool
	}{
		"valid configuration": {
			in: &bootstrapv1.KubeadmConfigTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: bootstrapv1.KubeadmConfigTemplateSpec{
					Template: bootstrapv1.KubeadmConfigTemplateResource{
						Spec: bootstrapv1.KubeadmConfigSpec{},
					},
				},
			},
		},
		"should return error for invalid labels and annotations": {
			in: &bootstrapv1.KubeadmConfigTemplate{Spec: bootstrapv1.KubeadmConfigTemplateSpec{
				Template: bootstrapv1.KubeadmConfigTemplateResource{ObjectMeta: clusterv1.ObjectMeta{
					Labels: map[string]string{
						"foo":          "$invalid-key",
						"bar":          strings.Repeat("a", 64) + "too-long-value",
						"/invalid-key": "foo",
					},
					Annotations: map[string]string{
						"/invalid-key": "foo",
					},
				}},
			}},
			expectErr: true,
		},
	}

	for name, tt := range cases {
		webhook := &KubeadmConfigTemplate{}

		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			warnings, err := webhook.ValidateCreate(ctx, tt.in)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(warnings).To(BeEmpty())
			warnings, err = webhook.ValidateUpdate(ctx, nil, tt.in)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(warnings).To(BeEmpty())
		})
	}
}
