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

package internal

import (
	"context"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/failuredomains"
	"sigs.k8s.io/cluster-api/util/patch"
)

// ControlPlane holds business logic around control planes.
// It should never need to connect to a service, that responsibility lies outside of this struct.
// Going forward we should be trying to add more logic to here and reduce the amount of logic in the reconciler.
type ControlPlane struct {
	KCP                  *controlplanev1.KubeadmControlPlane
	Cluster              *clusterv1.Cluster
	Machines             collections.Machines
	machinesPatchHelpers map[string]*patch.Helper

	// reconciliationTime is the time of the current reconciliation, and should be used for all "now" calculations
	reconciliationTime metav1.Time

	// TODO: we should see if we can combine these with the Machine objects so we don't have all these separate lookups
	// See discussion on https://github.com/kubernetes-sigs/cluster-api/pull/3405
	KubeadmConfigs map[string]*bootstrapv1.KubeadmConfig
	InfraResources map[string]*unstructured.Unstructured

	managementCluster ManagementCluster
	workloadCluster   WorkloadCluster
}

// NewControlPlane returns an instantiated ControlPlane.
func NewControlPlane(ctx context.Context, managementCluster ManagementCluster, client client.Client, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane, ownedMachines collections.Machines) (*ControlPlane, error) {
	infraObjects, err := getInfraResources(ctx, client, ownedMachines)
	if err != nil {
		return nil, err
	}
	kubeadmConfigs, err := getKubeadmConfigs(ctx, client, ownedMachines)
	if err != nil {
		return nil, err
	}
	patchHelpers := map[string]*patch.Helper{}
	for _, machine := range ownedMachines {
		patchHelper, err := patch.NewHelper(machine, client)
		if err != nil {
			return nil, err
		}
		patchHelpers[machine.Name] = patchHelper
	}

	return &ControlPlane{
		KCP:                  kcp,
		Cluster:              cluster,
		Machines:             ownedMachines,
		machinesPatchHelpers: patchHelpers,
		KubeadmConfigs:       kubeadmConfigs,
		InfraResources:       infraObjects,
		reconciliationTime:   metav1.Now(),
		managementCluster:    managementCluster,
	}, nil
}

// FailureDomains returns a slice of failure domain objects synced from the infrastructure provider into Cluster.Status.
func (c *ControlPlane) FailureDomains() clusterv1.FailureDomains {
	if c.Cluster.Status.FailureDomains == nil {
		return clusterv1.FailureDomains{}
	}
	return c.Cluster.Status.FailureDomains
}

// MachineInFailureDomainWithMostMachines returns the first matching failure domain with machines that has the most control-plane machines on it.
func (c *ControlPlane) MachineInFailureDomainWithMostMachines(ctx context.Context, machines collections.Machines) (*clusterv1.Machine, error) {
	fd := c.FailureDomainWithMostMachines(ctx, machines)
	machinesInFailureDomain := machines.Filter(collections.InFailureDomains(fd))
	machineToMark := machinesInFailureDomain.Oldest()
	if machineToMark == nil {
		return nil, errors.New("failed to pick control plane Machine to mark for deletion")
	}
	return machineToMark, nil
}

// MachineWithDeleteAnnotation returns a machine that has been annotated with DeleteMachineAnnotation key.
func (c *ControlPlane) MachineWithDeleteAnnotation(machines collections.Machines) collections.Machines {
	// See if there are any machines with DeleteMachineAnnotation key.
	annotatedMachines := machines.Filter(collections.HasAnnotationKey(clusterv1.DeleteMachineAnnotation))
	// If there are, return list of annotated machines.
	return annotatedMachines
}

// FailureDomainWithMostMachines returns a fd which exists both in machines and control-plane machines and has the most
// control-plane machines on it.
func (c *ControlPlane) FailureDomainWithMostMachines(ctx context.Context, machines collections.Machines) *string {
	// See if there are any Machines that are not in currently defined failure domains first.
	notInFailureDomains := machines.Filter(
		collections.Not(collections.InFailureDomains(c.FailureDomains().FilterControlPlane().GetIDs()...)),
	)
	if len(notInFailureDomains) > 0 {
		// return the failure domain for the oldest Machine not in the current list of failure domains
		// this could be either nil (no failure domain defined) or a failure domain that is no longer defined
		// in the cluster status.
		return notInFailureDomains.Oldest().Spec.FailureDomain
	}
	return failuredomains.PickMost(ctx, c.Cluster.Status.FailureDomains.FilterControlPlane(), c.Machines, machines)
}

// NextFailureDomainForScaleUp returns the failure domain with the fewest number of up-to-date machines.
func (c *ControlPlane) NextFailureDomainForScaleUp(ctx context.Context) *string {
	if len(c.Cluster.Status.FailureDomains.FilterControlPlane()) == 0 {
		return nil
	}
	return failuredomains.PickFewest(ctx, c.FailureDomains().FilterControlPlane(), c.UpToDateMachines())
}

// InitialControlPlaneConfig returns a new KubeadmConfigSpec that is to be used for an initializing control plane.
func (c *ControlPlane) InitialControlPlaneConfig() *bootstrapv1.KubeadmConfigSpec {
	bootstrapSpec := c.KCP.Spec.KubeadmConfigSpec.DeepCopy()
	bootstrapSpec.JoinConfiguration = nil
	return bootstrapSpec
}

// JoinControlPlaneConfig returns a new KubeadmConfigSpec that is to be used for joining control planes.
func (c *ControlPlane) JoinControlPlaneConfig() *bootstrapv1.KubeadmConfigSpec {
	bootstrapSpec := c.KCP.Spec.KubeadmConfigSpec.DeepCopy()
	bootstrapSpec.InitConfiguration = nil
	// NOTE: For the joining we are preserving the ClusterConfiguration in order to determine if the
	// cluster is using an external etcd in the kubeadm bootstrap provider (even if this is not required by kubeadm Join).
	// TODO: Determine if this copy of cluster configuration can be used for rollouts (thus allowing to remove the annotation at machine level)
	return bootstrapSpec
}

// HasDeletingMachine returns true if any machine in the control plane is in the process of being deleted.
func (c *ControlPlane) HasDeletingMachine() bool {
	return len(c.Machines.Filter(collections.HasDeletionTimestamp)) > 0
}

// GetKubeadmConfig returns the KubeadmConfig of a given machine.
func (c *ControlPlane) GetKubeadmConfig(machineName string) (*bootstrapv1.KubeadmConfig, bool) {
	kubeadmConfig, ok := c.KubeadmConfigs[machineName]
	return kubeadmConfig, ok
}

// MachinesNeedingRollout return a list of machines that need to be rolled out.
func (c *ControlPlane) MachinesNeedingRollout() (collections.Machines, map[string]string) {
	// Ignore machines to be deleted.
	machines := c.Machines.Filter(collections.Not(collections.HasDeletionTimestamp))

	// Return machines if they are scheduled for rollout or if with an outdated configuration.
	machinesNeedingRollout := make(collections.Machines, len(machines))
	rolloutReasons := map[string]string{}
	for _, m := range machines {
		reason, needsRollout := NeedsRollout(&c.reconciliationTime, c.KCP.Spec.RolloutAfter, c.KCP.Spec.RolloutBefore, c.InfraResources, c.KubeadmConfigs, c.KCP, m)
		if needsRollout {
			machinesNeedingRollout.Insert(m)
			rolloutReasons[m.Name] = reason
		}
	}
	return machinesNeedingRollout, rolloutReasons
}

// UpToDateMachines returns the machines that are up to date with the control
// plane's configuration and therefore do not require rollout.
func (c *ControlPlane) UpToDateMachines() collections.Machines {
	upToDateMachines := make(collections.Machines, len(c.Machines))
	for _, m := range c.Machines {
		_, needsRollout := NeedsRollout(&c.reconciliationTime, c.KCP.Spec.RolloutAfter, c.KCP.Spec.RolloutBefore, c.InfraResources, c.KubeadmConfigs, c.KCP, m)
		if !needsRollout {
			upToDateMachines.Insert(m)
		}
	}
	return upToDateMachines
}

// getInfraResources fetches the external infrastructure resource for each machine in the collection and returns a map of machine.Name -> infraResource.
func getInfraResources(ctx context.Context, cl client.Client, machines collections.Machines) (map[string]*unstructured.Unstructured, error) {
	result := map[string]*unstructured.Unstructured{}
	for _, m := range machines {
		infraObj, err := external.Get(ctx, cl, &m.Spec.InfrastructureRef, m.Namespace)
		if err != nil {
			if apierrors.IsNotFound(errors.Cause(err)) {
				continue
			}
			return nil, errors.Wrapf(err, "failed to retrieve infra obj for machine %q", m.Name)
		}
		result[m.Name] = infraObj
	}
	return result, nil
}

// getKubeadmConfigs fetches the kubeadm config for each machine in the collection and returns a map of machine.Name -> KubeadmConfig.
func getKubeadmConfigs(ctx context.Context, cl client.Client, machines collections.Machines) (map[string]*bootstrapv1.KubeadmConfig, error) {
	result := map[string]*bootstrapv1.KubeadmConfig{}
	for _, m := range machines {
		bootstrapRef := m.Spec.Bootstrap.ConfigRef
		if bootstrapRef == nil {
			continue
		}
		machineConfig := &bootstrapv1.KubeadmConfig{}
		if err := cl.Get(ctx, client.ObjectKey{Name: bootstrapRef.Name, Namespace: m.Namespace}, machineConfig); err != nil {
			if apierrors.IsNotFound(errors.Cause(err)) {
				continue
			}
			return nil, errors.Wrapf(err, "failed to retrieve bootstrap config for machine %q", m.Name)
		}
		result[m.Name] = machineConfig
	}
	return result, nil
}

// IsEtcdManaged returns true if the control plane relies on a managed etcd.
func (c *ControlPlane) IsEtcdManaged() bool {
	return c.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration == nil || c.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.External == nil
}

// UnhealthyMachinesWithNonMHCUnhealthyCondition returns all unhealthy control plane machines that
// does not have a Kubernetes node or have any control plane component condition set to False.
// It is different from the UnhealthyMachines func which checks MachineHealthCheck conditions.
func (c *ControlPlane) UnhealthyMachinesWithNonMHCUnhealthyCondition(machines collections.Machines) collections.Machines {
	return machines.Filter(collections.HasUnhealthyControlPlaneComponentCondition(c.IsEtcdManaged()))
}

// UnhealthyMachines returns the list of control plane machines marked as unhealthy by MHC.
func (c *ControlPlane) UnhealthyMachines() collections.Machines {
	return c.Machines.Filter(collections.HasUnhealthyCondition)
}

// HealthyMachines returns the list of control plane machines not marked as unhealthy by MHC.
func (c *ControlPlane) HealthyMachines() collections.Machines {
	return c.Machines.Filter(collections.Not(collections.HasUnhealthyCondition))
}

// HasUnhealthyMachine returns true if any machine in the control plane is marked as unhealthy by MHC.
func (c *ControlPlane) HasUnhealthyMachine() bool {
	return len(c.UnhealthyMachines()) > 0
}

// HasHealthyMachineStillProvisioning returns true if any healthy machine in the control plane is still in the process of being provisioned.
func (c *ControlPlane) HasHealthyMachineStillProvisioning() bool {
	return len(c.HealthyMachines().Filter(collections.Not(collections.HasNode()))) > 0
}

// PatchMachines patches all the machines conditions.
func (c *ControlPlane) PatchMachines(ctx context.Context) error {
	errList := []error{}
	for i := range c.Machines {
		machine := c.Machines[i]
		if helper, ok := c.machinesPatchHelpers[machine.Name]; ok {
			if err := helper.Patch(ctx, machine, patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
				controlplanev1.MachineAPIServerPodHealthyCondition,
				controlplanev1.MachineControllerManagerPodHealthyCondition,
				controlplanev1.MachineSchedulerPodHealthyCondition,
				controlplanev1.MachineEtcdPodHealthyCondition,
				controlplanev1.MachineEtcdMemberHealthyCondition,
			}}); err != nil {
				errList = append(errList, err)
			}
			continue
		}
		errList = append(errList, errors.Errorf("failed to get patch helper for machine %s", machine.Name))
	}
	return kerrors.NewAggregate(errList)
}

// SetPatchHelpers updates the patch helpers.
func (c *ControlPlane) SetPatchHelpers(patchHelpers map[string]*patch.Helper) {
	if c.machinesPatchHelpers == nil {
		c.machinesPatchHelpers = map[string]*patch.Helper{}
	}
	for machineName, patchHelper := range patchHelpers {
		c.machinesPatchHelpers[machineName] = patchHelper
	}
}

// GetWorkloadCluster builds a cluster object.
// The cluster comes with an etcd client generator to connect to any etcd pod living on a managed machine.
func (c *ControlPlane) GetWorkloadCluster(ctx context.Context) (WorkloadCluster, error) {
	if c.workloadCluster != nil {
		return c.workloadCluster, nil
	}

	workloadCluster, err := c.managementCluster.GetWorkloadCluster(ctx, client.ObjectKeyFromObject(c.Cluster))
	if err != nil {
		return nil, err
	}
	c.workloadCluster = workloadCluster
	return c.workloadCluster, nil
}

// InjectTestManagementCluster allows to inject a test ManagementCluster during tests.
// NOTE: This approach allows to keep the managementCluster field private, which will
// prevent people from using managementCluster.GetWorkloadCluster because it creates a new
// instance of WorkloadCluster at every call. People instead should use ControlPlane.GetWorkloadCluster
// that creates only a single instance of WorkloadCluster for each reconcile.
func (c *ControlPlane) InjectTestManagementCluster(managementCluster ManagementCluster) {
	c.managementCluster = managementCluster
	c.workloadCluster = nil
}
