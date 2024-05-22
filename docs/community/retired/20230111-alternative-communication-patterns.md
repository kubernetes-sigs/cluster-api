---
title: Feature Group for alternative communication patterns
authors:
  - "@fgutmann"
  - "@richardcase"
reviewers:
  - "@fgutmann"
  - "@richardcase"
  - "@alexander-demicev"
creation-date: 2023-01-11
last-updated: 2023-01-26
status: proposed
see-also:
  - https://github.com/kubernetes-sigs/cluster-api/issues/6520
  - https://docs.google.com/document/d/1j4sCPGO_0e1G-IyiI_8s98R3RVrYsgY9n0VFcde3ELo/edit#heading=h.lcjlkg7scook
---

# This Feature Group is Retired!

We are putting this work on hold due to lack of contributors/lack of interest.

# Alternative Communication Patterns Feature Group

This document briefly outlines the scope, communication media, and stakeholders for a formal Feature Group dedicated to defining a Cluster API-approved solution for supporting alternative communication patterns. Currently Cluster API assumes that it can initiate direct connections to all child clusters and/or to an infrastructure control plane. For some users / usage scenarios this isn't possible (technically or by policy) and so this group will investigate alternatives to enable these types of usage scenarios.

## User Story and Problem Statement

From [discussion](https://github.com/kubernetes-sigs/cluster-api/issues/6520#issuecomment-1341517675) this group will consider 3 uses cases:

1. A solution for managed clusters only, which does not require API-server connectivity from CAPI at all.
1. The "common cluster as a service offering" scenario where only the workload clusters' worker nodes are in a different network.
1. The scenario where a management cluster manages workload clusters (including control plane nodes), which are in a different network than the management cluster.

> Although use case 3 will likely be the main focus.

## Scope

The scope of this group will be the following:

1. Investigate alternative communication patterns in relation to the user stories
2. Create a proof-of-concept (if applicable)
3. Create a CAPI proposal with recommended changes (if applicable)
4. Implement the proposal (if applicable)

## Communication

We will bi-weekly meet on Fridays @ 09:00 PT on [Zoom][zoomMetting]. [Convert to your timezone](https://dateful.com/convert/pst-pdt-pacific-time?t=01&tz2=Greenwich-Mean-Time-GMT). Meeting notes will be documented in this [Google Doc][meetingnotes].

Regular, summarized updates of group progress will be provided during weekly Cluster API office hours on Wednesdays @ 10:00 PT on [Zoom][zoomMeeting].

Chat with stakeholders on Kubernetes [Slack](http://slack.k8s.io/) in the [cluster-api](https://kubernetes.slack.com/archives/C8TSNPY4T) channel.

## Stakeholders

Primary Stakeholders are listed below:

- Florian Gutmann (@fgutmann, AWS)
- Richard Case (@richardcase, SUSE)
- Alexander Demicev (@alexander-demicev, SUSE)

[zoomMeeting]: https://zoom.us/j/861487554
[meetingnotes]: https://docs.google.com/document/d/1Q1ShR7H_W1EUYOB_5MCtM91kDuGg59bmuVcR4qkwaeA/edit?usp=sharing
