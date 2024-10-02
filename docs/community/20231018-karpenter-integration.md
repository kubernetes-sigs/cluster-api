---
title: Feature Group for Integrating Karpenter with Cluster API
authors:
  - "@elmiko"
reviewers:
    - "@mtougeron"
    - "@cnmcavoy"
    - "@nishant221"
    - "@faermanj"
    - "@ben-wilson-peak"
creation-date: 2023-10-18
last-updated: 2023-10-18
status: proposed
see-also:
  - https://github.com/kubernetes-sigs/cluster-api/issues/9523
  - https://www.youtube.com/watch?v=t1Uo18v8g48
  - https://karpenter.sh
---

# Karpenter Integration with Cluster API

This document briefly outlines the scope, communication media, and
stakeholders for a formal Feature Group dedicated to exploring and
defining how Karpenter can be integrated with Cluster API.

## User Story and Problem Statement

As a Cluster API administrator who uses node autoscaling to manage resource
in my clusters, I would like to utilize the Karpenter project with Cluster API
so that I can gain access to its features and capabilities.

At the time of writing, the Cluster API project only officially supports the
Kubernetes Cluster Autoscaler (CAS)for automatically managing node resources.
The Karpenter project is an alternative node autoscaler and autoprovisioner
which has community requested features, such as pod consolidation, that do not
have equivalents in the CAS. We would like to determine how Cluster API users
can utilize Karpenter, and if necessary create community supported projects to
support this activity.

## Scope

The scope of this effort will be the following:

1. Determine what options are available for a consistent Karpenter on Cluster
   API experience.
2. Agree upon a path forward to guide the Cluster API community with Karpenter
   usage.
3. Work towards a Cluster API Enhancement that describes the Karpenter integration.
4. Provide a space and time for discussion of issues related to the Karpenter Cluster API project.

Depending on the outcome of the 2nd item above, we may choose to keep this
Feature Group active during the longer term to support Karpenter integration
design and work efforts in Cluster API, with the blessing of the larger
Cluster API community.

_Update: 2024-10-02_

After discussion within the feature group and the broader Cluster API community,
we are continuing the feature group with a focus on experimentation and working
towards a CAEP that describes the integration with Karpenter. Additionally, we
will field any project related questions during the feature group.

## Communication

We will meet on [Wednesdays at 11:00 PT (Pacific Time)][zoomMeeting].
[Convert to your timezone][convert]. Meeting notes will be documented in this
[HackMD document][agenda]. Meetings will be recorded and posted to the
[SIG Cluster Lifecycle YouTube playlist][playlist].

Regular, summarized updates of group progress will be provided during weekly
Cluster API office hours on Wednesdays @ 10:00 PT on [Zoom][zoomMeeting].

Chat with stakeholders on Kubernetes [Slack](http://slack.k8s.io/) in the
[cluster-api](https://kubernetes.slack.com/archives/C8TSNPY4T) channel.

## Stakeholders

Primary Stakeholders are listed below:

- Michael McCune (@elmiko, Red Hat)

[zoomMeeting]: https://zoom.us/j/861487554
[convert]: http://www.thetimezoneconverter.com/?t=11:00&tz=PT%20%28Pacific%20Time%29
[agenda]: https://hackmd.io/@elmiko/ryR2VXR0n
[playlist]: https://www.youtube.com/playlist?list=PL69nYSiGNLP29D0nYgAGWt1ZFqS9Z7lw4
