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
	"fmt"
	"sort"
	"strings"

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
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
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

	machinesNotUptoDate                  collections.Machines
	machinesNotUptoDateLogMessages       map[string][]string
	machinesNotUptoDateConditionMessages map[string][]string

	// reconciliationTime is the time of the current reconciliation, and should be used for all "now" calculations
	reconciliationTime metav1.Time

	// InfraMachineTemplateIsNotFound is true if getting the infra machine template object failed with an NotFound err
	InfraMachineTemplateIsNotFound bool

	// PreflightChecks contains description about pre flight check results blocking machines creation or deletion.
	PreflightCheckResults PreflightCheckResults

	// TODO: we should see if we can combine these with the Machine objects so we don't have all these separate lookups
	// See discussion on https://github.com/kubernetes-sigs/cluster-api/pull/3405
	KubeadmConfigs map[string]*bootstrapv1.KubeadmConfig
	InfraResources map[string]*unstructured.Unstructured

	// EtcdMembers is the list of members read while computing reconcileControlPlaneConditions; also additional info below
	// comes from the same func.
	// NOTE: Those info are computed based on the info KCP was able to collect during inspection (e.g. if on a 3 CP
	// control plane one etcd member is down, those info are based on the answer collected from two members only).
	// NOTE: Those info are specifically designed for computing KCP's Available condition.
	EtcdMembers                       []*etcd.Member
	EtcdMembersAgreeOnMemberList      bool
	EtcdMembersAgreeOnClusterID       bool
	EtcdMembersAndMachinesAreMatching bool

	managementCluster ManagementCluster
	workloadCluster   WorkloadCluster

	// deletingReason is the reason that should be used when setting the Deleting condition.
	DeletingReason string

	// deletingMessage is the message that should be used when setting the Deleting condition.
	DeletingMessage string
}

// PreflightCheckResults contains description about pre flight check results blocking machines creation or deletion.
type PreflightCheckResults struct {
	// HasDeletingMachine reports true if preflight check detected a deleting machine.
	HasDeletingMachine bool
	// ControlPlaneComponentsNotHealthy reports true if preflight check detected that the control plane components are not fully healthy.
	ControlPlaneComponentsNotHealthy bool
	// EtcdClusterNotHealthy reports true if preflight check detected that the etcd cluster is not fully healthy.
	EtcdClusterNotHealthy bool
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

	// Select machines that should be rolled out because of an outdated configuration or because rolloutAfter/Before expired.
	reconciliationTime := metav1.Now()
	machinesNotUptoDate := make(collections.Machines, len(ownedMachines))
	machinesNotUptoDateLogMessages := map[string][]string{}
	machinesNotUptoDateConditionMessages := map[string][]string{}
	for _, m := range ownedMachines {
		upToDate, logMessages, conditionMessages, err := UpToDate(m, kcp, &reconciliationTime, infraObjects, kubeadmConfigs)
		if err != nil {
			return nil, err
		}
		if !upToDate {
			machinesNotUptoDate.Insert(m)
			machinesNotUptoDateLogMessages[m.Name] = logMessages
			machinesNotUptoDateConditionMessages[m.Name] = conditionMessages
		}
	}

	return &ControlPlane{
		KCP:                                  kcp,
		Cluster:                              cluster,
		Machines:                             ownedMachines,
		machinesPatchHelpers:                 patchHelpers,
		machinesNotUptoDate:                  machinesNotUptoDate,
		machinesNotUptoDateLogMessages:       machinesNotUptoDateLogMessages,
		machinesNotUptoDateConditionMessages: machinesNotUptoDateConditionMessages,
		KubeadmConfigs:                       kubeadmConfigs,
		InfraResources:                       infraObjects,
		reconciliationTime:                   reconciliationTime,
		managementCluster:                    managementCluster,
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
// Note: if there are eligibleMachines machines in failure domain that do not exists anymore, getting rid of those machines take precedence.
func (c *ControlPlane) MachineInFailureDomainWithMostMachines(ctx context.Context, eligibleMachines collections.Machines) (*clusterv1.Machine, error) {
	fd := c.FailureDomainWithMostMachines(ctx, eligibleMachines)
	machinesInFailureDomain := eligibleMachines.Filter(collections.InFailureDomains(fd))
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

// FailureDomainWithMostMachines returns the fd with most machines in it and at least one eligible machine in it.
// Note: if there are eligibleMachines machines in failure domain that do not exist anymore, cleaning up those failure domains takes precedence.
func (c *ControlPlane) FailureDomainWithMostMachines(ctx context.Context, eligibleMachines collections.Machines) *string {
	// See if there are any Machines that are not in currently defined failure domains first.
	notInFailureDomains := eligibleMachines.Filter(
		collections.Not(collections.InFailureDomains(c.FailureDomains().FilterControlPlane().GetIDs()...)),
	)
	if len(notInFailureDomains) > 0 {
		// return the failure domain for the oldest Machine not in the current list of failure domains
		// this could be either nil (no failure domain defined) or a failure domain that is no longer defined
		// in the cluster status.
		return notInFailureDomains.Oldest().Spec.FailureDomain
	}

	// Pick the failure domain with most machines in it and at least one eligible machine in it.
	return failuredomains.PickMost(ctx, c.Cluster.Status.FailureDomains.FilterControlPlane(), c.Machines, eligibleMachines)
}

// NextFailureDomainForScaleUp returns the failure domain with the fewest number of up-to-date, not deleted machines
// (the ultimate goal is to achieve ideal spreading of machines at stable state/when only up-to-date machines will exist).
//
// In case of tie (more failure domain with the same number of up-to-date, not deleted machines) the failure domain with the fewest number of
// machine overall is picked to ensure a better spreading of machines while the rollout is performed.
func (c *ControlPlane) NextFailureDomainForScaleUp(ctx context.Context) (*string, error) {
	if len(c.Cluster.Status.FailureDomains.FilterControlPlane()) == 0 {
		return nil, nil
	}
	return failuredomains.PickFewest(ctx, c.FailureDomains().FilterControlPlane(), c.Machines, c.UpToDateMachines().Filter(collections.Not(collections.HasDeletionTimestamp))), nil
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

// DeletingMachines returns machines in the control plane that are in the process of being deleted.
func (c *ControlPlane) DeletingMachines() collections.Machines {
	return c.Machines.Filter(collections.HasDeletionTimestamp)
}

// GetKubeadmConfig returns the KubeadmConfig of a given machine.
func (c *ControlPlane) GetKubeadmConfig(machineName string) (*bootstrapv1.KubeadmConfig, bool) {
	kubeadmConfig, ok := c.KubeadmConfigs[machineName]
	return kubeadmConfig, ok
}

// MachinesNeedingRollout return a list of machines that need to be rolled out.
func (c *ControlPlane) MachinesNeedingRollout() (collections.Machines, map[string][]string) {
	// Note: Machines already deleted are dropped because they will be replaced by new machines after deletion completes.
	return c.machinesNotUptoDate.Filter(collections.Not(collections.HasDeletionTimestamp)), c.machinesNotUptoDateLogMessages
}

// NotUpToDateMachines return a list of machines that are not up to date with the control
// plane's configuration.
func (c *ControlPlane) NotUpToDateMachines() (collections.Machines, map[string][]string) {
	return c.machinesNotUptoDate, c.machinesNotUptoDateConditionMessages
}

// UpToDateMachines returns the machines that are up to date with the control
// plane's configuration.
func (c *ControlPlane) UpToDateMachines() collections.Machines {
	return c.Machines.Difference(c.machinesNotUptoDate)
}

// getInfraResources fetches the external infrastructure resource for each machine in the collection and returns a map of machine.Name -> infraResource.
func getInfraResources(ctx context.Context, cl client.Client, machines collections.Machines) (map[string]*unstructured.Unstructured, error) {
	result := map[string]*unstructured.Unstructured{}
	for _, m := range machines {
		infraObj, err := external.Get(ctx, cl, &m.Spec.InfrastructureRef)
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

// UnhealthyMachinesWithUnhealthyControlPlaneComponents returns all unhealthy control plane machines that
// have unhealthy control plane components.
// It differs from UnhealthyMachinesByHealthCheck which checks `MachineHealthCheck` conditions.
func (c *ControlPlane) UnhealthyMachinesWithUnhealthyControlPlaneComponents(machines collections.Machines) collections.Machines {
	return machines.Filter(collections.HasUnhealthyControlPlaneComponents(c.IsEtcdManaged()))
}

// UnhealthyMachines returns the list of control plane machines marked as unhealthy by MHC, no matter
// if they are set to be remediated by KCP or not.
func (c *ControlPlane) UnhealthyMachines() collections.Machines {
	return c.Machines.Filter(collections.IsUnhealthy)
}

// HealthyMachines returns the list of control plane machines marked as healthy by MHC (or not targeted by any MHC instance).
func (c *ControlPlane) HealthyMachines() collections.Machines {
	return c.Machines.Filter(collections.Not(collections.IsUnhealthy))
}

// MachinesToBeRemediatedByKCP returns the list of control plane machines to be remediated by KCP.
func (c *ControlPlane) MachinesToBeRemediatedByKCP() collections.Machines {
	return c.Machines.Filter(collections.IsUnhealthyAndOwnerRemediated)
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
			}}, patch.WithOwnedV1Beta2Conditions{Conditions: []string{
				clusterv1.MachineUpToDateV1Beta2Condition,
				controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyV1Beta2Condition,
				controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyV1Beta2Condition,
				controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyV1Beta2Condition,
				controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyV1Beta2Condition,
				controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition,
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

// StatusToLogKeyAndValues returns the following key/value pairs describing the overall status of the control plane:
// - machines is the list of KCP machines; each machine might have additional notes surfacing
//   - if the machine has been created in the current reconcile
//   - if machines node ref is not yet set
//   - if the machine has been marked for remediation
//   - if there are unhealthy control plane component on the machine
//   - if the machine has a deletion timestamp/has been deleted in the current reconcile
//   - if the machine is not up to date with the KCP spec
//
// - etcdMembers list as reported by etcd.
func (c *ControlPlane) StatusToLogKeyAndValues(newMachine, deletedMachine *clusterv1.Machine) []any {
	controlPlaneMachineHealthConditions := []clusterv1.ConditionType{
		controlplanev1.MachineAPIServerPodHealthyCondition,
		controlplanev1.MachineControllerManagerPodHealthyCondition,
		controlplanev1.MachineSchedulerPodHealthyCondition,
	}
	if c.IsEtcdManaged() {
		controlPlaneMachineHealthConditions = append(controlPlaneMachineHealthConditions,
			controlplanev1.MachineEtcdPodHealthyCondition,
			controlplanev1.MachineEtcdMemberHealthyCondition,
		)
	}

	machines := []string{}
	for _, m := range c.Machines {
		notes := []string{}

		if m.Status.NodeRef == nil {
			notes = append(notes, "status.nodeRef not set")
		}

		if c.MachinesToBeRemediatedByKCP().Has(m) {
			notes = append(notes, "marked for remediation")
		}

		for _, condition := range controlPlaneMachineHealthConditions {
			if conditions.IsUnknown(m, condition) {
				notes = append(notes, strings.Replace(string(condition), "Healthy", " health unknown", -1))
			}
			if conditions.IsFalse(m, condition) {
				notes = append(notes, strings.Replace(string(condition), "Healthy", " not healthy", -1))
			}
		}

		if !c.UpToDateMachines().Has(m) {
			notes = append(notes, "not up-to-date")
		}

		if deletedMachine != nil && m.Name == deletedMachine.Name {
			notes = append(notes, "just deleted")
		} else if !m.DeletionTimestamp.IsZero() {
			notes = append(notes, "deleting")
		}

		name := m.Name
		if len(notes) > 0 {
			name = fmt.Sprintf("%s (%s)", name, strings.Join(notes, ", "))
		}
		machines = append(machines, name)
	}

	if newMachine != nil {
		machines = append(machines, fmt.Sprintf("%s (just created)", newMachine.Name))
	}
	sort.Strings(machines)

	etcdMembers := []string{}
	for _, m := range c.EtcdMembers {
		etcdMembers = append(etcdMembers, m.Name)
	}
	sort.Strings(etcdMembers)

	return []any{
		"machines", strings.Join(machines, ", "),
		"etcdMembers", strings.Join(etcdMembers, ", "),
	}
}
