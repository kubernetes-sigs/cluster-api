---
name: 🚀 Kubernetes bump
about: "[Only for release team lead] Create an issue to track tasks to support a new Kubernetes minor release."
title: Tasks to bump to Kubernetes v1.<minor-version>
labels: ''
assignees: ''

---

This issue is tracking the tasks that should be implemented **after** the Kubernetes minor release has been released.

## Tasks

Prerequisites:
* [ ] Decide which Cluster API release series will support the new Kubernetes version
  * If feasible we usually cherry-pick the changes back to the latest release series.

### Supporting managing and running on the new Kubernetes version

This section contains tasks to update our book, e2e testing and CI to use and test the new Kubernetes version
as well as changes to Cluster API that we might have to make to support the new Kubernetes version. All of these
changes should be cherry-picked to all release series that will support the new Kubernetes version.

* [ ] Modify quickstart and CAPD to use the new Kubernetes release:
  * Bump the Kubernetes version in:
    * `test/*`: search for occurrences of the previous Kubernetes version
    * `Tiltfile`
  * Ensure the latest available kind version is used as well.
  * Verify the quickstart manually
  * Prior art: #7156
  * bump `InitWithKubernetesVersion` in `clusterctl_upgrade_test.go`
* [ ] Job configurations:
  * For all releases which will support the new Kubernetes version:
    * Update `INIT_WITH_KUBERNETES_VERSION`.
    * Add new periodic upgrade jobs .
    * Adjust presubmit jobs so that we have the latest upgrade jobs available on PRs.
  * Prior art: https://github.com/kubernetes/test-infra/pull/27421
* [ ] Update book:
  * Update supported versions in `versions.md`
  * Update job documentation in `jobs.md`
  * Prior art: #7194 #7196
* [ ] Issues specific to the Kubernetes minor release:
  * Sometimes there are adjustments that we have to make in Cluster API to be able to support
    a new Kubernetes minor version. Please add these issues here when they are identified.

### Using new Kubernetes dependencies

This section contains tasks to update Cluster API to use the latest Kubernetes Go dependencies and related topics
like using the right Go version and build images. These changes are only made on the main branch. We don't
need them in older releases as they are not necessary to manage workload clusters of the new Kubernetes version or
run the Cluster API controllers on the new Kubernetes version.

* [ ] Ensure there is a new controller-runtime minor release which uses the new Kubernetes Go dependencies.
* [ ] Update our Prow jobs for the `main` branch to use the correct `kubekins-e2e` image
  * It is recommended to have one PR for presubmit and one for periodic jobs to reduce the risk of breaking the periodic jobs.
  * Prior art: presubmit jobs: https://github.com/kubernetes/test-infra/pull/27311
  * Prior art: periodic jobs: https://github.com/kubernetes/test-infra/pull/27311
* [ ] Bump the Go version in Cluster API: (if Kubernetes is using a new Go minor version)
  * Search for the currently used Go version across the repository and update it
  * We have to at least modify it in: `.github/workflows`, `hack/ensure-go.sh`, `.golangci.yml`, `cloudbuild*.yaml`, `go.mod`, `Makefile`, `netlify.toml`, `Tiltfile`
  * Prior art: #7135
* [ ] Bump controller-runtime
* [ ] Bump controller-tools
* [ ] Bump the Kubernetes version used in integration tests via `KUBEBUILDER_ENVTEST_KUBERNETES_VERSION` in `Makefile`
  * **Note**: This PR should be cherry-picked as well. It is part of this section as it depends on kubebuilder/controller-runtime
    releases and is not strictly necessary for [Supporting managing and running on the new Kubernetes version](#supporting-managing-and-running-on-the-new-kubernetes-version).
  * Prior art: #7193
* [ ] Bump conversion-gen via `CONVERSION_GEN_VER` in `Makefile`
  * Prior art: #7118
