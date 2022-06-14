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

package hooks

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/internal/runtime/catalog"
)

func TestIsMarked(t *testing.T) {
	tests := []struct {
		name string
		obj  client.Object
		hook runtimecatalog.Hook
		want bool
	}{
		{
			name: "should return true if the hook is marked",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
					Annotations: map[string]string{
						runtimehooksv1.PendingHooksAnnotation: "AfterClusterUpgrade",
					},
				},
			},
			hook: runtimehooksv1.AfterClusterUpgrade,
			want: true,
		},
		{
			name: "should return true if the hook is marked - other hooks are marked too",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
					Annotations: map[string]string{
						runtimehooksv1.PendingHooksAnnotation: "AfterClusterUpgrade,AfterControlPlaneUpgrade",
					},
				},
			},
			hook: runtimehooksv1.AfterClusterUpgrade,
			want: true,
		},
		{
			name: "should return false if the hook is not marked",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
				},
			},
			hook: runtimehooksv1.AfterClusterUpgrade,
			want: false,
		},
		{
			name: "should return false if the hook is not marked - other hooks are marked",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
					Annotations: map[string]string{
						runtimehooksv1.PendingHooksAnnotation: "AfterControlPlaneUpgrade",
					},
				},
			},
			hook: runtimehooksv1.AfterClusterUpgrade,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(IsPending(tt.hook, tt.obj)).To(Equal(tt.want))
		})
	}
}

func TestMark(t *testing.T) {
	tests := []struct {
		name string
		obj  client.Object
		hook runtimecatalog.Hook
	}{
		{
			name: "should add the marker if not already present",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
				},
			},
			hook: runtimehooksv1.AfterClusterUpgrade,
		},
		{
			name: "should add the marker if not already present - other hooks are present",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
					Annotations: map[string]string{
						runtimehooksv1.PendingHooksAnnotation: "AfterControlPlaneUpgrade",
					},
				},
			},
			hook: runtimehooksv1.AfterClusterUpgrade,
		},
		{
			name: "should pass if the marker is already present",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
					Annotations: map[string]string{
						runtimehooksv1.PendingHooksAnnotation: "AfterClusterUpgrade",
					},
				},
			},
			hook: runtimehooksv1.AfterClusterUpgrade,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewClientBuilder().WithObjects(tt.obj).Build()
			ctx := context.Background()
			g.Expect(MarkAsPending(ctx, fakeClient, tt.obj, tt.hook)).To(Succeed())
			annotations := tt.obj.GetAnnotations()
			g.Expect(annotations[runtimehooksv1.PendingHooksAnnotation]).To(ContainSubstring(runtimecatalog.HookName(tt.hook)))
		})
	}
}

func TestUnmark(t *testing.T) {
	tests := []struct {
		name string
		obj  client.Object
		hook runtimecatalog.Hook
	}{
		{
			name: "should pass if the marker is not already present",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
				},
			},
			hook: runtimehooksv1.AfterClusterUpgrade,
		},
		{
			name: "should remove if the marker is already present",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
					Annotations: map[string]string{
						runtimehooksv1.PendingHooksAnnotation: "AfterClusterUpgrade",
					},
				},
			},
			hook: runtimehooksv1.AfterClusterUpgrade,
		},
		{
			name: "should remove if the marker is already present among multiple hooks",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
					Annotations: map[string]string{
						runtimehooksv1.PendingHooksAnnotation: "AfterClusterUpgrade,AfterControlPlaneUpgrade",
					},
				},
			},
			hook: runtimehooksv1.AfterClusterUpgrade,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewClientBuilder().WithObjects(tt.obj).Build()
			ctx := context.Background()
			g.Expect(MarkAsDone(ctx, fakeClient, tt.obj, tt.hook)).To(Succeed())
			annotations := tt.obj.GetAnnotations()
			g.Expect(annotations[runtimehooksv1.PendingHooksAnnotation]).NotTo(ContainSubstring(runtimecatalog.HookName(tt.hook)))
		})
	}
}
