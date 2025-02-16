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

package machineset

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

func TestMachineSetTopologyFinalizer(t *testing.T) {
	mdName := "md"

	msBT := builder.BootstrapTemplate(metav1.NamespaceDefault, "msBT").Build()
	msIMT := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "msIMT").Build()

	cluster := builder.Cluster(metav1.NamespaceDefault, "fake-cluster").Build()
	msBuilder := builder.MachineSet(metav1.NamespaceDefault, "ms").
		WithBootstrapTemplate(msBT).
		WithInfrastructureTemplate(msIMT).
		WithClusterName(cluster.Name).
		WithOwnerReferences([]metav1.OwnerReference{
			{
				Kind:       "MachineDeployment",
				APIVersion: clusterv1.GroupVersion.String(),
				Name:       "md",
			},
		}).
		WithLabels(map[string]string{
			clusterv1.MachineDeploymentNameLabel: mdName,
			clusterv1.ClusterTopologyOwnedLabel:  "",
		})

	ms := msBuilder.Build()
	msWithoutTopologyOwnedLabel := ms.DeepCopy()
	delete(msWithoutTopologyOwnedLabel.Labels, clusterv1.ClusterTopologyOwnedLabel)
	msWithFinalizer := msBuilder.Build()
	msWithFinalizer.Finalizers = []string{clusterv1.MachineSetTopologyFinalizer}

	testCases := []struct {
		name            string
		ms              *clusterv1.MachineSet
		expectFinalizer bool
	}{
		// Note: We are not testing the case of a MS with deletionTimestamp and no finalizer.
		// This case is impossible to reproduce in fake client without deleting the object.
		{
			name:            "should add ClusterTopology finalizer to a MachineSet with no finalizer",
			ms:              ms,
			expectFinalizer: true,
		},
		{
			name:            "should retain ClusterTopology finalizer on MachineSet with finalizer",
			ms:              msWithFinalizer,
			expectFinalizer: true,
		},
		{
			name:            "should not add ClusterTopology finalizer on MachineSet without topology owned label",
			ms:              msWithoutTopologyOwnedLabel,
			expectFinalizer: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(tc.ms, msBT, msIMT, cluster).
				Build()

			msr := &Reconciler{
				Client:    fakeClient,
				APIReader: fakeClient,
			}

			_, err := msr.Reconcile(ctx, reconcile.Request{
				NamespacedName: util.ObjectKey(tc.ms),
			})
			g.Expect(err).ToNot(HaveOccurred())

			key := client.ObjectKey{Namespace: tc.ms.Namespace, Name: tc.ms.Name}
			var actual clusterv1.MachineSet
			g.Expect(msr.Client.Get(ctx, key, &actual)).To(Succeed())
			if tc.expectFinalizer {
				g.Expect(actual.Finalizers).To(ConsistOf(clusterv1.MachineSetTopologyFinalizer))
			} else {
				g.Expect(actual.Finalizers).To(BeEmpty())
			}
		})
	}
}

func TestMachineSetReconciler_ReconcileDelete(t *testing.T) {
	deletionTimeStamp := metav1.Now()

	mdName := "md"

	msBT := builder.BootstrapTemplate(metav1.NamespaceDefault, "msBT").Build()
	msIMT := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "msIMT").Build()
	ms := builder.MachineSet(metav1.NamespaceDefault, "ms").
		WithBootstrapTemplate(msBT).
		WithInfrastructureTemplate(msIMT).
		WithLabels(map[string]string{
			clusterv1.MachineDeploymentNameLabel: mdName,
		}).
		Build()
	ms.SetDeletionTimestamp(&deletionTimeStamp)
	ms.SetFinalizers([]string{clusterv1.MachineSetTopologyFinalizer})
	ms.SetOwnerReferences([]metav1.OwnerReference{
		{
			Kind:       "MachineDeployment",
			APIVersion: clusterv1.GroupVersion.String(),
			Name:       "md",
		},
	})

	t.Run("Should delete templates of a MachineSet", func(t *testing.T) {
		g := NewWithT(t)

		// Copying the MS so changes made by reconcileDelete do not affect other tests.
		ms := ms.DeepCopy()

		fakeClient := fake.NewClientBuilder().
			WithScheme(fakeScheme).
			WithObjects(ms, msBT, msIMT).
			Build()

		r := &Reconciler{
			Client:    fakeClient,
			APIReader: fakeClient,
		}
		err := r.reconcileDelete(ctx, ms)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controllerutil.ContainsFinalizer(ms, clusterv1.MachineSetTopologyFinalizer)).To(BeFalse())
		g.Expect(templateExists(fakeClient, msBT)).To(BeFalse())
		g.Expect(templateExists(fakeClient, msIMT)).To(BeFalse())
	})

	t.Run("Should delete infra template of a MachineSet without a bootstrap template", func(t *testing.T) {
		g := NewWithT(t)

		msWithoutBootstrapTemplateIMT := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "msWithoutBootstrapTemplateIMT").Build()
		msWithoutBootstrapTemplate := builder.MachineSet(metav1.NamespaceDefault, "msWithoutBootstrapTemplate").
			WithInfrastructureTemplate(msWithoutBootstrapTemplateIMT).
			WithLabels(map[string]string{
				clusterv1.MachineDeploymentNameLabel: mdName,
			}).
			Build()
		msWithoutBootstrapTemplate.SetDeletionTimestamp(&deletionTimeStamp)
		msWithoutBootstrapTemplate.SetFinalizers([]string{clusterv1.MachineSetTopologyFinalizer})
		msWithoutBootstrapTemplate.SetOwnerReferences([]metav1.OwnerReference{
			{
				Kind:       "MachineDeployment",
				APIVersion: clusterv1.GroupVersion.String(),
				Name:       "md",
			},
		})

		fakeClient := fake.NewClientBuilder().
			WithScheme(fakeScheme).
			WithObjects(msWithoutBootstrapTemplate, msWithoutBootstrapTemplateIMT).
			Build()

		r := &Reconciler{
			Client:    fakeClient,
			APIReader: fakeClient,
		}
		err := r.reconcileDelete(ctx, msWithoutBootstrapTemplate)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controllerutil.ContainsFinalizer(msWithoutBootstrapTemplate, clusterv1.MachineSetTopologyFinalizer)).To(BeFalse())
		g.Expect(templateExists(fakeClient, msWithoutBootstrapTemplateIMT)).To(BeFalse())
	})

	t.Run("Should not delete templates of a MachineSet when they are still in use in a MachineDeployment", func(t *testing.T) {
		g := NewWithT(t)

		// Copying the MS so changes made by reconcileDelete do not affect other tests.
		ms := ms.DeepCopy()

		md := builder.MachineDeployment(metav1.NamespaceDefault, "md").
			WithBootstrapTemplate(msBT).
			WithInfrastructureTemplate(msIMT).
			Build()

		fakeClient := fake.NewClientBuilder().
			WithScheme(fakeScheme).
			WithObjects(md, ms, msBT, msIMT).
			Build()

		r := &Reconciler{
			Client:    fakeClient,
			APIReader: fakeClient,
		}
		err := r.reconcileDelete(ctx, ms)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controllerutil.ContainsFinalizer(ms, clusterv1.MachineSetTopologyFinalizer)).To(BeFalse())
		g.Expect(templateExists(fakeClient, msBT)).To(BeTrue())
		g.Expect(templateExists(fakeClient, msIMT)).To(BeTrue())
	})

	t.Run("Should not delete templates of a MachineSet when they are still in use in another MachineSet", func(t *testing.T) {
		g := NewWithT(t)

		md := builder.MachineDeployment(metav1.NamespaceDefault, "md").
			WithBootstrapTemplate(msBT).
			WithInfrastructureTemplate(msIMT).
			Build()
		md.SetDeletionTimestamp(&deletionTimeStamp)
		md.SetFinalizers([]string{clusterv1.MachineDeploymentTopologyFinalizer})

		// anotherMS is another MachineSet of the same MachineDeployment using the same templates.
		// Because anotherMS is not in deleting, reconcileDelete should not delete the templates.
		anotherMS := builder.MachineSet(metav1.NamespaceDefault, "anotherMS").
			WithBootstrapTemplate(msBT).
			WithInfrastructureTemplate(msIMT).
			WithLabels(map[string]string{
				clusterv1.MachineDeploymentNameLabel: mdName,
			}).
			Build()
		anotherMS.SetOwnerReferences([]metav1.OwnerReference{
			{
				Kind:       "MachineDeployment",
				APIVersion: clusterv1.GroupVersion.String(),
				Name:       "md",
			},
		})

		fakeClient := fake.NewClientBuilder().
			WithScheme(fakeScheme).
			WithObjects(md, anotherMS, ms, msBT, msIMT).
			Build()

		r := &Reconciler{
			Client:    fakeClient,
			APIReader: fakeClient,
		}
		err := r.reconcileDelete(ctx, ms)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(controllerutil.ContainsFinalizer(ms, clusterv1.MachineSetTopologyFinalizer)).To(BeFalse())
		g.Expect(templateExists(fakeClient, msBT)).To(BeTrue())
		g.Expect(templateExists(fakeClient, msIMT)).To(BeTrue())
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
