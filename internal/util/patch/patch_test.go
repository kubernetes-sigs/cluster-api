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

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/contract"
)

func TestCopyFields(t *testing.T) {
	tests := []struct {
		name    string
		input   CopyFieldsInput
		want    *unstructured.Unstructured
		wantErr bool
	}{
		{
			name: "Field both in src and dest, no-op when equal",
			input: CopyFieldsInput{
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
				Fields: []CopyFieldsInputField{
					{
						Src:  []string{"spec"},
						Dest: []string{"spec"},
					},
				},
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
			input: CopyFieldsInput{
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
				Fields: []CopyFieldsInputField{
					{
						Src:  []string{"spec"},
						Dest: []string{"spec"},
					},
				},
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
			input: CopyFieldsInput{
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
				Fields: []CopyFieldsInputField{
					{
						Src:  []string{"spec"},
						Dest: []string{"spec"},
					},
				},
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
			input: CopyFieldsInput{
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
				Fields: []CopyFieldsInputField{
					{
						Src:  []string{"spec"},
						Dest: []string{"spec"},
					},
				},
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
			input: CopyFieldsInput{
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
				Fields: []CopyFieldsInputField{
					{
						Src:  []string{"spec"},
						Dest: []string{"spec"},
					},
				},
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
			input: CopyFieldsInput{
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
				Fields: []CopyFieldsInputField{
					{
						Src:  []string{"spec"},
						Dest: []string{"spec"},
					},
				},
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
			input: CopyFieldsInput{
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
				Fields: []CopyFieldsInputField{
					{
						Src:  []string{"spec", "template", "spec"},
						Dest: []string{"spec"},
					},
				},
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
			input: CopyFieldsInput{
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
				Fields: []CopyFieldsInputField{
					{
						Src:  []string{"spec", "template", "spec"},
						Dest: []string{"spec"},
					},
				},
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
			input: CopyFieldsInput{
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
				Fields: []CopyFieldsInputField{
					{
						Src:  []string{"spec", "template", "spec"},
						Dest: []string{"spec"},
					},
				},
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
			input: CopyFieldsInput{
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
				Fields: []CopyFieldsInputField{
					{
						Src:  []string{"spec"},
						Dest: []string{"spec"},
					},
				},
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
			name: "Copy metadata.labels & metadata.annotations",
			input: CopyFieldsInput{
				Src: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"label-keep":   "label-keep-value",
								"label-update": "label-update-new-value",
								"label-add":    "label-add-value",
							},
							"annotations": map[string]interface{}{
								"annotation-keep":   "annotation-keep-value",
								"annotation-update": "annotation-update-new-value",
								"annotation-add":    "annotation-add-value",
							},
						},
					},
				},
				Dest: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								clusterv1.ClusterNameLabel:                          "cluster-1",
								clusterv1.ClusterTopologyOwnedLabel:                 "",
								clusterv1.ClusterTopologyMachineDeploymentNameLabel: "md-topology-1",
								clusterv1.ClusterTopologyMachinePoolNameLabel:       "mp-topology-1",
								"label-keep":   "label-keep-value",
								"label-update": "label-update-value",
								"label-delete": "label-delete-value",
							},
							"annotations": map[string]interface{}{
								clusterv1.TemplateClonedFromNameAnnotation:      "cloned-from-name",
								clusterv1.TemplateClonedFromGroupKindAnnotation: "cloned-from-kind",
								"annotation-keep":   "annotation-keep-value",
								"annotation-update": "annotation-update-value",
								"annotation-delete": "annotation-delete-value",
							},
						},
					},
				},
				Fields: []CopyFieldsInputField{
					{Src: []string{"doesnotexist"}, Dest: []string{"doesnotexist"}}, // Should not influence metadata copying
					{Src: []string{"metadata", "labels"}, Dest: []string{"metadata", "labels"}},
					{Src: []string{"metadata", "annotations"}, Dest: []string{"metadata", "annotations"}},
				},
				FieldsToPreserve: []contract.Path{
					{"metadata", "labels", clusterv1.ClusterNameLabel},
					{"metadata", "labels", clusterv1.ClusterTopologyOwnedLabel},
					{"metadata", "labels", clusterv1.ClusterTopologyMachineDeploymentNameLabel},
					{"metadata", "labels", clusterv1.ClusterTopologyMachinePoolNameLabel},
					{"metadata", "annotations", clusterv1.TemplateClonedFromNameAnnotation},
					{"metadata", "annotations", clusterv1.TemplateClonedFromGroupKindAnnotation},
				},
			},
			want: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							clusterv1.ClusterNameLabel:                          "cluster-1",
							clusterv1.ClusterTopologyOwnedLabel:                 "",
							clusterv1.ClusterTopologyMachineDeploymentNameLabel: "md-topology-1",
							clusterv1.ClusterTopologyMachinePoolNameLabel:       "mp-topology-1",
							"label-keep":   "label-keep-value",
							"label-update": "label-update-new-value",
							"label-add":    "label-add-value",
						},
						"annotations": map[string]interface{}{
							clusterv1.TemplateClonedFromNameAnnotation:      "cloned-from-name",
							clusterv1.TemplateClonedFromGroupKindAnnotation: "cloned-from-kind",
							"annotation-keep":   "annotation-keep-value",
							"annotation-update": "annotation-update-new-value",
							"annotation-add":    "annotation-add-value",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := CopyFields(tt.input)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(tt.input.Dest).To(BeComparableTo(tt.want))
		})
	}
}
