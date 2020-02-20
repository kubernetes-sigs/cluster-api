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
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/klogr"
	utilpointer "k8s.io/utils/pointer"
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
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/hash"
	"sigs.k8s.io/cluster-api/util"
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

	result, err := r.initializeControlPlane(context.Background(), cluster, kcp)
	g.Expect(result).To(Equal(ctrl.Result{Requeue: true}))
	g.Expect(err).NotTo(HaveOccurred())

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
	g.Expect(result).To(Equal(ctrl.Result{Requeue: true}))
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

	k, err := kubeconfig.FromSecret(context.Background(), fakeClient, util.ObjectKey(cluster))
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
		Client:            fakeClient,
		Log:               log.Log,
		managementCluster: &internal.ManagementCluster{Client: fakeClient},
	}
	g.Expect(r.generateMachine(context.Background(), kcp, cluster, infraRef, bootstrapRef)).To(Succeed())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(machineList.Items).NotTo(BeEmpty())
	g.Expect(machineList.Items).To(HaveLen(1))
	machine := machineList.Items[0]
	g.Expect(machine.Name).To(HavePrefix(kcp.Name))
	g.Expect(machine.Namespace).To(Equal(kcp.Namespace))
	g.Expect(machine.Labels).To(Equal(internal.ControlPlaneLabelsForClusterWithHash(cluster.Name, hash.Compute(&kcp.Spec))))
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
	g.Expect(bootstrapConfig.Labels).To(Equal(internal.ControlPlaneLabelsForClusterWithHash(cluster.Name, hash.Compute(&kcp.Spec))))
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

func createMachineNodePair(name string, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane, ready bool) (*clusterv1.Machine, *corev1.Node) {
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      name,
			Labels:    internal.ControlPlaneLabelsForCluster(cluster.Name),
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
		managementCluster:  &internal.ManagementCluster{Client: fakeClient},
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
		managementCluster:  &internal.ManagementCluster{Client: fakeClient},
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
		managementCluster:  &internal.ManagementCluster{Client: fakeClient},
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

func createClusterWithControlPlane() (*clusterv1.Cluster, *controlplanev1.KubeadmControlPlane, *unstructured.Unstructured) {
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

	return cluster, kcp, genericMachineTemplate
}

func fakeClient() (client.Client, error) {
	if err := clusterv1.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}
	if err := bootstrapv1.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}
	if err := controlplanev1.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}
	fakeClient := fake.NewFakeClientWithScheme(scheme.Scheme)
	return fakeClient, nil
}

func TestKubeadmControlPlaneReconciler_reconcileDelete(t *testing.T) {
	t.Run("removes all control plane Machines", func(t *testing.T) {
		g := NewWithT(t)

		fakeClient, err := fakeClient()
		g.Expect(err).NotTo(HaveOccurred())

		cluster, kcp, _ := createClusterWithControlPlane()
		controllerutil.AddFinalizer(kcp, controlplanev1.KubeadmControlPlaneFinalizer)

		for i := 0; i < 3; i++ {
			m, _ := createMachineNodePair(fmt.Sprintf("test-%d", i), cluster, kcp, true)
			g.Expect(fakeClient.Create(context.Background(), m)).To(Succeed())
		}

		r := &KubeadmControlPlaneReconciler{
			Client:            fakeClient,
			managementCluster: &internal.ManagementCluster{Client: fakeClient},
		}

		result, err := r.reconcileDelete(context.Background(), cluster, kcp, log.Log)
		g.Expect(result).To(Equal(ctrl.Result{RequeueAfter: DeleteRequeueAfter}))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(kcp.Finalizers).To(ContainElement(controlplanev1.KubeadmControlPlaneFinalizer))

		controlPlaneMachines := clusterv1.MachineList{}
		g.Expect(fakeClient.List(context.Background(), &controlPlaneMachines)).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(BeEmpty())

		result, err = r.reconcileDelete(context.Background(), cluster, kcp, log.Log)
		g.Expect(result).To(Equal(ctrl.Result{}))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(kcp.Finalizers).To(BeEmpty())
	})

	t.Run("does not remove any control plane Machines if other Machines exist", func(t *testing.T) {
		g := NewWithT(t)

		fakeClient, err := fakeClient()
		g.Expect(err).NotTo(HaveOccurred())

		cluster, kcp, _ := createClusterWithControlPlane()
		controllerutil.AddFinalizer(kcp, controlplanev1.KubeadmControlPlaneFinalizer)

		for i := 0; i < 3; i++ {
			m, _ := createMachineNodePair(fmt.Sprintf("test-%d", i), cluster, kcp, true)
			g.Expect(fakeClient.Create(context.Background(), m)).To(Succeed())
		}

		workerMachine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "worker",
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterLabelName: cluster.Name,
				},
			},
		}
		g.Expect(fakeClient.Create(context.Background(), workerMachine)).To(Succeed())

		r := &KubeadmControlPlaneReconciler{
			Client:            fakeClient,
			managementCluster: &internal.ManagementCluster{Client: fakeClient},
		}

		result, err := r.reconcileDelete(context.Background(), cluster, kcp, log.Log)
		g.Expect(result).To(Equal(ctrl.Result{Requeue: true}))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(kcp.Finalizers).To(ContainElement(controlplanev1.KubeadmControlPlaneFinalizer))

		controlPlaneMachines := clusterv1.MachineList{}
		labels := map[string]string{
			clusterv1.MachineControlPlaneLabelName: "",
		}
		g.Expect(fakeClient.List(context.Background(), &controlPlaneMachines, client.MatchingLabels(labels))).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(HaveLen(3))
	})

	t.Run("removes the finalizer if no control plane Machines exist", func(t *testing.T) {
		g := NewWithT(t)

		fakeClient, err := fakeClient()
		g.Expect(err).NotTo(HaveOccurred())

		cluster, kcp, _ := createClusterWithControlPlane()
		controllerutil.AddFinalizer(kcp, controlplanev1.KubeadmControlPlaneFinalizer)

		r := &KubeadmControlPlaneReconciler{
			Client:            fakeClient,
			managementCluster: &internal.ManagementCluster{Client: fakeClient},
		}

		result, err := r.reconcileDelete(context.Background(), cluster, kcp, log.Log)
		g.Expect(result).To(Equal(ctrl.Result{}))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(kcp.Finalizers).To(BeEmpty())
	})

}

type fakeManagementCluster struct {
	ControlPlaneHealthy bool
	EtcdHealthy         bool
	Machines            internal.FilterableMachineCollection
}

func (f *fakeManagementCluster) GetMachinesForCluster(ctx context.Context, cluster types.NamespacedName, filters ...internal.MachineFilter) (internal.FilterableMachineCollection, error) {
	return f.Machines, nil
}

func (f *fakeManagementCluster) TargetClusterControlPlaneIsHealthy(ctx context.Context, clusterKey types.NamespacedName, controlPlaneName string) error {
	if !f.ControlPlaneHealthy {
		return errors.New("control plane is not healthy")
	}
	return nil
}

func (f *fakeManagementCluster) TargetClusterEtcdIsHealthy(ctx context.Context, clusterKey types.NamespacedName, controlPlaneName string) error {
	if !f.EtcdHealthy {
		return errors.New("etcd is not healthy")
	}
	return nil
}

func TestKubeadmControlPlaneReconciler_scaleUpControlPlane(t *testing.T) {
	t.Run("creates a control plane Machine if health checks pass", func(t *testing.T) {
		g := NewWithT(t)

		fakeClient, err := fakeClient()
		g.Expect(err).NotTo(HaveOccurred())

		cluster, kcp, genericMachineTemplate := createClusterWithControlPlane()
		g.Expect(fakeClient.Create(context.Background(), genericMachineTemplate)).To(Succeed())

		fmc := &fakeManagementCluster{
			Machines:            internal.NewFilterableMachineCollection(),
			ControlPlaneHealthy: true,
			EtcdHealthy:         true,
		}

		for i := 0; i < 2; i++ {
			m, _ := createMachineNodePair(fmt.Sprintf("test-%d", i), cluster, kcp, true)
			g.Expect(fakeClient.Create(context.Background(), m)).To(Succeed())
			fmc.Machines.Insert(m.DeepCopy())
		}

		r := &KubeadmControlPlaneReconciler{
			Client:            fakeClient,
			managementCluster: fmc,
		}

		result, err := r.scaleUpControlPlane(context.Background(), cluster, kcp)
		g.Expect(result).To(Equal(ctrl.Result{Requeue: true}))
		g.Expect(err).ToNot(HaveOccurred())

		controlPlaneMachines := clusterv1.MachineList{}
		g.Expect(fakeClient.List(context.Background(), &controlPlaneMachines)).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(HaveLen(3))
	})
	t.Run("does not create a control plane Machine if any health check fails", func(t *testing.T) {
		g := NewWithT(t)

		fmc := &fakeManagementCluster{}

		r := &KubeadmControlPlaneReconciler{
			managementCluster: fmc,
		}

		fmc.ControlPlaneHealthy = true
		fmc.EtcdHealthy = false
		result, err := r.scaleUpControlPlane(context.Background(), &clusterv1.Cluster{}, &controlplanev1.KubeadmControlPlane{})
		g.Expect(result).To(Equal(ctrl.Result{RequeueAfter: HealthCheckFailedRequeueAfter}))
		g.Expect(err).To(HaveOccurred())

		fmc.ControlPlaneHealthy = false
		fmc.EtcdHealthy = true
		result, err = r.scaleUpControlPlane(context.Background(), &clusterv1.Cluster{}, &controlplanev1.KubeadmControlPlane{})
		g.Expect(result).To(Equal(ctrl.Result{RequeueAfter: HealthCheckFailedRequeueAfter}))
		g.Expect(err).To(HaveOccurred())

	})
}

func TestKubeadmControlPlaneReconciler_scaleDownControlPlane(t *testing.T) {
	t.Run("deletes a control plane Machine if health checks pass", func(t *testing.T) {
		g := NewWithT(t)

		fakeClient, err := fakeClient()
		g.Expect(err).NotTo(HaveOccurred())

		cluster, kcp, genericMachineTemplate := createClusterWithControlPlane()
		g.Expect(fakeClient.Create(context.Background(), genericMachineTemplate)).To(Succeed())

		fmc := &fakeManagementCluster{
			Machines:            internal.NewFilterableMachineCollection(),
			ControlPlaneHealthy: true,
			EtcdHealthy:         true,
		}

		for i := 0; i < 2; i++ {
			m, _ := createMachineNodePair(fmt.Sprintf("test-%d", i), cluster, kcp, true)
			g.Expect(fakeClient.Create(context.Background(), m)).To(Succeed())
			fmc.Machines.Insert(m.DeepCopy())
		}

		r := &KubeadmControlPlaneReconciler{
			Client:            fakeClient,
			managementCluster: fmc,
		}

		fmc.ControlPlaneHealthy = true
		fmc.EtcdHealthy = true
		result, err := r.scaleDownControlPlane(context.Background(), &clusterv1.Cluster{}, &controlplanev1.KubeadmControlPlane{})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(Equal(ctrl.Result{Requeue: true}))

		controlPlaneMachines := clusterv1.MachineList{}
		g.Expect(fakeClient.List(context.Background(), &controlPlaneMachines)).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(HaveLen(1))
	})
	t.Run("does not delete a control plane Machine if health checks fail", func(t *testing.T) {
		g := NewWithT(t)

		fakeClient, err := fakeClient()
		g.Expect(err).NotTo(HaveOccurred())

		cluster, kcp, genericMachineTemplate := createClusterWithControlPlane()
		g.Expect(fakeClient.Create(context.Background(), genericMachineTemplate)).To(Succeed())

		fmc := &fakeManagementCluster{
			Machines:            internal.NewFilterableMachineCollection(),
			ControlPlaneHealthy: true,
			EtcdHealthy:         true,
		}

		for i := 0; i < 2; i++ {
			m, _ := createMachineNodePair(fmt.Sprintf("test-%d", i), cluster, kcp, true)
			g.Expect(fakeClient.Create(context.Background(), m)).To(Succeed())
			fmc.Machines.Insert(m.DeepCopy())
		}

		r := &KubeadmControlPlaneReconciler{
			Client:            fakeClient,
			managementCluster: fmc,
		}

		controlPlaneMachines := clusterv1.MachineList{}

		fmc.ControlPlaneHealthy = false
		fmc.EtcdHealthy = true
		result, err := r.scaleDownControlPlane(context.Background(), &clusterv1.Cluster{}, &controlplanev1.KubeadmControlPlane{})
		g.Expect(err).To(HaveOccurred())
		g.Expect(result).To(Equal(ctrl.Result{RequeueAfter: HealthCheckFailedRequeueAfter}))
		g.Expect(fakeClient.List(context.Background(), &controlPlaneMachines)).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(HaveLen(2))

		fmc.ControlPlaneHealthy = true
		fmc.EtcdHealthy = false
		result, err = r.scaleDownControlPlane(context.Background(), &clusterv1.Cluster{}, &controlplanev1.KubeadmControlPlane{})
		g.Expect(err).To(HaveOccurred())
		g.Expect(result).To(Equal(ctrl.Result{RequeueAfter: HealthCheckFailedRequeueAfter}))
		g.Expect(fakeClient.List(context.Background(), &controlPlaneMachines)).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(HaveLen(2))
	})
}
