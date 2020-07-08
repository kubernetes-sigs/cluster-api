# Configure a MachineHealthCheck

## Prerequisites

Before attempting to configure a MachineHealthCheck, you should have a working [management cluster] with at least one MachineDeployment or MachineSet deployed.

<aside class="note warning">

<h1> Important </h1>

Please note that MachineHealthChecks currently **only** support Machines that are owned by a MachineSet.
Please review the [Limitations and Caveats of a MachineHealthCheck](#limitations-and-caveats-of-a-machinehealthcheck)
at the bottom of this page for full details of MachineHealthCheck limitations.

</aside>

## What is a MachineHealthCheck?

A MachineHealthCheck is a resource within the Cluster API which allows users to define conditions under which Machine's within a Cluster should be considered unhealthy.

When defining a MachineHealthCheck, users specify a timeout for each of the conditions that they define to check on the Machine's Node,
if any of these conditions is met for the duration of the timeout, the Machine will be remediated.
The action of remediating a Machine should trigger a new Machine to be created, to replace the failed one.

## Creating a MachineHealthCheck

Use the following example as a basis for creating a MachineHealthCheck:

```yaml
apiVersion: cluster.x-k8s.io/v1alpha3
kind: MachineHealthCheck
metadata:
  name: capi-quickstart-node-unhealthy-5m
spec:
  # clusterName is required to associate this MachineHealthCheck with a particular cluster
  clusterName: capi-quickstart
  # (Optional) maxUnhealthy prevents further remediation if the cluster is already partially unhealthy
  maxUnhealthy: 40%
  # (Optional) nodeStartupTimeout determines how long a MachineHealthCheck should wait for
  # a Node to join the cluster, before considering a Machine unhealthy
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

<aside class="note warning">

<h1> Important </h1>

If you are defining more than one `MachineHealthCheck` for the same Cluster, make sure that the selectors **do not overlap**
n order to prevent conflicts or unexpected behaviors when trying to remediate the same set of machines.

</aside>

## Remediation short-circuiting

To ensure that MachineHealthChecks only remediate Machines when the cluster is healthy,
short-circuiting is implemented to prevent further remediation via the `maxUnhealthy` field within the MachineHealthCheck spec.

If the user defines a value for the `maxUnhealthy` field (either an absolute number or a percentage of the total Machines checked by this MachineHealthCheck),
before remediating any Machines, the MachineHealthCheck will compare the value of `maxUnhealthy` with the number of Machines it has determined to be unhealthy.
If the number of unhealthy Machines exceeds the limit set by `maxUnhealthy`, remediation will **not** be performed.

<aside class="note warning">

<h1> Warning </h1>

The default value for `maxUnhealthy` is `100%`.
This means the short circuiting mechanism is **disabled by default** and Machines will be remediated no matter the state of the cluster.

</aside>

#### With an absolute value

If `maxUnhealthy` is set to `2`:
- If 2 or fewer nodes are unhealthy, remediation will be performed
- If 3 or more nodes are unhealthy, remediation will not be performed

These values are independent of how many Machines are being checked by the MachineHealthCheck.

#### With percentages

If `maxUnhealthy` is set to `40%` and there are 25 Machines being checked:
- If 10 or fewer nodes are unhealthy, remediation will be performed
- If 11 or more nodes are unhealthy, remediation will not be performed

If `maxUnhealthy` is set to `40%` and there are 6 Machines being checked:
- If 2 or fewer nodes are unhealthy, remediation will be performed
- If 3 or more nodes are unhealthy, remediation will not be performed

Note, when the percentage is not a whole number, the allowed number is rounded down.

## Limitations and Caveats of a MachineHealthCheck

Before deploying a MachineHealthCheck, please familiarise yourself with the following limitations and caveats:

- Only Machines owned by a MachineSet will be remediated by a MachineHealthCheck
- Control Plane Machines are currently not supported and will **not** be remediated if they are unhealthy
- If the Node for a Machine is removed from the cluster, a MachineHealthCheck will consider this Machine unhealthy and remediate it immediately
- If no Node joins the cluster for a Node after the `NodeStartupTimeout`, the Machine will be remediated
- If a Machine fails for any reason (if the FailureReason is set), the Machine will be remediated immediately

<!-- links -->
[management cluster]: ../reference/glossary.md#management-cluster
