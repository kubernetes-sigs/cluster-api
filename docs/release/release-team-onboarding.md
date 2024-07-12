<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Onboarding Notes for Release Team](#onboarding-notes-for-release-team)
  - [Overview](#overview)
    - [Release Lead Team](#release-lead-team)
    - [Comms Team](#comms-team)
    - [CI Signal Team](#ci-signal-team)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

# Onboarding Notes for Release Team

## Overview

Welcome to the Cluster API Release Team onboarding documentation!

As a member of the Release Team, you play a crucial role in helping to ensure project releases
are delivered smoothly and in a timely manner. While there are specific responsibilities for each sub-team within
the Release Team, below are some general notes that every member of the Release Team might benefit by going 
through at the beginning of the cycle:

- Slack:
    - Most discussions related CAPI Release topics happens in the #cluster-api channel on the Kubernetes Slack. If you need access to the Kubernetes Slack, please visit http://slack.k8s.io/.
- Kubernetes SIG membership:
    -  Try to become an official member of the Kubernetes SIG, if possible. More information on the membership and requirements can be found [here](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-team.md#cluster-api-release-team-vs-kuberneteskubernetes-sig-membership).
- Familiarize yourself with the Release Process:
    - Review the release [team roles](../release/release-team.md#team-roles) which explains the responsibilities and tasks for each role within the release team.
- Check the Release Timeline:
    - Go through the [release timeline](../release/releases) of the release cycle you are involved in (i.e checkout `release-1.6.md` if you are part of the 1.6 cycle release team) to better understand the key milestones and deadlines.

Now, let's dive into the specific onboarding notes for each sub-team below.

### Release Lead Team

- Explore CI Setup and Tools:
    - Gain some insights into how CI is setup in the Cluster API and tools used. You can check out CI Team onboarding [notes](#ci-signal-team) for more details.
- Learn about Release Notes Generation:
    - Get to know how the release notes generation tools works.
- Review the onboarding notes for both the Comms and CI Lead teams:
    - Check out the onboarding notes of [Comms](#comms-team) and [CI Signal](#ci-signal-team) teams.

### Comms Team

- Understand Release Process: 
    - Get to know how project's release process works.
    - Walk through the [release note generation process](../release/role-handbooks/communications/README.md#create-pr-for-release-notes) and try to generate notes by yourself. This is the most important process the comms team is in charge of.
    - Familiarize yourself with the release notes tool [code](https://github.com/kubernetes-sigs/cluster-api/tree/main/hack/tools/release). You'll probably need to update this code during the release cycle to cover new cases or add new features.
- Documentation familiarity:
    - Explore project's documentation and start learning how to update and maintain it.
- Communication channels:
    - Learn about project's primary communication channels, including Slack, mailing list and office hours. Try to understand how information is distributed and how to use them effectively for announcements, updates and discussions related to the release cycle.

### CI Signal Team

- Start by gaining a general understanding of GitHub labels and how to find issues and pull requests for the current milestone.
- Familiarize yourself with Prow commands: The Cluster API project utilizes [Prow](https://docs.prow.k8s.io/docs/overview/) to manage CI automation. Issues and PRs are categorized by applying specific list of area labels, which helps in prioritization of that specific issue/PR during the release cycle or release process. Learn more about the available [labels](https://github.com/kubernetes/test-infra/blob/master/label_sync/labels.md#labels-that-apply-to-kubernetes-sigscluster-api-for-both-issues-and-prs) and prow [commands](https://prow.k8s.io/command-help).
- Take a look at [TestGrid](https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#Summary), an interactive dashboard for visualizing the CI job results of the project in a grid format!
- Examine the [CI jobs](https://github.com/kubernetes/test-infra/tree/master/config/jobs/kubernetes-sigs/cluster-api) in the test-infra repository. These jobs are defined in YAML and represent various job types, such as periodics and presubmits, that we run in the project. You can also find dedicated [notes](https://cluster-api.sigs.k8s.io/reference/jobs) for them in the book.
- Explore [k8s-triage](https://storage.googleapis.com/k8s-triage/index.html?job=periodic-cluster-api-*), a tool that identifies groups of similar test failures across all jobs.
- Experiment with running [end-to-end tests](https://cluster-api.sigs.k8s.io/developer/testing#running-the-end-to-end-tests-locally) on your local machine to gain a better understanding of the tests and build a confidence debugging CI issues.