---
title: Propagating taints from Cluster API to `Node`s
authors:
  - "@nrb"
reviewers:
  - "@JoelSpeed"
  - "@fabriziopandini"
  - "@sbueringer"
creation-date: 2025-05-13
last-updated: 2025-06-06
status: provisional
see-also:
  - 20221003-In-place-propagation-of-Kubernetes-objects-only-changes.md
---

# Propagating taints from `MachineTemplate`s to `Node`s

## Table of Contents

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Glossary](#glossary)
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals/Future Work](#non-goalsfuture-work)
- [Proposal](#proposal)
  - [User Stories](#user-stories)
    - [Story 0](#story-0)
    - [Story 1](#story-1)
    - [Story 2](#story-2)
    - [Story 3](#story-3)
  - [Requirements (Optional)](#requirements-optional)
    - [Functional Requirements](#functional-requirements)
      - [FR1](#fr1)
      - [FR2](#fr2)
  - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
      - [Proposed API changes](#proposed-api-changes)
  - [Security Model](#security-model)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Alternatives](#alternatives)
- [Upgrade Strategy](#upgrade-strategy)
- [Additional Details](#additional-details)
  - [Test Plan [optional]](#test-plan-optional)
  - [Graduation Criteria [optional]](#graduation-criteria-optional)
  - [Version Skew Strategy [optional]](#version-skew-strategy-optional)
- [Implementation History](#implementation-history)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Glossary

Refer to the [Cluster API Book Glossary](https://cluster-api.sigs.k8s.io/reference/glossary.html).

## Summary

Users should be able to taint `Node` resources created via Cluster API using Cluster API's higher order resources such as `MachineSet`s, `MachineDeployment`s, and `MachinePool`s.
These taints should be additive and continuously reconciled.
However, any taints that are not managed by Cluster API should be unchanged during reconciliation.

NOTE: This new proposal has been created rather than updating the prior [in-place metadata propagation](20221003-In-place-propagation-of-Kubernetes-objects-only-changes.md) proposal because taints are different enough from labels or annotations that a different set of constraints will need to be considered.
Very early versions of Kubernetes tracked taints as annotations, but they have long since been [promoted to their own API type](https://github.com/kubernetes/kubernetes/commit/9b640838a5f5e28db1c1f084afa393fa0b6d1166)

## Motivation

Users of Cluster API can currently update labels and annotations and have those values propagate from their high level resources all the way down to nodes.
While this is useful, it does not provider a way to, for example, reserve a set of nodes for specific workloads.

Doing so requires allowing users to specify taints for groups of Machines within the cluster, which this proposal aims to do.

### Goals

- Taints can be defined in a MachineTemplate and any resources that reference a template, then propagate down to the resources they manage.
- Taints defined on a `Machine` will ultimately propagate to the owned `Node`.
- Taints managed by Cluster API should not interfere with taints applied by other actors.

### Non-Goals/Future Work

- Supporting taints on individual devices via [Dynamic Resource Allocation](https://kubernetes.io/docs/concepts/scheduling-eviction/dynamic-resource-allocation/#device-taints-and-tolerations). This may be added in the future, but is currently out of scope.
- Supporting taints on cluster-level resources. Taints are a lower level concern, describing a subset of nodes within a given cluster, rather than cluster-wide metadata.

## Proposal

### User Stories

#### Story 0

In Kubernetes clusters, Taints and Tolerations are what allow a workload to be scheduled to a specific node.
As a user, I would like to use this community-standard mechanism within the framework that Cluster API provides.

#### Story 1

As a user, I wish to use Cluster API to manage a set of machines that have very specific characteristics for targetting workloads.
Some examples of this might be:
    - Designating nodes as `edge` nodes and steering locality-critical workloads only to `edge` nodes.
    - Designating nodes as having a particular hardware capability, such as high performance GPUs

#### Story 2

As a user, I wish to have autoscaling capabilities using Kubernets and Cluster API resources and conventions.
I would like for taints defined on a Cluster API resource representing some collection (including but not limited to `Cluster`s, `MachineSets`, `MachinePools`, and `MachineDeployments`).
This is especially useful in scale-from-zero scenarios, where that autoscaling technology can reference Taints on a collection to make decisions about the cloud resources available.

#### Story 3

As a user, I would like to update Taint metadata on my collection resources without forcing a complete replacement of an owned resource, such as a `Machine` or `Node`.

### Requirements (Optional)

#### Functional Requirements

Functional requirements are the properties that this design should include.

##### FR1

Users should be able to define Taints on collection resources and have the taints propagate to the owned resources.
This would start at a `ClusterClass` or `Cluster` level, and ultimately be written to a `Node`.

##### FR2

Users should be able to remove Taints managed by Cluster API without removing taints that Cluster API does not manage.

### Implementation Details/Notes/Constraints

Cluster API already supports propagating labels and annotations downward in its resource heirarchy.
This support is implemented such that when these fields are updated, the underlying compute resources are _not_ replaced.

Taints present a challenge to this, because they are defined as an "atomic" field by Kubernetes.[^1]
This means that when updating Taints on a `Node`, _all_ `Taint`s are replaced; it is not possible to add and replace individual elements like labels and annotations allow.
As a concrete example, if a node has 3 taints and some client submits a patch request with only one, the end result is one taint on the node.
It also means that Server-Side Apply ownership rules could not be applied to individual taints, which could present conflicts between controllers or users trying to modify `Taints` on the resultant `Node`.

For Cluster API to support propagating `Taint`s, it will need to implement its own mechanism for tracking what `Taint`s it owns.

##### Proposed API changes

The following changes are proposed to the Go API:

```go
type MachineSpec{
    // Taints are the node taints that Cluster API will manage.
    // This list is not necessarily complete: other Kubernetes components may add or remove other taints.
    // Only those taints defined in this list will be added or removed by Cluster API.
    // 
    // NOTE: This list is implemented as a "granular map" type, meaning that individual elements can be managed by different owners.
    //       As of Kubernetes 1.33, this is different from the upstream implementation, but is different in order to provide a more flexible API for components building on top of Cluster API.
    // +optional
    // +listType=map
	// +listMapKey=key
	// +listMapKey=effect
	// +mapType=granular
    Taints []Taint `json:"taints,omitempty"`
}
```

The MachineSet and MachineDeployment controllers will watch their associated MachineTemplate for updates to the `spec.Taints` field, update their managed Machines to reflect the new values.

The Machine controller will track taints on its node by adding a new annotation, `cluster.x-k8s.io/taints-from-machine`, to nodes in order to track taints managed by the Machine.
This follows the convention established by `cluster.x-k8s.io/labels-from-machine`.
The taints will be concatenated with `,` and the serialization will look like this given the 4 ways a taint can be specified:

`cluster.x-k8s.io/taints-from-machine:<key1>=<value1>:<effect1>,<key2>=<value2>:,<key3>:<effect3>,<key4>`

See the usptream [string implementation](https://github.com/kubernetes/kubernetes/blob/master/staging/src/k8s.io/api/core/v1/taint.go) for more details.

### Security Model

Users who can define `Taint`s that get placed on `Node`s will be able to steer workloads, possibly to malicious hosts in order to extract sensitive data.
However, users who can define Cluster API resources already have this capability - an attacker who receives the permissions to update a `MachineTemplate` could alter the definition in a similar manner.

This proposal therefore does not present any heightened security requirements than Cluster API already has.

### Risks and Mitigations

Managing the `Taint`s on `Node`s is considered a highly privileged action in Kubernetes; it even has its own top level `kubectl taint` command.
The Kubernetes scheduler uses taints rather than `Conditions` to decide when to evict workloads.
Updating these in place could then evict workloads unintentionally, or disrupt other systems that rely on taints being present.

This risk can be mitigated by ensuring Cluster API only modifies taints that it owns on nodes, as decribed in the implementaton above.

## Alternatives

Deciding whether or not to reconcile taint changes continuously has been a challenge for the Cluster API.
Historically, the v1alpha1 API included a `Machine.Taints` field.
However, since this field was mostly used in cluster bootstrapping, it was later extracted into bootstrap provider implementations.

Moving forward, two broad alternatives that have been explored in light of this: adding taints only at bootstrap time, and making taints immutable at the `Machine` level.
Both methods would require that a node be replaced in order to make any changes to the taints.

While this simplifies the implementation logic for Cluster API, it may be surprising to many users, since the Kubernetes documentation presents taints as a mutable field on a node.
This would also mean that there are two different behaviors when modifying metadata within Cluster API, which could again be very confusing.
There is already precedent for leaving infrastructure in-place when Kubernetes-only fields are modified, and this proposal seeks to align with the established function.

## Upgrade Strategy

Taint support is a net-new field, and therefore must be optional and not affect upgrades.

## Additional Details

[^1]: It is worth noting that there has been discussion about making the taints on a node a "map" list type and allowing for ownership of individual taints.
As of this writing, the [pull request](https://github.com/kubernetes/kubernetes/pull/128866) and [issue](https://github.com/kubernetes/kubernetes/issues/117142) remain open.
This proposal should be unaffected regardless of any upstream change to the handling of taints, except that using a "map" type would simplify the implementation and allow us to cooperate with other field managers.


### Test Plan [optional]

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

No need to outline all of the test cases, just the general strategy.
Anything that would count as tricky in the implementation and anything particularly challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage expectations).
Please adhere to the [Kubernetes testing guidelines][testing-guidelines] when drafting this test plan.

[testing-guidelines]: https://git.k8s.io/community/contributors/devel/sig-testing/testing.md

### Graduation Criteria [optional]

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal should keep
this high-level with a focus on what signals will be looked at to determine graduation.

Consider the following in developing the graduation criteria for this enhancement:
- [Maturity levels (`alpha`, `beta`, `stable`)][maturity-levels]
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

### Version Skew Strategy [optional]


## Implementation History

- [ ] MM/DD/YYYY: Proposed idea in an issue or [community meeting]
- [ ] MM/DD/YYYY: Compile a Google Doc following the CAEP template (link here)
- [ ] MM/DD/YYYY: First round of feedback from community
- [ ] MM/DD/YYYY: Present proposal at a [community meeting]
- [ ] MM/DD/YYYY: Open proposal PR

<!-- Links -->
[community meeting]: https://docs.google.com/document/d/1ushaVqAKYnZ2VN_aa3GyKlS4kEd6bSug13xaXOakAQI/edit#heading=h.pxsq37pzkbdq
