# Code Review in Cluster API

## Goal of this document

- To help newcomers to the project in implementing better PRs given the knowledge of what will be evaluated 
  during the review.
- To help contributors in stepping up as a reviewer given a common understanding of what are the most relevant
  things to be evaluated during the review.

IMPORTANT: improving and maintaining this document is a collaborative effort, so we are encouraging constructive
feedback and suggestions.

   * [Code Review in Cluster API](#code-review-in-cluster-api)
      * [Goal of this document](#goal-of-this-document)
      * [Resources](#resources)
      * [Definition](#definition)
         * [Controller reentrancy](#controller-reentrancy)
         * [API design](#api-design)
            * [Serialization](#serialization)
            * [Owner References](#owner-references)
         * [The Cluster API contract](#the-cluster-api-contract)
         * [Logging](#logging)

## Resources

- [Writing inclusive documentation](https://developers.google.com/style/inclusive-documentation)
- [Contributor Summit NA 2019: Keeping the Bar High - How to be a bad ass Code Reviewer](https://www.youtube.com/watch?v=OZVv7-o8i40)
- [Code Review Developer Guide - Google](https://google.github.io/eng-practices/review/)
- [The Gentle Art Of Patch Review](https://sage.thesharps.us/2014/09/01/the-gentle-art-of-patch-review/)

## Definition

(from [Code Review Developer Guide - Google](https://google.github.io/eng-practices/review/)) 

_"A code review is a process where someone other than the author(s) of a piece of code examines that code"_

Within the context of cluster API the following design items should be carefully evaluated when reviewing a PR:

### Controller reentrancy

In CAPI most of the coding activities happen in controllers, and in order to make robust controllers,
we should strive for implementing reentrant code.

A reentrant code can be interrupted in the middle of its execution and then safely be called again
("re-entered"); this concept, applied to Kubernetes controllers, means that a controller should be capable
of recovering from interruptions, observe the current state of things, and act accordingly. e.g.
 
- We should not rely on flags/conditions from previous reconciliations since we are the controller
  setting the conditions. Instead, we should detect the status of things through introspection at
  every reconciliation and act accordingly.
- It is acceptable to rely on status flags/conditions that we've previously set as part
  of the current reconciliation.
- It is acceptable to rely on status flags/conditions set by other controllers.

NOTE: An important use case for reentrancy is the move operation, where Cluster API objects gets moved
to a different management cluster and the controller running on the target cluster has to
rebuild the object status from scratch by observing the current state of the underlying infrastructure.

### API design

The API defines the main contract with the Cluster API users. As most of the APIs in Kubernetes,
each API version encompasses a set of guarantees to the user in terms of support window, stability,
and upgradability. 

This makes API design a critical part of Cluster API development and usually:

- Breaking/major API changes should go through the CAEP process and be strictly synchronized with the major
  release cadence.
- Non-breaking/minor API changes can go in minor releases; non-breaking changes are generally:
  - additive in nature
  - default to pre-existing behavior
  - optional as part of the API contract

On top of that, following API design considerations apply.

#### Serialization

The Kubernetes API-machinery that is used for API serialization is build on top of three
technologies, most specifically:

- JSON serialization
- Open-API (for CRDs)
- the go type system

One of the areas where the interaction between those technologies is critical in the handling of optional
values in the API; also the usage of nested slices might lead to problems in case of concurrent
edits of the object.

#### Owner References

Cluster API leverages the owner ref chain of objects for several tasks, so it is crucial to evaluate the
impacts of any change that can impact this area. Above all:

- The delete operation leverages on the owner ref chain for ensuring the cleanup of all the resources when
  a cluster is deleted; 
- clusterctl move uses the owner ref chain for determining which object to move and the create/delete order.

### The Cluster API contract

The Cluster API rules define a set of rules/conventions the different provider authors should follow in
order to implement providers that can interact with the core Cluster API controllers, as 
documented [here](https://cluster-api.sigs.k8s.io/developer/guide.html) and [here](https://cluster-api.sigs.k8s.io/clusterctl/provider-contract.html).

By extension, the Cluster API contract includes all the util methods that Cluster API exposes for
making the development of providers simpler and consistent (e.g. everything under `/util` or in  `/test/framework`);
documentation of the utility is available [here](https://pkg.go.dev/sigs.k8s.io/cluster-api?tab=subdirectories).

The Cluster API contract is linked to the version of the API (e.g. v1alpha3 Contract), and it is expected to
provide the same set of guarantees in terms of support window, stability, and upgradability. 

This makes any change that can impact the Cluster API contract critical and usually:

- Breaking/major contract changes should go through the CAEP process and be strictly synchronized with the major
  release cadence.
- Non-breaking/minor changes can go in minor releases; non-breaking changes are generally:
  - Additive in nature
  - Default to pre-existing behavior
  - Optional as part of the API contract

### Logging

- For CAPI controllers see [Kubernetes logging conventions](https://git.k8s.io/community/contributors/devel/sig-instrumentation/logging.md).
- For clusterctl see [clusterctl logging conventions](https://github.com/kubernetes-sigs/cluster-api/blob/main/cmd/clusterctl/log/doc.go).

### Testing

Testing plays an crucial role in ensuring the long term maintainability of the project.

In Cluster API we are committed to have a good test coverage and also to have a nice and consistent style in implementing
tests. For more information see [testing Cluster API](https://cluster-api.sigs.k8s.io/developer/testing.html).  