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

package mergepatch

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/contract"
)

func TestNewHelper(t *testing.T) {
	mustManagedFieldAnnotation := func(managedFieldsMap map[string]interface{}) string {
		annotation, err := toManagedFieldAnnotation(managedFieldsMap)
		if err != nil {
			panic(fmt.Sprintf("failed to generated managed field annotation: %v", err))
		}
		return annotation
	}

	tests := []struct {
		name                                string
		original                            *unstructured.Unstructured // current
		modified                            *unstructured.Unstructured // desired
		options                             []HelperOption
		ignoreManagedFieldAnnotationChanges bool // Prevent changes to managed field annotation to be generated, so the test can focus on how values are merged.
		wantHasChanges                      bool
		wantHasSpecChanges                  bool
		wantPatch                           []byte
	}{
		// Field both in original and in modified --> align to modified

		{
			name: "Field (spec) both in original and in modified, no-op when equal",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
			ignoreManagedFieldAnnotationChanges: true,
			wantHasChanges:                      false,
			wantHasSpecChanges:                  false,
			wantPatch:                           []byte("{}"),
		},
		{
			name: "Field both in original and in modified, align to modified when different",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar-changed",
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
			ignoreManagedFieldAnnotationChanges: true,
			wantHasChanges:                      true,
			wantHasSpecChanges:                  true,
			wantPatch:                           []byte("{\"spec\":{\"foo\":\"bar\"}}"),
		},
		{
			name: "Field (metadata.label) both in original and in modified, align to modified when different",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "bar-changed",
						},
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "bar",
						},
					},
				},
			},
			ignoreManagedFieldAnnotationChanges: true,
			wantHasChanges:                      true,
			wantHasSpecChanges:                  false,
			wantPatch:                           []byte("{\"metadata\":{\"labels\":{\"foo\":\"bar\"}}}"),
		},
		{
			name: "Field (metadata.label) preserve instance specific values when path is not authoritative",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"a": "a",
							"b": "b-changed",
						},
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							// a missing
							"b": "b",
							"c": "c",
						},
					},
				},
			},
			ignoreManagedFieldAnnotationChanges: true,
			wantHasChanges:                      true,
			wantHasSpecChanges:                  false,
			wantPatch:                           []byte("{\"metadata\":{\"labels\":{\"b\":\"b\",\"c\":\"c\"}}}"),
		},
		{
			name: "Field (metadata.label) align to modified when path is authoritative",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"a": "a",
							"b": "b-changed",
						},
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							// a missing
							"b": "b",
							"c": "c",
						},
					},
				},
			},
			options:                             []HelperOption{AuthoritativePaths{contract.Path{"metadata", "labels"}}},
			ignoreManagedFieldAnnotationChanges: true,
			wantHasChanges:                      true,
			wantHasSpecChanges:                  false,
			wantPatch:                           []byte("{\"metadata\":{\"labels\":{\"a\":null,\"b\":\"b\",\"c\":\"c\"}}}"),
		},
		{
			name: "IgnorePaths supersede AuthoritativePaths",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"a": "a",
							"b": "b-changed",
						},
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							// a missing
							"b": "b",
							"c": "c",
						},
					},
				},
			},
			options:                             []HelperOption{AuthoritativePaths{contract.Path{"metadata", "labels"}}, IgnorePaths{contract.Path{"metadata", "labels"}}},
			ignoreManagedFieldAnnotationChanges: true,
			wantHasChanges:                      false,
			wantHasSpecChanges:                  false,
			wantPatch:                           []byte("{}"),
		},
		{
			name: "Nested field both in original and in modified, no-op when equal",
			original: &unstructured.Unstructured{ // current
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
			modified: &unstructured.Unstructured{ // desired
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
			ignoreManagedFieldAnnotationChanges: true,
			wantHasChanges:                      false,
			wantHasSpecChanges:                  false,
			wantPatch:                           []byte("{}"),
		},
		{
			name: "Nested field both in original and in modified, align to modified when different",
			original: &unstructured.Unstructured{ // current
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
			modified: &unstructured.Unstructured{ // desired
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
			ignoreManagedFieldAnnotationChanges: true,
			wantHasChanges:                      true,
			wantHasSpecChanges:                  true,
			wantPatch:                           []byte("{\"spec\":{\"template\":{\"spec\":{\"A\":\"A\"}}}}"),
		},
		{
			name: "Value of type map, enforces entries from modified, preserve entries only in original",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"map": map[string]interface{}{
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
						"map": map[string]interface{}{
							"A": "A",
							// B missing
							"C": "C",
						},
					},
				},
			},
			ignoreManagedFieldAnnotationChanges: true,
			wantHasChanges:                      true,
			wantHasSpecChanges:                  true,
			wantPatch:                           []byte("{\"spec\":{\"map\":{\"A\":\"A\",\"C\":\"C\"}}}"),
		},
		{
			name: "Value of type map, enforces entries from modified if the path is authoritative",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"map": map[string]interface{}{
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
						"map": map[string]interface{}{
							"A": "A",
							// B missing
							"C": "C",
						},
					},
				},
			},
			options:                             []HelperOption{AuthoritativePaths{contract.Path{"spec", "map"}}},
			ignoreManagedFieldAnnotationChanges: true,
			wantHasChanges:                      true,
			wantHasSpecChanges:                  true,
			wantPatch:                           []byte("{\"spec\":{\"map\":{\"A\":\"A\",\"B\":null,\"C\":\"C\"}}}"),
		},
		{
			name: "Value of type Array or Slice, align to modified",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"slice": []interface{}{
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
						"slice": []interface{}{
							"A",
							"B",
							"C",
						},
					},
				},
			},
			ignoreManagedFieldAnnotationChanges: true,
			wantHasChanges:                      true,
			wantHasSpecChanges:                  true,
			wantPatch:                           []byte("{\"spec\":{\"slice\":[\"A\",\"B\",\"C\"]}}"),
		},

		// Field only in modified (not existing in original) --> align to modified

		{
			name: "Field only in modified, align to modified",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
			ignoreManagedFieldAnnotationChanges: true,
			wantHasChanges:                      true,
			wantHasSpecChanges:                  true,
			wantPatch:                           []byte("{\"spec\":{\"foo\":\"bar\"}}"),
		},
		{
			name: "Nested field only in modified, align to modified",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{ // desired
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
			ignoreManagedFieldAnnotationChanges: true,
			wantHasChanges:                      true,
			wantHasSpecChanges:                  true,
			wantPatch:                           []byte("{\"spec\":{\"template\":{\"spec\":{\"A\":\"A\"}}}}"),
		},

		// Field only in original (not existing in modified) --> preserve original

		{
			name: "Field only in original, align to modified",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{},
			},
			ignoreManagedFieldAnnotationChanges: true,
			wantHasChanges:                      false,
			wantHasSpecChanges:                  false,
			wantPatch:                           []byte("{}"),
		},
		{
			name: "Nested field only in original, align to modified",
			original: &unstructured.Unstructured{ // current
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
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{},
			},
			ignoreManagedFieldAnnotationChanges: true,
			wantHasChanges:                      false,
			wantHasSpecChanges:                  false,
			wantPatch:                           []byte("{}"),
		},

		// Diff for metadata fields computed by the system or in status are discarded

		{
			name: "Diff for metadata fields computed by the system or in status are discarded",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{ // desired
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
			ignoreManagedFieldAnnotationChanges: true,
			wantHasChanges:                      false,
			wantHasSpecChanges:                  false,
			wantPatch:                           []byte("{}"),
		},
		{
			name: "Relevant Diff for metadata (labels and annotations) are preserved",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{ // desired
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
			ignoreManagedFieldAnnotationChanges: true,
			wantHasChanges:                      true,
			wantHasSpecChanges:                  false,
			wantPatch:                           []byte("{\"metadata\":{\"annotations\":{\"foo\":\"bar\"},\"labels\":{\"foo\":\"bar\"}}}"),
		},

		// Ignore fields

		{
			name: "Ignore fields are removed from the diff",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "",
							"port": int64(0),
						},
					},
				},
			},
			options:                             []HelperOption{IgnorePaths{contract.Path{"spec", "controlPlaneEndpoint"}}},
			ignoreManagedFieldAnnotationChanges: true,
			wantHasChanges:                      false,
			wantHasSpecChanges:                  false,
			wantPatch:                           []byte("{}"),
		},

		// Managed fields

		{
			name: "Managed field annotation are generated when modified have spec values",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": map[string]interface{}{
							"bar": "",
							"baz": int64(0),
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
			wantPatch: []byte(fmt.Sprintf(
				"{\"metadata\":{\"annotations\":{%q:%q}},\"spec\":{\"foo\":{\"bar\":\"\",\"baz\":0}}}",
				clusterv1.ClusterTopologyManagedFieldsAnnotation,
				mustManagedFieldAnnotation(map[string]interface{}{
					"foo": map[string]interface{}{
						"bar": map[string]interface{}{},
						"baz": map[string]interface{}{},
					},
				}),
			)),
		},
		{
			name: "Managed field annotation is empty when modified have no spec values",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": map[string]interface{}{
							"bar": "",
							"baz": int64(0),
						},
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: false,
			wantPatch: []byte(fmt.Sprintf(
				"{\"metadata\":{\"annotations\":{%q:%q}}}",
				clusterv1.ClusterTopologyManagedFieldsAnnotation,
				"",
			)),
		},
		{
			name: "Managed field annotation from a previous reconcile are cleaned up when modified have no spec values",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							clusterv1.ClusterTopologyManagedFieldsAnnotation: mustManagedFieldAnnotation(map[string]interface{}{
								"something": map[string]interface{}{
									"from": map[string]interface{}{
										"a": map[string]interface{}{
											"previous": map[string]interface{}{
												"reconcile": map[string]interface{}{},
											},
										},
									},
								},
							}),
						},
					},
					"spec": map[string]interface{}{
						"foo": map[string]interface{}{
							"bar": "",
							"baz": int64(0),
						},
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
			wantPatch: []byte(fmt.Sprintf(
				"{\"metadata\":{\"annotations\":{%q:%q}},\"spec\":{\"something\":{\"from\":{\"a\":{\"previous\":{\"reconcile\":null}}}}}}",
				clusterv1.ClusterTopologyManagedFieldsAnnotation,
				"",
			)),
		},
		{
			name: "Managed field annotation does not include ignored paths - exact match",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": map[string]interface{}{
							"bar": "",
							"baz": int64(0),
						},
					},
				},
			},
			options:            []HelperOption{IgnorePaths{contract.Path{"spec", "foo", "bar"}}},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
			wantPatch: []byte(fmt.Sprintf(
				"{\"metadata\":{\"annotations\":{%q:%q}},\"spec\":{\"foo\":{\"baz\":0}}}",
				clusterv1.ClusterTopologyManagedFieldsAnnotation,
				mustManagedFieldAnnotation(map[string]interface{}{
					"foo": map[string]interface{}{
						"baz": map[string]interface{}{},
					},
				}),
			)),
		},
		{
			name: "Managed field annotation does not include ignored paths - path prefix",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": map[string]interface{}{
							"bar": "",
							"baz": int64(0),
						},
					},
				},
			},
			options:            []HelperOption{IgnorePaths{contract.Path{"spec", "foo"}}},
			wantHasChanges:     true,
			wantHasSpecChanges: false,
			wantPatch: []byte(fmt.Sprintf(
				"{\"metadata\":{\"annotations\":{%q:%q}}}",
				clusterv1.ClusterTopologyManagedFieldsAnnotation,
				"",
			)),
		},
		{
			name: "changes to managed field are applied",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							clusterv1.ClusterTopologyManagedFieldsAnnotation: mustManagedFieldAnnotation(map[string]interface{}{
								"kubeadmConfigSpec": map[string]interface{}{
									"clusterConfiguration": map[string]interface{}{
										"controllerManager": map[string]interface{}{
											"extraArgs": map[string]interface{}{
												"enable-hostpath-provisioner": map[string]interface{}{},
											},
										},
									},
								},
							}),
						},
					},
					"spec": map[string]interface{}{
						"kubeadmConfigSpec": map[string]interface{}{
							"clusterConfiguration": map[string]interface{}{
								"controllerManager": map[string]interface{}{
									"extraArgs": map[string]interface{}{
										"enable-hostpath-provisioner": "true",  // managed field previously set by a template
										"enable-garbage-collector":    "false", // user added field (should not be changed)
									},
								},
							},
						},
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"kubeadmConfigSpec": map[string]interface{}{
							"clusterConfiguration": map[string]interface{}{
								"controllerManager": map[string]interface{}{
									"extraArgs": map[string]interface{}{
										"enable-hostpath-provisioner": "false",
									},
								},
							},
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
			wantPatch:          []byte("{\"spec\":{\"kubeadmConfigSpec\":{\"clusterConfiguration\":{\"controllerManager\":{\"extraArgs\":{\"enable-hostpath-provisioner\":\"false\"}}}}}}"),
		},
		{
			name: "changes managed field to null is applied",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							clusterv1.ClusterTopologyManagedFieldsAnnotation: mustManagedFieldAnnotation(map[string]interface{}{
								"kubeadmConfigSpec": map[string]interface{}{
									"clusterConfiguration": map[string]interface{}{
										"controllerManager": map[string]interface{}{
											"extraArgs": map[string]interface{}{
												"enable-hostpath-provisioner": map[string]interface{}{},
											},
										},
									},
								},
							}),
						},
					},
					"spec": map[string]interface{}{
						"kubeadmConfigSpec": map[string]interface{}{
							"clusterConfiguration": map[string]interface{}{
								"controllerManager": map[string]interface{}{
									"extraArgs": map[string]interface{}{
										"enable-hostpath-provisioner": "true",  // managed field previously set by a template
										"enable-garbage-collector":    "false", // user added field (should not be changed)
									},
								},
							},
						},
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"kubeadmConfigSpec": map[string]interface{}{
							"clusterConfiguration": map[string]interface{}{
								"controllerManager": map[string]interface{}{
									"extraArgs": map[string]interface{}{
										"enable-hostpath-provisioner": nil,
									},
								},
							},
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
			wantPatch:          []byte("{\"spec\":{\"kubeadmConfigSpec\":{\"clusterConfiguration\":{\"controllerManager\":{\"extraArgs\":{\"enable-hostpath-provisioner\":null}}}}}}"),
		},
		{
			name: "dropping managed field trigger deletion; field should not be managed anymore",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							clusterv1.ClusterTopologyManagedFieldsAnnotation: mustManagedFieldAnnotation(map[string]interface{}{
								"kubeadmConfigSpec": map[string]interface{}{
									"clusterConfiguration": map[string]interface{}{
										"controllerManager": map[string]interface{}{
											"extraArgs": map[string]interface{}{
												"enable-hostpath-provisioner": map[string]interface{}{},
											},
										},
									},
								},
							}),
						},
					},
					"spec": map[string]interface{}{
						"kubeadmConfigSpec": map[string]interface{}{
							"clusterConfiguration": map[string]interface{}{
								"controllerManager": map[string]interface{}{
									"extraArgs": map[string]interface{}{
										"enable-hostpath-provisioner": "true",  // managed field previously set by a template (it is going to be dropped)
										"enable-garbage-collector":    "false", // user added field (should not be changed)
									},
								},
							},
						},
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"kubeadmConfigSpec": map[string]interface{}{
							"clusterConfiguration": map[string]interface{}{
								"controllerManager": map[string]interface{}{
									"extraArgs": map[string]interface{}{},
								},
							},
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
			wantPatch: []byte(fmt.Sprintf(
				"{\"metadata\":{\"annotations\":{%q:%q}},\"spec\":{\"kubeadmConfigSpec\":{\"clusterConfiguration\":{\"controllerManager\":{\"extraArgs\":{\"enable-hostpath-provisioner\":null}}}}}}",
				clusterv1.ClusterTopologyManagedFieldsAnnotation,
				mustManagedFieldAnnotation(map[string]interface{}{}),
			)),
		},
		{
			name: "changes managed object (a field with nested fields) to null is applied; managed field is updated accordingly",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							clusterv1.ClusterTopologyManagedFieldsAnnotation: mustManagedFieldAnnotation(map[string]interface{}{
								"kubeadmConfigSpec": map[string]interface{}{
									"clusterConfiguration": map[string]interface{}{
										"controllerManager": map[string]interface{}{
											"extraArgs": map[string]interface{}{
												"enable-hostpath-provisioner": map[string]interface{}{},
											},
										},
									},
								},
							}),
						},
					},
					"spec": map[string]interface{}{
						"kubeadmConfigSpec": map[string]interface{}{
							"clusterConfiguration": map[string]interface{}{
								"controllerManager": map[string]interface{}{
									"extraArgs": map[string]interface{}{
										"enable-hostpath-provisioner": "true",  // managed field previously set by a template (it is going to be dropped given that modified is providing an opinion on a parent object)
										"enable-garbage-collector":    "false", // user added field (it is going to be dropped given that modified is providing an opinion on a parent object)
									},
								},
							},
						},
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"kubeadmConfigSpec": map[string]interface{}{
							"clusterConfiguration": map[string]interface{}{
								"controllerManager": map[string]interface{}{
									"extraArgs": nil,
								},
							},
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
			wantPatch: []byte(fmt.Sprintf(
				"{\"metadata\":{\"annotations\":{%q:%q}},\"spec\":{\"kubeadmConfigSpec\":{\"clusterConfiguration\":{\"controllerManager\":{\"extraArgs\":null}}}}}",
				clusterv1.ClusterTopologyManagedFieldsAnnotation,
				mustManagedFieldAnnotation(map[string]interface{}{
					"kubeadmConfigSpec": map[string]interface{}{
						"clusterConfiguration": map[string]interface{}{
							"controllerManager": map[string]interface{}{
								"extraArgs": map[string]interface{}{},
							},
						},
					},
				}),
			)),
		},
		{
			name: "dropping managed object (a field with nested fields) to null is applied; managed field is updated accordingly",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							clusterv1.ClusterTopologyManagedFieldsAnnotation: mustManagedFieldAnnotation(map[string]interface{}{
								"kubeadmConfigSpec": map[string]interface{}{
									"clusterConfiguration": map[string]interface{}{
										"controllerManager": map[string]interface{}{
											"extraArgs": map[string]interface{}{
												"enable-hostpath-provisioner": map[string]interface{}{},
											},
										},
									},
								},
							}),
						},
					},
					"spec": map[string]interface{}{
						"kubeadmConfigSpec": map[string]interface{}{
							"clusterConfiguration": map[string]interface{}{
								"controllerManager": map[string]interface{}{
									"extraArgs": map[string]interface{}{
										"enable-hostpath-provisioner": "true",  // managed field previously set by a template (it is going to be dropped given that modified is dropping the parent object)
										"enable-garbage-collector":    "false", // user added field (should be preserved)
									},
								},
							},
						},
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"kubeadmConfigSpec": map[string]interface{}{
							"clusterConfiguration": map[string]interface{}{
								"controllerManager": map[string]interface{}{},
							},
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
			wantPatch: []byte(fmt.Sprintf(
				"{\"metadata\":{\"annotations\":{%q:%q}},\"spec\":{\"kubeadmConfigSpec\":{\"clusterConfiguration\":{\"controllerManager\":{\"extraArgs\":{\"enable-hostpath-provisioner\":null}}}}}}",
				clusterv1.ClusterTopologyManagedFieldsAnnotation,
				mustManagedFieldAnnotation(map[string]interface{}{}),
			)),
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
			ignoreManagedFieldAnnotationChanges: true,
			wantHasChanges:                      false,
			wantHasSpecChanges:                  false,
			wantPatch:                           []byte("{}"),
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
			ignoreManagedFieldAnnotationChanges: true,
			wantHasChanges:                      true,
			wantHasSpecChanges:                  true,
			wantPatch:                           []byte("{\"spec\":{\"B\":\"B\"}}"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Prevent changes to managed field annotation to be generated, so the test can focus on how values are merged.
			if tt.ignoreManagedFieldAnnotationChanges {
				modified := tt.modified.DeepCopy()

				var ignorePaths []contract.Path
				for _, o := range tt.options {
					if i, ok := o.(IgnorePaths); ok {
						ignorePaths = i
					}
				}

				// Compute the managed field annotation for modified.
				g.Expect(storeManagedPaths(modified, ignorePaths)).To(Succeed())

				// Clone the managed field annotation back to original.
				annotations := tt.original.GetAnnotations()
				if annotations == nil {
					annotations = make(map[string]string, 1)
				}
				annotations[clusterv1.ClusterTopologyManagedFieldsAnnotation] = modified.GetAnnotations()[clusterv1.ClusterTopologyManagedFieldsAnnotation]
				tt.original.SetAnnotations(annotations)
			}

			patch, err := NewHelper(tt.original, tt.modified, nil, tt.options...)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(patch.HasChanges()).To(Equal(tt.wantHasChanges))
			g.Expect(patch.HasSpecChanges()).To(Equal(tt.wantHasSpecChanges))
			g.Expect(patch.patch).To(Equal(tt.wantPatch))
		})
	}
}

func Test_filterPaths(t *testing.T) {
	tests := []struct {
		name     string
		patchMap map[string]interface{}
		paths    []contract.Path
		want     map[string]interface{}
	}{
		{
			name: "Allow values",
			patchMap: map[string]interface{}{
				"foo": "123",
				"bar": 123,
				"baz": map[string]interface{}{
					"foo": "123",
					"bar": 123,
				},
			},
			paths: []contract.Path{
				[]string{"foo"},
				[]string{"baz", "foo"},
			},
			want: map[string]interface{}{
				"foo": "123",
				"baz": map[string]interface{}{
					"foo": "123",
				},
			},
		},
		{
			name: "Allow maps",
			patchMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"foo": "123",
					"bar": 123,
				},
				"bar": map[string]interface{}{
					"foo": "123",
					"bar": 123,
				},
				"baz": map[string]interface{}{
					"foo": map[string]interface{}{
						"foo": "123",
						"bar": 123,
					},
					"bar": map[string]interface{}{
						"foo": "123",
						"bar": 123,
					},
				},
			},
			paths: []contract.Path{
				[]string{"foo"},
				[]string{"baz", "foo"},
			},
			want: map[string]interface{}{
				"foo": map[string]interface{}{
					"foo": "123",
					"bar": 123,
				},
				"baz": map[string]interface{}{
					"foo": map[string]interface{}{
						"foo": "123",
						"bar": 123,
					},
				},
			},
		},
		{
			name: "Cleanup empty maps",
			patchMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "123",
					"baz": map[string]interface{}{
						"bar": "123",
					},
				},
			},
			paths: []contract.Path{},
			want:  map[string]interface{}{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			filterPaths(tt.patchMap, tt.paths)

			g.Expect(tt.patchMap).To(Equal(tt.want))
		})
	}
}

func Test_removePath(t *testing.T) {
	tests := []struct {
		name     string
		patchMap map[string]interface{}
		path     contract.Path
		want     map[string]interface{}
	}{
		{
			name: "Remove value",
			patchMap: map[string]interface{}{
				"foo": "123",
			},
			path: contract.Path([]string{"foo"}),
			want: map[string]interface{}{},
		},
		{
			name: "Remove map",
			patchMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "123",
				},
			},
			path: contract.Path([]string{"foo"}),
			want: map[string]interface{}{},
		},
		{
			name: "Remove nested value",
			patchMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "123",
					"baz": "123",
				},
			},
			path: contract.Path([]string{"foo", "bar"}),
			want: map[string]interface{}{
				"foo": map[string]interface{}{
					"baz": "123",
				},
			},
		},
		{
			name: "Remove nested map",
			patchMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": map[string]interface{}{
						"baz": "123",
					},
					"baz": "123",
				},
			},
			path: contract.Path([]string{"foo", "bar"}),
			want: map[string]interface{}{
				"foo": map[string]interface{}{
					"baz": "123",
				},
			},
		},
		{
			name: "Ignore partial match",
			patchMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "123",
				},
			},
			path: contract.Path([]string{"foo", "bar", "baz"}),
			want: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "123",
				},
			},
		},
		{
			name: "Cleanup empty maps",
			patchMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"baz": map[string]interface{}{
						"bar": "123",
					},
				},
			},
			path: contract.Path([]string{"foo", "baz", "bar"}),
			want: map[string]interface{}{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			removePath(tt.patchMap, tt.path)

			g.Expect(tt.patchMap).To(Equal(tt.want))
		})
	}
}

func Test_enforcePath(t *testing.T) {
	tests := []struct {
		name             string
		authoritativeMap map[string]interface{}
		modified         map[string]interface{}
		twoWaysMap       map[string]interface{}
		path             contract.Path
		want             map[string]interface{}
	}{
		{
			name: "Keep value not enforced",
			authoritativeMap: map[string]interface{}{
				"foo": nil,
			},
			twoWaysMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "123",
				},
			},
			want: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "123",
				},
			},
			// no enforcing path
		},
		{
			name: "Enforce value",
			authoritativeMap: map[string]interface{}{
				"foo": nil,
			},
			twoWaysMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "123", // value enforced, it should be overridden.
				},
			},
			path: contract.Path([]string{"foo"}),
			want: map[string]interface{}{
				"foo": nil,
			},
		},
		{
			name: "Enforce nested value",
			authoritativeMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": nil,
				},
			},
			twoWaysMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "123", // value enforced, it should be overridden.
					"baz": "345", // value not enforced, it should be preserved.
				},
			},
			path: contract.Path([]string{"foo", "bar"}),
			want: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": nil,
					"baz": "345",
				},
			},
		},
		{
			name: "Enforce nested value",
			authoritativeMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": nil,
				},
			},
			twoWaysMap: map[string]interface{}{
				"foo": map[string]interface{}{ // value enforced, it should be overridden.
					"bar": "123",
					"baz": "345",
				},
			},
			path: contract.Path([]string{"foo"}),
			want: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": nil,
				},
			},
		},
		{
			name: "Enforce nested value rebuilding struct if missing",
			authoritativeMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": nil,
				},
			},
			twoWaysMap: map[string]interface{}{}, // foo enforced, it should be rebuilt/overridden.
			path:       contract.Path([]string{"foo"}),
			want: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": nil,
				},
			},
		},
		{
			name: "authoritative has no changes, twoWays has no changes, no changes",
			authoritativeMap: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": nil,
					},
				},
			},
			twoWaysMap: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": nil,
					},
				},
			},
			path: contract.Path([]string{"spec", "template", "metadata"}),
			want: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": nil,
					},
				},
			},
		},
		{
			name: "authoritative has no changes, twoWays has no changes, no changes",
			authoritativeMap: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{},
				},
			},
			twoWaysMap: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{},
				},
			},
			path: contract.Path([]string{"spec", "template", "metadata"}),
			want: map[string]interface{}{},
		},
		{
			name: "authoritative has changes, twoWays has no changes, authoritative apply",
			authoritativeMap: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"foo": "bar",
							},
						},
					},
				},
			},
			twoWaysMap: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{},
				},
			},
			path: contract.Path([]string{"spec", "template", "metadata"}),
			want: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"foo": "bar",
							},
						},
					},
				},
			},
		},
		{
			name: "authoritative has changes, twoWays has changes, authoritative apply",
			authoritativeMap: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"foo": "bar",
							},
						},
					},
				},
			},
			twoWaysMap: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"foo": "baz",
							},
						},
					},
				},
			},
			path: contract.Path([]string{"spec", "template", "metadata"}),
			want: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"foo": "bar",
							},
						},
					},
				},
			},
		},
		{
			name: "authoritative has no changes, twoWays has changes, twoWays changes blanked out",
			authoritativeMap: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{},
				},
			},
			twoWaysMap: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"foo": "baz",
							},
						},
					},
				},
			},
			path: contract.Path([]string{"spec", "template", "metadata"}),
			want: map[string]interface{}{},
		},
		{
			name: "authoritative sets to null a parent object and the change is intentional (parent null in modified).",
			authoritativeMap: map[string]interface{}{
				"kubeadmConfigSpec": map[string]interface{}{
					"clusterConfiguration": map[string]interface{}{
						"controllerManager": map[string]interface{}{
							"extraArgs": nil, // extra arg is a parent object in the authoritative path
						},
					},
				},
			},
			modified: map[string]interface{}{
				"kubeadmConfigSpec": map[string]interface{}{
					"clusterConfiguration": map[string]interface{}{
						"controllerManager": map[string]interface{}{
							"extraArgs": nil, // extra arg has been explicitly set to null
						},
					},
				},
			},
			twoWaysMap: map[string]interface{}{},
			path:       contract.Path([]string{"kubeadmConfigSpec", "clusterConfiguration", "controllerManager", "extraArgs", "enable-hostpath-provisioner"}),
			want: map[string]interface{}{
				"kubeadmConfigSpec": map[string]interface{}{
					"clusterConfiguration": map[string]interface{}{
						"controllerManager": map[string]interface{}{
							"extraArgs": nil,
						},
					},
				},
			},
		},
		{
			name: "authoritative sets to null a parent object and the change is a consequence of the object being dropped (parent does not exists in modified).",
			authoritativeMap: map[string]interface{}{
				"kubeadmConfigSpec": map[string]interface{}{
					"clusterConfiguration": map[string]interface{}{
						"controllerManager": map[string]interface{}{
							"extraArgs": nil, // extra arg is a parent object in the authoritative path
						},
					},
				},
			},
			modified: map[string]interface{}{
				"kubeadmConfigSpec": map[string]interface{}{
					"clusterConfiguration": map[string]interface{}{
						"controllerManager": map[string]interface{}{
							// extra arg has been dropped from modified
						},
					},
				},
			},
			twoWaysMap: map[string]interface{}{},
			path:       contract.Path([]string{"kubeadmConfigSpec", "clusterConfiguration", "controllerManager", "extraArgs", "enable-hostpath-provisioner"}),
			want: map[string]interface{}{
				"kubeadmConfigSpec": map[string]interface{}{
					"clusterConfiguration": map[string]interface{}{
						"controllerManager": map[string]interface{}{
							"extraArgs": map[string]interface{}{
								"enable-hostpath-provisioner": nil,
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			enforcePath(tt.authoritativeMap, tt.modified, tt.twoWaysMap, tt.path)

			g.Expect(tt.twoWaysMap).To(Equal(tt.want))
		})
	}
}
