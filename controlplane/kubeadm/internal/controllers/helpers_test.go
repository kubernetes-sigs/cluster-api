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
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	clientutil "sigs.k8s.io/cluster-api/internal/util/client"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util/collections"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/secret"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestReconcileKubeconfigEmptyAPIEndpoints(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneEndpoint: clusterv1.APIEndpoint{},
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
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
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneEndpoint: clusterv1.APIEndpoint{Host: "test.local", Port: 8443},
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
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
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneEndpoint: clusterv1.APIEndpoint{Host: "test.local", Port: 8443},
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
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
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneEndpoint: clusterv1.APIEndpoint{Host: "test.local", Port: 8443},
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
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

func TestCloneConfigsAndGenerateMachineAndSyncMachines(t *testing.T) {
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
			"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
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
			KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
				JoinConfiguration: bootstrapv1.JoinConfiguration{
					NodeRegistration: bootstrapv1.NodeRegistrationOptions{
						KubeletExtraArgs: []bootstrapv1.Arg{
							{
								Name:  "v",
								Value: ptr.To("8"),
							},
						},
					},
				},
			},
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						Kind:     genericInfrastructureMachineTemplate.GetKind(),
						APIGroup: genericInfrastructureMachineTemplate.GroupVersionKind().Group,
						Name:     genericInfrastructureMachineTemplate.GetName(),
					},
				},
			},
			Version: "v1.16.6",
			MachineNaming: controlplanev1.MachineNamingSpec{
				Template: "{{ .kubeadmControlPlane.name }}" + namingTemplateKey + "-{{ .random }}",
			},
		},
	}

	r := &KubeadmControlPlaneReconciler{
		Client:              env,
		SecretCachingClient: secretCachingClient,
		ssaCache:            ssa.NewCache("test-controller"),
		recorder:            record.NewFakeRecorder(32),
	}

	_, err := r.cloneConfigsAndGenerateMachine(ctx, cluster, kcp, true, "")
	g.Expect(err).To(Succeed())

	machineList := &clusterv1.MachineList{}
	g.Expect(env.GetAPIReader().List(ctx, machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(machineList.Items).To(HaveLen(1))

	m := machineList.Items[0]
	g.Expect(m.Namespace).To(Equal(cluster.Namespace))
	g.Expect(m.Name).NotTo(BeEmpty())
	g.Expect(m.Name).To(HavePrefix(kcp.Name + namingTemplateKey))
	g.Expect(m.Spec.InfrastructureRef.Name).To(Equal(m.Name))
	g.Expect(m.Spec.InfrastructureRef.APIGroup).To(Equal(genericInfrastructureMachineTemplate.GroupVersionKind().Group))
	g.Expect(m.Spec.InfrastructureRef.Kind).To(Equal("GenericInfrastructureMachine"))

	g.Expect(m.Spec.Bootstrap.ConfigRef.Name).To(Equal(m.Name))
	g.Expect(m.Spec.Bootstrap.ConfigRef.APIGroup).To(Equal(bootstrapv1.GroupVersion.Group))
	g.Expect(m.Spec.Bootstrap.ConfigRef.Kind).To(Equal("KubeadmConfig"))

	infraObj, err := external.GetObjectFromContractVersionedRef(ctx, env.GetAPIReader(), m.Spec.InfrastructureRef, m.Namespace)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(infraObj.GetOwnerReferences()).To(HaveLen(1))
	g.Expect(infraObj.GetOwnerReferences()).To(ContainElement(metav1.OwnerReference{
		APIVersion: controlplanev1.GroupVersion.String(),
		Kind:       "KubeadmControlPlane",
		Name:       kcp.Name,
		UID:        kcp.UID,
	}))
	g.Expect(infraObj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromNameAnnotation, genericInfrastructureMachineTemplate.GetName()))
	g.Expect(infraObj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromGroupKindAnnotation, genericInfrastructureMachineTemplate.GroupVersionKind().GroupKind().String()))
	// Note: capi-kubeadmcontrolplane should own ownerReferences and spec, labels and annotations should be orphaned.
	// 		 Labels and annotations will be owned by capi-kubeadmcontrolplane-metadata after the next update
	//		 of labels and annotations.
	g.Expect(cleanupTime(infraObj.GetManagedFields())).To(ConsistOf(toManagedFields([]managedFieldEntry{{
		APIVersion: infraObj.GetAPIVersion(),
		Manager:    kcpManagerName,
		Operation:  metav1.ManagedFieldsOperationApply,
		FieldsV1: `{
"f:metadata":{
	"f:ownerReferences":{
		"k:{\"uid\":\"abc-123-kcp-control-plane\"}":{}
	}
},
"f:spec":{
	"f:hello":{}
}}`,
	}})))

	kubeadmConfig := &bootstrapv1.KubeadmConfig{}
	err = env.GetAPIReader().Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: m.Spec.Bootstrap.ConfigRef.Name}, kubeadmConfig)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(kubeadmConfig.OwnerReferences).To(HaveLen(1))
	g.Expect(kubeadmConfig.OwnerReferences).To(ContainElement(metav1.OwnerReference{
		Kind:       "KubeadmControlPlane",
		APIVersion: controlplanev1.GroupVersion.String(),
		Name:       kcp.Name,
		UID:        kcp.UID,
	}))
	g.Expect(kubeadmConfig.Spec.InitConfiguration).To(BeComparableTo(bootstrapv1.InitConfiguration{}))
	expectedJoinConfiguration := kcp.Spec.KubeadmConfigSpec.JoinConfiguration.DeepCopy()
	expectedJoinConfiguration.ControlPlane = &bootstrapv1.JoinControlPlane{}
	g.Expect(kubeadmConfig.Spec.JoinConfiguration).To(BeComparableTo(*expectedJoinConfiguration))
	// Note: capi-kubeadmcontrolplane should own ownerReferences and spec, labels and annotations should be orphaned.
	// 		 Labels and annotations will be owned by capi-kubeadmcontrolplane-metadata after the next update
	//		 of labels and annotations.
	g.Expect(cleanupTime(kubeadmConfig.GetManagedFields())).To(ConsistOf(toManagedFields([]managedFieldEntry{{
		APIVersion: bootstrapv1.GroupVersion.String(),
		Manager:    kcpManagerName,
		Operation:  metav1.ManagedFieldsOperationApply,
		FieldsV1: `{
"f:metadata":{
	"f:ownerReferences":{
		"k:{\"uid\":\"abc-123-kcp-control-plane\"}":{}
	}
},
"f:spec":{
	"f:joinConfiguration":{
		"f:controlPlane":{},
		"f:nodeRegistration":{
			"f:kubeletExtraArgs":{
				"k:{\"name\":\"v\",\"value\":\"8\"}":{
					".":{},"f:name":{},"f:value":{}}
				}
			}
		}
}}`,
	}})))

	// Sync Machines

	// Note: Ensure the client observed the latest objects so syncMachines below is not failing with conflict errors.
	// Note: Not adding a WaitForCacheToBeUpToDate for infraObj for now as we didn't have test flakes because of it and
	//       WaitForCacheToBeUpToDate does not support Unstructured as of now.
	g.Expect(clientutil.WaitForCacheToBeUpToDate(ctx, r.Client, "cloneConfigsAndGenerateMachine", &m)).To(Succeed())
	g.Expect(clientutil.WaitForCacheToBeUpToDate(ctx, r.Client, "cloneConfigsAndGenerateMachine", kubeadmConfig)).To(Succeed())

	controlPlane, err := internal.NewControlPlane(ctx, r.managementCluster, r.Client, cluster, kcp, collections.FromMachines(&m))
	g.Expect(err).ToNot(HaveOccurred())
	stopReconcile, err := r.syncMachines(ctx, controlPlane)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(stopReconcile).To(BeFalse())

	// Verify managedFields again.
	infraObj, err = external.GetObjectFromContractVersionedRef(ctx, env.GetAPIReader(), m.Spec.InfrastructureRef, m.Namespace)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cleanupTime(infraObj.GetManagedFields())).To(ConsistOf(toManagedFields([]managedFieldEntry{{
		// capi-kubeadmcontrolplane-metadata owns labels and annotations
		APIVersion: infraObj.GetAPIVersion(),
		Manager:    kcpMetadataManagerName,
		Operation:  metav1.ManagedFieldsOperationApply,
		FieldsV1: `{
"f:metadata":{
	"f:annotations":{},
	"f:labels":{
		"f:cluster.x-k8s.io/cluster-name":{},
		"f:cluster.x-k8s.io/control-plane":{},
		"f:cluster.x-k8s.io/control-plane-name":{}
	}
}}`,
	}, {
		// capi-kubeadmcontrolplane owns ownerReferences and spec
		APIVersion: infraObj.GetAPIVersion(),
		Manager:    kcpManagerName,
		Operation:  metav1.ManagedFieldsOperationApply,
		FieldsV1: `{
"f:metadata":{
	"f:ownerReferences":{
		"k:{\"uid\":\"abc-123-kcp-control-plane\"}":{}
	}
},
"f:spec":{
	"f:hello":{}
}}`,
	}})))
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: m.Spec.Bootstrap.ConfigRef.Name}, kubeadmConfig)).To(Succeed())
	g.Expect(cleanupTime(kubeadmConfig.GetManagedFields())).To(ConsistOf(toManagedFields([]managedFieldEntry{{
		// capi-kubeadmcontrolplane-metadata owns labels and annotations
		APIVersion: bootstrapv1.GroupVersion.String(),
		Manager:    kcpMetadataManagerName,
		Operation:  metav1.ManagedFieldsOperationApply,
		FieldsV1: `{
"f:metadata":{
	"f:annotations":{},
	"f:labels":{
		"f:cluster.x-k8s.io/cluster-name":{},
		"f:cluster.x-k8s.io/control-plane":{},
		"f:cluster.x-k8s.io/control-plane-name":{}
	}
}}`,
	}, {
		// capi-kubeadmcontrolplane owns ownerReferences and spec
		APIVersion: bootstrapv1.GroupVersion.String(),
		Manager:    kcpManagerName,
		Operation:  metav1.ManagedFieldsOperationApply,
		FieldsV1: `{
"f:metadata":{
	"f:ownerReferences":{
		"k:{\"uid\":\"abc-123-kcp-control-plane\"}":{}
	}
},
"f:spec":{
	"f:joinConfiguration":{
		"f:controlPlane":{},
		"f:nodeRegistration":{
			"f:kubeletExtraArgs":{
				"k:{\"name\":\"v\",\"value\":\"8\"}":{
					".":{},"f:name":{},"f:value":{}}
				}
			}
		}
}}`,
	}})))

	// Purge managedFields from objects.
	jsonPatch := []map[string]interface{}{
		{
			"op":    "replace",
			"path":  "/metadata/managedFields",
			"value": []metav1.ManagedFieldsEntry{{}},
		},
	}
	patch, err := json.Marshal(jsonPatch)
	g.Expect(err).ToNot(HaveOccurred())
	for _, object := range []client.Object{&m, infraObj, kubeadmConfig} {
		g.Expect(env.Client.Patch(ctx, object, client.RawPatch(types.JSONPatchType, patch))).To(Succeed())
		g.Expect(object.GetManagedFields()).To(BeEmpty())
	}

	// syncMachines to run mitigation code.
	controlPlane.Machines[m.Name] = &m
	controlPlane.InfraResources[infraObj.GetName()] = infraObj
	controlPlane.KubeadmConfigs[kubeadmConfig.Name] = kubeadmConfig
	stopReconcile, err = r.syncMachines(ctx, controlPlane)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(stopReconcile).To(BeTrue())

	// verify mitigation worked
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(&m), &m)).To(Succeed())
	g.Expect(cleanupTime(m.GetManagedFields())).To(ConsistOf(toManagedFields([]managedFieldEntry{{
		APIVersion: clusterv1.GroupVersion.String(),
		Manager:    kcpManagerName, // matches manager of next Apply.
		Operation:  metav1.ManagedFieldsOperationApply,
		FieldsV1:   `{"f:metadata":{"f:name":{}}}`,
	}})))
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(infraObj), infraObj)).To(Succeed())
	g.Expect(cleanupTime(infraObj.GetManagedFields())).To(ConsistOf(toManagedFields([]managedFieldEntry{{
		APIVersion: infraObj.GetAPIVersion(),
		Manager:    kcpMetadataManagerName, // matches manager of next Apply.
		Operation:  metav1.ManagedFieldsOperationApply,
		FieldsV1:   `{"f:metadata":{"f:name":{}}}`,
	}})))
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(kubeadmConfig), kubeadmConfig)).To(Succeed())
	g.Expect(cleanupTime(kubeadmConfig.GetManagedFields())).To(ConsistOf(toManagedFields([]managedFieldEntry{{
		APIVersion: bootstrapv1.GroupVersion.String(),
		Manager:    kcpMetadataManagerName, // matches manager of next Apply.
		Operation:  metav1.ManagedFieldsOperationApply,
		FieldsV1:   `{"f:metadata":{"f:name":{}}}`,
	}})))
}

func TestCloneConfigsAndGenerateMachineFailInfraMachineCreation(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
	}

	genericMachineTemplate := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       builder.GenericInfrastructureMachineTemplateKind,
			"apiVersion": builder.InfrastructureGroupVersion.String(),
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
				Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						Kind:     genericMachineTemplate.GetKind(),
						APIGroup: genericMachineTemplate.GroupVersionKind().Group,
						Name:     genericMachineTemplate.GetName(),
					},
				},
			},
			Version: "v1.16.6",
		},
	}

	fakeClient := newFakeClient(cluster.DeepCopy(), kcp.DeepCopy(), genericMachineTemplate.DeepCopy(), builder.GenericInfrastructureMachineTemplateCRD)

	r := &KubeadmControlPlaneReconciler{
		Client:              fakeClient,
		SecretCachingClient: fakeClient,
		recorder:            record.NewFakeRecorder(32),
	}

	// Break InfraMachine cloning
	kcp.Spec.MachineTemplate.Spec.InfrastructureRef.Name = "something_invalid"
	_, err := r.cloneConfigsAndGenerateMachine(ctx, cluster, kcp, true, "")
	g.Expect(err).To(HaveOccurred())
	g.Expect(&kcp.GetV1Beta1Conditions()[0]).Should(v1beta1conditions.HaveSameStateOf(&clusterv1.Condition{
		Type:     controlplanev1.MachinesCreatedV1Beta1Condition,
		Status:   corev1.ConditionFalse,
		Severity: clusterv1.ConditionSeverityError,
		Reason:   controlplanev1.InfrastructureTemplateCloningFailedV1Beta1Reason,
		Message:  "failed to create InfraMachine: failed to compute desired InfraMachine: failed to retrieve GenericInfrastructureMachineTemplate default/something_invalid: genericinfrastructuremachinetemplates.infrastructure.cluster.x-k8s.io \"something_invalid\" not found",
	}))
	// No objects should exist.
	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(machineList.Items).To(BeEmpty())
	infraMachineList := &unstructured.UnstructuredList{}
	infraMachineList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   builder.InfrastructureGroupVersion.Group,
		Version: builder.InfrastructureGroupVersion.Version,
		Kind:    builder.GenericInfrastructureMachineKind + "List",
	})
	g.Expect(fakeClient.List(ctx, infraMachineList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(infraMachineList.Items).To(BeEmpty())
	kubeadmConfigList := &bootstrapv1.KubeadmConfigList{}
	g.Expect(fakeClient.List(ctx, kubeadmConfigList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(kubeadmConfigList.Items).To(BeEmpty())
}

func TestCloneConfigsAndGenerateMachineFailKubeadmConfigCreation(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
	}

	genericMachineTemplate := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       builder.GenericInfrastructureMachineTemplateKind,
			"apiVersion": builder.InfrastructureGroupVersion.String(),
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
				Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						Kind:     genericMachineTemplate.GetKind(),
						APIGroup: genericMachineTemplate.GroupVersionKind().Group,
						Name:     genericMachineTemplate.GetName(),
					},
				},
			},
			Version: "v1.16.6",
		},
	}

	fakeClient := newFakeClient(cluster.DeepCopy(), kcp.DeepCopy(), genericMachineTemplate.DeepCopy(), builder.GenericInfrastructureMachineTemplateCRD)

	r := &KubeadmControlPlaneReconciler{
		Client:              fakeClient,
		SecretCachingClient: fakeClient,
		recorder:            record.NewFakeRecorder(32),
		// Note: This field is only used for unit tests that use fake client because the fake client does not properly set resourceVersion
		//       on BootstrapConfig/InfraMachine after ssa.Patch and then ssa.RemoveManagedFieldsForLabelsAndAnnotations would fail.
		disableRemoveManagedFieldsForLabelsAndAnnotations: true,
	}

	// Break KubeadmConfig computation
	kcp.Spec.Version = "something_invalid"
	_, err := r.cloneConfigsAndGenerateMachine(ctx, cluster, kcp, true, "")
	g.Expect(err).To(HaveOccurred())
	g.Expect(&kcp.GetV1Beta1Conditions()[0]).Should(v1beta1conditions.HaveSameStateOf(&clusterv1.Condition{
		Type:     controlplanev1.MachinesCreatedV1Beta1Condition,
		Status:   corev1.ConditionFalse,
		Severity: clusterv1.ConditionSeverityError,
		Reason:   controlplanev1.BootstrapTemplateCloningFailedV1Beta1Reason,
		Message:  "failed to create KubeadmConfig: failed to compute desired KubeadmConfig: failed to parse Kubernetes version \"something_invalid\": Invalid character(s) found in major number \"0something_invalid\"",
	}))
	// No objects should exist.
	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(machineList.Items).To(BeEmpty())
	infraMachineList := &unstructured.UnstructuredList{}
	infraMachineList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   builder.InfrastructureGroupVersion.Group,
		Version: builder.InfrastructureGroupVersion.Version,
		Kind:    builder.GenericInfrastructureMachineKind + "List",
	})
	g.Expect(fakeClient.List(ctx, infraMachineList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(infraMachineList.Items).To(BeEmpty())
	kubeadmConfigList := &bootstrapv1.KubeadmConfigList{}
	g.Expect(fakeClient.List(ctx, kubeadmConfigList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(kubeadmConfigList.Items).To(BeEmpty())
}

func TestCloneConfigsAndGenerateMachineFailMachineCreation(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
	}

	genericMachineTemplate := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       builder.GenericInfrastructureMachineTemplateKind,
			"apiVersion": builder.InfrastructureGroupVersion.String(),
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
				Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						Kind:     genericMachineTemplate.GetKind(),
						APIGroup: genericMachineTemplate.GroupVersionKind().Group,
						Name:     genericMachineTemplate.GetName(),
					},
				},
			},
			Version: "v1.16.6",
		},
	}

	fakeClient := newFakeClient(cluster.DeepCopy(), kcp.DeepCopy(), genericMachineTemplate.DeepCopy(), builder.GenericInfrastructureMachineTemplateCRD)
	// Break Machine creation by injecting an error into the Machine apply call.
	fakeClient = interceptor.NewClient(fakeClient, interceptor.Funcs{
		Apply: func(ctx context.Context, c client.WithWatch, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
			clientObject, ok := obj.(client.Object)
			if !ok {
				return errors.Errorf("error during Machine creation: unexpected ApplyConfiguration")
			}
			if clientObject.GetObjectKind().GroupVersionKind().Kind == "Machine" {
				return errors.Errorf("fake error during Machine creation")
			}
			return c.Apply(ctx, obj, opts...)
		},
	})

	r := &KubeadmControlPlaneReconciler{
		Client:              fakeClient,
		SecretCachingClient: fakeClient,
		recorder:            record.NewFakeRecorder(32),
		// Note: This field is only used for unit tests that use fake client because the fake client does not properly set resourceVersion
		//       on BootstrapConfig/InfraMachine after ssa.Patch and then ssa.RemoveManagedFieldsForLabelsAndAnnotations would fail.
		disableRemoveManagedFieldsForLabelsAndAnnotations: true,
	}

	_, err := r.cloneConfigsAndGenerateMachine(ctx, cluster, kcp, true, "")
	g.Expect(err).To(HaveOccurred())
	g.Expect(&kcp.GetV1Beta1Conditions()[0]).Should(v1beta1conditions.HaveSameStateOf(&clusterv1.Condition{
		Type:     controlplanev1.MachinesCreatedV1Beta1Condition,
		Status:   corev1.ConditionFalse,
		Severity: clusterv1.ConditionSeverityError,
		Reason:   controlplanev1.MachineGenerationFailedV1Beta1Reason,
		Message:  "failed to apply Machine: fake error during Machine creation",
	}))
	// No objects should exist.
	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(machineList.Items).To(BeEmpty())
	infraMachineList := &unstructured.UnstructuredList{}
	infraMachineList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   builder.InfrastructureGroupVersion.Group,
		Version: builder.InfrastructureGroupVersion.Version,
		Kind:    builder.GenericInfrastructureMachineKind + "List",
	})
	g.Expect(fakeClient.List(ctx, infraMachineList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(infraMachineList.Items).To(BeEmpty())
	kubeadmConfigList := &bootstrapv1.KubeadmConfigList{}
	g.Expect(fakeClient.List(ctx, kubeadmConfigList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(kubeadmConfigList.Items).To(BeEmpty())
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
				Kind:               "KubeadmControlPlane",
				APIVersion:         controlplanev1.GroupVersion.String(),
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
				Kind:               "KubeadmControlPlane",
				APIVersion:         controlplanev1.GroupVersion.String(),
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

func cleanupTime(fields []metav1.ManagedFieldsEntry) []metav1.ManagedFieldsEntry {
	for i := range fields {
		fields[i].Time = nil
	}
	return fields
}

type managedFieldEntry struct {
	Manager     string
	Operation   metav1.ManagedFieldsOperationType
	APIVersion  string
	FieldsV1    string
	Subresource string
}

func toManagedFields(managedFields []managedFieldEntry) []metav1.ManagedFieldsEntry {
	res := []metav1.ManagedFieldsEntry{}
	for _, f := range managedFields {
		res = append(res, metav1.ManagedFieldsEntry{
			Manager:     f.Manager,
			Operation:   f.Operation,
			APIVersion:  f.APIVersion,
			FieldsType:  "FieldsV1",
			FieldsV1:    &metav1.FieldsV1{Raw: []byte(trimSpaces(f.FieldsV1))},
			Subresource: f.Subresource,
		})
	}
	return res
}

func trimSpaces(s string) string {
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\t", "")
	return s
}
