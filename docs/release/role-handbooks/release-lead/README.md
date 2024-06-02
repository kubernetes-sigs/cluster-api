# Release Team Lead

## Overview

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Responsibilities](#responsibilities)
- [Tasks](#tasks)
  - [Finalize release schedule and team](#finalize-release-schedule-and-team)
  - [Add/remove release team members](#addremove-release-team-members)
  - [Prepare main branch for development of the new release](#prepare-main-branch-for-development-of-the-new-release)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Responsibilities

* Coordination:
  * Take ultimate accountability for all release tasks to be completed on time
  * Coordinate release activities
  * Create and maintain the GitHub release milestone
  * Track tasks needed to add support for new Kubernetes versions in upcoming releases
  * Ensure a retrospective happens
  * Ensure one of the [maintainers](https://github.com/kubernetes-sigs/cluster-api/blob/main/OWNERS_ALIASES) is available when a release needs to be cut.
* Staffing:
  * Assemble the release team for the next release cycle
  * Ensure a release lead for the next release cycle is selected and trained
  * Set a tentative release date for the next release cycle
* Cutting releases:
  * Release patch releases for supported previous releases at least monthly or more often if needed
  * Create beta, RC and GA releases for the minor release of the current release cycle
* Release lead should keep an eye on what is going on in the project to be able to react if necessary

## Tasks

### Finalize release schedule and team

1. Finalize release schedule and team in the [docs/release/releases](../../releases), e.g. [release-1.6.md](../../releases/release-1.6.md).
2. Update @cluster-api-release-team Slack user group and GitHub team accordingly.
   <br>Prior art `org`: https://github.com/kubernetes/org/pull/4353
   <br>Prior art `community`: https://github.com/kubernetes/community/pull/7423
3. Update @cluster-api-release-lead and @cluster-api-release-team aliases in root OWNERS_ALIASES file with Release Team members.
   <br>Prior art: https://github.com/kubernetes-sigs/cluster-api/pull/9111/files#diff-4985b733677adf9dda6b5187397d4700868248ef646d64aecfb66c1ced575499
4. Announce the _release team_ and _release schedule_ to the mailing list.

### Add/remove release team members

If necessary, the release lead can adjust the release team during the cycle to handle unexpected changes in staffing due to personal/professional issues, no-shows, or unplanned work spikes. Adding/removing members can be done by opening a PR to update the release team members list for the release cycle in question.

### Prepare main branch for development of the new release

The goal of this issue is to bump the versions on the main branch so that the upcoming release version
is used for e.g. local development and e2e tests. We also modify tests so that they are testing the previous release.

This comes down to changing occurrences of the old version to the new version, e.g. `v1.5` to `v1.6`:

1. Setup E2E tests for the new release:
   1. Goal is that we have clusterctl upgrade tests for the latest stable versions of each contract / for each supported branch. For `v1.6` this means:
      * v1beta1: `v1.0`, `v1.4`, `v1.5` (will change with each new release)
   2. Update providers in `docker.yaml`:
       1. Add a new `v1.6.0` entry.
       2. Remove providers that are not used anymore in clusterctl upgrade tests.
       3. Change `v1.5.99` to `v1.6.99`.
   3. Adjust `metadata.yaml`'s:
      1. Create a new `v1.6` `metadata.yaml` (`test/e2e/data/shared/v1.6/metadata.yaml`) by copying
   `test/e2e/data/shared/main/metadata.yaml`
      2. Add the new release to the main `metadata.yaml` (`test/e2e/data/shared/main/metadata.yaml`).
      3. Add the new release to the root level `metadata.yaml`
      4. Remove old `metadata.yaml`'s that are not used anymore in clusterctl upgrade tests.
   4. Adjust cluster templates in `test/e2e/data/infrastructure-docker`:
      1. Create a new `v1.6` folder. It should be created based on the `main` folder and only contain the templates we use in the clusterctl upgrade tests (as of today `cluster-template` and `cluster-template-topology`).
      2. Remove old folders that are not used anymore in clusterctl upgrade tests.
   5. Modify the test specs in `test/e2e/clusterctl_upgrade_test.go` (according to the versions we want to test described above).
      Please note that both `InitWithKubernetesVersion` and `WorkloadKubernetesVersion` should be the highest mgmt cluster version supported by the respective Cluster API version.
2. Update `create-local-repository.py` and `tools/internal/tilt-prepare/main.go`: `v1.5.99` => `v1.6.99`.
3. Make sure all tests are green (also run `pull-cluster-api-e2e-full-main` and `pull-cluster-api-e2e-workload-upgrade-1-27-latest-main`).
4. Remove an unsupported release version of Cluster API from the Makefile target that generates e2e templates. For example, remove `v1.3` while working on `v1.6`.

Prior art: 

* 1.5 - https://github.com/kubernetes-sigs/cluster-api/pull/8430/files
* 1.6 - https://github.com/kubernetes-sigs/cluster-api/pull/9097/files
* 1.7 - https://github.com/kubernetes-sigs/cluster-api/pull/9799/files