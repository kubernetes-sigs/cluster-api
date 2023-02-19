/*
Copyright 2022 The Kubernetes Authors.

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

package controllers

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
)

func TestReconcileStrategyScopeNeedsApply(t *testing.T) {
	tests := []struct {
		name  string
		scope *reconcileStrategyScope
		want  bool
	}{
		{
			name: "no ResourceBinding",
			scope: &reconcileStrategyScope{
				baseResourceReconcileScope: baseResourceReconcileScope{
					resourceSetBinding: &addonsv1.ResourceSetBinding{},
					resourceRef: addonsv1.ResourceRef{
						Name: "cp",
						Kind: "ConfigMap",
					},
				},
			},
			want: true,
		},
		{
			name: "not applied ResourceBinding",
			scope: &reconcileStrategyScope{
				baseResourceReconcileScope: baseResourceReconcileScope{
					resourceSetBinding: &addonsv1.ResourceSetBinding{
						Resources: []addonsv1.ResourceBinding{
							{
								ResourceRef: addonsv1.ResourceRef{
									Name: "cp",
									Kind: "ConfigMap",
								},
								Applied: false,
							},
						},
					},
					resourceRef: addonsv1.ResourceRef{
						Name: "cp",
						Kind: "ConfigMap",
					},
				},
			},
			want: true,
		},
		{
			name: "applied ResourceBinding and different hash",
			scope: &reconcileStrategyScope{
				baseResourceReconcileScope: baseResourceReconcileScope{
					resourceSetBinding: &addonsv1.ResourceSetBinding{
						Resources: []addonsv1.ResourceBinding{
							{
								ResourceRef: addonsv1.ResourceRef{
									Name: "cp",
									Kind: "ConfigMap",
								},
								Applied: true,
								Hash:    "111",
							},
						},
					},
					resourceRef: addonsv1.ResourceRef{
						Name: "cp",
						Kind: "ConfigMap",
					},
					computedHash: "222",
				},
			},
			want: true,
		},
		{
			name: "applied ResourceBinding and same hash",
			scope: &reconcileStrategyScope{
				baseResourceReconcileScope: baseResourceReconcileScope{
					resourceSetBinding: &addonsv1.ResourceSetBinding{
						Resources: []addonsv1.ResourceBinding{
							{
								ResourceRef: addonsv1.ResourceRef{
									Name: "cp",
									Kind: "ConfigMap",
								},
								Applied: true,
								Hash:    "111",
							},
						},
					},
					resourceRef: addonsv1.ResourceRef{
						Name: "cp",
						Kind: "ConfigMap",
					},
					computedHash: "111",
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := NewWithT(t)
			gs.Expect(tt.scope.needsApply()).To(Equal(tt.want))
		})
	}
}

func TestReconcileApplyOnceScopeNeedsApply(t *testing.T) {
	tests := []struct {
		name  string
		scope *reconcileApplyOnceScope
		want  bool
	}{
		{
			name: "not applied ResourceBinding",
			scope: &reconcileApplyOnceScope{
				baseResourceReconcileScope: baseResourceReconcileScope{
					resourceSetBinding: &addonsv1.ResourceSetBinding{
						Resources: []addonsv1.ResourceBinding{
							{
								ResourceRef: addonsv1.ResourceRef{
									Name: "cp",
									Kind: "ConfigMap",
								},
								Applied: false,
							},
						},
					},
					resourceRef: addonsv1.ResourceRef{
						Name: "cp",
						Kind: "ConfigMap",
					},
				},
			},
			want: true,
		},
		{
			name: "applied ResourceBinding",
			scope: &reconcileApplyOnceScope{
				baseResourceReconcileScope: baseResourceReconcileScope{
					resourceSetBinding: &addonsv1.ResourceSetBinding{
						Resources: []addonsv1.ResourceBinding{
							{
								ResourceRef: addonsv1.ResourceRef{
									Name: "cp",
									Kind: "ConfigMap",
								},
								Applied: true,
							},
						},
					},
					resourceRef: addonsv1.ResourceRef{
						Name: "cp",
						Kind: "ConfigMap",
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := NewWithT(t)
			gs.Expect(tt.scope.needsApply()).To(Equal(tt.want))
		})
	}
}

func TestReconcileApplyOnceScopeApplyObj(t *testing.T) {
	tests := []struct {
		name         string
		existingObjs []client.Object
		obj          *unstructured.Unstructured
		wantErr      string
	}{
		{
			name: "object doesn't exist",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "my-cm",
						"namespace": "that-ns",
					},
				},
			},
		},
		{
			name: "object exists",
			existingObjs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-cm",
						Namespace: "that-ns",
					},
				},
			},
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "my-cm",
						"namespace": "that-ns",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := NewWithT(t)
			ctx := context.Background()
			client := fake.NewClientBuilder().WithObjects(tt.existingObjs...).Build()
			scope := &reconcileApplyOnceScope{}
			err := scope.applyObj(ctx, client, tt.obj)
			if tt.wantErr == "" {
				gs.Expect(err).NotTo(HaveOccurred())
			} else {
				gs.Expect(err).To(MatchError(ContainSubstring(tt.wantErr)))
			}
		})
	}
}
