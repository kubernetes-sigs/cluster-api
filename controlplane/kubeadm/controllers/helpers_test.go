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
	"encoding/json"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	utilpointer "k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	kubeadmv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/secret"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestReconcileKubeconfigEmptyAPIEndpoints(t *testing.T) {
	g := NewWithT(t)

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "test",
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.16.6",
		},
	}
	clusterName := client.ObjectKey{Namespace: "test", Name: "foo"}

	fakeClient := newFakeClient(g, kcp.DeepCopy())
	r := &KubeadmControlPlaneReconciler{
		Client:   fakeClient,
		Log:      log.Log,
		recorder: record.NewFakeRecorder(32),
	}

	g.Expect(r.reconcileKubeconfig(context.Background(), clusterName, clusterv1.APIEndpoint{}, kcp)).To(Succeed())

	kubeconfigSecret := &corev1.Secret{}
	secretName := client.ObjectKey{
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
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.16.6",
		},
	}
	clusterName := client.ObjectKey{Namespace: "test", Name: "foo"}
	endpoint := clusterv1.APIEndpoint{Host: "test.local", Port: 8443}

	fakeClient := newFakeClient(g, kcp.DeepCopy())
	r := &KubeadmControlPlaneReconciler{
		Client:   fakeClient,
		Log:      log.Log,
		recorder: record.NewFakeRecorder(32),
	}

	g.Expect(r.reconcileKubeconfig(context.Background(), clusterName, endpoint, kcp)).NotTo(Succeed())

	kubeconfigSecret := &corev1.Secret{}
	secretName := client.ObjectKey{
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
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.16.6",
		},
	}
	clusterName := util.ObjectKey(cluster)
	endpoint := clusterv1.APIEndpoint{Host: "test.local", Port: 8443}

	existingKubeconfigSecret := kubeconfig.GenerateSecretWithOwner(
		client.ObjectKey{Name: "foo", Namespace: "test"},
		[]byte{},
		*metav1.NewControllerRef(cluster, clusterv1.GroupVersion.WithKind("Cluster")),
	)

	fakeClient := newFakeClient(g, kcp.DeepCopy(), existingKubeconfigSecret.DeepCopy())
	r := &KubeadmControlPlaneReconciler{
		Client:   fakeClient,
		Log:      log.Log,
		recorder: record.NewFakeRecorder(32),
	}

	g.Expect(r.reconcileKubeconfig(context.Background(), clusterName, endpoint, kcp)).To(Succeed())

	kubeconfigSecret := &corev1.Secret{}
	secretName := client.ObjectKey{
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
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.16.6",
		},
	}
	clusterName := util.ObjectKey(cluster)
	endpoint := clusterv1.APIEndpoint{Host: "test.local", Port: 8443}

	clusterCerts := secret.NewCertificatesForInitialControlPlane(&kubeadmv1.ClusterConfiguration{})
	g.Expect(clusterCerts.Generate()).To(Succeed())
	caCert := clusterCerts.GetByPurpose(secret.ClusterCA)
	existingCACertSecret := caCert.AsSecret(
		client.ObjectKey{Namespace: "test", Name: "foo"},
		*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane")),
	)

	fakeClient := newFakeClient(g, kcp.DeepCopy(), existingCACertSecret.DeepCopy())
	r := &KubeadmControlPlaneReconciler{
		Client:   fakeClient,
		Log:      log.Log,
		recorder: record.NewFakeRecorder(32),
	}
	g.Expect(r.reconcileKubeconfig(context.Background(), clusterName, endpoint, kcp)).To(Succeed())

	kubeconfigSecret := &corev1.Secret{}
	secretName := client.ObjectKey{
		Namespace: "test",
		Name:      secret.Name(clusterName.Name, secret.Kubeconfig),
	}
	g.Expect(r.Client.Get(context.Background(), secretName, kubeconfigSecret)).To(Succeed())
	g.Expect(kubeconfigSecret.OwnerReferences).NotTo(BeEmpty())
	g.Expect(kubeconfigSecret.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane"))))
	g.Expect(kubeconfigSecret.Labels).To(HaveKeyWithValue(clusterv1.ClusterLabelName, clusterName.Name))
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

func TestMachinesNeedingUpgrade(t *testing.T) {
	g := NewWithT(t)

	namespace := "default"
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: namespace,
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

	genericMachineWithoutAnnotation := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericMachine",
			"apiVersion": "generic.io/v1",
			"metadata": map[string]interface{}{
				"name":      "infra-no-annotation",
				"namespace": cluster.Namespace,
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{},
				},
			},
		},
	}
	infraRefWithoutAnnotation := corev1.ObjectReference{
		Kind:       genericMachineWithoutAnnotation.GetKind(),
		Namespace:  genericMachineWithoutAnnotation.GetNamespace(),
		Name:       genericMachineWithoutAnnotation.GetName(),
		APIVersion: genericMachineWithoutAnnotation.GetAPIVersion(),
	}

	genericMachineWithAnnotation := genericMachineWithoutAnnotation.DeepCopy()
	genericMachineWithAnnotation.SetName("infra-with-annotation")
	genericMachineWithAnnotation.SetAnnotations(map[string]string{clusterv1.TemplateClonedFromNameAnnotation: genericMachineTemplate.GetName(),
		clusterv1.TemplateClonedFromGroupKindAnnotation: genericMachineTemplate.GroupVersionKind().GroupKind().String()})

	infraRefWithAnnotation := corev1.ObjectReference{
		Kind:       genericMachineWithAnnotation.GetKind(),
		Namespace:  genericMachineWithAnnotation.GetNamespace(),
		Name:       genericMachineWithAnnotation.GetName(),
		APIVersion: genericMachineWithAnnotation.GetAPIVersion(),
	}

	clusterConfig, initConfig, joinConfig := createConfigs(cluster.Name)

	initKubeadmConfigMapName := "init"
	joinKubeadmConfigMapName := "join"

	initKubeadmConfig := &bootstrapv1.KubeadmConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmConfig",
			APIVersion: bootstrapv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      initKubeadmConfigMapName,
		},
		Spec: bootstrapv1.KubeadmConfigSpec{
			ClusterConfiguration: &clusterConfig,
			InitConfiguration:    &initConfig,
		},
	}

	joinKubeadmConfig := &bootstrapv1.KubeadmConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmConfig",
			APIVersion: bootstrapv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      joinKubeadmConfigMapName,
		},
		Spec: bootstrapv1.KubeadmConfigSpec{
			JoinConfiguration: &joinConfig,
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kcp-foo",
			Namespace: cluster.Namespace,
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.18.4",
			InfrastructureTemplate: corev1.ObjectReference{
				Kind:       genericMachineTemplate.GetKind(),
				APIVersion: genericMachineTemplate.GetAPIVersion(),
				Name:       genericMachineTemplate.GetName(),
				Namespace:  cluster.Namespace,
			},
			KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: &clusterConfig,
				InitConfiguration:    &initConfig,
				JoinConfiguration:    &joinConfig,
			},
		},
	}

	machine := func(name string) *clusterv1.Machine {
		m := &clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					clusterv1.ClusterLabelName: "foo",
				},
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: cluster.Name,
				Version:     utilpointer.StringPtr("v1.18.4"),
			},
		}
		m.CreationTimestamp = metav1.Time{Time: time.Date(1900, 0, 0, 0, 0, 0, 0, time.UTC)}
		return m
	}

	initMachine := machine("init-machine")
	initMachine.Spec.Bootstrap = clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{
		Namespace: namespace,
		Name:      initKubeadmConfigMapName,
	}, DataSecretName: nil}
	initMachine.Spec.InfrastructureRef = infraRefWithAnnotation

	joinMachine := machine("join-machine")
	joinMachine.Spec.Bootstrap = clusterv1.Bootstrap{ConfigRef: &corev1.ObjectReference{
		Namespace: namespace,
		Name:      joinKubeadmConfigMapName,
	}, DataSecretName: nil}
	joinMachine.Spec.InfrastructureRef = infraRefWithAnnotation

	unmatchingJoinMachine := joinMachine.DeepCopy()
	unmatchingJoinConfig := joinConfig.DeepCopy()
	unmatchingJoinConfig.NodeRegistration.Name = "different"
	unmatchingJoinMachine.Name = "join-unmatching"

	versionMismatchMachine := machine("version-mismatch")
	versionMismatchMachine.Spec.Version = utilpointer.StringPtr("v1.19.1")

	noInfraAnnotationMachine := initMachine.DeepCopy()
	noInfraAnnotationMachine.Name = "no-annotation"
	noInfraAnnotationMachine.Spec.InfrastructureRef = infraRefWithoutAnnotation

	deletedMachine := initMachine.DeepCopy()
	deletedMachine.Name = "deleted"
	deletedMachine.DeletionTimestamp = &metav1.Time{Time: time.Date(1900, 0, 0, 0, 0, 0, 0, time.UTC)}

	machineAnnotatedWithCorrectClusterConfig := joinMachine.DeepCopy()
	machineAnnotatedWithCorrectClusterConfig.Name = "annotated-with-cluster-configuration"
	clusterConfigMarshalled, err := json.Marshal(clusterConfig)
	g.Expect(err).NotTo(HaveOccurred())
	machineAnnotatedWithCorrectClusterConfig.SetAnnotations(map[string]string{controlplanev1.KubeadmClusterConfigurationAnnotation: string(clusterConfigMarshalled)})

	machineAnnotatedWithWrongClusterConfig := joinMachine.DeepCopy()
	machineAnnotatedWithWrongClusterConfig.Name = "annotated-with-wrong-cluster-configuration"
	clusterConf := kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.DeepCopy()
	clusterConf.ClusterName = "not-nil"
	clusterConfigMarshalled, err = json.Marshal(clusterConf)
	g.Expect(err).NotTo(HaveOccurred())
	machineAnnotatedWithWrongClusterConfig.SetAnnotations(map[string]string{controlplanev1.KubeadmClusterConfigurationAnnotation: string(clusterConfigMarshalled)})

	kcpInitEmpty := (*kcp).DeepCopy()
	kcpInitEmpty.Spec.KubeadmConfigSpec.InitConfiguration = nil

	kcpRetryJoinSet := (*kcp).DeepCopy()
	kcpRetryJoinSet.Spec.KubeadmConfigSpec.UseExperimentalRetryJoin = true

	kcpUpgradeAfterFuture := (*kcp).DeepCopy()
	kcpUpgradeAfterFuture.Spec.UpgradeAfter = &metav1.Time{Time: time.Date(3000, 0, 0, 0, 0, 0, 0, time.UTC)}

	kcpUpgradeAfterPast := (*kcp).DeepCopy()
	kcpUpgradeAfterPast.Spec.UpgradeAfter = &metav1.Time{Time: time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC)}

	kcpJoinNil := (*kcp).DeepCopy()
	kcpJoinNil.Spec.KubeadmConfigSpec.JoinConfiguration = nil

	objs := []runtime.Object{cluster.DeepCopy(), kcp.DeepCopy(), initKubeadmConfig.DeepCopy(), joinKubeadmConfig.DeepCopy(),
		genericMachineTemplate.DeepCopy(), genericMachineWithAnnotation.DeepCopy(), genericMachineWithoutAnnotation.DeepCopy()}

	fakeClient := newFakeClient(g, objs...)

	r := &KubeadmControlPlaneReconciler{
		Client:   fakeClient,
		Log:      log.Log,
		recorder: record.NewFakeRecorder(32),
		scheme:   scheme.Scheme,
	}

	tests := []struct {
		name     string
		kcp      *controlplanev1.KubeadmControlPlane
		machines []*clusterv1.Machine
		result   internal.FilterableMachineCollection
	}{
		{
			name:     "should not return any machines if KCP upgradeAfter is after machines' creation time",
			kcp:      kcpUpgradeAfterFuture,
			machines: []*clusterv1.Machine{joinMachine, initMachine},
			result:   internal.FilterableMachineCollection{},
		},
		{
			name:     "should return machines if KCP upgradeAfter is before machines' creation time (but not deleted ones)",
			kcp:      kcpUpgradeAfterPast,
			machines: []*clusterv1.Machine{initMachine, joinMachine, deletedMachine},
			result:   internal.FilterableMachineCollection{"init-machine": initMachine, "join-machine": joinMachine},
		},
		{
			name:     "should not return any machines if owned machines are empty",
			kcp:      kcp,
			machines: []*clusterv1.Machine{},
			result:   internal.FilterableMachineCollection{},
		},
		{
			name:     "should return the machine if there is a version mismatch",
			kcp:      kcp,
			machines: []*clusterv1.Machine{versionMismatchMachine, initMachine, joinMachine},
			result:   internal.FilterableMachineCollection{"version-mismatch": versionMismatchMachine},
		},
		{
			name:     "should not return any machines if machine has only init config or join config; and it matches with kcp",
			kcp:      kcp,
			machines: []*clusterv1.Machine{initMachine, joinMachine},
			result:   internal.FilterableMachineCollection{},
		},
		{
			name:     "should return machines that are not matching with KCP KubeadmConfig",
			kcp:      kcpInitEmpty,
			machines: []*clusterv1.Machine{initMachine, joinMachine},
			result:   internal.FilterableMachineCollection{"init-machine": initMachine},
		},
		{
			name:     "should return machines that are not matching with KCP KubeadmConfig when additional fields are set",
			kcp:      kcpRetryJoinSet,
			machines: []*clusterv1.Machine{initMachine, joinMachine},
			result:   internal.FilterableMachineCollection{"init-machine": initMachine, "join-machine": joinMachine},
		},
		{
			name:     "should not return the machine if it is missing infra ref annotation",
			kcp:      kcp,
			machines: []*clusterv1.Machine{noInfraAnnotationMachine},
			result:   internal.FilterableMachineCollection{},
		},
		{
			name:     "should not return the machine if its ClusterConfiguration annotation matches KCP",
			kcp:      kcp,
			machines: []*clusterv1.Machine{machineAnnotatedWithCorrectClusterConfig},
			result:   internal.FilterableMachineCollection{},
		},
		{
			name:     "should return the machine if its ClusterConfiguration annotation does not match KCP",
			kcp:      kcp,
			machines: []*clusterv1.Machine{machineAnnotatedWithWrongClusterConfig},
			result:   internal.FilterableMachineCollection{"annotated-with-wrong-cluster-configuration": machineAnnotatedWithWrongClusterConfig},
		},
		{
			name:     "should not return the machine if KCP JoinConfiguration is nil",
			kcp:      kcpJoinNil,
			machines: []*clusterv1.Machine{initMachine, joinMachine},
			result:   internal.FilterableMachineCollection{},
		},
		{
			name:     "should not return the machine if JoinConfiguration is unmatching due to",
			kcp:      kcpJoinNil,
			machines: []*clusterv1.Machine{initMachine, joinMachine},
			result:   internal.FilterableMachineCollection{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			controlPlane := &internal.ControlPlane{
				KCP:      tt.kcp,
				Cluster:  cluster,
				Machines: internal.NewFilterableMachineCollection(tt.machines...),
			}
			g.Expect(r.machinesNeedingRollout(context.Background(), controlPlane)).To(BeEquivalentTo(tt.result))
		})
	}
}

func createConfigs(clusterName string) (kubeadmv1.ClusterConfiguration, kubeadmv1.InitConfiguration, kubeadmv1.JoinConfiguration) {
	clusterConf := kubeadmv1.ClusterConfiguration{
		APIServer: kubeadmv1.APIServer{
			ControlPlaneComponent: kubeadmv1.ControlPlaneComponent{
				ExtraArgs: map[string]string{"arg": "arg"},
				ExtraVolumes: []kubeadmv1.HostPathMount{{
					HostPath: "/var/log/kubernetes",
					ReadOnly: false,
				}},
			},
			TimeoutForControlPlane: &metav1.Duration{},
		},
		CertificatesDir:      "tmpdir",
		ClusterName:          clusterName,
		ControlPlaneEndpoint: "genericprovider.com:443",
		ControllerManager: kubeadmv1.ControlPlaneComponent{
			ExtraArgs: map[string]string{"controller manager field": "controller manager value"},
		},
		DNS: kubeadmv1.DNS{
			Type:      "CoreDNS",
			ImageMeta: kubeadmv1.ImageMeta{},
		},
		Etcd: kubeadmv1.Etcd{
			Local: &kubeadmv1.LocalEtcd{
				ImageMeta: kubeadmv1.ImageMeta{},
				DataDir:   "/var/lib/etcd",
				ExtraArgs: map[string]string{"arg": "arg"},
			},
		},
		KubernetesVersion: "v1.18.4",
		Networking: kubeadmv1.Networking{
			ServiceSubnet: "10.96.0.0/12",
			PodSubnet:     "192.168.0.0/16",
			DNSDomain:     "",
		},
		Scheduler: kubeadmv1.ControlPlaneComponent{
			ExtraArgs: map[string]string{"arg": "not-nil"},
		},
	}

	initConf := kubeadmv1.InitConfiguration{
		TypeMeta:        metav1.TypeMeta{},
		BootstrapTokens: nil,
		NodeRegistration: kubeadmv1.NodeRegistrationOptions{
			Name:             "{{ ds.meta_data.hostname }}",
			CRISocket:        "",
			Taints:           nil,
			KubeletExtraArgs: map[string]string{"arg": "arg"},
		},
		LocalAPIEndpoint: kubeadmv1.APIEndpoint{
			AdvertiseAddress: "",
			BindPort:         0,
		},
	}

	joinConf := kubeadmv1.JoinConfiguration{
		TypeMeta:         metav1.TypeMeta{},
		NodeRegistration: initConf.NodeRegistration,
		Discovery: kubeadmv1.Discovery{
			BootstrapToken: &kubeadmv1.BootstrapTokenDiscovery{
				APIServerEndpoint:        "genericprovider.com:443",
				CACertHashes:             []string{"str"},
				Token:                    "token",
				UnsafeSkipCAVerification: false,
			},
			File:              nil,
			TLSBootstrapToken: "",
			Timeout:           nil,
		},
		ControlPlane: &kubeadmv1.JoinControlPlane{LocalAPIEndpoint: kubeadmv1.APIEndpoint{
			AdvertiseAddress: "",
			BindPort:         0,
		}},
	}

	return clusterConf, initConf, joinConf
}

// TODO
func TestReconcileExternalReference(t *testing.T) {}

// TODO
func TestCleanupFromGeneration(t *testing.T) {}

// TODO
func TestMarkWithAnnotationKey(t *testing.T) {}
