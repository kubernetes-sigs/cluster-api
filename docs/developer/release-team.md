# Cluster API Release Team

## Overview

In the past, releasing Cluster API has been mostly ad-hoc and relied on one or more contributors to do most of the chore work necessary to prepare the release. One of the major downsides of this approach is that it is often difficult for users and providers to plan around Cluster API releases as they often have little visibility around when a release will happen. 

This document introduces the concept of a release team with the following goals and non-goals:

### Goals

- To improve communication to end users and contributors about CAPI release cadence and target release dates
- To spread the load of release tasks and involve a bigger, more diverse set of CAPI contributors in cutting releases
- To look at the Kubernetes SIG release team processes for guidance on releasing best practices
- To improve tooling, documentation, and automation of the CAPI release process

### Non-Goals/Future work

- To increase the frequency of releases (AKA release cadence). This will be revisited in the future once the release process stabilizes.
- To change the current proposal (CAEP) process
- To copy implement all steps of the Kubernetes release process for CAPI releases

Note that this document is intended to be a starting point for the release team. It is not a complete release process document.

More details on the CAPI release process can be found in [this past issue tracking release tasks](https://github.com/kubernetes-sigs/cluster-api/issues/6615) and the current [release documentation](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/developer/releasing.md).

## Duration of Term

Each release team term will last approximately four months, to align with one minor release cycle. A minor release cycle starts right after a minor release and concludes with the release of the next minor release. There is no limit to the number of terms a release team member can serve, meaning that it's possible for a release team member to serve multiple consecutive terms.

As noted above, making changes to  the CAPI release cadence is out of scope for this initial release team process. 

## Specific Responsibilities

- Release patch releases monthly or more often as needed, so users will get fixes & updated dependencies with CVE fix on a predictable cadence
- Release a minor release every 4 months (to be revised, see future work), so users will get new features on a predictable cadence
- Create beta and release candidate (rc) tags for upcoming minor releases
- Ensure only eligible PRs are cherry-picked to the active release branches
- Monitor CI signal, so a release can be cut at any time, and add CI signal for each new release branch
- Maintain and improve user facing documentation about releases, release policy and calendar
- Update the CAPI Netlify book certificates and DNS
- Update the clusterctl Homebrew formula
- Create and maintain the GitHub release milestone
- Maintain and improve and release automation, tooling & related developer docs
- Track tasks needed to add support for new Kubernetes versions in the next CAPI release


## Team Roles

- **Release Lead**: responsible for coordinating release activities, assembling the release team, taking ultimate accountability for all release tasks to be completed on time, and ensuring that a retrospective happens. The lead is also responsible for ensuring a successor is selected and trained for future release cycles.
- **Communications/Docs/Release Notes Manager**: Responsible for communicating key dates to the community, improving release process documentation, and polishing release notes. Also responsible for ensuring the user-facing Netlify book and provider upgrade documentation are up to date.
- **CI Signal/Bug Triage/Automation Manager**: Assumes the responsibility of the quality gate for the release and makes sure blocking issues and bugs are triaged and dealt with in a timely fashion. Helps improve release automation and tools.
- **Shadow**: Any Release Team member may select one or more mentees to shadow the release process in order to help fulfill future Release Team staffing requirements and continue to grow the CAPI community in general.

## Team Selection

To start, the release team will be assembled by the release team lead based on volunteers. A call for volunteers can be made through the usual communication channels (office hours, Slack, mailing list, etc.). In the future, we may consider introducing an application process similar to the Kubernetes release team application process. 

### Selection Criteria

When assembling a release team, the release team lead should look for volunteers who:

- Can commit to the amount of time required across the release cycle
- Are enthusiastic about being on the release team
- Are members of the Cluster Lifecycle SIG
- Have some prior experience with contributing to CAPI releases (such as shadowing a role for a prior release)
- Have diverse company affiliations (i.e. not all from the same company)

## Time Commitment

As a member of the release team, you should expect to spend approximately 4-8 hours a week on release related activities for the duration of the term. 

Before you volunteer to be part of a CAPI release team, please make certain that your employer is aware and supportive of your commitment to the release team.

## Why should I volunteer?

Volunteering to be part of a CAPI release team is a great way to contribute to the community and to the release process:

- Get more familiar with the CAPI release process
- Create lasting relationships with other members of the community
- Contribute to the CAPI project health