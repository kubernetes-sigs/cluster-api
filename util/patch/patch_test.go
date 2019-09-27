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

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/controllers/external"
)

func TestHelperUnstructuredPatch(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.TODO()

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "BootstrapConfig",
			"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha2",
			"metadata": map[string]interface{}{
				"name":      "test-bootstrap",
				"namespace": "default",
			},
			"status": map[string]interface{}{
				"ready": true,
			},
		},
	}
	fakeClient := fake.NewFakeClient()
	fakeClient.Create(ctx, obj)

	h, err := NewHelper(obj, fakeClient)
	if err != nil {
		t.Fatalf("Expected no error initializing helper: %v", err)
	}

	refs := []metav1.OwnerReference{
		{
			APIVersion: "cluster.x-k8s.io/v1alpha2",
			Kind:       "Cluster",
			Name:       "test",
		},
	}
	obj.SetOwnerReferences(refs)

	err = h.Patch(ctx, obj)
	Expect(err).ToNot(HaveOccurred())

	// Make sure that the status has been preserved.
	ready, err := external.IsReady(obj)
	Expect(err).ToNot(HaveOccurred())
	Expect(ready).To(BeTrue())

	// Make sure that the object has been patched properly.
	afterObj := obj.DeepCopy()
	err = fakeClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "test-bootstrap"}, afterObj)
	Expect(err).ToNot(HaveOccurred())
	Expect(afterObj.GetOwnerReferences()).To(Equal(refs))
}

func TestHelperPatch(t *testing.T) {

	tests := []struct {
		name    string
		before  runtime.Object
		after   runtime.Object
		wantErr bool
	}{
		{
			name: "Only remove finalizer update",
			before: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
					Finalizers: []string{
						clusterv1.ClusterFinalizer,
					},
				},
			},
			after: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
				},
			},
		},
		{
			name: "Only status update",
			before: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
				},
			},
			after: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
				},
				Status: clusterv1.ClusterStatus{
					InfrastructureReady: true,
				},
			},
		},
		{
			name: "Only spec update",
			before: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
				},
			},
			after: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
				},
				Spec: clusterv1.ClusterSpec{
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
			before: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
				},
			},
			after: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
				},
				Spec: clusterv1.ClusterSpec{
					InfrastructureRef: &corev1.ObjectReference{
						Kind:      "test-kind",
						Name:      "test-ref",
						Namespace: "test-namespace",
					},
				},
				Status: clusterv1.ClusterStatus{
					InfrastructureReady: true,
				},
			},
		},
		{
			name: "Only add finalizer update",
			before: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
				},
			},
			after: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
					Finalizers: []string{
						clusterv1.ClusterFinalizer,
					},
				},
			},
		},
		{
			name: "Only add ownerref update to unstructured object",
			before: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "BootstrapConfig",
					"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha2",
					"metadata": map[string]interface{}{
						"name":      "test-bootstrap",
						"namespace": "default",
					},
				},
			},
			after: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "BootstrapConfig",
					"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha2",
					"metadata": map[string]interface{}{
						"name":      "test-bootstrap",
						"namespace": "default",
						"ownerReferences": []interface{}{
							map[string]interface{}{
								"kind":       "TestOwner",
								"apiVersion": "test.cluster.x-k8s.io/v1alpha2",
								"name":       "test",
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clusterv1.AddToScheme(scheme.Scheme)
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
