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

package machinedeployment

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

const (
	machineDeploymentNamespace = "md-test"
)

var _ reconcile.Reconciler = &Reconciler{}

func TestMachineDeploymentReconciler(t *testing.T) {
	setup := func(t *testing.T, g *WithT) (*corev1.Namespace, *clusterv1.Cluster) {
		t.Helper()

		utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.MachineTaintPropagation, true)

		t.Log("Creating the namespace")
		ns, err := env.CreateNamespace(ctx, machineDeploymentNamespace)
		g.Expect(err).ToNot(HaveOccurred())

		t.Log("Creating the Cluster")
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      "test-cluster",
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

	t.Run("Should reconcile a MachineDeployment", func(t *testing.T) {
		g := NewWithT(t)
		namespace, testCluster := setup(t, g)
		defer teardown(t, g, namespace, testCluster)

		labels := map[string]string{
			"foo":                      "bar",
			clusterv1.ClusterNameLabel: testCluster.Name,
		}
		version := "v1.10.3"
		deployment := &clusterv1.MachineDeployment{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "md-",
				Namespace:    namespace.Name,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: testCluster.Name,
				},
			},
			Spec: clusterv1.MachineDeploymentSpec{
				ClusterName: testCluster.Name,
				Replicas:    ptr.To[int32](2),
				Selector: metav1.LabelSelector{
					// We're using the same labels for spec.selector and spec.template.labels.
					// The labels are later changed and we will use the initial labels later to
					// verify that all original MachineSets have been deleted.
					MatchLabels: labels,
				},
				Rollout: clusterv1.MachineDeploymentRolloutSpec{
					Strategy: clusterv1.MachineDeploymentRolloutStrategy{
						Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
						RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
							MaxUnavailable: intOrStrPtr(0),
							MaxSurge:       intOrStrPtr(1),
						},
					},
				},
				Deletion: clusterv1.MachineDeploymentDeletionSpec{
					Order: clusterv1.OldestMachineSetDeletionOrder,
				},
				Template: clusterv1.MachineTemplateSpec{
					ObjectMeta: clusterv1.ObjectMeta{
						Labels: labels,
					},
					Spec: clusterv1.MachineSpec{
						ClusterName: testCluster.Name,
						Version:     version,
						InfrastructureRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: clusterv1.GroupVersionInfrastructure.Group,
							Kind:     "GenericInfrastructureMachineTemplate",
							Name:     "md-template",
						},
						Bootstrap: clusterv1.Bootstrap{
							DataSecretName: ptr.To("data-secret-name"),
						},
					},
				},
			},
		}
		msListOpts := []client.ListOption{
			client.InNamespace(namespace.Name),
			client.MatchingLabels(labels),
		}

		// Create infrastructure template resource.
		infraResource := map[string]interface{}{
			"kind":       "GenericInfrastructureMachine",
			"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
			"metadata":   map[string]interface{}{},
			"spec": map[string]interface{}{
				"size": "3xlarge",
			},
		}
		infraTmpl := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       "GenericInfrastructureMachineTemplate",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"metadata": map[string]interface{}{
					"name":      "md-template",
					"namespace": namespace.Name,
				},
				"spec": map[string]interface{}{
					"template": infraResource,
				},
			},
		}
		t.Log("Creating the infrastructure template")
		g.Expect(env.Create(ctx, infraTmpl)).To(Succeed())

		// Create the MachineDeployment object and expect Reconcile to be called.
		t.Log("Creating the MachineDeployment")
		g.Expect(env.Create(ctx, deployment)).To(Succeed())
		defer func() {
			t.Log("Deleting the MachineDeployment")
			g.Expect(env.Delete(ctx, deployment)).To(Succeed())
		}()

		t.Log("Verifying the MachineDeployment has a cluster label and ownerRef")
		g.Eventually(func() bool {
			key := client.ObjectKey{Name: deployment.Name, Namespace: deployment.Namespace}
			if err := env.Get(ctx, key, deployment); err != nil {
				return false
			}
			if len(deployment.Labels) == 0 || deployment.Labels[clusterv1.ClusterNameLabel] != testCluster.Name {
				return false
			}
			if len(deployment.OwnerReferences) == 0 || deployment.OwnerReferences[0].Name != testCluster.Name {
				return false
			}
			return true
		}, timeout).Should(BeTrue())

		// Verify that the MachineSet was created.
		t.Log("Verifying the MachineSet was created")
		machineSets := &clusterv1.MachineSetList{}
		g.Eventually(func() int {
			if err := env.List(ctx, machineSets, msListOpts...); err != nil {
				return -1
			}
			return len(machineSets.Items)
		}, timeout).Should(BeEquivalentTo(1))

		//
		// Verify managedField mitigation (purge managedFields and verify they are re-added)
		//
		jsonPatch := []map[string]interface{}{
			{
				"op":    "replace",
				"path":  "/metadata/managedFields",
				"value": []metav1.ManagedFieldsEntry{{}},
			},
		}
		patch, err := json.Marshal(jsonPatch)
		g.Expect(err).ToNot(HaveOccurred())
		ms := machineSets.Items[0].DeepCopy()
		g.Expect(env.Client.Patch(ctx, ms, client.RawPatch(types.JSONPatchType, patch))).To(Succeed())
		g.Expect(ms.GetManagedFields()).To(BeEmpty())
		g.Eventually(func(g Gomega) {
			g.Expect(env.Client.Get(ctx, client.ObjectKeyFromObject(ms), ms)).To(Succeed())
			g.Expect(ms.GetManagedFields()).ToNot(BeEmpty())
		}, timeout).Should(Succeed())

		t.Log("Verifying that the deployment's deletion.order was propagated to the machineset")
		g.Expect(machineSets.Items[0].Spec.Deletion.Order).To(Equal(clusterv1.OldestMachineSetDeletionOrder))

		t.Log("Verifying the linked infrastructure template has a cluster owner reference")
		g.Eventually(func() bool {
			obj, err := external.GetObjectFromContractVersionedRef(ctx, env, deployment.Spec.Template.Spec.InfrastructureRef, deployment.Namespace)
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

		t.Log("Verify MachineSet has expected replicas and version")
		firstMachineSet := machineSets.Items[0]
		g.Expect(*firstMachineSet.Spec.Replicas).To(BeEquivalentTo(2))
		g.Expect(firstMachineSet.Spec.Template.Spec.Version).To(BeEquivalentTo("v1.10.3"))

		t.Log("Verify MachineSet has expected ClusterNameLabel and MachineDeploymentNameLabel")
		g.Expect(firstMachineSet.Labels[clusterv1.ClusterNameLabel]).To(Equal(testCluster.Name))
		g.Expect(firstMachineSet.Labels[clusterv1.MachineDeploymentNameLabel]).To(Equal(deployment.Name))

		t.Log("Verify expected number of Machines are created")
		machines := &clusterv1.MachineList{}
		g.Eventually(func() int {
			if err := env.List(ctx, machines, client.InNamespace(namespace.Name)); err != nil {
				return -1
			}
			return len(machines.Items)
		}, timeout).Should(BeEquivalentTo(*deployment.Spec.Replicas))

		t.Log("Verify Machines have expected ClusterNameLabel, MachineDeploymentNameLabel and MachineSetNameLabel")
		for _, m := range machines.Items {
			g.Expect(m.Labels[clusterv1.ClusterNameLabel]).To(Equal(testCluster.Name))
			g.Expect(m.Labels[clusterv1.MachineDeploymentNameLabel]).To(Equal(deployment.Name))
			g.Expect(m.Labels[clusterv1.MachineSetNameLabel]).To(Equal(firstMachineSet.Name))
		}

		//
		// Delete firstMachineSet and expect Reconcile to be called to replace it.
		//
		t.Log("Deleting the initial MachineSet")
		g.Expect(env.Delete(ctx, &firstMachineSet)).To(Succeed())
		g.Eventually(func() bool {
			if err := env.List(ctx, machineSets, msListOpts...); err != nil {
				return false
			}
			for _, ms := range machineSets.Items {
				if ms.UID == firstMachineSet.UID {
					return false
				}
			}
			return len(machineSets.Items) > 0
		}, timeout).Should(BeTrue())

		//
		// Scale the MachineDeployment and expect Reconcile to be called.
		//
		secondMachineSet := machineSets.Items[0]
		t.Log("Scaling the MachineDeployment to 3 replicas")
		desiredMachineDeploymentReplicas := int32(3)
		modifyFunc := func(d *clusterv1.MachineDeployment) {
			d.Spec.Replicas = ptr.To[int32](desiredMachineDeploymentReplicas)
		}
		g.Expect(updateMachineDeployment(ctx, env, deployment, modifyFunc)).To(Succeed())
		g.Eventually(func() int {
			key := client.ObjectKey{Name: secondMachineSet.Name, Namespace: secondMachineSet.Namespace}
			if err := env.Get(ctx, key, &secondMachineSet); err != nil {
				return -1
			}
			return int(*secondMachineSet.Spec.Replicas)
		}, timeout).Should(BeEquivalentTo(desiredMachineDeploymentReplicas))

		//
		// Update the InfraStructureRef of the MachineDeployment, expect Reconcile to be called and a new MachineSet to appear.
		//

		t.Log("Updating the InfrastructureRef on the MachineDeployment")
		// Create the InfrastructureTemplate
		// Create infrastructure template resource.
		infraTmpl2 := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       "GenericInfrastructureMachineTemplate",
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"metadata": map[string]interface{}{
					"name":      "md-template-2",
					"namespace": namespace.Name,
				},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"kind":       "GenericInfrastructureMachine",
						"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
						"metadata":   map[string]interface{}{},
						"spec": map[string]interface{}{
							"size": "5xlarge",
						},
					},
				},
			},
		}
		t.Log("Creating the infrastructure template")
		g.Expect(env.Create(ctx, infraTmpl2)).To(Succeed())

		infraTmpl2Ref := clusterv1.ContractVersionedObjectReference{
			APIGroup: clusterv1.GroupVersionInfrastructure.Group,
			Kind:     "GenericInfrastructureMachineTemplate",
			Name:     "md-template-2",
		}
		modifyFunc = func(d *clusterv1.MachineDeployment) { d.Spec.Template.Spec.InfrastructureRef = infraTmpl2Ref }
		g.Expect(updateMachineDeployment(ctx, env, deployment, modifyFunc)).To(Succeed())
		g.Eventually(func() int {
			if err := env.List(ctx, machineSets, msListOpts...); err != nil {
				return -1
			}
			return len(machineSets.Items)
		}, timeout).Should(BeEquivalentTo(2))

		// Update the Labels of the MachineDeployment, expect Reconcile to be called and the MachineSet to be updated in-place.
		t.Log("Setting a label on the MachineDeployment")
		modifyFunc = func(d *clusterv1.MachineDeployment) { d.Spec.Template.Labels["updated"] = "true" }
		g.Expect(updateMachineDeployment(ctx, env, deployment, modifyFunc)).To(Succeed())
		g.Eventually(func(g Gomega) {
			g.Expect(env.List(ctx, machineSets, msListOpts...)).To(Succeed())
			// Verify we still only have 2 MachineSets.
			g.Expect(machineSets.Items).To(HaveLen(2))
			// Verify that the new and old MachineSet gets the updated labels.
			g.Expect(machineSets.Items[0].Spec.Template.Labels).To(HaveKeyWithValue("updated", "true"))
			g.Expect(machineSets.Items[1].Spec.Template.Labels).To(HaveKeyWithValue("updated", "true"))
		}, timeout).Should(Succeed())

		// Update the NodeDrainTimout, NodeDeletionTimeoutSeconds, NodeVolumeDetachTimeoutSeconds of the MachineDeployment,
		// expect the Reconcile to be called and the MachineSet to be updated in-place.
		t.Log("Setting NodeDrainTimout, NodeDeletionTimeoutSeconds, NodeVolumeDetachTimeoutSeconds on the MachineDeployment")
		duration10s := int32(10)
		modifyFunc = func(d *clusterv1.MachineDeployment) {
			d.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds = &duration10s
			d.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds = &duration10s
			d.Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds = &duration10s
		}
		g.Expect(updateMachineDeployment(ctx, env, deployment, modifyFunc)).To(Succeed())
		g.Eventually(func(g Gomega) {
			g.Expect(env.List(ctx, machineSets, msListOpts...)).Should(Succeed())
			// Verify we still only have 2 MachineSets.
			g.Expect(machineSets.Items).To(HaveLen(2))
			// Verify the NodeDrainTimeoutSeconds value is updated
			g.Expect(machineSets.Items[0].Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds).Should(And(
				Not(BeNil()),
				HaveValue(Equal(duration10s)),
			), "NodeDrainTimout value does not match expected")
			// Verify the NodeDeletionTimeoutSeconds value is updated
			g.Expect(machineSets.Items[0].Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds).Should(And(
				Not(BeNil()),
				HaveValue(Equal(duration10s)),
			), "NodeDeletionTimeoutSeconds value does not match expected")
			// Verify the NodeVolumeDetachTimeoutSeconds value is updated
			g.Expect(machineSets.Items[0].Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds).Should(And(
				Not(BeNil()),
				HaveValue(Equal(duration10s)),
			), "NodeVolumeDetachTimeoutSeconds value does not match expected")

			// Verify that the old machine set have the new values.
			g.Expect(machineSets.Items[1].Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds).Should(And(
				Not(BeNil()),
				HaveValue(Equal(duration10s)),
			), "NodeDrainTimout value does not match expected")
			g.Expect(machineSets.Items[1].Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds).Should(And(
				Not(BeNil()),
				HaveValue(Equal(duration10s)),
			), "NodeDeletionTimeoutSeconds value does not match expected")
			g.Expect(machineSets.Items[1].Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds).Should(And(
				Not(BeNil()),
				HaveValue(Equal(duration10s)),
			), "NodeVolumeDetachTimeoutSeconds value does not match expected")
		}).Should(Succeed())

		// Update the deletion.order of the MachineDeployment,
		// expect the Reconcile to be called and the MachineSet to be updated in-place.
		t.Log("Updating deletion.order on the MachineDeployment")
		modifyFunc = func(d *clusterv1.MachineDeployment) {
			d.Spec.Deletion.Order = clusterv1.NewestMachineSetDeletionOrder
		}
		g.Expect(updateMachineDeployment(ctx, env, deployment, modifyFunc)).To(Succeed())
		g.Eventually(func(g Gomega) {
			g.Expect(env.List(ctx, machineSets, msListOpts...)).Should(Succeed())
			// Verify we still only have 2 MachineSets.
			g.Expect(machineSets.Items).To(HaveLen(2))
			// Verify the deletion.order value is updated
			g.Expect(machineSets.Items[0].Spec.Deletion.Order).Should(Equal(clusterv1.NewestMachineSetDeletionOrder))

			// Verify that the old machine set have the new values.
			g.Expect(machineSets.Items[1].Spec.Deletion.Order).Should(Equal(clusterv1.NewestMachineSetDeletionOrder))
		}).Should(Succeed())

		// Update the taints of the MachineDeployment,
		// expect the Reconcile to be called and the MachineSet to be updated in-place.
		t.Log("Updating template.spec.taints on the MachineDeployment")
		additionalTaint := clusterv1.MachineTaint{
			Key:         "additional-taint-key",
			Value:       "additional-taint-value",
			Effect:      corev1.TaintEffectNoSchedule,
			Propagation: clusterv1.MachineTaintPropagationAlways,
		}
		modifyFunc = func(d *clusterv1.MachineDeployment) {
			d.Spec.Template.Spec.Taints = append(d.Spec.Template.Spec.Taints, additionalTaint)
		}
		g.Expect(updateMachineDeployment(ctx, env, deployment, modifyFunc)).To(Succeed())
		g.Eventually(func(g Gomega) {
			g.Expect(env.List(ctx, machineSets, msListOpts...)).Should(Succeed())
			// Verify we still only have 2 MachineSets.
			g.Expect(machineSets.Items).To(HaveLen(2))
			// Verify the taints value is updated
			g.Expect(machineSets.Items[0].Spec.Template.Spec.Taints).Should(ContainElement(additionalTaint))

			// Verify that the old machine set has the new taints.
			g.Expect(machineSets.Items[1].Spec.Template.Spec.Taints).Should(ContainElement(additionalTaint))
		}).Should(Succeed())

		// Verify that all the MachineSets have the expected OwnerRef.
		t.Log("Verifying MachineSet owner references")
		g.Eventually(func() bool {
			if err := env.List(ctx, machineSets, msListOpts...); err != nil {
				return false
			}
			for range len(machineSets.Items) {
				ms := machineSets.Items[0]
				if !metav1.IsControlledBy(&ms, deployment) || metav1.GetControllerOf(&ms).Kind != "MachineDeployment" {
					return false
				}
			}
			return true
		}, timeout).Should(BeTrue())

		t.Log("Locating the newest MachineSet")
		var newestMachineSet *clusterv1.MachineSet
		for i := range machineSets.Items {
			ms := &machineSets.Items[i]
			if ms.UID != secondMachineSet.UID {
				newestMachineSet = ms
				break
			}
		}
		g.Expect(newestMachineSet).NotTo(BeNil())

		t.Log("Verifying the initial MachineSet is deleted")
		g.Eventually(func() int {
			// Set the all non-deleted machines as ready with a NodeRef, so the MachineSet controller can proceed
			// to properly set AvailableReplicas.
			foundMachines := &clusterv1.MachineList{}
			g.Expect(env.List(ctx, foundMachines, client.InNamespace(namespace.Name))).To(Succeed())
			for i := range len(foundMachines.Items) {
				m := foundMachines.Items[i]
				// Skip over deleted Machines
				if !m.DeletionTimestamp.IsZero() {
					continue
				}
				// Skip over Machines controlled by other (previous) MachineSets
				if !metav1.IsControlledBy(&m, newestMachineSet) {
					continue
				}

				if !m.Status.NodeRef.IsDefined() {
					providerID := fakeInfrastructureRefProvisioned(m.Spec.InfrastructureRef, m.Namespace, infraResource, g)
					fakeMachineNodeRef(&m, providerID, g)
				}
			}

			if err := env.List(ctx, machineSets, msListOpts...); err != nil {
				return -1
			}
			return len(machineSets.Items)
		}, timeout*3).Should(BeEquivalentTo(1))

		t.Log("Verifying new MachineSet has desired number of replicas")
		g.Eventually(func() bool {
			g.Expect(env.List(ctx, machineSets, msListOpts...)).Should(Succeed())
			newms := machineSets.Items[0]
			// Set the all non-deleted machines as ready with a NodeRef, so the MachineSet controller can proceed
			// to properly set AvailableReplicas.
			foundMachines := &clusterv1.MachineList{}
			g.Expect(env.List(ctx, foundMachines, client.InNamespace(namespace.Name))).To(Succeed())
			for i := range len(foundMachines.Items) {
				m := foundMachines.Items[i]
				if !m.DeletionTimestamp.IsZero() {
					continue
				}
				// Skip over Machines controlled by other (previous) MachineSets
				if !metav1.IsControlledBy(&m, &newms) {
					continue
				}
				providerID := fakeInfrastructureRefProvisioned(m.Spec.InfrastructureRef, m.Namespace, infraResource, g)
				fakeMachineNodeRef(&m, providerID, g)
			}

			return ptr.Deref(newms.Status.Replicas, 0) == desiredMachineDeploymentReplicas
		}, timeout*5).Should(BeTrue())

		t.Log("Verifying MachineDeployment has correct Conditions")
		g.Eventually(func() bool {
			key := client.ObjectKey{Name: deployment.Name, Namespace: deployment.Namespace}
			g.Expect(env.Get(ctx, key, deployment)).To(Succeed())
			return v1beta1conditions.IsTrue(deployment, clusterv1.MachineDeploymentAvailableV1Beta1Condition)
		}, timeout).Should(BeTrue())

		// Validate that the controller set the cluster name label in selector.
		g.Expect(deployment.Status.Selector).To(ContainSubstring(testCluster.Name))
	})
}

func TestMachineSetToDeployments(t *testing.T) {
	g := NewWithT(t)

	machineDeployment := &clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withMatchingLabels",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachineDeploymentSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo":                      "bar",
					clusterv1.ClusterNameLabel: "test-cluster",
				},
			},
		},
	}

	machineDeplopymentList := []client.Object{machineDeployment}

	ms1 := clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withOwnerRef",
			Namespace: metav1.NamespaceDefault,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(machineDeployment, machineDeploymentKind),
			},
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "test-cluster",
			},
		},
	}
	ms2 := clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "noOwnerRefNoLabels",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "test-cluster",
			},
		},
	}
	ms3 := clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withMatchingLabels",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				"foo":                      "bar",
				clusterv1.ClusterNameLabel: "test-cluster",
			},
		},
	}

	testsCases := []struct {
		machineSet clusterv1.MachineSet
		mapObject  client.Object
		expected   []reconcile.Request
	}{
		{
			machineSet: ms1,
			mapObject:  &ms1,
			expected:   []reconcile.Request{},
		},
		{
			machineSet: ms2,
			mapObject:  &ms2,
			expected:   nil,
		},
		{
			machineSet: ms3,
			mapObject:  &ms3,
			expected: []reconcile.Request{
				{NamespacedName: client.ObjectKey{Namespace: metav1.NamespaceDefault, Name: "withMatchingLabels"}},
			},
		},
	}

	r := &Reconciler{
		Client:   fake.NewClientBuilder().WithObjects(machineDeplopymentList...).Build(),
		recorder: record.NewFakeRecorder(32),
	}

	for _, tc := range testsCases {
		got := r.MachineSetToDeployments(ctx, tc.mapObject)
		g.Expect(got).To(BeComparableTo(tc.expected))
	}
}

func TestGetMachineDeploymentsForMachineSet(t *testing.T) {
	g := NewWithT(t)

	machineDeployment := &clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withLabels",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachineDeploymentSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			},
		},
	}
	machineDeploymentList := []client.Object{machineDeployment}

	ms1 := clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "NoMatchingLabels",
			Namespace: metav1.NamespaceDefault,
		},
	}
	ms2 := clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withMatchingLabels",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				"foo": "bar",
			},
		},
	}

	testCases := []struct {
		machineSet clusterv1.MachineSet
		expected   []client.Object
	}{
		{
			machineSet: ms1,
			expected:   nil,
		},
		{
			machineSet: ms2,
			expected:   []client.Object{machineDeployment},
		},
	}

	r := &Reconciler{
		Client:   fake.NewClientBuilder().WithObjects(append(machineDeploymentList, &ms1, &ms2)...).Build(),
		recorder: record.NewFakeRecorder(32),
	}

	for i := range testCases {
		tc := testCases[i]
		var got []client.Object
		for _, x := range r.getMachineDeploymentsForMachineSet(ctx, &tc.machineSet) {
			got = append(got, x)
		}
		g.Expect(got).To(BeComparableTo(tc.expected))
	}
}

func TestGetMachineSetsForDeployment(t *testing.T) {
	machineDeployment1 := clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withMatchingOwnerRefAndLabels",
			Namespace: metav1.NamespaceDefault,
			UID:       "UID",
		},
		Spec: clusterv1.MachineDeploymentSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			},
		},
	}
	machineDeployment2 := clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withNoMatchingOwnerRef",
			Namespace: metav1.NamespaceDefault,
			UID:       "unMatchingUID",
		},
		Spec: clusterv1.MachineDeploymentSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar2",
				},
			},
		},
	}
	machineDeployment3 := clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withMatchingOwnerRefAndNoMatchingLabels",
			Namespace: metav1.NamespaceDefault,
			UID:       "UID3",
		},
		Spec: clusterv1.MachineDeploymentSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			},
		},
	}

	ms1 := clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withNoOwnerRefShouldBeAdopted2",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				"foo": "bar2",
			},
		},
	}
	ms2 := clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withOwnerRefAndLabels",
			Namespace: metav1.NamespaceDefault,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(&machineDeployment1, machineDeploymentKind),
			},
			Labels: map[string]string{
				"foo": "bar",
			},
		},
	}
	ms3 := clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withNoOwnerRefShouldBeAdopted1",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				"foo": "bar",
			},
		},
	}
	ms4 := clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withNoOwnerRefNoMatch",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				"foo": "nomatch",
			},
		},
	}
	ms5 := clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withOwnerRefAndNoMatchLabels",
			Namespace: metav1.NamespaceDefault,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(&machineDeployment3, machineDeploymentKind),
			},
			Labels: map[string]string{
				"foo": "nomatch",
			},
		},
	}
	machineSetList := []client.Object{
		&ms1,
		&ms2,
		&ms3,
		&ms4,
		&ms5,
	}

	testCases := []struct {
		name              string
		machineDeployment clusterv1.MachineDeployment
		expected          []*clusterv1.MachineSet
	}{
		{
			name:              "matching ownerRef and labels",
			machineDeployment: machineDeployment1,
			expected:          []*clusterv1.MachineSet{&ms3, &ms2},
		},
		{
			name:              "no matching ownerRef, matching labels",
			machineDeployment: machineDeployment2,
			expected:          []*clusterv1.MachineSet{&ms1},
		},
		{
			name:              "matching ownerRef, mismatch labels",
			machineDeployment: machineDeployment3,
			expected:          []*clusterv1.MachineSet{&ms3, &ms5},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			r := &Reconciler{
				Client:   fake.NewClientBuilder().WithObjects(machineSetList...).Build(),
				recorder: record.NewFakeRecorder(32),
			}

			s := &scope{
				machineDeployment: &tc.machineDeployment,
			}
			err := r.getAndAdoptMachineSetsForDeployment(ctx, s)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(s.machineSets).To(HaveLen(len(tc.expected)))
			for idx, res := range s.machineSets {
				g.Expect(res.Name).To(Equal(tc.expected[idx].Name))
				g.Expect(res.Namespace).To(Equal(tc.expected[idx].Namespace))
			}
		})
	}
}

// We have this as standalone variant to be able to use it from the tests.
func updateMachineDeployment(ctx context.Context, c client.Client, md *clusterv1.MachineDeployment, modify func(*clusterv1.MachineDeployment)) error {
	mdObjectKey := util.ObjectKey(md)
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Note: We intentionally don't re-use the passed in MachineDeployment md here as that would
		// overwrite any local changes we might have previously made to the MachineDeployment with the version
		// we get here from the apiserver.
		md := &clusterv1.MachineDeployment{}
		if err := c.Get(ctx, mdObjectKey, md); err != nil {
			return err
		}
		patchHelper, err := patch.NewHelper(md, c)
		if err != nil {
			return err
		}
		modify(md)
		return patchHelper.Patch(ctx, md)
	})
}

func TestReconciler_reconcileDelete(t *testing.T) {
	labels := map[string]string{
		"some": "labelselector",
	}
	md := builder.MachineDeployment("default", "md0").WithClusterName("test").Build()
	md.Finalizers = []string{
		clusterv1.MachineDeploymentFinalizer,
	}
	md.DeletionTimestamp = ptr.To(metav1.Now())
	md.Spec.Selector = metav1.LabelSelector{
		MatchLabels: labels,
	}
	mdWithoutFinalizer := md.DeepCopy()
	mdWithoutFinalizer.Finalizers = []string{}
	tests := []struct {
		name              string
		machineDeployment *clusterv1.MachineDeployment
		want              *clusterv1.MachineDeployment
		objs              []client.Object
		wantMachineSets   []clusterv1.MachineSet
		expectError       bool
	}{
		{
			name:              "Should do nothing when no descendant MachineSets exist and finalizer is already gone",
			machineDeployment: mdWithoutFinalizer.DeepCopy(),
			want:              mdWithoutFinalizer.DeepCopy(),
			objs:              nil,
			wantMachineSets:   nil,
			expectError:       false,
		},
		{
			name:              "Should remove finalizer when no descendant MachineSets exist",
			machineDeployment: md.DeepCopy(),
			want:              mdWithoutFinalizer.DeepCopy(),
			objs:              nil,
			wantMachineSets:   nil,
			expectError:       false,
		},
		{
			name:              "Should keep finalizer when descendant MachineSets exist and trigger deletion only for descendant MachineSets",
			machineDeployment: md.DeepCopy(),
			want:              md.DeepCopy(),
			objs: []client.Object{
				builder.MachineSet("default", "ms0").WithClusterName("test").WithLabels(labels).Build(),
				builder.MachineSet("default", "ms1").WithClusterName("test").WithLabels(labels).Build(),
				builder.MachineSet("default", "ms2-not-part-of-md").WithClusterName("test").Build(),
				builder.MachineSet("default", "ms3-not-part-of-md").WithClusterName("test").Build(),
			},
			wantMachineSets: []clusterv1.MachineSet{
				*builder.MachineSet("default", "ms2-not-part-of-md").WithClusterName("test").Build(),
				*builder.MachineSet("default", "ms3-not-part-of-md").WithClusterName("test").Build(),
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
				machineDeployment: tt.machineDeployment,
			}
			err := r.reconcileDelete(ctx, s)
			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			g.Expect(tt.machineDeployment).To(BeComparableTo(tt.want))

			machineSetList := &clusterv1.MachineSetList{}
			g.Expect(c.List(ctx, machineSetList, client.InNamespace("default"))).ToNot(HaveOccurred())

			// Remove ResourceVersion so we can actually compare.
			for i := range machineSetList.Items {
				machineSetList.Items[i].ResourceVersion = ""
			}

			g.Expect(machineSetList.Items).To(ConsistOf(tt.wantMachineSets))
		})
	}
}
