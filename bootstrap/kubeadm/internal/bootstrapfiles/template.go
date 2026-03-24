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

// Package bootstrapfiles contains helpers for KubeadmConfig spec.files processing.
package bootstrapfiles

import (
	"bytes"
	"text/template"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
)

// TemplateData is passed to Go text/template when a KubeadmConfig spec.files entry uses contentFormat "go-template".
type TemplateData struct {
	// KubernetesVersion is the effective Kubernetes version for bootstrap data (semver.String(), no "v" prefix).
	// For a worker Machine joining a cluster, this is the control plane Kubernetes version when the controller
	// can read it; otherwise the Machine's version.
	KubernetesVersion string
}

// DataFromVersion returns template data using semver.Version.String() (no "v" prefix).
func DataFromVersion(v semver.Version) TemplateData {
	return TemplateData{KubernetesVersion: v.String()}
}

// RenderTemplates renders go-template file contents and clears contentFormat on those entries.
func RenderTemplates(files []bootstrapv1.File, data TemplateData) ([]bootstrapv1.File, error) {
	out := make([]bootstrapv1.File, len(files))
	copy(out, files)
	for i := range out {
		if out[i].ContentFormat != bootstrapv1.FileContentFormatGoTemplate {
			continue
		}
		tpl, err := template.New(out[i].Path).Option("missingkey=error").Parse(out[i].Content)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse go-template for file %q", out[i].Path)
		}
		var buf bytes.Buffer
		if err := tpl.Execute(&buf, data); err != nil {
			return nil, errors.Wrapf(err, "failed to execute go-template for file %q", out[i].Path)
		}
		out[i].Content = buf.String()
		out[i].ContentFormat = ""
	}
	return out, nil
}
