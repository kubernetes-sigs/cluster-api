---
title: ClusterAPI Node Bootstrapping Working Group
authors:
    - "@t-lo"
reviewers:
    - "@elmiko"
    - "@eljohnson92"
    - "@johananl"
    - "@tormath1"
    - "@fabriziopandini"
    - "@sbueringer"
    - "@enxebre"

creation-date: 2024-11-12
last-updated: 2024-11-15
status: proposed
see-also:
  - https://github.com/kubernetes-sigs/cluster-api/issues/5294
  - https://docs.google.com/document/d/1Fz5vWwhWA-d25_QDqep0LWF6ae0DnTqd5-8k8N0vDDM/edit
  - https://github.com/kubernetes-sigs/cluster-api/issues/9157
  - https://youtu.be/0AhA4Box3MM?si=IfKJfMWmlA9EW7ri&t=172
  - https://docs.google.com/document/d/12v6NFr7xal9RH3GEB_1IR6HENq2GMuzALS3l5elvrMg/edit
---

# Node Bootstrapping Working Group ("CAPI-NoBo")

This document briefly outlines the scope, communication media, and
stakeholders for a Working Group dedicated to the Provisioning aspect of
Node Bootstrapping.

Provisioning, in this context, refers to configuring and customizing the
node's operating system to prepare it to serve as a ClusterAPI cluster node.


## User Story and Problem Statement

As a Linux Distribution maintainer aiming to support ClusterAPI, or to improve
my existing support of ClusterAPI in my distribution, I would like to be able to
cleanly integrate my contributions into the node bootstrapping process without
interfering with other implementations.
I would like my contributions to be as re-usable as possible across different
bootstrap providers.
I would like to read and to follow documentation, guidelines, and specifications
on the above.
I would like to offer a choice of node bootstrapping configuration systems to users,
enable distribution vendors to extend the choices available by adding new
configuration systems, and ease maintenance and enable extension of currently
supported systems for the ClusterAPI developer community.
I would like to enable the bootstrap API consumer to express intent to aid node
bootstrapping through provisioning-time customizations like setting up disks and
file systems decoupled from specific OS provisioning configuration implementations.


**Problem statement / Example issues**

As there is currently no OS provisioning configuration abstraction, the kubeadm bootstrap provider
is tightly coupled with cloud-init and Ignition. Furthermore, the Ignition implementation is built
on top of the cloud-init one. This makes it hard to develop provisioning implementations
independently and makes it hard to reuse code effectively between bootstrap implementations: Other
bootstrap providers such as MicroK8s must implement their own OS provisioning, and MicroK8s
currently only supports cloud-init.

In addition, the current design mixes bootstrap-related provisioning code (such as the commands
necessary for executing the kubeadm binary on a host) with generic OS customizations (such as disk
partitioning or Linux user configuration).

Outside of ClusterAPI, both [cloud-init](https://github.com/canonical/cloud-init)
and [ignition](https://github.com/coreos/ignition) provisioning are widely adopted
across distributions.
While cloud-init is used by general-purpose Linux distributions like Ubuntu/Debian,
SUSE Linux, Alma, Rocky, Fedora, and Red Hat Enterprise Linux, ignition is popular
with special-purpose distributions like Fedora CoreOS / Red Hat CoreOS, SuSE MicroOS
/ SLE Micro / Kalpa, and Flatcar Container Linux.
It is likely that more provisioning systems exist; a clean separation between bootstrap and provisioning
and guidelines on how each is developed will make it easier to add support to ClusterAPI as well as to
share implementations across bootstrap providers.

We recognize that a node's OS provisioning configuration can be overridden and fully customized via custom
userdata.
We consider this feature out of scope of this working group as the user data would need to be kept in sync with
ClusterAPI internals to meet ClusterAPI's provisioning needs and to avoid conflicts.
This would inflict a heavy maintenance toll on users.

**Compatibility and API guarantees**

ClusterAPI has been stable for multiple years and is in widespread production use.
Proposals and implementations of this working group must ensure to not exert operational
risk on existing integrations.
Any changes in the kubeadm bootstrap provider in particular must uphold guarantees and
expectations set by the current implementation and must continue to support current use
cases OR must provide means for current use cases to continue functioning for an appropriate
migration period, as well as define a clear path for migration.

## Scope

1. Propose architectural improvements that abstract OS provisioning configuration with the goal of
   reducing duplicate functionality in current implementations, easing development of new OS provisioning
   features, and simplifying integration of new configuration systems for OS provisioning.
2. Deliver an example implementation which works with the kubeadm bootstrap provider.
3. Approach other bootstrap providers and provide help and guidance for separating provisioning
   and re-using provisioning implementations.

## Communication

* Meeting schedule: Fortnightly; every first and third Thursday of the month at 17:00 UTC
  (WG [calendar](https://calendar.google.com/calendar/u/0?cid=OTBkMjJjZGU0OTcyZjI0OGQ2NTE2YTk2ZGUwNWVmNjI1NTM2NDRmZmYyNjFlMjE1MGY1ZjIyOTU0NmQ1OWQ0MUBncm91cC5jYWxlbmRhci5nb29nbGUuY29t),
      [iCal](https://calendar.google.com/calendar/ical/90d22cde4972f248d6516a96de05ef62553644fff261e2150f5f229546d59d41@group.calendar.google.com/public/basic.ics) )
* Meeting agenda / minutes: https://docs.google.com/document/d/12v6NFr7xal9RH3GEB_1IR6HENq2GMuzALS3l5elvrMg/edit

We will join the [regular ClusterAPI meetings](https://github.com/flatcar-hub/cluster-api?tab=readme-ov-file#-community-discussion-contribution-and-support)
and share summary updates on our work and our progress in the "Feature Groups" section, which is a regular part of
the ClusterAPI meetings.

Chat with stakeholders on Kubernetes [Slack](http://slack.k8s.io/) in the
[cluster-api](https://kubernetes.slack.com/archives/C8TSNPY4T) channel.

## Stakeholders

Primary Stakeholders are listed below:

- The Flatcar Container Linux project
  - Johanan Liebermann (@johananl, Microsoft)
  - Mathieu Tortuyaux (@tormath1, Microsoft)
  - Thilo Fromm (@t-lo, Microsoft)
- Fabrizio Pandini (@fabriziopandini, VMWare)
- Michael McCune (@elmiko, Red Hat)
- Jakob Schrettenbrunner (@schrej, Deutsche Telekom AG)
- Mohamed Chiheb Ben Jemaa (@mcbenjemaa, IONOS Cloud)
- Kashif Khan (@kashifest)
- Evan Johnson (@eljohnson92, Akamai)
- Mauro Morales (@mauromorales, KairOS)
