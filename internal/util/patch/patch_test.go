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

package patch

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"sigs.k8s.io/cluster-api/internal/contract"
)

func TestCopySpec(t *testing.T) {
	tests := []struct {
		name    string
		input   CopySpecInput
		want    *unstructured.Unstructured
		wantErr bool
	}{
		{
			name: "Field both in src and dest, no-op when equal",
			input: CopySpecInput{
				Src: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"A": "A",
						},
					},
				},
				Dest: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"A": "A",
						},
					},
				},
				SrcSpecPath:  "spec",
				DestSpecPath: "spec",
			},
			want: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"A": "A",
					},
				},
			},
		},
		{
			name: "Field both in src and dest, overwrite dest when different",
			input: CopySpecInput{
				Src: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"A": "A",
						},
					},
				},
				Dest: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"A": "A-different",
						},
					},
				},
				SrcSpecPath:  "spec",
				DestSpecPath: "spec",
			},
			want: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"A": "A",
					},
				},
			},
		},
		{
			name: "Nested field both in src and dest, no-op when equal",
			input: CopySpecInput{
				Src: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{
									"A": "A",
								},
							},
						},
					},
				},
				Dest: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{
									"A": "A",
								},
							},
						},
					},
				},
				SrcSpecPath:  "spec",
				DestSpecPath: "spec",
			},
			want: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"A": "A",
							},
						},
					},
				},
			},
		},
		{
			name: "Nested field both in src and dest, overwrite dest when different",
			input: CopySpecInput{
				Src: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{
									"A": "A",
								},
							},
						},
					},
				},
				Dest: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{
									"A": "A-different",
								},
							},
						},
					},
				},
				SrcSpecPath:  "spec",
				DestSpecPath: "spec",
			},
			want: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"A": "A",
							},
						},
					},
				},
			},
		},
		{
			name: "Field only in src, copy to dest",
			input: CopySpecInput{
				Src: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"foo": "bar",
						},
					},
				},
				Dest: &unstructured.Unstructured{
					Object: map[string]interface{}{},
				},
				SrcSpecPath:  "spec",
				DestSpecPath: "spec",
			},
			want: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
		},
		{
			name: "Nested field only in src, copy to dest",
			input: CopySpecInput{
				Src: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{
									"A": "A",
								},
							},
						},
					},
				},
				Dest: &unstructured.Unstructured{
					Object: map[string]interface{}{},
				},
				SrcSpecPath:  "spec",
				DestSpecPath: "spec",
			},
			want: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"A": "A",
							},
						},
					},
				},
			},
		},
		{
			name: "Copy field from spec.template.spec in src to spec in dest",
			input: CopySpecInput{
				Src: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{
									"A": "A",
								},
							},
						},
					},
				},
				Dest: &unstructured.Unstructured{
					Object: map[string]interface{}{},
				},
				SrcSpecPath:  "spec.template.spec",
				DestSpecPath: "spec",
			},
			want: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"A": "A",
					},
				},
			},
		},
		{
			name: "Copy field from spec.template.spec in src to spec in dest (overwrite when different)",
			input: CopySpecInput{
				Src: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{
									"A": "A",
								},
							},
						},
					},
				},
				Dest: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"A": "A-different",
						},
					},
				},
				SrcSpecPath:  "spec.template.spec",
				DestSpecPath: "spec",
			},
			want: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"A": "A",
					},
				},
			},
		},
		{
			name: "Field both in src and dest, overwrite when different and preserve fields",
			input: CopySpecInput{
				Src: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{
									"machineTemplate": map[string]interface{}{
										"infrastructureRef": map[string]interface{}{
											"apiVersion": "invalid",
											"kind":       "invalid",
											"namespace":  "invalid",
											"name":       "invalid",
										},
									},
									"replicas": float64(10),
									"version":  "v1.15.0",
									"A":        "A",
								},
							},
						},
					},
				},
				Dest: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"machineTemplate": map[string]interface{}{
								"infrastructureRef": map[string]interface{}{
									"apiVersion": "v1",
									"kind":       "kind",
									"namespace":  "namespace",
									"name":       "name",
								},
							},
							"replicas": float64(3),
							"version":  "v1.22.0",
							"A":        "A-different",
						},
					},
				},
				SrcSpecPath:  "spec.template.spec",
				DestSpecPath: "spec",
				FieldsToPreserve: []contract.Path{
					{"spec", "machineTemplate", "infrastructureRef"},
					{"spec", "replicas"},
					{"spec", "version"},
				},
			},
			want: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"machineTemplate": map[string]interface{}{
							"infrastructureRef": map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "kind",
								"namespace":  "namespace",
								"name":       "name",
							},
						},
						"replicas": float64(3),
						"version":  "v1.22.0",
						"A":        "A",
					},
				},
			},
		},
		{
			name: "Field not in src, no-op",
			input: CopySpecInput{
				Src: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"differentSpec": map[string]interface{}{
							"B": "B",
						},
					},
				},
				Dest: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"A": "A",
						},
					},
				},
				SrcSpecPath:  "spec",
				DestSpecPath: "spec",
			},
			want: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"A": "A",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := CopySpec(tt.input)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(tt.input.Dest).To(BeComparableTo(tt.want))
		})
	}
}
