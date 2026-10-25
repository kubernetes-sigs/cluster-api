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

package yaml

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestToUnstructured(t *testing.T) {
	type args struct {
		rawyaml []byte
	}
	tests := []struct {
		name          string
		args          args
		wantObjsCount int
		wantErr       bool
		err           string
	}{
		{
			name: "single object",
			args: args{
				rawyaml: []byte("apiVersion: v1\n" +
					"kind: ConfigMap\n"),
			},
			wantObjsCount: 1,
			wantErr:       false,
		},
		{
			name: "multiple objects are detected",
			args: args{
				rawyaml: []byte("apiVersion: v1\n" +
					"kind: ConfigMap\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"kind: Secret\n"),
			},
			wantObjsCount: 2,
			wantErr:       false,
		},
		{
			name: "empty object are dropped",
			args: args{
				rawyaml: []byte("---\n" + // empty objects before
					"---\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"kind: ConfigMap\n" +
					"---\n" + // empty objects in the middle
					"---\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"kind: Secret\n" +
					"---\n" + // empty objects after
					"---\n" +
					"---\n"),
			},
			wantObjsCount: 2,
			wantErr:       false,
		},
		{
			name: "--- in the middle of objects are ignored",
			args: args{
				[]byte("apiVersion: v1\n" +
					"kind: ConfigMap\n" +
					"data: \n" +
					" key: |\n" +
					"  ··Several lines of text,\n" +
					"  ··with some --- \n" +
					"  ---\n" +
					"  ··in the middle\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"kind: Secret\n"),
			},
			wantObjsCount: 2,
			wantErr:       false,
		},
		{
			name: "returns error for invalid yaml",
			args: args{
				rawyaml: []byte("apiVersion: v1\n" +
					"kind: ConfigMap\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"foobar\n" +
					"kind: Secret\n"),
			},
			wantErr: true,
			err:     "failed to unmarshal the 2nd yaml document",
		},
		{
			name: "returns error for invalid yaml",
			args: args{
				rawyaml: []byte("apiVersion: v1\n" +
					"kind: ConfigMap\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"kind: Pod\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"kind: Deployment\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"foobar\n" +
					"kind: ConfigMap\n"),
			},
			wantErr: true,
			err:     "failed to unmarshal the 4th yaml document",
		},
		{
			name: "returns error for invalid yaml",
			args: args{
				rawyaml: []byte("apiVersion: v1\n" +
					"foobar\n" +
					"kind: ConfigMap\n" +
					"---\n" +
					"apiVersion: v1\n" +
					"kind: Secret\n"),
			},
			wantErr: true,
			err:     "failed to unmarshal the 1st yaml document",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := ToUnstructured(tt.args.rawyaml)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				if tt.err != "" {
					g.Expect(err.Error()).To(ContainSubstring(tt.err))
				}
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(HaveLen(tt.wantObjsCount))
		})
	}
}

func TestFromUnstructured(t *testing.T) {
	rawyaml := []byte("apiVersion: v1\n" +
		"kind: ConfigMap")

	unstructuredObj := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
		},
	}

	convertedyaml, err := FromUnstructured([]unstructured.Unstructured{unstructuredObj})
	g := NewWithT(t)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(rawyaml)).To(Equal(string(convertedyaml)))
}

func TestRaw(t *testing.T) {
	g := NewWithT(t)

	input := `
		apiVersion:v1
		kind:newKind
		spec:
			param: abc
	`
	output := "apiVersion:v1\nkind:newKind\nspec:\n\tparam: abc\n"
	result := Raw(input)
	g.Expect(result).To(Equal(output))
}
