---
name: 🚋 Release cycle tracking
about: Create a new release cycle tracking issue for a Cluster API minor release
about: "[Only for release team lead] Create an issue to track tasks for a Cluster API minor release."
title: Tasks for v<release-tag> release cycle
labels: ''
assignees: ''

---

Please see the corresponding section in [release-tasks.md](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md) for documentation of individual tasks.  

## Tasks

**Notes**:
* Weeks are only specified to give some orientation.
* The following is based on the v1.6 release cycle. Modify according to the tracked release cycle.

Week 1:
* [ ] [Release Lead] [Finalize release schedule and team](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#finalize-release-schedule-and-team)
* [ ] [Release Lead] [Add/remove release team members](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#addremove-release-team-members)
* [ ] [Release Lead] [Prepare main branch for development of the new release](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#prepare-main-branch-for-development-of-the-new-release)
* [ ] [Communications Manager] [Add docs to collect release notes for users and migration notes for provider implementers](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#add-docs-to-collect-release-notes-for-users-and-migration-notes-for-provider-implementers)
* [ ] [Communications Manager] [Update supported versions](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#update-supported-versions)

Week 1 to 4:
* [ ] [Release Lead] [Track] [Remove previously deprecated code](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#track-remove-previously-deprecated-code)

Week 6:
* [ ] [Release Lead] [Cut the v1.5.1 & v1.4.6 releases](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#repeatedly-cut-a-release)

Week 9:
* [ ] [Release Lead] [Cut the v1.5.2 & v1.4.7 releases](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#repeatedly-cut-a-release)

Week 11 to 12:
* [ ] [Release Lead] [Track] [Bump dependencies](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#track-bump-dependencies)

Week 13:
* [ ] [Release Lead] [Cut the v1.6.0-beta.0 release](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#repeatedly-cut-a-release)
* [ ] [Release Lead] [Cut the v1.5.3 & v1.4.8 releases](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#repeatedly-cut-a-release)
* [ ] [Release Lead] [Create a new GitHub milestone for the next release](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#create-a-new-github-milestone-for-the-next-release)
* [ ] [Communications Manager] [Communicate beta to providers](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#communicate-beta-to-providers)

Week 14:
* [ ] [Release Lead] [Cut the v1.6.0-beta.1 release](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#repeatedly-cut-a-release)
* [ ] [Release Lead] [Set a tentative release date for the next minor release](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#set-a-tentative-release-date-for-the-next-minor-release)
* [ ] [Release Lead] [Assemble next release team](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#assemble-next-release-team)
* [ ] [Release Lead] Select release lead for the next release cycle

Week 15:

* KubeCon idle week

Week 16:
* [ ] [Release Lead] [Cut the v1.6.0-rc.0 release](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#repeatedly-cut-a-release)
* [ ] [Release Lead] [Update milestone applier and GitHub Actions](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#update-milestone-applier-and-github-actions)
* [ ] [CI Manager] [Setup jobs and dashboards for the release-1.6 release branch](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#setup-jobs-and-dashboards-for-a-new-release-branch)
* [ ] [Communications Manager] [Ensure the book for the new release is available](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#ensure-the-book-for-the-new-release-is-available)

Week 17:
* [ ] [Release Lead] [Cut the v1.6.0-rc.1 release](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#repeatedly-cut-a-release)

Week 18:
* [ ] [Release Lead] [Cut the v1.6.0 release](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#repeatedly-cut-a-release)
* [ ] [Release Lead] [Cut the v1.5.4 & v1.4.9 releases](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#repeatedly-cut-a-release)
* [ ] [Release Lead] Organize release retrospective
* [ ] [Communications Manager] [Change production branch in Netlify to the new release branch](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#change-production-branch-in-netlify-to-the-new-release-branch)
* [ ] [Communications Manager] [Update clusterctl links in the quickstart](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#update-clusterctl-links-in-the-quickstart)

Continuously:
* [Release lead] [Maintain the GitHub release milestone](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#continuously-maintain-the-github-release-milestone)
* [Release lead] [Bump the Go version](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#continuously-bump-the-go-version)
* [Communications Manager] [Communicate key dates to the community](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#continuously-communicate-key-dates-to-the-community)
* [Communications Manager] Improve release process documentation
* [Communications Manager] Maintain and improve user facing documentation about releases, release policy and release calendar
* [CI Manager] [Monitor CI signal](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#continuously-monitor-ci-signal)
* [CI Manager] [Reduce the amount of flaky tests](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#continuously-reduce-the-amount-of-flaky-tests)
* [CI Manager] [Bug triage](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#continuously-bug-triage)
* [CI Manager] Maintain and improve release automation, tooling & related developer docs

If and when necessary:
* [ ] [Release Lead] [Track] [Bump the Cluster API apiVersion](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#optional-track-bump-the-cluster-api-apiversion)
* [ ] [Release Lead] [Track] [Bump the Kubernetes version](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#optional-track-bump-the-kubernetes-version)
* [ ] [Release Lead] [Track Release and Improvement tasks](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/release/release-tasks.md#optional-track-release-and-improvement-tasks)