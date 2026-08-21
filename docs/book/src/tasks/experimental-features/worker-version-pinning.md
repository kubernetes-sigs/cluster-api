# Worker Version Pinning

**Feature gate name**: `ClusterTopologyWorkerVersionPinning`

**Variable name to enable/disable the feature gate**: `EXP_CLUSTER_TOPOLOGY_WORKER_VERSION_PINNING`

By default every MachineDeployment/MachinePool of a Cluster with a managed topology follows
`Cluster.spec.topology.version`, so workers cannot be upgraded on their own schedule.

Worker version pinning adds an optional `version` field to `MachineDeploymentTopology` and
`MachinePoolTopology`. When set, that MachineDeployment/MachinePool is pinned to the given
Kubernetes version and is upgraded manually, independently of the control plane and of the other
MachineDeployments/MachinePools.

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
spec:
  topology:
    version: v1.32.0
    workers:
      machineDeployments:
        - class: default-worker
          name: md-0
          version: v1.31.0 # pinned, upgraded manually
        - class: default-worker
          name: md-1       # not pinned, follows spec.topology.version
```

## Workflow

Pinning and unpinning are always explicit user actions. Cluster API never sets or clears the field.

1. **Pin.** Set `version` to the version the MachineDeployment/MachinePool currently runs, which at a
   stable Cluster is the control plane version. This does not trigger a rollout, it only opts the
   MachineDeployment/MachinePool out of cluster-managed versioning.
2. **Diverge.** Upgrade the Cluster: pinned MachineDeployments/MachinePools hold back while the
   control plane and the cluster-managed ones move on.
3. **Catch up.** Raise the pin to roll only that MachineDeployment/MachinePool.
4. **Unpin.** Clear the field to return to cluster-managed versioning. This is allowed only once the
   pin equals `Cluster.spec.topology.version`.

## Rules

A pinned version:

- must be a valid semantic version and one of `ClusterClass.spec.kubernetesVersions`, if that list is set,
- can never be decreased,
- can never be greater than the control plane version,
- must conform to the [Kubernetes version skew policy](https://kubernetes.io/releases/version-skew-policy/).
  `Cluster.spec.topology.version` is validated against every pinned version too, so the Cluster
  cannot be upgraded past what the skew policy allows for a pinned MachineDeployment/MachinePool.
  This also bounds chained upgrades,
- cannot be combined with the `topology.cluster.x-k8s.io/defer-upgrade` or
  `topology.cluster.x-k8s.io/hold-upgrade-sequence` annotations, which control the cluster-level
  upgrade sequence that a pinned MachineDeployment/MachinePool is excluded from.

These rules are enforced by the Cluster validating webhook and backstopped at runtime by the
[MachineSet preflight checks](./machineset-preflight-checks.md).

## Behavior

A pinned MachineDeployment/MachinePool is excluded from the cluster-level rollout:

- a change to `Cluster.spec.topology.version` does not roll it,
- it does not consume the upgrade concurrency configured with
  `topology.cluster.x-k8s.io/upgrade-concurrency`, and raising several pins at once starts several
  rollouts in parallel,
- it does not block the control plane from advancing. Its rollout is surfaced by the Cluster
  `RollingOut` condition,
- lifecycle hooks ignore it: `AfterClusterUpgrade` is not delayed by a pinned
  MachineDeployment/MachinePool that is still behind, a blocking `AfterControlPlaneUpgrade` or
  `BeforeWorkersUpgrade` response does not hold back its rollout, and it does not contribute to the
  minimum workers version passed to `BeforeWorkersUpgrade`,
- unlike a MachineDeployment/MachinePool waiting for a cluster-level upgrade, it keeps reconciling
  scale and configuration changes while it holds back.

Version-aware patches keep working: `builtin.machineDeployment.version` and
`builtin.machinePool.version` report the pinned version, while
`builtin.cluster.topology.version` stays the cluster-wide version.

## Joining at an older version with kubeadm

kubeadm only supports joining with the same major and minor version as the control plane, so a new
Machine of a MachineDeployment that is pinned behind the control plane needs a matching kubeadm
binary. This is not solved by Cluster API itself. The supported pattern is:

- write the control plane version into a bootstrap file using `contentFormat: Template` and the
  `{{ .controlPlane.version }}` variable, and install the matching kubeadm binary from a
  `preKubeadmCommand`,
- skip the `KubeadmVersionSkew` preflight check for that MachineDeployment with the
  `machineset.cluster.x-k8s.io/skip-preflight-checks` annotation.

The `ControlPlaneVersionSkew` preflight check is skipped automatically for pinned
MachineDeployments, because it requires the MachineSet version to equal the control plane version.
`KubernetesVersionSkew` remains enforced.

<aside class="note warning">

<h1>Caveat</h1>

While a cluster-level upgrade is in progress, the `ControlPlaneIsStable` preflight check also blocks
scale up and remediation of pinned MachineDeployments, because the control plane version does not
match `Cluster.spec.topology.version` yet.

</aside>
