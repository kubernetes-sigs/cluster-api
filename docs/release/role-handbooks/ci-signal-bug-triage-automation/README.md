# Onboarding Notes

Welcome to the Cluster API CI Signal Team onboarding documentation!

## Overview

- Start by gaining a general understanding of GitHub labels and how to find issues and pull requests for the current milestone.
- Familiarize yourself with Prow commands: The Cluster API project utilizes [Prow](https://docs.prow.k8s.io/docs/overview/) to manage CI automation. Issues and PRs are categorized by applying specific list of area labels, which helps in prioritization of that specific issue/PR during the release cycle or release process. Learn more about the available [labels](https://github.com/kubernetes/test-infra/blob/master/label_sync/labels.md#labels-that-apply-to-kubernetes-sigscluster-api-for-both-issues-and-prs) and prow [commands](https://prow.k8s.io/command-help).
- Take a look at [TestGrid](https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#Summary), an interactive dashboard for visualizing the CI job results of the project in a grid format!
- Examine the [CI jobs](https://github.com/kubernetes/test-infra/tree/master/config/jobs/kubernetes-sigs/cluster-api) in the test-infra repository. These jobs are defined in YAML and represent various job types, such as periodics and presubmits, that we run in the project. You can also find dedicated [notes](https://cluster-api.sigs.k8s.io/reference/jobs) for them in the book.
- Explore [k8s-triage](https://storage.googleapis.com/k8s-triage/index.html?job=periodic-cluster-api-*), a tool that identifies groups of similar test failures across all jobs.
- Experiment with running [end-to-end tests](https://cluster-api.sigs.k8s.io/developer/testing#running-the-end-to-end-tests-locally) on your local machine to gain a better understanding of the tests and build a confidence debugging CI issues.

## Release Tasks

This document details the responsibilities and tasks for CI Signal team in the release process.

  - [Responsibilities](#responsibilities)
  - [Tasks](#tasks)
    - [Setup jobs and dashboards for a new release branch](#setup-jobs-and-dashboards-for-a-new-release-branch)
    - [[Continuously] Monitor CI signal](#continuously-monitor-ci-signal)
    - [[Continuously] Reduce the amount of flaky tests](#continuously-reduce-the-amount-of-flaky-tests)
    - [[Continuously] Bug triage](#continuously-bug-triage)

❗Notes:

- The examples in this document are based on the v1.6 release cycle.
- This document focuses on tasks that are done for every release. One-time improvement tasks are out of scope.
- If a task is prefixed with [Track] it means it should be ensured that this task is done, but the folks with the corresponding role are not responsible to do it themselves.


## Responsibilities

- Signal:
  - Responsibility for the quality of the release
  - Continuously monitor CI signal, so a release can be cut at any time
  - Add CI signal for new release branches
- Bug Triage:
  - Make sure blocking issues and bugs are triaged and dealt with in a timely fashion
- Automation:
  - Maintain and improve release automation, tooling & related developer docs

## Tasks

### Setup jobs and dashboards for a new release branch

The goal of this task is to have test coverage for the new release branch and results in testgrid.
While we add test coverage for the new release branch we will also drop the tests for old release branches if necessary.

1. Create new jobs based on the jobs running against our `main` branch:
    1. Copy `test-infra/config/jobs/kubernetes-sigs/cluster-api/cluster-api-periodics-main.yaml` to `config/jobs/kubernetes-sigs/cluster-api/cluster-api-periodics-release-1-6.yaml`.
    2. Copy `test-infra/config/jobs/kubernetes-sigs/cluster-api/cluster-api-periodics-main-upgrades.yaml` to `test-infra/config/jobs/kubernetes-sigs/cluster-api/cluster-api-periodics-release-1-6-upgrades.yaml`.
    3. Copy `test-infra/config/jobs/kubernetes-sigs/cluster-api/cluster-api-presubmits-main.yaml` to `test-infra/config/jobs/kubernetes-sigs/cluster-api/cluster-api-presubmits-release-1-6.yaml`.
    4. Modify the following:
        1. Rename the jobs, e.g.: `periodic-cluster-api-test-main` => `periodic-cluster-api-test-release-1-6`.
        2. Change `annotations.testgrid-dashboards` to `sig-cluster-lifecycle-cluster-api-1.6`.
        3. Change `annotations.testgrid-tab-name`, e.g. `capi-test-main` => `capi-test-release-1-6`.
        4. For periodics additionally:
            - Change `extra_refs[].base_ref` to `release-1.6` (for repo: `cluster-api`).
            - Change interval (let's use the same as for `1.5`).
        5. For presubmits additionally: Adjust branches: `^main$` => `^release-1.6$`.
2. Create a new dashboard for the new branch in: `test-infra/config/testgrids/kubernetes/sig-cluster-lifecycle/config.yaml` (`dashboard_groups` and `dashboards`).
3. Remove tests from the [test-infra](https://github.com/kubernetes/test-infra) repository for old release branches according to our policy documented in [Support and guarantees](../../../../CONTRIBUTING.md#support-and-guarantees). For example, let's assume we just created tests for v1.6, then we can now drop test coverage for the release-1.3 branch.
4. Verify the jobs and dashboards a day later by taking a look at: `https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api-1.6`
5. Update `.github/workflows/weekly-security-scan.yaml` - to setup Trivy and govulncheck scanning - `.github/workflows/weekly-md-link-check.yaml` - to setup link checking in the CAPI book - and `.github/workflows/weekly-test-release.yaml` - to verify the release target is working - for the currently supported branches.
6. Update the [PR markdown link checker](https://github.com/kubernetes-sigs/cluster-api/blob/main/.github/workflows/pr-md-link-check.yaml) accordingly (e.g. `main` -> `release-1.6`).
   <br>Prior art: [Update branch for link checker](https://github.com/kubernetes-sigs/cluster-api/pull/9206)

Prior art:

- [Add jobs for CAPI release 1.6](https://github.com/kubernetes/test-infra/pull/31208)
- [Update github workflows](https://github.com/kubernetes-sigs/cluster-api/pull/8398)

### [Continuously] Monitor CI signal

The goal of this task is to keep our tests running in CI stable.

**Note**: To be very clear, this is not meant to be an on-call role for Cluster API tests.

1. Add yourself to the [Cluster API alert mailing list](https://github.com/kubernetes/k8s.io/blob/151899b2de933e58a4dfd1bfc2c133ce5a8bbe22/groups/sig-cluster-lifecycle/groups.yaml#L20-L35)
    <br\>**Note**: An alternative to the alert mailing list is manually monitoring the [testgrid dashboards](https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api)
    (also dashboards of previous releases). Using the alert mailing list has proven to be a lot less effort though.
2. Subscribe to `CI Activity` notifications for the Cluster API repo.
3. Check the existing **failing-test** and **flaking-test** issue templates under `.github/ISSUE_TEMPLATE/` folder of the repo, used to create an issue for failing or flaking tests respectively. Please make sure they are up-to-date and if not, send a PR to update or improve them.
4. Check if there are any existing jobs that got stuck (have been running for more than 12 hours) in a ['pending'](https://prow.k8s.io/?repo=kubernetes-sigs%2Fcluster-api&state=pending) state:
   - If that is the case, notify the maintainers and ask them to manually cancel and re-run the stuck jobs.
5. Triage CI failures reported by mail alerts or found by monitoring the testgrid dashboards:
    1. Create an issue using an appropriate template (failing-test) in the Cluster API repository to surface the CI failure.
    2. Identify if the issue is a known issue, new issue or a regression.
    3. Mark the issue as `release-blocking` if applicable.
6. Triage periodic GitHub actions failures, with special attention to image scan results;
   Eventually open issues as described above.
7. Run periodic deep-dive sessions with the CI team to investigate failing and flaking tests. Example session recording: <https://www.youtube.com/watch?v=YApWftmiDTg>

### [Continuously] Reduce the amount of flaky tests

The Cluster API tests are pretty stable, but there are still some flaky tests from time to time.

To reduce the amount of flakes please periodically:

1. Take a look at recent CI failures via `k8s-triage`:
    - [main: e2e, e2e-mink8s, test, test-mink8s](https://storage.googleapis.com/k8s-triage/index.html?job=.*cluster-api.*(test%7Ce2e)-(mink8s-)*main&xjob=.*-provider-.*)
2. Open issues using an appropriate template (flaking-test) for occurring flakes and ideally fix them or find someone who can.
   **Note**: Given resource limitations in the Prow cluster it might not be possible to fix all flakes.
   Let's just try to pragmatically keep the amount of flakes pretty low.

### [Continuously] Bug triage

The goal of bug triage is to triage incoming issues and if necessary flag them with `release-blocking`
and add them to the milestone of the current release.

We probably have to figure out some details about the overlap between the bug triage task here, release leads
and Cluster API maintainers.
