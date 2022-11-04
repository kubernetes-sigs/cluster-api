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
	"testing"

	. "github.com/onsi/gomega"

	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
)

func TestReconcileStrategyScopeNeedsApply(t *testing.T) {
	tests := []struct {
		name  string
		scope *reconcileStrategyScope
		want  bool
	}{
		{
			name: "not binding",
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
			name: "not applied binding",
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
			name: "applied binding and different hash",
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
			name: "applied binding and same hash",
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
			name: "not applied binding",
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
			name: "applied binding",
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
