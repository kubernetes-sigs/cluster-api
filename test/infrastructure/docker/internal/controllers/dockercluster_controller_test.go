/*
Copyright 2019 The Kubernetes Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
)

func TestDockerClusterReconciler_ExternallyManaged(t *testing.T) {
	g := NewWithT(t)

	dockerCluster := &infrav1.DockerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-docker-cluster",
			Namespace: "default",
			Annotations: map[string]string{
				clusterv1.ManagedByAnnotation: "external-controller",
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(dockerCluster).
		Build()

	r := &DockerClusterReconciler{
		Client: c,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dockerCluster.Name,
			Namespace: dockerCluster.Namespace,
		},
	})

	// Should return early without error
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))

	// Verify finalizer was not added (early return before EnsureFinalizer)
	updatedCluster := &infrav1.DockerCluster{}
	err = c.Get(context.Background(), types.NamespacedName{
		Name:      dockerCluster.Name,
		Namespace: dockerCluster.Namespace,
	}, updatedCluster)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(updatedCluster.Finalizers).To(BeEmpty())
}
