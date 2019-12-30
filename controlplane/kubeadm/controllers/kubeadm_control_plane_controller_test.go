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
	"fmt"
	"testing"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/klogr"
	utilpointer "k8s.io/utils/pointer"
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
		Client: fakeClient,
		Log:    log.Log,
	}

	g.Expect(r.initializeControlPlane(context.Background(), cluster, kcp, log.Log)).To(gomega.Succeed())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace(cluster.Namespace))).To(gomega.Succeed())
	g.Expect(machineList.Items).NotTo(gomega.BeEmpty())
	g.Expect(machineList.Items).To(gomega.HaveLen(1))

	g.Expect(machineList.Items[0].Namespace).To(gomega.Equal(cluster.Namespace))
	g.Expect(machineList.Items[0].Name).To(gomega.HavePrefix(kcp.Name))

	g.Expect(machineList.Items[0].Spec.InfrastructureRef.Namespace).To(gomega.Equal(cluster.Namespace))
	g.Expect(machineList.Items[0].Spec.InfrastructureRef.Name).To(gomega.HavePrefix(genericMachineTemplate.GetName()))
	g.Expect(machineList.Items[0].Spec.InfrastructureRef.APIVersion).To(gomega.Equal(genericMachineTemplate.GetAPIVersion()))
	g.Expect(machineList.Items[0].Spec.InfrastructureRef.Kind).To(gomega.Equal("GenericMachine"))

	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.Namespace).To(gomega.Equal(cluster.Namespace))
	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.Name).To(gomega.HavePrefix(kcp.Name))
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
		Client: fakeClient,
		Log:    log.Log,
	}

	result, err := r.reconcile(context.Background(), kcp, r.Log)
	g.Expect(err).NotTo(gomega.HaveOccurred())
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
		Client: fakeClient,
		Log:    log.Log,
	}

	_, err := r.reconcile(context.Background(), kcp, r.Log)
	g.Expect(err).To(gomega.HaveOccurred())

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
		Client: fakeClient,
		Log:    log.Log,
		remoteClient: func(c client.Client, _ *clusterv1.Cluster, _ *runtime.Scheme) (client.Client, error) {
			return c, nil
		},
	}

	result, err := r.reconcile(context.Background(), kcp, r.Log)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(result).To(gomega.Equal(ctrl.Result{}))

	// Always expect that the Finalizer is set on the passed in resource
	g.Expect(kcp.Finalizers).To(gomega.ContainElement(controlplanev1.KubeadmControlPlaneFinalizer))

	g.Expect(kcp.Status.Selector).NotTo(gomega.BeEmpty())

	_, err = secret.GetFromNamespacedName(fakeClient, client.ObjectKey{Namespace: "test", Name: "foo"}, secret.ClusterCA)
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

	genericMachineTemplate := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericMachineTemplate",
			"apiVersion": "generic.io/v1",
			"metadata": map[string]interface{}{
				"name":      "infra-foo",
				"namespace": cluster.Namespace,
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
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Replicas: nil,
			Version:  "",
			InfrastructureTemplate: corev1.ObjectReference{
				Kind:       genericMachineTemplate.GetKind(),
				APIVersion: genericMachineTemplate.GetAPIVersion(),
				Name:       genericMachineTemplate.GetName(),
				Namespace:  cluster.Namespace,
			},
			KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{},
		},
	}
	kcp.Default()
	g.Expect(kcp.ValidateCreate()).To(gomega.Succeed())

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(
		scheme.Scheme,
		kcp.DeepCopy(),
		cluster.DeepCopy(),
		genericMachineTemplate.DeepCopy(),
	)
	log.SetLogger(klogr.New())

	expectedLabels := map[string]string{clusterv1.ClusterLabelName: "foo"}

	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
		remoteClient: func(c client.Client, _ *clusterv1.Cluster, _ *runtime.Scheme) (client.Client, error) {
			return c, nil
		},
	}

	result, err := r.reconcile(context.Background(), kcp, r.Log)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(result).To(gomega.Equal(ctrl.Result{}))

	// Always expect that the Finalizer is set on the passed in resource
	g.Expect(kcp.Finalizers).To(gomega.ContainElement(controlplanev1.KubeadmControlPlaneFinalizer))

	g.Expect(kcp.Status.Selector).NotTo(gomega.BeEmpty())
	g.Expect(kcp.Status.Replicas).To(gomega.BeEquivalentTo(1))

	secret, err := secret.GetFromNamespacedName(fakeClient, client.ObjectKey{Namespace: "test", Name: "foo"}, secret.ClusterCA)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(secret).NotTo(gomega.BeNil())
	g.Expect(secret.Data).NotTo(gomega.BeEmpty())
	g.Expect(secret.Labels).To(gomega.Equal(expectedLabels))

	k, err := kubeconfig.FromSecret(fakeClient, cluster)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(k).NotTo(gomega.BeEmpty())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace("test"))).To(gomega.Succeed())
	g.Expect(machineList.Items).To(gomega.HaveLen(1))

	machine := machineList.Items[0]
	g.Expect(machine.Name).To(gomega.HavePrefix(kcp.Name))
}

func TestKubeadmControlPlaneReconciler_generateMachineWithOwner(t *testing.T) {
	g := gomega.NewWithT(t)
	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme)

	namespace := "test"
	namePrefix := "generate2"
	clusterName := "testCluster"
	version := "my-version"
	infraRef := &corev1.ObjectReference{
		Kind:       "InfraKind",
		APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
		Name:       "infra",
		Namespace:  namespace,
	}
	bootstrapRef := &corev1.ObjectReference{
		Kind:       "BootstrapKind",
		APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha3",
		Name:       "bootstrap",
		Namespace:  namespace,
	}
	owner := metav1.OwnerReference{
		Kind:       "Cluster",
		APIVersion: clusterv1.GroupVersion.String(),
		Name:       clusterName,
	}
	labels := map[string]string{"color": "blue"}
	expectedMachineSpec := clusterv1.MachineSpec{
		ClusterName: clusterName,
		Version:     utilpointer.StringPtr(version),
		Bootstrap: clusterv1.Bootstrap{
			ConfigRef: bootstrapRef.DeepCopy(),
		},
		InfrastructureRef: *infraRef.DeepCopy(),
	}
	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
	}
	g.Expect(r.generateMachine(context.Background(), namespace, namePrefix, clusterName, version, infraRef, bootstrapRef, labels, owner.DeepCopy())).To(gomega.Succeed())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace(namespace))).To(gomega.Succeed())
	g.Expect(machineList.Items).NotTo(gomega.BeEmpty())
	g.Expect(machineList.Items).To(gomega.HaveLen(1))
	machine := machineList.Items[0]
	g.Expect(machine.Name).To(gomega.HavePrefix(namePrefix))
	g.Expect(machine.Namespace).To(gomega.Equal(namespace))
	g.Expect(machine.Labels).To(gomega.Equal(labels))
	g.Expect(machine.OwnerReferences).To(gomega.HaveLen(1))
	g.Expect(machine.OwnerReferences).To(gomega.ContainElement(owner))
	g.Expect(machine.Spec).To(gomega.Equal(expectedMachineSpec))
}

func TestKubeadmControlPlaneReconciler_generateMachineWithoutOwner(t *testing.T) {
	g := gomega.NewWithT(t)
	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme)

	namespace := "test"
	namePrefix := "generate2"
	clusterName := "testCluster"
	version := "my-version"
	infraRef := &corev1.ObjectReference{
		Kind:       "InfraKind",
		APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
		Name:       "infra",
		Namespace:  namespace,
	}
	bootstrapRef := &corev1.ObjectReference{
		Kind:       "BootstrapKind",
		APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha3",
		Name:       "bootstrap",
		Namespace:  namespace,
	}
	labels := map[string]string{"color": "blue"}
	expectedMachineSpec := clusterv1.MachineSpec{
		ClusterName: clusterName,
		Version:     utilpointer.StringPtr(version),
		Bootstrap: clusterv1.Bootstrap{
			ConfigRef: bootstrapRef.DeepCopy(),
		},
		InfrastructureRef: *infraRef.DeepCopy(),
	}
	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
	}
	g.Expect(r.generateMachine(context.Background(), namespace, namePrefix, clusterName, version, infraRef, bootstrapRef, labels, nil)).To(gomega.Succeed())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace(namespace))).To(gomega.Succeed())
	g.Expect(machineList.Items).NotTo(gomega.BeEmpty())
	g.Expect(machineList.Items).To(gomega.HaveLen(1))
	machine := machineList.Items[0]
	g.Expect(machine.Name).To(gomega.HavePrefix(namePrefix))
	g.Expect(machine.Namespace).To(gomega.Equal(namespace))
	g.Expect(machine.Labels).To(gomega.Equal(labels))
	g.Expect(machine.OwnerReferences).To(gomega.BeEmpty())
	g.Expect(machine.Spec).To(gomega.Equal(expectedMachineSpec))
}

func TestKubeadmControlPlaneReconciler_generateKubeadmConfigWithOwner(t *testing.T) {
	g := gomega.NewWithT(t)
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme)

	namespace := "test"
	namePrefix := "generate"
	clusterName := "foo"
	spec := bootstrapv1.KubeadmConfigSpec{}
	owner := metav1.OwnerReference{
		Kind:       "Cluster",
		APIVersion: clusterv1.GroupVersion.String(),
		Name:       clusterName,
	}
	expectedReferenceKind := "KubeadmConfig"
	expectedReferenceAPIVersion := bootstrapv1.GroupVersion.String()
	expectedLabels := map[string]string{clusterv1.ClusterLabelName: "foo"}

	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
	}

	got, err := r.generateKubeadmConfig(
		context.Background(),
		namespace,
		namePrefix,
		clusterName,
		spec.DeepCopy(),
		owner.DeepCopy(),
	)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(got).NotTo(gomega.BeNil())
	g.Expect(got.Name).To(gomega.HavePrefix(namePrefix))
	g.Expect(got.Namespace).To(gomega.Equal(namespace))
	g.Expect(got.Kind).To(gomega.Equal(expectedReferenceKind))
	g.Expect(got.APIVersion).To(gomega.Equal(expectedReferenceAPIVersion))

	bootstrapConfig := &bootstrapv1.KubeadmConfig{}
	key := client.ObjectKey{Name: got.Name, Namespace: got.Namespace}
	g.Expect(fakeClient.Get(context.Background(), key, bootstrapConfig)).To(gomega.Succeed())
	g.Expect(bootstrapConfig.Labels).To(gomega.Equal(expectedLabels))
	g.Expect(bootstrapConfig.OwnerReferences).To(gomega.HaveLen(1))
	g.Expect(bootstrapConfig.OwnerReferences).To(gomega.ContainElement(owner))
	g.Expect(bootstrapConfig.Spec).To(gomega.Equal(spec))
}

func TestKubeadmControlPlaneReconciler_generateKubeadmConfigWithoutOwner(t *testing.T) {
	g := gomega.NewWithT(t)
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme)

	namespace := "test"
	namePrefix := "generate"
	clusterName := "foo"
	spec := bootstrapv1.KubeadmConfigSpec{}
	expectedReferenceKind := "KubeadmConfig"
	expectedReferenceAPIVersion := bootstrapv1.GroupVersion.String()
	expectedLabels := map[string]string{clusterv1.ClusterLabelName: "foo"}

	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
	}

	got, err := r.generateKubeadmConfig(
		context.Background(),
		namespace,
		namePrefix,
		clusterName,
		spec.DeepCopy(),
		nil,
	)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(got).NotTo(gomega.BeNil())
	g.Expect(got.Name).To(gomega.HavePrefix(namePrefix))
	g.Expect(got.Namespace).To(gomega.Equal(namespace))
	g.Expect(got.Kind).To(gomega.Equal(expectedReferenceKind))
	g.Expect(got.APIVersion).To(gomega.Equal(expectedReferenceAPIVersion))

	bootstrapConfig := &bootstrapv1.KubeadmConfig{}
	key := client.ObjectKey{Name: got.Name, Namespace: got.Namespace}
	g.Expect(fakeClient.Get(context.Background(), key, bootstrapConfig)).To(gomega.Succeed())
	g.Expect(bootstrapConfig.Labels).To(gomega.Equal(expectedLabels))
	g.Expect(bootstrapConfig.OwnerReferences).To(gomega.BeEmpty())
	g.Expect(bootstrapConfig.Spec).To(gomega.Equal(spec))
}

func Test_getMachineNodeNoNodeRef(t *testing.T) {
	g := gomega.NewWithT(t)

	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme)

	m := &clusterv1.Machine{}
	node, err := getMachineNode(context.Background(), fakeClient, m)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(node).To(gomega.BeNil())
}

func Test_getMachineNodeNotFound(t *testing.T) {
	g := gomega.NewWithT(t)

	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme)

	m := &clusterv1.Machine{
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Kind:       "Node",
				APIVersion: corev1.SchemeGroupVersion.String(),
				Name:       "notfound",
			},
		},
	}
	node, err := getMachineNode(context.Background(), fakeClient, m)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(node).To(gomega.BeNil())
}

func Test_getMachineNodeFound(t *testing.T) {
	g := gomega.NewWithT(t)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testNode",
		},
	}
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, node.DeepCopy())

	m := &clusterv1.Machine{
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Kind:       "Node",
				APIVersion: corev1.SchemeGroupVersion.String(),
				Name:       "testNode",
			},
		},
	}
	node, err := getMachineNode(context.Background(), fakeClient, m)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(node).To(gomega.Equal(node))
}

func TestKubeadmControlPlaneReconciler_updateStatusNoMachines(t *testing.T) {
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
		Client: fakeClient,
		Log:    log.Log,
		remoteClient: func(c client.Client, _ *clusterv1.Cluster, _ *runtime.Scheme) (client.Client, error) {
			return c, nil
		},
	}

	g.Expect(r.updateStatus(context.Background(), kcp, cluster)).To(gomega.Succeed())
	g.Expect(kcp.Status.Replicas).To(gomega.BeEquivalentTo(0))
	g.Expect(kcp.Status.ReadyReplicas).To(gomega.BeEquivalentTo(0))
	g.Expect(kcp.Status.UnavailableReplicas).To(gomega.BeEquivalentTo(0))
	g.Expect(kcp.Status.Initialized).To(gomega.BeFalse())
	g.Expect(kcp.Status.Ready).To(gomega.BeFalse())
	g.Expect(kcp.Status.Selector).NotTo(gomega.BeEmpty())
	g.Expect(kcp.Status.FailureMessage).To(gomega.BeNil())
	g.Expect(kcp.Status.FailureReason).To(gomega.BeEquivalentTo(""))
}

func createMachineNodePair(name string, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane, ready bool) (*clusterv1.Machine, *corev1.Node) {
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cluster.Namespace,
			Labels:    generateKubeadmControlPlaneLabels(cluster.Name),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane")),
			},
		},
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Kind:       "Node",
				APIVersion: corev1.SchemeGroupVersion.String(),
				Name:       name,
			},
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	if ready {
		node.Status.Conditions = []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			},
		}
	}

	return machine, node
}

func TestKubeadmControlPlaneReconciler_updateStatusAllMachinesNotReady(t *testing.T) {
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
		},
	}
	kcp.Default()
	g.Expect(kcp.ValidateCreate()).To(gomega.Succeed())

	objs := []runtime.Object{cluster.DeepCopy(), kcp.DeepCopy()}
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("test-%d", i)
		m, n := createMachineNodePair(name, cluster, kcp, false)
		objs = append(objs, m, n)
	}

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, objs...)
	log.SetLogger(klogr.New())

	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
		remoteClient: func(c client.Client, _ *clusterv1.Cluster, _ *runtime.Scheme) (client.Client, error) {
			return c, nil
		},
	}

	g.Expect(r.updateStatus(context.Background(), kcp, cluster)).To(gomega.Succeed())
	g.Expect(kcp.Status.Replicas).To(gomega.BeEquivalentTo(3))
	g.Expect(kcp.Status.ReadyReplicas).To(gomega.BeEquivalentTo(0))
	g.Expect(kcp.Status.UnavailableReplicas).To(gomega.BeEquivalentTo(3))
	g.Expect(kcp.Status.Selector).NotTo(gomega.BeEmpty())
	g.Expect(kcp.Status.FailureMessage).To(gomega.BeNil())
	g.Expect(kcp.Status.FailureReason).To(gomega.BeEquivalentTo(""))
	g.Expect(kcp.Status.Initialized).To(gomega.BeFalse())
	g.Expect(kcp.Status.Ready).To(gomega.BeFalse())
}

func TestKubeadmControlPlaneReconciler_updateStatusAllMachinesReady(t *testing.T) {
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
		},
	}
	kcp.Default()
	g.Expect(kcp.ValidateCreate()).To(gomega.Succeed())

	objs := []runtime.Object{cluster.DeepCopy(), kcp.DeepCopy()}
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("test-%d", i)
		m, n := createMachineNodePair(name, cluster, kcp, true)
		objs = append(objs, m, n)
	}

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, objs...)
	log.SetLogger(klogr.New())

	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
		remoteClient: func(c client.Client, _ *clusterv1.Cluster, _ *runtime.Scheme) (client.Client, error) {
			return c, nil
		},
	}

	g.Expect(r.updateStatus(context.Background(), kcp, cluster)).To(gomega.Succeed())
	g.Expect(kcp.Status.Replicas).To(gomega.BeEquivalentTo(3))
	g.Expect(kcp.Status.ReadyReplicas).To(gomega.BeEquivalentTo(3))
	g.Expect(kcp.Status.UnavailableReplicas).To(gomega.BeEquivalentTo(0))
	g.Expect(kcp.Status.Selector).NotTo(gomega.BeEmpty())
	g.Expect(kcp.Status.FailureMessage).To(gomega.BeNil())
	g.Expect(kcp.Status.FailureReason).To(gomega.BeEquivalentTo(""))
	g.Expect(kcp.Status.Initialized).To(gomega.BeTrue())

	// TODO: will need to be updated once we start handling Ready
	g.Expect(kcp.Status.Ready).To(gomega.BeFalse())
}

func TestKubeadmControlPlaneReconciler_updateStatusMachinesReadyMixed(t *testing.T) {
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
		},
	}
	kcp.Default()
	g.Expect(kcp.ValidateCreate()).To(gomega.Succeed())

	objs := []runtime.Object{cluster.DeepCopy(), kcp.DeepCopy()}
	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("test-%d", i)
		m, n := createMachineNodePair(name, cluster, kcp, false)
		objs = append(objs, m, n)
	}
	m, n := createMachineNodePair("testReady", cluster, kcp, true)
	objs = append(objs, m, n)

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(gomega.Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, objs...)
	log.SetLogger(klogr.New())

	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
		remoteClient: func(c client.Client, _ *clusterv1.Cluster, _ *runtime.Scheme) (client.Client, error) {
			return c, nil
		},
	}

	g.Expect(r.updateStatus(context.Background(), kcp, cluster)).To(gomega.Succeed())
	g.Expect(kcp.Status.Replicas).To(gomega.BeEquivalentTo(5))
	g.Expect(kcp.Status.ReadyReplicas).To(gomega.BeEquivalentTo(1))
	g.Expect(kcp.Status.UnavailableReplicas).To(gomega.BeEquivalentTo(4))
	g.Expect(kcp.Status.Selector).NotTo(gomega.BeEmpty())
	g.Expect(kcp.Status.FailureMessage).To(gomega.BeNil())
	g.Expect(kcp.Status.FailureReason).To(gomega.BeEquivalentTo(""))
	g.Expect(kcp.Status.Initialized).To(gomega.BeTrue())

	// TODO: will need to be updated once we start handling Ready
	g.Expect(kcp.Status.Ready).To(gomega.BeFalse())
}
