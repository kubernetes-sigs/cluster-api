---
title: Feature Group for Mixed Clusters in Cluster API
authors:
  - "@prometherion"
  - "@lentzi90"
reviewers:
  - "@lentzi90"
  - "@richardcase"
  - "@fabriziopandini"
creation-date: 2023-01-04
last-updated: 2023-02-07
status: proposed
see-also:
  - https://github.com/kubernetes-sigs/cluster-api/issues/7475#issuecomment-1302454745
  - https://kubernetes.slack.com/archives/C8TSNPY4T/p1667458956589639
---
# Mixed Clusters in Cluster API Feature Group

This document briefly outlines the scope, communication media, and stakeholders for a formal Feature Group dedicated to defining a Cluster API-approved solution for "mixed clusters" which combines two or more infrastructure providers.

## User Story

As a Kubernetes platform administrator, I have to offer a customer-facing platform being able to provision multiple Kubernetes, acting as a single pane of glass of the whole clusters.

Rather than managing a 1:1 relation between Control Plane and Worker components for each cluster, I want to adopt a hyper-scale approach offering a Control Plane service as a Cluster API Control Plane provider (e.g.: Kamaji), responsible to provision the required components in a dedicated management cluster. These Control Planes are used on behalf of each customer to finalize the provisioning of owned compute nodes in multiple and different infrastructures, as well as on-prem as in the cloud.

Another example is the [CAPO](https://github.com/kubernetes-sigs/cluster-api-provider-openstack) for the Control Plane component and Auto Scaling node pool, along with [Metal3](https://metal3.io/) for bare metal needs.

## Problem Statement

The current design of most CAPI Cluster providers is have both components (Control Plane and Worker) backed by the same underlying infrastructure.

We aim to extend and build on the great work done so far by providers, making it possible to use different infrastructure providers in the same cluster as well as ensuring a consistent UX when using different combinations of providers.

## Scope

The scope of this group is to write proposal(s) and engage with the community to reach consensus around how to support this feature.

## Communication

We will bi-weekly meet on [Tuesday at 10:00 AM (CET)][https://zoom.us/j/861487554]. [Convert to your timezone](https://dateful.com/time-zone-converter?t=10:00&tz=CET%20%28Central%20European%20Time%29). Meeting notes will be documented in this [Google Doc](https://docs.google.com/document/d/12TGubbGJa3w4ux8kJnR7sb4l39iHxcOAtbJqLwSlnkc/edit?usp=sharing).

Regular, summarized updates of group progress will be provided during weekly Cluster API office hours on Wednesdays @ 10:00 PT on [Zoom][https://zoom.us/j/861487554].

Chat with stakeholders on Kubernetes [Slack](http://slack.k8s.io/) in the [cluster-api](https://kubernetes.slack.com/archives/C8TSNPY4T) channel.

## Stakeholders

Primary Stakeholders are listed below:

- Lennart Jern (@lentzi90, Ericsson)
- Dario Tranchitella (@prometherion, CLASTIX)
- Richard Case (@richardcase, SUSE)

[zoomMeeting]: https://zoom.us/j/861487554
