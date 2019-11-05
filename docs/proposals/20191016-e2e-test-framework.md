---
title: Cluster API testing framework
authors:
  - "@chuckha"
  - "@liztio"
reviewers: 
  - "@akutz"
  - "@andrewsykim"
  - "@ashish-amarnath"
  - "@detiber"
  - "@joonas"
  - "@ncdc"
  - "@vincepri"
  - "@wfernandes"
creation-date: 2019-09-26
last-updated: 2019-09-26
status: implementable
see-also: []
replaces: []
superseded-by: []
---

# Cluster API testing framework

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
    - [As a Cluster API provider implementor I want to know my provider behaves in a way that Cluster API expects](#as-a-cluster-api-provider-implementor-i-want-to-know-my-provider-behaves-in-a-way-that-cluster-api-expects)
    - [As a developer in the Cluster API ecosystem I want to have confidence that my change does not break expected behaviors](#as-a-developer-in-the-cluster-api-ecosystem-i-want-to-have-confidence-that-my-change-does-not-break-expected-behaviors)
    - [As a Cluster API developer, I want to be sure I’m not accidentally disrupting existing providers.](#as-a-cluster-api-developer-i-want-to-be-sure-im-not-accidentally-disrupting-existing-providers)
    - [As the release manager of a Cluster API provider, I want to be able to recommend new releases to users as soon as they come up](#as-the-release-manager-of-a-cluster-api-provider-i-want-to-be-able-to-recommend-new-releases-to-users-as-soon-as-they-come-up)
    - [As a user of Cluster API, I want to be sure new releases I’m installing won’t cause regressions.](#as-a-user-of-cluster-api-i-want-to-be-sure-new-releases-im-installing-wont-cause-regressions)
  - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
    - [Behaviors to test](#behaviors-to-test)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Alternatives](#alternatives)
  - [Do not provide an end-to-end test suite](#do-not-provide-an-end-to-end-test-suite)
  - [Extend existing e2es](#extend-existing-e2es)
- [Upgrade Strategy](#upgrade-strategy)
- [Additional Details](#additional-details)
  - [Test Plan](#test-plan)
  - [Version Skew Strategy [optional]](#version-skew-strategy-optional)
- [Implementation History](#implementation-history)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Glossary

Refer to the [Cluster API Book Glossary](https://cluster-api.sigs.k8s.io/reference/glossary.html).

If this proposal adds new terms, or defines some, make the changes to the book's glossary when in PR stage.

## Summary

Cluster API's providers could benefit from having a set of generic behavioral e2e tests. Most providers have some form
of end-to-end testing but they are not uniform and do not all test the same behavior that Cluster API promises.

A pluggable set of behavioral tests for Cluster API Providers would prove beneficial for the project health as a whole.

As stated in the non-goals, this proposal does not intend to define the conformance requirements for Cluster API
providers.

## Motivation

Every infrastructure and bootstrap provider wants to know if it "works", but testing today is a manual process. When
we (developers) want to test a change using the entire stack we have a lot of steps to manually go through. Because it
is such tedious work we are likely to only exercise one or two scenarios at most. This test framework will remove the
toil of testing by hand.

A bonus to creating a pluggable suite of behaviors is the consolidation of expectations in one place. The test suite is
a direct reflection of Cluster API's expectations and thus is versioned along with the cluster-api repository.

### Goals

* Define sets of tests that all providers can pass
  * End-to-end tests
  * (optional) Unit tests
* Build a test suite as a go library that providers can import, supply their own types/objects that satisfy whatever
  interface is required of the tests
  * Test suite can be run in parallel
  * Tests should be organized so providers can specify certain tests to run or skip. For example, a fast-focus could be
  a PR blocking job whereas the slow tests could run periodically
* Create documentation for plugging a provider into this library
    * Particularly tricky bits here will be providing guidance on how to manage secrets

### Non-Goals/Future Work

* Replace any testing that is specific to a provider
* To define conformance using these tests
* Produce a binary to be run that executes CAPI tests (a la k8s conformance)
* Generic secret management for Cluster API infrastructure cloud providers
  * Secret management for running this set of tests will be decoupled from this framework and put out of scope. The
  provider will be responsible for setting up their own set of secrets.
* Provide test-infra / prow integration. This is left up to each provider as not all providers will be inside the
  kubernetes or kubernetes-sigs organization.

## Proposal

The crux of this proposal is to implement a test suite/framework/library that providers can use as e2e tests. It will
not cover provider specific edge cases. Providers are still left to implement their own provider specific e2es.

This framework will capture behaviors of Cluster API that span all providers.

### User Stories

#### As a Cluster API provider implementor I want to know my provider behaves in a way that Cluster API expects

By providing a pluggable framework that contains generic Cluster API behavior for the provider implementor to use, we
are able to satisfy this user story.

#### As a developer in the Cluster API ecosystem I want to have confidence that my change does not break expected behaviors

By writing down the expected behaviors of Cluster API in the form of a test framework, we will be able to know if our
changes break those expected behaviors and thus satisfy this user story.

#### As a Cluster API developer, I want to be sure I’m not accidentally disrupting existing providers.

Existing providers will be depending on Cluster API behavior. This is an alternative perspective of the previous user
story.

#### As the release manager of a Cluster API provider, I want to be able to recommend new releases to users as soon as they come up

Not all providers have automated testing that use all components of Cluster API and test interactions. This framework
would be able to provide that signal to release managers to know if a release will at least pass this suite.

#### As a user of Cluster API, I want to be sure new releases I’m installing won’t cause regressions.

Regressions are easy to introduce. If we happen to introduce a regression that is not covered by these behaviors we will
be able to identify how to write a test to reproduce the bad behavior and to fix the underlying bug.

### Implementation Details/Notes/Constraints

This framework will be implemented using the Ginkgo/Gomega behavioral testing framework because that is the Kubernetes
standard and does a good job of structuring and organizing behavior based tests.

This framework will live in the `sigs.k8s.io/cluster-api` module because it will be formalizing behaviors of Cluster
API. Therefore the set of behaviors for each version may change, so will the test suite.

This framework is not conformance nor will be built in the same style as the conformance framework of kubernetes. That
is to say, there will not be a distributed binary. The only artifact needed is Cluster API's go module appropriately
tagged and pushed.

These tests will be consumed by a provider just like any other go library dependency. They can be imported and provider
custom types can be passed in and the e2es will be run for the given provider, assuming secrets are also managed

The exact interface that a provider must satisfy is to be determined by what the tests demand. The plan for
implementation at a high level is to build out reusable but pluggable testing components. For example, the framework
will need a way to start a management cluster. We will use KIND initially and abstract the actual details away and
expose an interface. If you want to use a different style of management cluster, say, a cloud-formation based cluster,
then you will have to wrap up the cloud-formation code in some object and implement the testing framework’s exposed
interface for the management cluster and use your object where the sample code uses the KIND-based management cluster
struct. I expect but cannot guarantee that there will be interfaces for each provider but exact details are to be
determined as the implementation gets more fleshed out.

#### Behaviors to test

* There is a Kubernetes cluster with one node that passes a healthcheck after creating a properly configured Cluster,
  InfraCluster, Machine, InfraMachine and BootstrapConfig.
* Creating the resources necessary for three control planes will create a three node cluster.
* Deleting a cluster deletes all resources associated with that cluster including Machines, BootstrapConfigs,
  InfraMachines, InfraCluster, and generated secrets.
* Creating a cluster with one control plane and one worker node will result in a cluster with two nodes.
* The version fields in Machines are respected within the bounds of the Kubernetes skew policy.
* Creating a control plane machine and a MachineDeployment with two replicas will create a three node cluster with one
  control plane node and two worker nodes.
* MachineDeployments do their best to keep Machines in an expected state. For example:
  * Modifying a replica count on a MachineDeployment will modify the number of worker nodes and Machines in running state
  the cluster has.
  * Deleting a machine that is managed by a MachineDeployment will be recreated by the MachineDeployment
* Optionally, Machines report failures when their underlying InfraMachines report failures.
* Manage multiple workload clusters in different namespaces.
* Workload Clusters created pass Kubernetes Conformance.

### Risks and Mitigations

These tests have two main risks: False positives/negatives and long-term technical debt. 

We want these E2E tests to be useful for providers to use in a test-infra prow job. Depending on the providers’ choices,
these tests will be able to be run as a regular test suite. Therefore they can run as pre jobs, post jobs, periodics or
any other type of job that Prow supports. *Ideally these tests will replace, at least partially, the need for manual
testing*. If the tests are too brittle, development velocity could take a hit, and if they’re too lenient, bugs are
more likely to make it into production.

To keep these tests healthy and useful, cluster-api will need clear ownership on this common testing infrastructure.
Keeping these tests happy and healthy across providers will be key to success, not just now, but going forward as well.

The Cluster API project is developing rapidly, and any testing framework must match that pace. The framework will need
to be flexible enough to handle v1alpha2, 3, and onward; including beta and general releases. If the test framework is
too restrictive, then the tests could become less useful over time, or worse, constrain development of Cluster API
itself. The framework needs to be one of the first things ported to a new release of Cluster API, or it will be much
more difficult to validate providers against those new releases. Clear ownership is necessary here, as is committing to
the test framework as a release artifact.

## Alternatives

### Do not provide an end-to-end test suite

We could not do this and leave end-to-end testing up to each provider. This is a fine approach but then we are in the
same place as we are today. What signal do we use when releasing a provider? Generally a developer tests one or two
cases without testing every case we'd like to test. This framework aims to be used by providers to improve signal for
release quality.

### Extend existing e2es

Extending our existing e2es from a given provider is a fine approach, but instead of attempting to extend code beyond
what it was expected to do, we could also learn from their approach and design with a library in mind instead of having
a focus on an individual provider. Doing this will help keep tests separate from "what is provider behavior" vs "what is
Cluster API behavior".

## Upgrade Strategy

Users can use go modules to manage the up and downgrade of this framework.

## Additional Details

An end-to-end test should be designed to exercise the whole system top to bottom. This means we're essentially
automating all the things a developer would do when testing a change for all the different cluster configurations we
decide we care about.

First, create a management cluster. Apply the provider components (swap ⅓) of them out for the provider under test.

### Test Plan

As these are tests designed to run as part of a prow job they, they will be run fairly frequently. I’m not sure we need
a test plan for a test framework, but if we do need tricky logic we can always write unit tests for the test framework.

### Version Skew Strategy [optional]

This framework will follow Cluster API’s release cadence as it will be a package of the `sigs.k8s.io/cluster-api`
module. Therefore the version skew is handled identically to Cluster API.

## Implementation History

- [x] 09/23/2019: Compile a Google Doc following the CAEP template (link here)
- [x] 09/23/2019: First round of feedback from community
- [x] 09/25/2019: Present proposal at a [community meeting]
- [x] 10/16/2019: Open proposal PR



