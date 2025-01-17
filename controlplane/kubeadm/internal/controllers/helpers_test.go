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
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/secret"
)

func TestReconcileKubeconfigEmptyAPIEndpoints(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneEndpoint: clusterv1.APIEndpoint{},
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmControlPlane",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.16.6",
		},
	}
	clusterName := client.ObjectKey{Namespace: metav1.NamespaceDefault, Name: "foo"}

	fakeClient := newFakeClient(kcp.DeepCopy())
	r := &KubeadmControlPlaneReconciler{
		Client:              fakeClient,
		SecretCachingClient: fakeClient,
		recorder:            record.NewFakeRecorder(32),
	}

	controlPlane := &internal.ControlPlane{
		KCP:     kcp,
		Cluster: cluster,
	}

	result, err := r.reconcileKubeconfig(ctx, controlPlane)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(BeZero())

	kubeconfigSecret := &corev1.Secret{}
	secretName := client.ObjectKey{
		Namespace: metav1.NamespaceDefault,
		Name:      secret.Name(clusterName.Name, secret.Kubeconfig),
	}
	g.Expect(r.Client.Get(ctx, secretName, kubeconfigSecret)).To(MatchError(ContainSubstring("not found")))
}

func TestReconcileKubeconfigMissingCACertificate(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
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
			Namespace: metav1.NamespaceDefault,
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.16.6",
		},
	}

	fakeClient := newFakeClient(kcp.DeepCopy())
	r := &KubeadmControlPlaneReconciler{
		Client:              fakeClient,
		SecretCachingClient: fakeClient,
		recorder:            record.NewFakeRecorder(32),
	}

	controlPlane := &internal.ControlPlane{
		KCP:     kcp,
		Cluster: cluster,
	}

	result, err := r.reconcileKubeconfig(ctx, controlPlane)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(BeComparableTo(ctrl.Result{RequeueAfter: dependentCertRequeueAfter}))

	kubeconfigSecret := &corev1.Secret{}
	secretName := client.ObjectKey{
		Namespace: metav1.NamespaceDefault,
		Name:      secret.Name(cluster.Name, secret.Kubeconfig),
	}
	g.Expect(r.Client.Get(ctx, secretName, kubeconfigSecret)).To(MatchError(ContainSubstring("not found")))
}

func TestReconcileKubeconfigSecretDoesNotAdoptsUserSecrets(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
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
			Namespace: metav1.NamespaceDefault,
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.16.6",
		},
	}

	existingKubeconfigSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name("foo", secret.Kubeconfig),
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "foo",
			},
			OwnerReferences: []metav1.OwnerReference{},
		},
		Data: map[string][]byte{
			secret.KubeconfigDataName: {},
		},
		// KCP identifies CAPI-created Secrets using the clusterv1.ClusterSecretType. Setting any other type allows
		// the controllers to treat it as a user-provided Secret.
		Type: corev1.SecretTypeOpaque,
	}

	fakeClient := newFakeClient(kcp.DeepCopy(), existingKubeconfigSecret.DeepCopy())
	r := &KubeadmControlPlaneReconciler{
		Client:              fakeClient,
		SecretCachingClient: fakeClient,
		recorder:            record.NewFakeRecorder(32),
	}

	controlPlane := &internal.ControlPlane{
		KCP:     kcp,
		Cluster: cluster,
	}

	result, err := r.reconcileKubeconfig(ctx, controlPlane)
	g.Expect(err).To(Succeed())
	g.Expect(result).To(BeZero())

	kubeconfigSecret := &corev1.Secret{}
	secretName := client.ObjectKey{
		Namespace: metav1.NamespaceDefault,
		Name:      secret.Name(cluster.Name, secret.Kubeconfig),
	}
	g.Expect(r.Client.Get(ctx, secretName, kubeconfigSecret)).To(Succeed())
	g.Expect(kubeconfigSecret.Labels).To(Equal(existingKubeconfigSecret.Labels))
	g.Expect(kubeconfigSecret.Data).To(Equal(existingKubeconfigSecret.Data))
	g.Expect(kubeconfigSecret.OwnerReferences).ToNot(ContainElement(*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane"))))
}

func TestKubeadmControlPlaneReconciler_reconcileKubeconfig(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
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
			Namespace: metav1.NamespaceDefault,
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.16.6",
		},
	}

	clusterCerts := secret.NewCertificatesForInitialControlPlane(&bootstrapv1.ClusterConfiguration{})
	g.Expect(clusterCerts.Generate()).To(Succeed())
	caCert := clusterCerts.GetByPurpose(secret.ClusterCA)
	existingCACertSecret := caCert.AsSecret(
		client.ObjectKey{Namespace: metav1.NamespaceDefault, Name: "foo"},
		*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane")),
	)

	fakeClient := newFakeClient(kcp.DeepCopy(), existingCACertSecret.DeepCopy())
	r := &KubeadmControlPlaneReconciler{
		Client:              fakeClient,
		SecretCachingClient: fakeClient,
		recorder:            record.NewFakeRecorder(32),
	}

	controlPlane := &internal.ControlPlane{
		KCP:     kcp,
		Cluster: cluster,
	}

	result, err := r.reconcileKubeconfig(ctx, controlPlane)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(BeComparableTo(ctrl.Result{}))

	kubeconfigSecret := &corev1.Secret{}
	secretName := client.ObjectKey{
		Namespace: metav1.NamespaceDefault,
		Name:      secret.Name(cluster.Name, secret.Kubeconfig),
	}
	g.Expect(r.Client.Get(ctx, secretName, kubeconfigSecret)).To(Succeed())
	g.Expect(kubeconfigSecret.OwnerReferences).NotTo(BeEmpty())
	g.Expect(kubeconfigSecret.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane"))))
	g.Expect(kubeconfigSecret.Labels).To(HaveKeyWithValue(clusterv1.ClusterNameLabel, cluster.Name))
}

func TestCloneConfigsAndGenerateMachine(t *testing.T) {
	setup := func(t *testing.T, g *WithT) *corev1.Namespace {
		t.Helper()

		t.Log("Creating the namespace")
		ns, err := env.CreateNamespace(ctx, "test-applykubeadmconfig")
		g.Expect(err).ToNot(HaveOccurred())

		return ns
	}

	teardown := func(t *testing.T, g *WithT, ns *corev1.Namespace) {
		t.Helper()

		t.Log("Deleting the namespace")
		g.Expect(env.Delete(ctx, ns)).To(Succeed())
	}

	g := NewWithT(t)
	namespace := setup(t, g)
	defer teardown(t, g, namespace)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: namespace.Name,
		},
	}

	genericInfrastructureMachineTemplate := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericInfrastructureMachineTemplate",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
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
	g.Expect(env.CreateAndWait(ctx, genericInfrastructureMachineTemplate)).To(Succeed())

	namingTemplateKey := "-testkcp"
	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kcp-foo",
			Namespace: cluster.Namespace,
			UID:       "abc-123-kcp-control-plane",
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       genericInfrastructureMachineTemplate.GetKind(),
					APIVersion: genericInfrastructureMachineTemplate.GetAPIVersion(),
					Name:       genericInfrastructureMachineTemplate.GetName(),
					Namespace:  cluster.Namespace,
				},
			},
			Version: "v1.16.6",
			MachineNamingStrategy: &controlplanev1.MachineNamingStrategy{
				Template: "{{ .kubeadmControlPlane.name }}" + namingTemplateKey + "-{{ .random }}",
			},
		},
	}

	r := &KubeadmControlPlaneReconciler{
		Client:              env,
		SecretCachingClient: secretCachingClient,
		recorder:            record.NewFakeRecorder(32),
	}

	bootstrapSpec := &bootstrapv1.KubeadmConfigSpec{
		JoinConfiguration: &bootstrapv1.JoinConfiguration{},
	}
	_, err := r.cloneConfigsAndGenerateMachine(ctx, cluster, kcp, bootstrapSpec, nil)
	g.Expect(err).To(Succeed())

	machineList := &clusterv1.MachineList{}
	g.Expect(env.GetAPIReader().List(ctx, machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(machineList.Items).To(HaveLen(1))

	for i := range machineList.Items {
		m := machineList.Items[i]
		g.Expect(m.Namespace).To(Equal(cluster.Namespace))
		g.Expect(m.Name).NotTo(BeEmpty())
		g.Expect(m.Name).To(HavePrefix(kcp.Name + namingTemplateKey))

		infraObj, err := external.Get(ctx, r.Client, &m.Spec.InfrastructureRef)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(infraObj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromNameAnnotation, genericInfrastructureMachineTemplate.GetName()))
		g.Expect(infraObj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromGroupKindAnnotation, genericInfrastructureMachineTemplate.GroupVersionKind().GroupKind().String()))

		g.Expect(m.Spec.InfrastructureRef.Namespace).To(Equal(cluster.Namespace))
		g.Expect(m.Spec.InfrastructureRef.Name).To(Equal(m.Name))
		g.Expect(m.Spec.InfrastructureRef.APIVersion).To(Equal(genericInfrastructureMachineTemplate.GetAPIVersion()))
		g.Expect(m.Spec.InfrastructureRef.Kind).To(Equal("GenericInfrastructureMachine"))

		g.Expect(m.Spec.Bootstrap.ConfigRef.Namespace).To(Equal(cluster.Namespace))
		g.Expect(m.Spec.Bootstrap.ConfigRef.Name).To(Equal(m.Name))
		g.Expect(m.Spec.Bootstrap.ConfigRef.APIVersion).To(Equal(bootstrapv1.GroupVersion.String()))
		g.Expect(m.Spec.Bootstrap.ConfigRef.Kind).To(Equal("KubeadmConfig"))
	}
}

func TestCloneConfigsAndGenerateMachineFail(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
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
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       genericMachineTemplate.GetKind(),
					APIVersion: genericMachineTemplate.GetAPIVersion(),
					Name:       genericMachineTemplate.GetName(),
					Namespace:  cluster.Namespace,
				},
			},
			Version: "v1.16.6",
		},
	}

	fakeClient := newFakeClient(cluster.DeepCopy(), kcp.DeepCopy(), genericMachineTemplate.DeepCopy())

	r := &KubeadmControlPlaneReconciler{
		Client:              fakeClient,
		SecretCachingClient: fakeClient,
		recorder:            record.NewFakeRecorder(32),
	}

	bootstrapSpec := &bootstrapv1.KubeadmConfigSpec{
		JoinConfiguration: &bootstrapv1.JoinConfiguration{},
	}

	// Try to break Infra Cloning
	kcp.Spec.MachineTemplate.InfrastructureRef.Name = "something_invalid"
	_, err := r.cloneConfigsAndGenerateMachine(ctx, cluster, kcp, bootstrapSpec, nil)
	g.Expect(err).To(HaveOccurred())
	g.Expect(&kcp.GetConditions()[0]).Should(conditions.HaveSameStateOf(&clusterv1.Condition{
		Type:     controlplanev1.MachinesCreatedCondition,
		Status:   corev1.ConditionFalse,
		Severity: clusterv1.ConditionSeverityError,
		Reason:   controlplanev1.InfrastructureTemplateCloningFailedReason,
		Message:  "failed to retrieve GenericMachineTemplate default/something_invalid: genericmachinetemplates.generic.io \"something_invalid\" not found",
	}))
}

func TestKubeadmControlPlaneReconciler_computeDesiredMachine(t *testing.T) {
	namingTemplateKey := "-kcp"
	kcpName := "testControlPlane"
	clusterName := "testCluster"

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: metav1.NamespaceDefault,
		},
	}
	duration5s := &metav1.Duration{Duration: 5 * time.Second}
	duration10s := &metav1.Duration{Duration: 10 * time.Second}
	kcpMachineTemplateObjectMeta := clusterv1.ObjectMeta{
		Labels: map[string]string{
			"machineTemplateLabel": "machineTemplateLabelValue",
		},
		Annotations: map[string]string{
			"machineTemplateAnnotation": "machineTemplateAnnotationValue",
		},
	}
	kcpMachineTemplateObjectMetaCopy := kcpMachineTemplateObjectMeta.DeepCopy()

	clusterConfigurationString := "{\"etcd\":{},\"networking\":{},\"apiServer\":{},\"controllerManager\":{},\"scheduler\":{},\"dns\":{},\"clusterName\":\"testCluster\"}"

	infraRef := &corev1.ObjectReference{
		Kind:       "InfraKind",
		APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
		Name:       "infra",
		Namespace:  cluster.Namespace,
	}
	bootstrapRef := &corev1.ObjectReference{
		Kind:       "BootstrapKind",
		APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
		Name:       "bootstrap",
		Namespace:  cluster.Namespace,
	}

	tests := []struct {
		name                      string
		kcp                       *controlplanev1.KubeadmControlPlane
		isUpdatingExistingMachine bool
		want                      []gomegatypes.GomegaMatcher
		wantErr                   bool
	}{
		{
			name: "should return the correct Machine object when creating a new Machine",
			kcp: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kcpName,
					Namespace: cluster.Namespace,
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.16.6",
					MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
						ObjectMeta:              kcpMachineTemplateObjectMeta,
						NodeDrainTimeout:        duration5s,
						NodeDeletionTimeout:     duration5s,
						NodeVolumeDetachTimeout: duration5s,
					},
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							ClusterName: clusterName,
						},
					},
					MachineNamingStrategy: &controlplanev1.MachineNamingStrategy{
						Template: "{{ .kubeadmControlPlane.name }}" + namingTemplateKey + "-{{ .random }}",
					},
				},
			},
			isUpdatingExistingMachine: false,
			want: []gomegatypes.GomegaMatcher{
				HavePrefix(kcpName + namingTemplateKey),
				Not(HaveSuffix("00000")),
			},
			wantErr: false,
		},
		{
			name: "should return error when creating a new Machine when '.random' is not added in template",
			kcp: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kcpName,
					Namespace: cluster.Namespace,
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.16.6",
					MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
						ObjectMeta:              kcpMachineTemplateObjectMeta,
						NodeDrainTimeout:        duration5s,
						NodeDeletionTimeout:     duration5s,
						NodeVolumeDetachTimeout: duration5s,
					},
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							ClusterName: clusterName,
						},
					},
					MachineNamingStrategy: &controlplanev1.MachineNamingStrategy{
						Template: "{{ .kubeadmControlPlane.name }}" + namingTemplateKey,
					},
				},
			},
			isUpdatingExistingMachine: false,
			wantErr:                   true,
		},
		{
			name: "should not return error when creating a new Machine when the generated name exceeds 63",
			kcp: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kcpName,
					Namespace: cluster.Namespace,
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.16.6",
					MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
						ObjectMeta:              kcpMachineTemplateObjectMeta,
						NodeDrainTimeout:        duration5s,
						NodeDeletionTimeout:     duration5s,
						NodeVolumeDetachTimeout: duration5s,
					},
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							ClusterName: clusterName,
						},
					},
					MachineNamingStrategy: &controlplanev1.MachineNamingStrategy{
						Template: "{{ .random }}" + fmt.Sprintf("%059d", 0),
					},
				},
			},
			isUpdatingExistingMachine: false,
			want: []gomegatypes.GomegaMatcher{
				ContainSubstring(fmt.Sprintf("%053d", 0)),
				Not(HaveSuffix("00000")),
			},
			wantErr: false,
		},
		{
			name: "should return error when creating a new Machine",
			kcp: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kcpName,
					Namespace: cluster.Namespace,
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.16.6",
					MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
						ObjectMeta:              kcpMachineTemplateObjectMeta,
						NodeDrainTimeout:        duration5s,
						NodeDeletionTimeout:     duration5s,
						NodeVolumeDetachTimeout: duration5s,
					},
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							ClusterName: clusterName,
						},
					},
					MachineNamingStrategy: &controlplanev1.MachineNamingStrategy{
						Template: "some-hardcoded-name-{{ .doesnotexistindata }}-{{ .random }}", // invalid template
					},
				},
			},
			isUpdatingExistingMachine: false,
			wantErr:                   true,
		},
		{
			name: "should return the correct Machine object when creating a new Machine with default templated name",
			kcp: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kcpName,
					Namespace: cluster.Namespace,
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.16.6",
					MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
						ObjectMeta:              kcpMachineTemplateObjectMeta,
						NodeDrainTimeout:        duration5s,
						NodeDeletionTimeout:     duration5s,
						NodeVolumeDetachTimeout: duration5s,
					},
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							ClusterName: clusterName,
						},
					},
				},
			},
			isUpdatingExistingMachine: false,
			wantErr:                   false,
			want: []gomegatypes.GomegaMatcher{
				HavePrefix(kcpName),
				Not(HaveSuffix("00000")),
			},
		},
		{
			name: "should return the correct Machine object when updating an existing Machine",
			kcp: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kcpName,
					Namespace: cluster.Namespace,
				},
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.16.6",
					MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
						ObjectMeta:              kcpMachineTemplateObjectMeta,
						NodeDrainTimeout:        duration5s,
						NodeDeletionTimeout:     duration5s,
						NodeVolumeDetachTimeout: duration5s,
					},
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							ClusterName: clusterName,
						},
					},
					MachineNamingStrategy: &controlplanev1.MachineNamingStrategy{
						Template: "{{ .kubeadmControlPlane.name }}" + namingTemplateKey + "-{{ .random }}",
					},
				},
			},
			isUpdatingExistingMachine: true,
			wantErr:                   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			var desiredMachine *clusterv1.Machine
			failureDomain := ptr.To("fd-1")
			var expectedMachineSpec clusterv1.MachineSpec
			var err error

			if tt.isUpdatingExistingMachine {
				machineName := "existing-machine"
				machineUID := types.UID("abc-123-existing-machine")
				// Use different ClusterConfiguration string than the information present in KCP
				// to verify that for an existing machine we do not override this information.
				existingClusterConfigurationString := "existing-cluster-configuration-information"
				remediationData := "remediation-data"
				machineVersion := ptr.To("v1.25.3")
				existingMachine := &clusterv1.Machine{
					ObjectMeta: metav1.ObjectMeta{
						Name: machineName,
						UID:  machineUID,
						Annotations: map[string]string{
							controlplanev1.KubeadmClusterConfigurationAnnotation: existingClusterConfigurationString,
							controlplanev1.RemediationForAnnotation:              remediationData,
						},
					},
					Spec: clusterv1.MachineSpec{
						Version:                 machineVersion,
						FailureDomain:           failureDomain,
						NodeDrainTimeout:        duration10s,
						NodeDeletionTimeout:     duration10s,
						NodeVolumeDetachTimeout: duration10s,
						Bootstrap: clusterv1.Bootstrap{
							ConfigRef: bootstrapRef,
						},
						InfrastructureRef: *infraRef,
						ReadinessGates:    []clusterv1.MachineReadinessGate{{ConditionType: "Foo"}},
					},
				}
				desiredMachine, err = (&KubeadmControlPlaneReconciler{}).computeDesiredMachine(
					tt.kcp, cluster,
					existingMachine.Spec.FailureDomain, existingMachine,
				)
				if tt.wantErr {
					g.Expect(err).To(HaveOccurred())
					return
				}
				g.Expect(err).ToNot(HaveOccurred())
				expectedMachineSpec = clusterv1.MachineSpec{
					ClusterName: cluster.Name,
					Version:     machineVersion, // Should use the Machine version and not the version from KCP.
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: bootstrapRef,
					},
					InfrastructureRef:       *infraRef,
					FailureDomain:           failureDomain,
					NodeDrainTimeout:        tt.kcp.Spec.MachineTemplate.NodeDrainTimeout,
					NodeDeletionTimeout:     tt.kcp.Spec.MachineTemplate.NodeDeletionTimeout,
					NodeVolumeDetachTimeout: tt.kcp.Spec.MachineTemplate.NodeVolumeDetachTimeout,
					ReadinessGates:          append([]clusterv1.MachineReadinessGate{{ConditionType: "Foo"}}, mandatoryMachineReadinessGates...),
				}

				// Verify the Name and UID of the Machine remain unchanged
				g.Expect(desiredMachine.Name).To(Equal(machineName))
				g.Expect(desiredMachine.UID).To(Equal(machineUID))
				// Verify annotations.
				expectedAnnotations := map[string]string{}
				for k, v := range kcpMachineTemplateObjectMeta.Annotations {
					expectedAnnotations[k] = v
				}
				expectedAnnotations[controlplanev1.KubeadmClusterConfigurationAnnotation] = existingClusterConfigurationString
				expectedAnnotations[controlplanev1.RemediationForAnnotation] = remediationData
				// The pre-terminate annotation should always be added
				expectedAnnotations[controlplanev1.PreTerminateHookCleanupAnnotation] = ""
				g.Expect(desiredMachine.Annotations).To(Equal(expectedAnnotations))
			} else {
				desiredMachine, err = (&KubeadmControlPlaneReconciler{}).computeDesiredMachine(
					tt.kcp, cluster,
					failureDomain, nil,
				)
				if tt.wantErr {
					g.Expect(err).To(HaveOccurred())
					return
				}
				g.Expect(err).ToNot(HaveOccurred())

				expectedMachineSpec = clusterv1.MachineSpec{
					ClusterName:             cluster.Name,
					Version:                 ptr.To(tt.kcp.Spec.Version),
					FailureDomain:           failureDomain,
					NodeDrainTimeout:        tt.kcp.Spec.MachineTemplate.NodeDrainTimeout,
					NodeDeletionTimeout:     tt.kcp.Spec.MachineTemplate.NodeDeletionTimeout,
					NodeVolumeDetachTimeout: tt.kcp.Spec.MachineTemplate.NodeVolumeDetachTimeout,
					ReadinessGates:          mandatoryMachineReadinessGates,
				}
				// Verify Name.
				for _, matcher := range tt.want {
					g.Expect(desiredMachine.Name).To(matcher)
				}
				// Verify annotations.
				expectedAnnotations := map[string]string{}
				for k, v := range kcpMachineTemplateObjectMeta.Annotations {
					expectedAnnotations[k] = v
				}
				expectedAnnotations[controlplanev1.KubeadmClusterConfigurationAnnotation] = clusterConfigurationString
				// The pre-terminate annotation should always be added
				expectedAnnotations[controlplanev1.PreTerminateHookCleanupAnnotation] = ""
				g.Expect(desiredMachine.Annotations).To(Equal(expectedAnnotations))
			}

			g.Expect(desiredMachine.Namespace).To(Equal(tt.kcp.Namespace))
			g.Expect(desiredMachine.OwnerReferences).To(HaveLen(1))
			g.Expect(desiredMachine.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(tt.kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane"))))
			g.Expect(desiredMachine.Spec).To(BeComparableTo(expectedMachineSpec))

			// Verify that the machineTemplate.ObjectMeta has been propagated to the Machine.
			// Verify labels.
			expectedLabels := map[string]string{}
			for k, v := range kcpMachineTemplateObjectMeta.Labels {
				expectedLabels[k] = v
			}
			expectedLabels[clusterv1.ClusterNameLabel] = cluster.Name
			expectedLabels[clusterv1.MachineControlPlaneLabel] = ""
			expectedLabels[clusterv1.MachineControlPlaneNameLabel] = tt.kcp.Name
			g.Expect(desiredMachine.Labels).To(Equal(expectedLabels))

			// Verify that machineTemplate.ObjectMeta in KCP has not been modified.
			g.Expect(tt.kcp.Spec.MachineTemplate.ObjectMeta.Labels).To(Equal(kcpMachineTemplateObjectMetaCopy.Labels))
			g.Expect(tt.kcp.Spec.MachineTemplate.ObjectMeta.Annotations).To(Equal(kcpMachineTemplateObjectMetaCopy.Annotations))
		})
	}
}

func TestKubeadmControlPlaneReconciler_generateKubeadmConfig(t *testing.T) {
	g := NewWithT(t)
	fakeClient := newFakeClient()

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testCluster",
			Namespace: metav1.NamespaceDefault,
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
		Client:              fakeClient,
		SecretCachingClient: fakeClient,
		recorder:            record.NewFakeRecorder(32),
	}

	got, err := r.generateKubeadmConfig(ctx, kcp, cluster, spec.DeepCopy(), "kubeadmconfig-name")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(got).NotTo(BeNil())
	g.Expect(got.Name).To(Equal("kubeadmconfig-name"))
	g.Expect(got.Namespace).To(Equal(kcp.Namespace))
	g.Expect(got.Kind).To(Equal(expectedReferenceKind))
	g.Expect(got.APIVersion).To(Equal(expectedReferenceAPIVersion))

	bootstrapConfig := &bootstrapv1.KubeadmConfig{}
	key := client.ObjectKey{Name: got.Name, Namespace: got.Namespace}
	g.Expect(fakeClient.Get(ctx, key, bootstrapConfig)).To(Succeed())
	g.Expect(bootstrapConfig.OwnerReferences).To(HaveLen(1))
	g.Expect(bootstrapConfig.OwnerReferences).To(ContainElement(expectedOwner))
	g.Expect(bootstrapConfig.Spec).To(BeComparableTo(spec))
}

func TestKubeadmControlPlaneReconciler_adoptKubeconfigSecret(t *testing.T) {
	g := NewWithT(t)
	otherOwner := metav1.OwnerReference{
		Name:               "testcontroller",
		UID:                "5",
		Kind:               "OtherController",
		APIVersion:         clusterv1.GroupVersion.String(),
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}

	// A KubeadmConfig secret created by CAPI controllers with no owner references.
	capiKubeadmConfigSecretNoOwner := kubeconfig.GenerateSecretWithOwner(
		client.ObjectKey{Name: "test1", Namespace: metav1.NamespaceDefault},
		[]byte{},
		metav1.OwnerReference{})
	capiKubeadmConfigSecretNoOwner.OwnerReferences = []metav1.OwnerReference{}

	// A KubeadmConfig secret created by CAPI controllers with a non-KCP owner reference.
	capiKubeadmConfigSecretOtherOwner := capiKubeadmConfigSecretNoOwner.DeepCopy()
	capiKubeadmConfigSecretOtherOwner.OwnerReferences = []metav1.OwnerReference{otherOwner}

	// A user provided KubeadmConfig secret with no owner reference.
	userProvidedKubeadmConfigSecretNoOwner := kubeconfig.GenerateSecretWithOwner(
		client.ObjectKey{Name: "test1", Namespace: metav1.NamespaceDefault},
		[]byte{},
		metav1.OwnerReference{})
	userProvidedKubeadmConfigSecretNoOwner.Type = corev1.SecretTypeOpaque

	// A user provided KubeadmConfig with a non-KCP owner reference.
	userProvidedKubeadmConfigSecretOtherOwner := userProvidedKubeadmConfigSecretNoOwner.DeepCopy()
	userProvidedKubeadmConfigSecretOtherOwner.OwnerReferences = []metav1.OwnerReference{otherOwner}

	kcp := &controlplanev1.KubeadmControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmControlPlane",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testControlPlane",
			Namespace: metav1.NamespaceDefault,
		},
	}
	tests := []struct {
		name             string
		configSecret     *corev1.Secret
		expectedOwnerRef metav1.OwnerReference
	}{
		{
			name:         "add KCP owner reference on kubeconfig secret generated by CAPI",
			configSecret: capiKubeadmConfigSecretNoOwner,
			expectedOwnerRef: metav1.OwnerReference{
				Name:               kcp.Name,
				UID:                kcp.UID,
				Kind:               kcp.Kind,
				APIVersion:         kcp.APIVersion,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			},
		},
		{
			name:         "replace owner reference with KCP on kubeconfig secret generated by CAPI with other owner",
			configSecret: capiKubeadmConfigSecretOtherOwner,
			expectedOwnerRef: metav1.OwnerReference{
				Name:               kcp.Name,
				UID:                kcp.UID,
				Kind:               kcp.Kind,
				APIVersion:         kcp.APIVersion,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			},
		},
		{
			name:             "don't add ownerReference on kubeconfig secret provided by user",
			configSecret:     userProvidedKubeadmConfigSecretNoOwner,
			expectedOwnerRef: metav1.OwnerReference{},
		},
		{
			name:             "don't replace ownerReference on kubeconfig secret provided by user",
			configSecret:     userProvidedKubeadmConfigSecretOtherOwner,
			expectedOwnerRef: otherOwner,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(*testing.T) {
			fakeClient := newFakeClient(kcp, tt.configSecret)
			r := &KubeadmControlPlaneReconciler{
				Client:              fakeClient,
				SecretCachingClient: fakeClient,
			}
			g.Expect(r.adoptKubeconfigSecret(ctx, tt.configSecret, kcp)).To(Succeed())
			actualSecret := &corev1.Secret{}
			g.Expect(fakeClient.Get(ctx, client.ObjectKey{Namespace: tt.configSecret.Namespace, Name: tt.configSecret.Name}, actualSecret)).To(Succeed())
			g.Expect(actualSecret.GetOwnerReferences()).To(ConsistOf(tt.expectedOwnerRef))
		})
	}
}
