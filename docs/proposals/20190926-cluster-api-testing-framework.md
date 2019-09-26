---
title: Cluster API testing framework
authors:
  - "@chuckha"
reviewers: []
creation-date: 2019-09-26
last-updated: 2019-09-26
status: provisional
see-also: []
replaces: []
superseded-by: []
---

# Cluster API testing framework

## Table of contents

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->


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

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Summary

Cluster API can improve stability and the stability of providers by haivng a pluggable test suite. Providers would be
able to import the test suite, plug in their own controller and have a set of e2e tests that exercise all
expected functionality of Cluster API. 

## Motivation

Every provider, infrastructure, core, or bootstrap, will end up rewriting the same high-level end-to-end tests. We can ease this process
by writing a set of tests that providers can reuse to create an end-to-end suite that they are responsible for running.

### Goals

* Define a set of tests that all providers can pass
* Create an easily pluggable framework for providers to adopt
* Integrate the framework into Cluster API
* Integrate the framework into Cluster API Provider Docker
* Integrate the framework into Cluster API Bootstrap Provider Kubeadm
* Help interested parties adopt the framework

### Non-Goals

* Replace any testing that is specific to a provider
* To define conformance using these tests

## Proposal

### User Stories [optional]

#### Story 1

#### Story 2

### Implementation Details/Notes/Constraints [optional]

### Risks and Mitigations

## Design Details

### Test Plan

### Graduation Criteria

#### Examples

##### Alpha -> Beta Graduation

##### Beta -> GA Graduation

##### Removing a deprecated flag

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

## Drawbacks [optional]

## Alternatives [optional]
