# Jobs

This document intents to provide an overview over our jobs running via Prow, GitHub actions and Google Cloud Build.
It also documents the cluster-api specific configuration in test-infra.

## Builds and Tests running on the main branch

> NOTE: To see which test jobs execute which tests or e2e tests, you can click on the links which lead to the respective test overviews in testgrid.

The dashboards for the ProwJobs can be found here: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api

More details about ProwJob configurations can be found here: [cluster-api-prowjob-gen.yaml](https://github.com/kubernetes/test-infra/blob/master/config/jobs/kubernetes-sigs/cluster-api/cluster-api-prowjob-gen.yaml)

### Presubmits

Prow Presubmits:
* mandatory for merge, always run:
  * pull-cluster-api-build-main `./scripts/ci-build.sh`
  * pull-cluster-api-verify-main `./scripts/ci-verify.sh`
* mandatory for merge, run if go code changes:
  * pull-cluster-api-test-main `./scripts/ci-test.sh`
  * pull-cluster-api-e2e-blocking-main `./scripts/ci-e2e.sh`
    * GINKGO_FOCUS: `[PR-Blocking]`
* optional for merge, run if go code changes:
  * pull-cluster-api-apidiff-main `./scripts/ci-apidiff.sh`
* mandatory for merge, run if manually triggered:
  * pull-cluster-api-test-mink8s-main `./scripts/ci-test.sh`
  * pull-cluster-api-e2e-mink8s-main `./scripts/ci-e2e.sh`
    * GINKGO_SKIP: `[Conformance]|[IPv6]`
  * pull-cluster-api-e2e-dualstack-and-ipv6-main `./scripts/ci-e2e.sh`
    * DOCKER_IN_DOCKER_IPV6_ENABLED: `true`
    * GINKGO_SKIP: `[Conformance]`
  * pull-cluster-api-e2e-main `./scripts/ci-e2e.sh`
    * GINKGO_SKIP: `[Conformance]|[IPv6]`
  * pull-cluster-api-e2e-upgrade-* `./scripts/ci-e2e.sh`
    * GINKGO_FOCUS: `[Conformance] [K8s-Upgrade]`  
  * pull-cluster-api-e2e-conformance-main `./scripts/ci-e2e.sh`
    * GINKGO_FOCUS: `[Conformance] [K8s-Install]`
  * pull-cluster-api-e2e-conformance-ci-latest-main `./scripts/ci-e2e.sh`
    * GINKGO_FOCUS: `[Conformance] [K8s-Install-ci-latest]`

GitHub Presubmit Workflows:
* PR golangci-lint: golangci/golangci-lint-action
  * Runs golangci-lint. Can be run locally via `make lint`.
* PR verify: kubernetes-sigs/kubebuilder-release-tools verifier
  * Verifies the PR titles have a valid format, i.e. contains one of the valid icons.
  * Verifies the PR description is valid, i.e. is long enough.
* PR check Markdown links (run when markdown files changed)
  * Checks markdown modified in PR for broken links.
* PR dependabot (run on dependabot PRs)
  * Regenerates Go modules and code.
* PR approve GH Workflows
  * Approves other GH workflows if the `ok-to-test` label is set.

GitHub Weekly Workflows:
* Weekly check all Markdown links
  * Checks markdown across the repo for broken links.
* Weekly image scan:
  * Scan all images for vulnerabilities. Can be run locally via `make verify-container-images`
* Weekly release test:
  * Test the the `release` make target is working without errors.
  
Other Github workflows
* release (runs when tags are pushed)
  * Creates a GitHub release with release notes for the tag.

### Postsubmits

Prow Postsubmits:
* [post-cluster-api-push-images] Google Cloud Build: `make release-staging`

### Periodics

Prow Periodics:
* periodic-cluster-api-test-main `./scripts/ci-test.sh`
* periodic-cluster-api-test-mink8s-main `./scripts/ci-test.sh`
* periodic-cluster-api-e2e-main `./scripts/ci-e2e.sh`
  * GINKGO_SKIP: `[Conformance]|[IPv6]`
* periodic-cluster-api-e2e-mink8s-main `./scripts/ci-e2e.sh`
  * GINKGO_SKIP: `[Conformance]|[IPv6]`
* periodic-cluster-api-e2e-dualstack-and-ipv6-main `./scripts/ci-e2e.sh`
  * DOCKER_IN_DOCKER_IPV6_ENABLED: `true`
  * GINKGO_SKIP: `[Conformance]`
* periodic-cluster-api-e2e-upgrade-* `./scripts/ci-e2e.sh`
  * GINKGO_FOCUS: `[Conformance] [K8s-Upgrade]`
* periodic-cluster-api-e2e-conformance-main `./scripts/ci-e2e.sh`
  * GINKGO_FOCUS: `[Conformance] [K8s-Install]`
* periodic-cluster-api-e2e-conformance-ci-latest-main `./scripts/ci-e2e.sh`
  * GINKGO_FOCUS: `[Conformance] [K8s-Install-ci-latest]`
* [cluster-api-push-images-nightly] Google Cloud Build: `make release-staging-nightly`

## Test-infra configuration

* config/jobs/image-pushing/k8s-staging-cluster-api.yaml
  * Configures nightly and postsubmit jobs to push images and manifests.
* config/jobs/kubernetes-sigs/cluster-api/
  * Configures Cluster API  presubmit and periodic jobs.
* config/testgrids/kubernetes/sig-cluster-lifecycle/config.yaml
  * Configures Cluster API testgrid dashboards.
* config/prow/config.yaml
  * `branch-protection` and `tide` are configured to make the golangci-lint GitHub action mandatory for merge
* config/prow/plugins.yaml
  * `triggers`: configures `/ok-to-test`
  * `approve`: disable auto-approval of PR authors, ignore GitHub reviews (/approve is explicitly required)
  * `milestone_applier`: configures that merged PRs are automatically added to the correct milestone after merge
  * `repo_milestone`: configures `cluster-api-maintainers` as maintainers
  * `require_matching_label`: configures `needs-triage`
  * `plugins`: enables `milestone`, `override` and `require-matching-label` plugins
  * `external_plugins`: enables `cherrypicker`
* label_sync/labels.yaml
  * Configures labels for the `cluster-api` repository.


<!-- links -->
[cluster-api-push-images-nightly]: https://testgrid.k8s.io/sig-cluster-lifecycle-image-pushes#cluster-api-push-images-nightly
[post-cluster-api-push-images]: https://testgrid.k8s.io/sig-cluster-lifecycle-image-pushes#post-cluster-api-push-images
