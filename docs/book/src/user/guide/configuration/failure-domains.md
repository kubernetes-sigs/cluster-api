# Failure Domains

## What failure domains are

A failure domain is a part of the infrastructure that can fail without affecting the other parts.
A common example is an availability zone in a cloud. What exactly a failure domain is depends on the infrastructure provider.

Cluster API does not create failure domains. An infrastructure provider that supports them reports the available
failure domains in the `status.failureDomains` field of its InfraCluster resource (for example, an `OpenStackCluster`).
After the InfraCluster reports that it is provisioned, the Cluster controller copies this list to
`Cluster.status.failureDomains`.
The copy runs on every reconcile, so this is a field to read, not a field to set.
If the InfraCluster does not report failure domains, the Cluster has none.

Some providers let you define the list in the InfraCluster spec. For example, the Docker provider (CAPD), which is
used for development and testing, copies the failure domains from its `DevCluster` spec to its status.

In the v1beta2 API, `Cluster.status.failureDomains` is a list. Each entry has these fields:

- `name`: the unique name of the failure domain. Machines refer to a failure domain by this name.
- `controlPlane`: if `true`, control plane Machines can be placed in this failure domain.
  If the field is not set, Cluster API treats it as `false`.
- `attributes`: a free-form map of strings. The infrastructure provider can use it. Core Cluster API does not read it.

For example, a provider could report three failure domains for control plane Machines and one for workers only.
This is status written by the provider. Do not apply it as a resource.

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: my-cluster
status:
  failureDomains:
  - name: zone-a
    controlPlane: true
  - name: zone-b
    controlPlane: true
  - name: zone-c
    controlPlane: true
  - name: zone-d
    controlPlane: false
    attributes:
      example-key: example-value
```

To see the failure domains of a Cluster:

```bash
kubectl get cluster <cluster-name> -o jsonpath='{.status.failureDomains}'
```

> [!NOTE]
> In the v1beta1 API, `failureDomains` is a map keyed by the failure domain name, and `controlPlane` is a boolean.

A Machine has two failure domain fields:

- `spec.failureDomain`: the failure domain where the Machine should be created.
  The infrastructure provider is expected to create the Machine in this failure domain.
  It should match a `name` in `Cluster.status.failureDomains`. Cluster API does not validate this.
  What happens with a name that is not in the list depends on the infrastructure provider.
- `status.failureDomain`: the failure domain where the Machine was placed, as reported by the infrastructure
  provider in the InfraMachine `status.failureDomain` field. Providers are not required to report it.
  If the provider does not report it, this field stays empty.

The "Failure domain" column of `kubectl get machines` shows `status.failureDomain`:

```bash
kubectl get machines -n <namespace>
```

> [!IMPORTANT]
> Cluster API does not set the `topology.kubernetes.io/zone` label on Nodes.
> This label is usually set by the cloud provider integration of the workload cluster, for example its cloud
> controller manager. Features such as pod topology spread constraints use this Node label, so check that your
> cloud provider integration sets it. Configuring failure domains in Cluster API alone does not add it.
> To check the label, run this command against the workload cluster:
>
> ```bash
> kubectl get nodes -L topology.kubernetes.io/zone
> ```

## Control plane

The KubeadmControlPlane (KCP) controller spreads control plane Machines across failure domains by itself.
KCP has no failure domain field. It uses the list in `Cluster.status.failureDomains`.

KCP only uses failure domains with `controlPlane: true`. If no failure domain has `controlPlane: true`,
KCP creates Machines without `spec.failureDomain`, and the infrastructure provider decides where to place them.

When KCP creates a control plane Machine, it picks the failure domain in this order:

1. The failure domain with the fewest up-to-date Machines that are not being deleted.
2. If there is a tie, the failure domain with the fewest Machines in total.
3. If there is still a tie, any of the tied failure domains can be picked.

This applies to the first Machine, to scale up, to rollouts, and to the replacement Machine created after a
remediation. KCP sets `spec.failureDomain` only when it creates a Machine. It never changes it later.

When KCP removes a control plane Machine, for example during scale down or a rollout, it first selects the
Machines that can be removed, for example outdated Machines or Machines with the `cluster.x-k8s.io/delete-machine`
annotation. From these Machines:

1. Machines without a failure domain, or in a failure domain that KCP no longer uses, are removed first.
   This is a failure domain that is no longer in the list, or that no longer has `controlPlane: true`.
2. Otherwise, KCP picks the failure domain with the most of these Machines. If there is a tie, it picks the
   failure domain with the most Machines in total. Then it removes the oldest of these Machines in that failure domain.

To see the failure domain of each control plane Machine:

```bash
kubectl get machines -n <namespace> -l cluster.x-k8s.io/control-plane
```

The spreading is best effort. Keep in mind:

- If there are more replicas than failure domains, some failure domains get more Machines than others.
  For example, 5 replicas in 3 failure domains are typically spread as 2, 2 and 1.
- KCP counts Machines by `spec.failureDomain`, which is the requested failure domain.
  It expects the infrastructure provider to create each Machine in that failure domain.
- KCP does not move existing Machines to other failure domains. A change in the list of failure domains does not
  make a Machine outdated, so it does not start a rollout. New failure domains are used the next time KCP creates
  a Machine, and Machines in failure domains that KCP no longer uses are removed first the next time KCP removes
  a Machine.

> [!TIP]
> etcd needs a majority of members to work. With 3 control plane replicas in 3 failure domains, the control plane
> can keep working if one failure domain fails. With 3 replicas in only 2 failure domains, one failure domain has
> 2 Machines, and losing it breaks quorum. Use an odd number of replicas, and enough failure domains with
> `controlPlane: true` for the failures you want to survive.

For how KCP remediates unhealthy Machines, see [MachineHealthCheck and Remediation].

## Workers with MachineDeployments

A MachineDeployment has a single failure domain field: `spec.template.spec.failureDomain`.
All Machines of a MachineDeployment are created with this value, so one MachineDeployment uses one failure domain.
The only exception is a rollout that changes the failure domain: during the rollout, old and new Machines are in
different failure domains. If the field is not set, the Machines have no `spec.failureDomain`, and the infrastructure provider
decides where to place them.

To spread workers across failure domains, the common pattern is one MachineDeployment per failure domain.
This is a pattern, not an API requirement. The MachineDeployments use the same templates and differ only in
name and `failureDomain`:

```yaml
---
apiVersion: cluster.x-k8s.io/v1beta2
kind: MachineDeployment
metadata:
  name: my-cluster-md-zone-a
spec:
  clusterName: my-cluster
  replicas: 2
  template:
    spec:
      bootstrap:
        configRef:
          apiGroup: bootstrap.cluster.x-k8s.io
          kind: KubeadmConfigTemplate
          name: my-cluster-workers
      failureDomain: zone-a
      infrastructureRef:
        apiGroup: infrastructure.cluster.x-k8s.io
        kind: VSphereMachineTemplate
        name: my-cluster-worker-machine
      version: v1.34.0
# Create my-cluster-md-zone-b and my-cluster-md-zone-c in the same way,
# with failureDomain: zone-b and failureDomain: zone-c.
```

Cluster API does not balance Machines between MachineDeployments. Each MachineDeployment has its own replica
count. To get an even spread, give each MachineDeployment the same number of replicas.

One MachineDeployment per failure domain can also help workloads with zonal storage. A Pod that uses a zonal
persistent volume can only be scheduled in the zone of that volume. If you use the Cluster Autoscaler, its
[FAQ](https://github.com/kubernetes/autoscaler/blob/master/cluster-autoscaler/FAQ.md#i-have-a-couple-of-pending-pods-but-there-was-no-scale-up)
recommends separate node groups per zone, so that the autoscaler knows which node group adds Nodes in the zone the
Pod needs. In a node group that spans several zones, such as a MachinePool with several failure domains, the
infrastructure provider chooses the zone of each new Node. Volume placement and Pod scheduling are done by
Kubernetes and the storage provider, not by Cluster API. See also [Cluster Autoscaler](#cluster-autoscaler).

Changing `spec.template.spec.failureDomain` makes the existing Machines outdated and starts a rollout. The new
Machines are created in the new failure domain. See [Rollout Changes] for more details.

If you are using [ClusterClass], a MachineDeployment class can set a default `failureDomain`, and each
MachineDeployment in `Cluster.spec.topology.workers.machineDeployments` can override it.
For example, one MachineDeployment per failure domain with the same class:

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: my-cluster
spec:
  # Other fields are omitted.
  topology:
    classRef:
      name: vsphere-clusterclass-v0.1.0
    version: v1.34.0
    controlPlane:
      replicas: 3
    workers:
      machineDeployments:
      - class: default-worker
        name: md-zone-a
        replicas: 2
        failureDomain: zone-a
      - class: default-worker
        name: md-zone-b
        replicas: 2
        failureDomain: zone-b
      - class: default-worker
        name: md-zone-c
        replicas: 2
        failureDomain: zone-c
```

## Workers with MachinePools

A MachinePool has a list field: `spec.failureDomains`. If you are using [ClusterClass], a MachinePool class can
set a default list, and each MachinePool in `Cluster.spec.topology.workers.machinePools` can override it.

The core MachinePool controller does not use `spec.failureDomains`. It does not pick a failure domain for each
instance, and it does not balance instances across the list. An infrastructure provider that supports this field
reads the list and decides how to place the instances. For example, a provider can pass the list to a cloud scaling group that spans
several zones. The Docker provider (CAPD), which is used for testing, picks a random failure domain from the list
for each instance.

Because of this:

- Do not assume that instances are spread evenly across the listed failure domains.
- Cluster API does not check that the names exist in `Cluster.status.failureDomains`.
- What happens when a listed failure domain is not available, or when the list changes, depends on the provider.

Check your infrastructure provider's documentation for how it uses `spec.failureDomains`.
See [MachinePools] for more details about MachinePools.

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: MachinePool
metadata:
  name: my-cluster-mp-0
spec:
  clusterName: my-cluster
  replicas: 6
  # The infrastructure provider decides how instances are placed across these failure domains.
  failureDomains:
  - zone-a
  - zone-b
  - zone-c
  template:
    spec:
      # Bootstrap, infrastructure and version fields are omitted.
```

## MachineHealthCheck and Cluster Autoscaler

MachineHealthCheck and Cluster Autoscaler do different jobs, and neither of them uses Cluster API failure domains.

### MachineHealthCheck

A MachineHealthCheck finds unhealthy Machines and marks them for remediation. The owner of the Machine does the
remediation. Only Machines owned by a MachineSet (so also Machines of a MachineDeployment) or by a
KubeadmControlPlane are remediated this way:

- MachineSet: the unhealthy Machine is deleted, and the MachineSet creates a new one from its template.
  The new Machine gets the `spec.failureDomain` of the template, so it is created in the same failure domain.
- KubeadmControlPlane: KCP remediates only when it is safe, for example when deleting the Machine keeps etcd
  quorum. Otherwise it waits. If it remediates, it deletes the unhealthy Machine, and the new Machine gets a
  failure domain as described in [Control plane](#control-plane).

The MachinePool controller does not remediate Machines. If the infrastructure provider supports MachinePool
Machines, a MachineHealthCheck can mark a MachinePool Machine as unhealthy, but Cluster API does not replace it,
unless you use external remediation (the MachineHealthCheck's `spec.remediation.templateRef`). In that case, the external remediation controller decides what happens. The infrastructure behind the MachinePool,
for example a cloud scaling group, can have its own health checks, but they are not part of MachineHealthCheck.
See [MachineHealthCheck and Remediation] for all limitations.

Keep in mind when a whole failure domain fails:

- Replacement Machines of a MachineDeployment are created in the same failure domain. If the failure domain is
  still down, the replacements cannot become healthy either. `spec.remediation.maxInFlight` on a
  MachineDeployment limits how many of its Machines are remediated at the same time.
- KCP creates a replacement control plane Machine only after the unhealthy Machine has been deleted. It does not
  reuse the deleted Machine's failure domain; instead, it selects a domain using the same rules described in
  [Control plane](#control-plane). If Machines were evenly distributed before the failure, the domain that lost
  a Machine now has the fewest up-to-date Machines and is therefore usually selected again. If that domain is
  still unavailable but remains reported with `controlPlane: true`, KCP may request the replacement in that
  domain again. If deletion of the unhealthy Machine does not complete, KCP waits and does not create the
  replacement.
- The MachineHealthCheck's `spec.remediation.triggerIf` (`unhealthyLessThanOrEqualTo` or `unhealthyInRange`)
  counts all the Machines selected by the MachineHealthCheck, not the Machines per failure domain. For example, one MachineHealthCheck
  covers three MachineDeployments of the same size in three failure domains. If one failure domain fails, about a
  third (33%) of the Machines become unhealthy. With `unhealthyLessThanOrEqualTo: 40%` remediation still runs.
  With `unhealthyLessThanOrEqualTo: 30%` it stops.
- To get a limit per failure domain, use one MachineHealthCheck per MachineDeployment.
  For example, select the Machines with the `cluster.x-k8s.io/deployment-name` label. If you are using
  [ClusterClass] and the health check is enabled for MachineDeployments, Cluster API already creates one
  MachineHealthCheck per MachineDeployment.

### Cluster Autoscaler

The Cluster Autoscaler changes the number of replicas of node groups based on the pods in the cluster and its
own configuration. With Cluster API, a node group is a MachineDeployment, MachineSet or MachinePool with the
`cluster.x-k8s.io/cluster-api-autoscaler-node-group-min-size` and
`cluster.x-k8s.io/cluster-api-autoscaler-node-group-max-size` annotations.
For MachinePools, the infrastructure provider must support MachinePool Machines.
See [Autoscaling] for how to configure it.

With one MachineDeployment per failure domain, each MachineDeployment with these annotations is a separate
node group, so each failure domain is scaled on its own.
The autoscaler does not balance between node groups by default. Balancing is configured with Cluster Autoscaler
flags, not in Cluster API:

- Enable `--balance-similar-node-groups` so that the autoscaler balances new nodes between similar node groups.
- Node groups are similar only if their nodes have the same capacity, labels and taints. The autoscaler already
  ignores the zone label `topology.kubernetes.io/zone`. If your provider adds other zone labels, add them with
  `--balancing-ignore-label`.
- Balancing is best effort, and it only happens at scale up. Scale down removes underutilized nodes without
  looking at the size of each node group, so node groups can become uneven.

If a node group scales from zero, there is no Node to read labels from. If pods need a specific zone, make sure
the zone label is part of the node group's template, for example with the
`capacity.cluster-autoscaler.kubernetes.io/labels` annotation. See [Autoscaling] for scale from zero.

How the autoscaler reacts when a node group cannot create Machines, for example because its failure domain is
down, depends on the autoscaler and its configuration. See the
[Cluster Autoscaler documentation](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler) for details.

[ClusterClass]: ./cluster-class/index.md
[Rollout Changes]: ../lifecycle/rollout.md
[MachinePools]: ./workers/machine-pools.md
[MachineHealthCheck and Remediation]: ../lifecycle/healthchecking.md
[Autoscaling]: ../lifecycle/autoscaling.md
