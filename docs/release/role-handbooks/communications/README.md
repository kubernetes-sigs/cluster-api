# Communications/Docs/Release Notes Manager

## Overview

* If a task is prefixed with `[Track]` it means it should be ensured that this task is done, but the folks with the corresponding role are not responsible to do it themselves.

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->


- [Responsibilities](#responsibilities)
- [Tasks](#tasks)
  - [Add docs to collect release notes for users and migration notes for provider implementers](#add-docs-to-collect-release-notes-for-users-and-migration-notes-for-provider-implementers)
  - [Update supported versions](#update-supported-versions)
  - [Ensure the book for the new release is available](#ensure-the-book-for-the-new-release-is-available)
  - [Generate weekly PR updates to post in Slack](#generate-weekly-pr-updates-to-post-in-slack)
  - [Create PR for release notes](#create-pr-for-release-notes)
  - [Change production branch in Netlify to the new release branch](#change-production-branch-in-netlify-to-the-new-release-branch)
  - [Update clusterctl links in the quickstart](#update-clusterctl-links-in-the-quickstart)
  - [Continuously: Communicate key dates to the community](#continuously-communicate-key-dates-to-the-community)
  - [Communicate beta to providers](#communicate-beta-to-providers)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Responsibilities

* Communication:
  * Communicate key dates to the community
* Documentation:
  * Improve release process documentation
  * Ensure the book and provider upgrade documentation are up-to-date
  * Maintain and improve user facing documentation about releases, release policy and release calendar
* Release Notes:
  * Create PR with release notes

## Tasks

### Add docs to collect release notes for users and migration notes for provider implementers

The goal of this task is to initially create the docs so that we can continuously add notes going forward.
The release notes doc will be used to collect release notes during the release cycle and will be eventually
used to write the final release notes. The provider migration doc is part of the book and contains instructions
for provider authors on how to adopt to the new Cluster API version.

1. Add a new migration doc for provider implementers.
   <br>Prior art: [Add v1.5 -> v1.6 migration doc](part of: https://github.com/kubernetes-sigs/cluster-api/pull/8996) - see changes to [SUMMARY.md](https://github.com/kubernetes-sigs/cluster-api/pull/8996/files#diff-72d1da5cbeb1afbe684444ec598fbe1815dd2ddc6aa99078ab577cefb9e279ac) and addition of [v1.5-to-v1.6.md](https://github.com/kubernetes-sigs/cluster-api/pull/8996/files#diff-135e34a16773fd40a82b4adbb265444a4fed6c1a973f48d621082b957e7ef93f)

### Update supported versions

1. Update supported versions in versions.md.
   <br>Prior art: [Update supported versions for v1.6](https://github.com/kubernetes-sigs/cluster-api/pull/9119)

### Ensure the book for the new release is available

The goal of this task to make the book for the current release available under e.g. `https://release-1-6.cluster-api.sigs.k8s.io`.

1. Add a DNS entry for the book of the new release (should be available under e.g. `https://release-1-6.cluster-api.sigs.k8s.io`).
   <br>Prior art: [Add DNS for CAPI release-1.6 release branch](https://github.com/kubernetes/k8s.io/pull/6114)
2. Open `https://release-1-6.cluster-api.sigs.k8s.io/` and verify that the certificates are valid.  If they are not, reach out to CAPI maintainers and request someone with access Netlify [click the `renew certificate` button](https://app.netlify.com/sites/kubernetes-sigs-cluster-api/settings/domain#https) in the Netlify UI.
    - To add new subdomains to the certificate config, checkout the email snippet [template](https://github.com/kubernetes-sigs/cluster-api/issues/6017#issuecomment-1823306891) for reference.
3. Update references in introduction.md only on the main branch (drop unsupported versions, add the new release version).
   <br>Prior art: [Add release 1.5 book link](https://github.com/kubernetes-sigs/cluster-api/pull/9767)

### Generate weekly PR updates to post in Slack
The goal of this task is to keep the CAPI community updated on recent PRs that have been merged. This is done by using the weekly update tool in `hack/tools/release/weekly/main.go`. Here is how to use it:
1. Build the release weekly update tools binary.
   ```bash
   make release-weekly-update-tool
   ```
2. Generate the weekly update with the following command for desired branches:
   ```bash
   ./bin/weekly --since <YYYY-MM-DD> --until <YYYY-MM-DD> --branch <branch_name>
   ```
   **Note:** the `GITHUB_TOKEN` environment variable can be set to prevent/reduce GitHub rate limiting issues.
3. Paste the output into a new Slack message in the [`#cluster-api`](https://kubernetes.slack.com/archives/C8TSNPY4T) channel. Currently, we post separate messages in a thread for the branch `main` - which corresponds to the active milestone - as well as the two most recent release branches (e.g. `release-1.6` and `release-1.5`).

   Example commands to run for the weekly update during ongoing development towards release 1.7:
   ```bash
   # Main branch changes, which correspond to actively worked on release 1.7
   ./bin/weekly --since 2024-01-22 --until 2024-01-28 --branch main

   # Previous 2 release branches
   ./bin/weekly --since 2024-01-22 --until 2024-01-28 --branch release-1.6
   ./bin/weekly --since 2024-01-22 --until 2024-01-28 --branch release-1.5
   ```

### Create PR for release notes

1. Checkout the `main` branch.
2. Generate release notes with:

   1. RELEASE CANDIDATE/BETA RELEASE example:

      ```bash
      # RELEASE_TAG should be the new desired tag (note: at this point the tag does not yet exist).
      # PREVIOUS_RELEASE_TAG is the previous released tag for determining the changes.
      RELEASE_TAG=v1.7.x-rc.1 PREVIOUS_RELEASE_TAG=tags/v1.7.x-rc.0 make release-notes
      ```

      **Note**: For a first pre-release version without a pre-release precedent, use above command without `PREVIOUS_RELEASE_TAG`.

   2. STABLE RELEASE example:

      ```bash
      # RELEASE_TAG should be the new desired tag (note: at this point the tag does not yet exist).
      RELEASE_TAG=v1.7.x make release-notes
      ```

3. This will generate a new release notes file at `CHANGELOG/<RELEASE_TAG>.md`. Finalize the release notes:
    - [ ] Look for any `MISSING_AREA` entries. Add the corresponding label to the PR and regenerate the notes.
    - [ ] Look for any `MULTIPLE_AREAS` entries. If the PR does indeed guarantee multiple areas, just remove the `MULTIPLE_AREAS` prefix and just leave the areas. Otherwise, fix the labels in the PR and regenerate the notes.
    - [ ] Review that all areas are correctly assigned to each PR. If not, correct the labels and regenerate the notes.
    - [ ] Update the `Kubernetes version support section`. If this is a patch release you can most probably copy the same values from the previous patch release notes. Except if this is the release where a new Kubernetes version support is added.
       <br>**Note**: Check our [Kubernetes support policy](https://cluster-api.sigs.k8s.io/reference/versions.html#supported-kubernetes-versions) in the CAPI book. In case of doubt, reach out to the current release lead.
    - [ ] If this is a `vX.X.0` release, fill in the content for the `Highlights` section. Otherwise, remove the section altogether.
    - [ ] If there a deprecations in this release (for example, a CAPI API version drop), add them, to the `Deprecation Warning` section. Otherwise, remove the section altogether.
    - [ ] Look for area duplications in PR title. Sometimes authors add a prefix in their PR title that matches the area label. When the notes are generated, the area is as a prefix to the PR title, which can create redundant information. Remove the one from the PR title and just leave the area. Make sure you capitalize the title after this.
    - [ ] Check that all entries are in the right section. Sometimes the wrong emoji prefix is added to the PR title, which drives the section in which the entry is added in the release notes. Manually move any entry as needed. Note that fixing the PR title won't fix this even after regenerating the notes, since the notes tool reads this info from the commit messages and these don't get rewritten.
    - [ ] Sort manually all entries if you made any manual edits that might have altered the correct order.
    - [ ] **For minor releases:** Modify `Changes since v1.x.y` to `Changes since v1.x`
       <br>**Note**: The release notes tool includes all merges since the previous release branch was branched of.
4. Checkout `main`, branch out from it and add `CHANGELOG/<RELEASE_TAG>.md`.
5. Open a pull request **against the main branch** with all manual edits to `CHANGELOG/<RELEASE_TAG>.md` which is used for the new release notes. The commit and PR title should be `🚀 Release v1.x.y`.
       <br>**Note**: Important! The commit should only contain the release notes file, nothing else, otherwise automation will not work.


### Change production branch in Netlify to the new release branch

The goal of this task to make the book for the current release available under `https://cluster-api.sigs.k8s.io`.

Reach out to the CAPI maintainers to request someone with access to Netlify perform the following steps:

1. Change production branch in Netlify the current release branch (e.g. `release-1.6`) to make the book available under `https://cluster-api.sigs.k8s.io`. It's done under [production branch settings](https://app.netlify.com/sites/kubernetes-sigs-cluster-api/settings/deploys#branches-and-deploy-contexts)
2. [Trigger a redeploy](https://app.netlify.com/sites/kubernetes-sigs-cluster-api/deploys).

### Update clusterctl links in the quickstart

The goal of this task is to ensure the quickstart has links to the latest `clusterctl` binaries.

Update clusterctl links in the quickstart (on main and cherry-pick onto release-1.6).
<br>Prior art: [Update clusterctl version to v1.6.x in quick start](https://github.com/kubernetes-sigs/cluster-api/pull/9801)

**Note**: The PR for this should be merged after the minor release has been published. Recommended to create it before
the release but with `/hold`. This will allow maintainers to review and approve before the release. When the release is
done just remove the hold to merge it.

### Continuously: Communicate key dates to the community

The goal of this task is to ensure all stakeholders are informed about the current release cycle. For example announcing
upcoming code freezes etc based on the [release timeline (1.6 example)](../../releases/release-1.6.md).

Templates for all types of communication can be found in the [release-templates page](../../release-templates.md).

Information can be distributed via:

* `sig-cluster-lifecycle` mailing list
  * Note: The person sending out the email should ensure that they are first part of the mailing list. If the email is sent out is not received by the community, reach out to the maintainers to unblock and approve the email.
* #cluster-api Slack channel
* Office hours
* Release Team meetings
* Cluster API book
* [Github Issue](#communicate-beta-to-providers) (when communicating beta release to providers)

Relevant information includes:

* Beta, RC, GA and patch release
* Start of code freeze
* Implementation progress
* Release delays and changes if applicable

Stakeholders are:

* End users of Cluster API
* Contributors to core Cluster API
* Provider implementers

### Communicate beta to providers

The goal of this task is to inform all providers that a new beta.0 version a release is out and that it should be tested. We want to prevent issues where providers don't have enough time to test before a new version of CAPI is released. This stems from a previous issue we are trying to avoid: https://github.com/kubernetes-sigs/cluster-api/issues/8498

We should inform at least the following providers via a new issue on their respective repos that a new version of CAPI is being released (provide the release date) and that the beta.0 version is ready for them to test.

* Addon provider helm: https://github.com/kubernetes-sigs/cluster-api-addon-provider-helm/issues/new
* AWS: https://github.com/kubernetes-sigs/cluster-api-provider-aws/issues/new
* Azure: https://github.com/kubernetes-sigs/cluster-api-provider-azure/issues/new
* Cloudstack: https://github.com/kubernetes-sigs/cluster-api-provider-cloudstack/issues/new
* Digital Ocean: https://github.com/kubernetes-sigs/cluster-api-provider-digitalocean/issues/new
* GCP: https://github.com/kubernetes-sigs/cluster-api-provider-gcp/issues/new
* Kubemark: https://github.com/kubernetes-sigs/cluster-api-provider-kubemark/issues/new
* Kubevirt: https://github.com/kubernetes-sigs/cluster-api-provider-kubevirt/issues/new
* IBMCloud: https://github.com/kubernetes-sigs/cluster-api-provider-ibmcloud/issues/new
* Metal3: https://github.com/metal3-io/cluster-api-provider-metal3/issues/new
* Nested: https://github.com/kubernetes-sigs/cluster-api-provider-nested/issues/new
* OCI: https://github.com/oracle/cluster-api-provider-oci/issues/new
* Openstack: https://github.com/kubernetes-sigs/cluster-api-provider-openstack/issues/new
* Operator: https://github.com/kubernetes-sigs/cluster-api-operator/issues/new
* Packet: https://github.com/kubernetes-sigs/cluster-api-provider-packet/issues/new
* vSphere: https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/issues/new

To create GitHub issues at the Cluster API providers repositories and inform about a new minor beta release, use ["provider_issues.go"](../../../../hack/tools/release/internal/update_providers/provider_issues.go) go utility.
- Ensure that the [provider repos pre-requisites](../../../../hack/tools/release/internal/update_providers/README.md#pre-requisites) are completed.
- From the root of this repository, run `make release-provider-issues-tool` to create git issues at the provider repositories.