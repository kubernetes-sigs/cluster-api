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
	"bytes"
	"text/template"

	"github.com/pkg/errors"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
)

// templateData returns the data map passed to Go text/template when a KubeadmConfig spec.files entry uses
// contentFormat "template". The map uses lowercase keys to match CAPI's builtin variable naming convention
// (e.g. {{ .controlPlane.version }}).
func templateData(controlPlaneVersion string) map[string]interface{} {
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
		tpl, err := template.New(out[i].Path).Option("missingkey=error").Parse(out[i].Content)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse template for file %q", out[i].Path)
		}
		var buf bytes.Buffer
		if err := tpl.Execute(&buf, data); err != nil {
			return nil, errors.Wrapf(err, "failed to execute template for file %q", out[i].Path)
		}
		out[i].Content = buf.String()
		out[i].ContentFormat = ""
	}
	return out, nil
}
