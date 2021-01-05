/*
Copyright 2020 The Kubernetes Authors.

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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	utilpointer "k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	kubeadmv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/secret"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestReconcileKubeconfig(t *testing.T) {
	cluster := &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "test",
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneEndpoint: clusterv1.APIEndpoint{Host: "test.local", Port: 8443},
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmControlPlane",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "test",
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.16.6",
		},
	}

	t.Run("Empty API Endpoints", func(t *testing.T) {
		g := NewWithT(t)
		fakeClient := newFakeClient(g, kcp.DeepCopy())
		r := &KubeadmControlPlaneReconciler{
			Client:   fakeClient,
			Log:      log.Log,
			recorder: record.NewFakeRecorder(32),
		}

		c := cluster.DeepCopy()
		c.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{}
		g.Expect(r.reconcileKubeconfig(context.Background(), c, kcp)).To(Succeed())

		kubeconfigSecret := &corev1.Secret{}
		secretName := client.ObjectKey{
			Namespace: "test",
			Name:      secret.Name(c.Name, secret.Kubeconfig),
		}
		g.Expect(r.Client.Get(context.Background(), secretName, kubeconfigSecret)).To(MatchError(ContainSubstring("not found")))
	})

	t.Run("Missing CA Certificate", func(t *testing.T) {
		g := NewWithT(t)
		fakeClient := newFakeClient(g, kcp.DeepCopy())
		r := &KubeadmControlPlaneReconciler{
			Client:   fakeClient,
			Log:      log.Log,
			recorder: record.NewFakeRecorder(32),
		}

		g.Expect(r.reconcileKubeconfig(context.Background(), cluster, kcp)).NotTo(Succeed())

		kubeconfigSecret := &corev1.Secret{}
		secretName := client.ObjectKey{
			Namespace: "test",
			Name:      secret.Name(cluster.Name, secret.Kubeconfig),
		}
		g.Expect(r.Client.Get(context.Background(), secretName, kubeconfigSecret)).To(MatchError(ContainSubstring("not found")))
	})

	t.Run("Adopts v1alpha2 Secrets", func(t *testing.T) {
		g := NewWithT(t)
		existingKubeconfigSecret := kubeconfig.GenerateSecretWithOwner(
			client.ObjectKey{Name: "foo", Namespace: "test"},
			[]byte{},
			metav1.OwnerReference{
				APIVersion: clusterv1.GroupVersion.String(),
				Kind:       "Cluster",
				Name:       cluster.Name,
				UID:        cluster.UID,
			}, // the Cluster ownership defines v1alpha2 controlled secrets
		)

		fakeClient := newFakeClient(g, kcp.DeepCopy(), existingKubeconfigSecret.DeepCopy())
		r := &KubeadmControlPlaneReconciler{
			Client:   fakeClient,
			Log:      log.Log,
			recorder: record.NewFakeRecorder(32),
		}

		g.Expect(r.reconcileKubeconfig(context.Background(), cluster, kcp)).To(Succeed())

		kubeconfigSecret := &corev1.Secret{}
		secretName := client.ObjectKey{
			Namespace: "test",
			Name:      secret.Name(cluster.Name, secret.Kubeconfig),
		}
		g.Expect(r.Client.Get(context.Background(), secretName, kubeconfigSecret)).To(Succeed())
		g.Expect(kubeconfigSecret.Labels).To(Equal(existingKubeconfigSecret.Labels))
		g.Expect(kubeconfigSecret.Data).To(Equal(existingKubeconfigSecret.Data))
		g.Expect(kubeconfigSecret.OwnerReferences).ToNot(ContainElement(metav1.OwnerReference{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
			Name:       cluster.Name,
			UID:        cluster.UID,
		}))
		g.Expect(kubeconfigSecret.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane"))))
	})

	t.Run("Does not adopt user secrets", func(t *testing.T) {
		g := NewWithT(t)
		existingKubeconfigSecret := kubeconfig.GenerateSecretWithOwner(
			client.ObjectKey{Name: "foo", Namespace: "test"},
			[]byte{},
			metav1.OwnerReference{},
		)

		fakeClient := newFakeClient(g, kcp.DeepCopy(), existingKubeconfigSecret.DeepCopy())
		r := &KubeadmControlPlaneReconciler{
			Client:   fakeClient,
			Log:      log.Log,
			recorder: record.NewFakeRecorder(32),
		}

		g.Expect(r.reconcileKubeconfig(context.Background(), cluster, kcp)).To(Succeed())

		kubeconfigSecret := &corev1.Secret{}
		secretName := client.ObjectKey{
			Namespace: "test",
			Name:      secret.Name(cluster.Name, secret.Kubeconfig),
		}
		g.Expect(r.Client.Get(context.Background(), secretName, kubeconfigSecret)).To(Succeed())
		g.Expect(kubeconfigSecret.Labels).To(Equal(existingKubeconfigSecret.Labels))
		g.Expect(kubeconfigSecret.Data).To(Equal(existingKubeconfigSecret.Data))
		g.Expect(kubeconfigSecret.OwnerReferences).ToNot(ContainElement(*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane"))))
	})
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
			Version: "v1.16.6",
		},
	}

	fakeClient := newFakeClient(g, cluster.DeepCopy(), kcp.DeepCopy(), genericMachineTemplate.DeepCopy())

	r := &KubeadmControlPlaneReconciler{
		Client:   fakeClient,
		Log:      log.Log,
		recorder: record.NewFakeRecorder(32),
		scheme:   scheme.Scheme,
	}

	bootstrapSpec := &bootstrapv1.KubeadmConfigSpec{
		JoinConfiguration: &kubeadmv1.JoinConfiguration{},
	}
	g.Expect(r.cloneConfigsAndGenerateMachine(context.Background(), cluster, kcp, bootstrapSpec, nil)).To(Succeed())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(machineList.Items).To(HaveLen(1))

	for _, m := range machineList.Items {
		g.Expect(m.Namespace).To(Equal(cluster.Namespace))
		g.Expect(m.Name).NotTo(BeEmpty())
		g.Expect(m.Name).To(HavePrefix(kcp.Name))

		infraObj, err := external.Get(context.TODO(), r.Client, &m.Spec.InfrastructureRef, m.Spec.InfrastructureRef.Namespace)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(infraObj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromNameAnnotation, genericMachineTemplate.GetName()))
		g.Expect(infraObj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromGroupKindAnnotation, genericMachineTemplate.GroupVersionKind().GroupKind().String()))

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

func TestKubeadmControlPlaneReconciler_generateMachine(t *testing.T) {
	g := NewWithT(t)
	fakeClient := newFakeClient(g)

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
			Version: "v1.16.6",
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
		managementCluster: &internal.Management{Client: fakeClient},
		recorder:          record.NewFakeRecorder(32),
	}
	g.Expect(r.generateMachine(context.Background(), kcp, cluster, infraRef, bootstrapRef, nil)).To(Succeed())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(context.Background(), machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(machineList.Items).To(HaveLen(1))
	machine := machineList.Items[0]
	g.Expect(machine.Name).To(HavePrefix(kcp.Name))
	g.Expect(machine.Namespace).To(Equal(kcp.Namespace))
	g.Expect(machine.OwnerReferences).To(HaveLen(1))
	g.Expect(machine.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane"))))
	g.Expect(machine.Spec).To(Equal(expectedMachineSpec))
}

func TestKubeadmControlPlaneReconciler_generateKubeadmConfig(t *testing.T) {
	g := NewWithT(t)
	fakeClient := newFakeClient(g)

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
		Client:   fakeClient,
		Log:      log.Log,
		recorder: record.NewFakeRecorder(32),
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
	g.Expect(bootstrapConfig.OwnerReferences).To(HaveLen(1))
	g.Expect(bootstrapConfig.OwnerReferences).To(ContainElement(expectedOwner))
	g.Expect(bootstrapConfig.Spec).To(Equal(spec))
}

// TODO
func TestReconcileExternalReference(t *testing.T) {}

// TODO
func TestCleanupFromGeneration(t *testing.T) {}

// TODO
func TestMarkWithAnnotationKey(t *testing.T) {}
