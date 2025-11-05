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
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
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
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      testClusterName,
			},
			Spec: clusterv1.ClusterSpec{
				ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
					APIGroup: builder.ControlPlaneGroupVersion.Group,
					Kind:     builder.GenericControlPlaneKind,
					Name:     "cp1",
				},
			},
		}
		g.Expect(env.Create(ctx, cluster)).To(Succeed())

		t.Log("Creating the Cluster Kubeconfig Secret")
		g.Expect(env.CreateKubeconfigSecret(ctx, cluster)).To(Succeed())

		// Set InfrastructureReady to true so ClusterCache creates the clusterAccessor.
		patch := client.MergeFrom(cluster.DeepCopy())
		cluster.Status.Initialization.InfrastructureProvisioned = ptr.To(true)
		cluster.Status.Deprecated = &clusterv1.ClusterDeprecatedStatus{
			V1Beta1: &clusterv1.ClusterV1Beta1DeprecatedStatus{
				Conditions: clusterv1.Conditions{
					{Type: clusterv1.ControlPlaneInitializedV1Beta1Condition, Status: corev1.ConditionTrue, LastTransitionTime: metav1.Now()},
				},
			},
		}
		cluster.Status.Conditions = []metav1.Condition{{Type: clusterv1.ClusterControlPlaneInitializedCondition, Status: metav1.ConditionTrue, Reason: clusterv1.ClusterControlPlaneInitializedReason, LastTransitionTime: metav1.Now()}}
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

		duration10m := ptr.To(int32(10 * 60))
		duration5m := ptr.To(int32(5 * 60))
		replicas := int32(2)
		version := "v1.14.2"
		machineTemplateSpec := clusterv1.MachineTemplateSpec{
			ObjectMeta: clusterv1.ObjectMeta{
				Labels: map[string]string{
					"label-1":    "true",
					"precedence": "MachineSet",
				},
				Annotations: map[string]string{
					"annotation-1": "true",
					"precedence":   "MachineSet",
				},
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: testCluster.Name,
				Version:     version,
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: clusterv1.GroupVersionBootstrap.Group,
						Kind:     "GenericBootstrapConfigTemplate",
						Name:     "ms-template",
					},
				},
				InfrastructureRef: clusterv1.ContractVersionedObjectReference{
					APIGroup: clusterv1.GroupVersionInfrastructure.Group,
					Kind:     "GenericInfrastructureMachineTemplate",
					Name:     "ms-template",
				},
				Deletion: clusterv1.MachineDeletionSpec{
					NodeDrainTimeoutSeconds:        duration10m,
					NodeDeletionTimeoutSeconds:     duration10m,
					NodeVolumeDetachTimeoutSeconds: duration10m,
				},
				MinReadySeconds: ptr.To[int32](0),
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
			"apiVersion": clusterv1.GroupVersionBootstrap.String(),
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"precedence": "GenericBootstrapConfig",
				},
				"labels": map[string]interface{}{
					"precedence": "GenericBootstrapConfig",
				},
			},
			"spec": map[string]interface{}{
				"format": "cloud-init",
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
		bootstrapTmpl.SetAPIVersion(clusterv1.GroupVersionBootstrap.String())
		bootstrapTmpl.SetName("ms-template")
		bootstrapTmpl.SetNamespace(namespace.Name)
		g.Expect(env.Create(ctx, bootstrapTmpl)).To(Succeed())

		// Create infrastructure template resource.
		infraResource := map[string]interface{}{
			"kind":       "GenericInfrastructureMachine",
			"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"precedence": "GenericInfrastructureMachineTemplate",
				},
				"labels": map[string]interface{}{
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
		infraTmpl.SetAPIVersion(clusterv1.GroupVersionInfrastructure.String())
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
			obj, err := external.GetObjectFromContractVersionedRef(ctx, env, instance.Spec.Template.Spec.Bootstrap.ConfigRef, instance.Namespace)
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
			obj, err := external.GetObjectFromContractVersionedRef(ctx, env, instance.Spec.Template.Spec.InfrastructureRef, instance.Namespace)
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
		machinesMap := map[string]*clusterv1.Machine{}
		for _, m := range machines.Items {
			machinesMap[m.Name] = &m
		}

		t.Log("Verify InfrastructureMachine for each Machine")
		infraMachines := &unstructured.UnstructuredList{}
		infraMachines.SetAPIVersion(clusterv1.GroupVersionInfrastructure.String())
		infraMachines.SetKind("GenericInfrastructureMachine")
		g.Eventually(func(g Gomega) {
			g.Expect(env.List(ctx, infraMachines, client.InNamespace(namespace.Name))).To(Succeed())
			g.Expect(infraMachines.Items).To(HaveLen(int(replicas)))
			for _, im := range infraMachines.Items {
				g.Expect(machinesMap).To(HaveKey(im.GetName()))
				g.Expect(im.GetAnnotations()).To(HaveKeyWithValue("annotation-1", "true"), "have annotations of MachineTemplate applied")
				g.Expect(im.GetAnnotations()).To(HaveKeyWithValue("precedence", "MachineSet"), "the annotations from the MachineSpec template to overwrite the infrastructure template ones")
				g.Expect(im.GetLabels()).To(HaveKeyWithValue("label-1", "true"), "have labels of MachineTemplate applied")
				g.Expect(im.GetLabels()).To(HaveKeyWithValue("precedence", "MachineSet"), "the labels from the MachineSpec template to overwrite the infrastructure template ones")
				g.Expect(cleanupTime(im.GetManagedFields())).To(ConsistOf(toManagedFields([]managedFieldEntry{{
					// capi-machineset-metadata owns labels and annotations.
					APIVersion: im.GetAPIVersion(),
					Manager:    machineSetMetadataManagerName,
					Operation:  metav1.ManagedFieldsOperationApply,
					FieldsV1: `{					
"f:metadata":{
	"f:annotations":{
		"f:annotation-1":{},
		"f:precedence":{}
	},
	"f:labels":{
		"f:cluster.x-k8s.io/cluster-name":{},
		"f:cluster.x-k8s.io/deployment-name":{},
		"f:cluster.x-k8s.io/set-name":{},
		"f:label-1":{},
		"f:precedence":{}
	}
}}`,
				}, {
					// capi-machineset owns spec.
					APIVersion: im.GetAPIVersion(),
					Manager:    machineSetManagerName,
					Operation:  metav1.ManagedFieldsOperationApply,
					FieldsV1: `{
"f:spec":{
	"f:size":{}
}}`,
				}, {
					// Machine manager owns ownerReference.
					APIVersion: im.GetAPIVersion(),
					Manager:    "manager",
					Operation:  metav1.ManagedFieldsOperationUpdate,
					FieldsV1: fmt.Sprintf(`{
"f:metadata":{
	"f:ownerReferences":{
		"k:{\"uid\":\"%s\"}":{}
	}
}}`, machinesMap[im.GetName()].UID),
				}})))
			}
		}, timeout).Should(Succeed())

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

		t.Log("Verify BootstrapConfig for each Machine")
		bootstrapConfigs := &unstructured.UnstructuredList{}
		bootstrapConfigs.SetAPIVersion(clusterv1.GroupVersionBootstrap.String())
		bootstrapConfigs.SetKind("GenericBootstrapConfig")
		g.Eventually(func(g Gomega) {
			g.Expect(env.List(ctx, bootstrapConfigs, client.InNamespace(namespace.Name))).To(Succeed())
			g.Expect(bootstrapConfigs.Items).To(HaveLen(int(replicas)))
			for _, im := range bootstrapConfigs.Items {
				g.Expect(im.GetAnnotations()).To(HaveKeyWithValue("annotation-1", "true"), "have annotations of MachineTemplate applied")
				g.Expect(im.GetAnnotations()).To(HaveKeyWithValue("precedence", "MachineSet"), "the annotations from the MachineSpec template to overwrite the bootstrap config template ones")
				g.Expect(im.GetLabels()).To(HaveKeyWithValue("label-1", "true"), "have labels of MachineTemplate applied")
				g.Expect(im.GetLabels()).To(HaveKeyWithValue("precedence", "MachineSet"), "the labels from the MachineSpec template to overwrite the bootstrap config template ones")
				g.Expect(cleanupTime(im.GetManagedFields())).To(ConsistOf(toManagedFields([]managedFieldEntry{{
					// capi-machineset-metadata owns labels and annotations.
					APIVersion: im.GetAPIVersion(),
					Manager:    machineSetMetadataManagerName,
					Operation:  metav1.ManagedFieldsOperationApply,
					FieldsV1: `{					
"f:metadata":{
	"f:annotations":{
		"f:annotation-1":{},
		"f:precedence":{}
	},
	"f:labels":{
		"f:cluster.x-k8s.io/cluster-name":{},
		"f:cluster.x-k8s.io/deployment-name":{},
		"f:cluster.x-k8s.io/set-name":{},
		"f:label-1":{},
		"f:precedence":{}
	}
}}`,
				}, {
					// capi-machineset owns spec.
					APIVersion: im.GetAPIVersion(),
					Manager:    machineSetManagerName,
					Operation:  metav1.ManagedFieldsOperationApply,
					FieldsV1: `{
"f:spec":{
	"f:format":{}
}}`,
				}, {
					// Machine manager owns ownerReference.
					APIVersion: im.GetAPIVersion(),
					Manager:    "manager",
					Operation:  metav1.ManagedFieldsOperationUpdate,
					FieldsV1: fmt.Sprintf(`{
"f:metadata":{
	"f:ownerReferences":{
		"k:{\"uid\":\"%s\"}":{}
	}
}}`, machinesMap[im.GetName()].UID),
				}})))
			}
		}, timeout).Should(Succeed())
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
			fakeBootstrapRefDataSecretCreated(m.Spec.Bootstrap.ConfigRef, m.Namespace, bootstrapResource, g)
			fakeInfrastructureRefProvisioned(m.Spec.InfrastructureRef, m.Namespace, infraResource, g)
		}

		// Verify that in-place mutable fields propagate from MachineSet to Machines.
		t.Log("Updating NodeDrainTimeoutSeconds on MachineSet")
		patchHelper, err := patch.NewHelper(instance, env)
		g.Expect(err).ToNot(HaveOccurred())
		instance.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds = duration5m
		g.Expect(patchHelper.Patch(ctx, instance)).Should(Succeed())

		t.Log("Verifying new NodeDrainTimeoutSeconds value is set on Machines")
		g.Eventually(func() bool {
			if err := env.List(ctx, machines, client.InNamespace(namespace.Name)); err != nil {
				return false
			}
			// All the machines should have the new NodeDrainTimeoutValue
			for _, m := range machines.Items {
				if m.Spec.Deletion.NodeDrainTimeoutSeconds == nil {
					return false
				}
				if *m.Spec.Deletion.NodeDrainTimeoutSeconds != *duration5m {
					return false
				}
			}
			return true
		}, timeout).Should(BeTrue(), "machine should have the updated NodeDrainTimeoutSeconds value")

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

			g.Expect(m.Spec.Version).To(BeEquivalentTo("v1.14.2"))
			fakeBootstrapRefDataSecretCreated(m.Spec.Bootstrap.ConfigRef, m.Namespace, bootstrapResource, g)
			providerID := fakeInfrastructureRefProvisioned(m.Spec.InfrastructureRef, m.Namespace, infraResource, g)
			fakeMachineNodeRef(&m, providerID, g)
		}

		// Verify that all Machines are Ready.
		g.Eventually(func() int32 {
			key := client.ObjectKey{Name: instance.Name, Namespace: instance.Namespace}
			if err := env.Get(ctx, key, instance); err != nil {
				return -1
			}
			return ptr.Deref(instance.Status.AvailableReplicas, 0)
		}, timeout).Should(BeEquivalentTo(replicas))

		t.Log("Verifying MachineSet has MachinesCreatedCondition")
		g.Eventually(func() bool {
			key := client.ObjectKey{Name: instance.Name, Namespace: instance.Namespace}
			if err := env.Get(ctx, key, instance); err != nil {
				return false
			}
			return v1beta1conditions.IsTrue(instance, clusterv1.MachinesCreatedV1Beta1Condition)
		}, timeout).Should(BeTrue())

		t.Log("Verifying MachineSet has ResizedCondition")
		g.Eventually(func() bool {
			key := client.ObjectKey{Name: instance.Name, Namespace: instance.Namespace}
			if err := env.Get(ctx, key, instance); err != nil {
				return false
			}
			return v1beta1conditions.IsTrue(instance, clusterv1.ResizedV1Beta1Condition)
		}, timeout).Should(BeTrue())

		t.Log("Verifying MachineSet has MachinesReadyCondition")
		g.Eventually(func() bool {
			key := client.ObjectKey{Name: instance.Name, Namespace: instance.Namespace}
			if err := env.Get(ctx, key, instance); err != nil {
				return false
			}
			return v1beta1conditions.IsTrue(instance, clusterv1.MachinesReadyV1Beta1Condition)
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
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Cluster",
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
			Status: clusterv1.MachineSetStatus{Conditions: []metav1.Condition{{
				Type:   clusterv1.PausedCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.NotPausedReason,
			}},
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

type newMachineSetOption func(m *clusterv1.MachineSet)

func newMachineSet(name, cluster string, replicas int32, opts ...newMachineSetOption) *clusterv1.MachineSet {
	ms := &clusterv1.MachineSet{
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
		Status: clusterv1.MachineSetStatus{Conditions: []metav1.Condition{{
			Type:   clusterv1.PausedCondition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.NotPausedReason,
		}},
		},
	}
	for _, opt := range opts {
		opt(ms)
	}
	return ms
}

func withMachineSetLabels(labels map[string]string) newMachineSetOption {
	return func(m *clusterv1.MachineSet) {
		m.Labels = labels
	}
}

func withMachineSetAnnotations(annotations map[string]string) newMachineSetOption {
	return func(m *clusterv1.MachineSet) {
		m.Annotations = annotations
	}
}

func withDeletionOrder(order clusterv1.MachineSetDeletionOrder) newMachineSetOption {
	return func(m *clusterv1.MachineSet) {
		m.Spec.Deletion.Order = order
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
			ClusterName: cluster.Name,
			Replicas:    &replicas,
			Template: clusterv1.MachineTemplateSpec{
				ObjectMeta: clusterv1.ObjectMeta{
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: cluster.Name,
					},
				},
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						Kind:     builder.GenericInfrastructureMachineTemplateKind,
						APIGroup: builder.InfrastructureGroupVersion.Group,
						// Try to break Infra Cloning
						Name: "something_invalid",
					},
					Version: version,
				},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
			},
		},
		Status: clusterv1.MachineSetStatus{
			Conditions: []metav1.Condition{{
				Type:   clusterv1.PausedCondition,
				Status: metav1.ConditionFalse,
				Reason: clusterv1.NotPausedReason,
			}},
		},
	}

	key := util.ObjectKey(ms)
	request := reconcile.Request{
		NamespacedName: key,
	}
	scheme := runtime.NewScheme()
	g.Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, ms, builder.GenericInfrastructureMachineTemplateCRD.DeepCopy()).WithStatusSubresource(&clusterv1.MachineSet{}).Build()

	msr := &Reconciler{
		Client:   fakeClient,
		recorder: record.NewFakeRecorder(32),
	}
	_, err := msr.Reconcile(ctx, request)
	g.Expect(err).To(HaveOccurred())
	g.Expect(fakeClient.Get(ctx, key, ms)).To(Succeed())
	gotCond := v1beta1conditions.Get(ms, clusterv1.MachinesCreatedV1Beta1Condition)
	g.Expect(gotCond).ToNot(BeNil())
	g.Expect(gotCond.Status).To(Equal(corev1.ConditionFalse))
	g.Expect(gotCond.Reason).To(Equal(clusterv1.InfrastructureTemplateCloningFailedV1Beta1Reason))
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
			expectedReason:  clusterv1.ScalingUpV1Beta1Reason,
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
			expectedReason:  clusterv1.ScalingDownV1Beta1Reason,
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
			g.Expect(msr.reconcileV1Beta1Status(ctx, s)).To(Succeed())
			gotCond := v1beta1conditions.Get(tc.machineSet, clusterv1.ResizedV1Beta1Condition)
			g.Expect(gotCond).ToNot(BeNil())
			g.Expect(gotCond.Status).To(Equal(corev1.ConditionFalse))
			g.Expect(gotCond.Reason).To(Equal(tc.expectedReason))
			g.Expect(gotCond.Message).To(Equal(tc.expectedMessage))
		})
	}
}

func TestMachineSetReconciler_syncMachines(t *testing.T) {
	teardown := func(t *testing.T, g *WithT, ns *corev1.Namespace, cluster *clusterv1.Cluster) {
		t.Helper()

		t.Log("Deleting the Cluster")
		g.Expect(env.Delete(ctx, cluster)).To(Succeed())
		t.Log("Deleting the namespace")
		g.Expect(env.Delete(ctx, ns)).To(Succeed())
	}

	g := NewWithT(t)

	t.Log("Creating the namespace")
	namespace, err := env.CreateNamespace(ctx, "test-machine-set-reconciler-sync-machines")
	g.Expect(err).ToNot(HaveOccurred())

	t.Log("Creating the Cluster")
	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace.Name,
			Name:      testClusterName,
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: builder.ControlPlaneGroupVersion.Group,
				Kind:     builder.GenericControlPlaneKind,
				Name:     "cp1",
			},
		},
	}
	g.Expect(env.CreateAndWait(ctx, testCluster)).To(Succeed())

	t.Log("Creating the Cluster Kubeconfig Secret")
	g.Expect(env.CreateKubeconfigSecret(ctx, testCluster)).To(Succeed())
	defer teardown(t, g, namespace, testCluster)

	replicas := int32(2)
	version := "v1.25.3"
	duration10s := ptr.To(int32(10))
	ms := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ms-1",
			Namespace: namespace.Name,
			Annotations: map[string]string{
				// Pause the MachineSet controller that is running in the background as part of the controller
				// suite as we want to control which part of the MachineSet controller is run in this test
				// by calling the syncMachines func explicitly.
				clusterv1.PausedAnnotation: "",
			},
			Labels: map[string]string{
				"label-1": "true",
				// Note: not adding the clusterv1.MachineDeploymentNameLabel to keep the test simple.
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
					Version:     version,
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: clusterv1.GroupVersionBootstrap.Group,
							Kind:     "GenericBootstrapConfigTemplate",
							Name:     "ms-template",
						},
					},
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: clusterv1.GroupVersionInfrastructure.Group,
						Kind:     "GenericInfrastructureMachineTemplate",
						Name:     "ms-template",
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
			"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
			"metadata": map[string]interface{}{
				"name":      "infra-machine-1",
				"namespace": namespace.Name,
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

	bootstrapConfigSpec := map[string]interface{}{
		"bootstrap-field": "bootstrap-value",
	}
	bootstrapConfig := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericBootstrapConfig",
			"apiVersion": clusterv1.GroupVersionBootstrap.String(),
			"metadata": map[string]interface{}{
				"name":      "bootstrap-config-1",
				"namespace": namespace.Name,
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
			"spec": bootstrapConfigSpec,
		},
	}

	inPlaceMutatingMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
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
			ClusterName: testCluster.Name,
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				Name:     infraMachine.GetName(),
				APIGroup: infraMachine.GroupVersionKind().Group,
				Kind:     infraMachine.GetKind(),
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: clusterv1.ContractVersionedObjectReference{
					Name:     bootstrapConfig.GetName(),
					APIGroup: bootstrapConfig.GroupVersionKind().Group,
					Kind:     bootstrapConfig.GetKind(),
				},
			},
		},
	}

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
				APIGroup: builder.InfrastructureGroupVersion.Group,
				Kind:     builder.TestInfrastructureMachineKind,
				Name:     "inframachine",
			},
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: ptr.To("machine-bootstrap-secret"),
			},
		},
	}

	//
	// Create objects
	//

	// Create MachineSet
	g.Expect(env.CreateAndWait(ctx, ms)).To(Succeed())

	// Create InfraMachine (same as in createInfraMachine)
	g.Expect(ssa.Patch(ctx, env.Client, machineSetManagerName, infraMachine)).To(Succeed())
	g.Expect(ssa.RemoveManagedFieldsForLabelsAndAnnotations(ctx, env.Client, env.GetAPIReader(), infraMachine, machineSetManagerName)).To(Succeed())

	// Create BootstrapConfig (same as in createBootstrapConfig)
	g.Expect(ssa.Patch(ctx, env.Client, machineSetManagerName, bootstrapConfig)).To(Succeed())
	g.Expect(ssa.RemoveManagedFieldsForLabelsAndAnnotations(ctx, env.Client, env.GetAPIReader(), bootstrapConfig, machineSetManagerName)).To(Succeed())

	// Set ownerReferences of the Machines to the MachineSet to ensure deterministic field ownership by capi-machineset below.
	// Note: There is code in the Machine controller that sets ownerRefs to the Cluster on standalone Machines.
	inPlaceMutatingMachine.SetOwnerReferences([]metav1.OwnerReference{*metav1.NewControllerRef(ms, machineSetKind)})
	deletingMachine.SetOwnerReferences([]metav1.OwnerReference{*metav1.NewControllerRef(ms, machineSetKind)})

	// Create Machines (same as in syncReplicas)
	g.Expect(ssa.Patch(ctx, env.Client, machineSetManagerName, inPlaceMutatingMachine)).To(Succeed())
	g.Expect(ssa.Patch(ctx, env.Client, machineSetManagerName, deletingMachine)).To(Succeed())
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
		// Note: Ensure the fieldManager defaults to manager like in prod.
		//       Otherwise it defaults to the binary name which is not manager in tests.
		Client:   client.WithFieldOwner(env.Client, "manager"),
		ssaCache: ssa.NewCache("test-controller"),
	}
	s := &scope{
		machineSet: ms,
		machines:   machines,
		getAndAdoptMachinesForMachineSetSucceeded: true,
	}
	_, err = reconciler.syncMachines(ctx, s)
	g.Expect(err).ToNot(HaveOccurred())

	updatedInPlaceMutatingMachine := inPlaceMutatingMachine.DeepCopy()
	g.Eventually(func(g Gomega) {
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedInPlaceMutatingMachine), updatedInPlaceMutatingMachine)).To(Succeed())
		statusManagedFields := managedFieldsMatching(updatedInPlaceMutatingMachine.ManagedFields, "manager", metav1.ManagedFieldsOperationUpdate, "status")
		g.Expect(statusManagedFields).To(HaveLen(1))
		g.Expect(cleanupTime(updatedInPlaceMutatingMachine.ManagedFields)).To(ConsistOf(toManagedFields([]managedFieldEntry{{
			// capi-machineset owns almost everything.
			Manager:    machineSetManagerName,
			Operation:  metav1.ManagedFieldsOperationApply,
			APIVersion: clusterv1.GroupVersion.String(),
			FieldsV1:   fmt.Sprintf("{\"f:metadata\":{\"f:annotations\":{\"f:dropped-annotation\":{},\"f:modified-annotation\":{},\"f:preserved-annotation\":{}},\"f:finalizers\":{\"v:\\\"machine.cluster.x-k8s.io\\\"\":{}},\"f:labels\":{\"f:cluster.x-k8s.io/set-name\":{},\"f:dropped-label\":{},\"f:modified-label\":{},\"f:preserved-label\":{}},\"f:ownerReferences\":{\"k:{\\\"uid\\\":\\\"%s\\\"}\":{}}},\"f:spec\":{\"f:bootstrap\":{\"f:configRef\":{\"f:apiGroup\":{},\"f:kind\":{},\"f:name\":{}}},\"f:clusterName\":{},\"f:infrastructureRef\":{\"f:apiGroup\":{},\"f:kind\":{},\"f:name\":{}}}}", ms.UID),
		}, {
			// manager owns the finalizer.
			Manager:    "manager",
			Operation:  metav1.ManagedFieldsOperationUpdate,
			APIVersion: clusterv1.GroupVersion.String(),
			FieldsV1:   "{\"f:metadata\":{\"f:finalizers\":{\".\":{},\"v:\\\"machine.cluster.x-k8s.io\\\"\":{}}}}",
		}, {
			// manager owns status.
			Manager:    "manager",
			Operation:  metav1.ManagedFieldsOperationUpdate,
			APIVersion: clusterv1.GroupVersion.String(),
			// No need to verify details (a bit non-deterministic, as it depends on Machine controller reconciles).
			FieldsV1:    string(statusManagedFields[0].FieldsV1.Raw),
			Subresource: "status",
		}})))
	}, timeout).Should(Succeed())

	updatedInfraMachine := infraMachine.DeepCopy()
	g.Eventually(func(g Gomega) {
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedInfraMachine), updatedInfraMachine)).To(Succeed())
		g.Expect(cleanupTime(updatedInfraMachine.GetManagedFields())).To(ConsistOf(toManagedFields([]managedFieldEntry{{
			// capi-machineset-metadata owns labels and annotations.
			Manager:    machineSetMetadataManagerName,
			Operation:  metav1.ManagedFieldsOperationApply,
			APIVersion: updatedInfraMachine.GetAPIVersion(),
			FieldsV1:   "{\"f:metadata\":{\"f:annotations\":{\"f:dropped-annotation\":{},\"f:modified-annotation\":{},\"f:preserved-annotation\":{}},\"f:labels\":{\"f:cluster.x-k8s.io/set-name\":{},\"f:dropped-label\":{},\"f:modified-label\":{},\"f:preserved-label\":{}}}}",
		}, {
			// capi-machineset owns spec.
			Manager:    machineSetManagerName,
			Operation:  metav1.ManagedFieldsOperationApply,
			APIVersion: updatedInfraMachine.GetAPIVersion(),
			FieldsV1:   "{\"f:spec\":{\"f:infra-field\":{}}}",
		}, {
			// manager owns ClusterName label and ownerReferences.
			Manager:    "manager",
			Operation:  metav1.ManagedFieldsOperationUpdate,
			APIVersion: updatedInfraMachine.GetAPIVersion(),
			FieldsV1:   fmt.Sprintf("{\"f:metadata\":{\"f:labels\":{\"f:cluster.x-k8s.io/cluster-name\":{}},\"f:ownerReferences\":{\".\":{},\"k:{\\\"uid\\\":\\\"%s\\\"}\":{}}}}", inPlaceMutatingMachine.UID),
		}})))
	}, timeout).Should(Succeed())

	updatedBootstrapConfig := bootstrapConfig.DeepCopy()
	g.Eventually(func(g Gomega) {
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedBootstrapConfig), updatedBootstrapConfig)).To(Succeed())
		g.Expect(cleanupTime(updatedBootstrapConfig.GetManagedFields())).To(ConsistOf(toManagedFields([]managedFieldEntry{{
			// capi-machineset-metadata owns labels and annotations.
			Manager:    machineSetMetadataManagerName,
			Operation:  metav1.ManagedFieldsOperationApply,
			APIVersion: updatedBootstrapConfig.GetAPIVersion(),
			FieldsV1:   "{\"f:metadata\":{\"f:annotations\":{\"f:dropped-annotation\":{},\"f:modified-annotation\":{},\"f:preserved-annotation\":{}},\"f:labels\":{\"f:cluster.x-k8s.io/set-name\":{},\"f:dropped-label\":{},\"f:modified-label\":{},\"f:preserved-label\":{}}}}",
		}, {
			// capi-machineset owns spec.
			Manager:    machineSetManagerName,
			Operation:  metav1.ManagedFieldsOperationApply,
			APIVersion: updatedBootstrapConfig.GetAPIVersion(),
			FieldsV1:   "{\"f:spec\":{\"f:bootstrap-field\":{}}}",
		}, {
			// manager owns ClusterName label and ownerReferences.
			Manager:    "manager",
			Operation:  metav1.ManagedFieldsOperationUpdate,
			APIVersion: updatedBootstrapConfig.GetAPIVersion(),
			FieldsV1:   fmt.Sprintf("{\"f:metadata\":{\"f:labels\":{\"f:cluster.x-k8s.io/cluster-name\":{}},\"f:ownerReferences\":{\".\":{},\"k:{\\\"uid\\\":\\\"%s\\\"}\":{}}}}", inPlaceMutatingMachine.UID),
		}})))
	}, timeout).Should(Succeed())

	//
	// Verify In-place mutating fields
	//

	// Update the MachineSet and verify the in-place mutating fields are propagated.
	ms.Spec.Template.Labels = map[string]string{
		"preserved-label": "preserved-value",  // Keep the label and value as is
		"modified-label":  "modified-value-2", // Modify the value of the label
		// Drop "dropped-label"
	}
	expectedLabels := map[string]string{
		"preserved-label":             "preserved-value",
		"modified-label":              "modified-value-2",
		clusterv1.MachineSetNameLabel: ms.Name,
		clusterv1.ClusterNameLabel:    testCluster.Name, // This label is added by the Machine controller.
	}
	ms.Spec.Template.Annotations = map[string]string{
		"preserved-annotation": "preserved-value",  // Keep the annotation and value as is
		"modified-annotation":  "modified-value-2", // Modify the value of the annotation
		// Drop "dropped-annotation"
	}
	readinessGates := []clusterv1.MachineReadinessGate{{ConditionType: "foo"}}
	ms.Spec.Template.Spec.ReadinessGates = readinessGates
	ms.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds = duration10s
	ms.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds = duration10s
	ms.Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds = duration10s
	ms.Spec.Template.Spec.MinReadySeconds = ptr.To[int32](10)
	s = &scope{
		machineSet: ms,
		machines:   []*clusterv1.Machine{updatedInPlaceMutatingMachine, deletingMachine},
		getAndAdoptMachinesForMachineSetSucceeded: true,
	}
	_, err = reconciler.syncMachines(ctx, s)
	g.Expect(err).ToNot(HaveOccurred())

	// Verify in-place mutable fields are updated on the Machine.
	updatedInPlaceMutatingMachine = inPlaceMutatingMachine.DeepCopy()
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedInPlaceMutatingMachine), updatedInPlaceMutatingMachine)).To(Succeed())
	g.Expect(updatedInPlaceMutatingMachine.Labels).Should(Equal(expectedLabels))
	g.Expect(updatedInPlaceMutatingMachine.Annotations).Should(Equal(ms.Spec.Template.Annotations))
	g.Expect(updatedInPlaceMutatingMachine.Spec.Deletion.NodeDrainTimeoutSeconds).Should(Equal(ms.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds))
	g.Expect(updatedInPlaceMutatingMachine.Spec.Deletion.NodeDeletionTimeoutSeconds).Should(Equal(ms.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds))
	g.Expect(updatedInPlaceMutatingMachine.Spec.Deletion.NodeVolumeDetachTimeoutSeconds).Should(Equal(ms.Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds))
	g.Expect(updatedInPlaceMutatingMachine.Spec.MinReadySeconds).Should(Equal(ms.Spec.Template.Spec.MinReadySeconds))
	g.Expect(updatedInPlaceMutatingMachine.Spec.ReadinessGates).Should(Equal(readinessGates))

	// Verify in-place mutable fields are updated on InfrastructureMachine
	updatedInfraMachine = infraMachine.DeepCopy()
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedInfraMachine), updatedInfraMachine)).To(Succeed())
	g.Expect(updatedInfraMachine.GetLabels()).Should(Equal(expectedLabels))
	g.Expect(updatedInfraMachine.GetAnnotations()).Should(Equal(ms.Spec.Template.Annotations))
	// Verify spec remains the same
	g.Expect(updatedInfraMachine.Object).Should(HaveKeyWithValue("spec", infraMachineSpec))

	// Verify in-place mutable fields are updated on the BootstrapConfig.
	updatedBootstrapConfig = bootstrapConfig.DeepCopy()
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedBootstrapConfig), updatedBootstrapConfig)).To(Succeed())
	g.Expect(updatedBootstrapConfig.GetLabels()).Should(Equal(expectedLabels))
	g.Expect(updatedBootstrapConfig.GetAnnotations()).Should(Equal(ms.Spec.Template.Annotations))
	// Verify spec remains the same
	g.Expect(updatedBootstrapConfig.Object).Should(HaveKeyWithValue("spec", bootstrapConfigSpec))

	// Verify ManagedFields
	g.Eventually(func(g Gomega) {
		updatedDeletingMachine := deletingMachine.DeepCopy()
		g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedDeletingMachine), updatedDeletingMachine)).To(Succeed())
		statusManagedFields := managedFieldsMatching(updatedDeletingMachine.ManagedFields, "manager", metav1.ManagedFieldsOperationUpdate, "status")
		g.Expect(statusManagedFields).To(HaveLen(1))
		g.Expect(cleanupTime(updatedDeletingMachine.ManagedFields)).To(ConsistOf(toManagedFields([]managedFieldEntry{{
			// capi-machineset owns almost everything.
			Manager:    machineSetManagerName,
			Operation:  metav1.ManagedFieldsOperationApply,
			APIVersion: clusterv1.GroupVersion.String(),
			FieldsV1:   fmt.Sprintf("{\"f:metadata\":{\"f:finalizers\":{\"v:\\\"testing-finalizer\\\"\":{}},\"f:ownerReferences\":{\"k:{\\\"uid\\\":\\\"%s\\\"}\":{}}},\"f:spec\":{\"f:bootstrap\":{\"f:dataSecretName\":{}},\"f:clusterName\":{},\"f:infrastructureRef\":{\"f:apiGroup\":{},\"f:kind\":{},\"f:name\":{}}}}", ms.UID),
		}, {
			// manager owns the fields that are propagated in-place for deleting Machines in syncMachines via patchHelper.
			Manager:    "manager",
			Operation:  metav1.ManagedFieldsOperationUpdate,
			APIVersion: clusterv1.GroupVersion.String(),
			FieldsV1:   "{\"f:spec\":{\"f:deletion\":{\"f:nodeDrainTimeoutSeconds\":{},\"f:nodeVolumeDetachTimeoutSeconds\":{}},\"f:minReadySeconds\":{},\"f:readinessGates\":{\".\":{},\"k:{\\\"conditionType\\\":\\\"foo\\\"}\":{\".\":{},\"f:conditionType\":{}}}}}",
		}, {
			// manager owns status.
			Manager:    "manager",
			Operation:  metav1.ManagedFieldsOperationUpdate,
			APIVersion: clusterv1.GroupVersion.String(),
			// No need to verify details (a bit non-deterministic, as it depends on Machine controller reconciles).
			FieldsV1:    string(statusManagedFields[0].FieldsV1.Raw),
			Subresource: "status",
		}})))
	}, timeout).Should(Succeed())

	// Verify in-place mutable fields are updated on the deleting Machine.
	updatedDeletingMachine := deletingMachine.DeepCopy()
	g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(updatedDeletingMachine), updatedDeletingMachine)).To(Succeed())
	g.Expect(updatedDeletingMachine.Labels).Should(Equal(deletingMachine.Labels))           // Not propagated to a deleting Machine
	g.Expect(updatedDeletingMachine.Annotations).Should(Equal(deletingMachine.Annotations)) // Not propagated to a deleting Machine
	g.Expect(updatedDeletingMachine.Spec.Deletion.NodeDrainTimeoutSeconds).Should(Equal(ms.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds))
	g.Expect(updatedDeletingMachine.Spec.Deletion.NodeDeletionTimeoutSeconds).Should(Equal(ms.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds))
	g.Expect(updatedDeletingMachine.Spec.Deletion.NodeVolumeDetachTimeoutSeconds).Should(Equal(ms.Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds))
	g.Expect(updatedDeletingMachine.Spec.MinReadySeconds).Should(Equal(ms.Spec.Template.Spec.MinReadySeconds))
	g.Expect(updatedDeletingMachine.Spec.ReadinessGates).Should(Equal(readinessGates))
	// Verify the machine spec is otherwise unchanged.
	deletingMachine.Spec.Deletion.NodeDrainTimeoutSeconds = ms.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds
	deletingMachine.Spec.Deletion.NodeDeletionTimeoutSeconds = ms.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds
	deletingMachine.Spec.Deletion.NodeVolumeDetachTimeoutSeconds = ms.Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds
	deletingMachine.Spec.MinReadySeconds = ms.Spec.Template.Spec.MinReadySeconds
	deletingMachine.Spec.ReadinessGates = ms.Spec.Template.Spec.ReadinessGates
	g.Expect(updatedDeletingMachine.Spec).Should(BeComparableTo(deletingMachine.Spec))
}

func TestMachineSetReconciler_reconcileUnhealthyMachines(t *testing.T) {
	// Use a separate scheme for fake client to avoid race conditions with the global scheme.
	scheme := runtime.NewScheme()
	_ = apiextensionsv1.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)

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
				ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneStable),
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
				Conditions: []metav1.Condition{
					{
						Type:    clusterv1.MachineOwnerRemediatedCondition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationReason,
						Message: "Waiting for remediation",
					},
					{
						Type:    clusterv1.MachineHealthCheckSucceededCondition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineHealthCheckHasRemediateAnnotationReason,
						Message: "Marked for remediation via cluster.x-k8s.io/remediate-machine annotation",
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
				Conditions: []metav1.Condition{
					{
						// This condition should be cleaned up because HealthCheckSucceeded is true.
						Type:    clusterv1.MachineOwnerRemediatedCondition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationReason,
						Message: "Waiting for remediation",
					},
					{
						Type:   clusterv1.MachineHealthCheckSucceededCondition,
						Status: metav1.ConditionTrue,
						Reason: clusterv1.MachineHealthCheckSucceededReason,
					},
				},
			},
		}

		machines := []*clusterv1.Machine{unhealthyMachine, healthyMachine}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(controlPlaneStable, unhealthyMachine, healthyMachine).WithStatusSubresource(&clusterv1.Machine{}).Build()
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
		g.Expect(v1beta1conditions.IsTrue(m, clusterv1.MachineOwnerRemediatedV1Beta1Condition)).To(BeTrue())
		c := conditions.Get(m, clusterv1.MachineOwnerRemediatedCondition)
		g.Expect(c).ToNot(BeNil())
		g.Expect(*c).To(conditions.MatchCondition(metav1.Condition{
			Type:    clusterv1.MachineOwnerRemediatedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineSetMachineRemediationMachineDeletingReason,
			Message: "Machine is deleting",
		}, conditions.IgnoreLastTransitionTime(true)))

		// Verify the healthy machine is not deleted and does not have the OwnerRemediated condition.
		m = &clusterv1.Machine{}
		g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(healthyMachine), m)).Should(Succeed())
		g.Expect(m.DeletionTimestamp.IsZero()).To(BeTrue())
		g.Expect(v1beta1conditions.Has(m, clusterv1.MachineOwnerRemediatedV1Beta1Condition)).To(BeFalse())
		g.Expect(conditions.Has(m, clusterv1.MachineOwnerRemediatedCondition)).To(BeFalse())
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
				ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneUpgrading),
			},
		}
		machineSet := &clusterv1.MachineSet{}

		unhealthyMachine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "unhealthy-machine",
				Namespace: "default",
			},
			Status: clusterv1.MachineStatus{
				Conditions: []metav1.Condition{
					{
						Type:    clusterv1.MachineOwnerRemediatedCondition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationReason,
						Message: "Waiting for remediation",
					},
					{
						Type:    clusterv1.MachineHealthCheckSucceededCondition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineHealthCheckHasRemediateAnnotationReason,
						Message: "Marked for remediation via cluster.x-k8s.io/remediate-machine annotation",
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
				Conditions: []metav1.Condition{
					{
						// This condition should be cleaned up because HealthCheckSucceeded is true.
						Type:    clusterv1.MachineOwnerRemediatedCondition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationReason,
						Message: "Waiting for remediation",
					},
					{
						Type:   clusterv1.MachineHealthCheckSucceededCondition,
						Status: metav1.ConditionTrue,
						Reason: clusterv1.MachineHealthCheckSucceededReason,
					},
				},
			},
		}

		machines := []*clusterv1.Machine{unhealthyMachine, healthyMachine}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(controlPlaneUpgrading, builder.GenericControlPlaneCRD, unhealthyMachine, healthyMachine).WithStatusSubresource(&clusterv1.Machine{}).Build()
		r := &Reconciler{
			Client:          fakeClient,
			PreflightChecks: sets.Set[clusterv1.MachineSetPreflightCheck]{}.Insert(clusterv1.MachineSetPreflightCheckAll),
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
		condition := clusterv1.MachineOwnerRemediatedV1Beta1Condition
		m := &clusterv1.Machine{}
		g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(unhealthyMachine), m)).To(Succeed())
		g.Expect(m.DeletionTimestamp.IsZero()).To(BeTrue())
		g.Expect(v1beta1conditions.Has(m, condition)).
			To(BeTrue(), "Machine should have the %s condition set", condition)
		machineOwnerRemediatedCondition := v1beta1conditions.Get(m, condition)
		g.Expect(machineOwnerRemediatedCondition.Status).
			To(Equal(corev1.ConditionFalse), "%s condition status should be false", condition)
		g.Expect(machineOwnerRemediatedCondition.Reason).
			To(Equal(clusterv1.WaitingForRemediationV1Beta1Reason), "%s condition should have reason %s", condition, clusterv1.WaitingForRemediationV1Beta1Reason)
		c := conditions.Get(m, clusterv1.MachineOwnerRemediatedCondition)
		g.Expect(c).ToNot(BeNil())
		g.Expect(*c).To(conditions.MatchCondition(metav1.Condition{
			Type:    clusterv1.MachineOwnerRemediatedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineSetMachineRemediationDeferredReason,
			Message: "* GenericControlPlane default/cp1 is upgrading (\"ControlPlaneIsStable\" preflight check failed)",
		}, conditions.IgnoreLastTransitionTime(true)))

		// Verify the healthy machine is not deleted and does not have the OwnerRemediated condition.
		m = &clusterv1.Machine{}
		g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(healthyMachine), m)).To(Succeed())
		g.Expect(m.DeletionTimestamp.IsZero()).To(BeTrue())
		g.Expect(v1beta1conditions.Has(m, condition)).
			To(BeFalse(), "Machine should not have the %s condition set", condition)
		g.Expect(conditions.Has(m, clusterv1.MachineOwnerRemediatedCondition)).To(BeFalse())
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
				Conditions: []metav1.Condition{
					{
						Type:    clusterv1.MachineOwnerRemediatedCondition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationReason,
						Message: "Waiting for remediation",
					},
					{
						Type:    clusterv1.MachineHealthCheckSucceededCondition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineHealthCheckHasRemediateAnnotationReason,
						Message: "Marked for remediation via cluster.x-k8s.io/remediate-machine annotation",
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
				Conditions: []metav1.Condition{
					{
						// This condition should be cleaned up because HealthCheckSucceeded is true.
						Type:    clusterv1.MachineOwnerRemediatedCondition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationReason,
						Message: "Waiting for remediation",
					},
					{
						Type:   clusterv1.MachineHealthCheckSucceededCondition,
						Status: metav1.ConditionTrue,
						Reason: clusterv1.MachineHealthCheckSucceededReason,
					},
				},
			},
		}

		machines := []*clusterv1.Machine{unhealthyMachine, healthyMachine}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
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

		condition := clusterv1.MachineOwnerRemediatedV1Beta1Condition
		m := &clusterv1.Machine{}

		// Verify that no action was taken on the Machine: MachineOwnerRemediated should be false
		// and the Machine wasn't deleted.
		g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(unhealthyMachine), m)).To(Succeed())
		g.Expect(unhealthyMachine.DeletionTimestamp).Should(BeZero())
		c := conditions.Get(m, clusterv1.MachineOwnerRemediatedCondition)
		g.Expect(c).ToNot(BeNil())
		g.Expect(*c).To(conditions.MatchCondition(metav1.Condition{
			Type:    clusterv1.MachineOwnerRemediatedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineSetMachineCannotBeRemediatedReason,
			Message: "Machine won't be remediated because it is pending removal due to rollout",
		}, conditions.IgnoreLastTransitionTime(true)))

		// Verify the healthy machine is not deleted and does not have the OwnerRemediated condition.
		m = &clusterv1.Machine{}
		g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(healthyMachine), m)).To(Succeed())
		g.Expect(m.DeletionTimestamp.IsZero()).To(BeTrue())
		g.Expect(v1beta1conditions.Has(m, condition)).
			To(BeFalse(), "Machine should not have the %s condition set", condition)
		g.Expect(conditions.Has(m, clusterv1.MachineOwnerRemediatedCondition)).To(BeFalse())

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
		g.Expect(v1beta1conditions.IsTrue(m, clusterv1.MachineOwnerRemediatedV1Beta1Condition)).To(BeTrue())
		c = conditions.Get(m, clusterv1.MachineOwnerRemediatedCondition)
		g.Expect(c).ToNot(BeNil())
		g.Expect(*c).To(conditions.MatchCondition(metav1.Condition{
			Type:    clusterv1.MachineOwnerRemediatedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineSetMachineRemediationMachineDeletingReason,
			Message: "Machine is deleting",
		}, conditions.IgnoreLastTransitionTime(true)))

		// Verify (again) the healthy machine is not deleted and does not have the OwnerRemediated condition.
		m = &clusterv1.Machine{}
		g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(healthyMachine), m)).To(Succeed())
		g.Expect(m.DeletionTimestamp.IsZero()).To(BeTrue())
		g.Expect(v1beta1conditions.Has(m, condition)).
			To(BeFalse(), "Machine should not have the %s condition set", condition)
		g.Expect(conditions.Has(m, clusterv1.MachineOwnerRemediatedCondition)).To(BeFalse())
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
				Remediation: clusterv1.MachineDeploymentRemediationSpec{
					MaxInFlight: ptr.To(intstr.FromInt32(int32(maxInFlight))),
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
					Conditions: []metav1.Condition{
						{
							Type:    clusterv1.MachineOwnerRemediatedCondition,
							Status:  metav1.ConditionFalse,
							Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationReason,
							Message: "Waiting for remediation",
						},
						{
							Type:    clusterv1.MachineHealthCheckSucceededCondition,
							Status:  metav1.ConditionFalse,
							Reason:  clusterv1.MachineHealthCheckHasRemediateAnnotationReason,
							Message: "Marked for remediation via cluster.x-k8s.io/remediate-machine annotation",
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
				Conditions: []metav1.Condition{
					{
						// This condition should be cleaned up because HealthCheckSucceeded is true.
						Type:    clusterv1.MachineOwnerRemediatedCondition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationReason,
						Message: "Waiting for remediation",
					},
					{
						Type:   clusterv1.MachineHealthCheckSucceededCondition,
						Status: metav1.ConditionTrue,
						Reason: clusterv1.MachineHealthCheckSucceededReason,
					},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, machineDeployment, healthyMachine).
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

		condition := clusterv1.MachineOwnerRemediatedV1Beta1Condition

		// Iterate over the unhealthy machines and verify that the last maxInFlight were deleted.
		for i := range unhealthyMachines {
			m := unhealthyMachines[i]

			err = r.Client.Get(ctx, client.ObjectKeyFromObject(m), m)
			if i < total-maxInFlight {
				// Machines before the maxInFlight should not be deleted.
				g.Expect(err).ToNot(HaveOccurred())
				c := conditions.Get(m, clusterv1.MachineOwnerRemediatedCondition)
				g.Expect(c).ToNot(BeNil())
				g.Expect(*c).To(conditions.MatchCondition(metav1.Condition{
					Type:    clusterv1.MachineOwnerRemediatedCondition,
					Status:  metav1.ConditionFalse,
					Reason:  clusterv1.MachineSetMachineRemediationDeferredReason,
					Message: "Waiting because there are already too many remediations in progress (spec.strategy.remediation.maxInFlight is 3)",
				}, conditions.IgnoreLastTransitionTime(true)))
			} else {
				// Machines after maxInFlight, should be deleted.
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected machine %d to be deleted", i)
			}
		}

		// Verify the healthy machine is not deleted and does not have the OwnerRemediated condition.
		m := &clusterv1.Machine{}
		g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(healthyMachine), m)).To(Succeed())
		g.Expect(m.DeletionTimestamp.IsZero()).To(BeTrue())
		g.Expect(v1beta1conditions.Has(m, condition)).
			To(BeFalse(), "Machine should not have the %s condition set", condition)
		g.Expect(conditions.Has(m, clusterv1.MachineOwnerRemediatedCondition)).To(BeFalse())

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
					c := conditions.Get(m, clusterv1.MachineOwnerRemediatedCondition)
					g.Expect(c).ToNot(BeNil())
					g.Expect(*c).To(conditions.MatchCondition(metav1.Condition{
						Type:    clusterv1.MachineOwnerRemediatedCondition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineSetMachineRemediationDeferredReason,
						Message: "Waiting because there are already too many remediations in progress (spec.strategy.remediation.maxInFlight is 3)",
					}, conditions.IgnoreLastTransitionTime(true)))
					g.Expect(m.DeletionTimestamp).To(BeZero())
				} else if i < total-maxInFlight {
					// Machines before the maxInFlight should have a deletion timestamp
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(v1beta1conditions.Has(m, condition)).
						To(BeTrue(), "Machine should have the %s condition set", condition)
					machineOwnerRemediatedCondition := v1beta1conditions.Get(m, condition)
					g.Expect(machineOwnerRemediatedCondition.Status).
						To(Equal(corev1.ConditionTrue), "%s condition status should be true", condition)
					c := conditions.Get(m, clusterv1.MachineOwnerRemediatedCondition)
					g.Expect(c).ToNot(BeNil())
					g.Expect(*c).To(conditions.MatchCondition(metav1.Condition{
						Type:    clusterv1.MachineOwnerRemediatedCondition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineSetMachineRemediationMachineDeletingReason,
						Message: "Machine is deleting",
					}, conditions.IgnoreLastTransitionTime(true)))
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
		g.Expect(v1beta1conditions.Has(m, condition)).
			To(BeFalse(), "Machine should not have the %s condition set", condition)
		g.Expect(conditions.Has(m, clusterv1.MachineOwnerRemediatedCondition)).To(BeFalse())

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
		g.Expect(v1beta1conditions.Has(m, condition)).
			To(BeFalse(), "Machine should not have the %s condition set", condition)
		g.Expect(conditions.Has(m, clusterv1.MachineOwnerRemediatedCondition)).To(BeFalse())

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
		g.Expect(v1beta1conditions.Has(m, condition)).
			To(BeFalse(), "Machine should not have the %s condition set", condition)
		g.Expect(conditions.Has(m, clusterv1.MachineOwnerRemediatedCondition)).To(BeFalse())
	})
}

func TestMachineSetReconciler_syncReplicas(t *testing.T) {
	tests := []struct {
		name                                      string
		getAndAdoptMachinesForMachineSetSucceeded bool
		machineSet                                *clusterv1.MachineSet
		machines                                  []*clusterv1.Machine
		expectMachinesToAdd                       *int
		expectMachinesToDelete                    *int
		expectedTargetMSName                      *string
		expectedMachinesToMove                    *int
	}{
		{
			name: "no op when getAndAdoptMachinesForMachineSetSucceeded is false",
			getAndAdoptMachinesForMachineSetSucceeded: false,
			machineSet:             newMachineSet("ms1", "cluster1", 0),
			machines:               []*clusterv1.Machine{},
			expectMachinesToAdd:    nil,
			expectMachinesToDelete: nil,
			expectedTargetMSName:   nil,
			expectedMachinesToMove: nil,
		},
		{
			name: "no op when all the expected machine already exists",
			getAndAdoptMachinesForMachineSetSucceeded: true,
			machineSet: newMachineSet("ms1", "cluster1", 2),
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
				fakeMachine("m2"),
			},
			expectMachinesToAdd:    nil,
			expectMachinesToDelete: nil,
			expectedTargetMSName:   nil,
			expectedMachinesToMove: nil,
		},
		{
			name: "should create machines when too few exists",
			getAndAdoptMachinesForMachineSetSucceeded: true,
			machineSet: newMachineSet("ms1", "cluster1", 5),
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
				fakeMachine("m2"),
			},
			expectMachinesToAdd:    ptr.To(3),
			expectMachinesToDelete: nil,
			expectedTargetMSName:   nil,
			expectedMachinesToMove: nil,
		},
		{
			name: "should delete machines when too many exists and MS is not instructed to move to another MachineSet",
			getAndAdoptMachinesForMachineSetSucceeded: true,
			machineSet: newMachineSet("ms1", "cluster1", 2),
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
				fakeMachine("m2"),
				fakeMachine("m3"),
				fakeMachine("m4"),
			},
			expectMachinesToAdd:    nil,
			expectMachinesToDelete: ptr.To(2),
			expectedTargetMSName:   nil,
			expectedMachinesToMove: nil,
		},
		{
			name: "should move machines when too many exists and MS is instructed to move to another MachineSet",
			getAndAdoptMachinesForMachineSetSucceeded: true,
			machineSet: newMachineSet("ms1", "cluster1", 2, withMachineSetAnnotations(map[string]string{clusterv1.MachineSetMoveMachinesToMachineSetAnnotation: "ms2"})),
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
				fakeMachine("m2"),
				fakeMachine("m3"),
				fakeMachine("m4"),
			},
			expectMachinesToAdd:    nil,
			expectMachinesToDelete: nil,
			expectedTargetMSName:   ptr.To("ms2"),
			expectedMachinesToMove: ptr.To(2),
		},
		{
			name: "When the Machine set is receiving machines from other MachineSets, delete machines should delete only exceeding machines left after taking into account pending machines",
			getAndAdoptMachinesForMachineSetSucceeded: true,
			machineSet: newMachineSet("ms2", "cluster1", 2, withMachineSetAnnotations(map[string]string{clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation: "ms1"})),
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
				fakeMachine("m2"),
				fakeMachine("m3"),
				fakeMachine("m4", withMachineAnnotations(map[string]string{clusterv1.PendingAcknowledgeMoveAnnotation: ""})),
			},
			expectMachinesToAdd:    nil,
			expectMachinesToDelete: ptr.To(1),
			expectedTargetMSName:   nil,
			expectedMachinesToMove: nil,
		},
		{
			name: "When the Machine set is receiving machines from other MachineSets, should not delete if there are no exceeding machines left after taking into account pending machines",
			getAndAdoptMachinesForMachineSetSucceeded: true,
			machineSet: newMachineSet("ms2", "cluster1", 2, withMachineSetAnnotations(map[string]string{clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation: "ms1"})),
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
				fakeMachine("m2"),
				fakeMachine("m3", withMachineAnnotations(map[string]string{clusterv1.PendingAcknowledgeMoveAnnotation: ""})),
			},
			expectMachinesToAdd:    nil,
			expectMachinesToDelete: nil,
			expectedTargetMSName:   nil,
			expectedMachinesToMove: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			s := &scope{
				machineSet: tt.machineSet,
				machines:   tt.machines,
				getAndAdoptMachinesForMachineSetSucceeded: tt.getAndAdoptMachinesForMachineSetSucceeded,
			}

			r := &Reconciler{
				overrideCreateMachines: func(_ context.Context, _ *scope, machinesToAdd int) (ctrl.Result, error) {
					g.Expect(tt.expectMachinesToAdd).ToNot(BeNil(), "unexpected call to create machines")
					g.Expect(machinesToAdd).To(Equal(*tt.expectMachinesToAdd), "call to create machines does not have the expected machinesToAdd number")
					return ctrl.Result{}, nil
				},
				overrideMoveMachines: func(_ context.Context, _ *scope, targetMSName string, machinesToMove int) (ctrl.Result, error) {
					g.Expect(tt.expectedMachinesToMove).ToNot(BeNil(), "unexpected call to move machines")
					g.Expect(tt.expectedTargetMSName).ToNot(BeNil(), "unexpected call to move machines")
					g.Expect(targetMSName).To(Equal(*tt.expectedTargetMSName), "call to move machines does not have the expected targetMS name")
					g.Expect(machinesToMove).To(Equal(*tt.expectedMachinesToMove), "call to move machines does not have the expected machinesToMove number")
					return ctrl.Result{}, nil
				},
				overrideDeleteMachines: func(_ context.Context, _ *scope, machinesToDelete int) (ctrl.Result, error) {
					g.Expect(tt.expectMachinesToDelete).ToNot(BeNil(), "unexpected call to delete machines")
					g.Expect(machinesToDelete).To(Equal(*tt.expectMachinesToDelete), "call to delete machines does not have the expected machinesToDelete number")
					return ctrl.Result{}, nil
				},
			}
			res, err := r.syncReplicas(ctx, s)
			g.Expect(err).ToNot(HaveOccurred(), "unexpected error when syncing replicas")
			g.Expect(res.IsZero()).To(BeTrue(), "unexpected non zero result when syncing replicas")
		})
	}
}

func TestMachineSetReconciler_createMachines_preflightChecks(t *testing.T) {
	// This test is not included in the table test for createMachines because it requires a specific setup.
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
			ControlPlaneRef: contract.ObjToContractVersionedObjectReference(controlPlaneUpgrading),
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

	fakeClient := fake.NewClientBuilder().WithObjects(controlPlaneUpgrading, builder.GenericControlPlaneCRD, machineSet).WithStatusSubresource(&clusterv1.MachineSet{}).Build()
	r := &Reconciler{
		Client:          fakeClient,
		PreflightChecks: sets.Set[clusterv1.MachineSetPreflightCheck]{}.Insert(clusterv1.MachineSetPreflightCheckAll),
	}
	s := &scope{
		cluster:    cluster,
		machineSet: machineSet,
		machines:   []*clusterv1.Machine{},
		getAndAdoptMachinesForMachineSetSucceeded: true,
	}
	result, err := r.createMachines(ctx, s, 3)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.IsZero()).To(BeFalse(), "syncReplicas should not return a 'zero' result")

	// Verify the proper condition is set on the MachineSet.
	condition := clusterv1.MachinesCreatedV1Beta1Condition
	g.Expect(v1beta1conditions.Has(machineSet, condition)).
		To(BeTrue(), "MachineSet should have the %s condition set", condition)
	machinesCreatedCondition := v1beta1conditions.Get(machineSet, condition)
	g.Expect(machinesCreatedCondition.Status).
		To(Equal(corev1.ConditionFalse), "%s condition status should be %s", condition, corev1.ConditionFalse)
	g.Expect(machinesCreatedCondition.Reason).
		To(Equal(clusterv1.PreflightCheckFailedV1Beta1Reason), "%s condition reason should be %s", condition, clusterv1.PreflightCheckFailedV1Beta1Reason)

	// Verify no new Machines are created.
	machineList := &clusterv1.MachineList{}
	g.Expect(r.Client.List(ctx, machineList)).To(Succeed())
	g.Expect(machineList.Items).To(BeEmpty(), "There should not be any machines")
}

func TestMachineSetReconciler_createMachines(t *testing.T) {
	machineSet := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "machineset1",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachineSetSpec{
			Replicas:    ptr.To[int32](1),
			ClusterName: "test-cluster",
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					ClusterName: "test-cluster",
					Version:     "v1.14.2",
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: clusterv1.GroupVersionBootstrap.Group,
							Kind:     builder.GenericBootstrapConfigTemplateKind,
							Name:     "ms-template",
						},
					},
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: clusterv1.GroupVersionInfrastructure.Group,
						Kind:     builder.GenericInfrastructureMachineTemplateKind,
						Name:     "ms-template",
					},
				},
			},
		},
	}

	// Create bootstrap template resource.
	bootstrapResource := map[string]interface{}{
		"kind":       builder.GenericBootstrapConfigKind,
		"apiVersion": clusterv1.GroupVersionBootstrap.String(),
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
	bootstrapTmpl.SetKind(builder.GenericBootstrapConfigTemplateKind)
	bootstrapTmpl.SetAPIVersion(clusterv1.GroupVersionBootstrap.String())
	bootstrapTmpl.SetName("ms-template")
	bootstrapTmpl.SetNamespace(metav1.NamespaceDefault)

	// Create infrastructure template resource.
	infraResource := map[string]interface{}{
		"kind":       builder.GenericInfrastructureMachineKind,
		"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
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
	infraTmpl.SetKind(builder.GenericInfrastructureMachineTemplateKind)
	infraTmpl.SetAPIVersion(clusterv1.GroupVersionInfrastructure.String())
	infraTmpl.SetName("ms-template")
	infraTmpl.SetNamespace(metav1.NamespaceDefault)

	tests := []struct {
		name             string
		machinesToAdd    int
		interceptorFuncs func(i *int) interceptor.Funcs
		wantMachines     int
		wantErr          bool
		wantErrorMessage string
	}{
		{
			name:             "should create machines",
			machinesToAdd:    4,
			interceptorFuncs: func(_ *int) interceptor.Funcs { return interceptor.Funcs{} },
			wantMachines:     4,
			wantErr:          false,
		},
		{
			name:          "should stop creating machines when there are failures and rollback partial changes",
			machinesToAdd: 4,
			interceptorFuncs: func(i *int) interceptor.Funcs {
				return interceptor.Funcs{
					Apply: func(ctx context.Context, c client.WithWatch, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
						clientObject, ok := obj.(client.Object)
						if !ok {
							return errors.Errorf("error during object creation: unexpected ApplyConfiguration")
						}
						if clientObject.GetObjectKind().GroupVersionKind().Kind == "Machine" {
							*i++
							if *i == 2 { // Note: fail for the second machine only (there should not be call for following machines)
								return fmt.Errorf("inject error for create Machine")
							}
						}
						return c.Apply(ctx, obj, opts...)
					},
				}
			},
			wantMachines: 1,
			wantErr:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			scheme := runtime.NewScheme()
			g.Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())
			g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

			i := 0
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
				builder.GenericBootstrapConfigTemplateCRD,
				builder.GenericInfrastructureMachineTemplateCRD,
				machineSet,
				bootstrapTmpl,
				infraTmpl,
			).WithInterceptorFuncs(tt.interceptorFuncs(&i)).Build()

			// TODO(controller-runtime-0.23): This workaround is needed because controller-runtime v0.22 does not set resourceVersion correctly with SSA (fixed with v0.23).
			fakeClient = interceptor.NewClient(fakeClient, interceptor.Funcs{
				Apply: func(ctx context.Context, c client.WithWatch, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
					clientObject, ok := obj.(client.Object)
					if !ok {
						return errors.Errorf("error during object creation: unexpected ApplyConfiguration")
					}
					clientObject.SetResourceVersion("1")
					return c.Apply(ctx, obj, opts...)
				},
			})

			r := &Reconciler{
				Client:   fakeClient,
				recorder: record.NewFakeRecorder(32),
				// Note: This field is only used for unit tests that use fake client because the fake client does not properly set resourceVersion
				//       on BootstrapConfig/InfraMachine after ssa.Patch and then ssa.RemoveManagedFieldsForLabelsAndAnnotations would fail.
				disableRemoveManagedFieldsForLabelsAndAnnotations: true,
			}
			s := &scope{
				machineSet: machineSet,
				machines:   []*clusterv1.Machine{},
				getAndAdoptMachinesForMachineSetSucceeded: true,
			}
			res, err := r.createMachines(ctx, s, tt.machinesToAdd)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred(), "expected error when creating machines, got none")
			} else {
				g.Expect(err).ToNot(HaveOccurred(), "unexpected error when creating machines")
			}
			g.Expect(res.IsZero()).To(BeTrue(), "unexpected non zero result when creating machines")

			// Verify new Machines are created.
			machineList := &clusterv1.MachineList{}
			g.Expect(r.Client.List(ctx, machineList)).To(Succeed())
			g.Expect(machineList.Items).To(HaveLen(tt.wantMachines), "Unexpected machine")

			for _, machine := range machineList.Items {
				// Verify boostrap object created
				bootstrap := &unstructured.Unstructured{}
				bootstrap.SetKind(machine.Spec.Bootstrap.ConfigRef.Kind)
				bootstrap.SetAPIVersion(clusterv1.GroupVersionBootstrap.String())
				bootstrap.SetNamespace(machine.Namespace)
				bootstrap.SetName(machine.Spec.Bootstrap.ConfigRef.Name)
				g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(bootstrap), bootstrap)).To(Succeed(), "failed to get bootstrap object")

				// Verify infra object created
				infra := &unstructured.Unstructured{}
				infra.SetKind(machine.Spec.InfrastructureRef.Kind)
				infra.SetAPIVersion(clusterv1.GroupVersionInfrastructure.String())
				infra.SetNamespace(machine.Namespace)
				infra.SetName(machine.Spec.InfrastructureRef.Name)
				g.Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(infra), infra)).To(Succeed(), "failed to get infra object")
			}

			// Verify no additional boostrap object created
			bootstrapList := &unstructured.UnstructuredList{}
			bootstrapList.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   bootstrapTmpl.GetObjectKind().GroupVersionKind().Group,
				Version: bootstrapTmpl.GetObjectKind().GroupVersionKind().Version,
				Kind:    strings.TrimSuffix(bootstrapTmpl.GetObjectKind().GroupVersionKind().Kind, clusterv1.TemplateSuffix),
			})
			g.Expect(r.Client.List(ctx, bootstrapList)).To(Succeed())
			g.Expect(bootstrapList.Items).To(HaveLen(tt.wantMachines), "Unexpected bootstrap objects")

			// Verify no additional  infra object created
			infraList := &unstructured.UnstructuredList{}
			infraList.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   infraTmpl.GetObjectKind().GroupVersionKind().Group,
				Version: infraTmpl.GetObjectKind().GroupVersionKind().Version,
				Kind:    strings.TrimSuffix(infraTmpl.GetObjectKind().GroupVersionKind().Kind, clusterv1.TemplateSuffix),
			})
			g.Expect(r.Client.List(ctx, infraList)).To(Succeed())
			g.Expect(infraList.Items).To(HaveLen(tt.wantMachines), "Unexpected infra objects")
		})
	}
}

func TestMachineSetReconciler_deleteMachines(t *testing.T) {
	tests := []struct {
		name             string
		ms               *clusterv1.MachineSet
		machines         []*clusterv1.Machine
		machinesToDelete int
		interceptorFuncs interceptor.Funcs
		wantMachines     []string
		wantErr          bool
		wantErrorMessage string
	}{
		{
			name: "should delete machines using the given deletion order",
			ms:   newMachineSet("ms1", "cluster1", 2, withDeletionOrder(clusterv1.NewestMachineSetDeletionOrder)),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-4*time.Minute)), withHealthyNode()), // oldest
				fakeMachine("m2", withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-3*time.Minute)), withHealthyNode()),
				fakeMachine("m3", withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-2*time.Minute)), withHealthyNode()),
				fakeMachine("m4", withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-1*time.Minute)), withHealthyNode()), // newest
			},
			machinesToDelete: 2,
			interceptorFuncs: interceptor.Funcs{},
			wantMachines:     []string{"m1", "m2"}, // m3 and m4 deleted because they are newest and deletion order is NewestMachineSetDeletionOrder
			wantErr:          false,
		},
		{
			name: "should not delete more machines when enough machines are already deleting",
			ms:   newMachineSet("ms1", "cluster1", 2, withDeletionOrder(clusterv1.NewestMachineSetDeletionOrder)),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-4*time.Minute)), withHealthyNode()), // oldest
				fakeMachine("m2", withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-3*time.Minute)), withHealthyNode(), withDeletionTimestamp()),
				fakeMachine("m3", withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-2*time.Minute)), withHealthyNode()),
				fakeMachine("m4", withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-1*time.Minute)), withHealthyNode(), withDeletionTimestamp()), // newest
			},
			machinesToDelete: 2,
			interceptorFuncs: interceptor.Funcs{},
			wantMachines:     []string{"m1", "m3"}, // m2 and m4 already deleted, no additional machines deleted
			wantErr:          false,
		},
		{
			name: "should delete machines when not enough machines are already deleting",
			ms:   newMachineSet("ms1", "cluster1", 2, withDeletionOrder(clusterv1.NewestMachineSetDeletionOrder)),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-4*time.Minute)), withHealthyNode()), // oldest
				fakeMachine("m2", withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-3*time.Minute)), withHealthyNode()),
				fakeMachine("m3", withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-2*time.Minute)), withHealthyNode()),
				fakeMachine("m4", withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-1*time.Minute)), withHealthyNode(), withDeletionTimestamp()), // newest
			},
			machinesToDelete: 2,
			interceptorFuncs: interceptor.Funcs{},
			wantMachines:     []string{"m1", "m2"}, // m3 deleted, m4 already deleted
			wantErr:          false,
		},
		{
			name: "should keep deleting machines when one deletion fails",
			ms:   newMachineSet("ms1", "cluster1", 2), // use default deletion order, oldest machine deleted first
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-4*time.Minute)), withHealthyNode()), // oldest
				fakeMachine("m2", withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-3*time.Minute)), withHealthyNode()),
				fakeMachine("m3", withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-2*time.Minute)), withHealthyNode()),
				fakeMachine("m4", withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-1*time.Minute)), withHealthyNode()), // newest
			},
			machinesToDelete: 2,
			interceptorFuncs: interceptor.Funcs{
				Delete: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					if obj.GetName() == "m1" {
						return fmt.Errorf("error when deleting m1")
					}
					return client.Delete(ctx, obj, opts...)
				},
			},
			wantMachines:     []string{"m1", "m3", "m4"}, // m1 and m2 deleted, but m1 failed to delete
			wantErr:          true,
			wantErrorMessage: "error when deleting m1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			objs := []client.Object{tt.ms}
			for _, m := range tt.machines {
				objs = append(objs, m)
			}
			fakeClient := fake.NewClientBuilder().WithObjects(objs...).WithInterceptorFuncs(tt.interceptorFuncs).Build()
			r := &Reconciler{
				Client:   fakeClient,
				recorder: record.NewFakeRecorder(32),
			}
			s := &scope{
				machineSet: tt.ms,
				machines:   tt.machines,
			}
			res, err := r.deleteMachines(ctx, s, tt.machinesToDelete)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred(), "expected error when deleting machines, got none")
				g.Expect(err.Error()).To(ContainSubstring(tt.wantErrorMessage))
			} else {
				g.Expect(err).ToNot(HaveOccurred(), "unexpected error when deleting machines")
			}
			g.Expect(res.IsZero()).To(BeTrue(), "unexpected non zero result when deleting machines")

			machines := &clusterv1.MachineList{}
			g.Expect(fakeClient.List(ctx, machines)).ToNot(HaveOccurred(), "unexpected error when listing machines")

			machineNames := []string{}
			for i := range machines.Items {
				if machines.Items[i].DeletionTimestamp.IsZero() {
					machineNames = append(machineNames, machines.Items[i].Name)
				}
			}
			g.Expect(machineNames).To(ConsistOf(tt.wantMachines), "Unexpected machine")
		})
	}
}

func TestMachineSetReconciler_startMoveMachines(t *testing.T) {
	machinesByMachineSet := func(machines *clusterv1.MachineList, ms *clusterv1.MachineSet) []string {
		msMachines := []string{}
		for i := range machines.Items {
			// Note: Checking both ownerReferences and unique label
			if len(machines.Items[i].OwnerReferences) == 1 &&
				machines.Items[i].OwnerReferences[0].Kind == "MachineSet" &&
				machines.Items[i].OwnerReferences[0].Name == ms.Name &&
				machines.Items[i].Labels[clusterv1.MachineDeploymentUniqueLabel] == ms.Labels[clusterv1.MachineDeploymentUniqueLabel] {
				msMachines = append(msMachines, machines.Items[i].Name)
			}
		}
		sort.Strings(msMachines)
		return msMachines
	}

	tests := []struct {
		name                 string
		ms                   *clusterv1.MachineSet
		targetMS             *clusterv1.MachineSet
		machines             []*clusterv1.Machine
		machinesToMove       int
		interceptorFuncs     interceptor.Funcs
		wantMachinesNotMoved []string
		wantMovedMachines    []string
		wantErr              bool
		wantErrorMessage     string
	}{
		{
			name: "should fail when taget ms cannot be found",
			ms: newMachineSet("ms1", "cluster1", 2,
				withMachineSetLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}),
				withMachineSetAnnotations(map[string]string{clusterv1.MachineSetMoveMachinesToMachineSetAnnotation: "ms2"}),
			),
			targetMS: newMachineSet("do-not-exist", "cluster1", 2),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withOwnerMachineSet("ms1"), withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-4*time.Minute)), withHealthyNode()), // oldest
				fakeMachine("m2", withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withOwnerMachineSet("ms1"), withCreationTimestamp(time.Now().Add(-1*time.Minute)), withHealthyNode()),
			},
			machinesToMove:       1,
			interceptorFuncs:     interceptor.Funcs{},
			wantMachinesNotMoved: []string{"m1", "m2"},
			wantMovedMachines:    nil,
			wantErr:              true,
			wantErrorMessage:     "failed to get MachineSet",
		},
		{
			name: "should fail when current and taget ms disagree on the move operation",
			ms: newMachineSet("ms1", "cluster1", 2,
				withMachineSetLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}),
				withMachineSetAnnotations(map[string]string{clusterv1.MachineSetMoveMachinesToMachineSetAnnotation: "ms2"}),
			),
			targetMS: newMachineSet("ms2", "cluster1", 2,
				withMachineSetLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "456"}),
				withMachineSetAnnotations(map[string]string{clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation: "ms3,ms4"}),
			),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withOwnerMachineSet("ms1"), withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-4*time.Minute)), withHealthyNode()), // oldest
				fakeMachine("m2", withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withOwnerMachineSet("ms1"), withCreationTimestamp(time.Now().Add(-1*time.Minute)), withHealthyNode()),
			},
			machinesToMove:       1,
			interceptorFuncs:     interceptor.Funcs{},
			wantMachinesNotMoved: []string{"m1", "m2"},
			wantMovedMachines:    nil,
			wantErr:              true,
			wantErrorMessage:     "MachineSet ms1 is set to move replicas to ms2, but ms2 only accepts Machines from ms3,ms4",
		},
		{
			name: "should fail when target MS doesn't have a unique label",
			ms: newMachineSet("ms1", "cluster1", 2,
				withMachineSetLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}),
				withMachineSetAnnotations(map[string]string{clusterv1.MachineSetMoveMachinesToMachineSetAnnotation: "ms2"}),
			),
			targetMS: newMachineSet("ms2", "cluster1", 2,
				// unique label missing
				withMachineSetAnnotations(map[string]string{clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation: "ms1,ms3"}),
			),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withOwnerMachineSet("ms1"), withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-4*time.Minute)), withHealthyNode()), // oldest
				fakeMachine("m2", withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withOwnerMachineSet("ms1"), withCreationTimestamp(time.Now().Add(-1*time.Minute)), withHealthyNode()),
			},
			machinesToMove:       1,
			interceptorFuncs:     interceptor.Funcs{},
			wantMachinesNotMoved: []string{"m1", "m2"},
			wantMovedMachines:    nil,
			wantErr:              true,
			wantErrorMessage:     fmt.Sprintf("MachineSet ms2 does not have the %s label", clusterv1.MachineDeploymentUniqueLabel),
		},
		{
			name: "should move machines",
			ms: newMachineSet("ms1", "cluster1", 2,
				withDeletionOrder(clusterv1.NewestMachineSetDeletionOrder),
				withMachineSetLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}),
				withMachineSetAnnotations(map[string]string{clusterv1.MachineSetMoveMachinesToMachineSetAnnotation: "ms2"}),
			),
			targetMS: newMachineSet("ms2", "cluster1", 2,
				withMachineSetLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "456"}),
				withMachineSetAnnotations(map[string]string{clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation: "ms1,ms3"}),
			),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withOwnerMachineSet("ms1"), withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-4*time.Minute)), withHealthyNode()), // oldest
				fakeMachine("m2", withOwnerMachineSet("ms1"), withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-3*time.Minute)), withHealthyNode()),
				fakeMachine("m3", withOwnerMachineSet("ms1"), withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-2*time.Minute)), withHealthyNode()),
				fakeMachine("m4", withOwnerMachineSet("ms1"), withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-1*time.Minute)), withHealthyNode()), // newest
			},
			machinesToMove:       2,
			interceptorFuncs:     interceptor.Funcs{},
			wantMachinesNotMoved: []string{"m1", "m2"},
			wantMovedMachines:    []string{"m3", "m4"}, // newest machines moved first with NewestMachineSetDeletionOrder
			wantErr:              false,
		},
		{
			name: "should not move deleting machines, decrease the move count",
			ms: newMachineSet("ms1", "cluster1", 2,
				withDeletionOrder(clusterv1.NewestMachineSetDeletionOrder),
				withMachineSetLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}),
				withMachineSetAnnotations(map[string]string{clusterv1.MachineSetMoveMachinesToMachineSetAnnotation: "ms2"}),
			),
			targetMS: newMachineSet("ms2", "cluster1", 2,
				withMachineSetLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "456"}),
				withMachineSetAnnotations(map[string]string{clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation: "ms1,ms3"}),
			),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withOwnerMachineSet("ms1"), withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-4*time.Minute)), withHealthyNode()), // oldest
				fakeMachine("m2", withOwnerMachineSet("ms1"), withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-3*time.Minute)), withHealthyNode()),
				fakeMachine("m3", withOwnerMachineSet("ms1"), withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-2*time.Minute)), withHealthyNode()),
				fakeMachine("m4", withOwnerMachineSet("ms1"), withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-1*time.Minute)), withHealthyNode(), withDeletionTimestamp()), // newest
			},
			machinesToMove:       2,
			interceptorFuncs:     interceptor.Funcs{},
			wantMachinesNotMoved: []string{"m1", "m2", "m4"},
			wantMovedMachines:    []string{"m3"}, // newest machines moved first with NewestMachineSetDeletionOrder, but m4 is deleting so don't touch it
			wantErr:              false,
		},
		{
			name: "should not move more machines that are already updating in place, pick another machine instead",
			ms: newMachineSet("ms1", "cluster1", 2,
				// use default deletion order, oldest machine move first
				withMachineSetLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}),
				withMachineSetAnnotations(map[string]string{clusterv1.MachineSetMoveMachinesToMachineSetAnnotation: "ms2"}),
			),
			targetMS: newMachineSet("ms2", "cluster1", 2,
				withMachineSetLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "456"}),
				withMachineSetAnnotations(map[string]string{clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation: "ms1,ms3"}),
			),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withOwnerMachineSet("ms1"), withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-4*time.Minute)), withHealthyNode()), // oldest
				fakeMachine("m2", withOwnerMachineSet("ms1"), withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-3*time.Minute)), withHealthyNode(), withMachineAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: ""})),
				fakeMachine("m3", withOwnerMachineSet("ms1"), withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-2*time.Minute)), withHealthyNode(), withMachineAnnotations(map[string]string{runtimev1.PendingHooksAnnotation: "UpdateMachine"})),
				fakeMachine("m4", withOwnerMachineSet("ms1"), withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-1*time.Minute)), withHealthyNode()), // newest
			},
			machinesToMove:       2,
			interceptorFuncs:     interceptor.Funcs{},
			wantMachinesNotMoved: []string{"m2", "m3"},
			wantMovedMachines:    []string{"m1", "m4"}, // oldest machines moved first with the default deletion order, but m2 and m3 are updating in place so don't touch them but pick other machines instead
			wantErr:              false,
		},
		{
			name: "should keep moving machines when one move fails",
			ms: newMachineSet("ms1", "cluster1", 2,
				// use default deletion order, oldest machine move first
				withMachineSetLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}),
				withMachineSetAnnotations(map[string]string{clusterv1.MachineSetMoveMachinesToMachineSetAnnotation: "ms2"}),
			),
			targetMS: newMachineSet("ms2", "cluster1", 2,
				withMachineSetLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "456"}),
				withMachineSetAnnotations(map[string]string{clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation: "ms1,ms3"}),
			),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withOwnerMachineSet("ms1"), withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-4*time.Minute)), withHealthyNode()), // oldest
				fakeMachine("m2", withOwnerMachineSet("ms1"), withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-3*time.Minute)), withHealthyNode()),
				fakeMachine("m3", withOwnerMachineSet("ms1"), withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-2*time.Minute)), withHealthyNode()),
				fakeMachine("m4", withOwnerMachineSet("ms1"), withMachineLabels(map[string]string{clusterv1.MachineDeploymentUniqueLabel: "123"}), withMachineFinalizer(), withCreationTimestamp(time.Now().Add(-1*time.Minute)), withHealthyNode()), // newest
			},
			machinesToMove: 2,
			interceptorFuncs: interceptor.Funcs{
				Patch: func(ctx context.Context, client client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					if obj.GetName() == "m1" {
						return fmt.Errorf("error when moving m1")
					}
					return client.Patch(ctx, obj, patch, opts...)
				},
			},
			wantMachinesNotMoved: []string{"m1", "m3", "m4"},
			wantMovedMachines:    []string{"m2"}, // oldest machines moved first with the default deletion order, but m1 failed to move
			wantErr:              true,
			wantErrorMessage:     "error when moving m1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			objs := []client.Object{tt.ms}
			if tt.targetMS.Name != "do-not-exist" {
				objs = append(objs, tt.targetMS)
			}
			for _, m := range tt.machines {
				objs = append(objs, m)
			}
			fakeClient := fake.NewClientBuilder().WithObjects(objs...).WithInterceptorFuncs(tt.interceptorFuncs).Build()
			r := &Reconciler{
				Client:   fakeClient,
				recorder: record.NewFakeRecorder(32),
			}
			s := &scope{
				machineSet: tt.ms,
				machines:   tt.machines,
			}
			res, err := r.startMoveMachines(ctx, s, tt.targetMS.Name, tt.machinesToMove)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred(), "expected error when moving machines, got none")
				g.Expect(err.Error()).To(ContainSubstring(tt.wantErrorMessage))
			} else {
				g.Expect(err).ToNot(HaveOccurred(), "unexpected error when moving machines")
			}
			g.Expect(res.IsZero()).To(BeTrue(), "unexpected non zero result when moving machines")

			machines := &clusterv1.MachineList{}
			g.Expect(fakeClient.List(ctx, machines)).ToNot(HaveOccurred(), "unexpected error when listing machines")
			g.Expect(machinesByMachineSet(machines, tt.ms)).To(ConsistOf(tt.wantMachinesNotMoved))

			movedMachines := machinesByMachineSet(machines, tt.targetMS)
			g.Expect(movedMachines).To(ConsistOf(tt.wantMovedMachines))
			for _, name := range movedMachines {
				for _, m := range machines.Items {
					if m.Name == name {
						g.Expect(m.Annotations).To(HaveKeyWithValue(clusterv1.UpdateInProgressAnnotation, ""))
						g.Expect(m.Annotations).To(HaveKeyWithValue(clusterv1.PendingAcknowledgeMoveAnnotation, ""))
					}
				}
			}
		})
	}
}

func TestMachineSetReconciler_triggerInPlaceUpdate(t *testing.T) {
	machinesNotInPlaceUpdating := func(machines *clusterv1.MachineList) []string {
		msMachines := []string{}
		for i := range machines.Items {
			if machines.Items[i].Annotations[runtimev1.PendingHooksAnnotation] != "UpdateMachine" {
				msMachines = append(msMachines, machines.Items[i].Name)
			}
		}
		sort.Strings(msMachines)
		return msMachines
	}
	machinesInPlaceUpdating := func(machines *clusterv1.MachineList) []string {
		msMachines := []string{}
		for i := range machines.Items {
			if machines.Items[i].Annotations[runtimev1.PendingHooksAnnotation] == "UpdateMachine" {
				msMachines = append(msMachines, machines.Items[i].Name)
			}
		}
		sort.Strings(msMachines)
		return msMachines
	}

	// Create bootstrap template resource.
	bootstrapResource := map[string]interface{}{
		"kind":       builder.GenericBootstrapConfigKind,
		"apiVersion": clusterv1.GroupVersionBootstrap.String(),
		"spec": map[string]interface{}{
			"foo": "bar",
		},
	}

	bootstrapTmpl := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"template": bootstrapResource,
			},
		},
	}
	bootstrapTmpl.SetKind(builder.GenericBootstrapConfigTemplateKind)
	bootstrapTmpl.SetAPIVersion(clusterv1.GroupVersionBootstrap.String())
	bootstrapTmpl.SetNamespace(metav1.NamespaceDefault)
	bootstrapTmpl.SetName("ms-bootstrap-template")

	bootstrapObj := &unstructured.Unstructured{
		Object: bootstrapResource["spec"].(map[string]interface{}),
	}
	bootstrapObj.SetKind(builder.GenericBootstrapConfigKind)
	bootstrapObj.SetAPIVersion(clusterv1.GroupVersionBootstrap.String())
	bootstrapObj.SetNamespace(metav1.NamespaceDefault)
	bootstrapObj.SetName("bootstrap")

	// Create infrastructure template resource.
	infraResource := map[string]interface{}{
		"kind":       builder.GenericInfrastructureMachineKind,
		"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
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
	infraTmpl.SetKind(builder.GenericInfrastructureMachineTemplateKind)
	infraTmpl.SetAPIVersion(clusterv1.GroupVersionInfrastructure.String())
	infraTmpl.SetNamespace(metav1.NamespaceDefault)
	infraTmpl.SetName("ms-infra-template")

	infraObj := &unstructured.Unstructured{
		Object: infraResource["spec"].(map[string]interface{}),
	}
	infraObj.SetKind(builder.GenericInfrastructureMachineKind)
	infraObj.SetAPIVersion(clusterv1.GroupVersionInfrastructure.String())
	infraObj.SetNamespace(metav1.NamespaceDefault)
	infraObj.SetName("infra")

	tests := []struct {
		name                           string
		ms                             *clusterv1.MachineSet
		machines                       []*clusterv1.Machine
		noBootStrapConfig              bool
		interceptorFuncs               interceptor.Funcs
		wantMachinesNotInPlaceUpdating []string
		wantMachinesInPlaceUpdating    []string
		wantErr                        bool
		wantErrorMessage               string
	}{
		{
			name: "No op when machines did not start move / do not have the UpdateInProgressAnnotation",
			ms:   newMachineSet("ms1", "cluster1", 2),
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
				fakeMachine("m2"),
			},
			interceptorFuncs:               interceptor.Funcs{},
			wantMachinesNotInPlaceUpdating: []string{"m1", "m2"},
			wantMachinesInPlaceUpdating:    nil,
			wantErr:                        false,
		},
		{
			name: "No op when in place upgrade has been already triggered",
			ms:   newMachineSet("ms1", "cluster1", 2),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: "", runtimev1.PendingHooksAnnotation: "UpdateMachine"})),
				fakeMachine("m2", withMachineAnnotations(map[string]string{runtimev1.PendingHooksAnnotation: "UpdateMachine"})), // updating in place, failed to complete (to remove both UpdateInProgressAnnotation and PendingHooksAnnotation)
			},
			interceptorFuncs: interceptor.Funcs{
				Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
					return errors.New("injected error performing get") // we should not perform any get if in place upgrade has been already triggered
				},
			},
			wantMachinesNotInPlaceUpdating: nil,
			wantMachinesInPlaceUpdating:    []string{"m1", "m2"},
			wantErr:                        false,
		},
		{
			name: "No op when it fails to get InfraMachine",
			ms:   newMachineSet("ms1", "cluster1", 1),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: ""})),
			},
			interceptorFuncs: interceptor.Funcs{
				Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if obj.GetObjectKind().GroupVersionKind().Kind == builder.GenericInfrastructureMachineKind && key.Name == "m1" {
						return errors.New("injected error when getting m1-infra")
					}
					return client.Get(ctx, key, obj, opts...)
				},
			},
			wantMachinesNotInPlaceUpdating: []string{"m1"},
			wantErr:                        true,
			wantErrorMessage:               "injected error when getting m1-infra",
		},
		{
			name: "No op when it fails to compute desired InfraMachine",
			ms:   newMachineSet("ms1", "cluster1", 1),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: ""})),
			},
			interceptorFuncs: interceptor.Funcs{
				Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if key.Name == "ms-infra-template" {
						return errors.New("injected error when getting ms-infra-template")
					}
					return client.Get(ctx, key, obj, opts...)
				},
			},
			wantMachinesNotInPlaceUpdating: []string{"m1"},
			wantErr:                        true,
			wantErrorMessage:               "injected error when getting ms-infra-template",
		},
		{
			name: "No op when it fails to get BootstrapConfig",
			ms:   newMachineSet("ms1", "cluster1", 1),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: ""})),
			},
			interceptorFuncs: interceptor.Funcs{
				Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if obj.GetObjectKind().GroupVersionKind().Kind == builder.GenericBootstrapConfigKind && key.Name == "m1" {
						return errors.New("injected error when getting m1-bootstrap")
					}
					return client.Get(ctx, key, obj, opts...)
				},
			},
			wantMachinesNotInPlaceUpdating: []string{"m1"},
			wantErr:                        true,
			wantErrorMessage:               "injected error when getting m1-bootstrap",
		},
		{
			name: "No op when it fails to compute desired BootstrapConfig",
			ms:   newMachineSet("ms1", "cluster1", 1),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: ""})),
			},
			interceptorFuncs: interceptor.Funcs{
				Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if key.Name == "ms-bootstrap-template" {
						return errors.New("injected error when getting ms-bootstrap-template")
					}
					return client.Get(ctx, key, obj, opts...)
				},
			},
			wantMachinesNotInPlaceUpdating: []string{"m1"},
			wantErr:                        true,
			wantErrorMessage:               "injected error when getting ms-bootstrap-template",
		},
		{
			name: "No op when it fails to update InfraMachine",
			ms:   newMachineSet("ms1", "cluster1", 1),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: ""})),
			},
			interceptorFuncs: interceptor.Funcs{
				Apply: func(ctx context.Context, c client.WithWatch, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
					clientObject, ok := obj.(client.Object)
					if !ok {
						return errors.Errorf("error during object creation: unexpected ApplyConfiguration")
					}
					if clientObject.GetObjectKind().GroupVersionKind().Kind == builder.GenericInfrastructureMachineKind && clientObject.GetName() == "m1" {
						return errors.New("injected error when applying m1-infra")
					}
					return c.Apply(ctx, obj, opts...)
				},
			},
			wantMachinesNotInPlaceUpdating: []string{"m1"},
			wantErr:                        true,
			wantErrorMessage:               "injected error when applying m1-infra",
		},
		{
			name: "No op when it fails to update boostrap config",
			ms:   newMachineSet("ms1", "cluster1", 1),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: ""})),
			},
			interceptorFuncs: interceptor.Funcs{
				Apply: func(ctx context.Context, c client.WithWatch, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
					clientObject, ok := obj.(client.Object)
					if !ok {
						return errors.Errorf("error during object creation: unexpected ApplyConfiguration")
					}
					if clientObject.GetObjectKind().GroupVersionKind().Kind == builder.GenericBootstrapConfigKind && clientObject.GetName() == "m1" {
						return errors.New("injected error when applying m1-bootstrap")
					}
					return c.Apply(ctx, obj, opts...)
				},
			},
			wantMachinesNotInPlaceUpdating: []string{"m1"},
			wantErr:                        true,
			wantErrorMessage:               "injected error when applying m1-bootstrap",
		},
		{
			name: "No op when it fails to update machine",
			ms:   newMachineSet("ms1", "cluster1", 1),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: ""})),
			},
			interceptorFuncs: interceptor.Funcs{
				Apply: func(ctx context.Context, c client.WithWatch, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
					clientObject, ok := obj.(client.Object)
					if !ok {
						return errors.Errorf("error during object creation: unexpected ApplyConfiguration")
					}
					if clientObject.GetName() == "m1" {
						return errors.New("injected error when applying m1")
					}
					return c.Apply(ctx, obj, opts...)
				},
			},
			wantMachinesNotInPlaceUpdating: []string{"m1"},
			wantErr:                        true,
			wantErrorMessage:               "injected error when applying m1",
		},
		{
			name: "No op when ms is accepting moved replicas, machine is still pending acknowledge",
			ms:   newMachineSet("ms1", "cluster1", 1, withMachineSetAnnotations(map[string]string{clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation: "ms2"})),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: "", clusterv1.PendingAcknowledgeMoveAnnotation: ""})),
			},
			interceptorFuncs:               interceptor.Funcs{},
			wantMachinesNotInPlaceUpdating: []string{"m1"},
			wantErr:                        false,
		},
		{
			name: "Trigger in-place when ms is accepting moved replicas, machine is still pending acknowledge, machine is acknowledged",
			ms:   newMachineSet("ms1", "cluster1", 1, withMachineSetAnnotations(map[string]string{clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation: "ms2", clusterv1.AcknowledgedMoveAnnotation: "m1"})),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: "", clusterv1.PendingAcknowledgeMoveAnnotation: ""})),
			},
			interceptorFuncs:            interceptor.Funcs{},
			wantMachinesInPlaceUpdating: []string{"m1"},
			wantErr:                     false,
		},
		{
			name: "Trigger in-place when ms is not accepting anymore moved replicas, machine is still pending acknowledge",
			ms:   newMachineSet("ms1", "cluster1", 1),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: "", clusterv1.PendingAcknowledgeMoveAnnotation: ""})),
			},
			interceptorFuncs:            interceptor.Funcs{},
			wantMachinesInPlaceUpdating: []string{"m1"},
			wantErr:                     false,
		},
		{
			name: "Keeps triggering in-place when one machine fails",
			ms:   newMachineSet("ms1", "cluster1", 1),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: "", clusterv1.PendingAcknowledgeMoveAnnotation: ""})),
				fakeMachine("m2", withMachineAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: "", clusterv1.PendingAcknowledgeMoveAnnotation: ""})),
				fakeMachine("m3", withMachineAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: "", clusterv1.PendingAcknowledgeMoveAnnotation: ""})),
			},
			interceptorFuncs: interceptor.Funcs{
				Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if obj.GetObjectKind().GroupVersionKind().Kind == builder.GenericInfrastructureMachineKind && key.Name == "m1" {
						return errors.New("injected error when getting m1-infra")
					}
					return client.Get(ctx, key, obj, opts...)
				},
			},
			wantMachinesNotInPlaceUpdating: []string{"m1"},
			wantMachinesInPlaceUpdating:    []string{"m2", "m3"},
			wantErr:                        true,
			wantErrorMessage:               "injected error when getting m1-infra",
		},
		{
			name: "Trigger in-place for machines without bootstrap config",
			ms:   newMachineSet("ms1", "cluster1", 1),
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineAnnotations(map[string]string{clusterv1.UpdateInProgressAnnotation: "", clusterv1.PendingAcknowledgeMoveAnnotation: ""})),
			},
			noBootStrapConfig:           true,
			interceptorFuncs:            interceptor.Funcs{},
			wantMachinesInPlaceUpdating: []string{"m1"},
			wantErr:                     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			tt.ms.Spec.Template.Spec.InfrastructureRef = clusterv1.ContractVersionedObjectReference{
				APIGroup: infraTmpl.GroupVersionKind().Group,
				Kind:     infraTmpl.GetKind(),
				Name:     infraTmpl.GetName(),
			}

			objs := []client.Object{
				builder.GenericInfrastructureMachineTemplateCRD.DeepCopy(),
				builder.GenericInfrastructureMachineCRD.DeepCopy(),
				builder.GenericBootstrapConfigTemplateCRD.DeepCopy(),
				builder.GenericBootstrapConfigCRD.DeepCopy(),
				tt.ms,
				infraTmpl,
			}
			if !tt.noBootStrapConfig {
				tt.ms.Spec.Template.Spec.Bootstrap.ConfigRef = clusterv1.ContractVersionedObjectReference{
					APIGroup: bootstrapTmpl.GroupVersionKind().Group,
					Kind:     bootstrapTmpl.GetKind(),
					Name:     bootstrapTmpl.GetName(),
				}
				objs = append(objs, bootstrapTmpl)
			}

			for _, m := range tt.machines {
				m.SetNamespace(tt.ms.Namespace)

				mInfraObj := infraObj.DeepCopy()
				mInfraObj.SetName(m.Name)
				m.Spec.InfrastructureRef = clusterv1.ContractVersionedObjectReference{
					APIGroup: mInfraObj.GroupVersionKind().Group,
					Kind:     mInfraObj.GetKind(),
					Name:     mInfraObj.GetName(),
				}
				objs = append(objs, m, mInfraObj)

				if !tt.noBootStrapConfig {
					mBootstrapObj := bootstrapObj.DeepCopy()
					mBootstrapObj.SetName(m.Name)
					m.Spec.Bootstrap.ConfigRef = clusterv1.ContractVersionedObjectReference{
						APIGroup: mBootstrapObj.GroupVersionKind().Group,
						Kind:     mBootstrapObj.GetKind(),
						Name:     mBootstrapObj.GetName(),
					}
					objs = append(objs, mBootstrapObj)
				}
			}
			fakeClient := fake.NewClientBuilder().WithObjects(objs...).WithInterceptorFuncs(tt.interceptorFuncs).Build()
			r := &Reconciler{
				Client:   fakeClient,
				recorder: record.NewFakeRecorder(32),
			}
			s := &scope{
				machineSet: tt.ms,
				machines:   tt.machines,
			}
			res, err := r.triggerInPlaceUpdate(ctx, s)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred(), "expected error when triggering in place update, got none")
				g.Expect(err.Error()).To(ContainSubstring(tt.wantErrorMessage))
			} else {
				g.Expect(err).ToNot(HaveOccurred(), "unexpected error when triggering in place update")
			}
			g.Expect(res.IsZero()).To(BeTrue(), "unexpected non zero result when triggering in place update")

			machines := &clusterv1.MachineList{}
			g.Expect(fakeClient.List(ctx, machines)).ToNot(HaveOccurred(), "unexpected error when listing machines")
			g.Expect(machinesNotInPlaceUpdating(machines)).To(ConsistOf(tt.wantMachinesNotInPlaceUpdating))

			updatingMachines := machinesInPlaceUpdating(machines)
			g.Expect(updatingMachines).To(ConsistOf(tt.wantMachinesInPlaceUpdating))
		})
	}
}

func TestComputeDesiredMachine(t *testing.T) {
	duration5s := ptr.To(int32(5))
	duration10s := ptr.To(int32(10))

	namingTemplateKey := "-md"
	mdName := "testmd"
	msName := "ms1"
	testCluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
	}

	infraRef := clusterv1.ContractVersionedObjectReference{
		Kind:     "GenericInfrastructureMachineTemplate",
		Name:     "infra-template-1",
		APIGroup: clusterv1.GroupVersionInfrastructure.Group,
	}
	bootstrapRef := clusterv1.ContractVersionedObjectReference{
		Kind:     "GenericBootstrapConfigTemplate",
		Name:     "bootstrap-template-1",
		APIGroup: clusterv1.GroupVersionBootstrap.Group,
	}

	machineTemplateSpec := clusterv1.MachineTemplateSpec{
		ObjectMeta: clusterv1.ObjectMeta{
			Labels:      map[string]string{"machine-label1": "machine-value1"},
			Annotations: map[string]string{"machine-annotation1": "machine-value1"},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName:       testClusterName,
			Version:           "v1.25.3",
			InfrastructureRef: infraRef,
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: bootstrapRef,
			},
			Deletion: clusterv1.MachineDeletionSpec{
				NodeDrainTimeoutSeconds:        duration10s,
				NodeVolumeDetachTimeoutSeconds: duration10s,
				NodeDeletionTimeoutSeconds:     duration10s,
			},
			MinReadySeconds: ptr.To[int32](10),
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
			ClusterName: testClusterName,
			Version:     "v1.25.3",
			Deletion: clusterv1.MachineDeletionSpec{
				NodeDrainTimeoutSeconds:        duration10s,
				NodeVolumeDetachTimeoutSeconds: duration10s,
				NodeDeletionTimeoutSeconds:     duration10s,
			},
			MinReadySeconds: ptr.To[int32](10),
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
	existingMachine.Spec.InfrastructureRef = clusterv1.ContractVersionedObjectReference{
		Kind:     "GenericInfrastructureMachine",
		Name:     "infra-machine-1",
		APIGroup: clusterv1.GroupVersionInfrastructure.Group,
	}
	existingMachine.Spec.Bootstrap.ConfigRef = clusterv1.ContractVersionedObjectReference{
		Kind:     "GenericBootstrapConfig",
		Name:     "bootstrap-config-1",
		APIGroup: clusterv1.GroupVersionBootstrap.Group,
	}
	existingMachine.Spec.Deletion.NodeDrainTimeoutSeconds = duration5s
	existingMachine.Spec.Deletion.NodeDeletionTimeoutSeconds = duration5s
	existingMachine.Spec.Deletion.NodeVolumeDetachTimeoutSeconds = duration5s
	existingMachine.Spec.MinReadySeconds = ptr.To[int32](5)

	expectedUpdatedMachine := skeletonMachine.DeepCopy()
	expectedUpdatedMachine.Name = existingMachine.Name
	expectedUpdatedMachine.UID = existingMachine.UID
	// Pre-existing finalizer should be preserved.
	expectedUpdatedMachine.Finalizers = []string{"pre-existing-finalizer", clusterv1.MachineFinalizer}
	expectedUpdatedMachine.Spec.InfrastructureRef = *existingMachine.Spec.InfrastructureRef.DeepCopy()
	expectedUpdatedMachine.Spec.Bootstrap.ConfigRef = *existingMachine.Spec.Bootstrap.ConfigRef.DeepCopy()

	tests := []struct {
		name            string
		ms              *clusterv1.MachineSet
		existingMachine *clusterv1.Machine
		wantMachine     *clusterv1.Machine
		wantName        []gomegatypes.GomegaMatcher
	}{
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
					ClusterName: testClusterName,
					Replicas:    ptr.To[int32](3),
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"k1": "v1"},
					},
					MachineNaming: clusterv1.MachineNamingSpec{
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
					ClusterName: testClusterName,
					Replicas:    ptr.To[int32](3),
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"k1": "v1"},
					},
					MachineNaming: clusterv1.MachineNamingSpec{
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
					ClusterName: testClusterName,
					Replicas:    ptr.To[int32](3),
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"k1": "v1"},
					},
					MachineNaming: clusterv1.MachineNamingSpec{
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
					ClusterName: testClusterName,
					Replicas:    ptr.To[int32](3),
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{"k1": "v1"},
					},
					MachineNaming: clusterv1.MachineNamingSpec{
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
					ClusterName: testClusterName,
					Replicas:    ptr.To[int32](3),
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
					ClusterName: testClusterName,
					Replicas:    ptr.To[int32](3),
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
				Conditions: []metav1.Condition{
					{
						Type:    clusterv1.MachineOwnerRemediatedCondition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationReason,
						Message: "Waiting for remediation",
					},
					{
						Type:    clusterv1.MachineHealthCheckSucceededCondition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineHealthCheckHasRemediateAnnotationReason,
						Message: "Marked for remediation via cluster.x-k8s.io/remediate-machine annotation",
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
				Conditions: []metav1.Condition{
					{
						Type:    clusterv1.MachineOwnerRemediatedCondition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineOwnerRemediatedWaitingForRemediationReason,
						Message: "Waiting for remediation",
					},
					{
						Type:    clusterv1.MachineHealthCheckSucceededCondition,
						Status:  metav1.ConditionFalse,
						Reason:  clusterv1.MachineHealthCheckHasRemediateAnnotationReason,
						Message: "Marked for remediation via cluster.x-k8s.io/remediate-machine annotation",
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

func managedFieldsMatching(managedFields []metav1.ManagedFieldsEntry, manager string, operation metav1.ManagedFieldsOperationType, subresource string) []metav1.ManagedFieldsEntry {
	res := []metav1.ManagedFieldsEntry{}
	for _, f := range managedFields {
		if f.Manager == manager && f.Operation == operation && f.Subresource == subresource {
			res = append(res, f)
		}
	}
	return res
}
