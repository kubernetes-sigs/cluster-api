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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/blang/semver/v4"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	controlplanev1webhooks "sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/webhooks"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/test/builder"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/internal/webhooks"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/certs"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/secret"
)

func TestClusterToKubeadmControlPlane(t *testing.T) {
	g := NewWithT(t)
	fakeClient := newFakeClient()

	cluster := newCluster(&types.NamespacedName{Name: "foo", Namespace: metav1.NamespaceDefault})
	cluster.Spec = clusterv1.ClusterSpec{
		ControlPlaneRef: &corev1.ObjectReference{
			Kind:       "KubeadmControlPlane",
			Namespace:  metav1.NamespaceDefault,
			Name:       "kcp-foo",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
	}

	expectedResult := []ctrl.Request{
		{
			NamespacedName: client.ObjectKey{
				Namespace: cluster.Spec.ControlPlaneRef.Namespace,
				Name:      cluster.Spec.ControlPlaneRef.Name},
		},
	}

	r := &KubeadmControlPlaneReconciler{
		Client:              fakeClient,
		SecretCachingClient: fakeClient,
		recorder:            record.NewFakeRecorder(32),
	}

	got := r.ClusterToKubeadmControlPlane(ctx, cluster)
	g.Expect(got).To(BeComparableTo(expectedResult))
}

func TestClusterToKubeadmControlPlaneNoControlPlane(t *testing.T) {
	g := NewWithT(t)
	fakeClient := newFakeClient()

	cluster := newCluster(&types.NamespacedName{Name: "foo", Namespace: metav1.NamespaceDefault})

	r := &KubeadmControlPlaneReconciler{
		Client:              fakeClient,
		SecretCachingClient: fakeClient,
		recorder:            record.NewFakeRecorder(32),
	}

	got := r.ClusterToKubeadmControlPlane(ctx, cluster)
	g.Expect(got).To(BeNil())
}

func TestClusterToKubeadmControlPlaneOtherControlPlane(t *testing.T) {
	g := NewWithT(t)
	fakeClient := newFakeClient()

	cluster := newCluster(&types.NamespacedName{Name: "foo", Namespace: metav1.NamespaceDefault})
	cluster.Spec = clusterv1.ClusterSpec{
		ControlPlaneRef: &corev1.ObjectReference{
			Kind:       "OtherControlPlane",
			Namespace:  metav1.NamespaceDefault,
			Name:       "other-foo",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
	}

	r := &KubeadmControlPlaneReconciler{
		Client:              fakeClient,
		SecretCachingClient: fakeClient,
		recorder:            record.NewFakeRecorder(32),
	}

	got := r.ClusterToKubeadmControlPlane(ctx, cluster)
	g.Expect(got).To(BeNil())
}

func TestReconcileReturnErrorWhenOwnerClusterIsMissing(t *testing.T) {
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "test-reconcile-return-error")
	g.Expect(err).ToNot(HaveOccurred())

	cluster, kcp, _ := createClusterWithControlPlane(ns.Name)
	g.Expect(env.Create(ctx, cluster)).To(Succeed())
	g.Expect(env.Create(ctx, kcp)).To(Succeed())
	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(kcp, ns)

	r := &KubeadmControlPlaneReconciler{
		Client:              env,
		SecretCachingClient: secretCachingClient,
		recorder:            record.NewFakeRecorder(32),
	}

	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(kcp)})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(BeComparableTo(ctrl.Result{}))

	// calling reconcile should return error
	g.Expect(env.CleanupAndWait(ctx, cluster)).To(Succeed())

	g.Eventually(func() error {
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(kcp)})
		return err
	}, 10*time.Second).Should(HaveOccurred())
}

func TestReconcileUpdateObservedGeneration(t *testing.T) {
	t.Skip("Disabling this test temporarily until we can get a fix for https://github.com/kubernetes/kubernetes/issues/80609 in controller runtime + switch to a live client in test env.")

	g := NewWithT(t)
	r := &KubeadmControlPlaneReconciler{
		Client:              env,
		SecretCachingClient: secretCachingClient,
		recorder:            record.NewFakeRecorder(32),
		managementCluster:   &internal.Management{Client: env.Client, ClusterCache: nil},
	}

	ns, err := env.CreateNamespace(ctx, "test-reconcile-upd-og")
	g.Expect(err).ToNot(HaveOccurred())

	cluster, kcp, _ := createClusterWithControlPlane(ns.Name)
	g.Expect(env.Create(ctx, cluster)).To(Succeed())
	g.Expect(env.Create(ctx, kcp)).To(Succeed())
	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(cluster, kcp, ns)

	// read kcp.Generation after create
	errGettingObject := env.Get(ctx, util.ObjectKey(kcp), kcp)
	g.Expect(errGettingObject).ToNot(HaveOccurred())
	generation := kcp.Generation

	// Set cluster.status.InfrastructureReady so we actually enter in the reconcile loop
	patch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf("{\"status\":{\"infrastructureReady\":%t}}", true)))
	g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())

	// call reconcile the first time, so we can check if observedGeneration is set when adding a finalizer
	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(kcp)})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(BeComparableTo(ctrl.Result{}))

	g.Eventually(func() int64 {
		errGettingObject = env.Get(ctx, util.ObjectKey(kcp), kcp)
		g.Expect(errGettingObject).ToNot(HaveOccurred())
		return kcp.Status.ObservedGeneration
	}, 10*time.Second).Should(Equal(generation))

	// triggers a generation change by changing the spec
	kcp.Spec.Replicas = ptr.To[int32](*kcp.Spec.Replicas + 2)
	g.Expect(env.Update(ctx, kcp)).To(Succeed())

	// read kcp.Generation after the update
	errGettingObject = env.Get(ctx, util.ObjectKey(kcp), kcp)
	g.Expect(errGettingObject).ToNot(HaveOccurred())
	generation = kcp.Generation

	// call reconcile the second time, so we can check if observedGeneration is set when calling defer patch
	// NB. The call to reconcile fails because KCP is not properly setup (e.g. missing InfrastructureTemplate)
	// but this is not important because what we want is KCP to be patched
	_, _ = r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(kcp)})

	g.Eventually(func() int64 {
		errGettingObject = env.Get(ctx, util.ObjectKey(kcp), kcp)
		g.Expect(errGettingObject).ToNot(HaveOccurred())
		return kcp.Status.ObservedGeneration
	}, 10*time.Second).Should(Equal(generation))
}

func TestReconcileNoClusterOwnerRef(t *testing.T) {
	g := NewWithT(t)

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceDefault,
			Name:      "foo",
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.16.6",
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       "UnknownInfraMachine",
					APIVersion: "test/v1alpha1",
					Name:       "foo",
					Namespace:  metav1.NamespaceDefault,
				},
			},
		},
	}
	webhook := &controlplanev1webhooks.KubeadmControlPlane{}
	g.Expect(webhook.Default(ctx, kcp)).To(Succeed())
	_, err := webhook.ValidateCreate(ctx, kcp)
	g.Expect(err).ToNot(HaveOccurred())

	fakeClient := newFakeClient(kcp.DeepCopy())
	r := &KubeadmControlPlaneReconciler{
		Client:              fakeClient,
		SecretCachingClient: fakeClient,
		recorder:            record.NewFakeRecorder(32),
	}

	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(kcp)})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(BeComparableTo(ctrl.Result{}))

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(metav1.NamespaceDefault))).To(Succeed())
	g.Expect(machineList.Items).To(BeEmpty())
}

func TestReconcileNoKCP(t *testing.T) {
	g := NewWithT(t)

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceDefault,
			Name:      "foo",
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.16.6",
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       "UnknownInfraMachine",
					APIVersion: "test/v1alpha1",
					Name:       "foo",
					Namespace:  metav1.NamespaceDefault,
				},
			},
		},
	}

	fakeClient := newFakeClient()
	r := &KubeadmControlPlaneReconciler{
		Client:              fakeClient,
		SecretCachingClient: fakeClient,
		recorder:            record.NewFakeRecorder(32),
	}

	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(kcp)})
	g.Expect(err).ToNot(HaveOccurred())
}

func TestReconcileNoCluster(t *testing.T) {
	g := NewWithT(t)

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceDefault,
			Name:      "foo",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
					Name:       "foo",
				},
			},
			Finalizers: []string{
				controlplanev1.KubeadmControlPlaneFinalizer,
			},
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.16.6",
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       "UnknownInfraMachine",
					APIVersion: "test/v1alpha1",
					Name:       "foo",
					Namespace:  metav1.NamespaceDefault,
				},
			},
		},
	}
	webhook := &controlplanev1webhooks.KubeadmControlPlane{}
	g.Expect(webhook.Default(ctx, kcp)).To(Succeed())
	_, err := webhook.ValidateCreate(ctx, kcp)
	g.Expect(err).ToNot(HaveOccurred())

	fakeClient := newFakeClient(kcp.DeepCopy())
	r := &KubeadmControlPlaneReconciler{
		Client:              fakeClient,
		SecretCachingClient: fakeClient,
		recorder:            record.NewFakeRecorder(32),
	}

	_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(kcp)})
	g.Expect(err).To(HaveOccurred())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(metav1.NamespaceDefault))).To(Succeed())
	g.Expect(machineList.Items).To(BeEmpty())
}

func TestReconcilePaused(t *testing.T) {
	g := NewWithT(t)

	clusterName := "foo"

	// Test: cluster is paused and kcp is not
	cluster := newCluster(&types.NamespacedName{Namespace: metav1.NamespaceDefault, Name: clusterName})
	cluster.Spec.Paused = true
	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceDefault,
			Name:      clusterName,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
					Name:       clusterName,
				},
			},
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.16.6",
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       "UnknownInfraMachine",
					APIVersion: "test/v1alpha1",
					Name:       "foo",
					Namespace:  metav1.NamespaceDefault,
				},
			},
		},
	}
	webhook := &controlplanev1webhooks.KubeadmControlPlane{}
	g.Expect(webhook.Default(ctx, kcp)).To(Succeed())
	_, err := webhook.ValidateCreate(ctx, kcp)
	g.Expect(err).ToNot(HaveOccurred())
	fakeClient := newFakeClient(kcp.DeepCopy(), cluster.DeepCopy())
	r := &KubeadmControlPlaneReconciler{
		Client:              fakeClient,
		SecretCachingClient: fakeClient,
		recorder:            record.NewFakeRecorder(32),
	}

	_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(kcp)})
	g.Expect(err).ToNot(HaveOccurred())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(metav1.NamespaceDefault))).To(Succeed())
	g.Expect(machineList.Items).To(BeEmpty())

	// Test: kcp is paused and cluster is not
	cluster.Spec.Paused = false
	kcp.ObjectMeta.Annotations = map[string]string{}
	kcp.ObjectMeta.Annotations[clusterv1.PausedAnnotation] = "paused"
	_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(kcp)})
	g.Expect(err).ToNot(HaveOccurred())
}

func TestReconcileClusterNoEndpoints(t *testing.T) {
	g := NewWithT(t)

	cluster := newCluster(&types.NamespacedName{Name: "foo", Namespace: metav1.NamespaceDefault})
	cluster.Status = clusterv1.ClusterStatus{InfrastructureReady: true}

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
			Version: "v1.16.6",
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       "UnknownInfraMachine",
					APIVersion: "test/v1alpha1",
					Name:       "foo",
					Namespace:  metav1.NamespaceDefault,
				},
			},
		},
		Status: controlplanev1.KubeadmControlPlaneStatus{
			V1Beta2: &controlplanev1.KubeadmControlPlaneV1Beta2Status{Conditions: []metav1.Condition{{
				Type:   clusterv1.PausedV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.NotPausedV1Beta2Reason,
			}}},
		},
	}
	webhook := &controlplanev1webhooks.KubeadmControlPlane{}
	g.Expect(webhook.Default(ctx, kcp)).To(Succeed())
	_, err := webhook.ValidateCreate(ctx, kcp)
	g.Expect(err).ToNot(HaveOccurred())

	fakeClient := newFakeClient(kcp.DeepCopy(), cluster.DeepCopy())
	r := &KubeadmControlPlaneReconciler{
		Client:              fakeClient,
		SecretCachingClient: fakeClient,
		recorder:            record.NewFakeRecorder(32),
		managementCluster: &fakeManagementCluster{
			Management: &internal.Management{Client: fakeClient},
			Workload:   &fakeWorkloadCluster{},
		},
	}

	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(kcp)})
	g.Expect(err).ToNot(HaveOccurred())
	// this first requeue is to add finalizer
	g.Expect(result).To(BeComparableTo(ctrl.Result{}))
	g.Expect(r.Client.Get(ctx, util.ObjectKey(kcp), kcp)).To(Succeed())
	g.Expect(kcp.Finalizers).To(ContainElement(controlplanev1.KubeadmControlPlaneFinalizer))

	result, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(kcp)})
	g.Expect(err).ToNot(HaveOccurred())
	// TODO: this should stop to re-queue as soon as we have a proper remote cluster cache in place.
	g.Expect(result).To(BeComparableTo(ctrl.Result{Requeue: false, RequeueAfter: 20 * time.Second}))
	g.Expect(r.Client.Get(ctx, util.ObjectKey(kcp), kcp)).To(Succeed())

	// Always expect that the Finalizer is set on the passed in resource
	g.Expect(kcp.Finalizers).To(ContainElement(controlplanev1.KubeadmControlPlaneFinalizer))

	g.Expect(kcp.Status.Selector).NotTo(BeEmpty())

	_, err = secret.GetFromNamespacedName(ctx, fakeClient, client.ObjectKey{Namespace: metav1.NamespaceDefault, Name: "foo"}, secret.ClusterCA)
	g.Expect(err).ToNot(HaveOccurred())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(metav1.NamespaceDefault))).To(Succeed())
	g.Expect(machineList.Items).To(BeEmpty())
}

func TestKubeadmControlPlaneReconciler_adoption(t *testing.T) {
	version := "v2.0.0"
	t.Run("adopts existing Machines", func(t *testing.T) {
		g := NewWithT(t)

		cluster, kcp, tmpl := createClusterWithControlPlane(metav1.NamespaceDefault)
		cluster.Spec.ControlPlaneEndpoint.Host = "bar"
		cluster.Spec.ControlPlaneEndpoint.Port = 6443
		cluster.Status.InfrastructureReady = true
		kcp.Spec.Version = version

		fmc := &fakeManagementCluster{
			Machines: collections.Machines{},
			Workload: &fakeWorkloadCluster{},
		}
		objs := []client.Object{builder.GenericInfrastructureMachineTemplateCRD, cluster.DeepCopy(), kcp.DeepCopy(), tmpl.DeepCopy()}
		for i := range 3 {
			name := fmt.Sprintf("test-%d", i)
			m := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cluster.Namespace,
					Name:      name,
					Labels:    internal.ControlPlaneMachineLabelsForCluster(kcp, cluster.Name),
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: bootstrapv1.GroupVersion.String(),
							Kind:       "KubeadmConfig",
							Name:       name,
						},
					},
					Version: &version,
				},
			}
			cfg := &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cluster.Namespace,
					Name:      name,
				},
			}
			objs = append(objs, m, cfg)
			fmc.Machines.Insert(m)
		}

		fakeClient := newFakeClient(objs...)
		fmc.Reader = fakeClient
		r := &KubeadmControlPlaneReconciler{
			Client:                    fakeClient,
			SecretCachingClient:       fakeClient,
			managementCluster:         fmc,
			managementClusterUncached: fmc,
		}

		_, adoptableMachineFound, err := r.initControlPlaneScope(ctx, cluster, kcp)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(adoptableMachineFound).To(BeTrue())

		machineList := &clusterv1.MachineList{}
		g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
		g.Expect(machineList.Items).To(HaveLen(3))
		for _, machine := range machineList.Items {
			g.Expect(machine.OwnerReferences).To(HaveLen(1))
			g.Expect(machine.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane"))))
			// Machines are adopted but since they are not originally created by KCP, infra template annotation will be missing.
			g.Expect(machine.GetAnnotations()).NotTo(HaveKey(clusterv1.TemplateClonedFromGroupKindAnnotation))
			g.Expect(machine.GetAnnotations()).NotTo(HaveKey(clusterv1.TemplateClonedFromNameAnnotation))
		}
	})

	t.Run("adopts v1alpha2 cluster secrets", func(t *testing.T) {
		g := NewWithT(t)

		cluster, kcp, tmpl := createClusterWithControlPlane(metav1.NamespaceDefault)
		cluster.Spec.ControlPlaneEndpoint.Host = "validhost"
		cluster.Spec.ControlPlaneEndpoint.Port = 6443
		cluster.Status.InfrastructureReady = true
		kcp.Spec.Version = version

		fmc := &fakeManagementCluster{
			Machines: collections.Machines{},
			Workload: &fakeWorkloadCluster{},
		}
		objs := []client.Object{builder.GenericInfrastructureMachineTemplateCRD, cluster.DeepCopy(), kcp.DeepCopy(), tmpl.DeepCopy()}
		for i := range 3 {
			name := fmt.Sprintf("test-%d", i)
			m := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cluster.Namespace,
					Name:      name,
					Labels:    internal.ControlPlaneMachineLabelsForCluster(kcp, cluster.Name),
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: bootstrapv1.GroupVersion.String(),
							Kind:       "KubeadmConfig",
							Name:       name,
						},
					},
					Version: &version,
				},
			}
			cfg := &bootstrapv1.KubeadmConfig{
				TypeMeta: metav1.TypeMeta{
					APIVersion: bootstrapv1.GroupVersion.String(),
					Kind:       "KubeadmConfig",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cluster.Namespace,
					Name:      name,
					UID:       types.UID(fmt.Sprintf("my-uid-%d", i)),
				},
			}

			// A simulacrum of the various Certificate and kubeconfig secrets
			// it's a little weird that this is one per KubeadmConfig rather than just whichever config was "first,"
			// but the intent is to ensure that the owner is changed regardless of which Machine we start with
			clusterSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cluster.Namespace,
					Name:      fmt.Sprintf("important-cluster-secret-%d", i),
					Labels: map[string]string{
						"cluster.x-k8s.io/cluster-name": cluster.Name,
						"previous-owner":                "kubeadmconfig",
					},
					// See: https://github.com/kubernetes-sigs/cluster-api-bootstrap-provider-kubeadm/blob/38af74d92056e64e571b9ea1d664311769779453/internal/cluster/certificates.go#L323-L330
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: bootstrapv1.GroupVersion.String(),
							Kind:       "KubeadmConfig",
							Name:       cfg.Name,
							UID:        cfg.UID,
						},
					},
				},
			}
			objs = append(objs, m, cfg, clusterSecret)
			fmc.Machines.Insert(m)
		}

		fakeClient := newFakeClient(objs...)
		fmc.Reader = fakeClient
		r := &KubeadmControlPlaneReconciler{
			Client:                    fakeClient,
			SecretCachingClient:       fakeClient,
			managementCluster:         fmc,
			managementClusterUncached: fmc,
		}

		_, adoptableMachineFound, err := r.initControlPlaneScope(ctx, cluster, kcp)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(adoptableMachineFound).To(BeTrue())

		machineList := &clusterv1.MachineList{}
		g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
		g.Expect(machineList.Items).To(HaveLen(3))
		for _, machine := range machineList.Items {
			g.Expect(machine.OwnerReferences).To(HaveLen(1))
			g.Expect(machine.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane"))))
			// Machines are adopted but since they are not originally created by KCP, infra template annotation will be missing.
			g.Expect(machine.GetAnnotations()).NotTo(HaveKey(clusterv1.TemplateClonedFromGroupKindAnnotation))
			g.Expect(machine.GetAnnotations()).NotTo(HaveKey(clusterv1.TemplateClonedFromNameAnnotation))
		}

		secrets := &corev1.SecretList{}
		g.Expect(fakeClient.List(ctx, secrets, client.InNamespace(cluster.Namespace), client.MatchingLabels{"previous-owner": "kubeadmconfig"})).To(Succeed())
		g.Expect(secrets.Items).To(HaveLen(3))
		for _, secret := range secrets.Items {
			g.Expect(secret.OwnerReferences).To(HaveLen(1))
			g.Expect(secret.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane"))))
		}
	})

	t.Run("Deleted KubeadmControlPlanes don't adopt machines", func(t *testing.T) {
		// Usually we won't get into the inner reconcile with a deleted control plane, but it's possible when deleting with "orphanDependents":
		// 1. The deletion timestamp is set in the API server, but our cache has not yet updated
		// 2. The garbage collector removes our ownership reference from a Machine, triggering a re-reconcile (or we get unlucky with the periodic reconciliation)
		// 3. We get into the inner reconcile function and re-adopt the Machine
		// 4. The update to our cache for our deletion timestamp arrives
		g := NewWithT(t)

		cluster, kcp, tmpl := createClusterWithControlPlane(metav1.NamespaceDefault)
		cluster.Spec.ControlPlaneEndpoint.Host = "nodomain.example.com1"
		cluster.Spec.ControlPlaneEndpoint.Port = 6443
		cluster.Status.InfrastructureReady = true
		kcp.Spec.Version = version

		now := metav1.Now()
		kcp.DeletionTimestamp = &now
		// We also have to set a finalizer as fake client doesn't accept objects
		// with a deletionTimestamp without a finalizer.
		kcp.Finalizers = []string{"block-deletion"}

		fmc := &fakeManagementCluster{
			Machines: collections.Machines{},
			Workload: &fakeWorkloadCluster{},
		}
		objs := []client.Object{builder.GenericInfrastructureMachineTemplateCRD, cluster.DeepCopy(), kcp.DeepCopy(), tmpl.DeepCopy()}
		for i := range 3 {
			name := fmt.Sprintf("test-%d", i)
			m := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cluster.Namespace,
					Name:      name,
					Labels:    internal.ControlPlaneMachineLabelsForCluster(kcp, cluster.Name),
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: bootstrapv1.GroupVersion.String(),
							Kind:       "KubeadmConfig",
							Name:       name,
						},
					},
					Version: &version,
				},
			}
			cfg := &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cluster.Namespace,
					Name:      name,
				},
			}
			objs = append(objs, m, cfg)
			fmc.Machines.Insert(m)
		}
		fakeClient := newFakeClient(objs...)
		fmc.Reader = fakeClient
		r := &KubeadmControlPlaneReconciler{
			Client:                    fakeClient,
			SecretCachingClient:       fakeClient,
			managementCluster:         fmc,
			managementClusterUncached: fmc,
		}

		_, adoptableMachineFound, err := r.initControlPlaneScope(ctx, cluster, kcp)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(adoptableMachineFound).To(BeFalse())

		machineList := &clusterv1.MachineList{}
		g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
		g.Expect(machineList.Items).To(HaveLen(3))
		for _, machine := range machineList.Items {
			g.Expect(machine.OwnerReferences).To(BeEmpty())
		}
	})

	t.Run("Do not adopt Machines that are more than one version old", func(t *testing.T) {
		g := NewWithT(t)

		cluster, kcp, tmpl := createClusterWithControlPlane(metav1.NamespaceDefault)
		cluster.Spec.ControlPlaneEndpoint.Host = "nodomain.example.com2"
		cluster.Spec.ControlPlaneEndpoint.Port = 6443
		cluster.Status.InfrastructureReady = true
		kcp.Spec.Version = "v1.17.0"

		fmc := &fakeManagementCluster{
			Machines: collections.Machines{
				"test0": &clusterv1.Machine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: cluster.Namespace,
						Name:      "test0",
						Labels:    internal.ControlPlaneMachineLabelsForCluster(kcp, cluster.Name),
					},
					Spec: clusterv1.MachineSpec{
						Bootstrap: clusterv1.Bootstrap{
							ConfigRef: &corev1.ObjectReference{
								APIVersion: bootstrapv1.GroupVersion.String(),
								Kind:       "KubeadmConfig",
							},
						},
						Version: ptr.To("v1.15.0"),
					},
				},
			},
			Workload: &fakeWorkloadCluster{},
		}

		fakeClient := newFakeClient(builder.GenericInfrastructureMachineTemplateCRD, cluster.DeepCopy(), kcp.DeepCopy(), tmpl.DeepCopy(), fmc.Machines["test0"].DeepCopy())
		fmc.Reader = fakeClient
		recorder := record.NewFakeRecorder(32)
		r := &KubeadmControlPlaneReconciler{
			Client:                    fakeClient,
			SecretCachingClient:       fakeClient,
			recorder:                  recorder,
			managementCluster:         fmc,
			managementClusterUncached: fmc,
		}

		_, adoptableMachineFound, err := r.initControlPlaneScope(ctx, cluster, kcp)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(adoptableMachineFound).To(BeTrue())

		// Message: Warning AdoptionFailed Could not adopt Machine test/test0: its version ("v1.15.0") is outside supported +/- one minor version skew from KCP's ("v1.17.0")
		g.Expect(recorder.Events).To(Receive(ContainSubstring("minor version")))

		machineList := &clusterv1.MachineList{}
		g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
		g.Expect(machineList.Items).To(HaveLen(1))
		for _, machine := range machineList.Items {
			g.Expect(machine.OwnerReferences).To(BeEmpty())
		}
	})
}

func TestKubeadmControlPlaneReconciler_ensureOwnerReferences(t *testing.T) {
	g := NewWithT(t)

	cluster, kcp, tmpl := createClusterWithControlPlane(metav1.NamespaceDefault)
	cluster.Spec.ControlPlaneEndpoint.Host = "bar"
	cluster.Spec.ControlPlaneEndpoint.Port = 6443
	cluster.Status.InfrastructureReady = true
	kcp.Spec.Version = "v1.21.0"
	key, err := certs.NewPrivateKey()
	g.Expect(err).ToNot(HaveOccurred())
	crt, err := getTestCACert(key)
	g.Expect(err).ToNot(HaveOccurred())

	clusterSecret := &corev1.Secret{
		// The Secret's Type is used by KCP to determine whether it is user-provided.
		// clusterv1.ClusterSecretType signals that the Secret is CAPI-provided.
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      "",
			Labels: map[string]string{
				"cluster.x-k8s.io/cluster-name": cluster.Name,
				"testing":                       "yes",
			},
		},
		Data: map[string][]byte{
			secret.TLSCrtDataName: certs.EncodeCertPEM(crt),
			secret.TLSKeyDataName: certs.EncodePrivateKeyPEM(key),
		},
	}

	kcpOwner := *metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane"))

	t.Run("add KCP owner for secrets with no controller reference", func(*testing.T) {
		objs := []client.Object{builder.GenericInfrastructureMachineTemplateCRD, cluster.DeepCopy(), kcp.DeepCopy(), tmpl.DeepCopy()}
		certificates := secret.Certificates{
			{Purpose: secret.ClusterCA},
			{Purpose: secret.FrontProxyCA},
			{Purpose: secret.ServiceAccount},
			{Purpose: secret.EtcdCA},
		}
		for _, c := range certificates {
			s := clusterSecret.DeepCopy()
			// Set the secret name to the purpose
			s.Name = secret.Name(cluster.Name, c.Purpose)
			// Set the Secret Type to clusterv1.ClusterSecretType which signals this Secret was generated by CAPI.
			s.Type = clusterv1.ClusterSecretType

			// Store the secret in the certificate.
			c.Secret = s

			objs = append(objs, s)
		}

		fakeClient := newFakeClient(objs...)

		r := KubeadmControlPlaneReconciler{
			Client:              fakeClient,
			SecretCachingClient: fakeClient,
		}
		err = r.ensureCertificatesOwnerRef(ctx, certificates, kcpOwner)
		g.Expect(err).ToNot(HaveOccurred())

		secrets := &corev1.SecretList{}
		g.Expect(fakeClient.List(ctx, secrets, client.InNamespace(cluster.Namespace), client.MatchingLabels{"testing": "yes"})).To(Succeed())
		for _, secret := range secrets.Items {
			g.Expect(secret.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane"))))
		}
	})

	t.Run("replace non-KCP controller with KCP controller reference", func(*testing.T) {
		objs := []client.Object{builder.GenericInfrastructureMachineTemplateCRD, cluster.DeepCopy(), kcp.DeepCopy(), tmpl.DeepCopy()}
		certificates := secret.Certificates{
			{Purpose: secret.ClusterCA},
			{Purpose: secret.FrontProxyCA},
			{Purpose: secret.ServiceAccount},
			{Purpose: secret.EtcdCA},
		}
		for _, c := range certificates {
			s := clusterSecret.DeepCopy()
			// Set the secret name to the purpose
			s.Name = secret.Name(cluster.Name, c.Purpose)
			// Set the Secret Type to clusterv1.ClusterSecretType which signals this Secret was generated by CAPI.
			s.Type = clusterv1.ClusterSecretType

			// Set the a controller owner reference of an unknown type on the secret.
			s.SetOwnerReferences([]metav1.OwnerReference{
				{
					APIVersion: bootstrapv1.GroupVersion.String(),
					// KCP should take ownership of any Secret of the correct type linked to the Cluster.
					Kind:       "OtherController",
					Name:       "name",
					UID:        "uid",
					Controller: ptr.To(true),
				},
			})

			// Store the secret in the certificate.
			c.Secret = s

			objs = append(objs, s)
		}

		fakeClient := newFakeClient(objs...)

		r := KubeadmControlPlaneReconciler{
			Client:              fakeClient,
			SecretCachingClient: fakeClient,
		}
		err := r.ensureCertificatesOwnerRef(ctx, certificates, kcpOwner)
		g.Expect(err).ToNot(HaveOccurred())

		secrets := &corev1.SecretList{}
		g.Expect(fakeClient.List(ctx, secrets, client.InNamespace(cluster.Namespace), client.MatchingLabels{"testing": "yes"})).To(Succeed())
		for _, secret := range secrets.Items {
			g.Expect(secret.OwnerReferences).To(HaveLen(1))
			g.Expect(secret.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane"))))
		}
	})

	t.Run("does not add owner reference to user-provided secrets", func(t *testing.T) {
		g := NewWithT(t)
		objs := []client.Object{builder.GenericInfrastructureMachineTemplateCRD, cluster.DeepCopy(), kcp.DeepCopy(), tmpl.DeepCopy()}
		certificates := secret.Certificates{
			{Purpose: secret.ClusterCA},
			{Purpose: secret.FrontProxyCA},
			{Purpose: secret.ServiceAccount},
			{Purpose: secret.EtcdCA},
		}
		for _, c := range certificates {
			s := clusterSecret.DeepCopy()
			// Set the secret name to the purpose
			s.Name = secret.Name(cluster.Name, c.Purpose)
			// Set the Secret Type to any type which signals this Secret was is user-provided.
			s.Type = corev1.SecretTypeOpaque
			// Set the a controller owner reference of an unknown type on the secret.
			s.SetOwnerReferences(util.EnsureOwnerRef(s.GetOwnerReferences(),
				metav1.OwnerReference{
					APIVersion: bootstrapv1.GroupVersion.String(),
					// This owner reference to a different controller should be preserved.
					Kind:               "OtherController",
					Name:               kcp.Name,
					UID:                kcp.UID,
					Controller:         ptr.To(true),
					BlockOwnerDeletion: ptr.To(true),
				},
			))

			// Store the secret in the certificate.
			c.Secret = s

			objs = append(objs, s)
		}

		fakeClient := newFakeClient(objs...)

		r := KubeadmControlPlaneReconciler{
			Client:              fakeClient,
			SecretCachingClient: fakeClient,
		}
		err := r.ensureCertificatesOwnerRef(ctx, certificates, kcpOwner)
		g.Expect(err).ToNot(HaveOccurred())

		secrets := &corev1.SecretList{}
		g.Expect(fakeClient.List(ctx, secrets, client.InNamespace(cluster.Namespace), client.MatchingLabels{"testing": "yes"})).To(Succeed())
		for _, secret := range secrets.Items {
			g.Expect(secret.OwnerReferences).To(HaveLen(1))
			g.Expect(secret.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(kcp, bootstrapv1.GroupVersion.WithKind("OtherController"))))
		}
	})
}

func TestReconcileCertificateExpiries(t *testing.T) {
	g := NewWithT(t)

	preExistingExpiry := time.Now().Add(5 * 24 * time.Hour)
	detectedExpiry := time.Now().Add(25 * 24 * time.Hour)

	cluster := newCluster(&types.NamespacedName{Name: "foo", Namespace: metav1.NamespaceDefault})
	kcp := &controlplanev1.KubeadmControlPlane{
		Status: controlplanev1.KubeadmControlPlaneStatus{Initialized: true},
	}
	machineWithoutExpiryAnnotation := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machineWithoutExpiryAnnotation",
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				Kind:       "GenericMachine",
				APIVersion: "generic.io/v1",
				Namespace:  metav1.NamespaceDefault,
				Name:       "machineWithoutExpiryAnnotation-infra",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					Kind:       "KubeadmConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
					Namespace:  metav1.NamespaceDefault,
					Name:       "machineWithoutExpiryAnnotation-bootstrap",
				},
			},
		},
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Name: "machineWithoutExpiryAnnotation",
			},
		},
	}
	machineWithoutExpiryAnnotationKubeadmConfig := &bootstrapv1.KubeadmConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machineWithoutExpiryAnnotation-bootstrap",
		},
	}
	machineWithExpiryAnnotation := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machineWithExpiryAnnotation",
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				Kind:       "GenericMachine",
				APIVersion: "generic.io/v1",
				Namespace:  metav1.NamespaceDefault,
				Name:       "machineWithExpiryAnnotation-infra",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					Kind:       "KubeadmConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
					Namespace:  metav1.NamespaceDefault,
					Name:       "machineWithExpiryAnnotation-bootstrap",
				},
			},
		},
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Name: "machineWithExpiryAnnotation",
			},
		},
	}
	machineWithExpiryAnnotationKubeadmConfig := &bootstrapv1.KubeadmConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machineWithExpiryAnnotation-bootstrap",
			Annotations: map[string]string{
				clusterv1.MachineCertificatesExpiryDateAnnotation: preExistingExpiry.Format(time.RFC3339),
			},
		},
	}
	machineWithDeletionTimestamp := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "machineWithDeletionTimestamp",
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				Kind:       "GenericMachine",
				APIVersion: "generic.io/v1",
				Namespace:  metav1.NamespaceDefault,
				Name:       "machineWithDeletionTimestamp-infra",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					Kind:       "KubeadmConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
					Namespace:  metav1.NamespaceDefault,
					Name:       "machineWithDeletionTimestamp-bootstrap",
				},
			},
		},
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Name: "machineWithDeletionTimestamp",
			},
		},
	}
	machineWithDeletionTimestampKubeadmConfig := &bootstrapv1.KubeadmConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machineWithDeletionTimestamp-bootstrap",
		},
	}
	machineWithoutNodeRef := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machineWithoutNodeRef",
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				Kind:       "GenericMachine",
				APIVersion: "generic.io/v1",
				Namespace:  metav1.NamespaceDefault,
				Name:       "machineWithoutNodeRef-infra",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					Kind:       "KubeadmConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
					Namespace:  metav1.NamespaceDefault,
					Name:       "machineWithoutNodeRef-bootstrap",
				},
			},
		},
	}
	machineWithoutNodeRefKubeadmConfig := &bootstrapv1.KubeadmConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machineWithoutNodeRef-bootstrap",
		},
	}
	machineWithoutKubeadmConfig := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machineWithoutKubeadmConfig",
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				Kind:       "GenericMachine",
				APIVersion: "generic.io/v1",
				Namespace:  metav1.NamespaceDefault,
				Name:       "machineWithoutKubeadmConfig-infra",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					Kind:       "KubeadmConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
					Namespace:  metav1.NamespaceDefault,
					Name:       "machineWithoutKubeadmConfig-bootstrap",
				},
			},
		},
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Name: "machineWithoutKubeadmConfig",
			},
		},
	}

	ownedMachines := collections.FromMachines(
		machineWithoutExpiryAnnotation,
		machineWithExpiryAnnotation,
		machineWithDeletionTimestamp,
		machineWithoutNodeRef,
		machineWithoutKubeadmConfig,
	)

	fakeClient := newFakeClient(
		machineWithoutExpiryAnnotationKubeadmConfig,
		machineWithExpiryAnnotationKubeadmConfig,
		machineWithDeletionTimestampKubeadmConfig,
		machineWithoutNodeRefKubeadmConfig,
	)

	managementCluster := &fakeManagementCluster{
		Workload: &fakeWorkloadCluster{
			APIServerCertificateExpiry: &detectedExpiry,
		},
	}

	r := &KubeadmControlPlaneReconciler{
		Client:              fakeClient,
		SecretCachingClient: fakeClient,
		managementCluster:   managementCluster,
	}

	controlPlane, err := internal.NewControlPlane(ctx, managementCluster, fakeClient, cluster, kcp, ownedMachines)
	g.Expect(err).ToNot(HaveOccurred())

	err = r.reconcileCertificateExpiries(ctx, controlPlane)
	g.Expect(err).ToNot(HaveOccurred())

	// Verify machineWithoutExpiryAnnotation has detectedExpiry.
	actualKubeadmConfig := bootstrapv1.KubeadmConfig{}
	err = fakeClient.Get(ctx, client.ObjectKeyFromObject(machineWithoutExpiryAnnotationKubeadmConfig), &actualKubeadmConfig)
	g.Expect(err).ToNot(HaveOccurred())
	actualExpiry := actualKubeadmConfig.Annotations[clusterv1.MachineCertificatesExpiryDateAnnotation]
	g.Expect(actualExpiry).To(Equal(detectedExpiry.Format(time.RFC3339)))

	// Verify machineWithExpiryAnnotation has still preExistingExpiry.
	err = fakeClient.Get(ctx, client.ObjectKeyFromObject(machineWithExpiryAnnotationKubeadmConfig), &actualKubeadmConfig)
	g.Expect(err).ToNot(HaveOccurred())
	actualExpiry = actualKubeadmConfig.Annotations[clusterv1.MachineCertificatesExpiryDateAnnotation]
	g.Expect(actualExpiry).To(Equal(preExistingExpiry.Format(time.RFC3339)))

	// Verify machineWithDeletionTimestamp has still no expiry annotation.
	err = fakeClient.Get(ctx, client.ObjectKeyFromObject(machineWithDeletionTimestampKubeadmConfig), &actualKubeadmConfig)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actualKubeadmConfig.Annotations).ToNot(ContainElement(clusterv1.MachineCertificatesExpiryDateAnnotation))

	// Verify machineWithoutNodeRef has still no expiry annotation.
	err = fakeClient.Get(ctx, client.ObjectKeyFromObject(machineWithoutNodeRefKubeadmConfig), &actualKubeadmConfig)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actualKubeadmConfig.Annotations).ToNot(ContainElement(clusterv1.MachineCertificatesExpiryDateAnnotation))
}

func TestReconcileInitializeControlPlane(t *testing.T) {
	setup := func(t *testing.T, g *WithT) *corev1.Namespace {
		t.Helper()

		t.Log("Creating the namespace")
		ns, err := env.CreateNamespace(ctx, "test-kcp-reconcile-initializecontrolplane")
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

	cluster := newCluster(&types.NamespacedName{Name: "foo", Namespace: namespace.Name})
	cluster.Spec = clusterv1.ClusterSpec{
		ControlPlaneEndpoint: clusterv1.APIEndpoint{
			Host: "test.local",
			Port: 9999,
		},
	}
	g.Expect(env.Create(ctx, cluster)).To(Succeed())
	patchHelper, err := patch.NewHelper(cluster, env)
	g.Expect(err).ToNot(HaveOccurred())
	cluster.Status = clusterv1.ClusterStatus{InfrastructureReady: true}
	g.Expect(patchHelper.Patch(ctx, cluster)).To(Succeed())

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

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      "foo",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
					Name:       cluster.Name,
					UID:        cluster.UID,
				},
			},
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Replicas: nil,
			Version:  "v1.16.6",
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       genericInfrastructureMachineTemplate.GetKind(),
					APIVersion: genericInfrastructureMachineTemplate.GetAPIVersion(),
					Name:       genericInfrastructureMachineTemplate.GetName(),
					Namespace:  cluster.Namespace,
				},
			},
			KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{},
		},
	}
	g.Expect(env.Create(ctx, kcp)).To(Succeed())

	corednsCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns",
			Namespace: namespace.Name,
		},
		Data: map[string]string{
			"Corefile": "original-core-file",
		},
	}
	g.Expect(env.Create(ctx, corednsCM)).To(Succeed())

	kubeadmCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubeadm-config",
			Namespace: namespace.Name,
		},
		Data: map[string]string{
			"ClusterConfiguration": `apiServer:
dns:
  type: CoreDNS
imageRepository: registry.k8s.io
kind: ClusterConfiguration
kubernetesVersion: metav1.16.1
`,
		},
	}
	g.Expect(env.Create(ctx, kubeadmCM)).To(Succeed())

	corednsDepl := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns",
			Namespace: namespace.Name,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"coredns": "",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: "coredns",
					Labels: map[string]string{
						"coredns": "",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "coredns",
						Image: "registry.k8s.io/coredns:1.6.2",
					}},
				},
			},
		},
	}
	g.Expect(env.Create(ctx, corednsDepl)).To(Succeed())

	expectedLabels := map[string]string{clusterv1.ClusterNameLabel: "foo"}
	r := &KubeadmControlPlaneReconciler{
		Client:              env,
		SecretCachingClient: secretCachingClient,
		recorder:            record.NewFakeRecorder(32),
		managementCluster: &fakeManagementCluster{
			Management: &internal.Management{Client: env},
			Workload: &fakeWorkloadCluster{
				Workload: &internal.Workload{
					Client: env,
				},
				Status: internal.ClusterStatus{},
			},
		},
		managementClusterUncached: &fakeManagementCluster{
			Management: &internal.Management{Client: env},
			Workload: &fakeWorkloadCluster{
				Workload: &internal.Workload{
					Client: env,
				},
				Status: internal.ClusterStatus{},
			},
		},
		ssaCache: ssa.NewCache(),
	}

	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(kcp)})
	g.Expect(err).ToNot(HaveOccurred())
	// this first requeue is to add finalizer
	g.Expect(result).To(BeComparableTo(ctrl.Result{}))
	g.Expect(env.GetAPIReader().Get(ctx, util.ObjectKey(kcp), kcp)).To(Succeed())
	g.Expect(kcp.Finalizers).To(ContainElement(controlplanev1.KubeadmControlPlaneFinalizer))

	g.Eventually(func(g Gomega) {
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(kcp)})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKey{Name: kcp.Name, Namespace: kcp.Namespace}, kcp)).To(Succeed())
		// Expect the referenced infrastructure template to have a Cluster Owner Reference.
		g.Expect(env.GetAPIReader().Get(ctx, util.ObjectKey(genericInfrastructureMachineTemplate), genericInfrastructureMachineTemplate)).To(Succeed())
		g.Expect(genericInfrastructureMachineTemplate.GetOwnerReferences()).To(ContainElement(metav1.OwnerReference{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
			Name:       cluster.Name,
			UID:        cluster.UID,
		}))

		// Always expect that the Finalizer is set on the passed in resource
		g.Expect(kcp.Finalizers).To(ContainElement(controlplanev1.KubeadmControlPlaneFinalizer))

		g.Expect(kcp.Status.Selector).NotTo(BeEmpty())
		g.Expect(kcp.Status.Replicas).To(BeEquivalentTo(1))
		g.Expect(conditions.IsFalse(kcp, controlplanev1.AvailableCondition)).To(BeTrue())

		s, err := secret.GetFromNamespacedName(ctx, env, client.ObjectKey{Namespace: cluster.Namespace, Name: "foo"}, secret.ClusterCA)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(s).NotTo(BeNil())
		g.Expect(s.Data).NotTo(BeEmpty())
		g.Expect(s.Labels).To(Equal(expectedLabels))

		k, err := kubeconfig.FromSecret(ctx, env, util.ObjectKey(cluster))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(k).NotTo(BeEmpty())

		machineList := &clusterv1.MachineList{}
		g.Expect(env.GetAPIReader().List(ctx, machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
		g.Expect(machineList.Items).To(HaveLen(1))

		machine := machineList.Items[0]
		g.Expect(machine.Name).To(HavePrefix(kcp.Name))
		// Newly cloned infra objects should have the infraref annotation.
		infraObj, err := external.Get(ctx, r.Client, &machine.Spec.InfrastructureRef, machine.Spec.InfrastructureRef.Namespace)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(infraObj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromNameAnnotation, genericInfrastructureMachineTemplate.GetName()))
		g.Expect(infraObj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromGroupKindAnnotation, genericInfrastructureMachineTemplate.GroupVersionKind().GroupKind().String()))
	}, 30*time.Second).Should(Succeed())
}

func TestReconcileInitializeControlPlane_withUserCA(t *testing.T) {
	setup := func(t *testing.T, g *WithT) *corev1.Namespace {
		t.Helper()

		t.Log("Creating the namespace")
		ns, err := env.CreateNamespace(ctx, "test-kcp-reconcile-initializecontrolplane")
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

	cluster := newCluster(&types.NamespacedName{Name: "foo", Namespace: namespace.Name})
	cluster.Spec = clusterv1.ClusterSpec{
		ControlPlaneEndpoint: clusterv1.APIEndpoint{
			Host: "test.local",
			Port: 9999,
		},
	}

	caCertificate := &secret.Certificate{
		Purpose:  secret.ClusterCA,
		CertFile: path.Join(secret.DefaultCertificatesDir, "ca.crt"),
		KeyFile:  path.Join(secret.DefaultCertificatesDir, "ca.key"),
	}
	// The certificate is user provided so no owner references should be added.
	g.Expect(caCertificate.Generate()).To(Succeed())
	certSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace.Name,
			Name:      cluster.Name + "-ca",
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: cluster.Name,
			},
		},
		Data: map[string][]byte{
			secret.TLSKeyDataName: caCertificate.KeyPair.Key,
			secret.TLSCrtDataName: caCertificate.KeyPair.Cert,
		},
		Type: clusterv1.ClusterSecretType,
	}

	g.Expect(env.Create(ctx, cluster)).To(Succeed())
	patchHelper, err := patch.NewHelper(cluster, env)
	g.Expect(err).ToNot(HaveOccurred())
	cluster.Status = clusterv1.ClusterStatus{InfrastructureReady: true}
	g.Expect(patchHelper.Patch(ctx, cluster)).To(Succeed())

	g.Expect(env.Create(ctx, certSecret)).To(Succeed())

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
	g.Expect(env.Create(ctx, genericInfrastructureMachineTemplate)).To(Succeed())

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      "foo",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
					Name:       cluster.Name,
					UID:        cluster.UID,
				},
			},
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Replicas: nil,
			Version:  "v1.16.6",
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       genericInfrastructureMachineTemplate.GetKind(),
					APIVersion: genericInfrastructureMachineTemplate.GetAPIVersion(),
					Name:       genericInfrastructureMachineTemplate.GetName(),
					Namespace:  cluster.Namespace,
				},
			},
			KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{},
		},
	}
	g.Expect(env.Create(ctx, kcp)).To(Succeed())

	corednsCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns",
			Namespace: namespace.Name,
		},
		Data: map[string]string{
			"Corefile": "original-core-file",
		},
	}
	g.Expect(env.Create(ctx, corednsCM)).To(Succeed())

	kubeadmCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubeadm-config",
			Namespace: namespace.Name,
		},
		Data: map[string]string{
			"ClusterConfiguration": `apiServer:
dns:
  type: CoreDNS
imageRepository: registry.k8s.io
kind: ClusterConfiguration
kubernetesVersion: metav1.16.1`,
		},
	}
	g.Expect(env.Create(ctx, kubeadmCM)).To(Succeed())

	corednsDepl := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns",
			Namespace: namespace.Name,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"coredns": "",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: "coredns",
					Labels: map[string]string{
						"coredns": "",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "coredns",
						Image: "registry.k8s.io/coredns:1.6.2",
					}},
				},
			},
		},
	}
	g.Expect(env.Create(ctx, corednsDepl)).To(Succeed())

	r := &KubeadmControlPlaneReconciler{
		Client:              env,
		SecretCachingClient: secretCachingClient,
		recorder:            record.NewFakeRecorder(32),
		managementCluster: &fakeManagementCluster{
			Management: &internal.Management{Client: env},
			Workload: &fakeWorkloadCluster{
				Workload: &internal.Workload{
					Client: env,
				},
				Status: internal.ClusterStatus{},
			},
		},
		managementClusterUncached: &fakeManagementCluster{
			Management: &internal.Management{Client: env},
			Workload: &fakeWorkloadCluster{
				Workload: &internal.Workload{
					Client: env,
				},
				Status: internal.ClusterStatus{},
			},
		},
		ssaCache: ssa.NewCache(),
	}

	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(kcp)})
	g.Expect(err).ToNot(HaveOccurred())
	// this first requeue is to add finalizer
	g.Expect(result).To(BeComparableTo(ctrl.Result{}))
	g.Expect(env.GetAPIReader().Get(ctx, util.ObjectKey(kcp), kcp)).To(Succeed())
	g.Expect(kcp.Finalizers).To(ContainElement(controlplanev1.KubeadmControlPlaneFinalizer))

	g.Eventually(func(g Gomega) {
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(kcp)})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKey{Name: kcp.Name, Namespace: kcp.Namespace}, kcp)).To(Succeed())
		// Expect the referenced infrastructure template to have a Cluster Owner Reference.
		g.Expect(env.GetAPIReader().Get(ctx, util.ObjectKey(genericInfrastructureMachineTemplate), genericInfrastructureMachineTemplate)).To(Succeed())
		g.Expect(genericInfrastructureMachineTemplate.GetOwnerReferences()).To(ContainElement(metav1.OwnerReference{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
			Name:       cluster.Name,
			UID:        cluster.UID,
		}))

		// Always expect that the Finalizer is set on the passed in resource
		g.Expect(kcp.Finalizers).To(ContainElement(controlplanev1.KubeadmControlPlaneFinalizer))

		g.Expect(kcp.Status.Selector).NotTo(BeEmpty())
		g.Expect(kcp.Status.Replicas).To(BeEquivalentTo(1))
		g.Expect(conditions.IsFalse(kcp, controlplanev1.AvailableCondition)).To(BeTrue())

		// Verify that the kubeconfig is using the custom CA
		kBytes, err := kubeconfig.FromSecret(ctx, env, util.ObjectKey(cluster))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(kBytes).NotTo(BeEmpty())
		k, err := clientcmd.Load(kBytes)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(k).NotTo(BeNil())
		g.Expect(k.Clusters[cluster.Name]).NotTo(BeNil())
		g.Expect(k.Clusters[cluster.Name].CertificateAuthorityData).To(Equal(caCertificate.KeyPair.Cert))

		machineList := &clusterv1.MachineList{}
		g.Expect(env.GetAPIReader().List(ctx, machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
		g.Expect(machineList.Items).To(HaveLen(1))

		machine := machineList.Items[0]
		g.Expect(machine.Name).To(HavePrefix(kcp.Name))
		// Newly cloned infra objects should have the infraref annotation.
		infraObj, err := external.Get(ctx, r.Client, &machine.Spec.InfrastructureRef, machine.Spec.InfrastructureRef.Namespace)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(infraObj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromNameAnnotation, genericInfrastructureMachineTemplate.GetName()))
		g.Expect(infraObj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromGroupKindAnnotation, genericInfrastructureMachineTemplate.GroupVersionKind().GroupKind().String()))
	}, 30*time.Second).Should(Succeed())
}

func TestKubeadmControlPlaneReconciler_syncMachines(t *testing.T) {
	setup := func(t *testing.T, g *WithT) (*corev1.Namespace, *clusterv1.Cluster) {
		t.Helper()

		t.Log("Creating the namespace")
		ns, err := env.CreateNamespace(ctx, "test-kcp-reconciler-sync-machines")
		g.Expect(err).ToNot(HaveOccurred())

		t.Log("Creating the Cluster")
		cluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Namespace: ns.Name, Name: "test-cluster"}}
		g.Expect(env.Create(ctx, cluster)).To(Succeed())

		t.Log("Creating the Cluster Kubeconfig Secret")
		g.Expect(env.CreateKubeconfigSecret(ctx, cluster)).To(Succeed())

		return ns, cluster
	}

	teardown := func(t *testing.T, g *WithT, ns *corev1.Namespace, cluster *clusterv1.Cluster) {
		t.Helper()

		t.Log("Deleting the Cluster")
		g.Expect(env.Delete(ctx, cluster)).To(Succeed())
		t.Log("Deleting the namespace")
		g.Expect(env.Delete(ctx, ns)).To(Succeed())
	}

	g := NewWithT(t)
	namespace, testCluster := setup(t, g)
	defer teardown(t, g, namespace, testCluster)

	classicManager := "manager"
	duration5s := &metav1.Duration{Duration: 5 * time.Second}
	duration10s := &metav1.Duration{Duration: 10 * time.Second}

	// Existing InfraMachine
	infraMachineSpec := map[string]interface{}{
		"infra-field": "infra-value",
	}
	existingInfraMachine := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericInfrastructureMachine",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"name":      "existing-inframachine",
				"namespace": testCluster.Namespace,
				"labels": map[string]string{
					"preserved-label": "preserved-value",
					"dropped-label":   "dropped-value",
					"modified-label":  "modified-value",
				},
				"annotations": map[string]string{
					"preserved-annotation": "preserved-value",
					"dropped-annotation":   "dropped-value",
					"modified-annotation":  "modified-value",
				},
			},
			"spec": infraMachineSpec,
		},
	}
	infraMachineRef := &corev1.ObjectReference{
		Kind:       "GenericInfrastructureMachine",
		Namespace:  namespace.Name,
		Name:       "existing-inframachine",
		APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
	}
	// Note: use "manager" as the field owner to mimic the manager used before ClusterAPI v1.4.0.
	g.Expect(env.Create(ctx, existingInfraMachine, client.FieldOwner("manager"))).To(Succeed())

	// Existing KubeadmConfig
	bootstrapSpec := &bootstrapv1.KubeadmConfigSpec{
		Users:             []bootstrapv1.User{{Name: "test-user"}},
		JoinConfiguration: &bootstrapv1.JoinConfiguration{},
	}
	existingKubeadmConfig := &bootstrapv1.KubeadmConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmConfig",
			APIVersion: bootstrapv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "existing-kubeadmconfig",
			Namespace: namespace.Name,
			Labels: map[string]string{
				"preserved-label": "preserved-value",
				"dropped-label":   "dropped-value",
				"modified-label":  "modified-value",
			},
			Annotations: map[string]string{
				"preserved-annotation": "preserved-value",
				"dropped-annotation":   "dropped-value",
				"modified-annotation":  "modified-value",
			},
		},
		Spec: *bootstrapSpec,
	}
	bootstrapRef := &corev1.ObjectReference{
		Kind:       "KubeadmConfig",
		Namespace:  namespace.Name,
		Name:       "existing-kubeadmconfig",
		APIVersion: bootstrapv1.GroupVersion.String(),
	}
	// Note: use "manager" as the field owner to mimic the manager used before ClusterAPI v1.4.0.
	g.Expect(env.Create(ctx, existingKubeadmConfig, client.FieldOwner("manager"))).To(Succeed())

	// Existing Machine to validate in-place mutation
	fd := ptr.To("fd1")
	inPlaceMutatingMachine := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Machine",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "existing-machine",
			Namespace: namespace.Name,
			Labels: map[string]string{
				"preserved-label": "preserved-value",
				"dropped-label":   "dropped-value",
				"modified-label":  "modified-value",
			},
			Annotations: map[string]string{
				"preserved-annotation": "preserved-value",
				"dropped-annotation":   "dropped-value",
				"modified-annotation":  "modified-value",
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: testCluster.Name,
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: bootstrapRef,
			},
			InfrastructureRef:       *infraMachineRef,
			Version:                 ptr.To("v1.25.3"),
			FailureDomain:           fd,
			ProviderID:              ptr.To("provider-id"),
			NodeDrainTimeout:        duration5s,
			NodeVolumeDetachTimeout: duration5s,
			NodeDeletionTimeout:     duration5s,
		},
	}
	// Note: use "manager" as the field owner to mimic the manager used before ClusterAPI v1.4.0.
	g.Expect(env.Create(ctx, inPlaceMutatingMachine, client.FieldOwner("manager"))).To(Succeed())

	// Existing machine that is in deleting state
	deletingMachine := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "deleting-machine",
			Namespace:   namespace.Name,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
			Finalizers:  []string{"testing-finalizer"},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: testCluster.Name,
			InfrastructureRef: corev1.ObjectReference{
				Namespace: namespace.Name,
			},
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: ptr.To("machine-bootstrap-secret"),
			},
			NodeDrainTimeout:        duration5s,
			NodeVolumeDetachTimeout: duration5s,
			NodeDeletionTimeout:     duration5s,
		},
	}
	g.Expect(env.Create(ctx, deletingMachine, client.FieldOwner(classicManager))).To(Succeed())
	// Delete the machine to put it in the deleting state
	g.Expect(env.Delete(ctx, deletingMachine)).To(Succeed())
	// Wait till the machine is marked for deletion
	g.Eventually(func() bool {
		if err := env.Get(ctx, client.ObjectKeyFromObject(deletingMachine), deletingMachine); err != nil {
			return false
		}
		return !deletingMachine.DeletionTimestamp.IsZero()
	}, 30*time.Second).Should(BeTrue())

	// Existing machine that has a InfrastructureRef which does not exist.
	nilInfraMachineMachine := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "nil-infra-machine-machine",
			Namespace:   namespace.Name,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
			Finalizers:  []string{"testing-finalizer"},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: testCluster.Name,
			InfrastructureRef: corev1.ObjectReference{
				Namespace: namespace.Name,
			},
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: ptr.To("machine-bootstrap-secret"),
			},
		},
	}
	g.Expect(env.Create(ctx, nilInfraMachineMachine, client.FieldOwner(classicManager))).To(Succeed())
	// Delete the machine to put it in the deleting state

	kcp := &controlplanev1.KubeadmControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "KubeadmControlPlane",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			UID:       types.UID("abc-123-control-plane"),
			Name:      "existing-kcp",
			Namespace: namespace.Name,
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version:           "v1.26.1",
			KubeadmConfigSpec: *bootstrapSpec,
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				ObjectMeta: clusterv1.ObjectMeta{
					Labels: map[string]string{
						"preserved-label": "preserved-value", // Label will be preserved while testing in-place mutation.
						"dropped-label":   "dropped-value",   // Label will be dropped while testing in-place mutation.
						"modified-label":  "modified-value",  // Label value will be modified while testing in-place mutation.
					},
					Annotations: map[string]string{
						"preserved-annotation": "preserved-value", // Annotation will be preserved while testing in-place mutation.
						"dropped-annotation":   "dropped-value",   // Annotation will be dropped while testing in-place mutation.
						"modified-annotation":  "modified-value",  // Annotation value will be modified while testing in-place mutation.
					},
				},
				InfrastructureRef: corev1.ObjectReference{
					Kind:       "GenericInfrastructureMachineTemplate",
					Namespace:  namespace.Name,
					Name:       "infra-foo",
					APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				},
				NodeDrainTimeout:        duration5s,
				NodeVolumeDetachTimeout: duration5s,
				NodeDeletionTimeout:     duration5s,
			},
		},
	}

	controlPlane := &internal.ControlPlane{
		KCP:     kcp,
		Cluster: testCluster,
		Machines: collections.Machines{
			inPlaceMutatingMachine.Name: inPlaceMutatingMachine.DeepCopy(),
			deletingMachine.Name:        deletingMachine.DeepCopy(),
			nilInfraMachineMachine.Name: nilInfraMachineMachine.DeepCopy(),
		},
		KubeadmConfigs: map[string]*bootstrapv1.KubeadmConfig{
			inPlaceMutatingMachine.Name: existingKubeadmConfig.DeepCopy(),
			deletingMachine.Name:        nil,
		},
		InfraResources: map[string]*unstructured.Unstructured{
			inPlaceMutatingMachine.Name: existingInfraMachine.DeepCopy(),
			deletingMachine.Name:        nil,
		},
	}

	//
	// Verify Managed Fields
	//

	// Run syncMachines to clean up managed fields and have proper field ownership
	// for Machines, InfrastructureMachines and KubeadmConfigs.
	reconciler := &KubeadmControlPlaneReconciler{
		Client:              env,
		SecretCachingClient: secretCachingClient,
		ssaCache:            ssa.NewCache(),
	}
	g.Expect(reconciler.syncMachines(ctx, controlPlane)).To(Succeed())

	// The inPlaceMutatingMachine should have cleaned up managed fields.
	updatedInplaceMutatingMachine := inPlaceMutatingMachine.DeepCopy()
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedInplaceMutatingMachine), updatedInplaceMutatingMachine)).To(Succeed())
	// Verify ManagedFields
	g.Expect(updatedInplaceMutatingMachine.ManagedFields).Should(
		ContainElement(ssa.MatchManagedFieldsEntry(kcpManagerName, metav1.ManagedFieldsOperationApply)),
		"in-place mutable machine should contain an entry for SSA manager",
	)
	g.Expect(updatedInplaceMutatingMachine.ManagedFields).ShouldNot(
		ContainElement(ssa.MatchManagedFieldsEntry(classicManager, metav1.ManagedFieldsOperationUpdate)),
		"in-place mutable machine should not contain an entry for old manager",
	)

	// The InfrastructureMachine should have ownership of "labels" and "annotations" transferred to
	// "capi-kubeadmcontrolplane" manager.
	updatedInfraMachine := existingInfraMachine.DeepCopy()
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedInfraMachine), updatedInfraMachine)).To(Succeed())

	// Verify ManagedFields
	g.Expect(updatedInfraMachine.GetManagedFields()).Should(
		ssa.MatchFieldOwnership(kcpManagerName, metav1.ManagedFieldsOperationApply, contract.Path{"f:metadata", "f:labels"}))
	g.Expect(updatedInfraMachine.GetManagedFields()).Should(
		ssa.MatchFieldOwnership(kcpManagerName, metav1.ManagedFieldsOperationApply, contract.Path{"f:metadata", "f:annotations"}))
	g.Expect(updatedInfraMachine.GetManagedFields()).ShouldNot(
		ssa.MatchFieldOwnership(classicManager, metav1.ManagedFieldsOperationUpdate, contract.Path{"f:metadata", "f:labels"}))
	g.Expect(updatedInfraMachine.GetManagedFields()).ShouldNot(
		ssa.MatchFieldOwnership(classicManager, metav1.ManagedFieldsOperationUpdate, contract.Path{"f:metadata", "f:annotations"}))
	// Verify ownership of other fields is not changed.
	g.Expect(updatedInfraMachine.GetManagedFields()).Should(
		ssa.MatchFieldOwnership(classicManager, metav1.ManagedFieldsOperationUpdate, contract.Path{"f:spec"}))

	// The KubeadmConfig should have ownership of "labels" and "annotations" transferred to
	// "capi-kubeadmcontrolplane" manager.
	updatedKubeadmConfig := existingKubeadmConfig.DeepCopy()
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedKubeadmConfig), updatedKubeadmConfig)).To(Succeed())

	// Verify ManagedFields
	g.Expect(updatedKubeadmConfig.GetManagedFields()).Should(
		ssa.MatchFieldOwnership(kcpManagerName, metav1.ManagedFieldsOperationApply, contract.Path{"f:metadata", "f:labels"}))
	g.Expect(updatedKubeadmConfig.GetManagedFields()).Should(
		ssa.MatchFieldOwnership(kcpManagerName, metav1.ManagedFieldsOperationApply, contract.Path{"f:metadata", "f:annotations"}))
	g.Expect(updatedKubeadmConfig.GetManagedFields()).ShouldNot(
		ssa.MatchFieldOwnership(classicManager, metav1.ManagedFieldsOperationUpdate, contract.Path{"f:metadata", "f:labels"}))
	g.Expect(updatedKubeadmConfig.GetManagedFields()).ShouldNot(
		ssa.MatchFieldOwnership(classicManager, metav1.ManagedFieldsOperationUpdate, contract.Path{"f:metadata", "f:annotations"}))
	// Verify ownership of other fields is not changed.
	g.Expect(updatedKubeadmConfig.GetManagedFields()).Should(
		ssa.MatchFieldOwnership(classicManager, metav1.ManagedFieldsOperationUpdate, contract.Path{"f:spec"}))

	//
	// Verify In-place mutating fields
	//

	// Update KCP and verify the in-place mutating fields are propagated.
	kcp.Spec.MachineTemplate.ObjectMeta.Labels = map[string]string{
		"preserved-label": "preserved-value",  // Keep the label and value as is
		"modified-label":  "modified-value-2", // Modify the value of the label
		// Drop "dropped-label"
	}
	expectedLabels := map[string]string{
		"preserved-label":                      "preserved-value",
		"modified-label":                       "modified-value-2",
		clusterv1.ClusterNameLabel:             testCluster.Name,
		clusterv1.MachineControlPlaneLabel:     "",
		clusterv1.MachineControlPlaneNameLabel: kcp.Name,
	}
	kcp.Spec.MachineTemplate.ObjectMeta.Annotations = map[string]string{
		"preserved-annotation": "preserved-value",  // Keep the annotation and value as is
		"modified-annotation":  "modified-value-2", // Modify the value of the annotation
		// Drop "dropped-annotation"
	}
	kcp.Spec.MachineTemplate.NodeDrainTimeout = duration10s
	kcp.Spec.MachineTemplate.NodeDeletionTimeout = duration10s
	kcp.Spec.MachineTemplate.NodeVolumeDetachTimeout = duration10s

	// Use the updated KCP.
	controlPlane.KCP = kcp
	g.Expect(reconciler.syncMachines(ctx, controlPlane)).To(Succeed())

	// Verify in-place mutable fields are updated on the Machine.
	updatedInplaceMutatingMachine = inPlaceMutatingMachine.DeepCopy()
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedInplaceMutatingMachine), updatedInplaceMutatingMachine)).To(Succeed())
	// Verify Labels
	g.Expect(updatedInplaceMutatingMachine.Labels).Should(Equal(expectedLabels))
	// Verify Annotations
	expectedAnnotations := map[string]string{}
	for k, v := range kcp.Spec.MachineTemplate.ObjectMeta.Annotations {
		expectedAnnotations[k] = v
	}
	// The pre-terminate annotation should always be added
	expectedAnnotations[controlplanev1.PreTerminateHookCleanupAnnotation] = ""
	g.Expect(updatedInplaceMutatingMachine.Annotations).Should(Equal(expectedAnnotations))
	// Verify Node timeout values
	g.Expect(updatedInplaceMutatingMachine.Spec.NodeDrainTimeout).Should(And(
		Not(BeNil()),
		HaveValue(BeComparableTo(*kcp.Spec.MachineTemplate.NodeDrainTimeout)),
	))
	g.Expect(updatedInplaceMutatingMachine.Spec.NodeDeletionTimeout).Should(And(
		Not(BeNil()),
		HaveValue(BeComparableTo(*kcp.Spec.MachineTemplate.NodeDeletionTimeout)),
	))
	g.Expect(updatedInplaceMutatingMachine.Spec.NodeVolumeDetachTimeout).Should(And(
		Not(BeNil()),
		HaveValue(BeComparableTo(*kcp.Spec.MachineTemplate.NodeVolumeDetachTimeout)),
	))
	// Verify that the non in-place mutating fields remain the same.
	g.Expect(updatedInplaceMutatingMachine.Spec.FailureDomain).Should(Equal(inPlaceMutatingMachine.Spec.FailureDomain))
	g.Expect(updatedInplaceMutatingMachine.Spec.ProviderID).Should(Equal(inPlaceMutatingMachine.Spec.ProviderID))
	g.Expect(updatedInplaceMutatingMachine.Spec.Version).Should(Equal(inPlaceMutatingMachine.Spec.Version))
	g.Expect(updatedInplaceMutatingMachine.Spec.InfrastructureRef).Should(BeComparableTo(inPlaceMutatingMachine.Spec.InfrastructureRef))
	g.Expect(updatedInplaceMutatingMachine.Spec.Bootstrap).Should(BeComparableTo(inPlaceMutatingMachine.Spec.Bootstrap))

	// Verify in-place mutable fields are updated on InfrastructureMachine
	updatedInfraMachine = existingInfraMachine.DeepCopy()
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedInfraMachine), updatedInfraMachine)).To(Succeed())
	// Verify Labels
	g.Expect(updatedInfraMachine.GetLabels()).Should(Equal(expectedLabels))
	// Verify Annotations
	g.Expect(updatedInfraMachine.GetAnnotations()).Should(Equal(kcp.Spec.MachineTemplate.ObjectMeta.Annotations))
	// Verify spec remains the same
	g.Expect(updatedInfraMachine.Object).Should(HaveKeyWithValue("spec", infraMachineSpec))

	// Verify in-place mutable fields are updated on the KubeadmConfig.
	updatedKubeadmConfig = existingKubeadmConfig.DeepCopy()
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedKubeadmConfig), updatedKubeadmConfig)).To(Succeed())
	// Verify Labels
	g.Expect(updatedKubeadmConfig.GetLabels()).Should(Equal(expectedLabels))
	// Verify Annotations
	g.Expect(updatedKubeadmConfig.GetAnnotations()).Should(Equal(kcp.Spec.MachineTemplate.ObjectMeta.Annotations))
	// Verify spec remains the same
	g.Expect(updatedKubeadmConfig.Spec).Should(BeComparableTo(existingKubeadmConfig.Spec))

	// The deleting machine should not change.
	updatedDeletingMachine := deletingMachine.DeepCopy()
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedDeletingMachine), updatedDeletingMachine)).To(Succeed())

	// Verify ManagedFields
	g.Expect(updatedDeletingMachine.ManagedFields).ShouldNot(
		ContainElement(ssa.MatchManagedFieldsEntry(kcpManagerName, metav1.ManagedFieldsOperationApply)),
		"deleting machine should not contain an entry for SSA manager",
	)
	g.Expect(updatedDeletingMachine.ManagedFields).Should(
		ContainElement(ssa.MatchManagedFieldsEntry("manager", metav1.ManagedFieldsOperationUpdate)),
		"in-place mutable machine should still contain an entry for old manager",
	)

	// Verify the machine labels and annotations are unchanged.
	g.Expect(updatedDeletingMachine.Labels).Should(Equal(deletingMachine.Labels))
	g.Expect(updatedDeletingMachine.Annotations).Should(Equal(deletingMachine.Annotations))
	// Verify Node timeout values
	g.Expect(updatedDeletingMachine.Spec.NodeDrainTimeout).Should(Equal(kcp.Spec.MachineTemplate.NodeDrainTimeout))
	g.Expect(updatedDeletingMachine.Spec.NodeDeletionTimeout).Should(Equal(kcp.Spec.MachineTemplate.NodeDeletionTimeout))
	g.Expect(updatedDeletingMachine.Spec.NodeVolumeDetachTimeout).Should(Equal(kcp.Spec.MachineTemplate.NodeVolumeDetachTimeout))
	// Verify the machine spec is otherwise unchanged.
	deletingMachine.Spec.NodeDrainTimeout = kcp.Spec.MachineTemplate.NodeDrainTimeout
	deletingMachine.Spec.NodeDeletionTimeout = kcp.Spec.MachineTemplate.NodeDeletionTimeout
	deletingMachine.Spec.NodeVolumeDetachTimeout = kcp.Spec.MachineTemplate.NodeVolumeDetachTimeout
	g.Expect(updatedDeletingMachine.Spec).Should(BeComparableTo(deletingMachine.Spec))
}

func TestKubeadmControlPlaneReconciler_reconcilePreTerminateHook(t *testing.T) {
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
	}
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machine",
			Annotations: map[string]string{
				controlplanev1.PreTerminateHookCleanupAnnotation: "",
			},
		},
	}
	deletingMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "deleting-machine",
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
			Finalizers:        []string{clusterv1.MachineFinalizer},
		},
	}
	deletingMachineWithKCPPreTerminateHook := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "deleting-machine-with-kcp-pre-terminate-hook",
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
			Finalizers:        []string{clusterv1.MachineFinalizer},
			Annotations: map[string]string{
				controlplanev1.PreTerminateHookCleanupAnnotation: "",
			},
		},
	}
	deletingMachineWithKCPAndOtherPreTerminateHooksOld := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "deleting-machine-with-kcp-and-other-pre-terminate-hooks",
			DeletionTimestamp: &metav1.Time{Time: time.Now().Add(-1 * time.Duration(1) * time.Minute)},
			Finalizers:        []string{clusterv1.MachineFinalizer},
			Annotations: map[string]string{
				controlplanev1.PreTerminateHookCleanupAnnotation:           "",
				clusterv1.PreTerminateDeleteHookAnnotationPrefix + "/test": "",
			},
		},
	}

	tests := []struct {
		name                                 string
		controlPlane                         *internal.ControlPlane
		wantResult                           ctrl.Result
		wantErr                              string
		wantForwardEtcdLeadershipCalled      int
		wantRemoveEtcdMemberForMachineCalled int
		wantMachineAnnotations               map[string]map[string]string
	}{
		{
			name: "Do nothing if there are no deleting Machines",
			controlPlane: &internal.ControlPlane{
				Machines: collections.Machines{
					machine.Name: machine,
				},
			},
			wantResult: ctrl.Result{},
			// Annotation are unchanged.
			wantMachineAnnotations: map[string]map[string]string{
				machine.Name: machine.Annotations,
			},
		},
		{
			name: "Requeue, if there is a deleting Machine without the KCP pre-terminate hook",
			controlPlane: &internal.ControlPlane{
				Machines: collections.Machines{
					deletingMachine.Name:                                    deletingMachine, // Does not have the pre-terminate hook anymore.
					deletingMachineWithKCPPreTerminateHook.Name:             deletingMachineWithKCPPreTerminateHook,
					deletingMachineWithKCPAndOtherPreTerminateHooksOld.Name: deletingMachineWithKCPAndOtherPreTerminateHooksOld,
				},
			},
			wantResult: ctrl.Result{RequeueAfter: deleteRequeueAfter},
			// Annotation are unchanged.
			wantMachineAnnotations: map[string]map[string]string{
				deletingMachine.Name:                                    deletingMachine.Annotations,
				deletingMachineWithKCPPreTerminateHook.Name:             deletingMachineWithKCPPreTerminateHook.Annotations,
				deletingMachineWithKCPAndOtherPreTerminateHooksOld.Name: deletingMachineWithKCPAndOtherPreTerminateHooksOld.Annotations,
			},
		},
		{
			name: "Requeue, if the oldest deleting Machine has other pre-terminate hooks with Kubernetes 1.31",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec: controlplanev1.KubeadmControlPlaneSpec{
						Version: "v1.31.0",
					},
				},
				Machines: collections.Machines{
					deletingMachineWithKCPPreTerminateHook.Name:             deletingMachineWithKCPPreTerminateHook,
					deletingMachineWithKCPAndOtherPreTerminateHooksOld.Name: deletingMachineWithKCPAndOtherPreTerminateHooksOld,
				},
			},
			wantResult: ctrl.Result{RequeueAfter: deleteRequeueAfter},
			// Annotation are unchanged.
			wantMachineAnnotations: map[string]map[string]string{
				deletingMachineWithKCPPreTerminateHook.Name:             deletingMachineWithKCPPreTerminateHook.Annotations,
				deletingMachineWithKCPAndOtherPreTerminateHooksOld.Name: deletingMachineWithKCPAndOtherPreTerminateHooksOld.Annotations,
			},
		},
		{
			name: "Requeue, if the deleting Machine has no PreTerminateDeleteHookSucceeded condition",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec: controlplanev1.KubeadmControlPlaneSpec{
						Version: "v1.31.0",
					},
				},
				Machines: collections.Machines{
					deletingMachineWithKCPPreTerminateHook.Name: deletingMachineWithKCPPreTerminateHook,
				},
			},
			wantResult: ctrl.Result{RequeueAfter: deleteRequeueAfter},
			// Annotation are unchanged.
			wantMachineAnnotations: map[string]map[string]string{
				deletingMachineWithKCPPreTerminateHook.Name: deletingMachineWithKCPPreTerminateHook.Annotations,
			},
		},
		{
			name: "Requeue, if the deleting Machine has PreTerminateDeleteHookSucceeded condition true",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec: controlplanev1.KubeadmControlPlaneSpec{
						Version: "v1.31.0",
					},
				},
				Machines: collections.Machines{
					deletingMachineWithKCPPreTerminateHook.Name: func() *clusterv1.Machine {
						m := deletingMachineWithKCPPreTerminateHook.DeepCopy()
						conditions.MarkTrue(m, clusterv1.PreTerminateDeleteHookSucceededCondition)
						return m
					}(),
				},
			},
			wantResult: ctrl.Result{RequeueAfter: deleteRequeueAfter},
			// Annotation are unchanged.
			wantMachineAnnotations: map[string]map[string]string{
				deletingMachineWithKCPPreTerminateHook.Name: deletingMachineWithKCPPreTerminateHook.Annotations,
			},
		},
		{
			name: "Requeue, if the deleting Machine has PreTerminateDeleteHookSucceeded condition false but not waiting for hook",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec: controlplanev1.KubeadmControlPlaneSpec{
						Version: "v1.31.0",
					},
				},
				Machines: collections.Machines{
					deletingMachineWithKCPPreTerminateHook.Name: func() *clusterv1.Machine {
						m := deletingMachineWithKCPPreTerminateHook.DeepCopy()
						conditions.MarkFalse(m, clusterv1.PreTerminateDeleteHookSucceededCondition, "some-other-reason", clusterv1.ConditionSeverityInfo, "some message")
						return m
					}(),
				},
			},
			wantResult: ctrl.Result{RequeueAfter: deleteRequeueAfter},
			// Annotation are unchanged.
			wantMachineAnnotations: map[string]map[string]string{
				deletingMachineWithKCPPreTerminateHook.Name: deletingMachineWithKCPPreTerminateHook.Annotations,
			},
		},
		{
			name: "Forward etcd leadership, remove member and remove pre-terminate hook if > 1 CP Machines && Etcd is managed",
			controlPlane: &internal.ControlPlane{
				Cluster: cluster,
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec: controlplanev1.KubeadmControlPlaneSpec{
						Version: "v1.31.0",
					},
				},
				Machines: collections.Machines{
					machine.Name: machine, // Leadership will be forwarded to this Machine.
					deletingMachineWithKCPPreTerminateHook.Name: func() *clusterv1.Machine {
						m := deletingMachineWithKCPPreTerminateHook.DeepCopy()
						conditions.MarkFalse(m, clusterv1.PreTerminateDeleteHookSucceededCondition, clusterv1.WaitingExternalHookReason, clusterv1.ConditionSeverityInfo, "some message")
						return m
					}(),
				},
			},
			wantForwardEtcdLeadershipCalled:      1,
			wantRemoveEtcdMemberForMachineCalled: 1,
			wantResult:                           ctrl.Result{RequeueAfter: deleteRequeueAfter},
			wantMachineAnnotations: map[string]map[string]string{
				machine.Name: machine.Annotations, // unchanged
				deletingMachineWithKCPPreTerminateHook.Name: nil, // pre-terminate hook has been removed
			},
		},
		{
			name: "Skip forward etcd leadership (no other non-deleting Machine), remove member and remove pre-terminate hook if > 1 CP Machines && Etcd is managed",
			controlPlane: &internal.ControlPlane{
				Cluster: cluster,
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec: controlplanev1.KubeadmControlPlaneSpec{
						Version: "v1.31.0",
					},
				},
				Machines: collections.Machines{
					deletingMachineWithKCPPreTerminateHook.Name: func() *clusterv1.Machine {
						m := deletingMachineWithKCPPreTerminateHook.DeepCopy()
						m.DeletionTimestamp.Time = m.DeletionTimestamp.Add(-1 * time.Duration(1) * time.Second) // Make sure this (the oldest) Machine is selected to run the pre-terminate hook.
						conditions.MarkFalse(m, clusterv1.PreTerminateDeleteHookSucceededCondition, clusterv1.WaitingExternalHookReason, clusterv1.ConditionSeverityInfo, "some message")
						return m
					}(),
					deletingMachineWithKCPPreTerminateHook.Name + "-2": func() *clusterv1.Machine {
						m := deletingMachineWithKCPPreTerminateHook.DeepCopy()
						m.Name += "-2"
						conditions.MarkFalse(m, clusterv1.PreTerminateDeleteHookSucceededCondition, clusterv1.WaitingExternalHookReason, clusterv1.ConditionSeverityInfo, "some message")
						return m
					}(),
				},
			},
			wantForwardEtcdLeadershipCalled:      0, // skipped as there is no non-deleting Machine to forward to.
			wantRemoveEtcdMemberForMachineCalled: 1,
			wantResult:                           ctrl.Result{RequeueAfter: deleteRequeueAfter},
			wantMachineAnnotations: map[string]map[string]string{
				deletingMachineWithKCPPreTerminateHook.Name:        nil,                                                // pre-terminate hook has been removed
				deletingMachineWithKCPPreTerminateHook.Name + "-2": deletingMachineWithKCPPreTerminateHook.Annotations, // unchanged
			},
		},
		{
			name: "Skip forward etcd leadership, skip remove member and remove pre-terminate hook if 1 CP Machine && Etcd is managed",
			controlPlane: &internal.ControlPlane{
				Cluster: cluster,
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec: controlplanev1.KubeadmControlPlaneSpec{
						Version: "v1.31.0",
					},
				},
				Machines: collections.Machines{
					deletingMachineWithKCPPreTerminateHook.Name: func() *clusterv1.Machine {
						m := deletingMachineWithKCPPreTerminateHook.DeepCopy()
						conditions.MarkFalse(m, clusterv1.PreTerminateDeleteHookSucceededCondition, clusterv1.WaitingExternalHookReason, clusterv1.ConditionSeverityInfo, "some message")
						return m
					}(),
				},
			},
			wantForwardEtcdLeadershipCalled:      0, // skipped
			wantRemoveEtcdMemberForMachineCalled: 0, // skipped
			wantResult:                           ctrl.Result{RequeueAfter: deleteRequeueAfter},
			wantMachineAnnotations: map[string]map[string]string{
				deletingMachineWithKCPPreTerminateHook.Name: nil, // pre-terminate hook has been removed
			},
		},
		{
			name: "Skip forward etcd leadership, skip remove member and remove pre-terminate hook if > 1 CP Machines && Etcd is *not* managed",
			controlPlane: &internal.ControlPlane{
				Cluster: cluster,
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec: controlplanev1.KubeadmControlPlaneSpec{
						Version: "v1.31.0",
						KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
							ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
								Etcd: bootstrapv1.Etcd{
									External: &bootstrapv1.ExternalEtcd{
										Endpoints: []string{"1.2.3.4"}, // Etcd is not managed by KCP
									},
								},
							},
						},
					},
				},
				Machines: collections.Machines{
					machine.Name: machine,
					deletingMachineWithKCPPreTerminateHook.Name: func() *clusterv1.Machine {
						m := deletingMachineWithKCPPreTerminateHook.DeepCopy()
						conditions.MarkFalse(m, clusterv1.PreTerminateDeleteHookSucceededCondition, clusterv1.WaitingExternalHookReason, clusterv1.ConditionSeverityInfo, "some message")
						return m
					}(),
				},
			},
			wantForwardEtcdLeadershipCalled:      0, // skipped
			wantRemoveEtcdMemberForMachineCalled: 0, // skipped
			wantResult:                           ctrl.Result{RequeueAfter: deleteRequeueAfter},
			wantMachineAnnotations: map[string]map[string]string{
				machine.Name: machine.Annotations, // unchanged
				deletingMachineWithKCPPreTerminateHook.Name: nil, // pre-terminate hook has been removed
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			objs := []client.Object{}
			for _, m := range tt.controlPlane.Machines {
				objs = append(objs, m)
			}
			fakeClient := fake.NewClientBuilder().WithObjects(objs...).Build()

			r := &KubeadmControlPlaneReconciler{
				Client: fakeClient,
			}

			workloadCluster := fakeWorkloadCluster{}
			tt.controlPlane.InjectTestManagementCluster(&fakeManagementCluster{
				Workload: &workloadCluster,
			})

			res, err := r.reconcilePreTerminateHook(ctx, tt.controlPlane)
			if tt.wantErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(err.Error()))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(res).To(Equal(tt.wantResult))

			g.Expect(workloadCluster.forwardEtcdLeadershipCalled).To(Equal(tt.wantForwardEtcdLeadershipCalled))
			g.Expect(workloadCluster.removeEtcdMemberForMachineCalled).To(Equal(tt.wantRemoveEtcdMemberForMachineCalled))

			machineList := &clusterv1.MachineList{}
			g.Expect(fakeClient.List(ctx, machineList)).To(Succeed())
			g.Expect(machineList.Items).To(HaveLen(len(tt.wantMachineAnnotations)))
			for _, machine := range machineList.Items {
				g.Expect(machine.Annotations).To(BeComparableTo(tt.wantMachineAnnotations[machine.Name]), "Unexpected annotations for Machine %s", machine.Name)
			}
		})
	}
}

func TestKubeadmControlPlaneReconciler_machineHasOtherPreTerminateHooks(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{
			name: "only KCP pre-terminate hook",
			annotations: map[string]string{
				controlplanev1.PreTerminateHookCleanupAnnotation: "",
				"some-other-annotation":                          "",
			},
			want: false,
		},
		{
			name: "KCP & additional pre-terminate hooks",
			annotations: map[string]string{
				controlplanev1.PreTerminateHookCleanupAnnotation:           "",
				"some-other-annotation":                                    "",
				clusterv1.PreTerminateDeleteHookAnnotationPrefix + "test":  "",
				clusterv1.PreTerminateDeleteHookAnnotationPrefix + "/test": "",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			m := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.annotations,
				},
			}
			g.Expect(machineHasOtherPreTerminateHooks(m)).To(Equal(tt.want))
		})
	}
}

func TestKubeadmControlPlaneReconciler_updateCoreDNS(t *testing.T) {
	// TODO: (wfernandes) This test could use some refactor love.

	cluster := newCluster(&types.NamespacedName{Name: "foo", Namespace: metav1.NamespaceDefault})
	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      "foo",
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Replicas: nil,
			Version:  "v1.16.6",
			KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
					DNS: bootstrapv1.DNS{
						ImageMeta: bootstrapv1.ImageMeta{
							ImageRepository: "registry.k8s.io",
							ImageTag:        "1.7.2",
						},
					},
				},
			},
		},
	}
	depl := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns",
			Namespace: metav1.NamespaceSystem,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: "coredns",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "coredns",
						Image: "registry.k8s.io/coredns:1.6.2",
					}},
					Volumes: []corev1.Volume{{
						Name: "config-volume",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "coredns",
								},
								Items: []corev1.KeyToPath{{
									Key:  "Corefile",
									Path: "Corefile",
								}},
							},
						},
					}},
				},
			},
		},
	}
	originalCorefile := "original core file"
	corednsCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns",
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			"Corefile": originalCorefile,
		},
	}

	kubeadmCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubeadm-config",
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			"ClusterConfiguration": `apiServer:
dns:
  type: CoreDNS
imageRepository: registry.k8s.io
kind: ClusterConfiguration
kubernetesVersion: metav1.16.1`,
		},
	}

	t.Run("updates configmaps and deployments successfully", func(t *testing.T) {
		t.Skip("Updating the corefile, after updating controller runtime somehow makes this test fail in a conflict, needs investigation")

		g := NewWithT(t)
		objs := []client.Object{
			cluster.DeepCopy(),
			kcp.DeepCopy(),
			depl.DeepCopy(),
			corednsCM.DeepCopy(),
			kubeadmCM.DeepCopy(),
		}
		fakeClient := newFakeClient(objs...)

		workloadCluster := &fakeWorkloadCluster{
			Workload: &internal.Workload{
				Client: fakeClient,
				CoreDNSMigrator: &fakeMigrator{
					migratedCorefile: "new core file",
				},
			},
		}

		g.Expect(workloadCluster.UpdateCoreDNS(ctx, kcp, semver.MustParse("1.19.1"))).To(Succeed())

		var actualCoreDNSCM corev1.ConfigMap
		g.Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "coredns", Namespace: metav1.NamespaceSystem}, &actualCoreDNSCM)).To(Succeed())
		g.Expect(actualCoreDNSCM.Data).To(HaveLen(2))
		g.Expect(actualCoreDNSCM.Data).To(HaveKeyWithValue("Corefile", "new core file"))
		g.Expect(actualCoreDNSCM.Data).To(HaveKeyWithValue("Corefile-backup", originalCorefile))

		var actualKubeadmConfig corev1.ConfigMap
		g.Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "kubeadm-config", Namespace: metav1.NamespaceSystem}, &actualKubeadmConfig)).To(Succeed())
		g.Expect(actualKubeadmConfig.Data).To(HaveKey("ClusterConfiguration"))
		g.Expect(actualKubeadmConfig.Data["ClusterConfiguration"]).To(ContainSubstring("1.7.2"))

		expectedVolume := corev1.Volume{
			Name: "config-volume",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "coredns",
					},
					Items: []corev1.KeyToPath{{
						Key:  "Corefile",
						Path: "Corefile",
					}},
				},
			},
		}
		var actualCoreDNSDeployment appsv1.Deployment
		g.Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "coredns", Namespace: metav1.NamespaceSystem}, &actualCoreDNSDeployment)).To(Succeed())
		g.Expect(actualCoreDNSDeployment.Spec.Template.Spec.Containers[0].Image).To(Equal("registry.k8s.io/coredns:1.7.2"))
		g.Expect(actualCoreDNSDeployment.Spec.Template.Spec.Volumes).To(ConsistOf(expectedVolume))
	})

	t.Run("returns no error when no ClusterConfiguration is specified", func(t *testing.T) {
		g := NewWithT(t)
		kcp := kcp.DeepCopy()
		kcp.Spec.KubeadmConfigSpec.ClusterConfiguration = nil

		objs := []client.Object{
			cluster.DeepCopy(),
			kcp,
			depl.DeepCopy(),
			corednsCM.DeepCopy(),
			kubeadmCM.DeepCopy(),
		}

		fakeClient := newFakeClient(objs...)

		workloadCluster := fakeWorkloadCluster{
			Workload: &internal.Workload{
				Client: fakeClient,
				CoreDNSMigrator: &fakeMigrator{
					migratedCorefile: "new core file",
				},
			},
		}

		g.Expect(workloadCluster.UpdateCoreDNS(ctx, kcp, semver.MustParse("1.19.1"))).To(Succeed())
	})

	t.Run("should not return an error when there is no CoreDNS configmap", func(t *testing.T) {
		g := NewWithT(t)
		objs := []client.Object{
			cluster.DeepCopy(),
			kcp.DeepCopy(),
			depl.DeepCopy(),
			kubeadmCM.DeepCopy(),
		}

		fakeClient := newFakeClient(objs...)
		workloadCluster := fakeWorkloadCluster{
			Workload: &internal.Workload{
				Client: fakeClient,
				CoreDNSMigrator: &fakeMigrator{
					migratedCorefile: "new core file",
				},
			},
		}

		g.Expect(workloadCluster.UpdateCoreDNS(ctx, kcp, semver.MustParse("1.19.1"))).To(Succeed())
	})

	t.Run("should not return an error when there is no CoreDNS deployment", func(t *testing.T) {
		g := NewWithT(t)
		objs := []client.Object{
			cluster.DeepCopy(),
			kcp.DeepCopy(),
			corednsCM.DeepCopy(),
			kubeadmCM.DeepCopy(),
		}

		fakeClient := newFakeClient(objs...)

		workloadCluster := fakeWorkloadCluster{
			Workload: &internal.Workload{
				Client: fakeClient,
				CoreDNSMigrator: &fakeMigrator{
					migratedCorefile: "new core file",
				},
			},
		}

		g.Expect(workloadCluster.UpdateCoreDNS(ctx, kcp, semver.MustParse("1.19.1"))).To(Succeed())
	})

	t.Run("should not return an error when no DNS upgrade is requested", func(t *testing.T) {
		g := NewWithT(t)
		objs := []client.Object{
			cluster.DeepCopy(),
			corednsCM.DeepCopy(),
			kubeadmCM.DeepCopy(),
		}
		kcp := kcp.DeepCopy()
		kcp.Annotations = map[string]string{controlplanev1.SkipCoreDNSAnnotation: ""}

		depl := depl.DeepCopy()

		depl.Spec.Template.Spec.Containers[0].Image = "my-cool-image!!!!" // something very unlikely for getCoreDNSInfo to parse
		objs = append(objs, depl)

		fakeClient := newFakeClient(objs...)
		workloadCluster := fakeWorkloadCluster{
			Workload: &internal.Workload{
				Client: fakeClient,
			},
		}

		g.Expect(workloadCluster.UpdateCoreDNS(ctx, kcp, semver.MustParse("1.19.1"))).To(Succeed())

		var actualCoreDNSCM corev1.ConfigMap
		g.Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "coredns", Namespace: metav1.NamespaceSystem}, &actualCoreDNSCM)).To(Succeed())
		g.Expect(actualCoreDNSCM.Data).To(Equal(corednsCM.Data))

		var actualKubeadmConfig corev1.ConfigMap
		g.Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "kubeadm-config", Namespace: metav1.NamespaceSystem}, &actualKubeadmConfig)).To(Succeed())
		g.Expect(actualKubeadmConfig.Data).To(Equal(kubeadmCM.Data))

		var actualCoreDNSDeployment appsv1.Deployment
		g.Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "coredns", Namespace: metav1.NamespaceSystem}, &actualCoreDNSDeployment)).To(Succeed())
		g.Expect(actualCoreDNSDeployment.Spec.Template.Spec.Containers[0].Image).ToNot(ContainSubstring("coredns"))
	})

	t.Run("returns error when unable to UpdateCoreDNS", func(t *testing.T) {
		g := NewWithT(t)
		objs := []client.Object{
			cluster.DeepCopy(),
			kcp.DeepCopy(),
			depl.DeepCopy(),
			corednsCM.DeepCopy(),
		}

		fakeClient := newFakeClient(objs...)

		workloadCluster := fakeWorkloadCluster{
			Workload: &internal.Workload{
				Client: fakeClient,
				CoreDNSMigrator: &fakeMigrator{
					migratedCorefile: "new core file",
				},
			},
		}

		g.Expect(workloadCluster.UpdateCoreDNS(ctx, kcp, semver.MustParse("1.19.1"))).ToNot(Succeed())
	})
}

func TestKubeadmControlPlaneReconciler_reconcileDelete(t *testing.T) {
	t.Run("removes all control plane Machines", func(t *testing.T) {
		g := NewWithT(t)

		cluster, kcp, _ := createClusterWithControlPlane(metav1.NamespaceDefault)
		controllerutil.AddFinalizer(kcp, controlplanev1.KubeadmControlPlaneFinalizer)
		initObjs := []client.Object{cluster.DeepCopy(), kcp.DeepCopy()}

		machines := collections.New()
		for i := range 3 {
			m, _ := createMachineNodePair(fmt.Sprintf("test-%d", i), cluster, kcp, true)
			m.Annotations[controlplanev1.PreTerminateHookCleanupAnnotation] = ""
			// Note: Block deletion so we can later verify the pre-terminate hook was removed
			m.Finalizers = []string{"cluster.x-k8s.io/block-deletion"}
			initObjs = append(initObjs, m)
			machines.Insert(m)
		}
		// One Machine was already deleted before KCP, validate the pre-terminate hook is still removed.
		machines.UnsortedList()[2].DeletionTimestamp = &metav1.Time{Time: time.Now()}

		fakeClient := newFakeClient(initObjs...)

		r := &KubeadmControlPlaneReconciler{
			Client:              fakeClient,
			SecretCachingClient: fakeClient,
			managementCluster: &fakeManagementCluster{
				Management: &internal.Management{Client: fakeClient},
				Workload:   &fakeWorkloadCluster{},
			},

			recorder: record.NewFakeRecorder(32),
		}

		controlPlane := &internal.ControlPlane{
			KCP:      kcp,
			Cluster:  cluster,
			Machines: machines,
		}

		result, err := r.reconcileDelete(ctx, controlPlane)
		g.Expect(result).To(Equal(ctrl.Result{RequeueAfter: deleteRequeueAfter}))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(kcp.Finalizers).To(ContainElement(controlplanev1.KubeadmControlPlaneFinalizer))

		controlPlaneMachines := clusterv1.MachineList{}
		g.Expect(fakeClient.List(ctx, &controlPlaneMachines)).To(Succeed())
		for _, machine := range controlPlaneMachines.Items {
			// Verify pre-terminate hook was removed
			g.Expect(machine.Annotations).ToNot(HaveKey(controlplanev1.PreTerminateHookCleanupAnnotation))

			// Remove finalizer
			originalMachine := machine.DeepCopy()
			machine.Finalizers = []string{}
			g.Expect(fakeClient.Patch(ctx, &machine, client.MergeFrom(originalMachine))).To(Succeed())
		}

		// Verify all Machines are gone.
		g.Expect(fakeClient.List(ctx, &controlPlaneMachines)).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(BeEmpty())

		controlPlane = &internal.ControlPlane{
			KCP:     kcp,
			Cluster: cluster,
		}

		result, err = r.reconcileDelete(ctx, controlPlane)
		g.Expect(result).To(BeComparableTo(ctrl.Result{}))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(kcp.Finalizers).To(BeEmpty())
	})

	t.Run("does not remove any control plane Machines if other Machines exist", func(t *testing.T) {
		g := NewWithT(t)

		cluster, kcp, _ := createClusterWithControlPlane(metav1.NamespaceDefault)
		controllerutil.AddFinalizer(kcp, controlplanev1.KubeadmControlPlaneFinalizer)

		workerMachine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "worker",
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
			},
		}

		initObjs := []client.Object{cluster.DeepCopy(), kcp.DeepCopy(), workerMachine.DeepCopy()}

		machines := collections.New()
		for i := range 3 {
			m, _ := createMachineNodePair(fmt.Sprintf("test-%d", i), cluster, kcp, true)
			initObjs = append(initObjs, m)
			machines.Insert(m)
		}

		fakeClient := newFakeClient(initObjs...)

		r := &KubeadmControlPlaneReconciler{
			Client:              fakeClient,
			SecretCachingClient: fakeClient,
			managementCluster: &fakeManagementCluster{
				Management: &internal.Management{Client: fakeClient},
				Workload:   &fakeWorkloadCluster{},
			},
			recorder: record.NewFakeRecorder(32),
		}

		controlPlane := &internal.ControlPlane{
			KCP:      kcp,
			Cluster:  cluster,
			Machines: machines,
		}

		result, err := r.reconcileDelete(ctx, controlPlane)
		g.Expect(result).To(BeComparableTo(ctrl.Result{RequeueAfter: deleteRequeueAfter}))
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(kcp.Finalizers).To(ContainElement(controlplanev1.KubeadmControlPlaneFinalizer))

		controlPlaneMachines := clusterv1.MachineList{}
		labels := map[string]string{
			clusterv1.MachineControlPlaneLabel: "",
		}
		g.Expect(fakeClient.List(ctx, &controlPlaneMachines, client.MatchingLabels(labels))).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(HaveLen(3))
	})

	t.Run("does not remove any control plane Machines if MachinePools exist", func(t *testing.T) {
		_ = feature.MutableGates.Set("MachinePool=true")
		g := NewWithT(t)

		cluster, kcp, _ := createClusterWithControlPlane(metav1.NamespaceDefault)
		controllerutil.AddFinalizer(kcp, controlplanev1.KubeadmControlPlaneFinalizer)

		workerMachinePool := &expv1.MachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "worker",
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
			},
		}

		initObjs := []client.Object{cluster.DeepCopy(), kcp.DeepCopy(), workerMachinePool.DeepCopy()}

		machines := collections.New()
		for i := range 3 {
			m, _ := createMachineNodePair(fmt.Sprintf("test-%d", i), cluster, kcp, true)
			initObjs = append(initObjs, m)
			machines.Insert(m)
		}

		fakeClient := newFakeClient(initObjs...)

		r := &KubeadmControlPlaneReconciler{
			Client:              fakeClient,
			SecretCachingClient: fakeClient,
			managementCluster: &fakeManagementCluster{
				Management: &internal.Management{Client: fakeClient},
				Workload:   &fakeWorkloadCluster{},
			},
			recorder: record.NewFakeRecorder(32),
		}

		controlPlane := &internal.ControlPlane{
			KCP:      kcp,
			Cluster:  cluster,
			Machines: machines,
		}

		result, err := r.reconcileDelete(ctx, controlPlane)
		g.Expect(result).To(BeComparableTo(ctrl.Result{RequeueAfter: deleteRequeueAfter}))
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(kcp.Finalizers).To(ContainElement(controlplanev1.KubeadmControlPlaneFinalizer))

		controlPlaneMachines := clusterv1.MachineList{}
		labels := map[string]string{
			clusterv1.MachineControlPlaneLabel: "",
		}
		g.Expect(fakeClient.List(ctx, &controlPlaneMachines, client.MatchingLabels(labels))).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(HaveLen(3))
	})

	t.Run("removes the finalizer if no control plane Machines exist", func(t *testing.T) {
		g := NewWithT(t)

		cluster, kcp, _ := createClusterWithControlPlane(metav1.NamespaceDefault)
		controllerutil.AddFinalizer(kcp, controlplanev1.KubeadmControlPlaneFinalizer)

		fakeClient := newFakeClient(cluster.DeepCopy(), kcp.DeepCopy())

		r := &KubeadmControlPlaneReconciler{
			Client:              fakeClient,
			SecretCachingClient: fakeClient,
			managementCluster: &fakeManagementCluster{
				Management: &internal.Management{Client: fakeClient},
				Workload:   &fakeWorkloadCluster{},
			},
			recorder: record.NewFakeRecorder(32),
		}

		controlPlane := &internal.ControlPlane{
			KCP:     kcp,
			Cluster: cluster,
		}

		result, err := r.reconcileDelete(ctx, controlPlane)
		g.Expect(result).To(BeComparableTo(ctrl.Result{}))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(kcp.Finalizers).To(BeEmpty())
	})
}

// test utils.

func newFakeClient(initObjs ...client.Object) client.Client {
	return &fakeClient{
		startTime: time.Now(),
		Client:    fake.NewClientBuilder().WithObjects(initObjs...).WithStatusSubresource(&controlplanev1.KubeadmControlPlane{}).Build(),
	}
}

type fakeClient struct {
	startTime time.Time
	mux       sync.Mutex
	client.Client
}

type fakeClientI interface {
	SetCreationTimestamp(timestamp metav1.Time)
}

// controller-runtime's fake client doesn't set a CreationTimestamp
// this sets one that increments by a minute for each object created.
func (c *fakeClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if f, ok := obj.(fakeClientI); ok {
		c.mux.Lock()
		c.startTime = c.startTime.Add(time.Minute)
		f.SetCreationTimestamp(metav1.NewTime(c.startTime))
		c.mux.Unlock()
	}
	return c.Client.Create(ctx, obj, opts...)
}

func createClusterWithControlPlane(namespace string) (*clusterv1.Cluster, *controlplanev1.KubeadmControlPlane, *unstructured.Unstructured) {
	kcpName := fmt.Sprintf("kcp-foo-%s", util.RandomString(6))

	cluster := newCluster(&types.NamespacedName{Name: kcpName, Namespace: namespace})
	cluster.Spec = clusterv1.ClusterSpec{
		ControlPlaneRef: &corev1.ObjectReference{
			Kind:       "KubeadmControlPlane",
			Namespace:  namespace,
			Name:       kcpName,
			APIVersion: controlplanev1.GroupVersion.String(),
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
		TypeMeta: metav1.TypeMeta{
			APIVersion: controlplanev1.GroupVersion.String(),
			Kind:       "KubeadmControlPlane",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kcpName,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
					Name:       kcpName,
					UID:        "1",
				},
			},
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			MachineTemplate: controlplanev1.KubeadmControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       "GenericInfrastructureMachineTemplate",
					Namespace:  namespace,
					Name:       "infra-foo",
					APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				},
			},
			Replicas: ptr.To[int32](int32(3)),
			Version:  "v1.31.0",
			RolloutStrategy: &controlplanev1.RolloutStrategy{
				Type: "RollingUpdate",
				RollingUpdate: &controlplanev1.RollingUpdate{
					MaxSurge: &intstr.IntOrString{
						IntVal: 1,
					},
				},
			},
		},
	}

	genericMachineTemplate := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericInfrastructureMachineTemplate",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"name":      "infra-foo",
				"namespace": namespace,
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

func setKCPHealthy(kcp *controlplanev1.KubeadmControlPlane) {
	conditions.MarkTrue(kcp, controlplanev1.ControlPlaneComponentsHealthyCondition)
	conditions.MarkTrue(kcp, controlplanev1.EtcdClusterHealthyCondition)
}

func createMachineNodePair(name string, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane, ready bool) (*clusterv1.Machine, *corev1.Node) {
	machine := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Machine",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   cluster.Namespace,
			Name:        name,
			Labels:      internal.ControlPlaneMachineLabelsForCluster(kcp, cluster.Name),
			Annotations: map[string]string{},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane")),
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: cluster.Name,
			InfrastructureRef: corev1.ObjectReference{
				Kind:       builder.GenericInfrastructureMachineCRD.Kind,
				APIVersion: builder.GenericInfrastructureMachineCRD.APIVersion,
				Name:       builder.GenericInfrastructureMachineCRD.Name,
				Namespace:  builder.GenericInfrastructureMachineCRD.Namespace,
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
	webhook := webhooks.Machine{}
	if err := webhook.Default(ctx, machine); err != nil {
		panic(err)
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"node-role.kubernetes.io/control-plane": ""},
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

func setMachineHealthy(m *clusterv1.Machine) {
	m.Status.NodeRef = &corev1.ObjectReference{
		Kind: "Node",
		Name: "node-1",
	}
	conditions.MarkTrue(m, controlplanev1.MachineAPIServerPodHealthyCondition)
	conditions.MarkTrue(m, controlplanev1.MachineControllerManagerPodHealthyCondition)
	conditions.MarkTrue(m, controlplanev1.MachineSchedulerPodHealthyCondition)
	conditions.MarkTrue(m, controlplanev1.MachineEtcdPodHealthyCondition)
	conditions.MarkTrue(m, controlplanev1.MachineEtcdMemberHealthyCondition)
}

// newCluster return a CAPI cluster object.
func newCluster(namespacedName *types.NamespacedName) *clusterv1.Cluster {
	return &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespacedName.Namespace,
			Name:      namespacedName.Name,
		},
	}
}

func getTestCACert(key *rsa.PrivateKey) (*x509.Certificate, error) {
	cfg := certs.Config{
		CommonName: "kubernetes",
	}

	now := time.Now().UTC()

	tmpl := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(0),
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		NotBefore:             now.Add(time.Minute * -5),
		NotAfter:              now.Add(time.Hour * 24), // 1 day
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		MaxPathLenZero:        true,
		BasicConstraintsValid: true,
		MaxPathLen:            0,
		IsCA:                  true,
	}

	b, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, key.Public(), key)
	if err != nil {
		return nil, err
	}

	c, err := x509.ParseCertificate(b)
	return c, err
}
