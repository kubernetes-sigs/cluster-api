---
title: Code organization
authors:
  - "@fabriziopandini"
reviewers:
  - "@sbueringer"
  - "@sivchari"
  - "@Karthik-K-N "
creation-date: 2026-07-01
last-updated: 2026-07-01
status: implementable
---

# Code organization

## Table of Contents

<!-- TOC -->
* [Code organization](#code-organization)
  * [Table of Contents](#table-of-contents)
  * [Summary](#summary)
  * [Motivation](#motivation)
    * [Goals](#goals)
    * [Non-Goals](#non-goals)
  * [Proposal](#proposal)
    * [sigs.k8s.io/cluster-api/api](#sigsk8siocluster-apiapi)
    * [sigs.k8s.io/cluster-api/utils](#sigsk8siocluster-apiutils)
    * [sigs.k8s.io/cluster-api](#sigsk8siocluster-api)
    * [sigs.k8s.io/cluster-api/test](#sigsk8siocluster-apitest)
  * [Code organization](#code-organization-1)
    * [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
    * [Security Model](#security-model)
    * [Risks and Mitigations](#risks-and-mitigations)
      * [Improper assumptions](#improper-assumptions-)
      * [Excessive use of /internal](#excessive-use-of-internal)
  * [Implementation History](#implementation-history)
<!-- TOC -->

## Summary

This proposal documents the principles and guidelines for code organization in Cluster API.

This is relevant for maintainers, contributors and to everyone importing Cluster API as Go dependencies. 
Notably, this proposal also clarifies the guarantees that apply to different parts of the codebase. 

## Motivation

Ensuring a clean code organization is a continuous effort in projects like Cluster API.

However, in this particular moment there are a few relevant changes/issues impacting code organization,
so we decided to write this proposal to ensure a shared and clear understanding of the target end state. 

Issues related to code organization:
- [Importing API types pulls in lots of dependencies](https://github.com/kubernetes-sigs/cluster-api/issues/9011)
- [Make packages related to in-place updates public](https://github.com/kubernetes-sigs/cluster-api/pull/13507)
- [Expose Machine drain planning as a reusable library function](https://github.com/kubernetes-sigs/cluster-api/issues/13650)

On top of that, the Cluster API codebase is under pressure from a significant increase of the number
of CVEs in the Go SDK and in direct and indirect Go dependencies.

Last but not least, the end state should also tackle most of the code organization tech debt, e.g.:

- Complete the cleanup of the `exp` folder 
- Ensure a cleaner separation of the components in the Cluster API code base (core Cluster API, KCP, CABPK)

### Goals

- Provide a clean definition of which parts of Cluster API are intended for "general usage" via a Go dependency by other projects,
  e.g. usage of Cluster API v1beta2 Go API types 
  - Document the guarantees that each consumer can rely-on for the code that is intended for "general usage" as a Go dependency in other projects.
- Make it possible for consumer to have a "tighter integration" with Cluster API, like e.g. re-use some of the code in CAPI controllers.
  - Document the guarantees that each consumer can rely-on when they opt in to a "tighter integration" with Cluster API.
- Make the Cluster API code organization as simple, clean and maintainable as possible.

### Non-Goals

- Split the Cluster API repository in multiple repositories. 
  - We already experimented with this approach in the past, and the additional toil was not sustainable for 
    Cluster API maintainers (despite a lively community, contributors bandwidth is still an issue).

- Acknowledge or support [Hyrum's Law](https://www.hyrumslaw.com/).
  - Each consumer of a Go dependency must perform careful due diligence before adding dependencies to their projects, 
    and this implies that they have to take into account the guarantees that any dependencies including Cluster API offers.
  - See also [The right to be Unfinished](https://cluster-api.sigs.k8s.io/user/manifesto#the-right-to-be-unfinished) and [The complexity budget](https://cluster-api.sigs.k8s.io/user/manifesto#the-complexity-budget) in the Cluster API manifest.

## Proposal

Cluster API code organization takes inspiration from the code organization principles used in Kubernetes.

In Kubernetes, the code intended to be used as a Go library by other projects is clearly separated into separate sub-projects / Go modules,
e.g `k8s.io/api`, `k8s.io/utils`, etc.

Also in Cluster API the code intended to be used as a Go library by other projects will be clearly separated from the
rest of the codebase, but in this case we will implement this separation by introducing dedicated nested Go modules, 
more specifically `sigs.k8s.io/cluster-api/api`, `sigs.k8s.io/cluster-api/utils`.

The `sigs.k8s.io/cluster-api/api` and the `sigs.k8s.io/cluster-api/utils` Go modules are expected to cover most of the needs
of other projects consuming Cluster API as a Go dependency, and those Go modules will have the highest level of guarantees 
the Cluster API project can offer.

If we continue the comparison with Kubernetes, at this point it is worth to notice that the main Kubernetes repository,
which corresponds to the `k8s.io/kubernetes` Go module, is not intended to be used as a Go library by other projects (use at your own risk).

In Cluster API instead we are going to make an additional effort to provide a limited set of guarantees 
for both the "top level" `sigs.k8s.io/cluster-api` Go module and the `sigs.k8s.io/cluster-api/test` Go module.

While we understand that "limited set of guarantees" might seem not ideal, we kindly invite you to carefully
consider the other side of the coin.
- The Cluster API maintainers are willing to go above and beyond to help projects willing to have a tighter 
  integration with this project, and we are making that possible with a clear contract and well-defined trade-offs 
  documented in the following paragraphs.
- There is a clear path for consumers willing to achieve more guarantees on the code they are relying on, which is
  to engage with maintainers to discuss moving code from `sigs.k8s.io/cluster-api` / `sigs.k8s.io/cluster-api/test` to
  `sigs.k8s.io/cluster-api/utils`.
- Even if the `sigs.k8s.io/cluster-api` and the `sigs.k8s.io/cluster-api/test` Go modules offer limited guarantees,
  usually breaking changes happens for a good reason (so also consumers will benefit from those changes).

Below there is a summary of the Go modules offered by Cluster API and corresponding guarantees, please see following
paragraphs for more details.

| Go module                       | Kubernetes API guarantees   | Kubernetes semver<br/>guarantees 1/2<br/>breaking changes in minor release | Kubernetes semver<br/>guarantees 2/2<br/>breaking changes in patch release | Strict control<br/>of direct dependencies | min Go version<br/>bump in minor release | min Go version<br/>bump in patch release |
|---------------------------------|-----------------------------|----------------------------------------------------------------------------|----------------------------------------------------------------------------|-------------------------------------------|------------------------------------------|------------------------------------------|
| `sigs.k8s.io/cluster-api/api`   | Yes (only for API resouces) | Allowed                                                                    | Not allowed                                                                | Yes                                       | Allowed                                  | If required to fix critical CVE          |
| `sigs.k8s.io/cluster-api/utils` | No                          | Allowed                                                                    | Not allowed                                                                | Yes                                       | Allowed                                  | If required to fix critical CVE          |
| `sigs.k8s.io/cluster-api`       | No                          | Allowed                                                                    | If necessary                                                               | Best effort                               | Allowed                                  | If required to fix CVE                   |
| `sigs.k8s.io/cluster-api/test`  | No                          | Allowed                                                                    | If necessary                                                               | Best effort                               | Allowed                                  | If required to fix CVE                   |

### sigs.k8s.io/cluster-api/api

The `sigs.k8s.io/cluster-api/api` Go module is defined in the `/api `folder, and it will contain Go types
for each API resource defined in Cluster API. For sake of simplicity, also KCP and CABPK are included.

This is the Go module that will be most widely used by Cluster API consumers, and as a consequence it is where we 
will offer the wider set of guaranteed to consumers. More specifically, most of the types defined in this module 
have to comply both to Kubernetes API guarantees and Kubernetes semver guarantees.

Kubernetes API guarantees are documented in the [Kubernetes Deprecation Policy](https://kubernetes.io/docs/reference/using-api/deprecation-policy/),
and the Cluster API project abides to the same rules for its public API resources.

According to Kubernetes semver guarantees, breaking changes are not allowed in release branches, as these represent 
minor versions that have already been released. These versions have consumers who expect the API resources, corresponding Go types,
behaviors, etc. to remain stable during the lifetime of the patch stream for the minor release.

Please note that while we will try to avoid this as much as possible, when introducing a new minor release it is 
technically possible to change Go types in this package if the resulting API resources (the CRD definition) will remain the same.

Last but not least:
- We are going to invest additional effort in trying to keep the list of direct dependencies of the 
  `sigs.k8s.io/cluster-api/api` Go module as small as possible.
- We are committed to not bump the min Go version required to build this package, unless it is required 
  to fix critical CVEs in this module or in one of its dependencies.

### sigs.k8s.io/cluster-api/utils

The `sigs.k8s.io/cluster-api/utils` Go module will be defined in the `/utils` folder (new).

The plan is to selectively move code that is designed to help in the implementation of Cluster API providers 
or to help in building systems on top of Cluster API to this new Go module.

You can consider this Go module a better version of the current `/util` folder, where we will also move other
packages intended for usage in providers, like e.g. the `clustercache` that is currently located under `controllers`.

The code in this Go module will comply to Kubernetes semver guarantees, and thus breaking changes are not allowed in 
release branches, but are possible when introducing a new minor release. 

Last but not least:
- We are going to invest additional effort in trying to keep the list of direct dependencies of the
  `sigs.k8s.io/cluster-api/utils` Go module as small as possible.
- We are committed to not bump the min Go version required to build this package, unless it is required
  to fix critical CVEs in this module or in one of its dependencies.

### sigs.k8s.io/cluster-api

The `sigs.k8s.io/cluster-api` Go module contains the code for the "core" Cluster API components, as well as 
the code for in-tree providers like KCP and CABPK.

The code in this Go module is mostly considered internal to Cluster API - not for general consumption -, even
if in some cases the code is technically implemented as public types or functions (e.g. to allow sharing between 
Cluster API components, or re-use in CI, etc.)

Nevertheless, users can import the `sigs.k8s.io/cluster-api` Go module, being aware that Kubernetes semver guarantees 
for this Go module (e.g. avoid breaking changes on release branches) are provided only on best effort bases.

Please note that this doesn't mean we do expect frequent breaking changes in patch versions, but we are reserving
the opportunity to introduce breaking changes in patch releases when necessary, e.g. to fix issues, CVEs or for 
backporting some changes.

Other projects importing this Go modules should also be aware that invasive changes might happen in minor releases, 
e.g. when introducing a new feature, or performing a refactor/paying down technical debt.

Consumers willing to achieve more guarantees on the code they are relying on, should engage with maintainers 
to discuss moving code from `sigs.k8s.io/cluster-api` to `sigs.k8s.io/cluster-api/utils`.

While doing so, you should always keep in mind that `sigs.k8s.io/cluster-api/utils` is intended to host common components or utilities 
shared by core Cluster API, Cluster API providers and systems built on top of Cluster API. It is not, and it should not
become a general purpose Go library (providing a general purpose Go library is not in the scope of the Cluster API project).

Accordingly, moving code to `sigs.k8s.io/cluster-api/utils` will be subject to a careful evaluation from maintainers;
the decision will also consider factors like:
- Community interest / number of possible consumers / number of candidate maintainers 
- The readiness of the code to become part of a library intended for broader usage across the CAPI ecosystem
- The notion of [the complexity budget](https://cluster-api.sigs.k8s.io/user/manifesto#the-complexity-budget)
- etc.

Last but not least, please note that for the `sigs.k8s.io/cluster-api` Go module it is allowed to bump the min Go version 
required to build this package also on release branches, e.g. for fixing CVEs in this module or in one of its dependencies.

### sigs.k8s.io/cluster-api/test

The `sigs.k8s.io/cluster-api/test` Go module is defined in the `/test` folder.

This Go module has a few functions:
- The `sigs.k8s.io/cluster-api/test/framework` package offers test frameworks to be used for implementing E2E tests in 
  Cluster API and in Cluster API providers.
- The `sigs.k8s.io/cluster-api/test/e2e` package offers a set of e2e tests that in most cases can be easily re-used also 
  in Cluster API providers.
- The `sigs.k8s.io/cluster-api/test/infrastructure` and the `sigs.k8s.io/cluster-api/test/extension` packages implement 
  components used for Cluster API development and test.
- It prevents the leaking of test dependency into the top level `sigs.k8s.io/cluster-api` Go module (e.g. docker/moby).

Considering the specific nature of this Go module (testing CAPI), the limited number of users (a subset of CAPI providers), the limited 
set of maintainers working on it, Kubernetes semver guarantees for this Go module are provided only on best effort bases.

Please note that this doesn't mean we do expect frequent breaking changes in patch versions, but we are reserving
the opportunity to introduce breaking changes in patch releases when necessary, e.g. to fix CI issue, or when it is required/advisable
to minimize the effort required to run tests across different minors.

For the `sigs.k8s.io/cluster-api/test` Go module it is also allowed bump to the min Go version required to build
this package also on release branches, e.g. for fixing CVEs in this module or in one of its dependencies.

## Code organization

In order to simplify code organization in Cluster API and to introduce cleaner separation of the components hosted in the 
Cluster API code base (core Cluster API, KCP, CABPK) we are going to complete the cleanup of the `/exp` folder and establish
the following structure:

* `/api`: public consumption, guarantees according to Kubernetes API guarantees (`sigs.k8s.io/cluster-api/api` Go module)
* `/bootstrap/kubeadm`: CABPK provider, internal code/not for general consumption, limited guarantees
* `/CHANGELOG`: documentation
* `/cmd/clusterctl`: clusterctl command, internal code/not for general consumption, limited guarantees
* `/controlplane/kubeadm`: KCP internal, internal code/not for general consumption, limited guarantees
* `/core`: "core" Cluster API provider, internal code/not for general consumption, limited guarantees
* `/docs`: documentation
* `/hack`: scripting, CI, no guarantees (Go module: hack/tools)
* `/pkg`: internal code shared among the above components/not for general consumption, limited guarantees
* `/test`: public e2e test code & test providers, limited guarantee (`sigs.k8s.io/cluster-api/test` Go module)
* `/utils`: public consumption, strong guarantees (`sigs.k8s.io/cluster-api/utils` Go module)

Please note we are also going to ensure a consistent structure for all production providers (core Cluster API, KCP, CABPK),
having the same set of folders in `/core`, `/bootstrap/kubeadm`, `/controlplane/kubeadm`:
* `/config`
* `/pkg` 
* `/reconcilers/<reconciler type>`
* `/setup`
* `/webhooks/{admission,conversion}`
* `/main.go`

Note: In general provider-specific code should be located within the provider package.

### Implementation Details/Notes/Constraints

Cluster API already had a `sigs.k8s.io/cluster-api/test` Go module since a long time and the
`sigs.k8s.io/cluster-api/api` Go module has been recently introduced. 

Other changes described in this proposal are already ongoing for a few releases and have been
discussed in the office hours, e.g. cleaning up the `/exp` folder.

The remaining changes, like the introduction of the `sigs.k8s.io/cluster-api/utils` Go module 
and code organization changes will be implemented incrementally.

### Security Model

This proposal doesn't change the security posture of the project, however, it is worth to notice that
by introducing separate Go modules, the project is improving the options available to fix CVE on release branches.

Most specifically, by bumping the minimum Go version required to build Cluster API on Go modules selectively
the project can limit the impact of this change. 

### Risks and Mitigations

#### Improper assumptions 

The is no way we can prevent other projects to make improper assumptions on the guarantees offered by
code in the Cluster API project.

As a mitigation to the risk of other projects making improper assumptions on CAPI code, this proposal 
introduces dedicated Go modules with higher guarantees that are specifically designed for consumption as well 
as it is documenting as clearly as possible guarantees offered by each Go module.

At the same time, we are also doing our best to increase awareness and clear accountability for a thoughtful due diligence that 
each project must perform before consuming any dependency, not just Cluster API.

#### Excessive use of /internal

In the past, we tried to reduce risks of other projects making improper assumptions on CAPI code by documenting
guarantees at package level and by moving most of the component specific code under `/internal` packages.

However, over time we realized that in some cases this is not technically feasible, e.g. if we want to share code between core
Cluster API and the test module.

In other cases we also noticed that using `/internal` leads to a not ideal code organization, because correlated packages might end up
far away in the folder struct.

Using `/internal` also prevents usage of Cluster API of projects willing to accept the trade-off that a tighter integration
with Cluster API implies and this is something we are not happy about.

As a consequence, we plan to reduce the usage of `/internal` packages, limiting it to cases where it is possible and advisable.

## Implementation History

- [ ] 2026-07-01: Present proposal at a [community meeting]

<!-- Links -->
[community meeting]: https://docs.google.com/document/d/1ushaVqAKYnZ2VN_aa3GyKlS4kEd6bSug13xaXOakAQI/edit#heading=h.pxsq37pzkbdq
