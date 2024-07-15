# Configure a MachineHealthCheck

## Prerequisites

Before attempting to configure a MachineHealthCheck, you should have a working [management cluster] with at least one MachineDeployment or MachineSet deployed.

<aside class="note warning">

<h1> Important </h1>

Please note that MachineHealthChecks currently **only** support Machines that are owned by a MachineSet or a KubeadmControlPlane.
Please review the [Limitations and Caveats of a MachineHealthCheck](#limitations-and-caveats-of-a-machinehealthcheck)
at the bottom of this page for full details of MachineHealthCheck limitations.

</aside>

## What is a MachineHealthCheck?

A MachineHealthCheck is a resource within the Cluster API which allows users to define conditions under which Machines within a Cluster should be considered unhealthy.
A MachineHealthCheck is defined on a management cluster and scoped to a particular workload cluster.

When defining a MachineHealthCheck, users specify a timeout for each of the conditions that they define to check on the Machine's Node.
If any of these conditions are met for the duration of the timeout, the Machine will be remediated. Also, Machines with `failureMessage` or `failureMessage` (terminal failures) are automatically remediated.
By default, the action of remediating a Machine should trigger a new Machine to be created to replace the failed one, but providers are allowed to plug in more sophisticated external remediation solutions.

## Creating a MachineHealthCheck

Use the following example as a basis for creating a MachineHealthCheck for worker nodes:

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: MachineHealthCheck
metadata:
  name: capi-quickstart-node-unhealthy-5m
spec:
  # clusterName is required to associate this MachineHealthCheck with a particular cluster
  clusterName: capi-quickstart
  # (Optional) maxUnhealthy prevents further remediation if the cluster is already partially unhealthy
  maxUnhealthy: 40%
  # (Optional) nodeStartupTimeout determines how long a MachineHealthCheck should wait for
  # a Node to join the cluster, before considering a Machine unhealthy.
  # Defaults to 10 minutes if not specified.
  # Set to 0 to disable the node startup timeout.
  # Disabling this timeout will prevent a Machine from being considered unhealthy when
  # the Node it created has not yet registered with the cluster. This can be useful when
  # Nodes take a long time to start up or when you only want condition based checks for
  # Machine health.
  nodeStartupTimeout: 10m
  # selector is used to determine which Machines should be health checked
  selector:
    matchLabels:
      nodepool: nodepool-0
  # Conditions to check on Nodes for matched Machines, if any condition is matched for the duration of its timeout, the Machine is considered unhealthy
  unhealthyConditions:
  - type: Ready
    status: Unknown
    timeout: 300s
  - type: Ready
    status: "False"
    timeout: 300s
```

Use this example as the basis for defining a MachineHealthCheck for control plane nodes managed via
the KubeadmControlPlane:

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: MachineHealthCheck
metadata:
  name: capi-quickstart-kcp-unhealthy-5m
spec:
  clusterName: capi-quickstart
  maxUnhealthy: 100%
  selector:
    matchLabels:
      cluster.x-k8s.io/control-plane: ""
  unhealthyConditions:
    - type: Ready
      status: Unknown
      timeout: 300s
    - type: Ready
      status: "False"
      timeout: 300s
```

<aside class="note warning">

<h1> Important </h1>

If you are defining more than one `MachineHealthCheck` for the same Cluster, make sure that the selectors **do not overlap**
in order to prevent conflicts or unexpected behaviors when trying to remediate the same set of machines.

</aside>

## Controlling remediation retries

<aside class="note warning">

<h1> Important </h1>

This feature is only available for KubeadmControlPlane.

</aside>

KubeadmControlPlane allows to control how remediation happen by defining an optional `remediationStrategy`;
this feature can be used for preventing unnecessary load on infrastructure provider e.g. in case of quota problems,or for allowing the infrastructure provider to stabilize in case of temporary problems.

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: KubeadmControlPlane
metadata:
  name: my-control-plane
spec:
  ...
  remediationStrategy:
    maxRetry: 5
    retryPeriod: 2m
    minHealthyPeriod: 2h
```

`maxRetry` is the maximum number of retries while attempting to remediate an unhealthy machine.
A retry happens when a machine that was created as a replacement for an unhealthy machine also fails.
For example, given a control plane with three machines M1, M2, M3:

- M1 become unhealthy; remediation happens, and M1-1 is created as a replacement.
- If M1-1 (replacement of M1) has problems while bootstrapping it will become unhealthy, and then be 
  remediated. This operation is considered a retry - remediation-retry #1.
- If M1-2 (replacement of M1-1) becomes unhealthy, remediation-retry #2 will happen, etc.

A retry will only happen after the `retryPeriod` from the previous retry has elapsed. If `retryPeriod` is not set (default), a retry will happen immediately.

If a machine is marked as unhealthy after `minHealthyPeriod` (default 1h) has passed since the previous remediation this is no longer considered a retry because the new issue is assumed unrelated from the previous one.

If `maxRetry` is not set (default), remediation will be retried infinitely.

<aside class="note">

<h1> Retry again once maxRetry is exhausted</h1>

If for some reasons you want to remediate once maxRetry is exhausted there are two options:
- Temporarily increase  `maxRetry` (recommended)
- Remove the `controlplane.cluster.x-k8s.io/remediation-for` annotation from the unhealthy machine or decrease `retryCount` in the annotation value.

</aside>

## Remediation Short-Circuiting

To ensure that MachineHealthChecks only remediate Machines when the cluster is healthy,
short-circuiting is implemented to prevent further remediation via the `maxUnhealthy` and `unhealthyRange` fields within the MachineHealthCheck spec.

### Max Unhealthy

If the user defines a value for the `maxUnhealthy` field (either an absolute number or a percentage of the total Machines checked by this MachineHealthCheck),
before remediating any Machines, the MachineHealthCheck will compare the value of `maxUnhealthy` with the number of Machines it has determined to be unhealthy.
If the number of unhealthy Machines exceeds the limit set by `maxUnhealthy`, remediation will **not** be performed.

<aside class="note warning">

<h1> Warning </h1>

The default value for `maxUnhealthy` is `100%`.
This means the short circuiting mechanism is **disabled by default** and Machines will be remediated no matter the state of the cluster.

</aside>

#### With an Absolute Value

If `maxUnhealthy` is set to `2`:
- If 2 or fewer nodes are unhealthy, remediation will be performed
- If 3 or more nodes are unhealthy, remediation will not be performed

These values are independent of how many Machines are being checked by the MachineHealthCheck.

#### With Percentages

If `maxUnhealthy` is set to `40%` and there are 25 Machines being checked:
- If 10 or fewer nodes are unhealthy, remediation will be performed
- If 11 or more nodes are unhealthy, remediation will not be performed

If `maxUnhealthy` is set to `40%` and there are 6 Machines being checked:
- If 2 or fewer nodes are unhealthy, remediation will be performed
- If 3 or more nodes are unhealthy, remediation will not be performed

Note, when the percentage is not a whole number, the allowed number is rounded down.

### Unhealthy Range

If the user defines a value for the `unhealthyRange` field (bracketed values that specify a start and an end value), before remediating any Machines,
the MachineHealthCheck will check if the number of Machines it has determined to be unhealthy is within the range specified by `unhealthyRange`.
If it is not within the range set by `unhealthyRange`, remediation will **not** be performed.

<aside class="note warning">

<h1> Important </h1>

If both `maxUnhealthy` and `unhealthyRange` are specified, `unhealthyRange` takes precedence.

</aside>

#### With a range of values

If `unhealthyRange` is set to `[3-5]` and there are 10 Machines being checked:
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
    - One of the following apply:
        - The cluster MUST not be initialized yet (the failure happens before KCP reaches the initialized state)
        - The cluster MUST have at least two control plane machines, because this is the smallest cluster size that can be remediated.
    - Previous remediation (delete and re-create) MUST have been completed. This rule prevents KCP from remediating more machines while the replacement for the previous machine is not yet created.
    - The cluster MUST have no machines with a deletion timestamp. This rule prevents KCP taking actions while the cluster is in a transitional state.
    - Remediation MUST preserve etcd quorum. This rule ensures that we will not remove a member that would result in etcd losing a majority of members and thus become unable to field new requests (note: this rule applies only to CP already initialized and with managed etcd)
- If the Node for a Machine is removed from the cluster, a MachineHealthCheck will consider this Machine unhealthy and remediate it immediately
- If no Node joins the cluster for a Machine after the `NodeStartupTimeout`, the Machine will be remediated
- If a Machine fails for any reason (if the FailureReason is set), the Machine will be remediated immediately
- Important: if the kubelet on the node hosting the etcd leader member is not working, this prevents KCP from doing some checks it is expecting to do on the leader - and specifically on the leader -.
  This prevents remediation to happen. There are ongoing discussions about how to overcome this limitation in https://github.com/kubernetes-sigs/cluster-api/issues/8465; as of today users facing this situation
  are recommended to manually forward leadership to another etcd member and manually delete the corresponding machine.

<!-- links -->
[management cluster]: ../../reference/glossary.md#management-cluster
