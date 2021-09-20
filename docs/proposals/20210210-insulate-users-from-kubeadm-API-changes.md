---
title: Insulate users from kubeadm API version changes
authors:
- "@fabriziopandini"
reviewers:
- "@vincepri"
creation-date: 2021-02-10
last-updated: 2021-02-10
status: implementable
see-also:
- "/docs/proposals/20190610-machine-states-preboot-bootstrapping.md"
replaces:
superseded-by:
---

# Insulate users from kubeadm API version changes

## Table of Contents

* [Insulate users from kubeadm API version changes](#insulate-users-from-kubeadm-api-version-changes)
  * [Table of Contents](#table-of-contents)
  * [Glossary](#glossary)
  * [Summary](#summary)
  * [Motivation](#motivation)
    * [Goals](#goals)
    * [Non-Goals](#non-goals)
    * [Future work](#future-work)
  * [Proposal](#proposal)
    * [User Stories](#user-stories)
      * [Story 1](#story-1)
      * [Story 2](#story-2)
    * [Requirements](#requirements)
    * [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
      * [Background info about kubeadm API version](#background-info-about-kubeadm-api-version)
      * [Background info about kubeadm types into the KubeadmConfig/KubeadmControlPlane specs](#background-info-about-kubeadm-types-into-the-kubeadmconfigkubeadmcontrolplane-specs)
      * [Cluster API v1alpha3 changes](#cluster-api-v1alpha3-changes)
      * [Cluster API v1alpha4 changes](#cluster-api-v1alpha4-changes)
    * [Security Model](#security-model)
    * [Risks and Mitigations](#risks-and-mitigations)
  * [Alternatives](#alternatives)
  * [Upgrade Strategy](#upgrade-strategy)
  * [Additional Details](#additional-details)
    * [Test Plan](#test-plan)
  * [Implementation History](#implementation-history)

## Glossary

Refer to the [Cluster API Book Glossary](https://cluster-api.sigs.k8s.io/reference/glossary.html).

## Summary

Make CABPK and KCP to use more recent versions of the kubeadm API and insulate users from
kubeadm API version changes.

## Motivation

Cluster bootstrap provider for kubeadm (CABPK) and the control plane provider for kubeadm
(KCP) API still relying on kubeadm v1beta1 API, which has been deprecated, and it is going
to be removed ASAP.

While moving to a more recent version of the kubeadm API is a required move, Cluster API
should take this opportunity to stop relying on the assumption that the kubeadm API types in the
KubeadmConfig/KubeadmControlPlane specs are supported(1) by all the Kubernetes/kubeadm version
in the support range.

This would allow to separate what users fill in the KubeadmConfig/KubeadmControlPlane
from which kubeadm API version Cluster API end up using in the bootstrap data.

(1) Supported in this context means that the serialization format of the types is the same,
because types are already different (see background info in the implementation details paragraph).

### Goals

- Define a stop-gap for using the most recent version of the kubeadm API in Cluster API
  v1alpha3 - introducing any breaking changes.
- Define how to stop to exposing the kubeadm v1betax types in the KubeadmConfig/KubeadmControlPlane
  specs for v1alpha4.
- Define how to use the right version of the kubeadm types generating our kubeadm yaml
  file and when interacting with the kubeadm-config ConfigMap.
- Ensure a clean and smooth v1alpha3 to v1aplha4 upgrade experience.

### Non-Goals

- Adding or removing fields in KubeadmConfig/KubeadmControlPlane spec.
- Introduce behavioral changes in CABPK or KCP.

### Future work

- Evaluate improvements for the Cluster API owned version of KubeadmConfig/KubeadmControlPlane
  specs types.
- Make it possible for the Cluster API users to take benefit of changes introduced
  in recent versions of the Kubeadm API.

## Proposal

### User Stories

#### Story 1

As a user, I want to user kubeadm providers for Cluster API (CABPK & KPC) in the same
way across the entire spectrum of supported kubernetes versions.

#### Story 2

As the kubeadm tool, I want the kubeadm providers for Cluster API (CABPK & KPC) to
use the latest kubeadm API supported by the target kubeadm/kubernetes version.

### Requirements

R1 - avoid breaking changes in v1alpha3
R2 - ensure a clean v1alpha3 to v1alpha4 upgrade

### Implementation Details/Notes/Constraints

#### Background info about kubeadm API version

kubeadm v1beta1 types:

- Introduced in v1.13.
- Deprecation cycle started with v1.17.
- Removal scheduled for v1.20 but then postponed to v1.21.

kubeadm v1beta2 types:

- Introduced in v1.15.
- Changes from the previous version are minimal:
  - Support for IgnorePreflightErrors into the NodeRegistrationOptions.
  - Support for CertificateKey into InitConfiguration and JoinConfiguration
    configuration (not relevant for Cluster API because it is not using the
    automatic certificate copy feature).
  - improved serialization support (in practice a set of `omitempty` fixes).

#### Background info about kubeadm types into the KubeadmConfig/KubeadmControlPlane specs

Given the fact that importing kubeadm (which is part of the Kubernetes codebase)
in Cluster API is impractical, Cluster API hosts a mirror of kubeadm API types.

kubeadm v1beta1 mirror-types:

- Hosted in `bootstrap/kubeadm/types/upstreamv1beta1`.
- Diverged from the original v1beta1 types for better CRD support (in practice
  a set of `+optional` fixes, few `omitempty` differences).

kubeadm v1beta2 mirror-types:

- Hosted in `bootstrap/kubeadm/types/upstreamv1beta2`.
- Currently, not used in the Cluster API codebase.
- Does not include changes for better CRD support introduced in kubeadm v1beta1
  mirror-types.

#### Cluster API v1alpha3 changes

Changes to cluster API v1alpha3 release should be minimal and no breaking change
should be introduced while implementing this proposal.

According to this principle and to the feedback to this proposal,
we are going to implement alternative 2 described below.

__Alternative 1:__

Keep kubeadm v1beta1 types as a Hub type (1); implement conversion to kubeadm API
version f(Kubernetes Version) when generating the kubeadm config for init/join (
e.g convert to kubeadm API v1beta2 for Kubernetes version >= v1.15, convert to
kubeadm API v1beta1 for Kubernetes version < v1.15).

This alternative is the more clean, robust and forward looking, but it requires
much more work than alternative 2.

__Alternative 2:__

Keep kubeadm v1beta1 types as a Hub type (1); only change the apiVersion to
`kubeadm.k8s.io/v1beta2` in the generated kubeadm config for init/join.

This alternative is based on following considerations:

- kubeadm v1beta1 mirror types are "compatible" with kubeadm v1beta2 types
  - Having support for IgnorePreflightErrors into the NodeRegistrationOptions
    is not required for Cluster API v1alpha3
  - The automatic certificate copy feature is not used by Cluster API
  - Improved serialization support has been already applied to the kubeadm
    v1beta1 mirror-types (and further extended).
- the minimal Kubernetes version supported by Cluster API is v1.16, and kubeadm
  v1.16 could work with v1beta2 types. Same for all the other versions up to latest(1.20)
  and next (1.21).
- limitations: this approach is not future proof, and it should be reconsidered
  whenever a new version of kubeadm types is created while v1alpha3 is still supported.

__Common considerations for both alternatives__

KCP is modifying the kubeadm-config Config Map generated by kubeadm, and ideally also
this bit of code should be made kubeadm version aware.

However, given that the current implementation currently uses unstructured, and
a change for using the kubeadm types, requires a big refactor, the proposed approach
for v1alpha3 is to limit the changes to only upgrade the apiVersion when required.

- limitations: this approach is not future proof, and it should be reconsidered
  whenever a new version of kubeadm types is changing one of the fields
  edited during upgrades.

(1) See https://book.kubebuilder.io/multiversion-tutorial/conversion-concepts.html
for a definition of Hub or spoke types/version.

#### Cluster API v1alpha4 changes

Changes to cluster API v1alpha4 could be more invasive and seek for a
forward-looking solution.

Planned actions are:

- introduce a Cluster API owned version of the kubeadm config types
  (starting from kubeadm v1beta1) to be used by KubeadmConfig/KubeadmControlPlane
  specs; this should also act as a serialization/deserialization hub (1).
  Please note that those types will be part of Cluster API types, and thus initially
  versioned as v1alpha4; once conversion will be in place, those types are not required
  anymore to have the same serialization format of the real kubeadm types.
- preserve `bootstrap/kubeadm/types/upstreamv1beta1` as a serialization/deserialization
  spoke (1)(2) for v1alpha4 (also, this will be used by v1alpha3 API types until removal)
- preserve `bootstrap/kubeadm/types/upstreamv1beta2` as serialization/deserialization
  spoke (1)(2) for v1alpha4
- implement hub/spoke conversions (1)
- make CABPK to use conversion while generating the kubeadm config file for init/join
- make KCP to use conversion while serializing/deserializing the kubeadm-config Config Map
- make KCP to use the Cluster API owned version of the kubeadm config types instead
  of using `Unstructured` for the kubeadm-config Config Map handling
- add the `IgnorePreflightError` field to the Cluster API owned types; this field will be silently
  ignored when converting to v1beta1 (because this version does not support this field).
  Note: we are not planning to add `CertificateKey` to the Cluster API owned types because
  this field is not relevant for Cluster API.

(1) See https://book.kubebuilder.io/multiversion-tutorial/conversion-concepts.html
for a definition of Hub or spoke types/version.
(2) As soon as it will be possible to vendor kubeadm types, we should drop this copy
and use kubeadm library as a source of truth. Hover there is no concrete plan for this yet.

### Security Model

This proposal does not introduce changes the existing security model.

### Risks and Mitigations

- Time crunch

This change has been postponed several times for different reasons,
and now it is being worked with a strict deadline before kubeadm type removal.
Mitigation: Changes to the KubeadmConfig/KubeadmControlPlane specs types considered
as a future work.

## Alternatives

The `Alternatives` section is used to highlight and record other possible approaches
to delivering the value proposed by a proposal.

## Upgrade Strategy

Given the requirement to provide a clean upgrade path from v1alpha3 to v1alpha4,
upgrades should be handles using conversion web-hooks only.
No external upgrade tool/additional manual steps should be required for upgrade.

## Additional Details

### Test Plan

For v1alpha4 this will be tested by the periodic jobs testing
creating cluster with different Kubernetes releases, doing upgrades to the next version
and running Kubernetes conformance.

For v1alpha3 there are no such jobs. We should explore if to backport all the
required changes to the test framework (complex) or if to create ad-hoc test for this
using what is supported by the v1alpha3 version of the test framework (possible limitations).

## Implementation History

- 2021-02-10: First draft and round of feedback from community
