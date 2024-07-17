---
title: Feature Group for In-place Updates support in Cluster API
authors:
  - "@g-gaston"
reviewers:
    - "@dharmjit"
    - "@vincepri"
    - "@sbueringer"
    - "@fabriziopandini"
creation-date: 2023-10-16
last-updated: 2024-07-17
status: proposed
see-also:
  - https://github.com/kubernetes-sigs/cluster-api/issues/9489
  - https://docs.google.com/document/d/1CqQ1SAqJD264PsDeMj_Z3HhZxe7DViNkpJ9d5q-2Zck
---
# In-place Updates Feature Group

This document briefly outlines the scope, communication media, and stakeholders for a formal Feature Group dedicated to defining a Cluster API-approved solution to upgrade the kubernetes components running in CAPI managed nodes without replacing those machines.

## User Story and Problem Statement

As a CAPI Kubernetes cluster admin I want to upgrade the k8s components (for example, a minor Kubernetes upgrade) of my cluster without replacing the machines.

At present, CAPI has "immutable infrastructure" as one of its axioms: once created, Machines are not updated, they are just replaced (a new one is created and old one is deleted). As a consequence, the only supported update strategy is rolling update. However, for certain use cases (such as Single-Node Clusters with no spare capacity, Multi-Node Clusters with VM/OS customizations for high-performance/low-latency workloads or dependency on local persistent storage), upgrading a cluster via RollingUpdate strategy could either be not feasible or a costly operation (requiring to re-apply customizations on newer nodes and hence more downtime).

## Scope

The scope of this effort will be the following:

1. Define the scope of what In-place updates means in the context of Cluster API.
2. Write a CAPI proposal with the recommended solution.

## Communication

We will meet on [Wednesdays at 08:00 PT (Pacific Time)][zoomMeeting]. [Convert to your timezone](https://dateful.com/time-zone-converter?t=09:00&tz=PT%20%28Pacific%20Time%29). Meeting notes will be documented in this [Google Doc](https://docs.google.com/document/d/1GmRd6MyQ0mWAoJV6rCHhZTSTtKMKHdJzhXm0BLBXOnw).

Regular, summarized updates of group progress will be provided during weekly Cluster API office hours on Wednesdays @ 10:00 PT on [Zoom][zoomMeeting].

Chat with stakeholders on Kubernetes [Slack](http://slack.k8s.io/) in the [cluster-api](https://kubernetes.slack.com/archives/C8TSNPY4T) channel.

## Stakeholders

Primary Stakeholders are listed below:

- Mayur Das (@mayur-tolexo, VMware)
- Alexander Demicev (@alexander-demicev, SUSE)
- Scott Dodson (@sdodson, Red Hat)
- Guillermo Gaston (@g-gaston, AWS)
- Furkat Gofurov (@furkatgofurov7, SUSE)
- Dharmjit Singh (@dharmjit, VMware)

[zoomMeeting]: https://zoom.us/j/861487554
