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

package kubeadmconfig

import (
	"bytes"
	"io"
	"text/template"

	pkgerrors "github.com/pkg/errors"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
)

// maxRenderedTemplateBytes bounds the size of a single rendered spec.files entry.
const maxRenderedTemplateBytes = 1 << 20 // 1 MiB

type limitedWriter struct {
	w         io.Writer
	remaining int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if len(p) > l.remaining {
		return 0, pkgerrors.Errorf("rendered output exceeds the %d byte limit", maxRenderedTemplateBytes)
	}
	l.remaining -= len(p)
	return l.w.Write(p)
}

// templateData returns the data map passed to Go text/template when a KubeadmConfig spec.files entry uses
// contentFormat "Template". The map uses lowercase keys to match CAPI's builtin variable naming convention
// (e.g. {{ .controlPlane.version }}).
//
// The controlPlane key is only set when the control plane version is known. When it is empty (the cluster
// has no control plane ref or the referenced object does not expose spec.version) the key is omitted, so
// template authors can detect its absence with {{ if .controlPlane }}, consistent with how builtin variables
// behave.
func templateData(controlPlaneVersion string) map[string]interface{} {
	if controlPlaneVersion == "" {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"controlPlane": map[string]interface{}{
			"version": controlPlaneVersion,
		},
	}
}

// renderTemplates renders template file contents and clears contentFormat on those entries.
func renderTemplates(files []bootstrapv1.File, data map[string]interface{}) ([]bootstrapv1.File, error) {
	out := make([]bootstrapv1.File, len(files))
	copy(out, files)
	for i := range out {
		if out[i].ContentFormat != bootstrapv1.FileContentFormatTemplate {
			continue
		}
		tpl, err := template.New(out[i].Path).Parse(out[i].Content)
		if err != nil {
			return nil, pkgerrors.Wrapf(err, "failed to parse template for file %q", out[i].Path)
		}
		var buf bytes.Buffer
		if err := tpl.Execute(&limitedWriter{w: &buf, remaining: maxRenderedTemplateBytes}, data); err != nil {
			return nil, pkgerrors.Wrapf(err, "failed to execute template for file %q", out[i].Path)
		}
		out[i].Content = buf.String()
		out[i].ContentFormat = ""
	}
	return out, nil
}
