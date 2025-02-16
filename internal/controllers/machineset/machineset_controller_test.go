/*
Copyright 2018 The Kubernetes Authors.

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

package machineset

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

var _ reconcile.Reconciler = &Reconciler{}

func TestMachineSetReconciler(t *testing.T) {
	setup := func(t *testing.T, g *WithT) (*corev1.Namespace, *clusterv1.Cluster) {
		t.Helper()

		t.Log("Creating the namespace")
		ns, err := env.CreateNamespace(ctx, "test-machine-set-reconciler")
		g.Expect(err).ToNot(HaveOccurred())

		t.Log("Creating the Cluster")
		cluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Namespace: ns.Name, Name: testClusterName}}
		g.Expect(env.Create(ctx, cluster)).To(Succeed())

		t.Log("Creating the Cluster Kubeconfig Secret")
		g.Expect(env.CreateKubeconfigSecret(ctx, cluster)).To(Succeed())

		// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.InfrastructureReady = true
		g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())

		return ns, cluster
	}

	teardown := func(t *testing.T, g *WithT, ns *corev1.Namespace, cluster *clusterv1.Cluster) {
		t.Helper()

		t.Log("Deleting the Cluster")
		g.Expect(env.Delete(ctx, cluster)).To(Succeed())
		t.Log("Deleting the namespace")
		g.Expect(env.Delete(ctx, ns)).To(Succeed())
	}

	t.Run("Should reconcile a MachineSet", func(t *testing.T) {
		g := NewWithT(t)
		namespace, testCluster := setup(t, g)
		defer teardown(t, g, namespace, testCluster)

		duration10m := &metav1.Duration{Duration: 10 * time.Minute}
		duration5m := &metav1.Duration{Duration: 5 * time.Minute}
		replicas := int32(2)
		version := "v1.14.2"
		machineTemplateSpec := clusterv1.MachineTemplateSpec{
			ObjectMeta: clusterv1.ObjectMeta{
				Labels: map[string]string{
					"label-1": "true",
				},
				Annotations: map[string]string{
					"annotation-1": "true",
					"precedence":   "MachineSet",
				},
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: testCluster.Name,
				Version:     &version,
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &corev1.ObjectReference{
						APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
						Kind:       "GenericBootstrapConfigTemplate",
						Name:       "ms-template",
						Namespace:  namespace.Name,
					},
				},
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
					Kind:       "GenericInfrastructureMachineTemplate",
					Name:       "ms-template",
					Namespace:  namespace.Name,
				},
				NodeDrainTimeout:        duration10m,
				NodeDeletionTimeout:     duration10m,
				NodeVolumeDetachTimeout: duration10m,
			},
		}

		machineDeployment := &clusterv1.MachineDeployment{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "md-",
				Namespace:    namespace.Name,
				Annotations: map[string]string{
					clusterv1.RevisionAnnotation: "10",
				},
			},
			Spec: clusterv1.MachineDeploymentSpec{
				ClusterName: testCluster.Name,
				Replicas:    &replicas,
				Template:    machineTemplateSpec,
			},
		}
		g.Expect(env.Create(ctx, machineDeployment)).To(Succeed())

		instance := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "ms-",
				Namespace:    namespace.Name,
				Labels: map[string]string{
					"label-1":                            "true",
					clusterv1.MachineDeploymentNameLabel: machineDeployment.Name,
				},
				Annotations: map[string]string{
					clusterv1.RevisionAnnotation: "10",
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "MachineDeployment",
						Name:       machineDeployment.Name,
						UID:        machineDeployment.UID,
					},
				},
			},
			Spec: clusterv1.MachineSetSpec{
				ClusterName: testCluster.Name,
				Replicas:    &replicas,
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"label-1": "true",
					},
				},
				Template: machineTemplateSpec,
			},
		}

		// Create bootstrap template resource.
		bootstrapResource := map[string]interface{}{
			"kind":       "GenericBootstrapConfig",
			"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"precedence": "GenericBootstrapConfig",
				},
			},
		}
		bootstrapTmpl := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": bootstrapResource,
				},
			},
		}
		bootstrapTmpl.SetKind("GenericBootstrapConfigTemplate")
		bootstrapTmpl.SetAPIVersion("bootstrap.cluster.x-k8s.io/v1beta1")
		bootstrapTmpl.SetName("ms-template")
		bootstrapTmpl.SetNamespace(namespace.Name)
		g.Expect(env.Create(ctx, bootstrapTmpl)).To(Succeed())

		// Create infrastructure template resource.
		infraResource := map[string]interface{}{
			"kind":       "GenericInfrastructureMachine",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"precedence": "GenericInfrastructureMachineTemplate",
				},
			},
			"spec": map[string]interface{}{
				"size": "3xlarge",
			},
		}
		infraTmpl := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": infraResource,
				},
			},
		}
		infraTmpl.SetKind("GenericInfrastructureMachineTemplate")
		infraTmpl.SetAPIVersion("infrastructure.cluster.x-k8s.io/v1beta1")
		infraTmpl.SetName("ms-template")
		infraTmpl.SetNamespace(namespace.Name)
		g.Expect(env.Create(ctx, infraTmpl)).To(Succeed())

		// Create the MachineSet.
		g.Expect(env.Create(ctx, instance)).To(Succeed())
		defer func() {
			g.Expect(env.Delete(ctx, instance)).To(Succeed())
		}()

		t.Log("Verifying the linked bootstrap template has a cluster owner reference")
		g.Eventually(func() bool {
			obj, err := external.Get(ctx, env, instance.Spec.Template.Spec.Bootstrap.ConfigRef)
			if err != nil {
				return false
			}

			return util.HasOwnerRef(obj.GetOwnerReferences(), metav1.OwnerReference{
				APIVersion: clusterv1.GroupVersion.String(),
				Kind:       "Cluster",
				Name:       testCluster.Name,
				UID:        testCluster.UID,
			})
		}, timeout).Should(BeTrue())

		t.Log("Verifying the linked infrastructure template has a cluster owner reference")
		g.Eventually(func() bool {
			obj, err := external.Get(ctx, env, &instance.Spec.Template.Spec.InfrastructureRef)
			if err != nil {
				return false
			}

			return util.HasOwnerRef(obj.GetOwnerReferences(), metav1.OwnerReference{
				APIVersion: clusterv1.GroupVersion.String(),
				Kind:       "Cluster",
				Name:       testCluster.Name,
				UID:        testCluster.UID,
			})
		}, timeout).Should(BeTrue())

		machines := &clusterv1.MachineList{}

		// Verify that we have 2 replicas.
		g.Eventually(func() int {
			if err := env.List(ctx, machines, client.InNamespace(namespace.Name)); err != nil {
				return -1
			}
			return len(machines.Items)
		}, timeout).Should(BeEquivalentTo(replicas))

		t.Log("Creating a InfrastructureMachine for each Machine")
		infraMachines := &unstructured.UnstructuredList{}
		infraMachines.SetAPIVersion("infrastructure.cluster.x-k8s.io/v1beta1")
		infraMachines.SetKind("GenericInfrastructureMachine")
		g.Eventually(func() int {
			if err := env.List(ctx, infraMachines, client.InNamespace(namespace.Name)); err != nil {
				return -1
			}
			return len(machines.Items)
		}, timeout).Should(BeEquivalentTo(replicas))
		for _, im := range infraMachines.Items {
			g.Expect(im.GetAnnotations()).To(HaveKeyWithValue("annotation-1", "true"), "have annotations of MachineTemplate applied")
			g.Expect(im.GetAnnotations()).To(HaveKeyWithValue("precedence", "MachineSet"), "the annotations from the MachineSpec template to overwrite the infrastructure template ones")
			g.Expect(im.GetLabels()).To(HaveKeyWithValue("label-1", "true"), "have labels of MachineTemplate applied")
		}
		g.Eventually(func() bool {
			g.Expect(env.List(ctx, infraMachines, client.InNamespace(namespace.Name))).To(Succeed())
			// The Machine reconciler should remove the ownerReference to the MachineSet on the InfrastructureMachine.
			hasMSOwnerRef := false
			hasMachineOwnerRef := false
			for _, im := range infraMachines.Items {
				for _, o := range im.GetOwnerReferences() {
					if o.Kind == machineSetKind.Kind {
						hasMSOwnerRef = true
					}
					if o.Kind == "Machine" {
						hasMachineOwnerRef = true
					}
				}
			}
			return !hasMSOwnerRef && hasMachineOwnerRef
		}, timeout).Should(BeTrue(), "infraMachine should not have ownerRef to MachineSet")

		t.Log("Creating a BootstrapConfig for each Machine")
		bootstrapConfigs := &unstructured.UnstructuredList{}
		bootstrapConfigs.SetAPIVersion("bootstrap.cluster.x-k8s.io/v1beta1")
		bootstrapConfigs.SetKind("GenericBootstrapConfig")
		g.Eventually(func() int {
			if err := env.List(ctx, bootstrapConfigs, client.InNamespace(namespace.Name)); err != nil {
				return -1
			}
			return len(machines.Items)
		}, timeout).Should(BeEquivalentTo(replicas))
		for _, im := range bootstrapConfigs.Items {
			g.Expect(im.GetAnnotations()).To(HaveKeyWithValue("annotation-1", "true"), "have annotations of MachineTemplate applied")
			g.Expect(im.GetAnnotations()).To(HaveKeyWithValue("precedence", "MachineSet"), "the annotations from the MachineSpec template to overwrite the bootstrap config template ones")
			g.Expect(im.GetLabels()).To(HaveKeyWithValue("label-1", "true"), "have labels of MachineTemplate applied")
		}
		g.Eventually(func() bool {
			g.Expect(env.List(ctx, bootstrapConfigs, client.InNamespace(namespace.Name))).To(Succeed())
			// The Machine reconciler should remove the ownerReference to the MachineSet on the Bootstrap object.
			hasMSOwnerRef := false
			hasMachineOwnerRef := false
			for _, im := range bootstrapConfigs.Items {
				for _, o := range im.GetOwnerReferences() {
					if o.Kind == machineSetKind.Kind {
						hasMSOwnerRef = true
					}
					if o.Kind == "Machine" {
						hasMachineOwnerRef = true
					}
				}
			}
			return !hasMSOwnerRef && hasMachineOwnerRef
		}, timeout).Should(BeTrue(), "bootstrap should not have ownerRef to MachineSet")

		// Set the infrastructure reference as ready.
		for _, m := range machines.Items {
			fakeBootstrapRefReady(*m.Spec.Bootstrap.ConfigRef, bootstrapResource, g)
			fakeInfrastructureRefReady(m.Spec.InfrastructureRef, infraResource, g)
		}

		// Verify that in-place mutable fields propagate from MachineSet to Machines.
		t.Log("Updating NodeDrainTimeout on MachineSet")
		patchHelper, err := patch.NewHelper(instance, env)
		g.Expect(err).ToNot(HaveOccurred())
		instance.Spec.Template.Spec.NodeDrainTimeout = duration5m
		g.Expect(patchHelper.Patch(ctx, instance)).Should(Succeed())

		t.Log("Verifying new NodeDrainTimeout value is set on Machines")
		g.Eventually(func() bool {
			if err := env.List(ctx, machines, client.InNamespace(namespace.Name)); err != nil {
				return false
			}
			// All the machines should have the new NodeDrainTimeoutValue
			for _, m := range machines.Items {
				if m.Spec.NodeDrainTimeout == nil {
					return false
				}
				if m.Spec.NodeDrainTimeout.Duration != duration5m.Duration {
					return false
				}
			}
			return true
		}, timeout).Should(BeTrue(), "machine should have the updated NodeDrainTimeout value")

		// Try to delete 1 machine and check the MachineSet scales back up.
		machineToBeDeleted := machines.Items[0]
		g.Expect(env.Delete(ctx, &machineToBeDeleted)).To(Succeed())

		// Verify that the Machine has been deleted.
		g.Eventually(func() bool {
			key := client.ObjectKey{Name: machineToBeDeleted.Name, Namespace: machineToBeDeleted.Namespace}
			if err := env.Get(ctx, key, &machineToBeDeleted); apierrors.IsNotFound(err) || !machineToBeDeleted.DeletionTimestamp.IsZero() {
				return true
			}
			return false
		}, timeout).Should(BeTrue())

		// Verify that we have 2 replicas.
		g.Eventually(func() (ready int) {
			if err := env.List(ctx, machines, client.InNamespace(namespace.Name)); err != nil {
				return -1
			}
			for _, m := range machines.Items {
				if !m.DeletionTimestamp.IsZero() {
					continue
				}
				ready++
			}
			return
		}, timeout*3).Should(BeEquivalentTo(replicas))

		// Verify that each machine has the desired kubelet version,
		// create a fake node in Ready state, update NodeRef, and wait for a reconciliation request.
		for i := range len(machines.Items) {
			m := machines.Items[i]
			if !m.DeletionTimestamp.IsZero() {
				// Skip deleted Machines
				continue
			}

			g.Expect(m.Spec.Version).ToNot(BeNil())
			g.Expect(*m.Spec.Version).To(BeEquivalentTo("v1.14.2"))
			fakeBootstrapRefReady(*m.Spec.Bootstrap.ConfigRef, bootstrapResource, g)
			providerID := fakeInfrastructureRefReady(m.Spec.InfrastructureRef, infraResource, g)
			fakeMachineNodeRef(&m, providerID, g)
		}

		// Verify that all Machines are Ready.
		g.Eventually(func() int32 {
			key := client.ObjectKey{Name: instance.Name, Namespace: instance.Namespace}
			if err := env.Get(ctx, key, instance); err != nil {
				return -1
			}
			return instance.Status.AvailableReplicas
		}, timeout).Should(BeEquivalentTo(replicas))

		t.Log("Verifying MachineSet has MachinesCreatedCondition")
		g.Eventually(func() bool {
			key := client.ObjectKey{Name: instance.Name, Namespace: instance.Namespace}
			if err := env.Get(ctx, key, instance); err != nil {
				return false
			}
			return conditions.IsTrue(instance, clusterv1.MachinesCreatedCondition)
		}, timeout).Should(BeTrue())

		t.Log("Verifying MachineSet has ResizedCondition")
		g.Eventually(func() bool {
			key := client.ObjectKey{Name: instance.Name, Namespace: instance.Namespace}
			if err := env.Get(ctx, key, instance); err != nil {
				return false
			}
			return conditions.IsTrue(instance, clusterv1.ResizedCondition)
		}, timeout).Should(BeTrue())

		t.Log("Verifying MachineSet has MachinesReadyCondition")
		g.Eventually(func() bool {
			key := client.ObjectKey{Name: instance.Name, Namespace: instance.Namespace}
			if err := env.Get(ctx, key, instance); err != nil {
				return false
			}
			return conditions.IsTrue(instance, clusterv1.MachinesReadyCondition)
		}, timeout).Should(BeTrue())

		// Validate that the controller set the cluster name label in selector.
		g.Expect(instance.Status.Selector).To(ContainSubstring(testCluster.Name))

		t.Log("Verifying MachineSet can be scaled down when templates don't exist, and MachineSet is not current")
		g.Expect(env.CleanupAndWait(ctx, bootstrapTmpl)).To(Succeed())
		g.Expect(env.CleanupAndWait(ctx, infraTmpl)).To(Succeed())

		t.Log("Updating Replicas on MachineSet")
		patchHelper, err = patch.NewHelper(instance, env)
		g.Expect(err).ToNot(HaveOccurred())
		instance.SetAnnotations(map[string]string{
			clusterv1.RevisionAnnotation: "9",
		})
		instance.Spec.Replicas = ptr.To(int32(1))
		g.Expect(patchHelper.Patch(ctx, instance)).Should(Succeed())

		// Verify that we have 1 replicas.
		g.Eventually(func() (ready int) {
			if err := env.List(ctx, machines, client.InNamespace(namespace.Name)); err != nil {
				return -1
			}
			for _, m := range machines.Items {
				if !m.DeletionTimestamp.IsZero() {
					continue
				}
				ready++
			}
			return
		}, timeout*3).Should(BeEquivalentTo(1))
	})
}

func TestMachineSetOwnerReference(t *testing.T) {
	testCluster := &clusterv1.Cluster{
		TypeMeta:   metav1.TypeMeta{Kind: "Cluster", APIVersion: clusterv1.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: testClusterName},
	}

	validMD := &clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "valid-machinedeployment",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: testCluster.Name,
			},
		},
		Spec: clusterv1.MachineDeploymentSpec{
			ClusterName: testCluster.Name,
		},
	}

	ms1 := newMachineSet("machineset1", "valid-cluster", int32(0))
	ms2 := newMachineSet("machineset2", "invalid-cluster", int32(0))
	ms3 := newMachineSet("machineset3", "valid-cluster", int32(0))
	ms3.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "MachineDeployment",
			Name:       "valid-machinedeployment",
		},
	}

	testCases := []struct {
		name               string
		request            reconcile.Request
		ms                 *clusterv1.MachineSet
		expectReconcileErr bool
		expectedOR         []metav1.OwnerReference
	}{
		{
			name: "should add cluster owner reference to machine set",
			request: reconcile.Request{
				NamespacedName: util.ObjectKey(ms1),
			},
			ms: ms1,
			expectedOR: []metav1.OwnerReference{
				{
					APIVersion: testCluster.APIVersion,
					Kind:       testCluster.Kind,
					Name:       testCluster.Name,
					UID:        testCluster.UID,
				},
			},
		},
		{
			name: "should not add cluster owner reference if machine is owned by a machine deployment",
			request: reconcile.Request{
				NamespacedName: util.ObjectKey(ms3),
			},
			ms: ms3,
			expectedOR: []metav1.OwnerReference{
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "MachineDeployment",
					Name:       "valid-machinedeployment",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			c := fake.NewClientBuilder().WithObjects(
				testCluster,
				ms1,
				ms2,
				ms3,
				validMD,
			).WithStatusSubresource(&clusterv1.MachineSet{}).Build()
			msr := &Reconciler{
				Client:   c,
				recorder: record.NewFakeRecorder(32),
			}

			_, err := msr.Reconcile(ctx, tc.request)
			if tc.expectReconcileErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			key := client.ObjectKey{Namespace: tc.ms.Namespace, Name: tc.ms.Name}
			var actual clusterv1.MachineSet
			if len(tc.expectedOR) > 0 {
				g.Expect(msr.Client.Get(ctx, key, &actual)).To(Succeed())
				g.Expect(actual.OwnerReferences).To(BeComparableTo(tc.expectedOR))
			} else {
				g.Expect(actual.OwnerReferences).To(BeEmpty())
			}
		})
	}
}

func TestMachineSetReconcile(t *testing.T) {
	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: testClusterName},
	}

	t.Run("ignore machine sets marked for deletion", func(t *testing.T) {
		g := NewWithT(t)

		dt := metav1.Now()
		ms := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "machineset1",
				Namespace:         metav1.NamespaceDefault,
				DeletionTimestamp: &dt,
				Finalizers:        []string{"block-deletion"},
			},
			Spec: clusterv1.MachineSetSpec{
				ClusterName: testClusterName,
				Replicas:    ptr.To[int32](0),
			},
			Status: clusterv1.MachineSetStatus{
				V1Beta2: &clusterv1.MachineSetV1Beta2Status{Conditions: []metav1.Condition{{
					Type:   clusterv1.PausedV1Beta2Condition,
					Status: metav1.ConditionFalse,
					Reason: clusterv1.NotPausedV1Beta2Reason,
				}}},
			},
		}
		request := reconcile.Request{
			NamespacedName: util.ObjectKey(ms),
		}

		c := fake.NewClientBuilder().WithObjects(testCluster, ms).WithStatusSubresource(&clusterv1.MachineSet{}).Build()
		msr := &Reconciler{
			Client:   c,
			recorder: record.NewFakeRecorder(32),
		}
		result, err := msr.Reconcile(ctx, request)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(BeComparableTo(reconcile.Result{}))
	})

	t.Run("reconcile successfully when labels are missing", func(t *testing.T) {
		g := NewWithT(t)

		ms := newMachineSet("machineset1", testClusterName, int32(0))
		ms.Labels = nil
		ms.Spec.Selector.MatchLabels = nil
		ms.Spec.Template.Labels = nil

		request := reconcile.Request{
			NamespacedName: util.ObjectKey(ms),
		}

		rec := record.NewFakeRecorder(32)
		c := fake.NewClientBuilder().WithObjects(testCluster, ms).WithStatusSubresource(&clusterv1.MachineSet{}).Build()
		msr := &Reconciler{
			Client:   c,
			recorder: rec,
		}
		_, err := msr.Reconcile(ctx, request)
		g.Expect(err).ToNot(HaveOccurred())
	})
}

func TestMachineSetToMachines(t *testing.T) {
	machineSetList := []client.Object{
		&clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "withMatchingLabels",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: clusterv1.MachineSetSpec{
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"foo":                      "bar",
						clusterv1.ClusterNameLabel: testClusterName,
					},
				},
			},
		},
	}
	controller := true
	m := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withOwnerRef",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: testClusterName,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Name:       "Owner",
					Kind:       machineSetKind.Kind,
					Controller: &controller,
				},
			},
		},
	}
	m2 := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "noOwnerRefNoLabels",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: testClusterName,
			},
		},
	}
	m3 := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withMatchingLabels",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				"foo":                      "bar",
				clusterv1.ClusterNameLabel: testClusterName,
			},
		},
	}
	testsCases := []struct {
		name      string
		mapObject client.Object
		expected  []reconcile.Request
	}{
		{
			name:      "should return empty request when controller is set",
			mapObject: &m,
			expected:  []reconcile.Request{},
		},
		{
			name:      "should return nil if machine has no owner reference",
			mapObject: &m2,
			expected:  nil,
		},
		{
			name:      "should return request if machine set's labels matches machine's labels",
			mapObject: &m3,
			expected: []reconcile.Request{
				{NamespacedName: client.ObjectKey{Namespace: metav1.NamespaceDefault, Name: "withMatchingLabels"}},
			},
		},
	}

	c := fake.NewClientBuilder().WithObjects(append(machineSetList, &m, &m2, &m3)...).Build()
	r := &Reconciler{
		Client: c,
	}

	for _, tc := range testsCases {
		t.Run(tc.name, func(t *testing.T) {
			gs := NewWithT(t)

			got := r.MachineToMachineSets(ctx, tc.mapObject)
			gs.Expect(got).To(BeComparableTo(tc.expected))
		})
	}
}

func TestShouldExcludeMachine(t *testing.T) {
	controller := true
	testCases := []struct {
		machineSet clusterv1.MachineSet
		machine    clusterv1.Machine
		expected   bool
	}{
		{
			machineSet: clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{UID: "1"},
			},
			machine: clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "withNoMatchingOwnerRef",
					Namespace: metav1.NamespaceDefault,
					OwnerReferences: []metav1.OwnerReference{
						{
							Name:       "Owner",
							Kind:       machineSetKind.Kind,
							Controller: &controller,
							UID:        "not-1",
						},
					},
				},
			},
			expected: true,
		},
		{
			machineSet: clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{UID: "1"},
			},
			machine: clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "withMatchingOwnerRef",
					Namespace: metav1.NamespaceDefault,
					OwnerReferences: []metav1.OwnerReference{
						{
							Name:       "Owner",
							Kind:       machineSetKind.Kind,
							Controller: &controller,
							UID:        "1",
						},
					},
				},
			},
			expected: false,
		},
		{
			machineSet: clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
			machine: clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "withMatchingLabels",
					Namespace: metav1.NamespaceDefault,
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			},
			expected: false,
		},
		{
			machineSet: clusterv1.MachineSet{},
			machine: clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "withDeletionTimestamp",
					Namespace:         metav1.NamespaceDefault,
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			},
			expected: false,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		g := NewWithT(t)

		got := shouldExcludeMachine(&tc.machineSet, &tc.machine)

		g.Expect(got).To(Equal(tc.expected))
	}
}

func TestAdoptOrphan(t *testing.T) {
	g := NewWithT(t)

	m := clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "orphanMachine",
		},
	}
	ms := clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "adoptOrphanMachine",
		},
	}
	controller := true
	blockOwnerDeletion := true
	testCases := []struct {
		machineSet clusterv1.MachineSet
		machine    clusterv1.Machine
		expected   []metav1.OwnerReference
	}{
		{
			machine:    m,
			machineSet: ms,
			expected: []metav1.OwnerReference{
				{
					APIVersion:         clusterv1.GroupVersion.String(),
					Kind:               machineSetKind.Kind,
					Name:               "adoptOrphanMachine",
					UID:                "",
					Controller:         &controller,
					BlockOwnerDeletion: &blockOwnerDeletion,
				},
			},
		},
	}

	c := fake.NewClientBuilder().WithObjects(&m).Build()
	r := &Reconciler{
		Client: c,
	}
	for i := range testCases {
		tc := testCases[i]
		g.Expect(r.adoptOrphan(ctx, tc.machineSet.DeepCopy(), tc.machine.DeepCopy())).To(Succeed())

		key := client.ObjectKey{Namespace: tc.machine.Namespace, Name: tc.machine.Name}
		g.Expect(r.Client.Get(ctx, key, &tc.machine)).To(Succeed())

		got := tc.machine.GetOwnerReferences()
		g.Expect(got).To(BeComparableTo(tc.expected))
	}
}

func newMachineSet(name, cluster string, replicas int32) *clusterv1.MachineSet {
	return &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
			Finalizers: []string{
				clusterv1.MachineSetFinalizer,
			},
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: cluster,
			},
		},
		Spec: clusterv1.MachineSetSpec{
			ClusterName: testClusterName,
			Replicas:    &replicas,
			Template: clusterv1.MachineTemplateSpec{
				ObjectMeta: clusterv1.ObjectMeta{
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: cluster,
					},
				},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					clusterv1.ClusterNameLabel: cluster,
				},
			},
		},
		Status: clusterv1.MachineSetStatus{
			V1Beta2: &clusterv1.MachineSetV1Beta2Status{Conditions: []metav1.Condition{{
				Type:   clusterv1.PausedV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.NotPausedV1Beta2Reason,
			}}},
		},
	}
}

func TestMachineSetReconcile_MachinesCreatedConditionFalseOnBadInfraRef(t *testing.T) {
	g := NewWithT(t)
	replicas := int32(1)
	version := "v1.21.0"
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
	}

	ms := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ms-foo",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: cluster.Name,
			},
			Finalizers: []string{
				clusterv1.MachineSetFinalizer,
			},
		},
		Spec: clusterv1.MachineSetSpec{
			ClusterName: cluster.ObjectMeta.Name,
			Replicas:    &replicas,
			Template: clusterv1.MachineTemplateSpec{
				ObjectMeta: clusterv1.ObjectMeta{
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: cluster.Name,
					},
				},
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: corev1.ObjectReference{
						Kind:       builder.GenericInfrastructureMachineTemplateCRD.Kind,
						APIVersion: builder.GenericInfrastructureMachineTemplateCRD.APIVersion,
						// Try to break Infra Cloning
						Name:      "something_invalid",
						Namespace: cluster.Namespace,
					},
					Version: &version,
				},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
			},
		},
		Status: clusterv1.MachineSetStatus{
			V1Beta2: &clusterv1.MachineSetV1Beta2Status{Conditions: []metav1.Condition{{
				Type:   clusterv1.PausedV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.NotPausedV1Beta2Reason,
			}}},
		},
	}

	key := util.ObjectKey(ms)
	request := reconcile.Request{
		NamespacedName: key,
	}
	fakeClient := fake.NewClientBuilder().WithObjects(cluster, ms, builder.GenericInfrastructureMachineTemplateCRD.DeepCopy()).WithStatusSubresource(&clusterv1.MachineSet{}).Build()

	msr := &Reconciler{
		Client:   fakeClient,
		recorder: record.NewFakeRecorder(32),
	}
	_, err := msr.Reconcile(ctx, request)
	g.Expect(err).To(HaveOccurred())
	g.Expect(fakeClient.Get(ctx, key, ms)).To(Succeed())
	gotCond := conditions.Get(ms, clusterv1.MachinesCreatedCondition)
	g.Expect(gotCond).ToNot(BeNil())
	g.Expect(gotCond.Status).To(Equal(corev1.ConditionFalse))
	g.Expect(gotCond.Reason).To(Equal(clusterv1.InfrastructureTemplateCloningFailedReason))
}

func TestMachineSetReconciler_updateStatusResizedCondition(t *testing.T) {
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
	}

	testCases := []struct {
		name            string
		machineSet      *clusterv1.MachineSet
		machines        []*clusterv1.Machine
		expectedReason  string
		expectedMessage string
	}{
		{
			name:            "MachineSet should have ResizedCondition=false on scale up",
			machineSet:      newMachineSet("ms-scale-up", cluster.Name, int32(1)),
			machines:        []*clusterv1.Machine{},
			expectedReason:  clusterv1.ScalingUpReason,
			expectedMessage: "Scaling up MachineSet to 1 replicas (actual 0)",
		},
		{
			name:       "MachineSet should have ResizedCondition=false on scale down",
			machineSet: newMachineSet("ms-scale-down", cluster.Name, int32(0)),
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "machine-a",
						Namespace: metav1.NamespaceDefault,
						Labels: map[string]string{
							clusterv1.ClusterNameLabel: cluster.Name,
						},
					},
				},
			},
			expectedReason:  clusterv1.ScalingDownReason,
			expectedMessage: "Scaling down MachineSet to 0 replicas (actual 1)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			c := fake.NewClientBuilder().WithObjects().Build()
			msr := &Reconciler{
				Client:   c,
				recorder: record.NewFakeRecorder(32),
			}
			s := &scope{
				cluster:    cluster,
				machineSet: tc.machineSet,
				machines:   tc.machines,
				getAndAdoptMachinesForMachineSetSucceeded: true,
			}
			setReplicas(ctx, s.machineSet, s.machines, tc.machines != nil)
			g.Expect(msr.reconcileStatus(ctx, s)).To(Succeed())
			gotCond := conditions.Get(tc.machineSet, clusterv1.ResizedCondition)
			g.Expect(gotCond).ToNot(BeNil())
			g.Expect(gotCond.Status).To(Equal(corev1.ConditionFalse))
			g.Expect(gotCond.Reason).To(Equal(tc.expectedReason))
			g.Expect(gotCond.Message).To(Equal(tc.expectedMessage))
		})
	}
}

func TestMachineSetReconciler_syncMachines(t *testing.T) {
	setup := func(t *testing.T, g *WithT) (*corev1.Namespace, *clusterv1.Cluster) {
		t.Helper()

		t.Log("Creating the namespace")
		ns, err := env.CreateNamespace(ctx, "test-machine-set-reconciler-sync-machines")
		g.Expect(err).ToNot(HaveOccurred())

		t.Log("Creating the Cluster")
		cluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Namespace: ns.Name, Name: testClusterName}}
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
	replicas := int32(2)
	version := "v1.25.3"
	duration10s := &metav1.Duration{Duration: 10 * time.Second}
	duration11s := &metav1.Duration{Duration: 11 * time.Second}
	ms := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "abc-123-ms-uid",
			Name:      "ms-1",
			Namespace: namespace.Name,
			Labels: map[string]string{
				"label-1":                            "true",
				clusterv1.MachineDeploymentNameLabel: "md-1",
			},
		},
		Spec: clusterv1.MachineSetSpec{
			ClusterName: testCluster.Name,
			Replicas:    &replicas,
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"preserved-label": "preserved-value",
				},
			},
			Template: clusterv1.MachineTemplateSpec{
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
				Spec: clusterv1.MachineSpec{
					ClusterName: testCluster.Name,
					Version:     &version,
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
							Kind:       "GenericBootstrapConfigTemplate",
							Name:       "ms-template",
						},
					},
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
						Kind:       "GenericInfrastructureMachineTemplate",
						Name:       "ms-template",
					},
				},
			},
		},
	}

	infraMachineSpec := map[string]interface{}{
		"infra-field": "infra-value",
	}
	infraMachine := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericInfrastructureMachine",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"name":      "infra-machine-1",
				"namespace": namespace.Name,
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
	g.Expect(env.Create(ctx, infraMachine, client.FieldOwner(classicManager))).To(Succeed())

	bootstrapConfigSpec := map[string]interface{}{
		"bootstrap-field": "bootstrap-value",
	}
	bootstrapConfig := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericBootstrapConfig",
			"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"name":      "bootstrap-config-1",
				"namespace": namespace.Name,
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
			"spec": bootstrapConfigSpec,
		},
	}
	g.Expect(env.Create(ctx, bootstrapConfig, client.FieldOwner(classicManager))).To(Succeed())

	inPlaceMutatingMachine := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			UID:       "abc-123-uid",
			Name:      "in-place-mutating-machine",
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
			ClusterName: testClusterName,
			InfrastructureRef: corev1.ObjectReference{
				Namespace:  infraMachine.GetNamespace(),
				Name:       infraMachine.GetName(),
				UID:        infraMachine.GetUID(),
				APIVersion: infraMachine.GetAPIVersion(),
				Kind:       infraMachine.GetKind(),
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					Namespace:  bootstrapConfig.GetNamespace(),
					Name:       bootstrapConfig.GetName(),
					UID:        bootstrapConfig.GetUID(),
					APIVersion: bootstrapConfig.GetAPIVersion(),
					Kind:       bootstrapConfig.GetKind(),
				},
			},
		},
	}
	g.Expect(env.Create(ctx, inPlaceMutatingMachine, client.FieldOwner(classicManager))).To(Succeed())

	deletingMachine := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			UID:         "abc-123-uid",
			Name:        "deleting-machine",
			Namespace:   namespace.Name,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
			Finalizers:  []string{"testing-finalizer"},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: testClusterName,
			InfrastructureRef: corev1.ObjectReference{
				Namespace: namespace.Name,
			},
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: ptr.To("machine-bootstrap-secret"),
			},
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
	}, timeout).Should(BeTrue())

	machines := []*clusterv1.Machine{inPlaceMutatingMachine, deletingMachine}

	//
	// Verify Managed Fields
	//

	// Run syncMachines to clean up managed fields and have proper field ownership
	// for Machines, InfrastructureMachines and BootstrapConfigs.
	reconciler := &Reconciler{
		Client:   env,
		ssaCache: ssa.NewCache("test-controller"),
	}
	s := &scope{
		machineSet: ms,
		machines:   machines,
		getAndAdoptMachinesForMachineSetSucceeded: true,
	}
	_, err := reconciler.syncMachines(ctx, s)
	g.Expect(err).ToNot(HaveOccurred())

	// The inPlaceMutatingMachine should have cleaned up managed fields.
	updatedInPlaceMutatingMachine := inPlaceMutatingMachine.DeepCopy()
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedInPlaceMutatingMachine), updatedInPlaceMutatingMachine)).To(Succeed())
	// Verify ManagedFields
	g.Expect(updatedInPlaceMutatingMachine.ManagedFields).Should(
		ContainElement(ssa.MatchManagedFieldsEntry(machineSetManagerName, metav1.ManagedFieldsOperationApply)),
		"in-place mutable machine should contain an entry for SSA manager",
	)
	g.Expect(updatedInPlaceMutatingMachine.ManagedFields).ShouldNot(
		ContainElement(ssa.MatchManagedFieldsEntry(classicManager, metav1.ManagedFieldsOperationUpdate)),
		"in-place mutable machine should not contain an entry for old manager",
	)

	// The InfrastructureMachine should have ownership of "labels" and "annotations" transferred to
	// "capi-machineset" manager.
	updatedInfraMachine := infraMachine.DeepCopy()
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedInfraMachine), updatedInfraMachine)).To(Succeed())

	// Verify ManagedFields
	g.Expect(updatedInfraMachine.GetManagedFields()).Should(
		ssa.MatchFieldOwnership(machineSetManagerName, metav1.ManagedFieldsOperationApply, contract.Path{"f:metadata", "f:labels"}))
	g.Expect(updatedInfraMachine.GetManagedFields()).Should(
		ssa.MatchFieldOwnership(machineSetManagerName, metav1.ManagedFieldsOperationApply, contract.Path{"f:metadata", "f:annotations"}))
	g.Expect(updatedInfraMachine.GetManagedFields()).ShouldNot(
		ssa.MatchFieldOwnership(classicManager, metav1.ManagedFieldsOperationUpdate, contract.Path{"f:metadata", "f:labels"}))
	g.Expect(updatedInfraMachine.GetManagedFields()).ShouldNot(
		ssa.MatchFieldOwnership(classicManager, metav1.ManagedFieldsOperationUpdate, contract.Path{"f:metadata", "f:annotations"}))
	g.Expect(updatedInfraMachine.GetManagedFields()).Should(
		ssa.MatchFieldOwnership(classicManager, metav1.ManagedFieldsOperationUpdate, contract.Path{"f:spec"}))

	// The BootstrapConfig should have ownership of "labels" and "annotations" transferred to
	// "capi-machineset" manager.
	updatedBootstrapConfig := bootstrapConfig.DeepCopy()
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedBootstrapConfig), updatedBootstrapConfig)).To(Succeed())

	// Verify ManagedFields
	g.Expect(updatedBootstrapConfig.GetManagedFields()).Should(
		ssa.MatchFieldOwnership(machineSetManagerName, metav1.ManagedFieldsOperationApply, contract.Path{"f:metadata", "f:labels"}))
	g.Expect(updatedBootstrapConfig.GetManagedFields()).Should(
		ssa.MatchFieldOwnership(machineSetManagerName, metav1.ManagedFieldsOperationApply, contract.Path{"f:metadata", "f:annotations"}))
	g.Expect(updatedBootstrapConfig.GetManagedFields()).ShouldNot(
		ssa.MatchFieldOwnership(classicManager, metav1.ManagedFieldsOperationUpdate, contract.Path{"f:metadata", "f:labels"}))
	g.Expect(updatedBootstrapConfig.GetManagedFields()).ShouldNot(
		ssa.MatchFieldOwnership(classicManager, metav1.ManagedFieldsOperationUpdate, contract.Path{"f:metadata", "f:annotations"}))
	g.Expect(updatedBootstrapConfig.GetManagedFields()).Should(
		ssa.MatchFieldOwnership(classicManager, metav1.ManagedFieldsOperationUpdate, contract.Path{"f:spec"}))

	//
	// Verify In-place mutating fields
	//

	// Update the MachineSet and verify the in-mutating fields are propagated.
	ms.Spec.Template.Labels = map[string]string{
		"preserved-label": "preserved-value",  // Keep the label and value as is
		"modified-label":  "modified-value-2", // Modify the value of the label
		// Drop "dropped-label"
	}
	expectedLabels := map[string]string{
		"preserved-label":                    "preserved-value",
		"modified-label":                     "modified-value-2",
		clusterv1.MachineSetNameLabel:        ms.Name,
		clusterv1.MachineDeploymentNameLabel: "md-1",
		clusterv1.ClusterNameLabel:           testClusterName, // This label is added by the Machine controller.
	}
	ms.Spec.Template.Annotations = map[string]string{
		"preserved-annotation": "preserved-value",  // Keep the annotation and value as is
		"modified-annotation":  "modified-value-2", // Modify the value of the annotation
		// Drop "dropped-annotation"
	}
	readinessGates := []clusterv1.MachineReadinessGate{{ConditionType: "foo"}}
	ms.Spec.Template.Spec.ReadinessGates = readinessGates
	ms.Spec.Template.Spec.NodeDrainTimeout = duration10s
	ms.Spec.Template.Spec.NodeDeletionTimeout = duration10s
	ms.Spec.Template.Spec.NodeVolumeDetachTimeout = duration10s
	s = &scope{
		machineSet: ms,
		machines:   []*clusterv1.Machine{updatedInPlaceMutatingMachine, deletingMachine},
		getAndAdoptMachinesForMachineSetSucceeded: true,
	}
	_, err = reconciler.syncMachines(ctx, s)
	g.Expect(err).ToNot(HaveOccurred())

	// Verify in-place mutable fields are updated on the Machine.
	updatedInPlaceMutatingMachine = inPlaceMutatingMachine.DeepCopy()
	g.Eventually(func(g Gomega) {
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedInPlaceMutatingMachine), updatedInPlaceMutatingMachine)).To(Succeed())
		// Verify Labels
		g.Expect(updatedInPlaceMutatingMachine.Labels).Should(Equal(expectedLabels))
		// Verify Annotations
		g.Expect(updatedInPlaceMutatingMachine.Annotations).Should(Equal(ms.Spec.Template.Annotations))
		// Verify Node timeout values
		g.Expect(updatedInPlaceMutatingMachine.Spec.NodeDrainTimeout).Should(And(
			Not(BeNil()),
			HaveValue(Equal(*ms.Spec.Template.Spec.NodeDrainTimeout)),
		))
		g.Expect(updatedInPlaceMutatingMachine.Spec.NodeDeletionTimeout).Should(And(
			Not(BeNil()),
			HaveValue(Equal(*ms.Spec.Template.Spec.NodeDeletionTimeout)),
		))
		g.Expect(updatedInPlaceMutatingMachine.Spec.NodeVolumeDetachTimeout).Should(And(
			Not(BeNil()),
			HaveValue(Equal(*ms.Spec.Template.Spec.NodeVolumeDetachTimeout)),
		))
		// Verify readiness gates.
		g.Expect(updatedInPlaceMutatingMachine.Spec.ReadinessGates).Should(Equal(readinessGates))
	}, timeout).Should(Succeed())

	// Verify in-place mutable fields are updated on InfrastructureMachine
	updatedInfraMachine = infraMachine.DeepCopy()
	g.Eventually(func(g Gomega) {
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedInfraMachine), updatedInfraMachine)).To(Succeed())
		// Verify Labels
		g.Expect(updatedInfraMachine.GetLabels()).Should(Equal(expectedLabels))
		// Verify Annotations
		g.Expect(updatedInfraMachine.GetAnnotations()).Should(Equal(ms.Spec.Template.Annotations))
		// Verify spec remains the same
		g.Expect(updatedInfraMachine.Object).Should(HaveKeyWithValue("spec", infraMachineSpec))
	}, timeout).Should(Succeed())

	// Verify in-place mutable fields are updated on the BootstrapConfig.
	updatedBootstrapConfig = bootstrapConfig.DeepCopy()
	g.Eventually(func(g Gomega) {
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedBootstrapConfig), updatedBootstrapConfig)).To(Succeed())
		// Verify Labels
		g.Expect(updatedBootstrapConfig.GetLabels()).Should(Equal(expectedLabels))
		// Verify Annotations
		g.Expect(updatedBootstrapConfig.GetAnnotations()).Should(Equal(ms.Spec.Template.Annotations))
		// Verify spec remains the same
		g.Expect(updatedBootstrapConfig.Object).Should(HaveKeyWithValue("spec", bootstrapConfigSpec))
	}, timeout).Should(Succeed())

	// Wait to ensure Machine is not updated.
	// Verify that the machine stays the same consistently.
	g.Consistently(func(g Gomega) {
		// The deleting machine should not change.
		updatedDeletingMachine := deletingMachine.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedDeletingMachine), updatedDeletingMachine)).To(Succeed())

		// Verify ManagedFields
		g.Expect(updatedDeletingMachine.ManagedFields).ShouldNot(
			ContainElement(ssa.MatchManagedFieldsEntry(machineSetManagerName, metav1.ManagedFieldsOperationApply)),
			"deleting machine should not contain an entry for SSA manager",
		)
		g.Expect(updatedDeletingMachine.ManagedFields).Should(
			ContainElement(ssa.MatchManagedFieldsEntry("manager", metav1.ManagedFieldsOperationUpdate)),
			"in-place mutable machine should still contain an entry for old manager",
		)

		// Verify in-place mutable fields are still the same.
		g.Expect(updatedDeletingMachine.Labels).Should(Equal(deletingMachine.Labels))
		g.Expect(updatedDeletingMachine.Annotations).Should(Equal(deletingMachine.Annotations))
		g.Expect(updatedDeletingMachine.Spec.NodeDrainTimeout).Should(Equal(deletingMachine.Spec.NodeDrainTimeout))
		g.Expect(updatedDeletingMachine.Spec.NodeDeletionTimeout).Should(Equal(deletingMachine.Spec.NodeDeletionTimeout))
		g.Expect(updatedDeletingMachine.Spec.NodeVolumeDetachTimeout).Should(Equal(deletingMachine.Spec.NodeVolumeDetachTimeout))
	}, 5*time.Second).Should(Succeed())

	// Verify in-place mutable fields are updated on the deleting machine
	ms.Spec.Template.Spec.NodeDrainTimeout = duration11s
	ms.Spec.Template.Spec.NodeDeletionTimeout = duration11s
	ms.Spec.Template.Spec.NodeVolumeDetachTimeout = duration11s
	s = &scope{
		machineSet: ms,
		machines:   []*clusterv1.Machine{updatedInPlaceMutatingMachine, deletingMachine},
		getAndAdoptMachinesForMachineSetSucceeded: true,
	}
	_, err = reconciler.syncMachines(ctx, s)
	g.Expect(err).ToNot(HaveOccurred())
	updatedDeletingMachine := deletingMachine.DeepCopy()

	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedDeletingMachine), updatedDeletingMachine)).To(Succeed())
	// Verify Node timeout values
	g.Expect(updatedDeletingMachine.Spec.NodeDrainTimeout).Should(And(
		Not(BeNil()),
		HaveValue(Equal(*ms.Spec.Template.Spec.NodeDrainTimeout)),
	))
	g.Expect(updatedDeletingMachine.Spec.NodeDeletionTimeout).Should(And(
		Not(BeNil()),
		HaveValue(Equal(*ms.Spec.Template.Spec.NodeDeletionTimeout)),
	))
	g.Expect(updatedDeletingMachine.Spec.NodeVolumeDetachTimeout).Should(And(
		Not(BeNil()),
		HaveValue(Equal(*ms.Spec.Template.Spec.NodeVolumeDetachTimeout)),
	))
}

func TestMachineSetReconciler_reconcileUnhealthyMachines(t *testing.T) {
	t.Run("should delete unhealthy machines if preflight checks pass", func(t *testing.T) {
		g := NewWithT(t)

		controlPlaneStable := builder.ControlPlane("default", "cp1").
			WithVersion("v1.26.2").
			WithStatusFields(map[string]interface{}{
				"status.version": "v1.26.2",
			}).
			Build()
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: contract.ObjToRef(controlPlaneStable),
			},
		}
		machineSet := &clusterv1.MachineSet{}

		unhealthyMachine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "unhealthy-machine",
				Namespace: "default",
				// Blocking deletion so we can confirm conditions were updated as expected.
				Finalizers: []string{"block-deletion"},
			},
			Status: clusterv1.MachineStatus{
				Conditions: []clusterv1.Condition{
					{
						Type:   clusterv1.MachineOwnerRemediatedCondition,
						Status: corev1.ConditionFalse,
					},
					{
						Type:   clusterv1.MachineHealthCheckSucceededCondition,
						Status: corev1.ConditionFalse,
					},
				},
				V1Beta2: &clusterv1.MachineV1Beta2Status{
					Conditions: []metav1.Condition{
						{
							Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
							Status:  metav1.ConditionFalse,
							Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationV1Beta2Reason,
							Message: "Waiting for remediation",
						},
						{
							Type:    clusterv1.MachineHealthCheckSucceededV1Beta2Condition,
							Status:  metav1.ConditionFalse,
							Reason:  clusterv1.MachineHealthCheckHasRemediateAnnotationV1Beta2Reason,
							Message: "Marked for remediation via cluster.x-k8s.io/remediate-machine annotation",
						},
					},
				},
			},
		}
		healthyMachine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "healthy-machine",
				Namespace: "default",
			},
			Status: clusterv1.MachineStatus{
				Conditions: []clusterv1.Condition{
					{
						// This condition should be cleaned up because HealthCheckSucceeded is true.
						Type:   clusterv1.MachineOwnerRemediatedCondition,
						Status: corev1.ConditionFalse,
					},
					{
						Type:   clusterv1.MachineHealthCheckSucceededCondition,
						Status: corev1.ConditionTrue,
					},
				},
				V1Beta2: &clusterv1.MachineV1Beta2Status{
					Conditions: []metav1.Condition{
						{
							// This condition should be cleaned up because HealthCheckSucceeded is true.
							Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
							Status:  metav1.ConditionFalse,
							Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationV1Beta2Reason,
							Message: "Waiting for remediation",
						},
						{
							Type:   clusterv1.MachineHealthCheckSucceededV1Beta2Condition,
							Status: metav1.ConditionTrue,
							Reason: clusterv1.MachineHealthCheckSucceededV1Beta2Reason,
						},
					},
				},
			},
		}

		machines := []*clusterv1.Machine{unhealthyMachine, healthyMachine}

		fakeClient := fake.NewClientBuilder().WithObjects(controlPlaneStable, unhealthyMachine, healthyMachine).WithStatusSubresource(&clusterv1.Machine{}).Build()
		r := &Reconciler{
			Client: fakeClient,
		}

		s := &scope{
			cluster:    cluster,
			machineSet: machineSet,
			machines:   machines,
			getAndAdoptMachinesForMachineSetSucceeded: true,
		}

		_, err := r.reconcileUnhealthyMachines(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())

		// Verify the unhealthy machine is deleted (deletionTimestamp must be set).
		m := &clusterv1.Machine{}
		g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(unhealthyMachine), m)).To(Succeed())
		g.Expect(m.DeletionTimestamp.IsZero()).To(BeFalse())
		g.Expect(conditions.IsTrue(m, clusterv1.MachineOwnerRemediatedCondition)).To(BeTrue())
		c := v1beta2conditions.Get(m, clusterv1.MachineOwnerRemediatedV1Beta2Condition)
		g.Expect(c).ToNot(BeNil())
		g.Expect(*c).To(v1beta2conditions.MatchCondition(metav1.Condition{
			Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineSetMachineRemediationMachineDeletingV1Beta2Reason,
			Message: "Machine is deleting",
		}, v1beta2conditions.IgnoreLastTransitionTime(true)))

		// Verify the healthy machine is not deleted and does not have the OwnerRemediated condition.
		m = &clusterv1.Machine{}
		g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(healthyMachine), m)).Should(Succeed())
		g.Expect(m.DeletionTimestamp.IsZero()).To(BeTrue())
		g.Expect(conditions.Has(m, clusterv1.MachineOwnerRemediatedCondition)).To(BeFalse())
		g.Expect(v1beta2conditions.Has(m, clusterv1.MachineOwnerRemediatedV1Beta2Condition)).To(BeFalse())
	})

	t.Run("should update the unhealthy machine MachineOwnerRemediated condition if preflight checks did not pass", func(t *testing.T) {
		g := NewWithT(t)

		// An upgrading control plane should cause the preflight checks to not pass.
		controlPlaneUpgrading := builder.ControlPlane("default", "cp1").
			WithVersion("v1.26.2").
			WithStatusFields(map[string]interface{}{
				"status.version": "v1.25.2",
			}).
			Build()
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: contract.ObjToRef(controlPlaneUpgrading),
			},
		}
		machineSet := &clusterv1.MachineSet{}

		unhealthyMachine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "unhealthy-machine",
				Namespace: "default",
			},
			Status: clusterv1.MachineStatus{
				Conditions: []clusterv1.Condition{
					{
						Type:   clusterv1.MachineOwnerRemediatedCondition,
						Status: corev1.ConditionFalse,
					},
					{
						Type:   clusterv1.MachineHealthCheckSucceededCondition,
						Status: corev1.ConditionFalse,
					},
				},
				V1Beta2: &clusterv1.MachineV1Beta2Status{
					Conditions: []metav1.Condition{
						{
							Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
							Status:  metav1.ConditionFalse,
							Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationV1Beta2Reason,
							Message: "Waiting for remediation",
						},
						{
							Type:    clusterv1.MachineHealthCheckSucceededV1Beta2Condition,
							Status:  metav1.ConditionFalse,
							Reason:  clusterv1.MachineHealthCheckHasRemediateAnnotationV1Beta2Reason,
							Message: "Marked for remediation via cluster.x-k8s.io/remediate-machine annotation",
						},
					},
				},
			},
		}
		healthyMachine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "healthy-machine",
				Namespace: "default",
			},
			Status: clusterv1.MachineStatus{
				Conditions: []clusterv1.Condition{
					{
						// This condition should be cleaned up because HealthCheckSucceeded is true.
						Type:   clusterv1.MachineOwnerRemediatedCondition,
						Status: corev1.ConditionFalse,
					},
					{
						Type:   clusterv1.MachineHealthCheckSucceededCondition,
						Status: corev1.ConditionTrue,
					},
				},
				V1Beta2: &clusterv1.MachineV1Beta2Status{
					Conditions: []metav1.Condition{
						{
							// This condition should be cleaned up because HealthCheckSucceeded is true.
							Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
							Status:  metav1.ConditionFalse,
							Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationV1Beta2Reason,
							Message: "Waiting for remediation",
						},
						{
							Type:   clusterv1.MachineHealthCheckSucceededV1Beta2Condition,
							Status: metav1.ConditionTrue,
							Reason: clusterv1.MachineHealthCheckSucceededV1Beta2Reason,
						},
					},
				},
			},
		}

		machines := []*clusterv1.Machine{unhealthyMachine, healthyMachine}
		fakeClient := fake.NewClientBuilder().WithObjects(controlPlaneUpgrading, unhealthyMachine, healthyMachine).WithStatusSubresource(&clusterv1.Machine{}).Build()
		r := &Reconciler{
			Client: fakeClient,
		}
		s := &scope{
			cluster:    cluster,
			machineSet: machineSet,
			machines:   machines,
			getAndAdoptMachinesForMachineSetSucceeded: true,
		}
		_, err := r.reconcileUnhealthyMachines(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())

		// Verify the unhealthy machine has the updated condition.
		condition := clusterv1.MachineOwnerRemediatedCondition
		m := &clusterv1.Machine{}
		g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(unhealthyMachine), m)).To(Succeed())
		g.Expect(m.DeletionTimestamp.IsZero()).To(BeTrue())
		g.Expect(conditions.Has(m, condition)).
			To(BeTrue(), "Machine should have the %s condition set", condition)
		machineOwnerRemediatedCondition := conditions.Get(m, condition)
		g.Expect(machineOwnerRemediatedCondition.Status).
			To(Equal(corev1.ConditionFalse), "%s condition status should be false", condition)
		g.Expect(machineOwnerRemediatedCondition.Reason).
			To(Equal(clusterv1.WaitingForRemediationReason), "%s condition should have reason %s", condition, clusterv1.WaitingForRemediationReason)
		c := v1beta2conditions.Get(m, clusterv1.MachineOwnerRemediatedV1Beta2Condition)
		g.Expect(c).ToNot(BeNil())
		g.Expect(*c).To(v1beta2conditions.MatchCondition(metav1.Condition{
			Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineSetMachineRemediationDeferredV1Beta2Reason,
			Message: "* GenericControlPlane default/cp1 is upgrading (\"ControlPlaneIsStable\" preflight check failed)",
		}, v1beta2conditions.IgnoreLastTransitionTime(true)))

		// Verify the healthy machine is not deleted and does not have the OwnerRemediated condition.
		m = &clusterv1.Machine{}
		g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(healthyMachine), m)).To(Succeed())
		g.Expect(m.DeletionTimestamp.IsZero()).To(BeTrue())
		g.Expect(conditions.Has(m, condition)).
			To(BeFalse(), "Machine should not have the %s condition set", condition)
		g.Expect(v1beta2conditions.Has(m, clusterv1.MachineOwnerRemediatedV1Beta2Condition)).To(BeFalse())
	})

	t.Run("should only try to remediate MachineOwnerRemediated if MachineSet is current", func(t *testing.T) {
		g := NewWithT(t)

		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: clusterv1.ClusterSpec{},
		}

		machineDeployment := &clusterv1.MachineDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-machinedeployment",
				Namespace: "default",
				Annotations: map[string]string{
					clusterv1.RevisionAnnotation: "10",
				},
			},
			Spec: clusterv1.MachineDeploymentSpec{
				ClusterName: "test-cluster",
			},
		}

		machineSetOld := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-machinedeployment-old",
				Namespace: "default",
				Labels: map[string]string{
					clusterv1.MachineDeploymentNameLabel: "test-machinedeployment",
				},
				Annotations: map[string]string{
					clusterv1.RevisionAnnotation: "7",
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "MachineDeployment",
						Name:       "test-machinedeployment",
					},
				},
			},
			Spec: clusterv1.MachineSetSpec{
				ClusterName: "test-cluster",
			},
		}

		machineSetCurrent := machineSetOld.DeepCopy()
		machineSetCurrent.Name = "test-machinedeployment-current"
		machineSetCurrent.Annotations[clusterv1.RevisionAnnotation] = "10"

		unhealthyMachine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "unhealthy-machine",
				Namespace: "default",
				// Blocking deletion so we can confirm conditions were updated as expected.
				Finalizers: []string{"block-deletion"},
			},
			Status: clusterv1.MachineStatus{
				Conditions: []clusterv1.Condition{
					{
						Type:   clusterv1.MachineOwnerRemediatedCondition,
						Status: corev1.ConditionFalse,
					},
					{
						Type:   clusterv1.MachineHealthCheckSucceededCondition,
						Status: corev1.ConditionFalse,
					},
				},
				V1Beta2: &clusterv1.MachineV1Beta2Status{
					Conditions: []metav1.Condition{
						{
							Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
							Status:  metav1.ConditionFalse,
							Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationV1Beta2Reason,
							Message: "Waiting for remediation",
						},
						{
							Type:    clusterv1.MachineHealthCheckSucceededV1Beta2Condition,
							Status:  metav1.ConditionFalse,
							Reason:  clusterv1.MachineHealthCheckHasRemediateAnnotationV1Beta2Reason,
							Message: "Marked for remediation via cluster.x-k8s.io/remediate-machine annotation",
						},
					},
				},
			},
		}
		healthyMachine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "healthy-machine",
				Namespace: "default",
			},
			Status: clusterv1.MachineStatus{
				Conditions: []clusterv1.Condition{
					{
						// This condition should be cleaned up because HealthCheckSucceeded is true.
						Type:   clusterv1.MachineOwnerRemediatedCondition,
						Status: corev1.ConditionFalse,
					},
					{
						Type:   clusterv1.MachineHealthCheckSucceededCondition,
						Status: corev1.ConditionTrue,
					},
				},
				V1Beta2: &clusterv1.MachineV1Beta2Status{
					Conditions: []metav1.Condition{
						{
							// This condition should be cleaned up because HealthCheckSucceeded is true.
							Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
							Status:  metav1.ConditionFalse,
							Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationV1Beta2Reason,
							Message: "Waiting for remediation",
						},
						{
							Type:   clusterv1.MachineHealthCheckSucceededV1Beta2Condition,
							Status: metav1.ConditionTrue,
							Reason: clusterv1.MachineHealthCheckSucceededV1Beta2Reason,
						},
					},
				},
			},
		}

		machines := []*clusterv1.Machine{unhealthyMachine, healthyMachine}
		fakeClient := fake.NewClientBuilder().WithObjects(
			machineDeployment,
			machineSetOld,
			machineSetCurrent,
			unhealthyMachine,
			healthyMachine,
		).WithStatusSubresource(&clusterv1.Machine{}, &clusterv1.MachineSet{}, &clusterv1.MachineDeployment{}).Build()
		r := &Reconciler{
			Client: fakeClient,
		}

		s := &scope{
			cluster:                 cluster,
			machineSet:              machineSetOld,
			machines:                machines,
			owningMachineDeployment: machineDeployment,
			getAndAdoptMachinesForMachineSetSucceeded: true,
		}

		// Test first with the old MachineSet.
		_, err := r.reconcileUnhealthyMachines(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())

		condition := clusterv1.MachineOwnerRemediatedCondition
		m := &clusterv1.Machine{}

		// Verify that no action was taken on the Machine: MachineOwnerRemediated should be false
		// and the Machine wasn't deleted.
		g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(unhealthyMachine), m)).To(Succeed())
		g.Expect(conditions.Has(m, condition)).
			To(BeTrue(), "Machine should have the %s condition set", condition)
		machineOwnerRemediatedCondition := conditions.Get(m, condition)
		g.Expect(machineOwnerRemediatedCondition.Status).
			To(Equal(corev1.ConditionFalse), "%s condition status should be false", condition)
		g.Expect(unhealthyMachine.DeletionTimestamp).Should(BeZero())
		c := v1beta2conditions.Get(m, clusterv1.MachineOwnerRemediatedV1Beta2Condition)
		g.Expect(c).ToNot(BeNil())
		g.Expect(*c).To(v1beta2conditions.MatchCondition(metav1.Condition{
			Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineSetMachineCannotBeRemediatedV1Beta2Reason,
			Message: "Machine won't be remediated because it is pending removal due to rollout",
		}, v1beta2conditions.IgnoreLastTransitionTime(true)))

		// Verify the healthy machine is not deleted and does not have the OwnerRemediated condition.
		m = &clusterv1.Machine{}
		g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(healthyMachine), m)).To(Succeed())
		g.Expect(m.DeletionTimestamp.IsZero()).To(BeTrue())
		g.Expect(conditions.Has(m, condition)).
			To(BeFalse(), "Machine should not have the %s condition set", condition)
		g.Expect(v1beta2conditions.Has(m, clusterv1.MachineOwnerRemediatedV1Beta2Condition)).To(BeFalse())

		// Test with the current MachineSet.
		s = &scope{
			cluster:                 cluster,
			machineSet:              machineSetCurrent,
			machines:                machines,
			owningMachineDeployment: machineDeployment,
			getAndAdoptMachinesForMachineSetSucceeded: true,
		}
		_, err = r.reconcileUnhealthyMachines(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())

		// Verify the unhealthy machine has been deleted.
		g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(unhealthyMachine), m)).To(Succeed())
		g.Expect(m.DeletionTimestamp.IsZero()).To(BeFalse())
		g.Expect(conditions.IsTrue(m, clusterv1.MachineOwnerRemediatedCondition)).To(BeTrue())
		c = v1beta2conditions.Get(m, clusterv1.MachineOwnerRemediatedV1Beta2Condition)
		g.Expect(c).ToNot(BeNil())
		g.Expect(*c).To(v1beta2conditions.MatchCondition(metav1.Condition{
			Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineSetMachineRemediationMachineDeletingV1Beta2Reason,
			Message: "Machine is deleting",
		}, v1beta2conditions.IgnoreLastTransitionTime(true)))

		// Verify (again) the healthy machine is not deleted and does not have the OwnerRemediated condition.
		m = &clusterv1.Machine{}
		g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(healthyMachine), m)).To(Succeed())
		g.Expect(m.DeletionTimestamp.IsZero()).To(BeTrue())
		g.Expect(conditions.Has(m, condition)).
			To(BeFalse(), "Machine should not have the %s condition set", condition)
		g.Expect(v1beta2conditions.Has(m, clusterv1.MachineOwnerRemediatedV1Beta2Condition)).To(BeFalse())
	})

	t.Run("should only try to remediate up to MaxInFlight unhealthy", func(t *testing.T) {
		g := NewWithT(t)

		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: clusterv1.ClusterSpec{},
		}

		maxInFlight := 3
		machineDeployment := &clusterv1.MachineDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-machinedeployment",
				Namespace: "default",
				Annotations: map[string]string{
					clusterv1.RevisionAnnotation: "10",
				},
			},
			Spec: clusterv1.MachineDeploymentSpec{
				ClusterName: "test-cluster",
				Strategy: &clusterv1.MachineDeploymentStrategy{
					Remediation: &clusterv1.RemediationStrategy{
						MaxInFlight: ptr.To(intstr.FromInt32(int32(maxInFlight))),
					},
				},
			},
		}

		machineSet := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-machinedeployment-old",
				Namespace: "default",
				Labels: map[string]string{
					clusterv1.MachineDeploymentNameLabel: "test-machinedeployment",
				},
				Annotations: map[string]string{
					clusterv1.RevisionAnnotation: "10",
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "MachineDeployment",
						Name:       "test-machinedeployment",
					},
				},
			},
			Spec: clusterv1.MachineSetSpec{
				ClusterName: "test-cluster",
			},
		}

		unhealthyMachines := []*clusterv1.Machine{}
		total := 8
		for i := range total {
			unhealthyMachines = append(unhealthyMachines, &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              fmt.Sprintf("unhealthy-machine-%d", i),
					Namespace:         "default",
					CreationTimestamp: metav1.Time{Time: metav1.Now().Add(time.Duration(i) * time.Second)},
				},
				Status: clusterv1.MachineStatus{
					Conditions: []clusterv1.Condition{
						{
							Type:   clusterv1.MachineOwnerRemediatedCondition,
							Status: corev1.ConditionFalse,
						},
						{
							Type:   clusterv1.MachineHealthCheckSucceededCondition,
							Status: corev1.ConditionFalse,
						},
					},
					V1Beta2: &clusterv1.MachineV1Beta2Status{
						Conditions: []metav1.Condition{
							{
								Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
								Status:  metav1.ConditionFalse,
								Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationV1Beta2Reason,
								Message: "Waiting for remediation",
							},
							{
								Type:    clusterv1.MachineHealthCheckSucceededV1Beta2Condition,
								Status:  metav1.ConditionFalse,
								Reason:  clusterv1.MachineHealthCheckHasRemediateAnnotationV1Beta2Reason,
								Message: "Marked for remediation via cluster.x-k8s.io/remediate-machine annotation",
							},
						},
					},
				},
			})
		}

		healthyMachine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "healthy-machine",
				Namespace: "default",
			},
			Status: clusterv1.MachineStatus{
				Conditions: []clusterv1.Condition{
					{
						// This condition should be cleaned up because HealthCheckSucceeded is true.
						Type:   clusterv1.MachineOwnerRemediatedCondition,
						Status: corev1.ConditionFalse,
					},
					{
						Type:   clusterv1.MachineHealthCheckSucceededCondition,
						Status: corev1.ConditionTrue,
					},
				},
				V1Beta2: &clusterv1.MachineV1Beta2Status{
					Conditions: []metav1.Condition{
						{
							// This condition should be cleaned up because HealthCheckSucceeded is true.
							Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
							Status:  metav1.ConditionFalse,
							Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationV1Beta2Reason,
							Message: "Waiting for remediation",
						},
						{
							Type:   clusterv1.MachineHealthCheckSucceededV1Beta2Condition,
							Status: metav1.ConditionTrue,
							Reason: clusterv1.MachineHealthCheckSucceededV1Beta2Reason,
						},
					},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithObjects(cluster, machineDeployment, healthyMachine).
			WithStatusSubresource(&clusterv1.Machine{}, &clusterv1.MachineSet{}, &clusterv1.MachineDeployment{})
		// Create the unhealthy machines.
		for _, machine := range unhealthyMachines {
			fakeClient.WithObjects(machine)
		}
		r := &Reconciler{
			Client: fakeClient.Build(),
		}

		//
		// First pass.
		//
		s := &scope{
			cluster:                 cluster,
			machineSet:              machineSet,
			machines:                append(unhealthyMachines, healthyMachine),
			owningMachineDeployment: machineDeployment,
			getAndAdoptMachinesForMachineSetSucceeded: true,
		}
		_, err := r.reconcileUnhealthyMachines(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())

		condition := clusterv1.MachineOwnerRemediatedCondition

		// Iterate over the unhealthy machines and verify that the last maxInFlight were deleted.
		for i := range unhealthyMachines {
			m := unhealthyMachines[i]

			err = r.Client.Get(ctx, client.ObjectKeyFromObject(m), m)
			if i < total-maxInFlight {
				// Machines before the maxInFlight should not be deleted.
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(conditions.Has(m, condition)).
					To(BeTrue(), "Machine should have the %s condition set", condition)
				machineOwnerRemediatedCondition := conditions.Get(m, condition)
				g.Expect(machineOwnerRemediatedCondition.Status).
					To(Equal(corev1.ConditionFalse), "%s condition status should be false", condition)
				c := v1beta2conditions.Get(m, clusterv1.MachineOwnerRemediatedV1Beta2Condition)
				g.Expect(c).ToNot(BeNil())
				g.Expect(*c).To(v1beta2conditions.MatchCondition(metav1.Condition{
					Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineSetMachineRemediationDeferredV1Beta2Reason,
					Message: "Waiting because there are already too many remediations in progress (spec.strategy.remediation.maxInFlight is 3)",
				}, v1beta2conditions.IgnoreLastTransitionTime(true)))
			} else {
				// Machines after maxInFlight, should be deleted.
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected machine %d to be deleted", i)
			}
		}

		// Verify the healthy machine is not deleted and does not have the OwnerRemediated condition.
		m := &clusterv1.Machine{}
		g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(healthyMachine), m)).To(Succeed())
		g.Expect(m.DeletionTimestamp.IsZero()).To(BeTrue())
		g.Expect(conditions.Has(m, condition)).
			To(BeFalse(), "Machine should not have the %s condition set", condition)
		g.Expect(v1beta2conditions.Has(m, clusterv1.MachineOwnerRemediatedV1Beta2Condition)).To(BeFalse())

		//
		// Second pass.
		//
		// Set a finalizer on the next set of machines that should be remediated.
		for i := maxInFlight - 1; i < total-maxInFlight; i++ {
			m := unhealthyMachines[i]
			m.Finalizers = append(m.Finalizers, "test")
			g.Expect(r.Client.Update(ctx, m)).To(Succeed())
		}

		// Perform the second pass.
		allMachines := func() (res []*clusterv1.Machine) {
			var machineList clusterv1.MachineList
			g.Expect(r.Client.List(ctx, &machineList)).To(Succeed())
			for i := range machineList.Items {
				m := &machineList.Items[i]
				res = append(res, m)
			}
			return
		}

		s = &scope{
			cluster:                 cluster,
			machineSet:              machineSet,
			machines:                allMachines(),
			owningMachineDeployment: machineDeployment,
			getAndAdoptMachinesForMachineSetSucceeded: true,
		}
		_, err = r.reconcileUnhealthyMachines(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())

		validateSecondPass := func(cleanFinalizer bool) {
			t.Helper()
			for i := range unhealthyMachines {
				m := unhealthyMachines[i]

				err = r.Client.Get(ctx, client.ObjectKeyFromObject(m), m)
				if i < total-(maxInFlight*2) {
					// Machines before the maxInFlight*2 should not be deleted, and should have the remediated condition to false.
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(conditions.Has(m, condition)).
						To(BeTrue(), "Machine should have the %s condition set", condition)
					machineOwnerRemediatedCondition := conditions.Get(m, condition)
					g.Expect(machineOwnerRemediatedCondition.Status).
						To(Equal(corev1.ConditionFalse), "%s condition status should be false", condition)
					c := v1beta2conditions.Get(m, clusterv1.MachineOwnerRemediatedV1Beta2Condition)
					g.Expect(c).ToNot(BeNil())
					g.Expect(*c).To(v1beta2conditions.MatchCondition(metav1.Condition{
						Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineSetMachineRemediationDeferredV1Beta2Reason,
						Message: "Waiting because there are already too many remediations in progress (spec.strategy.remediation.maxInFlight is 3)",
					}, v1beta2conditions.IgnoreLastTransitionTime(true)))
					g.Expect(m.DeletionTimestamp).To(BeZero())
				} else if i < total-maxInFlight {
					// Machines before the maxInFlight should have a deletion timestamp
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(conditions.Has(m, condition)).
						To(BeTrue(), "Machine should have the %s condition set", condition)
					machineOwnerRemediatedCondition := conditions.Get(m, condition)
					g.Expect(machineOwnerRemediatedCondition.Status).
						To(Equal(corev1.ConditionTrue), "%s condition status should be true", condition)
					c := v1beta2conditions.Get(m, clusterv1.MachineOwnerRemediatedV1Beta2Condition)
					g.Expect(c).ToNot(BeNil())
					g.Expect(*c).To(v1beta2conditions.MatchCondition(metav1.Condition{
						Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineSetMachineRemediationMachineDeletingV1Beta2Reason,
						Message: "Machine is deleting",
					}, v1beta2conditions.IgnoreLastTransitionTime(true)))
					g.Expect(m.DeletionTimestamp).ToNot(BeZero())

					if cleanFinalizer {
						g.Expect(controllerutil.RemoveFinalizer(m, "test")).To(BeTrue())
						g.Expect(r.Client.Update(ctx, m)).To(Succeed())
					}
				} else {
					// Machines after maxInFlight, should be deleted.
					g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected machine %d to be deleted", i)
				}
			}
		}
		validateSecondPass(false)

		// Verify (again) the healthy machine is not deleted and does not have the OwnerRemediated condition.
		g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(healthyMachine), m)).To(Succeed())
		g.Expect(m.DeletionTimestamp.IsZero()).To(BeTrue())
		g.Expect(conditions.Has(m, condition)).
			To(BeFalse(), "Machine should not have the %s condition set", condition)
		g.Expect(v1beta2conditions.Has(m, clusterv1.MachineOwnerRemediatedV1Beta2Condition)).To(BeFalse())

		// Perform another pass with the same exact configuration.
		// This is testing that, given that we have Machines that are being deleted and are in flight,
		// we have reached the maximum amount of tokens we have and we should wait to remediate the rest.
		s = &scope{
			cluster:                 cluster,
			machineSet:              machineSet,
			machines:                allMachines(),
			owningMachineDeployment: machineDeployment,
			getAndAdoptMachinesForMachineSetSucceeded: true,
		}
		_, err = r.reconcileUnhealthyMachines(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())

		// Validate and remove finalizers for in flight machines.
		validateSecondPass(true)

		// Verify (again) the healthy machine is not deleted and does not have the OwnerRemediated condition.
		g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(healthyMachine), m)).To(Succeed())
		g.Expect(m.DeletionTimestamp.IsZero()).To(BeTrue())
		g.Expect(conditions.Has(m, condition)).
			To(BeFalse(), "Machine should not have the %s condition set", condition)
		g.Expect(v1beta2conditions.Has(m, clusterv1.MachineOwnerRemediatedV1Beta2Condition)).To(BeFalse())

		// Call again to verify that the remaining unhealthy machines are deleted,
		// at this point all unhealthy machines should be deleted given the max in flight
		// is greater than the number of unhealthy machines.
		s = &scope{
			cluster:                 cluster,
			machineSet:              machineSet,
			machines:                allMachines(),
			owningMachineDeployment: machineDeployment,
			getAndAdoptMachinesForMachineSetSucceeded: true,
		}
		_, err = r.reconcileUnhealthyMachines(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())

		// Iterate over the unhealthy machines and verify that all were deleted.
		for i, m := range unhealthyMachines {
			err = r.Client.Get(ctx, client.ObjectKeyFromObject(m), m)
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected machine %d to be deleted: %v", i)
		}

		// Verify (again) the healthy machine is not deleted and does not have the OwnerRemediated condition.
		g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(healthyMachine), m)).To(Succeed())
		g.Expect(m.DeletionTimestamp.IsZero()).To(BeTrue())
		g.Expect(conditions.Has(m, condition)).
			To(BeFalse(), "Machine should not have the %s condition set", condition)
		g.Expect(v1beta2conditions.Has(m, clusterv1.MachineOwnerRemediatedV1Beta2Condition)).To(BeFalse())
	})
}

func TestMachineSetReconciler_syncReplicas(t *testing.T) {
	t.Run("should hold off on creating new machines when preflight checks do not pass", func(t *testing.T) {
		g := NewWithT(t)

		// An upgrading control plane should cause the preflight checks to not pass.
		controlPlaneUpgrading := builder.ControlPlane("default", "test-cp").
			WithVersion("v1.26.2").
			WithStatusFields(map[string]interface{}{
				"status.version": "v1.25.2",
			}).
			Build()
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: contract.ObjToRef(controlPlaneUpgrading),
			},
		}
		machineSet := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-machineset",
				Namespace: "default",
			},
			Spec: clusterv1.MachineSetSpec{
				Replicas: ptr.To[int32](1),
			},
		}

		fakeClient := fake.NewClientBuilder().WithObjects(controlPlaneUpgrading, machineSet).WithStatusSubresource(&clusterv1.MachineSet{}).Build()
		r := &Reconciler{
			Client: fakeClient,
		}
		s := &scope{
			cluster:    cluster,
			machineSet: machineSet,
			machines:   []*clusterv1.Machine{},
			getAndAdoptMachinesForMachineSetSucceeded: true,
		}
		result, err := r.syncReplicas(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.IsZero()).To(BeFalse(), "syncReplicas should not return a 'zero' result")

		// Verify the proper condition is set on the MachineSet.
		condition := clusterv1.MachinesCreatedCondition
		g.Expect(conditions.Has(machineSet, condition)).
			To(BeTrue(), "MachineSet should have the %s condition set", condition)
		machinesCreatedCondition := conditions.Get(machineSet, condition)
		g.Expect(machinesCreatedCondition.Status).
			To(Equal(corev1.ConditionFalse), "%s condition status should be %s", condition, corev1.ConditionFalse)
		g.Expect(machinesCreatedCondition.Reason).
			To(Equal(clusterv1.PreflightCheckFailedReason), "%s condition reason should be %s", condition, clusterv1.PreflightCheckFailedReason)

		// Verify no new Machines are created.
		machineList := &clusterv1.MachineList{}
		g.Expect(r.Client.List(ctx, machineList)).To(Succeed())
		g.Expect(machineList.Items).To(BeEmpty(), "There should not be any machines")
	})
}

func TestMachineSetReconciler_syncReplicas_WithErrors(t *testing.T) {
	t.Run("should hold off on sync replicas when create Infrastructure of machine failed ", func(t *testing.T) {
		g := NewWithT(t)
		scheme := runtime.NewScheme()
		g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects().WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				// simulate scenarios where infra object creation fails
				if obj.GetObjectKind().GroupVersionKind().Kind == "GenericInfrastructureMachine" {
					return fmt.Errorf("inject error for create machineInfrastructure")
				}
				return client.Create(ctx, obj, opts...)
			},
		}).Build()

		r := &Reconciler{
			Client: fakeClient,
		}
		testCluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "default",
			},
		}

		duration10m := &metav1.Duration{Duration: 10 * time.Minute}
		machineSet := &clusterv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "machineset1",
				Namespace:  metav1.NamespaceDefault,
				Finalizers: []string{"block-deletion"},
			},
			Spec: clusterv1.MachineSetSpec{
				Replicas:    ptr.To[int32](1),
				ClusterName: "test-cluster",
				Template: clusterv1.MachineTemplateSpec{
					Spec: clusterv1.MachineSpec{
						ClusterName: testCluster.Name,
						Version:     ptr.To("v1.14.2"),
						Bootstrap: clusterv1.Bootstrap{
							ConfigRef: &corev1.ObjectReference{
								APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
								Kind:       "GenericBootstrapConfigTemplate",
								Name:       "ms-template",
								Namespace:  metav1.NamespaceDefault,
							},
						},
						InfrastructureRef: corev1.ObjectReference{
							APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
							Kind:       "GenericInfrastructureMachineTemplate",
							Name:       "ms-template",
							Namespace:  metav1.NamespaceDefault,
						},
						NodeDrainTimeout:        duration10m,
						NodeDeletionTimeout:     duration10m,
						NodeVolumeDetachTimeout: duration10m,
					},
				},
			},
		}

		// Create bootstrap template resource.
		bootstrapResource := map[string]interface{}{
			"kind":       "GenericBootstrapConfig",
			"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"precedence": "GenericBootstrapConfig",
				},
			},
		}

		bootstrapTmpl := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": bootstrapResource,
				},
			},
		}
		bootstrapTmpl.SetKind("GenericBootstrapConfigTemplate")
		bootstrapTmpl.SetAPIVersion("bootstrap.cluster.x-k8s.io/v1beta1")
		bootstrapTmpl.SetName("ms-template")
		bootstrapTmpl.SetNamespace(metav1.NamespaceDefault)
		g.Expect(r.Client.Create(context.TODO(), bootstrapTmpl)).To(Succeed())

		// Create infrastructure template resource.
		infraResource := map[string]interface{}{
			"kind":       "GenericInfrastructureMachine",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"precedence": "GenericInfrastructureMachineTemplate",
				},
			},
			"spec": map[string]interface{}{
				"size": "3xlarge",
			},
		}
		infraTmpl := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": infraResource,
				},
			},
		}
		infraTmpl.SetKind("GenericInfrastructureMachineTemplate")
		infraTmpl.SetAPIVersion("infrastructure.cluster.x-k8s.io/v1beta1")
		infraTmpl.SetName("ms-template")
		infraTmpl.SetNamespace(metav1.NamespaceDefault)
		g.Expect(r.Client.Create(context.TODO(), infraTmpl)).To(Succeed())

		s := &scope{
			cluster:    testCluster,
			machineSet: machineSet,
			machines:   []*clusterv1.Machine{},
			getAndAdoptMachinesForMachineSetSucceeded: true,
		}
		_, err := r.syncReplicas(ctx, s)
		g.Expect(err).To(HaveOccurred())

		// Verify the proper condition is set on the MachineSet.
		condition := clusterv1.MachinesCreatedCondition
		g.Expect(conditions.Has(machineSet, condition)).To(BeTrue(), "MachineSet should have the %s condition set", condition)

		machinesCreatedCondition := conditions.Get(machineSet, condition)
		g.Expect(machinesCreatedCondition.Status).
			To(Equal(corev1.ConditionFalse), "%s condition status should be %s", condition, corev1.ConditionFalse)
		g.Expect(machinesCreatedCondition.Reason).
			To(Equal(clusterv1.InfrastructureTemplateCloningFailedReason), "%s condition reason should be %s", condition, clusterv1.InfrastructureTemplateCloningFailedReason)

		// Verify no new Machines are created.
		machineList := &clusterv1.MachineList{}
		g.Expect(r.Client.List(ctx, machineList)).To(Succeed())
		g.Expect(machineList.Items).To(BeEmpty(), "There should not be any machines")

		// Verify no boostrap object created
		bootstrapList := &unstructured.UnstructuredList{}
		bootstrapList.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   bootstrapTmpl.GetObjectKind().GroupVersionKind().Group,
			Version: bootstrapTmpl.GetObjectKind().GroupVersionKind().Version,
			Kind:    strings.TrimSuffix(bootstrapTmpl.GetObjectKind().GroupVersionKind().Kind, clusterv1.TemplateSuffix),
		})
		g.Expect(r.Client.List(ctx, bootstrapList)).To(Succeed())
		g.Expect(bootstrapList.Items).To(BeEmpty(), "There should not be any bootstrap object")

		// Verify no infra object created
		infraList := &unstructured.UnstructuredList{}
		infraList.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   infraTmpl.GetObjectKind().GroupVersionKind().Group,
			Version: infraTmpl.GetObjectKind().GroupVersionKind().Version,
			Kind:    strings.TrimSuffix(infraTmpl.GetObjectKind().GroupVersionKind().Kind, clusterv1.TemplateSuffix),
		})
		g.Expect(r.Client.List(ctx, infraList)).To(Succeed())
		g.Expect(infraList.Items).To(BeEmpty(), "There should not be any infra object")
	})
}

type computeDesiredMachineTestCase struct {
	name            string
	ms              *clusterv1.MachineSet
	existingMachine *clusterv1.Machine
	wantMachine     *clusterv1.Machine
	wantName        []gomegatypes.GomegaMatcher
}

func TestComputeDesiredMachine(t *testing.T) {
	duration5s := &metav1.Duration{Duration: 5 * time.Second}
	duration10s := &metav1.Duration{Duration: 10 * time.Second}

	namingTemplateKey := "-md"
	mdName := "testmd"
	msName := "ms1"
	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
	}

	infraRef := corev1.ObjectReference{
		Kind:       "GenericInfrastructureMachineTemplate",
		Name:       "infra-template-1",
		APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
	}
	bootstrapRef := corev1.ObjectReference{
		Kind:       "GenericBootstrapConfigTemplate",
		Name:       "bootstrap-template-1",
		APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
	}

	machineTemplateSpec := clusterv1.MachineTemplateSpec{
		ObjectMeta: clusterv1.ObjectMeta{
			Labels:      map[string]string{"machine-label1": "machine-value1"},
			Annotations: map[string]string{"machine-annotation1": "machine-value1"},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName:       testClusterName,
			Version:           ptr.To("v1.25.3"),
			InfrastructureRef: infraRef,
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &bootstrapRef,
			},
			NodeDrainTimeout:        duration10s,
			NodeVolumeDetachTimeout: duration10s,
			NodeDeletionTimeout:     duration10s,
		},
	}

	skeletonMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				"machine-label1":                     "machine-value1",
				clusterv1.MachineSetNameLabel:        msName,
				clusterv1.MachineDeploymentNameLabel: mdName,
			},
			Annotations: map[string]string{"machine-annotation1": "machine-value1"},
			Finalizers:  []string{clusterv1.MachineFinalizer},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName:             testClusterName,
			Version:                 ptr.To("v1.25.3"),
			NodeDrainTimeout:        duration10s,
			NodeVolumeDetachTimeout: duration10s,
			NodeDeletionTimeout:     duration10s,
		},
	}

	// Creating a new Machine
	expectedNewMachine := skeletonMachine.DeepCopy()

	// Updating an existing Machine
	existingMachine := skeletonMachine.DeepCopy()
	existingMachine.Name = "exiting-machine-1"
	existingMachine.UID = "abc-123-existing-machine-1"
	existingMachine.Labels = nil
	existingMachine.Annotations = nil
	// Pre-existing finalizer should be preserved.
	existingMachine.Finalizers = []string{"pre-existing-finalizer"}
	existingMachine.Spec.InfrastructureRef = corev1.ObjectReference{
		Kind:       "GenericInfrastructureMachine",
		Name:       "infra-machine-1",
		APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
	}
	existingMachine.Spec.Bootstrap.ConfigRef = &corev1.ObjectReference{
		Kind:       "GenericBootstrapConfig",
		Name:       "bootstrap-config-1",
		APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
	}
	existingMachine.Spec.NodeDrainTimeout = duration5s
	existingMachine.Spec.NodeDeletionTimeout = duration5s
	existingMachine.Spec.NodeVolumeDetachTimeout = duration5s

	expectedUpdatedMachine := skeletonMachine.DeepCopy()
	expectedUpdatedMachine.Name = existingMachine.Name
	expectedUpdatedMachine.UID = existingMachine.UID
	// Pre-existing finalizer should be preserved.
	expectedUpdatedMachine.Finalizers = []string{"pre-existing-finalizer", clusterv1.MachineFinalizer}
	expectedUpdatedMachine.Spec.InfrastructureRef = *existingMachine.Spec.InfrastructureRef.DeepCopy()
	expectedUpdatedMachine.Spec.Bootstrap.ConfigRef = existingMachine.Spec.Bootstrap.ConfigRef.DeepCopy()

	tests := []computeDesiredMachineTestCase{
		{
			name: "should return the correct Machine object when creating a new Machine",
			ms: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: metav1.NamespaceDefault,
					Name:      msName,
					Labels: map[string]string{
						clusterv1.MachineDeploymentNameLabel: mdName,
					},
				},
				Spec: clusterv1.MachineSetSpec{
					ClusterName:     testClusterName,
					Replicas:        ptr.To[int32](3),
					MinReadySeconds: 10,
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"k1": "v1"},
					},
					MachineNamingStrategy: &clusterv1.MachineNamingStrategy{
						Template: "{{ .machineSet.name }}" + namingTemplateKey + "-{{ .random }}",
					},
					Template: machineTemplateSpec,
				},
			},
			existingMachine: nil,
			wantMachine:     expectedNewMachine,
			wantName: []gomegatypes.GomegaMatcher{
				HavePrefix(msName + namingTemplateKey + "-"),
				Not(HaveSuffix("-")),
			},
		},
		{
			name: "should return error when creating a new Machine when '.random' is not added in template",
			ms: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: metav1.NamespaceDefault,
					Name:      msName,
					Labels: map[string]string{
						clusterv1.MachineDeploymentNameLabel: mdName,
					},
				},
				Spec: clusterv1.MachineSetSpec{
					ClusterName:     testClusterName,
					Replicas:        ptr.To[int32](3),
					MinReadySeconds: 10,
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"k1": "v1"},
					},
					MachineNamingStrategy: &clusterv1.MachineNamingStrategy{
						Template: "{{ .machineSet.name }}" + namingTemplateKey,
					},
					Template: machineTemplateSpec,
				},
			},
			existingMachine: nil,
			wantMachine:     nil,
		},
		{
			name: "should not return error when creating a new Machine when the generated name exceeds 63",
			ms: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: metav1.NamespaceDefault,
					Name:      msName,
					Labels: map[string]string{
						clusterv1.MachineDeploymentNameLabel: mdName,
					},
				},
				Spec: clusterv1.MachineSetSpec{
					ClusterName:     testClusterName,
					Replicas:        ptr.To[int32](3),
					MinReadySeconds: 10,
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"k1": "v1"},
					},
					MachineNamingStrategy: &clusterv1.MachineNamingStrategy{
						Template: "{{ .random }}" + fmt.Sprintf("%059d", 0),
					},
					Template: machineTemplateSpec,
				},
			},
			existingMachine: nil,
			wantMachine:     expectedNewMachine,
			wantName: []gomegatypes.GomegaMatcher{
				ContainSubstring(fmt.Sprintf("%053d", 0)),
				Not(HaveSuffix("00000")),
			},
		},
		{
			name: "should return error when creating a new Machine with invalid template",
			ms: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: metav1.NamespaceDefault,
					Name:      msName,
					Labels: map[string]string{
						clusterv1.MachineDeploymentNameLabel: mdName,
					},
				},
				Spec: clusterv1.MachineSetSpec{
					ClusterName:     testClusterName,
					Replicas:        ptr.To[int32](3),
					MinReadySeconds: 10,
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"k1": "v1"},
					},
					MachineNamingStrategy: &clusterv1.MachineNamingStrategy{
						Template: "some-hardcoded-name-{{ .doesnotexistindata }}-{{ .random }}", // invalid template
					},
					Template: machineTemplateSpec,
				},
			},
			existingMachine: nil,
			wantMachine:     nil,
		},
		{
			name: "should return the correct Machine object when creating a new Machine with default templated name",
			ms: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: metav1.NamespaceDefault,
					Name:      msName,
					Labels: map[string]string{
						clusterv1.MachineDeploymentNameLabel: mdName,
					},
				},
				Spec: clusterv1.MachineSetSpec{
					ClusterName:     testClusterName,
					Replicas:        ptr.To[int32](3),
					MinReadySeconds: 10,
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"k1": "v1"},
					},
					Template: machineTemplateSpec,
				},
			},
			existingMachine: nil,
			wantMachine:     expectedNewMachine,
			wantName: []gomegatypes.GomegaMatcher{
				HavePrefix(msName),
			},
		},
		{
			name: "updating an existing Machine",
			ms: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: metav1.NamespaceDefault,
					Name:      msName,
					Labels: map[string]string{
						clusterv1.MachineDeploymentNameLabel: mdName,
					},
				},
				Spec: clusterv1.MachineSetSpec{
					ClusterName:     testClusterName,
					Replicas:        ptr.To[int32](3),
					MinReadySeconds: 10,
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"k1": "v1"},
					},
					Template: machineTemplateSpec,
				},
			},
			existingMachine: existingMachine,
			wantMachine:     expectedUpdatedMachine,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			var got *clusterv1.Machine
			var err error
			msr := &Reconciler{
				Client: fake.NewClientBuilder().WithObjects(
					testCluster,
					tt.ms,
				).WithStatusSubresource(&clusterv1.MachineSet{}).Build(),
				recorder: record.NewFakeRecorder(32),
			}
			got, err = msr.computeDesiredMachine(tt.ms, tt.existingMachine)

			if tt.wantMachine == nil {
				g.Expect(err).To(HaveOccurred())
				return
			}
			assertMachine(g, got, tt.wantMachine, tt.existingMachine, tt.wantName)
		})
	}
}

func assertMachine(g *WithT, actualMachine *clusterv1.Machine, expectedMachine *clusterv1.Machine, existingMachine *clusterv1.Machine, nameMatches []gomegatypes.GomegaMatcher) {
	// Check Name
	if existingMachine == nil {
		for _, matcher := range nameMatches {
			g.Expect(actualMachine.Name).To(matcher)
		}
	}
	if expectedMachine.Name != "" {
		g.Expect(actualMachine.Name).Should(Equal(expectedMachine.Name))
	}
	// Check UID
	if expectedMachine.UID != "" {
		g.Expect(actualMachine.UID).Should(Equal(expectedMachine.UID))
	}
	// Check Namespace
	g.Expect(actualMachine.Namespace).Should(Equal(expectedMachine.Namespace))
	// Check Labels
	for k, v := range expectedMachine.Labels {
		g.Expect(actualMachine.Labels).Should(HaveKeyWithValue(k, v))
	}
	// Check Annotations
	for k, v := range expectedMachine.Annotations {
		g.Expect(actualMachine.Annotations).Should(HaveKeyWithValue(k, v))
	}
	// Check Spec
	g.Expect(actualMachine.Spec).Should(BeComparableTo(expectedMachine.Spec))
	// Check Finalizer
	if expectedMachine.Finalizers != nil {
		g.Expect(actualMachine.Finalizers).Should(Equal(expectedMachine.Finalizers))
	}
}

func TestReconciler_reconcileDelete(t *testing.T) {
	labels := map[string]string{
		"some": "labelselector",
	}
	ms := builder.MachineSet("default", "ms0").WithClusterName("test").Build()
	ms.Finalizers = []string{
		clusterv1.MachineSetFinalizer,
	}
	ms.DeletionTimestamp = ptr.To(metav1.Now())
	ms.Spec.Selector = metav1.LabelSelector{
		MatchLabels: labels,
	}
	msWithoutFinalizer := ms.DeepCopy()
	msWithoutFinalizer.Finalizers = []string{}
	tests := []struct {
		name         string
		machineSet   *clusterv1.MachineSet
		want         *clusterv1.MachineSet
		objs         []client.Object
		wantMachines []clusterv1.Machine
		expectError  bool
	}{
		{
			name:         "Should do nothing when no descendant Machines exist and finalizer is already gone",
			machineSet:   msWithoutFinalizer.DeepCopy(),
			want:         msWithoutFinalizer.DeepCopy(),
			objs:         nil,
			wantMachines: nil,
			expectError:  false,
		},
		{
			name:         "Should remove finalizer when no descendant Machines exist",
			machineSet:   ms.DeepCopy(),
			want:         msWithoutFinalizer.DeepCopy(),
			objs:         nil,
			wantMachines: nil,
			expectError:  false,
		},
		{
			name:       "Should keep finalizer when descendant Machines exist and trigger deletion only for descendant Machines",
			machineSet: ms.DeepCopy(),
			want:       ms.DeepCopy(),
			objs: []client.Object{
				builder.Machine("default", "m0").WithClusterName("test").WithLabels(labels).Build(),
				builder.Machine("default", "m1").WithClusterName("test").WithLabels(labels).Build(),
				builder.Machine("default", "m2-not-part-of-ms").WithClusterName("test").Build(),
				builder.Machine("default", "m3-not-part-of-ms").WithClusterName("test").Build(),
			},
			wantMachines: []clusterv1.Machine{
				*builder.Machine("default", "m2-not-part-of-ms").WithClusterName("test").Build(),
				*builder.Machine("default", "m3-not-part-of-ms").WithClusterName("test").Build(),
			},
			expectError: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := fake.NewClientBuilder().WithObjects(tt.objs...).Build()
			r := &Reconciler{
				Client:   c,
				recorder: record.NewFakeRecorder(32),
			}

			s := &scope{
				machineSet: tt.machineSet,
			}

			// populate s.machines
			_, err := r.getAndAdoptMachinesForMachineSet(ctx, s)
			g.Expect(err).ToNot(HaveOccurred())
			_, err = r.reconcileDelete(ctx, s)
			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			g.Expect(tt.machineSet).To(BeComparableTo(tt.want))

			machineList := &clusterv1.MachineList{}
			g.Expect(c.List(ctx, machineList, client.InNamespace("default"))).ToNot(HaveOccurred())

			// Remove ResourceVersion so we can actually compare.
			for i := range machineList.Items {
				machineList.Items[i].ResourceVersion = ""
			}

			g.Expect(machineList.Items).To(ConsistOf(tt.wantMachines))
		})
	}
}

func TestNewMachineUpToDateCondition(t *testing.T) {
	reconciliationTime := time.Now()
	tests := []struct {
		name              string
		machineDeployment *clusterv1.MachineDeployment
		machineSet        *clusterv1.MachineSet
		expectCondition   *metav1.Condition
	}{
		{
			name:              "no condition returned for stand-alone MachineSet",
			machineDeployment: nil,
			machineSet:        &clusterv1.MachineSet{},
			expectCondition:   nil,
		},
		{
			name: "up-to-date",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: ptr.To("v1.31.0"),
						},
					},
				},
			},
			machineSet: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: ptr.To("v1.31.0"),
						},
					},
				},
			},
			expectCondition: &metav1.Condition{
				Type:   clusterv1.MachineUpToDateV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineUpToDateV1Beta2Reason,
			},
		},
		{
			name: "not up-to-date",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: ptr.To("v1.31.0"),
						},
					},
				},
			},
			machineSet: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: ptr.To("v1.30.0"),
						},
					},
				},
			},
			expectCondition: &metav1.Condition{
				Type:    clusterv1.MachineUpToDateV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineNotUpToDateV1Beta2Reason,
				Message: "* Version v1.30.0, v1.31.0 required",
			},
		},
		{
			name: "up-to-date, spec.rolloutAfter not expired",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					RolloutAfter: &metav1.Time{Time: reconciliationTime.Add(1 * time.Hour)}, // rollout after not yet expired
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: ptr.To("v1.31.0"),
						},
					},
				},
			},
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Time{Time: reconciliationTime.Add(-1 * time.Hour)}, // MS created before rollout after
				},
				Spec: clusterv1.MachineSetSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: ptr.To("v1.31.0"),
						},
					},
				},
			},
			expectCondition: &metav1.Condition{
				Type:   clusterv1.MachineUpToDateV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineUpToDateV1Beta2Reason,
			},
		},
		{
			name: "not up-to-date, rollout After expired",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					RolloutAfter: &metav1.Time{Time: reconciliationTime.Add(-1 * time.Hour)}, // rollout after expired
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: ptr.To("v1.31.0"),
						},
					},
				},
			},
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Time{Time: reconciliationTime.Add(-2 * time.Hour)}, // MS created before rollout after
				},
				Spec: clusterv1.MachineSetSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: ptr.To("v1.31.0"),
						},
					},
				},
			},
			expectCondition: &metav1.Condition{
				Type:    clusterv1.MachineUpToDateV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineNotUpToDateV1Beta2Reason,
				Message: "* MachineDeployment spec.rolloutAfter expired",
			},
		},
		{
			name: "not up-to-date, rollout After expired and a new MS created",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					RolloutAfter: &metav1.Time{Time: reconciliationTime.Add(-2 * time.Hour)}, // rollout after expired
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: ptr.To("v1.31.0"),
						},
					},
				},
			},
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Time{Time: reconciliationTime.Add(-1 * time.Hour)}, // MS created after rollout after
				},
				Spec: clusterv1.MachineSetSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: ptr.To("v1.31.0"),
						},
					},
				},
			},
			expectCondition: &metav1.Condition{
				Type:   clusterv1.MachineUpToDateV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineUpToDateV1Beta2Reason,
			},
		},
		{
			name: "not up-to-date, version changed, rollout After expired",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					RolloutAfter: &metav1.Time{Time: reconciliationTime.Add(-1 * time.Hour)}, // rollout after expired
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: ptr.To("v1.30.0"),
						},
					},
				},
			},
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Time{Time: reconciliationTime.Add(-2 * time.Hour)}, // MS created before rollout after
				},
				Spec: clusterv1.MachineSetSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: ptr.To("v1.31.0"),
						},
					},
				},
			},
			expectCondition: &metav1.Condition{
				Type:   clusterv1.MachineUpToDateV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.MachineNotUpToDateV1Beta2Reason,
				Message: "* Version v1.31.0, v1.30.0 required\n" +
					"* MachineDeployment spec.rolloutAfter expired",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			s := &scope{
				owningMachineDeployment: tt.machineDeployment,
				machineSet:              tt.machineSet,
				reconciliationTime:      reconciliationTime,
			}

			condition := newMachineUpToDateCondition(s)
			if tt.expectCondition != nil {
				g.Expect(condition).ToNot(BeNil())
				g.Expect(*condition).To(v1beta2conditions.MatchCondition(*tt.expectCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
			} else {
				g.Expect(condition).To(BeNil())
			}
		})
	}
}

func TestSortMachinesToRemediate(t *testing.T) {
	unhealthyMachinesWithAnnotations := []*clusterv1.Machine{}
	for i := range 4 {
		unhealthyMachinesWithAnnotations = append(unhealthyMachinesWithAnnotations, &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:              fmt.Sprintf("unhealthy-annotated-machine-%d", i),
				Namespace:         "default",
				CreationTimestamp: metav1.Time{Time: metav1.Now().Add(time.Duration(i) * time.Second)},
				Annotations: map[string]string{
					clusterv1.RemediateMachineAnnotation: "",
				},
			},
			Status: clusterv1.MachineStatus{
				Conditions: []clusterv1.Condition{
					{
						Type:   clusterv1.MachineOwnerRemediatedCondition,
						Status: corev1.ConditionFalse,
					},
					{
						Type:   clusterv1.MachineHealthCheckSucceededCondition,
						Status: corev1.ConditionFalse,
					},
				},
				V1Beta2: &clusterv1.MachineV1Beta2Status{
					Conditions: []metav1.Condition{
						{
							Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
							Status:  metav1.ConditionFalse,
							Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationV1Beta2Reason,
							Message: "Waiting for remediation",
						},
						{
							Type:    clusterv1.MachineHealthCheckSucceededV1Beta2Condition,
							Status:  metav1.ConditionFalse,
							Reason:  clusterv1.MachineHealthCheckHasRemediateAnnotationV1Beta2Reason,
							Message: "Marked for remediation via cluster.x-k8s.io/remediate-machine annotation",
						},
					},
				},
			},
		})
	}

	unhealthyMachines := []*clusterv1.Machine{}
	for i := range 4 {
		unhealthyMachines = append(unhealthyMachines, &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:              fmt.Sprintf("unhealthy-machine-%d", i),
				Namespace:         "default",
				CreationTimestamp: metav1.Time{Time: metav1.Now().Add(time.Duration(i) * time.Second)},
			},
			Status: clusterv1.MachineStatus{
				Conditions: []clusterv1.Condition{
					{
						Type:   clusterv1.MachineOwnerRemediatedCondition,
						Status: corev1.ConditionFalse,
					},
					{
						Type:   clusterv1.MachineHealthCheckSucceededCondition,
						Status: corev1.ConditionFalse,
					},
				},
				V1Beta2: &clusterv1.MachineV1Beta2Status{
					Conditions: []metav1.Condition{
						{
							Type:    clusterv1.MachineOwnerRemediatedV1Beta2Condition,
							Status:  metav1.ConditionFalse,
							Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationV1Beta2Reason,
							Message: "Waiting for remediation",
						},
						{
							Type:    clusterv1.MachineHealthCheckSucceededV1Beta2Condition,
							Status:  metav1.ConditionFalse,
							Reason:  clusterv1.MachineHealthCheckHasRemediateAnnotationV1Beta2Reason,
							Message: "Marked for remediation via cluster.x-k8s.io/remediate-machine annotation",
						},
					},
				},
			},
		})
	}

	t.Run("remediation machines should be sorted with newest first", func(t *testing.T) {
		g := NewWithT(t)
		machines := make([]*clusterv1.Machine, len(unhealthyMachines))
		copy(machines, unhealthyMachines)
		sortMachinesToRemediate(machines)
		sort.SliceStable(unhealthyMachines, func(i, j int) bool {
			return unhealthyMachines[i].CreationTimestamp.After(unhealthyMachines[j].CreationTimestamp.Time)
		})
		g.Expect(unhealthyMachines).To(Equal(machines))
	})

	t.Run("remediation machines with annotation should be prioritised over other machines", func(t *testing.T) {
		g := NewWithT(t)

		machines := make([]*clusterv1.Machine, len(unhealthyMachines))
		copy(machines, unhealthyMachines)
		machines = append(machines, unhealthyMachinesWithAnnotations...)
		sortMachinesToRemediate(machines)

		sort.SliceStable(unhealthyMachines, func(i, j int) bool {
			return unhealthyMachines[i].CreationTimestamp.After(unhealthyMachines[j].CreationTimestamp.Time)
		})
		sort.SliceStable(unhealthyMachinesWithAnnotations, func(i, j int) bool {
			return unhealthyMachinesWithAnnotations[i].CreationTimestamp.After(unhealthyMachinesWithAnnotations[j].CreationTimestamp.Time)
		})
		g.Expect(machines).To(Equal(append(unhealthyMachinesWithAnnotations, unhealthyMachines...)))
	})
}
