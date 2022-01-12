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

package matchers

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestEqualObjectMatcher(t *testing.T) {
	tests := []struct {
		name     string
		original *unstructured.Unstructured
		modified *unstructured.Unstructured
		options  []MatchOption
		want     bool
	}{

		// Test when objects are equal.
		{
			name: "Equal field (spec) both in original and in modified",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
			want: true,
		},

		{
			name: "Equal nested field both in original and in modified",
			original: &unstructured.Unstructured{
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
			modified: &unstructured.Unstructured{
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
			want: true,
		},

		// Test when there is a difference between the objects.
		{
			name: "Unequal field both in original and in modified",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar-changed",
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
			want: false,
		},
		{
			name: "Unequal nested field both in original and modified",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"A": "A-Changed",
							},
						},
					},
				},
			},
			modified: &unstructured.Unstructured{
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
			want: false,
		},

		{
			name: "Value of type map with different values",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"map": map[string]string{
							"A": "A-changed",
							"B": "B",
							// C missing
						},
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"map": map[string]string{
							"A": "A",
							// B missing
							"C": "C",
						},
					},
				},
			},
			want: false,
		},

		{
			name: "Value of type Array or Slice with same length but different values",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"slice": []string{
							"D",
							"C",
							"B",
						},
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"slice": []string{
							"A",
							"B",
							"C",
						},
					},
				},
			},
			want: false,
		},

		// This tests specific behaviour in how Kubernetes marshals the zero value of metav1.Time{}.
		{
			name: "Creation timestamp set to empty value on both original and modified",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"A": "A",
					},
					"metadata": map[string]interface{}{
						"selfLink":          "foo",
						"creationTimestamp": metav1.Time{},
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"A": "A",
					},
					"metadata": map[string]interface{}{
						"selfLink":          "foo",
						"creationTimestamp": metav1.Time{},
					},
				},
			},
			want: true,
		},

		// Cases to test diff when fields exist only in modified object.
		{
			name: "Field only in modified",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
			want: false,
		},
		{
			name: "Nested field only in modified",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{
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
			want: false,
		},
		{
			name: "Creation timestamp exists on modified but not on original",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"A": "A",
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"A": "A",
					},
					"metadata": map[string]interface{}{
						"selfLink":          "foo",
						"creationTimestamp": "2021-11-03T11:05:17Z",
					},
				},
			},
			want: false,
		},

		// Test when fields are exists only in the original object.
		{
			name: "Field only in original",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{},
			},
			want: false,
		},
		{
			name: "Nested field only in original",
			original: &unstructured.Unstructured{
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
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{},
			},
			want: false,
		},
		{
			name: "Creation timestamp exists on original but not on modified",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"A": "A",
					},
					"metadata": map[string]interface{}{
						"selfLink":          "foo",
						"creationTimestamp": "2021-11-03T11:05:17Z",
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"A": "A",
					},
				},
			},

			want: false,
		},

		// Test metadata fields computed by the system or in status are compared.
		{
			name: "Unequal Metadata fields computed by the system or in status",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"selfLink":        "foo",
						"uid":             "foo",
						"resourceVersion": "foo",
						"generation":      "foo",
						"managedFields":   "foo",
					},
					"status": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
			want: false,
		},
		{
			name: "Unequal labels and annotations",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "bar",
						},
						"annotations": map[string]interface{}{
							"foo": "bar",
						},
					},
				},
			},
			want: false,
		},

		// Ignore fields MatchOption
		{
			name: "Unequal metadata fields ignored by IgnorePaths MatchOption",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"selfLink":        "foo",
						"uid":             "foo",
						"resourceVersion": "foo",
						"generation":      "foo",
						"managedFields":   "foo",
					},
				},
			},
			options: []MatchOption{IgnoreAutogeneratedMetadata},
			want:    true,
		},
		{
			name: "Unequal labels and annotations ignored by IgnorePaths MatchOption",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "bar",
						},
						"annotations": map[string]interface{}{
							"foo": "bar",
						},
					},
				},
			},
			options: []MatchOption{IgnorePaths{{"metadata", "labels"}, {"metadata", "annotations"}}},
			want:    true,
		},
		{
			name: "Ignore fields are not compared",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "",
							"port": 0,
						},
					},
				},
			},
			options: []MatchOption{IgnorePaths{{"spec", "controlPlaneEndpoint"}}},
			want:    true,
		},

		// AllowPaths MatchOption
		{
			name: "Unequal metadata fields not compared by setting AllowPaths MatchOption",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"A": "A",
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"A": "A",
					},
					"metadata": map[string]interface{}{
						"selfLink": "foo",
						"uid":      "foo",
					},
				},
			},
			options: []MatchOption{AllowPaths{{"spec"}}},
			want:    true,
		},

		// More tests
		{
			name: "No changes",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"A": "A",
						"B": "B",
						"C": "C", // C only in modified
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"A": "A",
						"B": "B",
					},
				},
			},
			want: false,
		},
		{
			name: "Many changes",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"A": "A",
						// B missing
						"C": "C", // C only in modified
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"A": "A",
						"B": "B",
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			matcher := EqualObject(tt.original, tt.options...)
			g.Expect(matcher.Match(tt.modified)).To(Equal(tt.want))
		})
	}
}
