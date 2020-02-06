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

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/klogr"
	"k8s.io/utils/pointer"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	kubeadmv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	fakeremote "sigs.k8s.io/cluster-api/controllers/remote/fake"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	fakeetcd "sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd/fake"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/secret"
)

func TestClusterToKubeadmControlPlane(t *testing.T) {
	g := NewWithT(t)
	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(Succeed())
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
	g.Expect(got).To(Equal(expectedResult))
}

func TestClusterToKubeadmControlPlaneNoControlPlane(t *testing.T) {
	g := NewWithT(t)
	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(Succeed())
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
	g.Expect(got).To(BeNil())
}

func TestClusterToKubeadmControlPlaneOtherControlPlane(t *testing.T) {
	g := NewWithT(t)
	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(Succeed())
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
	g.Expect(got).To(BeNil())
}

func TestReconcileKubeconfigEmptyAPIEndpoints(t *testing.T) {
	g := NewWithT(t)

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "test",
		},
	}
	clusterName := types.NamespacedName{Namespace: "test", Name: "foo"}

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, kcp)
	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
	}

	g.Expect(r.reconcileKubeconfig(context.Background(), clusterName, clusterv1.APIEndpoint{}, kcp)).To(Succeed())

	kubeconfigSecret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Namespace: "test",
		Name:      secret.Name(clusterName.Name, secret.Kubeconfig),
	}
	g.Expect(r.Client.Get(context.Background(), secretName, kubeconfigSecret)).To(MatchError(ContainSubstring("not found")))
}

func TestReconcileKubeconfigMissingCACertificate(t *testing.T) {
	g := NewWithT(t)

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "test",
		},
	}
	clusterName := types.NamespacedName{Namespace: "test", Name: "foo"}
	endpoint := clusterv1.APIEndpoint{Host: "test.local", Port: 8443}

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, kcp)
	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
	}

	g.Expect(r.reconcileKubeconfig(context.Background(), clusterName, endpoint, kcp)).NotTo(Succeed())

	kubeconfigSecret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Namespace: "test",
		Name:      secret.Name(clusterName.Name, secret.Kubeconfig),
	}
	g.Expect(r.Client.Get(context.Background(), secretName, kubeconfigSecret)).To(MatchError(ContainSubstring("not found")))
}

func TestReconcileKubeconfigSecretAlreadyExists(t *testing.T) {
	g := NewWithT(t)

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

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, kcp, existingKubeconfigSecret)
	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
	}

	g.Expect(r.reconcileKubeconfig(context.Background(), clusterName, endpoint, kcp)).To(Succeed())

	kubeconfigSecret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Namespace: "test",
		Name:      secret.Name(clusterName.Name, secret.Kubeconfig),
	}
	g.Expect(r.Client.Get(context.Background(), secretName, kubeconfigSecret)).To(Succeed())
	g.Expect(kubeconfigSecret.Labels).To(Equal(existingKubeconfigSecret.Labels))
	g.Expect(kubeconfigSecret.Data).To(Equal(existingKubeconfigSecret.Data))
	g.Expect(kubeconfigSecret.OwnerReferences).NotTo(ContainElement(*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane"))))

}

func TestKubeadmControlPlaneReconciler_reconcileKubeconfig(t *testing.T) {
	g := NewWithT(t)

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
	g.Expect(clusterCerts.Generate()).To(Succeed())
	caCert := clusterCerts.GetByPurpose(secret.ClusterCA)
	existingCACertSecret := caCert.AsSecret(
		types.NamespacedName{Namespace: "test", Name: "foo"},
		*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane")),
	)

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, kcp, existingCACertSecret)
	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
	}
	g.Expect(r.reconcileKubeconfig(context.Background(), clusterName, endpoint, kcp)).To(Succeed())

	kubeconfigSecret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Namespace: "test",
		Name:      secret.Name(clusterName.Name, secret.Kubeconfig),
	}
	g.Expect(r.Client.Get(context.Background(), secretName, kubeconfigSecret)).To(Succeed())
	g.Expect(kubeconfigSecret.OwnerReferences).NotTo(BeEmpty())
	g.Expect(kubeconfigSecret.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane"))))
	g.Expect(kubeconfigSecret.Labels).To(HaveKeyWithValue(clusterv1.ClusterLabelName, clusterName.Name))
}

func TestKubeadmControlPlaneReconciler_initializeControlPlane(t *testing.T) {
	g := NewWithT(t)

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

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(Succeed())
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

	g.Expect(r.initializeControlPlane(context.Background(), cluster, kcp)).To(Succeed())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(machineList.Items).NotTo(BeEmpty())
	g.Expect(machineList.Items).To(HaveLen(1))

	g.Expect(machineList.Items[0].Namespace).To(Equal(cluster.Namespace))
	g.Expect(machineList.Items[0].Name).To(HavePrefix(kcp.Name))

	g.Expect(machineList.Items[0].Spec.InfrastructureRef.Namespace).To(Equal(cluster.Namespace))
	g.Expect(machineList.Items[0].Spec.InfrastructureRef.Name).To(HavePrefix(genericMachineTemplate.GetName()))
	g.Expect(machineList.Items[0].Spec.InfrastructureRef.APIVersion).To(Equal(genericMachineTemplate.GetAPIVersion()))
	g.Expect(machineList.Items[0].Spec.InfrastructureRef.Kind).To(Equal("GenericMachine"))

	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.Namespace).To(Equal(cluster.Namespace))
	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.Name).To(HavePrefix(kcp.Name))
	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.APIVersion).To(Equal(bootstrapv1.GroupVersion.String()))
	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.Kind).To(Equal("KubeadmConfig"))
}

func TestReconcileNoClusterOwnerRef(t *testing.T) {
	g := NewWithT(t)

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Name:      "foo",
		},
	}
	kcp.Default()
	g.Expect(kcp.ValidateCreate()).To(Succeed())

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, kcp)
	log.SetLogger(klogr.New())

	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
	}

	result, err := r.Reconcile(ctrl.Request{NamespacedName: types.NamespacedName{Name: kcp.Name, Namespace: kcp.Namespace}})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace("test"))).To(Succeed())
	g.Expect(machineList.Items).To(BeEmpty())
}

func TestReconcileNoCluster(t *testing.T) {
	g := NewWithT(t)

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
	g.Expect(kcp.ValidateCreate()).To(Succeed())

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, kcp)
	log.SetLogger(klogr.New())

	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
	}

	_, err := r.Reconcile(ctrl.Request{NamespacedName: types.NamespacedName{Name: kcp.Name, Namespace: kcp.Namespace}})
	g.Expect(err).To(HaveOccurred())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace("test"))).To(Succeed())
	g.Expect(machineList.Items).To(BeEmpty())
}

func TestReconcileClusterNoEndpoints(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "test",
		},
		Status: clusterv1.ClusterStatus{
			InfrastructureReady: true,
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
	g.Expect(kcp.ValidateCreate()).To(Succeed())

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, kcp, cluster)
	log.SetLogger(klogr.New())

	r := &KubeadmControlPlaneReconciler{
		Client:             fakeClient,
		Log:                log.Log,
		remoteClientGetter: fakeremote.NewClusterClient,
	}

	result, err := r.Reconcile(ctrl.Request{NamespacedName: types.NamespacedName{Name: kcp.Name, Namespace: kcp.Namespace}})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))
	g.Expect(r.Client.Get(context.Background(), types.NamespacedName{Name: kcp.Name, Namespace: kcp.Namespace}, kcp)).To(Succeed())

	// Always expect that the Finalizer is set on the passed in resource
	g.Expect(kcp.Finalizers).To(ContainElement(controlplanev1.KubeadmControlPlaneFinalizer))

	g.Expect(kcp.Status.Selector).NotTo(BeEmpty())

	_, err = secret.GetFromNamespacedName(context.Background(), fakeClient, client.ObjectKey{Namespace: "test", Name: "foo"}, secret.ClusterCA)
	g.Expect(err).NotTo(HaveOccurred())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace("test"))).To(Succeed())
	g.Expect(machineList.Items).To(BeEmpty())
}

func TestReconcileInitializeControlPlane(t *testing.T) {
	g := NewWithT(t)

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
		Status: clusterv1.ClusterStatus{
			InfrastructureReady: true,
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
	g.Expect(kcp.ValidateCreate()).To(Succeed())

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(Succeed())
	fakeClient := fake.NewFakeClientWithScheme(
		scheme.Scheme,
		kcp.DeepCopy(),
		cluster.DeepCopy(),
		genericMachineTemplate.DeepCopy(),
	)
	log.SetLogger(klogr.New())

	expectedLabels := map[string]string{clusterv1.ClusterLabelName: "foo"}

	r := &KubeadmControlPlaneReconciler{
		Client:             fakeClient,
		Log:                log.Log,
		remoteClientGetter: fakeremote.NewClusterClient,
		scheme:             scheme.Scheme,
	}

	result, err := r.Reconcile(ctrl.Request{NamespacedName: types.NamespacedName{Name: kcp.Name, Namespace: kcp.Namespace}})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))
	g.Expect(r.Client.Get(context.Background(), types.NamespacedName{Name: kcp.Name, Namespace: kcp.Namespace}, kcp)).To(Succeed())

	// Expect the referenced infrastructure template to have a Cluster Owner Reference.
	g.Expect(fakeClient.Get(context.TODO(), client.ObjectKey{Namespace: genericMachineTemplate.GetNamespace(), Name: genericMachineTemplate.GetName()}, genericMachineTemplate)).To(Succeed())
	g.Expect(genericMachineTemplate.GetOwnerReferences()).To(ContainElement(metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
	}))

	// Always expect that the Finalizer is set on the passed in resource
	g.Expect(kcp.Finalizers).To(ContainElement(controlplanev1.KubeadmControlPlaneFinalizer))

	g.Expect(kcp.Status.Selector).NotTo(BeEmpty())
	g.Expect(kcp.Status.Replicas).To(BeEquivalentTo(1))

	s, err := secret.GetFromNamespacedName(context.Background(), fakeClient, client.ObjectKey{Namespace: "test", Name: "foo"}, secret.ClusterCA)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(s).NotTo(BeNil())
	g.Expect(s.Data).NotTo(BeEmpty())
	g.Expect(s.Labels).To(Equal(expectedLabels))

	k, err := kubeconfig.FromSecret(context.Background(), fakeClient, cluster)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(k).NotTo(BeEmpty())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace("test"))).To(Succeed())
	g.Expect(machineList.Items).To(HaveLen(1))

	machine := machineList.Items[0]
	g.Expect(machine.Name).To(HavePrefix(kcp.Name))
}

func TestKubeadmControlPlaneReconciler_generateMachine(t *testing.T) {
	g := NewWithT(t)
	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testCluster",
			Namespace: "test",
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testControlPlane",
			Namespace: cluster.Namespace,
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "my-version",
		},
	}

	infraRef := &corev1.ObjectReference{
		Kind:       "InfraKind",
		APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
		Name:       "infra",
		Namespace:  cluster.Namespace,
	}
	bootstrapRef := &corev1.ObjectReference{
		Kind:       "BootstrapKind",
		APIVersion: "bootstrap.cluster.x-k8s.io/v1alpha3",
		Name:       "bootstrap",
		Namespace:  cluster.Namespace,
	}
	expectedMachineSpec := clusterv1.MachineSpec{
		ClusterName: cluster.Name,
		Version:     utilpointer.StringPtr(kcp.Spec.Version),
		Bootstrap: clusterv1.Bootstrap{
			ConfigRef: bootstrapRef.DeepCopy(),
		},
		InfrastructureRef: *infraRef.DeepCopy(),
	}
	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
	}
	g.Expect(r.generateMachine(context.Background(), kcp, cluster, infraRef, bootstrapRef)).To(Succeed())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(machineList.Items).NotTo(BeEmpty())
	g.Expect(machineList.Items).To(HaveLen(1))
	machine := machineList.Items[0]
	g.Expect(machine.Name).To(HavePrefix(kcp.Name))
	g.Expect(machine.Namespace).To(Equal(kcp.Namespace))
	g.Expect(machine.Labels).To(Equal(generateKubeadmControlPlaneLabels(cluster.Name)))
	g.Expect(machine.OwnerReferences).To(HaveLen(1))
	g.Expect(machine.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane"))))
	g.Expect(machine.Spec).To(Equal(expectedMachineSpec))
}

func TestKubeadmControlPlaneReconciler_generateKubeadmConfig(t *testing.T) {
	g := NewWithT(t)
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testCluster",
			Namespace: "test",
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testControlPlane",
			Namespace: cluster.Namespace,
		},
	}

	spec := bootstrapv1.KubeadmConfigSpec{}
	expectedReferenceKind := "KubeadmConfig"
	expectedReferenceAPIVersion := bootstrapv1.GroupVersion.String()
	expectedLabels := map[string]string{clusterv1.ClusterLabelName: cluster.Name}
	expectedOwner := metav1.OwnerReference{
		Kind:       "KubeadmControlPlane",
		APIVersion: controlplanev1.GroupVersion.String(),
		Name:       kcp.Name,
	}

	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		Log:    log.Log,
	}

	got, err := r.generateKubeadmConfig(context.Background(), kcp, cluster, spec.DeepCopy())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).NotTo(BeNil())
	g.Expect(got.Name).To(HavePrefix(kcp.Name))
	g.Expect(got.Namespace).To(Equal(kcp.Namespace))
	g.Expect(got.Kind).To(Equal(expectedReferenceKind))
	g.Expect(got.APIVersion).To(Equal(expectedReferenceAPIVersion))

	bootstrapConfig := &bootstrapv1.KubeadmConfig{}
	key := client.ObjectKey{Name: got.Name, Namespace: got.Namespace}
	g.Expect(fakeClient.Get(context.Background(), key, bootstrapConfig)).To(Succeed())
	g.Expect(bootstrapConfig.Labels).To(Equal(expectedLabels))
	g.Expect(bootstrapConfig.OwnerReferences).To(HaveLen(1))
	g.Expect(bootstrapConfig.OwnerReferences).To(ContainElement(expectedOwner))
	g.Expect(bootstrapConfig.Spec).To(Equal(spec))
}

func Test_getMachineNodeNoNodeRef(t *testing.T) {
	g := NewWithT(t)

	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme)

	m := &clusterv1.Machine{}
	node, err := getMachineNode(context.Background(), fakeClient, m)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(node).To(BeNil())
}

func Test_getMachineNodeNotFound(t *testing.T) {
	g := NewWithT(t)

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
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(node).To(BeNil())
}

func Test_getMachineNodeFound(t *testing.T) {
	g := NewWithT(t)

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
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(node).To(Equal(node))
}

func TestKubeadmControlPlaneReconciler_updateStatusNoMachines(t *testing.T) {
	g := NewWithT(t)

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
	g.Expect(kcp.ValidateCreate()).To(Succeed())

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, kcp, cluster)
	log.SetLogger(klogr.New())

	r := &KubeadmControlPlaneReconciler{
		Client:             fakeClient,
		Log:                log.Log,
		remoteClientGetter: fakeremote.NewClusterClient,
		scheme:             scheme.Scheme,
		managementCluster:  &internal.ManagementCluster{Client: fakeClient},
	}

	g.Expect(r.updateStatus(context.Background(), kcp, cluster)).To(Succeed())
	g.Expect(kcp.Status.Replicas).To(BeEquivalentTo(0))
	g.Expect(kcp.Status.ReadyReplicas).To(BeEquivalentTo(0))
	g.Expect(kcp.Status.UnavailableReplicas).To(BeEquivalentTo(0))
	g.Expect(kcp.Status.Initialized).To(BeFalse())
	g.Expect(kcp.Status.Ready).To(BeFalse())
	g.Expect(kcp.Status.Selector).NotTo(BeEmpty())
	g.Expect(kcp.Status.FailureMessage).To(BeNil())
	g.Expect(kcp.Status.FailureReason).To(BeEquivalentTo(""))
}

func createNodeAndNodeRefForMachine(machine *clusterv1.Machine, ready bool) (*clusterv1.Machine, *corev1.Node) {
	machine.Status = clusterv1.MachineStatus{
		NodeRef: &corev1.ObjectReference{
			Kind:       "Node",
			APIVersion: corev1.SchemeGroupVersion.String(),
			Name:       machine.GetName(),
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: machine.GetName(),
		},
	}

	if ready {
		node.Spec.ProviderID = fmt.Sprintf("test://%s", machine.GetName())
		node.Status.Conditions = []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			},
		}
	}

	return machine, node
}

func generateKubeadmControlPlaneLabels(name string) map[string]string {
	return map[string]string{
		clusterv1.ClusterLabelName:             name,
		clusterv1.MachineControlPlaneLabelName: "",
	}
}

func createMachineNodePair(name string, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane, ready bool) (*clusterv1.Machine, *corev1.Node) {
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      name,
			Labels:    generateKubeadmControlPlaneLabels(cluster.Name),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane")),
			},
		},
	}

	return createNodeAndNodeRefForMachine(machine, ready)
}

func TestKubeadmControlPlaneReconciler_updateStatusAllMachinesNotReady(t *testing.T) {
	g := NewWithT(t)

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
	g.Expect(kcp.ValidateCreate()).To(Succeed())

	objs := []runtime.Object{cluster.DeepCopy(), kcp.DeepCopy()}
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("test-%d", i)
		m, n := createMachineNodePair(name, cluster, kcp, false)
		objs = append(objs, m, n)
	}

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, objs...)
	log.SetLogger(klogr.New())

	r := &KubeadmControlPlaneReconciler{
		Client:             fakeClient,
		Log:                log.Log,
		remoteClientGetter: fakeremote.NewClusterClient,
		scheme:             scheme.Scheme,
	}

	g.Expect(r.updateStatus(context.Background(), kcp, cluster)).To(Succeed())
	g.Expect(kcp.Status.Replicas).To(BeEquivalentTo(3))
	g.Expect(kcp.Status.ReadyReplicas).To(BeEquivalentTo(0))
	g.Expect(kcp.Status.UnavailableReplicas).To(BeEquivalentTo(3))
	g.Expect(kcp.Status.Selector).NotTo(BeEmpty())
	g.Expect(kcp.Status.FailureMessage).To(BeNil())
	g.Expect(kcp.Status.FailureReason).To(BeEquivalentTo(""))
	g.Expect(kcp.Status.Initialized).To(BeFalse())
	g.Expect(kcp.Status.Ready).To(BeFalse())
}

func TestKubeadmControlPlaneReconciler_updateStatusAllMachinesReady(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Name:      "foo",
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      "foo",
		},
	}
	kcp.Default()
	g.Expect(kcp.ValidateCreate()).To(Succeed())

	objs := []runtime.Object{cluster.DeepCopy(), kcp.DeepCopy()}
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("test-%d", i)
		m, n := createMachineNodePair(name, cluster, kcp, true)
		objs = append(objs, m, n)
	}

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, objs...)
	log.SetLogger(klogr.New())

	r := &KubeadmControlPlaneReconciler{
		Client:             fakeClient,
		Log:                log.Log,
		remoteClientGetter: fakeremote.NewClusterClient,
		scheme:             scheme.Scheme,
	}

	g.Expect(r.updateStatus(context.Background(), kcp, cluster)).To(Succeed())
	g.Expect(kcp.Status.Replicas).To(BeEquivalentTo(3))
	g.Expect(kcp.Status.ReadyReplicas).To(BeEquivalentTo(3))
	g.Expect(kcp.Status.UnavailableReplicas).To(BeEquivalentTo(0))
	g.Expect(kcp.Status.Selector).NotTo(BeEmpty())
	g.Expect(kcp.Status.FailureMessage).To(BeNil())
	g.Expect(kcp.Status.FailureReason).To(BeEquivalentTo(""))
	g.Expect(kcp.Status.Initialized).To(BeTrue())

	// TODO: will need to be updated once we start handling Ready
	g.Expect(kcp.Status.Ready).To(BeFalse())
}

func TestKubeadmControlPlaneReconciler_updateStatusMachinesReadyMixed(t *testing.T) {
	g := NewWithT(t)

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
	g.Expect(kcp.ValidateCreate()).To(Succeed())

	objs := []runtime.Object{cluster.DeepCopy(), kcp.DeepCopy()}
	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("test-%d", i)
		m, n := createMachineNodePair(name, cluster, kcp, false)
		objs = append(objs, m, n)
	}
	m, n := createMachineNodePair("testReady", cluster, kcp, true)
	objs = append(objs, m, n)

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(Succeed())
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme, objs...)
	log.SetLogger(klogr.New())

	r := &KubeadmControlPlaneReconciler{
		Client:             fakeClient,
		Log:                log.Log,
		remoteClientGetter: fakeremote.NewClusterClient,
		scheme:             scheme.Scheme,
	}

	g.Expect(r.updateStatus(context.Background(), kcp, cluster)).To(Succeed())
	g.Expect(kcp.Status.Replicas).To(BeEquivalentTo(5))
	g.Expect(kcp.Status.ReadyReplicas).To(BeEquivalentTo(1))
	g.Expect(kcp.Status.UnavailableReplicas).To(BeEquivalentTo(4))
	g.Expect(kcp.Status.Selector).NotTo(BeEmpty())
	g.Expect(kcp.Status.FailureMessage).To(BeNil())
	g.Expect(kcp.Status.FailureReason).To(BeEquivalentTo(""))
	g.Expect(kcp.Status.Initialized).To(BeTrue())

	// TODO: will need to be updated once we start handling Ready
	g.Expect(kcp.Status.Ready).To(BeFalse())
}

func TestReconcileControlPlaneScaleUp(t *testing.T) {
	g := NewWithT(t)

	name := "foo"
	namespace := "test"
	desiredReplicas := 3

	cluster, genericMachineTemplate, kcp, err := createKubeadmControlPlaneResources(name, namespace, desiredReplicas)
	g.Expect(err).To(BeNil())

	fakeClient, err := createFakeClient()
	g.Expect(err).To(BeNil())

	g.Expect(fakeClient.Create(context.Background(), cluster.DeepCopy())).To(Succeed())
	g.Expect(fakeClient.Create(context.Background(), genericMachineTemplate.DeepCopy())).To(Succeed())
	g.Expect(fakeClient.Create(context.Background(), kcp.DeepCopy())).To(Succeed())

	fakeEtcdClient := fakeetcd.NewClient()

	r := &KubeadmControlPlaneReconciler{
		Client:                 fakeClient,
		Log:                    log.Log,
		recorder:               record.NewFakeRecorder(32),
		scheme:                 scheme.Scheme,
		remoteClientGetter:     fakeremote.NewClusterClient,
	}

	for i := 1; i <= desiredReplicas; i++ {
		// Create control plane machine
		_, err := r.reconcile(context.Background(), cluster, kcp, r.Log)
		g.Expect(err).To(BeNil())
		g.Expect(r.updateStatus(context.Background(), kcp, cluster)).To(Succeed())

		// Find the created machine
		ownedMachines := &clusterv1.MachineList{}
		var newMachine *clusterv1.Machine
		g.Expect(fakeClient.List(context.Background(), ownedMachines)).To(Succeed())
		g.Expect(ownedMachines.Items).To(HaveLen(i))
		for i := range ownedMachines.Items {
			if ownedMachines.Items[i].Status.NodeRef == nil {
				newMachine = &ownedMachines.Items[i]
			}
		}

		// Create Node and NodeRef for the new machine
		newMachine, node := createNodeAndNodeRefForMachine(newMachine, true)
		g.Expect(fakeClient.Create(context.Background(), node)).To(Succeed())
		g.Expect(fakeClient.Status().Update(context.Background(), newMachine)).To(Succeed())

		// Update KubeadmControlPlane Status to reflect the new Node and NodeRef
		g.Expect(r.updateStatus(context.Background(), kcp, cluster)).To(Succeed())

		// Add and start etcd member for the first control plane replica
		g.Expect(fakeEtcdClient.AddMember(uint64(i), []string{fmt.Sprintf("https://%s:2379", newMachine.Name)})).To(Succeed())
		g.Expect(fakeEtcdClient.StartMember(uint64(i), newMachine.Name, []string{fmt.Sprintf("https://%s:2380", newMachine.Name)})).To(Succeed())
	}

	// Verify that controller created a new machine
	ownedMachines := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), ownedMachines)).To(Succeed())
	g.Expect(ownedMachines.Items).To(HaveLen(desiredReplicas))
	for _, m := range ownedMachines.Items {
		g.Expect(m.Spec.Bootstrap.ConfigRef.Name).To(HavePrefix(kcp.Name))
	}
}

func createKubeadmControlPlaneResources(name, namespace string, replicas int) (*clusterv1.Cluster, *unstructured.Unstructured, *controlplanev1.KubeadmControlPlane, error) {
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneEndpoint: clusterv1.APIEndpoint{
				Host: "test.local",
				Port: 9999,
			},
		},
		Status: clusterv1.ClusterStatus{
			InfrastructureReady: true,
		},
	}

	genericMachineTemplate := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericMachineTemplate",
			"apiVersion": "generic.io/v1",
			"metadata": map[string]interface{}{
				"name":      fmt.Sprintf("infra-%s", name),
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
			Name:      fmt.Sprintf("kcp-%s", name),
			Namespace: cluster.Namespace,
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Replicas: pointer.Int32Ptr(int32(replicas)),
			InfrastructureTemplate: corev1.ObjectReference{
				Kind:       genericMachineTemplate.GetKind(),
				Namespace:  genericMachineTemplate.GetNamespace(),
				Name:       genericMachineTemplate.GetName(),
				APIVersion: genericMachineTemplate.GetAPIVersion(),
			},
			KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: &kubeadmv1.ClusterConfiguration{},
				InitConfiguration:    &kubeadmv1.InitConfiguration{},
				JoinConfiguration:    &kubeadmv1.JoinConfiguration{},
			},
		},
	}
	kcp.Default()
	err := kcp.ValidateCreate()

	return cluster, genericMachineTemplate, kcp, err
}

func createFakeClient() (client.Client, error) {
	if err := clusterv1.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}
	if err := bootstrapv1.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}
	if err := controlplanev1.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}

	fakeClient := fake.NewFakeClientWithScheme(
		scheme.Scheme,
	)

	return fakeClient, nil
}

func TestScaleUpControlPlaneAddsANewMachine(t *testing.T) {
	g := NewWithT(t)

	name := "foo"
	namespace := "test"
	cluster, genericMachineTemplate, kcp, err := createKubeadmControlPlaneResources(name, namespace, 1)
	g.Expect(err).To(BeNil())

	fakeClient, err := createFakeClient()
	g.Expect(err).To(BeNil())

	g.Expect(fakeClient.Create(context.Background(), cluster.DeepCopy())).To(Succeed())
	g.Expect(fakeClient.Create(context.Background(), genericMachineTemplate.DeepCopy())).To(Succeed())
	g.Expect(fakeClient.Create(context.Background(), kcp.DeepCopy())).To(Succeed())

	fakeEtcdClient := fakeetcd.NewClient()

	r := &KubeadmControlPlaneReconciler{
		Client:                 fakeClient,
		Log:                    log.Log,
		recorder:               record.NewFakeRecorder(32),
		scheme:                 scheme.Scheme,
		remoteClientGetter:     fakeremote.NewClusterClient,
	}

	// Create the first control plane replica
	g.Expect(r.initializeControlPlane(context.Background(), cluster, kcp)).To(Succeed())
	ownedMachines := &clusterv1.MachineList{}

	// Create Node and NodeRef for the first control plane replica
	g.Expect(fakeClient.List(context.Background(), ownedMachines)).To(Succeed())
	g.Expect(ownedMachines.Items).To(HaveLen(1))
	initialMachine := &ownedMachines.Items[0]
	initialMachine, node := createNodeAndNodeRefForMachine(initialMachine, true)
	g.Expect(fakeClient.Create(context.Background(), node)).To(Succeed())
	g.Expect(fakeClient.Status().Update(context.Background(), initialMachine)).To(Succeed())

	// Add and start etcd member for the first control plane replica
	g.Expect(fakeEtcdClient.AddMember(1, []string{fmt.Sprintf("https://%s:2379", initialMachine.Name)})).To(Succeed())
	g.Expect(fakeEtcdClient.StartMember(1, initialMachine.Name, []string{fmt.Sprintf("https://%s:2380", initialMachine.Name)})).To(Succeed())

	// Tell the controller to scale up
	g.Expect(fakeClient.List(context.Background(), ownedMachines)).To(Succeed())
	g.Expect(r.scaleUpControlPlane(context.Background(), cluster, kcp)).To(Succeed())

	// Verify that controller created a new machine
	g.Expect(fakeClient.List(context.Background(), ownedMachines)).To(Succeed())
	for _, m := range ownedMachines.Items {
		g.Expect(m.Spec.Bootstrap.ConfigRef.Name).To(HavePrefix(kcp.Name))
	}
}

func TestCloneConfigsAndGenerateMachine(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "test",
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
			Name:      "kcp-foo",
			Namespace: cluster.Namespace,
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			InfrastructureTemplate: corev1.ObjectReference{
				Kind:       genericMachineTemplate.GetKind(),
				APIVersion: genericMachineTemplate.GetAPIVersion(),
				Name:       genericMachineTemplate.GetName(),
				Namespace:  cluster.Namespace,
			},
		},
	}

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme.Scheme)).To(Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme.Scheme)).To(Succeed())
	fakeClient := fake.NewFakeClientWithScheme(
		scheme.Scheme,
		cluster.DeepCopy(),
		kcp.DeepCopy(),
		genericMachineTemplate.DeepCopy(),
	)

	r := &KubeadmControlPlaneReconciler{
		Client:   fakeClient,
		Log:      log.Log,
		recorder: record.NewFakeRecorder(32),
		scheme:   scheme.Scheme,
	}

	bootstrapSpec := &bootstrapv1.KubeadmConfigSpec{
		JoinConfiguration: &kubeadmv1.JoinConfiguration{},
	}
	g.Expect(r.cloneConfigsAndGenerateMachine(context.Background(), cluster, kcp, bootstrapSpec)).To(Succeed())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(machineList.Items).To(HaveLen(1))

	for _, m := range machineList.Items {
		g.Expect(m.Namespace).To(Equal(cluster.Namespace))
		g.Expect(m.Name).NotTo(BeEmpty())
		g.Expect(m.Name).To(HavePrefix(kcp.Name))

		g.Expect(m.Spec.InfrastructureRef.Namespace).To(Equal(cluster.Namespace))
		g.Expect(m.Spec.InfrastructureRef.Name).To(HavePrefix(genericMachineTemplate.GetName()))
		g.Expect(m.Spec.InfrastructureRef.APIVersion).To(Equal(genericMachineTemplate.GetAPIVersion()))
		g.Expect(m.Spec.InfrastructureRef.Kind).To(Equal("GenericMachine"))

		g.Expect(m.Spec.Bootstrap.ConfigRef.Namespace).To(Equal(cluster.Namespace))
		g.Expect(m.Spec.Bootstrap.ConfigRef.Name).To(HavePrefix(kcp.Name))
		g.Expect(m.Spec.Bootstrap.ConfigRef.APIVersion).To(Equal(bootstrapv1.GroupVersion.String()))
		g.Expect(m.Spec.Bootstrap.ConfigRef.Kind).To(Equal("KubeadmConfig"))
	}
}

func TestReconcileControlPlaneDelete(t *testing.T) {
	t.Run("delete control plane machines", func(t *testing.T) {
		g := NewWithT(t)

		name := "foo"
		namespace := "test"
		replicas := 3

		cluster, genericMachineTemplate, kcp, err := createKubeadmControlPlaneResources(name, namespace, replicas)
		g.Expect(err).To(BeNil())

		controllerutil.AddFinalizer(kcp, controlplanev1.KubeadmControlPlaneFinalizer)

		fakeClient, err := createFakeClient()
		g.Expect(err).To(BeNil())

		g.Expect(fakeClient.Create(context.Background(), cluster.DeepCopy())).To(Succeed())
		g.Expect(fakeClient.Create(context.Background(), genericMachineTemplate.DeepCopy())).To(Succeed())
		g.Expect(fakeClient.Create(context.Background(), kcp.DeepCopy())).To(Succeed())

		log.SetLogger(klogr.New())

		r := &KubeadmControlPlaneReconciler{
			Client:             fakeClient,
			Log:                log.Log,
			remoteClientGetter: fakeremote.NewClusterClient,
			recorder:           record.NewFakeRecorder(32),
			scheme:             scheme.Scheme,
		}

		// Create control plane machines
		for i := 1; i <= replicas; i++ {
			machine, node := createMachineNodePair(fmt.Sprintf("%s-control-plane-machine-%d", name, i), cluster, kcp, true)
			g.Expect(fakeClient.Create(context.Background(), machine.DeepCopy())).To(Succeed())
			g.Expect(fakeClient.Create(context.Background(), node.DeepCopy())).To(Succeed())
		}

		// Delete control plane machines and requeue, but do not remove finalizer
		result, err := r.reconcileDelete(context.Background(), cluster, kcp, r.Log)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(Equal(ctrl.Result{RequeueAfter: DeleteRequeueAfter}))
		g.Expect(r.updateStatus(context.Background(), kcp, cluster)).To(Succeed())

		g.Expect(kcp.Status.Replicas).To(BeEquivalentTo(0))
		g.Expect(kcp.Finalizers).To(Equal([]string{controlplanev1.KubeadmControlPlaneFinalizer}))

		// Remove finalizer
		result, err = r.reconcileDelete(context.Background(), cluster, kcp, r.Log)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result).To(Equal(ctrl.Result{}))

		g.Expect(kcp.Status.Replicas).To(BeEquivalentTo(0))
		g.Expect(kcp.Finalizers).To(Equal([]string{}))
	})

	t.Run("fail to delete control plane machines because at least one machine is not owned by the control plane", func(t *testing.T) {
		g := NewWithT(t)

		name := "foo"
		namespace := "test"
		replicas := 3

		cluster, genericMachineTemplate, kcp, err := createKubeadmControlPlaneResources(name, namespace, replicas)
		g.Expect(err).To(BeNil())

		controllerutil.AddFinalizer(kcp, controlplanev1.KubeadmControlPlaneFinalizer)

		fakeClient, err := createFakeClient()
		g.Expect(err).To(BeNil())

		g.Expect(fakeClient.Create(context.Background(), cluster.DeepCopy())).To(Succeed())
		g.Expect(fakeClient.Create(context.Background(), genericMachineTemplate.DeepCopy())).To(Succeed())
		g.Expect(fakeClient.Create(context.Background(), kcp.DeepCopy())).To(Succeed())

		log.SetLogger(klogr.New())

		r := &KubeadmControlPlaneReconciler{
			Client:             fakeClient,
			Log:                log.Log,
			remoteClientGetter: fakeremote.NewClusterClient,
			recorder:           record.NewFakeRecorder(32),
			scheme:             scheme.Scheme,
		}

		// Create control plane machines
		for i := 1; i <= replicas; i++ {
			machine, node := createMachineNodePair(fmt.Sprintf("%s-control-plane-machine-%d", name, i), cluster, kcp, true)
			g.Expect(fakeClient.Create(context.Background(), machine.DeepCopy())).To(Succeed())
			g.Expect(fakeClient.Create(context.Background(), node.DeepCopy())).To(Succeed())
		}

		// Create worker machine
		workerMachine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-worker-machine", name),
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterLabelName: cluster.Name,
				},
			},
		}
		g.Expect(fakeClient.Create(context.Background(), workerMachine.DeepCopy())).To(Succeed())

		result, err := r.reconcileDelete(context.Background(), cluster, kcp, r.Log)
		g.Expect(err).Should(MatchError(ContainSubstring("at least one machine is not owned by the control plane")))
		g.Expect(result).To(Equal(ctrl.Result{}))
	})
}
