/*
Copyright 2019 The Kubernetes Authors.

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

package v1alpha2

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
)

func TestConvertKubeadmConfig(t *testing.T) {
	t.Run("from hub", func(t *testing.T) {
		t.Run("preserves fields from hub version", func(t *testing.T) {
			g := NewWithT(t)

			src := &v1alpha3.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hub",
				},
				Spec: v1alpha3.KubeadmConfigSpec{
					Files: []v1alpha3.File{
						{
							Path:        "/etc/another/file",
							Owner:       "ubuntu:ubuntu",
							Encoding:    v1alpha3.GzipBase64,
							Permissions: "0600",
							ContentFrom: &v1alpha3.FileSource{
								Secret: v1alpha3.SecretFileSource{
									Name: "foo",
									Key:  "bar",
								},
							},
						},
						{
							Path:        "/etc/kubernetes/azure.json",
							Owner:       "root:root",
							Encoding:    v1alpha3.Base64,
							Permissions: "0644",
							Content:     "baz",
						},
					},
				},
				Status: v1alpha3.KubeadmConfigStatus{
					Ready:          true,
					DataSecretName: pointer.StringPtr("secret-data"),
				},
			}

			dst := &KubeadmConfig{}
			g.Expect(dst.ConvertFrom(src.DeepCopy())).To(Succeed())
			restored := &v1alpha3.KubeadmConfig{}
			g.Expect(dst.ConvertTo(restored)).To(Succeed())

			// Test field restored fields.
			g.Expect(restored.Name).To(Equal(src.Name))
			g.Expect(restored.Status.Ready).To(Equal(src.Status.Ready))
			g.Expect(restored.Status.DataSecretName).To(Equal(src.Status.DataSecretName))

			diff := cmp.Diff(src.Spec.Files, restored.Spec.Files, cmpopts.SortSlices(func(i, j v1alpha3.File) bool {
				return i.Path < j.Path
			}))
			if diff != "" {
				t.Fatalf(diff)
			}
		})
	})
}
