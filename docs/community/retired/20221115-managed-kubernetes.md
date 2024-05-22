---
title: Feature Group for Managed Kubernetes in Cluster API
authors:
  - "@jackfrancis"
reviewers:
    - "@pydctw”"
    - "@richardcase"
    - "@vincepri"
    - "@sbueringer”"
    - "@fabriziopandini"
    - "@killianmuldoon"
creation-date: 2022-11-15
last-updated: 2022-12-06
status: retired
see-also:
  - https://github.com/kubernetes-sigs/cluster-api/issues/7494
  - https://github.com/kubernetes-sigs/cluster-api/issues/6988
---
# This Feature Group is Retired!

The Managed Kuberentes Feature Group produced this CAEP, which defines practical design and implementation work for evolving Managed Kubernetes stories in the Cluster API project, and negates the need for a dedicated feature group going forward:

- https://github.com/kubernetes-sigs/cluster-api/pull/8500

# Managed Kubernetes in Cluster API Feature Group

This document briefly outlines the scope, communication media, and stakeholders for a formal Feature Group dedicated to defining a Cluster API-approved solution to manage clusters with Managed Kubernetes services.

## User Story and Problem Statement

As Kubernetes cluster admin who leverages Cluster API to maintain Managed Kubernetes clusters (e.g., GKE, EKS, OKE, AKS), I want a consistent experience across providers (e.g., GCP, AWS, Oracle Cloud, Azure) to empower multi-cloud, Managed Kubernetes scenarios.

At present, the Cluster API Managed Kubernetes implementations (e.g., `AzureManagedCluster`, `AzureManagedControlPlane`, and `AzureManagedMachinePool` in CAPZ; and `AWSManagedCluster`, `AWSManagedControlPlane`, and `AWSManagedMachinePool` in CAPA) differ in non-trivial ways across cloud providers, imposing friction for multi-cloud Cluster API users of Managed Kubernetes. We want to reduce that cross-provider friction and provide a more consistent management experience for all Cluster API Managed Kubernetes solutions, in order to accelerate multi-cloud innovations for users managing highly distributed fleets of Kubernetes clusters at scale.

## Scope

The scope of this effort will be the following:

1. Ensure current inconsistencies in Managed Kubernetes implementations across providers are well documented.
2. Agree upon a path forward to achieve provider consistency for Managed Kubernetes in Cluster API.

Depending on the outcome of the 2nd item above, we may choose to keep this Feature Group active during the longer term to support Managed Kubernetes design and work efforts in Cluster API, with the blessing of the larger Cluster API community.

## Communication

We will meet on [Wednesdays at 09:00 PT (Pacific Time)][zoomMeeting]. [Convert to your timezone](http://www.thetimezoneconverter.com/?t=09:00&tz=PT%20%28Pacific%20Time%29). Meeting notes will be documented in this [Google Doc](https://docs.google.com/document/d/1WIdlkYU53r5UWM3m6YaDXPYyMBNyiJcKL6UX7VekfYY).

Regular, summarized updates of group progress will be provided during weekly Cluster API office hours on Wednesdays @ 10:00 PT on [Zoom][zoomMeeting].

Chat with stakeholders on Kubernetes [Slack](http://slack.k8s.io/) in the [cluster-api](https://kubernetes.slack.com/archives/C8TSNPY4T) channel.

## Stakeholders

Primary Stakeholders are listed below:

- Winnie Kwon (@pydctw, VMware)
- Richard Case (@richardcase, SUSE)
- Jack Francis (@jackfrancis, Microsoft)

[zoomMeeting]: https://zoom.us/j/861487554
