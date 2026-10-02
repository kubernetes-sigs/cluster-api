# MachineHealthCheck and Remediation

A MachineHealthCheck is a resource within the Cluster API which allows users to define conditions under which a Machine within a cluster should be considered unhealthy.
Each MachineHealthCheck resource is scoped to Machines in a particular Cluster.

When defining a MachineHealthCheck, users specify a check on the Machine or on the Machine's Node.

If any of these conditions are met, the Machine will be remediated.

By default, the action of remediating a Machine should trigger deletion of the unhealthy Machine and creation of a new Machine,
but providers are allowed to plug in more sophisticated external remediation solutions.

> [!IMPORTANT]
> Please note that MachineHealthChecks currently **only** support Machines that are owned by a `MachineDeployment`, a `MachineSet` or a `KubeadmControlPlane`.
> Please review the [Limitations and Caveats of a MachineHealthCheck](#limitations-and-caveats-of-a-machinehealthcheck) for full details of MachineHealthCheck limitations.

> [!TIP]
> If you are using [ClusterClass], by default there is no need to define MachineHealthCheck manually because
> your Cluster will use MachineHealthCheck defined by the ClusterClass author, if any.
>
> If required, it is also possible to override MachineHealthCheck default configurations provided by the ClusterClass author.
> See [Using ClusterClass](#using-clusterclass).

## Creating a MachineHealthCheck

Use the following example as a basis for creating a MachineHealthCheck for worker nodes:

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: MachineHealthCheck
metadata:
  name: capi-quickstart-node-unhealthy-5m
spec:
  # clusterName is required to associate this MachineHealthCheck with a particular cluster
  clusterName: capi-quickstart
  # selector is used to determine which Machines should be health checked
  selector:
    matchLabels:
      nodepool: nodepool-0
  # checks are the checks that are used to evaluate if a Machine is healthy.
  checks:
      # (Optional) nodeStartupTimeout determines how long a MachineHealthCheck should wait for
      # a Node to join the cluster, before considering a Machine unhealthy.
      # Defaults to 10 minutes if not specified.
      # Set to 0 to disable the node startup timeout.
      # Disabling this timeout will prevent a Machine from being considered unhealthy when
      # the Node it created has not yet registered with the cluster. This can be useful when
      # Nodes take a long time to start up or when you only want condition based checks for
      # Machine health.
      nodeStartupTimeoutSeconds: 600

      # Conditions to check on Nodes for matched Machines, if any condition is matched for the duration of its timeout, the Machine is considered unhealthy
      unhealthyNodeConditions:
      - type: Ready
        status: Unknown
        timeoutSeconds: 300
      - type: Ready
        status: "False"
        timeoutSeconds: 300
      unhealthyMachineConditions:
      - type: "NodeReady"
        status: Unknown
        timeoutSeconds: 1800
      - type: "InfrastructureReady"
        status: "False"
        timeoutSeconds: 1800
  # remediation configures if and how remediation is triggered if a Machine is unhealthy.
  remediation:
    triggerIf:
      # (Optional) unhealthyLessThanOrEqualTo prevents further remediation if the cluster is already partially unhealthy
      unhealthyLessThanOrEqualTo: 40%
```

Use this example as the basis for defining a MachineHealthCheck for control plane nodes managed via
the KubeadmControlPlane:

```yaml
apiVersion: cluster.x-k8s.io/v1beta2
kind: MachineHealthCheck
metadata:
  name: capi-quickstart-kcp-unhealthy-5m
spec:
  clusterName: capi-quickstart
  selector:
    matchLabels:
      cluster.x-k8s.io/control-plane: ""
  checks:
    unhealthyNodeConditions:
    - type: Ready
      status: Unknown
      timeoutSeconds: 300
    - type: Ready
      status: "False"
      timeoutSeconds: 300
  remediation:
    triggerIf:
      unhealthyLessThanOrEqualTo: 100%
```

> [!IMPORTANT]
> If you are defining more than one `MachineHealthCheck` for the same Cluster, make sure that the selectors **do not overlap**
> in order to prevent conflicts or unexpected behaviors when trying to remediate the same set of machines.

## Controlling Remediation Retries

> [!IMPORTANT]
> This feature is only available for KubeadmControlPlane.

KubeadmControlPlane allows controlling how remediation happens by defining an optional `remediation`;
this feature can be used for preventing unnecessary load on infrastructure provider e.g. in case of quota problems, or for allowing the infrastructure provider to stabilize in case of temporary problems.

```yaml
apiVersion: controlplane.cluster.x-k8s.io/v1beta2
kind: KubeadmControlPlane
metadata:
  name: my-control-plane
spec:
  ...
  remediation:
    maxRetry: 5
    retryPeriodSeconds: 120 # 2m
    minHealthyPeriodSeconds: 7200 # 2h
```

`maxRetry` is the maximum number of retries while attempting to remediate an unhealthy machine.
A retry happens when a machine that was created as a replacement for an unhealthy machine also fails.
For example, given a control plane with three machines M1, M2, M3:

- M1 becomes unhealthy; remediation happens, and M1-1 is created as a replacement.
- If M1-1 (replacement of M1) has problems while bootstrapping it will become unhealthy, and then be
  remediated. This operation is considered a retry - remediation-retry #1.
- If M1-2 (replacement of M1-1) becomes unhealthy, remediation-retry #2 will happen, etc.

A retry will only happen after the `retryPeriodSeconds` from the previous retry has elapsed. If `retryPeriodSeconds` is not set (default), a retry will happen immediately.

If a machine is marked as unhealthy after `minHealthyPeriodSeconds` (default 3600) has passed since the previous remediation this is no longer considered a retry because the new issue is assumed unrelated to the previous one.

If `maxRetry` is not set (default), remediation will be retried infinitely.

> [!TIP]
> **Retry again once maxRetry is exhausted**
>
> If for some reasons you want to remediate once maxRetry is exhausted there are two options:
> - Temporarily increase  `maxRetry` (recommended)
> - Remove the `controlplane.cluster.x-k8s.io/remediation-for` annotation from the unhealthy machine or decrease `retryCount` in the annotation value.

## Remediation Short-Circuiting

To ensure that MachineHealthChecks do not perform excessive remediation of Machines,
short-circuiting is implemented to prevent further remediation via the `remediation.triggerIf` field within the MachineHealthCheck spec.

### Unhealthy Less Than or Equal To

If the user defines a value for the `unhealthyLessThanOrEqualTo` field (either an absolute number or a percentage of the total Machines checked by this MachineHealthCheck),
before remediating any Machines, the MachineHealthCheck will compare the value of `unhealthyLessThanOrEqualTo` with the number of Machines it has determined to be unhealthy.
If the number of unhealthy Machines exceeds the limit set by `unhealthyLessThanOrEqualTo`, remediation will **not** be performed.

> [!WARNING]
> The default value for `unhealthyLessThanOrEqualTo` is `100%`.
> This means the short circuiting mechanism is **disabled by default** and Machines will be remediated no matter the state of the cluster.

#### With an Absolute Value

If `unhealthyLessThanOrEqualTo` is set to `2`:
- If 2 or fewer nodes are unhealthy, remediation will be performed
- If 3 or more nodes are unhealthy, remediation will not be performed

These values are independent of how many Machines are being checked by the MachineHealthCheck.

#### With Percentages

If `unhealthyLessThanOrEqualTo` is set to `40%` and there are 25 Machines being checked:
- If 10 or fewer nodes are unhealthy, remediation will be performed
- If 11 or more nodes are unhealthy, remediation will not be performed

If `unhealthyLessThanOrEqualTo` is set to `40%` and there are 6 Machines being checked:
- If 2 or fewer nodes are unhealthy, remediation will be performed
- If 3 or more nodes are unhealthy, remediation will not be performed

Note, when the percentage is not a whole number, the allowed number is rounded down.

### Unhealthy in Range

If the user defines a value for the `unhealthyInRange` field (bracketed values that specify a start and an end value), before remediating any Machines,
the MachineHealthCheck will check if the number of Machines it has determined to be unhealthy is within the range specified by `unhealthyInRange`.
If it is not within the range set by `unhealthyInRange`, remediation will **not** be performed.

> [!NOTE]
> If both `unhealthyLessThanOrEqualTo` and `unhealthyInRange` are specified, `unhealthyInRange` takes precedence.

#### With a Range of Values

If `unhealthyInRange` is set to `[3-5]` and there are 10 Machines being checked:
- If 2 or fewer nodes are unhealthy, remediation will not be performed.
- If 6 or more nodes are unhealthy, remediation will not be performed.
- In all other cases, remediation will be performed.

Note, the above example had 10 machines as sample set. But, this would work the same way for any other number.
This is useful for dynamically scaling clusters where the number of machines keep changing frequently.

## Skipping Remediation

There are scenarios where remediation for a machine may be undesirable (eg. during cluster migration using `clusterctl move`). For such cases, MachineHealthCheck skips marking a Machine for remediation if:

- the Machine has the `cluster.x-k8s.io/skip-remediation` annotation
- the Machine has the `cluster.x-k8s.io/paused` annotation
- the MachineHealthCheck has the `cluster.x-k8s.io/paused` annotation
- the Cluster has `.spec.paused` set to `true`

## Limitations and Caveats of a MachineHealthCheck

Before deploying a MachineHealthCheck, please familiarise yourself with the following limitations and caveats:

- Only Machines owned by a MachineSet or a KubeadmControlPlane can be remediated by a MachineHealthCheck (since a MachineDeployment uses a MachineSet, then this includes Machines that are part of a MachineDeployment)
- Machines managed by a KubeadmControlPlane are remediated according to [the delete-and-recreate guidelines described in the KubeadmControlPlane proposal](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20191017-kubeadm-based-control-plane.md#remediation-using-delete-and-recreate)
  - The following rules should be satisfied in order to start remediation of a control plane machine:
    - One of the following applies:
      - The cluster MUST NOT be initialized yet (the failure happens before KCP reaches the initialized state)
      - The cluster MUST have at least two control plane machines, because this is the smallest cluster size that can be remediated.
    - Previous remediation (delete and re-create) MUST have been completed. This rule prevents KCP from remediating more machines while the replacement for the previous machine is not yet created.
    - The cluster MUST have no machines with a deletion timestamp. This rule prevents KCP taking actions while the cluster is in a transitional state.
    - Remediation MUST preserve etcd quorum. This rule ensures that we will not remove a member that would result in etcd losing a majority of members and thus become unable to field new requests (note: this rule applies only to CP already initialized and with managed etcd)
- If the Node for a Machine is removed from the cluster, a MachineHealthCheck will consider this Machine unhealthy and remediate it immediately
- If no Node joins the cluster for a Machine after the `NodeStartupTimeout`, the Machine will be remediated
- Important: if the kubelet on the node hosting the etcd leader member is not working, this prevents KCP from doing some checks it expects to do specifically on the leader.
  This prevents remediation from happening. There are ongoing discussions about how to overcome this limitation in https://github.com/kubernetes-sigs/cluster-api/issues/8465; as of today users facing this situation
  are recommended to manually forward leadership to another etcd member and manually delete the corresponding machine.

## Using ClusterClass

If you are using [ClusterClass], by default there is no need to define MachineHealthCheck manually because
your Cluster will use MachineHealthCheck defined by the ClusterClass author, if any.

If required, it is also possible to override MachineHealthCheck default configurations provided by the ClusterClass author
by setting the `healthCheck` field for control plane or workers in the Cluster object.

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
    version: v1.34.0
    controlPlane:
      replicas: 3
+     healthCheck:  # disable default MachineHealthCheck from the ClusterClass
+       enabled: false
    workers:
      machineDeployments:
      - class: default-worker
        name: md-0
+       healthCheck: # override default MachineHealthCheck from the ClusterClass
+         checks:
+           unhealthyNodeConditions:
+           - type: Ready
+             status: Unknown
+             timeoutSeconds: 600
```

> [!NOTE]
> If a cluster is using a [ClusterClass], direct changes to the `MachineHealthCheck`
> resources will be overridden with the corresponding health check configuration from the ClusterClass or
> from the `Cluster` resource.

[ClusterClass]: ../configuration/cluster-class/index.md
