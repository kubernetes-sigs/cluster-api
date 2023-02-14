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
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
)

const (
	machineDeploymentNamespace = "md-test"
)

var _ reconcile.Reconciler = &Reconciler{}

func TestMachineDeploymentReconciler(t *testing.T) {
	setup := func(t *testing.T, g *WithT) (*corev1.Namespace, *clusterv1.Cluster) {
		t.Helper()

		t.Log("Creating the namespace")
		ns, err := env.CreateNamespace(ctx, machineDeploymentNamespace)
		g.Expect(err).To(BeNil())

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
				ClusterName:          testCluster.Name,
				MinReadySeconds:      pointer.Int32(0),
				Replicas:             pointer.Int32(2),
				RevisionHistoryLimit: pointer.Int32(0),
				Selector: metav1.LabelSelector{
					// We're using the same labels for spec.selector and spec.template.labels.
					// The labels are later changed and we will use the initial labels later to
					// verify that all original MachineSets have been deleted.
					MatchLabels: labels,
				},
				Strategy: &clusterv1.MachineDeploymentStrategy{
					Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
					RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
						MaxUnavailable: intOrStrPtr(0),
						MaxSurge:       intOrStrPtr(1),
						DeletePolicy:   pointer.String("Oldest"),
					},
				},
				Template: clusterv1.MachineTemplateSpec{
					ObjectMeta: clusterv1.ObjectMeta{
						Labels: labels,
					},
					Spec: clusterv1.MachineSpec{
						ClusterName: testCluster.Name,
						Version:     &version,
						InfrastructureRef: corev1.ObjectReference{
							APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
							Kind:       "GenericInfrastructureMachineTemplate",
							Name:       "md-template",
						},
						Bootstrap: clusterv1.Bootstrap{
							DataSecretName: pointer.String("data-secret-name"),
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
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
			"metadata":   map[string]interface{}{},
			"spec": map[string]interface{}{
				"size": "3xlarge",
			},
		}
		infraTmpl := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       "GenericInfrastructureMachineTemplate",
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
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

		t.Log("Verifying that the deployment's deletePolicy was propagated to the machineset")
		g.Expect(machineSets.Items[0].Spec.DeletePolicy).To(Equal("Oldest"))

		t.Log("Verifying the linked infrastructure template has a cluster owner reference")
		g.Eventually(func() bool {
			obj, err := external.Get(ctx, env, &deployment.Spec.Template.Spec.InfrastructureRef, deployment.Namespace)
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
		g.Expect(*firstMachineSet.Spec.Template.Spec.Version).To(BeEquivalentTo("v1.10.3"))

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
			d.Spec.Replicas = pointer.Int32(desiredMachineDeploymentReplicas)
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
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"metadata": map[string]interface{}{
					"name":      "md-template-2",
					"namespace": namespace.Name,
				},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"kind":       "GenericInfrastructureMachine",
						"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
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

		infraTmpl2Ref := corev1.ObjectReference{
			APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
			Kind:       "GenericInfrastructureMachineTemplate",
			Name:       "md-template-2",
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
			// Verify that the new MachineSet gets the updated labels.
			g.Expect(machineSets.Items[0].Spec.Template.Labels).To(HaveKeyWithValue("updated", "true"))
			// Verify that the old MachineSet does not get the updated labels.
			g.Expect(machineSets.Items[1].Spec.Template.Labels).ShouldNot(HaveKeyWithValue("updated", "true"))
		}, timeout).Should(Succeed())

		// Update the NodeDrainTimout, NodeDeletionTimeout, NodeVolumeDetachTimeout of the MachineDeployment,
		// expect the Reconcile to be called and the MachineSet to be updated in-place.
		t.Log("Setting NodeDrainTimout, NodeDeletionTimeout, NodeVolumeDetachTimeout on the MachineDeployment")
		duration10s := metav1.Duration{Duration: 10 * time.Second}
		modifyFunc = func(d *clusterv1.MachineDeployment) {
			d.Spec.Template.Spec.NodeDrainTimeout = &duration10s
			d.Spec.Template.Spec.NodeDeletionTimeout = &duration10s
			d.Spec.Template.Spec.NodeVolumeDetachTimeout = &duration10s
		}
		g.Expect(updateMachineDeployment(ctx, env, deployment, modifyFunc)).To(Succeed())
		g.Eventually(func(g Gomega) {
			g.Expect(env.List(ctx, machineSets, msListOpts...)).Should(Succeed())
			// Verify we still only have 2 MachineSets.
			g.Expect(machineSets.Items).To(HaveLen(2))
			// Verify the NodeDrainTimeout value is updated
			g.Expect(machineSets.Items[0].Spec.Template.Spec.NodeDrainTimeout).Should(And(
				Not(BeNil()),
				HaveValue(Equal(duration10s)),
			), "NodeDrainTimout value does not match expected")
			// Verify the NodeDeletionTimeout value is updated
			g.Expect(machineSets.Items[0].Spec.Template.Spec.NodeDeletionTimeout).Should(And(
				Not(BeNil()),
				HaveValue(Equal(duration10s)),
			), "NodeDeletionTimeout value does not match expected")
			// Verify the NodeVolumeDetachTimeout value is updated
			g.Expect(machineSets.Items[0].Spec.Template.Spec.NodeVolumeDetachTimeout).Should(And(
				Not(BeNil()),
				HaveValue(Equal(duration10s)),
			), "NodeVolumeDetachTimeout value does not match expected")

			// Verify that the old machine set keeps the old values.
			g.Expect(machineSets.Items[1].Spec.Template.Spec.NodeDrainTimeout).Should(BeNil())
			g.Expect(machineSets.Items[1].Spec.Template.Spec.NodeDeletionTimeout).Should(BeNil())
			g.Expect(machineSets.Items[1].Spec.Template.Spec.NodeVolumeDetachTimeout).Should(BeNil())
		}).Should(Succeed())

		// Update the DeletePolicy of the MachineDeployment,
		// expect the Reconcile to be called and the MachineSet to be updated in-place.
		t.Log("Updating deletePolicy on the MachineDeployment")
		modifyFunc = func(d *clusterv1.MachineDeployment) {
			d.Spec.Strategy.RollingUpdate.DeletePolicy = pointer.String("Newest")
		}
		g.Expect(updateMachineDeployment(ctx, env, deployment, modifyFunc)).To(Succeed())
		g.Eventually(func(g Gomega) {
			g.Expect(env.List(ctx, machineSets, msListOpts...)).Should(Succeed())
			// Verify we still only have 2 MachineSets.
			g.Expect(machineSets.Items).To(HaveLen(2))
			// Verify the DeletePolicy value is updated
			g.Expect(machineSets.Items[0].Spec.DeletePolicy).Should(Equal("Newest"))

			// Verify that the old machine set retains its delete policy
			g.Expect(machineSets.Items[1].Spec.DeletePolicy).To(Equal("Oldest"))
		}).Should(Succeed())

		// Verify that all the MachineSets have the expected OwnerRef.
		t.Log("Verifying MachineSet owner references")
		g.Eventually(func() bool {
			if err := env.List(ctx, machineSets, msListOpts...); err != nil {
				return false
			}
			for i := 0; i < len(machineSets.Items); i++ {
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
			for i := 0; i < len(foundMachines.Items); i++ {
				m := foundMachines.Items[i]
				// Skip over deleted Machines
				if !m.DeletionTimestamp.IsZero() {
					continue
				}
				// Skip over Machines controlled by other (previous) MachineSets
				if !metav1.IsControlledBy(&m, newestMachineSet) {
					continue
				}
				providerID := fakeInfrastructureRefReady(m.Spec.InfrastructureRef, infraResource, g)
				fakeMachineNodeRef(&m, providerID, g)
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
			for i := 0; i < len(foundMachines.Items); i++ {
				m := foundMachines.Items[i]
				if !m.DeletionTimestamp.IsZero() {
					continue
				}
				// Skip over Machines controlled by other (previous) MachineSets
				if !metav1.IsControlledBy(&m, &newms) {
					continue
				}
				providerID := fakeInfrastructureRefReady(m.Spec.InfrastructureRef, infraResource, g)
				fakeMachineNodeRef(&m, providerID, g)
			}

			return newms.Status.Replicas == desiredMachineDeploymentReplicas
		}, timeout*5).Should(BeTrue())

		t.Log("Verifying MachineDeployment has correct Conditions")
		g.Eventually(func() bool {
			key := client.ObjectKey{Name: deployment.Name, Namespace: deployment.Namespace}
			g.Expect(env.Get(ctx, key, deployment)).To(Succeed())
			return conditions.IsTrue(deployment, clusterv1.MachineDeploymentAvailableCondition)
		}, timeout).Should(BeTrue())

		// Validate that the controller set the cluster name label in selector.
		g.Expect(deployment.Status.Selector).To(ContainSubstring(testCluster.Name))
	})
}

func TestMachineDeploymentReconciler_CleanUpManagedFieldsForSSAAdoption(t *testing.T) {
	setup := func(t *testing.T, g *WithT) (*corev1.Namespace, *clusterv1.Cluster) {
		t.Helper()

		t.Log("Creating the namespace")
		ns, err := env.CreateNamespace(ctx, machineDeploymentNamespace)
		g.Expect(err).To(BeNil())

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
			Paused:               true, // Set this to true as we do not want to test the other parts of the reconciler in this test.
			ClusterName:          testCluster.Name,
			MinReadySeconds:      pointer.Int32(0),
			Replicas:             pointer.Int32(2),
			RevisionHistoryLimit: pointer.Int32(0),
			Selector: metav1.LabelSelector{
				// We're using the same labels for spec.selector and spec.template.labels.
				MatchLabels: labels,
			},
			Strategy: &clusterv1.MachineDeploymentStrategy{
				Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
				RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
					MaxUnavailable: intOrStrPtr(0),
					MaxSurge:       intOrStrPtr(1),
					DeletePolicy:   pointer.String("Oldest"),
				},
			},
			Template: clusterv1.MachineTemplateSpec{
				ObjectMeta: clusterv1.ObjectMeta{
					Labels: labels,
				},
				Spec: clusterv1.MachineSpec{
					ClusterName: testCluster.Name,
					Version:     &version,
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
						Kind:       "GenericInfrastructureMachineTemplate",
						Name:       "md-template",
					},
					Bootstrap: clusterv1.Bootstrap{
						DataSecretName: pointer.String("data-secret-name"),
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
		"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
		"metadata":   map[string]interface{}{},
		"spec": map[string]interface{}{
			"size": "3xlarge",
		},
	}
	infraTmpl := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericInfrastructureMachineTemplate",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
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

	// Create a MachineSet for the MachineDeployment.
	classicManagerMS := &clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachineSet",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployment.Name + "-" + "classic-ms",
			Namespace: testCluster.Namespace,
			Labels:    labels,
		},
		Spec: clusterv1.MachineSetSpec{
			ClusterName:     testCluster.Name,
			Replicas:        pointer.Int32(0),
			MinReadySeconds: 0,
			Selector: metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: clusterv1.MachineTemplateSpec{
				ObjectMeta: clusterv1.ObjectMeta{
					Labels: labels,
				},
				Spec: clusterv1.MachineSpec{
					ClusterName: testCluster.Name,
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
						Kind:       "GenericInfrastructureMachineTemplate",
						Name:       "md-template",
					},
					Bootstrap: clusterv1.Bootstrap{
						DataSecretName: pointer.String("data-secret-name"),
					},
					Version: &version,
				},
			},
		},
	}
	ssaManagerMS := classicManagerMS.DeepCopy()
	ssaManagerMS.Name = deployment.Name + "-" + "ssa-ms"

	// Create one using the "old manager".
	g.Expect(env.Create(ctx, classicManagerMS, client.FieldOwner("manager"))).To(Succeed())

	// Create one using SSA.
	g.Expect(env.Patch(ctx, ssaManagerMS, client.Apply, client.FieldOwner(machineDeploymentManagerName), client.ForceOwnership)).To(Succeed())

	// Verify that for both the MachineSets the ManagedFields are updated.
	g.Eventually(func(g Gomega) {
		machineSets := &clusterv1.MachineSetList{}
		g.Expect(env.List(ctx, machineSets, msListOpts...)).To(Succeed())

		g.Expect(machineSets.Items).To(HaveLen(2))
		for _, ms := range machineSets.Items {
			// Verify the ManagedFields are updated.
			g.Expect(ms.GetManagedFields()).Should(
				ContainElement(ssa.MatchManagedFieldsEntry(machineDeploymentManagerName, metav1.ManagedFieldsOperationApply)))
			g.Expect(ms.GetManagedFields()).ShouldNot(
				ContainElement(ssa.MatchManagedFieldsEntry("manager", metav1.ManagedFieldsOperationUpdate)))
		}
	}).Should(Succeed())
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
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
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
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "noOwnerRefNoLabels",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "test-cluster",
			},
		},
	}
	ms3 := clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
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
		got := r.MachineSetToDeployments(tc.mapObject)
		g.Expect(got).To(Equal(tc.expected))
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
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "NoMatchingLabels",
			Namespace: metav1.NamespaceDefault,
		},
	}
	ms2 := clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
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

	for _, tc := range testCases {
		var got []client.Object
		for _, x := range r.getMachineDeploymentsForMachineSet(ctx, &tc.machineSet) {
			got = append(got, x)
		}
		g.Expect(got).To(Equal(tc.expected))
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
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withNoOwnerRefShouldBeAdopted2",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				"foo": "bar2",
			},
		},
	}
	ms2 := clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
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
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withNoOwnerRefShouldBeAdopted1",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				"foo": "bar",
			},
		},
	}
	ms4 := clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withNoOwnerRefNoMatch",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				"foo": "nomatch",
			},
		},
	}
	ms5 := clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			r := &Reconciler{
				Client:   fake.NewClientBuilder().WithObjects(machineSetList...).Build(),
				recorder: record.NewFakeRecorder(32),
			}

			got, err := r.getMachineSetsForDeployment(ctx, &tc.machineDeployment)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(HaveLen(len(tc.expected)))

			for idx, res := range got {
				g.Expect(res.Name).To(Equal(tc.expected[idx].Name))
				g.Expect(res.Namespace).To(Equal(tc.expected[idx].Namespace))
			}
		})
	}
}
