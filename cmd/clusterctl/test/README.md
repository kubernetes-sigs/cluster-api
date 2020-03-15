# Running the tests

NOTE: the e2e tests may not work on a Mac; they should however work just fine on any Linux distro. Mac support will be added in a follow-up PR.

Currently, overrides are needed for the cluster-api, kubeadm-bootstrap and kubeadm-control-plane providers. The override for the infra provider docker must be removed as it is generated locally by the e2e test script:

	cmd/clusterctl/hack/local-overrides.py
	rm -rf $HOME/.cluster-api/overrides/docker

The entire test suite can be run using the script:

	./run-e2e.sh

To run specific tests, use the `GINKGO_FOCUS`

	GINKGO_FOCUS="clusterctl create cluster" ./run-e2e.sh

## Skip local build of CAPD

By default, the a local capd image will be built and loaded into kind. This can be skipped as so:

	SKIP_DOCKER_BUILD=1 ./run-e2e.sh

You can also specify a pre-build image and skip the build:

	SKIP_DOCKER_BUILD=1 MANAGER_IMAGE=gcr.io/my-project-name/docker-provider-manager-amd64:dev ./run-e2e.sh