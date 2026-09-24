/*
Copyright 2026 The Kubernetes Authors.

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

package clusterresourcesetbinding

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	addonsv1 "sigs.k8s.io/cluster-api/api/addons/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
)

// notFoundOnDelete returns an interceptor.Funcs that makes Delete calls for
// ClusterResourceSetBindings return a NotFound error, regardless of the fake
// client's actual state. This simulates the binding having already been
// deleted (e.g. by an earlier reconcile) while the controller's cache still
// has a stale copy.
func notFoundOnDelete() interceptor.Funcs {
	return interceptor.Funcs{
		Delete: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.DeleteOption) error {
			return apierrors.NewNotFound(addonsv1.GroupVersion.WithResource("clusterresourcesetbindings").GroupResource(), obj.GetName())
		},
	}
}

func TestReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	g := NewWithT(t)
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(addonsv1.AddToScheme(scheme)).To(Succeed())

	t.Run("returns success when owner Cluster no longer exists and the Binding was already deleted", func(t *testing.T) {
		g := NewWithT(t)

		binding := &addonsv1.ClusterResourceSetBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-binding",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: addonsv1.ClusterResourceSetBindingSpec{
				ClusterName: "missing-cluster",
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(binding).
			WithInterceptorFuncs(notFoundOnDelete()).
			Build()

		r := &Reconciler{Client: fakeClient}
		result, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: util.ObjectKey(binding)})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(Equal(reconcile.Result{}))
	})

	t.Run("returns success when owner Cluster is being deleted and the Binding was already deleted", func(t *testing.T) {
		g := NewWithT(t)

		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "test-cluster",
				Namespace:         metav1.NamespaceDefault,
				DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
				Finalizers:        []string{"test.cluster.x-k8s.io/block-deletion"},
			},
		}
		binding := &addonsv1.ClusterResourceSetBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-binding",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: addonsv1.ClusterResourceSetBindingSpec{
				ClusterName: cluster.Name,
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster, binding).
			WithInterceptorFuncs(notFoundOnDelete()).
			Build()

		r := &Reconciler{Client: fakeClient}
		result, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: util.ObjectKey(binding)})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(Equal(reconcile.Result{}))
	})
}
