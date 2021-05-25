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

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/klog/v2/klogr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha4"
	"sigs.k8s.io/cluster-api/controllers/external"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha4"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/failuredomains"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Log is the global logger for the internal package.
var Log = klogr.New()

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
	kubeadmConfigs map[string]*bootstrapv1.KubeadmConfig
	infraResources map[string]*unstructured.Unstructured
}

// NewControlPlane returns an instantiated ControlPlane.
func NewControlPlane(ctx context.Context, client client.Client, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane, ownedMachines collections.Machines) (*ControlPlane, error) {
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
			return nil, errors.Wrapf(err, "failed to create patch helper for machine %s", machine.Name)
		}
		patchHelpers[machine.Name] = patchHelper
	}

	return &ControlPlane{
		KCP:                  kcp,
		Cluster:              cluster,
		Machines:             ownedMachines,
		machinesPatchHelpers: patchHelpers,
		kubeadmConfigs:       kubeadmConfigs,
		infraResources:       infraObjects,
		reconciliationTime:   metav1.Now(),
	}, nil
}

// Logger returns a logger with useful context.
func (c *ControlPlane) Logger() logr.Logger {
	return Log.WithValues("namespace", c.KCP.Namespace, "name", c.KCP.Name, "cluster-name", c.Cluster.Name)
}

// FailureDomains returns a slice of failure domain objects synced from the infrastructure provider into Cluster.Status.
func (c *ControlPlane) FailureDomains() clusterv1.FailureDomains {
	if c.Cluster.Status.FailureDomains == nil {
		return clusterv1.FailureDomains{}
	}
	return c.Cluster.Status.FailureDomains
}

// Version returns the KubeadmControlPlane's version.
func (c *ControlPlane) Version() *string {
	return &c.KCP.Spec.Version
}

// MachineInfrastructureTemplateRef returns the KubeadmControlPlane's infrastructure template for Machines.
func (c *ControlPlane) MachineInfrastructureTemplateRef() *corev1.ObjectReference {
	return &c.KCP.Spec.MachineTemplate.InfrastructureRef
}

// AsOwnerReference returns an owner reference to the KubeadmControlPlane.
func (c *ControlPlane) AsOwnerReference() *metav1.OwnerReference {
	return &metav1.OwnerReference{
		APIVersion: controlplanev1.GroupVersion.String(),
		Kind:       "KubeadmControlPlane",
		Name:       c.KCP.Name,
		UID:        c.KCP.UID,
	}
}

// EtcdImageData returns the etcd image data embedded in the ClusterConfiguration or empty strings if none are defined.
func (c *ControlPlane) EtcdImageData() (string, string) {
	if c.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration != nil && c.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local != nil {
		meta := c.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local.ImageMeta
		return meta.ImageRepository, meta.ImageTag
	}
	return "", ""
}

// MachineInFailureDomainWithMostMachines returns the first matching failure domain with machines that has the most control-plane machines on it.
func (c *ControlPlane) MachineInFailureDomainWithMostMachines(machines collections.Machines) (*clusterv1.Machine, error) {
	fd := c.FailureDomainWithMostMachines(machines)
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
func (c *ControlPlane) FailureDomainWithMostMachines(machines collections.Machines) *string {
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
	return failuredomains.PickMost(c.Cluster.Status.FailureDomains.FilterControlPlane(), c.Machines, machines)
}

// NextFailureDomainForScaleUp returns the failure domain with the fewest number of up-to-date machines.
func (c *ControlPlane) NextFailureDomainForScaleUp() *string {
	if len(c.Cluster.Status.FailureDomains.FilterControlPlane()) == 0 {
		return nil
	}
	return failuredomains.PickFewest(c.FailureDomains().FilterControlPlane(), c.UpToDateMachines())
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

// GenerateKubeadmConfig generates a new kubeadm config for creating new control plane nodes.
func (c *ControlPlane) GenerateKubeadmConfig(spec *bootstrapv1.KubeadmConfigSpec) *bootstrapv1.KubeadmConfig {
	// Create an owner reference without a controller reference because the owning controller is the machine controller
	owner := metav1.OwnerReference{
		APIVersion: controlplanev1.GroupVersion.String(),
		Kind:       "KubeadmControlPlane",
		Name:       c.KCP.Name,
		UID:        c.KCP.UID,
	}

	bootstrapConfig := &bootstrapv1.KubeadmConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:            names.SimpleNameGenerator.GenerateName(c.KCP.Name + "-"),
			Namespace:       c.KCP.Namespace,
			Labels:          ControlPlaneMachineLabelsForCluster(c.KCP, c.Cluster.Name),
			Annotations:     c.KCP.Spec.MachineTemplate.ObjectMeta.Annotations,
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: *spec,
	}
	return bootstrapConfig
}

// NewMachine returns a machine configured to be a part of the control plane.
func (c *ControlPlane) NewMachine(infraRef, bootstrapRef *corev1.ObjectReference, failureDomain *string) *clusterv1.Machine {
	return &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        names.SimpleNameGenerator.GenerateName(c.KCP.Name + "-"),
			Namespace:   c.KCP.Namespace,
			Labels:      ControlPlaneMachineLabelsForCluster(c.KCP, c.Cluster.Name),
			Annotations: c.KCP.Spec.MachineTemplate.ObjectMeta.Annotations,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(c.KCP, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane")),
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName:       c.Cluster.Name,
			Version:           c.Version(),
			InfrastructureRef: *infraRef,
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: bootstrapRef,
			},
			FailureDomain: failureDomain,
		},
	}
}

// NeedsReplacementNode determines if the control plane needs to create a replacement node during upgrade.
func (c *ControlPlane) NeedsReplacementNode() bool {
	// Can't do anything with an unknown number of desired replicas.
	if c.KCP.Spec.Replicas == nil {
		return false
	}
	// if the number of existing machines is exactly 1 > than the number of replicas.
	return len(c.Machines)+1 == int(*c.KCP.Spec.Replicas)
}

// HasDeletingMachine returns true if any machine in the control plane is in the process of being deleted.
func (c *ControlPlane) HasDeletingMachine() bool {
	return len(c.Machines.Filter(collections.HasDeletionTimestamp)) > 0
}

// MachinesNeedingRollout return a list of machines that need to be rolled out.
func (c *ControlPlane) MachinesNeedingRollout() collections.Machines {
	// Ignore machines to be deleted.
	machines := c.Machines.Filter(collections.Not(collections.HasDeletionTimestamp))

	// Return machines if they are scheduled for rollout or if with an outdated configuration.
	return machines.AnyFilter(
		// Machines that are scheduled for rollout (KCP.Spec.RolloutAfter set, the RolloutAfter deadline is expired, and the machine was created before the deadline).
		collections.ShouldRolloutAfter(&c.reconciliationTime, c.KCP.Spec.RolloutAfter),
		// Machines that do not match with KCP config.
		collections.Not(MatchesMachineSpec(c.infraResources, c.kubeadmConfigs, c.KCP)),
	)
}

// UpToDateMachines returns the machines that are up to date with the control
// plane's configuration and therefore do not require rollout.
func (c *ControlPlane) UpToDateMachines() collections.Machines {
	return c.Machines.Difference(c.MachinesNeedingRollout())
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
				errList = append(errList, errors.Wrapf(err, "failed to patch machine %s", machine.Name))
			}
			continue
		}
		errList = append(errList, errors.Errorf("failed to get patch helper for machine %s", machine.Name))
	}
	return kerrors.NewAggregate(errList)
}
