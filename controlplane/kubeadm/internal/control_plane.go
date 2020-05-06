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
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/storage/names"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/hash"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/machinefilters"
)

// MachineHealthCheck remediation is only supported on clusters with >= 3 machines to avoid disrupting etcd consensus
const minimumClusterSizeForRemediation = 3

// ControlPlane holds business logic around control planes.
// It should never need to connect to a service, that responsibility lies outside of this struct.
type ControlPlane struct {
	KCP      *controlplanev1.KubeadmControlPlane
	Cluster  *clusterv1.Cluster
	Machines FilterableMachineCollection
	Logger   logr.Logger
}

// NewControlPlane returns an instantiated ControlPlane.
func NewControlPlane(cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane, ownedMachines FilterableMachineCollection, logger logr.Logger) *ControlPlane {
	return &ControlPlane{
		KCP:      kcp,
		Cluster:  cluster,
		Machines: ownedMachines,
		Logger:   logger,
	}
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

// InfrastructureTemplate returns the KubeadmControlPlane's infrastructure template.
func (c *ControlPlane) InfrastructureTemplate() *corev1.ObjectReference {
	return &c.KCP.Spec.InfrastructureTemplate
}

// SpecHash returns the hash of the KubeadmControlPlane spec.
func (c *ControlPlane) SpecHash() string {
	return hash.Compute(&c.KCP.Spec)
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

// MachinesNeedingUpgrade return a list of machines that need to be upgraded.
func (c *ControlPlane) MachinesNeedingUpgrade() FilterableMachineCollection {
	now := metav1.Now()
	if c.KCP.Spec.UpgradeAfter != nil && c.KCP.Spec.UpgradeAfter.Before(&now) {
		return c.Machines.AnyFilter(
			machinefilters.Not(machinefilters.MatchesConfigurationHash(c.SpecHash())),
			machinefilters.OlderThan(c.KCP.Spec.UpgradeAfter),
		)
	}

	return c.Machines.Filter(
		machinefilters.Not(machinefilters.MatchesConfigurationHash(c.SpecHash())),
	)
}

// MachineInFailureDomainWithMostMachines returns the first matching failure domain with machines that has the most control-plane machines on it.
func (c *ControlPlane) MachineInFailureDomainWithMostMachines(machines FilterableMachineCollection) *clusterv1.Machine {
	fd := c.FailureDomainWithMostMachines(machines)
	machinesInFailureDomain := machines.Filter(machinefilters.InFailureDomains(fd))
	return machinesInFailureDomain.Oldest()
}

// FailureDomainWithMostMachines returns a fd which exists both in machines and control-plane machines and has the most
// control-plane machines on it.
func (c *ControlPlane) FailureDomainWithMostMachines(machines FilterableMachineCollection) *string {
	// See if there are any Machines that are not in currently defined failure domains first.
	notInFailureDomains := machines.Filter(
		machinefilters.Not(machinefilters.InFailureDomains(c.FailureDomains().FilterControlPlane().GetIDs()...)),
	)
	if len(notInFailureDomains) > 0 {
		// return the failure domain for the oldest Machine not in the current list of failure domains
		// this could be either nil (no failure domain defined) or a failure domain that is no longer defined
		// in the cluster status.
		return notInFailureDomains.Oldest().Spec.FailureDomain
	}

	return PickMost(c, machines)
}

// FailureDomainWithFewestMachines returns the failure domain with the fewest number of machines.
// Used when scaling up.
func (c *ControlPlane) FailureDomainWithFewestMachines() *string {
	if len(c.Cluster.Status.FailureDomains.FilterControlPlane()) == 0 {
		return nil
	}
	return PickFewest(c.FailureDomains().FilterControlPlane(), c.Machines)
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
	bootstrapSpec.ClusterConfiguration = nil
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
			Labels:          ControlPlaneLabelsForClusterWithHash(c.Cluster.Name, c.SpecHash()),
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
			Name:      names.SimpleNameGenerator.GenerateName(c.KCP.Name + "-"),
			Namespace: c.KCP.Namespace,
			Labels:    ControlPlaneLabelsForClusterWithHash(c.Cluster.Name, c.SpecHash()),
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

// HasDeletingMachine returns true if any machine in the control plane is in
// the process of being deleted.
func (c *ControlPlane) DeletingMachines() FilterableMachineCollection {
	return c.Machines.Filter(machinefilters.HasDeletionTimestamp)
}

// ProvisioningMachines returns machines that are still booting.  In the case
// of 3 node or larger clusters, it excludes unhealthy machines.
func (c *ControlPlane) ProvisioningMachines() FilterableMachineCollection {
	machines := c.Machines.Filter(machinefilters.IsProvisioning).
		Filter(machinefilters.Not(machinefilters.IsFailed))

	if c.Machines.Len() < minimumClusterSizeForRemediation {
		return machines
	}
	return machines.Filter(machinefilters.Not(machinefilters.IsUnhealthy))
}

// ReadyMachines returns machines that are not provisioning or failed.  In the
// case of 3 node or larger clusters, it excludes unhealthy machines.
func (c *ControlPlane) ReadyMachines() FilterableMachineCollection {
	machines := c.Machines.
		Filter(machinefilters.Not(machinefilters.IsProvisioning)).
		Filter(machinefilters.Not(machinefilters.IsFailed))

	if c.Machines.Len() < minimumClusterSizeForRemediation {
		return machines
	}
	return machines.Filter(machinefilters.Not(machinefilters.IsUnhealthy))
}

// UnhealthyMachines returns the machines with the MachineHealthCheck unhealthy
// annotation.  If cluster size is less than 3, will return an empty list.
func (c *ControlPlane) UnhealthyMachines() FilterableMachineCollection {
	if c.Machines.Len() < minimumClusterSizeForRemediation {
		return nil
	}
	return c.Machines.Filter(machinefilters.IsUnhealthy)
}

// FailedMachines returns the machines with a FailureMessage or FailureReason.
func (c *ControlPlane) FailedMachines() FilterableMachineCollection {
	return c.Machines.Filter(machinefilters.IsFailed)
}

// NeedsInitialization returns whether the control plane has yet to create the
// first machine.  Returns false in the case that zero replicas are desired.
func (c *ControlPlane) NeedsInitialization() bool {
	return c.Machines.Len() == 0 && *c.KCP.Spec.Replicas > 0
}

// NeedsScaleDown returns whether the control plane needs to remove a machine
// from the cluster.  Scale down can be caused by the presence of unhealthy
// machines or when there are more machines than desired replicas.
func (c *ControlPlane) NeedsScaleDown() bool {
	if c.UnhealthyMachines().Any() {
		c.Logger.Info("Scale down is required because unhealthy machines were found", "machines", c.UnhealthyMachines().Names())
		return true
	}
	if c.Machines.Len() > int(*c.KCP.Spec.Replicas) {
		c.Logger.Info("Scale down is required", "existing", c.Machines.Len(), "desired", int(*c.KCP.Spec.Replicas))
		return true
	}
	return false
}

// NeedsScaleUp returns whether the control plane needs to add a machine to the
// cluster.  Scale up can be caused by an upgrade, where the cluster will
// replace outdated machines one by one, or by the more common case of having
// fewer machines than the number of desired replicas.
func (c *ControlPlane) NeedsScaleUp() bool {
	if c.MachinesNeedingUpgrade().Any() && !c.NeedsScaleDown() {
		c.Logger.Info("Scale up is required for upgrade")
		return true
	}
	if c.Machines.Len() < int(*c.KCP.Spec.Replicas) {
		c.Logger.Info("Scale up is required", "desired", int(*c.KCP.Spec.Replicas), "existing", c.Machines.Len())
		return true
	}
	return false
}

// NextMachineForScaleDown returns the next machine that should be selected for removal.
// It will first recommend the oldest unhealthy machine, if there are any.
// It then will recommend the oldest outdated machine in the failure domain with the most outdated machines, if there are any.
// Lastly it will recommend the oldest machine in the largest failure domain.
func (c *ControlPlane) NextMachineForScaleDown() *clusterv1.Machine {
	if c.UnhealthyMachines().Any() {
		machine := c.UnhealthyMachines().Oldest()
		c.Logger.Info("Scaling down unhealthy machine", "machine", machine.Name)
		return machine
	}
	if c.MachinesNeedingUpgrade().Any() {
		machine := c.MachineInFailureDomainWithMostMachines(c.MachinesNeedingUpgrade())
		c.Logger.Info("Scaling down outdated machine", "machine", machine.Name)
		return machine
	}
	machine := c.MachineInFailureDomainWithMostMachines(c.Machines)
	c.Logger.Info("Scaling down unneeded machine", "machine", machine.Name)
	return machine
}
