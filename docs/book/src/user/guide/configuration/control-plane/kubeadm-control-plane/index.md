# Kubeadm Control Plane

Using the Kubeadm control plane type to manage a control plane provides several ways to upgrade control plane machines.

> [!IMPORTANT]
> KubeadmControlPlane is solely supporting CoreDNS as a DNS server at this time.

> [!IMPORTANT]
> KubeadmControlPlane does not automatically upgrade CoreDNS when the Kubernetes version is upgraded, unlike
> `kubeadm upgrade`. CoreDNS is only upgraded if you explicitly set the target version, see
> [How to upgrade CoreDNS][upgrade-coredns].

### Kubeconfig Management

KCP will generate and manage the admin Kubeconfig for clusters. The client certificate for the admin user is created
with a valid lifespan of a year, and will be automatically regenerated when the cluster is reconciled and has less than
6 months of validity remaining.

### Upgrades

See [Upgrade the Kubernetes version].

### Running Workloads on Control Plane Machines

We don't suggest running workloads on control planes, and highly encourage avoiding it unless absolutely necessary.

However, in the case the user wants to run non-control plane workloads on control plane machines they
are ultimately responsible for ensuring the proper functioning of those workloads, given that KCP is not
aware of the specific requirements for each type of workload (e.g. preserving quorum, shutdown procedures etc.).

In order to do so, the user could leverage on the same assumption that applies to all the
Cluster API Machines:

- The Kubernetes node hosted on the Machine will be cordoned & drained before removal (with well
  known exceptions like full Cluster deletion).
- The Machine will respect PreDrainDeleteHook and PreTerminateDeleteHook. see the
  [Machine Deletion Phase Hooks proposal](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20200602-machine-deletion-phase-hooks.md)
  for additional details.

### In-place Propagation
Changes to the following fields of KubeadmControlPlane are propagated in-place to the Machines and do not trigger a full rollout:
- `.spec.machineTemplate.metadata.labels`
- `.spec.machineTemplate.metadata.annotations`
- `.spec.nodeDrainTimeout`
- `.spec.nodeDeletionTimeout`
- `.spec.nodeVolumeDetachTimeout`

Changes to the following fields of KubeadmControlPlane are propagated in-place to the InfrastructureMachine and KubeadmConfig:
- `.spec.machineTemplate.metadata.labels`
- `.spec.machineTemplate.metadata.annotations`

Note: Changes to these fields will not be propagated to Machines, InfraMachines and KubeadmConfigs that are marked for deletion (example: because of scale down).

<!-- links -->
[Upgrade the Kubernetes version]: ../../../lifecycle/upgrades.md#control-plane
[upgrade-coredns]: ../../../lifecycle/upgrades.md#how-to-upgrade-coredns
