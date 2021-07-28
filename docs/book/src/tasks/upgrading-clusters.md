# Upgrading management and workload clusters

## Considerations

### Supported versions of Kubernetes

If you are upgrading the version of Kubernetes for a cluster managed by Cluster API, check that the [running version of
Cluster API on the Management Cluster supports the target Kubernetes version](../reference/versions.md).

You may need to [upgrade the version of Cluster API](upgrading-cluster-api-versions.md) in order to support the target
Kubernetes version.

In addition, you must always upgrade between Kubernetes minor versions in sequence, e.g. if you need to upgrade from
Kubernetes v1.17 to v1.19, you must first upgrade to v1.18.

### Images

For kubeadm based clusters, infrastructure providers require a "machine image" containing pre-installed, matching
versions of `kubeadm` and `kubelet`, ensure that relevant infrastructure machine templates reference the appropriate
image for the Kubernetes version.

## Upgrading using Cluster API

The high level steps to fully upgrading a cluster are to first upgrade the control plane and then upgrade
the worker machines.

### Upgrading the control plane machines

#### How to upgrade the underlying machine image

To upgrade the control plane machines underlying machine images, the `MachineTemplate` resource referenced by the
`KubeadmControlPlane` must be changed. Since `MachineTemplate` resources are immutable, the recommended approach is to

1. Copy the existing `MachineTemplate`.
2. Modify the values that need changing, such as instance type or image ID.
3. Create the new `MachineTemplate` on the management cluster.
4. Modify the existing `KubeadmControlPlane` resource to reference the new `MachineTemplate` resource in the `infrastructureRef` field.

The next step will trigger a rolling update of the control plane using the new values found in the new `MachineTemplate`.

#### How to upgrade the Kubernetes control plane version

To upgrade the Kubernetes control plane version make a modification to the `KubeadmControlPlane` resource's `Spec.Version` field. This will trigger a rolling upgrade of the control plane and, depending on the provider, also upgrade the underlying machine image.

Some infrastructure providers, such as [AWS](https://github.com/kubernetes-sigs/cluster-api-provider-aws), require
that if a specific machine image is specified, it has to match the Kubernetes version specified in the
`KubeadmControlPlane` spec. In order to only trigger a single upgrade, the new `MachineTemplate` should be created first
and then both the `Version` and `InfrastructureTemplate` should be modified in a single transaction.

#### How to schedule a machine rollout

A `KubeadmControlPlane` resource has a field `RolloutAfter` that can be set to a timestamp
(RFC-3339) after which a rollout should be triggered regardless of whether there were any changes
to the `KubeadmControlPlane.Spec` or not. This would roll out replacement control plane nodes
which can be useful e.g. to perform certificate rotation, reflect changes to machine templates,
move to new machines, etc.

Note that this field can only be used for triggering a rollout, not for delaying one. Specifically,
a rollout can also happen before the time specified in `RolloutAfter` if any changes are made to
the spec before that time.

To do the same for machines managed by a `MachineDeployment` it's enough to make an arbitrary
change to its `Spec.Template`, one common approach is to run:

``` shell
clusterctl alpha rollout restart machinedeployment/my-md-0
```

This will modify the template by setting an `cluster.x-k8s.io/restartedAt` annotation which will
trigger a rollout.

### Upgrading machines managed by a `MachineDeployment`

Upgrades are not limited to just the control plane. This section is not related to Kubeadm control plane specifically,
but is the final step in fully upgrading a Cluster API managed cluster.

It is recommended to manage machines with one or more `MachineDeployment`s. `MachineDeployment`s will
transparently manage `MachineSet`s and `Machine`s to allow for a seamless scaling experience. A modification to the
`MachineDeployment`s spec will begin a rolling update of the machines. Follow
[these instructions](updating-machine-templates.md) for changing the
template for an existing `MachineDeployment`.

`MachineDeployment`s support different strategies for rolling out changes to `Machines`:

- RollingUpdate

Changes are rolled out by honouring `MaxUnavailable` and `MaxSurge` values.
Only values allowed are of type Int or Strings with an integer and percentage symbol e.g "5%".

- OnDelete

Changes are rolled out driven by the user or any entity deleting the old `Machines`. Only when a `Machine` is fully deleted a new one will come up.

For a more in-depth look at how `MachineDeployments` manage scaling events, take a look at the [`MachineDeployment`
controller documentation](../developer/architecture/controllers/machine-deployment.md) and the [`MachineSet` controller
documentation](../developer/architecture/controllers/machine-set.md).
