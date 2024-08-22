# CI Signal

## Overview

* If a task is prefixed with `[Track]` it means it should be ensured that this task is done, but the folks with the corresponding role are not responsible to do it themselves.

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Responsibilities](#responsibilities)
- [Tasks](#tasks)
  - [Setup jobs and dashboards for a new release branch](#setup-jobs-and-dashboards-for-a-new-release-branch)
  - [[Continuously] Monitor CI signal](#continuously-monitor-ci-signal)
  - [[Continuously] Reduce the amount of flaky tests](#continuously-reduce-the-amount-of-flaky-tests)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Responsibilities

* Signal:
  * Responsibility for the quality of the release
  * Continuously monitor CI signal, so a release can be cut at any time
  * Add CI signal for new release branches

## Tasks

### Setup jobs and dashboards for a new release branch

The goal of this task is to have test coverage for the new release branch and results in testgrid.

This task is performed after the new release branch is cut [by the release workflow](https://github.com/kubernetes-sigs/cluster-api/blob/defa62d5340f4b49f1acab80cc8cc10727b85291/.github/workflows/release.yaml#L61-L63) during the final weeks of the release cycle.

While we add test coverage for the new release branch we will also drop the tests for old release branches if necessary.  Examples to follow assume the new release branch is `release-1.8`

1. Create new jobs based on the jobs running against our `main` branch:
    1. Copy the `main` branch entry as `release-1.8` in the `cluster-api-prowjob-gen.yaml` file in [test-infra](https://github.com/kubernetes/test-infra/blob/master/config/jobs/kubernetes-sigs/cluster-api/).
    2. Modify the following at the `release-1.8` branch entry:
            * Change intervals (let's use the same as for `release-1.7`).
2. Create a new dashboard for the new branch in: `test-infra/config/testgrids/kubernetes/sig-cluster-lifecycle/config.yaml` (`dashboard_groups` and `dashboards`).
3. Remove old release branches and unused versions from the `cluster-api-prowjob-gen.yaml` file in [test-infra](https://github.com/kubernetes/test-infra/blob/master/config/jobs/kubernetes-sigs/cluster-api/) according to our policy documented in [Support and guarantees](../../../../CONTRIBUTING.md#support-and-guarantees). As we just added `release-1.8`, then we can now drop test coverage for the `release-1.5` branch.
4. Regenerate the prowjob configuration running `make generate-test-infra-prowjobs` command from cluster-api repository. Before running this command, ensure to export the `TEST_INFRA_DIR` variable, specifying the location of the [test-infra](https://github.com/kubernetes/test-infra/) repository in your environment. For further information, refer to this [link](https://github.com/kubernetes-sigs/cluster-api/pull/9937).

 ```sh
  TEST_INFRA_DIR=../../k8s.io/test-infra make generate-test-infra-prowjobs
  ```
5. Verify the jobs and dashboards a day later by taking a look at: `https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api-1.8`
6. Update the [PR markdown link checker](https://github.com/kubernetes-sigs/cluster-api/blob/main/.github/workflows/pr-md-link-check.yaml) accordingly (e.g. `main` -> `release-1.8`).

Prior art:

* [Add jobs for CAPI release 1.8](https://github.com/kubernetes/test-infra/pull/33156)

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
7. Run periodic deep-dive sessions with the CI team to investigate failing and flaking tests. Example session recording: https://www.youtube.com/watch?v=YApWftmiDTg

  **Note**: Maintaining the health of the project is a community effort.  CI team should use all of the tools available to them to attempt to keep the CI signal clean, however the [#cluster-api](https://kubernetes.slack.com/archives/C8TSNPY4T) Slack channel should be used to increase visibility of release blocking interruptions to the CI signal and seek help from community.  This should be *additive* to the steps described above. When in doubt, err on the side of overcommunication to promote awareness and drive disruptions to resolution.  

### [Continuously] Reduce the amount of flaky tests

The Cluster API tests are pretty stable, but there are still some flaky tests from time to time.

To reduce the amount of flakes please periodically:

1. Take a look at recent CI failures via `k8s-triage`:
    * [main: e2e, e2e-mink8s, test, test-mink8s](https://storage.googleapis.com/k8s-triage/index.html?job=.*cluster-api.*(test%7Ce2e)-(mink8s-)*main&xjob=.*-provider-.*)
2. Open issues using an appropriate template (flaking-test) for occurring flakes and ideally fix them or find someone who can.

   **Note**: Given resource limitations in the Prow cluster it might not be possible to fix all flakes. Let's just try to pragmatically keep the amount of flakes pretty low.
