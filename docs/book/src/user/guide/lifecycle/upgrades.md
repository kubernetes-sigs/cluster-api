# Upgrading the Kubernetes Version

When the change to the Cluster configuration includes the change of the Kubernetes version, the
operation is referred to as an upgrade or Cluster upgrade.

At a high level, upgrades are performed in the same way as other changes to the Cluster configuration,
but there is a notable difference that derives from the fact that upgrades must comply with the
[Kubernetes version skew policy].

In practice:
- The control plane should always be upgraded before workers.
- For the control plane you must always upgrade between Kubernetes minor versions in sequence, e.g. if
  you need to upgrade from Kubernetes v1.37 to v1.39, you must first upgrade to v1.38.
- It is possible to skip versions for worker nodes, but they should remain within the allowed version skew from the control plane (n-3).

> [!IMPORTANT]
> if you are not using [ClusterClass], and thus managing directly different resources like `ControlPlane`, `MachineDeployment`
> or `MachinePool`, it is also your responsibility to ensure that changes to the Kubernetes version are performed
> respecting the [Kubernetes version skew policy].

In addition to compliance to the [Kubernetes version skew policy], you should also consider the [Kubernetes version support matrix]
for Cluster API itself and for the providers you are using.

As a consequence, you may need to [upgrade the version of Cluster API] in order
to support the target Kubernetes version.

## Control Plane

Similar to rollouts, when upgrading the control plane you should first prepare a new `InfrastructureTemplate` (if required)
and then perform the following changes _in a single operation_:

- Change the `ControlPlane` resource; to upgrade the Kubernetes version for the `ControlPlane` resource
  you should change at least the `spec.version` field.
- If required, change the bootstrap configuration setting embedded in the `ControlPlane` resource.
  e.g. in the `KubeadmControlPlane` you should change fields under `kubeadmConfigSpec`. See e.g. [How to upgrade CoreDNS](#how-to-upgrade-coredns).
- If required, modify the existing `ControlPlane` resource to reference the new `InfrastructureTemplate` by
  changing the `spec.machineTemplate.spec.infrastructureRef` field.

e.g.

```diff
---
apiVersion: controlplane.cluster.x-k8s.io/v1beta2
kind: KubeadmControlPlane
metadata:
  name: my-cluster-controlplane
spec:
  ...
  replicas: 3
+ version: v1.35.0
- version: v1.34.0
  kubeadmConfigSpec:
    clusterConfiguration:
      dns:
+       imageTag: v1.14.6
-       imageTag: v1.14.0
  machineTemplate:
    spec:
      infrastructureRef:
        apiGroup: infrastructure.cluster.x-k8s.io
        kind: VSphereMachineTemplate
+       name: my-cluster-control-plane-machine-infrastructure-v1.35.0
-       name: my-cluster-control-plane-machine-infrastructure-v1.34.0
```

Once all the changes above are submitted, Cluster API will trigger a rollout of the machines controlled by the control plane.

> [!TIP]
> If you are using [ClusterClass], you can change the control plane and the corresponding infrastructure configuration and
> bootstrap configuration with a single change to the Cluster resource.

### How to Upgrade CoreDNS

> [!NOTE]
> This paragraph applies only to `KubeadmControlPlane` resources

Unlike a plain `kubeadm upgrade`, the Kubeadm Control Plane provider does
**not** automatically upgrade CoreDNS to a new default version as part of a
Kubernetes upgrade. CoreDNS is only reconciled by KCP when the `imageTag` (and
optionally `imageRepository`) fields are explicitly set under
`KubeadmControlPlane.spec.kubeadmConfigSpec.clusterConfiguration.dns`. If left
unset, KCP leaves the currently deployed CoreDNS version untouched, even while
the rest of the control plane is upgraded.

See [CoreDNS Support](../../../reference/versions.md#coredns-support) for how to
determine the maximum CoreDNS version supported by a given Cluster API release.
If you'd rather manage CoreDNS yourself, or with another tool, you can have KCP
skip reconciling it entirely by adding the
[`controlplane.cluster.x-k8s.io/skip-coredns`](../../../reference/api/labels-and-annotations.md)
annotation to the `KubeadmControlPlane` resource.

For more context and discussion about this behavior, see
[kubernetes-sigs/cluster-api#6429](https://github.com/kubernetes-sigs/cluster-api/issues/6429).

> [!TIP]
> If you are using [ClusterClass], the ClusterClass author could take care of automating the
> CoreDNS upgrade for you.

## Workers

Similar to rollouts, when upgrading the `MachineDeployment` you should first prepare a new `InfrastructureTemplate`
and/or a new `BootstrapConfigTemplate` (if required) and then perform the following changes _in a single operation_:

- Edit the `MachineDeployment` resource. e.g. to upgrade the Kubernetes version for the `MachineDeployment` resource
  you should change the `spec.template.spec.version` field.
- If required, modify the existing `MachineDeployment` resource to reference the new `BootstrapConfigTemplate` by
  changing the `spec.template.spec.bootstrap.configRef` field.
- If required, modify the existing `MachineDeployment` resource to reference the new `InfrastructureTemplate` by
  changing the `spec.template.spec.infrastructureRef` field.

e.g.

```diff
---
apiVersion: cluster.x-k8s.io/v1beta2
kind: MachineDeployment
metadata:
  name: my-cluster-md-0
spec:
  ...
  template:
    spec:
+     version: v1.35.0
-     version: v1.34.0
      bootstrap:
        configRef:
          apiGroup: bootstrap.cluster.x-k8s.io
          kind: KubeadmConfigTemplate
+         name: my-cluster-worker-machine-config-v1.35.0
-         name: my-cluster-worker-machine-config-v1.34.0
      infrastructureRef:
        apiGroup: infrastructure.cluster.x-k8s.io
        kind: VSphereMachineTemplate
+       name: my-cluster-worker-machine-infrastructure-v1.35.0
-       name: my-cluster-worker-machine-infrastructure-v1.34.0
```

Once all the changes above are submitted, Cluster API will trigger a rollout of the machines controlled by the `MachineDeployment`.

The `MachinePool` resource has a similar behavior.

> [!TIP]
> If you are using [ClusterClass], you can change the `MachineDeployment` or the `MachinePool`, the corresponding
> infrastructure configuration and bootstrap configuration with a single change to the Cluster resource.

## Using ClusterClass

If you are using [ClusterClass], you can upgrade the Kubernetes version for both control plane, workers, and
the corresponding infrastructure configuration and bootstrap configuration with a single change to the Cluster resource.

e.g. In order to upgrade the Kubernetes version all you have to do is:

```diff
---
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: my-docker-cluster
spec:
  ...
  topology:
    classRef:
      name: vsphere-clusterclass-v0.1.0
+   version: v1.35.0
-   version: v1.34.0
    controlPlane:
      replicas: 5
    workers:
      machineDeployments:
      - class: default-worker
        name: md-0
        replicas: 10
```

> [!NOTE]
> When using [ClusterClass] infrastructure configurations or machine bootstrap configurations are exposed through
> variables defined by the [ClusterClass] author.

Cluster API will take care of propagating all changes, including the Kubernetes version upgrade to the underlying
`ControlPlane`, `MachineDeployment` or `MachinePool` and corresponding `BootstrapConfigTemplate` and `InfrastructureTemplate` automatically.

Most notably, Cluster API will also take care of respecting the [Kubernetes version skew policy] while performing this operation by:

- Ensuring control plane is upgraded before workers.
- Ensuring control plane upgrades only by one minor at a time, and it will continue upgrading until the target version is reached (chained upgrades)
- Skipping minor version upgrades for workers when allowed by the Kubernetes version skew (efficient upgrades)

Additionally, in case both version upgrade and other changes are applied for workers, Cluster API defers both according to the upgrade sequence, thus
minimizing the number of rollouts for workers.

> [!NOTE]
> If a cluster is using a [ClusterClass], direct changes to the `ControlPlane`, `MachineDeployment` or `MachinePool`
> resources will be overridden with the corresponding settings from the `Cluster` resource.

[Kubernetes version skew policy]: https://kubernetes.io/releases/version-skew-policy/
[ClusterClass]: ../configuration/cluster-class/index.md
[Kubernetes version support matrix]: ../../../reference/versions.md#kubernetes-versions-support
[upgrade the version of Cluster API]: ../providers/upgrade.md
