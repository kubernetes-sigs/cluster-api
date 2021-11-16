# Jobs

This document intents to provide an overview over our jobs running via Prow, GitHub actions and Google Cloud Build.

## Builds and Tests running on the main branch

> NOTE: To see which test jobs execute which tests or e2e tests, you can click on the links which lead to the respective test overviews in testgrid.

### Presubmits

**Legend**:
* ✳️️ jobs that don't have to be run successfully for merge
* ✴️ jobs that are not triggered automatically for every commit

Prow Presubmits:
* [pull-cluster-api-build-main] `./scripts/ci-build.sh`
* ✳️️ ✴️ [pull-cluster-api-make-main] `./scripts/ci-make.sh`
* ✳️️ [pull-cluster-api-apidiff-main] `./scripts/ci-apidiff.sh`
* [pull-cluster-api-verify] `./scripts/ci-verify.sh`
* [pull-cluster-api-test-main] `./scripts/ci-test.sh`
* [pull-cluster-api-test-mink8s-main] `./scripts/ci-test.sh`
* [pull-cluster-api-e2e-main] `./scripts/ci-e2e.sh`
  * GINKGO_FOCUS: `[PR-Blocking]`
* ✳️️ [pull-cluster-api-e2e-ipv6-main] `./scripts/ci-e2e.sh`
  * GINKGO_FOCUS: `[PR-Blocking]`, IP_FAMILY: `IPv6`
* ✳️️ ✴️ [pull-cluster-api-e2e-full-main] `./scripts/ci-e2e.sh`
  * GINKGO_SKIP: `[PR-Blocking] [Conformance] [K8s-Upgrade]` (i.e. "no tags")
* ✳️️ ✴️ [pull-cluster-api-e2e-workload-upgrade-1-22-latest-main] `./scripts/ci-e2e.sh` FROM: `stable-1.22` TO: `ci/latest-1.23`
  * GINKGO_FOCUS: `[K8s-Upgrade]`

GitHub Presubmit Workflows:
* golangci-lint: golangci/golangci-lint-action@v2 (locally via `make lint`)
* verify: kubernetes-sigs/kubebuilder-release-tools@v0.1 verifier

### Postsubmits

Prow Postsubmits:
* [post-cluster-api-push-images] Google Cloud Build: `make release-staging`, `make -C test/infrastructure/docker release-staging`

### Periodics

Prow Periodics:
* [periodic-cluster-api-verify-book-links-main] `make verify-book-links`
* [periodic-cluster-api-test-main] `./scripts/ci-test.sh`
* [periodic-cluster-api-e2e-main] `./scripts/ci-e2e.sh`
  * GINKGO_SKIP: `[Conformance] [K8s-Upgrade]`
* [periodic-cluster-api-e2e-upgrade-v0-3-to-main] `./scripts/ci-e2e.sh`
  * GINKGO_FOCUS: `[clusterctl-Upgrade]`
* [periodic-cluster-api-e2e-upgrade-v1-0-to-main] `./scripts/ci-e2e.sh`
  * GINKGO_FOCUS: `[clusterctl-Upgrade]`
* [periodic-cluster-api-e2e-mink8s-main] `./scripts/ci-e2e.sh`
  * GINKGO_SKIP: `[Conformance] [K8s-Upgrade]`
* [periodic-cluster-api-e2e-workload-upgrade-1-18-1-19-main] `./scripts/ci-e2e.sh` FROM: `stable-1.18` TO: `stable-1.19`
  * GINKGO_FOCUS: `[K8s-Upgrade]`
* [periodic-cluster-api-e2e-workload-upgrade-1-19-1-20-main] `./scripts/ci-e2e.sh` FROM: `stable-1.19` TO: `stable-1.20`
  * GINKGO_FOCUS: `[K8s-Upgrade]`
* [periodic-cluster-api-e2e-workload-upgrade-1-20-1-21-main] `./scripts/ci-e2e.sh` FROM: `stable-1.20` TO: `stable-1.21`
  * GINKGO_FOCUS: `[K8s-Upgrade]`
* [periodic-cluster-api-e2e-workload-upgrade-1-21-1-22-main] `./scripts/ci-e2e.sh` FROM: `stable-1.21` TO: `stable-1.22`
  * GINKGO_FOCUS: `[K8s-Upgrade]`
* [periodic-cluster-api-e2e-workload-upgrade-1-22-latest-main] `./scripts/ci-e2e.sh` FROM: `stable-1.22` TO: `ci/latest-1.23`
  * GINKGO_FOCUS: `[K8s-Upgrade]`
* [cluster-api-push-images-nightly] Google Cloud Build: `make release-staging-nightly`, `make -C test/infrastructure/docker release-staging-nightly`

## Builds and Tests running on releases

GitHub (On Release) Workflows:
* Update Homebrew Formula On Release: dawidd6/action-homebrew-bump-formula@v3 clusterctl

<!-- links -->
[pull-cluster-api-build-main]: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi-pr-build-main
[pull-cluster-api-make-main]: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi-pr-make-main
[pull-cluster-api-apidiff-main]: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi-pr-apidiff-main
[pull-cluster-api-verify]: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi-pr-verify-main
[pull-cluster-api-test-main]: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi-pr-test-main
[pull-cluster-api-test-mink8s-main]: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi-pr-test-mink8s-main
[pull-cluster-api-e2e-main]: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi-pr-e2e-main
[pull-cluster-api-e2e-ipv6-main]: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi-pr-e2e-main-ipv6
[pull-cluster-api-e2e-full-main]: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi-pr-e2e-full-main
[pull-cluster-api-e2e-workload-upgrade-1-22-latest-main]: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi-pr-e2e-main-1-22-latest
[periodic-cluster-api-verify-book-links-main]: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi-verify-book-links-main
[periodic-cluster-api-test-main]: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi-test-main
[periodic-cluster-api-e2e-main]: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi-e2e-main
[periodic-cluster-api-e2e-upgrade-v0-3-to-main]: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi-e2e-upgrade-v0-3-to-main
[periodic-cluster-api-e2e-upgrade-v1-0-to-main]: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi-e2e-upgrade-v1-0-to-main
[periodic-cluster-api-e2e-mink8s-main]: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi-e2e-mink8s-main
[periodic-cluster-api-e2e-workload-upgrade-1-18-1-19-main]: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi-e2e-main-1-18-1-19
[periodic-cluster-api-e2e-workload-upgrade-1-19-1-20-main]: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi-e2e-main-1-19-1-20
[periodic-cluster-api-e2e-workload-upgrade-1-20-1-21-main]: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi-e2e-main-1-20-1-21
[periodic-cluster-api-e2e-workload-upgrade-1-21-1-22-main]: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi-e2e-main-1-21-1-22
[periodic-cluster-api-e2e-workload-upgrade-1-22-latest-main]: https://testgrid.k8s.io/sig-cluster-lifecycle-cluster-api#capi-e2e-main-1-22-latest
[cluster-api-push-images-nightly]: https://testgrid.k8s.io/sig-cluster-lifecycle-image-pushes#cluster-api-push-images-nightly
[post-cluster-api-push-images]: https://testgrid.k8s.io/sig-cluster-lifecycle-image-pushes#post-cluster-api-push-images
