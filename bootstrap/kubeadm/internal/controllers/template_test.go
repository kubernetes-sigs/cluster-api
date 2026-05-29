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

package controllers

import (
	"testing"

	. "github.com/onsi/gomega"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
)

func TestRenderTemplates(t *testing.T) {
	data := templateData("v1.29.0")

	t.Run("plain files unchanged", func(t *testing.T) {
		g := NewWithT(t)
		in := []bootstrapv1.File{
			{Path: "/a", Content: "hello {{ .controlPlane.version }}"},
		}
		out, err := renderTemplates(in, data)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(out[0].Content).To(Equal("hello {{ .controlPlane.version }}"))
		g.Expect(out[0].ContentFormat).To(BeEmpty())
	})

	t.Run("template renders and clears format", func(t *testing.T) {
		g := NewWithT(t)
		in := []bootstrapv1.File{
			{Path: "/b", ContentFormat: bootstrapv1.FileContentFormatTemplate, Content: "v={{ .controlPlane.version }}"},
		}
		out, err := renderTemplates(in, data)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(out[0].Content).To(Equal("v=v1.29.0"))
		g.Expect(out[0].ContentFormat).To(BeEmpty())
	})

	t.Run("bad template errors", func(t *testing.T) {
		g := NewWithT(t)
		in := []bootstrapv1.File{
			{Path: "/c", ContentFormat: bootstrapv1.FileContentFormatTemplate, Content: "{{ .controlPlane.version "},
		}
		_, err := renderTemplates(in, data)
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("template can guard on controlPlane absence when version is unavailable", func(t *testing.T) {
		// On the worker join path the control plane version may be unavailable
		// (no control plane ref or referenced object does not expose spec.version).
		// In that case the controlPlane key is omitted from the template data, and
		// template authors can detect its absence with {{ if .controlPlane }}.
		g := NewWithT(t)
		emptyData := templateData("")
		g.Expect(emptyData).ToNot(HaveKey("controlPlane"))
		in := []bootstrapv1.File{
			{Path: "/e", ContentFormat: bootstrapv1.FileContentFormatTemplate, Content: "{{ if .controlPlane }}v={{ .controlPlane.version }}{{ end }}done"},
		}
		out, err := renderTemplates(in, emptyData)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(out[0].Content).To(Equal("done"))
		g.Expect(out[0].ContentFormat).To(BeEmpty())
	})

	t.Run("template execution errors are surfaced", func(t *testing.T) {
		// A template that parses but fails at execution time (here, ranging over a string value) must
		// surface a render error rather than silently producing partial content.
		g := NewWithT(t)
		in := []bootstrapv1.File{
			{Path: "/d", ContentFormat: bootstrapv1.FileContentFormatTemplate, Content: "{{ range .controlPlane.version }}{{ end }}"},
		}
		_, err := renderTemplates(in, data)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring(`failed to execute template for file "/d"`))
	})
}
