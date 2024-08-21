---
name: 🚀 Kubernetes bump
about: "[Only for release team lead] Create an issue to track tasks to support a new Kubernetes minor release."
title: Tasks to bump to Kubernetes v1.<minor-version>
labels: ''
assignees: ''

---

This issue is tracking the tasks that should be implemented **after** the Kubernetes minor release has been released.

## Tasks

**Note:** If feasible we usually cherry-pick the changes back to the latest release series.

### Supporting managing and running on the new Kubernetes version

This section contains tasks to update our book, e2e testing and CI to use and test the new Kubernetes version
as well as changes to Cluster API that we might have to make to support the new Kubernetes version. All of these
changes should be cherry-picked to all release series that will support the new Kubernetes version.

* [ ] Continuously modify CAPD to use early versions of the upcoming Kubernetes release (betas and rcs):
  * Bump the Kubernetes version in `test/*` except for `test/infrastructure/kind/*`.
  * Prior art: https://github.com/kubernetes-sigs/cluster-api/pull/10922
* [ ] Modify CAPD to use the new Kubernetes release after it is GA:
  * Bump the Kubernetes version in `test/*` except for `test/infrastructure/kind/*`.
  * Prior art: https://github.com/kubernetes-sigs/cluster-api/pull/11030
* [ ] Ensure the jobs are adjusted to provide test coverage according to our [support policy](https://cluster-api.sigs.k8s.io/reference/versions.html#supported-kubernetes-versions):

  * At the `.versions`  section in the `cluster-api-prowjob-gen.yaml` file in [test-infra](https://github.com/kubernetes/test-infra/blob/master/config/jobs/kubernetes-sigs/cluster-api/):
    * Add a new entry for the new Kubernetes version
    * Adjust the released Kubernetes's version entry to refer `stable-1.<minor>` instead of `ci/latest-1.<minor>`
    * Check and update the versions for the keys `etcd` and `coreDNS` if necessary:
      * For etcd, see the `DefaultEtcdVersion` kubeadm constant: [e.g. for v1.28.0](https://github.com/kubernetes/kubernetes/blob/v1.28.0/cmd/kubeadm/app/constants/constants.go#L308)
      * For coredns, see the `CoreDNSVersion` kubeadm constant:[e.g. for v1.28.0](https://github.com/kubernetes/kubernetes/blob/v1.28.0/cmd/kubeadm/app/constants/constants.go#L344)
  * For the `.branches.main` section in the `cluster-api-prowjob-gen.yaml` file in [test-infra](https://github.com/kubernetes/test-infra/blob/master/config/jobs/kubernetes-sigs/cluster-api/):
    * For the `.upgrades` section:
      * Drop the oldest upgrade
      * Add a new upgrade entry from the previous to the new Kubernetes version
    * Bump the version set at `.kubernetesVersionManagement` to the new minimum supported management cluster version (This is the image version available as kind image).
    * Bump the version set at `.kubebuilderEnvtestKubernetesVersion` to the new minimum supported management cluster version.
  * Run `make generate-test-infra-prowjobs` to generate the resulting prowjob configuration:

    ```sh
    TEST_INFRA_DIR=../../k8s.io/test-infra make generate-test-infra-prowjobs
    ```

  * Prior art: https://github.com/kubernetes/test-infra/pull/33294

* [ ] Update book:
  * Update supported versions in `versions.md`
  * Prior art: https://github.com/kubernetes-sigs/cluster-api/pull/11030

* [ ] Issues specific to the Kubernetes minor release:
  * Sometimes there are adjustments that we have to make in Cluster API to be able to support
    a new Kubernetes minor version. Please add these issues here when they are identified.

### Bump quickstart and kind image references in CAPD

Prerequisites:

* The target Kubernetes version is GA
* There is a new [kind version with/or a new set of kind images](https://github.com/kubernetes-sigs/kind/releases) for the target Kubernetes version

* [ ] Bump quickstart and kind image references in CAPD:
  * Bump the Kubernetes version in:
    * `docs/*`
    * `Tiltfile`
  * Bump kind image references in CAPD (and also kind if necessary, including the latest images for this kind release)
    * Add new images in the [kind mapper.go](https://github.com/kubernetes-sigs/cluster-api/blob/0f47a19e038ee6b0d3b1e7675a62cdaf84face8c/test/infrastructure/kind/mapper.go#L79).
      * See the [kind releases page](https://github.com/kubernetes-sigs/kind/releases) for the list of released images.
    * Set new default image for the [test framework](https://github.com/kubernetes-sigs/cluster-api/blob/0f47a19e038ee6b0d3b1e7675a62cdaf84face8c/test/framework/bootstrap/kind_provider.go#L40)
    * If code changes are required for CAPD to incorporate the new Kind version, update [kind latestMode](https://github.com/kubernetes-sigs/cluster-api/blob/0f47a19e038ee6b0d3b1e7675a62cdaf84face8c/test/infrastructure/kind/mapper.go#L66)
  * Verify the quickstart manually
  * Prior art: https://github.com/kubernetes-sigs/cluster-api/pull/10610
* [ ] Cherry-pick above PR to the latest release branch.

### Using new Kubernetes dependencies

This section contains tasks to update Cluster API to use the latest Kubernetes Go dependencies and related topics
like using the right Go version and build images. These changes are only made on the main branch. We don't
need them in older releases as they are not necessary to manage workload clusters of the new Kubernetes version or
run the Cluster API controllers on the new Kubernetes version.

* [ ] Ensure there is a new controller-runtime minor release which uses the new Kubernetes Go dependencies.
* [ ] Update our ProwJobs for the `main` branch to use the `kubekins-e2e` with the correct Kubernetes version via [cluster-api-prowjob-gen.yaml](https://github.com/kubernetes/test-infra/blob/master/config/jobs/kubernetes-sigs/cluster-api/cluster-api-prowjob-gen.yaml) and by running `make generate-test-infra-prowjobs`.
  * It is recommended to have one PR for presubmit and one for periodic jobs to reduce the risk of breaking the periodic jobs.
  * Prior art: https://github.com/kubernetes/test-infra/pull/32380
* [ ] Bump the Go version in Cluster API: (if Kubernetes is using a new Go minor version)
  * Search for the currently used Go version across the repository and update it
  * We have to at least modify it in: `hack/ensure-go.sh`, `.golangci.yml`, `cloudbuild*.yaml`, `go.mod`, `Makefile`, `netlify.toml`, `Tiltfile`
  * Prior art: https://github.com/kubernetes-sigs/cluster-api/pull/10452
* [ ] Bumps in Cluster API repo:
  * controller-runtime & controller-tools in go.mod files
  * setup-envtest via `SETUP_ENVTEST_VER` in `Makefile`
  * controller-gen via `CONTROLLER_GEN_VER` in `Makefile`
  * conversion-gen via `CONVERSION_GEN_VER` in `Makefile`
  * Prior art: https://github.com/kubernetes-sigs/cluster-api/pull/10803
* [ ] Bump the Kubernetes version used in integration tests via `KUBEBUILDER_ENVTEST_KUBERNETES_VERSION` in `Makefile`
  * **Note**: This PR should be cherry-picked as well. It is part of this section as it depends on kubebuilder/controller-runtime releases and is not strictly necessary for [Supporting managing and running on the new Kubernetes version](#supporting-managing-and-running-on-the-new-kubernetes-version).
  * Prior art to release envtest binaries: https://github.com/kubernetes-sigs/controller-tools/pull/1032
  * Prior art: #7193

