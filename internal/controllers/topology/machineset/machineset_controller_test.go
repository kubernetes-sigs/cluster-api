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

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/test/builder"
)

func TestMachineSetReconciler_ReconcileDelete(t *testing.T) {
	deletionTimeStamp := metav1.Now()

	mdName := "md"

	msBT := builder.BootstrapTemplate(metav1.NamespaceDefault, "msBT").Build()
	msIMT := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "msIMT").Build()
	ms := builder.MachineSet(metav1.NamespaceDefault, "ms").
		WithBootstrapTemplate(msBT).
		WithInfrastructureTemplate(msIMT).
		WithLabels(map[string]string{
			clusterv1.MachineDeploymentLabelName: mdName,
		}).
		Build()
	ms.SetDeletionTimestamp(&deletionTimeStamp)
	ms.SetOwnerReferences([]metav1.OwnerReference{
		{
			Kind:       "MachineDeployment",
			APIVersion: clusterv1.GroupVersion.String(),
			Name:       "md",
		},
	})

	t.Run("Should delete templates of a MachineSet", func(t *testing.T) {
		g := NewWithT(t)

		fakeClient := fake.NewClientBuilder().
			WithScheme(fakeScheme).
			WithObjects(ms, msBT, msIMT).
			Build()

		r := &Reconciler{
			Client:    fakeClient,
			APIReader: fakeClient,
		}
		_, err := r.reconcileDelete(ctx, ms)
		g.Expect(err).ToNot(HaveOccurred())

		afterMS := &clusterv1.MachineSet{}
		g.Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(ms), afterMS)).To(Succeed())

		g.Expect(controllerutil.ContainsFinalizer(afterMS, clusterv1.MachineSetTopologyFinalizer)).To(BeFalse())
		g.Expect(templateExists(fakeClient, msBT)).To(BeFalse())
		g.Expect(templateExists(fakeClient, msIMT)).To(BeFalse())
	})

	t.Run("Should delete infra template of a MachineSet without a bootstrap template", func(t *testing.T) {
		g := NewWithT(t)

		msWithoutBootstrapTemplateIMT := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "msWithoutBootstrapTemplateIMT").Build()
		msWithoutBootstrapTemplate := builder.MachineSet(metav1.NamespaceDefault, "msWithoutBootstrapTemplate").
			WithInfrastructureTemplate(msWithoutBootstrapTemplateIMT).
			WithLabels(map[string]string{
				clusterv1.MachineDeploymentLabelName: mdName,
			}).
			Build()
		msWithoutBootstrapTemplate.SetDeletionTimestamp(&deletionTimeStamp)
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
		_, err := r.reconcileDelete(ctx, msWithoutBootstrapTemplate)
		g.Expect(err).ToNot(HaveOccurred())

		afterMS := &clusterv1.MachineSet{}
		g.Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(msWithoutBootstrapTemplate), afterMS)).To(Succeed())

		g.Expect(controllerutil.ContainsFinalizer(afterMS, clusterv1.MachineSetTopologyFinalizer)).To(BeFalse())
		g.Expect(templateExists(fakeClient, msWithoutBootstrapTemplateIMT)).To(BeFalse())
	})

	t.Run("Should not delete templates of a MachineSet when they are still in use in a MachineDeployment", func(t *testing.T) {
		g := NewWithT(t)

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
		_, err := r.reconcileDelete(ctx, ms)
		g.Expect(err).ToNot(HaveOccurred())

		afterMS := &clusterv1.MachineSet{}
		g.Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(ms), afterMS)).To(Succeed())

		g.Expect(controllerutil.ContainsFinalizer(afterMS, clusterv1.MachineSetTopologyFinalizer)).To(BeFalse())
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

		// anotherMS is another MachineSet of the same MachineDeployment using the same templates.
		// Because anotherMS is not in deleting, reconcileDelete should not delete the templates.
		anotherMS := builder.MachineSet(metav1.NamespaceDefault, "anotherMS").
			WithBootstrapTemplate(msBT).
			WithInfrastructureTemplate(msIMT).
			WithLabels(map[string]string{
				clusterv1.MachineDeploymentLabelName: mdName,
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
		_, err := r.reconcileDelete(ctx, ms)
		g.Expect(err).ToNot(HaveOccurred())

		afterMS := &clusterv1.MachineSet{}
		g.Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(ms), afterMS)).To(Succeed())

		g.Expect(controllerutil.ContainsFinalizer(afterMS, clusterv1.MachineSetTopologyFinalizer)).To(BeFalse())
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
