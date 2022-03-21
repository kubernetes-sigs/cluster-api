package cmd

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/tree"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	client2 "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_Conditions(t *testing.T) {
	cluster := &clusterv1.Cluster{}
	machineSets := []*clusterv1.MachineSet{}
	machineDeployments := []*clusterv1.MachineDeployment{}
	machines := []*clusterv1.Machine{}

	{ // Define Cluster
		cluster = &clusterv1.Cluster{
			TypeMeta: metav1.TypeMeta{
				Kind: "Cluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: corev1.NamespaceDefault,
				Name:      "my-cluster",
				UID:       "my-cluster",
			},
		}
		conditions.MarkTrue(cluster, clusterv1.InfrastructureReadyCondition) // Faking a local condition on the Cluster, not relevant for rolling up the machine status

		{ // Define MachineDeployment md1
			md1 := &clusterv1.MachineDeployment{
				TypeMeta: metav1.TypeMeta{
					Kind: "MachineDeployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: corev1.NamespaceDefault,
					Name:      "md1",
					UID:       "md1",
					Labels:    map[string]string{clusterv1.ClusterLabelName: cluster.Name},
				},
			}
			conditions.MarkTrue(md1, clusterv1.MachineDeploymentAvailableCondition) // Faking a local condition on the MachineDeployment, not relevant for rolling up the machine status
			machineDeployments = append(machineDeployments, md1)

			{ // Define a MachineSet ms1
				md1ms1 := &clusterv1.MachineSet{
					TypeMeta: metav1.TypeMeta{
						Kind: "MachineSet",
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: corev1.NamespaceDefault,
						Name:      "ms1",
						UID:       "ms1",
						Labels:    map[string]string{clusterv1.ClusterLabelName: cluster.Name},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: md1.APIVersion,
								Kind:       md1.Kind,
								Name:       md1.Name,
								UID:        md1.UID,
								Controller: pointer.BoolPtr(true),
							},
						},
					},
				}
				conditions.MarkTrue(md1ms1, clusterv1.MachinesCreatedCondition) // Faking a local condition on the MachineSet, not relevant for rolling up the machine status
				conditions.MarkTrue(md1ms1, clusterv1.ResizedCondition)         // Faking a local condition on the MachineSet, not relevant for rolling up the machine status
				conditions.MarkTrue(md1ms1, clusterv1.MachinesReadyCondition)   // Faking a local condition on the MachineSet, not relevant for rolling up the machine status
				machineSets = append(machineSets, md1ms1)

				{ // Define a Machines
					md1ms1m1 := &clusterv1.Machine{
						TypeMeta: metav1.TypeMeta{
							Kind: "Machine",
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: corev1.NamespaceDefault,
							Name:      "m1",
							UID:       "m1",
							Labels:    map[string]string{clusterv1.ClusterLabelName: cluster.Name},
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: md1ms1.APIVersion,
									Kind:       md1ms1.Kind,
									Name:       md1ms1.Name,
									UID:        md1ms1.UID,
									Controller: pointer.BoolPtr(true),
								},
							},
						},
					}
					conditions.MarkTrue(md1ms1m1, clusterv1.InfrastructureReadyCondition) // Faking a local condition on the Machine, not relevant for rolling up the machine status
					conditions.MarkTrue(md1ms1m1, clusterv1.BootstrapReadyCondition)      // Faking a local condition on the Machine, not relevant for rolling up the machine status
					machines = append(machines, md1ms1m1)
				}
			}
		}
	}

	// Simulate controllers computing conditions

	{ // MachineController
		for _, m := range machines {
			// From MachineController.Reconcile
			conditions.SetSummary(m,
				conditions.WithConditions(
					// Infrastructure problems should take precedence over all the other conditions
					clusterv1.InfrastructureReadyCondition,
					// Boostrap comes after, but it is relevant only during initial machine provisioning.
					clusterv1.BootstrapReadyCondition,
					// MHC reported condition should take precedence over the remediation progress
					clusterv1.MachineHealthCheckSucceededCondition,
					clusterv1.MachineOwnerRemediatedCondition,
				),
				conditions.WithStepCounterIf(m.ObjectMeta.DeletionTimestamp.IsZero() && m.Spec.ProviderID == nil),
				conditions.WithStepCounterIfOnly(
					clusterv1.BootstrapReadyCondition,
					clusterv1.InfrastructureReadyCondition,
				),
			)
		}
	}

	{ // MachineSetController
		for _, ms := range machineSets {
			controlledMachines := []*clusterv1.Machine{}
			for _, m := range machines {
				if util.IsControlledBy(m, ms) {
					controlledMachines = append(controlledMachines, m)
				}
			}

			// From MachineSetController.UpdateStatus
			// Note: This rollouts state from the machine
			conditions.SetAggregate(ms, clusterv1.MachinesReadyCondition, collections.FromMachines(controlledMachines...).ConditionGetters(), conditions.AddSourceRef(), conditions.WithStepCounterIf(false))

			// From MachineSet.Reconcile
			conditions.SetSummary(ms,
				conditions.WithConditions(
					clusterv1.MachinesCreatedCondition,
					clusterv1.ResizedCondition,
					clusterv1.MachinesReadyCondition,
				),
			)
		}
	}

	{ // MachineDeploymentController
		for _, md := range machineDeployments {
			// TODO: implement something that bubbles up the state from owner machines into the MD

			// From MachineDeploymentController.Reconcile
			conditions.SetSummary(md,
				conditions.WithConditions(
					clusterv1.MachineDeploymentAvailableCondition,
				),
			)
		}
	}

	{ // ClusterController
		// TODO: implement something that bubbles up the state from owner machines into the Cluster

		// From ClusterController.Reconcile
		conditions.SetSummary(cluster,
			conditions.WithConditions(
				clusterv1.ControlPlaneReadyCondition,
				clusterv1.InfrastructureReadyCondition,
			),
		)
	}

	{ // Show the resulting object tree with all the conditions
		objs := []client2.Object{cluster}
		for _, md := range machineDeployments {
			objs = append(objs, md)
		}
		for _, ms := range machineSets {
			objs = append(objs, ms)
		}
		for _, m := range machines {
			objs = append(objs, m)
		}
		client := fake.NewClientBuilder().
			WithScheme(test.FakeScheme).
			WithObjects(objs...).
			Build()

		// Gets the object tree representing the status of a Cluster API cluster.
		tree, err := tree.Discovery(context.TODO(), client, corev1.NamespaceDefault, "my-cluster", tree.DiscoverOptions{
			ShowOtherConditions: "all",
			ShowMachineSets:     true,
			Echo:                false,
			Grouping:            false,
		})
		if err != nil {
			t.Fatal(err)
		}
		printObjectTree(tree)
	}
}
