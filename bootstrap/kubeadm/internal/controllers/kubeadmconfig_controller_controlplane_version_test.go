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

package controllers

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestKubeadmConfigReconciler_getControlPlaneVersionForJoin(t *testing.T) {
	ctx := context.Background()

	buildScope := func(cluster *clusterv1.Cluster) *Scope {
		return &Scope{
			Logger:  logr.Discard(),
			Cluster: cluster,
		}
	}

	t.Run("returns empty when ControlPlaneRef is not defined", func(t *testing.T) {
		g := NewWithT(t)
		cluster := builder.Cluster(metav1.NamespaceDefault, "c").Build()
		r := &KubeadmConfigReconciler{Client: fake.NewClientBuilder().Build()}
		v, err := r.getControlPlaneVersionForJoin(ctx, buildScope(cluster))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(v).To(BeEmpty())
	})

	t.Run("returns version when control plane exists", func(t *testing.T) {
		g := NewWithT(t)
		scheme := runtime.NewScheme()
		g.Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())
		g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

		cp := builder.TestControlPlane(metav1.NamespaceDefault, "cp").WithVersion("v1.30.0").Build()
		crd := builder.TestControlPlaneCRD.DeepCopy()
		cluster := builder.Cluster(metav1.NamespaceDefault, "c").Build()
		cluster.Spec.ControlPlaneRef = clusterv1.ContractVersionedObjectReference{
			APIGroup: builder.ControlPlaneGroupVersion.Group,
			Kind:     builder.TestControlPlaneKind,
			Name:     "cp",
		}

		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crd, cp, cluster).Build()
		r := &KubeadmConfigReconciler{Client: c}
		v, err := r.getControlPlaneVersionForJoin(ctx, buildScope(cluster))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(v).To(Equal("v1.30.0"))
	})

	t.Run("returns error when control plane object is missing", func(t *testing.T) {
		g := NewWithT(t)
		scheme := runtime.NewScheme()
		g.Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())
		g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

		crd := builder.TestControlPlaneCRD.DeepCopy()
		cluster := builder.Cluster(metav1.NamespaceDefault, "c").Build()
		cluster.Spec.ControlPlaneRef = clusterv1.ContractVersionedObjectReference{
			APIGroup: builder.ControlPlaneGroupVersion.Group,
			Kind:     builder.TestControlPlaneKind,
			Name:     "missing",
		}

		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crd, cluster).Build()
		r := &KubeadmConfigReconciler{Client: c}
		_, err := r.getControlPlaneVersionForJoin(ctx, buildScope(cluster))
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("returns empty when control plane has no spec.version", func(t *testing.T) {
		g := NewWithT(t)
		scheme := runtime.NewScheme()
		g.Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())
		g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

		cp := builder.TestControlPlane(metav1.NamespaceDefault, "cp").Build()
		crd := builder.TestControlPlaneCRD.DeepCopy()
		cluster := builder.Cluster(metav1.NamespaceDefault, "c").Build()
		cluster.Spec.ControlPlaneRef = clusterv1.ContractVersionedObjectReference{
			APIGroup: builder.ControlPlaneGroupVersion.Group,
			Kind:     builder.TestControlPlaneKind,
			Name:     "cp",
		}

		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crd, cp, cluster).Build()
		r := &KubeadmConfigReconciler{Client: c}
		v, err := r.getControlPlaneVersionForJoin(ctx, buildScope(cluster))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(v).To(BeEmpty())
	})
}
