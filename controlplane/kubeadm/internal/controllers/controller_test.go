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

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/desiredstate"
	controlplanev1webhooks "sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/webhooks"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/internal/webhooks"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/certs"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/secret"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

const (
	timeout = time.Second * 30
)

func TestClusterToKubeadmControlPlane(t *testing.T) {
	g := NewWithT(t)
	fakeClient := newFakeClient()

	cluster := newCluster(&types.NamespacedName{Name: "foo", Namespace: metav1.NamespaceDefault})
	cluster.Spec = clusterv1.ClusterSpec{
		ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
			APIGroup: controlplanev1.GroupVersion.Group,
			Kind:     "KubeadmControlPlane",
			Name:     "kcp-foo",
		},
	}

	expectedResult := []ctrl.Request{
		{
			NamespacedName: client.ObjectKey{
				Namespace: cluster.Namespace,
				Name:      cluster.Spec.ControlPlaneRef.Name,
			},
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
		ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
			APIGroup: controlplanev1.GroupVersion.Group,
			Kind:     "OtherControlPlane",
			Name:     "other-foo",
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
				Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "UnknownInfraMachine",
						APIGroup: "test",
						Name:     "foo",
					},
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
				Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "UnknownInfraMachine",
						APIGroup: "test",
						Name:     "foo",
					},
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
				Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "UnknownInfraMachine",
						APIGroup: "test",
						Name:     "foo",
					},
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
	cluster.Spec.Paused = ptr.To(true)
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
				Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "UnknownInfraMachine",
						APIGroup: "test",
						Name:     "foo",
					},
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
	cluster.Spec.Paused = ptr.To(false)
	kcp.Annotations = map[string]string{}
	kcp.Annotations[clusterv1.PausedAnnotation] = "paused"
	_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(kcp)})
	g.Expect(err).ToNot(HaveOccurred())
}

func TestReconcileClusterNoEndpoints(t *testing.T) {
	g := NewWithT(t)

	cluster := newCluster(&types.NamespacedName{Name: "foo", Namespace: metav1.NamespaceDefault})
	cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)

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
				Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "UnknownInfraMachine",
						APIGroup: "test",
						Name:     "foo",
					},
				},
			},
		},
		Status: controlplanev1.KubeadmControlPlaneStatus{
			Conditions: []metav1.Condition{{
				Type:   clusterv1.PausedCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.NotPausedReason,
			}},
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
		cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
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
					Labels:    desiredstate.ControlPlaneMachineLabels(kcp, cluster.Name),
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: bootstrapv1.GroupVersion.Group,
							Kind:     "KubeadmConfig",
							Name:     name,
						},
					},
					Version: version,
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
		cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
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
					Labels:    desiredstate.ControlPlaneMachineLabels(kcp, cluster.Name),
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: bootstrapv1.GroupVersion.Group,
							Kind:     "KubeadmConfig",
							Name:     name,
						},
					},
					Version: version,
				},
			}
			cfg := &bootstrapv1.KubeadmConfig{
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
		cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
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
					Labels:    desiredstate.ControlPlaneMachineLabels(kcp, cluster.Name),
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: bootstrapv1.GroupVersion.Group,
							Kind:     "KubeadmConfig",
							Name:     name,
						},
					},
					Version: version,
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
		cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
		kcp.Spec.Version = "v1.17.0"

		fmc := &fakeManagementCluster{
			Machines: collections.Machines{
				"test0": &clusterv1.Machine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: cluster.Namespace,
						Name:      "test0",
						Labels:    desiredstate.ControlPlaneMachineLabels(kcp, cluster.Name),
					},
					Spec: clusterv1.MachineSpec{
						Bootstrap: clusterv1.Bootstrap{
							ConfigRef: clusterv1.ContractVersionedObjectReference{
								APIGroup: bootstrapv1.GroupVersion.Group,
								Kind:     "KubeadmConfig",
							},
						},
						Version: "v1.15.0",
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
	cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
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
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.30.0",
		},
		Status: controlplanev1.KubeadmControlPlaneStatus{
			Initialization: controlplanev1.KubeadmControlPlaneInitializationStatus{
				ControlPlaneInitialized: ptr.To(true),
			},
		},
	}
	machineWithoutExpiryAnnotation := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machineWithoutExpiryAnnotation",
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				Kind:     builder.TestInfrastructureMachineKind,
				APIGroup: builder.InfrastructureGroupVersion.Group,
				Name:     "machineWithoutExpiryAnnotation-infra",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: clusterv1.ContractVersionedObjectReference{
					Kind:     "KubeadmConfig",
					APIGroup: bootstrapv1.GroupVersion.Group,
					Name:     "machineWithoutExpiryAnnotation-bootstrap",
				},
			},
		},
		Status: clusterv1.MachineStatus{
			NodeRef: clusterv1.MachineNodeReference{
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
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				Kind:     builder.TestInfrastructureMachineKind,
				APIGroup: builder.InfrastructureGroupVersion.Group,
				Name:     "machineWithExpiryAnnotation-infra",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: clusterv1.ContractVersionedObjectReference{
					Kind:     "KubeadmConfig",
					APIGroup: bootstrapv1.GroupVersion.Group,
					Name:     "machineWithExpiryAnnotation-bootstrap",
				},
			},
		},
		Status: clusterv1.MachineStatus{
			NodeRef: clusterv1.MachineNodeReference{
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
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				Kind:     builder.TestInfrastructureMachineKind,
				APIGroup: builder.InfrastructureGroupVersion.Group,
				Name:     "machineWithDeletionTimestamp-infra",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: clusterv1.ContractVersionedObjectReference{
					Kind:     "KubeadmConfig",
					APIGroup: bootstrapv1.GroupVersion.Group,
					Name:     "machineWithDeletionTimestamp-bootstrap",
				},
			},
		},
		Status: clusterv1.MachineStatus{
			NodeRef: clusterv1.MachineNodeReference{
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
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				Kind:     builder.TestInfrastructureMachineKind,
				APIGroup: builder.InfrastructureGroupVersion.Group,
				Name:     "machineWithoutNodeRef-infra",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: clusterv1.ContractVersionedObjectReference{
					Kind:     "KubeadmConfig",
					APIGroup: bootstrapv1.GroupVersion.Group,
					Name:     "machineWithoutNodeRef-bootstrap",
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
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				Kind:     builder.TestInfrastructureMachineKind,
				APIGroup: builder.InfrastructureGroupVersion.Group,
				Name:     "machineWithoutKubeadmConfig-infra",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: clusterv1.ContractVersionedObjectReference{
					Kind:     "KubeadmConfig",
					APIGroup: bootstrapv1.GroupVersion.Group,
					Name:     "machineWithoutKubeadmConfig-bootstrap",
				},
			},
		},
		Status: clusterv1.MachineStatus{
			NodeRef: clusterv1.MachineNodeReference{
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
		// Note: CRD is needed to look up the apiVersion from contract labels.
		builder.TestInfrastructureMachineCRD,
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
		InfrastructureRef: clusterv1.ContractVersionedObjectReference{
			APIGroup: builder.InfrastructureGroupVersion.Group,
			Kind:     builder.GenericInfrastructureClusterKind,
			Name:     "infracluster1",
		},
	}
	g.Expect(env.Create(ctx, cluster)).To(Succeed())
	patchHelper, err := patch.NewHelper(cluster, env)
	g.Expect(err).ToNot(HaveOccurred())
	cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
	g.Expect(patchHelper.Patch(ctx, cluster)).To(Succeed())

	genericInfrastructureMachineTemplate := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       builder.GenericInfrastructureMachineTemplateKind,
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
				Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						Kind:     genericInfrastructureMachineTemplate.GetKind(),
						APIGroup: genericInfrastructureMachineTemplate.GroupVersionKind().Group,
						Name:     genericInfrastructureMachineTemplate.GetName(),
					},
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
		ssaCache: ssa.NewCache("test-controller"),
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
		g.Expect(kcp.Status.Replicas).To(HaveValue(BeEquivalentTo(1)))
		g.Expect(v1beta1conditions.IsFalse(kcp, controlplanev1.AvailableV1Beta1Condition)).To(BeTrue())
		g.Expect(conditions.IsFalse(kcp, controlplanev1.KubeadmControlPlaneInitializedCondition)).To(BeTrue())

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
		infraObj, err := external.GetObjectFromContractVersionedRef(ctx, r.Client, machine.Spec.InfrastructureRef, machine.Namespace)
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
		InfrastructureRef: clusterv1.ContractVersionedObjectReference{
			APIGroup: builder.InfrastructureGroupVersion.Group,
			Kind:     builder.GenericInfrastructureClusterKind,
			Name:     "infracluster1",
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
	cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
	g.Expect(patchHelper.Patch(ctx, cluster)).To(Succeed())

	g.Expect(env.CreateAndWait(ctx, certSecret)).To(Succeed())

	genericInfrastructureMachineTemplate := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       builder.GenericInfrastructureMachineTemplateKind,
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
				Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						Kind:     genericInfrastructureMachineTemplate.GetKind(),
						APIGroup: genericInfrastructureMachineTemplate.GroupVersionKind().Group,
						Name:     genericInfrastructureMachineTemplate.GetName(),
					},
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
		ssaCache: ssa.NewCache("test-controller"),
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
		g.Expect(kcp.Status.Replicas).To(HaveValue(BeEquivalentTo(1)))
		g.Expect(v1beta1conditions.IsFalse(kcp, controlplanev1.AvailableV1Beta1Condition)).To(BeTrue())

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
		infraObj, err := external.GetObjectFromContractVersionedRef(ctx, r.Client, machine.Spec.InfrastructureRef, machine.Namespace)
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
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      "test-cluster",
			},
			Spec: clusterv1.ClusterSpec{
				InfrastructureRef: clusterv1.ContractVersionedObjectReference{
					APIGroup: builder.InfrastructureGroupVersion.Group,
					Kind:     builder.GenericInfrastructureClusterKind,
					Name:     "infracluster1",
				},
			},
		}
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

	duration5s := ptr.To(int32(5))
	duration10s := ptr.To(int32(10))

	// Existing InfraMachine
	infraMachineSpec := map[string]interface{}{
		"infra-field": "infra-value",
	}
	existingInfraMachine := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericInfrastructureMachine",
			"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
			"metadata": map[string]interface{}{
				"name":      "existing-inframachine",
				"namespace": testCluster.Namespace,
				"labels": map[string]interface{}{
					"preserved-label": "preserved-value",
					"dropped-label":   "dropped-value",
					"modified-label":  "modified-value",
				},
				"annotations": map[string]interface{}{
					"preserved-annotation": "preserved-value",
					"dropped-annotation":   "dropped-value",
					"modified-annotation":  "modified-value",
				},
			},
			"spec": infraMachineSpec,
		},
	}
	infraMachineRef := &clusterv1.ContractVersionedObjectReference{
		Kind:     "GenericInfrastructureMachine",
		Name:     "existing-inframachine",
		APIGroup: clusterv1.GroupVersionInfrastructure.Group,
	}

	// Existing KubeadmConfig
	bootstrapSpec := &bootstrapv1.KubeadmConfigSpec{
		Users: []bootstrapv1.User{{Name: "test-user"}},
	}
	existingKubeadmConfig := &bootstrapv1.KubeadmConfig{
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
	bootstrapRef := clusterv1.ContractVersionedObjectReference{
		Kind:     "KubeadmConfig",
		Name:     "existing-kubeadmconfig",
		APIGroup: bootstrapv1.GroupVersion.Group,
	}

	// Existing Machine to validate in-place mutation
	fd := "fd1"
	inPlaceMutatingMachine := &clusterv1.Machine{
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
			InfrastructureRef: *infraMachineRef,
			Version:           "v1.25.3",
			FailureDomain:     fd,
			// ProviderID is intentionally not set here, this field is set by the Machine controller.
			Deletion: clusterv1.MachineDeletionSpec{
				NodeDrainTimeoutSeconds:        duration5s,
				NodeVolumeDetachTimeoutSeconds: duration5s,
				NodeDeletionTimeoutSeconds:     duration5s,
			},
		},
	}

	// Existing machine that is in deleting state
	deletingMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "deleting-machine",
			Namespace:   namespace.Name,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
			Finalizers:  []string{"testing-finalizer"},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: testCluster.Name,
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: clusterv1.GroupVersionInfrastructure.Group,
				Kind:     builder.GenericInfrastructureMachineKind,
				Name:     "inframachine",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: clusterv1.ContractVersionedObjectReference{
					Kind:     "KubeadmConfig",
					Name:     "non-existing-kubeadmconfig",
					APIGroup: bootstrapv1.GroupVersion.Group,
				},
			},
			Deletion: clusterv1.MachineDeletionSpec{
				NodeDrainTimeoutSeconds:        duration5s,
				NodeVolumeDetachTimeoutSeconds: duration5s,
				NodeDeletionTimeoutSeconds:     duration5s,
			},
			ReadinessGates: desiredstate.MandatoryMachineReadinessGates,
		},
	}

	// Existing machine that has a InfrastructureRef which does not exist.
	nilInfraMachineMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "nil-infra-machine-machine",
			Namespace:   namespace.Name,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
			Finalizers:  []string{"testing-finalizer"},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: testCluster.Name,
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: clusterv1.GroupVersionInfrastructure.Group,
				Kind:     builder.GenericInfrastructureMachineKind,
				Name:     "inframachine",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: clusterv1.ContractVersionedObjectReference{
					Kind:     "KubeadmConfig",
					Name:     "non-existing-kubeadmconfig",
					APIGroup: bootstrapv1.GroupVersion.Group,
				},
			},
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
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
				Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						Kind:     "GenericInfrastructureMachineTemplate",
						Name:     "infra-foo",
						APIGroup: clusterv1.GroupVersionInfrastructure.Group,
					},
					Deletion: controlplanev1.KubeadmControlPlaneMachineTemplateDeletionSpec{
						NodeDrainTimeoutSeconds:        duration5s,
						NodeVolumeDetachTimeoutSeconds: duration5s,
						NodeDeletionTimeoutSeconds:     duration5s,
					},
				},
			},
		},
	}

	//
	// Create objects
	//

	// Create InfraMachine (same as in createInfraMachine)
	g.Expect(ssa.Patch(ctx, env.Client, kcpManagerName, existingInfraMachine)).To(Succeed())
	g.Expect(ssa.RemoveManagedFieldsForLabelsAndAnnotations(ctx, env.Client, env.GetAPIReader(), existingInfraMachine, kcpManagerName)).To(Succeed())

	// Create KubeadmConfig (same as in createKubeadmConfig)
	g.Expect(ssa.Patch(ctx, env.Client, kcpManagerName, existingKubeadmConfig)).To(Succeed())
	g.Expect(ssa.RemoveManagedFieldsForLabelsAndAnnotations(ctx, env.Client, env.GetAPIReader(), existingKubeadmConfig, kcpManagerName)).To(Succeed())

	// Create Machines (same as in createMachine)
	g.Expect(ssa.Patch(ctx, env.Client, kcpManagerName, inPlaceMutatingMachine)).To(Succeed())
	g.Expect(ssa.Patch(ctx, env.Client, kcpManagerName, deletingMachine)).To(Succeed())
	// Delete the machine to put it in the deleting state
	g.Expect(env.Delete(ctx, deletingMachine)).To(Succeed())
	// Wait till the machine is marked for deletion
	g.Eventually(func() bool {
		if err := env.Get(ctx, client.ObjectKeyFromObject(deletingMachine), deletingMachine); err != nil {
			return false
		}
		return !deletingMachine.DeletionTimestamp.IsZero()
	}, timeout).Should(BeTrue())
	g.Expect(ssa.Patch(ctx, env.Client, kcpManagerName, nilInfraMachineMachine)).To(Succeed())

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
		// Note: Ensure the fieldManager defaults to manager like in prod.
		//       Otherwise it defaults to the binary name which is not manager in tests.
		Client:              client.WithFieldOwner(env.Client, "manager"),
		SecretCachingClient: secretCachingClient,
		ssaCache:            ssa.NewCache("test-controller"),
	}
	g.Expect(reconciler.syncMachines(ctx, controlPlane)).To(Succeed())

	updatedInPlaceMutatingMachine := inPlaceMutatingMachine.DeepCopy()
	g.Eventually(func(g Gomega) {
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedInPlaceMutatingMachine), updatedInPlaceMutatingMachine)).To(Succeed())
		g.Expect(cleanupTime(updatedInPlaceMutatingMachine.ManagedFields)).To(ConsistOf(toManagedFields([]managedFieldEntry{{
			// capi-kubeadmcontrolplane owns almost everything.
			Manager:    kcpManagerName,
			Operation:  metav1.ManagedFieldsOperationApply,
			APIVersion: clusterv1.GroupVersion.String(),
			FieldsV1:   "{\"f:metadata\":{\"f:annotations\":{\"f:dropped-annotation\":{},\"f:modified-annotation\":{},\"f:pre-terminate.delete.hook.machine.cluster.x-k8s.io/kcp-cleanup\":{},\"f:preserved-annotation\":{}},\"f:labels\":{\"f:cluster.x-k8s.io/cluster-name\":{},\"f:cluster.x-k8s.io/control-plane\":{},\"f:cluster.x-k8s.io/control-plane-name\":{},\"f:dropped-label\":{},\"f:modified-label\":{},\"f:preserved-label\":{}},\"f:ownerReferences\":{\"k:{\\\"uid\\\":\\\"abc-123-control-plane\\\"}\":{}}},\"f:spec\":{\"f:bootstrap\":{\"f:configRef\":{\"f:apiGroup\":{},\"f:kind\":{},\"f:name\":{}}},\"f:clusterName\":{},\"f:deletion\":{\"f:nodeDeletionTimeoutSeconds\":{},\"f:nodeDrainTimeoutSeconds\":{},\"f:nodeVolumeDetachTimeoutSeconds\":{}},\"f:failureDomain\":{},\"f:infrastructureRef\":{\"f:apiGroup\":{},\"f:kind\":{},\"f:name\":{}},\"f:readinessGates\":{\"k:{\\\"conditionType\\\":\\\"APIServerPodHealthy\\\"}\":{\".\":{},\"f:conditionType\":{}},\"k:{\\\"conditionType\\\":\\\"ControllerManagerPodHealthy\\\"}\":{\".\":{},\"f:conditionType\":{}},\"k:{\\\"conditionType\\\":\\\"EtcdMemberHealthy\\\"}\":{\".\":{},\"f:conditionType\":{}},\"k:{\\\"conditionType\\\":\\\"EtcdPodHealthy\\\"}\":{\".\":{},\"f:conditionType\":{}},\"k:{\\\"conditionType\\\":\\\"SchedulerPodHealthy\\\"}\":{\".\":{},\"f:conditionType\":{}}},\"f:version\":{}}}",
		}})))
	}, timeout).Should(Succeed())

	updatedInfraMachine := existingInfraMachine.DeepCopy()
	g.Eventually(func(g Gomega) {
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedInfraMachine), updatedInfraMachine)).To(Succeed())
		g.Expect(cleanupTime(updatedInfraMachine.GetManagedFields())).To(ConsistOf(toManagedFields([]managedFieldEntry{{
			// capi-kubeadmcontrolplane-metadata owns labels and annotations.
			Manager:    kcpMetadataManagerName,
			Operation:  metav1.ManagedFieldsOperationApply,
			APIVersion: updatedInfraMachine.GetAPIVersion(),
			FieldsV1:   "{\"f:metadata\":{\"f:annotations\":{\"f:dropped-annotation\":{},\"f:modified-annotation\":{},\"f:preserved-annotation\":{}},\"f:labels\":{\"f:cluster.x-k8s.io/cluster-name\":{},\"f:cluster.x-k8s.io/control-plane\":{},\"f:cluster.x-k8s.io/control-plane-name\":{},\"f:dropped-label\":{},\"f:modified-label\":{},\"f:preserved-label\":{}}}}",
		}, {
			// capi-kubeadmcontrolplane owns almost everything.
			Manager:    kcpManagerName,
			Operation:  metav1.ManagedFieldsOperationApply,
			APIVersion: updatedInfraMachine.GetAPIVersion(),
			FieldsV1:   "{\"f:spec\":{\"f:infra-field\":{}}}",
		}})))
	}, timeout).Should(Succeed())

	updatedKubeadmConfig := existingKubeadmConfig.DeepCopy()
	g.Eventually(func(g Gomega) {
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedKubeadmConfig), updatedKubeadmConfig)).To(Succeed())
		g.Expect(cleanupTime(updatedKubeadmConfig.GetManagedFields())).To(ConsistOf(toManagedFields([]managedFieldEntry{{
			// capi-kubeadmcontrolplane-metadata owns labels and annotations.
			Manager:    kcpMetadataManagerName,
			Operation:  metav1.ManagedFieldsOperationApply,
			APIVersion: bootstrapv1.GroupVersion.String(),
			FieldsV1:   "{\"f:metadata\":{\"f:annotations\":{\"f:dropped-annotation\":{},\"f:modified-annotation\":{},\"f:preserved-annotation\":{}},\"f:labels\":{\"f:cluster.x-k8s.io/cluster-name\":{},\"f:cluster.x-k8s.io/control-plane\":{},\"f:cluster.x-k8s.io/control-plane-name\":{},\"f:dropped-label\":{},\"f:modified-label\":{},\"f:preserved-label\":{}}}}",
		}, {
			// capi-kubeadmcontrolplane owns almost everything.
			Manager:    kcpManagerName,
			Operation:  metav1.ManagedFieldsOperationApply,
			APIVersion: bootstrapv1.GroupVersion.String(),
			FieldsV1:   "{\"f:spec\":{\"f:users\":{}}}",
		}})))
	}, timeout).Should(Succeed())

	//
	// Verify In-place mutating fields
	//

	// Update the KCP and verify the in-place mutating fields are propagated.
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
	kcp.Spec.MachineTemplate.Spec.Deletion.NodeDrainTimeoutSeconds = duration10s
	kcp.Spec.MachineTemplate.Spec.Deletion.NodeDeletionTimeoutSeconds = duration10s
	kcp.Spec.MachineTemplate.Spec.Deletion.NodeVolumeDetachTimeoutSeconds = duration10s
	controlPlane.KCP = kcp
	g.Expect(reconciler.syncMachines(ctx, controlPlane)).To(Succeed())

	// Verify in-place mutable fields are updated on the Machine.
	updatedInPlaceMutatingMachine = inPlaceMutatingMachine.DeepCopy()
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedInPlaceMutatingMachine), updatedInPlaceMutatingMachine)).To(Succeed())
	g.Expect(updatedInPlaceMutatingMachine.Labels).Should(Equal(expectedLabels))
	expectedAnnotations := map[string]string{}
	for k, v := range kcp.Spec.MachineTemplate.ObjectMeta.Annotations {
		expectedAnnotations[k] = v
	}
	// The pre-terminate annotation should always be added
	expectedAnnotations[controlplanev1.PreTerminateHookCleanupAnnotation] = ""
	g.Expect(updatedInPlaceMutatingMachine.Annotations).Should(Equal(expectedAnnotations))
	g.Expect(updatedInPlaceMutatingMachine.Spec.Deletion.NodeDrainTimeoutSeconds).Should(Equal(kcp.Spec.MachineTemplate.Spec.Deletion.NodeDrainTimeoutSeconds))
	g.Expect(updatedInPlaceMutatingMachine.Spec.Deletion.NodeDeletionTimeoutSeconds).Should(Equal(kcp.Spec.MachineTemplate.Spec.Deletion.NodeDeletionTimeoutSeconds))
	g.Expect(updatedInPlaceMutatingMachine.Spec.Deletion.NodeVolumeDetachTimeoutSeconds).Should(Equal(kcp.Spec.MachineTemplate.Spec.Deletion.NodeVolumeDetachTimeoutSeconds))
	// Verify that the non in-place mutating fields remain the same.
	g.Expect(updatedInPlaceMutatingMachine.Spec.FailureDomain).Should(Equal(inPlaceMutatingMachine.Spec.FailureDomain))
	g.Expect(updatedInPlaceMutatingMachine.Spec.ProviderID).Should(Equal(inPlaceMutatingMachine.Spec.ProviderID))
	g.Expect(updatedInPlaceMutatingMachine.Spec.Version).Should(Equal(inPlaceMutatingMachine.Spec.Version))
	g.Expect(updatedInPlaceMutatingMachine.Spec.InfrastructureRef).Should(BeComparableTo(inPlaceMutatingMachine.Spec.InfrastructureRef))
	g.Expect(updatedInPlaceMutatingMachine.Spec.Bootstrap).Should(BeComparableTo(inPlaceMutatingMachine.Spec.Bootstrap))

	// Verify in-place mutable fields are updated on InfrastructureMachine
	updatedInfraMachine = existingInfraMachine.DeepCopy()
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedInfraMachine), updatedInfraMachine)).To(Succeed())
	g.Expect(updatedInfraMachine.GetLabels()).Should(Equal(expectedLabels))
	g.Expect(updatedInfraMachine.GetAnnotations()).Should(Equal(kcp.Spec.MachineTemplate.ObjectMeta.Annotations))
	// Verify spec remains the same
	g.Expect(updatedInfraMachine.Object).Should(HaveKeyWithValue("spec", infraMachineSpec))

	// Verify in-place mutable fields are updated on the KubeadmConfig.
	updatedKubeadmConfig = existingKubeadmConfig.DeepCopy()
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedKubeadmConfig), updatedKubeadmConfig)).To(Succeed())
	g.Expect(updatedKubeadmConfig.GetLabels()).Should(Equal(expectedLabels))
	g.Expect(updatedKubeadmConfig.GetAnnotations()).Should(Equal(kcp.Spec.MachineTemplate.ObjectMeta.Annotations))
	// Verify spec remains the same
	g.Expect(updatedKubeadmConfig.Spec).Should(BeComparableTo(existingKubeadmConfig.Spec))

	// Verify ManagedFields
	g.Eventually(func(g Gomega) {
		updatedDeletingMachine := deletingMachine.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedDeletingMachine), updatedDeletingMachine)).To(Succeed())
		g.Expect(cleanupTime(updatedDeletingMachine.ManagedFields)).To(ConsistOf(toManagedFields([]managedFieldEntry{{
			// capi-kubeadmcontrolplane owns almost everything.
			Manager:    kcpManagerName,
			Operation:  metav1.ManagedFieldsOperationApply,
			APIVersion: clusterv1.GroupVersion.String(),
			FieldsV1:   "{\"f:metadata\":{\"f:finalizers\":{\"v:\\\"testing-finalizer\\\"\":{}}},\"f:spec\":{\"f:bootstrap\":{\"f:configRef\":{\"f:apiGroup\":{},\"f:kind\":{},\"f:name\":{}}},\"f:clusterName\":{},\"f:infrastructureRef\":{\"f:apiGroup\":{},\"f:kind\":{},\"f:name\":{}},\"f:readinessGates\":{\"k:{\\\"conditionType\\\":\\\"APIServerPodHealthy\\\"}\":{\".\":{},\"f:conditionType\":{}},\"k:{\\\"conditionType\\\":\\\"ControllerManagerPodHealthy\\\"}\":{\".\":{},\"f:conditionType\":{}},\"k:{\\\"conditionType\\\":\\\"SchedulerPodHealthy\\\"}\":{\".\":{},\"f:conditionType\":{}}}}}",
		}, {
			// capi-kubeadmcontrolplane owns the fields that are propagated in-place for deleting Machines in syncMachines via patchHelper.
			Manager:    "manager",
			Operation:  metav1.ManagedFieldsOperationUpdate,
			APIVersion: clusterv1.GroupVersion.String(),
			FieldsV1:   "{\"f:spec\":{\"f:deletion\":{\"f:nodeDeletionTimeoutSeconds\":{},\"f:nodeDrainTimeoutSeconds\":{},\"f:nodeVolumeDetachTimeoutSeconds\":{}}}}",
		}})))
	}, timeout).Should(Succeed())

	// Verify in-place mutable fields are updated on the deleting Machine.
	updatedDeletingMachine := deletingMachine.DeepCopy()
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedDeletingMachine), updatedDeletingMachine)).To(Succeed())
	g.Expect(updatedDeletingMachine.Labels).Should(Equal(deletingMachine.Labels))           // Not propagated to a deleting Machine
	g.Expect(updatedDeletingMachine.Annotations).Should(Equal(deletingMachine.Annotations)) // Not propagated to a deleting Machine
	g.Expect(updatedDeletingMachine.Spec.Deletion.NodeDrainTimeoutSeconds).Should(Equal(kcp.Spec.MachineTemplate.Spec.Deletion.NodeDrainTimeoutSeconds))
	g.Expect(updatedDeletingMachine.Spec.Deletion.NodeDeletionTimeoutSeconds).Should(Equal(kcp.Spec.MachineTemplate.Spec.Deletion.NodeDeletionTimeoutSeconds))
	g.Expect(updatedDeletingMachine.Spec.Deletion.NodeVolumeDetachTimeoutSeconds).Should(Equal(kcp.Spec.MachineTemplate.Spec.Deletion.NodeVolumeDetachTimeoutSeconds))
	// Verify the machine spec is otherwise unchanged.
	deletingMachine.Spec.Deletion.NodeDrainTimeoutSeconds = kcp.Spec.MachineTemplate.Spec.Deletion.NodeDrainTimeoutSeconds
	deletingMachine.Spec.Deletion.NodeDeletionTimeoutSeconds = kcp.Spec.MachineTemplate.Spec.Deletion.NodeDeletionTimeoutSeconds
	deletingMachine.Spec.Deletion.NodeVolumeDetachTimeoutSeconds = kcp.Spec.MachineTemplate.Spec.Deletion.NodeVolumeDetachTimeoutSeconds
	g.Expect(updatedDeletingMachine.Spec).Should(BeComparableTo(deletingMachine.Spec))
}

func TestKubeadmControlPlaneReconciler_reconcileControlPlaneAndMachinesConditions(t *testing.T) {
	now := time.Now()

	defaultMachine1 := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine1-test",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachineSpec{
			Version:    "v1.31.0",
			ProviderID: "foo",
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				Kind:     "GenericInfrastructureMachine",
				APIGroup: clusterv1.GroupVersionInfrastructure.Group,
				Name:     "m1",
			},
		},
	}
	defaultMachine1NotUpToDate := defaultMachine1.DeepCopy()
	defaultMachine1NotUpToDate.Spec.Version = "v1.30.0"

	defaultMachine2 := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machine2-test",
			Namespace: metav1.NamespaceDefault,
		},
	}

	defaultCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-test",
			Namespace: metav1.NamespaceDefault,
		},
	}

	defaultKCP := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kcp-test",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Version: "v1.31.0",
		},
		Status: controlplanev1.KubeadmControlPlaneStatus{
			Initialization: controlplanev1.KubeadmControlPlaneInitializationStatus{
				ControlPlaneInitialized: ptr.To(true),
			},
			Conditions: []metav1.Condition{
				{
					Type:               controlplanev1.KubeadmControlPlaneInitializedCondition,
					Status:             metav1.ConditionTrue,
					Reason:             controlplanev1.KubeadmControlPlaneInitializedReason,
					LastTransitionTime: metav1.Time{Time: now.Add(-5 * time.Second)},
				},
			},
		},
	}

	testCases := []struct {
		name                    string
		controlPlane            *internal.ControlPlane
		managementCluster       internal.ManagementCluster
		lastProbeSuccessTime    time.Time
		expectErr               string
		expectKCPConditions     []metav1.Condition
		expectMachineConditions []metav1.Condition
	}{
		{
			name: "Cluster control plane is not initialized",
			controlPlane: &internal.ControlPlane{
				KCP: func() *controlplanev1.KubeadmControlPlane {
					kcp := defaultKCP.DeepCopy()
					kcp.Status.Initialization.ControlPlaneInitialized = ptr.To(false)
					conditions.Set(kcp, metav1.Condition{
						Type:   controlplanev1.KubeadmControlPlaneInitializedCondition,
						Status: metav1.ConditionFalse,
						Reason: controlplanev1.KubeadmControlPlaneNotInitializedReason,
					})
					return kcp
				}(),
				Machines: map[string]*clusterv1.Machine{
					defaultMachine1.Name: defaultMachine1.DeepCopy(),
				},
			},
			expectKCPConditions: []metav1.Condition{
				{
					Type:   controlplanev1.KubeadmControlPlaneInitializedCondition,
					Status: metav1.ConditionFalse,
					Reason: controlplanev1.KubeadmControlPlaneNotInitializedReason,
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneEtcdClusterInspectionFailedReason,
					Message: "Waiting for Cluster control plane to be initialized",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneControlPlaneComponentsInspectionFailedReason,
					Message: "Waiting for Cluster control plane to be initialized",
				},
			},
			expectMachineConditions: []metav1.Condition{
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
					Message: "Waiting for Cluster control plane to be initialized",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
					Message: "Waiting for Cluster control plane to be initialized",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
					Message: "Waiting for Cluster control plane to be initialized",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
					Message: "Waiting for Cluster control plane to be initialized",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedReason,
					Message: "Waiting for Cluster control plane to be initialized",
				},
				{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineUpToDateReason,
				},
			},
		},
		{
			name: "Machines up to date",
			controlPlane: func() *internal.ControlPlane {
				controlPlane, err := internal.NewControlPlane(ctx, nil, env.GetClient(), defaultCluster, defaultKCP.DeepCopy(), collections.FromMachines(
					defaultMachine1.DeepCopy(),
				))
				if err != nil {
					panic(err)
				}
				return controlPlane
			}(),
			managementCluster: &fakeManagementCluster{
				Workload: &fakeWorkloadCluster{
					Workload: &internal.Workload{
						Client: fake.NewClientBuilder().Build(),
					},
				},
			},
			lastProbeSuccessTime: now.Add(-3 * time.Minute),
			expectKCPConditions: []metav1.Condition{
				{
					Type:   controlplanev1.KubeadmControlPlaneInitializedCondition,
					Status: metav1.ConditionTrue,
					Reason: controlplanev1.KubeadmControlPlaneInitializedReason,
				},
				{
					Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
					Status: metav1.ConditionUnknown,
					Reason: controlplanev1.KubeadmControlPlaneEtcdClusterHealthUnknownReason,
					Message: "* Machine machine1-test:\n" +
						"  * EtcdMemberHealthy: Waiting for a Node with spec.providerID foo to exist",
				},
				{
					Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
					Status: metav1.ConditionUnknown,
					Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthUnknownReason,
					Message: "* Machine machine1-test:\n" +
						"  * Control plane components: Waiting for a Node with spec.providerID foo to exist",
				},
			},
			expectMachineConditions: []metav1.Condition{
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
					Message: "Waiting for a Node with spec.providerID foo to exist",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
					Message: "Waiting for a Node with spec.providerID foo to exist",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
					Message: "Waiting for a Node with spec.providerID foo to exist",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
					Message: "Waiting for a Node with spec.providerID foo to exist",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedReason,
					Message: "Waiting for a Node with spec.providerID foo to exist",
				},
				{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineUpToDateReason,
				},
			},
		},
		{
			name: "Machines in place updating, machine not up-to-date date",
			controlPlane: func() *internal.ControlPlane {
				controlPlane, err := internal.NewControlPlane(ctx, nil, env.GetClient(), defaultCluster, defaultKCP.DeepCopy(), collections.FromMachines(
					func() *clusterv1.Machine {
						m := defaultMachine1.DeepCopy()
						m.Annotations = map[string]string{
							clusterv1.UpdateInProgressAnnotation: "",
						}
						return m
					}(),
				))
				if err != nil {
					panic(err)
				}
				return controlPlane
			}(),
			managementCluster: &fakeManagementCluster{
				Workload: &fakeWorkloadCluster{
					Workload: &internal.Workload{
						Client: fake.NewClientBuilder().Build(),
					},
				},
			},
			lastProbeSuccessTime: now.Add(-3 * time.Minute),
			expectKCPConditions: []metav1.Condition{
				{
					Type:   controlplanev1.KubeadmControlPlaneInitializedCondition,
					Status: metav1.ConditionTrue,
					Reason: controlplanev1.KubeadmControlPlaneInitializedReason,
				},
				{
					Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
					Status: metav1.ConditionUnknown,
					Reason: controlplanev1.KubeadmControlPlaneEtcdClusterHealthUnknownReason,
					Message: "* Machine machine1-test:\n" +
						"  * EtcdMemberHealthy: Waiting for a Node with spec.providerID foo to exist",
				},
				{
					Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
					Status: metav1.ConditionUnknown,
					Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthUnknownReason,
					Message: "* Machine machine1-test:\n" +
						"  * Control plane components: Waiting for a Node with spec.providerID foo to exist",
				},
			},
			expectMachineConditions: []metav1.Condition{
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
					Message: "Waiting for a Node with spec.providerID foo to exist",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
					Message: "Waiting for a Node with spec.providerID foo to exist",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
					Message: "Waiting for a Node with spec.providerID foo to exist",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
					Message: "Waiting for a Node with spec.providerID foo to exist",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedReason,
					Message: "Waiting for a Node with spec.providerID foo to exist",
				},
				{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionFalse,
					Reason: clusterv1.MachineUpToDateUpdatingReason,
				},
			},
		},
		{
			name: "Machines not up to date",
			controlPlane: func() *internal.ControlPlane {
				controlPlane, err := internal.NewControlPlane(ctx, nil, env.GetClient(), defaultCluster, defaultKCP.DeepCopy(), collections.FromMachines(
					defaultMachine1NotUpToDate.DeepCopy(),
				))
				if err != nil {
					panic(err)
				}
				return controlPlane
			}(),
			managementCluster: &fakeManagementCluster{
				Workload: &fakeWorkloadCluster{
					Workload: &internal.Workload{
						Client: fake.NewClientBuilder().Build(),
					},
				},
			},
			lastProbeSuccessTime: now.Add(-3 * time.Minute),
			expectKCPConditions: []metav1.Condition{
				{
					Type:   controlplanev1.KubeadmControlPlaneInitializedCondition,
					Status: metav1.ConditionTrue,
					Reason: controlplanev1.KubeadmControlPlaneInitializedReason,
				},
				{
					Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
					Status: metav1.ConditionUnknown,
					Reason: controlplanev1.KubeadmControlPlaneEtcdClusterHealthUnknownReason,
					Message: "* Machine machine1-test:\n" +
						"  * EtcdMemberHealthy: Waiting for a Node with spec.providerID foo to exist",
				},
				{
					Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
					Status: metav1.ConditionUnknown,
					Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthUnknownReason,
					Message: "* Machine machine1-test:\n" +
						"  * Control plane components: Waiting for a Node with spec.providerID foo to exist",
				},
			},
			expectMachineConditions: []metav1.Condition{
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
					Message: "Waiting for a Node with spec.providerID foo to exist",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
					Message: "Waiting for a Node with spec.providerID foo to exist",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
					Message: "Waiting for a Node with spec.providerID foo to exist",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
					Message: "Waiting for a Node with spec.providerID foo to exist",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedReason,
					Message: "Waiting for a Node with spec.providerID foo to exist",
				},
				{
					Type:    clusterv1.MachineUpToDateCondition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineNotUpToDateReason,
					Message: "* Version v1.30.0, v1.31.0 required",
				},
			},
		},
		{
			name: "connection down, preserve conditions as they have been set before (probe did not succeed yet)",
			controlPlane: &internal.ControlPlane{
				Cluster: defaultCluster,
				KCP: func() *controlplanev1.KubeadmControlPlane {
					kcp := defaultKCP.DeepCopy()
					for i, condition := range kcp.Status.Conditions {
						if condition.Type == controlplanev1.KubeadmControlPlaneInitializedCondition {
							kcp.Status.Conditions[i].LastTransitionTime.Time = now.Add(-4 * time.Minute)
						}
					}
					conditions.Set(kcp, metav1.Condition{
						Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
						Status: metav1.ConditionTrue,
						Reason: controlplanev1.KubeadmControlPlaneEtcdClusterHealthyReason,
					})
					conditions.Set(kcp, metav1.Condition{
						Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
						Status: metav1.ConditionTrue,
						Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyReason,
					})
					return kcp
				}(),
				Machines: map[string]*clusterv1.Machine{
					defaultMachine1.Name: func() *clusterv1.Machine {
						m := defaultMachine1.DeepCopy()
						conditions.Set(m, metav1.Condition{
							Type:   controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
							Status: metav1.ConditionTrue,
							Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason,
						})
						return m
					}(),
					defaultMachine2.Name: func() *clusterv1.Machine {
						m := defaultMachine2.DeepCopy()
						conditions.Set(m, metav1.Condition{
							Type:   controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
							Status: metav1.ConditionTrue,
							Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason,
						})
						return m
					}(),
				},
			},
			managementCluster: &fakeManagementCluster{
				Workload: &fakeWorkloadCluster{
					Workload: &internal.Workload{
						Client: fake.NewClientBuilder().Build(),
					},
				},
			},
			lastProbeSuccessTime: time.Time{}, // probe did not succeed yet
			expectErr:            "connection to the workload cluster not established yet",
			expectKCPConditions: []metav1.Condition{
				{
					Type:   controlplanev1.KubeadmControlPlaneInitializedCondition,
					Status: metav1.ConditionTrue,
					Reason: controlplanev1.KubeadmControlPlaneInitializedReason,
				},
				{
					Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
					Status: metav1.ConditionTrue,
					Reason: controlplanev1.KubeadmControlPlaneEtcdClusterHealthyReason,
				},
				{
					Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
					Status: metav1.ConditionTrue,
					Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyReason,
				},
			},
			expectMachineConditions: []metav1.Condition{
				{
					Type:   controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
					Status: metav1.ConditionTrue,
					Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason,
				},
				{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineUpToDateReason,
				},
			},
		},
		{
			name: "connection down, set conditions as they haven't been set before (probe did not succeed yet)",
			controlPlane: func() *internal.ControlPlane {
				controlPlane, err := internal.NewControlPlane(ctx, nil, env.GetClient(), defaultCluster, defaultKCP.DeepCopy(), collections.FromMachines(
					defaultMachine1.DeepCopy(),
				))
				if err != nil {
					panic(err)
				}
				return controlPlane
			}(),
			managementCluster: &fakeManagementCluster{
				Workload: &fakeWorkloadCluster{
					Workload: &internal.Workload{
						Client: fake.NewClientBuilder().Build(),
					},
				},
			},
			lastProbeSuccessTime: time.Time{}, // probe did not succeed yet
			expectErr:            "connection to the workload cluster not established yet",
			expectKCPConditions: []metav1.Condition{
				{
					Type:   controlplanev1.KubeadmControlPlaneInitializedCondition,
					Status: metav1.ConditionTrue,
					Reason: controlplanev1.KubeadmControlPlaneInitializedReason,
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneEtcdClusterConnectionDownReason,
					Message: "Remote connection not established yet",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneControlPlaneComponentsConnectionDownReason,
					Message: "Remote connection not established yet",
				},
			},
			expectMachineConditions: []metav1.Condition{
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodConnectionDownReason,
					Message: "Remote connection not established yet",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodConnectionDownReason,
					Message: "Remote connection not established yet",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodConnectionDownReason,
					Message: "Remote connection not established yet",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodConnectionDownReason,
					Message: "Remote connection not established yet",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachineEtcdMemberConnectionDownReason,
					Message: "Remote connection not established yet",
				},
				{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineUpToDateReason,
				},
			},
		},
		{
			name: "connection down, preserve conditions as they have been set before (remote conditions grace period not passed yet)",
			controlPlane: &internal.ControlPlane{
				Cluster: defaultCluster,
				KCP: func() *controlplanev1.KubeadmControlPlane {
					kcp := defaultKCP.DeepCopy()
					for i, condition := range kcp.Status.Conditions {
						if condition.Type == controlplanev1.KubeadmControlPlaneInitializedCondition {
							kcp.Status.Conditions[i].LastTransitionTime.Time = now.Add(-4 * time.Minute)
						}
					}
					conditions.Set(kcp, metav1.Condition{
						Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
						Status: metav1.ConditionTrue,
						Reason: controlplanev1.KubeadmControlPlaneEtcdClusterHealthyReason,
					})
					conditions.Set(kcp, metav1.Condition{
						Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
						Status: metav1.ConditionTrue,
						Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyReason,
					})
					return kcp
				}(),
				Machines: map[string]*clusterv1.Machine{
					defaultMachine1.Name: func() *clusterv1.Machine {
						m := defaultMachine1.DeepCopy()
						conditions.Set(m, metav1.Condition{
							Type:   controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
							Status: metav1.ConditionTrue,
							Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason,
						})
						return m
					}(),
					defaultMachine2.Name: func() *clusterv1.Machine {
						m := defaultMachine2.DeepCopy()
						conditions.Set(m, metav1.Condition{
							Type:   controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
							Status: metav1.ConditionTrue,
							Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason,
						})
						return m
					}(),
				},
			},
			managementCluster: &fakeManagementCluster{
				WorkloadErr: errors.Wrapf(clustercache.ErrClusterNotConnected, "error getting REST config"),
			},
			lastProbeSuccessTime: now.Add(-3 * time.Minute),
			// Conditions have not been updated.
			// remoteConditionsGracePeriod is 5m
			// control plane is initialized since 4m ago, last probe success was 3m ago.
			expectErr: "cannot get client for the workload cluster: error getting REST config: " +
				"connection to the workload cluster is down",
			expectKCPConditions: []metav1.Condition{
				{
					Type:   controlplanev1.KubeadmControlPlaneInitializedCondition,
					Status: metav1.ConditionTrue,
					Reason: controlplanev1.KubeadmControlPlaneInitializedReason,
				},
				{
					Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
					Status: metav1.ConditionTrue,
					Reason: controlplanev1.KubeadmControlPlaneEtcdClusterHealthyReason,
				},
				{
					Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
					Status: metav1.ConditionTrue,
					Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyReason,
				},
			},
			expectMachineConditions: []metav1.Condition{
				{
					Type:   controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
					Status: metav1.ConditionTrue,
					Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason,
				},
				{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineUpToDateReason,
				},
			},
		},
		{
			name: "connection down, set conditions as they haven't been set before (remote conditions grace period not passed yet)",
			controlPlane: &internal.ControlPlane{
				Cluster: defaultCluster,
				KCP: func() *controlplanev1.KubeadmControlPlane {
					kcp := defaultKCP.DeepCopy()
					for i, condition := range kcp.Status.Conditions {
						if condition.Type == controlplanev1.KubeadmControlPlaneInitializedCondition {
							kcp.Status.Conditions[i].LastTransitionTime.Time = now.Add(-4 * time.Minute)
						}
					}
					return kcp
				}(),
				Machines: map[string]*clusterv1.Machine{
					defaultMachine1.Name: defaultMachine1.DeepCopy(),
					defaultMachine2.Name: defaultMachine2.DeepCopy(),
				},
			},
			managementCluster: &fakeManagementCluster{
				WorkloadErr: errors.Wrapf(clustercache.ErrClusterNotConnected, "error getting REST config"),
			},
			lastProbeSuccessTime: now.Add(-3 * time.Minute),
			// Conditions have been set.
			// remoteConditionsGracePeriod is 5m
			// control plane is initialized since 4m ago, last probe success was 3m ago.
			expectErr: "cannot get client for the workload cluster: error getting REST config: " +
				"connection to the workload cluster is down",
			expectKCPConditions: []metav1.Condition{
				{
					Type:   controlplanev1.KubeadmControlPlaneInitializedCondition,
					Status: metav1.ConditionTrue,
					Reason: controlplanev1.KubeadmControlPlaneInitializedReason,
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneEtcdClusterConnectionDownReason,
					Message: fmt.Sprintf("Last successful probe at %s", now.Add(-3*time.Minute).Format(time.RFC3339)),
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneControlPlaneComponentsConnectionDownReason,
					Message: fmt.Sprintf("Last successful probe at %s", now.Add(-3*time.Minute).Format(time.RFC3339)),
				},
			},
			expectMachineConditions: []metav1.Condition{
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodConnectionDownReason,
					Message: fmt.Sprintf("Last successful probe at %s", now.Add(-3*time.Minute).Format(time.RFC3339)),
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodConnectionDownReason,
					Message: fmt.Sprintf("Last successful probe at %s", now.Add(-3*time.Minute).Format(time.RFC3339)),
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodConnectionDownReason,
					Message: fmt.Sprintf("Last successful probe at %s", now.Add(-3*time.Minute).Format(time.RFC3339)),
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodConnectionDownReason,
					Message: fmt.Sprintf("Last successful probe at %s", now.Add(-3*time.Minute).Format(time.RFC3339)),
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachineEtcdMemberConnectionDownReason,
					Message: fmt.Sprintf("Last successful probe at %s", now.Add(-3*time.Minute).Format(time.RFC3339)),
				},
				{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineUpToDateReason,
				},
			},
		},
		{
			name: "connection down, set conditions to unknown (remote conditions grace period passed)",
			controlPlane: &internal.ControlPlane{
				Cluster: defaultCluster,
				KCP: func() *controlplanev1.KubeadmControlPlane {
					kcp := defaultKCP.DeepCopy()
					for i, condition := range kcp.Status.Conditions {
						if condition.Type == controlplanev1.KubeadmControlPlaneInitializedCondition {
							kcp.Status.Conditions[i].LastTransitionTime.Time = now.Add(-7 * time.Minute)
						}
					}
					conditions.Set(kcp, metav1.Condition{
						Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
						Status: metav1.ConditionTrue,
						Reason: controlplanev1.KubeadmControlPlaneEtcdClusterHealthyReason,
					})
					conditions.Set(kcp, metav1.Condition{
						Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
						Status: metav1.ConditionTrue,
						Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyReason,
					})
					return kcp
				}(),
				Machines: map[string]*clusterv1.Machine{
					defaultMachine1.Name: func() *clusterv1.Machine {
						m := defaultMachine1.DeepCopy()
						conditions.Set(m, metav1.Condition{
							Type:   controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
							Status: metav1.ConditionTrue,
							Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason,
						})
						return m
					}(),
					defaultMachine2.Name: func() *clusterv1.Machine {
						m := defaultMachine2.DeepCopy()
						conditions.Set(m, metav1.Condition{
							Type:   controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
							Status: metav1.ConditionTrue,
							Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason,
						})
						return m
					}(),
				},
			},
			lastProbeSuccessTime: now.Add(-6 * time.Minute),
			// Conditions have been updated.
			// remoteConditionsGracePeriod is 5m
			// control plane is initialized since 7m ago, last probe success was 6m ago.
			expectErr: "connection to the workload cluster is down",
			expectKCPConditions: []metav1.Condition{
				{
					Type:   controlplanev1.KubeadmControlPlaneInitializedCondition,
					Status: metav1.ConditionTrue,
					Reason: controlplanev1.KubeadmControlPlaneInitializedReason,
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneEtcdClusterConnectionDownReason,
					Message: fmt.Sprintf("Last successful probe at %s", now.Add(-6*time.Minute).Format(time.RFC3339)),
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneControlPlaneComponentsConnectionDownReason,
					Message: fmt.Sprintf("Last successful probe at %s", now.Add(-6*time.Minute).Format(time.RFC3339)),
				},
			},
			expectMachineConditions: []metav1.Condition{
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodConnectionDownReason,
					Message: fmt.Sprintf("Last successful probe at %s", now.Add(-6*time.Minute).Format(time.RFC3339)),
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodConnectionDownReason,
					Message: fmt.Sprintf("Last successful probe at %s", now.Add(-6*time.Minute).Format(time.RFC3339)),
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodConnectionDownReason,
					Message: fmt.Sprintf("Last successful probe at %s", now.Add(-6*time.Minute).Format(time.RFC3339)),
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodConnectionDownReason,
					Message: fmt.Sprintf("Last successful probe at %s", now.Add(-6*time.Minute).Format(time.RFC3339)),
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachineEtcdMemberConnectionDownReason,
					Message: fmt.Sprintf("Last successful probe at %s", now.Add(-6*time.Minute).Format(time.RFC3339)),
				},
				{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineUpToDateReason,
				},
			},
		},
		{
			name: "internal error occurred when trying to get workload cluster (InspectionFailed)",
			controlPlane: &internal.ControlPlane{
				Cluster: defaultCluster,
				KCP:     defaultKCP.DeepCopy(),
				Machines: map[string]*clusterv1.Machine{
					defaultMachine1.Name: defaultMachine1.DeepCopy(),
					defaultMachine2.Name: defaultMachine2.DeepCopy(),
				},
			},
			managementCluster: &fakeManagementCluster{
				WorkloadErr: errors.Errorf("failed to get secret; etcd CA bundle"),
			},
			lastProbeSuccessTime: now.Add(-3 * time.Minute),
			expectErr:            "cannot get client for the workload cluster: failed to get secret; etcd CA bundle",
			expectKCPConditions: []metav1.Condition{
				{
					Type:   controlplanev1.KubeadmControlPlaneInitializedCondition,
					Status: metav1.ConditionTrue,
					Reason: controlplanev1.KubeadmControlPlaneInitializedReason,
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneEtcdClusterInspectionFailedReason,
					Message: "Please check controller logs for errors",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneControlPlaneComponentsInspectionFailedReason,
					Message: "Please check controller logs for errors",
				},
			},
			expectMachineConditions: []metav1.Condition{
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
					Message: "Please check controller logs for errors",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
					Message: "Please check controller logs for errors",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
					Message: "Please check controller logs for errors",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
					Message: "Please check controller logs for errors",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedReason,
					Message: "Please check controller logs for errors",
				},
				{
					Type:   clusterv1.MachineUpToDateCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.MachineUpToDateReason,
				},
			},
		},
		{
			name: "successfully got workload cluster (without Machines)",
			controlPlane: &internal.ControlPlane{
				Cluster: defaultCluster,
				KCP:     defaultKCP.DeepCopy(),
			},
			managementCluster: &fakeManagementCluster{
				Workload: &fakeWorkloadCluster{
					Workload: &internal.Workload{
						Client: fake.NewClientBuilder().Build(),
					},
				},
			},
			lastProbeSuccessTime: now.Add(-3 * time.Minute),
			expectKCPConditions: []metav1.Condition{
				{
					Type:   controlplanev1.KubeadmControlPlaneInitializedCondition,
					Status: metav1.ConditionTrue,
					Reason: controlplanev1.KubeadmControlPlaneInitializedReason,
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneEtcdClusterHealthUnknownReason,
					Message: "No Machines reporting etcd member status",
				},
				{
					Type:    controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthUnknownReason,
					Message: "No Machines reporting control plane status",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			r := &KubeadmControlPlaneReconciler{
				ClusterCache: &fakeClusterCache{
					lastProbeSuccessTime: tc.lastProbeSuccessTime,
				},
				RemoteConditionsGracePeriod: 5 * time.Minute,
			}

			if tc.managementCluster != nil {
				tc.controlPlane.InjectTestManagementCluster(tc.managementCluster)
			}

			var objs []client.Object
			for _, machine := range tc.controlPlane.Machines {
				objs = append(objs, machine)
			}
			fakeClient := fake.NewClientBuilder().WithObjects(objs...).WithStatusSubresource(&clusterv1.Machine{}).Build()

			patchHelpers := map[string]*patch.Helper{}
			for _, machine := range tc.controlPlane.Machines {
				helper, err := patch.NewHelper(machine, fakeClient)
				g.Expect(err).ToNot(HaveOccurred())
				patchHelpers[machine.Name] = helper
			}
			tc.controlPlane.SetPatchHelpers(patchHelpers)

			err := r.reconcileControlPlaneAndMachinesConditions(ctx, tc.controlPlane)
			if tc.expectErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tc.expectErr))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			g.Expect(tc.controlPlane.KCP.GetConditions()).To(conditions.MatchConditions(tc.expectKCPConditions, conditions.IgnoreLastTransitionTime(true)))
			for _, machine := range tc.controlPlane.Machines {
				g.Expect(machine.GetConditions()).To(conditions.MatchConditions(tc.expectMachineConditions, conditions.IgnoreLastTransitionTime(true)))
			}
		})
	}
}

type fakeClusterCache struct {
	clustercache.ClusterCache
	lastProbeSuccessTime time.Time
}

func (cc *fakeClusterCache) GetHealthCheckingState(_ context.Context, _ client.ObjectKey) clustercache.HealthCheckingState {
	return clustercache.HealthCheckingState{
		LastProbeTime:        time.Time{},
		LastProbeSuccessTime: cc.lastProbeSuccessTime,
		ConsecutiveFailures:  0,
	}
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
			name: "Requeue, if the deleting Machine has no Deleting condition",
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
			name: "Requeue, if the deleting Machine has Deleting condition false",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec: controlplanev1.KubeadmControlPlaneSpec{
						Version: "v1.31.0",
					},
				},
				Machines: collections.Machines{
					deletingMachineWithKCPPreTerminateHook.Name: func() *clusterv1.Machine {
						m := deletingMachineWithKCPPreTerminateHook.DeepCopy()
						conditions.Set(m, metav1.Condition{Type: clusterv1.MachineDeletingCondition, Status: metav1.ConditionFalse})
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
			name: "Requeue, if the deleting Machine has Deleting condition true but not waiting for hook",
			controlPlane: &internal.ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{
					Spec: controlplanev1.KubeadmControlPlaneSpec{
						Version: "v1.31.0",
					},
				},
				Machines: collections.Machines{
					deletingMachineWithKCPPreTerminateHook.Name: func() *clusterv1.Machine {
						m := deletingMachineWithKCPPreTerminateHook.DeepCopy()
						conditions.Set(m, metav1.Condition{Type: clusterv1.MachineDeletingCondition, Status: metav1.ConditionTrue, Reason: "Some other reason"})
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
						conditions.Set(m, metav1.Condition{Type: clusterv1.MachineDeletingCondition, Status: metav1.ConditionTrue, Reason: clusterv1.MachineDeletingWaitingForPreTerminateHookReason})
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
						conditions.Set(m, metav1.Condition{Type: clusterv1.MachineDeletingCondition, Status: metav1.ConditionTrue, Reason: clusterv1.MachineDeletingWaitingForPreTerminateHookReason})
						return m
					}(),
					deletingMachineWithKCPPreTerminateHook.Name + "-2": func() *clusterv1.Machine {
						m := deletingMachineWithKCPPreTerminateHook.DeepCopy()
						m.Name += "-2"
						conditions.Set(m, metav1.Condition{Type: clusterv1.MachineDeletingCondition, Status: metav1.ConditionTrue, Reason: clusterv1.MachineDeletingWaitingForPreTerminateHookReason})
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
						conditions.Set(m, metav1.Condition{Type: clusterv1.MachineDeletingCondition, Status: metav1.ConditionTrue, Reason: clusterv1.MachineDeletingWaitingForPreTerminateHookReason})
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
							ClusterConfiguration: bootstrapv1.ClusterConfiguration{
								Etcd: bootstrapv1.Etcd{
									External: bootstrapv1.ExternalEtcd{
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
						conditions.Set(m, metav1.Condition{Type: clusterv1.MachineDeletingCondition, Status: metav1.ConditionTrue, Reason: clusterv1.MachineDeletingWaitingForPreTerminateHookReason})
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
				ClusterConfiguration: bootstrapv1.ClusterConfiguration{
					DNS: bootstrapv1.DNS{
						ImageRepository: "registry.k8s.io",
						ImageTag:        "1.7.2",
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

		g.Expect(workloadCluster.UpdateCoreDNS(ctx, kcp)).To(Succeed())

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
		kcp.Spec.KubeadmConfigSpec.ClusterConfiguration = bootstrapv1.ClusterConfiguration{}

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

		g.Expect(workloadCluster.UpdateCoreDNS(ctx, kcp)).To(Succeed())
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

		g.Expect(workloadCluster.UpdateCoreDNS(ctx, kcp)).To(Succeed())
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

		g.Expect(workloadCluster.UpdateCoreDNS(ctx, kcp)).To(Succeed())
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

		g.Expect(workloadCluster.UpdateCoreDNS(ctx, kcp)).To(Succeed())

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

		g.Expect(workloadCluster.UpdateCoreDNS(ctx, kcp)).ToNot(Succeed())
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
		g.Expect(controlPlane.DeletingReason).To(Equal(controlplanev1.KubeadmControlPlaneDeletingWaitingForMachineDeletionReason))
		g.Expect(controlPlane.DeletingMessage).To(Equal("Deleting 3 Machines"))

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
		g.Expect(controlPlane.DeletingReason).To(Equal(controlplanev1.KubeadmControlPlaneDeletingDeletionCompletedReason))
		g.Expect(controlPlane.DeletingMessage).To(Equal("Deletion completed"))
	})

	t.Run("does not remove any control plane Machines if other Machines exist", func(t *testing.T) {
		g := NewWithT(t)

		cluster, kcp, _ := createClusterWithControlPlane(metav1.NamespaceDefault)
		controllerutil.AddFinalizer(kcp, controlplanev1.KubeadmControlPlaneFinalizer)

		initObjs := []client.Object{cluster.DeepCopy(), kcp.DeepCopy()}

		for i := range 10 {
			initObjs = append(initObjs, &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("worker-%d", i),
					Namespace: cluster.Namespace,
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: cluster.Name,
					},
				},
			})
		}

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
		g.Expect(controlPlane.DeletingReason).To(Equal(controlplanev1.KubeadmControlPlaneDeletingWaitingForWorkersDeletionReason))
		g.Expect(controlPlane.DeletingMessage).To(Equal("KubeadmControlPlane deletion blocked because following objects still exist:\n* Machines: worker-0, worker-1, worker-2, worker-3, worker-4, ... (5 more)"))

		controlPlaneMachines := clusterv1.MachineList{}
		labels := map[string]string{
			clusterv1.MachineControlPlaneLabel: "",
		}
		g.Expect(fakeClient.List(ctx, &controlPlaneMachines, client.MatchingLabels(labels))).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(HaveLen(3))
	})

	t.Run("does not remove any control plane Machines if MachinePools exist", func(t *testing.T) {
		utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.MachinePool, true)
		g := NewWithT(t)

		cluster, kcp, _ := createClusterWithControlPlane(metav1.NamespaceDefault)
		controllerutil.AddFinalizer(kcp, controlplanev1.KubeadmControlPlaneFinalizer)

		initObjs := []client.Object{cluster.DeepCopy(), kcp.DeepCopy()}

		for i := range 10 {
			initObjs = append(initObjs, &clusterv1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("mp-%d", i),
					Namespace: cluster.Namespace,
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: cluster.Name,
					},
				},
			})
		}

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
		g.Expect(controlPlane.DeletingReason).To(Equal(controlplanev1.KubeadmControlPlaneDeletingWaitingForWorkersDeletionReason))
		g.Expect(controlPlane.DeletingMessage).To(Equal("KubeadmControlPlane deletion blocked because following objects still exist:\n* MachinePools: mp-0, mp-1, mp-2, mp-3, mp-4, ... (5 more)"))

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
		g.Expect(controlPlane.DeletingReason).To(Equal(controlplanev1.KubeadmControlPlaneDeletingDeletionCompletedReason))
		g.Expect(controlPlane.DeletingMessage).To(Equal("Deletion completed"))
	})
}

func TestObjectsPendingDelete(t *testing.T) {
	c := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cluster",
		},
	}

	cpMachineLabels := map[string]string{
		clusterv1.ClusterNameLabel:         c.Name,
		clusterv1.MachineControlPlaneLabel: "",
	}
	workerMachineLabels := map[string]string{
		clusterv1.ClusterNameLabel: c.Name,
	}

	allMachines := collections.FromMachineList(&clusterv1.MachineList{
		Items: []clusterv1.Machine{
			*machine("cp1", withLabels(cpMachineLabels)),
			*machine("cp2", withLabels(cpMachineLabels)),
			*machine("cp3", withLabels(cpMachineLabels)),
			*machine("w1", withLabels(workerMachineLabels)),
			*machine("w2", withLabels(workerMachineLabels)),
			*machine("w3", withLabels(workerMachineLabels)),
			*machine("w4", withLabels(workerMachineLabels)),
			*machine("w5", withLabels(workerMachineLabels)),
			*machine("w6", withLabels(workerMachineLabels)),
			*machine("w7", withLabels(workerMachineLabels)),
			*machine("w8", withLabels(workerMachineLabels)),
		},
	})
	machinePools := &clusterv1.MachinePoolList{
		Items: []clusterv1.MachinePool{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mp1",
				},
			},
		},
	}

	g := NewWithT(t)

	g.Expect(objectsPendingDeleteNames(allMachines, machinePools, c)).To(Equal([]string{"MachinePools: mp1", "Machines: w1, w2, w3, w4, w5, ... (3 more)"}))
}

// test utils.

func newFakeClient(initObjs ...client.Object) client.WithWatch {
	// Use a new scheme to avoid side effects if multiple tests are sharing the same global scheme.
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	_ = bootstrapv1.AddToScheme(scheme)
	_ = controlplanev1.AddToScheme(scheme)
	return &fakeClient{
		startTime: time.Now(),
		WithWatch: fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjs...).WithStatusSubresource(&controlplanev1.KubeadmControlPlane{}).Build(),
	}
}

type fakeClient struct {
	startTime time.Time
	mux       sync.Mutex
	client.WithWatch
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
	return c.WithWatch.Create(ctx, obj, opts...)
}

func createClusterWithControlPlane(namespace string) (*clusterv1.Cluster, *controlplanev1.KubeadmControlPlane, *unstructured.Unstructured) {
	kcpName := fmt.Sprintf("kcp-foo-%s", util.RandomString(6))

	cluster := newCluster(&types.NamespacedName{Name: kcpName, Namespace: namespace})
	cluster.Spec = clusterv1.ClusterSpec{
		ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
			APIGroup: controlplanev1.GroupVersion.Group,
			Kind:     "KubeadmControlPlane",
			Name:     kcpName,
		},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
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
				Spec: controlplanev1.KubeadmControlPlaneMachineTemplateSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						Kind:     builder.GenericInfrastructureMachineTemplateKind,
						Name:     "infra-foo",
						APIGroup: clusterv1.GroupVersionInfrastructure.Group,
					},
				},
			},
			Replicas: ptr.To[int32](int32(3)),
			Version:  "v1.31.0",
			Rollout: controlplanev1.KubeadmControlPlaneRolloutSpec{
				Strategy: controlplanev1.KubeadmControlPlaneRolloutStrategy{
					Type: controlplanev1.RollingUpdateStrategyType,
					RollingUpdate: controlplanev1.KubeadmControlPlaneRolloutStrategyRollingUpdate{
						MaxSurge: &intstr.IntOrString{
							IntVal: 1,
						},
					},
				},
			},
		},
	}

	genericMachineTemplate := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       builder.GenericInfrastructureMachineTemplateKind,
			"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
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
	conditions.Set(kcp, metav1.Condition{Type: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition, Status: metav1.ConditionTrue})
	conditions.Set(kcp, metav1.Condition{Type: controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition, Status: metav1.ConditionTrue})
}

func createMachineNodePair(name string, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane, ready bool) (*clusterv1.Machine, *corev1.Node) {
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   cluster.Namespace,
			Name:        name,
			Labels:      desiredstate.ControlPlaneMachineLabels(kcp, cluster.Name),
			Annotations: map[string]string{},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane")),
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: cluster.Name,
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				Kind:     builder.GenericInfrastructureMachineKind,
				APIGroup: builder.InfrastructureGroupVersion.Group,
				Name:     "inframachine",
			},
		},
		Status: clusterv1.MachineStatus{
			NodeRef: clusterv1.MachineNodeReference{
				Name: name,
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
	m.Status.NodeRef = clusterv1.MachineNodeReference{
		Name: "node-1",
	}
	conditions.Set(m, metav1.Condition{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionTrue})
	conditions.Set(m, metav1.Condition{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionTrue})
	conditions.Set(m, metav1.Condition{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionTrue})
	conditions.Set(m, metav1.Condition{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionTrue})
	conditions.Set(m, metav1.Condition{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue})
}

// newCluster return a CAPI cluster object.
func newCluster(namespacedName *types.NamespacedName) *clusterv1.Cluster {
	return &clusterv1.Cluster{
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
