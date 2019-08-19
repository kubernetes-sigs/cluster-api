/*
Copyright 2017 The Kubernetes Authors.

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
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"sigs.k8s.io/cluster-api/api/v1alpha2"
)

func TestHelperPatch(t *testing.T) {
	tests := []struct {
		name    string
		before  runtime.Object
		after   runtime.Object
		wantErr bool
	}{
		{
			name: "Only remove finalizer update",
			before: &v1alpha2.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
					Finalizers: []string{
						v1alpha2.ClusterFinalizer,
					},
				},
			},
			after: &v1alpha2.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
				},
			},
		},
		{
			name: "Only status update",
			before: &v1alpha2.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
				},
			},
			after: &v1alpha2.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
				},
				Status: v1alpha2.ClusterStatus{
					InfrastructureReady: true,
				},
			},
		},
		{
			name: "Only spec update",
			before: &v1alpha2.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
				},
			},
			after: &v1alpha2.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
				},
				Spec: v1alpha2.ClusterSpec{
					InfrastructureRef: &corev1.ObjectReference{
						Kind:      "test-kind",
						Name:      "test-ref",
						Namespace: "test-namespace",
					},
				},
			},
		},
		{
			name: "Both spec and status update",
			before: &v1alpha2.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
				},
			},
			after: &v1alpha2.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
				},
				Spec: v1alpha2.ClusterSpec{
					InfrastructureRef: &corev1.ObjectReference{
						Kind:      "test-kind",
						Name:      "test-ref",
						Namespace: "test-namespace",
					},
				},
				Status: v1alpha2.ClusterStatus{
					InfrastructureReady: true,
				},
			},
		},
		{
			name: "Only add finalizer update",
			before: &v1alpha2.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
				},
			},
			after: &v1alpha2.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
					Finalizers: []string{
						v1alpha2.ClusterFinalizer,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v1alpha2.AddToScheme(scheme.Scheme)
			ctx := context.Background()
			fakeClient := fake.NewFakeClient()

			beforeCopy := tt.before.DeepCopyObject()
			fakeClient.Create(ctx, beforeCopy)

			h, err := NewHelper(beforeCopy, fakeClient)
			if err != nil {
				t.Fatalf("Expected no error initializing helper: %v", err)
			}

			afterCopy := tt.after.DeepCopyObject()
			if err := h.Patch(ctx, afterCopy); (err != nil) != tt.wantErr {
				t.Errorf("Helper.Patch() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !reflect.DeepEqual(tt.after, afterCopy) {
				t.Errorf("Expected after to be the same after patching\n tt.after: %v\n afterCopy: %v\n", tt.after, afterCopy)
			}
		})
	}
}
