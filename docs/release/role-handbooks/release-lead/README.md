# Release Team Lead

## Overview

* If a task is prefixed with `[Track]` it means it should be ensured that this task is done, but the folks with the corresponding role are not responsible to do it themselves.

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->


- [Responsibilities](#responsibilities)
- [Tasks](#tasks)
  - [Finalize release schedule and team](#finalize-release-schedule-and-team)
  - [Add/remove release team members](#addremove-release-team-members)
  - [Prepare main branch for development of the new release](#prepare-main-branch-for-development-of-the-new-release)
  - [Create a new GitHub milestone for the next release](#create-a-new-github-milestone-for-the-next-release)
  - [[Track] Remove previously deprecated code](#track-remove-previously-deprecated-code)
  - [[Track] Bump dependencies](#track-bump-dependencies)
  - [Set a tentative release date for the next minor release](#set-a-tentative-release-date-for-the-next-minor-release)
  - [Assemble next release team](#assemble-next-release-team)
  - [Update milestone applier and GitHub Actions](#update-milestone-applier-and-github-actions)
  - [[Continuously] Maintain the GitHub release milestone](#continuously-maintain-the-github-release-milestone)
  - [[Continuously] Bump the Go version](#continuously-bump-the-go-version)
  - [[Repeatedly] Cut a release](#repeatedly-cut-a-release)
  - [[Optional] Public release session](#optional-public-release-session)
  - [[Optional] [Track] Bump the Cluster API apiVersion](#optional-track-bump-the-cluster-api-apiversion)
  - [[Optional] [Track] Bump the Kubernetes version](#optional-track-bump-the-kubernetes-version)
  - [[Optional] Track Release and Improvement tasks](#optional-track-release-and-improvement-tasks)

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
   1. Goal is that we have clusterctl upgrade tests for all relevant upgrade cases
       1. Modify the test specs in `test/e2e/clusterctl_upgrade_test.go`. Please note the comments above each test case (look for `This test should be changed during "prepare main branch"`)
          Please note that both `InitWithKubernetesVersion` and `WorkloadKubernetesVersion` should be the highest management cluster version supported by the respective Cluster API version.
       2. Please ping maintainers after these changes are made for a first round of feedback before continuing with the steps below.
   2. Update providers in `docker.yaml`:
       1. Add a new `v1.6` entry.
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
   5. Add a new Makefile target (e.g. `generate-e2e-templates-v1.6`) and potentially remove the Makefile target of versions that are not used anymore (if something was removed in 4.2)
2. Update `create-local-repository.py` and `tools/internal/tilt-prepare/main.go`: `v1.5.99` => `v1.6.99`.
3. Make sure all tests are green (also run `pull-cluster-api-e2e-full-main` and `pull-cluster-api-e2e-workload-upgrade-1-27-latest-main`).

Prior art:

* 1.9 - https://github.com/kubernetes-sigs/cluster-api/pull/11059

### Create a new GitHub milestone for the next release

The goal of this task is to create [a new GitHub milestone](https://github.com/kubernetes-sigs/cluster-api/milestones) for the next release, so that we can already move tasks
out of the current milestone if necessary.

1. Create the milestone for the new release via GitHub UI.

### [Track] Remove previously deprecated code

The goal of this task is to remove all previously deprecated code that can be now removed.

1. Check for deprecated code and remove it.
    * We can't just remove all code flagged with `Deprecated`. In some cases like e.g. in API packages
      we have to keep the old code.

Prior art: [Remove code deprecated in v1.6](https://github.com/kubernetes-sigs/cluster-api/pull/9136)

### [Track] Bump dependencies

The goal of this task is to ensure that we have relatively up-to-date dependencies at the time of the release.
This reduces the risk that CVEs are found in outdated dependencies after our release.

We should take a look at the following dependencies:

* Go dependencies in `go.mod` files.
* Tools used in our Makefile (e.g. kustomize).

### Set a tentative release date for the next minor release

1. Set a tentative release date for the next minor release and document it by creating a `release-X.Y.md` in [docs/release/releases](../../releases).
   <br>Prior art: https://github.com/kubernetes-sigs/cluster-api/pull/9635

### Assemble next release team

There is currently no formalized process to assemble the release team.
As of now we ask for volunteers in Slack and office hours.

Overweighing the CI team with members is preferred as maintaining a clean CI signal is crucial to the health of the project.

### Update milestone applier and GitHub Actions

Once release branch is created by GitHub Automation, the goal of this task would be to ensure we have the milestone
applier that applies milestones accordingly and to update GitHub actions to work with new release version.
From this point forward changes which should land in the release have to be cherry-picked into the release branch.

1. Update the [milestone applier config](https://github.com/kubernetes/test-infra/blob/0b17ef5ffd6c7aa7d8ca1372d837acfb85f7bec6/config/prow/plugins.yaml#L371) accordingly (e.g. `release-1.5: v1.5` and `main: v1.6`)
   <br>Prior art: [cluster-api: update milestone applier config for v1.5](https://github.com/kubernetes/test-infra/pull/30058)

2. Update the GitHub Actions to work with the new release version.
   <br>Prior art: [Update actions for v1.7](https://github.com/kubernetes-sigs/cluster-api/pull/10357)

### [Continuously] Maintain the GitHub release milestone

The goal of this task is to keep an overview over the current release milestone and the implementation
progress of issues assigned to the milestone.

This can be done by:

1. Regularly checking in with folks implementing an issue in the milestone.
2. If nobody is working on an issue in the milestone, drop it from the milestone.
3. Ensuring we have a plan to get `release-blocking` issues implemented in time.

### [Continuously] Bump the Go version

The goal of this task is to ensure we are always using the latest Go version for our releases.

1. Keep track of new Go versions
2. Bump the Go version in supported branches if necessary
   <br>Prior art: [Bump to Go 1.19.5](https://github.com/kubernetes-sigs/cluster-api/pull/7981)

Note: If the Go minor version of one of our supported branches goes out of supported, we should consider bumping
to a newer Go minor version according to our [backport policy](./../../../../CONTRIBUTING.md#backporting-a-patch).

### [Repeatedly] Cut a release

1. Ensure CI is stable before cutting the release (e.g. by checking with the CI manager)
   Note: special attention should be given to image scan results, so we can avoid cutting a release with CVE or document known CVEs in release notes.
2. Ask the [Communications/Docs/Release Notes Manager](../communications/README.md) to [create a PR with the release notes](../communications/README.md#create-pr-for-release-notes) for the new desired tag and review the PR. Once the PR merges, it will trigger a [GitHub Action](https://github.com/kubernetes-sigs/cluster-api/actions/workflows/release.yaml) to create a release branch, push release tags, and create a draft release. This will also trigger a [ProwJob](https://prow.k8s.io/?repo=kubernetes-sigs%2Fcluster-api&job=post-cluster-api-push-images) to publish images to the staging repository.
3. Promote images from the staging repository to the production registry (`registry.k8s.io/cluster-api`):
    1. Wait until images for the tag have been built and pushed to the [staging repository](https://console.cloud.google.com/gcr/images/k8s-staging-cluster-api) by the [post push images job](https://prow.k8s.io/?repo=kubernetes-sigs%2Fcluster-api&job=post-cluster-api-push-images).
    2. If you don't have a GitHub token, create one by going to your GitHub settings, in [Personal access tokens](https://github.com/settings/tokens). Make sure you give the token the `repo` scope.
    3. Create a PR to promote the images to the production registry:

       ```bash
       # Export the tag of the release to be cut, e.g.:
       export RELEASE_TAG=v1.0.1
       export GITHUB_TOKEN=<your GH token>
       make promote-images
       ```

       **Notes**:
        * `make promote-images` target tries to figure out your Github user handle in order to find the forked [k8s.io](https://github.com/kubernetes/k8s.io) repository.
          If you have not forked the repo, please do it before running the Makefile target.
        * if `make promote-images` fails with an error like `FATAL while checking fork of kubernetes/k8s.io` you may be able to solve it by manually setting the USER_FORK variable i.e.  `export USER_FORK=<personal GitHub handle>`
        * `kpromo` uses `git@github.com:...` as remote to push the branch for the PR. If you don't have `ssh` set up you can configure
          git to use `https` instead via `git config --global url."https://github.com/".insteadOf git@github.com:`.
        * This will automatically create a PR in [k8s.io](https://github.com/kubernetes/k8s.io) and assign the CAPI maintainers.
    4. Merge the PR (/lgtm + /hold cancel) and verify the images are available in the production registry:
         * Wait for the [promotion prow job](https://prow.k8s.io/?repo=kubernetes%2Fk8s.io&job=post-k8sio-image-promo) to complete successfully. Then test the production images are accessible:

         ```bash
         docker pull registry.k8s.io/cluster-api/clusterctl:${RELEASE_TAG} &&
         docker pull registry.k8s.io/cluster-api/cluster-api-controller:${RELEASE_TAG} &&
         docker pull registry.k8s.io/cluster-api/kubeadm-bootstrap-controller:${RELEASE_TAG} &&
         docker pull registry.k8s.io/cluster-api/kubeadm-control-plane-controller:${RELEASE_TAG}
         ```

4. Publish the release in GitHub:
   1. Reach out to one of the maintainers over the Slack to publish the release in GitHub.
      * The draft release should be automatically created via the [Create Release GitHub Action](https://github.com/kubernetes-sigs/cluster-api/actions/workflows/release.yaml) with release notes previously committed to the repo by the release team. Ensure by reminding the maintainer that release is flagged as `pre-release` for all `beta` and `rc` releases or `latest` for a new release in the most recent release branch.

5. Publish `clusterctl` to Homebrew by bumping the version in [clusterctl.rb](https://github.com/Homebrew/homebrew-core/blob/master/Formula/c/clusterctl.rb).
   <br>**Notes**:
    * This is only done for new latest stable releases, not for beta / RC releases and not for previous release branches.
    * Check if homebrew already has a PR to update the version (homebrew introduced automation that picks it up). Open one if no PR exists.
      * To open a PR, you need two things: `tag` (i.e v1.5.3 & v1.4.8 releases are being published, where release-1.5 is the latest stable release branch, so tag would be v1.5.4) and `revision` (it is a commit hash of the tag, i.e if the tag is v1.5.3, it can be found by looking for commit id in [v1.5.3 tag page](https://github.com/kubernetes-sigs/cluster-api/releases/tag/v1.5.3)).
      * Once the PR is open, no action should be needed. Homebrew bot should push a second commit (see an example [here](https://github.com/Homebrew/homebrew-core/pull/129986/commits/0da6edddf1143aa50033f7e8ae1ebd07ecdd0941)) to the same PR to update the binary hashes automatically.
      * For an example please see: [PR: clusterctl 1.5.3](https://github.com/Homebrew/homebrew-core/pull/152279).
      * Homebrew has [conventions for commit messages](https://docs.brew.sh/Formula-Cookbook#commit) usually
        the commit message for us should look like: `clusterctl 1.5.3`.
6. **For minor releases** Set EOL date for previous release and update Cluster API support and guarantees in CONTRIBUTING.md (prior art: https://github.com/kubernetes-sigs/cluster-api/pull/9817/files).
7. **For latest stable releases** Index the most recent CRDs in the release by navigating to `https://doc.crds.dev/github.com/kubernetes-sigs/cluster-api@<CURRENT_RELEASE>` 

Additional information:

* [Versioning documentation](./../../../../CONTRIBUTING.md#versioning) for more information.
* Cutting a release as of today requires permissions to:
  * Create a release tag on the GitHub repository.
  * Create/update/publish GitHub releases.

### [Optional] Public release session
   1. Host a release session over a public zoom meeting.
   2. Record the session for future reference and transparency.
   3. Use release process-related waiting periods as a forum for discussing issues/questions.
   4. Publish the recording on YouTube channel.

### [Optional] [Track] Bump the Cluster API apiVersion

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

### [Optional] [Track] Bump the Kubernetes version

1. Create an issue for the new Kubernetes version via: [New Issue: Kubernetes bump](https://github.com/kubernetes-sigs/cluster-api/issues/new/choose).
2. Track the issue to ensure the work is completed in time.

### [Optional] Track Release and Improvement tasks

1. Create an issue for easier tracking of all the tasks for the release cycle in question.
   <br>Prior art: [Tasks for v1.6 release cycle](https://github.com/kubernetes-sigs/cluster-api/issues/9094)
2. Create a release improvement tasks [GitHub Project Board](https://github.com/orgs/kubernetes-sigs/projects/55) to track 
   the current status of all improvement tasks planned for the release, their priorities, status (i.e `Done`/`In Progress`)
   and to distribute the work among the Release Team members.

   **Notes**:
   * At the beginning of the cycle, Release Team Lead should prepare the improvement tasks board for the ongoing release cycle.
     The following steps can be taken:
      - Edit improvement tasks board name for current cycle (e.g. `CAPI vX.Y release improvement tasks`)
      - Add/move all individual missing issues to the board
   * Tasks that improve release automation, tooling & related developer docs are ideal candidates and should be prioritized.