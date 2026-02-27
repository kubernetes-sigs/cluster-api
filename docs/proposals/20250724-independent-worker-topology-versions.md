---
title: Independent Worker Topology Versions
authors:
  - "@ivelichkovich"
  - "@alejandroesc"
  - "@acidleroy"
reviewers:
  - "@janedoe"
creation-date: 2025-07-29
last-updated: 2025-07-29
status: provisional|experimental|implementable|implemented|deferred|rejected|withdrawn|replaced
---

# Independent Worker Topology Versions

## Table of Contents

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Glossary](#glossary)
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
    - [Switching between cluster-level and independent Kubernetes versions when versions don't match](#switching-between-cluster-level-and-independent-kubernetes-versions-when-versions-dont-match)
    - [Allowing version skews that are unsupported by kubernetes](#allowing-version-skews-that-are-unsupported-by-kubernetes)
    - [Changing behavior of cluster-level worker node group configuration rollouts](#changing-behavior-of-cluster-level-worker-node-group-configuration-rollouts)
    - [Add builtin version-aware patches for independent worker node groups](#add-builtin-version-aware-patches-for-independent-worker-node-groups)
  - [Future Work](#future-work)
- [Proposal](#proposal)
  - [Overview](#overview)
  - [User Stories](#user-stories)
    - [Story 1](#story-1)
    - [Story 2](#story-2)
    - [Story 3](#story-3)
    - [Story 4](#story-4)
    - [Story 5](#story-5)
  - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
  - [Security Model](#security-model)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Alternatives](#alternatives)
  - [Raw MachineDeployments/MachinePools](#raw-machinedeploymentsmachinepools)
  - [Multiple clusters instead of multiple worker groups](#multiple-clusters-instead-of-multiple-worker-groups)
- [Upgrade Strategy](#upgrade-strategy)
- [Additional Details](#additional-details)
  - [Test Plan](#test-plan)
    - [Scenario: Existing behavior with ClusterClass Topology Version](#scenario-existing-behavior-with-clusterclass-topology-version)
    - [Scenario: Single cluster uses Worker Node Group Version for the First Time](#scenario-single-cluster-uses-worker-node-group-version-for-the-first-time)
    - [Scenario: Cluster Version Topology Upgraded on a Cluster with Independent Worker Node Group Versions](#scenario-cluster-version-topology-upgraded-on-a-cluster-with-independent-worker-node-group-versions)
    - [Scenario: Upgrading Both Cluster Version Topology and Independent Worker Node Group Versions on Existing Cluster](#scenario-upgrading-both-cluster-version-topology-and-independent-worker-node-group-versions-on-existing-cluster)
    - [Scenario: ClusterClass Change with Version Upgrades](#scenario-clusterclass-change-with-version-upgrades)
    - [Scenario: Failed Upgrade Recovery](#scenario-failed-upgrade-recovery)
    - [Scenario: Configuration and Scale Changes During Upgrades](#scenario-configuration-and-scale-changes-during-upgrades)
    - [Scenario: Remediation During Upgrades](#scenario-remediation-during-upgrades)
    - [Scenario: Auto-scaler Operations During Upgrades](#scenario-auto-scaler-operations-during-upgrades)
    - [Scenario: Admission Control and Validation](#scenario-admission-control-and-validation)
    - [Scenario: Feature Flag Behavior](#scenario-feature-flag-behavior)
    - [Scenario: Chained Upgrades](#scenario-chained-upgrades)
    - [Scenario: In Place Upgrades](#scenario-in-place-upgrades)
  - [Graduation Criteria [optional]](#graduation-criteria-optional)
  - [Version Skew Strategy [optional]](#version-skew-strategy-optional)
- [Implementation History](#implementation-history)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Glossary

Refer to the [Cluster API Book Glossary](https://cluster-api.sigs.k8s.io/reference/glossary.html).

**Worker Node Group Version**: An optional Kubernetes version field that can be specified for individual MachineDeploymentTopology or MachinePoolTopology objects within a Cluster's topology. When set, this version overrides the cluster-wide topology version for that specific worker node group, enabling independent version management and upgrade scheduling for different sets of worker nodes.

## Summary

This proposal enables Kubernetes clusters managed with ClusterClass to specify and support version skew between control plane and worker node groups, as well as among individual worker node groups, within the limits of Kubernetes' version skew policy. By allowing independent Kubernetes versions for each worker node group, cluster administrators gain flexible, granular upgrade control and can reduce operational disruptions, all while retaining the benefits of ClusterClass management.

## Motivation

Today, ClusterClass enforces a single Kubernetes version across the entire cluster topology. 
This design simplifies version management but introduces significant operational friction during upgrades—particularly 
for larger or more heterogeneous clusters.

Upgrades are performed in a managed sequence: first the control plane, then each worker node group in turn. 
While an upgrade is in progress, operations on worker node groups (e.g., scaling, remediation, configuration changes) are blocked, 
even if unrelated to the upgrade. This tightly coupled behavior creates bottlenecks for teams that require fine-grained control 
over their upgrade cadence or must accommodate workloads with varying availability guarantees.

For example, cluster operators managing mission-critical inference workloads alongside long-running training jobs may need 
to stagger upgrades or selectively upgrade only a subset of nodes. In the current model, this level of control is not possible 
without abandoning ClusterClass altogether or creating multiple clusters—each of which increases operational complexity and cost.

In addition, we are motivated by modern Artificial Intelligence workflows in which a custom scheduler can move nodes from one worker 
node group to another. This type of activity cannot be done easily if multiple clusters are used. Again, this emphasizes the need
to keep workflows contained in single cluster and not spread versions over multiple clusters.

This proposal introduces an enhancement to support version skew between worker node groups in ClusterClass-managed clusters, enabling:

    Safe, independent upgrades of individual worker node groups;

    Reduced operational blast radius during upgrades;

    Continued cluster operations (e.g., autoscaling, remediation) in unaffected worker node groups;

    Greater flexibility for cluster vendors and multi-tenant platform teams who must balance upgrade safety with workload uptime guarantees.

By decoupling worker node group upgrades from the global cluster version, we preserve the high-level abstraction and usability
of ClusterClass while introducing operational flexibility needed for production environments at scale.

The broader motivation aligns with the goals of Cluster API: declarative infrastructure, safe upgrades, and supporting real-world
platform needs without requiring users to abandon upstream tools or adopt brittle workarounds.

Optionally, future experience reports can be gathered from teams running large-scale Kubernetes fleets with mixed workloads
(e.g., inference vs training, customer A/B rollout clusters) to further validate this direction.


### Goals

- Maintain existing behavior of ClusterClass versions.
- Support upgrades of any single worker node group independent of the control plane and other worker node groups.
- Operations on worker node groups *not* upgrading should continue to operate normally while an independent worker node group is upgrading.
- Support version skew, as allowed by Kubernetes' [Version Skew Policy](https://kubernetes.io/releases/version-skew-policy/), between worker node groups and control planes. 
- Maintain supported skew checks and guardrails for Kubernetes versions
- Maintain the ease of use of ClusterClass while providing this added flexibility
- Support switching back to cluster-level Kubernetes versions if all independent worker node groups and the control plane are at the same version.
- Gracefully handle [version-aware patches](https://cluster-api.sigs.k8s.io/tasks/experimental-features/cluster-class/write-clusterclass#version-aware-patches) (i.e.  ClusterClass variables can reference `builtin.cluster.topology.version`, `builtin.controlPlane.version`, `builtin.machineDeployment.version`, and `builtin.machinePool.version`)
- Update documentation in the [cluster-api book](https://cluster-api.sigs.k8s.io/tasks/experimental-features/cluster-class/operate-cluster#upgrade-a-cluster) to describe the alternative approach to upgrading cluster versions.

### Non-Goals

#### Switching between cluster-level and independent Kubernetes versions when versions don't match
To minimize potential edges cases, if a cluster vendor wishes to use worker node group versioning, they cannot switch back without ensuring that the independent version matches the version that is the cluster.spec.topology version. Otherwise, they will be denied and asked to take manual steps to get the control versions the same. 

#### Allowing version skews that are unsupported by kubernetes
Only version skew allowable by Kubernetes [Version Skew Policy](https://kubernetes.io/releases/version-skew-policy/) can be implemented. 

#### Changing behavior of cluster-level worker node group configuration rollouts
Rollouts for cluster-level behavior will be preserved. 

#### Add builtin version-aware patches for independent worker node groups
For this iteration, we will not be implementing version-aware patches for independent worker node groups. 

### Future Work

- Allow removal of worker group version overrides when all worker nodes can be upgraded to match the control plane’s version. For example, if the control plane is running 1.31.x and all independent worker node groups are at 1.30.x, a cluster vendor can eliminate the worker group version specifications. This will trigger all worker groups to upgrade to 1.31.x and effectively revert to cluster-level version management.
- Add version-aware patches for independent worker node groups.

## Proposal

### Overview

This proposal introduces independent worker topology versions to clusters managed by `ClusterClass` through the following changes:

- Add webhooks that validate:
  - allowable version skew
  - that versions cannot be edited on clusters created after the feature flag has been turned off
  - that versions managed by independent worker node groups can be moved back to cluster-level versioning if there is no version skew and denied otherwise
- Remove preflight check for version matching when the worker topology version is set
- Remove workers with worker version set from controller logic listing workers to upgrade

The implementation adds a version field directly to both topology structs:

```go
type MachinePoolTopology struct {
  /*
    Existing fields omitted
  */

	// Version specifies the Kubernetes version for this MachinePool worker node group.
	// When set, this version overrides the cluster-wide version specified in Cluster.Spec.Topology.Version
	// for this specific MachinePool, enabling independent version management and upgrade scheduling.
	// 
	// Version skew constraints as defined by Kubernetes' Version Skew Policy are enforced between
	// this worker node group version and the control plane version. The worker nodes cannot be more
	// than 2 minor versions behind the control plane (e.g., control plane at v1.30.x supports
	// worker nodes down to v1.28.x).
	//
	// This field enables use cases such as:
	// - Staggered upgrades across different worker node groups
	// - Testing upgrades on a subset of worker nodes before rolling out cluster-wide
	// - Managing different upgrade cadences for workloads with varying availability requirements
	//
	// If not specified, the MachinePool will use the version from Cluster.Spec.Topology.Version.
	// +optional
	// +featureGate=IndependentWorkerTopologyVersion
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Version *string `json:"version,omitempty"`
}

type MachineDeploymentTopology struct {
  /*
    Existing fields omitted
  */

	// Version specifies the Kubernetes version for this MachineDeployment worker node group.
	// When set, this version overrides the cluster-wide version specified in Cluster.Spec.Topology.Version
	// for this specific MachineDeployment, enabling independent version management and upgrade scheduling.
	//
	// See MachinePoolTopology.Version for more details 
  //
	// If not specified, the MachineDeployment will use the version from Cluster.Spec.Topology.Version.
	// +optional
	// +featureGate=IndependentWorkerTopologyVersion
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Version *string `json:"version,omitempty"`
}
```
This functionality will be initially protected by a feature flag, following the same approach as the ClusterTopology flag. For example, if a cluster is created with the flag enabled and the flag is later removed from the controller, the controller webhooks will reject any version changes to the created cluster both for `topology.version` and `machinedeployment/machinepool.version`. We will follow the the guide for [adding a new field in existing API version](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api_changes.md#new-field-in-existing-api-version)

Ideas: 
- [x] - Add a Kubernetes version to the [MachineDeploymentTopology spec](https://github.com/kubernetes-sigs/cluster-api/blob/ead3737e81eff878355a31e8aea6416ecdc0598c/api/core/v1beta2/cluster_types.go#L813-L889).
- [x] - Add a Kubernetes version to the [MachinePoolTopology spec][https://github.com/kubernetes-sigs/cluster-api/blob/ead3737e81eff878355a31e8aea6416ecdc0598c/api/core/v1beta2/cluster_types.go#L1123-L1178]
- [ ] - Update documentation on the [Topology.Version spec](https://github.com/kubernetes-sigs/cluster-api/blob/ead3737e81eff878355a31e8aea6416ecdc0598c/api/core/v1beta2/cluster_types.go#L545-L549)



### User Stories


#### Story 1
As a cluster vendor, I would like to be able to upgrade/patch my control planes 
independent of worker node groups. Control-plane upgrades are non-disruptive
while worker node group upgrades can be disruptive to customers using the cluster and need to be coordinated with them.

#### Story 2
As a cluster vendor, I want to be able to allow customers to schedule patching cycles across their fleet of worker node groups. 
They may have multiple worker node groups used for different use cases in the same cluster which require different upgrade behavior and timing.
An example of this is a cluster that is used for both critical customer facing inference and uninterruptable training jobs. 

#### Story 3
As a cluster vendor, I would like to perform upgrades to a worker node group, 
in a large cluster with many worker node groups, and I do not want other worker node groups 
to be in a frozen state while upgrades to one worker node group are on-going. These worker node groups could be very large and slowly be 
rolling out an upgrade which could take multiple days per group.

#### Story 4
As a cluster vendor, I want to upgrade a single worker node group of potentially many, 
so that I can test the changes with a smaller blast radius 
before I roll it out globally through the cluster.

#### Story 5
As a cluster user, I have multiple training jobs running that could run potentially for weeks at a time across different node groups. I want to coordinate the upgrades of the worker groups in between training runs. I can't afford to wait for all jobs across groups to stop and then wait for all groups to upgrade because they're running at different intervals and the wasted cost of GPUs would be too much. 


### Implementation Details/Notes/Constraints

- validating webhook for flows outlined throughout doc and test cases for new validation cases when using independent worker topology versions
- in exp/topology/desiredstate/desired_state.go, when using independent worker topology versions simply returned the explicit worker topology version
- when using independent worker topology versions, don't include the `controlPlaneVersionPreflightCheck` for that topology group
- add preflightcheck with the additional validation outlined in this doc for `cluster.spec.topology.version`  
- make sure independent worker topology versions are excluded from computations and logic for the cluster.spec.topology.version upgrades
  - for example in exp/topology/desiredstate/desired_state.go make sure they're excluded from the Upgrading machinedeployments list
- very very rough draft with some base functionality: https://github.com/kubernetes-sigs/cluster-api/pull/12515/files 


### Security Model

Document the intended security model for the proposal, including implications
on the Kubernetes RBAC model. Questions you may want to answer include:

* Does this proposal implement security controls or require the need to do so?
  * If so, consider describing the different roles and permissions with tables.
* Are their adequate security warnings where appropriate (see https://shostack.org/files/papers/ReederEtAl_NEATatMicrosoft.pdf for guidance).
* Are regex expressions going to be used, and are their appropriate defenses against DOS.
* Is any sensitive data being stored in a secret, and only exists for as long as necessary?

### Risks and Mitigations

- What are the risks of this proposal and how do we mitigate? Think broadly.
- How will UX be reviewed and by whom?
- How will security be reviewed and by whom?
- Consider including folks that also work outside the SIG or subproject.

## Alternatives

There are two possible alternatives to supporting independent worker topology versions

### Raw MachineDeployments/MachinePools

Admins can join MachineDeployments not using topology directly to topology based clusters. These would be fully independently managed. The drawbacks here
are:
- you would lose some of the guardrails that you would gain by having independent version support in worker topology
  - i.e. minor version skew safety
- you would lose worker classes which are a valuable feature for consistency and fleet management

### Multiple clusters instead of multiple worker groups

Admins could create multiple clusters, one for each worker group, rather than having multiple worker groups in the same cluster. This way the 
upgrades of the different worker groups wouldn't impact the other worker groups as they're in completely different clusters. There's many drawbacks
here:
- there's legitimate reasons to have multiple groups in the same cluster
  - i.e. you have AI training and inference in the same cluster. You may have a node group for inference and a node group for training. When you have high priority training it is allowed to consume a portion of the inference
    nodes to improve the training speed and later that is returned to the inference nodes when training is complete
- increased compute cost of additional control planes
- shared services across a multi-tenant environment in same cluster
- single cluster for configuration management
  - advanced users know how to propogate configurations across clusters but it comes with additional complexity that some customers may be averse to
- even with multiple clusters cannot manage the control plane and worker versions independently

## Upgrade Strategy

Will not have impact in upgrading capi, feature flag is needed and the api must be used to have any effect.

## Additional Details

### Test Plan

#### Scenario: Existing behavior with ClusterClass Topology Version
When a cluster admin updates the `spec.topology.version` of their existing cluster with *no* worker node group versions defined, the default behavior should be verified. 
- Ensure rolling upgrades of control planes and Workers are updated to the desired version. 

#### Scenario: Single cluster uses Worker Node Group Version for the First Time
When a cluster admin adds a new version to a specific MachineDeployment/MachinePool Topology, the controller should: 
- Validate that the upgrade is supported (allowable version skew)
- A rolling upgrade of the target MachineDeployment/MachinePool should take place.
- Remediation and other modifications/upgrades on control planes and MachineDeployment/MachinePools not managed by the version change should occur as usual, meaning the independt worker group upgrading will not block other operations.
  - Both old and new worker group versions must be within supported skew if changing cluster.spec.topology.

#### Scenario: Cluster Version Topology Upgraded on a Cluster with Independent Worker Node Group Versions
When a cluster admin updates the version on `spec.topology.version` on a cluster which has independent worker node group versions defined, the controller should: 
- Validate that the upgrade is supported (allowable version skew).
  - This now has to take into account that there may be none or many independent worker node group versions defined within the cluster and validate the skew on each one. 
- Remediation should be paused **only** for worker node groups that use the cluster-level version (those that **do not** have independent worker node group versions defined). Control planes will by default use only the version specified in the `spec.topology.version`. 
- A rolling upgrade of control planes and all worker node groups that use the cluster-level version should take place. 
- Remediation and other actions on the MachineDeployment/MachinePool topologies with independent worker node group versions should occur as usual. 

#### Scenario: Upgrading Both Cluster Version Topology and Independent Worker Node Group Versions on Existing Cluster
When a cluster admin simultaneously upgrades both the cluster's `Cluster.spec.topology.version` and one or more MachineDeployment/MachinePool independent worker node group versions, the controller should: 

- Validate that all version transitions are allowed according to Kubernetes' version skew policy:
  - No downgrades are permitted for any component
  - The maximum minor version skew between control plane and any worker node group must not exceed 2
    - it should validate against old and new versions
  - All intermediate upgrade states must maintain supported version skew
- Perform upgrades in the correct order: control plane nodes first, then worker node groups. Meaning workers with independent version settings
  would wait for the control plane to complete upgrade before upgrading themselves.
- Block the operation if any combination would violate version skew policy at any point during the upgrade

The validation must check version skew between these component pairs:
- Current control plane version and incoming control plane version
- Current worker node group versions and incoming worker node group versions  
- Current control plane version and incoming worker node group versions
- Incoming control plane version and current worker node group versions 

**Examples** (where y represents the minor version in semver MAJOR.MINOR.PATCH format):

| Current Cluster Version | Current Worker Version | Incoming Cluster Version | Incoming Worker Version | Allowed? | Comments |
|-------------------------|------------------------|--------------------------|-------------------------|----------|----------|
| v1.y.z | v1.y.z | v1.(y+1).z | v1.(y+1).z | Yes | Standard synchronized upgrade. Control plane upgrades first (y → y+1), creating temporary skew of 1, then workers upgrade. |
| v1.y.z | v1.(y+1).z | v1.(y+1).z | v1.(y+2).z | Yes | Control plane catches up to workers first (y → y+1), then workers advance (y+1 → y+2). Max skew remains ≤ 2. |
| v1.(y+1).z | v1.y.z | v1.(y+2).z | v1.(y+1).z | No | Control plane upgrade (y+1 → y+2) would create skew of 2 with current workers at y, exceeding policy. |
| v1.y.z | v1.y.z | v1.(y+2).z | v1.(y+1).z | No | Control plane cannot skip minor versions (y → y+2 is invalid). |
| v1.(y+1).z | v1.(y+1).z | v1.y.z | v1.y.z | No | Downgrades are not permitted. |
| v1.y.z | v1.(y+1).z | v1.y.z | v1.(y+2).z | No | Not permitted because control plane version and incoming worker node group version exceeds 2. |

**Additional edge cases to consider:**
- Upgrade failures that leave the cluster in an intermediate state
  - treated as upgrade in progress until failure resolved
- Worker group already in progress of upgrade when cluster.spec.topology is updated
  - let both updates run simultaneously 

#### Scenario: ClusterClass Change with Version Upgrades
When a cluster admin changes the `Cluster.spec.topology.class` while simultaneously updating versions, the controller should maintain current behavior and batch the changes together.

#### Scenario: Failed Upgrade Recovery
When an upgrade fails partway through it will be considered as in progress until it completes.


#### Scenario: Configuration and Scale Changes During Upgrades
When a cluster admin makes configuration or scale changes to worker node groups with independent versions while upgrades are ongoing, the controller should:

- Allow configuration changes to propagate to worker node groups with independent versions regardless of whether:
  - A cluster-level upgrade is in progress
  - An independent worker node group upgrade is in progress on the same or different worker node groups
- Trigger rolling upgrades for the modified worker node groups immediately
- Not block or freeze the configuration/scale changes based on upgrade state
- Continue to enforce version skew validation for any new machines created during scaling operations

This ensures that operational changes are not blocked by ongoing upgrade operations when using independent versioning.

#### Scenario: Remediation During Upgrades
When machine remediation is triggered on worker node groups with independent versions during various upgrade states, the controller should:

**Allowed Remediation Cases:**
- Allow machine creation/deletion/remediation when the current version skew is within Kubernetes policy limits
- Continue remediation operations regardless of whether cluster-level or independent upgrades are ongoing

**Blocked Remediation Cases:**
- Block machine creation/deletion when remediation would result in unsupported version skew
- Provide clear error messages indicating why remediation is blocked
- Resume remediation automatically once version skew constraints are satisfied

**Test cases should cover:**
- Remediation during cluster-level upgrades
- Remediation during independent worker node group upgrades
- Remediation when multiple worker node groups have different versions
- Remediation scenarios with both supported and unsupported version skew combinations

#### Scenario: Auto-scaler Operations During Upgrades

This seems out of scope of capi changes but we suggest auto-scaler implementations adhere to the following:

**Supported Version Skew Cases:**
- Allow auto-scaler operations to proceed normally regardless of ongoing upgrades
- Create new machines with the correct version for the target worker node group
- Maintain scale-up/scale-down functionality during cluster-level upgrades for workers with independent versions
- Maintain scale-up/scale-down functionality during independent worker upgrades for workers with cluster topology managed versions

**Unsupported Version Skew Cases:**
- Block auto-scaler operations that would create machines with unsupported version skew
- Provide appropriate error responses to prevent the auto-scaler from retrying indefinitely
- Resume auto-scaler operations once version constraints are satisfied

#### Scenario: Admission Control and Validation
The controller's admission webhooks should validate and block invalid version changes at admission time:

**Cluster-level Version Changes:**
- Block `cluster.spec.topology.version` changes that would create unsupported version skew with existing independent worker node group versions
  - while upgrade is in progress both old and new versions should be validated against
- Provide clear error messages explaining the version skew violation
- Allow valid cluster-level version changes that maintain supported skew

**Independent Worker Version Changes:**
- Block individual worker node group version changes that would exceed supported version skew with the control plane
- Block individual worker node group version changes that would exceed supported version skew with other worker node groups (if such cross-worker validation is implemented)
- Reject downgrade attempts for any worker node group
- Allow valid independent version changes within skew policy

**Validation Examples:**
- Control plane at v1.30.x should reject worker version changes to v1.33.x (exceeds 2 minor version skew)
- Control plane at v1.28.x should accept worker version changes to v1.28.x, v1.29.x, or v1.30.x
- Should reject any attempt to downgrade worker versions
- Should provide specific error messages indicating which version skew constraint was violated

#### Scenario: Feature Flag Behavior
When a cluster admin creates a cluster with the feature enabled, the controller should: 
- Ensure that worker node groups created with the version do not honor the cluster-level version 

When a cluster admin creates a cluster without the feature flag enabled, the controller should: 
- Ensure that any changes to the independent worker node group version is denied, stating that the feature flag must be enabled.
- Cluster-level version upgrades happen as they do today.

When a cluster admin creates a cluster with the feature flag enabled, and then disables the feature flag, the controller should: 
- Reject any updates to either the cluster-level or independent worker node group versions via webhook. 
  - Message should inform user that they must re-enable (enable) feature flag to modify these values in the cluster. 
- Edits to other fields should be allowable

#### Scenario: Chained Upgrades

Chained upgrade of cluster.spec.topology should only be allowed if they don't break version skew with any independently managed worker versions. 

#### Scenario: In Place Upgrades

In place upgrades should adhere to the same rules as rolling upgrades.


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

If applicable, how will the component handle version skew with other components? What are the guarantees? Make sure
this is in the test plan.

Consider the following in developing a version skew strategy for this enhancement:
- Does this enhancement involve coordinating behavior in the control plane and in the kubelet? How does an n-2 kubelet without this feature available behave when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI or CNI may require updating that component before the kubelet.

## Implementation History

- [ ] MM/DD/YYYY: Proposed idea in an issue or [community meeting]
- [ ] MM/DD/YYYY: Compile a Google Doc following the CAEP template (link here)
- [ ] MM/DD/YYYY: First round of feedback from community
- [ ] MM/DD/YYYY: Present proposal at a [community meeting]
- [ ] MM/DD/YYYY: Open proposal PR

<!-- Links -->
[community meeting]: https://docs.google.com/document/d/1ushaVqAKYnZ2VN_aa3GyKlS4kEd6bSug13xaXOakAQI/edit#heading=h.pxsq37pzkbdq
