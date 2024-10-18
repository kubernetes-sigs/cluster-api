---
title: MachineDrainRules
authors:
- "@sbueringer"
reviewers:
- "@fabriziopandini"
- "@chrischdi"
- "@vincepri"
creation-date: 2024-09-30
last-updated: 2024-09-30
status: implementable
---

# MachineDrainRules

## Table of Contents

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Summary](#summary)
- [Motivation](#motivation)
  - [User Stories](#user-stories)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
  - [Future work](#future-work)
- [Proposal](#proposal)
  - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
    - [`MachineDrainRule` CRD](#machinedrainrule-crd)
      - [Example: Exclude Pods from drain](#example-exclude-pods-from-drain)
      - [Example: Drain order](#example-drain-order)
    - [Drain labels](#drain-labels)
    - [Node drain behavior](#node-drain-behavior)
    - [Changes to wait for volume detach](#changes-to-wait-for-volume-detach)
  - [Security Model](#security-model)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Alternatives](#alternatives)
- [Upgrade Strategy](#upgrade-strategy)
- [Additional Details](#additional-details)
  - [Test Plan](#test-plan)
  - [Graduation Criteria](#graduation-criteria)
  - [Version Skew Strategy](#version-skew-strategy)
- [Implementation History](#implementation-history)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Summary

Today, when Cluster API deletes a Machine it drains the corresponding Node to ensure all Pods running on the Node have 
been gracefully terminated before deleting the corresponding infrastructure. The current drain implementation has 
hard-coded rules to decide which Pods should be evicted. This implementation is aligned to `kubectl drain` (see 
[Machine deletion process](https://main.cluster-api.sigs.k8s.io/tasks/automated-machine-management/machine_deletions) 
for more details).

With recent changes in Cluster API, we can now have finer control on the drain process, and thus we propose a new 
`MachineDrainRule` CRD to make the drain rules configurable per Pod. Additionally, we're proposing labels that 
workload cluster admins can add to individual Pods to control their drain behavior.

This would be a huge improvement over the “standard” `kubectl drain` aligned implementation we have today and help to 
solve a family of issues identified when running Cluster API in production.

## Motivation

While the “standard” `kubectl drain` rules have served us well, new user stories have emerged where a more sophisticated 
behavior is needed.

### User Stories

**Story 1: Define drain order**

As a cluster admin/user, I would like to define in which order Pods are evicted. For example, I want to configure that 
Portworx Pods are evicted after all other Pods ([CAPI-11024](https://github.com/kubernetes-sigs/cluster-api/issues/11024)).

Note:  As of today the only way to address this use case with Cluster API is to implement a pre-drain hook with a custom 
drain process. Due to the complexity of this solution it is not viable for most users.

**Story 2: Exclude “undrainable” Pods from drain**

As a cluster admin/user, I would like the node drain process to ignore pods that will obviously not drain properly 
(e.g., because they're tolerating the unschedulable or all taints).

Note: As of today Pods that are tolerating the `node.kubernetes.io/unschedulable:NoSchedule` taint will just get rescheduled 
to the Node after they have been evicted, and this can lead to race conditions and then prevent the Machine to be deleted.

**Story 3: Exclude Pods that should not be drained from drain**

As a cluster admin/user, I would like to instruct the Node drain process to exclude some Pods, e.g. because I don't 
care if they're gracefully terminated and they should run until the Node goes offline (e.g. monitoring agents).

**Story 4: Define drain order & exclude Pods from drain without management cluster access**

As a workload cluster admin/user, I want to be able to define drain order of Pods and also exclude Pods from drain 
(similar to Story 1-3, just without management cluster access).

**Story 5: Apply drain configuration to a set of Machines / Clusters**

As a cluster admin/user, I would like to be able to configure the drain behavior for all Machines of a management 
cluster or for a subset of these Machines without having to configure each Machine individually. I would also like 
to be able to change the drain configuration without having to modify all Machine objects.

### Goals

* Allow users to customize how/if Pods are evicted:
  * Allow to define the order in which Pods are evicted
  * Allow to define if Pods should be excluded from drain
* Use the current drain default rules as a fallback if neither `MachineDrainRule` CRs nor drain annotations
  apply to a Pod (this also ensures compatibility with current behavior)
* Stop waiting for volume detachment for Pods that are excluded from drain

### Non-Goals

* Change the drain behavior for DaemonSet and static Pods (we’ll continue to skip draining for both).
  While the drain behavior itself won't be changed, we will stop waiting for detachment of volumes of DaemonSet Pods.

### Future work

* Align with the evolving landscape for Node drain in Kubernetes as soon as the new features are available, 
  stable and can be used for the Kubernetes versions Cluster API supports (e.g. [KEP-4212](https://github.com/kubernetes/enhancements/issues/4212), [KEP-4563](https://github.com/kubernetes/enhancements/issues/4563)).

  Because of the wide range of workload cluster Kubernetes versions supported in Cluster API (n-5), it will take 
  significant time until we can use new Kubernetes features in all supported versions. So an implementation in
  Cluster API is the only option to improve drain anytime soon (even for the oldest supported Kubernetes versions).

  Also, please note that while we would like to align with Kubernetes and leverage its new features as soon as 
  possible, there will always be a delta to be addressed in Cluster API due to the fact that Cluster API is operated 
  from the management cluster, while drain is done in workload clusters.

  Additionally, Cluster API should take care of many clusters, while Kubernetes features are by design scoped to a single cluster.

## Proposal

### Implementation Details/Notes/Constraints

#### `MachineDrainRule` CRD

We propose to introduce a new `MachineDrainRule` namespace-scoped CRD. The goal of this CRD is to allow management 
cluster admins/users to define a drain rule (a specific drain behavior) for a set of Pods.

Additionally, drain rules can be restricted to a specific set of Machines, e.g. targeting only controlplane Machines, 
Machines of specific Clusters, Linux Machines, etc.

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: MachineDrainRule
metadata:
  name: example-rule
  namespace: default
spec:
  drain:
    # behavior can be Drain or Skip
    behavior: Drain 
    # order can only be set if behavior == Drain
    # Pods with higher order are drained later
    order: 100
  machines:
  - selector:
      matchLabels:
      matchExpressions:
    clusterSelector:
      matchLabels:
      matchExpressions:
  pods:
  - selector:
      matchLabels:
      matchExpressions:
    namespaceSelector:
      matchLabels:
      matchExpressions:
```

Notes:

* The Machine controller will evaluate `machines` and `pods` selectors to determine for which Machine and to which Pods
  the rule should be applied.
* All selectors are of type `metav1.LabelSelector`
* In order to allow expressing selectors with AND and OR predicates, both `machines` and `pods` have a list of 
  selectors instead of a single selector. Within a single element of the selector lists AND is used, while between multiple 
  entries in the selector lists OR is used.

##### Example: Exclude Pods from drain

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: MachineDrainRule
metadata:
  name: skip-pods
  namespace: default
spec:
  drain:
    behavior: Skip
  machines:
  - selector: {}
  pods:
   # This selects all Pods with the app label in (example-app1,example-app2) AND in the example-namespace 
  - selector:  
      matchExpressions:
      - key: app
        operator: In
        values:
        - example-app1
        - example-app2
    namespaceSelector:
      matchLabels:
        kubernetes.io/metadata.name: example-namespace
  # This additionally selects Pods with the app label == monitoring AND in the monitoring namespace
  - selector:
      matchExpressions:
        - key: app
          operator: In
          values:
            - monitoring
    namespaceSelector:
      matchLabels:
        kubernetes.io/metadata.name: monitoring
```

##### Example: Drain order

(related to [CAPI-11024](https://github.com/kubernetes-sigs/cluster-api/issues/11024))

```yaml
apiVersion: cluster.x-k8s.io/v1beta1
kind: MachineDrainRule
metadata:
  name: portworx
  namespace: default
spec:
  drain:
    behavior: Drain
    order: 100
  machines:
  - selector: {}
  pods:
  - selector:
      matchLabels:
        app: portworx
```

#### Drain labels

We propose to introduce new labels to allow workload cluster admins/users to define drain behavior 
for Pods. These labels would be either added directly to Pods or indirectly via Deployments, StatefulSets, etc.
The labels will take precedence over `MachineDrainRules` specified in the management cluster.

* `cluster.x-k8s.io/drain: skip`

Initially we also considered adding a `cluster.x-k8s.io/drain-order` label. But we're not entirely sure about it 
yet as someone who has access to the workload cluster (or maybe only specific Pods) would be able to influence the 
drain order of the entire cluster, which might lead to problems. The skip label is safe in comparison because it 
only influences the drain behavior of the Pod that has the label.

#### Node drain behavior

The following describes the new algorithm that decides which Pods should be drained and in which order.

* List all Pods on the Node
* For each Pod (first match applies):
  * If the Pod is a DaemonSet Pod or a static Pod
    * \=\> use `behavior: Skip`
  * If the Pod has `cluster.x-k8s.io/drain: skip`
    * \=\> use `behavior: Skip`
  * If there is a matching `MachineDrainRule`
    * \=\> use `behavior` and `order` from the first matching `MachineDrainRule` (based on alphabetical order)
  * Otherwise:
    * \=\> use `behavior: Drain` and `order: 0`
* If there are no more Pods to be drained
  * \=\> drain is completed
* Else:
  * \=\> evict all Pods with the lowest `order`, update condition and requeue

Notes:

* It does not make sense to evict DaemonSet Pods, because the DaemonSet controller always [adds tolerations for the 
  unschedulable taint to all its Pods](https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/#taints-and-tolerations)
  so they would be just immediately rescheduled.
* It does not make sense to evict static Pods, because the kubelet won’t actually terminate them and the Pod object 
  will get recreated.
* If multiple `MachineDrainRules` match the same Pod, the “first” rule (based on alphabetical order) will be applied
  * Alphabetical order is used because:
    * There is no way to ensure only a single `MachineDrainRule` applies to a specific Pod, because not all Pods and their 
      labels are known when `MachineDrainRules` are created/updated especially as the Pods will also change over time. 
    * So a criteria is needed to decide which `MachineDrainRule` is used when multiple `MachineDrainRules` apply
    * For now, we decided to use simple alphabetical order. An alternative would have been to add a `priority` field
      to `MachineDrainRules` and enforce that each priority is used only once, but this is not entirely race condition
      free (when two `MachineDrainRules` are created "at the same time") and adding "another" priority field on top
      of `order` can be confusing to users.
* `Pods` with drain behavior `Drain` and given order will be evicted only after all other `Pods` with a lower order 
  have been already deleted
* When waiting for volume detach, volumes belonging to `Pods` with drain behavior `Skip` are going to be ignored.
  Or in other words, Machine deletion can continue if only volumes belonging to `Pods` with drain behavior `Skip`
  are still attached to the Node.

#### Changes to wait for volume detach

Today, after Node drain we are waiting for **all** volumes to be detached. We are going to change that behavior to ignore
all attached volumes that belong to Pods for which we skipped the drain.

Please note, today the only Pods for which we skip drain that can have volumes are DaemonSet Pods. If a DaemonSet Pod 
has a volume currently wait for volume detach would block indefinitely. The only way around this today is to set either
the `Machine.spec.nodeVolumeDetachTimeout` field or the `machine.cluster.x-k8s.io/exclude-wait-for-node-volume-detach`
annotation. With this change we will stop waiting for volumes of DaemonSet Pods to be detached.

**Note**: We can't exclude the possibility that with some CSI implementations deleting Machines while volumes are still attached
will lead to problems. So please use this carefully. However, we verified that the `kube-controller-manager` will handle these
situations correctly by ensuring the volume gets freed up after the Machine & Node have been deleted and can be used again.

### Security Model

This proposal will add a new `MachineDrainRule` CRD. The Cluster API core controller needs permissions to read this CRD.
While permissions to write the CRD would be typically given to the cluster admin (the persona who has permissions to manage
Cluster CRs), it is also possible to further restrict access.

### Risks and Mitigations

* There is ongoing work in Kubernetes in this area that we should eventually align with ([KEP-4212](https://github.com/kubernetes/enhancements/issues/4212),
  [KEP-4563](https://github.com/kubernetes/enhancements/issues/4563))
  * We are going to provide feedback to the KEPs to try to make sure the same use cases as in the current proposal are covered.

## Alternatives

**Add Node drain configuration to the Machine object**

We considered adding the drain rules directly to the Machine objects (and thus also to the MachineDeployment & MachineSet objects) 
instead. We discarded this option because it would have made it necessary to configure Node drain on every single Machine. 
By having a separate CRD it is now possible to configure the Node drain for all Clusters / Machines or a specific subset 
of Machines at once. This also means that the Node drain configuration can be immediately changed without having to propagate 
configuration changes to all Machines.

**MachineDrainRequestTemplates**
We considered defining separate MachineDrainRequest and MachineDrainRequestTemplate CRDs. The MachineDrainRequestTemplates 
would be used to define the drain behavior. When the Machine controller starts a drain a MachineDrainRequest would be 
created based on the applicable MachineDrainRequestTemplates. The MachineDrainRequest would then be used to reconcile 
and track the drain. Downside would be that if you have a lot of templates, you make a really large custom resource 
that is unwieldy to manage. Additionally it is not clear if it's necessary to have a separate MachineDrainRequest CRD, 
it's probably easier to track the drain directly on the Machine object.

## Upgrade Strategy

`MachineDrainRules` are orthogonal to the state of the Cluster / Machines as they only configure how the Machine controller 
should drain Nodes. Accordingly, they are not part of the Machine specs. Thus, as soon as the new Cluster API version that 
supports this feature is deployed, `MachineDrainRules` can be immediately used without rolling out / re-configuring any 
Clusters / Machines.

Please note that while the drain behavior of DaemonSet Pods won't change (we’ll continue to skip draining),
we will stop waiting for detachment of volumes of DaemonSet Pods.

## Additional Details

### Test Plan

* Extensive unit test coverage for the Node drain code for all supported cases
* Extend the Node drain e2e test to cover draining Pods using various `MachineDrainRules` and the labels
  (including validation of condition messages).

### Graduation Criteria

The `MachineDrainRules` CRD will be added as `v1beta1` CRD to the `cluster.x-k8s.io` apiGroup.  
An additional feature gate is not required as the behavior of the Machine controller will stay the same if neither 
`MachineDrainRule` CRDs nor the labels are used.

### Version Skew Strategy

No limitations apply, the new feature will not depend on any external APIs.

## Implementation History

- [x] 2024-09-30: Open proposal PR
- [ ] 2024-10-02: Present proposal at a community meeting
