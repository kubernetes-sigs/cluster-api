# Release Tasks

This document details the responsibilities and tasks for each role in the release team.

**Notes**:
* The examples in this document are based on the v1.4 release cycle.
* This document focuses on tasks that are done for every release. One-time improvement tasks are out of scope.
* If a task is prefixed with `[Track]` it means it should be ensured that this task is done, but the folks with
  the corresponding role are not responsible to do it themselves.

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**

- [Release Tasks](#release-tasks)
  - [Release Lead](#release-lead)
    - [Responsibilities](#responsibilities)
    - [Tasks](#tasks)
      - [Set a tentative release date for the minor release](#set-a-tentative-release-date-for-the-minor-release)
      - [Assemble release team](#assemble-release-team)
      - [Finalize release schedule and team](#finalize-release-schedule-and-team)
      - [Prepare main branch for development of the new release](#prepare-main-branch-for-development-of-the-new-release)
      - [Create a new GitHub milestone for the next release](#create-a-new-github-milestone-for-the-next-release)
      - [\[Track\] Remove previously deprecated code](#track-remove-previously-deprecated-code)
      - [\[Track\] Bump dependencies](#track-bump-dependencies)
      - [Create a release branch](#create-a-release-branch)
      - [\[Continuously\] Maintain the GitHub release milestone](#continuously-maintain-the-github-release-milestone)
      - [\[Continuously\] Bump the Go version](#continuously-bump-the-go-version)
      - [\[Repeatedly\] Cut a release](#repeatedly-cut-a-release)
      - [\[Optional\] \[Track\] Bump the Cluster API apiVersion](#optional-track-bump-the-cluster-api-apiversion)
      - [\[Optional\] \[Track\] Bump the Kubernetes version](#optional-track-bump-the-kubernetes-version)
  - [Communications/Docs/Release Notes Manager](#communicationsdocsrelease-notes-manager)
    - [Responsibilities](#responsibilities-1)
    - [Tasks](#tasks-1)
      - [Add docs to collect release notes for users and migration notes for provider implementers](#add-docs-to-collect-release-notes-for-users-and-migration-notes-for-provider-implementers)
      - [Update supported versions](#update-supported-versions)
      - [Ensure the book for the new release is available](#ensure-the-book-for-the-new-release-is-available)
      - [Polish release notes](#polish-release-notes)
      - [Change production branch in Netlify to the new release branch](#change-production-branch-in-netlify-to-the-new-release-branch)
      - [Update clusterctl links in the quickstart](#update-clusterctl-links-in-the-quickstart)
      - [Continuously: Communicate key dates to the community](#continuously-communicate-key-dates-to-the-community)
  - [CI Signal/Bug Triage/Automation Manager](#ci-signalbug-triageautomation-manager)
    - [Responsibilities](#responsibilities-2)
    - [Tasks](#tasks-2)
      - [Setup jobs and dashboards for a new release branch](#setup-jobs-and-dashboards-for-a-new-release-branch)
      - [\[Continuously\] Monitor CI signal](#continuously-monitor-ci-signal)
      - [\[Continuously\] Reduce the amount of flaky tests](#continuously-reduce-the-amount-of-flaky-tests)
      - [\[Continuously\] Bug triage](#continuously-bug-triage)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Release Lead

### Responsibilities

* Coordination:
    * Take ultimate accountability for all release tasks to be completed on time
    * Coordinate release activities
    * Create and maintain the GitHub release milestone
    * Track tasks needed to add support for new Kubernetes versions in upcoming releases
    * Ensure a retrospective happens
* Staffing:
    * Assemble the release team for the current release cycle
    * Ensure a release lead for the next release cycle is selected and trained
    * Set a tentative release date for the next release cycle
* Cutting releases:
    * Release patch releases for supported previous releases at least monthly or more often if needed
    * Create beta, RC and GA releases for the minor release of the current release cycle
* Release lead should keep an eye on what is going on in the project to be able to react if necessary

### Tasks

#### Set a tentative release date for the minor release

1. Set a tentative release date for the release and document it by creating a `release-1.4.md`.

#### Assemble release team

There is currently no formalized process to assemble the release team.
As of now we ask for volunteers in Slack and office hours.

#### Finalize release schedule and team

1. Finalize release schedule and team in `release-1.4.md`.
2. Update Slack user group and GitHub team accordingly.
   <br>Prior art: https://github.com/kubernetes-sigs/cluster-api/issues/7476

#### Prepare main branch for development of the new release

The goal of this issue is to bump the versions on the main branch so that the upcoming release version
is used for e.g. local development and e2e tests. We also modify tests so that they are testing the previous release.

This comes down to changing occurrences of the old version to the new version, e.g. `v1.3` to `v1.4`:
1. Setup E2E tests for the new release:
   1. Goal is that we have clusterctl upgrade tests for the latest stable versions of each contract / for each supported branch. For `v1.4` this means:
      * v1alpha3: `v0.3`
      * v1alpha4: `v0.4`
      * v1beta1: `v1.0`, `v1.2`, `v1.3` (will change with each new release)
   2. Update providers in `docker.yaml`:
       1. Add a new `v1.3.0` entry.
       2. Remove providers that are not used anymore (for `v1.4` we don't have to remove any).
       3. Change `v1.3.99` to `v1.4.99`.
   3. Adjust `metadata.yaml`'s:
      1. Create a new `v1.3` `metadata.yaml` (`test/e2e/data/shared/v1.3/metadata.yaml`) by copying
   `test/e2e/data/shared/main/metadata.yaml`
      2. Add the new release to the main `metadata.yaml` (`test/e2e/data/shared/main/metadata.yaml`).
      3. Remove old `metadata.yaml`'s that are not used anymore (for `v1.4` we don't have to remove any).
   4. Adjust cluster templates in `test/e2e/data/infrastructure-docker`:
      1. Create a new `v1.3` folder. It should be created based on the `main` folder and only contain the templates
         we use in the clusterctl upgrade tests (as of today `cluster-template` and `cluster-template-topology`).
      2. Remove old folders that are not used anymore (for `v1.4` we don't have to remove any).
   5. Modify the test specs in `test/e2e/clusterctl_upgrade_test.go` (according to the versions we want to test described above).
      Please note that `InitWithKubernetesVersion` should be the highest mgmt cluster version supported by the respective Cluster API version.
2. Update `create-local-repository.py` and `tools/tilt-prepare/main.go`: `v1.3.99` => `v1.4.99`.
3. Update `.github/workflows/scan.yml` - to setup Trivy scanning - and `.github/workflows/lint-docs-weekly.yml` - to setup link checking in the CAPI book - for the currently supported branches.
4. Make sure all tests are green (also run `pull-cluster-api-e2e-full-main` and `pull-cluster-api-e2e-workload-upgrade-1-23-latest-main`).

Prior art: https://github.com/kubernetes-sigs/cluster-api/pull/6834/files

#### Create a new GitHub milestone for the next release

The goal of this task is to create a new GitHub milestone for the next release, so that we can already move tasks
out of the current milestone if necessary.

1. Create the milestone for the new release via GitHub UI.

#### [Track] Remove previously deprecated code

The goal of this task is to remove all previously deprecated code that can be now removed.

1. Check for deprecated code and remove it.
    * We can't just remove all code flagged with `Deprecated`. In some cases like e.g. in API packages
      we have to keep the old code.

Prior art: [Remove code deprecated in v1.2](https://github.com/kubernetes-sigs/cluster-api/pull/6779)

#### [Track] Bump dependencies

The goal of this task is to ensure that we have relatively up-to-date dependencies at the time of the release.
This reduces the risk that CVEs are found in outdated dependencies after our release.

We should take a look at the following dependencies:
* Go dependencies in `go.mod` files.
* Tools used in our Makefile (e.g. kustomize).

#### Create a release branch

The goal of this task is to ensure we have a release branch and the milestone applier applies milestones accordingly.
From this point forward changes which should land in the release have to be cherry-picked into the release branch.

1. Create the release branch locally based on the latest commit on main and push it:
   ```bash
   # Create the release branch
   git checkout -b release-1.4

   # Push the release branch
   # Note: `upstream` must be the remote pointing to `github.com/kubernetes-sigs/cluster-api`.
   git push -u upstream release-1.4
   ```
2. Update the [milestone applier config](https://github.com/kubernetes/test-infra/blob/0b17ef5ffd6c7aa7d8ca1372d837acfb85f7bec6/config/prow/plugins.yaml#L371) accordingly (e.g. `release-1.4: v1.4` and `main: v1.5`)
   <br>Prior art: [cluster-api: update milestone applier config for v1.3](https://github.com/kubernetes/test-infra/pull/26631)

#### [Continuously] Maintain the GitHub release milestone

The goal of this task is to keep an overview over the current release milestone and the implementation
progress of issues assigned to the milestone.

This can be done by:
1. Regularly checking in with folks implementing an issue in the milestone.
2. If nobody is working on an issue in the milestone, drop it from the milestone.
3. Ensuring we have a plan to get `release-blocking` issues implemented in time.

#### [Continuously] Bump the Go version

The goal of this task is to ensure we are always using the latest Go version for our releases.

1. Keep track of new Go versions
2. Bump the Go version in supported branches if necessary
   <br>Prior art: [Bump to Go 1.19.5](https://github.com/kubernetes-sigs/cluster-api/pull/7981)

Note: If the Go minor version of one of our supported branches goes out of supported, we should consider bumping
to a newer Go minor version according to our [backport policy](./../../CONTRIBUTING.md#backporting-a-patch).

#### [Repeatedly] Cut a release

1. Ensure CI is stable before cutting the release (e.g. by checking with the CI manager)
   Note: special attention should be given to image scan results, so we can avoid cutting a release with CVE or document known CVEs in release notes.
2. Create and push the release tags to the GitHub repository:
   ```bash
   # Export the tag of the release to be cut, e.g.:
   export RELEASE_TAG=v1.0.1

   # Create tags locally
   # Warning: The test tag MUST NOT be an annotated tag.
   git tag -s -a ${RELEASE_TAG} -m ${RELEASE_TAG}
   git tag test/${RELEASE_TAG}

   # Push tags
   # Note: `upstream` must be the remote pointing to `github.com/kubernetes-sigs/cluster-api`.
   git push upstream ${RELEASE_TAG}
   git push upstream test/${RELEASE_TAG}
   ```
   **Note**: This will automatically trigger a [GitHub Action](https://github.com/kubernetes-sigs/cluster-api/actions/workflows/release.yml) to create a draft release and a [ProwJob](https://prow.k8s.io/?repo=kubernetes-sigs%2Fcluster-api&job=post-cluster-api-push-images) to publish images to the staging repository.
3. Promote images from the staging repository to the production registry (`registry.k8s.io/cluster-api`):
    1. Wait until images for the tag have been built and pushed to the [staging repository](https://console.cloud.google.com/gcr/images/k8s-staging-cluster-api) by the [post push images job](https://prow.k8s.io/?repo=kubernetes-sigs%2Fcluster-api&job=post-cluster-api-push-images).
    2. If you don't have a GitHub token, create one by going to your GitHub settings, in [Personal access tokens](https://github.com/settings/tokens). Make sure you give the token the `repo` scope.
    3. Create a PR to promote the images to the production registry:
       ```bash
       export GITHUB_TOKEN=<your GH token>
       make promote-images
       ```
       **Notes**:
        * `kpromo` uses `git@github.com:...` as remote to push the branch for the PR. If you don't have `ssh` set up you can configure
          git to use `https` instead via `git config --global url."https://github.com/".insteadOf git@github.com:`.
        * This will automatically create a PR in [k8s.io](https://github.com/kubernetes/k8s.io) and assign the CAPI maintainers.
    4. Merge the PR (/lgtm + /hold cancel) and verify the images are available in the production registry:
       ```bash
       docker pull registry.k8s.io/cluster-api/clusterctl:${RELEASE_TAG}
       docker pull registry.k8s.io/cluster-api/cluster-api-controller:${RELEASE_TAG}
       docker pull registry.k8s.io/cluster-api/kubeadm-bootstrap-controller:${RELEASE_TAG}
       docker pull registry.k8s.io/cluster-api/kubeadm-control-plane-controller:${RELEASE_TAG}
       ```
4. Publish the release in GitHub:
    1. Get the final release notes from the docs team and add them to the GitHub release.
    2. Publish the release (ensure to flag the release as pre-release if necessary).
5. Publish `clusterctl` to Homebrew by bumping the version in [clusterctl.rb](https://github.com/Homebrew/homebrew-core/blob/master/Formula/clusterctl.rb).
   <br>**Notes**:
    * This is only done for new latest stable releases, not for beta / RC releases and not for previous release branches.
    * Check if homebrew already has a PR to update the version (homebrew introduced automation that picks it up). Open one if no PR exists.
      * For an example please see: [PR: clusterctl 1.1.5](https://github.com/Homebrew/homebrew-core/pull/105075/files).
      * Homebrew has [conventions for commit messages](https://docs.brew.sh/Formula-Cookbook#commit) usually
        the commit message for us should look like: `clusterctl 1.1.5`.
6. Set EOL date for previous release (prior art: https://github.com/kubernetes-sigs/cluster-api/issues/7146).

Additional information:
* [Versioning documentation](./../../CONTRIBUTING.md#versioning) for more information.
* Cutting a release as of today requires permissions to:
    * Create a release tag on the GitHub repository.
    * Create/update/publish GitHub releases.

#### [Optional] [Track] Bump the Cluster API apiVersion

**Note** This should only be done when we have to bump the apiVersion of our APIs.

1. Add new version of the types:
    1. Create new api packages by copying existing packages.
    2. Make sure webhooks only exist in the latest apiVersion (same for other subpackages like `index`).
    3. Add conversion and conversion tests.
    4. Adjust generate targets in the Makefile.
    5. Consider dropping fields deprecated in the previous apiVersion.
2. Update import aliases in `.golangci.yml`.
3. Switch other code over to the new version (imports across the code base, e.g. controllers).
    1. Add all versions to the schema in the `main.go` files.
4. Add types to the `PROJECT` files of the respective provider.
5. Add test data for the new version in `test/e2e/data/{infrastructure-docker,shared}` (also update top-level `.gitignore`).
6. Update `docker.yaml`, make sure all tests are successful in CI.

#### [Optional] [Track] Bump the Kubernetes version

1. Create an issue for the new Kubernetes version via: [New Issue: Kubernetes bump](https://github.com/kubernetes-sigs/cluster-api/issues/new/choose).
2. Track the issue to ensure the work is completed in time.

## Communications/Docs/Release Notes Manager

### Responsibilities

* Communication:
    * Communicate key dates to the community
* Documentation:
    * Improve release process documentation
    * Ensure the book and provider upgrade documentation are up-to-date
    * Maintain and improve user facing documentation about releases, release policy and release calendar
* Release Notes:
    * Polish release notes

### Tasks

#### Add docs to collect release notes for users and migration notes for provider implementers

The goal of this task is to initially create the docs so that we can continuously add notes going forward.
The release notes doc will be used to collect release notes during the release cycle and will be eventually
used to write the final release notes. The provider migration doc is part of the book and contains instructions
for provider authors on how to adopt to the new Cluster API version.

1. Add a new migration doc for provider implementers.
   <br>Prior art: [Add v1.2 -> v1.3 migration doc](https://github.com/kubernetes-sigs/cluster-api/pull/6698)
2. Add a doc to collect initial release notes for users.

#### Update supported versions

1. Update supported versions in versions.md.
   <br>Prior art: [Update supported versions for v1.3](https://github.com/kubernetes-sigs/cluster-api/pull/6850)

#### Ensure the book for the new release is available

The goal of this task to make the book for the current release available under e.g. `https://release-1-4.cluster-api.sigs.k8s.io`.

1. Add a DNS entry for the book of the new release (should be available under e.g. `https://release-1-4.cluster-api.sigs.k8s.io`).
   <br>Prior art: [Add DNS for CAPI release-1.2 release branch](https://github.com/kubernetes/k8s.io/pull/3872)
2. Open `https://release-1-4.cluster-api.sigs.k8s.io/` and verify that the certificates are valid
   If they are not, talk to someone with access to Netlify, they have to click the `renew certificate` button in the Netlify UI.
3. Update references in introduction.md only on the main branch (drop unsupported versions, add the new release version).
   <br>Prior art: [Add release 1.2 book link](https://github.com/kubernetes-sigs/cluster-api/pull/6697)

#### Polish release notes

1. Checkout the latest commit on the release branch, e.g. `release-1.4`.
2. Generate release notes with:
   ```bash
   # PREVIOUS_TAG should be the last patch release of the previous minor release.
   PREVIOUS_TAG=v1.3.x
   go run ./hack/tools/release/notes.go --from=${PREVIOUS_TAG} > tmp.md
   ```
3. Finalize the release notes:
    1. Copy & paste the release notes into a hackmd (makes collaboration very easy).
    2. Pay close attention to the `## :question: Sort these by hand` section, as it contains items that need to be manually sorted.
    3. Ensure consistent formatting of entries (e.g. prefix (see [v1.2.0](https://github.com/kubernetes-sigs/cluster-api/releases/tag/v1.2.0) release notes)).
    4. Merge dependency bump PR entries for the same dependency into a single entry.
    5. Move minor changes into a single line at the end of each section.
    6. Sort entries within a section alphabetically.
    7. Write highlights section based on the initial release notes doc.
    8. Add Kubernetes version support section.
    9. Modify `Changes since v1.x.y` to `Changes since v1.x`
       <br>**Note**: The release notes tool includes all merges since the previous release branch was branched of.
4. Iterate until the GA release by generating incremental release notes and modifying the release notes in hackmd accordingly:
   ```bash
   # PREVIOUS_TAG should be the tag from which the previous release notes have been generated, e.g.:
   PREVIOUS_TAG=v1.4.0-rc.0
   go run ./hack/tools/release/notes.go --from=${PREVIOUS_TAG} > tmp.md
   ```

#### Change production branch in Netlify to the new release branch

The goal of this task to make the book for the current release available under `https://cluster-api.sigs.k8s.io`.

Someone with access to Netlify should:
1. Change production branch in Netlify the current release branch (e.g. `release-1.4`) to make the book available under `https://cluster-api.sigs.k8s.io`.
2. Re-deploy via the Netlify UI.

#### Update clusterctl links in the quickstart

The goal of this task is to ensure the quickstart has links to the latest `clusterctl` binaries.

1. Update clusterctl links in the quickstart (on main and cherry-pick onto release-1.4).
   <br>Prior art: [Update clusterctl version to v1.2.x in quick start](https://github.com/kubernetes-sigs/cluster-api/pull/6716)

**Note**: The PR for this should be merged after the minor release has been published.

#### Continuously: Communicate key dates to the community

The goal of this task is to ensure all stakeholders are informed about the current release cycle.

Information can be distributed via: (TBD)
* `sig-cluster-lifecycle` mailing list
* Slack
* Office hours
* ClusterAPI book
* ...

Relevant information includes: (TBD)
* Beta, RC, GA release
* Start of code freeze
* Implementation progress
* ...

Stakeholders are: (TBD)
* End users of ClusterAPI
* Contributors to core ClusterAPI
* Provider implementers
* ...

## CI Signal/Bug Triage/Automation Manager

### Responsibilities

* Signal:
    * Responsibility for the quality of the release
    * Continuously monitor CI signal, so a release can be cut at any time
    * Add CI signal for new release branches
* Bug Triage:
    * Make sure blocking issues and bugs are triaged and dealt with in a timely fashion
* Automation
    * Maintain and improve release automation, tooling & related developer docs

### Tasks

#### Setup jobs and dashboards for a new release branch

The goal of this task is to have test coverage for the new release branch and results in testgrid.
While we add test coverage for the new release branch we will also drop the tests for old release branches if necessary.

1. Create new jobs based on the jobs running against our `main` branch:
    1. Copy `config/jobs/kubernetes-sigs/cluster-api/cluster-api-periodics-main.yaml` to `config/jobs/kubernetes-sigs/cluster-api/cluster-api-periodics-release-1-4.yaml`.
    2. Copy `test-infra/config/jobs/kubernetes-sigs/cluster-api/cluster-api-periodics-main-upgrades.yaml` to `test-infra/config/jobs/kubernetes-sigs/cluster-api/cluster-api-periodics-release-1-4-upgrades.yaml`.
    3. Copy `test-infra/config/jobs/kubernetes-sigs/cluster-api/cluster-api-presubmits-main.yaml` to `test-infra/config/jobs/kubernetes-sigs/cluster-api/cluster-api-presubmits-release-1-4.yaml`.
    4. Modify the following:
        1. Rename the jobs, e.g.: `periodic-cluster-api-test-main` => `periodic-cluster-api-test-release-1-4`.
        2. Change `annotations.testgrid-dashboards` to `sig-cluster-lifecycle-cluster-api-1.4`.
        3. Change `annotations.testgrid-tab-name`, e.g. `capi-test-main` => `capi-test-release-1-4`.
        4. For periodics additionally:
            * Change `extra_refs[].base_ref` to `release-1.4` (for repo: `cluster-api`).
            * Change interval (let's use the same as for `1.3`).
        5. For presubmits additionally: Adjust branches: `^main$` => `^release-1.4$`.
2. Create a new dashboard for the new branch in: `test-infra/config/testgrids/kubernetes/sig-cluster-lifecycle/config.yaml` (`dashboard_groups` and `dashboards`).
3. Remove tests for old release branches according to our policy documented in [Support and guarantees](../../CONTRIBUTING.md#support-and-guarantees)
   For example, let's assume we just created tests for v1.4, then we can now drop test coverage for the release-1.1 branch.
4. Verify the jobs and dashboards a day later by taking a look at: `https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api-1.4`

Prior art: [Add jobs for CAPI release 1.2](https://github.com/kubernetes/test-infra/pull/26621)

#### [Continuously] Monitor CI signal

The goal of this task is to keep our tests running in CI stable.

**Note**: To be very clear, this is not meant to be an on-call role for Cluster API tests.

1. Add yourself to the [Cluster API alert mailing list](https://github.com/kubernetes/k8s.io/blob/151899b2de933e58a4dfd1bfc2c133ce5a8bbe22/groups/sig-cluster-lifecycle/groups.yaml#L20-L35)
    <br\>**Note**: An alternative to the alert mailing list is manually monitoring the [testgrid dashboards](https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api)
    (also dashboards of previous releases). Using the alert mailing list has proven to be a lot less effort though.
2. Check the existing **failing-test** and **flaking-test** issue templates under `.github/ISSUE_TEMPLATE/` folder of the repo, used to create an issue for failing or flaking tests respectively. Please make sure they are up-to-date and if not, send a PR to update or improve them.
3. Triage CI failures reported by mail alerts or found by monitoring the testgrid dashboards:
    1. Create an issue using an appropriate template (failing-test) in the Cluster API repository to surface the CI failure.
    2. Identify if the issue is a known issue, new issue or a regression.
    3. Mark the issue as `release-blocking` if applicable.
4. Triage periodic GitHub actions failures, with special attention to image scan results;
   Eventually open issues as described above.
5. Monitor IPv6 testing PR informing jobs (look for `capi-pr-e2e-informing-ipv6-<branch_name>` tab on main and supported releases testgrid dashboards), since they are not part of any periodic jobs.

#### [Continuously] Reduce the amount of flaky tests

The Cluster API tests are pretty stable, but there are still some flaky tests from time to time.

To reduce the amount of flakes please periodically:
1. Take a look at recent CI failures via `k8s-triage`:
    * [periodic-cluster-api-e2e-main](https://storage.googleapis.com/k8s-triage/index.html?pr=1&job=periodic-cluster-api-e2e-main)
    * [periodic-cluster-api-e2e-mink8s-main](https://storage.googleapis.com/k8s-triage/index.html?pr=1&job=periodic-cluster-api-e2e-mink8s-main)
    * [periodic-cluster-api-test-main](https://storage.googleapis.com/k8s-triage/index.html?pr=1&job=periodic-cluster-api-test-main)
    * [periodic-cluster-api-test-mink8s-main](https://storage.googleapis.com/k8s-triage/index.html?pr=1&job=periodic-cluster-api-test-mink8s-main)
2. Open issues using an appropriate template (flaking-test) for occurring flakes and ideally fix them or find someone who can.
   **Note**: Given resource limitations in the Prow cluster it might not be possible to fix all flakes.
   Let's just try to pragmatically keep the amount of flakes pretty low.

#### [Continuously] Bug triage

The goal of bug triage is to triage incoming issues and if necessary flag them with `release-blocking`
and add them to the milestone of the current release.

We probably have to figure out some details about the overlap between the bug triage task here, release leads
and Cluster API maintainers.
