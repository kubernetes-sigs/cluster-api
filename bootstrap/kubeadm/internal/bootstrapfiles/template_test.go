/*
Copyright 2026 The Kubernetes Authors.

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

package bootstrapfiles

import (
	"testing"

	"github.com/blang/semver/v4"
	. "github.com/onsi/gomega"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
)

func TestRenderTemplates(t *testing.T) {
	v := semver.MustParse("1.29.0")
	data := DataFromVersion(v)

	t.Run("plain files unchanged", func(t *testing.T) {
		g := NewWithT(t)
		in := []bootstrapv1.File{
			{Path: "/a", Content: "hello {{ .KubernetesVersion }}"},
		}
		out, err := RenderTemplates(in, data)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(out[0].Content).To(Equal("hello {{ .KubernetesVersion }}"))
		g.Expect(out[0].ContentFormat).To(BeEmpty())
	})

	t.Run("go-template renders and clears format", func(t *testing.T) {
		g := NewWithT(t)
		in := []bootstrapv1.File{
			{Path: "/b", ContentFormat: bootstrapv1.FileContentFormatGoTemplate, Content: "v={{ .KubernetesVersion }}"},
		}
		out, err := RenderTemplates(in, data)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(out[0].Content).To(Equal("v=1.29.0"))
		g.Expect(out[0].ContentFormat).To(BeEmpty())
	})

	t.Run("bad template errors", func(t *testing.T) {
		g := NewWithT(t)
		in := []bootstrapv1.File{
			{Path: "/c", ContentFormat: bootstrapv1.FileContentFormatGoTemplate, Content: "{{ .KubernetesVersion "},
		}
		_, err := RenderTemplates(in, data)
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("template execution errors on missing field", func(t *testing.T) {
		g := NewWithT(t)
		in := []bootstrapv1.File{
			{Path: "/d", ContentFormat: bootstrapv1.FileContentFormatGoTemplate, Content: "{{ .NonExistentField }}"},
		}
		_, err := RenderTemplates(in, data)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring(`failed to execute go-template for file "/d"`))
	})
}
