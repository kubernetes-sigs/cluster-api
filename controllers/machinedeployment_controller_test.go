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
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util"
)

var _ reconcile.Reconciler = &MachineDeploymentReconciler{}

var _ = Describe("MachineDeployment Reconciler", func() {
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "md-test"}}
	testCluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Namespace: namespace.Name, Name: "test-cluster"}}

	BeforeEach(func() {
		By("Creating the namespace")
		Expect(testEnv.Create(ctx, namespace)).To(Succeed())
		By("Creating the Cluster")
		Expect(testEnv.Create(ctx, testCluster)).To(Succeed())
		By("Creating the Cluster Kubeconfig Secret")
		Expect(testEnv.CreateKubeconfigSecret(testCluster)).To(Succeed())
	})

	AfterEach(func() {
		By("Deleting the Cluster")
		Expect(testEnv.Delete(ctx, testCluster)).To(Succeed())
		By("Deleting the namespace")
		Expect(testEnv.Delete(ctx, namespace)).To(Succeed())
	})

	It("Should reconcile a MachineDeployment", func() {
		labels := map[string]string{
			"foo":                      "bar",
			clusterv1.ClusterLabelName: testCluster.Name,
		}
		version := "v1.10.3"
		deployment := &clusterv1.MachineDeployment{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "md-",
				Namespace:    namespace.Name,
				Labels: map[string]string{
					clusterv1.ClusterLabelName: testCluster.Name,
				},
			},
			Spec: clusterv1.MachineDeploymentSpec{
				ClusterName:          testCluster.Name,
				MinReadySeconds:      pointer.Int32Ptr(0),
				Replicas:             pointer.Int32Ptr(2),
				RevisionHistoryLimit: pointer.Int32Ptr(0),
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						clusterv1.ClusterLabelName: testCluster.Name,
					},
				},
				Strategy: &clusterv1.MachineDeploymentStrategy{
					Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
					RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
						MaxUnavailable: intOrStrPtr(0),
						MaxSurge:       intOrStrPtr(1),
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
							APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
							Kind:       "InfrastructureMachineTemplate",
							Name:       "md-template",
						},
						Bootstrap: clusterv1.Bootstrap{
							DataSecretName: pointer.StringPtr("data-secret-name"),
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
			"kind":       "InfrastructureMachine",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha3",
			"metadata":   map[string]interface{}{},
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
		infraTmpl.SetKind("InfrastructureMachineTemplate")
		infraTmpl.SetAPIVersion("infrastructure.cluster.x-k8s.io/v1alpha3")
		infraTmpl.SetName("md-template")
		infraTmpl.SetNamespace(namespace.Name)
		By("Creating the infrastructure template")
		Expect(testEnv.Create(ctx, infraTmpl)).To(Succeed())

		// Create the MachineDeployment object and expect Reconcile to be called.
		By("Creating the MachineDeployment")
		Expect(testEnv.Create(ctx, deployment)).To(Succeed())
		defer func() {
			By("Deleting the MachineDeployment")
			Expect(testEnv.Delete(ctx, deployment)).To(Succeed())
		}()

		By("Verifying the MachineDeployment has a cluster label and ownerRef")
		Eventually(func() bool {
			key := client.ObjectKey{Name: deployment.Name, Namespace: deployment.Namespace}
			if err := testEnv.Get(ctx, key, deployment); err != nil {
				return false
			}
			if len(deployment.Labels) == 0 || deployment.Labels[clusterv1.ClusterLabelName] != testCluster.Name {
				return false
			}
			if len(deployment.OwnerReferences) == 0 || deployment.OwnerReferences[0].Name != testCluster.Name {
				return false
			}
			return true
		}, timeout).Should(BeTrue())

		// Verify that the MachineSet was created.
		By("Verifying the MachineSet was created")
		machineSets := &clusterv1.MachineSetList{}
		Eventually(func() int {
			if err := testEnv.List(ctx, machineSets, msListOpts...); err != nil {
				return -1
			}
			return len(machineSets.Items)
		}, timeout).Should(BeEquivalentTo(1))

		By("Verifying the linked infrastructure template has a cluster owner reference")
		Eventually(func() bool {
			obj, err := external.Get(ctx, testEnv, &deployment.Spec.Template.Spec.InfrastructureRef, deployment.Namespace)
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

		// Verify that expected number of machines are created
		By("Verify expected number of machines are created")
		machines := &clusterv1.MachineList{}
		Eventually(func() int {
			if err := testEnv.List(ctx, machines, client.InNamespace(namespace.Name)); err != nil {
				return -1
			}
			return len(machines.Items)
		}, timeout).Should(BeEquivalentTo(*deployment.Spec.Replicas))

		// Verify that machines has MachineSetLabelName and MachineDeploymentLabelName labels
		By("Verify machines have expected MachineSetLabelName and MachineDeploymentLabelName")
		for _, m := range machines.Items {
			Expect(m.Labels[clusterv1.ClusterLabelName]).To(Equal(testCluster.Name))
		}

		firstMachineSet := machineSets.Items[0]
		Expect(*firstMachineSet.Spec.Replicas).To(BeEquivalentTo(2))
		Expect(*firstMachineSet.Spec.Template.Spec.Version).To(BeEquivalentTo("v1.10.3"))

		//
		// Delete firstMachineSet and expect Reconcile to be called to replace it.
		//
		By("Deleting the initial MachineSet")
		Expect(testEnv.Delete(ctx, &firstMachineSet)).To(Succeed())
		Eventually(func() bool {
			if err := testEnv.List(ctx, machineSets, msListOpts...); err != nil {
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
		By("Scaling the MachineDeployment to 3 replicas")
		modifyFunc := func(d *clusterv1.MachineDeployment) { d.Spec.Replicas = pointer.Int32Ptr(3) }
		Expect(updateMachineDeployment(testEnv, deployment, modifyFunc)).To(Succeed())
		Eventually(func() int {
			key := client.ObjectKey{Name: secondMachineSet.Name, Namespace: secondMachineSet.Namespace}
			if err := testEnv.Get(ctx, key, &secondMachineSet); err != nil {
				return -1
			}
			return int(*secondMachineSet.Spec.Replicas)
		}, timeout).Should(BeEquivalentTo(3))

		//
		// Update a MachineDeployment, expect Reconcile to be called and a new MachineSet to appear.
		//
		By("Setting a label on the MachineDeployment")
		modifyFunc = func(d *clusterv1.MachineDeployment) { d.Spec.Template.Labels["updated"] = "true" }
		Expect(updateMachineDeployment(testEnv, deployment, modifyFunc)).To(Succeed())
		Eventually(func() int {
			if err := testEnv.List(ctx, machineSets, msListOpts...); err != nil {
				return -1
			}
			return len(machineSets.Items)
		}, timeout).Should(BeEquivalentTo(2))

		// Verify that all the MachineSets have the expected OwnerRef.
		By("Verifying MachineSet owner references")
		Eventually(func() bool {
			if err := testEnv.List(ctx, machineSets, msListOpts...); err != nil {
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

		By("Locating the newest MachineSet")
		var thirdMachineSet *clusterv1.MachineSet
		for i := range machineSets.Items {
			ms := &machineSets.Items[i]
			if ms.UID != secondMachineSet.UID {
				thirdMachineSet = ms
				break
			}
		}
		Expect(thirdMachineSet).NotTo(BeNil())

		By("Verifying the initial MachineSet is deleted")
		Eventually(func() int {
			// Set the all non-deleted machines as ready with a NodeRef, so the MachineSet controller can proceed
			// to properly set AvailableReplicas.
			foundMachines := &clusterv1.MachineList{}
			Expect(testEnv.List(ctx, foundMachines, client.InNamespace(namespace.Name))).To(Succeed())
			for i := 0; i < len(foundMachines.Items); i++ {
				m := foundMachines.Items[i]
				// Skip over deleted Machines
				if !m.DeletionTimestamp.IsZero() {
					continue
				}
				// Skip over Machines controlled by other (previous) MachineSets
				if !metav1.IsControlledBy(&m, thirdMachineSet) {
					continue
				}
				providerID := fakeInfrastructureRefReady(m.Spec.InfrastructureRef, infraResource)
				fakeMachineNodeRef(&m, providerID)
			}

			if err := testEnv.List(ctx, machineSets, msListOpts...); err != nil {
				return -1
			}
			return len(machineSets.Items)
		}, timeout*3).Should(BeEquivalentTo(1))

		//
		// Update a MachineDeployment spec.Selector.Matchlabels spec.Template.Labels
		// expect Reconcile to be called and a new MachineSet to appear
		// expect old MachineSets with old labels to be deleted
		//
		oldLabels := deployment.Spec.Selector.MatchLabels
		oldLabels[clusterv1.MachineDeploymentLabelName] = deployment.Name

		newLabels := map[string]string{
			"new-key":                  "new-value",
			clusterv1.ClusterLabelName: testCluster.Name,
		}

		By("Updating MachineDeployment label")
		modifyFunc = func(d *clusterv1.MachineDeployment) {
			d.Spec.Selector.MatchLabels = newLabels
			d.Spec.Template.Labels = newLabels
		}
		Expect(updateMachineDeployment(testEnv, deployment, modifyFunc)).To(Succeed())

		By("Verifying if a new MachineSet with updated labels are created")
		Eventually(func() int {
			listOpts := client.MatchingLabels(newLabels)
			if err := testEnv.List(ctx, machineSets, listOpts); err != nil {
				return -1
			}
			return len(machineSets.Items)
		}, timeout).Should(BeEquivalentTo(1))
		newms := machineSets.Items[0]

		By("Verifying new MachineSet has desired number of replicas")
		Eventually(func() bool {
			// Set the all non-deleted machines as ready with a NodeRef, so the MachineSet controller can proceed
			// to properly set AvailableReplicas.
			foundMachines := &clusterv1.MachineList{}
			Expect(testEnv.List(ctx, foundMachines, client.InNamespace(namespace.Name))).To(Succeed())
			for i := 0; i < len(foundMachines.Items); i++ {
				m := foundMachines.Items[i]
				if !m.DeletionTimestamp.IsZero() {
					continue
				}
				// Skip over Machines controlled by other (previous) MachineSets
				if !metav1.IsControlledBy(&m, &newms) {
					continue
				}
				providerID := fakeInfrastructureRefReady(m.Spec.InfrastructureRef, infraResource)
				fakeMachineNodeRef(&m, providerID)
			}

			listOpts := client.MatchingLabels(newLabels)
			if err := testEnv.List(ctx, machineSets, listOpts); err != nil {
				return false
			}
			return machineSets.Items[0].Status.Replicas == *deployment.Spec.Replicas
		}, timeout*5).Should(BeTrue())

		By("Verifying MachineSets with old labels are deleted")
		Eventually(func() int {
			listOpts := client.MatchingLabels(oldLabels)
			if err := testEnv.List(ctx, machineSets, listOpts); err != nil {
				return -1
			}

			return len(machineSets.Items)
		}, timeout*5).Should(BeEquivalentTo(0))

		// Validate that the controller set the cluster name label in selector.
		Expect(deployment.Status.Selector).To(ContainSubstring(testCluster.Name))
	})
})

func TestMachineSetToDeployments(t *testing.T) {
	g := NewWithT(t)

	machineDeployment := clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withMatchingLabels",
			Namespace: "test",
		},
		Spec: clusterv1.MachineDeploymentSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo":                      "bar",
					clusterv1.ClusterLabelName: "test-cluster",
				},
			},
		},
	}

	machineDeplopymentList := &clusterv1.MachineDeploymentList{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineDeploymentList",
		},
		Items: []clusterv1.MachineDeployment{machineDeployment},
	}

	ms1 := clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withOwnerRef",
			Namespace: "test",
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(&machineDeployment, machineDeploymentKind),
			},
			Labels: map[string]string{
				clusterv1.ClusterLabelName: "test-cluster",
			},
		},
	}
	ms2 := clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "noOwnerRefNoLabels",
			Namespace: "test",
			Labels: map[string]string{
				clusterv1.ClusterLabelName: "test-cluster",
			},
		},
	}
	ms3 := clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withMatchingLabels",
			Namespace: "test",
			Labels: map[string]string{
				"foo":                      "bar",
				clusterv1.ClusterLabelName: "test-cluster",
			},
		},
	}

	testsCases := []struct {
		machineSet clusterv1.MachineSet
		mapObject  handler.MapObject
		expected   []reconcile.Request
	}{
		{
			machineSet: ms1,
			mapObject: handler.MapObject{
				Meta:   ms1.GetObjectMeta(),
				Object: &ms1,
			},
			expected: []reconcile.Request{},
		},
		{
			machineSet: ms2,
			mapObject: handler.MapObject{
				Meta:   ms2.GetObjectMeta(),
				Object: &ms2,
			},
			expected: nil,
		},
		{
			machineSet: ms3,
			mapObject: handler.MapObject{
				Meta:   ms3.GetObjectMeta(),
				Object: &ms3,
			},
			expected: []reconcile.Request{
				{NamespacedName: client.ObjectKey{Namespace: "test", Name: "withMatchingLabels"}},
			},
		},
	}

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
	r := &MachineDeploymentReconciler{
		Client:   fake.NewFakeClientWithScheme(scheme.Scheme, machineDeplopymentList),
		Log:      log.Log,
		recorder: record.NewFakeRecorder(32),
	}

	for _, tc := range testsCases {
		got := r.MachineSetToDeployments(tc.mapObject)
		g.Expect(got).To(Equal(tc.expected))
	}
}

func TestGetMachineDeploymentsForMachineSet(t *testing.T) {
	g := NewWithT(t)

	machineDeployment := clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withLabels",
			Namespace: "test",
		},
		Spec: clusterv1.MachineDeploymentSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			},
		},
	}
	machineDeplopymentList := &clusterv1.MachineDeploymentList{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineDeploymentList",
		},
		Items: []clusterv1.MachineDeployment{
			machineDeployment,
		},
	}
	ms1 := clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "NoMatchingLabels",
			Namespace: "test",
		},
	}
	ms2 := clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withMatchingLabels",
			Namespace: "test",
			Labels: map[string]string{
				"foo": "bar",
			},
		},
	}

	testCases := []struct {
		machineDeploymentList clusterv1.MachineDeploymentList
		machineSet            clusterv1.MachineSet
		expected              []*clusterv1.MachineDeployment
	}{
		{
			machineDeploymentList: *machineDeplopymentList,
			machineSet:            ms1,
			expected:              nil,
		},
		{
			machineDeploymentList: *machineDeplopymentList,
			machineSet:            ms2,
			expected:              []*clusterv1.MachineDeployment{&machineDeployment},
		},
	}

	g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())
	r := &MachineDeploymentReconciler{
		Client:   fake.NewFakeClientWithScheme(scheme.Scheme, &ms1, &ms2, machineDeplopymentList),
		Log:      log.Log,
		recorder: record.NewFakeRecorder(32),
	}

	for _, tc := range testCases {
		got := r.getMachineDeploymentsForMachineSet(&tc.machineSet)
		g.Expect(got).To(Equal(tc.expected))
	}
}

func TestGetMachineSetsForDeployment(t *testing.T) {
	machineDeployment1 := clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "withMatchingOwnerRefAndLabels",
			Namespace: "test",
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
			Namespace: "test",
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
			Namespace: "test",
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
			Namespace: "test",
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
			Namespace: "test",
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
			Namespace: "test",
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
			Namespace: "test",
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
			Namespace: "test",
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(&machineDeployment3, machineDeploymentKind),
			},
			Labels: map[string]string{
				"foo": "nomatch",
			},
		},
	}
	machineSetList := &clusterv1.MachineSetList{
		TypeMeta: metav1.TypeMeta{
			Kind: "MachineSetList",
		},
		Items: []clusterv1.MachineSet{
			ms1,
			ms2,
			ms3,
			ms4,
			ms5,
		},
	}

	testCases := []struct {
		name              string
		machineDeployment clusterv1.MachineDeployment
		expected          []*clusterv1.MachineSet
	}{
		{
			name:              "matching ownerRef and labels",
			machineDeployment: machineDeployment1,
			expected:          []*clusterv1.MachineSet{&ms2, &ms3},
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

			g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())

			r := &MachineDeploymentReconciler{
				Client:   fake.NewFakeClientWithScheme(scheme.Scheme, machineSetList),
				Log:      log.Log,
				recorder: record.NewFakeRecorder(32),
			}

			got, err := r.getMachineSetsForDeployment(&tc.machineDeployment)
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(got).To(HaveLen(len(tc.expected)))

			for idx, res := range got {
				g.Expect(res.Name).To(Equal(tc.expected[idx].Name))
				g.Expect(res.Namespace).To(Equal(tc.expected[idx].Namespace))
			}
		})
	}
}
