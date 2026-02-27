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

package templates

import (
	"bytes"
	"testing"
	"text/template"

	. "github.com/onsi/gomega"
)

func Test_TemplateFunctions(t *testing.T) {
	tests := []struct {
		name          string
		valueTemplate string
		data          map[string]interface{}
		want          string
	}{
		{
			"no template",
			"a b c",
			nil,
			"a b c",
		},
		{
			"simple template",
			"{{ tpl .variableABC . }}",
			map[string]interface{}{
				"variableA":   "a",
				"variableB":   "b",
				"variableC":   "c",
				"variableABC": "{{ .variableA }}-{{ .variableB }}-{{ .variableC }}",
			},
			"a-b-c",
		},
		{
			"nested template",
			"{{ tpl .variableABC . }}",
			map[string]interface{}{
				"variableA":   "{{ `A` }}",
				"variableB":   "b",
				"variableC":   "c",
				"variableABC": "{{ tpl .variableA . }}-{{ .variableB }}-{{ .variableC }}",
			},
			"A-b-c",
		},
		{
			"include",
			`{{- define "my-definition" }}{{ .variableZ }}{{- end }}{{ include "my-definition" .variableB }}`,
			map[string]interface{}{
				"variableA": "{{ `A` }}",
				"variableB": map[string]interface{}{
					"variableZ": "ZinB",
				},
			},
			"ZinB",
		},
		{
			"include and template",
			`{{- define "my-definition" }}{{ .variableZ }}{{- end }}{{ include (tpl .definitionName .) .variableB }}`,
			map[string]interface{}{
				"variableA": "{{ `A` }}",
				"variableB": map[string]interface{}{
					"variableZ": "ZinB",
				},
				"definitionName": "my-definition",
			},
			"ZinB",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			tpl := template.New("tpl")
			tpl.Funcs(TemplateFunctions(tpl))
			_, err := tpl.Parse(tt.valueTemplate)
			g.Expect(err).ToNot(HaveOccurred())
			var buf bytes.Buffer
			err = tpl.Execute(&buf, tt.data)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(buf.String()).To(Equal(tt.want))
		})
	}

}
