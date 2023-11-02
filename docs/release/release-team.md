<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Cluster API Release Team](#cluster-api-release-team)
  - [Overview](#overview)
    - [Goals](#goals)
    - [Non-Goals/Future work](#non-goalsfuture-work)
  - [Duration of Term](#duration-of-term)
  - [Specific Responsibilities](#specific-responsibilities)
  - [Team Roles](#team-roles)
  - [Team repo permissions](#team-repo-permissions)
  - [Team Selection](#team-selection)
    - [Selection Criteria](#selection-criteria)
  - [Time Commitment](#time-commitment)
  - [Suggestions for Team Leads](#suggestions-for-team-leads)
  - [Why should I volunteer?](#why-should-i-volunteer)
  - [Cluster API release team vs kubernetes/kubernetes-SIG membership](#cluster-api-release-team-vs-kuberneteskubernetes-sig-membership)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

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

More details on the CAPI release process can be found in the [release cycle](./release-cycle.md) and [release task](./release-tasks.md) documentation.

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
- **Team member**: Any Release Team lead or manager may select one or more additional members to help with their tasks. These team members will help fulfill future Release Team staffing requirements and continue to grow the CAPI community in general.
- **Maintainer**: Responsible for tasks which require write access to the Cluster API repo including creating release tags and creating a release branch. This role must be filled by someone on the [`cluster-api-maintainers` list](https://github.com/kubernetes-sigs/cluster-api/blob/main/OWNERS_ALIASES).
*Note*: This is also documented in [Release tasks](./release-tasks.md) together with a mapping to specific tasks.  

## Team repo permissions
- Release notes (`CHANGELOG` folder)
  - The Release Lead has approval permissions, which allows them to merge PRs that add new release notes. This will start an automated release process through GitHub Actions: creating tags, create GitHub Release draft, etc.
  - All members of the release team have `lgtm` permissions for PRs that add release notes in this folder.
- Release notes tool (`hack/tools/release` folder)
  - The Release Lead has approval permissions, which allows them to merge code changes to this tool. It's not their responsibility to always review the code changes (although they can), but to make sure the right folks have `lgtm`ed the PR.
  - All members of the release team have `lgtm` permissions for the release notes tool code.

## Team Selection

To start, the release team will be assembled by the release team lead based on volunteers. A call for volunteers can be made through the usual communication channels (office hours, Slack, mailing list, etc.). In the future, we may consider introducing an application process similar to the Kubernetes release team application process. 

### Selection Criteria

When assembling a release team, the release team lead should look for volunteers who:

- Can commit to the amount of time required across the release cycle
- Are enthusiastic about being on the release team
- Preferably are [members of the kubernetes or the kubernetes-SIG org](https://github.com/kubernetes/community/blob/master/community-membership.md) (see [notes](#cluster-api-release-team-vs-kuberneteskubernetes-sig-membership)).
- Have some prior experience with contributing to CAPI releases (such having been in the release team for a prior release)
- Have diverse company affiliations (i.e. not all from the same company)
- Are members of the Kubernetes slack community (register if you are not!)
- Are members of the Cluster Lifecycle SIG mailing list (subscribe to the [SIG Cluster Lifecycle Google Group](https://groups.google.com/forum/#!forum/kubernetes-sig-cluster-lifecycle) if you are not!)

## Time Commitment

As a member of the release team, you should expect to spend approximately 4-8 hours a week on release related activities for the duration of the term.

Specific time commitments include:
   * Release Team meetings once a week throughout the entire release cycle.
   * Release Day meetings ideally occurring once during the actual release weeks. Refer to release cycle timeline for more specific details.
   * Any other release-related critical meetings with prior notice.

While we don't anticipate individuals to be available every week during the release cycle, please feel free to inform the team of any unavailability so we can plan accordingly.

Before you volunteer to be part of a CAPI release team, please make certain that your employer is aware and supportive of your commitment to the release team.

## Suggestions for Team Leads

  * In the first week of the release cycle, organize an onboarding session with members of your team (i.e CI Lead with CI team members) to go over the general responsibilities and expectations.
  * Clearly communicate with the team members you are responsible for, that the majority of the work during the release cycle will be a collaborative effort.
  * Establish an ownership rotation policy in consultation with respective team members.
  * Provide opportunities for team members to take the lead in cutting a release within the cycle, based on feasibility.
  * Define backup ownership and tasks for team members, such as:
      * Hosting release team meetings.
      * Communicating key updates during office hours meetings when necessary.
      * Sharing release-related updates with the CAPI community.
      * Monitoring and reporting CI status regularly to the release team.
      * Scheduling additional meetings as required to facilitate a smooth release cycle.

## Why should I volunteer?

Volunteering to be part of a CAPI release team is a great way to contribute to the community and to the release process:

- Get more familiar with the CAPI release process
- Create lasting relationships with other members of the community
- Contribute to the CAPI project health

## Cluster API release team vs kubernetes/kubernetes-SIG membership

Candidates for the Cluster API release team should preferably be [members of the kubernetes or the kubernetes-SIG org](https://github.com/kubernetes/community/blob/master/community-membership.md), but this is not a hard requirement.

Non-org members can perform all the tasks of the release team, but they can't be added to the [cluster-api-release-team](https://github.com/kubernetes/org/blob/99343225f3ce39c2d3da594b7aca40ca8043bd54/config/kubernetes-sigs/sig-cluster-lifecycle/teams.yaml#L341) GitHub group.

Being part of the Cluster API release team could be a great start for people willing to become a members of the kubernetes-SIG org, see
the official list of [requirements](https://github.com/kubernetes/community/blob/master/community-membership.md#requirements).
