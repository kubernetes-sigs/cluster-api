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

package cloudinit

const (
	filesTemplate = `{{ define "files" -}}
write_files:{{ range . }}
-   path: {{.Path}}
    encoding: "base64"
    owner: {{.Owner}}
    permissions: '{{.Permissions}}'
    content: |
{{.Content | Base64Encode | Indent 6}}
{{- end -}}
{{- end -}}
`
)

// Files defines the input for generating write_files in cloud-init.
type Files struct {
	// Path specifies the full path on disk where to store the file.
	Path string `json:"path"`

	// Owner specifies the ownership of the file, e.g. "root:root".
	Owner string `json:"owner"`

	// Permissions specifies the permissions to assign to the file, e.g. "0640".
	Permissions string `json:"permissions"`

	// Content is the actual content of the file.
	Content string `json:"content"`
}
