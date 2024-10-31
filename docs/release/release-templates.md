# Release Templates

This document contains a collection of announcement templates for the release team.

## Email Release Announcement Template

To use this template, send an email using the following template to the `sig-cluster-lifecycle@kubernetes.io` mailing list. The person sending out the email should ensure that they are first part of the mailing list.

```
Hello everyone,

Cluster API patch version vX.Y.Z has been released!

This release was focused on improving existing features and stability of CAPI. 
There were a total of A commits and B bugs fixed by our awesome contributors! Kudos!

Below is the release note of the vX.Y.Z patch release:
- https://github.com/kubernetes-sigs/cluster-api/releases/tag/vX.Y.Z

Thanks to all our contributors!

Best, 
CAPI Release Team
```

## Slack Weekly Announcement Template

Post the following template to the `#cluster-api` channel on the Kubernetes Slack under a thread. Make a separate post under the thread for the current release as well as the past two releases.

```
# Weekly announcement template

Weekly update :rotating_light:
Week `x` - `yyyy-mm-dd` to `yyyy-mm-dd` (If unsure what week check either Google Calendar or https://vecka.nu/)
From your friendly comms release team

Fun stats :wookie_party_time:
1. X PRs merged for all releases
1. X PRs merged into 1.X (main)
1. X bugs fixed

New Features
- :sparkles: Some feature..
- For a full list of all the changes see last weeks closed PRs see : https://github.com/kubernetes-sigs/cluster-api/pulls?q=is%3Apr+closed%3AYYYY-MM-DD..YYYY-MM-DD+is%3Amerged+

Important upcoming dates :spiral_calendar_pad: (OPTIONAL)
- vX.X.0-beta.0 released - (Add date here)
- release-X.X branch created (Begin [Code Freeze]) - (Add date here)
- vX.X.0 released - (Add date here)
```

## Slack Release Announcement Template

Post the following template to the `#cluster-api` channel on the Kubernetes Slack for each release.

```
# X.Y.Z Slack announcement example

:tada: :tada: :tada: 

Cluster API vX.Y.Z has been released :cluster-api:

This release was focused on improving existing features and stability of CAPI. 
There were a total of A commits and B bugs fixed by our awesome contributors! Kudos!

**Some of the hightlights in this release are**:
- :sparkles: Some feature..

Full list of changes: https://github.com/kubernetes-sigs/cluster-api/releases/tag/vX.Y.Z
