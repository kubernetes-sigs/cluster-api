---
title: Machine health checking a.k.a node auto repair
authors:
  - "@enxebre"
  - "@bison"
  - "@benmoss"
reviewers:
  - "@detiber"
  - "@vincepri"
  - "@ncdc"
  - "@timothysc"
creation-date: 2019-10-30
last-updated: 2020-04-05
status: implementable
see-also:
replaces:
superseded-by:
---

# Title
- Machine health checking a.k.a node auto repair

## Table of Contents

A table of contents is helpful for quickly jumping to sections of a proposal and for highlighting
any additional information provided beyond the standard proposal template.
[Tools for generating](https://github.com/ekalinin/github-markdown-toc) a table of contents from markdown are available.

- [Title](#title)
  - [Table of Contents](#table-of-contents)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals](#non-goals)
  - [Proposal](#proposal)
    - [User Stories [optional]](#user-stories-optional)
      - [Story 1](#story-1)
      - [Story 2](#story-2)
    - [Implementation Details/Notes/Constraints [optional]](#implementation-detailsnotesconstraints-optional)
    - [Risks and Mitigations](#risks-and-mitigations)
  - [Design Details](#design-details)
    - [Test Plan](#test-plan)
    - [Graduation Criteria](#graduation-criteria)
      - [Examples](#examples)
        - [Alpha -> Beta Graduation](#alpha---beta-graduation)
        - [Beta -> GA Graduation](#beta---ga-graduation)
        - [Removing a deprecated flag](#removing-a-deprecated-flag)
    - [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy)
    - [Version Skew Strategy](#version-skew-strategy)
  - [Implementation History](#implementation-history)
  - [Drawbacks [optional]](#drawbacks-optional)
  - [Alternatives [optional]](#alternatives-optional)
  - [Infrastructure Needed [optional]](#infrastructure-needed-optional)

## Glossary
Refer to the [Cluster API Book Glossary](https://cluster-api.sigs.k8s.io/reference/glossary.html).

## Summary
Enable opt in automated health checking and remediation of unhealthy nodes backed by machines.

## Motivation
- Reduce administrative overhead to run a cluster.
- Increase ability to respond to failures of machines and keep the cluster nodes healthy.

### Goals
- Enable automated remediation for groups of machines/nodes (e.g a machineSet)
- Allow users to define different health criteria based on node conditions for different groups of nodes.
- Provide a means for the cluster administrator to configure thresholds for disabling automated remediation when multiple nodes are unhealthy at the same time.

### Non-Goals/Future Work
- Record long-term stable history of all health-check failures or remediations.
- Provide a mechanism to guarantee that application quorum for N members is maintained at any time.

## Proposal
The machine health checker (MHC) is responsible for marking machines backing unhealthy Nodes with a condition so that they can be remediated by their owning controller.

It provides a short-circuit mechanism and limits remediation when the `maxUnhealthy` threshold is reached for a targeted group of machines.
This is similar to what the node life cycle controller does for reducing the eviction rate as nodes become unhealthy in a given zone. E.g a large number of nodes in a single zone are down due to a networking issue.

The machine health checker is an integration point between node problem detection tooling expressed as node conditions and remediation to achieve a node auto repairing feature.

### Unhealthy criteria:
A machine is unhealthy when:
- The referenced node meets the unhealthy node conditions criteria defined.
- The Machine has no nodeRef.
- The Machine has a nodeRef but the referenced node is not found.

If any of those criteria are met for longer than the given timeouts and the `maxUnhealthy` threshold has not been reached yet, the machine will be marked as failing the healthcheck.

Timeouts:
- For the node conditions the time outs are defined by the admin.
- For a machine with no nodeRef an opinionated value could be assumed e.g 10 min.

### Remediation:
- Remediation is not an integral part or responsibility of MachineHealthCheck. This controller only functions as a means for others to act when a Machine is unhealthy in the best way possible.

### User Stories

#### Story 1
- As a user of a Workload Cluster, I only care about my app's availability, so I want my cluster infrastructure to be self-healing and the nodes to be remediated transparently in the case of failures.

#### Story 2
As an operator of a Management Cluster, I want my machines to be self-healing and to be recreated, resulting in a new healthy node in the case of matching my unhealthy criteria.

### Implementation Details/Notes/Constraints

#### Machine conditions:
```go
const ConditionHealthCheckSucceeded ConditionType = "HealthCheckSucceeded"
const ConditionOwnerRemediated ConditionType = "OwnerRemediated"
```

- Both of these conditions are applied by the MHC controller. HealthCheckSucceeded should only be updated by the MHC after running the health check.
- If a health check passes after it has failed, the conditions will not be updated. When in-place remediation is needed we can address the challenges around this.
- OwnerRemediated is set to False after a health check fails, but should be changed to True by the owning controller after remediation succeeds.
- If remediation fails OwnerRemediated can be updated to a higher severity and the reason can be updated to aid in troubleshooting.

#### MachineHealthCheck CRD:
- Enable watching a group of machines (based on a label selector).
- Enable defining an unhealthy node criteria (based on a list of node conditions).
- Enable setting a threshold of unhealthy nodes. If the current number is at or above this threshold no further remediation will take place. This can be expressed as an int or as a percentage of the total targets in the pool.


E.g:
- I want a machine to be remediated when its associated node has `ready=false` or `ready=Unknown` condition for more than 5m.
- I want to disable auto-remediation if 40% or more of the matching machines are unhealthy.

```yaml
apiVersion: cluster.x-k8s.io/v1alpha3
kind: MachineHealthCheck
metadata:
  name: example
  namespace: machine-api
spec:
  selector:
    matchLabels:
      role: worker
  unhealthyConditions:
  - type:    "Ready"
    status:  "Unknown"
    timeout: "5m"
  - type:    "Ready"
    status:  "False"
    timeout: "5m"
  maxUnhealthy: "40%"
status:
  currentHealthy: 5
  expectedMachines: 5
```

#### MachineHealthCheck controller:
Watch:
- Watch machineHealthCheck resources
- Watch machines and nodes with an event handler e.g controller runtime `EnqueueRequestsFromMapFunc` which returns machineHealthCheck resources.

Reconcile:
- Fetch all the machines targeted by the MachineHealthCheck and operate over machine/node targets. E.g:
```
type target struct {
  Machine capi.Machine
  Node    *corev1.Node
  MHC     capi.MachineHealthCheck
}
```

- Calculate the number of unhealthy targets.
- Compare current number against `maxUnhealthy` threshold and temporary short circuits remediation if the threshold is met.
- Marks unhealthy target machines with the conditions as described above.

Out of band:
- The owning controller observes the OwnerRemediated=False condition and is responsible to remediate the machine.
- The owning controller performs any pre-deletion tasks required.
- The owning controller MUST delete the machine.
- The machine controller drains the unhealthy node.
- The machine controller provider deletes the instance.

![Machine health check](./images/machine-health-check/mhc.svg)

### Risks and Mitigations

## Contradictory signal

If a user configures healthchecks such that more than one check applies to
machines these healthchecks can potentially become a source of contradictory
information. For instance, one healthcheck could pass for a given machine,
while another fails. This will result in undefined behavior since the order in
which the checks are run and the timing of the remediating controller will
determine which condition is used for remediation decisions.

There are safeguards that can be put in place to prevent this but it is out of
scope for this proposal. Documentation should be updated to warn users to
ensure their healthchecks do not overlap.

## Alternatives

This proposal was originally adopted and implemented having the MHC do the
deletion itself. This was revised since it did not allow for more complex
controllers, for example the Kubeadm Control Plane, to account for the
remediation strategies they require.

It was also adopted in a later iteration using annotations on the machine.
However it was realized that this would amount to a privilege escalation
vector, as the `machines.edit` permission would grant effective access to
`machines.delete`. By using conditions, this requires `machines/status.edit`,
which is a less common privilege.

Considered to bake this functionality into machineSets.
This was discarded as different controllers than a machineSet could be owning the targeted machines.
For those cases as a user you still want to benefit from automated node remediation.

Considered allowing to target machineSets instead of using a label selector.
This was discarded because of the reason above.
Also there might be upper level controllers doing things that the MHC does not need to account for, e.g machineDeployment flipping machineSets for a rolling update.
Therefore considering machines and label selectors as the fundamental operational entity results in a good and convenient level of flexibility and decoupling from other controllers.

Considered a more strict short-circuiting mechanism decoupled from the machine health checker i.e machine disruption budget analogous to pod disruption budget.
This was discarded because it added an unjustified level of complexity and additional API resources.
Instead we opt for a simpler approach and will consider RFE that requires additional complexity based on real use feedback.

## Upgrade Strategy
This is an opt in feature supported by a new CRD and controller so there is no need to handle upgrades for existing clusters. 

## Additional Details

### Test Plan [optional]
Extensive unit testing for all the cases supported when remediating.

e2e testing as part of the cluster-api e2e test suite.

For failing early testing we could consider a test suite leveraging kubemark as a provider to simulate healthy/unhealthy nodes in a cloud agnostic manner without the need to bring up a real instance.

[testing-guidelines]: https://git.k8s.io/community/contributors/devel/sig-testing/testing.md

### Graduation Criteria [optional]
This propose the new CRD to belong to the same API group than other cluster-api resources, e.g machine, machineSet and to follow the same release cadence.

### Version Skew Strategy [optional]

## Implementation History
- [ ] 10/30/2019: Opened proposal PR
- [ ] 04/15/2020: Revised to move reconciliation to owner controller

<!-- Links -->
[community meeting]: https://docs.google.com/document/d/1Ys-DOR5UsgbMEeciuG0HOgDQc8kZsaWIWJeKJ1-UfbY
