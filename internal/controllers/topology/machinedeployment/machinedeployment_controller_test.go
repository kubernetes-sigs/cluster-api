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

package machinedeployment

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestMachineDeploymentTopologyFinalizer(t *testing.T) {
	cluster := builder.Cluster(metav1.NamespaceDefault, "fake-cluster").Build()
	mdBT := builder.BootstrapTemplate(metav1.NamespaceDefault, "mdBT").Build()
	mdIMT := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "mdIMT").Build()
	mdBuilder := builder.MachineDeployment(metav1.NamespaceDefault, "md").
		WithClusterName("fake-cluster").
		WithBootstrapTemplate(mdBT).
		WithInfrastructureTemplate(mdIMT).
		WithLabels(map[string]string{
			clusterv1.ClusterTopologyOwnedLabel: "",
		})
	md := mdBuilder.Build()

	mdWithoutTopologyOwnedLabel := md.DeepCopy()
	delete(mdWithoutTopologyOwnedLabel.Labels, clusterv1.ClusterTopologyOwnedLabel)
	mdWithFinalizer := mdBuilder.Build()
	mdWithFinalizer.Finalizers = []string{clusterv1.MachineDeploymentTopologyFinalizer}

	testCases := []struct {
		name            string
		md              *clusterv1.MachineDeployment
		expectFinalizer bool
	}{
		// Note: We are not testing the case of a MD with deletionTimestamp and no finalizer.
		// This case is impossible to reproduce in fake client without deleting the object.
		{
			name:            "should add ClusterTopology finalizer to a MachineDeployment with no finalizer",
			md:              md,
			expectFinalizer: true,
		},
		{
			name:            "should retain ClusterTopology finalizer on MachineDeployment with finalizer",
			md:              mdWithFinalizer,
			expectFinalizer: true,
		},
		{
			name:            "should not add ClusterTopology finalizer on MachineDeployment without topology owned label",
			md:              mdWithoutTopologyOwnedLabel,
			expectFinalizer: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(tc.md, mdBT, mdIMT, cluster).
				Build()

			mdr := &Reconciler{
				Client:    fakeClient,
				APIReader: fakeClient,
			}

			_, err := mdr.Reconcile(ctx, reconcile.Request{
				NamespacedName: util.ObjectKey(tc.md),
			})
			g.Expect(err).ToNot(HaveOccurred())

			key := client.ObjectKey{Namespace: tc.md.Namespace, Name: tc.md.Name}
			var actual clusterv1.MachineDeployment
			g.Expect(mdr.Client.Get(ctx, key, &actual)).To(Succeed())
			if tc.expectFinalizer {
				g.Expect(actual.Finalizers).To(ConsistOf(clusterv1.MachineDeploymentTopologyFinalizer))
			} else {
				g.Expect(actual.Finalizers).To(BeEmpty())
			}
		})
	}
}

func TestMachineDeploymentReconciler_ReconcileDelete(t *testing.T) {
	deletionTimeStamp := metav1.Now()

	mdBT := builder.BootstrapTemplate(metav1.NamespaceDefault, "mdBT").Build()
	mdIMT := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "mdIMT").Build()
	md := builder.MachineDeployment(metav1.NamespaceDefault, "md").
		WithBootstrapTemplate(mdBT).
		WithInfrastructureTemplate(mdIMT).
		Build()
	mhc := builder.MachineHealthCheck(metav1.NamespaceDefault, "md").Build()
	md.SetDeletionTimestamp(&deletionTimeStamp)
	md.SetFinalizers([]string{clusterv1.MachineDeploymentTopologyFinalizer})

	t.Run("Should delete templates of a MachineDeployment", func(t *testing.T) {
		g := NewWithT(t)

		// Copying the MD so changes made by reconcileDelete do not affect other tests.
		md := md.DeepCopy()

		fakeClient := fake.NewClientBuilder().
			WithScheme(fakeScheme).
			WithObjects(md, mdBT, mdIMT).
			Build()

		r := &Reconciler{
			Client:    fakeClient,
			APIReader: fakeClient,
		}
		err := r.reconcileDelete(ctx, md)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controllerutil.ContainsFinalizer(md, clusterv1.MachineDeploymentTopologyFinalizer)).To(BeFalse())
		g.Expect(templateExists(fakeClient, mdBT)).To(BeFalse())
		g.Expect(templateExists(fakeClient, mdIMT)).To(BeFalse())
	})

	t.Run("Should delete infra template of a MachineDeployment without a bootstrap template", func(t *testing.T) {
		g := NewWithT(t)

		mdWithoutBootstrapTemplateIMT := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "mdWithoutBootstrapTemplateIMT").Build()
		mdWithoutBootstrapTemplate := builder.MachineDeployment(metav1.NamespaceDefault, "mdWithoutBootstrapTemplate").
			WithInfrastructureTemplate(mdWithoutBootstrapTemplateIMT).
			Build()
		mdWithoutBootstrapTemplate.SetDeletionTimestamp(&deletionTimeStamp)
		mdWithoutBootstrapTemplate.SetFinalizers([]string{clusterv1.MachineDeploymentTopologyFinalizer})

		fakeClient := fake.NewClientBuilder().
			WithScheme(fakeScheme).
			WithObjects(mdWithoutBootstrapTemplate, mdWithoutBootstrapTemplateIMT).
			Build()

		r := &Reconciler{
			Client:    fakeClient,
			APIReader: fakeClient,
		}
		err := r.reconcileDelete(ctx, mdWithoutBootstrapTemplate)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controllerutil.ContainsFinalizer(mdWithoutBootstrapTemplate, clusterv1.MachineDeploymentTopologyFinalizer)).To(BeFalse())
		g.Expect(templateExists(fakeClient, mdWithoutBootstrapTemplateIMT)).To(BeFalse())
	})

	t.Run("Should not delete templates of a MachineDeployment when they are still in use in a MachineSet", func(t *testing.T) {
		g := NewWithT(t)

		// Copying the MD so changes made by reconcileDelete do not affect other tests.
		md := md.DeepCopy()

		ms := builder.MachineSet(md.Namespace, "md").
			WithBootstrapTemplate(mdBT).
			WithInfrastructureTemplate(mdIMT).
			WithLabels(map[string]string{
				clusterv1.MachineDeploymentNameLabel: md.Name,
			}).
			Build()

		fakeClient := fake.NewClientBuilder().
			WithScheme(fakeScheme).
			WithObjects(md, ms, mdBT, mdIMT).
			Build()

		r := &Reconciler{
			Client:    fakeClient,
			APIReader: fakeClient,
		}
		err := r.reconcileDelete(ctx, md)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controllerutil.ContainsFinalizer(md, clusterv1.MachineDeploymentTopologyFinalizer)).To(BeFalse())
		g.Expect(templateExists(fakeClient, mdBT)).To(BeTrue())
		g.Expect(templateExists(fakeClient, mdIMT)).To(BeTrue())
	})
	t.Run("Should delete a MachineHealthCheck when its linked MachineDeployment is deleted", func(t *testing.T) {
		g := NewWithT(t)

		// Copying the MD so changes made by reconcileDelete do not affect other tests.
		md := md.DeepCopy()

		fakeClient := fake.NewClientBuilder().
			WithScheme(fakeScheme).
			WithObjects(md, mhc).
			Build()

		r := &Reconciler{
			Client:    fakeClient,
			APIReader: fakeClient,
		}
		err := r.reconcileDelete(ctx, md)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controllerutil.ContainsFinalizer(md, clusterv1.MachineDeploymentTopologyFinalizer)).To(BeFalse())

		gotMHC := clusterv1.MachineHealthCheck{}
		err = fakeClient.Get(ctx, client.ObjectKeyFromObject(mhc), &gotMHC)
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})
}

func templateExists(fakeClient client.Reader, template *unstructured.Unstructured) bool {
	obj := &unstructured.Unstructured{}
	obj.SetKind(template.GetKind())
	obj.SetAPIVersion(template.GetAPIVersion())

	err := fakeClient.Get(ctx, client.ObjectKeyFromObject(template), obj)
	if err != nil && !apierrors.IsNotFound(err) {
		panic(errors.Wrapf(err, "failed to get %s/%s", template.GetKind(), template.GetName()))
	}
	return err == nil
}
