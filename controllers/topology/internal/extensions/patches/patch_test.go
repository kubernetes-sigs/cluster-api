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

package patches

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/contract"
)

func TestCopySpec(t *testing.T) {
	tests := []struct {
		name    string
		input   copySpecInput
		want    *unstructured.Unstructured
		wantErr bool
	}{
		{
			name: "Field both in src and dest, no-op when equal",
			input: copySpecInput{
				src: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"A": "A",
						},
					},
				},
				dest: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"A": "A",
						},
					},
				},
				srcSpecPath:  "spec",
				destSpecPath: "spec",
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
			input: copySpecInput{
				src: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"A": "A",
						},
					},
				},
				dest: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"A": "A-different",
						},
					},
				},
				srcSpecPath:  "spec",
				destSpecPath: "spec",
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
			input: copySpecInput{
				src: &unstructured.Unstructured{
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
				dest: &unstructured.Unstructured{
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
				srcSpecPath:  "spec",
				destSpecPath: "spec",
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
			input: copySpecInput{
				src: &unstructured.Unstructured{
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
				dest: &unstructured.Unstructured{
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
				srcSpecPath:  "spec",
				destSpecPath: "spec",
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
			input: copySpecInput{
				src: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"foo": "bar",
						},
					},
				},
				dest: &unstructured.Unstructured{
					Object: map[string]interface{}{},
				},
				srcSpecPath:  "spec",
				destSpecPath: "spec",
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
			input: copySpecInput{
				src: &unstructured.Unstructured{
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
				dest: &unstructured.Unstructured{
					Object: map[string]interface{}{},
				},
				srcSpecPath:  "spec",
				destSpecPath: "spec",
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
			input: copySpecInput{
				src: &unstructured.Unstructured{
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
				dest: &unstructured.Unstructured{
					Object: map[string]interface{}{},
				},
				srcSpecPath:  "spec.template.spec",
				destSpecPath: "spec",
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
			input: copySpecInput{
				src: &unstructured.Unstructured{
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
				dest: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": map[string]interface{}{
							"A": "A-different",
						},
					},
				},
				srcSpecPath:  "spec.template.spec",
				destSpecPath: "spec",
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
			input: copySpecInput{
				src: &unstructured.Unstructured{
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
				dest: &unstructured.Unstructured{
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
				srcSpecPath:  "spec.template.spec",
				destSpecPath: "spec",
				fieldsToPreserve: []contract.Path{
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := copySpec(tt.input)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(tt.input.dest).To(Equal(tt.want))
		})
	}
}
