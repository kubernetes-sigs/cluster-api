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
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/external"
)

func TestHelperUnstructuredPatch(t *testing.T) {
	g := NewWithT(t)
	ctx := context.TODO()

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "BootstrapConfig",
			"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
			"metadata": map[string]interface{}{
				"name":      "test-bootstrap",
				"namespace": "default",
			},
			"status": map[string]interface{}{
				"ready": true,
			},
		},
	}
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme)
	g.Expect(fakeClient.Create(ctx, obj)).To(Succeed())

	h, err := NewHelper(obj, fakeClient)
	g.Expect(err).NotTo(HaveOccurred())

	refs := []metav1.OwnerReference{
		{
			APIVersion: "cluster.x-k8s.io/v1alpha3",
			Kind:       "Cluster",
			Name:       "test",
		},
	}
	obj.SetOwnerReferences(refs)

	g.Expect(h.Patch(ctx, obj)).To(Succeed())

	// Make sure that the status has been preserved.
	ready, err := external.IsReady(obj)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ready).To(BeTrue())

	// Make sure that the object has been patched properly.
	afterObj := obj.DeepCopy()
	g.Expect(fakeClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "test-bootstrap"}, afterObj)).To(Succeed())
	g.Expect(afterObj.GetOwnerReferences()).To(Equal(refs))
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
					"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
					"metadata": map[string]interface{}{
						"name":      "test-bootstrap",
						"namespace": "default",
					},
				},
			},
			after: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "BootstrapConfig",
					"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha3",
					"metadata": map[string]interface{}{
						"name":      "test-bootstrap",
						"namespace": "default",
						"ownerReferences": []interface{}{
							map[string]interface{}{
								"kind":       "TestOwner",
								"apiVersion": "test.cluster.x-k8s.io/v1alpha3",
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
			g := NewWithT(t)

			g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())

			ctx := context.Background()
			fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme)

			beforeCopy := tt.before.DeepCopyObject()
			g.Expect(fakeClient.Create(ctx, beforeCopy)).To(Succeed())

			h, err := NewHelper(beforeCopy, fakeClient)
			g.Expect(err).NotTo(HaveOccurred())

			afterCopy := tt.after.DeepCopyObject()
			err = h.Patch(ctx, afterCopy)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}

			g.Expect(afterCopy).To(Equal(tt.after))
		})
	}
}
