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

package topology

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestMachineDeploymentReconciler_ReconcileDelete(t *testing.T) {
	deletionTimeStamp := metav1.Now()

	mdBT := builder.BootstrapTemplate(metav1.NamespaceDefault, "mdBT").Build()
	mdIMT := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "mdIMT").Build()
	md := builder.MachineDeployment(metav1.NamespaceDefault, "md").
		WithBootstrapTemplate(mdBT).
		WithInfrastructureTemplate(mdIMT).
		Build()
	md.SetDeletionTimestamp(&deletionTimeStamp)

	t.Run("Should delete templates of a MachineDeployment", func(t *testing.T) {
		g := NewWithT(t)

		fakeClient := fake.NewClientBuilder().
			WithScheme(fakeScheme).
			WithObjects(md, mdBT, mdIMT).
			Build()

		r := &MachineDeploymentReconciler{
			Client:    fakeClient,
			APIReader: fakeClient,
		}
		_, err := r.reconcileDelete(ctx, md)
		g.Expect(err).ToNot(HaveOccurred())

		afterMD := &clusterv1.MachineDeployment{}
		g.Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(md), afterMD)).To(Succeed())

		g.Expect(controllerutil.ContainsFinalizer(afterMD, clusterv1.MachineDeploymentTopologyFinalizer)).To(BeFalse())
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

		fakeClient := fake.NewClientBuilder().
			WithScheme(fakeScheme).
			WithObjects(mdWithoutBootstrapTemplate, mdWithoutBootstrapTemplateIMT).
			Build()

		r := &MachineDeploymentReconciler{
			Client:    fakeClient,
			APIReader: fakeClient,
		}
		_, err := r.reconcileDelete(ctx, mdWithoutBootstrapTemplate)
		g.Expect(err).ToNot(HaveOccurred())

		afterMD := &clusterv1.MachineDeployment{}
		g.Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(mdWithoutBootstrapTemplate), afterMD)).To(Succeed())

		g.Expect(controllerutil.ContainsFinalizer(afterMD, clusterv1.MachineDeploymentTopologyFinalizer)).To(BeFalse())
		g.Expect(templateExists(fakeClient, mdWithoutBootstrapTemplateIMT)).To(BeFalse())
	})

	t.Run("Should not delete templates of a MachineDeployment when they are still in use in a MachineSet", func(t *testing.T) {
		g := NewWithT(t)

		ms := builder.MachineSet(md.Namespace, "ms").
			WithBootstrapTemplate(mdBT).
			WithInfrastructureTemplate(mdIMT).
			WithLabels(map[string]string{
				clusterv1.MachineDeploymentLabelName: md.Name,
			}).
			Build()

		fakeClient := fake.NewClientBuilder().
			WithScheme(fakeScheme).
			WithObjects(md, ms, mdBT, mdIMT).
			Build()

		r := &MachineDeploymentReconciler{
			Client:    fakeClient,
			APIReader: fakeClient,
		}
		_, err := r.reconcileDelete(ctx, md)
		g.Expect(err).ToNot(HaveOccurred())

		afterMD := &clusterv1.MachineDeployment{}
		g.Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(md), afterMD)).To(Succeed())

		g.Expect(controllerutil.ContainsFinalizer(afterMD, clusterv1.MachineDeploymentTopologyFinalizer)).To(BeFalse())
		g.Expect(templateExists(fakeClient, mdBT)).To(BeTrue())
		g.Expect(templateExists(fakeClient, mdIMT)).To(BeTrue())
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
