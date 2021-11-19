# Contributing Guidelines
<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Contributing Guidelines](#contributing-guidelines)
    - [Contributor License Agreements](#contributor-license-agreements)
    - [Finding Things That Need Help](#finding-things-that-need-help)
    - [Contributing a Patch](#contributing-a-patch)
    - [Documentation changes](#documentation-changes)
    - [Releases](#releases)
    - [Proposal process (CAEP)](#proposal-process-caep)
    - [Triaging E2E test failures](#triaging-e2e-test-failures)
    - [Reviewing a Patch](#reviewing-a-patch)
    - [Reviews](#reviews)
      - [Approvals](#approvals)
    - [Backporting a Patch](#backporting-a-patch)
    - [Features and bugs](#features-and-bugs)
    - [Experiments](#experiments)
    - [Breaking Changes](#breaking-changes)
    - [API conventions](#api-conventions)
      - [Optional vs. Required](#optional-vs-required)
        - [Example](#example)
        - [Exceptions](#exceptions)
      - [CRD additionalPrinterColumns](#crd-additionalprintercolumns)
    - [Google Doc Viewing Permissions](#google-doc-viewing-permissions)
    - [Issue and Pull Request Management](#issue-and-pull-request-management)
    - [Contributors ladder](#contributors-ladder)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

Read the following guide if you're interested in contributing to cluster-api.

## Contributor License Agreements

We'd love to accept your patches! Before we can take them, we have to jump a couple of legal hurdles.

Please fill out either the individual or corporate Contributor License Agreement (CLA). More information about the CLA
and instructions for signing it [can be found here](https://git.k8s.io/community/CLA.md).

***NOTE***: Only original source code from you and other people that have signed the CLA can be accepted into the
*repository.

## Finding Things That Need Help

If you're new to the project and want to help, but don't know where to start, we have a semi-curated list of issues that
should not need deep knowledge of the system. [Have a look and see if anything sounds
interesting](https://github.com/kubernetes-sigs/cluster-api/issues?q=is%3Aopen+is%3Aissue+label%3A%22good+first+issue%22).
Before starting to work on the issue, make sure that it doesn't have a [lifecycle/active](https://github.com/kubernetes-sigs/cluster-api/labels/lifecycle%2Factive) label. If the issue has been assigned, reach out to the assignee.
Alternatively, read some of the docs on other controllers and try to write your own, file and fix any/all issues that
come up, including gaps in documentation!

If you're a more experienced contributor, looking at unassigned issues in the next release milestone is a good way to find work that has been prioritized. For example, if the latest minor release is `v1.0`, the next release milestone is `v1.1`.

Help and contributions are very welcome in the form of code contributions but also in helping to moderate office hours, triaging issues, fixing/investigating flaky tests, cutting releases, helping new contributors with their questions, reviewing proposals, etc.

## Versioning

### Codebase and Go Modules

> :warning: The project does not follow Go Modules guidelines for compatibility requirements for 1.x semver releases.

Cluster API follows upstream Kubernetes semantic versioning. With the v1 release of our codebase, we guarantee the following:

- A (*minor*) release CAN include:
  - Introduction of new API versions, or new Kinds.
  - Compatible API changes like field additions, deprecation notices, etc.
  - Breaking API changes for deprecated APIs, fields, or code.
  - Features, promotion or removal of feature gates.
  - And more!

- A (*patch*) release SHOULD only include backwards compatible set of bugfixes.

These guarantees extend to all code exposed in our Go Module, including
*types from dependencies in public APIs*.
Types and functions not in public APIs are not considered part of the guarantee.
The test module, clusterctl, and experiments do not provide any backward compatible guarantees.

#### Backporting a patch

We only accept backports of critical bugs, security issues, or bugs without easy workarounds, any
backport MUST not be breaking for either API or behavioral changes.
We generally do not accept PRs against older release branches.

### APIs

API versioning and guarantees are inspired by the [Kubernetes deprecation policy](https://kubernetes.io/docs/reference/using-api/deprecation-policy/)
and [API change guidelines](https://github.com/kubernetes/community/blob/f0eec4d19d407c13681431b3c436be67da8c448d/contributors/devel/sig-architecture/api_changes.md).
We follow the API guidelines as much as possible adapting them if necessary and on a case-by-case basis to CustomResourceDefinition.

### CLIs

Any command line interface in Cluster API (e.g. clusterctl) share the same versioning schema of the codebase.
CLI guarantees are inspired by [Kubernetes deprecation policy for CLI](https://kubernetes.io/docs/reference/using-api/deprecation-policy/#deprecating-a-flag-or-cli),
however we allow breaking changes after 8 months or 2 releases (whichever is longer) from deprecation.

## Branches

Cluster API has two types of branches: the *main* branch and
*release-X* branches.

The *main* branch is where development happens. All the latest and
greatest code, including breaking changes, happens on main.

The *release-X* branches contain stable, backwards compatible code. On every
major or minor release, a new branch is created. It is from these
branches that minor and patch releases are tagged. In some cases, it may
be necessary to open PRs for bugfixes directly against stable branches, but
this should generally not be the case.

### Support and guarantees

Cluster API maintains the most recent release branch for all supported API and contract versions. Support for this section refers to the ability to backport and release patch versions.

| API Version   | Branch | Supported Until |
| ------------- | ----------- | ---------- |
| **v1beta1**   | release-1.0 | current stable |
| **v1alpha4**  | release-0.4 | 2022-04-06 |
| **v1alpha3**  | release-0.3 | 2022-02-23 |

- The API version is determined from the GroupVersion defined in the top-level `api/` package.
- The EOL date is determined from the last release available once a new API version is published.
- For each given API version only the most recent associated release branch is supported, older branches are immediately unsupported. Exceptions can be filed with maintainers and taken into consideration on a case-by-case basis.

## Contributing a Patch

1. If you haven't already done so, sign a Contributor License Agreement (see details above).
1. If working on an issue, signal other contributors that you are actively working on it using `/lifecycle active`.
1. Fork the desired repo, develop and test your code changes.
1. Submit a pull request.
    1. All code PR must be labeled with one of
        - ⚠️ (`:warning:`, major or breaking changes)
        - ✨ (`:sparkles:`, feature additions)
        - 🐛 (`:bug:`, patch and bugfixes)
        - 📖 (`:book:`, documentation or proposals)
        - 🌱 (`:seedling:`, minor or other)

Individual commits should not be tagged separately, but will generally be
assumed to match the PR. For instance, if you have a bugfix in with
a breaking change, it's generally encouraged to submit the bugfix
separately, but if you must put them in one PR, mark the commit
separately.

All changes must be code reviewed. Coding conventions and standards are explained in the official [developer
docs](https://git.k8s.io/community/contributors/devel). Expect reviewers to request that you
avoid common [go style mistakes](https://github.com/golang/go/wiki/CodeReviewComments) in your PRs.

## Documentation changes

The documentation is published in form of a book at:

- [Current stable release](https://cluster-api.sigs.k8s.io)
- [Tip of the main branch](https://main.cluster-api.sigs.k8s.io/)
- [v1alpha4 release branch](https://release-0-4.cluster-api.sigs.k8s.io/)
- [v1alpha3 release branch](https://release-0-3.cluster-api.sigs.k8s.io/)
- [v1alpha2 release branch](https://release-0-2.cluster-api.sigs.k8s.io/)

The source for the book is [this folder](https://github.com/kubernetes-sigs/cluster-api/tree/main/docs/book/src)
containing markdown files and we use [mdBook][] to build it into a static
website.

After making changes locally you can run `make serve-book` which will build the HTML version
and start a web server so you can preview if the changes render correctly at
http://localhost:3000; the preview auto-updates when changes are detected.

Note: you don't need to have [mdBook][] installed, `make serve-book` will ensure
appropriate binaries for mdBook and any used plugins are downloaded into
`hack/tools/bin/` directory.

When submitting the PR remember to label it with the 📖 (:book:) icon.

[mdBook]: https://github.com/rust-lang/mdBook

## Releases

Cluster API uses [GitHub milestones](https://github.com/kubernetes-sigs/cluster-api/milestones) to track releases. Issues in a release milestone have been prioritized and accepted for the release. However, these issues are not committed to the release, unless they are marked as `kind/release-blocking`. Getting them into the release is dependent on someone in the community getting assigned to the issue and completing the work.

- Minor versions CAN be planned and scheduled for each quarter, or sooner if necessary.
  - Each minor version is preceded with one or more planning session.
  - Planning consists of one or more backlog grooming meetings, roadmap amendments,
    and CAEP proposal reviews.
- Patch versions CAN be planned and scheduled each month for each of the currently supported series (usually N and N-1).
- Code freeze is in effect 72 hours (3 days) before a release.
  - Maintainers should communicate the code freeze date at a community meeting preceding the code freeze date.
  - Only critical bug fixes may be merged in between freeze & release.
    - Each bug MUST be associated with an open issue and properly triaged.
    - PRs MUST be approved by at least 2 project maintainers.
      - First approver should `/approve` and `/hold`.
      - Second approver should `/approve` and `/hold cancel`.
  - [E2E Test grid](https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi%20e2e%20tests) SHOULD be green before cutting a release.
- Dates in a release are approximations and always subject to change.
- `Next` milestone is for work that has been triaged, but not prioritized/accepted for any release.

## Proposal process (CAEP)

The Cluster API Enhacement Proposal is the process this project uses to adopt new features, changes to the APIs, changes to contracts between components, or changes to CLI interfaces.

The [template](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/YYYYMMDD-template.md), and accepted proposals live under [docs/proposals](https://github.com/kubernetes-sigs/cluster-api/tree/main/docs/proposals).

- Proposals or requests for enhacements (RFEs) MUST be associated with an issue.
  - Issues can be placed on the roadmap during planning if there is one or more folks
    that can dedicate time to writing a CAEP and/or implementating it after approval.
- A proposal SHOULD be introduced and discussed during the weekly community meetings,
  [Kubernetes SIG Cluster Lifecycle mailing list](https://groups.google.com/forum/#!forum/kubernetes-sig-cluster-lifecycle),
  or [discuss forum](https://discuss.kubernetes.io/c/contributors/cluster-api/).
  - Submit and discuss proposals using a collaborative writing platform, preferably Google Docs, share documents with edit permissions with the [Kubernetes SIG Cluster Lifecycle mailing list](https://groups.google.com/forum/#!forum/kubernetes-sig-cluster-lifecycle).
- A proposal in a Google Doc MUST turn into a [Pull Request](https://github.com/kubernetes-sigs/cluster-api/pulls).
- Proposals MUST be merged and in `implementable` state to be considered part of a major or minor release.

## Triaging E2E test failures

When you submit a change to the Cluster API repository as set of validation jobs is automatically executed by
prow and the results report is added to a comment at the end of your PR.

Some jobs run linters or unit test, and in case of failures, you can repeat the same operation locally using `make test lint [etc..]`
in order to investigate and potential issues. Prow logs usually provide hints about the make target you should use
(there might be more than one command that needs to be run).

End-to-end (E2E) jobs create real Kubernetes clusters by building Cluster API artifacts with the latest changes.
In case of E2E test failures, usually it's required to access the "Artifacts" link on the top of the prow logs page to triage the problem.

The artifact folder contains:
- A folder with the clusterctl local repository used for the test, where you can find components yaml and cluster templates.
- A folder with logs for all the clusters created during the test. Following logs/info are available:
    - Controller logs (only if the cluster is a management cluster).
    - Dump of the Cluster API resources (only if the cluster is a management cluster).
    - Machine logs (only if the cluster is a workload cluster)

In case you want to run E2E test locally, please refer to the [Testing](https://cluster-api.sigs.k8s.io/developer/testing.html#running-unit-and-integration-tests) guide. An overview over our e2e test jobs (and also all our other jobs) can be found in [Jobs](https://cluster-api.sigs.k8s.io/reference/jobs.html).

## Reviewing a Patch

## Reviews

> Parts of the following content have been adapted from https://google.github.io/eng-practices/review.

Any Kubernetes organization member can leave reviews and `/lgtm` a pull request.

Code reviews should generally look at:

- **Design**: Is the code well-designed and consistent with the rest of the system?
- **Functionality**: Does the code behave as the author (or linked issue) intended? Is the way the code behaves good for its users?
- **Complexity**: Could the code be made simpler?  Would another developer be able to easily understand and use this code when they come across it in the future?
- **Tests**: Does the code have correct and well-designed tests?
- **Naming**: Did the developer choose clear names for variable, types, methods, functions, etc.?
- **Comments**: Are the comments clear and useful? Do they explain the why rather than what?
- **Documentation**: Did the developer also update relevant documentation?

See [Code Review in Cluster API](REVIEWING.md) for a more focused list of review items.

### Approvals

Please see the [Kubernetes community document on pull
requests](https://git.k8s.io/community/contributors/guide/pull-requests.md) for more information about the merge
process.

- A PR is approved by one of the project maintainers and owners after reviews.
- Approvals should be the very last action a maintainer takes on a pull request.

## Features and bugs

Open [issues](https://github.com/kubernetes-sigs/cluster-api/issues/new/choose) to report bugs, or minor features.

For big feature, API and contract amendments, we follow the CAEP process as outlined below.

## Experiments

Proof of concepts, code experiments, or other initiatives can live under the `exp` folder or behind a feature gate.

- Experiments SHOULD not modify any of the publicly exposed APIs (e.g. CRDs).
- Experiments SHOULD not modify any existing CRD types outside of the experimental API group(s).
- Experiments SHOULD not modify any existing command line contracts.
- Experiments MUST not cause any breaking changes to existing (non-experimental) Go APIs.
- Experiments SHOULD introduce utility helpers in the go APIs for experiments that cross multiple components
  and require support from bootstrap, control plane, or infrastructure providers.
- Experiments follow a strict lifecycle: Alpha -> Beta prior to Graduation.
  - Alpha-stage experiments:
    - SHOULD not be enabled by default and any feature gates MUST be marked as 'Alpha'
    - MUST be associated with a CAEP that is merged and in at least a provisional state
    - MAY be considered inactive and marked as deprecated if the following does not happen within the course of 1 minor release cycle:
      - Transition to Beta-stage
      - Active development towards progressing to Beta-stage
      - Either direct or downstream user evaluation
    - Any deprecated Alpha-stage experiment MAY be removed in the next minor release.
  - Beta-stage experiments:
    - SHOULD be enabled by default, and any feature gates MUST be marked as 'Beta'
    - MUST be associated with a CAEP that is at least in the experimental state
    - MUST support conversions for any type changes
    - MUST remain backwards compatible unless updates are coinciding with a breaking Cluster API release
    - MAY be considered inactive and marked as deprecated if the following does not happen within the course of 1 minor release cycle:
      - Graduate
      - Active development towards Graduation
      - Either direct or downstream user consumption
    - Any deprecated Beta-stage experiment MAY be removed after being deprecated for an entire minor release.
- Experiment Graduation MUST coincide with a breaking Cluster API release
- Experiment Graduation checklist:
  - [ ] MAY provide a way to be disabled, any feature gates MUST be marked as 'GA'
  - [ ] MUST undergo a full Kubernetes-style API review and update the CAEP with the plan to address any issues raised
  - [ ] CAEP MUST be in an implementable state and is fully up to date with the current implementation
  - [ ] CAEP MUST define transition plan for moving out of the experimental api group and code directories
  - [ ] CAEP MUST define any upgrade steps required for Existing Management and Workload Clusters
  - [ ] CAEP MUST define any upgrade steps required to be implemented by out-of-tree bootstrap, control plane, and infrastructure providers.

## Breaking Changes

Breaking changes are generally allowed in the `main` branch, as this is the branch used to develop the next minor
release of Cluster API.

There may be times, however, when `main` is closed for breaking changes. This is likely to happen as we near the
release of a new minor version.

Breaking changes are not allowed in release branches, as these represent minor versions that have already been released.
These versions have consumers who expect the APIs, behaviors, etc. to remain stable during the life time of the patch
stream for the minor release.

Examples of breaking changes include:

- Removing or renaming a field in a CRD
- Removing or renaming a CRD
- Removing or renaming an exported constant, variable, type, or function
- Updating the version of critical libraries such as controller-runtime, client-go, apimachinery, etc.
    - Some version updates may be acceptable, for picking up bug fixes, but maintainers must exercise caution when
      reviewing.

There may, at times, need to be exceptions where breaking changes are allowed in release branches. These are at the
discretion of the project's maintainers, and must be carefully considered before merging. An example of an allowed
breaking change might be a fix for a behavioral bug that was released in an initial minor version (such as `v0.3.0`).


## API conventions

This project follows the [Kubernetes API conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md). Minor modifications or additions to the conventions are listed below.

### Optional vs. Required

* Status fields MUST be optional. Our controllers are patching selected fields instead of updating the entire status in every reconciliation.

* If a field is required (for our controllers to work) and has a default value specified via OpenAPI schema, but we don't want to force users to set the field, we have to mark the field as optional. Otherwise, the client-side kubectl OpenAPI schema validation will force the user to set it even though it would be defaulted on the server-side.

Optional fields have the following properties:
* An optional field MUST be marked with `+optional` and include an `omitempty` JSON tag.
* Fields SHOULD be pointers if the nil and the zero values (by Go standards) have semantic differences.
  * Note: This doesn't apply to map or slice types as they are assignable to `nil`.

#### Example

When using ClusterClass, the semantic difference is important when you have a field in a template which will
have instance-specific different values in derived objects. Because in this case it's possible to set the field to `nil`
in the template and then the value can be set in derived objects without being overwritten by the cluster topology controller.

#### Exceptions

* Fields in root objects should be kept as scaffolded by kubebuilder, e.g.:
  ```golang
  type Machine struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   MachineSpec   `json:"spec,omitempty"`
    Status MachineStatus `json:"status,omitempty"`
  }
  type MachineList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []Machine `json:"items"`
  }
  ```

* Top-level fields in `status` must always have the `+optional` annotation. If we want the field to be always visible even if it
  has the zero value, it must **not** have the `omitempty` JSON tag, e.g.:
  * Replica counters like `availableReplicas` in the `MachineDeployment`
  * Flags expressing progress in the object lifecycle like `infrastructureReady` in `Machine`

### CRD additionalPrinterColumns

All our CRD objects should have the following `additionalPrinterColumns` order (if the respective field exists in the CRD):
* Namespace (added automatically)
* Name (added automatically)
* Cluster
* Other fields
* Replica-related fields
* Phase
* Age (mandatory field for all CRDs)
* Version
* Other fields for -o wide (fields with priority `1` are only shown with `-o wide` and not per default)

***NOTE***: The columns can be configured via the `kubebuilder:printcolumn` annotation on root objects. For examples, please see the `./api` package.

Examples:
```bash
$ kubectl get kubeadmcontrolplane
NAMESPACE            NAME                               INITIALIZED   API SERVER AVAILABLE   REPLICAS   READY   UPDATED   UNAVAILABLE   AGE     VERSION
quick-start-d5ufye   quick-start-ntysk0-control-plane   true          true                   1          1       1                       2m44s   v1.22.0
$ kubectl get machinedeployment
NAMESPACE            NAME                      CLUSTER              REPLICAS   READY   UPDATED   UNAVAILABLE   PHASE       AGE     VERSION
quick-start-d5ufye   quick-start-ntysk0-md-0   quick-start-ntysk0   1                  1         1             ScalingUp   3m28s   v1.22.0
```

## Google Doc Viewing Permissions

To gain viewing permissions to google docs in this project, please join either the
[kubernetes-dev](https://groups.google.com/forum/#!forum/kubernetes-dev) or
[kubernetes-sig-cluster-lifecycle](https://groups.google.com/forum/#!forum/kubernetes-sig-cluster-lifecycle) google
group.

## Issue and Pull Request Management

Anyone may comment on issues and submit reviews for pull requests. However, in order to be assigned an issue or pull
request, you must be a member of the [Kubernetes SIGs](https://github.com/kubernetes-sigs) GitHub organization.

If you are a Kubernetes GitHub organization member, you are eligible for membership in the Kubernetes SIGs GitHub
organization and can request membership by [opening an
issue](https://github.com/kubernetes/org/issues/new?template=membership.md&title=REQUEST%3A%20New%20membership%20for%20%3Cyour-GH-handle%3E)
against the kubernetes/org repo.

However, if you are a member of any of the related Kubernetes GitHub organizations but not of the Kubernetes org, you
will need explicit sponsorship for your membership request. You can read more about Kubernetes membership and
sponsorship [here](https://git.k8s.io/community/community-membership.md).

Cluster API maintainers can assign you an issue or pull request by leaving a `/assign <your Github ID>` comment on the
issue or pull request.

## Contributors Ladder

New contributors are welcomed to the community by existing members, helped with PR workflow, and directed to relevant documentation and communication channels.
We are also committed in helping people willing to do so in stepping up through the contributor ladder and this paragraph describes how we are trying to make this to happen.

As the project adoption increases and the codebase keeps growing, we’re trying to break down ownership into self-driven subareas of interest.
Requirements from the [Kubernetes community membership guidelines](https://github.com/kubernetes/community/blob/master/community-membership.md) apply for reviewers, maintainers and any member of these subareas.
Whenever you meet requisites for taking responsibilities in a subarea, the following procedure should be followed:
1. Submit a PR.
2. Propose at community meeting.
3. Get positive feedback and +1s in the PR and wait one week lazy consensus after agreement.

As of today there are following OWNERS files/Owner groups defining sub areas:
- [Clusterctl](cmd/clusterctl)
- [kubeadm Bootstrap Provider (CABPK)](bootstrap/kubeadm)
- [kubeadm Control Plane Provider (KCP)](controlplane/kubeadm)
- [Cluster Managed topologies, ClusterClass](controllers/topology)
- [Infrastructure Provider Docker (CAPD)](test/infrastructure/docker)
- [Test](test)
- [Test Framework](test/framework)
- [Docs](docs)