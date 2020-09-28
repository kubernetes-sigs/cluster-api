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

### Upgrading machines managed by a `MachineDeployment`

Upgrades are not limited to just the control plane. This section is not related to Kubeadm control plane specifically,
but is the final step in fully upgrading a Cluster API managed cluster.

It is recommended to manage machines with one or more `MachineDeployment`s. `MachineDeployment`s will
transparently manage `MachineSet`s and `Machine`s to allow for a seamless scaling experience. A modification to the
`MachineDeployment`s spec will begin a rolling update of the machines. Follow
[these instructions](./change-machine-template.md) for changing the
template for an existing `MachineDeployment`.

For a more in-depth look at how `MachineDeployments` manage scaling events, take a look at the [`MachineDeployment`
controller documentation](../developer/architecture/controllers/machine-deployment.md) and the [`MachineSet` controller
documentation](../developer/architecture/controllers/machine-set.md).
