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

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/klogr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	kubeadmv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/secret"
)

type templateCloner struct{}

func (tc *templateCloner) CloneTemplate(_ context.Context, _ client.Client, ref *corev1.ObjectReference, namespace string, _ string, _ *metav1.OwnerReference) (*corev1.ObjectReference, error) {
	result := &corev1.ObjectReference{
		Kind:       "Clone",
		APIVersion: ref.APIVersion,
		Namespace:  namespace,
		Name:       "clone",
	}
	return result, nil
}

type kubeadmConfigGenerator struct{}

func (kcg *kubeadmConfigGenerator) GenerateKubeadmConfig(_ context.Context, _ client.Client, namespace, _, _ string, _ *bootstrapv1.KubeadmConfigSpec, _ *metav1.OwnerReference) (*corev1.ObjectReference, error) {
	result := &corev1.ObjectReference{
		Kind:       "KubeadmConfig",
		APIVersion: bootstrapv1.GroupVersion.String(),
		Namespace:  namespace,
		Name:       "generatedKubeadmConfig",
	}
	return result, nil
}

type machineGenerator struct{}

func (mg *machineGenerator) GenerateMachine(ctx context.Context, c client.Client, namespace, _, _, _ string, infraRef, bootstrapRef *corev1.ObjectReference, _ map[string]string, _ *metav1.OwnerReference) error {
	machine := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.Version,
			Kind:       "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "generatedMachine",
			Namespace: namespace,
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: *infraRef,
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: bootstrapRef,
			},
		},
	}

	if err := c.Create(ctx, machine); err != nil {
		return err
	}
	return nil
}

func TestClusterToKubeadmControlPlane(t *testing.T) {
	g := gomega.NewWithT(t)
	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "test",
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: &corev1.ObjectReference{
				Kind:       "KubeadmControlPlane",
				Namespace:  "test",
				Name:       "kcp-foo",
				APIVersion: controlplanev1.GroupVersion.String(),
			},
		},
	}

	expectedResult := []ctrl.Request{
		{
			NamespacedName: types.NamespacedName{
				Namespace: cluster.Spec.ControlPlaneRef.Namespace,
				Name:      cluster.Spec.ControlPlaneRef.Name},
		},
	}

	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
	}

	got := r.ClusterToKubeadmControlPlane(
		handler.MapObject{
			Meta:   cluster.GetObjectMeta(),
			Object: cluster,
		},
	)
	g.Expect(got).To(gomega.Equal(expectedResult))
}

func TestClusterToKubeadmControlPlaneNoControlPlane(t *testing.T) {
	g := gomega.NewWithT(t)
	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "test",
		},
		Spec: clusterv1.ClusterSpec{},
	}

	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
	}

	got := r.ClusterToKubeadmControlPlane(
		handler.MapObject{
			Meta:   cluster.GetObjectMeta(),
			Object: cluster,
		},
	)
	g.Expect(got).To(gomega.BeNil())
}

func TestClusterToKubeadmControlPlaneOtherControlPlane(t *testing.T) {
	g := gomega.NewWithT(t)
	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "test",
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: &corev1.ObjectReference{
				Kind:       "OtherControlPlane",
				Namespace:  "test",
				Name:       "other-foo",
				APIVersion: controlplanev1.GroupVersion.String(),
			},
		},
	}

	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
	}

	got := r.ClusterToKubeadmControlPlane(
		handler.MapObject{
			Meta:   cluster.GetObjectMeta(),
			Object: cluster,
		},
	)
	g.Expect(got).To(gomega.BeNil())
}

func TestReconcileKubeconfigEmptyAPIEndpoints(t *testing.T) {
	g := gomega.NewWithT(t)

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "test",
		},
	}
	clusterName := types.NamespacedName{Namespace: "test", Name: "foo"}

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, kcp)
	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
	}

	g.Expect(r.reconcileKubeconfig(context.Background(), clusterName, clusterv1.APIEndpoint{}, kcp)).To(gomega.Succeed())

	kubeconfigSecret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Namespace: "test",
		Name:      secret.Name(clusterName.Name, secret.Kubeconfig),
	}
	g.Expect(r.Client.Get(context.Background(), secretName, kubeconfigSecret)).To(gomega.MatchError(gomega.ContainSubstring("not found")))
}

func TestReconcileKubeconfigMissingCACertificate(t *testing.T) {
	g := gomega.NewWithT(t)

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "test",
		},
	}
	clusterName := types.NamespacedName{Namespace: "test", Name: "foo"}
	endpoint := clusterv1.APIEndpoint{Host: "test.local", Port: 8443}

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, kcp)
	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
	}

	g.Expect(r.reconcileKubeconfig(context.Background(), clusterName, endpoint, kcp)).NotTo(gomega.Succeed())

	kubeconfigSecret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Namespace: "test",
		Name:      secret.Name(clusterName.Name, secret.Kubeconfig),
	}
	g.Expect(r.Client.Get(context.Background(), secretName, kubeconfigSecret)).To(gomega.MatchError(gomega.ContainSubstring("not found")))
}

func TestReconcileKubeconfigSecretAlreadyExists(t *testing.T) {
	g := gomega.NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "test",
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "test",
		},
	}
	clusterName := types.NamespacedName{Namespace: cluster.Namespace, Name: cluster.Name}
	endpoint := clusterv1.APIEndpoint{Host: "test.local", Port: 8443}

	existingKubeconfigSecret := kubeconfig.GenerateSecretWithOwner(
		types.NamespacedName{Name: "foo", Namespace: "test"},
		[]byte{},
		*metav1.NewControllerRef(cluster, clusterv1.GroupVersion.WithKind("Cluster")),
	)

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, kcp, existingKubeconfigSecret)
	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
	}

	g.Expect(r.reconcileKubeconfig(context.Background(), clusterName, endpoint, kcp)).To(gomega.Succeed())

	kubeconfigSecret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Namespace: "test",
		Name:      secret.Name(clusterName.Name, secret.Kubeconfig),
	}
	g.Expect(r.Client.Get(context.Background(), secretName, kubeconfigSecret)).To(gomega.Succeed())
	g.Expect(kubeconfigSecret.Labels).To(gomega.Equal(existingKubeconfigSecret.Labels))
	g.Expect(kubeconfigSecret.Data).To(gomega.Equal(existingKubeconfigSecret.Data))
	g.Expect(kubeconfigSecret.OwnerReferences).NotTo(gomega.ContainElement(*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane"))))

}

func TestKubeadmControlPlaneReconciler_reconcileKubeconfig(t *testing.T) {
	g := gomega.NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "test",
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "test",
		},
	}
	clusterName := types.NamespacedName{Namespace: cluster.Namespace, Name: cluster.Name}
	endpoint := clusterv1.APIEndpoint{Host: "test.local", Port: 8443}

	clusterCerts := secret.NewCertificatesForInitialControlPlane(&kubeadmv1.ClusterConfiguration{})
	g.Expect(clusterCerts.Generate()).To(gomega.Succeed())
	caCert := clusterCerts.GetByPurpose(secret.ClusterCA)
	existingCACertSecret := caCert.AsSecret(
		types.NamespacedName{Namespace: "test", Name: "foo"},
		*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane")),
	)

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, kcp, existingCACertSecret)
	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
	}
	g.Expect(r.reconcileKubeconfig(context.Background(), clusterName, endpoint, kcp)).To(gomega.Succeed())

	kubeconfigSecret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Namespace: "test",
		Name:      secret.Name(clusterName.Name, secret.Kubeconfig),
	}
	g.Expect(r.Client.Get(context.Background(), secretName, kubeconfigSecret)).To(gomega.Succeed())
	g.Expect(kubeconfigSecret.OwnerReferences).NotTo(gomega.BeEmpty())
	g.Expect(kubeconfigSecret.OwnerReferences).To(gomega.ContainElement(*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane"))))
	g.Expect(kubeconfigSecret.Labels).To(gomega.HaveKeyWithValue(clusterv1.ClusterLabelName, clusterName.Name))
}

func TestKubeadmControlPlaneReconciler_initializeControlPlane(t *testing.T) {
	g := gomega.NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "test",
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: &corev1.ObjectReference{
				Kind:       "KubeadmControlPlane",
				Namespace:  "test",
				Name:       "kcp-foo",
				APIVersion: controlplanev1.GroupVersion.String(),
			},
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kcp-foo",
			Namespace: cluster.Namespace,
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			InfrastructureTemplate: corev1.ObjectReference{
				Kind:       "GenericMachineTemplate",
				Namespace:  "test",
				Name:       "infra-foo",
				APIVersion: "generic.io/v1",
			},
		},
	}

	genericMachineTemplate := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericMachineTemplate",
			"apiVersion": "generic.io/v1",
			"metadata": map[string]interface{}{
				"name":      "infra-foo",
				"namespace": "test",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"hello": "world",
					},
				},
			},
		},
	}

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(
		scheme.Scheme,
		cluster.DeepCopy(),
		kcp.DeepCopy(),
		genericMachineTemplate.DeepCopy(),
	)

	r := &KubeadmControlPlaneReconciler{
		Client:                 fakeClient,
		Log:                    log.Log,
		TemplateCloner:         &templateCloner{},
		KubeadmConfigGenerator: &kubeadmConfigGenerator{},
		MachineGenerator:       &machineGenerator{},
	}

	g.Expect(r.initializeControlPlane(context.Background(), cluster, kcp, log.Log)).To(gomega.Succeed())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace(cluster.Namespace))).To(gomega.Succeed())
	g.Expect(machineList.Items).NotTo(gomega.BeEmpty())
	g.Expect(machineList.Items).To(gomega.HaveLen(1))

	g.Expect(machineList.Items[0].Namespace).To(gomega.Equal(cluster.Namespace))
	g.Expect(machineList.Items[0].Name).NotTo(gomega.BeEmpty())
	g.Expect(machineList.Items[0].Name).To(gomega.Equal("generatedMachine"))

	g.Expect(machineList.Items[0].Spec.InfrastructureRef.Namespace).To(gomega.Equal(cluster.Namespace))
	g.Expect(machineList.Items[0].Spec.InfrastructureRef.Name).NotTo(gomega.BeEmpty())
	g.Expect(machineList.Items[0].Spec.InfrastructureRef.Name).To(gomega.Equal("clone"))
	g.Expect(machineList.Items[0].Spec.InfrastructureRef.APIVersion).To(gomega.Equal("generic.io/v1"))
	g.Expect(machineList.Items[0].Spec.InfrastructureRef.Kind).To(gomega.Equal("Clone"))

	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.Namespace).To(gomega.Equal(cluster.Namespace))
	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.Name).NotTo(gomega.BeEmpty())
	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.Name).To(gomega.Equal("generatedKubeadmConfig"))
	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.APIVersion).To(gomega.Equal(bootstrapv1.GroupVersion.String()))
	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.Kind).To(gomega.Equal("KubeadmConfig"))
}

func TestReconcileNoClusterOwnerRef(t *testing.T) {
	g := gomega.NewWithT(t)

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Name:      "foo",
		},
	}
	kcp.Default()
	g.Expect(kcp.ValidateCreate()).To(gomega.Succeed())

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, kcp)
	log.SetLogger(klogr.New())

	r := &KubeadmControlPlaneReconciler{
		Client:                 fakeClient,
		Log:                    log.Log,
		TemplateCloner:         &templateCloner{},
		KubeadmConfigGenerator: &kubeadmConfigGenerator{},
		MachineGenerator:       &machineGenerator{},
	}

	result := r.reconcile(context.Background(), kcp, r.Log)
	g.Expect(result).To(gomega.Equal(ctrl.Result{}))

	// Always expect that the Finalizer is set on the passed in resource
	g.Expect(kcp.Finalizers).To(gomega.ContainElement(controlplanev1.KubeadmControlPlaneFinalizer))

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace("test"))).To(gomega.Succeed())
	g.Expect(machineList.Items).To(gomega.BeEmpty())
}

func TestReconcileNoCluster(t *testing.T) {
	g := gomega.NewWithT(t)

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Name:      "foo",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
					Name:       "foo",
				},
			},
		},
	}
	kcp.Default()
	g.Expect(kcp.ValidateCreate()).To(gomega.Succeed())

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, kcp)
	log.SetLogger(klogr.New())

	r := &KubeadmControlPlaneReconciler{
		Client:                 fakeClient,
		Log:                    log.Log,
		TemplateCloner:         &templateCloner{},
		KubeadmConfigGenerator: &kubeadmConfigGenerator{},
		MachineGenerator:       &machineGenerator{},
	}

	result := r.reconcile(context.Background(), kcp, r.Log)
	g.Expect(result).To(gomega.Equal(ctrl.Result{Requeue: true}))

	// Always expect that the Finalizer is set on the passed in resource
	g.Expect(kcp.Finalizers).To(gomega.ContainElement(controlplanev1.KubeadmControlPlaneFinalizer))

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace("test"))).To(gomega.Succeed())
	g.Expect(machineList.Items).To(gomega.BeEmpty())
}

func TestReconcileClusterNoEndpoints(t *testing.T) {
	g := gomega.NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "test",
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      "foo",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
					Name:       cluster.Name,
				},
			},
		},
	}
	kcp.Default()
	g.Expect(kcp.ValidateCreate()).To(gomega.Succeed())

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, kcp, cluster)
	log.SetLogger(klogr.New())

	r := &KubeadmControlPlaneReconciler{
		Client:                 fakeClient,
		Log:                    log.Log,
		TemplateCloner:         &templateCloner{},
		KubeadmConfigGenerator: &kubeadmConfigGenerator{},
		MachineGenerator:       &machineGenerator{},
	}

	result := r.reconcile(context.Background(), kcp, r.Log)
	g.Expect(result).To(gomega.Equal(ctrl.Result{}))

	// Always expect that the Finalizer is set on the passed in resource
	g.Expect(kcp.Finalizers).To(gomega.ContainElement(controlplanev1.KubeadmControlPlaneFinalizer))

	g.Expect(kcp.Status.Selector).NotTo(gomega.BeEmpty())

	_, err := secret.GetFromNamespacedName(fakeClient, client.ObjectKey{Namespace: "test", Name: "foo"}, secret.ClusterCA)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace("test"))).To(gomega.Succeed())
	g.Expect(machineList.Items).To(gomega.BeEmpty())
}

func TestReconcileInitializeControlPlane(t *testing.T) {
	g := gomega.NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "test",
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneEndpoint: clusterv1.APIEndpoint{
				Host: "test.local",
				Port: 9999,
			},
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      "foo",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
					Name:       cluster.Name,
				},
			},
		},
	}
	kcp.Default()
	g.Expect(kcp.ValidateCreate()).To(gomega.Succeed())

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, kcp, cluster)
	log.SetLogger(klogr.New())

	r := &KubeadmControlPlaneReconciler{
		Client:                 fakeClient,
		Log:                    log.Log,
		TemplateCloner:         &templateCloner{},
		KubeadmConfigGenerator: &kubeadmConfigGenerator{},
		MachineGenerator:       &machineGenerator{},
	}

	result := r.reconcile(context.Background(), kcp, r.Log)
	// TODO: This should be changed to ctrl.Result{} after status updates are implemented
	g.Expect(result).To(gomega.Equal(ctrl.Result{Requeue: true}))

	// Always expect that the Finalizer is set on the passed in resource
	g.Expect(kcp.Finalizers).To(gomega.ContainElement(controlplanev1.KubeadmControlPlaneFinalizer))

	g.Expect(kcp.Status.Selector).NotTo(gomega.BeEmpty())

	_, err := secret.GetFromNamespacedName(fakeClient, client.ObjectKey{Namespace: "test", Name: "foo"}, secret.ClusterCA)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	c := &clusterv1.Cluster{}
	g.Expect(fakeClient.Get(context.Background(), client.ObjectKey{Namespace: "test", Name: "foo"}, c)).To(gomega.Succeed())
	k, err := kubeconfig.FromSecret(fakeClient, c)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(k).NotTo(gomega.BeEmpty())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace("test"))).To(gomega.Succeed())
	g.Expect(machineList.Items).To(gomega.HaveLen(1))
}
