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

package check

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type referencedObjectsCompatibilityTestCase struct {
	name    string
	current *unstructured.Unstructured
	desired *unstructured.Unstructured
	wantErr bool
}

var referencedObjectsCompatibilityTestCases = []referencedObjectsCompatibilityTestCase{
	{
		name: "Fails if group changes",
		current: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "foo/v1beta1",
			},
		},
		desired: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "bar/v1beta1",
			},
		},
		wantErr: true,
	},
	{
		name: "Fails if kind changes",
		current: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind": "foo",
			},
		},
		desired: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind": "bar",
			},
		},
		wantErr: true,
	},
	{
		name: "Pass if gvk remains the same",
		current: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "infrastructure.cluster.x-k8s.io/foo",
				"kind":       "foo",
			},
		},
		desired: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "infrastructure.cluster.x-k8s.io/foo",
				"kind":       "foo",
			},
		},
		wantErr: false,
	},
	{
		name: "Pass if version changes but group and kind remains the same",
		current: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "infrastructure.cluster.x-k8s.io/foo",
				"kind":       "foo",
			},
		},
		desired: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "infrastructure.cluster.x-k8s.io/bar",
				"kind":       "foo",
			},
		},
		wantErr: false,
	},
	{
		name: "Fails if namespace changes",
		current: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"namespace": "foo",
				},
			},
		},
		desired: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"namespace": "bar",
				},
			},
		},
		wantErr: true,
	},
}

func TestCheckReferencedObjectsAreCompatible(t *testing.T) {
	for _, tt := range referencedObjectsCompatibilityTestCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := ReferencedObjectsAreCompatible(tt.current, tt.desired)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func TestCheckReferencedObjectsAreStrictlyCompatible(t *testing.T) {
	referencedObjectsStrictCompatibilityTestCases := append(referencedObjectsCompatibilityTestCases, []referencedObjectsCompatibilityTestCase{
		{
			name: "Fails if name changes",
			current: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "foo",
					},
				},
			},
			desired: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "bar",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Pass if name remains the same",
			current: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "foo",
					},
				},
			},
			desired: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "foo",
					},
				},
			},
			wantErr: false,
		},
	}...)

	for _, tt := range referencedObjectsStrictCompatibilityTestCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := ReferencedObjectsAreStrictlyCompatible(tt.current, tt.desired)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}
