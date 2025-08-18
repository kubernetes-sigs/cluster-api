/*
Copyright 2025 The Kubernetes Authors.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestRequestItemBuilder(t *testing.T) {
	tests := []struct {
		name            string
		template        *unstructured.Unstructured
		holderObject    client.Object
		holderGVK       schema.GroupVersionKind
		holderFieldPath string
		want            runtimehooksv1.GeneratePatchesRequestItem
	}{
		{
			name: "cleanup Unstructured",
			template: func() *unstructured.Unstructured {
				u := builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "ict").Build()
				// Add managedFields and annotations that should be cleaned up.
				u.SetManagedFields([]metav1.ManagedFieldsEntry{
					{
						APIVersion: builder.InfrastructureGroupVersion.String(),
						Manager:    "manager",
						Operation:  "Apply",
						Time:       ptr.To(metav1.Now()),
						FieldsType: "FieldsV1",
					},
				})
				u.SetAnnotations(map[string]string{
					"fizz":                             "buzz",
					corev1.LastAppliedConfigAnnotation: "should be cleaned up",
					conversion.DataAnnotation:          "should be cleaned up",
				})
				return u
			}(),
			holderObject:    builder.Cluster(metav1.NamespaceDefault, "cluster").Build(),
			holderGVK:       clusterv1.GroupVersion.WithKind("Cluster"),
			holderFieldPath: "spec.infrastructureRef",
			want: runtimehooksv1.GeneratePatchesRequestItem{
				HolderReference: runtimehooksv1.HolderReference{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Cluster",
					Namespace:  "default",
					Name:       "cluster",
					FieldPath:  "spec.infrastructureRef",
				},
				Object: runtime.RawExtension{
					Raw: []uint8(`{"apiVersion":"infrastructure.cluster.x-k8s.io/v1beta2","kind":"GenericInfrastructureClusterTemplate","metadata":{"annotations":{"fizz":"buzz"},"name":"ict","namespace":"default"},"spec":{"template":{"spec":{}}}}`),
					Object: &unstructured.Unstructured{Object: map[string]any{
						"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta2",
						"kind":       "GenericInfrastructureClusterTemplate",
						"metadata": map[string]any{
							"namespace": "default",
							"name":      "ict",
							"annotations": map[string]any{
								"fizz": "buzz",
							},
						},
						"spec": map[string]any{
							"template": map[string]any{
								"spec": map[string]any{},
							},
						},
					}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := newRequestItemBuilder(tt.template).
				WithHolder(tt.holderObject, tt.holderGVK, tt.holderFieldPath).
				Build()
			g.Expect(err).ToNot(HaveOccurred())
			got.UID = "" // UID is random generated.
			g.Expect(*got).To(BeComparableTo(tt.want))
		})
	}
}
